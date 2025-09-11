package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"path"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/bls"
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

	if cardanofw.ShouldSkipE2RRedundantTests() {
		t.Skip()
	}

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

	// check multisig amount
	getMultisigAndFeeAmount := func(chainID cardanofw.ChainID) (uint64, uint64) {
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

	require.NoError(t, cluster.WaitUntil(3*time.Minute, 2*time.Second, func() bool {
		multisig, fee := getMultisigAndFeeAmount(cardanofw.ChainIDPrime)

		return multisig == primeConfig.FundAmount && fee == primeConfig.FundFeeAmount
	}))

	require.NoError(t, cluster.WaitUntil(3*time.Minute, 2*time.Second, func() bool {
		multisig, fee := getMultisigAndFeeAmount(cardanofw.ChainIDVector)

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
	apex.GenerateForNonValidator(t, ctx, 4)

	primeKeys := getMultisigAndFeeFromDataDir(t, newValidatorSrv.DataDir(), "prime")
	vectorKeys := getMultisigAndFeeFromDataDir(t, newValidatorSrv.DataDir(), "vector")

	keysToStr := func(chain string, keys *cardanofw.CardanoWallet) string {
		return fmt.Sprintf("%s:%s:%s:%s:%s",
			chain,
			addressToHex(keys.Multisig.VerificationKey),
			addressToHex(keys.MultisigFee.VerificationKey),
			addressToHex(keys.Multisig.StakeVerificationKey),
			addressToHex(keys.MultisigFee.StakeVerificationKey),
		)
	}

	executeValidatorChangeProposal(t, relayer, proposerAcc, []*addedValidator{
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
	require.NoError(t, cluster.WaitUntil(5*time.Minute, 10*time.Second, func() bool {
		input, err := (&contractsapi.IsNewValidatorSetPendingApexBridgeContractsBridgeFn{}).EncodeAbi()
		require.NoError(t, err)

		ret, err := relayer.Call(types.ZeroAddress, contracts.Bridge, input)
		require.NoError(t, err)

		num, err := hex.DecodeUint64(ret)
		require.NoError(t, err)

		t.Log("Validator set change status", num == 1)

		return num == 0
	}))

	t.Log("Finished VSC")

	checkValidatorActive(t, newValidatorAcc.Address(), relayer, true)

	t.Logf("Added new validator")

	// stop one of validators to check if new validator participates in voting
	require.NoError(t, cluster.Servers[1].Stop())

	// create new multisig and fee addresses
	require.NoError(t, apex.UpdateConfigs())

	// restart validators
	require.NoError(t, apex.RestartBridges(ctx, 1))

	// check on new multisig
	require.NoError(t, cluster.WaitUntil(3*time.Minute, 2*time.Second, func() bool {
		multisig, fee := getMultisigAndFeeAmount(cardanofw.ChainIDPrime)

		t.Log("prime multisig", multisig)

		return multisig == primeConfig.FundAmount && fee > 0 && fee < primeConfig.FundFeeAmount
	}))

	require.NoError(t, cluster.WaitUntil(3*time.Minute, 2*time.Second, func() bool {
		multisig, fee := getMultisigAndFeeAmount(cardanofw.ChainIDVector)

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

	if cardanofw.ShouldSkipE2RRedundantTests() {
		t.Skip()
	}

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

	// check multisig amount
	getMultisigAndFeeAmount := func(chainID cardanofw.ChainID) (uint64, uint64) {
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

	require.NoError(t, cluster.WaitUntil(3*time.Minute, 2*time.Second, func() bool {
		multisig, fee := getMultisigAndFeeAmount(cardanofw.ChainIDPrime)

		return multisig == primeConfig.FundAmount && fee == primeConfig.FundFeeAmount
	}))

	require.NoError(t, cluster.WaitUntil(3*time.Minute, 2*time.Second, func() bool {
		multisig, fee := getMultisigAndFeeAmount(cardanofw.ChainIDVector)

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

	executeValidatorChangeProposal(t, relayer, proposerAcc, nil,
		[]types.Address{removeValidatorKey.Address()}, cluster, polybftCfg)

	t.Log("Executed validator set change")

	// wait for vsc to be sent for sure
	currentBlock, err := proposer.JSONRPC().BlockNumber()
	require.NoError(t, err)

	require.NoError(t, cluster.WaitForBlock(currentBlock+10, time.Minute))

	// wait for validator set change to finish
	require.NoError(t, cluster.WaitUntil(5*time.Minute, 10*time.Second, func() bool {
		input, err := (&contractsapi.IsNewValidatorSetPendingApexBridgeContractsBridgeFn{}).EncodeAbi()
		require.NoError(t, err)

		ret, err := relayer.Call(types.ZeroAddress, contracts.Bridge, input)
		require.NoError(t, err)

		num, err := hex.DecodeUint64(ret)
		require.NoError(t, err)

		t.Log("Validator set change status", num == 1)

		return num == 0
	}))

	t.Log("Finished VSC")

	checkValidatorActive(t, removeValidatorKey.Address(), relayer, false)

	t.Logf("Removed validator")

	// stop removed validator
	require.NoError(t, removeValidator.Stop())

	// create new multisig and fee addresses
	require.NoError(t, apex.UpdateConfigs())

	// restart some validators & stop ones not used
	require.NoError(t, apex.RestartBridges(ctx, 4))

	// check on new multisig
	require.NoError(t, cluster.WaitUntil(3*time.Minute, 2*time.Second, func() bool {
		multisig, fee := getMultisigAndFeeAmount(cardanofw.ChainIDPrime)

		t.Log("prime multisig", multisig)

		return multisig == primeConfig.FundAmount && fee > 0 && fee < primeConfig.FundFeeAmount
	}))

	require.NoError(t, cluster.WaitUntil(3*time.Minute, 2*time.Second, func() bool {
		multisig, fee := getMultisigAndFeeAmount(cardanofw.ChainIDVector)

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

	if cardanofw.ShouldSkipE2RRedundantTests() {
		t.Skip()
	}

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

	// check multisig amount
	getMultisigAndFeeAmount := func(chainID cardanofw.ChainID) (uint64, uint64) {
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

	require.NoError(t, cluster.WaitUntil(3*time.Minute, 2*time.Second, func() bool {
		multisig, fee := getMultisigAndFeeAmount(cardanofw.ChainIDPrime)

		return multisig == primeConfig.FundAmount && fee == primeConfig.FundFeeAmount
	}))

	require.NoError(t, cluster.WaitUntil(3*time.Minute, 2*time.Second, func() bool {
		multisig, fee := getMultisigAndFeeAmount(cardanofw.ChainIDVector)

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
	apex.GenerateForNonValidator(t, ctx, 4)

	primeKeys := getMultisigAndFeeFromDataDir(t, newValidatorSrv.DataDir(), "prime")
	vectorKeys := getMultisigAndFeeFromDataDir(t, newValidatorSrv.DataDir(), "vector")

	keysToStr := func(chain string, keys *cardanofw.CardanoWallet) string {
		return fmt.Sprintf("%s:%s:%s:%s:%s",
			chain,
			addressToHex(keys.Multisig.VerificationKey),
			addressToHex(keys.MultisigFee.VerificationKey),
			addressToHex(keys.Multisig.StakeVerificationKey),
			addressToHex(keys.MultisigFee.StakeVerificationKey),
		)
	}

	// wait some time until funding is processed and last observed slot updated on Bridge SC
	<-time.After(time.Minute)

	executeValidatorChangeProposal(t, relayer, proposerAcc, []*addedValidator{
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
	require.NoError(t, cluster.WaitUntil(10*time.Minute, 10*time.Second, func() bool {
		input, err := (&contractsapi.IsNewValidatorSetPendingApexBridgeContractsBridgeFn{}).EncodeAbi()
		require.NoError(t, err)

		ret, err := relayer.Call(types.ZeroAddress, contracts.Bridge, input)
		require.NoError(t, err)

		num, err := hex.DecodeUint64(ret)
		require.NoError(t, err)

		t.Log("Validator set change status", num == 1)

		return num == 0
	}))

	t.Log("Finished VSC")

	checkValidatorActive(t, newValidatorAcc.Address(), relayer, true)
	checkValidatorActive(t, removeValidatorKey.Address(), relayer, false)

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
		multisig, fee := getMultisigAndFeeAmount(cardanofw.ChainIDPrime)

		t.Log("prime multisig", multisig)

		return multisig == primeConfig.FundAmount && fee > 0 && fee < primeConfig.FundFeeAmount
	}))

	require.NoError(t, cluster.WaitUntil(3*time.Minute, 2*time.Second, func() bool {
		multisig, fee := getMultisigAndFeeAmount(cardanofw.ChainIDVector)

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

	if cardanofw.ShouldSkipE2RRedundantTests() {
		t.Skip()
	}

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

	// check multisig amount
	getMultisigAndFeeAmount := func(chainID cardanofw.ChainID) (uint64, uint64) {
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

	require.NoError(t, cluster.WaitUntil(3*time.Minute, 2*time.Second, func() bool {
		multisig, fee := getMultisigAndFeeAmount(cardanofw.ChainIDPrime)

		return multisig == primeConfig.FundAmount && fee == primeConfig.FundFeeAmount
	}))

	require.NoError(t, cluster.WaitUntil(3*time.Minute, 2*time.Second, func() bool {
		multisig, fee := getMultisigAndFeeAmount(cardanofw.ChainIDVector)

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

	executeValidatorChangeProposal(t, relayer, proposerAcc, nil,
		[]types.Address{removeValidatorKey.Address()}, cluster, polybftCfg)

	t.Log("Executed validator set change")

	// wait for vsc to be sent for sure
	currentBlock, err := proposer.JSONRPC().BlockNumber()
	require.NoError(t, err)

	require.NoError(t, cluster.WaitForBlock(currentBlock+10, time.Minute))

	// wait for validator set change to finish
	require.NoError(t, cluster.WaitUntil(5*time.Minute, 10*time.Second, func() bool {
		input, err := (&contractsapi.IsNewValidatorSetPendingApexBridgeContractsBridgeFn{}).EncodeAbi()
		require.NoError(t, err)

		ret, err := relayer.Call(types.ZeroAddress, contracts.Bridge, input)
		require.NoError(t, err)

		num, err := hex.DecodeUint64(ret)
		require.NoError(t, err)

		t.Log("Validator set change status", num == 1)

		return num == 0
	}))

	t.Log("Finished VSC")

	checkValidatorActive(t, removeValidatorKey.Address(), relayer, false)

	t.Logf("Removed validator")

	// stop removed validator
	require.NoError(t, removeValidator.Stop())

	// create new multisig and fee addresses
	require.NoError(t, apex.UpdateConfigs())

	// restart some apex bridges & stop ones not used
	require.NoError(t, apex.RestartBridges(ctx, 4))

	// wait until bridge is initialized and check on new multisig
	<-time.After(15 * time.Second)

	multisig, fee := getMultisigAndFeeAmount(cardanofw.ChainIDPrime)
	t.Log("prime multisig", multisig)

	require.Zero(t, multisig)
	require.Zero(t, fee)

	multisig, fee = getMultisigAndFeeAmount(cardanofw.ChainIDVector)
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
	const (
		apiKey  = "test_api_key"
		userCnt = 40
	)

	if cardanofw.ShouldSkipE2RRedundantTests() {
		t.Skip()
	}

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

	// check multisig amount
	getMultisigAndFeeAmount := func(chainID cardanofw.ChainID) (uint64, uint64) {
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

	require.NoError(t, cluster.WaitUntil(3*time.Minute, 2*time.Second, func() bool {
		multisig, fee := getMultisigAndFeeAmount(cardanofw.ChainIDPrime)

		return multisig == primeConfig.FundAmount && fee == primeConfig.FundFeeAmount
	}))

	require.NoError(t, cluster.WaitUntil(3*time.Minute, 2*time.Second, func() bool {
		multisig, fee := getMultisigAndFeeAmount(cardanofw.ChainIDVector)

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

	executeValidatorChangeProposal(t, relayer, proposerAcc, nil,
		[]types.Address{removeValidatorKey.Address()}, cluster, polybftCfg)

	t.Log("Executed validator set change")

	// Stop the 1st Blade
	require.NoError(t, removeValidator.Stop())

	// wait until at least 1 batch is executed
	require.NoError(t, cluster.WaitUntil(3*time.Minute, 2*time.Second, func() bool {
		multisig, fee := getMultisigAndFeeAmount(cardanofw.ChainIDPrime)

		return multisig < primeConfig.FundAmount && fee < primeConfig.FundFeeAmount
	}))

	// Stop the 2nd Blade
	require.NoError(t, cluster.Servers[3].Stop())

	// wait some time and start the 1st Blade
	<-time.After(10 * time.Second)
	require.NoError(t, removeValidator.Start())

	// wait for validator set change to finish
	require.NoError(t, cluster.WaitUntil(10*time.Minute, 10*time.Second, func() bool {
		input, err := (&contractsapi.IsNewValidatorSetPendingApexBridgeContractsBridgeFn{}).EncodeAbi()
		require.NoError(t, err)

		ret, err := relayer.Call(types.ZeroAddress, contracts.Bridge, input)
		require.NoError(t, err)

		num, err := hex.DecodeUint64(ret)
		require.NoError(t, err)

		t.Log("Validator set change status", num == 1)

		return num == 0
	}))

	t.Log("Finished VSC")

	checkValidatorActive(t, removeValidatorKey.Address(), relayer, false)

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
		multisig, fee := getMultisigAndFeeAmount(cardanofw.ChainIDPrime)

		t.Log("prime multisig", multisig)

		return multisig == primeConfig.FundAmount && fee > 0 && fee < primeConfig.FundFeeAmount
	}))

	require.NoError(t, cluster.WaitUntil(3*time.Minute, 2*time.Second, func() bool {
		multisig, fee := getMultisigAndFeeAmount(cardanofw.ChainIDVector)

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
func TestE2E_DynamicValidators_StopApexBridgesDuringVSU(t *testing.T) {
	const (
		apiKey  = "test_api_key"
		userCnt = 40
	)

	if cardanofw.ShouldSkipE2RRedundantTests() {
		t.Skip()
	}

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

	// check multisig amount
	getMultisigAndFeeAmount := func(chainID cardanofw.ChainID) (uint64, uint64) {
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

	require.NoError(t, cluster.WaitUntil(3*time.Minute, 2*time.Second, func() bool {
		multisig, fee := getMultisigAndFeeAmount(cardanofw.ChainIDPrime)

		return multisig == primeConfig.FundAmount && fee == primeConfig.FundFeeAmount
	}))

	require.NoError(t, cluster.WaitUntil(3*time.Minute, 2*time.Second, func() bool {
		multisig, fee := getMultisigAndFeeAmount(cardanofw.ChainIDVector)

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

	executeValidatorChangeProposal(t, relayer, proposerAcc, nil,
		[]types.Address{removeValidatorKey.Address()}, cluster, polybftCfg)

	t.Log("Executed validator set change")

	// Stop the 1st apex bridge
	require.NoError(t, apex.GetValidator(t, 4).Stop())

	// wait until at least 1 batch is executed
	require.NoError(t, cluster.WaitUntil(3*time.Minute, 2*time.Second, func() bool {
		multisig, fee := getMultisigAndFeeAmount(cardanofw.ChainIDPrime)

		return multisig < primeConfig.FundAmount && fee < primeConfig.FundFeeAmount
	}))

	// Stop the 2nd apex bridge
	require.NoError(t, apex.GetValidator(t, 3).Stop())

	// wait some time and start the 1st apex pridge
	<-time.After(10 * time.Second)
	require.NoError(t, apex.GetValidator(t, 4).Start(ctx, true))

	// wait for validator set change to finish
	require.NoError(t, cluster.WaitUntil(10*time.Minute, 10*time.Second, func() bool {
		input, err := (&contractsapi.IsNewValidatorSetPendingApexBridgeContractsBridgeFn{}).EncodeAbi()
		require.NoError(t, err)

		ret, err := relayer.Call(types.ZeroAddress, contracts.Bridge, input)
		require.NoError(t, err)

		num, err := hex.DecodeUint64(ret)
		require.NoError(t, err)

		t.Log("Validator set change status", num == 1)

		return num == 0
	}))

	t.Log("Finished VSC")

	checkValidatorActive(t, removeValidatorKey.Address(), relayer, false)

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
		multisig, fee := getMultisigAndFeeAmount(cardanofw.ChainIDPrime)

		t.Log("prime multisig", multisig)

		return multisig == primeConfig.FundAmount && fee > 0 && fee < primeConfig.FundFeeAmount
	}))

	require.NoError(t, cluster.WaitUntil(3*time.Minute, 2*time.Second, func() bool {
		multisig, fee := getMultisigAndFeeAmount(cardanofw.ChainIDVector)

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

func executeValidatorChangeProposal(t *testing.T, relayer txrelayer.TxRelayer, proposerAcc *wallet.Account,
	addedValidators []*addedValidator, removedValidators []types.Address, cluster *framework.TestCluster, polybftCfg polybft.PolyBFTConfig) {
	t.Helper()

	description := "validatorSetChange"

	filePath := fmt.Sprintf("test_proposal_%d", time.Now().UTC().UnixMilli())

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

	submitResult, err := server.SubmitProposal(filePath, hexKey, description)
	require.NoError(t, err)

	proposalIDBig, ok := new(big.Int).SetString(submitResult.ProposalID, 10)
	require.True(t, ok)

	require.NoError(t, cluster.WaitUntil(3*time.Minute, 2*time.Second, func() bool {
		proposalState := getProposalState(t, proposalIDBig,
			polybftCfg.GovernanceConfig.ChildGovernorAddr, relayer)

		return proposalState == Active
	}))

	for _, s := range cluster.Servers {
		voterAcc, err := helper.GetAccountFromDir(s.DataDir())
		require.NoError(t, err)

		voteKey, err := voterAcc.Ecdsa.MarshallPrivateKey()
		require.NoError(t, err)

		require.NoError(t, server.VoteProposal(submitResult.ProposalID, addressToHex(voteKey), false))
	}

	require.NoError(t, cluster.WaitUntil(3*time.Minute, 2*time.Second, func() bool {
		proposalState := getProposalState(t, proposalIDBig,
			polybftCfg.GovernanceConfig.ChildGovernorAddr, relayer)

		return proposalState == Succeeded
	}))

	require.NoError(t, server.QueueProposal(submitResult.Input, description, hexKey))

	require.NoError(t, cluster.WaitUntil(3*time.Minute, 2*time.Second, func() bool {
		proposalState := getProposalState(t, proposalIDBig,
			polybftCfg.GovernanceConfig.ChildGovernorAddr, relayer)

		return proposalState == Queued
	}))

	currentBlockNumber, err := relayer.Client().BlockNumber()
	require.NoError(t, err)

	require.NoError(t, cluster.WaitForBlock(currentBlockNumber+2, 10*time.Second))

	require.NoError(t, server.ExecuteProposal(submitResult.Input, description, hexKey))
}

type addedValidator struct {
	Address           types.Address
	Key               *bls.PublicKey
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
