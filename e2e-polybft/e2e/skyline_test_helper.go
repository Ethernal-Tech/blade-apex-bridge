package e2e

import (
	"bytes"
	"context"
	"fmt"
	"math/big"
	"strconv"
	"testing"

	"github.com/0xPolygon/polygon-edge/e2e-polybft/cardanofw"
	"github.com/Ethernal-Tech/cardano-infrastructure/sendtx"
	"github.com/Ethernal-Tech/cardano-infrastructure/wallet"
	"github.com/stretchr/testify/require"
)

func executeInvalidBridgingFee(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem,
	config *testConfig, maxWaitTimeSec, retryIntervalSec uint, bridgingType cardanofw.BridgingType, refundEnabled bool,
	addrIndex uint8,
) {
	t.Helper()

	user := apex.Users[len(apex.Users)-1]
	receivers := createReceivers(apex, 1, config.srcChainID, config.dstChainID, defaultSendAmount, bridgingType)

	operationFee := apex.GetMinOperationFee(config.srcChainID)
	minBridgingFee := apex.GetMinBridgingFee(config.srcChainID, bridgingType == cardanofw.BridgingTypeWrappedTokenOnSource)

	metadata, feeAmount := createMetadata(t, ctx, apex, config.srcChainID, config.dstChainID,
		minBridgingFee, operationFee, user, receivers, bridgingType)
	bytesToReplace := []byte(strconv.FormatUint(feeAmount, 10))
	metadata = bytes.ReplaceAll(metadata, bytesToReplace, []byte(fmt.Sprintf("%d", minBridgingFee-1)))

	beforeSendingAmountDfm, err := apex.GetBalance(ctx, user, config.srcChainID)
	require.NoError(t, err)

	lovelaceAmount, sentTokenAmount, waitForAmount := getDefaultSendAmounts(
		t, config, feeAmount, operationFee, bridgingType)

	txHash, err := apex.SubmitTx(
		ctx, config.srcChainID, user, apex.GetCardanoInfo(config.srcChainID).MultisigAddr[addrIndex],
		lovelaceAmount, sentTokenAmount, metadata)
	require.NoError(t, err)

	fmt.Printf("txHash: %s\n", txHash)

	WaitForTestResult(t, ctx, apex, config, user, txHash, beforeSendingAmountDfm, waitForAmount,
		bridgingType, refundEnabled, maxWaitTimeSec, retryIntervalSec)
}

func executeInvalidFeeReceiverAddr(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem,
	config *testConfig, maxWaitTimeSec, retryIntervalSec uint, bridgingType cardanofw.BridgingType, refundEnabled bool,
	addrIndex uint8,
) {
	t.Helper()

	operationFee := apex.GetMinOperationFee(config.srcChainID)
	minBridgingFee := apex.GetMinBridgingFee(config.srcChainID, bridgingType == cardanofw.BridgingTypeWrappedTokenOnSource)
	tokenID := apex.GetTokenIDForChain(config.srcChainID, bridgingType == cardanofw.BridgingTypeCurrencyOnSource)
	require.NotZero(t, tokenID)

	user := apex.Users[len(apex.Users)-1]
	receivers := []sendtx.BridgingTxReceiver{
		{
			Addr:    apex.GetCardanoInfo(config.dstChainID).FeeAddr,
			Amount:  minBridgingFee,
			TokenID: tokenID,
		},
	}

	metadata, feeAmount := createMetadata(t, ctx, apex, config.srcChainID, config.dstChainID,
		minBridgingFee, operationFee, user, receivers, bridgingType)

	sentAmount, sentTokenAmount, _ := getDefaultSendAmounts(t, config, feeAmount, operationFee, bridgingType)

	initialBalances, err := apex.GetBalance(ctx, user, config.srcChainID)
	require.NoError(t, err)

	if bridgingType == cardanofw.BridgingTypeCurrencyOnSource {
		txHash, err := apex.SubmitTx(
			ctx, config.srcChainID, user, apex.GetCardanoInfo(config.srcChainID).MultisigAddr[addrIndex],
			sentAmount, nil, metadata)
		require.NoError(t, err)

		fmt.Printf("txHash: %s\n", txHash)

		WaitForTestResult(t, ctx, apex, config, user, txHash, initialBalances, sentAmount.Uint64(),
			bridgingType, refundEnabled, maxWaitTimeSec, retryIntervalSec)
	} else {
		txHash, err := apex.SubmitTx(
			ctx, config.srcChainID, user, apex.GetCardanoInfo(config.srcChainID).MultisigAddr[addrIndex],
			new(big.Int).SetUint64(feeAmount+operationFee), sentTokenAmount, metadata)
		require.NoError(t, err)

		fmt.Printf("txHash: %s\n", txHash)

		WaitForTestResult(t, ctx, apex, config, user, txHash, initialBalances, sentTokenAmount[0].Amount,
			bridgingType, refundEnabled, maxWaitTimeSec, retryIntervalSec)
	}
}

func executeInvalidMetadataSlicedOff(t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem,
	config *testConfig, bridgingType cardanofw.BridgingType, addrIndex uint8,
) {
	t.Helper()

	sendAmount := uint64(1_000_000)

	user := apex.Users[len(apex.Users)-1]
	tokenID := apex.GetTokenIDForChain(config.srcChainID, bridgingType == cardanofw.BridgingTypeCurrencyOnSource)
	require.NotZero(t, tokenID)

	receivers := []sendtx.BridgingTxReceiver{
		{
			Addr:    user.GetAddress(config.dstChainID),
			Amount:  sendAmount,
			TokenID: tokenID,
		},
	}

	if bridgingType == cardanofw.BridgingTypeWrappedTokenOnSource {
		_, err := cardanofw.FundUserWithToken(
			ctx, apex, config.srcChainID,
			config.srcMinterWallet, user,
			cardanofw.DefaultTokenName, cardanofw.DefaultTokenMintAmount,
			uint64(10_000_000), uint64(1_123_000))
		require.NoError(t, err)
	}

	multisigAddr := apex.GetCardanoInfo(config.srcChainID).MultisigAddr[addrIndex]

	operationFee := apex.GetMinOperationFee(config.srcChainID)
	minBridgingFee := apex.GetMinBridgingFee(config.srcChainID, bridgingType == cardanofw.BridgingTypeWrappedTokenOnSource)

	feeAmount, err := apex.GetChainMust(t, config.srcChainID).GetBridgingFee(
		ctx, config.dstChainID, receivers, minBridgingFee, operationFee, multisigAddr)
	require.NoError(t, err)

	metadata, err := apex.GetChainMust(t, config.srcChainID).CreateMetadata(
		user.GetAddress(config.srcChainID), config.dstChainID,
		receivers, feeAmount, operationFee)
	require.NoError(t, err)

	// Send only half bytes of metadata making it invalid
	metadata = metadata[0 : len(metadata)/2]

	_, err = apex.SubmitTx(
		ctx, config.srcChainID, user,
		multisigAddr, new(big.Int).SetUint64(sendAmount+feeAmount+operationFee), nil, metadata)
	require.Error(t, err)
}

func executeInvalidMismatchSendNativeTokenAmount(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, user *cardanofw.TestApexUser,
	config *testConfig, nativeTokenAmount wallet.TokenAmount,
	maxWaitTimeSec, retryIntervalSec uint, refundEnabled bool, addrIndex uint8,
) {
	t.Helper()

	bridgingType := cardanofw.BridgingTypeWrappedTokenOnSource

	tokenID := apex.GetTokenIDForChain(config.srcChainID, bridgingType == cardanofw.BridgingTypeCurrencyOnSource)
	require.NotZero(t, tokenID)

	receivers := []sendtx.BridgingTxReceiver{
		{
			Addr:    user.GetAddress(config.dstChainID),
			Amount:  nativeTokenAmount.Amount,
			TokenID: tokenID,
		},
	}

	operationFee := apex.GetMinOperationFee(config.srcChainID)
	minBridgingFee := apex.GetMinBridgingFee(config.srcChainID, bridgingType == cardanofw.BridgingTypeWrappedTokenOnSource)

	metadata, feeAmount := createMetadata(
		t, ctx, apex, config.srcChainID, config.dstChainID,
		minBridgingFee, operationFee, user, receivers, bridgingType)

	bridgingRequestMetadata := bytes.Replace(metadata,
		[]byte(fmt.Sprintf("%d", nativeTokenAmount.Amount)), []byte(fmt.Sprintf("%d", nativeTokenAmount.Amount+1)), 1)

	beforeSendingAmountDfm, err := apex.GetBalance(ctx, user, config.srcChainID)
	require.NoError(t, err)

	txHash, err := apex.SubmitTx(ctx, config.srcChainID,
		user, apex.GetCardanoInfo(config.srcChainID).MultisigAddr[addrIndex],
		new(big.Int).SetUint64(feeAmount+operationFee),
		[]wallet.TokenAmount{nativeTokenAmount}, bridgingRequestMetadata,
	)
	require.NoError(t, err)

	fmt.Printf("txHash: %s\n", txHash)

	WaitForTestResult(t, ctx, apex, config, user, txHash, beforeSendingAmountDfm, nativeTokenAmount.Amount,
		bridgingType, refundEnabled, maxWaitTimeSec, retryIntervalSec)
}

func executeInvalidSendNativeToken(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, user *cardanofw.TestApexUser,
	config *testConfig, nativeTokenAmount wallet.TokenAmount,
	maxWaitTimeSec, retryIntervalSec uint, refundEnabled bool, addrIndex uint8, bridgingType cardanofw.BridgingType,
) {
	t.Helper()

	tokenID := apex.GetTokenIDForChain(config.srcChainID, bridgingType == cardanofw.BridgingTypeCurrencyOnSource)
	require.NotZero(t, tokenID)

	receivers := []sendtx.BridgingTxReceiver{
		{
			Addr:    user.GetAddress(config.dstChainID),
			Amount:  defaultSendAmount,
			TokenID: tokenID,
		},
	}

	// for fee calculation, because of unknown token
	receiversForFeeCalculation := []sendtx.BridgingTxReceiver{
		{
			Addr:    user.GetAddress(config.dstChainID),
			Amount:  defaultSendAmount,
			TokenID: tokenID,
		},
	}

	operationFee := apex.GetMinOperationFee(config.srcChainID)
	minBridgingFee := apex.GetMinBridgingFee(config.srcChainID, bridgingType == cardanofw.BridgingTypeWrappedTokenOnSource)

	feeAmount, err := apex.GetChainMust(t, config.srcChainID).GetBridgingFee(
		ctx, config.dstChainID, receiversForFeeCalculation, minBridgingFee,
		operationFee, apex.GetCardanoInfo(config.srcChainID).MultisigAddr[addrIndex])
	require.NoError(t, err)

	metadata, err := apex.GetChainMust(t, config.srcChainID).CreateMetadata(
		user.GetAddress(config.srcChainID), config.dstChainID, receivers, minBridgingFee, operationFee)
	require.NoError(t, err)

	beforeSendingAmountDfm, err := apex.GetBalance(ctx, user, config.srcChainID)
	require.NoError(t, err)

	lovelaceAmount := defaultSendAmount + feeAmount + operationFee

	txHash, err := apex.SubmitTx(ctx, config.srcChainID, user,
		apex.GetCardanoInfo(config.srcChainID).MultisigAddr[addrIndex],
		new(big.Int).SetUint64(lovelaceAmount), []wallet.TokenAmount{nativeTokenAmount}, metadata)
	require.NoError(t, err)

	fmt.Printf("txHash: %s\n", txHash)

	WaitForTestResult(t, ctx, apex, config, user, txHash, beforeSendingAmountDfm, lovelaceAmount,
		bridgingType, refundEnabled, maxWaitTimeSec, retryIntervalSec)
}
