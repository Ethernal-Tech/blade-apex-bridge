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
	config           *TestCardanoChainConfig
	cluster          *TestCardanoCluster
	ogmiosURL        string
	blockfrostURL    string
	blockfrostAPIKey string
	multisigAddr     string
	multisigFeeAddr  string
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

	args := []string{
		"create-address",
		"--network-id", fmt.Sprint(ec.config.NetworkType),
		"--testnet-magic", fmt.Sprint(GetNetworkMagic(ec.config.NetworkType, ec.ChainID())),
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

func (ec *TestCardanoChain) FundAddress(ctx context.Context, address string, amount uint64) error {
	privateKey, err := ec.GetAdminPrivateKey()
	if err != nil {
		return err
	}

	paymentKey, _, err := FromCardanoPrivateKeyString(privateKey)
	if err != nil {
		return err
	}

	txHash, err := ec.SendSimpleTx(ctx, paymentKey, nil, []string{address}, []uint64{amount}, nil, 0, false)
	if err != nil {
		return err
	}

	fmt.Printf("%s addr funded with %d: tx: %s\n", address, amount, txHash)

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
) (uint64, error) {
	return ec.txSender.GetBridgingFee(
		ctx, ec.ChainID(), dstChainID, receivers, bridgingFee, operationFee)
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

	for receiverAddress, receiverAmount := range receiversMap {
		receivers = append(receivers, sendtx.BridgingTxReceiver{
			Addr:         receiverAddress,
			Amount:       DfmToChainNativeTokenAmount(srcChainID, receiverAmount).Uint64(),
			BridgingType: bridgingType,
		})
	}

	txInfo, _, err := ec.txSender.CreateBridgingTx(
		ctx,
		srcChainID,
		dstChainID,
		walletAddr.String(),
		receivers,
		feeAmount.Uint64(),
		operationFee,
	)
	if err != nil {
		return "", err
	}

	return ec.submitTx(ctx, txInfo.TxRaw, txInfo.TxHash, ec.multisigAddr, []infrawallet.ITxSigner{wallet})
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

	_, err = ec.submitTx(ctx, txInfo.TxRaw, txInfo.TxHash, receiverAddr, []infrawallet.ITxSigner{wallet})
	if err != nil {
		return "", fmt.Errorf("failed to send tx %s to receiver %s: %w", txInfo.TxHash, receiverAddr, err)
	}

	return txInfo.TxHash, nil
}

func (ec *TestCardanoChain) SendSimpleTx(
	ctx context.Context,
	privateKey []byte,
	stakePrivateKey []byte,
	receiversAddr []string,
	amounts []uint64,
	certificates []*sendtx.CertificatesWithScript,
	setFee uint64,
	fullTransfer bool,
) (string, error) {
	wallet := infrawallet.NewWallet(privateKey, stakePrivateKey)

	walletAddr, err := GetAddress(ec.config.NetworkType, wallet)
	if err != nil {
		return "", err
	}

	txInfo, err := ec.txSender.CreateTxSimple(
		ctx,
		ec.ChainID(),
		walletAddr.String(),
		receiversAddr,
		amounts,
		certificates,
		setFee,
		fullTransfer,
	)
	if err != nil {
		return "", err
	}

	// Since these kind of transactions: stake key registration and stake delegation
	// usually don't have the receiver we set the receiverAddr to senderAddr
	// that way we check if the tx pass - sender get's change utxo
	receiverAddr := ""
	if len(receiversAddr) == 0 {
		receiverAddr = walletAddr.String()
	} else {
		receiverAddr = receiversAddr[0]
	}

	signers := []infrawallet.ITxSigner{wallet}
	if stakePrivateKey != nil {
		signers = append(signers, infrawallet.NewWallet(stakePrivateKey, []byte{}))
	}

	_, err = ec.submitTx(ctx, txInfo.TxRaw, txInfo.TxHash, receiverAddr, signers)
	if err != nil {
		return "", fmt.Errorf("failed to send tx %s to receiver %s: %w", txInfo.TxHash, receiverAddr, err)
	}

	return txInfo.TxHash, nil
}

func (ec *TestCardanoChain) SendTxWithFeePayer(
	ctx context.Context,
	privateKeys [][]byte,
	stakePrivateKeys [][]byte,
	amountPerSender []uint64,
	receiversAddr []string,
	amountPerReceiver []uint64,
	certificates []*sendtx.CertificatesWithScript,
	feePayerAddr string,
) (string, error) {
	if len(receiversAddr) != len(amountPerReceiver) {
		return "", fmt.Errorf("receiversAddr and amountPerReceiver must have the same length")
	}

	if len(privateKeys) != len(stakePrivateKeys) || len(privateKeys) != len(amountPerSender) {
		return "", fmt.Errorf("privateKeys, stakePrivateKeys and amountPerSender must have the same length")
	}

	wallets := make([]*infrawallet.Wallet, len(privateKeys))
	for i, key := range privateKeys {
		wallets[i] = infrawallet.NewWallet(key, stakePrivateKeys[i])
	}

	senderAddresses := make([]string, len(wallets))
	for i, wallet := range wallets {
		addr, err := GetAddress(ec.config.NetworkType, wallet)
		if err != nil {
			return "", err
		}
		senderAddresses[i] = addr.String()
	}

	txInfo, err := ec.txSender.CreateComplexTx(
		ctx,
		ec.ChainID(),
		senderAddresses,
		amountPerSender,
		receiversAddr,
		amountPerReceiver,
		certificates,
		feePayerAddr,
	)
	if err != nil {
		return "", err
	}
	fmt.Println(txInfo)
	// Since these kind of transactions: stake key registration and stake delegation
	// usually don't have the receiver we set the receiverAddr to senderAddr
	// that way we check if the tx pass - sender get's change utxo
	receiverAddr := ""
	if len(receiversAddr) == 0 {
		receiverAddr = senderAddresses[0]
	} else {
		receiverAddr = receiversAddr[0]
	}

	signers := make([]infrawallet.ITxSigner, len(wallets))
	for i, wallet := range wallets {
		signers[i] = wallet
	}

	_, err = ec.submitTx(ctx, txInfo.TxRaw, txInfo.TxHash, receiverAddr, signers)
	if err != nil {
		return "", fmt.Errorf("failed to send tx %s to receiver %s: %w", txInfo.TxHash, receiverAddr, err)
	}

	return txInfo.TxHash, nil
}

func (ec *TestCardanoChain) CreateRegAndDelegTx(
	ctx context.Context,
	wallets []*infrawallet.Wallet,
	certificates []*sendtx.CertificatesWithScript,
	feePayerWallet *infrawallet.Wallet,
) (string, error) {
	feePayerAddress, err := GetAddress(ConfigNetworkType, feePayerWallet)
	if err != nil {
		return "", err
	}

	registrationFee := uint64(0)
	regDepositAmnt := uint64(2_000_000)
	for _, certs := range certificates {
		for _, cert := range certs.Certificates {
			if cert.GetDescription() == "Stake Address Registration Certificate" {
				registrationFee += regDepositAmnt
			}
		}
	}

	txInfo, err := ec.txSender.CreateComplexTx(
		ctx,
		ec.ChainID(),
		[]string{feePayerAddress.String()},
		[]uint64{100_000_000},
		[]string{feePayerAddress.String()},
		[]uint64{100_000_000 - registrationFee},
		certificates,
		feePayerAddress.String(),
	)
	if err != nil {
		return "", err
	}

	signers := make([]infrawallet.ITxSigner, len(wallets)+1)
	for i, wallet := range wallets {
		//signers[i] = wallet
		signers[i] = infrawallet.NewWallet(wallet.StakeSigningKey, nil)
	}

	signers[len(wallets)] = feePayerWallet

	_, err = ec.submitTx(ctx, txInfo.TxRaw, txInfo.TxHash, feePayerAddress.String(), signers)
	if err != nil {
		return "", fmt.Errorf("failed to send tx %s to receiver %s: %w", txInfo.TxHash, feePayerAddress.String(), err)
	}

	return txInfo.TxHash, nil
}

func (ec *TestCardanoChain) SendWithdrawRewardsTx(
	ctx context.Context,
	privateKey []byte,
	stakePrivateKey []byte,
	stakeAddr string,
	rewardAmount uint64,
	setFee uint64,
	receiverAddr ...string,
) (string, error) {
	fmt.Printf("Sending withdraw rewards tx for stake addr: %s, reward amount: %d, set fee: %d\n", stakeAddr, rewardAmount, setFee)
	wallet := infrawallet.NewWallet(privateKey, stakePrivateKey)
	stakeWallet := infrawallet.NewWallet(stakePrivateKey, []byte{})

	walletAddr, err := GetAddress(ec.config.NetworkType, wallet)
	if err != nil {
		return "", err
	}

	receiver := ""
	if len(receiverAddr) > 0 {
		receiver = receiverAddr[0]
	}

	txInfo, err := ec.txSender.CreateWithdrawRewardsTx(
		ctx,
		ec.ChainID(),
		walletAddr.String(),
		stakeAddr,
		rewardAmount,
		receiver,
		setFee,
	)
	if err != nil {
		return "", err
	}

	return ec.submitTx(ctx, txInfo.TxRaw, txInfo.TxHash, walletAddr.String(), []infrawallet.ITxSigner{wallet, stakeWallet})
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
	signers []infrawallet.ITxSigner,
) (string, error) {
	const (
		retryCount    = 40
		retryWaitTime = time.Second * 5
	)

	txProvider := infrawallet.NewTxProviderOgmios(ec.ogmiosURL)

	if err := ec.txSender.SubmitTx(ctx, ec.ChainID(), rawTx, signers); err != nil {
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
