package e2e

import (
	"bytes"
	"context"
	"fmt"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/e2e-polybft/cardanofw"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/e2ehelper"
	"github.com/Ethernal-Tech/cardano-infrastructure/sendtx"
	infrawallet "github.com/Ethernal-Tech/cardano-infrastructure/wallet"
	"github.com/stretchr/testify/require"
)

func TestE2E_SkylineRefund_ValidScenarios(t *testing.T) {
	const (
		apiKey  = "test_api_key"
		userCnt = 15

		maxWaitTimeSec = 600
		retryDelaySec  = 5
	)

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	primeConfig, vectorConfig, cardanoConfig := cardanofw.NewPrimeChainConfig(), cardanofw.NewVectorChainConfig(), cardanofw.NewCardanoChainConfig(true)
	primeConfig.FundAmount = 1_000_000_000
	vectorConfig.FundAmount = 1_000_000_000
	cardanoConfig.FundTokenAmount = 1_000_000_000

	apex := cardanofw.SetupAndRunSkylineBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithUserCnt(userCnt),
		cardanofw.WithCardanoConfig(cardanoConfig),
		cardanofw.WithPrimeConfig(primeConfig),
		cardanofw.WithVectorConfig(vectorConfig),
		cardanofw.WithCustomConfigHandlers(func(_ *cardanofw.ApexSystem, mp map[string]interface{}) {
			tryCountLimitsSettings := cardanofw.GetMapFromInterfaceKey(mp, "tryCountLimits")
			tryCountLimitsSettings["maxBatchTryCount"] = 1
			tryCountLimitsSettings["maxSubmitTryCount"] = 2
		}, nil, nil, nil),
		cardanofw.WithBridgingAddrCnt(cardanofw.ChainIDPrime, bridgeAddrCnt),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	user := apex.Users[userCnt-1]
	fmt.Println("prime user addr: ", user.PrimeAddress)
	fmt.Println("vector user addr: ", user.VectorAddress)
	fmt.Println("cardano user addr: ", user.CardanoAddress)
	fmt.Println("prime multisig addr: ", apex.PrimeInfo.MultisigAddr)
	fmt.Println("prime fee addr: ", apex.PrimeInfo.FeeAddr)
	fmt.Printf("prime socket path: %s\n", apex.PrimeInfo.SocketPath)
	fmt.Println("vector multisig addr: ", apex.VectorInfo.MultisigAddr)
	fmt.Println("vector fee addr: ", apex.VectorInfo.FeeAddr)
	fmt.Printf("vector socket path: %s\n", apex.VectorInfo.SocketPath)
	fmt.Println("cardano multisig addr: ", apex.CardanoInfo.MultisigAddr)
	fmt.Println("cardano fee addr: ", apex.CardanoInfo.FeeAddr)
	fmt.Printf("cardano socket path: %s\n", apex.CardanoInfo.SocketPath)

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
		}, apex.Users[:userCnt], uint64(10_000_000), cardanofw.DefaultTokenMintAmount)

	primeCardanoTestConfig := newTestConfig(
		t, apex, apex.Config.PrimeConfig, &apex.PrimeInfo, cardanofw.ChainIDCardano, cardanofw.AP3XTokenID)

	cardanoPrimeTestConfig := newTestConfig(
		t, apex, apex.Config.CardanoConfig, &apex.CardanoInfo, cardanofw.ChainIDPrime, cardanofw.CAP3XTokenID)

	vectorCardanoTestConfig := newTestConfig(
		t, apex, apex.Config.VectorConfig, &apex.VectorInfo, cardanofw.ChainIDCardano, cardanofw.XADATokenID)

	cardanoVectorTestConfig := newTestConfig(
		t, apex, apex.Config.CardanoConfig, &apex.CardanoInfo, cardanofw.ChainIDVector, cardanofw.ADATokenID)

	fmt.Printf("Prime->Cardano test config: %+v\n", primeCardanoTestConfig)
	fmt.Printf("Vector->Cardano test config: %+v\n", vectorCardanoTestConfig)
	fmt.Printf("Cardano->Prime test config: %+v\n", cardanoPrimeTestConfig)
	fmt.Printf("Cardano->Vector test config: %+v\n", cardanoVectorTestConfig)

	t.Run("1.1 Prime -> Cardano - Mismatch submitted and receiver amounts", func(t *testing.T) {
		executeInvalidMismatchSendLovelaceAmount(t, ctx, apex, primeCardanoTestConfig, user, maxWaitTimeSec, retryDelaySec, true, 0)
	})

	t.Run("1.2 Cardano -> Prime - Mismatch submitted and receiver amounts", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() {
			t.Skip()
		}

		executeInvalidMismatchSendLovelaceAmount(t, ctx, apex, cardanoPrimeTestConfig, user, maxWaitTimeSec, retryDelaySec, true, 0)
	})

	t.Run("1.3 Cardano -> Vector - Mismatch submitted and receiver amounts", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() {
			t.Skip()
		}

		executeInvalidMismatchSendLovelaceAmount(t, ctx, apex, cardanoVectorTestConfig, user, maxWaitTimeSec, retryDelaySec, true, 0)
	})

	t.Run("2.1 Prime -> Cardano - Multiple submitters mismatch submitted and receiver amounts", func(t *testing.T) {
		executeInvalidMismatchSendAmountMultipleInstances(t, ctx, apex, primeCardanoTestConfig, maxWaitTimeSec, retryDelaySec, true, 0)
	})

	t.Run("2.2 Cardano -> Prime - Multiple submitters mismatch submitted and receiver amounts", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() {
			t.Skip()
		}

		executeInvalidMismatchSendAmountMultipleInstances(t, ctx, apex, cardanoPrimeTestConfig, maxWaitTimeSec, retryDelaySec, true, 0)
	})

	t.Run("2.3 Vector -> Cardano - Multiple submitters mismatch submitted and receiver amounts", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() {
			t.Skip()
		}

		executeInvalidMismatchSendAmountMultipleInstances(t, ctx, apex, vectorCardanoTestConfig, maxWaitTimeSec, retryDelaySec, true, 0)
	})

	t.Run("3.1 Prime -> Cardano - Multiple submitters mismatch submitted and receiver amounts parallel", func(t *testing.T) {
		executeInvalidMismatchSendAmountMultipleInstancesParalel(t, ctx, apex, primeCardanoTestConfig, maxWaitTimeSec, retryDelaySec, true, 0)
	})

	t.Run("3.2 Cardano -> Prime - Multiple submitters mismatch submitted and receiver amounts parallel", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() {
			t.Skip()
		}

		executeInvalidMismatchSendAmountMultipleInstancesParalel(t, ctx, apex, cardanoPrimeTestConfig, maxWaitTimeSec, retryDelaySec, true, 0)
	})

	t.Run("3.3 Cardano -> Vector - Multiple submitters mismatch submitted and receiver amounts parallel", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() {
			t.Skip()
		}

		executeInvalidMismatchSendAmountMultipleInstancesParalel(t, ctx, apex, cardanoVectorTestConfig, maxWaitTimeSec, retryDelaySec, true, 0)
	})

	t.Run("4.1 Prime -> Cardano - Submitted invalid metadata - sliced off", func(t *testing.T) {
		executeInvalidMetadataSlicedOff(t, ctx, apex, primeCardanoTestConfig, 0)
	})

	t.Run("4.2 Cardano -> Prime - Submitted invalid metadata - sliced off", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() {
			t.Skip()
		}

		executeInvalidMetadataSlicedOff(t, ctx, apex, cardanoPrimeTestConfig, 0)
	})

	t.Run("4.3 Vector -> Cardano - Submitted invalid metadata - sliced off", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() {
			t.Skip()
		}

		executeInvalidMetadataSlicedOff(t, ctx, apex, vectorCardanoTestConfig, 0)
	})

	t.Run("5.1 Prime -> Cardano - Submitted invalid metadata - wrong type", func(t *testing.T) {
		executeInvalidMetadataType(t, ctx, apex, primeCardanoTestConfig, user, maxWaitTimeSec, retryDelaySec, true, 0)
	})

	t.Run("5.2 Cardano -> Prime - Submitted invalid metadata - wrong type", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() {
			t.Skip()
		}

		executeInvalidMetadataType(t, ctx, apex, cardanoPrimeTestConfig, user, maxWaitTimeSec, retryDelaySec, true, 0)
	})

	t.Run("5.3 Cardano -> Vector - Submitted invalid metadata - wrong type", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() {
			t.Skip()
		}

		executeInvalidMetadataType(t, ctx, apex, cardanoVectorTestConfig, user, maxWaitTimeSec, retryDelaySec, true, 0)
	})

	t.Run("6.1 Prime -> Cardano - Submitted invalid metadata - invalid sender", func(t *testing.T) {
		executeInvalidMetadataInvalidSender(t, ctx, apex, primeCardanoTestConfig, user, maxWaitTimeSec, 0)
	})

	t.Run("6.2 Cardano -> Prime - Submitted invalid metadata - invalid sender", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() {
			t.Skip()
		}

		executeInvalidMetadataInvalidSender(t, ctx, apex, cardanoPrimeTestConfig, user, maxWaitTimeSec, 0)
	})

	t.Run("6.3 Cardano -> Vector - Submitted invalid metadata - invalid sender", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() {
			t.Skip()
		}

		executeInvalidMetadataInvalidSender(t, ctx, apex, cardanoVectorTestConfig, user, maxWaitTimeSec, 0)
	})

	t.Run("7.1 Prime -> Cardano - Submitted invalid metadata - invalid bridging fee", func(t *testing.T) {
		executeInvalidBridgingFee(t, ctx, apex, primeCardanoTestConfig, maxWaitTimeSec, retryDelaySec, true, 0)
	})

	t.Run("7.2 Cardano -> Prime - Submitted invalid metadata - invalid bridging fee", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() {
			t.Skip()
		}

		executeInvalidBridgingFee(t, ctx, apex, cardanoPrimeTestConfig, maxWaitTimeSec, retryDelaySec, true, 0)
	})

	t.Run("7.3 Vector -> Cardano - Submitted invalid metadata - invalid bridging fee", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() {
			t.Skip()
		}

		executeInvalidBridgingFee(t, ctx, apex, vectorCardanoTestConfig, maxWaitTimeSec, retryDelaySec, true, 0)
	})

	t.Run("8.1 Cardano -> Prime - Submitted invalid metadata - invalid fee receiver address", func(t *testing.T) {
		executeInvalidFeeReceiverAddr(t, ctx, apex, cardanoPrimeTestConfig, cardanofw.CAP3XTokenID, maxWaitTimeSec, retryDelaySec, true, 0)
	})

	t.Run("8.2 Cardano -> Vector - Submitted invalid metadata - invalid fee receiver address", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() {
			t.Skip()
		}

		executeInvalidFeeReceiverAddr(t, ctx, apex, cardanoVectorTestConfig, cardanofw.CAP3XTokenID, maxWaitTimeSec, retryDelaySec, true, 0)
	})

	t.Run("9.1 Prime -> Cardano - Submitted invalid metadata - empty receivers", func(t *testing.T) {
		executeInvalidEmptyReceivers(t, ctx, apex, primeCardanoTestConfig, user, maxWaitTimeSec, retryDelaySec, true, 0)
	})

	t.Run("9.2 Cardano -> Prime - Submitted invalid metadata - empty receivers", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() {
			t.Skip()
		}

		executeInvalidEmptyReceivers(t, ctx, apex, cardanoPrimeTestConfig, user, maxWaitTimeSec, retryDelaySec, true, 0)
	})

	t.Run("9.3 Vector -> Cardano - Submitted invalid metadata - empty receivers", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() {
			t.Skip()
		}

		executeInvalidEmptyReceivers(t, ctx, apex, vectorCardanoTestConfig, user, maxWaitTimeSec, retryDelaySec, true, 0)
	})

	t.Run("10. Submitted with unknown tokens to bridging addr", func(t *testing.T) {
		user := apex.Users[0]
		minterWallet, _ := user.GetCardanoWallet(cardanofw.ChainIDCardano)

		tokensFunded, err := cardanofw.FundUserWithToken(
			ctx, apex, cardanofw.ChainIDCardano,
			minterWallet, user,
			cardanofw.DefaultTokenName, cardanofw.DefaultTokenMintAmount,
			uint64(1_500_000), uint64(1_000_000))
		require.NoError(t, err)

		executeInvalidSendNativeToken(t, ctx, apex, user, cardanoPrimeTestConfig, *tokensFunded, maxWaitTimeSec, retryDelaySec, true, 0)
	})

	t.Run("11. Submitted invalid metadata - invalid send amount - token on source", func(t *testing.T) {
		user, err := cardanofw.NewTestApexUser(cardanofw.NewApexNetworkTypesFromSystem(apex))
		require.NoError(t, err)

		tokensFunded, err := cardanofw.FundUserWithToken(
			ctx, apex, cardanofw.ChainIDVector,
			apex.VectorInfo.GenesisWallet, user,
			cardanofw.XADATokenName, cardanofw.DefaultTokenMintAmount,
			uint64(10_000_000), uint64(1_123_000))
		require.NoError(t, err)

		executeInvalidMismatchSendNativeTokenAmount(t, ctx, apex, user, vectorCardanoTestConfig, *tokensFunded, maxWaitTimeSec, retryDelaySec, true, 0)
	})

	t.Run("12. vector -> cardano - currency on src", func(t *testing.T) {
		executeInvalidTokenDirection(t, ctx, apex, vectorCardanoTestConfig, cardanofw.AP3XTokenID, user, maxWaitTimeSec, retryDelaySec, true, 0)
	})
}

func TestE2E_SkylineRefund_NexusDest_ValidScenarios(t *testing.T) {
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
			setting := cardanofw.GetMapFromInterfaceKey(mp, "bridgingSettings")
			setting["minColCoinsAllowedToBridge"] = minColCoinsAllowedToBridge
		}, nil, nil, nil),
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
		}, apex.Users[:userCnt], uint64(10_000_000), cardanofw.DefaultTokenMintAmount)

	// Fund user on Nexus with USDT token
	nexusChain := apex.GetChainMust(t, cardanofw.ChainIDNexus).(*cardanofw.TestEVMChain)
	err := nexusChain.FundUsersWithToken(user.GetAddress(cardanofw.ChainIDNexus), big.NewInt(100), cardanofw.USDTTokenID)
	require.NoError(t, err)

	cardanoNexusTestConfig := newTestConfig(
		t, apex, apex.Config.CardanoConfig, &apex.CardanoInfo, cardanofw.ChainIDNexus, cardanofw.ADATokenID)
	vectorNexusXADATestConfig := newTestConfig(
		t, apex, apex.Config.VectorConfig, &apex.VectorInfo, cardanofw.ChainIDNexus, cardanofw.XADATokenID)
	vectorNexusUSDTTestConfig := newTestConfig(
		t, apex, apex.Config.VectorConfig, &apex.VectorInfo, cardanofw.ChainIDNexus, cardanofw.USDTTokenID)

	fmt.Printf("User: %+v\n", user.GetAddress(cardanofw.ChainIDNexus))

	t.Run("1. Cardano -> Nexus - Mismatch submitted and receiver amounts", func(t *testing.T) {
		executeInvalidMismatchSendLovelaceAmount(t, ctx, apex, cardanoNexusTestConfig, user, maxWaitTimeSec, retryDelaySec, true, 0)
	})

	t.Run("2. Vector -> Nexus - Submitted invalid metadata - invalid send amount - token on source", func(t *testing.T) {
		user, err := cardanofw.NewTestApexUser(cardanofw.NewApexNetworkTypesFromSystem(apex))
		require.NoError(t, err)

		tokensFunded, err := cardanofw.FundUserWithToken(
			ctx, apex, cardanofw.ChainIDVector,
			apex.VectorInfo.GenesisWallet, user,
			cardanofw.XADATokenName, cardanofw.DefaultTokenMintAmount,
			uint64(10_000_000), uint64(1_123_000))
		require.NoError(t, err)

		executeInvalidMismatchSendNativeTokenAmount(t, ctx, apex, user, vectorNexusXADATestConfig, *tokensFunded, maxWaitTimeSec, retryDelaySec, true, 0)
	})

	t.Run("3. Cardano -> Nexus - Invalid destination - native token on source", func(t *testing.T) {
		executeInvalidTokenDirection(t, ctx, apex, cardanoNexusTestConfig, cardanofw.CAP3XTokenID, user, maxWaitTimeSec, retryDelaySec, true, 0)
	})

	t.Run("4. Vector -> Nexus - Mismatch submitted and receiver amounts - USDT on source", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDNexus, cardanofw.ChainIDVector, big.NewInt(int64(minColCoinsAllowedToBridge)),
			cardanofw.USDTTokenID, true)

		initialTreasuryBalance, err := apex.GetTreasuryAddressBalance(ctx, t, cardanofw.ChainIDVector)
		require.NoError(t, err)

		executeInvalidColCoin(t, ctx, apex, vectorNexusUSDTTestConfig, user, maxWaitTimeSec, retryDelaySec, 0,
			colCoinInvalidOpts{
				receivers:  createReceivers(apex, 1, vectorNexusUSDTTestConfig.dstChainID, minColCoinsAllowedToBridge*10, cardanofw.USDTTokenID),
				amount:     minColCoinsAllowedToBridge,
				waitOption: WaitRefundEnabled,
			},
		)

		err = apex.ValidateTreasuryAddressBalance(ctx, t, cardanofw.ChainIDVector, initialTreasuryBalance, 1)
		require.NoError(t, err)
	})

	t.Run("5. Vector -> Nexus - Mismatch submitted and multiple receiver amounts - USDT on source", func(t *testing.T) {
		instances := 3

		for i := range instances {
			e2ehelper.ExecuteSingleBridging(
				t, ctx, apex, user, apex.Users[i], cardanofw.ChainIDNexus, cardanofw.ChainIDVector, big.NewInt(int64(minColCoinsAllowedToBridge)),
				cardanofw.USDTTokenID, true)
		}

		initialTreasuryBalance, err := apex.GetTreasuryAddressBalance(ctx, t, cardanofw.ChainIDVector)
		require.NoError(t, err)

		executeInvalidMismatchSendColCoinsMultipleInstancesParalel(t, ctx, apex, vectorNexusUSDTTestConfig, minColCoinsAllowedToBridge, instances, maxWaitTimeSec, retryDelaySec, true, 0)

		err = apex.ValidateTreasuryAddressBalance(ctx, t, cardanofw.ChainIDVector, initialTreasuryBalance, uint64(instances))
		require.NoError(t, err)
	})

	t.Run("6. Vector -> Nexus - Invalid destination - USDT on source", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDNexus, cardanofw.ChainIDVector, big.NewInt(int64(minColCoinsAllowedToBridge)),
			cardanofw.USDTTokenID, true)

		initialTreasuryBalance, err := apex.GetTreasuryAddressBalance(ctx, t, cardanofw.ChainIDVector)
		require.NoError(t, err)

		executeInvalidColCoin(t, ctx, apex, vectorNexusUSDTTestConfig, user, maxWaitTimeSec, retryDelaySec, 0,
			colCoinInvalidOpts{
				receivers:  createReceivers(apex, 1, vectorNexusXADATestConfig.dstChainID, minColCoinsAllowedToBridge, cardanofw.USDTTokenID),
				amount:     minColCoinsAllowedToBridge,
				waitOption: WaitRefundEnabled,
				metadataModifier: func(metadata []byte) []byte {
					return bytes.Replace(metadata, fmt.Appendf(nil, "\"%s\"", vectorNexusXADATestConfig.dstChainID), []byte("\"unknown\""), 1)
				},
			},
		)

		err = apex.ValidateTreasuryAddressBalance(ctx, t, cardanofw.ChainIDVector, initialTreasuryBalance, 1)
		require.NoError(t, err)
	})

	t.Run("7. Vector -> Nexus - Invalid metadata type - USDT on source", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDNexus, cardanofw.ChainIDVector, big.NewInt(int64(minColCoinsAllowedToBridge)),
			cardanofw.USDTTokenID, true)

		initialTreasuryBalance, err := apex.GetTreasuryAddressBalance(ctx, t, cardanofw.ChainIDVector)
		require.NoError(t, err)

		executeInvalidColCoin(t, ctx, apex, vectorNexusUSDTTestConfig, user, maxWaitTimeSec, retryDelaySec, 0,
			colCoinInvalidOpts{
				receivers:  createReceivers(apex, 1, vectorNexusUSDTTestConfig.dstChainID, minColCoinsAllowedToBridge, cardanofw.USDTTokenID),
				amount:     minColCoinsAllowedToBridge,
				waitOption: WaitRefundEnabled,
				metadataModifier: func(metadata []byte) []byte {
					return bytes.Replace(metadata, []byte("bridge"), []byte("xxxxx"), 1)
				},
			},
		)

		err = apex.ValidateTreasuryAddressBalance(ctx, t, cardanofw.ChainIDVector, initialTreasuryBalance, 1)
		require.NoError(t, err)
	})

	t.Run("8. Vector -> Nexus - Invalid receiver amount - USDT on source", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDNexus, cardanofw.ChainIDVector, big.NewInt(int64(minColCoinsAllowedToBridge)),
			cardanofw.USDTTokenID, true)

		initialTreasuryBalance, err := apex.GetTreasuryAddressBalance(ctx, t, cardanofw.ChainIDVector)
		require.NoError(t, err)

		invalidAmount := minColCoinsAllowedToBridge - 1
		executeInvalidColCoin(t, ctx, apex, vectorNexusUSDTTestConfig, user, maxWaitTimeSec, retryDelaySec, 0,
			colCoinInvalidOpts{
				receivers:  createReceivers(apex, 1, vectorNexusUSDTTestConfig.dstChainID, invalidAmount, cardanofw.USDTTokenID),
				amount:     invalidAmount,
				waitOption: WaitRefundEnabled,
			},
		)

		err = apex.ValidateTreasuryAddressBalance(ctx, t, cardanofw.ChainIDVector, initialTreasuryBalance, 1)
		require.NoError(t, err)
	})

	t.Run("9. Vector -> Nexus - Invalid receiver address - USDT on source", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDNexus, cardanofw.ChainIDVector, big.NewInt(int64(minColCoinsAllowedToBridge)),
			cardanofw.USDTTokenID, true)

		initialTreasuryBalance, err := apex.GetTreasuryAddressBalance(ctx, t, cardanofw.ChainIDVector)
		require.NoError(t, err)

		executeInvalidColCoin(t, ctx, apex, vectorNexusUSDTTestConfig, user, maxWaitTimeSec, retryDelaySec, 0,
			colCoinInvalidOpts{
				receivers: []sendtx.BridgingTxReceiver{
					{
						Addr:    "addr1qxyz...invalidaddress",
						Amount:  minColCoinsAllowedToBridge,
						TokenID: cardanofw.USDTTokenID,
					},
				},
				amount:     minColCoinsAllowedToBridge,
				waitOption: WaitRefundEnabled,
			},
		)

		err = apex.ValidateTreasuryAddressBalance(ctx, t, cardanofw.ChainIDVector, initialTreasuryBalance, 1)
		require.NoError(t, err)
	})
}

func TestE2E_SkylineRefund_MBASpecific(t *testing.T) {
	const (
		apiKey  = "test_api_key"
		userCnt = 15

		maxWaitTimeSec = 600
		retryDelaySec  = 5
	)

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	primeConfig, cardanoConfig := cardanofw.NewPrimeChainConfig(), cardanofw.NewCardanoChainConfig(true)
	cardanoConfig.FundTokenAmount = 1_000_000_000

	apex := cardanofw.SetupAndRunSkylineBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithUserCnt(userCnt),
		cardanofw.WithCardanoConfig(cardanoConfig),
		cardanofw.WithPrimeConfig(primeConfig),
		cardanofw.WithCustomConfigHandlers(func(_ *cardanofw.ApexSystem, mp map[string]interface{}) {
			tryCountLimitsSettings := cardanofw.GetMapFromInterfaceKey(mp, "tryCountLimits")
			tryCountLimitsSettings["maxBatchTryCount"] = 1
			tryCountLimitsSettings["maxSubmitTryCount"] = 2
		}, nil, nil, nil),
		cardanofw.WithBridgingAddrCnt(cardanofw.ChainIDPrime, bridgeAddrCnt),
		cardanofw.WithBridgingAddrCnt(cardanofw.ChainIDCardano, bridgeAddrCnt),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	user := apex.Users[userCnt-1]
	fmt.Println("prime user addr: ", user.PrimeAddress)
	fmt.Println("cardano user addr: ", user.CardanoAddress)
	fmt.Println("prime multisig addr: ", apex.PrimeInfo.MultisigAddr)
	fmt.Println("prime fee addr: ", apex.PrimeInfo.FeeAddr)
	fmt.Printf("prime socket path: %s\n", apex.PrimeInfo.SocketPath)
	fmt.Println("cardano multisig addr: ", apex.CardanoInfo.MultisigAddr)
	fmt.Println("cardano fee addr: ", apex.CardanoInfo.FeeAddr)
	fmt.Printf("cardano socket path: %s\n", apex.CardanoInfo.SocketPath)

	tokens := fundTestUsersWithToken(
		t, ctx, apex, []*testConfig{
			{
				srcChainID:      cardanofw.ChainIDCardano,
				srcMinterWallet: apex.CardanoInfo.GenesisWallet,
			},
		}, apex.Users[:userCnt], uint64(10_000_000), cardanofw.DefaultTokenMintAmount)
	cardanoToken := tokens[0]

	primeCardanoTestConfig := newTestConfig(
		t, apex, apex.Config.PrimeConfig, &apex.PrimeInfo, cardanofw.ChainIDCardano, cardanofw.AP3XTokenID)

	cardanoPrimeTestConfig := newTestConfig(
		t, apex, apex.Config.CardanoConfig, &apex.CardanoInfo, cardanofw.ChainIDPrime, cardanofw.CAP3XTokenID)

	fmt.Printf("Cardano test config: %+v\n", cardanoPrimeTestConfig.tokensInfo.SrcTokenName)

	t.Run("1. Prime -> Cardano - Mismatch submitted and receiver amounts", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() {
			t.Skip()
		}

		executeInvalidMismatchSendLovelaceAmount(t, ctx, apex, primeCardanoTestConfig, user, maxWaitTimeSec, retryDelaySec, true, 1)
	})

	t.Run("2. Prime -> Cardano - Multiple submitters mismatch submitted and receiver amounts", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() {
			t.Skip()
		}

		executeInvalidMismatchSendAmountMultipleInstances(t, ctx, apex, primeCardanoTestConfig, maxWaitTimeSec, retryDelaySec, true, 2)
	})

	t.Run("3. Prime -> Cardano - Multiple submitters mismatch submitted and receiver amounts parallel", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() {
			t.Skip()
		}

		executeInvalidMismatchSendAmountMultipleInstancesParalel(t, ctx, apex, primeCardanoTestConfig, maxWaitTimeSec, retryDelaySec, true, 3)
	})

	t.Run("4. Prime -> Cardano - Submitted invalid metadata - sliced off", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() {
			t.Skip()
		}

		executeInvalidMetadataSlicedOff(t, ctx, apex, primeCardanoTestConfig, 1)
	})

	t.Run("5. Prime -> Cardano - Submitted invalid metadata - wrong type", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() {
			t.Skip()
		}

		executeInvalidMetadataType(t, ctx, apex, primeCardanoTestConfig, user, maxWaitTimeSec, retryDelaySec, true, 2)
	})

	t.Run("6. Prime -> Cardano - Submitted invalid metadata - invalid sender", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() {
			t.Skip()
		}

		executeInvalidMetadataInvalidSender(t, ctx, apex, primeCardanoTestConfig, user, maxWaitTimeSec, 1)
	})

	t.Run("7. Prime -> Cardano - Submitted invalid metadata - invalid bridging fee", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() {
			t.Skip()
		}

		executeInvalidBridgingFee(t, ctx, apex, primeCardanoTestConfig, maxWaitTimeSec, retryDelaySec, true, 2)
	})

	t.Run("8. Cardano -> Prime - Submitted invalid metadata - invalid fee receiver address", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() {
			t.Skip()
		}

		executeInvalidFeeReceiverAddr(t, ctx, apex, cardanoPrimeTestConfig, cardanofw.CAP3XTokenID, maxWaitTimeSec, retryDelaySec, true, 0)
	})

	t.Run("9. Prime -> Cardano - Submitted invalid metadata - empty receivers", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() {
			t.Skip()
		}

		executeInvalidEmptyReceivers(t, ctx, apex, primeCardanoTestConfig, user, maxWaitTimeSec, retryDelaySec, true, 1)
	})

	t.Run("10. Submitted with unknown tokens to bridging addr", func(t *testing.T) {
		user := apex.Users[0]
		minterWallet, _ := user.GetCardanoWallet(cardanofw.ChainIDCardano)

		tokensFunded, err := cardanofw.FundUserWithToken(
			ctx, apex, cardanofw.ChainIDCardano,
			minterWallet, user,
			cardanofw.DefaultTokenName, cardanofw.DefaultTokenMintAmount,
			uint64(1_500_000), uint64(1_000_000))
		require.NoError(t, err)

		cardanoAddrAmounts, err := apex.GetBridgingAddressesTokenAmounts(ctx, cardanofw.ChainIDCardano)
		require.NoError(t, err)
		fmt.Println("Cardano multisig addresses amounts: ", cardanoAddrAmounts)

		executeInvalidSendNativeToken(t, ctx, apex, user, cardanoPrimeTestConfig, *tokensFunded, maxWaitTimeSec, retryDelaySec, true, 0)

		cardanoAddrAmounts, err = apex.GetBridgingAddressesTokenAmounts(ctx, cardanofw.ChainIDCardano)
		require.NoError(t, err)
		fmt.Println("Cardano multisig addresses amounts: ", cardanoAddrAmounts)
	})

	t.Run("11. Submitted invalid metadata - invalid send amount - token on source", func(t *testing.T) {
		cardanoAddrAmounts, err := apex.GetBridgingAddressesTokenAmounts(ctx, cardanofw.ChainIDCardano)
		require.NoError(t, err)
		fmt.Println("Cardano multisig addresses amounts: ", cardanoAddrAmounts)

		user, err := cardanofw.NewTestApexUser(cardanofw.NewApexNetworkTypesFromSystem(apex))
		require.NoError(t, err)

		tokensFunded, err := cardanofw.FundUserWithToken(
			ctx, apex, cardanofw.ChainIDCardano,
			apex.CardanoInfo.GenesisWallet, user,
			cardanofw.CAP3XTokenName, cardanofw.DefaultTokenMintAmount,
			uint64(10_000_000), uint64(1_123_000))
		require.NoError(t, err)

		executeInvalidMismatchSendNativeTokenAmount(t, ctx, apex, user, cardanoPrimeTestConfig, *tokensFunded, maxWaitTimeSec, retryDelaySec, true, 0)

		cardanoAddrAmounts, err = apex.GetBridgingAddressesTokenAmounts(ctx, cardanofw.ChainIDCardano)
		require.NoError(t, err)
		fmt.Println("Cardano multisig addresses amounts: ", cardanoAddrAmounts)
	})

	t.Run("12. Submitted tokens to bridging addr other than 0", func(t *testing.T) {
		user := apex.Users[0]

		cardanoAddrAmounts, err := apex.GetBridgingAddressesTokenAmounts(ctx, cardanofw.ChainIDCardano)
		require.NoError(t, err)
		fmt.Println("Cardano multisig addresses amounts: ", cardanoAddrAmounts)

		executeInvalidSendNativeToken(t, ctx, apex, user, cardanoPrimeTestConfig, *cardanoToken, maxWaitTimeSec, retryDelaySec, true, 2)

		cardanoAddrAmounts, err = apex.GetBridgingAddressesTokenAmounts(ctx, cardanofw.ChainIDCardano)
		require.NoError(t, err)
		fmt.Println("Cardano multisig addresses amounts: ", cardanoAddrAmounts)
	})
}

func TestE2E_SkylineRefund_Over_Max_Allowed_To_Bridge(t *testing.T) {
	if cardanofw.ShouldSkipE2RRedundantTests() {
		t.Skip()
	}

	const (
		apiKey = "test_api_key"
	)

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	primeConfig, cardanoConfig := cardanofw.NewPrimeChainConfig(), cardanofw.NewCardanoChainConfig(true)
	cardanoConfig.FundTokenAmount = 1_000_000_000

	apex := cardanofw.SetupAndRunSkylineBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithUserCnt(1),
		cardanofw.WithCardanoConfig(cardanoConfig),
		cardanofw.WithPrimeConfig(primeConfig),
		cardanofw.WithCustomConfigHandlers(func(_ *cardanofw.ApexSystem, mp map[string]interface{}) {
			setting := cardanofw.GetMapFromInterfaceKey(mp, "bridgingSettings")
			setting["maxAmountAllowedToBridge"] = new(big.Int).SetUint64(5_000_000)
		}, nil, nil, nil),
		cardanofw.WithBridgingAddrCnt(cardanofw.ChainIDPrime, bridgeAddrCnt),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	var (
		user             = apex.Users[0]
		apexSendAmount   = cardanofw.ApexToDfm(big.NewInt(10))
		bridgingRequests = []struct {
			src        string
			dest       string
			sender     *cardanofw.TestApexUser
			srcTokenID uint16
		}{
			{src: cardanofw.ChainIDPrime, dest: cardanofw.ChainIDCardano, srcTokenID: cardanofw.AP3XTokenID, sender: apex.Users[0]},
			{src: cardanofw.ChainIDCardano, dest: cardanofw.ChainIDVector, srcTokenID: cardanofw.ADATokenID, sender: apex.Users[0]},
		}
		txHashes = make([]string, len(bridgingRequests))
	)

	var wg sync.WaitGroup

	beforeSendingAmountDfm := make([]map[string]*big.Int, len(bridgingRequests))

	for idx, br := range bridgingRequests {
		wg.Add(1)

		go func(i int, src string, dest string, srcTokenID uint16, sender *cardanofw.TestApexUser) {
			defer wg.Done()

			var err error

			beforeSendingAmountDfm[idx], err = apex.GetBalance(ctx, user, src)
			require.NoError(t, err)

			txHashes[i], err = apex.SubmitBridgingRequest(cardanofw.SubmitBridgingRequestData{
				Context:          ctx,
				SourceChain:      src,
				DestinationChain: dest,
				Sender:           sender,
				DFMAmount:        apexSendAmount,
				SrcTokenID:       srcTokenID,
				Receivers:        []*cardanofw.TestApexUser{user},
			})
			require.NoError(t, err)

			fmt.Printf("Bridging request: %v to %v sent. hash: %s\n", src, dest, txHashes[i])
		}(idx, br.src, br.dest, br.srcTokenID, br.sender)
	}

	wg.Wait()

	for idx, br := range bridgingRequests {
		wg.Add(1)

		go func() {
			defer wg.Done()

			lowerBoundaryDfm := new(big.Int).Sub(
				beforeSendingAmountDfm[idx][infrawallet.AdaTokenName],
				new(big.Int).Add(apexSendAmount, new(big.Int).SetUint64(apex.GetMinBridgingFee(br.src, false))))

			fmt.Printf("Tx sent. hash: %s, lowerBoundaryDfm: %d, higherBoundaryDfm: %+v\n", txHashes[idx], lowerBoundaryDfm, beforeSendingAmountDfm[idx][infrawallet.AdaTokenName])

			err := apex.WaitForAmountInRange(ctx, user, br.src, lowerBoundaryDfm, beforeSendingAmountDfm[idx][infrawallet.AdaTokenName], 20, time.Second*30, infrawallet.AdaTokenName)
			require.NoError(t, err)
		}()
	}

	wg.Wait()
}

func TestE2E_SkylineRefund_Over_Max_Tokens_Allowed_To_Bridge(t *testing.T) {
	if cardanofw.ShouldSkipE2RRedundantTests() {
		t.Skip()
	}

	const (
		apiKey = "test_api_key"
	)

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	primeConfig, vectorConfig, cardanoConfig := cardanofw.NewPrimeChainConfig(), cardanofw.NewVectorChainConfig(), cardanofw.NewCardanoChainConfig(true)
	vectorConfig.FundTokenAmount = 1_000_000_000
	cardanoConfig.FundTokenAmount = 1_000_000_000

	apex := cardanofw.SetupAndRunSkylineBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithUserCnt(1),
		cardanofw.WithCardanoConfig(cardanoConfig),
		cardanofw.WithPrimeConfig(primeConfig),
		cardanofw.WithVectorConfig(vectorConfig),
		cardanofw.WithCustomConfigHandlers(func(_ *cardanofw.ApexSystem, mp map[string]interface{}) {
			setting := cardanofw.GetMapFromInterfaceKey(mp, "bridgingSettings")
			setting["maxTokenAmountAllowedToBridge"] = new(big.Int).SetUint64(5_000_000)
		}, nil, nil, nil),
		cardanofw.WithBridgingAddrCnt(cardanofw.ChainIDPrime, bridgeAddrCnt),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	var (
		user             = apex.Users[0]
		apexSendAmount   = cardanofw.ApexToDfm(big.NewInt(10))
		bridgingRequests = []struct {
			src        string
			dest       string
			sender     *cardanofw.TestApexUser
			srcTokenID uint16
		}{
			{src: cardanofw.ChainIDVector, dest: cardanofw.ChainIDCardano, srcTokenID: cardanofw.XADATokenID, sender: apex.Users[0]},
			{src: cardanofw.ChainIDCardano, dest: cardanofw.ChainIDPrime, srcTokenID: cardanofw.CAP3XTokenID, sender: apex.Users[0]},
		}
		initialBalances = map[string]map[string]*big.Int{}

		mu sync.RWMutex
	)

	fundTestUsersWithToken(t, ctx, apex, []*testConfig{
		{
			srcChainID:      cardanofw.ChainIDVector,
			srcMinterWallet: apex.VectorInfo.GenesisWallet,
		},
		{
			srcChainID:      cardanofw.ChainIDCardano,
			srcMinterWallet: apex.CardanoInfo.GenesisWallet,
		},
	}, apex.Users[:1], uint64(5_000_000), uint64(1_000_000_000))

	var (
		wg  sync.WaitGroup
		err error
	)

	for idx, br := range bridgingRequests {
		wg.Add(1)

		go func(i int, src, dest string, srcTokenID uint16, sender *cardanofw.TestApexUser) {
			defer wg.Done()

			mu.Lock()
			initialBalances[src], err = apex.GetBalance(ctx, sender, src)
			mu.Unlock()
			require.NoError(t, err)

			txHash, err := apex.SubmitBridgingRequest(
				cardanofw.SubmitBridgingRequestData{
					Context:          ctx,
					SourceChain:      src,
					DestinationChain: dest,
					Sender:           sender,
					DFMAmount:        apexSendAmount,
					SrcTokenID:       srcTokenID,
					Receivers:        []*cardanofw.TestApexUser{user},
				})
			require.NoError(t, err)

			fmt.Printf("Bridging request: %v to %v sent. hash: %s\n", src, dest, txHash)
		}(idx, br.src, br.dest, br.srcTokenID, br.sender)
	}

	wg.Wait()

	for _, br := range bridgingRequests {
		wg.Add(1)

		go func(src, dest string, srcTokenID uint16, sender *cardanofw.TestApexUser) {
			defer wg.Done()

			tokensInfo, err := apex.GetBridgingTokensInfo(src, dest, srcTokenID)
			require.NoError(t, err)

			mu.RLock()
			tokenBalance := initialBalances[br.src][tokensInfo.SrcTokenName]
			mu.RUnlock()

			err = apex.WaitForExactAmount(ctx, br.sender, br.src, tokenBalance, 30, 30*time.Second, tokensInfo.SrcTokenName)
			require.NoError(t, err)
		}(br.src, br.dest, br.srcTokenID, br.sender)
	}

	wg.Wait()
}

func TestE2E_SkylineRefund_DisabledDirection(t *testing.T) {
	if cardanofw.ShouldSkipE2RRedundantTests() {
		t.Skip()
	}

	type bridgingRequest struct {
		src        string
		dest       string
		sender     *cardanofw.TestApexUser
		srcTokenID uint16
		isValid    bool
	}

	const apiKey = "test_api_key"

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	primeConfig, vectorConfig, cardanoConfig := cardanofw.NewPrimeChainConfig(), cardanofw.NewVectorChainConfig(), cardanofw.NewCardanoChainConfig(true)
	primeConfig.FundTokenAmount = 0 // very important otherwise HWIC wont work

	apex := cardanofw.SetupAndRunSkylineBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithUserCnt(3),
		cardanofw.WithCardanoConfig(cardanoConfig),
		cardanofw.WithPrimeConfig(primeConfig),
		cardanofw.WithVectorConfig(vectorConfig),
		cardanofw.WithCustomConfigHandlers(nil, nil, func(a *cardanofw.ApexSystem, mp map[string]interface{}) {
			cardanoCfg := cardanofw.GetMapFromInterfaceKey(mp, "directions", "cardano")
			// remove vector from map
			delete(cardanoCfg["destChain"].(map[string]interface{}), "vector")

			vectorCfg := cardanofw.GetMapFromInterfaceKey(mp, "directions", "vector")
			// remove cardano from map
			delete(vectorCfg["destChain"].(map[string]interface{}), "cardano")
		}, nil),
		cardanofw.WithBridgingAddrCnt(cardanofw.ChainIDPrime, bridgeAddrCnt),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	var (
		user             = apex.Users[0]
		sendAmount       = cardanofw.ApexToDfm(big.NewInt(2))
		bridgingRequests = []bridgingRequest{
			{src: cardanofw.ChainIDPrime, dest: cardanofw.ChainIDCardano, sender: apex.Users[1], srcTokenID: cardanofw.AP3XTokenID, isValid: true},
			{src: cardanofw.ChainIDVector, dest: cardanofw.ChainIDCardano, sender: apex.Users[2], srcTokenID: cardanofw.XADATokenID, isValid: false},
			{src: cardanofw.ChainIDCardano, dest: cardanofw.ChainIDVector, sender: apex.Users[1], srcTokenID: cardanofw.ADATokenID, isValid: false},
			{src: cardanofw.ChainIDCardano, dest: cardanofw.ChainIDPrime, sender: apex.Users[2], srcTokenID: cardanofw.CAP3XTokenID, isValid: true},
		}
		txHashes = make([]string, len(bridgingRequests))

		// map that contains initial balances of users that will receive refunds, per chains
		initialBalance = map[string]map[string]*big.Int{}

		err error
	)

	var wg sync.WaitGroup

	for idx, br := range bridgingRequests {
		wg.Add(1)

		go func(i int, br bridgingRequest) {
			defer wg.Done()

			tokenName := cardanofw.CAP3XTokenName
			if br.src == cardanofw.ChainIDVector {
				tokenName = cardanofw.XADATokenName
			}

			if br.srcTokenID != cardanofw.AP3XTokenID && br.srcTokenID != cardanofw.ADATokenID {
				token, err := cardanofw.FundUserWithToken(
					ctx, apex, br.src,
					apex.GetCardanoInfo(br.src).GenesisWallet, br.sender,
					tokenName, cardanofw.DefaultTokenMintAmount,
					uint64(10_000_000), uint64(100_000_000))
				require.NoError(t, err)

				fmt.Printf("Added new token for chain: %s. Token: %s\n", br.src, token.TokenName())
			}

			if !br.isValid {
				initialBalance[br.sender.GetAddress(br.src)], err = apex.GetBalance(ctx, br.sender, br.src)
				require.NoError(t, err)
			}

			txHashes[i], err = apex.SubmitBridgingRequest(cardanofw.SubmitBridgingRequestData{
				Context:          ctx,
				SourceChain:      br.src,
				DestinationChain: br.dest,
				Sender:           br.sender,
				DFMAmount:        sendAmount,
				SrcTokenID:       br.srcTokenID,
				Receivers:        []*cardanofw.TestApexUser{user},
			})
			require.NoError(t, err)

			fmt.Printf("Bridging request: %v to %v sent tokenID: %v. hash: %s\n", br.src, br.dest, br.srcTokenID, txHashes[i])
		}(idx, br)
	}

	wg.Wait()

	for idx, br := range bridgingRequests {
		wg.Add(1)

		go func(br bridgingRequest, txHash string) {
			defer wg.Done()

			if !br.isValid {
				isNativeToken := br.srcTokenID != cardanofw.AP3XTokenID && br.srcTokenID != cardanofw.ADATokenID
				userSpending := new(big.Int).Set(sendAmount)
				addr := br.sender.GetAddress(br.src)

				tokensInfo, err := apex.GetBridgingTokensInfo(br.src, br.dest, br.srcTokenID)
				require.NoError(t, err)

				tokenName := tokensInfo.SrcTokenName

				if !isNativeToken {
					userSpending.Add(userSpending, new(big.Int).SetUint64(apex.GetMinBridgingFee(br.src, isNativeToken)))
				}

				initialAmount := initialBalance[addr][tokenName]

				// minExpected = initial - (sendAmount + feeAmount)
				minExpectedAmount := new(big.Int).Sub(initialAmount, userSpending)

				require.NoError(t,
					apex.WaitForAmountInRange(ctx, br.sender, br.src, minExpectedAmount, initialAmount, 20, 30*time.Second, tokenName))
			} else {
				state, timeout := "ExecutedOnDestination", uint(60*8)

				_, err := cardanofw.WaitForRequestStates(ctx, apex, br.src, txHash, apiKey, []string{state}, timeout)
				require.NoError(t, err)

				fmt.Printf("%s is %s\n", txHash, state)
			}
		}(br, txHashes[idx])
	}

	wg.Wait()
}
