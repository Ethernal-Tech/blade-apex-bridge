package precompiled

import (
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/0xPolygon/polygon-edge/bls"
	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/bitmap"
	"github.com/0xPolygon/polygon-edge/state/runtime"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/Ethernal-Tech/bn256"
	"github.com/ethereum/go-ethereum/accounts/abi"
	ethgo_abi "github.com/umbracle/ethgo/abi"
)

const (
	apexBLSSingleTypeByte = 0
	apexBLSMultiTypeByte  = 1
)

var (
	errApexBLSSignatureVerificationInvalidInput     = errors.New("invalid input")
	errApexBLSSignatureVerificationQuorumNotReached = errors.New("quorum not reached")
	// apexBLSInputDataMultiABIType is the ABI signature of the precompiled contract input data
	apexBLSInputDataMultiABIType, _ = abi.JSON(strings.NewReader(`[{
		"outputs":[
        	{"name":"hash","type":"bytes32"},
        	{"name":"signature","type":"bytes"},
        	{
				"name":"validatorsData",
				"type":"tuple[]",
				"components":[
					{"name":"key","type":"uint256[4]"}
				]
			},
        	{"name":"bitmap","type":"uint256"}
		],
    	"name":"nexus",
		"type":"function"
	}]`))

	// abi.MustNewType("tuple(bytes32, bytes, uint256[4][], uint256)")
	// (hash, signature, blsPublicKey)
	apexBLSInputDataSingleABIType = ethgo_abi.MustNewType("tuple(bytes32, bytes, uint256[4])")
)

// apexBLSSignatureVerification verifies the given aggregated signatures using the default BLS utils functions.
// apexBLSSignatureVerification returns ABI encoded boolean value depends on validness of the given signatures.
type apexBLSSignatureVerification struct {
	domain []byte
}

// gas returns the gas required to execute the pre-compiled contract
func (c *apexBLSSignatureVerification) gas(input []byte, _ *chain.ForksInTime) uint64 {
	return 50000
}

type nexusValidatorData struct {
	Key [4]*big.Int `abi:"key"`
}

type nexusBlsPrecompileParams struct {
	Hash           [32]byte             `abi:"hash"`
	Signature      []byte               `abi:"signature"`
	ValidatorsData []nexusValidatorData `abi:"validatorsData"`
	Bitmap         *big.Int             `abi:"bitmap"`
}

// Run runs the precompiled contract with the given input.
// Input must be ABI encoded:
// - if first byte is 0 then other bytes are decoded as tuple(bytes32, bytes, uint256[4])
// - otherwise other input bytes are decoded as tuple(bytes32, bytes, uint256[4][], bytes)
// Output could be an error or ABI encoded "bool" value
func (c *apexBLSSignatureVerification) run(input []byte, caller types.Address, host runtime.Host) ([]byte, error) {
	if len(input) == 0 {
		return nil, errApexBLSSignatureVerificationInvalidInput
	}

	isSingle := input[0] == apexBLSSingleTypeByte

	if isSingle {
		return runSingle(input, c.domain)
	} else {
		return runMulti(input, c.domain)
	}
}

func runSingle(input []byte, domain []byte) ([]byte, error) {
	var (
		rawData interface{}
		err     error
	)

	rawData, err = ethgo_abi.Decode(apexBLSInputDataSingleABIType, input[1:])
	if err != nil {
		return nil, fmt.Errorf("%w: single = %v - %w",
			errApexBLSSignatureVerificationInvalidInput, true, err)
	}

	var (
		data                 = rawData.(map[string]interface{})   //nolint:forcetypeassert
		msg                  = data["0"].([types.HashLength]byte) //nolint:forcetypeassert
		signatureBytes       = data["1"].([]byte)                 //nolint:forcetypeassert
		publicKeysSerialized [][4]*big.Int
	)

	publicKey := data["2"].([4]*big.Int) //nolint:forcetypeassert
	publicKeysSerialized = [][4]*big.Int{publicKey}

	signature, err := bls.UnmarshalSignature(signatureBytes)
	if err != nil {
		return nil, fmt.Errorf("%w: signature - %w", errApexBLSSignatureVerificationInvalidInput, err)
	}

	blsPubKeys := make([]*bls.PublicKey, len(publicKeysSerialized))

	for i, pk := range publicKeysSerialized {
		blsPubKey, err := bls.UnmarshalPublicKeyFromBigInt(pk)
		if err != nil {
			return nil, fmt.Errorf("%w: public key - %w", errApexBLSSignatureVerificationInvalidInput, err)
		}

		blsPubKeys[i] = blsPubKey
	}

	if signature.VerifyAggregated(blsPubKeys, msg[:], domain) {
		return abiBoolTrue, nil
	}

	return abiBoolFalse, nil
}

func runMulti(input []byte, domain []byte) ([]byte, error) {
	if len(input) < 2 {
		return nil, errApexBLSSignatureVerificationInvalidInput
	}

	var inputObj nexusBlsPrecompileParams

	if err := apexBLSInputDataMultiABIType.UnpackIntoInterface(&inputObj, "nexus", input[1:]); err != nil {
		return nil, err
	}

	publicKeys := make([]*bn256.PublicKey, 0, len(inputObj.ValidatorsData))
	bitmap := bitmap.Bitmap(inputObj.Bitmap.Bytes())

	for i, pkSerialized := range inputObj.ValidatorsData {
		//nolint:gosec
		if !bitmap.IsSet(uint64(i)) {
			continue
		}

		pubKey, err := bn256.UnmarshalPublicKeyFromBigInt(pkSerialized.Key)
		if err != nil {
			return nil, fmt.Errorf("%w: public key - %w", errApexBLSSignatureVerificationInvalidInput, err)
		}

		publicKeys = append(publicKeys, pubKey)
	}

	quorumCnt := (len(inputObj.ValidatorsData)*2)/3 + 1
	// ensure that the number of serialized public keys meets the required quorum count
	if len(publicKeys) < quorumCnt {
		return nil, errApexBLSSignatureVerificationQuorumNotReached
	}

	signature, err := bn256.UnmarshalSignature(inputObj.Signature)
	if err != nil {
		return nil, fmt.Errorf("%w: signature - %w", errApexBLSSignatureVerificationInvalidInput, err)
	}

	if signature.VerifyAggregated(publicKeys, inputObj.Hash[:], domain) {
		return abiBoolTrue, nil
	}

	return abiBoolFalse, nil
}
