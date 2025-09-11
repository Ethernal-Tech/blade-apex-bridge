package e2ehelper

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/e2e-polybft/cardanofw"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/framework"
	"github.com/Ethernal-Tech/cardano-infrastructure/sendtx"
	cardanowallet "github.com/Ethernal-Tech/cardano-infrastructure/wallet"
)

type IApexSystem interface {
	SubmitBridgingRequest(
		t *testing.T, ctx context.Context,
		sourceChain cardanofw.ChainID, destinationChain cardanofw.ChainID,
		sender *cardanofw.TestApexUser, dfmAmount *big.Int, bridgingType sendtx.BridgingType,
		receivers ...*cardanofw.TestApexUser,
	) string
	WaitForGreaterAmount(
		ctx context.Context, user *cardanofw.TestApexUser, dstChain cardanofw.ChainID, srcChain cardanofw.ChainID,
		expectedAmountDfm *big.Int, numRetries int, waitTime time.Duration, isNativeToken ...bool,
	) error
	WaitForExactAmount(
		ctx context.Context, user *cardanofw.TestApexUser, dstChain cardanofw.ChainID, srcChain cardanofw.ChainID,
		expectedAmountDfm *big.Int, numRetries int, waitTime time.Duration, isNativeToken ...bool,
	) error
	WaitForAmount(
		ctx context.Context, user *cardanofw.TestApexUser, dstChain cardanofw.ChainID, srcChain cardanofw.ChainID,
		cmpHandler func(*big.Int) bool, numRetries int, waitTime time.Duration, isNativeToken ...bool,
	) (*big.Int, error)
	SubmitTx(
		ctx context.Context, sourceChain cardanofw.ChainID, sender *cardanofw.TestApexUser,
		receiver string, dfmAmount *big.Int, nativeTokenAmounts []cardanowallet.TokenAmount, data []byte,
	) (string, error)
	RedistributeTokens(
		ctx context.Context, chainID cardanofw.ChainID,
	) error
	WaitForRedistribution(
		ctx context.Context, chainID cardanofw.ChainID, cmpHandler func(*big.Int, *big.Int) bool,
		numRetries int, waitTime time.Duration,
	) error
	GetBalance(
		ctx context.Context, user *cardanofw.TestApexUser, chainID cardanofw.ChainID,
	) (map[string]*big.Int, error)
	GetTokenNameForChains(dstChain, srcChain cardanofw.ChainID) string
	GetValidator(t *testing.T, idx int) *cardanofw.TestApexValidator
	GetBridgeNode(t *testing.T, idx int) *framework.TestServer
	GetChainMust(t *testing.T, chainID cardanofw.ChainID) cardanofw.ITestApexChain
}

func getAllDestionationChains(chains []string, chainsDst map[string][]string) (res []string) {
	mp := map[string]bool{}

	for _, srcChain := range chains {
		for _, dstChain := range chainsDst[srcChain] {
			if !mp[dstChain] {
				mp[dstChain] = true

				res = append(res, dstChain)
			}
		}
	}

	return res
}

type srcDstChainPair struct {
	srcChain string
	dstChain string
}

func getAllChainPairs(chains []string, chainsDst map[string][]string) (res []srcDstChainPair) {
	for _, srcChain := range chains {
		for _, dstChain := range chainsDst[srcChain] {
			res = append(res, srcDstChainPair{
				srcChain: srcChain,
				dstChain: dstChain,
			})
		}
	}

	return res
}
