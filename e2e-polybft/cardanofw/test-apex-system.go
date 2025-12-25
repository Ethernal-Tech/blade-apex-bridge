package cardanofw

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/framework"
	"github.com/0xPolygon/polygon-edge/types"
	infracommon "github.com/Ethernal-Tech/cardano-infrastructure/common"
	"github.com/Ethernal-Tech/cardano-infrastructure/sendtx"
	cardanowallet "github.com/Ethernal-Tech/cardano-infrastructure/wallet"
	"github.com/Ethernal-Tech/ethgo"
	"github.com/stretchr/testify/require"
)

type CardanoChainInfo struct {
	NetworkAddress   string
	OgmiosURL        string
	BlockfrostURL    string
	BlockfrostAPIKey string
	MultisigAddr     string
	FeeAddr          string
	SocketPath       string
}

func (ci *CardanoChainInfo) GetTxProvider() (cardanowallet.ITxProvider, error) {
	if ci.OgmiosURL != "" {
		return cardanowallet.NewTxProviderOgmios(ci.OgmiosURL), nil
	}

	if ci.BlockfrostURL != "" && ci.BlockfrostAPIKey != "" {
		return cardanowallet.NewTxProviderBlockFrost(ci.BlockfrostURL, ci.BlockfrostAPIKey), nil
	}

	return nil, errors.New("neither a blockfrost nor a ogmios is specified")
}

type EVMChainInfo struct {
	GatewayAddress types.Address
	RelayerAddress types.Address
	JSONRPCAddr    string
	AdminKey       *crypto.ECDSAKey
	FundBlockNum   uint64
}

type ApexSystem struct {
	BridgeCluster   *framework.TestCluster
	Config          *ApexSystemConfig
	bladeAdmin      *crypto.ECDSAKey
	bladeProxyAdmin *crypto.ECDSAKey

	validators  []*TestApexValidator
	relayerNode *framework.Node

	chains []ITestApexChain

	PrimeInfo  CardanoChainInfo
	VectorInfo CardanoChainInfo
	NexusInfo  EVMChainInfo

	dataDirPath string

	bridgingAPIs []string

	FunderUser *TestApexUser
	Users      []*TestApexUser
}

func NewApexSystem(
	dataDirPath string, opts ...ApexSystemOptions,
) (*ApexSystem, error) {
	config := getDefaultApexSystemConfig()
	for _, opt := range opts {
		opt(config)
	}

	initAllowedDirections(config)

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

func (a *ApexSystem) StopAll() error {
	fmt.Println("Stopping chains...")

	errs := make([]error, len(a.chains))
	wg := sync.WaitGroup{}

	wg.Add(len(a.chains))

	for i, chain := range a.chains {
		go func(idx int, chain ITestApexChain) {
			defer wg.Done()

			var err1, err2 error

			if err := chain.GetIndexer().Close(); err != nil {
				err1 = fmt.Errorf("failed to close chain indexer %d: %w", idx, err)
			}

			if err := chain.Stop(); err != nil {
				err2 = fmt.Errorf("failed to stop chain %d: %w", idx, err)
			}

			errs[idx] = errors.Join(err1, err2)
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

func (a *ApexSystem) CheckAndTerminateAPIProcess() error {
	fmt.Printf("Attempting to terminate process on port %d...\n", a.Config.APIPortStart)

	command := fmt.Sprintf("fuser -k %d/tcp", a.Config.APIPortStart)
	cmd := exec.Command("bash", "-c", command)

	err := cmd.Run()
	if err == nil {
		fmt.Printf("Process on port %d is terminated successfully\n", a.Config.APIPortStart)

		return nil
	}

	if isExitCode(err, 1) {
		fmt.Printf("Port %d is already free\n", a.Config.APIPortStart)

		return nil
	}

	fmt.Printf("Termination error: %v\n", err)

	return nil
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

	bladeProxyAdmin, err := crypto.GenerateECDSAKey()
	require.NoError(t, err)

	a.bladeAdmin = bladeAdmin
	a.bladeProxyAdmin = bladeProxyAdmin
	a.BridgeCluster = framework.NewTestCluster(t, a.Config.BladeValidatorCount,
		framework.WithBladeAdmin(bladeAdmin.Address().String()),
		framework.WithEpochReward(0),
		framework.WithNativeTokenConfig("Blade:BLADE:18:true"),
		framework.WithProxyContractsAdmin(bladeProxyAdmin.Address().String()),
		framework.WithNonValidators(a.Config.BladeNonValidatorCount),
		framework.WithSecretsCallback(func(addresses []types.Address, config *framework.TestClusterConfig) {
			for range addresses {
				config.StakeAmounts = append(config.StakeAmounts, ethgo.Ether(1000))
			}
		}),
	)

	// create validators
	a.validators = make([]*TestApexValidator, a.Config.BladeValidatorCount)

	for idx := range a.validators {
		a.validators[idx] = NewTestApexValidator(
			a.dataDirPath, idx+1, a.BridgeCluster.Servers[idx])
	}

	a.BridgeCluster.WaitForReady(t)
}

func (a *ApexSystem) GetBridgeNode(t *testing.T, idx int) *framework.TestServer {
	t.Helper()

	require.True(t, idx >= 0 && idx < len(a.BridgeCluster.Servers))

	return a.BridgeCluster.Servers[idx]
}

func (a *ApexSystem) AddNewValidator(
	t *testing.T, ctx context.Context, bladeNode *framework.TestServer,
) *TestApexValidator {
	t.Helper()

	idx := len(a.validators)
	validator := NewTestApexValidator(a.dataDirPath, idx+1, bladeNode)

	a.validators = append(a.validators, validator)

	for _, chain := range a.chains {
		require.NoError(t, chain.CreateWallets(validator))
		require.NoError(t, chain.CreateAddresses(a.bladeAdmin, a.GetBridgeDefaultJSONRPCAddr()))
	}

	require.NoError(t, a.generateConfigForValidator(idx))

	return validator
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
	// must not be parallelized because each request use same admin wallet
	for _, chain := range a.chains {
		if err := chain.CreateAddresses(a.bladeAdmin, a.GetBridgeDefaultJSONRPCAddr()); err != nil {
			return err
		}
	}

	return nil
}

func (a *ApexSystem) InitContracts(ctx context.Context) error {
	// must not be parallelized because each request use same admin wallet
	for _, chain := range a.chains {
		if err := chain.InitContracts(ctx, a.bladeAdmin, a.GetBridgeDefaultJSONRPCAddr()); err != nil {
			return err
		}
	}

	return nil
}

func (a *ApexSystem) FinishConfiguring(t *testing.T) error {
	t.Helper()

	// after contracts have been initialized populate all the needed things into apex object
	for _, chain := range a.chains {
		if err := chain.PopulateApexSystem(a); err != nil {
			return err
		}
	}

	a.InitTxSendChainConfiguration()

	return nil
}

func (a *ApexSystem) UpdateConfigs() error {
	if err := a.CreateAddresses(); err != nil {
		return err
	}

	for _, chain := range a.chains {
		if err := chain.PopulateApexSystem(a); err != nil {
			return err
		}
	}

	return nil
}

func (a *ApexSystem) RestartBridges(ctx context.Context, validatorsNotToStart ...int) error {
	for _, validator := range a.validators {
		if err := validator.Stop(); err != nil {
			return err
		}
	}

	for i, validator := range a.validators {
		hasAPI := a.Config.APIValidatorID == -1 || validator.ID == a.Config.APIValidatorID

		if !slices.Contains(validatorsNotToStart, i) {
			if err := validator.Start(ctx, hasAPI); err != nil {
				return err
			}
		}
	}

	return nil
}

func (a *ApexSystem) InitTxSendChainConfiguration() {
	txSenderChainConfigs := map[string]sendtx.ChainConfig{
		ChainIDPrime: {
			CardanoCliBinary:     ResolveCardanoCliBinary(a.Config.PrimeConfig.NetworkType),
			TxProvider:           cardanowallet.NewTxProviderOgmios(a.PrimeInfo.OgmiosURL),
			TestNetMagic:         a.Config.PrimeConfig.NetworkMagic,
			TTLSlotNumberInc:     ttlSlotNumberInc,
			MinUtxoValue:         MinUTxODefaultValue,
			MinBridgingFeeAmount: a.Config.PrimeConfig.MinBridgingFee,
			PotentialFee:         PotentialFee,
		},
	}

	if a.Config.VectorConfig != nil && a.Config.VectorConfig.IsEnabled {
		txSenderChainConfigs[ChainIDVector] = sendtx.ChainConfig{
			CardanoCliBinary:     ResolveCardanoCliBinary(a.Config.VectorConfig.NetworkType),
			TxProvider:           cardanowallet.NewTxProviderOgmios(a.VectorInfo.OgmiosURL),
			TestNetMagic:         a.Config.VectorConfig.NetworkMagic,
			TTLSlotNumberInc:     ttlSlotNumberInc,
			MinUtxoValue:         MinUTxODefaultValue,
			MinBridgingFeeAmount: a.Config.VectorConfig.MinBridgingFee,
			PotentialFee:         PotentialFee,
		}
	}

	if a.Config.NexusConfig != nil && a.Config.NexusConfig.IsEnabled {
		txSenderChainConfigs[ChainIDNexus] = sendtx.ChainConfig{
			MinBridgingFeeAmount: a.Config.NexusConfig.MinBridgingFee,
		}
	}

	// set txSenderChainConfigs configuration for each chain
	for _, chain := range a.chains {
		chain.UpdateTxSendChainConfiguration(txSenderChainConfigs)
	}
}

func (a *ApexSystem) FundWallets(ctx context.Context) error {
	return a.execForEachChain(func(chain ITestApexChain) error {
		return chain.FundWallets(ctx)
	})
}

func (a *ApexSystem) FundChainHotWallet(ctx context.Context, chainID string, dfmAmount *big.Int) error {
	chain, err := a.getChain(chainID)
	if err != nil {
		return err
	}

	pk, err := chain.GetAdminPrivateKey()
	if err != nil {
		return err
	}

	receivers := []GenericTxReceiver{
		{
			Addr:   chain.GetHotWalletAddress(),
			Amount: DfmToChainNativeTokenAmount(chainID, dfmAmount),
		},
	}

	_, err = chain.SendTx(ctx, pk, nil, receivers)

	return err
}

func (a *ApexSystem) RegisterChains() error {
	return a.execForEachValidator(func(i int, validator *TestApexValidator) error {
		for _, chain := range a.chains {
			if err := chain.RegisterChain(validator); err != nil {
				return fmt.Errorf("operation failed for validator = %d and chain = %s: %w",
					i, chain.ChainID(), err)
			}
		}

		return nil
	})
}

func (a *ApexSystem) GenerateConfigs() error {
	err := a.execForEachValidator(func(i int, validator *TestApexValidator) error {
		return a.generateConfigForValidator(i)
	})
	if err != nil {
		return err
	}

	return a.setBridgingAPIs()
}

func (a *ApexSystem) GetBridgeDefaultJSONRPCAddr() string {
	return a.BridgeCluster.Servers[0].JSONRPCAddr()
}

func (a *ApexSystem) GetBridgeAdmin() *crypto.ECDSAKey {
	return a.bladeAdmin
}

func (a *ApexSystem) GetBridgeProxyAdmin() *crypto.ECDSAKey {
	return a.bladeProxyAdmin
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

func (a *ApexSystem) setBridgingAPIs() error {
	var bridgingAPIs []string

	for _, validator := range a.validators {
		hasAPI := a.Config.APIValidatorID == -1 || validator.ID == a.Config.APIValidatorID

		if hasAPI {
			if validator.APIPort == 0 {
				return fmt.Errorf("api port not defined")
			}

			bridgingAPIs = append(bridgingAPIs, fmt.Sprintf("http://localhost:%d", validator.APIPort))
		}
	}

	a.bridgingAPIs = bridgingAPIs

	return nil
}

func (a *ApexSystem) GetBridgingAPIs() ([]string, error) {
	if len(a.bridgingAPIs) == 0 {
		return nil, fmt.Errorf("not running API")
	}

	return a.bridgingAPIs, nil
}

func (a *ApexSystem) GetBridgingAPI() (string, error) {
	apis, err := a.GetBridgingAPIs()
	if err != nil {
		return "", err
	}

	return apis[0], nil
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
	chain, err := a.getChain(chainID)
	if err != nil {
		return nil, err
	}

	amount, err := chain.GetAddressBalance(ctx, user.GetAddress(chainID))
	if err != nil {
		return nil, err
	}

	return ChainNativeTokenAmountToDfm(chainID, amount), nil
}

func (a *ApexSystem) WaitForGreaterAmount(
	ctx context.Context, user *TestApexUser, chain ChainID,
	expectedAmountDfm *big.Int, numRetries int, waitTime time.Duration,
) error {
	lastAmount, err := a.WaitForAmount(ctx, user, chain, func(val *big.Int) bool {
		return val.Cmp(expectedAmountDfm) == 1
	}, numRetries, waitTime)
	if err != nil {
		return fmt.Errorf("amount mismatch: expected %s, but received %s: %w",
			expectedAmountDfm, lastAmount, err)
	}

	return nil
}

func (a *ApexSystem) WaitForAmountInRange(
	ctx context.Context, user *TestApexUser, chain ChainID,
	lowerBoundaryDfm *big.Int, higherBoundaryDfm *big.Int, numRetries int, waitTime time.Duration,
) error {
	lastAmount, err := a.WaitForAmount(ctx, user, chain, func(val *big.Int) bool {
		return val.Cmp(lowerBoundaryDfm) == 1 && val.Cmp(higherBoundaryDfm) == -1
	}, numRetries, waitTime)
	if err != nil {
		return fmt.Errorf("amount mismatch: expected amount between %s and %s, but received %s: %w",
			lowerBoundaryDfm, higherBoundaryDfm, lastAmount, err)
	}

	return nil
}

func (a *ApexSystem) WaitForExactAmount(
	ctx context.Context, user *TestApexUser, chain ChainID,
	expectedAmountDfm *big.Int, numRetries int, waitTime time.Duration,
) error {
	lastAmount, err := a.WaitForAmount(ctx, user, chain, func(val *big.Int) bool {
		return val.Cmp(expectedAmountDfm) >= 0
	}, numRetries, waitTime)
	if err != nil {
		return fmt.Errorf("amount mismatch: expected %s, but received %s: %w",
			expectedAmountDfm, lastAmount, err)
	} else if lastAmount.Cmp(expectedAmountDfm) > 0 {
		return fmt.Errorf("amount mismatch: received amount %s is greater than expected %s",
			lastAmount, expectedAmountDfm)
	}

	return nil
}

func (a *ApexSystem) WaitForAmount(
	ctx context.Context, user *TestApexUser, chain ChainID,
	cmpHandler func(*big.Int) bool, numRetries int, waitTime time.Duration,
) (*big.Int, error) {
	return infracommon.ExecuteWithRetry(ctx, func(ctx context.Context) (*big.Int, error) {
		newBalance, err := a.GetBalance(ctx, user, chain)
		if err != nil {
			return nil, err
		}

		if !cmpHandler(newBalance) {
			return newBalance, infracommon.ErrRetryTryAgain
		}

		return newBalance, nil
	}, infracommon.WithRetryCount(numRetries), infracommon.WithRetryWaitTime(waitTime))
}

func (a *ApexSystem) DefundHotWallet(
	chain ChainID, defundReceiverAddress string, defundDfm *big.Int,
) error {
	pkBytes, err := a.GetBridgeAdmin().MarshallPrivateKey()
	if err != nil {
		return err
	}

	pk := hex.EncodeToString(pkBytes)

	return RunCommand(ResolveApexBridgeBinary(), []string{
		"bridge-admin", "defund",
		"--bridge-url", a.GetBridgeDefaultJSONRPCAddr(),
		"--chain", chain,
		"--amount", defundDfm.String(),
		"--key", pk,
		"--addr", defundReceiverAddress,
	}, os.Stdout)
}

func (a *ApexSystem) SubmitTx(
	ctx context.Context, sourceChain ChainID, sender *TestApexUser,
	receiverAddr string, dfmAmount *big.Int, nativeTokens []cardanowallet.TokenAmount, data []byte,
) (string, error) {
	const (
		numRetries = 5
		waitTime   = time.Second * 10
	)

	privateKey, err := sender.GetPrivateKey(sourceChain)
	if err != nil {
		return "", err
	}

	chain, err := a.getChain(sourceChain)
	if err != nil {
		return "", err
	}

	receivers := []GenericTxReceiver{
		{
			Addr:         receiverAddr,
			Amount:       DfmToChainNativeTokenAmount(sourceChain, dfmAmount),
			NativeTokens: nativeTokens,
		},
	}

	txHash, err := infracommon.ExecuteWithRetry(ctx, func(ctx context.Context) (string, error) {
		txHash, err := chain.SendTx(ctx, privateKey, data, receivers)
		if err != nil {
			if strings.Contains(err.Error(), "The transaction contains unknown UTxO references as inputs") {
				return "", infracommon.ErrRetryTryAgain
			}

			return "", err
		}

		return txHash, nil
	}, infracommon.WithRetryCount(numRetries), infracommon.WithRetryWaitTime(waitTime))

	return txHash, err
}

func (a *ApexSystem) SubmitBridgingRequest(
	ctx context.Context,
	sourceChain ChainID, destinationChain ChainID,
	sender *TestApexUser, dfmAmount *big.Int, receivers ...*TestApexUser,
) (string, error) {
	const (
		numRetries = 5
		waitTime   = time.Second * 10

		numReceiversMin = 1
		numReceiversMax = 5
	)

	if sourceChain == destinationChain {
		return "", fmt.Errorf("source and destination chains are equal")
	}

	// check if sourceChain is supported
	isSourceChainSupported := sourceChain == ChainIDPrime ||
		sourceChain == ChainIDVector ||
		sourceChain == ChainIDNexus

	if !isSourceChainSupported {
		return "", fmt.Errorf("source chain is not supported")
	}

	// check if destinationChain is supported
	isDestinationChainSupported := destinationChain == ChainIDPrime ||
		destinationChain == ChainIDVector ||
		destinationChain == ChainIDNexus

	if !isDestinationChainSupported {
		return "", fmt.Errorf("destination chain is not supported")
	}

	// check if chains are configured and enabled
	if (a.Config.VectorConfig == nil || !a.Config.VectorConfig.IsEnabled) &&
		(sourceChain == ChainIDVector || destinationChain == ChainIDVector) {
		return "", fmt.Errorf("vector is not configured or enabled, but it is specified as source or destination")
	}

	if (a.Config.NexusConfig == nil || !a.Config.NexusConfig.IsEnabled) &&
		(sourceChain == ChainIDNexus || destinationChain == ChainIDNexus) {
		return "", fmt.Errorf("nexus is not configured or enabled, but it is specified as source or destination")
	}

	// check if bridging direction is supported
	isValidDirection := (sourceChain != ChainIDVector && destinationChain != ChainIDVector) ||
		(sourceChain == ChainIDVector && destinationChain == ChainIDPrime) ||
		(sourceChain == ChainIDPrime && destinationChain == ChainIDVector) ||
		(sourceChain != ChainIDNexus && destinationChain != ChainIDNexus) ||
		(sourceChain == ChainIDVector && destinationChain == ChainIDNexus) ||
		(sourceChain == ChainIDNexus && destinationChain == ChainIDVector)

	if !isValidDirection {
		return "", fmt.Errorf("invalid bridging direction")
	}

	// check if number of receivers is valid
	if len(receivers) < numReceiversMin ||
		len(receivers) > numReceiversMax {
		return "", fmt.Errorf("invalid number of receivers")
	}

	const feeAmountDfm = uint64(1_100_000)

	feeAmount := DfmToChainNativeTokenAmount(sourceChain, new(big.Int).SetUint64(feeAmountDfm))

	receiversMap := make(map[string]*big.Int, len(receivers))

	// check if receivers are valid for the bridging - do they have necessary wallets
	for i, receiver := range receivers {
		if destinationChain == ChainIDVector && !receiver.HasVectorWallet {
			return "", fmt.Errorf("receiver %d does not have a vector wallet for vector chain transfer", i)
		}

		if destinationChain == ChainIDNexus && !receiver.HasNexusWallet {
			return "", fmt.Errorf("receiver %d does not have a nexus wallet for nexus chain transfer", i)
		}

		receiversMap[receiver.GetAddress(destinationChain)] = DfmToChainNativeTokenAmount(sourceChain, dfmAmount)
	}

	// check if users are valid for the bridging - do they have necessary wallets
	if sourceChain == ChainIDVector && !sender.HasVectorWallet {
		return "", fmt.Errorf("sender does not have a vector wallet for vector chain transfer")
	}

	if sourceChain == ChainIDNexus && !sender.HasNexusWallet {
		return "", fmt.Errorf("sender does not have a nexus wallet for nexus chain transfer")
	}

	privateKey, err := sender.GetPrivateKey(sourceChain)
	if err != nil {
		return "", fmt.Errorf("error while retrieving the private key: %w", err)
	}

	srcChain, err := a.getChain(sourceChain)
	if err != nil {
		return "", err
	}

	txHash, err := infracommon.ExecuteWithRetry(ctx, func(ctx context.Context) (string, error) {
		txHash, err := srcChain.BridgingRequest(
			ctx, destinationChain, privateKey, receiversMap, feeAmount)
		if err != nil {
			if strings.Contains(err.Error(), "The transaction contains unknown UTxO references as inputs") {
				return "", infracommon.ErrRetryTryAgain
			}

			return "", err
		}

		return txHash, nil
	}, infracommon.WithRetryCount(numRetries), infracommon.WithRetryWaitTime(waitTime))
	if err != nil {
		return "", fmt.Errorf("error while submitting bridging request: %w", err)
	}

	return txHash, nil
}

func (a *ApexSystem) GetChainMust(t *testing.T, chainID ChainID) ITestApexChain {
	t.Helper()

	chain, err := a.getChain(chainID)
	require.NoError(t, err)

	return chain
}

func (a *ApexSystem) ResetIndexers() {
	_ = a.execForEachChain(func(chain ITestApexChain) error {
		chain.GetIndexer().ResetData()

		return nil
	})
}

func (a *ApexSystem) GetCardanoInfo(chainID string) CardanoChainInfo {
	switch chainID {
	case ChainIDPrime:
		return a.PrimeInfo
	case ChainIDVector:
		return a.VectorInfo
	default:
		return CardanoChainInfo{}
	}
}

func (a *ApexSystem) execForEachChain(handler func(chain ITestApexChain) error) error {
	errs := make([]error, len(a.chains))
	wg := &sync.WaitGroup{}

	wg.Add(len(a.chains))

	for i, ch := range a.chains {
		go func(idx int, chain ITestApexChain) {
			defer wg.Done()

			if err := handler(chain); err != nil {
				errs[idx] = fmt.Errorf("operation failed for chain %s: %w", chain.ChainID(), err)
			}
		}(i, ch)
	}

	wg.Wait()

	return errors.Join(errs...)
}

func (a *ApexSystem) execForEachValidator(handler func(i int, validator *TestApexValidator) error) error {
	errs := make([]error, len(a.validators))
	wg := &sync.WaitGroup{}

	wg.Add(len(a.validators))

	for i, valid := range a.validators {
		go func(idx int, validator *TestApexValidator) {
			defer wg.Done()

			if err := handler(idx, validator); err != nil {
				errs[idx] = fmt.Errorf("operation failed for validator = %d: %w", idx, err)
			}
		}(i, valid)
	}

	wg.Wait()

	return errors.Join(errs...)
}

func (a *ApexSystem) getChain(chainID string) (ITestApexChain, error) {
	for _, chain := range a.chains {
		if chain.ChainID() == chainID {
			return chain, nil
		}
	}

	return nil, fmt.Errorf("unknown chain: %s", chainID)
}

func (a *ApexSystem) generateConfigForValidator(i int) error {
	validator := a.validators[i]
	getHandler := func(callback CustomConfigHandler) func(data map[string]any) {
		return func(data map[string]any) {
			callback(a, data)
		}
	}

	serverIndx := i
	if a.Config.TargetOneClusterServer {
		serverIndx = 0
	}

	err := validator.GenerateConfigs(
		a.Config.APIPortStart+i, a.Config.APIKey, a.Config.GetTelemetryForValidatorIdx(i))
	if err != nil {
		return err
	}

	for _, chain := range a.chains {
		if err := chain.GenerateChainConfigs(serverIndx, validator); err != nil {
			return err
		}
	}

	if handler := a.Config.CustomOracleConfigHandler; handler != nil {
		fileName := validator.GetValidatorComponentsConfig()
		if err := UpdateJSONFile(fileName, fileName, getHandler(handler), false); err != nil {
			return err
		}
	}

	if handler := a.Config.CustomRelayerConfigHandler; handler != nil && RunRelayerOnValidatorID == validator.ID {
		fileName := validator.GetRelayerConfig()
		if err := UpdateJSONFile(fileName, fileName, getHandler(handler), false); err != nil {
			return err
		}
	}

	return nil
}
