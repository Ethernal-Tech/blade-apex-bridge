package removevalidator

import (
	"fmt"

	"github.com/0xPolygon/polygon-edge/command/proposal/common"
	"github.com/0xPolygon/polygon-edge/command/proposal/schema"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/spf13/cobra"
)

var (
	addressParam string
	fileParam    string
)

func GetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove-validator",
		Short: "Adds a validator to the proposal as part of the group for removal from the validator set.",
		Long:  doc,
		PreRun: func(cmd *cobra.Command, args []string) {
			fileParam, _ = cmd.Flags().GetString("file")
		},
		RunE: runCommand,
	}

	cmd.Flags().StringVar(
		&addressParam,
		"address",
		"",
		"Address of the validator to be removed from the validator set.",
	)

	_ = cmd.MarkFlagRequired("address")

	return cmd
}

func runCommand(cmd *cobra.Command, _ []string) error {
	address, err := types.IsValidAddress(addressParam, false)
	if err != nil {
		return fmt.Errorf("not a valid address, %w", err)
	}

	proposal, err := common.LoadProposal[schema.ValidatorSetChangeProposal](fileParam)
	if err != nil {
		return err
	}

	for i, added := range proposal.Added {
		if added.Address == address.String() {
			proposal.Added = append(proposal.Added[:i], proposal.Added[i+1:]...)
		}
	}

	alreadyRemoved := false

	for _, removed := range proposal.Removed {
		if removed == address.String() {
			alreadyRemoved = true

			break
		}
	}

	if !alreadyRemoved {
		proposal.Removed = append(proposal.Removed, address.String())
	}

	return common.StoreProposal(proposal, fileParam)
}
