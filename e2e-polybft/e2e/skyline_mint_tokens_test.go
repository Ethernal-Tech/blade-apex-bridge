package e2e

import (
	"context"
	"fmt"
	"math/big"
	"testing"

	"github.com/0xPolygon/polygon-edge/e2e-polybft/cardanofw"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/e2ehelper"
	"github.com/Ethernal-Tech/cardano-infrastructure/sendtx"
	"github.com/Ethernal-Tech/cardano-infrastructure/wallet"
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
			cardanofw.BridgingTypeCurrencyOnSource)
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

	t.Run("Nexus <-> Vector USTD <-> wUSDT", func(t *testing.T) {
		nexusChain := apex.GetChainMust(t, cardanofw.ChainIDNexus).(*cardanofw.TestEVMChain)
		err := nexusChain.FundUsersWithToken(user.GetAddress(cardanofw.ChainIDNexus), big.NewInt(1), cardanofw.USDTTokenID)
		require.NoError(t, err)

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDNexus, cardanofw.ChainIDVector, big.NewInt(1),
			cardanofw.BridgingTypeColoredCoinOnSource, e2ehelper.WithColoredCoins([]uint16{cardanofw.USDTTokenID}))

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDVector, cardanofw.ChainIDNexus, big.NewInt(1),
			cardanofw.BridgingTypeColoredCoinOnSource, e2ehelper.WithColoredCoins([]uint16{cardanofw.USDTTokenID}))
	})

	t.Run("Cardano -> Nexus - ADA -> xADA", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDCardano, cardanofw.ChainIDNexus, big.NewInt(100_000_000),
			cardanofw.BridgingTypeCurrencyOnSource)
	})

	t.Run("Nexus -> Cardano - xADA -> ADA", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDNexus, cardanofw.ChainIDCardano, big.NewInt(10_000_000),
			cardanofw.BridgingTypeColoredCoinOnSource, e2ehelper.WithColoredCoins([]uint16{cardanofw.XADATokenID}))
	})

	t.Run("Cardano -> Vector - ADA -> xADA", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDCardano, cardanofw.ChainIDVector, big.NewInt(20_000_000),
			cardanofw.BridgingTypeCurrencyOnSource)
	})

	t.Run("Vector -> Cardano - xADA -> ADA", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDVector, cardanofw.ChainIDCardano, big.NewInt(10_000_000),
			cardanofw.BridgingTypeWrappedTokenOnSource)
	})

	t.Run("Vector -> Nexus xADA -> xADA", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDVector, cardanofw.ChainIDNexus, big.NewInt(1),
			cardanofw.BridgingTypeColoredCoinOnSource, e2ehelper.WithColoredCoins([]uint16{cardanofw.XADATokenID}))
	})

	t.Run("Nexus -> Vector xADA -> xADA", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDNexus, cardanofw.ChainIDVector, big.NewInt(1),
			cardanofw.BridgingTypeColoredCoinOnSource, e2ehelper.WithColoredCoins([]uint16{cardanofw.XADATokenID}))
	})

	t.Run("Prime -> Cardano - AP3X -> CAP3X", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDPrime, cardanofw.ChainIDCardano, big.NewInt(10_000_000),
			cardanofw.BridgingTypeCurrencyOnSource)
	})

	t.Run("Cardano -> Prime - CAP3X -> AP3X", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDCardano, cardanofw.ChainIDPrime, big.NewInt(10_000_000),
			cardanofw.BridgingTypeWrappedTokenOnSource)
	})
}

func TestE2E_SkylineMintTokens_InvalidScenarios_RefundDisabled(t *testing.T) {
	const (
		apiKey  = "test_api_key"
		userCnt = 10

		maxWaitTimeSec = 600
		retryDelaySec  = 5
	)

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
		cardanofw.WithCustomConfigHandlers(func(_ *cardanofw.ApexSystem, mp map[string]interface{}) {
			mp["refundEnabled"] = false
		}, nil, nil),
		cardanofw.WithBridgingAddrCnt(cardanofw.ChainIDPrime, bridgeAddrCnt),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	user := apex.Users[0]

	fmt.Printf("User: %+v\n", user.GetAddress(cardanofw.ChainIDNexus))

	cardanoTestConfig := newTestConfig(
		t, apex.Config.CardanoConfig, &apex.CardanoInfo, cardanofw.ChainIDNexus, "")
	vectorTestConfig := newTestConfig(
		t, apex.Config.VectorConfig, &apex.VectorInfo, cardanofw.ChainIDNexus, "")

	bridgingType := cardanofw.BridgingTypeCurrencyOnSource

	t.Run("1. Cardano -> Nexus - Mismatch submitted and receiver amounts", func(t *testing.T) {
		executeInvalidMismatchSendLovelaceAmount(t, ctx, apex, cardanoTestConfig, user, maxWaitTimeSec, retryDelaySec, bridgingType, false, 0)
	})

	t.Run("2. Cardano -> Nexus - Multiple submitters mismatch submitted and receiver amounts", func(t *testing.T) {
		executeInvalidMismatchSendAmountMultipleInstances(t, ctx, apex, cardanoTestConfig, maxWaitTimeSec, retryDelaySec, bridgingType, false, 0)
	})

	t.Run("3. Invalid bridging type Vector -> Nexus - currency on src", func(t *testing.T) {
		executeInvalidTokenDirection(t, ctx, apex, vectorTestConfig, user, maxWaitTimeSec, retryDelaySec, cardanofw.BridgingTypeCurrencyOnSource, false, 0)
	})

	//nolint:dupl
	t.Run("4.Submitted invalid metadata - currency under min - token on source", func(t *testing.T) {
		sendAmount := uint64(1_000_000)

		user, err := cardanofw.NewTestApexUser(cardanofw.NewApexNetworkTypesFromSystem(apex))
		require.NoError(t, err)

		tokensFunded, err := cardanofw.FundUserWithToken(
			ctx, apex, cardanofw.ChainIDVector,
			apex.VectorInfo.GenesisWallet, user,
			cardanofw.XADATokenName, cardanofw.DefaultTokenMintAmount,
			uint64(10_000_000), uint64(10_000_000))
		require.NoError(t, err)

		receivers := []sendtx.BridgingTxReceiver{
			{
				Addr:    user.GetAddress(cardanofw.ChainIDNexus),
				Amount:  sendAmount,
				TokenID: cardanofw.XADATokenID,
			},
		}

		operationFee := apex.GetMinOperationFee(cardanofw.ChainIDVector)

		feeAmount, err := apex.GetChainMust(t, cardanofw.ChainIDVector).GetBridgingFee(
			ctx, cardanofw.ChainIDNexus, receivers, apex.GetMinBridgingFee(cardanofw.ChainIDVector, true),
			operationFee, apex.VectorInfo.MultisigAddr[0])
		require.NoError(t, err)

		feeAmount -= 1_000_000

		metadata, err := apex.GetChainMust(t, cardanofw.ChainIDVector).CreateMetadata(
			user.GetAddress(cardanofw.ChainIDVector), cardanofw.ChainIDNexus,
			receivers, feeAmount, operationFee)
		require.NoError(t, err)

		txHash, err := apex.SubmitTx(
			ctx, cardanofw.ChainIDVector, user,
			apex.VectorInfo.MultisigAddr[0], new(big.Int).SetUint64(sendAmount+feeAmount+operationFee),
			[]wallet.TokenAmount{
				{Token: tokensFunded.Token, Amount: sendAmount},
			},
			metadata)
		require.NoError(t, err)

		cardanofw.WaitForInvalidState(t, ctx, apex, cardanofw.ChainIDVector, txHash, apex.Config.APIKey, 0)
	})

	t.Run("5. Cardano -> Nexus - Submitted invalid metadata - wrong type", func(t *testing.T) {
		executeInvalidMetadataType(t, ctx, apex, cardanoTestConfig, user, 60, retryDelaySec, bridgingType, false, 0)
	})

	t.Run("6. Cardano -> Nexus - Submitted invalid metadata - invalid destination", func(t *testing.T) {
		executeInvalidDestination(t, ctx, apex, cardanoTestConfig, user, maxWaitTimeSec, retryDelaySec, bridgingType, false, 0)
	})

	t.Run("7. Cardano -> Nexus - Submitted invalid metadata - invalid sender", func(t *testing.T) {
		executeInvalidMetadataInvalidSender(t, ctx, apex, cardanoTestConfig, user, maxWaitTimeSec, bridgingType, 0)
	})

	t.Run("8. Cardano -> Nexus - Submitted invalid metadata - invalid bridging fee", func(t *testing.T) {
		executeInvalidBridgingFee(t, ctx, apex, cardanoTestConfig, maxWaitTimeSec, retryDelaySec, cardanofw.BridgingTypeCurrencyOnSource, false, 0)
	})

	t.Run("9. Cardano -> Nexus - Submitted invalid metadata - empty receivers", func(t *testing.T) {
		executeInvalidEmptyReceivers(t, ctx, apex, cardanoTestConfig, user, maxWaitTimeSec, retryDelaySec, cardanofw.BridgingTypeCurrencyOnSource, false, 0)
	})

	t.Run("10. Vector -> Nexus - Submitted with unknown tokens to bridging addr", func(t *testing.T) {
		user := apex.Users[userCnt-1]
		minterWallet, _ := user.GetCardanoWallet(cardanofw.ChainIDVector)

		tokensFunded, err := cardanofw.FundUserWithToken(
			ctx, apex, cardanofw.ChainIDVector,
			minterWallet, user,
			cardanofw.XADATokenName, cardanofw.DefaultTokenMintAmount,
			uint64(1_500_000), uint64(1_000_000))
		require.NoError(t, err)

		executeInvalidSendNativeToken(t, ctx, apex, user, vectorTestConfig, *tokensFunded, maxWaitTimeSec, retryDelaySec, false, 0, cardanofw.BridgingTypeCurrencyOnSource)
	})

	t.Run("11. Vector -> Nexus - Submitted invalid metadata - invalid send amount - token on source", func(t *testing.T) {
		user, err := cardanofw.NewTestApexUser(cardanofw.NewApexNetworkTypesFromSystem(apex))
		require.NoError(t, err)

		tokensFunded, err := cardanofw.FundUserWithToken(
			ctx, apex, cardanofw.ChainIDVector,
			apex.VectorInfo.GenesisWallet, user,
			cardanofw.XADATokenName, cardanofw.DefaultTokenMintAmount,
			uint64(10_000_000), uint64(1_123_000))
		require.NoError(t, err)

		executeInvalidMismatchSendNativeTokenAmount(t, ctx, apex, user, vectorTestConfig, *tokensFunded, maxWaitTimeSec, retryDelaySec, false, 0)
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
			cardanofw.BridgingTypeCurrencyOnSource)

		checkAmounts(0, user, sendAmountDfm.Uint64())
	})

	t.Run("2. partial mint", func(t *testing.T) {
		t.Cleanup(func() {
			apex.ResetIndexers()
		})

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDCardano, cardanofw.ChainIDPrime, big.NewInt(5_000_000),
			cardanofw.BridgingTypeWrappedTokenOnSource)

		checkAmounts(5_000_000, user, 5_000_000)

		sendAmountDfm := big.NewInt(10_000_000)

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDPrime, cardanofw.ChainIDCardano, sendAmountDfm,
			cardanofw.BridgingTypeCurrencyOnSource)

		checkAmounts(0, user, 15_000_000)
	})

	t.Run("3. burn", func(t *testing.T) {
		t.Cleanup(func() {
			apex.ResetIndexers()
		})

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDCardano, cardanofw.ChainIDPrime, big.NewInt(10_000_000),
			cardanofw.BridgingTypeWrappedTokenOnSource)

		checkAmounts(10_000_000, user, 5_000_000)

		sendAmountDfm := big.NewInt(5_000_000)

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDPrime, cardanofw.ChainIDCardano, sendAmountDfm,
			cardanofw.BridgingTypeCurrencyOnSource)

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
					cardanofw.BridgingTypeCurrencyOnSource)
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
				cardanofw.BridgingTypeWrappedTokenOnSource)

			e2ehelper.ExecuteSingleBridging(
				t, ctx, apex, user, user, cardanofw.ChainIDPrime, cardanofw.ChainIDCardano, big.NewInt(5_000_000),
				cardanofw.BridgingTypeCurrencyOnSource)
		}
	})
}

*/
