package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/e2e-polybft/cardanofw"
	"github.com/Ethernal-Tech/cardano-infrastructure/sendtx"
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
	bytesToReplace := []byte(cardanofw.WeiToChainNativeTokenAmount(config.srcChainID, feeAmount).String())

	metadata = bytes.ReplaceAll(
		metadata,
		bytesToReplace,
		[]byte(cardanofw.WeiToChainNativeTokenAmount(
			config.srcChainID,
			new(big.Int).Sub(minBridgingFee, cardanofw.DfmToWei(big.NewInt(1))),
		).String()))

	beforeSendingAmount, err := apex.GetBalance(ctx, user, config.srcChainID)
	require.NoError(t, err)

	defaultAmount, sentTokenAmount, waitForAmount := getDefaultSendAmounts(
		t, config, feeAmount, operationFee)

	txHash, err := apex.SubmitTx(
		ctx, config.srcChainID, user, apex.GetCardanoInfo(config.srcChainID).MultisigAddr[addrIndex],
		defaultAmount, sentTokenAmount, metadata)
	require.NoError(t, err)

	fmt.Printf("txHash: %s\n", txHash)

	WaitForInvalidTestResult(t, ctx, apex, config, user, txHash, beforeSendingAmount, waitForAmount,
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
			Amount:  cardanofw.WeiToDfm(minBridgingFee).Uint64(),
			TokenID: invalidSrcTokenID,
		},
	}

	metadata, feeAmount := createMetadata(t, ctx, apex, config.srcChainID, config.dstChainID,
		minBridgingFee,
		operationFee, user, receivers, config.isCurrency)

	sentAmount, sentTokenAmount, _ := getDefaultSendAmounts(t, config, feeAmount, operationFee)

	initialBalances, err := apex.GetBalance(ctx, user, config.srcChainID)
	require.NoError(t, err)

	if config.isCurrency {
		txHash, err := apex.SubmitTx(
			ctx, config.srcChainID, user, apex.GetCardanoInfo(config.srcChainID).MultisigAddr[addrIndex],
			sentAmount, nil, metadata)
		require.NoError(t, err)

		fmt.Printf("txHash: %s\n", txHash)

		WaitForInvalidTestResult(t, ctx, apex, config, user, txHash, initialBalances, sentAmount,
			refundEnabled, maxWaitTimeSec, retryIntervalSec)
	} else {
		txHash, err := apex.SubmitTx(
			ctx, config.srcChainID, user, apex.GetCardanoInfo(config.srcChainID).MultisigAddr[addrIndex],
			new(big.Int).Add(feeAmount, operationFee), sentTokenAmount, metadata)
		require.NoError(t, err)

		fmt.Printf("txHash: %s\n", txHash)

		WaitForInvalidTestResult(t, ctx, apex, config, user, txHash, initialBalances,
			sentTokenAmount[0].Amount, refundEnabled, maxWaitTimeSec, retryIntervalSec)
	}
}

func executeInvalidMetadataSlicedOff(t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem,
	config *testConfig, addrIndex uint8,
) {
	t.Helper()

	sendAmount := cardanofw.ApexToWei(big.NewInt(1))

	user := apex.Users[len(apex.Users)-1]

	receivers := []sendtx.BridgingTxReceiver{
		{
			Addr:    user.GetAddress(config.dstChainID),
			Amount:  cardanofw.WeiToDfm(sendAmount).Uint64(),
			TokenID: config.tokensInfo.SrcTokenID,
		},
	}

	multisigAddr := apex.GetCardanoInfo(config.srcChainID).MultisigAddr[addrIndex]

	operationFee := apex.GetMinOperationFee(config.srcChainID)
	minBridgingFee := apex.GetMinBridgingFee(config.srcChainID, !config.isCurrency)

	feeAmount, err := apex.GetChainMust(t, config.srcChainID).GetBridgingFee(
		ctx, config.dstChainID, receivers, minBridgingFee,
		operationFee, multisigAddr)
	require.NoError(t, err)

	metadata, err := apex.GetChainMust(t, config.srcChainID).CreateMetadata(
		user.GetAddress(config.srcChainID), config.dstChainID,
		receivers, feeAmount, operationFee)
	require.NoError(t, err)

	// Send only half bytes of metadata making it invalid
	metadata = metadata[0 : len(metadata)/2]

	totalAmount := new(big.Int).Add(sendAmount, feeAmount)
	totalAmount.Add(totalAmount, operationFee)

	_, err = apex.SubmitTx(
		ctx, config.srcChainID, user, multisigAddr,
		totalAmount, nil, metadata)
	require.Error(t, err)
}

func executeInvalidMismatchSendNativeTokenAmount(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, user *cardanofw.TestApexUser,
	config *testConfig, nativeTokenAmount cardanofw.GenericTokenAmount,
	maxWaitTimeSec, retryIntervalSec uint, refundEnabled bool, addrIndex uint8,
) {
	t.Helper()

	nativeTokenAmountDfm := cardanofw.WeiToDfm(nativeTokenAmount.Amount).Uint64()
	receivers := []sendtx.BridgingTxReceiver{
		{
			Addr:    user.GetAddress(config.dstChainID),
			Amount:  nativeTokenAmountDfm,
			TokenID: config.tokensInfo.SrcTokenID,
		},
	}

	operationFee := apex.GetMinOperationFee(config.srcChainID)
	minBridgingFee := apex.GetMinBridgingFee(config.srcChainID, !config.isCurrency)

	metadata, feeAmount := createMetadata(
		t, ctx, apex, config.srcChainID, config.dstChainID,
		minBridgingFee, operationFee, user, receivers, config.isCurrency)

	bridgingRequestMetadata := bytes.Replace(metadata,
		[]byte(fmt.Sprintf("%d", nativeTokenAmountDfm)), []byte(fmt.Sprintf("%d", nativeTokenAmountDfm+1)), 1)

	beforeSendingAmountDfm, err := apex.GetBalance(ctx, user, config.srcChainID)
	require.NoError(t, err)

	txHash, err := apex.SubmitTx(ctx, config.srcChainID,
		user, apex.GetCardanoInfo(config.srcChainID).MultisigAddr[addrIndex],
		new(big.Int).Add(feeAmount, operationFee),
		[]cardanofw.GenericTokenAmount{nativeTokenAmount}, bridgingRequestMetadata,
	)
	require.NoError(t, err)

	fmt.Printf("txHash: %s\n", txHash)

	WaitForInvalidTestResult(t, ctx, apex, config, user, txHash, beforeSendingAmountDfm,
		nativeTokenAmount.Amount, refundEnabled, maxWaitTimeSec, retryIntervalSec)
}

func executeInvalidSendNativeToken(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, user *cardanofw.TestApexUser,
	config *testConfig, nativeTokenAmount cardanofw.GenericTokenAmount,
	maxWaitTimeSec, retryIntervalSec uint, refundEnabled bool, addrIndex uint8,
) {
	t.Helper()

	receivers := []sendtx.BridgingTxReceiver{
		{
			Addr:    user.GetAddress(config.dstChainID),
			Amount:  cardanofw.WeiToDfm(defaultSendAmount).Uint64(),
			TokenID: config.tokensInfo.SrcTokenID,
		},
	}

	// for fee calculation, because of unknown token
	receiversForFeeCalculation := []sendtx.BridgingTxReceiver{
		{
			Addr:    user.GetAddress(config.dstChainID),
			Amount:  cardanofw.WeiToDfm(defaultSendAmount).Uint64(),
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

	weiAmount := new(big.Int).Add(defaultSendAmount, feeAmount)

	weiAmount.Add(weiAmount, operationFee)

	txHash, err := apex.SubmitTx(ctx, config.srcChainID, user,
		apex.GetCardanoInfo(config.srcChainID).MultisigAddr[addrIndex],
		weiAmount, []cardanofw.GenericTokenAmount{nativeTokenAmount}, metadata)
	require.NoError(t, err)

	fmt.Printf("txHash: %s\n", txHash)

	WaitForInvalidTestResult(t, ctx, apex, config, user, txHash, beforeSendingAmountDfm, weiAmount,
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

	txHash, err := apex.SubmitTx(ctx, cardanofw.ChainIDPrime, user, apex.PrimeInfo.MultisigAddr[0],
		new(big.Int).SetUint64(sendAmount), nil, bridgingRequestMetadata)
	require.NoError(t, err)

	fmt.Printf("Tx sent. hash: %s\n", txHash)

	_, err = cardanofw.WaitForRequestStates(
		ctx, apex, cardanofw.ChainIDPrime, txHash,
		apex.Config.APIKey, nil, cardanofw.DefaultRequestStateTimeoutSec)
	require.Error(t, err)
	require.ErrorContains(t, err, "timeout")
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

	feeAmount := cardanofw.WeiToChainNativeTokenAmount(
		cardanofw.ChainIDNexus,
		apex.GetMinBridgingFee(cardanofw.ChainIDNexus, true))

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
