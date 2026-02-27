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
	cardanowallet "github.com/Ethernal-Tech/cardano-infrastructure/wallet"
	"github.com/stretchr/testify/require"
)

func ExecuteSingleBridging(
	t *testing.T, ctx context.Context, apex IApexSystem, senderUser,
	receiverUser *cardanofw.TestApexUser, srcChain, dstChain string, sendAmount *big.Int,
	srcTokenID uint16, validateTreasury bool, options ...ExecuteBridgingOption,
) {
	t.Helper()

	config := newExecuteBridgingConfig(options...)

	tokensInfo, err := apex.GetBridgingTokensInfo(srcChain, dstChain, srcTokenID)
	require.NoError(t, err)

	fmt.Printf("Tokens Info: %+v\n", tokensInfo)

	senderBalance, err := apex.GetBalanceWithTokenName(ctx, senderUser, srcChain, tokensInfo.SrcTokenName)
	require.NoError(t, err)
	fmt.Printf("Sender balance: %+v\n", senderBalance)

	balance, err := apex.GetBalanceWithTokenName(ctx, receiverUser, dstChain, tokensInfo.DstTokenName)
	fmt.Printf("Receiver balance: %+v\n", balance)
	require.NoError(t, err)

	prevAmount := cardanofw.SetOrDefault(balance[tokensInfo.DstTokenName], big.NewInt(0))

	initialTreasuryBalance, err := apex.GetTreasuryAddressBalance(ctx, t, srcChain)
	require.NoError(t, err)

	shouldCheckTreasuryBalance := initialTreasuryBalance != nil && validateTreasury

	txHash, err := apex.SubmitBridgingRequest(
		cardanofw.SubmitBridgingRequestData{
			Context:          ctx,
			SourceChain:      srcChain,
			DestinationChain: dstChain,
			Sender:           senderUser,
			WeiAmount:        sendAmount,
			SrcTokenID:       srcTokenID,
			Receivers:        []*cardanofw.TestApexUser{receiverUser},
			TokensInfo:       tokensInfo,
		},
	)
	require.NoError(t, err)

	fmt.Printf("Tx sent. hash: %s\n", txHash)

	expectedAmount := new(big.Int).Add(prevAmount, new(big.Int).Set(sendAmount))

	fmt.Printf("Expected amount: %+v\n", expectedAmount)

	err = apex.WaitForExactAmount(ctx, receiverUser, dstChain, expectedAmount,
		config.timeoutConfig.bridgingNumRetries, config.timeoutConfig.bridgingRetryWaitTime, tokensInfo.DstTokenName)
	require.NoError(t, err)

	if shouldCheckTreasuryBalance {
		err = apex.ValidateTreasuryAddressBalance(ctx, t, srcChain, initialTreasuryBalance, 1)
		require.NoError(t, err)
		fmt.Printf("Treasury address balance validated\n")
	}
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
	receiverUser *cardanofw.TestApexUser, srcChain, dstChain string,
	sendAmount *big.Int, srcTokenID uint16,
	options ...ExecuteBridgingOption,
) {
	t.Helper()

	config := newExecuteBridgingConfig(options...)

	for i := 0; i < txCountPerSender; i++ {
		tokensInfo, err := apex.GetBridgingTokensInfo(srcChain, dstChain, srcTokenID)
		require.NoError(t, err)

		balance, err := apex.GetBalanceWithTokenName(ctx, receiverUser, dstChain, tokensInfo.DstTokenName)
		require.NoError(t, err)

		prevAmount := cardanofw.SetOrDefault(balance[tokensInfo.DstTokenName], big.NewInt(0))

		txHash, err := apex.SubmitBridgingRequest(
			cardanofw.SubmitBridgingRequestData{
				Context:          ctx,
				SourceChain:      srcChain,
				DestinationChain: dstChain,
				Sender:           receiverUser,
				WeiAmount:        sendAmount,
				SrcTokenID:       srcTokenID,
				Receivers:        []*cardanofw.TestApexUser{receiverUser},
				TokensInfo:       tokensInfo,
			})
		require.NoError(t, err)

		fmt.Printf("Tx sent. hash: %s\n", txHash)

		expectedAmount := new(big.Int).Add(prevAmount, sendAmount)

		err = apex.WaitForExactAmount(ctx, receiverUser, dstChain, expectedAmount,
			config.timeoutConfig.bridgingNumRetries, config.timeoutConfig.bridgingRetryWaitTime, tokensInfo.DstTokenName)

		require.NoError(t, err)
	}
}

func ExecuteBridgingWaitAfterSubmits(
	t *testing.T, ctx context.Context, apex IApexSystem, txCountPerSender int,
	receiverUser *cardanofw.TestApexUser, srcChain, dstChain string,
	sendAmount *big.Int, srcTokenID uint16,
	options ...ExecuteBridgingOption,
) {
	t.Helper()

	config := newExecuteBridgingConfig(options...)
	tokensInfo, err := apex.GetBridgingTokensInfo(srcChain, dstChain, srcTokenID)
	require.NoError(t, err)

	balance, err := apex.GetBalanceWithTokenName(ctx, receiverUser, dstChain, tokensInfo.DstTokenName)
	require.NoError(t, err)

	initialTreasuryBalance, err := apex.GetTreasuryAddressBalance(ctx, t, srcChain)
	require.NoError(t, err)

	prevAmount := cardanofw.SetOrDefault(balance[tokensInfo.DstTokenName], big.NewInt(0))

	expectedAmount := prevAmount

	for i := 0; i < txCountPerSender; i++ {
		txHash, err := apex.SubmitBridgingRequest(
			cardanofw.SubmitBridgingRequestData{
				Context:          ctx,
				SourceChain:      srcChain,
				DestinationChain: dstChain,
				Sender:           receiverUser,
				WeiAmount:        sendAmount,
				SrcTokenID:       srcTokenID,
				Receivers:        []*cardanofw.TestApexUser{receiverUser},
				TokensInfo:       tokensInfo,
			})
		require.NoError(t, err)

		fmt.Printf("Tx[%d] sent. hash: %s\n", i, txHash)

		expectedAmount = expectedAmount.Add(expectedAmount, sendAmount)
	}

	err = apex.WaitForExactAmount(ctx, receiverUser, dstChain, expectedAmount,
		config.timeoutConfig.bridgingNumRetries, config.timeoutConfig.bridgingRetryWaitTime, tokensInfo.DstTokenName)

	if initialTreasuryBalance != nil {
		err = apex.ValidateTreasuryAddressBalance(ctx, t, srcChain, initialTreasuryBalance, uint64(txCountPerSender))
		require.NoError(t, err)
		fmt.Printf("Treasury address balance validated\n")
	}

	require.NoError(t, err)
}

func ExecuteBridgingWithRefund(
	t *testing.T, ctx context.Context, apex IApexSystem, senderUser, receiverUser *cardanofw.TestApexUser,
	srcChain, dstChain string, sendAmount *big.Int, srcTokenID uint16,
	refundTrigger func(t *testing.T, ctx context.Context,
		apex *cardanofw.ApexSystem, srcChain, dstChain string, user *cardanofw.TestApexUser,
		tokenID uint16),
	options ...ExecuteBridgingOption,
) {
	t.Helper()

	config := newExecuteBridgingConfig(options...)

	tokensInfo, err := apex.GetBridgingTokensInfo(srcChain, dstChain, srcTokenID)
	require.NoError(t, err)

	fmt.Printf("Tokens Info: %+v\n", tokensInfo)

	balance, err := apex.GetBalanceWithTokenName(ctx, receiverUser, dstChain, tokensInfo.DstTokenName)
	fmt.Printf("Receiver balance: %+v\n", balance)
	require.NoError(t, err)

	prevAmount := cardanofw.SetOrDefault(balance[tokensInfo.DstTokenName], big.NewInt(0))

	apexSystem, ok := apex.(*cardanofw.ApexSystem)
	require.True(t, ok, "apex should be of type *cardanofw.ApexSystem")

	refundTrigger(t, ctx, apexSystem, dstChain, srcChain, senderUser, tokensInfo.DstTokenID)

	txHash, err := apex.SubmitBridgingRequest(
		cardanofw.SubmitBridgingRequestData{
			Context:          ctx,
			SourceChain:      srcChain,
			DestinationChain: dstChain,
			Sender:           senderUser,
			WeiAmount:        sendAmount,
			SrcTokenID:       srcTokenID,
			Receivers:        []*cardanofw.TestApexUser{receiverUser},
			TokensInfo:       tokensInfo,
		},
	)
	require.NoError(t, err)

	fmt.Printf("Tx sent. hash: %s\n", txHash)

	// Destination currency (e.g. ADA) may lose fees on refund,
	// so we validate amount in a range instead of exact match.
	if tokensInfo.DstTokenName == cardanowallet.AdaTokenName {
		// decrease upper boundary by 1 to ensure refund has happened, not only bridging
		upperBoundary := new(big.Int).Sub(
			new(big.Int).Add(prevAmount, sendAmount),
			big.NewInt(1),
		)

		fmt.Printf("Tx sent. hash: %s, lowerBoundaryDfm: %d, higherBoundaryDfm: %+v\n", txHash, prevAmount,
			upperBoundary)

		err = apex.WaitForAmountInRange(ctx, receiverUser, dstChain, prevAmount,
			upperBoundary, config.timeoutConfig.bridgingNumRetries, config.timeoutConfig.bridgingRetryWaitTime,
			tokensInfo.DstTokenName)
		require.NoError(t, err)

		return
	}

	expectedAmount := new(big.Int).Add(prevAmount, sendAmount)

	fmt.Printf("Expected amount: %+v\n", expectedAmount)

	err = apex.WaitForExactAmount(ctx, receiverUser, dstChain, expectedAmount,
		config.timeoutConfig.bridgingNumRetries, config.timeoutConfig.bridgingRetryWaitTime, tokensInfo.DstTokenName)

	require.NoError(t, err)
}

type ExecuteBridgingConfig struct {
	SrcChain      string
	DstChain      string
	SrcTokenID    uint16
	SendAmountWei *big.Int
}

func ExecuteBridgingWaitAfterSubmitsExtended(
	t *testing.T, ctx context.Context, apex IApexSystem, txCountPerSender int,
	receiverUser *cardanofw.TestApexUser, directions []ExecuteBridgingConfig,
	options ...ExecuteBridgingOption,
) {
	t.Helper()

	config := newExecuteBridgingConfig(options...)

	type expectedAmountInfo struct {
		tokenName      string
		srcChain       string
		dstChain       string
		expectedAmount *big.Int
	}

	expectedAmounts := make([]expectedAmountInfo, len(directions))

	// We have to set initial balances here since some bridging may finish before the other ones
	// with same tokens and scramble the expected amounts
	// (e.g. Nexus -> Vector and Cardano -> Vector in parallel - sequential xADA)
	initialBalances := make(map[string]*big.Int)

	for _, direction := range directions {
		tokensInfo, err := apex.GetBridgingTokensInfo(
			direction.SrcChain, direction.DstChain, direction.SrcTokenID)
		require.NoError(t, err)

		balance, err := apex.GetBalanceWithTokenName(ctx, receiverUser, direction.DstChain, tokensInfo.DstTokenName)
		require.NoError(t, err)

		initialBalances[tokensInfo.DstTokenName] = cardanofw.SetOrDefault(
			balance[tokensInfo.DstTokenName],
			big.NewInt(0),
		)
	}

	initialTreasuryBalances := make(map[string]*big.Int)
	numberOfBridgingRequestsPerChain := make(map[string]uint64)

	for _, direction := range directions {
		initialTreasuryBalance, err := apex.GetTreasuryAddressBalance(ctx, t, direction.SrcChain)
		require.NoError(t, err)

		initialTreasuryBalances[direction.SrcChain] = initialTreasuryBalance

		numberOfBridgingRequestsPerChain[direction.SrcChain] += uint64(txCountPerSender)
	}

	// Send all bridging requests in parallel per direction
	var wgSend sync.WaitGroup

	for i, direction := range directions {
		wgSend.Add(1)

		go func(idx int, dir ExecuteBridgingConfig) {
			defer wgSend.Done()

			tokensInfo, err := apex.GetBridgingTokensInfo(dir.SrcChain, dir.DstChain, dir.SrcTokenID)
			require.NoError(t, err)

			prevAmount := initialBalances[tokensInfo.DstTokenName]
			expectedAmount := new(big.Int).Set(prevAmount)

			for j := 0; j < txCountPerSender; j++ {
				txHash, err := apex.SubmitBridgingRequest(
					cardanofw.SubmitBridgingRequestData{
						Context:          ctx,
						SourceChain:      dir.SrcChain,
						DestinationChain: dir.DstChain,
						Sender:           receiverUser,
						WeiAmount:        dir.SendAmountWei,
						SrcTokenID:       dir.SrcTokenID,
						Receivers:        []*cardanofw.TestApexUser{receiverUser},
						TokensInfo:       tokensInfo,
					})
				require.NoError(t, err)

				fmt.Printf("Direction %d Tx[%d] sent. hash: %s\n", idx, j, txHash)

				expectedAmount.Add(expectedAmount, dir.SendAmountWei)
			}

			expectedAmounts[idx] = expectedAmountInfo{
				tokenName:      tokensInfo.DstTokenName,
				srcChain:       dir.SrcChain,
				dstChain:       dir.DstChain,
				expectedAmount: expectedAmount,
			}
		}(i, direction)
	}

	wgSend.Wait()

	// Sum up all the expected amounts for the same token name
	amountsToWait := make(map[string]expectedAmountInfo)
	for _, exp := range expectedAmounts {
		if data, exists := amountsToWait[exp.tokenName]; !exists {
			amountsToWait[exp.tokenName] = expectedAmountInfo{
				tokenName:      exp.tokenName,
				srcChain:       exp.srcChain,
				dstChain:       exp.dstChain,
				expectedAmount: exp.expectedAmount,
			}
		} else {
			data.expectedAmount.Add(data.expectedAmount, exp.expectedAmount)
			data.expectedAmount.Sub(data.expectedAmount, initialBalances[exp.tokenName])
			amountsToWait[exp.tokenName] = data
		}
	}

	// Wait for all expected amounts in parallel
	var wgWait sync.WaitGroup

	for _, exp := range amountsToWait {
		wgWait.Add(1)

		go func(e expectedAmountInfo) {
			defer wgWait.Done()

			err := apex.WaitForExactAmount(
				ctx,
				receiverUser,
				e.dstChain,
				e.expectedAmount,
				config.timeoutConfig.bridgingNumRetries,
				config.timeoutConfig.bridgingRetryWaitTime,
				e.tokenName,
			)
			require.NoError(t, err)
		}(exp)
	}

	wgWait.Wait()

	for _, direction := range directions {
		if initialTreasuryBalances[direction.SrcChain] != nil {
			err := apex.ValidateTreasuryAddressBalance(
				ctx, t, direction.SrcChain,
				initialTreasuryBalances[direction.SrcChain], numberOfBridgingRequestsPerChain[direction.SrcChain])
			require.NoError(t, err)
			fmt.Printf("Treasury address balance validated for %s\n", direction.SrcChain)
		}
	}
}

func ExecuteBridging(
	t *testing.T, ctx context.Context, apex IApexSystem, txCountPerSender int,
	senderUsers []*cardanofw.TestApexUser, receiverUsers []*cardanofw.TestApexUser,
	chains []string, chainsDst map[string][]string, srcTokenIDs map[SrcDstChainPair]uint16,
	sendAmount *big.Int, options ...ExecuteBridgingOption,
) {
	t.Helper()

	var (
		config     = newExecuteBridgingConfig(options...)
		chainPairs = getAllChainPairs(chains, chainsDst)
		// per each receiver -> per each chain -> per each token
		initialAmountsPerRecv = make([]map[string]map[string]*big.Int, len(receiverUsers))
	)

	// calculate receivers initial balances
	for i, receiverUser := range receiverUsers {
		initialAmountsPerRecv[i] = map[string]map[string]*big.Int{}

		for _, pair := range chainPairs {
			tokensInfo, err := apex.GetBridgingTokensInfo(pair.srcChain, pair.dstChain, srcTokenIDs[pair])
			require.NoError(t, err)

			balance, err := apex.GetBalanceWithTokenName(ctx, receiverUser, pair.dstChain, tokensInfo.DstTokenName)
			require.NoError(t, err)

			if _, exists := initialAmountsPerRecv[i][pair.dstChain]; !exists {
				initialAmountsPerRecv[i][pair.dstChain] = map[string]*big.Int{}
			}

			initialAmountsPerRecv[i][pair.dstChain][tokensInfo.DstTokenName] = cardanofw.SetOrDefault(
				balance[tokensInfo.DstTokenName],
				big.NewInt(0),
			)
		}
	}

	initialTreasuryBalances := make(map[string]*big.Int)
	numberOfBridgingRequestsPerChain := make(map[string]uint64)

	for _, pair := range chainPairs {
		initialTreasuryBalance, err := apex.GetTreasuryAddressBalance(ctx, t, pair.srcChain)
		require.NoError(t, err)

		initialTreasuryBalances[pair.srcChain] = initialTreasuryBalance

		numberOfBridgingRequestsPerChain[pair.srcChain] += uint64(txCountPerSender) * uint64(len(senderUsers))
	}

	// send transactions
	sendTxDatas := config.sendTxStrategy(
		ctx, apex, chainsDst, senderUsers, receiverUsers,
		sendAmount, txCountPerSender, srcTokenIDs)

	for _, d := range sendTxDatas {
		require.NoError(t, d.err)
	}

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
		tokensInfo := txData.TokensInfo

		if _, exists := originalDesiredAmounts[txData.DstChainID]; !exists {
			originalDesiredAmounts[txData.DstChainID] = make(map[string]*big.Int)
		}

		if _, exists := originalDesiredAmounts[txData.DstChainID][tokensInfo.DstTokenName]; !exists {
			originalDesiredAmounts[txData.DstChainID][tokensInfo.DstTokenName] = big.NewInt(0)
		}

		expectedAmount := new(big.Int).Set(txData.SendAmount)

		originalDesiredAmounts[txData.DstChainID][tokensInfo.DstTokenName].Add(
			originalDesiredAmounts[txData.DstChainID][tokensInfo.DstTokenName], expectedAmount)

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

				tokensInfo, err := apex.GetBridgingTokensInfo(
					chainPair.srcChain, chainPair.dstChain, srcTokenIDs[chainPair])
				require.NoError(t, err)

				// Retrieve all failed transactions on the source chain, if any
				for _, txHash := range apex.GetChainMust(t, chainPair.srcChain).GetIndexer().GetFailedTxs() {
					// check whether failed transaction is one of these sent from the users (ignore funding transaction rollbacks)
					if _, exists := txHashTxDataMap[txHash]; exists {
						sum.Add(sum, txHashTxDataMap[txHash].SendAmount)
					}
				}

				lock.Lock()
				oldValue := new(big.Int).Set(desiredAmounts[chainPair.dstChain][tokensInfo.DstTokenName])

				// Subtract failed transaction amounts from the original desired amounts on the destination chain
				desiredAmounts[chainPair.dstChain][tokensInfo.DstTokenName].Sub(
					originalDesiredAmounts[chainPair.dstChain][tokensInfo.DstTokenName], sum)

				newValue := desiredAmounts[chainPair.dstChain][tokensInfo.DstTokenName]
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
		tokensInfo, err := apex.GetBridgingTokensInfo(pair.srcChain, pair.dstChain, srcTokenIDs[pair])
		require.NoError(t, err)

		key := fmt.Sprintf("%s-%s", pair.dstChain, tokensInfo.DstTokenName)
		// It doesn't matter if two source chains have the same token name (e.g., "lovelace")
		// for the same destination chain — just pick any one.
		srcChainMap[key] = pair.srcChain
	}

	// wait for amounts
	for i, userRecv := range receiverUsers {
		for j, chainPair := range chainPairs {
			tokensInfo, err := apex.GetBridgingTokensInfo(
				chainPair.srcChain, chainPair.dstChain, srcTokenIDs[chainPair])
			require.NoError(t, err)

			wgResults.Add(1)

			go func(idx, idxChain int, receiver *cardanofw.TestApexUser,
				dstChain string, srcChain string, initialAmount *big.Int) {
				defer wgResults.Done()

				bigIntCache := new(big.Int)

				getDesiredAmount := func() *big.Int {
					lock.RLock()
					defer lock.RUnlock()

					receivedAmount := bigIntCache.Add(
						bigIntCache.Set(initialAmount), desiredAmounts[dstChain][tokensInfo.DstTokenName])

					return receivedAmount
				}

				receivedAmount, err := apex.WaitForAmount(
					ctx, receiver, dstChain, func(currentAmount *big.Int) bool {
						return currentAmount.Cmp(getDesiredAmount()) == 0
					},
					len(receiverUsers)*config.timeoutConfig.bridgingNumRetries,
					config.timeoutConfig.bridgingRetryWaitTime,
					tokensInfo.DstTokenName,
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
						ctx, receiver, dstChain, receivedAmount,
						config.timeoutConfig.unexpectedBridgesNumRetries, config.timeoutConfig.unexpectedBridgesRetryWaitTime,
						tokensInfo.DstTokenName)
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
				srcChainMap[fmt.Sprintf("%s-%s", chainPair.dstChain, tokensInfo.DstTokenName)],
				initialAmountsPerRecv[i][chainPair.dstChain][tokensInfo.DstTokenName])
		}
	}

	wgResults.Wait()

	close(closeCh)

	require.NoError(t, errors.Join(errs...))

	for _, pair := range chainPairs {
		if initialTreasuryBalances[pair.srcChain] != nil {
			err := apex.ValidateTreasuryAddressBalance(
				ctx, t, pair.srcChain, initialTreasuryBalances[pair.srcChain], numberOfBridgingRequestsPerChain[pair.srcChain])
			require.NoError(t, err)
			fmt.Printf("Treasury address balance validated for %s\n", pair.srcChain)
		}
	}
}

// This allows defining multiple directions between the same src / dst pair
// (e.g. two different vector -> nexus bridgings with different token IDs).
func ExecuteBridgingExtended(
	t *testing.T, ctx context.Context, apex IApexSystem, txCountPerSender int,
	senderUsers []*cardanofw.TestApexUser, receiverUsers []*cardanofw.TestApexUser,
	directions []BridgingDirectionConfig, sendAmount *big.Int, options ...ExecuteBridgingOption,
) {
	t.Helper()

	if len(directions) == 0 {
		return
	}

	var (
		config = newExecuteBridgingConfig(options...)
		// per each receiver -> per each chain -> per each token
		initialAmountsPerRecv = make([]map[string]map[string]*big.Int, len(receiverUsers))
	)

	// Precompute tokens info per direction and build unique (src, dst, token) combinations.
	type directionRuntime struct {
		BridgingDirectionConfig
		TokensInfo *cardanofw.BridgingTokensInfo
	}

	type comboKey struct {
		srcChain     string
		dstChain     string
		dstTokenName string
	}

	dirsRuntime := make([]directionRuntime, len(directions))
	combosMap := make(map[comboKey]struct{})
	initialTreasuryBalances := make(map[string]*big.Int)
	numberOfBridgingRequestsPerChain := make(map[string]uint64)

	for i, d := range directions {
		tokensInfo, err := apex.GetBridgingTokensInfo(d.SrcChain, d.DstChain, d.SrcTokenID)
		require.NoError(t, err)

		dirsRuntime[i] = directionRuntime{
			BridgingDirectionConfig: d,
			TokensInfo:              tokensInfo,
		}

		combosMap[comboKey{
			srcChain:     d.SrcChain,
			dstChain:     d.DstChain,
			dstTokenName: tokensInfo.DstTokenName,
		}] = struct{}{}

		initialTreasuryBalance, err := apex.GetTreasuryAddressBalance(ctx, t, d.SrcChain)
		require.NoError(t, err)

		initialTreasuryBalances[d.SrcChain] = initialTreasuryBalance

		numberOfBridgingRequestsPerChain[d.SrcChain] += uint64(txCountPerSender) * uint64(len(senderUsers))
	}

	combos := make([]comboKey, 0, len(combosMap))
	for k := range combosMap {
		combos = append(combos, k)
	}

	// calculate receivers initial balances
	for i, receiverUser := range receiverUsers {
		initialAmountsPerRecv[i] = map[string]map[string]*big.Int{}

		for _, dr := range dirsRuntime {
			dstChain := dr.DstChain

			balance, err := apex.GetBalanceWithTokenName(ctx, receiverUser, dstChain, dr.TokensInfo.DstTokenName)
			require.NoError(t, err)

			if _, exists := initialAmountsPerRecv[i][dstChain]; !exists {
				initialAmountsPerRecv[i][dstChain] = map[string]*big.Int{}
			}

			initialAmountsPerRecv[i][dstChain][dr.TokensInfo.DstTokenName] = cardanofw.SetOrDefault(
				balance[dr.TokensInfo.DstTokenName],
				big.NewInt(0),
			)
		}
	}

	// send transactions
	type extendedTxData struct {
		SrcChainID   string
		DstChainID   string
		TxHash       string
		SendAmount   *big.Int
		DstTokenName string
		err          error
	}

	var (
		wgSend         sync.WaitGroup
		muSend         sync.Mutex
		sendTxDatas    []*extendedTxData
		totalCombos    = len(combos)
		totalReceivers = len(receiverUsers)
	)

	for i, sender := range senderUsers {
		for _, dr := range dirsRuntime {
			wgSend.Add(1)

			// capture loop variables
			drCopy := dr

			go func(idx int, senderUser *cardanofw.TestApexUser, dr directionRuntime) {
				defer wgSend.Done()

				for j := 0; j < txCountPerSender; j++ {
					txHash, err := apex.SubmitBridgingRequest(
						cardanofw.SubmitBridgingRequestData{
							Context:          ctx,
							SourceChain:      dr.SrcChain,
							DestinationChain: dr.DstChain,
							Sender:           senderUser,
							WeiAmount:        sendAmount,
							SrcTokenID:       dr.SrcTokenID,
							Receivers:        receiverUsers,
							TokensInfo:       dr.TokensInfo,
						},
					)

					if err != nil {
						muSend.Lock()

						sendTxDatas = append(sendTxDatas, &extendedTxData{
							err: err,
						})

						muSend.Unlock()

						continue
					}

					fmt.Printf("Sender: %d. run: %d. %s->%s tx sent: %s (token=%s)\n",
						idx+1, j+1, dr.SrcChain, dr.DstChain, txHash, dr.TokensInfo.DstTokenName)

					muSend.Lock()
					sendTxDatas = append(sendTxDatas, &extendedTxData{
						SrcChainID:   dr.SrcChain,
						DstChainID:   dr.DstChain,
						TxHash:       txHash,
						SendAmount:   sendAmount,
						DstTokenName: dr.TokensInfo.DstTokenName,
					})
					muSend.Unlock()
				}
			}(i, sender, drCopy)
		}
	}

	wgSend.Wait()

	for _, d := range sendTxDatas {
		require.NoError(t, d.err)
	}

	var (
		wgResults              sync.WaitGroup
		lock                   sync.RWMutex
		originalDesiredAmounts = make(map[string]map[string]*big.Int, len(combos))
		desiredAmounts         = make(map[string]map[string]*big.Int, len(combos))
		closeCh                = make(chan struct{})
		txHashTxDataMap        = make(map[string]*extendedTxData)
		errs                   = make([]error, totalReceivers*totalCombos)
		srcChainMap            = map[string]string{}
	)

	// calculate desired amounts per chain/token
	for _, txData := range sendTxDatas {
		if _, exists := originalDesiredAmounts[txData.DstChainID]; !exists {
			originalDesiredAmounts[txData.DstChainID] = make(map[string]*big.Int)
		}

		if _, exists := originalDesiredAmounts[txData.DstChainID][txData.DstTokenName]; !exists {
			originalDesiredAmounts[txData.DstChainID][txData.DstTokenName] = big.NewInt(0)
		}

		expectedAmount := new(big.Int).Set(txData.SendAmount)

		originalDesiredAmounts[txData.DstChainID][txData.DstTokenName].Add(
			originalDesiredAmounts[txData.DstChainID][txData.DstTokenName], expectedAmount)

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

			for _, combo := range combos {
				sum := new(big.Int)

				// Retrieve all failed transactions on the source chain, if any
				for _, txHash := range apex.GetChainMust(t, combo.srcChain).GetIndexer().GetFailedTxs() {
					// check whether failed transaction is one of these sent from the users (ignore funding transaction rollbacks)
					if txData, exists := txHashTxDataMap[txHash]; exists &&
						txData.DstChainID == combo.dstChain &&
						txData.DstTokenName == combo.dstTokenName {
						sum.Add(sum, txData.SendAmount)
					}
				}

				lock.Lock()
				oldValue := new(big.Int).Set(desiredAmounts[combo.dstChain][combo.dstTokenName])

				// Subtract failed transaction amounts from the original desired amounts on the destination chain
				desiredAmounts[combo.dstChain][combo.dstTokenName].Sub(
					originalDesiredAmounts[combo.dstChain][combo.dstTokenName],
					sum,
				)

				newValue := desiredAmounts[combo.dstChain][combo.dstTokenName]
				isDifferent := oldValue.Cmp(newValue) != 0

				lock.Unlock()

				if isDifferent {
					fmt.Printf("Desired amount for %s (%s) is %d (was %d)\n",
						combo.dstChain, combo.dstTokenName, newValue, oldValue)
				}
			}
		}
	}()

	// prepare the map (dstChain + tokenName -> sourceChain)
	for _, combo := range combos {
		key := fmt.Sprintf("%s-%s", combo.dstChain, combo.dstTokenName)
		// It doesn't matter if two source chains have the same token name (e.g., "lovelace")
		// for the same destination chain — just pick any one.
		srcChainMap[key] = combo.srcChain
	}

	// wait for amounts
	for i, userRecv := range receiverUsers {
		for j, combo := range combos {
			dstChain := combo.dstChain
			tokenName := combo.dstTokenName

			initialAmount, ok := initialAmountsPerRecv[i][dstChain][tokenName]
			if !ok {
				// If there was no initial amount recorded for this token on this chain for this receiver,
				// treat it as zero.
				initialAmount = big.NewInt(0)
			}

			wgResults.Add(1)

			go func(idx, idxCombo int, receiver *cardanofw.TestApexUser,
				dstChain string, srcChain string, tokenName string, initialAmountDfm *big.Int) {
				defer wgResults.Done()

				bigIntCache := new(big.Int)

				getDesiredAmount := func() *big.Int {
					lock.RLock()
					defer lock.RUnlock()

					receivedAmount := bigIntCache.Add(bigIntCache.Set(initialAmountDfm), desiredAmounts[dstChain][tokenName])

					return receivedAmount
				}

				receivedAmount, err := apex.WaitForAmount(
					ctx, receiver, dstChain, func(currentAmount *big.Int) bool {
						return currentAmount.Cmp(getDesiredAmount()) == 0
					},
					len(receiverUsers)*config.timeoutConfig.bridgingNumRetries,
					config.timeoutConfig.bridgingRetryWaitTime,
					tokenName,
				)
				if err != nil {
					errs[idx*totalCombos+idxCombo] = fmt.Errorf("receiver %d on %s (%s vs %s): %w",
						idx, dstChain, receivedAmount, getDesiredAmount(), err)

					return
				}

				fmt.Printf("TXs on %s for user %d expected amount received %s\n", dstChain, idx, receivedAmount)

				if config.waitForUnexpectedBridges {
					// nothing else should be bridged for 2 minutes
					err := apex.WaitForGreaterAmount(
						ctx, receiver, dstChain, receivedAmount,
						config.timeoutConfig.unexpectedBridgesNumRetries, config.timeoutConfig.unexpectedBridgesRetryWaitTime,
						tokenName,
					)
					if !errors.Is(err, infracommon.ErrRetryTimeout) {
						lock.Lock()
						errs = append(errs, fmt.Errorf(
							"receiver %d on %s->%s received more than expected tokens: %w", idx, srcChain, dstChain, err))
						lock.Unlock()

						return
					}

					fmt.Printf("TXs on %s for user %d finished with success\n", dstChain, idx)
				}
			}(i, j, userRecv, dstChain,
				srcChainMap[fmt.Sprintf("%s-%s", dstChain, tokenName)],
				tokenName,
				initialAmount)
		}
	}

	wgResults.Wait()

	close(closeCh)

	require.NoError(t, errors.Join(errs...))

	for _, direction := range directions {
		if initialTreasuryBalances[direction.SrcChain] != nil {
			err := apex.ValidateTreasuryAddressBalance(
				ctx, t, direction.SrcChain, initialTreasuryBalances[direction.SrcChain],
				numberOfBridgingRequestsPerChain[direction.SrcChain])
			require.NoError(t, err)
			fmt.Printf("Treasury address balance validated for %s\n", direction.SrcChain)
		}
	}
}
