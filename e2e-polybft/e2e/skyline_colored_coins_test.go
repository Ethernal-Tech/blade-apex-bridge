package e2e

import (
	"bytes"
	"context"
	"fmt"
	"math/big"
	"sync"
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

	t.Run("Test SendTx with erc20 tokens on Nexus", func(t *testing.T) {
		nexusChain := apex.GetChainMust(t, cardanofw.ChainIDNexus).(*cardanofw.TestEVMChain)
		err := nexusChain.FundUsersWithToken(user.GetAddress(cardanofw.ChainIDNexus), big.NewInt(2), cardanofw.USDTTokenID)
		require.NoError(t, err)

		receiver := apex.Users[1]

		balance, err := nexusChain.GetAddressBalance(ctx, receiver.GetAddress(cardanofw.ChainIDNexus))
		require.NoError(t, err)
		fmt.Printf("Receiver token balance before: %+v\n", balance)

		usdtTokenBalance, err := apex.GetBalanceWithTokenName(ctx, receiver, cardanofw.ChainIDNexus, apex.NexusInfo.Tokens[cardanofw.USDTTokenID].ChainSpecific)
		require.NoError(t, err)
		fmt.Printf("Receiver USDT token balance before: %+v\n", usdtTokenBalance)

		_, err = apex.SubmitTx(
			ctx, cardanofw.ChainIDNexus, user,
			receiver.GetAddress(cardanofw.ChainIDNexus), big.NewInt(1_000_000),
			[]wallet.TokenAmount{
				{Token: wallet.Token{PolicyID: apex.NexusInfo.Tokens[cardanofw.USDTTokenID].ChainSpecific}, Amount: 2},
			},
			[]byte{})
		require.NoError(t, err)

		balance, err = nexusChain.GetAddressBalance(ctx, receiver.GetAddress(cardanofw.ChainIDNexus))
		require.NoError(t, err)
		fmt.Printf("Receiver token balance after: %+v\n", balance)

		usdtTokenBalance, err = apex.GetBalanceWithTokenName(ctx, receiver, cardanofw.ChainIDNexus, apex.NexusInfo.Tokens[cardanofw.USDTTokenID].ChainSpecific)
		require.NoError(t, err)
		fmt.Printf("Receiver USDT token balance after: %+v\n", usdtTokenBalance)
	})

	t.Run("Cardano -> Nexus - ADA -> xADA", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDCardano, cardanofw.ChainIDNexus, big.NewInt(100_000_000),
			cardanofw.BridgingTypeCurrencyOnSource)
	})
}

func Test_SkylineBridgeCC_General(t *testing.T) {
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
		err := nexusChain.FundUsersWithToken(user.GetAddress(cardanofw.ChainIDNexus), big.NewInt(2), cardanofw.USDTTokenID)
		require.NoError(t, err)

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDNexus, cardanofw.ChainIDVector, big.NewInt(1),
			cardanofw.BridgingTypeColoredCoinOnSource, e2ehelper.WithColoredCoins([]uint16{cardanofw.USDTTokenID}))

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDVector, cardanofw.ChainIDNexus, big.NewInt(1),
			cardanofw.BridgingTypeColoredCoinOnSource, e2ehelper.WithColoredCoins([]uint16{cardanofw.USDTTokenID}))

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDNexus, cardanofw.ChainIDVector, big.NewInt(1),
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

func Test_SkylineBridgeCC_InvalidScenarios_RefundDisabled(t *testing.T) {
	const (
		apiKey  = "test_api_key"
		userCnt = 10

		maxWaitTimeSec = 600
		retryDelaySec  = 5

		minColCoinsAllowedToBridge = uint64(2)
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
			setting := cardanofw.GetMapFromInterfaceKey(mp, "bridgingSettings")
			setting["minColCoinsAllowedToBridge"] = minColCoinsAllowedToBridge
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

	// Fund user with USDT tokens on Nexus for tests
	nexusChain := apex.GetChainMust(t, cardanofw.ChainIDNexus).(*cardanofw.TestEVMChain)
	err := nexusChain.FundUsersWithToken(user.GetAddress(cardanofw.ChainIDNexus), big.NewInt(100), cardanofw.USDTTokenID)
	require.NoError(t, err)

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
			cardanofw.DefaultTokenName, cardanofw.DefaultTokenMintAmount,
			uint64(1_500_000), uint64(1_000_000))
		require.NoError(t, err)

		executeInvalidSendNativeToken(t, ctx, apex, user, vectorTestConfig, *tokensFunded, maxWaitTimeSec, retryDelaySec, false, 0, cardanofw.BridgingTypeColoredCoinOnSource, cardanofw.XADATokenID)
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

	t.Run("12. Vector -> Nexus - Mismatch submitted and receiver amounts - USDT on source", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDNexus, cardanofw.ChainIDVector, big.NewInt(int64(minColCoinsAllowedToBridge)),
			cardanofw.BridgingTypeColoredCoinOnSource, e2ehelper.WithColoredCoins([]uint16{cardanofw.USDTTokenID}))

		executeInvalidColCoin(t, ctx, apex, vectorTestConfig, user, cardanofw.USDTTokenID, maxWaitTimeSec, retryDelaySec, 0,
			colCoinInvalidOpts{
				receivers:  createReceiversColCoin(apex, 1, vectorTestConfig.dstChainID, minColCoinsAllowedToBridge*10, cardanofw.USDTTokenID),
				amount:     minColCoinsAllowedToBridge,
				waitOption: WaitRefundDisabled,
			},
		)
	})

	t.Run("13. Vector -> Nexus - Mismatch submitted and multiple receiver amounts - USDT on source", func(t *testing.T) {
		instances := 3

		for i := range instances {
			e2ehelper.ExecuteSingleBridging(
				t, ctx, apex, user, apex.Users[i], cardanofw.ChainIDNexus, cardanofw.ChainIDVector, big.NewInt(int64(minColCoinsAllowedToBridge)),
				cardanofw.BridgingTypeColoredCoinOnSource, e2ehelper.WithColoredCoins([]uint16{cardanofw.USDTTokenID}))
		}

		executeInvalidMismatchSendColCoinsMultipleInstancesParalel(t, ctx, apex, vectorTestConfig, minColCoinsAllowedToBridge, cardanofw.USDTTokenID, instances, maxWaitTimeSec, retryDelaySec, false, 0)
	})

	t.Run("14. Vector -> Nexus - Invalid destination - USDT on source", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDNexus, cardanofw.ChainIDVector, big.NewInt(int64(minColCoinsAllowedToBridge)),
			cardanofw.BridgingTypeColoredCoinOnSource, e2ehelper.WithColoredCoins([]uint16{cardanofw.USDTTokenID}))

		executeInvalidColCoin(t, ctx, apex, vectorTestConfig, user, cardanofw.USDTTokenID, maxWaitTimeSec, retryDelaySec, 0,
			colCoinInvalidOpts{
				receivers:  createReceiversColCoin(apex, 1, vectorTestConfig.dstChainID, minColCoinsAllowedToBridge, cardanofw.USDTTokenID),
				amount:     minColCoinsAllowedToBridge,
				waitOption: WaitRefundDisabled,
				metadataModifier: func(metadata []byte) []byte {
					return bytes.Replace(metadata, fmt.Appendf(nil, "\"%s\"", vectorTestConfig.dstChainID), []byte("\"unknown\""), 1)
				},
			},
		)
	})

	t.Run("15. Vector -> Nexus - Invalid metadata type - USDT on source", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDNexus, cardanofw.ChainIDVector, big.NewInt(int64(minColCoinsAllowedToBridge)),
			cardanofw.BridgingTypeColoredCoinOnSource, e2ehelper.WithColoredCoins([]uint16{cardanofw.USDTTokenID}))

		executeInvalidColCoin(t, ctx, apex, vectorTestConfig, user, cardanofw.USDTTokenID, 60, retryDelaySec, 0,
			colCoinInvalidOpts{
				receivers:  createReceiversColCoin(apex, 1, vectorTestConfig.dstChainID, minColCoinsAllowedToBridge, cardanofw.USDTTokenID),
				amount:     minColCoinsAllowedToBridge,
				waitOption: WaitTimeoutRefundDisabled,
				metadataModifier: func(metadata []byte) []byte {
					return bytes.Replace(metadata, []byte("bridge"), []byte("xxxxx"), 1)
				},
			},
		)
	})

	t.Run("16. Vector -> Nexus - Invalid receiver amount - USDT on source", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDNexus, cardanofw.ChainIDVector, big.NewInt(int64(minColCoinsAllowedToBridge)),
			cardanofw.BridgingTypeColoredCoinOnSource, e2ehelper.WithColoredCoins([]uint16{cardanofw.USDTTokenID}))

		invalidAmount := minColCoinsAllowedToBridge - 1
		executeInvalidColCoin(t, ctx, apex, vectorTestConfig, user, cardanofw.USDTTokenID, maxWaitTimeSec, retryDelaySec, 0,
			colCoinInvalidOpts{
				receivers:  createReceiversColCoin(apex, 1, vectorTestConfig.dstChainID, invalidAmount, cardanofw.USDTTokenID),
				amount:     invalidAmount,
				waitOption: WaitRefundDisabled,
			},
		)
	})

	t.Run("17. Vector -> Nexus - Invalid receiver address - USDT on source", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDNexus, cardanofw.ChainIDVector, big.NewInt(int64(minColCoinsAllowedToBridge)),
			cardanofw.BridgingTypeColoredCoinOnSource, e2ehelper.WithColoredCoins([]uint16{cardanofw.USDTTokenID}))

		executeInvalidColCoin(t, ctx, apex, vectorTestConfig, user, cardanofw.USDTTokenID, maxWaitTimeSec, retryDelaySec, 0,
			colCoinInvalidOpts{
				receivers: []sendtx.BridgingTxReceiver{
					{
						Addr:    "addr1qxyz...invalidaddress",
						Amount:  minColCoinsAllowedToBridge,
						TokenID: cardanofw.USDTTokenID,
					},
				},
				amount:     minColCoinsAllowedToBridge,
				waitOption: WaitRefundDisabled,
			},
		)
	})
}

func Test_SkylineBridgeCC_InvalidScenarios_NexusSrc(t *testing.T) {
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

	// funding for tests
	nexusChain := apex.GetChainMust(t, cardanofw.ChainIDNexus).(*cardanofw.TestEVMChain)
	err := nexusChain.FundUsersWithToken(user.GetAddress(cardanofw.ChainIDNexus), big.NewInt(1000000000001), cardanofw.USDTTokenID)
	require.NoError(t, err)

	sendAmount := cardanofw.DfmToWei(big.NewInt(1_000_000))

	tokenInfo := apex.GetBridgingTokensInfo(cardanofw.ChainIDNexus, cardanofw.ChainIDVector, cardanofw.BridgingTypeColoredCoinOnSource, cardanofw.USDTTokenID)
	require.NotNil(t, tokenInfo)

	//nolint:dupl
	t.Run("1. Invalid destination in bridging request", func(t *testing.T) {
		t.Run("1. Destination is Nexus", func(t *testing.T) {
			err := executeInvalidNexusBridgingRequest(t, ctx, apex, user, InvalidNexusBridgingRequest{
				dstChainID: cardanofw.ChainIDToInt(cardanofw.ChainIDNexus),
				sender:     user,
				receivers: map[string]cardanofw.ReceiverAmount{
					user.GetAddress(cardanofw.ChainIDVector): {
						TokenID: cardanofw.USDTTokenID,
						Amount:  sendAmount,
					},
				},
				operationFee: big.NewInt(0),
				tokenInfo:    tokenInfo,
			})
			require.NoError(t, err)
		})

		t.Run("2. Destination is unregistered", func(t *testing.T) {
			err := executeInvalidNexusBridgingRequest(t, ctx, apex, user, InvalidNexusBridgingRequest{
				dstChainID: 99,
				sender:     user,
				receivers: map[string]cardanofw.ReceiverAmount{
					user.GetAddress(cardanofw.ChainIDVector): {
						TokenID: cardanofw.USDTTokenID,
						Amount:  sendAmount,
					},
				},
				operationFee: big.NewInt(0),
				tokenInfo:    tokenInfo,
			})
			require.NoError(t, err)
		})
	})

	t.Run("3. Invalid destination in receiver", func(t *testing.T) {
		err := executeInvalidNexusBridgingRequest(t, ctx, apex, user, InvalidNexusBridgingRequest{
			dstChainID: cardanofw.ChainIDToInt(cardanofw.ChainIDVector),
			sender:     user,
			receivers: map[string]cardanofw.ReceiverAmount{
				user.GetAddress(cardanofw.ChainIDNexus): {
					TokenID: cardanofw.USDTTokenID,
					Amount:  sendAmount,
				},
			},
			operationFee: big.NewInt(0),
			tokenInfo:    tokenInfo,
		})
		require.NoError(t, err)
	})

	t.Run("4. 0 receivers in bridging request", func(t *testing.T) {
		err := executeInvalidNexusBridgingRequest(t, ctx, apex, user, InvalidNexusBridgingRequest{
			dstChainID:   cardanofw.ChainIDToInt(cardanofw.ChainIDVector),
			sender:       user,
			operationFee: big.NewInt(0),
			tokenInfo:    tokenInfo,
		})
		require.NoError(t, err)
	})

	t.Run("5. Too many receivers in bridging request", func(t *testing.T) {
		receivers := make(map[string]cardanofw.ReceiverAmount)
		for i := range 6 {
			receivers[apex.Users[i].GetAddress(cardanofw.ChainIDVector)] = cardanofw.ReceiverAmount{
				TokenID: cardanofw.USDTTokenID,
				Amount:  sendAmount,
			}
		}

		err := executeInvalidNexusBridgingRequest(t, ctx, apex, user, InvalidNexusBridgingRequest{
			dstChainID:   cardanofw.ChainIDToInt(cardanofw.ChainIDVector),
			sender:       user,
			receivers:    receivers,
			operationFee: big.NewInt(0),
			tokenInfo:    tokenInfo,
		})
		require.NoError(t, err)
	})

	t.Run("6. Invalid receiver address", func(t *testing.T) {
		err := executeInvalidNexusBridgingRequest(t, ctx, apex, user, InvalidNexusBridgingRequest{
			dstChainID: cardanofw.ChainIDToInt(cardanofw.ChainIDVector),
			sender:     user,
			receivers: map[string]cardanofw.ReceiverAmount{
				"addr_test1invalidaddress": {
					TokenID: cardanofw.USDTTokenID,
					Amount:  sendAmount,
				},
			},
			operationFee: big.NewInt(0),
			tokenInfo:    tokenInfo,
		})
		require.NoError(t, err)
	})

	t.Run("7. Fee address in receivers", func(t *testing.T) {
		err := executeInvalidNexusBridgingRequest(t, ctx, apex, user, InvalidNexusBridgingRequest{
			dstChainID: cardanofw.ChainIDToInt(cardanofw.ChainIDVector),
			sender:     user,
			receivers: map[string]cardanofw.ReceiverAmount{
				apex.VectorInfo.FeeAddr: {
					TokenID: cardanofw.USDTTokenID,
					Amount:  sendAmount,
				},
			},
			operationFee: big.NewInt(0),
			tokenInfo:    tokenInfo,
		})
		require.NoError(t, err)
	})

	t.Run("8. Less than allowed to bridge", func(t *testing.T) {
		err := executeInvalidNexusBridgingRequest(t, ctx, apex, user, InvalidNexusBridgingRequest{
			dstChainID: cardanofw.ChainIDToInt(cardanofw.ChainIDVector),
			sender:     user,
			receivers: map[string]cardanofw.ReceiverAmount{
				user.GetAddress(cardanofw.ChainIDVector): {
					TokenID: cardanofw.USDTTokenID,
					Amount:  big.NewInt(0),
				},
			},
			operationFee: big.NewInt(0),
			tokenInfo:    tokenInfo,
		})
		require.Error(t, err)
		require.ErrorContains(t, err, "transaction receipt status is unsuccessful")
	})

	t.Run("9. Negative amount in receivers", func(t *testing.T) {
		err := executeInvalidNexusBridgingRequest(t, ctx, apex, user, InvalidNexusBridgingRequest{
			dstChainID: cardanofw.ChainIDToInt(cardanofw.ChainIDVector),
			sender:     user,
			receivers: map[string]cardanofw.ReceiverAmount{
				user.GetAddress(cardanofw.ChainIDVector): {
					TokenID: cardanofw.USDTTokenID,
					Amount:  sendAmount,
				},
				apex.Users[1].GetAddress(cardanofw.ChainIDVector): {
					TokenID: cardanofw.USDTTokenID,
					Amount:  big.NewInt(-1),
				},
			},
			operationFee: big.NewInt(0),
			tokenInfo:    tokenInfo,
		})
		require.Error(t, err)
		require.ErrorContains(t, err, "transaction receipt status is unsuccessful")
	})

	t.Run("10. Incorrect token id in receivers", func(t *testing.T) {
		req := InvalidNexusBridgingRequest{
			dstChainID: cardanofw.ChainIDToInt(cardanofw.ChainIDVector),
			sender:     user,
			receivers: map[string]cardanofw.ReceiverAmount{
				user.GetAddress(cardanofw.ChainIDVector): {
					TokenID: 0,
					Amount:  sendAmount,
				},
			},
			operationFee: big.NewInt(0),
			tokenInfo:    tokenInfo,
		}

		err := executeInvalidNexusBridgingRequest(t, ctx, apex, user, req)
		require.Error(t, err)
		require.ErrorContains(t, err, "transaction receipt status is unsuccessful")
	})

	t.Run("11. Over max allowed to bridge", func(t *testing.T) {
		t.Run("1. Nexus -> Vector usdt", func(t *testing.T) {
			err := executeInvalidNexusBridgingRequest(t, ctx, apex, user, InvalidNexusBridgingRequest{
				dstChainID: cardanofw.ChainIDToInt(cardanofw.ChainIDVector),
				sender:     user,
				receivers: map[string]cardanofw.ReceiverAmount{
					user.GetAddress(cardanofw.ChainIDVector): {
						TokenID: cardanofw.USDTTokenID,
						Amount:  cardanofw.DfmToWei(big.NewInt(1000000000001)),
					},
				},
				operationFee: big.NewInt(0),
				tokenInfo:    tokenInfo,
			})
			require.NoError(t, err)
		})

		vectorChain := apex.GetChainMust(t, cardanofw.ChainIDVector).(*cardanofw.TestCardanoChain)
		err = cardanofw.MintToken(vectorChain, apex.VectorInfo.GenesisWallet, cardanofw.XADATokenName, 10000000000010)
		require.NoError(t, err)

		_, err = cardanofw.FundUsersWithToken(
			ctx, vectorChain, apex.VectorInfo.GenesisWallet,
			apex.Users, cardanofw.XADATokenName, 2_000_000, 1000000000001)
		require.NoError(t, err)

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDVector, cardanofw.ChainIDNexus, big.NewInt(500000000001),
			cardanofw.BridgingTypeColoredCoinOnSource, e2ehelper.WithColoredCoins([]uint16{cardanofw.XADATokenID}))

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDVector, cardanofw.ChainIDNexus, big.NewInt(500000000000),
			cardanofw.BridgingTypeColoredCoinOnSource, e2ehelper.WithColoredCoins([]uint16{cardanofw.XADATokenID}))

		t.Run("2. Nexus -> Vector xada", func(t *testing.T) {
			tokenInfo = apex.GetBridgingTokensInfo(cardanofw.ChainIDNexus, cardanofw.ChainIDVector, cardanofw.BridgingTypeColoredCoinOnSource, cardanofw.XADATokenID)
			require.NotNil(t, tokenInfo)

			err = executeInvalidNexusBridgingRequest(t, ctx, apex, user, InvalidNexusBridgingRequest{
				dstChainID: cardanofw.ChainIDToInt(cardanofw.ChainIDVector),
				sender:     user,
				receivers: map[string]cardanofw.ReceiverAmount{
					user.GetAddress(cardanofw.ChainIDVector): {
						TokenID: cardanofw.XADATokenID,
						Amount:  cardanofw.DfmToWei(big.NewInt(1000000000001)),
					},
				},
				operationFee: big.NewInt(0),
				tokenInfo:    tokenInfo,
			})
			require.NoError(t, err)
		})

		t.Run("3. Nexus -> Cardano xada", func(t *testing.T) {
			tokenInfo := apex.GetBridgingTokensInfo(cardanofw.ChainIDNexus, cardanofw.ChainIDCardano, cardanofw.BridgingTypeColoredCoinOnSource, cardanofw.XADATokenID)
			require.NotNil(t, tokenInfo)

			err = executeInvalidNexusBridgingRequest(t, ctx, apex, user, InvalidNexusBridgingRequest{
				dstChainID: cardanofw.ChainIDToInt(cardanofw.ChainIDCardano),
				sender:     user,
				receivers: map[string]cardanofw.ReceiverAmount{
					user.GetAddress(cardanofw.ChainIDCardano): {
						TokenID: cardanofw.XADATokenID,
						Amount:  cardanofw.DfmToWei(big.NewInt(1000000000001)),
					},
				},
				operationFee: big.NewInt(0),
				tokenInfo:    tokenInfo,
			})
			require.NoError(t, err)
		})
	})

	t.Run("12. Insufficient balance", func(t *testing.T) {
		err := executeInvalidNexusBridgingRequest(t, ctx, apex, user, InvalidNexusBridgingRequest{
			dstChainID: cardanofw.ChainIDToInt(cardanofw.ChainIDVector),
			sender:     user,
			receivers: map[string]cardanofw.ReceiverAmount{
				user.GetAddress(cardanofw.ChainIDVector): {
					TokenID: cardanofw.USDTTokenID,
					Amount:  new(big.Int).Mul(sendAmount, big.NewInt(1000000000000000000)),
				},
			},
			operationFee: big.NewInt(0),
			tokenInfo:    tokenInfo,
		})
		require.Error(t, err)
		require.ErrorContains(t, err, "transaction receipt status is unsuccessful")
	})
	/*
		Uncomnent when the operation fee is set to != 0 in settings
		t.Run("13. Wrong operation fee", func(t *testing.T) {
			tokenInfo := apex.GetBridgingTokensInfo(cardanofw.ChainIDNexus, cardanofw.ChainIDVector, cardanofw.BridgingTypeColoredCoinOnSource, cardanofw.USDTTokenID)
			require.NotNil(t, tokenInfo)

			err := executeInvalidNexusBridgingRequest(t, ctx, apex, user, InvalidNexusBridgingRequest{
				dstChainID: cardanofw.ChainIDToInt(cardanofw.ChainIDVector),
				sender:     user,
				receivers: map[string]cardanofw.ReceiverAmount{
					user.GetAddress(cardanofw.ChainIDVector): {
						TokenID: cardanofw.USDTTokenID,
						Amount:  validSendAmount,
					},
				},
				operationFee: new(big.Int).Sub(apex.Config.NexusConfig.MinOperationFee, big.NewInt(1)),
				tokenInfo:    tokenInfo,
			})
			require.Error(t, err)
			require.ErrorContains(t, err, "timeout")
		})
	*/

	t.Run("13. Insufficient fee", func(t *testing.T) {
		tokenInfo := apex.GetBridgingTokensInfo(cardanofw.ChainIDNexus, cardanofw.ChainIDVector, cardanofw.BridgingTypeColoredCoinOnSource, cardanofw.USDTTokenID)
		require.NotNil(t, tokenInfo)

		err := executeInvalidNexusBridgingRequest(t, ctx, apex, user, InvalidNexusBridgingRequest{
			dstChainID: cardanofw.ChainIDToInt(cardanofw.ChainIDVector),
			sender:     user,
			receivers: map[string]cardanofw.ReceiverAmount{
				user.GetAddress(cardanofw.ChainIDVector): {
					TokenID: cardanofw.USDTTokenID,
					Amount:  sendAmount,
				},
			},
			operationFee: big.NewInt(0),
			feeAmount:    big.NewInt(1000000000),
			tokenInfo:    tokenInfo,
		})
		require.Error(t, err)
		require.ErrorContains(t, err, "transaction receipt status is unsuccessful")
	})
}

func Test_SkylineBridgeCC_ValidScenarios(t *testing.T) {
	const (
		apiKey = "test_api_key"
		usrCnt = 15
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
		cardanofw.WithUserCnt(usrCnt),
		cardanofw.WithCardanoConfig(cardanoConfig),
		cardanofw.WithPrimeConfig(primeConfig),
		cardanofw.WithVectorConfig(vectorConfig),
		cardanofw.WithNexusConfig(nexusConfig),
		cardanofw.WithBridgingAddrCnt(cardanofw.ChainIDPrime, bridgeAddrCnt),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	user := apex.Users[usrCnt-1]

	sendAmount := big.NewInt(1_000_000)

	t.Run("Send radom tokens to custodial, relayer and plutus addresses", func(t *testing.T) {
		vectorChain := apex.GetChainMust(t, cardanofw.ChainIDVector)

		addresses := []string{
			vectorChain.GetCustodialAddress(),
			vectorChain.GetRelayerAddress(),
			vectorChain.GetCardanoScriptInfo().PlutusAddress,
		}

		err := cardanofw.MintToken(vectorChain.(*cardanofw.TestCardanoChain), apex.VectorInfo.GenesisWallet, "ranodom-token", 3*1000000000000)
		require.NoError(t, err)

		_, err = cardanofw.FundAddressesWithToken(
			ctx, vectorChain.(*cardanofw.TestCardanoChain), apex.VectorInfo.GenesisWallet,
			addresses, "ranodom-token", 40000000000, 1000000000000)
		require.NoError(t, err)
	})

	t.Run("Bridge to relayer, custodial and plutus addresses", func(t *testing.T) {
		vectorChain := apex.GetChainMust(t, cardanofw.ChainIDVector)
		sendAmountDfm := big.NewInt(5_000_000)

		addresses := []string{
			vectorChain.GetCustodialAddress(),
			vectorChain.GetRelayerAddress(),
			vectorChain.GetCardanoScriptInfo().PlutusAddress,
		}

		for _, addr := range addresses {
			vectorAddr, err := wallet.NewCardanoAddressFromString(addr)
			require.NoError(t, err)

			apexUser := &cardanofw.TestApexUser{
				HasVectorWallet: true,
				VectorAddress:   vectorAddr,
			}

			for range 2 {
				e2ehelper.ExecuteSingleBridging(
					t, ctx, apex, user, apexUser, cardanofw.ChainIDCardano, cardanofw.ChainIDVector, sendAmountDfm,
					cardanofw.BridgingTypeCurrencyOnSource)
			}
		}
	})

	t.Run("1. Cardano -> Vector -> Nexus -> Cardano - ADA/xADA", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDCardano, cardanofw.ChainIDVector, sendAmount,
			cardanofw.BridgingTypeCurrencyOnSource)

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDVector, cardanofw.ChainIDNexus, sendAmount,
			cardanofw.BridgingTypeWrappedTokenOnSource)

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDNexus, cardanofw.ChainIDCardano, sendAmount,
			cardanofw.BridgingTypeColoredCoinOnSource, e2ehelper.WithColoredCoins([]uint16{cardanofw.XADATokenID}))
	})

	t.Run("2. Cardano -> Nexus -> Vector -> Cardano - ADA/xADA", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDCardano, cardanofw.ChainIDNexus, sendAmount,
			cardanofw.BridgingTypeCurrencyOnSource)

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDNexus, cardanofw.ChainIDVector, sendAmount,
			cardanofw.BridgingTypeColoredCoinOnSource, e2ehelper.WithColoredCoins([]uint16{cardanofw.XADATokenID}))

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDVector, cardanofw.ChainIDCardano, sendAmount,
			cardanofw.BridgingTypeWrappedTokenOnSource)
	})

	t.Run("3. Cardano -> Nexus; Cardano -> Vector -> Nexus; Nexus -> Vector - ADA/xADA", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDCardano, cardanofw.ChainIDNexus, sendAmount,
			cardanofw.BridgingTypeCurrencyOnSource)

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDCardano, cardanofw.ChainIDVector, sendAmount,
			cardanofw.BridgingTypeCurrencyOnSource)

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDVector, cardanofw.ChainIDNexus, sendAmount,
			cardanofw.BridgingTypeWrappedTokenOnSource)

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDNexus, cardanofw.ChainIDVector, new(big.Int).Mul(sendAmount, big.NewInt(2)),
			cardanofw.BridgingTypeColoredCoinOnSource, e2ehelper.WithColoredCoins([]uint16{cardanofw.XADATokenID}))
	})

	type bridgingRequest struct {
		src             string
		dest            string
		requestType     cardanofw.BridgingType
		srcMinterWallet *wallet.Wallet
		tokenID         uint16
	}

	var (
		bridgingRequests = []bridgingRequest{
			{src: cardanofw.ChainIDVector, dest: cardanofw.ChainIDNexus, requestType: cardanofw.BridgingTypeColoredCoinOnSource, srcMinterWallet: apex.VectorInfo.GenesisWallet, tokenID: cardanofw.USDTTokenID},
			{src: cardanofw.ChainIDNexus, dest: cardanofw.ChainIDVector, requestType: cardanofw.BridgingTypeColoredCoinOnSource, tokenID: cardanofw.USDTTokenID},
			{src: cardanofw.ChainIDVector, dest: cardanofw.ChainIDNexus, requestType: cardanofw.BridgingTypeWrappedTokenOnSource, srcMinterWallet: apex.VectorInfo.GenesisWallet, tokenID: cardanofw.XADATokenID},
			{src: cardanofw.ChainIDNexus, dest: cardanofw.ChainIDVector, requestType: cardanofw.BridgingTypeColoredCoinOnSource, tokenID: cardanofw.XADATokenID},
			{src: cardanofw.ChainIDCardano, dest: cardanofw.ChainIDVector, requestType: cardanofw.BridgingTypeCurrencyOnSource, srcMinterWallet: apex.CardanoInfo.GenesisWallet, tokenID: cardanofw.ADATokenID},
			{src: cardanofw.ChainIDVector, dest: cardanofw.ChainIDCardano, requestType: cardanofw.BridgingTypeWrappedTokenOnSource, srcMinterWallet: apex.VectorInfo.GenesisWallet, tokenID: cardanofw.XADATokenID},
		}
	)

	getBridgingRequests := func(bridgingRequests []bridgingRequest) []e2ehelper.BridgingDirectionConfig {
		res := make([]e2ehelper.BridgingDirectionConfig, len(bridgingRequests))
		for i, br := range bridgingRequests {
			res[i] = e2ehelper.BridgingDirectionConfig{
				SrcChain:     br.src,
				DstChain:     br.dest,
				BridgingType: br.requestType,
				TokenID:      br.tokenID,
			}
		}

		return res
	}

	// This test is required because we cannot fund vector users with USDT tokens
	// nor nexus users with xADA tokens
	// and it is requirement for the tests that test bridging Vector -> Nexus with USDT tokens
	// and Nexus -> Vector with xADA tokens
	fundingPassed := t.Run("4. Nexus -> Vector USDT, Vector -> Nexus xADA funding test", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() {
			t.Skip()
		}

		// end goal for every user:
		// - 100_000_000 USDT and 200_000_000 xADA on Nexus and Vector
		// this way we don't need to fund users with tokens in every following test

		nexusChain := apex.GetChainMust(t, cardanofw.ChainIDNexus).(*cardanofw.TestEVMChain)
		vectorChain := apex.GetChainMust(t, cardanofw.ChainIDVector).(*cardanofw.TestCardanoChain)

		var wg sync.WaitGroup

		err := cardanofw.MintToken(vectorChain, apex.VectorInfo.GenesisWallet, cardanofw.XADATokenName, uint64(len(apex.Users)*400_000_000))
		require.NoError(t, err)

		_, err = cardanofw.FundUsersWithToken(
			ctx, vectorChain, apex.VectorInfo.GenesisWallet,
			apex.Users, cardanofw.XADATokenName, 400_000_000, 400_000_000)
		require.NoError(t, err)

		for _, user := range apex.Users {
			err := nexusChain.FundUsersWithToken(
				user.GetAddress(cardanofw.ChainIDNexus),
				big.NewInt(200_000_000),
				cardanofw.USDTTokenID,
			)
			require.NoError(t, err)

			wg.Add(2)

			go func() {
				defer wg.Done()

				e2ehelper.ExecuteSingleBridging(
					t, ctx, apex, user, user, cardanofw.ChainIDNexus, cardanofw.ChainIDVector, big.NewInt(100_000_000),
					cardanofw.BridgingTypeColoredCoinOnSource, e2ehelper.WithColoredCoins([]uint16{cardanofw.USDTTokenID}))
			}()

			go func() {
				defer wg.Done()

				e2ehelper.ExecuteSingleBridging(
					t, ctx, apex, user, user, cardanofw.ChainIDVector, cardanofw.ChainIDNexus, big.NewInt(200_000_000),
					cardanofw.BridgingTypeWrappedTokenOnSource, e2ehelper.WithColoredCoins([]uint16{cardanofw.XADATokenID}))
			}()
		}

		wg.Wait()
	})

	t.Run("5. Parallel bridging tests", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() || !fundingPassed {
			t.Skip()
		}

		const (
			sendAmount = uint64(1_000_000)
			instances  = 5
		)

		for idx, br := range bridgingRequests {
			fmt.Printf("5.%d %s -> %s - %s\n", idx+1, br.src, br.dest, br.requestType)

			e2ehelper.ExecuteBridging(
				t, ctx, apex, 1, apex.Users[:instances], []*cardanofw.TestApexUser{user},
				[]string{br.src},
				map[string][]string{
					br.src: {br.dest},
				},
				map[e2ehelper.SrcDstChainPair]cardanofw.BridgingType{
					e2ehelper.NewChainPair(br.src, br.dest): br.requestType,
				},
				new(big.Int).SetUint64(sendAmount), e2ehelper.WithColoredCoins([]uint16{br.tokenID}))
		}
	})

	t.Run("6. Sequential bridging tests", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() || !fundingPassed {
			t.Skip()
		}

		const (
			sendAmount          = uint64(1_000_000)
			sequentialInstances = 5
			parallelInstances   = 10
			receivers           = 1
		)

		for idx, br := range bridgingRequests {
			fmt.Printf("6.%d %s -> %s - %s\n", idx+1, br.src, br.dest, br.requestType)

			e2ehelper.ExecuteBridging(
				t, ctx, apex, sequentialInstances,
				apex.Users[:sequentialInstances],
				apex.Users[:receivers],
				[]string{br.src},
				map[string][]string{
					br.src: {br.dest},
				},
				map[e2ehelper.SrcDstChainPair]cardanofw.BridgingType{
					e2ehelper.NewChainPair(br.src, br.dest): br.requestType,
				},
				new(big.Int).SetUint64(sendAmount), e2ehelper.WithColoredCoins([]uint16{br.tokenID}),
			)
		}
	})

	t.Run("7. All directions parallel", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() || !fundingPassed {
			t.Skip()
		}

		const (
			sendAmount = uint64(1_000_000)
			instances  = 5
		)

		e2ehelper.ExecuteBridgingExtended(
			t, ctx, apex, 1,
			apex.Users[:instances],
			[]*cardanofw.TestApexUser{user},
			getBridgingRequests(bridgingRequests),
			new(big.Int).SetUint64(sendAmount),
		)
	})

	t.Run("8. All directions sequential and parallel", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() || !fundingPassed {
			t.Skip()
		}

		const (
			sendAmount = uint64(1_000_000)
			instances  = 5
		)

		e2ehelper.ExecuteBridgingExtended(
			t, ctx, apex, instances,
			apex.Users[:instances],
			[]*cardanofw.TestApexUser{user},
			getBridgingRequests(bridgingRequests),
			new(big.Int).SetUint64(sendAmount),
		)
	})
}

func Test_SkylineBridgeCC_WithRefund(t *testing.T) {
	const (
		apiKey  = "test_api_key"
		userCnt = 10

		maxWaitTimeSec = 600
		retryDelaySec  = 5

		colCoinsAmount = uint64(2)
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
		cardanofw.WithBridgingAddrCnt(cardanofw.ChainIDPrime, bridgeAddrCnt),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	user := apex.Users[0]

	tokens := fundTestUsersWithToken(
		t, ctx, apex, []*testConfig{
			{
				srcChainID:      cardanofw.ChainIDVector,
				srcMinterWallet: apex.VectorInfo.GenesisWallet,
			},
			{
				srcChainID:      cardanofw.ChainIDCardano,
				srcMinterWallet: apex.CardanoInfo.GenesisWallet,
			},
		}, apex.Users[:userCnt], uint64(10_000_000), cardanofw.DefaultTokenMintAmount)
	vectorToken, cardanoToken := tokens[0], tokens[1]

	cardanoNexusTestConfig := newTestConfig(
		t, apex.Config.CardanoConfig, &apex.CardanoInfo, cardanofw.ChainIDNexus, cardanoToken.TokenName())
	vectorNexusTestConfig := newTestConfig(
		t, apex.Config.VectorConfig, &apex.VectorInfo, cardanofw.ChainIDNexus, vectorToken.TokenName())
	vectorCardanoTestConfig := newTestConfig(
		t, apex.Config.VectorConfig, &apex.VectorInfo, cardanofw.ChainIDCardano, vectorToken.TokenName())

	// Fund user on Nexus with USDT token
	nexusChain := apex.GetChainMust(t, cardanofw.ChainIDNexus).(*cardanofw.TestEVMChain)
	err := nexusChain.FundUsersWithToken(user.GetAddress(cardanofw.ChainIDNexus), big.NewInt(100), cardanofw.USDTTokenID)
	require.NoError(t, err)

	fmt.Printf("User: %+v\n", user.GetAddress(cardanofw.ChainIDNexus))

	refundTrigger := func(t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, srcChain, dstChain string,
		user *cardanofw.TestApexUser, tokenID uint16, bridgingType cardanofw.BridgingType) {
		t.Helper()

		var testConfig *testConfig

		switch {
		case srcChain == cardanofw.ChainIDVector && dstChain == cardanofw.ChainIDNexus:
			testConfig = vectorNexusTestConfig
		case srcChain == cardanofw.ChainIDCardano && dstChain == cardanofw.ChainIDNexus:
			testConfig = cardanoNexusTestConfig
		case srcChain == cardanofw.ChainIDVector && dstChain == cardanofw.ChainIDCardano:
			testConfig = vectorCardanoTestConfig
		default:
			require.Fail(t, "unsupported chain combination: src=%s dst=%s", srcChain, dstChain)
		}

		switch bridgingType {
		case cardanofw.BridgingTypeColoredCoinOnSource:
			executeInvalidColCoin(t, ctx, apex, testConfig, user, tokenID, maxWaitTimeSec, retryDelaySec, 0,
				colCoinInvalidOpts{
					receivers:  createReceiversColCoin(apex, 1, testConfig.dstChainID, colCoinsAmount*10, tokenID),
					amount:     colCoinsAmount,
					waitOption: NoWait,
				},
			)
		case cardanofw.BridgingTypeCurrencyOnSource:
			executeInvalidMismatchSendLovelaceAmount(t, ctx, apex, testConfig, user, maxWaitTimeSec, retryDelaySec, cardanofw.BridgingTypeCurrencyOnSource, true, 0)
		case cardanofw.BridgingTypeWrappedTokenOnSource:
			executeInvalidEmptyReceivers(t, ctx, apex, testConfig, user, maxWaitTimeSec, retryDelaySec, cardanofw.BridgingTypeWrappedTokenOnSource, true, 0)
		}
	}

	t.Run("1. Bridging xADA from Nexus to Cardano (ADA), Refund ADA on Cardano", func(t *testing.T) {
		// mint some xADA on Nexus for user
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDCardano, cardanofw.ChainIDNexus, big.NewInt(100_000_000),
			cardanofw.BridgingTypeCurrencyOnSource)

		e2ehelper.ExecuteBridgingWithRefund(
			t, ctx, apex, user, user, cardanofw.ChainIDNexus, cardanofw.ChainIDCardano, big.NewInt(10_000_000),
			cardanofw.BridgingTypeColoredCoinOnSource, cardanofw.BridgingTypeCurrencyOnSource, refundTrigger, e2ehelper.WithColoredCoins([]uint16{cardanofw.XADATokenID}))
	})

	t.Run("2. Bridging USDT from Nexus to Vector (USDT), Refund USDT on Vector", func(t *testing.T) {
		// mint some USDT on Nexus for user
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDNexus, cardanofw.ChainIDVector, big.NewInt(int64(10*colCoinsAmount)),
			cardanofw.BridgingTypeColoredCoinOnSource, e2ehelper.WithColoredCoins([]uint16{cardanofw.USDTTokenID}))

		e2ehelper.ExecuteBridgingWithRefund(
			t, ctx, apex, user, user, cardanofw.ChainIDNexus, cardanofw.ChainIDVector, big.NewInt(int64(colCoinsAmount)),
			cardanofw.BridgingTypeColoredCoinOnSource, cardanofw.BridgingTypeColoredCoinOnSource, refundTrigger, e2ehelper.WithColoredCoins([]uint16{cardanofw.USDTTokenID}))
	})

	t.Run("3. Bridging ADA from Cardano to Vector (xADA), Refund xADA on Vector", func(t *testing.T) {
		e2ehelper.ExecuteBridgingWithRefund(
			t, ctx, apex, user, user, cardanofw.ChainIDCardano, cardanofw.ChainIDVector, big.NewInt(10_000_000),
			cardanofw.BridgingTypeCurrencyOnSource, cardanofw.BridgingTypeWrappedTokenOnSource, refundTrigger, e2ehelper.WithColoredCoins([]uint16{cardanofw.XADATokenID}))
	})
}
