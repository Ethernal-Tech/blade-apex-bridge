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
	"github.com/stretchr/testify/require"
)

func ExecuteSingleBridging(
	t *testing.T, ctx context.Context, apex IApexSystem, senderUser, receiverUser *cardanofw.TestApexUser,
	srcChain, dstChain string, sendAmountDfm *big.Int, bridgingType sendtx.BridgingType, options ...ExecuteBridgingOption,
) {
	t.Helper()

	config := newExecuteBridgingConfig(options...)

	prevAmountDfm, err := apex.GetBalance(ctx, receiverUser, dstChain)
	require.NoError(t, err)

	prevTokenAmount, err := apex.GetNativeTokenBalance(ctx, receiverUser, dstChain, srcChain)
	require.NoError(t, err)

	txHash := apex.SubmitBridgingRequest(
		t, ctx, srcChain, dstChain, senderUser, sendAmountDfm, bridgingType, receiverUser)

	expectedAmountDest := sendAmountDfm
	if bridgingType == sendtx.BridgingTypeCurrencyOnSource {
		expectedAmountDest = new(big.Int).SetUint64(cardanofw.MinUTxODefaultValue)
	}

	expectedAmountDfm := new(big.Int).Add(prevAmountDfm, expectedAmountDest)

	fmt.Printf("Tx sent. hash: %s\n", txHash)

	// check expected amount cardano
	if bridgingType != sendtx.BridgingTypeCurrencyOnSource {
		err = apex.WaitForExactAmount(ctx, receiverUser, dstChain, expectedAmountDfm,
			config.timeoutConfig.bridgingNumRetries, config.timeoutConfig.bridgingRetryWaitTime)
	} else {
		expectedTokenAmount := new(big.Int).Add(prevTokenAmount, sendAmountDfm)

		err = apex.WaitForExactTokenAmount(ctx, receiverUser, dstChain, expectedTokenAmount, srcChain,
			config.timeoutConfig.bridgingNumRetries, config.timeoutConfig.bridgingRetryWaitTime)
	}

	require.NoError(t, err)
}

func ExecuteBridgingOneByOneWaitOnOtherSide(
	t *testing.T, ctx context.Context, apex IApexSystem, txCountPerSender int,
	user *cardanofw.TestApexUser, srcChain, dstChain string, sendAmountDfm *big.Int, bridgingType sendtx.BridgingType,
	options ...ExecuteBridgingOption,
) {
	t.Helper()

	config := newExecuteBridgingConfig(options...)

	for i := 0; i < txCountPerSender; i++ {
		prevAmountDfm, err := apex.GetBalance(ctx, user, dstChain)
		require.NoError(t, err)

		prevTokenAmount, err := apex.GetNativeTokenBalance(ctx, user, dstChain, srcChain)
		require.NoError(t, err)

		apex.SubmitBridgingRequest(t, ctx, srcChain, dstChain, user, sendAmountDfm, bridgingType, user)
		expectedAmountDfm := new(big.Int).Add(prevAmountDfm, sendAmountDfm)
		expectedTokenAmount := new(big.Int).Add(prevTokenAmount, sendAmountDfm)

		if bridgingType != sendtx.BridgingTypeCurrencyOnSource {
			err = apex.WaitForExactAmount(ctx, user, dstChain, expectedAmountDfm,
				config.timeoutConfig.bridgingNumRetries, config.timeoutConfig.bridgingRetryWaitTime)
		} else {
			err = apex.WaitForExactTokenAmount(ctx, user, dstChain, expectedTokenAmount, srcChain,
				config.timeoutConfig.bridgingNumRetries, config.timeoutConfig.bridgingRetryWaitTime)
		}

		require.NoError(t, err)
	}
}

func ExecuteBridgingWaitAfterSubmits(
	t *testing.T, ctx context.Context, apex IApexSystem, txCountPerSender int,
	user *cardanofw.TestApexUser, srcChain, dstChain string, sendAmountDfm *big.Int, bridgingType sendtx.BridgingType,
	options ...ExecuteBridgingOption,
) {
	t.Helper()

	config := newExecuteBridgingConfig(options...)

	prevAmountDfm, err := apex.GetBalance(ctx, user, dstChain)
	require.NoError(t, err)

	prevTokenAmount, err := apex.GetNativeTokenBalance(ctx, user, dstChain, srcChain)
	require.NoError(t, err)

	expectedAmountDfm := new(big.Int).Set(prevAmountDfm)
	expectedTokenAmount := new(big.Int).Add(prevTokenAmount,
		new(big.Int).Mul(sendAmountDfm, big.NewInt(int64(txCountPerSender))))

	for i := 0; i < txCountPerSender; i++ {
		apex.SubmitBridgingRequest(t, ctx, srcChain, dstChain, user, sendAmountDfm, bridgingType, user)
		expectedAmountDfm = expectedAmountDfm.Add(expectedAmountDfm, sendAmountDfm)
	}

	if bridgingType != sendtx.BridgingTypeCurrencyOnSource {
		err = apex.WaitForExactAmount(ctx, user, dstChain, expectedAmountDfm,
			config.timeoutConfig.bridgingNumRetries, config.timeoutConfig.bridgingRetryWaitTime)
	} else {
		err = apex.WaitForExactTokenAmount(ctx, user, dstChain, expectedTokenAmount, srcChain,
			config.timeoutConfig.bridgingNumRetries, config.timeoutConfig.bridgingRetryWaitTime)
	}

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

	for i, receiverUser := range receiverUsers {
		expectedAmountPerChainDfm[i] = make(map[string]*big.Int)

		for _, dstChain := range dstChains {
			if bridgingType != sendtx.BridgingTypeCurrencyOnSource {
				dfm, err := apex.GetBalance(ctx, receiverUser, dstChain)
				require.NoError(t, err)

				expectedAmountPerChainDfm[i][dstChain] = dfm
			} else {
				srcChain := getSrcFromDstChain(chainPairs, dstChain)

				token, err := apex.GetNativeTokenBalance(ctx, receiverUser, dstChain, srcChain)
				require.NoError(t, err)

				expectedAmountPerChainDfm[i][dstChain] = token
			}
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

			go func(idx int, idxChain int, receiverUser *cardanofw.TestApexUser, dstChain string, expectedAmountDfm *big.Int) {
				defer wgResults.Done()

				var err error

				if bridgingType == sendtx.BridgingTypeNormal || bridgingType == sendtx.BridgingTypeNativeTokenOnSource {
					err = apex.WaitForExactAmount(
						ctx, receiverUser, dstChain, expectedAmountDfm,
						len(receiverUsers)*config.timeoutConfig.bridgingNumRetries,
						config.timeoutConfig.bridgingRetryWaitTime)
				} else {
					srcChain := getSrcFromDstChain(chainPairs, dstChain)

					err = apex.WaitForExactTokenAmount(ctx, receiverUser, dstChain, expectedAmountDfm, srcChain,
						len(receiverUsers)*config.timeoutConfig.bridgingNumRetries,
						config.timeoutConfig.bridgingRetryWaitTime)
				}

				if err != nil {
					errs[idx*len(dstChains)+idxChain] = fmt.Errorf("receiver %d on %s: %w", idx, dstChain, err)

					return
				}

				fmt.Printf("TXs on %s for user %d expected amount received\n", dstChain, idx)

				if config.waitForUnexpectedBridges {
					// nothing else should be bridged for 2 minutes
					if bridgingType != sendtx.BridgingTypeCurrencyOnSource {
						err = apex.WaitForGreaterAmount(
							ctx, receiverUser, dstChain, expectedAmountDfm, 12, time.Second*10)
					} else {
						srcChain := getSrcFromDstChain(chainPairs, dstChain)

						err = apex.WaitForGreaterTokenAmount(
							ctx, receiverUser, dstChain, expectedAmountDfm, srcChain, 12, time.Second*10)
					}

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
