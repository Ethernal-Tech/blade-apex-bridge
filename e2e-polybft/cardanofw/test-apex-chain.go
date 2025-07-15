package cardanofw

import (
	"context"
	"math/big"
	"testing"

	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/Ethernal-Tech/cardano-infrastructure/sendtx"
	infrawallet "github.com/Ethernal-Tech/cardano-infrastructure/wallet"
)

type ITestApexChainServer interface {
	Stop() error
	Start() error
}

type ITestApexChain interface {
	RunChain(t *testing.T) error
	Stop() error
	CreateWallets(validator *TestApexValidator) error
	CreateAddresses(bladeAdmin *crypto.ECDSAKey, bridgeURL string) error
	FundWallets(ctx context.Context) error
	RegisterChain(validator *TestApexValidator) error
	InitContracts(ctx context.Context, bridgeAdmin *crypto.ECDSAKey, bridgeURL string) error
	GetGenerateConfigsParams(indx int) []string
	PopulateApexSystem(t *testing.T, apexSystem *ApexSystem)
	UpdateTxSendChainConfiguration(configs map[string]sendtx.ChainConfig)
	ChainID() string
	GetAddressBalance(ctx context.Context, addr string) (map[string]*big.Int, error)
	BridgingRequest(
		ctx context.Context,
		destChainID ChainID,
		privateKey string,
		receivers map[string]*big.Int,
		feeAmount *big.Int,
		operationFee uint64,
		bridgingTypes ...sendtx.BridgingType,
	) (string, error)
	SendTx(
		ctx context.Context, privateKey string, receiver string,
		amount *big.Int, nativeTokenAmounts []infrawallet.TokenAmount, data []byte,
	) (string, error)
	GetHotWalletAddress() string
	GetAdminPrivateKey() (string, error)
	// on skyline, txSender will in some cases correct the bridging fee based on the calculated min utxo
	GetBridgingFee(
		ctx context.Context,
		dstChainID string,
		receivers []sendtx.BridgingTxReceiver,
		bridgingFee uint64,
		operationFee uint64,
	) (uint64, error)
	CreateMetadata(
		senderAddr string,
		dstChainID string,
		receivers []sendtx.BridgingTxReceiver,
		bridgingFee uint64,
		operationFee uint64,
	) ([]byte, error)
	GetServerMust(t *testing.T, indx int) ITestApexChainServer
	GetExistingStakePools(t *testing.T, ctx context.Context) []string
	GetBridgingStakeAddressInfo(
		t *testing.T,
		ctx context.Context,
		indx uint8,
	) infrawallet.QueryStakeAddressInfo
}

type TestApexChainDummy struct {
	configParams []string
}

// GetBridgingStakeAddressInfo implements ITestApexChain.
func (td *TestApexChainDummy) GetBridgingStakeAddressInfo(
	t *testing.T, ctx context.Context, indx uint8,
) infrawallet.QueryStakeAddressInfo {
	t.Helper()

	return infrawallet.QueryStakeAddressInfo{}
}

// GetExistingStakePools implements ITestApexChain.
func (td *TestApexChainDummy) GetExistingStakePools(t *testing.T, ctx context.Context) []string {
	t.Helper()

	return []string{}
}

func NewTestApexChainDummy(configParams []string) *TestApexChainDummy {
	return &TestApexChainDummy{
		configParams: configParams,
	}
}

func (td *TestApexChainDummy) BridgingRequest(
	ctx context.Context,
	destChainID string,
	privateKey string,
	receivers map[string]*big.Int,
	feeAmount *big.Int,
	operationFee uint64,
	bridgingTypes ...sendtx.BridgingType,
) (string, error) {
	return "", nil
}

func (td *TestApexChainDummy) ChainID() string {
	return ""
}

func (td *TestApexChainDummy) CreateAddresses(bladeAdmin *crypto.ECDSAKey, bridgeURL string) error {
	return nil
}

func (td *TestApexChainDummy) CreateWallets(validator *TestApexValidator) error {
	return nil
}

func (td *TestApexChainDummy) FundWallets(ctx context.Context) error {
	return nil
}

func (td *TestApexChainDummy) GetAddressBalance(ctx context.Context, addr string) (map[string]*big.Int, error) {
	return nil, nil
}

func (td *TestApexChainDummy) GetGenerateConfigsParams(indx int) []string {
	return td.configParams
}

func (td *TestApexChainDummy) InitContracts(ctx context.Context, bridgeAdmin *crypto.ECDSAKey, bridgeURL string) error {
	return nil
}

func (*TestApexChainDummy) PopulateApexSystem(t *testing.T, apexSystem *ApexSystem) {
	t.Helper()
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
	ctx context.Context, privateKey string, receiver string,
	amount *big.Int, nativeTokenAmounts []infrawallet.TokenAmount, data []byte,
) (string, error) {
	return "", nil
}

func (td *TestApexChainDummy) Stop() error {
	return nil
}

func (td *TestApexChainDummy) GetHotWalletAddress() string {
	return ""
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

var _ ITestApexChain = (*TestApexChainDummy)(nil)
