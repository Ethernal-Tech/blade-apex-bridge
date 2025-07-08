package showproposal

import (
	"fmt"
	"strconv"

	"github.com/0xPolygon/polygon-edge/command/proposal/common"
	"github.com/0xPolygon/polygon-edge/command/proposal/schema"
	"github.com/spf13/cobra"
)

var fileParam string

func GetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Shows the current state of the validator set change proposal being created.",
		PreRun: func(cmd *cobra.Command, args []string) {
			fileParam, _ = cmd.Flags().GetString("file")
		},
		RunE: runCommand,
	}

	return cmd
}

func runCommand(cmd *cobra.Command, _ []string) error {
	proposal, err := common.LoadProposal[schema.ValidatorSetChangeProposal](fileParam)
	if err != nil {
		return err
	}

	output := "=========================="
	output += " VALIDATOR SET CHANGE "
	output += "==========================\n\nAdded validators:\n"

	if len(proposal.Added) == 0 {
		output += "No validators added.\n"
	} else {
		for i, v := range proposal.Added {
			output += "\r" + strconv.Itoa(i+1) + ". " + v.Address + "\n"
			output += "Keys per chain:\n\t"
			i := 0

			for chain, v := range v.Chains {
				output += strconv.Itoa(i+1) + ". " + chain + "\n\t"
				i++

				for i := range 4 {
					output += "- " + v.Key[i] + "\n\t"
				}
			}
		}
	}

	output += "\nRemoved validators:\n"
	if len(proposal.Removed) == 0 {
		output += "No validators removed."
	} else {
		for i, v := range proposal.Removed {
			output += strconv.Itoa(i+1) + ". " + v
		}
	}

	fmt.Println(output)

	return nil
}
