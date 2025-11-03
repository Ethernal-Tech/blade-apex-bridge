package smartcontract

import (
	"github.com/0xPolygon/polygon-edge/command/smart_contracts/deploy"
	"github.com/spf13/cobra"
)

func GetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sc",
		Short: "Top level command to work with smart contracts. Currently supported: deployment and proxy upgrade.",
	}

	cmd.AddCommand(deploy.GetCommand())

	return cmd
}
