package e2e

import (
	"bytes"
	"context"
	"fmt"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/cardanofw"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/e2ehelper"
	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/Ethernal-Tech/cardano-infrastructure/sendtx"
	"github.com/Ethernal-Tech/cardano-infrastructure/wallet"
	infrawallet "github.com/Ethernal-Tech/cardano-infrastructure/wallet"
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

		bridgingFeeAmount = uint64(1_100_000)
		operationFee      = uint64(1_000_010)
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
		sendAmount := uint64(1_000_000)
		feeAmount := uint64(1_100_000)

		receivers := []sendtx.BridgingTxReceiver{
			{
				Addr:         user.GetAddress(cardanofw.ChainIDCardano),
				Amount:       sendAmount * 10,
				BridgingType: sendtx.BridgingTypeCurrencyOnSource,
			},
		}

		metadata, err := apex.GetChainMust(t, cardanofw.ChainIDPrime).CreateMetadata(
			ctx, user.GetAddress(cardanofw.ChainIDPrime), cardanofw.ChainIDCardano,
			receivers, bridgingFeeAmount, operationFee)
		require.NoError(t, err)

		txHash, err := apex.SubmitTx(
			ctx, cardanofw.ChainIDPrime, user,
			apex.PrimeInfo.MultisigAddr, new(big.Int).SetUint64(sendAmount+feeAmount+operationFee), metadata)
		require.NoError(t, err)

		cardanofw.WaitForInvalidState(t, ctx, apex, cardanofw.ChainIDPrime, txHash, apex.Config.APIKey, 0)
	})

	t.Run("2. Multiple submitters mismatch submitted and receiver amounts", func(t *testing.T) {
		for i := 0; i < 5; i++ {
			sendAmount := uint64(1_000_000)
			feeAmount := uint64(1_100_000)

			metadata, err := apex.GetChainMust(t, cardanofw.ChainIDPrime).CreateMetadata(
				ctx, apex.Users[i].GetAddress(cardanofw.ChainIDPrime), cardanofw.ChainIDCardano,
				[]sendtx.BridgingTxReceiver{
					{
						Addr:         apex.Users[i].GetAddress(cardanofw.ChainIDCardano),
						Amount:       sendAmount * 10,
						BridgingType: sendtx.BridgingTypeCurrencyOnSource,
					},
				}, bridgingFeeAmount, operationFee)
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
		feeAmount := uint64(1_100_000)

		var wg sync.WaitGroup

		for i := 0; i < instances; i++ {
			wg.Add(1)

			go func(idx int) {
				defer wg.Done()

				testUser := apex.Users[idx]

				metadata, err := apex.GetChainMust(t, cardanofw.ChainIDPrime).CreateMetadata(
					ctx, testUser.GetAddress(cardanofw.ChainIDPrime), cardanofw.ChainIDCardano,
					[]sendtx.BridgingTxReceiver{
						{
							Addr:         apex.Users[idx].GetAddress(cardanofw.ChainIDCardano),
							Amount:       sendAmount * 10,
							BridgingType: sendtx.BridgingTypeCurrencyOnSource,
						},
					}, bridgingFeeAmount, operationFee)
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
		feeAmount := uint64(1_100_000)

		receivers := []sendtx.BridgingTxReceiver{
			{
				Addr:         user.GetAddress(cardanofw.ChainIDCardano),
				Amount:       sendAmount,
				BridgingType: sendtx.BridgingTypeCurrencyOnSource,
			},
		}

		metadata, err := apex.GetChainMust(t, cardanofw.ChainIDPrime).CreateMetadata(
			ctx, user.GetAddress(cardanofw.ChainIDPrime), cardanofw.ChainIDCardano,
			receivers, bridgingFeeAmount, operationFee)
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
		feeAmount := uint64(1_100_000)

		metadata, err := apex.GetChainMust(t, cardanofw.ChainIDPrime).CreateMetadata(
			ctx, user.GetAddress(cardanofw.ChainIDPrime), cardanofw.ChainIDCardano,
			[]sendtx.BridgingTxReceiver{
				{
					Addr:         user.GetAddress(cardanofw.ChainIDCardano),
					Amount:       sendAmount,
					BridgingType: sendtx.BridgingTypeCurrencyOnSource,
				},
			}, bridgingFeeAmount, operationFee)
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
		feeAmount := uint64(1_100_000)

		metadata, err := apex.GetChainMust(t, cardanofw.ChainIDPrime).CreateMetadata(
			ctx, user.GetAddress(cardanofw.ChainIDPrime), cardanofw.ChainIDCardano,
			[]sendtx.BridgingTxReceiver{
				{
					Addr:         user.GetAddress(cardanofw.ChainIDCardano),
					Amount:       sendAmount,
					BridgingType: sendtx.BridgingTypeCurrencyOnSource,
				},
			}, bridgingFeeAmount, operationFee)
		require.NoError(t, err)

		bridgingRequestMetadata := bytes.Replace(metadata,
			[]byte(fmt.Sprintf("\"%s\"", cardanofw.ChainIDCardano)), []byte("\"hector\""), 1)

		txHash, err := apex.SubmitTx(ctx, cardanofw.ChainIDPrime, user,
			apex.PrimeInfo.MultisigAddr, new(big.Int).SetUint64(sendAmount+feeAmount+operationFee), bridgingRequestMetadata)
		require.NoError(t, err)

		cardanofw.WaitForInvalidState(t, ctx, apex, cardanofw.ChainIDPrime, txHash, apex.Config.APIKey, 0)
	})

	t.Run("7. Submitted invalid metadata - invalid sender", func(t *testing.T) {
		sendAmount := uint64(1_000_000)
		feeAmount := uint64(1_100_000)

		metadata, err := apex.GetChainMust(t, cardanofw.ChainIDPrime).CreateMetadata(
			ctx, "dummy", cardanofw.ChainIDCardano,
			[]sendtx.BridgingTxReceiver{
				{
					Addr:         user.GetAddress(cardanofw.ChainIDCardano),
					Amount:       sendAmount,
					BridgingType: sendtx.BridgingTypeCurrencyOnSource,
				},
			}, bridgingFeeAmount, operationFee)
		require.NoError(t, err)

		// remove this after we make correct validation on oracle!
		bridgingRequestMetadata := bytes.Replace(metadata,
			[]byte("[\"dummy\"]"), []byte("\"\""), 1)

		txHash, err := apex.SubmitTx(ctx, cardanofw.ChainIDPrime, user, apex.PrimeInfo.MultisigAddr,
			new(big.Int).SetUint64(sendAmount+feeAmount+operationFee), bridgingRequestMetadata)
		require.NoError(t, err)

		cardanofw.WaitForInvalidState(t, ctx, apex, cardanofw.ChainIDPrime, txHash, apex.Config.APIKey, 0)
	})

	t.Run("8. Submitted invalid metadata - invalid operationFee", func(t *testing.T) {
		sendAmount := uint64(1_000_000)
		feeAmount := uint64(1_100_000)

		metadata, err := apex.GetChainMust(t, cardanofw.ChainIDPrime).CreateMetadata(
			ctx, "dummy", cardanofw.ChainIDCardano,
			[]sendtx.BridgingTxReceiver{
				{
					Addr:         user.GetAddress(cardanofw.ChainIDCardano),
					Amount:       sendAmount,
					BridgingType: sendtx.BridgingTypeCurrencyOnSource,
				},
			}, bridgingFeeAmount, operationFee)
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
		feeAmount := uint64(1_100_000)

		brSubmitterUser, err := cardanofw.NewTestApexUser(
			apex.Config.PrimeConfig.NetworkType, false, 0, false)
		require.NoError(t, err)

		brSubmitterWallet, _ := brSubmitterUser.GetCardanoWallet(cardanofw.ChainIDPrime)

		minterWallet := apex.PrimeInfo.GenesisWallet

		tokensFunded, err := cardanofw.FundUserWithToken(
			ctx, cardanofw.ChainIDPrime, apex.Config.PrimeConfig.NetworkType, txProviderPrime,
			minterWallet, brSubmitterUser, uint64(10_000_000), uint64(1_000_000))
		require.NoError(t, err)

		metadata, err := apex.GetChainMust(t, cardanofw.ChainIDPrime).CreateMetadata(
			ctx, "dummy", cardanofw.ChainIDCardano,
			[]sendtx.BridgingTxReceiver{
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
			}, bridgingFeeAmount, operationFee)
		require.NoError(t, err)

		txHash, err := cardanofw.SendTxWithTokens(ctx, apex.Config.PrimeConfig.NetworkType, txProviderPrime,
			brSubmitterWallet, apex.PrimeInfo.MultisigAddr,
			feeAmount+operationFee, []wallet.TokenAmount{*tokensFunded}, metadata,
		)
		require.NoError(t, err)

		cardanofw.WaitForInvalidState(t, ctx, apex, cardanofw.ChainIDPrime, txHash, apex.Config.APIKey, 0)
	})

	t.Run("10. Submitted invalid metadata - empty tx", func(t *testing.T) {
		sendAmount := uint64(1_000_000)

		metadata, err := apex.GetChainMust(t, cardanofw.ChainIDPrime).CreateMetadata(
			ctx, user.GetAddress(cardanofw.ChainIDPrime), cardanofw.ChainIDCardano,
			[]sendtx.BridgingTxReceiver{}, bridgingFeeAmount, operationFee)
		require.NoError(t, err)

		txHash, err := apex.SubmitTx(ctx, cardanofw.ChainIDPrime, user, apex.PrimeInfo.MultisigAddr,
			new(big.Int).SetUint64(sendAmount), metadata)
		require.NoError(t, err)

		cardanofw.WaitForInvalidState(t, ctx, apex, cardanofw.ChainIDPrime, txHash, apex.Config.APIKey, 0)
	})

	t.Run("11. Submitted with tokens to bridging addr", func(t *testing.T) {
		sendAmount := uint64(5_000_000)
		feeAmount := uint64(1_100_000)

		minterUser := apex.Users[userCnt-1]

		brSubmitterUser, err := cardanofw.NewTestApexUser(
			apex.Config.PrimeConfig.NetworkType, false, 0, false)
		require.NoError(t, err)

		minterWallet, _ := minterUser.GetCardanoWallet(cardanofw.ChainIDPrime)

		tokensFunded, err := cardanofw.FundUserWithToken(
			ctx, cardanofw.ChainIDPrime, apex.Config.PrimeConfig.NetworkType, txProviderPrime,
			minterWallet, brSubmitterUser, uint64(10_000_000), uint64(1_000_000))
		require.NoError(t, err)

		metadata, err := apex.GetChainMust(t, cardanofw.ChainIDPrime).CreateMetadata(
			ctx, user.GetAddress(cardanofw.ChainIDPrime), cardanofw.ChainIDCardano,
			[]sendtx.BridgingTxReceiver{
				{
					Addr:         user.GetAddress(cardanofw.ChainIDCardano),
					Amount:       sendAmount - feeAmount,
					BridgingType: sendtx.BridgingTypeCurrencyOnSource,
				},
			}, bridgingFeeAmount, operationFee)
		require.NoError(t, err)

		brSubmitterWallet, _ := brSubmitterUser.GetCardanoWallet(cardanofw.ChainIDPrime)

		txHash, err := cardanofw.SendTxWithTokens(ctx, apex.Config.PrimeConfig.NetworkType, txProviderPrime,
			brSubmitterWallet, apex.PrimeInfo.MultisigAddr,
			sendAmount+operationFee, []wallet.TokenAmount{*tokensFunded}, metadata,
		)
		require.NoError(t, err)

		cardanofw.WaitForInvalidState(t, ctx, apex, cardanofw.ChainIDPrime, txHash, apiKey, 0)
	})

	t.Run("12. Submitted invalid metadata - invalid send amount - token on source", func(t *testing.T) {
		sendAmount := uint64(1_123_000)
		feeAmount := uint64(1_100_000)

		brSubmitterUser, err := cardanofw.NewTestApexUser(
			apex.Config.PrimeConfig.NetworkType, false, 0, false)
		require.NoError(t, err)

		brSubmitterWallet, _ := brSubmitterUser.GetCardanoWallet(cardanofw.ChainIDPrime)

		minterWallet := apex.PrimeInfo.GenesisWallet

		tokensFunded, err := cardanofw.FundUserWithToken(
			ctx, cardanofw.ChainIDPrime, apex.Config.PrimeConfig.NetworkType, txProviderPrime,
			minterWallet, brSubmitterUser, uint64(10_000_000), sendAmount)
		require.NoError(t, err)

		metadata, err := apex.GetChainMust(t, cardanofw.ChainIDPrime).CreateMetadata(
			ctx, "dummy", cardanofw.ChainIDCardano,
			[]sendtx.BridgingTxReceiver{
				{
					Addr:         user.GetAddress(cardanofw.ChainIDCardano),
					Amount:       sendAmount,
					BridgingType: sendtx.BridgingTypeNativeTokenOnSource,
				},
			}, bridgingFeeAmount, operationFee)
		require.NoError(t, err)

		bridgingRequestMetadata := bytes.Replace(metadata,
			[]byte("1123000"), []byte("1000000"), 1)

		txHash, err := cardanofw.SendTxWithTokens(ctx, apex.Config.PrimeConfig.NetworkType, txProviderPrime,
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

func TestE2E_SkylineUTxOConsolidation(t *testing.T) {
	if cardanofw.ShouldSkipE2RRedundantTests() {
		t.Skip()
	}

	const (
		fundUtxoCount   = 6
		maxFeeUtxoCount = 1
		maxUtxoCount    = 3
	)

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	minUtxoCurrency := cardanofw.MinUTxODefaultValue * 4
	primeConfig := cardanofw.NewPrimeChainConfig()
	primeConfig.FundUTxOCount = fundUtxoCount
	primeConfig.FundAmount = minUtxoCurrency * fundUtxoCount
	primeConfig.FundTokenAmount = cardanofw.MinUTxODefaultValue * fundUtxoCount
	primeConfig.InitialHotWalletAmount = new(big.Int).SetUint64(primeConfig.FundAmount)
	primeConfig.InitialHotWalletTokenAmount = new(big.Int).SetUint64(primeConfig.FundTokenAmount)
	sendAmountTokens := cardanofw.MinUTxODefaultValue * 3
	sendAmountCurrency := minUtxoCurrency * 3

	var (
		initialUtxos []map[string]any
		tipData      infrawallet.QueryTipData
		lock         sync.Mutex
	)

	apex := cardanofw.SetupAndRunSkylineBridge(
		t, ctx,
		cardanofw.WithUserCnt(1),
		cardanofw.WithPrimeConfig(primeConfig),
		cardanofw.WithCustomConfigHandlers(func(a *cardanofw.ApexSystem, mp map[string]any) {
			t.Helper()

			lock.Lock()
			defer lock.Unlock()

			// retrieve only once for all validators
			if len(initialUtxos) == 0 {
				initialUtxos, tipData = getInitialUtxosAndTip(
					t, ctx, a.PrimeInfo, a.PrimeInfo.MultisigAddr, a.PrimeInfo.FeeAddr)
			}

			// Prime indexer should start after multisig funding is done
			vcCfg := cardanofw.GetMapFromInterfaceKey(mp, "cardanoChains", cardanofw.ChainIDPrime)
			vcCfg["startBlockHash"] = tipData.Hash
			vcCfg["startSlot"] = tipData.Slot
			vcCfg["initialUtxos"] = initialUtxos
			vcCfg["maxFeeUtxoCount"] = maxFeeUtxoCount
			vcCfg["maxUtxoCount"] = maxUtxoCount
		}, nil),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	txRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithClient(apex.BridgeCluster.Servers[0].JSONRPC()))
	require.NoError(t, err)

	txProviderPrime, err := apex.PrimeInfo.GetTxProvider()
	require.NoError(t, err)

	_, err = cardanofw.FundUserWithToken(
		ctx, cardanofw.ChainIDPrime, apex.Config.PrimeConfig.NetworkType, txProviderPrime,
		apex.PrimeInfo.GenesisWallet, apex.Users[0], minUtxoCurrency+sendAmountCurrency, sendAmountTokens)
	require.NoError(t, err)

	getLastConfirmedBatchID := func(chainID string) uint64 {
		input, err := contractsapi.ApexBridgeContracts.SignedBatches.Abi.GetMethod("getConfirmedBatchId").
			Encode([]any{cardanofw.ChainIDToInt(chainID)})
		require.NoError(t, err)

		response, err := txRelayer.Call(types.ZeroAddress, contracts.SignedBatches, input)
		require.NoError(t, err)

		val, err := common.ParseUint64orHex(&response)
		require.NoError(t, err)

		return val
	}

	require.Equal(t, uint64(0), getLastConfirmedBatchID(cardanofw.ChainIDCardano))

	utxos, err := txProviderPrime.GetUtxos(ctx, apex.PrimeInfo.MultisigAddr)
	require.NoError(t, err)

	require.Len(t, utxos, primeConfig.FundUTxOCount)

	e2ehelper.ExecuteSingleBridging(
		t, ctx, apex, apex.Users[0], apex.Users[0],
		cardanofw.ChainIDPrime, cardanofw.ChainIDCardano,
		new(big.Int).SetUint64(sendAmountTokens),
		sendtx.BridgingTypeNativeTokenOnSource)

	require.Equal(t, uint64(2), getLastConfirmedBatchID(cardanofw.ChainIDCardano))

	utxos, err = txProviderPrime.GetUtxos(ctx, apex.PrimeInfo.MultisigAddr)
	require.NoError(t, err)

	require.Len(t, utxos, 1)
}
