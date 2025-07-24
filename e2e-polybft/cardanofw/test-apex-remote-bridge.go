package cardanofw

import (
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/Ethernal-Tech/cardano-infrastructure/wallet"
)

type RemoteApexBridgeConfig struct {
	PrimeInfo      CardanoChainInfo
	VectorInfo     CardanoChainInfo
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
			MultisigAddr:   "addr_test1w2h482rf4gf44ek0rekamxksulazkr64yf2fhmm7f5gxjpsdm4zsg",
			FeeAddr:        "addr_test1wtyslvqxffyppmzhs7ecwunsnpq6g2p6kf9r4aa8ntfzc4qj925fr",
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
			MultisigAddr:   "addr_test1wr44r7qudqwrpsgfs3m4t47x7xmw55dk4k96faak0w4aeqqxxwlvt",
			FeeAddr:        "addr_test1wzct9v2gj9j9rmwx6atkjhcesglf3zcpz6c4y99u3nvg9ksfjj3zd",
		},
		VectorInfo: CardanoChainInfo{
			NetworkAddress: "relay-0.vector.testnet.apexfusion.org:7522",
			OgmiosURL:      "http://ogmios.vector.testnet.apexfusion.org:1337",
			MultisigAddr:   "addr_test1wtnv7cp7revdt70yuc96z4ke9pasa70grc5clhyf7q70f4sheemvm",
			FeeAddr:        "addr_test1wtr7nnz8xg2hmudtfgp9u77uwttkuwef6g26dl6zppmwmsqqnmjc7",
		},
		NexusInfo: EVMChainInfo{
			GatewayAddress: types.StringToAddress("0x00A4436E859532fcc10D2477dcB6441b6C182c3D"),
			JSONRPCAddr:    "https://rpc.nexus.testnet.apexfusion.org",
		},
		BridgingAPIs: []string{
			"http://bridge-api-testnet.apexfusion.org:10003",
		},
		BridgingAPIKey: os.Getenv("PARTNER_TESTNET_BRIDGING_API_KEY"),
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

func GetTestnetApexUsers(
	primeNetworkType wallet.CardanoNetworkType,
	vectorNetworkType wallet.CardanoNetworkType,
) (*ApexUsersData, error) {
	userKeysData, err := GetTestnetUserKeys()
	if err != nil {
		return nil, err
	}

	funder, err := userKeysData.Funder.User(primeNetworkType, vectorNetworkType)
	if err != nil {
		return nil, err
	}

	users := make([]*TestApexUser, len(userKeysData.Users))

	for i, keys := range userKeysData.Users {
		user, err := keys.User(primeNetworkType, vectorNetworkType)
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
		PrimeConfig:  NewRemotePrimeChainConfig(),
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
		apexConfig.PrimeConfig.NetworkType,
		apexConfig.VectorConfig.NetworkType)
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
