package execute

import (
	"fmt"
	"math/big"

	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/spf13/cobra"

	bridgeHelper "github.com/0xPolygon/polygon-edge/command/bridge/helper"
	proposalCommon "github.com/0xPolygon/polygon-edge/command/proposal/common"
)

var (
	params executeParams
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
	cmd.Flags().StringVar(
		&params.privateKey,
		privateKeyFlag,
		"",
		"Private key",
	)

	_ = cmd.MarkFlagRequired(privateKeyFlag)

	cmd.Flags().StringVar(
		&params.jsonRPCAddress,
		jsonRPCAddressFlag,
		"",
		"JSON-RPC Address",
	)

	_ = cmd.MarkFlagRequired(jsonRPCAddressFlag)

	cmd.Flags().StringVar(
		&params.proposalID,
		proposalIDFlag,
		"",
		"Proposal ID to vote for",
	)

	_ = cmd.MarkFlagRequired(proposalIDFlag)
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

	proposer, err := bridgeHelper.DecodePrivateKey(params.privateKey)
	if err != nil {
		outputter.SetError(err)

		return
	}

	proposalID, err := common.ParseUint256orHex(&params.proposalID)
	if err != nil {
		outputter.SetError(err)

		return
	}

	proposalData, err := proposalCommon.GetProposalData(proposalID.String())
	if err != nil {
		outputter.SetError(err)

		return
	}

	executeFn := contractsapi.ExecuteChildGovernorFn{
		Targets:         []types.Address{contracts.NetworkParamsContract},
		Calldatas:       [][]byte{proposalData.Input},
		DescriptionHash: crypto.Keccak256Hash([]byte(proposalData.Description)),
		Values:          []*big.Int{big.NewInt(0)},
	}

	input, err := executeFn.EncodeAbi()
	if err != nil {
		outputter.SetError(err)

		return
	}

	txn := types.NewTx(types.NewLegacyTx(
		types.WithTo(&contracts.ChildGovernorContract),
		types.WithInput(input),
	))

	receipt, err := relayer.SendTransaction(txn, proposer)
	if err != nil {
		outputter.SetError(err)

		return
	}

	if receipt.Status != uint64(types.ReceiptSuccess) {
		outputter.SetError(fmt.Errorf("receipt status not success %+v", receipt))

		return
	}
}
