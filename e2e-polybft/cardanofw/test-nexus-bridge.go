package cardanofw

import (
	"context"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/framework"
	"github.com/0xPolygon/polygon-edge/jsonrpc"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
	ci "github.com/Ethernal-Tech/cardano-infrastructure/wallet"
	"github.com/umbracle/ethgo/abi"
)

var (
	tokenName = "TEST"
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

	// TODO: create cardano validator wallets (need to pass prime)
	// ^ do this outside, like start apex bridge or w/e

	// TODO: relayerAddr
	relayerAddr := types.Address{}

	erc20PredicateProxy, err := deployWithProxy(txRelayer, admin, ERC20TokenPredicate, ERC1967Proxy, InitializeNoParams())
	if err != nil {
		return nil, err
	}

	nativeErc20MintableProxy, err := deployWithProxy(txRelayer, admin, NativeERC20Mintable, ERC1967Proxy, InitializeNoParams())
	if err != nil {
		return nil, err
	}

	validatorsTypeApi := abi.MustNewType("address[]")
	data, err := validatorsTypeApi.Encode(premineAddrs[:validatorsCount])
	if err != nil {
		return nil, err
	}

	validatorsProxy, err := deployWithProxy(txRelayer, admin, Validators, ERC1967Proxy, InitializeWithParams(data))
	if err != nil {
		return nil, err
	}

	gatewayProxy, err := deployWithProxy(txRelayer, admin, Gateway, ERC1967Proxy, InitializeNoParams())
	if err != nil {
		return nil, err
	}

	err = gatewaySetDependencies(txRelayer, admin, gatewayProxy.proxyAddr,
		erc20PredicateProxy.proxyAddr, validatorsProxy.proxyAddr, relayerAddr)
	if err != nil {
		return nil, err
	}

	err = erc20predicateSetDependencies(txRelayer, admin, erc20PredicateProxy.proxyAddr,
		gatewayProxy.proxyAddr, nativeErc20MintableProxy.proxyAddr)
	if err != nil {
		return nil, err
	}

	err = nativeErc20SetDependencies(txRelayer, admin, nativeErc20MintableProxy.proxyAddr,
		erc20PredicateProxy.proxyAddr, tokenName, tokenName, 18, 0)
	if err != nil {
		return nil, err
	}

	// TODO cardano validator data
	// validators.setdepend(gateway.addr, valAddrCardData) {v.address, valCardData}
	err = validatorsSetDependencies(txRelayer, admin, validatorsProxy.proxyAddr,
		gatewayProxy.proxyAddr, validators)
	if err != nil {
		return nil, err
	}

	fmt.Printf("EVM chain %d setup done\n", initialPort)

	return &TestEVMChain{
		Admin:      admin,
		Cluster:    cluster,
		Validators: validators,
	}, nil
}

func gatewaySetDependencies(
	txRelayer txrelayer.TxRelayer,
	admin *wallet.Account,
	gatewayProxyAddr types.Address,
	erc20PredicateAddr types.Address,
	validatorsAddr types.Address,
	relayerAddr types.Address,
) error {
	gateway := GatewaySetDependenciesFn{
		ERC20_:      erc20PredicateAddr,
		Validators_: validatorsAddr,
		Relayer_:    relayerAddr,
	}

	encoded, err := gateway.EncodeAbi()
	if err != nil {
		return err
	}

	receipt, err := txRelayer.SendTransaction(
		types.NewTx(types.NewLegacyTx(
			types.WithFrom(admin.Address()),
			types.WithTo(&gatewayProxyAddr),
			types.WithInput(encoded),
		)), admin.Ecdsa)
	if err != nil {
		return err
	} else if receipt.Status != uint64(1) {
		return fmt.Errorf("calling setDependencies failed: %d", receipt.Status)
	}

	return nil
}

func erc20predicateSetDependencies(
	txRelayer txrelayer.TxRelayer,
	admin *wallet.Account,
	erc20PredicateProxyAddr types.Address,
	gatewayAddr types.Address,
	nativeTokenAddr types.Address,
) error {
	erc20Predicate := ERC20PredicateSetDependenciesFn{
		Gateway_:     gatewayAddr,
		NativeToken_: nativeTokenAddr,
	}

	encoded, err := erc20Predicate.EncodeAbi()
	if err != nil {
		return err
	}

	receipt, err := txRelayer.SendTransaction(
		types.NewTx(types.NewLegacyTx(
			types.WithFrom(admin.Address()),
			types.WithTo(&erc20PredicateProxyAddr),
			types.WithInput(encoded),
		)), admin.Ecdsa)
	if err != nil {
		return err
	} else if receipt.Status != uint64(1) {
		return fmt.Errorf("calling setDependencies failed: %d", receipt.Status)
	}

	return nil
}

func nativeErc20SetDependencies(
	txRelayer txrelayer.TxRelayer,
	admin *wallet.Account,
	nativeErc20ProxyAddr types.Address,
	predicateAddr types.Address,
	name string, symbol string,
	decimals uint8, tokenSupply int64,
) error {
	nativeErc20 := NativeERC20SetDependenciesFn{
		Predicate_: predicateAddr,
		Owner_:     admin.Address(),
		Name_:      name,
		Symbol_:    symbol,
		Decimals_:  decimals,
		Supply_:    big.NewInt(tokenSupply),
	}

	encoded, err := nativeErc20.EncodeAbi()
	if err != nil {
		return err
	}

	receipt, err := txRelayer.SendTransaction(
		types.NewTx(types.NewLegacyTx(
			types.WithFrom(admin.Address()),
			types.WithTo(&nativeErc20ProxyAddr),
			types.WithInput(encoded),
		)), admin.Ecdsa)
	if err != nil {
		return err
	} else if receipt.Status != uint64(1) {
		return fmt.Errorf("calling setDependencies failed: %d", receipt.Status)
	}

	return nil
}

func validatorsSetDependencies(
	txRelayer txrelayer.TxRelayer,
	admin *wallet.Account,
	validatorsProxyAddr types.Address,
	gatewayAddr types.Address,
	validators []*wallet.Account,
) error {
	// TODO: Make this with validators
	chainData := []ValidatorAddressChainData{
		{
			Address_: validators[0].Address(),
			Data_: ValidatorChainData{
				Key_: []*big.Int{
					big.NewInt(0),
					big.NewInt(1),
					big.NewInt(2),
					big.NewInt(3),
				},
			},
		},
		{
			Address_: validators[1].Address(),
			Data_: ValidatorChainData{
				Key_: []*big.Int{
					big.NewInt(0),
					big.NewInt(1),
					big.NewInt(2),
					big.NewInt(3),
				},
			},
		},
		{
			Address_: validators[2].Address(),
			Data_: ValidatorChainData{
				Key_: []*big.Int{
					big.NewInt(0),
					big.NewInt(1),
					big.NewInt(2),
					big.NewInt(3),
				},
			},
		},
		{
			Address_: validators[3].Address(),
			Data_: ValidatorChainData{
				Key_: []*big.Int{
					big.NewInt(0),
					big.NewInt(1),
					big.NewInt(2),
					big.NewInt(3),
				},
			},
		},
	}

	validatorsData := ValidatorsSetDependenciesFn{
		Gateway_:   gatewayAddr,
		ChainData_: chainData,
	}
	_ = validators

	encoded, err := validatorsData.EncodeAbi()
	if err != nil {
		return err
	}

	receipt, err := txRelayer.SendTransaction(
		types.NewTx(types.NewLegacyTx(
			types.WithFrom(admin.Address()),
			types.WithTo(&validatorsProxyAddr),
			types.WithInput(encoded),
		)), admin.Ecdsa)
	if err != nil {
		return err
	} else if receipt.Status != uint64(1) {
		return fmt.Errorf("calling setDependencies failed: %d", receipt.Status)
	}

	return nil
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
	initParams []byte,
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
			types.WithInput(initParams),
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
