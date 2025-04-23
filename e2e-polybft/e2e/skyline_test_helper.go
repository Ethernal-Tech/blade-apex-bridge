package e2e

import (
	"bytes"
	"context"
	"fmt"
	"math/big"
	"testing"

	"github.com/0xPolygon/polygon-edge/e2e-polybft/cardanofw"
	commonhelper "github.com/0xPolygon/polygon-edge/helper/common"
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

func executeSkylineMismatchedAndReceivedAmounts(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem,
	srcChain, dstChain cardanofw.ChainID, bridgingFee, operationFee uint64, timeoutSec uint,
) {
	t.Helper()

	sendAmount := uint64(1_000_000)
	user := apex.Users[len(apex.Users)-1]
	receivers := createReceivers(apex, 1, dstChain, sendAmount*10, sendtx.BridgingTypeCurrencyOnSource)

	feeAmount, err := apex.GetChainMust(t, srcChain).GetBridgingFee(
		ctx, dstChain, receivers, bridgingFee, operationFee)
	require.NoError(t, err)

	metadata, err := apex.GetChainMust(t, srcChain).CreateMetadata(
		user.GetAddress(srcChain), dstChain, receivers, feeAmount, operationFee)
	require.NoError(t, err)

	txHash, err := apex.SubmitTx(
		ctx, srcChain, user, apex.GetCardanoInfo(srcChain).MultisigAddr,
		new(big.Int).SetUint64(sendAmount+feeAmount+operationFee), metadata)
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

	feeAmount, err := apex.GetChainMust(t, srcChain).GetBridgingFee(
		ctx, dstChain, receivers, bridgingFee, operationFee)
	require.NoError(t, err)

	metadata, err := apex.GetChainMust(t, srcChain).CreateMetadata(
		user.GetAddress(srcChain), dstChain, receivers, feeAmount, operationFee)
	require.NoError(t, err)

	metadata = bytes.Replace(metadata, []byte("bridge"), []byte("xxxxx"), 1)

	txHash, err := apex.SubmitTx(
		ctx, srcChain, user, apex.GetCardanoInfo(srcChain).MultisigAddr,
		new(big.Int).SetUint64(sendAmount+feeAmount+operationFee), metadata)
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
		new(big.Int).SetUint64(sendAmount+feeAmount+operationFee), metadata)
	require.NoError(t, err)

	cardanofw.WaitForInvalidState(t, ctx, apex, srcChain, txHash, apex.Config.APIKey, timeoutSec)
}

func executeInvalidOperationFee(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem,
	srcChain, dstChain cardanofw.ChainID, bridgingFee, operationFee uint64, timeoutSec uint,
) {
	t.Helper()

	sendAmount := uint64(1_000_000)
	user := apex.Users[len(apex.Users)-1]
	receivers := createReceivers(apex, 1, dstChain, sendAmount, sendtx.BridgingTypeCurrencyOnSource)

	feeAmount, err := apex.GetChainMust(t, srcChain).GetBridgingFee(
		ctx, dstChain, receivers, bridgingFee, operationFee)
	require.NoError(t, err)

	metadata, err := apex.GetChainMust(t, srcChain).CreateMetadata(
		user.GetAddress(srcChain), dstChain, receivers, feeAmount, operationFee)
	require.NoError(t, err)

	metadata = bytes.Replace(metadata, commonhelper.EncodeUint64ToBytes(operationFee), []byte("1"), 1)

	txHash, err := apex.SubmitTx(
		ctx, srcChain, user, apex.GetCardanoInfo(srcChain).MultisigAddr,
		new(big.Int).SetUint64(sendAmount+feeAmount+operationFee), metadata)
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
	receivers := createReceivers(apex, 0, dstChain, sendAmount, sendtx.BridgingTypeCurrencyOnSource)

	feeAmount, err := apex.GetChainMust(t, srcChain).GetBridgingFee(
		ctx, dstChain, receivers, bridgingFee, operationFee)
	require.NoError(t, err)

	metadata, err := apex.GetChainMust(t, srcChain).CreateMetadata(
		user.GetAddress(srcChain), dstChain, receivers, feeAmount, operationFee)
	require.NoError(t, err)

	txHash, err := apex.SubmitTx(
		ctx, srcChain, user, apex.GetCardanoInfo(srcChain).MultisigAddr,
		new(big.Int).SetUint64(sendAmount+feeAmount+operationFee), metadata)
	require.NoError(t, err)

	cardanofw.WaitForInvalidState(t, ctx, apex, srcChain, txHash, apex.Config.APIKey, timeoutSec)
}

func executeInvalidDestionation(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem,
	srcChain, dstChain cardanofw.ChainID, bridgingFee, operationFee uint64, timeoutSec uint,
) {
	t.Helper()

	sendAmount := uint64(1_000_000)
	user := apex.Users[len(apex.Users)-1]
	receivers := createReceivers(apex, 0, dstChain, sendAmount, sendtx.BridgingTypeCurrencyOnSource)

	feeAmount, err := apex.GetChainMust(t, srcChain).GetBridgingFee(
		ctx, dstChain, receivers, bridgingFee, operationFee)
	require.NoError(t, err)

	metadata, err := apex.GetChainMust(t, srcChain).CreateMetadata(
		user.GetAddress(srcChain), dstChain, receivers, feeAmount, operationFee)
	require.NoError(t, err)

	metadata = bytes.Replace(metadata, fmt.Appendf([]byte("\"%s\""), dstChain), []byte("\"unknown\""), 1)

	txHash, err := apex.SubmitTx(
		ctx, srcChain, user, apex.GetCardanoInfo(srcChain).MultisigAddr,
		new(big.Int).SetUint64(sendAmount+feeAmount+operationFee), metadata)
	require.NoError(t, err)

	cardanofw.WaitForInvalidState(t, ctx, apex, srcChain, txHash, apex.Config.APIKey, timeoutSec)
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
