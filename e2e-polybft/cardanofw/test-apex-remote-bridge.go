package cardanofw

import (
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/0xPolygon/polygon-edge/e2e-polybft/e2eindexer"
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
			MultisigAddr:   []string{"addr_test1wrz24vv4tvfqsywkxn36rv5zagys2d7euafcgt50gmpgqpq4ju9uv"},
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
			MultisigAddr:   []string{"addr_test1wr44r7qudqwrpsgfs3m4t47x7xmw55dk4k96faak0w4aeqqxxwlvt"},
			FeeAddr:        "addr_test1wzct9v2gj9j9rmwx6atkjhcesglf3zcpz6c4y99u3nvg9ksfjj3zd",
		},
		VectorInfo: CardanoChainInfo{
			NetworkAddress: "vector-node.onprem.ethernal.work:5571",
			OgmiosURL:      "https://vector-ogmios.onprem.ethernal.work",
			MultisigAddr:   []string{"addr1w8nv7cp7revdt70yuc96z4ke9pasa70grc5clhyf7q70f4spev3dn"},
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

func GetTestnetSkylineBridgeConfig() *RemoteApexBridgeConfig {
	return GetPartnerTestnetSkylineBridgeConfig()
}

func GetPartnerTestnetSkylineBridgeConfig() *RemoteApexBridgeConfig {
	return &RemoteApexBridgeConfig{
		PrimeInfo: CardanoChainInfo{
			NetworkAddress: "relay-0.prime.testnet.apexfusion.org:5521",
			OgmiosURL:      "http://ogmios.prime.testnet.apexfusion.org:1337",
			MultisigAddr: []string{
				"addr_test1xzg90aa683qrmp7nplcpvjrh33wj0l77wmuzl9fyeljzjwnu8600uw5fkfran3y3knsvvaleyf0u73xdn5gytsqmu9gqjjclpu",
				"addr_test1xr7qm59ynky87nc984an30mvs4mwgc00sndm5l0kn87qx7vze83rmtk58nqqpnfu0jhrey5c5h76w4p88k8p67zvnpns5u600w",
				"addr_test1xqh969hh9fhrr2jcjarf9trsudrng20sa37p09e44dkmv8zgen9tyu67v5lzm65kmad43p2yterufm4ga90l38hdw6aqwftw97",
				"addr_test1xp4vckvhx0y6tlrkapcpyryyjlk5yhw0n3pwd9jqv4t6t0hhsap3pamhjvarygggn5rxsn96yauc40w4y0cezm6dk62s6qlcfy",
			},
			FeeAddr: "addr_test1xr06xce9aq6atg0hwuucxe7eu5g6nx8mmnvw2d2e848cz4y93epqj6zxan4pykvt4ux34uzwcwnts4akrfrus070ntss82juq8", //nolint:lll
		},
		VectorInfo: CardanoChainInfo{
			NetworkAddress: "vector-node.onprem.ethernal.work:5571",
			OgmiosURL:      "https://vector-ogmios.onprem.ethernal.work",
			MultisigAddr: []string{
				"addr1xypy8dp8q9seraqws8ncjnl4wmtctrqu6phcnacke5cuaz2w22ds0zvg63ejvfhrq8ngsyxzfpl2rhqgvpl4qm3uescsuwwf68",
			},
			FeeAddr: "addr1x8m2clera4ucuj9hwvmux6k4g9mdplqna0ezg0fkd5u3r3ngx8nt5azu82cqd0plerhpg38a8wg6rwtj5jvz3epyh3sq42gx85",
			NativeTokens: []sendtx.TokenExchangeConfig{
				{
					DstChainID: ChainIDCardano,
					TokenName: cardanowallet.NewToken(
						"e243e802ff88962c9084a13de96fa875c8a6bb3ef2d1d29b0a0a7e90", "wADA").String(),
				},
			},
		},
		CardanoInfo: CardanoChainInfo{
			NetworkAddress: "preview-node.onprem.ethernal.work:5561",
			OgmiosURL:      "https://preview-ogmios.onprem.ethernal.work",
			MultisigAddr:   []string{"addr_test1xp3g6ayyt3e0m9w3jtxr84mf877nhqh4snt2g7ww43yf6lx4w8kmdszpx27e3wpawvkcqcrhrl9ra09stpe8ahtznzesm8x8rk"}, //nolint:lll
			FeeAddr:        "addr_test1xz429ta7d8akqvk6rtkavja8kshy4m3dplm2sgx60rp0fk3pmuk902u7lh609tzz54f32s49s5uf6sphu2zer00a2k4qkq40f9",           //nolint:lll
			NativeTokens: []sendtx.TokenExchangeConfig{
				{
					DstChainID: ChainIDPrime,
					TokenName: cardanowallet.NewToken(
						"64c6ea243c3133d44f2022299e74b027f02b1c13397324819e8465c7", "WAPEX").String(),
				},
			},
		},
		BridgingAPIs: []string{
			"http://validator-1-skyline-partner.testnet.ethernal.work:10003",
		},
		BridgingAPIKey: os.Getenv("PARTNER_TESTNET_SKYLINE_BRIDGING_API_KEY"),
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

	vectorEnabled := len(remoteConfig.VectorInfo.MultisigAddr) > 0

	apexConfig := &ApexSystemConfig{
		PrimeConfig:  NewRemotePrimeChainConfig(0, 0),
		VectorConfig: NewRemoteVectorChainConfig(0, 0),
		NexusConfig:  NewRemoteNexusChainConfig(true),
		APIKey:       remoteConfig.BridgingAPIKey,
	}

	for _, opt := range apexOpts {
		opt(apexConfig)
	}

	initAllowedDirections(apexConfig, false)

	primeChain := &TestCardanoChain{
		config:           apexConfig.PrimeConfig,
		multisigAddr:     remoteConfig.PrimeInfo.MultisigAddr,
		multisigFeeAddr:  remoteConfig.PrimeInfo.FeeAddr,
		ogmiosURL:        remoteConfig.PrimeInfo.OgmiosURL,
		blockfrostURL:    remoteConfig.PrimeInfo.BlockfrostURL,
		blockfrostAPIKey: remoteConfig.PrimeInfo.BlockfrostAPIKey,
		indexer:          e2eindexer.NewTxsExecutedComponentDummy(),
	}

	enabledChains := []ITestApexChain{primeChain}

	var vectorChain *TestCardanoChain
	if vectorEnabled {
		vectorChain = &TestCardanoChain{
			config:           apexConfig.VectorConfig,
			multisigAddr:     remoteConfig.VectorInfo.MultisigAddr,
			multisigFeeAddr:  remoteConfig.VectorInfo.FeeAddr,
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
		NewApexNetworkTypes(apexConfig.PrimeConfig, apexConfig.VectorConfig, nil, apexConfig.NexusConfig))
	if err != nil {
		return nil, err
	}

	enabledChains = append(enabledChains, nexusChain)

	apexSystem := &ApexSystem{
		Config:       apexConfig,
		FunderUser:   usersData.Funder,
		Users:        usersData.Users,
		chains:       enabledChains,
		bridgingAPIs: remoteConfig.BridgingAPIs,
		PrimeInfo:    remoteConfig.PrimeInfo,
		VectorInfo:   remoteConfig.VectorInfo,
		NexusInfo:    remoteConfig.NexusInfo,
	}

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
		VectorConfig:  NewRemoteVectorChainConfig(defaultMinBridgingFeeAmount, 0),
		CardanoConfig: NewRemoteCardanoChainConfig(true, defaultMinBridgingFeeAmount, 0),
		APIKey:        remoteConfig.BridgingAPIKey,
	}

	for _, opt := range apexOpts {
		opt(apexConfig)
	}

	initAllowedDirections(apexConfig, true)

	primeChain := &TestCardanoChain{
		config:           apexConfig.PrimeConfig,
		multisigAddr:     remoteConfig.PrimeInfo.MultisigAddr,
		multisigFeeAddr:  remoteConfig.PrimeInfo.FeeAddr,
		ogmiosURL:        remoteConfig.PrimeInfo.OgmiosURL,
		blockfrostURL:    remoteConfig.PrimeInfo.BlockfrostURL,
		blockfrostAPIKey: remoteConfig.PrimeInfo.BlockfrostAPIKey,
		indexer:          e2eindexer.NewTxsExecutedComponentDummy(),
	}

	vectorChain := &TestCardanoChain{
		config:           apexConfig.VectorConfig,
		multisigAddr:     remoteConfig.VectorInfo.MultisigAddr,
		multisigFeeAddr:  remoteConfig.VectorInfo.FeeAddr,
		ogmiosURL:        remoteConfig.VectorInfo.OgmiosURL,
		blockfrostURL:    remoteConfig.VectorInfo.BlockfrostURL,
		blockfrostAPIKey: remoteConfig.VectorInfo.BlockfrostAPIKey,
		indexer:          e2eindexer.NewTxsExecutedComponentDummy(),
	}

	cardanoChain := &TestCardanoChain{
		config:           apexConfig.CardanoConfig,
		multisigAddr:     remoteConfig.CardanoInfo.MultisigAddr,
		multisigFeeAddr:  remoteConfig.CardanoInfo.FeeAddr,
		ogmiosURL:        remoteConfig.CardanoInfo.OgmiosURL,
		blockfrostURL:    remoteConfig.CardanoInfo.BlockfrostURL,
		blockfrostAPIKey: remoteConfig.CardanoInfo.BlockfrostAPIKey,
		indexer:          e2eindexer.NewTxsExecutedComponentDummy(),
	}

	usersData, err := GetTestnetApexUsers(
		NewApexNetworkTypes(apexConfig.PrimeConfig, apexConfig.VectorConfig, apexConfig.CardanoConfig, nil))
	if err != nil {
		return nil, err
	}

	apexSystem := &ApexSystem{
		Config:       apexConfig,
		FunderUser:   usersData.Funder,
		Users:        usersData.Users,
		IsSkyline:    true,
		chains:       []ITestApexChain{primeChain, vectorChain, cardanoChain},
		bridgingAPIs: remoteConfig.BridgingAPIs,
		PrimeInfo:    remoteConfig.PrimeInfo,
		VectorInfo:   remoteConfig.VectorInfo,
		CardanoInfo:  remoteConfig.CardanoInfo,
	}

	apexSystem.InitTxSendChainConfiguration()

	return apexSystem, nil
}
