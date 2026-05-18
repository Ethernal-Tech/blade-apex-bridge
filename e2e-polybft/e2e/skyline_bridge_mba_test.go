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
	"github.com/Ethernal-Tech/cardano-infrastructure/wallet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	oldMinBridgingFee = 1_000_010
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
	cardanoConfig.DefaultMinBridgingFee = oldMinBridgingFee
	cardanoConfig.MinBridgingFeeForTokens = oldMinBridgingFee
	cardanoConfig.UseIndexer = true

	primeConfig := cardanofw.NewPrimeChainConfig()
	primeConfig.BridgingAddressCnt = bridgeAddrCnt
	primeConfig.FundUTxOCount = fundUtxoCount
	primeConfig.FundAmount = fundFactor * minValue * fundUtxoCount
	primeConfig.InitialHotWalletAmount = new(big.Int).SetUint64(cardanoConfig.FundAmount)
	primeConfig.InitialHotWalletTokenAmount = new(big.Int).SetUint64(cardanoConfig.FundTokenAmount)
	primeConfig.DefaultMinBridgingFee = oldMinBridgingFee
	primeConfig.MinBridgingFeeForTokens = oldMinBridgingFee
	primeConfig.UseIndexer = true

	sendAmountTokens := cardanofw.DfmToWei(new(big.Int).SetUint64(minValue*sendMinValueFactor*fundFactor + sendMinValueIncrement))   // when we send tokens, this amount of currency will be released from multisig address
	sendAmountCurrency := cardanofw.DfmToWei(new(big.Int).SetUint64(minValue*sendMinValueFactor*fundFactor + sendMinValueIncrement)) // when we send currency, this amount of native tokens will be released from multisig address

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
		}, nil, nil, nil),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	txProviderCardano, err := apex.CardanoInfo.GetTxProvider()
	require.NoError(t, err)

	fundTestUsersWithToken(
		t, ctx, apex, []*testConfig{
			{
				srcChainID:      cardanofw.ChainIDCardano,
				srcMinterWallet: apex.CardanoInfo.GenesisWallet,
			},
		}, apex.Users[:parallelInstances], cardanofw.ApexToWei(big.NewInt(2_000)), cardanofw.ApexToWei(big.NewInt(2_000)))

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
			sendAmountTokens,
			cardanofw.CAP3XTokenID, true)

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
			sendAmountCurrency,
			cardanofw.AP3XTokenID, true)

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
		sendAmountCurrency := cardanofw.DfmToWei(new(big.Int).SetUint64(minValue*sendMinValueFactor + sendMinValueIncrement))

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
			map[e2ehelper.SrcDstChainPair]uint16{
				e2ehelper.NewChainPair(cardanofw.ChainIDCardano, cardanofw.ChainIDPrime): cardanofw.CAP3XTokenID,
				e2ehelper.NewChainPair(cardanofw.ChainIDPrime, cardanofw.ChainIDCardano): cardanofw.AP3XTokenID,
			},
			sendAmountCurrency,
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
	cardanoConfig.BridgeAddrHasStake = true
	primeConfig.DefaultMinBridgingFee = oldMinBridgingFee
	primeConfig.MinBridgingFeeForTokens = oldMinBridgingFee
	cardanoConfig.DefaultMinBridgingFee = oldMinBridgingFee
	cardanoConfig.MinBridgingFeeForTokens = oldMinBridgingFee

	apex := cardanofw.SetupAndRunSkylineBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithCardanoConfig(cardanoConfig),
		cardanofw.WithPrimeConfig(primeConfig),
		cardanofw.WithBridgingAddrCnt(cardanofw.ChainIDPrime, bridgeAddrCnt),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	fundTestUsersWithToken(
		t, ctx, apex, []*testConfig{
			{
				srcChainID:      cardanofw.ChainIDCardano,
				srcMinterWallet: apex.GetCardanoInfo(cardanofw.ChainIDCardano).GenesisWallet,
			},
		}, apex.Users[:2], cardanofw.ApexToWei(big.NewInt(2)), cardanofw.ApexToWei(big.NewInt(100)))

	sendAmount := cardanofw.DfmToWei(big.NewInt(1_500_000))

	executeBridging := func(
		srcChainID, dstChainID cardanofw.ChainID, sendAmount *big.Int,
		senders, receivers []*cardanofw.TestApexUser,
	) {
		wg := sync.WaitGroup{}
		wg.Add(2)

		bridgingTypes := map[e2ehelper.SrcDstChainPair]uint16{
			e2ehelper.NewChainPair(cardanofw.ChainIDPrime, cardanofw.ChainIDCardano): cardanofw.AP3XTokenID,
			e2ehelper.NewChainPair(cardanofw.ChainIDCardano, cardanofw.ChainIDPrime): cardanofw.CAP3XTokenID,
		}

		for i := range len(bridgingTypes) {
			go func(idx int) {
				defer wg.Done()
				e2ehelper.ExecuteSingleBridging(
					t, ctx, apex, senders[idx], receivers[idx], srcChainID, dstChainID,
					sendAmount, bridgingTypes[e2ehelper.NewChainPair(srcChainID, dstChainID)], false)
			}(i)
		}

		wg.Wait()
	}

	executeBridging(cardanofw.ChainIDPrime, cardanofw.ChainIDCardano, sendAmount,
		[]*cardanofw.TestApexUser{apex.Users[0], apex.Users[1]}, []*cardanofw.TestApexUser{apex.Users[2], apex.Users[3]})

	// 1. Check existing stake pools in the system
	stakePoolsPrime := apex.GetChainMust(t, cardanofw.ChainIDPrime).GetExistingStakePools(t, ctx)
	require.NotEmpty(t, stakePoolsPrime)

	stakePoolsCardano := apex.GetChainMust(t, cardanofw.ChainIDCardano).GetExistingStakePools(t, ctx)
	require.NotEmpty(t, stakePoolsCardano)

	getStakePools := func(chainID cardanofw.ChainID, index int) string {
		if chainID == cardanofw.ChainIDPrime {
			return stakePoolsPrime[index]
		}

		return stakePoolsCardano[index]
	}

	getDestChain := func(chainID cardanofw.ChainID) cardanofw.ChainID {
		if chainID == cardanofw.ChainIDPrime {
			return cardanofw.ChainIDCardano
		}

		return cardanofw.ChainIDPrime
	}

	chains := []cardanofw.ChainID{cardanofw.ChainIDPrime, cardanofw.ChainIDCardano}

	t.Run("redeleg before reg and del should fail", func(t *testing.T) {
		for idx, chain := range chains {
			fmt.Printf("%d. %s\n", idx+1, chain)
			err := apex.DelegateStakeAddress(ctx, chain, 0, getStakePools(chain, 1), false)
			require.Error(t, err)
		}
	})

	t.Run("reg and del should pass", func(t *testing.T) {
		for idx, chain := range chains {
			fmt.Printf("%d. %s\n", idx+1, chain)
			err := apex.DelegateStakeAddress(ctx, chain, 0, getStakePools(chain, 0), true)
			require.NoError(t, err)

			addrInfo, err := apex.GetChainMust(t, chain).GetBridgingStakeAddressInfo(t, ctx, 0, false)
			require.NoError(t, err)
			require.Equal(t, getStakePools(chain, 0), addrInfo.StakeDelegation)

			executeBridging(chain, getDestChain(chain), sendAmount,
				[]*cardanofw.TestApexUser{apex.Users[0], apex.Users[1]}, []*cardanofw.TestApexUser{apex.Users[2], apex.Users[3]})
		}
	})

	t.Run("reg and del again should fail", func(t *testing.T) {
		// Registering already registered address should fail:
		for idx, chain := range chains {
			fmt.Printf("%d. %s\n", idx+1, chain)
			err := apex.DelegateStakeAddress(ctx, chain, 0, getStakePools(chain, 0), true)
			require.Error(t, err)
		}
	})

	t.Run("redeleg should pass", func(t *testing.T) {
		for idx, chain := range chains {
			fmt.Printf("%d. %s\n", idx+1, chain)
			err := apex.DelegateStakeAddress(ctx, chain, 0, getStakePools(chain, 1), false)
			require.NoError(t, err)

			previousStakePool := getStakePools(chain, 0)

			for range 60 {
				select {
				case <-ctx.Done():
					return
				case <-time.After(time.Second):
				}

				addrInfo, err := apex.GetChainMust(t, chain).GetBridgingStakeAddressInfo(t, ctx, 0, false)
				require.NoError(t, err)

				if addrInfo.StakeDelegation != previousStakePool {
					require.Equal(t, getStakePools(chain, 1), addrInfo.StakeDelegation)

					break
				}
			}

			executeBridging(chain, getDestChain(chain), sendAmount,
				[]*cardanofw.TestApexUser{apex.Users[0], apex.Users[1]}, []*cardanofw.TestApexUser{apex.Users[2], apex.Users[3]})
		}
	})

	t.Run("dereg should pass", func(t *testing.T) {
		for idx, chain := range chains {
			fmt.Printf("%d. %s\n", idx+1, chain)
			err := apex.DeregisterStakeAddress(ctx, chain, 0)
			require.NoError(t, err)

			for range 60 {
				select {
				case <-ctx.Done():
					return
				case <-time.After(time.Second):
				}

				addrInfo, err := apex.GetChainMust(t, chain).GetBridgingStakeAddressInfo(t, ctx, 0, true)

				if err != nil {
					require.ErrorContains(t, err, "stake address is not registered yet")
					require.Error(t, err)
					require.Equal(t, addrInfo, wallet.QueryStakeAddressInfo{})

					break
				}
			}

			executeBridging(chain, getDestChain(chain), sendAmount,
				[]*cardanofw.TestApexUser{apex.Users[0], apex.Users[1]}, []*cardanofw.TestApexUser{apex.Users[2], apex.Users[3]})
		}
	})

	t.Run("simultaneous test", func(t *testing.T) {
		executeBridging := func(
			srcChainID, dstChainID cardanofw.ChainID, sendAmount *big.Int,
			senders, receivers []*cardanofw.TestApexUser, doRegDeleg bool,
		) {
			bridgingTypes := map[e2ehelper.SrcDstChainPair]uint16{
				e2ehelper.NewChainPair(cardanofw.ChainIDPrime, cardanofw.ChainIDCardano): cardanofw.AP3XTokenID,
				e2ehelper.NewChainPair(cardanofw.ChainIDCardano, cardanofw.ChainIDPrime): cardanofw.CAP3XTokenID,
			}

			wg := sync.WaitGroup{}
			wg.Add(len(bridgingTypes) + 1)

			for i := range len(bridgingTypes) {
				go func(idx int) {
					defer wg.Done()
					e2ehelper.ExecuteSingleBridging(
						t, ctx, apex, senders[idx], receivers[idx], srcChainID, dstChainID,
						sendAmount, bridgingTypes[e2ehelper.NewChainPair(srcChainID, dstChainID)], false)
				}(i)
			}

			go func() {
				defer wg.Done()

				if !doRegDeleg {
					return
				}

				// 1. Check existing stake pools in the system
				stakePools := apex.GetChainMust(t, srcChainID).GetExistingStakePools(t, ctx)
				require.NotEmpty(t, stakePools)

				// 2. Register and delegate bridging address
				err := apex.DelegateStakeAddress(ctx, srcChainID, 0, stakePools[0], true)
				require.NoError(t, err)

				// 3. Check if the registration and delegation was successful
				addrInfo, err := apex.GetChainMust(t, srcChainID).GetBridgingStakeAddressInfo(t, ctx, 0, false)
				require.NoError(t, err)
				require.Equal(t, stakePools[0], addrInfo.StakeDelegation)
				fmt.Println("Bridging address staked successfully")
			}()

			wg.Wait()
		}

		doRegDelegValues := []bool{false, true, false}

		for idx, srcChainID := range chains {
			fmt.Printf("%d. %s\n", idx+1, srcChainID)

			for _, doRegDeleg := range doRegDelegValues {
				executeBridging(
					srcChainID,
					getDestChain(srcChainID),
					sendAmount,
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
	})
}

// go test -timeout 0 -run ^TestE2E_SkylineBridgeMBA_MutltipleAddresses$ github.com/0xPolygon/polygon-edge/e2e-polybft/e2e -v
func TestE2E_SkylineBridgeMBA_MutltipleAddresses(t *testing.T) {
	const apiKey = "test_api_key"

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	// Combined configuration for both currency and native token tests
	primeConfig, cardanoConfig := cardanofw.NewPrimeChainConfig(), cardanofw.NewCardanoChainConfig(true)
	primeConfig.DefaultMinBridgingFee = oldMinBridgingFee
	primeConfig.MinBridgingFeeForTokens = oldMinBridgingFee
	cardanoConfig.DefaultMinBridgingFee = oldMinBridgingFee
	cardanoConfig.MinBridgingFeeForTokens = oldMinBridgingFee
	primeConfig.FundAmount = 0
	primeConfig.FundTokenAmount = 0
	cardanoConfig.FundTokenAmount = uint64(1_000_000_000)
	primeConfig.BridgingAddressCnt = bridgeAddrCnt
	bridgingAmount := cardanofw.ApexToWei(big.NewInt(1))

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
			cardanofw.CAP3XTokenName, cardanofw.DefaultTokenMintAmount,
			cardanofw.ApexToWei(big.NewInt(2)), cardanofw.ApexToWei(big.NewInt(100)))
		require.NoError(t, err)

		// Initial funding for currency tests
		for range bridgeAddrCnt {
			e2ehelper.ExecuteSingleBridging(
				t, ctx, apex, apex.Users[0], apex.Users[1],
				cardanofw.ChainIDPrime, cardanofw.ChainIDCardano,
				big.NewInt(0).Mul(bridgingAmount, big.NewInt(5)), cardanofw.AP3XTokenID, true)
		}

		addrAmounts, err := apex.GetBridgingAddressesTokenAmounts(ctx, cardanofw.ChainIDPrime)
		require.NoError(t, err)
		fmt.Println("Currency tests - Initial multisig addresses amounts: ", addrAmounts)
	})

	t.Run("Currency Bridging - Bridge partial amount from single address", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, apex.Users[1], apex.Users[0],
			cardanofw.ChainIDCardano, cardanofw.ChainIDPrime,
			big.NewInt(0).Mul(bridgingAmount, big.NewInt(1)), cardanofw.CAP3XTokenID, true)

		addrAmounts, err := apex.GetBridgingAddressesTokenAmounts(ctx, cardanofw.ChainIDPrime)
		require.NoError(t, err)
		fmt.Println("Currency tests - Multisig addresses amounts: ", addrAmounts)

		require.Equal(t, cardanofw.DfmToWei(big.NewInt(4_000_010)), addrAmounts[0][wallet.AdaTokenName])
	})

	t.Run("Currency Bridging - Bridge full amount from single address", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, apex.Users[1], apex.Users[0],
			cardanofw.ChainIDCardano, cardanofw.ChainIDPrime,
			cardanofw.DfmToWei(big.NewInt(5_000_010)), cardanofw.CAP3XTokenID, true)

		addrAmounts, err := apex.GetBridgingAddressesTokenAmounts(ctx, cardanofw.ChainIDPrime)
		require.NoError(t, err)
		fmt.Println("Currency tests - Multisig addresses amounts: ", addrAmounts)

		require.Equal(t, big.NewInt(0), addrAmounts[1][wallet.AdaTokenName])
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
			cardanofw.ApexToWei(big.NewInt(5)), cardanofw.CAP3XTokenID, true)

		addrAmounts, err := apex.GetBridgingAddressesTokenAmounts(ctx, cardanofw.ChainIDPrime)
		require.NoError(t, err)
		fmt.Println("Currency tests - Multisig addresses amounts: ", addrAmounts)

		require.Equal(t, cardanofw.ApexToWei(big.NewInt(1)), addrAmounts[2][wallet.AdaTokenName])
		require.Equal(t, cardanofw.DfmToWei(big.NewInt(5_000_020)), addrAmounts[3][wallet.AdaTokenName])
	})

	t.Run("Currency Bridging - Bridge full amount from 2 addresses", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, apex.Users[1], apex.Users[0],
			cardanofw.ChainIDCardano, cardanofw.ChainIDPrime,
			cardanofw.DfmToWei(big.NewInt(9_000_030)), cardanofw.CAP3XTokenID, true)

		addrAmounts, err := apex.GetBridgingAddressesTokenAmounts(ctx, cardanofw.ChainIDPrime)
		require.NoError(t, err)
		fmt.Println("Currency tests - Multisig addresses amounts: ", addrAmounts)

		require.Equal(t, big.NewInt(0), addrAmounts[0][wallet.AdaTokenName])
		require.Equal(t, big.NewInt(0), addrAmounts[3][wallet.AdaTokenName])
	})

	t.Run("Currency Bridging - Replenish for remaining tests", func(t *testing.T) {
		// replenish
		for range bridgeAddrCnt {
			e2ehelper.ExecuteSingleBridging(
				t, ctx, apex, apex.Users[0], apex.Users[1],
				cardanofw.ChainIDPrime, cardanofw.ChainIDCardano,
				big.NewInt(0).Mul(bridgingAmount, big.NewInt(5)), cardanofw.AP3XTokenID, true)
		}

		addrAmounts, err := apex.GetBridgingAddressesTokenAmounts(ctx, cardanofw.ChainIDPrime)
		require.NoError(t, err)
		fmt.Println("Currency tests - Multisig addresses amounts: ", addrAmounts)
	})

	t.Run("Currency Bridging - Insufficient change + full", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, apex.Users[1], apex.Users[0],
			cardanofw.ChainIDCardano, cardanofw.ChainIDPrime,
			cardanofw.ApexToWei(big.NewInt(11)), cardanofw.CAP3XTokenID, true)

		addrAmounts, err := apex.GetBridgingAddressesTokenAmounts(ctx, cardanofw.ChainIDPrime)
		require.NoError(t, err)
		fmt.Println("Currency tests - Multisig addresses amounts: ", addrAmounts)

		require.Equal(t, big.NewInt(0), addrAmounts[0][wallet.AdaTokenName])
		require.Equal(t, cardanofw.ApexToWei(big.NewInt(1)), addrAmounts[1][wallet.AdaTokenName])
		require.Equal(t, cardanofw.DfmToWei(big.NewInt(5_000_030)), addrAmounts[2][wallet.AdaTokenName])
	})

	t.Run("Currency Bridging - Replenish for final tests", func(t *testing.T) {
		// replenish
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, apex.Users[0], apex.Users[1],
			cardanofw.ChainIDPrime, cardanofw.ChainIDCardano,
			big.NewInt(0).Mul(bridgingAmount, big.NewInt(5)), cardanofw.AP3XTokenID, true)
	})

	t.Run("Currency Bridging - Bridge full amount from 2 + partial from 1", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, apex.Users[1], apex.Users[0],
			cardanofw.ChainIDCardano, cardanofw.ChainIDPrime,
			cardanofw.DfmToWei(big.NewInt(6_000_010+6_000_010+1_000_030)), cardanofw.CAP3XTokenID, true)

		addrAmounts, err := apex.GetBridgingAddressesTokenAmounts(ctx, cardanofw.ChainIDPrime)
		require.NoError(t, err)
		fmt.Println("Currency tests - Multisig addresses amounts: ", addrAmounts)

		require.Equal(t, big.NewInt(0), addrAmounts[0][wallet.AdaTokenName])
		require.Equal(t, cardanofw.ApexToWei(big.NewInt(1)), addrAmounts[1][wallet.AdaTokenName])
		require.Equal(t, cardanofw.ApexToWei(big.NewInt(3)), addrAmounts[2][wallet.AdaTokenName])
	})

	t.Run("Currency Bridging - Final replenish", func(t *testing.T) {
		for range bridgeAddrCnt - 1 {
			e2ehelper.ExecuteSingleBridging(
				t, ctx, apex, apex.Users[0], apex.Users[1],
				cardanofw.ChainIDPrime, cardanofw.ChainIDCardano,
				big.NewInt(0).Mul(bridgingAmount, big.NewInt(5)), cardanofw.AP3XTokenID, true)
		}

		addrAmounts, err := apex.GetBridgingAddressesTokenAmounts(ctx, cardanofw.ChainIDPrime)
		require.NoError(t, err)
		fmt.Println("Currency tests - Multisig addresses amounts: ", addrAmounts)
	})

	t.Run("Currency Bridging - Bridge full amount from 2 (addr0) + partial from 1", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, apex.Users[1], apex.Users[0],
			cardanofw.ChainIDCardano, cardanofw.ChainIDPrime,
			cardanofw.ApexToWei(big.NewInt(17)), cardanofw.CAP3XTokenID, true)

		addrAmounts, err := apex.GetBridgingAddressesTokenAmounts(ctx, cardanofw.ChainIDPrime)
		require.NoError(t, err)
		fmt.Println("Currency tests - Multisig addresses amounts: ", addrAmounts)

		require.Equal(t, big.NewInt(0), addrAmounts[0][wallet.AdaTokenName])
		require.Equal(t, big.NewInt(0), addrAmounts[1][wallet.AdaTokenName])
		require.Equal(t, cardanofw.ApexToWei(big.NewInt(3)), addrAmounts[2][wallet.AdaTokenName])
		require.Equal(t, cardanofw.DfmToWei(big.NewInt(1000030)), addrAmounts[3][wallet.AdaTokenName])
	})

	t.Run("Currency Bridging - Test carry over consolidation, 0 on addr 0", func(t *testing.T) {
		ctxChild, cncl := context.WithCancel(ctx)
		defer cncl()

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, apex.Users[1], apex.Users[0],
			cardanofw.ChainIDCardano, cardanofw.ChainIDPrime,
			cardanofw.DfmToWei(big.NewInt(1_999_900)), cardanofw.CAP3XTokenID, true)

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

		require.Equal(t, cardanofw.DfmToWei(big.NewInt(1000130)), addrAmounts[0][wallet.AdaTokenName])
		require.Equal(t, big.NewInt(0), addrAmounts[1][wallet.AdaTokenName])
		require.Equal(t, big.NewInt(0), addrAmounts[2][wallet.AdaTokenName])
		require.Equal(t, big.NewInt(0), addrAmounts[3][wallet.AdaTokenName])
	})
}

func TestE2E_SkylineBridgeMBA_MutltipleAddresses_Native(t *testing.T) {
	const apiKey = "test_api_key"

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	// Combined configuration for both currency and native token tests
	primeConfig, cardanoConfig, vectorConfig := cardanofw.NewPrimeChainConfig(), cardanofw.NewCardanoChainConfig(true), cardanofw.NewVectorChainConfig()
	primeConfig.DefaultMinBridgingFee = oldMinBridgingFee
	primeConfig.MinBridgingFeeForTokens = oldMinBridgingFee
	cardanoConfig.DefaultMinBridgingFee = oldMinBridgingFee
	cardanoConfig.MinBridgingFeeForTokens = oldMinBridgingFee
	vectorConfig.DefaultMinBridgingFee = oldMinBridgingFee
	vectorConfig.MinBridgingFeeForTokens = oldMinBridgingFee
	primeConfig.FundAmount = 1_000_000_000
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
			cardanofw.CAP3XTokenName, cardanofw.DefaultTokenMintAmount,
			cardanofw.ApexToWei(big.NewInt(2)), cardanofw.ApexToWei(big.NewInt(100)))
		require.NoError(t, err)

		tokenName = cardanoToken.Token.String()

		// Fund users for native token tests on Vector chain
		_, err = cardanofw.FundUserWithToken(
			ctx, apex, cardanofw.ChainIDVector,
			apex.GetCardanoInfo(cardanofw.ChainIDVector).GenesisWallet, apex.Users[1],
			cardanofw.XADATokenName, cardanofw.DefaultTokenMintAmount,
			cardanofw.ApexToWei(big.NewInt(2)), cardanofw.ApexToWei(big.NewInt(100)))
		require.NoError(t, err)

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, apex.Users[4], apex.Users[4],
			cardanofw.ChainIDCardano, cardanofw.ChainIDVector,
			cardanofw.ApexToWei(big.NewInt(9)), cardanofw.ADATokenID, true)

		addrAmounts, err := apex.GetBridgingAddressesTokenAmounts(ctx, cardanofw.ChainIDCardano)
		require.NoError(t, err)
		fmt.Println("Native token tests - Initial multisig addresses amounts: ", addrAmounts)
	})

	// Helper function for parallel bridging vector -> cardano and prime -> cardano
	executeBridging := func(
		sendAmounts []*big.Int,
		senders, receivers []*cardanofw.TestApexUser,
		stcTokenIDs []uint16,
	) {
		wg := sync.WaitGroup{}
		wg.Add(len(stcTokenIDs))

		for i := range stcTokenIDs {
			go func(idx int) {
				defer wg.Done()

				var srcChainID, dstChainID string

				if sendAmounts[idx].Uint64() != 0 {
					if stcTokenIDs[idx] == cardanofw.XADATokenID {
						srcChainID, dstChainID = cardanofw.ChainIDVector, cardanofw.ChainIDCardano
					} else {
						srcChainID, dstChainID = cardanofw.ChainIDPrime, cardanofw.ChainIDCardano
					}

					e2ehelper.ExecuteSingleBridging(
						t, ctx, apex, senders[idx], receivers[idx], srcChainID, dstChainID, sendAmounts[idx], stcTokenIDs[idx], false)
				}
			}(i)
		}

		wg.Wait()
	}

	t.Run("Native Token Bridging - Bridge partial amount native and currency addr0 second", func(t *testing.T) {
		sendAmountToken := cardanofw.ApexToWei(big.NewInt(1))
		sendAmountNative := cardanofw.ApexToWei(big.NewInt(1))
		sendAmounts := []*big.Int{sendAmountNative, sendAmountToken}
		srcTokenIDs := []uint16{
			cardanofw.AP3XTokenID,
			cardanofw.XADATokenID,
		}

		executeBridging(sendAmounts,
			[]*cardanofw.TestApexUser{apex.Users[0], apex.Users[1]},
			[]*cardanofw.TestApexUser{apex.Users[2], apex.Users[3]},
			srcTokenIDs,
		)

		addrAmounts, err := apex.GetBridgingAddressesTokenAmounts(ctx, cardanofw.ChainIDCardano)
		require.NoError(t, err)
		fmt.Println("Native token tests - Multisig addresses amounts: ", addrAmounts)

		require.Equal(t, cardanofw.ApexToWei(new(big.Int).SetUint64(6)), addrAmounts[0][wallet.AdaTokenName])
		require.Equal(t, cardanofw.ApexToWei(new(big.Int).SetUint64(9)), addrAmounts[0][tokenName])
		require.Equal(t, cardanofw.DfmToWei(new(big.Int).SetUint64(5961300)), addrAmounts[1][wallet.AdaTokenName])
	})

	t.Run("Native Token Bridging - Bridge partial amount native and currency from 2 addrs", func(t *testing.T) {
		sendAmountToken := cardanofw.ApexToWei(big.NewInt(3))
		sendAmountNative := cardanofw.ApexToWei(big.NewInt(1))
		sendAmounts := []*big.Int{sendAmountNative, sendAmountToken}
		srcTokenIDs := []uint16{
			cardanofw.AP3XTokenID,
			cardanofw.XADATokenID,
		}

		executeBridging(sendAmounts,
			[]*cardanofw.TestApexUser{apex.Users[0], apex.Users[1]},
			[]*cardanofw.TestApexUser{apex.Users[2], apex.Users[3]},
			srcTokenIDs,
		)

		addrAmounts, err := apex.GetBridgingAddressesTokenAmounts(ctx, cardanofw.ChainIDCardano)
		require.NoError(t, err)
		fmt.Println("Native token tests - Multisig addresses amounts: ", addrAmounts)

		// This is due the fact that sometimes the tx's are processed in the same batch
		// and sometimes they are processed in different batches
		if addrAmounts[0][wallet.AdaTokenName].Cmp(cardanofw.DfmToWei(new(big.Int).SetUint64(3961290))) == 0 {
			require.Equal(t, cardanofw.DfmToWei(new(big.Int).SetUint64(3961290)), addrAmounts[0][wallet.AdaTokenName])
			require.Equal(t, cardanofw.ApexToWei(new(big.Int).SetUint64(8)), addrAmounts[0][tokenName])
			require.Equal(t, cardanofw.DfmToWei(new(big.Int).SetUint64(1961300)), addrAmounts[1][wallet.AdaTokenName])
		} else {
			require.Equal(t, cardanofw.DfmToWei(new(big.Int).SetUint64(1038710)), addrAmounts[0][wallet.AdaTokenName])
			require.Equal(t, cardanofw.DfmToWei(new(big.Int).SetUint64(8000000)), addrAmounts[0][tokenName])
			require.Equal(t, cardanofw.DfmToWei(new(big.Int).SetUint64(4883880)), addrAmounts[1][wallet.AdaTokenName])
		}
	})

	t.Run("Native Token Bridging - Send all native and curr tokens", func(t *testing.T) {
		sendAmountToken := cardanofw.DfmToWei(big.NewInt(2_883_880))
		sendAmountNative := cardanofw.ApexToWei(big.NewInt(8))
		sendAmounts := []*big.Int{sendAmountNative, sendAmountToken}
		srcTokenIDs := []uint16{
			cardanofw.AP3XTokenID,
			cardanofw.XADATokenID,
		}

		executeBridging(sendAmounts,
			[]*cardanofw.TestApexUser{apex.Users[0], apex.Users[1]},
			[]*cardanofw.TestApexUser{apex.Users[2], apex.Users[3]},
			srcTokenIDs,
		)

		addrAmounts, err := apex.GetBridgingAddressesTokenAmounts(ctx, cardanofw.ChainIDCardano)
		require.NoError(t, err)
		fmt.Println("Native token tests - Multisig addresses amounts: ", addrAmounts)

		require.Equal(t, big.NewInt(0), addrAmounts[0][wallet.AdaTokenName])
		require.Equal(t, 1, len(addrAmounts[0]))
		require.Equal(t, big.NewInt(0), addrAmounts[1][wallet.AdaTokenName])
	})
}

// go test -timeout 0 -run ^TestE2E_SkylineBridgeMBA_RedistributeTokens$ github.com/0xPolygon/polygon-edge/e2e-polybft/e2e -v
func TestE2E_SkylineBridgeMBA_RedistributeTokens(t *testing.T) {
	const (
		apiKey = "test_api_key"
	)

	bridgeAddCnt := 3
	bridgingAmount := cardanofw.DfmToWei(big.NewInt(10_000_002))

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	primeConfig, cardanoConfig := cardanofw.NewPrimeChainConfig(), cardanofw.NewCardanoChainConfig(true)
	primeConfig.DefaultMinBridgingFee = oldMinBridgingFee
	primeConfig.MinBridgingFeeForTokens = oldMinBridgingFee
	cardanoConfig.DefaultMinBridgingFee = oldMinBridgingFee
	cardanoConfig.MinBridgingFeeForTokens = oldMinBridgingFee
	primeConfig.BridgingAddressCnt = bridgeAddCnt
	cardanoConfig.FundTokenAmount = uint64(1_000_000_000)

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
		bridgingAmount, cardanofw.AP3XTokenID, true)

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
		bridgingAmount, cardanofw.CAP3XTokenID, true)

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
			bridgingAmount, cardanofw.AP3XTokenID, true)

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
				bridgingAmount, cardanofw.CAP3XTokenID, false)
		}()

		wg.Wait()

		addrAmounts, err = apex.GetBridgingAddressesTokenAmounts(ctx, cardanofw.ChainIDPrime)
		require.NoError(t, err)
		fmt.Println("Multisig addresses amounts: ", addrAmounts)

		require.Equal(t, bridgeAddCnt, len(addrAmounts))
		require.False(t, e2ehelper.IsDiffGreaterThanOne(addrAmounts[1][wallet.AdaTokenName], addrAmounts[2][wallet.AdaTokenName]))
	})
}
