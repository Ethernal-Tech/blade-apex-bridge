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

type testConfig struct {
	srcChainID cardanofw.ChainID
	dstChainID cardanofw.ChainID

	srcMinterWallet *wallet.Wallet
	srcNetworkType  wallet.CardanoNetworkType
	srcTxProvider   wallet.ITxProvider
}

func newTestConfig(
	t *testing.T, config *cardanofw.TestCardanoChainConfig, info *cardanofw.CardanoChainInfo, dstChainID cardanofw.ChainID,
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
	}
}

func executeInvalidMismatchSendLovelaceAmount(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem,
	srcChain, dstChain cardanofw.ChainID, bridgingFee, operationFee uint64, timeoutSec uint,
) {
	t.Helper()

	sendAmount := uint64(1_000_000)
	user := apex.Users[len(apex.Users)-1]
	receivers := createReceivers(apex, 1, dstChain, sendAmount*10, sendtx.BridgingTypeCurrencyOnSource)

	metadata, feeAmount := createMetadata(t, ctx, apex, srcChain, dstChain, bridgingFee, operationFee, user, receivers)

	txHash, err := apex.SubmitTx(
		ctx, srcChain, user, apex.GetCardanoInfo(srcChain).MultisigAddr,
		new(big.Int).SetUint64(sendAmount+feeAmount+operationFee), nil, metadata)
	require.NoError(t, err)

	cardanofw.WaitForInvalidState(t, ctx, apex, srcChain, txHash, apex.Config.APIKey, timeoutSec)
}

func excuteInvalidMetadataType(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem,
	srcChain, dstChain cardanofw.ChainID, bridgingFee, operationFee uint64, timeoutSec uint,
) {
	t.Helper()

	sendAmount := uint64(1_000_000)
	user := apex.Users[len(apex.Users)-1]
	receivers := createReceivers(apex, 1, dstChain, sendAmount, sendtx.BridgingTypeCurrencyOnSource)

	metadata, feeAmount := createMetadata(t, ctx, apex, srcChain, dstChain, bridgingFee, operationFee, user, receivers)
	metadata = bytes.Replace(metadata, []byte("bridge"), []byte("xxxxx"), 1)

	txHash, err := apex.SubmitTx(
		ctx, srcChain, user, apex.GetCardanoInfo(srcChain).MultisigAddr,
		new(big.Int).SetUint64(sendAmount+feeAmount+operationFee), nil, metadata)
	require.NoError(t, err)

	_, err = cardanofw.WaitForRequestStates(ctx, apex, srcChain, txHash, apex.Config.APIKey, nil, timeoutSec)
	require.ErrorContains(t, err, "timeout")
}

func executeInvalidMetadataSender(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem,
	srcChain, dstChain cardanofw.ChainID, bridgingFee, operationFee uint64, timeoutSec uint,
) {
	t.Helper()

	sendAmount := uint64(1_000_000)
	user := apex.Users[0]
	receivers := createReceivers(apex, 1, dstChain, sendAmount, sendtx.BridgingTypeCurrencyOnSource)

	feeAmount, err := apex.GetChainMust(t, srcChain).GetBridgingFee(
		ctx, dstChain, receivers, bridgingFee, operationFee)
	require.NoError(t, err)

	metadata, err := apex.GetChainMust(t, srcChain).CreateMetadata(
		"dummy", dstChain, receivers, feeAmount, operationFee)
	require.NoError(t, err)

	metadata = bytes.Replace(metadata, []byte("[\"dummy\"]"), []byte("\"\""), 1)

	txHash, err := apex.SubmitTx(
		ctx, srcChain, user, apex.GetCardanoInfo(srcChain).MultisigAddr,
		new(big.Int).SetUint64(sendAmount+feeAmount+operationFee), nil, metadata)
	require.NoError(t, err)

	cardanofw.WaitForInvalidState(t, ctx, apex, srcChain, txHash, apex.Config.APIKey, timeoutSec)
}

func executeInvalidBridgingFee(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem,
	srcChain, dstChain cardanofw.ChainID, bridgingFee, operationFee uint64, timeoutSec uint,
) {
	t.Helper()

	sendAmount := uint64(1_000_000)
	user := apex.Users[len(apex.Users)-1]
	receivers := createReceivers(apex, 1, dstChain, sendAmount, sendtx.BridgingTypeCurrencyOnSource)

	metadata, feeAmount := createMetadata(t, ctx, apex, srcChain, dstChain, bridgingFee, operationFee, user, receivers)
	bytesToReplace := []byte(strconv.FormatUint(bridgingFee, 10))
	metadata = bytes.Replace(metadata, bytesToReplace, []byte("1"), 1)

	txHash, err := apex.SubmitTx(
		ctx, srcChain, user, apex.GetCardanoInfo(srcChain).MultisigAddr,
		new(big.Int).SetUint64(sendAmount+feeAmount+operationFee), nil, metadata)
	require.NoError(t, err)

	cardanofw.WaitForInvalidState(t, ctx, apex, srcChain, txHash, apex.Config.APIKey, timeoutSec)
}

func executeInvalidEmptyReceivers(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem,
	srcChain, dstChain cardanofw.ChainID, bridgingFee, operationFee uint64, timeoutSec uint,
) {
	t.Helper()

	sendAmount := uint64(1_000_000)
	user := apex.Users[len(apex.Users)-1]
	receivers := []sendtx.BridgingTxReceiver{}

	metadata, feeAmount := createMetadata(t, ctx, apex, srcChain, dstChain, bridgingFee, operationFee, user, receivers)

	txHash, err := apex.SubmitTx(
		ctx, srcChain, user, apex.GetCardanoInfo(srcChain).MultisigAddr,
		new(big.Int).SetUint64(sendAmount+feeAmount+operationFee), nil, metadata)
	require.NoError(t, err)

	cardanofw.WaitForInvalidState(t, ctx, apex, srcChain, txHash, apex.Config.APIKey, timeoutSec)
}

func executeInvalidDestination(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem,
	srcChain, dstChain cardanofw.ChainID, bridgingFee, operationFee uint64, timeoutSec uint,
) {
	t.Helper()

	sendAmount := uint64(1_000_000)
	user := apex.Users[len(apex.Users)-1]
	receivers := createReceivers(apex, 0, dstChain, sendAmount, sendtx.BridgingTypeCurrencyOnSource)

	metadata, feeAmount := createMetadata(t, ctx, apex, srcChain, dstChain, bridgingFee, operationFee, user, receivers)
	metadata = bytes.Replace(
		metadata, fmt.Appendf([]byte("\"%s\""), dstChain), []byte("\"unknown\""), 1)

	txHash, err := apex.SubmitTx(
		ctx, srcChain, user, apex.GetCardanoInfo(srcChain).MultisigAddr,
		new(big.Int).SetUint64(sendAmount+feeAmount+operationFee), nil, metadata)
	require.NoError(t, err)

	cardanofw.WaitForInvalidState(t, ctx, apex, srcChain, txHash, apex.Config.APIKey, timeoutSec)
}

func executeInvalidFeeReceiverAddr(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem,
	srcChain, dstChain cardanofw.ChainID, bridgingFee, operationFee uint64, timeoutSec uint,
) {
	t.Helper()

	user := apex.Users[len(apex.Users)-1]

	receivers := []sendtx.BridgingTxReceiver{
		{
			Addr:         apex.GetCardanoInfo(dstChain).FeeAddr,
			Amount:       bridgingFee,
			BridgingType: sendtx.BridgingTypeNativeTokenOnSource,
		},
	}

	metadata, feeAmount := createMetadata(t, ctx, apex, srcChain, dstChain, bridgingFee, operationFee, user, receivers)

	txHash, err := apex.SubmitTx(
		ctx, srcChain, user, apex.GetCardanoInfo(srcChain).MultisigAddr,
		new(big.Int).SetUint64(feeAmount+operationFee), nil, metadata)

	require.NoError(t, err)

	cardanofw.WaitForInvalidState(t, ctx, apex, srcChain, txHash, apex.Config.APIKey, timeoutSec)
}

func executeInvalidMismatchSendNativeTokenAmount(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, user *cardanofw.TestApexUser,
	srcChain, dstChain cardanofw.ChainID,
	bridgingFee, operationFee uint64, nativeTokenAmount wallet.TokenAmount,
	timeoutSec uint,
) {
	t.Helper()

	receivers := []sendtx.BridgingTxReceiver{
		{
			Addr:         user.GetAddress(dstChain),
			Amount:       nativeTokenAmount.Amount,
			BridgingType: sendtx.BridgingTypeNativeTokenOnSource,
		},
	}

	metadata, feeAmount := createMetadata(t, ctx, apex, srcChain, dstChain, bridgingFee, operationFee, user, receivers)

	bridgingRequestMetadata := bytes.Replace(metadata,
		[]byte(fmt.Sprintf("%d", nativeTokenAmount.Amount)), []byte(fmt.Sprintf("%d", nativeTokenAmount.Amount+1)), 1)

	txHash, err := apex.SubmitTx(ctx, srcChain,
		user, apex.GetCardanoInfo(srcChain).MultisigAddr,
		new(big.Int).SetUint64(feeAmount+operationFee), &nativeTokenAmount, bridgingRequestMetadata,
	)
	require.NoError(t, err)

	cardanofw.WaitForInvalidState(t, ctx, apex, srcChain, txHash, apex.Config.APIKey, timeoutSec)
}

func executeInvalidSendUnknownToken(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, user *cardanofw.TestApexUser,
	srcChain, dstChain cardanofw.ChainID,
	bridgingFee, operationFee uint64, lovelaceAmount uint64, nativeTokenAmount wallet.TokenAmount,
	timeoutSec uint,
) {
	t.Helper()

	receivers := []sendtx.BridgingTxReceiver{
		{
			Addr:         user.GetAddress(dstChain),
			Amount:       lovelaceAmount,
			BridgingType: sendtx.BridgingTypeCurrencyOnSource,
		},
	}

	metadata, feeAmount := createMetadata(t, ctx, apex, srcChain, dstChain, bridgingFee, operationFee, user, receivers)

	txHash, err := apex.SubmitTx(ctx, srcChain,
		user, apex.GetCardanoInfo(srcChain).MultisigAddr,
		new(big.Int).SetUint64(lovelaceAmount+feeAmount+operationFee), &nativeTokenAmount, metadata)
	require.NoError(t, err)

	cardanofw.WaitForInvalidState(t, ctx, apex, srcChain, txHash, apex.Config.APIKey, timeoutSec)
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
