package e2e

import (
	"bytes"
	"context"
	"fmt"
	"math/big"
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

type testConfig struct {
	srcChainID string
	dstChainID string

	srcMinterWallet *wallet.Wallet
	srcNetworkType  wallet.CardanoNetworkType
	srcTxProvider   wallet.ITxProvider
}

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

	transactionTypes := map[sendtx.BridgingType]string{
		sendtx.BridgingTypeCurrencyOnSource:    "BridgingTypeCurrencyOnSource",
		sendtx.BridgingTypeNativeTokenOnSource: "BridgingTypeNativeTokenOnSource",
	}

	txProviderPrime, err := apex.PrimeInfo.GetTxProvider()
	require.NoError(t, err)

	txProviderCardano, err := apex.CardanoInfo.GetTxProvider()
	require.NoError(t, err)

	primeTestConfig := testConfig{
		srcChainID: cardanofw.ChainIDPrime,
		dstChainID: cardanofw.ChainIDCardano,

		srcMinterWallet: apex.PrimeInfo.GenesisWallet,
		srcNetworkType:  apex.Config.PrimeConfig.NetworkType,
		srcTxProvider:   txProviderPrime,
	}

	cardanoTestConfig := testConfig{
		srcChainID: cardanofw.ChainIDCardano,
		dstChainID: cardanofw.ChainIDPrime,

		srcMinterWallet: apex.CardanoInfo.GenesisWallet,
		srcNetworkType:  apex.Config.CardanoConfig.NetworkType,
		srcTxProvider:   txProviderCardano,
	}

	testConfigs := []testConfig{primeTestConfig, cardanoTestConfig}

	minterWalletPrime := apex.PrimeInfo.GenesisWallet
	minterWalletCardano := apex.CardanoInfo.GenesisWallet

	t.Run("1. prime -> cardano - currency on src", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() {
			t.Skip()
		}

		sendAmountDfm := big.NewInt(1_500_000)

		brSubmitterUser, err := cardanofw.NewTestApexUser(cardanofw.NewApexNetworkTypesFromSystem(apex))
		require.NoError(t, err)

		txProviderPrime, err := apex.PrimeInfo.GetTxProvider()
		require.NoError(t, err)

		_, err = cardanofw.FundUserWithToken(
			ctx, cardanofw.ChainIDPrime, apex.Config.PrimeConfig.NetworkType, txProviderPrime,
			minterWalletPrime, brSubmitterUser, uint64(1_100_000_000), uint64(0))
		require.NoError(t, err)

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, brSubmitterUser, user, cardanofw.ChainIDPrime, cardanofw.ChainIDCardano, sendAmountDfm,
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
			ctx, cardanofw.ChainIDPrime, apex.Config.PrimeConfig.NetworkType, txProviderPrime,
			minterWalletPrime, brSubmitterUser, uint64(1_100_000_000), uint64(2_500_000))
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

		brSubmitterUser, err := cardanofw.NewTestApexUser(cardanofw.NewApexNetworkTypesFromSystem(apex))
		require.NoError(t, err)

		_, err = cardanofw.FundUserWithToken(
			ctx, cardanofw.ChainIDCardano, apex.Config.CardanoConfig.NetworkType, txProviderCardano,
			minterWalletCardano, brSubmitterUser, uint64(1_100_000_000), uint64(0))
		require.NoError(t, err)

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, brSubmitterUser, user, cardanofw.ChainIDCardano, cardanofw.ChainIDPrime, sendAmountDfm,
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
			ctx, cardanofw.ChainIDCardano, apex.Config.CardanoConfig.NetworkType, txProviderCardano,
			minterWalletCardano, brSubmitterUser, uint64(1_100_000_000), uint64(2_500_000))
		require.NoError(t, err)

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, brSubmitterUser, user, cardanofw.ChainIDCardano, cardanofw.ChainIDPrime, sendAmountDfm,
			sendtx.BridgingTypeNativeTokenOnSource)
	})

	for idx, cfg := range testConfigs {
		for txType, txTypeString := range transactionTypes {
			t.Run(fmt.Sprintf("1.%d %s -> %s - %s", idx+1, cfg.srcChainID, cfg.dstChainID, txTypeString), func(t *testing.T) {
				if cardanofw.ShouldSkipE2RRedundantTests() {
					t.Skip()
				}

				sendAmountDfm := big.NewInt(1_500_000)

				brSubmitterUser, err := cardanofw.NewTestApexUser(cardanofw.NewApexNetworkTypesFromSystem(apex))
				require.NoError(t, err)

				_, err = cardanofw.FundUserWithToken(
					ctx, cfg.srcChainID, cfg.srcNetworkType, cfg.srcTxProvider,
					cfg.srcMinterWallet, brSubmitterUser, uint64(1_100_000_000), uint64(10_000_000))
				require.NoError(t, err)

				e2ehelper.ExecuteSingleBridging(
					t, ctx, apex, brSubmitterUser, user, cfg.srcChainID, cfg.dstChainID, sendAmountDfm,
					txType)
			})
		}
	}

	for idx, cfg := range testConfigs {
		for txType, txTypeString := range transactionTypes {
			t.Run(fmt.Sprintf("2.%d %s -> %s - Submitter has tokens - %s", idx+1, cfg.srcChainID, cfg.dstChainID, txTypeString), func(t *testing.T) {
				if cardanofw.ShouldSkipE2RRedundantTests() {
					t.Skip()
				}

				sendAmountDfm := big.NewInt(5_000_000)
				minterUser := apex.Users[userCnt-2]

				brSubmitterUser, err := cardanofw.NewTestApexUser(cardanofw.NewApexNetworkTypesFromSystem(apex))
				require.NoError(t, err)

				minterWallet, _ := minterUser.GetCardanoWallet(cfg.srcChainID)

				_, err = cardanofw.FundUserWithToken(
					ctx, cfg.srcChainID, cfg.srcNetworkType, cfg.srcTxProvider,
					minterWallet, brSubmitterUser, uint64(50_000_000), uint64(1_000_000))
				require.NoError(t, err)

				_, err = cardanofw.FundUserWithToken(
					ctx, cfg.srcChainID, cfg.srcNetworkType, cfg.srcTxProvider,
					cfg.srcMinterWallet, brSubmitterUser, uint64(50_000_000), uint64(10_000_000))
				require.NoError(t, err)

				e2ehelper.ExecuteSingleBridging(
					t, ctx, apex, brSubmitterUser, user, cfg.srcChainID, cfg.dstChainID, sendAmountDfm,
					txType)
			})
		}
	}

	for idx, cfg := range testConfigs {
		for txType, txTypeString := range transactionTypes {
			t.Run(fmt.Sprintf("3.%d %s -> %s - wait for each submit - %s", idx+1, cfg.srcChainID, cfg.dstChainID, txTypeString), func(t *testing.T) {
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
					ctx, cfg.srcChainID, cfg.srcNetworkType, cfg.srcTxProvider,
					cfg.srcMinterWallet, brSubmitterUser, uint64(1_100_000_000), uint64(10_000_000))
				require.NoError(t, err)

				e2ehelper.ExecuteBridgingOneByOneWaitOnOtherSide(
					t, ctx, apex, instances, brSubmitterUser, cfg.srcChainID, cfg.dstChainID, new(big.Int).SetUint64(sendAmount),
					txType)
			})
		}
	}

	for idx, cfg := range testConfigs {
		for txType, txTypeString := range transactionTypes {
			t.Run(fmt.Sprintf("4.%d %s -> %s - one by one - %s", idx+1, cfg.srcChainID, cfg.dstChainID, txTypeString), func(t *testing.T) {
				if cardanofw.ShouldSkipE2RRedundantTests() {
					t.Skip()
				}

				const (
					sendAmount = uint64(1_000_001)
					instances  = 5
				)

				brSubmitterUser, err := cardanofw.NewTestApexUser(cardanofw.NewApexNetworkTypesFromSystem(apex))
				require.NoError(t, err)

				_, err = cardanofw.FundUserWithToken(
					ctx, cfg.srcChainID, cfg.srcNetworkType, cfg.srcTxProvider,
					cfg.srcMinterWallet, brSubmitterUser, uint64(1_100_000_000), uint64(10_000_000))
				require.NoError(t, err)

				e2ehelper.ExecuteBridgingWaitAfterSubmits(
					t, ctx, apex, instances, brSubmitterUser, cfg.srcChainID, cfg.dstChainID, new(big.Int).SetUint64(sendAmount),
					txType)
			})
		}
	}

	for idx, cfg := range testConfigs {
		for txType, txTypeString := range transactionTypes {
			t.Run(fmt.Sprintf("5.%d %s -> %s - parallel - %s", idx+1, cfg.srcChainID, cfg.dstChainID, txTypeString), func(t *testing.T) {
				if cardanofw.ShouldSkipE2RRedundantTests() {
					t.Skip()
				}

				const (
					sendAmount = uint64(1_000_000)
					instances  = 5
				)

				for _, cfg := range testConfigs {
					for _, sender := range apex.Users[:instances] {
						_, err = cardanofw.FundUserWithToken(
							ctx, cfg.srcChainID, cfg.srcNetworkType, cfg.srcTxProvider,
							cfg.srcMinterWallet, sender, uint64(1_100_000_000), uint64(10_000_000))
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
		for txType, txTypeString := range transactionTypes {
			t.Run(fmt.Sprintf("6.%d %s -> %s - sequential and parallel - %s", idx+1, cfg.srcChainID, cfg.dstChainID, txTypeString), func(t *testing.T) {
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
						_, err = cardanofw.FundUserWithToken(
							ctx, cfg.srcChainID, cfg.srcNetworkType, cfg.srcTxProvider,
							cfg.srcMinterWallet, sender, uint64(1_100_000_000), uint64(210_000_000))
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

	for txType, txTypeString := range transactionTypes {
		t.Run(fmt.Sprintf("7.%d Both directions sequential - %s", idx, txTypeString), func(t *testing.T) {
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
					ctx, cfg.srcChainID, cfg.srcNetworkType, cfg.srcTxProvider,
					cfg.srcMinterWallet, brSubmitterUser, uint64(1_100_000_000), uint64(10_000_000))
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

	for txType, txTypeString := range transactionTypes {
		t.Run(fmt.Sprintf("8.%d Both directions sequential and parallel - %s", idx, txTypeString), func(t *testing.T) {
			const (
				sendAmount          = uint64(1_000_000)
				sequentialInstances = 5
				parallelInstances   = 6
			)

			for _, cfg := range testConfigs {
				for _, sender := range apex.Users[:parallelInstances] {
					_, err = cardanofw.FundUserWithToken(
						ctx, cfg.srcChainID, cfg.srcNetworkType, cfg.srcTxProvider,
						cfg.srcMinterWallet, sender, uint64(1_100_000_000), uint64(10_000_000))
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

	for txType, txTypeString := range transactionTypes {
		t.Run(fmt.Sprintf("9.%d Both directions sequential and parallel - one node goes off in the middle - %s",
			idx, txTypeString), func(t *testing.T) {
			const (
				sendAmount           = uint64(1_000_000)
				sequentialInstances  = 5
				parallelInstances    = 6
				stopAfter            = time.Second * 60
				validatorStoppingIdx = 1
			)

			for _, cfg := range testConfigs {
				for _, sender := range apex.Users[:parallelInstances] {
					_, err = cardanofw.FundUserWithToken(
						ctx, cfg.srcChainID, cfg.srcNetworkType, cfg.srcTxProvider,
						cfg.srcMinterWallet, sender, uint64(1_100_000_000), uint64(10_000_000))
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

	for txType, txTypeString := range transactionTypes {
		t.Run(fmt.Sprintf("10.%d Both directions sequential and parallel - one node goes off in the middle - %s",
			idx, txTypeString), func(t *testing.T) {
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
					_, err = cardanofw.FundUserWithToken(
						ctx, cfg.srcChainID, cfg.srcNetworkType, cfg.srcTxProvider,
						cfg.srcMinterWallet, sender, uint64(1_100_000_000), uint64(10_000_000))
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

		bridgingFeeAmount = uint64(1_000_010)
		operationFee      = uint64(0)
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

	txProviderPrime, err := apex.PrimeInfo.GetTxProvider()
	require.NoError(t, err)

	fmt.Println("prime user addr: ", user.PrimeAddress)
	fmt.Println("cardano user addr: ", user.CardanoAddress)
	fmt.Println("prime multisig addr: ", apex.PrimeInfo.MultisigAddr)
	fmt.Println("prime fee addr: ", apex.PrimeInfo.FeeAddr)
	fmt.Printf("prime socket path: %s\n", apex.PrimeInfo.SocketPath)
	fmt.Println("cardano multisig addr: ", apex.CardanoInfo.MultisigAddr)
	fmt.Println("cardano fee addr: ", apex.CardanoInfo.FeeAddr)
	fmt.Printf("cardano socket path: %s\n", apex.CardanoInfo.SocketPath)

	t.Run("1. Mismatch submitted and receiver amounts", func(t *testing.T) {
		executeSkylineMismatchedAndReceivedAmounts(
			t, ctx, apex, cardanofw.ChainIDPrime, cardanofw.ChainIDCardano,
			bridgingFeeAmount, operationFee, 0)
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
				ctx, cardanofw.ChainIDCardano, receivers, bridgingFeeAmount, operationFee)
			require.NoError(t, err)

			metadata, err := apex.GetChainMust(t, cardanofw.ChainIDPrime).CreateMetadata(
				apex.Users[i].GetAddress(cardanofw.ChainIDPrime), cardanofw.ChainIDCardano,
				receivers, feeAmount, operationFee)
			require.NoError(t, err)

			txHash, err := apex.SubmitTx(
				ctx, cardanofw.ChainIDPrime, apex.Users[i],
				apex.PrimeInfo.MultisigAddr, new(big.Int).SetUint64(sendAmount+feeAmount+operationFee), metadata)
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
					ctx, cardanofw.ChainIDCardano, receivers, bridgingFeeAmount, operationFee)
				require.NoError(t, err)

				metadata, err := apex.GetChainMust(t, cardanofw.ChainIDPrime).CreateMetadata(
					testUser.GetAddress(cardanofw.ChainIDPrime), cardanofw.ChainIDCardano,
					receivers, feeAmount, operationFee)
				require.NoError(t, err)

				txHashes[idx], err = apex.SubmitTx(
					ctx, cardanofw.ChainIDPrime, testUser,
					apex.PrimeInfo.MultisigAddr, new(big.Int).SetUint64(sendAmount+feeAmount+operationFee), metadata)
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
			ctx, cardanofw.ChainIDCardano, receivers, bridgingFeeAmount, operationFee)
		require.NoError(t, err)

		metadata, err := apex.GetChainMust(t, cardanofw.ChainIDPrime).CreateMetadata(
			user.GetAddress(cardanofw.ChainIDPrime), cardanofw.ChainIDCardano,
			receivers, feeAmount, operationFee)
		require.NoError(t, err)

		// Send only half bytes of metadata making it invalid
		metadata = metadata[0 : len(metadata)/2]

		_, err = apex.SubmitTx(
			ctx, cardanofw.ChainIDPrime, user,
			apex.PrimeInfo.MultisigAddr, new(big.Int).SetUint64(sendAmount+feeAmount+operationFee), metadata)
		require.Error(t, err)
	})

	t.Run("5. Submitted invalid metadata - wrong type", func(t *testing.T) {
		sendAmount := uint64(1_000_000)
		receivers := []sendtx.BridgingTxReceiver{
			{
				Addr:         user.GetAddress(cardanofw.ChainIDCardano),
				Amount:       sendAmount,
				BridgingType: sendtx.BridgingTypeCurrencyOnSource,
			},
		}

		feeAmount, err := apex.GetChainMust(t, cardanofw.ChainIDPrime).GetBridgingFee(
			ctx, cardanofw.ChainIDCardano, receivers, bridgingFeeAmount, operationFee)
		require.NoError(t, err)

		metadata, err := apex.GetChainMust(t, cardanofw.ChainIDPrime).CreateMetadata(
			user.GetAddress(cardanofw.ChainIDPrime), cardanofw.ChainIDCardano,
			receivers, feeAmount, operationFee)
		require.NoError(t, err)

		bridgingRequestMetadata := bytes.Replace(metadata, []byte("bridge"), []byte("xxxxx"), 1)

		txHash, err := apex.SubmitTx(
			ctx, cardanofw.ChainIDPrime, user,
			apex.PrimeInfo.MultisigAddr, new(big.Int).SetUint64(sendAmount+feeAmount+operationFee), bridgingRequestMetadata)
		require.NoError(t, err)

		_, err = cardanofw.WaitForRequestStates(ctx, apex, cardanofw.ChainIDPrime, txHash, apex.Config.APIKey, nil, 60)
		require.Error(t, err)
		require.ErrorContains(t, err, "timeout")
	})

	t.Run("6. Submitted invalid metadata - invalid destination", func(t *testing.T) {
		sendAmount := uint64(1_000_000)
		receivers := []sendtx.BridgingTxReceiver{
			{
				Addr:         user.GetAddress(cardanofw.ChainIDCardano),
				Amount:       sendAmount,
				BridgingType: sendtx.BridgingTypeCurrencyOnSource,
			},
		}

		feeAmount, err := apex.GetChainMust(t, cardanofw.ChainIDPrime).GetBridgingFee(
			ctx, cardanofw.ChainIDCardano, receivers, bridgingFeeAmount, operationFee)
		require.NoError(t, err)

		metadata, err := apex.GetChainMust(t, cardanofw.ChainIDPrime).CreateMetadata(
			user.GetAddress(cardanofw.ChainIDPrime), cardanofw.ChainIDCardano,
			receivers, feeAmount, operationFee)
		require.NoError(t, err)

		bridgingRequestMetadata := bytes.Replace(metadata,
			[]byte(fmt.Sprintf("\"%s\"", cardanofw.ChainIDCardano)), []byte("\"hector\""), 1)

		txHash, err := apex.SubmitTx(ctx, cardanofw.ChainIDPrime, user,
			apex.PrimeInfo.MultisigAddr, new(big.Int).SetUint64(sendAmount+feeAmount+operationFee), bridgingRequestMetadata)
		require.NoError(t, err)

		cardanofw.WaitForInvalidState(t, ctx, apex, cardanofw.ChainIDPrime, txHash, apex.Config.APIKey, 0)
	})

	//nolint:dupl
	t.Run("7. Submitted invalid metadata - invalid sender", func(t *testing.T) {
		sendAmount := uint64(1_000_000)
		receivers := []sendtx.BridgingTxReceiver{
			{
				Addr:         user.GetAddress(cardanofw.ChainIDCardano),
				Amount:       sendAmount,
				BridgingType: sendtx.BridgingTypeCurrencyOnSource,
			},
		}

		feeAmount, err := apex.GetChainMust(t, cardanofw.ChainIDPrime).GetBridgingFee(
			ctx, cardanofw.ChainIDCardano, receivers, bridgingFeeAmount, operationFee)
		require.NoError(t, err)

		metadata, err := apex.GetChainMust(t, cardanofw.ChainIDPrime).CreateMetadata(
			"dummy", cardanofw.ChainIDCardano,
			receivers, feeAmount, operationFee)
		require.NoError(t, err)

		// remove this after we make correct validation on oracle!
		bridgingRequestMetadata := bytes.Replace(metadata,
			[]byte("[\"dummy\"]"), []byte("\"\""), 1)

		txHash, err := apex.SubmitTx(ctx, cardanofw.ChainIDPrime, user, apex.PrimeInfo.MultisigAddr,
			new(big.Int).SetUint64(sendAmount+feeAmount+operationFee), bridgingRequestMetadata)
		require.NoError(t, err)

		cardanofw.WaitForInvalidState(t, ctx, apex, cardanofw.ChainIDPrime, txHash, apex.Config.APIKey, 0)
	})

	//nolint:dupl
	t.Run("8. Submitted invalid metadata - invalid operationFee", func(t *testing.T) {
		sendAmount := uint64(1_000_000)
		receivers := []sendtx.BridgingTxReceiver{
			{
				Addr:         user.GetAddress(cardanofw.ChainIDCardano),
				Amount:       sendAmount,
				BridgingType: sendtx.BridgingTypeCurrencyOnSource,
			},
		}

		feeAmount, err := apex.GetChainMust(t, cardanofw.ChainIDPrime).GetBridgingFee(
			ctx, cardanofw.ChainIDCardano, receivers, bridgingFeeAmount, operationFee)
		require.NoError(t, err)

		metadata, err := apex.GetChainMust(t, cardanofw.ChainIDPrime).CreateMetadata(
			"dummy", cardanofw.ChainIDCardano,
			receivers, feeAmount, operationFee)
		require.NoError(t, err)

		bridgingRequestMetadata := bytes.Replace(metadata,
			[]byte("1000010"), []byte("1"), 1)

		txHash, err := apex.SubmitTx(ctx, cardanofw.ChainIDPrime, user, apex.PrimeInfo.MultisigAddr,
			new(big.Int).SetUint64(sendAmount+feeAmount+operationFee), bridgingRequestMetadata)
		require.NoError(t, err)

		cardanofw.WaitForInvalidState(t, ctx, apex, cardanofw.ChainIDPrime, txHash, apex.Config.APIKey, 0)
	})

	t.Run("9. Submitted invalid metadata - invalid receiver address - token on source", func(t *testing.T) {
		sendAmount := uint64(1_000_000)

		brSubmitterUser, err := cardanofw.NewTestApexUser(cardanofw.NewApexNetworkTypesFromSystem(apex))
		require.NoError(t, err)

		brSubmitterWallet, _ := brSubmitterUser.GetCardanoWallet(cardanofw.ChainIDPrime)

		minterWallet := apex.PrimeInfo.GenesisWallet

		tokensFunded, err := cardanofw.FundUserWithToken(
			ctx, cardanofw.ChainIDPrime, apex.Config.PrimeConfig.NetworkType, txProviderPrime,
			minterWallet, brSubmitterUser, uint64(10_000_000), uint64(1_000_000))
		require.NoError(t, err)

		receivers := []sendtx.BridgingTxReceiver{
			{
				Addr:         user.GetAddress(cardanofw.ChainIDCardano),
				Amount:       sendAmount,
				BridgingType: sendtx.BridgingTypeNativeTokenOnSource,
			},
			{
				Addr:         apex.CardanoInfo.FeeAddr,
				Amount:       bridgingFeeAmount,
				BridgingType: sendtx.BridgingTypeNativeTokenOnSource,
			},
		}

		feeAmount, err := apex.GetChainMust(t, cardanofw.ChainIDPrime).GetBridgingFee(
			ctx, cardanofw.ChainIDCardano, receivers, bridgingFeeAmount, operationFee)
		require.NoError(t, err)

		metadata, err := apex.GetChainMust(t, cardanofw.ChainIDPrime).CreateMetadata(
			"dummy", cardanofw.ChainIDCardano,
			receivers, feeAmount, operationFee)
		require.NoError(t, err)

		txHash, err := cardanofw.SendTxWithTokens(ctx, cardanofw.ChainIDPrime, apex.Config.PrimeConfig.NetworkType, txProviderPrime,
			brSubmitterWallet, apex.PrimeInfo.MultisigAddr,
			feeAmount+operationFee, []wallet.TokenAmount{*tokensFunded}, metadata,
		)
		require.NoError(t, err)

		cardanofw.WaitForInvalidState(t, ctx, apex, cardanofw.ChainIDPrime, txHash, apex.Config.APIKey, 0)
	})

	t.Run("10. Submitted invalid metadata - empty tx", func(t *testing.T) {
		sendAmount := uint64(1_000_000)
		receivers := []sendtx.BridgingTxReceiver{}

		feeAmount, err := apex.GetChainMust(t, cardanofw.ChainIDPrime).GetBridgingFee(
			ctx, cardanofw.ChainIDCardano, receivers, bridgingFeeAmount, operationFee)
		require.NoError(t, err)

		metadata, err := apex.GetChainMust(t, cardanofw.ChainIDPrime).CreateMetadata(
			user.GetAddress(cardanofw.ChainIDPrime), cardanofw.ChainIDCardano,
			receivers, feeAmount, operationFee)
		require.NoError(t, err)

		txHash, err := apex.SubmitTx(ctx, cardanofw.ChainIDPrime, user, apex.PrimeInfo.MultisigAddr,
			new(big.Int).SetUint64(sendAmount), metadata)
		require.NoError(t, err)

		cardanofw.WaitForInvalidState(t, ctx, apex, cardanofw.ChainIDPrime, txHash, apex.Config.APIKey, 0)
	})

	t.Run("11. Submitted with tokens to bridging addr", func(t *testing.T) {
		sendAmount := uint64(5_000_000)

		minterUser := apex.Users[userCnt-1]

		brSubmitterUser, err := cardanofw.NewTestApexUser(cardanofw.NewApexNetworkTypesFromSystem(apex))
		require.NoError(t, err)

		minterWallet, _ := minterUser.GetCardanoWallet(cardanofw.ChainIDPrime)

		tokensFunded, err := cardanofw.FundUserWithToken(
			ctx, cardanofw.ChainIDPrime, apex.Config.PrimeConfig.NetworkType, txProviderPrime,
			minterWallet, brSubmitterUser, uint64(10_000_000), uint64(1_000_000))
		require.NoError(t, err)

		receivers := []sendtx.BridgingTxReceiver{
			{
				Addr:         user.GetAddress(cardanofw.ChainIDCardano),
				Amount:       sendAmount,
				BridgingType: sendtx.BridgingTypeCurrencyOnSource,
			},
		}

		feeAmount, err := apex.GetChainMust(t, cardanofw.ChainIDPrime).GetBridgingFee(
			ctx, cardanofw.ChainIDCardano, receivers, bridgingFeeAmount, operationFee)
		require.NoError(t, err)

		metadata, err := apex.GetChainMust(t, cardanofw.ChainIDPrime).CreateMetadata(
			user.GetAddress(cardanofw.ChainIDPrime), cardanofw.ChainIDCardano,
			receivers, feeAmount, operationFee)
		require.NoError(t, err)

		brSubmitterWallet, _ := brSubmitterUser.GetCardanoWallet(cardanofw.ChainIDPrime)

		txHash, err := cardanofw.SendTxWithTokens(ctx, cardanofw.ChainIDPrime, apex.Config.PrimeConfig.NetworkType, txProviderPrime,
			brSubmitterWallet, apex.PrimeInfo.MultisigAddr,
			sendAmount+feeAmount+operationFee, []wallet.TokenAmount{*tokensFunded}, metadata,
		)
		require.NoError(t, err)

		cardanofw.WaitForInvalidState(t, ctx, apex, cardanofw.ChainIDPrime, txHash, apiKey, 0)
	})

	t.Run("12. Submitted invalid metadata - invalid send amount - token on source", func(t *testing.T) {
		sendAmount := uint64(1_123_000)

		brSubmitterUser, err := cardanofw.NewTestApexUser(cardanofw.NewApexNetworkTypesFromSystem(apex))
		require.NoError(t, err)

		brSubmitterWallet, _ := brSubmitterUser.GetCardanoWallet(cardanofw.ChainIDPrime)

		minterWallet := apex.PrimeInfo.GenesisWallet

		tokensFunded, err := cardanofw.FundUserWithToken(
			ctx, cardanofw.ChainIDPrime, apex.Config.PrimeConfig.NetworkType, txProviderPrime,
			minterWallet, brSubmitterUser, uint64(10_000_000), sendAmount)
		require.NoError(t, err)

		receivers := []sendtx.BridgingTxReceiver{
			{
				Addr:         user.GetAddress(cardanofw.ChainIDCardano),
				Amount:       sendAmount,
				BridgingType: sendtx.BridgingTypeNativeTokenOnSource,
			},
		}

		feeAmount, err := apex.GetChainMust(t, cardanofw.ChainIDPrime).GetBridgingFee(
			ctx, cardanofw.ChainIDCardano, receivers, bridgingFeeAmount, operationFee)
		require.NoError(t, err)

		metadata, err := apex.GetChainMust(t, cardanofw.ChainIDPrime).CreateMetadata(
			"dummy", cardanofw.ChainIDCardano,
			receivers, feeAmount, operationFee)
		require.NoError(t, err)

		bridgingRequestMetadata := bytes.Replace(metadata,
			[]byte("1123000"), []byte("1000000"), 1)

		txHash, err := cardanofw.SendTxWithTokens(ctx, cardanofw.ChainIDPrime, apex.Config.PrimeConfig.NetworkType, txProviderPrime,
			brSubmitterWallet, apex.PrimeInfo.MultisigAddr,
			feeAmount+operationFee, []wallet.TokenAmount{*tokensFunded}, bridgingRequestMetadata,
		)
		require.NoError(t, err)

		cardanofw.WaitForInvalidState(t, ctx, apex, cardanofw.ChainIDPrime, txHash, apex.Config.APIKey, 0)
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

	txProviderPrime, err := apex.PrimeInfo.GetTxProvider()
	require.NoError(t, err)

	txProviderCardano, err := apex.CardanoInfo.GetTxProvider()
	require.NoError(t, err)

	for _, sender := range apex.Users[:parallelInstances] {
		_, err = cardanofw.FundUserWithToken(
			ctx, cardanofw.ChainIDPrime, apex.Config.PrimeConfig.NetworkType, txProviderPrime,
			apex.PrimeInfo.GenesisWallet, sender, uint64(2_000_000_000), uint64(2_000_000_000))
		require.NoError(t, err)

		_, err = cardanofw.FundUserWithToken(
			ctx, cardanofw.ChainIDCardano, apex.Config.CardanoConfig.NetworkType, txProviderCardano,
			apex.CardanoInfo.GenesisWallet, sender, uint64(2_000_000_000), uint64(2_000_000_000))
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

	txProviderPrime, err := apex.PrimeInfo.GetTxProvider()
	require.NoError(t, err)

	txProviderCardano, err := apex.CardanoInfo.GetTxProvider()
	require.NoError(t, err)

	for _, sender := range apex.Users[:parallelInstances] {
		_, err = cardanofw.FundUserWithToken(
			ctx, cardanofw.ChainIDPrime, apex.Config.PrimeConfig.NetworkType, txProviderPrime,
			apex.PrimeInfo.GenesisWallet, sender, uint64(2_000_000_000), uint64(2_000_000_000))
		require.NoError(t, err)

		_, err = cardanofw.FundUserWithToken(
			ctx, cardanofw.ChainIDCardano, apex.Config.CardanoConfig.NetworkType, txProviderCardano,
			apex.CardanoInfo.GenesisWallet, sender, uint64(2_000_000_000), uint64(2_000_000_000))
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

		txProviderPrime, err := apex.PrimeInfo.GetTxProvider()
		require.NoError(t, err)

		txProviderCardano, err := apex.CardanoInfo.GetTxProvider()
		require.NoError(t, err)

		_, err = cardanofw.FundUserWithToken(
			ctx, cardanofw.ChainIDPrime, apex.Config.PrimeConfig.NetworkType, txProviderPrime,
			minterWalletPrime, apex.Users[0], uint64(2_000_000), uint64(50_000_000))
		require.NoError(t, err)

		_, err = cardanofw.FundUserWithToken(
			ctx, cardanofw.ChainIDCardano, apex.Config.CardanoConfig.NetworkType, txProviderCardano,
			minterWalletCardano, apex.Users[0], uint64(2_000_000), uint64(50_000_000))
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

		txProviderPrime, err := apex.PrimeInfo.GetTxProvider()
		require.NoError(t, err)

		txProviderCardano, err := apex.CardanoInfo.GetTxProvider()
		require.NoError(t, err)

		_, err = cardanofw.FundUserWithToken(
			ctx, cardanofw.ChainIDPrime, apex.Config.PrimeConfig.NetworkType, txProviderPrime,
			minterWalletPrime, apex.Users[0], uint64(2_000_000), uint64(250_000_000))
		require.NoError(t, err)

		_, err = cardanofw.FundUserWithToken(
			ctx, cardanofw.ChainIDCardano, apex.Config.CardanoConfig.NetworkType, txProviderCardano,
			minterWalletCardano, apex.Users[1], uint64(2_000_000), uint64(250_000_000))
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

		txProviderPrime, err := apex.PrimeInfo.GetTxProvider()
		require.NoError(t, err)

		txProviderCardano, err := apex.CardanoInfo.GetTxProvider()
		require.NoError(t, err)

		_, err = cardanofw.FundUserWithToken(
			ctx, cardanofw.ChainIDPrime, apex.Config.PrimeConfig.NetworkType, txProviderPrime,
			minterWalletPrime, apex.Users[0], uint64(2_000_000), uint64(50_000_000))
		require.NoError(t, err)

		_, err = cardanofw.FundUserWithToken(
			ctx, cardanofw.ChainIDCardano, apex.Config.CardanoConfig.NetworkType, txProviderCardano,
			minterWalletCardano, apex.Users[0], uint64(2_000_000), uint64(150_000_000))
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

		txProviderPrime, err := apex.PrimeInfo.GetTxProvider()
		require.NoError(t, err)

		txProviderCardano, err := apex.CardanoInfo.GetTxProvider()
		require.NoError(t, err)

		_, err = cardanofw.FundUserWithToken(
			ctx, cardanofw.ChainIDPrime, apex.Config.PrimeConfig.NetworkType, txProviderPrime,
			minterWalletPrime, apex.Users[0], uint64(2_000_000), uint64(250_000_000))
		require.NoError(t, err)

		_, err = cardanofw.FundUserWithToken(
			ctx, cardanofw.ChainIDCardano, apex.Config.CardanoConfig.NetworkType, txProviderCardano,
			minterWalletCardano, apex.Users[0], uint64(2_000_000), uint64(250_000_000))
		require.NoError(t, err)

		_, err = cardanofw.FundUserWithToken(
			ctx, cardanofw.ChainIDPrime, apex.Config.PrimeConfig.NetworkType, txProviderPrime,
			minterWalletPrime, apex.Users[1], uint64(2_000_000), uint64(250_000_000))
		require.NoError(t, err)

		_, err = cardanofw.FundUserWithToken(
			ctx, cardanofw.ChainIDCardano, apex.Config.CardanoConfig.NetworkType, txProviderCardano,
			minterWalletCardano, apex.Users[1], uint64(2_000_000), uint64(250_000_000))
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
