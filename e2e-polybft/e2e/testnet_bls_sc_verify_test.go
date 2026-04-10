package e2e

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/jsonrpc"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/Ethernal-Tech/bn256"
	"github.com/Ethernal-Tech/ethgo/abi"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/sha3"
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

func TestE2E_SkylineTestnetBridge_SC_BLS_Verify_Hardcoded_Valid(t *testing.T) {
	hardcodedTestCase, err := getHardcodedTestCase()
	require.NoError(t, err)

	t.Logf("\n%s\n", hardcodedTestCase)

	isValid, err := callBLSVerifySC(rpcURL, contractAddrStr, hardcodedTestCase)

	require.NoError(t, err)
	require.True(t, isValid)
}

func TestE2E_SkylineTestnetBridge_SC_BLS_Verify_Hardcoded_Invalid(t *testing.T) {
	hardcodedTestCase, err := getHardcodedTestCase()
	require.NoError(t, err)

	t.Logf("\n\nOriginal: \n%s\n\n", hardcodedTestCase)

	t.Logf("Replacing all 'a' with 'b' from signature\n")

	hardcodedTestCase.signature = strings.ReplaceAll(hardcodedTestCase.signature, "a", "b")

	t.Logf("\n\nAltered: \n%s\n\n", hardcodedTestCase)

	isValid, err := callBLSVerifySC(rpcURL, contractAddrStr, hardcodedTestCase)

	require.NoError(t, err)
	require.False(t, isValid)
}

func TestE2E_SkylineTestnetBridge_SC_BLS_Verify_Valid(t *testing.T) {
	testCase, err := generateTestCase(randomValCnt())
	require.NoError(t, err)

	t.Logf("\n%s\n\n", testCase)

	data, err := testCase.ToSCData()
	require.NoError(t, err)

	isValid, err := callBLSVerifySC(rpcURL, contractAddrStr, data)

	require.NoError(t, err)
	require.True(t, isValid)
}

func TestE2E_SkylineTestnetBridge_SC_BLS_Verify_WrongSig(t *testing.T) {
	testCase, err := generateTestCase(randomValCnt())
	require.NoError(t, err)

	t.Logf("\n\nOriginal: \n%s\n\n", testCase)

	data, err := testCase.ToSCData()
	require.NoError(t, err)

	t.Logf("\n\nOriginal: \n%s\n\n", data)

	isValid, err := callBLSVerifySC(rpcURL, contractAddrStr, data)

	require.NoError(t, err)
	require.True(t, isValid)

	bytes, err := hex.DecodeString(data.signature)
	require.NoError(t, err)

	t.Logf("Setting first byte of the aggregated signature to ' '\n")

	bytes[0] = ' '
	data.signature = hex.EncodeToString(bytes)

	t.Logf("\n\nAltered: \n%s\n\n", data)

	isValid, err = callBLSVerifySC(rpcURL, contractAddrStr, data)

	require.NoError(t, err)
	require.False(t, isValid)
}

func TestE2E_SkylineTestnetBridge_SC_BLS_Verify_WrongDomain(t *testing.T) {
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

func TestE2E_SkylineTestnetBridge_SC_BLS_Verify_WrongMsg(t *testing.T) {
	testCase, err := generateTestCase(randomValCnt())
	require.NoError(t, err)

	t.Logf("\n\nOriginal: \n%s\n\n", testCase)

	data, err := testCase.ToSCData()
	require.NoError(t, err)

	isValid, err := callBLSVerifySC(rpcURL, contractAddrStr, data)

	require.NoError(t, err)
	require.True(t, isValid)

	t.Logf("Setting first byte of the message to ' '\n")

	testCase.message[0] = ' '

	t.Logf("\n\nAltered: \n%s\n\n", testCase)

	data, err = testCase.ToSCData()
	require.NoError(t, err)

	isValid, err = callBLSVerifySC(rpcURL, contractAddrStr, data)

	require.NoError(t, err)
	require.False(t, isValid)
}

func TestE2E_SkylineTestnetBridge_SC_BLS_Verify_WrongBitmap(t *testing.T) {
	valCnt := randomValCnt()
	testCase, err := generateTestCase(valCnt)
	require.NoError(t, err)

	t.Logf("\n\nOriginal: \n%s\n\n", testCase)

	data, err := testCase.ToSCData()
	require.NoError(t, err)

	isValid, err := callBLSVerifySC(rpcURL, contractAddrStr, data)

	require.NoError(t, err)
	require.True(t, isValid)

	data.bitmap = new(big.Int).Add(data.bitmap, big.NewInt(1))

	t.Logf("\nTrying bitmap: %v\n", data.bitmap)

	isValid, err = callBLSVerifySC(rpcURL, contractAddrStr, data)

	require.NoError(t, err)
	require.False(t, isValid)

	data.bitmap = new(big.Int).Sub(new(big.Int).Exp(big.NewInt(2), big.NewInt(int64(valCnt)), nil), big.NewInt(1))

	t.Logf("\nTrying bitmap: %v\n", data.bitmap)

	isValid, err = callBLSVerifySC(rpcURL, contractAddrStr, data)

	require.NoError(t, err)
	require.False(t, isValid)

	data.bitmap = big.NewInt(0)

	t.Logf("\nTrying bitmap: %v\n", data.bitmap)

	isValid, err = callBLSVerifySC(rpcURL, contractAddrStr, data)

	require.NoError(t, err)
	require.False(t, isValid)
}

func TestE2E_SkylineTestnetBridge_SC_BLS_Verify_ReplaceWithWrongValidator(t *testing.T) {
	testCase, err := generateTestCase(randomValCnt())
	require.NoError(t, err)

	t.Logf("\n\nOriginal: \n%s\n\n", testCase)

	data, err := testCase.ToSCData()
	require.NoError(t, err)

	t.Logf("\n\nOriginal: \n%s\n\n", data)

	isValid, err := callBLSVerifySC(rpcURL, contractAddrStr, data)

	require.NoError(t, err)
	require.True(t, isValid)

	key, err := bn256.GeneratePrivateKey()
	require.NoError(t, err)

	data.validators[0].Key = key.PublicKey().ToBigInt()

	t.Logf("Setting the first validator to a new value\n")

	t.Logf("\n\nAltered: \n%s\n\n", data)

	isValid, err = callBLSVerifySC(rpcURL, contractAddrStr, data)

	require.NoError(t, err)
	require.False(t, isValid)
}

func TestE2E_SkylineTestnetBridge_SC_BLS_Verify_WrongValidatorAddedToStart(t *testing.T) {
	testCase, err := generateTestCase(randomValCnt())
	require.NoError(t, err)

	t.Logf("\n\nOriginal: \n%s\n\n", testCase)

	data, err := testCase.ToSCData()
	require.NoError(t, err)

	t.Logf("\n\nOriginal: \n%s\n\n", data)

	isValid, err := callBLSVerifySC(rpcURL, contractAddrStr, data)

	require.NoError(t, err)
	require.True(t, isValid)

	key, err := bn256.GeneratePrivateKey()
	require.NoError(t, err)

	t.Logf("Adding a new validator to the start\n")

	data.validators = append([]validatorChainData{{Key: key.PublicKey().ToBigInt()}}, data.validators...)

	t.Logf("\n\nAltered: \n%s\n\n", data)

	isValid, err = callBLSVerifySC(rpcURL, contractAddrStr, data)

	require.NoError(t, err)
	require.False(t, isValid)
}

func TestE2E_SkylineTestnetBridge_SC_BLS_Verify_SomeValidatorsRemoved(t *testing.T) {
	testCase, err := generateTestCase(randomValCnt())
	require.NoError(t, err)

	t.Logf("\n\nOriginal: \n%s\n\n", testCase)

	data, err := testCase.ToSCData()
	require.NoError(t, err)

	t.Logf("\n\nOriginal: \n%s\n\n", data)

	isValid, err := callBLSVerifySC(rpcURL, contractAddrStr, data)

	require.NoError(t, err)
	require.True(t, isValid)

	t.Logf("Removing first 2 validators from the start\n")

	data.validators = data.validators[2:]

	t.Logf("\n\nAltered: \n%s\n\n", data)

	isValid, err = callBLSVerifySC(rpcURL, contractAddrStr, data)

	require.NoError(t, err)
	require.False(t, isValid)
}

func TestE2E_SkylineTestnetBridge_SC_BLS_Verify_ReplaceWithWrongSignature(t *testing.T) {
	testCase, err := generateTestCase(randomValCnt())
	require.NoError(t, err)

	t.Logf("\n\nOriginal: \n%s\n\n", testCase)

	data, err := testCase.ToSCData()
	require.NoError(t, err)

	isValid, err := callBLSVerifySC(rpcURL, contractAddrStr, data)

	require.NoError(t, err)
	require.True(t, isValid)

	key, err := bn256.GeneratePrivateKey()
	require.NoError(t, err)

	domain, err := Keccak256([]byte(testCase.domain))
	require.NoError(t, err)

	sig, err := key.Sign(testCase.message, domain)
	require.NoError(t, err)

	t.Logf("Setting the first signature to a new signature created by a new validator\n")

	testCase.signatures[0] = sig

	t.Logf("\n\nAltered: \n%s\n\n", testCase)

	data, err = testCase.ToSCData()
	require.NoError(t, err)

	isValid, err = callBLSVerifySC(rpcURL, contractAddrStr, data)

	require.NoError(t, err)
	require.False(t, isValid)
}

func TestE2E_SkylineTestnetBridge_SC_BLS_Verify_WrongSignatureAddedToStart(t *testing.T) {
	testCase, err := generateTestCase(randomValCnt())
	require.NoError(t, err)

	t.Logf("\n\nOriginal: \n%s\n\n", testCase)

	data, err := testCase.ToSCData()
	require.NoError(t, err)

	isValid, err := callBLSVerifySC(rpcURL, contractAddrStr, data)

	require.NoError(t, err)
	require.True(t, isValid)

	key, err := bn256.GeneratePrivateKey()
	require.NoError(t, err)

	domain, err := Keccak256([]byte(testCase.domain))
	require.NoError(t, err)

	sig, err := key.Sign(testCase.message, domain)
	require.NoError(t, err)

	t.Logf("Adding a new signature created by a new validator to the start\n")

	testCase.signatures = append(bn256.Signatures{sig}, testCase.signatures...)

	t.Logf("\n\nAltered: \n%s\n\n", testCase)

	data, err = testCase.ToSCData()
	require.NoError(t, err)

	isValid, err = callBLSVerifySC(rpcURL, contractAddrStr, data)

	require.NoError(t, err)
	require.False(t, isValid)
}

func TestE2E_SkylineTestnetBridge_SC_BLS_Verify_SomeSignaturesRemoved(t *testing.T) {
	testCase, err := generateTestCase(randomValCnt())
	require.NoError(t, err)

	t.Logf("\n\nOriginal: \n%s\n\n", testCase)

	data, err := testCase.ToSCData()
	require.NoError(t, err)

	isValid, err := callBLSVerifySC(rpcURL, contractAddrStr, data)

	require.NoError(t, err)
	require.True(t, isValid)

	t.Logf("Removing first 2 signatures from the start\n")

	testCase.signatures = testCase.signatures[2:]

	t.Logf("\n\nAltered: \n%s\n\n", testCase)

	data, err = testCase.ToSCData()
	require.NoError(t, err)

	isValid, err = callBLSVerifySC(rpcURL, contractAddrStr, data)

	require.NoError(t, err)
	require.False(t, isValid)
}

func TestE2E_SkylineTestnetBridge_SC_BLS_Verify_QuorumCheck_1(t *testing.T) {
	testCase, err := generateTestCase(randomValCnt())
	require.NoError(t, err)

	t.Logf("\n\nOriginal: \n%s\n\n", testCase)

	data, err := testCase.ToSCData()
	require.NoError(t, err)

	isValid, err := callBLSVerifySC(rpcURL, contractAddrStr, data)

	require.NoError(t, err)
	require.True(t, isValid)

	t.Logf("Removing the first signature from the start and setting the bitmap accordingly\n")

	testCase.signatures = testCase.signatures[1:]
	testCase.bitmap = new(big.Int).Sub(testCase.bitmap, big.NewInt(3))

	t.Logf("\n\nAltered: \n%s\n\n", testCase)

	data, err = testCase.ToSCData()
	require.NoError(t, err)

	isValid, err = callBLSVerifySC(rpcURL, contractAddrStr, data)

	require.NoError(t, err)
	require.False(t, isValid)
}

func TestE2E_SkylineTestnetBridge_SC_BLS_Verify_QuorumCheck_2(t *testing.T) {
	testCase, err := generateTestCase(randomValCnt())
	require.NoError(t, err)

	t.Logf("\n\nOriginal: \n%s\n\n", testCase)

	data, err := testCase.ToSCData()
	require.NoError(t, err)

	isValid, err := callBLSVerifySC(rpcURL, contractAddrStr, data)

	require.NoError(t, err)
	require.True(t, isValid)

	t.Logf("Replacing the signatures with just the first signature and setting the bitmap accordingly\n")

	testCase.signatures = []*bn256.Signature{testCase.signatures[0]}
	testCase.bitmap = big.NewInt(1)

	t.Logf("\n\nAltered: \n%s\n\n", testCase)

	data, err = testCase.ToSCData()
	require.NoError(t, err)

	isValid, err = callBLSVerifySC(rpcURL, contractAddrStr, data)

	require.NoError(t, err)
	require.False(t, isValid)
}

func generateTestCase(totalValCnt uint32) (*blsVerifyTestCase, error) {
	msg, err := Keccak256([]byte(randomString(20)))
	if err != nil {
		return nil, fmt.Errorf("failed to generate msg: %w", err)
	}

	domainStr := randomString(10)
	domain, err := Keccak256([]byte(domainStr))
	if err != nil {
		return nil, fmt.Errorf("failed to generate domain: %w", err)
	}

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

func Keccak256(v ...[]byte) ([]byte, error) {
	h := sha3.NewLegacyKeccak256()

	for _, i := range v {
		_, err := h.Write(i)
		if err != nil {
			return nil, err
		}
	}

	return h.Sum(nil), nil
}

const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

func randomString(length int) string {
	rand.New(rand.NewSource(time.Now().UnixNano()))

	b := make([]byte, length)

	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}

	return string(b)
}

func randomValCnt() uint32 {
	return rand.Uint32()%10 + 5
}
