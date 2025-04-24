package e2ehelper

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"testing"

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
		// per each reciever -> per each chain -> per each token
		expectedAmountsPerRecv = make([]map[string]map[string]*big.Int, len(receiverUsers))
		expectNativeTokens     = bridgingType == sendtx.BridgingTypeCurrencyOnSource
	)

	for i, receiverUser := range receiverUsers {
		expectedAmountsPerRecv[i] = map[string]map[string]*big.Int{}
		balancePerChain := map[string]map[string]*big.Int{}

		for _, pair := range chainPairs {
			balance, exists := balancePerChain[pair.dstChain]
			if !exists {
				balance, err = apex.GetBalance(ctx, receiverUser, pair.dstChain)
				require.NoError(t, err)

				balancePerChain[pair.dstChain] = balance
				expectedAmountsPerRecv[i][pair.dstChain] = map[string]*big.Int{}
			}

			tokenName := getTokenNameForChains(apex, pair.dstChain, pair.srcChain, expectNativeTokens)
			expectedAmountsPerRecv[i][pair.dstChain][tokenName] = cardanofw.SetOrDefault(balance[tokenName], big.NewInt(0))
		}
	}

	config.sendTxStrategy(
		t, ctx, apex, chainsDst, senderUsers, receiverUsers, sendAmountDfm, txCountPerSender, bridgingType)

	// update expectedAmountPerChainDfm
	incrementPerReceiver := new(big.Int).Mul(
		sendAmountDfm, big.NewInt(int64(txCountPerSender)*int64(len(senderUsers))))

	for _, perChainMap := range expectedAmountsPerRecv {
		for _, perTokenMap := range perChainMap {
			for _, amount := range perTokenMap {
				amount.Add(amount, incrementPerReceiver)
			}
		}
	}

	config.restartValidatorStrategy(t, ctx, apex, config.restartValidatorsConfigs)

	err = waitForAmounts(
		ctx, apex, config, chainPairs, receiverUsers, expectedAmountsPerRecv, expectNativeTokens)
	require.NoError(t, err)
}

func waitForAmounts(
	ctx context.Context, apex IApexSystem, config *executeBridgingConfig, chainPairs []srcDstChainPair,
	receiverUsers []*cardanofw.TestApexUser, expectedAmountsPerRecv []map[string]map[string]*big.Int, expectNativeTokens bool,
) error {
	var (
		wg   sync.WaitGroup
		lock sync.Mutex
		errs []error
		// WaitForExactAmount recieves srcChain instead of tokenName so we need mapping
		srcChainMap = map[string]string{}
	)

	for _, pair := range chainPairs {
		tokenName := getTokenNameForChains(apex, pair.dstChain, pair.srcChain, expectNativeTokens)
		key := fmt.Sprintf("%s-%s", pair.dstChain, tokenName)
		// It doesn't matter if two source chains have the same token name (e.g., "lovelace")
		// for the same destination chain — just pick any one.
		srcChainMap[key] = pair.srcChain
	}

	for i, perChainMap := range expectedAmountsPerRecv {
		for dstChain, perTokenMap := range perChainMap {
			for tokenName, expectedAmount := range perTokenMap {
				wg.Add(1)

				go func(idx int, receiver *cardanofw.TestApexUser, dstChain string, srcChain string, expectedAmount *big.Int) {
					defer wg.Done()

					err := apex.WaitForExactAmount(
						ctx, receiver, dstChain, srcChain, expectedAmount,
						config.timeoutConfig.bridgingNumRetries, config.timeoutConfig.bridgingRetryWaitTime,
						expectNativeTokens)
					if err != nil {
						lock.Lock()
						errs = append(errs, fmt.Errorf("receiver %d on %s->%s error: %w", idx, srcChain, dstChain, err))
						lock.Unlock()

						return
					}

					fmt.Printf("TXs on %s for user %d expected amount received\n", dstChain, idx)

					if config.waitForUnexpectedBridges {
						// nothing else should be bridged for 2 minutes
						err := apex.WaitForGreaterAmount(
							ctx, receiver, dstChain, srcChain, expectedAmount,
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
				}(i, receiverUsers[i], dstChain, srcChainMap[fmt.Sprintf("%s-%s", dstChain, tokenName)], expectedAmount)
			}
		}
	}

	wg.Wait()

	return errors.Join(errs...)
}

func getTokenNameForChains(apex IApexSystem, dstChain, srcChain string, expectNativeTokens bool) string {
	if expectNativeTokens {
		return apex.GetTokenNameForChains(dstChain, srcChain)
	}

	return cardanowallet.AdaTokenName
}
