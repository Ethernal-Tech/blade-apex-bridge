package submit

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"

	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/command/proposal/common"
	"github.com/0xPolygon/polygon-edge/command/proposal/schema"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/spf13/cobra"

	bridgeHelper "github.com/0xPolygon/polygon-edge/command/bridge/helper"
)

var (
	params submitParams
)

func GetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "submit",
		Short: "Submit proposal",
		Run:   runCommand,
	}

	helper.RegisterJSONOutputFlag(cmd)

	setFlags(cmd)

	return cmd
}

func setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(
		&params.filePath,
		filePathFlag,
		"",
		"File path for data",
	)

	_ = cmd.MarkFlagRequired(filePathFlag)

	cmd.Flags().StringVar(
		&params.privateKey,
		privateKeyFlag,
		"",
		"Private key",
	)

	_ = cmd.MarkFlagRequired(privateKeyFlag)

	cmd.Flags().StringVar(
		&params.jsonRPCAddress,
		jsonRPCAddressFlag,
		"",
		"JSON-RPC Address",
	)

	_ = cmd.MarkFlagRequired(jsonRPCAddressFlag)

	cmd.Flags().StringVar(
		&params.description,
		descriptionFlag,
		"",
		"Proposal description",
	)

	_ = cmd.MarkFlagRequired(descriptionFlag)
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

	propType, err := common.GetProposalType(params.filePath)
	if err != nil {
		outputter.SetError(err)

		return
	}

	proposer, err := bridgeHelper.GetPrivateKeyForCommand(params.privateKey)
	if err != nil {
		outputter.SetError(err)

		return
	}

	var (
		validatorSetChange = &schema.ValidatorSetChangeProposal{}
	)

	switch propType {
	case validatorSetChange.Name():
		validatorSetChange, err = common.LoadProposal[schema.ValidatorSetChangeProposal](params.filePath)
		if err != nil {
			outputter.SetError(err)

			return
		}
	default:
		outputter.SetError(fmt.Errorf("type of data unknown"))

		return
	}

	var methodProposing = &contractsapi.NewValidatorSetNetworkParamsFn{
		ValidatorDelta: &contractsapi.ValidatorDelta{
			AddedValidators:   []*contractsapi.BridgeValidatorsData{},
			RemovedValidators: []types.Address{},
		},
	}

	// convert added validators
	for _, v := range validatorSetChange.Added {
		address := types.StringToAddress(v.Address)

		for mapKey, key := range v.Chains {
			converted, ok := common.ChainIDMap[mapKey]
			if !ok {
				outputter.SetError(fmt.Errorf("unknown chain name"))

				return
			}

			var (
				blsKey [4]*big.Int
			)

			for i := range key.Key {
				blsKey[i], ok = new(big.Int).SetString(key.Key[i], 16)
				if !ok {
					outputter.SetError(fmt.Errorf("cannot convert string to big int in public key"))

					return
				}
			}

			validator := contractsapi.BridgeValidatorsData{
				ChainID: uint8(converted),
				ValidatorData: []*contractsapi.ValidatorData{
					{
						Addr:         address,
						Key:          blsKey,
						Signature:    []byte(""),
						FeeSignature: []byte(""),
					},
				},
			}

			methodProposing.ValidatorDelta.AddedValidators = append(methodProposing.ValidatorDelta.AddedValidators, &validator)
		}
	}

	// convert removed
	for _, v := range validatorSetChange.Removed {
		address := types.StringToAddress(v)

		methodProposing.ValidatorDelta.RemovedValidators = append(methodProposing.ValidatorDelta.RemovedValidators, address)
	}

	// propose
	input, err := methodProposing.EncodeAbi()
	if err != nil {
		outputter.SetError(err)

		return
	}

	proposeFn := &contractsapi.ProposeChildGovernorFn{
		Targets:     []types.Address{contracts.NetworkParamsContract},
		Calldatas:   [][]byte{input},
		Description: params.description,
		Values:      []*big.Int{big.NewInt(0)},
	}

	proposalInput, err := proposeFn.EncodeAbi()
	if err != nil {
		outputter.SetError(err)

		return
	}

	txn := types.NewTx(types.NewLegacyTx(
		types.WithTo(&contracts.ChildGovernorContract),
		types.WithInput(proposalInput),
	))

	receipt, err := relayer.SendTransaction(txn, proposer)
	if err != nil {
		outputter.SetError(err)

		return
	}

	if receipt.Status != uint64(types.ReceiptSuccess) {
		outputter.SetError(fmt.Errorf("receipt status not success %+v", receipt))

		return
	}

	var proposalCreatedEvent contractsapi.ProposalCreatedEvent
	for _, log := range receipt.Logs {
		doesMatch, err := proposalCreatedEvent.ParseLog(log)
		if err != nil {
			outputter.SetError(err)

			return
		}

		if doesMatch {
			break
		}
	}

	result := &SubmitResult{
		ProposalID: proposalCreatedEvent.ProposalID.String(),
		Input:      hex.EncodeToString(input),
	}

	outputter.SetCommandResult(result)
}

type SubmitResult struct {
	ProposalID string `json:"proposal_id"`
	Input      string `json:"input"`
}

func (pr SubmitResult) GetOutput() string {
	enc, err := json.Marshal(&pr)
	if err != nil {
		return fmt.Sprintf("could not marshal proposal result: %v", err)
	}

	return string(enc)
}
