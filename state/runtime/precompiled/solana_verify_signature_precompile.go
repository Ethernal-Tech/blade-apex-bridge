package precompiled

import (
	"errors"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/state/runtime"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/Ethernal-Tech/ethgo/abi"
	"github.com/gagliardetto/solana-go"
)

var solanaVerifySignaturePrecompileInputABIType = abi.MustNewType("tuple(bytes, bytes, bytes32)")

type solanaVerifySignaturePrecompile struct {
}

func (c *solanaVerifySignaturePrecompile) gas(_ []byte, _ *chain.ForksInTime) uint64 {
	return 50000
}

func (c *solanaVerifySignaturePrecompile) run(input []byte, caller types.Address, _ runtime.Host) ([]byte, error) {
	rawData, err := abi.Decode(solanaVerifySignaturePrecompileInputABIType, input)
	if err != nil {
		return nil, errors.Join(runtime.ErrInvalidInputData, err)
	}

	data := rawData.(map[string]interface{}) //nolint: forcetypeassert
	rawTxOrMessage := data["0"].([]byte)     //nolint: forcetypeassert
	signatureBytes := data["1"].([]byte)     //nolint: forcetypeassert
	verifyingKey := data["2"].([32]byte)     //nolint: forcetypeassert

	if len(signatureBytes) != solana.SignatureLength {
		return nil, runtime.ErrInvalidInputData
	}

	tx, err := solana.TransactionFromBytes(rawTxOrMessage)
	if err != nil {
		// check for a raw message
		isValid := solana.PublicKey(verifyingKey).Verify(rawTxOrMessage, solana.SignatureFromBytes(signatureBytes))
		if !isValid {
			return abiBoolFalse, nil
		}

		return abiBoolTrue, nil
	}

	message, err := tx.Message.MarshalBinary()
	if err != nil {
		return nil, errors.Join(runtime.ErrInvalidInputData, err)
	}

	isValid := solana.PublicKey(verifyingKey).Verify(message, solana.SignatureFromBytes(signatureBytes))
	if !isValid {
		return abiBoolFalse, nil
	}

	return abiBoolTrue, nil
}
