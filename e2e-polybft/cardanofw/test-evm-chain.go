package cardanofw

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/command/genesis"
	"github.com/0xPolygon/polygon-edge/consensus/polybft"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/e2eindexer"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/framework"
	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/jsonrpc"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
	infracommon "github.com/Ethernal-Tech/cardano-infrastructure/common"
	"github.com/Ethernal-Tech/cardano-infrastructure/sendtx"
	infrawallet "github.com/Ethernal-Tech/cardano-infrastructure/wallet"
	"github.com/Ethernal-Tech/ethgo/abi"

	"github.com/Ethernal-Tech/ethgo"
	"github.com/stretchr/testify/require"
)

const (
	defaultFundEthTokenAmount        = uint64(100_000)
	defaultPremineEthTokenAmount     = uint64(1_000_000_000_000)
	defaultFundRelayerEthTokenAmount = uint64(5)

	defaultEvmTreasuryAddress     = "0xcCB2dDA531690E0eacf03338116Ea214c6379cD4"
	defaultNexusTreasuryAddress   = "0xcCB2dDA531690E0eacf03338116Ea214c6379cD4"
	defaultPolygonTreasuryAddress = "0x721a6a568e78588e8226e8AeEeBa77f8ce7Db62e"

	initContractsTryCount      = 3
	initContractsRetryWaitTime = time.Second * 5
)

type EVMTokenInfo struct {
	ID     uint16
	Name   string
	Symbol string
}

type TestEVMChainConfig struct {
	ChainID   string
	IsEnabled bool

	ValidatorCount         int
	InitialHotWalletAmount *big.Int // in wei
	FundAmount             *big.Int
	FundRelayerAmount      *big.Int
	PreminesAddresses      []types.Address
	PremineAmount          *big.Int
	StartingPort           int64
	ApexConfig             uint8
	BurnContractInfo       *polybft.BurnContractInfo

	MinBridgingFee         *big.Int
	MinBridgingAmount      *big.Int
	MinTokenBridgingAmount *big.Int
	MinOperationFee        *big.Int
	FeeAddrBridging        *big.Int
	CurrencyID             uint16

	TreasuryAddress string

	// Tokens that should be locked/unlocked on this chain
	LockUnlockTokens []EVMTokenInfo

	// Tokens that should be minted on this chain
	MintTokens []EVMTokenInfo

	// Tokens that are being configured while starting the system (minting or locking/unlocking)
	ConfigurableTokens map[uint16]string
}

func NewNexusChainConfig(isEnabled bool) *TestEVMChainConfig {
	return &TestEVMChainConfig{
		ChainID:        ChainIDNexus,
		IsEnabled:      isEnabled,
		ValidatorCount: 4,
		StartingPort:   int64(30400),
		BurnContractInfo: &polybft.BurnContractInfo{
			BlockNumber: 0,
			Address:     types.ZeroAddress,
		},
		ApexConfig:             genesis.ApexConfigEthChain,
		InitialHotWalletAmount: big.NewInt(0),
		PremineAmount:          ApexToWei(new(big.Int).SetUint64(defaultPremineEthTokenAmount)),
		FundAmount:             ApexToWei(new(big.Int).SetUint64(defaultFundEthTokenAmount)),
		FundRelayerAmount:      ApexToWei(new(big.Int).SetUint64(defaultFundRelayerEthTokenAmount)),
		MinBridgingFee:         defaultMinBridgingFeeAmount,
		MinBridgingAmount:      MinUTxODefaultValue,
		MinTokenBridgingAmount: DfmToWei(big.NewInt(1)),
		MinOperationFee:        DefaultMinOperationFee,
		CurrencyID:             AP3XTokenID,

		TreasuryAddress: defaultNexusTreasuryAddress,

		LockUnlockTokens: []EVMTokenInfo{
			{
				ID:     USDTTokenID,
				Name:   USDTTokenName,
				Symbol: USDTTokenName,
			},
			{
				ID:     NSTokenID,
				Name:   NSTokenName,
				Symbol: NSTokenName,
			},
		},
		MintTokens: []EVMTokenInfo{
			{
				ID:     XADATokenID,
				Name:   XADATokenName,
				Symbol: XADATokenName,
			},
			{
				ID:     XPOLTokenID,
				Name:   XPOLTokenName,
				Symbol: XPOLTokenName,
			},
			{
				ID:     ASOLTokenID,
				Name:   ASOLTokenName,
				Symbol: ASOLTokenName,
			},
			{
				ID:     SAP3XTokenID,
				Name:   SAP3XTokenName,
				Symbol: SAP3XTokenName,
			},
		},
	}
}

func getEvmConfigurableTokens(tokensAddrs map[uint16]Token) map[uint16]string {
	configurableTokens := make(map[uint16]string, len(tokensAddrs))

	for id, info := range tokensAddrs {
		if info.ChainSpecific != infrawallet.AdaTokenName {
			configurableTokens[id] = info.ChainSpecific
		}
	}

	return configurableTokens
}

func NewRemoteNexusChainConfig(
	isEnabled bool, minBridgingFeeAmount, minOperationFee *big.Int, treasuryAddress string, tokensAddrs map[uint16]Token,
) *TestEVMChainConfig {
	configurableTokens := getEvmConfigurableTokens(tokensAddrs)

	return &TestEVMChainConfig{
		IsEnabled:       isEnabled,
		ChainID:         ChainIDNexus,
		MinBridgingFee:  minBridgingFeeAmount,
		MinOperationFee: minOperationFee,
		TreasuryAddress: treasuryAddress,
		CurrencyID:      AP3XTokenID,
		LockUnlockTokens: []EVMTokenInfo{
			{
				ID:     USDTTokenID,
				Name:   USDTTokenName,
				Symbol: USDTTokenName,
			},
		},
		MintTokens: []EVMTokenInfo{
			{
				ID:     XADATokenID,
				Name:   XADATokenName,
				Symbol: XADATokenName,
			},
			{
				ID:     XPOLTokenID,
				Name:   XPOLTokenName,
				Symbol: XPOLTokenName,
			},
		},
		ConfigurableTokens: configurableTokens,
	}
}

func NewPolygonChainConfig(isEnabled bool) *TestEVMChainConfig {
	return &TestEVMChainConfig{
		ChainID:        ChainIDPolygon,
		IsEnabled:      isEnabled,
		ValidatorCount: 4,
		StartingPort:   int64(30500),
		BurnContractInfo: &polybft.BurnContractInfo{
			BlockNumber: 0,
			Address:     types.ZeroAddress,
		},
		ApexConfig:             genesis.ApexConfigEthChain,
		InitialHotWalletAmount: big.NewInt(0),
		PremineAmount:          ApexToWei(new(big.Int).SetUint64(defaultPremineEthTokenAmount)),
		FundAmount:             ApexToWei(new(big.Int).SetUint64(defaultFundEthTokenAmount)),
		FundRelayerAmount:      ApexToWei(new(big.Int).SetUint64(defaultFundRelayerEthTokenAmount)),
		MinBridgingFee:         defaultMinBridgingFeeAmountEvm[ChainIDPolygon],
		MinBridgingAmount:      MinUTxODefaultValue,
		MinTokenBridgingAmount: DfmToWei(big.NewInt(1)),
		MinOperationFee:        DefaultMinOperationFee,
		CurrencyID:             POLTokenID,
		FeeAddrBridging:        defaultFeeAddrBridgingAmountEvm[ChainIDPolygon],

		TreasuryAddress: defaultPolygonTreasuryAddress,

		LockUnlockTokens: []EVMTokenInfo{},
		MintTokens: []EVMTokenInfo{
			{
				ID:     PAP3XTokenID,
				Name:   PAP3XTokenName,
				Symbol: PAP3XTokenName,
			},
			{
				ID:     USDTTokenID,
				Name:   USDTTokenName,
				Symbol: USDTTokenName,
			},
		},
	}
}

func NewRemotePolygonChainConfig(
	isEnabled bool, minBridgingFeeAmount, minOperationFee *big.Int, treasuryAddress string, tokensAddrs map[uint16]Token,
) *TestEVMChainConfig {
	configurableTokens := getEvmConfigurableTokens(tokensAddrs)

	return &TestEVMChainConfig{
		IsEnabled:       isEnabled,
		ChainID:         ChainIDPolygon,
		MinBridgingFee:  minBridgingFeeAmount,
		MinOperationFee: minOperationFee,
		CurrencyID:      POLTokenID,
		TreasuryAddress: treasuryAddress,
		MintTokens: []EVMTokenInfo{
			{
				ID:     PAP3XTokenID,
				Name:   PAP3XTokenName,
				Symbol: PAP3XTokenName,
			},
		},
		ConfigurableTokens: configurableTokens,
	}
}

func NewEthereumChainConfig(isEnabled bool) *TestEVMChainConfig {
	return &TestEVMChainConfig{
		ChainID:        ChainIDEthereum,
		IsEnabled:      isEnabled,
		ValidatorCount: 4,
		StartingPort:   int64(30600),
		BurnContractInfo: &polybft.BurnContractInfo{
			BlockNumber: 0,
			Address:     types.ZeroAddress,
		},
		ApexConfig:             genesis.ApexConfigEthChain,
		InitialHotWalletAmount: big.NewInt(0),
		PremineAmount:          ApexToWei(new(big.Int).SetUint64(defaultPremineEthTokenAmount)),
		FundAmount:             ApexToWei(new(big.Int).SetUint64(defaultFundEthTokenAmount)),
		FundRelayerAmount:      ApexToWei(new(big.Int).SetUint64(defaultFundRelayerEthTokenAmount)),
		MinBridgingFee:         defaultMinBridgingFeeAmountEvm[ChainIDEthereum],
		MinBridgingAmount:      MinUTxODefaultValue,
		MinTokenBridgingAmount: DfmToWei(big.NewInt(1)),
		MinOperationFee:        DfmToWei(DefaultMinOperationFee),
		CurrencyID:             ETHTokenID,
		FeeAddrBridging:        defaultFeeAddrBridgingAmountEvm[ChainIDEthereum],

		TreasuryAddress: defaultEvmTreasuryAddress,

		LockUnlockTokens: []EVMTokenInfo{},
		MintTokens:       []EVMTokenInfo{},
	}
}

func NewRemoteEthereumChainConfig(
	isEnabled bool, minBridgingFeeAmount, minOperationFee *big.Int, treasuryAddress string, tokensAddrs map[uint16]Token,
) *TestEVMChainConfig {
	configurableTokens := getEvmConfigurableTokens(tokensAddrs)

	return &TestEVMChainConfig{
		IsEnabled:          isEnabled,
		ChainID:            ChainIDEthereum,
		MinBridgingFee:     minBridgingFeeAmount,
		MinOperationFee:    minOperationFee,
		CurrencyID:         ETHTokenID,
		TreasuryAddress:    treasuryAddress,
		MintTokens:         []EVMTokenInfo{},
		ConfigurableTokens: configurableTokens,
	}
}

func NewKatanaChainConfig(isEnabled bool) *TestEVMChainConfig {
	return &TestEVMChainConfig{
		ChainID:        ChainIDKatana,
		IsEnabled:      isEnabled,
		ValidatorCount: 4,
		StartingPort:   int64(30700),
		BurnContractInfo: &polybft.BurnContractInfo{
			BlockNumber: 0,
			Address:     types.ZeroAddress,
		},
		ApexConfig:             genesis.ApexConfigEthChain,
		InitialHotWalletAmount: big.NewInt(0),
		PremineAmount:          ApexToWei(new(big.Int).SetUint64(defaultPremineEthTokenAmount)),
		FundAmount:             ApexToWei(new(big.Int).SetUint64(defaultFundEthTokenAmount)),
		FundRelayerAmount:      ApexToWei(new(big.Int).SetUint64(defaultFundRelayerEthTokenAmount)),
		MinBridgingFee:         defaultMinBridgingFeeAmountEvm[ChainIDKatana],
		MinBridgingAmount:      MinUTxODefaultValue,
		MinTokenBridgingAmount: DfmToWei(big.NewInt(1)),
		MinOperationFee:        DfmToWei(DefaultMinOperationFee),
		CurrencyID:             KatanaETHTokenID,
		FeeAddrBridging:        defaultFeeAddrBridgingAmountEvm[ChainIDKatana],

		TreasuryAddress: defaultEvmTreasuryAddress,

		LockUnlockTokens: []EVMTokenInfo{},
		MintTokens:       []EVMTokenInfo{},
	}
}

func NewRemoteKatanaChainConfig(
	isEnabled bool, minBridgingFeeAmount, minOperationFee *big.Int, treasuryAddress string, tokensAddrs map[uint16]Token,
) *TestEVMChainConfig {
	configurableTokens := getEvmConfigurableTokens(tokensAddrs)

	return &TestEVMChainConfig{
		IsEnabled:          isEnabled,
		ChainID:            ChainIDKatana,
		MinBridgingFee:     minBridgingFeeAmount,
		MinOperationFee:    minOperationFee,
		CurrencyID:         KatanaETHTokenID,
		TreasuryAddress:    treasuryAddress,
		MintTokens:         []EVMTokenInfo{},
		ConfigurableTokens: configurableTokens,
	}
}

func NewSeiChainConfig(isEnabled bool) *TestEVMChainConfig {
	return &TestEVMChainConfig{
		ChainID:        ChainIDSei,
		IsEnabled:      isEnabled,
		ValidatorCount: 4,
		StartingPort:   int64(30800),
		BurnContractInfo: &polybft.BurnContractInfo{
			BlockNumber: 0,
			Address:     types.ZeroAddress,
		},
		ApexConfig:             genesis.ApexConfigEthChain,
		InitialHotWalletAmount: big.NewInt(0),
		PremineAmount:          ApexToWei(new(big.Int).SetUint64(defaultPremineEthTokenAmount)),
		FundAmount:             ApexToWei(new(big.Int).SetUint64(defaultFundEthTokenAmount)),
		FundRelayerAmount:      ApexToWei(new(big.Int).SetUint64(defaultFundRelayerEthTokenAmount)),
		MinBridgingFee:         defaultMinBridgingFeeAmountEvm[ChainIDSei],
		MinBridgingAmount:      MinUTxODefaultValue,
		MinTokenBridgingAmount: DfmToWei(big.NewInt(1)),
		MinOperationFee:        DfmToWei(DefaultMinOperationFee),
		CurrencyID:             SEITokenID,
		FeeAddrBridging:        defaultFeeAddrBridgingAmountEvm[ChainIDSei],

		TreasuryAddress: defaultEvmTreasuryAddress,

		LockUnlockTokens: []EVMTokenInfo{},
		MintTokens:       []EVMTokenInfo{},
	}
}

func NewRemoteSeiChainConfig(
	isEnabled bool, minBridgingFeeAmount, minOperationFee *big.Int, treasuryAddress string, tokensAddrs map[uint16]Token,
) *TestEVMChainConfig {
	configurableTokens := getEvmConfigurableTokens(tokensAddrs)

	return &TestEVMChainConfig{
		IsEnabled:          isEnabled,
		ChainID:            ChainIDSei,
		MinBridgingFee:     minBridgingFeeAmount,
		MinOperationFee:    minOperationFee,
		CurrencyID:         SEITokenID,
		TreasuryAddress:    treasuryAddress,
		MintTokens:         []EVMTokenInfo{},
		ConfigurableTokens: configurableTokens,
	}
}

func NewArbitrumChainConfig(isEnabled bool) *TestEVMChainConfig {
	return &TestEVMChainConfig{
		ChainID:        ChainIDArbitrum,
		IsEnabled:      isEnabled,
		ValidatorCount: 4,
		StartingPort:   int64(30900),
		BurnContractInfo: &polybft.BurnContractInfo{
			BlockNumber: 0,
			Address:     types.ZeroAddress,
		},
		ApexConfig:             genesis.ApexConfigEthChain,
		InitialHotWalletAmount: big.NewInt(0),
		PremineAmount:          ApexToWei(new(big.Int).SetUint64(defaultPremineEthTokenAmount)),
		FundAmount:             ApexToWei(new(big.Int).SetUint64(defaultFundEthTokenAmount)),
		FundRelayerAmount:      ApexToWei(new(big.Int).SetUint64(defaultFundRelayerEthTokenAmount)),
		MinBridgingFee:         defaultMinBridgingFeeAmountEvm[ChainIDArbitrum],
		MinBridgingAmount:      MinUTxODefaultValue,
		MinTokenBridgingAmount: DfmToWei(big.NewInt(1)),
		MinOperationFee:        DfmToWei(DefaultMinOperationFee),
		CurrencyID:             ArbitrumETHTokenID,
		FeeAddrBridging:        defaultFeeAddrBridgingAmountEvm[ChainIDArbitrum],

		TreasuryAddress: defaultEvmTreasuryAddress,

		LockUnlockTokens: []EVMTokenInfo{},
		MintTokens:       []EVMTokenInfo{},
	}
}

func NewRemoteArbitrumChainConfig(
	isEnabled bool, minBridgingFeeAmount, minOperationFee *big.Int, treasuryAddress string, tokensAddrs map[uint16]Token,
) *TestEVMChainConfig {
	configurableTokens := getEvmConfigurableTokens(tokensAddrs)

	return &TestEVMChainConfig{
		IsEnabled:          isEnabled,
		ChainID:            ChainIDArbitrum,
		MinBridgingFee:     minBridgingFeeAmount,
		MinOperationFee:    minOperationFee,
		CurrencyID:         ArbitrumETHTokenID,
		TreasuryAddress:    treasuryAddress,
		MintTokens:         []EVMTokenInfo{},
		ConfigurableTokens: configurableTokens,
	}
}

func NewScrollChainConfig(isEnabled bool) *TestEVMChainConfig {
	return &TestEVMChainConfig{
		ChainID:        ChainIDScroll,
		IsEnabled:      isEnabled,
		ValidatorCount: 4,
		StartingPort:   int64(31000),
		BurnContractInfo: &polybft.BurnContractInfo{
			BlockNumber: 0,
			Address:     types.ZeroAddress,
		},
		ApexConfig:             genesis.ApexConfigEthChain,
		InitialHotWalletAmount: big.NewInt(0),
		PremineAmount:          ApexToWei(new(big.Int).SetUint64(defaultPremineEthTokenAmount)),
		FundAmount:             ApexToWei(new(big.Int).SetUint64(defaultFundEthTokenAmount)),
		FundRelayerAmount:      ApexToWei(new(big.Int).SetUint64(defaultFundRelayerEthTokenAmount)),
		MinBridgingFee:         defaultMinBridgingFeeAmountEvm[ChainIDScroll],
		MinBridgingAmount:      MinUTxODefaultValue,
		MinTokenBridgingAmount: DfmToWei(big.NewInt(1)),
		MinOperationFee:        DfmToWei(DefaultMinOperationFee),
		CurrencyID:             ScrollETHTokenID,
		FeeAddrBridging:        defaultFeeAddrBridgingAmountEvm[ChainIDScroll],

		TreasuryAddress: defaultEvmTreasuryAddress,

		LockUnlockTokens: []EVMTokenInfo{},
		MintTokens:       []EVMTokenInfo{},
	}
}

func NewRemoteScrollChainConfig(
	isEnabled bool, minBridgingFeeAmount, minOperationFee *big.Int, treasuryAddress string, tokensAddrs map[uint16]Token,
) *TestEVMChainConfig {
	configurableTokens := getEvmConfigurableTokens(tokensAddrs)

	return &TestEVMChainConfig{
		IsEnabled:          isEnabled,
		ChainID:            ChainIDScroll,
		MinBridgingFee:     minBridgingFeeAmount,
		MinOperationFee:    minOperationFee,
		CurrencyID:         ScrollETHTokenID,
		TreasuryAddress:    treasuryAddress,
		MintTokens:         []EVMTokenInfo{},
		ConfigurableTokens: configurableTokens,
	}
}

func NewUnichainChainConfig(isEnabled bool) *TestEVMChainConfig {
	return &TestEVMChainConfig{
		ChainID:        ChainIDUnichain,
		IsEnabled:      isEnabled,
		ValidatorCount: 4,
		StartingPort:   int64(31100),
		BurnContractInfo: &polybft.BurnContractInfo{
			BlockNumber: 0,
			Address:     types.ZeroAddress,
		},
		ApexConfig:             genesis.ApexConfigEthChain,
		InitialHotWalletAmount: big.NewInt(0),
		PremineAmount:          ApexToWei(new(big.Int).SetUint64(defaultPremineEthTokenAmount)),
		FundAmount:             ApexToWei(new(big.Int).SetUint64(defaultFundEthTokenAmount)),
		FundRelayerAmount:      ApexToWei(new(big.Int).SetUint64(defaultFundRelayerEthTokenAmount)),
		MinBridgingFee:         defaultMinBridgingFeeAmountEvm[ChainIDUnichain],
		MinBridgingAmount:      MinUTxODefaultValue,
		MinTokenBridgingAmount: DfmToWei(big.NewInt(1)),
		MinOperationFee:        DfmToWei(DefaultMinOperationFee),
		CurrencyID:             UnichainETHTokenID,
		FeeAddrBridging:        defaultFeeAddrBridgingAmountEvm[ChainIDUnichain],

		TreasuryAddress: defaultEvmTreasuryAddress,

		LockUnlockTokens: []EVMTokenInfo{},
		MintTokens:       []EVMTokenInfo{},
	}
}

func NewRemoteUnichainChainConfig(
	isEnabled bool, minBridgingFeeAmount, minOperationFee *big.Int, treasuryAddress string, tokensAddrs map[uint16]Token,
) *TestEVMChainConfig {
	configurableTokens := getEvmConfigurableTokens(tokensAddrs)

	return &TestEVMChainConfig{
		IsEnabled:          isEnabled,
		ChainID:            ChainIDUnichain,
		MinBridgingFee:     minBridgingFeeAmount,
		MinOperationFee:    minOperationFee,
		CurrencyID:         UnichainETHTokenID,
		TreasuryAddress:    treasuryAddress,
		MintTokens:         []EVMTokenInfo{},
		ConfigurableTokens: configurableTokens,
	}
}

type TestEVMChain struct {
	config                *TestEVMChainConfig
	admin                 *crypto.ECDSAKey
	cluster               *framework.TestCluster
	jsonRPCAddr           string
	gatewayAddr           types.Address
	nativeTokenWalletAddr types.Address
	relayerWallet         *crypto.ECDSAKey
	fundBlockNum          uint64
	indexer               e2eindexer.TxsExecutedComponent
}

// GetCustodialAddress implements ITestApexChain.
func (ec *TestEVMChain) GetCustodialAddress() string {
	panic("unimplemented") //nolint:gocritic
}

// SetCustodialNFT implements ITestApexChain.
func (ec *TestEVMChain) SetCustodialNFT(token infrawallet.Token) {}

// GetRelayerAddress implements ITestApexChain.
func (ec *TestEVMChain) GetRelayerAddress() string {
	panic("unimplemented") //nolint:gocritic
}

// GetMintableTokens implements ITestApexChain.
func (ec *TestEVMChain) GetMintableTokens() map[uint16]string {
	// We return the tokens that are being configured while starting the system (minting or locking/unlocking)
	// We use this to populate the apex system config with the tokens that are being configured while starting the system
	return ec.config.ConfigurableTokens
}

// GetMintTokenPolicyID implements ITestApexChain.
func (ec *TestEVMChain) GetCardanoScriptInfo() *CardanoScriptInfo {
	panic("unimplemented") //nolint:gocritic
}

// GetBridgingStakeAddressInfo implements ITestApexChain.
func (ec *TestEVMChain) GetBridgingStakeAddressInfo(
	t *testing.T, ctx context.Context, indx uint8, expectError bool,
) (infrawallet.QueryStakeAddressInfo, error) {
	t.Helper()

	panic("unimplemented") //nolint:gocritic
}

// GetExistingStakePools implements ITestApexChain.
func (ec *TestEVMChain) GetExistingStakePools(t *testing.T, ctx context.Context) []string {
	t.Helper()

	panic("unimplemented") //nolint:gocritic
}

func (ec *TestEVMChain) GetTreasuryAddress() string {
	return ec.config.TreasuryAddress
}

var _ ITestApexChain = (*TestEVMChain)(nil)

func NewTestEVMChain(config *TestEVMChainConfig) (ITestApexChain, error) {
	if !config.IsEnabled {
		getFlag := func(suffix string) string {
			return fmt.Sprintf("--%s-%s", config.ChainID, suffix)
		}

		return NewTestApexChainDummy([]string{
			getFlag("node-url"), "http://localhost:5500",
		}), nil
	}

	admin, err := crypto.GenerateECDSAKey()
	if err != nil {
		return nil, err
	}

	return &TestEVMChain{
		config:  config,
		admin:   admin,
		indexer: e2eindexer.NewTxsExecutedComponentDummy(),
	}, nil
}

func (ec *TestEVMChain) GetServerMust(t *testing.T, indx int) ITestApexChainServer {
	t.Helper()

	require.True(t, ec.cluster != nil && ec.cluster.Servers != nil && len(ec.cluster.Servers) > indx)

	return ec.cluster.Servers[indx]
}

func (ec *TestEVMChain) RunChain(t *testing.T) error {
	t.Helper()

	cluster := framework.NewTestCluster(t, ec.config.ValidatorCount,
		framework.WithPremine(ec.admin.Address()),
		framework.WithPremine(ec.config.PreminesAddresses...),
		framework.WithInitialPort(ec.config.StartingPort),
		framework.WithLogsDirSuffix(ec.config.ChainID),
		framework.WithBladeAdmin(ec.admin.Address().String()),
		framework.WithApexConfig(ec.config.ApexConfig),
		framework.WithBurnContract(ec.config.BurnContractInfo),
	)

	if err := cluster.WaitForBlock(1, time.Minute); err != nil {
		return err
	}

	fmt.Printf("%s chain setup done: port = %d\n", ec.config.ChainID, ec.config.StartingPort)

	ec.cluster = cluster
	ec.jsonRPCAddr = ec.cluster.Servers[0].JSONRPCAddr()

	return nil
}

func (ec *TestEVMChain) Stop() error {
	if ec.cluster != nil {
		ec.cluster.Stop()
	}

	return nil
}

func (ec *TestEVMChain) JSONRPC() (*jsonrpc.EthClient, error) {
	return JSONRPCClient(ec.jsonRPCAddr)
}

func (ec *TestEVMChain) CreateWallets(validator *TestApexValidator) error {
	_, err := validator.getEvmBatcherWallet()
	if err != nil {
		return err
	}

	if validator.ID == RunRelayerOnValidatorID {
		if err = validator.createEvmSpecificWallet(ec.ChainID(), "relayer-evm"); err != nil {
			return err
		}

		ec.relayerWallet, err = validator.getEvmRelayerWallet(ec.ChainID())
		if err != nil {
			return err
		}
	}

	return nil
}

func (ec *TestEVMChain) DeployMintingContract(ctx context.Context, chainIDsConfig string) error {
	fmt.Println("Deploying minting contract for chain =", ec.ChainID())

	pk, err := ec.admin.MarshallPrivateKey()
	if err != nil {
		return err
	}

	regexHelper := func(params []string, expression string) (types.Address, error) {
		var b bytes.Buffer

		err := RunCommand(ResolveApexBridgeBinary(), params, io.MultiWriter(os.Stdout, &b))
		if err != nil {
			return types.Address{}, err
		}

		output := b.String()
		reGateway := regexp.MustCompile(fmt.Sprintf(`%s\s*=\s*0x([a-fA-F0-9]+)`, expression))

		if match := reGateway.FindStringSubmatch(output); len(match) > 0 {
			return types.StringToAddress(match[1]), nil
		}

		return types.Address{}, errors.New("cannot find gateway address")
	}

	// For mint tokens we just register the token on gateway
	tokenAddrs := make(map[uint16]string)

	var (
		execErr   error
		tokenAddr types.Address
	)

	// For lock unlock tokens we need to
	// 1. deploy ERC20 contract for the token
	// 2. register the token on gateway
	for _, token := range ec.config.LockUnlockTokens {
		fmt.Printf("Deploying ERC20 token for token = %+v and chain id = %s\n", token, ec.ChainID())

		if tokenAddr, execErr = ec.deployERC20Token(token); execErr != nil {
			fmt.Printf("Failed to deploy ERC20 token for token = %+v and chain id = %s: %+v\n", token, ec.ChainID(), execErr)

			return err
		}

		fmt.Printf("Deployed ERC20 token for token = %+v, token addr = %+v and chain id = %s\n",
			token, tokenAddr, ec.ChainID())

		params := []string{
			"bridge-admin",
			"register-gateway-token",
			"--chain", ec.ChainID(),
			"--chain-ids-config", chainIDsConfig,
			"--node-url", ec.jsonRPCAddr,
			"--key", hex.EncodeToString(pk),
			"--gateway-addr", ec.gatewayAddr.String(),
			"--token-sc-addr", tokenAddr.String(),
			"--token-id", fmt.Sprint(token.ID),
			"--token-name", token.Name,
			"--token-symbol", token.Symbol,
		}

		if err := retry(ctx, "", func() error {
			tokenAddr, execErr = regexHelper(params, "contractAddr")

			return execErr
		}); err != nil {
			return err
		}

		tokenAddrs[token.ID] = tokenAddr.String()
	}

	for _, token := range ec.config.MintTokens {
		params := []string{
			"bridge-admin",
			"register-gateway-token",
			"--chain", ec.ChainID(),
			"--node-url", ec.jsonRPCAddr,
			"--chain-ids-config", chainIDsConfig,
			"--key", hex.EncodeToString(pk),
			"--gateway-addr", ec.gatewayAddr.String(),
			"--token-sc-addr", "0x0000000000000000000000000000000000000000",
			"--token-id", fmt.Sprint(token.ID),
			"--token-name", token.Name,
			"--token-symbol", token.Symbol,
		}

		if err := retry(ctx, "", func() error {
			tokenAddr, execErr = regexHelper(params, "contractAddr")

			return execErr
		}); err != nil {
			return err
		}

		tokenAddrs[token.ID] = tokenAddr.String()
	}

	ec.config.ConfigurableTokens = tokenAddrs
	fmt.Println("Mintable tokens =", ec.config.ConfigurableTokens)

	return nil
}

func (ec *TestEVMChain) deployERC20Token(token EVMTokenInfo) (types.Address, error) {
	privateKey, err := ec.GetAdminPrivateKey()
	if err != nil {
		return types.ZeroAddress, fmt.Errorf("failed to get admin private key: %w", err)
	}

	privateKeyECDSA, err := crypto.HexToECDSA(privateKey)
	if err != nil {
		return types.ZeroAddress, fmt.Errorf("failed to parse private key: %w", err)
	}

	key := crypto.NewECDSAKey(privateKeyECDSA)

	// Check if SimpleERC20 artifact is loaded
	if contractsapi.SimpleERC20 == nil {
		return types.ZeroAddress, fmt.Errorf("SimpleERC20 artifact is nil")
	}

	if contractsapi.SimpleERC20.Abi == nil {
		return types.ZeroAddress, fmt.Errorf("SimpleERC20 ABI is nil")
	}

	if contractsapi.SimpleERC20.Abi.Constructor == nil {
		return types.ZeroAddress, fmt.Errorf("SimpleERC20 Constructor is nil")
	}

	// Encode constructor with name, symbol
	constructorArgs, err := contractsapi.SimpleERC20.Abi.Constructor.Inputs.Encode([]interface{}{
		token.Name,
		token.Symbol,
	})
	if err != nil {
		return types.ZeroAddress, fmt.Errorf("failed to encode constructor args: %w", err)
	}

	// Combine bytecode + constructor args
	deploymentData := append([]byte{}, contractsapi.SimpleERC20.Bytecode...)
	deploymentData = append(deploymentData, constructorArgs...)

	txRelayer, err := txrelayer.NewTxRelayer(
		txrelayer.WithIPAddress(ec.jsonRPCAddr),
		txrelayer.WithReceiptsTimeout(1*time.Minute),
		txrelayer.WithEstimateGasFallback(),
	)
	if err != nil {
		return types.ZeroAddress, fmt.Errorf("failed to create tx relayer: %w", err)
	}

	tx := types.NewTx(types.NewLegacyTx(
		types.WithFrom(key.Address()),
		types.WithInput(deploymentData),
	))

	receipt, err := txRelayer.SendTransaction(tx, key)
	if err != nil {
		return types.ZeroAddress, fmt.Errorf("failed to send deployment tx: %w", err)
	}

	if receipt.Status != uint64(types.ReceiptSuccess) {
		return types.ZeroAddress, fmt.Errorf(
			"ERC20 deployment failed with status: %d (tx: %s)",
			receipt.Status, receipt.TransactionHash.String())
	}

	if receipt.ContractAddress.String() == "" ||
		receipt.ContractAddress.String() == "0x0000000000000000000000000000000000000000" {
		return types.ZeroAddress, fmt.Errorf("no contract address in receipt")
	}

	fmt.Printf("Successfully deployed ERC20 at: %s\n", receipt.ContractAddress.String())

	return types.StringToAddress(receipt.ContractAddress.String()), nil
}

func (ec *TestEVMChain) FundUsersWithToken(address string, amount *big.Int, tokenID uint16) error {
	// Look up the token contract address
	tokenAddrHex, ok := ec.config.ConfigurableTokens[tokenID]
	if !ok || tokenAddrHex == "" {
		return fmt.Errorf("token with ID %d not found in configured tokens", tokenID)
	}

	tokenAddr := types.StringToAddress(tokenAddrHex)
	recipient := types.StringToAddress(address)

	fmt.Printf("Minting %s tokens (ID: %d) to user %s from contract: %s\n",
		amount.String(), tokenID, recipient.String(), tokenAddr.String())

	// Get admin private key (owner of the ERC20 contract)
	privateKey, err := ec.GetAdminPrivateKey()
	if err != nil {
		return fmt.Errorf("failed to get admin private key: %w", err)
	}

	privateKeyECDSA, err := crypto.HexToECDSA(privateKey)
	if err != nil {
		return fmt.Errorf("failed to parse private key: %w", err)
	}

	key := crypto.NewECDSAKey(privateKeyECDSA)
	adminAddr := key.Address()

	// Check if SimpleERC20 artifact is loaded
	if contractsapi.SimpleERC20 == nil || contractsapi.SimpleERC20.Abi == nil {
		return fmt.Errorf("SimpleERC20 artifact not loaded")
	}

	// Encode mint(address, uint256) call
	mintMethod := contractsapi.SimpleERC20.Abi.Methods["mint"]
	if mintMethod == nil {
		return fmt.Errorf("mint method not found in SimpleERC20 ABI")
	}

	mintData, err := mintMethod.Encode([]interface{}{recipient, amount})
	if err != nil {
		return fmt.Errorf("failed to encode mint call: %w", err)
	}

	// Send mint transaction
	txRelayer, err := txrelayer.NewTxRelayer(
		txrelayer.WithIPAddress(ec.jsonRPCAddr),
		txrelayer.WithReceiptsTimeout(1*time.Minute),
		txrelayer.WithEstimateGasFallback(),
	)
	if err != nil {
		return fmt.Errorf("failed to create tx relayer: %w", err)
	}

	receipt, err := txRelayer.SendTransaction(types.NewTx(types.NewLegacyTx(
		types.WithFrom(adminAddr),
		types.WithTo(&tokenAddr),
		types.WithInput(mintData),
	)), key)
	if err != nil {
		return fmt.Errorf("failed to send mint tx: %w", err)
	}

	if receipt.Status != uint64(types.ReceiptSuccess) {
		return fmt.Errorf("token mint failed with status: %d (tx: %s)", receipt.Status, receipt.TransactionHash.String())
	}

	fmt.Printf("Successfully minted %s tokens to user %s (tx: %s)\n",
		amount.String(), recipient.String(), receipt.TransactionHash.String())

	return nil
}

func (ec *TestEVMChain) CreateAddresses(
	bladeAdmin *crypto.ECDSAKey, bridgeURL, chainIDsConfig string,
) error {
	return nil
}

func (ec *TestEVMChain) FundWallets(ctx context.Context) error {
	privateKey, err := ec.GetAdminPrivateKey()
	if err != nil {
		return err
	}

	if ec.config.FundRelayerAmount != nil && ec.config.FundRelayerAmount.BitLen() > 0 {
		_, err = ec.sendTx(privateKey, ec.relayerWallet.Address().String(), ec.config.FundRelayerAmount, nil)
		if err != nil {
			return err
		}
	}

	if ec.config.FundAmount != nil && ec.config.FundAmount.BitLen() > 0 {
		receipt, err := ec.sendTx(privateKey, ec.gatewayAddr.String(), ec.config.FundAmount, nil)
		if err != nil {
			return err
		}

		ec.fundBlockNum = receipt.BlockNumber
	}

	return nil
}

func (ec *TestEVMChain) InitContracts(
	ctx context.Context, bridgeAdmin *crypto.ECDSAKey, bridgeURL, chainIDsConfig string,
) error {
	pk, err := ec.admin.MarshallPrivateKey()
	if err != nil {
		return err
	}

	regexHelper := func(workingDirectory string, params []string) (types.Address, types.Address, error) {
		// if everything works fine, the working directory will be reused
		if err := common.CreateDirSafe(workingDirectory, 0750); err != nil {
			return types.Address{}, types.Address{}, err
		}

		var b bytes.Buffer

		err := RunCommand(ResolveApexBridgeBinary(), params, io.MultiWriter(os.Stdout, &b))
		if err != nil {
			return types.Address{}, types.Address{}, err
		}

		output := b.String()
		reGateway := regexp.MustCompile(`Gateway Proxy Address\s*=\s*0x([a-fA-F0-9]+)`)
		reNativeTokenWallet := regexp.MustCompile(`NativeTokenWallet Proxy Address\s*=\s*0x([a-fA-F0-9]+)`)

		gatewayMatch := reGateway.FindStringSubmatch(output)
		if gatewayMatch == nil {
			return types.Address{}, types.Address{}, errors.New("cannot find gateway address")
		}

		nativeTokenWalletMatch := reNativeTokenWallet.FindStringSubmatch(output)
		if nativeTokenWalletMatch == nil {
			return types.Address{}, types.Address{}, errors.New("cannot find native token wallet address")
		}

		return types.StringToAddress(gatewayMatch[1]), types.StringToAddress(nativeTokenWalletMatch[1]), nil
	}

	bridgeAdminPk, err := bridgeAdmin.MarshallPrivateKey()
	if err != nil {
		return err
	}

	workingDirectory := filepath.Join(os.TempDir(), "deploy-apex-bridge-evm-gateway")

	params := []string{
		"deploy-evm",
		"--url", ec.jsonRPCAddr,
		"--key", hex.EncodeToString(pk),
		"--chain", ec.ChainID(),
		"--chain-ids-config", chainIDsConfig,
		"--bridge-url", bridgeURL,
		"--bridge-addr", contracts.Bridge.String(),
		"--bridge-key", hex.EncodeToString(bridgeAdminPk),
		"--dir", workingDirectory,
		"--min-fee", ec.config.MinBridgingFee.String(),
		"--min-bridging-amount", ec.config.MinBridgingAmount.String(),
		"--min-token-bridging-amount", ec.config.MinTokenBridgingAmount.String(),
		"--min-operation-fee", ec.config.MinOperationFee.String(),
		"--currency-token-id", fmt.Sprint(ec.config.CurrencyID),
		"--treasury-addr", ec.config.TreasuryAddress,
		"--clone",
	}

	if err := retry(ctx, workingDirectory, func() error {
		var execErr error
		ec.gatewayAddr, ec.nativeTokenWalletAddr, execErr = regexHelper(workingDirectory, params)

		return execErr
	}); err != nil {
		return err
	}

	return nil
}

func retry(ctx context.Context, workingDirectory string, action func() error) error {
	tryCounter := 0

	for {
		if err := action(); err == nil {
			return nil
		} else {
			tryCounter++
			if tryCounter >= initContractsTryCount {
				return err
			}
			// remove directory if something went wrong and try again
			if workingDirectory != "" {
				if err := common.RemoveDirSafe(workingDirectory); err != nil {
					return err
				}
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(initContractsRetryWaitTime):
			}
		}
	}
}

func (ec *TestEVMChain) RegisterChain(validator *TestApexValidator) error {
	return validator.RegisterChain(
		ec.ChainID(), ec.config.InitialHotWalletAmount, big.NewInt(0), ChainTypeEVM)
}

func (ec *TestEVMChain) GenerateChainConfigs(
	indx int,
	validator *TestApexValidator,
) error {
	server := ec.cluster.Servers[indx%len(ec.cluster.Servers)]
	dbsPath := filepath.Join(validator.dataDirPath, BridgingDBsDir)

	args := []string{
		"generate-configs", "evm-chain",
		"--chain-id", ec.ChainID(),
		"--evm-node-url", server.JSONRPCAddr(),
		"--output-dir", validator.GetBridgingConfigsDir(),
		"--output-validator-components-file-name", ValidatorComponentsConfigFileName,
		"--output-relayer-file-name", RelayerConfigFileName,
		"--dbs-path", dbsPath,
		"--relayer-data-dir", validator.server.DataDir(),
		"--evm-min-fee-for-bridging", ec.config.MinBridgingFee.String(),
		"--min-operation-fee", ec.config.MinOperationFee.String(),
	}

	if ec.config.FeeAddrBridging != nil {
		args = append(args,
			"--evm-fee-addr-bridging",
			ec.config.FeeAddrBridging.String(),
		)
	}

	return RunCommand(ResolveApexBridgeBinary(), args, os.Stdout)
}

func (ec *TestEVMChain) PopulateApexSystem(t *testing.T, apexSystem *ApexSystem) error {
	t.Helper()

	switch ec.ChainID() {
	case ChainIDNexus:
		apexSystem.NexusInfo = ec.getChainInfo(t)
	case ChainIDPolygon:
		apexSystem.PolygonInfo = ec.getChainInfo(t)
	case ChainIDEthereum:
		apexSystem.EthereumInfo = ec.getChainInfo(t)
	case ChainIDKatana:
		apexSystem.KatanaInfo = ec.getChainInfo(t)
	case ChainIDSei:
		apexSystem.SeiInfo = ec.getChainInfo(t)
	case ChainIDArbitrum:
		apexSystem.ArbitrumInfo = ec.getChainInfo(t)
	case ChainIDScroll:
		apexSystem.ScrollInfo = ec.getChainInfo(t)
	case ChainIDUnichain:
		apexSystem.UnichainInfo = ec.getChainInfo(t)
	}

	return nil
}

func (ec *TestEVMChain) UpdateTxSendChainConfiguration(_ map[string]sendtx.ChainConfig) {
}

func (ec *TestEVMChain) ChainID() string {
	return ec.config.ChainID
}

func (ec *TestEVMChain) GetAddressBalance(ctx context.Context, addr string) (map[string]*big.Int, error) {
	rpc, err := ec.JSONRPC()
	if err != nil {
		return nil, err
	}

	amount, err := infracommon.ExecuteWithRetry(ctx, func(ctx context.Context) (*big.Int, error) {
		return rpc.GetBalance(types.StringToAddress(addr), jsonrpc.LatestBlockNumberOrHash)
	})
	if err != nil {
		return nil, err
	}

	return map[string]*big.Int{
		infrawallet.AdaTokenName: amount,
	}, err
}

func (ec *TestEVMChain) GetAddressBalanceWithTokenName(
	ctx context.Context, addr string, tokenName string) (map[string]*big.Int, error) {
	if tokenName == infrawallet.AdaTokenName {
		return ec.GetAddressBalance(ctx, addr)
	}

	rpc, err := ec.JSONRPC()
	if err != nil {
		return nil, err
	}

	tokenAddr := types.StringToAddress(tokenName)
	receiverAddress := types.StringToAddress(addr)

	callData, err := (&contractsapi.BalanceOfRootERC20Fn{
		Account: receiverAddress,
	}).EncodeAbi()
	if err != nil {
		return nil, err
	}

	balance, err := infracommon.ExecuteWithRetry(ctx, func(ctx context.Context) (*big.Int, error) {
		outHex, err := rpc.Call(&jsonrpc.CallMsg{
			To:   &tokenAddr,
			Data: callData,
		}, jsonrpc.LatestBlockNumber, nil)
		if err != nil {
			return nil, err
		}

		balance, err := common.ParseUint256orHex(&outHex)
		if err != nil {
			return nil, err
		}

		return balance, nil
	})
	if err != nil {
		return nil, err
	}

	return map[string]*big.Int{
		tokenName: balance,
	}, nil
}

func (ec *TestEVMChain) GetBridgingFee(
	_ context.Context,
	_ string,
	_ []sendtx.BridgingTxReceiver,
	bridgingFee *big.Int,
	_ *big.Int,
	_ string,
) (*big.Int, error) {
	return bridgingFee, nil
}

func (ec *TestEVMChain) CreateMetadata(
	senderAddr string,
	dstChainID string,
	receivers []sendtx.BridgingTxReceiver,
	bridgingFee *big.Int,
	operationFee *big.Int,
) ([]byte, error) {
	return nil, nil
}

func (ec *TestEVMChain) BridgingRequest(brParams BridgingRequestParams) (string, error) {
	//nolint:prealloc
	var params []string

	binary := ""
	if IsCardanoTypeChain(brParams.DestChainID) {
		binary = ResolveCardanoCliBinary(brParams.DestChainID)
	}

	if brParams.IsCurrencySrc && brParams.IsCurrencyDest {
		params = []string{
			"sendtx",
			"--tx-type", "evm",
			"--chain-ids-config", brParams.ChainIDsConfig,
			"--gateway-addr", ec.gatewayAddr.String(),
			"--rpc-url", ec.jsonRPCAddr,
			"--key", brParams.PrivateKey,
			"--chain-src", ec.config.ChainID,
			"--chain-dst", brParams.DestChainID,
			"--fee", brParams.FeeAmount.String(),
			"--operation-fee", brParams.OperationFee.String(),
		}

		if binary != "" {
			params = append(params, "--cardano-cli-binary-name", binary)
		}
	} else {
		receiverTokenID := uint16(0)
		for _, receiver := range brParams.Receivers {
			receiverTokenID = receiver.TokenID

			break
		}

		isTokenLockUnlock := false

		for _, token := range ec.config.LockUnlockTokens {
			if token.ID == receiverTokenID {
				isTokenLockUnlock = true

				break
			}
		}

		params = []string{
			"sendtx",
			"skyline",
			"evm",
			"--chain-ids-config", brParams.ChainIDsConfig,
			"--gateway-addr", ec.gatewayAddr.String(),
			"--rpc-url", ec.jsonRPCAddr,
			"--key", brParams.PrivateKey,
			"--chain-src", ec.config.ChainID,
			"--chain-dst", brParams.DestChainID,
			"--fee", brParams.FeeAmount.String(),
			"--operation-fee", brParams.OperationFee.String(),
			"--src-token-id", fmt.Sprint(receiverTokenID),
		}

		if binary != "" {
			params = append(params, "--cardano-cli-binary-name", binary)
		}

		if brParams.IsCurrencySrc {
			params = append(params, "--src-token-name", infrawallet.AdaTokenName)
		} else if isTokenLockUnlock {
			params = append(params,
				"--native-token-wallet-contract-addr", ec.nativeTokenWalletAddr.String(),
				"--src-token-contract-addr", ec.config.ConfigurableTokens[receiverTokenID])
		}
	}

	for addr, amount := range brParams.Receivers {
		params = append(params,
			"--receiver", fmt.Sprintf("%s:%s", addr, amount.Amount.String()),
		)
	}

	var outb bytes.Buffer

	if err := RunCommand(ResolveApexBridgeBinary(), params, io.MultiWriter(os.Stdout, &outb)); err != nil {
		return "", err
	}

	output := outb.String()
	reTxHash := regexp.MustCompile(`Tx Hash\s*=\s*([^\s]+)`)

	if match := reTxHash.FindStringSubmatch(output); len(match) > 0 {
		ec.indexer.Add(match[1])

		return match[1], nil
	}

	return "", errors.New("tx hash not found in command output")
}

func (ec *TestEVMChain) DirectBridgingRequest(
	dstChainID uint8,
	privateKey string,
	receivers map[string]ReceiverAmount,
	feeAmount *big.Int,
	operationFee *big.Int,
	tokenContractAddrSrc string,
	isReactorBridging bool,
) (
	string, error,
) {
	contractAddress := types.StringToAddress(ec.gatewayAddr.String())

	// Build ReceiverWithdraw tuples from the receivers map
	type gatewayReceiverWithdraw struct {
		Receiver string   `abi:"receiver"`
		Amount   *big.Int `abi:"amount"`
		TokenID  uint16   `abi:"tokenId"`
	}

	gatewayReceivers := make([]gatewayReceiverWithdraw, 0, len(receivers))
	totalTokenAmount := big.NewInt(0)

	for addr, ra := range receivers {
		gatewayReceivers = append(gatewayReceivers, gatewayReceiverWithdraw{
			Receiver: addr,
			Amount:   ra.Amount,
			TokenID:  ra.TokenID,
		})

		totalTokenAmount.Add(totalTokenAmount, ra.Amount)
	}

	totalAmount := big.NewInt(0)
	totalAmount.Add(totalAmount, feeAmount)
	totalAmount.Add(totalAmount, operationFee)

	if isReactorBridging {
		totalAmount.Add(totalAmount, totalTokenAmount)
	}

	txRelayer, err := txrelayer.NewTxRelayer(
		txrelayer.WithIPAddress(ec.jsonRPCAddr),
		txrelayer.WithReceiptsTimeout(1*time.Minute),
		txrelayer.WithEstimateGasFallback(),
		txrelayer.WithWriter(os.Stdout),
	)
	if err != nil {
		return "", err
	}

	privateKeyECDSA, err := crypto.HexToECDSA(privateKey)
	if err != nil {
		return "", err
	}

	key := crypto.NewECDSAKey(privateKeyECDSA)

	if strings.Contains(tokenContractAddrSrc, "0x") {
		approveMethod, err := abi.NewMethod("function approve(address spender, uint256 value)")
		if err != nil {
			return "", err
		}

		// Approve the gateway / native token wallet to transfer tokens
		spenderAddr := ec.nativeTokenWalletAddr

		approveData, err := approveMethod.Encode([]interface{}{spenderAddr, totalTokenAmount})
		if err != nil {
			return "", fmt.Errorf("failed to encode approve call: %w", err)
		}

		tokenAddr := types.StringToAddress(tokenContractAddrSrc)

		receipt, err := txRelayer.SendTransaction(types.NewTx(types.NewLegacyTx(
			types.WithFrom(key.Address()),
			types.WithTo(&tokenAddr),
			types.WithInput(approveData),
		)), key)
		if err != nil {
			return "", fmt.Errorf("failed to send approve tx: %w", err)
		}

		if receipt.Status != uint64(types.ReceiptSuccess) {
			return "", fmt.Errorf("approve transaction receipt status is unsuccessful: %d", receipt.Status)
		}
	}

	// Encode and send Gateway.withdraw(chainID, receivers, feeAmount, operationFee)
	withdrawMethod, err := abi.NewMethod(
		"function withdraw(uint8 _destinationChainId, " +
			"(string receiver,uint256 amount,uint16 tokenId)[] _receivers, " +
			"uint256 _fee, uint256 _operationFee) payable",
	)
	if err != nil {
		return "", err
	}

	withdrawData, err := withdrawMethod.Encode([]interface{}{
		dstChainID,
		gatewayReceivers,
		feeAmount,
		operationFee,
	})
	if err != nil {
		return "", fmt.Errorf("failed to encode withdraw call: %w", err)
	}

	receipt, err := txRelayer.SendTransaction(types.NewTx(types.NewLegacyTx(
		types.WithFrom(key.Address()),
		types.WithTo(&contractAddress),
		types.WithValue(totalAmount),
		types.WithInput(withdrawData),
	)), key)
	if err != nil {
		return "", fmt.Errorf("failed to send withdraw tx: %w", err)
	}

	if receipt.Status != uint64(types.ReceiptSuccess) {
		return "", fmt.Errorf("transaction receipt status is unsuccessful: %d", receipt.Status)
	}

	return receipt.TransactionHash.String(), nil
}

func (ec *TestEVMChain) SendTx(
	ctx context.Context, privateKey string, metadata []byte, receivers []GenericTxReceiver, operationFee uint64,
) (string, error) {
	if ln := len(receivers); ln != 1 {
		return "", fmt.Errorf("evm SendTx currently supports only one receiver but got %d", ln)
	}

	opFee := new(big.Int).SetUint64(operationFee)

	rec, err := ec.sendTxWithNativeTokens(privateKey, receivers[0].Addr, receivers[0].Amount,
		metadata, receivers[0].NativeTokens, opFee)
	if err != nil {
		return "", err
	}

	return rec.TransactionHash.String(), nil
}

func (ec *TestEVMChain) GetHotWalletAddresses() []string {
	return []string{ec.gatewayAddr.String()}
}

func (ec *TestEVMChain) GetAdminPrivateKey() (string, error) {
	key, err := ec.admin.MarshallPrivateKey()
	if err != nil {
		return "", err
	}

	return hex.EncodeToString(key), nil
}

func (ec *TestEVMChain) GetIndexer() e2eindexer.TxsExecutedComponent {
	return ec.indexer
}

func (ec *TestEVMChain) sendTx(
	privateKey string, receiver string, amount *big.Int, data []byte,
) (*ethgo.Receipt, error) {
	privateKeyECDSA, err := crypto.HexToECDSA(privateKey)
	if err != nil {
		return nil, err
	}

	txRelayer, err := txrelayer.NewTxRelayer(
		txrelayer.WithIPAddress(ec.jsonRPCAddr),
		txrelayer.WithReceiptsTimeout(1*time.Minute),
		txrelayer.WithEstimateGasFallback(),
	)
	if err != nil {
		return nil, err
	}

	key := crypto.NewECDSAKey(privateKeyECDSA)
	receiverAddr := types.StringToAddress(receiver)

	receipt, err := txRelayer.SendTransaction(types.NewTx(types.NewLegacyTx(
		types.WithFrom(key.Address()),
		types.WithValue(amount),
		types.WithInput(data),
		types.WithTo(&receiverAddr),
	)), key)
	if err != nil {
		return nil, err
	} else if receipt.Status != uint64(types.ReceiptSuccess) {
		return nil, fmt.Errorf("fund relayer failed: %d", receipt.Status)
	}

	return receipt, nil
}

func (ec *TestEVMChain) sendTxWithNativeTokens(
	privateKey string, receiver string, amount *big.Int, data []byte,
	nativeTokens []GenericTokenAmount, operationFee *big.Int,
) (*ethgo.Receipt, error) {
	privateKeyECDSA, err := crypto.HexToECDSA(privateKey)
	if err != nil {
		return nil, err
	}

	txRelayer, err := txrelayer.NewTxRelayer(
		txrelayer.WithIPAddress(ec.jsonRPCAddr),
		txrelayer.WithReceiptsTimeout(1*time.Minute),
		txrelayer.WithEstimateGasFallback(),
	)
	if err != nil {
		return nil, err
	}

	key := crypto.NewECDSAKey(privateKeyECDSA)
	receiverAddr := types.StringToAddress(receiver)
	recipient := receiverAddr

	treasuryAddress := types.StringToAddress(ec.config.TreasuryAddress)

	for _, nativeToken := range nativeTokens {
		// We interpret the Cardano token PolicyID as the ERC20 contract address on the EVM chain.
		tokenAddr := types.StringToAddress(nativeToken.Token.PolicyID)

		// Encode ERC20 transfer(recipient, amount)
		if contractsapi.SimpleERC20 == nil || contractsapi.SimpleERC20.Abi == nil {
			return nil, fmt.Errorf("SimpleERC20 artifact not loaded")
		}

		transferMethod := contractsapi.SimpleERC20.Abi.Methods["transfer"]
		if transferMethod == nil {
			return nil, fmt.Errorf("transfer method not found in SimpleERC20 ABI")
		}

		transferData, err := transferMethod.Encode([]interface{}{recipient, nativeToken.Amount})
		if err != nil {
			return nil, fmt.Errorf("failed to encode transfer call: %w", err)
		}

		receipt, err := txRelayer.SendTransaction(types.NewTx(types.NewLegacyTx(
			types.WithFrom(key.Address()),
			types.WithTo(&tokenAddr),
			types.WithInput(transferData),
		)), key)
		if err != nil {
			return nil, fmt.Errorf("failed to send transfer tx for token %s: %w", nativeToken.Token.PolicyID, err)
		}

		if receipt.Status != uint64(types.ReceiptSuccess) {
			return nil, fmt.Errorf("token transfer for token %s failed with status: %d (tx: %s)", nativeToken.Token.PolicyID,
				receipt.Status, receipt.TransactionHash.String())
		}
	}

	receipt, err := txRelayer.SendTransaction(types.NewTx(types.NewLegacyTx(
		types.WithFrom(key.Address()),
		types.WithValue(amount),
		types.WithInput(data),
		types.WithTo(&receiverAddr),
	)), key)
	if err != nil {
		return nil, err
	} else if receipt.Status != uint64(types.ReceiptSuccess) {
		return nil, fmt.Errorf("currency transfer for chain %s failed: %d", ec.config.ChainID, receipt.Status)
	}

	if operationFee.Cmp(big.NewInt(0)) == 1 {
		opFeeReceipt, err := txRelayer.SendTransaction(types.NewTx(types.NewLegacyTx(
			types.WithFrom(key.Address()),
			types.WithValue(operationFee),
			types.WithInput(data),
			types.WithTo(&treasuryAddress),
		)), key)
		if err != nil {
			return nil, err
		}

		if opFeeReceipt.Status != uint64(types.ReceiptSuccess) {
			return nil, fmt.Errorf("operation fee transfer for chain %s failed: %d (tx: %s)", ec.config.ChainID,
				opFeeReceipt.Status, opFeeReceipt.TransactionHash.String())
		}
	}

	return receipt, nil
}

func (ec *TestEVMChain) getChainInfo(t *testing.T) EVMChainInfo {
	t.Helper()

	return EVMChainInfo{
		GatewayAddress: ec.gatewayAddr,
		JSONRPCAddr:    ec.jsonRPCAddr,
		RelayerAddress: ec.relayerWallet.Address(),
		AdminKey:       ec.admin,
		FundBlockNum:   ec.fundBlockNum,
	}
}

func (ec *TestEVMChain) GetAddressToBridgeTo(ctx context.Context, hasTokens bool) (string, error) {
	return ec.gatewayAddr.String(), nil
}
