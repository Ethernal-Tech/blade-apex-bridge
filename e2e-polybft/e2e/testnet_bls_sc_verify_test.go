package e2e

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/bitmap"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/signer"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/jsonrpc"
	"github.com/0xPolygon/polygon-edge/types"
	bn256 "github.com/Ethernal-Tech/bn256"
	"github.com/Ethernal-Tech/ethgo/abi"
	"github.com/stretchr/testify/require"
)

// contract: https://github.com/Ethernal-Tech/apex-evm-gateway/blob/feat/bls_checker/contracts/BLSChecker.sol

const (
	contractAddrStr = "0xfa5Ea009062A1344AD7c194FC9DBc869E05CCfc1"
	rpcURL          = "https://rpc.nexus.testnet.apexfusion.org"
)

type validatorChainData struct {
	Key [4]*big.Int `abi:"key"`
}

type blsVerifyTestCase struct {
	validators []validatorChainData
	signatures bn256.Signatures
	message    []byte
	bitmap     *big.Int
	domain     string
}

func (tc blsVerifyTestCase) String() string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("TestCase: %d\n", len(tc.validators)))

	sb.WriteString("\tValidators: {\n")

	for _, v := range tc.validators {
		sb.WriteString(fmt.Sprintf("\t\t%s %s %s %s\n",
			hex.EncodeToString(v.Key[0].Bytes()),
			hex.EncodeToString(v.Key[1].Bytes()),
			hex.EncodeToString(v.Key[2].Bytes()),
			hex.EncodeToString(v.Key[3].Bytes())))
	}

	sb.WriteString("\t}\n")

	sb.WriteString("\tSignatures: {\n")

	for _, s := range tc.signatures {
		sBytes, _ := s.Marshal()

		sb.WriteString(fmt.Sprintf("\t\t%s\n", hex.EncodeToString(sBytes)))
	}

	sb.WriteString("\t}\n")

	sb.WriteString(fmt.Sprintf("\tMessage: %s\n", hex.EncodeToString(tc.message)))
	sb.WriteString(fmt.Sprintf("\tDomain: %s\n", tc.domain))
	sb.WriteString(fmt.Sprintf("\tBitmap: %v", tc.bitmap))

	return sb.String()
}

type blsVerifySCData struct {
	validators []validatorChainData
	message    string
	signature  string
	bitmap     *big.Int
	domain     string
}

func (tc blsVerifySCData) String() string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("SC Data: %d\n", len(tc.validators)))

	sb.WriteString("\tValidators: {\n")

	for _, v := range tc.validators {
		sb.WriteString(fmt.Sprintf("\t\t%s %s %s %s\n",
			hex.EncodeToString(v.Key[0].Bytes()),
			hex.EncodeToString(v.Key[1].Bytes()),
			hex.EncodeToString(v.Key[2].Bytes()),
			hex.EncodeToString(v.Key[3].Bytes())))
	}

	sb.WriteString("\t}\n")

	sb.WriteString(fmt.Sprintf("\tSignature: %s\n", tc.signature))
	sb.WriteString(fmt.Sprintf("\tMessage: %s\n", tc.message))
	sb.WriteString(fmt.Sprintf("\tDomain: %s\n", tc.domain))
	sb.WriteString(fmt.Sprintf("\tBitmap: %v", tc.bitmap))

	return sb.String()
}

func (tc blsVerifyTestCase) ToSCData() (*blsVerifySCData, error) {
	aggSignature, err := tc.signatures.Aggregate().Marshal()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal agg signature: %w", err)
	}

	return &blsVerifySCData{
		validators: tc.validators,
		signature:  hex.EncodeToString(aggSignature),
		message:    hex.EncodeToString(tc.message),
		bitmap:     tc.bitmap,
		domain:     tc.domain,
	}, nil
}

func TestE2E_Testnet_SC_BLS_Verify_Hardcoded_Valid(t *testing.T) {
	hardcodedTestCase, err := getHardcodedTestCase()
	require.NoError(t, err)

	t.Logf("\n%s\n", hardcodedTestCase)

	requireBLSVerifyBothValid(t, hardcodedTestCase)
}

func TestE2E_Testnet_SC_BLS_Verify_Hardcoded_Invalid(t *testing.T) {
	hardcodedTestCase, err := getHardcodedTestCase()
	require.NoError(t, err)

	t.Logf("\n\nOriginal: \n%s\n\n", hardcodedTestCase)

	t.Logf("Replacing all 'a' with 'b' from signature\n")

	hardcodedTestCase.signature = strings.ReplaceAll(hardcodedTestCase.signature, "a", "b")

	t.Logf("\n\nAltered: \n%s\n\n", hardcodedTestCase)

	requireBLSVerifyBothInvalid(t, hardcodedTestCase)
}

func TestE2E_Testnet_SC_BLS_Verify_Valid(t *testing.T) {
	testCase, err := generateTestCase(randomValCnt())
	require.NoError(t, err)

	t.Logf("\n%s\n\n", testCase)

	data, err := testCase.ToSCData()
	require.NoError(t, err)

	requireBLSVerifyBothValid(t, data)
}

func TestE2E_Testnet_SC_BLS_Verify_WrongSig(t *testing.T) {
	testCase, err := generateTestCase(randomValCnt())
	require.NoError(t, err)

	t.Logf("\n\nOriginal: \n%s\n\n", testCase)

	data, err := testCase.ToSCData()
	require.NoError(t, err)

	t.Logf("\n\nOriginal: \n%s\n\n", data)

	requireBLSVerifyBothValid(t, data)

	bytes, err := hex.DecodeString(data.signature)
	require.NoError(t, err)

	t.Logf("Changing first byte of the aggregated signature\n")

	bytes[0]++
	data.signature = hex.EncodeToString(bytes)

	t.Logf("\n\nAltered: \n%s\n\n", data)

	requireBLSVerifyBothInvalid(t, data)
}

// Domain is only a parameter of the SC call; the precompile has it baked in
// (DOMAIN_APEX_BRIDGE_EVM), so this scenario is SC-only.
func TestE2E_Testnet_SC_BLS_Verify_WrongDomain(t *testing.T) {
	testCase, err := generateTestCase(randomValCnt())
	require.NoError(t, err)

	t.Logf("\n\nOriginal: \n%s\n\n", testCase)

	data, err := testCase.ToSCData()
	require.NoError(t, err)

	t.Logf("\n\nOriginal: \n%s\n\n", data)

	isValid, err := callBLSVerifySC(rpcURL, contractAddrStr, data)

	require.NoError(t, err)
	require.True(t, isValid)

	t.Logf("Adding ' ' at the end of domain\n")

	data.domain = fmt.Sprintf("%s ", data.domain)

	t.Logf("\n\nAltered: \n%s\n\n", data)

	isValid, err = callBLSVerifySC(rpcURL, contractAddrStr, data)

	require.NoError(t, err)
	require.False(t, isValid)
}

func TestE2E_Testnet_SC_BLS_Verify_WrongMsg(t *testing.T) {
	testCase, err := generateTestCase(randomValCnt())
	require.NoError(t, err)

	t.Logf("\n\nOriginal: \n%s\n\n", testCase)

	data, err := testCase.ToSCData()
	require.NoError(t, err)

	requireBLSVerifyBothValid(t, data)

	t.Logf("Changing first byte of the message\n")

	testCase.message[0]++
	t.Logf("\n\nAltered: \n%s\n\n", testCase)

	data, err = testCase.ToSCData()
	require.NoError(t, err)

	requireBLSVerifyBothInvalid(t, data)
}

func TestE2E_Testnet_SC_BLS_Verify_WrongBitmap(t *testing.T) {
	valCnt := randomValCnt()
	testCase, err := generateTestCase(valCnt)
	require.NoError(t, err)

	t.Logf("\n\nOriginal: \n%s\n\n", testCase)

	data, err := testCase.ToSCData()
	require.NoError(t, err)

	requireBLSVerifyBothValid(t, data)

	data.bitmap = new(big.Int).Add(data.bitmap, big.NewInt(1))

	t.Logf("\nTrying bitmap: %v\n", data.bitmap)

	requireBLSVerifyBothInvalid(t, data)

	data.bitmap = new(big.Int).Sub(new(big.Int).Exp(big.NewInt(2), big.NewInt(int64(valCnt)), nil), big.NewInt(1))

	t.Logf("\nTrying bitmap: %v\n", data.bitmap)

	requireBLSVerifyBothInvalid(t, data)

	data.bitmap = big.NewInt(0)

	t.Logf("\nTrying bitmap: %v\n", data.bitmap)

	requireBLSVerifyBothInvalid(t, data)
}

func TestE2E_Testnet_SC_BLS_Verify_ReplaceWithWrongValidator(t *testing.T) {
	testCase, err := generateTestCase(randomValCnt())
	require.NoError(t, err)

	t.Logf("\n\nOriginal: \n%s\n\n", testCase)

	data, err := testCase.ToSCData()
	require.NoError(t, err)

	t.Logf("\n\nOriginal: \n%s\n\n", data)

	requireBLSVerifyBothValid(t, data)

	key, err := bn256.GeneratePrivateKey()
	require.NoError(t, err)

	data.validators[0].Key = key.PublicKey().ToBigInt()

	t.Logf("Setting the first validator to a new value\n")

	t.Logf("\n\nAltered: \n%s\n\n", data)

	requireBLSVerifyBothInvalid(t, data)
}

func TestE2E_Testnet_SC_BLS_Verify_WrongValidatorAddedToStart(t *testing.T) {
	testCase, err := generateTestCase(randomValCnt())
	require.NoError(t, err)

	t.Logf("\n\nOriginal: \n%s\n\n", testCase)

	data, err := testCase.ToSCData()
	require.NoError(t, err)

	t.Logf("\n\nOriginal: \n%s\n\n", data)

	requireBLSVerifyBothValid(t, data)

	key, err := bn256.GeneratePrivateKey()
	require.NoError(t, err)

	t.Logf("Adding a new validator to the start\n")

	data.validators = append([]validatorChainData{{Key: key.PublicKey().ToBigInt()}}, data.validators...)

	t.Logf("\n\nAltered: \n%s\n\n", data)

	requireBLSVerifyBothInvalid(t, data)
}

func TestE2E_Testnet_SC_BLS_Verify_SomeValidatorsRemoved(t *testing.T) {
	testCase, err := generateTestCase(randomValCnt())
	require.NoError(t, err)

	t.Logf("\n\nOriginal: \n%s\n\n", testCase)

	data, err := testCase.ToSCData()
	require.NoError(t, err)

	t.Logf("\n\nOriginal: \n%s\n\n", data)

	requireBLSVerifyBothValid(t, data)

	t.Logf("Removing first 2 validators from the start\n")

	data.validators = data.validators[2:]

	t.Logf("\n\nAltered: \n%s\n\n", data)

	requireBLSVerifyBothInvalid(t, data)
}

func TestE2E_Testnet_SC_BLS_Verify_ReplaceWithWrongSignature(t *testing.T) {
	testCase, err := generateTestCase(randomValCnt())
	require.NoError(t, err)

	t.Logf("\n\nOriginal: \n%s\n\n", testCase)

	data, err := testCase.ToSCData()
	require.NoError(t, err)

	requireBLSVerifyBothValid(t, data)

	key, err := bn256.GeneratePrivateKey()
	require.NoError(t, err)

	domain := crypto.Keccak256([]byte(testCase.domain))

	sig, err := key.Sign(testCase.message, domain)
	require.NoError(t, err)

	t.Logf("Setting the first signature to a new signature created by a new validator\n")

	testCase.signatures[0] = sig

	t.Logf("\n\nAltered: \n%s\n\n", testCase)

	data, err = testCase.ToSCData()
	require.NoError(t, err)

	requireBLSVerifyBothInvalid(t, data)
}

func TestE2E_Testnet_SC_BLS_Verify_WrongSignatureAddedToStart(t *testing.T) {
	testCase, err := generateTestCase(randomValCnt())
	require.NoError(t, err)

	t.Logf("\n\nOriginal: \n%s\n\n", testCase)

	data, err := testCase.ToSCData()
	require.NoError(t, err)

	requireBLSVerifyBothValid(t, data)

	key, err := bn256.GeneratePrivateKey()
	require.NoError(t, err)

	domain := crypto.Keccak256([]byte(testCase.domain))

	sig, err := key.Sign(testCase.message, domain)
	require.NoError(t, err)

	t.Logf("Adding a new signature created by a new validator to the start\n")

	testCase.signatures = append(bn256.Signatures{sig}, testCase.signatures...)

	t.Logf("\n\nAltered: \n%s\n\n", testCase)

	data, err = testCase.ToSCData()
	require.NoError(t, err)

	requireBLSVerifyBothInvalid(t, data)
}

func TestE2E_Testnet_SC_BLS_Verify_SomeSignaturesRemoved(t *testing.T) {
	testCase, err := generateTestCase(randomValCnt())
	require.NoError(t, err)

	t.Logf("\n\nOriginal: \n%s\n\n", testCase)

	data, err := testCase.ToSCData()
	require.NoError(t, err)

	requireBLSVerifyBothValid(t, data)

	t.Logf("Removing first 2 signatures from the start\n")

	testCase.signatures = testCase.signatures[2:]

	t.Logf("\n\nAltered: \n%s\n\n", testCase)

	data, err = testCase.ToSCData()
	require.NoError(t, err)

	requireBLSVerifyBothInvalid(t, data)
}

func TestE2E_Testnet_SC_BLS_Verify_QuorumCheck_1(t *testing.T) {
	testCase, err := generateTestCase(randomValCnt())
	require.NoError(t, err)

	t.Logf("\n\nOriginal: \n%s\n\n", testCase)

	data, err := testCase.ToSCData()
	require.NoError(t, err)

	requireBLSVerifyBothValid(t, data)

	t.Logf("Removing the first signature from the start and setting the bitmap accordingly\n")

	testCase.signatures = testCase.signatures[1:]
	testCase.bitmap = new(big.Int).Sub(testCase.bitmap, big.NewInt(3))

	t.Logf("\n\nAltered: \n%s\n\n", testCase)

	data, err = testCase.ToSCData()
	require.NoError(t, err)

	requireBLSVerifyBothInvalid(t, data)
}

func TestE2E_Testnet_SC_BLS_Verify_QuorumCheck_2(t *testing.T) {
	testCase, err := generateTestCase(randomValCnt())
	require.NoError(t, err)

	t.Logf("\n\nOriginal: \n%s\n\n", testCase)

	data, err := testCase.ToSCData()
	require.NoError(t, err)

	requireBLSVerifyBothValid(t, data)

	t.Logf("Replacing the signatures with just the first signature and setting the bitmap accordingly\n")

	testCase.signatures = []*bn256.Signature{testCase.signatures[0]}
	testCase.bitmap = big.NewInt(1)

	t.Logf("\n\nAltered: \n%s\n\n", testCase)

	data, err = testCase.ToSCData()
	require.NoError(t, err)

	requireBLSVerifyBothInvalid(t, data)
}

// generateTestCase generates a test case signed with the DOMAIN_APEX_BRIDGE_EVM domain,
// which is the domain baked into the precompile, so the generated data can be verified
// through both the Solidity SC and the precompile
func generateTestCase(totalValCnt uint32) (*blsVerifyTestCase, error) {
	return generateTestCaseWithDomain(totalValCnt, signer.DomainApexBridgeEVMString)
}

func generateTestCaseWithDomain(totalValCnt uint32, domainStr string) (*blsVerifyTestCase, error) {
	msg := crypto.Keccak256([]byte(randomString(20)))

	domain := crypto.Keccak256([]byte(domainStr))

	validatorPKs := make([]*bn256.PrivateKey, totalValCnt)
	validatorPubKeys := make([]validatorChainData, totalValCnt)

	for i := range totalValCnt {
		key, err := bn256.GeneratePrivateKey()
		if err != nil {
			return nil, fmt.Errorf("failed to create validator pk: %w", err)
		}

		validatorPKs[i] = key
		validatorPubKeys[i] = validatorChainData{Key: key.PublicKey().ToBigInt()}
	}

	quorumCnt := totalValCnt*2/3 + 1

	signatures := make(bn256.Signatures, quorumCnt)

	for i := range quorumCnt {
		sig, err := validatorPKs[i].Sign(msg, domain)
		if err != nil {
			return nil, fmt.Errorf("failed to sign msg with validator pk: %w", err)
		}

		signatures[i] = sig
	}

	bitmap := new(big.Int).Sub(new(big.Int).Exp(big.NewInt(2), big.NewInt(int64(quorumCnt)), nil), big.NewInt(1))

	return &blsVerifyTestCase{
		validators: validatorPubKeys,
		bitmap:     bitmap,
		signatures: signatures,
		message:    msg,
		domain:     domainStr,
	}, nil
}

func getFailingHardcodedTestCase() (*blsVerifyTestCase, error) {
	validatorsDataStr := [][4]string{
		{
			"2c55e1b7fc65245c5af1940a3f0a588f1fbd5d1899a4e73330d1b054629688f5",
			"10342d974b69b0d9f733827f239e98daa411ba0d5d41d4680ac78b4cc9c59211",
			"1fce76d0a1d97aab820e3df8982b28dca5be71646e77a625fc80ced7161dd2ce",
			"1659cca493b14b4ff6b6b69f814035af6c81f5894fce01cc71fd5632260a129a",
		},
		{
			"136cc857784c24268d8c3e349ec9d2366efb75c8f2253d711ff472957cf1165a",
			"042c054e19ab129f3b3af23c00cc4fc8a2d4b2d06ee6d63820f1c0ead063273f",
			"12a8545d735b225248736115c7587e9f9a3d4bbc3f039ddea208550accb5613f",
			"2673401045eae9715455347790175f005ac828c8e52c376f7995bfe4536793c9",
		},
		{
			"1075b5afd9648b7bf96c39ce8aa15f685885e4b4a86b9532cd3b5a8fc76a6ce3",
			"229ade784cae23a5ef7a79fd0b89a3fc98b42e4ced505e286795ec64a73343a7",
			"053ebdf33d776b27da2de5ae3b131ea308ed707e66212355fc4da19ba7c48385",
			"0627801c47d60cab028c882010298fe75cac1f50a23412535c2e149982a89b0d",
		},
		{
			"15d833657328fd3077e50e8eebcc4f5d1e533f8504241ef0d645139dc989577b",
			"16f56046425c14bb72b9089f533aaff05239c33849c7856dff2352503aaef21f",
			"29254a1416113ffe8390f2c7e32287fe784671c3ca2df7511fe17735a326d5e2",
			"0dd703ec60994981b80926baf2bca772c3027f9c6b3267236957434685ce359b",
		},
		{
			"042f51a816fcfa9abd2036ddf453277d2bfe89dfee39b22aa17cd651fd7ff7cc",
			"2972af2ac13c840155ff807307db4d8af6bcd26d3fbdabd016108f93a4f535ef",
			"2ed7d6d02f58ba76491fe15f78908f6caba1e6800cfcc2b7d2886f93b2418fea",
			"1561a15084a687531c7675a6d89c89063af3a48c17ee017b42b8414348a2f60b",
		},

		{
			"1b28a05fa5d8cc5042d1db5f87947730097e616f3f3c32fff181d3b0f36ad21f",
			"0fc7c10dac68f9f1b1a62f9379fc16d8ea4f10d83eee1a4f3e1e899da947ccc1",
			"1de02eb48412886dbe5d5f1c7bcaf70630aeeb92a73e6a01792a77342f9c4c4f",
			"100188f180d852741cb4e206f247e880c1e5fbb1707ed3e4dc4f0330a8a22ce0",
		},
		{
			"05d9f2ea347b26b850883e6766d7876dc712acbc49b4e94e2e2418d2c39aa386",
			"08617f12750467286e06c7122cf33228a9e9a71a12ec9d6d566d7a971ecc2ea8",
			"19d0e3f65876b0855c930e53642dc93f2dc2ea4bb0ff082148c8abbac1405f8d",
			"192bb44bb92ebcb3058e7dda24fb8b6dba8293cbe4c34a76ba0549e69afd4fb4",
		},
		{
			"2fd0886750c96db3d3fa0d3a40708dcfe7aecb34127c3372ac888f336324e8e4",
			"0f8c7982557c9e9c067ec620b64cb02464e207da1bf577b41d0f1ecacd02210d",
			"06d4a613d9e075a10b9120c82d2ee7d797c58bd090cb3b8d9b93734714606913",
			"09f60d5f59f8f4e7759b1436978abc8ccca993682082f5cc857f86b288dbea79",
		},
		{
			"02f8397de3797e5470ce4d1faff4d48483f464f8282f8cdb3dbd7f757353e92f",
			"0b1ccecc88c5732c43bf682ba3a1de831cc7f076df859baff94409334732822f",
			"1fce7d4a1abc75799dde0ea506571c9f5658ab0e89d17f260b2f57c27568278a",
			"2bb053f7ca35be54bc19e3171285e14ef86bcfbee63be34af38290d4e7928d8f",
		},
		{
			"14868262a32fa793bb6f14fef650d00e9dd9cfee4d30bb37e1ea0de41edaab46",
			"25b0aba963410d0aa2434d0308483125e0b44dbaa823c8c879eb675904e675eb",
			"2845cb97b7d6b62d98bf8610d165ad322da016b06b332bd1ba059cba7a0729bb",
			"057f2557b47e543b1815786aa29e4c3e8005b361828c90600cc5f148d538ce4f",
		},
	}

	msgStr := "7ce70b5a7f3f94983cb95bd90533b8e699993a128576638839099adca766c4f9"

	msg, err := hex.DecodeString(msgStr)
	if err != nil {
		return nil, fmt.Errorf("failed to decode msg str: %s %w", msgStr, err)
	}

	bitmap := big.NewInt(127)
	domain := "LUcNqv9zvl"

	validatorsData := make([]validatorChainData, 0, len(validatorsDataStr))

	for _, vDataStr := range validatorsDataStr {
		validatorData := make([]*big.Int, 0, len(vDataStr))

		for _, keyStr := range vDataStr {
			keyBytes, err := hex.DecodeString(keyStr)
			if err != nil {
				return nil, fmt.Errorf("couldn't hex decode validator: %s %w", keyStr, err)
			}

			validatorData = append(validatorData, new(big.Int).SetBytes(keyBytes))
		}

		vDataReal := [4]*big.Int{
			new(big.Int).Set(validatorData[0]),
			new(big.Int).Set(validatorData[1]),
			new(big.Int).Set(validatorData[2]),
			new(big.Int).Set(validatorData[3]),
		}

		validatorsData = append(validatorsData, validatorChainData{Key: vDataReal})
	}

	signaturesStr := []string{
		"2d1c1522886c061141949de7961ed33a183c6e6b7e3600d6b16f913b72092cbb0440a6b2af7f183b59c640af0aae03366c565b76d82d2303111fc26ab8a426ca",
		"229bab940cccaba87803fd6e950a1439f925da913169affba7b3d039cd579b8909d13868819a9abdb4bbb65da47399390ddb19fcea2ddf2b303843a29d8784ef",
		"175e8cce8bfb38466554290a9fd9839cd74fe3baa9963320b953e0b31fec668b2a83b93a79637bf8d1d0a195d2765b22b20dbd6cfd69de39fdd8e9080cdee33e",
		"2bd85dd6553b2cbc01131ad5d635751e4278caff25cc84193b63bf66c9ccd16f0c02c8461848662f18a038a38baab09a074934fe2a2208cdd1433a981fe6b80a",
		"1a045e1b3a6e80ac6dc9044f2c313cb2feff21034f69eba93da2f60a3513953027f836ea136fe10772fd614a090590f0df3d186fe5605f76a90196ee5fed3854",
		"243681d14f90ac27dab538c1d12093c565180b58b80fe2c16057b0831b9e2b7814f54dc71849df4ddfbac95c7c157ca39da245bac591b89f30b496ee160be232",
		"1dee7d1620a75e3259ea3b25d56f2be4a88605562b4fa6cc5a22fadf2212c29112701009de8e876eb415064d43fc056e98359eef24f7cc2897a2b1636b758238",
	}

	signatures := make(bn256.Signatures, len(signaturesStr))

	for i, sigStr := range signaturesStr {
		sigBytes, err := hex.DecodeString(sigStr)
		if err != nil {
			return nil, fmt.Errorf("failed to hex decode sig str: %s %w", sigStr, err)
		}

		sig, err := bn256.UnmarshalSignature(sigBytes)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal sig str: %s %w", sigStr, err)
		}

		signatures[i] = sig
	}

	return &blsVerifyTestCase{
		validators: validatorsData,
		message:    msg,
		signatures: signatures,
		bitmap:     bitmap,
		domain:     domain,
	}, nil
}

func getHardcodedTestCase() (*blsVerifySCData, error) {
	validatorsDataStr := [][4]string{
		{
			"15701873806374165850870967628552138813442366237653248646332839595927358653601",
			"7658892991666924408470950539263072517326259928846891924465774938431028130881",
			"12657144703834455129265212040239156633504577346242820420815960398857557801954",
			"8906418974649167687964114989524814641987178660973184698561473836705686736593",
		},
		{
			"20590591856956730598003470969006234150694435173912860186438138965872898157547",
			"13586484450911837720775208292881824032630980786267989577208840590380364282790",
			"8202650316668796376943065988677772692605922495260748831851444906264523183714",
			"19695527430647132590332039258416643857709287559432539574580412026658506369637",
		},
		{
			"2683304820825451000389109221889245137323161128201647909088226087689580149876",
			"20225732498899454014654089792233883158072231598308161035709788978498075724376",
			"2586538199415369400382544304664745174840954915531626748265424881940879245418",
			"4306616956024289728827929185339756231962023240775669822109987393879036137801",
		},
		{
			"17917688665551140964431290754156251599127694187309066084341479724069944457830",
			"9109678565491767746524466409516061682916842837857038225326981948183647564008",
			"13646051973707407195053778018897466807639113509035731079672740614983628496656",
			"910684085218120840954743899165286247691039755343297723785016050632435859716",
		},
		{
			"11953357817893311561701669909628652045310131989763242307605931731871078586642",
			"3856931862907101293412602092155220361946411750575165833275785228068691210541",
			"20669388861458762276340390378820645130844960278590435809660541721619154599200",
			"10221122115669595064387937641616667291065278211681329788392094566471141070932",
		},
	}

	hash := "f08442bf50132cb8da60441649bf3c0decbb0f46230616e305ffe8aad5760607"
	signature := "233b2db0bab740f8656fc37e510181ffbc9a7f0c096918bb625dd5faed5195b11f5451583eedcd0f3ca1dcbec3e793b4498167ed4e372441f339c00f6badd29a"
	bitmap := big.NewInt(23)
	domain := "DOMAIN_APEX_BRIDGE_EVM"

	validatorsData := make([]validatorChainData, 0, len(validatorsDataStr))

	for _, vDataStr := range validatorsDataStr {
		validatorData := make([]*big.Int, 0, len(vDataStr))

		for _, keyStr := range vDataStr {
			key, ok := new(big.Int).SetString(keyStr, 0)
			if !ok {
				return nil, fmt.Errorf("couldn't convert %v to big.Int", keyStr)
			}

			validatorData = append(validatorData, key)
		}

		vDataReal := [4]*big.Int{
			new(big.Int).Set(validatorData[0]),
			new(big.Int).Set(validatorData[1]),
			new(big.Int).Set(validatorData[2]),
			new(big.Int).Set(validatorData[3]),
		}

		validatorsData = append(validatorsData, validatorChainData{Key: vDataReal})
	}

	return &blsVerifySCData{
		validators: validatorsData,
		message:    hash,
		signature:  signature,
		bitmap:     bitmap,
		domain:     domain,
	}, nil
}

func callBLSVerifySC(rpcURL string, contractAddrStr string, data *blsVerifySCData) (bool, error) {
	clt, err := jsonrpc.NewEthClient(rpcURL)
	if err != nil {
		return false, fmt.Errorf("failed to create new eth client: %w", err)
	}

	isBlsSignatureValidMethod, err := abi.NewMethod(`function isBlsSignatureValid(
        tuple(uint256[4] key)[] _validatorsChainData,
        bytes32 _hash,
        bytes _signature,
        uint256 _bitmap,
        string _domain
    ) external view returns (bool)`)
	if err != nil {
		return false, fmt.Errorf("failed to create method abi: %w", err)
	}

	isBlsSignatureValidData, err := isBlsSignatureValidMethod.Encode([]interface{}{
		data.validators,
		data.message,
		data.signature,
		data.bitmap,
		data.domain,
	})
	if err != nil {
		return false, fmt.Errorf("failed to encode isBlsSignatureValid call: %w", err)
	}

	contractAddr := types.StringToAddress(contractAddrStr)

	outHex, err := clt.Call(&jsonrpc.CallMsg{
		To:   &contractAddr,
		Data: isBlsSignatureValidData,
	}, jsonrpc.LatestBlockNumber, nil)
	if err != nil {
		return false, fmt.Errorf("failed to call the method: %w", err)
	}

	res, err := common.ParseUint256orHex(&outHex)
	if err != nil {
		return false, fmt.Errorf("failed to parse the call output: %w", err)
	}

	return res.BitLen() > 0, nil
}

// apexBLSPrecompileInputABIType is the input type of the multi (aggregated) mode of the
// apex BLS verification precompile: (hash, signature, blsPublicKeys, bitmap)
var apexBLSPrecompileInputABIType = abi.MustNewType("tuple(bytes32, bytes, uint256[4][], uint256)")

// valueBitmapToPrecompileBitmap re-encodes a validator participation bitmap so
// the same set of validators is selected by both the Solidity contract and the
// precompile.
//
// The contract treats the bitmap as a plain integer: bit i of the number means
// validator i participated.
//
// The precompile instead takes big.Int.Bytes() and interprets that byte slice as
// a bitmap.Bitmap, where the first byte holds validators 0-7, the second byte
// validators 8-15, and so on. For values that fit in one byte the two views
// match; for wider bitmaps the big-endian Bytes() layout no longer lines up
// with those validator indices, so this helper reads the integer bit indices
// and writes them back via bitmap.Set into the layout the precompile expects.
func valueBitmapToPrecompileBitmap(value *big.Int) *big.Int {
	bmp := bitmap.Bitmap{}

	for i := 0; i < value.BitLen(); i++ {
		if value.Bit(i) == 1 {
			bmp.Set(uint64(i))
		}
	}

	return new(big.Int).SetBytes(bmp)
}

// callBLSVerifyPrecompile calls the apex BLS verification precompile (0x2060) directly
// via eth_call, using the same test data as the Solidity SC call. Note that, unlike the
// SC, the precompile does not accept a domain parameter - the domain is fixed in the
// node to keccak256("DOMAIN_APEX_BRIDGE_EVM"), so data.domain is ignored here.
func callBLSVerifyPrecompile(rpcURL string, data *blsVerifySCData) (bool, error) {
	clt, err := jsonrpc.NewEthClient(rpcURL)
	if err != nil {
		return false, fmt.Errorf("failed to create new eth client: %w", err)
	}

	msgBytes, err := hex.DecodeString(data.message)
	if err != nil {
		return false, fmt.Errorf("failed to decode message hex: %w", err)
	}

	sigBytes, err := hex.DecodeString(data.signature)
	if err != nil {
		return false, fmt.Errorf("failed to decode signature hex: %w", err)
	}

	publicKeys := make([][4]*big.Int, len(data.validators))
	for i, v := range data.validators {
		publicKeys[i] = v.Key
	}

	encoded, err := abi.Encode([]interface{}{
		msgBytes,
		sigBytes,
		publicKeys,
		valueBitmapToPrecompileBitmap(data.bitmap),
	}, apexBLSPrecompileInputABIType)
	if err != nil {
		return false, fmt.Errorf("failed to encode precompile input: %w", err)
	}

	// first byte 1 denotes the multi (aggregated signature) input type
	input := append([]byte{1}, encoded...)

	precompileAddr := contracts.ApexBLSSignaturesVerificationPrecompile

	outHex, err := clt.Call(&jsonrpc.CallMsg{
		To:   &precompileAddr,
		Data: input,
	}, jsonrpc.LatestBlockNumber, nil)
	if err != nil {
		return false, fmt.Errorf("failed to call the precompile: %w", err)
	}

	res, err := common.ParseUint256orHex(&outHex)
	if err != nil {
		return false, fmt.Errorf("failed to parse the precompile call output: %w", err)
	}

	return res.BitLen() > 0, nil
}

// requireBLSVerifyBothValid asserts that both the Solidity SC and the precompile
// successfully verify the given data
func requireBLSVerifyBothValid(t *testing.T, data *blsVerifySCData) {
	t.Helper()

	isValid, err := callBLSVerifySC(rpcURL, contractAddrStr, data)

	require.NoError(t, err)
	require.True(t, isValid, "SC verification expected to succeed")

	isValid, err = callBLSVerifyPrecompile(rpcURL, data)

	require.NoError(t, err)
	require.True(t, isValid, "precompile verification expected to succeed")
}

// requireBLSVerifyBothInvalid asserts that both the Solidity SC and the precompile
// reject the given data. The SC always returns false for invalid data, while the
// precompile returns an error (reverts) for malformed input or when the quorum is
// not reached, so an error from the precompile is also treated as a rejection.
func requireBLSVerifyBothInvalid(t *testing.T, data *blsVerifySCData) {
	t.Helper()

	isValid, err := callBLSVerifySC(rpcURL, contractAddrStr, data)

	require.NoError(t, err)
	require.False(t, isValid, "SC verification expected to fail")

	isValid, err = callBLSVerifyPrecompile(rpcURL, data)
	if err != nil {
		t.Logf("precompile call returned an error (treated as a rejection): %v\n", err)

		return
	}

	require.False(t, isValid, "precompile verification expected to fail")
}

const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

func randomString(length int) string {
	rand.New(rand.NewSource(time.Now().UTC().UnixNano()))

	b := make([]byte, length)

	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}

	return string(b)
}

func randomValCnt() uint32 {
	return rand.Uint32()%10 + 5
}
