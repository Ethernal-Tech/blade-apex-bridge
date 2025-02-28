package e2e

import (
	"context"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/e2e-polybft/cardanofw"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/e2ehelper"
	"github.com/Ethernal-Tech/cardano-infrastructure/sendtx"
	"github.com/Ethernal-Tech/cardano-infrastructure/wallet"
	"github.com/stretchr/testify/require"
)

type testConfig struct {
	srcChainID string
	dstChainID string

	srcMinterWallet *wallet.Wallet
	srcNetworkType  wallet.CardanoNetworkType
	srcTxProvider   wallet.ITxProvider
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

		brSubmitterUser, err := cardanofw.NewTestApexUserSkyline(
			apex.Config.PrimeConfig.NetworkType, false, 0, false, 0, false)
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

		brSubmitterUser, err := cardanofw.NewTestApexUserSkyline(
			apex.Config.PrimeConfig.NetworkType, false, 0, false, 0, false)
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

		brSubmitterUser, err := cardanofw.NewTestApexUserSkyline(
			apex.Config.PrimeConfig.NetworkType, false, 0, true, apex.Config.CardanoConfig.NetworkType, false)
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

		brSubmitterUser, err := cardanofw.NewTestApexUserSkyline(
			apex.Config.PrimeConfig.NetworkType, false, 0, true, apex.Config.CardanoConfig.NetworkType, false)
		require.NoError(t, err)

		_, err = cardanofw.FundUserWithToken(
			ctx, cardanofw.ChainIDCardano, apex.Config.CardanoConfig.NetworkType, txProviderCardano,
			minterWalletCardano, brSubmitterUser, uint64(1_100_000_000), uint64(2_500_000))
		require.NoError(t, err)

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, brSubmitterUser, user, cardanofw.ChainIDCardano, cardanofw.ChainIDPrime, sendAmountDfm,
			sendtx.BridgingTypeNativeTokenOnSource)
	})

	idx := 1

	for _, cfg := range testConfigs {
		for txType, txTypeString := range transactionTypes {
			t.Run(fmt.Sprintf("1.%d %s -> %s - %s", idx, cfg.srcChainID, cfg.dstChainID, txTypeString), func(t *testing.T) {
				if cardanofw.ShouldSkipE2RRedundantTests() {
					t.Skip()
				}

				sendAmountDfm := big.NewInt(1_500_000)

				brSubmitterUser, err := cardanofw.NewTestApexUserSkyline(
					apex.Config.PrimeConfig.NetworkType, false, 0, true, apex.Config.CardanoConfig.NetworkType, false)
				require.NoError(t, err)

				_, err = cardanofw.FundUserWithToken(
					ctx, cfg.srcChainID, cfg.srcNetworkType, cfg.srcTxProvider,
					cfg.srcMinterWallet, brSubmitterUser, uint64(1_100_000_000), uint64(10_000_000))
				require.NoError(t, err)

				e2ehelper.ExecuteSingleBridging(
					t, ctx, apex, brSubmitterUser, user, cfg.srcChainID, cfg.dstChainID, sendAmountDfm,
					txType)
			})

			idx++
		}
	}

	idx = 1

	for _, cfg := range testConfigs {
		for txType, txTypeString := range transactionTypes {
			t.Run(fmt.Sprintf("2.%d %s -> %s - Submitter has tokens - %s", idx, cfg.srcChainID, cfg.dstChainID, txTypeString), func(t *testing.T) {
				if cardanofw.ShouldSkipE2RRedundantTests() {
					t.Skip()
				}

				sendAmountDfm := big.NewInt(5_000_000)
				minterUser := apex.Users[userCnt-2]

				brSubmitterUser, err := cardanofw.NewTestApexUserSkyline(
					apex.Config.PrimeConfig.NetworkType, false, 0, true, apex.Config.CardanoConfig.NetworkType, false)
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

			idx++
		}
	}

	idx = 1
	//nolint:dupl
	for _, cfg := range testConfigs {
		for txType, txTypeString := range transactionTypes {
			t.Run(fmt.Sprintf("3.%d %s -> %s - wait for each submit - %s", idx, cfg.srcChainID, cfg.dstChainID, txTypeString), func(t *testing.T) {
				if cardanofw.ShouldSkipE2RRedundantTests() {
					t.Skip()
				}

				const (
					sendAmount = uint64(1_000_000)
					instances  = 5
				)

				brSubmitterUser, err := cardanofw.NewTestApexUserSkyline(
					apex.Config.PrimeConfig.NetworkType, false, 0, true, apex.Config.CardanoConfig.NetworkType, false)
				require.NoError(t, err)

				_, err = cardanofw.FundUserWithToken(
					ctx, cfg.srcChainID, cfg.srcNetworkType, cfg.srcTxProvider,
					cfg.srcMinterWallet, brSubmitterUser, uint64(1_100_000_000), uint64(10_000_000))
				require.NoError(t, err)

				e2ehelper.ExecuteBridgingOneByOneWaitOnOtherSide(
					t, ctx, apex, instances, brSubmitterUser, cfg.srcChainID, cfg.dstChainID, new(big.Int).SetUint64(sendAmount),
					txType)
			})

			idx++
		}
	}

	idx = 1
	//nolint:dupl
	for _, cfg := range testConfigs {
		for txType, txTypeString := range transactionTypes {
			t.Run(fmt.Sprintf("4.%d %s -> %s - one by one - %s", idx, cfg.srcChainID, cfg.dstChainID, txTypeString), func(t *testing.T) {
				if cardanofw.ShouldSkipE2RRedundantTests() {
					t.Skip()
				}

				const (
					sendAmount = uint64(1_000_000)
					instances  = 5
				)

				brSubmitterUser, err := cardanofw.NewTestApexUserSkyline(
					apex.Config.PrimeConfig.NetworkType, false, 0, true, apex.Config.CardanoConfig.NetworkType, false)
				require.NoError(t, err)

				_, err = cardanofw.FundUserWithToken(
					ctx, cfg.srcChainID, cfg.srcNetworkType, cfg.srcTxProvider,
					cfg.srcMinterWallet, brSubmitterUser, uint64(1_100_000_000), uint64(10_000_000))
				require.NoError(t, err)

				e2ehelper.ExecuteBridgingWaitAfterSubmits(
					t, ctx, apex, instances, brSubmitterUser, cfg.srcChainID, cfg.dstChainID, new(big.Int).SetUint64(sendAmount),
					txType)
			})

			idx++
		}
	}

	idx = 1

	for _, cfg := range testConfigs {
		for txType, txTypeString := range transactionTypes {
			t.Run(fmt.Sprintf("5.%d %s -> %s - parallel - %s", idx, cfg.srcChainID, cfg.dstChainID, txTypeString), func(t *testing.T) {
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

			idx++
		}
	}

	idx = 1

	for _, cfg := range testConfigs {
		for txType, txTypeString := range transactionTypes {
			t.Run(fmt.Sprintf("6.%d %s -> %s - sequential and parallel - %s", idx, cfg.srcChainID, cfg.dstChainID, txTypeString), func(t *testing.T) {
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

			idx++
		}
	}

	idx = 1

	for txType, txTypeString := range transactionTypes {
		t.Run(fmt.Sprintf("7.%d Both directions sequential - %s", idx, txTypeString), func(t *testing.T) {
			if cardanofw.ShouldSkipE2RRedundantTests() {
				t.Skip()
			}

			const (
				sendAmount = uint64(1_000_000)
				instances  = 5
			)

			brSubmitterUser, err := cardanofw.NewTestApexUserSkyline(
				apex.Config.PrimeConfig.NetworkType, false, 0, true, apex.Config.CardanoConfig.NetworkType, false)
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
			if cardanofw.ShouldSkipE2RRedundantTests() {
				t.Skip()
			}

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
			if cardanofw.ShouldSkipE2RRedundantTests() {
				t.Skip()
			}

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
			if cardanofw.ShouldSkipE2RRedundantTests() {
				t.Skip()
			}

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
