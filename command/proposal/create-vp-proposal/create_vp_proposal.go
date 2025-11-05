package createvpproposal

import (
	"errors"
	"math/big"

	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/command/proposal/common"
	"github.com/0xPolygon/polygon-edge/command/proposal/schema"
	submitpkg "github.com/0xPolygon/polygon-edge/command/proposal/submit"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/spf13/cobra"
)

var (
	period      int64
	file        string
	submit      bool
	privateKey  string
	rpcURL      string
	description string
)

func GetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create-vp-proposal",
		Short: "Create (and optionally submit) voting period governance proposal.",
		Long:  doc,
		Run:   runCommand,
	}

	helper.RegisterJSONOutputFlag(cmd)

	cmd.Flags().Int64VarP(
		&period,
		"voting-period",
		"p",
		0,
		"New voting period.",
	)

	_ = cmd.MarkFlagRequired("period")

	cmd.Flags().StringVarP(
		&file,
		"file",
		"f",
		"",
		"Proposal file.",
	)

	common.AddSubmitAndRelatedFlags(
		cmd,
		&submit,
		&privateKey,
		&rpcURL,
		&description)

	return cmd
}

func runCommand(cmd *cobra.Command, _ []string) {
	outputter := command.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	if period <= 0 {
		outputter.SetError(errors.New("voting period must be greater than zero"))
	}

	if file != "" {
		err := common.ValidateFileFlag(cmd, nil)
		if err != nil {
			outputter.SetError(err)
			return
		}

		common.StoreProposal(schema.VotingPeriodProposal{Period: period}, file)
	}

	if submit {
		if err := common.ValidateSubmitRelatedFlags(description, rpcURL, privateKey); err != nil {
			outputter.SetError(err)
			return
		}

		proposal := contractsapi.SetNewVotingPeriodNetworkParamsFn{
			NewVotingPeriod: big.NewInt(period),
		}

		submitpkg.SubmitProposal(outputter, &proposal, description, rpcURL, privateKey)
	}
}
