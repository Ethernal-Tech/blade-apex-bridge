package proposal

import (
	"errors"
)

const (
	proposalIDFlag     = "proposal-id"
	jsonRPCAddressFlag = "json-rpc"
)

var (
	errInvalidProposalID     = errors.New("proposal ID must be a integer")
	errInvalidJSONRPCAddress = errors.New("JSON-RPC address must be provided")
)

type getProposalParams struct {
	proposalID     string
	jsonRPCAddress string
}

func (gpp *getProposalParams) validateFlags() error {
	if gpp.proposalID == "" {
		return errInvalidProposalID
	}

	if gpp.jsonRPCAddress == "" {
		return errInvalidJSONRPCAddress
	}

	return nil
}
