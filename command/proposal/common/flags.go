package common

import "github.com/spf13/cobra"

func AddSubmitAndRelatedFlags(
	cmd *cobra.Command,
	submit *bool,
	privateKey,
	rpcURL,
	description *string) {
	cmd.Flags().BoolVarP(
		submit,
		"submit",
		"s",
		false,
		"If set, the proposal is automatically submitted after creation.",
	)

	cmd.Flags().StringVarP(
		privateKey,
		"private-key",
		"k",
		"",
		"Private key used for signing the proposal transaction.",
	)

	cmd.Flags().StringVarP(
		rpcURL,
		"rpc-url",
		"u",
		"",
		"RPC endpoint URL for blockchain network.",
	)

	cmd.Flags().StringVarP(
		description,
		"description",
		"d",
		"",
		"Proposal description.",
	)
}
