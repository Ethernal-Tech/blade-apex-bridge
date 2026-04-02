package cardanofw

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/e2eindexer"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/solanafw"
	carsendtx "github.com/Ethernal-Tech/cardano-infrastructure/sendtx"
	carwallet "github.com/Ethernal-Tech/cardano-infrastructure/wallet"
	solsendtx "github.com/Ethernal-Tech/solana-infrastructure/sendtx"
	solanawallet "github.com/Ethernal-Tech/solana-infrastructure/wallet"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	"github.com/stretchr/testify/require"
)

// WSOL (Wrapped SOL) mint address on Solana. 9 decimals.
var wsolMint = solana.MustPublicKeyFromBase58(WSOLMintAddress)

const (
	solanaProgramDir         = "skyline-solana-programs"
	solanaProgramBuildPath   = "program_build/skyline_program.so"
	solanaProgramKeypairPath = "program_build/skyline_program-keypair.json"

	TreasuryAddress = "AXXWYCH6PNm6AGjaasPG1maarfQvRedSw18wj91Nem1F"

	MaxConfirmationWaitTime = 2 * time.Minute
)

type TestSolanaChainConfig struct {
	ChainID   string
	IsEnabled bool

	InitialHotWalletAmount *big.Int
	FundAmount             *big.Int
	PreminesAddresses      []string
	StartingPort           int

	MinBridgingFee         *big.Int
	MinBridgingAmount      *big.Int
	MinTokenBridgingAmount *big.Int
	MinOperationFee        *big.Int
	CurrencyID             uint16

	TreasuryAddress solana.PublicKey

	TokensMint map[uint16]string
}

func NewSolanaChainConfig(enabled bool) *TestSolanaChainConfig {
	return &TestSolanaChainConfig{
		ChainID:                ChainIDSolana,
		IsEnabled:              enabled,
		StartingPort:           8899,
		InitialHotWalletAmount: SolanaToWei(big.NewInt(1000)),
		FundAmount:             LamportToWei(SolanaToLamport(big.NewInt(100000))),
		MinBridgingFee:         SolanaToLamport(big.NewInt(1)), // 1 SOL
		MinBridgingAmount:      big.NewInt(1_000_000_000),      // 1.000000 SOL
		MinTokenBridgingAmount: big.NewInt(1_000_000_000),      // 1.000000 SOL
		MinOperationFee:        big.NewInt(500000000),          // 0.5 SOL
		CurrencyID:             WSOLTokenID,
		TreasuryAddress:        solana.MustPublicKeyFromBase58(TreasuryAddress),
		TokensMint: map[uint16]string{
			WSOLTokenID: WSOLMintAddress, // by default add wSOL to the tokens mint map
		},
	}
}

type TestSolanaChain struct {
	config           *TestSolanaChainConfig
	relayerAddr      string
	validatorPubKeys []string
	cluster          *solanafw.TestSolanaCluster
	admin            *solanawallet.Wallet
	jsonRPCAddr      string
	gatewayAddr      string
	indexer          e2eindexer.TxsExecutedComponent
	programID        string
}

var _ ITestApexChain = (*TestSolanaChain)(nil)

func NewTestSolanaChain(config *TestSolanaChainConfig) (ITestApexChain, error) {
	if !config.IsEnabled {
		getFlag := func(suffix string) string {
			return fmt.Sprintf("--%s-%s", config.ChainID, suffix)
		}

		return NewTestApexChainDummy([]string{
			getFlag("node-url"), "http://localhost:5500",
		}), nil
	}

	// Generate a new admin wallet
	admin, err := solanawallet.NewWallet()
	if err != nil {
		return nil, err
	}

	return &TestSolanaChain{
		config:  config,
		admin:   admin,
		indexer: e2eindexer.NewTxsExecutedComponentDummy(),
	}, nil
}

func (sc *TestSolanaChain) GetTxProvider() (*solanawallet.Provider, error) {
	return solanawallet.NewProvider(sc.jsonRPCAddr)
}

func (sc *TestSolanaChain) GetTreasuryAddress() string {
	return sc.config.TreasuryAddress.String()
}

// wTODO: Implement this for sending bridging requests to the solana chain
func (sc *TestSolanaChain) BridgingRequest(params BridgingRequestParams) (string, error) {
	fmt.Println("bridging request: ", params)

	txProvider, err := sc.GetTxProvider()
	if err != nil {
		return "", err
	}

	relayerAddr, err := solanawallet.PublicKeyFromAddress(sc.relayerAddr)
	if err != nil {
		return "", err
	}

	txSender := solsendtx.NewTxSender(txProvider, &solsendtx.ChainConfig{
		TreasuryAddress:    sc.config.TreasuryAddress,
		BridgingFeeAddress: relayerAddr,
	})

	txReceivers := make([]solsendtx.BridgingTxReceiver, 0, len(params.Receivers))
	for addr, amount := range params.Receivers {
		txReceivers = append(txReceivers, solsendtx.BridgingTxReceiver{
			Address: addr,
			TokenAmount: solanawallet.TokenAmount{
				TokenMint: sc.config.TokensMint[amount.TokenID],
				Amount:    WeiToLamport(amount.Amount),
			},
		})
	}

	senderWallet, err := solanawallet.NewWalletFromPrivateKey(params.PrivateKey)
	if err != nil {
		return "", err
	}

	txDto := solsendtx.BridgeRequestDto{
		DstChainID:   params.DestChainID,
		SenderAddr:   senderWallet.PublicKey.String(),
		Receivers:    txReceivers,
		BridgingFee:  params.FeeAmount.Uint64(),
		OperationFee: params.OperationFee.Uint64(),
	}

	fmt.Println("txDto: ", txDto)

	recentBlockhash, err := txProvider.GetLatestBlockhash(params.Ctx)
	if err != nil {
		return "", err
	}

	tx, err := txSender.CreateTx(
		params.Ctx, senderWallet.PublicKey,
		solsendtx.InstructionTypeBridgingRequest,
		recentBlockhash,
		txDto,
	)
	if err != nil {
		return "", err
	}

	_, err = tx.Sign(func(key solana.PublicKey) *solana.PrivateKey {
		return &senderWallet.PrivateKey
	})
	if err != nil {
		return "", fmt.Errorf("sign instruction: %w", err)
	}

	sig, err := txSender.SendTx(params.Ctx, tx)
	if err != nil {
		return "", err
	}

	if err := txProvider.WaitForSignature(params.Ctx, *sig, rpc.CommitmentConfirmed, MaxConfirmationWaitTime); err != nil {
		return "", fmt.Errorf("wait for bridging request confirmation: %w", err)
	}

	return sig.String(), nil
}

// ChainID implements ITestApexChain.
func (sc *TestSolanaChain) ChainID() string {
	return sc.config.ChainID
}

func (sc *TestSolanaChain) CreateAddresses(bladeAdmin *crypto.ECDSAKey, bridgeURL string, chainIDsConfig string) error {
	return nil
}

func (sc *TestSolanaChain) CreateMetadata(senderAddr string,
	dstChainID string, receivers []carsendtx.BridgingTxReceiver,
	bridgingFee *big.Int, operationFee *big.Int) ([]byte, error) {
	return nil, nil
}

func (sc *TestSolanaChain) CreateWallets(validator *TestApexValidator) error {
	var (
		err error
	)

	if RunRelayerOnValidatorID == validator.ID {
		sc.relayerAddr, err = validator.RelayerWalletCreate(sc.ChainID())
		if err != nil {
			return err
		}
	}

	validatorPubKey, err := validator.SolanaWalletCreate(sc.ChainID())
	if err != nil {
		return err
	}

	sc.validatorPubKeys = append(sc.validatorPubKeys, validatorPubKey)

	return nil
}

func (sc *TestSolanaChain) DeployMintingContract(ctx context.Context, chainIDsConfig string) error {
	// Program must be deployed here since during InitContracts the wallets are not yet funded
	// 1. Save admin private key to temp file as JSON array of uint8 (e.g. [38,32,44,...])
	adminPkFile, err := os.CreateTemp(os.TempDir(), "admin-pk-*.json")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}

	defer adminPkFile.Close()

	pkString := fmt.Sprintf("%v", []byte(sc.admin.PrivateKey))
	pkString = strings.ReplaceAll(pkString, " ", ",")

	if _, err := adminPkFile.Write([]byte(pkString)); err != nil {
		return fmt.Errorf("write admin private key: %w", err)
	}

	params := []string{
		"deploy-solana",
		"deploy-program",
		"--url", sc.jsonRPCAddr,
		"--fee-payer", adminPkFile.Name(),
		"--key", filepath.Join("..", "..", solanaProgramDir, solanaProgramKeypairPath),
		"--build-path", filepath.Join("..", "..", solanaProgramDir, solanaProgramBuildPath),
		"--commitment", "finalized",
	}

	var b bytes.Buffer

	err = RunCommand(ResolveApexBridgeBinary(), params, io.MultiWriter(os.Stdout, &b))
	if err != nil {
		return err
	}

	output := b.String()

	reProgramID := regexp.MustCompile(`Program Id:\s*(\S+)`)

	programIDMatch := reProgramID.FindStringSubmatch(output)
	if programIDMatch == nil {
		return fmt.Errorf("program ID not found in output")
	}

	programID := programIDMatch[1]

	sc.programID = programID

	if err := sc.initializeProgram(ctx); err != nil {
		return fmt.Errorf("initialize program: %w", err)
	}

	if err := sc.registerTokens(ctx); err != nil {
		return fmt.Errorf("register tokens: %w", err)
	}

	return nil
}

func (sc *TestSolanaChain) initializeProgram(ctx context.Context) error {
	provider, err := sc.GetTxProvider()
	if err != nil {
		return fmt.Errorf("get tx provider: %w", err)
	}

	recentBlockhash, err := provider.GetLatestBlockhash(ctx)
	if err != nil {
		return fmt.Errorf("get latest blockhash: %w", err)
	}

	relayerAddr, err := solanawallet.PublicKeyFromAddress(sc.relayerAddr)
	if err != nil {
		return fmt.Errorf("get relayer address: %w", err)
	}

	txSender := solsendtx.NewTxSender(provider, &solsendtx.ChainConfig{
		MinOperationFeeAmount: sc.config.MinOperationFee.Uint64(),
		MinFeeForBridging:     sc.config.MinBridgingFee.Uint64(),
		MinAmountToBridge:     sc.config.MinTokenBridgingAmount.Uint64(),
		TreasuryAddress:       sc.config.TreasuryAddress,
		BridgingFeeAddress:    relayerAddr,
	})

	txDto := solsendtx.InitializeDto{
		AuthorityAddr: sc.admin.PublicKey.String(),
		Validators:    sc.validatorPubKeys,
		LastID:        0,
	}

	tx, err := txSender.CreateTx(
		ctx, sc.admin.PublicKey,
		solsendtx.InstructionTypeInitialize,
		recentBlockhash,
		txDto,
	)
	if err != nil {
		return fmt.Errorf("create initialize tx: %w", err)
	}

	_, err = tx.Sign(func(key solana.PublicKey) *solana.PrivateKey {
		return &sc.admin.PrivateKey
	})
	if err != nil {
		return fmt.Errorf("sign instruction: %w", err)
	}

	sig, err := txSender.SendTx(ctx, tx)
	if err != nil {
		return fmt.Errorf("send initialize tx: %w", err)
	}

	if err := provider.WaitForSignature(ctx, *sig, rpc.CommitmentFinalized, MaxConfirmationWaitTime); err != nil {
		return fmt.Errorf("wait for initialize confirmation: %w", err)
	}

	return nil
}

func (sc *TestSolanaChain) registerTokens(ctx context.Context) error {
	provider, err := sc.GetTxProvider()
	if err != nil {
		return fmt.Errorf("get tx provider: %w", err)
	}

	recentBlockhash, err := provider.GetLatestBlockhash(ctx)
	if err != nil {
		return fmt.Errorf("get latest blockhash: %w", err)
	}

	relayerAddr, err := solanawallet.PublicKeyFromAddress(sc.relayerAddr)
	if err != nil {
		return fmt.Errorf("get relayer address: %w", err)
	}

	txSender := solsendtx.NewTxSender(provider, &solsendtx.ChainConfig{
		TreasuryAddress:    sc.config.TreasuryAddress,
		BridgingFeeAddress: relayerAddr,
	})

	for tokenID, tokenMint := range sc.config.TokensMint {
		txDto := solsendtx.RegisterTokenLockUnlockDto{
			AuthorityAddr:     sc.admin.PublicKey.String(),
			TokenMint:         tokenMint,
			TokenID:           tokenID,
			MinBridgingAmount: sc.config.MinTokenBridgingAmount.Uint64(),
		}

		tx, err := txSender.CreateTx(
			ctx, sc.admin.PublicKey,
			solsendtx.InstructionTypeRegisterTokensLockUnlock,
			recentBlockhash,
			txDto,
		)
		if err != nil {
			return fmt.Errorf("create register token lock unlock tx: %w", err)
		}

		_, err = tx.Sign(func(key solana.PublicKey) *solana.PrivateKey {
			return &sc.admin.PrivateKey
		})
		if err != nil {
			return fmt.Errorf("sign instruction: %w", err)
		}

		sig, err := txSender.SendTx(ctx, tx)
		if err != nil {
			return fmt.Errorf("send register token lock unlock tx: %w", err)
		}

		if err := provider.WaitForSignature(ctx, *sig, rpc.CommitmentFinalized, MaxConfirmationWaitTime); err != nil {
			return fmt.Errorf("wait for register token lock unlock confirmation: %w", err)
		}

		fmt.Printf("registered token %d with mint %s\n", tokenID, tokenMint)
	}

	return nil
}

func (sc *TestSolanaChain) FundWallets(ctx context.Context) error {
	if sc.jsonRPCAddr == "" {
		return nil
	}

	provider, err := sc.GetTxProvider()
	if err != nil {
		return fmt.Errorf("get tx provider: %w", err)
	}

	solFundAmount := WeiToLamport(sc.config.FundAmount)

	premineAddresses := sc.config.PreminesAddresses
	premineAddresses = append(premineAddresses, sc.relayerAddr)

	for _, addr := range premineAddresses {
		if err := sc.airdropSOL(ctx, provider, addr, solFundAmount); err != nil {
			return fmt.Errorf("airdrop SOL to %s: %w", addr, err)
		}
	}

	// Fund the admin wallet with SOL, wrap to wSOL, send to premine wallets
	solFundAmount = solFundAmount.Mul(solFundAmount, big.NewInt(int64(len(sc.config.PreminesAddresses)+3)))

	if err := sc.airdropSOL(ctx, provider, sc.admin.PublicKey.String(), solFundAmount); err != nil {
		return fmt.Errorf("airdrop SOL to admin: %w", err)
	}

	wrapSolAmount := solFundAmount.Mul(
		WeiToLamport(sc.config.FundAmount),
		big.NewInt(int64(len(sc.config.PreminesAddresses)+1)),
	)

	// wrap SOL to wSOL
	if err := sc.wrapSOL(ctx, provider, sc.admin, wrapSolAmount); err != nil {
		return fmt.Errorf("wrap SOL: %w", err)
	}

	// send wSOL to premine wallets
	receivers := make([]GenericTxReceiver, len(sc.config.PreminesAddresses))
	for i, addr := range sc.config.PreminesAddresses {
		receivers[i] = GenericTxReceiver{
			Addr:   addr,
			Amount: big.NewInt(0),
			NativeTokens: []GenericTokenAmount{
				{
					Token: carwallet.Token{
						PolicyID: wsolMint.String(),
					},
					Amount: WeiToLamport(sc.config.FundAmount),
				},
			},
		}
	}

	if _, err := sc.SendTx(ctx, sc.admin.PrivateKey.String(), nil,
		receivers, sc.config.MinOperationFee.Uint64()); err != nil {
		return fmt.Errorf("send wSOL to premine wallets: %w", err)
	}

	return nil
}

func (sc *TestSolanaChain) airdropSOL(
	ctx context.Context, provider *solanawallet.Provider, addr string, amount *big.Int,
) error {
	fmt.Printf("airdropping to %s with amount %s\n", addr, amount.String())

	pubKey, err := solanawallet.PublicKeyFromAddress(addr)
	if err != nil {
		return fmt.Errorf("parse public key: %w", err)
	}

	sig, err := provider.RequestSolAirdrop(
		ctx,
		pubKey,
		amount.Uint64(),
	)
	if err != nil {
		return fmt.Errorf("request airdrop: %w", err)
	}

	if err := provider.WaitForSignature(ctx, sig, rpc.CommitmentFinalized, MaxConfirmationWaitTime); err != nil {
		return fmt.Errorf("wait for airdrop confirmation: %w", err)
	}

	return nil
}

// wrapSOL wraps native SOL into WSOL for the given owner using the spl-token wrap CLI.
func (sc *TestSolanaChain) wrapSOL(
	ctx context.Context, provider *solanawallet.Provider,
	owner *solanawallet.Wallet,
	amount *big.Int,
) error {
	if !amount.IsUint64() {
		return fmt.Errorf("wrap amount too large: %s", amount)
	}

	solAmount := LamportToSolana(amount).Uint64()
	if solAmount == 0 {
		return nil
	}

	fmt.Printf("wrapping %s with amount %s\n", owner.PublicKey.String(), amount.String())

	keypairFile, err := os.CreateTemp(os.TempDir(), "wrap-keypair-*.json")
	if err != nil {
		return fmt.Errorf("create keypair file: %w", err)
	}

	defer os.Remove(keypairFile.Name())
	defer keypairFile.Close()

	// Same format as InitContracts: JSON array of bytes [n1,n2,...] required by Solana/SPL CLI
	pkString := fmt.Sprintf("%v", []byte(owner.PrivateKey))
	pkString = strings.ReplaceAll(pkString, " ", ",")

	if _, err := keypairFile.Write([]byte(pkString)); err != nil {
		return fmt.Errorf("write keypair: %w", err)
	}

	if err := keypairFile.Sync(); err != nil {
		return fmt.Errorf("sync keypair file: %w", err)
	}

	// spl-token wrap <AMOUNT> [KEYPAIR] --url <RPC>. Amount is in SOL.
	args := []string{
		"wrap",
		strconv.FormatUint(solAmount, 10),
		keypairFile.Name(),
		"--url", sc.jsonRPCAddr,
		"--fee-payer", keypairFile.Name(),
	}

	var b bytes.Buffer

	if err := RunCommand(ResolveSPLTokenBinary(), args, io.MultiWriter(os.Stdout, &b)); err != nil {
		return fmt.Errorf("spl-token wrap: %w", err)
	}

	output := b.String()

	reWrapSignature := regexp.MustCompile(`Signature:\s*(\S+)`)

	wrapSignatureMatch := reWrapSignature.FindStringSubmatch(output)
	if wrapSignatureMatch == nil {
		return fmt.Errorf("wrap signature not found in output")
	}

	if err := provider.WaitForSignature(
		ctx,
		solana.MustSignatureFromBase58(wrapSignatureMatch[1]),
		rpc.CommitmentConfirmed, MaxConfirmationWaitTime); err != nil {
		return fmt.Errorf("wait for wrap confirmation: %w", err)
	}

	return nil
}

func (sc *TestSolanaChain) GenerateChainConfigs(indx int, validator *TestApexValidator) error {
	dbsPath := filepath.Join(validator.dataDirPath, BridgingDBsDir)

	args := []string{
		"generate-configs", "solana-chain",
		"--chain-id", sc.ChainID(),
		"--sol-node-url", sc.jsonRPCAddr,
		"--sol-tracked-program", sc.programID,
		"--sol-min-fee-for-bridging", sc.config.MinBridgingFee.String(),
		"--sol-min-operation-fee", sc.config.MinOperationFee.String(),
		"--output-dir", validator.GetBridgingConfigsDir(),
		"--output-validator-components-file-name", ValidatorComponentsConfigFileName,
		"--output-relayer-file-name", RelayerConfigFileName,
		"--relayer-data-dir", validator.GetRelayerDataDir(),
		"--dbs-path", dbsPath,
		"--treasury-address", sc.config.TreasuryAddress.String(),
		"--fee-addr-bridging", sc.relayerAddr,
	}

	return RunCommand(ResolveApexBridgeBinary(), args, os.Stdout)
}

func (sc *TestSolanaChain) GetAddressBalance(ctx context.Context, addr string) (map[string]*big.Int, error) {
	txProvider, err := sc.GetTxProvider()
	if err != nil {
		return nil, err
	}

	pubKey, err := solanawallet.PublicKeyFromAddress(addr)
	if err != nil {
		return nil, err
	}

	balance, err := txProvider.GetBalance(ctx, pubKey)
	if err != nil {
		return nil, err
	}

	return map[string]*big.Int{addr: LamportToWei(big.NewInt(int64(balance)))}, nil
}

func (sc *TestSolanaChain) GetAddressBalanceWithTokenName(
	ctx context.Context, addr string, tokenName string) (map[string]*big.Int, error) {
	pubKey, err := solanawallet.PublicKeyFromAddress(addr)
	if err != nil {
		return nil, fmt.Errorf("GetAddressBalanceWithTokenName parse address: %w", err)
	}

	mintPubKey, err := solanawallet.PublicKeyFromAddress(tokenName)
	if err != nil {
		return nil, fmt.Errorf("GetAddressBalanceWithTokenName parse mint address: %w", err)
	}

	ata, _, err := solanawallet.FindAssociatedTokenAddress(pubKey, mintPubKey)
	if err != nil {
		return nil, fmt.Errorf("GetAddressBalanceWithTokenName find associated token address: %w", err)
	}

	txProvider, err := sc.GetTxProvider()
	if err != nil {
		return nil, err
	}

	res, err := txProvider.GetTokenAccountBalance(ctx, ata)
	if err != nil {
		return map[string]*big.Int{tokenName: big.NewInt(0)}, err // return 0 so caller can still log "failed to query"
	}

	if res == nil || res.Value == nil {
		return map[string]*big.Int{tokenName: big.NewInt(0)}, nil
	}

	amountBigInt, ok := new(big.Int).SetString(res.Value.Amount, 10)
	if !ok {
		return map[string]*big.Int{tokenName: big.NewInt(0)},
			fmt.Errorf("GetAddressBalanceWithTokenName parse amount: %s", res.Value.Amount)
	}

	return map[string]*big.Int{tokenName: LamportToWei(amountBigInt)}, nil
}

func (sc *TestSolanaChain) GetAddressToBridgeTo(ctx context.Context, hasTokens bool) (string, error) {
	return sc.gatewayAddr, nil
}

// wTODO: Implement this for getting the admin private key on the solana chain
func (sc *TestSolanaChain) GetAdminPrivateKey() (string, error) {
	if sc.admin == nil {
		return "", fmt.Errorf("admin private key is not set")
	}

	return sc.admin.PrivateKey.String(), nil
}

func (sc *TestSolanaChain) GetBridgingFee(
	_ context.Context,
	_ string,
	_ []carsendtx.BridgingTxReceiver,
	bridgingFee *big.Int,
	_ *big.Int,
	_ string,
) (*big.Int, error) {
	return bridgingFee, nil
}

// GetBridgingStakeAddressInfo implements ITestApexChain.
func (*TestSolanaChain) GetBridgingStakeAddressInfo(
	t *testing.T,
	ctx context.Context,
	indx uint8,
	expectError bool,
) (carwallet.QueryStakeAddressInfo, error) {
	t.Helper()

	panic("unimplemented") //nolint:gocritic
}

func (sc *TestSolanaChain) GetCardanoScriptInfo() *CardanoScriptInfo {
	panic("unimplemented") //nolint:gocritic
}

func (sc *TestSolanaChain) GetCustodialAddress() string {
	panic("unimplemented") //nolint:gocritic
}

func (*TestSolanaChain) GetExistingStakePools(t *testing.T, ctx context.Context) []string {
	t.Helper()
	panic("unimplemented") //nolint:gocritic
}

func (sc *TestSolanaChain) GetHotWalletAddresses() []string {
	return []string{sc.gatewayAddr}
}

// GetIndexer implements ITestApexChain.
func (sc *TestSolanaChain) GetIndexer() e2eindexer.TxsExecutedComponent {
	return sc.indexer
}

func (sc *TestSolanaChain) GetMintableTokens() map[uint16]string {
	// Unnecessary until we need a coloredcoin support
	return make(map[uint16]string)
}

// GetRelayerAddress implements ITestApexChain.
func (sc *TestSolanaChain) GetRelayerAddress() string {
	return ""
}

// GetServerMust implements ITestApexChain.
func (sc *TestSolanaChain) GetServerMust(t *testing.T, indx int) ITestApexChainServer {
	t.Helper()

	require.True(t, sc.cluster != nil && sc.cluster.Servers != nil && len(sc.cluster.Servers) > indx)

	return sc.cluster.Servers[indx]
}

// wTODO: Implement this for initializing solana contract
func (sc *TestSolanaChain) InitContracts(
	ctx context.Context, bridgeAdmin *crypto.ECDSAKey, bridgeURL string, chainIDsConfig string) error {
	return nil
}

// wTODO: Implement this when SolanaInfo is added to the ApexSystem
func (*TestSolanaChain) PopulateApexSystem(t *testing.T, apexSystem *ApexSystem) error {
	t.Helper()

	return nil
}

// wTODO: Implement this for registering the solana chain
func (sc *TestSolanaChain) RegisterChain(validator *TestApexValidator) error {
	return validator.RegisterChain(
		sc.ChainID(), sc.config.InitialHotWalletAmount, big.NewInt(0), ChainTypeSolana)
}

// RunChain implements ITestApexChain.
func (sc *TestSolanaChain) RunChain(t *testing.T) error {
	t.Helper()

	cluster, err := solanafw.NewSolanaTestCluster(t,
		solanafw.WithPort(sc.config.StartingPort),
		solanafw.WithWSPort(sc.config.StartingPort+1),
	)
	if err != nil {
		return err
	}

	fmt.Printf("%s chain setup done: port = %d\n", sc.config.ChainID, sc.config.StartingPort)

	sc.cluster = cluster
	sc.jsonRPCAddr = sc.cluster.Servers[0].NetworkAddress()

	return nil
}

func (sc *TestSolanaChain) SendTx(
	ctx context.Context,
	privateKey string,
	metadata []byte,
	receivers []GenericTxReceiver,
	operationFee uint64,
) (string, error) {
	wallet, err := solanawallet.NewWalletFromPrivateKey(privateKey)
	if err != nil {
		return "", err
	}

	txProvider, err := sc.GetTxProvider()
	if err != nil {
		return "", err
	}

	relayerAddr, err := solanawallet.PublicKeyFromAddress(sc.relayerAddr)
	if err != nil {
		return "", fmt.Errorf("get relayer address: %w", err)
	}

	txSender := solsendtx.NewTxSender(txProvider, &solsendtx.ChainConfig{
		TreasuryAddress:    sc.config.TreasuryAddress,
		BridgingFeeAddress: relayerAddr,
	})

	for _, receiver := range receivers {
		if receiver.NativeTokens != nil {
			// Check sender's token balance before attempting wSOL/SPL transfer
			if bal, err := sc.GetAddressBalanceWithTokenName(ctx, wallet.PublicKey.String(),
				receiver.NativeTokens[0].PolicyID); err != nil {
				fmt.Printf("sender %s token balance (mint %s): failed to query: %v\n",
					wallet.PublicKey.String(),
					receiver.NativeTokens[0].PolicyID,
					err,
				)
			} else {
				fmt.Printf("sender %s token balance (mint %s): %s (attempting to send %s)\n",
					wallet.PublicKey.String(),
					receiver.NativeTokens[0].PolicyID,
					bal[receiver.NativeTokens[0].PolicyID].String(),
					receiver.NativeTokens[0].Amount.String())
			}

			// Create receiver ATA in a separate confirmed transaction before transferring.
			// Combining CreateATA + Transfer in one tx fails during simulation because the runtime
			// preloads accounts before instructions run, so the destination ATA appears as "not found".
			if err := sc.ensureReceiverTokenAccount(ctx, txProvider, txSender, wallet,
				receiver.Addr, receiver.NativeTokens[0].PolicyID); err != nil {
				return "", err
			}

			err = splTokenTransfer(ctx, txProvider, txSender, wallet, receiver)
			if err != nil {
				return "", err
			}
		}

		err := tokenTransfer(ctx, txProvider, txSender, wallet, receiver)
		if err != nil {
			return "", err
		}
	}

	return "", nil
}

// ensureReceiverTokenAccount creates the receiver's Associated Token Account for the given mint if it does not exist.
// This fixes "AccountNotFound" when sending wSOL (or any SPL token) to a wallet that has never held that token.
func (sc *TestSolanaChain) ensureReceiverTokenAccount(
	ctx context.Context,
	txProvider *solanawallet.Provider,
	txSender *solsendtx.TxSender,
	senderWallet *solanawallet.Wallet,
	receiverAddr, mintAddress string,
) error {
	receiverPubKey, err := solanawallet.PublicKeyFromAddress(receiverAddr)
	if err != nil {
		return fmt.Errorf("receiver address: %w", err)
	}

	mintPubKey, err := solanawallet.PublicKeyFromAddress(mintAddress)
	if err != nil {
		return fmt.Errorf("mint address: %w", err)
	}

	receiverAta, _, err := solanawallet.FindAssociatedTokenAddress(receiverPubKey, mintPubKey)
	if err != nil {
		return fmt.Errorf("receiver ATA: %w", err)
	}

	// Check if account already exists: Solana returns {Value: null} with no error when account doesn't exist
	info, err := txProvider.GetAccountInfo(ctx, receiverAta)
	if err == nil && info != nil && info.Value != nil {
		return nil // already exists
	}

	fmt.Printf("creating receiver ATA for %s with mint %s\n", receiverAddr, mintAddress)

	txDto := solsendtx.CreateInstructionDto{
		SenderPublicKey:   senderWallet.PublicKey.String(),
		ReceiverPublicKey: receiverAddr,
		MintTokenAddress:  mintAddress,
	}

	recentBlockhash, err := txProvider.GetLatestBlockhash(ctx)
	if err != nil {
		return err
	}

	tx, err := txSender.CreateTx(
		ctx,
		senderWallet.PublicKey,
		solsendtx.InstructionCreateInstruction,
		recentBlockhash,
		txDto,
	)
	if err != nil {
		return fmt.Errorf("create instruction: %w", err)
	}

	_, err = tx.Sign(func(key solana.PublicKey) *solana.PrivateKey {
		return &senderWallet.PrivateKey
	})
	if err != nil {
		return fmt.Errorf("sign instruction: %w", err)
	}

	sig, err := txSender.SendTx(ctx, tx)
	if err != nil {
		return fmt.Errorf("send create instruction: %w", err)
	}

	fmt.Printf("created receiver ATA for %s with mint %s: %s\n", receiverAddr, mintAddress, sig.String())

	if err := txProvider.WaitForSignature(ctx, *sig, rpc.CommitmentConfirmed, MaxConfirmationWaitTime); err != nil {
		return fmt.Errorf("wait for create instruction confirmation: %w", err)
	}

	return nil
}

func splTokenTransfer(
	ctx context.Context,
	txProvider *solanawallet.Provider,
	txSender *solsendtx.TxSender,
	wallet *solanawallet.Wallet,
	receiver GenericTxReceiver,
) error {
	if receiver.NativeTokens == nil || len(receiver.NativeTokens) == 0 {
		return nil
	}

	if receiver.NativeTokens[0].Amount.Cmp(big.NewInt(0)) == 0 {
		return nil
	}

	// Amount must be in token base units (lamports for wSOL).
	// NativeTokens[0].Amount is already in lamports (set via WeiToLamport).
	txDto := solsendtx.SPLTransferDto{
		SenderPublicKey:   wallet.PublicKey.String(),
		ReceiverPublicKey: receiver.Addr,
		Amount:            receiver.NativeTokens[0].Amount.Uint64(),
		MintTokenAddress:  receiver.NativeTokens[0].PolicyID,
		TokenDecimals:     solana.SolDecimals,
	}

	recentBlockhash, err := txProvider.GetLatestBlockhash(ctx)
	if err != nil {
		return err
	}

	tx, err := txSender.CreateTx(ctx, wallet.PublicKey, solsendtx.InstructionTypeSPLTransfer, recentBlockhash, txDto)
	if err != nil {
		return err
	}

	_, err = tx.Sign(func(key solana.PublicKey) *solana.PrivateKey {
		return &wallet.PrivateKey
	})
	if err != nil {
		return fmt.Errorf("sign instruction: %w", err)
	}

	sig, err := txSender.SendTx(ctx, tx)
	if err != nil {
		return err
	}

	err = txProvider.WaitForSignature(ctx, *sig, rpc.CommitmentConfirmed, MaxConfirmationWaitTime)
	if err != nil {
		return fmt.Errorf("wait for token transfer confirmation: %w", err)
	}

	fmt.Println("token transfer confirmed: ", sig.String())

	return nil
}

func tokenTransfer(
	ctx context.Context,
	txProvider *solanawallet.Provider,
	txSender *solsendtx.TxSender,
	wallet *solanawallet.Wallet,
	receiver GenericTxReceiver,
) error {
	if receiver.Amount.Cmp(big.NewInt(0)) == 0 {
		return nil
	}

	txDto := solsendtx.SOLTransferDto{
		SenderPublicKey:   wallet.PublicKey.String(),
		ReceiverPublicKey: receiver.Addr,
		Amount:            WeiToLamport(receiver.Amount).Uint64(),
	}

	recentBlockhash, err := txProvider.GetLatestBlockhash(ctx)
	if err != nil {
		return err
	}

	tx, err := txSender.CreateTx(ctx, wallet.PublicKey, solsendtx.InstructionTypeSOLTransfer, recentBlockhash, txDto)
	if err != nil {
		return err
	}

	_, err = tx.Sign(func(key solana.PublicKey) *solana.PrivateKey {
		return &wallet.PrivateKey
	})
	if err != nil {
		return fmt.Errorf("sign instruction: %w", err)
	}

	sig, err := txSender.SendTx(ctx, tx)
	if err != nil {
		return err
	}

	err = txProvider.WaitForSignature(ctx, *sig, rpc.CommitmentConfirmed, MaxConfirmationWaitTime)
	if err != nil {
		return fmt.Errorf("wait for sol transfer confirmation: %w", err)
	}

	fmt.Println("sol transfer confirmed: ", sig.String())

	return nil
}

func (sc *TestSolanaChain) SetCustodialNFT(token carwallet.Token) {
	panic("unimplemented") //nolint:gocritic
}

func (sc *TestSolanaChain) Stop() error {
	if sc.cluster != nil {
		return sc.cluster.Stop()
	}

	return nil
}

func (sc *TestSolanaChain) UpdateTxSendChainConfiguration(_ map[string]carsendtx.ChainConfig) {
}
