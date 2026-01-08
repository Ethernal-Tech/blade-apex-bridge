package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/e2e-polybft/cardanofw"
	"github.com/Ethernal-Tech/cardano-infrastructure/sendtx"
	"github.com/Ethernal-Tech/cardano-infrastructure/wallet"
	"github.com/stretchr/testify/require"
)

type WaitOption int

const (
	WaitRefundDisabled WaitOption = iota
	WaitRefundEnabled
	WaitTimeoutRefundDisabled
	NoWait
)

const (
	bridgingMetaDataType sendtx.BridgingRequestType = "bridge"
	metadataMapKey       int                        = 1
)

type colCoinInvalidOpts struct {
	receivers        []sendtx.BridgingTxReceiver
	amount           uint64
	waitOption       WaitOption
	metadataModifier func([]byte) []byte
}

// backward compatibility
type BridgingRequestMetadataTransactionBC struct {
	Address                     []string `cbor:"a" json:"a"`
	IsNativeTokenOnSrc_Obsolete byte     `cbor:"nt" json:"nt"` //nolint:stylecheck
	Amount                      uint64   `cbor:"m" json:"m"`
	TokenID                     uint16   `cbor:"t" json:"t"`
}

// backward compatibility
type BridgingRequestMetadataBC struct {
	BridgingTxType     sendtx.BridgingRequestType             `cbor:"t" json:"t"`
	DestinationChainID string                                 `cbor:"d" json:"d"`
	SenderAddr         []string                               `cbor:"s" json:"s"`
	Transactions       []BridgingRequestMetadataTransactionBC `cbor:"tx" json:"tx"`
	BridgingFee        uint64                                 `cbor:"fa" json:"fa"`
	OperationFee       uint64                                 `cbor:"of" json:"of"`
}

type testConfig struct {
	srcChainID cardanofw.ChainID
	dstChainID cardanofw.ChainID

	srcMinterWallet *wallet.Wallet
	srcNetworkType  wallet.CardanoNetworkType
	srcTxProvider   wallet.ITxProvider
	srcMultiSigAddr string
	tokenID         uint16
	tokensInfo      *cardanofw.BridgingTokensInfo
	isCurrency      bool
}

const (
	defaultSendAmount = uint64(1_000_000)
)

func newTestConfig(
	t *testing.T, apex *cardanofw.ApexSystem, config *cardanofw.TestCardanoChainConfig,
	info *cardanofw.CardanoChainInfo, dstChainID cardanofw.ChainID, srcTokenID uint16,
) *testConfig {
	t.Helper()

	txProvider, err := info.GetTxProvider()
	require.NoError(t, err)

	tokenInfo, err := apex.GetBridgingTokensInfo(config.ChainType, dstChainID, srcTokenID)
	require.NoError(t, err)

	currencyID, err := apex.GetChainCurrencyID(config.ChainType)
	require.NoError(t, err)

	return &testConfig{
		srcChainID:      config.ChainType,
		dstChainID:      dstChainID,
		srcNetworkType:  config.NetworkType,
		srcTxProvider:   txProvider,
		srcMinterWallet: info.GenesisWallet,
		srcMultiSigAddr: info.MultisigAddr[0],
		tokenID:         srcTokenID,
		tokensInfo:      tokenInfo,
		isCurrency:      currencyID == srcTokenID,
	}
}

// Util methods
func WaitForInvalidTestResult(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, config *testConfig, user *cardanofw.TestApexUser,
	txHash string, beforeSendingAmountDfm map[string]*big.Int, sentAmount uint64,
	refundEnabled bool, maxWaitTimeSec, retryIntervalSec uint,
) {
	t.Helper()

	retryIntervalSec = max(retryIntervalSec, 1)
	numRetries := max(1, int(maxWaitTimeSec/retryIntervalSec))

	if refundEnabled {
		lowerBoundaryDfm := new(big.Int).Sub(
			beforeSendingAmountDfm[config.tokensInfo.SrcTokenName], new(big.Int).SetUint64(sentAmount))

		fmt.Printf("Tx sent. hash: %s, lowerBoundaryDfm: %d, higherBoundaryDfm: %+v\n", txHash, lowerBoundaryDfm,
			beforeSendingAmountDfm)

		err := apex.WaitForAmountInRange(ctx, user, config.srcChainID, lowerBoundaryDfm,
			beforeSendingAmountDfm[config.tokensInfo.SrcTokenName], numRetries,
			time.Second*time.Duration(retryIntervalSec), config.tokensInfo.SrcTokenName)
		require.NoError(t, err)
	} else {
		cardanofw.WaitForInvalidState(t, ctx, apex, config.srcChainID, txHash, apex.Config.APIKey, maxWaitTimeSec)
	}
}

// Test methods
func submitMismatchAndWait(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, config *testConfig, user *cardanofw.TestApexUser,
	metadata []byte, lovelaceAmount *big.Int, sentTokenAmount []wallet.TokenAmount, waitForAmount uint64,
	waitOption WaitOption, maxWaitTimeSec, retryIntervalSec uint, addrIndex uint8,
	operationFee uint64,
) {
	t.Helper()

	beforeSendingAmountDfm, err := apex.GetBalance(ctx, user, config.srcChainID)
	require.NoError(t, err)

	txHash, err := apex.SubmitTx(
		ctx, config.srcChainID, user, apex.GetCardanoInfo(config.srcChainID).MultisigAddr[addrIndex],
		lovelaceAmount, sentTokenAmount, metadata, new(big.Int).SetUint64(operationFee))
	require.NoError(t, err)

	if waitOption == WaitTimeoutRefundDisabled {
		_, err = cardanofw.WaitForRequestStates(ctx, apex, config.srcChainID, txHash, apex.Config.APIKey, nil, maxWaitTimeSec)
		require.Error(t, err)
		require.ErrorContains(t, err, "timeout")

		return
	}

	if waitOption != NoWait {
		WaitForInvalidTestResult(t, ctx, apex, config, user, txHash, beforeSendingAmountDfm, waitForAmount,
			waitOption == WaitRefundEnabled, maxWaitTimeSec, retryIntervalSec)
	}
}

func submitColCoinsMismatchAndWait(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, config *testConfig, user *cardanofw.TestApexUser,
	receivers []sendtx.BridgingTxReceiver, amount uint64,
	maxWaitTimeSec, retryIntervalSec uint, addrIndex uint8,
	waitOption WaitOption, metadataModifier func([]byte) []byte,
) {
	t.Helper()

	operationFee := apex.GetMinOperationFee(config.srcChainID)

	metadata, feeAmount := createMetadata(t, ctx, apex, config.srcChainID, config.dstChainID,
		apex.GetMinBridgingFee(config.srcChainID, !config.isCurrency),
		operationFee, user, receivers, config.isCurrency)

	if metadataModifier != nil {
		metadata = metadataModifier(metadata)
	}

	waitForAmount := amount
	lovelaceAmount := new(big.Int).SetUint64(feeAmount)

	token, err := wallet.NewTokenWithFullName(config.tokensInfo.SrcTokenName, true)
	require.NoError(t, err)

	sentTokenAmount := []wallet.TokenAmount{{
		Token:  token,
		Amount: amount,
	}}

	submitMismatchAndWait(t, ctx, apex, config, user, metadata, lovelaceAmount, sentTokenAmount, waitForAmount,
		waitOption, maxWaitTimeSec, retryIntervalSec, addrIndex, operationFee)
}

func executeInvalidMismatchSendLovelaceAmount(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, config *testConfig, user *cardanofw.TestApexUser,
	maxWaitTimeSec, retryIntervalSec uint, refundEnabled bool, addrIndex uint8,
) {
	t.Helper()

	receivers := createReceivers(apex, 1, config.dstChainID, defaultSendAmount*10, config.tokenID)

	operationFee := apex.GetMinOperationFee(config.srcChainID)

	metadata, feeAmount := createMetadata(t, ctx, apex, config.srcChainID, config.dstChainID,
		apex.GetMinBridgingFee(config.srcChainID, !config.isCurrency),
		operationFee,
		user, receivers, config.isCurrency)

	lovelaceAmount, sentTokenAmount, waitForAmount := getDefaultSendAmounts(
		t, config, feeAmount, operationFee)

	waitOption := WaitRefundDisabled
	if refundEnabled {
		waitOption = WaitRefundEnabled
	}

	submitMismatchAndWait(t, ctx, apex, config, user, metadata, lovelaceAmount, sentTokenAmount, waitForAmount,
		waitOption, maxWaitTimeSec, retryIntervalSec, addrIndex, operationFee)
}

func executeInvalidColCoin(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, config *testConfig, user *cardanofw.TestApexUser,
	maxWaitTimeSec, retryIntervalSec uint, addrIndex uint8, opts colCoinInvalidOpts,
) {
	t.Helper()

	submitColCoinsMismatchAndWait(
		t, ctx, apex, config, user, opts.receivers, opts.amount,
		maxWaitTimeSec, retryIntervalSec, addrIndex, opts.waitOption, opts.metadataModifier)
}

func executeInvalidMismatchSendColCoinsMultipleInstancesParalel(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, config *testConfig, amount uint64,
	instances int, maxWaitTimeSec, retryIntervalSec uint, refundEnabled bool,
	addrIndex uint8,
) {
	t.Helper()

	var wg sync.WaitGroup

	for i := range instances {
		wg.Add(1)

		go func(idx int) {
			defer wg.Done()

			receivers := createReceivers(apex, 1, config.dstChainID, amount*10, config.tokenID)

			waitOption := WaitRefundDisabled
			if refundEnabled {
				waitOption = WaitRefundEnabled
			}

			submitColCoinsMismatchAndWait(t, ctx, apex, config, apex.Users[idx], receivers, amount,
				maxWaitTimeSec, retryIntervalSec, addrIndex, waitOption, nil)
		}(i)
	}

	wg.Wait()
}

func executeInvalidMismatchSendAmountMultipleInstances(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, config *testConfig,
	maxWaitTimeSec, retryIntervalSec uint, refundEnabled bool, addrIndex uint8,
) {
	t.Helper()

	const instances = 5

	for i := 0; i < instances; i++ {
		receivers := createReceivers(apex, 1, config.dstChainID, defaultSendAmount*10, config.tokenID)

		operationFee := apex.GetMinOperationFee(config.srcChainID)

		metadata, feeAmount := createMetadata(t, ctx, apex, config.srcChainID, config.dstChainID,
			apex.GetMinBridgingFee(config.srcChainID, !config.isCurrency),
			operationFee,
			apex.Users[i], receivers, config.isCurrency)

		beforeSendingAmountDfm, err := apex.GetBalance(ctx, apex.Users[i], config.srcChainID)
		require.NoError(t, err)

		lovelaceAmount, sentTokenAmount, waitForAmount := getDefaultSendAmounts(
			t, config, feeAmount, operationFee)

		txHash, err := apex.SubmitTx(
			ctx, config.srcChainID, apex.Users[i],
			apex.GetCardanoInfo(config.srcChainID).MultisigAddr[addrIndex],
			lovelaceAmount, sentTokenAmount, metadata, new(big.Int).SetUint64(operationFee))
		require.NoError(t, err)

		WaitForInvalidTestResult(t, ctx, apex, config, apex.Users[i], txHash, beforeSendingAmountDfm, waitForAmount,
			refundEnabled, maxWaitTimeSec, retryIntervalSec)
	}
}

func executeInvalidMismatchSendAmountMultipleInstancesParalel(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, config *testConfig,
	maxWaitTimeSec, retryIntervalSec uint, refundEnabled bool,
	addrIndex uint8,
) {
	t.Helper()

	instances := 5

	var wg sync.WaitGroup

	for i := 0; i < instances; i++ {
		wg.Add(1)

		go func(idx int) {
			defer wg.Done()

			receivers := createReceivers(apex, 1, config.dstChainID, defaultSendAmount*10, config.tokenID)

			operationFee := apex.GetMinOperationFee(config.srcChainID)

			metadata, feeAmount := createMetadata(t, ctx, apex, config.srcChainID, config.dstChainID,
				apex.GetMinBridgingFee(config.srcChainID, !config.isCurrency),
				operationFee,
				apex.Users[i], receivers, config.isCurrency)

			beforeSendingAmountDfm, err := apex.GetBalance(ctx, apex.Users[idx], config.srcChainID)
			require.NoError(t, err)

			lovelaceAmount, sentTokenAmount, waitForAmount := getDefaultSendAmounts(
				t, config, feeAmount, operationFee)

			txHashe, err := apex.SubmitTx(
				ctx, config.srcChainID, apex.Users[idx],
				apex.GetCardanoInfo(config.srcChainID).MultisigAddr[addrIndex],
				lovelaceAmount, sentTokenAmount, metadata, new(big.Int).SetUint64(operationFee))
			require.NoError(t, err)

			WaitForInvalidTestResult(t, ctx, apex, config, apex.Users[idx], txHashe, beforeSendingAmountDfm, waitForAmount,
				refundEnabled, maxWaitTimeSec, retryIntervalSec)
		}(i)
	}

	wg.Wait()
}

func executeInvalidMetadataType(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, config *testConfig, user *cardanofw.TestApexUser,
	maxWaitTimeSec, retryIntervalSec uint, refundEnabled bool, addrIndex uint8,
) {
	t.Helper()

	receivers := createReceivers(apex, 1, config.dstChainID, defaultSendAmount, config.tokenID)

	operationFee := apex.GetMinOperationFee(config.srcChainID)

	metadata, feeAmount := createMetadata(t, ctx, apex, config.srcChainID, config.dstChainID,
		apex.GetMinBridgingFee(config.srcChainID, !config.isCurrency),
		operationFee, user, receivers, config.isCurrency)
	metadata = bytes.Replace(metadata, []byte("bridge"), []byte("xxxxx"), 1)

	beforeSendingAmountDfm, err := apex.GetBalance(ctx, user, config.srcChainID)
	require.NoError(t, err)

	lovelaceAmount, sentTokenAmount, waitForAmount := getDefaultSendAmounts(
		t, config, feeAmount, operationFee)

	txHash, err := apex.SubmitTx(
		ctx, config.srcChainID, user, apex.GetCardanoInfo(config.srcChainID).MultisigAddr[addrIndex],
		lovelaceAmount, sentTokenAmount, metadata, new(big.Int).SetUint64(operationFee))
	require.NoError(t, err)

	if refundEnabled {
		WaitForInvalidTestResult(t, ctx, apex, config, user, txHash, beforeSendingAmountDfm, waitForAmount,
			refundEnabled, maxWaitTimeSec, retryIntervalSec)
	} else {
		_, err = cardanofw.WaitForRequestStates(ctx, apex, config.srcChainID, txHash, apex.Config.APIKey, nil, maxWaitTimeSec)
		require.Error(t, err)
		require.ErrorContains(t, err, "timeout")
	}
}

func executeObsoleteMetadata(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, config *testConfig, user *cardanofw.TestApexUser,
	maxWaitTimeSec, retryIntervalSec uint, addrIndex uint8,
) {
	t.Helper()

	receivers := createReceivers(apex, 1, config.dstChainID, defaultSendAmount, config.tokenID)

	operationFee := apex.GetMinOperationFee(config.srcChainID) + 500_000

	metadata, feeAmount := createObsoleteMetadata(t, ctx, apex, config.srcChainID, config.dstChainID,
		apex.GetMinBridgingFee(config.srcChainID, !config.isCurrency),
		operationFee, user, receivers, config.isCurrency)

	balance, err := apex.GetBalanceWithTokenName(
		ctx, apex.Users[len(apex.Users)-1], config.dstChainID, config.tokensInfo.DstTokenName)
	fmt.Printf("Receiver balance: %+v\n", balance)
	require.NoError(t, err)

	lovelaceAmount, sentTokenAmount, _ := getDefaultSendAmounts(
		t, config, feeAmount, operationFee)

	txHash, err := apex.SubmitTx(
		ctx, config.srcChainID, user, apex.GetCardanoInfo(config.srcChainID).MultisigAddr[addrIndex],
		lovelaceAmount, sentTokenAmount, metadata, new(big.Int).SetUint64(operationFee))
	require.NoError(t, err)

	require.NoError(t, err)

	fmt.Printf("Tx sent. hash: %s\n", txHash)

	currentAmount, ok := balance[config.tokensInfo.DstTokenName]
	if !ok {
		currentAmount = big.NewInt(0)
	}

	expectedAmount := new(big.Int).Add(currentAmount, big.NewInt(int64(defaultSendAmount)))

	numRetries := max(1, int(maxWaitTimeSec/retryIntervalSec))

	err = apex.WaitForExactAmount(ctx, user, config.dstChainID, expectedAmount,
		numRetries, time.Second*time.Duration(retryIntervalSec), config.tokensInfo.DstTokenName)

	require.NoError(t, err)
}

func executeInvalidDestination(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, config *testConfig, user *cardanofw.TestApexUser,
	maxWaitTimeSec, retryIntervalSec uint, refundEnabled bool, addrIndex uint8,
) {
	t.Helper()

	receivers := createReceivers(apex, 0, config.dstChainID, defaultSendAmount, config.tokenID)
	receiversForFeeCalculation := []sendtx.BridgingTxReceiver{
		{
			Addr:    user.GetAddress(config.dstChainID),
			Amount:  defaultSendAmount,
			TokenID: config.tokensInfo.SrcTokenID,
		},
	}

	srcTestChain := apex.GetChainMust(t, config.srcChainID)

	operationFee := apex.GetMinOperationFee(config.srcChainID)

	feeAmount, err := srcTestChain.GetBridgingFee(
		ctx, config.dstChainID, receiversForFeeCalculation,
		apex.GetMinBridgingFee(config.srcChainID, !config.isCurrency),
		operationFee, config.srcMultiSigAddr)
	require.NoError(t, err)

	metadata, err := srcTestChain.CreateMetadata(
		user.GetAddress(config.srcChainID), config.dstChainID, receivers, feeAmount,
		operationFee,
	)
	require.NoError(t, err)

	metadata = bytes.Replace(metadata, fmt.Appendf(nil, "\"%s\"", config.dstChainID), []byte("\"unknown\""), 1)

	beforeSendingAmountDfm, err := apex.GetBalanceWithTokenName(
		ctx, user, config.srcChainID, config.tokensInfo.SrcTokenName)
	require.NoError(t, err)

	lovelaceAmount, sentTokenAmount, waitForAmount := getDefaultSendAmounts(
		t, config, feeAmount, operationFee)

	txHash, err := apex.SubmitTx(
		ctx, config.srcChainID, user, apex.GetCardanoInfo(config.srcChainID).MultisigAddr[addrIndex],
		lovelaceAmount, sentTokenAmount, metadata, new(big.Int).SetUint64(operationFee))
	require.NoError(t, err)

	WaitForInvalidTestResult(t, ctx, apex, config, user, txHash, beforeSendingAmountDfm, waitForAmount,
		refundEnabled, maxWaitTimeSec, retryIntervalSec)
}

func executeInvalidMetadataInvalidSender(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, config *testConfig, user *cardanofw.TestApexUser,
	maxWaitTimeSec uint, addrIndex uint8,
) {
	t.Helper()

	receivers := createReceivers(apex, 1, config.dstChainID, defaultSendAmount, config.tokenID)

	srcTestChain := apex.GetChainMust(t, config.srcChainID)

	operationFee := apex.GetMinOperationFee(config.srcChainID)

	feeAmount, err := srcTestChain.GetBridgingFee(
		ctx, config.dstChainID, receivers,
		apex.GetMinBridgingFee(config.srcChainID, !config.isCurrency),
		operationFee, config.srcMultiSigAddr)
	require.NoError(t, err)

	metadata, err := srcTestChain.CreateMetadata(
		"dummy", config.dstChainID, receivers, feeAmount,
		operationFee,
	)
	require.NoError(t, err)

	// remove this after we make correct validation on oracle!
	metadata = bytes.Replace(metadata, []byte("[\"dummy\"]"), []byte("\"\""), 1)

	lovelaceAmount, sentTokenAmount, _ := getDefaultSendAmounts(t, config, feeAmount, operationFee)

	txHash, err := apex.SubmitTx(
		ctx, config.srcChainID, user, apex.GetCardanoInfo(config.srcChainID).MultisigAddr[addrIndex],
		lovelaceAmount, sentTokenAmount, metadata, new(big.Int).SetUint64(operationFee))
	require.NoError(t, err)

	cardanofw.WaitForInvalidState(t, ctx, apex, config.srcChainID, txHash, apex.Config.APIKey, maxWaitTimeSec)
}

func executeInvalidEmptyReceivers(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, config *testConfig, user *cardanofw.TestApexUser,
	maxWaitTimeSec, retryIntervalSec uint, refundEnabled bool, addrIndex uint8,
) {
	t.Helper()

	receivers := []sendtx.BridgingTxReceiver{}

	receiversForFeeCalculation := []sendtx.BridgingTxReceiver{
		{
			Addr:    user.GetAddress(config.dstChainID),
			Amount:  defaultSendAmount,
			TokenID: config.tokensInfo.SrcTokenID,
		},
	}

	srcTestChain := apex.GetChainMust(t, config.srcChainID)

	operationFee := apex.GetMinOperationFee(config.srcChainID)

	feeAmount, err := srcTestChain.GetBridgingFee(
		ctx, config.dstChainID, receiversForFeeCalculation,
		apex.GetMinBridgingFee(config.srcChainID, !config.isCurrency),
		operationFee, config.srcMultiSigAddr)
	require.NoError(t, err)

	metadata, err := srcTestChain.CreateMetadata(
		user.GetAddress(config.srcChainID), config.dstChainID, receivers, feeAmount, operationFee)
	require.NoError(t, err)

	beforeSendingAmountDfm, err := apex.GetBalance(ctx, user, config.srcChainID)
	require.NoError(t, err)

	lovelaceAmount, sentTokenAmount, waitForAmount := getDefaultSendAmounts(
		t, config, feeAmount, operationFee)

	txHash, err := apex.SubmitTx(
		ctx, config.srcChainID, user, apex.GetCardanoInfo(config.srcChainID).MultisigAddr[addrIndex],
		lovelaceAmount, sentTokenAmount, metadata, new(big.Int).SetUint64(operationFee))
	require.NoError(t, err)

	WaitForInvalidTestResult(t, ctx, apex, config, user, txHash, beforeSendingAmountDfm, waitForAmount,
		refundEnabled, maxWaitTimeSec, retryIntervalSec)
}

func executeInvalidTokenDirection(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, config *testConfig,
	invalidTokenID uint16, user *cardanofw.TestApexUser,
	maxWaitTimeSec, retryIntervalSec uint, refundEnabled bool, addrIndex uint8,
) {
	t.Helper()

	receivers := createReceivers(apex, 1, config.dstChainID, defaultSendAmount, invalidTokenID)

	operationFee := apex.GetMinOperationFee(config.srcChainID)

	metadata, feeAmount := createMetadata(t, ctx, apex, config.srcChainID, config.dstChainID,
		apex.GetMinBridgingFee(config.srcChainID, false),
		operationFee, user, receivers, config.isCurrency)

	beforeSendingAmountDfm, err := apex.GetBalance(ctx, user, config.srcChainID)
	require.NoError(t, err)

	lovelaceAmount, sentTokenAmount, waitForAmount := getDefaultSendAmounts(
		t, config, feeAmount, operationFee)

	txHash, err := apex.SubmitTx(
		ctx, config.srcChainID, user, apex.GetCardanoInfo(config.srcChainID).MultisigAddr[addrIndex],
		lovelaceAmount, sentTokenAmount, metadata, new(big.Int).SetUint64(operationFee))
	require.NoError(t, err)

	WaitForInvalidTestResult(t, ctx, apex, config, user, txHash, beforeSendingAmountDfm, waitForAmount,
		refundEnabled, maxWaitTimeSec, retryIntervalSec)
}

func getDefaultSendAmounts(
	t *testing.T, config *testConfig,
	feeAmount uint64, operationFee uint64,
) (*big.Int, []wallet.TokenAmount, uint64) {
	t.Helper()

	lovelaceAmount := defaultSendAmount + feeAmount
	waitForAmount := lovelaceAmount + operationFee

	tokens := []wallet.TokenAmount(nil)

	if !config.isCurrency {
		waitForAmount = defaultSendAmount
		lovelaceAmount = feeAmount

		token, err := wallet.NewTokenWithFullName(config.tokensInfo.SrcTokenName, true)
		require.NoError(t, err)

		tokens = []wallet.TokenAmount{{
			Token:  token,
			Amount: defaultSendAmount,
		}}
	}

	return new(big.Int).SetUint64(lovelaceAmount), tokens, waitForAmount
}

func createMetadata(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem,
	srcChain, dstChain cardanofw.ChainID, bridgingFee, operationFee uint64,
	sender *cardanofw.TestApexUser, receivers []sendtx.BridgingTxReceiver,
	isCurrency bool,
) ([]byte, uint64) {
	t.Helper()

	srcTestChain := apex.GetChainMust(t, srcChain)

	multisig, err := srcTestChain.GetAddressToBridgeTo(ctx, !isCurrency)
	require.NoError(t, err)

	feeAmount, err := srcTestChain.GetBridgingFee(ctx, dstChain, receivers, bridgingFee, operationFee, multisig)
	require.NoError(t, err)

	metadata, err := srcTestChain.CreateMetadata(sender.GetAddress(srcChain), dstChain, receivers, feeAmount, operationFee)
	require.NoError(t, err)

	return metadata, feeAmount
}

func createObsoleteMetadata(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem,
	srcChain, dstChain cardanofw.ChainID, bridgingFee, operationFee uint64,
	sender *cardanofw.TestApexUser, receivers []sendtx.BridgingTxReceiver, isCurrency bool,
) ([]byte, uint64) {
	t.Helper()

	srcTestChain := apex.GetChainMust(t, srcChain)

	multisig, err := srcTestChain.GetAddressToBridgeTo(ctx, !isCurrency)
	require.NoError(t, err)

	feeAmount, err := srcTestChain.GetBridgingFee(ctx, dstChain, receivers, bridgingFee, operationFee, multisig)
	require.NoError(t, err)

	txs := make([]BridgingRequestMetadataTransactionBC, len(receivers))
	boolToByte := map[bool]byte{true: 1, false: 0}

	for i, x := range receivers {
		txs[i] = BridgingRequestMetadataTransactionBC{
			Address:                     sendtx.AddrToMetaDataAddr(x.Addr),
			IsNativeTokenOnSrc_Obsolete: boolToByte[!isCurrency],
			Amount:                      x.Amount,
			TokenID:                     0,
		}
	}

	metadata := BridgingRequestMetadataBC{
		BridgingTxType:     bridgingMetaDataType,
		DestinationChainID: dstChain,
		SenderAddr:         sendtx.AddrToMetaDataAddr(sender.GetAddress(srcChain)),
		Transactions:       txs,
		BridgingFee:        feeAmount,
		OperationFee:       operationFee,
	}

	metadataBytes, err := json.Marshal(map[int]BridgingRequestMetadataBC{
		metadataMapKey: metadata,
	})
	require.NoError(t, err)

	return metadataBytes, feeAmount
}

func createReceivers(
	apex *cardanofw.ApexSystem, receiversCount int, dstChain string, sendAmount uint64, tokenID uint16,
) []sendtx.BridgingTxReceiver {
	receivers := make([]sendtx.BridgingTxReceiver, receiversCount)

	for i := range receivers {
		receivers[i] = sendtx.BridgingTxReceiver{
			Addr:    apex.Users[len(apex.Users)-1-i].GetAddress(dstChain),
			Amount:  sendAmount,
			TokenID: tokenID,
		}
	}

	return receivers
}
