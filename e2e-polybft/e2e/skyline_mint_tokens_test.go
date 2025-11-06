package e2e

import (
	"context"
	"fmt"
	"math/big"
	"sync"
	"testing"

	"github.com/0xPolygon/polygon-edge/e2e-polybft/cardanofw"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/e2ehelper"
	"github.com/Ethernal-Tech/cardano-infrastructure/sendtx"
	"github.com/stretchr/testify/require"
)

func TestE2E_SkylineBridgeMBA_Minting(t *testing.T) {
	const apiKey = "test_api_key"

	var lock sync.Mutex

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	// Combined configuration for both currency and native token tests
	primeConfig, cardanoConfig := cardanofw.NewPrimeChainConfig(), cardanofw.NewCardanoChainConfigWithMinting(true)

	apex := cardanofw.SetupAndRunSkylineBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithCardanoConfig(cardanoConfig),
		cardanofw.WithPrimeConfig(primeConfig),
		cardanofw.WithCustomConfigHandlers(func(a *cardanofw.ApexSystem, mp map[string]any) {
			t.Helper()

			lock.Lock()
			defer lock.Unlock()

			vcCfg := cardanofw.GetMapFromInterfaceKey(mp, "cardanoChains", cardanofw.ChainIDCardano)

			// Get nativeTokens slice
			nativeTokensInterface, ok := vcCfg["nativeTokens"].([]any)
			if !ok || len(nativeTokensInterface) == 0 {
				t.Fatalf("no native tokens found in config")

				return
			}

			// Get first token as a map
			firstToken, ok := nativeTokensInterface[0].(map[string]any)
			if !ok {
				t.Fatalf("invalid native token format")

				return
			}

			firstToken["mint"] = true
		}, nil),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	// Needed for this test to avoid NotEnoughFunds error on sc
	err := apex.UpdateChainTokenQuantity(cardanofw.ChainIDCardano, big.NewInt(100_000_000_000), true)
	require.NoError(t, err)

	fmt.Println("cardano native tokens: ", apex.CardanoInfo.NativeTokens)
	cardanoMintTokenName := apex.CardanoInfo.NativeTokens[0].TokenName

	user := apex.Users[0]

	checkAmounts := func(bridgingAddrAmount uint64, userAddrAmount uint64) {
		addrAmounts, err := apex.GetBridgingAddressesTokenAmounts(ctx, cardanofw.ChainIDCardano)
		require.NoError(t, err)

		userBalance, err := apex.GetBalance(ctx, user, cardanofw.ChainIDCardano)
		require.NoError(t, err)

		if bridgingAddrAmount == 0 {
			require.Nil(t, addrAmounts[0][cardanoMintTokenName])
		} else {
			require.Equal(t, bridgingAddrAmount, addrAmounts[0][cardanoMintTokenName].Uint64())
		}

		require.Equal(t, userAddrAmount, userBalance[cardanoMintTokenName].Uint64())
	}

	t.Run("1. full mint", func(t *testing.T) {
		t.Cleanup(func() {
			apex.ResetIndexers()
		})

		sendAmountDfm := big.NewInt(10_000_000)

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDPrime, cardanofw.ChainIDCardano, sendAmountDfm,
			sendtx.BridgingTypeCurrencyOnSource)

		checkAmounts(0, sendAmountDfm.Uint64())
	})

	t.Run("2. partial mint", func(t *testing.T) {
		t.Cleanup(func() {
			apex.ResetIndexers()
		})

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDCardano, cardanofw.ChainIDPrime, big.NewInt(5_000_000),
			sendtx.BridgingTypeNativeTokenOnSource)

		checkAmounts(5_000_000, 5_000_000)

		sendAmountDfm := big.NewInt(10_000_000)

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDPrime, cardanofw.ChainIDCardano, sendAmountDfm,
			sendtx.BridgingTypeCurrencyOnSource)

		checkAmounts(0, 15_000_000)
	})

	t.Run("3. burn", func(t *testing.T) {
		t.Cleanup(func() {
			apex.ResetIndexers()
		})

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDCardano, cardanofw.ChainIDPrime, big.NewInt(10_000_000),
			sendtx.BridgingTypeNativeTokenOnSource)

		checkAmounts(10_000_000, 5_000_000)

		sendAmountDfm := big.NewInt(5_000_000)

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDPrime, cardanofw.ChainIDCardano, sendAmountDfm,
			sendtx.BridgingTypeCurrencyOnSource)

		checkAmounts(0, 10_000_000)
	})

	t.Run("4. bridging to custodial addr", func(t *testing.T) {
		t.Cleanup(func() {
			apex.ResetIndexers()
		})

		custodialUser, err := cardanofw.NewApexUserTesting(apex.Config.CardanoConfig.CustodialAddress)
		require.NoError(t, err)

		sendAmountDfm := big.NewInt(5_000_000)

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, custodialUser, cardanofw.ChainIDPrime, cardanofw.ChainIDCardano, sendAmountDfm,
			sendtx.BridgingTypeCurrencyOnSource)

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, custodialUser, cardanofw.ChainIDPrime, cardanofw.ChainIDCardano, sendAmountDfm,
			sendtx.BridgingTypeCurrencyOnSource)
	})
}
