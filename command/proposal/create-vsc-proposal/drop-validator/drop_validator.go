package dropvalidator

import (
	"fmt"

	"github.com/0xPolygon/polygon-edge/command/proposal/common"
	"github.com/0xPolygon/polygon-edge/command/proposal/schema"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/spf13/cobra"
)

var (
	addressParam             string
	fileParam                string
	considerAddedListParam   bool
	considerRemovedListParam bool
)

func GetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "drop-validator",
		Short: "Drop validator from the proposal.",
		PreRun: func(cmd *cobra.Command, args []string) {
			fileParam, _ = cmd.Flags().GetString("file")
		},
		RunE: runCommand,
	}

	cmd.Flags().StringVar(
		&addressParam,
		"address",
		"",
		"Address of the validator to be dropped from the proposal.",
	)

	_ = cmd.MarkFlagRequired("address")

	cmd.Flags().BoolVar(
		&considerAddedListParam,
		"added",
		false,
		"Drop validator only if it is part of the validators to be added to the validator set.",
	)

	cmd.Flags().BoolVar(
		&considerRemovedListParam,
		"removed",
		false,
		"Drop validator only if it is part of the validators to be removed from the validator set.",
	)

	return cmd
}

func runCommand(cmd *cobra.Command, args []string) error {
	address, err := types.IsValidAddress(addressParam, false)
	if err != nil {
		return fmt.Errorf("not a valid address, %w", err)
	}

	proposal, err := common.LoadProposal[schema.ValidatorSetChangeProposal](fileParam)
	if err != nil {
		return err
	}

	dropFromAddedListFn := func() bool {
		for i, added := range proposal.Added {
			if added.Address == address.String() {
				proposal.Added = append(proposal.Added[:i], proposal.Added[i+1:]...)

				return true
			}
		}

		return false
	}

	dropFromRemovedListFn := func() bool {
		for i, removed := range proposal.Removed {
			if removed == address.String() {
				proposal.Removed = append(proposal.Removed[:i], proposal.Removed[i+1:]...)

				return true
			}
		}

		return false
	}

	dropped := false

	if considerAddedListParam {
		dropped = dropFromAddedListFn()
	}

	// A validator can't be in both lists, so skip 'removed' if already dropped from 'added'.
	if !dropped && considerRemovedListParam {
		dropFromRemovedListFn()
	}

	if !cmd.Flags().Changed("added") && !cmd.Flags().Changed("removed") && !dropFromAddedListFn() {
		dropFromRemovedListFn()
	}

	return common.StoreProposal(proposal, fileParam)
}
