package e2e

import (
	"bytes"
	"context"
	"fmt"
	"math/big"
	"strconv"
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

	bridgingFee  uint64
	operationFee uint64
}

func newTestConfig(
	t *testing.T, config *cardanofw.TestCardanoChainConfig, info *cardanofw.CardanoChainInfo, dstChainID cardanofw.ChainID,
	brFee, opFee uint64, srcTokenName string,
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
		srcMultiSigAddr: info.MultisigAddr,
		bridgingFee:     brFee,
		operationFee:    opFee,
		srcTokenName:    srcTokenName,
	}
}

func executeInvalidMismatchSendLovelaceAmount(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem,
	config *testConfig, timeoutSec uint, bridgingType sendtx.BridgingType, refundEnabled bool,
) {
	t.Helper()

	sendAmount := uint64(1_000_000)
	user := apex.Users[len(apex.Users)-1]
	receivers := createReceivers(apex, 1, config.dstChainID, sendAmount*10, bridgingType)

	metadata, feeAmount := createMetadata(t, ctx, apex, config.srcChainID, config.dstChainID, config.bridgingFee,
		config.operationFee, user, receivers)

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

		fmt.Printf("Tx sent. hash: %s, lowerBoundaryDfm: %d, higherBoundaryDfm: %d\n", txHash, lowerBoundaryDfm,
			beforeSendingAmountDfm)

		err := apex.WaitForAmountInRange(ctx, user, config.dstChainID, config.srcChainID, lowerBoundaryDfm,
			beforeSendingAmountDfm[tokeName], 20, time.Second*30)
		require.NoError(t, err)
	} else {
		cardanofw.WaitForInvalidState(t, ctx, apex, config.srcChainID, txHash, apex.Config.APIKey, timeoutSec)
	}
}

func executeInvalidMetadataType(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem,
	config *testConfig, timeoutSec uint, bridgingType sendtx.BridgingType, refundEnabled bool,
) {
	t.Helper()

	sendAmount := uint64(1_000_000)
	user := apex.Users[len(apex.Users)-1]
	receivers := createReceivers(apex, 1, config.dstChainID, sendAmount, bridgingType)

	metadata, feeAmount := createMetadata(t, ctx, apex, config.srcChainID, config.dstChainID, config.bridgingFee,
		config.operationFee, user, receivers)
	metadata = bytes.Replace(metadata, []byte("bridge"), []byte("xxxxx"), 1)

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

		fmt.Printf("Tx sent. hash: %s, lowerBoundaryDfm: %d, higherBoundaryDfm: %d\n", txHash, lowerBoundaryDfm,
			beforeSendingAmountDfm)

		err := apex.WaitForAmountInRange(ctx, user, config.dstChainID, config.srcChainID, lowerBoundaryDfm,
			beforeSendingAmountDfm[tokeName], 20, time.Second*30)
		require.NoError(t, err)
	} else {
		_, err = cardanofw.WaitForRequestStates(ctx, apex, config.srcChainID, txHash, apex.Config.APIKey, nil, timeoutSec)
		require.ErrorContains(t, err, "timeout")
	}
}

func executeInvalidMetadataSender(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem,
	config *testConfig, timeoutSec uint, bridgingType sendtx.BridgingType, refundEnabled bool,
) {
	t.Helper()

	sendAmount := uint64(1_000_000)
	user := apex.Users[0]
	receivers := createReceivers(apex, 1, config.dstChainID, sendAmount, sendtx.BridgingTypeCurrencyOnSource)

	feeAmount, err := apex.GetChainMust(t, config.srcChainID).GetBridgingFee(
		ctx, config.dstChainID, receivers, config.bridgingFee, config.operationFee)
	require.NoError(t, err)

	metadata, err := apex.GetChainMust(t, config.srcChainID).CreateMetadata(
		"dummy", config.dstChainID, receivers, feeAmount, config.operationFee)
	require.NoError(t, err)

	metadata = bytes.Replace(metadata, []byte("[\"dummy\"]"), []byte("\"\""), 1)

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

		fmt.Printf("Tx sent. hash: %s, lowerBoundaryDfm: %d, higherBoundaryDfm: %d\n", txHash, lowerBoundaryDfm,
			beforeSendingAmountDfm)

		err := apex.WaitForAmountInRange(ctx, user, config.dstChainID, config.srcChainID, lowerBoundaryDfm,
			beforeSendingAmountDfm[tokeName], 20, time.Second*30)
		require.NoError(t, err)
	} else {
		cardanofw.WaitForInvalidState(t, ctx, apex, config.srcChainID, txHash, apex.Config.APIKey, timeoutSec)
	}
}

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

		fmt.Printf("Tx sent. hash: %s, lowerBoundaryDfm: %d, higherBoundaryDfm: %d\n", txHash, lowerBoundaryDfm,
			beforeSendingAmountDfm)

		err := apex.WaitForAmountInRange(ctx, user, config.dstChainID, config.srcChainID, lowerBoundaryDfm,
			beforeSendingAmountDfm[tokeName], 20, time.Second*30)
		require.NoError(t, err)
	} else {
		cardanofw.WaitForInvalidState(t, ctx, apex, config.srcChainID, txHash, apex.Config.APIKey, timeoutSec)
	}
}

func executeInvalidEmptyReceivers(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem,
	config *testConfig, timeoutSec uint, bridgingType sendtx.BridgingType, refundEnabled bool,
) {
	t.Helper()

	sendAmount := uint64(1_000_000)
	user := apex.Users[len(apex.Users)-1]
	receivers := []sendtx.BridgingTxReceiver{}

	metadata, feeAmount := createMetadata(t, ctx, apex, config.srcChainID, config.dstChainID, config.bridgingFee,
		config.operationFee, user, receivers)

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

		fmt.Printf("Tx sent. hash: %s, lowerBoundaryDfm: %d, higherBoundaryDfm: %d\n", txHash, lowerBoundaryDfm,
			beforeSendingAmountDfm)

		err := apex.WaitForAmountInRange(ctx, user, config.dstChainID, config.srcChainID, lowerBoundaryDfm,
			beforeSendingAmountDfm[tokeName], 20, time.Second*30)
		require.NoError(t, err)
	} else {
		cardanofw.WaitForInvalidState(t, ctx, apex, config.srcChainID, txHash, apex.Config.APIKey, timeoutSec)
	}
}

func executeInvalidDestination(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem,
	config *testConfig, timeoutSec uint, bridgingType sendtx.BridgingType, refundEnabled bool,
) {
	t.Helper()

	sendAmount := uint64(1_000_000)
	user := apex.Users[len(apex.Users)-1]
	receivers := createReceivers(apex, 0, config.dstChainID, sendAmount, sendtx.BridgingTypeCurrencyOnSource)

	metadata, feeAmount := createMetadata(t, ctx, apex, config.srcChainID, config.dstChainID, config.bridgingFee,
		config.operationFee, user, receivers)
	metadata = bytes.Replace(
		metadata, fmt.Appendf([]byte("\"%s\""), config.dstChainID), []byte("\"unknown\""), 1)

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

		fmt.Printf("Tx sent. hash: %s, lowerBoundaryDfm: %d, higherBoundaryDfm: %d\n", txHash, lowerBoundaryDfm,
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

		fmt.Printf("Tx sent. hash: %s, lowerBoundaryDfm: %d, higherBoundaryDfm: %d\n", txHash, lowerBoundaryDfm,
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

		fmt.Printf("Tx sent. hash: %s, lowerBoundaryDfm: %d, higherBoundaryDfm: %d\n", txHash, lowerBoundaryDfm,
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

		fmt.Printf("Tx sent. hash: %s, lowerBoundaryDfm: %d, higherBoundaryDfm: %d\n", txHash, lowerBoundaryDfm,
			beforeSendingAmountDfm)

		err := apex.WaitForAmountInRange(ctx, user, config.dstChainID, config.srcChainID, lowerBoundaryDfm,
			beforeSendingAmountDfm[tokeName], 20, time.Second*30)
		require.NoError(t, err)
	} else {
		cardanofw.WaitForInvalidState(t, ctx, apex, config.srcChainID, txHash, apex.Config.APIKey, timeoutSec)
	}
}

func executeInvalidMismatchSendAmountMultipleInstances(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, user *cardanofw.TestApexUser, tc *testConfig,
	txType sendtx.BridgingType,
) {
	t.Helper()

	for i := 0; i < 5; i++ {
		sendAmount := uint64(1_000_000)

		tokenName := wallet.AdaTokenName

		if txType == sendtx.BridgingTypeNativeTokenOnSource {
			tokensFunded, err := cardanofw.FundUserWithToken(
				ctx, apex, tc.srcChainID,
				tc.srcMinterWallet, user,
				cardanofw.DefaultTokenName, cardanofw.DefaultTokenMintAmount,
				uint64(10_000_000), uint64(1_123_000))
			require.NoError(t, err)

			tokenName = tokensFunded.TokenName()
		}

		receivers := []sendtx.BridgingTxReceiver{
			{
				Addr:         apex.Users[i].GetAddress(tc.dstChainID),
				Amount:       sendAmount * 10,
				BridgingType: txType,
			},
		}

		feeAmount, err := apex.GetChainMust(t, tc.srcChainID).GetBridgingFee(
			ctx, tc.dstChainID, receivers, tc.bridgingFee, tc.operationFee)
		require.NoError(t, err)

		metadata, err := apex.GetChainMust(t, tc.srcChainID).CreateMetadata(
			apex.Users[i].GetAddress(tc.srcChainID), tc.dstChainID,
			receivers, feeAmount, tc.operationFee)
		require.NoError(t, err)

		beforeSendingAmountDfm, err := apex.GetBalance(ctx, user, tc.srcChainID)
		require.NoError(t, err)

		txHash, err := apex.SubmitTx(
			ctx, tc.srcChainID, apex.Users[i],
			tc.srcMultiSigAddr, new(big.Int).SetUint64(sendAmount+feeAmount+tc.operationFee), nil, metadata)
		require.NoError(t, err)

		lowerBoundaryDfm := new(big.Int).Sub(beforeSendingAmountDfm[tokenName], new(big.Int).SetUint64(sendAmount+feeAmount))

		fmt.Printf("Tx sent. hash: %s, lowerBoundaryDfm: %d, higherBoundaryDfm: %d\n", txHash, lowerBoundaryDfm,
			beforeSendingAmountDfm)

		err = apex.WaitForAmountInRange(ctx, user, tc.dstChainID, tc.srcChainID, lowerBoundaryDfm,
			beforeSendingAmountDfm[tokenName], 20, time.Second*30, txType == sendtx.BridgingTypeNativeTokenOnSource)
		require.NoError(t, err)
	}
}

func executeInvalidMismatchSendAndReceiveAmountParallel(t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem,
	config *testConfig, timeoutSec uint, bridgingType sendtx.BridgingType, refundEnabled bool,
) {
	t.Helper()

	instances := 5
	txHashes := make([]string, instances)

	sendAmount := uint64(1_000_000)

	user := apex.Users[len(apex.Users)-1]

	var wg sync.WaitGroup

	tokenName := wallet.AdaTokenName

	if bridgingType == sendtx.BridgingTypeNativeTokenOnSource {
		tokensFunded, err := cardanofw.FundUserWithToken(
			ctx, apex, config.srcChainID,
			config.srcMinterWallet, user,
			cardanofw.DefaultTokenName, cardanofw.DefaultTokenMintAmount,
			uint64(10_000_000), uint64(1_123_000))
		require.NoError(t, err)

		tokenName = tokensFunded.TokenName()
	}

	beforeSendingAmountDfm, err := apex.GetBalance(ctx, user, config.srcChainID)
	require.NoError(t, err)

	for i := 0; i < instances; i++ {
		wg.Add(1)

		go func(idx int) {
			defer wg.Done()

			testUser := apex.Users[idx]
			receivers := []sendtx.BridgingTxReceiver{
				{
					Addr:         apex.Users[idx].GetAddress(config.dstChainID),
					Amount:       sendAmount * 10,
					BridgingType: bridgingType,
				},
			}

			feeAmount, err := apex.GetChainMust(t, config.srcChainID).GetBridgingFee(
				ctx, config.dstChainID, receivers, config.bridgingFee, config.operationFee)
			require.NoError(t, err)

			metadata, err := apex.GetChainMust(t, config.srcChainID).CreateMetadata(
				testUser.GetAddress(config.srcChainID), config.dstChainID,
				receivers, feeAmount, config.operationFee)
			require.NoError(t, err)

			txHashes[idx], err = apex.SubmitTx(
				ctx, config.srcChainID, testUser,
				config.srcMultiSigAddr, new(big.Int).SetUint64(sendAmount+feeAmount+config.operationFee), nil, metadata)
			require.NoError(t, err)
		}(i)
	}

	wg.Wait()

	lowerBoundaryDfm := new(big.Int).Sub(beforeSendingAmountDfm[tokenName],
		new(big.Int).SetUint64(sendAmount*uint64(instances)))

	fmt.Printf("Txs sent. hashes: %s, lowerBoundaryDfm: %d, higherBoundaryDfm: %d\n", txHashes, lowerBoundaryDfm,
		beforeSendingAmountDfm)

	err = apex.WaitForAmountInRange(ctx, user, config.dstChainID, config.srcChainID, lowerBoundaryDfm,
		beforeSendingAmountDfm[tokenName], 20, time.Second*30)
	require.NoError(t, err)
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
