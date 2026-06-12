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
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/crypto"

	"github.com/0xPolygon/polygon-edge/e2e-polybft/framework"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
	infracommon "github.com/Ethernal-Tech/cardano-infrastructure/common"
	"github.com/Ethernal-Tech/cardano-infrastructure/sendtx"
	cardanowallet "github.com/Ethernal-Tech/cardano-infrastructure/wallet"
	"github.com/stretchr/testify/require"
)

type Token struct {
	ChainSpecific     string `json:"chainSpecific"`
	LockUnlock        bool   `json:"lockUnlock"`
	IsWrappedCurrency bool   `json:"isWrappedCurrency"`
}

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
	Tokens map[uint16]Token

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
	AlwaysTrackCurrencyAndWrappedCurrency bool                    `json:"alwaysTrackCurrencyAndWrappedCurrency"`
	DestinationChain                      map[ChainID][]Direction `json:"destChain"`
	Tokens                                map[uint16]Token        `json:"tokens"`
}

type DirectionConfigFile struct {
	Directions      map[string]DirectionConfig `json:"directions"`
	EcosystemTokens []EcosystemToken           `json:"ecosystemTokens"`
}

type ChainIDsConfigFile struct {
	ChainIDConfig []ChainIDConfig `json:"chainIDs"`
}

type ChainIDConfig struct {
	ChainID    string `json:"chainID"`
	ChainIDNum uint8  `json:"chainIDNum"`
	ChainType  string `json:"chainType,omitempty"`
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
	GatewayAddress           types.Address
	NativeTokenWalletAddress types.Address
	RelayerAddress           types.Address
	JSONRPCAddr              string
	AdminKey                 *crypto.ECDSAKey
	FundBlockNum             uint64

	// Bridging directions
	DestChain map[ChainID][]Direction
	// Tokens config
	Tokens map[uint16]Token
}

type SolanaChainInfo struct {
	DestChain      map[ChainID][]Direction
	Tokens         map[uint16]Token
	RelayerAddress string
	JSONRPCAddr    string
	ProgramID      string
	AltPublicKey   string
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
	PolygonInfo EVMChainInfo
	SolanaInfo  SolanaChainInfo

	EcosystemTokens map[uint16]string

	dataDirPath       string
	chainIDConfigPath string

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
	dataDirPath, chainIDsConfigPath string, opts ...ApexSystemOptions,
) (*ApexSystem, error) {
	config := getDefaultApexSystemConfig()
	for _, opt := range opts {
		opt(config)
	}

	config.PrimeConfig.MinOperationFee = 0
	config.VectorConfig.MinOperationFee = 0
	config.NexusConfig.MinOperationFee = big.NewInt(0)

	nexus, err := NewTestEVMChain(config.NexusConfig)
	if err != nil {
		return nil, err
	}

	users := make([]*TestApexUser, config.UserCnt)
	for i := range users {
		users[i], err = NewTestApexUser(
			NewApexNetworkTypes(ApexNetworkTypesParams{
				PrimeConfig: config.PrimeConfig, VectorConfig: config.VectorConfig, CardanoConfig: config.CardanoConfig,
				NexusConfig: config.NexusConfig, PolygonConfig: config.PolygonConfig,
			}),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create a new apex user: %w", err)
		}
	}

	apex := &ApexSystem{
		Config:            config,
		Users:             users,
		dataDirPath:       dataDirPath,
		chainIDConfigPath: chainIDsConfigPath,
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
	dataDirPath, chainIDsConfigPath string, opts ...ApexSystemOptions,
) (*ApexSystem, error) {
	config := getDefaultSkylineSystemConfig()
	for _, opt := range opts {
		opt(config)
	}

	users := make([]*TestApexUser, config.UserCnt)

	var err error

	for i := range users {
		users[i], err = NewTestApexUser(
			NewApexNetworkTypes(ApexNetworkTypesParams{
				PrimeConfig: config.PrimeConfig, VectorConfig: config.VectorConfig, CardanoConfig: config.CardanoConfig,
				NexusConfig: config.NexusConfig, PolygonConfig: config.PolygonConfig, SolanaConfig: config.SolanaConfig,
			}),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create a new skyline user: %w", err)
		}
	}

	nexus, err := NewTestEVMChain(config.NexusConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create nexus chain: %w", err)
	}

	polygon, err := NewTestEVMChain(config.PolygonConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create polygon chain: %w", err)
	}

	solana, err := NewTestSolanaChain(config.SolanaConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create solana chain: %w", err)
	}

	apex := &ApexSystem{
		Config:            config,
		Users:             users,
		dataDirPath:       dataDirPath,
		chainIDConfigPath: chainIDsConfigPath,
		chains: []ITestApexChain{
			NewTestCardanoChain(config.PrimeConfig),
			NewTestCardanoChain(config.VectorConfig),
			NewTestCardanoChain(config.CardanoConfig),
			nexus, polygon, solana,
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
		if err := chain.CreateAddresses(a.bladeAdmin, a.GetBridgeDefaultJSONRPCAddr(), a.GetChainIDsConfig()); err != nil {
			return err
		}
	}

	return nil
}

func (a *ApexSystem) InitContracts(ctx context.Context) error {
	// must not be parallelized because each request use same admin wallet
	for _, chain := range a.chains {
		if err := chain.InitContracts(ctx, a.bladeAdmin, a.GetBridgeDefaultJSONRPCAddr(), a.GetChainIDsConfig()); err != nil {
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
			if chain.ChainID() == ChainIDNexus ||
				chain.ChainID() == ChainIDPolygon ||
				chain.ChainID() == ChainIDSolana {
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

		a.PrimeInfo.Tokens = map[uint16]Token{
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

		a.CardanoInfo.Tokens = map[uint16]Token{
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

		a.VectorInfo.Tokens = map[uint16]Token{
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
					{
						SourceTokenID:      USDTTokenID,
						DestinationTokenID: USDTTokenID,
					},
				},
				ChainIDCardano: {
					{
						SourceTokenID:      XADATokenID,
						DestinationTokenID: ADATokenID,
						TrackDestination:   true,
					},
				},
			}

			a.NexusInfo.Tokens = map[uint16]Token{
				XADATokenID: {
					ChainSpecific:     "",
					LockUnlock:        false,
					IsWrappedCurrency: true,
				},
				USDTTokenID: {
					ChainSpecific:     "",
					LockUnlock:        true,
					IsWrappedCurrency: false,
				},
				AP3XTokenID: { // currency token on Nexus - required by validatorcomponents
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
				{
					SourceTokenID:      USDTTokenID,
					DestinationTokenID: USDTTokenID,
				},
			}

			a.VectorInfo.Tokens[USDTTokenID] = Token{
				ChainSpecific:     "",
				LockUnlock:        false,
				IsWrappedCurrency: false,
			}

			a.CardanoInfo.DestChain[ChainIDNexus] = []Direction{
				{
					SourceTokenID:      ADATokenID,
					DestinationTokenID: XADATokenID,
					TrackSource:        true,
				},
			}

			a.EcosystemTokens[USDTTokenID] = USDTTokenName

			if _, ok := a.Config.CardanoConfig.MintableTokens[USDCxTokenID]; ok {
				a.NexusInfo.DestChain[ChainIDCardano] = append(a.NexusInfo.DestChain[ChainIDCardano], Direction{
					SourceTokenID:      USDCTokenID,
					DestinationTokenID: USDCxTokenID,
				})
				a.NexusInfo.Tokens[USDCTokenID] = Token{
					ChainSpecific:     "",
					LockUnlock:        true,
					IsWrappedCurrency: false,
				}

				a.CardanoInfo.DestChain[ChainIDNexus] = append(a.CardanoInfo.DestChain[ChainIDNexus], Direction{
					SourceTokenID:      USDCxTokenID,
					DestinationTokenID: USDCTokenID,
				})
				a.CardanoInfo.Tokens[USDCxTokenID] = Token{
					ChainSpecific:     "",
					LockUnlock:        false,
					IsWrappedCurrency: false,
				}

				a.EcosystemTokens[USDCTokenID] = USDCTokenName
				a.EcosystemTokens[USDCxTokenID] = USDCxTokenName
			}

			if a.Config.PolygonConfig != nil && a.Config.PolygonConfig.IsEnabled {
				// In case Polygon is enabled, we need to add:
				// - Polygon <-> Nexus = wUSDT/MATIC <-> USDT/xMATIC
				a.NexusInfo.DestChain[ChainIDPolygon] = []Direction{
					{
						SourceTokenID:      USDTTokenID,
						DestinationTokenID: USDTTokenID,
					},
					{
						SourceTokenID:      XPOLTokenID,
						DestinationTokenID: POLTokenID,
					},
					{
						SourceTokenID:      AP3XTokenID,
						DestinationTokenID: PAP3XTokenID,
					},
				}

				a.NexusInfo.Tokens[XPOLTokenID] = Token{
					ChainSpecific:     "",
					LockUnlock:        false,
					IsWrappedCurrency: true,
				}

				a.PolygonInfo.DestChain = map[ChainID][]Direction{
					ChainIDNexus: {
						{
							SourceTokenID:      USDTTokenID,
							DestinationTokenID: USDTTokenID,
						},
						{
							SourceTokenID:      POLTokenID,
							DestinationTokenID: XPOLTokenID,
						},
						{
							SourceTokenID:      PAP3XTokenID,
							DestinationTokenID: AP3XTokenID,
						},
					},
				}

				a.PolygonInfo.Tokens = map[uint16]Token{
					USDTTokenID: {
						ChainSpecific:     "",
						LockUnlock:        false,
						IsWrappedCurrency: false,
					},
					POLTokenID: { // currency token on Polygon - required by validatorcomponents
						ChainSpecific:     cardanowallet.AdaTokenName,
						LockUnlock:        true,
						IsWrappedCurrency: false,
					},
					PAP3XTokenID: {
						ChainSpecific:     "",
						LockUnlock:        false,
						IsWrappedCurrency: true,
					},
				}

				a.EcosystemTokens[POLTokenID] = POLTokenName
				a.EcosystemTokens[XPOLTokenID] = XPOLTokenName
				a.EcosystemTokens[PAP3XTokenID] = PAP3XTokenName
			}
		}

		if a.Config.SolanaConfig != nil && a.Config.SolanaConfig.IsEnabled {
			if a.Config.VectorConfig != nil && a.Config.VectorConfig.IsEnabled {
				vsToken, _, err := GetTokenAndPolicyForVerificationKey(
					a.Config.VectorConfig.ChainType, a.Config.VectorConfig.NetworkType,
					a.VectorInfo.GenesisWallet.VerificationKey, VSTokenName)
				require.NoError(t, err)

				a.SolanaInfo.DestChain = map[ChainID][]Direction{
					ChainIDVector: {
						{
							SourceTokenID:      WSOLTokenID,
							DestinationTokenID: ASOLTokenID,
							TrackSource:        false, // true
							TrackDestination:   false,
						},
						{
							SourceTokenID:      SAP3XTokenID,
							DestinationTokenID: AP3XTokenID,
							TrackSource:        false,
							TrackDestination:   true,
						},
						{
							SourceTokenID:      VSTokenID,
							DestinationTokenID: VSTokenID,
							TrackSource:        false,
							TrackDestination:   false,
						},
					},
				}

				a.SolanaInfo.Tokens = map[uint16]Token{
					WSOLTokenID: {
						ChainSpecific:     WSOLMintAddress,
						LockUnlock:        true,
						IsWrappedCurrency: false, // true
					},
					SOLTokenID: {
						ChainSpecific:     cardanowallet.AdaTokenName,
						LockUnlock:        true,
						IsWrappedCurrency: false,
					},
					SAP3XTokenID: {
						ChainSpecific:     "",
						LockUnlock:        false,
						IsWrappedCurrency: true,
					},
					VSTokenID: {
						ChainSpecific:     "",
						LockUnlock:        false,
						IsWrappedCurrency: false,
					},
				}

				a.VectorInfo.DestChain[ChainIDSolana] = []Direction{
					{
						SourceTokenID:      ASOLTokenID,
						DestinationTokenID: WSOLTokenID,
						TrackSource:        false,
						TrackDestination:   false, // true
					},
					{
						SourceTokenID:      AP3XTokenID,
						DestinationTokenID: SAP3XTokenID,
						TrackSource:        true,
						TrackDestination:   false,
					},
					{
						SourceTokenID:      VSTokenID,
						DestinationTokenID: VSTokenID,
						TrackSource:        false,
						TrackDestination:   false,
					},
				}

				a.VectorInfo.Tokens[ASOLTokenID] = Token{
					ChainSpecific:     "",
					LockUnlock:        false,
					IsWrappedCurrency: false,
				}

				a.VectorInfo.Tokens[VSTokenID] = Token{
					ChainSpecific:     vsToken.String(),
					LockUnlock:        true,
					IsWrappedCurrency: false,
				}

				a.EcosystemTokens[WSOLTokenID] = WSOLANATokenName
				a.EcosystemTokens[SOLTokenID] = cardanowallet.AdaTokenName
				a.EcosystemTokens[ASOLTokenID] = ASOLTokenName
				a.EcosystemTokens[SAP3XTokenID] = SAP3XTokenName
				a.EcosystemTokens[VSTokenID] = VSTokenName
			}

			if a.Config.NexusConfig != nil && a.Config.NexusConfig.IsEnabled {
				a.SolanaInfo.DestChain[ChainIDNexus] = []Direction{
					{
						SourceTokenID:      WSOLTokenID,
						DestinationTokenID: ASOLTokenID,
						TrackSource:        false, // true
						TrackDestination:   false,
					},
					{
						SourceTokenID:      SAP3XTokenID,
						DestinationTokenID: AP3XTokenID,
						TrackSource:        false,
						TrackDestination:   true,
					},
					{
						SourceTokenID:      NSTokenID,
						DestinationTokenID: NSTokenID,
						TrackSource:        false,
						TrackDestination:   false,
					},
				}

				a.NexusInfo.DestChain[ChainIDSolana] = []Direction{
					{
						SourceTokenID:      ASOLTokenID,
						DestinationTokenID: WSOLTokenID,
						TrackSource:        false,
						TrackDestination:   false, // true
					},
					{
						SourceTokenID:      AP3XTokenID,
						DestinationTokenID: SAP3XTokenID,
						TrackSource:        true,
						TrackDestination:   false,
					},
					{
						SourceTokenID:      NSTokenID,
						DestinationTokenID: NSTokenID,
						TrackSource:        false,
						TrackDestination:   false,
					},
				}

				a.NexusInfo.Tokens[ASOLTokenID] = Token{
					ChainSpecific:     "",
					LockUnlock:        false,
					IsWrappedCurrency: false,
				}

				a.NexusInfo.Tokens[NSTokenID] = Token{
					ChainSpecific:     "",
					LockUnlock:        false,
					IsWrappedCurrency: false,
				}

				a.SolanaInfo.Tokens[NSTokenID] = Token{
					ChainSpecific:     "",
					LockUnlock:        false,
					IsWrappedCurrency: false,
				}

				a.EcosystemTokens[NSTokenID] = NSTokenName
			}
		}
	} else {
		require.NotNil(t, a.PrimeInfo.GenesisWallet)
		require.NotNil(t, a.VectorInfo.GenesisWallet)

		// By default we have the following directions:
		// - Prime <-> Vector = AP3X <-> AP3X
		// And tokens: AP3X

		a.PrimeInfo.DestChain = map[ChainID][]Direction{
			ChainIDVector: {
				{
					SourceTokenID:      AP3XTokenID,
					DestinationTokenID: AP3XTokenID,
					TrackSource:        true,
					TrackDestination:   true,
				},
			},
		}

		a.PrimeInfo.Tokens = map[uint16]Token{
			AP3XTokenID: {
				ChainSpecific:     cardanowallet.AdaTokenName,
				LockUnlock:        true,
				IsWrappedCurrency: false,
			},
		}

		a.VectorInfo.DestChain = map[ChainID][]Direction{
			ChainIDPrime: {
				{
					SourceTokenID:      AP3XTokenID,
					DestinationTokenID: AP3XTokenID,
					TrackSource:        true,
					TrackDestination:   true,
				},
			},
		}

		a.VectorInfo.Tokens = map[uint16]Token{
			AP3XTokenID: {
				ChainSpecific:     cardanowallet.AdaTokenName,
				LockUnlock:        true,
				IsWrappedCurrency: false,
			},
		}

		a.EcosystemTokens = map[uint16]string{
			AP3XTokenID: AP3XTokenName,
		}

		if a.Config.NexusConfig != nil && a.Config.NexusConfig.IsEnabled {
			// In case Nexus is enabled, we need to add:
			// - Nexus <-> Vector = AP3X <-> AP3X
			// - Nexus <-> Prime = AP3X <-> AP3X
			a.NexusInfo.DestChain = map[ChainID][]Direction{
				ChainIDPrime: {
					{
						SourceTokenID:      AP3XTokenID,
						DestinationTokenID: AP3XTokenID,
						TrackSource:        true,
						TrackDestination:   true,
					},
				},
				ChainIDVector: {
					{
						SourceTokenID:      AP3XTokenID,
						DestinationTokenID: AP3XTokenID,
						TrackSource:        true,
						TrackDestination:   true,
					},
				},
			}

			a.NexusInfo.Tokens = map[uint16]Token{
				AP3XTokenID: {
					ChainSpecific:     cardanowallet.AdaTokenName,
					LockUnlock:        true,
					IsWrappedCurrency: false,
				},
			}

			a.PrimeInfo.DestChain[ChainIDNexus] = []Direction{
				{
					SourceTokenID:      AP3XTokenID,
					DestinationTokenID: AP3XTokenID,
					TrackSource:        true,
					TrackDestination:   true,
				},
			}

			a.VectorInfo.DestChain[ChainIDNexus] = []Direction{
				{
					SourceTokenID:      AP3XTokenID,
					DestinationTokenID: AP3XTokenID,
					TrackSource:        true,
					TrackDestination:   true,
				},
			}
		}
	}

	a.InitTxSendChainConfiguration()

	if a.Config.SolanaConfig != nil && a.Config.SolanaConfig.IsEnabled {
		err := a.UpdateChainMaxNumberOfTransactions(ChainIDSolana, 4)
		if err != nil {
			return err
		}
	}

	return nil
}

func (a *ApexSystem) InitTxSendChainConfiguration() {
	primeTokens := make(map[uint16]sendtx.ApexToken, len(a.PrimeInfo.Tokens))
	for tID, t := range a.PrimeInfo.Tokens {
		primeTokens[tID] = sendtx.ApexToken{
			FullName:          t.ChainSpecific,
			IsWrappedCurrency: t.IsWrappedCurrency,
		}
	}

	txSenderChainConfigs := map[string]sendtx.ChainConfig{
		ChainIDPrime: {
			CardanoCliBinary:         ResolveCardanoCliBinary(a.Config.PrimeConfig.ChainType),
			TxProvider:               cardanowallet.NewTxProviderOgmios(a.PrimeInfo.OgmiosURL),
			TestNetMagic:             a.Config.PrimeConfig.NetworkMagic,
			TTLSlotNumberInc:         ttlSlotNumberInc,
			MinUtxoValue:             WeiToDfm(MinUTxODefaultValue).Uint64(),
			DefaultMinFeeForBridging: a.Config.PrimeConfig.DefaultMinBridgingFee,
			MinFeeForBridgingTokens:  a.Config.PrimeConfig.MinBridgingFeeForTokens,
			MinOperationFeeAmount:    a.Config.PrimeConfig.MinOperationFee,
			PotentialFee:             WeiToDfm(PotentialFee).Uint64(),
			TreasuryAddress:          a.Config.PrimeConfig.TreasuryAddress,
			Tokens:                   primeTokens,
		},
	}

	if a.Config.VectorConfig != nil && a.Config.VectorConfig.IsEnabled {
		vectorTokens := make(map[uint16]sendtx.ApexToken, len(a.VectorInfo.Tokens))
		for tID, t := range a.VectorInfo.Tokens {
			vectorTokens[tID] = sendtx.ApexToken{
				FullName:          t.ChainSpecific,
				IsWrappedCurrency: t.IsWrappedCurrency,
			}
		}

		txSenderChainConfigs[ChainIDVector] = sendtx.ChainConfig{
			CardanoCliBinary:         ResolveCardanoCliBinary(a.Config.VectorConfig.ChainType),
			TxProvider:               cardanowallet.NewTxProviderOgmios(a.VectorInfo.OgmiosURL),
			TestNetMagic:             a.Config.VectorConfig.NetworkMagic,
			TTLSlotNumberInc:         ttlSlotNumberInc,
			MinUtxoValue:             WeiToDfm(MinUTxODefaultValue).Uint64(),
			DefaultMinFeeForBridging: a.Config.VectorConfig.DefaultMinBridgingFee,
			MinFeeForBridgingTokens:  a.Config.VectorConfig.MinBridgingFeeForTokens,
			PotentialFee:             WeiToDfm(PotentialFee).Uint64(),
			TreasuryAddress:          a.Config.VectorConfig.TreasuryAddress,
			Tokens:                   vectorTokens,
		}
	}

	if a.Config.CardanoConfig != nil && a.Config.CardanoConfig.IsEnabled {
		cardanoTokens := make(map[uint16]sendtx.ApexToken, len(a.CardanoInfo.Tokens))
		for tID, t := range a.CardanoInfo.Tokens {
			cardanoTokens[tID] = sendtx.ApexToken{
				FullName:          t.ChainSpecific,
				IsWrappedCurrency: t.IsWrappedCurrency,
			}
		}

		txSenderChainConfigs[ChainIDCardano] = sendtx.ChainConfig{
			CardanoCliBinary:         ResolveCardanoCliBinary(a.Config.CardanoConfig.ChainType),
			TxProvider:               cardanowallet.NewTxProviderOgmios(a.CardanoInfo.OgmiosURL),
			TestNetMagic:             a.Config.CardanoConfig.NetworkMagic,
			TTLSlotNumberInc:         ttlSlotNumberInc,
			MinUtxoValue:             WeiToDfm(MinUTxODefaultValue).Uint64(),
			DefaultMinFeeForBridging: a.Config.CardanoConfig.DefaultMinBridgingFee,
			MinFeeForBridgingTokens:  a.Config.CardanoConfig.MinBridgingFeeForTokens,
			MinOperationFeeAmount:    a.Config.CardanoConfig.MinOperationFee,
			Tokens:                   cardanoTokens,
			TreasuryAddress:          a.Config.CardanoConfig.TreasuryAddress,
			PotentialFee:             WeiToDfm(PotentialFee).Uint64(),
		}
	}

	if a.Config.NexusConfig != nil && a.Config.NexusConfig.IsEnabled {
		txSenderChainConfigs[ChainIDNexus] = sendtx.ChainConfig{
			DefaultMinFeeForBridging: WeiToDfm(a.Config.NexusConfig.MinBridgingFee).Uint64(),
		}
	}

	if a.Config.PolygonConfig != nil && a.Config.PolygonConfig.IsEnabled {
		txSenderChainConfigs[ChainIDPolygon] = sendtx.ChainConfig{
			DefaultMinFeeForBridging: WeiToDfm(a.Config.PolygonConfig.MinBridgingFee).Uint64(),
		}
	}

	if a.Config.SolanaConfig != nil && a.Config.SolanaConfig.IsEnabled {
		txSenderChainConfigs[ChainIDSolana] = sendtx.ChainConfig{
			DefaultMinFeeForBridging: WeiToDfm(a.Config.SolanaConfig.MinBridgingFee).Uint64(),
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

func (a *ApexSystem) FundChainHotWallet(ctx context.Context, chainID string, weiAmount *big.Int) error {
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
			Amount: weiAmount,
		},
	}

	_, err = chain.SendTx(ctx, pk, nil, receivers, 0)

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
		err := a.execForEachChain(func(chain ITestApexChain) error {
			err := chain.DeployMintingContract(ctx, a.GetChainIDsConfig())
			if err != nil {
				return err
			}

			mintableTokens := chain.GetMintableTokens()

			if len(mintableTokens) > 0 {
				switch chain.ChainID() {
				case ChainIDCardano, ChainIDPrime, ChainIDVector:
					chainInfo := a.GetCardanoInfo(chain.ChainID())
					for tokenID, tokenName := range mintableTokens {
						if token, ok := chainInfo.Tokens[tokenID]; ok {
							token.ChainSpecific = tokenName
							chainInfo.Tokens[tokenID] = token
						}
					}
				case ChainIDNexus, ChainIDPolygon:
					chainInfo := a.GetEvmInfo(chain.ChainID())
					for tokenID, tokenName := range mintableTokens {
						if token, ok := chainInfo.Tokens[tokenID]; ok {
							token.ChainSpecific = tokenName
							chainInfo.Tokens[tokenID] = token
						}
					}
				case ChainIDSolana:
					chainInfo := a.SolanaInfo

					for tokenID, tokenName := range mintableTokens {
						if tokenID == SOLTokenID {
							continue
						}

						if token, ok := chainInfo.Tokens[tokenID]; ok {
							token.ChainSpecific = tokenName
							chainInfo.Tokens[tokenID] = token
						}
					}
				default:
					return fmt.Errorf("unimplemented cardano contract setup for chain %s", chain.ChainID())
				}
			}

			return nil
		})
		if err != nil {
			return err
		}

		a.InitTxSendChainConfiguration()

		return nil
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

func (a *ApexSystem) GenerateChainIDsConfig() error {
	chainIDsConfigFile, err := a.loadChainIDsConfigFile()
	if err != nil {
		return err
	}

	err = a.execForEachValidator(func(i int, validator *TestApexValidator) error {
		getHandler := func(callback CustomConfigHandler) func(data map[string]any) {
			return func(data map[string]any) {
				callback(a, data)
			}
		}

		if err := validator.GenerateChainIDsConfig(chainIDsConfigFile); err != nil {
			return fmt.Errorf("chain ID config generation failed for validator = %d: %w", i, err)
		}

		if handler := a.Config.CustomChainIDsConfigHandler; handler != nil {
			fileName := validator.GetChainIDsConfig()
			if err := UpdateJSONFile(fileName, fileName, getHandler(handler), false); err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return err
	}

	return nil
}

func (a *ApexSystem) loadChainIDsConfigFile() (*ChainIDsConfigFile, error) {
	chainIDsConfigFilePath := a.GetChainIDsDefaultConfigPath()

	chainIDsConfigFile, err := LoadJSON[ChainIDsConfigFile](chainIDsConfigFilePath)
	if err != nil {
		return nil, fmt.Errorf("error while reading ChainIDConfig JSON: %w", err)
	}

	var chainConfigs = make([]ChainIDConfig, 0, len(chainIDsConfigFile.ChainIDConfig))

	for _, chainIDConfig := range chainIDsConfigFile.ChainIDConfig {
		if ((a.Config.CardanoConfig != nil && a.Config.CardanoConfig.IsEnabled) && chainIDConfig.ChainID == ChainIDCardano) ||
			((a.Config.NexusConfig != nil && a.Config.NexusConfig.IsEnabled) && chainIDConfig.ChainID == ChainIDNexus) ||
			((a.Config.PolygonConfig != nil && a.Config.PolygonConfig.IsEnabled) && chainIDConfig.ChainID == ChainIDPolygon) ||
			((a.Config.SolanaConfig != nil && a.Config.SolanaConfig.IsEnabled) && chainIDConfig.ChainID == ChainIDSolana) ||
			(chainIDConfig.ChainID == ChainIDPrime) || (chainIDConfig.ChainID == ChainIDVector) {
			chainConfigs = append(chainConfigs, chainIDConfig)
		}
	}

	return &ChainIDsConfigFile{
		ChainIDConfig: chainConfigs,
	}, nil
}

func (a *ApexSystem) generateDirectionsConfigFile() *DirectionConfigFile {
	ecosystemTokens := make([]EcosystemToken, 0, len(a.EcosystemTokens))
	for id, name := range a.EcosystemTokens {
		ecosystemTokens = append(ecosystemTokens, EcosystemToken{ID: id, Name: name})
	}

	directionConfigFile := DirectionConfigFile{
		Directions: map[string]DirectionConfig{
			ChainIDPrime: {
				DestinationChain:                      a.PrimeInfo.DestChain,
				Tokens:                                a.PrimeInfo.Tokens,
				AlwaysTrackCurrencyAndWrappedCurrency: true,
			},
			ChainIDVector: {
				DestinationChain:                      a.VectorInfo.DestChain,
				Tokens:                                a.VectorInfo.Tokens,
				AlwaysTrackCurrencyAndWrappedCurrency: true,
			},
		},
		EcosystemTokens: ecosystemTokens,
	}

	if a.Config.CardanoConfig != nil && a.Config.CardanoConfig.IsEnabled {
		directionConfigFile.Directions[ChainIDCardano] = DirectionConfig{
			DestinationChain:                      a.CardanoInfo.DestChain,
			Tokens:                                a.CardanoInfo.Tokens,
			AlwaysTrackCurrencyAndWrappedCurrency: true,
		}
	}

	if a.Config.NexusConfig != nil && a.Config.NexusConfig.IsEnabled {
		directionConfigFile.Directions[ChainIDNexus] = DirectionConfig{
			DestinationChain:                      a.NexusInfo.DestChain,
			Tokens:                                a.NexusInfo.Tokens,
			AlwaysTrackCurrencyAndWrappedCurrency: false,
		}
	}

	if a.Config.PolygonConfig != nil && a.Config.PolygonConfig.IsEnabled {
		directionConfigFile.Directions[ChainIDPolygon] = DirectionConfig{
			DestinationChain:                      a.PolygonInfo.DestChain,
			Tokens:                                a.PolygonInfo.Tokens,
			AlwaysTrackCurrencyAndWrappedCurrency: false,
		}
	}

	if a.Config.SolanaConfig != nil && a.Config.SolanaConfig.IsEnabled {
		directionConfigFile.Directions[ChainIDSolana] = DirectionConfig{
			DestinationChain:                      a.SolanaInfo.DestChain,
			Tokens:                                a.SolanaInfo.Tokens,
			AlwaysTrackCurrencyAndWrappedCurrency: false, // true
		}
	}

	return &directionConfigFile
}

func (a *ApexSystem) generateCommonValidatorConfigs(
	validator *TestApexValidator, serverIndx int, directionConfigFile *DirectionConfigFile,
) error {
	getHandler := func(callback CustomConfigHandler) func(data map[string]any) {
		return func(data map[string]any) {
			callback(a, data)
		}
	}

	err := validator.GenerateDirectionsConfig(directionConfigFile)
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
}

func (a *ApexSystem) generateReactorConfigs() error {
	directionConfigFile := a.generateDirectionsConfigFile()

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

		return a.generateCommonValidatorConfigs(validator, serverIndx, directionConfigFile)
	})
	if err != nil {
		return err
	}

	return a.setBridgingAPIs()
}

func (a *ApexSystem) generateSkylineConfigs() error {
	directionConfigFile := a.generateDirectionsConfigFile()

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

		return a.generateCommonValidatorConfigs(validator, serverIndx, directionConfigFile)
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
			"--chain-ids-config", validator.GetChainIDsConfig(),
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

	return balance, err
}

func (a *ApexSystem) GetTreasuryAddressBalance(ctx context.Context, t *testing.T, chainID ChainID) (*big.Int, error) {
	t.Helper()

	var (
		balance map[string]*big.Int
		err     error
	)

	if !a.IsSkyline {
		return nil, nil
	}

	chain := a.GetChainMust(t, chainID)

	treasuryAddress := chain.GetTreasuryAddress()

	if treasuryAddress == "" {
		return nil, nil
	}

	balance, err = chain.GetAddressBalance(ctx, treasuryAddress)
	if err != nil {
		return nil, err
	}

	if balance[cardanowallet.AdaTokenName] == nil {
		return big.NewInt(0), nil
	}

	return balance[cardanowallet.AdaTokenName], nil
}

func (a *ApexSystem) ValidateTreasuryAddressBalance(
	ctx context.Context, t *testing.T, chainID ChainID, previousBalance *big.Int, numberOfBridgingRequests uint64,
) error {
	t.Helper()

	opFee := a.GetMinOperationFee(chainID)
	if chainID == ChainIDSolana {
		opFee = LamportToWei(opFee)
	}

	expectedBalance := new(big.Int).Add(previousBalance,
		new(big.Int).Mul(new(big.Int).SetUint64(numberOfBridgingRequests),
			opFee))

	timeout := time.NewTimer(1 * time.Minute)
	defer timeout.Stop()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		treasuryBalance, err := a.GetTreasuryAddressBalance(ctx, t, chainID)
		if err != nil {
			return err
		}

		if treasuryBalance.Cmp(expectedBalance) == 0 {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout.C:
			return fmt.Errorf("treasury address balance mismatch: expected %s, but received %s",
				expectedBalance, treasuryBalance)
		case <-ticker.C:
		}
	}
}

func (a *ApexSystem) GetBalanceWithTokenName(
	ctx context.Context, user *TestApexUser, chainID ChainID, tokenName string) (map[string]*big.Int, error) {
	chain, err := a.getChain(chainID)
	if err != nil {
		return nil, err
	}

	balance, err := chain.GetAddressBalanceWithTokenName(ctx, user.GetAddress(chainID), tokenName)
	if err != nil {
		return nil, err
	}

	return balance, err
}

func (a *ApexSystem) WaitForGreaterAmount(
	ctx context.Context, user *TestApexUser, chain ChainID,
	expectedAmount *big.Int, numRetries int, waitTime time.Duration, tokenName string,
) error {
	var (
		lastAmount *big.Int
		err        error
	)

	lastAmount, err = a.WaitForAmount(ctx, user, chain, func(val *big.Int) bool {
		return val.Cmp(expectedAmount) == 1
	}, numRetries, waitTime, tokenName)

	if err != nil {
		return fmt.Errorf("amount mismatch: expected greater than %s, but received %s: %w",
			expectedAmount, lastAmount, err)
	}

	return nil
}

func (a *ApexSystem) WaitForAmountInRange(
	ctx context.Context, user *TestApexUser, chain ChainID,
	lowerBoundaryDfm *big.Int, higherBoundaryDfm *big.Int, numRetries int, retryDelay time.Duration, tokenName string,
) error {
	lastAmount, err := a.WaitForAmount(ctx, user, chain, func(val *big.Int) bool {
		return val.Cmp(lowerBoundaryDfm) == 1 && val.Cmp(higherBoundaryDfm) != 1
	}, numRetries, retryDelay, tokenName)
	if err != nil {
		return fmt.Errorf("amount mismatch: expected amount between %s and %s, but received %s: %w",
			lowerBoundaryDfm, higherBoundaryDfm, lastAmount, err)
	}

	return nil
}

func (a *ApexSystem) WaitForExactAmount(
	ctx context.Context, user *TestApexUser, chain ChainID,
	expectedAmount *big.Int, numRetries int, waitTime time.Duration, tokenName string,
) error {
	var (
		lastAmount *big.Int
		err        error
	)

	lastAmount, err = a.WaitForAmount(ctx, user, chain, func(val *big.Int) bool {
		return val.Cmp(expectedAmount) >= 0
	}, numRetries, waitTime, tokenName)

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
	ctx context.Context, user *TestApexUser, chain ChainID,
	cmpHandler func(*big.Int) bool, numRetries int, retryDelay time.Duration, tokenName string,
) (*big.Int, error) {
	return infracommon.ExecuteWithRetry(ctx, func(ctx context.Context) (*big.Int, error) {
		var (
			amounts map[string]*big.Int
			err     error
		)

		amounts, err = a.GetBalanceWithTokenName(ctx, user, chain, tokenName)
		if err != nil {
			return nil, err
		}

		// fmt.Printf("Amounts: %+v, tokenName: %+v\n", amounts, tokenName)
		newBalance := amounts[tokenName]
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
		"--chain-ids-config", a.GetChainIDsConfig(),
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

func (a *ApexSystem) UpdateChainMaxNumberOfTransactions(
	chain ChainID,
	maxNumberOfTransactions int,
) error {
	pkBytes, err := a.GetBridgeAdmin().MarshallPrivateKey()
	if err != nil {
		return err
	}

	pk := hex.EncodeToString(pkBytes)

	return RunCommand(ResolveApexBridgeBinary(), []string{
		"bridge-admin", "update-chain-max-number-of-transactions",
		"--chain", chain,
		"--max-number-of-transactions", fmt.Sprint(maxNumberOfTransactions),
		"--bridge-url", a.GetBridgeDefaultJSONRPCAddr(),
		"--key", pk,
		"--chain-ids-config", a.GetChainIDsConfig(),
	}, os.Stdout)
}

func (a *ApexSystem) DefundHotWallet(
	chain ChainID, defundReceiverAddress string, defundAmount *big.Int, defundNativeTokenAmount *big.Int,
) error {
	pkBytes, err := a.GetBridgeAdmin().MarshallPrivateKey()
	if err != nil {
		return err
	}

	pk := hex.EncodeToString(pkBytes)

	return RunCommand(ResolveApexBridgeBinary(), []string{
		"bridge-admin", "defund",
		"--chain-ids-config", a.GetChainIDsConfig(),
		"--bridge-url", a.GetBridgeDefaultJSONRPCAddr(),
		"--chain", chain,
		"--amount", defundAmount.String(),
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
		"--chain-ids-config", a.GetChainIDsConfig(),
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
		"--chain-ids-config", a.GetChainIDsConfig(),
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
		"--chain-ids-config", a.GetChainIDsConfig(),
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
		"--chain-ids-config", a.GetChainIDsConfig(),
		"--bridge-url", a.GetBridgeDefaultJSONRPCAddr(),
		"--chain", chain.ChainID(),
		"--key", pk,
	}, os.Stdout)
}

func IsRetryableSubmitTx(err error) bool {
	// receipt polling failures must not trigger a full resubmit (tx may already be on-chain)
	if errors.Is(err, txrelayer.ErrFailedToRetrieveTxReceipt) {
		return false
	}

	return infracommon.IsRetryableError(err)
}

func (a *ApexSystem) SubmitTx(
	ctx context.Context, sourceChain ChainID, sender *TestApexUser,
	receiverAddr string, amount *big.Int, nativeTokens []GenericTokenAmount, data []byte, opFee *big.Int,
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

	var operationFee uint64

	if opFee == nil {
		operationFee = 0
	} else {
		operationFee = WeiToDfm(opFee).Uint64()
	}

	receivers := []GenericTxReceiver{
		{
			Addr:         receiverAddr,
			Amount:       amount,
			NativeTokens: nativeTokens,
		},
	}

	txHash, err := infracommon.ExecuteWithRetry(ctx, func(ctx context.Context) (string, error) {
		txHash, err := chain.SendTx(ctx, privateKey, data, receivers, operationFee)
		if err != nil {
			if strings.Contains(err.Error(), "The transaction contains unknown UTxO references as inputs") {
				return "", infracommon.ErrRetryTryAgain
			}

			return "", err
		}

		return txHash, nil
	}, infracommon.WithRetryCount(numRetries), infracommon.WithRetryWaitTime(waitTime),
		infracommon.WithIsRetryableError(IsRetryableSubmitTx))

	return txHash, err
}

type SubmitBridgingRequestData struct {
	Context          context.Context
	SourceChain      ChainID
	DestinationChain ChainID
	Sender           *TestApexUser
	WeiAmount        *big.Int
	SrcTokenID       uint16
	Receivers        []*TestApexUser
	TokensInfo       *BridgingTokensInfo
}

func (a *ApexSystem) SubmitBridgingRequest(
	data SubmitBridgingRequestData,
) (string, error) {
	if data.TokensInfo == nil {
		var err error

		data.TokensInfo, err = a.GetBridgingTokensInfo(data.SourceChain, data.DestinationChain, data.SrcTokenID)
		if err != nil {
			return "", err
		}
	}

	const (
		numRetries = 10
		waitTime   = time.Second * 10

		numReceiversMin = 1
		numReceiversMax = 5
	)

	if data.SourceChain == data.DestinationChain {
		return "", fmt.Errorf("source and destination chains are equal")
	}

	isSourceChainSupported := data.SourceChain == ChainIDPrime ||
		data.SourceChain == ChainIDVector ||
		data.SourceChain == ChainIDNexus ||
		data.SourceChain == ChainIDPolygon ||
		data.SourceChain == ChainIDCardano ||
		data.SourceChain == ChainIDSolana

	if !isSourceChainSupported {
		return "", fmt.Errorf("source chain is not supported")
	}

	isDestinationChainSupported := data.DestinationChain == ChainIDPrime ||
		data.DestinationChain == ChainIDVector ||
		data.DestinationChain == ChainIDNexus ||
		data.DestinationChain == ChainIDPolygon ||
		data.DestinationChain == ChainIDCardano ||
		data.DestinationChain == ChainIDSolana

	if !isDestinationChainSupported {
		return "", fmt.Errorf("destination chain is not supported")
	}

	// check if chains are configured and enabled
	if (a.Config.VectorConfig == nil || !a.Config.VectorConfig.IsEnabled) &&
		(data.SourceChain == ChainIDVector || data.DestinationChain == ChainIDVector) {
		return "", fmt.Errorf("vector is not configured or enabled, but it is specified as source or destination")
	}

	if (a.Config.CardanoConfig == nil || !a.Config.CardanoConfig.IsEnabled) &&
		(data.SourceChain == ChainIDCardano || data.DestinationChain == ChainIDCardano) {
		return "", fmt.Errorf("cardano is not configured or enabled, but it is specified as source or destination")
	}

	if (a.Config.NexusConfig == nil || !a.Config.NexusConfig.IsEnabled) &&
		(data.SourceChain == ChainIDNexus || data.DestinationChain == ChainIDNexus) {
		return "", fmt.Errorf("nexus is not configured or enabled, but it is specified as source or destination")
	}

	if (a.Config.PolygonConfig == nil || !a.Config.PolygonConfig.IsEnabled) &&
		(data.SourceChain == ChainIDPolygon || data.DestinationChain == ChainIDPolygon) {
		return "", fmt.Errorf("polygon is not configured or enabled, but it is specified as source or destination")
	}

	// check if bridging direction is supported
	isSourceChainCardanoType := data.SourceChain == ChainIDCardano ||
		data.SourceChain == ChainIDPrime || data.SourceChain == ChainIDVector
	isSourceChainEvmType := data.SourceChain == ChainIDNexus || data.SourceChain == ChainIDPolygon
	isSourceChainSolanaType := data.SourceChain == ChainIDSolana

	//nolint:gocritic
	if isSourceChainCardanoType {
		srcChainInfo := a.GetCardanoInfo(data.SourceChain)

		_, ok := srcChainInfo.DestChain[data.DestinationChain]
		if !ok {
			return "", fmt.Errorf("invalid bridging direction")
		}
	} else if isSourceChainEvmType {
		srcChainInfo := a.GetEvmInfo(data.SourceChain)

		_, ok := srcChainInfo.DestChain[data.DestinationChain]
		if !ok {
			return "", fmt.Errorf("invalid bridging direction")
		}
	} else if isSourceChainSolanaType {
		_, ok := a.SolanaInfo.DestChain[data.DestinationChain]
		if !ok {
			return "", fmt.Errorf("invalid bridging direction")
		}
	} else {
		return "", fmt.Errorf("invalid source chain")
	}

	if len(data.Receivers) < numReceiversMin ||
		len(data.Receivers) > numReceiversMax {
		return "", fmt.Errorf("invalid number of receivers")
	}

	receiversMap := make(map[string]ReceiverAmount, len(data.Receivers))

	// check if receivers are valid for the bridging - do they have necessary wallets
	for i, receiver := range data.Receivers {
		if data.DestinationChain == ChainIDVector && !receiver.HasVectorWallet {
			return "", fmt.Errorf("receiver %d does not have a vector wallet for vector chain transfer", i)
		}

		if data.DestinationChain == ChainIDNexus && !receiver.HasNexusWallet {
			return "", fmt.Errorf("receiver %d does not have a nexus wallet for nexus chain transfer", i)
		}

		if data.DestinationChain == ChainIDPolygon && !receiver.HasPolygonWallet {
			return "", fmt.Errorf("receiver %d does not have a polygon wallet for polygon chain transfer", i)
		}

		if data.DestinationChain == ChainIDCardano && !receiver.HasCardanoWallet {
			return "", fmt.Errorf("receiver %d does not have a cardano wallet for cardano chain transfer", i)
		}

		receiversMap[receiver.GetAddress(data.DestinationChain)] = ReceiverAmount{
			TokenID: data.TokensInfo.SrcTokenID,
			Amount:  data.WeiAmount,
		}
	}

	// check if users are valid for the bridging - do they have necessary wallets
	if data.SourceChain == ChainIDVector && !data.Sender.HasVectorWallet {
		return "", fmt.Errorf("sender does not have a vector wallet for vector chain transfer")
	}

	if data.SourceChain == ChainIDNexus && !data.Sender.HasNexusWallet {
		return "", fmt.Errorf("sender does not have a nexus wallet for nexus chain transfer")
	}

	if data.SourceChain == ChainIDPolygon && !data.Sender.HasPolygonWallet {
		return "", fmt.Errorf("sender does not have a polygon wallet for polygon chain transfer")
	}

	if data.SourceChain == ChainIDCardano && !data.Sender.HasCardanoWallet {
		return "", fmt.Errorf("sender does not have a cardano wallet for cardano chain transfer")
	}

	privateKey, err := data.Sender.GetPrivateKey(data.SourceChain)
	if err != nil {
		return "", fmt.Errorf("error while retrieving the private key: %w", err)
	}

	srcChain, err := a.getChain(data.SourceChain)
	if err != nil {
		return "", err
	}

	operationFee := big.NewInt(0)
	if a.IsSkyline {
		operationFee = a.GetMinOperationFee(data.SourceChain)
	}

	srcCurrencyID, err := a.GetChainCurrencyID(data.SourceChain)
	if err != nil {
		return "", err
	}

	destCurrencyID, err := a.GetChainCurrencyID(data.DestinationChain)
	if err != nil {
		return "", err
	}

	isCurrencySrc := srcCurrencyID == data.SrcTokenID

	feeAmount := a.GetMinBridgingFee(data.SourceChain, !isCurrencySrc)

	txHash, err := infracommon.ExecuteWithRetry(data.Context, func(ctx context.Context) (string, error) {
		txHash, err := srcChain.BridgingRequest(BridgingRequestParams{
			Ctx:            ctx,
			DestChainID:    data.DestinationChain,
			PrivateKey:     privateKey,
			ChainIDsConfig: a.GetChainIDsConfig(),
			Receivers:      receiversMap,
			FeeAmount:      feeAmount,
			OperationFee:   operationFee,
			IsCurrencySrc:  isCurrencySrc,
			IsCurrencyDest: destCurrencyID == data.TokensInfo.DstTokenID,
		})
		if err != nil {
			if strings.Contains(err.Error(), "The transaction contains unknown UTxO references as inputs") {
				return "", infracommon.ErrRetryTryAgain
			}

			return "", err
		}

		return txHash, nil
	}, infracommon.WithRetryCount(numRetries), infracommon.WithRetryWaitTime(waitTime),
		infracommon.WithIsRetryableError(IsRetryableSubmitTx))
	if err != nil {
		return "", fmt.Errorf("error while submitting bridging request: %w", err)
	}

	return txHash, nil
}

type BridgingTokensInfo struct {
	SrcTokenID   uint16
	DstTokenID   uint16
	SrcTokenName string
	DstTokenName string
}

func (a *ApexSystem) GetChainDirectionsAndTokens(chain ChainID) (map[ChainID][]Direction, map[uint16]Token) {
	switch chain {
	case ChainIDNexus, ChainIDPolygon:
		info := a.GetEvmInfo(chain)

		return info.DestChain, info.Tokens
	case ChainIDCardano, ChainIDPrime, ChainIDVector:
		info := a.GetCardanoInfo(chain)

		return info.DestChain, info.Tokens
	case ChainIDSolana:
		return a.SolanaInfo.DestChain, a.SolanaInfo.Tokens
	default:
		return nil, nil
	}
}

func (a *ApexSystem) GetChainCurrencyID(chain ChainID) (uint16, error) {
	_, tokens := a.GetChainDirectionsAndTokens(chain)
	if tokens == nil {
		return 0, fmt.Errorf("tokens not defined for chain %s", chain)
	}

	for tokID, tokInfo := range tokens {
		if tokInfo.ChainSpecific == cardanowallet.AdaTokenName {
			return tokID, nil
		}
	}

	return 0, fmt.Errorf("currency token not defined for chain %s", chain)
}

func (a *ApexSystem) GetChainWrappedCurrencyID(chain ChainID) (uint16, error) {
	_, tokens := a.GetChainDirectionsAndTokens(chain)
	if tokens == nil {
		return 0, fmt.Errorf("tokens not defined for chain %s", chain)
	}

	for tokID, tokInfo := range tokens {
		if tokInfo.IsWrappedCurrency {
			return tokID, nil
		}
	}

	return 0, fmt.Errorf("wrapped currency token not defined for chain %s", chain)
}

func (a *ApexSystem) GetChainTokenInfo(chain ChainID, tokenID uint16) (Token, error) {
	_, tokens := a.GetChainDirectionsAndTokens(chain)
	if tokens == nil {
		return Token{}, fmt.Errorf("tokens not defined for chain %s", chain)
	}

	tokenInfo, ok := tokens[tokenID]
	if !ok {
		return Token{}, fmt.Errorf("token info for tokenID: %d not found", tokenID)
	}

	return tokenInfo, nil
}

func (a *ApexSystem) GetBridgingTokensInfo(
	srcChain, dstChain ChainID, srcTokenID uint16) (*BridgingTokensInfo, error) {
	srcDirs, srcTokens := a.GetChainDirectionsAndTokens(srcChain)
	_, dstTokens := a.GetChainDirectionsAndTokens(dstChain)

	if srcDirs == nil || srcTokens == nil || dstTokens == nil {
		return nil, fmt.Errorf(
			"srcDirs or srcTokens or dstTokens not defined for srcChain %s, dstChain %s", srcChain, dstChain)
	}

	srcTokenInfo, ok := srcTokens[srcTokenID]
	if !ok {
		return nil, fmt.Errorf("token info for srcTokenID: %d not found", srcTokenID)
	}

	tokenPairs := srcDirs[dstChain]
	for _, pair := range tokenPairs {
		if pair.SourceTokenID == srcTokenID {
			dstTokenInfo, ok := dstTokens[pair.DestinationTokenID]
			if !ok {
				return nil, fmt.Errorf("token info for DestinationTokenID: %d not found", pair.DestinationTokenID)
			}

			return &BridgingTokensInfo{
				SrcTokenID:   srcTokenID,
				SrcTokenName: srcTokenInfo.ChainSpecific,
				DstTokenID:   pair.DestinationTokenID,
				DstTokenName: dstTokenInfo.ChainSpecific,
			}, nil
		}
	}

	return nil, fmt.Errorf("bridging dir for (%s, %s, tokenID: %d) not found", srcChain, dstChain, srcTokenID)
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

func (a *ApexSystem) GetEvmInfo(chainID string) EVMChainInfo {
	switch chainID {
	case ChainIDNexus:
		return a.NexusInfo
	case ChainIDPolygon:
		return a.PolygonInfo
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

func (a *ApexSystem) GetMinBridgingFee(chainID ChainID, isNativeTokenBridging bool) *big.Int {
	switch chainID {
	case ChainIDNexus:
		return a.Config.NexusConfig.MinBridgingFee
	case ChainIDPolygon:
		return a.Config.PolygonConfig.MinBridgingFee
	case ChainIDSolana:
		return a.Config.SolanaConfig.MinBridgingFee
	default:
		config := a.getCardanoConfig(chainID)

		if isNativeTokenBridging {
			return DfmToWei(new(big.Int).SetUint64(config.MinBridgingFeeForTokens))
		}

		return DfmToWei(new(big.Int).SetUint64(config.DefaultMinBridgingFee))
	}
}

func (a *ApexSystem) GetMinOperationFee(chainID ChainID) *big.Int {
	switch chainID {
	case ChainIDNexus:
		return a.Config.NexusConfig.MinOperationFee
	case ChainIDPolygon:
		return a.Config.PolygonConfig.MinOperationFee
	case ChainIDSolana:
		return a.Config.SolanaConfig.MinOperationFee
	default:
		return DfmToWei(new(big.Int).SetUint64(a.getCardanoConfig(chainID).MinOperationFee))
	}
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

func (a *ApexSystem) GetChainIDsConfig() string {
	if len(a.validators) > 0 {
		return a.validators[0].GetChainIDsConfig()
	}

	return a.GetChainIDsDefaultConfigPath()
}

func (a *ApexSystem) GetChainIDsDefaultConfigPath() string {
	return filepath.Join(a.chainIDConfigPath, ChainIDsConfigFileName)
}
