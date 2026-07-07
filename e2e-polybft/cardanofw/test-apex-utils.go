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
	"sync"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/jsonrpc"
	infracommon "github.com/Ethernal-Tech/cardano-infrastructure/common"
	"github.com/Ethernal-Tech/cardano-infrastructure/wallet"
	"github.com/stretchr/testify/require"
)

const (
	ChainTypeCardano = iota
	ChainTypeEVM
	ChainTypeSolana

	BatchStateFailedToExecute           = "FailedToExecuteOnDestination"
	BatchStateIncludedInBatch           = "IncludedInBatch"
	BatchStateSubmittedToDestination    = "SubmittedToDestination"
	BatchStateExecuted                  = "ExecutedOnDestination"
	BridgingRequestStatusInvalidRequest = "InvalidRequest"

	ttlSlotNumberInc = 500
	maxInputs        = 40

	DefaultRequestStateTimeoutSec = 300

	DefaultTokenName = "test1"

	MintNFTTokenName = "custodial_nft_token"
	MintNFTAmount    = uint64(1)
)

var (
	defaultMinBridgingFeeAmount          = ApexToWei(big.NewInt(4))        // 4 Apex (4*10^18)
	MinUTxODefaultValue                  = ApexToWei(big.NewInt(1))        // 1 Apex  (1*10^18)
	PotentialFee                         = DfmToWei(big.NewInt(500_000))   // 0.5 Apex
	defaultMinBridgingFeeAmountForTokens = DfmToWei(big.NewInt(2_860_000)) // 2.86 Apex
	DefaultTokenMintAmount               = ApexToWei(big.NewInt(1_000))    // 1000 Apex (1000*10^18)
	DefaultMinOperationFee               = DfmToWei(big.NewInt(1_000_001)) // 1.000001 Apex (1.000001*10^18)

	// default min bridging fee
	defaultFeeAddrBridgingAmountEvm = map[ChainID]*big.Int{
		ChainIDPolygon:  DfmToWei(big.NewInt(170_000)),
		ChainIDEthereum: DfmToWei(big.NewInt(620)),
		ChainIDKatana:   DfmToWei(big.NewInt(80)),
		ChainIDSei:      DfmToWei(big.NewInt(80_000)),
		ChainIDArbitrum: DfmToWei(big.NewInt(30)),
		ChainIDScroll:   DfmToWei(big.NewInt(20)),
		ChainIDUnichain: big.NewInt(526_000_000_000),
	}

	defaultMinBridgingFeeAmountEvm = map[ChainID]*big.Int{
		ChainIDPolygon:  DfmToWei(big.NewInt(340_000)),
		ChainIDEthereum: DfmToWei(big.NewInt(1_240)),
		ChainIDKatana:   DfmToWei(big.NewInt(160)),
		ChainIDSei:      DfmToWei(big.NewInt(160_000)),
		ChainIDArbitrum: DfmToWei(big.NewInt(60)),
		ChainIDScroll:   DfmToWei(big.NewInt(40)),
		ChainIDUnichain: DfmToWei(big.NewInt(1)),
	}
)

type BatchTypes uint8

const (
	BatchTypeNormal BatchTypes = iota
	BatchTypeConsolidation
	BatchTypeValidatorSet
	BatchTypeValidatorSetFinal
)

func ResolveCardanoCliBinary(chainID ChainID) string {
	env, name := "CARDANO_CLI_BINARY", "cardano-cli"
	if chainID == ChainIDCardano {
		env, name = "CARDANO_CLI_11_BINARY", "cardano-cli-11"
	}

	return tryResolveFromEnv(env, name)
}

func ResolveOgmiosBinary(chainID ChainID) string {
	env, name := "OGMIOS", "ogmios"

	if chainID == ChainIDCardano {
		env, name = "OGMIOS_11_BINARY", "ogmios-11"
	}

	return tryResolveFromEnv(env, name)
}

func ResolveCardanoNodeBinary(chainID ChainID) string {
	env, name := "CARDANO_NODE_BINARY", "cardano-node"
	if chainID == ChainIDCardano {
		env, name = "CARDANO_NODE_11_BINARY", "cardano-node-11"
	}

	return tryResolveFromEnv(env, name)
}

func ResolveApexBridgeBinary() string {
	return tryResolveFromEnv("APEX_BRIDGE_BINARY", "apex-bridge")
}

func ResolveBladeBinary() string {
	return tryResolveFromEnv("BLADE_BINARY", "blade")
}

func ResolveSPLTokenBinary() string {
	return tryResolveFromEnv("SPL_TOKEN_BINARY", "spl-token")
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
	str string, chainID ChainID, networkID wallet.CardanoNetworkType, networkMagic uint,
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

	cliUtils := wallet.NewCliUtils(ResolveCardanoCliBinary(chainID))

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
		currentState  *BridgingRequestStateResponse
	)

	fmt.Printf("requestURL: %s\n", requestURL)

	timeoutTimer := time.NewTimer(timeout)
	defer timeoutTimer.Stop()

	for {
		select {
		case <-timeoutTimer.C:
			fmt.Printf("Timeout\n")

			return fmt.Errorf("timeout. last err: %w", err)
		case <-ctx.Done():
			return errors.New("context done")
		case <-time.After(time.Second * 3):
		}

		currentState, err = GetBridgingRequestState(ctx, requestURL, apiKey)
		if err != nil {
			if !strings.Contains(err.Error(), fmt.Sprintf("%d", http.StatusNotFound)) {
				fmt.Printf("error while GetBridgingRequestState. err: %v\n", err)
			}

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

func SetOrDefault[T comparable](val, def T) T {
	var zero T

	if val == zero {
		return def
	}

	return val
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
	case ChainIDCardano:
		return 4
	case ChainIDPolygon:
		return 5
	case ChainIDEthereum:
		return 6
	case ChainIDKatana:
		return 7
	case ChainIDSei:
		return 8
	case ChainIDArbitrum:
		return 9
	case ChainIDScroll:
		return 10
	case ChainIDUnichain:
		return 11
	default:
		return 0
	}
}

func populateEvmAndSolTokenBalances(
	ctx context.Context,
	apex *ApexSystem,
	user *TestApexUser,
	chain ChainID,
	addr string,
	balance map[string]*big.Int,
) (map[string]*big.Int, []error) {
	if balance == nil {
		balance = make(map[string]*big.Int)
	}

	var errs []error

	getTokenBalance := func(name string) {
		byToken, err := apex.GetBalanceWithTokenName(ctx, user, chain, name)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to get balance for (%s, %s): %w", chain, addr, err))

			return
		}

		if byToken != nil {
			if v := byToken[name]; v != nil {
				balance[name] = v
			}
		}
	}

	if chain == ChainIDSolana {
		for _, token := range apex.SolanaInfo.Tokens {
			name := token.ChainSpecific
			if name == wallet.AdaTokenName {
				continue
			}

			getTokenBalance(name)
		}

		return balance, errs
	}

	for _, token := range apex.GetEvmInfo(chain).Tokens {
		name := token.ChainSpecific
		if name == wallet.AdaTokenName {
			continue
		}

		getTokenBalance(name)
	}

	return balance, errs
}

// GetUsersBalances returns a map keyed by chain and then by address.
// Keying by chain first is required because EVM chains share the same address for a given user
func GetUsersBalances(
	ctx context.Context, apex *ApexSystem, chains []ChainID, users []*TestApexUser,
) (map[ChainID]map[string]map[string]*big.Int, error) {
	var (
		balances = make(map[ChainID]map[string]map[string]*big.Int, len(chains))
		wg       sync.WaitGroup
		mu       sync.Mutex
		errs     []error
	)

	for _, chain := range chains {
		balances[chain] = make(map[string]map[string]*big.Int, len(users)+1)
	}

	baseUsers := []*TestApexUser(nil)
	if apex.FunderUser != nil {
		baseUsers = append(baseUsers, apex.FunderUser)
	}

	for _, user := range append(baseUsers, users...) {
		for _, chain := range chains {
			wg.Add(1)

			go func(user *TestApexUser, chain string, addr string) {
				defer wg.Done()

				balance, err := infracommon.ExecuteWithRetry(
					ctx, func(ctx context.Context) (map[string]*big.Int, error) {
						return apex.GetBalance(ctx, user, chain)
					},
				)

				var tokenErrs []error
				if err == nil && (IsEVMChain(chain) || chain == ChainIDSolana) {
					balance, tokenErrs = populateEvmAndSolTokenBalances(ctx, apex, user, chain, addr, balance)
				}

				mu.Lock()
				defer mu.Unlock()

				if err != nil {
					errs = append(errs, fmt.Errorf("failed to get balance for (%s, %s): %w", chain, addr, err))
				} else {
					errs = append(errs, tokenErrs...)
					balances[chain][addr] = balance
				}
			}(user, chain, user.GetAddress(chain))
		}
	}

	wg.Wait()

	return balances, errors.Join(errs...)
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

	pid, err := wallet.NewCliUtils(ResolveCardanoCliBinary(chainID)).GetPolicyID(policyScript)
	if err != nil {
		return wallet.Token{}, nil, err
	}

	return wallet.NewToken(pid, tokenName), policyScript, nil
}

func FundUserWithToken(
	ctx context.Context, apex *ApexSystem, chainID ChainID,
	minterWallet *wallet.Wallet, userToFund *TestApexUser,
	tokenName string, mintAmount *big.Int,
	fundAmount *big.Int, tokenFundAmount *big.Int,
) (*GenericTokenAmount, error) {
	chain, err := apex.getChain(chainID)
	if err != nil {
		return nil, err
	}

	cardanoChain, ok := chain.(*TestCardanoChain)
	if !ok {
		return nil, fmt.Errorf("failed to cast the chain to cardano chain")
	}

	return FundAddressWithToken(
		ctx, cardanoChain, minterWallet, userToFund.GetAddress(chain.ChainID()),
		tokenName, mintAmount, fundAmount, tokenFundAmount)
}

func FundAddressWithToken(
	ctx context.Context, chain *TestCardanoChain,
	minterWallet *wallet.Wallet, addrToFund string,
	tokenName string, mintAmount *big.Int,
	weiFundAmount *big.Int, tokenFundAmount *big.Int,
) (*GenericTokenAmount, error) {
	if weiFundAmount == nil || weiFundAmount.Sign() <= 0 {
		return nil, fmt.Errorf("wei amount must be greater than zero")
	}

	if mintAmount != nil && mintAmount.Sign() > 0 {
		if err := MintToken(chain, minterWallet, tokenName, WeiToDfm(mintAmount)); err != nil {
			return nil, err
		}
	}

	token, _, err := GetTokenAndPolicyForVerificationKey(
		chain.ChainID(), chain.config.NetworkType, minterWallet.VerificationKey, tokenName)
	if err != nil {
		return nil, err
	}

	tokenAmount := NewGenericTokenAmount(token, tokenFundAmount)

	minterAddr, err := GetAddress(chain.config.NetworkType, minterWallet)
	if err != nil {
		return nil, err
	}

	if minterAddr.String() == addrToFund {
		return &tokenAmount, nil
	}

	return FundAddressesWithToken(
		ctx, chain, minterWallet, []string{addrToFund}, tokenName, weiFundAmount, tokenFundAmount)
}

func MintToken(
	chain *TestCardanoChain, minterWallet *wallet.Wallet, tokenName string, mintDfmAmount *big.Int,
) error {
	args := []string{
		"bridge-admin", "mint-native-token",
		"--key", hex.EncodeToString(minterWallet.SigningKey),
		"--ogmios", chain.ogmiosURL,
		"--network-id", fmt.Sprintf("%v", chain.config.NetworkType),
		"--testnet-magic", fmt.Sprintf("%v", chain.config.NetworkMagic),
		"--token-name", tokenName,
		"--amount", mintDfmAmount.String(),
		"--cardano-cli-binary-name", ResolveCardanoCliBinary(chain.ChainID()),
	}

	if len(minterWallet.StakeSigningKey) > 0 {
		args = append(args, "--stake-key", hex.EncodeToString(minterWallet.StakeSigningKey))
	}

	return RunCommand(ResolveApexBridgeBinary(), args, os.Stdout)
}

func FundUsersWithToken(
	ctx context.Context, chain *TestCardanoChain,
	sender *wallet.Wallet, users []*TestApexUser,
	tokenName string, fundAmount *big.Int, tokenFundAmount *big.Int,
) (*GenericTokenAmount, error) {
	addrs := make([]string, len(users))

	for i, u := range users {
		addrs[i] = u.GetAddress(chain.ChainID())
	}

	return FundAddressesWithToken(
		ctx, chain, sender, addrs, tokenName, fundAmount, tokenFundAmount)
}

func FundAddressesWithToken(
	ctx context.Context, chain *TestCardanoChain,
	sender *wallet.Wallet, addrs []string,
	tokenName string, fundAmount *big.Int, tokenFundAmount *big.Int,
) (*GenericTokenAmount, error) {
	token, _, err := GetTokenAndPolicyForVerificationKey(
		chain.ChainID(), chain.config.NetworkType, sender.VerificationKey, tokenName)
	if err != nil {
		return nil, err
	}

	tokenAmount := NewGenericTokenAmount(token, tokenFundAmount)
	privateKey := ToCardanoPrivateKeyString(sender.SigningKey, sender.StakeSigningKey)
	receivers := make([]GenericTxReceiver, len(addrs))

	for i, addr := range addrs {
		receivers[i] = GenericTxReceiver{
			Addr:   addr,
			Amount: fundAmount,
			NativeTokens: []GenericTokenAmount{
				tokenAmount,
			},
		}
	}

	txHash, err := chain.SendTx(ctx, privateKey, nil, receivers, 0)
	if err != nil {
		return nil, err
	}

	fmt.Printf("Funded %s with lovelace: %d, native tokens: %s. txHash: %s\n",
		addrs, fundAmount, tokenAmount, txHash)

	return &tokenAmount, nil
}

func IsEVMChain(chain ChainID) bool {
	return chain == ChainIDNexus || chain == ChainIDPolygon || chain == ChainIDEthereum ||
		chain == ChainIDKatana || chain == ChainIDSei || chain == ChainIDArbitrum ||
		chain == ChainIDScroll || chain == ChainIDUnichain
}

func IsCardanoChain(chain ChainID) bool {
	return chain == ChainIDPrime || chain == ChainIDVector || chain == ChainIDCardano
}

func isExitCode(err error, code int) bool {
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		return exitError.ExitCode() == code
	}

	return false
}
