package proposal

import (
	"encoding/json"
	"fmt"

	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/spf13/cobra"
)

var (
	params getProposalParams
)

func GetCommand() *cobra.Command {
	proposalCmd := &cobra.Command{
		Use:   "proposal",
		Short: "Commands for getting proposals",
		Run:   runCommand,
	}

	helper.RegisterJSONOutputFlag(proposalCmd)

	setFlags(proposalCmd)

	return proposalCmd
}

func setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(
		&params.proposalID,
		proposalIDFlag,
		"",
		"the ID of the proposal to get",
	)

	cmd.Flags().StringVar(
		&params.jsonRPCAddress,
		jsonRPCAddressFlag,
		"",
		"the JSON-RPC address of the node to connect to",
	)
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

	id, err := common.ParseUint256orHex(&params.proposalID)
	if err != nil {
		outputter.SetError(fmt.Errorf("could not parse proposal ID: %w", err))

		return
	}

	proposal := contractsapi.ChildGovernor.Abi.GetMethod("getActions")

	encoded, err := proposal.Encode([]interface{}{id})
	if err != nil {
		outputter.SetError(fmt.Errorf("could not encode proposal ID: %w", err))

		return
	}

	response, err := relayer.Call(types.ZeroAddress, contracts.ChildGovernorContract, encoded)
	if err != nil {
		outputter.SetError(fmt.Errorf("could not get proposal from contract: %w", err))

		return
	}

	byteResponse, err := hex.DecodeHex(response)
	if err != nil {
		outputter.SetError(fmt.Errorf("could not decode response: %w", err))

		return
	}

	decoded, err := proposal.Outputs.Decode(byteResponse)
	if err != nil {
		outputter.SetError(fmt.Errorf("could not decode proposal outputs: %w", err))

		return
	}

	decodedOutputsMap, ok := decoded.(map[string]interface{})
	if !ok {
		outputter.SetError(fmt.Errorf("could not convert decoded outputs to map"))

		return
	}

	calldatas, ok := decodedOutputsMap["calldatas"].([][]byte)
	if !ok {
		outputter.SetError(fmt.Errorf("could not get calldatas"))

		return
	}

	first := calldatas[0]

	var (
		newValidatorSet contractsapi.NewValidatorSetNetworkParamsFn
	)

	switch string(first[:4]) {
	case string(newValidatorSet.Sig()):
		if err := newValidatorSet.DecodeAbi(first); err != nil {
			outputter.SetError(fmt.Errorf("could not decode new validator set proposal: %w", err))

			return
		}

		proposalResult := proposalResult{
			ValidatorSet:      newValidatorSet.ValidatorSet,
			RemovedValidators: newValidatorSet.RemovedValidators,
		}

		outputter.SetCommandResult(proposalResult)
	default:
		outputter.SetError(fmt.Errorf("unknown proposal action: %s", first[:4]))

		return
	}
}

type proposalResult struct {
	ValidatorSet      []*contractsapi.ValidatorSetApex `json:"ValidatorSet"`
	RemovedValidators []types.Address                  `json:"RemovedValidators"`
}

func (pr proposalResult) GetOutput() string {
	enc, err := json.Marshal(&pr)
	if err != nil {
		return fmt.Sprintf("could not marshal proposal result: %v", err)
	}

	return string(enc)
}
