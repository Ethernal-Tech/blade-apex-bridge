package cardanofw

import (
	"context"
	"fmt"
	"math/big"
	"testing"

	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/e2eindexer"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/solanafw"
	carsendtx "github.com/Ethernal-Tech/cardano-infrastructure/sendtx"
	carwallet "github.com/Ethernal-Tech/cardano-infrastructure/wallet"
	solanawallet "github.com/Ethernal-Tech/solana-infrastructure/wallet"
	"github.com/stretchr/testify/require"
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
}

func NewSolanaChainConfig(enabled bool) *TestSolanaChainConfig {
	return &TestSolanaChainConfig{
		ChainID:                ChainIDSolana,
		IsEnabled:              enabled,
		StartingPort:           8899,
		InitialHotWalletAmount: big.NewInt(0),
		FundAmount:             big.NewInt(0),
		MinBridgingFee:         big.NewInt(0),
		MinBridgingAmount:      big.NewInt(0),
		MinTokenBridgingAmount: big.NewInt(0),
		MinOperationFee:        big.NewInt(0),
		CurrencyID:             SOLTokenID,
	}
}

type TestSolanaChain struct {
	config      *TestSolanaChainConfig
	cluster     *solanafw.TestSolanaCluster
	admin       *solanawallet.Wallet
	jsonRPCAddr string
	gatewayAddr string
	indexer     e2eindexer.TxsExecutedComponent
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
	return solanawallet.NewProvider(sc.jsonRPCAddr), nil
}

// wTODO: Implement this for sending bridging requests to the solana chain
func (sc *TestSolanaChain) BridgingRequest(params BridgingRequestParams) (string, error) {
	panic("unimplemented") //nolint:gocritic
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

// wTODO: Implement this for creating wallets on the solana chain
func (sc *TestSolanaChain) CreateWallets(validator *TestApexValidator) error {
	return nil
}

func (sc *TestSolanaChain) DeployMintingContract(ctx context.Context, chainIDsConfig string) error {
	return nil
}

// wTODO: Implement this for funding wallets on the solana chain
func (sc *TestSolanaChain) FundWallets(ctx context.Context) error {
	return nil
}

// wTODO: Implement this for generating chain configs on the solana chain
// generate-configs solana-chain cli command required
func (sc *TestSolanaChain) GenerateChainConfigs(indx int, validator *TestApexValidator) error {
	return nil
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

	return map[string]*big.Int{addr: SolanaToWei(big.NewInt(int64(balance)))}, nil
}

func (sc *TestSolanaChain) GetAddressBalanceWithTokenName(
	ctx context.Context, addr string, tokenName string) (map[string]*big.Int, error) {
	return nil, nil
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
	// return validator.RegisterChain(
	// 	sc.ChainID(), sc.config.InitialHotWalletAmount, big.NewInt(0), ChainTypeSolana)
	return nil
}

// RunChain implements ITestApexChain.
func (sc *TestSolanaChain) RunChain(t *testing.T) error {
	t.Helper()

	cluster, err := solanafw.NewSolanaTestCluster(t,
		solanafw.WithPremine(sc.admin.PublicKey.String()),
		solanafw.WithPremine(sc.config.PreminesAddresses...),
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

// wTODO: Implement this for sending a transaction to the solana chain
func (sc *TestSolanaChain) SendTx(ctx context.Context,
	privateKey string, metadata []byte, receivers []GenericTxReceiver) (string, error) {
	return "", nil
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
