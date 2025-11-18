package cardanofw

import (
	"encoding/hex"
	"fmt"

	"github.com/0xPolygon/polygon-edge/types"
)

type ChainID = string
type TelemetryConfig = int
type CustomConfigHandler = func(apex *ApexSystem, mp map[string]interface{})

const (
	ChainIDPrime  ChainID = "prime"
	ChainIDVector ChainID = "vector"
	ChainIDNexus  ChainID = "nexus"

	ChainIDCardano ChainID = "cardano"

	RunRelayerOnValidatorID = 1

	NoTelemetry TelemetryConfig = iota
	PrometheusTelemetry
	PrometheusAndDataDogTelemetry
)

type ApexSystemConfig struct {
	APIValidatorID int // -1 all validators
	APIPortStart   int
	APIKey         string

	TelemetryConfig        TelemetryConfig
	TargetOneClusterServer bool

	BladeValidatorCount int

	PrimeConfig   *TestCardanoChainConfig
	VectorConfig  *TestCardanoChainConfig
	CardanoConfig *TestCardanoChainConfig
	NexusConfig   *TestEVMChainConfig

	CustomOracleConfigHandler  CustomConfigHandler
	CustomRelayerConfigHandler CustomConfigHandler

	UserCnt                  uint
	UpdateAddressCountChains []ChainID

	ColoredCoins []ColoredCoinConfig
}

type ColoredCoinConfig struct {
	ID                     uint16
	Name                   string
	EcosystemOriginChainID ChainID
	DestinationChainID     ChainID
}

func (c ColoredCoinConfig) String() string {
	return fmt.Sprintf("%d:%s:%s", c.ID, c.Name, c.EcosystemOriginChainID)
}

type ApexSystemOptions func(*ApexSystemConfig)

func WithAPIValidatorID(apiValidatorID int) ApexSystemOptions {
	return func(h *ApexSystemConfig) {
		h.APIValidatorID = apiValidatorID
	}
}

func WithAPIPortStart(apiPortStart int) ApexSystemOptions {
	return func(h *ApexSystemConfig) {
		h.APIPortStart = apiPortStart
	}
}

func WithAPIKey(apiKey string) ApexSystemOptions {
	return func(h *ApexSystemConfig) {
		h.APIKey = apiKey
	}
}

func WithVectorEnabled(enabled bool) ApexSystemOptions {
	return func(h *ApexSystemConfig) {
		h.VectorConfig.IsEnabled = enabled
	}
}

func WithCardanoEnabled(enabled bool) ApexSystemOptions {
	return func(h *ApexSystemConfig) {
		h.CardanoConfig.IsEnabled = enabled
	}
}

func WithNexusEnabled(enabled bool) ApexSystemOptions {
	return func(h *ApexSystemConfig) {
		h.NexusConfig.IsEnabled = enabled
	}
}

func WithTelemetryConfig(tc TelemetryConfig) ApexSystemOptions {
	return func(h *ApexSystemConfig) {
		h.TelemetryConfig = tc
	}
}

func WithTargetOneClusterServer(targetOneClusterServer bool) ApexSystemOptions {
	return func(h *ApexSystemConfig) {
		h.TargetOneClusterServer = targetOneClusterServer
	}
}

func WithPrimeConfig(config *TestCardanoChainConfig) ApexSystemOptions {
	return func(h *ApexSystemConfig) {
		h.PrimeConfig = config
	}
}

func WithVectorConfig(config *TestCardanoChainConfig) ApexSystemOptions {
	return func(h *ApexSystemConfig) {
		h.VectorConfig = config
	}
}

func WithCardanoConfig(config *TestCardanoChainConfig) ApexSystemOptions {
	return func(h *ApexSystemConfig) {
		h.CardanoConfig = config
	}
}

func WithNexusConfig(config *TestEVMChainConfig) ApexSystemOptions {
	return func(h *ApexSystemConfig) {
		h.NexusConfig = config
	}
}

func WithCustomConfigHandlers(callbackOracle, callbackRelayer CustomConfigHandler) ApexSystemOptions {
	return func(h *ApexSystemConfig) {
		h.CustomOracleConfigHandler = callbackOracle
		h.CustomRelayerConfigHandler = callbackRelayer
	}
}

func WithUserCnt(userCnt uint) ApexSystemOptions {
	return func(h *ApexSystemConfig) {
		h.UserCnt = userCnt
	}
}

func WithBridgingAddrCnt(chainID ChainID, addressCnt int) ApexSystemOptions {
	return func(h *ApexSystemConfig) {
		h.UpdateAddressCountChains = append(h.UpdateAddressCountChains, chainID)

		switch chainID {
		case ChainIDPrime:
			h.PrimeConfig.BridgingAddressCnt = addressCnt
		case ChainIDVector:
			h.VectorConfig.BridgingAddressCnt = addressCnt
		case ChainIDCardano:
			h.CardanoConfig.BridgingAddressCnt = addressCnt
		}
	}
}

func WithColoredCoins(coloredCoins []ColoredCoinConfig) ApexSystemOptions {
	return func(h *ApexSystemConfig) {
		h.ColoredCoins = coloredCoins
	}
}

func getDefaultApexSystemConfig() *ApexSystemConfig {
	return &ApexSystemConfig{
		APIValidatorID: 1,
		APIPortStart:   40000,
		APIKey:         "test_api_key",

		BladeValidatorCount: 4,

		PrimeConfig:   NewPrimeChainConfig(),
		VectorConfig:  NewVectorChainConfig(),
		CardanoConfig: NewCardanoChainConfig(false),
		NexusConfig:   NewNexusChainConfig(false),

		UserCnt: 10,
	}
}

func getDefaultSkylinexSystemConfig() *ApexSystemConfig {
	return &ApexSystemConfig{
		APIValidatorID: 1,
		APIPortStart:   40000,
		APIKey:         "test_api_key",

		BladeValidatorCount: 4,

		PrimeConfig:   NewPrimeChainConfig(),
		VectorConfig:  NewVectorChainConfig(),
		CardanoConfig: NewCardanoChainConfig(true),
		NexusConfig:   NewNexusChainConfig(false),

		UserCnt: 10,
	}
}

func initAllowedDirections(config *ApexSystemConfig, isSkyline bool) {
	if isSkyline {
		coloredCoinsMap := make(map[ChainID]map[ChainID][]uint16)
		chains := []ChainID{ChainIDCardano, ChainIDVector, ChainIDPrime, ChainIDNexus}

		for _, originChain := range chains {
			coloredCoinsMap[originChain] = make(map[ChainID][]uint16)
			for _, destinationChain := range chains {
				coloredCoinsMap[originChain][destinationChain] = make([]uint16, 0)
			}
		}

		for _, cc := range config.ColoredCoins {
			switch cc.EcosystemOriginChainID {
			case ChainIDCardano:
				coloredCoinsMap[cc.EcosystemOriginChainID][cc.DestinationChainID] =
					append(coloredCoinsMap[cc.EcosystemOriginChainID][cc.DestinationChainID], cc.ID)
			case ChainIDVector:
				if cc.ID != 2 {
					coloredCoinsMap[cc.EcosystemOriginChainID][cc.DestinationChainID] =
						append(coloredCoinsMap[cc.EcosystemOriginChainID][cc.DestinationChainID], cc.ID)
				}
			}

			switch cc.DestinationChainID {
			case ChainIDVector:
				coloredCoinsMap[cc.DestinationChainID][cc.EcosystemOriginChainID] =
					append(coloredCoinsMap[cc.DestinationChainID][cc.EcosystemOriginChainID], cc.ID)
			}
		}

		if len(config.CardanoConfig.AllowedDirections) == 0 {
			config.CardanoConfig.AllowedDirections = AllowedDirections{
				ChainIDCardano: {
					ChainIDPrime:  {false, true, coloredCoinsMap[ChainIDCardano][ChainIDPrime]},
					ChainIDVector: {true, false, coloredCoinsMap[ChainIDCardano][ChainIDVector]},
				},
			}
		}

		if len(config.VectorConfig.AllowedDirections) == 0 {
			config.VectorConfig.AllowedDirections = AllowedDirections{
				ChainIDVector: {
					ChainIDCardano: {false, true, coloredCoinsMap[ChainIDVector][ChainIDCardano]},
				},
			}
		}

		if len(config.PrimeConfig.AllowedDirections) == 0 {
			config.PrimeConfig.AllowedDirections = AllowedDirections{
				ChainIDPrime: {
					ChainIDCardano: {true, false, coloredCoinsMap[ChainIDPrime][ChainIDCardano]},
				},
			}
		}
	} else {
		//nolint
		// TODO: Fix this up for reactor
		if len(config.NexusConfig.AllowedDirections) == 0 {
			config.NexusConfig.AllowedDirections = []ChainID{ChainIDPrime, ChainIDVector}
		}

		if len(config.VectorConfig.AllowedDirections) == 0 {
			config.VectorConfig.AllowedDirections = AllowedDirections{
				ChainIDVector: {
					ChainIDPrime: {true, false, []uint16{}},
					ChainIDNexus: {true, false, []uint16{}},
				},
			}
		}

		if len(config.PrimeConfig.AllowedDirections) == 0 {
			config.PrimeConfig.AllowedDirections = AllowedDirections{
				ChainIDPrime: {
					ChainIDVector: {true, false, []uint16{}},
					ChainIDNexus:  {true, false, []uint16{}},
				},
			}
		}
	}
}

func (asc *ApexSystemConfig) ServiceCount() int {
	// Prime
	count := 1

	if asc.VectorConfig.IsEnabled {
		count++
	}

	if asc.CardanoConfig.IsEnabled {
		count++
	}

	if asc.NexusConfig.IsEnabled {
		count++
	}

	return count
}

func (asc *ApexSystemConfig) applyPremineFundingOptions(users []*TestApexUser) {
	if len(asc.PrimeConfig.PreminesAddresses) == 0 {
		asc.PrimeConfig.PreminesAddresses = make([]string, 0, len(users))
	}

	if len(asc.VectorConfig.PreminesAddresses) == 0 {
		asc.VectorConfig.PreminesAddresses = make([]string, 0, len(users))
	}

	if len(asc.CardanoConfig.PreminesAddresses) == 0 {
		asc.CardanoConfig.PreminesAddresses = make([]string, 0, len(users))
	}

	if len(asc.NexusConfig.PreminesAddresses) == 0 {
		asc.NexusConfig.PreminesAddresses = make([]types.Address, 0, len(users))
	}

	for _, user := range users {
		asc.PrimeConfig.PreminesAddresses = append(asc.PrimeConfig.PreminesAddresses,
			hex.EncodeToString(user.PrimeAddress.GetBytes()))

		if user.HasVectorWallet {
			asc.VectorConfig.PreminesAddresses = append(asc.VectorConfig.PreminesAddresses,
				hex.EncodeToString(user.VectorAddress.GetBytes()))
		}

		if user.HasCardanoWallet {
			asc.CardanoConfig.PreminesAddresses = append(asc.CardanoConfig.PreminesAddresses,
				hex.EncodeToString(user.CardanoAddress.GetBytes()))
		}

		if user.HasNexusWallet {
			asc.NexusConfig.PreminesAddresses = append(asc.NexusConfig.PreminesAddresses, user.NexusAddress)
		}
	}
}

func (asc *ApexSystemConfig) GetTelemetryForValidatorIdx(idx int) string {
	switch asc.TelemetryConfig {
	case PrometheusTelemetry:
		return fmt.Sprintf("0.0.0.0:%d", 5001+idx)
	case PrometheusAndDataDogTelemetry:
		return fmt.Sprintf("0.0.0.0:%d,localhost:%d", 5001+idx, 8126+idx)
	default:
		return ""
	}
}
