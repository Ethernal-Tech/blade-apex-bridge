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
	fmt.Printf("Receiver balance: %+v\n", balance)
	require.NoError(t, err)

	tokensInfo := apex.GetBridgingTokensInfo(srcChain, dstChain, expectNativeTokens, config.coloredCoins...)
	fmt.Printf("Tokens Info: %+v\n", tokensInfo)

	prevAmount := cardanofw.SetOrDefault(balance[tokensInfo.DstTokenName], big.NewInt(0))

	txHash, err := apex.SubmitBridgingRequest(
		ctx, srcChain, dstChain, senderUser, sendAmount, bridgingType, receiverUser)
	require.NoError(t, err)

	fmt.Printf("Tx sent. hash: %s\n", txHash)

	expectedAmount := new(big.Int).Add(prevAmount, sendAmount)

	fmt.Printf("Expected amount: %+v\n", expectedAmount)

	err = apex.WaitForExactAmount(ctx, receiverUser, dstChain, srcChain, expectedAmount,
		config.timeoutConfig.bridgingNumRetries, config.timeoutConfig.bridgingRetryWaitTime, expectNativeTokens)

	require.NoError(t, err)
}

func ExecuteTokenRedistribution(
	t *testing.T, ctx context.Context, apex IApexSystem, chainID string, numRetries int, waitTime time.Duration,
) {
	t.Helper()

	err := apex.RedistributeTokens(ctx, chainID)
	require.NoError(t, err)

	err = apex.WaitForRedistribution(ctx, chainID, IsDiffGreaterThanOne, numRetries, waitTime)
	require.NoError(t, err)
}

func IsDiffGreaterThanOne(a, b *big.Int) bool {
	diff := new(big.Int).Sub(a, b)
	if diff.Sign() < 0 {
		diff.Neg(diff) // Make it absolute
	}

	return diff.Cmp(big.NewInt(1)) > 0
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

		txHash, err := apex.SubmitBridgingRequest(
			ctx, srcChain, dstChain, receiverUser, sendAmount, bridgingType, receiverUser)
		require.NoError(t, err)

		fmt.Printf("Tx sent. hash: %s\n", txHash)

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
		txHash, err := apex.SubmitBridgingRequest(
			ctx, srcChain, dstChain, receiverUser, sendAmount, bridgingType, receiverUser)
		require.NoError(t, err)

		fmt.Printf("Tx[%d] sent. hash: %s\n", i, txHash)

		expectedAmount = expectedAmount.Add(expectedAmount, sendAmount)
	}

	err = apex.WaitForExactAmount(ctx, receiverUser, dstChain, srcChain, expectedAmount,
		config.timeoutConfig.bridgingNumRetries, config.timeoutConfig.bridgingRetryWaitTime, expectNativeTokens)

	require.NoError(t, err)
}

func ExecuteBridging(
	t *testing.T, ctx context.Context, apex IApexSystem, txCountPerSender int,
	senderUsers []*cardanofw.TestApexUser, receiverUsers []*cardanofw.TestApexUser,
	chains []string, chainsDst map[string][]string, bridgingTypes map[SrcDstChainPair]sendtx.BridgingType,
	sendAmountDfm *big.Int, options ...ExecuteBridgingOption,
) {
	t.Helper()

	var (
		err        error
		config     = newExecuteBridgingConfig(options...)
		chainPairs = getAllChainPairs(chains, chainsDst)
		// per each receiver -> per each chain -> per each token
		initialAmountsPerRecv = make([]map[string]map[string]*big.Int, len(receiverUsers))
		expectNativeTokens    = func(bridgingType sendtx.BridgingType) bool {
			return bridgingType == sendtx.BridgingTypeCurrencyOnSource
		}
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

			tokenName := getTokenNameForChains(
				apex, pair.dstChain, pair.srcChain, expectNativeTokens(bridgingTypes[pair]))
			initialAmountsPerRecv[i][pair.dstChain][tokenName] = cardanofw.SetOrDefault(balance[tokenName], big.NewInt(0))
		}
	}

	// send transactions
	sendTxDatas := config.sendTxStrategy(
		t, ctx, apex, chainsDst, senderUsers, receiverUsers, sendAmountDfm, txCountPerSender, bridgingTypes)

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
		tokenName := getTokenNameForChains(
			apex, txData.DstChainID, txData.SrcChainID, expectNativeTokens(txData.BridgingTxType))

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
				sum := new(big.Int)

				tokenName := getTokenNameForChains(
					apex, chainPair.dstChain, chainPair.srcChain, expectNativeTokens(bridgingTypes[chainPair]))

				// Retrieve all failed transactions on the source chain, if any
				for _, txHash := range apex.GetChainMust(t, chainPair.srcChain).GetIndexer().GetFailedTxs() {
					// check whether failed transaction is one of these sent from the users (ignore funding transaction rollbacks)
					if _, exists := txHashTxDataMap[txHash]; exists {
						sum.Add(sum, txHashTxDataMap[txHash].SendAmountDfm)
					}
				}

				lock.Lock()
				oldValue := new(big.Int).Set(desiredAmounts[chainPair.dstChain][tokenName])

				// Subtract failed transaction amounts from the original desired amounts on the destination chain
				desiredAmounts[chainPair.dstChain][tokenName].Sub(originalDesiredAmounts[chainPair.dstChain][tokenName], sum)

				newValue := desiredAmounts[chainPair.dstChain][tokenName]
				isDifferent := oldValue.Cmp(newValue) != 0

				lock.Unlock()

				if isDifferent {
					fmt.Printf("Desired amount for %s is %d (was %d)", chainPair.dstChain, newValue, oldValue)
				}
			}
		}
	}()

	// prepare the map (dstChain + tokenName -> sourceChain)
	for _, pair := range chainPairs {
		tokenName := getTokenNameForChains(
			apex, pair.dstChain, pair.srcChain, expectNativeTokens(bridgingTypes[pair]))
		key := fmt.Sprintf("%s-%s", pair.dstChain, tokenName)
		// It doesn't matter if two source chains have the same token name (e.g., "lovelace")
		// for the same destination chain — just pick any one.
		srcChainMap[key] = pair.srcChain
	}

	// wait for amounts
	for i, userRecv := range receiverUsers {
		for j, chainPair := range chainPairs {
			tokenName := getTokenNameForChains(
				apex, chainPair.dstChain, chainPair.srcChain, expectNativeTokens(bridgingTypes[chainPair]))

			wgResults.Add(1)

			go func(idx, idxChain int, receiver *cardanofw.TestApexUser,
				dstChain string, srcChain string, initialAmountDfm *big.Int) {
				defer wgResults.Done()

				bigIntCache := new(big.Int)

				getDesiredAmount := func() *big.Int {
					lock.RLock()
					defer lock.RUnlock()

					receivedAmount := bigIntCache.Add(bigIntCache.Set(initialAmountDfm), desiredAmounts[dstChain][tokenName])

					return receivedAmount
				}

				receivedAmount, err := apex.WaitForAmount(
					ctx, receiver, dstChain, srcChain, func(currentAmount *big.Int) bool {
						return currentAmount.Cmp(getDesiredAmount()) == 0
					},
					len(receiverUsers)*config.timeoutConfig.bridgingNumRetries,
					config.timeoutConfig.bridgingRetryWaitTime,
					expectNativeTokens(bridgingTypes[NewChainPair(srcChain, dstChain)]),
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
						expectNativeTokens(bridgingTypes[NewChainPair(srcChain, dstChain)]))
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

// Return token name of destination chain token from source chain and if it's native token on dest
func getTokenNameForChains(apex IApexSystem, dstChain, srcChain string, expectNativeTokens bool) string {
	if expectNativeTokens {
		srcTokenID := apex.GetTokenIDForChain(srcChain, true)
		if srcTokenID == 0 {
			return ""
		}

		return apex.GetTokenNameForChains(dstChain, srcChain, srcTokenID)
	}

	return cardanowallet.AdaTokenName
}
