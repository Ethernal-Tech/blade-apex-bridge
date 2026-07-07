package cardanofw

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/jsonrpc"
	"github.com/Ethernal-Tech/cardano-infrastructure/wallet"
	"github.com/stretchr/testify/require"
)

const (
	ChainTypeCardano = iota
	ChainTypeEVM

	BatchStateFailedToExecute           = "FailedToExecuteOnDestination"
	BatchStateIncludedInBatch           = "IncludedInBatch"
	BatchStateSubmittedToDestination    = "SubmittedToDestination"
	BatchStateExecuted                  = "ExecutedOnDestination"
	BridgingRequestStatusInvalidRequest = "InvalidRequest"

	MinUTxODefaultValue         = uint64(1_000_000)
	defaultMinBridgingFeeAmount = uint64(1_000_010)

	DefaultRequestStateTimeoutSec = 300

	PotentialFee     = 500_000
	ttlSlotNumberInc = 500

	DefaultTokenName       = "test1"
	DefaultTokenMintAmount = uint64(1_000_000_000)

	splitStringLength = 40
)

func ResolveCardanoCliBinary(networkID wallet.CardanoNetworkType) string {
	env, name := "CARDANO_CLI_BINARY", "cardano-cli"

	return tryResolveFromEnv(env, name)
}

func ResolveOgmiosBinary(networkID wallet.CardanoNetworkType) string {
	env, name := "OGMIOS", "ogmios"

	return tryResolveFromEnv(env, name)
}

func ResolveCardanoNodeBinary(networkID wallet.CardanoNetworkType) string {
	env, name := "CARDANO_NODE_BINARY", "cardano-node"

	return tryResolveFromEnv(env, name)
}

func ResolveApexBridgeBinary() string {
	return tryResolveFromEnv("APEX_BRIDGE_BINARY", "apex-bridge")
}

func RunCommandContext(
	ctx context.Context, binary string, args []string, stdout io.Writer, envVariables ...string,
) error {
	cmd := exec.CommandContext(ctx, binary, args...)

	return runCommand(cmd, stdout, envVariables...)
}

// runCommand executes command with given arguments
func RunCommand(binary string, args []string, stdout io.Writer, envVariables ...string) error {
	cmd := exec.Command(binary, args...)

	return runCommand(cmd, stdout, envVariables...)
}

func runCommand(cmd *exec.Cmd, stdout io.Writer, envVariables ...string) error {
	var stdErr bytes.Buffer

	cmd.Stderr = &stdErr
	cmd.Stdout = stdout

	cmd.Env = append(os.Environ(), envVariables...)

	if err := cmd.Run(); err != nil {
		if stdErr.Len() > 0 {
			return fmt.Errorf("failed to execute command: %s", stdErr.String())
		}

		return fmt.Errorf("failed to execute command: %w", err)
	}

	if stdErr.Len() > 0 {
		return fmt.Errorf("error during command execution: %s", stdErr.String())
	}

	return nil
}

func LoadJSON[TReturn any](path string) (*TReturn, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open %v. error: %w", path, err)
	}

	defer f.Close()

	var value TReturn

	if err = json.NewDecoder(f).Decode(&value); err != nil {
		return nil, fmt.Errorf("failed to decode %v. error: %w", path, err)
	}

	return &value, nil
}

// SplitString splits large string into slice of substrings
func SplitString(s string, mxlen int) (res []string) {
	for i := 0; i < len(s); i += mxlen {
		end := i + mxlen
		if end > len(s) {
			end = len(s)
		}

		res = append(res, s[i:end])
	}

	return res
}

func ToCardanoPrivateKeyString(paymentKey, stakeKey []byte) string {
	paymentSK := hex.EncodeToString(paymentKey)
	if len(stakeKey) == 0 {
		return paymentSK
	}

	return fmt.Sprintf("%s_%s", paymentSK, hex.EncodeToString(stakeKey))
}

func FromCardanoPrivateKeyString(
	str string, networkID wallet.CardanoNetworkType, networkMagic uint,
) (wallets []*wallet.Wallet, policyScript *wallet.PolicyScript, addr string, err error) {
	if !strings.HasPrefix(str, "ps") {
		parts := strings.Split(str, "_")

		paymentKey, err := hex.DecodeString(parts[0])
		if err != nil {
			return nil, nil, "", err
		}

		var stakeKey []byte

		if len(parts) > 1 && len(parts[1]) > 0 {
			stakeKey, err = hex.DecodeString(parts[1])
			if err != nil {
				return nil, nil, "", err
			}
		}

		wallets = []*wallet.Wallet{wallet.NewWallet(paymentKey, stakeKey)}

		walletAddress, err := GetAddress(networkID, wallets[0])
		if err != nil {
			return nil, nil, "", err
		}

		return wallets, nil, walletAddress.String(), nil
	}

	parts := strings.Split(str[2:], "_")
	if len(parts) < 2 {
		return nil, nil, "", fmt.Errorf("invalid nuber of parts: %d", len(parts))
	}

	psBytes, err := hex.DecodeString(parts[0])
	if err != nil {
		return nil, nil, "", err
	}

	if err := json.Unmarshal(psBytes, &policyScript); err != nil {
		return nil, nil, "", err
	}

	wallets = make([]*wallet.Wallet, len(parts)-1)

	for i, keyHex := range parts[1:] {
		paymentKey, err := hex.DecodeString(keyHex)
		if err != nil {
			return nil, nil, "", err
		}

		wallets[i] = wallet.NewWallet(paymentKey, nil)
	}

	cliUtils := wallet.NewCliUtils(ResolveCardanoCliBinary(networkID))

	walletAddress, err := cliUtils.GetPolicyScriptEnterpriseAddress(networkMagic, policyScript)
	if err != nil {
		return nil, nil, "", err
	}

	return wallets, policyScript, walletAddress, nil
}

func JSONRPCClient(jsonRPCAddr string) (*jsonrpc.EthClient, error) {
	clt, err := jsonrpc.NewEthClient(jsonRPCAddr)
	if err != nil {
		return nil, err
	}

	return clt, nil
}

func GetBridgingRequestState(ctx context.Context, requestURL string, apiKey string) (
	*BridgingRequestStateResponse, error,
) {
	return GetAPIRequestGeneric[*BridgingRequestStateResponse](ctx, requestURL, apiKey)
}

func GetOracleState(ctx context.Context, requestURL string, apiKey string) (
	*OracleStateResponse, error,
) {
	return GetAPIRequestGeneric[*OracleStateResponse](ctx, requestURL, apiKey)
}

func GetAPIRequestGeneric[T any](ctx context.Context, requestURL string, apiKey string) (t T, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return t, err
	}

	req.Header.Set("X-API-KEY", apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return t, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return t, fmt.Errorf("http status for %s code is %d", requestURL, resp.StatusCode)
	}

	resBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return t, err
	}

	var responseModel T

	err = json.Unmarshal(resBody, &responseModel)
	if err != nil {
		return t, err
	}

	return responseModel, nil
}

type FaucetRequestBody struct {
	Addr  string `json:"address"`
	Token string `json:"token"`
}

func FaucetRequest(ctx context.Context, addr string) (err error) {
	requestURL := "https://developers.apexfusion.org/api/faucet"

	requestBody := FaucetRequestBody{
		Addr:  addr,
		Token: os.Getenv("TESTNET_FAUCET_API_KEY"),
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return err
	}

	body := bytes.NewBuffer(bodyBytes)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL, body)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("http status for %s code is %d", requestURL, resp.StatusCode)
	}

	return nil
}

type BridgingRequestStateResponse struct {
	SourceChainID      string `json:"sourceChainId"`
	SourceTxHash       string `json:"sourceTxHash"`
	DestinationChainID string `json:"destinationChainId"`
	Status             string `json:"status"`
	DestinationTxHash  string `json:"destinationTxHash"`
}

type CardanoChainConfigUtxo struct {
	Hash    [32]byte `json:"id"`
	Index   uint32   `json:"index"`
	Address string   `json:"address"`
	Amount  uint64   `json:"amount"`
	Slot    uint64   `json:"slot"`
}

type OracleStateResponse struct {
	ChainID   string                   `json:"chainID"`
	Utxos     []CardanoChainConfigUtxo `json:"utxos"`
	BlockSlot uint64                   `json:"slot"`
	BlockHash string                   `json:"hash"`
}

func GetAddress(networkType wallet.CardanoNetworkType, cardanoWallet *wallet.Wallet) (*wallet.CardanoAddress, error) {
	if len(cardanoWallet.StakeVerificationKey) > 0 {
		return wallet.NewBaseAddress(networkType,
			cardanoWallet.VerificationKey, cardanoWallet.StakeVerificationKey)
	}

	return wallet.NewEnterpriseAddress(networkType, cardanoWallet.VerificationKey)
}

func GetTestNetMagicArgs(testnetMagic uint) []string {
	if testnetMagic == 0 || testnetMagic == wallet.MainNetProtocolMagic {
		return []string{"--mainnet"}
	}

	return []string{"--testnet-magic", strconv.FormatUint(uint64(testnetMagic), 10)}
}

type BridgingRequestMetadataTransaction struct {
	Address []string `cbor:"a" json:"a"`
	Amount  uint64   `cbor:"m" json:"m"`
}

func CreateCardanoBridgingMetaData(
	sender string, receivers map[string]uint64, destinationChain ChainID, feeAmount uint64,
) ([]byte, error) {
	var transactions = make([]BridgingRequestMetadataTransaction, 0, len(receivers))
	for addr, amount := range receivers {
		transactions = append(transactions, BridgingRequestMetadataTransaction{
			Address: AddrToMetaDataAddr(addr),
			Amount:  amount,
		})
	}

	metadata := map[string]interface{}{
		"1": map[string]interface{}{
			"t":  "bridge",
			"d":  destinationChain,
			"s":  AddrToMetaDataAddr(sender),
			"tx": transactions,
			"fa": feeAmount,
		},
	}

	return json.Marshal(metadata)
}

func tryResolveFromEnv(env, name string) string {
	if bin := os.Getenv(env); bin != "" {
		return bin
	}
	// fallback
	return name
}

func GetLogsFile(t *testing.T, filePath string, withStdout bool) io.Writer {
	t.Helper()

	var writers []io.Writer

	f, err := os.OpenFile(filePath, os.O_RDWR|os.O_APPEND|os.O_CREATE, 0600)
	if err != nil {
		t.Log("failed to create log file", "err", err, "file", filePath)
	} else {
		writers = append(writers, f)

		t.Cleanup(func() {
			if err := f.Close(); err != nil {
				t.Log("GetStdout close file error", "err", err)
			}
		})
	}

	if withStdout {
		writers = append(writers, os.Stdout)
	}

	if len(writers) == 0 {
		return io.Discard
	}

	return io.MultiWriter(writers...)
}

func IsEnvVarTrue(name string) bool {
	return strings.ToLower(os.Getenv(name)) == "true"
}

func ShouldSkipE2RRedundantTests() bool {
	return IsEnvVarTrue("SKIP_E2E_REDUNDANT_TESTS")
}

func WaitForRequestStateGeneric(
	ctx context.Context, apex *ApexSystem, chainID string, txHash string,
	apiKey string, timeout time.Duration, handler func(status string) bool,
) error {
	apiURL, err := apex.GetBridgingAPI()
	if err != nil {
		return err
	}

	var (
		requestURL = fmt.Sprintf(
			"%s/api/BridgingRequestState/Get?chainId=%s&txHash=%s", apiURL, chainID, txHash)
		currentStatus string
	)

	timeoutTimer := time.NewTimer(timeout)
	defer timeoutTimer.Stop()

	for {
		select {
		case <-timeoutTimer.C:
			fmt.Printf("Timeout\n")

			return errors.New("timeout")
		case <-ctx.Done():
			return errors.New("context done")
		case <-time.After(time.Millisecond * 500):
		}

		currentState, err := GetBridgingRequestState(ctx, requestURL, apiKey)
		if err != nil {
			continue
		}

		if currentStatus != currentState.Status {
			currentStatus = currentState.Status
			fmt.Printf("currentStatus = %s\n", currentStatus)

			if finished := handler(currentStatus); finished {
				return nil
			}
		}
	}
}

func WaitForBatchState(
	ctx context.Context, apex *ApexSystem, chainID string, txHash string,
	apiKey string, breakIfFailed bool, failAtLeastOnce bool, batchState string, otherGoodBatchStates ...string,
) (int, bool) {
	failedToExecuteCount := 0
	err := WaitForRequestStateGeneric(ctx, apex, chainID, txHash, apiKey, time.Second*400, func(status string) bool {
		if status == BatchStateFailedToExecute {
			failedToExecuteCount++

			if breakIfFailed {
				return true
			}
		}

		found := status == batchState
		if !found {
			for _, otherState := range otherGoodBatchStates {
				if status == otherState {
					found = true

					break
				}
			}
		}

		return found && (!failAtLeastOnce || failedToExecuteCount > 0)
	})

	return failedToExecuteCount, err != nil
}

func WaitForRequestStates(
	ctx context.Context, apex *ApexSystem, chainID string, txHash string,
	apiKey string, expectedStates []string, timeoutSec uint,
) (string, error) {
	selectedState := ""
	timeoutTime := time.Duration(timeoutSec) * time.Second
	err := WaitForRequestStateGeneric(ctx, apex, chainID, txHash, apiKey, timeoutTime, func(status string) bool {
		if len(expectedStates) == 0 {
			selectedState = status

			return true
		}

		for _, expectedState := range expectedStates {
			if strings.Compare(status, expectedState) == 0 {
				selectedState = expectedState

				return true
			}
		}

		return false
	})

	return selectedState, err
}

func WaitForInvalidState(
	t *testing.T, ctx context.Context, apex *ApexSystem, chainID string, txHash string, apiKey string, timeoutSec uint) {
	t.Helper()

	if timeoutSec == 0 {
		timeoutSec = DefaultRequestStateTimeoutSec
	}

	state, err := WaitForRequestStates(
		ctx, apex, chainID, txHash, apiKey, []string{BridgingRequestStatusInvalidRequest}, timeoutSec)
	require.NoError(t, err)
	require.Equal(t, BridgingRequestStatusInvalidRequest, state)
}

func SplitAmountNTimes(totalAmount *big.Int, cnt int) (*big.Int, *big.Int) {
	amount := new(big.Int).Div(totalAmount, big.NewInt(int64(cnt)))
	amountWithChange := new(big.Int).Sub(totalAmount, new(big.Int).Mul(amount, big.NewInt(int64(cnt-1))))

	return amount, amountWithChange
}

func ChainIDToInt(chainID string) uint8 {
	switch chainID {
	case ChainIDPrime:
		return 1
	case ChainIDVector:
		return 2
	case ChainIDNexus:
		return 3
	default:
		return 0
	}
}

func GetTokenAndPolicyForVerificationKey(
	chainID ChainID, networkType wallet.CardanoNetworkType, verificationKey []byte, tokenName string,
) (wallet.Token, *wallet.PolicyScript, error) {
	keyHash, err := wallet.GetKeyHash(verificationKey)
	if err != nil {
		return wallet.Token{}, nil, err
	}

	policyScript := &wallet.PolicyScript{
		Type:    wallet.PolicyScriptSigType,
		KeyHash: keyHash,
	}

	pid, err := wallet.NewCliUtils(wallet.ResolveCardanoCliBinary(networkType)).GetPolicyID(policyScript)
	if err != nil {
		return wallet.Token{}, nil, err
	}

	return wallet.NewToken(pid, tokenName), policyScript, nil
}

func AddrToMetaDataAddr(addr string) []string {
	addr = strings.TrimPrefix(strings.TrimPrefix(addr, "0x"), "0X")

	return SplitString(addr, splitStringLength)
}

func isExitCode(err error, code int) bool {
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		return exitError.ExitCode() == code
	}

	return false
}

func GetGenesisWalletFromCluster(
	dirPath string,
	keyID uint,
) (*wallet.Wallet, error) {
	keyFileName := strings.Join([]string{"utxo", fmt.Sprint(keyID)}, "")

	sKey, err := wallet.NewKey(filepath.Join(dirPath, "utxo-keys", fmt.Sprintf("%s.skey", keyFileName)))
	if err != nil {
		return nil, err
	}

	sKeyBytes, err := sKey.GetKeyBytes()
	if err != nil {
		return nil, err
	}

	return wallet.NewWallet(sKeyBytes, nil), nil
}
