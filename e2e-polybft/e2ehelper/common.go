package e2ehelper

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/e2e-polybft/cardanofw"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/framework"
	cardanowallet "github.com/Ethernal-Tech/cardano-infrastructure/wallet"
)

type IApexSystem interface {
	SubmitBridgingRequest(
		data cardanofw.SubmitBridgingRequestData,
	) (string, error)
	WaitForGreaterAmount(
		ctx context.Context, user *cardanofw.TestApexUser, chain cardanofw.ChainID,
		expectedAmount *big.Int, numRetries int, waitTime time.Duration, tokenName string,
	) error
	WaitForExactAmount(
		ctx context.Context, user *cardanofw.TestApexUser, chain cardanofw.ChainID,
		expectedAmountDfm *big.Int, numRetries int, waitTime time.Duration, tokenName string,
	) error
	WaitForAmount(
		ctx context.Context, user *cardanofw.TestApexUser, chain cardanofw.ChainID,
		cmpHandler func(*big.Int) bool, numRetries int, waitTime time.Duration, tokenName string,
	) (*big.Int, error)
	WaitForAmountInRange(
		ctx context.Context, user *cardanofw.TestApexUser, chain cardanofw.ChainID,
		lowerBoundaryDfm *big.Int, higherBoundaryDfm *big.Int, numRetries int, retryDelay time.Duration, tokenName string,
	) error
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
	GetBalanceWithTokenName(
		ctx context.Context, user *cardanofw.TestApexUser, chainID cardanofw.ChainID, tokenName string,
	) (map[string]*big.Int, error)
	GetChainCurrencyID(chain cardanofw.ChainID) (uint16, error)
	GetChainWrappedCurrencyID(chain cardanofw.ChainID) (uint16, error)
	GetChainTokenInfo(chain cardanofw.ChainID, tokenID uint16) (cardanofw.Token, error)
	GetBridgingTokensInfo(
		srcChain, dstChain cardanofw.ChainID, srcTokenID uint16) (*cardanofw.BridgingTokensInfo, error)
	GetValidator(t *testing.T, idx int) *cardanofw.TestApexValidator
	GetBridgeNode(t *testing.T, idx int) *framework.TestServer
	GetChainMust(t *testing.T, chainID cardanofw.ChainID) cardanofw.ITestApexChain
}

type SrcDstChainPair struct {
	srcChain string
	dstChain string
}

func NewChainPair(src, dst string) SrcDstChainPair {
	return SrcDstChainPair{
		srcChain: src,
		dstChain: dst,
	}
}

func getAllChainPairs(chains []string, chainsDst map[string][]string) (res []SrcDstChainPair) {
	for _, srcChain := range chains {
		for _, dstChain := range chainsDst[srcChain] {
			res = append(res, SrcDstChainPair{
				srcChain: srcChain,
				dstChain: dstChain,
			})
		}
	}

	return res
}

// For ExecuteBridgingExtended
// It allows specifying multiple bridging directions between the same src / dst chains by
// including the token ID as part of the configuration.
type BridgingDirectionConfig struct {
	SrcChain   string
	DstChain   string
	SrcTokenID uint16
}
