package cardanofw

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"testing"

	"github.com/0xPolygon/polygon-edge/e2e-polybft/e2eindexer"
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
			NetworkAddress: "vector-node.onprem.ethernal.work:5571",
			OgmiosURL:      "https://vector-ogmios.onprem.ethernal.work",
			MultisigAddr:   "addr1w8nv7cp7revdt70yuc96z4ke9pasa70grc5clhyf7q70f4spev3dn",
			FeeAddr:        "addr1w8r7nnz8xg2hmudtfgp9u77uwttkuwef6g26dl6zppmwmsqknwcek",
		},
		NexusInfo: EVMChainInfo{
			GatewayAddress: types.StringToAddress("0x43Bca3122Efa14C68F9d385e3b4Da8847eca32Ba"),
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

type bridgingAddrs struct {
	Address    string `json:"address"`
	FeeAddress string `json:"feeAddress"`
}

func FetchBridgingAddresses(url, apiKey string) (map[string]bridgingAddrs, error) {
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/api/Settings/GetMultiSigBridgingAddr", url), nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("X-API-Key", apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var r struct {
		BridgingAddress map[string]bridgingAddrs `json:"bridgingAddress"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, err
	}

	return r.BridgingAddress, nil
}

func SetupRemoteApexBridge(
	t *testing.T,
	remoteConfig *RemoteApexBridgeConfig,
	apexOpts ...ApexSystemOptions,
) (*ApexSystem, error) {
	t.Helper()

	vectorEnabled := remoteConfig.VectorInfo.MultisigAddr != ""

	apexConfig := &ApexSystemConfig{
		PrimeConfig:  NewRemotePrimeChainConfig(),
		VectorConfig: NewRemoteVectorChainConfig(vectorEnabled),
		NexusConfig:  NewRemoteNexusChainConfig(true),
		APIKey:       remoteConfig.BridgingAPIKey,
	}

	for _, opt := range apexOpts {
		opt(apexConfig)
	}

	initAllowedDirections(apexConfig)

	addrs, err := FetchBridgingAddresses(remoteConfig.BridgingAPIs[0], remoteConfig.BridgingAPIKey)
	if err != nil {
		return nil, err
	}

	if _, ok := addrs["prime"]; !ok {
		return nil, fmt.Errorf("cannot fetch bridging addresses for prime")
	}

	primeChain := &TestCardanoChain{
		config:           apexConfig.PrimeConfig,
		multisigAddr:     addrs["prime"].Address,
		multisigFeeAddr:  addrs["prime"].FeeAddress,
		ogmiosURL:        remoteConfig.PrimeInfo.OgmiosURL,
		blockfrostURL:    remoteConfig.PrimeInfo.BlockfrostURL,
		blockfrostAPIKey: remoteConfig.PrimeInfo.BlockfrostAPIKey,
		indexer:          e2eindexer.NewTxsExecutedComponentDummy(),
	}

	enabledChains := []ITestApexChain{primeChain}

	var vectorChain *TestCardanoChain

	if vectorEnabled {
		if _, ok := addrs["vector"]; !ok {
			return nil, fmt.Errorf("cannot fetch bridging addresses for vector")
		}

		vectorChain = &TestCardanoChain{
			config:           apexConfig.VectorConfig,
			multisigAddr:     addrs["vector"].Address,
			multisigFeeAddr:  addrs["vector"].FeeAddress,
			ogmiosURL:        remoteConfig.VectorInfo.OgmiosURL,
			blockfrostURL:    remoteConfig.VectorInfo.BlockfrostURL,
			blockfrostAPIKey: remoteConfig.VectorInfo.BlockfrostAPIKey,
			indexer:          e2eindexer.NewTxsExecutedComponentDummy(),
		}

		enabledChains = append(enabledChains, vectorChain)
	}

	nexusChain := &TestEVMChain{
		config:      apexConfig.NexusConfig,
		gatewayAddr: remoteConfig.NexusInfo.GatewayAddress,
		jsonRPCAddr: remoteConfig.NexusInfo.JSONRPCAddr,
		indexer:     e2eindexer.NewTxsExecutedComponentDummy(),
	}

	enabledChains = append(enabledChains, nexusChain)

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
		chains:       enabledChains,
		bridgingAPIs: remoteConfig.BridgingAPIs,
	}

	apexSystem.PrimeInfo = remoteConfig.PrimeInfo
	apexSystem.VectorInfo = remoteConfig.VectorInfo
	apexSystem.NexusInfo = remoteConfig.NexusInfo

	return apexSystem, nil
}
