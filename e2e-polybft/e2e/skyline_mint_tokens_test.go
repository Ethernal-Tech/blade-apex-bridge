package e2e

import (
	"context"
	"fmt"
	"math/big"
	"testing"

	"github.com/0xPolygon/polygon-edge/e2e-polybft/cardanofw"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/e2ehelper"
	"github.com/Ethernal-Tech/cardano-infrastructure/sendtx"
	"github.com/stretchr/testify/require"
)

func TestE2E_SkylineBridgeMint_Test1(t *testing.T) {
	const apiKey = "test_api_key"

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	// Combined configuration for both currency and native token tests
	primeConfig, cardanoConfig := cardanofw.NewPrimeChainConfig(), cardanofw.NewCardanoChainConfig(true)
	primeConfig.FundAmount = 0
	primeConfig.FundTokenAmount = 0
	cardanoConfig.FundTokenAmount = 1_500_000

	// Relayer funding for minting native tokens
	cardanoConfig.FundRelayerAmount = 100_000_000

	apex := cardanofw.SetupAndRunSkylineBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithCardanoConfig(cardanoConfig),
		cardanofw.WithPrimeConfig(primeConfig),
		cardanofw.WithCustomConfigHandlers(func(a *cardanofw.ApexSystem, _ map[string]interface{}) {
			a.CardanoInfo.NativeTokens[0].Mint = false
			// a.CardanoInfo.NativeTokens[0].TokenName = "policyID.mintable_token"
		}, nil),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	fmt.Println("cardano native tokens: ", apex.CardanoInfo.NativeTokens)

	user := apex.Users[0]

	t.Run("1. prime -> cardano - currency on src", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() {
			t.Skip()
		}

		t.Cleanup(func() {
			apex.ResetIndexers()
		})

		sendAmountDfm := big.NewInt(1_500_000)

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDPrime, cardanofw.ChainIDCardano, sendAmountDfm,
			sendtx.BridgingTypeCurrencyOnSource)
	})
}
