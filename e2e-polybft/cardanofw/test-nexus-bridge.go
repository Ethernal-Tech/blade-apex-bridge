package cardanofw

import (
	"testing"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/framework"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/require"
)

type NexusBridgeOption func(*TestNexusBridge)

type TestNexusBridge struct {
	validatorCount   int
	cluster          *framework.TestCluster
	testContractAddr types.Address
}

type ContractProxy struct {
	contractAddr types.Address
	proxyAddr    types.Address
}

func SetupAndRunNexusBridge(
	t *testing.T,
	bladeValidatorsNum int,
) *TestNexusBridge {
	t.Helper()

	admin, err := wallet.GenerateAccount()
	require.NoError(t, err)

	cluster := framework.NewTestCluster(t, bladeValidatorsNum,
		framework.WithBladeAdmin(admin.Address().String()),
	)

	txRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithClient(cluster.Servers[0].JSONRPC()))
	require.NoError(t, err)

	testContractAddr := DeployWithProxy(t, txRelayer, admin, "ClaimsTest")

	cv := &TestNexusBridge{
		validatorCount:   bladeValidatorsNum,
		cluster:          cluster,
		testContractAddr: testContractAddr.proxyAddr,
	}

	return cv
}

func DeployWithProxy(
	t *testing.T,
	txRelayer txrelayer.TxRelayer,
	admin *wallet.Account,
	contract string,
	// aditional params?
) *ContractProxy {
	t.Helper()

	// deploy contract
	testProxy, err := contractsapi.GetArtifactFromArtifactName(contract)
	require.NoError(t, err)

	receipt, err := txRelayer.SendTransaction(
		types.NewTx(types.NewLegacyTx(
			types.WithFrom(admin.Ecdsa.Address()),
			types.WithInput(testProxy.Bytecode),
		)),
		admin.Ecdsa)
	require.NoError(t, err)
	require.Equal(t, receipt.Status, uint64(1))

	contractAddr := types.Address(receipt.ContractAddress)

	// deploy proxy contract and call initialize
	proxyContract, err := contractsapi.GetArtifactFromArtifactName("ERC1967Proxy")
	require.NoError(t, err)

	receipt, err = txRelayer.SendTransaction(
		types.NewTx(types.NewLegacyTx(
			types.WithFrom(admin.Ecdsa.Address()),
			types.WithInput(proxyContract.Bytecode),
			types.WithInput(contractAddr[:]),
			types.WithInput([]byte("initialize")),
		)),
		admin.Ecdsa)
	require.NoError(t, err)
	require.Equal(t, receipt.Status, uint64(1))

	proxyAddr := types.Address(receipt.ContractAddress)

	return &ContractProxy{
		contractAddr: contractAddr,
		proxyAddr:    proxyAddr,
	}
}
