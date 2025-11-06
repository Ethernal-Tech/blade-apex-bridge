package cardanofw

import (
	"context"
	"math/big"
	"testing"

	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/e2eindexer"
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
	GenerateChainConfigs(indx int, validator *TestApexValidator) error
	PopulateApexSystem(apexSystem *ApexSystem) error
	ChainID() string
	GetAddressBalance(ctx context.Context, addr string) (*big.Int, error)
	BridgingRequest(
		ctx context.Context, destChainID ChainID, privateKey string, receivers map[string]*big.Int, feeAmount *big.Int,
	) (string, error)
	SendTx(
		ctx context.Context, privateKey string, receiver string, amount *big.Int, data []byte,
	) (string, error)
	GetHotWalletAddress() string
	GetAdminPrivateKey() (string, error)
	GetServerMust(t *testing.T, indx int) ITestApexChainServer
	GetIndexer() e2eindexer.TxsExecutedComponent
}

type TestApexChainDummy struct {
	configParams []string
	indexer      e2eindexer.TxsExecutedComponent
}

func NewTestApexChainDummy(configParams []string) *TestApexChainDummy {
	return &TestApexChainDummy{
		configParams: configParams,
		indexer:      e2eindexer.NewTxsExecutedComponentDummy(),
	}
}

func (td *TestApexChainDummy) BridgingRequest(
	ctx context.Context, destChainID string, privateKey string, receivers map[string]*big.Int, feeAmount *big.Int,
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

func (td *TestApexChainDummy) GenerateChainConfigs(indx int, validator *TestApexValidator) error {
	return nil
}

func (td *TestApexChainDummy) InitContracts(ctx context.Context, bridgeAdmin *crypto.ECDSAKey, bridgeURL string) error {
	return nil
}

func (td *TestApexChainDummy) PopulateApexSystem(apexSystem *ApexSystem) error {
	return nil
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

func (td *TestApexChainDummy) GetIndexer() e2eindexer.TxsExecutedComponent {
	return td.indexer
}

var _ ITestApexChain = (*TestApexChainDummy)(nil)
