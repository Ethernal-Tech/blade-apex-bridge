package cardanofw

import (
	"encoding/json"
	"io"
	"os"
)

type BridgeConfig struct {
	ValidatorDataDir        string           `json:"validatorDataDir"`
	ValidatorConfigPath     string           `json:"validatorConfigPath"`
	CardanoChains           CardanoChains    `json:"cardanoChains"`
	EthChains               EthChains        `json:"ethChains"`
	Bridge                  Bridge           `json:"bridge"`
	BridgingSettings        BridgingSettings `json:"bridgingSettings"`
	AppSettings             AppSettings      `json:"appSettings"`
	RelayerImitatorPullTime int              `json:"relayerImitatorPullTime"`
	BatcherPullTime         int              `json:"batcherPullTime"`
	BatcherTestMode         int              `json:"batcherTestMode"`
	API                     API              `json:"api"`
	Telemetry               Telemetry        `json:"telemetry"`
}

type CardanoChains struct {
	Prime  ChainConfig `json:"prime"`
	Vector ChainConfig `json:"vector"`
}

type ChainConfig struct {
	NetworkAddress           string   `json:"networkAddress"`
	NetworkID                int      `json:"networkID"`
	NetworkMagic             int      `json:"networkMagic"`
	StartBlockHash           string   `json:"startBlockHash"`
	StartSlot                int      `json:"startSlot"`
	InitialUtxos             *string  `json:"initialUtxos"`
	TTLSlotNumberIncrement   int      `json:"ttlSlotNumberIncrement"`
	ConfirmationBlockCount   int      `json:"confirmationBlockCount"`
	OtherAddressesOfInterest []string `json:"otherAddressesOfInterest"`
	OgmiosURL                string   `json:"ogmiosUrl"`
	BlockfrostURL            string   `json:"blockfrostUrl"`
	BlockfrostAPIKey         string   `json:"blockfrostApiKey"`
	SocketPath               string   `json:"socketPath"`
	PotentialFee             int      `json:"potentialFee"`
	SlotRoundingThreshold    int      `json:"slotRoundingThreshold"`
	NoBatchPeriodPercent     float64  `json:"noBatchPeriodPercent"`
	TakeAtLeastUtxoCount     int      `json:"takeAtLeastUtxoCount"`
}

type EthChains struct {
	Nexus EthChainConfig `json:"nexus"`
}

type EthChainConfig struct {
	ChainID                string            `json:"ChainID"`
	BridgingAddresses      BridgingAddresses `json:"bridgingAddresses"`
	NodeURL                string            `json:"nodeUrl"`
	SyncBatchSize          int               `json:"syncBatchSize"`
	NumBlockConfirmations  int               `json:"numBlockConfirmations"`
	StartBlockNumber       int               `json:"startBlockNumber"`
	PoolIntervalMs         int               `json:"poolIntervalMs"`
	TTLBlockNumberInc      int               `json:"ttlBlockNumberInc"`
	BlockRoundingThreshold int               `json:"blockRoundingThreshold"`
	NoBatchPeriodPercent   float64           `json:"noBatchPeriodPercent"`
	DynamicTx              bool              `json:"dynamicTx"`
}

type BridgingAddresses struct {
	Address    string `json:"address"`
	FeeAddress string `json:"feeAddress"`
}

type Bridge struct {
	NodeURL      string       `json:"nodeUrl"`
	DynamicTx    bool         `json:"dynamicTx"`
	ScAddress    string       `json:"scAddress"`
	SubmitConfig SubmitConfig `json:"submitConfig"`
}

type SubmitConfig struct {
	ConfirmedBlocksThreshold  int `json:"confirmedBlocksThreshold"`
	ConfirmedBlocksSubmitTime int `json:"confirmedBlocksSubmitTime"`
}

type BridgingSettings struct {
	MinFeeForBridging              int `json:"minFeeForBridging"`
	UtxoMinValue                   int `json:"utxoMinValue"`
	MaxReceiversPerBridgingRequest int `json:"maxReceiversPerBridgingRequest"`
	MaxBridgingClaimsToGroup       int `json:"maxBridgingClaimsToGroup"`
}

type AppSettings struct {
	Logger  Logger `json:"logger"`
	DbsPath string `json:"dbsPath"`
}

type Logger struct {
	LogLevel      int    `json:"logLevel"`
	JSONLogFormat bool   `json:"jsonLogFormat"`
	AppendFile    bool   `json:"appendFile"`
	LogFilePath   string `json:"logFilePath"`
	Name          string `json:"name"`
}

type API struct {
	Port           int      `json:"port"`
	PathPrefix     string   `json:"pathPrefix"`
	AllowedHeaders []string `json:"allowedHeaders"`
	AllowedOrigins []string `json:"allowedOrigins"`
	AllowedMethods []string `json:"allowedMethods"`
	APIKeyHeader   string   `json:"apiKeyHeader"`
	APIKeys        []string `json:"apiKeys"`
}

type Telemetry struct {
	PrometheusAddr string `json:"prometheusAddr"`
	DataDogAddr    string `json:"dataDogAddr"`
}

func ParseConfig(configFile string) (*BridgeConfig, error) {
	var config BridgeConfig

	jsonFile, err := os.Open(configFile)
	if err != nil {
		return nil, err
	}
	defer jsonFile.Close()

	byteValue, err := io.ReadAll(jsonFile)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(byteValue, &config)
	if err != nil {
		return nil, err
	}

	return &config, nil
}
