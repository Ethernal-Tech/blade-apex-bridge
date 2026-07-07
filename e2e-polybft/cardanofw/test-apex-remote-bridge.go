package cardanofw

import (
	"encoding/json"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"testing"

	"github.com/0xPolygon/polygon-edge/e2e-polybft/e2eindexer"
	"github.com/0xPolygon/polygon-edge/types"
	cardanowallet "github.com/Ethernal-Tech/cardano-infrastructure/wallet"
)

type RemoteCardanoChainConfig struct {
	Info                    CardanoChainInfo
	DefaultMinBridgingFee   uint64
	MinBridgingFeeForTokens uint64
	MinOperationFee         uint64
	TreasuryAddress         string
}

type RemoteEVMChainConfig struct {
	Info            EVMChainInfo
	MinBridgingFee  *big.Int
	MinOperationFee *big.Int
	TreasuryAddress string
}

type RemoteSolanaChainConfig struct {
	Info            SolanaChainInfo
	MinBridgingFee  *big.Int
	MinOperationFee *big.Int
	TreasuryAddress string
}

type RemoteApexBridgeConfig struct {
	CardanoChains  map[string]RemoteCardanoChainConfig
	EVMChains      map[string]RemoteEVMChainConfig
	SolanaChains   map[string]RemoteSolanaChainConfig
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
		CardanoChains: map[string]RemoteCardanoChainConfig{
			ChainIDPrime: {
				Info: CardanoChainInfo{
					NetworkAddress: "relay-0.prime.testnet.apexfusion.org:5521",
					OgmiosURL:      "http://ogmios.prime.testnet.apexfusion.org:1337",
					MultisigAddr:   []string{"addr_test1wrz24vv4tvfqsywkxn36rv5zagys2d7euafcgt50gmpgqpq4ju9uv"},
					FeeAddr:        "addr_test1wq5dw0g9mpmjy0xd6g58kncapdf6vgcka9el4llhzwy5vhqz80tcq",
				},
				DefaultMinBridgingFee:   1_000_010,
				MinBridgingFeeForTokens: 1_000_010,
			},
		},
		EVMChains: map[string]RemoteEVMChainConfig{
			ChainIDNexus: {
				Info: EVMChainInfo{
					GatewayAddress: types.StringToAddress("0xc68221AD72397d85084f2D5C7089e4e9487c118c"),
					JSONRPCAddr:    "https://rpc.nexus.testnet.apexfusion.org",
				},
				MinBridgingFee: DfmToWei(big.NewInt(1_000_010)),
			},
		},
		BridgingAPIs: []string{
			"http://internal-bridge-api-testnet.apexfusion.org:10003",
		},
		BridgingAPIKey: os.Getenv("TESTNET_BRIDGING_API_KEY"),
	}
}

func GetPartnerTestnetApexBridgeConfig() *RemoteApexBridgeConfig {
	return &RemoteApexBridgeConfig{
		CardanoChains: map[string]RemoteCardanoChainConfig{
			ChainIDPrime: {
				Info: CardanoChainInfo{
					NetworkAddress: "relay-0.prime.testnet.apexfusion.org:5521",
					OgmiosURL:      "http://ogmios.prime.testnet.apexfusion.org:1337",
					MultisigAddr:   []string{"addr_test1wr44r7qudqwrpsgfs3m4t47x7xmw55dk4k96faak0w4aeqqxxwlvt"},
					FeeAddr:        "addr_test1wzct9v2gj9j9rmwx6atkjhcesglf3zcpz6c4y99u3nvg9ksfjj3zd",
				},
				DefaultMinBridgingFee:   1_000_010,
				MinBridgingFeeForTokens: 1_000_010,
			},
			ChainIDVector: {
				Info: CardanoChainInfo{
					NetworkAddress: "vector-node.onprem.ethernal.work:5571",
					OgmiosURL:      "https://vector-ogmios.onprem.ethernal.work",
					MultisigAddr:   []string{"addr1w8nv7cp7revdt70yuc96z4ke9pasa70grc5clhyf7q70f4spev3dn"},
					FeeAddr:        "addr1w8r7nnz8xg2hmudtfgp9u77uwttkuwef6g26dl6zppmwmsqknwcek",
				},
				DefaultMinBridgingFee:   1_000_010,
				MinBridgingFeeForTokens: 1_000_010,
			},
		},
		EVMChains: map[string]RemoteEVMChainConfig{
			ChainIDNexus: {
				Info: EVMChainInfo{
					GatewayAddress: types.StringToAddress("0x43Bca3122Efa14C68F9d385e3b4Da8847eca32Ba"),
					JSONRPCAddr:    "https://rpc.nexus.testnet.apexfusion.org",
				},
				MinBridgingFee: DfmToWei(big.NewInt(1_000_010)),
			},
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
		CardanoChains: map[string]RemoteCardanoChainConfig{
			ChainIDPrime: {
				Info: CardanoChainInfo{
					NetworkAddress: "relay-0.prime.testnet.apexfusion.org:5521",
					OgmiosURL:      "http://ogmios.prime.testnet.apexfusion.org:1337",
					MultisigAddr: []string{
						"addr_test1xzg90aa683qrmp7nplcpvjrh33wj0l77wmuzl9fyeljzjwnu8600uw5fkfran3y3knsvvaleyf0u73xdn5gytsqmu9gqjjclpu",
						"addr_test1xr7qm59ynky87nc984an30mvs4mwgc00sndm5l0kn87qx7vze83rmtk58nqqpnfu0jhrey5c5h76w4p88k8p67zvnpns5u600w",
						"addr_test1xqh969hh9fhrr2jcjarf9trsudrng20sa37p09e44dkmv8zgen9tyu67v5lzm65kmad43p2yterufm4ga90l38hdw6aqwftw97",
						"addr_test1xp4vckvhx0y6tlrkapcpyryyjlk5yhw0n3pwd9jqv4t6t0hhsap3pamhjvarygggn5rxsn96yauc40w4y0cezm6dk62s6qlcfy",
					},
					FeeAddr: "addr_test1xr06xce9aq6atg0hwuucxe7eu5g6nx8mmnvw2d2e848cz4y93epqj6zxan4pykvt4ux34uzwcwnts4akrfrus070ntss82juq8", //nolint:lll
					Tokens: map[uint16]Token{
						AP3XTokenID: {
							ChainSpecific:     cardanowallet.AdaTokenName,
							LockUnlock:        true,
							IsWrappedCurrency: false,
						},
					},
					DestChain: map[ChainID][]Direction{
						ChainIDCardano: {
							{
								SourceTokenID:      AP3XTokenID,
								DestinationTokenID: CAP3XTokenID,
								TrackSource:        true,
								TrackDestination:   true,
							},
						},
					},
				},
				DefaultMinBridgingFee:   4_000_000,
				MinBridgingFeeForTokens: 2_860_000,
				MinOperationFee:         0,
				TreasuryAddress:         "",
			},
			ChainIDVector: {
				Info: CardanoChainInfo{
					NetworkAddress: "vector-node.onprem.ethernal.work:5571",
					OgmiosURL:      "https://vector-ogmios.onprem.ethernal.work",
					MultisigAddr: []string{
						"addr1xypy8dp8q9seraqws8ncjnl4wmtctrqu6phcnacke5cuaz2w22ds0zvg63ejvfhrq8ngsyxzfpl2rhqgvpl4qm3uescsuwwf68",
					},
					FeeAddr: "addr1x8m2clera4ucuj9hwvmux6k4g9mdplqna0ezg0fkd5u3r3ngx8nt5azu82cqd0plerhpg38a8wg6rwtj5jvz3epyh3sq42gx85",
					Tokens: map[uint16]Token{
						AP3XTokenID: {
							ChainSpecific:     cardanowallet.AdaTokenName,
							LockUnlock:        true,
							IsWrappedCurrency: false,
						},
						XADATokenID: {
							ChainSpecific: cardanowallet.NewToken(
								"e243e802ff88962c9084a13de96fa875c8a6bb3ef2d1d29b0a0a7e90", "wADA").String(),
							LockUnlock:        true,
							IsWrappedCurrency: true,
						},
						USDTTokenID: {
							ChainSpecific: cardanowallet.NewToken(
								"ae97251d15dd961a8f2f6dc54a50daa8e34a9f18c793a96b62f72d24", "myTestToken").String(),
							LockUnlock:        false,
							IsWrappedCurrency: false,
						},
						ASOLTokenID: {
							ChainSpecific: cardanowallet.NewToken(
								"ae97251d15dd961a8f2f6dc54a50daa8e34a9f18c793a96b62f72d24", "xwSOL").String(),
							LockUnlock:        false,
							IsWrappedCurrency: false,
						},
					},
					DestChain: map[ChainID][]Direction{
						ChainIDCardano: {
							{
								SourceTokenID:      XADATokenID,
								DestinationTokenID: ADATokenID,
								TrackSource:        true,
								TrackDestination:   true,
							},
						},
						ChainIDNexus: {
							{
								SourceTokenID:      XADATokenID,
								DestinationTokenID: XADATokenID,
								TrackSource:        false,
								TrackDestination:   false,
							},
							{
								SourceTokenID:      USDTTokenID,
								DestinationTokenID: USDTTokenID,
								TrackSource:        false,
								TrackDestination:   false,
							},
						},
						ChainIDSolana: {
							{
								SourceTokenID:      ASOLTokenID,
								DestinationTokenID: WSOLTokenID,
								TrackSource:        false,
								TrackDestination:   false,
							},
							{
								SourceTokenID:      AP3XTokenID,
								DestinationTokenID: SAP3XTokenID,
								TrackSource:        true,
								TrackDestination:   false,
							},
						},
					},
				},
				DefaultMinBridgingFee:   4_000_000,
				MinBridgingFeeForTokens: 2_860_000,
				MinOperationFee:         0,
				TreasuryAddress:         "",
			},
			ChainIDCardano: {
				Info: CardanoChainInfo{
					NetworkAddress: "preview-node.onprem.ethernal.work:5561",
					OgmiosURL:      "https://preview-ogmios.onprem.ethernal.work",
					MultisigAddr:   []string{"addr_test1xp3g6ayyt3e0m9w3jtxr84mf877nhqh4snt2g7ww43yf6lx4w8kmdszpx27e3wpawvkcqcrhrl9ra09stpe8ahtznzesm8x8rk"}, //nolint:lll
					FeeAddr:        "addr_test1xz429ta7d8akqvk6rtkavja8kshy4m3dplm2sgx60rp0fk3pmuk902u7lh609tzz54f32s49s5uf6sphu2zer00a2k4qkq40f9",           //nolint:lll
					Tokens: map[uint16]Token{
						ADATokenID: {
							ChainSpecific: cardanowallet.AdaTokenName,
							LockUnlock:    true,
						},
						CAP3XTokenID: {
							ChainSpecific: cardanowallet.NewToken(
								"64c6ea243c3133d44f2022299e74b027f02b1c13397324819e8465c7", "WAPEX").String(),
							LockUnlock:        true,
							IsWrappedCurrency: true,
						},
						CPOLTokenID: {
							ChainSpecific: cardanowallet.NewToken(
								"1601bc807001f56ed18509f2b5e3b1d04ea333dd3c0d48de7ba765d6", "cPOL").String(),
							LockUnlock:        false,
							IsWrappedCurrency: false,
						},
						CKatanaETHTokenID: {
							ChainSpecific: cardanowallet.NewToken(
								"1601bc807001f56ed18509f2b5e3b1d04ea333dd3c0d48de7ba765d6", "cKatanaETH").String(),
							LockUnlock:        false,
							IsWrappedCurrency: false,
						},
						CETHTokenID: {
							ChainSpecific: cardanowallet.NewToken(
								"1601bc807001f56ed18509f2b5e3b1d04ea333dd3c0d48de7ba765d6", "cETH").String(),
							LockUnlock:        false,
							IsWrappedCurrency: false,
						},
						CSEITokenID: {
							ChainSpecific: cardanowallet.NewToken(
								"1601bc807001f56ed18509f2b5e3b1d04ea333dd3c0d48de7ba765d6", "cSEI").String(),
							LockUnlock:        false,
							IsWrappedCurrency: false,
						},
						CArbitrumETHTokenID: {
							ChainSpecific: cardanowallet.NewToken(
								"1601bc807001f56ed18509f2b5e3b1d04ea333dd3c0d48de7ba765d6", "cArbitrumETH").String(),
							LockUnlock:        false,
							IsWrappedCurrency: false,
						},
						CScrollETHTokenID: {
							ChainSpecific: cardanowallet.NewToken(
								"1601bc807001f56ed18509f2b5e3b1d04ea333dd3c0d48de7ba765d6", "cScrollETH").String(),
							LockUnlock:        false,
							IsWrappedCurrency: false,
						},
						CUnichainETHTokenID: {
							ChainSpecific: cardanowallet.NewToken(
								"1601bc807001f56ed18509f2b5e3b1d04ea333dd3c0d48de7ba765d6", "cUnichainETH").String(),
							LockUnlock:        false,
							IsWrappedCurrency: false,
						},
					},
					DestChain: map[ChainID][]Direction{
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
						ChainIDNexus: {
							{
								SourceTokenID:      ADATokenID,
								DestinationTokenID: XADATokenID,
								TrackSource:        true,
								TrackDestination:   false,
							},
						},
						ChainIDPolygon: {
							{
								SourceTokenID:      CPOLTokenID,
								DestinationTokenID: POLTokenID,
								TrackSource:        false,
								TrackDestination:   false,
							},
						},
						ChainIDEthereum: {
							{
								SourceTokenID:      CETHTokenID,
								DestinationTokenID: ETHTokenID,
								TrackSource:        false,
								TrackDestination:   false,
							},
						},
						ChainIDKatana: {
							{
								SourceTokenID:      CKatanaETHTokenID,
								DestinationTokenID: KatanaETHTokenID,
								TrackSource:        false,
								TrackDestination:   false,
							},
						},
						ChainIDSei: {
							{
								SourceTokenID:      CSEITokenID,
								DestinationTokenID: SEITokenID,
								TrackSource:        false,
								TrackDestination:   false,
							},
						},
						ChainIDArbitrum: {
							{
								SourceTokenID:      CArbitrumETHTokenID,
								DestinationTokenID: ArbitrumETHTokenID,
								TrackSource:        false,
								TrackDestination:   false,
							},
						},
						ChainIDScroll: {
							{
								SourceTokenID:      CScrollETHTokenID,
								DestinationTokenID: ScrollETHTokenID,
								TrackSource:        false,
								TrackDestination:   false,
							},
						},
						ChainIDUnichain: {
							{
								SourceTokenID:      CUnichainETHTokenID,
								DestinationTokenID: UnichainETHTokenID,
								TrackSource:        false,
								TrackDestination:   false,
							},
						},
					},
				},
				DefaultMinBridgingFee:   4_000_000,
				MinBridgingFeeForTokens: 2_860_000,
				MinOperationFee:         0,
				TreasuryAddress:         "",
			},
		},
		EVMChains: map[string]RemoteEVMChainConfig{
			ChainIDNexus: {
				Info: EVMChainInfo{
					GatewayAddress:           types.StringToAddress("0x53F9124643E3D15f8d753733C5d908CD6aA65178"),
					NativeTokenWalletAddress: types.StringToAddress("0x55f32E6DbDC141fd395555a4238bD15FDC386F8D"),
					JSONRPCAddr:              "https://rpc.nexus.testnet.apexfusion.org",
					Tokens: map[uint16]Token{
						AP3XTokenID: {
							ChainSpecific:     cardanowallet.AdaTokenName,
							LockUnlock:        true,
							IsWrappedCurrency: false,
						},
						XADATokenID: {
							ChainSpecific:     "0xEB8cDa7443d0eDbe917Ae19ADFc02d460DDfCC9f",
							LockUnlock:        false,
							IsWrappedCurrency: false,
						},
						USDTTokenID: {
							ChainSpecific:     "0xEb0d073E1Da42d1cA3609F6DcA26547945D37cC0",
							LockUnlock:        true,
							IsWrappedCurrency: false,
						},
						XPOLTokenID: {
							ChainSpecific:     "0xD273f181d575aD1a3b9d1f555EA3982b3FBFd825",
							LockUnlock:        false,
							IsWrappedCurrency: false,
						},
					},
					DestChain: map[ChainID][]Direction{
						ChainIDCardano: {
							{
								SourceTokenID:      XADATokenID,
								DestinationTokenID: ADATokenID,
								TrackSource:        false,
								TrackDestination:   true,
							},
						},
						ChainIDVector: {
							{
								SourceTokenID:      XADATokenID,
								DestinationTokenID: XADATokenID,
								TrackSource:        false,
								TrackDestination:   false,
							},
							{
								SourceTokenID:      USDTTokenID,
								DestinationTokenID: USDTTokenID,
								TrackSource:        false,
								TrackDestination:   false,
							},
						},
						ChainIDPolygon: {
							{
								SourceTokenID:      AP3XTokenID,
								DestinationTokenID: PAP3XTokenID,
								TrackSource:        false,
								TrackDestination:   false,
							},
							{
								SourceTokenID:      XPOLTokenID,
								DestinationTokenID: POLTokenID,
								TrackSource:        false,
								TrackDestination:   false,
							},
						},
					},
				},
				MinBridgingFee:  ApexToWei(big.NewInt(4)),
				MinOperationFee: big.NewInt(0),
				TreasuryAddress: "",
			},
			ChainIDPolygon: {
				Info: EVMChainInfo{
					GatewayAddress:           types.StringToAddress("0xb21565df525a795e18C12d49d710766774dAEaDe"),
					NativeTokenWalletAddress: types.StringToAddress("0x55A1A578fCc44A9E403E6F411CFb2B61A7eef5b2"),
					JSONRPCAddr:              "https://rpc-amoy.polygon.technology",
					Tokens: map[uint16]Token{
						POLTokenID: {
							ChainSpecific:     cardanowallet.AdaTokenName,
							LockUnlock:        true,
							IsWrappedCurrency: false,
						},
						PAP3XTokenID: {
							ChainSpecific:     "0x325E3AEf88F57d9DCA1744cEe740cD8104d1814a",
							LockUnlock:        false,
							IsWrappedCurrency: false,
						},
					},
					DestChain: map[ChainID][]Direction{
						ChainIDNexus: {
							{
								SourceTokenID:      POLTokenID,
								DestinationTokenID: XPOLTokenID,
								TrackSource:        false,
								TrackDestination:   false,
							},
							{
								SourceTokenID:      PAP3XTokenID,
								DestinationTokenID: AP3XTokenID,
								TrackSource:        false,
								TrackDestination:   false,
							},
						},
						ChainIDCardano: {
							{
								SourceTokenID:      POLTokenID,
								DestinationTokenID: CPOLTokenID,
								TrackSource:        false,
								TrackDestination:   false,
							},
						},
					},
				},
				MinBridgingFee:  defaultMinBridgingFeeAmountEvm[ChainIDPolygon],
				MinOperationFee: big.NewInt(0),
				TreasuryAddress: "",
			},
			ChainIDEthereum: {
				Info: EVMChainInfo{
					GatewayAddress:           types.StringToAddress("0x92D7d42368d2092954B56BFeedc87fF34F81CcB6"),
					NativeTokenWalletAddress: types.StringToAddress("0xaDf8263B7D5A8E9Af2c67B39852cF90bD92D7b73"),
					JSONRPCAddr:              "https://ethereum-sepolia-rpc.publicnode.com",
					Tokens: map[uint16]Token{
						ETHTokenID: {
							ChainSpecific:     cardanowallet.AdaTokenName,
							LockUnlock:        true,
							IsWrappedCurrency: false,
						},
					},
					DestChain: map[ChainID][]Direction{
						ChainIDCardano: {
							{
								SourceTokenID:      ETHTokenID,
								DestinationTokenID: CETHTokenID,
								TrackSource:        false,
								TrackDestination:   false,
							},
						},
					},
				},
				MinBridgingFee:  defaultMinBridgingFeeAmountEvm[ChainIDEthereum],
				MinOperationFee: big.NewInt(0),
				TreasuryAddress: "",
			},
			ChainIDKatana: {
				Info: EVMChainInfo{
					GatewayAddress:           types.StringToAddress("0x0389D656eb1EC436cBc662329462F51A70e7e29d"),
					NativeTokenWalletAddress: types.StringToAddress("0xe26C3C393261a821B49AC8D72c9EAD1e90435726"),
					JSONRPCAddr:              "https://rpc-bokuto.katanarpc.com",
					Tokens: map[uint16]Token{
						KatanaETHTokenID: {
							ChainSpecific:     cardanowallet.AdaTokenName,
							LockUnlock:        true,
							IsWrappedCurrency: false,
						},
					},
					DestChain: map[ChainID][]Direction{
						ChainIDCardano: {
							{
								SourceTokenID:      KatanaETHTokenID,
								DestinationTokenID: CKatanaETHTokenID,
								TrackSource:        false,
								TrackDestination:   false,
							},
						},
					},
				},
				MinBridgingFee:  defaultMinBridgingFeeAmountEvm[ChainIDKatana],
				MinOperationFee: big.NewInt(0),
				TreasuryAddress: "",
			},
			ChainIDSei: {
				Info: EVMChainInfo{
					GatewayAddress:           types.StringToAddress("0x92D7d42368d2092954B56BFeedc87fF34F81CcB6"),
					NativeTokenWalletAddress: types.StringToAddress("0xaDf8263B7D5A8E9Af2c67B39852cF90bD92D7b73"),
					JSONRPCAddr:              "https://evm-rpc-testnet.sei-apis.com",
					Tokens: map[uint16]Token{
						SEITokenID: {
							ChainSpecific:     cardanowallet.AdaTokenName,
							LockUnlock:        true,
							IsWrappedCurrency: false,
						},
					},
					DestChain: map[ChainID][]Direction{
						ChainIDCardano: {
							{
								SourceTokenID:      SEITokenID,
								DestinationTokenID: CSEITokenID,
								TrackSource:        false,
								TrackDestination:   false,
							},
						},
					},
				},
				MinBridgingFee:  defaultMinBridgingFeeAmountEvm[ChainIDSei],
				MinOperationFee: big.NewInt(0),
				TreasuryAddress: "",
			},
			ChainIDArbitrum: {
				Info: EVMChainInfo{
					GatewayAddress:           types.StringToAddress("0x92D7d42368d2092954B56BFeedc87fF34F81CcB6"),
					NativeTokenWalletAddress: types.StringToAddress("0xaDf8263B7D5A8E9Af2c67B39852cF90bD92D7b73"),
					JSONRPCAddr:              "https://sepolia-rollup.arbitrum.io/rpc",
					Tokens: map[uint16]Token{
						ArbitrumETHTokenID: {
							ChainSpecific:     cardanowallet.AdaTokenName,
							LockUnlock:        true,
							IsWrappedCurrency: false,
						},
					},
					DestChain: map[ChainID][]Direction{
						ChainIDCardano: {
							{
								SourceTokenID:      ArbitrumETHTokenID,
								DestinationTokenID: CArbitrumETHTokenID,
								TrackSource:        false,
								TrackDestination:   false,
							},
						},
					},
				},
				MinBridgingFee:  defaultMinBridgingFeeAmountEvm[ChainIDArbitrum],
				MinOperationFee: big.NewInt(0),
				TreasuryAddress: "",
			},
			ChainIDScroll: {
				Info: EVMChainInfo{
					GatewayAddress:           types.StringToAddress("0x92D7d42368d2092954B56BFeedc87fF34F81CcB6"),
					NativeTokenWalletAddress: types.StringToAddress("0xaDf8263B7D5A8E9Af2c67B39852cF90bD92D7b73"),
					JSONRPCAddr:              "https://sepolia-rpc.scroll.io",
					Tokens: map[uint16]Token{
						ScrollETHTokenID: {
							ChainSpecific:     cardanowallet.AdaTokenName,
							LockUnlock:        true,
							IsWrappedCurrency: false,
						},
					},
					DestChain: map[ChainID][]Direction{
						ChainIDCardano: {
							{
								SourceTokenID:      ScrollETHTokenID,
								DestinationTokenID: CScrollETHTokenID,
								TrackSource:        false,
								TrackDestination:   false,
							},
						},
					},
				},
				MinBridgingFee:  defaultMinBridgingFeeAmountEvm[ChainIDScroll],
				MinOperationFee: big.NewInt(0),
				TreasuryAddress: "",
			},
			ChainIDUnichain: {
				Info: EVMChainInfo{
					GatewayAddress:           types.StringToAddress("0x92D7d42368d2092954B56BFeedc87fF34F81CcB6"),
					NativeTokenWalletAddress: types.StringToAddress("0xaDf8263B7D5A8E9Af2c67B39852cF90bD92D7b73"),
					JSONRPCAddr:              "https://unichain-sepolia-rpc.publicnode.com",
					Tokens: map[uint16]Token{
						UnichainETHTokenID: {
							ChainSpecific:     cardanowallet.AdaTokenName,
							LockUnlock:        true,
							IsWrappedCurrency: false,
						},
					},
					DestChain: map[ChainID][]Direction{
						ChainIDCardano: {
							{
								SourceTokenID:      UnichainETHTokenID,
								DestinationTokenID: CUnichainETHTokenID,
								TrackSource:        false,
								TrackDestination:   false,
							},
						},
					},
				},
				MinBridgingFee:  defaultMinBridgingFeeAmountEvm[ChainIDUnichain],
				MinOperationFee: big.NewInt(0),
				TreasuryAddress: "",
			},
		},
		SolanaChains: map[string]RemoteSolanaChainConfig{
			ChainIDSolana: {
				Info: SolanaChainInfo{
					DestChain: map[ChainID][]Direction{
						ChainIDVector: {
							{
								SourceTokenID:      WSOLTokenID,
								DestinationTokenID: ASOLTokenID,
								TrackSource:        false,
								TrackDestination:   false,
							},
							{
								SourceTokenID:      SAP3XTokenID,
								DestinationTokenID: AP3XTokenID,
								TrackSource:        false,
								TrackDestination:   true,
							},
						},
					},
					Tokens: map[uint16]Token{
						SOLTokenID: {
							ChainSpecific:     cardanowallet.AdaTokenName,
							LockUnlock:        true,
							IsWrappedCurrency: false,
						},
						WSOLTokenID: {
							ChainSpecific:     WSOLMintAddress,
							LockUnlock:        true,
							IsWrappedCurrency: false,
						},
						SAP3XTokenID: {
							ChainSpecific:     "6V2Qv5UddyqAiZR12JF9yE4aed2TQ3rmyZYb7CegXiA6",
							LockUnlock:        false,
							IsWrappedCurrency: false,
						},
					},
					RelayerAddress: "7bP47jShWo1xn1gVX4Be5oNNwcwLCKKmCt7aX2W2go8c",
					JSONRPCAddr:    "https://api.devnet.solana.com",
					ProgramID:      "6R9GdZEpwBFTicsZCqN7e7P4gqoKDTdiDQSceJz5pGHY",
					AltPublicKey:   "9Xf3VFhcs1ZW55NBuSqhDaSX3Jb1PbNHLwrtgh6j4TJG",
				},
				MinBridgingFee:  big.NewInt(6000000),
				MinOperationFee: big.NewInt(0),
				TreasuryAddress: "BrQciKpBZg47NU8x3chSFAnXUoY9zszRbsy6oGm8Dp3p",
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

	chainIDConfigDir := filepath.Join("..", "/cardanofw/test-configs")

	vectorEnabled := len(remoteConfig.CardanoChains[ChainIDVector].Info.MultisigAddr) > 0

	primeRemoteConfig := remoteConfig.CardanoChains[ChainIDPrime]
	vectorRemoteConfig := remoteConfig.CardanoChains[ChainIDVector]
	nexusRemoteConfig := remoteConfig.EVMChains[ChainIDNexus]
	apexConfig := &ApexSystemConfig{
		PrimeConfig: NewRemotePrimeChainConfig(
			primeRemoteConfig.DefaultMinBridgingFee,
			primeRemoteConfig.MinBridgingFeeForTokens, primeRemoteConfig.MinOperationFee, primeRemoteConfig.TreasuryAddress),
		VectorConfig: NewRemoteVectorChainConfig(
			vectorRemoteConfig.DefaultMinBridgingFee,
			vectorRemoteConfig.MinBridgingFeeForTokens, vectorRemoteConfig.MinOperationFee, vectorRemoteConfig.TreasuryAddress),
		NexusConfig: NewRemoteNexusChainConfig(true,
			nexusRemoteConfig.MinBridgingFee, nexusRemoteConfig.MinOperationFee, "", map[uint16]Token{}),
		APIKey: remoteConfig.BridgingAPIKey,
	}

	for _, opt := range apexOpts {
		opt(apexConfig)
	}

	primeChain := &TestCardanoChain{
		config:           apexConfig.PrimeConfig,
		multisigAddr:     primeRemoteConfig.Info.MultisigAddr,
		multisigFeeAddr:  primeRemoteConfig.Info.FeeAddr,
		ogmiosURL:        primeRemoteConfig.Info.OgmiosURL,
		blockfrostURL:    primeRemoteConfig.Info.BlockfrostURL,
		blockfrostAPIKey: primeRemoteConfig.Info.BlockfrostAPIKey,
		indexer:          e2eindexer.NewTxsExecutedComponentDummy(),
	}

	enabledChains := []ITestApexChain{primeChain}

	var vectorChain *TestCardanoChain
	if vectorEnabled {
		vectorChain = &TestCardanoChain{
			config:           apexConfig.VectorConfig,
			multisigAddr:     vectorRemoteConfig.Info.MultisigAddr,
			multisigFeeAddr:  vectorRemoteConfig.Info.FeeAddr,
			ogmiosURL:        vectorRemoteConfig.Info.OgmiosURL,
			blockfrostURL:    vectorRemoteConfig.Info.BlockfrostURL,
			blockfrostAPIKey: vectorRemoteConfig.Info.BlockfrostAPIKey,
			indexer:          e2eindexer.NewTxsExecutedComponentDummy(),
		}

		enabledChains = append(enabledChains, vectorChain)
	}

	nexusChain := &TestEVMChain{
		config:                apexConfig.NexusConfig,
		gatewayAddr:           nexusRemoteConfig.Info.GatewayAddress,
		nativeTokenWalletAddr: nexusRemoteConfig.Info.NativeTokenWalletAddress,
		jsonRPCAddr:           nexusRemoteConfig.Info.JSONRPCAddr,
		indexer:               e2eindexer.NewTxsExecutedComponentDummy(),
	}

	enabledChains = append(enabledChains, nexusChain)

	usersData, err := GetTestnetApexUsers(
		NewApexNetworkTypes(ApexNetworkTypesParams{
			PrimeConfig:  apexConfig.PrimeConfig,
			VectorConfig: apexConfig.VectorConfig,
			NexusConfig:  apexConfig.NexusConfig,
		}),
	)
	if err != nil {
		return nil, err
	}

	enabledChains = append(enabledChains, nexusChain)

	apexSystem := &ApexSystem{
		Config:            apexConfig,
		FunderUser:        usersData.Funder,
		Users:             usersData.Users,
		chains:            enabledChains,
		bridgingAPIs:      remoteConfig.BridgingAPIs,
		PrimeInfo:         primeRemoteConfig.Info,
		VectorInfo:        vectorRemoteConfig.Info,
		NexusInfo:         nexusRemoteConfig.Info,
		chainIDConfigPath: chainIDConfigDir,
	}

	return apexSystem, nil
}

func SetupSkylineRemoteBridge(
	t *testing.T,
	remoteConfig *RemoteApexBridgeConfig,
	apexOpts ...ApexSystemOptions,
) (*ApexSystem, error) {
	t.Helper()

	chainIDConfigDir := filepath.Join("..", "/cardanofw/test-configs")

	primeRemoteConfig := remoteConfig.CardanoChains[ChainIDPrime]
	vectorRemoteConfig := remoteConfig.CardanoChains[ChainIDVector]
	cardanoRemoteConfig := remoteConfig.CardanoChains[ChainIDCardano]
	nexusRemoteConfig := remoteConfig.EVMChains[ChainIDNexus]
	polygonRemoteConfig := remoteConfig.EVMChains[ChainIDPolygon]
	ethereumRemoteConfig := remoteConfig.EVMChains[ChainIDEthereum]
	katanaRemoteConfig := remoteConfig.EVMChains[ChainIDKatana]
	seiRemoteConfig := remoteConfig.EVMChains[ChainIDSei]
	arbitrumRemoteConfig := remoteConfig.EVMChains[ChainIDArbitrum]
	scrollRemoteConfig := remoteConfig.EVMChains[ChainIDScroll]
	unichainRemoteConfig := remoteConfig.EVMChains[ChainIDUnichain]
	solanaRemoteConfig := remoteConfig.SolanaChains[ChainIDSolana]
	apexConfig := &ApexSystemConfig{
		PrimeConfig: NewRemotePrimeChainConfig(
			primeRemoteConfig.DefaultMinBridgingFee, primeRemoteConfig.MinBridgingFeeForTokens,
			primeRemoteConfig.MinOperationFee, primeRemoteConfig.TreasuryAddress),
		VectorConfig: NewRemoteVectorChainConfig(
			vectorRemoteConfig.DefaultMinBridgingFee, vectorRemoteConfig.MinBridgingFeeForTokens,
			vectorRemoteConfig.MinOperationFee, vectorRemoteConfig.TreasuryAddress),
		CardanoConfig: NewRemoteCardanoChainConfig(
			true, cardanoRemoteConfig.DefaultMinBridgingFee, cardanoRemoteConfig.MinBridgingFeeForTokens,
			cardanoRemoteConfig.MinOperationFee, cardanoRemoteConfig.TreasuryAddress),
		NexusConfig: NewRemoteNexusChainConfig(true,
			nexusRemoteConfig.MinBridgingFee, nexusRemoteConfig.MinOperationFee,
			nexusRemoteConfig.TreasuryAddress, nexusRemoteConfig.Info.Tokens),
		PolygonConfig: NewRemotePolygonChainConfig(true,
			polygonRemoteConfig.MinBridgingFee, polygonRemoteConfig.MinOperationFee,
			polygonRemoteConfig.TreasuryAddress, polygonRemoteConfig.Info.Tokens),
		EthereumConfig: NewRemoteEthereumChainConfig(true,
			ethereumRemoteConfig.MinBridgingFee, ethereumRemoteConfig.MinOperationFee,
			ethereumRemoteConfig.TreasuryAddress, ethereumRemoteConfig.Info.Tokens),
		KatanaConfig: NewRemoteKatanaChainConfig(true,
			katanaRemoteConfig.MinBridgingFee, katanaRemoteConfig.MinOperationFee,
			katanaRemoteConfig.TreasuryAddress, katanaRemoteConfig.Info.Tokens),
		SeiConfig: NewRemoteSeiChainConfig(true,
			seiRemoteConfig.MinBridgingFee, seiRemoteConfig.MinOperationFee,
			seiRemoteConfig.TreasuryAddress, seiRemoteConfig.Info.Tokens),
		ArbitrumConfig: NewRemoteArbitrumChainConfig(true,
			arbitrumRemoteConfig.MinBridgingFee, arbitrumRemoteConfig.MinOperationFee,
			arbitrumRemoteConfig.TreasuryAddress, arbitrumRemoteConfig.Info.Tokens),
		ScrollConfig: NewRemoteScrollChainConfig(true,
			scrollRemoteConfig.MinBridgingFee, scrollRemoteConfig.MinOperationFee,
			scrollRemoteConfig.TreasuryAddress, scrollRemoteConfig.Info.Tokens),
		UnichainConfig: NewRemoteUnichainChainConfig(true,
			unichainRemoteConfig.MinBridgingFee, unichainRemoteConfig.MinOperationFee,
			unichainRemoteConfig.TreasuryAddress, unichainRemoteConfig.Info.Tokens),
		SolanaConfig: NewRemoteSolanaChainConfig(true,
			solanaRemoteConfig.MinBridgingFee, solanaRemoteConfig.MinOperationFee, solanaRemoteConfig.TreasuryAddress),
		APIKey: remoteConfig.BridgingAPIKey,
	}

	for _, opt := range apexOpts {
		opt(apexConfig)
	}

	primeChain := &TestCardanoChain{
		config:           apexConfig.PrimeConfig,
		multisigAddr:     primeRemoteConfig.Info.MultisigAddr,
		multisigFeeAddr:  primeRemoteConfig.Info.FeeAddr,
		ogmiosURL:        primeRemoteConfig.Info.OgmiosURL,
		blockfrostURL:    primeRemoteConfig.Info.BlockfrostURL,
		blockfrostAPIKey: primeRemoteConfig.Info.BlockfrostAPIKey,
		indexer:          e2eindexer.NewTxsExecutedComponentDummy(),
	}

	vectorChain := &TestCardanoChain{
		config:           apexConfig.VectorConfig,
		multisigAddr:     vectorRemoteConfig.Info.MultisigAddr,
		multisigFeeAddr:  vectorRemoteConfig.Info.FeeAddr,
		ogmiosURL:        vectorRemoteConfig.Info.OgmiosURL,
		blockfrostURL:    vectorRemoteConfig.Info.BlockfrostURL,
		blockfrostAPIKey: vectorRemoteConfig.Info.BlockfrostAPIKey,
		indexer:          e2eindexer.NewTxsExecutedComponentDummy(),
	}

	cardanoChain := &TestCardanoChain{
		config:           apexConfig.CardanoConfig,
		multisigAddr:     cardanoRemoteConfig.Info.MultisigAddr,
		multisigFeeAddr:  cardanoRemoteConfig.Info.FeeAddr,
		ogmiosURL:        cardanoRemoteConfig.Info.OgmiosURL,
		blockfrostURL:    cardanoRemoteConfig.Info.BlockfrostURL,
		blockfrostAPIKey: cardanoRemoteConfig.Info.BlockfrostAPIKey,
		indexer:          e2eindexer.NewTxsExecutedComponentDummy(),
	}

	nexusChain := &TestEVMChain{
		config:                apexConfig.NexusConfig,
		gatewayAddr:           nexusRemoteConfig.Info.GatewayAddress,
		nativeTokenWalletAddr: nexusRemoteConfig.Info.NativeTokenWalletAddress,
		jsonRPCAddr:           nexusRemoteConfig.Info.JSONRPCAddr,
		indexer:               e2eindexer.NewTxsExecutedComponentDummy(),
	}

	polygonChain := &TestEVMChain{
		config:                apexConfig.PolygonConfig,
		gatewayAddr:           polygonRemoteConfig.Info.GatewayAddress,
		nativeTokenWalletAddr: polygonRemoteConfig.Info.NativeTokenWalletAddress,
		jsonRPCAddr:           polygonRemoteConfig.Info.JSONRPCAddr,
		indexer:               e2eindexer.NewTxsExecutedComponentDummy(),
	}

	ethereumChain := &TestEVMChain{
		config:                apexConfig.EthereumConfig,
		gatewayAddr:           ethereumRemoteConfig.Info.GatewayAddress,
		nativeTokenWalletAddr: ethereumRemoteConfig.Info.NativeTokenWalletAddress,
		jsonRPCAddr:           ethereumRemoteConfig.Info.JSONRPCAddr,
		indexer:               e2eindexer.NewTxsExecutedComponentDummy(),
	}

	katanaChain := &TestEVMChain{
		config:                apexConfig.KatanaConfig,
		gatewayAddr:           katanaRemoteConfig.Info.GatewayAddress,
		nativeTokenWalletAddr: katanaRemoteConfig.Info.NativeTokenWalletAddress,
		jsonRPCAddr:           katanaRemoteConfig.Info.JSONRPCAddr,
		indexer:               e2eindexer.NewTxsExecutedComponentDummy(),
	}

	seiChain := &TestEVMChain{
		config:                apexConfig.SeiConfig,
		gatewayAddr:           seiRemoteConfig.Info.GatewayAddress,
		nativeTokenWalletAddr: seiRemoteConfig.Info.NativeTokenWalletAddress,
		jsonRPCAddr:           seiRemoteConfig.Info.JSONRPCAddr,
		indexer:               e2eindexer.NewTxsExecutedComponentDummy(),
	}

	arbitrumChain := &TestEVMChain{
		config:                apexConfig.ArbitrumConfig,
		gatewayAddr:           arbitrumRemoteConfig.Info.GatewayAddress,
		nativeTokenWalletAddr: arbitrumRemoteConfig.Info.NativeTokenWalletAddress,
		jsonRPCAddr:           arbitrumRemoteConfig.Info.JSONRPCAddr,
		indexer:               e2eindexer.NewTxsExecutedComponentDummy(),
	}

	scrollChain := &TestEVMChain{
		config:                apexConfig.ScrollConfig,
		gatewayAddr:           scrollRemoteConfig.Info.GatewayAddress,
		nativeTokenWalletAddr: scrollRemoteConfig.Info.NativeTokenWalletAddress,
		jsonRPCAddr:           scrollRemoteConfig.Info.JSONRPCAddr,
		indexer:               e2eindexer.NewTxsExecutedComponentDummy(),
	}

	unichainChain := &TestEVMChain{
		config:                apexConfig.UnichainConfig,
		gatewayAddr:           unichainRemoteConfig.Info.GatewayAddress,
		nativeTokenWalletAddr: unichainRemoteConfig.Info.NativeTokenWalletAddress,
		jsonRPCAddr:           unichainRemoteConfig.Info.JSONRPCAddr,
		indexer:               e2eindexer.NewTxsExecutedComponentDummy(),
	}

	solanaChain := &TestSolanaChain{
		config:       apexConfig.SolanaConfig,
		relayerAddr:  solanaRemoteConfig.Info.RelayerAddress,
		jsonRPCAddr:  solanaRemoteConfig.Info.JSONRPCAddr,
		indexer:      e2eindexer.NewTxsExecutedComponentDummy(),
		programID:    solanaRemoteConfig.Info.ProgramID,
		altPublicKey: solanaRemoteConfig.Info.AltPublicKey,
	}

	usersData, err := GetTestnetApexUsers(
		NewApexNetworkTypes(ApexNetworkTypesParams{
			PrimeConfig:    apexConfig.PrimeConfig,
			VectorConfig:   apexConfig.VectorConfig,
			CardanoConfig:  apexConfig.CardanoConfig,
			NexusConfig:    apexConfig.NexusConfig,
			PolygonConfig:  apexConfig.PolygonConfig,
			EthereumConfig: apexConfig.EthereumConfig,
			KatanaConfig:   apexConfig.KatanaConfig,
			SeiConfig:      apexConfig.SeiConfig,
			ArbitrumConfig: apexConfig.ArbitrumConfig,
			ScrollConfig:   apexConfig.ScrollConfig,
			UnichainConfig: apexConfig.UnichainConfig,
			SolanaConfig:   apexConfig.SolanaConfig,
		}),
	)
	if err != nil {
		return nil, err
	}

	apexSystem := &ApexSystem{
		Config:     apexConfig,
		FunderUser: usersData.Funder,
		Users:      usersData.Users,
		IsSkyline:  true,
		chains: []ITestApexChain{
			primeChain, vectorChain, cardanoChain, nexusChain, polygonChain,
			ethereumChain, katanaChain, seiChain, arbitrumChain, scrollChain, unichainChain,
			solanaChain},
		bridgingAPIs: remoteConfig.BridgingAPIs,
		PrimeInfo:    primeRemoteConfig.Info,
		VectorInfo:   vectorRemoteConfig.Info,
		CardanoInfo:  cardanoRemoteConfig.Info,
		NexusInfo:    nexusRemoteConfig.Info,
		PolygonInfo:  polygonRemoteConfig.Info,
		EthereumInfo: ethereumRemoteConfig.Info,
		KatanaInfo:   katanaRemoteConfig.Info,
		SeiInfo:      seiRemoteConfig.Info,
		ArbitrumInfo: arbitrumRemoteConfig.Info,
		ScrollInfo:   scrollRemoteConfig.Info,
		UnichainInfo: unichainRemoteConfig.Info,
		SolanaInfo:   solanaRemoteConfig.Info,
		EcosystemTokens: map[uint16]string{
			AP3XTokenID:         cardanowallet.AdaTokenName,
			ADATokenID:          cardanowallet.AdaTokenName,
			CAP3XTokenID:        CAP3XTokenName,
			XADATokenID:         XADATokenName,
			USDTTokenID:         USDTTokenName,
			POLTokenID:          cardanowallet.AdaTokenName,
			XPOLTokenID:         XPOLTokenName,
			PAP3XTokenID:        PAP3XTokenName,
			CPOLTokenID:         CPOLTokenName,
			KatanaETHTokenID:    cardanowallet.AdaTokenName,
			CKatanaETHTokenID:   CKatanaETHTokenName,
			ETHTokenID:          cardanowallet.AdaTokenName,
			CETHTokenID:         CETHTokenName,
			SEITokenID:          cardanowallet.AdaTokenName,
			CSEITokenID:         CSEITokenName,
			ArbitrumETHTokenID:  cardanowallet.AdaTokenName,
			CArbitrumETHTokenID: CArbitrumETHTokenName,
			ScrollETHTokenID:    cardanowallet.AdaTokenName,
			CScrollETHTokenID:   CScrollETHTokenName,
			UnichainETHTokenID:  cardanowallet.AdaTokenName,
			CUnichainETHTokenID: CUnichainETHTokenName,
			SOLTokenID:          cardanowallet.AdaTokenName,
			WSOLTokenID:         WSOLANATokenName,
			ASOLTokenID:         ASOLTokenName,
			SAP3XTokenID:        SAP3XTokenName,
		},
		chainIDConfigPath: chainIDConfigDir,
	}

	apexSystem.InitTxSendChainConfiguration()

	return apexSystem, nil
}
