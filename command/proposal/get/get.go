package get

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
	idParam         string
	rpcAddressParam string
)

func GetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Gets (and display) the proposal with the given ID.",
		Run:   runCommand,
	}

	cmd.Flags().StringVar(
		&idParam,
		"id",
		"",
		"ID of the proposal to retrieve.",
	)

	_ = cmd.MarkFlagRequired("id")

	cmd.Flags().StringVar(
		&rpcAddressParam,
		"rpc-address",
		"",
		"JSON-RPC endpoint of the node to connect to.",
	)

	_ = cmd.MarkFlagRequired("rpc-address")

	helper.RegisterJSONOutputFlag(cmd)

	return cmd
}

func runCommand(cmd *cobra.Command, _ []string) {
	outputter := command.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	// Potentially consider additional validation of flags (besides checking if they are present).

	relayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(rpcAddressParam))
	if err != nil {
		outputter.SetError(err)

		return
	}

	id, err := common.ParseUint256orHex(&idParam)
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

	switch [4]byte(first[:4]) {
	case [4]byte(newValidatorSet.Sig()):
		if err := newValidatorSet.DecodeAbi(first); err != nil {
			outputter.SetError(fmt.Errorf("could not decode validator set change proposal: %w", err))

			return
		}

		proposalResult := proposalResult{
			AddedValidators:   newValidatorSet.ValidatorDelta.AddedValidators,
			RemovedValidators: newValidatorSet.ValidatorDelta.RemovedValidators,
		}

		outputter.SetCommandResult(proposalResult)
	default:
		outputter.SetError(fmt.Errorf("unknown proposal action: %s", first[:4]))
	}
}

type proposalResult struct {
	AddedValidators   []*contractsapi.BridgeValidatorsData `json:"ValidatorsData"`
	RemovedValidators []types.Address                      `json:"RemovedValidators"`
}

func (pr proposalResult) GetOutput() string {
	enc, err := json.Marshal(&pr)
	if err != nil {
		return fmt.Sprintf("could not marshal proposal result: %v", err)
	}

	return string(enc)
}
