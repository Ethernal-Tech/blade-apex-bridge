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
	"github.com/0xPolygon/polygon-edge/e2e-polybft/e2ehelper"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/framework"
	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/Ethernal-Tech/ethgo"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"

	secretsCardano "github.com/Ethernal-Tech/cardano-infrastructure/secrets"
	secretsHelper "github.com/Ethernal-Tech/cardano-infrastructure/secrets/helper"
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
		framework.WithTestBridge(true),
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
		framework.WithTestBridge(true),
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
		framework.WithTestBridge(true),
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
		// cardanofw.WithTestBridge(),
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

	t.Logf("multisig, fee prime = %d, %d", primeMultisigAmount, primeFeeAmount)

	vectorMultisigAmount, vectorFeeAmount := getMultisigAndFeeAmount(cardanofw.ChainIDVector)
	require.Equal(t, vectorMultisigAmount, vectorConfig.FundAmount)
	require.Equal(t, vectorFeeAmount, vectorConfig.FundFeeAmount)

	t.Logf("multisig, fee vector = %d, %d", vectorMultisigAmount, vectorFeeAmount)

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

	// send some blade for transactions
	recp, err := relayer.SendTransaction(types.NewTx(
		types.NewLegacyTx(
			types.WithFrom(proposerAcc.Address()),
			types.WithTo(newValidatorAcc.Address().Ptr()),
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
		types.WithFrom(newValidatorAcc.Address()),
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
	}, []types.Address{removeValidatorKey.Address()}, cluster, polybftCfg)

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

		t.Log(ret)

		return num == 0
	}))

	t.Log("Finished VSC")

	checkValidatorActive(t, newValidatorAcc.Address(), relayer, true)
	checkValidatorActive(t, removeValidatorKey.Address(), relayer, false)

	t.Logf("Added new validator")

	// create new multisig and fee addresses
	require.NoError(t, apex.UpdateConfigs(ctx))

	// check on new multisig
	primeMultisigAmount, primeFeeAmount = getMultisigAndFeeAmount(cardanofw.ChainIDPrime)
	require.Equal(t, primeMultisigAmount, primeConfig.FundAmount)
	require.True(t, primeFeeAmount > 0 && primeFeeAmount < primeConfig.FundFeeAmount)

	vectorMultisigAmount, vectorFeeAmount = getMultisigAndFeeAmount(cardanofw.ChainIDVector)
	require.Equal(t, vectorMultisigAmount, vectorConfig.FundAmount)
	require.True(t, vectorFeeAmount > 0 && vectorFeeAmount < vectorConfig.FundFeeAmount)

	// stop removed validator
	require.NoError(t, removeValidator.Stop())

	// stop one of validators to check if new validator participates in voting
	require.NoError(t, cluster.Servers[1].Stop())

	// send transaction to check
	sender := apex.Users[0]
	receiver := apex.Users[1]

	sendAmountDfm := big.NewInt(500_000)

	e2ehelper.ExecuteSingleBridging(t, ctx,
		apex, sender, receiver, cardanofw.ChainIDPrime,
		cardanofw.ChainIDVector, sendAmountDfm)
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
