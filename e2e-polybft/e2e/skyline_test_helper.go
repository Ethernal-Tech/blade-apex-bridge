package e2e

import (
	"context"
	"math/big"
	"testing"

	"github.com/0xPolygon/polygon-edge/e2e-polybft/cardanofw"
	"github.com/Ethernal-Tech/cardano-infrastructure/sendtx"
	"github.com/stretchr/testify/require"
)

func executeSkylineMismatchedAndReceivedAmounts(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem,
	srcChain, dstChain cardanofw.ChainID, bridgingFee, operationFee uint64, timeoutSec uint,
) {
	t.Helper()

	user := apex.Users[len(apex.Users)-1]
	sendAmount := uint64(1_000_000)

	receivers := []sendtx.BridgingTxReceiver{
		{
			Addr:         user.GetAddress(dstChain),
			Amount:       sendAmount * 10,
			BridgingType: sendtx.BridgingTypeCurrencyOnSource,
		},
	}

	feeAmount, err := apex.GetChainMust(t, srcChain).GetBridgingFee(
		ctx, dstChain, receivers, bridgingFee, operationFee)
	require.NoError(t, err)

	metadata, err := apex.GetChainMust(t, srcChain).CreateMetadata(
		user.GetAddress(srcChain), dstChain, receivers, feeAmount, operationFee)
	require.NoError(t, err)

	txHash, err := apex.SubmitTx(
		ctx, srcChain, user, apex.PrimeInfo.MultisigAddr,
		new(big.Int).SetUint64(sendAmount+feeAmount+operationFee), metadata)
	require.NoError(t, err)

	cardanofw.WaitForInvalidState(t, ctx, apex, srcChain, txHash, apex.Config.APIKey, timeoutSec)
}
