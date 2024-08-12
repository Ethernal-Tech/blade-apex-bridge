package cardanofw

import (
	"context"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/helper/common"
	cardanowallet "github.com/Ethernal-Tech/cardano-infrastructure/wallet"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"
)

const FundTokenAmount = uint64(100_000_000_000)

func RunCardanoCluster(
	t *testing.T,
	ctx context.Context,
	id int,
	cardanoNodesNum int,
	networkType cardanowallet.CardanoNetworkType,
) (*TestCardanoCluster, error) {
	t.Helper()

	networkMagic := GetNetworkMagic(networkType)
	networkName := GetNetworkName(networkType)
	logsDir := fmt.Sprintf("%s/%d", getCardanoBaseLogsDir(t, networkName), id)

	if err := common.CreateDirSafe(logsDir, 0750); err != nil {
		return nil, err
	}

	cluster, err := NewCardanoTestCluster(t,
		WithID(id+1),
		WithNodesCount(cardanoNodesNum),
		WithStartTimeDelay(time.Second*5),
		WithPort(5100+id*100),
		WithOgmiosPort(1337+id),
		WithLogsDir(logsDir),
		WithNetworkMagic(networkMagic),
		WithNetworkType(networkType),
		WithConfigGenesisDir(networkName),
	)
	if err != nil {
		return nil, err
	}

	fmt.Printf("Waiting for sockets to be ready\n")

	if err := cluster.WaitForReady(time.Minute * 2); err != nil {
		return nil, err
	}

	if err := cluster.StartOgmios(t, id); err != nil {
		return nil, err
	}

	if err := cluster.WaitForBlockWithState(10, time.Second*120); err != nil {
		return nil, err
	}

	fmt.Printf("Cluster %d is ready\n", id)

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

	primeCluster := apexSystem.PrimeCluster
	vectorCluster := apexSystem.VectorCluster

	cb := NewTestCardanoBridge(dataDir, apexSystem.Config)

	require.NoError(t, cb.CardanoCreateWalletsAndAddresses(primeCluster.NetworkConfig(), vectorCluster.NetworkConfig()))

	fmt.Printf("Wallets and addresses created\n")

	txProviderPrime := cardanowallet.NewTxProviderOgmios(primeCluster.OgmiosURL())
	txProviderVector := cardanowallet.NewTxProviderOgmios(vectorCluster.OgmiosURL())

	primeGenesisWallet, err := GetGenesisWalletFromCluster(primeCluster.Config.TmpDir, 1)
	require.NoError(t, err)

	res, err := SendTx(ctx, txProviderPrime, primeGenesisWallet, FundTokenAmount,
		cb.PrimeMultisigAddr, primeCluster.NetworkConfig(), []byte{})
	require.NoError(t, err)

	err = cardanowallet.WaitForAmount(context.Background(), txProviderPrime, cb.PrimeMultisigAddr, func(val uint64) bool {
		return val == FundTokenAmount
	}, numOfRetries, waitTime, IsRecoverableError)
	require.NoError(t, err)

	fmt.Printf("Prime multisig addr funded: %s\n", res)

	res, err = SendTx(ctx, txProviderPrime, primeGenesisWallet, FundTokenAmount,
		cb.PrimeMultisigFeeAddr, primeCluster.NetworkConfig(), []byte{})
	require.NoError(t, err)

	err = cardanowallet.WaitForAmount(context.Background(), txProviderPrime,
		cb.PrimeMultisigFeeAddr, func(val uint64) bool {
			return val == FundTokenAmount
		}, numOfRetries, waitTime, IsRecoverableError)
	require.NoError(t, err)

	fmt.Printf("Prime multisig fee addr funded: %s\n", res)

	vectorGenesisWallet, err := GetGenesisWalletFromCluster(vectorCluster.Config.TmpDir, 1)
	require.NoError(t, err)

	res, err = SendTx(ctx, txProviderVector, vectorGenesisWallet, FundTokenAmount,
		cb.VectorMultisigAddr, vectorCluster.NetworkConfig(), []byte{})
	require.NoError(t, err)

	err = cardanowallet.WaitForAmount(context.Background(), txProviderVector,
		cb.VectorMultisigAddr, func(val uint64) bool {
			return val == FundTokenAmount
		}, numOfRetries, waitTime, IsRecoverableError)
	require.NoError(t, err)

	fmt.Printf("Vector multisig addr funded: %s\n", res)

	res, err = SendTx(ctx, txProviderVector, vectorGenesisWallet, FundTokenAmount,
		cb.VectorMultisigFeeAddr, vectorCluster.NetworkConfig(), []byte{})
	require.NoError(t, err)

	err = cardanowallet.WaitForAmount(context.Background(), txProviderVector,
		cb.VectorMultisigFeeAddr, func(val uint64) bool {
			return val == FundTokenAmount
		}, numOfRetries, waitTime, IsRecoverableError)
	require.NoError(t, err)

	fmt.Printf("Vector multisig fee addr funded: %s\n", res)

	cb.StartValidators(t, bladeEpochSize)

	fmt.Printf("Validators started\n")

	cb.WaitForValidatorsReady(t)

	fmt.Printf("Validators ready\n")

	return cb
}

func (a *ApexSystem) SetupAndRunValidatorsAndRelayer(
	t *testing.T,
	ctx context.Context,
) {
	t.Helper()

	// need params for it to work properly
	primeTokenSupply := new(big.Int).SetUint64(FundTokenAmount)
	vectorTokenSupply := new(big.Int).SetUint64(FundTokenAmount)
	nexusTokenSupplyDfm := new(big.Int).SetUint64(FundEthTokenAmount)
	exp := new(big.Int).Exp(big.NewInt(10), big.NewInt(6), nil)
	nexusTokenSupplyDfm.Mul(nexusTokenSupplyDfm, exp)
	require.NoError(t, a.Bridge.RegisterChains(primeTokenSupply, vectorTokenSupply, nexusTokenSupplyDfm, a))

	fmt.Printf("Chains registered\n")

	// need params for it to work properly
	require.NoError(t, a.Bridge.GenerateConfigs(
		a.PrimeCluster,
		a.VectorCluster,
		a.Nexus,
	))

	fmt.Printf("Configs generated\n")

	require.NoError(t, a.Bridge.StartValidatorComponents(ctx))
	fmt.Printf("Validator components started\n")

	require.NoError(t, a.Bridge.StartRelayer(ctx))
	fmt.Printf("Relayer started\n")
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

func (a *ApexSystem) CreateAndFundUser(t *testing.T, ctx context.Context, sendAmount uint64,
	primeNetworkConfig TestCardanoNetworkConfig, vectorNetworkConfig TestCardanoNetworkConfig,
) *TestApexUser {
	t.Helper()

	user := NewTestApexUser(t, primeNetworkConfig.NetworkType, vectorNetworkConfig.NetworkType)

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

func (a *ApexSystem) CreateAndFundNexusUser(t *testing.T, ctx context.Context, ethAmount uint64) *wallet.Account {
	t.Helper()

	user, err := wallet.GenerateAccount()
	require.NoError(t, err)

	txRes := a.Nexus.Cluster.Transfer(t, a.Nexus.Admin.Ecdsa, user.Address(), ethgo.Ether(ethAmount))
	require.True(t, txRes.Succeed())

	return user
}

func getCardanoBaseLogsDir(t *testing.T, name string) string {
	t.Helper()

	return filepath.Join("../..",
		fmt.Sprintf("e2e-logs-cardano-%s-%d", name, time.Now().UTC().Unix()), t.Name())
}
