package e2e

import (
	"context"
	"fmt"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/e2e-polybft/cardanofw"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/e2ehelper"
	infracommon "github.com/Ethernal-Tech/cardano-infrastructure/common"
	"github.com/Ethernal-Tech/cardano-infrastructure/sendtx"
	"github.com/Ethernal-Tech/cardano-infrastructure/wallet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestE2E_SkylineBridgeMBA_UTxOConsolidation(t *testing.T) {
	if cardanofw.ShouldSkipE2RRedundantTests() {
		t.Skip()
	}

	const (
		bridgeAddrCnt                 = 4
		fundUtxoCount                 = 13
		maxFeeUtxoCount               = 1
		maxUtxoCount                  = 5
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
	cardanoConfig.UseIndexer = true

	primeConfig := cardanofw.NewPrimeChainConfig()
	primeConfig.BridgingAddressCnt = bridgeAddrCnt
	primeConfig.FundUTxOCount = fundUtxoCount
	primeConfig.FundAmount = fundFactor * minValue * fundUtxoCount
	primeConfig.InitialHotWalletAmount = new(big.Int).SetUint64(cardanoConfig.FundAmount)
	primeConfig.InitialHotWalletTokenAmount = new(big.Int).SetUint64(cardanoConfig.FundTokenAmount)
	primeConfig.UseIndexer = true

	sendAmountTokens := minValue*sendMinValueFactor*fundFactor + sendMinValueIncrement   // when we send tokens, this amount of currency will be released from multisig address
	sendAmountCurrency := minValue*sendMinValueFactor*fundFactor + sendMinValueIncrement // when we send currency, this amount of native tokens will be released from multisig address

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
				fmt.Print("\nCARDANO: \n")

				initialUtxosCardano, tipDataCardano = getInitialUtxosAndTip(
					t, ctx, a.CardanoInfo, a.CardanoInfo.MultisigAddr, a.CardanoInfo.FeeAddr)

				fmt.Print("\nPRIME: \n")

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
			ctx, apex, cardanofw.ChainIDCardano,
			apex.CardanoInfo.GenesisWallet, sender,
			cardanofw.DefaultTokenName, cardanofw.DefaultTokenMintAmount,
			uint64(2_000_000_000), uint64(2_000_000_000))
		require.NoError(t, err)
	}

	utxos, err := infracommon.ExecuteWithRetry(
		ctx, func(ctx context.Context) ([]wallet.Utxo, error) {
			return txProviderCardano.GetUtxos(ctx, apex.CardanoInfo.MultisigAddr[0])
		},
	)
	require.NoError(t, err)

	require.Len(t, utxos, cardanoConfig.FundUTxOCount)

	primeAddrAmounts, err := apex.GetBridgingAddressesTokenAmounts(ctx, cardanofw.ChainIDPrime)
	require.NoError(t, err)
	fmt.Println("Prime multisig addresses amounts: ", primeAddrAmounts)

	cardanoAddrAmounts, err := apex.GetBridgingAddressesTokenAmounts(ctx, cardanofw.ChainIDCardano)
	require.NoError(t, err)
	fmt.Println("Cardano multisig addresses amounts: ", cardanoAddrAmounts)

	var (
		lastBatchIDs           map[string]uint64 = map[string]uint64{cardanofw.ChainIDPrime: 0, cardanofw.ChainIDCardano: 0}
		getCntConsolidationMap func() map[string]int
	)

	t.Run("with tokens", func(t *testing.T) {
		ctxChild, cncl := context.WithCancel(ctx)
		defer cncl()

		getCntConsolidationMap, lastBatchIDs = checkConsolidationBatchCounts(
			t, ctxChild,
			apex.BridgeCluster.Servers[0].JSONRPC(),
			[]string{cardanofw.ChainIDPrime},
			lastBatchIDs,
		)

		fmt.Println("lastBatchIDs", lastBatchIDs)

		e2ehelper.ExecuteSingleBridging(
			t, ctxChild, apex, apex.Users[0], apex.Users[0],
			cardanofw.ChainIDCardano, cardanofw.ChainIDPrime,
			new(big.Int).SetUint64(sendAmountTokens),
			sendtx.BridgingTypeNativeTokenOnSource)

		for _, cnt := range getCntConsolidationMap() {
			assert.GreaterOrEqual(t, cnt, minimumExpectedConsolidations)
		}
	})

	primeAddrAmounts, err = apex.GetBridgingAddressesTokenAmounts(ctx, cardanofw.ChainIDPrime)
	require.NoError(t, err)
	fmt.Println("Prime multisig addresses amounts: ", primeAddrAmounts)

	cardanoAddrAmounts, err = apex.GetBridgingAddressesTokenAmounts(ctx, cardanofw.ChainIDCardano)
	require.NoError(t, err)
	fmt.Println("Cardano multisig addresses amounts: ", cardanoAddrAmounts)

	t.Run("with currency", func(t *testing.T) {
		ctxChild, cncl := context.WithCancel(ctx)
		defer cncl()

		getCntConsolidationMap, lastBatchIDs = checkConsolidationBatchCounts(
			t, ctxChild,
			apex.BridgeCluster.Servers[0].JSONRPC(),
			[]string{cardanofw.ChainIDCardano},
			lastBatchIDs,
		)

		fmt.Println("lastBatchIDs", lastBatchIDs)

		e2ehelper.ExecuteSingleBridging(
			t, ctxChild, apex, apex.Users[0], apex.Users[0],
			cardanofw.ChainIDPrime, cardanofw.ChainIDCardano,
			new(big.Int).SetUint64(sendAmountCurrency),
			sendtx.BridgingTypeCurrencyOnSource)

		for _, cnt := range getCntConsolidationMap() {
			assert.GreaterOrEqual(t, cnt, minimumExpectedConsolidations)
		}
	})

	primeAddrAmounts, err = apex.GetBridgingAddressesTokenAmounts(ctx, cardanofw.ChainIDPrime)
	require.NoError(t, err)
	fmt.Println("Prime multisig addresses amounts: ", primeAddrAmounts)

	cardanoAddrAmounts, err = apex.GetBridgingAddressesTokenAmounts(ctx, cardanofw.ChainIDCardano)
	require.NoError(t, err)
	fmt.Println("Cardano multisig addresses amounts: ", cardanoAddrAmounts)

	t.Run("both directions", func(t *testing.T) {
		ctxChild, cncl := context.WithCancel(ctx)
		defer cncl()

		txProviderPrime, err := apex.PrimeInfo.GetTxProvider()
		require.NoError(t, err)
		txProviderCardano, err := apex.CardanoInfo.GetTxProvider()
		require.NoError(t, err)

		// when we send currency, this amount of native tokens will be released from multisig address
		sendAmountCurrency := minValue*sendMinValueFactor + sendMinValueIncrement

		getCntConsolidationMap, lastBatchIDs = checkConsolidationBatchCounts(
			t, ctxChild,
			apex.BridgeCluster.Servers[0].JSONRPC(),
			[]string{cardanofw.ChainIDCardano, cardanofw.ChainIDPrime},
			lastBatchIDs,
		)

		fmt.Println("lastBatchIDs", lastBatchIDs)

		fmt.Print("\nBEFORE: Prime chain")

		for idx, addr := range apex.PrimeInfo.MultisigAddr {
			multisigUtoxs, err := infracommon.ExecuteWithRetry(
				ctx, func(ctx context.Context) ([]wallet.Utxo, error) {
					return txProviderPrime.GetUtxos(ctx, addr)
				},
			)
			require.NoError(t, err)

			fmt.Printf("\n\tmultisig addr: %s[%d]: %v\n", addr, idx, multisigUtoxs)
		}

		fmt.Print("\nBEFORE: Cardano chain")

		for idx, addr := range apex.CardanoInfo.MultisigAddr {
			multisigUtoxs, err := infracommon.ExecuteWithRetry(
				ctx, func(ctx context.Context) ([]wallet.Utxo, error) {
					return txProviderCardano.GetUtxos(ctx, addr)
				},
			)
			require.NoError(t, err)

			fmt.Printf("\n\tmultisig addr: %s[%d]: %v\n", addr, idx, multisigUtoxs)
		}

		e2ehelper.ExecuteBridging(
			t, ctxChild, apex, sequentialInstances,
			apex.Users[:parallelInstances],
			[]*cardanofw.TestApexUser{apex.Users[parallelInstances]},
			[]string{cardanofw.ChainIDCardano, cardanofw.ChainIDPrime},
			map[string][]string{
				cardanofw.ChainIDCardano: {cardanofw.ChainIDPrime},
				cardanofw.ChainIDPrime:   {cardanofw.ChainIDCardano},
			},
			map[e2ehelper.SrcDstChainPair]sendtx.BridgingType{
				e2ehelper.NewChainPair(cardanofw.ChainIDCardano, cardanofw.ChainIDPrime): sendtx.BridgingTypeNativeTokenOnSource,
				e2ehelper.NewChainPair(cardanofw.ChainIDPrime, cardanofw.ChainIDCardano): sendtx.BridgingTypeCurrencyOnSource,
			},
			new(big.Int).SetUint64(sendAmountCurrency),
			e2ehelper.WithWaitForUnexpectedBridges(true),
		)

		for _, cnt := range getCntConsolidationMap() {
			assert.GreaterOrEqual(t, cnt, minimumExpectedConsolidations)
		}

		primeAddrAmounts, err = apex.GetBridgingAddressesTokenAmounts(ctx, cardanofw.ChainIDPrime)
		require.NoError(t, err)
		fmt.Println("Prime multisig addresses amounts: ", primeAddrAmounts)

		cardanoAddrAmounts, err = apex.GetBridgingAddressesTokenAmounts(ctx, cardanofw.ChainIDCardano)
		require.NoError(t, err)
		fmt.Println("Cardano multisig addresses amounts: ", cardanoAddrAmounts)
	})

	t.Run("with redistribution", func(t *testing.T) {
		ctxChild, cncl := context.WithCancel(ctx)
		defer cncl()

		getCntConsolidationMap, lastBatchIDs = checkConsolidationBatchCounts(
			t, ctxChild,
			apex.BridgeCluster.Servers[0].JSONRPC(),
			[]string{cardanofw.ChainIDCardano},
			lastBatchIDs,
		)

		fmt.Println("lastBatchIDs", lastBatchIDs)

		e2ehelper.ExecuteTokenRedistribution(t, ctx, apex, cardanofw.ChainIDCardano, 50, 5*time.Minute)

		for _, cnt := range getCntConsolidationMap() {
			assert.GreaterOrEqual(t, cnt, minimumExpectedConsolidations)
		}
	})

	primeAddrAmounts, err = apex.GetBridgingAddressesTokenAmounts(ctx, cardanofw.ChainIDPrime)
	require.NoError(t, err)
	fmt.Println("Prime multisig addresses amounts: ", primeAddrAmounts)

	cardanoAddrAmounts, err = apex.GetBridgingAddressesTokenAmounts(ctx, cardanofw.ChainIDCardano)
	require.NoError(t, err)
	fmt.Println("Cardano multisig addresses amounts: ", cardanoAddrAmounts)
}

// go test -timeout 0 -run ^TestE2E_SkylineBridgeMBA_StakeAddressOperationsTest$ github.com/0xPolygon/polygon-edge/e2e-polybft/e2e -v
func TestE2E_SkylineBridgeMBA_StakeAddressOperationsTest(t *testing.T) {
	const apiKey = "test_api_key"

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	primeConfig, cardanoConfig := cardanofw.NewPrimeChainConfig(), cardanofw.NewCardanoChainConfig(true)
	cardanoConfig.FundTokenAmount = 1_000_000_000

	apex := cardanofw.SetupAndRunSkylineBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithCardanoConfig(cardanoConfig),
		cardanofw.WithPrimeConfig(primeConfig),
		cardanofw.WithBridgingAddrCnt(cardanofw.ChainIDPrime, bridgeAddrCnt),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	_, err := cardanofw.FundUserWithToken(
		ctx, apex, cardanofw.ChainIDCardano,
		apex.GetCardanoInfo(cardanofw.ChainIDCardano).GenesisWallet, apex.Users[0],
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

		bridgingTypes := map[e2ehelper.SrcDstChainPair]sendtx.BridgingType{
			e2ehelper.NewChainPair(cardanofw.ChainIDPrime, cardanofw.ChainIDCardano): sendtx.BridgingTypeCurrencyOnSource,
			e2ehelper.NewChainPair(cardanofw.ChainIDCardano, cardanofw.ChainIDPrime): sendtx.BridgingTypeNativeTokenOnSource,
		}

		for i := range len(bridgingTypes) {
			go func(idx int) {
				defer wg.Done()
				e2ehelper.ExecuteSingleBridging(
					t, ctx, apex, senders[idx], receivers[idx], srcChainID, dstChainID, sendAmountDfm, bridgingTypes[e2ehelper.NewChainPair(srcChainID, dstChainID)])
			}(i)
		}

		wg.Wait()
	}

	executeBridging(cardanofw.ChainIDPrime, cardanofw.ChainIDCardano, sendAmountDfm,
		[]*cardanofw.TestApexUser{apex.Users[0], apex.Users[1]}, []*cardanofw.TestApexUser{apex.Users[2], apex.Users[3]})

	// 1. Check existing stake pools in the system
	stakePools := apex.GetChainMust(t, cardanofw.ChainIDPrime).GetExistingStakePools(t, ctx)
	require.NotEmpty(t, stakePools)

	t.Run("redeleg before reg and del should fail", func(t *testing.T) {
		err = apex.DelegateStakeAddress(ctx, cardanofw.ChainIDPrime, 0, stakePools[1], false)
		require.Error(t, err)
	})

	t.Run("reg and del should pass", func(t *testing.T) {
		err = apex.DelegateStakeAddress(ctx, cardanofw.ChainIDPrime, 0, stakePools[0], true)
		require.NoError(t, err)

		addrInfo, err := apex.GetChainMust(t, cardanofw.ChainIDPrime).GetBridgingStakeAddressInfo(t, ctx, 0, false)
		require.NoError(t, err)
		require.Equal(t, stakePools[0], addrInfo.StakeDelegation)

		executeBridging(cardanofw.ChainIDCardano, cardanofw.ChainIDPrime, sendAmountDfm,
			[]*cardanofw.TestApexUser{apex.Users[0], apex.Users[1]}, []*cardanofw.TestApexUser{apex.Users[2], apex.Users[3]})
	})

	t.Run("reg and del again should fail", func(t *testing.T) {
		// Registering already registered address should fail:
		err = apex.DelegateStakeAddress(ctx, cardanofw.ChainIDPrime, 0, stakePools[0], true)
		require.Error(t, err)
	})

	t.Run("redeleg should pass", func(t *testing.T) {
		err = apex.DelegateStakeAddress(ctx, cardanofw.ChainIDPrime, 0, stakePools[1], false)
		require.NoError(t, err)

		previousStakePool := stakePools[0]

		for range 60 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
			}

			addrInfo, err := apex.GetChainMust(t, cardanofw.ChainIDPrime).GetBridgingStakeAddressInfo(t, ctx, 0, false)
			require.NoError(t, err)

			if addrInfo.StakeDelegation != previousStakePool {
				require.Equal(t, stakePools[1], addrInfo.StakeDelegation)

				break
			}
		}

		executeBridging(cardanofw.ChainIDCardano, cardanofw.ChainIDPrime, sendAmountDfm,
			[]*cardanofw.TestApexUser{apex.Users[0], apex.Users[1]}, []*cardanofw.TestApexUser{apex.Users[2], apex.Users[3]})
	})

	t.Run("dereg should pass", func(t *testing.T) {
		err = apex.DeregisterStakeAddress(ctx, cardanofw.ChainIDPrime, 0)
		require.NoError(t, err)

		for range 60 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
			}

			addrInfo, err := apex.GetChainMust(t, cardanofw.ChainIDPrime).GetBridgingStakeAddressInfo(t, ctx, 0, true)

			if err != nil {
				require.ErrorContains(t, err, "stake address is not registered yet")
				require.Error(t, err)
				require.Equal(t, addrInfo, wallet.QueryStakeAddressInfo{})

				break
			}
		}

		executeBridging(cardanofw.ChainIDCardano, cardanofw.ChainIDPrime, sendAmountDfm,
			[]*cardanofw.TestApexUser{apex.Users[0], apex.Users[1]}, []*cardanofw.TestApexUser{apex.Users[2], apex.Users[3]})
	})

	t.Run("simultaneous test", func(t *testing.T) {
		executeBridging := func(
			srcChainID, dstChainID cardanofw.ChainID, sendAmountDfm *big.Int,
			senders, receivers []*cardanofw.TestApexUser, doRegDeleg bool,
		) {
			bridgingTypes := map[e2ehelper.SrcDstChainPair]sendtx.BridgingType{
				e2ehelper.NewChainPair(cardanofw.ChainIDPrime, cardanofw.ChainIDCardano): sendtx.BridgingTypeCurrencyOnSource,
				e2ehelper.NewChainPair(cardanofw.ChainIDCardano, cardanofw.ChainIDPrime): sendtx.BridgingTypeNativeTokenOnSource,
			}

			wg := sync.WaitGroup{}
			wg.Add(len(bridgingTypes) + 1)

			for i := range len(bridgingTypes) {
				go func(idx int) {
					defer wg.Done()
					e2ehelper.ExecuteSingleBridging(
						t, ctx, apex, senders[idx], receivers[idx], srcChainID, dstChainID, sendAmountDfm, bridgingTypes[e2ehelper.NewChainPair(srcChainID, dstChainID)])
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
				err = apex.DelegateStakeAddress(ctx, cardanofw.ChainIDPrime, 0, stakePools[0], true)
				require.NoError(t, err)

				// 3. Check if the registration and delegation was successful
				addrInfo, err := apex.GetChainMust(t, cardanofw.ChainIDPrime).GetBridgingStakeAddressInfo(t, ctx, 0, false)
				require.NoError(t, err)
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
	})
}

// go test -timeout 0 -run ^TestE2E_SkylineBridgeMBA_MutltipleAddresses$ github.com/0xPolygon/polygon-edge/e2e-polybft/e2e -v
func TestE2E_SkylineBridgeMBA_MutltipleAddresses(t *testing.T) {
	const apiKey = "test_api_key"

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	// Combined configuration for both currency and native token tests
	primeConfig, cardanoConfig := cardanofw.NewPrimeChainConfig(), cardanofw.NewCardanoChainConfig(true)
	primeConfig.FundAmount = 0
	primeConfig.FundTokenAmount = 0
	cardanoConfig.FundTokenAmount = 1_000_000_000
	primeConfig.BridgingAddressCnt = bridgeAddrCnt
	bridgingAmount := big.NewInt(1_000_000)

	apex := cardanofw.SetupAndRunSkylineBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithCardanoConfig(cardanoConfig),
		cardanofw.WithPrimeConfig(primeConfig),
		cardanofw.WithBridgingAddrCnt(cardanofw.ChainIDPrime, bridgeAddrCnt),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	fmt.Println("multisig addresses: ", apex.PrimeInfo.MultisigAddr)

	// === CURRENCY BRIDGING TESTS ===
	t.Run("Currency Bridging - Setup and Initial Funding", func(t *testing.T) {
		// Prepare for currency tests
		_, err := cardanofw.FundUserWithToken(
			ctx, apex, cardanofw.ChainIDCardano,
			apex.GetCardanoInfo(cardanofw.ChainIDCardano).GenesisWallet, apex.Users[1],
			cardanofw.DefaultTokenName, cardanofw.DefaultTokenMintAmount,
			uint64(2_000_000), uint64(100_000_000))
		require.NoError(t, err)

		// Initial funding for currency tests
		for range bridgeAddrCnt {
			e2ehelper.ExecuteSingleBridging(
				t, ctx, apex, apex.Users[0], apex.Users[1],
				cardanofw.ChainIDPrime, cardanofw.ChainIDCardano,
				big.NewInt(0).Mul(bridgingAmount, big.NewInt(5)), sendtx.BridgingTypeCurrencyOnSource)
		}

		addrAmounts, err := apex.GetBridgingAddressesTokenAmounts(ctx, cardanofw.ChainIDPrime)
		require.NoError(t, err)
		fmt.Println("Currency tests - Initial multisig addresses amounts: ", addrAmounts)
	})

	t.Run("Currency Bridging - Bridge partial amount from single address", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, apex.Users[1], apex.Users[0],
			cardanofw.ChainIDCardano, cardanofw.ChainIDPrime,
			big.NewInt(0).Mul(bridgingAmount, big.NewInt(1)), sendtx.BridgingTypeNativeTokenOnSource)

		addrAmounts, err := apex.GetBridgingAddressesTokenAmounts(ctx, cardanofw.ChainIDPrime)
		require.NoError(t, err)
		fmt.Println("Currency tests - Multisig addresses amounts: ", addrAmounts)

		require.Equal(t, uint64(4_000_010), addrAmounts[0][wallet.AdaTokenName].Uint64())
	})

	t.Run("Currency Bridging - Bridge full amount from single address", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, apex.Users[1], apex.Users[0],
			cardanofw.ChainIDCardano, cardanofw.ChainIDPrime,
			big.NewInt(5_000_010), sendtx.BridgingTypeNativeTokenOnSource)

		addrAmounts, err := apex.GetBridgingAddressesTokenAmounts(ctx, cardanofw.ChainIDPrime)
		require.NoError(t, err)
		fmt.Println("Currency tests - Multisig addresses amounts: ", addrAmounts)

		require.Equal(t, uint64(0), addrAmounts[1][wallet.AdaTokenName].Uint64())
	})

	var (
		lastBatchIDs           map[string]uint64 = map[string]uint64{cardanofw.ChainIDPrime: 0, cardanofw.ChainIDCardano: 0}
		getCntConsolidationMap func() map[string]int
	)

	const expectedConsolidations = 1

	t.Run("Currency Bridging - Insufficient change", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, apex.Users[1], apex.Users[0],
			cardanofw.ChainIDCardano, cardanofw.ChainIDPrime,
			big.NewInt(5_000_000), sendtx.BridgingTypeNativeTokenOnSource)

		addrAmounts, err := apex.GetBridgingAddressesTokenAmounts(ctx, cardanofw.ChainIDPrime)
		require.NoError(t, err)
		fmt.Println("Currency tests - Multisig addresses amounts: ", addrAmounts)

		require.Equal(t, uint64(1_000_000), addrAmounts[2][wallet.AdaTokenName].Uint64())
		require.Equal(t, uint64(5_000_020), addrAmounts[3][wallet.AdaTokenName].Uint64())
	})

	t.Run("Currency Bridging - Bridge full amount from 2 addresses", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, apex.Users[1], apex.Users[0],
			cardanofw.ChainIDCardano, cardanofw.ChainIDPrime,
			big.NewInt(9_000_030), sendtx.BridgingTypeNativeTokenOnSource)

		addrAmounts, err := apex.GetBridgingAddressesTokenAmounts(ctx, cardanofw.ChainIDPrime)
		require.NoError(t, err)
		fmt.Println("Currency tests - Multisig addresses amounts: ", addrAmounts)

		require.Equal(t, uint64(0), addrAmounts[0][wallet.AdaTokenName].Uint64())
		require.Equal(t, uint64(0), addrAmounts[3][wallet.AdaTokenName].Uint64())
	})

	t.Run("Currency Bridging - Replenish for remaining tests", func(t *testing.T) {
		// replenish
		for range bridgeAddrCnt {
			e2ehelper.ExecuteSingleBridging(
				t, ctx, apex, apex.Users[0], apex.Users[1],
				cardanofw.ChainIDPrime, cardanofw.ChainIDCardano,
				big.NewInt(0).Mul(bridgingAmount, big.NewInt(5)), sendtx.BridgingTypeCurrencyOnSource)
		}

		addrAmounts, err := apex.GetBridgingAddressesTokenAmounts(ctx, cardanofw.ChainIDPrime)
		require.NoError(t, err)
		fmt.Println("Currency tests - Multisig addresses amounts: ", addrAmounts)
	})

	t.Run("Currency Bridging - Insufficient change + full", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, apex.Users[1], apex.Users[0],
			cardanofw.ChainIDCardano, cardanofw.ChainIDPrime,
			big.NewInt(11_000_000), sendtx.BridgingTypeNativeTokenOnSource)

		addrAmounts, err := apex.GetBridgingAddressesTokenAmounts(ctx, cardanofw.ChainIDPrime)
		require.NoError(t, err)
		fmt.Println("Currency tests - Multisig addresses amounts: ", addrAmounts)

		require.Equal(t, uint64(0), addrAmounts[0][wallet.AdaTokenName].Uint64())
		require.Equal(t, uint64(1_000_000), addrAmounts[1][wallet.AdaTokenName].Uint64())
		require.Equal(t, uint64(5_000_030), addrAmounts[2][wallet.AdaTokenName].Uint64())
	})

	t.Run("Currency Bridging - Replenish for final tests", func(t *testing.T) {
		// replenish
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, apex.Users[0], apex.Users[1],
			cardanofw.ChainIDPrime, cardanofw.ChainIDCardano,
			big.NewInt(0).Mul(bridgingAmount, big.NewInt(5)), sendtx.BridgingTypeCurrencyOnSource)
	})

	t.Run("Currency Bridging - Bridge full amount from 2 + partial from 1", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, apex.Users[1], apex.Users[0],
			cardanofw.ChainIDCardano, cardanofw.ChainIDPrime,
			big.NewInt(6000010+6000010+1000030), sendtx.BridgingTypeNativeTokenOnSource)

		addrAmounts, err := apex.GetBridgingAddressesTokenAmounts(ctx, cardanofw.ChainIDPrime)
		require.NoError(t, err)
		fmt.Println("Currency tests - Multisig addresses amounts: ", addrAmounts)

		require.Equal(t, uint64(0), addrAmounts[0][wallet.AdaTokenName].Uint64())
		require.Equal(t, uint64(1_000_000), addrAmounts[1][wallet.AdaTokenName].Uint64())
		require.Equal(t, uint64(3_000_000), addrAmounts[2][wallet.AdaTokenName].Uint64())
	})

	t.Run("Currency Bridging - Final replenish", func(t *testing.T) {
		for range bridgeAddrCnt - 1 {
			e2ehelper.ExecuteSingleBridging(
				t, ctx, apex, apex.Users[0], apex.Users[1],
				cardanofw.ChainIDPrime, cardanofw.ChainIDCardano,
				big.NewInt(0).Mul(bridgingAmount, big.NewInt(5)), sendtx.BridgingTypeCurrencyOnSource)
		}

		addrAmounts, err := apex.GetBridgingAddressesTokenAmounts(ctx, cardanofw.ChainIDPrime)
		require.NoError(t, err)
		fmt.Println("Currency tests - Multisig addresses amounts: ", addrAmounts)
	})

	t.Run("Currency Bridging - Bridge full amount from 2 (addr0) + partial from 1", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, apex.Users[1], apex.Users[0],
			cardanofw.ChainIDCardano, cardanofw.ChainIDPrime,
			big.NewInt(17_000_000), sendtx.BridgingTypeNativeTokenOnSource)

		addrAmounts, err := apex.GetBridgingAddressesTokenAmounts(ctx, cardanofw.ChainIDPrime)
		require.NoError(t, err)
		fmt.Println("Currency tests - Multisig addresses amounts: ", addrAmounts)

		require.Equal(t, uint64(0), addrAmounts[0][wallet.AdaTokenName].Uint64())
		require.Equal(t, uint64(0), addrAmounts[1][wallet.AdaTokenName].Uint64())
		require.Equal(t, uint64(3000000), addrAmounts[2][wallet.AdaTokenName].Uint64())
		require.Equal(t, uint64(1000030), addrAmounts[3][wallet.AdaTokenName].Uint64())
	})

	t.Run("Currency Bridging - Test carry over consolidation, 0 on addr 0", func(t *testing.T) {
		ctxChild, cncl := context.WithCancel(ctx)
		defer cncl()

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, apex.Users[1], apex.Users[0],
			cardanofw.ChainIDCardano, cardanofw.ChainIDPrime,
			big.NewInt(1999900), sendtx.BridgingTypeNativeTokenOnSource)

		getCntConsolidationMap, lastBatchIDs = checkConsolidationBatchCounts(
			t, ctxChild,
			apex.BridgeCluster.Servers[0].JSONRPC(),
			[]string{cardanofw.ChainIDPrime},
			lastBatchIDs,
		)

		for _, cnt := range getCntConsolidationMap() {
			assert.Equal(t, cnt, expectedConsolidations)
		}

		addrAmounts, err := apex.GetBridgingAddressesTokenAmounts(ctx, cardanofw.ChainIDPrime)
		require.NoError(t, err)
		fmt.Println("Currency tests - Multisig addresses amounts: ", addrAmounts)

		require.Equal(t, uint64(1000130), addrAmounts[0][wallet.AdaTokenName].Uint64())
		require.Equal(t, uint64(0), addrAmounts[1][wallet.AdaTokenName].Uint64())
		require.Equal(t, uint64(0), addrAmounts[2][wallet.AdaTokenName].Uint64())
		require.Equal(t, uint64(0), addrAmounts[3][wallet.AdaTokenName].Uint64())
	})
}

func TestE2E_SkylineBridgeMBA_MutltipleAddresses_Native(t *testing.T) {
	const apiKey = "test_api_key"

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	// Combined configuration for both currency and native token tests
	primeConfig, cardanoConfig, vectorConfig := cardanofw.NewPrimeChainConfig(), cardanofw.NewCardanoChainConfig(true), cardanofw.NewVectorChainConfig()
	primeConfig.FundAmount = 1_000_000_000
	primeConfig.FundTokenAmount = 1_000_000_000
	cardanoConfig.FundTokenAmount = 10_000_000
	cardanoConfig.FundAmount = 6_000_000
	vectorConfig.FundAmount = 1_000_000_000
	vectorConfig.FundTokenAmount = 1_000_000_000
	cardanoConfig.BridgingAddressCnt = bridgeAddrCnt

	apex := cardanofw.SetupAndRunSkylineBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithCardanoConfig(cardanoConfig),
		cardanofw.WithPrimeConfig(primeConfig),
		cardanofw.WithVectorConfig(vectorConfig),
		cardanofw.WithBridgingAddrCnt(cardanofw.ChainIDCardano, bridgeAddrCnt),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	fmt.Println("multisig addresses cardano: ", apex.CardanoInfo.MultisigAddr)

	var tokenName string

	t.Run("Native Token Bridging - Setup and Initial Funding", func(t *testing.T) {
		cardanoToken, err := cardanofw.FundUserWithToken(
			ctx, apex, cardanofw.ChainIDCardano,
			apex.GetCardanoInfo(cardanofw.ChainIDCardano).GenesisWallet, apex.Users[0],
			cardanofw.DefaultTokenName, cardanofw.DefaultTokenMintAmount,
			uint64(2_000_000), uint64(100_000_000))
		require.NoError(t, err)

		tokenName = cardanoToken.TokenName()

		// Fund users for native token tests on Vector chain
		_, err = cardanofw.FundUserWithToken(
			ctx, apex, cardanofw.ChainIDVector,
			apex.GetCardanoInfo(cardanofw.ChainIDVector).GenesisWallet, apex.Users[1],
			cardanofw.DefaultTokenName, cardanofw.DefaultTokenMintAmount,
			uint64(2_000_000), uint64(100_000_000))
		require.NoError(t, err)

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, apex.Users[4], apex.Users[4],
			cardanofw.ChainIDCardano, cardanofw.ChainIDVector,
			big.NewInt(9_000_000), sendtx.BridgingTypeCurrencyOnSource)

		addrAmounts, err := apex.GetBridgingAddressesTokenAmounts(ctx, cardanofw.ChainIDCardano)
		require.NoError(t, err)
		fmt.Println("Native token tests - Initial multisig addresses amounts: ", addrAmounts)
	})

	// Helper function for parallel bridging vector -> cardano and prime -> cardano
	executeBridging := func(
		sendAmounts []*big.Int,
		senders, receivers []*cardanofw.TestApexUser,
		bridgingTypes []sendtx.BridgingType,
	) {
		wg := sync.WaitGroup{}
		wg.Add(len(bridgingTypes))

		for i := range bridgingTypes {
			go func(idx int) {
				defer wg.Done()

				var srcChainID, dstChainID string

				if sendAmounts[idx].Uint64() != 0 {
					if bridgingTypes[idx] == sendtx.BridgingTypeNativeTokenOnSource {
						srcChainID, dstChainID = cardanofw.ChainIDVector, cardanofw.ChainIDCardano
					} else {
						srcChainID, dstChainID = cardanofw.ChainIDPrime, cardanofw.ChainIDCardano
					}

					e2ehelper.ExecuteSingleBridging(
						t, ctx, apex, senders[idx], receivers[idx], srcChainID, dstChainID, sendAmounts[idx], bridgingTypes[idx])
				}
			}(i)
		}

		wg.Wait()
	}

	t.Run("Native Token Bridging - Bridge partial amount native and currency addr0 second", func(t *testing.T) {
		sendAmountToken := big.NewInt(1_000_000)
		sendAmountNative := big.NewInt(1_000_000)
		sendAmounts := []*big.Int{sendAmountNative, sendAmountToken}
		bridgingTypes := []sendtx.BridgingType{
			sendtx.BridgingTypeCurrencyOnSource,
			sendtx.BridgingTypeNativeTokenOnSource,
		}

		executeBridging(sendAmounts,
			[]*cardanofw.TestApexUser{apex.Users[0], apex.Users[1]},
			[]*cardanofw.TestApexUser{apex.Users[2], apex.Users[3]},
			bridgingTypes,
		)

		addrAmounts, err := apex.GetBridgingAddressesTokenAmounts(ctx, cardanofw.ChainIDCardano)
		require.NoError(t, err)
		fmt.Println("Native token tests - Multisig addresses amounts: ", addrAmounts)

		require.Equal(t, uint64(6000000), addrAmounts[0][wallet.AdaTokenName].Uint64())
		require.Equal(t, uint64(9000000), addrAmounts[0][tokenName].Uint64())
		require.Equal(t, uint64(5961300), addrAmounts[1][wallet.AdaTokenName].Uint64())
	})

	t.Run("Native Token Bridging - Bridge partial amount native and currency from 2 addrs", func(t *testing.T) {
		sendAmountToken := big.NewInt(3_000_000)
		sendAmountNative := big.NewInt(1_000_000)
		sendAmounts := []*big.Int{sendAmountNative, sendAmountToken}
		bridgingTypes := []sendtx.BridgingType{
			sendtx.BridgingTypeCurrencyOnSource,
			sendtx.BridgingTypeNativeTokenOnSource,
		}

		executeBridging(sendAmounts,
			[]*cardanofw.TestApexUser{apex.Users[0], apex.Users[1]},
			[]*cardanofw.TestApexUser{apex.Users[2], apex.Users[3]},
			bridgingTypes,
		)

		addrAmounts, err := apex.GetBridgingAddressesTokenAmounts(ctx, cardanofw.ChainIDCardano)
		require.NoError(t, err)
		fmt.Println("Native token tests - Multisig addresses amounts: ", addrAmounts)

		// This is due the fact that sometimes the tx's are processed in the same batch
		// and sometimes they are processed in different batches
		if addrAmounts[0][wallet.AdaTokenName].Uint64() == 3961290 {
			require.Equal(t, uint64(3961290), addrAmounts[0][wallet.AdaTokenName].Uint64())
			require.Equal(t, uint64(8000000), addrAmounts[0][tokenName].Uint64())
			require.Equal(t, uint64(1961300), addrAmounts[1][wallet.AdaTokenName].Uint64())
		} else {
			require.Equal(t, uint64(1038710), addrAmounts[0][wallet.AdaTokenName].Uint64())
			require.Equal(t, uint64(8000000), addrAmounts[0][tokenName].Uint64())
			require.Equal(t, uint64(4883880), addrAmounts[1][wallet.AdaTokenName].Uint64())
		}
	})

	t.Run("Native Token Bridging - Send all native and curr tokens", func(t *testing.T) {
		sendAmountToken := big.NewInt(2883880)
		sendAmountNative := big.NewInt(8_000_000)
		sendAmounts := []*big.Int{sendAmountNative, sendAmountToken}
		bridgingTypes := []sendtx.BridgingType{
			sendtx.BridgingTypeCurrencyOnSource,
			sendtx.BridgingTypeNativeTokenOnSource,
		}

		executeBridging(sendAmounts,
			[]*cardanofw.TestApexUser{apex.Users[0], apex.Users[1]},
			[]*cardanofw.TestApexUser{apex.Users[2], apex.Users[3]},
			bridgingTypes,
		)

		addrAmounts, err := apex.GetBridgingAddressesTokenAmounts(ctx, cardanofw.ChainIDCardano)
		require.NoError(t, err)
		fmt.Println("Native token tests - Multisig addresses amounts: ", addrAmounts)

		require.Equal(t, uint64(0), addrAmounts[0][wallet.AdaTokenName].Uint64())
		require.Equal(t, int(1), len(addrAmounts[0]))
		require.Equal(t, uint64(0), addrAmounts[1][wallet.AdaTokenName].Uint64())
	})
}

// go test -timeout 0 -run ^TestE2E_SkylineBridgeMBA_RedistributeTokens$ github.com/0xPolygon/polygon-edge/e2e-polybft/e2e -v
func TestE2E_SkylineBridgeMBA_RedistributeTokens(t *testing.T) {
	const (
		apiKey = "test_api_key"
	)

	bridgeAddCnt := 3
	bridgingAmount := big.NewInt(10_000_002)

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	primeConfig, cardanoConfig := cardanofw.NewPrimeChainConfig(), cardanofw.NewCardanoChainConfig(true)
	primeConfig.BridgingAddressCnt = bridgeAddCnt
	cardanoConfig.FundTokenAmount = 1_000_000_000

	apex := cardanofw.SetupAndRunSkylineBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithCardanoConfig(cardanoConfig),
		cardanofw.WithPrimeConfig(primeConfig),
		cardanofw.WithBridgingAddrCnt(cardanofw.ChainIDPrime, bridgeAddCnt),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	e2ehelper.ExecuteSingleBridging(
		t, ctx, apex, apex.Users[0], apex.Users[1],
		cardanofw.ChainIDPrime, cardanofw.ChainIDCardano,
		bridgingAmount, sendtx.BridgingTypeCurrencyOnSource)

	addrAmounts, err := apex.GetBridgingAddressesTokenAmounts(ctx, cardanofw.ChainIDPrime)
	require.NoError(t, err)
	fmt.Println("Multisig addresses amounts after the initial bridging: ", addrAmounts)

	e2ehelper.ExecuteTokenRedistribution(t, ctx, apex, cardanofw.ChainIDPrime, 30, 2*time.Minute)

	addrAmounts, err = apex.GetBridgingAddressesTokenAmounts(ctx, cardanofw.ChainIDPrime)
	require.NoError(t, err)
	fmt.Println("Multisig addresses amounts after redistribution: ", addrAmounts)

	e2ehelper.ExecuteSingleBridging(
		t, ctx, apex, apex.Users[1], apex.Users[0],
		cardanofw.ChainIDCardano, cardanofw.ChainIDPrime,
		bridgingAmount, sendtx.BridgingTypeNativeTokenOnSource)

	addrAmounts, err = apex.GetBridgingAddressesTokenAmounts(ctx, cardanofw.ChainIDPrime)
	require.NoError(t, err)
	fmt.Println("Multisig addresses amounts: ", addrAmounts)

	require.Equal(t, bridgeAddCnt, len(addrAmounts))
	require.False(t, e2ehelper.IsDiffGreaterThanOne(addrAmounts[1][wallet.AdaTokenName], addrAmounts[2][wallet.AdaTokenName]))
	require.True(t, addrAmounts[0][wallet.AdaTokenName].Cmp(addrAmounts[1][wallet.AdaTokenName]) < 0)

	t.Run("simultaneous test", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, apex.Users[0], apex.Users[1],
			cardanofw.ChainIDPrime, cardanofw.ChainIDCardano,
			bridgingAmount, sendtx.BridgingTypeCurrencyOnSource)

		addrAmounts, err := apex.GetBridgingAddressesTokenAmounts(ctx, cardanofw.ChainIDPrime)
		require.NoError(t, err)
		fmt.Println("Multisig addresses amounts after the initial bridging: ", addrAmounts)

		wg := sync.WaitGroup{}
		wg.Add(2)

		go func() {
			defer wg.Done()

			for range 3 {
				err = apex.RedistributeTokens(ctx, cardanofw.ChainIDPrime)
				require.NoError(t, err)
				time.Sleep(500 * time.Millisecond)
			}
		}()

		go func() {
			defer wg.Done()

			e2ehelper.ExecuteSingleBridging(
				t, ctx, apex, apex.Users[1], apex.Users[0],
				cardanofw.ChainIDCardano, cardanofw.ChainIDPrime,
				bridgingAmount, sendtx.BridgingTypeNativeTokenOnSource)
		}()

		wg.Wait()

		addrAmounts, err = apex.GetBridgingAddressesTokenAmounts(ctx, cardanofw.ChainIDPrime)
		require.NoError(t, err)
		fmt.Println("Multisig addresses amounts: ", addrAmounts)

		require.Equal(t, bridgeAddCnt, len(addrAmounts))
		require.False(t, e2ehelper.IsDiffGreaterThanOne(addrAmounts[1][wallet.AdaTokenName], addrAmounts[2][wallet.AdaTokenName]))
	})
}
