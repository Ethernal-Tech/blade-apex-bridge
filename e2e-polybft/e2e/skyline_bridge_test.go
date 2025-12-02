package e2e

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"math/rand"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/e2e-polybft/cardanofw"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/e2ehelper"
	"github.com/0xPolygon/polygon-edge/helper/hex"
	infracommon "github.com/Ethernal-Tech/cardano-infrastructure/common"
	"github.com/Ethernal-Tech/cardano-infrastructure/sendtx"
	"github.com/Ethernal-Tech/cardano-infrastructure/wallet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const bridgeAddrCnt = 4

// cd e2e-polybft/e2e
// ONLY_RUN_SKYLINE_BRIDGE=true go test -v -timeout 0 -run ^Test_OnlyRunSkylineBridge$ github.com/0xPolygon/polygon-edge/e2e-polybft/e2e
func Test_OnlyRunSkylineBridge(t *testing.T) {
	if !cardanofw.IsEnvVarTrue("ONLY_RUN_SKYLINE_BRIDGE") {
		t.Skip()
	}

	const (
		apiKey = "test_api_key"
	)

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	primeConfig, cardanoConfig := cardanofw.NewPrimeChainConfig(), cardanofw.NewCardanoChainConfig(true)
	primeConfig.FundTokenAmount = 1_000_000_000
	cardanoConfig.FundTokenAmount = 1_000_000_000

	vectorConfig := cardanofw.NewVectorChainConfig()
	vectorConfig.FundTokenAmount = 1_000_000_000

	apex := cardanofw.SetupAndRunSkylineBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithCardanoConfig(cardanoConfig),
		cardanofw.WithPrimeConfig(primeConfig),
		cardanofw.WithVectorConfig(vectorConfig),
		cardanofw.WithUserCnt(1),
		cardanofw.WithBridgingAddrCnt(cardanofw.ChainIDPrime, bridgeAddrCnt),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	oracleAPI, err := apex.GetBridgingAPI()
	require.NoError(t, err)

	fmt.Printf("oracle API: %s\n", oracleAPI)
	fmt.Printf("oracle API key: %s\n", apiKey)

	fmt.Printf("prime network url: %s\n", apex.PrimeInfo.NetworkAddress)
	fmt.Printf("prime ogmios url: %s\n", apex.PrimeInfo.OgmiosURL)
	fmt.Printf("prime bridging addr: %s\n", apex.PrimeInfo.MultisigAddr[0])
	fmt.Printf("prime fee addr: %s\n", apex.PrimeInfo.FeeAddr)
	fmt.Printf("prime socket path: %s\n", apex.PrimeInfo.SocketPath)

	fmt.Printf("cardano network url: %s\n", apex.CardanoInfo.NetworkAddress)
	fmt.Printf("cardano ogmios url: %s\n", apex.CardanoInfo.OgmiosURL)
	fmt.Printf("cardano bridging addr: %s\n", apex.CardanoInfo.MultisigAddr[0])
	fmt.Printf("cardano fee addr: %s\n", apex.CardanoInfo.FeeAddr)
	fmt.Printf("cardano socket path: %s\n", apex.CardanoInfo.SocketPath)

	user := apex.Users[0]
	userPrimeSK, err := user.GetPrivateKey(cardanofw.ChainIDPrime)
	require.NoError(t, err)
	userCardanoSK, err := user.GetPrivateKey(cardanofw.ChainIDCardano)
	require.NoError(t, err)

	fmt.Printf("user prime addr: %s\n", user.GetAddress(cardanofw.ChainIDPrime))
	fmt.Printf("user prime signing key hex: %s\n", userPrimeSK)
	fmt.Printf("user cardano addr: %s\n", user.GetAddress(cardanofw.ChainIDCardano))
	fmt.Printf("user cardano signing key hex: %s\n", userCardanoSK)

	proxyAdminPrivateKeyRaw, err := apex.GetBridgeProxyAdmin().MarshallPrivateKey()
	require.NoError(t, err)

	privateKeyRaw, err := apex.GetBridgeAdmin().MarshallPrivateKey()
	require.NoError(t, err)

	fmt.Printf("bridge url: %s\n", apex.GetBridgeDefaultJSONRPCAddr())
	fmt.Printf("bridge admin key: %s\n", hex.EncodeToString(privateKeyRaw))
	fmt.Printf("bridge admin address: %s\n", apex.GetBridgeAdmin().Address())
	fmt.Printf("bridge proxy admin key: %s\n", hex.EncodeToString(proxyAdminPrivateKeyRaw))
	fmt.Printf("bridge proxy admin address: %s\n", apex.GetBridgeProxyAdmin().Address())

	for i := 0; i < apex.GetValidatorsCount(); i++ {
		fmt.Printf("validator %d `--telemetry` flag telemetry url(s): %s\n",
			i+1, apex.Config.GetTelemetryForValidatorIdx(i))
	}

	signalChannel := make(chan os.Signal, 1)
	// Notify the signalChannel when the interrupt signal is received (Ctrl+C)
	signal.Notify(signalChannel, os.Interrupt, syscall.SIGTERM)

	<-signalChannel
}

func TestE2E_SkylineBridge_ValidScenarios(t *testing.T) {
	type bridgingRequest struct {
		src             string
		dest            string
		sender          *cardanofw.TestApexUser
		requestType     sendtx.BridgingType
		isValid         bool
		srcMinterWallet *wallet.Wallet
	}

	const (
		apiKey  = "test_api_key"
		userCnt = 15
	)

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	primeConfig, cardanoConfig := cardanofw.NewPrimeChainConfig(), cardanofw.NewCardanoChainConfig(true)
	cardanoConfig.FundTokenAmount = 1_000_000_000
	primeConfig.UseIndexer = true
	cardanoConfig.UseIndexer = true

	vectorConfig := cardanofw.NewVectorChainConfig()
	vectorConfig.FundTokenAmount = 1_000_000_000
	vectorConfig.UseIndexer = true

	apex := cardanofw.SetupAndRunSkylineBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithUserCnt(userCnt),
		cardanofw.WithCardanoConfig(cardanoConfig),
		cardanofw.WithPrimeConfig(primeConfig),
		cardanofw.WithVectorConfig(vectorConfig),
		cardanofw.WithBridgingAddrCnt(cardanofw.ChainIDPrime, bridgeAddrCnt),
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
	fmt.Println("vector user addr: ", user.VectorAddress)
	fmt.Println("vector multisig addr: ", apex.VectorInfo.MultisigAddr)
	fmt.Println("vector fee addr: ", apex.VectorInfo.FeeAddr)
	fmt.Printf("vector socket path: %s\n", apex.VectorInfo.SocketPath)

	var (
		bridgingRequests = []bridgingRequest{
			{src: cardanofw.ChainIDPrime, dest: cardanofw.ChainIDCardano, sender: apex.Users[1], requestType: sendtx.BridgingTypeCurrencyOnSource, isValid: true, srcMinterWallet: apex.PrimeInfo.GenesisWallet},
			{src: cardanofw.ChainIDVector, dest: cardanofw.ChainIDCardano, sender: apex.Users[2], requestType: sendtx.BridgingTypeWrappedTokenOnSource, isValid: true, srcMinterWallet: apex.VectorInfo.GenesisWallet},
			{src: cardanofw.ChainIDCardano, dest: cardanofw.ChainIDVector, sender: apex.Users[3], requestType: sendtx.BridgingTypeCurrencyOnSource, isValid: true, srcMinterWallet: apex.CardanoInfo.GenesisWallet},
			{src: cardanofw.ChainIDCardano, dest: cardanofw.ChainIDPrime, sender: apex.Users[4], requestType: sendtx.BridgingTypeWrappedTokenOnSource, isValid: true, srcMinterWallet: apex.CardanoInfo.GenesisWallet},
		}
	)

	testConfigPrime := newTestConfig(t, apex.Config.PrimeConfig, &apex.PrimeInfo, cardanofw.ChainIDCardano, "")
	testConfigCardano := newTestConfig(t, apex.Config.CardanoConfig, &apex.CardanoInfo, cardanofw.ChainIDPrime, "")
	testConfigs := []*testConfig{testConfigPrime, testConfigCardano}
	minterWalletCardano := apex.CardanoInfo.GenesisWallet
	minterWalletVector := apex.VectorInfo.GenesisWallet

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

	t.Run("2. vector -> cardano - native token on src", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() {
			t.Skip()
		}

		t.Cleanup(func() {
			apex.ResetIndexers()
		})

		sendAmountDfm := big.NewInt(1_500_000)

		brSubmitterUser, err := cardanofw.NewTestApexUser(cardanofw.NewApexNetworkTypesFromSystem(apex))
		require.NoError(t, err)

		_, err = cardanofw.FundUserWithToken(
			ctx, apex, cardanofw.ChainIDVector,
			minterWalletVector, brSubmitterUser,
			cardanofw.XADATokenName, cardanofw.DefaultTokenMintAmount,
			uint64(1_100_000_000), uint64(2_500_000))
		require.NoError(t, err)

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, brSubmitterUser, user, cardanofw.ChainIDVector, cardanofw.ChainIDCardano, sendAmountDfm,
			sendtx.BridgingTypeWrappedTokenOnSource)
	})

	t.Run("3. cardano -> vector - currency on src", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() {
			t.Skip()
		}

		t.Cleanup(func() {
			apex.ResetIndexers()
		})

		sendAmountDfm := big.NewInt(1_500_000)

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDCardano, cardanofw.ChainIDVector, sendAmountDfm,
			sendtx.BridgingTypeCurrencyOnSource)
	})

	t.Run("4. cardano -> prime - native token on src", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() {
			t.Skip()
		}

		t.Cleanup(func() {
			apex.ResetIndexers()
		})

		sendAmountDfm := big.NewInt(1_500_000)

		brSubmitterUser, err := cardanofw.NewTestApexUser(cardanofw.NewApexNetworkTypesFromSystem(apex))
		require.NoError(t, err)

		_, err = cardanofw.FundUserWithToken(
			ctx, apex, cardanofw.ChainIDCardano,
			minterWalletCardano, brSubmitterUser,
			cardanofw.CAP3XTokenName, cardanofw.DefaultTokenMintAmount,
			uint64(1_100_000_000), uint64(2_500_000))
		require.NoError(t, err)

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, brSubmitterUser, user, cardanofw.ChainIDCardano, cardanofw.ChainIDPrime, sendAmountDfm,
			sendtx.BridgingTypeWrappedTokenOnSource)
	})

	t.Run("5. Submitter has tokens", func(t *testing.T) {
		for idx, br := range bridgingRequests {
			fmt.Printf("5.%d %s -> %s - %s", idx+1, br.src, br.dest, br.requestType)

			if cardanofw.ShouldSkipE2RRedundantTests() {
				t.Skip()
			}

			t.Cleanup(func() {
				apex.ResetIndexers()
			})

			sendAmountDfm := big.NewInt(1_000_000)
			minterUser := apex.Users[userCnt-2]

			brSubmitterUser, err := cardanofw.NewTestApexUser(cardanofw.NewApexNetworkTypesFromSystem(apex))
			require.NoError(t, err)

			minterWallet, _ := minterUser.GetCardanoWallet(br.src)

			fundTestUsersWithToken(
				t, ctx, apex, []*testConfig{
					{
						srcChainID:      br.src,
						srcMinterWallet: minterWallet,
					},
					{
						srcChainID:      br.src,
						srcMinterWallet: br.srcMinterWallet,
					},
				}, []*cardanofw.TestApexUser{brSubmitterUser},
				uint64(50_000_000), uint64(10_000_000))

			e2ehelper.ExecuteSingleBridging(
				t, ctx, apex, brSubmitterUser, br.sender, br.src, br.dest, sendAmountDfm,
				br.requestType)
		}
	})

	//nolint:dupl
	t.Run("6. Wait for each submit", func(t *testing.T) {
		for idx, br := range bridgingRequests {
			fmt.Printf("6.%d %s -> %s - %s\n", idx+1, br.src, br.dest, br.requestType)

			if cardanofw.ShouldSkipE2RRedundantTests() {
				t.Skip()
			}

			t.Cleanup(func() {
				apex.ResetIndexers()
			})

			const (
				sendAmount = uint64(1_000_000)
				instances  = 5
			)

			brSubmitterUser, err := cardanofw.NewTestApexUser(cardanofw.NewApexNetworkTypesFromSystem(apex))
			require.NoError(t, err)

			tokenName := cardanofw.CAP3XTokenName
			if br.src == cardanofw.ChainIDVector {
				tokenName = cardanofw.XADATokenName
			}

			_, err = cardanofw.FundUserWithToken(
				ctx, apex, br.src,
				br.srcMinterWallet, brSubmitterUser,
				tokenName, cardanofw.DefaultTokenMintAmount,
				uint64(10_100_000_000), uint64(10_000_000))
			require.NoError(t, err)

			e2ehelper.ExecuteBridgingOneByOneWaitOnOtherSide(
				t, ctx, apex, instances, brSubmitterUser, br.src, br.dest, new(big.Int).SetUint64(sendAmount),
				br.requestType)
		}
	})

	//nolint:dupl
	t.Run("7. One by one", func(t *testing.T) {
		for idx, br := range bridgingRequests {
			fmt.Printf("7.%d %s -> %s - %s\n", idx+1, br.src, br.dest, br.requestType)

			if cardanofw.ShouldSkipE2RRedundantTests() {
				t.Skip()
			}

			t.Cleanup(func() {
				apex.ResetIndexers()
			})

			const (
				sendAmount = uint64(1_000_000)
				instances  = 5
			)

			brSubmitterUser, err := cardanofw.NewTestApexUser(cardanofw.NewApexNetworkTypesFromSystem(apex))
			require.NoError(t, err)

			tokenName := cardanofw.CAP3XTokenName
			if br.src == cardanofw.ChainIDVector {
				tokenName = cardanofw.XADATokenName
			}

			_, err = cardanofw.FundUserWithToken(
				ctx, apex, br.src,
				br.srcMinterWallet, brSubmitterUser,
				tokenName, cardanofw.DefaultTokenMintAmount,
				uint64(1_100_000_000), uint64(10_000_000))
			require.NoError(t, err)

			e2ehelper.ExecuteBridgingWaitAfterSubmits(
				t, ctx, apex, instances, brSubmitterUser, br.src, br.dest, new(big.Int).SetUint64(sendAmount),
				br.requestType)
		}
	})
	t.Run("8. Parallel", func(t *testing.T) {
		for idx, br := range bridgingRequests {
			fmt.Printf("8.%d %s -> %s - %s\n", idx+1, br.src, br.dest, br.requestType)

			if cardanofw.ShouldSkipE2RRedundantTests() {
				t.Skip()
			}

			t.Cleanup(func() {
				apex.ResetIndexers()
			})

			const (
				sendAmount = uint64(1_000_000)
				instances  = 5
			)

			if br.requestType == sendtx.BridgingTypeWrappedTokenOnSource {
				fundTestUsersWithToken(
					t, ctx, apex, []*testConfig{
						{
							srcChainID:      br.src,
							srcMinterWallet: br.srcMinterWallet,
						},
					}, apex.Users[:instances], uint64(1_100_000_000), uint64(210_000_000))
			}

			e2ehelper.ExecuteBridging(
				t, ctx, apex, 1, apex.Users[:instances], []*cardanofw.TestApexUser{user},
				[]string{br.src},
				map[string][]string{
					br.src: {br.dest},
				},
				map[e2ehelper.SrcDstChainPair]sendtx.BridgingType{
					e2ehelper.NewChainPair(br.src, br.dest): br.requestType,
				},
				new(big.Int).SetUint64(sendAmount))
		}
	})

	t.Run("9. Sequential and parallel", func(t *testing.T) {
		for idx, br := range bridgingRequests {
			fmt.Printf("9.%d %s -> %s - %s\n", idx+1, br.src, br.dest, br.requestType)

			if cardanofw.ShouldSkipE2RRedundantTests() {
				t.Skip()
			}

			t.Cleanup(func() {
				apex.ResetIndexers()
			})

			const (
				sendAmount          = uint64(1_000_000)
				sequentialInstances = 5
				parallelInstances   = 10
				receivers           = 1
			)

			if br.requestType == sendtx.BridgingTypeWrappedTokenOnSource {
				fundTestUsersWithToken(
					t, ctx, apex, []*testConfig{
						{
							srcChainID:      br.src,
							srcMinterWallet: br.srcMinterWallet,
						},
					}, apex.Users[:parallelInstances], uint64(1_100_000_000), uint64(210_000_000))
			}

			e2ehelper.ExecuteBridging(
				t, ctx, apex, sequentialInstances,
				apex.Users[:parallelInstances],
				apex.Users[:receivers],
				[]string{br.src},
				map[string][]string{
					br.src: {br.dest},
				},
				map[e2ehelper.SrcDstChainPair]sendtx.BridgingType{
					e2ehelper.NewChainPair(br.src, br.dest): br.requestType,
				},
				new(big.Int).SetUint64(sendAmount),
			)
		}
	})
	t.Run("10. Both directions sequential", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() {
			t.Skip()
		}

		t.Cleanup(func() {
			apex.ResetIndexers()
		})

		const (
			sendAmount = uint64(1_000_000)
			instances  = 5
		)

		brSubmitterUser, err := cardanofw.NewTestApexUser(cardanofw.NewApexNetworkTypesFromSystem(apex))
		require.NoError(t, err)

		fundTestUsersWithToken(
			t, ctx, apex, testConfigs, []*cardanofw.TestApexUser{brSubmitterUser},
			uint64(1_100_000_000), uint64(10_000_000))

		e2ehelper.ExecuteBridging(
			t, ctx, apex, instances,
			[]*cardanofw.TestApexUser{brSubmitterUser},
			[]*cardanofw.TestApexUser{user},
			[]string{cardanofw.ChainIDPrime, cardanofw.ChainIDCardano},
			map[string][]string{
				cardanofw.ChainIDPrime:   {cardanofw.ChainIDCardano},
				cardanofw.ChainIDCardano: {cardanofw.ChainIDPrime},
			},
			map[e2ehelper.SrcDstChainPair]sendtx.BridgingType{
				e2ehelper.NewChainPair(cardanofw.ChainIDPrime, cardanofw.ChainIDCardano): sendtx.BridgingTypeCurrencyOnSource,
				e2ehelper.NewChainPair(cardanofw.ChainIDCardano, cardanofw.ChainIDPrime): sendtx.BridgingTypeWrappedTokenOnSource,
			},
			new(big.Int).SetUint64(sendAmount),
		)
	})

	t.Run("11. Both directions sequential and parallel", func(t *testing.T) {
		t.Cleanup(func() {
			apex.ResetIndexers()
		})

		const (
			sendAmount          = uint64(1_000_000)
			sequentialInstances = 5
			parallelInstances   = 6
		)

		fundTestUsersWithToken(
			t, ctx, apex, testConfigs, apex.Users[:parallelInstances],
			uint64(1_100_000_000), uint64(10_000_000))

		e2ehelper.ExecuteBridging(
			t, ctx, apex, sequentialInstances,
			apex.Users[:parallelInstances],
			[]*cardanofw.TestApexUser{user},
			[]string{cardanofw.ChainIDPrime, cardanofw.ChainIDCardano},
			map[string][]string{
				cardanofw.ChainIDPrime:   {cardanofw.ChainIDCardano},
				cardanofw.ChainIDCardano: {cardanofw.ChainIDPrime},
			},
			map[e2ehelper.SrcDstChainPair]sendtx.BridgingType{
				e2ehelper.NewChainPair(cardanofw.ChainIDPrime, cardanofw.ChainIDCardano): sendtx.BridgingTypeCurrencyOnSource,
				e2ehelper.NewChainPair(cardanofw.ChainIDCardano, cardanofw.ChainIDPrime): sendtx.BridgingTypeWrappedTokenOnSource,
			},
			new(big.Int).SetUint64(sendAmount),
			e2ehelper.WithWaitForUnexpectedBridges(true))
	})

	t.Run("12. Both directions sequential and parallel - one node goes offline midway", func(t *testing.T) {
		const (
			sendAmount           = uint64(1_000_000)
			sequentialInstances  = 5
			parallelInstances    = 6
			stopAfter            = time.Second * 60
			validatorStoppingIdx = 1
		)

		t.Cleanup(func() {
			apex.ResetIndexers()

			_ = apex.GetValidator(t, validatorStoppingIdx).Stop() // make sure it was stopped
			require.NoError(t, apex.GetValidator(t, validatorStoppingIdx).Start(ctx, false))
		})

		fundTestUsersWithToken(
			t, ctx, apex, testConfigs, apex.Users[:parallelInstances],
			uint64(1_100_000_000), uint64(10_000_000))

		e2ehelper.ExecuteBridging(
			t, ctx, apex, sequentialInstances,
			apex.Users[:parallelInstances],
			[]*cardanofw.TestApexUser{user},
			[]string{cardanofw.ChainIDPrime, cardanofw.ChainIDCardano},
			map[string][]string{
				cardanofw.ChainIDPrime:   {cardanofw.ChainIDCardano},
				cardanofw.ChainIDCardano: {cardanofw.ChainIDPrime},
			},
			map[e2ehelper.SrcDstChainPair]sendtx.BridgingType{
				e2ehelper.NewChainPair(cardanofw.ChainIDPrime, cardanofw.ChainIDCardano): sendtx.BridgingTypeCurrencyOnSource,
				e2ehelper.NewChainPair(cardanofw.ChainIDCardano, cardanofw.ChainIDPrime): sendtx.BridgingTypeWrappedTokenOnSource,
			},
			new(big.Int).SetUint64(sendAmount),
			e2ehelper.WithWaitForUnexpectedBridges(true),
			e2ehelper.WithRestartValidatorsConfig([]e2ehelper.RestartValidatorsConfig{
				{WaitTime: stopAfter, StopIndxs: []int{validatorStoppingIdx}},
			}))
	})

	t.Run("13. Both directions sequential and parallel — two nodes go offline midway, one node recovers", func(t *testing.T) {
		const (
			sequentialInstances   = 5
			parallelInstances     = 10
			stopAfter             = time.Second * 60
			startAgainAfter       = time.Second * 120
			validatorStoppingIdx1 = 1
			validatorStoppingIdx2 = 2
			sendAmount            = uint64(1_000_000)
		)

		t.Cleanup(func() {
			apex.ResetIndexers()

			_ = apex.GetValidator(t, validatorStoppingIdx2).Stop() // make sure it was stopped
			require.NoError(t, apex.GetValidator(t, validatorStoppingIdx2).Start(ctx, false))
		})

		fundTestUsersWithToken(
			t, ctx, apex, testConfigs, apex.Users[:parallelInstances],
			uint64(1_100_000_000), uint64(10_000_000))

		e2ehelper.ExecuteBridging(
			t, ctx, apex, sequentialInstances,
			apex.Users[:parallelInstances],
			[]*cardanofw.TestApexUser{user},
			[]string{cardanofw.ChainIDPrime, cardanofw.ChainIDCardano},
			map[string][]string{
				cardanofw.ChainIDPrime:   {cardanofw.ChainIDCardano},
				cardanofw.ChainIDCardano: {cardanofw.ChainIDPrime},
			},
			map[e2ehelper.SrcDstChainPair]sendtx.BridgingType{
				e2ehelper.NewChainPair(cardanofw.ChainIDPrime, cardanofw.ChainIDCardano): sendtx.BridgingTypeCurrencyOnSource,
				e2ehelper.NewChainPair(cardanofw.ChainIDCardano, cardanofw.ChainIDPrime): sendtx.BridgingTypeWrappedTokenOnSource,
			},
			new(big.Int).SetUint64(sendAmount),
			e2ehelper.WithWaitForUnexpectedBridges(true),
			e2ehelper.WithRestartValidatorsConfig([]e2ehelper.RestartValidatorsConfig{
				{WaitTime: stopAfter, StopIndxs: []int{validatorStoppingIdx1, validatorStoppingIdx2}},
				{WaitTime: startAgainAfter, StartIndxs: []int{validatorStoppingIdx1}},
			}))
	})
}

func TestE2E_SkylineBridge_WithVector_InvalidScenarios(t *testing.T) {
	const (
		apiKey  = "test_api_key"
		userCnt = 15

		maxWaitTimeSec = 600
		retryDelaySec  = 5
	)

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	cardanoConfig, vectorConfig := cardanofw.NewCardanoChainConfig(true), cardanofw.NewVectorChainConfig()
	cardanoConfig.FundTokenAmount = 1_000_000_000
	cardanoConfig.UseIndexer = true

	vectorConfig.FundTokenAmount = 1_000_000_000
	vectorConfig.UseIndexer = true

	apex := cardanofw.SetupAndRunSkylineBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithUserCnt(userCnt),
		cardanofw.WithVectorConfig(vectorConfig),
		cardanofw.WithCardanoConfig(cardanoConfig),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	user := apex.Users[userCnt-1]

	fmt.Println("cardano user addr: ", user.CardanoAddress)
	fmt.Println("cardano multisig addr: ", apex.CardanoInfo.MultisigAddr)
	fmt.Println("cardano fee addr: ", apex.CardanoInfo.FeeAddr)
	fmt.Printf("cardano socket path: %s\n", apex.CardanoInfo.SocketPath)
	fmt.Println("vector user addr: ", user.VectorAddress)
	fmt.Println("vector multisig addr: ", apex.VectorInfo.MultisigAddr)
	fmt.Println("vector fee addr: ", apex.VectorInfo.FeeAddr)
	fmt.Printf("vector socket path: %s\n", apex.VectorInfo.SocketPath)

	vectorToken, err := cardanofw.FundUserWithToken(
		ctx, apex, cardanofw.ChainIDVector,
		apex.VectorInfo.GenesisWallet, user,
		cardanofw.XADATokenName, cardanofw.DefaultTokenMintAmount,
		uint64(10_000_000), cardanofw.DefaultTokenMintAmount)
	require.NoError(t, err)

	vectorCardanoTestConfig := newTestConfig(
		t, apex.Config.VectorConfig, &apex.VectorInfo, cardanofw.ChainIDCardano, vectorToken.TokenName())

	t.Run("1. vector -> cardano - currency on src", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() {
			t.Skip()
		}

		t.Cleanup(func() {
			apex.ResetIndexers()
		})

		executeInvalidTokenDirection(t, ctx, apex, vectorCardanoTestConfig, user, maxWaitTimeSec, retryDelaySec, sendtx.BridgingTypeCurrencyOnSource, true, 0)
	})
}

func TestE2E_SkylineBridge_InvalidScenarios_RefundDisabled(t *testing.T) {
	const (
		apiKey  = "test_api_key"
		userCnt = 15

		maxWaitTimeSec = 600
		retryDelaySec  = 5

		sendAmount = uint64(1_000_000)
	)

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	primeConfig, cardanoConfig := cardanofw.NewPrimeChainConfig(), cardanofw.NewCardanoChainConfig(true)
	cardanoConfig.FundTokenAmount = 1_000_000_000

	vectorConfig := cardanofw.NewVectorChainConfig()
	vectorConfig.FundTokenAmount = 1_000_000_000

	apex := cardanofw.SetupAndRunSkylineBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithUserCnt(userCnt),
		cardanofw.WithCardanoConfig(cardanoConfig),
		cardanofw.WithPrimeConfig(primeConfig),
		cardanofw.WithVectorConfig(vectorConfig),
		cardanofw.WithCustomConfigHandlers(func(_ *cardanofw.ApexSystem, mp map[string]interface{}) {
			mp["refundEnabled"] = false
		}, nil),
		cardanofw.WithBridgingAddrCnt(cardanofw.ChainIDPrime, bridgeAddrCnt),
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

	cardanoTokenAmount, err := cardanofw.FundUserWithToken(
		ctx, apex, cardanofw.ChainIDCardano,
		apex.CardanoInfo.GenesisWallet, user,
		cardanofw.CAP3XTokenName, cardanofw.DefaultTokenMintAmount,
		sendAmount*100, cardanofw.DefaultTokenMintAmount)
	require.NoError(t, err)

	vectorToken, err := cardanofw.FundUserWithToken(
		ctx, apex, cardanofw.ChainIDVector,
		apex.VectorInfo.GenesisWallet, user,
		cardanofw.XADATokenName, cardanofw.DefaultTokenMintAmount,
		uint64(10_000_000), cardanofw.DefaultTokenMintAmount)
	require.NoError(t, err)

	primeTestConfig := newTestConfig(
		t, apex.Config.PrimeConfig, &apex.PrimeInfo, cardanofw.ChainIDCardano, "")
	vectorTestConfig := newTestConfig(
		t, apex.Config.VectorConfig, &apex.VectorInfo, cardanofw.ChainIDCardano, vectorToken.TokenName())
	bridgingType := sendtx.BridgingTypeCurrencyOnSource

	fmt.Printf("cardano user tokenAmount: %+v\n", cardanoTokenAmount)

	t.Run("1. Mismatch submitted and receiver amounts", func(t *testing.T) {
		executeInvalidMismatchSendLovelaceAmount(t, ctx, apex, primeTestConfig, user, maxWaitTimeSec, retryDelaySec, bridgingType, false, 0)
	})

	t.Run("2 Multiple submitters mismatch submitted and receiver amounts", func(t *testing.T) {
		executeInvalidMismatchSendAmountMultipleInstances(t, ctx, apex, primeTestConfig, maxWaitTimeSec, retryDelaySec, bridgingType, false, 0)
	})

	t.Run("3. Multiple submitters mismatch submitted and receiver amounts parallel", func(t *testing.T) {
		executeInvalidMismatchSendAmountMultipleInstancesParalel(t, ctx, apex, primeTestConfig, maxWaitTimeSec, retryDelaySec, bridgingType, false, 0)
	})

	t.Run("4. Invalid bridging type vector -> cardano - currency on src", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() {
			t.Skip()
		}

		executeInvalidTokenDirection(t, ctx, apex, vectorTestConfig, user, maxWaitTimeSec, retryDelaySec, sendtx.BridgingTypeCurrencyOnSource, false, 0)
	})

	t.Run("5.Submitted invalid metadata - currency under min - token on source", func(t *testing.T) {
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
				Addr:   user.GetAddress(cardanofw.ChainIDCardano),
				Amount: sendAmount,
				Token:  cardanofw.XADATokenID,
			},
		}

		operationFee := apex.GetMinOperationFee(cardanofw.ChainIDVector)

		feeAmount, err := apex.GetChainMust(t, cardanofw.ChainIDVector).GetBridgingFee(
			ctx, cardanofw.ChainIDCardano, receivers, apex.GetMinBridgingFee(cardanofw.ChainIDVector, true),
			operationFee, apex.VectorInfo.MultisigAddr[0])
		require.NoError(t, err)

		feeAmount -= 1_000_000

		metadata, err := apex.GetChainMust(t, cardanofw.ChainIDVector).CreateMetadata(
			user.GetAddress(cardanofw.ChainIDVector), cardanofw.ChainIDCardano,
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

	t.Run("6. Submitted invalid metadata - wrong type", func(t *testing.T) {
		executeInvalidMetadataType(t, ctx, apex, primeTestConfig, user, 60, retryDelaySec, bridgingType, false, 0)
	})

	t.Run("7. Submitted invalid metadata - invalid destination", func(t *testing.T) {
		executeInvalidDestination(t, ctx, apex, primeTestConfig, user, maxWaitTimeSec, retryDelaySec, bridgingType, false, 0)
	})

	t.Run("8. Submitted invalid metadata - invalid sender", func(t *testing.T) {
		executeInvalidMetadataInvalidSender(t, ctx, apex, primeTestConfig, user, maxWaitTimeSec, bridgingType, 0)
	})

	t.Run("9. Submitted invalid metadata - invalid bridging fee", func(t *testing.T) {
		executeInvalidBridgingFee(t, ctx, apex, primeTestConfig, maxWaitTimeSec, retryDelaySec, sendtx.BridgingTypeCurrencyOnSource, false, 0)
	})

	t.Run("10. Submitted invalid metadata - invalid fee receiver address - token on source", func(t *testing.T) {
		executeInvalidFeeReceiverAddr(t, ctx, apex, primeTestConfig, 2*60, retryDelaySec, sendtx.BridgingTypeCurrencyOnSource, false, 0)
	})

	t.Run("11. Submitted invalid metadata - empty receivers", func(t *testing.T) {
		executeInvalidEmptyReceivers(t, ctx, apex, primeTestConfig, user, maxWaitTimeSec, retryDelaySec, sendtx.BridgingTypeCurrencyOnSource, false, 0)
	})

	t.Run("12. Submitted with unknown tokens to bridging addr", func(t *testing.T) {
		user := apex.Users[userCnt-1]
		minterWallet, _ := user.GetCardanoWallet(cardanofw.ChainIDVector)

		tokensFunded, err := cardanofw.FundUserWithToken(
			ctx, apex, cardanofw.ChainIDVector,
			minterWallet, user,
			cardanofw.XADATokenName, cardanofw.DefaultTokenMintAmount,
			uint64(1_500_000), uint64(1_000_000))
		require.NoError(t, err)

		executeInvalidSendNativeToken(t, ctx, apex, user, vectorTestConfig, *tokensFunded, maxWaitTimeSec, retryDelaySec, false, 0, sendtx.BridgingTypeCurrencyOnSource)
	})

	t.Run("13. Submitted invalid metadata - invalid send amount - token on source", func(t *testing.T) {
		user, err := cardanofw.NewTestApexUser(cardanofw.NewApexNetworkTypesFromSystem(apex))
		require.NoError(t, err)

		tokensFunded, err := cardanofw.FundUserWithToken(
			ctx, apex, cardanofw.ChainIDVector,
			apex.VectorInfo.GenesisWallet, user,
			cardanofw.XADATokenName, cardanofw.DefaultTokenMintAmount,
			uint64(10_000_000), uint64(1_123_000))
		require.NoError(t, err)

		primeTestConfig.srcTokenName = tokensFunded.TokenName()

		executeInvalidMismatchSendNativeTokenAmount(t, ctx, apex, user, vectorTestConfig, *tokensFunded, maxWaitTimeSec, retryDelaySec, false, 0)
	})

	t.Run("14.1 Prime -> Cardano - Submitted invalid metadata - invalid destination", func(t *testing.T) {
		executeInvalidDestination(t, ctx, apex, primeTestConfig, user, maxWaitTimeSec, retryDelaySec, sendtx.BridgingTypeCurrencyOnSource, false, 0)
	})

	cardanoPrimeTestConfig := newTestConfig(
		t, apex.Config.CardanoConfig, &apex.CardanoInfo, cardanofw.ChainIDPrime, apex.GetTokenNameForChains(cardanofw.ChainIDCardano, cardanofw.ChainIDPrime, apex.GetTokenIDForChain(cardanofw.ChainIDPrime, true)))

	t.Run("14.2 Cardano -> Prime - Submitted invalid metadata - invalid destination", func(t *testing.T) {
		executeInvalidDestination(t, ctx, apex, cardanoPrimeTestConfig, user, maxWaitTimeSec, retryDelaySec, sendtx.BridgingTypeWrappedTokenOnSource, false, 0)
	})

	t.Run("14.3 Vector -> Cardano - Submitted invalid metadata - invalid destination", func(t *testing.T) {
		executeInvalidDestination(t, ctx, apex, vectorTestConfig, user, maxWaitTimeSec, retryDelaySec, sendtx.BridgingTypeWrappedTokenOnSource, false, 0)
	})
}

func TestE2E_SkylineBridge_Over_Max_Allowed_To_Bridge(t *testing.T) {
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

	vectorConfig := cardanofw.NewVectorChainConfig()
	vectorConfig.FundTokenAmount = 1_000_000_000
	vectorConfig.UseIndexer = true

	apex := cardanofw.SetupAndRunSkylineBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithUserCnt(1),
		cardanofw.WithCardanoConfig(cardanoConfig),
		cardanofw.WithPrimeConfig(primeConfig),
		cardanofw.WithVectorConfig(vectorConfig),
		cardanofw.WithCustomConfigHandlers(func(_ *cardanofw.ApexSystem, mp map[string]interface{}) {
			setting := cardanofw.GetMapFromInterfaceKey(mp, "bridgingSettings")
			setting["maxAmountAllowedToBridge"] = new(big.Int).SetUint64(5_000_000)
			mp["refundEnabled"] = false
		}, nil),
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

	for idx, br := range bridgingRequests {
		var err error

		wg.Add(1)

		go func(i int, src string, dest string, sender *cardanofw.TestApexUser) {
			defer wg.Done()

			txHashes[i], err = apex.SubmitBridgingRequest(ctx, src, dest, sender, apexSendAmount, sendtx.BridgingTypeCurrencyOnSource,
				user)
			require.NoError(t, err)

			fmt.Printf("Bridging request: %v to %v sent. hash: %s\n", src, dest, txHashes[i])
		}(idx, br.src, br.dest, br.sender)
	}

	wg.Wait()

	for idx, br := range bridgingRequests {
		wg.Add(1)

		go func() {
			defer wg.Done()

			cardanofw.WaitForInvalidState(t, ctx, apex, br.src, txHashes[idx], apiKey, 0)
		}()
	}

	wg.Wait()
}

func TestE2E_SkylineBridge_Over_Max_Tokens_Allowed_To_Bridge(t *testing.T) {
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

	vectorConfig := cardanofw.NewVectorChainConfig()
	vectorConfig.FundTokenAmount = 1_000_000_000
	vectorConfig.UseIndexer = true

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
			mp["refundEnabled"] = false
		}, nil),
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
		txHashes = make([]string, len(bridgingRequests))
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

		go func(i int, src string, dest string, sender *cardanofw.TestApexUser) {
			defer wg.Done()

			txHashes[i], err = apex.SubmitBridgingRequest(ctx, src, dest, sender, apexSendAmount, sendtx.BridgingTypeWrappedTokenOnSource,
				user)
			require.NoError(t, err)

			fmt.Printf("Bridging request: %v to %v sent. hash: %s\n", src, dest, txHashes[i])
		}(idx, br.src, br.dest, br.sender)
	}

	wg.Wait()

	for idx, br := range bridgingRequests {
		wg.Add(1)

		go func() {
			defer wg.Done()

			cardanofw.WaitForInvalidState(t, ctx, apex, br.src, txHashes[idx], apiKey, 0)
		}()
	}

	wg.Wait()
}

func TestE2E_SkylineBridge_UTxOConsolidationBothDirectionsWithCurrencyAndTokens(t *testing.T) {
	if cardanofw.ShouldSkipE2RRedundantTests() {
		t.Skip()
	}

	const (
		fundUtxoCount                 = 9
		maxFeeUtxoCount               = 1
		maxUtxoCount                  = 3
		minimumExpectedConsolidations = 3

		sequentialInstances = 3
		parallelInstances   = 6

		sendMinValueIncrement = 10
		fundFactor            = 7
	)

	var (
		sendMinValueFactor uint64 = maxUtxoCount - maxFeeUtxoCount + 1
	)

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	minValue := uint64(1_100_000)
	cardanoConfig := cardanofw.NewCardanoChainConfig(true)
	cardanoConfig.FundUTxOCount = fundUtxoCount
	cardanoConfig.FundAmount = fundFactor * minValue * fundUtxoCount
	cardanoConfig.FundTokenAmount = fundFactor * minValue * fundUtxoCount
	cardanoConfig.InitialHotWalletAmount = new(big.Int).SetUint64(cardanoConfig.FundAmount)
	cardanoConfig.InitialHotWalletTokenAmount = new(big.Int).SetUint64(cardanoConfig.FundTokenAmount)
	cardanoConfig.UseIndexer = true

	primeConfig := cardanofw.NewPrimeChainConfig()
	primeConfig.FundUTxOCount = fundUtxoCount
	primeConfig.FundAmount = fundFactor * minValue * fundUtxoCount
	primeConfig.FundTokenAmount = 0
	primeConfig.InitialHotWalletAmount = new(big.Int).SetUint64(cardanoConfig.FundAmount)
	primeConfig.InitialHotWalletTokenAmount = new(big.Int).SetUint64(0)
	primeConfig.UseIndexer = true

	sendAmountTokens := minValue*sendMinValueFactor + sendMinValueIncrement   // when we send tokens, this amount of currency will be released from multisig address
	sendAmountCurrency := minValue*sendMinValueFactor + sendMinValueIncrement // when we send currency, this amount of native tokens will be released from multisig address

	var (
		initialUtxosCardano, initialUtxosPrime []map[string]any
		tipDataCardano, tipDataPrime           wallet.QueryTipData
		lock                                   sync.Mutex
	)

	apex := cardanofw.SetupAndRunSkylineBridge(
		t, ctx,
		cardanofw.WithUserCnt(parallelInstances+1),
		cardanofw.WithCardanoConfig(cardanoConfig),
		cardanofw.WithPrimeConfig(primeConfig),
		cardanofw.WithBridgingAddrCnt(cardanofw.ChainIDPrime, bridgeAddrCnt),
		cardanofw.WithCustomConfigHandlers(func(a *cardanofw.ApexSystem, mp map[string]any) {
			t.Helper()

			lock.Lock()
			defer lock.Unlock()

			// retrieve only once for all validators
			if len(initialUtxosCardano) == 0 {
				initialUtxosCardano, tipDataCardano = getInitialUtxosAndTip(
					t, ctx, a.CardanoInfo, a.CardanoInfo.MultisigAddr, a.CardanoInfo.FeeAddr)
				initialUtxosPrime, tipDataPrime = getInitialUtxosAndTip(
					t, ctx, a.PrimeInfo, a.PrimeInfo.MultisigAddr, a.PrimeInfo.FeeAddr,
				)
			}

			// Prime and Cardano indexers should start after multisig funding is done
			vcCfg := cardanofw.GetMapFromInterfaceKey(mp, "cardanoChains", cardanofw.ChainIDCardano)
			vcCfg["startBlockHash"] = tipDataCardano.Hash
			vcCfg["startSlot"] = tipDataCardano.Slot
			vcCfg["initialUtxos"] = initialUtxosCardano
			vcCfg["maxFeeUtxoCount"] = maxFeeUtxoCount
			vcCfg["maxUtxoCount"] = maxUtxoCount
			vcCfg["takeAtLeastUtxoCount"] = 1
			vcCfg = cardanofw.GetMapFromInterfaceKey(mp, "cardanoChains", cardanofw.ChainIDPrime)
			vcCfg["startBlockHash"] = tipDataPrime.Hash
			vcCfg["startSlot"] = tipDataPrime.Slot
			vcCfg["initialUtxos"] = initialUtxosPrime
			vcCfg["maxFeeUtxoCount"] = maxFeeUtxoCount
			vcCfg["maxUtxoCount"] = maxUtxoCount
			vcCfg["takeAtLeastUtxoCount"] = 1
		}, nil),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	txProviderCardano, err := apex.CardanoInfo.GetTxProvider()
	require.NoError(t, err)

	fundTestUsersWithToken(t, ctx, apex, []*testConfig{
		{
			srcChainID:      cardanofw.ChainIDPrime,
			srcMinterWallet: apex.PrimeInfo.GenesisWallet,
		},
		{
			srcChainID:      cardanofw.ChainIDCardano,
			srcMinterWallet: apex.CardanoInfo.GenesisWallet,
		},
	}, apex.Users[:parallelInstances], uint64(2_000_000_000), uint64(2_000_000_000))

	utxos, err := infracommon.ExecuteWithRetry(
		ctx, func(ctx context.Context) ([]wallet.Utxo, error) {
			return txProviderCardano.GetUtxos(ctx, apex.CardanoInfo.MultisigAddr[0])
		},
	)
	require.NoError(t, err)

	require.Len(t, utxos, cardanoConfig.FundUTxOCount)

	var (
		utxosCardanoTokenSum1 uint64
		utxosCardanoTokenSum2 uint64
		lastBatchIDs          map[string]uint64 = map[string]uint64{"prime": 0, "cardano": 0}
	)

	t.Run("with currency from prime to cardano", func(t *testing.T) {
		t.Cleanup(func() {
			apex.ResetIndexers()
		})

		ctxChild, cncl := context.WithCancel(ctx)
		defer cncl()

		utxosCardano, err := infracommon.ExecuteWithRetry(
			ctx, func(ctx context.Context) ([]wallet.Utxo, error) {
				return txProviderCardano.GetUtxos(ctx, apex.CardanoInfo.MultisigAddr[0])
			},
		)
		require.NoError(t, err)

		utxosCardanoSum := wallet.GetUtxosSum(utxosCardano)

		// sum of tokens on cardano multisig address in the beginning
		tokenID := apex.GetTokenIDForChain(cardanofw.ChainIDPrime, true)
		tokenName := apex.GetTokenNameForChains(cardanofw.ChainIDCardano, cardanofw.ChainIDPrime, tokenID)
		utxosCardanoTokenSum1 = utxosCardanoSum[tokenName]

		getCntConsolidationMap, lastBatchIDsRet := checkConsolidationBatchCounts(
			t, ctxChild,
			apex.BridgeCluster.Servers[0].JSONRPC(),
			[]string{cardanofw.ChainIDCardano},
			lastBatchIDs,
		)

		for chain, id := range lastBatchIDsRet {
			lastBatchIDs[chain] = id
		}

		e2ehelper.ExecuteBridging(
			t, ctxChild, apex, sequentialInstances,
			apex.Users[:parallelInstances],
			[]*cardanofw.TestApexUser{apex.Users[parallelInstances]},
			[]string{cardanofw.ChainIDPrime},
			map[string][]string{
				cardanofw.ChainIDPrime: {cardanofw.ChainIDCardano},
			},
			map[e2ehelper.SrcDstChainPair]sendtx.BridgingType{
				e2ehelper.NewChainPair(cardanofw.ChainIDPrime, cardanofw.ChainIDCardano): sendtx.BridgingTypeCurrencyOnSource,
			},
			new(big.Int).SetUint64(sendAmountCurrency),
			e2ehelper.WithWaitForUnexpectedBridges(true),
		)

		for _, cnt := range getCntConsolidationMap() {
			assert.GreaterOrEqual(t, cnt, minimumExpectedConsolidations)
		}
	})

	t.Run("with tokens from cardano to prime", func(t *testing.T) {
		t.Cleanup(func() {
			apex.ResetIndexers()
		})

		ctxChild, cncl := context.WithCancel(ctx)
		defer cncl()

		getCntConsolidationMap, lastBatchIDsRet := checkConsolidationBatchCounts(
			t, ctxChild,
			apex.BridgeCluster.Servers[0].JSONRPC(),
			[]string{cardanofw.ChainIDPrime},
			lastBatchIDs,
		)

		for chain, id := range lastBatchIDsRet {
			lastBatchIDs[chain] = id
		}

		e2ehelper.ExecuteBridging(
			t, ctxChild, apex, sequentialInstances,
			apex.Users[:parallelInstances],
			[]*cardanofw.TestApexUser{apex.Users[parallelInstances]},
			[]string{cardanofw.ChainIDCardano},
			map[string][]string{
				cardanofw.ChainIDCardano: {cardanofw.ChainIDPrime},
			},
			map[e2ehelper.SrcDstChainPair]sendtx.BridgingType{
				e2ehelper.NewChainPair(cardanofw.ChainIDCardano, cardanofw.ChainIDPrime): sendtx.BridgingTypeWrappedTokenOnSource,
			},
			new(big.Int).SetUint64(sendAmountTokens),
			e2ehelper.WithWaitForUnexpectedBridges(true),
		)

		utxosCardano, err := infracommon.ExecuteWithRetry(
			ctx, func(ctx context.Context) ([]wallet.Utxo, error) {
				return txProviderCardano.GetUtxos(ctx, apex.CardanoInfo.MultisigAddr[0])
			},
		)
		require.NoError(t, err)

		utxosCardanoSum := wallet.GetUtxosSum(utxosCardano)
		tokenID := apex.GetTokenIDForChain(cardanofw.ChainIDCardano, false)
		tokenName := apex.GetTokenNameForChains(cardanofw.ChainIDPrime, cardanofw.ChainIDCardano, tokenID)

		// sum of tokens on cardano multisig address in the end
		utxosCardanoTokenSum2 = utxosCardanoSum[tokenName]
		require.Equal(t, utxosCardanoTokenSum1, utxosCardanoTokenSum2)

		for _, cnt := range getCntConsolidationMap() {
			assert.GreaterOrEqual(t, cnt, minimumExpectedConsolidations)
		}
	})
}

func TestE2E_SkylineBridge_Fund_Defund(t *testing.T) {
	if cardanofw.ShouldSkipE2RRedundantTests() {
		t.Skip()
	}

	const (
		apiKey  = "test_api_key"
		userCnt = 10
	)

	var (
		err error
	)

	type chainStageKey struct {
		chain    string
		srcChain string
		receiver uint
	}

	type bridingRequest struct {
		src         string
		dest        string
		sender      *cardanofw.TestApexUser
		amount      *big.Int
		receiverIdx uint
		requestType sendtx.BridgingType
	}

	createBridgingData := func(ctx context.Context, apex *cardanofw.ApexSystem,
		bridgingRequests []*bridingRequest, receivers map[uint]*cardanofw.TestApexUser,
		defundReceiver *cardanofw.TestApexUser, defundAmount *big.Int) (
		map[chainStageKey]*big.Int, map[chainStageKey]*big.Int,
		map[chainStageKey]*cardanofw.TestApexUser,
		map[chainStageKey]*big.Int, map[chainStageKey]*big.Int,
		map[chainStageKey]*cardanofw.TestApexUser,
	) {
		var (
			chainPrevAmounts     = make(map[chainStageKey]*big.Int)
			chainExpectedAmounts = make(map[chainStageKey]*big.Int)
			chainReceivers       = make(map[chainStageKey]*cardanofw.TestApexUser)

			defundReceiversPrevAmount     = make(map[chainStageKey]*big.Int)
			defundReceiversExpectedAmount = make(map[chainStageKey]*big.Int)
			defundReceivers               = make(map[chainStageKey]*cardanofw.TestApexUser)
		)

		for _, br := range bridgingRequests {

			tokenID := apex.GetTokenIDForChain(br.src, br.requestType == sendtx.BridgingTypeCurrencyOnSource)
			tokenName := apex.GetTokenNameForChains(br.dest, br.src, tokenID)

			key := chainStageKey{chain: br.dest, srcChain: br.src, receiver: br.receiverIdx}
			if _, exists := chainPrevAmounts[key]; !exists {
				balance, err := apex.GetBalance(ctx, receivers[br.receiverIdx], br.dest)
				require.NoError(t, err)

				chainPrevAmounts[key] = cardanofw.SetOrDefault(balance[tokenName], big.NewInt(0))
			}

			if _, exists := chainExpectedAmounts[key]; !exists {
				chainExpectedAmounts[key] = big.NewInt(0)
			}

			chainExpectedAmounts[key].Add(chainExpectedAmounts[key], cardanofw.ApexToDfm(br.amount))

			if _, exists := chainReceivers[key]; !exists {
				chainReceivers[key] = receivers[br.receiverIdx]
			}

			if defundAmount != nil && defundReceiver != nil {
				if _, exists := defundReceiversPrevAmount[key]; !exists {
					balance, err := apex.GetBalance(ctx, defundReceiver, br.dest)
					require.NoError(t, err)

					defundReceiversPrevAmount[key] = cardanofw.SetOrDefault(balance[tokenName], big.NewInt(0))
				}

				if _, exist := defundReceiversExpectedAmount[key]; !exist {
					defundReceiversExpectedAmount[key] = big.NewInt(0)
				}

				defundReceiversExpectedAmount[key].Add(defundReceiversExpectedAmount[key], cardanofw.ApexToDfm(defundAmount))

				if _, exists := defundReceivers[key]; !exists {
					defundReceivers[key] = defundReceiver
				}
			}
		}

		return chainPrevAmounts, chainExpectedAmounts, chainReceivers, defundReceiversPrevAmount, defundReceiversExpectedAmount, defundReceivers
	}

	bridgeTransactions := func(ctx context.Context, apex *cardanofw.ApexSystem,
		bridgingRequests []*bridingRequest, receivers map[uint]*cardanofw.TestApexUser,
	) {
		var wg sync.WaitGroup

		for _, br := range bridgingRequests {
			wg.Add(1)

			go func(src string, dest string, sender *cardanofw.TestApexUser, receiver *cardanofw.TestApexUser, amount *big.Int) {
				defer wg.Done()

				txHash, err := apex.SubmitBridgingRequest(ctx, src, dest, sender, amount, br.requestType, receiver)
				require.NoError(t, err)

				fmt.Printf("Bridging request: %v to %v sent. hash: %s\n", src, dest, txHash)
			}(br.src, br.dest, br.sender, receivers[br.receiverIdx], cardanofw.ApexToDfm(br.amount))
		}

		wg.Wait()
	}

	waitOnDestination := func(
		ctx context.Context, apex *cardanofw.ApexSystem,
		chainPrevAmounts map[chainStageKey]*big.Int, chainExpectedAmounts map[chainStageKey]*big.Int,
		chainReceivers map[chainStageKey]*cardanofw.TestApexUser, numRetries int, waitTime time.Duration, isNativeToken bool,
	) map[chainStageKey]error {
		var (
			wg           sync.WaitGroup
			errsPerChain = make(map[chainStageKey]error, len(chainPrevAmounts))
			mu           sync.Mutex
		)

		for chainKey, prevAmount := range chainPrevAmounts {
			wg.Add(1)

			go func() {
				defer wg.Done()

				fmt.Printf("Waiting for %v Amount on %v\n", chainExpectedAmounts[chainKey], chainKey.chain)

				expectedAmount := new(big.Int).Set(chainExpectedAmounts[chainKey])
				expectedAmount.Add(expectedAmount, prevAmount)

				err = apex.WaitForExactAmount(
					ctx, chainReceivers[chainKey], chainKey.chain, chainKey.srcChain, expectedAmount, numRetries, waitTime, isNativeToken)

				mu.Lock()
				defer mu.Unlock()

				errsPerChain[chainKey] = err
			}()
		}

		wg.Wait()

		return errsPerChain
	}

	fundWallets := func(
		ctx context.Context, apex *cardanofw.ApexSystem,
		fundAmountApex *big.Int, isNativeToken bool,
	) error {
		fmt.Printf("Funding hot wallets\n")

		chains := []string{cardanofw.ChainIDPrime, cardanofw.ChainIDCardano}
		for _, chain := range chains {
			if isNativeToken {
				apex.Config.PrimeConfig.FundAmount = cardanofw.ApexToDfm(fundAmountApex).Uint64()
				apex.Config.VectorConfig.FundAmount = cardanofw.ApexToDfm(fundAmountApex).Uint64()
				apex.Config.VectorConfig.FundTokenAmount = cardanofw.ApexToDfm(fundAmountApex).Uint64()
				apex.Config.CardanoConfig.FundAmount = cardanofw.ApexToDfm(fundAmountApex).Uint64()
				apex.Config.CardanoConfig.FundTokenAmount = cardanofw.ApexToDfm(fundAmountApex).Uint64()

				fmt.Printf("Funding wallets with %+v\n", apex.Config.PrimeConfig.FundAmount)

				if err = apex.FundWallets(ctx); err != nil {
					return err
				}
			} else {
				if err = apex.FundChainHotWallet(ctx, chain, cardanofw.ApexToDfm(fundAmountApex)); err != nil {
					return err
				}
			}
		}

		fmt.Printf("Hot wallets have been funded\n")

		return nil
	}

	defundWallets := func(
		ctx context.Context, apex *cardanofw.ApexSystem,
		defundReceiver *cardanofw.TestApexUser, defundAmountApex *big.Int,
		defundReceiverPrevAmounts map[chainStageKey]*big.Int, defundReceiverExpectedAmounts map[chainStageKey]*big.Int,
		defundReceivers map[chainStageKey]*cardanofw.TestApexUser, isNativeToken bool,
	) {
		fmt.Printf("Defunding hot wallets\n")

		defundAmount := cardanofw.ApexToDfm(defundAmountApex)

		require.NoError(t, apex.DefundHotWallet(
			cardanofw.ChainIDPrime, defundReceiver.GetAddress(cardanofw.ChainIDPrime), defundAmount, big.NewInt(0)))

		require.NoError(t, apex.DefundHotWallet(
			cardanofw.ChainIDCardano, defundReceiver.GetAddress(cardanofw.ChainIDCardano), defundAmount, defundAmount))

		require.NoError(t, apex.DefundHotWallet(
			cardanofw.ChainIDVector, defundReceiver.GetAddress(cardanofw.ChainIDVector), defundAmount, defundAmount))

		errsPerChain := waitOnDestination(ctx, apex,
			defundReceiverPrevAmounts, defundReceiverExpectedAmounts, defundReceivers,
			200, time.Second*10, isNativeToken)
		for chainKey, err := range errsPerChain {
			require.NoError(t, err)
			fmt.Printf("Defund on %v confirmed\n", chainKey.chain)
		}
	}

	t.Run("1. Basic defund test", func(t *testing.T) {
		ctx, cncl := context.WithCancel(context.Background())
		defer cncl()

		initialFundInDfm := cardanofw.ApexToDfm(big.NewInt(100))

		primeConfig, cardanoConfig := cardanofw.NewPrimeChainConfig(), cardanofw.NewCardanoChainConfig(true)
		primeConfig.FundAmount = cardanofw.DfmToChainNativeTokenAmount(cardanofw.ChainIDPrime, initialFundInDfm).Uint64()
		cardanoConfig.FundAmount = cardanofw.DfmToChainNativeTokenAmount(cardanofw.ChainIDCardano, initialFundInDfm).Uint64()
		cardanoConfig.FundTokenAmount = cardanofw.DfmToChainNativeTokenAmount(cardanofw.ChainIDCardano, initialFundInDfm).Uint64()

		vectorConfig := cardanofw.NewVectorChainConfig()
		vectorConfig.FundAmount = cardanofw.DfmToChainNativeTokenAmount(cardanofw.ChainIDVector, initialFundInDfm).Uint64()
		vectorConfig.FundTokenAmount = cardanofw.DfmToChainNativeTokenAmount(cardanofw.ChainIDVector, initialFundInDfm).Uint64()

		apex := cardanofw.SetupAndRunSkylineBridge(
			t, ctx,
			cardanofw.WithAPIKey(apiKey),
			cardanofw.WithUserCnt(userCnt),
			cardanofw.WithPrimeConfig(primeConfig),
			cardanofw.WithCardanoConfig(cardanoConfig),
			cardanofw.WithVectorConfig(vectorConfig),
			cardanofw.WithBridgingAddrCnt(cardanofw.ChainIDPrime, bridgeAddrCnt),
		)

		defer require.True(t, apex.ApexBridgeProcessesRunning())

		// give time for oracles to submit hot wallet increment claims for initial fundings
		select {
		case <-ctx.Done():
			return
		case <-time.After(90 * time.Second):
		}

		var (
			defundReceiver          = apex.Users[userCnt-2]
			apexDefundAndFundAmount = big.NewInt(70)
			apexSendAmount          = big.NewInt(50)

			bridgignType = sendtx.BridgingTypeWrappedTokenOnSource

			bridgingRequests = []*bridingRequest{
				{src: cardanofw.ChainIDCardano, dest: cardanofw.ChainIDPrime, sender: apex.Users[0], amount: apexSendAmount, receiverIdx: 0, requestType: sendtx.BridgingTypeWrappedTokenOnSource},
				{src: cardanofw.ChainIDVector, dest: cardanofw.ChainIDCardano, sender: apex.Users[0], amount: apexSendAmount, receiverIdx: 0, requestType: sendtx.BridgingTypeWrappedTokenOnSource},
			}

			receivers = map[uint]*cardanofw.TestApexUser{
				0: apex.Users[userCnt-1],
			}
		)

		isNativeToken := bridgignType == sendtx.BridgingTypeCurrencyOnSource

		fundTestUsersWithToken(t, ctx, apex, []*testConfig{
			{
				srcChainID:      cardanofw.ChainIDVector,
				srcMinterWallet: apex.VectorInfo.GenesisWallet,
			},
			{
				srcChainID:      cardanofw.ChainIDCardano,
				srcMinterWallet: apex.CardanoInfo.GenesisWallet,
			},
		}, apex.Users[:1], uint64(2_000_000), uint64(50_000_000))

		chainPrevAmounts, chainExpectedAmounts, chainReceivers,
			defundReceiversPrevAmount, defundReceiversExpectedAmount, defundReceivers :=
			createBridgingData(ctx, apex, bridgingRequests, receivers, defundReceiver, apexDefundAndFundAmount)

		defundWallets(ctx, apex, defundReceiver, apexDefundAndFundAmount,
			defundReceiversPrevAmount, defundReceiversExpectedAmount, defundReceivers, isNativeToken)

		bridgeTransactions(ctx, apex, bridgingRequests, receivers)

		fmt.Printf("Confirming that bridging requests will not be processed\n")

		errsPerChain := waitOnDestination(ctx, apex, chainPrevAmounts, chainExpectedAmounts, chainReceivers, 30, time.Second*10, isNativeToken)
		for chain, err := range errsPerChain {
			require.Error(t, err)
			fmt.Printf("As intended, %v TXs on %v not yet arrived\n", chainExpectedAmounts[chain], chain)
		}

		require.NoError(t, fundWallets(ctx, apex, apexDefundAndFundAmount, isNativeToken))

		errsPerChain = waitOnDestination(ctx, apex, chainPrevAmounts, chainExpectedAmounts, chainReceivers, 200, time.Second*10, isNativeToken)
		for chain, err := range errsPerChain {
			require.NoError(t, err)
			fmt.Printf("%v TXs on %v confirmed\n", chainExpectedAmounts[chain], chain)
		}
	})

	t.Run("2. Defund after bridging request is sent", func(t *testing.T) {
		ctx, cncl := context.WithCancel(context.Background())
		defer cncl()

		initialFundInDfm := cardanofw.ApexToDfm(big.NewInt(100))

		primeConfig, cardanoConfig := cardanofw.NewPrimeChainConfig(), cardanofw.NewCardanoChainConfig(true)
		primeConfig.FundAmount = cardanofw.DfmToChainNativeTokenAmount(cardanofw.ChainIDPrime, initialFundInDfm).Uint64()
		cardanoConfig.FundAmount = cardanofw.DfmToChainNativeTokenAmount(cardanofw.ChainIDCardano, initialFundInDfm).Uint64()
		cardanoConfig.FundTokenAmount = cardanofw.DfmToChainNativeTokenAmount(cardanofw.ChainIDCardano, initialFundInDfm).Uint64()

		vectorConfig := cardanofw.NewVectorChainConfig()
		vectorConfig.FundAmount = cardanofw.DfmToChainNativeTokenAmount(cardanofw.ChainIDVector, initialFundInDfm).Uint64()
		vectorConfig.FundTokenAmount = cardanofw.DfmToChainNativeTokenAmount(cardanofw.ChainIDVector, initialFundInDfm).Uint64()

		apex := cardanofw.SetupAndRunSkylineBridge(
			t, ctx,
			cardanofw.WithAPIKey(apiKey),
			cardanofw.WithUserCnt(userCnt),
			cardanofw.WithPrimeConfig(primeConfig),
			cardanofw.WithCardanoConfig(cardanoConfig),
			cardanofw.WithVectorConfig(vectorConfig),
			cardanofw.WithBridgingAddrCnt(cardanofw.ChainIDPrime, bridgeAddrCnt),
		)

		defer require.True(t, apex.ApexBridgeProcessesRunning())

		// give time for oracles to submit hot wallet increment claims for initial fundings
		select {
		case <-ctx.Done():
			return
		case <-time.After(90 * time.Second):
		}

		var (
			defundReceiver          = apex.Users[userCnt-2]
			apexDefundAndFundAmount = big.NewInt(70)
			apexSendAmount          = big.NewInt(50)

			bridgignType = sendtx.BridgingTypeWrappedTokenOnSource

			bridgingRequests = []*bridingRequest{
				{src: cardanofw.ChainIDVector, dest: cardanofw.ChainIDCardano, sender: apex.Users[0], amount: apexSendAmount, receiverIdx: 0, requestType: sendtx.BridgingTypeWrappedTokenOnSource},
				{src: cardanofw.ChainIDCardano, dest: cardanofw.ChainIDPrime, sender: apex.Users[1], amount: apexSendAmount, receiverIdx: 0, requestType: sendtx.BridgingTypeWrappedTokenOnSource},
			}

			receivers = map[uint]*cardanofw.TestApexUser{
				0: apex.Users[userCnt-1],
			}
		)

		isNativeToken := bridgignType == sendtx.BridgingTypeCurrencyOnSource

		minterWalletVector := apex.VectorInfo.GenesisWallet
		minterWalletCardano := apex.CardanoInfo.GenesisWallet

		_, err = cardanofw.FundUserWithToken(
			ctx, apex, cardanofw.ChainIDVector,
			minterWalletVector, apex.Users[0],
			cardanofw.XADATokenName, cardanofw.DefaultTokenMintAmount,
			uint64(2_000_000), uint64(250_000_000))
		require.NoError(t, err)

		_, err = cardanofw.FundUserWithToken(
			ctx, apex, cardanofw.ChainIDCardano,
			minterWalletCardano, apex.Users[1],
			cardanofw.CAP3XTokenName, cardanofw.DefaultTokenMintAmount,
			uint64(2_000_000), uint64(250_000_000))
		require.NoError(t, err)

		chainPrevAmounts, chainExpectedAmounts, chainReceivers, _, _, _ :=
			createBridgingData(ctx, apex, bridgingRequests, receivers, defundReceiver, apexDefundAndFundAmount)

		for _, request := range bridgingRequests {
			bridgeTransactions(ctx, apex, []*bridingRequest{request}, receivers)

			require.NoError(t, apex.DefundHotWallet(
				request.dest, defundReceiver.GetAddress(request.dest), cardanofw.ApexToDfm(apexDefundAndFundAmount), big.NewInt(0)))
		}

		fmt.Printf("Confirming that bridging requests will not be processed\n")

		errsPerChain := waitOnDestination(ctx, apex, chainPrevAmounts, chainExpectedAmounts, chainReceivers, 30, time.Second*10, isNativeToken)
		for chainKey, err := range errsPerChain {
			require.Error(t, err)
			fmt.Printf("As intended, %v TX on %v not yet arrived\n", chainExpectedAmounts[chainKey], chainKey.chain)
		}

		require.NoError(t, fundWallets(ctx, apex, apexDefundAndFundAmount, isNativeToken))

		errsPerChain = waitOnDestination(ctx, apex, chainPrevAmounts, chainExpectedAmounts, chainReceivers, 200, time.Second*10, isNativeToken)
		for chainKey, err := range errsPerChain {
			require.NoError(t, err)
			fmt.Printf("%v TX on %v confirmed\n", chainExpectedAmounts[chainKey], chainKey.chain)
		}
	})

	t.Run("3. Fund_Parallel_Send_BRs_Then_Full_Fund", func(t *testing.T) {
		ctx, cncl := context.WithCancel(context.Background())
		defer cncl()

		primeConfig, cardanoConfig := cardanofw.NewPrimeChainConfig(), cardanofw.NewCardanoChainConfig(true)
		primeConfig.FundAmount = 0
		cardanoConfig.FundAmount = 0
		primeConfig.FundTokenAmount = 0
		cardanoConfig.FundTokenAmount = 0

		vectorConfig := cardanofw.NewVectorChainConfig()
		vectorConfig.FundAmount = 0
		vectorConfig.FundTokenAmount = 0

		apex := cardanofw.SetupAndRunSkylineBridge(
			t, ctx,
			cardanofw.WithAPIKey(apiKey),
			cardanofw.WithUserCnt(userCnt),
			cardanofw.WithPrimeConfig(primeConfig),
			cardanofw.WithCardanoConfig(cardanoConfig),
			cardanofw.WithVectorConfig(vectorConfig),
			cardanofw.WithBridgingAddrCnt(cardanofw.ChainIDPrime, bridgeAddrCnt),
		)

		defer require.True(t, apex.ApexBridgeProcessesRunning())

		var (
			bridgingRequests = []*bridingRequest{
				{src: cardanofw.ChainIDVector, dest: cardanofw.ChainIDCardano, sender: apex.Users[0], amount: big.NewInt(1), receiverIdx: 0, requestType: sendtx.BridgingTypeWrappedTokenOnSource},
				{src: cardanofw.ChainIDCardano, dest: cardanofw.ChainIDPrime, sender: apex.Users[0], amount: big.NewInt(1), receiverIdx: 0, requestType: sendtx.BridgingTypeWrappedTokenOnSource},
			}

			receivers = map[uint]*cardanofw.TestApexUser{
				0: apex.Users[userCnt-1],
			}

			bridgignType = sendtx.BridgingTypeWrappedTokenOnSource
		)

		isNativeToken := bridgignType == sendtx.BridgingTypeCurrencyOnSource

		fundTestUsersWithToken(t, ctx, apex, []*testConfig{
			{
				srcChainID:      cardanofw.ChainIDVector,
				srcMinterWallet: apex.VectorInfo.GenesisWallet,
			},
			{
				srcChainID:      cardanofw.ChainIDCardano,
				srcMinterWallet: apex.CardanoInfo.GenesisWallet,
			},
		}, apex.Users[:1], uint64(2_000_000), uint64(50_000_000))

		chainPrevAmounts, chainExpectedAmounts, chainReceivers, _, _, _ := createBridgingData(ctx, apex, bridgingRequests, receivers, nil, nil)

		bridgeTransactions(ctx, apex, bridgingRequests, receivers)

		fmt.Printf("Confirming that bridging requests will not be processed\n")

		errsPerChain := waitOnDestination(ctx, apex, chainPrevAmounts, chainExpectedAmounts, chainReceivers, 30, time.Second*10, isNativeToken)
		for chainKey, err := range errsPerChain {
			require.Error(t, err)
			fmt.Printf("As intended, %v TXs on %v not yet arrived\n", chainExpectedAmounts[chainKey], chainKey.chain)
		}

		require.NoError(t, fundWallets(ctx, apex, big.NewInt(100), isNativeToken))

		errsPerChain = waitOnDestination(ctx, apex, chainPrevAmounts, chainExpectedAmounts, chainReceivers, 200, time.Second*10, isNativeToken)
		for chainKey, err := range errsPerChain {
			require.NoError(t, err)
			fmt.Printf("%v TXs on %v confirmed\n", chainExpectedAmounts[chainKey], chainKey)
		}
	})

	t.Run("4. Fund_Parallel_Send_BRs_Then_Fund_Twice", func(t *testing.T) {
		ctx, cncl := context.WithCancel(context.Background())
		defer cncl()

		primeConfig, cardanoConfig := cardanofw.NewPrimeChainConfig(), cardanofw.NewCardanoChainConfig(true)
		primeConfig.FundAmount = 0
		cardanoConfig.FundAmount = 0
		primeConfig.FundTokenAmount = 0
		cardanoConfig.FundTokenAmount = 0

		vectorConfig := cardanofw.NewVectorChainConfig()
		vectorConfig.FundAmount = 0
		vectorConfig.FundTokenAmount = 0

		apex := cardanofw.SetupAndRunSkylineBridge(
			t, ctx,
			cardanofw.WithAPIKey(apiKey),
			cardanofw.WithUserCnt(userCnt),
			cardanofw.WithPrimeConfig(primeConfig),
			cardanofw.WithCardanoConfig(cardanoConfig),
			cardanofw.WithVectorConfig(vectorConfig),
			cardanofw.WithBridgingAddrCnt(cardanofw.ChainIDPrime, bridgeAddrCnt),
		)

		defer require.True(t, apex.ApexBridgeProcessesRunning())

		//nolint:dupl
		var (
			bridgingRequests = []*bridingRequest{
				{src: cardanofw.ChainIDVector, dest: cardanofw.ChainIDCardano, sender: apex.Users[0], amount: big.NewInt(1), receiverIdx: 0, requestType: sendtx.BridgingTypeWrappedTokenOnSource},
				{src: cardanofw.ChainIDVector, dest: cardanofw.ChainIDCardano, sender: apex.Users[1], amount: big.NewInt(100), receiverIdx: 1, requestType: sendtx.BridgingTypeWrappedTokenOnSource},
				{src: cardanofw.ChainIDCardano, dest: cardanofw.ChainIDPrime, sender: apex.Users[0], amount: big.NewInt(1), receiverIdx: 0, requestType: sendtx.BridgingTypeWrappedTokenOnSource},
				{src: cardanofw.ChainIDCardano, dest: cardanofw.ChainIDPrime, sender: apex.Users[1], amount: big.NewInt(100), receiverIdx: 1, requestType: sendtx.BridgingTypeWrappedTokenOnSource},
			}

			receivers = map[uint]*cardanofw.TestApexUser{
				0: apex.Users[userCnt-1],
				1: apex.Users[userCnt-2],
			}

			bridgignType = sendtx.BridgingTypeWrappedTokenOnSource
		)

		isNativeToken := bridgignType == sendtx.BridgingTypeCurrencyOnSource

		fundTestUsersWithToken(t, ctx, apex, []*testConfig{
			{
				srcChainID:      cardanofw.ChainIDVector,
				srcMinterWallet: apex.VectorInfo.GenesisWallet,
			},
			{
				srcChainID:      cardanofw.ChainIDCardano,
				srcMinterWallet: apex.CardanoInfo.GenesisWallet,
			},
		}, apex.Users[:2], uint64(2_000_000), uint64(250_000_000))

		chainPrevAmounts, chainExpectedAmounts, chainReceivers, _, _, _ := createBridgingData(ctx, apex, bridgingRequests, receivers, nil, nil)

		bridgeTransactions(ctx, apex, bridgingRequests, receivers)

		fmt.Printf("Confirming that bridging requests will not be processed\n")

		errsPerChain := waitOnDestination(ctx, apex, chainPrevAmounts, chainExpectedAmounts, chainReceivers, 30, time.Second*10, isNativeToken)
		for chainKey, err := range errsPerChain {
			require.Error(t, err)
			fmt.Printf("As intended, %v TXs on %v not yet arrived\n", chainExpectedAmounts[chainKey], chainKey.chain)
		}

		require.NoError(t, fundWallets(ctx, apex, big.NewInt(10), isNativeToken))

		errsPerChain = waitOnDestination(ctx, apex, chainPrevAmounts, chainExpectedAmounts, chainReceivers, 30, time.Second*10, isNativeToken)
		for chainKey, err := range errsPerChain {
			if chainKey.receiver == 1 {
				require.Error(t, err)
				fmt.Printf("As intended, %v TXs on %v not yet arrived\n", chainExpectedAmounts[chainKey], chainKey)
			} else {
				require.NoError(t, err)
				fmt.Printf("%v TXs on %v confirmed\n", chainExpectedAmounts[chainKey], chainKey.chain)
			}
		}

		require.NoError(t, fundWallets(ctx, apex, big.NewInt(1000), isNativeToken))

		errsPerChain = waitOnDestination(ctx, apex, chainPrevAmounts, chainExpectedAmounts, chainReceivers, 200, time.Second*10, isNativeToken)
		for chainKey, err := range errsPerChain {
			require.NoError(t, err)
			fmt.Printf("%v TXs on %v confirmed\n", chainExpectedAmounts[chainKey], chainKey.chain)
		}
	})

	t.Run("5. Basic defund test - currency on src", func(t *testing.T) {
		ctx, cncl := context.WithCancel(context.Background())
		defer cncl()

		initialFundInDfm := cardanofw.ApexToDfm(big.NewInt(100))

		primeConfig, cardanoConfig := cardanofw.NewPrimeChainConfig(), cardanofw.NewCardanoChainConfig(true)
		primeConfig.FundAmount = cardanofw.DfmToChainNativeTokenAmount(cardanofw.ChainIDPrime, initialFundInDfm).Uint64()
		cardanoConfig.FundAmount = cardanofw.DfmToChainNativeTokenAmount(cardanofw.ChainIDCardano, initialFundInDfm).Uint64()
		cardanoConfig.FundTokenAmount = cardanofw.DfmToChainNativeTokenAmount(cardanofw.ChainIDCardano, initialFundInDfm).Uint64()

		vectorConfig := cardanofw.NewVectorChainConfig()
		vectorConfig.FundAmount = cardanofw.DfmToChainNativeTokenAmount(cardanofw.ChainIDVector, initialFundInDfm).Uint64()
		vectorConfig.FundTokenAmount = cardanofw.DfmToChainNativeTokenAmount(cardanofw.ChainIDVector, initialFundInDfm).Uint64()

		apex := cardanofw.SetupAndRunSkylineBridge(
			t, ctx,
			cardanofw.WithAPIKey(apiKey),
			cardanofw.WithUserCnt(userCnt),
			cardanofw.WithPrimeConfig(primeConfig),
			cardanofw.WithCardanoConfig(cardanoConfig),
			cardanofw.WithVectorConfig(vectorConfig),
			cardanofw.WithBridgingAddrCnt(cardanofw.ChainIDPrime, bridgeAddrCnt),
		)

		defer require.True(t, apex.ApexBridgeProcessesRunning())

		// give time for oracles to submit hot wallet increment claims for initial fundings
		select {
		case <-ctx.Done():
			return
		case <-time.After(90 * time.Second):
		}

		var (
			defundReceiver          = apex.Users[userCnt-2]
			apexDefundAndFundAmount = big.NewInt(70)
			apexSendAmount          = big.NewInt(50)

			bridgignType = sendtx.BridgingTypeCurrencyOnSource

			bridgingRequests = []*bridingRequest{
				{src: cardanofw.ChainIDPrime, dest: cardanofw.ChainIDCardano, sender: apex.Users[0], amount: apexSendAmount, receiverIdx: 0, requestType: sendtx.BridgingTypeCurrencyOnSource},
				{src: cardanofw.ChainIDCardano, dest: cardanofw.ChainIDVector, sender: apex.Users[0], amount: apexSendAmount, receiverIdx: 0, requestType: sendtx.BridgingTypeCurrencyOnSource},
			}

			receivers = map[uint]*cardanofw.TestApexUser{
				0: apex.Users[userCnt-1],
			}
		)

		isNativeToken := bridgignType == sendtx.BridgingTypeCurrencyOnSource

		chainPrevAmounts, chainExpectedAmounts, chainReceivers,
			defundReceiversPrevAmount, defundReceiversExpectedAmount, defundReceivers :=
			createBridgingData(ctx, apex, bridgingRequests, receivers, defundReceiver, apexDefundAndFundAmount)

		defundWallets(ctx, apex, defundReceiver, apexDefundAndFundAmount,
			defundReceiversPrevAmount, defundReceiversExpectedAmount, defundReceivers, isNativeToken)

		bridgeTransactions(ctx, apex, bridgingRequests, receivers)

		fmt.Printf("Confirming that bridging requests will not be processed\n")

		errsPerChain := waitOnDestination(ctx, apex, chainPrevAmounts, chainExpectedAmounts, chainReceivers, 30, time.Second*10, isNativeToken)
		for chain, err := range errsPerChain {
			require.Error(t, err)
			fmt.Printf("As intended, %v TXs on %v not yet arrived\n", chainExpectedAmounts[chain], chain)
		}

		require.NoError(t, fundWallets(ctx, apex, apexDefundAndFundAmount, isNativeToken))

		errsPerChain = waitOnDestination(ctx, apex, chainPrevAmounts, chainExpectedAmounts, chainReceivers, 200, time.Second*10, isNativeToken)
		for chain, err := range errsPerChain {
			require.NoError(t, err)
			fmt.Printf("%v TXs on %v confirmed\n", chainExpectedAmounts[chain], chain)
		}
	})

	t.Run("6. Defund after bridging request is sent - currency on src", func(t *testing.T) {
		ctx, cncl := context.WithCancel(context.Background())
		defer cncl()

		initialFundInDfm := cardanofw.ApexToDfm(big.NewInt(100))

		primeConfig, cardanoConfig := cardanofw.NewPrimeChainConfig(), cardanofw.NewCardanoChainConfig(true)
		primeConfig.FundAmount = cardanofw.DfmToChainNativeTokenAmount(cardanofw.ChainIDPrime, initialFundInDfm).Uint64()
		cardanoConfig.FundAmount = cardanofw.DfmToChainNativeTokenAmount(cardanofw.ChainIDCardano, initialFundInDfm).Uint64()
		cardanoConfig.FundTokenAmount = cardanofw.DfmToChainNativeTokenAmount(cardanofw.ChainIDCardano, initialFundInDfm).Uint64()

		vectorConfig := cardanofw.NewVectorChainConfig()
		vectorConfig.FundAmount = cardanofw.DfmToChainNativeTokenAmount(cardanofw.ChainIDVector, initialFundInDfm).Uint64()
		vectorConfig.FundTokenAmount = cardanofw.DfmToChainNativeTokenAmount(cardanofw.ChainIDVector, initialFundInDfm).Uint64()

		apex := cardanofw.SetupAndRunSkylineBridge(
			t, ctx,
			cardanofw.WithAPIKey(apiKey),
			cardanofw.WithUserCnt(userCnt),
			cardanofw.WithPrimeConfig(primeConfig),
			cardanofw.WithCardanoConfig(cardanoConfig),
			cardanofw.WithVectorConfig(vectorConfig),
			cardanofw.WithBridgingAddrCnt(cardanofw.ChainIDPrime, bridgeAddrCnt),
		)

		defer require.True(t, apex.ApexBridgeProcessesRunning())

		// give time for oracles to submit hot wallet increment claims for initial fundings
		select {
		case <-ctx.Done():
			return
		case <-time.After(90 * time.Second):
		}

		var (
			defundReceiver          = apex.Users[userCnt-2]
			apexDefundAndFundAmount = big.NewInt(70)
			apexSendAmount          = big.NewInt(50)

			bridgignType = sendtx.BridgingTypeCurrencyOnSource

			bridgingRequests = []*bridingRequest{
				{src: cardanofw.ChainIDPrime, dest: cardanofw.ChainIDCardano, sender: apex.Users[0], amount: apexSendAmount, receiverIdx: 0, requestType: sendtx.BridgingTypeCurrencyOnSource},
				{src: cardanofw.ChainIDCardano, dest: cardanofw.ChainIDVector, sender: apex.Users[1], amount: apexSendAmount, receiverIdx: 0, requestType: sendtx.BridgingTypeCurrencyOnSource},
			}

			receivers = map[uint]*cardanofw.TestApexUser{
				0: apex.Users[userCnt-1],
			}
		)

		isNativeToken := bridgignType == sendtx.BridgingTypeCurrencyOnSource

		chainPrevAmounts, chainExpectedAmounts, chainReceivers, _, _, _ :=
			createBridgingData(ctx, apex, bridgingRequests, receivers, defundReceiver, apexDefundAndFundAmount)

		for _, request := range bridgingRequests {
			bridgeTransactions(ctx, apex, []*bridingRequest{request}, receivers)

			require.NoError(t, apex.DefundHotWallet(
				request.dest, defundReceiver.GetAddress(request.dest), cardanofw.ApexToDfm(apexDefundAndFundAmount), cardanofw.ApexToDfm(apexDefundAndFundAmount)))
		}

		fmt.Printf("Confirming that bridging requests will not be processed\n")

		errsPerChain := waitOnDestination(ctx, apex, chainPrevAmounts, chainExpectedAmounts, chainReceivers, 30, time.Second*10, isNativeToken)
		for chainKey, err := range errsPerChain {
			require.Error(t, err)
			fmt.Printf("As intended, %v TX on %v not yet arrived\n", chainExpectedAmounts[chainKey], chainKey.chain)
		}

		require.NoError(t, fundWallets(ctx, apex, apexDefundAndFundAmount, isNativeToken))

		errsPerChain = waitOnDestination(ctx, apex, chainPrevAmounts, chainExpectedAmounts, chainReceivers, 200, time.Second*10, isNativeToken)
		for chainKey, err := range errsPerChain {
			require.NoError(t, err)
			fmt.Printf("%v TX on %v confirmed\n", chainExpectedAmounts[chainKey], chainKey.chain)
		}
	})

	t.Run("7. Fund_Parallel_Send_BRs_Then_Full_Fund - currency on src", func(t *testing.T) {
		ctx, cncl := context.WithCancel(context.Background())
		defer cncl()

		primeConfig, cardanoConfig := cardanofw.NewPrimeChainConfig(), cardanofw.NewCardanoChainConfig(true)
		primeConfig.FundAmount = 0
		cardanoConfig.FundAmount = 0
		primeConfig.FundTokenAmount = 0
		cardanoConfig.FundTokenAmount = 0

		vectorConfig := cardanofw.NewVectorChainConfig()
		vectorConfig.FundAmount = 0
		vectorConfig.FundTokenAmount = 0

		apex := cardanofw.SetupAndRunSkylineBridge(
			t, ctx,
			cardanofw.WithAPIKey(apiKey),
			cardanofw.WithUserCnt(userCnt),
			cardanofw.WithPrimeConfig(primeConfig),
			cardanofw.WithCardanoConfig(cardanoConfig),
			cardanofw.WithVectorConfig(vectorConfig),
			cardanofw.WithBridgingAddrCnt(cardanofw.ChainIDPrime, bridgeAddrCnt),
		)

		defer require.True(t, apex.ApexBridgeProcessesRunning())

		var (
			bridgingRequests = []*bridingRequest{
				{src: cardanofw.ChainIDPrime, dest: cardanofw.ChainIDCardano, sender: apex.Users[0], amount: big.NewInt(1), receiverIdx: 0, requestType: sendtx.BridgingTypeCurrencyOnSource},
				{src: cardanofw.ChainIDCardano, dest: cardanofw.ChainIDVector, sender: apex.Users[0], amount: big.NewInt(1), receiverIdx: 0, requestType: sendtx.BridgingTypeCurrencyOnSource},
			}

			receivers = map[uint]*cardanofw.TestApexUser{
				0: apex.Users[userCnt-1],
			}

			bridgignType = sendtx.BridgingTypeCurrencyOnSource
		)

		isNativeToken := bridgignType == sendtx.BridgingTypeCurrencyOnSource

		chainPrevAmounts, chainExpectedAmounts, chainReceivers, _, _, _ := createBridgingData(ctx, apex, bridgingRequests, receivers, nil, nil)

		bridgeTransactions(ctx, apex, bridgingRequests, receivers)

		fmt.Printf("Confirming that bridging requests will not be processed\n")

		errsPerChain := waitOnDestination(ctx, apex, chainPrevAmounts, chainExpectedAmounts, chainReceivers, 30, time.Second*10, isNativeToken)
		for chainKey, err := range errsPerChain {
			require.Error(t, err)
			fmt.Printf("As intended, %v TXs on %v not yet arrived\n", chainExpectedAmounts[chainKey], chainKey.chain)
		}

		require.NoError(t, fundWallets(ctx, apex, big.NewInt(100), isNativeToken))

		errsPerChain = waitOnDestination(ctx, apex, chainPrevAmounts, chainExpectedAmounts, chainReceivers, 200, time.Second*10, isNativeToken)
		for chainKey, err := range errsPerChain {
			require.NoError(t, err)
			fmt.Printf("%v TXs on %v confirmed\n", chainExpectedAmounts[chainKey], chainKey)
		}
	})

	t.Run("8. Fund_Parallel_Send_BRs_Then_Fund_Twice - currency on src", func(t *testing.T) {
		ctx, cncl := context.WithCancel(context.Background())
		defer cncl()

		primeConfig, cardanoConfig := cardanofw.NewPrimeChainConfig(), cardanofw.NewCardanoChainConfig(true)
		primeConfig.FundAmount = 0
		cardanoConfig.FundAmount = 0
		primeConfig.FundTokenAmount = 0
		cardanoConfig.FundTokenAmount = 0

		vectorConfig := cardanofw.NewVectorChainConfig()
		vectorConfig.FundAmount = 0
		vectorConfig.FundTokenAmount = 0

		apex := cardanofw.SetupAndRunSkylineBridge(
			t, ctx,
			cardanofw.WithAPIKey(apiKey),
			cardanofw.WithUserCnt(userCnt),
			cardanofw.WithPrimeConfig(primeConfig),
			cardanofw.WithCardanoConfig(cardanoConfig),
			cardanofw.WithVectorConfig(vectorConfig),
			cardanofw.WithBridgingAddrCnt(cardanofw.ChainIDPrime, bridgeAddrCnt),
		)

		defer require.True(t, apex.ApexBridgeProcessesRunning())

		//nolint:dupl
		var (
			bridgingRequests = []*bridingRequest{
				{src: cardanofw.ChainIDPrime, dest: cardanofw.ChainIDCardano, sender: apex.Users[0], amount: big.NewInt(1), receiverIdx: 0, requestType: sendtx.BridgingTypeCurrencyOnSource},
				{src: cardanofw.ChainIDPrime, dest: cardanofw.ChainIDCardano, sender: apex.Users[1], amount: big.NewInt(100), receiverIdx: 1, requestType: sendtx.BridgingTypeCurrencyOnSource},
				{src: cardanofw.ChainIDCardano, dest: cardanofw.ChainIDVector, sender: apex.Users[0], amount: big.NewInt(1), receiverIdx: 0, requestType: sendtx.BridgingTypeCurrencyOnSource},
				{src: cardanofw.ChainIDCardano, dest: cardanofw.ChainIDVector, sender: apex.Users[1], amount: big.NewInt(100), receiverIdx: 1, requestType: sendtx.BridgingTypeCurrencyOnSource},
			}

			receivers = map[uint]*cardanofw.TestApexUser{
				0: apex.Users[userCnt-1],
				1: apex.Users[userCnt-2],
			}

			bridgignType = sendtx.BridgingTypeCurrencyOnSource
		)

		isNativeToken := bridgignType == sendtx.BridgingTypeCurrencyOnSource

		chainPrevAmounts, chainExpectedAmounts, chainReceivers, _, _, _ := createBridgingData(ctx, apex, bridgingRequests, receivers, nil, nil)

		bridgeTransactions(ctx, apex, bridgingRequests, receivers)

		fmt.Printf("Confirming that bridging requests will not be processed\n")

		errsPerChain := waitOnDestination(ctx, apex, chainPrevAmounts, chainExpectedAmounts, chainReceivers, 30, time.Second*10, isNativeToken)
		for chainKey, err := range errsPerChain {
			require.Error(t, err)
			fmt.Printf("As intended, %v TXs on %v not yet arrived\n", chainExpectedAmounts[chainKey], chainKey.chain)
		}

		apex.Config.PrimeConfig.FundAmount = cardanofw.DfmToChainNativeTokenAmount(cardanofw.ChainIDPrime, cardanofw.ApexToDfm(big.NewInt(10))).Uint64()
		apex.Config.CardanoConfig.FundAmount = cardanofw.DfmToChainNativeTokenAmount(cardanofw.ChainIDPrime, cardanofw.ApexToDfm(big.NewInt(10))).Uint64()
		apex.Config.CardanoConfig.FundTokenAmount = cardanofw.DfmToChainNativeTokenAmount(cardanofw.ChainIDPrime, cardanofw.ApexToDfm(big.NewInt(10))).Uint64()
		apex.Config.VectorConfig.FundAmount = cardanofw.DfmToChainNativeTokenAmount(cardanofw.ChainIDVector, cardanofw.ApexToDfm(big.NewInt(10))).Uint64()
		apex.Config.VectorConfig.FundTokenAmount = cardanofw.DfmToChainNativeTokenAmount(cardanofw.ChainIDVector, cardanofw.ApexToDfm(big.NewInt(10))).Uint64()

		require.NoError(t, apex.FundWallets(ctx))

		errsPerChain = waitOnDestination(ctx, apex, chainPrevAmounts, chainExpectedAmounts, chainReceivers, 30, time.Second*10, isNativeToken)
		for chainKey, err := range errsPerChain {
			if chainKey.receiver == 1 {
				require.Error(t, err)
				fmt.Printf("As intended, %v TXs on %v not yet arrived\n", chainExpectedAmounts[chainKey], chainKey)
			} else {
				require.NoError(t, err)
				fmt.Printf("%v TXs on %v confirmed\n", chainExpectedAmounts[chainKey], chainKey.chain)
			}
		}

		apex.Config.PrimeConfig.FundAmount = cardanofw.DfmToChainNativeTokenAmount(cardanofw.ChainIDPrime, cardanofw.ApexToDfm(big.NewInt(1000))).Uint64()
		apex.Config.CardanoConfig.FundAmount = cardanofw.DfmToChainNativeTokenAmount(cardanofw.ChainIDPrime, cardanofw.ApexToDfm(big.NewInt(1000))).Uint64()
		apex.Config.CardanoConfig.FundTokenAmount = cardanofw.DfmToChainNativeTokenAmount(cardanofw.ChainIDPrime, cardanofw.ApexToDfm(big.NewInt(1000))).Uint64()
		apex.Config.VectorConfig.FundAmount = cardanofw.DfmToChainNativeTokenAmount(cardanofw.ChainIDVector, cardanofw.ApexToDfm(big.NewInt(1000))).Uint64()
		apex.Config.VectorConfig.FundTokenAmount = cardanofw.DfmToChainNativeTokenAmount(cardanofw.ChainIDVector, cardanofw.ApexToDfm(big.NewInt(1000))).Uint64()

		require.NoError(t, apex.FundWallets(ctx))

		errsPerChain = waitOnDestination(ctx, apex, chainPrevAmounts, chainExpectedAmounts, chainReceivers, 200, time.Second*10, isNativeToken)
		for chainKey, err := range errsPerChain {
			require.NoError(t, err)
			fmt.Printf("%v TXs on %v confirmed\n", chainExpectedAmounts[chainKey], chainKey.chain)
		}
	})
}

func TestE2E_SkylineBridge_ValidScenarios_BigTests_AllDirections(t *testing.T) {
	if !cardanofw.IsEnvVarTrue("RUN_E2E_SKYLINE_BIG_TESTS") {
		t.Skip()
	}

	const (
		apiKey  = "test_api_key"
		userCnt = 2010 // max 1000 parallel instances, userCnot >= 2 * instances + 1
	)

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	primeConfig, cardanoConfig := cardanofw.NewPrimeChainConfig(), cardanofw.NewCardanoChainConfig(true)
	primeConfig.PremineAmount = 30_000_000_000
	cardanoConfig.PremineAmount = 30_000_000_000
	cardanoConfig.FundTokenAmount = 1_000_000_000

	vectorConfig := cardanofw.NewVectorChainConfig()
	vectorConfig.FundTokenAmount = 1_000_000_000
	vectorConfig.PremineAmount = 30_000_000_000
	vectorConfig.UseIndexer = true

	apex := cardanofw.SetupAndRunSkylineBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithUserCnt(userCnt),
		cardanofw.WithPrimeConfig(primeConfig),
		cardanofw.WithCardanoConfig(cardanoConfig),
		cardanofw.WithVectorConfig(vectorConfig),
		cardanofw.WithBridgingAddrCnt(cardanofw.ChainIDPrime, bridgeAddrCnt),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	currencyReceiver := apex.Users[userCnt-1]
	nativeTokenReceiver := apex.Users[userCnt-2]

	fmt.Println("prime user addr: ", currencyReceiver.PrimeAddress)
	fmt.Println("cardano user addr: ", currencyReceiver.CardanoAddress)
	fmt.Println("prime multisig addr: ", apex.PrimeInfo.MultisigAddr[0])
	fmt.Println("prime fee addr: ", apex.PrimeInfo.FeeAddr)
	fmt.Println("cardano multisig addr: ", apex.CardanoInfo.MultisigAddr[0])
	fmt.Println("cardano fee addr: ", apex.CardanoInfo.FeeAddr)
	fmt.Println("vector user addr: ", currencyReceiver.VectorAddress)
	fmt.Println("vector multisig addr: ", apex.VectorInfo.MultisigAddr)
	fmt.Println("vector fee addr: ", apex.VectorInfo.FeeAddr)
	fmt.Printf("vector socket path: %s\n", apex.VectorInfo.SocketPath)

	t.Run("Both directions 1000x 60min 90%", func(t *testing.T) {
		const (
			instances     = 1000
			maxWaitTime   = 3600
			successChance = 90 // 90%

			// wait for tx timeout
			numRetries = 500
			waitTime   = time.Second * 10
		)

		sendAmount := new(big.Int).SetInt64(1_000_000)

		type bridgingRequest struct {
			src            cardanofw.ChainID
			dest           cardanofw.ChainID
			firstSenderIdx int
			bridgingType   sendtx.BridgingType
			receiver       *cardanofw.TestApexUser
			multiSigAddr   string
		}

		bridgingRequests := []bridgingRequest{
			{src: cardanofw.ChainIDVector, multiSigAddr: apex.VectorInfo.MultisigAddr[0], dest: cardanofw.ChainIDCardano, firstSenderIdx: 0, bridgingType: sendtx.BridgingTypeWrappedTokenOnSource, receiver: currencyReceiver},
			{src: cardanofw.ChainIDCardano, multiSigAddr: apex.CardanoInfo.MultisigAddr[0], dest: cardanofw.ChainIDPrime, firstSenderIdx: 0, bridgingType: sendtx.BridgingTypeWrappedTokenOnSource, receiver: currencyReceiver},
			{src: cardanofw.ChainIDPrime, multiSigAddr: apex.PrimeInfo.MultisigAddr[0], dest: cardanofw.ChainIDCardano, firstSenderIdx: instances, bridgingType: sendtx.BridgingTypeCurrencyOnSource, receiver: nativeTokenReceiver},
			{src: cardanofw.ChainIDCardano, multiSigAddr: apex.CardanoInfo.MultisigAddr[0], dest: cardanofw.ChainIDVector, firstSenderIdx: instances, bridgingType: sendtx.BridgingTypeCurrencyOnSource, receiver: nativeTokenReceiver},
		}

		seed := rand.Int63n(1_000_000_000)
		r := rand.New(rand.NewSource(seed)) // New seeded random number generator

		fmt.Printf("Test seed: %v\n", seed)

		fmt.Printf("Funding users with native tokens\n")

		fundTestUsersWithToken(t, ctx, apex, []*testConfig{
			{
				srcChainID:      cardanofw.ChainIDVector,
				srcMinterWallet: apex.VectorInfo.GenesisWallet,
			},
			{
				srcChainID:      cardanofw.ChainIDCardano,
				srcMinterWallet: apex.CardanoInfo.GenesisWallet,
			},
		}, apex.Users[:instances], uint64(1_100_000_000), uint64(2_500_000))

		fmt.Printf("Sending %v transactions in %v seconds\n", instances*len(bridgingRequests), maxWaitTime)

		prevAmounts := make(map[int]map[string]*big.Int)
		expectedAmounts := make(map[int]*big.Int)

		var wg sync.WaitGroup

		for brIdx, br := range bridgingRequests {
			var err error

			succeededCount := int64(0)

			prevAmounts[brIdx], err = apex.GetBalance(ctx, br.receiver, br.dest)
			require.NoError(t, err)

			var tokenName string

			tokenID := apex.GetTokenIDForChain(br.src, br.bridgingType == sendtx.BridgingTypeCurrencyOnSource)
			tokenName = apex.GetTokenNameForChains(br.dest, br.src, tokenID)

			if amount, ok := prevAmounts[brIdx][tokenName]; ok {
				expectedAmounts[brIdx] = new(big.Int).Set(amount)
			} else {
				expectedAmounts[brIdx] = big.NewInt(0)
			}

			for i := 0; i < instances; i++ {
				success := successChance > r.Intn(100)
				if success {
					succeededCount++
				}

				wg.Add(1)

				go func(idx int, br bridgingRequest, valid bool) {
					defer wg.Done()

					if valid {
						time.Sleep(time.Second * time.Duration(r.Intn(maxWaitTime)))

						_, err := apex.SubmitBridgingRequest(ctx, br.src, br.dest, apex.Users[idx], sendAmount, br.bridgingType, br.receiver)
						require.NoError(t, err)
					} else {
						sendInvalidSendAmountTransaction(t, ctx, apex, br.src, br.dest, apex.Users[idx], sendAmount, br.receiver.GetAddress(br.dest), br.multiSigAddr)
					}
				}(br.firstSenderIdx+i, br, success)
			}

			totalSent := new(big.Int).Mul(sendAmount, big.NewInt(succeededCount))
			expectedAmounts[brIdx].Add(expectedAmounts[brIdx], totalSent)
		}

		wg.Wait()

		fmt.Printf("All tx sent, waiting for confirmation.\n")

		for i, br := range bridgingRequests {
			wg.Add(1)

			go func(brIdx int, br bridgingRequest) {
				defer wg.Done()

				var tokenName string

				tokenID := apex.GetTokenIDForChain(br.src, br.bridgingType == sendtx.BridgingTypeCurrencyOnSource)
				tokenName = apex.GetTokenNameForChains(br.dest, br.src, tokenID)

				prevAmount := prevAmounts[brIdx][tokenName]

				if prevAmount == nil {
					prevAmount = big.NewInt(0)
				}

				succeededCount := new(big.Int).Sub(expectedAmounts[brIdx], prevAmount).Uint64() / sendAmount.Uint64()

				fmt.Printf("Waiting for %+v TXs on %s, prevAmount: %v, expectedAmount: %v\n",
					succeededCount, br.dest, prevAmounts[brIdx], expectedAmounts[brIdx])

				err := apex.WaitForExactAmount(ctx, br.receiver, br.dest, br.src, expectedAmounts[brIdx], numRetries, waitTime,
					br.bridgingType == sendtx.BridgingTypeCurrencyOnSource)
				require.NoError(t, err)

				fmt.Printf("TXs on %s confirmed\n", br.dest)
			}(i, br)
		}

		wg.Wait()
	})
}

func sendInvalidSendAmountTransaction(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, src, dest cardanofw.ChainID, senderUser *cardanofw.TestApexUser, sendAmount *big.Int,
	receiverUserAddr string, multiSigAddr string,
) {
	t.Helper()

	receivers := []sendtx.BridgingTxReceiver{
		{
			Addr:   receiverUserAddr,
			Amount: sendAmount.Uint64() * 10,
			// TODO: FIX THIS UP WITH ID
			// BridgingType: sendtx.BridgingTypeCurrencyOnSource,
		},
	}

	srcTestChain := apex.GetChainMust(t, src)

	operationFee := apex.GetMinOperationFee(src)

	feeAmount, err := srcTestChain.GetBridgingFee(
		ctx, dest, receivers, apex.GetMinBridgingFee(src, false), operationFee, multiSigAddr)
	require.NoError(t, err)

	metadata, err := srcTestChain.CreateMetadata(
		senderUser.GetAddress(src), dest,
		receivers, feeAmount, operationFee)
	require.NoError(t, err)

	_, err = apex.SubmitTx(
		ctx, src, senderUser, srcTestChain.GetHotWalletAddresses()[0],
		new(big.Int).Add(sendAmount, new(big.Int).SetUint64(feeAmount+operationFee)), nil, metadata)
	require.NoError(t, err)
}

func TestE2E_SkylineBridge_DisabledDirection(t *testing.T) {
	if cardanofw.ShouldSkipE2RRedundantTests() {
		t.Skip()
	}

	type bridgingRequest struct {
		src         string
		dest        string
		sender      *cardanofw.TestApexUser
		requestType sendtx.BridgingType
		isValid     bool
	}

	const apiKey = "test_api_key"

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	primeConfig, vectorConfig, cardanoConfig := cardanofw.NewPrimeChainConfig(), cardanofw.NewVectorChainConfig(), cardanofw.NewCardanoChainConfig(true)
	primeConfig.FundTokenAmount = 0 // very important otherwise HWIC wont work
	vectorConfig.FundTokenAmount = 0

	apex := cardanofw.SetupAndRunSkylineBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithUserCnt(3),
		cardanofw.WithCardanoConfig(cardanoConfig),
		cardanofw.WithPrimeConfig(primeConfig),
		cardanofw.WithVectorConfig(vectorConfig),
		cardanofw.WithBridgingAddrCnt(cardanofw.ChainIDPrime, bridgeAddrCnt),
		cardanofw.WithCustomConfigHandlers(func(_ *cardanofw.ApexSystem, mp map[string]interface{}) {
			primeSettings := cardanofw.GetMapFromInterfaceKey(mp, "cardanoChains", "prime")
			primeSettings["nativeTokens"] = nil
			vectorSettings := cardanofw.GetMapFromInterfaceKey(mp, "cardanoChains", "vector")
			vectorSettings["nativeTokens"] = nil
			mp["refundEnabled"] = false
		}, nil),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	var (
		user             = apex.Users[0]
		apexSendAmount   = cardanofw.ApexToDfm(big.NewInt(2))
		bridgingRequests = []bridgingRequest{
			{src: cardanofw.ChainIDPrime, dest: cardanofw.ChainIDCardano, sender: apex.Users[1], requestType: sendtx.BridgingTypeCurrencyOnSource, isValid: true},
			{src: cardanofw.ChainIDVector, dest: cardanofw.ChainIDCardano, sender: apex.Users[2], requestType: sendtx.BridgingTypeWrappedTokenOnSource, isValid: false},
			{src: cardanofw.ChainIDCardano, dest: cardanofw.ChainIDVector, sender: apex.Users[1], requestType: sendtx.BridgingTypeCurrencyOnSource, isValid: false},
			{src: cardanofw.ChainIDCardano, dest: cardanofw.ChainIDPrime, sender: apex.Users[2], requestType: sendtx.BridgingTypeWrappedTokenOnSource, isValid: true},
		}
		txHashes = make([]string, len(bridgingRequests))
	)

	var wg sync.WaitGroup

	for idx, br := range bridgingRequests {
		wg.Add(1)

		go func(i int, br bridgingRequest) {
			var err error

			defer wg.Done()

			tokenName := cardanofw.CAP3XTokenName
			if br.src == cardanofw.ChainIDVector {
				tokenName = cardanofw.XADATokenName
			}

			if br.requestType == sendtx.BridgingTypeWrappedTokenOnSource {
				_, err := cardanofw.FundUserWithToken(
					ctx, apex, br.src,
					apex.GetCardanoInfo(br.src).GenesisWallet, br.sender,
					tokenName, cardanofw.DefaultTokenMintAmount,
					uint64(10_000_000), uint64(100_000_000))
				require.NoError(t, err)
			}

			txHashes[i], err = apex.SubmitBridgingRequest(ctx, br.src, br.dest, br.sender, apexSendAmount, br.requestType, user)
			require.NoError(t, err)

			fmt.Printf("Bridging request: %v to %v sent %v. hash: %s\n", br.src, br.dest, br.requestType, txHashes[i])
		}(idx, br)
	}

	wg.Wait()

	for idx, br := range bridgingRequests {
		wg.Add(1)

		go func(src, hash string) {
			defer wg.Done()

			state, timeout := "ExecutedOnDestination", uint(60*8)
			if !br.isValid {
				state, timeout = "InvalidRequest", 60*5
			}

			_, err := cardanofw.WaitForRequestStates(ctx, apex, src, hash, apiKey, []string{state}, timeout)
			require.NoError(t, err)

			fmt.Printf("%s is %s\n", hash, state)
		}(br.src, txHashes[idx])
	}

	wg.Wait()
}

func fundTestUsersWithToken(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, testConfigs []*testConfig,
	users []*cardanofw.TestApexUser, amount, tokenAmount uint64,
) []*wallet.TokenAmount {
	t.Helper()

	wg := sync.WaitGroup{}
	errs := make([]error, len(testConfigs))
	tokenAmounts := make([]*wallet.TokenAmount, len(testConfigs))

	for i, tcfg := range testConfigs {
		wg.Add(1)

		go func(cfg *testConfig, indx int) {
			defer wg.Done()

			chain := apex.GetChainMust(t, cfg.srcChainID).(*cardanofw.TestCardanoChain)

			tokenName := cardanofw.CAP3XTokenName
			if cfg.srcChainID == cardanofw.ChainIDVector {
				tokenName = cardanofw.XADATokenName
			}

			errs[indx] = cardanofw.MintToken(
				chain, cfg.srcMinterWallet, tokenName, tokenAmount*uint64(len(users)))
			if errs[indx] != nil {
				return
			}

			tokenAmounts[indx], errs[indx] = cardanofw.FundUsersWithToken(
				ctx, chain, cfg.srcMinterWallet,
				users, tokenName, amount, tokenAmount)
		}(tcfg, i)
	}

	wg.Wait()

	require.NoError(t, errors.Join(errs...))

	return tokenAmounts
}
