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

func Test_SkylineBridgeMint_ValidScenarios(t *testing.T) {
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
	fundingPassed := t.Run("3. Nexus -> Vector USDT, Vector -> Nexus xADA funding test", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() {
			t.Skip()
		}

		nexusChain := apex.GetChainMust(t, cardanofw.ChainIDNexus).(*cardanofw.TestEVMChain)
		vectorChain := apex.GetChainMust(t, cardanofw.ChainIDVector).(*cardanofw.TestCardanoChain)

		var wg sync.WaitGroup

		err := cardanofw.MintToken(vectorChain, apex.VectorInfo.GenesisWallet, cardanofw.XADATokenName, uint64(len(apex.Users)*100_000_000))
		require.NoError(t, err)

		_, err = cardanofw.FundUsersWithToken(
			ctx, vectorChain, apex.VectorInfo.GenesisWallet,
			apex.Users, cardanofw.XADATokenName, 100_000_000, 100_000_000)
		require.NoError(t, err)

		for _, user := range apex.Users {
			err := nexusChain.FundUsersWithToken(
				user.GetAddress(cardanofw.ChainIDNexus),
				big.NewInt(100_000_000),
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
					t, ctx, apex, user, user, cardanofw.ChainIDVector, cardanofw.ChainIDNexus, big.NewInt(100_000_000),
					cardanofw.BridgingTypeWrappedTokenOnSource, e2ehelper.WithColoredCoins([]uint16{cardanofw.XADATokenID}))
			}()
		}

		wg.Wait()
	})

	t.Run("4. Parallel bridging tests", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() || !fundingPassed {
			t.Skip()
		}

		const (
			sendAmount = uint64(1_000_000)
			instances  = 5
		)

		for idx, br := range bridgingRequests {
			fmt.Printf("4.%d %s -> %s - %s\n", idx+1, br.src, br.dest, br.requestType)

			if br.requestType != cardanofw.BridgingTypeCurrencyOnSource {
				fundTestUsersWithTokenID(
					t, ctx, apex, []*testConfig{
						{
							srcChainID:      br.src,
							srcMinterWallet: br.srcMinterWallet,
						},
					}, apex.Users[:instances],
					uint64(100_000_000), sendAmount, br.tokenID)
			}

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

	t.Run("5. Sequential bridging tests", func(t *testing.T) {
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
			fmt.Printf("5.%d %s -> %s - %s\n", idx+1, br.src, br.dest, br.requestType)

			if br.requestType != cardanofw.BridgingTypeCurrencyOnSource {
				fundTestUsersWithTokenID(
					t, ctx, apex, []*testConfig{
						{
							srcChainID:      br.src,
							srcMinterWallet: br.srcMinterWallet,
						},
					}, apex.Users[:parallelInstances],
					uint64(100_000_000), sendAmount*sequentialInstances, br.tokenID)
			}

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

	t.Run("6. All directions parallel", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() || !fundingPassed {
			t.Skip()
		}

		const (
			sendAmount = uint64(1_000_000)
			instances  = 5
		)

		for _, br := range bridgingRequests {
			if br.requestType != cardanofw.BridgingTypeCurrencyOnSource {
				fundTestUsersWithTokenID(
					t, ctx, apex, []*testConfig{
						{
							srcChainID:      br.src,
							srcMinterWallet: br.srcMinterWallet,
						},
					}, apex.Users[:instances],
					uint64(100_000_000), sendAmount, br.tokenID)
			}
		}

		e2ehelper.ExecuteBridgingExtended(
			t, ctx, apex, 1,
			apex.Users[:instances],
			[]*cardanofw.TestApexUser{user},
			getBridgingRequests(bridgingRequests),
			new(big.Int).SetUint64(sendAmount),
		)
	})

	t.Run("7. All directions sequential and parallel", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() || !fundingPassed {
			t.Skip()
		}

		const (
			sendAmount = uint64(1_000_000)
			instances  = 5
		)

		for _, br := range bridgingRequests {
			if br.requestType != cardanofw.BridgingTypeCurrencyOnSource {
				multiplier := uint64(1)
				if br.src == cardanofw.ChainIDNexus {
					multiplier = instances
				}

				fundTestUsersWithTokenID(
					t, ctx, apex, []*testConfig{
						{
							srcChainID:      br.src,
							srcMinterWallet: br.srcMinterWallet,
						},
					}, apex.Users[:instances],
					uint64(100_000_000), sendAmount*multiplier, br.tokenID)
			}
		}

		e2ehelper.ExecuteBridgingExtended(
			t, ctx, apex, instances,
			apex.Users[:instances],
			[]*cardanofw.TestApexUser{user},
			getBridgingRequests(bridgingRequests),
			new(big.Int).SetUint64(sendAmount),
		)
	})
}

func fundTestUsersWithTokenID(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem,
	testConfig []*testConfig, testApexUser []*cardanofw.TestApexUser,
	currenctAmnt, tokenAmnt uint64, tokenID uint16) {

	fmt.Printf("Funding test users on chain: %v with token ID: %d\n", testConfig, tokenID)

	for _, cfg := range testConfig {
		if cfg.srcChainID == cardanofw.ChainIDVector && tokenID == cardanofw.USDTTokenID {
			fmt.Printf("Skipping funding test users on chain: %v with token ID: %d\n", cfg, tokenID)
			continue
		}

		if cfg.srcChainID == cardanofw.ChainIDNexus && tokenID == cardanofw.XADATokenID {
			fmt.Printf("Skipping funding test users on chain: %v with token ID: %d\n", cfg, tokenID)
			continue
		}

		fmt.Printf("Funding test users on chain: %v with token ID: %d\n", cfg, tokenID)

		if cfg.srcChainID == cardanofw.ChainIDNexus {
			nexusChain := apex.GetChainMust(t, cardanofw.ChainIDNexus).(*cardanofw.TestEVMChain)

			for _, user := range testApexUser {
				err := nexusChain.FundUsersWithToken(
					user.GetAddress(cardanofw.ChainIDNexus),
					big.NewInt(0).SetUint64(tokenAmnt),
					tokenID,
				)
				require.NoError(t, err)
			}
		} else {
			cardanoChain := apex.GetChainMust(t, testConfig[0].srcChainID).(*cardanofw.TestCardanoChain)
			tokenName := apex.GetHumanReadableTokenNameForChain(tokenID)
			err := cardanofw.MintToken(
				cardanoChain, cfg.srcMinterWallet, tokenName, tokenAmnt*uint64(len(testApexUser)))
			require.NoError(t, err)

			_, err = cardanofw.FundUsersWithToken(
				ctx, cardanoChain, cfg.srcMinterWallet,
				testApexUser, tokenName, currenctAmnt, tokenAmnt)
			require.NoError(t, err)
		}
	}
}
