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

func Test_CardanoToNexus(t *testing.T) {
	const apiKey = "test_api_key"

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	primeConfig, cardanoConfig := cardanofw.NewPrimeChainConfig(), cardanofw.NewCardanoChainConfig(true)
	vectorConfig := cardanofw.NewVectorChainConfig(map[uint16]string{cardanofw.USDTTokenID: cardanofw.USDTTokenName})
	nexusConfig := cardanofw.NewNexusChainConfig(true)
	cardanoConfig.FundTokenAmount = 1_000_000_000
	vectorConfig.FundTokenAmount = 1_000_000_000

	apex := cardanofw.SetupAndRunSkylineBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithCardanoConfig(cardanoConfig),
		cardanofw.WithPrimeConfig(primeConfig),
		cardanofw.WithVectorConfig(vectorConfig),
		cardanofw.WithNexusConfig(nexusConfig),
		cardanofw.WithBridgingAddrCnt(cardanofw.ChainIDPrime, bridgeAddrCnt),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	user := apex.Users[0]

	fmt.Printf("User: %+v\n", user.GetAddress(cardanofw.ChainIDNexus))

	t.Run("Cardano -> Nexus - ADA -> xADA", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDCardano, cardanofw.ChainIDNexus, big.NewInt(100_000_000),
			sendtx.BridgingTypeCurrencyOnSource)
	})

}

func Test_SkylineBridgeMint_General(t *testing.T) {
	const apiKey = "test_api_key"

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	primeConfig, cardanoConfig := cardanofw.NewPrimeChainConfig(), cardanofw.NewCardanoChainConfig(true)
	vectorConfig := cardanofw.NewVectorChainConfig(map[uint16]string{cardanofw.USDTTokenID: cardanofw.USDTTokenName})
	nexusConfig := cardanofw.NewNexusChainConfig(true)
	cardanoConfig.FundTokenAmount = 1_000_000_000
	vectorConfig.FundTokenAmount = 1_000_000_000

	apex := cardanofw.SetupAndRunSkylineBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithCardanoConfig(cardanoConfig),
		cardanofw.WithPrimeConfig(primeConfig),
		cardanofw.WithVectorConfig(vectorConfig),
		cardanofw.WithNexusConfig(nexusConfig),
		cardanofw.WithBridgingAddrCnt(cardanofw.ChainIDPrime, bridgeAddrCnt),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	user := apex.Users[0]

	fmt.Printf("User: %+v\n", user.GetAddress(cardanofw.ChainIDNexus))

	t.Run("Nexus -> Vector USTD -> wUSDT", func(t *testing.T) {
		nexusChain := apex.GetChainMust(t, cardanofw.ChainIDNexus).(*cardanofw.TestEVMChain)
		err := nexusChain.FundUsersWithToken(user.GetAddress(cardanofw.ChainIDNexus), big.NewInt(1), cardanofw.USDTTokenID)
		require.NoError(t, err)

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDNexus, cardanofw.ChainIDVector, big.NewInt(1),
			sendtx.BridgingTypeColoredCoinOnSource, e2ehelper.WithColoredCoins([]uint16{cardanofw.USDTTokenID}))
	})

	t.Run("Vector -> Nexus wUSDT -> USTD", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDVector, cardanofw.ChainIDNexus, big.NewInt(1),
			sendtx.BridgingTypeColoredCoinOnSource, e2ehelper.WithColoredCoins([]uint16{cardanofw.USDTTokenID}))
	})

	t.Run("Cardano -> Nexus - ADA -> xADA", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDCardano, cardanofw.ChainIDNexus, big.NewInt(100_000_000),
			sendtx.BridgingTypeCurrencyOnSource)
	})

	t.Run("Nexus -> Cardano - xADA -> ADA", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDNexus, cardanofw.ChainIDCardano, big.NewInt(10_000_000),
			sendtx.BridgingTypeCurrencyOnSource)
	})

	t.Run("Cardano -> Vector - ADA -> xADA", func(t *testing.T) {
		t.Cleanup(func() {
			apex.ResetIndexers()
		})

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDCardano, cardanofw.ChainIDVector, big.NewInt(10_000_000),
			sendtx.BridgingTypeCurrencyOnSource)
	})

	t.Run("Vector -> Cardano - xADA -> ADA", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDVector, cardanofw.ChainIDCardano, big.NewInt(10_000_000),
			sendtx.BridgingTypeWrappedTokenOnSource)
	})

	t.Run("Vector -> Nexus xADA -> xADA", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDVector, cardanofw.ChainIDNexus, big.NewInt(1),
			sendtx.BridgingTypeColoredCoinOnSource, e2ehelper.WithColoredCoins([]uint16{cardanofw.XADATokenID}))
	})

	t.Run("Nexus -> Vector xADA -> xADA", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDNexus, cardanofw.ChainIDVector, big.NewInt(1),
			sendtx.BridgingTypeColoredCoinOnSource, e2ehelper.WithColoredCoins([]uint16{cardanofw.XADATokenID}))
	})

	t.Run("Prime -> Cardano - AP3X -> CAP3X", func(t *testing.T) {
		t.Cleanup(func() {
			apex.ResetIndexers()
		})

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDPrime, cardanofw.ChainIDCardano, big.NewInt(10_000_000),
			sendtx.BridgingTypeCurrencyOnSource)
	})

	t.Run("Cardano -> Prime - CAP3X -> AP3X", func(t *testing.T) {
		t.Cleanup(func() {
			apex.ResetIndexers()
		})

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDCardano, cardanofw.ChainIDPrime, big.NewInt(10_000_000),
			sendtx.BridgingTypeWrappedTokenOnSource)
	})
}

/*
func TestE2E_SkylineBridgeMint_General(t *testing.T) {
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

	checkAmounts := func(bridgingAddrAmount uint64, apexUser *cardanofw.TestApexUser, userAddrAmount uint64) {
		addrAmounts, err := apex.GetBridgingAddressesTokenAmounts(ctx, cardanofw.ChainIDCardano)
		require.NoError(t, err)

		userBalance, err := apex.GetBalance(ctx, apexUser, cardanofw.ChainIDCardano)
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

		checkAmounts(0, user, sendAmountDfm.Uint64())
	})

	t.Run("2. partial mint", func(t *testing.T) {
		t.Cleanup(func() {
			apex.ResetIndexers()
		})

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDCardano, cardanofw.ChainIDPrime, big.NewInt(5_000_000),
			sendtx.BridgingTypeWrappedTokenOnSource)

		checkAmounts(5_000_000, user, 5_000_000)

		sendAmountDfm := big.NewInt(10_000_000)

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDPrime, cardanofw.ChainIDCardano, sendAmountDfm,
			sendtx.BridgingTypeCurrencyOnSource)

		checkAmounts(0, user, 15_000_000)
	})

	t.Run("3. burn", func(t *testing.T) {
		t.Cleanup(func() {
			apex.ResetIndexers()
		})

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDCardano, cardanofw.ChainIDPrime, big.NewInt(10_000_000),
			sendtx.BridgingTypeWrappedTokenOnSource)

		checkAmounts(10_000_000, user, 5_000_000)

		sendAmountDfm := big.NewInt(5_000_000)

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDPrime, cardanofw.ChainIDCardano, sendAmountDfm,
			sendtx.BridgingTypeCurrencyOnSource)

		checkAmounts(0, user, 10_000_000)
	})

	t.Run("4. bridging to custodial, relayer and cardano script addrs", func(t *testing.T) {
		t.Cleanup(func() {
			apex.ResetIndexers()
		})

		cardanoChain := apex.GetChainMust(t, cardanofw.ChainIDCardano)
		sendAmountDfm := big.NewInt(5_000_000)
		doubleAmount := new(big.Int).Mul(sendAmountDfm, big.NewInt(2)).Uint64()

		addresses := []string{
			cardanoChain.GetCustodialAddress(),
			cardanoChain.GetRelayerAddress(),
			cardanoChain.GetCardanoScriptInfo().PlutusAddress,
		}

		for _, addr := range addresses {
			cardanoAddr, err := cardanowallet.NewCardanoAddressFromString(addr)
			require.NoError(t, err)

			apexUser := &cardanofw.TestApexUser{
				HasCardanoWallet: true,
				CardanoAddress:   cardanoAddr,
			}

			for range 2 {
				e2ehelper.ExecuteSingleBridging(
					t, ctx, apex, user, apexUser, cardanofw.ChainIDPrime, cardanofw.ChainIDCardano, sendAmountDfm,
					sendtx.BridgingTypeCurrencyOnSource)
			}

			checkAmounts(0, apexUser, doubleAmount)
		}
	})

	t.Run("5. send invalid token to to special addrs then bridge", func(t *testing.T) {
		cardanoChain := apex.GetChainMust(t, cardanofw.ChainIDCardano).(*cardanofw.TestCardanoChain)

		invalidTokenAmount := uint64(1000)
		invalidTokenName := "invalid-token"
		err = cardanofw.MintToken(cardanoChain, apex.CardanoInfo.GenesisWallet, invalidTokenName, invalidTokenAmount*3)
		require.NoError(t, err)

		addresses := []string{
			cardanoChain.GetCustodialAddress(),
			cardanoChain.GetRelayerAddress(),
			cardanoChain.GetCardanoScriptInfo().PlutusAddress,
		}

		for _, addr := range addresses {
			cardanoAddr, err := cardanowallet.NewCardanoAddressFromString(addr)
			require.NoError(t, err)

			apexUser := &cardanofw.TestApexUser{
				HasCardanoWallet: true,
				CardanoAddress:   cardanoAddr,
			}

			_, err = cardanofw.FundUsersWithToken(ctx, cardanoChain, apex.CardanoInfo.GenesisWallet,
				[]*cardanofw.TestApexUser{apexUser}, invalidTokenName, 2_000_000, invalidTokenAmount)
			require.NoError(t, err)

			e2ehelper.ExecuteSingleBridging(
				t, ctx, apex, user, user, cardanofw.ChainIDCardano, cardanofw.ChainIDPrime, big.NewInt(2_000_000),
				sendtx.BridgingTypeWrappedTokenOnSource)

			e2ehelper.ExecuteSingleBridging(
				t, ctx, apex, user, user, cardanofw.ChainIDPrime, cardanofw.ChainIDCardano, big.NewInt(5_000_000),
				sendtx.BridgingTypeCurrencyOnSource)
		}
	})
}

*/
