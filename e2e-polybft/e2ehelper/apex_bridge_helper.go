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
	"github.com/Ethernal-Tech/cardano-infrastructure/sendtx"
	cardanowallet "github.com/Ethernal-Tech/cardano-infrastructure/wallet"
	"github.com/stretchr/testify/require"
)

func ExecuteSingleBridging(
	t *testing.T, ctx context.Context, apex IApexSystem, senderUser, receiverUser *cardanofw.TestApexUser,
	srcChain, dstChain string, sendAmount *big.Int, bridgingType sendtx.BridgingType, options ...ExecuteBridgingOption,
) {
	t.Helper()

	config := newExecuteBridgingConfig(options...)
	expectNativeTokens := bridgingType == sendtx.BridgingTypeCurrencyOnSource

	balance, err := apex.GetBalance(ctx, receiverUser, dstChain)
	require.NoError(t, err)

	tokenName := getTokenNameForChains(apex, dstChain, srcChain, expectNativeTokens)
	prevAmount := cardanofw.SetOrDefault(balance[tokenName], big.NewInt(0))

	txHash := apex.SubmitBridgingRequest(
		t, ctx, srcChain, dstChain, senderUser, sendAmount, bridgingType, receiverUser)

	fmt.Printf("Tx sent. hash: %s\n", txHash)

	expectedAmount := new(big.Int).Add(prevAmount, sendAmount)

	err = apex.WaitForExactAmount(ctx, receiverUser, dstChain, srcChain, expectedAmount,
		config.timeoutConfig.bridgingNumRetries, config.timeoutConfig.bridgingRetryWaitTime, expectNativeTokens)

	require.NoError(t, err)
}

func ExecuteBridgingOneByOneWaitOnOtherSide(
	t *testing.T, ctx context.Context, apex IApexSystem, txCountPerSender int,
	receiverUser *cardanofw.TestApexUser, srcChain, dstChain string, sendAmount *big.Int, bridgingType sendtx.BridgingType,
	options ...ExecuteBridgingOption,
) {
	t.Helper()

	config := newExecuteBridgingConfig(options...)

	for i := 0; i < txCountPerSender; i++ {
		expectNativeTokens := bridgingType == sendtx.BridgingTypeCurrencyOnSource

		balance, err := apex.GetBalance(ctx, receiverUser, dstChain)
		require.NoError(t, err)

		tokenName := getTokenNameForChains(apex, dstChain, srcChain, expectNativeTokens)
		prevAmount := cardanofw.SetOrDefault(balance[tokenName], big.NewInt(0))

		apex.SubmitBridgingRequest(t, ctx, srcChain, dstChain, receiverUser, sendAmount, bridgingType, receiverUser)

		expectedAmount := new(big.Int).Add(prevAmount, sendAmount)

		err = apex.WaitForExactAmount(ctx, receiverUser, dstChain, srcChain, expectedAmount,
			config.timeoutConfig.bridgingNumRetries, config.timeoutConfig.bridgingRetryWaitTime, expectNativeTokens)

		require.NoError(t, err)
	}
}

func ExecuteBridgingWaitAfterSubmits(
	t *testing.T, ctx context.Context, apex IApexSystem, txCountPerSender int,
	receiverUser *cardanofw.TestApexUser, srcChain, dstChain string, sendAmount *big.Int, bridgingType sendtx.BridgingType,
	options ...ExecuteBridgingOption,
) {
	t.Helper()

	config := newExecuteBridgingConfig(options...)
	expectNativeTokens := bridgingType == sendtx.BridgingTypeCurrencyOnSource

	balance, err := apex.GetBalance(ctx, receiverUser, dstChain)
	require.NoError(t, err)

	tokenName := getTokenNameForChains(apex, dstChain, srcChain, expectNativeTokens)
	prevAmount := cardanofw.SetOrDefault(balance[tokenName], big.NewInt(0))

	expectedAmount := prevAmount

	for i := 0; i < txCountPerSender; i++ {
		apex.SubmitBridgingRequest(t, ctx, srcChain, dstChain, receiverUser, sendAmount, bridgingType, receiverUser)

		expectedAmount = expectedAmount.Add(expectedAmount, sendAmount)
	}

	err = apex.WaitForExactAmount(ctx, receiverUser, dstChain, srcChain, expectedAmount,
		config.timeoutConfig.bridgingNumRetries, config.timeoutConfig.bridgingRetryWaitTime, expectNativeTokens)

	require.NoError(t, err)
}

func ExecuteBridging(
	t *testing.T, ctx context.Context, apex IApexSystem, txCountPerSender int,
	senderUsers []*cardanofw.TestApexUser, receiverUsers []*cardanofw.TestApexUser,
	chains []string, chainsDst map[string][]string, bridgingType sendtx.BridgingType,
	sendAmountDfm *big.Int, options ...ExecuteBridgingOption,
) {
	t.Helper()

	var (
		err        error
		config     = newExecuteBridgingConfig(options...)
		chainPairs = getAllChainPairs(chains, chainsDst)
		// per each receiver -> per each chain -> per each token
		initialAmountsPerRecv = make([]map[string]map[string]*big.Int, len(receiverUsers))
		expectNativeTokens    = bridgingType == sendtx.BridgingTypeCurrencyOnSource
	)

	// calculate receivers initial balances
	for i, receiverUser := range receiverUsers {
		initialAmountsPerRecv[i] = map[string]map[string]*big.Int{}
		balancePerChain := map[string]map[string]*big.Int{}

		for _, pair := range chainPairs {
			balance, exists := balancePerChain[pair.dstChain]
			if !exists {
				balance, err = apex.GetBalance(ctx, receiverUser, pair.dstChain)
				require.NoError(t, err)

				balancePerChain[pair.dstChain] = balance
				initialAmountsPerRecv[i][pair.dstChain] = map[string]*big.Int{}
			}

			tokenName := getTokenNameForChains(apex, pair.dstChain, pair.srcChain, expectNativeTokens)
			initialAmountsPerRecv[i][pair.dstChain][tokenName] = cardanofw.SetOrDefault(balance[tokenName], big.NewInt(0))
		}
	}

	// send transactions
	sendTxDatas := config.sendTxStrategy(
		t, ctx, apex, chainsDst, senderUsers, receiverUsers, sendAmountDfm, txCountPerSender, bridgingType)

	var (
		wgResults              sync.WaitGroup
		lock                   sync.RWMutex
		originalDesiredAmounts = make(map[string]map[string]*big.Int, len(chainPairs))
		desiredAmounts         = make(map[string]map[string]*big.Int, len(chainPairs))
		closeCh                = make(chan struct{})
		txHashTxDataMap        = make(map[string]*SubmittedTxData)
		errs                   = make([]error, len(receiverUsers)*len(chainPairs))
		srcChainMap            = map[string]string{}
	)

	// calculate desired amounts per chain
	for _, txData := range sendTxDatas {
		tokenName := getTokenNameForChains(apex, txData.DstChainID, txData.SrcChainID, expectNativeTokens)

		if _, exists := originalDesiredAmounts[txData.DstChainID]; !exists {
			originalDesiredAmounts[txData.DstChainID] = make(map[string]*big.Int)
		}

		if _, exists := originalDesiredAmounts[txData.DstChainID][tokenName]; !exists {
			originalDesiredAmounts[txData.DstChainID][tokenName] = big.NewInt(0)
		}

		originalDesiredAmounts[txData.DstChainID][tokenName].Add(
			originalDesiredAmounts[txData.DstChainID][tokenName], txData.SendAmountDfm)

		txHashTxDataMap[txData.TxHash] = txData
	}

	for chainID, perTokenMap := range originalDesiredAmounts {
		desiredAmounts[chainID] = make(map[string]*big.Int, len(perTokenMap))
		for token, amount := range perTokenMap {
			desiredAmounts[chainID][token] = new(big.Int).Set(amount)
		}
	}

	// recalculate desired amounts every N seconds
	go func() {
		for {
			select {
			case <-time.After(time.Second * 10):
			case <-closeCh:
				return
			}

			for _, chainPair := range chainPairs {
				dstChain := chainPair.dstChain
				sum := new(big.Int)

				tokenName := getTokenNameForChains(apex, dstChain, chainPair.srcChain, expectNativeTokens)

				// Retrieve all failed transactions on the source chain, if any
				for _, txHash := range apex.GetChainMust(t, chainPair.srcChain).GetIndexer().GetFailedTxs() {
					sum.Add(sum, txHashTxDataMap[txHash].SendAmountDfm)
				}

				lock.Lock()
				// Subtract failed transaction amounts from the original desired amounts on the destination chain
				desiredAmounts[dstChain][tokenName].Sub(originalDesiredAmounts[dstChain][tokenName], sum)
				lock.Unlock()
			}
		}
	}()

	// prepare the map (dstChain + tokenName -> sourceChain)
	for _, pair := range chainPairs {
		tokenName := getTokenNameForChains(apex, pair.dstChain, pair.srcChain, expectNativeTokens)
		key := fmt.Sprintf("%s-%s", pair.dstChain, tokenName)
		// It doesn't matter if two source chains have the same token name (e.g., "lovelace")
		// for the same destination chain — just pick any one.
		srcChainMap[key] = pair.srcChain
	}

	// wait for amounts
	for i, userRecv := range receiverUsers {
		for j, chainPair := range chainPairs {
			tokenName := getTokenNameForChains(apex, chainPair.dstChain, chainPair.srcChain, expectNativeTokens)

			wgResults.Add(1)

			go func(idx, idxChain int, receiver *cardanofw.TestApexUser,
				dstChain string, srcChain string, initialAmountDfm *big.Int) {
				defer wgResults.Done()

				bigIntCache := new(big.Int)

				getDesiredAmount := func() *big.Int {
					lock.RLock()
					defer lock.RUnlock()

					receivedAmount := bigIntCache.Add(bigIntCache.Set(initialAmountDfm), desiredAmounts[dstChain][tokenName])

					fmt.Printf("TXs on %s for user %d expected amount to receive %s\n", dstChain, idx, receivedAmount)

					return receivedAmount
				}

				receivedAmount, err := apex.WaitForAmount(
					ctx, receiver, dstChain, srcChain, func(currentAmount *big.Int) bool {
						return currentAmount.Cmp(getDesiredAmount()) == 0
					},
					len(receiverUsers)*config.timeoutConfig.bridgingNumRetries,
					config.timeoutConfig.bridgingRetryWaitTime,
					expectNativeTokens,
				)
				if err != nil {
					errs[idx*len(chainPairs)+idxChain] = fmt.Errorf("receiver %d on %s (%s vs %s): %w",
						idx, dstChain, receivedAmount, getDesiredAmount(), err)

					return
				}

				fmt.Printf("TXs on %s for user %d expected amount received %s\n", dstChain, idx, receivedAmount)

				if config.waitForUnexpectedBridges {
					// nothing else should be bridged for 2 minutes
					err := apex.WaitForGreaterAmount(
						ctx, receiver, dstChain, srcChain, receivedAmount,
						config.timeoutConfig.unexpectedBridgesNumRetries, config.timeoutConfig.unexpectedBridgesRetryWaitTime,
						expectNativeTokens)
					if !errors.Is(err, infracommon.ErrRetryTimeout) {
						lock.Lock()
						errs = append(errs, fmt.Errorf(
							"receiver %d on %s->%s received more than expected tokens: %w", idx, srcChain, dstChain, err))
						lock.Unlock()

						return
					}

					fmt.Printf("TXs on %s for user %d finished with success\n", dstChain, idx)
				}
			}(i, j, userRecv, chainPair.dstChain,
				srcChainMap[fmt.Sprintf("%s-%s", chainPair.dstChain, tokenName)],
				initialAmountsPerRecv[i][chainPair.dstChain][tokenName])
		}
	}

	wgResults.Wait()

	close(closeCh)

	require.NoError(t, errors.Join(errs...))
}

func getTokenNameForChains(apex IApexSystem, dstChain, srcChain string, expectNativeTokens bool) string {
	if expectNativeTokens {
		return apex.GetTokenNameForChains(dstChain, srcChain)
	}

	return cardanowallet.AdaTokenName
}
