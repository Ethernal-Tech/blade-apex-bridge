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
	"github.com/Ethernal-Tech/cardano-infrastructure/sendtx"
	cardanowallet "github.com/Ethernal-Tech/cardano-infrastructure/wallet"
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

	apex := cardanofw.SetupAndRunReactorBridge(
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

	t.Run("From prime to vector - not enough funds on destination multisig address", func(t *testing.T) {
		const (
			sendAmount   = uint64(100_600_000_000)
			feeAmount    = uint64(1_100_000)
			operationFee = uint64(0)
		)

		beforeSendingAmountDfm, err := apex.GetBalance(ctx, user, cardanofw.ChainIDPrime)
		require.NoError(t, err)

		metadata, err := apex.GetChainMust(t, cardanofw.ChainIDPrime).CreateMetadata(
			user.GetAddress(cardanofw.ChainIDPrime), cardanofw.ChainIDVector,
			[]sendtx.BridgingTxReceiver{
				{
					Addr:         user.GetAddress(cardanofw.ChainIDVector),
					Amount:       sendAmount,
					BridgingType: sendtx.BridgingTypeNormal,
				},
			}, feeAmount, operationFee)
		require.NoError(t, err)

		txHash, err := apex.SubmitTx(
			ctx, cardanofw.ChainIDPrime, user,
			apex.PrimeInfo.MultisigAddr, new(big.Int).SetUint64(sendAmount+feeAmount), nil, metadata)
		require.NoError(t, err)

		lowerBoundaryDfm := new(big.Int).Sub(beforeSendingAmountDfm[cardanowallet.AdaTokenName], new(big.Int).SetUint64(sendAmount+feeAmount))

		fmt.Printf("Tx sent. hash: %s, lowerBoundaryDfm: %d, higherBoundaryDfm: %d\n", txHash, lowerBoundaryDfm, beforeSendingAmountDfm)

		err = apex.WaitForAmountInRange(ctx, user, cardanofw.ChainIDPrime, cardanofw.ChainIDVector, lowerBoundaryDfm,
			beforeSendingAmountDfm[cardanowallet.AdaTokenName], 20, time.Second*30)
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
		operationFee := uint64(0)
		minterUser := apex.Users[userCnt-1]

		brSubmitterUser, err := cardanofw.NewTestApexUser(cardanofw.NewApexNetworkTypes(apex.Config.PrimeConfig, apex.Config.VectorConfig, nil, nil))
		require.NoError(t, err)

		minterWallet, _ := minterUser.GetCardanoWallet(cardanofw.ChainIDPrime)

		tokensFunded, err := cardanofw.FundUserWithToken(
			ctx, apex, cardanofw.ChainIDPrime,
			minterWallet, brSubmitterUser,
			cardanofw.DefaultTokenName, cardanofw.DefaultTokenMintAmount,
			uint64(10_000_000), uint64(1_000_000))
		require.NoError(t, err)

		beforeSendingAmountDfm, err := apex.GetBalance(ctx, brSubmitterUser, cardanofw.ChainIDPrime)
		require.NoError(t, err)

		metadata, err := apex.GetChainMust(t, cardanofw.ChainIDPrime).CreateMetadata(
			user.GetAddress(cardanofw.ChainIDPrime), cardanofw.ChainIDVector,
			[]sendtx.BridgingTxReceiver{
				{
					Addr:   user.GetAddress(cardanofw.ChainIDVector),
					Amount: sendAmount - feeAmount,
				},
			}, feeAmount, operationFee)
		require.NoError(t, err)

		txHash, err := apex.SubmitTx(ctx, cardanofw.ChainIDPrime, brSubmitterUser, apex.PrimeInfo.MultisigAddr,
			new(big.Int).SetUint64(sendAmount), []infrawallet.TokenAmount{*tokensFunded}, metadata)
		require.NoError(t, err)

		lowerBoundaryDfm := new(big.Int).Sub(beforeSendingAmountDfm[cardanowallet.AdaTokenName], new(big.Int).SetUint64(sendAmount+feeAmount))

		fmt.Printf("Tx sent. hash: %s, lowerBoundaryDfm: %d, higherBoundaryDfm: %d\n", txHash, lowerBoundaryDfm, beforeSendingAmountDfm)

		err = apex.WaitForAmountInRange(ctx, brSubmitterUser, cardanofw.ChainIDPrime, cardanofw.ChainIDVector, lowerBoundaryDfm,
			beforeSendingAmountDfm[cardanowallet.AdaTokenName], 20, time.Second*30)
		require.NoError(t, err)
	})

	t.Run("Mismatch submitted and receiver amounts", func(t *testing.T) {
		PrimeToVectorMismatchSubmittedAndReceiverAmounts(t, ctx, apex, user, 0, true)
	})

	t.Run("Multiple submitters mismatch submitted and receiver amounts", func(t *testing.T) {
		for i := 0; i < 5; i++ {
			sendAmount := uint64(1_000_000)
			feeAmount := uint64(1_100_000)
			operationFee := uint64(0)

			beforeSendingAmountDfm, err := apex.GetBalance(ctx, apex.Users[i], cardanofw.ChainIDPrime)
			require.NoError(t, err)

			metadata, err := apex.GetChainMust(t, cardanofw.ChainIDPrime).CreateMetadata(
				apex.Users[i].GetAddress(cardanofw.ChainIDPrime), cardanofw.ChainIDVector,
				[]sendtx.BridgingTxReceiver{
					{
						Addr:         apex.Users[i].GetAddress(cardanofw.ChainIDVector),
						Amount:       sendAmount * 10,
						BridgingType: sendtx.BridgingTypeNormal,
					},
				}, feeAmount, operationFee)
			require.NoError(t, err)

			txHash, err := apex.SubmitTx(
				ctx, cardanofw.ChainIDPrime, apex.Users[i],
				apex.PrimeInfo.MultisigAddr, new(big.Int).SetUint64(sendAmount+feeAmount), nil, metadata)
			require.NoError(t, err)

			lowerBoundaryDfm := new(big.Int).Sub(beforeSendingAmountDfm[cardanowallet.AdaTokenName], new(big.Int).SetUint64(sendAmount+feeAmount))

			fmt.Printf("Tx sent. hash: %s, lowerBoundaryDfm: %d, higherBoundaryDfm: %d\n", txHash, lowerBoundaryDfm, beforeSendingAmountDfm)

			err = apex.WaitForAmountInRange(ctx, apex.Users[i], cardanofw.ChainIDPrime, cardanofw.ChainIDVector, lowerBoundaryDfm,
				beforeSendingAmountDfm[cardanowallet.AdaTokenName], 50, time.Second*30)
			require.NoError(t, err)
		}
	})

	t.Run("Multiple submitters mismatch submitted and receiver amounts parallel", func(t *testing.T) {
		instances := 5
		txHashes := make([]string, instances)

		sendAmount := uint64(1_000_000)
		feeAmount := uint64(1_100_000)
		operationFee := uint64(0)

		var wg sync.WaitGroup

		for i := 0; i < instances; i++ {
			idx := i

			wg.Add(1)

			go func() {
				defer wg.Done()

				testUser := apex.Users[idx]

				beforeSendingAmountDfm, err := apex.GetBalance(ctx, testUser, cardanofw.ChainIDPrime)
				require.NoError(t, err)

				lowerBoundaryDfm := new(big.Int).Sub(beforeSendingAmountDfm[cardanowallet.AdaTokenName], new(big.Int).SetUint64(sendAmount+feeAmount))

				metadata, err := apex.GetChainMust(t, cardanofw.ChainIDPrime).CreateMetadata(
					testUser.GetAddress(cardanofw.ChainIDPrime), cardanofw.ChainIDVector,
					[]sendtx.BridgingTxReceiver{
						{
							Addr:         apex.Users[idx].GetAddress(cardanofw.ChainIDVector),
							Amount:       sendAmount * 10,
							BridgingType: sendtx.BridgingTypeNormal,
						},
					}, feeAmount, operationFee)
				require.NoError(t, err)

				txHashes[idx], err = apex.SubmitTx(
					ctx, cardanofw.ChainIDPrime, testUser,
					apex.PrimeInfo.MultisigAddr, new(big.Int).SetUint64(sendAmount+feeAmount), nil, metadata)
				require.NoError(t, err)

				fmt.Printf("Tx sent. hash: %s, lowerBoundaryDfm: %d, higherBoundaryDfm: %d\n", txHashes[idx], lowerBoundaryDfm, beforeSendingAmountDfm)

				err = apex.WaitForAmountInRange(ctx, apex.Users[i], cardanofw.ChainIDPrime, cardanofw.ChainIDVector, lowerBoundaryDfm,
					beforeSendingAmountDfm[cardanowallet.AdaTokenName], 50, time.Second*30)
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

	apex := cardanofw.SetupAndRunReactorBridge(
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
		brSubmitterUser, new(big.Int).SetUint64(sendAmount), sendtx.BridgingTypeNormal, brSubmitterUser,
	)

	lowerBoundaryDfm := new(big.Int).Sub(beforeSendingAmountDfm[cardanowallet.AdaTokenName], new(big.Int).SetUint64(sendAmount+feeAmount))

	fmt.Printf("Tx sent. hash: %s, lowerBoundaryDfm: %d, higherBoundaryDfm: %d\n", txHash, lowerBoundaryDfm, beforeSendingAmountDfm)

	err = apex.WaitForAmountInRange(ctx, brSubmitterUser, cardanofw.ChainIDPrime, cardanofw.ChainIDVector, lowerBoundaryDfm,
		beforeSendingAmountDfm[cardanowallet.AdaTokenName], 60, time.Second*30)
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

	apex := cardanofw.SetupAndRunReactorBridge(
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
		user, new(big.Int).SetUint64(sendAmount), sendtx.BridgingTypeNormal, user,
	)

	lowerBoundaryDfm := new(big.Int).Sub(beforeSendingAmountDfm[cardanowallet.AdaTokenName], new(big.Int).SetUint64(sendAmount+feeAmount))

	fmt.Printf("Tx sent. hash: %s, lowerBoundaryDfm: %d, higherBoundaryDfm: %d\n", txHash, lowerBoundaryDfm, beforeSendingAmountDfm)

	e2ehelper.ExecuteBridging(
		t, ctx, apex, 1, apex.Users[1:instances+1], []*cardanofw.TestApexUser{user},
		[]string{cardanofw.ChainIDPrime},
		map[string][]string{
			cardanofw.ChainIDPrime: {cardanofw.ChainIDVector},
		},
		sendtx.BridgingTypeNormal,
		new(big.Int).SetUint64(sendAmount2))

	err = apex.WaitForAmountInRange(ctx, user, cardanofw.ChainIDPrime, cardanofw.ChainIDVector, lowerBoundaryDfm,
		beforeSendingAmountDfm[cardanowallet.AdaTokenName], 20, time.Second*30)
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

	apex := cardanofw.SetupAndRunReactorBridge(
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
		user, new(big.Int).SetUint64(sendAmount), sendtx.BridgingTypeNormal, user,
	)

	lowerBoundaryDfm := new(big.Int).Sub(beforeSendingAmountDfm[cardanowallet.AdaTokenName], new(big.Int).SetUint64(sendAmount+feeAmount))

	fmt.Printf("Tx sent. hash: %s, lowerBoundaryDfm: %d, higherBoundaryDfm: %d\n", txHash, lowerBoundaryDfm, beforeSendingAmountDfm)

	err = apex.WaitForAmountInRange(ctx, user, cardanofw.ChainIDPrime, cardanofw.ChainIDVector, lowerBoundaryDfm,
		beforeSendingAmountDfm[cardanowallet.AdaTokenName], 50, time.Second*30)
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

	apex := cardanofw.SetupAndRunReactorBridge(
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
		user, new(big.Int).SetUint64(sendAmount), sendtx.BridgingTypeNormal, user,
	)

	fmt.Printf("Tx sent. hash: %s\n", txHash)

	_, timeout := cardanofw.WaitForBatchState(
		ctx, apex, cardanofw.ChainIDPrime, txHash, apiKey, false, true,
		cardanofw.BridgingRequestStatusInvalidRequest, cardanofw.BridgingRequestStatusInvalidRequest)
	require.False(t, timeout)
}
