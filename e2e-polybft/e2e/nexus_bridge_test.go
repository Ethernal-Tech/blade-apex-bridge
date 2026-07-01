package e2e

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"math/big"
	"os"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/e2e-polybft/cardanofw"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/e2ehelper"
	"github.com/Ethernal-Tech/cardano-infrastructure/sendtx"
	cardanowallet "github.com/Ethernal-Tech/cardano-infrastructure/wallet"
	"github.com/stretchr/testify/require"
)

func TestE2E_ApexBridgeWithNexus_SingleBridging(t *testing.T) {
	if cardanofw.ShouldSkipE2RRedundantTests() {
		t.Skip()
	}

	const (
		apiKey = "test_api_key"
	)

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	apex := cardanofw.SetupAndRunReactorBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithNexusEnabled(true),
		cardanofw.WithUserCnt(1),
	)
	sendAmount := cardanofw.ApexToWei(big.NewInt(1))

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	directions := map[string][]string{
		cardanofw.ChainIDPrime:  {cardanofw.ChainIDVector, cardanofw.ChainIDNexus},
		cardanofw.ChainIDVector: {cardanofw.ChainIDPrime, cardanofw.ChainIDNexus},
		cardanofw.ChainIDNexus:  {cardanofw.ChainIDPrime, cardanofw.ChainIDVector},
	}

	t.Run("From Nexus", func(t *testing.T) {
		srcChain := cardanofw.ChainIDNexus

		for _, dstChain := range directions[srcChain] {
			fmt.Printf("Testing bridging from %s to %s\n", srcChain, dstChain)
			e2ehelper.ExecuteSingleBridging(
				t, ctx, apex, apex.Users[0], apex.Users[0], srcChain, dstChain, sendAmount, cardanofw.AP3XTokenID, true)
		}
	})

	t.Run("From Prime to Nexus", func(t *testing.T) {
		srcChain, dstChain := cardanofw.ChainIDPrime, cardanofw.ChainIDNexus

		dstTestChain := apex.GetChainMust(t, dstChain)

		relayerBalanceBefore, err := dstTestChain.GetAddressBalance(
			ctx, apex.NexusInfo.RelayerAddress.String())
		require.NoError(t, err)

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, apex.Users[0], apex.Users[0], srcChain, dstChain, sendAmount, cardanofw.AP3XTokenID, true)

		relayerBalanceAfter, err := dstTestChain.GetAddressBalance(
			ctx, apex.NexusInfo.RelayerAddress.String())
		require.NoError(t, err)

		require.True(t, relayerBalanceAfter[cardanowallet.AdaTokenName].Cmp(relayerBalanceBefore[cardanowallet.AdaTokenName]) == 1)
	})

	t.Run("From Vector to Nexus", func(t *testing.T) {
		srcChain, dstChain := cardanofw.ChainIDVector, cardanofw.ChainIDNexus

		dstTestChain := apex.GetChainMust(t, dstChain)

		relayerBalanceBefore, err := dstTestChain.GetAddressBalance(
			ctx, apex.NexusInfo.RelayerAddress.String())
		require.NoError(t, err)

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, apex.Users[0], apex.Users[0], srcChain, dstChain, sendAmount, cardanofw.AP3XTokenID, true)

		relayerBalanceAfter, err := dstTestChain.GetAddressBalance(
			ctx, apex.NexusInfo.RelayerAddress.String())
		require.NoError(t, err)

		fmt.Printf("Relayer balance before: %s, after: %s\n", relayerBalanceBefore[cardanowallet.AdaTokenName].String(), relayerBalanceAfter[cardanowallet.AdaTokenName].String())

		require.True(t, relayerBalanceAfter[cardanowallet.AdaTokenName].Cmp(relayerBalanceBefore[cardanowallet.AdaTokenName]) == 1)
	})

	t.Run("From Vector to Nexus", func(t *testing.T) {
		srcChain, dstChain := cardanofw.ChainIDVector, cardanofw.ChainIDNexus

		dstTestChain := apex.GetChainMust(t, dstChain)

		relayerBalanceBefore, err := dstTestChain.GetAddressBalance(
			ctx, apex.NexusInfo.RelayerAddress.String())
		require.NoError(t, err)

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, apex.Users[0], apex.Users[0], srcChain, dstChain, sendAmount, cardanofw.AP3XTokenID, true)

		relayerBalanceAfter, err := dstTestChain.GetAddressBalance(
			ctx, apex.NexusInfo.RelayerAddress.String())
		require.NoError(t, err)

		fmt.Printf("Relayer balance before: %s, after: %s\n", relayerBalanceBefore[cardanowallet.AdaTokenName].String(), relayerBalanceAfter[cardanowallet.AdaTokenName].String())

		require.True(t, relayerBalanceAfter[cardanowallet.AdaTokenName].Cmp(relayerBalanceBefore[cardanowallet.AdaTokenName]) == 1)
	})
}

func TestE2E_ApexBridgeWithNexus_SrcNexus_ValidScenarios(t *testing.T) {
	if cardanofw.ShouldSkipE2RRedundantTests() {
		t.Skip()
	}

	const (
		apiKey  = "test_api_key"
		userCnt = 15
	)

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	apex := cardanofw.SetupAndRunReactorBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithNexusEnabled(true),
		cardanofw.WithUserCnt(userCnt),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	sendAmount := cardanofw.ApexToWei(big.NewInt(1))
	user := apex.Users[userCnt-1]
	srcChain := cardanofw.ChainIDNexus

	t.Run("One by one - wait for other side", func(t *testing.T) {
		const instances = 5

		e2ehelper.ExecuteBridgingOneByOneWaitOnOtherSide(
			t, ctx, apex, instances, user, srcChain, cardanofw.ChainIDPrime, sendAmount, cardanofw.AP3XTokenID)
	})

	t.Run("One by one - don't wait", func(t *testing.T) {
		const instances = 5

		e2ehelper.ExecuteBridgingWaitAfterSubmits(
			t, ctx, apex, instances, user, srcChain, cardanofw.ChainIDVector, sendAmount, cardanofw.AP3XTokenID)
	})

	t.Run("One by one - don't wait", func(t *testing.T) {
		const instances = 2

		e2ehelper.ExecuteBridgingWaitAfterSubmits(
			t, ctx, apex, instances, user, srcChain, cardanofw.ChainIDPrime, sendAmount, cardanofw.AP3XTokenID)
	})

	t.Run("Parallel", func(t *testing.T) {
		const instances = 5

		e2ehelper.ExecuteBridging(
			t, ctx, apex, 1,
			apex.Users[:instances],
			[]*cardanofw.TestApexUser{user},
			[]string{srcChain},
			map[string][]string{
				srcChain: {cardanofw.ChainIDVector},
			},
			map[e2ehelper.SrcDstChainPair]uint16{
				e2ehelper.NewChainPair(srcChain, cardanofw.ChainIDVector): cardanofw.AP3XTokenID,
			},
			sendAmount)
	})

	t.Run("Sequential and parallel", func(t *testing.T) {
		const (
			instances         = 5
			parallelInstances = 6
		)

		e2ehelper.ExecuteBridging(
			t, ctx, apex, instances,
			apex.Users[:parallelInstances],
			[]*cardanofw.TestApexUser{user},
			[]string{srcChain},
			map[string][]string{
				srcChain: {cardanofw.ChainIDPrime},
			},
			map[e2ehelper.SrcDstChainPair]uint16{
				e2ehelper.NewChainPair(srcChain, cardanofw.ChainIDPrime): cardanofw.AP3XTokenID,
			},
			sendAmount)
	})

	t.Run("Sequential and parallel multiple receivers", func(t *testing.T) {
		const (
			sequentialInstances = 5
			parallelInstances   = 6
		)

		SrcNexusSequentialAndParallelWithMaxReceivers(
			t, ctx, apex, cardanofw.ChainIDVector, sequentialInstances, parallelInstances, sendAmount)
	})

	t.Run("Sequential and parallel, one node goes off in the middle", func(t *testing.T) {
		const (
			instances            = 5
			parallelInstances    = 6
			stopAfter            = time.Second * 60
			validatorStoppingIdx = 1
		)

		e2ehelper.ExecuteBridging(
			t, ctx, apex, instances,
			apex.Users[:parallelInstances],
			apex.Users[len(apex.Users)-1:],
			[]string{srcChain},
			map[string][]string{
				srcChain: {cardanofw.ChainIDPrime},
			},
			map[e2ehelper.SrcDstChainPair]uint16{
				e2ehelper.NewChainPair(srcChain, cardanofw.ChainIDPrime): cardanofw.AP3XTokenID,
			},
			sendAmount,
			e2ehelper.WithRestartValidatorsConfig([]e2ehelper.RestartValidatorsConfig{
				{WaitTime: stopAfter, StopIndxs: []int{validatorStoppingIdx}},
			}))
	})
}

// this runs with refund tests because that job is underutilized, and the ApexBridgeWithNexus job seems to be
// struggling on GH Actions
func TestE2E_ABWithNexus_ApexRefund_SrcNexus_InvalidScenarios(t *testing.T) {
	if cardanofw.ShouldSkipE2RRedundantTests() {
		t.Skip()
	}

	const (
		apiKey  = "test_api_key"
		userCnt = 1
	)

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	apex := cardanofw.SetupAndRunReactorBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithNexusEnabled(true),
		cardanofw.WithUserCnt(userCnt),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	srcChain := cardanofw.ChainIDNexus
	dstChains := []string{cardanofw.ChainIDPrime, cardanofw.ChainIDVector}

	nexusAdminUser := &cardanofw.TestApexUser{
		NexusWallet:    apex.NexusInfo.AdminKey,
		NexusAddress:   apex.NexusInfo.AdminKey.Address(),
		HasNexusWallet: true,
	}

	nexusChain := apex.GetChainMust(t, cardanofw.ChainIDNexus).(*cardanofw.TestEVMChain)

	for _, dstChain := range dstChains {
		t.Run("Submitter not enough funds", func(t *testing.T) {
			SrcNexusSubmitterNotEnoughFunds(t, ctx, apex, dstChain)
		})

		t.Run("Big receiver amount", func(t *testing.T) {
			unfundedUser, err := cardanofw.NewTestApexUser(cardanofw.NewApexNetworkTypesFromSystem(apex))
			require.NoError(t, err)

			unfundedUserPk, err := unfundedUser.GetPrivateKey(srcChain)
			require.NoError(t, err)

			tokenInfo, err := apex.GetBridgingTokensInfo(cardanofw.ChainIDNexus, dstChain, cardanofw.AP3XTokenID)
			require.NoError(t, err)

			_, err = apex.SubmitTx(
				ctx, srcChain, nexusAdminUser, unfundedUser.NexusAddress.String(), cardanofw.ApexToWei(big.NewInt(10)), nil, nil, nil)
			require.NoError(t, err) // fund unfundedUser with amount less than sending amount

			feeAmount := cardanofw.WeiToChainNativeTokenAmount(
				cardanofw.ChainIDNexus,
				apex.GetMinBridgingFee(cardanofw.ChainIDNexus, true))

			sendAmount := cardanofw.ApexToWei(big.NewInt(20)) // try to send 20 apexs with users without enough funds

			txHash, err := nexusChain.DirectBridgingRequest(
				cardanofw.ChainIDToInt(dstChain),
				unfundedUserPk,
				map[string]cardanofw.ReceiverAmount{
					unfundedUser.GetAddress(cardanofw.ChainIDVector): {
						TokenID: cardanofw.AP3XTokenID,
						Amount:  sendAmount,
					},
				},
				feeAmount,
				big.NewInt(0),
				tokenInfo.SrcTokenName,
				true,
			)

			require.Equal(t, "", txHash)
			require.Error(t, err)
		})
	}
}

func TestE2E_ApexBridgeWithNexus_SrcNexus_InvalidScenarios_MinValuesMisconfigured(t *testing.T) {
	if cardanofw.ShouldSkipE2RRedundantTests() {
		t.Skip()
	}

	const (
		apiKey  = "test_api_key"
		userCnt = 1
	)

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	apex := cardanofw.SetupAndRunReactorBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithNexusEnabled(true),
		cardanofw.WithUserCnt(userCnt),
		cardanofw.WithCustomConfigHandlers(func(_ *cardanofw.ApexSystem, mp map[string]interface{}) {
			mp["refundEnabled"] = false
		}, nil, nil, nil),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	adminPrivKey, err := apex.NexusInfo.AdminKey.MarshallPrivateKey()
	require.NoError(t, err)

	require.NoError(t, misconfiguredMinAmounts(
		apex.NexusInfo.JSONRPCAddr,
		hex.EncodeToString(adminPrivKey),
		apex.NexusInfo.GatewayAddress.String(),
		big.NewInt(1),
		big.NewInt(1), nil, nil),
	)

	nexusChain := apex.GetChainMust(t, cardanofw.ChainIDNexus).(*cardanofw.TestEVMChain)

	dstChains := []string{cardanofw.ChainIDPrime, cardanofw.ChainIDVector}

	t.Run("Bridging min fee on sc less than oracle", func(t *testing.T) {
		user := apex.Users[userCnt-1]
		privKey, err := user.NexusWallet.MarshallPrivateKey()
		require.NoError(t, err)

		feeAmount := big.NewInt(1)

		sendAmount := cardanofw.ApexToWei(big.NewInt(1))

		for _, dstChain := range dstChains {
			tokenInfo, err := apex.GetBridgingTokensInfo(cardanofw.ChainIDNexus, dstChain, cardanofw.AP3XTokenID)
			require.NoError(t, err)

			txHash, err := nexusChain.DirectBridgingRequest(
				cardanofw.ChainIDToInt(dstChain),
				hex.EncodeToString(privKey),
				map[string]cardanofw.ReceiverAmount{
					user.GetAddress(cardanofw.ChainIDVector): {
						TokenID: cardanofw.AP3XTokenID,
						Amount:  sendAmount,
					},
				},
				feeAmount,
				big.NewInt(0),
				tokenInfo.SrcTokenName,
				true,
			)

			require.NotEqual(t, "", txHash)
			require.NoError(t, err)

			cardanofw.WaitForInvalidState(t, ctx, apex, cardanofw.ChainIDNexus, txHash, apiKey, 300)
		}
	})

	t.Run("Bridging 1 wei to cardano chain", func(t *testing.T) {
		user := apex.Users[userCnt-1]
		privKey, err := user.NexusWallet.MarshallPrivateKey()
		require.NoError(t, err)

		feeAmount := cardanofw.WeiToChainNativeTokenAmount(
			cardanofw.ChainIDNexus,
			apex.GetMinBridgingFee(cardanofw.ChainIDNexus, true))

		sendAmount := big.NewInt(1)

		for _, dstChain := range dstChains {
			tokenInfo, err := apex.GetBridgingTokensInfo(cardanofw.ChainIDNexus, dstChain, cardanofw.AP3XTokenID)
			require.NoError(t, err)

			txHash, err := nexusChain.DirectBridgingRequest(
				cardanofw.ChainIDToInt(dstChain),
				hex.EncodeToString(privKey),
				map[string]cardanofw.ReceiverAmount{
					user.GetAddress(dstChain): {
						TokenID: cardanofw.AP3XTokenID,
						Amount:  sendAmount,
					},
				},
				feeAmount,
				big.NewInt(0),
				tokenInfo.SrcTokenName,
				true,
			)

			require.NotEqual(t, "", txHash)
			require.NoError(t, err)

			cardanofw.WaitForInvalidState(t, ctx, apex, cardanofw.ChainIDNexus, txHash, apiKey, 300)
		}
	})
}

func TestE2E_ApexBridgeWithNexus_DestNexusAndBoth_ValidScenarios(t *testing.T) {
	const (
		apiKey  = "test_api_key"
		userCnt = 15
	)

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	primeConfig := cardanofw.NewPrimeChainConfig()
	primeConfig.UseIndexer = true

	vectorConfig := cardanofw.NewVectorChainConfig()
	vectorConfig.UseIndexer = true

	apex := cardanofw.SetupAndRunReactorBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithNexusEnabled(true),
		cardanofw.WithUserCnt(userCnt),
		cardanofw.WithPrimeConfig(primeConfig),
		cardanofw.WithVectorConfig(vectorConfig),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	user := apex.Users[userCnt-1]
	sendAmount := cardanofw.ApexToWei(big.NewInt(1))

	t.Run("From Prime to Nexus one by one - wait for other side", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() {
			t.Skip()
		}

		t.Cleanup(func() {
			apex.ResetIndexers()
		})

		const instances = 5

		e2ehelper.ExecuteBridgingOneByOneWaitOnOtherSide(
			t, ctx, apex, instances, user, cardanofw.ChainIDPrime, cardanofw.ChainIDNexus, sendAmount,
			cardanofw.AP3XTokenID)
	})

	t.Run("From Vector to Nexus one by one - don't wait for other side", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() {
			t.Skip()
		}

		t.Cleanup(func() {
			apex.ResetIndexers()
		})

		const instances = 5

		e2ehelper.ExecuteBridgingWaitAfterSubmits(
			t, ctx, apex, instances, user, cardanofw.ChainIDVector, cardanofw.ChainIDNexus, sendAmount,
			cardanofw.AP3XTokenID)
	})

	t.Run("From Prime to Nexus parallel", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() {
			t.Skip()
		}

		t.Cleanup(func() {
			apex.ResetIndexers()
		})

		const instances = 5

		e2ehelper.ExecuteBridging(
			t, ctx, apex, 1,
			apex.Users[:instances],
			[]*cardanofw.TestApexUser{user},
			[]string{cardanofw.ChainIDPrime},
			map[string][]string{
				cardanofw.ChainIDPrime: {cardanofw.ChainIDNexus},
			},
			map[e2ehelper.SrcDstChainPair]uint16{
				e2ehelper.NewChainPair(cardanofw.ChainIDPrime, cardanofw.ChainIDNexus): cardanofw.AP3XTokenID,
			},
			sendAmount)
	})

	t.Run("From Vector to Nexus sequential and parallel", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() {
			t.Skip()
		}

		t.Cleanup(func() {
			apex.ResetIndexers()
		})

		const (
			sequentialInstances = 5
			parallelInstances   = 10
		)

		e2ehelper.ExecuteBridging(
			t, ctx, apex,
			sequentialInstances,
			apex.Users[:parallelInstances],
			[]*cardanofw.TestApexUser{user},
			[]string{cardanofw.ChainIDVector},
			map[string][]string{
				cardanofw.ChainIDVector: {cardanofw.ChainIDNexus},
			},
			map[e2ehelper.SrcDstChainPair]uint16{
				e2ehelper.NewChainPair(cardanofw.ChainIDVector, cardanofw.ChainIDNexus): cardanofw.AP3XTokenID,
			},
			sendAmount)
	})

	t.Run("From Prime to Nexus sequential and parallel with max receivers", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() {
			t.Skip()
		}

		t.Cleanup(func() {
			apex.ResetIndexers()
		})

		const (
			sequentialInstances = 5
			parallelInstances   = 10
		)

		DstNexusSequentialAndParallelWithMaxReceivers(
			t, ctx, apex, cardanofw.ChainIDPrime, sequentialInstances, parallelInstances, sendAmount)
	})

	t.Run("From Vector to Nexus sequential and parallel - one node goes off in the midle", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() {
			t.Skip()
		}

		t.Cleanup(func() {
			apex.ResetIndexers()
		})

		const (
			sequentialInstances  = 5
			parallelInstances    = 6
			stopAfter            = time.Second * 60
			validatorStoppingIdx = 1
		)

		e2ehelper.ExecuteBridging(
			t, ctx, apex,
			sequentialInstances,
			apex.Users[:parallelInstances],
			[]*cardanofw.TestApexUser{user},
			[]string{cardanofw.ChainIDVector},
			map[string][]string{
				cardanofw.ChainIDVector: {cardanofw.ChainIDNexus},
			},
			map[e2ehelper.SrcDstChainPair]uint16{
				e2ehelper.NewChainPair(cardanofw.ChainIDVector, cardanofw.ChainIDNexus): cardanofw.AP3XTokenID,
			},
			sendAmount,
			e2ehelper.WithRestartValidatorsConfig([]e2ehelper.RestartValidatorsConfig{
				{WaitTime: stopAfter, StopIndxs: []int{validatorStoppingIdx}},
			}))
	})

	t.Run("Both directions sequential", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() {
			t.Skip()
		}

		t.Cleanup(func() {
			apex.ResetIndexers()
		})

		const instances = 3

		e2ehelper.ExecuteBridging(
			t, ctx, apex,
			instances,
			apex.Users[:1],
			[]*cardanofw.TestApexUser{user},
			[]string{cardanofw.ChainIDPrime, cardanofw.ChainIDVector, cardanofw.ChainIDNexus},
			map[string][]string{
				cardanofw.ChainIDPrime:  {cardanofw.ChainIDNexus},
				cardanofw.ChainIDNexus:  {cardanofw.ChainIDPrime, cardanofw.ChainIDVector},
				cardanofw.ChainIDVector: {cardanofw.ChainIDNexus},
			},
			map[e2ehelper.SrcDstChainPair]uint16{
				e2ehelper.NewChainPair(cardanofw.ChainIDPrime, cardanofw.ChainIDNexus):  cardanofw.AP3XTokenID,
				e2ehelper.NewChainPair(cardanofw.ChainIDNexus, cardanofw.ChainIDPrime):  cardanofw.AP3XTokenID,
				e2ehelper.NewChainPair(cardanofw.ChainIDNexus, cardanofw.ChainIDVector): cardanofw.AP3XTokenID,
				e2ehelper.NewChainPair(cardanofw.ChainIDVector, cardanofw.ChainIDNexus): cardanofw.AP3XTokenID,
			},
			sendAmount)
	})

	t.Run("Both directions sequential and parallel", func(t *testing.T) {
		t.Cleanup(func() {
			apex.ResetIndexers()
		})

		const (
			sequentialInstances = 4
			parallelInstances   = 5
		)

		t.Cleanup(func() {
			apex.ResetIndexers()
		})

		DstNexusBothDirectionsSequentialAndParallel(
			t, ctx, apex, cardanofw.ChainIDVector, user, sequentialInstances, parallelInstances, sendAmount)
	})

	t.Run("Both directions sequential and parallel - one node goes off in the midle", func(t *testing.T) {
		t.Cleanup(func() {
			apex.ResetIndexers()
		})

		const (
			sequentialInstances  = 5
			parallelInstances    = 6
			stopAfter            = time.Second * 60
			validatorStoppingIdx = 1
		)

		e2ehelper.ExecuteBridging(
			t, ctx, apex,
			sequentialInstances,
			apex.Users[:parallelInstances],
			[]*cardanofw.TestApexUser{user},
			[]string{cardanofw.ChainIDPrime, cardanofw.ChainIDNexus},
			map[string][]string{
				cardanofw.ChainIDPrime: {cardanofw.ChainIDNexus},
				cardanofw.ChainIDNexus: {cardanofw.ChainIDPrime},
			},
			map[e2ehelper.SrcDstChainPair]uint16{
				e2ehelper.NewChainPair(cardanofw.ChainIDPrime, cardanofw.ChainIDNexus): cardanofw.AP3XTokenID,
				e2ehelper.NewChainPair(cardanofw.ChainIDNexus, cardanofw.ChainIDPrime): cardanofw.AP3XTokenID,
			},
			sendAmount,
			e2ehelper.WithWaitForUnexpectedBridges(true),
			e2ehelper.WithRestartValidatorsConfig([]e2ehelper.RestartValidatorsConfig{
				{WaitTime: stopAfter, StopIndxs: []int{validatorStoppingIdx}},
			}))
	})

	t.Run("Both directions sequential and parallel - two nodes go off in the middle and then one comes back", func(t *testing.T) {
		t.Cleanup(func() {
			apex.ResetIndexers()
		})

		const (
			sequentialInstances   = 5
			parallelInstances     = 10
			stopAfter             = time.Second * 60
			startAgainAfter       = time.Second * 120
			validatorStoppingIdx1 = 1
			validatorStoppingIdx2 = 2
		)

		e2ehelper.ExecuteBridging(
			t, ctx, apex,
			sequentialInstances,
			apex.Users[:parallelInstances],
			[]*cardanofw.TestApexUser{user},
			[]string{cardanofw.ChainIDVector, cardanofw.ChainIDNexus},
			map[string][]string{
				cardanofw.ChainIDVector: {cardanofw.ChainIDNexus},
				cardanofw.ChainIDNexus:  {cardanofw.ChainIDVector},
			},
			map[e2ehelper.SrcDstChainPair]uint16{
				e2ehelper.NewChainPair(cardanofw.ChainIDVector, cardanofw.ChainIDNexus): cardanofw.AP3XTokenID,
				e2ehelper.NewChainPair(cardanofw.ChainIDNexus, cardanofw.ChainIDVector): cardanofw.AP3XTokenID,
			},
			sendAmount,
			e2ehelper.WithWaitForUnexpectedBridges(true),
			e2ehelper.WithRestartValidatorsConfig([]e2ehelper.RestartValidatorsConfig{
				{WaitTime: stopAfter, StopIndxs: []int{validatorStoppingIdx1, validatorStoppingIdx2}},
				{WaitTime: startAgainAfter, StartIndxs: []int{validatorStoppingIdx1}},
			}))
	})
}

// this runs with refund tests because that job is underutilized, and the ApexBridgeWithNexus job seems to be
// struggling on GH Actions
func TestE2E_ABWithNexus_ApexRefund_DstN_InvalidScenarios(t *testing.T) {
	const (
		apiKey        = "test_api_key"
		userCnt       = 15
		premineAmount = uint64(50_000_000)
	)

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	primeConfig := cardanofw.NewPrimeChainConfig()
	primeConfig.PremineAmount = premineAmount

	vectorConfig := cardanofw.NewVectorChainConfig()
	vectorConfig.PremineAmount = premineAmount

	apex := cardanofw.SetupAndRunReactorBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithNexusEnabled(true),
		cardanofw.WithPrimeConfig(primeConfig),
		cardanofw.WithVectorConfig(vectorConfig),
		cardanofw.WithUserCnt(userCnt),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	user := apex.Users[userCnt-1]

	t.Run("Submitter not enough funds", func(t *testing.T) {
		sendAmount := cardanofw.ApexToWei(big.NewInt(100))

		DstNexusSubmitterNotEnoughFunds(t, ctx, apex, cardanofw.ChainIDPrime, user, sendAmount)
	})

	t.Run("Submitted invalid metadata", func(t *testing.T) {
		sendAmount := cardanofw.ApexToWei(big.NewInt(1))

		DstNexusInvalidMetadataSlicedOff(t, ctx, apex, cardanofw.ChainIDVector, user, sendAmount)
	})

	t.Run("Submitted invalid metadata - wrong type", func(t *testing.T) {
		sendAmount := cardanofw.ApexToWei(big.NewInt(1))

		DstNexusInvalidMetadataWrongType(
			t, ctx, apex, cardanofw.ChainIDPrime, user, cardanofw.DefaultRequestStateTimeoutSec,
			sendAmount)
	})

	t.Run("Submitted invalid metadata - invalid destination", func(t *testing.T) {
		sendAmount := cardanofw.ApexToWei(big.NewInt(1))

		DstNexusInvalidMetadataInvalidDestination(t, ctx, apex, cardanofw.ChainIDVector, user, 0, sendAmount)
	})

	t.Run("Submitted invalid metadata - invalid sender", func(t *testing.T) {
		sendAmount := cardanofw.ApexToWei(big.NewInt(1))

		DstNexusInvalidMetadataInvalidSender(t, ctx, apex, cardanofw.ChainIDPrime, user, 0, sendAmount)
	})

	t.Run("Submitted invalid metadata - empty tx", func(t *testing.T) {
		sendAmount := cardanofw.ApexToWei(big.NewInt(1))

		DstNexusInvalidMetadataInvalidTransactions(t, ctx, apex, cardanofw.ChainIDVector, user, 0, sendAmount)
	})
}

// this runs with refund tests because that job is underutilized, and the ApexBridgeWithNexus job seems to be
// struggling on GH Actions
func TestE2E_ABWithNexus_ApexRefund_BatchFailed(t *testing.T) {
	const (
		apiKey  = "test_api_key"
		userCnt = 1
	)

	sendAmount := cardanofw.ApexToWei(big.NewInt(1))

	t.Run("Test insufficient gas price dynamicTx=true", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() {
			t.Skip()
		}

		srcChain := cardanofw.ChainIDPrime

		ctx, cncl := context.WithCancel(context.Background())
		defer cncl()

		var (
			failedToExecute int
			timeout         bool
		)

		apex := cardanofw.SetupAndRunReactorBridge(
			t, ctx,
			cardanofw.WithAPIKey(apiKey),
			cardanofw.WithNexusEnabled(true),
			cardanofw.WithUserCnt(userCnt),
			cardanofw.WithCustomConfigHandlers(nil, func(_ *cardanofw.ApexSystem, mp map[string]interface{}) {
				block := cardanofw.GetMapFromInterfaceKey(mp, "chains", cardanofw.ChainIDNexus, "config")
				block["gasFeeCap"] = uint64(10)
				block["gasTipCap"] = uint64(11)
			}, nil, nil),
		)

		user := apex.Users[userCnt-1]

		txHash, err := apex.SubmitBridgingRequest(cardanofw.SubmitBridgingRequestData{
			Context:          ctx,
			SourceChain:      srcChain,
			DestinationChain: cardanofw.ChainIDNexus,
			Sender:           user,
			WeiAmount:        sendAmount,
			SrcTokenID:       cardanofw.AP3XTokenID,
			Receivers:        []*cardanofw.TestApexUser{user},
		})
		require.NoError(t, err)

		fmt.Printf("Tx sent. hash: %s\n", txHash)

		// Check relay failed
		failedToExecute, timeout = cardanofw.WaitForBatchState(
			ctx, apex, srcChain, txHash, apiKey, true, false, cardanofw.BatchStateExecuted)

		require.Equal(t, failedToExecute, 1)
		require.False(t, timeout)

		// Restart relayer after config fix
		require.NoError(t, apex.StopRelayer())

		err = cardanofw.UpdateJSONFile(
			apex.GetValidator(t, 0).GetRelayerConfig(),
			apex.GetValidator(t, 0).GetRelayerConfig(),
			func(mp map[string]interface{}) {
				block := cardanofw.GetMapFromInterfaceKey(mp, "chains", cardanofw.ChainIDNexus, "config")
				block["gasFeeCap"] = uint64(0)
				block["gasTipCap"] = uint64(0)
			},
			false,
		)
		require.NoError(t, err)

		err = apex.StartRelayer(ctx)
		require.NoError(t, err)

		failedToExecute, timeout = cardanofw.WaitForBatchState(
			ctx, apex, srcChain, txHash, apiKey, false, false, cardanofw.BatchStateExecuted)

		require.LessOrEqual(t, failedToExecute, 1)
		require.False(t, timeout)
	})

	t.Run("Test insufficient gas price dynamicTx=false", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() {
			t.Skip()
		}

		ctx, cncl := context.WithCancel(context.Background())
		defer cncl()

		var (
			failedToExecute int
			timeout         bool
		)

		srcChain := cardanofw.ChainIDVector

		apex := cardanofw.SetupAndRunReactorBridge(
			t, ctx,
			cardanofw.WithAPIKey(apiKey),
			cardanofw.WithNexusEnabled(true),
			cardanofw.WithUserCnt(userCnt),
			cardanofw.WithCustomConfigHandlers(nil, func(_ *cardanofw.ApexSystem, mp map[string]interface{}) {
				block := cardanofw.GetMapFromInterfaceKey(mp, "chains", cardanofw.ChainIDNexus, "config")
				block["gasPrice"] = uint64(10)
				block["dynamicTx"] = bool(false)
			}, nil, nil),
		)

		user := apex.Users[userCnt-1]

		txHash, err := apex.SubmitBridgingRequest(cardanofw.SubmitBridgingRequestData{
			Context:          ctx,
			SourceChain:      srcChain,
			DestinationChain: cardanofw.ChainIDNexus,
			Sender:           user,
			WeiAmount:        sendAmount,
			SrcTokenID:       cardanofw.AP3XTokenID,
			Receivers:        []*cardanofw.TestApexUser{user},
		})
		require.NoError(t, err)

		fmt.Printf("Tx sent. hash: %s\n", txHash)

		// Check relay failed
		failedToExecute, timeout = cardanofw.WaitForBatchState(
			ctx, apex, srcChain, txHash, apiKey, true, false, cardanofw.BatchStateExecuted)

		require.Equal(t, failedToExecute, 1)
		require.False(t, timeout)

		// Restart relayer after config fix
		require.NoError(t, apex.StopRelayer())

		err = cardanofw.UpdateJSONFile(
			apex.GetValidator(t, 0).GetRelayerConfig(),
			apex.GetValidator(t, 0).GetRelayerConfig(),
			func(mp map[string]interface{}) {
				block := cardanofw.GetMapFromInterfaceKey(mp, "chains", cardanofw.ChainIDNexus, "config")
				block["gasPrice"] = uint64(0)
			},
			false,
		)
		require.NoError(t, err)

		err = apex.StartRelayer(ctx)
		require.NoError(t, err)

		failedToExecute, timeout = cardanofw.WaitForBatchState(
			ctx, apex, srcChain, txHash, apiKey, false, false, cardanofw.BatchStateExecuted)

		require.LessOrEqual(t, failedToExecute, 1)
		require.False(t, timeout)
	})

	t.Run("Test small fee", func(t *testing.T) {
		ctx, cncl := context.WithCancel(context.Background())
		defer cncl()

		var (
			failedToExecute int
			timeout         bool
		)

		srcChain := cardanofw.ChainIDPrime

		apex := cardanofw.SetupAndRunReactorBridge(
			t, ctx,
			cardanofw.WithAPIKey(apiKey),
			cardanofw.WithNexusEnabled(true),
			cardanofw.WithUserCnt(userCnt),
			cardanofw.WithCustomConfigHandlers(nil, func(_ *cardanofw.ApexSystem, mp map[string]interface{}) {
				cardanofw.GetMapFromInterfaceKey(mp, "chains", cardanofw.ChainIDNexus, "config")["depositGasLimit"] = uint64(10)
			}, nil, nil),
		)

		user := apex.Users[userCnt-1]

		txHash, err := apex.SubmitBridgingRequest(cardanofw.SubmitBridgingRequestData{
			Context:          ctx,
			SourceChain:      srcChain,
			DestinationChain: cardanofw.ChainIDNexus,
			Sender:           user,
			WeiAmount:        sendAmount,
			SrcTokenID:       cardanofw.AP3XTokenID,
			Receivers:        []*cardanofw.TestApexUser{user},
		})
		require.NoError(t, err)

		fmt.Printf("Tx sent. hash: %s\n", txHash)

		// Check relay failed
		failedToExecute, timeout = cardanofw.WaitForBatchState(ctx,
			apex, srcChain, txHash, apiKey, true, false, cardanofw.BatchStateExecuted)

		require.Equal(t, 1, failedToExecute)
		require.False(t, timeout)

		// Restart relayer after config fix
		require.NoError(t, apex.StopRelayer())

		err = cardanofw.UpdateJSONFile(
			apex.GetValidator(t, 0).GetRelayerConfig(),
			apex.GetValidator(t, 0).GetRelayerConfig(),
			func(mp map[string]interface{}) {
				cardanofw.GetMapFromInterfaceKey(mp, "chains", cardanofw.ChainIDNexus, "config")["depositGasLimit"] = uint64(0)
			},
			false,
		)
		require.NoError(t, err)

		err = apex.StartRelayer(ctx)
		require.NoError(t, err)

		failedToExecute, timeout = cardanofw.WaitForBatchState(ctx,
			apex, srcChain, txHash, apiKey, false, false, cardanofw.BatchStateExecuted)

		require.LessOrEqual(t, failedToExecute, 1)
		require.False(t, timeout)
	})

	t.Run("Test failed batch", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() {
			t.Skip()
		}

		ctx, cncl := context.WithCancel(context.Background())
		defer cncl()

		var (
			failedToExecute int
			timeout         bool
		)

		srcChain := cardanofw.ChainIDPrime

		apex := cardanofw.SetupAndRunReactorBridge(
			t, ctx,
			cardanofw.WithAPIKey(apiKey),
			cardanofw.WithNexusEnabled(true),
			cardanofw.WithUserCnt(userCnt),
			cardanofw.WithCustomConfigHandlers(func(_ *cardanofw.ApexSystem, mp map[string]interface{}) {
				cardanofw.GetMapFromInterfaceKey(mp, "ethChains", cardanofw.ChainIDNexus)["testMode"] = uint8(1)
			}, nil, nil, nil),
		)

		defer require.True(t, apex.ApexBridgeProcessesRunning())

		user := apex.Users[userCnt-1]

		prevBalance, err := apex.GetBalance(ctx, user, cardanofw.ChainIDNexus)
		require.NoError(t, err)

		tokensInfo, err := apex.GetBridgingTokensInfo(cardanofw.ChainIDNexus, cardanofw.ChainIDPrime, cardanofw.AP3XTokenID)
		require.NoError(t, err)

		prevBalanceDfm := prevBalance[cardanowallet.AdaTokenName]

		fmt.Printf("Dfm before Tx %d\n", prevBalanceDfm)

		expectedAmount := new(big.Int).Set(sendAmount)
		expectedAmount = expectedAmount.Add(expectedAmount, prevBalanceDfm)

		txHash, err := apex.SubmitBridgingRequest(cardanofw.SubmitBridgingRequestData{
			Context:          ctx,
			SourceChain:      srcChain,
			DestinationChain: cardanofw.ChainIDNexus,
			Sender:           user,
			WeiAmount:        sendAmount,
			SrcTokenID:       cardanofw.AP3XTokenID,
			TokensInfo:       tokensInfo,
			Receivers:        []*cardanofw.TestApexUser{user},
		})
		require.NoError(t, err)

		fmt.Printf("Tx sent. hash: %s\n", txHash)

		// Check batch failed
		failedToExecute, timeout = cardanofw.WaitForBatchState(
			ctx, apex, srcChain, txHash, apiKey, false, false, cardanofw.BatchStateExecuted)

		require.Equal(t, failedToExecute, 1)
		require.False(t, timeout)

		err = apex.WaitForExactAmount(ctx, user, cardanofw.ChainIDNexus, expectedAmount, 3, time.Second*10, tokensInfo.DstTokenName)
		require.NoError(t, err)
	})

	t.Run("Test failed batch 5 times in a row", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() {
			t.Skip()
		}

		ctx, cncl := context.WithCancel(context.Background())
		defer cncl()

		var (
			failedToExecute int
			timeout         bool
		)

		srcChain := cardanofw.ChainIDPrime

		apex := cardanofw.SetupAndRunReactorBridge(
			t, ctx,
			cardanofw.WithAPIKey(apiKey),
			cardanofw.WithNexusEnabled(true),
			cardanofw.WithUserCnt(userCnt),
			cardanofw.WithCustomConfigHandlers(func(_ *cardanofw.ApexSystem, mp map[string]interface{}) {
				cardanofw.GetMapFromInterfaceKey(mp, "ethChains", cardanofw.ChainIDNexus)["testMode"] = uint8(2)
			}, nil, nil, nil),
		)

		defer require.True(t, apex.ApexBridgeProcessesRunning())

		user := apex.Users[userCnt-1]

		prevBalance, err := apex.GetBalance(ctx, user, cardanofw.ChainIDNexus)
		require.NoError(t, err)

		tokensInfo, err := apex.GetBridgingTokensInfo(cardanofw.ChainIDNexus, cardanofw.ChainIDPrime, cardanofw.AP3XTokenID)
		require.NoError(t, err)

		prevBalanceDfm := prevBalance[cardanowallet.AdaTokenName]

		fmt.Printf("DFM Amount before Tx %d\n", prevBalanceDfm)

		expectedAmount := new(big.Int).Set(sendAmount)
		expectedAmount = expectedAmount.Add(expectedAmount, prevBalanceDfm)

		txHash, err := apex.SubmitBridgingRequest(cardanofw.SubmitBridgingRequestData{
			Context:          ctx,
			SourceChain:      srcChain,
			DestinationChain: cardanofw.ChainIDNexus,
			Sender:           user,
			WeiAmount:        sendAmount,
			SrcTokenID:       cardanofw.AP3XTokenID,
			TokensInfo:       tokensInfo,
			Receivers:        []*cardanofw.TestApexUser{user},
		})
		require.NoError(t, err)

		fmt.Printf("Tx sent. hash: %s\n", txHash)

		// Check batch failed
		failedToExecute, timeout = cardanofw.WaitForBatchState(
			ctx, apex, srcChain, txHash, apiKey, false, false, cardanofw.BatchStateExecuted)

		require.Equal(t, 5, failedToExecute)
		require.False(t, timeout)

		err = apex.WaitForExactAmount(ctx, user, cardanofw.ChainIDNexus, expectedAmount, 3, time.Second*10, tokensInfo.DstTokenName)
		require.NoError(t, err)
	})

	t.Run("Test multiple failed batches in a row", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() {
			t.Skip()
		}

		ctx, cncl := context.WithCancel(context.Background())
		defer cncl()

		instances := 5
		failedToExecute := make([]int, instances)
		timeout := make([]bool, instances)
		srcChain := cardanofw.ChainIDPrime

		apex := cardanofw.SetupAndRunReactorBridge(
			t, ctx,
			cardanofw.WithAPIKey(apiKey),
			cardanofw.WithNexusEnabled(true),
			cardanofw.WithUserCnt(userCnt),
			cardanofw.WithCustomConfigHandlers(func(_ *cardanofw.ApexSystem, mp map[string]interface{}) {
				cardanofw.GetMapFromInterfaceKey(mp, "ethChains", cardanofw.ChainIDNexus)["testMode"] = uint8(3)
			}, nil, nil, nil),
		)

		defer require.True(t, apex.ApexBridgeProcessesRunning())

		user := apex.Users[userCnt-1]

		prevBalance, err := apex.GetBalance(ctx, user, cardanofw.ChainIDNexus)
		require.NoError(t, err)

		tokensInfo, err := apex.GetBridgingTokensInfo(cardanofw.ChainIDNexus, cardanofw.ChainIDPrime, cardanofw.AP3XTokenID)
		require.NoError(t, err)

		prevBalanceWei := prevBalance[cardanowallet.AdaTokenName]

		fmt.Printf("DFM Amount before Tx %d\n", prevBalanceWei)

		ethExpectedBalance := big.NewInt(int64(instances))
		ethExpectedBalance.Mul(ethExpectedBalance, sendAmount)
		ethExpectedBalance.Add(ethExpectedBalance, prevBalanceWei)

		for i := 0; i < instances; i++ {
			txHash, err := apex.SubmitBridgingRequest(cardanofw.SubmitBridgingRequestData{
				Context:          ctx,
				SourceChain:      srcChain,
				DestinationChain: cardanofw.ChainIDNexus,
				Sender:           user,
				WeiAmount:        sendAmount,
				SrcTokenID:       cardanofw.AP3XTokenID,
				TokensInfo:       tokensInfo,
				Receivers:        []*cardanofw.TestApexUser{user},
			})
			require.NoError(t, err)

			fmt.Printf("Tx %v sent. hash: %s\n", i, txHash)

			failedToExecute[i], timeout[i] = cardanofw.WaitForBatchState(
				ctx, apex, srcChain, txHash, apiKey, false, false, cardanofw.BatchStateExecuted)
		}

		for i := 0; i < instances; i++ {
			require.Equal(t, failedToExecute[i], 1)
			require.False(t, timeout[i])
		}

		err = apex.WaitForExactAmount(ctx, user, cardanofw.ChainIDNexus, ethExpectedBalance, 20, time.Second*10, tokensInfo.DstTokenName)
		require.NoError(t, err)
	})

	t.Run("Test failed batches at random", func(t *testing.T) {
		ctx, cncl := context.WithCancel(context.Background())
		defer cncl()

		instances := 5
		failedToExecute := make([]int, instances)
		timeout := make([]bool, instances)

		srcChain := cardanofw.ChainIDPrime

		apex := cardanofw.SetupAndRunReactorBridge(
			t, ctx,
			cardanofw.WithAPIKey(apiKey),
			cardanofw.WithNexusEnabled(true),
			cardanofw.WithUserCnt(userCnt),
			cardanofw.WithCustomConfigHandlers(func(_ *cardanofw.ApexSystem, mp map[string]interface{}) {
				cardanofw.GetMapFromInterfaceKey(mp, "ethChains", cardanofw.ChainIDNexus)["testMode"] = uint8(4)
			}, nil, nil, nil),
		)

		defer require.True(t, apex.ApexBridgeProcessesRunning())

		user := apex.Users[userCnt-1]

		prevBalance, err := apex.GetBalance(ctx, user, cardanofw.ChainIDNexus)
		require.NoError(t, err)

		tokensInfo, err := apex.GetBridgingTokensInfo(cardanofw.ChainIDNexus, cardanofw.ChainIDPrime, cardanofw.AP3XTokenID)
		require.NoError(t, err)

		prevBalanceWei := prevBalance[cardanowallet.AdaTokenName]

		fmt.Printf("DFM Amount before Tx %d\n", prevBalanceWei)

		ethExpectedBalance := big.NewInt(int64(instances))
		ethExpectedBalance.Mul(ethExpectedBalance, sendAmount)
		ethExpectedBalance.Add(ethExpectedBalance, prevBalanceWei)

		for i := 0; i < instances; i++ {
			txHash, err := apex.SubmitBridgingRequest(cardanofw.SubmitBridgingRequestData{
				Context:          ctx,
				SourceChain:      srcChain,
				DestinationChain: cardanofw.ChainIDNexus,
				Sender:           user,
				WeiAmount:        sendAmount,
				SrcTokenID:       cardanofw.AP3XTokenID,
				TokensInfo:       tokensInfo,
				Receivers:        []*cardanofw.TestApexUser{user},
			})
			require.NoError(t, err)

			fmt.Printf("Tx %v sent. hash: %s\n", i, txHash)

			// Check batch failed
			failedToExecute[i], timeout[i] = cardanofw.WaitForBatchState(
				ctx, apex, srcChain, txHash, apiKey, false, false, cardanofw.BatchStateExecuted)
		}

		for i := 0; i < instances; i++ {
			if i%2 == 0 {
				require.Equal(t, 1, failedToExecute[i])
			}

			require.False(t, timeout[i])
		}

		err = apex.WaitForExactAmount(ctx, user, cardanofw.ChainIDNexus, ethExpectedBalance, 3, time.Second*10, tokensInfo.DstTokenName)
		require.NoError(t, err)
	})
}

func TestE2E_ApexBridgeWithNexus_NexusFundAmount(t *testing.T) {
	if cardanofw.ShouldSkipE2RRedundantTests() {
		t.Skip()
	}

	const (
		apiKey  = "test_api_key"
		userCnt = 10
	)

	fundAmount := cardanofw.ApexToWei(big.NewInt(100))

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	primeConfig := cardanofw.NewPrimeChainConfig()
	primeConfig.FundAmount = 1_000_000

	vectorConfig := cardanofw.NewVectorChainConfig()
	vectorConfig.FundAmount = 1_000_000

	nexusConfig := cardanofw.NewNexusChainConfig(true)
	nexusConfig.FundAmount = big.NewInt(1)

	apex := cardanofw.SetupAndRunReactorBridge(
		t, ctx,
		cardanofw.WithPrimeConfig(primeConfig),
		cardanofw.WithVectorConfig(vectorConfig),
		cardanofw.WithNexusConfig(nexusConfig),
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithNexusEnabled(true),
		cardanofw.WithUserCnt(userCnt),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	user := apex.Users[userCnt-1]

	fmt.Println("prime user addr: ", user.PrimeAddress)
	fmt.Println("nexus user addr: ", user.NexusAddress)
	fmt.Println("vector user addr: ", user.VectorAddress)
	fmt.Println("prime multisig addr: ", apex.PrimeInfo.MultisigAddr)
	fmt.Println("prime fee addr: ", apex.PrimeInfo.FeeAddr)
	fmt.Println("nexus gateway addr ", apex.NexusInfo.GatewayAddress)
	fmt.Println("vector multisig addr: ", apex.VectorInfo.MultisigAddr)
	fmt.Println("vector fee addr: ", apex.VectorInfo.FeeAddr)

	testCases := []struct {
		name       string
		sendAmount *big.Int
		fromChain  cardanofw.ChainID
		toChain    cardanofw.ChainID
		fundAmount *big.Int
	}{
		{
			name:       "From nexus to prime - not enough funds",
			sendAmount: cardanofw.ApexToWei(big.NewInt(5)),
			fromChain:  cardanofw.ChainIDNexus,
			toChain:    cardanofw.ChainIDPrime,
			fundAmount: fundAmount,
		},
		{
			name:       "From prime to nexus - not enough funds",
			sendAmount: cardanofw.ApexToWei(big.NewInt(15)),
			fromChain:  cardanofw.ChainIDPrime,
			toChain:    cardanofw.ChainIDNexus,
			fundAmount: fundAmount,
		},
		{
			name:       "From nexus to vector - not enough funds",
			sendAmount: cardanofw.ApexToWei(big.NewInt(5)),
			fromChain:  cardanofw.ChainIDNexus,
			toChain:    cardanofw.ChainIDVector,
			fundAmount: fundAmount,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			prevBalance, err := apex.GetBalance(ctx, user, tc.toChain)
			require.NoError(t, err)

			tokensInfo, err := apex.GetBridgingTokensInfo(tc.toChain, tc.fromChain, cardanofw.AP3XTokenID)
			require.NoError(t, err)

			prevAmount := prevBalance[cardanowallet.AdaTokenName]

			fmt.Printf("prevAmount %v\n", prevAmount)

			expectedAmount := new(big.Int).Set(tc.sendAmount)
			expectedAmount = expectedAmount.Add(expectedAmount, prevAmount)

			txHash, err := apex.SubmitBridgingRequest(cardanofw.SubmitBridgingRequestData{
				Context:          ctx,
				SourceChain:      tc.fromChain,
				DestinationChain: tc.toChain,
				Sender:           user,
				WeiAmount:        tc.sendAmount,
				SrcTokenID:       cardanofw.AP3XTokenID,
				TokensInfo:       tokensInfo,
				Receivers:        []*cardanofw.TestApexUser{user},
			})
			require.NoError(t, err)

			fmt.Printf("Tx sent. hash: %s. %v - expectedAmount\n", txHash, expectedAmount)

			err = apex.WaitForExactAmount(ctx, user, tc.toChain, expectedAmount, 20, time.Second*10, tokensInfo.DstTokenName)
			require.Error(t, err)

			require.NoError(t, apex.FundChainHotWallet(ctx, tc.toChain, tc.fundAmount))

			fmt.Printf("Funded %s with %v\n", tc.toChain, tc.fundAmount)

			err = apex.WaitForExactAmount(ctx, user, tc.toChain, expectedAmount, 30, time.Second*20, tokensInfo.DstTokenName)
			require.NoError(t, err)
		})
	}
}

func TestE2E_ApexBridgeWithNexus_PrimeGoesDownAndThenUp(t *testing.T) {
	t.Skip()

	const (
		apiKey = "test_api_key"
	)

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	apex := cardanofw.SetupAndRunApexBridge(
		t, ctx, cardanofw.SystemIDReactor,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithNexusEnabled(true),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	user := apex.Users[0]
	sendAmount := cardanofw.ApexToWei(big.NewInt(1))

	// execute nexus to prime -> no wait
	prevAmountPrime, err := apex.GetBalance(ctx, user, cardanofw.ChainIDPrime)
	require.NoError(t, err)

	tokensInfo, err := apex.GetBridgingTokensInfo(cardanofw.ChainIDPrime, cardanofw.ChainIDNexus, cardanofw.AP3XTokenID)
	require.NoError(t, err)

	// give time to oracle to submit hot wallet increment claims
	select {
	case <-ctx.Done():
		return
	case <-time.After(60 * time.Second):
	}

	txHash, err := apex.SubmitBridgingRequest(
		cardanofw.SubmitBridgingRequestData{
			Context:          ctx,
			SourceChain:      cardanofw.ChainIDNexus,
			DestinationChain: cardanofw.ChainIDPrime,
			Sender:           user,
			WeiAmount:        sendAmount,
			SrcTokenID:       cardanofw.AP3XTokenID,
			TokensInfo:       tokensInfo,
			Receivers:        []*cardanofw.TestApexUser{user},
		})
	require.NoError(t, err)

	fmt.Printf("Submitted bridging request from Nexus to Prime, txHash: %s\n", txHash)

	// close prime chain for some time
	primeChainServer := apex.GetChainMust(t, cardanofw.ChainIDPrime).GetServerMust(t, 1)

	require.NoError(t, primeChainServer.Stop(true))

	select {
	case <-ctx.Done():
		return
	case <-time.After(720 * time.Second):
	}

	// start prime chain again
	require.NoError(t, primeChainServer.Start())

	// wait for tx on destination
	expectedAmount := new(big.Int).Add(prevAmountPrime[cardanowallet.AdaTokenName], sendAmount)

	err = apex.WaitForExactAmount(
		ctx, user, cardanofw.ChainIDPrime, expectedAmount, 100, time.Second*10, tokensInfo.DstTokenName)
	require.NoError(t, err)

	fmt.Printf("Expected amount on Prime received\n")

	// send prime -> nexus
	e2ehelper.ExecuteBridging(
		t, ctx, apex, 1,
		[]*cardanofw.TestApexUser{user},
		[]*cardanofw.TestApexUser{user},
		[]string{cardanofw.ChainIDPrime},
		map[string][]string{
			cardanofw.ChainIDPrime: {cardanofw.ChainIDNexus},
		},
		map[e2ehelper.SrcDstChainPair]uint16{
			e2ehelper.NewChainPair(cardanofw.ChainIDPrime, cardanofw.ChainIDNexus): cardanofw.AP3XTokenID,
		},
		sendAmount)
}

func SrcNexusSequentialAndParallelWithMaxReceivers(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, dstChain string,
	instances, parallelInstances int, sendAmount *big.Int, options ...e2ehelper.ExecuteBridgingOption,
) {
	t.Helper()

	const (
		receivers = 4
	)

	e2ehelper.ExecuteBridging(
		t, ctx, apex, instances,
		apex.Users[:parallelInstances],
		apex.Users[len(apex.Users)-receivers:],
		[]string{cardanofw.ChainIDNexus},
		map[string][]string{
			cardanofw.ChainIDNexus: {dstChain},
		},
		map[e2ehelper.SrcDstChainPair]uint16{
			e2ehelper.NewChainPair(cardanofw.ChainIDNexus, dstChain): cardanofw.AP3XTokenID,
		},
		sendAmount,
		options...)
}

func SrcNexusSubmitterNotEnoughFunds(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, dstChain string,
) {
	t.Helper()

	nexusChain := apex.GetChainMust(t, cardanofw.ChainIDNexus).(*cardanofw.TestEVMChain)

	fee := cardanofw.WeiToChainNativeTokenAmount(
		cardanofw.ChainIDNexus, apex.GetMinBridgingFee(cardanofw.ChainIDNexus, false))
	sendAmount := cardanofw.ApexToWei(big.NewInt(2))

	unfundedUser, err := cardanofw.NewTestApexUser(cardanofw.NewApexNetworkTypesFromSystem(apex))
	require.NoError(t, err)

	tokenInfo, err := apex.GetBridgingTokensInfo(cardanofw.ChainIDNexus,
		dstChain,
		cardanofw.AP3XTokenID)
	require.NoError(t, err)

	unfundedUserPk, err := unfundedUser.GetPrivateKey(cardanofw.ChainIDNexus)
	require.NoError(t, err)

	txHash, err := nexusChain.DirectBridgingRequest(
		cardanofw.ChainIDToInt(dstChain),
		unfundedUserPk,
		map[string]cardanofw.ReceiverAmount{
			unfundedUser.GetAddress(dstChain): {
				TokenID: cardanofw.AP3XTokenID,
				Amount:  sendAmount,
			},
		},
		fee,
		big.NewInt(0),
		tokenInfo.SrcTokenName,
		true,
	)

	require.Equal(t, "", txHash)
	require.Error(t, err)
}

func DstNexusSequentialAndParallelWithMaxReceivers(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, srcChain string,
	sequentialInstances, parallelInstances int, sendAmountDfm *big.Int, options ...e2ehelper.ExecuteBridgingOption,
) {
	t.Helper()

	const (
		receivers = 4
	)

	e2ehelper.ExecuteBridging(
		t, ctx, apex,
		sequentialInstances,
		apex.Users[:parallelInstances],
		apex.Users[len(apex.Users)-receivers:],
		[]string{srcChain},
		map[string][]string{
			srcChain: {cardanofw.ChainIDNexus},
		},
		map[e2ehelper.SrcDstChainPair]uint16{
			e2ehelper.NewChainPair(srcChain, cardanofw.ChainIDNexus): cardanofw.AP3XTokenID,
		},
		sendAmountDfm,
		options...)
}

func DstNexusBothDirectionsSequentialAndParallel(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, srcChain string, receiverUser *cardanofw.TestApexUser,
	sequentialInstances, parallelInstances int, sendAmountDfm *big.Int, options ...e2ehelper.ExecuteBridgingOption,
) {
	t.Helper()

	const ()

	e2ehelper.ExecuteBridging(
		t, ctx, apex,
		sequentialInstances,
		apex.Users[:parallelInstances],
		[]*cardanofw.TestApexUser{receiverUser},
		[]string{srcChain, cardanofw.ChainIDNexus},
		map[string][]string{
			srcChain:               {cardanofw.ChainIDNexus},
			cardanofw.ChainIDNexus: {srcChain},
		},
		map[e2ehelper.SrcDstChainPair]uint16{
			e2ehelper.NewChainPair(srcChain, cardanofw.ChainIDNexus): cardanofw.AP3XTokenID,
			e2ehelper.NewChainPair(cardanofw.ChainIDNexus, srcChain): cardanofw.AP3XTokenID,
		},
		sendAmountDfm,
		options...)
}

func DstNexusSubmitterNotEnoughFunds(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, srcChain string, user *cardanofw.TestApexUser,
	sendAmount *big.Int,
) {
	t.Helper()

	dstChain := cardanofw.ChainIDNexus
	receiverAddr := apex.PrimeInfo.MultisigAddr[0]

	operationFee := apex.GetMinOperationFee(srcChain)
	minBridgingFee := apex.GetMinBridgingFee(srcChain, false)

	tokensInfo, err := apex.GetBridgingTokensInfo(srcChain, dstChain, cardanofw.AP3XTokenID)
	require.NoError(t, err)

	receivers := []sendtx.BridgingTxReceiver{
		{
			Addr:    user.GetAddress(dstChain),
			Amount:  cardanofw.WeiToDfm(sendAmount).Uint64(),
			TokenID: tokensInfo.SrcTokenID,
		},
	}

	metadata, err := apex.GetChainMust(t, srcChain).CreateMetadata(
		user.GetAddress(srcChain), dstChain,
		receivers, minBridgingFee, operationFee)
	require.NoError(t, err)

	_, err = apex.SubmitTx(
		ctx, srcChain, user, receiverAddr, sendAmount, nil, metadata, nil)

	require.Error(t, err)
	require.ErrorContains(t, err, "couldn't select UTXOs")
}

func DstNexusInvalidMetadataSlicedOff(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, srcChain string, user *cardanofw.TestApexUser,
	sendAmount *big.Int,
) {
	t.Helper()

	dstChain := cardanofw.ChainIDNexus
	receiverAddr := apex.PrimeInfo.MultisigAddr[0]

	operationFee := apex.GetMinOperationFee(srcChain)
	minBridgingFee := apex.GetMinBridgingFee(srcChain, false)

	tokensInfo, err := apex.GetBridgingTokensInfo(srcChain, dstChain, cardanofw.AP3XTokenID)
	require.NoError(t, err)

	receivers := []sendtx.BridgingTxReceiver{
		{
			Addr:    user.GetAddress(dstChain),
			Amount:  cardanofw.WeiToDfm(sendAmount).Uint64() * 10,
			TokenID: tokensInfo.SrcTokenID,
		},
	}

	metadata, err := apex.GetChainMust(t, srcChain).CreateMetadata(
		user.GetAddress(srcChain), dstChain,
		receivers, minBridgingFee, operationFee)
	require.NoError(t, err)

	// Send only half bytes of metadata making it invalid
	metadata = metadata[0 : len(metadata)/2]

	_, err = apex.SubmitTx(
		ctx, srcChain, user, receiverAddr, sendAmount, nil, metadata, nil)
	require.Error(t, err)
}

func DstNexusInvalidMetadataWrongType(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, srcChain string, user *cardanofw.TestApexUser,
	requestStateTimeoutSec uint, sendAmount *big.Int,
) {
	t.Helper()

	dstChain := cardanofw.ChainIDNexus
	receiverAddr := apex.PrimeInfo.MultisigAddr[0]

	operationFee := apex.GetMinOperationFee(srcChain)
	minBridgingFee := apex.GetMinBridgingFee(srcChain, false)

	tokensInfo, err := apex.GetBridgingTokensInfo(srcChain, dstChain, cardanofw.AP3XTokenID)
	require.NoError(t, err)

	metadata, err := apex.GetChainMust(t, srcChain).CreateMetadata(
		user.GetAddress(srcChain), dstChain,
		[]sendtx.BridgingTxReceiver{
			{
				Addr:    user.GetAddress(dstChain),
				Amount:  cardanofw.WeiToDfm(sendAmount).Uint64() * 10,
				TokenID: tokensInfo.SrcTokenID,
			},
		}, minBridgingFee, operationFee)
	require.NoError(t, err)

	bridgingRequestMetadata := bytes.Replace(metadata, []byte("bridge"), []byte("xxxxx"), 1)
	beforeSendingAmount, err := apex.GetBalance(ctx, user, cardanofw.ChainIDPrime)
	require.NoError(t, err)

	txHash, err := apex.SubmitTx(
		ctx, srcChain, user, receiverAddr,
		new(big.Int).Add(sendAmount, minBridgingFee), nil, bridgingRequestMetadata, nil)
	require.NoError(t, err)

	lowerBoundary := new(big.Int).Sub(beforeSendingAmount[cardanowallet.AdaTokenName], new(big.Int).Add(sendAmount, minBridgingFee))

	fmt.Printf("Tx sent. hash: %s, lowerBoundaryDfm: %d, higherBoundaryDfm: %+v\n", txHash, lowerBoundary, beforeSendingAmount)

	err = apex.WaitForAmountInRange(ctx, user, cardanofw.ChainIDPrime, lowerBoundary, beforeSendingAmount[cardanowallet.AdaTokenName],
		50, time.Second*30, tokensInfo.DstTokenName)
	require.NoError(t, err)
}

func DstNexusInvalidMetadataInvalidDestination(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, srcChain string, user *cardanofw.TestApexUser,
	invalidStateTimeoutSec uint, sendAmount *big.Int,
) {
	t.Helper()

	dstChain := cardanofw.ChainIDNexus

	receiverAddr := apex.PrimeInfo.MultisigAddr[0]
	if srcChain == cardanofw.ChainIDVector {
		receiverAddr = apex.VectorInfo.MultisigAddr[0]
	}

	operationFee := apex.GetMinOperationFee(srcChain)
	minBridgingFee := apex.GetMinBridgingFee(srcChain, false)

	tokensInfo, err := apex.GetBridgingTokensInfo(srcChain, dstChain, cardanofw.AP3XTokenID)
	require.NoError(t, err)

	metadata, err := apex.GetChainMust(t, srcChain).CreateMetadata(
		user.GetAddress(srcChain), dstChain,
		[]sendtx.BridgingTxReceiver{
			{
				Addr:    user.GetAddress(dstChain),
				Amount:  cardanofw.WeiToDfm(sendAmount).Uint64() * 10,
				TokenID: tokensInfo.SrcTokenID,
			},
		}, minBridgingFee, operationFee)
	require.NoError(t, err)

	bridgingRequestMetadata := bytes.Replace(metadata,
		[]byte(fmt.Sprintf("\"%s\"", dstChain)), []byte("\"hector\""), 1)

	beforeSendingAmount, err := apex.GetBalance(ctx, user, cardanofw.ChainIDPrime)
	require.NoError(t, err)

	txHash, err := apex.SubmitTx(
		ctx, srcChain, user, receiverAddr,
		new(big.Int).Add(sendAmount, minBridgingFee), nil, bridgingRequestMetadata, nil)
	require.NoError(t, err)

	lowerBoundary := new(big.Int).Sub(beforeSendingAmount[cardanowallet.AdaTokenName], new(big.Int).Add(sendAmount, minBridgingFee))

	fmt.Printf("Tx sent. hash: %s, lowerBoundaryDfm: %d, higherBoundaryDfm: %+v\n", txHash, lowerBoundary, beforeSendingAmount)

	err = apex.WaitForAmountInRange(ctx, user, cardanofw.ChainIDPrime, lowerBoundary, beforeSendingAmount[cardanowallet.AdaTokenName],
		50, time.Second*30, cardanowallet.AdaTokenName)
	require.NoError(t, err)
}

func DstNexusInvalidMetadataInvalidSender(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, srcChain string, user *cardanofw.TestApexUser,
	invalidStateTimeoutSec uint, sendAmount *big.Int,
) {
	t.Helper()

	dstChain := cardanofw.ChainIDNexus

	receiverAddr := apex.PrimeInfo.MultisigAddr[0]
	if srcChain == cardanofw.ChainIDVector {
		receiverAddr = apex.VectorInfo.MultisigAddr[0]
	}

	operationFee := apex.GetMinOperationFee(srcChain)
	minBridgingFee := apex.GetMinBridgingFee(srcChain, false)

	tokensInfo, err := apex.GetBridgingTokensInfo(srcChain, dstChain, cardanofw.AP3XTokenID)
	require.NoError(t, err)

	metadata, err := apex.GetChainMust(t, srcChain).CreateMetadata(
		"dummy", dstChain,
		[]sendtx.BridgingTxReceiver{
			{
				Addr:    user.GetAddress(dstChain),
				Amount:  cardanofw.WeiToDfm(sendAmount).Uint64() * 10,
				TokenID: tokensInfo.SrcTokenID,
			},
		}, minBridgingFee, operationFee)
	require.NoError(t, err)

	// remove this after we make correct validation on oracle!
	bridgingRequestMetadata := bytes.Replace(metadata,
		[]byte("[\"dummy\"]"), []byte("\"\""), 1)

	txHash, err := apex.SubmitTx(
		ctx, srcChain, user, receiverAddr,
		new(big.Int).Add(sendAmount, minBridgingFee), nil, bridgingRequestMetadata, nil)
	require.NoError(t, err)

	fmt.Printf("Tx sent. hash: %s\n", txHash)

	cardanofw.WaitForInvalidState(t, ctx, apex, srcChain, txHash, apex.Config.APIKey, invalidStateTimeoutSec)
}

func DstNexusInvalidMetadataInvalidTransactions(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, srcChain string, user *cardanofw.TestApexUser,
	invalidStateTimeoutSec uint, sendAmount *big.Int,
) {
	t.Helper()

	dstChain := cardanofw.ChainIDNexus

	receiverAddr := apex.PrimeInfo.MultisigAddr[0]
	if srcChain == cardanofw.ChainIDVector {
		receiverAddr = apex.VectorInfo.MultisigAddr[0]
	}

	operationFee := apex.GetMinOperationFee(srcChain)
	minBridgingFee := apex.GetMinBridgingFee(srcChain, false)

	metadata, err := apex.GetChainMust(t, srcChain).CreateMetadata(
		user.GetAddress(srcChain), dstChain,
		[]sendtx.BridgingTxReceiver{},
		minBridgingFee, operationFee)
	require.NoError(t, err)

	beforeSendingAmount, err := apex.GetBalance(ctx, user, cardanofw.ChainIDPrime)
	require.NoError(t, err)

	txHash, err := apex.SubmitTx(
		ctx, srcChain, user, receiverAddr,
		new(big.Int).Add(sendAmount, minBridgingFee), nil, metadata, nil)
	require.NoError(t, err)

	lowerBoundary := new(big.Int).Sub(beforeSendingAmount[cardanowallet.AdaTokenName], new(big.Int).Add(sendAmount, minBridgingFee))

	fmt.Printf("Tx sent. hash: %s, lowerBoundaryDfm: %d, higherBoundaryDfm: %+v\n", txHash, lowerBoundary, beforeSendingAmount)

	tokensInfo, err := apex.GetBridgingTokensInfo(cardanofw.ChainIDPrime, cardanofw.ChainIDNexus, cardanofw.AP3XTokenID)
	require.NoError(t, err)

	err = apex.WaitForAmountInRange(ctx, user, cardanofw.ChainIDPrime, lowerBoundary, beforeSendingAmount[cardanowallet.AdaTokenName],
		50, time.Second*30, tokensInfo.DstTokenName)
	require.NoError(t, err)
}

func misconfiguredMinAmounts(url, key, contractAddr string, minBridgingFee, minBridgingAmount, minBridgingTokenAmount, minOperationFee *big.Int) error {
	args := []string{
		"bridge-admin",
		"set-min-amounts",
		"--url", url,
		"--key", key,
		"--contract-addr", contractAddr,
	}

	if minBridgingFee != nil {
		args = append(args, "--min-fee", minBridgingFee.String())
	}

	if minBridgingAmount != nil {
		args = append(args, "--min-bridging-amount", minBridgingAmount.String())
	}

	if minBridgingTokenAmount != nil {
		args = append(args, "--min-token-bridging-amount", minBridgingTokenAmount.String())
	}

	if minOperationFee != nil {
		args = append(args, "--min-operation-fee", minOperationFee.String())
	}

	return cardanofw.RunCommand(
		cardanofw.ResolveApexBridgeBinary(),
		args,
		os.Stdout,
	)
}

func TestE2E_ApexBridgeWithNexus_NexusGoesDownAndThenUp(t *testing.T) {
	if cardanofw.ShouldSkipE2RRedundantTests() {
		t.Skip()
	}

	const (
		apiKey = "test_api_key"
	)

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	apex := cardanofw.SetupAndRunReactorBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithNexusEnabled(true),
		cardanofw.WithTargetOneClusterServer(true),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	user := apex.Users[0]
	sendAmount := cardanofw.ApexToWei(big.NewInt(1))

	// execute prime to nexus -> no wait
	prevAmountNexus, err := apex.GetBalance(ctx, user, cardanofw.ChainIDNexus)
	require.NoError(t, err)

	tokensInfo, err := apex.GetBridgingTokensInfo(cardanofw.ChainIDNexus, cardanofw.ChainIDPrime, cardanofw.AP3XTokenID)
	require.NoError(t, err)

	// give time to oracle to submit hot wallet increment claims
	select {
	case <-ctx.Done():
		return
	case <-time.After(60 * time.Second):
	}

	txHash, err := apex.SubmitBridgingRequest(cardanofw.SubmitBridgingRequestData{
		Context:          ctx,
		SourceChain:      cardanofw.ChainIDPrime,
		DestinationChain: cardanofw.ChainIDNexus,
		Sender:           user,
		WeiAmount:        sendAmount,
		SrcTokenID:       cardanofw.AP3XTokenID,
		TokensInfo:       tokensInfo,
		Receivers:        []*cardanofw.TestApexUser{user},
	})
	require.NoError(t, err)

	fmt.Printf("Submitted bridging request from Prime to Nexus, txHash: %s\n", txHash)

	// close nexus chain for some time
	nexusChainServer := apex.GetChainMust(t, cardanofw.ChainIDNexus).GetServerMust(t, 0)

	require.NoError(t, nexusChainServer.Stop())

	select {
	case <-ctx.Done():
		return
	case <-time.After(360 * time.Second):
	}

	// start nexus chain again
	require.NoError(t, nexusChainServer.Start())

	// wait for tx on destination
	expectedAmount := new(big.Int).Add(prevAmountNexus[cardanowallet.AdaTokenName], sendAmount)

	err = apex.WaitForExactAmount(ctx, user, cardanofw.ChainIDNexus, expectedAmount, 100, time.Second*10, tokensInfo.DstTokenName)
	require.NoError(t, err)

	// send nexus -> prime
	e2ehelper.ExecuteBridging(
		t, ctx, apex, 1,
		[]*cardanofw.TestApexUser{user},
		[]*cardanofw.TestApexUser{user},
		[]string{cardanofw.ChainIDNexus},
		map[string][]string{
			cardanofw.ChainIDNexus: {cardanofw.ChainIDPrime},
		},
		map[e2ehelper.SrcDstChainPair]uint16{
			e2ehelper.NewChainPair(cardanofw.ChainIDNexus, cardanofw.ChainIDPrime): cardanofw.AP3XTokenID,
		},
		sendAmount)
}
