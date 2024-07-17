package cardanofw

import (
	"fmt"
	"log"
	"testing"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/framework"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/require"
)

type NexusBridgeOption func(*TestEVMChain)

type TestEVMChain struct {
	Admin            *wallet.Account
	Cluster          *framework.TestCluster
	TestContractAddr types.Address

	validatorCount int
}

type ContractProxy struct {
	contractAddr types.Address
	proxyAddr    types.Address
}

func SetupAndRunEVMChain(
	t *testing.T,
	bladeValidatorsNum int,
	initialPort int64,
) *TestEVMChain {
	t.Helper()

	// Nexus contracts
	err := InitNexusContracts()
	if err != nil {
		log.Fatal(err)
	}

	admin, err := wallet.GenerateAccount()
	require.NoError(t, err)

	cluster := framework.NewTestCluster(t, bladeValidatorsNum,
		framework.WithInitialPort(initialPort),
		framework.WithBladeAdmin(admin.Address().String()),
	)

	cleanupFunc := func() {
		fmt.Printf("Stopping nexus bridge\n")

		cluster.Stop()

		fmt.Printf("Stopped nexus bridge\n")
	}

	t.Cleanup(cleanupFunc)

	cluster.WaitForReady(t)

	txRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithClient(cluster.Servers[0].JSONRPC()))
	require.NoError(t, err)

	testContractAddr := DeployWithProxy(t, txRelayer, admin, ClaimsTest, ERC1967Proxy)

	fmt.Printf("EVM chain %d setup done\n", initialPort)

	return &TestEVMChain{
		Admin:            admin,
		Cluster:          cluster,
		TestContractAddr: testContractAddr.proxyAddr,

		validatorCount: bladeValidatorsNum,
	}
}

func DeployWithProxy(
	t *testing.T,
	txRelayer txrelayer.TxRelayer,
	admin *wallet.Account,
	contract *contracts.Artifact,
	proxy *contracts.Artifact,
	// aditional params?
) *ContractProxy {
	t.Helper()

	// deploy contract
	receipt, err := txRelayer.SendTransaction(
		types.NewTx(types.NewLegacyTx(
			types.WithFrom(admin.Ecdsa.Address()),
			types.WithInput(contract.Bytecode),
		)),
		admin.Ecdsa)
	require.NoError(t, err)
	require.Equal(t, receipt.Status, uint64(1))

	contractAddr := types.Address(receipt.ContractAddress)

	// deploy proxy contract and call initialize
	receipt, err = txRelayer.SendTransaction(
		types.NewTx(types.NewLegacyTx(
			types.WithFrom(admin.Ecdsa.Address()),
			types.WithInput(proxy.Bytecode),
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
