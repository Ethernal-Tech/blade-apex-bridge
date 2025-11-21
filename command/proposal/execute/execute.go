package execute

import (
	"encoding/hex"
	"fmt"
	"math/big"

	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/spf13/cobra"

	bridgeHelper "github.com/0xPolygon/polygon-edge/command/bridge/helper"
)

var (
	params executeParams
)

func GetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "execute",
		Short: "Execute for proposal",
		Run:   runCommand,
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
		&params.input,
		inputFlag,
		"",
		"Input to queue",
	)

	_ = cmd.MarkFlagRequired(inputFlag)

	cmd.Flags().StringVar(
		&params.description,
		descriptionFlag,
		"",
		"description to queue",
	)

	_ = cmd.MarkFlagRequired(descriptionFlag)
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

	proposer, err := bridgeHelper.GetPrivateKeyForCommand(params.privateKey)
	if err != nil {
		outputter.SetError(err)

		return
	}

	input, err := hex.DecodeString(params.input)
	if err != nil {
		outputter.SetError(err)

		return
	}

	executeFn := contractsapi.ExecuteChildGovernorFn{
		Targets:         []types.Address{contracts.NetworkParamsContract},
		Calldatas:       [][]byte{input},
		DescriptionHash: crypto.Keccak256Hash([]byte(params.description)),
		Values:          []*big.Int{big.NewInt(0)},
	}

	fmt.Printf("Execute: %+v", executeFn)

	execInput, err := executeFn.EncodeAbi()
	if err != nil {
		outputter.SetError(err)

		return
	}

	txn := types.NewTx(types.NewLegacyTx(
		types.WithTo(&contracts.ChildGovernorContract),
		types.WithInput(execInput),
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
