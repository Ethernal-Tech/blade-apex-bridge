package cardanofw

import (
	"context"
	"math/big"
	"testing"

	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/e2eindexer"
	"github.com/Ethernal-Tech/cardano-infrastructure/sendtx"
	infrawallet "github.com/Ethernal-Tech/cardano-infrastructure/wallet"
)

type ITestApexChainServer interface {
	Stop(removeDB ...bool) error
	Start() error
}

type GenericTxReceiver struct {
	Addr         string
	Amount       *big.Int
	NativeTokens []infrawallet.TokenAmount
}

type ReceiverAmount struct {
	TokenID uint16
	Amount  *big.Int
}

type BridgingRequestParams struct {
	Ctx            context.Context
	DestChainID    ChainID
	PrivateKey     string
	ChainIDsConfig string
	Receivers      map[string]ReceiverAmount
	FeeAmount      *big.Int
	OperationFee   uint64
	IsCurrency     bool
}

type ITestApexChain interface {
	RunChain(t *testing.T) error
	Stop() error
	CreateWallets(validator *TestApexValidator) error
	CreateAddresses(bladeAdmin *crypto.ECDSAKey, bridgeURL, chainIDsConfig string) error
	FundWallets(ctx context.Context) error
	RegisterChain(validator *TestApexValidator) error
	InitContracts(ctx context.Context, bridgeAdmin *crypto.ECDSAKey, bridgeURL, chainIDsConfig string) error
	GenerateChainConfigs(
		indx int,
		validator *TestApexValidator,
	) error
	PopulateApexSystem(t *testing.T, apexSystem *ApexSystem) error
	UpdateTxSendChainConfiguration(configs map[string]sendtx.ChainConfig)
	DeployMintingContract(ctx context.Context, chainIDsConfig string) error
	ChainID() string
	GetAddressBalance(ctx context.Context, addr string) (map[string]*big.Int, error)
	GetAddressBalanceWithTokenName(ctx context.Context, addr string, tokenName string) (map[string]*big.Int, error)
	BridgingRequest(params BridgingRequestParams) (string, error)
	SendTx(
		ctx context.Context, privateKey string, metadata []byte, receivers []GenericTxReceiver,
	) (string, error)
	GetHotWalletAddresses() []string
	GetAdminPrivateKey() (string, error)
	// on skyline, txSender will in some cases correct the bridging fee based on the calculated min utxo
	GetBridgingFee(
		ctx context.Context,
		dstChainID string,
		receivers []sendtx.BridgingTxReceiver,
		bridgingFee uint64,
		operationFee uint64,
		multiSigAddr string,
	) (uint64, error)
	CreateMetadata(
		senderAddr string,
		dstChainID string,
		receivers []sendtx.BridgingTxReceiver,
		bridgingFee uint64,
		operationFee uint64,
	) ([]byte, error)
	GetServerMust(t *testing.T, indx int) ITestApexChainServer
	GetIndexer() e2eindexer.TxsExecutedComponent
	GetExistingStakePools(t *testing.T, ctx context.Context) []string
	GetBridgingStakeAddressInfo(
		t *testing.T,
		ctx context.Context,
		indx uint8,
		expectError bool,
	) (infrawallet.QueryStakeAddressInfo, error)
	GetAddressToBridgeTo(ctx context.Context, hasTokens bool) (string, error)
	GetMintableTokens() map[uint16]string
	GetCardanoScriptInfo() *CardanoScriptInfo
	GetRelayerAddress() string
	GetCustodialAddress() string
	SetCustodialNFT(token infrawallet.Token)
}

type TestApexChainDummy struct {
	configParams []string
	indexer      e2eindexer.TxsExecutedComponent
}

// GetBridgingStakeAddressInfo implements ITestApexChain.
func (td *TestApexChainDummy) GetBridgingStakeAddressInfo(
	t *testing.T, ctx context.Context, indx uint8, expectError bool,
) (infrawallet.QueryStakeAddressInfo, error) {
	t.Helper()

	return infrawallet.QueryStakeAddressInfo{}, nil
}

// GetExistingStakePools implements ITestApexChain.
func (td *TestApexChainDummy) GetExistingStakePools(t *testing.T, ctx context.Context) []string {
	t.Helper()

	return []string{}
}

func NewTestApexChainDummy(configParams []string) *TestApexChainDummy {
	return &TestApexChainDummy{
		configParams: configParams,
		indexer:      e2eindexer.NewTxsExecutedComponentDummy(),
	}
}

func (td *TestApexChainDummy) BridgingRequest(params BridgingRequestParams) (string, error) {
	return "", nil
}

func (td *TestApexChainDummy) ChainID() string {
	return ""
}

func (td *TestApexChainDummy) CreateAddresses(bladeAdmin *crypto.ECDSAKey, bridgeURL, chainIDsConfig string) error {
	return nil
}

func (td *TestApexChainDummy) CreateWallets(validator *TestApexValidator) error {
	return nil
}

func (td *TestApexChainDummy) DeployMintingContract(ctx context.Context, chainIDsConfig string) error {
	return nil
}

func (td *TestApexChainDummy) FundWallets(ctx context.Context) error {
	return nil
}

func (td *TestApexChainDummy) GetAddressBalance(ctx context.Context, addr string) (map[string]*big.Int, error) {
	return nil, nil
}

func (td *TestApexChainDummy) GenerateChainConfigs(
	indx int,
	validator *TestApexValidator) error {
	return nil
}

func (td *TestApexChainDummy) InitContracts(ctx context.Context, bridgeAdmin *crypto.ECDSAKey, bridgeURL, chainIDsConfig string) error {
	return nil
}

func (*TestApexChainDummy) PopulateApexSystem(t *testing.T, apexSystem *ApexSystem) error {
	t.Helper()

	return nil
}

func (td *TestApexChainDummy) UpdateTxSendChainConfiguration(_ map[string]sendtx.ChainConfig) {
}

func (td *TestApexChainDummy) RegisterChain(validator *TestApexValidator) error {
	return nil
}

func (*TestApexChainDummy) RunChain(t *testing.T) error {
	t.Helper()

	return nil
}

func (td *TestApexChainDummy) SendTx(
	ctx context.Context, privateKey string, metadata []byte, receivers []GenericTxReceiver,
) (string, error) {
	return "", nil
}

func (td *TestApexChainDummy) Stop() error {
	return nil
}

func (td *TestApexChainDummy) GetHotWalletAddresses() []string {
	return nil
}

func (td *TestApexChainDummy) GetAdminPrivateKey() (string, error) {
	return "", nil
}

func (td *TestApexChainDummy) GetBridgingFee(
	ctx context.Context,
	dstChainID string,
	receivers []sendtx.BridgingTxReceiver,
	bridgingFee uint64,
	operationFee uint64,
	multiSigAddr string,
) (uint64, error) {
	return 0, nil
}

func (td *TestApexChainDummy) CreateMetadata(
	senderAddr string,
	dstChainID string,
	receivers []sendtx.BridgingTxReceiver,
	bridgingFee uint64,
	operationFee uint64,
) ([]byte, error) {
	return nil, nil
}

func (td *TestApexChainDummy) GetServerMust(t *testing.T, indx int) ITestApexChainServer {
	t.Helper()
	t.Fail()

	return nil
}

func (td *TestApexChainDummy) GetIndexer() e2eindexer.TxsExecutedComponent {
	return td.indexer
}

func (td *TestApexChainDummy) GetAddressToBridgeTo(
	ctx context.Context,
	hasTokens bool,
) (string, error) {
	return "", nil
}

// GetMintableTokens implements ITestApexChain.
func (td *TestApexChainDummy) GetMintableTokens() map[uint16]string {
	return make(map[uint16]string)
}

// GetRelayerAddress implements ITestApexChain.
func (td *TestApexChainDummy) GetRelayerAddress() string {
	return ""
}

// GetCustodialAddress implements ITestApexChain.
func (td *TestApexChainDummy) GetCustodialAddress() string {
	return ""
}

// SetCustodialNFT implements ITestApexChain.
func (td *TestApexChainDummy) SetCustodialNFT(token infrawallet.Token) {}

// GetCardanoScriptInfo implements ITestApexChain.
func (td *TestApexChainDummy) GetCardanoScriptInfo() *CardanoScriptInfo {
	return &CardanoScriptInfo{}
}

func (td *TestApexChainDummy) GetAddressBalanceWithTokenName(
	ctx context.Context, addr string, tokenName string) (map[string]*big.Int, error) {
	return nil, nil
}

var _ ITestApexChain = (*TestApexChainDummy)(nil)

func createTxReceiver(
	addr string, amount *big.Int, token *infrawallet.Token, tokenAmount *big.Int,
) GenericTxReceiver {
	var nativeTokens []infrawallet.TokenAmount

	if token != nil && tokenAmount != nil && tokenAmount.BitLen() != 0 {
		nativeTokens = []infrawallet.TokenAmount{
			{
				Token:  *token,
				Amount: tokenAmount.Uint64(),
			},
		}
	}

	return GenericTxReceiver{
		Addr:         addr,
		Amount:       amount,
		NativeTokens: nativeTokens,
	}
}
