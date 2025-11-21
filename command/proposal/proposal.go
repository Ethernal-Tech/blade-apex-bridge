package proposal

import (
	createesproposal "github.com/0xPolygon/polygon-edge/command/proposal/create-es-proposal"
	createvpproposal "github.com/0xPolygon/polygon-edge/command/proposal/create-vp-proposal"
	createvscproposal "github.com/0xPolygon/polygon-edge/command/proposal/create-vsc-proposal"
	"github.com/0xPolygon/polygon-edge/command/proposal/execute"
	"github.com/0xPolygon/polygon-edge/command/proposal/get"
	"github.com/0xPolygon/polygon-edge/command/proposal/queue"
	"github.com/0xPolygon/polygon-edge/command/proposal/submit"
	"github.com/0xPolygon/polygon-edge/command/proposal/vote"
	"github.com/spf13/cobra"
)

func GetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "proposal",
		Short: "Top level command to work with governance proposals.",
	}

	cmd.AddCommand(createesproposal.GetCommand())
	cmd.AddCommand(createvpproposal.GetCommand())
	cmd.AddCommand(createvscproposal.GetCommand())
	cmd.AddCommand(get.GetCommand())
	cmd.AddCommand(submit.GetCommand())
	cmd.AddCommand(vote.GetCommand())
	cmd.AddCommand(queue.GetCommand())
	cmd.AddCommand(execute.GetCommand())

	return cmd
}
