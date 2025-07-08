package vote

import (
	"fmt"

	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/spf13/cobra"

	bridgeHelper "github.com/0xPolygon/polygon-edge/command/bridge/helper"
)

var (
	params voteParams
)

func GetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "vote",
		Short: "Vote for proposal",
	}

	setFlags(cmd)

	return cmd
}

func setFlags(cmd *cobra.Command) {
	cmd.MarkFlagRequired(privateKeyFlag)
	cmd.Flags().StringVar(
		&params.privateKey,
		privateKeyFlag,
		"",
		"Private key",
	)

	cmd.MarkFlagRequired(jsonRPCAddressFlag)
	cmd.Flags().StringVar(
		&params.jsonRPCAddress,
		jsonRPCAddressFlag,
		"",
		"JSON-RPC Address",
	)

	cmd.MarkFlagRequired(proposalIDFlag)
	cmd.Flags().StringVar(
		&params.proposalID,
		proposalIDFlag,
		"",
		"Proposal ID to vote for",
	)

	cmd.Flags().BoolVar(
		&params.against,
		againstFlag,
		false,
		"flag for voting against",
	)
}

func runCommand(cmd *cobra.Command, _ []string) {
	outputter := command.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	relayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(
		params.jsonRPCAddress,
	))
	if err != nil {
		outputter.SetError(err)

		return
	}

	voter, err := bridgeHelper.DecodePrivateKey(params.privateKey)
	if err != nil {
		outputter.SetError(err)

		return
	}

	proposalID, err := common.ParseUint256orHex(&params.proposalID)
	if err != nil {
		outputter.SetError(err)

		return
	}

	vote := For
	if params.against {
		vote = Against
	}

	castVoteFn := &contractsapi.CastVoteChildGovernorFn{
		ProposalID: proposalID,
		Support:    uint8(vote),
	}

	input, err := castVoteFn.EncodeAbi()
	if err != nil {
		outputter.SetError(err)

		return
	}

	txn := types.NewTx(types.NewLegacyTx(
		types.WithTo(&contracts.ChildGovernorContract),
		types.WithInput(input),
	))

	receipt, err := relayer.SendTransaction(txn, voter)
	if err != nil {
		outputter.SetError(err)

		return
	}

	if receipt.Status != uint64(types.ReceiptSuccess) {
		outputter.SetError(fmt.Errorf("receipt status not success %+v", receipt))

		return
	}
}

type VoteType uint8

const (
	Against VoteType = iota
	For
	Abstain
)
