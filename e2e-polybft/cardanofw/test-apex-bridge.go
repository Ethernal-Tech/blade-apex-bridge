package cardanofw

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
	cardanowallet "github.com/Ethernal-Tech/cardano-infrastructure/wallet"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"
)

const FundTokenAmount = uint64(100_000_000_000)

func RunCardanoCluster(
	t *testing.T,
	ctx context.Context,
	config *RunCardanoClusterConfig,
) (*TestCardanoCluster, error) {
	t.Helper()

	networkMagic := GetNetworkMagic(config.NetworkType)
	networkName := GetNetworkName(config.NetworkType)
	logsDir := fmt.Sprintf("%s/%d", getCardanoBaseLogsDir(t, networkName), config.ID)

	if err := common.CreateDirSafe(logsDir, 0750); err != nil {
		return nil, err
	}

	cluster, err := NewCardanoTestCluster(t,
		WithID(config.ID+1),
		WithNodesCount(config.NodesCount),
		WithStartTimeDelay(time.Second*5),
		WithPort(5100+config.ID*100),
		WithOgmiosPort(1337+config.ID),
		WithLogsDir(logsDir),
		WithNetworkMagic(networkMagic),
		WithNetworkType(config.NetworkType),
		WithConfigGenesisDir(networkName),
		WithInitialFunds(config.InitialFundsKeys, config.InitialFundsAmount),
	)
	if err != nil {
		return nil, err
	}

	fmt.Printf("Waiting for sockets to be ready\n")

	if err := cluster.WaitForReady(time.Minute * 2); err != nil {
		return nil, err
	}

	if err := cluster.StartOgmios(t, config.ID); err != nil {
		return nil, err
	}

	if err := cluster.WaitForBlockWithState(10, time.Second*120); err != nil {
		return nil, err
	}

	fmt.Printf("Cluster %d is ready\n", config.ID)

	return cluster, nil
}

func SetupAndRunApexBridge(
	t *testing.T,
	ctx context.Context,
	dataDir string,
	apexSystem *ApexSystem,
) *TestCardanoBridge {
	t.Helper()

	const (
		bladeEpochSize = 5
		numOfRetries   = 90
		waitTime       = time.Second * 2
	)

	cleanupDataDir := func() {
		os.RemoveAll(dataDir)
	}

	cleanupDataDir()

	cb := NewTestCardanoBridge(dataDir, apexSystem.Config)

	require.NoError(t, cb.CreateCardanoWallets())

	fmt.Printf("Wallets and addresses created\n")

	cb.StartValidators(t, bladeEpochSize)

	fmt.Printf("Validators started\n")

	cb.WaitForValidatorsReady(t)

	require.NoError(t, cb.NexusCreateWalletsAndAddresses(false))

	fmt.Printf("Validators ready\n")

	require.NoError(t, cb.RegisterChains(FundTokenAmount))

	fmt.Printf("Chains registered\n")

	require.NoError(t, cb.CreateCardanoMultisigAddresses(
		apexSystem.PrimeCluster.Config.NetworkType, apexSystem.GetVectorNetworkType()))

	fmt.Printf("Cardano Multisig addresses created\n")

	require.NoError(t, cb.FundCardanoMultisigAddresses(
		ctx, apexSystem.PrimeCluster, apexSystem.VectorCluster, FundTokenAmount))

	require.NoError(t, cb.GenerateConfigs(
		apexSystem.PrimeCluster,
		apexSystem.VectorCluster,
		apexSystem.Nexus,
	))

	fmt.Printf("Configs generated\n")

	if apexSystem.Config.NexusEnabled {
		apexSystem.Nexus.SetupChain(t, ctx, cb)
	}

	require.NoError(t, cb.StartValidatorComponents(ctx))

	fmt.Printf("Validator components started\n")

	require.NoError(t, cb.StartRelayer(ctx))

	fmt.Printf("Relayer started. Apex bridge setup done\n")

	return cb
}

func (a *ApexSystem) GetPrimeGenesisWallet(t *testing.T) cardanowallet.IWallet {
	t.Helper()

	primeGenesisWallet, err := GetGenesisWalletFromCluster(a.PrimeCluster.Config.TmpDir, 2)
	require.NoError(t, err)

	return primeGenesisWallet
}

func (a *ApexSystem) GetVectorGenesisWallet(t *testing.T) cardanowallet.IWallet {
	t.Helper()

	vectorGenesisWallet, err := GetGenesisWalletFromCluster(a.VectorCluster.Config.TmpDir, 2)
	require.NoError(t, err)

	return vectorGenesisWallet
}

func (a *ApexSystem) GetPrimeTxProvider() cardanowallet.ITxProvider {
	return cardanowallet.NewTxProviderOgmios(a.PrimeCluster.OgmiosURL())
}

func (a *ApexSystem) GetVectorTxProvider() cardanowallet.ITxProvider {
	return cardanowallet.NewTxProviderOgmios(a.VectorCluster.OgmiosURL())
}

func (a *ApexSystem) CreateAndFundUser(t *testing.T, ctx context.Context, sendAmount uint64) *TestApexUser {
	t.Helper()

	user := NewTestApexUser(t, a.PrimeCluster.Config.NetworkType, a.GetVectorNetworkType())

	txProviderPrime := a.GetPrimeTxProvider()
	// Fund prime address
	primeGenesisWallet := a.GetPrimeGenesisWallet(t)

	user.SendToUser(
		t, ctx, txProviderPrime, primeGenesisWallet, sendAmount, a.PrimeCluster.NetworkConfig())

	fmt.Printf("Prime user address funded\n")

	if a.Config.VectorEnabled {
		txProviderVector := a.GetVectorTxProvider()
		// Fund vector address
		vectorGenesisWallet := a.GetVectorGenesisWallet(t)

		user.SendToUser(
			t, ctx, txProviderVector, vectorGenesisWallet, sendAmount, a.VectorCluster.NetworkConfig())

		fmt.Printf("Vector user address funded\n")
	}

	return user
}

func (a *ApexSystem) CreateAndFundExistingUser(
	t *testing.T, ctx context.Context, primePrivateKey, vectorPrivateKey string, sendAmount uint64,
	primeNetworkConfig TestCardanoNetworkConfig, vectorNetworkConfig TestCardanoNetworkConfig,
) *TestApexUser {
	t.Helper()

	user := NewTestApexUserWithExistingWallets(t, primePrivateKey, vectorPrivateKey,
		primeNetworkConfig.NetworkType, vectorNetworkConfig.NetworkType)

	txProviderPrime := a.GetPrimeTxProvider()
	txProviderVector := a.GetVectorTxProvider()

	// Fund prime address
	primeGenesisWallet := a.GetPrimeGenesisWallet(t)

	user.SendToUser(t, ctx, txProviderPrime, primeGenesisWallet, sendAmount, primeNetworkConfig)

	fmt.Printf("Prime user address funded\n")

	// Fund vector address
	vectorGenesisWallet := a.GetVectorGenesisWallet(t)

	user.SendToUser(t, ctx, txProviderVector, vectorGenesisWallet, sendAmount, vectorNetworkConfig)

	fmt.Printf("Vector user address funded\n")

	return user
}

func (a *ApexSystem) CreateAndFundNexusUser(ctx context.Context, ethAmount uint64) (*wallet.Account, error) {
	user, err := wallet.GenerateAccount()
	if err != nil {
		return nil, err
	}

	txRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithClient(a.Nexus.Cluster.Servers[0].JSONRPC()))
	if err != nil {
		return nil, err
	}

	addr := user.Address()

	receipt, err := txRelayer.SendTransaction(
		types.NewTx(types.NewLegacyTx(
			types.WithFrom(a.Nexus.Admin.Ecdsa.Address()),
			types.WithTo(&addr),
			types.WithValue(ethgo.Ether(ethAmount)),
		)),
		a.Nexus.Admin.Ecdsa)
	if err != nil {
		return nil, err
	} else if receipt.Status != uint64(types.ReceiptSuccess) {
		return nil, fmt.Errorf("fund user tx failed: %d", receipt.Status)
	}

	return user, nil
}

func (a *ApexSystem) GetVectorNetworkType() cardanowallet.CardanoNetworkType {
	if a.Config.VectorEnabled && a.VectorCluster != nil {
		return a.VectorCluster.Config.NetworkType
	}

	return cardanowallet.TestNetNetwork // does not matter really
}

func getCardanoBaseLogsDir(t *testing.T, name string) string {
	t.Helper()

	return filepath.Join("../..",
		fmt.Sprintf("e2e-logs-cardano-%s-%d", name, time.Now().UTC().Unix()), t.Name())
}
