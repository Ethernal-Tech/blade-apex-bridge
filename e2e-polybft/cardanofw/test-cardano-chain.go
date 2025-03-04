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
	"github.com/Ethernal-Tech/cardano-infrastructure/sendtx"
	infrawallet "github.com/Ethernal-Tech/cardano-infrastructure/wallet"
	"github.com/stretchr/testify/require"
)

const (
	defaultFundTokenAmount   = uint64(100_000_000_000)
	defaultPremineAmount     = uint64(20_000_000_000)
	defaultNativeTokenAmount = uint64(0)
)

type ChainType string

type TestCardanoChainConfig struct {
	IsEnabled                   bool
	ID                          int
	NetworkType                 infrawallet.CardanoNetworkType
	NodesCount                  int
	InitialHotWalletAmount      *big.Int
	InitialHotWalletTokenAmount *big.Int
	ChainType                   ChainType
	FundAmount                  uint64
	FundFeeAmount               uint64
	FundTokenAmount             uint64
	PreminesAddresses           []string
	PremineAmount               uint64
	SlotRoundingThreshold       uint64
	TTLInc                      uint64
	MinBridgingFee              uint64
	MinOperationFee             uint64
	NativeTokens                []sendtx.TokenExchangeConfig
}

func NewPrimeChainConfig() *TestCardanoChainConfig {
	return &TestCardanoChainConfig{
		IsEnabled:                   true,
		ID:                          0,
		NetworkType:                 infrawallet.TestNetNetwork,
		ChainType:                   ChainType(ChainIDPrime),
		NodesCount:                  4,
		InitialHotWalletAmount:      big.NewInt(0),
		InitialHotWalletTokenAmount: big.NewInt(0),
		PremineAmount:               defaultPremineAmount,
		FundAmount:                  defaultFundTokenAmount,
		FundFeeAmount:               defaultFundTokenAmount,
		FundTokenAmount:             defaultNativeTokenAmount,
		MinBridgingFee:              defaultMinBridgingFeeAmount,
		MinOperationFee:             uint64(0),
	}
}

func NewVectorChainConfig(isEnabled bool) *TestCardanoChainConfig {
	return &TestCardanoChainConfig{
		IsEnabled:                   isEnabled,
		ID:                          1,
		NetworkType:                 infrawallet.VectorTestNetNetwork,
		ChainType:                   ChainType(ChainIDVector),
		NodesCount:                  4,
		InitialHotWalletAmount:      big.NewInt(0),
		InitialHotWalletTokenAmount: big.NewInt(0),
		PremineAmount:               defaultPremineAmount,
		FundAmount:                  defaultFundTokenAmount,
		FundFeeAmount:               defaultFundTokenAmount,
		FundTokenAmount:             defaultNativeTokenAmount,
		MinBridgingFee:              defaultMinBridgingFeeAmount,
		MinOperationFee:             uint64(0),
	}
}

func NewCardanoChainConfig(isEnabled bool) *TestCardanoChainConfig {
	return &TestCardanoChainConfig{
		IsEnabled:                   isEnabled,
		ID:                          4,
		NetworkType:                 infrawallet.TestNetNetwork,
		ChainType:                   ChainType(ChainIDCardano),
		NodesCount:                  4,
		InitialHotWalletAmount:      big.NewInt(0),
		InitialHotWalletTokenAmount: big.NewInt(0),
		PremineAmount:               defaultPremineAmount,
		FundAmount:                  defaultFundTokenAmount,
		FundFeeAmount:               defaultFundTokenAmount,
		FundTokenAmount:             defaultNativeTokenAmount,
		MinBridgingFee:              defaultMinBridgingFeeAmount,
		MinOperationFee:             DefaultMinOperationFee,
	}
}

func NewRemotePrimeChainConfig() *TestCardanoChainConfig {
	return &TestCardanoChainConfig{
		IsEnabled:   true,
		ID:          0,
		NetworkType: infrawallet.TestNetNetwork,
	}
}

func NewRemoteVectorChainConfig(isEnabled bool) *TestCardanoChainConfig {
	return &TestCardanoChainConfig{
		IsEnabled:   isEnabled,
		ID:          1,
		NetworkType: infrawallet.VectorTestNetNetwork,
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
	fundBlockSlot    uint64
	fundBlockHash    string
	txSender         *sendtx.TxSender
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
			return fmt.Sprintf("--%s-%s", GetNetworkName(config), suffix)
		}

		return NewTestApexChainDummy([]string{
			getFlag("network-address"), "localhost:1000",
			getFlag("network-magic"), fmt.Sprint(GetNetworkMagic(config.NetworkType)),
			getFlag("network-id"), fmt.Sprint(config.NetworkType),
			getFlag("ogmios-url"), "http://localhost:5500",
		})
	}

	return &TestCardanoChain{
		config: config,
	}
}

func (ec *TestCardanoChain) RunChain(t *testing.T) error {
	t.Helper()

	networkName := GetGenesisType(ec.config)
	ogmiosLogsFilePath := filepath.Join("..", "..", "e2e-logs-cardano",
		fmt.Sprintf("ogmios-%s-%s.log", networkName, strings.ReplaceAll(t.Name(), "/", "_")))

	cluster, err := NewCardanoTestCluster(
		WithID(ec.config.ID+1),
		WithNodesCount(ec.config.NodesCount),
		WithStartTimeDelay(time.Second*5),
		WithPort(5100+ec.config.ID*100),
		WithOgmiosPort(1337+ec.config.ID),
		WithNetworkType(ec.config.NetworkType),
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
	return validator.CardanoWalletCreate(GetNetworkName(ec.config))
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
		"--bridge-url", bridgeURL,
		"--bridge-addr", contracts.Bridge.String(),
		"--bridge-key", hex.EncodeToString(bridgeAdminPk),
		"--chain", GetNetworkName(ec.config),
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

	if ec.config.FundFeeAmount != 0 {
		txHash, err := ec.SendTx(
			ctx, privateKey, ec.multisigFeeAddr, new(big.Int).SetUint64(ec.config.FundFeeAmount), nil)
		if err != nil {
			return err
		}

		fmt.Printf("%s fee addr funded: %s\n", GetNetworkName(ec.config), txHash)
	}

	if ec.config.FundTokenAmount != 0 || ec.config.FundAmount != 0 {
		minterWallet, err := GetGenesisWalletFromCluster(ec.cluster.Config.TmpDir, 1)
		if err != nil {
			return err
		}

		tokenAmount, err := FundAddressWithToken(
			ctx, ec.ChainID(), ec.config.NetworkType, infrawallet.NewTxProviderOgmios(ec.ogmiosURL),
			minterWallet, ec.GetHotWalletAddress(), max(2*MinUTxODefaultValue, ec.config.FundAmount), ec.config.FundTokenAmount)
		if err != nil {
			return err
		}

		fmt.Printf("%s multisig addr funded with native currency: %+v and tokens: %+v\n", GetNetworkName(ec.config),
			ec.config.FundAmount, tokenAmount)
	}

	txProvider, err := ec.GetTxProvider()
	if err != nil {
		return err
	}

	// retrieve latest tip
	tip, err := txProvider.GetTip(ctx)
	if err != nil {
		return err
	}

	ec.fundBlockHash = tip.Hash
	ec.fundBlockSlot = tip.Slot

	return nil
}

func (ec *TestCardanoChain) InitContracts(bridgeAdmin *crypto.ECDSAKey, bridgeURL string) error {
	return nil
}

func (ec *TestCardanoChain) RegisterChain(validator *TestApexValidator) error {
	return validator.RegisterChain(ec.ChainID(), ec.config.InitialHotWalletAmount, ec.config.InitialHotWalletTokenAmount,
		ChainTypeCardano)
}

func (ec *TestCardanoChain) GetGenerateConfigsParams(indx int) (result []string) {
	getFlag := func(suffix string) string {
		return fmt.Sprintf("--%s-%s", ec.ChainID(), suffix)
	}

	server := ec.cluster.Servers[indx%len(ec.cluster.Servers)]
	result = []string{
		getFlag("network-address"), server.NetworkAddress(),
		getFlag("network-magic"), fmt.Sprint(GetNetworkMagic(ec.config.NetworkType)),
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

func (ec *TestCardanoChain) PopulateApexSystem(t *testing.T, apexSystem *ApexSystem) {
	t.Helper()

	genesisWallet, err := GetGenesisWalletFromCluster(ec.cluster.Config.TmpDir, 1)
	require.NoError(t, err)

	chainInfo := CardanoChainInfo{
		NetworkAddress: ec.cluster.Servers[0].NetworkAddress(),
		OgmiosURL:      ec.ogmiosURL,
		MultisigAddr:   ec.multisigAddr,
		FeeAddr:        ec.multisigFeeAddr,
		SocketPath:     ec.cluster.OgmiosServer.SocketPath(),
		FundBlockHash:  ec.fundBlockHash,
		FundBlockSlot:  ec.fundBlockSlot,
		GenesisWallet:  genesisWallet,
	}

	switch ec.ChainID() {
	case ChainIDPrime:
		apexSystem.PrimeInfo = chainInfo
	case ChainIDVector:
		apexSystem.VectorInfo = chainInfo
	case ChainIDCardano:
		apexSystem.CardanoInfo = chainInfo
	}
}

func (ec *TestCardanoChain) UpdateTxSendChainConfiguration(configs map[string]sendtx.ChainConfig) {
	ec.txSender = sendtx.NewTxSender(configs)
}

func (ec *TestCardanoChain) ChainID() string {
	return GetNetworkName(ec.config)
}

func (ec *TestCardanoChain) GetAddressBalance(ctx context.Context, addr string) (map[string]uint64, error) {
	txProvider, err := ec.GetTxProvider()
	if err != nil {
		return nil, err
	}

	utxos, err := txProvider.GetUtxos(ctx, addr)
	if err != nil {
		return nil, err
	}

	return infrawallet.GetUtxosSum(utxos), nil
}

func (ec *TestCardanoChain) CreateMetadata(
	context context.Context,
	senderAddr string,
	dstChainID string,
	receivers []sendtx.BridgingTxReceiver,
	bridgingFee uint64,
	operationFee uint64,
) ([]byte, error) {
	metadata, err := ec.txSender.CreateMetadata(
		context, senderAddr, GetNetworkName(ec.config), dstChainID, receivers, bridgingFee, operationFee)
	if err != nil {
		return nil, err
	}

	return metadata.Marshal()
}

func (ec *TestCardanoChain) BridgingRequest(
	ctx context.Context,
	dstChainID ChainID,
	privateKey string,
	receiversMap map[string]*big.Int,
	feeAmount *big.Int,
	operationFee uint64,
	bridgingTypes ...sendtx.BridgingType,
) (string, error) {
	paymentKey, stakeKey, err := FromCardanoPrivateKeyString(privateKey)
	if err != nil {
		return "", err
	}

	wallet := infrawallet.NewWallet(paymentKey, stakeKey)
	srcChainID := GetNetworkName(ec.config)

	walletAddr, err := GetAddress(ec.config.NetworkType, wallet)
	if err != nil {
		return "", err
	}

	receivers := make([]sendtx.BridgingTxReceiver, 0, len(receiversMap))

	for receiverAddress, receiverAmount := range receiversMap {
		bridgingType := sendtx.BridgingTypeNormal
		if idx := len(receivers); idx < len(bridgingTypes) {
			bridgingType = bridgingTypes[idx]
		}

		receivers = append(receivers, sendtx.BridgingTxReceiver{
			Addr:         receiverAddress,
			Amount:       DfmToChainNativeTokenAmount(srcChainID, receiverAmount).Uint64(),
			BridgingType: bridgingType,
		})
	}

	rawTx, txHash, _, err := ec.txSender.CreateBridgingTx(
		ctx,
		srcChainID,
		dstChainID,
		walletAddr.String(),
		receivers,
		bridgingFeeAmount,
		operationFee,
	)
	if err != nil {
		return "", err
	}

	return ec.submitTx(ctx, rawTx, txHash, ec.multisigAddr, wallet)
}

func (ec *TestCardanoChain) SendTx(
	ctx context.Context,
	privateKey string,
	receiverAddr string,
	amount *big.Int,
	metadata []byte,
) (string, error) {
	paymentKey, stakeKey, err := FromCardanoPrivateKeyString(privateKey)
	if err != nil {
		return "", err
	}

	wallet := infrawallet.NewWallet(paymentKey, stakeKey)

	walletAddr, err := GetAddress(ec.config.NetworkType, wallet)
	if err != nil {
		return "", err
	}

	rawTx, txHash, err := ec.txSender.CreateTxGeneric(
		ctx,
		GetNetworkName(ec.config),
		walletAddr.String(),
		receiverAddr,
		metadata,
		amount.Uint64(),
		0,
		"",
	)
	if err != nil {
		return "", err
	}

	_, err = ec.submitTx(ctx, rawTx, txHash, receiverAddr, wallet)
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

func (ec *TestCardanoChain) submitTx(
	ctx context.Context,
	rawTx []byte,
	txHash string,
	receiverAddr string,
	signer infrawallet.ITxSigner,
) (string, error) {
	const (
		retryCount    = 40
		retryWaitTime = time.Second * 5
	)

	txProvider := infrawallet.NewTxProviderOgmios(ec.ogmiosURL)

	if err := ec.txSender.SubmitTx(ctx, GetNetworkName(ec.config), rawTx, signer); err != nil {
		return "", err
	}

	return infracommon.ExecuteWithRetry(ctx, func(ctx context.Context) (string, error) {
		contains, err := infrawallet.IsTxInUtxos(ctx, txProvider, receiverAddr, txHash)
		if err != nil {
			return "", err
		} else if !contains {
			return "", infracommon.ErrRetryTryAgain
		}

		return txHash, nil
	}, infracommon.WithRetryCount(retryCount), infracommon.WithRetryWaitTime(retryWaitTime))
}
