package addvalidator

import (
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/0xPolygon/polygon-edge/bls"
	"github.com/0xPolygon/polygon-edge/command/proposal/common"
	"github.com/0xPolygon/polygon-edge/command/proposal/schema"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/spf13/cobra"
)

var (
	fileParam              string
	addressParam           string
	cardanoLikeChainsParam []string
	bladeParam             string
	nexusParam             bool
	//evmLikeChainsParam     []string
)

func GetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add-validator",
		Short: "Adds a validator to the proposal as part of the group for inclusion in the validator set.",
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
		"Address of the validator to be added to the validator set.",
	)

	_ = cmd.MarkFlagRequired("address")

	cmd.Flags().StringSliceVar(
		&cardanoLikeChainsParam,
		"cardano-like-chain",
		nil,
		"chain_name:multisig_verification:fee_verification:multisig_stake_verification:fee_stake_verification",
	)

	_ = cmd.MarkFlagRequired("cardano-like-chain")

	// cmd.Flags().StringSliceVar(
	// 	&evmLikeChainsParam,
	// 	"evm-like-chain",
	// 	nil,
	// 	"chain_name:bls_key",
	// )

	cmd.Flags().StringVar(
		&bladeParam,
		"blade",
		"",
		"bls_key",
	)

	_ = cmd.MarkFlagRequired("blade")

	cmd.Flags().BoolVar(
		&nexusParam,
		"nexus",
		false,
		"If set, the validator will also be added to the Nexus network.",
	)

	return cmd
}

func parseCardanoLikeChainEntry(entry string) (string, [4]string, error) {
	errReturnFn := func(err error) (string, [4]string, error) {
		return "", [4]string{}, err
	}

	parts := strings.Split(entry, ":")
	if len(parts) != 5 {
		return errReturnFn(fmt.Errorf("invalid entry format"))
	}

	chains := map[string]struct{}{
		"prime":   {},
		"vector":  {},
		"cardano": {},
	}

	if _, ok := chains[parts[0]]; parts[0] == "nexus" || !ok {
		return errReturnFn(fmt.Errorf("invalid chain name: %s", parts[0]))
	}

	for _, key := range parts[1:] {
		if _, err := hex.DecodeString(key); err != nil {
			return errReturnFn(fmt.Errorf("invalid key hex format, %w", err))
		}
	}

	return parts[0], [4]string{parts[1], parts[2], parts[3], parts[4]}, nil
}

// func parseEVMLikeChainEntry(entry string) (string, [4]string, error) {
// 	errReturnFn := func(err error) (string, [4]string, error) {
// 		return "", [4]string{}, err
// 	}

// 	parts := strings.Split(entry, ":")
// 	if len(parts) != 2 {
// 		return errReturnFn(fmt.Errorf("invalid entry format"))
// 	}

// 	if parts[0] != "nexus" {
// 		return errReturnFn(fmt.Errorf("invalid chain name: %s", parts[0]))
// 	}

// 	keyBytes, err := hex.DecodeString(parts[1])
// 	if err != nil {
// 		return errReturnFn(fmt.Errorf("invalid key hex format, %w", err))
// 	}

// 	pubKey, err := bls.UnmarshalPublicKey(keyBytes)
// 	if err != nil {
// 		return errReturnFn(fmt.Errorf("cannot unmarshal public key, %w", err))
// 	}

// 	bigInts := pubKey.ToBigInt()

// 	keys := [4]string{
// 		fmt.Sprintf("%064x", bigInts[0]),
// 		fmt.Sprintf("%064x", bigInts[1]),
// 		fmt.Sprintf("%064x", bigInts[2]),
// 		fmt.Sprintf("%064x", bigInts[3]),
// 	}

// 	return parts[0], keys, nil
// }

func runCommand(cmd *cobra.Command, _ []string) error {
	address, err := types.IsValidAddress(addressParam, false)
	if err != nil {
		return fmt.Errorf("not a valid address, %w", err)
	}

	proposal, err := common.LoadProposal[schema.ValidatorSetChangeProposal](fileParam)
	if err != nil {
		return err
	}

	validator := schema.Validator{
		Address: address.String(),
		Chains:  make(map[string]schema.Key),
	}

	validChains := map[string]struct{}{}

	if len(proposal.Added) == 0 {
		validChains = map[string]struct{}{
			"prime":   {},
			"vector":  {},
			"cardano": {},
			"nexus":   {},
		}
	} else {
		for chain := range proposal.Added[0].Chains {
			validChains[chain] = struct{}{}
		}
	}

	for _, chain := range cardanoLikeChainsParam {
		name, keys, err := parseCardanoLikeChainEntry(chain)
		if err != nil {
			return fmt.Errorf("invalid cardano-like chain entry: %w", err)
		}

		if _, ok := validator.Chains[name]; ok {
			return fmt.Errorf("duplicate chain entry: %s", name)
		}

		if _, ok := validChains[name]; !ok {
			return fmt.Errorf("chain entry %s not found for other validators", name)
		}

		validator.Chains[name] = schema.Key{Key: keys}
	}

	// for _, chain := range evmLikeChainsParam {
	// 	name, keys, err := parseEVMLikeChainEntry(chain)
	// 	if err != nil {
	// 		return fmt.Errorf("invalid evm-like chain entry: %w", err)
	// 	}

	// 	if _, ok := validator.Chains[name]; ok {
	// 		return fmt.Errorf("duplicate chain entry: %s", name)
	// 	}

	// 	if _, ok := validChains[name]; !ok {
	// 		return fmt.Errorf("chain entry %s not found for other validators", name)
	// 	}

	// 	validator.Chains[name] = schema.Key{Key: keys}
	// }

	keyBytes, err := hex.DecodeString(bladeParam)
	if err != nil {
		return fmt.Errorf("invalid key hex format for blade network, %w", err)
	}

	pubKey, err := bls.UnmarshalPublicKey(keyBytes)
	if err != nil {
		return fmt.Errorf("cannot unmarshal blade public key, %w", err)
	}

	bigInts := pubKey.ToBigInt()

	bladeKeys := [4]string{
		fmt.Sprintf("%064x", bigInts[0]),
		fmt.Sprintf("%064x", bigInts[1]),
		fmt.Sprintf("%064x", bigInts[2]),
		fmt.Sprintf("%064x", bigInts[3]),
	}

	validator.Chains["blade"] = schema.Key{Key: bladeKeys}

	if nexusParam {
		if _, ok := validChains["nexus"]; !ok {
			return fmt.Errorf("chain entry nexus not found for other validators")
		}
		validator.Chains["nexus"] = schema.Key{Key: bladeKeys}
	}

	for i, removed := range proposal.Removed {
		if removed == validator.Address {
			proposal.Removed = append(proposal.Removed[:i], proposal.Removed[i+1:]...)

			break
		}
	}

	replacement := false

	for i, existing := range proposal.Added {
		if existing.Address == validator.Address {
			proposal.Added[i] = validator
			replacement = true

			break
		}
	}

	if !replacement {
		proposal.Added = append(proposal.Added, validator)
	}

	return common.StoreProposal(proposal, fileParam)
}
