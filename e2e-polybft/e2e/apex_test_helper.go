package e2e

import (
	"bytes"
	"context"
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
	txHash string, beforeSendingAmountDfm map[string]*big.Int, sentAmount uint64, bridgingType sendtx.BridgingType,
	refundEnabled bool, maxWaitTimeSec, retryIntervalSec uint,
) {
	t.Helper()

	retryIntervalSec = max(retryIntervalSec, 1)
	numRetries := max(1, int(maxWaitTimeSec/retryIntervalSec))

	if refundEnabled {
		tokeName := wallet.AdaTokenName

		if bridgingType == sendtx.BridgingTypeNativeTokenOnSource {
			tokeName = config.srcTokenName
		}

		lowerBoundaryDfm := new(big.Int).Sub(beforeSendingAmountDfm[tokeName], new(big.Int).SetUint64(sentAmount))

		fmt.Printf("Tx sent. hash: %s, lowerBoundaryDfm: %d, higherBoundaryDfm: %+v\n", txHash, lowerBoundaryDfm,
			beforeSendingAmountDfm)

		err := apex.WaitForAmountInRange(ctx, user, config.srcChainID, config.dstChainID, lowerBoundaryDfm,
			beforeSendingAmountDfm[tokeName], numRetries, time.Second*time.Duration(retryIntervalSec),
			bridgingType == sendtx.BridgingTypeNativeTokenOnSource)
		require.NoError(t, err)
	} else {
		cardanofw.WaitForInvalidState(t, ctx, apex, config.srcChainID, txHash, apex.Config.APIKey, maxWaitTimeSec)
	}
}

// Test methods
func executeInvalidMismatchSendLovelaceAmount(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, config *testConfig, user *cardanofw.TestApexUser,
	maxWaitTimeSec, retryIntervalSec uint, bridgingType sendtx.BridgingType, refundEnabled bool, addrIndex uint8,
) {
	t.Helper()

	receivers := createReceivers(apex, 1, config.dstChainID, defaultSendAmount*10, bridgingType)

	operationFee := apex.GetMinOperationFee(config.srcChainID)

	metadata, feeAmount := createMetadata(t, ctx, apex, config.srcChainID, config.dstChainID,
		apex.GetMinBridgingFee(config.srcChainID, bridgingType == sendtx.BridgingTypeNativeTokenOnSource),
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

func executeInvalidMismatchSendAmountMultipleInstances(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, config *testConfig,
	maxWaitTimeSec, retryIntervalSec uint, bridgingType sendtx.BridgingType, refundEnabled bool, addrIndex uint8,
) {
	t.Helper()

	const instances = 5

	for i := 0; i < instances; i++ {
		receivers := createReceivers(apex, 1, config.dstChainID, defaultSendAmount*10, bridgingType)

		operationFee := apex.GetMinOperationFee(config.srcChainID)

		metadata, feeAmount := createMetadata(t, ctx, apex, config.srcChainID, config.dstChainID,
			apex.GetMinBridgingFee(config.srcChainID, bridgingType == sendtx.BridgingTypeNativeTokenOnSource),
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
	maxWaitTimeSec, retryIntervalSec uint, bridgingType sendtx.BridgingType, refundEnabled bool,
	addrIndex uint8,
) {
	t.Helper()

	instances := 5

	var wg sync.WaitGroup

	for i := 0; i < instances; i++ {
		wg.Add(1)

		go func(idx int) {
			defer wg.Done()

			receivers := createReceivers(apex, 1, config.dstChainID, defaultSendAmount*10, bridgingType)

			operationFee := apex.GetMinOperationFee(config.srcChainID)

			metadata, feeAmount := createMetadata(t, ctx, apex, config.srcChainID, config.dstChainID,
				apex.GetMinBridgingFee(config.srcChainID, bridgingType == sendtx.BridgingTypeNativeTokenOnSource),
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
	maxWaitTimeSec, retryIntervalSec uint, bridgingType sendtx.BridgingType, refundEnabled bool, addrIndex uint8,
) {
	t.Helper()

	receivers := createReceivers(apex, 1, config.dstChainID, defaultSendAmount, bridgingType)

	operationFee := apex.GetMinOperationFee(config.srcChainID)

	metadata, feeAmount := createMetadata(t, ctx, apex, config.srcChainID, config.dstChainID,
		apex.GetMinBridgingFee(config.srcChainID, bridgingType == sendtx.BridgingTypeNativeTokenOnSource),
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

func executeInvalidDestination(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, config *testConfig, user *cardanofw.TestApexUser,
	maxWaitTimeSec, retryIntervalSec uint, bridgingType sendtx.BridgingType, refundEnabled bool,
	addrIndex uint8,
) {
	t.Helper()

	receivers := createReceivers(apex, 0, config.dstChainID, defaultSendAmount, bridgingType)
	receiversForFeeCalculation := []sendtx.BridgingTxReceiver{
		{
			Addr:         user.GetAddress(config.dstChainID),
			Amount:       defaultSendAmount,
			BridgingType: bridgingType,
		},
	}

	srcTestChain := apex.GetChainMust(t, config.srcChainID)

	operationFee := apex.GetMinOperationFee(config.srcChainID)

	feeAmount, err := srcTestChain.GetBridgingFee(
		ctx, config.dstChainID, receiversForFeeCalculation,
		apex.GetMinBridgingFee(config.srcChainID, bridgingType == sendtx.BridgingTypeNativeTokenOnSource),
		operationFee,
		config.srcMultiSigAddr)
	require.NoError(t, err)

	metadata, err := srcTestChain.CreateMetadata(
		user.GetAddress(config.srcChainID), config.dstChainID, receivers, feeAmount,
		operationFee,
	)
	require.NoError(t, err)

	metadata = bytes.Replace(metadata, fmt.Appendf([]byte("\"%s\""), config.dstChainID), []byte("\"unknown\""), 1)

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

func executeInvalidMetadataInvalidSender(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, config *testConfig, user *cardanofw.TestApexUser,
	maxWaitTimeSec uint, bridgingType sendtx.BridgingType, addrIndex uint8,
) {
	t.Helper()

	receivers := createReceivers(apex, 1, config.dstChainID, defaultSendAmount, bridgingType)

	srcTestChain := apex.GetChainMust(t, config.srcChainID)

	operationFee := apex.GetMinOperationFee(config.srcChainID)

	feeAmount, err := srcTestChain.GetBridgingFee(
		ctx, config.dstChainID, receivers,
		apex.GetMinBridgingFee(config.srcChainID, bridgingType == sendtx.BridgingTypeNativeTokenOnSource),
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
	maxWaitTimeSec, retryIntervalSec uint, bridgingType sendtx.BridgingType, refundEnabled bool, addrIndex uint8,
) {
	t.Helper()

	receivers := []sendtx.BridgingTxReceiver{}

	receiversForFeeCalculation := []sendtx.BridgingTxReceiver{
		{
			Addr:         user.GetAddress(config.dstChainID),
			Amount:       defaultSendAmount,
			BridgingType: bridgingType,
		},
	}

	srcTestChain := apex.GetChainMust(t, config.srcChainID)

	operationFee := apex.GetMinOperationFee(config.srcChainID)

	feeAmount, err := srcTestChain.GetBridgingFee(
		ctx, config.dstChainID, receiversForFeeCalculation,
		apex.GetMinBridgingFee(config.srcChainID, bridgingType == sendtx.BridgingTypeNativeTokenOnSource),
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
	maxWaitTimeSec, retryIntervalSec uint, bridgingType sendtx.BridgingType, refundEnabled bool, addrIndex uint8,
) {
	t.Helper()

	receivers := createReceivers(apex, 1, config.dstChainID, defaultSendAmount, bridgingType)

	operationFee := apex.GetMinOperationFee(config.srcChainID)

	metadata, feeAmount := createMetadata(t, ctx, apex, config.srcChainID, config.dstChainID,
		apex.GetMinBridgingFee(config.srcChainID, bridgingType == sendtx.BridgingTypeNativeTokenOnSource),
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
	feeAmount uint64, operationFee uint64, bridgingType sendtx.BridgingType,
) (*big.Int, []wallet.TokenAmount, uint64) {
	t.Helper()

	lovelaceAmount := defaultSendAmount + feeAmount + operationFee
	waitForAmount := lovelaceAmount

	tokens := []wallet.TokenAmount(nil)

	if bridgingType == sendtx.BridgingTypeNativeTokenOnSource {
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
	bridgingType sendtx.BridgingType,
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

func createReceivers(
	apex *cardanofw.ApexSystem, receiversCount int, dstChain string, sendAmount uint64, bridgingType sendtx.BridgingType,
) []sendtx.BridgingTxReceiver {
	receivers := make([]sendtx.BridgingTxReceiver, receiversCount)

	for i := range receivers {
		receivers[i] = sendtx.BridgingTxReceiver{
			Addr:         apex.Users[len(apex.Users)-1-i].GetAddress(dstChain),
			Amount:       sendAmount,
			BridgingType: bridgingType,
		}
	}

	return receivers
}
