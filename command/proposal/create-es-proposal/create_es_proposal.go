package createesproposal

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
	epochSize   int64
	file        string
	submit      bool
	privateKey  string
	rpcURL      string
	description string
)

func GetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create-es-proposal",
		Short: "Create (and optionally submit) epoch size governance proposal.",
		Long:  doc,
		Run:   runCommand,
	}

	helper.RegisterJSONOutputFlag(cmd)

	cmd.Flags().Int64VarP(
		&epochSize,
		"epoch-size",
		"e",
		0,
		"New epoch size.",
	)

	_ = cmd.MarkFlagRequired("epoch-size")

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

	if epochSize <= 0 {
		outputter.SetError(errors.New("epoch size must be greater than zero"))

		return
	}

	if file != "" {
		err := common.ValidateFileFlag(cmd, nil)
		if err != nil {
			outputter.SetError(err)

			return
		}

		if err := common.StoreProposal(schema.EpochSizeProposal{Size: epochSize}, file); err != nil {
			outputter.SetError(err)

			return
		}
	}

	if submit {
		if err := common.ValidateSubmitRelatedFlags(description, rpcURL, privateKey); err != nil {
			outputter.SetError(err)

			return
		}

		proposal := contractsapi.SetNewEpochSizeNetworkParamsFn{
			NewEpochSize: big.NewInt(epochSize),
		}

		submitpkg.SubmitProposal(outputter, &proposal, description, rpcURL, privateKey)
	}
}
