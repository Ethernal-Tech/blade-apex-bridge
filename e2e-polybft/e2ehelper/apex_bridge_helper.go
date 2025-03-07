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

	config := newExecuteBridgingConfig(options...)
	dstChains := getAllDestionationChains(chains, chainsDst)
	chainPairs := getAllChainPairs(chains, chainsDst)
	expectedAmountPerChainDfm := make([]map[string]*big.Int, len(receiverUsers))

	expectNativeTokens := bridgingType == sendtx.BridgingTypeCurrencyOnSource

	for i, receiverUser := range receiverUsers {
		expectedAmountPerChainDfm[i] = make(map[string]*big.Int)

		for _, dstChain := range dstChains {
			srcChain := getSrcFromDstChain(chainPairs, dstChain)

			balance, err := apex.GetBalance(ctx, receiverUser, dstChain)
			require.NoError(t, err)

			tokenName := getTokenNameForChains(apex, dstChain, srcChain, expectNativeTokens)
			expectedAmountPerChainDfm[i][dstChain] = cardanofw.SetOrDefault(balance[tokenName], big.NewInt(0))
		}
	}

	config.sendTxStrategy(t, ctx, apex, chainPairs, senderUsers, receiverUsers, sendAmountDfm, txCountPerSender,
		bridgingType)

	// update expectedAmountPerChainDfm
	for recieverUserIdx := range receiverUsers {
		for _, chainPair := range chainPairs {
			tmp := expectedAmountPerChainDfm[recieverUserIdx][chainPair.dstChain]
			tmp.Add(tmp, new(big.Int).Mul(sendAmountDfm, big.NewInt(int64(txCountPerSender)*int64(len(senderUsers)))))
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

			go func(idx int, idxChain int, receiverUser *cardanofw.TestApexUser, dstChain string, expectedAmount *big.Int) {
				defer wgResults.Done()

				var err error

				srcChain := getSrcFromDstChain(chainPairs, dstChain)

				err = apex.WaitForExactAmount(ctx, receiverUser, dstChain, srcChain, expectedAmount,
					config.timeoutConfig.bridgingNumRetries, config.timeoutConfig.bridgingRetryWaitTime, expectNativeTokens)

				if err != nil {
					errs[idx*len(dstChains)+idxChain] = fmt.Errorf("receiver %d on %s: %w", idx, dstChain, err)

					return
				}

				fmt.Printf("TXs on %s for user %d expected amount received\n", dstChain, idx)

				if config.waitForUnexpectedBridges {
					// nothing else should be bridged for 2 minutes
					srcChain := getSrcFromDstChain(chainPairs, dstChain)

					err = apex.WaitForGreaterAmount(
						ctx, receiverUser, dstChain, srcChain, expectedAmount, 12, time.Second*10, expectNativeTokens)

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

func getSrcFromDstChain(chainPairs []srcDstChainPair, dstChain string) string {
	for _, chainPair := range chainPairs {
		if chainPair.dstChain == dstChain {
			return chainPair.srcChain
		}
	}

	return ""
}

func getTokenNameForChains(apex IApexSystem, dstChain, srcChain string, expectNativeTokens bool) string {
	if expectNativeTokens {
		return apex.GetTokenNameForChains(dstChain, srcChain)
	}

	return cardanowallet.AdaTokenName
}
