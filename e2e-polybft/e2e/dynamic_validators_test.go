package e2e

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/command/proposal/submit"
	"github.com/0xPolygon/polygon-edge/command/validator/helper"
	"github.com/0xPolygon/polygon-edge/consensus/polybft"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/validator"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/cardanofw"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/e2ehelper"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/framework"
	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
	bn256 "github.com/Ethernal-Tech/bn256"
	infracommon "github.com/Ethernal-Tech/cardano-infrastructure/common"
	"github.com/Ethernal-Tech/ethgo"
	"github.com/stretchr/testify/require"

	secretsCardano "github.com/Ethernal-Tech/cardano-infrastructure/secrets"
	secretsHelper "github.com/Ethernal-Tech/cardano-infrastructure/secrets/helper"
)

func TestE2E_DynamicValidators_AddValidator(t *testing.T) {
	const (
		apiKey  = "test_api_key"
		userCnt = 40
	)

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	primeConfig, vectorConfig := cardanofw.NewPrimeChainConfig(), cardanofw.NewVectorChainConfig(true)
	primeConfig.BridgeAddrHasStake = true
	primeConfig.PremineAmount = 500_000_000
	vectorConfig.BridgeAddrHasStake = true
	vectorConfig.PremineAmount = 500_000_000

	apex := cardanofw.SetupAndRunApexBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithUserCnt(userCnt),
		cardanofw.WithPrimeConfig(primeConfig),
		cardanofw.WithVectorConfig(vectorConfig),
		cardanofw.WithAPIValidatorID(-1),
		cardanofw.WithNonValidators(1),
		cardanofw.WithNexusEnabled(true),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	t.Log("Cluster started")

	cluster := apex.BridgeCluster

	require.NoError(t, cluster.WaitUntil(3*time.Minute, 2*time.Second, func() bool {
		multisig, fee := getMultisigAndFeeAmount(t, ctx, apex, apiKey, cardanofw.ChainIDPrime)

		return multisig == primeConfig.FundAmount && fee == primeConfig.FundFeeAmount
	}))

	require.NoError(t, cluster.WaitUntil(3*time.Minute, 2*time.Second, func() bool {
		multisig, fee := getMultisigAndFeeAmount(t, ctx, apex, apiKey, cardanofw.ChainIDVector)

		return multisig == vectorConfig.FundAmount && fee == vectorConfig.FundFeeAmount
	}))

	sendAmountDfm := cardanofw.WeiToDfm(ethgo.Ether(1))

	newValidatorSrv := cluster.Servers[4]

	proposer := cluster.Servers[0]

	proposerAcc, err := helper.GetAccountFromDir(proposer.DataDir())
	require.NoError(t, err)

	relayer, err := txrelayer.NewTxRelayer(txrelayer.WithClient(proposer.JSONRPC()))
	require.NoError(t, err)

	polybftCfg, err := polybft.LoadPolyBFTConfig(path.Join(cluster.Config.TmpDir, chainConfigFileName))
	require.NoError(t, err)

	newValidatorAcc, err := helper.GetAccountFromDir(newValidatorSrv.DataDir())
	require.NoError(t, err)

	// generate for non validator
	require.NoError(t, apex.AddNewValidator(t, ctx, newValidatorSrv).Start(ctx, false))

	primeKeys := getMultisigAndFeeFromDataDir(t, newValidatorSrv.DataDir(), "prime")
	vectorKeys := getMultisigAndFeeFromDataDir(t, newValidatorSrv.DataDir(), "vector")

	executeValidatorChangeProposal(t, ctx, relayer, proposerAcc, []*addedValidator{
		{
			Address: newValidatorAcc.Address(),
			Key:     newValidatorAcc.Bls.PublicKey(),
			CardanoLikeChains: []string{
				keysToStr("prime", &primeKeys),
				keysToStr("vector", &vectorKeys),
			},
		},
	}, nil, cluster, polybftCfg)

	t.Log("Executed validator set change")

	// wait for vsc to be sent for sure
	currentBlock, err := proposer.JSONRPC().BlockNumber()
	require.NoError(t, err)

	require.NoError(t, cluster.WaitForBlock(currentBlock+10, time.Minute))

	// wait for validator set change to finish
	waitUntilValidatorSetUpdateIsFinished(t, cluster, relayer, 5*time.Minute, 10*time.Second)

	t.Log("Finished VSC")

	checkValidatorActive(t, newValidatorAcc.Address(), relayer, true)
	// check stake amount to be equal to staked amount on the 1st validator
	checkValidatorStake(t, newValidatorAcc.Address(), relayer, cluster.Config.StakeAmounts[0])

	t.Logf("Added new validator")

	// stop one of validators to check if new validator participates in voting
	require.NoError(t, cluster.Servers[1].Stop())

	// create new multisig and fee addresses
	require.NoError(t, apex.UpdateConfigs())

	// restart validators
	require.NoError(t, apex.RestartBridges(ctx, 1))

	// check on new multisig
	require.NoError(t, cluster.WaitUntil(3*time.Minute, 2*time.Second, func() bool {
		multisig, fee := getMultisigAndFeeAmount(t, ctx, apex, apiKey, cardanofw.ChainIDPrime)

		t.Log("prime multisig", multisig)

		return multisig == primeConfig.FundAmount && fee > 0 && fee < primeConfig.FundFeeAmount
	}))

	require.NoError(t, cluster.WaitUntil(3*time.Minute, 2*time.Second, func() bool {
		multisig, fee := getMultisigAndFeeAmount(t, ctx, apex, apiKey, cardanofw.ChainIDVector)

		t.Log("vector multisig", multisig)

		return multisig == vectorConfig.FundAmount && fee > 0 && fee < vectorConfig.FundFeeAmount
	}))

	e2ehelper.ExecuteBridging(t, ctx, apex, 1,
		apex.Users[:1],
		apex.Users[1:2],
		[]string{cardanofw.ChainIDPrime, cardanofw.ChainIDVector, cardanofw.ChainIDNexus},
		map[string][]string{
			cardanofw.ChainIDPrime:  {cardanofw.ChainIDVector, cardanofw.ChainIDNexus},
			cardanofw.ChainIDVector: {cardanofw.ChainIDPrime},
			cardanofw.ChainIDNexus:  {cardanofw.ChainIDPrime},
		},
		sendAmountDfm)
}

func TestE2E_DynamicValidators_RemoveValidator(t *testing.T) {
	const (
		apiKey  = "test_api_key"
		userCnt = 40
	)

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	primeConfig, vectorConfig := cardanofw.NewPrimeChainConfig(), cardanofw.NewVectorChainConfig(true)
	primeConfig.BridgeAddrHasStake = true
	primeConfig.PremineAmount = 500_000_000
	vectorConfig.BridgeAddrHasStake = true
	vectorConfig.PremineAmount = 500_000_000

	apex := cardanofw.SetupAndRunApexBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithUserCnt(userCnt),
		cardanofw.WithPrimeConfig(primeConfig),
		cardanofw.WithVectorConfig(vectorConfig),
		cardanofw.WithAPIValidatorID(-1),
		cardanofw.WithValidators(5),
		cardanofw.WithNexusEnabled(true),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	t.Log("Cluster started")

	cluster := apex.BridgeCluster

	require.NoError(t, cluster.WaitUntil(3*time.Minute, 2*time.Second, func() bool {
		multisig, fee := getMultisigAndFeeAmount(t, ctx, apex, apiKey, cardanofw.ChainIDPrime)

		return multisig == primeConfig.FundAmount && fee == primeConfig.FundFeeAmount
	}))

	require.NoError(t, cluster.WaitUntil(3*time.Minute, 2*time.Second, func() bool {
		multisig, fee := getMultisigAndFeeAmount(t, ctx, apex, apiKey, cardanofw.ChainIDVector)

		return multisig == vectorConfig.FundAmount && fee == vectorConfig.FundFeeAmount
	}))

	sendAmountDfm := cardanofw.WeiToDfm(ethgo.Ether(1))

	proposer := cluster.Servers[0]

	proposerAcc, err := helper.GetAccountFromDir(proposer.DataDir())
	require.NoError(t, err)

	relayer, err := txrelayer.NewTxRelayer(txrelayer.WithClient(proposer.JSONRPC()))
	require.NoError(t, err)

	polybftCfg, err := polybft.LoadPolyBFTConfig(path.Join(cluster.Config.TmpDir, chainConfigFileName))
	require.NoError(t, err)

	removeValidator := cluster.Servers[4]
	removeValidatorKey, err := helper.GetAccountFromDir(removeValidator.DataDir())
	require.NoError(t, err)

	executeValidatorChangeProposal(t, ctx, relayer, proposerAcc, nil,
		[]types.Address{removeValidatorKey.Address()}, cluster, polybftCfg)

	t.Log("Executed validator set change")

	// wait for vsc to be sent for sure
	currentBlock, err := proposer.JSONRPC().BlockNumber()
	require.NoError(t, err)

	require.NoError(t, cluster.WaitForBlock(currentBlock+10, time.Minute))

	// wait for validator set change to finish
	waitUntilValidatorSetUpdateIsFinished(t, cluster, relayer, 5*time.Minute, 10*time.Second)

	t.Log("Finished VSC")

	checkValidatorActive(t, removeValidatorKey.Address(), relayer, false)
	checkValidatorStake(t, removeValidatorKey.Address(), relayer, big.NewInt(0))

	t.Logf("Removed validator")

	// stop removed validator
	require.NoError(t, removeValidator.Stop())

	// create new multisig and fee addresses
	require.NoError(t, apex.UpdateConfigs())

	// restart some validators & stop ones not used
	require.NoError(t, apex.RestartBridges(ctx, 4))

	// check on new multisig
	require.NoError(t, cluster.WaitUntil(3*time.Minute, 2*time.Second, func() bool {
		multisig, fee := getMultisigAndFeeAmount(t, ctx, apex, apiKey, cardanofw.ChainIDPrime)

		t.Log("prime multisig", multisig)

		return multisig == primeConfig.FundAmount && fee > 0 && fee < primeConfig.FundFeeAmount
	}))

	require.NoError(t, cluster.WaitUntil(3*time.Minute, 2*time.Second, func() bool {
		multisig, fee := getMultisigAndFeeAmount(t, ctx, apex, apiKey, cardanofw.ChainIDVector)

		t.Log("vector multisig", multisig)

		return multisig == vectorConfig.FundAmount && fee > 0 && fee < vectorConfig.FundFeeAmount
	}))

	e2ehelper.ExecuteBridging(t, ctx, apex, 1,
		apex.Users[:1],
		apex.Users[1:2],
		[]string{cardanofw.ChainIDPrime, cardanofw.ChainIDVector, cardanofw.ChainIDNexus},
		map[string][]string{
			cardanofw.ChainIDPrime:  {cardanofw.ChainIDVector, cardanofw.ChainIDNexus},
			cardanofw.ChainIDVector: {cardanofw.ChainIDPrime},
			cardanofw.ChainIDNexus:  {cardanofw.ChainIDPrime},
		},
		sendAmountDfm)
}

func TestE2E_DynamicValidators_AddAndRemoveValidator(t *testing.T) {
	const (
		apiKey  = "test_api_key"
		userCnt = 40
	)

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	primeConfig, vectorConfig := cardanofw.NewPrimeChainConfig(), cardanofw.NewVectorChainConfig(true)
	primeConfig.BridgeAddrHasStake = true
	primeConfig.PremineAmount = 500_000_000
	primeConfig.FundUTxOCount = 80
	primeConfig.FundFeeUTxOCount = 80
	vectorConfig.BridgeAddrHasStake = true
	vectorConfig.PremineAmount = 500_000_000
	vectorConfig.FundUTxOCount = 80
	vectorConfig.FundFeeUTxOCount = 80

	apex := cardanofw.SetupAndRunApexBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithUserCnt(userCnt),
		cardanofw.WithPrimeConfig(primeConfig),
		cardanofw.WithVectorConfig(vectorConfig),
		cardanofw.WithAPIValidatorID(-1),
		cardanofw.WithNonValidators(1),
		cardanofw.WithNexusEnabled(true),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	t.Log("Cluster started")

	cluster := apex.BridgeCluster

	require.NoError(t, cluster.WaitUntil(3*time.Minute, 2*time.Second, func() bool {
		multisig, fee := getMultisigAndFeeAmount(t, ctx, apex, apiKey, cardanofw.ChainIDPrime)

		return multisig == primeConfig.FundAmount && fee == primeConfig.FundFeeAmount
	}))

	require.NoError(t, cluster.WaitUntil(3*time.Minute, 2*time.Second, func() bool {
		multisig, fee := getMultisigAndFeeAmount(t, ctx, apex, apiKey, cardanofw.ChainIDVector)

		return multisig == vectorConfig.FundAmount && fee == vectorConfig.FundFeeAmount
	}))

	sendAmountDfm := cardanofw.WeiToDfm(ethgo.Ether(1))

	newValidatorSrv := cluster.Servers[4]

	proposer := cluster.Servers[0]

	proposerAcc, err := helper.GetAccountFromDir(proposer.DataDir())
	require.NoError(t, err)

	relayer, err := txrelayer.NewTxRelayer(txrelayer.WithClient(proposer.JSONRPC()))
	require.NoError(t, err)

	polybftCfg, err := polybft.LoadPolyBFTConfig(path.Join(cluster.Config.TmpDir, chainConfigFileName))
	require.NoError(t, err)

	newValidatorAcc, err := helper.GetAccountFromDir(newValidatorSrv.DataDir())
	require.NoError(t, err)

	removeValidator := cluster.Servers[3]
	removeValidatorKey, err := helper.GetAccountFromDir(removeValidator.DataDir())
	require.NoError(t, err)

	// generate for non validator
	require.NoError(t, apex.AddNewValidator(t, ctx, newValidatorSrv).Start(ctx, false))

	primeKeys := getMultisigAndFeeFromDataDir(t, newValidatorSrv.DataDir(), "prime")
	vectorKeys := getMultisigAndFeeFromDataDir(t, newValidatorSrv.DataDir(), "vector")

	// wait some time until funding is processed and last observed slot updated on Bridge SC
	<-time.After(time.Minute)

	executeValidatorChangeProposal(t, ctx, relayer, proposerAcc, []*addedValidator{
		{
			Address: newValidatorAcc.Address(),
			Key:     newValidatorAcc.Bls.PublicKey(),
			CardanoLikeChains: []string{
				keysToStr("prime", &primeKeys),
				keysToStr("vector", &vectorKeys),
			},
		},
	}, []types.Address{removeValidatorKey.Address()}, cluster, polybftCfg)

	t.Log("Executed validator set change")

	// wait for vsc to be sent for sure
	currentBlock, err := proposer.JSONRPC().BlockNumber()
	require.NoError(t, err)

	require.NoError(t, cluster.WaitForBlock(currentBlock+10, time.Minute))

	// wait for validator set change to finish
	waitUntilValidatorSetUpdateIsFinished(t, cluster, relayer, 10*time.Minute, 10*time.Second)

	t.Log("Finished VSC")

	checkValidatorActive(t, newValidatorAcc.Address(), relayer, true)
	checkValidatorActive(t, removeValidatorKey.Address(), relayer, false)
	// check stake amount to be equal to staked amount on the 1st validator
	checkValidatorStake(t, newValidatorAcc.Address(), relayer, cluster.Config.StakeAmounts[0])
	checkValidatorStake(t, removeValidatorKey.Address(), relayer, big.NewInt(0))

	t.Logf("Added new validator")

	// stop one of validators to check if new validator participates in voting
	require.NoError(t, cluster.Servers[1].Stop())

	// stop removed validator
	require.NoError(t, removeValidator.Stop())

	// create new multisig and fee addresses
	require.NoError(t, apex.UpdateConfigs())

	// restart some validators & stop ones not used
	require.NoError(t, apex.RestartBridges(ctx, 1, 3))

	// check on new multisig
	require.NoError(t, cluster.WaitUntil(3*time.Minute, 2*time.Second, func() bool {
		multisig, fee := getMultisigAndFeeAmount(t, ctx, apex, apiKey, cardanofw.ChainIDPrime)

		t.Log("prime multisig", multisig)

		return multisig == primeConfig.FundAmount && fee > 0 && fee < primeConfig.FundFeeAmount
	}))

	require.NoError(t, cluster.WaitUntil(3*time.Minute, 2*time.Second, func() bool {
		multisig, fee := getMultisigAndFeeAmount(t, ctx, apex, apiKey, cardanofw.ChainIDVector)

		t.Log("vector multisig", multisig)

		return multisig == vectorConfig.FundAmount && fee > 0 && fee < vectorConfig.FundFeeAmount
	}))

	e2ehelper.ExecuteBridging(t, ctx, apex, 1,
		apex.Users[:1],
		apex.Users[1:2],
		[]string{cardanofw.ChainIDPrime, cardanofw.ChainIDVector, cardanofw.ChainIDNexus},
		map[string][]string{
			cardanofw.ChainIDPrime:  {cardanofw.ChainIDVector, cardanofw.ChainIDNexus},
			cardanofw.ChainIDVector: {cardanofw.ChainIDPrime},
			cardanofw.ChainIDNexus:  {cardanofw.ChainIDPrime},
		},
		sendAmountDfm)
}

// this test will do VSU without multisig transfer since fee utxo value < 2 * MinUTxODefaultValue
func TestE2E_DynamicValidators_OneFeeUtxo(t *testing.T) {
	const (
		apiKey  = "test_api_key"
		userCnt = 40
	)

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	primeConfig, vectorConfig := cardanofw.NewPrimeChainConfig(), cardanofw.NewVectorChainConfig(true)
	primeConfig.BridgeAddrHasStake = true
	primeConfig.FundFeeAmount = cardanofw.MinUTxODefaultValue
	vectorConfig.BridgeAddrHasStake = true
	vectorConfig.FundFeeAmount = cardanofw.MinUTxODefaultValue

	apex := cardanofw.SetupAndRunApexBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithUserCnt(userCnt),
		cardanofw.WithPrimeConfig(primeConfig),
		cardanofw.WithVectorConfig(vectorConfig),
		cardanofw.WithAPIValidatorID(-1),
		cardanofw.WithValidators(5),
		cardanofw.WithNexusEnabled(true),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	t.Log("Cluster started")

	cluster := apex.BridgeCluster

	require.NoError(t, cluster.WaitUntil(3*time.Minute, 2*time.Second, func() bool {
		multisig, fee := getMultisigAndFeeAmount(t, ctx, apex, apiKey, cardanofw.ChainIDPrime)

		return multisig == primeConfig.FundAmount && fee == primeConfig.FundFeeAmount
	}))

	require.NoError(t, cluster.WaitUntil(3*time.Minute, 2*time.Second, func() bool {
		multisig, fee := getMultisigAndFeeAmount(t, ctx, apex, apiKey, cardanofw.ChainIDVector)

		return multisig == vectorConfig.FundAmount && fee == vectorConfig.FundFeeAmount
	}))

	proposer := cluster.Servers[0]

	proposerAcc, err := helper.GetAccountFromDir(proposer.DataDir())
	require.NoError(t, err)

	relayer, err := txrelayer.NewTxRelayer(txrelayer.WithClient(proposer.JSONRPC()))
	require.NoError(t, err)

	polybftCfg, err := polybft.LoadPolyBFTConfig(path.Join(cluster.Config.TmpDir, chainConfigFileName))
	require.NoError(t, err)

	removeValidator := cluster.Servers[4]
	removeValidatorKey, err := helper.GetAccountFromDir(removeValidator.DataDir())
	require.NoError(t, err)

	executeValidatorChangeProposal(t, ctx, relayer, proposerAcc, nil,
		[]types.Address{removeValidatorKey.Address()}, cluster, polybftCfg)

	t.Log("Executed validator set change")

	// wait for vsc to be sent for sure
	currentBlock, err := proposer.JSONRPC().BlockNumber()
	require.NoError(t, err)

	require.NoError(t, cluster.WaitForBlock(currentBlock+10, time.Minute))

	// wait for validator set change to finish
	waitUntilValidatorSetUpdateIsFinished(t, cluster, relayer, 5*time.Minute, 10*time.Second)

	t.Log("Finished VSC")

	checkValidatorActive(t, removeValidatorKey.Address(), relayer, false)
	checkValidatorStake(t, removeValidatorKey.Address(), relayer, big.NewInt(0))

	t.Logf("Removed validator")

	// stop removed validator
	require.NoError(t, removeValidator.Stop())

	// create new multisig and fee addresses
	require.NoError(t, apex.UpdateConfigs())

	// restart some apex bridges & stop ones not used
	require.NoError(t, apex.RestartBridges(ctx, 4))

	// wait until bridge is initialized and check on new multisig
	<-time.After(15 * time.Second)

	multisig, fee := getMultisigAndFeeAmount(t, ctx, apex, apiKey, cardanofw.ChainIDPrime)
	t.Log("prime multisig", multisig)

	require.Zero(t, multisig)
	require.Zero(t, fee)

	multisig, fee = getMultisigAndFeeAmount(t, ctx, apex, apiKey, cardanofw.ChainIDVector)
	t.Log("vector multisig", multisig)

	require.Zero(t, multisig)
	require.Zero(t, fee)
}

// The test stops 1 blade before VSU and 2nd during VSU, then starts the 1st one. After VSU is done
// we start the 2nd one and checking if bridge continues with batching with 2nd one Blade included in
// consensus. It is expected to continue working, unlike the 1st one becase apex bridge missed signal for
// VSU start (because Blade was stopped) and therefore 1st stopped node will miss new multisig utxos
// received through Cardano blocks sync. The 2nd one Blade was up when VSU started so it received the start
// signal for VSU and it was able to receive all new multisig utxos through the sync during VSU because apex
// bridge was up all the time. It is expected old multisig utxos to be deleted entirely from all nodes.
func TestE2E_DynamicValidators_StopBladesDuringVSU(t *testing.T) {
	if cardanofw.ShouldSkipE2RRedundantTests() {
		t.Skip()
	}

	const (
		apiKey  = "test_api_key"
		userCnt = 40
	)

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	primeConfig, vectorConfig := cardanofw.NewPrimeChainConfig(), cardanofw.NewVectorChainConfig(true)
	primeConfig.BridgeAddrHasStake = true
	primeConfig.PremineAmount = 500_000_000
	primeConfig.FundUTxOCount = 80
	primeConfig.FundFeeUTxOCount = 80
	vectorConfig.BridgeAddrHasStake = true
	vectorConfig.PremineAmount = 500_000_000
	vectorConfig.FundUTxOCount = 80
	vectorConfig.FundFeeUTxOCount = 80

	apex := cardanofw.SetupAndRunApexBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithUserCnt(userCnt),
		cardanofw.WithPrimeConfig(primeConfig),
		cardanofw.WithVectorConfig(vectorConfig),
		cardanofw.WithAPIValidatorID(-1),
		cardanofw.WithValidators(5),
		cardanofw.WithNexusEnabled(true),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	t.Log("Cluster started")

	cluster := apex.BridgeCluster

	require.NoError(t, cluster.WaitUntil(3*time.Minute, 2*time.Second, func() bool {
		multisig, fee := getMultisigAndFeeAmount(t, ctx, apex, apiKey, cardanofw.ChainIDPrime)

		return multisig == primeConfig.FundAmount && fee == primeConfig.FundFeeAmount
	}))

	require.NoError(t, cluster.WaitUntil(3*time.Minute, 2*time.Second, func() bool {
		multisig, fee := getMultisigAndFeeAmount(t, ctx, apex, apiKey, cardanofw.ChainIDVector)

		return multisig == vectorConfig.FundAmount && fee == vectorConfig.FundFeeAmount
	}))

	sendAmountDfm := cardanofw.WeiToDfm(ethgo.Ether(1))

	proposer := cluster.Servers[0]

	proposerAcc, err := helper.GetAccountFromDir(proposer.DataDir())
	require.NoError(t, err)

	relayer, err := txrelayer.NewTxRelayer(txrelayer.WithClient(proposer.JSONRPC()))
	require.NoError(t, err)

	polybftCfg, err := polybft.LoadPolyBFTConfig(path.Join(cluster.Config.TmpDir, chainConfigFileName))
	require.NoError(t, err)

	removeValidator := cluster.Servers[4]
	removeValidatorKey, err := helper.GetAccountFromDir(removeValidator.DataDir())
	require.NoError(t, err)

	// wait some time until funding is processed and last observed slot updated on Bridge SC
	<-time.After(time.Minute)

	executeValidatorChangeProposal(t, ctx, relayer, proposerAcc, nil,
		[]types.Address{removeValidatorKey.Address()}, cluster, polybftCfg)

	t.Log("Executed validator set change")

	// Stop the 1st Blade
	require.NoError(t, removeValidator.Stop())

	// wait until at least 1 batch is executed
	require.NoError(t, cluster.WaitUntil(3*time.Minute, 2*time.Second, func() bool {
		multisig, fee := getMultisigAndFeeAmount(t, ctx, apex, apiKey, cardanofw.ChainIDPrime)

		return multisig < primeConfig.FundAmount && fee < primeConfig.FundFeeAmount
	}))

	// Stop the 2nd Blade
	require.NoError(t, cluster.Servers[3].Stop())

	// wait some time and start the 1st Blade
	<-time.After(10 * time.Second)
	require.NoError(t, removeValidator.Start())

	// wait for validator set change to finish
	waitUntilValidatorSetUpdateIsFinished(t, cluster, relayer, 10*time.Minute, 10*time.Second)

	t.Log("Finished VSC")

	checkValidatorActive(t, removeValidatorKey.Address(), relayer, false)
	checkValidatorStake(t, removeValidatorKey.Address(), relayer, big.NewInt(0))

	t.Logf("Removed validator")

	// stop removed validator and one more
	require.NoError(t, removeValidator.Stop())
	require.NoError(t, cluster.Servers[2].Stop())

	// Start the 2nd Blade, now we have 0, 1 and 3 working
	require.NoError(t, cluster.Servers[3].Start())

	// create new multisig and fee addresses
	require.NoError(t, apex.UpdateConfigs())

	// restart needed bridges & stop ones not used
	require.NoError(t, apex.RestartBridges(ctx, 2, 4))

	// check on new multisig
	require.NoError(t, cluster.WaitUntil(3*time.Minute, 2*time.Second, func() bool {
		multisig, fee := getMultisigAndFeeAmount(t, ctx, apex, apiKey, cardanofw.ChainIDPrime)

		t.Log("prime multisig", multisig)

		return multisig == primeConfig.FundAmount && fee > 0 && fee < primeConfig.FundFeeAmount
	}))

	require.NoError(t, cluster.WaitUntil(3*time.Minute, 2*time.Second, func() bool {
		multisig, fee := getMultisigAndFeeAmount(t, ctx, apex, apiKey, cardanofw.ChainIDVector)

		t.Log("vector multisig", multisig)

		return multisig == vectorConfig.FundAmount && fee > 0 && fee < vectorConfig.FundFeeAmount
	}))

	e2ehelper.ExecuteBridging(t, ctx, apex, 1,
		apex.Users[:1],
		apex.Users[1:2],
		[]string{cardanofw.ChainIDPrime, cardanofw.ChainIDVector, cardanofw.ChainIDNexus},
		map[string][]string{
			cardanofw.ChainIDPrime:  {cardanofw.ChainIDVector, cardanofw.ChainIDNexus},
			cardanofw.ChainIDVector: {cardanofw.ChainIDPrime},
			cardanofw.ChainIDNexus:  {cardanofw.ChainIDPrime},
		},
		sendAmountDfm)
}

// The test stops 1 apex bridge before VSU and 2nd during VSU, then starts the 1st one. After VSU is done
// we start the 2nd one and checking if bridge continues with batching with 2nd one apex bridge included in
// consensus. It is expected to continue working and that new multisig utxos are transferred entirely to all nodes.
func TestE2E_DynamicValidators_StopApxBridgesDuringVSU(t *testing.T) {
	if cardanofw.ShouldSkipE2RRedundantTests() {
		t.Skip()
	}

	const (
		apiKey  = "test_api_key"
		userCnt = 40
	)

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	primeConfig, vectorConfig := cardanofw.NewPrimeChainConfig(), cardanofw.NewVectorChainConfig(true)
	primeConfig.BridgeAddrHasStake = true
	primeConfig.PremineAmount = 500_000_000
	primeConfig.FundUTxOCount = 80
	primeConfig.FundFeeUTxOCount = 80
	vectorConfig.BridgeAddrHasStake = true
	vectorConfig.PremineAmount = 500_000_000
	vectorConfig.FundUTxOCount = 80
	vectorConfig.FundFeeUTxOCount = 80

	apex := cardanofw.SetupAndRunApexBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithUserCnt(userCnt),
		cardanofw.WithPrimeConfig(primeConfig),
		cardanofw.WithVectorConfig(vectorConfig),
		cardanofw.WithAPIValidatorID(-1),
		cardanofw.WithValidators(5),
		cardanofw.WithNexusEnabled(true),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	t.Log("Cluster started")

	cluster := apex.BridgeCluster

	require.NoError(t, cluster.WaitUntil(3*time.Minute, 2*time.Second, func() bool {
		multisig, fee := getMultisigAndFeeAmount(t, ctx, apex, apiKey, cardanofw.ChainIDPrime)

		return multisig == primeConfig.FundAmount && fee == primeConfig.FundFeeAmount
	}))

	require.NoError(t, cluster.WaitUntil(3*time.Minute, 2*time.Second, func() bool {
		multisig, fee := getMultisigAndFeeAmount(t, ctx, apex, apiKey, cardanofw.ChainIDVector)

		return multisig == vectorConfig.FundAmount && fee == vectorConfig.FundFeeAmount
	}))

	sendAmountDfm := cardanofw.WeiToDfm(ethgo.Ether(1))

	proposer := cluster.Servers[0]

	proposerAcc, err := helper.GetAccountFromDir(proposer.DataDir())
	require.NoError(t, err)

	relayer, err := txrelayer.NewTxRelayer(txrelayer.WithClient(proposer.JSONRPC()))
	require.NoError(t, err)

	polybftCfg, err := polybft.LoadPolyBFTConfig(path.Join(cluster.Config.TmpDir, chainConfigFileName))
	require.NoError(t, err)

	removeValidator := cluster.Servers[4]
	removeValidatorKey, err := helper.GetAccountFromDir(removeValidator.DataDir())
	require.NoError(t, err)

	// wait some time until funding is processed and last observed slot updated on Bridge SC
	<-time.After(time.Minute)

	executeValidatorChangeProposal(t, ctx, relayer, proposerAcc, nil,
		[]types.Address{removeValidatorKey.Address()}, cluster, polybftCfg)

	t.Log("Executed validator set change")

	// Stop the 1st apex bridge
	require.NoError(t, apex.GetValidator(t, 4).Stop())

	// wait until at least 1 batch is executed
	require.NoError(t, cluster.WaitUntil(3*time.Minute, 2*time.Second, func() bool {
		multisig, fee := getMultisigAndFeeAmount(t, ctx, apex, apiKey, cardanofw.ChainIDPrime)

		return multisig < primeConfig.FundAmount && fee < primeConfig.FundFeeAmount
	}))

	// Stop the 2nd apex bridge
	require.NoError(t, apex.GetValidator(t, 3).Stop())

	// wait some time and start the 1st apex pridge
	<-time.After(10 * time.Second)
	require.NoError(t, apex.GetValidator(t, 4).Start(ctx, true))

	// wait for validator set change to finish
	waitUntilValidatorSetUpdateIsFinished(t, cluster, relayer, 10*time.Minute, 10*time.Second)

	t.Log("Finished VSC")

	checkValidatorActive(t, removeValidatorKey.Address(), relayer, false)
	checkValidatorStake(t, removeValidatorKey.Address(), relayer, big.NewInt(0))

	t.Logf("Removed validator")

	// stop removed validator and one more
	require.NoError(t, removeValidator.Stop())
	require.NoError(t, cluster.Servers[2].Stop())

	// Start the 2nd apex bridge, now we have 0, 1 and 3 working
	require.NoError(t, apex.GetValidator(t, 3).Start(ctx, true))

	// create new multisig and fee addresses
	require.NoError(t, apex.UpdateConfigs())

	// restart needed bridges & stop ones not used
	require.NoError(t, apex.RestartBridges(ctx, 2, 4))

	// check on new multisig
	require.NoError(t, cluster.WaitUntil(3*time.Minute, 2*time.Second, func() bool {
		multisig, fee := getMultisigAndFeeAmount(t, ctx, apex, apiKey, cardanofw.ChainIDPrime)

		t.Log("prime multisig", multisig)

		return multisig == primeConfig.FundAmount && fee > 0 && fee < primeConfig.FundFeeAmount
	}))

	require.NoError(t, cluster.WaitUntil(3*time.Minute, 2*time.Second, func() bool {
		multisig, fee := getMultisigAndFeeAmount(t, ctx, apex, apiKey, cardanofw.ChainIDVector)

		t.Log("vector multisig", multisig)

		return multisig == vectorConfig.FundAmount && fee > 0 && fee < vectorConfig.FundFeeAmount
	}))

	e2ehelper.ExecuteBridging(t, ctx, apex, 1,
		apex.Users[:1],
		apex.Users[1:2],
		[]string{cardanofw.ChainIDPrime, cardanofw.ChainIDVector, cardanofw.ChainIDNexus},
		map[string][]string{
			cardanofw.ChainIDPrime:  {cardanofw.ChainIDVector, cardanofw.ChainIDNexus},
			cardanofw.ChainIDVector: {cardanofw.ChainIDPrime},
			cardanofw.ChainIDNexus:  {cardanofw.ChainIDPrime},
		},
		sendAmountDfm)
}

// The test starts 5 validators (0-4), in the first VSU operation removes validator #3
// and adds a new validator, in the second VSU operation removes previously added validator.
func TestE2E_DynamicValidators_AddRemoveAndRemoveValidator(t *testing.T) {
	if cardanofw.ShouldSkipE2RRedundantTests() {
		t.Skip()
	}

	const (
		apiKey  = "test_api_key"
		userCnt = 40
	)

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	primeConfig, vectorConfig := cardanofw.NewPrimeChainConfig(), cardanofw.NewVectorChainConfig(true)
	primeConfig.BridgeAddrHasStake = true
	primeConfig.PremineAmount = 500_000_000
	vectorConfig.BridgeAddrHasStake = true
	vectorConfig.PremineAmount = 500_000_000

	apex := cardanofw.SetupAndRunApexBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithUserCnt(userCnt),
		cardanofw.WithPrimeConfig(primeConfig),
		cardanofw.WithVectorConfig(vectorConfig),
		cardanofw.WithAPIValidatorID(-1),
		cardanofw.WithValidators(5),
		cardanofw.WithNonValidators(1),
		cardanofw.WithNexusEnabled(true),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	t.Log("Cluster started")

	cluster := apex.BridgeCluster

	require.NoError(t, cluster.WaitUntil(3*time.Minute, 2*time.Second, func() bool {
		multisig, fee := getMultisigAndFeeAmount(t, ctx, apex, apiKey, cardanofw.ChainIDPrime)

		return multisig == primeConfig.FundAmount && fee == primeConfig.FundFeeAmount
	}))

	require.NoError(t, cluster.WaitUntil(3*time.Minute, 2*time.Second, func() bool {
		multisig, fee := getMultisigAndFeeAmount(t, ctx, apex, apiKey, cardanofw.ChainIDVector)

		return multisig == vectorConfig.FundAmount && fee == vectorConfig.FundFeeAmount
	}))

	newValidatorSrv := cluster.Servers[5]

	proposer := cluster.Servers[0]

	proposerAcc, err := helper.GetAccountFromDir(proposer.DataDir())
	require.NoError(t, err)

	relayer, err := txrelayer.NewTxRelayer(txrelayer.WithClient(proposer.JSONRPC()))
	require.NoError(t, err)

	polybftCfg, err := polybft.LoadPolyBFTConfig(path.Join(cluster.Config.TmpDir, chainConfigFileName))
	require.NoError(t, err)

	newValidatorAcc, err := helper.GetAccountFromDir(newValidatorSrv.DataDir())
	require.NoError(t, err)

	removeValidator := cluster.Servers[3]
	removeValidatorKey, err := helper.GetAccountFromDir(removeValidator.DataDir())
	require.NoError(t, err)

	// generate for non validator
	require.NoError(t, apex.AddNewValidator(t, ctx, newValidatorSrv).Start(ctx, false))

	primeKeys := getMultisigAndFeeFromDataDir(t, newValidatorSrv.DataDir(), "prime")
	vectorKeys := getMultisigAndFeeFromDataDir(t, newValidatorSrv.DataDir(), "vector")

	// wait some time until funding is processed and last observed slot updated on Bridge SC
	<-time.After(time.Minute)

	executeValidatorChangeProposal(t, ctx, relayer, proposerAcc, []*addedValidator{
		{
			Address: newValidatorAcc.Address(),
			Key:     newValidatorAcc.Bls.PublicKey(),
			CardanoLikeChains: []string{
				keysToStr("prime", &primeKeys),
				keysToStr("vector", &vectorKeys),
			},
		},
	}, []types.Address{removeValidatorKey.Address()}, cluster, polybftCfg)

	t.Log("Executed validator set change")

	// wait for vsc to be sent for sure
	currentBlock, err := proposer.JSONRPC().BlockNumber()
	require.NoError(t, err)

	require.NoError(t, cluster.WaitForBlock(currentBlock+10, time.Minute))

	// wait for validator set change to finish
	waitUntilValidatorSetUpdateIsFinished(t, cluster, relayer, 10*time.Minute, 10*time.Second)

	t.Log("Finished VSC")

	checkValidatorActive(t, newValidatorAcc.Address(), relayer, true)
	checkValidatorActive(t, removeValidatorKey.Address(), relayer, false)
	// check stake amount to be equal to staked amount on the 1st validator
	checkValidatorStake(t, newValidatorAcc.Address(), relayer, cluster.Config.StakeAmounts[0])
	checkValidatorStake(t, removeValidatorKey.Address(), relayer, big.NewInt(0))

	t.Logf("Added new validator")

	// stop removed validator
	require.NoError(t, removeValidator.Stop())

	// create new multisig and fee addresses
	require.NoError(t, apex.UpdateConfigs())

	// restart validators & stop ones not used
	require.NoError(t, apex.RestartBridges(ctx, 3))

	// check on new multisig
	require.NoError(t, cluster.WaitUntil(3*time.Minute, 2*time.Second, func() bool {
		multisig, fee := getMultisigAndFeeAmount(t, ctx, apex, apiKey, cardanofw.ChainIDPrime)

		t.Log("prime multisig", multisig)

		return multisig == primeConfig.FundAmount && fee > 0 && fee < primeConfig.FundFeeAmount
	}))

	require.NoError(t, cluster.WaitUntil(3*time.Minute, 2*time.Second, func() bool {
		multisig, fee := getMultisigAndFeeAmount(t, ctx, apex, apiKey, cardanofw.ChainIDVector)

		t.Log("vector multisig", multisig)

		return multisig == vectorConfig.FundAmount && fee > 0 && fee < vectorConfig.FundFeeAmount
	}))

	t.Log("Removing previously added validator")

	// now remove the previously added validator
	executeValidatorChangeProposal(t, ctx, relayer, proposerAcc, nil,
		[]types.Address{newValidatorAcc.Address()}, cluster, polybftCfg)

	t.Log("Executed validator set change")

	// wait for vsc to be sent for sure
	currentBlock, err = proposer.JSONRPC().BlockNumber()
	require.NoError(t, err)

	require.NoError(t, cluster.WaitForBlock(currentBlock+10, time.Minute))

	// wait for validator set change to finish
	waitUntilValidatorSetUpdateIsFinished(t, cluster, relayer, 5*time.Minute, 10*time.Second)

	t.Log("Finished VSC")

	checkValidatorActive(t, newValidatorAcc.Address(), relayer, false)
	checkValidatorStake(t, newValidatorAcc.Address(), relayer, big.NewInt(0))

	t.Logf("Removed validator")

	// stop removed validator
	require.NoError(t, newValidatorSrv.Stop())

	// create new multisig and fee addresses
	require.NoError(t, apex.UpdateConfigs())

	// restart validators & stop ones not used
	require.NoError(t, apex.RestartBridges(ctx, 3, 5))

	// check on new multisig
	require.NoError(t, cluster.WaitUntil(3*time.Minute, 2*time.Second, func() bool {
		multisig, fee := getMultisigAndFeeAmount(t, ctx, apex, apiKey, cardanofw.ChainIDPrime)

		t.Log("prime multisig", multisig)

		return multisig == primeConfig.FundAmount && fee > 0 && fee < primeConfig.FundFeeAmount
	}))

	require.NoError(t, cluster.WaitUntil(3*time.Minute, 2*time.Second, func() bool {
		multisig, fee := getMultisigAndFeeAmount(t, ctx, apex, apiKey, cardanofw.ChainIDVector)

		t.Log("vector multisig", multisig)

		return multisig == vectorConfig.FundAmount && fee > 0 && fee < vectorConfig.FundFeeAmount
	}))
}

func TestE2E_DynamicValidators_AddValidatorSyncFromStart(t *testing.T) {
	const (
		apiKey  = "test_api_key"
		userCnt = 10
	)

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	primeConfig, vectorConfig := cardanofw.NewPrimeChainConfig(), cardanofw.NewVectorChainConfig(true)
	primeConfig.BridgeAddrHasStake = true
	primeConfig.PremineAmount = 500_000_000
	vectorConfig.BridgeAddrHasStake = true
	vectorConfig.PremineAmount = 500_000_000

	apex := cardanofw.SetupAndRunApexBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithUserCnt(userCnt),
		cardanofw.WithPrimeConfig(primeConfig),
		cardanofw.WithVectorConfig(vectorConfig),
		cardanofw.WithAPIValidatorID(-1),
		cardanofw.WithValidators(4),
		cardanofw.WithNonValidators(1),
		cardanofw.WithNexusEnabled(true),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	cluster := apex.BridgeCluster

	newValidatorSrv := cluster.Servers[4]

	t.Log("Cluster started")

	require.NoError(t, cluster.WaitUntil(3*time.Minute, 2*time.Second, func() bool {
		multisig, fee := getMultisigAndFeeAmount(t, ctx, apex, apiKey, cardanofw.ChainIDPrime)

		return multisig == primeConfig.FundAmount && fee == primeConfig.FundFeeAmount
	}))

	require.NoError(t, cluster.WaitUntil(3*time.Minute, 2*time.Second, func() bool {
		multisig, fee := getMultisigAndFeeAmount(t, ctx, apex, apiKey, cardanofw.ChainIDVector)

		return multisig == vectorConfig.FundAmount && fee == vectorConfig.FundFeeAmount
	}))

	require.NoError(t, newValidatorSrv.Stop(true))

	executeBridging := func() {
		t.Helper()

		e2ehelper.ExecuteBridging(t, ctx, apex, 1,
			apex.Users[:1],
			apex.Users[1:2],
			[]string{cardanofw.ChainIDPrime, cardanofw.ChainIDVector, cardanofw.ChainIDNexus},
			map[string][]string{
				cardanofw.ChainIDPrime:  {cardanofw.ChainIDVector, cardanofw.ChainIDNexus},
				cardanofw.ChainIDVector: {cardanofw.ChainIDPrime},
				cardanofw.ChainIDNexus:  {cardanofw.ChainIDPrime},
			},
			cardanofw.WeiToDfm(ethgo.Ether(1)))
	}

	executeBridging()

	proposer := cluster.Servers[0]

	proposerAcc, err := helper.GetAccountFromDir(proposer.DataDir())
	require.NoError(t, err)

	relayer, err := txrelayer.NewTxRelayer(txrelayer.WithClient(proposer.JSONRPC()))
	require.NoError(t, err)

	polybftCfg, err := polybft.LoadPolyBFTConfig(path.Join(cluster.Config.TmpDir, chainConfigFileName))
	require.NoError(t, err)

	newValidatorAcc, err := helper.GetAccountFromDir(newValidatorSrv.DataDir())
	require.NoError(t, err)

	// generate for non validator
	newValidator := apex.AddNewValidator(t, ctx, newValidatorSrv)

	primeKeys := getMultisigAndFeeFromDataDir(t, newValidatorSrv.DataDir(), "prime")
	vectorKeys := getMultisigAndFeeFromDataDir(t, newValidatorSrv.DataDir(), "vector")

	executeValidatorChangeProposal(t, ctx, relayer, proposerAcc, []*addedValidator{
		{
			Address: newValidatorAcc.Address(),
			Key:     newValidatorAcc.Bls.PublicKey(),
			CardanoLikeChains: []string{
				keysToStr("prime", &primeKeys),
				keysToStr("vector", &vectorKeys),
			},
		},
	}, nil, cluster, polybftCfg)

	t.Log("Executed validator set change")

	// wait for vsc to be sent for sure
	currentBlock, err := proposer.JSONRPC().BlockNumber()
	require.NoError(t, err)

	require.NoError(t, cluster.WaitForBlock(currentBlock+10, time.Minute))

	// wait for validator set change to finish
	waitUntilValidatorSetUpdateIsFinished(t, cluster, relayer, 5*time.Minute, 10*time.Second)

	t.Log("Finished VSC")

	// The blade must start first because its synchronization lags behind;
	// multisig addresses must be updated before the Cardano indexers
	// process the corresponding blocks.
	require.NoError(t, newValidatorSrv.Start())
	time.Sleep(90 * time.Second)
	require.NoError(t, newValidator.Start(ctx, false))

	checkValidatorActive(t, newValidatorAcc.Address(), relayer, true)
	// check stake amount to be equal to staked amount on the 1st validator
	checkValidatorStake(t, newValidatorAcc.Address(), relayer, cluster.Config.StakeAmounts[0])

	t.Logf("Added new validator")

	require.NoError(t, apex.UpdateConfigs())

	// stop blade node for validator with index 1
	require.NoError(t, cluster.Servers[1].Stop())

	// restart all validators except the one with index 1
	require.NoError(t, apex.RestartBridges(ctx, 1))

	time.Sleep(20 * time.Second)
	// check if bridging is working with the new validator added without validator with index 1
	executeBridging()
}

func addressToHex(address []byte) string {
	return hex.EncodeToHex(address)[2:]
}

func getMultisigAndFeeFromDataDir(t *testing.T, dataDir, chain string) (keys cardanofw.CardanoWallet) {
	t.Helper()

	secretsManager, err := secretsHelper.CreateSecretsManager(&secretsCardano.SecretsManagerConfig{
		Path: dataDir,
		Type: secretsCardano.Local,
	})
	require.NoError(t, err)

	secret, err := secretsManager.GetSecret(fmt.Sprintf("%s%s_key", secretsCardano.CardanoKeyLocalPrefix, chain))
	require.NoError(t, err)

	require.NoError(t, json.Unmarshal(secret, &keys))

	return keys
}

func checkValidatorActive(t *testing.T, address types.Address,
	relayer txrelayer.TxRelayer, isAdded bool) {
	t.Helper()

	getValidatorFn := contractsapi.GetValidatorStakeManagerFn{
		Validator_: address,
	}

	input, err := getValidatorFn.EncodeAbi()
	require.NoError(t, err)

	data, err := relayer.Call(types.ZeroAddress, contracts.StakeManagerContract, input)
	require.NoError(t, err)

	outputs := contractsapi.StakeManager.Abi.Methods["getValidator"].Outputs

	byteHex, err := hex.DecodeHex(data)
	require.NoError(t, err)

	mappedOutput, err := outputs.Decode(byteHex)
	require.NoError(t, err)

	mapped, ok := mappedOutput.(map[string]interface{})
	require.True(t, ok)

	validatorData, ok := mapped["0"]
	require.True(t, ok)

	validatorDataMap, ok := validatorData.(map[string]interface{})
	require.True(t, ok)

	require.Equal(t, isAdded, validatorDataMap["isActive"])
}

func checkValidatorStake(t *testing.T, address types.Address, relayer txrelayer.TxRelayer, expected *big.Int) {
	t.Helper()

	stakeOfFn := contractsapi.StakeOfStakeManagerFn{
		Validator: address,
	}

	input, err := stakeOfFn.EncodeAbi()
	require.NoError(t, err)

	data, err := relayer.Call(types.ZeroAddress, contracts.StakeManagerContract, input)
	require.NoError(t, err)

	stake := new(big.Int)
	stake.SetString(data[2:], 16)

	require.Equal(t, 0, expected.Cmp(stake))
}

func executeValidatorChangeProposal(
	t *testing.T, ctx context.Context, relayer txrelayer.TxRelayer, proposerAcc *wallet.Account,
	addedValidators []*addedValidator, removedValidators []types.Address,
	cluster *framework.TestCluster, polybftCfg polybft.PolyBFTConfig,
) {
	t.Helper()
	// A validator's oracle in the validator components process may send a claim transaction at the
	// same moment it attempts to execute part of the proposal process.
	execWithRetry := func(handler func() error) error {
		_, err := infracommon.ExecuteWithRetry(ctx, func(ctx context.Context) (string, error) {
			err := handler()
			if err != nil && strings.Contains(err.Error(), "replacement tx underpriced") {
				return "", infracommon.ErrRetryTryAgain
			}

			return "", err
		}, infracommon.WithRetryCount(20), infracommon.WithRetryWaitTime(time.Second*5))

		return err
	}

	description := "validatorSetChange"

	tmpDir, err := os.MkdirTemp("", "validator-change-proposal")
	require.NoError(t, err)

	defer os.RemoveAll(tmpDir)

	filePath := filepath.Join(tmpDir, "proposal.json")

	server := cluster.Servers[0]

	for _, added := range addedValidators {
		require.NoError(t, server.AddValidatorToVSCProposal(filePath, added.Address, added.CardanoLikeChains, addressToHex(added.Key.Marshal()), true))
	}

	for _, removed := range removedValidators {
		require.NoError(t, server.RemoveValidatorToVSCProposal(filePath, removed))
	}

	key, err := proposerAcc.Ecdsa.MarshallPrivateKey()
	require.NoError(t, err)

	hexKey := addressToHex(key)

	var submitResult *submit.SubmitResult
	// the proposer validator should propose validator set change
	require.NoError(t, execWithRetry(func() error {
		submitResult, err = server.SubmitProposal(filePath, hexKey, description)

		return err
	}))

	proposalIDBig, ok := new(big.Int).SetString(submitResult.ProposalID, 10)
	require.True(t, ok)

	require.NoError(t, cluster.WaitUntil(3*time.Minute, 2*time.Second, func() bool {
		proposalState := getProposalState(t, proposalIDBig,
			polybftCfg.GovernanceConfig.ChildGovernorAddr, relayer)

		return proposalState == Active
	}))

	wg := sync.WaitGroup{}
	errs := make([]error, len(cluster.Servers))

	for i, s := range cluster.Servers {
		wg.Add(1)

		go func(i int, s *framework.TestServer) {
			defer wg.Done()

			voterAcc, err := helper.GetAccountFromDir(s.DataDir())
			if err != nil {
				errs[i] = err

				return
			}

			voteKey, err := voterAcc.Ecdsa.MarshallPrivateKey()
			if err != nil {
				errs[i] = err

				return
			}

			// a quorum of validators is required to vote
			errs[i] = execWithRetry(func() error {
				return server.VoteProposal(submitResult.ProposalID, addressToHex(voteKey), false)
			})
		}(i, s)
	}

	wg.Wait()

	require.NoError(t, errors.Join(errs...))

	require.NoError(t, cluster.WaitUntil(3*time.Minute, 2*time.Second, func() bool {
		proposalState := getProposalState(t, proposalIDBig,
			polybftCfg.GovernanceConfig.ChildGovernorAddr, relayer)

		return proposalState == Succeeded
	}))
	// the proposer validator is responsible for queuing the proposal
	require.NoError(t, execWithRetry(func() error {
		return server.QueueProposal(submitResult.Input, description, hexKey)
	}))

	require.NoError(t, cluster.WaitUntil(3*time.Minute, 2*time.Second, func() bool {
		proposalState := getProposalState(t, proposalIDBig,
			polybftCfg.GovernanceConfig.ChildGovernorAddr, relayer)

		return proposalState == Queued
	}))

	currentBlockNumber, err := relayer.Client().BlockNumber()
	require.NoError(t, err)

	require.NoError(t, cluster.WaitForBlock(currentBlockNumber+2, 10*time.Second))
	// the proposer validator is responsible for executing the proposal
	require.NoError(t, execWithRetry(func() error {
		return server.ExecuteProposal(submitResult.Input, description, hexKey)
	}))
}

type addedValidator struct {
	Address           types.Address
	Key               *bn256.PublicKey
	CardanoLikeChains []string
}

type validatorSetState struct {
	BlockNumber          uint64                                         `json:"block"`
	EpochID              uint64                                         `json:"epoch"`
	UpdatedAtBlockNumber uint64                                         `json:"updated_at_block"`
	Validators           map[types.Address]*validator.ValidatorMetadata `json:"validators"`
}

func (vs *validatorSetState) Unmarshal(b []byte) error {
	return json.Unmarshal(b, vs)
}

// check multisig amount
func getMultisigAndFeeAmount(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, apiKey string, chainID cardanofw.ChainID,
) (uint64, uint64) {
	t.Helper()

	apiURL, err := apex.GetBridgingAPI()
	require.NoError(t, err)

	requestURL := fmt.Sprintf("%s/api/OracleState/Get?chainId=%s", apiURL, chainID)
	currentState, err := cardanofw.GetOracleState(ctx, requestURL, apiKey)

	if err != nil || currentState == nil {
		return 0, 0
	}

	var multisigAddr, feeAddr string

	switch chainID {
	case cardanofw.ChainIDPrime:
		multisigAddr, feeAddr = apex.PrimeInfo.MultisigAddr, apex.PrimeInfo.FeeAddr
	case cardanofw.ChainIDVector:
		multisigAddr, feeAddr = apex.VectorInfo.MultisigAddr, apex.VectorInfo.FeeAddr
	}

	sumMultiSig, sumFee := uint64(0), uint64(0)

	for _, utxo := range currentState.Utxos {
		switch utxo.Address {
		case multisigAddr:
			sumMultiSig += utxo.Amount
		case feeAddr:
			sumFee += utxo.Amount
		}
	}

	return sumMultiSig, sumFee
}

func waitUntilValidatorSetUpdateIsFinished(
	t *testing.T, cluster *framework.TestCluster, relayer txrelayer.TxRelayer,
	timeout, pullFrequency time.Duration,
) {
	t.Helper()
	require.NoError(t, cluster.WaitUntil(timeout, pullFrequency, func() bool {
		input, err := (&contractsapi.IsNewValidatorSetPendingApexBridgeContractsBridgeFn{}).EncodeAbi()
		require.NoError(t, err)

		ret, err := relayer.Call(types.ZeroAddress, contracts.Bridge, input)
		require.NoError(t, err)

		num, err := hex.DecodeUint64(ret)
		require.NoError(t, err)

		t.Log("Validator set change status", num == 1)

		return num == 0
	}))
}

func keysToStr(chain string, keys *cardanofw.CardanoWallet) string {
	return fmt.Sprintf("%s:%s:%s:%s:%s",
		chain,
		addressToHex(keys.Multisig.VerificationKey),
		addressToHex(keys.MultisigFee.VerificationKey),
		addressToHex(keys.Multisig.StakeVerificationKey),
		addressToHex(keys.MultisigFee.StakeVerificationKey),
	)
}
