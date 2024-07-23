package cardanofw

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/helper/common"
	cardanowallet "github.com/Ethernal-Tech/cardano-infrastructure/wallet"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"
)

const FundTokenAmount = uint64(100_000_000_000)

type ApexSystem struct {
	PrimeCluster  *TestCardanoCluster
	VectorCluster *TestCardanoCluster
	Nexus         *TestEVMChain
	Bridge        *TestCardanoBridge
}

type ApexSystemRunOpts struct {
	vectorEnabled bool
	evmEnabled    bool
}

func (as *ApexSystemRunOpts) CountApexServices() int {
	count := 0

	if as.vectorEnabled {
		count++
	}

	if as.evmEnabled {
		count++
	}

	return count
}

type ApexSystemOpts func(*ApexSystemRunOpts)

func WithVectorEnabled(vectorEnabled bool) ApexSystemOpts {
	return func(h *ApexSystemRunOpts) {
		h.vectorEnabled = vectorEnabled
	}
}

func WithEvmEnabled(evmEnabled bool) ApexSystemOpts {
	return func(h *ApexSystemRunOpts) {
		h.evmEnabled = evmEnabled
	}
}

func NewApexSystemRunOpts(opts ...ApexSystemOpts) *ApexSystemRunOpts {
	config := &ApexSystemRunOpts{
		vectorEnabled: true,
		evmEnabled:    false,
	}

	for _, opt := range opts {
		opt(config)
	}

	return config
}

func RunCardanoCluster(
	t *testing.T,
	ctx context.Context,
	id int,
	cardanoNodesNum int,
	networkType cardanowallet.CardanoNetworkType,
	genesisConfigDir string,
	baseLogsDir string,
) (*TestCardanoCluster, error) {
	t.Helper()

	networkMagic := GetNetworkMagic(networkType)
	logsDir := fmt.Sprintf("%s/%d", baseLogsDir, id)

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
		WithConfigGenesisDir(genesisConfigDir),
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
	)

	cleanupDataDir := func() {
		os.RemoveAll(dataDir)
	}

	cleanupDataDir()

	cb := NewTestCardanoBridge(dataDir, bladeValidatorsNum, opts...)

	require.NoError(t, cb.CardanoCreateWalletsAndAddresses(primeCluster.NetworkConfig(), vectorCluster.NetworkConfig()))

	fmt.Printf("Wallets and addresses created\n")

	txProviderPrime := cardanowallet.NewTxProviderOgmios(primeCluster.OgmiosURL())
	txProviderVector := cardanowallet.NewTxProviderOgmios(vectorCluster.OgmiosURL())

	primeGenesisWallet, err := GetGenesisWalletFromCluster(primeCluster.Config.TmpDir, 1)
	require.NoError(t, err)

	res, err := SendTx(ctx, txProviderPrime, primeGenesisWallet, sendAmount,
		cb.PrimeMultisigAddr, primeCluster.NetworkConfig(), []byte{})
	require.NoError(t, err)

	err = cardanowallet.WaitForAmount(context.Background(), txProviderPrime, cb.PrimeMultisigAddr, func(val uint64) bool {
		return val == FundTokenAmount
	}, numOfRetries, waitTime, IsRecoverableError)
	require.NoError(t, err)

	fmt.Printf("Prime multisig addr funded: %s\n", res)

	res, err = SendTx(ctx, txProviderPrime, primeGenesisWallet, sendAmount,
		cb.PrimeMultisigFeeAddr, primeCluster.NetworkConfig(), []byte{})
	require.NoError(t, err)

	err = cardanowallet.WaitForAmount(context.Background(), txProviderPrime, cb.PrimeMultisigFeeAddr, func(val uint64) bool {
		return val == FundTokenAmount
	}, numOfRetries, waitTime, IsRecoverableError)
	require.NoError(t, err)

	fmt.Printf("Prime multisig fee addr funded: %s\n", res)

	vectorGenesisWallet, err := GetGenesisWalletFromCluster(vectorCluster.Config.TmpDir, 1)
	require.NoError(t, err)

	res, err = SendTx(ctx, txProviderVector, vectorGenesisWallet, sendAmount,
		cb.VectorMultisigAddr, vectorCluster.NetworkConfig(), []byte{})
	require.NoError(t, err)

	err = cardanowallet.WaitForAmount(context.Background(), txProviderVector, cb.VectorMultisigAddr, func(val uint64) bool {
		return val == FundTokenAmount
	}, numOfRetries, waitTime, IsRecoverableError)
	require.NoError(t, err)

	fmt.Printf("Vector multisig addr funded: %s\n", res)

	res, err = SendTx(ctx, txProviderVector, vectorGenesisWallet, sendAmount,
		cb.VectorMultisigFeeAddr, vectorCluster.NetworkConfig(), []byte{})
	require.NoError(t, err)

	err = cardanowallet.WaitForAmount(context.Background(), txProviderVector, cb.VectorMultisigFeeAddr, func(val uint64) bool {
		return val == FundTokenAmount
	}, numOfRetries, waitTime, IsRecoverableError)
	require.NoError(t, err)

	fmt.Printf("Vector multisig fee addr funded: %s\n", res)

	cb.StartValidators(t, bladeEpochSize)

	fmt.Printf("Validators started\n")

	cb.WaitForValidatorsReady(t)

	fmt.Printf("Validators ready\n")

	// need params for it to work properly
	primeTokenSupply := new(big.Int).SetUint64(FundTokenAmount)
	vectorTokenSupply := new(big.Int).SetUint64(FundTokenAmount)
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
	apexOpts *ApexSystemRunOpts,
	opts ...CardanoBridgeOption,
) *ApexSystem {
	t.Helper()

	const (
		cardanoNodesNum    = 4
		bladeValidatorsNum = 4
		nexusValidatorsNum = 4
		nexusStartingPort  = int64(30400)
	)

	apexSystem := &ApexSystem{}
	wg := &sync.WaitGroup{}
	errorsContainer := [3]error{}

	wg.Add(1 + apexOpts.CountApexServices())

	go func() {
		defer wg.Done()

		apexSystem.PrimeCluster, errorsContainer[0] = RunCardanoCluster(
			t, ctx, 0, cardanoNodesNum, cardanowallet.TestNetNetwork, "prime", getCardanoBaseLogsDir(t, "prime"))
	}()

	if apexOpts.vectorEnabled {
		go func() {
			defer wg.Done()

			apexSystem.VectorCluster, errorsContainer[1] = RunCardanoCluster(
				t, ctx, 1, cardanoNodesNum, cardanowallet.VectorTestNetNetwork, "vector", getCardanoBaseLogsDir(t, "vector"))
		}()
	}

	if apexOpts.evmEnabled {
		go func() {
			defer wg.Done()

			apexSystem.Nexus, errorsContainer[2] = SetupAndRunEVMChain(t, nexusValidatorsNum, nexusStartingPort)
		}()
	}

	t.Cleanup(func() {
		fmt.Println("Stopping chains...")

		if apexSystem.PrimeCluster != nil {
			wg.Add(1)

			go func() {
				defer wg.Done()

				errorsContainer[0] = apexSystem.PrimeCluster.Stop()
			}()
		}

		if apexSystem.VectorCluster != nil {
			wg.Add(1)

			go func() {
				defer wg.Done()

				errorsContainer[1] = apexSystem.VectorCluster.Stop()
			}()
		}

		if apexSystem.Nexus != nil {
			wg.Add(1)

			go func() {
				defer wg.Done()

				apexSystem.Nexus.Cluster.Stop()
			}()
		}

		if apexSystem.Bridge != nil {
			wg.Add(1)

			go func() {
				defer wg.Done()

				fmt.Printf("Cleaning up apex bridge\n")
				apexSystem.Bridge.StopValidators()
				fmt.Printf("Done cleaning up apex bridge\n")
			}()
		}

		wg.Wait()

		fmt.Printf("Chains has been stopped...%v\n", errors.Join(errorsContainer[:]...))
	})

	fmt.Println("Starting chains...")

	wg.Wait()

	fmt.Println("Chains has been started...")

	require.NoError(t, errors.Join(errorsContainer[:]...))

	//nolint: godox
	// TODO: apex bridge should receive nexus too, even better whole ApexSystem struct
	apexSystem.Bridge = SetupAndRunApexBridge(t,
		ctx,
		// path.Join(path.Dir(primeCluster.Config.TmpDir), "bridge"),
		"../../e2e-bridge-data-tmp-"+t.Name(),
		bladeValidatorsNum,
		apexSystem.PrimeCluster,
		apexSystem.VectorCluster,
		opts...,
	)

	fmt.Printf("Apex bridge setup done\n")

	return apexSystem
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
