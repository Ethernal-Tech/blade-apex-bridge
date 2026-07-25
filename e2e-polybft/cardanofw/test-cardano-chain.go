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
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/e2eindexer"
	infracommon "github.com/Ethernal-Tech/cardano-infrastructure/common"
	"github.com/Ethernal-Tech/cardano-infrastructure/indexer"
	"github.com/Ethernal-Tech/cardano-infrastructure/indexer/gouroboros"
	"github.com/Ethernal-Tech/cardano-infrastructure/sendtx"
	infrawallet "github.com/Ethernal-Tech/cardano-infrastructure/wallet"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"
)

const (
	defaultFundTokenAmount   = uint64(100_000_000_000)
	defaultPremineAmount     = uint64(20_000_000_000)
	defaultNativeTokenAmount = uint64(0)
)

type TestCardanoChainConfig struct {
	IsEnabled              bool
	ID                     int
	ChainID                string
	NetworkType            infrawallet.CardanoNetworkType
	NetworkMagic           uint
	NodesCount             int
	IndexerStartBlockHash  indexer.Hash
	IndexerStartSlot       uint64
	InitialHotWalletAmount *big.Int
	FundAmount             uint64
	FundFeeAmount          uint64
	FundTokenAmount        uint64
	FundUTxOCount          int
	FundFeeUTxOCount       int
	PreminesAddresses      []string
	PremineAmount          uint64
	SlotRoundingThreshold  uint64
	TTLInc                 uint64
	MinBridgingFee         uint64
	BridgeAddrHasStake     bool
	UseIndexer             bool
	AllowedDirections      []ChainID
}

func NewPrimeChainConfig() *TestCardanoChainConfig {
	return &TestCardanoChainConfig{
		IsEnabled:              true,
		ID:                     0,
		ChainID:                ChainIDPrime,
		NetworkType:            infrawallet.TestNetNetwork,
		NetworkMagic:           infrawallet.PrimeTestNetProtocolMagic,
		NodesCount:             4,
		InitialHotWalletAmount: big.NewInt(0),
		PremineAmount:          defaultPremineAmount,
		FundAmount:             defaultFundTokenAmount,
		FundFeeAmount:          defaultFundTokenAmount,
		FundTokenAmount:        defaultNativeTokenAmount,
		FundUTxOCount:          1,
		FundFeeUTxOCount:       1,
		MinBridgingFee:         defaultMinBridgingFeeAmount,
		BridgeAddrHasStake:     true,
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
		InitialHotWalletAmount: big.NewInt(0),
		PremineAmount:          defaultPremineAmount,
		FundAmount:             defaultFundTokenAmount,
		FundFeeAmount:          defaultFundTokenAmount,
		FundTokenAmount:        defaultNativeTokenAmount,
		FundUTxOCount:          1,
		FundFeeUTxOCount:       1,
		MinBridgingFee:         defaultMinBridgingFeeAmount,
	}
}

func NewRemotePrimeChainConfig(minBridgingFeeAmount uint64) *TestCardanoChainConfig {
	return &TestCardanoChainConfig{
		IsEnabled:      true,
		ID:             0,
		ChainID:        ChainIDPrime,
		NetworkType:    infrawallet.TestNetNetwork,
		NetworkMagic:   infrawallet.PrimeTestNetProtocolMagic,
		MinBridgingFee: minBridgingFeeAmount,
	}
}

func NewRemoteVectorChainConfig(minBridgingFeeAmount uint64) *TestCardanoChainConfig {
	return &TestCardanoChainConfig{
		IsEnabled:      true,
		ID:             1,
		ChainID:        ChainIDVector,
		NetworkType:    infrawallet.MainNetNetwork,
		NetworkMagic:   infrawallet.MainNetProtocolMagic,
		MinBridgingFee: minBridgingFeeAmount,
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
	txSender         *sendtx.TxSender
	indexer          e2eindexer.TxsExecutedComponent
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
		config:  config,
		indexer: e2eindexer.NewTxsExecutedComponentDummy(),
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
		WithStdOutWritterFactory(func(id int, instanceType, dir string) io.Writer {
			if instanceType == cardanoNode {
				return nil
			}

			return GetLogsFile(t, filepath.Join(dir, fmt.Sprintf("%s-%d.log", instanceType, id)), false)
		}),
	)
	if err != nil {
		return err
	}

	fmt.Printf("Waiting for sockets to be ready %s (%d)\n", networkName, ec.config.ID)

	ec.cluster = cluster // at this point in time cluster has already been created

	if err := cluster.WaitForReady(time.Minute * 2); err != nil {
		return err
	}

	if err := cluster.StartOgmios(ec.config.ID); err != nil {
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
	receivers := []GenericTxReceiver(nil)
	outputInfo := []string(nil)

	minterWallet, err := GetGenesisWalletFromCluster(ec.cluster.Config.TmpDir, 1)
	if err != nil {
		return err
	}

	if totalAmount := ec.config.FundFeeAmount; totalAmount != 0 {
		amount := new(big.Int).SetUint64(totalAmount)

		if utxoCount := ec.config.FundFeeUTxOCount; utxoCount >= 2 {
			firstAmount, lastAmount := SplitAmountNTimes(amount, utxoCount)

			for range utxoCount - 1 {
				receivers = append(receivers, createTxReceiver(ec.multisigFeeAddr, firstAmount, nil, nil))
			}

			receivers = append(receivers, createTxReceiver(ec.multisigFeeAddr, lastAmount, nil, nil))
		} else {
			receivers = append(receivers, createTxReceiver(ec.multisigFeeAddr, amount, nil, nil))
		}

		outputInfo = append(outputInfo, fmt.Sprintf("fee (%d): %s", max(ec.config.FundFeeUTxOCount, 1), amount))
	}

	if ec.config.FundTokenAmount != 0 || ec.config.FundAmount != 0 {
		addr := ec.multisigAddr
		amount := new(big.Int).SetUint64(max(2*MinUTxODefaultValue, ec.config.FundAmount))
		tokenAmount := new(big.Int).SetUint64(ec.config.FundTokenAmount)

		token, _, err := GetTokenAndPolicyForVerificationKey(
			ec.ChainID(), ec.config.NetworkType, minterWallet.VerificationKey, DefaultTokenName)
		if err != nil {
			return err
		}

		if ta := ec.config.FundTokenAmount; ta != 0 {
			if err := MintToken(ec, minterWallet, DefaultTokenName, ta); err != nil {
				return err
			}
		}

		if utxoCount := ec.config.FundUTxOCount; utxoCount >= 2 {
			firstAmount, lastAmount := SplitAmountNTimes(amount, utxoCount)
			firstTokenAmount, lastTokenAmount := SplitAmountNTimes(tokenAmount, utxoCount)

			for range utxoCount - 1 {
				receivers = append(receivers, createTxReceiver(addr, firstAmount, &token, firstTokenAmount))
			}

			receivers = append(receivers, createTxReceiver(addr, lastAmount, &token, lastTokenAmount))
		} else {
			receivers = append(receivers, createTxReceiver(addr, amount, &token, tokenAmount))
		}

		outputInfo = append(outputInfo, fmt.Sprintf("multisig with currency and `%s` (%d): %s, %s",
			token, max(ec.config.FundUTxOCount, 1), amount, tokenAmount))
	}

	if len(receivers) == 0 {
		return nil
	}

	txHash, err := ec.SendTx(
		ctx, ToCardanoPrivateKeyString(minterWallet.SigningKey, minterWallet.StakeSigningKey), nil, receivers)
	if err != nil {
		return err
	}

	fmt.Printf("%s fund transaction: %s\n%s\n", ec.ChainID(), txHash, strings.Join(outputInfo, "\n"))

	return nil
}

func (ec *TestCardanoChain) InitContracts(_ context.Context, _ *crypto.ECDSAKey, _ string) error {
	return nil
}

func (ec *TestCardanoChain) RegisterChain(validator *TestApexValidator) error {
	return validator.RegisterChain(ec.ChainID(), ec.config.InitialHotWalletAmount, ChainTypeCardano)
}

func (ec *TestCardanoChain) GenerateChainConfigs(
	indx int,
	validator *TestApexValidator,
) error {
	server := ec.cluster.Servers[indx%len(ec.cluster.Servers)]
	dbsPath := filepath.Join(validator.dataDirPath, BridgingDBsDir)

	args := []string{
		"generate-configs", "cardano-chain",
		"--chain-id", ec.ChainID(),
		"--network-address", server.NetworkAddress(),
		"--network-magic", fmt.Sprint(ec.config.NetworkMagic),
		"--network-id", fmt.Sprint(ec.config.NetworkType),
		"--ogmios-url", ec.ogmiosURL,
		"--utxo-min-amount", strconv.FormatUint(MinUTxODefaultValue, 10),
		"--output-dir", validator.GetBridgingConfigsDir(),
		"--output-validator-components-file-name", ValidatorComponentsConfigFileName,
		"--output-relayer-file-name", RelayerConfigFileName,
		"--dbs-path", dbsPath,
	}

	for _, direction := range ec.config.AllowedDirections {
		args = append(args, "--allowed-directions", direction)
	}

	if ec.config.TTLInc > 0 {
		args = append(args, "--ttl-slot-inc", fmt.Sprint(ec.config.TTLInc))
	}

	if ec.config.SlotRoundingThreshold > 0 {
		args = append(args, "--slot-rounding-threshold", fmt.Sprint(ec.config.SlotRoundingThreshold))
	}

	return RunCommand(ResolveApexBridgeBinary(), args, os.Stdout)
}

func (ec *TestCardanoChain) PopulateApexSystem(apexSystem *ApexSystem) error {
	switch ec.ChainID() {
	case ChainIDPrime:
		apexSystem.PrimeInfo = ec.getChainInfo()
	case ChainIDVector:
		apexSystem.VectorInfo = ec.getChainInfo()
	}

	if ec.config.UseIndexer {
		indexer, err := ec.createIndexer()
		if err != nil {
			return err
		}

		ec.indexer = indexer
	}

	return nil
}

func (ec *TestCardanoChain) UpdateTxSendChainConfiguration(configs map[string]sendtx.ChainConfig) {
	ec.txSender = sendtx.NewTxSender(configs)
}

func (ec *TestCardanoChain) ChainID() string {
	return ec.config.ChainID
}

func (ec *TestCardanoChain) GetAddressBalance(ctx context.Context, addr string) (*big.Int, error) {
	txProvider, err := ec.GetTxProvider()
	if err != nil {
		return nil, err
	}

	utxos, err := infracommon.ExecuteWithRetry(
		ctx, func(ctx context.Context) ([]infrawallet.Utxo, error) {
			return txProvider.GetUtxos(ctx, addr)
		},
	)
	if err != nil {
		return nil, err
	}

	sum := infrawallet.GetUtxosSum(utxos)

	return new(big.Int).SetUint64(sum[infrawallet.AdaTokenName]), nil
}

func (ec *TestCardanoChain) GetBridgingFee(
	ctx context.Context,
	dstChainID string,
	receivers []sendtx.BridgingTxReceiver,
	bridgingFee uint64,
	multiSigAddr string,
) (uint64, error) {
	return ec.txSender.GetBridgingFee(
		ctx,
		sendtx.BridgingTxDto{
			SrcChainID:      ec.ChainID(),
			DstChainID:      dstChainID,
			Receivers:       receivers,
			BridgingAddress: multiSigAddr,
			BridgingFee:     bridgingFee,
		})
}

func (ec *TestCardanoChain) CreateMetadata(
	senderAddr string,
	dstChainID string,
	receivers []sendtx.BridgingTxReceiver,
	bridgingFee uint64,
) ([]byte, error) {
	metadata, err := ec.txSender.CreateMetadata(
		senderAddr, ec.ChainID(), dstChainID, receivers, bridgingFee)
	if err != nil {
		return nil, err
	}

	return metadata.Marshal()
}

func (ec *TestCardanoChain) BridgingRequest(
	ctx context.Context, destChainID ChainID, privateKey string, receiversMap map[string]*big.Int, feeAmount *big.Int,
) (string, error) {
	wallets, policyScript, senderAddr, err := FromCardanoPrivateKeyString(
		privateKey, ec.config.NetworkType, ec.config.NetworkMagic)
	if err != nil {
		return "", err
	}

	receivers := make([]sendtx.BridgingTxReceiver, 0, len(receiversMap))

	for receiverAddress, receiverAmount := range receiversMap {
		receivers = append(receivers, sendtx.BridgingTxReceiver{
			Addr:   receiverAddress,
			Amount: DfmToChainNativeTokenAmount(ec.ChainID(), receiverAmount).Uint64(),
		})
	}

	txInfo, _, err := ec.txSender.CreateBridgingTx(
		ctx,
		sendtx.BridgingTxDto{
			SrcChainID:             ec.ChainID(),
			DstChainID:             destChainID,
			SenderAddr:             senderAddr,
			SenderAddrPolicyScript: policyScript,
			Receivers:              receivers,
			BridgingAddress:        ec.multisigAddr,
			BridgingFee:            feeAmount.Uint64(),
		})
	if err != nil {
		return "", err
	}

	if ec.indexer != nil {
		ec.indexer.Add(txInfo.TxHash)
	}

	return ec.submitTx(ctx, txInfo.TxRaw, txInfo.TxHash, ec.multisigAddr, wallets)
}

func (ec *TestCardanoChain) SendTx(
	ctx context.Context, privateKey string, metadata []byte, receivers []GenericTxReceiver,
) (string, error) {
	if len(receivers) == 0 {
		return "", fmt.Errorf("cardano SendTx supports one or multiple receivers but got zero")
	}

	wallets, policyScript, senderAddr, err := FromCardanoPrivateKeyString(
		privateKey, ec.config.NetworkType, ec.config.NetworkMagic)
	if err != nil {
		return "", err
	}

	receiversDto := make([]sendtx.TxReceiversDto, len(receivers))
	for i, r := range receivers {
		receiversDto[i] = sendtx.TxReceiversDto{
			Addr:         r.Addr,
			Amount:       r.Amount.Uint64(),
			NativeTokens: r.NativeTokens,
		}
	}

	txInfo, err := ec.txSender.CreateTxGeneric(
		ctx,
		sendtx.GenericTxDto{
			SrcChainID:             ec.ChainID(),
			SenderAddr:             senderAddr,
			SenderAddrPolicyScript: policyScript,
			Metadata:               metadata,
			Receivers:              receiversDto,
		},
	)
	if err != nil {
		return "", err
	}

	if ec.indexer != nil {
		ec.indexer.Add(txInfo.TxHash)
	}

	// it sufficient enough to check first address utxo
	_, err = ec.submitTx(ctx, txInfo.TxRaw, txInfo.TxHash, receivers[0].Addr, wallets)
	if err != nil {
		var sb strings.Builder

		for i, r := range receivers {
			if i > 0 {
				sb.WriteString(", ")
			}

			sb.WriteString(r.Addr)
		}

		return "", fmt.Errorf("failed to send tx %s to receiver(s) %s: %w", txInfo.TxHash, sb.String(), err)
	}

	return txInfo.TxHash, nil
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

func (ec *TestCardanoChain) GetIndexer() e2eindexer.TxsExecutedComponent {
	return ec.indexer
}

func (ec *TestCardanoChain) createIndexer() (e2eindexer.TxsExecutedComponent, error) {
	const (
		indexerRestartDelay   = time.Second * 5
		indexerKeepAlive      = true
		indexerSyncStartTries = 1_000_000_000
	)

	return e2eindexer.NewTxsExecutedComponentCardano(
		&gouroboros.BlockSyncerConfig{
			NetworkMagic: uint32(ec.config.NetworkMagic),
			NodeAddress: strings.TrimPrefix(strings.TrimPrefix(
				ec.cluster.Servers[0].NetworkAddress(), "http://"), "https://"),
			RestartOnError: true, // always try to restart on non-fatal errors
			RestartDelay:   indexerRestartDelay,
			KeepAlive:      indexerKeepAlive,
			SyncStartTries: indexerSyncStartTries,
		}, indexer.BlockPoint{
			BlockSlot: ec.config.IndexerStartSlot,
			BlockHash: ec.config.IndexerStartBlockHash,
		}, hclog.New(&hclog.LoggerOptions{
			Name:   fmt.Sprintf("indexer_%d", ec.config.ID),
			Output: os.Stdout,
			Level:  hclog.Warn,
		}))
}

func (ec *TestCardanoChain) getChainInfo() CardanoChainInfo {
	return CardanoChainInfo{
		NetworkAddress: ec.cluster.Servers[0].NetworkAddress(),
		OgmiosURL:      ec.ogmiosURL,
		MultisigAddr:   ec.multisigAddr,
		FeeAddr:        ec.multisigFeeAddr,
		SocketPath:     ec.cluster.OgmiosServer.SocketPath(),
	}
}

func (ec *TestCardanoChain) submitTx(
	ctx context.Context,
	rawTx []byte,
	txHash string,
	receiverAddr string,
	signers []*infrawallet.Wallet,
) (string, error) {
	const (
		retryCount    = 50
		retryWaitTime = time.Second * 5
	)

	txBuilder, err := infrawallet.NewTxBuilder(ResolveCardanoCliBinary(ec.config.NetworkType))
	if err != nil {
		return "", err
	}

	defer txBuilder.Dispose()

	witnesses := make([][]byte, len(signers))
	txProvider := infrawallet.NewTxProviderOgmios(ec.ogmiosURL)

	for i, signer := range signers {
		witnesses[i], err = txBuilder.CreateTxWitness(rawTx, signer)
		if err != nil {
			return "", err
		}
	}

	txSigned, err := txBuilder.AssembleTxWitnesses(rawTx, witnesses)
	if err != nil {
		return "", err
	}

	_, err = infracommon.ExecuteWithRetry(ctx, func(ctx context.Context) (bool, error) {
		return true, txProvider.SubmitTx(ctx, txSigned)
	}, infracommon.WithRetryCount(retryCount), infracommon.WithRetryWaitTime(retryWaitTime))
	if err != nil {
		return "", fmt.Errorf("failed to submit tx %s to receiver %s: %w", txHash, receiverAddr, err)
	}

	_, err = infracommon.ExecuteWithRetry(ctx, func(ctx context.Context) (bool, error) {
		contains, err := infrawallet.IsTxInUtxos(ctx, txProvider, receiverAddr, txHash)
		if err != nil {
			return false, err
		} else if !contains {
			return false, infracommon.ErrRetryTryAgain
		}

		return true, nil
	}, infracommon.WithRetryCount(retryCount), infracommon.WithRetryWaitTime(retryWaitTime))
	if err != nil {
		if errors.Is(err, infracommon.ErrRetryTimeout) &&
			ec.indexer != nil && slices.Contains(ec.indexer.GetFailedTxs(), txHash) {
			fmt.Printf("Transaction %s timed out because it was rolled back\n", txHash)
			// Since the timeout happened because of rollback, we return txHash normally
			// so later all the submited txs can be compared against the hashes of
			// txs that were rolled back
			return txHash, nil
		}

		return "", fmt.Errorf("failed to send tx %s to receiver %s: %w", txHash, receiverAddr, err)
	}

	return txHash, nil
}
