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
	"github.com/0xPolygon/polygon-edge/e2e-polybft/e2eindexer"
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
	t *testing.T, ctx context.Context, apex IApexSystem, txCountPerSender int,
	senderUsers []*cardanofw.TestApexUser, receiverUsers []*cardanofw.TestApexUser,
	chains []string, chainsDst map[string][]string, sendAmountDfm *big.Int,
	options ...ExecuteBridgingOption,
) {
	t.Helper()

	config := newExecuteBridgingConfig(options...)
	dstChains := getAllDestionationChains(chains, chainsDst)
	chainPairs := getAllChainPairs(chains, chainsDst)

	var (
		initialReceiverAmounts = make([]map[string]*big.Int, len(receiverUsers))
		txExecutedComponents   = make(map[string]e2eindexer.TxsExecutedComponent)
		err                    error
	)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	for _, chain := range chains {
		if config.runIndexerInstance {
			txExecutedComponents[chain], err = apex.GetChainMust(t, chain).CreateIndexer(config.logger)
			require.NoError(t, err)
		} else {
			txExecutedComponents[chain] = e2eindexer.NewTxsExecutedComponentDummy()
		}
	}

	defer func() {
		for _, comp := range txExecutedComponents {
			comp.Close()
		}
	}()

	for i, receiverUser := range receiverUsers {
		initialReceiverAmounts[i] = make(map[string]*big.Int)

		for _, dstChain := range dstChains {
			dfm, err := apex.GetBalance(ctx, receiverUser, dstChain)
			require.NoError(t, err)

			initialReceiverAmounts[i][dstChain] = dfm
		}
	}

	sendTxDataPerReceiver := config.sendTxStrategy(
		t, ctx, apex, chainPairs, senderUsers, receiverUsers, sendAmountDfm, txCountPerSender, txExecutedComponents)

	config.restartValidatorStrategy(t, ctx, apex, config.restartValidatorsConfigs)

	var (
		wgResults sync.WaitGroup
		errs      = make([]error, len(receiverUsers)*len(dstChains))
	)

	for i, user := range receiverUsers {
		for j, dstChain := range dstChains {
			wgResults.Add(1)

			go func(
				idx int, idxChain int, receiverUser *cardanofw.TestApexUser, dstChain string,
				initialAmountDfm *big.Int, txsData []SubmittedTxData,
			) {
				defer wgResults.Done()

				receivedAmount, err := apex.WaitForAmount(
					ctx, receiverUser, dstChain, func(currentAmount *big.Int) bool {
						desiredAmount := getDesiredAmount(txExecutedComponents, initialAmountDfm, txsData)

						return currentAmount.Cmp(desiredAmount) == 0
					},
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
						ctx, receiverUser, dstChain, receivedAmount, 12, time.Second*10)
					if !errors.Is(err, infracommon.ErrRetryTimeout) {
						errs[idx*len(dstChains)+idxChain] = fmt.Errorf(
							"receiver %d on %s should not receive more tokens: %w", idx, dstChain, err)

						return
					}

					fmt.Printf("TXs on %s for user %d finished with success\n", dstChain, idx)
				}
			}(i, j, user, dstChain, initialReceiverAmounts[i][dstChain], sendTxDataPerReceiver[i][dstChain])
		}
	}

	wgResults.Wait()

	require.NoError(t, errors.Join(errs...))
}

func getDesiredAmount(
	txsExecutedComponents map[string]e2eindexer.TxsExecutedComponent,
	initialAmountDfm *big.Int, txsData []SubmittedTxData,
) *big.Int {
	failedTxsPerChain := map[string]map[string]bool{}
	expectedAmount := new(big.Int).Set(initialAmountDfm)

	for _, txData := range txsData {
		failedTxs, exists := failedTxsPerChain[txData.SrcChainID]
		if !exists {
			failedTxsSlice := txsExecutedComponents[txData.SrcChainID].GetTxs().Failed
			failedTxs = make(map[string]bool, len(failedTxs))

			for _, x := range failedTxsSlice {
				failedTxs[x] = true
			}

			failedTxsPerChain[txData.SrcChainID] = failedTxs
		}

		if !failedTxs[txData.TxHash] {
			expectedAmount.Add(expectedAmount, txData.SendAmountDfm)
		}
	}

	return expectedAmount
}
