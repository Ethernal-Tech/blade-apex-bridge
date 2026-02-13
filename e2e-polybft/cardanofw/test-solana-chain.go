package cardanofw

import (
	"context"
	"fmt"
	"math/big"
	"testing"

	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/e2eindexer"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/solanafw"
	"github.com/Ethernal-Tech/cardano-infrastructure/sendtx"
	"github.com/Ethernal-Tech/cardano-infrastructure/wallet"
	infrawallet "github.com/Ethernal-Tech/cardano-infrastructure/wallet"
	"github.com/stretchr/testify/require"
)

type TestSolanaChainConfig struct {
	ChainID   string
	IsEnabled bool

	InitialHotWalletAmount *big.Int
	FundAmount             *big.Int
	StartingPort           int

	MinBridgingFee         *big.Int
	MinBridgingAmount      *big.Int
	MinTokenBridgingAmount *big.Int
	MinOperationFee        *big.Int
	CurrencyID             uint16
}

func NewTestSolanaChainConfig() *TestSolanaChainConfig {
	return &TestSolanaChainConfig{
		ChainID:                ChainIDSolana,
		IsEnabled:              true,
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
	config  *TestSolanaChainConfig
	cluster *solanafw.TestSolanaCluster
	// admin wallet - funded from start
	jsonRPCAddr string
	gatewayAddr string
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

	return &TestSolanaChain{
		config: config,
		// admin wallet
	}, nil
}

// wTODO: Implement this for sending bridging requests to the solana chain
func (sc *TestSolanaChain) BridgingRequest(params BridgingRequestParams) (string, error) {
	panic("unimplemented")
}

// ChainID implements ITestApexChain.
func (sc *TestSolanaChain) ChainID() string {
	return sc.config.ChainID
}

func (sc *TestSolanaChain) CreateAddresses(bladeAdmin *crypto.ECDSAKey, bridgeURL string, chainIDsConfig string) error {
	return nil
}

func (sc *TestSolanaChain) CreateMetadata(senderAddr string, dstChainID string, receivers []sendtx.BridgingTxReceiver, bridgingFee *big.Int, operationFee *big.Int) ([]byte, error) {
	return nil, nil
}

// wTODO: Implement this for creating wallets on the solana chain
func (sc *TestSolanaChain) CreateWallets(validator *TestApexValidator) error {
	panic("unimplemented")
}

func (sc *TestSolanaChain) DeployMintingContract(ctx context.Context, chainIDsConfig string) error {
	// Unnecessary until we need a coloredcoin support
	return nil
}

// wTODO: Implement this for funding wallets on the solana chain
func (sc *TestSolanaChain) FundWallets(ctx context.Context) error {
	panic("unimplemented")
}

// wTODO: Implement this for generating chain configs on the solana chain
// generate-configs solana-chain cli command required
func (sc *TestSolanaChain) GenerateChainConfigs(indx int, validator *TestApexValidator) error {
	panic("unimplemented")
}

// wTODO: Implement this for getting the balance of an address on the solana chain
func (sc *TestSolanaChain) GetAddressBalance(ctx context.Context, addr string) (map[string]*big.Int, error) {
	panic("unimplemented")
}

func (sc *TestSolanaChain) GetAddressBalanceWithTokenName(ctx context.Context, addr string, tokenName string) (map[string]*big.Int, error) {
	panic("unimplemented")
}

func (sc *TestSolanaChain) GetAddressToBridgeTo(ctx context.Context, hasTokens bool) (string, error) {
	return sc.gatewayAddr, nil
}

// wTODO: Implement this for getting the admin private key on the solana chain
func (sc *TestSolanaChain) GetAdminPrivateKey() (string, error) {
	panic("unimplemented") //nolint:gocritic
}

func (sc *TestSolanaChain) GetBridgingFee(
	_ context.Context,
	_ string,
	_ []sendtx.BridgingTxReceiver,
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
) (infrawallet.QueryStakeAddressInfo, error) {
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
	panic("unimplemented") //nolint:gocritic
}

func (sc *TestSolanaChain) GetHotWalletAddresses() []string {
	return []string{sc.gatewayAddr}
}

// GetIndexer implements ITestApexChain.
func (sc *TestSolanaChain) GetIndexer() e2eindexer.TxsExecutedComponent {
	panic("unimplemented") //nolint:gocritic
}

func (sc *TestSolanaChain) GetMintableTokens() map[uint16]string {
	// Unnecessary until we need a coloredcoin support
	return make(map[uint16]string)
}

// GetRelayerAddress implements ITestApexChain.
func (sc *TestSolanaChain) GetRelayerAddress() string {
	panic("unimplemented") //nolint:gocritic
}

// GetServerMust implements ITestApexChain.
func (sc *TestSolanaChain) GetServerMust(t *testing.T, indx int) ITestApexChainServer {
	t.Helper()

	require.True(t, sc.cluster != nil && sc.cluster.Servers != nil && len(sc.cluster.Servers) > indx)

	return sc.cluster.Servers[indx]
}

// wTODO: Implement this for initializing solana contract
func (sc *TestSolanaChain) InitContracts(ctx context.Context, bridgeAdmin *crypto.ECDSAKey, bridgeURL string, chainIDsConfig string) error {
	panic("unimplemented") //nolint:gocritic
}

// wTODO: Implement this when SolanaInfo is added to the ApexSystem
func (*TestSolanaChain) PopulateApexSystem(t *testing.T, apexSystem *ApexSystem) error {
	panic("unimplemented") //nolint:gocritic
}

func (sc *TestSolanaChain) RegisterChain(validator *TestApexValidator) error {
	return validator.RegisterChain(
		sc.ChainID(), sc.config.InitialHotWalletAmount, big.NewInt(0), ChainTypeSolana)
}

// RunChain implements ITestApexChain.
func (sc *TestSolanaChain) RunChain(t *testing.T) error {
	t.Helper()

	cluster, err := solanafw.NewSolanaTestCluster(
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
	panic("unimplemented") //nolint:gocritic
}

func (sc *TestSolanaChain) SetCustodialNFT(token wallet.Token) {
	panic("unimplemented") //nolint:gocritic
}

func (sc *TestSolanaChain) Stop() error {
	if sc.cluster != nil {
		return sc.cluster.Stop()
	}

	return nil
}

func (sc *TestSolanaChain) UpdateTxSendChainConfiguration(_ map[string]sendtx.ChainConfig) {
}
