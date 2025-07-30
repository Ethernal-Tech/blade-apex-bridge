package e2e

import (
	"context"
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
	"github.com/Ethernal-Tech/cardano-infrastructure/sendtx"
	"github.com/Ethernal-Tech/cardano-infrastructure/wallet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

	apex := cardanofw.SetupAndRunSkylineBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithCardanoConfig(cardanoConfig),
		cardanofw.WithPrimeConfig(primeConfig),
		cardanofw.WithUserCnt(1),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	oracleAPI, err := apex.GetBridgingAPI()
	require.NoError(t, err)

	fmt.Printf("oracle API: %s\n", oracleAPI)
	fmt.Printf("oracle API key: %s\n", apiKey)

	fmt.Printf("prime network url: %s\n", apex.PrimeInfo.NetworkAddress)
	fmt.Printf("prime ogmios url: %s\n", apex.PrimeInfo.OgmiosURL)
	fmt.Printf("prime bridging addr: %s\n", apex.PrimeInfo.MultisigAddr)
	fmt.Printf("prime fee addr: %s\n", apex.PrimeInfo.FeeAddr)
	fmt.Printf("prime socket path: %s\n", apex.PrimeInfo.SocketPath)

	fmt.Printf("cardano network url: %s\n", apex.CardanoInfo.NetworkAddress)
	fmt.Printf("cardano ogmios url: %s\n", apex.CardanoInfo.OgmiosURL)
	fmt.Printf("cardano bridging addr: %s\n", apex.CardanoInfo.MultisigAddr)
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
	const (
		apiKey  = "test_api_key"
		userCnt = 15
	)

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	primeConfig, cardanoConfig := cardanofw.NewPrimeChainConfig(), cardanofw.NewCardanoChainConfig(true)
	primeConfig.FundTokenAmount = 1_000_000_000
	cardanoConfig.FundTokenAmount = 1_000_000_000

	apex := cardanofw.SetupAndRunSkylineBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithUserCnt(userCnt),
		cardanofw.WithCardanoConfig(cardanoConfig),
		cardanofw.WithPrimeConfig(primeConfig),
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

	transactionTypes := []sendtx.BridgingType{
		sendtx.BridgingTypeCurrencyOnSource, sendtx.BridgingTypeNativeTokenOnSource,
	}
	testConfigPrime := newTestConfig(t, apex.Config.PrimeConfig, &apex.PrimeInfo, cardanofw.ChainIDCardano)
	testConfigCardano := newTestConfig(t, apex.Config.CardanoConfig, &apex.CardanoInfo, cardanofw.ChainIDPrime)
	testConfigs := []*testConfig{testConfigPrime, testConfigCardano}

	minterWalletPrime := apex.PrimeInfo.GenesisWallet
	minterWalletCardano := apex.CardanoInfo.GenesisWallet

	t.Run("1. prime -> cardano - currency on src", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() {
			t.Skip()
		}

		sendAmountDfm := big.NewInt(1_500_000)

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDPrime, cardanofw.ChainIDCardano, sendAmountDfm,
			sendtx.BridgingTypeCurrencyOnSource)
	})

	t.Run("2. prime -> cardano - native token on src", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() {
			t.Skip()
		}

		sendAmountDfm := big.NewInt(1_500_000)

		brSubmitterUser, err := cardanofw.NewTestApexUser(cardanofw.NewApexNetworkTypesFromSystem(apex))
		require.NoError(t, err)

		_, err = cardanofw.FundUserWithToken(
			ctx, apex, cardanofw.ChainIDPrime,
			minterWalletPrime, brSubmitterUser,
			cardanofw.DefaultTokenName, cardanofw.DefaultTokenMintAmount,
			uint64(1_100_000_000), uint64(2_500_000))
		require.NoError(t, err)

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, brSubmitterUser, user, cardanofw.ChainIDPrime, cardanofw.ChainIDCardano, sendAmountDfm,
			sendtx.BridgingTypeNativeTokenOnSource)
	})

	t.Run("3. cardano -> prime - currency on src", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() {
			t.Skip()
		}

		sendAmountDfm := big.NewInt(1_500_000)

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDCardano, cardanofw.ChainIDPrime, sendAmountDfm,
			sendtx.BridgingTypeCurrencyOnSource)
	})

	t.Run("4. cardano -> prime - native token on src", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() {
			t.Skip()
		}

		sendAmountDfm := big.NewInt(1_500_000)

		brSubmitterUser, err := cardanofw.NewTestApexUser(cardanofw.NewApexNetworkTypesFromSystem(apex))
		require.NoError(t, err)

		_, err = cardanofw.FundUserWithToken(
			ctx, apex, cardanofw.ChainIDCardano,
			minterWalletCardano, brSubmitterUser,
			cardanofw.DefaultTokenName, cardanofw.DefaultTokenMintAmount,
			uint64(1_100_000_000), uint64(2_500_000))
		require.NoError(t, err)

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, brSubmitterUser, user, cardanofw.ChainIDCardano, cardanofw.ChainIDPrime, sendAmountDfm,
			sendtx.BridgingTypeNativeTokenOnSource)
	})

	for idx, cfg := range testConfigs {
		for _, txType := range transactionTypes {
			t.Run(fmt.Sprintf("1.%d %s -> %s - Submitter has tokens - %s", idx+1, cfg.srcChainID, cfg.dstChainID, txType), func(t *testing.T) {
				if cardanofw.ShouldSkipE2RRedundantTests() {
					t.Skip()
				}

				sendAmountDfm := big.NewInt(5_000_000)
				minterUser := apex.Users[userCnt-2]

				brSubmitterUser, err := cardanofw.NewTestApexUser(cardanofw.NewApexNetworkTypesFromSystem(apex))
				require.NoError(t, err)

				minterWallet, _ := minterUser.GetCardanoWallet(cfg.srcChainID)

				_, err = cardanofw.FundUserWithToken(
					ctx, apex, cfg.srcChainID,
					minterWallet, brSubmitterUser,
					cardanofw.DefaultTokenName, cardanofw.DefaultTokenMintAmount,
					uint64(50_000_000), uint64(1_000_000))
				require.NoError(t, err)

				_, err = cardanofw.FundUserWithToken(
					ctx, apex, cfg.srcChainID,
					cfg.srcMinterWallet, brSubmitterUser,
					cardanofw.DefaultTokenName, cardanofw.DefaultTokenMintAmount,
					uint64(50_000_000), uint64(10_000_000))
				require.NoError(t, err)

				e2ehelper.ExecuteSingleBridging(
					t, ctx, apex, brSubmitterUser, user, cfg.srcChainID, cfg.dstChainID, sendAmountDfm,
					txType)
			})
		}
	}

	for idx, cfg := range testConfigs {
		for _, txType := range transactionTypes {
			t.Run(fmt.Sprintf("2.%d %s -> %s - wait for each submit - %s", idx+1, cfg.srcChainID, cfg.dstChainID, txType), func(t *testing.T) {
				if cardanofw.ShouldSkipE2RRedundantTests() {
					t.Skip()
				}

				const (
					sendAmount = uint64(1_000_000)
					instances  = 5
				)

				brSubmitterUser, err := cardanofw.NewTestApexUser(cardanofw.NewApexNetworkTypesFromSystem(apex))
				require.NoError(t, err)

				_, err = cardanofw.FundUserWithToken(
					ctx, apex, cfg.srcChainID,
					cfg.srcMinterWallet, brSubmitterUser,
					cardanofw.DefaultTokenName, cardanofw.DefaultTokenMintAmount,
					uint64(1_100_000_000), uint64(10_000_000))
				require.NoError(t, err)

				e2ehelper.ExecuteBridgingOneByOneWaitOnOtherSide(
					t, ctx, apex, instances, brSubmitterUser, cfg.srcChainID, cfg.dstChainID, new(big.Int).SetUint64(sendAmount),
					txType)
			})
		}
	}

	for idx, cfg := range testConfigs {
		for _, txType := range transactionTypes {
			t.Run(fmt.Sprintf("3.%d %s -> %s - one by one - %s", idx+1, cfg.srcChainID, cfg.dstChainID, txType), func(t *testing.T) {
				if cardanofw.ShouldSkipE2RRedundantTests() {
					t.Skip()
				}

				const (
					sendAmount = uint64(1_000_000)
					instances  = 5
				)

				brSubmitterUser, err := cardanofw.NewTestApexUser(cardanofw.NewApexNetworkTypesFromSystem(apex))
				require.NoError(t, err)

				_, err = cardanofw.FundUserWithToken(
					ctx, apex, cfg.srcChainID,
					cfg.srcMinterWallet, brSubmitterUser,
					cardanofw.DefaultTokenName, cardanofw.DefaultTokenMintAmount,
					uint64(1_100_000_000), uint64(10_000_000))
				require.NoError(t, err)

				e2ehelper.ExecuteBridgingWaitAfterSubmits(
					t, ctx, apex, instances, brSubmitterUser, cfg.srcChainID, cfg.dstChainID, new(big.Int).SetUint64(sendAmount),
					txType)
			})
		}
	}

	for idx, cfg := range testConfigs {
		for _, txType := range transactionTypes {
			t.Run(fmt.Sprintf("4.%d %s -> %s - parallel - %s", idx+1, cfg.srcChainID, cfg.dstChainID, txType), func(t *testing.T) {
				if cardanofw.ShouldSkipE2RRedundantTests() {
					t.Skip()
				}

				const (
					sendAmount = uint64(1_000_000)
					instances  = 5
				)

				for _, cfg := range testConfigs {
					for _, sender := range apex.Users[:instances] {
						_, err := cardanofw.FundUserWithToken(
							ctx, apex, cfg.srcChainID,
							cfg.srcMinterWallet, sender,
							cardanofw.DefaultTokenName, cardanofw.DefaultTokenMintAmount,
							uint64(1_100_000_000), uint64(10_000_000))
						require.NoError(t, err)
					}
				}

				e2ehelper.ExecuteBridging(
					t, ctx, apex, 1, apex.Users[:instances], []*cardanofw.TestApexUser{user},
					[]string{cfg.srcChainID},
					map[string][]string{
						cfg.srcChainID: {cfg.dstChainID},
					}, txType, new(big.Int).SetUint64(sendAmount))
			})
		}
	}

	for idx, cfg := range testConfigs {
		for _, txType := range transactionTypes {
			t.Run(fmt.Sprintf("5.%d %s -> %s - sequential and parallel - %s", idx+1, cfg.srcChainID, cfg.dstChainID, txType), func(t *testing.T) {
				if cardanofw.ShouldSkipE2RRedundantTests() {
					t.Skip()
				}

				const (
					sendAmount          = uint64(1_000_000)
					sequentialInstances = 5
					parallelInstances   = 10
					receivers           = 1
				)

				for _, cfg := range testConfigs {
					for _, sender := range apex.Users[:parallelInstances] {
						_, err := cardanofw.FundUserWithToken(
							ctx, apex, cfg.srcChainID,
							cfg.srcMinterWallet, sender,
							cardanofw.DefaultTokenName, cardanofw.DefaultTokenMintAmount,
							uint64(1_100_000_000), uint64(210_000_000))
						require.NoError(t, err)
					}
				}

				e2ehelper.ExecuteBridging(
					t, ctx, apex, sequentialInstances,
					apex.Users[:parallelInstances],
					apex.Users[:receivers],
					[]string{cfg.srcChainID},
					map[string][]string{
						cfg.srcChainID: {cfg.dstChainID},
					}, txType, new(big.Int).SetUint64(sendAmount),
				)
			})
		}
	}

	idx := 1

	for _, txType := range transactionTypes {
		t.Run(fmt.Sprintf("6.%d Both directions sequential - %s", idx, txType), func(t *testing.T) {
			if cardanofw.ShouldSkipE2RRedundantTests() {
				t.Skip()
			}

			const (
				sendAmount = uint64(1_000_000)
				instances  = 5
			)

			brSubmitterUser, err := cardanofw.NewTestApexUser(cardanofw.NewApexNetworkTypesFromSystem(apex))
			require.NoError(t, err)

			for _, cfg := range testConfigs {
				_, err = cardanofw.FundUserWithToken(
					ctx, apex, cfg.srcChainID,
					cfg.srcMinterWallet, brSubmitterUser,
					cardanofw.DefaultTokenName, cardanofw.DefaultTokenMintAmount,
					uint64(1_100_000_000), uint64(10_000_000))
				require.NoError(t, err)
			}

			e2ehelper.ExecuteBridging(
				t, ctx, apex, instances,
				[]*cardanofw.TestApexUser{brSubmitterUser},
				[]*cardanofw.TestApexUser{user},
				[]string{cardanofw.ChainIDPrime, cardanofw.ChainIDCardano},
				map[string][]string{
					cardanofw.ChainIDPrime:   {cardanofw.ChainIDCardano},
					cardanofw.ChainIDCardano: {cardanofw.ChainIDPrime},
				}, txType, new(big.Int).SetUint64(sendAmount),
			)
		})

		idx++
	}

	idx = 1

	for _, txType := range transactionTypes {
		t.Run(fmt.Sprintf("7.%d Both directions sequential and parallel - %s", idx, txType), func(t *testing.T) {
			const (
				sendAmount          = uint64(1_000_000)
				sequentialInstances = 5
				parallelInstances   = 6
			)

			for _, cfg := range testConfigs {
				for _, sender := range apex.Users[:parallelInstances] {
					_, err := cardanofw.FundUserWithToken(
						ctx, apex, cfg.srcChainID,
						cfg.srcMinterWallet, sender,
						cardanofw.DefaultTokenName, cardanofw.DefaultTokenMintAmount,
						uint64(1_100_000_000), uint64(10_000_000))
					require.NoError(t, err)
				}
			}

			e2ehelper.ExecuteBridging(
				t, ctx, apex, sequentialInstances,
				apex.Users[:parallelInstances],
				[]*cardanofw.TestApexUser{user},
				[]string{cardanofw.ChainIDPrime, cardanofw.ChainIDCardano},
				map[string][]string{
					cardanofw.ChainIDPrime:   {cardanofw.ChainIDCardano},
					cardanofw.ChainIDCardano: {cardanofw.ChainIDPrime},
				}, txType, new(big.Int).SetUint64(sendAmount),
				e2ehelper.WithWaitForUnexpectedBridges(true))
		})

		idx++
	}

	idx = 1

	for _, txType := range transactionTypes {
		t.Run(fmt.Sprintf("8.%d Both directions sequential and parallel - one node goes offline midway - %s",
			idx, txType), func(t *testing.T) {
			const (
				sendAmount           = uint64(1_000_000)
				sequentialInstances  = 5
				parallelInstances    = 6
				stopAfter            = time.Second * 60
				validatorStoppingIdx = 1
			)

			for _, cfg := range testConfigs {
				for _, sender := range apex.Users[:parallelInstances] {
					_, err := cardanofw.FundUserWithToken(
						ctx, apex, cfg.srcChainID,
						cfg.srcMinterWallet, sender,
						cardanofw.DefaultTokenName, cardanofw.DefaultTokenMintAmount,
						uint64(1_100_000_000), uint64(10_000_000))
					require.NoError(t, err)
				}
			}

			e2ehelper.ExecuteBridging(
				t, ctx, apex, sequentialInstances,
				apex.Users[:parallelInstances],
				[]*cardanofw.TestApexUser{user},
				[]string{cardanofw.ChainIDPrime, cardanofw.ChainIDCardano},
				map[string][]string{
					cardanofw.ChainIDPrime:   {cardanofw.ChainIDCardano},
					cardanofw.ChainIDCardano: {cardanofw.ChainIDPrime},
				}, txType, new(big.Int).SetUint64(sendAmount),
				e2ehelper.WithWaitForUnexpectedBridges(true),
				e2ehelper.WithRestartValidatorsConfig([]e2ehelper.RestartValidatorsConfig{
					{WaitTime: stopAfter, StopIndxs: []int{validatorStoppingIdx}},
				}))
		})

		idx++
	}

	idx = 1

	for _, txType := range transactionTypes {
		t.Run(fmt.Sprintf("9.%d Both directions sequential and parallel — two nodes go offline midway, one node recovers - %s",
			idx, txType), func(t *testing.T) {
			const (
				sequentialInstances   = 5
				parallelInstances     = 10
				stopAfter             = time.Second * 60
				startAgainAfter       = time.Second * 120
				validatorStoppingIdx1 = 1
				validatorStoppingIdx2 = 2
				sendAmount            = uint64(1_000_000)
			)

			for _, cfg := range testConfigs {
				for _, sender := range apex.Users[:parallelInstances] {
					_, err := cardanofw.FundUserWithToken(
						ctx, apex, cfg.srcChainID,
						cfg.srcMinterWallet, sender,
						cardanofw.DefaultTokenName, cardanofw.DefaultTokenMintAmount,
						uint64(1_100_000_000), uint64(10_000_000))
					require.NoError(t, err)
				}
			}

			e2ehelper.ExecuteBridging(
				t, ctx, apex, sequentialInstances,
				apex.Users[:parallelInstances],
				[]*cardanofw.TestApexUser{user},
				[]string{cardanofw.ChainIDPrime, cardanofw.ChainIDCardano},
				map[string][]string{
					cardanofw.ChainIDPrime:   {cardanofw.ChainIDCardano},
					cardanofw.ChainIDCardano: {cardanofw.ChainIDPrime},
				}, txType, new(big.Int).SetUint64(sendAmount),
				e2ehelper.WithWaitForUnexpectedBridges(true),
				e2ehelper.WithRestartValidatorsConfig([]e2ehelper.RestartValidatorsConfig{
					{WaitTime: stopAfter, StopIndxs: []int{validatorStoppingIdx1, validatorStoppingIdx2}},
					{WaitTime: startAgainAfter, StartIndxs: []int{validatorStoppingIdx1}},
				}))
		})

		idx++
	}
}

func TestE2E_SkylineBridge_InvalidScenarios(t *testing.T) {
	const (
		apiKey  = "test_api_key"
		userCnt = 15

		bridgingFee  = uint64(1_000_010)
		operationFee = uint64(0)
	)

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	primeConfig, cardanoConfig := cardanofw.NewPrimeChainConfig(), cardanofw.NewCardanoChainConfig(true)
	primeConfig.FundTokenAmount = 1_000_000_000
	cardanoConfig.FundTokenAmount = 1_000_000_000

	apex := cardanofw.SetupAndRunSkylineBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithUserCnt(userCnt),
		cardanofw.WithCardanoConfig(cardanoConfig),
		cardanofw.WithPrimeConfig(primeConfig),
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

	t.Run("1. Mismatch submitted and receiver amounts", func(t *testing.T) {
		executeInvalidMismatchSendLovelaceAmount(
			t, ctx, apex, cardanofw.ChainIDPrime, cardanofw.ChainIDCardano, bridgingFee, operationFee, 0)
	})

	t.Run("2. Multiple submitters mismatch submitted and receiver amounts", func(t *testing.T) {
		for i := 0; i < 5; i++ {
			sendAmount := uint64(1_000_000)

			receivers := []sendtx.BridgingTxReceiver{
				{
					Addr:         apex.Users[i].GetAddress(cardanofw.ChainIDCardano),
					Amount:       sendAmount * 10,
					BridgingType: sendtx.BridgingTypeCurrencyOnSource,
				},
			}

			feeAmount, err := apex.GetChainMust(t, cardanofw.ChainIDPrime).GetBridgingFee(
				ctx, cardanofw.ChainIDCardano, receivers, bridgingFee, operationFee)
			require.NoError(t, err)

			metadata, err := apex.GetChainMust(t, cardanofw.ChainIDPrime).CreateMetadata(
				apex.Users[i].GetAddress(cardanofw.ChainIDPrime), cardanofw.ChainIDCardano,
				receivers, feeAmount, operationFee)
			require.NoError(t, err)

			txHash, err := apex.SubmitTx(
				ctx, cardanofw.ChainIDPrime, apex.Users[i],
				apex.PrimeInfo.MultisigAddr, new(big.Int).SetUint64(sendAmount+feeAmount+operationFee), nil, metadata)
			require.NoError(t, err)

			cardanofw.WaitForInvalidState(t, ctx, apex, cardanofw.ChainIDPrime, txHash, apiKey, 0)
		}
	})

	t.Run("3. Multiple submitters mismatch submitted and receiver amounts parallel", func(t *testing.T) {
		instances := 5
		txHashes := make([]string, instances)

		sendAmount := uint64(1_000_000)

		var wg sync.WaitGroup

		for i := 0; i < instances; i++ {
			wg.Add(1)

			go func(idx int) {
				defer wg.Done()

				testUser := apex.Users[idx]
				receivers := []sendtx.BridgingTxReceiver{
					{
						Addr:         apex.Users[idx].GetAddress(cardanofw.ChainIDCardano),
						Amount:       sendAmount * 10,
						BridgingType: sendtx.BridgingTypeCurrencyOnSource,
					},
				}

				feeAmount, err := apex.GetChainMust(t, cardanofw.ChainIDPrime).GetBridgingFee(
					ctx, cardanofw.ChainIDCardano, receivers, bridgingFee, operationFee)
				require.NoError(t, err)

				metadata, err := apex.GetChainMust(t, cardanofw.ChainIDPrime).CreateMetadata(
					testUser.GetAddress(cardanofw.ChainIDPrime), cardanofw.ChainIDCardano,
					receivers, feeAmount, operationFee)
				require.NoError(t, err)

				txHashes[idx], err = apex.SubmitTx(
					ctx, cardanofw.ChainIDPrime, testUser,
					apex.PrimeInfo.MultisigAddr, new(big.Int).SetUint64(sendAmount+feeAmount+operationFee), nil, metadata)
				require.NoError(t, err)
			}(i)
		}

		wg.Wait()

		for i := 0; i < instances; i++ {
			cardanofw.WaitForInvalidState(t, ctx, apex, cardanofw.ChainIDPrime, txHashes[i], apiKey, 0)
		}
	})

	t.Run("4. Submitted invalid metadata - sliced off", func(t *testing.T) {
		sendAmount := uint64(1_000_000)

		receivers := []sendtx.BridgingTxReceiver{
			{
				Addr:         user.GetAddress(cardanofw.ChainIDCardano),
				Amount:       sendAmount,
				BridgingType: sendtx.BridgingTypeCurrencyOnSource,
			},
		}

		feeAmount, err := apex.GetChainMust(t, cardanofw.ChainIDPrime).GetBridgingFee(
			ctx, cardanofw.ChainIDCardano, receivers, bridgingFee, operationFee)
		require.NoError(t, err)

		metadata, err := apex.GetChainMust(t, cardanofw.ChainIDPrime).CreateMetadata(
			user.GetAddress(cardanofw.ChainIDPrime), cardanofw.ChainIDCardano,
			receivers, feeAmount, operationFee)
		require.NoError(t, err)

		// Send only half bytes of metadata making it invalid
		metadata = metadata[0 : len(metadata)/2]

		_, err = apex.SubmitTx(
			ctx, cardanofw.ChainIDPrime, user,
			apex.PrimeInfo.MultisigAddr, new(big.Int).SetUint64(sendAmount+feeAmount+operationFee), nil, metadata)
		require.Error(t, err)
	})

	t.Run("5.Submitted invalid metadata - currency under min - token on source", func(t *testing.T) {
		sendAmount := uint64(1_000_000)

		user, err := cardanofw.NewTestApexUser(cardanofw.NewApexNetworkTypesFromSystem(apex))
		require.NoError(t, err)

		tokensFunded, err := cardanofw.FundUserWithToken(
			ctx, apex, cardanofw.ChainIDPrime,
			apex.PrimeInfo.GenesisWallet, user,
			cardanofw.DefaultTokenName, cardanofw.DefaultTokenMintAmount,
			uint64(10_000_000), uint64(10_000_000))
		require.NoError(t, err)

		receivers := []sendtx.BridgingTxReceiver{
			{
				Addr:         user.GetAddress(cardanofw.ChainIDCardano),
				Amount:       sendAmount,
				BridgingType: sendtx.BridgingTypeNativeTokenOnSource,
			},
		}

		feeAmount, err := apex.GetChainMust(t, cardanofw.ChainIDPrime).GetBridgingFee(
			ctx, cardanofw.ChainIDCardano, receivers, bridgingFee, operationFee)
		require.NoError(t, err)

		feeAmount -= 1_000_000

		metadata, err := apex.GetChainMust(t, cardanofw.ChainIDPrime).CreateMetadata(
			user.GetAddress(cardanofw.ChainIDPrime), cardanofw.ChainIDCardano,
			receivers, feeAmount, operationFee)
		require.NoError(t, err)

		txHash, err := apex.SubmitTx(
			ctx, cardanofw.ChainIDPrime, user,
			apex.PrimeInfo.MultisigAddr, new(big.Int).SetUint64(sendAmount+feeAmount+operationFee),
			[]wallet.TokenAmount{
				{Token: tokensFunded.Token, Amount: sendAmount},
			},
			metadata)
		require.NoError(t, err)

		cardanofw.WaitForInvalidState(t, ctx, apex, cardanofw.ChainIDPrime, txHash, apex.Config.APIKey, 0)
	})

	t.Run("6. Submitted invalid metadata - wrong type", func(t *testing.T) {
		executeInvalidMetadataType(t, ctx, apex, cardanofw.ChainIDPrime, cardanofw.ChainIDCardano, bridgingFee, operationFee, 60)
	})

	t.Run("7. Submitted invalid metadata - invalid destination", func(t *testing.T) {
		executeInvalidDestination(t, ctx, apex, cardanofw.ChainIDPrime, cardanofw.ChainIDCardano, bridgingFee, operationFee, 0)
	})

	t.Run("8. Submitted invalid metadata - invalid sender", func(t *testing.T) {
		executeInvalidMetadataSender(t, ctx, apex, cardanofw.ChainIDPrime, cardanofw.ChainIDCardano, bridgingFee, operationFee, 0)
	})

	t.Run("9. Submitted invalid metadata - invalid bridging fee", func(t *testing.T) {
		executeInvalidBridgingFee(t, ctx, apex, cardanofw.ChainIDPrime, cardanofw.ChainIDCardano, bridgingFee, operationFee, 0)
	})

	t.Run("10. Submitted invalid metadata - invalid fee receiver address - token on source", func(t *testing.T) {
		executeInvalidFeeReceiverAddr(t, ctx, apex, cardanofw.ChainIDPrime, cardanofw.ChainIDCardano, bridgingFee, operationFee, 0)
	})

	t.Run("11. Submitted invalid metadata - empty receivers", func(t *testing.T) {
		executeInvalidEmptyReceivers(
			t, ctx, apex, cardanofw.ChainIDPrime, cardanofw.ChainIDCardano, bridgingFee, operationFee, 0)
	})

	t.Run("12. Submitted with unknown tokens to bridging addr", func(t *testing.T) {
		sendAmount := uint64(1_500_000)
		user := apex.Users[userCnt-1]
		minterWallet, _ := user.GetCardanoWallet(cardanofw.ChainIDPrime)

		tokensFunded, err := cardanofw.FundUserWithToken(
			ctx, apex, cardanofw.ChainIDPrime,
			minterWallet, user,
			cardanofw.DefaultTokenName, cardanofw.DefaultTokenMintAmount,
			uint64(1_500_000), uint64(1_000_000))
		require.NoError(t, err)

		executeInvalidSendUnknownToken(
			t, ctx, apex, user, cardanofw.ChainIDPrime, cardanofw.ChainIDCardano,
			bridgingFee, operationFee, sendAmount, *tokensFunded, 0)
	})

	t.Run("13. Submitted invalid metadata - invalid send amount - token on source", func(t *testing.T) {
		user, err := cardanofw.NewTestApexUser(cardanofw.NewApexNetworkTypesFromSystem(apex))
		require.NoError(t, err)

		tokensFunded, err := cardanofw.FundUserWithToken(
			ctx, apex, cardanofw.ChainIDPrime,
			apex.PrimeInfo.GenesisWallet, user,
			cardanofw.DefaultTokenName, cardanofw.DefaultTokenMintAmount,
			uint64(10_000_000), uint64(1_123_000))
		require.NoError(t, err)

		executeInvalidMismatchSendNativeTokenAmount(
			t, ctx, apex, user, cardanofw.ChainIDPrime, cardanofw.ChainIDCardano,
			bridgingFee, operationFee, *tokensFunded, 0)
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
	primeConfig.FundTokenAmount = 1_000_000_000
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
		}, nil),
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
			{src: cardanofw.ChainIDCardano, dest: cardanofw.ChainIDPrime, sender: apex.Users[0]},
		}
		txHashes = make([]string, len(bridgingRequests))
	)

	var wg sync.WaitGroup

	for idx, br := range bridgingRequests {
		wg.Add(1)

		go func(i int, src string, dest string, sender *cardanofw.TestApexUser) {
			defer wg.Done()

			txHashes[i] = apex.SubmitBridgingRequest(t, ctx, src, dest, sender, apexSendAmount, sendtx.BridgingTypeCurrencyOnSource,
				user)
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
	primeConfig.FundTokenAmount = 1_000_000_000
	cardanoConfig.FundTokenAmount = 1_000_000_000

	apex := cardanofw.SetupAndRunSkylineBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithUserCnt(1),
		cardanofw.WithCardanoConfig(cardanoConfig),
		cardanofw.WithPrimeConfig(primeConfig),
		cardanofw.WithCustomConfigHandlers(func(_ *cardanofw.ApexSystem, mp map[string]interface{}) {
			setting := cardanofw.GetMapFromInterfaceKey(mp, "bridgingSettings")
			setting["maxTokenAmountAllowedToBridge"] = new(big.Int).SetUint64(5_000_000)
		}, nil),
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
			{src: cardanofw.ChainIDCardano, dest: cardanofw.ChainIDPrime, sender: apex.Users[0]},
		}
		txHashes = make([]string, len(bridgingRequests))
	)

	_, err := cardanofw.FundUserWithToken(
		ctx, apex, cardanofw.ChainIDPrime,
		apex.PrimeInfo.GenesisWallet, apex.Users[0],
		cardanofw.DefaultTokenName, cardanofw.DefaultTokenMintAmount,
		uint64(5_000_000), uint64(1_000_000_000))
	require.NoError(t, err)

	_, err = cardanofw.FundUserWithToken(
		ctx, apex, cardanofw.ChainIDCardano,
		apex.CardanoInfo.GenesisWallet, apex.Users[0],
		cardanofw.DefaultTokenName, cardanofw.DefaultTokenMintAmount,
		uint64(5_000_000), uint64(1_000_000_000))
	require.NoError(t, err)

	var wg sync.WaitGroup

	for idx, br := range bridgingRequests {
		wg.Add(1)

		go func(i int, src string, dest string, sender *cardanofw.TestApexUser) {
			defer wg.Done()

			txHashes[i] = apex.SubmitBridgingRequest(t, ctx, src, dest, sender, apexSendAmount, sendtx.BridgingTypeNativeTokenOnSource,
				user)
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

func TestE2E_SkylineBridge_UTxOConsolidation(t *testing.T) {
	if cardanofw.ShouldSkipE2RRedundantTests() {
		t.Skip()
	}

	const (
		fundUtxoCount                 = 9
		maxFeeUtxoCount               = 1
		maxUtxoCount                  = 3
		minimumExpectedConsolidations = 1

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

	primeConfig := cardanofw.NewPrimeChainConfig()
	primeConfig.FundUTxOCount = fundUtxoCount
	primeConfig.FundAmount = fundFactor * minValue * fundUtxoCount
	primeConfig.FundTokenAmount = fundFactor * minValue * fundUtxoCount
	primeConfig.InitialHotWalletAmount = new(big.Int).SetUint64(cardanoConfig.FundAmount)
	primeConfig.InitialHotWalletTokenAmount = new(big.Int).SetUint64(cardanoConfig.FundTokenAmount)

	sendAmountTokens := minValue*sendMinValueFactor*fundFactor + sendMinValueIncrement   // when we send tokens, this amount of currency will be released from multisig address
	sendAmountCurrency := minValue*sendMinValueFactor*fundFactor + sendMinValueIncrement // when we send currency, this amount of native tokens will be released from multisig address

	var (
		initialUtxosCardano, initialUtxosPrime []map[string]any
		tipDataCardano, tipDataPrime           wallet.QueryTipData
		lock                                   sync.Mutex
	)

	//nolint:dupl
	apex := cardanofw.SetupAndRunSkylineBridge(
		t, ctx,
		cardanofw.WithUserCnt(parallelInstances+1),
		cardanofw.WithCardanoConfig(cardanoConfig),
		cardanofw.WithPrimeConfig(primeConfig),
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

	for _, sender := range apex.Users[:parallelInstances] {
		_, err = cardanofw.FundUserWithToken(
			ctx, apex, cardanofw.ChainIDPrime,
			apex.PrimeInfo.GenesisWallet, sender,
			cardanofw.DefaultTokenName, cardanofw.DefaultTokenMintAmount,
			uint64(2_000_000_000), uint64(2_000_000_000))
		require.NoError(t, err)

		_, err = cardanofw.FundUserWithToken(
			ctx, apex, cardanofw.ChainIDCardano,
			apex.CardanoInfo.GenesisWallet, sender,
			cardanofw.DefaultTokenName, cardanofw.DefaultTokenMintAmount,
			uint64(2_000_000_000), uint64(2_000_000_000))
		require.NoError(t, err)
	}

	utxos, err := txProviderCardano.GetUtxos(ctx, apex.CardanoInfo.MultisigAddr)
	require.NoError(t, err)

	require.Len(t, utxos, cardanoConfig.FundUTxOCount)

	t.Run("with tokens", func(t *testing.T) {
		ctxChild, cncl := context.WithCancel(ctx)
		defer cncl()

		getCntConsolidationMap := checkConsolidationBatchCounts(
			t, ctxChild,
			apex.BridgeCluster.Servers[0].JSONRPC(),
			[]string{cardanofw.ChainIDCardano})

		e2ehelper.ExecuteSingleBridging(
			t, ctxChild, apex, apex.Users[0], apex.Users[0],
			cardanofw.ChainIDPrime, cardanofw.ChainIDCardano,
			new(big.Int).SetUint64(sendAmountTokens),
			sendtx.BridgingTypeNativeTokenOnSource)

		for _, cnt := range getCntConsolidationMap() {
			assert.GreaterOrEqual(t, cnt, minimumExpectedConsolidations)
		}
	})

	t.Run("with currency", func(t *testing.T) {
		ctxChild, cncl := context.WithCancel(ctx)
		defer cncl()

		getCntConsolidationMap := checkConsolidationBatchCounts(
			t, ctxChild,
			apex.BridgeCluster.Servers[0].JSONRPC(),
			[]string{cardanofw.ChainIDPrime})

		e2ehelper.ExecuteSingleBridging(
			t, ctxChild, apex, apex.Users[0], apex.Users[0],
			cardanofw.ChainIDCardano, cardanofw.ChainIDPrime,
			new(big.Int).SetUint64(sendAmountCurrency),
			sendtx.BridgingTypeCurrencyOnSource)

		for _, cnt := range getCntConsolidationMap() {
			assert.GreaterOrEqual(t, cnt, minimumExpectedConsolidations)
		}
	})

	t.Run("both directions", func(t *testing.T) {
		ctxChild, cncl := context.WithCancel(ctx)
		defer cncl()

		// when we send currency, this amount of native tokens will be released from multisig address
		sendAmountCurrency := minValue*sendMinValueFactor + sendMinValueIncrement

		getCntConsolidationMap := checkConsolidationBatchCounts(
			t, ctxChild,
			apex.BridgeCluster.Servers[0].JSONRPC(),
			[]string{cardanofw.ChainIDCardano, cardanofw.ChainIDPrime})

		e2ehelper.ExecuteBridging(
			t, ctxChild, apex, sequentialInstances,
			apex.Users[:parallelInstances],
			[]*cardanofw.TestApexUser{apex.Users[parallelInstances]},
			[]string{cardanofw.ChainIDCardano, cardanofw.ChainIDPrime},
			map[string][]string{
				cardanofw.ChainIDCardano: {cardanofw.ChainIDPrime},
				cardanofw.ChainIDPrime:   {cardanofw.ChainIDCardano},
			}, sendtx.BridgingTypeCurrencyOnSource,
			new(big.Int).SetUint64(sendAmountCurrency),
			e2ehelper.WithWaitForUnexpectedBridges(true),
		)

		for _, cnt := range getCntConsolidationMap() {
			assert.GreaterOrEqual(t, cnt, minimumExpectedConsolidations)
		}
	})
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

	primeConfig := cardanofw.NewPrimeChainConfig()
	primeConfig.FundUTxOCount = fundUtxoCount
	primeConfig.FundAmount = fundFactor * minValue * fundUtxoCount
	primeConfig.FundTokenAmount = fundFactor * minValue * fundUtxoCount
	primeConfig.InitialHotWalletAmount = new(big.Int).SetUint64(cardanoConfig.FundAmount)
	primeConfig.InitialHotWalletTokenAmount = new(big.Int).SetUint64(cardanoConfig.FundTokenAmount)

	sendAmountTokens := minValue*sendMinValueFactor + sendMinValueIncrement   // when we send tokens, this amount of currency will be released from multisig address
	sendAmountCurrency := minValue*sendMinValueFactor + sendMinValueIncrement // when we send currency, this amount of native tokens will be released from multisig address

	var (
		initialUtxosCardano, initialUtxosPrime []map[string]any
		tipDataCardano, tipDataPrime           wallet.QueryTipData
		lock                                   sync.Mutex
	)

	//nolint:dupl
	apex := cardanofw.SetupAndRunSkylineBridge(
		t, ctx,
		cardanofw.WithUserCnt(parallelInstances+1),
		cardanofw.WithCardanoConfig(cardanoConfig),
		cardanofw.WithPrimeConfig(primeConfig),
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

	for _, sender := range apex.Users[:parallelInstances] {
		_, err = cardanofw.FundUserWithToken(
			ctx, apex, cardanofw.ChainIDPrime,
			apex.PrimeInfo.GenesisWallet, sender,
			cardanofw.DefaultTokenName, cardanofw.DefaultTokenMintAmount,
			uint64(2_000_000_000), uint64(2_000_000_000))
		require.NoError(t, err)

		_, err = cardanofw.FundUserWithToken(
			ctx, apex, cardanofw.ChainIDCardano,
			apex.CardanoInfo.GenesisWallet, sender,
			cardanofw.DefaultTokenName, cardanofw.DefaultTokenMintAmount,
			uint64(2_000_000_000), uint64(2_000_000_000))
		require.NoError(t, err)
	}

	utxos, err := txProviderCardano.GetUtxos(ctx, apex.CardanoInfo.MultisigAddr)
	require.NoError(t, err)

	require.Len(t, utxos, cardanoConfig.FundUTxOCount)

	var (
		utxosCardanoTokenSum1 uint64
		utxosCardanoTokenSum2 uint64
	)

	t.Run("with currency from prime to cardano", func(t *testing.T) {
		ctxChild, cncl := context.WithCancel(ctx)
		defer cncl()

		utxosCardano, err := txProviderCardano.GetUtxos(ctx, apex.CardanoInfo.MultisigAddr)
		require.NoError(t, err)

		utxosCardanoSum := wallet.GetUtxosSum(utxosCardano)

		// sum of tokens on cardano multisig address in the beginning
		tokenName := apex.GetTokenNameForChains(cardanofw.ChainIDCardano, cardanofw.ChainIDPrime)
		utxosCardanoTokenSum1 = utxosCardanoSum[tokenName]

		getCntConsolidationMap := checkConsolidationBatchCounts(
			t, ctxChild,
			apex.BridgeCluster.Servers[0].JSONRPC(),
			[]string{cardanofw.ChainIDCardano})

		e2ehelper.ExecuteBridging(
			t, ctxChild, apex, sequentialInstances,
			apex.Users[:parallelInstances],
			[]*cardanofw.TestApexUser{apex.Users[parallelInstances]},
			[]string{cardanofw.ChainIDPrime},
			map[string][]string{
				cardanofw.ChainIDPrime: {cardanofw.ChainIDCardano},
			}, sendtx.BridgingTypeCurrencyOnSource,
			new(big.Int).SetUint64(sendAmountCurrency),
			e2ehelper.WithWaitForUnexpectedBridges(true),
		)

		for _, cnt := range getCntConsolidationMap() {
			assert.GreaterOrEqual(t, cnt, minimumExpectedConsolidations)
		}
	})

	t.Run("with tokens from cardano to prime", func(t *testing.T) {
		ctxChild, cncl := context.WithCancel(ctx)
		defer cncl()

		getCntConsolidationMap := checkConsolidationBatchCounts(
			t, ctxChild,
			apex.BridgeCluster.Servers[0].JSONRPC(),
			[]string{cardanofw.ChainIDPrime})

		e2ehelper.ExecuteBridging(
			t, ctxChild, apex, sequentialInstances,
			apex.Users[:parallelInstances],
			[]*cardanofw.TestApexUser{apex.Users[parallelInstances]},
			[]string{cardanofw.ChainIDCardano},
			map[string][]string{
				cardanofw.ChainIDCardano: {cardanofw.ChainIDPrime},
			}, sendtx.BridgingTypeNativeTokenOnSource,
			new(big.Int).SetUint64(sendAmountTokens),
			e2ehelper.WithWaitForUnexpectedBridges(true),
		)

		utxosCardano, err := txProviderCardano.GetUtxos(ctx, apex.CardanoInfo.MultisigAddr)
		require.NoError(t, err)

		utxosCardanoSum := wallet.GetUtxosSum(utxosCardano)
		tokenName := apex.GetTokenNameForChains(cardanofw.ChainIDCardano, cardanofw.ChainIDPrime)

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
		apiKey       = "test_api_key"
		userCnt      = 10
		feeAmountDfm = 1_100_000
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
	}

	createBridgingData := func(ctx context.Context, apex *cardanofw.ApexSystem,
		bridgingRequests []*bridingRequest, receivers map[uint]*cardanofw.TestApexUser,
		defundReceiver *cardanofw.TestApexUser, defundAmount *big.Int, isNativeToken bool) (
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
			tokenName := wallet.AdaTokenName

			if isNativeToken {
				tokenName = apex.GetTokenNameForChains(br.dest, br.src)
			}

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
		bridgingRequests []*bridingRequest, receivers map[uint]*cardanofw.TestApexUser, bridgingType sendtx.BridgingType,
	) {
		var wg sync.WaitGroup

		for _, br := range bridgingRequests {
			wg.Add(1)

			go func(src string, dest string, sender *cardanofw.TestApexUser, receiver *cardanofw.TestApexUser, amount *big.Int) {
				defer wg.Done()

				txHash := apex.SubmitBridgingRequest(t, ctx, src, dest, sender, amount, bridgingType, receiver)
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
				apex.Config.PrimeConfig.FundTokenAmount = cardanofw.ApexToDfm(fundAmountApex).Uint64()
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
			cardanofw.ChainIDPrime, defundReceiver.GetAddress(cardanofw.ChainIDPrime), defundAmount, defundAmount))

		require.NoError(t, apex.DefundHotWallet(
			cardanofw.ChainIDCardano, defundReceiver.GetAddress(cardanofw.ChainIDCardano), defundAmount, defundAmount))

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
		primeConfig.FundTokenAmount = cardanofw.DfmToChainNativeTokenAmount(cardanofw.ChainIDPrime, initialFundInDfm).Uint64()
		cardanoConfig.FundAmount = cardanofw.DfmToChainNativeTokenAmount(cardanofw.ChainIDCardano, initialFundInDfm).Uint64()
		cardanoConfig.FundTokenAmount = cardanofw.DfmToChainNativeTokenAmount(cardanofw.ChainIDCardano, initialFundInDfm).Uint64()

		apex := cardanofw.SetupAndRunSkylineBridge(
			t, ctx,
			cardanofw.WithAPIKey(apiKey),
			cardanofw.WithUserCnt(userCnt),
			cardanofw.WithPrimeConfig(primeConfig),
			cardanofw.WithCardanoConfig(cardanoConfig),
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

			bridgignType = sendtx.BridgingTypeNativeTokenOnSource

			bridgingRequests = []*bridingRequest{
				{src: cardanofw.ChainIDPrime, dest: cardanofw.ChainIDCardano, sender: apex.Users[0], amount: apexSendAmount, receiverIdx: 0},
				{src: cardanofw.ChainIDCardano, dest: cardanofw.ChainIDPrime, sender: apex.Users[0], amount: apexSendAmount, receiverIdx: 0},
			}

			receivers = map[uint]*cardanofw.TestApexUser{
				0: apex.Users[userCnt-1],
			}
		)

		require.True(t, cardanofw.ApexToDfm(apexSendAmount).Uint64()+feeAmountDfm < initialFundInDfm.Uint64())

		isNativeToken := bridgignType == sendtx.BridgingTypeCurrencyOnSource

		minterWalletPrime := apex.PrimeInfo.GenesisWallet
		minterWalletCardano := apex.CardanoInfo.GenesisWallet

		_, err = cardanofw.FundUserWithToken(
			ctx, apex, cardanofw.ChainIDPrime,
			minterWalletPrime, apex.Users[0],
			cardanofw.DefaultTokenName, cardanofw.DefaultTokenMintAmount,
			uint64(2_000_000), uint64(50_000_000))
		require.NoError(t, err)

		_, err = cardanofw.FundUserWithToken(
			ctx, apex, cardanofw.ChainIDCardano,
			minterWalletCardano, apex.Users[0],
			cardanofw.DefaultTokenName, cardanofw.DefaultTokenMintAmount,
			uint64(2_000_000), uint64(50_000_000))
		require.NoError(t, err)

		chainPrevAmounts, chainExpectedAmounts, chainReceivers,
			defundReceiversPrevAmount, defundReceiversExpectedAmount, defundReceivers :=
			createBridgingData(ctx, apex, bridgingRequests, receivers, defundReceiver, apexDefundAndFundAmount, isNativeToken)

		defundWallets(ctx, apex, defundReceiver, apexDefundAndFundAmount,
			defundReceiversPrevAmount, defundReceiversExpectedAmount, defundReceivers, isNativeToken)

		bridgeTransactions(ctx, apex, bridgingRequests, receivers, bridgignType)

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
		primeConfig.FundTokenAmount = cardanofw.DfmToChainNativeTokenAmount(cardanofw.ChainIDPrime, initialFundInDfm).Uint64()
		cardanoConfig.FundAmount = cardanofw.DfmToChainNativeTokenAmount(cardanofw.ChainIDCardano, initialFundInDfm).Uint64()
		cardanoConfig.FundTokenAmount = cardanofw.DfmToChainNativeTokenAmount(cardanofw.ChainIDCardano, initialFundInDfm).Uint64()

		apex := cardanofw.SetupAndRunSkylineBridge(
			t, ctx,
			cardanofw.WithAPIKey(apiKey),
			cardanofw.WithUserCnt(userCnt),
			cardanofw.WithPrimeConfig(primeConfig),
			cardanofw.WithCardanoConfig(cardanoConfig),
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

			bridgignType = sendtx.BridgingTypeNativeTokenOnSource

			bridgingRequests = []*bridingRequest{
				{src: cardanofw.ChainIDPrime, dest: cardanofw.ChainIDCardano, sender: apex.Users[0], amount: apexSendAmount, receiverIdx: 0},
				{src: cardanofw.ChainIDCardano, dest: cardanofw.ChainIDPrime, sender: apex.Users[1], amount: apexSendAmount, receiverIdx: 0},
			}

			receivers = map[uint]*cardanofw.TestApexUser{
				0: apex.Users[userCnt-1],
			}
		)

		isNativeToken := bridgignType == sendtx.BridgingTypeCurrencyOnSource

		minterWalletPrime := apex.PrimeInfo.GenesisWallet
		minterWalletCardano := apex.CardanoInfo.GenesisWallet

		_, err = cardanofw.FundUserWithToken(
			ctx, apex, cardanofw.ChainIDPrime,
			minterWalletPrime, apex.Users[0],
			cardanofw.DefaultTokenName, cardanofw.DefaultTokenMintAmount,
			uint64(2_000_000), uint64(250_000_000))
		require.NoError(t, err)

		_, err = cardanofw.FundUserWithToken(
			ctx, apex, cardanofw.ChainIDCardano,
			minterWalletCardano, apex.Users[1],
			cardanofw.DefaultTokenName, cardanofw.DefaultTokenMintAmount,
			uint64(2_000_000), uint64(250_000_000))
		require.NoError(t, err)

		require.True(t,
			cardanofw.ApexToDfm(apexSendAmount).Uint64()+feeAmountDfm < initialFundInDfm.Uint64())

		chainPrevAmounts, chainExpectedAmounts, chainReceivers, _, _, _ :=
			createBridgingData(ctx, apex, bridgingRequests, receivers, defundReceiver, apexDefundAndFundAmount, isNativeToken)

		for _, request := range bridgingRequests {
			bridgeTransactions(ctx, apex, []*bridingRequest{request}, receivers, bridgignType)

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

	t.Run("3. Fund_Parallel_Send_BRs_Then_Full_Fund", func(t *testing.T) {
		ctx, cncl := context.WithCancel(context.Background())
		defer cncl()

		primeConfig, cardanoConfig := cardanofw.NewPrimeChainConfig(), cardanofw.NewCardanoChainConfig(true)
		primeConfig.FundAmount = 0
		cardanoConfig.FundAmount = 0
		primeConfig.FundTokenAmount = 0
		cardanoConfig.FundTokenAmount = 0

		apex := cardanofw.SetupAndRunSkylineBridge(
			t, ctx,
			cardanofw.WithAPIKey(apiKey),
			cardanofw.WithUserCnt(userCnt),
			cardanofw.WithPrimeConfig(primeConfig),
			cardanofw.WithCardanoConfig(cardanoConfig),
		)

		defer require.True(t, apex.ApexBridgeProcessesRunning())

		var (
			bridgingRequests = []*bridingRequest{
				{src: cardanofw.ChainIDPrime, dest: cardanofw.ChainIDCardano, sender: apex.Users[0], amount: big.NewInt(1), receiverIdx: 0},
				{src: cardanofw.ChainIDCardano, dest: cardanofw.ChainIDPrime, sender: apex.Users[0], amount: big.NewInt(1), receiverIdx: 0},
			}

			receivers = map[uint]*cardanofw.TestApexUser{
				0: apex.Users[userCnt-1],
			}

			bridgignType = sendtx.BridgingTypeNativeTokenOnSource
		)

		isNativeToken := bridgignType == sendtx.BridgingTypeCurrencyOnSource

		minterWalletPrime := apex.PrimeInfo.GenesisWallet
		minterWalletCardano := apex.CardanoInfo.GenesisWallet

		_, err = cardanofw.FundUserWithToken(
			ctx, apex, cardanofw.ChainIDPrime,
			minterWalletPrime, apex.Users[0],
			cardanofw.DefaultTokenName, cardanofw.DefaultTokenMintAmount,
			uint64(2_000_000), uint64(50_000_000))
		require.NoError(t, err)

		_, err = cardanofw.FundUserWithToken(
			ctx, apex, cardanofw.ChainIDCardano,
			minterWalletCardano, apex.Users[0],
			cardanofw.DefaultTokenName, cardanofw.DefaultTokenMintAmount,
			uint64(2_000_000), uint64(150_000_000))
		require.NoError(t, err)

		chainPrevAmounts, chainExpectedAmounts, chainReceivers, _, _, _ := createBridgingData(ctx, apex, bridgingRequests, receivers, nil, nil, isNativeToken)

		bridgeTransactions(ctx, apex, bridgingRequests, receivers, bridgignType)

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

		apex := cardanofw.SetupAndRunSkylineBridge(
			t, ctx,
			cardanofw.WithAPIKey(apiKey),
			cardanofw.WithUserCnt(userCnt),
			cardanofw.WithPrimeConfig(primeConfig),
			cardanofw.WithCardanoConfig(cardanoConfig),
		)

		defer require.True(t, apex.ApexBridgeProcessesRunning())

		//nolint:dupl
		var (
			bridgingRequests = []*bridingRequest{
				{src: cardanofw.ChainIDPrime, dest: cardanofw.ChainIDCardano, sender: apex.Users[0], amount: big.NewInt(1), receiverIdx: 0},
				{src: cardanofw.ChainIDPrime, dest: cardanofw.ChainIDCardano, sender: apex.Users[1], amount: big.NewInt(100), receiverIdx: 1},
				{src: cardanofw.ChainIDCardano, dest: cardanofw.ChainIDPrime, sender: apex.Users[0], amount: big.NewInt(1), receiverIdx: 0},
				{src: cardanofw.ChainIDCardano, dest: cardanofw.ChainIDPrime, sender: apex.Users[1], amount: big.NewInt(100), receiverIdx: 1},
			}

			receivers = map[uint]*cardanofw.TestApexUser{
				0: apex.Users[userCnt-1],
				1: apex.Users[userCnt-2],
			}

			bridgignType = sendtx.BridgingTypeNativeTokenOnSource
		)

		isNativeToken := bridgignType == sendtx.BridgingTypeCurrencyOnSource

		minterWalletPrime := apex.PrimeInfo.GenesisWallet
		minterWalletCardano := apex.CardanoInfo.GenesisWallet

		_, err = cardanofw.FundUserWithToken(
			ctx, apex, cardanofw.ChainIDPrime,
			minterWalletPrime, apex.Users[0],
			cardanofw.DefaultTokenName, cardanofw.DefaultTokenMintAmount,
			uint64(2_000_000), uint64(250_000_000))
		require.NoError(t, err)

		_, err = cardanofw.FundUserWithToken(
			ctx, apex, cardanofw.ChainIDCardano,
			minterWalletCardano, apex.Users[0],
			cardanofw.DefaultTokenName, cardanofw.DefaultTokenMintAmount,
			uint64(2_000_000), uint64(250_000_000))
		require.NoError(t, err)

		_, err = cardanofw.FundUserWithToken(
			ctx, apex, cardanofw.ChainIDPrime,
			minterWalletPrime, apex.Users[1],
			cardanofw.DefaultTokenName, cardanofw.DefaultTokenMintAmount,
			uint64(2_000_000), uint64(250_000_000))
		require.NoError(t, err)

		_, err = cardanofw.FundUserWithToken(
			ctx, apex, cardanofw.ChainIDCardano,
			minterWalletCardano, apex.Users[1],
			cardanofw.DefaultTokenName, cardanofw.DefaultTokenMintAmount,
			uint64(2_000_000), uint64(250_000_000))
		require.NoError(t, err)

		chainPrevAmounts, chainExpectedAmounts, chainReceivers, _, _, _ := createBridgingData(ctx, apex, bridgingRequests, receivers, nil, nil, isNativeToken)

		bridgeTransactions(ctx, apex, bridgingRequests, receivers, bridgignType)

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
		primeConfig.FundTokenAmount = cardanofw.DfmToChainNativeTokenAmount(cardanofw.ChainIDPrime, initialFundInDfm).Uint64()
		cardanoConfig.FundAmount = cardanofw.DfmToChainNativeTokenAmount(cardanofw.ChainIDCardano, initialFundInDfm).Uint64()
		cardanoConfig.FundTokenAmount = cardanofw.DfmToChainNativeTokenAmount(cardanofw.ChainIDCardano, initialFundInDfm).Uint64()

		apex := cardanofw.SetupAndRunSkylineBridge(
			t, ctx,
			cardanofw.WithAPIKey(apiKey),
			cardanofw.WithUserCnt(userCnt),
			cardanofw.WithPrimeConfig(primeConfig),
			cardanofw.WithCardanoConfig(cardanoConfig),
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
				{src: cardanofw.ChainIDPrime, dest: cardanofw.ChainIDCardano, sender: apex.Users[0], amount: apexSendAmount, receiverIdx: 0},
				{src: cardanofw.ChainIDCardano, dest: cardanofw.ChainIDPrime, sender: apex.Users[0], amount: apexSendAmount, receiverIdx: 0},
			}

			receivers = map[uint]*cardanofw.TestApexUser{
				0: apex.Users[userCnt-1],
			}
		)

		require.True(t, cardanofw.ApexToDfm(apexSendAmount).Uint64()+feeAmountDfm < initialFundInDfm.Uint64())

		isNativeToken := bridgignType == sendtx.BridgingTypeCurrencyOnSource

		chainPrevAmounts, chainExpectedAmounts, chainReceivers,
			defundReceiversPrevAmount, defundReceiversExpectedAmount, defundReceivers :=
			createBridgingData(ctx, apex, bridgingRequests, receivers, defundReceiver, apexDefundAndFundAmount, isNativeToken)

		defundWallets(ctx, apex, defundReceiver, apexDefundAndFundAmount,
			defundReceiversPrevAmount, defundReceiversExpectedAmount, defundReceivers, isNativeToken)

		bridgeTransactions(ctx, apex, bridgingRequests, receivers, bridgignType)

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
		primeConfig.FundTokenAmount = cardanofw.DfmToChainNativeTokenAmount(cardanofw.ChainIDPrime, initialFundInDfm).Uint64()
		cardanoConfig.FundAmount = cardanofw.DfmToChainNativeTokenAmount(cardanofw.ChainIDCardano, initialFundInDfm).Uint64()
		cardanoConfig.FundTokenAmount = cardanofw.DfmToChainNativeTokenAmount(cardanofw.ChainIDCardano, initialFundInDfm).Uint64()

		apex := cardanofw.SetupAndRunSkylineBridge(
			t, ctx,
			cardanofw.WithAPIKey(apiKey),
			cardanofw.WithUserCnt(userCnt),
			cardanofw.WithPrimeConfig(primeConfig),
			cardanofw.WithCardanoConfig(cardanoConfig),
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
				{src: cardanofw.ChainIDPrime, dest: cardanofw.ChainIDCardano, sender: apex.Users[0], amount: apexSendAmount, receiverIdx: 0},
				{src: cardanofw.ChainIDCardano, dest: cardanofw.ChainIDPrime, sender: apex.Users[1], amount: apexSendAmount, receiverIdx: 0},
			}

			receivers = map[uint]*cardanofw.TestApexUser{
				0: apex.Users[userCnt-1],
			}
		)

		isNativeToken := bridgignType == sendtx.BridgingTypeCurrencyOnSource

		require.True(t,
			cardanofw.ApexToDfm(apexSendAmount).Uint64()+feeAmountDfm < initialFundInDfm.Uint64())

		chainPrevAmounts, chainExpectedAmounts, chainReceivers, _, _, _ :=
			createBridgingData(ctx, apex, bridgingRequests, receivers, defundReceiver, apexDefundAndFundAmount, isNativeToken)

		for _, request := range bridgingRequests {
			bridgeTransactions(ctx, apex, []*bridingRequest{request}, receivers, bridgignType)

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

		apex := cardanofw.SetupAndRunSkylineBridge(
			t, ctx,
			cardanofw.WithAPIKey(apiKey),
			cardanofw.WithUserCnt(userCnt),
			cardanofw.WithPrimeConfig(primeConfig),
			cardanofw.WithCardanoConfig(cardanoConfig),
		)

		defer require.True(t, apex.ApexBridgeProcessesRunning())

		var (
			bridgingRequests = []*bridingRequest{
				{src: cardanofw.ChainIDPrime, dest: cardanofw.ChainIDCardano, sender: apex.Users[0], amount: big.NewInt(1), receiverIdx: 0},
				{src: cardanofw.ChainIDCardano, dest: cardanofw.ChainIDPrime, sender: apex.Users[0], amount: big.NewInt(1), receiverIdx: 0},
			}

			receivers = map[uint]*cardanofw.TestApexUser{
				0: apex.Users[userCnt-1],
			}

			bridgignType = sendtx.BridgingTypeCurrencyOnSource
		)

		isNativeToken := bridgignType == sendtx.BridgingTypeCurrencyOnSource

		chainPrevAmounts, chainExpectedAmounts, chainReceivers, _, _, _ := createBridgingData(ctx, apex, bridgingRequests, receivers, nil, nil, isNativeToken)

		bridgeTransactions(ctx, apex, bridgingRequests, receivers, bridgignType)

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

		apex := cardanofw.SetupAndRunSkylineBridge(
			t, ctx,
			cardanofw.WithAPIKey(apiKey),
			cardanofw.WithUserCnt(userCnt),
			cardanofw.WithPrimeConfig(primeConfig),
			cardanofw.WithCardanoConfig(cardanoConfig),
		)

		defer require.True(t, apex.ApexBridgeProcessesRunning())

		//nolint:dupl
		var (
			bridgingRequests = []*bridingRequest{
				{src: cardanofw.ChainIDPrime, dest: cardanofw.ChainIDCardano, sender: apex.Users[0], amount: big.NewInt(1), receiverIdx: 0},
				{src: cardanofw.ChainIDPrime, dest: cardanofw.ChainIDCardano, sender: apex.Users[1], amount: big.NewInt(100), receiverIdx: 1},
				{src: cardanofw.ChainIDCardano, dest: cardanofw.ChainIDPrime, sender: apex.Users[0], amount: big.NewInt(1), receiverIdx: 0},
				{src: cardanofw.ChainIDCardano, dest: cardanofw.ChainIDPrime, sender: apex.Users[1], amount: big.NewInt(100), receiverIdx: 1},
			}

			receivers = map[uint]*cardanofw.TestApexUser{
				0: apex.Users[userCnt-1],
				1: apex.Users[userCnt-2],
			}

			bridgignType = sendtx.BridgingTypeCurrencyOnSource
		)

		isNativeToken := bridgignType == sendtx.BridgingTypeCurrencyOnSource

		chainPrevAmounts, chainExpectedAmounts, chainReceivers, _, _, _ := createBridgingData(ctx, apex, bridgingRequests, receivers, nil, nil, isNativeToken)

		bridgeTransactions(ctx, apex, bridgingRequests, receivers, bridgignType)

		fmt.Printf("Confirming that bridging requests will not be processed\n")

		errsPerChain := waitOnDestination(ctx, apex, chainPrevAmounts, chainExpectedAmounts, chainReceivers, 30, time.Second*10, isNativeToken)
		for chainKey, err := range errsPerChain {
			require.Error(t, err)
			fmt.Printf("As intended, %v TXs on %v not yet arrived\n", chainExpectedAmounts[chainKey], chainKey.chain)
		}

		apex.Config.PrimeConfig.FundAmount = cardanofw.DfmToChainNativeTokenAmount(cardanofw.ChainIDPrime, cardanofw.ApexToDfm(big.NewInt(10))).Uint64()
		apex.Config.CardanoConfig.FundAmount = cardanofw.DfmToChainNativeTokenAmount(cardanofw.ChainIDPrime, cardanofw.ApexToDfm(big.NewInt(10))).Uint64()
		apex.Config.PrimeConfig.FundTokenAmount = cardanofw.DfmToChainNativeTokenAmount(cardanofw.ChainIDPrime, cardanofw.ApexToDfm(big.NewInt(10))).Uint64()
		apex.Config.CardanoConfig.FundTokenAmount = cardanofw.DfmToChainNativeTokenAmount(cardanofw.ChainIDPrime, cardanofw.ApexToDfm(big.NewInt(10))).Uint64()

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
		apex.Config.PrimeConfig.FundTokenAmount = cardanofw.DfmToChainNativeTokenAmount(cardanofw.ChainIDPrime, cardanofw.ApexToDfm(big.NewInt(1000))).Uint64()
		apex.Config.CardanoConfig.FundTokenAmount = cardanofw.DfmToChainNativeTokenAmount(cardanofw.ChainIDPrime, cardanofw.ApexToDfm(big.NewInt(1000))).Uint64()

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
	primeConfig.FundTokenAmount = 1_000_000_000
	cardanoConfig.FundTokenAmount = 1_000_000_000

	apex := cardanofw.SetupAndRunSkylineBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithUserCnt(userCnt),
		cardanofw.WithPrimeConfig(primeConfig),
		cardanofw.WithCardanoConfig(cardanoConfig),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	currencyReceiver := apex.Users[userCnt-1]
	nativeTokenReceiver := apex.Users[userCnt-2]

	fmt.Println("prime user addr: ", currencyReceiver.PrimeAddress)
	fmt.Println("cardano user addr: ", currencyReceiver.CardanoAddress)
	fmt.Println("prime multisig addr: ", apex.PrimeInfo.MultisigAddr)
	fmt.Println("prime fee addr: ", apex.PrimeInfo.FeeAddr)
	fmt.Println("cardano multisig addr: ", apex.CardanoInfo.MultisigAddr)
	fmt.Println("cardano fee addr: ", apex.CardanoInfo.FeeAddr)

	minterWalletPrime := apex.PrimeInfo.GenesisWallet
	minterWalletCardano := apex.CardanoInfo.GenesisWallet

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
		}

		bridgingRequests := []bridgingRequest{
			{src: cardanofw.ChainIDPrime, dest: cardanofw.ChainIDCardano, firstSenderIdx: 0, bridgingType: sendtx.BridgingTypeNativeTokenOnSource, receiver: currencyReceiver},
			{src: cardanofw.ChainIDCardano, dest: cardanofw.ChainIDPrime, firstSenderIdx: 0, bridgingType: sendtx.BridgingTypeNativeTokenOnSource, receiver: currencyReceiver},
			{src: cardanofw.ChainIDPrime, dest: cardanofw.ChainIDCardano, firstSenderIdx: instances, bridgingType: sendtx.BridgingTypeCurrencyOnSource, receiver: nativeTokenReceiver},
			{src: cardanofw.ChainIDCardano, dest: cardanofw.ChainIDPrime, firstSenderIdx: instances, bridgingType: sendtx.BridgingTypeCurrencyOnSource, receiver: nativeTokenReceiver},
		}

		seed := rand.Int63n(1_000_000_000)
		r := rand.New(rand.NewSource(seed)) // New seeded random number generator

		fmt.Printf("Test seed: %v\n", seed)

		fmt.Printf("Funding users with native tokens\n")

		for i := range instances {
			_, err := cardanofw.FundUserWithToken(
				ctx, apex, cardanofw.ChainIDPrime,
				minterWalletPrime, apex.Users[i],
				cardanofw.DefaultTokenName, cardanofw.DefaultTokenMintAmount,
				uint64(1_100_000_000), uint64(2_500_000))
			require.NoError(t, err)

			_, err = cardanofw.FundUserWithToken(
				ctx, apex, cardanofw.ChainIDCardano,
				minterWalletCardano, apex.Users[i],
				cardanofw.DefaultTokenName, cardanofw.DefaultTokenMintAmount,
				uint64(1_100_000_000), uint64(2_500_000))
			require.NoError(t, err)
		}

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

			if br.bridgingType == sendtx.BridgingTypeNativeTokenOnSource {
				tokenName = wallet.AdaTokenName
			} else {
				tokenName = apex.GetTokenNameForChains(br.dest, br.src)
			}

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

						apex.SubmitBridgingRequest(t, ctx, br.src, br.dest, apex.Users[idx], sendAmount, br.bridgingType, br.receiver)
					} else {
						sendInvalidSendAmountTransaction(t, ctx, apex, br.src, br.dest, apex.Users[idx], sendAmount, br.receiver.GetAddress(br.dest))
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

				if br.bridgingType == sendtx.BridgingTypeNativeTokenOnSource {
					tokenName = wallet.AdaTokenName
				} else {
					tokenName = apex.GetTokenNameForChains(br.dest, br.src)
				}

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
	receiverUserAddr string,
) {
	t.Helper()

	const (
		bridgingFee  = uint64(1_000_010)
		operationFee = uint64(0)
	)

	receivers := []sendtx.BridgingTxReceiver{
		{
			Addr:         receiverUserAddr,
			Amount:       sendAmount.Uint64() * 10,
			BridgingType: sendtx.BridgingTypeCurrencyOnSource,
		},
	}

	feeAmount, err := apex.GetChainMust(t, src).GetBridgingFee(
		ctx, dest, receivers, bridgingFee, operationFee)
	require.NoError(t, err)

	metadata, err := apex.GetChainMust(t, src).CreateMetadata(
		senderUser.GetAddress(src), dest,
		receivers, feeAmount, operationFee)
	require.NoError(t, err)

	_, err = apex.SubmitTx(
		ctx, src, senderUser, apex.GetChainMust(t, src).GetHotWalletAddress(),
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

	primeConfig, cardanoConfig := cardanofw.NewPrimeChainConfig(), cardanofw.NewCardanoChainConfig(true)
	primeConfig.FundTokenAmount = 0 // very important otherwise HWIC wont work

	apex := cardanofw.SetupAndRunSkylineBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithUserCnt(3),
		cardanofw.WithCardanoConfig(cardanoConfig),
		cardanofw.WithPrimeConfig(primeConfig),
		cardanofw.WithCustomConfigHandlers(func(_ *cardanofw.ApexSystem, mp map[string]interface{}) {
			primeSettings := cardanofw.GetMapFromInterfaceKey(mp, "cardanoChains", "prime")
			primeSettings["nativeTokens"] = nil
		}, nil),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	var (
		user             = apex.Users[0]
		apexSendAmount   = cardanofw.ApexToDfm(big.NewInt(2))
		bridgingRequests = []bridgingRequest{
			{src: cardanofw.ChainIDPrime, dest: cardanofw.ChainIDCardano, sender: apex.Users[1], requestType: sendtx.BridgingTypeCurrencyOnSource, isValid: true},
			{src: cardanofw.ChainIDPrime, dest: cardanofw.ChainIDCardano, sender: apex.Users[2], requestType: sendtx.BridgingTypeNativeTokenOnSource, isValid: false},
			{src: cardanofw.ChainIDCardano, dest: cardanofw.ChainIDPrime, sender: apex.Users[1], requestType: sendtx.BridgingTypeCurrencyOnSource, isValid: false},
			{src: cardanofw.ChainIDCardano, dest: cardanofw.ChainIDPrime, sender: apex.Users[2], requestType: sendtx.BridgingTypeNativeTokenOnSource, isValid: true},
		}
		txHashes = make([]string, len(bridgingRequests))
	)

	var wg sync.WaitGroup

	for idx, br := range bridgingRequests {
		wg.Add(1)

		go func(i int, br bridgingRequest) {
			defer wg.Done()

			if br.requestType == sendtx.BridgingTypeNativeTokenOnSource {
				_, err := cardanofw.FundUserWithToken(
					ctx, apex, br.src,
					apex.GetCardanoInfo(br.src).GenesisWallet, br.sender,
					cardanofw.DefaultTokenName, cardanofw.DefaultTokenMintAmount,
					uint64(10_000_000), uint64(100_000_000))
				require.NoError(t, err)
			}

			txHashes[i] = apex.SubmitBridgingRequest(t, ctx, br.src, br.dest, br.sender, apexSendAmount, br.requestType, user)
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

// go test -timeout 0 -run ^TestE2E_SkylineBridge_SimpleStakingTest$ github.com/0xPolygon/polygon-edge/e2e-polybft/e2e -v
func TestE2E_SkylineBridge_SimpleStakingTest(t *testing.T) {
	const apiKey = "test_api_key"

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	primeConfig, cardanoConfig := cardanofw.NewPrimeChainConfig(), cardanofw.NewCardanoChainConfig(true)
	primeConfig.FundTokenAmount = 1_000_000_000
	cardanoConfig.FundTokenAmount = 1_000_000_000

	apex := cardanofw.SetupAndRunSkylineBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithCardanoConfig(cardanoConfig),
		cardanofw.WithPrimeConfig(primeConfig),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	_, err := cardanofw.FundUserWithToken(
		ctx, apex, cardanofw.ChainIDPrime,
		apex.GetCardanoInfo(cardanofw.ChainIDPrime).GenesisWallet, apex.Users[1],
		cardanofw.DefaultTokenName, cardanofw.DefaultTokenMintAmount,
		uint64(2_000_000), uint64(100_000_000))
	require.NoError(t, err)

	_, err = cardanofw.FundUserWithToken(
		ctx, apex, cardanofw.ChainIDCardano,
		apex.GetCardanoInfo(cardanofw.ChainIDCardano).GenesisWallet, apex.Users[1],
		cardanofw.DefaultTokenName, cardanofw.DefaultTokenMintAmount,
		uint64(2_000_000), uint64(100_000_000))
	require.NoError(t, err)

	sendAmountDfm := big.NewInt(1_500_000)

	executeBridging := func(
		srcChainID, dstChainID cardanofw.ChainID, sendAmountDfm *big.Int,
		senders, receivers []*cardanofw.TestApexUser,
	) {
		wg := sync.WaitGroup{}
		wg.Add(2)

		bridgingTypes := []sendtx.BridgingType{
			sendtx.BridgingTypeCurrencyOnSource,
			sendtx.BridgingTypeNativeTokenOnSource,
		}

		for i := range bridgingTypes {
			go func(idx int) {
				defer wg.Done()
				e2ehelper.ExecuteSingleBridging(
					t, ctx, apex, senders[idx], receivers[idx], srcChainID, dstChainID, sendAmountDfm, bridgingTypes[idx])
			}(i)
		}

		wg.Wait()
	}

	executeBridging(cardanofw.ChainIDPrime, cardanofw.ChainIDCardano, sendAmountDfm,
		[]*cardanofw.TestApexUser{apex.Users[0], apex.Users[1]}, []*cardanofw.TestApexUser{apex.Users[2], apex.Users[3]})

	// 1. Check existing stake pools in the system
	stakePools := apex.GetChainMust(t, cardanofw.ChainIDPrime).GetExistingStakePools(t, ctx)
	require.NotEmpty(t, stakePools)

	// 2. Register and delegate bridging address
	err = apex.RegisterAndDelegateStakeAddress(ctx, cardanofw.ChainIDPrime, 0, stakePools[0])
	require.NoError(t, err)

	// 3. Check if the registration and delegation was successful
	addrInfo := apex.GetChainMust(t, cardanofw.ChainIDPrime).GetBridgingStakeAddressInfo(t, ctx, 0)
	require.Equal(t, stakePools[0], addrInfo.StakeDelegation)
	fmt.Println("Bridging address staked successfully")

	executeBridging(cardanofw.ChainIDCardano, cardanofw.ChainIDPrime, sendAmountDfm,
		[]*cardanofw.TestApexUser{apex.Users[0], apex.Users[1]}, []*cardanofw.TestApexUser{apex.Users[2], apex.Users[3]})

	// Revert if we try to register again:
	err = apex.RegisterAndDelegateStakeAddress(ctx, cardanofw.ChainIDPrime, 0, stakePools[0])
	require.Error(t, err)

	// Test redelegation:
	err = apex.RedelegateStakeAddress(ctx, cardanofw.ChainIDPrime, 0, stakePools[1])
	require.NoError(t, err)

	// Check if the redelegation was successful
	previousStakePool := stakePools[0]

	for range 60 {
		time.Sleep(time.Second)

		addrInfo = apex.GetChainMust(t, cardanofw.ChainIDPrime).GetBridgingStakeAddressInfo(t, ctx, 0)

		if addrInfo.StakeDelegation != previousStakePool {
			fmt.Println("Bridging address redelegated successfully")

			break
		}
	}

	require.Equal(t, stakePools[1], addrInfo.StakeDelegation)
}

// go test -timeout 0 -run ^TestE2E_SkylineBridge_SimultaniousStakingTest$ github.com/0xPolygon/polygon-edge/e2e-polybft/e2e -v
func TestE2E_SkylineBridge_SimultaniousStakingTest(t *testing.T) {
	const apiKey = "test_api_key"

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	primeConfig, cardanoConfig := cardanofw.NewPrimeChainConfig(), cardanofw.NewCardanoChainConfig(true)
	primeConfig.FundTokenAmount = 1_000_000_000
	cardanoConfig.FundTokenAmount = 1_000_000_000

	apex := cardanofw.SetupAndRunSkylineBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithCardanoConfig(cardanoConfig),
		cardanofw.WithPrimeConfig(primeConfig),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	_, err := cardanofw.FundUserWithToken(
		ctx, apex, cardanofw.ChainIDPrime,
		apex.GetCardanoInfo(cardanofw.ChainIDPrime).GenesisWallet, apex.Users[1],
		cardanofw.DefaultTokenName, cardanofw.DefaultTokenMintAmount,
		uint64(2_000_000), uint64(100_000_000))
	require.NoError(t, err)

	_, err = cardanofw.FundUserWithToken(
		ctx, apex, cardanofw.ChainIDCardano,
		apex.GetCardanoInfo(cardanofw.ChainIDCardano).GenesisWallet, apex.Users[1],
		cardanofw.DefaultTokenName, cardanofw.DefaultTokenMintAmount,
		uint64(2_000_000), uint64(100_000_000))
	require.NoError(t, err)

	sendAmountDfm := big.NewInt(1_500_000)

	executeBridging := func(
		srcChainID, dstChainID cardanofw.ChainID, sendAmountDfm *big.Int,
		senders, receivers []*cardanofw.TestApexUser, doRegDeleg bool,
	) {
		wg := sync.WaitGroup{}
		wg.Add(5)

		bridgingTypes := []sendtx.BridgingType{
			sendtx.BridgingTypeCurrencyOnSource,
			sendtx.BridgingTypeNativeTokenOnSource,
			sendtx.BridgingTypeCurrencyOnSource,
			sendtx.BridgingTypeCurrencyOnSource,
		}

		for i := range bridgingTypes {
			go func(idx int) {
				defer wg.Done()
				e2ehelper.ExecuteSingleBridging(
					t, ctx, apex, senders[idx], receivers[idx], srcChainID, dstChainID, sendAmountDfm, bridgingTypes[idx])
			}(i)
		}

		go func() {
			defer wg.Done()

			if !doRegDeleg {
				return
			}

			// 1. Check existing stake pools in the system
			stakePools := apex.GetChainMust(t, cardanofw.ChainIDPrime).GetExistingStakePools(t, ctx)
			require.NotEmpty(t, stakePools)

			// 2. Register and delegate bridging address
			err = apex.RegisterAndDelegateStakeAddress(ctx, cardanofw.ChainIDPrime, 0, stakePools[0])
			require.NoError(t, err)

			// 3. Check if the registration and delegation was successful
			addrInfo := apex.GetChainMust(t, cardanofw.ChainIDPrime).GetBridgingStakeAddressInfo(t, ctx, 0)
			require.Equal(t, stakePools[0], addrInfo.StakeDelegation)
			fmt.Println("Bridging address staked successfully")
		}()

		wg.Wait()
	}

	doRegDelegValues := []bool{false, true, false}

	for _, doRegDeleg := range doRegDelegValues {
		executeBridging(
			cardanofw.ChainIDCardano,
			cardanofw.ChainIDPrime,
			sendAmountDfm,
			[]*cardanofw.TestApexUser{
				apex.Users[0], apex.Users[1], apex.Users[2], apex.Users[3],
			},
			[]*cardanofw.TestApexUser{
				apex.Users[4], apex.Users[5], apex.Users[6], apex.Users[7],
			},
			doRegDeleg,
		)
	}
}
