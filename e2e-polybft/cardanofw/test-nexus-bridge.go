package cardanofw

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/framework"
	"github.com/0xPolygon/polygon-edge/jsonrpc"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
	ci "github.com/Ethernal-Tech/cardano-infrastructure/wallet"
)

type NexusBridgeOption func(*TestEVMChain)

type TestEVMChain struct {
	Admin            *wallet.Account
	Validators       []*wallet.Account
	Cluster          *framework.TestCluster
	TestContractAddr types.Address
}

type ContractProxy struct {
	contractAddr types.Address
	proxyAddr    types.Address
}

func SetupAndRunEVMChain(
	t *testing.T,
	validatorsCount int,
	initialPort int64,
) (*TestEVMChain, error) {
	t.Helper()

	// Nexus contracts
	err := InitNexusContracts()
	if err != nil {
		return nil, err
	}

	premineAddrs := make([]types.Address, validatorsCount+1)

	admin, err := wallet.GenerateAccount()
	if err != nil {
		return nil, err
	}
	premineAddrs[validatorsCount] = admin.Address()

	validators := make([]*wallet.Account, validatorsCount)
	for i := 0; i < validatorsCount; i++ {
		validator, err := wallet.GenerateAccount()
		if err != nil {
			return nil, err
		}

		validators[i] = validator
		premineAddrs[i] = validator.Address()
	}

	cluster := framework.NewTestCluster(t, validatorsCount,
		framework.WithPremine(premineAddrs...),
		framework.WithInitialPort(initialPort),
		framework.WithLogsDirSuffix("nexus"),
		framework.WithBladeAdmin(admin.Address().String()),
	)

	cluster.WaitForReady(t)

	txRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithClient(cluster.Servers[0].JSONRPC()))
	if err != nil {
		return nil, err
	}

	testContractAddr, err := deployWithProxy(txRelayer, admin, ClaimsTest, ERC1967Proxy)
	if err != nil {
		return nil, err
	}

	fmt.Printf("EVM chain %d setup done\n", initialPort)

	return &TestEVMChain{
		Admin:            admin,
		Cluster:          cluster,
		TestContractAddr: testContractAddr.proxyAddr,
		Validators:       validators,
	}, nil
}

func GetEthAmount(ctx context.Context, evmChain *TestEVMChain, wallet *wallet.Account) (uint64, error) {
	ethAmount, err := evmChain.Cluster.Servers[1].JSONRPC().GetBalance(wallet.Address(), jsonrpc.LatestBlockNumberOrHash)
	if err != nil {
		return 0, err
	}

	return ethAmount.Uint64(), err
}

func WaitForEthAmount(ctx context.Context, evmChain *TestEVMChain, wallet *wallet.Account, cmpHandler func(uint64) bool, numRetries int, waitTime time.Duration,
	isRecoverableError ...ci.IsRecoverableErrorFn,
) error {
	return ci.ExecuteWithRetry(ctx, numRetries, waitTime, func() (bool, error) {
		ethers, err := GetEthAmount(ctx, evmChain, wallet)

		return err == nil && cmpHandler(ethers), err
	}, isRecoverableError...)
}

func deployWithProxy(
	txRelayer txrelayer.TxRelayer,
	admin *wallet.Account,
	contract *contracts.Artifact,
	proxy *contracts.Artifact,
	// aditional params?
) (*ContractProxy, error) {
	// deploy contract
	receipt, err := txRelayer.SendTransaction(
		types.NewTx(types.NewLegacyTx(
			types.WithFrom(admin.Ecdsa.Address()),
			types.WithInput(contract.Bytecode),
		)),
		admin.Ecdsa)
	if err != nil {
		return nil, err
	} else if receipt.Status != uint64(1) {
		return nil, fmt.Errorf("deploying smart contract failed: %d", receipt.Status)
	}

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
	if err != nil {
		return nil, err
	} else if receipt.Status != uint64(1) {
		return nil, fmt.Errorf("deploying proxy smart contract failed: %d", receipt.Status)
	}

	return &ContractProxy{
		contractAddr: contractAddr,
		proxyAddr:    types.Address(receipt.ContractAddress),
	}, nil
}
