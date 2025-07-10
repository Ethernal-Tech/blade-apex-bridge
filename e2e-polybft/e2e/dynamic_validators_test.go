package e2e

import (
	"path"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/bls"
	"github.com/0xPolygon/polygon-edge/command/validator/helper"
	"github.com/0xPolygon/polygon-edge/consensus/polybft"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/framework"
	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/Ethernal-Tech/ethgo"
	"github.com/stretchr/testify/require"
)

func TestE2E_DynamicValidators(t *testing.T) {
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
		Amount:  ethgo.Ether(2),
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

	method := contractsapi.NewValidatorSetNetworkParamsFn{
		ValidatorDelta: &contractsapi.ValidatorDelta{
			AddedValidators: []*contractsapi.BridgeValidatorsData{
				{
					ChainID: 0xFF,
					ValidatorData: []*contractsapi.ValidatorData{
						{
							Addr:         validatorAcc.Address(),
							Key:          blsKey.PublicKey().ToBigInt(),
							FeeSignature: []byte("feeSignature"),
							Signature:    []byte("signature"),
						},
					},
				},
			},
			RemovedValidators: []types.Address{},
		},
	}

	input, err := method.EncodeAbi()
	require.NoError(t, err)

	description := "validatorSetChange"

	proposalID := sendProposalTransaction(t, relayer, proposerAcc.Ecdsa,
		contracts.ChildGovernorContract, contracts.NetworkParamsContract,
		input, description)

	require.NoError(t, cluster.WaitUntil(3*time.Minute, 2*time.Second, func() bool {
		proposalState := getProposalState(t, proposalID,
			polybftCfg.GovernanceConfig.ChildGovernorAddr, relayer)

		return proposalState == Active
	}))

	for _, s := range cluster.Servers {
		voterAcc, err := helper.GetAccountFromDir(s.DataDir())
		require.NoError(t, err)

		sendVoteTransaction(t, proposalID, For, polybftCfg.GovernanceConfig.ChildGovernorAddr,
			relayer, voterAcc.Ecdsa)
	}

	require.NoError(t, cluster.WaitUntil(3*time.Minute, 2*time.Second, func() bool {
		proposalState := getProposalState(t, proposalID,
			polybftCfg.GovernanceConfig.ChildGovernorAddr, relayer)

		return proposalState == Succeeded
	}))

	sendQueueProposalTransaction(t, relayer, proposerAcc.Ecdsa,
		polybftCfg.GovernanceConfig.ChildGovernorAddr,
		polybftCfg.GovernanceConfig.NetworkParamsAddr,
		input, description)

	require.NoError(t, cluster.WaitUntil(3*time.Minute, 2*time.Second, func() bool {
		proposalState := getProposalState(t, proposalID,
			polybftCfg.GovernanceConfig.ChildGovernorAddr, relayer)

		return proposalState == Queued
	}))

	currentBlockNumber, err := relayer.Client().BlockNumber()
	require.NoError(t, err)

	require.NoError(t, cluster.WaitForBlock(currentBlockNumber+2, 10*time.Second))

	sendExecuteProposalTransaction(t, relayer, proposerAcc.Ecdsa,
		polybftCfg.GovernanceConfig.ChildGovernorAddr,
		polybftCfg.GovernanceConfig.NetworkParamsAddr,
		input, description)

	// Check on stake manager

	currentBlockNumber, err = relayer.Client().BlockNumber()
	require.NoError(t, err)

	require.NoError(t, cluster.WaitForBlock(currentBlockNumber+epochSize, 2*time.Minute))

	getValidatorFn := contractsapi.GetValidatorStakeManagerFn{
		Validator_: validatorAcc.Address(),
	}

	input, err = getValidatorFn.EncodeAbi()
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

	require.Equal(t, validatorDataMap["isActive"], true)
}
