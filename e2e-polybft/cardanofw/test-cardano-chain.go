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

type TestCardanoChainConfig struct {
	IsEnabled                   bool
	ID                          int
	NetworkType                 infrawallet.CardanoNetworkType
	NodesCount                  int
	InitialHotWalletAmount      *big.Int
	InitialHotWalletTokenAmount *big.Int
	ChainType                   ChainID
	FundAmount                  uint64
	FundFeeAmount               uint64
	FundTokenAmount             uint64
	FundUTxOCount               int
	FundFeeUTxOCount            int
	PreminesAddresses           []string
	PremineAmount               uint64
	SlotRoundingThreshold       uint64
	TTLInc                      uint64
	MinBridgingFee              uint64
	MinOperationFee             uint64
	BridgeAddrHasStake          bool
	BridgingAddressCnt          int
}

func NewPrimeChainConfig() *TestCardanoChainConfig {
	return &TestCardanoChainConfig{
		IsEnabled:                   true,
		ID:                          0,
		NetworkType:                 infrawallet.TestNetNetwork,
		ChainType:                   ChainIDPrime,
		NodesCount:                  4,
		InitialHotWalletAmount:      big.NewInt(0),
		InitialHotWalletTokenAmount: big.NewInt(0),
		PremineAmount:               defaultPremineAmount,
		FundAmount:                  defaultFundTokenAmount,
		FundFeeAmount:               defaultFundTokenAmount,
		FundTokenAmount:             defaultNativeTokenAmount,
		FundUTxOCount:               1,
		FundFeeUTxOCount:            1,
		MinBridgingFee:              defaultMinBridgingFeeAmount,
		MinOperationFee:             uint64(0),
		BridgeAddrHasStake:          true,
		BridgingAddressCnt:          1,
	}
}

func NewVectorChainConfig(isEnabled bool) *TestCardanoChainConfig {
	return &TestCardanoChainConfig{
		IsEnabled:                   isEnabled,
		ID:                          1,
		NetworkType:                 infrawallet.VectorTestNetNetwork,
		ChainType:                   ChainIDVector,
		NodesCount:                  4,
		InitialHotWalletAmount:      big.NewInt(0),
		InitialHotWalletTokenAmount: big.NewInt(0),
		PremineAmount:               defaultPremineAmount,
		FundAmount:                  defaultFundTokenAmount,
		FundFeeAmount:               defaultFundTokenAmount,
		FundTokenAmount:             defaultNativeTokenAmount,
		FundUTxOCount:               1,
		FundFeeUTxOCount:            1,
		MinBridgingFee:              defaultMinBridgingFeeAmount,
		MinOperationFee:             uint64(0),
	}
}

func NewCardanoChainConfig(isEnabled bool) *TestCardanoChainConfig {
	return &TestCardanoChainConfig{
		IsEnabled:                   isEnabled,
		ID:                          4,
		NetworkType:                 infrawallet.TestNetNetwork,
		ChainType:                   ChainIDCardano,
		NodesCount:                  4,
		InitialHotWalletAmount:      big.NewInt(0),
		InitialHotWalletTokenAmount: big.NewInt(0),
		PremineAmount:               defaultPremineAmount,
		FundAmount:                  defaultFundTokenAmount,
		FundFeeAmount:               defaultFundTokenAmount,
		FundTokenAmount:             defaultNativeTokenAmount,
		MinBridgingFee:              defaultMinBridgingFeeAmount,
		MinOperationFee:             DefaultMinOperationFee,
		BridgingAddressCnt:          1,
	}
}

func NewRemotePrimeChainConfig(minBridgingFeeAmount, minOperationFee uint64) *TestCardanoChainConfig {
	return &TestCardanoChainConfig{
		IsEnabled:       true,
		ID:              0,
		NetworkType:     infrawallet.TestNetNetwork,
		ChainType:       ChainIDPrime,
		MinBridgingFee:  minBridgingFeeAmount,
		MinOperationFee: minOperationFee,
	}
}

func NewRemoteVectorChainConfig(isEnabled bool) *TestCardanoChainConfig {
	return &TestCardanoChainConfig{
		IsEnabled:   isEnabled,
		ID:          1,
		NetworkType: infrawallet.VectorTestNetNetwork,
		ChainType:   ChainIDVector,
	}
}

func NewRemoteCardanoChainConfig(
	isEnabled bool, minBridgingFeeAmount, minOperationFee uint64,
) *TestCardanoChainConfig {
	return &TestCardanoChainConfig{
		IsEnabled:       isEnabled,
		ID:              4,
		NetworkType:     infrawallet.TestNetNetwork,
		ChainType:       ChainIDCardano,
		MinBridgingFee:  minBridgingFeeAmount,
		MinOperationFee: minOperationFee,
	}
}

type TestCardanoChain struct {
	config            *TestCardanoChainConfig
	cluster           *TestCardanoCluster
	ogmiosURL         string
	blockfrostURL     string
	blockfrostAPIKey  string
	multisigAddr      []string
	multisigStakeAddr []string
	multisigFeeAddr   string
	txSender          *sendtx.TxSender
}

// GetBridgingStakeAddressInfo implements ITestApexChain.
func (ec *TestCardanoChain) GetBridgingStakeAddressInfo(
	t *testing.T,
	ctx context.Context,
	indx uint8,
	expectError bool,
) (infrawallet.QueryStakeAddressInfo, error) {
	t.Helper()
	require.True(t, ec.config.BridgeAddrHasStake)

	txProvider, err := ec.GetTxProvider()
	require.NoError(t, err)

	stakeBridgingAddrInfo, err := infracommon.ExecuteWithRetry(ctx,
		func(ctx context.Context) (infrawallet.QueryStakeAddressInfo, error) {
			addrInfo, err := txProvider.GetStakeAddressInfo(ctx, ec.multisigStakeAddr[indx])
			if err != nil && !expectError {
				return infrawallet.QueryStakeAddressInfo{}, infracommon.ErrRetryTryAgain
			}

			return addrInfo, err
		}, infracommon.WithRetryCount(60), infracommon.WithRetryWaitTime(time.Second))
	if !expectError {
		require.NoError(t, err)
	}

	return stakeBridgingAddrInfo, err
}

// GetExistingStakePools implements ITestApexChain.
func (ec *TestCardanoChain) GetExistingStakePools(t *testing.T, ctx context.Context) []string {
	t.Helper()

	txProvider, err := ec.GetTxProvider()
	require.NoError(t, err)

	stakePools, err := txProvider.GetStakePools(ctx)
	require.NoError(t, err)

	return stakePools
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
			return fmt.Sprintf("--%s-%s", config.ChainType, suffix)
		}

		return NewTestApexChainDummy([]string{
			getFlag("network-address"), "localhost:1000",
			getFlag("network-magic"), fmt.Sprint(GetNetworkMagic(config.NetworkType, config.ChainType)),
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

	networkName := ec.ChainID()
	ogmiosLogsFilePath := filepath.Join("..", "..", "e2e-logs-cardano",
		fmt.Sprintf("ogmios-%s-%s.log", networkName, strings.ReplaceAll(t.Name(), "/", "_")))

	cluster, err := NewCardanoTestCluster(
		WithID(ec.config.ID+1),
		WithNodesCount(ec.config.NodesCount),
		WithStartTimeDelay(time.Second*5),
		WithPort(5100+ec.config.ID*100),
		WithOgmiosPort(1337+ec.config.ID),
		WithNetworkType(ec.config.NetworkType),
		WithChainType(networkName),
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

	for i := range ec.config.BridgingAddressCnt {
		args := []string{
			"create-address",
			"--network-id", fmt.Sprint(ec.config.NetworkType),
			"--testnet-magic", fmt.Sprint(GetNetworkMagic(ec.config.NetworkType, ec.ChainID())),
			"--addr-index", fmt.Sprint(i),
			"--bridge-url", bridgeURL,
			"--bridge-addr", contracts.Bridge.String(),
			"--bridge-key", hex.EncodeToString(bridgeAdminPk),
			"--chain", ec.ChainID(),
		}

		var outb bytes.Buffer

		err = RunCommand(ResolveApexBridgeBinary(), args, io.MultiWriter(os.Stdout, &outb))
		if err != nil {
			return err
		}

		output := outb.String()
		reMultisig := regexp.MustCompile(`Multisig Address\s*=\s*([^\s]+)`)
		reFee := regexp.MustCompile(`Fee Payer Address\s*=\s*([^\s]+)`)
		reMultisigStake := regexp.MustCompile(`Multisig Stake Address\s*=\s*([^\s]+)`)

		if match := reMultisig.FindStringSubmatch(output); len(match) > 0 {
			ec.multisigAddr = append(ec.multisigAddr, match[1])
		}

		if match := reMultisigStake.FindStringSubmatch(output); len(match) > 0 {
			ec.multisigStakeAddr = append(ec.multisigStakeAddr, match[1])
		}

		if match := reFee.FindStringSubmatch(output); len(match) > 0 {
			ec.multisigFeeAddr = match[1]
		}
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
			txHash, err := ec.SendTx(ctx, privateKey, ec.multisigFeeAddr, amount, nil, nil)
			if err != nil {
				return err
			}

			fmt.Printf("%s fee addr: %s funded with %s: %s\n", ec.ChainID(), ec.multisigFeeAddr, amount, txHash)
		}
	}

	if ec.config.FundTokenAmount != 0 || ec.config.FundAmount != 0 {
		minterWallet, err := GetGenesisWalletFromCluster(ec.cluster.Config.TmpDir, 1)
		if err != nil {
			return err
		}

		tokenAmounts := []*big.Int{
			new(big.Int).SetUint64(max(2*MinUTxODefaultValue, ec.config.FundAmount)),
			new(big.Int).SetUint64(ec.config.FundTokenAmount),
		}

		for _, amounts := range SplitAmountsNTimes(tokenAmounts, ec.config.FundUTxOCount) {
			token, err := FundAddressWithToken(
				ctx, ec,
				minterWallet, ec.GetHotWalletAddress(),
				DefaultTokenName, DefaultTokenMintAmount,
				amounts[0].Uint64(), amounts[1].Uint64())
			if err != nil {
				return err
			}

			fmt.Printf("%s multisig addr funded with native currency and token `%s` amount: %s, %s\n",
				ec.ChainID(), token.TokenName(), amounts[0], amounts[1])
		}
	}

	return nil
}

func (ec *TestCardanoChain) InitContracts(_ context.Context, _ *crypto.ECDSAKey, _ string) error {
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
		getFlag("network-magic"), fmt.Sprint(GetNetworkMagic(ec.config.NetworkType, ec.ChainID())),
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
	return ec.config.ChainType
}

func (ec *TestCardanoChain) GetAddressBalance(ctx context.Context, addr string) (map[string]*big.Int, error) {
	txProvider, err := ec.GetTxProvider()
	if err != nil {
		return nil, err
	}

	utxos, err := txProvider.GetUtxos(ctx, addr)
	if err != nil {
		return nil, err
	}

	balance := infrawallet.GetUtxosSum(utxos)

	balanceTransformed := make(map[string]*big.Int, len(balance))

	for key, value := range balance {
		balanceTransformed[key] = new(big.Int).SetUint64(value)
	}

	return balanceTransformed, nil
}

func (ec *TestCardanoChain) GetBridgingFee(
	ctx context.Context,
	dstChainID string,
	receivers []sendtx.BridgingTxReceiver,
	bridgingFee uint64,
	operationFee uint64,
	multiSigAddr string,
) (uint64, error) {
	return ec.txSender.GetBridgingFee(
		ctx, ec.ChainID(), dstChainID, receivers, multiSigAddr, bridgingFee, operationFee)
}

func (ec *TestCardanoChain) CreateMetadata(
	senderAddr string,
	dstChainID string,
	receivers []sendtx.BridgingTxReceiver,
	bridgingFee uint64,
	operationFee uint64,
) ([]byte, error) {
	metadata, err := ec.txSender.CreateMetadata(
		senderAddr, ec.ChainID(), dstChainID, receivers, bridgingFee, operationFee)
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
	srcChainID := ec.ChainID()

	walletAddr, err := GetAddress(ec.config.NetworkType, wallet)
	if err != nil {
		return "", err
	}

	receivers := make([]sendtx.BridgingTxReceiver, 0, len(receiversMap))

	bridgingType := sendtx.BridgingTypeNormal
	if len(bridgingTypes) > 0 {
		bridgingType = bridgingTypes[0]
	}

	totalAmnt := uint64(0)
	for receiverAddress, receiverAmount := range receiversMap {
		totalAmnt += receiverAmount.Uint64()
		receivers = append(receivers, sendtx.BridgingTxReceiver{
			Addr:         receiverAddress,
			Amount:       DfmToChainNativeTokenAmount(srcChainID, receiverAmount).Uint64(),
			BridgingType: bridgingType,
		})
	}

	multisigAddr, err := ec.determineMultisigAddressToSendTo(ctx, totalAmnt)
	if err != nil {
		return "", err
	}

	txInfo, _, err := ec.txSender.CreateBridgingTx(
		ctx,
		srcChainID,
		dstChainID,
		walletAddr.String(),
		receivers,
		multisigAddr,
		feeAmount.Uint64(),
		operationFee,
	)
	if err != nil {
		return "", err
	}

	return ec.submitTx(ctx, txInfo.TxRaw, txInfo.TxHash, multisigAddr, wallet)
}

func (ec *TestCardanoChain) determineMultisigAddressToSendTo(ctx context.Context, amount uint64) (string, error) {
	txProvider, err := ec.GetTxProvider()
	if err != nil {
		return "", err
	}

	minAmount := uint64(0)
	index := 0

	for i, address := range ec.multisigAddr {
		utxos, err := txProvider.GetUtxos(ctx, address)
		if err != nil {
			return "", err
		}

		addrAmount := uint64(0)
		for _, utxo := range utxos {
			addrAmount += utxo.Amount
		}

		if addrAmount == 0 {
			fmt.Printf("%s address with index %d chosen for bridging because of 0 amount\n", address, i)
			return address, nil
		}

		if i == 0 {
			minAmount = addrAmount
		} else if amount < minAmount {
			minAmount = amount
			index = i
		}
	}

	fmt.Printf("%s address with index %d chosen for bridging\n", ec.multisigAddr[index], index)
	return ec.multisigAddr[index], nil
}

func (ec *TestCardanoChain) SendTx(
	ctx context.Context,
	privateKey string,
	receiverAddr string,
	amount *big.Int,
	nativeTokenAmounts []infrawallet.TokenAmount,
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

	txInfo, err := ec.txSender.CreateTxGeneric(
		ctx,
		ec.ChainID(),
		walletAddr.String(),
		receiverAddr,
		metadata,
		amount.Uint64(),
		nativeTokenAmounts,
	)
	if err != nil {
		return "", err
	}

	_, err = ec.submitTx(ctx, txInfo.TxRaw, txInfo.TxHash, receiverAddr, wallet)
	if err != nil {
		return "", fmt.Errorf("failed to send tx %s to receiver %s: %w", txInfo.TxHash, receiverAddr, err)
	}

	return txInfo.TxHash, nil
}

func (ec *TestCardanoChain) GetHotWalletAddress() string {
	return ec.multisigAddr[0]
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

	if err := ec.txSender.SubmitTx(ctx, ec.ChainID(), rawTx, signer); err != nil {
		return "", err
	}

	_, err := infracommon.ExecuteWithRetry(ctx, func(ctx context.Context) (bool, error) {
		contains, err := infrawallet.IsTxInUtxos(ctx, txProvider, receiverAddr, txHash)
		if err != nil {
			return false, err
		} else if !contains {
			return false, infracommon.ErrRetryTryAgain
		}

		return true, nil
	}, infracommon.WithRetryCount(retryCount), infracommon.WithRetryWaitTime(retryWaitTime))
	if err != nil {
		return "", fmt.Errorf("failed to send tx %s to receiver %s: %w", txHash, receiverAddr, err)
	}

	return txHash, nil
}
