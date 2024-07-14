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
	validatorCount int
	cluster        *framework.TestCluster
	contractAddr   types.Address
}

func SetupAndRunNexusBridge(
	t *testing.T,
	bladeValidatorsNum int,
) *TestNexusBridge {

	admin, err := wallet.GenerateAccount()
	require.NoError(t, err)

	cluster := framework.NewTestCluster(t, bladeValidatorsNum,
		framework.WithBladeAdmin(admin.Address().String()),
	)

	txRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithClient(cluster.Servers[0].JSONRPC()))
	require.NoError(t, err)

	// deploy contract
	receipt, err := txRelayer.SendTransaction(
		types.NewTx(types.NewLegacyTx(
			types.WithFrom(admin.Ecdsa.Address()),
			types.WithInput(contractsapi.TestCardanoVerifySign.Bytecode),
		)),
		admin.Ecdsa)
	require.NoError(t, err)

	contractAddr := types.Address(receipt.ContractAddress)

	cv := &TestNexusBridge{
		validatorCount: bladeValidatorsNum,
		cluster:        cluster,
		contractAddr:   contractAddr,
	}

	return cv
}
