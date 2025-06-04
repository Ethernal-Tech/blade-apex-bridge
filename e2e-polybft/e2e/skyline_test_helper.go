package e2e

import (
	"bytes"
	"context"
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
	config *testConfig, timeoutSec uint, bridgingType sendtx.BridgingType, refundEnabled bool,
) {
	t.Helper()

	sendAmount := uint64(1_000_000)
	user := apex.Users[len(apex.Users)-1]
	receivers := createReceivers(apex, 1, config.dstChainID, sendAmount, sendtx.BridgingTypeCurrencyOnSource)

	metadata, feeAmount := createMetadata(t, ctx, apex, config.srcChainID, config.dstChainID, config.bridgingFee,
		config.operationFee, user, receivers)
	bytesToReplace := []byte(strconv.FormatUint(config.bridgingFee, 10))
	metadata = bytes.Replace(metadata, bytesToReplace, []byte("1"), 1)

	beforeSendingAmountDfm, err := apex.GetBalance(ctx, user, config.srcChainID)
	require.NoError(t, err)

	txHash, err := apex.SubmitTx(
		ctx, config.srcChainID, user, apex.GetCardanoInfo(config.srcChainID).MultisigAddr,
		new(big.Int).SetUint64(sendAmount+feeAmount+config.operationFee), nil, metadata)
	require.NoError(t, err)

	if refundEnabled {
		tokeName := wallet.AdaTokenName
		if bridgingType == sendtx.BridgingTypeNativeTokenOnSource {
			tokeName = config.srcTokenName
		}

		lowerBoundaryDfm := new(big.Int).Sub(beforeSendingAmountDfm[tokeName], new(big.Int).SetUint64(sendAmount+feeAmount))

		fmt.Printf("Tx sent. hash: %s, lowerBoundaryDfm: %d, higherBoundaryDfm: %+v\n", txHash, lowerBoundaryDfm,
			beforeSendingAmountDfm)

		err := apex.WaitForAmountInRange(ctx, user, config.dstChainID, config.srcChainID, lowerBoundaryDfm,
			beforeSendingAmountDfm[tokeName], 20, time.Second*30)
		require.NoError(t, err)
	} else {
		cardanofw.WaitForInvalidState(t, ctx, apex, config.srcChainID, txHash, apex.Config.APIKey, timeoutSec)
	}
}

func executeInvalidFeeReceiverAddr(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem,
	config *testConfig, timeoutSec uint, bridgingType sendtx.BridgingType, refundEnabled bool,
) {
	t.Helper()

	user := apex.Users[len(apex.Users)-1]

	receivers := []sendtx.BridgingTxReceiver{
		{
			Addr:         apex.GetCardanoInfo(config.dstChainID).FeeAddr,
			Amount:       config.bridgingFee,
			BridgingType: sendtx.BridgingTypeNativeTokenOnSource,
		},
	}

	metadata, feeAmount := createMetadata(t, ctx, apex, config.srcChainID, config.dstChainID, config.bridgingFee,
		config.operationFee, user, receivers)

	beforeSendingAmountDfm, err := apex.GetBalance(ctx, user, config.srcChainID)
	require.NoError(t, err)

	txHash, err := apex.SubmitTx(
		ctx, config.srcChainID, user, apex.GetCardanoInfo(config.srcChainID).MultisigAddr,
		new(big.Int).SetUint64(feeAmount+config.operationFee), nil, metadata)

	require.NoError(t, err)

	if refundEnabled {
		tokeName := wallet.AdaTokenName
		if bridgingType == sendtx.BridgingTypeNativeTokenOnSource {
			tokeName = config.srcTokenName
		}

		lowerBoundaryDfm := new(big.Int).Sub(beforeSendingAmountDfm[tokeName], new(big.Int).SetUint64(feeAmount))

		fmt.Printf("Tx sent. hash: %s, lowerBoundaryDfm: %d, higherBoundaryDfm: %+v\n", txHash, lowerBoundaryDfm,
			beforeSendingAmountDfm)

		err := apex.WaitForAmountInRange(ctx, user, config.dstChainID, config.srcChainID, lowerBoundaryDfm,
			beforeSendingAmountDfm[tokeName], 20, time.Second*30)
		require.NoError(t, err)
	} else {
		cardanofw.WaitForInvalidState(t, ctx, apex, config.srcChainID, txHash, apex.Config.APIKey, timeoutSec)
	}
}

func executeInvalidMetadataSlicedOff(t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem,
	config *testConfig, timeoutSec uint, bridgingType sendtx.BridgingType, refundEnabled bool,
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

	feeAmount, err := apex.GetChainMust(t, config.srcChainID).GetBridgingFee(
		ctx, config.dstChainID, receivers, config.bridgingFee, config.operationFee)
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
	timeoutSec uint, refundEnabled bool,
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

	metadata, feeAmount := createMetadata(t, ctx, apex, config.srcChainID, config.dstChainID, config.bridgingFee,
		config.operationFee, user, receivers)

	bridgingRequestMetadata := bytes.Replace(metadata,
		[]byte(fmt.Sprintf("%d", nativeTokenAmount.Amount)), []byte(fmt.Sprintf("%d", nativeTokenAmount.Amount+1)), 1)

	beforeSendingAmountDfm, err := apex.GetBalance(ctx, user, config.srcChainID)
	require.NoError(t, err)

	txHash, err := apex.SubmitTx(ctx, config.srcChainID,
		user, apex.GetCardanoInfo(config.srcChainID).MultisigAddr, new(big.Int).SetUint64(feeAmount+config.operationFee),
		[]wallet.TokenAmount{nativeTokenAmount}, bridgingRequestMetadata,
	)
	require.NoError(t, err)

	if refundEnabled {
		tokeName := wallet.AdaTokenName
		if bridgingType == sendtx.BridgingTypeNativeTokenOnSource {
			tokeName = config.srcTokenName
		}

		lowerBoundaryDfm := new(big.Int).Sub(beforeSendingAmountDfm[tokeName], new(big.Int).SetUint64(feeAmount))

		fmt.Printf("Tx sent. hash: %s, lowerBoundaryDfm: %d, higherBoundaryDfm: %+v\n", txHash, lowerBoundaryDfm,
			beforeSendingAmountDfm)

		err := apex.WaitForAmountInRange(ctx, user, config.dstChainID, config.srcChainID, lowerBoundaryDfm,
			beforeSendingAmountDfm[tokeName], 20, time.Second*30)
		require.NoError(t, err)
	} else {
		cardanofw.WaitForInvalidState(t, ctx, apex, config.srcChainID, txHash, apex.Config.APIKey, timeoutSec)
	}
}

func executeInvalidSendUnknownToken(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, user *cardanofw.TestApexUser,
	config *testConfig, lovelaceAmount uint64, nativeTokenAmount wallet.TokenAmount,
	timeoutSec uint, refundEnabled bool,
) {
	t.Helper()

	bridgingType := sendtx.BridgingTypeCurrencyOnSource

	receivers := []sendtx.BridgingTxReceiver{
		{
			Addr:         user.GetAddress(config.dstChainID),
			Amount:       lovelaceAmount,
			BridgingType: bridgingType,
		},
	}

	metadata, feeAmount := createMetadata(t, ctx, apex, config.srcChainID, config.dstChainID, config.bridgingFee,
		config.operationFee, user, receivers)

	beforeSendingAmountDfm, err := apex.GetBalance(ctx, user, config.srcChainID)
	require.NoError(t, err)

	txHash, err := apex.SubmitTx(ctx, config.srcChainID,
		user, apex.GetCardanoInfo(config.srcChainID).MultisigAddr,
		new(big.Int).SetUint64(lovelaceAmount+feeAmount+config.operationFee), []wallet.TokenAmount{nativeTokenAmount},
		metadata)
	require.NoError(t, err)

	if refundEnabled {
		tokeName := wallet.AdaTokenName
		if bridgingType == sendtx.BridgingTypeNativeTokenOnSource {
			tokeName = config.srcTokenName
		}

		lowerBoundaryDfm := new(big.Int).Sub(beforeSendingAmountDfm[tokeName], new(big.Int).SetUint64(feeAmount))

		fmt.Printf("Tx sent. hash: %s, lowerBoundaryDfm: %d, higherBoundaryDfm: %+v\n", txHash, lowerBoundaryDfm,
			beforeSendingAmountDfm)

		err := apex.WaitForAmountInRange(ctx, user, config.dstChainID, config.srcChainID, lowerBoundaryDfm,
			beforeSendingAmountDfm[tokeName], 20, time.Second*30)
		require.NoError(t, err)
	} else {
		cardanofw.WaitForInvalidState(t, ctx, apex, config.srcChainID, txHash, apex.Config.APIKey, timeoutSec)
	}
}

func createMetadata(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem,
	srcChain, dstChain cardanofw.ChainID, bridgingFee, operationFee uint64,
	sender *cardanofw.TestApexUser, receivers []sendtx.BridgingTxReceiver,
) ([]byte, uint64) {
	t.Helper()

	chain := apex.GetChainMust(t, srcChain)

	feeAmount, err := chain.GetBridgingFee(ctx, dstChain, receivers, bridgingFee, operationFee)
	require.NoError(t, err)

	metadata, err := chain.CreateMetadata(sender.GetAddress(srcChain), dstChain, receivers, feeAmount, operationFee)
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
