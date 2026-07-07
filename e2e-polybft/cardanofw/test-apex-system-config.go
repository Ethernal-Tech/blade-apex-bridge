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
	ChainIDPrime    ChainID = "prime"
	ChainIDVector   ChainID = "vector"
	ChainIDNexus    ChainID = "nexus"
	ChainIDPolygon  ChainID = "polygon"
	ChainIDEthereum ChainID = "ethereum"
	ChainIDKatana   ChainID = "katana"
	ChainIDSei      ChainID = "sei"
	ChainIDArbitrum ChainID = "arbitrum"
	ChainIDScroll   ChainID = "scroll"
	ChainIDUnichain ChainID = "unichain"
	ChainIDSolana   ChainID = "solana"

	ChainIDCardano ChainID = "cardano"

	ChainTypeCardanoStr = "cardano"
	ChainTypeEVMStr     = "evm"

	RunRelayerOnValidatorID = 1

	NoTelemetry TelemetryConfig = iota
	PrometheusTelemetry
	PrometheusAndDataDogTelemetry
)

// Token IDs
// 6-13 token IDs are registered tokens used only on web
const (
	AP3XTokenID         uint16 = 1
	ADATokenID          uint16 = 2
	CAP3XTokenID        uint16 = 3
	XADATokenID         uint16 = 4
	USDTTokenID         uint16 = 5
	POLTokenID          uint16 = 14
	XPOLTokenID         uint16 = 15
	PAP3XTokenID        uint16 = 16
	CPOLTokenID         uint16 = 17
	KatanaETHTokenID    uint16 = 18
	CKatanaETHTokenID   uint16 = 19
	ETHTokenID          uint16 = 20
	CETHTokenID         uint16 = 21
	SEITokenID          uint16 = 22
	CSEITokenID         uint16 = 23
	ArbitrumETHTokenID  uint16 = 24
	CArbitrumETHTokenID uint16 = 25
	ScrollETHTokenID    uint16 = 26
	CScrollETHTokenID   uint16 = 27
	UnichainETHTokenID  uint16 = 28
	CUnichainETHTokenID uint16 = 29

	// not on testnet - only for local tests
	USDCTokenID  uint16 = 60017
	USDCxTokenID uint16 = 60018

	SOLTokenID   uint16 = 30
	WSOLTokenID  uint16 = 31
	ASOLTokenID  uint16 = 32
	SAP3XTokenID uint16 = 33
	VSTokenID    uint16 = 34
	NSTokenID    uint16 = 35
)

// Human readable token names
const (
	AP3XTokenName         = "AP3X"
	ADATokenName          = "ADA"
	CAP3XTokenName        = "cAP3X"
	XADATokenName         = "xADA"
	USDTTokenName         = "USDT"
	POLTokenName          = "POL"
	XPOLTokenName         = "xPOL"
	PAP3XTokenName        = "pAP3X"
	CPOLTokenName         = "cPOL"
	KatanaETHTokenName    = "KatanaETH"
	CKatanaETHTokenName   = "cKatanaETH"
	ETHTokenName          = "ETH"
	CETHTokenName         = "cETH"
	SEITokenName          = "SEI"
	CSEITokenName         = "cSEI"
	ArbitrumETHTokenName  = "ArbitrumETH"  //nolint:gosec
	CArbitrumETHTokenName = "cArbitrumETH" //nolint:gosec
	ScrollETHTokenName    = "ScrollETH"
	CScrollETHTokenName   = "cScrollETH"
	UnichainETHTokenName  = "UnichainETH"
	CUnichainETHTokenName = "cUnichainETH" //nolint:gosec

	USDCTokenName    = "USDC"
	USDCxTokenName   = "USDCx"
	SOLANATokenName  = "SOL"
	WSOLANATokenName = "wSOL"
	ASOLTokenName    = "xwSOL"
	SAP3XTokenName   = "sAP3X"
	VSTokenName      = "VS"
	NSTokenName      = "NS"
	WSOLMintAddress  = "So11111111111111111111111111111111111111112"
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

	NexusConfig    *TestEVMChainConfig
	PolygonConfig  *TestEVMChainConfig
	EthereumConfig *TestEVMChainConfig
	KatanaConfig   *TestEVMChainConfig
	SeiConfig      *TestEVMChainConfig
	ArbitrumConfig *TestEVMChainConfig
	ScrollConfig   *TestEVMChainConfig
	UnichainConfig *TestEVMChainConfig

	SolanaConfig *TestSolanaChainConfig

	CustomOracleConfigHandler     CustomConfigHandler
	CustomRelayerConfigHandler    CustomConfigHandler
	CustomDirectionsConfigHandler CustomConfigHandler
	CustomChainIDsConfigHandler   CustomConfigHandler

	UserCnt                  uint
	UpdateAddressCountChains []ChainID
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

func WithPolygonEnabled(enabled bool) ApexSystemOptions {
	return func(h *ApexSystemConfig) {
		h.PolygonConfig.IsEnabled = enabled
	}
}

func WithEthereumEnabled(enabled bool) ApexSystemOptions {
	return func(h *ApexSystemConfig) {
		h.EthereumConfig.IsEnabled = enabled
	}
}

func WithKatanaEnabled(enabled bool) ApexSystemOptions {
	return func(h *ApexSystemConfig) {
		h.KatanaConfig.IsEnabled = enabled
	}
}

func WithSeiEnabled(enabled bool) ApexSystemOptions {
	return func(h *ApexSystemConfig) {
		h.SeiConfig.IsEnabled = enabled
	}
}

func WithArbitrumEnabled(enabled bool) ApexSystemOptions {
	return func(h *ApexSystemConfig) {
		h.ArbitrumConfig.IsEnabled = enabled
	}
}

func WithScrollEnabled(enabled bool) ApexSystemOptions {
	return func(h *ApexSystemConfig) {
		h.ScrollConfig.IsEnabled = enabled
	}
}

func WithUnichainEnabled(enabled bool) ApexSystemOptions {
	return func(h *ApexSystemConfig) {
		h.UnichainConfig.IsEnabled = enabled
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

func WithPolygonConfig(config *TestEVMChainConfig) ApexSystemOptions {
	return func(h *ApexSystemConfig) {
		h.PolygonConfig = config
	}
}

func WithEthereumConfig(config *TestEVMChainConfig) ApexSystemOptions {
	return func(h *ApexSystemConfig) {
		h.EthereumConfig = config
	}
}

func WithKatanaConfig(config *TestEVMChainConfig) ApexSystemOptions {
	return func(h *ApexSystemConfig) {
		h.KatanaConfig = config
	}
}

func WithSeiConfig(config *TestEVMChainConfig) ApexSystemOptions {
	return func(h *ApexSystemConfig) {
		h.SeiConfig = config
	}
}

func WithArbitrumConfig(config *TestEVMChainConfig) ApexSystemOptions {
	return func(h *ApexSystemConfig) {
		h.ArbitrumConfig = config
	}
}

func WithScrollConfig(config *TestEVMChainConfig) ApexSystemOptions {
	return func(h *ApexSystemConfig) {
		h.ScrollConfig = config
	}
}

func WithUnichainConfig(config *TestEVMChainConfig) ApexSystemOptions {
	return func(h *ApexSystemConfig) {
		h.UnichainConfig = config
	}
}

func WithSolanaConfig(config *TestSolanaChainConfig) ApexSystemOptions {
	return func(h *ApexSystemConfig) {
		h.SolanaConfig = config
		h.BladeValidatorCount = 5
	}
}

func WithCustomConfigHandlers(
	callbackOracle, callbackRelayer, callbackDirections, callbackChainIDs CustomConfigHandler) ApexSystemOptions {
	return func(h *ApexSystemConfig) {
		h.CustomOracleConfigHandler = callbackOracle
		h.CustomRelayerConfigHandler = callbackRelayer
		h.CustomDirectionsConfigHandler = callbackDirections
		h.CustomChainIDsConfigHandler = callbackChainIDs
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

func getDefaultApexSystemConfig() *ApexSystemConfig {
	return &ApexSystemConfig{
		APIValidatorID: 1,
		APIPortStart:   40000,
		APIKey:         "test_api_key",

		BladeValidatorCount: 4,

		PrimeConfig:    NewPrimeChainConfig(),
		VectorConfig:   NewVectorChainConfig(),
		CardanoConfig:  NewCardanoChainConfig(false),
		NexusConfig:    NewNexusChainConfig(false),
		PolygonConfig:  NewPolygonChainConfig(false),
		EthereumConfig: NewEthereumChainConfig(false),
		KatanaConfig:   NewKatanaChainConfig(false),
		SeiConfig:      NewSeiChainConfig(false),
		ArbitrumConfig: NewArbitrumChainConfig(false),
		ScrollConfig:   NewScrollChainConfig(false),
		UnichainConfig: NewUnichainChainConfig(false),
		SolanaConfig:   NewSolanaChainConfig(false),

		UserCnt: 10,
	}
}

func getDefaultSkylineSystemConfig() *ApexSystemConfig {
	return &ApexSystemConfig{
		APIValidatorID: 1,
		APIPortStart:   40000,
		APIKey:         "test_api_key",

		BladeValidatorCount: 4,

		PrimeConfig:    NewPrimeChainConfig(),
		VectorConfig:   NewVectorChainConfig(),
		CardanoConfig:  NewCardanoChainConfig(true),
		NexusConfig:    NewNexusChainConfig(false),
		PolygonConfig:  NewPolygonChainConfig(false),
		EthereumConfig: NewEthereumChainConfig(false),
		KatanaConfig:   NewKatanaChainConfig(false),
		SeiConfig:      NewSeiChainConfig(false),
		ArbitrumConfig: NewArbitrumChainConfig(false),
		ScrollConfig:   NewScrollChainConfig(false),
		UnichainConfig: NewUnichainChainConfig(false),
		SolanaConfig:   NewSolanaChainConfig(false),

		UserCnt: 10,
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

	if asc.PolygonConfig.IsEnabled {
		count++
	}

	if asc.EthereumConfig.IsEnabled {
		count++
	}

	if asc.KatanaConfig.IsEnabled {
		count++
	}

	if asc.SeiConfig.IsEnabled {
		count++
	}

	if asc.ArbitrumConfig.IsEnabled {
		count++
	}

	if asc.ScrollConfig.IsEnabled {
		count++
	}

	if asc.UnichainConfig.IsEnabled {
		count++
	}

	if asc.SolanaConfig.IsEnabled {
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

	if len(asc.PolygonConfig.PreminesAddresses) == 0 {
		asc.PolygonConfig.PreminesAddresses = make([]types.Address, 0, len(users))
	}

	if len(asc.EthereumConfig.PreminesAddresses) == 0 {
		asc.EthereumConfig.PreminesAddresses = make([]types.Address, 0, len(users))
	}

	if len(asc.KatanaConfig.PreminesAddresses) == 0 {
		asc.KatanaConfig.PreminesAddresses = make([]types.Address, 0, len(users))
	}

	if len(asc.SeiConfig.PreminesAddresses) == 0 {
		asc.SeiConfig.PreminesAddresses = make([]types.Address, 0, len(users))
	}

	if len(asc.ArbitrumConfig.PreminesAddresses) == 0 {
		asc.ArbitrumConfig.PreminesAddresses = make([]types.Address, 0, len(users))
	}

	if len(asc.ScrollConfig.PreminesAddresses) == 0 {
		asc.ScrollConfig.PreminesAddresses = make([]types.Address, 0, len(users))
	}

	if len(asc.UnichainConfig.PreminesAddresses) == 0 {
		asc.UnichainConfig.PreminesAddresses = make([]types.Address, 0, len(users))
	}

	if len(asc.SolanaConfig.PreminesAddresses) == 0 {
		asc.SolanaConfig.PreminesAddresses = make([]string, 0, len(users))
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

		if user.HasPolygonWallet {
			asc.PolygonConfig.PreminesAddresses = append(asc.PolygonConfig.PreminesAddresses, user.PolygonAddress)
		}

		if user.HasEthereumWallet {
			asc.EthereumConfig.PreminesAddresses = append(asc.EthereumConfig.PreminesAddresses, user.EthereumAddress)
		}

		if user.HasKatanaWallet {
			asc.KatanaConfig.PreminesAddresses = append(asc.KatanaConfig.PreminesAddresses, user.KatanaAddress)
		}

		if user.HasSeiWallet {
			asc.SeiConfig.PreminesAddresses = append(asc.SeiConfig.PreminesAddresses, user.SeiAddress)
		}

		if user.HasArbitrumWallet {
			asc.ArbitrumConfig.PreminesAddresses = append(asc.ArbitrumConfig.PreminesAddresses, user.ArbitrumAddress)
		}

		if user.HasScrollWallet {
			asc.ScrollConfig.PreminesAddresses = append(asc.ScrollConfig.PreminesAddresses, user.ScrollAddress)
		}

		if user.HasUnichainWallet {
			asc.UnichainConfig.PreminesAddresses = append(asc.UnichainConfig.PreminesAddresses, user.UnichainAddress)
		}

		if user.HasSolanaWallet {
			asc.SolanaConfig.PreminesAddresses = append(asc.SolanaConfig.PreminesAddresses, user.SolanaAddress)
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

func IsCardanoTypeChain(chainID ChainID) bool {
	return chainID == ChainIDCardano || chainID == ChainIDPrime || chainID == ChainIDVector
}
