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

type RefundOption int

const (
	RefundDisabled RefundOption = iota
	RefundEnabled
	RefundDisabledTimeout
)

const (
	bridgingMetaDataType sendtx.BridgingRequestType = "bridge"
	metadataMapKey       int                        = 1
)

type colCoinInvalidOpts struct {
	receivers        []sendtx.BridgingTxReceiver
	amount           uint64
	refundOption     RefundOption
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
	srcTokenName    string
}

const (
	defaultSendAmount = uint64(1_000_000)
)

func newTestConfig(
	t *testing.T, config *cardanofw.TestCardanoChainConfig, info *cardanofw.CardanoChainInfo,
	dstChainID cardanofw.ChainID, srcTokenName string,
) *testConfig {
	t.Helper()

	txProvider, err := info.GetTxProvider()
	require.NoError(t, err)

	return &testConfig{
		srcChainID:      config.ChainType,
		dstChainID:      dstChainID,
		srcNetworkType:  config.NetworkType,
		srcTxProvider:   txProvider,
		srcMinterWallet: info.GenesisWallet,
		srcMultiSigAddr: info.MultisigAddr[0],
		srcTokenName:    srcTokenName,
	}
}

// Util methods
func WaitForTestResult(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, config *testConfig, user *cardanofw.TestApexUser,
	txHash string, beforeSendingAmountDfm map[string]*big.Int, sentAmount uint64, bridgingType cardanofw.BridgingType,
	refundEnabled bool, maxWaitTimeSec, retryIntervalSec uint, coloredCoins ...uint16,
) {
	t.Helper()

	retryIntervalSec = max(retryIntervalSec, 1)
	numRetries := max(1, int(maxWaitTimeSec/retryIntervalSec))

	if refundEnabled {
		tokenName := wallet.AdaTokenName

		if bridgingType == cardanofw.BridgingTypeWrappedTokenOnSource {
			tokenName = config.srcTokenName
		}

		if bridgingType == cardanofw.BridgingTypeColoredCoinOnSource {
			tokensInfo := apex.GetBridgingTokensInfo(config.srcChainID, config.dstChainID, bridgingType, coloredCoins...)
			require.NotNil(t, tokensInfo)

			tokenName = tokensInfo.SrcTokenName
		}

		lowerBoundaryDfm := new(big.Int).Sub(beforeSendingAmountDfm[tokenName], new(big.Int).SetUint64(sentAmount))

		fmt.Printf("Tx sent. hash: %s, lowerBoundaryDfm: %d, higherBoundaryDfm: %+v\n", txHash, lowerBoundaryDfm,
			beforeSendingAmountDfm)

		err := apex.WaitForAmountInRange(ctx, user, config.srcChainID, config.dstChainID, lowerBoundaryDfm,
			beforeSendingAmountDfm[tokenName], numRetries, time.Second*time.Duration(retryIntervalSec),
			tokenName)
		require.NoError(t, err)
	} else {
		cardanofw.WaitForInvalidState(t, ctx, apex, config.srcChainID, txHash, apex.Config.APIKey, maxWaitTimeSec)
	}
}

// Test methods
func submitMismatchAndWait(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, config *testConfig, user *cardanofw.TestApexUser,
	metadata []byte, lovelaceAmount *big.Int, sentTokenAmount []wallet.TokenAmount, waitForAmount uint64,
	bridgingType cardanofw.BridgingType, refundOption RefundOption, maxWaitTimeSec, retryIntervalSec uint, addrIndex uint8,
	coloredCoins ...uint16,
) {
	t.Helper()

	beforeSendingAmountDfm, err := apex.GetBalance(ctx, user, config.srcChainID)
	require.NoError(t, err)

	txHash, err := apex.SubmitTx(
		ctx, config.srcChainID, user, apex.GetCardanoInfo(config.srcChainID).MultisigAddr[addrIndex],
		lovelaceAmount, sentTokenAmount, metadata)
	require.NoError(t, err)

	if refundOption == RefundDisabledTimeout {
		_, err = cardanofw.WaitForRequestStates(ctx, apex, config.srcChainID, txHash, apex.Config.APIKey, nil, maxWaitTimeSec)
		require.Error(t, err)
		require.ErrorContains(t, err, "timeout")
	} else {
		WaitForTestResult(t, ctx, apex, config, user, txHash, beforeSendingAmountDfm, waitForAmount,
			bridgingType, refundOption == RefundEnabled, maxWaitTimeSec, retryIntervalSec, coloredCoins...)
	}
}

func submitColCoinsMismatchAndWait(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, config *testConfig, user *cardanofw.TestApexUser,
	receivers []sendtx.BridgingTxReceiver, amount uint64, tokenID uint16, bridgingType cardanofw.BridgingType,
	maxWaitTimeSec, retryIntervalSec uint, addrIndex uint8,
	refundOption RefundOption, metadataModifier func([]byte) []byte,
) {
	t.Helper()

	operationFee := apex.GetMinOperationFee(config.srcChainID)

	metadata, feeAmount := createMetadata(t, ctx, apex, config.srcChainID, config.dstChainID,
		apex.GetMinBridgingFee(config.srcChainID, bridgingType == cardanofw.BridgingTypeColoredCoinOnSource),
		operationFee,
		user, receivers, bridgingType)

	if metadataModifier != nil {
		metadata = metadataModifier(metadata)
	}

	waitForAmount := amount
	lovelaceAmount := new(big.Int).SetUint64(feeAmount + operationFee)

	tokensInfo := apex.GetBridgingTokensInfo(
		config.srcChainID, config.dstChainID, cardanofw.BridgingTypeColoredCoinOnSource, []uint16{tokenID}...)

	token, err := wallet.NewTokenWithFullName(tokensInfo.SrcTokenName, true)
	require.NoError(t, err)

	sentTokenAmount := []wallet.TokenAmount{{
		Token:  token,
		Amount: amount,
	}}

	submitMismatchAndWait(t, ctx, apex, config, user, metadata, lovelaceAmount, sentTokenAmount, waitForAmount,
		cardanofw.BridgingTypeColoredCoinOnSource, refundOption, maxWaitTimeSec, retryIntervalSec, addrIndex, tokenID)
}

func executeInvalidMismatchSendLovelaceAmount(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, config *testConfig, user *cardanofw.TestApexUser,
	maxWaitTimeSec, retryIntervalSec uint, bridgingType cardanofw.BridgingType, refundEnabled bool, addrIndex uint8,
) {
	t.Helper()

	receivers := createReceivers(t, apex, 1, config.srcChainID, config.dstChainID, defaultSendAmount*10, bridgingType)

	operationFee := apex.GetMinOperationFee(config.srcChainID)

	metadata, feeAmount := createMetadata(t, ctx, apex, config.srcChainID, config.dstChainID,
		apex.GetMinBridgingFee(config.srcChainID, bridgingType == cardanofw.BridgingTypeWrappedTokenOnSource),
		operationFee,
		user, receivers, bridgingType)

	lovelaceAmount, sentTokenAmount, waitForAmount := getDefaultSendAmounts(
		t, config, feeAmount, operationFee, bridgingType)

	refundOption := RefundDisabled
	if refundEnabled {
		refundOption = RefundEnabled
	}

	submitMismatchAndWait(t, ctx, apex, config, user, metadata, lovelaceAmount, sentTokenAmount, waitForAmount,
		bridgingType, refundOption, maxWaitTimeSec, retryIntervalSec, addrIndex)
}

func executeInvalidColCoin(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, config *testConfig, user *cardanofw.TestApexUser,
	tokenID uint16, maxWaitTimeSec, retryIntervalSec uint, addrIndex uint8, opts colCoinInvalidOpts,
) {
	t.Helper()

	submitColCoinsMismatchAndWait(
		t, ctx, apex, config, user, opts.receivers, opts.amount, tokenID, cardanofw.BridgingTypeColoredCoinOnSource,
		maxWaitTimeSec, retryIntervalSec, addrIndex, opts.refundOption, opts.metadataModifier)
}

func executeInvalidMismatchSendColCoinsMultipleInstancesParalel(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, config *testConfig, amount uint64, tokenID uint16,
	instances int, maxWaitTimeSec, retryIntervalSec uint, refundEnabled bool,
	addrIndex uint8,
) {
	t.Helper()

	bridgingType := cardanofw.BridgingTypeColoredCoinOnSource

	var wg sync.WaitGroup

	for i := range instances {
		wg.Add(1)

		go func(idx int) {
			defer wg.Done()

			receivers := createReceiversColCoin(apex, 1, config.dstChainID, amount*10, tokenID)

			refundOption := RefundDisabled
			if refundEnabled {
				refundOption = RefundEnabled
			}

			submitColCoinsMismatchAndWait(t, ctx, apex, config, apex.Users[idx], receivers, amount, tokenID, bridgingType,
				maxWaitTimeSec, retryIntervalSec, addrIndex, refundOption, nil)
		}(i)
	}

	wg.Wait()
}

func executeInvalidMismatchSendAmountMultipleInstances(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, config *testConfig,
	maxWaitTimeSec, retryIntervalSec uint, bridgingType cardanofw.BridgingType, refundEnabled bool, addrIndex uint8,
) {
	t.Helper()

	const instances = 5

	for i := 0; i < instances; i++ {
		receivers := createReceivers(t, apex, 1, config.srcChainID, config.dstChainID, defaultSendAmount*10, bridgingType)

		operationFee := apex.GetMinOperationFee(config.srcChainID)

		metadata, feeAmount := createMetadata(t, ctx, apex, config.srcChainID, config.dstChainID,
			apex.GetMinBridgingFee(config.srcChainID, bridgingType == cardanofw.BridgingTypeWrappedTokenOnSource),
			operationFee,
			apex.Users[i], receivers, bridgingType)

		beforeSendingAmountDfm, err := apex.GetBalance(ctx, apex.Users[i], config.srcChainID)
		require.NoError(t, err)

		lovelaceAmount, sentTokenAmount, waitForAmount := getDefaultSendAmounts(
			t, config, feeAmount, operationFee, bridgingType)

		txHash, err := apex.SubmitTx(
			ctx, config.srcChainID, apex.Users[i],
			apex.GetCardanoInfo(config.srcChainID).MultisigAddr[addrIndex], lovelaceAmount, sentTokenAmount, metadata)
		require.NoError(t, err)

		WaitForTestResult(t, ctx, apex, config, apex.Users[i], txHash, beforeSendingAmountDfm, waitForAmount,
			bridgingType, refundEnabled, maxWaitTimeSec, retryIntervalSec)
	}
}

func executeInvalidMismatchSendAmountMultipleInstancesParalel(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, config *testConfig,
	maxWaitTimeSec, retryIntervalSec uint, bridgingType cardanofw.BridgingType, refundEnabled bool,
	addrIndex uint8,
) {
	t.Helper()

	instances := 5

	var wg sync.WaitGroup

	for i := 0; i < instances; i++ {
		wg.Add(1)

		go func(idx int) {
			defer wg.Done()

			receivers := createReceivers(t, apex, 1, config.srcChainID, config.dstChainID, defaultSendAmount*10, bridgingType)

			operationFee := apex.GetMinOperationFee(config.srcChainID)

			metadata, feeAmount := createMetadata(t, ctx, apex, config.srcChainID, config.dstChainID,
				apex.GetMinBridgingFee(config.srcChainID, bridgingType == cardanofw.BridgingTypeWrappedTokenOnSource),
				operationFee,
				apex.Users[i], receivers, bridgingType)

			beforeSendingAmountDfm, err := apex.GetBalance(ctx, apex.Users[idx], config.srcChainID)
			require.NoError(t, err)

			lovelaceAmount, sentTokenAmount, waitForAmount := getDefaultSendAmounts(
				t, config, feeAmount, operationFee, bridgingType)

			txHashe, err := apex.SubmitTx(
				ctx, config.srcChainID, apex.Users[idx],
				apex.GetCardanoInfo(config.srcChainID).MultisigAddr[addrIndex], lovelaceAmount, sentTokenAmount, metadata)
			require.NoError(t, err)

			WaitForTestResult(t, ctx, apex, config, apex.Users[idx], txHashe, beforeSendingAmountDfm, waitForAmount,
				bridgingType, refundEnabled, maxWaitTimeSec, retryIntervalSec)
		}(i)
	}

	wg.Wait()
}

func executeInvalidMetadataType(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, config *testConfig, user *cardanofw.TestApexUser,
	maxWaitTimeSec, retryIntervalSec uint, bridgingType cardanofw.BridgingType, refundEnabled bool, addrIndex uint8,
) {
	t.Helper()

	receivers := createReceivers(t, apex, 1, config.srcChainID, config.dstChainID, defaultSendAmount, bridgingType)

	operationFee := apex.GetMinOperationFee(config.srcChainID)

	metadata, feeAmount := createMetadata(t, ctx, apex, config.srcChainID, config.dstChainID,
		apex.GetMinBridgingFee(config.srcChainID, bridgingType == cardanofw.BridgingTypeWrappedTokenOnSource),
		operationFee,
		user, receivers, bridgingType)
	metadata = bytes.Replace(metadata, []byte("bridge"), []byte("xxxxx"), 1)

	beforeSendingAmountDfm, err := apex.GetBalance(ctx, user, config.srcChainID)
	require.NoError(t, err)

	lovelaceAmount, sentTokenAmount, waitForAmount := getDefaultSendAmounts(
		t, config, feeAmount, operationFee, bridgingType)

	txHash, err := apex.SubmitTx(
		ctx, config.srcChainID, user, apex.GetCardanoInfo(config.srcChainID).MultisigAddr[addrIndex],
		lovelaceAmount, sentTokenAmount, metadata)
	require.NoError(t, err)

	if refundEnabled {
		WaitForTestResult(t, ctx, apex, config, user, txHash, beforeSendingAmountDfm, waitForAmount,
			bridgingType, refundEnabled, maxWaitTimeSec, retryIntervalSec)
	} else {
		_, err = cardanofw.WaitForRequestStates(ctx, apex, config.srcChainID, txHash, apex.Config.APIKey, nil, maxWaitTimeSec)
		require.Error(t, err)
		require.ErrorContains(t, err, "timeout")
	}
}

func executeObsoleteMetadata(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, config *testConfig, user *cardanofw.TestApexUser,
	maxWaitTimeSec, retryIntervalSec uint, bridgingType cardanofw.BridgingType, addrIndex uint8,
) {
	t.Helper()

	receivers := createReceivers(t, apex, 1, config.srcChainID, config.dstChainID, defaultSendAmount, bridgingType)

	operationFee := apex.GetMinOperationFee(config.srcChainID)

	metadata, feeAmount := createObsoleteMetadata(t, ctx, apex, config.srcChainID, config.dstChainID,
		apex.GetMinBridgingFee(config.srcChainID, bridgingType == cardanofw.BridgingTypeWrappedTokenOnSource),
		operationFee,
		user, receivers, bridgingType)

	tokensInfo := apex.GetBridgingTokensInfo(config.srcChainID, config.dstChainID, bridgingType)
	require.NotNil(t, tokensInfo)

	balance, err := apex.GetBalanceWithTokenName(
		ctx, apex.Users[len(apex.Users)-1], config.dstChainID, tokensInfo.DstTokenName)
	fmt.Printf("Receiver balance: %+v\n", balance)
	require.NoError(t, err)

	lovelaceAmount, sentTokenAmount, _ := getDefaultSendAmounts(
		t, config, feeAmount, operationFee, bridgingType)

	txHash, err := apex.SubmitTx(
		ctx, config.srcChainID, user, apex.GetCardanoInfo(config.srcChainID).MultisigAddr[addrIndex],
		lovelaceAmount, sentTokenAmount, metadata)
	require.NoError(t, err)

	require.NoError(t, err)

	fmt.Printf("Tx sent. hash: %s\n", txHash)

	currentAmount, ok := balance[tokensInfo.DstTokenName]
	if !ok {
		currentAmount = big.NewInt(0)
	}

	expectedAmount := new(big.Int).Add(currentAmount, big.NewInt(int64(defaultSendAmount)))

	numRetries := max(1, int(maxWaitTimeSec/retryIntervalSec))

	err = apex.WaitForExactAmount(ctx, user, config.dstChainID, config.srcChainID, expectedAmount,
		numRetries, time.Second*time.Duration(retryIntervalSec), tokensInfo.DstTokenName)

	require.NoError(t, err)
}

func executeInvalidDestination(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, config *testConfig, user *cardanofw.TestApexUser,
	maxWaitTimeSec, retryIntervalSec uint, bridgingType cardanofw.BridgingType, refundEnabled bool, addrIndex uint8,
) {
	t.Helper()

	tokensInfo := apex.GetBridgingTokensInfo(config.srcChainID, config.dstChainID, bridgingType)
	require.NotNil(t, tokensInfo)

	receivers := createReceivers(t, apex, 0, config.srcChainID, config.dstChainID, defaultSendAmount, bridgingType)
	receiversForFeeCalculation := []sendtx.BridgingTxReceiver{
		{
			Addr:    user.GetAddress(config.dstChainID),
			Amount:  defaultSendAmount,
			TokenID: tokensInfo.SrcTokenID,
		},
	}

	srcTestChain := apex.GetChainMust(t, config.srcChainID)

	operationFee := apex.GetMinOperationFee(config.srcChainID)

	feeAmount, err := srcTestChain.GetBridgingFee(
		ctx, config.dstChainID, receiversForFeeCalculation,
		apex.GetMinBridgingFee(config.srcChainID, bridgingType == cardanofw.BridgingTypeWrappedTokenOnSource),
		operationFee,
		config.srcMultiSigAddr)
	require.NoError(t, err)

	metadata, err := srcTestChain.CreateMetadata(
		user.GetAddress(config.srcChainID), config.dstChainID, receivers, feeAmount,
		operationFee,
	)
	require.NoError(t, err)

	metadata = bytes.Replace(metadata, fmt.Appendf(nil, "\"%s\"", config.dstChainID), []byte("\"unknown\""), 1)

	beforeSendingAmountDfm, err := apex.GetBalanceWithTokenName(ctx, user, config.srcChainID, tokensInfo.SrcTokenName)
	require.NoError(t, err)

	lovelaceAmount, sentTokenAmount, waitForAmount := getDefaultSendAmounts(
		t, config, feeAmount, operationFee, bridgingType)

	txHash, err := apex.SubmitTx(
		ctx, config.srcChainID, user, apex.GetCardanoInfo(config.srcChainID).MultisigAddr[addrIndex],
		lovelaceAmount, sentTokenAmount, metadata)
	require.NoError(t, err)

	WaitForTestResult(t, ctx, apex, config, user, txHash, beforeSendingAmountDfm, waitForAmount,
		bridgingType, refundEnabled, maxWaitTimeSec, retryIntervalSec)
}

func executeInvalidMetadataInvalidSender(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, config *testConfig, user *cardanofw.TestApexUser,
	maxWaitTimeSec uint, bridgingType cardanofw.BridgingType, addrIndex uint8,
) {
	t.Helper()

	receivers := createReceivers(t, apex, 1, config.srcChainID, config.dstChainID, defaultSendAmount, bridgingType)

	srcTestChain := apex.GetChainMust(t, config.srcChainID)

	operationFee := apex.GetMinOperationFee(config.srcChainID)

	feeAmount, err := srcTestChain.GetBridgingFee(
		ctx, config.dstChainID, receivers,
		apex.GetMinBridgingFee(config.srcChainID, bridgingType == cardanofw.BridgingTypeWrappedTokenOnSource),
		operationFee,
		config.srcMultiSigAddr)
	require.NoError(t, err)

	metadata, err := srcTestChain.CreateMetadata(
		"dummy", config.dstChainID, receivers, feeAmount,
		operationFee,
	)
	require.NoError(t, err)

	// remove this after we make correct validation on oracle!
	metadata = bytes.Replace(metadata, []byte("[\"dummy\"]"), []byte("\"\""), 1)

	lovelaceAmount, sentTokenAmount, _ := getDefaultSendAmounts(t, config, feeAmount, operationFee, bridgingType)

	txHash, err := apex.SubmitTx(
		ctx, config.srcChainID, user, apex.GetCardanoInfo(config.srcChainID).MultisigAddr[addrIndex],
		lovelaceAmount, sentTokenAmount, metadata)
	require.NoError(t, err)

	cardanofw.WaitForInvalidState(t, ctx, apex, config.srcChainID, txHash, apex.Config.APIKey, maxWaitTimeSec)
}

func executeInvalidEmptyReceivers(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, config *testConfig, user *cardanofw.TestApexUser,
	maxWaitTimeSec, retryIntervalSec uint, bridgingType cardanofw.BridgingType, refundEnabled bool, addrIndex uint8,
) {
	t.Helper()

	receivers := []sendtx.BridgingTxReceiver{}

	tokensInfo := apex.GetBridgingTokensInfo(config.srcChainID, config.dstChainID, bridgingType)
	require.NotNil(t, tokensInfo)

	receiversForFeeCalculation := []sendtx.BridgingTxReceiver{
		{
			Addr:    user.GetAddress(config.dstChainID),
			Amount:  defaultSendAmount,
			TokenID: tokensInfo.SrcTokenID,
		},
	}

	srcTestChain := apex.GetChainMust(t, config.srcChainID)

	operationFee := apex.GetMinOperationFee(config.srcChainID)

	feeAmount, err := srcTestChain.GetBridgingFee(
		ctx, config.dstChainID, receiversForFeeCalculation,
		apex.GetMinBridgingFee(config.srcChainID, bridgingType == cardanofw.BridgingTypeWrappedTokenOnSource),
		operationFee,
		config.srcMultiSigAddr)
	require.NoError(t, err)

	metadata, err := srcTestChain.CreateMetadata(
		user.GetAddress(config.srcChainID), config.dstChainID, receivers, feeAmount,
		operationFee)
	require.NoError(t, err)

	beforeSendingAmountDfm, err := apex.GetBalance(ctx, user, config.srcChainID)
	require.NoError(t, err)

	lovelaceAmount, sentTokenAmount, waitForAmount := getDefaultSendAmounts(
		t, config, feeAmount, operationFee, bridgingType)

	txHash, err := apex.SubmitTx(
		ctx, config.srcChainID, user, apex.GetCardanoInfo(config.srcChainID).MultisigAddr[addrIndex],
		lovelaceAmount, sentTokenAmount, metadata)
	require.NoError(t, err)

	WaitForTestResult(t, ctx, apex, config, user, txHash, beforeSendingAmountDfm, waitForAmount,
		bridgingType, refundEnabled, maxWaitTimeSec, retryIntervalSec)
}

func executeInvalidTokenDirection(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, config *testConfig, user *cardanofw.TestApexUser,
	maxWaitTimeSec, retryIntervalSec uint, bridgingType cardanofw.BridgingType, refundEnabled bool, addrIndex uint8,
) {
	t.Helper()

	invalidBridgingType := cardanofw.BridgingTypeCurrencyOnSource
	if bridgingType == cardanofw.BridgingTypeCurrencyOnSource {
		invalidBridgingType = cardanofw.BridgingTypeWrappedTokenOnSource
	}

	receivers := createReceivers(t, apex, 1, config.srcChainID, config.dstChainID, defaultSendAmount, invalidBridgingType)

	operationFee := apex.GetMinOperationFee(config.srcChainID)

	metadata, feeAmount := createMetadata(t, ctx, apex, config.srcChainID, config.dstChainID,
		apex.GetMinBridgingFee(config.srcChainID, false),
		operationFee,
		user, receivers, bridgingType)

	beforeSendingAmountDfm, err := apex.GetBalance(ctx, user, config.srcChainID)
	require.NoError(t, err)

	lovelaceAmount, sentTokenAmount, waitForAmount := getDefaultSendAmounts(
		t, config, feeAmount, operationFee, bridgingType)

	txHash, err := apex.SubmitTx(
		ctx, config.srcChainID, user, apex.GetCardanoInfo(config.srcChainID).MultisigAddr[addrIndex],
		lovelaceAmount, sentTokenAmount, metadata)
	require.NoError(t, err)

	WaitForTestResult(t, ctx, apex, config, user, txHash, beforeSendingAmountDfm, waitForAmount, bridgingType,
		refundEnabled, maxWaitTimeSec, retryIntervalSec)
}

func getDefaultSendAmounts(
	t *testing.T, config *testConfig,
	feeAmount uint64, operationFee uint64, bridgingType cardanofw.BridgingType,
) (*big.Int, []wallet.TokenAmount, uint64) {
	t.Helper()

	lovelaceAmount := defaultSendAmount + feeAmount + operationFee
	waitForAmount := lovelaceAmount

	tokens := []wallet.TokenAmount(nil)

	if bridgingType == cardanofw.BridgingTypeWrappedTokenOnSource ||
		bridgingType == cardanofw.BridgingTypeColoredCoinOnSource {
		waitForAmount = defaultSendAmount
		lovelaceAmount = feeAmount + operationFee

		token, err := wallet.NewTokenWithFullName(config.srcTokenName, true)
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
	bridgingType cardanofw.BridgingType,
) ([]byte, uint64) {
	t.Helper()

	srcTestChain := apex.GetChainMust(t, srcChain)

	multisig, err := srcTestChain.GetAddressToBridgeTo(ctx, bridgingType)
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
	sender *cardanofw.TestApexUser, receivers []sendtx.BridgingTxReceiver,
	bridgingType cardanofw.BridgingType,
) ([]byte, uint64) {
	t.Helper()

	srcTestChain := apex.GetChainMust(t, srcChain)

	multisig, err := srcTestChain.GetAddressToBridgeTo(ctx, bridgingType)
	require.NoError(t, err)

	feeAmount, err := srcTestChain.GetBridgingFee(ctx, dstChain, receivers, bridgingFee, operationFee, multisig)
	require.NoError(t, err)

	txs := make([]BridgingRequestMetadataTransactionBC, len(receivers))
	boolToByte := map[bool]byte{true: 1, false: 0}

	for i, x := range receivers {
		txs[i] = BridgingRequestMetadataTransactionBC{
			Address:                     sendtx.AddrToMetaDataAddr(x.Addr),
			IsNativeTokenOnSrc_Obsolete: boolToByte[bridgingType == cardanofw.BridgingTypeWrappedTokenOnSource],
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

func createReceiversCore(
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

func createReceiversColCoin(
	apex *cardanofw.ApexSystem, receiversCount int, dstChain string, sendAmount uint64, tokenID uint16,
) []sendtx.BridgingTxReceiver {
	return createReceiversCore(apex, receiversCount, dstChain, sendAmount, tokenID)
}

func createReceivers(
	t *testing.T, apex *cardanofw.ApexSystem, receiversCount int, srcChain string, dstChain string,
	sendAmount uint64, bridgingType cardanofw.BridgingType,
) []sendtx.BridgingTxReceiver {
	t.Helper()

	tokensInfo := apex.GetBridgingTokensInfo(srcChain, dstChain, bridgingType)
	require.NotNil(t, tokensInfo)

	return createReceiversCore(apex, receiversCount, dstChain, sendAmount, tokensInfo.SrcTokenID)
}
