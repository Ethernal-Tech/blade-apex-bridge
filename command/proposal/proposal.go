package proposal

import (
	createvscproposal "github.com/0xPolygon/polygon-edge/command/proposal/create-vsc-proposal"
	"github.com/0xPolygon/polygon-edge/command/proposal/get"
	"github.com/spf13/cobra"
)

func GetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "proposal",
		Short: "Top level command to work with governance proposals.",
	}

	cmd.AddCommand(createvscproposal.GetCommand())
	cmd.AddCommand(get.GetCommand())

	return cmd
}
