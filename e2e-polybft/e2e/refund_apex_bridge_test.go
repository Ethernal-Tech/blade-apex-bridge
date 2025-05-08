package e2e

import (
	"context"
	"fmt"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/e2e-polybft/cardanofw"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/e2ehelper"
	infrawallet "github.com/Ethernal-Tech/cardano-infrastructure/wallet"
	"github.com/stretchr/testify/require"
)

func TestE2E_ApexRefund_ValidScenarios(t *testing.T) {
	const (
		apiKey  = "test_api_key"
		userCnt = 5
	)

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	primeConfig, vectorConfig := cardanofw.NewPrimeChainConfig(), cardanofw.NewVectorChainConfig(true)
	primeConfig.PremineAmount = 100_700_000_000
	vectorConfig.PremineAmount = 500_000_000

	apex := cardanofw.SetupAndRunApexBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithUserCnt(userCnt),
		cardanofw.WithPrimeConfig(primeConfig),
		cardanofw.WithVectorConfig(vectorConfig),
		cardanofw.WithCustomConfigHandlers(func(_ *cardanofw.ApexSystem, mp map[string]interface{}) {
			mp["refundEnabled"] = true
			tryCountLimitsSettings := cardanofw.GetMapFromInterfaceKey(mp, "tryCountLimits")
			tryCountLimitsSettings["maxBatchTryCount"] = 2
			tryCountLimitsSettings["maxSubmitTryCount"] = 2
		}, nil),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	user := apex.Users[0]

	txProviderPrime, err := apex.PrimeInfo.GetTxProvider()
	require.NoError(t, err)

	t.Run("From prime to vector - not enough funds on destination multisig address", func(t *testing.T) {
		const (
			sendAmount = uint64(100_600_000_000)
			feeAmount  = uint64(1_100_000)
		)

		beforeSendingAmountDfm, err := apex.GetBalance(ctx, user, cardanofw.ChainIDPrime)
		require.NoError(t, err)

		receivers := map[string]uint64{
			user.GetAddress(cardanofw.ChainIDVector): sendAmount,
		}

		bridgingRequestMetadata, err := cardanofw.CreateCardanoBridgingMetaData(
			user.GetAddress(cardanofw.ChainIDPrime), receivers,
			cardanofw.ChainIDVector, feeAmount)
		require.NoError(t, err)

		txHash, err := cardanofw.SendTx(
			ctx, txProviderPrime, user.PrimeWallet, sendAmount+feeAmount, apex.PrimeInfo.MultisigAddr,
			apex.Config.PrimeConfig.NetworkType, bridgingRequestMetadata)
		require.NoError(t, err)

		lowerBoundaryDfm := new(big.Int).Sub(beforeSendingAmountDfm, new(big.Int).SetUint64(sendAmount+feeAmount))

		fmt.Printf("Tx sent. hash: %s, lowerBoundaryDfm: %d, higherBoundaryDfm: %d\n", txHash, lowerBoundaryDfm, beforeSendingAmountDfm)

		err = apex.WaitForAmountInRange(ctx, user, cardanofw.ChainIDPrime, lowerBoundaryDfm, beforeSendingAmountDfm,
			20, time.Second*30)
		require.NoError(t, err)
	})

	t.Run("Submitted invalid metadata - wrong type", func(t *testing.T) {
		PrimeToVectorInvalidMetadataWrongType(t, ctx, apex, user, cardanofw.DefaultRequestStateTimeoutSec, true)
	})

	t.Run("Submitted invalid metadata - invalid destination", func(t *testing.T) {
		PrimeToVectorInvalidMetadataInvalidDestination(t, ctx, apex, user, 0, true)
	})

	t.Run("Submitted invalid metadata - empty tx", func(t *testing.T) {
		PrimeToVectorInvalidMetadataInvalidTransactions(t, ctx, apex, user, 0, true)
	})

	t.Run("Submitted invalid metadata - invalid sender", func(t *testing.T) {
		PrimeToVectorInvalidMetadataInvalidSender(t, ctx, apex, user, 0)
	})

	t.Run("Submitted with tokens to bridging addr", func(t *testing.T) {
		sendAmount := uint64(5_000_000)
		feeAmount := uint64(1_100_000)

		minterUser := apex.Users[userCnt-1]

		brSubmitterUser, err := cardanofw.NewTestApexUser(
			apex.Config.PrimeConfig.NetworkType, true, apex.Config.VectorConfig.NetworkType, false)
		require.NoError(t, err)

		tokensFunded, err := cardanofw.FundUserWithToken(
			ctx, cardanofw.ChainIDPrime, apex.Config.PrimeConfig.NetworkType, txProviderPrime,
			minterUser, brSubmitterUser, uint64(8_000_000), uint64(1_000_123))
		require.NoError(t, err)

		receivers := map[string]uint64{
			brSubmitterUser.GetAddress(cardanofw.ChainIDVector): sendAmount,
		}

		bridgingRequestMetadata, err := cardanofw.CreateCardanoBridgingMetaData(
			brSubmitterUser.GetAddress(cardanofw.ChainIDPrime), receivers,
			cardanofw.ChainIDVector, feeAmount)
		require.NoError(t, err)

		brSubmitterWallet, _ := brSubmitterUser.GetCardanoWallet(cardanofw.ChainIDPrime)

		beforeSendingAmountDfm, err := apex.GetBalance(ctx, brSubmitterUser, cardanofw.ChainIDPrime)
		require.NoError(t, err)

		txHash, err := cardanofw.SendTxWithTokens(ctx, apex.Config.PrimeConfig.NetworkType, txProviderPrime,
			brSubmitterWallet, apex.PrimeInfo.MultisigAddr,
			sendAmount+feeAmount, []infrawallet.TokenAmount{*tokensFunded}, bridgingRequestMetadata,
		)
		require.NoError(t, err)

		lowerBoundaryDfm := new(big.Int).Sub(beforeSendingAmountDfm, new(big.Int).SetUint64(sendAmount+feeAmount))

		fmt.Printf("Tx sent. hash: %s, lowerBoundaryDfm: %d, higherBoundaryDfm: %d\n", txHash, lowerBoundaryDfm, beforeSendingAmountDfm)

		err = apex.WaitForAmountInRange(ctx, brSubmitterUser, cardanofw.ChainIDPrime, lowerBoundaryDfm, beforeSendingAmountDfm,
			20, time.Second*30)
		require.NoError(t, err)
	})

	t.Run("Mismatch submitted and receiver amounts", func(t *testing.T) {
		PrimeToVectorMismatchSubmittedAndReceiverAmounts(t, ctx, apex, user, 0, true)
	})

	t.Run("Multiple submitters mismatch submitted and receiver amounts", func(t *testing.T) {
		for i := 0; i < 5; i++ {
			sendAmount := uint64(1_000_000)
			feeAmount := uint64(1_100_000)

			receivers := map[string]uint64{
				apex.Users[i].GetAddress(cardanofw.ChainIDVector): sendAmount * 10, // 10Ada
			}

			beforeSendingAmountDfm, err := apex.GetBalance(ctx, apex.Users[i], cardanofw.ChainIDPrime)
			require.NoError(t, err)

			bridgingRequestMetadata, err := cardanofw.CreateCardanoBridgingMetaData(
				apex.Users[i].GetAddress(cardanofw.ChainIDPrime), receivers,
				cardanofw.ChainIDVector, feeAmount)
			require.NoError(t, err)

			txHash, err := cardanofw.SendTx(
				ctx, txProviderPrime, apex.Users[i].PrimeWallet, sendAmount+feeAmount, apex.PrimeInfo.MultisigAddr,
				apex.Config.PrimeConfig.NetworkType, bridgingRequestMetadata)
			require.NoError(t, err)

			lowerBoundaryDfm := new(big.Int).Sub(beforeSendingAmountDfm, new(big.Int).SetUint64(sendAmount+feeAmount))

			fmt.Printf("Tx sent. hash: %s, lowerBoundaryDfm: %d, higherBoundaryDfm: %d\n", txHash, lowerBoundaryDfm, beforeSendingAmountDfm)

			err = apex.WaitForAmountInRange(ctx, apex.Users[i], cardanofw.ChainIDPrime, lowerBoundaryDfm, beforeSendingAmountDfm,
				50, time.Second*30)
			require.NoError(t, err)
		}
	})

	t.Run("Multiple submitters mismatch submitted and receiver amounts parallel", func(t *testing.T) {
		instances := 5
		txHashes := make([]string, instances)

		sendAmount := uint64(1_000_000)
		feeAmount := uint64(1_100_000)

		var wg sync.WaitGroup

		for i := 0; i < instances; i++ {
			idx := i
			receivers := map[string]uint64{
				apex.Users[idx].GetAddress(cardanofw.ChainIDVector): sendAmount * 10, // 10Ada
			}

			wg.Add(1)

			go func() {
				defer wg.Done()

				testUser := apex.Users[idx]

				beforeSendingAmountDfm, err := apex.GetBalance(ctx, testUser, cardanofw.ChainIDPrime)
				require.NoError(t, err)

				lowerBoundaryDfm := new(big.Int).Sub(beforeSendingAmountDfm, new(big.Int).SetUint64(sendAmount+feeAmount))

				bridgingRequestMetadata, err := cardanofw.CreateCardanoBridgingMetaData(
					testUser.GetAddress(cardanofw.ChainIDPrime), receivers,
					cardanofw.ChainIDVector, feeAmount)
				require.NoError(t, err)

				txHashes[idx], err = cardanofw.SendTx(
					ctx, txProviderPrime, testUser.PrimeWallet,
					sendAmount+feeAmount, apex.PrimeInfo.MultisigAddr,
					apex.Config.PrimeConfig.NetworkType, bridgingRequestMetadata)
				require.NoError(t, err)

				fmt.Printf("Tx sent. hash: %s, lowerBoundaryDfm: %d, higherBoundaryDfm: %d\n", txHashes[idx], lowerBoundaryDfm, beforeSendingAmountDfm)

				err = apex.WaitForAmountInRange(ctx, apex.Users[i], cardanofw.ChainIDPrime, lowerBoundaryDfm, beforeSendingAmountDfm,
					50, time.Second*30)
				require.NoError(t, err)
			}()
		}

		wg.Wait()
	})
}

func TestE2E_ApexRefund_BatchRecreated(t *testing.T) {
	const (
		apiKey = "test_api_key"
	)

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	primeConfig, vectorConfig := cardanofw.NewPrimeChainConfig(), cardanofw.NewVectorChainConfig(true)
	primeConfig.FundAmount = 500_000_000
	vectorConfig.FundAmount = 500_000_000
	primeConfig.TTLInc, primeConfig.SlotRoundingThreshold = 250, 50
	vectorConfig.TTLInc, vectorConfig.SlotRoundingThreshold = 5, 30

	apex := cardanofw.SetupAndRunApexBridge(
		t, ctx,
		cardanofw.WithPrimeConfig(primeConfig),
		cardanofw.WithVectorConfig(vectorConfig),
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithUserCnt(1),
		cardanofw.WithCustomConfigHandlers(func(_ *cardanofw.ApexSystem, mp map[string]interface{}) {
			mp["refundEnabled"] = true
			tryCountLimitsSettings := cardanofw.GetMapFromInterfaceKey(mp, "tryCountLimits")
			tryCountLimitsSettings["maxBatchTryCount"] = 2
			tryCountLimitsSettings["maxSubmitTryCount"] = 2
		}, nil),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	sendAmount := uint64(1_000_000)
	feeAmount := uint64(1_100_000)

	brSubmitterUser := apex.Users[0]

	beforeSendingAmountDfm, err := apex.GetBalance(ctx, brSubmitterUser, cardanofw.ChainIDPrime)
	require.NoError(t, err)

	txHash := apex.SubmitBridgingRequest(t, ctx,
		cardanofw.ChainIDPrime, cardanofw.ChainIDVector,
		brSubmitterUser, new(big.Int).SetUint64(sendAmount), brSubmitterUser,
	)

	lowerBoundaryDfm := new(big.Int).Sub(beforeSendingAmountDfm, new(big.Int).SetUint64(sendAmount+feeAmount))

	fmt.Printf("Tx sent. hash: %s, lowerBoundaryDfm: %d, higherBoundaryDfm: %d\n", txHash, lowerBoundaryDfm, beforeSendingAmountDfm)

	err = apex.WaitForAmountInRange(ctx, brSubmitterUser, cardanofw.ChainIDPrime, lowerBoundaryDfm, beforeSendingAmountDfm,
		60, time.Second*30)
	require.NoError(t, err)
}

func TestE2E_ApexRefund_ComplexScenarios_MaxSubmitTryCount(t *testing.T) {
	const (
		apiKey  = "test_api_key"
		userCnt = 10
	)

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	primeConfig, vectorConfig := cardanofw.NewPrimeChainConfig(), cardanofw.NewVectorChainConfig(true)
	primeConfig.PremineAmount = 100_700_000_000
	vectorConfig.PremineAmount = 500_000_000

	apex := cardanofw.SetupAndRunApexBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithUserCnt(userCnt),
		cardanofw.WithPrimeConfig(primeConfig),
		cardanofw.WithVectorConfig(vectorConfig),
		cardanofw.WithCustomConfigHandlers(func(_ *cardanofw.ApexSystem, mp map[string]interface{}) {
			mp["refundEnabled"] = true
			tryCountLimitsSettings := cardanofw.GetMapFromInterfaceKey(mp, "tryCountLimits")
			tryCountLimitsSettings["maxSubmitTryCount"] = 2
		}, nil),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	user := apex.Users[0]

	const (
		sendAmount  = uint64(100_600_000_000)
		sendAmount2 = uint64(1_000_000)
		feeAmount   = uint64(1_100_000)
		instances   = 5
	)

	beforeSendingAmountDfm, err := apex.GetBalance(ctx, user, cardanofw.ChainIDPrime)
	require.NoError(t, err)

	txHash := apex.SubmitBridgingRequest(t, ctx,
		cardanofw.ChainIDPrime, cardanofw.ChainIDVector,
		user, new(big.Int).SetUint64(sendAmount), user,
	)

	lowerBoundaryDfm := new(big.Int).Sub(beforeSendingAmountDfm, new(big.Int).SetUint64(sendAmount+feeAmount))

	fmt.Printf("Tx sent. hash: %s, lowerBoundaryDfm: %d, higherBoundaryDfm: %d\n", txHash, lowerBoundaryDfm, beforeSendingAmountDfm)

	e2ehelper.ExecuteBridging(
		t, ctx, apex, 1, apex.Users[1:instances+1], []*cardanofw.TestApexUser{user},
		[]string{cardanofw.ChainIDPrime},
		map[string][]string{
			cardanofw.ChainIDPrime: {cardanofw.ChainIDVector},
		}, new(big.Int).SetUint64(sendAmount2))

	err = apex.WaitForAmountInRange(ctx, user, cardanofw.ChainIDPrime, lowerBoundaryDfm, beforeSendingAmountDfm,
		20, time.Second*30)
	require.NoError(t, err)
}

func TestE2E_ApexRefund_ComplexScenarios_MaxBatchTryCount(t *testing.T) {
	const (
		apiKey  = "test_api_key"
		userCnt = 10
	)

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	primeConfig, vectorConfig := cardanofw.NewPrimeChainConfig(), cardanofw.NewVectorChainConfig(true)
	primeConfig.PremineAmount = 100_700_000_000
	vectorConfig.PremineAmount = 500_000_000
	vectorConfig.TTLInc, vectorConfig.SlotRoundingThreshold = 1, 30

	apex := cardanofw.SetupAndRunApexBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithUserCnt(userCnt),
		cardanofw.WithPrimeConfig(primeConfig),
		cardanofw.WithVectorConfig(vectorConfig),
		cardanofw.WithCustomConfigHandlers(func(_ *cardanofw.ApexSystem, mp map[string]interface{}) {
			mp["refundEnabled"] = true
			tryCountLimitsSettings := cardanofw.GetMapFromInterfaceKey(mp, "tryCountLimits")
			tryCountLimitsSettings["maxBatchTryCount"] = 1
			tryCountLimitsSettings["maxSubmitTryCount"] = 2
		}, nil),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	user := apex.Users[0]

	const (
		sendAmount = uint64(80_000_000_000)
		feeAmount  = uint64(1_100_000)
	)

	beforeSendingAmountDfm, err := apex.GetBalance(ctx, user, cardanofw.ChainIDPrime)
	require.NoError(t, err)

	txHash := apex.SubmitBridgingRequest(t, ctx,
		cardanofw.ChainIDPrime, cardanofw.ChainIDVector,
		user, new(big.Int).SetUint64(sendAmount), user,
	)

	lowerBoundaryDfm := new(big.Int).Sub(beforeSendingAmountDfm, new(big.Int).SetUint64(sendAmount+feeAmount))

	fmt.Printf("Tx sent. hash: %s, lowerBoundaryDfm: %d, higherBoundaryDfm: %d\n", txHash, lowerBoundaryDfm, beforeSendingAmountDfm)

	err = apex.WaitForAmountInRange(ctx, user, cardanofw.ChainIDPrime, lowerBoundaryDfm, beforeSendingAmountDfm,
		50, time.Second*30)
	require.NoError(t, err)
}

func TestE2E_ApexRefund_ComplexScenarios_MaxRefundTryCount(t *testing.T) {
	const (
		apiKey  = "test_api_key"
		userCnt = 1
	)

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	primeConfig, vectorConfig := cardanofw.NewPrimeChainConfig(), cardanofw.NewVectorChainConfig(true)
	primeConfig.PremineAmount = 100_700_000_000
	vectorConfig.PremineAmount = 500_000_000
	primeConfig.TTLInc, primeConfig.SlotRoundingThreshold = 1, 20
	vectorConfig.TTLInc, vectorConfig.SlotRoundingThreshold = 1, 30

	apex := cardanofw.SetupAndRunApexBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithUserCnt(userCnt),
		cardanofw.WithPrimeConfig(primeConfig),
		cardanofw.WithVectorConfig(vectorConfig),
		cardanofw.WithCustomConfigHandlers(func(_ *cardanofw.ApexSystem, mp map[string]interface{}) {
			mp["refundEnabled"] = true
			tryCountLimitsSettings := cardanofw.GetMapFromInterfaceKey(mp, "tryCountLimits")
			tryCountLimitsSettings["maxBatchTryCount"] = 1
			tryCountLimitsSettings["maxSubmitTryCount"] = 1
			tryCountLimitsSettings["maxRefundTryCount"] = 1
		}, nil),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	user := apex.Users[0]

	const (
		sendAmount = uint64(100_000_000)
	)

	txHash := apex.SubmitBridgingRequest(t, ctx,
		cardanofw.ChainIDPrime, cardanofw.ChainIDVector,
		user, new(big.Int).SetUint64(sendAmount), user,
	)

	fmt.Printf("Tx sent. hash: %s\n", txHash)

	_, timeout := cardanofw.WaitForBatchState(
		ctx, apex, cardanofw.ChainIDPrime, txHash, apiKey, false, true,
		cardanofw.BridgingRequestStatusInvalidRequest, cardanofw.BridgingRequestStatusInvalidRequest)
	require.False(t, timeout)
}
