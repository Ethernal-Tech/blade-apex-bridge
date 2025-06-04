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

// Util methodes
func WaitForTestResult(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, config *testConfig, user *cardanofw.TestApexUser,
	txHash string, beforeSendingAmountDfm map[string]*big.Int, sentAmount uint64, bridgingType sendtx.BridgingType,
	refundEnabled bool,
) {
	t.Helper()

	if refundEnabled {
		tokeName := wallet.AdaTokenName
		if bridgingType == sendtx.BridgingTypeNativeTokenOnSource {
			tokeName = config.srcTokenName
		}

		lowerBoundaryDfm := new(big.Int).Sub(beforeSendingAmountDfm[tokeName], new(big.Int).SetUint64(sentAmount))

		fmt.Printf("Tx sent. hash: %s, lowerBoundaryDfm: %d, higherBoundaryDfm: %+v\n", txHash, lowerBoundaryDfm,
			beforeSendingAmountDfm)

		err := apex.WaitForAmountInRange(ctx, user, config.srcChainID, config.dstChainID, lowerBoundaryDfm,
			beforeSendingAmountDfm[tokeName], 20, time.Second*30, bridgingType == sendtx.BridgingTypeNativeTokenOnSource)
		require.NoError(t, err)
	} else {
		cardanofw.WaitForInvalidState(t, ctx, apex, config.srcChainID, txHash, apex.Config.APIKey, 0)
	}
}

// Test methodes
func executeInvalidMismatchSendLovelaceAmount(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, config *testConfig, user *cardanofw.TestApexUser,
	timeoutSec uint, bridgingType sendtx.BridgingType, refundEnabled bool,
) {
	t.Helper()

	sendAmount := uint64(1_000_000)
	tokenAmount := []wallet.TokenAmount(nil)

	if bridgingType == sendtx.BridgingTypeNativeTokenOnSource {
		token, err := wallet.NewTokenWithFullName(config.srcTokenName, false)
		require.NoError(t, err)

		tokenAmount = []wallet.TokenAmount{wallet.NewTokenAmount(token, sendAmount)}
	}

	receivers := createReceivers(apex, 1, config.dstChainID, sendAmount*10, bridgingType)

	metadata, feeAmount := createMetadata(t, ctx, apex, config.srcChainID, config.dstChainID, config.bridgingFee,
		config.operationFee, user, receivers)

	beforeSendingAmountDfm, err := apex.GetBalance(ctx, user, config.srcChainID)
	require.NoError(t, err)

	txHash, err := apex.SubmitTx(
		ctx, config.srcChainID, user, apex.GetCardanoInfo(config.srcChainID).MultisigAddr,
		new(big.Int).SetUint64(sendAmount+feeAmount+config.operationFee), tokenAmount, metadata)
	require.NoError(t, err)

	WaitForTestResult(t, ctx, apex, config, user, txHash, beforeSendingAmountDfm, sendAmount+feeAmount, bridgingType,
		refundEnabled)
}

func executeInvalidMismatchSendAmountMultipleInstances(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, config *testConfig, user *cardanofw.TestApexUser,
	timeoutSec uint, bridgingType sendtx.BridgingType, refundEnabled bool,
) {
	t.Helper()

	sendAmount := uint64(1_000_000)

	for i := 0; i < 5; i++ {
		tokenName := wallet.AdaTokenName
		tokenAmount := []wallet.TokenAmount(nil)

		if bridgingType == sendtx.BridgingTypeNativeTokenOnSource {
			token, err := wallet.NewTokenWithFullName(config.srcTokenName, false)
			require.NoError(t, err)

			tokenAmount = []wallet.TokenAmount{wallet.NewTokenAmount(token, sendAmount)}
		}

		receivers := []sendtx.BridgingTxReceiver{
			{
				Addr:         apex.Users[i].GetAddress(config.dstChainID),
				Amount:       sendAmount * 10,
				BridgingType: bridgingType,
			},
		}

		feeAmount, err := apex.GetChainMust(t, config.srcChainID).GetBridgingFee(
			ctx, config.dstChainID, receivers, config.bridgingFee, config.operationFee)
		require.NoError(t, err)

		metadata, err := apex.GetChainMust(t, config.srcChainID).CreateMetadata(
			apex.Users[i].GetAddress(config.srcChainID), config.dstChainID,
			receivers, feeAmount, config.operationFee)
		require.NoError(t, err)

		beforeSendingAmountDfm, err := apex.GetBalance(ctx, apex.Users[i], config.srcChainID)
		require.NoError(t, err)

		txHash, err := apex.SubmitTx(
			ctx, config.srcChainID, apex.Users[i],
			config.srcMultiSigAddr, new(big.Int).SetUint64(sendAmount+feeAmount+config.operationFee), tokenAmount, metadata)
		require.NoError(t, err)

		lowerBoundaryDfm := new(big.Int).Sub(beforeSendingAmountDfm[tokenName], new(big.Int).SetUint64(sendAmount+feeAmount))

		fmt.Printf("Tx sent. hash: %s, lowerBoundaryDfm: %d, higherBoundaryDfm: %+v\n", txHash, lowerBoundaryDfm,
			beforeSendingAmountDfm)

		WaitForTestResult(t, ctx, apex, config, apex.Users[i], txHash, beforeSendingAmountDfm, sendAmount+feeAmount,
			bridgingType, refundEnabled)
	}
}

func executeInvalidMismatchSendAmountMultipleInstancesParalel(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, config *testConfig, user *cardanofw.TestApexUser,
	timeoutSec uint, bridgingType sendtx.BridgingType, refundEnabled bool,
) {
	t.Helper()

	instances := 5

	sendAmount := uint64(1_000_000)

	var wg sync.WaitGroup

	for i := 0; i < instances; i++ {
		tokenName := wallet.AdaTokenName

		wg.Add(1)

		go func(idx int) {
			tokenAmount := []wallet.TokenAmount(nil)

			if bridgingType == sendtx.BridgingTypeNativeTokenOnSource {
				token, err := wallet.NewTokenWithFullName(config.srcTokenName, false)
				require.NoError(t, err)

				tokenAmount = []wallet.TokenAmount{wallet.NewTokenAmount(token, sendAmount)}
			}

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
				apex.Users[idx].GetAddress(config.srcChainID), config.dstChainID,
				receivers, feeAmount, config.operationFee)
			require.NoError(t, err)

			beforeSendingAmountDfm, err := apex.GetBalance(ctx, apex.Users[idx], config.srcChainID)
			require.NoError(t, err)

			txHashe, err := apex.SubmitTx(
				ctx, config.srcChainID, apex.Users[idx],
				config.srcMultiSigAddr, new(big.Int).SetUint64(sendAmount+feeAmount+config.operationFee), tokenAmount, metadata)
			require.NoError(t, err)

			lowerBoundaryDfm := new(big.Int).Sub(beforeSendingAmountDfm[tokenName], new(big.Int).SetUint64(sendAmount+feeAmount))

			fmt.Printf("Tx sent. hash: %s, lowerBoundaryDfm: %d, higherBoundaryDfm: %+v\n", txHashe, lowerBoundaryDfm,
				beforeSendingAmountDfm)

			WaitForTestResult(t, ctx, apex, config, apex.Users[idx], txHashe, beforeSendingAmountDfm, sendAmount+feeAmount,
				bridgingType, refundEnabled)
		}(i)
	}

	wg.Wait()
}

func executeInvalidMetadataType(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, config *testConfig, user *cardanofw.TestApexUser,
	timeoutSec uint, bridgingType sendtx.BridgingType, refundEnabled bool,
) {
	t.Helper()

	sendAmount := uint64(1_000_000)
	tokenAmount := []wallet.TokenAmount(nil)

	if bridgingType == sendtx.BridgingTypeNativeTokenOnSource {
		token, err := wallet.NewTokenWithFullName(config.srcTokenName, false)
		require.NoError(t, err)

		tokenAmount = []wallet.TokenAmount{wallet.NewTokenAmount(token, sendAmount)}
	}

	receivers := createReceivers(apex, 1, config.dstChainID, sendAmount, bridgingType)

	metadata, feeAmount := createMetadata(t, ctx, apex, config.srcChainID, config.dstChainID, config.bridgingFee,
		config.operationFee, user, receivers)
	metadata = bytes.Replace(metadata, []byte("bridge"), []byte("xxxxx"), 1)

	beforeSendingAmountDfm, err := apex.GetBalance(ctx, user, config.srcChainID)
	require.NoError(t, err)

	txHash, err := apex.SubmitTx(
		ctx, config.srcChainID, user, apex.GetCardanoInfo(config.srcChainID).MultisigAddr,
		new(big.Int).SetUint64(sendAmount+feeAmount+config.operationFee), tokenAmount, metadata)
	require.NoError(t, err)

	WaitForTestResult(t, ctx, apex, config, user, txHash, beforeSendingAmountDfm, sendAmount+feeAmount,
		bridgingType, refundEnabled)
}

func executeInvalidDestination(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, config *testConfig, user *cardanofw.TestApexUser,
	timeoutSec uint, bridgingType sendtx.BridgingType, refundEnabled bool,
) {
	t.Helper()

	sendAmount := uint64(1_000_000)
	tokenAmount := []wallet.TokenAmount(nil)

	if bridgingType == sendtx.BridgingTypeNativeTokenOnSource {
		token, err := wallet.NewTokenWithFullName(config.srcTokenName, false)
		require.NoError(t, err)

		tokenAmount = []wallet.TokenAmount{wallet.NewTokenAmount(token, sendAmount)}
	}

	receivers := createReceivers(apex, 0, config.dstChainID, sendAmount, bridgingType)

	metadata, feeAmount := createMetadata(t, ctx, apex, config.srcChainID, config.dstChainID, config.bridgingFee,
		config.operationFee, user, receivers)
	metadata = bytes.Replace(metadata, fmt.Appendf([]byte("\"%s\""), config.dstChainID), []byte("\"unknown\""), 1)

	beforeSendingAmountDfm, err := apex.GetBalance(ctx, user, config.srcChainID)
	require.NoError(t, err)

	txHash, err := apex.SubmitTx(
		ctx, config.srcChainID, user, apex.GetCardanoInfo(config.srcChainID).MultisigAddr,
		new(big.Int).SetUint64(sendAmount+feeAmount+config.operationFee), tokenAmount, metadata)
	require.NoError(t, err)

	WaitForTestResult(t, ctx, apex, config, user, txHash, beforeSendingAmountDfm, sendAmount+feeAmount,
		bridgingType, refundEnabled)
}

func executeInvalidMetadataInvalidSender(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, config *testConfig, user *cardanofw.TestApexUser,
	timeoutSec uint, bridgingType sendtx.BridgingType,
) {
	t.Helper()

	sendAmount := uint64(1_000_000)
	tokenAmount := []wallet.TokenAmount(nil)

	if bridgingType == sendtx.BridgingTypeNativeTokenOnSource {
		token, err := wallet.NewTokenWithFullName(config.srcTokenName, false)
		require.NoError(t, err)

		tokenAmount = []wallet.TokenAmount{wallet.NewTokenAmount(token, sendAmount)}
	}

	receivers := createReceivers(apex, 1, config.dstChainID, sendAmount, bridgingType)

	feeAmount, err := apex.GetChainMust(t, config.srcChainID).GetBridgingFee(
		ctx, config.dstChainID, receivers, config.bridgingFee, config.operationFee)
	require.NoError(t, err)

	metadata, err := apex.GetChainMust(t, config.srcChainID).CreateMetadata(
		"dummy", config.dstChainID, receivers, feeAmount, config.operationFee)
	require.NoError(t, err)

	// remove this after we make correct validation on oracle!
	metadata = bytes.Replace(metadata, []byte("[\"dummy\"]"), []byte("\"\""), 1)

	txHash, err := apex.SubmitTx(
		ctx, config.srcChainID, user, apex.GetCardanoInfo(config.srcChainID).MultisigAddr,
		new(big.Int).SetUint64(sendAmount+feeAmount+config.operationFee), tokenAmount, metadata)
	require.NoError(t, err)

	cardanofw.WaitForInvalidState(t, ctx, apex, config.srcChainID, txHash, apex.Config.APIKey, timeoutSec)
}

func executeInvalidEmptyReceivers(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, config *testConfig, user *cardanofw.TestApexUser,
	timeoutSec uint, bridgingType sendtx.BridgingType, refundEnabled bool,
) {
	t.Helper()

	sendAmount := uint64(1_000_000)
	tokenAmount := []wallet.TokenAmount(nil)

	if bridgingType == sendtx.BridgingTypeNativeTokenOnSource {
		token, err := wallet.NewTokenWithFullName(config.srcTokenName, false)
		require.NoError(t, err)

		tokenAmount = []wallet.TokenAmount{wallet.NewTokenAmount(token, sendAmount)}
	}

	receivers := []sendtx.BridgingTxReceiver{}

	metadata, feeAmount := createMetadata(t, ctx, apex, config.srcChainID, config.dstChainID, config.bridgingFee,
		config.operationFee, user, receivers)

	beforeSendingAmountDfm, err := apex.GetBalance(ctx, user, config.srcChainID)
	require.NoError(t, err)

	txHash, err := apex.SubmitTx(
		ctx, config.srcChainID, user, apex.GetCardanoInfo(config.srcChainID).MultisigAddr,
		new(big.Int).SetUint64(sendAmount+feeAmount+config.operationFee), tokenAmount, metadata)
	require.NoError(t, err)

	WaitForTestResult(t, ctx, apex, config, user, txHash, beforeSendingAmountDfm, sendAmount+feeAmount,
		bridgingType, refundEnabled)
}
