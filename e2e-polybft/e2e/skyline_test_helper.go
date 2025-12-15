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
	config *testConfig, maxWaitTimeSec, retryIntervalSec uint, bridgingType cardanofw.BridgingType, refundEnabled bool,
	addrIndex uint8,
) {
	t.Helper()

	user := apex.Users[len(apex.Users)-1]
	receivers := createReceivers(t, apex, 1, config.srcChainID, config.dstChainID, defaultSendAmount, bridgingType)

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

	tokensInfo := apex.GetBridgingTokensInfo(config.srcChainID, config.dstChainID, bridgingType)
	require.NotNil(t, tokensInfo)

	user := apex.Users[len(apex.Users)-1]
	receivers := []sendtx.BridgingTxReceiver{
		{
			Addr:    apex.GetCardanoInfo(config.dstChainID).FeeAddr,
			Amount:  minBridgingFee,
			TokenID: tokensInfo.SrcTokenID,
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

	tokensInfo := apex.GetBridgingTokensInfo(config.srcChainID, config.dstChainID, bridgingType)
	require.NotNil(t, tokensInfo)

	receivers := []sendtx.BridgingTxReceiver{
		{
			Addr:    user.GetAddress(config.dstChainID),
			Amount:  sendAmount,
			TokenID: tokensInfo.SrcTokenID,
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

	tokenInfo := apex.GetBridgingTokensInfo(config.srcChainID, config.dstChainID, bridgingType)
	require.NotNil(t, tokenInfo)

	receivers := []sendtx.BridgingTxReceiver{
		{
			Addr:    user.GetAddress(config.dstChainID),
			Amount:  nativeTokenAmount.Amount,
			TokenID: tokenInfo.SrcTokenID,
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
	coloredCoins ...uint16,
) {
	t.Helper()

	tokensInfo := apex.GetBridgingTokensInfo(config.srcChainID, config.dstChainID, bridgingType, coloredCoins...)
	require.NotNil(t, tokensInfo)

	receivers := []sendtx.BridgingTxReceiver{
		{
			Addr:    user.GetAddress(config.dstChainID),
			Amount:  defaultSendAmount,
			TokenID: tokensInfo.SrcTokenID,
		},
	}

	// for fee calculation, because of unknown token
	receiversForFeeCalculation := []sendtx.BridgingTxReceiver{
		{
			Addr:    user.GetAddress(config.dstChainID),
			Amount:  defaultSendAmount,
			TokenID: tokensInfo.SrcTokenID,
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
	nexusChain := apex.GetChainMust(t, cardanofw.ChainIDNexus).(*cardanofw.TestEVMChain)

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

	fmt.Printf("feeAmount: %d\n", feeAmount)

	if data.feeAmount != nil {
		feeAmount = data.feeAmount
	}

	txHash, err := nexusChain.DirectBridgingRequest(data.dstChainID, pk, data.receivers, feeAmount, data.operationFee, data.tokenInfo.SrcTokenName)
	if err != nil {
		return err
	}

	fmt.Printf("Tx sent. hash: %s\n", txHash)

	err = apex.WaitForExactAmount(ctx, user, cardanofw.ChainIDNexus, cardanofw.ChainIDNexus,
		tokenBalance[data.tokenInfo.SrcTokenName], 10, time.Second*10, data.tokenInfo.SrcTokenName)
	return err
}
