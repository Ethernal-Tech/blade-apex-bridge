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
	"github.com/Ethernal-Tech/solana-infrastructure/sendtx/skyline_program"
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
	solanaProgramUpgradePath = "program_build/skyline_program_v2.so"

	TreasuryAddress = "AXXWYCH6PNm6AGjaasPG1maarfQvRedSw18wj91Nem1F"

	MaxConfirmationWaitTime = 3 * time.Minute

	solanaFixedSupplyMintAmount = "1000000000000000"
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

	// Human readable names of tokens that should be mintable on this chain
	MintableTokens   map[uint16]string
	LockUnlockTokens map[uint16]string
}

func NewSolanaChainConfig(enabled bool) *TestSolanaChainConfig {
	return &TestSolanaChainConfig{
		ChainID:                ChainIDSolana,
		IsEnabled:              enabled,
		StartingPort:           8899,
		InitialHotWalletAmount: SolanaToWei(big.NewInt(1000)),
		FundAmount:             LamportToWei(SolanaToLamport(big.NewInt(100000))),
		MinBridgingFee:         big.NewInt(6000000), // 0.006 SOL
		MinBridgingAmount:      big.NewInt(1000),    // 0.000001 SOL
		MinTokenBridgingAmount: big.NewInt(1000),    // 0.000001 SOL
		MinOperationFee:        big.NewInt(1500000), // 0.0015 SOL
		CurrencyID:             WSOLTokenID,
		TreasuryAddress:        solana.MustPublicKeyFromBase58(TreasuryAddress),
		TokensMint: map[uint16]string{
			WSOLTokenID: WSOLMintAddress, // by default add wSOL to the tokens mint map
			SOLTokenID:  skyline_program.NATIVE_SOL_MINT.String(),
		},

		LockUnlockTokens: map[uint16]string{
			WSOLTokenID: WSOLANATokenName,
			SOLTokenID:  SOLANATokenName,
		},

		MintableTokens: map[uint16]string{
			SAP3XTokenID: SAP3XTokenName,
			VSTokenID:    VSTokenName,
			NSTokenID:    NSTokenName,
		},
	}
}

func NewRemoteSolanaChainConfig(
	isEnabled bool, minBridgingFeeAmount, minOperationFee *big.Int, treasuryAddress string) *TestSolanaChainConfig {
	return &TestSolanaChainConfig{
		IsEnabled:       isEnabled,
		ChainID:         ChainIDSolana,
		MinBridgingFee:  minBridgingFeeAmount,
		MinOperationFee: minOperationFee,
		TreasuryAddress: solana.MustPublicKeyFromBase58(treasuryAddress),
		CurrencyID:      WSOLTokenID,
		LockUnlockTokens: map[uint16]string{
			WSOLTokenID: WSOLANATokenName,
		},
		MintableTokens: map[uint16]string{
			SAP3XTokenID: SAP3XTokenName,
		},
		TokensMint: map[uint16]string{},
	}
}

type TestSolanaChain struct {
	config           *TestSolanaChainConfig
	provider         *solanawallet.Provider
	relayerAddr      string
	validatorPubKeys []string
	cluster          *solanafw.TestSolanaCluster
	admin            *solanawallet.Wallet
	jsonRPCAddr      string
	gatewayAddr      string
	indexer          e2eindexer.TxsExecutedComponent
	programID        string
	altPublicKey     string
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

func NewRemoteTestSolanaChain(
	programID, jsonRPCURL, relayerAddress, treasuryAddress string,
	minBridgingFee, minOperationFee *big.Int,
) *TestSolanaChain {
	return &TestSolanaChain{
		config: NewRemoteSolanaChainConfig(
			true, minBridgingFee, minOperationFee, treasuryAddress),
		relayerAddr: relayerAddress,
		jsonRPCAddr: jsonRPCURL,
		programID:   programID,
		indexer:     e2eindexer.NewTxsExecutedComponentDummy(),
	}
}

func (sc *TestSolanaChain) GetTxProvider() (*solanawallet.Provider, error) {
	if sc.provider == nil {
		provider, err := solanawallet.NewProvider(sc.jsonRPCAddr, nil)
		if err != nil {
			return nil, fmt.Errorf("new provider: %w", err)
		}

		sc.provider = provider
	}

	return sc.provider, nil
}

func (sc *TestSolanaChain) GetTreasuryAddress() string {
	return sc.config.TreasuryAddress.String()
}

func (sc *TestSolanaChain) GetVaultAddress() (string, error) {
	programPubKey, err := solanawallet.PublicKeyFromAddress(sc.programID)
	if err != nil {
		return "", fmt.Errorf("parse program public key: %w", err)
	}

	vaultPda, _, err := solana.FindProgramAddress(
		[][]byte{[]byte("vault")},
		programPubKey,
	)
	if err != nil {
		return "", fmt.Errorf("derive vault PDA: %w", err)
	}

	return vaultPda.String(), nil
}

func (sc *TestSolanaChain) BridgingRequest(params BridgingRequestParams) (string, error) {
	txProvider, err := sc.GetTxProvider()
	if err != nil {
		return "", err
	}

	txSender := solsendtx.NewTxSender(txProvider, &solsendtx.ChainConfig{
		TreasuryAddress: sc.config.TreasuryAddress,
	})

	txReceivers := make([]solsendtx.BridgingTxReceiver, 0, len(params.Receivers))
	for addr, amount := range params.Receivers {
		txReceivers = append(txReceivers, solsendtx.BridgingTxReceiver{
			Address: addr,
			TokenAmount: solanawallet.TokenAmount{
				TokenID: amount.TokenID,
				Amount:  WeiToLamport(amount.Amount).Uint64(),
			},
		})
	}

	senderWallet, err := solanawallet.NewWalletFromPrivateKey(params.PrivateKey)
	if err != nil {
		return "", err
	}

	txDto := solsendtx.BridgeRequestDto{
		Ctx:          params.Ctx,
		ProgramID:    solana.MustPublicKeyFromBase58(sc.programID),
		DstChainID:   params.DestChainID,
		SenderAddr:   senderWallet.PublicKey.String(),
		Receivers:    txReceivers,
		BridgingFee:  params.FeeAmount.Uint64(),
		OperationFee: params.OperationFee.Uint64(),
	}

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

	if err := txProvider.WaitForSignature(params.Ctx, *sig, rpc.CommitmentFinalized, MaxConfirmationWaitTime); err != nil {
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

	adminBalanceBefore, err := sc.GetAddressBalance(ctx, sc.admin.PublicKey.String())
	if err != nil {
		return fmt.Errorf("get admin balance before deploying program: %w", err)
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

	adminBalanceAfter, err := sc.GetAddressBalance(ctx, sc.admin.PublicKey.String())
	if err != nil {
		return fmt.Errorf("get admin balance after deploying program: %w", err)
	}

	diff := new(big.Int).Sub(adminBalanceBefore["lovelace"], adminBalanceAfter["lovelace"])
	fmt.Println("Program deployment cost: ", WeiToLamport(diff))

	if err := sc.initializeProgram(); err != nil {
		return fmt.Errorf("initialize program: %w", err)
	}

	if err := sc.deployLockUnlockTokens(ctx); err != nil {
		return fmt.Errorf("deploy mintable tokens: %w", err)
	}

	if err := sc.registerTokens(); err != nil {
		return fmt.Errorf("register tokens: %w", err)
	}

	if err := sc.initializeALT(); err != nil {
		return fmt.Errorf("initialize ALT: %w", err)
	}

	if err := sc.hotWalletIncrementFunding(ctx); err != nil {
		return fmt.Errorf("increment hot wallet funding: %w", err)
	}

	return nil
}

func (sc *TestSolanaChain) hotWalletIncrementFunding(ctx context.Context) error {
	adminBalance, err := sc.GetAddressBalanceWithTokenName(ctx, sc.admin.PublicKey.String(), WSOLMintAddress)
	if err != nil {
		return fmt.Errorf("get admin balance: %w", err)
	}

	fmt.Printf("admin balance: %s\n", adminBalance)

	if adminBalance[WSOLMintAddress].Cmp(sc.config.InitialHotWalletAmount) < 0 {
		return fmt.Errorf("admin balance is less than initial hot wallet amount")
	}

	fmt.Printf("initial hot wallet amount: %s\n", sc.config.InitialHotWalletAmount.String())

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

	for tokenID, tokenName := range sc.config.LockUnlockTokens {
		if tokenName != WSOLANATokenName && tokenName != SOLANATokenName {
			continue
		}

		hotWalletIncrementParams := []string{
			"deploy-solana",
			"hot-wallet-increment",
			"--url", sc.jsonRPCAddr,
			"--key", adminPkFile.Name(),
			"--mint", sc.config.TokensMint[tokenID],
			"--program-id", sc.programID,
			"--amount", strconv.Itoa(int(WeiToLamport(sc.config.InitialHotWalletAmount).Uint64())),
		}

		var b bytes.Buffer

		err = RunCommand(ResolveApexBridgeBinary(), hotWalletIncrementParams, io.MultiWriter(os.Stdout, &b))
		if err != nil {
			return fmt.Errorf("increment hot wallet funding: %w", err)
		}

		fmt.Printf("incremented hot wallet funding for token %d with name %s on Solana\n", tokenID, tokenName)
	}

	return nil
}

func (sc *TestSolanaChain) initializeProgram() error {
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
		"initialize-program",
		"--url", sc.jsonRPCAddr,
		"--program-id", sc.programID,
		"--admin-key", adminPkFile.Name(),
		"--last-id", "0",
		"--min-operation-fee", strconv.Itoa(int(sc.config.MinOperationFee.Uint64())),
		"--min-fee-for-bridging", strconv.Itoa(int(sc.config.MinBridgingFee.Uint64())),
		"--min-amount-to-bridge", strconv.Itoa(int(sc.config.MinTokenBridgingAmount.Uint64())),
		"--treasury-address", sc.config.TreasuryAddress.String(),
		"--confirmation-timeout-seconds", strconv.Itoa(int(MaxConfirmationWaitTime.Seconds())),
	}

	for _, validatorPubKey := range sc.validatorPubKeys {
		params = append(params, "--validator", validatorPubKey)
	}

	var b bytes.Buffer

	err = RunCommand(ResolveApexBridgeBinary(), params, io.MultiWriter(os.Stdout, &b))
	if err != nil {
		return err
	}

	return nil
}

func (sc *TestSolanaChain) deployLockUnlockTokens(ctx context.Context) error {
	provider, err := sc.GetTxProvider()
	if err != nil {
		return fmt.Errorf("get tx provider: %w", err)
	}

	for tokenID, tokenName := range sc.config.LockUnlockTokens {
		if tokenName == WSOLANATokenName || tokenName == SOLANATokenName {
			// wSOL already exists on chain, so we don't need to deploy it again
			continue
		}

		fmt.Printf("deploying lock/unlock token %d with name %s on Solana\n", tokenID, tokenName)

		tokenMint, err := sc.createSPLToken(ctx, provider)
		if err != nil {
			return fmt.Errorf("create lock/unlock token %d (%s): %w", tokenID, tokenName, err)
		}

		fmt.Printf("deployed lock/unlock token %d with name %s on Solana with mint %s\n", tokenID, tokenName, tokenMint)

		sc.config.TokensMint[tokenID] = tokenMint
	}

	return nil
}

func (sc *TestSolanaChain) createSPLToken(
	ctx context.Context,
	provider *solanawallet.Provider,
) (string, error) {
	adminPkFile, err := os.CreateTemp(os.TempDir(), "solana-admin-pk-*.json")
	if err != nil {
		return "", fmt.Errorf("create temp keypair file: %w", err)
	}

	defer os.Remove(adminPkFile.Name())
	defer adminPkFile.Close()

	pkString := fmt.Sprintf("%v", []byte(sc.admin.PrivateKey))
	pkString = strings.ReplaceAll(pkString, " ", ",")

	if _, err := adminPkFile.Write([]byte(pkString)); err != nil {
		return "", fmt.Errorf("write keypair file: %w", err)
	}

	if err := adminPkFile.Sync(); err != nil {
		return "", fmt.Errorf("sync keypair file: %w", err)
	}

	// 1. Create SPL token
	createTokenArgs := []string{
		"create-token",
		"--url", sc.jsonRPCAddr,
		"--fee-payer", adminPkFile.Name(),
		"--owner", adminPkFile.Name(),
		"--decimals", strconv.Itoa(int(solana.SolDecimals)),
	}

	createTokenOutput, err := sc.runSPLTokenCommandAndWait(ctx, provider, createTokenArgs...)
	if err != nil {
		return "", fmt.Errorf("create token: %w", err)
	}

	mintAddress, err := parseSPLTokenMintAddress(createTokenOutput)
	if err != nil {
		return "", fmt.Errorf("parse mint address: %w", err)
	}

	// 2. Mint token to admin's ATA
	adminPubKey, err := solanawallet.PublicKeyFromAddress(sc.admin.PublicKey.String())
	if err != nil {
		return "", fmt.Errorf("parse admin public key: %w", err)
	}

	mintPubKey, err := solanawallet.PublicKeyFromAddress(mintAddress)
	if err != nil {
		return "", fmt.Errorf("parse mint public key: %w", err)
	}

	adminATA, _, err := solanawallet.FindAssociatedTokenAddress(adminPubKey, mintPubKey)
	if err != nil {
		return "", fmt.Errorf("find admin ATA: %w", err)
	}

	createAccountArgs := []string{
		"create-account", mintAddress,
		"--url", sc.jsonRPCAddr,
		"--fee-payer", adminPkFile.Name(),
		"--owner", sc.admin.PublicKey.String(),
	}
	if _, err := sc.runSPLTokenCommandAndWait(ctx, provider, createAccountArgs...); err != nil {
		return "", fmt.Errorf("create admin token account: %w", err)
	}

	mintArgs := []string{
		"mint", mintAddress, solanaFixedSupplyMintAmount, adminATA.String(),
		"--url", sc.jsonRPCAddr,
		"--fee-payer", adminPkFile.Name(),
		"--owner", adminPkFile.Name(),
	}
	if _, err := sc.runSPLTokenCommandAndWait(ctx, provider, mintArgs...); err != nil {
		return "", fmt.Errorf("mint fixed supply: %w", err)
	}

	// 3. Disable mint authority
	disableMintAuthorityArgs := []string{
		"authorize", mintAddress, "mint", "--disable",
		"--url", sc.jsonRPCAddr,
		"--fee-payer", adminPkFile.Name(),
		"--owner", adminPkFile.Name(),
	}
	if _, err := sc.runSPLTokenCommandAndWait(ctx, provider, disableMintAuthorityArgs...); err != nil {
		return "", fmt.Errorf("disable mint authority: %w", err)
	}

	adminLockUnlockTokenBalance, err := provider.GetTokenAccountBalance(ctx, adminATA)
	if err != nil {
		return "", fmt.Errorf("get admin lock/unlock token balance: %w", err)
	}

	if adminLockUnlockTokenBalance.Value == nil {
		return "", fmt.Errorf("get admin lock/unlock token balance: nil value")
	}

	fmt.Printf("admin lock/unlock token balance: %s\n", adminLockUnlockTokenBalance.Value.Amount)

	txSender := solsendtx.NewTxSender(provider, &solsendtx.ChainConfig{
		TreasuryAddress: sc.config.TreasuryAddress,
	})

	vaultAddress, err := sc.GetVaultAddress()
	if err != nil {
		return "", err
	}

	// 4. Create program's ATA before sending tokens to it
	if err := sc.ensureReceiverTokenAccount(ctx, provider, txSender,
		sc.admin, vaultAddress, mintAddress); err != nil {
		return "", fmt.Errorf("ensure receiver token account: %w", err)
	}

	// 5. Send tokens to program
	mintAmount, _ := new(big.Int).SetString(solanaFixedSupplyMintAmount, 10)

	if _, err := splTokenTransfer(ctx, provider, txSender, sc.admin, vaultAddress, GenericTokenAmount{
		Token:  carwallet.Token{PolicyID: mintAddress},
		Amount: LamportToWei(mintAmount),
	}); err != nil {
		return "", fmt.Errorf("send tokens to program: %w", err)
	}

	//------------------------------------------------------------------------------------------------

	return mintAddress, nil
}

func (sc *TestSolanaChain) runSPLTokenCommandAndWait(
	ctx context.Context,
	provider *solanawallet.Provider,
	args ...string,
) (string, error) {
	var b bytes.Buffer

	if err := RunCommand(ResolveSPLTokenBinary(), args, io.MultiWriter(os.Stdout, &b)); err != nil {
		return "", err
	}

	output := b.String()

	reSignature := regexp.MustCompile(`Signature:\s*(\S+)`)

	signatureMatch := reSignature.FindStringSubmatch(output)
	if signatureMatch != nil {
		if err := provider.WaitForSignature(
			ctx,
			solana.MustSignatureFromBase58(signatureMatch[1]),
			rpc.CommitmentConfirmed,
			MaxConfirmationWaitTime,
		); err != nil {
			return output, fmt.Errorf("wait for signature confirmation: %w", err)
		}
	}

	return output, nil
}

func parseSPLTokenMintAddress(output string) (string, error) {
	reAddress := regexp.MustCompile(`Address:\s*(\S+)`)

	addressMatch := reAddress.FindStringSubmatch(output)
	if addressMatch != nil {
		return addressMatch[1], nil
	}

	reCreatingToken := regexp.MustCompile(`Creating token\s+(\S+)`)

	creatingTokenMatch := reCreatingToken.FindStringSubmatch(output)
	if creatingTokenMatch != nil {
		return creatingTokenMatch[1], nil
	}

	return "", fmt.Errorf("mint address not found in spl-token output")
}

func (sc *TestSolanaChain) registerTokens() error {
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

	for tokenID, tokenName := range sc.config.LockUnlockTokens {
		if tokenName == SOLANATokenName {
			continue
		}

		tokenMint, ok := sc.config.TokensMint[tokenID]
		if !ok {
			return fmt.Errorf("token %d not found in configured mints", tokenID)
		}

		params := []string{
			"deploy-solana",
			"register-lock-unlock-token",
			"--url", sc.jsonRPCAddr,
			"--program-id", sc.programID,
			"--admin-key", adminPkFile.Name(),
			"--treasury-address", sc.config.TreasuryAddress.String(),
			"--token-id", strconv.Itoa(int(tokenID)),
			"--token-mint", tokenMint,
			"--min-bridging-amount", strconv.Itoa(int(sc.config.MinTokenBridgingAmount.Uint64())),
			"--confirmation-timeout-seconds", strconv.Itoa(int(MaxConfirmationWaitTime.Seconds())),
		}

		var b bytes.Buffer

		if err := RunCommand(ResolveApexBridgeBinary(), params, io.MultiWriter(os.Stdout, &b)); err != nil {
			return fmt.Errorf("register lock/unlock token %d (%s): %w", tokenID, tokenName, err)
		}

		fmt.Printf("registered token id %d (%s) with mint %s (lock/unlock)\n", tokenID, tokenName, tokenMint)
	}

	for tokenID, tokenName := range sc.config.MintableTokens {
		params := []string{
			"deploy-solana",
			"register-mint-burn-token",
			"--url", sc.jsonRPCAddr,
			"--program-id", sc.programID,
			"--admin-key", adminPkFile.Name(),
			"--treasury-address", sc.config.TreasuryAddress.String(),
			"--token-id", strconv.Itoa(int(tokenID)),
			"--token-name", tokenName,
			"--token-symbol", tokenName,
			"--token-uri", "",
			"--token-decimals", strconv.Itoa(int(solana.SolDecimals)),
			"--min-bridging-amount", strconv.Itoa(int(sc.config.MinTokenBridgingAmount.Uint64())),
			"--confirmation-timeout-seconds", strconv.Itoa(int(MaxConfirmationWaitTime.Seconds())),
		}

		var b bytes.Buffer

		if err := RunCommand(ResolveApexBridgeBinary(), params, io.MultiWriter(os.Stdout, &b)); err != nil {
			return fmt.Errorf("register mint/burn token %d (%s): %w", tokenID, tokenName, err)
		}

		output := b.String()

		reMintAddress := regexp.MustCompile(`Mint address:\s*(\S+)`)

		mintAddressMatch := reMintAddress.FindStringSubmatch(output)
		if mintAddressMatch == nil {
			return fmt.Errorf("mint address not found in output: %s", output)
		}

		mintAddress := mintAddressMatch[1]

		sc.config.TokensMint[tokenID] = mintAddress

		fmt.Printf("registered token id %d (%s) with mint %s (mint/burn)\n",
			tokenID, tokenName, mintAddress)
	}

	return nil
}

func (sc *TestSolanaChain) initializeALT() error {
	// 1. Create ALT
	adminPkFile, err := os.CreateTemp(os.TempDir(), "admin-pk-*.json")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}

	pkString := fmt.Sprintf("%v", []byte(sc.admin.PrivateKey))
	pkString = strings.ReplaceAll(pkString, " ", ",")

	if _, err := adminPkFile.Write([]byte(pkString)); err != nil {
		return fmt.Errorf("write admin private key: %w", err)
	}

	defer adminPkFile.Close()

	createALTParams := []string{
		"deploy-solana",
		"create-alt",
		"--url", sc.jsonRPCAddr,
		"--program-id", sc.programID,
		"--admin-key", adminPkFile.Name(),
		"--confirmation-timeout-seconds", strconv.Itoa(int(MaxConfirmationWaitTime.Seconds())),
	}

	var createOut bytes.Buffer

	if err := RunCommand(
		ResolveApexBridgeBinary(),
		createALTParams,
		io.MultiWriter(os.Stdout, &createOut),
	); err != nil {
		return err
	}

	createOutString := createOut.String()

	reALTPubKey := regexp.MustCompile(`ALT created:\s*(\S+)`)

	altPubKeyMatch := reALTPubKey.FindStringSubmatch(createOutString)
	if altPubKeyMatch == nil {
		return fmt.Errorf("ALT address not found in output: %s", createOutString)
	}

	altPubKeyStr := altPubKeyMatch[1]

	fmt.Printf("created ALT with public key: %s\n", altPubKeyStr)

	sc.altPublicKey = altPubKeyStr

	// 2. Extend ALT

	extendALTParams := []string{
		"deploy-solana",
		"extend-alt",
		"--url", sc.jsonRPCAddr,
		"--program-id", sc.programID,
		"--admin-key", adminPkFile.Name(),
		"--alt-address", sc.altPublicKey,
		"--confirmation-timeout-seconds", strconv.Itoa(int(MaxConfirmationWaitTime.Seconds())),
	}

	for tokenID, tokenMint := range sc.config.TokensMint {
		extendALTParams = append(extendALTParams, "--token-id-and-mint", fmt.Sprintf("%d:%s", tokenID, tokenMint))
	}

	var extendOut bytes.Buffer

	if err := RunCommand(
		ResolveApexBridgeBinary(),
		extendALTParams,
		io.MultiWriter(os.Stdout, &extendOut),
	); err != nil {
		return err
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
	solFundAmount = solFundAmount.Add(solFundAmount, WeiToLamport(sc.config.InitialHotWalletAmount))

	if err := sc.airdropSOL(ctx, provider, sc.admin.PublicKey.String(), solFundAmount); err != nil {
		return fmt.Errorf("airdrop SOL to admin: %w", err)
	}

	wrapSolAmount := solFundAmount.Mul(
		WeiToLamport(sc.config.FundAmount),
		big.NewInt(int64(len(sc.config.PreminesAddresses)+1)),
	)
	wrapSolAmount = wrapSolAmount.Add(wrapSolAmount, WeiToLamport(sc.config.InitialHotWalletAmount))

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
		"--alt-public-key", sc.altPublicKey,
		// "--sol-tracker-start-block", fmt.Sprintf("%d:%d", 300, 300), // slot:blockNum
		"--sol-confirmation-timeout", "60000000000",
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

	return map[string]*big.Int{carwallet.AdaTokenName: LamportToWei(big.NewInt(int64(balance)))}, nil
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
		// Missing ATA means this wallet does not hold this token yet.
		if strings.Contains(err.Error(), "could not find account") {
			return map[string]*big.Int{tokenName: big.NewInt(0)}, nil
		}

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
	return sc.config.TokensMint
}

// GetRelayerAddress implements ITestApexChain.
func (sc *TestSolanaChain) GetRelayerAddress() string {
	return sc.relayerAddr
}

// GetServerMust implements ITestApexChain.
func (sc *TestSolanaChain) GetServerMust(t *testing.T, indx int) ITestApexChainServer {
	t.Helper()

	require.True(t, sc.cluster != nil && sc.cluster.Servers != nil && len(sc.cluster.Servers) > indx)

	return sc.cluster.Servers[indx]
}

func (sc *TestSolanaChain) InitContracts(
	ctx context.Context, bridgeAdmin *crypto.ECDSAKey, bridgeURL string, chainIDsConfig string) error {
	return nil
}

func (sc *TestSolanaChain) PopulateApexSystem(t *testing.T, apexSystem *ApexSystem) error {
	t.Helper()

	apexSystem.SolanaInfo.RelayerAddress = sc.relayerAddr

	return nil
}

func (sc *TestSolanaChain) RegisterChain(validator *TestApexValidator) error {
	return validator.RegisterChain(
		sc.ChainID(), big.NewInt(0), big.NewInt(0), ChainTypeSolana)
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

func (sc *TestSolanaChain) CheckTxFee(ctx context.Context, txHash string) error {
	txProvider, err := sc.GetTxProvider()
	if err != nil {
		return fmt.Errorf("get tx provider: %w", err)
	}

	signature, err := solana.SignatureFromBase58(txHash)
	if err != nil {
		return fmt.Errorf("parse signature: %w", err)
	}

	tx, err := txProvider.GetTransaction(ctx, signature)
	if err != nil {
		return fmt.Errorf("get transaction: %w", err)
	}

	fmt.Println("SOLANA TX FEE: ", tx.Meta.Fee)

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

	txSender := solsendtx.NewTxSender(txProvider, &solsendtx.ChainConfig{
		TreasuryAddress: sc.config.TreasuryAddress,
	})

	var lastSig string

	for _, receiver := range receivers {
		for _, nativeToken := range receiver.NativeTokens {
			if nativeToken.Amount == nil || nativeToken.Amount.Sign() <= 0 {
				continue
			}

			// Check sender's token balance before attempting wSOL/SPL transfer
			if bal, err := sc.GetAddressBalanceWithTokenName(ctx, wallet.PublicKey.String(),
				nativeToken.PolicyID); err != nil {
				fmt.Printf("sender %s token balance (mint %s): failed to query: %v\n",
					wallet.PublicKey.String(),
					nativeToken.PolicyID,
					err,
				)
			} else {
				fmt.Printf("sender %s token balance (mint %s): %s (attempting to send %s wei, %s lamports)\n",
					wallet.PublicKey.String(),
					nativeToken.PolicyID,
					bal[nativeToken.PolicyID].String(),
					nativeToken.Amount.String(),
					WeiToLamport(nativeToken.Amount).String())
			}

			// Create receiver ATA in a separate confirmed transaction before transferring.
			// Combining CreateATA + Transfer in one tx fails during simulation because the runtime
			// preloads accounts before instructions run, so the destination ATA appears as "not found".
			if err := sc.ensureReceiverTokenAccount(ctx, txProvider, txSender, wallet,
				receiver.Addr, nativeToken.PolicyID); err != nil {
				return "", fmt.Errorf("ensure receiver ATA for %s mint %s: %w", receiver.Addr, nativeToken.PolicyID, err)
			}

			sig, err := splTokenTransfer(ctx, txProvider, txSender, wallet, receiver.Addr, nativeToken)
			if err != nil {
				return "", fmt.Errorf("spl token transfer to %s mint %s: %w", receiver.Addr, nativeToken.PolicyID, err)
			}

			lastSig = sig
		}

		if receiver.Amount != nil && receiver.Amount.Sign() > 0 {
			sig, err := tokenTransfer(ctx, txProvider, txSender, wallet, receiver)
			if err != nil {
				return "", fmt.Errorf("sol transfer to %s: %w", receiver.Addr, err)
			}

			lastSig = sig
		}
	}

	return lastSig, nil
}

// FundUserWithToken funds a user with an SPL token by token mode:
//   - MintableTokens: mint new supply directly to user.
//   - LockUnlockTokens: transfer from admin's pre-minted fixed supply.
func (sc *TestSolanaChain) FundUserWithToken(
	ctx context.Context,
	address string,
	amount *big.Int,
	tokenID uint16,
) error {
	if amount == nil || amount.Sign() <= 0 {
		return fmt.Errorf("amount must be greater than zero")
	}

	if tokenID == SOLTokenID {
		provider, err := sc.GetTxProvider()
		if err != nil {
			return fmt.Errorf("get tx provider: %w", err)
		}

		return sc.airdropSOL(ctx, provider, address, amount)
	}

	if tokenID == WSOLTokenID {
		provider, err := sc.GetTxProvider()
		if err != nil {
			return fmt.Errorf("get tx provider: %w", err)
		}

		err = sc.airdropSOL(ctx, provider, address, amount)
		if err != nil {
			return fmt.Errorf("airdrop SOL: %w", err)
		}

		wallet, err := solanawallet.NewWalletFromPrivateKey(address)
		if err != nil {
			return fmt.Errorf("new wallet from private key: %w", err)
		}

		return sc.wrapSOL(ctx, provider, wallet, amount)
	}

	tokenMint, ok := sc.config.TokensMint[tokenID]
	if !ok || tokenMint == "" {
		return fmt.Errorf("token with ID %d not found in configured mints", tokenID)
	}

	lamportAmount := WeiToLamport(amount)
	if !lamportAmount.IsUint64() {
		return fmt.Errorf("amount too large to fit uint64: %s", lamportAmount.String())
	}

	if _, isMintable := sc.config.MintableTokens[tokenID]; isMintable {
		provider, err := sc.GetTxProvider()
		if err != nil {
			return fmt.Errorf("get tx provider: %w", err)
		}

		txSender := solsendtx.NewTxSender(provider, &solsendtx.ChainConfig{
			TreasuryAddress: sc.config.TreasuryAddress,
		})

		// Ensure receiver ATA exists before minting to avoid AccountNotFound/AccountNotInitialized errors.
		if err := sc.ensureReceiverTokenAccount(ctx, provider, txSender, sc.admin, address, tokenMint); err != nil {
			return fmt.Errorf("ensure receiver ATA for mintable token %d: %w", tokenID, err)
		}

		receiverPubKey, err := solanawallet.PublicKeyFromAddress(address)
		if err != nil {
			return fmt.Errorf("parse receiver address: %w", err)
		}

		mintPubKey, err := solanawallet.PublicKeyFromAddress(tokenMint)
		if err != nil {
			return fmt.Errorf("parse token mint address: %w", err)
		}

		receiverATA, _, err := solanawallet.FindAssociatedTokenAddress(receiverPubKey, mintPubKey)
		if err != nil {
			return fmt.Errorf("find receiver ATA: %w", err)
		}

		adminPkFile, err := os.CreateTemp(os.TempDir(), "solana-admin-pk-*.json")
		if err != nil {
			return fmt.Errorf("create temp keypair file: %w", err)
		}

		defer os.Remove(adminPkFile.Name())
		defer adminPkFile.Close()

		pkString := fmt.Sprintf("%v", []byte(sc.admin.PrivateKey))
		pkString = strings.ReplaceAll(pkString, " ", ",")

		if _, err := adminPkFile.Write([]byte(pkString)); err != nil {
			return fmt.Errorf("write keypair file: %w", err)
		}

		if err := adminPkFile.Sync(); err != nil {
			return fmt.Errorf("sync keypair file: %w", err)
		}

		args := []string{
			"mint", tokenMint, lamportAmount.String(), receiverATA.String(),
			"--url", sc.jsonRPCAddr,
			"--fee-payer", adminPkFile.Name(),
			"--owner", adminPkFile.Name(),
		}
		if _, err := sc.runSPLTokenCommandAndWait(ctx, provider, args...); err != nil {
			return fmt.Errorf("mint token %d to %s: %w", tokenID, address, err)
		}

		fmt.Printf("minted token %d (mint %s) amount %s to %s\n", tokenID, tokenMint, lamportAmount.String(), address)

		return nil
	}

	if _, isLockUnlock := sc.config.LockUnlockTokens[tokenID]; isLockUnlock {
		_, err := sc.SendTx(
			ctx,
			sc.admin.PrivateKey.String(),
			nil,
			[]GenericTxReceiver{
				{
					Addr:   address,
					Amount: big.NewInt(0),
					NativeTokens: []GenericTokenAmount{
						{
							Token: carwallet.Token{
								PolicyID: tokenMint,
							},
							Amount: amount,
						},
					},
				},
			},
			0,
		)
		if err != nil {
			return fmt.Errorf("transfer lock/unlock token %d to %s: %w", tokenID, address, err)
		}

		fmt.Printf("transferred lock/unlock token %d (mint %s) amount %s to %s\n",
			tokenID, tokenMint, lamportAmount.String(), address)

		return nil
	}

	return fmt.Errorf("token with ID %d is neither mintable nor lock/unlock", tokenID)
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
	receiverAddr string,
	nativeToken GenericTokenAmount,
) (string, error) {
	if nativeToken.Amount == nil || nativeToken.Amount.Sign() <= 0 {
		return "", nil
	}

	lamportAmount := WeiToLamport(nativeToken.Amount)
	if !lamportAmount.IsUint64() {
		return "", fmt.Errorf("spl token amount too large: %s", lamportAmount.String())
	}

	txDto := solsendtx.SPLTransferDto{
		SenderPublicKey:   wallet.PublicKey.String(),
		ReceiverPublicKey: receiverAddr,
		Amount:            lamportAmount.Uint64(),
		MintTokenAddress:  nativeToken.PolicyID,
		TokenDecimals:     solana.SolDecimals,
	}

	recentBlockhash, err := txProvider.GetLatestBlockhash(ctx)
	if err != nil {
		return "", err
	}

	tx, err := txSender.CreateTx(ctx, wallet.PublicKey, solsendtx.InstructionTypeSPLTransfer, recentBlockhash, txDto)
	if err != nil {
		return "", err
	}

	_, err = tx.Sign(func(key solana.PublicKey) *solana.PrivateKey {
		return &wallet.PrivateKey
	})
	if err != nil {
		return "", fmt.Errorf("sign instruction: %w", err)
	}

	sig, err := txSender.SendTx(ctx, tx)
	if err != nil {
		return "", err
	}

	err = txProvider.WaitForSignature(ctx, *sig, rpc.CommitmentConfirmed, MaxConfirmationWaitTime)
	if err != nil {
		return "", fmt.Errorf("wait for token transfer confirmation: %w", err)
	}

	return sig.String(), nil
}

func tokenTransfer(
	ctx context.Context,
	txProvider *solanawallet.Provider,
	txSender *solsendtx.TxSender,
	wallet *solanawallet.Wallet,
	receiver GenericTxReceiver,
) (string, error) {
	if receiver.Amount == nil || receiver.Amount.Cmp(big.NewInt(0)) == 0 {
		return "", nil
	}

	txDto := solsendtx.SOLTransferDto{
		SenderPublicKey:   wallet.PublicKey.String(),
		ReceiverPublicKey: receiver.Addr,
		Amount:            WeiToLamport(receiver.Amount).Uint64(),
	}

	recentBlockhash, err := txProvider.GetLatestBlockhash(ctx)
	if err != nil {
		return "", err
	}

	tx, err := txSender.CreateTx(ctx, wallet.PublicKey, solsendtx.InstructionTypeSOLTransfer, recentBlockhash, txDto)
	if err != nil {
		return "", err
	}

	_, err = tx.Sign(func(key solana.PublicKey) *solana.PrivateKey {
		return &wallet.PrivateKey
	})
	if err != nil {
		return "", fmt.Errorf("sign instruction: %w", err)
	}

	sig, err := txSender.SendTx(ctx, tx)
	if err != nil {
		return "", err
	}

	err = txProvider.WaitForSignature(ctx, *sig, rpc.CommitmentConfirmed, MaxConfirmationWaitTime)
	if err != nil {
		return "", fmt.Errorf("wait for sol transfer confirmation: %w", err)
	}

	return sig.String(), nil
}

func (sc *TestSolanaChain) GetProgramVersion(ctx context.Context) (string, error) {
	txProvider, err := sc.GetTxProvider()
	if err != nil {
		return "", fmt.Errorf("get tx provider: %w", err)
	}

	txSender := solsendtx.NewTxSender(txProvider, &solsendtx.ChainConfig{
		TreasuryAddress: sc.config.TreasuryAddress,
	})

	programConfig, err := txSender.GetProgramConfig(ctx, solana.MustPublicKeyFromBase58(sc.programID))
	if err != nil {
		return "", fmt.Errorf("get program config: %w", err)
	}

	fmt.Println("program config: ", programConfig)

	return programConfig.VersionString, nil
}

func (sc *TestSolanaChain) UpgradeProgram(ctx context.Context) error {
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
		"upgrade-program",
		"--url", sc.jsonRPCAddr,
		"--fee-payer", adminPkFile.Name(),
		"--key", filepath.Join("..", "..", solanaProgramDir, solanaProgramKeypairPath),
		"--build-path", filepath.Join("..", "..", solanaProgramDir, solanaProgramBuildPath),
		"--program-id", sc.programID,
		"--upgrade-program-version", "0.2.0",
		"--admin-key", adminPkFile.Name(),
		"--confirmation-timeout-seconds", strconv.Itoa(int(MaxConfirmationWaitTime.Seconds())),
		"--commitment", "finalized",
	}

	var b bytes.Buffer

	err = RunCommand(ResolveApexBridgeBinary(), params, io.MultiWriter(os.Stdout, &b))
	if err != nil {
		return err
	}

	return nil
}

type UpdateFeeConfigDto struct {
	MinOperationFee *big.Int
	BridgeFee       *big.Int
	UpdateTreasury  bool
	TreasuryAddress string
}

func (sc *TestSolanaChain) UpdateFeeConfig(ctx context.Context, feeConfig UpdateFeeConfigDto) error {
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
		"update-fee-config",
		"--url", sc.jsonRPCAddr,
		"--admin-key", adminPkFile.Name(),
		"--program-id", sc.programID,
		"--min-operation-fee", strconv.Itoa(int(feeConfig.MinOperationFee.Uint64())),
		"--min-fee-for-bridging", strconv.Itoa(int(feeConfig.BridgeFee.Uint64())),
		"--confirmation-timeout-seconds", strconv.Itoa(int(MaxConfirmationWaitTime.Seconds())),
	}

	if feeConfig.UpdateTreasury {
		params = append(params, "--update-treasury", "true")
		params = append(params, "--new-treasury-address", feeConfig.TreasuryAddress)
	}

	var b bytes.Buffer

	err = RunCommand(ResolveApexBridgeBinary(), params, io.MultiWriter(os.Stdout, &b))
	if err != nil {
		return err
	}

	sc.config.MinBridgingFee = feeConfig.BridgeFee
	sc.config.MinOperationFee = feeConfig.MinOperationFee

	if feeConfig.UpdateTreasury {
		sc.config.TreasuryAddress = solana.MustPublicKeyFromBase58(feeConfig.TreasuryAddress)
	}

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
