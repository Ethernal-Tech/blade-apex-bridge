package e2e

// import (
// 	"path"
// 	"testing"
// 	"time"

// 	"github.com/0xPolygon/polygon-edge/command/validator/helper"
// 	"github.com/0xPolygon/polygon-edge/consensus/polybft"
// 	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
// 	"github.com/0xPolygon/polygon-edge/consensus/polybft/signer"
// 	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
// 	"github.com/0xPolygon/polygon-edge/contracts"
// 	"github.com/0xPolygon/polygon-edge/e2e-polybft/framework"
// 	"github.com/0xPolygon/polygon-edge/txrelayer"
// 	"github.com/0xPolygon/polygon-edge/types"
// 	"github.com/Ethernal-Tech/ethgo"
// 	"github.com/stretchr/testify/require"
// )

// func TestE2E_ValidatorSetChange(t *testing.T) {
// 	const (
// 		epochSize       = 10
// 		sprintSize      = uint64(5)
// 		numberOfBridges = 1
// 	)

// 	validatorAcc, err := wallet.GenerateAccount()
// 	require.NoError(t, err)

// 	cluster := framework.NewTestCluster(t, 4,
// 		framework.WithEpochSize(epochSize),
// 		framework.WithGovernanceVotingPeriod(3*epochSize),
// 		framework.WithGovernanceVotingDelay(1),
// 		framework.WithSecretsCallback(func(addrs []types.Address, tcc *framework.TestClusterConfig) {
// 			for i := 0; i < len(addrs); i++ {
// 				// premine receivers, so that they are able to do withdrawals
// 				tcc.StakeAmounts = append(tcc.StakeAmounts, ethgo.Ether(10))
// 			}

// 			tcc.StakeAmounts = append(tcc.StakeAmounts, ethgo.Ether(10))
// 			tcc.Premine = append(tcc.Premine, validatorAcc.Address().String())
// 		}))

// 	defer cluster.Stop()

// 	cluster.WaitForReady(t)

// 	validatorSrv := cluster.Servers[0]

// 	validatorEndpoint := validatorSrv.JSONRPC()

// 	polybftCfg, err := polybft.LoadPolyBFTConfig(path.Join(cluster.Config.TmpDir, chainConfigFileName))
// 	require.NoError(t, err)

// 	txRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithClient(validatorEndpoint))
// 	require.NoError(t, err)

// 	proposer := cluster.Servers[0]

// 	proposerAcc, err := helper.GetAccountFromDir(proposer.DataDir())
// 	require.NoError(t, err)

// 	// whitelist validator
// 	proposal := contractsapi.WhiteListNewValidatorNetworkParamsFn{
// 		Validator: validatorAcc.Address(),
// 	}

// 	proposalInput, err := proposal.EncodeAbi()
// 	require.NoError(t, err)

// 	proposalID := sendProposalTransaction(t, txRelayer, proposerAcc.Ecdsa,
// 		polybftCfg.GovernanceConfig.ChildGovernorAddr,
// 		polybftCfg.GovernanceConfig.NetworkParamsAddr,
// 		proposalInput, "whitelist")

// 	// check that proposal delay finishes, and porposal becomes active (ready to for voting)
// 	require.NoError(t, cluster.WaitUntil(3*time.Minute, 2*time.Second, func() bool {
// 		proposalState := getProposalState(t, proposalID,
// 			polybftCfg.GovernanceConfig.ChildGovernorAddr, txRelayer)

// 		return proposalState == Active
// 	}))

// 	// vote for the proposal
// 	for _, s := range cluster.Servers {
// 		voterAcc, err := helper.GetAccountFromDir(s.DataDir())
// 		require.NoError(t, err)

// 		sendVoteTransaction(t, proposalID, For, polybftCfg.GovernanceConfig.ChildGovernorAddr,
// 			txRelayer, voterAcc.Ecdsa)
// 	}

// 	// check if proposal has quorum (if it was accepted)
// 	require.NoError(t, cluster.WaitUntil(3*time.Minute, 2*time.Second, func() bool {
// 		proposalState := getProposalState(t, proposalID,
// 			polybftCfg.GovernanceConfig.ChildGovernorAddr, txRelayer)

// 		return proposalState == Succeeded
// 	}))

// 	// queue proposal for execution
// 	sendQueueProposalTransaction(t, txRelayer, proposerAcc.Ecdsa,
// 		polybftCfg.GovernanceConfig.ChildGovernorAddr,
// 		polybftCfg.GovernanceConfig.NetworkParamsAddr,
// 		proposalInput, "whitelist")

// 	// check if proposal has quorum (if it was accepted)
// 	require.NoError(t, cluster.WaitUntil(3*time.Minute, 2*time.Second, func() bool {
// 		proposalState := getProposalState(t, proposalID,
// 			polybftCfg.GovernanceConfig.ChildGovernorAddr, txRelayer)

// 		return proposalState == Queued
// 	}))

// 	// register validator
// 	chainID, err := validatorEndpoint.ChainID()
// 	require.NoError(t, err)

// 	koskSignature, err := signer.MakeKOSKSignature(
// 		validatorAcc.Bls, validatorAcc.Address(),
// 		chainID.Int64(), signer.DomainValidatorSet, contracts.StakeManagerContract)
// 	require.NoError(t, err)

// 	sigMarshal, err := koskSignature.ToBigInt()
// 	require.NoError(t, err)

// 	registerData := contractsapi.RegisterStakeManagerFn{
// 		Signature: sigMarshal,
// 		Pubkey:    validatorAcc.Bls.PublicKey().ToBigInt(),
// 	}

// 	enc, err := registerData.EncodeAbi()
// 	require.NoError(t, err)

// 	txn := types.NewTx(
// 		types.NewLegacyTx(
// 			types.WithFrom(validatorAcc.Address()),
// 			types.WithTo(&contracts.StakeManagerContract),
// 			types.WithInput(enc),
// 		),
// 	)

// 	rec, err := txRelayer.SendTransaction(txn, validatorAcc.Ecdsa)
// 	require.NoError(t, err)
// 	require.Equal(t, rec.Status, uint64(types.ReceiptSuccess))

// 	epochEndingBlock, err := waitForEpochEnding(t, validatorEndpoint, &rec.BlockNumber, epochSize)
// 	require.NoError(t, err)

// 	extra, err := polybft.GetIbftExtra(epochEndingBlock.ExtraData)
// 	require.NoError(t, err)

// 	require.NotNil(t, extra.Validators)
// 	require.False(t, extra.Validators.IsEmpty())
// 	require.True(t, extra.Validators.Added.ContainsAddress(validatorAcc.Address()))
// }
