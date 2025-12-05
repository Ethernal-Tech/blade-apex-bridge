package cardanofw

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"os"
	"os/exec"
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

	// Bridging directions
	DestChain map[ChainID][]Direction
	// Tokens config
	Tokens map[uint16]sendtx.ApexToken

	GenesisWallet *cardanowallet.Wallet
}

type Direction struct {
	SourceTokenID      uint16 `json:"srcTokenID"`
	DestinationTokenID uint16 `json:"dstTokenID"`
	TrackSource        bool   `json:"trackSource"`
	TrackDestination   bool   `json:"trackDestination"`
}

type EcosystemToken struct {
	ID   uint16 `json:"id"`
	Name string `json:"name"`
}

type DirectionConfig struct {
	DestinationChain map[ChainID][]Direction     `json:"destChain"`
	Tokens           map[uint16]sendtx.ApexToken `json:"tokens"`
}

type DirectionConfigFile struct {
	Directions      map[string]DirectionConfig `json:"directions"`
	EcosystemTokens []EcosystemToken           `json:"ecosystemTokens"`
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

	// Bridging directions
	DestChain map[ChainID][]Direction
	// Tokens config
	Tokens map[uint16]sendtx.ApexToken
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

	EcosystemTokens map[uint16]string

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

type SetDependenciesSCParams struct {
	contractName string
	contractsDir string
	proxyAddress string
	dependencies []string
	gasLimit     uint64
}

func NewApexSystem(
	dataDirPath string, opts ...ApexSystemOptions,
) (*ApexSystem, error) {
	config := getDefaultApexSystemConfig()
	for _, opt := range opts {
		opt(config)
	}

	initAllowedDirections(config, false)

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
	config.VectorConfig.MinOperationFee = DefaultMinOperationFee

	initAllowedDirections(config, true)

	users := make([]*TestApexUser, config.UserCnt)

	var err error

	for i := range users {
		users[i], err = NewTestApexUser(
			NewApexNetworkTypes(config.PrimeConfig, config.VectorConfig, config.CardanoConfig, config.NexusConfig))
		if err != nil {
			return nil, fmt.Errorf("failed to create a new skyline user: %w", err)
		}
	}

	nexus, err := NewTestEVMChain(config.NexusConfig)
	if err != nil {
		return nil, err
	}

	apex := &ApexSystem{
		Config:      config,
		Users:       users,
		dataDirPath: dataDirPath,
		chains: []ITestApexChain{
			NewTestCardanoChain(config.PrimeConfig),
			NewTestCardanoChain(config.VectorConfig),
			NewTestCardanoChain(config.CardanoConfig),
			nexus,
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
			err := chain.CreateWallets(validator)
			if err != nil {
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
		require.NotNil(t, a.VectorInfo.GenesisWallet)
		require.NotNil(t, a.CardanoInfo.GenesisWallet)

		xadaToken, _, err := GetTokenAndPolicyForVerificationKey(
			a.Config.VectorConfig.ChainType, a.Config.VectorConfig.NetworkType,
			a.VectorInfo.GenesisWallet.VerificationKey, XADATokenName)
		require.NoError(t, err)

		capexToken, _, err := GetTokenAndPolicyForVerificationKey(
			a.Config.CardanoConfig.ChainType, a.Config.CardanoConfig.NetworkType,
			a.CardanoInfo.GenesisWallet.VerificationKey, CAP3XTokenName)
		require.NoError(t, err)

		for _, chain := range a.chains {
			if chain.ChainID() == ChainIDNexus {
				continue
			}

			if chain.GetCustodialAddress() == "" {
				continue
			}

			cgf := a.getCardanoConfig(chain.ChainID())
			info := a.GetCardanoInfo(chain.ChainID())
			nftToken, _, err := GetTokenAndPolicyForVerificationKey(
				cgf.ChainType, cgf.NetworkType,
				info.GenesisWallet.VerificationKey, MintNFTTokenName)
			require.NoError(t, err)

			chain.SetCustodialNFT(nftToken)
		}

		// By default we have the following directions:
		// - Prime <-> Cardano = AP3X <-> cAP3X
		// - Cardano <-> Vector = ADA <-> xADA
		// And tokens: AP3X, cAP3X, ADA, xADA

		a.PrimeInfo.DestChain = map[ChainID][]Direction{
			ChainIDCardano: {
				{
					SourceTokenID:      AP3XTokenID,
					DestinationTokenID: CAP3XTokenID,
					TrackSource:        true,
					TrackDestination:   true,
				},
			},
		}

		a.PrimeInfo.Tokens = map[uint16]sendtx.ApexToken{
			AP3XTokenID: {
				ChainSpecific:     cardanowallet.AdaTokenName,
				LockUnlock:        true,
				IsWrappedCurrency: false,
			},
		}

		a.CardanoInfo.DestChain = map[ChainID][]Direction{
			ChainIDPrime: {
				{
					SourceTokenID:      CAP3XTokenID,
					DestinationTokenID: AP3XTokenID,
					TrackSource:        true,
					TrackDestination:   true,
				},
			},
			ChainIDVector: {
				{
					SourceTokenID:      ADATokenID,
					DestinationTokenID: XADATokenID,
					TrackSource:        true,
					TrackDestination:   true,
				},
			},
		}

		a.CardanoInfo.Tokens = map[uint16]sendtx.ApexToken{
			ADATokenID: {
				ChainSpecific:     cardanowallet.AdaTokenName,
				LockUnlock:        true,
				IsWrappedCurrency: false,
			},
			CAP3XTokenID: {
				ChainSpecific:     capexToken.String(),
				LockUnlock:        true,
				IsWrappedCurrency: true,
			},
		}

		a.VectorInfo.DestChain = map[ChainID][]Direction{
			ChainIDCardano: {
				{
					SourceTokenID:      XADATokenID,
					DestinationTokenID: ADATokenID,
					TrackSource:        true,
					TrackDestination:   true,
				},
			},
		}

		a.VectorInfo.Tokens = map[uint16]sendtx.ApexToken{
			XADATokenID: {
				ChainSpecific:     xadaToken.String(),
				LockUnlock:        true,
				IsWrappedCurrency: true,
			},
			AP3XTokenID: {
				ChainSpecific:     cardanowallet.AdaTokenName,
				LockUnlock:        true,
				IsWrappedCurrency: false,
			},
		}

		a.EcosystemTokens = map[uint16]string{
			AP3XTokenID:  AP3XTokenName,
			ADATokenID:   ADATokenName,
			CAP3XTokenID: CAP3XTokenName,
			XADATokenID:  XADATokenName,
		}

		if a.Config.NexusConfig != nil && a.Config.NexusConfig.IsEnabled {
			// In case Nexus is enabled, we need to add:
			// - Nexus <-> Vector = USDT/xADA <-> wUSDT/xADA
			// - Nexus <-> Cardano = xADA <-> xADA
			a.NexusInfo.DestChain = map[ChainID][]Direction{
				ChainIDVector: {
					{
						SourceTokenID:      XADATokenID,
						DestinationTokenID: XADATokenID,
					},
					// {
					// 	SourceTokenID:      USDTTokenID,
					// 	DestinationTokenID: USDTTokenID,
					// },
				},
				ChainIDCardano: {
					{
						SourceTokenID:      XADATokenID,
						DestinationTokenID: ADATokenID,
						TrackDestination:   true,
					},
				},
			}

			a.NexusInfo.Tokens = map[uint16]sendtx.ApexToken{
				XADATokenID: {
					ChainSpecific:     "",
					LockUnlock:        false,
					IsWrappedCurrency: true,
				},
				// USDTTokenID: {
				// 	ChainSpecific:     "",
				// 	LockUnlock:        true,
				// 	IsWrappedCurrency: false,
				// },
				AP3XTokenID: { // currecny token on Nexus - required by validatorcomponents
					ChainSpecific:     cardanowallet.AdaTokenName,
					LockUnlock:        true,
					IsWrappedCurrency: false,
				},
			}

			a.VectorInfo.DestChain[ChainIDNexus] = []Direction{
				{
					SourceTokenID:      XADATokenID,
					DestinationTokenID: XADATokenID,
				},
				// {
				// 	SourceTokenID:      USDTTokenID,
				// 	DestinationTokenID: USDTTokenID,
				// },
			}

			// a.VectorInfo.Tokens[USDTTokenID] = sendtx.ApexToken{
			// 	ChainSpecific:     "",
			// 	LockUnlock:        false,
			// 	IsWrappedCurrency: false,
			// }

			a.CardanoInfo.DestChain[ChainIDNexus] = []Direction{
				{
					SourceTokenID:      ADATokenID,
					DestinationTokenID: XADATokenID,
					TrackSource:        true,
				},
			}

			// a.EcosystemTokens[USDTTokenID] = USDTTokenName
		}
	}

	a.InitTxSendChainConfiguration()

	return nil
}

func (a *ApexSystem) InitTxSendChainConfiguration() {
	txSenderChainConfigs := map[string]sendtx.ChainConfig{
		ChainIDPrime: {
			CardanoCliBinary:         ResolveCardanoCliBinary(a.Config.PrimeConfig.NetworkType),
			TxProvider:               cardanowallet.NewTxProviderOgmios(a.PrimeInfo.OgmiosURL),
			TestNetMagic:             a.Config.PrimeConfig.NetworkMagic,
			TTLSlotNumberInc:         ttlSlotNumberInc,
			MinUtxoValue:             MinUTxODefaultValue,
			DefaultMinFeeForBridging: a.Config.PrimeConfig.DefaultMinBridgingFee,
			MinFeeForBridgingTokens:  a.Config.PrimeConfig.MinBridgingFeeForTokens,
			MinOperationFeeAmount:    a.Config.PrimeConfig.MinOperationFee,
			PotentialFee:             PotentialFee,
			Tokens:                   a.PrimeInfo.Tokens,
		},
	}

	if a.Config.VectorConfig != nil && a.Config.VectorConfig.IsEnabled {
		txSenderChainConfigs[ChainIDVector] = sendtx.ChainConfig{
			CardanoCliBinary:         ResolveCardanoCliBinary(a.Config.VectorConfig.NetworkType),
			TxProvider:               cardanowallet.NewTxProviderOgmios(a.VectorInfo.OgmiosURL),
			TestNetMagic:             a.Config.VectorConfig.NetworkMagic,
			TTLSlotNumberInc:         ttlSlotNumberInc,
			MinUtxoValue:             MinUTxODefaultValue,
			DefaultMinFeeForBridging: a.Config.VectorConfig.DefaultMinBridgingFee,
			MinFeeForBridgingTokens:  a.Config.VectorConfig.MinBridgingFeeForTokens,
			PotentialFee:             PotentialFee,
			Tokens:                   a.VectorInfo.Tokens,
		}
	}

	if a.Config.CardanoConfig != nil && a.Config.CardanoConfig.IsEnabled {
		txSenderChainConfigs[ChainIDCardano] = sendtx.ChainConfig{
			CardanoCliBinary:         ResolveCardanoCliBinary(a.Config.CardanoConfig.NetworkType),
			TxProvider:               cardanowallet.NewTxProviderOgmios(a.CardanoInfo.OgmiosURL),
			TestNetMagic:             a.Config.CardanoConfig.NetworkMagic,
			TTLSlotNumberInc:         ttlSlotNumberInc,
			MinUtxoValue:             MinUTxODefaultValue,
			DefaultMinFeeForBridging: a.Config.CardanoConfig.DefaultMinBridgingFee,
			MinFeeForBridgingTokens:  a.Config.CardanoConfig.MinBridgingFeeForTokens,
			MinOperationFeeAmount:    a.Config.CardanoConfig.MinOperationFee,
			Tokens:                   a.CardanoInfo.Tokens,
			PotentialFee:             PotentialFee,
		}
	}

	if a.Config.NexusConfig != nil && a.Config.NexusConfig.IsEnabled {
		txSenderChainConfigs[ChainIDNexus] = sendtx.ChainConfig{
			DefaultMinFeeForBridging: a.Config.NexusConfig.MinBridgingFee,
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
			Addr:   chain.GetHotWalletAddresses()[0],
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

func (a *ApexSystem) DeployMintingContracts(ctx context.Context) error {
	if a.IsSkyline {
		return a.execForEachChain(func(chain ITestApexChain) error {
			err := chain.DeployMintingContract(ctx)
			if err != nil {
				return err
			}

			mintableTokens := chain.GetMintableTokens()

			if len(mintableTokens) > 0 {
				switch chain.ChainID() {
				case ChainIDCardano, ChainIDPrime, ChainIDVector:
					chainInfo := a.GetCardanoInfo(chain.ChainID())
					for tokenID, tokenName := range mintableTokens {
						token := chainInfo.Tokens[tokenID]
						token.ChainSpecific = tokenName
						chainInfo.Tokens[tokenID] = token
					}
				case ChainIDNexus:
					for tokenID, tokenName := range mintableTokens {
						token := a.NexusInfo.Tokens[tokenID]
						token.ChainSpecific = tokenName
						a.NexusInfo.Tokens[tokenID] = token
					}
				default:
					return fmt.Errorf("unimplemented cardano contract setup for chain %s", chain.ChainID())
				}
			}

			return nil
		})
	}

	return nil
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

		if handler := a.Config.CustomDirectionsConfigHandler; handler != nil {
			fileName := validator.GetDirectionsConfig()
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

	ecosystemTokens := make([]EcosystemToken, 0, len(a.EcosystemTokens))
	for id, name := range a.EcosystemTokens {
		ecosystemTokens = append(ecosystemTokens, EcosystemToken{ID: id, Name: name})
	}

	directionConfigFile := DirectionConfigFile{
		Directions: map[string]DirectionConfig{
			ChainIDPrime:  {DestinationChain: a.PrimeInfo.DestChain, Tokens: a.PrimeInfo.Tokens},
			ChainIDVector: {DestinationChain: a.VectorInfo.DestChain, Tokens: a.VectorInfo.Tokens},
		},
		EcosystemTokens: ecosystemTokens,
	}

	if a.Config.CardanoConfig != nil && a.Config.CardanoConfig.IsEnabled {
		directionConfigFile.Directions[ChainIDCardano] = DirectionConfig{
			DestinationChain: a.CardanoInfo.DestChain,
			Tokens:           a.CardanoInfo.Tokens,
		}
	}

	if a.Config.NexusConfig != nil && a.Config.NexusConfig.IsEnabled {
		directionConfigFile.Directions[ChainIDNexus] = DirectionConfig{
			DestinationChain: a.NexusInfo.DestChain,
			Tokens:           a.NexusInfo.Tokens,
		}
	}

	err := a.execForEachValidator(func(i int, validator *TestApexValidator) error {
		serverIndx := i
		if a.Config.TargetOneClusterServer {
			serverIndx = 0
		}

		err := validator.GenerateSkylineConfigs(
			a.Config.APIPortStart+i, a.Config.APIKey, a.Config.GetTelemetryForValidatorIdx(i),
		)
		if err != nil {
			return err
		}

		err = validator.GenerateDirectionsConfig(directionConfigFile)
		if err != nil {
			return err
		}

		for _, chain := range a.chains {
			if err := chain.GenerateChainConfigs(
				serverIndx, validator); err != nil {
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

		if handler := a.Config.CustomDirectionsConfigHandler; handler != nil {
			fileName := validator.GetDirectionsConfig()
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
	fmt.Printf("Getting balance for chain: %+v and user: %+v\n", chainID, user.GetAddress(chainID))
	balance, err := chain.GetAddressBalance(ctx, user.GetAddress(chainID))
	if err != nil {
		return nil, err
	}

	for key, value := range balance {
		balance[key] = ChainNativeTokenAmountToDfm(chainID, value)
	}

	return balance, err
}

func (a *ApexSystem) GetBalanceWithTokenName(ctx context.Context, user *TestApexUser, chainID ChainID, tokenName string) (map[string]*big.Int, error) {
	chain, err := a.getChain(chainID)
	if err != nil {
		return nil, err
	}

	return chain.GetAddressBalanceWithTokenName(ctx, user.GetAddress(chainID), tokenName)
}

func (a *ApexSystem) GetTokenNameForChain(chainID ChainID, tokenID uint16) string {
	cardanoInfo := a.GetCardanoInfo(chainID)
	for id, token := range cardanoInfo.Tokens {
		if id == tokenID {
			return token.ChainSpecific
		}
	}

	return ""
}

// Returns token name for the given dest chain
func (a *ApexSystem) GetTokenNameForChains(dstChainID, srcChainID ChainID, srcTokenID uint16) string {
	srcInfo := a.GetCardanoInfo(srcChainID)
	dstInfo := a.GetCardanoInfo(dstChainID)

	for _, direction := range srcInfo.DestChain[dstChainID] {
		if direction.SourceTokenID == srcTokenID {
			return dstInfo.Tokens[direction.DestinationTokenID].ChainSpecific
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

func (a *ApexSystem) WaitForAmountInRange(
	ctx context.Context, user *TestApexUser, dstChain ChainID, srcChain ChainID,
	lowerBoundaryDfm *big.Int, higherBoundaryDfm *big.Int, numRetries int, retryDelay time.Duration, isNativeToken ...bool,
) error {
	lastAmount, err := a.WaitForAmount(ctx, user, dstChain, srcChain, func(val *big.Int) bool {
		return val.Cmp(lowerBoundaryDfm) == 1 && val.Cmp(higherBoundaryDfm) != 1
	}, numRetries, retryDelay, isNativeToken...)
	if err != nil {
		return fmt.Errorf("amount mismatch: expected amount between %s and %s, but received %s: %w",
			lowerBoundaryDfm, higherBoundaryDfm, lastAmount, err)
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
	cmpHandler func(*big.Int) bool, numRetries int, retryDelay time.Duration, isNativeToken ...bool,
) (*big.Int, error) {
	return infracommon.ExecuteWithRetry(ctx, func(ctx context.Context) (*big.Int, error) {
		currency := cardanowallet.AdaTokenName

		if len(isNativeToken) > 0 && isNativeToken[0] {
			tokensInfo := a.GetBridgingTokensInfo(srcChain, dstChain, true)
			fmt.Printf("Tokens Info: %+v\n", tokensInfo)
			currency = tokensInfo.DstTokenName
		}

		var amounts map[string]*big.Int
		var err error

		amounts, err = a.GetBalanceWithTokenName(ctx, user, dstChain, currency)
		if err != nil {
			return nil, err
		}

		fmt.Printf("Amounts: %+v, currency: %+v\n", amounts, currency)

		newBalance := amounts[currency]
		if newBalance == nil {
			newBalance = big.NewInt(0)
		}

		if !cmpHandler(newBalance) {
			return newBalance, infracommon.ErrRetryTryAgain
		}

		return newBalance, nil
	}, infracommon.WithRetryCount(numRetries), infracommon.WithRetryWaitTime(retryDelay))
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

func (a *ApexSystem) UpdateChainTokenQuantity(
	chain ChainID, amount *big.Int, isWrappedToken bool,
) error {
	pkBytes, err := a.GetBridgeAdmin().MarshallPrivateKey()
	if err != nil {
		return err
	}

	pk := hex.EncodeToString(pkBytes)

	args := []string{
		"bridge-admin", "update-chain-token-quantity",
		"--bridge-url", a.GetBridgeDefaultJSONRPCAddr(),
		"--chain", chain,
		"--amount", amount.String(),
		"--key", pk,
	}

	if isWrappedToken {
		args = append(args, "--is-wrapped-token")
	}

	return RunCommand(ResolveApexBridgeBinary(), args, os.Stdout)
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
	case ChainIDVector:
		bridingAddresses = a.VectorInfo.MultisigAddr
	case ChainIDCardano:
		bridingAddresses = a.CardanoInfo.MultisigAddr
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
	receiverAddr string, lovelaceDfmAmount *big.Int, nativeTokens []cardanowallet.TokenAmount, data []byte,
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
			Amount:       DfmToChainNativeTokenAmount(sourceChain, lovelaceDfmAmount),
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
	sender *TestApexUser, dfmAmount *big.Int, bridgingType sendtx.BridgingType, receivers ...*TestApexUser,
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

	isSourceChainSupported := sourceChain == ChainIDPrime ||
		sourceChain == ChainIDVector ||
		sourceChain == ChainIDNexus ||
		sourceChain == ChainIDCardano

	if !isSourceChainSupported {
		return "", fmt.Errorf("source chain is not supported")
	}

	isDestinationChainSupported := destinationChain == ChainIDPrime ||
		destinationChain == ChainIDVector ||
		destinationChain == ChainIDNexus ||
		destinationChain == ChainIDCardano

	if !isDestinationChainSupported {
		return "", fmt.Errorf("destination chain is not supported")
	}

	// check if chains are configured and enabled
	if (a.Config.VectorConfig == nil || !a.Config.VectorConfig.IsEnabled) &&
		(sourceChain == ChainIDVector || destinationChain == ChainIDVector) {
		return "", fmt.Errorf("vector is not configured or enabled, but it is specified as source or destination")
	}

	if (a.Config.CardanoConfig == nil || !a.Config.CardanoConfig.IsEnabled) &&
		(sourceChain == ChainIDCardano || destinationChain == ChainIDCardano) {
		return "", fmt.Errorf("cardano is not configured or enabled, but it is specified as source or destination")
	}

	if (a.Config.NexusConfig == nil || !a.Config.NexusConfig.IsEnabled) &&
		(sourceChain == ChainIDNexus || destinationChain == ChainIDNexus) {
		return "", fmt.Errorf("nexus is not configured or enabled, but it is specified as source or destination")
	}

	// check if bridging direction is supported
	isSourceChainCardanoType := sourceChain == ChainIDCardano || sourceChain == ChainIDPrime || sourceChain == ChainIDVector
	if isSourceChainCardanoType {
		srcChainInfo := a.GetCardanoInfo(sourceChain)
		_, ok := srcChainInfo.DestChain[destinationChain]
		if !ok {
			return "", fmt.Errorf("invalid bridging direction")
		}
	} else if sourceChain == ChainIDNexus {
		srcChainInfo := a.GetNexusInfo(sourceChain)
		_, ok := srcChainInfo.DestChain[destinationChain]
		if !ok {
			return "", fmt.Errorf("invalid bridging direction")
		}
	} else {
		return "", fmt.Errorf("invalid source chain")
	}

	if len(receivers) < numReceiversMin ||
		len(receivers) > numReceiversMax {
		return "", fmt.Errorf("invalid number of receivers")
	}

	receiversMap := make(map[string]ReceiverAmount, len(receivers))

	// check if receivers are valid for the bridging - do they have necessary wallets
	for i, receiver := range receivers {
		if destinationChain == ChainIDVector && !receiver.HasVectorWallet {
			return "", fmt.Errorf("receiver %d does not have a vector wallet for vector chain transfer", i)
		}

		if destinationChain == ChainIDNexus && !receiver.HasNexusWallet {
			return "", fmt.Errorf("receiver %d does not have a nexus wallet for nexus chain transfer", i)
		}

		if destinationChain == ChainIDCardano && !receiver.HasCardanoWallet {
			return "", fmt.Errorf("receiver %d does not have a cardano wallet for cardano chain transfer", i)
		}

		switch sourceChain {
		case ChainIDCardano, ChainIDPrime, ChainIDVector:
			// Figure out source token ID
			tokenID := a.GetTokenIDForChain(sourceChain, bridgingType == sendtx.BridgingTypeCurrencyOnSource)
			if tokenID == 0 {
				return "", fmt.Errorf("source token ID not found for chain %s", sourceChain)
			}

			receiversMap[receiver.GetAddress(destinationChain)] = ReceiverAmount{
				TokenID: tokenID,
				Amount:  DfmToChainNativeTokenAmount(sourceChain, dfmAmount),
			}
		default:
			// TODO: Implement for nexus
			receiversMap[receiver.GetAddress(destinationChain)] = ReceiverAmount{
				TokenID: 5,
				Amount:  DfmToChainNativeTokenAmount(sourceChain, dfmAmount),
			}
		}
	}

	// check if users are valid for the bridging - do they have necessary wallets
	if sourceChain == ChainIDVector && !sender.HasVectorWallet {
		return "", fmt.Errorf("sender does not have a vector wallet for vector chain transfer")
	}

	if sourceChain == ChainIDNexus && !sender.HasNexusWallet {
		return "", fmt.Errorf("sender does not have a nexus wallet for nexus chain transfer")
	}

	if sourceChain == ChainIDCardano && !sender.HasCardanoWallet {
		return "", fmt.Errorf("sender does not have a cardano wallet for cardano chain transfer")
	}

	privateKey, err := sender.GetPrivateKey(sourceChain)
	if err != nil {
		return "", fmt.Errorf("error while retrieving the private key: %w", err)
	}

	operationFee := uint64(0)
	if a.IsSkyline {
		operationFee = DefaultMinOperationFee
	}

	srcChain, err := a.getChain(sourceChain)
	if err != nil {
		return "", err
	}

	feeAmount := DfmToChainNativeTokenAmount(
		sourceChain, new(big.Int).SetUint64(
			a.GetMinBridgingFee(sourceChain, bridgingType == sendtx.BridgingTypeWrappedTokenOnSource)))

	txHash, err := infracommon.ExecuteWithRetry(ctx, func(ctx context.Context) (string, error) {
		txHash, err := srcChain.BridgingRequest(
			ctx, destinationChain, privateKey, receiversMap, feeAmount, operationFee, bridgingType)
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

func (a *ApexSystem) GetTokenIDForChain(sourceChain ChainID, isCurrencyBridging bool) uint16 {
	if isCurrencyBridging {
		switch sourceChain {
		case ChainIDCardano:
			return ADATokenID
		case ChainIDPrime:
			return AP3XTokenID
		case ChainIDVector:
			return AP3XTokenID
		default:
			return 0
		}
	}

	switch sourceChain {
	case ChainIDCardano:
		return CAP3XTokenID
	case ChainIDVector:
		return XADATokenID
	default:
		return 0
	}
}

type BridgingTokensInfo struct {
	SrcTokenID   uint16
	DstTokenID   uint16
	SrcTokenName string
	DstTokenName string
}

func (a *ApexSystem) GetBridgingTokensInfo(srcChain, dstChain ChainID, expectNativeTokens bool, coloredCoins ...uint16) *BridgingTokensInfo {
	isSourceChainCardanoType := srcChain == ChainIDCardano || srcChain == ChainIDPrime || srcChain == ChainIDVector
	isDestinationChainCardanoType := dstChain == ChainIDCardano || dstChain == ChainIDPrime || dstChain == ChainIDVector

	if coloredCoins != nil {
		// TODO: Implement for colored coins
		fmt.Printf("Colored coins: %+v\n", coloredCoins)
		srcTokenID := coloredCoins[0]

		if isSourceChainCardanoType && isDestinationChainCardanoType {
			srcChainInfo := a.GetCardanoInfo(srcChain)
			dstChainInfo := a.GetCardanoInfo(dstChain)
			srcTokenName := srcChainInfo.Tokens[srcTokenID].ChainSpecific

			for _, direction := range srcChainInfo.DestChain[dstChain] {
				if direction.SourceTokenID == srcTokenID {
					return &BridgingTokensInfo{
						SrcTokenID:   srcTokenID,
						DstTokenID:   direction.DestinationTokenID,
						SrcTokenName: srcTokenName,
						DstTokenName: dstChainInfo.Tokens[direction.DestinationTokenID].ChainSpecific,
					}
				}
			}
		} else if srcChain != ChainIDNexus {
			srcChainInfo := a.GetCardanoInfo(srcChain)
			dstChainInfo := a.GetNexusInfo(dstChain)

			srcTokenName := srcChainInfo.Tokens[srcTokenID].ChainSpecific

			for _, direction := range srcChainInfo.DestChain[dstChain] {
				if direction.SourceTokenID == srcTokenID {
					return &BridgingTokensInfo{
						SrcTokenID:   srcTokenID,
						DstTokenID:   direction.DestinationTokenID,
						SrcTokenName: srcTokenName,
						DstTokenName: dstChainInfo.Tokens[direction.DestinationTokenID].ChainSpecific,
					}
				}
			}
		} else {
			srcChainInfo := a.GetNexusInfo(srcChain)
			dstChainInfo := a.GetCardanoInfo(dstChain)

			srcTokenName := srcChainInfo.Tokens[srcTokenID].ChainSpecific

			for _, direction := range srcChainInfo.DestChain[dstChain] {
				if direction.SourceTokenID == srcTokenID {
					return &BridgingTokensInfo{
						SrcTokenID:   srcTokenID,
						DstTokenID:   direction.DestinationTokenID,
						SrcTokenName: srcTokenName,
						DstTokenName: dstChainInfo.Tokens[direction.DestinationTokenID].ChainSpecific,
					}
				}
			}
		}

		return nil
	}

	if isSourceChainCardanoType && isDestinationChainCardanoType {
		srcChainInfo := a.GetCardanoInfo(srcChain)
		dstChainInfo := a.GetCardanoInfo(dstChain)

		srcTokenID := a.GetTokenIDForChain(srcChain, expectNativeTokens)

		for _, direction := range srcChainInfo.DestChain[dstChain] {
			if direction.SourceTokenID == srcTokenID {
				return &BridgingTokensInfo{
					SrcTokenID:   srcTokenID,
					DstTokenID:   direction.DestinationTokenID,
					SrcTokenName: srcChainInfo.Tokens[srcTokenID].ChainSpecific,
					DstTokenName: dstChainInfo.Tokens[direction.DestinationTokenID].ChainSpecific,
				}
			}
		}
	} else if srcChain != ChainIDNexus {
		srcChainInfo := a.GetCardanoInfo(srcChain)
		dstChainInfo := a.GetNexusInfo(dstChain)

		srcTokenID := a.GetTokenIDForChain(srcChain, expectNativeTokens)

		for _, direction := range srcChainInfo.DestChain[dstChain] {
			if direction.SourceTokenID == srcTokenID {
				return &BridgingTokensInfo{
					SrcTokenID:   srcTokenID,
					DstTokenID:   direction.DestinationTokenID,
					SrcTokenName: srcChainInfo.Tokens[srcTokenID].ChainSpecific,
					DstTokenName: dstChainInfo.Tokens[direction.DestinationTokenID].ChainSpecific,
				}
			}
		}
	} else {
		srcChainInfo := a.GetNexusInfo(srcChain)
		dstChainInfo := a.GetCardanoInfo(dstChain)

		dstTokenID := a.GetTokenIDForChain(dstChain, !expectNativeTokens)

		for _, direction := range srcChainInfo.DestChain[dstChain] {
			if direction.DestinationTokenID == dstTokenID {
				return &BridgingTokensInfo{
					SrcTokenID:   direction.SourceTokenID,
					DstTokenID:   dstTokenID,
					SrcTokenName: srcChainInfo.Tokens[direction.SourceTokenID].ChainSpecific,
					DstTokenName: dstChainInfo.Tokens[dstTokenID].ChainSpecific,
				}
			}
		}
	}

	return nil
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
			case ChainIDVector:
				addrCount = a.Config.VectorConfig.BridgingAddressCnt
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

func (a *ApexSystem) GetNexusInfo(chainID string) EVMChainInfo {
	switch chainID {
	case ChainIDNexus:
		return a.NexusInfo
	default:
		return EVMChainInfo{}
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

func (a *ApexSystem) SetDependencies(upgradeParams *SetDependenciesSCParams) error {
	pkBytes, err := a.GetBridgeAdmin().MarshallPrivateKey()
	if err != nil {
		return err
	}

	cmnd := []string{
		"deploy-evm", "set-dependencies",
		"--contract-dir", upgradeParams.contractsDir,
		"--contract-name", upgradeParams.contractName,
		"--proxy-addr", upgradeParams.proxyAddress,
		"--dependencies", strings.Join(upgradeParams.dependencies, ";"),
		"--key", hex.EncodeToString(pkBytes),
		"--url", a.GetBridgeDefaultJSONRPCAddr(),
	}

	if upgradeParams.gasLimit > 0 {
		cmnd = append(cmnd, "--gas-limit", fmt.Sprintf("%d", upgradeParams.gasLimit))
	}

	return RunCommand(ResolveApexBridgeBinary(), cmnd, os.Stdout)
}

func (a *ApexSystem) GetMinBridgingFee(chainID ChainID, isNativeTokenBridging bool) uint64 {
	switch chainID {
	case ChainIDNexus:
		return a.Config.NexusConfig.MinBridgingFee
	default:
		config := a.getCardanoConfig(chainID)

		if isNativeTokenBridging {
			return config.MinBridgingFeeForTokens
		}

		return config.DefaultMinBridgingFee
	}
}

func (a *ApexSystem) GetMinOperationFee(chainID ChainID) uint64 {
	return a.getCardanoConfig(chainID).MinOperationFee
}

func (a *ApexSystem) getCardanoConfig(chainID ChainID) *TestCardanoChainConfig {
	switch chainID {
	case ChainIDPrime:
		return a.Config.PrimeConfig
	case ChainIDVector:
		return a.Config.VectorConfig
	case ChainIDCardano:
		return a.Config.CardanoConfig
	default:
		return &TestCardanoChainConfig{}
	}
}
