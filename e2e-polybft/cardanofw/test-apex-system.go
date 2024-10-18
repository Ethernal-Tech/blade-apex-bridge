package cardanofw

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/framework"
	"github.com/0xPolygon/polygon-edge/types"
	cardanowallet "github.com/Ethernal-Tech/cardano-infrastructure/wallet"
	"github.com/stretchr/testify/require"
)

type CardanoChainInfo struct {
	NetworkAddress string
	OgmiosURL      string
}

type ApexSystem struct {
	BridgeCluster *framework.TestCluster
	Config        *ApexSystemConfig

	validators  []*TestApexValidator
	relayerNode *framework.Node

	chains []ITestApexChain

	PrimeMultisigAddr     string
	PrimeMultisigFeeAddr  string
	PrimeInfo             CardanoChainInfo
	VectorMultisigAddr    string
	VectorMultisigFeeAddr string
	VectorInfo            CardanoChainInfo

	nexusGatewayAddress types.Address
	nexusNode           *framework.TestServer
	nexusRelayerAddress types.Address

	bladeAdmin *crypto.ECDSAKey

	dataDirPath string

	Users []*TestApexUser
}

func NewApexSystem(
	dataDirPath string, opts ...ApexSystemOptions,
) (*ApexSystem, error) {
	config := getDefaultApexSystemConfig()
	for _, opt := range opts {
		opt(config)
	}

	nexus, err := NewTestEVMChain(config.NexusConfig)
	if err != nil {
		return nil, err
	}

	users := make([]*TestApexUser, config.UserCnt)
	for i := range users {
		users[i], err = NewTestApexUser(
			config.PrimeConfig.NetworkType,
			config.VectorConfig.IsEnabled,
			config.VectorConfig.NetworkType,
			config.NexusConfig.IsEnabled,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create a new apex user: %w", err)
		}
	}

	apex := &ApexSystem{
		Config:      config,
		Users:       users,
		dataDirPath: dataDirPath,
		chains: []ITestApexChain{
			NewTestCardanoChain(config.PrimeConfig),
			NewTestCardanoChain(config.VectorConfig),
			nexus,
		},
	}

	apex.Config.applyPremineFundingOptions(apex.Users)

	return apex, nil
}

func (a *ApexSystem) StopChains() error {
	fmt.Println("Stopping chains...")

	errs := make([]error, len(a.chains))
	wg := sync.WaitGroup{}

	wg.Add(len(a.chains))

	for i, chain := range a.chains {
		go func(idx int, chain ITestApexChain) {
			defer wg.Done()

			errs[idx] = chain.Stop()
		}(i, chain)
	}

	if a.BridgeCluster != nil {
		wg.Add(1)

		go func() {
			defer wg.Done()

			fmt.Printf("Cleaning up apex bridge\n")
			a.BridgeCluster.Stop()
			fmt.Printf("Done cleaning up apex bridge\n")
		}()
	}

	wg.Wait()

	err := errors.Join(errs...)

	fmt.Printf("Chains has been stopped...%v\n", err)

	return err
}

func (a *ApexSystem) StartChains(t *testing.T) error {
	t.Helper()

	return a.execForEachChain(func(chain ITestApexChain) error {
		return chain.RunChain(t)
	})
}

func (a *ApexSystem) StartBridgeChain(t *testing.T) {
	t.Helper()

	bladeAdmin, err := crypto.GenerateECDSAKey()
	require.NoError(t, err)

	a.bladeAdmin = bladeAdmin
	a.BridgeCluster = framework.NewTestCluster(t, a.Config.BladeValidatorCount,
		framework.WithBladeAdmin(bladeAdmin.Address().String()),
	)

	// create validators
	a.validators = make([]*TestApexValidator, a.Config.BladeValidatorCount)

	for idx := range a.validators {
		a.validators[idx] = NewTestApexValidator(
			a.dataDirPath, idx+1, a.BridgeCluster, a.BridgeCluster.Servers[idx])
	}

	a.BridgeCluster.WaitForReady(t)
}

func (a *ApexSystem) CreateWallets() (err error) {
	return a.execForEachValidator(func(i int, validator *TestApexValidator) error {
		for _, chain := range a.chains {
			if err := chain.CreateWallets(validator); err != nil {
				return fmt.Errorf("operation failed for validator = %d and chain = %s: %w",
					i, chain.ChainID(), err)
			}
		}

		return nil
	})
}

func (a *ApexSystem) CreateAddresses() error {
	return a.execForEachChain(func(chain ITestApexChain) error {
		return chain.CreateAddresses(a.bladeAdmin, a.GetBridgeDefaultJSONRPCAddr())
	})
}

func (a *ApexSystem) InitContractsAndFundWallets(ctx context.Context) error {
	return a.execForEachChain(func(chain ITestApexChain) error {
		if err := chain.InitContracts(a.bladeAdmin, a.GetBridgeDefaultJSONRPCAddr()); err != nil {
			return err
		}

		return chain.FundWallets(ctx)
	})
}

func (a *ApexSystem) RegisterChains() error {
	return a.execForEachValidator(func(i int, validator *TestApexValidator) error {
		for _, chain := range a.chains {
			if err := chain.CreateWallets(validator); err != nil {
				return fmt.Errorf("operation failed for validator = %d and chain = %s: %w",
					i, chain.ChainID(), err)
			}
		}

		return nil
	})
}

func (a *ApexSystem) GenerateConfigs() error {
	return a.execForEachValidator(func(i int, validator *TestApexValidator) error {
		telemetryConfig := ""
		if i == 0 {
			telemetryConfig = a.Config.TelemetryConfig
		}

		serverIndx := i
		if a.Config.TargetOneCardanoClusterServer {
			serverIndx = 0
		}

		var args []string

		for _, chain := range a.chains {
			args = append(args, chain.GetGenerateConfigsParams(serverIndx)...)
		}

		err := validator.GenerateConfigs(a.Config.APIPortStart+i, a.Config.APIKey, telemetryConfig, args...)
		if err != nil {
			return err
		}

		if handler := a.Config.CustomOracleHandler; handler != nil {
			fileName := validator.GetValidatorComponentsConfig()
			if err := UpdateJSONFile(fileName, fileName, handler, false); err != nil {
				return err
			}
		}

		if handler := a.Config.CustomRelayerHandler; handler != nil && RunRelayerOnValidatorID == validator.ID {
			fileName := validator.GetRelayerConfig()
			if err := UpdateJSONFile(fileName, fileName, handler, false); err != nil {
				return err
			}
		}

		return nil
	})
}

func (a *ApexSystem) GetPrimeTxProvider() cardanowallet.ITxProvider {
	return cardanowallet.NewTxProviderOgmios(a.PrimeInfo.OgmiosURL)
}

func (a *ApexSystem) GetVectorTxProvider() cardanowallet.ITxProvider {
	return cardanowallet.NewTxProviderOgmios(a.VectorInfo.OgmiosURL)
}

func (a *ApexSystem) GetNexusNode() *framework.TestServer {
	return a.nexusNode
}

func (a *ApexSystem) GetGatewayAddress() types.Address {
	return a.nexusGatewayAddress
}

func (a *ApexSystem) GetNexusRelayerWalletAddr() types.Address {
	return a.nexusRelayerAddress
}

func (a *ApexSystem) PrimeTransfer(
	t *testing.T, ctx context.Context,
	sender cardanowallet.IWallet, sendAmount uint64, receiver string,
) {
	t.Helper()

	a.transferCardano(
		t, ctx, a.GetPrimeTxProvider(),
		sender, sendAmount, receiver, a.Config.PrimeConfig.NetworkType)
}

func (a *ApexSystem) VectorTransfer(
	t *testing.T, ctx context.Context,
	sender cardanowallet.IWallet, sendAmount uint64, receiver string,
) {
	t.Helper()

	a.transferCardano(
		t, ctx, a.GetVectorTxProvider(),
		sender, sendAmount, receiver, a.Config.VectorConfig.NetworkType)
}

func (a *ApexSystem) transferCardano(
	t *testing.T,
	ctx context.Context, txProvider cardanowallet.ITxProvider,
	sender cardanowallet.IWallet, sendAmount uint64,
	receiver string, networkType cardanowallet.CardanoNetworkType,
) {
	t.Helper()

	prevAmount, err := GetTokenAmount(ctx, txProvider, receiver)
	require.NoError(t, err)

	_, err = SendTx(ctx, txProvider, sender,
		sendAmount, receiver, networkType, []byte{})
	require.NoError(t, err)

	err = cardanowallet.WaitForAmount(
		context.Background(), txProvider, receiver, func(val uint64) bool {
			return val == prevAmount+sendAmount
		}, 60, time.Second*2, IsRecoverableError)
	require.NoError(t, err)
}

func (a *ApexSystem) GetBridgeDefaultJSONRPCAddr() string {
	return a.BridgeCluster.Servers[0].JSONRPCAddr()
}

func (a *ApexSystem) GetBridgeAdmin() *crypto.ECDSAKey {
	return a.bladeAdmin
}

func (a *ApexSystem) GetValidatorsCount() int {
	return len(a.validators)
}

func (a *ApexSystem) GetValidator(t *testing.T, idx int) *TestApexValidator {
	t.Helper()

	require.True(t, idx >= 0 && idx < len(a.validators))

	return a.validators[idx]
}

func (a *ApexSystem) StartValidatorComponents(ctx context.Context) (err error) {
	for _, validator := range a.validators {
		hasAPI := a.Config.APIValidatorID == -1 || validator.ID == a.Config.APIValidatorID

		if err = validator.Start(ctx, hasAPI); err != nil {
			return err
		}
	}

	return err
}

func (a *ApexSystem) StartRelayer(ctx context.Context) (err error) {
	for _, validator := range a.validators {
		if RunRelayerOnValidatorID != validator.ID {
			continue
		}

		a.relayerNode, err = framework.NewNodeWithContext(ctx, ResolveApexBridgeBinary(), []string{
			"run-relayer",
			"--config", validator.GetRelayerConfig(),
		}, os.Stdout)
		if err != nil {
			return err
		}
	}

	return nil
}

func (a ApexSystem) StopRelayer() error {
	if a.relayerNode == nil {
		return errors.New("relayer not started")
	}

	return a.relayerNode.Stop()
}

func (a *ApexSystem) GetBridgingAPI() (string, error) {
	apis, err := a.GetBridgingAPIs()
	if err != nil {
		return "", err
	}

	return apis[0], nil
}

func (a *ApexSystem) GetBridgingAPIs() (res []string, err error) {
	for _, validator := range a.validators {
		hasAPI := a.Config.APIValidatorID == -1 || validator.ID == a.Config.APIValidatorID

		if hasAPI {
			if validator.APIPort == 0 {
				return nil, fmt.Errorf("api port not defined")
			}

			res = append(res, fmt.Sprintf("http://localhost:%d", validator.APIPort))
		}
	}

	if len(res) == 0 {
		return nil, fmt.Errorf("not running API")
	}

	return res, nil
}

func (a *ApexSystem) ApexBridgeProcessesRunning() bool {
	if a.relayerNode == nil || a.relayerNode.ExitResult() != nil {
		return false
	}

	for _, validator := range a.validators {
		if validator.node == nil || validator.node.ExitResult() != nil {
			return false
		}
	}

	return true
}

func (a *ApexSystem) GetBalance(
	ctx context.Context, user *TestApexUser, chainID ChainID,
) (*big.Int, error) {
	return a.GetAddressBalance(ctx, chainID, user.GetAddress(chainID))
}

func (a *ApexSystem) GetAddressBalance(
	ctx context.Context, chainID ChainID, address string,
) (*big.Int, error) {
	for _, chain := range a.chains {
		if chain.ChainID() == chainID {
			return chain.GetAddressBalance(ctx, address)
		}
	}

	return nil, fmt.Errorf("unsupported chain")
}

func (a *ApexSystem) WaitForGreaterAmount(
	ctx context.Context, user *TestApexUser, chain ChainID,
	expectedAmount *big.Int, numRetries int, waitTime time.Duration,
	isRecoverableError ...cardanowallet.IsRecoverableErrorFn,
) error {
	return a.WaitForAmount(ctx, user, chain, func(val *big.Int) bool {
		return val.Cmp(expectedAmount) == 1
	}, numRetries, waitTime)
}

func (a *ApexSystem) WaitForExactAmount(
	ctx context.Context, user *TestApexUser, chain ChainID,
	expectedAmount *big.Int, numRetries int, waitTime time.Duration,
	isRecoverableError ...cardanowallet.IsRecoverableErrorFn,
) error {
	return a.WaitForAmount(ctx, user, chain, func(val *big.Int) bool {
		return val.Cmp(expectedAmount) == 0
	}, numRetries, waitTime)
}

func (a *ApexSystem) WaitForAmount(
	ctx context.Context, user *TestApexUser, chain ChainID,
	cmpHandler func(*big.Int) bool, numRetries int, waitTime time.Duration,
	isRecoverableError ...cardanowallet.IsRecoverableErrorFn,
) error {
	if chain == ChainIDPrime || chain == ChainIDVector {
		isRecoverableError = append(isRecoverableError, IsRecoverableError)
	}

	return cardanowallet.ExecuteWithRetry(ctx, numRetries, waitTime, func() (bool, error) {
		newBalance, err := a.GetBalance(ctx, user, chain)

		return err == nil && cmpHandler(newBalance), err
	}, isRecoverableError...)
}

func (a *ApexSystem) SubmitTx(
	t *testing.T, ctx context.Context,
	sourceChain ChainID, destinationChain ChainID,
	sender *TestApexUser, receiverAddr string, amount *big.Int, data []byte,
) string {
	t.Helper()

	privateKey, err := sender.GetPrivateKey(sourceChain)
	require.NoError(t, err)

	for _, chain := range a.chains {
		if chain.ChainID() == sourceChain {
			txHash, err := chain.SendTx(ctx, privateKey, receiverAddr, amount, data)
			require.NoError(t, err)

			return txHash
		}
	}

	return ""
}

func (a *ApexSystem) SubmitBridgingRequest(
	t *testing.T, ctx context.Context,
	sourceChain ChainID, destinationChain ChainID,
	sender *TestApexUser, sendAmount *big.Int, receivers ...*TestApexUser,
) string {
	t.Helper()

	require.True(t, sourceChain != destinationChain)

	// check if sourceChain is supported
	require.True(t,
		sourceChain == ChainIDPrime ||
			sourceChain == ChainIDVector ||
			sourceChain == ChainIDNexus,
	)

	// check if destinationChain is supported
	require.True(t,
		destinationChain == ChainIDPrime ||
			destinationChain == ChainIDVector ||
			destinationChain == ChainIDNexus,
	)

	// check if bridging direction is supported
	require.False(t,
		!a.Config.VectorConfig.IsEnabled && (sourceChain == ChainIDVector || destinationChain == ChainIDVector))
	require.False(t,
		!a.Config.NexusConfig.IsEnabled && (sourceChain == ChainIDNexus || destinationChain == ChainIDNexus))
	require.True(t,
		sourceChain == ChainIDPrime ||
			(sourceChain == ChainIDVector && destinationChain == ChainIDPrime) ||
			(sourceChain == ChainIDNexus && destinationChain == ChainIDPrime),
	)

	// check if number of receivers is valid
	require.Greater(t, len(receivers), 0)
	require.Less(t, len(receivers), 5)

	// check if users are valid for the bridging - do they have necessary wallets
	require.True(t, sourceChain != ChainIDVector || sender.HasVectorWallet)
	require.True(t, sourceChain != ChainIDNexus || sender.HasNexusWallet)

	receiversMap := make(map[string]*big.Int, len(receivers))

	for _, receiver := range receivers {
		require.True(t, destinationChain != ChainIDVector || receiver.HasVectorWallet)
		require.True(t, destinationChain != ChainIDNexus || receiver.HasNexusWallet)

		receiversMap[receiver.GetAddress(destinationChain)] = sendAmount
	}

	privateKey, err := sender.GetPrivateKey(sourceChain)
	require.NoError(t, err)

	for _, chain := range a.chains {
		if chain.ChainID() == sourceChain {
			txHash, err := chain.BridgingRequest(ctx, destinationChain, privateKey, receiversMap)
			require.NoError(t, err)

			return txHash
		}
	}

	return ""
}

func (a *ApexSystem) execForEachChain(handler func(chain ITestApexChain) error) error {
	errs := make([]error, len(a.chains))
	wg := &sync.WaitGroup{}

	wg.Add(len(a.chains))

	for i, chain := range a.chains {
		go func(idx int, chain ITestApexChain) {
			defer wg.Done()

			if err := handler(chain); err != nil {
				errs[i] = fmt.Errorf("operation failed for chain %s: %w", chain.ChainID(), err)
			}
		}(i, chain)
	}

	wg.Wait()

	return errors.Join(errs...)
}

func (a *ApexSystem) execForEachValidator(
	handler func(i int, validator *TestApexValidator) error,
) error {
	errs := make([]error, len(a.validators))
	wg := &sync.WaitGroup{}

	wg.Add(len(a.validators))

	for i, validator := range a.validators {
		go func(i int, validator *TestApexValidator) {
			defer wg.Done()

			if err := handler(i, validator); err != nil {
				errs[i] = fmt.Errorf("operation failed for validator = %d: %w", i, err)
			}
		}(i, validator)
	}

	wg.Wait()

	return errors.Join(errs...)
}
