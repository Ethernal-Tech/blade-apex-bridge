package e2ehelper

import (
	"context"
	"errors"
	"fmt"
	"math/big"
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

	txHash := apex.SubmitBridgingRequest(t, ctx, srcChain, dstChain, senderUser, sendAmountDfm, receiverUser)
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
	t *testing.T, ctx context.Context, apex IApexSystem, txCountPerSender int,
	senderUsers []*cardanofw.TestApexUser, receiverUsers []*cardanofw.TestApexUser,
	chains []string, chainsDst map[string][]string, sendAmountDfm *big.Int,
	options ...ExecuteBridgingOption,
) {
	t.Helper()

	config := newExecuteBridgingConfig(options...)
	dstChains := getAllDestionationChains(chains, chainsDst)
	chainPairs := getAllChainPairs(chains, chainsDst)
	initialReceiverAmounts := make([]map[string]*big.Int, len(receiverUsers))

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	for i, receiverUser := range receiverUsers {
		initialReceiverAmounts[i] = make(map[string]*big.Int)

		for _, dstChain := range dstChains {
			dfm, err := apex.GetBalance(ctx, receiverUser, dstChain)
			require.NoError(t, err)

			initialReceiverAmounts[i][dstChain] = dfm
		}
	}

	sendTxDatas := config.sendTxStrategy(
		t, ctx, apex, chainPairs, senderUsers, receiverUsers, sendAmountDfm, txCountPerSender)

	config.restartValidatorStrategy(t, ctx, apex, config.restartValidatorsConfigs)

	var (
		wgResults              sync.WaitGroup
		lock                   sync.RWMutex
		originalDesiredAmounts = make(map[string]*big.Int, len(dstChains))
		desiredAmounts         = make(map[string]*big.Int, len(dstChains))
		closeCh                = make(chan struct{})
		txHashTxDataMap        = make(map[string]*SubmittedTxData)
		errs                   = make([]error, len(receiverUsers)*len(dstChains))
	)
	// calculate desired amounts per chain
	for _, txData := range sendTxDatas {
		if _, exists := originalDesiredAmounts[txData.DstChainID]; !exists {
			originalDesiredAmounts[txData.DstChainID] = big.NewInt(0)
		}

		originalDesiredAmounts[txData.DstChainID].Add(originalDesiredAmounts[txData.DstChainID], txData.SendAmountDfm)
		txHashTxDataMap[txData.TxHash] = txData
	}
	// set initial desired amounts
	for _, chainID := range dstChains {
		desiredAmounts[chainID] = new(big.Int).Set(originalDesiredAmounts[chainID])
	}
	// recalculate desired amounts every N seconds
	go func() {
		for {
			select {
			case <-time.After(time.Second * 10):
			case <-closeCh:
				return
			}

			for _, chainID := range dstChains {
				sum := new(big.Int)

				for _, txHash := range apex.GetChainMust(t, chainID).GetIndexer().GetFailedTxs() {
					sum.Add(sum, txHashTxDataMap[txHash].SendAmountDfm)
				}

				lock.Lock()
				desiredAmounts[chainID].Sub(originalDesiredAmounts[chainID], sum)
				lock.Unlock()
			}
		}
	}()

	for i, user := range receiverUsers {
		for j, dstChain := range dstChains {
			wgResults.Add(1)

			go func(
				idx int, idxChain int, receiverUser *cardanofw.TestApexUser,
				dstChain string, initialAmountDfm *big.Int,
			) {
				defer wgResults.Done()

				bigIntCache := new(big.Int)

				getDesiredAmount := func() *big.Int {
					lock.RLock()
					defer lock.RUnlock()

					return bigIntCache.Add(bigIntCache.Set(initialAmountDfm), desiredAmounts[dstChain])
				}

				receivedAmount, err := apex.WaitForAmount(
					ctx, receiverUser, dstChain, func(currentAmount *big.Int) bool {
						return currentAmount.Cmp(getDesiredAmount()) == 0
					},
					len(receiverUsers)*config.timeoutConfig.bridgingNumRetries,
					config.timeoutConfig.bridgingRetryWaitTime)
				if err != nil {
					errs[idx*len(dstChains)+idxChain] = fmt.Errorf("receiver %d on %s (%s vs %s): %w",
						idx, dstChain, receivedAmount, getDesiredAmount(), err)

					return
				}

				fmt.Printf("TXs on %s for user %d expected amount received %s\n", dstChain, idx, receivedAmount)

				if config.waitForUnexpectedBridges {
					// nothing else should be bridged for 2 minutes
					err = apex.WaitForGreaterAmount(
						ctx, receiverUser, dstChain, receivedAmount, 12, time.Second*10)
					if !errors.Is(err, infracommon.ErrRetryTimeout) {
						errs[idx*len(dstChains)+idxChain] = fmt.Errorf(
							"receiver %d on %s should not receive more tokens: %w", idx, dstChain, err)

						return
					}

					fmt.Printf("TXs on %s for user %d finished with success\n", dstChain, idx)
				}
			}(i, j, user, dstChain, initialReceiverAmounts[i][dstChain])
		}
	}

	wgResults.Wait()

	close(closeCh)

	require.NoError(t, errors.Join(errs...))
}
