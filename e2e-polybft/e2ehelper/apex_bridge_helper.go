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
	"github.com/0xPolygon/polygon-edge/e2e-polybft/e2eindexer"
	infracommon "github.com/Ethernal-Tech/cardano-infrastructure/common"
	"github.com/hashicorp/go-hclog"
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
		t, ctx, srcChain, dstChain, senderUser, sendAmountDfm, nil, receiverUser)
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

		apex.SubmitBridgingRequest(t, ctx, srcChain, dstChain, user, sendAmountDfm, nil, user)
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
		apex.SubmitBridgingRequest(t, ctx, srcChain, dstChain, user, sendAmountDfm, nil, user)
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
	chains []string, chainsDst map[string][]string, sendAmountDfm *big.Int,
	logger hclog.Logger, options ...ExecuteBridgingOption,
) {
	t.Helper()

	config := newExecuteBridgingConfig(options...)
	dstChains := getAllDestionationChains(chains, chainsDst)
	chainPairs := getAllChainPairs(chains, chainsDst)
	expectedAmountPerChainDfm := make([]map[string]*big.Int, len(receiverUsers))

	var (
		txExecutedComponents = make(map[string]*e2eindexer.TxsExecutedComponent)
		err                  error
	)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	if config.runIndexerInstance {
		for _, chain := range chains {
			if chain == cardanofw.ChainIDNexus {
				// we want indexer to run only for cardano chains
				continue
			}
			indexerConfig, syncerConfig := loadSyncerConfigs(chainConfigs[chain], chainInfos[chain])

			txExecutedComponents[chain], err = e2eindexer.NewTxsExecutedComponent(
				syncerConfig, *indexerConfig.StartingBlockPoint, nil, logger)
			require.NoError(t, err)
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

	sentTxHashes := config.sendTxStrategy(t, ctx, apex, chainPairs, senderUsers, receiverUsers,
		sendAmountDfm, txCountPerSender, txExecutedComponents)

	config.restartValidatorStrategy(t, ctx, apex, config.restartValidatorsConfigs)

	var (
		wgResults sync.WaitGroup
		errs      = make([]error, len(receiverUsers)*len(dstChains))

		processedChains int
	)

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	if config.runIndexerInstance {
		for {
			<-ticker.C

			processedChains = 0

			for _, chainPair := range chainPairs {
				// if chain source is nexus, we just mark it as processed
				if chainPair.srcChain == cardanofw.ChainIDNexus {
					processedChains++
				} else if txExecutedComponents[chainPair.srcChain].GetTxs().IsEverythingProcessed() {
					processedChains++
				}
			}

			if processedChains == len(chainPairs) {
				break
			}
		}
	}

	// update expectedAmountPerChainDfm
	for recieverUserIdx := range receiverUsers {
		for _, chainPair := range chainPairs {
			// expected amount of user on destination chain (currently user's balance on destination chain)
			expectedUsrChainAmount := expectedAmountPerChainDfm[recieverUserIdx][chainPair.dstChain]

			if config.runIndexerInstance && chainPair.srcChain != cardanofw.ChainIDNexus {
				sentTxsForReceiver := sentTxHashes[chainPair.srcChain][recieverUserIdx]
				txsInfo := txExecutedComponents[chainPair.srcChain].GetTxs()

				for _, txHash := range txsInfo.Executed {
					if slices.Contains(sentTxsForReceiver, txHash.String()) {
						expectedUsrChainAmount.Add(expectedUsrChainAmount, sendAmountDfm)
					}
				}
			} else {
				expectedUsrChainAmount.Add(expectedUsrChainAmount,
					new(big.Int).Mul(sendAmountDfm, big.NewInt(int64(txCountPerSender)*int64(len(senderUsers)))))
			}

			expectedAmountPerChainDfm[recieverUserIdx][chainPair.dstChain] = expectedUsrChainAmount
		}
	}

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
