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
	config *testConfig, maxWaitTimeSec, retryIntervalSec uint, bridgingType sendtx.BridgingType, refundEnabled bool,
) {
	t.Helper()

	user := apex.Users[len(apex.Users)-1]
	receivers := createReceivers(apex, 1, config.dstChainID, defaultLovelaceAmount, bridgingType)

	metadata, feeAmount := createMetadata(t, ctx, apex, config.srcChainID, config.dstChainID, config.bridgingFee,
		config.operationFee, user, receivers, bridgingType)
	bytesToReplace := []byte(strconv.FormatUint(config.bridgingFee, 10))
	metadata = bytes.Replace(metadata, bytesToReplace, []byte("1"), 1)

	beforeSendingAmountDfm, err := apex.GetBalance(ctx, user, config.srcChainID)
	require.NoError(t, err)

	lovelaceAmount, sentTokenAmount, waitForAmount := getDefaultSendAmounts(t, config, feeAmount, bridgingType)

	txHash, err := apex.SubmitTx(
		ctx, config.srcChainID, user, apex.GetCardanoInfo(config.srcChainID).MultisigAddr[0],
		lovelaceAmount, sentTokenAmount, metadata)
	require.NoError(t, err)

	fmt.Printf("txHash: %s\n", txHash)

	WaitForTestResult(t, ctx, apex, config, user, txHash, beforeSendingAmountDfm, waitForAmount,
		bridgingType, refundEnabled, maxWaitTimeSec, retryIntervalSec)
}

func executeInvalidFeeReceiverAddr(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem,
	config *testConfig, maxWaitTimeSec, retryIntervalSec uint, bridgingType sendtx.BridgingType, refundEnabled bool,
) {
	t.Helper()

	user := apex.Users[len(apex.Users)-1]
	receivers := []sendtx.BridgingTxReceiver{
		{
			Addr:         apex.GetCardanoInfo(config.dstChainID).FeeAddr,
			Amount:       config.bridgingFee,
			BridgingType: bridgingType,
		},
	}

	metadata, feeAmount := createMetadata(t, ctx, apex, config.srcChainID, config.dstChainID, config.bridgingFee,
		config.operationFee, user, receivers, bridgingType)

	sentAmount, sentTokenAmount, _ := getDefaultSendAmounts(t, config, feeAmount, bridgingType)

	initialBalances, err := apex.GetBalance(ctx, user, config.srcChainID)
	require.NoError(t, err)

	if bridgingType == sendtx.BridgingTypeCurrencyOnSource {
		txHash, err := apex.SubmitTx(
			ctx, config.srcChainID, user, apex.GetCardanoInfo(config.srcChainID).MultisigAddr[0],
			sentAmount, []wallet.TokenAmount{}, metadata)
		require.NoError(t, err)

		fmt.Printf("txHash: %s\n", txHash)

		WaitForTestResult(t, ctx, apex, config, user, txHash, initialBalances, sentAmount.Uint64(),
			bridgingType, refundEnabled, maxWaitTimeSec, retryIntervalSec)
	} else {
		txHash, err := apex.SubmitTx(
			ctx, config.srcChainID, user, apex.GetCardanoInfo(config.srcChainID).MultisigAddr[0],
			new(big.Int).SetUint64(feeAmount+config.operationFee), sentTokenAmount, metadata)
		require.NoError(t, err)

		fmt.Printf("txHash: %s\n", txHash)

		WaitForTestResult(t, ctx, apex, config, user, txHash, initialBalances, sentTokenAmount[0].Amount,
			bridgingType, refundEnabled, maxWaitTimeSec, retryIntervalSec)
	}
}
func executeInvalidMetadataSender(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem,
	srcChain, dstChain cardanofw.ChainID, bridgingFee, operationFee uint64, timeoutSec uint,
) {
	t.Helper()

	sendAmount := uint64(1_000_000)
	user := apex.Users[0]

	bridgingType := sendtx.BridgingTypeCurrencyOnSource
	receivers := createReceivers(apex, 1, dstChain, sendAmount, bridgingType)

	multisigAddr, err := apex.GetChainMust(t, srcChain).GetAddressToBridgeTo(
		ctx, bridgingType,
	)
	require.NoError(t, err)

	feeAmount, err := apex.GetChainMust(t, srcChain).GetBridgingFee(
		ctx, dstChain, receivers, bridgingFee, operationFee, multisigAddr)
	require.NoError(t, err)

	metadata, err := apex.GetChainMust(t, srcChain).CreateMetadata(
		"dummy", dstChain, receivers, feeAmount, operationFee)
	require.NoError(t, err)

	metadata = bytes.Replace(metadata, []byte("[\"dummy\"]"), []byte("\"\""), 1)

	txHash, err := apex.SubmitTx(
		ctx, srcChain, user, multisigAddr,
		new(big.Int).SetUint64(sendAmount+feeAmount+operationFee), nil, metadata)
	require.NoError(t, err)

	fmt.Printf("txHash: %s\n", txHash)

	cardanofw.WaitForInvalidState(t, ctx, apex, srcChain, txHash, apex.Config.APIKey, timeoutSec)
}

func executeInvalidMetadataSlicedOff(t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem,
	config *testConfig, bridgingType sendtx.BridgingType,
) {
	t.Helper()

	sendAmount := uint64(1_000_000)

	user := apex.Users[len(apex.Users)-1]

	receivers := []sendtx.BridgingTxReceiver{
		{
			Addr:         user.GetAddress(config.dstChainID),
			Amount:       sendAmount,
			BridgingType: bridgingType,
		},
	}

	if bridgingType == sendtx.BridgingTypeNativeTokenOnSource {
		_, err := cardanofw.FundUserWithToken(
			ctx, apex, config.srcChainID,
			config.srcMinterWallet, user,
			cardanofw.DefaultTokenName, cardanofw.DefaultTokenMintAmount,
			uint64(10_000_000), uint64(1_123_000))
		require.NoError(t, err)
	}

	multisigAddr, err := apex.GetChainMust(t, config.srcChainID).GetAddressToBridgeTo(
		ctx, bridgingType,
	)
	require.NoError(t, err)

	feeAmount, err := apex.GetChainMust(t, config.srcChainID).GetBridgingFee(
		ctx, config.dstChainID, receivers, config.bridgingFee, config.operationFee, multisigAddr)
	require.NoError(t, err)

	metadata, err := apex.GetChainMust(t, config.srcChainID).CreateMetadata(
		user.GetAddress(config.srcChainID), config.dstChainID,
		receivers, feeAmount, config.operationFee)
	require.NoError(t, err)

	// Send only half bytes of metadata making it invalid
	metadata = metadata[0 : len(metadata)/2]

	_, err = apex.SubmitTx(
		ctx, config.srcChainID, user,
		config.srcMultiSigAddr, new(big.Int).SetUint64(sendAmount+feeAmount+config.operationFee), nil, metadata)
	require.Error(t, err)
}

func executeInvalidMismatchSendNativeTokenAmount(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, user *cardanofw.TestApexUser,
	config *testConfig, nativeTokenAmount wallet.TokenAmount,
	maxWaitTimeSec, retryIntervalSec uint, refundEnabled bool,
) {
	t.Helper()

	bridgingType := sendtx.BridgingTypeNativeTokenOnSource

	receivers := []sendtx.BridgingTxReceiver{
		{
			Addr:         user.GetAddress(config.dstChainID),
			Amount:       nativeTokenAmount.Amount,
			BridgingType: bridgingType,
		},
	}

	metadata, feeAmount := createMetadata(
		t, ctx, apex, config.srcChainID, config.dstChainID, config.bridgingFee, config.operationFee, user, receivers, bridgingType)

	bridgingRequestMetadata := bytes.Replace(metadata,
		[]byte(fmt.Sprintf("%d", nativeTokenAmount.Amount)), []byte(fmt.Sprintf("%d", nativeTokenAmount.Amount+1)), 1)

	beforeSendingAmountDfm, err := apex.GetBalance(ctx, user, config.srcChainID)
	require.NoError(t, err)

	txHash, err := apex.SubmitTx(ctx, config.srcChainID,
		user, apex.GetCardanoInfo(config.srcChainID).MultisigAddr[0],
		new(big.Int).SetUint64(feeAmount+config.operationFee),
		[]wallet.TokenAmount{nativeTokenAmount}, bridgingRequestMetadata,
	)
	require.NoError(t, err)

	fmt.Printf("txHash: %s\n", txHash)

	WaitForTestResult(t, ctx, apex, config, user, txHash, beforeSendingAmountDfm, nativeTokenAmount.Amount,
		bridgingType, refundEnabled, maxWaitTimeSec, retryIntervalSec)
}

func executeInvalidSendUnknownToken(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, user *cardanofw.TestApexUser,
	config *testConfig, nativeTokenAmount wallet.TokenAmount,
	maxWaitTimeSec, retryIntervalSec uint, refundEnabled bool,
) {
	t.Helper()

	bridgingType := sendtx.BridgingTypeCurrencyOnSource

	receivers := []sendtx.BridgingTxReceiver{
		{
			Addr:         user.GetAddress(config.dstChainID),
			Amount:       defaultLovelaceAmount,
			BridgingType: bridgingType,
		},
	}

	// for fee calculation, because of unknown token
	receiversForFeeCalculation := []sendtx.BridgingTxReceiver{
		{
			Addr:         user.GetAddress(config.dstChainID),
			Amount:       defaultLovelaceAmount,
			BridgingType: sendtx.BridgingTypeNativeTokenOnSource,
		},
	}

	feeAmount, err := apex.GetChainMust(t, config.srcChainID).GetBridgingFee(
		ctx, config.dstChainID, receiversForFeeCalculation, config.bridgingFee, config.operationFee, apex.GetCardanoInfo(config.srcChainID).MultisigAddr[0])
	require.NoError(t, err)

	metadata, err := apex.GetChainMust(t, config.srcChainID).CreateMetadata(
		user.GetAddress(config.srcChainID), config.dstChainID, receivers, config.bridgingFee, config.operationFee)
	require.NoError(t, err)

	beforeSendingAmountDfm, err := apex.GetBalance(ctx, user, config.srcChainID)
	require.NoError(t, err)

	lovelaceAmount := max(defaultLovelaceAmount, feeAmount) + config.bridgingFee + config.operationFee

	txHash, err := apex.SubmitTx(ctx, config.srcChainID, user, apex.GetCardanoInfo(config.srcChainID).MultisigAddr[0],
		new(big.Int).SetUint64(lovelaceAmount), []wallet.TokenAmount{nativeTokenAmount}, metadata)
	require.NoError(t, err)

	fmt.Printf("txHash: %s\n", txHash)

	WaitForTestResult(t, ctx, apex, config, user, txHash, beforeSendingAmountDfm, lovelaceAmount,
		bridgingType, refundEnabled, maxWaitTimeSec, retryIntervalSec)
}
