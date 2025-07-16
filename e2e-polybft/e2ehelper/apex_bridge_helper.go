package e2ehelper

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/e2e-polybft/cardanofw"
	infracommon "github.com/Ethernal-Tech/cardano-infrastructure/common"
	"github.com/stretchr/testify/require"
)

func ExecuteSingleBridging(
	t *testing.T, ctx context.Context, apex IApexSystem, senderUser, receiverUser *cardanofw.TestApexUser,
	srcChain, dstChain string, sendAmountDfm *big.Int, options ...ExecuteBridgingOption,
) {
	t.Helper()

	config := newExecuteBridgingConfig(options...)

	prevAmountDfm, err := apex.GetBalance(ctx, receiverUser, dstChain)
	require.NoError(t, err)

	txHash := apex.SubmitBridgingRequest(
		t, ctx, srcChain, dstChain, senderUser, sendAmountDfm, receiverUser)
	expectedAmountDfm := new(big.Int).Add(prevAmountDfm, sendAmountDfm)

	fmt.Printf("Tx sent. hash: %s\n", txHash)

	// check expected amount cardano
	err = apex.WaitForExactAmount(ctx, receiverUser, dstChain, expectedAmountDfm,
		config.timeoutConfig.bridgingNumRetries, config.timeoutConfig.bridgingRetryWaitTime)
	require.NoError(t, err)
}

func ExecuteBridgingOneByOneWaitOnOtherSide(
	t *testing.T, ctx context.Context, apex IApexSystem, txCountPerSender int,
	user *cardanofw.TestApexUser, srcChain, dstChain string, sendAmountDfm *big.Int, options ...ExecuteBridgingOption,
) {
	t.Helper()

	config := newExecuteBridgingConfig(options...)

	for i := 0; i < txCountPerSender; i++ {
		prevAmountDfm, err := apex.GetBalance(ctx, user, dstChain)
		require.NoError(t, err)

		apex.SubmitBridgingRequest(t, ctx, srcChain, dstChain, user, sendAmountDfm, user)
		expectedAmountDfm := new(big.Int).Add(prevAmountDfm, sendAmountDfm)

		err = apex.WaitForExactAmount(ctx, user, dstChain, expectedAmountDfm,
			config.timeoutConfig.bridgingNumRetries, config.timeoutConfig.bridgingRetryWaitTime)
		require.NoError(t, err)
	}
}

func ExecuteBridgingWaitAfterSubmits(
	t *testing.T, ctx context.Context, apex IApexSystem, txCountPerSender int,
	user *cardanofw.TestApexUser, srcChain, dstChain string, sendAmountDfm *big.Int, options ...ExecuteBridgingOption,
) {
	t.Helper()

	config := newExecuteBridgingConfig(options...)

	prevAmountDfm, err := apex.GetBalance(ctx, user, dstChain)
	require.NoError(t, err)

	expectedAmountDfm := new(big.Int).Set(prevAmountDfm)

	for i := 0; i < txCountPerSender; i++ {
		apex.SubmitBridgingRequest(t, ctx, srcChain, dstChain, user, sendAmountDfm, user)
		expectedAmountDfm = expectedAmountDfm.Add(expectedAmountDfm, sendAmountDfm)
	}

	err = apex.WaitForExactAmount(ctx, user, dstChain, expectedAmountDfm,
		config.timeoutConfig.bridgingNumRetries, config.timeoutConfig.bridgingRetryWaitTime)
	require.NoError(t, err)
}

func ExecuteBridging(
	t *testing.T, ctx context.Context, apex IApexSystem,
	chainConfigs map[string]*cardanofw.TestCardanoChainConfig,
	chainInfos map[string]*cardanofw.CardanoChainInfo, txCountPerSender int,
	senderUsers []*cardanofw.TestApexUser, receiverUsers []*cardanofw.TestApexUser,
	chains []string, chainsDst map[string][]string,
	sendAmountDfm *big.Int, options ...ExecuteBridgingOption,
) {
	t.Helper()

	config := newExecuteBridgingConfig(options...)
	dstChains := getAllDestionationChains(chains, chainsDst)
	chainPairs := getAllChainPairs(chains, chainsDst)
	expectedAmountPerChainDfm := make([]map[string]*big.Int, len(receiverUsers))

	var (
		observedTxs = make(map[int][]string)
		mu          sync.Mutex
	)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	if config.runIndexerInstance {
		indexerDBs, err := initIndexerDBs(chains)
		require.NoError(t, err)

		for _, chain := range chains {
			cardanoTxObserver, err := NewCardanoTxObserver(
				ctx, chainConfigs[chain],
				chainInfos[chain], indexerDBs[chain],
			)
			require.NoError(t, err)

			err = cardanoTxObserver.Start()
			require.NoError(t, err)

			// Launch listener goroutine
			go func(txChan <-chan channelMsg) {
				for {
					select {
					case <-ctx.Done():
						return
					case msg, ok := <-txChan:
						if !ok {
							fmt.Printf("ERROR: Failed to receive a channel message: %v", msg)
							return
						}
						mu.Lock()
						observedTxs[chainConfigs[chain].ID] = append(observedTxs[chainConfigs[chain].ID], msg.txHash.String())
						mu.Unlock()
					}
				}
			}(cardanoTxObserver.TxChan())
		}
	}

	for i, receiverUser := range receiverUsers {
		expectedAmountPerChainDfm[i] = make(map[string]*big.Int)

		for _, dstChain := range dstChains {
			dfm, err := apex.GetBalance(ctx, receiverUser, dstChain)
			require.NoError(t, err)

			expectedAmountPerChainDfm[i][dstChain] = dfm
		}
	}

	sentTxHashes := config.sendTxStrategy(t, ctx, apex, chainPairs, senderUsers, receiverUsers, sendAmountDfm, txCountPerSender)

	// Sleep for some time so indexer can observe all the transactions and update the expected amounts
	select {
	case <-ctx.Done():
		return
	case <-time.After(defaultObservingWaitTime):
	}

	// update expectedAmountPerChainDfm
	for recieverUserIdx := range receiverUsers {
		for _, chainPair := range chainPairs {
			tmp := expectedAmountPerChainDfm[recieverUserIdx][chainPair.dstChain]
			tmp.Add(tmp, new(big.Int).Mul(sendAmountDfm, big.NewInt(int64(txCountPerSender)*int64(len(senderUsers)))))
		}
	}

	if config.runIndexerInstance {
		// check whether expectedAmountPerChainDfm should be updated
		for recieverUserIdx, receiver := range receiverUsers {
			for _, chain := range chains {
				for _, sentTxHash := range sentTxHashes[chain][receiver] {
					if !slices.Contains(observedTxs[chainConfigs[chain].ID], sentTxHash) {
						// tx is rolled back, we need to update the users expected amount on this chain
						destChain := getDestinationChain(chainPairs, chain)

						oldExpectedValue := expectedAmountPerChainDfm[recieverUserIdx][destChain]
						newValue := new(big.Int).Sub(oldExpectedValue, sendAmountDfm)

						expectedAmountPerChainDfm[recieverUserIdx][destChain] = newValue

						fmt.Printf("\nTxHash %s not found in observed transactions\n", sentTxHash)
						fmt.Printf("\nUpdated expected amount for user idx %d on chain %s: %v\nTime: %v", recieverUserIdx, destChain, observedTxs[chainConfigs[chain].ID], time.Now())
					} else {
						fmt.Printf("\nSent transaction %s is found in observed ones: %v\n", sentTxHash, observedTxs[chainConfigs[chain].ID])
					}
				}
			}
		}
	}

	config.restartValidatorStrategy(t, ctx, apex, config.restartValidatorsConfigs)

	var (
		wgResults sync.WaitGroup
		errs      = make([]error, len(receiverUsers)*len(dstChains))
	)

	for i, user := range receiverUsers {
		for j, dstChain := range dstChains {
			wgResults.Add(1)

			go func(idx int, idxChain int, receiverUser *cardanofw.TestApexUser, dstChain string, expectedAmountDfm *big.Int) {
				defer wgResults.Done()

				err := apex.WaitForExactAmount(
					ctx, receiverUser, dstChain, expectedAmountDfm,
					len(receiverUsers)*config.timeoutConfig.bridgingNumRetries,
					config.timeoutConfig.bridgingRetryWaitTime)
				if err != nil {
					errs[idx*len(dstChains)+idxChain] = fmt.Errorf("receiver %d on %s: %w", idx, dstChain, err)

					return
				}

				fmt.Printf("TXs on %s for user %d expected amount received\n", dstChain, idx)

				if config.waitForUnexpectedBridges {
					// nothing else should be bridged for 2 minutes
					err = apex.WaitForGreaterAmount(
						ctx, receiverUser, dstChain, expectedAmountDfm, 12, time.Second*10)
					if !errors.Is(err, infracommon.ErrRetryTimeout) {
						errs[idx*len(dstChains)+idxChain] = fmt.Errorf(
							"receiver %d on %s should not receive more tokens: %w", idx, dstChain, err)

						return
					}

					fmt.Printf("TXs on %s for user %d finished with success\n", dstChain, idx)
				}
			}(i, j, user, dstChain, expectedAmountPerChainDfm[i][dstChain])
		}
	}

	wgResults.Wait()

	require.NoError(t, errors.Join(errs...))
}
