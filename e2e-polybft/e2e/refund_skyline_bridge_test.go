package e2e

import (
	"context"
	"fmt"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/e2e-polybft/cardanofw"
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
		}, nil, nil),
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

	primeTestConfig := newTestConfig(t, apex.Config.PrimeConfig, &apex.PrimeInfo, cardanofw.ChainIDCardano, "")

	cardanoPrimeTestConfig := newTestConfig(
		t, apex.Config.CardanoConfig, &apex.CardanoInfo, cardanofw.ChainIDPrime, cardanoToken.TokenName())

	vectorTestConfig := newTestConfig(
		t, apex.Config.VectorConfig, &apex.VectorInfo, cardanofw.ChainIDCardano, vectorToken.TokenName())

	cardanoVectorTestConfig := newTestConfig(t, apex.Config.CardanoConfig, &apex.CardanoInfo, cardanofw.ChainIDVector, "")

	fmt.Printf("Prime test config: %+v\n", primeTestConfig)
	fmt.Printf("Vector test config: %+v\n", vectorTestConfig)
	fmt.Printf("Cardano->Prime test config: %+v\n", cardanoPrimeTestConfig)
	fmt.Printf("Cardano->Vector test config: %+v\n", cardanoVectorTestConfig)

	t.Run("1.1 Prime -> Cardano - Mismatch submitted and receiver amounts", func(t *testing.T) {
		executeInvalidMismatchSendLovelaceAmount(t, ctx, apex, primeTestConfig, user, maxWaitTimeSec, retryDelaySec, cardanofw.BridgingTypeCurrencyOnSource, true, 0)
	})

	t.Run("1.2 Cardano -> Prime - Mismatch submitted and receiver amounts", func(t *testing.T) {
		executeInvalidMismatchSendLovelaceAmount(t, ctx, apex, cardanoPrimeTestConfig, user, maxWaitTimeSec, retryDelaySec, cardanofw.BridgingTypeWrappedTokenOnSource, true, 0)
	})

	t.Run("1.3 Cardano -> Vector - Mismatch submitted and receiver amounts", func(t *testing.T) {
		executeInvalidMismatchSendLovelaceAmount(t, ctx, apex, cardanoVectorTestConfig, user, maxWaitTimeSec, retryDelaySec, cardanofw.BridgingTypeCurrencyOnSource, true, 0)
	})

	t.Run("2.1 Prime -> Cardano - Multiple submitters mismatch submitted and receiver amounts", func(t *testing.T) {
		executeInvalidMismatchSendAmountMultipleInstances(t, ctx, apex, primeTestConfig, maxWaitTimeSec, retryDelaySec, cardanofw.BridgingTypeCurrencyOnSource, true, 0)
	})

	t.Run("2.2 Cardano -> Prime - Multiple submitters mismatch submitted and receiver amounts", func(t *testing.T) {
		executeInvalidMismatchSendAmountMultipleInstances(t, ctx, apex, cardanoPrimeTestConfig, maxWaitTimeSec, retryDelaySec, cardanofw.BridgingTypeWrappedTokenOnSource, true, 0)
	})

	t.Run("2.3 Vector -> Cardano - Multiple submitters mismatch submitted and receiver amounts", func(t *testing.T) {
		executeInvalidMismatchSendAmountMultipleInstances(t, ctx, apex, vectorTestConfig, maxWaitTimeSec, retryDelaySec, cardanofw.BridgingTypeWrappedTokenOnSource, true, 0)
	})

	t.Run("3.1 Prime -> Cardano - Multiple submitters mismatch submitted and receiver amounts parallel", func(t *testing.T) {
		executeInvalidMismatchSendAmountMultipleInstancesParalel(t, ctx, apex, primeTestConfig, maxWaitTimeSec, retryDelaySec, cardanofw.BridgingTypeCurrencyOnSource, true, 0)
	})

	t.Run("3.2 Cardano -> Prime - Multiple submitters mismatch submitted and receiver amounts parallel", func(t *testing.T) {
		executeInvalidMismatchSendAmountMultipleInstancesParalel(t, ctx, apex, cardanoPrimeTestConfig, maxWaitTimeSec, retryDelaySec, cardanofw.BridgingTypeWrappedTokenOnSource, true, 0)
	})

	t.Run("3.3 Cardano -> Vector - Multiple submitters mismatch submitted and receiver amounts parallel", func(t *testing.T) {
		executeInvalidMismatchSendAmountMultipleInstancesParalel(t, ctx, apex, cardanoVectorTestConfig, maxWaitTimeSec, retryDelaySec, cardanofw.BridgingTypeCurrencyOnSource, true, 0)
	})

	t.Run("4.1 Prime -> Cardano - Submitted invalid metadata - sliced off", func(t *testing.T) {
		executeInvalidMetadataSlicedOff(t, ctx, apex, primeTestConfig, cardanofw.BridgingTypeCurrencyOnSource, 0)
	})

	t.Run("4.2 Cardano -> Prime - Submitted invalid metadata - sliced off", func(t *testing.T) {
		executeInvalidMetadataSlicedOff(t, ctx, apex, cardanoPrimeTestConfig, cardanofw.BridgingTypeWrappedTokenOnSource, 0)
	})

	t.Run("4.3 Vector -> Cardano - Submitted invalid metadata - sliced off", func(t *testing.T) {
		executeInvalidMetadataSlicedOff(t, ctx, apex, vectorTestConfig, cardanofw.BridgingTypeWrappedTokenOnSource, 0)
	})

	t.Run("5.1 Prime -> Cardano - Submitted invalid metadata - wrong type", func(t *testing.T) {
		executeInvalidMetadataType(t, ctx, apex, primeTestConfig, user, maxWaitTimeSec, retryDelaySec, cardanofw.BridgingTypeCurrencyOnSource, true, 0)
	})

	t.Run("5.2 Cardano -> Prime - Submitted invalid metadata - wrong type", func(t *testing.T) {
		executeInvalidMetadataType(t, ctx, apex, cardanoPrimeTestConfig, user, maxWaitTimeSec, retryDelaySec, cardanofw.BridgingTypeWrappedTokenOnSource, true, 0)
	})

	t.Run("5.3 Cardano -> Vector - Submitted invalid metadata - wrong type", func(t *testing.T) {
		executeInvalidMetadataType(t, ctx, apex, cardanoVectorTestConfig, user, maxWaitTimeSec, retryDelaySec, cardanofw.BridgingTypeCurrencyOnSource, true, 0)
	})

	t.Run("6.1 Prime -> Cardano - Submitted invalid metadata - invalid sender", func(t *testing.T) {
		executeInvalidMetadataInvalidSender(t, ctx, apex, primeTestConfig, user, maxWaitTimeSec, cardanofw.BridgingTypeCurrencyOnSource, 0)
	})

	t.Run("6.2 Cardano -> Prime - Submitted invalid metadata - invalid sender", func(t *testing.T) {
		executeInvalidMetadataInvalidSender(t, ctx, apex, cardanoPrimeTestConfig, user, maxWaitTimeSec, cardanofw.BridgingTypeWrappedTokenOnSource, 0)
	})

	t.Run("6.3 Cardano -> Vector - Submitted invalid metadata - invalid sender", func(t *testing.T) {
		executeInvalidMetadataInvalidSender(t, ctx, apex, cardanoVectorTestConfig, user, maxWaitTimeSec, cardanofw.BridgingTypeCurrencyOnSource, 0)
	})

	t.Run("7.1 Prime -> Cardano - Submitted invalid metadata - invalid bridging fee", func(t *testing.T) {
		executeInvalidBridgingFee(t, ctx, apex, primeTestConfig, maxWaitTimeSec, retryDelaySec, cardanofw.BridgingTypeCurrencyOnSource, true, 0)
	})

	t.Run("7.2 Cardano -> Prime - Submitted invalid metadata - invalid bridging fee", func(t *testing.T) {
		executeInvalidBridgingFee(t, ctx, apex, cardanoPrimeTestConfig, maxWaitTimeSec, retryDelaySec, cardanofw.BridgingTypeWrappedTokenOnSource, true, 0)
	})

	t.Run("7.3 Vector -> Cardano - Submitted invalid metadata - invalid bridging fee", func(t *testing.T) {
		executeInvalidBridgingFee(t, ctx, apex, vectorTestConfig, maxWaitTimeSec, retryDelaySec, cardanofw.BridgingTypeWrappedTokenOnSource, true, 0)
	})

	t.Run("8.1 Prime -> Cardano - Submitted invalid metadata - invalid fee receiver address", func(t *testing.T) {
		executeInvalidFeeReceiverAddr(t, ctx, apex, primeTestConfig, maxWaitTimeSec, retryDelaySec, cardanofw.BridgingTypeCurrencyOnSource, true, 0)
	})

	t.Run("8.2 Cardano -> Prime - Submitted invalid metadata - invalid fee receiver address", func(t *testing.T) {
		executeInvalidFeeReceiverAddr(t, ctx, apex, cardanoPrimeTestConfig, maxWaitTimeSec, retryDelaySec, cardanofw.BridgingTypeWrappedTokenOnSource, true, 0)
	})

	t.Run("8.3 Cardano -> Vector - Submitted invalid metadata - invalid fee receiver address", func(t *testing.T) {
		executeInvalidFeeReceiverAddr(t, ctx, apex, cardanoVectorTestConfig, maxWaitTimeSec, retryDelaySec, cardanofw.BridgingTypeCurrencyOnSource, true, 0)
	})

	t.Run("9.1 Prime -> Cardano - Submitted invalid metadata - empty receivers", func(t *testing.T) {
		executeInvalidEmptyReceivers(t, ctx, apex, primeTestConfig, user, maxWaitTimeSec, retryDelaySec, cardanofw.BridgingTypeCurrencyOnSource, true, 0)
	})

	t.Run("9.2 Cardano -> Prime - Submitted invalid metadata - empty receivers", func(t *testing.T) {
		executeInvalidEmptyReceivers(t, ctx, apex, cardanoPrimeTestConfig, user, maxWaitTimeSec, retryDelaySec, cardanofw.BridgingTypeWrappedTokenOnSource, true, 0)
	})

	t.Run("9.3 Vector -> Cardano - Submitted invalid metadata - empty receivers", func(t *testing.T) {
		executeInvalidEmptyReceivers(t, ctx, apex, vectorTestConfig, user, maxWaitTimeSec, retryDelaySec, cardanofw.BridgingTypeWrappedTokenOnSource, true, 0)
	})

	t.Run("10. Submitted with unknown tokens to bridging addr", func(t *testing.T) {
		user := apex.Users[0]
		minterWallet, _ := user.GetCardanoWallet(cardanofw.ChainIDCardano)

		tokensFunded, err := cardanofw.FundUserWithToken(
			ctx, apex, cardanofw.ChainIDCardano,
			minterWallet, user,
			cardanofw.CAP3XTokenName, cardanofw.DefaultTokenMintAmount,
			uint64(1_500_000), uint64(1_000_000))
		require.NoError(t, err)

		executeInvalidSendNativeToken(t, ctx, apex, user, cardanoPrimeTestConfig, *tokensFunded, maxWaitTimeSec, retryDelaySec, true, 0, cardanofw.BridgingTypeWrappedTokenOnSource)
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

		executeInvalidMismatchSendNativeTokenAmount(t, ctx, apex, user, vectorTestConfig, *tokensFunded, maxWaitTimeSec, retryDelaySec, true, 0)
	})

	t.Run("12. vector -> cardano - currency on src", func(t *testing.T) {
		executeInvalidTokenDirection(t, ctx, apex, vectorTestConfig, user, maxWaitTimeSec, retryDelaySec, cardanofw.BridgingTypeCurrencyOnSource, true, 0)
	})
}

func TestE2E_SkylineRefund_NexusDest_ValidScenarios(t *testing.T) {
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
	vectorTestConfig := newTestConfig(
		t, apex.Config.VectorConfig, &apex.VectorInfo, cardanofw.ChainIDNexus, vectorToken.TokenName())

	fmt.Printf("User: %+v\n", user.GetAddress(cardanofw.ChainIDNexus))

	t.Run("1. Cardano -> Nexus - Mismatch submitted and receiver amounts", func(t *testing.T) {
		executeInvalidMismatchSendLovelaceAmount(t, ctx, apex, cardanoNexusTestConfig, user, maxWaitTimeSec, retryDelaySec, cardanofw.BridgingTypeCurrencyOnSource, true, 0)
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

		executeInvalidMismatchSendNativeTokenAmount(t, ctx, apex, user, vectorTestConfig, *tokensFunded, maxWaitTimeSec, retryDelaySec, true, 0)
	})

	t.Run("3. Cardano -> Nexus - Invalid destination - native token on source", func(t *testing.T) {
		executeInvalidTokenDirection(t, ctx, apex, cardanoNexusTestConfig, user, maxWaitTimeSec, retryDelaySec, cardanofw.BridgingTypeWrappedTokenOnSource, true, 0)
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
		}, nil, nil),
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

	primeTestConfig := newTestConfig(
		t, apex.Config.PrimeConfig, &apex.PrimeInfo, cardanofw.ChainIDCardano, "")

	cardanoTestConfig := newTestConfig(
		t, apex.Config.CardanoConfig, &apex.CardanoInfo, cardanofw.ChainIDPrime, cardanoToken.TokenName())

	fmt.Printf("Cardano test config: %+v\n", cardanoToken)

	t.Run("1. Prime -> Cardano - Mismatch submitted and receiver amounts", func(t *testing.T) {
		executeInvalidMismatchSendLovelaceAmount(t, ctx, apex, primeTestConfig, user, maxWaitTimeSec, retryDelaySec, cardanofw.BridgingTypeCurrencyOnSource, true, 1)
	})

	t.Run("2. Prime -> Cardano - Multiple submitters mismatch submitted and receiver amounts", func(t *testing.T) {
		executeInvalidMismatchSendAmountMultipleInstances(t, ctx, apex, primeTestConfig, maxWaitTimeSec, retryDelaySec, cardanofw.BridgingTypeCurrencyOnSource, true, 2)
	})

	t.Run("3. Prime -> Cardano - Multiple submitters mismatch submitted and receiver amounts parallel", func(t *testing.T) {
		executeInvalidMismatchSendAmountMultipleInstancesParalel(t, ctx, apex, primeTestConfig, maxWaitTimeSec, retryDelaySec, cardanofw.BridgingTypeCurrencyOnSource, true, 3)
	})

	t.Run("4. Prime -> Cardano - Submitted invalid metadata - sliced off", func(t *testing.T) {
		executeInvalidMetadataSlicedOff(t, ctx, apex, primeTestConfig, cardanofw.BridgingTypeCurrencyOnSource, 1)
	})

	t.Run("5. Prime -> Cardano - Submitted invalid metadata - wrong type", func(t *testing.T) {
		executeInvalidMetadataType(t, ctx, apex, primeTestConfig, user, maxWaitTimeSec, retryDelaySec, cardanofw.BridgingTypeCurrencyOnSource, true, 2)
	})

	t.Run("6. Prime -> Cardano - Submitted invalid metadata - invalid sender", func(t *testing.T) {
		executeInvalidMetadataInvalidSender(t, ctx, apex, primeTestConfig, user, maxWaitTimeSec, cardanofw.BridgingTypeCurrencyOnSource, 1)
	})

	t.Run("7. Prime -> Cardano - Submitted invalid metadata - invalid bridging fee", func(t *testing.T) {
		executeInvalidBridgingFee(t, ctx, apex, primeTestConfig, maxWaitTimeSec, retryDelaySec, cardanofw.BridgingTypeCurrencyOnSource, true, 2)
	})

	t.Run("8. Cardano -> Prime - Submitted invalid metadata - invalid fee receiver address", func(t *testing.T) {
		executeInvalidFeeReceiverAddr(t, ctx, apex, cardanoTestConfig, maxWaitTimeSec, retryDelaySec, cardanofw.BridgingTypeWrappedTokenOnSource, true, 0)
	})

	t.Run("9. Prime -> Cardano - Submitted invalid metadata - empty receivers", func(t *testing.T) {
		executeInvalidEmptyReceivers(t, ctx, apex, primeTestConfig, user, maxWaitTimeSec, retryDelaySec, cardanofw.BridgingTypeCurrencyOnSource, true, 1)
	})

	t.Run("10. Submitted with unknown tokens to bridging addr", func(t *testing.T) {
		user := apex.Users[0]
		minterWallet, _ := user.GetCardanoWallet(cardanofw.ChainIDCardano)

		tokensFunded, err := cardanofw.FundUserWithToken(
			ctx, apex, cardanofw.ChainIDCardano,
			minterWallet, user,
			cardanofw.CAP3XTokenName, cardanofw.DefaultTokenMintAmount,
			uint64(1_500_000), uint64(1_000_000))
		require.NoError(t, err)

		cardanoAddrAmounts, err := apex.GetBridgingAddressesTokenAmounts(ctx, cardanofw.ChainIDCardano)
		require.NoError(t, err)
		fmt.Println("Cardano multisig addresses amounts: ", cardanoAddrAmounts)

		executeInvalidSendNativeToken(t, ctx, apex, user, cardanoTestConfig, *tokensFunded, maxWaitTimeSec, retryDelaySec, true, 0, cardanofw.BridgingTypeCurrencyOnSource)

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

		executeInvalidMismatchSendNativeTokenAmount(t, ctx, apex, user, cardanoTestConfig, *tokensFunded, maxWaitTimeSec, retryDelaySec, true, 0)

		cardanoAddrAmounts, err = apex.GetBridgingAddressesTokenAmounts(ctx, cardanofw.ChainIDCardano)
		require.NoError(t, err)
		fmt.Println("Cardano multisig addresses amounts: ", cardanoAddrAmounts)
	})

	t.Run("12. Submitted tokens to bridging addr other than 0", func(t *testing.T) {
		user := apex.Users[0]

		cardanoAddrAmounts, err := apex.GetBridgingAddressesTokenAmounts(ctx, cardanofw.ChainIDCardano)
		require.NoError(t, err)
		fmt.Println("Cardano multisig addresses amounts: ", cardanoAddrAmounts)

		executeInvalidSendNativeToken(t, ctx, apex, user, cardanoTestConfig, *cardanoToken, maxWaitTimeSec, retryDelaySec, true, 2, cardanofw.BridgingTypeWrappedTokenOnSource)

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
		}, nil, nil),
		cardanofw.WithBridgingAddrCnt(cardanofw.ChainIDPrime, bridgeAddrCnt),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	var (
		user             = apex.Users[0]
		apexSendAmount   = cardanofw.ApexToDfm(big.NewInt(10))
		bridgingRequests = []struct {
			src    string
			dest   string
			sender *cardanofw.TestApexUser
		}{
			{src: cardanofw.ChainIDPrime, dest: cardanofw.ChainIDCardano, sender: apex.Users[0]},
			{src: cardanofw.ChainIDCardano, dest: cardanofw.ChainIDVector, sender: apex.Users[0]},
		}
		txHashes = make([]string, len(bridgingRequests))
	)

	var wg sync.WaitGroup

	beforeSendingAmountDfm := make([]map[string]*big.Int, len(bridgingRequests))

	for idx, br := range bridgingRequests {
		wg.Add(1)

		go func(i int, src string, dest string, sender *cardanofw.TestApexUser) {
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
				BridgingType:     cardanofw.BridgingTypeCurrencyOnSource,
				Receivers:        []*cardanofw.TestApexUser{user},
			})
			require.NoError(t, err)

			fmt.Printf("Bridging request: %v to %v sent. hash: %s\n", src, dest, txHashes[i])
		}(idx, br.src, br.dest, br.sender)
	}

	wg.Wait()

	for idx, br := range bridgingRequests {
		wg.Add(1)

		go func() {
			defer wg.Done()

			lowerBoundaryDfm := new(big.Int).Sub(
				beforeSendingAmountDfm[idx]["lovelace"],
				new(big.Int).Add(apexSendAmount, new(big.Int).SetUint64(apex.GetMinBridgingFee(br.src, false))))

			fmt.Printf("Tx sent. hash: %s, lowerBoundaryDfm: %d, higherBoundaryDfm: %+v\n", txHashes[idx], lowerBoundaryDfm, beforeSendingAmountDfm[idx]["lovelace"])

			err := apex.WaitForAmountInRange(ctx, user, br.src, br.dest, lowerBoundaryDfm, beforeSendingAmountDfm[idx]["lovelace"], 20, time.Second*30, "lovelace")
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
		}, nil, nil),
		cardanofw.WithBridgingAddrCnt(cardanofw.ChainIDPrime, bridgeAddrCnt),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	var (
		user             = apex.Users[0]
		apexSendAmount   = cardanofw.ApexToDfm(big.NewInt(10))
		bridgingRequests = []struct {
			src    string
			dest   string
			sender *cardanofw.TestApexUser
		}{
			{src: cardanofw.ChainIDVector, dest: cardanofw.ChainIDCardano, sender: apex.Users[0]},
			{src: cardanofw.ChainIDCardano, dest: cardanofw.ChainIDPrime, sender: apex.Users[0]},
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

		go func(i int, src, dest string, sender *cardanofw.TestApexUser) {
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
					BridgingType:     cardanofw.BridgingTypeWrappedTokenOnSource,
					Receivers:        []*cardanofw.TestApexUser{user},
				})
			require.NoError(t, err)

			fmt.Printf("Bridging request: %v to %v sent. hash: %s\n", src, dest, txHash)
		}(idx, br.src, br.dest, br.sender)
	}

	wg.Wait()

	for _, br := range bridgingRequests {
		wg.Add(1)

		go func(src, dest string, sender *cardanofw.TestApexUser) {
			defer wg.Done()

			tokenID := apex.GetTokenIDForChain(src, false)
			tokenName := apex.GetTokenNameForChain(src, tokenID)

			mu.RLock()
			tokenBalance := initialBalances[br.src][tokenName]
			mu.RUnlock()

			err := apex.WaitForExactAmount(ctx, br.sender, br.src, br.dest, tokenBalance, 30, 30*time.Second, tokenName)
			require.NoError(t, err)
		}(br.src, br.dest, br.sender)
	}

	wg.Wait()
}

func TestE2E_SkylineRefund_DisabledDirection(t *testing.T) {
	if cardanofw.ShouldSkipE2RRedundantTests() {
		t.Skip()
	}

	type bridgingRequest struct {
		src         string
		dest        string
		sender      *cardanofw.TestApexUser
		requestType cardanofw.BridgingType
		isValid     bool
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
		}),
		cardanofw.WithBridgingAddrCnt(cardanofw.ChainIDPrime, bridgeAddrCnt),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	var (
		user             = apex.Users[0]
		sendAmount       = cardanofw.ApexToDfm(big.NewInt(2))
		bridgingRequests = []bridgingRequest{
			{src: cardanofw.ChainIDPrime, dest: cardanofw.ChainIDCardano, sender: apex.Users[1], requestType: cardanofw.BridgingTypeCurrencyOnSource, isValid: true},
			{src: cardanofw.ChainIDVector, dest: cardanofw.ChainIDCardano, sender: apex.Users[2], requestType: cardanofw.BridgingTypeWrappedTokenOnSource, isValid: false},
			{src: cardanofw.ChainIDCardano, dest: cardanofw.ChainIDVector, sender: apex.Users[1], requestType: cardanofw.BridgingTypeCurrencyOnSource, isValid: false},
			{src: cardanofw.ChainIDCardano, dest: cardanofw.ChainIDPrime, sender: apex.Users[2], requestType: cardanofw.BridgingTypeWrappedTokenOnSource, isValid: true},
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

			if br.requestType == cardanofw.BridgingTypeWrappedTokenOnSource {
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
				BridgingType:     br.requestType,
				Receivers:        []*cardanofw.TestApexUser{user},
			})
			require.NoError(t, err)

			fmt.Printf("Bridging request: %v to %v sent %v. hash: %s\n", br.src, br.dest, br.requestType, txHashes[i])
		}(idx, br)
	}

	wg.Wait()

	for idx, br := range bridgingRequests {
		wg.Add(1)

		go func(br bridgingRequest, txHash string) {
			defer wg.Done()

			if !br.isValid {
				isNativeToken := br.requestType != cardanofw.BridgingTypeCurrencyOnSource
				userSpending := new(big.Int).Set(sendAmount)
				addr := br.sender.GetAddress(br.src)

				// reversed
				tokenID := apex.GetTokenIDForChain(br.dest, !(br.requestType == cardanofw.BridgingTypeCurrencyOnSource))
				tokenName := apex.GetTokenNameForChains(br.src, br.dest, tokenID)

				if br.requestType == cardanofw.BridgingTypeCurrencyOnSource {
					userSpending.Add(userSpending, new(big.Int).SetUint64(apex.GetMinBridgingFee(br.src, isNativeToken)))
				}

				initialAmount := initialBalance[addr][tokenName]

				// minExpected = initial - (sendAmount + feeAmount)
				minExpectedAmount := new(big.Int).Sub(initialAmount, userSpending)

				require.NoError(t,
					apex.WaitForAmountInRange(ctx, br.sender, br.src, br.dest, minExpectedAmount, initialAmount, 20, 30*time.Second, tokenName))
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
