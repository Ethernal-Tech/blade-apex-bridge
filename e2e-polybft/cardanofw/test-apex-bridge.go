package cardanofw

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path"
	"sync"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/Ethernal-Tech/cardano-infrastructure/wallet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type ApexSystem struct {
	PrimeCluster  *TestCardanoCluster
	VectorCluster *TestCardanoCluster
	Bridge        *TestCardanoBridge
}

func SetupAndRunApexCardanoCluster(
	t *testing.T,
	ctx context.Context,
	networkType wallet.CardanoNetworkType,
	networkMagic uint,
	num int,
	genesisConfigDir string,
	baseLogsDir string,
) *TestCardanoCluster {
	t.Helper()

	var (
		clError error
		cluster *TestCardanoCluster
	)

	cleanupFunc := func() {
		fmt.Printf("Cleaning up cardano chain %v %v", networkType.GetPrefix(), networkMagic)

		stopErrs := []error(nil)

		if cluster != nil {
			go func(cl *TestCardanoCluster) {
				stopErrs = append(stopErrs, cl.Stop())
			}(cluster)
		}

		fmt.Printf("Done cleaning up cardano chain %v %v: %v\n", networkType.GetPrefix(), networkMagic, errors.Join(stopErrs...))
	}

	t.Cleanup(cleanupFunc)

	wg := sync.WaitGroup{}

	wg.Add(1)
	go func(id int) {
		defer wg.Done()

		checkAndSetError := func(err error) bool {
			clError = err
			return err != nil
		}

		logsDir := fmt.Sprintf("%s/%d", baseLogsDir, id)

		err := common.CreateDirSafe(logsDir, 0750)
		if checkAndSetError(err) {
			return
		}

		cluster, err := NewCardanoTestCluster(t,
			WithID(id+1),
			WithNodesCount(4),
			WithStartTimeDelay(time.Second*5),
			WithPort(5100+id*100),
			WithOgmiosPort(1337+id),
			WithLogsDir(logsDir),
			WithNetworkMagic(networkMagic),
			WithNetworkType(networkType),
			WithConfigGenesisDir(genesisConfigDir),
		)
		if checkAndSetError(err) {
			return
		}

		cluster.Config.WithStdout = false

		fmt.Printf("Waiting for sockets to be ready\n")

		if checkAndSetError(cluster.WaitForReady(time.Minute * 2)) {
			return
		}

		if checkAndSetError(cluster.StartOgmios(t, id)) {
			return
		}

		if checkAndSetError(cluster.WaitForBlockWithState(10, time.Second*120)) {
			return
		}

		fmt.Printf("Cluster %d is ready\n", id)
	}(num)

	wg.Wait()
	assert.NoError(t, clError)

	return cluster
}

func SetupAndRunApexBridge(
	t *testing.T,
	ctx context.Context,
	dataDir string,
	bladeValidatorsNum int,
	primeCluster *TestCardanoCluster,
	vectorCluster *TestCardanoCluster,
	opts ...CardanoBridgeOption,
) *TestCardanoBridge {
	t.Helper()

	const (
		sendAmount     = uint64(100_000_000_000)
		bladeEpochSize = 5
		numOfRetries   = 90
		waitTime       = time.Second * 2
		apiPort        = 40000
		apiKey         = "test_api_key"
	)

	cleanupDataDir := func() {
		os.RemoveAll(dataDir)
	}

	cleanupDataDir()

	opts = append(opts,
		WithAPIPortStart(apiPort),
		WithAPIKey(apiKey),
	)

	cb := NewTestCardanoBridge(dataDir, bladeValidatorsNum, opts...)

	cleanupFunc := func() {
		fmt.Printf("Cleaning up apex bridge\n")

		// cleanupDataDir()
		cb.StopValidators()

		fmt.Printf("Done cleaning up apex bridge\n")
	}

	t.Cleanup(cleanupFunc)

	require.NoError(t, cb.CardanoCreateWalletsAndAddresses(primeCluster.NetworkConfig(), vectorCluster.NetworkConfig()))

	fmt.Printf("Wallets and addresses created\n")

	txProviderPrime := wallet.NewTxProviderOgmios(primeCluster.OgmiosURL())
	txProviderVector := wallet.NewTxProviderOgmios(vectorCluster.OgmiosURL())

	primeGenesisWallet, err := GetGenesisWalletFromCluster(primeCluster.Config.TmpDir, 1)
	require.NoError(t, err)

	_, err = SendTx(ctx, txProviderPrime, primeGenesisWallet, sendAmount,
		cb.PrimeMultisigAddr, primeCluster.NetworkConfig(), []byte{})
	require.NoError(t, err)

	err = wallet.WaitForAmount(context.Background(), txProviderPrime, cb.PrimeMultisigAddr, func(val uint64) bool {
		return val == sendAmount
	}, numOfRetries, waitTime, IsRecoverableError)
	require.NoError(t, err)

	fmt.Printf("Prime multisig addr funded\n")

	_, err = SendTx(ctx, txProviderPrime, primeGenesisWallet, sendAmount,
		cb.PrimeMultisigFeeAddr, primeCluster.NetworkConfig(), []byte{})
	require.NoError(t, err)

	err = wallet.WaitForAmount(context.Background(), txProviderPrime, cb.PrimeMultisigFeeAddr, func(val uint64) bool {
		return val == sendAmount
	}, numOfRetries, waitTime, IsRecoverableError)
	require.NoError(t, err)

	fmt.Printf("Prime multisig fee addr funded\n")

	vectorGenesisWallet, err := GetGenesisWalletFromCluster(vectorCluster.Config.TmpDir, 1)
	require.NoError(t, err)

	_, err = SendTx(ctx, txProviderVector, vectorGenesisWallet, sendAmount,
		cb.VectorMultisigAddr, vectorCluster.NetworkConfig(), []byte{})
	require.NoError(t, err)

	err = wallet.WaitForAmount(context.Background(), txProviderVector, cb.VectorMultisigAddr, func(val uint64) bool {
		return val == sendAmount
	}, numOfRetries, waitTime, IsRecoverableError)
	require.NoError(t, err)

	fmt.Printf("Vector multisig addr funded\n")

	_, err = SendTx(ctx, txProviderVector, vectorGenesisWallet, sendAmount,
		cb.VectorMultisigFeeAddr, vectorCluster.NetworkConfig(), []byte{})
	require.NoError(t, err)

	err = wallet.WaitForAmount(context.Background(), txProviderVector, cb.VectorMultisigFeeAddr, func(val uint64) bool {
		return val == sendAmount
	}, numOfRetries, waitTime, IsRecoverableError)
	require.NoError(t, err)

	fmt.Printf("Vector multisig fee addr funded\n")

	cb.StartValidators(t, bladeEpochSize)

	fmt.Printf("Validators started\n")

	cb.WaitForValidatorsReady(t)

	fmt.Printf("Validators ready\n")

	// need params for it to work properly
	primeTokenSupply := big.NewInt(int64(sendAmount))
	vectorTokenSupply := big.NewInt(int64(sendAmount))
	require.NoError(t, cb.RegisterChains(primeTokenSupply, vectorTokenSupply))

	fmt.Printf("Chain registered\n")

	// need params for it to work properly
	require.NoError(t, cb.GenerateConfigs(
		primeCluster,
		vectorCluster,
	))

	fmt.Printf("Configs generated\n")

	require.NoError(t, cb.StartValidatorComponents(ctx))
	fmt.Printf("Validator components started\n")

	require.NoError(t, cb.StartRelayer(ctx))
	fmt.Printf("Relayer started\n")

	return cb
}

func RunApexBridge(
	t *testing.T, ctx context.Context,
	opts ...CardanoBridgeOption,
) *ApexSystem {
	t.Helper()

	const (
		bladeValidatorsNum = 4
	)

	baseLogsDir := path.Join("../..", fmt.Sprintf("e2e-logs-cardano-%d", time.Now().UTC().Unix()), t.Name())

	primeCluster := SetupAndRunApexCardanoCluster(t, ctx, wallet.TestNetNetwork, GetNetworkMagic(wallet.TestNetNetwork, 0), 0, "prime", baseLogsDir)
	require.NotNil(t, primeCluster)

	time.Sleep(10000000 * time.Second)
	vectorCluster := SetupAndRunApexCardanoCluster(t, ctx, wallet.VectorTestNetNetwork, GetNetworkMagic(wallet.VectorTestNetNetwork, 0), 1, "vector", baseLogsDir)
	require.NotNil(t, vectorCluster)

	cb := SetupAndRunApexBridge(t,
		ctx,
		// path.Join(path.Dir(primeCluster.Config.TmpDir), "bridge"),
		"../../e2e-bridge-data-tmp-"+t.Name(),
		bladeValidatorsNum,
		primeCluster,
		vectorCluster,
		opts...,
	)

	fmt.Printf("Apex bridge setup done\n")

	return &ApexSystem{
		PrimeCluster:  primeCluster,
		VectorCluster: vectorCluster,
		Bridge:        cb,
	}
}

func (a *ApexSystem) GetPrimeGenesisWallet(t *testing.T) wallet.IWallet {
	t.Helper()

	primeGenesisWallet, err := GetGenesisWalletFromCluster(a.PrimeCluster.Config.TmpDir, 2)
	require.NoError(t, err)

	return primeGenesisWallet
}

func (a *ApexSystem) GetVectorGenesisWallet(t *testing.T) wallet.IWallet {
	t.Helper()

	vectorGenesisWallet, err := GetGenesisWalletFromCluster(a.VectorCluster.Config.TmpDir, 2)
	require.NoError(t, err)

	return vectorGenesisWallet
}

func (a *ApexSystem) GetPrimeTxProvider() wallet.ITxProvider {
	return wallet.NewTxProviderOgmios(a.PrimeCluster.OgmiosURL())
}

func (a *ApexSystem) GetVectorTxProvider() wallet.ITxProvider {
	return wallet.NewTxProviderOgmios(a.VectorCluster.OgmiosURL())
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

	user := NewTestApexUserWithExistingWallets(t, primePrivateKey, vectorPrivateKey, primeNetworkConfig.NetworkType, vectorNetworkConfig.NetworkType)

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
