package cardanofw

import (
	"context"
	"math/big"
	"testing"

	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/e2eindexer"
	"github.com/hashicorp/go-hclog"
)

type ITestApexChainServer interface {
	Stop(removeDB ...bool) error
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
	PopulateApexSystem(apexSystem *ApexSystem)
	ChainID() string
	GetAddressBalance(ctx context.Context, addr string) (*big.Int, error)
	BridgingRequest(
		ctx context.Context, destChainID ChainID, privateKey string, receivers map[string]*big.Int,
		feeAmount *big.Int, txsExecutedComponent e2eindexer.TxsExecutedComponent,
	) (string, error)
	SendTx(
		ctx context.Context, privateKey string, receiver string, amount *big.Int, data []byte,
	) (string, error)
	GetHotWalletAddress() string
	GetAdminPrivateKey() (string, error)
	GetServerMust(t *testing.T, indx int) ITestApexChainServer
	CreateIndexer(logger hclog.Logger) (e2eindexer.TxsExecutedComponent, error)
}

type TestApexChainDummy struct {
	configParams []string
}

func NewTestApexChainDummy(configParams []string) *TestApexChainDummy {
	return &TestApexChainDummy{
		configParams: configParams,
	}
}

func (td *TestApexChainDummy) BridgingRequest(
	ctx context.Context, destChainID string, privateKey string, receivers map[string]*big.Int,
	feeAmount *big.Int, _ e2eindexer.TxsExecutedComponent,
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

func (td *TestApexChainDummy) GetAddressBalance(ctx context.Context, addr string) (*big.Int, error) {
	return nil, nil
}

func (td *TestApexChainDummy) GetGenerateConfigsParams(indx int) []string {
	return td.configParams
}

func (td *TestApexChainDummy) InitContracts(ctx context.Context, bridgeAdmin *crypto.ECDSAKey, bridgeURL string) error {
	return nil
}

func (td *TestApexChainDummy) PopulateApexSystem(apexSystem *ApexSystem) {
}

func (td *TestApexChainDummy) RegisterChain(validator *TestApexValidator) error {
	return nil
}

func (*TestApexChainDummy) RunChain(t *testing.T) error {
	t.Helper()

	return nil
}

func (td *TestApexChainDummy) SendTx(
	ctx context.Context, privateKey string, receiver string, amount *big.Int, data []byte,
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

func (td *TestApexChainDummy) GetServerMust(t *testing.T, indx int) ITestApexChainServer {
	t.Helper()
	t.Fail()

	return nil
}

func (td *TestApexChainDummy) CreateIndexer(logger hclog.Logger) (e2eindexer.TxsExecutedComponent, error) {
	return e2eindexer.NewTxsExecutedComponentDummy(), nil
}

var _ ITestApexChain = (*TestApexChainDummy)(nil)
