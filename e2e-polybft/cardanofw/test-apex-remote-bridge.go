package cardanofw

import (
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/Ethernal-Tech/cardano-infrastructure/sendtx"
	cardanowallet "github.com/Ethernal-Tech/cardano-infrastructure/wallet"
)

type RemoteApexBridgeConfig struct {
	PrimeInfo      CardanoChainInfo
	VectorInfo     CardanoChainInfo
	CardanoInfo    CardanoChainInfo
	NexusInfo      EVMChainInfo
	BridgingAPIs   []string
	BridgingAPIKey string
}

type ApexKeysData struct {
	Funder *ApexPrivateKeys   `json:"funder"`
	Users  []*ApexPrivateKeys `json:"users"`
}

type ApexUsersData struct {
	Funder *TestApexUser
	Users  []*TestApexUser
}

const TestnetEnvsPartner = "partner"

func GetTestnetApexBridgeConfig() *RemoteApexBridgeConfig {
	if os.Getenv("TESTNET_ENV") == TestnetEnvsPartner {
		return GetPartnerTestnetApexBridgeConfig()
	}

	return GetInternalTestnetApexBridgeConfig()
}

func GetInternalTestnetApexBridgeConfig() *RemoteApexBridgeConfig {
	return &RemoteApexBridgeConfig{
		PrimeInfo: CardanoChainInfo{
			NetworkAddress: "relay-0.prime.testnet.apexfusion.org:5521",
			OgmiosURL:      "http://ogmios.prime.testnet.apexfusion.org:1337",
			MultisigAddr:   "addr_test1wrz24vv4tvfqsywkxn36rv5zagys2d7euafcgt50gmpgqpq4ju9uv",
			FeeAddr:        "addr_test1wq5dw0g9mpmjy0xd6g58kncapdf6vgcka9el4llhzwy5vhqz80tcq",
		},
		VectorInfo: CardanoChainInfo{
			NetworkAddress: "relay-0.vector.testnet.apexfusion.org:7522",
			OgmiosURL:      "http://ogmios.vector.testnet.apexfusion.org:1337",
			MultisigAddr:   "vector_test1w2h482rf4gf44ek0rekamxksulazkr64yf2fhmm7f5gxjpsdm4zsg",
			FeeAddr:        "vector_test1wtyslvqxffyppmzhs7ecwunsnpq6g2p6kf9r4aa8ntfzc4qj925fr",
		},
		NexusInfo: EVMChainInfo{
			GatewayAddress: types.StringToAddress("0xc68221AD72397d85084f2D5C7089e4e9487c118c"),
			JSONRPCAddr:    "https://rpc.nexus.testnet.apexfusion.org",
		},
		BridgingAPIs: []string{
			"http://internal-bridge-api-testnet.apexfusion.org:10003",
		},
		BridgingAPIKey: os.Getenv("TESTNET_BRIDGING_API_KEY"),
	}
}

func GetPartnerTestnetApexBridgeConfig() *RemoteApexBridgeConfig {
	return &RemoteApexBridgeConfig{
		PrimeInfo: CardanoChainInfo{
			NetworkAddress: "relay-0.prime.testnet.apexfusion.org:5521",
			OgmiosURL:      "http://ogmios.prime.testnet.apexfusion.org:1337",
			MultisigAddr:   "addr_test1wzqpmaaz67lerqfrludgl64tc892wu9g3cph5ens90l2cvqx4fllt",
			FeeAddr:        "addr_test1wp2s7dk9nl552rwa9twkw3m8x52r4p750h3ma97l60hve0sq2m2ek",
		},
		VectorInfo: CardanoChainInfo{
			NetworkAddress: "relay-0.vector.testnet.apexfusion.org:7522",
			OgmiosURL:      "http://ogmios.vector.testnet.apexfusion.org:1337",
			MultisigAddr:   "vector_test1w2mfypss95pq75qjzn8cxjr3xyl6vuknl0nlfwl3sldmpscauccc7",
			FeeAddr:        "vector_test1wg6pevsxk6e63eys550u6rwju0wakxsgfw49qxlp8c96h8c0lx60g",
		},
		NexusInfo: EVMChainInfo{
			GatewayAddress: types.StringToAddress("0x5a51812ADfF2A27297EF08475161E5f7B66250bd"),
			JSONRPCAddr:    "https://rpc.nexus.testnet.apexfusion.org",
		},
		BridgingAPIs: []string{
			"http://bridge-api-testnet.apexfusion.org:10003",
		},
		BridgingAPIKey: os.Getenv("PARTNER_TESTNET_BRIDGING_API_KEY"),
	}
}

func GetTestnetSkylineBridgeConfig() *RemoteApexBridgeConfig {
	if os.Getenv("TESTNET_ENV") == TestnetEnvsPartner {
		return GetPartnerTestnetSkylineBridgeConfig()
	}

	return GetInternalTestnetSkylineBridgeConfig()
}

func GetInternalTestnetSkylineBridgeConfig() *RemoteApexBridgeConfig {
	return &RemoteApexBridgeConfig{
		PrimeInfo: CardanoChainInfo{
			NetworkAddress: "relay-0.prime.testnet.apexfusion.org:5521",
			OgmiosURL:      "http://ogmios.prime.testnet.apexfusion.org:1337",
			MultisigAddr:   "addr_test1wr943cc3l8hxhjcnh8mjrh67yshksw2ugydx0r9a4k7xrfqsp4p7y",
			FeeAddr:        "addr_test1wpp9wrzm5249ksfucal6f9rdth9y0rlwecu0r608gysjmcsf764mj",
			NativeTokens: []sendtx.TokenExchangeConfig{
				{
					DstChainID: ChainIDCardano,
					TokenName: cardanowallet.NewToken(
						"a59a8df821056ddcaeae4eb16f272565a0b3581c61e04a9bd18d4b32", "WADA").String(),
				},
			},
		},
		CardanoInfo: CardanoChainInfo{
			NetworkAddress: "http://preview-services-skyline.testnet.ethernal.work:5521",
			OgmiosURL:      "http://preview-services-skyline.testnet.ethernal.work:1733",
			MultisigAddr:   "addr_test1wrntyxdelrw98ps0vrpuf4rr2mmknem83y9ywxfn0a3jeasxtsjz5",
			FeeAddr:        "addr_test1wz3q8gnjsuyf7etplmcuw8rfusckmn5dmjwnhwdsap6savqcx8xg3",
			NativeTokens: []sendtx.TokenExchangeConfig{
				{
					DstChainID: ChainIDPrime,
					TokenName: cardanowallet.NewToken(
						"64c6ea243c3133d44f2022299e74b027f02b1c13397324819e8465c7", "WAPEX").String(),
				},
			},
		},
		BridgingAPIs: []string{
			"http://validators-oracle-api-skyline.testnet.ethernal.work",
		},
		BridgingAPIKey: "ae8cf1ec-2ab8-1337-9840-9b442e59aee1",
	}
}

func GetPartnerTestnetSkylineBridgeConfig() *RemoteApexBridgeConfig {
	return &RemoteApexBridgeConfig{
		PrimeInfo: CardanoChainInfo{
			NetworkAddress: "relay-0.prime.testnet.apexfusion.org:5521",
			OgmiosURL:      "http://ogmios.prime.testnet.apexfusion.org:1337",
			MultisigAddr:   "addr_test1wr943cc3l8hxhjcnh8mjrh67yshksw2ugydx0r9a4k7xrfqsp4p7y",
			FeeAddr:        "addr_test1wpp9wrzm5249ksfucal6f9rdth9y0rlwecu0r608gysjmcsf764mj",
			NativeTokens: []sendtx.TokenExchangeConfig{
				{
					DstChainID: ChainIDCardano,
					TokenName: cardanowallet.NewToken(
						"a59a8df821056ddcaeae4eb16f272565a0b3581c61e04a9bd18d4b32", "WADA").String(),
				},
			},
		},
		CardanoInfo: CardanoChainInfo{
			NetworkAddress: "http://preview-services-skyline.testnet.ethernal.work:5521",
			OgmiosURL:      "http://preview-services-skyline.testnet.ethernal.work:1733",
			MultisigAddr:   "addr_test1wrntyxdelrw98ps0vrpuf4rr2mmknem83y9ywxfn0a3jeasxtsjz5",
			FeeAddr:        "addr_test1wz3q8gnjsuyf7etplmcuw8rfusckmn5dmjwnhwdsap6savqcx8xg3",
			NativeTokens: []sendtx.TokenExchangeConfig{
				{
					DstChainID: ChainIDPrime,
					TokenName: cardanowallet.NewToken(
						"64c6ea243c3133d44f2022299e74b027f02b1c13397324819e8465c7", "WAPEX").String(),
				},
			},
		},
		BridgingAPIs: []string{
			"http://validators-oracle-api-skyline.testnet.ethernal.work",
		},
		BridgingAPIKey: "ae8cf1ec-2ab8-1337-9840-9b442e59aee1",
	}
}

func GetTestnetUserKeys() (*ApexKeysData, error) {
	content := os.Getenv("E2E_TESTNET_WALLET_KEYS_CONTENT")
	if len(content) > 0 {
		var pks ApexKeysData

		err := json.Unmarshal([]byte(content), &pks)
		if err != nil {
			return nil, err
		}

		return &pks, nil
	}

	path := os.Getenv("E2E_TESTNET_WALLET_KEYS_PATH")
	if len(path) > 0 {
		pks, err := LoadJSON[ApexKeysData](path)
		if err != nil {
			return nil, err
		}

		return pks, nil
	}

	return nil, errors.New("E2E_TESTNET_WALLET_KEYS_CONTENT nor E2E_TESTNET_WALLET_KEYS_PATH env variables defined")
}

func GetTestnetApexUsers(networks *ApexNetworkTypes) (*ApexUsersData, error) {
	userKeysData, err := GetTestnetUserKeys()
	if err != nil {
		return nil, err
	}

	funder, err := userKeysData.Funder.User(networks)
	if err != nil {
		return nil, err
	}

	users := make([]*TestApexUser, len(userKeysData.Users))

	for i, keys := range userKeysData.Users {
		user, err := keys.User(networks)
		if err != nil {
			return nil, err
		}

		users[i] = user
	}

	return &ApexUsersData{
		Funder: funder,
		Users:  users,
	}, nil
}

func SetupRemoteApexBridge(
	t *testing.T,
	remoteConfig *RemoteApexBridgeConfig,
	apexOpts ...ApexSystemOptions,
) (*ApexSystem, error) {
	t.Helper()

	apexConfig := &ApexSystemConfig{
		PrimeConfig:  NewRemotePrimeChainConfig(0, 0),
		VectorConfig: NewRemoteVectorChainConfig(true),
		NexusConfig:  NewRemoteNexusChainConfig(true),
		APIKey:       remoteConfig.BridgingAPIKey,
	}

	for _, opt := range apexOpts {
		opt(apexConfig)
	}

	primeChain := &TestCardanoChain{
		config:           apexConfig.PrimeConfig,
		multisigAddr:     remoteConfig.PrimeInfo.MultisigAddr,
		multisigFeeAddr:  remoteConfig.PrimeInfo.FeeAddr,
		ogmiosURL:        remoteConfig.PrimeInfo.OgmiosURL,
		blockfrostURL:    remoteConfig.PrimeInfo.BlockfrostURL,
		blockfrostAPIKey: remoteConfig.PrimeInfo.BlockfrostAPIKey,
	}

	vectorChain := &TestCardanoChain{
		config:           apexConfig.VectorConfig,
		multisigAddr:     remoteConfig.VectorInfo.MultisigAddr,
		multisigFeeAddr:  remoteConfig.VectorInfo.FeeAddr,
		ogmiosURL:        remoteConfig.VectorInfo.OgmiosURL,
		blockfrostURL:    remoteConfig.VectorInfo.BlockfrostURL,
		blockfrostAPIKey: remoteConfig.VectorInfo.BlockfrostAPIKey,
	}

	nexusChain := &TestEVMChain{
		config:      apexConfig.NexusConfig,
		gatewayAddr: remoteConfig.NexusInfo.GatewayAddress,
		jsonRPCAddr: remoteConfig.NexusInfo.JSONRPCAddr,
	}

	usersData, err := GetTestnetApexUsers(
		NewApexNetworkTypes(apexConfig.PrimeConfig, apexConfig.VectorConfig, nil, apexConfig.NexusConfig))
	if err != nil {
		return nil, err
	}

	apexSystem := &ApexSystem{
		Config:       apexConfig,
		FunderUser:   usersData.Funder,
		Users:        usersData.Users,
		chains:       []ITestApexChain{primeChain, vectorChain, nexusChain},
		bridgingAPIs: remoteConfig.BridgingAPIs,
	}

	apexSystem.PrimeInfo = remoteConfig.PrimeInfo
	apexSystem.VectorInfo = remoteConfig.VectorInfo
	apexSystem.NexusInfo = remoteConfig.NexusInfo

	return apexSystem, nil
}

func SetupSkylineRemoteBridge(
	t *testing.T,
	remoteConfig *RemoteApexBridgeConfig,
	apexOpts ...ApexSystemOptions,
) (*ApexSystem, error) {
	t.Helper()

	apexConfig := &ApexSystemConfig{
		PrimeConfig:   NewRemotePrimeChainConfig(defaultMinBridgingFeeAmount, 0),
		CardanoConfig: NewRemoteCardanoChainConfig(true, defaultMinBridgingFeeAmount, 0),
		APIKey:        remoteConfig.BridgingAPIKey,
	}

	for _, opt := range apexOpts {
		opt(apexConfig)
	}

	primeChain := &TestCardanoChain{
		config:           apexConfig.PrimeConfig,
		multisigAddr:     remoteConfig.PrimeInfo.MultisigAddr,
		multisigFeeAddr:  remoteConfig.PrimeInfo.FeeAddr,
		ogmiosURL:        remoteConfig.PrimeInfo.OgmiosURL,
		blockfrostURL:    remoteConfig.PrimeInfo.BlockfrostURL,
		blockfrostAPIKey: remoteConfig.PrimeInfo.BlockfrostAPIKey,
	}

	cardanoChain := &TestCardanoChain{
		config:           apexConfig.CardanoConfig,
		multisigAddr:     remoteConfig.CardanoInfo.MultisigAddr,
		multisigFeeAddr:  remoteConfig.CardanoInfo.FeeAddr,
		ogmiosURL:        remoteConfig.CardanoInfo.OgmiosURL,
		blockfrostURL:    remoteConfig.CardanoInfo.BlockfrostURL,
		blockfrostAPIKey: remoteConfig.CardanoInfo.BlockfrostAPIKey,
	}

	usersData, err := GetTestnetApexUsers(
		NewApexNetworkTypes(apexConfig.PrimeConfig, nil, apexConfig.CardanoConfig, nil))
	if err != nil {
		return nil, err
	}

	apexSystem := &ApexSystem{
		Config:       apexConfig,
		FunderUser:   usersData.Funder,
		Users:        usersData.Users,
		IsSkyline:    true,
		chains:       []ITestApexChain{primeChain, cardanoChain},
		bridgingAPIs: remoteConfig.BridgingAPIs,
	}

	apexSystem.PrimeInfo = remoteConfig.PrimeInfo
	apexSystem.CardanoInfo = remoteConfig.CardanoInfo

	return apexSystem, nil
}
