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

	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/crypto"
	infracommon "github.com/Ethernal-Tech/cardano-infrastructure/common"
	infrawallet "github.com/Ethernal-Tech/cardano-infrastructure/wallet"
	"github.com/stretchr/testify/require"
)

const (
	defaultFundTokenAmount = uint64(100_000_000_000)
	defaultPremineAmount   = uint64(20_000_000_000)
)

type TestCardanoChainConfig struct {
	IsEnabled              bool
	ID                     int
	ChainID                string
	NetworkType            infrawallet.CardanoNetworkType
	NetworkMagic           uint
	NodesCount             int
	StartBlockHash         string
	StartSlot              uint64
	InitialHotWalletAmount *big.Int
	FundAmount             uint64
	FundFeeAmount          uint64
	FundUTxOCount          int
	FundFeeUTxOCount       int
	PreminesAddresses      []string
	PremineAmount          uint64
	SlotRoundingThreshold  uint64
	TTLInc                 uint64
	BridgeAddrHasStake     bool
	InitialUtxos           []CardanoChainConfigUtxo
}

func NewPrimeChainConfig() *TestCardanoChainConfig {
	return &TestCardanoChainConfig{
		IsEnabled:              true,
		ID:                     0,
		ChainID:                ChainIDPrime,
		NetworkType:            infrawallet.TestNetNetwork,
		NetworkMagic:           infrawallet.PrimeTestNetProtocolMagic,
		NodesCount:             4,
		StartBlockHash:         "0x0000000000000000000000000000000000000000000000000000000000000000",
		StartSlot:              0,
		InitialHotWalletAmount: big.NewInt(0),
		PremineAmount:          defaultPremineAmount,
		FundAmount:             defaultFundTokenAmount,
		FundFeeAmount:          defaultFundTokenAmount,
		FundUTxOCount:          1,
		FundFeeUTxOCount:       1,
		BridgeAddrHasStake:     true,
		InitialUtxos:           []CardanoChainConfigUtxo{},
	}
}

func NewVectorChainConfig(isEnabled bool) *TestCardanoChainConfig {
	return &TestCardanoChainConfig{
		IsEnabled:              isEnabled,
		ID:                     1,
		ChainID:                ChainIDVector,
		NetworkType:            infrawallet.TestNetNetwork,
		NetworkMagic:           infrawallet.VectorTestNetProtocolMagic,
		NodesCount:             4,
		StartBlockHash:         "0x0000000000000000000000000000000000000000000000000000000000000000",
		StartSlot:              0,
		InitialHotWalletAmount: big.NewInt(0),
		PremineAmount:          defaultPremineAmount,
		FundAmount:             defaultFundTokenAmount,
		FundFeeAmount:          defaultFundTokenAmount,
		FundUTxOCount:          1,
		FundFeeUTxOCount:       1,
		InitialUtxos:           []CardanoChainConfigUtxo{},
	}
}

func NewRemotePrimeChainConfig() *TestCardanoChainConfig {
	return &TestCardanoChainConfig{
		IsEnabled:    true,
		ID:           0,
		ChainID:      ChainIDPrime,
		NetworkType:  infrawallet.TestNetNetwork,
		NetworkMagic: infrawallet.PrimeTestNetProtocolMagic,
	}
}

func NewRemoteVectorChainConfig(isEnabled bool) *TestCardanoChainConfig {
	return &TestCardanoChainConfig{
		IsEnabled:    isEnabled,
		ID:           1,
		ChainID:      ChainIDVector,
		NetworkType:  infrawallet.TestNetNetwork,
		NetworkMagic: infrawallet.VectorTestNetProtocolMagic,
	}
}

type TestCardanoChain struct {
	config           *TestCardanoChainConfig
	cluster          *TestCardanoCluster
	ogmiosURL        string
	blockfrostURL    string
	blockfrostAPIKey string
	multisigAddr     string
	multisigFeeAddr  string
}

func (ec *TestCardanoChain) GetTxProvider() (infrawallet.ITxProvider, error) {
	if ec.ogmiosURL != "" {
		return infrawallet.NewTxProviderOgmios(ec.ogmiosURL), nil
	}

	if ec.blockfrostURL != "" && ec.blockfrostAPIKey != "" {
		return infrawallet.NewTxProviderBlockFrost(ec.blockfrostURL, ec.blockfrostAPIKey), nil
	}

	return nil, errors.New("neither a blockfrost nor a ogmios is specified")
}

var _ ITestApexChain = (*TestCardanoChain)(nil)

func NewTestCardanoChain(config *TestCardanoChainConfig) ITestApexChain {
	if !config.IsEnabled {
		getFlag := func(suffix string) string {
			return fmt.Sprintf("--%s-%s", config.ChainID, suffix)
		}

		return NewTestApexChainDummy([]string{
			getFlag("network-address"), "localhost:1000",
			getFlag("network-magic"), fmt.Sprint(config.NetworkMagic),
			getFlag("network-id"), fmt.Sprint(config.NetworkType),
			getFlag("ogmios-url"), "http://localhost:5500",
		})
	}

	return &TestCardanoChain{
		config: config,
	}
}

func (ec *TestCardanoChain) GetServerMust(t *testing.T, indx int) ITestApexChainServer {
	t.Helper()

	require.True(t, ec.cluster != nil && ec.cluster.Servers != nil && len(ec.cluster.Servers) > indx)

	return ec.cluster.Servers[indx]
}

func (ec *TestCardanoChain) RunChain(t *testing.T) error {
	t.Helper()

	networkName := ec.config.ChainID
	ogmiosLogsFilePath := filepath.Join("..", "..", "e2e-logs-cardano",
		fmt.Sprintf("ogmios-%s-%s.log", networkName, strings.ReplaceAll(t.Name(), "/", "_")))

	cluster, err := NewCardanoTestCluster(
		WithID(ec.config.ID+1),
		WithNodesCount(ec.config.NodesCount),
		WithStartTimeDelay(time.Second*5),
		WithPort(5100+ec.config.ID*100),
		WithOgmiosPort(1337+ec.config.ID),
		WithNetworkType(ec.config.NetworkType),
		WithNetworkMagic(ec.config.NetworkMagic),
		WithConfigGenesisDir(networkName),
		WithInitialFunds(ec.config.PreminesAddresses, ec.config.PremineAmount),
	)
	if err != nil {
		return err
	}

	fmt.Printf("Waiting for sockets to be ready %s (%d)\n", networkName, ec.config.ID)

	ec.cluster = cluster // at this point in time cluster has already been created

	if err := cluster.WaitForReady(time.Minute * 2); err != nil {
		return err
	}

	if err := cluster.StartOgmios(ec.config.ID, GetLogsFile(t, ogmiosLogsFilePath, false)); err != nil {
		return err
	}

	if err := cluster.WaitForBlockWithState(10, time.Second*120); err != nil {
		return err
	}

	ec.ogmiosURL = ec.cluster.OgmiosURL()

	fmt.Printf("Cluster %s (%d) is ready\n", networkName, ec.config.ID)

	return nil
}

func (ec *TestCardanoChain) Stop() error {
	if ec.cluster != nil {
		return ec.cluster.Stop()
	}

	return nil
}

func (ec *TestCardanoChain) CreateWallets(validator *TestApexValidator) error {
	walletType := ""
	if ec.config.BridgeAddrHasStake {
		walletType = "stake"
	}

	return validator.CardanoWalletCreate(ec.ChainID(), walletType)
}

func (ec *TestCardanoChain) CreateAddresses(
	bladeAdmin *crypto.ECDSAKey, bridgeURL string,
) error {
	bridgeAdminPk, err := bladeAdmin.MarshallPrivateKey()
	if err != nil {
		return err
	}

	args := []string{
		"create-address",
		"--network-id", fmt.Sprint(ec.config.NetworkType),
		"--testnet-magic", fmt.Sprint(ec.config.NetworkMagic),
		"--bridge-url", bridgeURL,
		"--bridge-addr", contracts.Bridge.String(),
		"--bridge-key", hex.EncodeToString(bridgeAdminPk),
		"--chain", ec.config.ChainID,
	}

	var outb bytes.Buffer

	err = RunCommand(ResolveApexBridgeBinary(), args, io.MultiWriter(os.Stdout, &outb))
	if err != nil {
		return err
	}

	output := outb.String()
	reMultisig := regexp.MustCompile(`Multisig Address\s*=\s*([^\s]+)`)
	reFee := regexp.MustCompile(`Fee Payer Address\s*=\s*([^\s]+)`)

	if match := reMultisig.FindStringSubmatch(output); len(match) > 0 {
		ec.multisigAddr = match[1]
	}

	if match := reFee.FindStringSubmatch(output); len(match) > 0 {
		ec.multisigFeeAddr = match[1]
	}

	return nil
}

func (ec *TestCardanoChain) FundWallets(ctx context.Context) error {
	privateKey, err := ec.GetAdminPrivateKey()
	if err != nil {
		return err
	}

	if totalAmount := ec.config.FundFeeAmount; totalAmount != 0 {
		for _, amount := range SplitAmountNTimes(new(big.Int).SetUint64(totalAmount), ec.config.FundFeeUTxOCount) {
			txHash, err := ec.SendTx(ctx, privateKey, ec.multisigFeeAddr, amount, nil)
			if err != nil {
				return err
			}

			fmt.Printf("%s fee addr funded with %s: %s\n", ec.config.ChainID, amount, txHash)
		}
	}

	if totalAmount := ec.config.FundAmount; totalAmount != 0 {
		for _, amount := range SplitAmountNTimes(new(big.Int).SetUint64(totalAmount), ec.config.FundUTxOCount) {
			txHash, err := ec.SendTx(ctx, privateKey, ec.multisigAddr, amount, nil)
			if err != nil {
				return err
			}

			fmt.Printf("%s addr funded with %s: %s\n", ec.config.ChainID, amount, txHash)
		}
	}

	return nil
}

func (ec *TestCardanoChain) InitContracts(_ context.Context, _ *crypto.ECDSAKey, _ string) error {
	return nil
}

func (ec *TestCardanoChain) RegisterChain(validator *TestApexValidator) error {
	return validator.RegisterChain(ec.ChainID(), ec.config.InitialHotWalletAmount, ChainTypeCardano)
}

func (ec *TestCardanoChain) GetGenerateConfigsParams(indx int) (result []string) {
	getFlag := func(suffix string) string {
		return fmt.Sprintf("--%s-%s", ec.ChainID(), suffix)
	}

	server := ec.cluster.Servers[indx%len(ec.cluster.Servers)]
	result = []string{
		getFlag("network-address"), server.NetworkAddress(),
		getFlag("network-magic"), fmt.Sprint(ec.config.NetworkMagic),
		getFlag("network-id"), fmt.Sprint(ec.config.NetworkType),
		getFlag("ogmios-url"), ec.ogmiosURL,
	}

	if ec.config.TTLInc > 0 {
		result = append(result, getFlag("ttl-slot-inc"), fmt.Sprint(ec.config.TTLInc))
	}

	if ec.config.SlotRoundingThreshold > 0 {
		result = append(result, getFlag("slot-rounding-threshold"), fmt.Sprint(ec.config.SlotRoundingThreshold))
	}

	return result
}

func (ec *TestCardanoChain) PopulateApexSystem(apexSystem *ApexSystem) {
	chainInfo := CardanoChainInfo{
		NetworkAddress: ec.cluster.Servers[0].NetworkAddress(),
		OgmiosURL:      ec.ogmiosURL,
		MultisigAddr:   ec.multisigAddr,
		FeeAddr:        ec.multisigFeeAddr,
		SocketPath:     ec.cluster.OgmiosServer.SocketPath(),
	}

	switch ec.ChainID() {
	case ChainIDPrime:
		apexSystem.PrimeInfo = chainInfo
	case ChainIDVector:
		apexSystem.VectorInfo = chainInfo
	}
}

func (ec *TestCardanoChain) ChainID() string {
	return ec.config.ChainID
}

func (ec *TestCardanoChain) GetAddressBalance(ctx context.Context, addr string) (*big.Int, error) {
	txProvider, err := ec.GetTxProvider()
	if err != nil {
		return nil, err
	}

	utxos, err := txProvider.GetUtxos(ctx, addr)
	if err != nil {
		return nil, err
	}

	sum := infrawallet.GetUtxosSum(utxos)

	return new(big.Int).SetUint64(sum[infrawallet.AdaTokenName]), nil
}

func (ec *TestCardanoChain) BridgingRequest(
	ctx context.Context, destChainID ChainID, privateKey string, receivers map[string]*big.Int, feeAmount *big.Int,
) (string, error) {
	paymentKey, stakeKey, err := FromCardanoPrivateKeyString(privateKey)
	if err != nil {
		return "", err
	}

	wallet := infrawallet.NewWallet(paymentKey, stakeKey)

	caddr, err := GetAddress(ec.config.NetworkType, wallet)
	if err != nil {
		return "", err
	}

	senderAddr := caddr.String()

	totalAmount := new(big.Int).Set(feeAmount)
	receiversMap := make(map[string]uint64, len(receivers))

	for addr, amount := range receivers {
		totalAmount.Add(totalAmount, amount)
		receiversMap[addr] = amount.Uint64()
	}

	bridgingRequestMetadata, err := CreateCardanoBridgingMetaData(
		senderAddr, receiversMap, destChainID, feeAmount.Uint64())
	if err != nil {
		return "", err
	}

	return ec.SendTx(ctx, privateKey, ec.multisigAddr, totalAmount, bridgingRequestMetadata)
}

func (ec *TestCardanoChain) SendTx(
	ctx context.Context, privateKey string, receiverAddr string, amount *big.Int, data []byte,
) (string, error) {
	const (
		retryCount    = 90
		retryWaitTime = time.Second * 2
	)

	paymentKey, stakeKey, err := FromCardanoPrivateKeyString(privateKey)
	if err != nil {
		return "", err
	}

	wallet := infrawallet.NewWallet(paymentKey, stakeKey)

	txProvider, err := ec.GetTxProvider()
	if err != nil {
		return "", err
	}

	txHash, err := SendTx(ctx, txProvider, wallet,
		amount.Uint64(), receiverAddr, ec.config.NetworkType, ec.config.NetworkMagic, data)
	if err != nil {
		return "", err
	}

	_, err = infracommon.ExecuteWithRetry(ctx, func(ctx context.Context) (string, error) {
		contains, err := infrawallet.IsTxInUtxos(ctx, txProvider, receiverAddr, txHash)
		if err != nil {
			return "", err
		} else if !contains {
			return "", infracommon.ErrRetryTryAgain
		}

		return txHash, nil
	}, infracommon.WithRetryCount(retryCount), infracommon.WithRetryWaitTime(retryWaitTime))
	if err != nil {
		return "", fmt.Errorf("failed to send tx %s to receiver %s: %w", txHash, receiverAddr, err)
	}

	return txHash, nil
}

func (ec *TestCardanoChain) GetHotWalletAddress() string {
	return ec.multisigAddr
}

func (ec *TestCardanoChain) GetAdminPrivateKey() (string, error) {
	genesisWallet, err := GetGenesisWalletFromCluster(ec.cluster.Config.TmpDir, 1)
	if err != nil {
		return "", err
	}

	return hex.EncodeToString(genesisWallet.SigningKey), nil
}
