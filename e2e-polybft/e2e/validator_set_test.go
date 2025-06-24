package e2e

import (
	"context"
	"math/big"
	"path"
	"sync"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/command/validator/helper"
	"github.com/0xPolygon/polygon-edge/consensus/polybft"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/signer"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/framework"
	"github.com/0xPolygon/polygon-edge/jsonrpc"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/Ethernal-Tech/ethgo"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
)

func TestE2E_ValidatorSetChange(t *testing.T) {
	const (
		epochSize    = 10
		votingPeriod = 3 * epochSize
		newAccsCount = 2
	)

	var err error

	newAccs := make([]*wallet.Account, newAccsCount)
	newAddrs := make([]types.Address, newAccsCount)

	for i := range newAccsCount {
		newAccs[i], err = wallet.GenerateAccount()
		require.NoError(t, err)

		newAddrs[i] = newAccs[i].Address()
	}

	cluster := framework.NewTestCluster(t, 4,
		framework.WithEpochSize(epochSize),
		framework.WithGovernanceVotingPeriod(votingPeriod),
		framework.WithGovernanceVotingDelay(1),
		framework.WithPremine(newAddrs...))

	defer cluster.Stop()

	cluster.WaitForReady(t)

	validatorSrv := cluster.Servers[0]

	validatorEndpoint := validatorSrv.JSONRPC()

	polybftCfg, err := polybft.LoadPolyBFTConfig(path.Join(cluster.Config.TmpDir, chainConfigFileName))
	require.NoError(t, err)

	txRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithClient(validatorEndpoint))
	require.NoError(t, err)

	proposer := cluster.Servers[0]

	proposerAcc, err := helper.GetAccountFromDir(proposer.DataDir())
	require.NoError(t, err)

	wg := sync.WaitGroup{}
	wg.Add(1)

	t.Run("Whitelist, register & unstake one", func(t *testing.T) {
		t.Logf("Whitelist, register & unstake one started")

		defer wg.Done()

		whitelist(t, []*wallet.Account{newAccs[0]}, txRelayer, proposerAcc, polybftCfg, cluster,
			validatorEndpoint, epochSize, []string{"WHITELIST 1"})

		approveAndRegister(t, txRelayer, newAccs[0], validatorEndpoint, epochSize)
		extra := commit(t, []*wallet.Account{newAccs[0]}, txRelayer, proposerAcc, polybftCfg, cluster,
			validatorEndpoint, epochSize, []bool{true}, []string{"COMMIT 1"})
		require.True(t, extra.Validators.Added.ContainsAddress(newAddrs[0]))

		unstakeValidator(t, newAccs[0], txRelayer, validatorEndpoint, epochSize)
		extra = commit(t, []*wallet.Account{newAccs[0]}, txRelayer, proposerAcc, polybftCfg, cluster,
			validatorEndpoint, epochSize, []bool{false}, []string{"COMMIT 2"})
		require.NotEqual(t, extra.Validators.Removed.Len(), 0)

		t.Logf("Whitelist, register & unstake one done")
	})

	wg.Wait()

	t.Run("Whitelist, register & unstake two", func(t *testing.T) {
		t.Skip()
		t.Logf("Whitelist, register & unstake two started")

		wg.Add(1)
		defer wg.Done()

		whitelist(t, newAccs, txRelayer, proposerAcc, polybftCfg, cluster,
			validatorEndpoint, epochSize, []string{"WHITELIST 2", "WHITELIST 3"})

		for _, a := range newAccs {
			approveAndRegister(t, txRelayer, a, validatorEndpoint, epochSize)
		}

		extra := commit(t, newAccs, txRelayer, proposerAcc, polybftCfg, cluster,
			validatorEndpoint, epochSize, []bool{true, true}, []string{"COMMIT 3", "COMMIT 4"})
		require.True(t, extra.Validators.Added.ContainsAddress(newAddrs[0]))
		require.True(t, extra.Validators.Added.ContainsAddress(newAddrs[1]))

		for _, a := range newAccs {
			unstakeValidator(t, a, txRelayer, validatorEndpoint, epochSize)
		}

		extra = commit(t, newAccs, txRelayer, proposerAcc, polybftCfg, cluster,
			validatorEndpoint, epochSize, []bool{false, false}, []string{"COMMIT 5", "COMMIT 6"})
		require.Equal(t, extra.Validators.Removed.Len(), 2)

		t.Logf("Whitelist, register & unstake two done")
	})

	wg.Wait()

	t.Run("Register & unstake together", func(t *testing.T) {
		t.Skip()
		t.Logf("Register & unstake together started")

		whitelist(t, newAccs, txRelayer, proposerAcc, polybftCfg, cluster,
			validatorEndpoint, epochSize, []string{"WHITELIST 4", "WHITELIST 5"})

		approveAndRegister(t, txRelayer, newAccs[0], validatorEndpoint, epochSize)
		extra := commit(t, []*wallet.Account{newAccs[0]}, txRelayer, proposerAcc, polybftCfg, cluster,
			validatorEndpoint, epochSize, []bool{true}, []string{"COMMIT 7"})
		require.True(t, extra.Validators.Added.ContainsAddress(newAddrs[0]))

		g, _ := errgroup.WithContext(context.Background())
		g.Go(func() error {
			approveAndRegister(t, txRelayer, newAccs[1], validatorEndpoint, epochSize)

			return nil
		})

		g.Go(func() error {
			unstakeValidator(t, newAccs[0], txRelayer, validatorEndpoint, epochSize)

			return nil
		})

		require.NoError(t, g.Wait())

		extra = commit(t, newAccs, txRelayer, proposerAcc, polybftCfg, cluster,
			validatorEndpoint, epochSize, []bool{false, true}, []string{"COMMIT 8", "COMMIT 9"})
		require.True(t, extra.Validators.Added.ContainsAddress(newAddrs[1]))
		require.Equal(t, extra.Validators.Removed.Len(), 1)

		t.Logf("Register & unstake together done")
	})
}

func unstakeValidator(t *testing.T, acc *wallet.Account, txRelayer txrelayer.TxRelayer, validatorEndpoint *jsonrpc.EthClient, epochSize uint64) {
	t.Helper()

	unstake := contractsapi.UnstakeStakeManagerFn{
		Amount: ethgo.Ether(1),
	}

	enc, err := unstake.EncodeAbi()
	require.NoError(t, err)

	txn := types.NewTx(types.NewLegacyTx(
		types.WithFrom(acc.Address()),
		types.WithTo(&contracts.StakeManagerContract),
		types.WithInput(enc),
	))

	rec, err := txRelayer.SendTransaction(txn, acc.Ecdsa)
	require.NoError(t, err)
	require.Equal(t, rec.Status, uint64(types.ReceiptSuccess))
}

func approveAndRegister(t *testing.T, txRelayer txrelayer.TxRelayer, validatorAcc *wallet.Account, validatorEndpoint *jsonrpc.EthClient, epochSize uint64) {
	t.Helper()

	approve := contractsapi.ApproveNativeERC20MintableFn{
		Spender: contracts.StakeManagerContract,
		Amount:  ethgo.Ether(1),
	}

	enc, err := approve.EncodeAbi()
	require.NoError(t, err)

	tx := types.NewTx(types.NewDynamicFeeTx(
		types.WithFrom(types.ZeroAddress),
		types.WithTo(&contracts.NativeERC20TokenContract),
		types.WithInput(enc)))

	rec, err := txRelayer.SendTransaction(tx, validatorAcc.Ecdsa)
	require.NoError(t, err)
	require.Equal(t, rec.Status, uint64(types.ReceiptSuccess))

	// register validator
	chainID, err := validatorEndpoint.ChainID()
	require.NoError(t, err)

	koskSignature, err := signer.MakeKOSKSignature(
		validatorAcc.Bls, validatorAcc.Address(),
		chainID.Int64(), signer.DomainValidatorSet, contracts.StakeManagerContract)
	require.NoError(t, err)

	sigMarshal, err := koskSignature.ToBigInt()
	require.NoError(t, err)

	registerData := contractsapi.RegisterStakeManagerFn{
		Signature: sigMarshal,
		Pubkey:    validatorAcc.Bls.PublicKey().ToBigInt(),
	}

	enc, err = registerData.EncodeAbi()
	require.NoError(t, err)

	txn := types.NewTx(
		types.NewLegacyTx(
			types.WithFrom(validatorAcc.Address()),
			types.WithTo(&contracts.StakeManagerContract),
			types.WithInput(enc),
		),
	)

	rec, err = txRelayer.SendTransaction(txn, validatorAcc.Ecdsa)
	require.NoError(t, err)
	require.Equal(t, rec.Status, uint64(types.ReceiptSuccess))
}

func whitelist(t *testing.T,
	newAccs []*wallet.Account,
	txRelayer txrelayer.TxRelayer,
	proposerAcc *wallet.Account,
	polybftCfg polybft.PolyBFTConfig,
	cluster *framework.TestCluster,
	validatorEndpoint *jsonrpc.EthClient,
	epochSize uint64,
	proposalDescs []string) {
	t.Helper()

	require.Equal(t, len(newAccs), len(proposalDescs))

	proposalIDs := make([]*big.Int, len(newAccs))
	proposalInputs := make([][]byte, len(newAccs))

	for i, a := range newAccs {
		proposal := contractsapi.WhitelistNewValidatorNetworkParamsFn{
			Validator: a.Address(),
		}

		proposalInput, err := proposal.EncodeAbi()
		require.NoError(t, err)

		proposalID := sendProposalTransaction(t, txRelayer, proposerAcc.Ecdsa,
			polybftCfg.GovernanceConfig.ChildGovernorAddr,
			polybftCfg.GovernanceConfig.NetworkParamsAddr,
			proposalInput, proposalDescs[i])

		// check that proposal delay finishes, and porposal becomes active (ready to for voting)
		require.NoError(t, cluster.WaitUntil(3*time.Minute, 2*time.Second, func() bool {
			proposalState := getProposalState(t, proposalID,
				polybftCfg.GovernanceConfig.ChildGovernorAddr, txRelayer)

			return proposalState == Active
		}))

		proposalIDs[i] = proposalID
		proposalInputs[i] = proposalInput
	}
	// vote for the proposal
	for _, s := range cluster.Servers {
		voterAcc, err := helper.GetAccountFromDir(s.DataDir())
		require.NoError(t, err)

		for _, proposalID := range proposalIDs {
			sendVoteTransaction(t, proposalID, For, polybftCfg.GovernanceConfig.ChildGovernorAddr,
				txRelayer, voterAcc.Ecdsa)
		}
	}

	// check if proposal has quorum (if it was accepted)
	require.NoError(t, cluster.WaitUntil(3*time.Minute, 2*time.Second, func() bool {
		for _, proposalID := range proposalIDs {
			proposalState := getProposalState(t, proposalID,
				polybftCfg.GovernanceConfig.ChildGovernorAddr, txRelayer)

			if proposalState != Succeeded {
				return false
			}
		}

		return true
	}))

	for i := range proposalInputs {
		// queue proposal for execution
		sendQueueProposalTransaction(t, txRelayer, proposerAcc.Ecdsa,
			polybftCfg.GovernanceConfig.ChildGovernorAddr,
			polybftCfg.GovernanceConfig.NetworkParamsAddr,
			proposalInputs[i], proposalDescs[i])
	}

	// check if proposal has quorum (if it was accepted)
	require.NoError(t, cluster.WaitUntil(3*time.Minute, 2*time.Second, func() bool {
		for _, proposalID := range proposalIDs {
			proposalState := getProposalState(t, proposalID,
				polybftCfg.GovernanceConfig.ChildGovernorAddr, txRelayer)

			if proposalState != Queued {
				return false
			}
		}

		return true
	}))

	currentBlock, err := validatorEndpoint.BlockNumber()
	require.NoError(t, err)

	// wait for couple of more blocks because of execution delay
	require.NoError(t, cluster.WaitForBlock(currentBlock+2, 10*time.Second))

	for i := range proposalInputs {
		sendExecuteProposalTransaction(t, txRelayer, proposerAcc.Ecdsa,
			polybftCfg.GovernanceConfig.ChildGovernorAddr,
			polybftCfg.GovernanceConfig.NetworkParamsAddr,
			proposalInputs[i], proposalDescs[i])
	}

	currentBlock, err = validatorEndpoint.BlockNumber()
	require.NoError(t, err)

	require.NoError(t, cluster.WaitForBlock(currentBlock+epochSize, 3*time.Minute))
}

func commit(t *testing.T,
	newAccs []*wallet.Account,
	txRelayer txrelayer.TxRelayer,
	proposerAcc *wallet.Account,
	polybftCfg polybft.PolyBFTConfig,
	cluster *framework.TestCluster,
	validatorEndpoint *jsonrpc.EthClient,
	epochSize uint64,
	registers []bool,
	proposalDescs []string) *polybft.Extra {
	t.Helper()

	require.Equal(t, len(newAccs), len(proposalDescs))
	require.Equal(t, len(newAccs), len(registers))

	proposalIDs := make([]*big.Int, len(newAccs))
	proposalInputs := make([][]byte, len(newAccs))

	for i, a := range newAccs {
		proposal := contractsapi.NewValidatorSetCommitNetworkParamsFn{
			Validator:  a.Address(),
			IsRegister: registers[i],
		}

		proposalInput, err := proposal.EncodeAbi()
		require.NoError(t, err)

		proposalID := sendProposalTransaction(t, txRelayer, proposerAcc.Ecdsa,
			polybftCfg.GovernanceConfig.ChildGovernorAddr,
			polybftCfg.GovernanceConfig.NetworkParamsAddr,
			proposalInput, proposalDescs[i])

		// check that proposal delay finishes, and porposal becomes active (ready to for voting)
		require.NoError(t, cluster.WaitUntil(3*time.Minute, 2*time.Second, func() bool {
			proposalState := getProposalState(t, proposalID,
				polybftCfg.GovernanceConfig.ChildGovernorAddr, txRelayer)

			return proposalState == Active
		}))

		proposalIDs[i] = proposalID
		proposalInputs[i] = proposalInput
	}
	// vote for the proposal
	for _, s := range cluster.Servers {
		voterAcc, err := helper.GetAccountFromDir(s.DataDir())
		require.NoError(t, err)

		for _, proposalID := range proposalIDs {
			sendVoteTransaction(t, proposalID, For, polybftCfg.GovernanceConfig.ChildGovernorAddr,
				txRelayer, voterAcc.Ecdsa)
		}
	}

	// check if proposal has quorum (if it was accepted)
	require.NoError(t, cluster.WaitUntil(3*time.Minute, 2*time.Second, func() bool {
		for _, proposalID := range proposalIDs {
			proposalState := getProposalState(t, proposalID,
				polybftCfg.GovernanceConfig.ChildGovernorAddr, txRelayer)

			if proposalState != Succeeded {
				return false
			}
		}

		return true
	}))

	for i := range proposalInputs {
		// queue proposal for execution
		sendQueueProposalTransaction(t, txRelayer, proposerAcc.Ecdsa,
			polybftCfg.GovernanceConfig.ChildGovernorAddr,
			polybftCfg.GovernanceConfig.NetworkParamsAddr,
			proposalInputs[i], proposalDescs[i])
	}

	// check if proposal has quorum (if it was accepted)
	require.NoError(t, cluster.WaitUntil(3*time.Minute, 2*time.Second, func() bool {
		for _, proposalID := range proposalIDs {
			proposalState := getProposalState(t, proposalID,
				polybftCfg.GovernanceConfig.ChildGovernorAddr, txRelayer)

			if proposalState != Queued {
				return false
			}
		}

		return true
	}))

	currentBlock, err := validatorEndpoint.BlockNumber()
	require.NoError(t, err)

	// wait for couple of more blocks because of execution delay
	require.NoError(t, cluster.WaitForBlock(currentBlock+2, 10*time.Second))

	for i := range proposalInputs {
		sendExecuteProposalTransaction(t, txRelayer, proposerAcc.Ecdsa,
			polybftCfg.GovernanceConfig.ChildGovernorAddr,
			polybftCfg.GovernanceConfig.NetworkParamsAddr,
			proposalInputs[i], proposalDescs[i])
	}

	currentBlock, err = validatorEndpoint.BlockNumber()
	require.NoError(t, err)

	header, err := waitForEpochEnding(t, validatorEndpoint, &currentBlock, epochSize)
	require.NoError(t, err)

	extra, err := polybft.GetIbftExtra(header.ExtraData)
	require.NoError(t, err)

	require.NotNil(t, extra.Validators)
	require.False(t, extra.Validators.IsEmpty())

	return extra
}
