package cardanofw

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"math/big"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/command/genesis"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/framework"
	"github.com/0xPolygon/polygon-edge/jsonrpc"
	"github.com/0xPolygon/polygon-edge/types"
	ci "github.com/Ethernal-Tech/cardano-infrastructure/wallet"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"
)

const (
	FundEthTokenAmount = uint64(100_000)
)

type NexusBridgeOption func(*TestEVMBridge)

type TestEVMBridge struct {
	Admin   *wallet.Account
	Config  *ApexSystemConfig
	Cluster *framework.TestCluster

	NativeTokenWallet types.Address
	Gateway           types.Address
}

func (ec *TestEVMBridge) SendTxEvm(privateKey string, receiver string, amount *big.Int) error {
	return ec.SendTxEvmMultipleReceivers(privateKey, []string{receiver}, amount)
}

func (ec *TestEVMBridge) SendTxEvmMultipleReceivers(privateKey string, receivers []string, amount *big.Int) error {
	params := []string{
		"sendtx",
		"--tx-type", "evm",
		"--gateway-addr", ec.Gateway.String(),
		"--nexus-url", ec.Cluster.Servers[0].JSONRPCAddr(),
		"--key", privateKey,
		"--chain-dst", "prime",
		"--fee", "1000010000000000000",
	}

	receiversParam := make([]string, 0, len(receivers))
	for i := 0; i < len(receivers); i++ {
		receiversParam = append(receiversParam, "--receiver", fmt.Sprintf("%s:%s", receivers[i], amount))
	}

	params = append(params, receiversParam...)

	return RunCommand(ResolveApexBridgeBinary(), params, os.Stdout)
}

func (ec *TestEVMBridge) NodeURL() string {
	return fmt.Sprintf("http://localhost:%d", ec.Config.NexusStartingPort)
}

func (ec *TestEVMBridge) GetGatewayAddress() types.Address {
	return ec.Gateway
}

func (ec *TestEVMBridge) GetHotWalletAddress() types.Address {
	return ec.NativeTokenWallet
}

func (ec *TestEVMBridge) InitSmartContracts(bridgeURL string) error {
	workingDirectory, err := os.MkdirTemp("", "deploy-evm")
	if err != nil {
		return err
	}

	defer func() {
		_ = os.RemoveAll(workingDirectory)
		_ = os.Remove(workingDirectory)
	}()

	pk, err := ec.Admin.Ecdsa.MarshallPrivateKey()
	if err != nil {
		return err
	}

	var b bytes.Buffer

	err = RunCommand(ResolveApexBridgeBinary(), []string{
		"deploy-evm",
		"--bridge-url", bridgeURL,
		"--url", ec.Cluster.Servers[0].JSONRPCAddr(),
		"--key", hex.EncodeToString(pk),
		"--dir", workingDirectory,
		"--clone",
	}, io.MultiWriter(os.Stdout, &b))
	if err != nil {
		return err
	}

	output := b.String()
	reGateway := regexp.MustCompile(`Gateway Proxy Address\s*=\s*0x([a-fA-F0-9]+)`)
	reNativeTokenWallet := regexp.MustCompile(`NativeTokenWallet Proxy Address\s*=\s*0x([a-fA-F0-9]+)`)

	if match := reGateway.FindStringSubmatch(output); len(match) > 0 {
		ec.Gateway = types.StringToAddress(match[1])
	}

	if match := reNativeTokenWallet.FindStringSubmatch(output); len(match) > 0 {
		ec.NativeTokenWallet = types.StringToAddress(match[1])
	}

	return nil
}

func RunEVMChain(
	t *testing.T,
	config *ApexSystemConfig,
) (*TestEVMBridge, error) {
	t.Helper()

	admin, err := wallet.GenerateAccount()
	if err != nil {
		return nil, err
	}

	cluster := framework.NewTestCluster(t, config.NexusValidatorCount,
		framework.WithPremine(admin.Address()),
		framework.WithInitialPort(config.NexusStartingPort),
		framework.WithLogsDirSuffix(ChainIDNexus),
		framework.WithBladeAdmin(admin.Address().String()),
		framework.WithApexConfig(genesis.ApexConfigNexus),
		framework.WithBurnContract(config.NexusBurnContractInfo),
	)

	cluster.WaitForReady(t)

	fmt.Printf("EVM chain %d setup done\n", config.NexusStartingPort)

	return &TestEVMBridge{
		Admin:   admin,
		Cluster: cluster,

		Config: config,
	}, nil
}

func SetupAndRunNexusBridge(
	t *testing.T,
	ctx context.Context,
	apexSystem *ApexSystem,
) {
	t.Helper()

	require.NoError(t,
		apexSystem.Nexus.InitSmartContracts(apexSystem.Bridge.cluster.Servers[0].JSONRPCAddr()))

	apexSystem.Nexus.Cluster.Transfer(t,
		apexSystem.Nexus.Admin.Ecdsa,
		apexSystem.Nexus.GetHotWalletAddress(),
		ethgo.Ether(FundTokenAmount),
	)

	apexSystem.Nexus.Cluster.Transfer(t,
		apexSystem.Nexus.Admin.Ecdsa,
		apexSystem.Bridge.GetRelayerWalletAddr(),
		ethgo.Ether(1),
	)
}

func GetEthAmount(ctx context.Context, evmChain *TestEVMBridge, wallet *wallet.Account) (*big.Int, error) {
	ethAmount, err := evmChain.Cluster.Servers[0].JSONRPC().GetBalance(wallet.Address(), jsonrpc.LatestBlockNumberOrHash)
	if err != nil {
		return nil, err
	}

	return ethAmount, err
}

func WaitForEthAmount(
	ctx context.Context,
	evmChain *TestEVMBridge,
	wallet *wallet.Account,
	cmpHandler func(*big.Int) bool,
	numRetries int,
	waitTime time.Duration,
	isRecoverableError ...ci.IsRecoverableErrorFn,
) error {
	return ci.ExecuteWithRetry(ctx, numRetries, waitTime, func() (bool, error) {
		ethers, err := GetEthAmount(ctx, evmChain, wallet)

		return err == nil && cmpHandler(ethers), err
	}, isRecoverableError...)
}
