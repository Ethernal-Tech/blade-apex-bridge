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
		err := nexusChain.FundUsersWithToken(user.GetAddress(cardanofw.ChainIDNexus), cardanofw.DfmToWei(big.NewInt(2)), cardanofw.USDTTokenID)
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
			receiver.GetAddress(cardanofw.ChainIDNexus), cardanofw.ApexToWei(big.NewInt(1)),
			[]cardanofw.GenericTokenAmount{cardanofw.NewGenericTokenAmount(
				wallet.Token{PolicyID: apex.NexusInfo.Tokens[cardanofw.USDTTokenID].ChainSpecific}, cardanofw.DfmToWei(big.NewInt(2))),
			}, []byte{})
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
			t, ctx, apex, user, user, cardanofw.ChainIDCardano, cardanofw.ChainIDNexus, cardanofw.ApexToWei(big.NewInt(100)),
			cardanofw.ADATokenID)
	})
}

func Test_SkylineBridgeCC_General(t *testing.T) {
	if cardanofw.ShouldSkipE2RRedundantTests() {
		t.Skip()
	}

	const apiKey = "test_api_key"

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	primeConfig, cardanoConfig := cardanofw.NewPrimeChainConfig(), cardanofw.NewCardanoChainConfig(true)
	vectorConfig := cardanofw.NewVectorChainConfig(map[uint16]string{cardanofw.USDTTokenID: cardanofw.USDTTokenName})
	nexusConfig := cardanofw.NewNexusChainConfig(true)
	polygonConfig := cardanofw.NewPolygonChainConfig(true)

	cardanoConfig.FundTokenAmount = 1_000_000_000
	vectorConfig.FundTokenAmount = 1_000_000_000

	apex := cardanofw.SetupAndRunSkylineBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithCardanoConfig(cardanoConfig),
		cardanofw.WithPrimeConfig(primeConfig),
		cardanofw.WithVectorConfig(vectorConfig),
		cardanofw.WithNexusConfig(nexusConfig),
		cardanofw.WithPolygonConfig(polygonConfig),
		cardanofw.WithBridgingAddrCnt(cardanofw.ChainIDPrime, bridgeAddrCnt),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	user := apex.Users[0]

	fmt.Printf("User: %+v\n", user.GetAddress(cardanofw.ChainIDNexus))

	t.Run("Nexus <-> Vector USTD <-> wUSDT", func(t *testing.T) {
		nexusChain := apex.GetChainMust(t, cardanofw.ChainIDNexus).(*cardanofw.TestEVMChain)
		err := nexusChain.FundUsersWithToken(user.GetAddress(cardanofw.ChainIDNexus), cardanofw.DfmToWei(big.NewInt(2)), cardanofw.USDTTokenID)
		require.NoError(t, err)

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDNexus, cardanofw.ChainIDVector, cardanofw.DfmToWei(big.NewInt(1)),
			cardanofw.USDTTokenID)

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDVector, cardanofw.ChainIDNexus, cardanofw.DfmToWei(big.NewInt(1)),
			cardanofw.USDTTokenID)

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDNexus, cardanofw.ChainIDVector, cardanofw.DfmToWei(big.NewInt(1)),
			cardanofw.USDTTokenID)
	})

	t.Run("Cardano -> Nexus - ADA -> xADA", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDCardano, cardanofw.ChainIDNexus, cardanofw.DfmToWei(big.NewInt(100_000_000)),
			cardanofw.ADATokenID)
	})

	t.Run("Nexus -> Cardano - xADA -> ADA", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDNexus, cardanofw.ChainIDCardano, cardanofw.DfmToWei(big.NewInt(10_000_000)),
			cardanofw.XADATokenID)
	})

	t.Run("Cardano -> Vector - ADA -> xADA", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDCardano, cardanofw.ChainIDVector, cardanofw.DfmToWei(big.NewInt(20_000_000)),
			cardanofw.ADATokenID)
	})

	t.Run("Vector -> Cardano - xADA -> ADA", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDVector, cardanofw.ChainIDCardano, cardanofw.DfmToWei(big.NewInt(10_000_000)),
			cardanofw.XADATokenID)
	})

	t.Run("Vector -> Nexus xADA -> xADA", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDVector, cardanofw.ChainIDNexus, cardanofw.DfmToWei(big.NewInt(1)),
			cardanofw.XADATokenID)
	})

	t.Run("Nexus -> Vector xADA -> xADA", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDNexus, cardanofw.ChainIDVector, cardanofw.DfmToWei(big.NewInt(1)),
			cardanofw.XADATokenID)
	})

	t.Run("Prime -> Cardano - AP3X -> CAP3X", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDPrime, cardanofw.ChainIDCardano, cardanofw.DfmToWei(big.NewInt(10_000_000)),
			cardanofw.AP3XTokenID)
	})

	t.Run("Cardano -> Prime - CAP3X -> AP3X", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDCardano, cardanofw.ChainIDPrime, cardanofw.DfmToWei(big.NewInt(10_000_000)),
			cardanofw.CAP3XTokenID)
	})

	t.Run("Nexus <-> Polygon USDT <-> wUSDT", func(t *testing.T) {
		nexusChain := apex.GetChainMust(t, cardanofw.ChainIDNexus).(*cardanofw.TestEVMChain)
		err := nexusChain.FundUsersWithToken(user.GetAddress(cardanofw.ChainIDNexus), cardanofw.DfmToWei(big.NewInt(2)), cardanofw.USDTTokenID)
		require.NoError(t, err)

		fmt.Printf("Starting bridging USDT Nexus -> Polygon\n")

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDNexus, cardanofw.ChainIDPolygon, cardanofw.DfmToWei(big.NewInt(1)),
			cardanofw.USDTTokenID)

		fmt.Printf("Starting bridging USDT Polygon -> Nexus\n")

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDPolygon, cardanofw.ChainIDNexus, cardanofw.DfmToWei(big.NewInt(1)),
			cardanofw.USDTTokenID)
	})

	t.Run("Polygon <-> Nexus USDC <-> wUSDC", func(t *testing.T) {
		polygonChain := apex.GetChainMust(t, cardanofw.ChainIDPolygon).(*cardanofw.TestEVMChain)
		err := polygonChain.FundUsersWithToken(user.GetAddress(cardanofw.ChainIDPolygon), cardanofw.DfmToWei(big.NewInt(2)), cardanofw.USDCTokenID)
		require.NoError(t, err)

		fmt.Printf("Starting bridging USDC Polygon -> Nexus\n")

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDPolygon, cardanofw.ChainIDNexus, cardanofw.DfmToWei(big.NewInt(1)),
			cardanofw.USDCTokenID)

		fmt.Printf("Starting bridging USDC Nexus -> Polygon\n")
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDNexus, cardanofw.ChainIDPolygon, cardanofw.DfmToWei(big.NewInt(1)),
			cardanofw.USDCTokenID)
	})

	t.Run("Polygon <-> Nexus MATIC <-> xMATIC", func(t *testing.T) {
		fmt.Printf("Starting bridging MATIC Polygon -> Nexus\n")

		sendAmount := cardanofw.ApexToWei(big.NewInt(1))

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDPolygon, cardanofw.ChainIDNexus, sendAmount,
			cardanofw.MATICTokenID)

		fmt.Printf("Starting bridging xMATIC Nexus -> Polygon\n")

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDNexus, cardanofw.ChainIDPolygon, sendAmount,
			cardanofw.XMATICTokenID)
	})
}

func Test_SkylineBridgeCC_InvalidScenarios_RefundDisabled(t *testing.T) {
	const (
		apiKey  = "test_api_key"
		userCnt = 10

		maxWaitTimeSec = 600
		retryDelaySec  = 5
	)

	var lock sync.Mutex

	minColCoinsAllowedToBridgeDfm := uint64(2)
	minColCoinsAllowedToBridge := cardanofw.DfmToWei(new(big.Int).SetUint64(minColCoinsAllowedToBridgeDfm))

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
			t.Helper()

			lock.Lock()
			defer lock.Unlock()

			mp["refundEnabled"] = false

			vcCfg := cardanofw.GetMapFromInterfaceKey(mp, "cardanoChains", cardanofw.ChainIDVector)
			vcCfg["minColCoinsAllowedToBridge"] = minColCoinsAllowedToBridgeDfm
		}, nil, nil, nil),
		cardanofw.WithBridgingAddrCnt(cardanofw.ChainIDPrime, bridgeAddrCnt),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	user := apex.Users[0]

	fmt.Printf("User: %+v\n", user.GetAddress(cardanofw.ChainIDNexus))

	cardanoNexusTestConfig := newTestConfig(
		t, apex, apex.Config.CardanoConfig, &apex.CardanoInfo, cardanofw.ChainIDNexus, cardanofw.ADATokenID)
	vectorNexusXADATestConfig := newTestConfig(
		t, apex, apex.Config.VectorConfig, &apex.VectorInfo, cardanofw.ChainIDNexus, cardanofw.XADATokenID)
	vectorNexusUSDTTestConfig := newTestConfig(
		t, apex, apex.Config.VectorConfig, &apex.VectorInfo, cardanofw.ChainIDNexus, cardanofw.USDTTokenID)

	// Fund user with USDT tokens on Nexus for tests
	nexusChain := apex.GetChainMust(t, cardanofw.ChainIDNexus).(*cardanofw.TestEVMChain)
	err := nexusChain.FundUsersWithToken(user.GetAddress(cardanofw.ChainIDNexus), cardanofw.DfmToWei(big.NewInt(100)), cardanofw.USDTTokenID)
	require.NoError(t, err)

	t.Run("1. Cardano -> Nexus - Mismatch submitted and receiver amounts", func(t *testing.T) {
		executeInvalidMismatchSendLovelaceAmount(t, ctx, apex, cardanoNexusTestConfig, user, maxWaitTimeSec, retryDelaySec, false, 0)
	})

	t.Run("2. Cardano -> Nexus - Multiple submitters mismatch submitted and receiver amounts", func(t *testing.T) {
		executeInvalidMismatchSendAmountMultipleInstances(t, ctx, apex, cardanoNexusTestConfig, maxWaitTimeSec, retryDelaySec, false, 0)
	})

	t.Run("3. Invalid receiver Vector -> Nexus - currency on src", func(t *testing.T) {
		_, err := cardanofw.FundUserWithToken(
			ctx, apex, cardanofw.ChainIDVector,
			apex.VectorInfo.GenesisWallet, user,
			cardanofw.XADATokenName, cardanofw.DefaultTokenMintAmount,
			cardanofw.ApexToWei(big.NewInt(10)), cardanofw.DfmToWei(big.NewInt(1_123_000)))
		require.NoError(t, err)

		executeInvalidTokenDirection(t, ctx, apex, vectorNexusXADATestConfig, cardanofw.AP3XTokenID, user, maxWaitTimeSec, retryDelaySec, false, 0)
	})

	//nolint:dupl
	t.Run("4.Submitted invalid metadata - currency under min - token on source", func(t *testing.T) {
		sendAmount := cardanofw.ApexToWei(big.NewInt(1))

		user, err := cardanofw.NewTestApexUser(cardanofw.NewApexNetworkTypesFromSystem(apex))
		require.NoError(t, err)

		tokensFunded, err := cardanofw.FundUserWithToken( // this is necessary to fix
			ctx, apex, cardanofw.ChainIDVector,
			apex.VectorInfo.GenesisWallet, user,
			cardanofw.XADATokenName, cardanofw.DefaultTokenMintAmount,
			cardanofw.ApexToWei(big.NewInt(10)), cardanofw.ApexToWei(big.NewInt(10)))
		require.NoError(t, err)

		receivers := []sendtx.BridgingTxReceiver{
			{
				Addr:    user.GetAddress(cardanofw.ChainIDNexus),
				Amount:  cardanofw.WeiToDfm(sendAmount).Uint64(),
				TokenID: cardanofw.XADATokenID,
			},
		}

		operationFee := apex.GetMinOperationFee(cardanofw.ChainIDVector)

		feeAmount, err := apex.GetChainMust(t, cardanofw.ChainIDVector).GetBridgingFee(
			ctx, cardanofw.ChainIDNexus, receivers, apex.GetMinBridgingFee(cardanofw.ChainIDVector, true),
			operationFee, apex.VectorInfo.MultisigAddr[0])
		require.NoError(t, err)

		feeAmount.Sub(feeAmount, cardanofw.ApexToWei(big.NewInt(1)))

		metadata, err := apex.GetChainMust(t, cardanofw.ChainIDVector).CreateMetadata(
			user.GetAddress(cardanofw.ChainIDVector), cardanofw.ChainIDNexus,
			receivers, feeAmount,
			operationFee)
		require.NoError(t, err)

		totalValue := new(big.Int).Add(sendAmount, feeAmount)
		totalValue.Add(totalValue, operationFee)

		txHash, err := apex.SubmitTx(
			ctx, cardanofw.ChainIDVector, user,
			apex.VectorInfo.MultisigAddr[0], totalValue,
			[]cardanofw.GenericTokenAmount{cardanofw.NewGenericTokenAmount(tokensFunded.Token, sendAmount)}, metadata)
		require.NoError(t, err)

		cardanofw.WaitForInvalidState(t, ctx, apex, cardanofw.ChainIDVector, txHash, apex.Config.APIKey, 0)
	})

	t.Run("5. Cardano -> Nexus - Submitted invalid metadata - wrong type", func(t *testing.T) {
		executeInvalidMetadataType(t, ctx, apex, cardanoNexusTestConfig, user, 60, retryDelaySec, false, 0)
	})

	t.Run("6. Cardano -> Nexus - Submitted invalid metadata - invalid destination", func(t *testing.T) {
		executeInvalidDestination(t, ctx, apex, cardanoNexusTestConfig, user, maxWaitTimeSec, retryDelaySec, false, 0)
	})

	t.Run("7. Cardano -> Nexus - Submitted invalid metadata - invalid sender", func(t *testing.T) {
		executeInvalidMetadataInvalidSender(t, ctx, apex, cardanoNexusTestConfig, user, maxWaitTimeSec, 0)
	})

	t.Run("8. Cardano -> Nexus - Submitted invalid metadata - invalid bridging fee", func(t *testing.T) {
		executeInvalidBridgingFee(t, ctx, apex, cardanoNexusTestConfig, maxWaitTimeSec, retryDelaySec, false, 0)
	})

	t.Run("9. Cardano -> Nexus - Submitted invalid metadata - empty receivers", func(t *testing.T) {
		executeInvalidEmptyReceivers(t, ctx, apex, cardanoNexusTestConfig, user, maxWaitTimeSec, retryDelaySec, false, 0)
	})

	t.Run("10. Vector -> Nexus - Submitted with unknown tokens to bridging addr", func(t *testing.T) {
		user := apex.Users[userCnt-1]
		minterWallet, _ := user.GetCardanoWallet(cardanofw.ChainIDVector)

		tokensFunded, err := cardanofw.FundUserWithToken(
			ctx, apex, cardanofw.ChainIDVector,
			minterWallet, user,
			cardanofw.DefaultTokenName, cardanofw.DefaultTokenMintAmount,
			cardanofw.DfmToWei(big.NewInt(1_500_000)), cardanofw.ApexToWei(big.NewInt(1)))
		require.NoError(t, err)

		executeInvalidSendNativeToken(t, ctx, apex, user, vectorNexusXADATestConfig, *tokensFunded, maxWaitTimeSec, retryDelaySec, false, 0)
	})

	t.Run("11. Vector -> Nexus - Submitted invalid metadata - invalid send amount - token on source", func(t *testing.T) {
		user, err := cardanofw.NewTestApexUser(cardanofw.NewApexNetworkTypesFromSystem(apex))
		require.NoError(t, err)

		tokensFunded, err := cardanofw.FundUserWithToken(
			ctx, apex, cardanofw.ChainIDVector,
			apex.VectorInfo.GenesisWallet, user,
			cardanofw.XADATokenName, cardanofw.DefaultTokenMintAmount,
			cardanofw.ApexToWei(big.NewInt(10)), cardanofw.DfmToWei(big.NewInt(1_123_000)))
		require.NoError(t, err)

		executeInvalidMismatchSendNativeTokenAmount(t, ctx, apex, user, vectorNexusXADATestConfig, *tokensFunded, maxWaitTimeSec, retryDelaySec, false, 0)
	})

	t.Run("12. Vector -> Nexus - Mismatch submitted and receiver amounts - USDT on source", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDNexus, cardanofw.ChainIDVector, minColCoinsAllowedToBridge,
			cardanofw.USDTTokenID)

		executeInvalidColCoin(t, ctx, apex, vectorNexusUSDTTestConfig, user, maxWaitTimeSec, retryDelaySec, 0,
			colCoinInvalidOpts{
				receivers:  createReceivers(apex, 1, vectorNexusUSDTTestConfig.dstChainID, new(big.Int).Mul(minColCoinsAllowedToBridge, big.NewInt(10)), cardanofw.USDTTokenID),
				amount:     minColCoinsAllowedToBridge,
				waitOption: WaitRefundDisabled,
			},
		)
	})

	t.Run("13. Vector -> Nexus - Mismatch submitted and multiple receiver amounts - USDT on source", func(t *testing.T) {
		instances := 3

		for i := range instances {
			e2ehelper.ExecuteSingleBridging(
				t, ctx, apex, user, apex.Users[i], cardanofw.ChainIDNexus, cardanofw.ChainIDVector, minColCoinsAllowedToBridge,
				cardanofw.USDTTokenID)
		}

		executeInvalidMismatchSendColCoinsMultipleInstancesParalel(t, ctx, apex, vectorNexusUSDTTestConfig, minColCoinsAllowedToBridge, instances, maxWaitTimeSec, retryDelaySec, false, 0)
	})

	t.Run("14. Vector -> Nexus - Invalid destination - USDT on source", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDNexus, cardanofw.ChainIDVector, minColCoinsAllowedToBridge,
			cardanofw.USDTTokenID)

		executeInvalidColCoin(t, ctx, apex, vectorNexusUSDTTestConfig, user, maxWaitTimeSec, retryDelaySec, 0,
			colCoinInvalidOpts{
				receivers:  createReceivers(apex, 1, vectorNexusUSDTTestConfig.dstChainID, minColCoinsAllowedToBridge, cardanofw.USDTTokenID),
				amount:     minColCoinsAllowedToBridge,
				waitOption: WaitRefundDisabled,
				metadataModifier: func(metadata []byte) []byte {
					return bytes.Replace(metadata, fmt.Appendf(nil, "\"%s\"", vectorNexusUSDTTestConfig.dstChainID), []byte("\"unknown\""), 1)
				},
			},
		)
	})

	t.Run("15. Vector -> Nexus - Invalid metadata type - USDT on source", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDNexus, cardanofw.ChainIDVector, minColCoinsAllowedToBridge,
			cardanofw.USDTTokenID)

		executeInvalidColCoin(t, ctx, apex, vectorNexusUSDTTestConfig, user, 60, retryDelaySec, 0,
			colCoinInvalidOpts{
				receivers:  createReceivers(apex, 1, vectorNexusUSDTTestConfig.dstChainID, minColCoinsAllowedToBridge, cardanofw.USDTTokenID),
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
			t, ctx, apex, user, user, cardanofw.ChainIDNexus, cardanofw.ChainIDVector, minColCoinsAllowedToBridge,
			cardanofw.USDTTokenID)

		invalidAmount := new(big.Int).Sub(minColCoinsAllowedToBridge, big.NewInt(1))
		executeInvalidColCoin(t, ctx, apex, vectorNexusUSDTTestConfig, user, maxWaitTimeSec, retryDelaySec, 0,
			colCoinInvalidOpts{
				receivers:  createReceivers(apex, 1, vectorNexusUSDTTestConfig.dstChainID, invalidAmount, cardanofw.USDTTokenID),
				amount:     invalidAmount,
				waitOption: WaitRefundDisabled,
			},
		)
	})

	t.Run("17. Vector -> Nexus - Invalid receiver address - USDT on source", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDNexus, cardanofw.ChainIDVector, minColCoinsAllowedToBridge,
			cardanofw.USDTTokenID)

		executeInvalidColCoin(t, ctx, apex, vectorNexusUSDTTestConfig, user, maxWaitTimeSec, retryDelaySec, 0,
			colCoinInvalidOpts{
				receivers: []sendtx.BridgingTxReceiver{
					{
						Addr:    "addr1qxyz...invalidaddress",
						Amount:  cardanofw.WeiToDfm(minColCoinsAllowedToBridge).Uint64(),
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
	const (
		apiKey                     = "test_api_key"
		minColCoinsAllowedToBridge = uint64(2)
	)

	var lock sync.Mutex

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	primeConfig, cardanoConfig := cardanofw.NewPrimeChainConfig(), cardanofw.NewCardanoChainConfig(true)
	vectorConfig := cardanofw.NewVectorChainConfig(map[uint16]string{cardanofw.USDTTokenID: cardanofw.USDTTokenName})
	nexusConfig := cardanofw.NewNexusChainConfig(true)
	polygonConfig := cardanofw.NewPolygonChainConfig(true)
	cardanoConfig.FundTokenAmount = 1_000_000_000
	vectorConfig.FundTokenAmount = 1_000_000_000

	apex := cardanofw.SetupAndRunSkylineBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithCardanoConfig(cardanoConfig),
		cardanofw.WithPrimeConfig(primeConfig),
		cardanofw.WithVectorConfig(vectorConfig),
		cardanofw.WithNexusConfig(nexusConfig),
		cardanofw.WithPolygonConfig(polygonConfig),
		cardanofw.WithBridgingAddrCnt(cardanofw.ChainIDPrime, bridgeAddrCnt),
		cardanofw.WithCustomConfigHandlers(func(_ *cardanofw.ApexSystem, mp map[string]interface{}) {
			t.Helper()

			lock.Lock()
			defer lock.Unlock()

			nexusCfg := cardanofw.GetMapFromInterfaceKey(mp, "ethChains", cardanofw.ChainIDNexus)
			nexusCfg["minColCoinsAllowedToBridge"] = cardanofw.DfmToWei(big.NewInt(int64(minColCoinsAllowedToBridge)))
		}, nil, nil, nil),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	user := apex.Users[0]

	// funding for tests
	nexusChain := apex.GetChainMust(t, cardanofw.ChainIDNexus).(*cardanofw.TestEVMChain)
	err := nexusChain.FundUsersWithToken(user.GetAddress(cardanofw.ChainIDNexus), cardanofw.DfmToWei(big.NewInt(1000000000001)), cardanofw.USDTTokenID)
	require.NoError(t, err)

	sendAmount := cardanofw.DfmToWei(big.NewInt(1_000_000))

	tokenInfo, err := apex.GetBridgingTokensInfo(cardanofw.ChainIDNexus, cardanofw.ChainIDVector, cardanofw.USDTTokenID)
	require.NoError(t, err)

	//nolint:dupl
	t.Run("1. Invalid destination in bridging request", func(t *testing.T) {
		t.Run("1.1. Destination is Nexus", func(t *testing.T) {
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

		t.Run("1.2. Destination is unregistered", func(t *testing.T) {
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

	t.Run("2. Invalid destination in receiver", func(t *testing.T) {
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

	t.Run("3. 0 receivers in bridging request", func(t *testing.T) {
		err := executeInvalidNexusBridgingRequest(t, ctx, apex, user, InvalidNexusBridgingRequest{
			dstChainID:   cardanofw.ChainIDToInt(cardanofw.ChainIDVector),
			sender:       user,
			operationFee: big.NewInt(0),
			tokenInfo:    tokenInfo,
		})
		require.NoError(t, err)
	})

	t.Run("4. Too many receivers in bridging request", func(t *testing.T) {
		receivers := make(map[string]cardanofw.ReceiverAmount)
		for i := range 6 {
			receivers[apex.Users[i].GetAddress(cardanofw.ChainIDVector)] = cardanofw.ReceiverAmount{
				TokenID: cardanofw.USDTTokenID,
				Amount:  sendAmount,
			}
		}

		err = executeInvalidNexusBridgingRequest(t, ctx, apex, user, InvalidNexusBridgingRequest{
			dstChainID:   cardanofw.ChainIDToInt(cardanofw.ChainIDVector),
			sender:       user,
			receivers:    receivers,
			operationFee: big.NewInt(0),
			tokenInfo:    tokenInfo,
		})
		require.NoError(t, err)
	})

	t.Run("5. Invalid receiver address", func(t *testing.T) {
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

	t.Run("6. Invalid eth receiver address", func(t *testing.T) {
		err := executeInvalidNexusBridgingRequest(t, ctx, apex, user, InvalidNexusBridgingRequest{
			dstChainID: cardanofw.ChainIDToInt(cardanofw.ChainIDPolygon),
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

		err = executeInvalidNexusBridgingRequest(t, ctx, apex, user, req)
		require.Error(t, err)
		require.ErrorContains(t, err, "transaction receipt status is unsuccessful")
	})

	t.Run("11. Token amount below minimum allowed", func(t *testing.T) {
		err := executeInvalidNexusBridgingRequest(t, ctx, apex, user, InvalidNexusBridgingRequest{
			dstChainID: cardanofw.ChainIDToInt(cardanofw.ChainIDVector),
			sender:     user,
			receivers: map[string]cardanofw.ReceiverAmount{
				user.GetAddress(cardanofw.ChainIDVector): {
					TokenID: cardanofw.USDTTokenID,
					Amount:  cardanofw.DfmToWei(big.NewInt(int64(minColCoinsAllowedToBridge - 1))),
				},
			},
			operationFee: big.NewInt(0),
			tokenInfo:    tokenInfo,
		})
		require.NoError(t, err)
	})

	t.Run("12. Token amount below minimum allowed - evm receiver", func(t *testing.T) {
		err := executeInvalidNexusBridgingRequest(t, ctx, apex, user, InvalidNexusBridgingRequest{
			dstChainID: cardanofw.ChainIDToInt(cardanofw.ChainIDPolygon),
			sender:     user,
			receivers: map[string]cardanofw.ReceiverAmount{
				user.GetAddress(cardanofw.ChainIDPolygon): {
					TokenID: cardanofw.USDTTokenID,
					Amount:  cardanofw.DfmToWei(big.NewInt(int64(minColCoinsAllowedToBridge - 1))),
				},
			},
			operationFee: big.NewInt(0),
			tokenInfo:    tokenInfo,
		})
		require.NoError(t, err)
	})

	t.Run("13. Over max allowed to bridge", func(t *testing.T) {
		t.Run("1. Nexus -> Vector usdt", func(t *testing.T) {
			err := executeInvalidNexusBridgingRequest(t, ctx, apex, user, InvalidNexusBridgingRequest{
				dstChainID: cardanofw.ChainIDToInt(cardanofw.ChainIDVector),
				sender:     user,
				receivers: map[string]cardanofw.ReceiverAmount{
					user.GetAddress(cardanofw.ChainIDVector): {
						TokenID: cardanofw.USDTTokenID,
						Amount:  cardanofw.DfmToWei(new(big.Int).SetUint64(1_000_000_000_001)),
					},
				},
				operationFee: big.NewInt(0),
				tokenInfo:    tokenInfo,
			})
			require.NoError(t, err)
		})

		vectorChain := apex.GetChainMust(t, cardanofw.ChainIDVector).(*cardanofw.TestCardanoChain)
		err = cardanofw.MintToken(vectorChain, apex.VectorInfo.GenesisWallet, cardanofw.XADATokenName,
			new(big.Int).SetUint64(10000000000010))
		require.NoError(t, err)

		_, err = cardanofw.FundUsersWithToken(
			ctx, vectorChain, apex.VectorInfo.GenesisWallet,
			apex.Users, cardanofw.XADATokenName, cardanofw.ApexToWei(big.NewInt(2)), cardanofw.DfmToWei(big.NewInt(1_000_000_000_001)))
		require.NoError(t, err)

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDVector, cardanofw.ChainIDNexus, cardanofw.DfmToWei(big.NewInt(500_000_000_001)),
			cardanofw.XADATokenID)

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDVector, cardanofw.ChainIDNexus, cardanofw.DfmToWei(big.NewInt(500_000_000_000)),
			cardanofw.XADATokenID)

		t.Run("2. Nexus -> Vector xada", func(t *testing.T) {
			tokenInfo, err = apex.GetBridgingTokensInfo(cardanofw.ChainIDNexus, cardanofw.ChainIDVector, cardanofw.XADATokenID)
			require.NoError(t, err)

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
			tokenInfo, err := apex.GetBridgingTokensInfo(cardanofw.ChainIDNexus, cardanofw.ChainIDCardano, cardanofw.XADATokenID)
			require.NoError(t, err)

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

	t.Run("14. Insufficient balance", func(t *testing.T) {
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
		Uncomment when the operation fee is set to != 0 in settings
		t.Run("13. Wrong operation fee", func(t *testing.T) {
			tokenInfo, err := apex.GetBridgingTokensInfo(cardanofw.ChainIDNexus, cardanofw.ChainIDVector, cardanofw.USDTTokenID)
			require.NoError(t, err)

			err = executeInvalidNexusBridgingRequest(t, ctx, apex, user, InvalidNexusBridgingRequest{
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

	t.Run("15. Insufficient fee", func(t *testing.T) {
		tokenInfo, err := apex.GetBridgingTokensInfo(cardanofw.ChainIDNexus, cardanofw.ChainIDVector, cardanofw.USDTTokenID)
		require.NoError(t, err)

		err = executeInvalidNexusBridgingRequest(t, ctx, apex, user, InvalidNexusBridgingRequest{
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

	sendAmount := cardanofw.ApexToWei(big.NewInt(1))

	t.Run("Send radom tokens to custodial, relayer and plutus addresses", func(t *testing.T) {
		vectorChain := apex.GetChainMust(t, cardanofw.ChainIDVector)

		addresses := []string{
			vectorChain.GetCustodialAddress(),
			vectorChain.GetRelayerAddress(),
			vectorChain.GetCardanoScriptInfo().PlutusAddress,
		}

		err := cardanofw.MintToken(vectorChain.(*cardanofw.TestCardanoChain), apex.VectorInfo.GenesisWallet, "ranodom-token",
			new(big.Int).Mul(big.NewInt(3), new(big.Int).SetUint64(1_000_000_000_000)))
		require.NoError(t, err)

		_, err = cardanofw.FundAddressesWithToken(
			ctx, vectorChain.(*cardanofw.TestCardanoChain), apex.VectorInfo.GenesisWallet,
			addresses, "ranodom-token", new(big.Int).SetUint64(40_000_000_000), new(big.Int).SetUint64(1_000_000_000_000))
		require.NoError(t, err)
	})

	t.Run("Bridge to relayer, custodial and plutus addresses", func(t *testing.T) {
		vectorChain := apex.GetChainMust(t, cardanofw.ChainIDVector)
		sendAmount := cardanofw.ApexToWei(big.NewInt(5))

		addresses := []string{
			vectorChain.GetCustodialAddress(),
			vectorChain.GetRelayerAddress(),
			vectorChain.GetCardanoScriptInfo().PlutusAddress,
		}

		apexUsers := make([]*cardanofw.TestApexUser, len(addresses))

		for i, addr := range addresses {
			vectorAddr, err := wallet.NewCardanoAddressFromString(addr)
			require.NoError(t, err)

			apexUsers[i] = &cardanofw.TestApexUser{
				HasVectorWallet: true,
				VectorAddress:   vectorAddr,
			}
		}

		nexusChain := apex.GetChainMust(t, cardanofw.ChainIDNexus).(*cardanofw.TestEVMChain)
		err := nexusChain.FundUsersWithToken(
			user.GetAddress(cardanofw.ChainIDNexus),
			cardanofw.DfmToWei(big.NewInt(200_000_000)),
			cardanofw.USDTTokenID,
		)
		require.NoError(t, err)

		e2ehelper.ExecuteBridgingExtended(
			t, ctx, apex, 1,
			[]*cardanofw.TestApexUser{user},
			apexUsers,
			[]e2ehelper.BridgingDirectionConfig{
				{SrcChain: cardanofw.ChainIDCardano, DstChain: cardanofw.ChainIDVector, SrcTokenID: cardanofw.ADATokenID},
				{SrcChain: cardanofw.ChainIDNexus, DstChain: cardanofw.ChainIDVector, SrcTokenID: cardanofw.USDTTokenID},
			},
			sendAmount,
		)
	})

	t.Run("1. Cardano -> Vector -> Nexus -> Cardano - ADA/xADA", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDCardano, cardanofw.ChainIDVector, sendAmount,
			cardanofw.ADATokenID)

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDVector, cardanofw.ChainIDNexus, sendAmount,
			cardanofw.XADATokenID)

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDNexus, cardanofw.ChainIDCardano, sendAmount,
			cardanofw.XADATokenID)
	})

	t.Run("2. Cardano -> Nexus -> Vector -> Cardano - ADA/xADA", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() {
			t.Skip()
		}

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDCardano, cardanofw.ChainIDNexus, sendAmount,
			cardanofw.ADATokenID)

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDNexus, cardanofw.ChainIDVector, sendAmount,
			cardanofw.XADATokenID)

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDVector, cardanofw.ChainIDCardano, sendAmount,
			cardanofw.XADATokenID)
	})

	t.Run("3. Cardano -> Nexus; Cardano -> Vector -> Nexus; Nexus -> Vector - ADA/xADA", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() {
			t.Skip()
		}

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDCardano, cardanofw.ChainIDNexus, sendAmount,
			cardanofw.ADATokenID)

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDCardano, cardanofw.ChainIDVector, sendAmount,
			cardanofw.ADATokenID)

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDVector, cardanofw.ChainIDNexus, sendAmount,
			cardanofw.XADATokenID)

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDNexus, cardanofw.ChainIDVector, new(big.Int).Mul(sendAmount, big.NewInt(2)),
			cardanofw.XADATokenID)
	})

	type bridgingRequest struct {
		src             string
		dest            string
		srcMinterWallet *wallet.Wallet
		srcTokenID      uint16
	}

	var (
		bridgingRequests = []bridgingRequest{
			{src: cardanofw.ChainIDVector, dest: cardanofw.ChainIDNexus, srcTokenID: cardanofw.USDTTokenID, srcMinterWallet: apex.VectorInfo.GenesisWallet},
			{src: cardanofw.ChainIDNexus, dest: cardanofw.ChainIDVector, srcTokenID: cardanofw.USDTTokenID},
			{src: cardanofw.ChainIDVector, dest: cardanofw.ChainIDNexus, srcTokenID: cardanofw.XADATokenID, srcMinterWallet: apex.VectorInfo.GenesisWallet},
			{src: cardanofw.ChainIDNexus, dest: cardanofw.ChainIDVector, srcTokenID: cardanofw.XADATokenID},
			{src: cardanofw.ChainIDCardano, dest: cardanofw.ChainIDVector, srcTokenID: cardanofw.ADATokenID, srcMinterWallet: apex.CardanoInfo.GenesisWallet},
			{src: cardanofw.ChainIDVector, dest: cardanofw.ChainIDCardano, srcTokenID: cardanofw.XADATokenID, srcMinterWallet: apex.VectorInfo.GenesisWallet},
		}
	)

	getBridgingRequests := func(bridgingRequests []bridgingRequest) []e2ehelper.BridgingDirectionConfig {
		res := make([]e2ehelper.BridgingDirectionConfig, len(bridgingRequests))
		for i, br := range bridgingRequests {
			res[i] = e2ehelper.BridgingDirectionConfig{
				SrcChain:   br.src,
				DstChain:   br.dest,
				SrcTokenID: br.srcTokenID,
			}
		}

		return res
	}

	// This test is required because we cannot fund vector users with USDT tokens
	// nor nexus users with xADA tokens
	// and it is requirement for the tests that test bridging Vector -> Nexus with USDT tokens
	// and Nexus -> Vector with xADA tokens
	fundingPassed := t.Run("4. Nexus -> Vector USDT, Vector -> Nexus xADA funding test", func(t *testing.T) {
		// end goal for every user:
		// - 100_000_000 USDT and 200_000_000 xADA on Nexus and Vector
		// this way we don't need to fund users with tokens in every following test
		nexusChain := apex.GetChainMust(t, cardanofw.ChainIDNexus).(*cardanofw.TestEVMChain)
		vectorChain := apex.GetChainMust(t, cardanofw.ChainIDVector).(*cardanofw.TestCardanoChain)

		var wg sync.WaitGroup

		err := cardanofw.MintToken(vectorChain, apex.VectorInfo.GenesisWallet, cardanofw.XADATokenName,
			new(big.Int).Mul(big.NewInt(int64(len(apex.Users))), new(big.Int).SetUint64(400_000_000)))
		require.NoError(t, err)

		_, err = cardanofw.FundUsersWithToken(
			ctx, vectorChain, apex.VectorInfo.GenesisWallet,
			apex.Users, cardanofw.XADATokenName, cardanofw.ApexToWei(big.NewInt(400)), cardanofw.ApexToWei(big.NewInt(400)))
		require.NoError(t, err)

		for _, user := range apex.Users {
			err := nexusChain.FundUsersWithToken(
				user.GetAddress(cardanofw.ChainIDNexus),
				cardanofw.DfmToWei(big.NewInt(200_000_000)),
				cardanofw.USDTTokenID,
			)
			require.NoError(t, err)

			wg.Add(2)

			go func() {
				defer wg.Done()

				e2ehelper.ExecuteSingleBridging(
					t, ctx, apex, user, user, cardanofw.ChainIDNexus, cardanofw.ChainIDVector,
					cardanofw.DfmToWei(big.NewInt(100_000_000)), cardanofw.USDTTokenID)
			}()

			go func() {
				defer wg.Done()

				e2ehelper.ExecuteSingleBridging(
					t, ctx, apex, user, user, cardanofw.ChainIDVector, cardanofw.ChainIDNexus,
					cardanofw.DfmToWei(big.NewInt(200_000_000)), cardanofw.XADATokenID)
			}()
		}

		wg.Wait()
	})

	t.Run("5. Parallel bridging tests", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() || !fundingPassed {
			t.Skip()
		}

		const (
			instances = 5
		)

		sendAmount := cardanofw.ApexToWei(big.NewInt(1))

		for idx, br := range bridgingRequests {
			fmt.Printf("5.%d %s -> %s - srcTokenID: %d\n", idx+1, br.src, br.dest, br.srcTokenID)

			e2ehelper.ExecuteBridging(
				t, ctx, apex, 1, apex.Users[:instances], []*cardanofw.TestApexUser{user},
				[]string{br.src},
				map[string][]string{
					br.src: {br.dest},
				},
				map[e2ehelper.SrcDstChainPair]uint16{
					e2ehelper.NewChainPair(br.src, br.dest): br.srcTokenID,
				},
				sendAmount)
		}
	})

	t.Run("6. Sequential bridging tests", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() || !fundingPassed {
			t.Skip()
		}

		const (
			sequentialInstances = 5
			parallelInstances   = 10
			receivers           = 1
		)

		sendAmount := cardanofw.ApexToWei(big.NewInt(1))

		for idx, br := range bridgingRequests {
			fmt.Printf("6.%d %s -> %s - srcTokenID: %d\n", idx+1, br.src, br.dest, br.srcTokenID)

			e2ehelper.ExecuteBridging(
				t, ctx, apex, sequentialInstances,
				apex.Users[:sequentialInstances],
				apex.Users[:receivers],
				[]string{br.src},
				map[string][]string{
					br.src: {br.dest},
				},
				map[e2ehelper.SrcDstChainPair]uint16{
					e2ehelper.NewChainPair(br.src, br.dest): br.srcTokenID,
				},
				sendAmount,
			)
		}
	})

	t.Run("7. All directions parallel", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() || !fundingPassed {
			t.Skip()
		}

		const (
			instances = 5
		)

		sendAmount := cardanofw.ApexToWei(big.NewInt(1))

		e2ehelper.ExecuteBridgingExtended(
			t, ctx, apex, 1,
			apex.Users[:instances],
			[]*cardanofw.TestApexUser{user},
			getBridgingRequests(bridgingRequests),
			sendAmount,
		)
	})

	t.Run("8. All directions sequential and parallel", func(t *testing.T) {
		const (
			instances = 5
		)

		sendAmount := cardanofw.ApexToWei(big.NewInt(1))

		e2ehelper.ExecuteBridgingExtended(
			t, ctx, apex, instances,
			apex.Users[:instances],
			[]*cardanofw.TestApexUser{user},
			getBridgingRequests(bridgingRequests),
			sendAmount,
		)
	})
}

func Test_SkylineBridgeCC_WithRefund(t *testing.T) {
	if cardanofw.ShouldSkipE2RRedundantTests() {
		t.Skip()
	}

	const (
		apiKey  = "test_api_key"
		userCnt = 10

		maxWaitTimeSec = 600
		retryDelaySec  = 5
	)

	colCoinsAmount := cardanofw.DfmToWei(big.NewInt(2))

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

	_ = fundTestUsersWithToken(
		t, ctx, apex, []*testConfig{
			{
				srcChainID:      cardanofw.ChainIDVector,
				srcMinterWallet: apex.VectorInfo.GenesisWallet,
			},
			{
				srcChainID:      cardanofw.ChainIDCardano,
				srcMinterWallet: apex.CardanoInfo.GenesisWallet,
			},
		}, apex.Users[:userCnt], cardanofw.ApexToWei(big.NewInt(10)), cardanofw.DefaultTokenMintAmount)

	cardanoNexusTestConfig := newTestConfig(
		t, apex, apex.Config.CardanoConfig, &apex.CardanoInfo, cardanofw.ChainIDNexus, cardanofw.ADATokenID)
	vectorNexusTestConfig := newTestConfig(
		t, apex, apex.Config.VectorConfig, &apex.VectorInfo, cardanofw.ChainIDNexus, cardanofw.XADATokenID)
	vectorCardanoTestConfig := newTestConfig(
		t, apex, apex.Config.VectorConfig, &apex.VectorInfo, cardanofw.ChainIDCardano, cardanofw.XADATokenID)

	// Fund user on Nexus with USDT token
	nexusChain := apex.GetChainMust(t, cardanofw.ChainIDNexus).(*cardanofw.TestEVMChain)
	err := nexusChain.FundUsersWithToken(user.GetAddress(cardanofw.ChainIDNexus), cardanofw.DfmToWei(big.NewInt(100)), cardanofw.USDTTokenID)
	require.NoError(t, err)

	fmt.Printf("User: %+v\n", user.GetAddress(cardanofw.ChainIDNexus))

	refundTrigger := func(t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, srcChain, dstChain string,
		user *cardanofw.TestApexUser, tokenID uint16) {
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

		currencyID, err := apex.GetChainCurrencyID(srcChain)
		require.NoError(t, err)

		wrappedCurrencyID, err := apex.GetChainWrappedCurrencyID(srcChain)
		hasWrappedCurrency := err == nil

		switch tokenID {
		case currencyID:
			executeInvalidMismatchSendLovelaceAmount(t, ctx, apex, testConfig, user, maxWaitTimeSec, retryDelaySec, true, 0)
		case wrappedCurrencyID:
			require.True(t, hasWrappedCurrency)
			executeInvalidEmptyReceivers(t, ctx, apex, testConfig, user, maxWaitTimeSec, retryDelaySec, true, 0)
		default:
			executeInvalidColCoin(t, ctx, apex, testConfig, user, maxWaitTimeSec, retryDelaySec, 0,
				colCoinInvalidOpts{
					receivers:  createReceivers(apex, 1, testConfig.dstChainID, new(big.Int).Mul(colCoinsAmount, big.NewInt(10)), tokenID),
					amount:     colCoinsAmount,
					waitOption: NoWait,
				},
			)
		}
	}

	t.Run("1. Bridging xADA from Nexus to Cardano (ADA), Refund ADA on Cardano", func(t *testing.T) {
		// mint some xADA on Nexus for user
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDCardano, cardanofw.ChainIDNexus, cardanofw.ApexToWei(big.NewInt(100)),
			cardanofw.ADATokenID)

		e2ehelper.ExecuteBridgingWithRefund(
			t, ctx, apex, user, user, cardanofw.ChainIDNexus, cardanofw.ChainIDCardano, cardanofw.ApexToWei(big.NewInt(10)),
			cardanofw.XADATokenID, refundTrigger)
	})

	t.Run("2. Bridging USDT from Nexus to Vector (USDT), Refund USDT on Vector", func(t *testing.T) {
		// mint some USDT on Nexus for user
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDNexus, cardanofw.ChainIDVector, new(big.Int).Mul(big.NewInt(10), colCoinsAmount),
			cardanofw.USDTTokenID)

		e2ehelper.ExecuteBridgingWithRefund(
			t, ctx, apex, user, user, cardanofw.ChainIDNexus, cardanofw.ChainIDVector, colCoinsAmount,
			cardanofw.USDTTokenID, refundTrigger)
	})

	t.Run("3. Bridging ADA from Cardano to Vector (xADA), Refund xADA on Vector", func(t *testing.T) {
		e2ehelper.ExecuteBridgingWithRefund(
			t, ctx, apex, user, user, cardanofw.ChainIDCardano, cardanofw.ChainIDVector, cardanofw.ApexToWei(big.NewInt(10)),
			cardanofw.ADATokenID, refundTrigger)
	})
}
