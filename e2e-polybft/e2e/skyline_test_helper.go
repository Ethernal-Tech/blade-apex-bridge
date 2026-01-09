package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"strconv"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/e2e-polybft/cardanofw"
	"github.com/Ethernal-Tech/cardano-infrastructure/sendtx"
	"github.com/Ethernal-Tech/cardano-infrastructure/wallet"
	"github.com/stretchr/testify/require"
)

func executeInvalidBridgingFee(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem,
	config *testConfig, maxWaitTimeSec, retryIntervalSec uint, refundEnabled bool,
	addrIndex uint8,
) {
	t.Helper()

	user := apex.Users[len(apex.Users)-1]
	receivers := createReceivers(apex, 1, config.dstChainID, defaultSendAmount, config.tokenID)

	operationFee := apex.GetMinOperationFee(config.srcChainID)
	minBridgingFee := apex.GetMinBridgingFee(config.srcChainID, !config.isCurrency)

	metadata, feeAmount := createMetadata(t, ctx, apex, config.srcChainID, config.dstChainID,
		minBridgingFee, operationFee, user, receivers, config.isCurrency)
	bytesToReplace := []byte(strconv.FormatUint(feeAmount, 10))
	metadata = bytes.ReplaceAll(metadata, bytesToReplace, []byte(fmt.Sprintf("%d", minBridgingFee-1)))

	beforeSendingAmountDfm, err := apex.GetBalance(ctx, user, config.srcChainID)
	require.NoError(t, err)

	lovelaceAmount, sentTokenAmount, waitForAmount := getDefaultSendAmounts(
		t, config, feeAmount, operationFee)

	txHash, err := apex.SubmitTx(
		ctx, config.srcChainID, user, apex.GetCardanoInfo(config.srcChainID).MultisigAddr[addrIndex],
		lovelaceAmount, sentTokenAmount, metadata, new(big.Int).SetUint64(operationFee))
	require.NoError(t, err)

	fmt.Printf("txHash: %s\n", txHash)

	WaitForInvalidTestResult(t, ctx, apex, config, user, txHash, beforeSendingAmountDfm, waitForAmount,
		refundEnabled, maxWaitTimeSec, retryIntervalSec)
}

func executeInvalidFeeReceiverAddr(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem,
	config *testConfig, invalidSrcTokenID uint16, maxWaitTimeSec, retryIntervalSec uint, refundEnabled bool,
	addrIndex uint8,
) {
	t.Helper()

	operationFee := apex.GetMinOperationFee(config.srcChainID)
	minBridgingFee := apex.GetMinBridgingFee(config.srcChainID, !config.isCurrency)

	user := apex.Users[len(apex.Users)-1]
	receivers := []sendtx.BridgingTxReceiver{
		{
			Addr:    apex.GetCardanoInfo(config.dstChainID).FeeAddr,
			Amount:  minBridgingFee,
			TokenID: invalidSrcTokenID,
		},
	}

	metadata, feeAmount := createMetadata(t, ctx, apex, config.srcChainID, config.dstChainID,
		minBridgingFee, operationFee, user, receivers, config.isCurrency)

	sentAmount, sentTokenAmount, _ := getDefaultSendAmounts(t, config, feeAmount, operationFee)

	initialBalances, err := apex.GetBalance(ctx, user, config.srcChainID)
	require.NoError(t, err)

	if config.isCurrency {
		txHash, err := apex.SubmitTx(
			ctx, config.srcChainID, user, apex.GetCardanoInfo(config.srcChainID).MultisigAddr[addrIndex],
			sentAmount, nil, metadata, new(big.Int).SetUint64(operationFee))
		require.NoError(t, err)

		fmt.Printf("txHash: %s\n", txHash)

		WaitForInvalidTestResult(t, ctx, apex, config, user, txHash, initialBalances, sentAmount.Uint64(),
			refundEnabled, maxWaitTimeSec, retryIntervalSec)
	} else {
		txHash, err := apex.SubmitTx(
			ctx, config.srcChainID, user, apex.GetCardanoInfo(config.srcChainID).MultisigAddr[addrIndex],
			new(big.Int).SetUint64(feeAmount+operationFee), sentTokenAmount, metadata, new(big.Int).SetUint64(operationFee))
		require.NoError(t, err)

		fmt.Printf("txHash: %s\n", txHash)

		WaitForInvalidTestResult(t, ctx, apex, config, user, txHash, initialBalances, sentTokenAmount[0].Amount,
			refundEnabled, maxWaitTimeSec, retryIntervalSec)
	}
}

func executeInvalidMetadataSlicedOff(t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem,
	config *testConfig, addrIndex uint8,
) {
	t.Helper()

	sendAmount := uint64(1_000_000)

	user := apex.Users[len(apex.Users)-1]

	receivers := []sendtx.BridgingTxReceiver{
		{
			Addr:    user.GetAddress(config.dstChainID),
			Amount:  sendAmount,
			TokenID: config.tokensInfo.SrcTokenID,
		},
	}

	multisigAddr := apex.GetCardanoInfo(config.srcChainID).MultisigAddr[addrIndex]

	operationFee := apex.GetMinOperationFee(config.srcChainID)
	minBridgingFee := apex.GetMinBridgingFee(config.srcChainID, !config.isCurrency)

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
		multisigAddr, new(big.Int).SetUint64(sendAmount+feeAmount+operationFee),
		nil, metadata, new(big.Int).SetUint64(operationFee))
	require.Error(t, err)
}

func executeInvalidMismatchSendNativeTokenAmount(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, user *cardanofw.TestApexUser,
	config *testConfig, nativeTokenAmount wallet.TokenAmount,
	maxWaitTimeSec, retryIntervalSec uint, refundEnabled bool, addrIndex uint8,
) {
	t.Helper()

	receivers := []sendtx.BridgingTxReceiver{
		{
			Addr:    user.GetAddress(config.dstChainID),
			Amount:  nativeTokenAmount.Amount,
			TokenID: config.tokensInfo.SrcTokenID,
		},
	}

	operationFee := apex.GetMinOperationFee(config.srcChainID)
	minBridgingFee := apex.GetMinBridgingFee(config.srcChainID, !config.isCurrency)

	metadata, feeAmount := createMetadata(
		t, ctx, apex, config.srcChainID, config.dstChainID,
		minBridgingFee, operationFee, user, receivers, config.isCurrency)

	bridgingRequestMetadata := bytes.Replace(metadata,
		[]byte(fmt.Sprintf("%d", nativeTokenAmount.Amount)), []byte(fmt.Sprintf("%d", nativeTokenAmount.Amount+1)), 1)

	beforeSendingAmountDfm, err := apex.GetBalance(ctx, user, config.srcChainID)
	require.NoError(t, err)

	txHash, err := apex.SubmitTx(ctx, config.srcChainID,
		user, apex.GetCardanoInfo(config.srcChainID).MultisigAddr[addrIndex],
		new(big.Int).SetUint64(feeAmount+operationFee),
		[]wallet.TokenAmount{nativeTokenAmount}, bridgingRequestMetadata,
		new(big.Int).SetUint64(operationFee))
	require.NoError(t, err)

	fmt.Printf("txHash: %s\n", txHash)

	WaitForInvalidTestResult(t, ctx, apex, config, user, txHash, beforeSendingAmountDfm, nativeTokenAmount.Amount,
		refundEnabled, maxWaitTimeSec, retryIntervalSec)
}

func executeInvalidSendNativeToken(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, user *cardanofw.TestApexUser,
	config *testConfig, nativeTokenAmount wallet.TokenAmount,
	maxWaitTimeSec, retryIntervalSec uint, refundEnabled bool, addrIndex uint8,
) {
	t.Helper()

	receivers := []sendtx.BridgingTxReceiver{
		{
			Addr:    user.GetAddress(config.dstChainID),
			Amount:  defaultSendAmount,
			TokenID: config.tokensInfo.SrcTokenID,
		},
	}

	// for fee calculation, because of unknown token
	receiversForFeeCalculation := []sendtx.BridgingTxReceiver{
		{
			Addr:    user.GetAddress(config.dstChainID),
			Amount:  defaultSendAmount,
			TokenID: config.tokensInfo.SrcTokenID,
		},
	}

	operationFee := apex.GetMinOperationFee(config.srcChainID)
	minBridgingFee := apex.GetMinBridgingFee(config.srcChainID, !config.isCurrency)

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
		new(big.Int).SetUint64(lovelaceAmount), []wallet.TokenAmount{nativeTokenAmount},
		metadata, new(big.Int).SetUint64(operationFee))
	require.NoError(t, err)

	fmt.Printf("txHash: %s\n", txHash)

	WaitForInvalidTestResult(t, ctx, apex, config, user, txHash, beforeSendingAmountDfm, lovelaceAmount,
		refundEnabled, maxWaitTimeSec, retryIntervalSec)
}

func executeInvalidMetadataWrongLabel(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, user *cardanofw.TestApexUser,
) {
	t.Helper()

	sendAmount := uint64(1_000_000)

	beforeSendingAmountDfm, err := apex.GetBalance(ctx, user, cardanofw.ChainIDPrime)
	require.NoError(t, err)

	fmt.Println("beforeSendingAmountDfm", beforeSendingAmountDfm)

	metadata := map[string]interface{}{
		"0": map[string]interface{}{"whatever": "2"},
	}

	bridgingRequestMetadata, err := json.Marshal(metadata)
	require.NoError(t, err)

	operationFee := apex.GetMinOperationFee(cardanofw.ChainIDPrime)

	txHash, err := apex.SubmitTx(ctx, cardanofw.ChainIDPrime, user, apex.PrimeInfo.MultisigAddr[0],
		new(big.Int).SetUint64(sendAmount), nil, bridgingRequestMetadata, new(big.Int).SetUint64(operationFee))
	require.NoError(t, err)

	fmt.Printf("Tx sent. hash: %s\n", txHash)

	_, err = cardanofw.WaitForRequestStates(
		ctx, apex, cardanofw.ChainIDPrime, txHash,
		apex.Config.APIKey, nil, cardanofw.DefaultRequestStateTimeoutSec)
	require.Error(t, err)
	require.ErrorContains(t, err, "timeout")
}

func executeBridgingRequestOperationFee(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem,
	user *cardanofw.TestApexUser, config *testConfig, addrIndex uint8,
	maxWaitTimeSec, retryIntervalSec uint, refundEnabled bool, wrongOpFeeInMetadata bool, customOperationFee uint64,
) {
	t.Helper()

	sendAmount := uint64(1_000_000)

	beforeSendingAmountDfm, err := apex.GetBalance(ctx, user, config.srcChainID)
	require.NoError(t, err)

	receivers := []sendtx.BridgingTxReceiver{
		{
			Addr:    user.GetAddress(config.dstChainID),
			Amount:  sendAmount,
			TokenID: config.tokensInfo.SrcTokenID,
		},
	}

	multisigAddr := apex.GetCardanoInfo(config.srcChainID).MultisigAddr[addrIndex]

	operationFee := apex.GetMinOperationFee(config.srcChainID)

	minBridgingFee := apex.GetMinBridgingFee(config.srcChainID, !config.isCurrency)

	feeAmount, err := apex.GetChainMust(t, config.srcChainID).GetBridgingFee(
		ctx, config.dstChainID, receivers, minBridgingFee, operationFee, multisigAddr)
	require.NoError(t, err)

	if wrongOpFeeInMetadata {
		operationFee = 0
	}

	metadata, err := apex.GetChainMust(t, config.srcChainID).CreateMetadata(
		user.GetAddress(config.srcChainID), config.dstChainID,
		receivers, feeAmount, operationFee)
	require.NoError(t, err)

	lovelaceAmount := sendAmount + feeAmount

	opFee := func(customOperationFee uint64) *big.Int {
		if customOperationFee > 0 {
			return new(big.Int).SetUint64(customOperationFee)
		}
		return nil
	}

	txHash, err := apex.SubmitTx(
		ctx, config.srcChainID, user,
		multisigAddr, new(big.Int).SetUint64(lovelaceAmount), nil, metadata, opFee(customOperationFee))
	require.NoError(t, err)

	WaitForInvalidTestResult(t, ctx, apex, config, user, txHash, beforeSendingAmountDfm, lovelaceAmount,
		refundEnabled, maxWaitTimeSec, retryIntervalSec)
}

type InvalidNexusBridgingRequest struct {
	dstChainID   uint8
	sender       *cardanofw.TestApexUser
	receivers    map[string]cardanofw.ReceiverAmount
	feeAmount    *big.Int
	operationFee *big.Int
	tokenInfo    *cardanofw.BridgingTokensInfo
}

// nexus test helper
func executeInvalidNexusBridgingRequest(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, user *cardanofw.TestApexUser,
	data InvalidNexusBridgingRequest,
) error {
	t.Helper()

	nexusChain := apex.GetChainMust(t, cardanofw.ChainIDNexus).(*cardanofw.TestEVMChain) //nolint:forcetypeassert

	pk, err := user.GetPrivateKey(cardanofw.ChainIDNexus)
	if err != nil {
		return err
	}

	tokenBalance, err := apex.GetBalanceWithTokenName(ctx, user, cardanofw.ChainIDNexus, data.tokenInfo.SrcTokenName)
	if err != nil {
		return err
	}

	feeAmount := cardanofw.DfmToChainNativeTokenAmount(
		cardanofw.ChainIDNexus, new(big.Int).SetUint64(
			apex.GetMinBridgingFee(cardanofw.ChainIDNexus, true)))

	if data.feeAmount != nil {
		feeAmount = data.feeAmount
	}

	txHash, err := nexusChain.DirectBridgingRequest(
		data.dstChainID, pk, data.receivers, feeAmount, data.operationFee, data.tokenInfo.SrcTokenName)
	if err != nil {
		return err
	}

	fmt.Printf("Tx sent. hash: %s\n", txHash)

	err = apex.WaitForExactAmount(ctx, user, cardanofw.ChainIDNexus,
		tokenBalance[data.tokenInfo.SrcTokenName], 10, time.Second*10, data.tokenInfo.SrcTokenName)

	return err
}
