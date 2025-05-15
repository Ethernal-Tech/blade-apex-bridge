package registration

import (
	"bytes"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/0xPolygon/polygon-edge/command/helper"
	sidechainHelper "github.com/0xPolygon/polygon-edge/command/validator/helper"
)

type registerParams struct {
	accountDir    string
	accountConfig string
	jsonRPC       string
	txTimeout     time.Duration
}

var errStakeTokenIsZeroAddress = errors.New("stake token address must not be zero address")

func (rp *registerParams) validateFlags() (err error) {
	// validate jsonrpc address
	_, err = helper.ParseJSONRPCAddress(rp.jsonRPC)
	if err != nil {
		return fmt.Errorf("failed to parse json rpc address. Error: %w", err)
	}

	return sidechainHelper.ValidateSecretFlags(rp.accountDir, rp.accountConfig)
}

type registerResult struct {
	ValidatorAddress string   `json:"validatorAddress"`
	KoskSignature    string   `json:"koskSignature"`
	Amount           *big.Int `json:"amount"`
}

func (rr registerResult) GetOutput() string {
	var buffer bytes.Buffer

	buffer.WriteString("\n[VALIDATOR REGISTRATION]\n")

	vals := make([]string, 0, 3)
	vals = append(vals, fmt.Sprintf("Validator Address|%s", rr.ValidatorAddress))
	vals = append(vals, fmt.Sprintf("KOSK Signature|%s", rr.KoskSignature))
	vals = append(vals, fmt.Sprintf("Amount Staked|%s", rr.Amount))
	buffer.WriteString(helper.FormatKV(vals))
	buffer.WriteString("\n")

	return buffer.String()
}
