package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"path"
	"path/filepath"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/bls"
	"github.com/0xPolygon/polygon-edge/command/validator/helper"
	"github.com/0xPolygon/polygon-edge/consensus/polybft"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/validator"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/cardanofw"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/framework"
	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/Ethernal-Tech/ethgo"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"
)

func TestE2E_DynamicValidators_AddValidator(t *testing.T) {
	const (
		epochSize = uint64(10)
	)

	validatorAcc, err := crypto.GenerateECDSAKey()
	require.NoError(t, err)

	blsKey, err := bls.GenerateBlsKey()
	require.NoError(t, err)

	cluster := framework.NewTestCluster(t, 4,
		framework.WithEpochSize(10),
		framework.WithGovernanceVotingDelay(1),
		framework.WithGovernanceVotingPeriod(3*epochSize),
		framework.WithPremine(validatorAcc.Address()),
		framework.WithTestBridge(),
	)
	defer cluster.Stop()

	cluster.WaitForReady(t)

	proposer := cluster.Servers[0]

	proposerAcc, err := helper.GetAccountFromDir(proposer.DataDir())
	require.NoError(t, err)

	relayer, err := txrelayer.NewTxRelayer(txrelayer.WithClient(proposer.JSONRPC()))
	require.NoError(t, err)

	polybftCfg, err := polybft.LoadPolyBFTConfig(path.Join(cluster.Config.TmpDir, chainConfigFileName))
	require.NoError(t, err)

	// approve native token
	approve := contractsapi.ApproveNativeERC20MintableFn{
		Spender: contracts.StakeManagerContract,
		Amount:  ethgo.Ether(1),
	}

	approveInput, err := approve.EncodeAbi()
	require.NoError(t, err)

	txn := types.NewTx(types.NewLegacyTx(
		types.WithFrom(validatorAcc.Address()),
		types.WithTo(&contracts.NativeERC20TokenContract),
		types.WithInput(approveInput),
	))

	recp, err := relayer.SendTransaction(txn, validatorAcc)
	require.NoError(t, err)
	require.NotNil(t, recp)
	require.Equal(t, recp.Status, uint64(types.ReceiptSuccess))

	// propose and execute validator set change
	addedValidators := []*addedValidator{
		{
			Address: validatorAcc.Address(),
			Key:     blsKey.PublicKey(),
		},
	}

	executeValidatorChangeProposal(t, relayer, proposerAcc, addedValidators, []types.Address{}, cluster, polybftCfg)

	// Check on stake manager
	currentBlockNumber, err := relayer.Client().BlockNumber()
	require.NoError(t, err)

	require.NoError(t, cluster.WaitForBlock(currentBlockNumber+epochSize, 2*time.Minute))

	checkValidatorActive(t, validatorAcc.Address(), relayer, true)

	require.NoError(t, proposer.Stop())

	validatorSet := getFullValidatorSet(t, proposer)
	validatorData, ok := validatorSet.Validators[validatorAcc.Address()]
	require.True(t, ok)
	require.NotNil(t, validatorData)
	require.True(t, validatorData.IsActive)
}

func TestE2E_DynamicValidators_RemoveValidator(t *testing.T) {
	const (
		epochSize = uint64(10)
	)

	cluster := framework.NewTestCluster(t, 5,
		framework.WithEpochSize(10),
		framework.WithGovernanceVotingDelay(1),
		framework.WithGovernanceVotingPeriod(3*epochSize),
		framework.WithTestBridge(),
	)
	defer cluster.Stop()

	cluster.WaitForReady(t)

	removeValidator := cluster.Servers[len(cluster.Servers)-1]
	removeValidatorKey, err := helper.GetAccountFromDir(removeValidator.DataDir())
	require.NoError(t, err)

	removeValidatorAddr := removeValidatorKey.Address()

	proposer := cluster.Servers[0]

	proposerAcc, err := helper.GetAccountFromDir(proposer.DataDir())
	require.NoError(t, err)

	relayer, err := txrelayer.NewTxRelayer(txrelayer.WithClient(proposer.JSONRPC()))
	require.NoError(t, err)

	polybftCfg, err := polybft.LoadPolyBFTConfig(path.Join(cluster.Config.TmpDir, chainConfigFileName))
	require.NoError(t, err)

	// propose and execute validator set change
	executeValidatorChangeProposal(t, relayer, proposerAcc, []*addedValidator{}, []types.Address{removeValidatorAddr}, cluster, polybftCfg)

	// Check on stake manager
	currentBlockNumber, err := relayer.Client().BlockNumber()
	require.NoError(t, err)

	require.NoError(t, cluster.WaitForBlock(currentBlockNumber+2*epochSize, 2*time.Minute))

	checkValidatorActive(t, removeValidatorAddr, relayer, false)

	require.NoError(t, proposer.Stop())

	validatorSet := getFullValidatorSet(t, proposer)
	validatorData, ok := validatorSet.Validators[removeValidatorAddr]
	require.True(t, ok)

	require.False(t, validatorData.IsActive)
}

func TestE2E_DynamicValidators_AddAndRemoveValidator(t *testing.T) {
	const (
		epochSize = uint64(10)
	)

	validatorAcc, err := crypto.GenerateECDSAKey()
	require.NoError(t, err)

	blsKey, err := bls.GenerateBlsKey()
	require.NoError(t, err)

	cluster := framework.NewTestCluster(t, 5,
		framework.WithEpochSize(10),
		framework.WithGovernanceVotingDelay(1),
		framework.WithGovernanceVotingPeriod(3*epochSize),
		framework.WithPremine(validatorAcc.Address()),
		framework.WithTestBridge(),
	)
	defer cluster.Stop()

	cluster.WaitForReady(t)

	proposer := cluster.Servers[0]

	proposerAcc, err := helper.GetAccountFromDir(proposer.DataDir())
	require.NoError(t, err)

	relayer, err := txrelayer.NewTxRelayer(txrelayer.WithClient(proposer.JSONRPC()))
	require.NoError(t, err)

	polybftCfg, err := polybft.LoadPolyBFTConfig(path.Join(cluster.Config.TmpDir, chainConfigFileName))
	require.NoError(t, err)

	// approve native token
	approve := contractsapi.ApproveNativeERC20MintableFn{
		Spender: contracts.StakeManagerContract,
		Amount:  ethgo.Ether(1),
	}

	approveInput, err := approve.EncodeAbi()
	require.NoError(t, err)

	txn := types.NewTx(types.NewLegacyTx(
		types.WithFrom(validatorAcc.Address()),
		types.WithTo(&contracts.NativeERC20TokenContract),
		types.WithInput(approveInput),
	))

	recp, err := relayer.SendTransaction(txn, validatorAcc)
	require.NoError(t, err)
	require.NotNil(t, recp)
	require.Equal(t, recp.Status, uint64(types.ReceiptSuccess))

	// propose and execute validator set change

	removeValidator := cluster.Servers[len(cluster.Servers)-1]
	removeValidatorKey, err := helper.GetAccountFromDir(removeValidator.DataDir())
	require.NoError(t, err)

	removeValidatorAddr := removeValidatorKey.Address()

	addedValidators := []*addedValidator{
		{
			Address: validatorAcc.Address(),
			Key:     blsKey.PublicKey(),
		},
	}

	executeValidatorChangeProposal(t, relayer, proposerAcc, addedValidators, []types.Address{removeValidatorAddr}, cluster, polybftCfg)

	// Check on stake manager
	currentBlockNumber, err := relayer.Client().BlockNumber()
	require.NoError(t, err)

	require.NoError(t, cluster.WaitForBlock(currentBlockNumber+epochSize, 2*time.Minute))

	checkValidatorActive(t, validatorAcc.Address(), relayer, true)
	checkValidatorActive(t, removeValidatorAddr, relayer, false)

	require.NoError(t, proposer.Stop())

	validatorSet := getFullValidatorSet(t, proposer)
	addedValidator, ok := validatorSet.Validators[validatorAcc.Address()]
	require.True(t, ok)
	require.NotNil(t, addedValidator)
	require.True(t, addedValidator.IsActive)

	removedValidator, ok := validatorSet.Validators[removeValidatorAddr]
	require.True(t, ok)
	require.NotNil(t, removedValidator)
	require.False(t, removedValidator.IsActive)
}

func TestE2E_DynamicValidators_CardanoAddAndRemoveValidator(t *testing.T) {
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
	primeConfig.PremineAmount = 500_000_000
	vectorConfig.PremineAmount = 500_000_000

	apex := cardanofw.SetupAndRunApexBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithUserCnt(userCnt),
		cardanofw.WithPrimeConfig(primeConfig),
		cardanofw.WithVectorConfig(vectorConfig),
		cardanofw.WithAPIValidatorID(-1),
		cardanofw.WithTestBridge(),
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

	primeMultisigAmount, primeFeeAmount := getMultisigAndFeeAmount(cardanofw.ChainIDPrime)
	require.Equal(t, primeMultisigAmount, primeConfig.FundAmount)
	require.Equal(t, primeFeeAmount, primeConfig.FundFeeAmount)

	t.Logf("multisig, fee = %d, %d", primeMultisigAmount, primeFeeAmount)

	datadir := fmt.Sprintf("%s%d", cluster.Config.ValidatorPrefix, len(cluster.Servers)+1)

	addresses, err := cluster.InitSecrets(datadir, 1)
	require.NoError(t, err)
	require.Len(t, addresses, 1)

	newValidatorAddr := addresses[0]
	proposer := cluster.Servers[0]

	proposerAcc, err := helper.GetAccountFromDir(proposer.DataDir())
	require.NoError(t, err)

	relayer, err := txrelayer.NewTxRelayer(txrelayer.WithClient(proposer.JSONRPC()))
	require.NoError(t, err)

	polybftCfg, err := polybft.LoadPolyBFTConfig(path.Join(cluster.Config.TmpDir, chainConfigFileName))
	require.NoError(t, err)

	newValidatorAcc, err := helper.GetAccountFromDir(path.Join(cluster.Config.TmpDir, datadir))
	require.NoError(t, err)

	// send some blade for transactions
	recp, err := relayer.SendTransaction(types.NewTx(
		types.NewLegacyTx(
			types.WithFrom(proposerAcc.Address()),
			types.WithTo(&newValidatorAddr),
			types.WithValue(ethgo.Ether(1)),
		),
	), proposerAcc.Ecdsa)
	require.NoError(t, err)
	require.NotNil(t, recp)
	require.Equal(t, recp.Status, uint64(types.ReceiptSuccess))

	// approve stake token
	approve := contractsapi.ApproveNativeERC20MintableFn{
		Spender: contracts.StakeManagerContract,
		Amount:  ethgo.Ether(1),
	}

	approveInput, err := approve.EncodeAbi()
	require.NoError(t, err)

	recp, err = relayer.SendTransaction(types.NewTx(types.NewLegacyTx(
		types.WithFrom(newValidatorAddr),
		types.WithTo(&polybftCfg.StakeTokenAddr),
		types.WithInput(approveInput),
	)), newValidatorAcc.Ecdsa)
	require.NoError(t, err)
	require.NotNil(t, recp)
	require.Equal(t, recp.Status, uint64(types.ReceiptSuccess))

	t.Log("Approved")

	removeValidator := cluster.Servers[3]
	removeValidatorKey, err := helper.GetAccountFromDir(removeValidator.DataDir())
	require.NoError(t, err)

	executeValidatorChangeProposal(t, relayer, proposerAcc, []*addedValidator{
		{
			Address: newValidatorAddr,
			Key:     newValidatorAcc.Bls.PublicKey(),
		},
	}, []types.Address{removeValidatorKey.Address()}, cluster, polybftCfg)

	t.Log("Executed validator set change")

	currentBlockNumber, err := proposer.JSONRPC().BlockNumber()
	require.NoError(t, err)

	require.NoError(t, cluster.WaitForBlock(currentBlockNumber+uint64(cluster.Config.EpochSize), 2*time.Minute))

	checkValidatorActive(t, newValidatorAddr, relayer, true)
	checkValidatorActive(t, removeValidatorKey.Address(), relayer, false)

	// wait for validator set change to finish
	apex.AddValidator(t, ctx)

	t.Logf("Added new validator")

	// wait to sync new validator
	// newValidator := cluster.Servers[len(cluster.Servers)-1]

	// require.NoError(t, cluster.WaitUntil(time.Minute*3, time.Second*2, func() bool {
	// 	proposerBlock, err := proposer.JSONRPC().BlockNumber()
	// 	if err != nil {
	// 		return false
	// 	}

	// 	newValidatorBlock, err := newValidator.JSONRPC().BlockNumber()
	// 	if err != nil {
	// 		return false
	// 	}

	// 	return proposerBlock == newValidatorBlock
	// }))

	// t.Logf("Synced new validator")

	// sender := apex.Users[0]
	// receiver := apex.Users[1]

	// sendAmountDfm := big.NewInt(500_000)

	// e2ehelper.ExecuteSingleBridging(t, ctx,
	// 	apex, sender, receiver, cardanofw.ChainIDPrime,
	// 	cardanofw.ChainIDVector, sendAmountDfm)

	// new multisig

	primeMultisigAmount, primeFeeAmount = getMultisigAndFeeAmount(cardanofw.ChainIDPrime)
	require.Equal(t, primeMultisigAmount, primeConfig.FundAmount)
	require.Equal(t, primeFeeAmount, primeConfig.FundFeeAmount)
}

func getFullValidatorSet(t *testing.T, proposer *framework.TestServer) *validatorSetState {
	t.Helper()

	db, err := bbolt.Open(filepath.Join(proposer.DataDir(), "consensus", "polybft", "consensusState.db"), 0444, nil)
	require.NoError(t, err)

	var (
		fullValidatorSet validatorSetState
		// bucket to store full validator set
		validatorSetBucket = []byte("fullValidatorSetBucket")
		// key of the full validator set in bucket
		fullValidatorSetKey = []byte("fullValidatorSet")
	)

	require.NoError(t, db.View(func(tx *bbolt.Tx) error {
		raw := tx.Bucket(validatorSetBucket).Get(fullValidatorSetKey)
		if raw == nil {
			return fmt.Errorf("no validator set")
		}

		return fullValidatorSet.Unmarshal(raw)
	}))

	return &fullValidatorSet
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

	require.Equal(t, validatorDataMap["isActive"], isAdded)
}

func executeValidatorChangeProposal(t *testing.T, relayer txrelayer.TxRelayer, proposerAcc *wallet.Account,
	addedValidators []*addedValidator, removedValidators []types.Address, cluster *framework.TestCluster, polybftCfg polybft.PolyBFTConfig) {
	t.Helper()

	description := "validatorSetChange"

	filePath := fmt.Sprintf("test_proposal_%d", time.Now().UTC().UnixMilli())

	server := cluster.Servers[0]

	for _, added := range addedValidators {
		require.NoError(t, server.AddValidatorToVSCProposal(filePath, added.Address, []string{}, hex.EncodeToHex(added.Key.Marshal())[2:], false))
	}

	for _, removed := range removedValidators {
		require.NoError(t, server.RemoveValidatorToVSCProposal(filePath, removed))
	}

	key, err := proposerAcc.Ecdsa.MarshallPrivateKey()
	require.NoError(t, err)

	hexKey := hex.EncodeToHex(key)[2:]

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

		require.NoError(t, server.VoteProposal(submitResult.ProposalID, hex.EncodeToHex(voteKey)[2:], false))
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
	Address types.Address
	Key     *bls.PublicKey
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
