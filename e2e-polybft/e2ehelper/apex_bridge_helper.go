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
	t *testing.T, ctx context.Context, apex IApexSystem, senderUser, receiverUser *cardanofw.TestApexUser,
	srcChain, dstChain string, sendAmount *big.Int, bridgingType cardanofw.BridgingType, options ...ExecuteBridgingOption,
) {
	t.Helper()

	config := newExecuteBridgingConfig(options...)

	tokensInfo := apex.GetBridgingTokensInfo(srcChain, dstChain, bridgingType, config.coloredCoins...)
	require.NotNil(t, tokensInfo)

	fmt.Printf("Tokens Info: %+v\n", tokensInfo)

	senderBalance, err := apex.GetBalanceWithTokenName(ctx, senderUser, srcChain, tokensInfo.SrcTokenName)
	require.NoError(t, err)
	fmt.Printf("Sender balance: %+v\n", senderBalance)

	balance, err := apex.GetBalanceWithTokenName(ctx, receiverUser, dstChain, tokensInfo.DstTokenName)
	fmt.Printf("Receiver balance: %+v\n", balance)
	require.NoError(t, err)

	prevAmount := cardanofw.SetOrDefault(balance[tokensInfo.DstTokenName], big.NewInt(0))

	txHash, err := apex.SubmitBridgingRequest(
		cardanofw.SubmitBridgingRequestData{
			Context:          ctx,
			SourceChain:      srcChain,
			DestinationChain: dstChain,
			Sender:           senderUser,
			DFMAmount:        sendAmount,
			BridgingType:     bridgingType,
			Receivers:        []*cardanofw.TestApexUser{receiverUser},
			TokensInfo:       tokensInfo,
		},
	)
	require.NoError(t, err)

	fmt.Printf("Tx sent. hash: %s\n", txHash)

	if dstChain == cardanofw.ChainIDNexus {
		sendAmount = cardanofw.DfmToWei(sendAmount)
	}

	expectedAmount := new(big.Int).Add(prevAmount, sendAmount)

	fmt.Printf("Expected amount: %+v\n", expectedAmount)

	err = apex.WaitForExactAmount(ctx, receiverUser, dstChain, srcChain, expectedAmount,
		config.timeoutConfig.bridgingNumRetries, config.timeoutConfig.bridgingRetryWaitTime, tokensInfo.DstTokenName)

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
	receiverUser *cardanofw.TestApexUser, srcChain, dstChain string, sendAmount *big.Int, bridgingType cardanofw.BridgingType,
	options ...ExecuteBridgingOption,
) {
	t.Helper()

	config := newExecuteBridgingConfig(options...)

	for i := 0; i < txCountPerSender; i++ {
		tokensInfo := apex.GetBridgingTokensInfo(srcChain, dstChain, bridgingType, config.coloredCoins...)
		require.NotNil(t, tokensInfo)

		balance, err := apex.GetBalanceWithTokenName(ctx, receiverUser, dstChain, tokensInfo.DstTokenName)
		require.NoError(t, err)

		prevAmount := cardanofw.SetOrDefault(balance[tokensInfo.DstTokenName], big.NewInt(0))

		txHash, err := apex.SubmitBridgingRequest(
			cardanofw.SubmitBridgingRequestData{
				Context:          ctx,
				SourceChain:      srcChain,
				DestinationChain: dstChain,
				Sender:           receiverUser,
				DFMAmount:        sendAmount,
				BridgingType:     bridgingType,
				Receivers:        []*cardanofw.TestApexUser{receiverUser},
				TokensInfo:       tokensInfo,
			})
		require.NoError(t, err)

		fmt.Printf("Tx sent. hash: %s\n", txHash)

		expectedAmount := new(big.Int).Add(prevAmount, sendAmount)

		err = apex.WaitForExactAmount(ctx, receiverUser, dstChain, srcChain, expectedAmount,
			config.timeoutConfig.bridgingNumRetries, config.timeoutConfig.bridgingRetryWaitTime, tokensInfo.DstTokenName)

		require.NoError(t, err)
	}
}

func ExecuteBridgingWaitAfterSubmits(
	t *testing.T, ctx context.Context, apex IApexSystem, txCountPerSender int,
	receiverUser *cardanofw.TestApexUser, srcChain, dstChain string, sendAmount *big.Int, bridgingType cardanofw.BridgingType,
	options ...ExecuteBridgingOption,
) {
	t.Helper()

	config := newExecuteBridgingConfig(options...)
	tokensInfo := apex.GetBridgingTokensInfo(srcChain, dstChain, bridgingType, config.coloredCoins...)
	require.NotNil(t, tokensInfo)

	balance, err := apex.GetBalanceWithTokenName(ctx, receiverUser, dstChain, tokensInfo.DstTokenName)
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
				DFMAmount:        sendAmount,
				BridgingType:     bridgingType,
				Receivers:        []*cardanofw.TestApexUser{receiverUser},
				TokensInfo:       tokensInfo,
			})
		require.NoError(t, err)

		fmt.Printf("Tx[%d] sent. hash: %s\n", i, txHash)

		expectedAmount = expectedAmount.Add(expectedAmount, sendAmount)
	}

	err = apex.WaitForExactAmount(ctx, receiverUser, dstChain, srcChain, expectedAmount,
		config.timeoutConfig.bridgingNumRetries, config.timeoutConfig.bridgingRetryWaitTime, tokensInfo.DstTokenName)

	require.NoError(t, err)
}

func ExecuteBridging(
	t *testing.T, ctx context.Context, apex IApexSystem, txCountPerSender int,
	senderUsers []*cardanofw.TestApexUser, receiverUsers []*cardanofw.TestApexUser,
	chains []string, chainsDst map[string][]string, bridgingTypes map[SrcDstChainPair]cardanofw.BridgingType,
	sendAmountDfm *big.Int, options ...ExecuteBridgingOption,
) {
	t.Helper()

	var (
		err        error
		config     = newExecuteBridgingConfig(options...)
		chainPairs = getAllChainPairs(chains, chainsDst)
		// per each receiver -> per each chain -> per each token
		initialAmountsPerRecv = make([]map[string]map[string]*big.Int, len(receiverUsers))
	)

	// calculate receivers initial balances
	for i, receiverUser := range receiverUsers {
		initialAmountsPerRecv[i] = map[string]map[string]*big.Int{}
		balancePerChain := map[string]map[string]*big.Int{}

		for _, pair := range chainPairs {
			tokensInfo := apex.GetBridgingTokensInfo(pair.srcChain, pair.dstChain, bridgingTypes[pair], config.coloredCoins...)
			require.NotNil(t, tokensInfo)

			balance, exists := balancePerChain[pair.dstChain]
			if !exists {
				balance, err = apex.GetBalanceWithTokenName(ctx, receiverUser, pair.dstChain, tokensInfo.DstTokenName)
				require.NoError(t, err)

				balancePerChain[pair.dstChain] = balance
				initialAmountsPerRecv[i][pair.dstChain] = map[string]*big.Int{}
			}

			initialAmountsPerRecv[i][pair.dstChain][tokensInfo.DstTokenName] = cardanofw.SetOrDefault(balance[tokensInfo.DstTokenName], big.NewInt(0))
		}
	}

	// send transactions
	sendTxDatas := config.sendTxStrategy(
		t, ctx, apex, chainsDst, senderUsers, receiverUsers, sendAmountDfm, txCountPerSender, bridgingTypes, config.coloredCoins...)

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
		tokensInfo := apex.GetBridgingTokensInfo(txData.SrcChainID, txData.DstChainID, txData.BridgingTxType, config.coloredCoins...)
		require.NotNil(t, tokensInfo)

		if _, exists := originalDesiredAmounts[txData.DstChainID]; !exists {
			originalDesiredAmounts[txData.DstChainID] = make(map[string]*big.Int)
		}

		if _, exists := originalDesiredAmounts[txData.DstChainID][tokensInfo.DstTokenName]; !exists {
			originalDesiredAmounts[txData.DstChainID][tokensInfo.DstTokenName] = big.NewInt(0)
		}

		expectedAmount := new(big.Int).Set(txData.SendAmountDfm)
		// Nexus uses Wei for sending amounts
		if txData.DstChainID == cardanofw.ChainIDNexus {
			expectedAmount = cardanofw.DfmToWei(expectedAmount)
		}

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

				//tokenName := getTokenNameForChains(
				//	apex, chainPair.dstChain, chainPair.srcChain, expectNativeTokens(bridgingTypes[chainPair]))
				tokensInfo := apex.GetBridgingTokensInfo(chainPair.srcChain, chainPair.dstChain, bridgingTypes[chainPair], config.coloredCoins...)
				require.NotNil(t, tokensInfo)

				// Retrieve all failed transactions on the source chain, if any
				for _, txHash := range apex.GetChainMust(t, chainPair.srcChain).GetIndexer().GetFailedTxs() {
					// check whether failed transaction is one of these sent from the users (ignore funding transaction rollbacks)
					if _, exists := txHashTxDataMap[txHash]; exists {
						sum.Add(sum, txHashTxDataMap[txHash].SendAmountDfm)
					}
				}

				lock.Lock()
				oldValue := new(big.Int).Set(desiredAmounts[chainPair.dstChain][tokensInfo.DstTokenName])

				// Subtract failed transaction amounts from the original desired amounts on the destination chain
				desiredAmounts[chainPair.dstChain][tokensInfo.DstTokenName].Sub(originalDesiredAmounts[chainPair.dstChain][tokensInfo.DstTokenName], sum)

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
		//tokenName := getTokenNameForChains(
		//	apex, pair.dstChain, pair.srcChain, expectNativeTokens(bridgingTypes[pair]))
		tokensInfo := apex.GetBridgingTokensInfo(pair.srcChain, pair.dstChain, bridgingTypes[pair], config.coloredCoins...)
		require.NotNil(t, tokensInfo)
		key := fmt.Sprintf("%s-%s", pair.dstChain, tokensInfo.DstTokenName)
		// It doesn't matter if two source chains have the same token name (e.g., "lovelace")
		// for the same destination chain — just pick any one.
		srcChainMap[key] = pair.srcChain
	}

	// wait for amounts
	for i, userRecv := range receiverUsers {
		for j, chainPair := range chainPairs {
			//tokenName := getTokenNameForChains(
			//	apex, chainPair.dstChain, chainPair.srcChain, expectNativeTokens(bridgingTypes[chainPair]))
			tokensInfo := apex.GetBridgingTokensInfo(chainPair.srcChain, chainPair.dstChain, bridgingTypes[chainPair], config.coloredCoins...)
			require.NotNil(t, tokensInfo)

			wgResults.Add(1)

			go func(idx, idxChain int, receiver *cardanofw.TestApexUser,
				dstChain string, srcChain string, initialAmountDfm *big.Int) {
				defer wgResults.Done()

				bigIntCache := new(big.Int)

				getDesiredAmount := func() *big.Int {
					lock.RLock()
					defer lock.RUnlock()

					receivedAmount := bigIntCache.Add(bigIntCache.Set(initialAmountDfm), desiredAmounts[dstChain][tokensInfo.DstTokenName])

					return receivedAmount
				}

				receivedAmount, err := apex.WaitForAmount(
					ctx, receiver, dstChain, srcChain, func(currentAmount *big.Int) bool {
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
						ctx, receiver, dstChain, srcChain, receivedAmount,
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
}

// This allows defining multiple directions between the same src / dst pair
// (e.g. two different vector -> nexus bridgings with different token IDs).
func ExecuteBridgingExtended(
	t *testing.T, ctx context.Context, apex IApexSystem, txCountPerSender int,
	senderUsers []*cardanofw.TestApexUser, receiverUsers []*cardanofw.TestApexUser,
	directions []BridgingDirectionConfig, sendAmountDfm *big.Int, options ...ExecuteBridgingOption,
) {
	t.Helper()

	if len(directions) == 0 {
		return
	}

	var (
		err    error
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

	for i, d := range directions {
		var colored []uint16

		// If TokenID is set, use it explicitly; otherwise fall back to global coloredCoins,
		// which keeps behaviour for non-colored bridging or when callers still rely on options.
		if d.TokenID != 0 {
			colored = []uint16{d.TokenID}
		} else {
			colored = config.coloredCoins
		}

		tokensInfo := apex.GetBridgingTokensInfo(d.SrcChain, d.DstChain, d.BridgingType, colored...)
		require.NotNil(t, tokensInfo)

		dirsRuntime[i] = directionRuntime{
			BridgingDirectionConfig: d,
			TokensInfo:              tokensInfo,
		}

		combosMap[comboKey{
			srcChain:     d.SrcChain,
			dstChain:     d.DstChain,
			dstTokenName: tokensInfo.DstTokenName,
		}] = struct{}{}
	}

	combos := make([]comboKey, 0, len(combosMap))
	for k := range combosMap {
		combos = append(combos, k)
	}

	// calculate receivers initial balances
	for i, receiverUser := range receiverUsers {
		initialAmountsPerRecv[i] = map[string]map[string]*big.Int{}
		balancePerChain := map[string]map[string]*big.Int{}

		for _, dr := range dirsRuntime {
			dstChain := dr.DstChain

			balance, exists := balancePerChain[dstChain]
			if !exists {
				balance, err = apex.GetBalanceWithTokenName(ctx, receiverUser, dstChain, dr.TokensInfo.DstTokenName)
				require.NoError(t, err)

				balancePerChain[dstChain] = balance
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
		SrcChainID     string
		DstChainID     string
		TxHash         string
		SendAmountDfm  *big.Int
		BridgingTxType cardanofw.BridgingType
		DstTokenName   string
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
							DFMAmount:        sendAmountDfm,
							BridgingType:     dr.BridgingType,
							Receivers:        receiverUsers,
							TokensInfo:       dr.TokensInfo,
						},
					)
					require.NoError(t, err)

					fmt.Printf("Sender: %d. run: %d. %s->%s tx sent: %s (token=%s)\n",
						idx+1, j+1, dr.SrcChain, dr.DstChain, txHash, dr.TokensInfo.DstTokenName)

					muSend.Lock()
					sendTxDatas = append(sendTxDatas, &extendedTxData{
						SrcChainID:     dr.SrcChain,
						DstChainID:     dr.DstChain,
						TxHash:         txHash,
						SendAmountDfm:  sendAmountDfm,
						BridgingTxType: dr.BridgingType,
						DstTokenName:   dr.TokensInfo.DstTokenName,
					})
					muSend.Unlock()
				}
			}(i, sender, drCopy)
		}
	}

	wgSend.Wait()

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

		expectedAmount := new(big.Int).Set(txData.SendAmountDfm)
		// Nexus uses Wei for sending amounts
		if txData.DstChainID == cardanofw.ChainIDNexus {
			expectedAmount = cardanofw.DfmToWei(expectedAmount)
		}

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
						sum.Add(sum, txData.SendAmountDfm)
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
					ctx, receiver, dstChain, srcChain, func(currentAmount *big.Int) bool {
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
						ctx, receiver, dstChain, srcChain, receivedAmount,
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
}

// Return token name of destination chain token from source chain and if it's native token on dest
func getTokenNameForChains(apex IApexSystem, dstChain, srcChain string, expectNativeTokens bool, coloredCoins ...uint16) string {
	if len(coloredCoins) > 0 {
		srcTokenID := coloredCoins[0]
		return apex.GetTokenNameForChains(dstChain, srcChain, srcTokenID)
	}

	if expectNativeTokens {
		srcTokenID := apex.GetTokenIDForChain(srcChain, true)
		if srcTokenID == 0 {
			return ""
		}

		return apex.GetTokenNameForChains(dstChain, srcChain, srcTokenID)
	}

	return cardanowallet.AdaTokenName
}
