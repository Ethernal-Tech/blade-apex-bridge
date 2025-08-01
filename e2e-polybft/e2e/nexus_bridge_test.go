package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/e2e-polybft/cardanofw"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/e2ehelper"
	"github.com/Ethernal-Tech/ethgo"
	"github.com/stretchr/testify/require"
)

func TestE2E_ApexBridgeWithNexus(t *testing.T) {
	if cardanofw.ShouldSkipE2RRedundantTests() {
		t.Skip()
	}

	const (
		apiKey = "test_api_key"
	)

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	apex := cardanofw.SetupAndRunApexBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithVectorEnabled(false),
		cardanofw.WithNexusEnabled(true),
		cardanofw.WithUserCnt(1),
	)
	sendAmountDfm := cardanofw.WeiToDfm(ethgo.Ether(1))

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	t.Run("From Nexus to Prime", func(t *testing.T) {
		srcChain, dstChain := cardanofw.ChainIDNexus, cardanofw.ChainIDPrime

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, apex.Users[0], apex.Users[0], srcChain, dstChain, sendAmountDfm)
	})

	t.Run("From Prime to Nexus", func(t *testing.T) {
		srcChain, dstChain := cardanofw.ChainIDPrime, cardanofw.ChainIDNexus

		relayerBalanceBefore, err := apex.GetChainMust(t, dstChain).GetAddressBalance(
			ctx, apex.NexusInfo.RelayerAddress.String())
		require.NoError(t, err)

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, apex.Users[0], apex.Users[0], srcChain, dstChain, sendAmountDfm)

		relayerBalanceAfter, err := apex.GetChainMust(t, dstChain).GetAddressBalance(
			ctx, apex.NexusInfo.RelayerAddress.String())
		require.NoError(t, err)

		require.True(t, relayerBalanceAfter.Cmp(relayerBalanceBefore) == 1)
	})
}

func TestE2E_ApexBridgeWithNexus_NtP_ValidScenarios(t *testing.T) {
	if cardanofw.ShouldSkipE2RRedundantTests() {
		t.Skip()
	}

	const (
		apiKey  = "test_api_key"
		userCnt = 15
	)

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	apex := cardanofw.SetupAndRunApexBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithVectorEnabled(false),
		cardanofw.WithNexusEnabled(true),
		cardanofw.WithUserCnt(userCnt),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	sendAmountDfm := cardanofw.WeiToDfm(ethgo.Ether(1))
	user := apex.Users[userCnt-1]
	srcChain := cardanofw.ChainIDNexus
	dstChain := cardanofw.ChainIDPrime

	t.Run("From Nexus to Prime one by one - wait for other side", func(t *testing.T) {
		const instances = 5

		e2ehelper.ExecuteBridgingOneByOneWaitOnOtherSide(
			t, ctx, apex, instances, user, srcChain, dstChain, sendAmountDfm)
	})

	t.Run("From Nexus to Prime one by one - don't wait", func(t *testing.T) {
		const instances = 5

		e2ehelper.ExecuteBridgingWaitAfterSubmits(
			t, ctx, apex, instances, user, srcChain, dstChain, sendAmountDfm)
	})

	t.Run("From Nexus to Prime - parallel", func(t *testing.T) {
		const instances = 5

		e2ehelper.ExecuteBridging(
			t, ctx, apex, 1,
			apex.Users[:instances],
			[]*cardanofw.TestApexUser{user},
			[]string{srcChain},
			map[string][]string{
				srcChain: {dstChain},
			},
			sendAmountDfm)
	})

	t.Run("From Nexus to Prime - sequential and parallel", func(t *testing.T) {
		const (
			instances         = 5
			parallelInstances = 10
		)

		e2ehelper.ExecuteBridging(
			t, ctx, apex, instances,
			apex.Users[:parallelInstances],
			[]*cardanofw.TestApexUser{user},
			[]string{srcChain},
			map[string][]string{
				srcChain: {dstChain},
			},
			sendAmountDfm)
	})

	t.Run("From Nexus to Prime - sequential and parallel multiple receivers", func(t *testing.T) {
		const (
			sequentialInstances = 5
			parallelInstances   = 10
		)

		NexusToPrimeSequentialAndParallelWithMaxReceivers(
			t, ctx, apex, sequentialInstances, parallelInstances, sendAmountDfm)
	})

	t.Run("From Nexus to Prime - sequential and parallel, one node goes off in the middle", func(t *testing.T) {
		const (
			instances            = 5
			parallelInstances    = 10
			stopAfter            = time.Second * 60
			validatorStoppingIdx = 1
		)

		e2ehelper.ExecuteBridging(
			t, ctx, apex, instances,
			apex.Users[:parallelInstances],
			apex.Users[len(apex.Users)-1:],
			[]string{srcChain},
			map[string][]string{
				srcChain: {dstChain},
			},
			sendAmountDfm,
			e2ehelper.WithRestartValidatorsConfig([]e2ehelper.RestartValidatorsConfig{
				{WaitTime: stopAfter, StopIndxs: []int{validatorStoppingIdx}},
			}))
	})
}

func TestE2E_ApexBridgeWithNexus_NtP_InvalidScenarios(t *testing.T) {
	const (
		apiKey  = "test_api_key"
		userCnt = 1
	)

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	apex := cardanofw.SetupAndRunApexBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithVectorEnabled(false),
		cardanofw.WithNexusEnabled(true),
		cardanofw.WithUserCnt(userCnt),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	user := apex.Users[userCnt-1]
	fee := cardanofw.DfmToChainNativeTokenAmount(cardanofw.ChainIDNexus, new(big.Int).SetUint64(uint64(1_100_000)))

	nexusAdminUser := &cardanofw.TestApexUser{
		NexusWallet:    apex.NexusInfo.AdminKey,
		NexusAddress:   apex.NexusInfo.AdminKey.Address(),
		HasNexusWallet: true,
	}

	t.Run("Wrong Tx-Type", func(t *testing.T) {
		sendAmountWei := ethgo.Ether(uint64(1))

		userPk, err := user.GetPrivateKey(cardanofw.ChainIDNexus)
		require.NoError(t, err)

		// call SendTx command
		err = sendTxParamsNPInvalidScenarios("cardano", // "cardano" instead of "evm"
			apex.NexusInfo.GatewayAddress.String(),
			apex.NexusInfo.JSONRPCAddr,
			userPk, cardanofw.ChainIDPrime,
			user.GetAddress(cardanofw.ChainIDPrime),
			sendAmountWei, fee,
		)
		require.ErrorContains(t, err, "failed to execute command")
	})

	t.Run("Wrong Nexus URL", func(t *testing.T) {
		sendAmountWei := ethgo.Ether(uint64(1))

		userPk, err := user.GetPrivateKey(cardanofw.ChainIDNexus)
		require.NoError(t, err)

		// call SendTx command
		err = sendTxParamsNPInvalidScenarios("evm",
			apex.NexusInfo.GatewayAddress.String(),
			"localhost:1234",
			userPk, cardanofw.ChainIDPrime,
			user.GetAddress(cardanofw.ChainIDPrime),
			sendAmountWei, fee,
		)
		require.ErrorContains(t, err, "Error: invalid --nexus-url flag")
	})

	t.Run("Submitter not enough funds", func(t *testing.T) {
		NexusToPrimeSubmitterNotEnoughFunds(t, ctx, apex)
	})

	t.Run("Big receiver amount", func(t *testing.T) {
		unfundedUser, err := cardanofw.NewTestApexUser(
			apex.Config.PrimeConfig.NetworkType,
			apex.Config.VectorConfig.IsEnabled,
			apex.Config.VectorConfig.NetworkType,
			apex.Config.NexusConfig.IsEnabled,
		)
		require.NoError(t, err)

		unfundedUserPk, err := unfundedUser.GetPrivateKey(cardanofw.ChainIDNexus)
		require.NoError(t, err)

		_, err = apex.SubmitTx(
			ctx, cardanofw.ChainIDNexus, nexusAdminUser, unfundedUser.NexusAddress.String(), big.NewInt(10), nil)
		require.NoError(t, err)

		sendAmountWei := ethgo.Ether(uint64(20)) // try to send 20 ethers with users without enough funds

		// call SendTx command
		err = sendTxParamsNPInvalidScenarios("evm",
			apex.NexusInfo.GatewayAddress.String(),
			apex.NexusInfo.JSONRPCAddr,
			unfundedUserPk, cardanofw.ChainIDPrime,
			unfundedUser.GetAddress(cardanofw.ChainIDPrime),
			sendAmountWei, fee,
		)
		require.ErrorContains(t, err, "insufficient funds for execution")
	})
}

func TestE2E_ApexBridgeWithNexus_PtNandBoth_ValidScenarios(t *testing.T) {
	const (
		apiKey  = "test_api_key"
		userCnt = 15
	)

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	primeConfig := cardanofw.NewPrimeChainConfig()
	primeConfig.UseIndexer = true

	apex := cardanofw.SetupAndRunApexBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithVectorEnabled(false),
		cardanofw.WithNexusEnabled(true),
		cardanofw.WithUserCnt(userCnt),
		cardanofw.WithPrimeConfig(primeConfig),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	user := apex.Users[userCnt-1]
	sendAmountDfm := cardanofw.WeiToDfm(ethgo.Ether(1))

	t.Run("From Prime to Nexus one by one - wait for other side", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() {
			t.Skip()
		}

		t.Cleanup(func() {
			apex.ResetIndexers()
		})

		const instances = 5

		e2ehelper.ExecuteBridgingOneByOneWaitOnOtherSide(
			t, ctx, apex, instances, user, cardanofw.ChainIDPrime, cardanofw.ChainIDNexus, sendAmountDfm)
	})

	t.Run("From Prime to Nexus one by one - don't wait for other side", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() {
			t.Skip()
		}

		t.Cleanup(func() {
			apex.ResetIndexers()
		})

		const instances = 5

		e2ehelper.ExecuteBridgingWaitAfterSubmits(
			t, ctx, apex, instances, user, cardanofw.ChainIDPrime, cardanofw.ChainIDNexus, sendAmountDfm)
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
			sendAmountDfm)
	})

	t.Run("From Prime to Nexus sequential and parallel", func(t *testing.T) {
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
			[]string{cardanofw.ChainIDPrime},
			map[string][]string{
				cardanofw.ChainIDPrime: {cardanofw.ChainIDNexus},
			},
			sendAmountDfm)
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

		PrimeToNexusSequentialAndParallelWithMaxReceivers(
			t, ctx, apex, sequentialInstances, parallelInstances, sendAmountDfm)
	})

	t.Run("From Prime to Nexus sequential and parallel - one node goes off in the midle", func(t *testing.T) {
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
			[]string{cardanofw.ChainIDPrime},
			map[string][]string{
				cardanofw.ChainIDPrime: {cardanofw.ChainIDNexus},
			},
			sendAmountDfm,
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

		const instances = 5

		e2ehelper.ExecuteBridging(
			t, ctx, apex,
			instances,
			apex.Users[:1],
			[]*cardanofw.TestApexUser{user},
			[]string{cardanofw.ChainIDPrime, cardanofw.ChainIDNexus},
			map[string][]string{
				cardanofw.ChainIDPrime: {cardanofw.ChainIDNexus},
				cardanofw.ChainIDNexus: {cardanofw.ChainIDPrime},
			},
			sendAmountDfm)
	})

	t.Run("Both directions sequential and parallel", func(t *testing.T) {
		t.Cleanup(func() {
			apex.ResetIndexers()
		})

		const (
			sequentialInstances = 5
			parallelInstances   = 6
		)

		PrimeNexusBothDirectionsSequentialAndParallel(
			t, ctx, apex, user, sequentialInstances, parallelInstances, sendAmountDfm)
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
			sendAmountDfm,
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
			[]string{cardanofw.ChainIDPrime, cardanofw.ChainIDNexus},
			map[string][]string{
				cardanofw.ChainIDPrime: {cardanofw.ChainIDNexus},
				cardanofw.ChainIDNexus: {cardanofw.ChainIDPrime},
			},
			sendAmountDfm,
			e2ehelper.WithWaitForUnexpectedBridges(true),
			e2ehelper.WithRestartValidatorsConfig([]e2ehelper.RestartValidatorsConfig{
				{WaitTime: stopAfter, StopIndxs: []int{validatorStoppingIdx1, validatorStoppingIdx2}},
				{WaitTime: startAgainAfter, StartIndxs: []int{validatorStoppingIdx1}},
			}))
	})
}

func TestE2E_ApexBridgeWithNexus_PtN_InvalidScenarios(t *testing.T) {
	const (
		apiKey  = "test_api_key"
		userCnt = 15
	)

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	const premineAmount = uint64(50_000_000)

	primeConfig := cardanofw.NewPrimeChainConfig()
	primeConfig.PremineAmount = premineAmount

	apex := cardanofw.SetupAndRunApexBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithVectorEnabled(false),
		cardanofw.WithNexusEnabled(true),
		cardanofw.WithPrimeConfig(primeConfig),
		cardanofw.WithUserCnt(userCnt),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	user := apex.Users[userCnt-1]

	t.Run("Submitter not enough funds", func(t *testing.T) {
		sendAmountDfm := cardanofw.WeiToDfm(ethgo.Ether(100))

		PrimeToNexusSubmitterNotEnoughFunds(t, ctx, apex, user, sendAmountDfm)
	})

	t.Run("Submitted invalid metadata - sliced off", func(t *testing.T) {
		PrimeToNexusInvalidMetadataSlicedOff(t, ctx, apex, user)
	})

	t.Run("Submitted invalid metadata - wrong type", func(t *testing.T) {
		PrimeToNexusInvalidMetadataWrongType(t, ctx, apex, user, cardanofw.DefaultRequestStateTimeoutSec)
	})

	t.Run("Submitted invalid metadata - invalid destination", func(t *testing.T) {
		PrimeToNexusInvalidMetadataInvalidDestination(t, ctx, apex, user, 0)
	})

	t.Run("Submitted invalid metadata - invalid sender", func(t *testing.T) {
		PrimeToNexusInvalidMetadataInvalidSender(t, ctx, apex, user, 0)
	})

	t.Run("Submitted invalid metadata - empty tx", func(t *testing.T) {
		PrimeToNexusInvalidMetadataInvalidTransactions(t, ctx, apex, user, 0)
	})
}

func TestE2E_ApexBridgeWithNexus_BatchFailed(t *testing.T) {
	const (
		apiKey  = "test_api_key"
		userCnt = 1
	)

	sendAmountDfm := cardanofw.WeiToDfm(ethgo.Ether(1))

	t.Run("Test insufficient gas price dynamicTx=true", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() {
			t.Skip()
		}

		ctx, cncl := context.WithCancel(context.Background())
		defer cncl()

		var (
			failedToExecute int
			timeout         bool
		)

		apex := cardanofw.SetupAndRunApexBridge(
			t, ctx,
			cardanofw.WithAPIKey(apiKey),
			cardanofw.WithVectorEnabled(false),
			cardanofw.WithNexusEnabled(true),
			cardanofw.WithUserCnt(userCnt),
			cardanofw.WithCustomConfigHandlers(nil, func(_ *cardanofw.ApexSystem, mp map[string]interface{}) {
				block := cardanofw.GetMapFromInterfaceKey(mp, "chains", cardanofw.ChainIDNexus, "config")
				block["gasFeeCap"] = uint64(10)
				block["gasTipCap"] = uint64(11)
			}),
		)

		user := apex.Users[userCnt-1]

		txHash := apex.SubmitBridgingRequest(
			t, ctx, cardanofw.ChainIDPrime, cardanofw.ChainIDNexus, user, sendAmountDfm, user)

		fmt.Printf("Tx sent. hash: %s\n", txHash)

		// Check relay failed
		failedToExecute, timeout = cardanofw.WaitForBatchState(
			ctx, apex, cardanofw.ChainIDPrime, txHash, apiKey, true, false, cardanofw.BatchStateExecuted)

		require.Equal(t, failedToExecute, 1)
		require.False(t, timeout)

		// Restart relayer after config fix
		require.NoError(t, apex.StopRelayer())

		err := cardanofw.UpdateJSONFile(
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
			ctx, apex, cardanofw.ChainIDPrime, txHash, apiKey, false, false, cardanofw.BatchStateExecuted)

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

		apex := cardanofw.SetupAndRunApexBridge(
			t, ctx,
			cardanofw.WithAPIKey(apiKey),
			cardanofw.WithVectorEnabled(false),
			cardanofw.WithNexusEnabled(true),
			cardanofw.WithUserCnt(userCnt),
			cardanofw.WithCustomConfigHandlers(nil, func(_ *cardanofw.ApexSystem, mp map[string]interface{}) {
				block := cardanofw.GetMapFromInterfaceKey(mp, "chains", cardanofw.ChainIDNexus, "config")
				block["gasPrice"] = uint64(10)
				block["dynamicTx"] = bool(false)
			}),
		)

		user := apex.Users[userCnt-1]

		txHash := apex.SubmitBridgingRequest(
			t, ctx, cardanofw.ChainIDPrime, cardanofw.ChainIDNexus, user, sendAmountDfm, user)

		fmt.Printf("Tx sent. hash: %s\n", txHash)

		// Check relay failed
		failedToExecute, timeout = cardanofw.WaitForBatchState(
			ctx, apex, cardanofw.ChainIDPrime, txHash, apiKey, true, false, cardanofw.BatchStateExecuted)

		require.Equal(t, failedToExecute, 1)
		require.False(t, timeout)

		// Restart relayer after config fix
		require.NoError(t, apex.StopRelayer())

		err := cardanofw.UpdateJSONFile(
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
			ctx, apex, cardanofw.ChainIDPrime, txHash, apiKey, false, false, cardanofw.BatchStateExecuted)

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

		apex := cardanofw.SetupAndRunApexBridge(
			t, ctx,
			cardanofw.WithAPIKey(apiKey),
			cardanofw.WithVectorEnabled(false),
			cardanofw.WithNexusEnabled(true),
			cardanofw.WithUserCnt(userCnt),
			cardanofw.WithCustomConfigHandlers(nil, func(_ *cardanofw.ApexSystem, mp map[string]interface{}) {
				cardanofw.GetMapFromInterfaceKey(mp, "chains", cardanofw.ChainIDNexus, "config")["depositGasLimit"] = uint64(10)
			}),
		)

		user := apex.Users[userCnt-1]

		txHash := apex.SubmitBridgingRequest(
			t, ctx, cardanofw.ChainIDPrime, cardanofw.ChainIDNexus, user, sendAmountDfm, user)

		fmt.Printf("Tx sent. hash: %s\n", txHash)

		// Check relay failed
		failedToExecute, timeout = cardanofw.WaitForBatchState(ctx,
			apex, cardanofw.ChainIDPrime, txHash, apiKey, true, false, cardanofw.BatchStateExecuted)

		require.Equal(t, failedToExecute, 1)
		require.False(t, timeout)

		// Restart relayer after config fix
		require.NoError(t, apex.StopRelayer())

		err := cardanofw.UpdateJSONFile(
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
			apex, cardanofw.ChainIDPrime, txHash, apiKey, false, false, cardanofw.BatchStateExecuted)

		require.LessOrEqual(t, failedToExecute, 1)
		require.False(t, timeout)
	})

	//nolint:dupl
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

		apex := cardanofw.SetupAndRunApexBridge(
			t, ctx,
			cardanofw.WithAPIKey(apiKey),
			cardanofw.WithVectorEnabled(false),
			cardanofw.WithNexusEnabled(true),
			cardanofw.WithUserCnt(userCnt),
			cardanofw.WithCustomConfigHandlers(func(_ *cardanofw.ApexSystem, mp map[string]interface{}) {
				cardanofw.GetMapFromInterfaceKey(mp, "ethChains", cardanofw.ChainIDNexus)["testMode"] = uint8(1)
			}, nil),
		)

		defer require.True(t, apex.ApexBridgeProcessesRunning())

		user := apex.Users[userCnt-1]

		prevBalanceDfm, err := apex.GetBalance(ctx, user, cardanofw.ChainIDNexus)
		require.NoError(t, err)

		fmt.Printf("Dfm before Tx %d\n", prevBalanceDfm)

		expectedAmount := new(big.Int).Set(sendAmountDfm)
		expectedAmount = expectedAmount.Add(expectedAmount, prevBalanceDfm)

		txHash := apex.SubmitBridgingRequest(
			t, ctx, cardanofw.ChainIDPrime, cardanofw.ChainIDNexus, user, sendAmountDfm, user)

		fmt.Printf("Tx sent. hash: %s\n", txHash)

		// Check batch failed
		failedToExecute, timeout = cardanofw.WaitForBatchState(
			ctx, apex, cardanofw.ChainIDPrime, txHash, apiKey, false, false, cardanofw.BatchStateExecuted)

		require.Equal(t, failedToExecute, 1)
		require.False(t, timeout)

		err = apex.WaitForExactAmount(ctx, user, cardanofw.ChainIDNexus, expectedAmount, 3, time.Second*10)
		require.NoError(t, err)
	})

	//nolint:dupl
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

		apex := cardanofw.SetupAndRunApexBridge(
			t, ctx,
			cardanofw.WithAPIKey(apiKey),
			cardanofw.WithVectorEnabled(false),
			cardanofw.WithNexusEnabled(true),
			cardanofw.WithUserCnt(userCnt),
			cardanofw.WithCustomConfigHandlers(func(_ *cardanofw.ApexSystem, mp map[string]interface{}) {
				cardanofw.GetMapFromInterfaceKey(mp, "ethChains", cardanofw.ChainIDNexus)["testMode"] = uint8(2)
			}, nil),
		)

		defer require.True(t, apex.ApexBridgeProcessesRunning())

		user := apex.Users[userCnt-1]

		prevBalanceDfm, err := apex.GetBalance(ctx, user, cardanofw.ChainIDNexus)
		require.NoError(t, err)

		fmt.Printf("DFM Amount before Tx %d\n", prevBalanceDfm)

		expectedAmount := new(big.Int).Set(sendAmountDfm)
		expectedAmount = expectedAmount.Add(expectedAmount, prevBalanceDfm)

		txHash := apex.SubmitBridgingRequest(
			t, ctx, cardanofw.ChainIDPrime, cardanofw.ChainIDNexus, user, sendAmountDfm, user)

		fmt.Printf("Tx sent. hash: %s\n", txHash)

		// Check batch failed
		failedToExecute, timeout = cardanofw.WaitForBatchState(
			ctx, apex, cardanofw.ChainIDPrime, txHash, apiKey, false, false, cardanofw.BatchStateExecuted)

		require.Equal(t, failedToExecute, 5)
		require.False(t, timeout)

		err = apex.WaitForExactAmount(ctx, user, cardanofw.ChainIDNexus, expectedAmount, 3, time.Second*10)
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

		apex := cardanofw.SetupAndRunApexBridge(
			t, ctx,
			cardanofw.WithAPIKey(apiKey),
			cardanofw.WithVectorEnabled(false),
			cardanofw.WithNexusEnabled(true),
			cardanofw.WithUserCnt(userCnt),
			cardanofw.WithCustomConfigHandlers(func(_ *cardanofw.ApexSystem, mp map[string]interface{}) {
				cardanofw.GetMapFromInterfaceKey(mp, "ethChains", cardanofw.ChainIDNexus)["testMode"] = uint8(3)
			}, nil),
		)

		defer require.True(t, apex.ApexBridgeProcessesRunning())

		user := apex.Users[userCnt-1]

		prevBalanceDfm, err := apex.GetBalance(ctx, user, cardanofw.ChainIDNexus)
		require.NoError(t, err)

		fmt.Printf("DFM Amount before Tx %d\n", prevBalanceDfm)

		ethExpectedBalance := big.NewInt(int64(instances))
		ethExpectedBalance.Mul(ethExpectedBalance, sendAmountDfm)
		ethExpectedBalance.Add(ethExpectedBalance, prevBalanceDfm)

		for i := 0; i < instances; i++ {
			txHash := apex.SubmitBridgingRequest(
				t, ctx, cardanofw.ChainIDPrime, cardanofw.ChainIDNexus, user, sendAmountDfm, user)

			fmt.Printf("Tx %v sent. hash: %s\n", i, txHash)

			failedToExecute[i], timeout[i] = cardanofw.WaitForBatchState(
				ctx, apex, cardanofw.ChainIDPrime, txHash, apiKey, false, false, cardanofw.BatchStateExecuted)
		}

		for i := 0; i < instances; i++ {
			require.Equal(t, failedToExecute[i], 1)
			require.False(t, timeout[i])
		}

		err = apex.WaitForExactAmount(ctx, user, cardanofw.ChainIDNexus, ethExpectedBalance, 20, time.Second*10)
		require.NoError(t, err)
	})

	t.Run("Test failed batches at random", func(t *testing.T) {
		ctx, cncl := context.WithCancel(context.Background())
		defer cncl()

		instances := 5
		failedToExecute := make([]int, instances)
		timeout := make([]bool, instances)

		apex := cardanofw.SetupAndRunApexBridge(
			t, ctx,
			cardanofw.WithAPIKey(apiKey),
			cardanofw.WithVectorEnabled(false),
			cardanofw.WithNexusEnabled(true),
			cardanofw.WithUserCnt(userCnt),
			cardanofw.WithCustomConfigHandlers(func(_ *cardanofw.ApexSystem, mp map[string]interface{}) {
				cardanofw.GetMapFromInterfaceKey(mp, "ethChains", cardanofw.ChainIDNexus)["testMode"] = uint8(4)
			}, nil),
		)

		defer require.True(t, apex.ApexBridgeProcessesRunning())

		user := apex.Users[userCnt-1]

		prevBalanceDfm, err := apex.GetBalance(ctx, user, cardanofw.ChainIDNexus)
		require.NoError(t, err)

		fmt.Printf("DFM Amount before Tx %d\n", prevBalanceDfm)

		ethExpectedBalance := big.NewInt(int64(instances))
		ethExpectedBalance.Mul(ethExpectedBalance, sendAmountDfm)
		ethExpectedBalance.Add(ethExpectedBalance, prevBalanceDfm)

		for i := 0; i < instances; i++ {
			txHash := apex.SubmitBridgingRequest(
				t, ctx, cardanofw.ChainIDPrime, cardanofw.ChainIDNexus, user, sendAmountDfm, user)

			fmt.Printf("Tx %v sent. hash: %s\n", i, txHash)

			// Check batch failed
			failedToExecute[i], timeout[i] = cardanofw.WaitForBatchState(
				ctx, apex, cardanofw.ChainIDPrime, txHash, apiKey, false, false, cardanofw.BatchStateExecuted)
		}

		for i := 0; i < instances; i++ {
			if i%2 == 0 {
				require.Equal(t, 1, failedToExecute[i])
			}

			require.False(t, timeout[i])
		}

		err = apex.WaitForExactAmount(ctx, user, cardanofw.ChainIDNexus, ethExpectedBalance, 3, time.Second*10)
		require.NoError(t, err)
	})
}

func TestE2E_NexusFundAmount(t *testing.T) {
	if cardanofw.ShouldSkipE2RRedundantTests() {
		t.Skip()
	}

	const (
		apiKey     = "test_api_key"
		userCnt    = 10
		fundAmount = 100_000_000
	)

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	primeConfig := cardanofw.NewPrimeChainConfig()
	primeConfig.FundAmount = 1_000_000

	nexusConfig := cardanofw.NewNexusChainConfig(true)
	nexusConfig.FundAmount = big.NewInt(1)

	apex := cardanofw.SetupAndRunApexBridge(
		t, ctx,
		cardanofw.WithPrimeConfig(primeConfig),
		cardanofw.WithNexusConfig(nexusConfig),
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithVectorEnabled(false),
		cardanofw.WithNexusEnabled(true),
		cardanofw.WithUserCnt(userCnt),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	user := apex.Users[userCnt-1]

	fmt.Println("prime user addr: ", user.PrimeAddress)
	fmt.Println("nexus user addr: ", user.NexusAddress)
	fmt.Println("prime multisig addr: ", apex.PrimeInfo.MultisigAddr)
	fmt.Println("prime fee addr: ", apex.PrimeInfo.FeeAddr)
	fmt.Println("nexus gateway addr ", apex.NexusInfo.GatewayAddress)

	testCases := []struct {
		name          string
		sendAmountDfm *big.Int
		fromChain     cardanofw.ChainID
		toChain       cardanofw.ChainID
		fundAmountDfm *big.Int
	}{
		{
			name:          "From nexus to prime - not enough funds",
			sendAmountDfm: cardanofw.WeiToDfm(ethgo.Ether(5)),
			fromChain:     cardanofw.ChainIDNexus,
			toChain:       cardanofw.ChainIDPrime,
			fundAmountDfm: new(big.Int).SetUint64(fundAmount),
		},
		{
			name:          "From prime to nexus - not enough funds",
			sendAmountDfm: cardanofw.WeiToDfm(ethgo.Ether(15)),
			fromChain:     cardanofw.ChainIDPrime,
			toChain:       cardanofw.ChainIDNexus,
			fundAmountDfm: new(big.Int).SetUint64(fundAmount),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			prevAmount, err := apex.GetBalance(ctx, user, tc.toChain)
			require.NoError(t, err)

			fmt.Printf("prevAmount %v\n", prevAmount)

			expectedAmount := new(big.Int).Set(tc.sendAmountDfm)
			expectedAmount = expectedAmount.Add(expectedAmount, prevAmount)

			txHash := apex.SubmitBridgingRequest(
				t, ctx, tc.fromChain, tc.toChain, user, tc.sendAmountDfm, user)

			fmt.Printf("Tx sent. hash: %s. %v - expectedAmount\n", txHash, expectedAmount)

			err = apex.WaitForExactAmount(ctx, user, tc.toChain, expectedAmount, 20, time.Second*10)
			require.Error(t, err)

			require.NoError(t, apex.FundChainHotWallet(ctx, tc.toChain, tc.fundAmountDfm))

			txHash = apex.SubmitBridgingRequest(
				t, ctx, tc.fromChain, tc.toChain, user, tc.sendAmountDfm, user)

			fmt.Printf("Tx sent. hash: %s. %v - expectedAmount\n", txHash, expectedAmount)

			err = apex.WaitForExactAmount(ctx, user, tc.toChain, expectedAmount, 20, time.Second*10)
			require.NoError(t, err)
		})
	}
}

func PrimeToNexusSequentialAndParallelWithMaxReceivers(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem,
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
		apex.Users[:receivers],
		[]string{cardanofw.ChainIDPrime},
		map[string][]string{
			cardanofw.ChainIDPrime: {cardanofw.ChainIDNexus},
		},
		sendAmountDfm,
		options...)
}

func PrimeNexusBothDirectionsSequentialAndParallel(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, receiverUser *cardanofw.TestApexUser,
	sequentialInstances, parallelInstances int, sendAmountDfm *big.Int, options ...e2ehelper.ExecuteBridgingOption,
) {
	t.Helper()

	const ()

	e2ehelper.ExecuteBridging(
		t, ctx, apex,
		sequentialInstances,
		apex.Users[:parallelInstances],
		[]*cardanofw.TestApexUser{receiverUser},
		[]string{cardanofw.ChainIDPrime, cardanofw.ChainIDNexus},
		map[string][]string{
			cardanofw.ChainIDPrime: {cardanofw.ChainIDNexus},
			cardanofw.ChainIDNexus: {cardanofw.ChainIDPrime},
		},
		sendAmountDfm,
		options...)
}

func NexusToPrimeSequentialAndParallelWithMaxReceivers(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem,
	sequentialInstances, parallelInstances int, sendAmountDfm *big.Int, options ...e2ehelper.ExecuteBridgingOption,
) {
	t.Helper()

	const (
		receivers = 4
	)

	e2ehelper.ExecuteBridging(
		t, ctx, apex, sequentialInstances,
		apex.Users[:parallelInstances],
		apex.Users[:receivers],
		[]string{cardanofw.ChainIDNexus},
		map[string][]string{
			cardanofw.ChainIDNexus: {cardanofw.ChainIDPrime},
		},
		sendAmountDfm,
		options...)
}

func PrimeToNexusSubmitterNotEnoughFunds(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, user *cardanofw.TestApexUser,
	sendAmountDfm *big.Int,
) {
	t.Helper()

	srcChain, dstChain := cardanofw.ChainIDPrime, cardanofw.ChainIDNexus
	receiverAddr := apex.PrimeInfo.MultisigAddr
	feeAmount := uint64(1_100_000)

	receivers := map[string]uint64{
		user.GetAddress(dstChain): sendAmountDfm.Uint64(),
	}

	bridgingRequestMetadata, err := cardanofw.CreateCardanoBridgingMetaData(
		user.GetAddress(srcChain), receivers, dstChain, feeAmount)
	require.NoError(t, err)

	_, err = apex.SubmitTx(
		ctx, srcChain, user, receiverAddr, sendAmountDfm, bridgingRequestMetadata)

	require.Error(t, err)
	require.ErrorContains(t, err, "not enough funds")
}

func PrimeToNexusInvalidMetadataSlicedOff(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, user *cardanofw.TestApexUser,
) {
	t.Helper()

	srcChain, dstChain := cardanofw.ChainIDPrime, cardanofw.ChainIDNexus
	receiverAddr := apex.PrimeInfo.MultisigAddr
	sendAmountDfm := cardanofw.WeiToDfm(ethgo.Ether(1))
	feeAmount := uint64(1_100_000)

	receivers := map[string]uint64{
		user.GetAddress(dstChain): sendAmountDfm.Uint64(),
	}

	bridgingRequestMetadata, err := cardanofw.CreateCardanoBridgingMetaData(
		user.GetAddress(srcChain), receivers, dstChain, feeAmount)
	require.NoError(t, err)

	// Send only half bytes of metadata making it invalid
	bridgingRequestMetadata = bridgingRequestMetadata[0 : len(bridgingRequestMetadata)/2]

	_, err = apex.SubmitTx(
		ctx, srcChain, user, receiverAddr, sendAmountDfm, bridgingRequestMetadata)
	require.Error(t, err)
}

func PrimeToNexusInvalidMetadataWrongType(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, user *cardanofw.TestApexUser,
	requestStateTimeoutSec uint,
) {
	t.Helper()

	srcChain, dstChain := cardanofw.ChainIDPrime, cardanofw.ChainIDNexus
	receiverAddr := apex.PrimeInfo.MultisigAddr
	sendAmountDfm := cardanofw.WeiToDfm(ethgo.Ether(1))
	feeAmount := uint64(1_100_000)

	receivers := map[string]uint64{
		user.GetAddress(dstChain): sendAmountDfm.Uint64(),
	}

	var transactions = make([]cardanofw.BridgingRequestMetadataTransaction, 0, len(receivers))
	for addr, amount := range receivers {
		transactions = append(transactions, cardanofw.BridgingRequestMetadataTransaction{
			Address: cardanofw.AddrToMetaDataAddr(addr),
			Amount:  amount,
		})
	}

	metadata := map[string]interface{}{
		"1": map[string]interface{}{
			"t":  "transaction", // should be "bridge"
			"d":  dstChain,
			"s":  cardanofw.AddrToMetaDataAddr(user.GetAddress(srcChain)),
			"tx": transactions,
			"fa": feeAmount,
		},
	}

	bridgingRequestMetadata, err := json.Marshal(metadata)
	require.NoError(t, err)

	beforeSendingAmountDfm, err := apex.GetBalance(ctx, user, cardanofw.ChainIDPrime)
	require.NoError(t, err)

	txHash, err := apex.SubmitTx(
		ctx, srcChain, user, receiverAddr,
		sendAmountDfm.Add(sendAmountDfm, new(big.Int).SetUint64(feeAmount)), bridgingRequestMetadata)
	require.NoError(t, err)

	lowerBoundaryDfm := new(big.Int).Sub(beforeSendingAmountDfm, new(big.Int).Add(sendAmountDfm, new(big.Int).SetUint64(feeAmount)))

	fmt.Printf("Tx sent. hash: %s, lowerBoundaryDfm: %d, higherBoundaryDfm: %d\n", txHash, lowerBoundaryDfm, beforeSendingAmountDfm)

	err = apex.WaitForAmountInRange(ctx, user, cardanofw.ChainIDPrime, lowerBoundaryDfm, beforeSendingAmountDfm,
		50, time.Second*30)
	require.NoError(t, err)
}

func PrimeToNexusInvalidMetadataInvalidDestination(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, user *cardanofw.TestApexUser,
	invalidStateTimeoutSec uint,
) {
	t.Helper()

	srcChain, dstChain := cardanofw.ChainIDPrime, cardanofw.ChainIDNexus
	receiverAddr := apex.PrimeInfo.MultisigAddr
	sendAmountDfm := cardanofw.WeiToDfm(ethgo.Ether(1))
	feeAmount := uint64(1_100_000)

	receivers := map[string]uint64{
		user.GetAddress(dstChain): sendAmountDfm.Uint64(),
	}

	var transactions = make([]cardanofw.BridgingRequestMetadataTransaction, 0, len(receivers))
	for addr, amount := range receivers {
		transactions = append(transactions, cardanofw.BridgingRequestMetadataTransaction{
			Address: cardanofw.AddrToMetaDataAddr(addr),
			Amount:  amount,
		})
	}

	metadata := map[string]interface{}{
		"1": map[string]interface{}{
			"t":  "bridge",
			"d":  "", // should be destination chain address
			"s":  cardanofw.AddrToMetaDataAddr(user.GetAddress(srcChain)),
			"tx": transactions,
			"fa": feeAmount,
		},
	}

	bridgingRequestMetadata, err := json.Marshal(metadata)
	require.NoError(t, err)

	beforeSendingAmountDfm, err := apex.GetBalance(ctx, user, cardanofw.ChainIDPrime)
	require.NoError(t, err)

	txHash, err := apex.SubmitTx(
		ctx, srcChain, user, receiverAddr,
		sendAmountDfm.Add(sendAmountDfm, new(big.Int).SetUint64(feeAmount)), bridgingRequestMetadata)
	require.NoError(t, err)

	lowerBoundaryDfm := new(big.Int).Sub(beforeSendingAmountDfm, new(big.Int).Add(sendAmountDfm, new(big.Int).SetUint64(feeAmount)))

	fmt.Printf("Tx sent. hash: %s, lowerBoundaryDfm: %d, higherBoundaryDfm: %d\n", txHash, lowerBoundaryDfm, beforeSendingAmountDfm)

	err = apex.WaitForAmountInRange(ctx, user, cardanofw.ChainIDPrime, lowerBoundaryDfm, beforeSendingAmountDfm,
		50, time.Second*30)
	require.NoError(t, err)
}

func PrimeToNexusInvalidMetadataInvalidSender(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, user *cardanofw.TestApexUser,
	invalidStateTimeoutSec uint,
) {
	t.Helper()

	srcChain, dstChain := cardanofw.ChainIDPrime, cardanofw.ChainIDNexus
	receiverAddr := apex.PrimeInfo.MultisigAddr
	sendAmountDfm := cardanofw.WeiToDfm(ethgo.Ether(1))
	feeAmount := uint64(1_100_000)

	receivers := map[string]uint64{
		user.GetAddress(dstChain): sendAmountDfm.Uint64(),
	}

	var transactions = make([]cardanofw.BridgingRequestMetadataTransaction, 0, len(receivers))
	for addr, amount := range receivers {
		transactions = append(transactions, cardanofw.BridgingRequestMetadataTransaction{
			Address: cardanofw.AddrToMetaDataAddr(addr),
			Amount:  amount,
		})
	}

	metadata := map[string]interface{}{
		"1": map[string]interface{}{
			"t":  "bridge",
			"d":  dstChain,
			"s":  "", // should be sender address (max len 40)
			"tx": transactions,
			"fa": feeAmount,
		},
	}

	bridgingRequestMetadata, err := json.Marshal(metadata)
	require.NoError(t, err)

	txHash, err := apex.SubmitTx(
		ctx, srcChain, user, receiverAddr,
		sendAmountDfm.Add(sendAmountDfm, new(big.Int).SetUint64(feeAmount)), bridgingRequestMetadata)
	require.NoError(t, err)

	cardanofw.WaitForInvalidState(t, ctx, apex, srcChain, txHash, apex.Config.APIKey, invalidStateTimeoutSec)
}

func PrimeToNexusInvalidMetadataInvalidTransactions(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, user *cardanofw.TestApexUser,
	invalidStateTimeoutSec uint,
) {
	t.Helper()

	srcChain, dstChain := cardanofw.ChainIDPrime, cardanofw.ChainIDNexus
	receiverAddr := apex.PrimeInfo.MultisigAddr
	sendAmountDfm := cardanofw.WeiToDfm(ethgo.Ether(1))
	feeAmount := uint64(1_100_000)

	metadata := map[string]interface{}{
		"1": map[string]interface{}{
			"t":  "bridge",
			"d":  dstChain,
			"s":  cardanofw.AddrToMetaDataAddr(user.GetAddress(srcChain)),
			"tx": []cardanofw.BridgingRequestMetadataTransaction{}, // should not be empty
			"fa": feeAmount,
		},
	}

	bridgingRequestMetadata, err := json.Marshal(metadata)
	require.NoError(t, err)

	beforeSendingAmountDfm, err := apex.GetBalance(ctx, user, cardanofw.ChainIDPrime)
	require.NoError(t, err)

	txHash, err := apex.SubmitTx(
		ctx, srcChain, user, receiverAddr,
		sendAmountDfm.Add(sendAmountDfm, new(big.Int).SetUint64(feeAmount)), bridgingRequestMetadata)
	require.NoError(t, err)

	lowerBoundaryDfm := new(big.Int).Sub(beforeSendingAmountDfm, new(big.Int).Add(sendAmountDfm, new(big.Int).SetUint64(feeAmount)))

	fmt.Printf("Tx sent. hash: %s, lowerBoundaryDfm: %d, higherBoundaryDfm: %d\n", txHash, lowerBoundaryDfm, beforeSendingAmountDfm)

	err = apex.WaitForAmountInRange(ctx, user, cardanofw.ChainIDPrime, lowerBoundaryDfm, beforeSendingAmountDfm,
		50, time.Second*30)
	require.NoError(t, err)
}

func NexusToPrimeSubmitterNotEnoughFunds(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem,
) {
	t.Helper()

	fee := cardanofw.DfmToChainNativeTokenAmount(cardanofw.ChainIDNexus, new(big.Int).SetUint64(uint64(1_100_000)))
	sendAmountWei := ethgo.Ether(uint64(2))

	unfundedUser, err := cardanofw.NewTestApexUser(
		apex.Config.PrimeConfig.NetworkType,
		apex.Config.VectorConfig.IsEnabled,
		apex.Config.VectorConfig.NetworkType,
		apex.Config.NexusConfig.IsEnabled,
	)
	require.NoError(t, err)

	unfundedUserPk, err := unfundedUser.GetPrivateKey(cardanofw.ChainIDNexus)
	require.NoError(t, err)

	// call SendTx command
	err = sendTxParamsNPInvalidScenarios("evm",
		apex.NexusInfo.GatewayAddress.String(),
		apex.NexusInfo.JSONRPCAddr,
		unfundedUserPk, cardanofw.ChainIDPrime,
		unfundedUser.GetAddress(cardanofw.ChainIDPrime),
		sendAmountWei, fee,
	)
	require.ErrorContains(t, err, "insufficient funds")
}

func sendTxParamsNPInvalidScenarios(txType, gatewayAddr, nexusURL, privateKey, chainDst, receiver string, amount, fee *big.Int) error {
	return cardanofw.RunCommand(cardanofw.ResolveApexBridgeBinary(), []string{
		"sendtx",
		"--tx-type", txType,
		"--gateway-addr", gatewayAddr,
		"--nexus-url", nexusURL,
		"--key", privateKey,
		"--chain-dst", chainDst,
		"--receiver", fmt.Sprintf("%s:%s", receiver, amount.String()),
		"--fee", fee.String(),
	}, os.Stdout)
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

	apex := cardanofw.SetupAndRunApexBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithVectorEnabled(false),
		cardanofw.WithNexusEnabled(true),
		cardanofw.WithTargetOneClusterServer(true),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	user := apex.Users[0]
	sendAmountDfm := cardanofw.WeiToDfm(ethgo.Ether(1))

	// execute prime to nexus -> no wait
	prevAmountNexusDfm, err := apex.GetBalance(ctx, user, cardanofw.ChainIDNexus)
	require.NoError(t, err)

	// give time to oracle to submit hot wallet increment claims
	select {
	case <-ctx.Done():
		return
	case <-time.After(60 * time.Second):
	}

	txHash := apex.SubmitBridgingRequest(t, ctx, cardanofw.ChainIDPrime, cardanofw.ChainIDNexus, user, sendAmountDfm, user)

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
	expectedAmountDfm := new(big.Int).Add(prevAmountNexusDfm, sendAmountDfm)

	err = apex.WaitForExactAmount(ctx, user, cardanofw.ChainIDNexus, expectedAmountDfm, 100, time.Second*10)
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
		sendAmountDfm)
}
