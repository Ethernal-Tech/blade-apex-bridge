package cardanofw

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"os"
	"regexp"
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
	"github.com/stretchr/testify/require"
)

type CardanoChainInfo struct {
	NetworkAddress   string
	OgmiosURL        string
	BlockfrostURL    string
	BlockfrostAPIKey string
	MultisigAddr     []string
	FeeAddr          string
	SocketPath       string

	NativeTokens  []sendtx.TokenExchangeConfig
	GenesisWallet *cardanowallet.Wallet
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

	PrimeInfo   CardanoChainInfo
	VectorInfo  CardanoChainInfo
	CardanoInfo CardanoChainInfo
	NexusInfo   EVMChainInfo

	dataDirPath string

	bridgingAPIs []string

	FunderUser *TestApexUser
	Users      []*TestApexUser

	IsSkyline bool
}

type ContractParams struct {
	contractName    string
	contractAddress string
	functionName    string
	functionArgs    []string
}

type UpgradeSCParams struct {
	contractsDir   string
	contractParams []ContractParams
	gasLimit       uint64
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
			NewApexNetworkTypes(config.PrimeConfig, config.VectorConfig, config.CardanoConfig, config.NexusConfig))
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
		IsSkyline: false,
	}

	apex.Config.applyPremineFundingOptions(apex.Users)

	return apex, nil
}

func NewSkylineSystem(
	dataDirPath string, opts ...ApexSystemOptions,
) (*ApexSystem, error) {
	config := getDefaultSkylinexSystemConfig()
	for _, opt := range opts {
		opt(config)
	}

	config.PrimeConfig.MinOperationFee = DefaultMinOperationFee

	users := make([]*TestApexUser, config.UserCnt)

	var err error

	for i := range users {
		users[i], err = NewTestApexUser(
			NewApexNetworkTypes(config.PrimeConfig, config.VectorConfig, config.CardanoConfig, config.NexusConfig))
		if err != nil {
			return nil, fmt.Errorf("failed to create a new skyline user: %w", err)
		}
	}

	apex := &ApexSystem{
		Config:      config,
		Users:       users,
		dataDirPath: dataDirPath,
		chains: []ITestApexChain{
			NewTestCardanoChain(config.PrimeConfig),
			NewTestCardanoChain(config.CardanoConfig),
		},
		IsSkyline: true,
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
		framework.WithNativeTokenConfig("AP3X:AP3X:18:true"),
		framework.WithProxyContractsAdmin(bladeProxyAdmin.Address().String()),
		framework.WithPremine(bladeAdmin.Address(), bladeProxyAdmin.Address()),
	)

	// create validators
	a.validators = make([]*TestApexValidator, a.Config.BladeValidatorCount)

	for idx := range a.validators {
		a.validators[idx] = NewTestApexValidator(
			a.dataDirPath, idx+1, a.BridgeCluster, a.BridgeCluster.Servers[idx])
	}

	a.BridgeCluster.WaitForReady(t)
}

func (a *ApexSystem) GetBridgeNode(t *testing.T, idx int) *framework.TestServer {
	t.Helper()

	require.True(t, idx >= 0 && idx < len(a.BridgeCluster.Servers))

	return a.BridgeCluster.Servers[idx]
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
		if err := chain.PopulateApexSystem(t, a); err != nil {
			return err
		}
	}

	if a.IsSkyline {
		require.NotNil(t, a.PrimeInfo.GenesisWallet)
		require.NotNil(t, a.CardanoInfo.GenesisWallet)

		tokenPrime, _, err := GetTokenAndPolicyForVerificationKey(
			a.Config.PrimeConfig.ChainType, a.Config.PrimeConfig.NetworkType,
			a.PrimeInfo.GenesisWallet.VerificationKey, DefaultTokenName)
		require.NoError(t, err)

		tokenCardano, _, err := GetTokenAndPolicyForVerificationKey(
			a.Config.CardanoConfig.ChainType, a.Config.CardanoConfig.NetworkType,
			a.CardanoInfo.GenesisWallet.VerificationKey, DefaultTokenName)
		require.NoError(t, err)

		a.PrimeInfo.NativeTokens = []sendtx.TokenExchangeConfig{
			{
				DstChainID: ChainIDCardano,
				TokenName:  tokenPrime.String(),
			},
		}
		a.CardanoInfo.NativeTokens = []sendtx.TokenExchangeConfig{
			{
				DstChainID: ChainIDPrime,
				TokenName:  tokenCardano.String(),
			},
		}
	}

	a.InitTxSendChainConfiguration()

	return nil
}

func (a *ApexSystem) InitTxSendChainConfiguration() {
	txSenderChainConfigs := map[string]sendtx.ChainConfig{
		ChainIDPrime: {
			CardanoCliBinary:      ResolveCardanoCliBinary(a.Config.PrimeConfig.NetworkType),
			TxProvider:            cardanowallet.NewTxProviderOgmios(a.PrimeInfo.OgmiosURL),
			TestNetMagic:          a.Config.PrimeConfig.NetworkMagic,
			TTLSlotNumberInc:      ttlSlotNumberInc,
			MinUtxoValue:          MinUTxODefaultValue,
			MinBridgingFeeAmount:  a.Config.PrimeConfig.MinBridgingFee,
			MinOperationFeeAmount: a.Config.PrimeConfig.MinOperationFee,
			NativeTokens:          a.PrimeInfo.NativeTokens,
			PotentialFee:          PotentialFee,
		},
	}

	if a.Config.VectorConfig != nil && a.Config.VectorConfig.IsEnabled {
		txSenderChainConfigs[ChainIDVector] = sendtx.ChainConfig{
			CardanoCliBinary:      ResolveCardanoCliBinary(a.Config.VectorConfig.NetworkType),
			TxProvider:            cardanowallet.NewTxProviderOgmios(a.VectorInfo.OgmiosURL),
			TestNetMagic:          a.Config.VectorConfig.NetworkMagic,
			TTLSlotNumberInc:      ttlSlotNumberInc,
			MinUtxoValue:          MinUTxODefaultValue,
			MinBridgingFeeAmount:  a.Config.VectorConfig.MinBridgingFee,
			MinOperationFeeAmount: a.Config.VectorConfig.MinOperationFee,
			PotentialFee:          PotentialFee,
		}
	}

	if a.Config.CardanoConfig != nil && a.Config.CardanoConfig.IsEnabled {
		txSenderChainConfigs[ChainIDCardano] = sendtx.ChainConfig{
			CardanoCliBinary:      ResolveCardanoCliBinary(a.Config.CardanoConfig.NetworkType),
			TxProvider:            cardanowallet.NewTxProviderOgmios(a.CardanoInfo.OgmiosURL),
			TestNetMagic:          a.Config.CardanoConfig.NetworkMagic,
			TTLSlotNumberInc:      ttlSlotNumberInc,
			MinUtxoValue:          MinUTxODefaultValue,
			MinBridgingFeeAmount:  a.Config.CardanoConfig.MinBridgingFee,
			MinOperationFeeAmount: a.Config.CardanoConfig.MinOperationFee,
			NativeTokens:          a.CardanoInfo.NativeTokens,
			PotentialFee:          PotentialFee,
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

	_, err = chain.SendTx(
		ctx, pk, chain.GetHotWalletAddresses()[0], DfmToChainNativeTokenAmount(chainID, dfmAmount), nil, nil)

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
	if a.IsSkyline {
		return a.generateSkylineConfigs()
	} else {
		return a.generateReactorConfigs()
	}
}

func (a *ApexSystem) generateReactorConfigs() error {
	getHandler := func(callback CustomConfigHandler) func(data map[string]any) {
		return func(data map[string]any) {
			callback(a, data)
		}
	}

	err := a.execForEachValidator(func(i int, validator *TestApexValidator) error {
		serverIndx := i
		if a.Config.TargetOneClusterServer {
			serverIndx = 0
		}

		var args []string

		for _, chain := range a.chains {
			args = append(args, chain.GetGenerateConfigsParams(serverIndx)...)
		}

		err := validator.GenerateConfigs(
			a.Config.APIPortStart+i, a.Config.APIKey, a.Config.GetTelemetryForValidatorIdx(i), args...)
		if err != nil {
			return err
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
	})
	if err != nil {
		return err
	}

	return a.setBridgingAPIs()
}

func (a *ApexSystem) generateSkylineConfigs() error {
	getHandler := func(callback CustomConfigHandler) func(data map[string]any) {
		return func(data map[string]any) {
			callback(a, data)
		}
	}

	err := a.execForEachValidator(func(i int, validator *TestApexValidator) error {
		serverIndx := i
		if a.Config.TargetOneClusterServer {
			serverIndx = 0
		}

		var args []string

		for _, chain := range a.chains {
			args = append(args, chain.GetGenerateConfigsParams(serverIndx)...)
		}

		cardanoPrimeTokenName := a.CardanoInfo.NativeTokens[0].TokenName
		primeCardanoTokenName := a.PrimeInfo.NativeTokens[0].TokenName

		err := validator.GenerateSkylineConfigs(
			a.Config.APIPortStart+i, a.Config.APIKey, a.Config.GetTelemetryForValidatorIdx(i),
			cardanoPrimeTokenName, primeCardanoTokenName, args...)
		if err != nil {
			return err
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
) (map[string]*big.Int, error) {
	chain, err := a.getChain(chainID)
	if err != nil {
		return nil, err
	}

	balance, err := chain.GetAddressBalance(ctx, user.GetAddress(chainID))
	if err != nil {
		return nil, err
	}

	for key, value := range balance {
		balance[key] = ChainNativeTokenAmountToDfm(chainID, value)
	}

	return balance, err
}

func (a *ApexSystem) GetTokenNameForChains(chainID, dstChainID ChainID) string {
	for _, token := range a.GetCardanoInfo(chainID).NativeTokens {
		if token.DstChainID == dstChainID {
			return token.TokenName
		}
	}

	return ""
}

func (a *ApexSystem) WaitForGreaterAmount(
	ctx context.Context, user *TestApexUser, dstChain ChainID, srcChain ChainID,
	expectedAmount *big.Int, numRetries int, waitTime time.Duration, isNativeToken ...bool,
) error {
	var (
		lastAmount *big.Int
		err        error
	)

	lastAmount, err = a.WaitForAmount(ctx, user, dstChain, srcChain, func(val *big.Int) bool {
		return val.Cmp(expectedAmount) == 1
	}, numRetries, waitTime, isNativeToken...)

	if err != nil {
		return fmt.Errorf("amount mismatch: expected greater than %s, but received %s: %w",
			expectedAmount, lastAmount, err)
	}

	return nil
}

func (a *ApexSystem) WaitForExactAmount(
	ctx context.Context, user *TestApexUser, dstChain ChainID, srcChain ChainID,
	expectedAmount *big.Int, numRetries int, waitTime time.Duration, isNativeToken ...bool,
) error {
	var (
		lastAmount *big.Int
		err        error
	)

	lastAmount, err = a.WaitForAmount(ctx, user, dstChain, srcChain, func(val *big.Int) bool {
		return val.Cmp(expectedAmount) >= 0
	}, numRetries, waitTime, isNativeToken...)

	if err != nil {
		return fmt.Errorf("amount mismatch: expected %s, but received %s: %w",
			expectedAmount, lastAmount, err)
	} else if lastAmount.Cmp(expectedAmount) > 0 {
		return fmt.Errorf("amount mismatch: received amount %s is greater than expected %s",
			lastAmount, expectedAmount)
	}

	return nil
}

func (a *ApexSystem) WaitForAmount(
	ctx context.Context, user *TestApexUser, dstChain ChainID, srcChain string,
	cmpHandler func(*big.Int) bool, numRetries int, waitTime time.Duration, isNativeToken ...bool,
) (*big.Int, error) {
	return infracommon.ExecuteWithRetry(ctx, func(ctx context.Context) (*big.Int, error) {
		amounts, err := a.GetBalance(ctx, user, dstChain)
		if err != nil {
			return nil, err
		}

		currency := cardanowallet.AdaTokenName

		if len(isNativeToken) > 0 && isNativeToken[0] {
			currency = a.GetTokenNameForChains(dstChain, srcChain)
		}

		newBalance := amounts[currency]
		if newBalance == nil {
			newBalance = big.NewInt(0)
		}

		if !cmpHandler(newBalance) {
			return newBalance, infracommon.ErrRetryTryAgain
		}

		return newBalance, nil
	}, infracommon.WithRetryCount(numRetries), infracommon.WithRetryWaitTime(waitTime))
}

func (a *ApexSystem) WaitForRedistribution(
	ctx context.Context, chainID ChainID, cmpHandler func(*big.Int, *big.Int) bool, numRetries int, waitTime time.Duration,
) error {
	_, err := infracommon.ExecuteWithRetry(ctx, func(ctx context.Context) (*big.Int, error) {
		addrAmounts, err := a.GetBridgingAddressesTokenAmounts(ctx, chainID)
		if err != nil {
			return nil, err
		}

		firstAddrAmount := addrAmounts[0][cardanowallet.AdaTokenName]
		for i := 1; i < len(addrAmounts); i++ {
			if cmpHandler(firstAddrAmount, addrAmounts[i][cardanowallet.AdaTokenName]) {
				return nil, infracommon.ErrRetryTryAgain
			}
		}

		return nil, nil
	}, infracommon.WithRetryCount(numRetries), infracommon.WithRetryWaitTime(waitTime))

	return err
}

func (a *ApexSystem) DefundHotWallet(
	chain ChainID, defundReceiverAddress string, defundDfm *big.Int, defundNativeTokenAmount *big.Int,
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
		"--native-token-amount", defundNativeTokenAmount.String(),
		"--key", pk,
		"--addr", defundReceiverAddress,
	}, os.Stdout)
}

func (a *ApexSystem) UpdateBridgingAddressCount(
	ctx context.Context, sourceChain ChainID,
	addressCount int,
) error {
	pkBytes, err := a.GetBridgeAdmin().MarshallPrivateKey()
	if err != nil {
		return err
	}

	pk := hex.EncodeToString(pkBytes)

	return RunCommand(ResolveApexBridgeBinary(), []string{
		"bridge-admin", "update-bridging-addrs-count",
		"--bridge-url", a.GetBridgeDefaultJSONRPCAddr(),
		"--chain", sourceChain,
		"--key", pk,
		"--bridging-addresses-count", fmt.Sprintf("%d", addressCount),
	}, os.Stdout)
}

func (a *ApexSystem) GetBridgingAddressesTokenAmounts(
	ctx context.Context, sourceChain ChainID,
) ([]map[string]*big.Int, error) {
	bridingAddresses := []string{}

	switch sourceChain {
	case ChainIDPrime:
		bridingAddresses = a.PrimeInfo.MultisigAddr
	case ChainIDCardano:
		bridingAddresses = a.CardanoInfo.MultisigAddr
	case ChainIDVector:
		bridingAddresses = a.VectorInfo.MultisigAddr
	}

	txProvider, err := a.getChain(sourceChain)
	if err != nil {
		return nil, err
	}

	balances := make([]map[string]*big.Int, 0, len(bridingAddresses))

	for _, addr := range bridingAddresses {
		addrBalances, err := txProvider.GetAddressBalance(ctx, addr)
		if err != nil {
			return nil, err
		}

		if addrBalances[cardanowallet.AdaTokenName] == nil {
			addrBalances[cardanowallet.AdaTokenName] = big.NewInt(0)
		}

		balances = append(balances, addrBalances)
	}

	return balances, nil
}

func (a *ApexSystem) DelegateStakeAddress(
	ctx context.Context, sourceChain ChainID,
	bridgeAddressIndex int8, stakePoolID string,
	doRegister bool,
) error {
	pkBytes, err := a.GetBridgeAdmin().MarshallPrivateKey()
	if err != nil {
		return err
	}

	pk := hex.EncodeToString(pkBytes)

	chain, err := a.getChain(sourceChain)
	if err != nil {
		return err
	}

	cmnd := []string{
		"bridge-admin", "delegate-address-to-stake-pool",
		"--bridge-url", a.GetBridgeDefaultJSONRPCAddr(),
		"--chain", chain.ChainID(),
		"--key", pk,
		"--stake-pool", stakePoolID,
		"--bridge-address-index", fmt.Sprintf("%d", bridgeAddressIndex),
	}

	if doRegister {
		cmnd = append(cmnd, "--do-registration")
	}

	return RunCommand(ResolveApexBridgeBinary(), cmnd, os.Stdout)
}

func (a *ApexSystem) DeregisterStakeAddress(
	ctx context.Context, sourceChain ChainID,
	bridgeAddressIndex int8,
) error {
	pkBytes, err := a.GetBridgeAdmin().MarshallPrivateKey()
	if err != nil {
		return err
	}

	pk := hex.EncodeToString(pkBytes)

	chain, err := a.getChain(sourceChain)
	if err != nil {
		return err
	}

	return RunCommand(ResolveApexBridgeBinary(), []string{
		"bridge-admin", "deregister-stake-address",
		"--bridge-url", a.GetBridgeDefaultJSONRPCAddr(),
		"--chain", chain.ChainID(),
		"--key", pk,
		"--bridge-address-index", fmt.Sprintf("%d", bridgeAddressIndex),
	}, os.Stdout)
}

func (a *ApexSystem) RedistributeTokens(
	ctx context.Context, chainID ChainID,
) error {
	pkBytes, err := a.GetBridgeAdmin().MarshallPrivateKey()
	if err != nil {
		return err
	}

	pk := hex.EncodeToString(pkBytes)

	chain, err := a.getChain(chainID)
	if err != nil {
		return err
	}

	return RunCommand(ResolveApexBridgeBinary(), []string{
		"bridge-admin", "redistribute-bridging-addresses-tokens",
		"--bridge-url", a.GetBridgeDefaultJSONRPCAddr(),
		"--chain", chain.ChainID(),
		"--key", pk,
	}, os.Stdout)
}

func (a *ApexSystem) SubmitTx(
	ctx context.Context, sourceChain ChainID, sender *TestApexUser,
	receiverAddr string, lovelaceDfmAmount *big.Int, nativeTokenAmounts []cardanowallet.TokenAmount, data []byte,
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

	txHash, err := infracommon.ExecuteWithRetry(ctx, func(ctx context.Context) (string, error) {
		txHash, err := chain.SendTx(
			ctx, privateKey, receiverAddr,
			DfmToChainNativeTokenAmount(sourceChain, lovelaceDfmAmount), nativeTokenAmounts, data)
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
	t *testing.T, ctx context.Context,
	sourceChain ChainID, destinationChain ChainID,
	sender *TestApexUser, dfmAmount *big.Int, bridgingType sendtx.BridgingType, receivers ...*TestApexUser,
) string {
	t.Helper()

	const (
		numRetries = 5
		waitTime   = time.Second * 10
	)

	require.True(t, sourceChain != destinationChain)

	// check if sourceChain is supported
	require.True(t,
		sourceChain == ChainIDPrime ||
			sourceChain == ChainIDVector ||
			sourceChain == ChainIDNexus ||
			sourceChain == ChainIDCardano,
	)

	// check if destinationChain is supported
	require.True(t,
		destinationChain == ChainIDPrime ||
			destinationChain == ChainIDVector ||
			destinationChain == ChainIDNexus ||
			destinationChain == ChainIDCardano,
	)

	// check if bridging direction is supported
	require.False(t, (a.Config.VectorConfig == nil || !a.Config.VectorConfig.IsEnabled) &&
		(sourceChain == ChainIDVector || destinationChain == ChainIDVector))
	require.False(t, (a.Config.CardanoConfig == nil || !a.Config.CardanoConfig.IsEnabled) &&
		(sourceChain == ChainIDCardano || destinationChain == ChainIDCardano))
	require.False(t, (a.Config.NexusConfig == nil || !a.Config.NexusConfig.IsEnabled) &&
		(sourceChain == ChainIDNexus || destinationChain == ChainIDNexus))
	require.True(t,
		(sourceChain != ChainIDCardano && destinationChain != ChainIDCardano) ||
			(sourceChain == ChainIDCardano && destinationChain == ChainIDPrime) ||
			(sourceChain == ChainIDPrime && destinationChain == ChainIDCardano))

	// check if number of receivers is valid
	require.Greater(t, len(receivers), 0)
	require.Less(t, len(receivers), 5)

	feeAmount := DfmToChainNativeTokenAmount(sourceChain, new(big.Int).SetUint64(defaultMinBridgingFeeAmount))

	receiversMap := make(map[string]*big.Int, len(receivers))

	for _, receiver := range receivers {
		require.True(t, destinationChain != ChainIDVector || receiver.HasVectorWallet)
		require.True(t, destinationChain != ChainIDNexus || receiver.HasNexusWallet)
		require.True(t, destinationChain != ChainIDCardano || receiver.HasCardanoWallet)

		receiversMap[receiver.GetAddress(destinationChain)] = DfmToChainNativeTokenAmount(sourceChain, dfmAmount)
	}
	// check if users are valid for the bridging - do they have necessary wallets
	require.True(t, sourceChain != ChainIDVector || sender.HasVectorWallet)
	require.True(t, sourceChain != ChainIDNexus || sender.HasNexusWallet)
	require.True(t, sourceChain != ChainIDCardano || sender.HasCardanoWallet)

	privateKey, err := sender.GetPrivateKey(sourceChain)
	require.NoError(t, err)

	operationFee := uint64(0)
	if a.IsSkyline {
		operationFee = DefaultMinOperationFee
	}

	txHash, err := infracommon.ExecuteWithRetry(ctx, func(ctx context.Context) (string, error) {
		txHash, err := a.GetChainMust(t, sourceChain).BridgingRequest(
			ctx, destinationChain, privateKey, receiversMap, feeAmount, operationFee, bridgingType)
		if err != nil {
			if strings.Contains(err.Error(), "The transaction contains unknown UTxO references as inputs") {
				return "", infracommon.ErrRetryTryAgain
			}

			return "", err
		}

		return txHash, nil
	}, infracommon.WithRetryCount(numRetries), infracommon.WithRetryWaitTime(waitTime))
	require.NoError(t, err)

	return txHash
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

func (a *ApexSystem) UpdateBridgingAddressCounts(ctx context.Context) error {
	if len(a.Config.UpdateAddressCountChains) > 0 {
		addrCount := 1

		for _, chainID := range a.Config.UpdateAddressCountChains {
			switch chainID {
			case ChainIDPrime:
				addrCount = a.Config.PrimeConfig.BridgingAddressCnt
			case ChainIDCardano:
				addrCount = a.Config.CardanoConfig.BridgingAddressCnt
			}

			if err := a.UpdateBridgingAddressCount(ctx, chainID, addrCount); err != nil {
				return fmt.Errorf("update bridging address count failed for chain %s: %w", chainID, err)
			}

			fmt.Printf("Bridging address count of %s have been updated to %d\n", chainID, addrCount)
		}
	}

	return nil
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

func (a *ApexSystem) GetCardanoInfo(chainID string) CardanoChainInfo {
	switch chainID {
	case ChainIDPrime:
		return a.PrimeInfo
	case ChainIDVector:
		return a.VectorInfo
	case ChainIDCardano:
		return a.CardanoInfo
	default:
		return CardanoChainInfo{}
	}
}

func (a *ApexSystem) DeploySmartContract(
	contractsDir, contractName string, addressesOfDependencies []string,
) (string, error) {
	pkBytes, err := a.GetBridgeAdmin().MarshallPrivateKey()
	if err != nil {
		return "", err
	}

	var stdoutBuf bytes.Buffer

	err = RunCommand(ResolveApexBridgeBinary(), []string{
		"deploy-evm", "deploy-contract",
		"--contract-dir", contractsDir,
		"--contract-name", contractName,
		"--dependencies", strings.Join(addressesOfDependencies, ";"),
		"--key", hex.EncodeToString(pkBytes),
		"--url", a.GetBridgeDefaultJSONRPCAddr(),
		"--owner", a.GetBridgeAdmin().Address().String(),
		"--upgrade-admin", a.GetBridgeProxyAdmin().Address().String(),
	}, &stdoutBuf)

	output := stdoutBuf.String()
	fmt.Println(output)

	if err != nil {
		return "", fmt.Errorf("deploy contract command failed: %w", err)
	}

	re := regexp.MustCompile(`(?i)Proxy Address\s*=\s*(0x[0-9a-fA-F]{40})`)

	if match := re.FindStringSubmatch(output); len(match) >= 2 {
		return match[1], nil
	}

	return "", fmt.Errorf("proxy address not found")
}

func (a *ApexSystem) UpgradeSmartContract(upgradeParams *UpgradeSCParams) error {
	pkBytes, err := a.GetBridgeProxyAdmin().MarshallPrivateKey()
	if err != nil {
		return err
	}

	cmnd := []string{
		"deploy-evm", "upgrade",
		"--dir", upgradeParams.contractsDir,
		"--key", hex.EncodeToString(pkBytes),
		"--url", a.GetBridgeDefaultJSONRPCAddr(),
	}

	for _, contactParams := range upgradeParams.contractParams {
		parts := []string{contactParams.contractName, contactParams.contractAddress}

		if contactParams.functionName != "" {
			parts = append(parts, contactParams.functionName)
		}

		if len(contactParams.functionArgs) > 0 {
			parts = append(parts, strings.Join(contactParams.functionArgs, ";"))
		}

		cmnd = append(cmnd, "--contract", strings.Join(parts, ":"))
	}

	if upgradeParams.gasLimit > 0 {
		cmnd = append(cmnd, "--gas-limit", fmt.Sprintf("%d", upgradeParams.gasLimit))
	}

	return RunCommand(ResolveApexBridgeBinary(), cmnd, os.Stdout)
}
