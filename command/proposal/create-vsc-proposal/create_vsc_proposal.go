package createvscproposal

import (
	"github.com/0xPolygon/polygon-edge/command/proposal/common"
	addvalidator "github.com/0xPolygon/polygon-edge/command/proposal/create-vsc-proposal/add-validator"
	dropvalidator "github.com/0xPolygon/polygon-edge/command/proposal/create-vsc-proposal/drop-validator"
	removevalidator "github.com/0xPolygon/polygon-edge/command/proposal/create-vsc-proposal/remove-validator"
	showproposal "github.com/0xPolygon/polygon-edge/command/proposal/create-vsc-proposal/show-proposal"
	"github.com/spf13/cobra"
)

func GetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "create-vsc-proposal",
		Short:             "Top level command to create validator set change proposal.",
		PersistentPreRunE: common.ValidateFileFlag,
	}

	cmd.PersistentFlags().String(
		"file",
		"",
		"Path to the file representing the proposal. "+
			"File will be created if missing, otherwise updated.",
	)

	cmd.AddCommand(addvalidator.GetCommand())
	cmd.AddCommand(removevalidator.GetCommand())
	cmd.AddCommand(dropvalidator.GetCommand())
	cmd.AddCommand(showproposal.GetCommand())

	return cmd
}
