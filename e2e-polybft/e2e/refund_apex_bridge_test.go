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
	infrawallet "github.com/Ethernal-Tech/cardano-infrastructure/wallet"
	"github.com/stretchr/testify/require"
)

func TestE2E_ApexRefund_ValidScenarios(t *testing.T) {
	const (
		apiKey  = "test_api_key"
		userCnt = 5

		requestStateTimeoutSec = 600
		retryDelaySec          = 5

		bridgingFee  = uint64(1_000_010)
		operationFee = uint64(0)
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

	primeTestConfig := newTestConfig(t, apex.Config.PrimeConfig, &apex.PrimeInfo, cardanofw.ChainIDVector, bridgingFee, operationFee, "")
	bridgingType := sendtx.BridgingTypeNormal

	t.Run("1. Mismatch submitted and receiver amounts", func(t *testing.T) {
		executeInvalidMismatchSendLovelaceAmount(t, ctx, apex, primeTestConfig, user, requestStateTimeoutSec, retryDelaySec, bridgingType, true)
	})

	t.Run("2. Multiple submitters mismatch submitted and receiver amounts", func(t *testing.T) {
		executeInvalidMismatchSendAmountMultipleInstances(t, ctx, apex, primeTestConfig, requestStateTimeoutSec, retryDelaySec, bridgingType, true)
	})

	t.Run("3. Multiple submitters mismatch submitted and receiver amounts parallel", func(t *testing.T) {
		executeInvalidMismatchSendAmountMultipleInstancesParalel(
			t, ctx, apex, primeTestConfig, requestStateTimeoutSec, retryDelaySec, bridgingType, true)
	})

	t.Run("4. From prime to vector - not enough funds on destination multisig address", func(t *testing.T) {
		const (
			sendAmount   = uint64(100_600_000_000)
			feeAmount    = uint64(1_100_000)
			operationFee = uint64(0)
		)

		beforeSendingAmountDfm, err := apex.GetBalance(ctx, user, cardanofw.ChainIDPrime)
		require.NoError(t, err)

		chain := apex.GetChainMust(t, cardanofw.ChainIDPrime)

		metadata, err := chain.CreateMetadata(
			user.GetAddress(cardanofw.ChainIDPrime), cardanofw.ChainIDVector,
			[]sendtx.BridgingTxReceiver{
				{
					Addr:         user.GetAddress(cardanofw.ChainIDVector),
					Amount:       sendAmount,
					BridgingType: sendtx.BridgingTypeNormal,
				},
			}, feeAmount, operationFee)
		require.NoError(t, err)

		multisigAddr, err := chain.GetAddressToBridgeTo(ctx, sendtx.BridgingTypeNormal)
		require.NoError(t, err)

		txHash, err := apex.SubmitTx(
			ctx, cardanofw.ChainIDPrime, user,
			multisigAddr, new(big.Int).SetUint64(sendAmount+feeAmount), nil, metadata)
		require.NoError(t, err)

		lowerBoundaryDfm := new(big.Int).Sub(beforeSendingAmountDfm[infrawallet.AdaTokenName], new(big.Int).SetUint64(sendAmount+feeAmount))

		fmt.Printf("Tx sent. hash: %s, lowerBoundaryDfm: %d, higherBoundaryDfm: %+v\n", txHash, lowerBoundaryDfm, beforeSendingAmountDfm)

		err = apex.WaitForAmountInRange(ctx, user, cardanofw.ChainIDPrime, cardanofw.ChainIDVector, lowerBoundaryDfm,
			beforeSendingAmountDfm[infrawallet.AdaTokenName], 20, time.Second*30)
		require.NoError(t, err)
	})

	t.Run("5. Submitted invalid metadata - wrong type", func(t *testing.T) {
		executeInvalidMetadataType(
			t, ctx, apex, primeTestConfig, user, requestStateTimeoutSec, retryDelaySec, bridgingType, true)
	})

	t.Run("6. Submitted invalid metadata - invalid destination", func(t *testing.T) {
		executeInvalidDestination(
			t, ctx, apex, primeTestConfig, user, requestStateTimeoutSec, retryDelaySec, bridgingType, true)
	})

	t.Run("7. Submitted invalid metadata - invalid sender", func(t *testing.T) {
		executeInvalidMetadataInvalidSender(
			t, ctx, apex, primeTestConfig, user, requestStateTimeoutSec, retryDelaySec, bridgingType)
	})

	t.Run("8. Submitted invalid metadata - empty receivers", func(t *testing.T) {
		executeInvalidEmptyReceivers(
			t, ctx, apex, primeTestConfig, user, requestStateTimeoutSec, retryDelaySec, bridgingType, true)
	})

	t.Run("9. Submitted with tokens to bridging addr", func(t *testing.T) {
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

		chain := apex.GetChainMust(t, cardanofw.ChainIDPrime)

		metadata, err := chain.CreateMetadata(
			user.GetAddress(cardanofw.ChainIDPrime), cardanofw.ChainIDVector,
			[]sendtx.BridgingTxReceiver{
				{
					Addr:   user.GetAddress(cardanofw.ChainIDVector),
					Amount: sendAmount - feeAmount,
				},
			}, feeAmount, operationFee)
		require.NoError(t, err)

		multisigAddr, err := chain.GetAddressToBridgeTo(ctx, sendtx.BridgingTypeNormal)
		require.NoError(t, err)

		txHash, err := apex.SubmitTx(ctx, cardanofw.ChainIDPrime, brSubmitterUser, multisigAddr,
			new(big.Int).SetUint64(sendAmount), []infrawallet.TokenAmount{*tokensFunded}, metadata)
		require.NoError(t, err)

		lowerBoundaryDfm := new(big.Int).Sub(beforeSendingAmountDfm[infrawallet.AdaTokenName], new(big.Int).SetUint64(sendAmount+feeAmount))

		fmt.Printf("Tx sent. hash: %s, lowerBoundaryDfm: %d, higherBoundaryDfm: %+v\n", txHash, lowerBoundaryDfm, beforeSendingAmountDfm)

		err = apex.WaitForAmountInRange(ctx, brSubmitterUser, cardanofw.ChainIDPrime, cardanofw.ChainIDVector, lowerBoundaryDfm,
			beforeSendingAmountDfm[infrawallet.AdaTokenName], 20, time.Second*30)
		require.NoError(t, err)
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

	lowerBoundaryDfm := new(big.Int).Sub(beforeSendingAmountDfm[infrawallet.AdaTokenName], new(big.Int).SetUint64(sendAmount+feeAmount))

	fmt.Printf("Tx sent. hash: %s, lowerBoundaryDfm: %d, higherBoundaryDfm: %+v\n", txHash, lowerBoundaryDfm, beforeSendingAmountDfm)

	err = apex.WaitForAmountInRange(ctx, brSubmitterUser, cardanofw.ChainIDPrime, cardanofw.ChainIDVector, lowerBoundaryDfm,
		beforeSendingAmountDfm[infrawallet.AdaTokenName], 60, time.Second*30)
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

	lowerBoundaryDfm := new(big.Int).Sub(beforeSendingAmountDfm[infrawallet.AdaTokenName], new(big.Int).SetUint64(sendAmount+feeAmount))

	fmt.Printf("Tx sent. hash: %s, lowerBoundaryDfm: %d, higherBoundaryDfm: %+v\n", txHash, lowerBoundaryDfm, beforeSendingAmountDfm)

	e2ehelper.ExecuteBridging(
		t, ctx, apex, 1, apex.Users[1:instances+1], []*cardanofw.TestApexUser{user},
		[]string{cardanofw.ChainIDPrime},
		map[string][]string{
			cardanofw.ChainIDPrime: {cardanofw.ChainIDVector},
		},
		sendtx.BridgingTypeNormal,
		new(big.Int).SetUint64(sendAmount2))

	err = apex.WaitForAmountInRange(ctx, user, cardanofw.ChainIDPrime, cardanofw.ChainIDVector, lowerBoundaryDfm,
		beforeSendingAmountDfm[infrawallet.AdaTokenName], 20, time.Second*30)
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

	lowerBoundaryDfm := new(big.Int).Sub(beforeSendingAmountDfm[infrawallet.AdaTokenName], new(big.Int).SetUint64(sendAmount+feeAmount))

	fmt.Printf("Tx sent. hash: %s, lowerBoundaryDfm: %d, higherBoundaryDfm: %+v\n", txHash, lowerBoundaryDfm, beforeSendingAmountDfm)

	err = apex.WaitForAmountInRange(ctx, user, cardanofw.ChainIDPrime, cardanofw.ChainIDVector, lowerBoundaryDfm,
		beforeSendingAmountDfm[infrawallet.AdaTokenName], 50, time.Second*30)
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

func TestE2E_ApexRefund_ComplexScenarios_BothBridgingDirectionsSimulation(t *testing.T) {
	type chainUserKey struct {
		src, dst cardanofw.ChainID
		user     *cardanofw.TestApexUser
	}

	const (
		apiKey  = "test_api_key"
		userCnt = 10

		sequentialInstances = 2
		parallelInstances   = 3

		sendAmount     = uint64(1_000_000)
		hugeSendAmount = uint64(100_600_000_000)
		feeAmount      = uint64(1_100_000)

		fundDefundTimeDelay = 60 * time.Second
	)

	var (
		fundDefundAmount = big.NewInt(100) // 100_000_000 in dfm

		chains = []struct {
			src, dst cardanofw.ChainID
		}{
			{
				src: cardanofw.ChainIDPrime,
				dst: cardanofw.ChainIDVector,
			},
			{
				src: cardanofw.ChainIDVector,
				dst: cardanofw.ChainIDPrime,
			},
		}

		userInitialAmounts = make(map[chainUserKey]*big.Int)

		defundCount uint64 = 0

		wgTest     sync.WaitGroup
		wgWithFund sync.WaitGroup

		err error
	)

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	// setup bridge environment
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

	userReceiver := apex.Users[0]
	defundUser := apex.Users[parallelInstances+2]

	for _, chain := range chains {
		for _, usr := range apex.Users[:parallelInstances+3] {
			key := chainUserKey{src: chain.src, dst: chain.dst, user: usr}

			balance, err := apex.GetBalance(ctx, usr, chain.src)
			require.NoError(t, err)

			userInitialAmounts[key] = balance[infrawallet.AdaTokenName]
		}
	}

	doneCh := make(chan struct{})

	wgTest.Add(3)

	// execute valid transactions
	go func() {
		defer wgTest.Done()
		// prime <-> vector
		e2ehelper.ExecuteBridging(
			t, ctx, apex, sequentialInstances,
			apex.Users[1:parallelInstances+1],
			[]*cardanofw.TestApexUser{userReceiver},
			[]string{cardanofw.ChainIDPrime, cardanofw.ChainIDVector},
			map[string][]string{
				cardanofw.ChainIDPrime:  {cardanofw.ChainIDVector},
				cardanofw.ChainIDVector: {cardanofw.ChainIDPrime},
			},
			sendtx.BridgingTypeNormal,
			new(big.Int).SetUint64(sendAmount))
	}()

	// execute invalid transactions that should increase tryCount and should be refunded
	go func() {
		defer wgTest.Done()

		fmt.Printf("\nSending txs with huge unallowed amounts...\n")

		for i, usr := range apex.Users[1 : parallelInstances+1] {
			select {
			case <-ctx.Done():
				return
			case <-time.After(10 * time.Second):
			}

			beforeSendingAmountDfm, err := apex.GetBalance(ctx, usr, cardanofw.ChainIDPrime)
			require.NoError(t, err)

			txHash := apex.SubmitBridgingRequest(t, ctx,
				cardanofw.ChainIDPrime, cardanofw.ChainIDVector,
				usr, new(big.Int).SetUint64(hugeSendAmount), sendtx.BridgingTypeNormal, userReceiver,
			)

			lowerBoundaryDfm := new(big.Int).Sub(beforeSendingAmountDfm[infrawallet.AdaTokenName], new(big.Int).SetUint64(hugeSendAmount+feeAmount))

			fmt.Printf("\nExecutied invalid TX for sender %d from prime to vector.\nTx sent. hash: %s, lowerBoundaryDfm: %d, higherBoundaryDfm: %+v\n",
				i, txHash, lowerBoundaryDfm, beforeSendingAmountDfm)
		}

		fmt.Printf("\nAll txs with huge unallowed amounts sent\n")
	}()

	// execute invalid transactions that should be refunded
	go func() {
		defer wgTest.Done()
		fmt.Printf("\nSending txs with invalid metadata...\n")

		for _, usr := range apex.Users[1 : parallelInstances+1] {
			// wait for some time in order to prevent UTXO double spending
			select {
			case <-ctx.Done():
				return
			case <-time.After(20 * time.Second):
			}

			// prime -> vector
			err := sendWithoutWaitInvalidMetadataWrongType(t, ctx, apex, usr, userReceiver,
				cardanofw.ChainIDPrime, cardanofw.ChainIDVector, sendAmount, feeAmount)
			require.NoError(t, err)

			// vector -> prime
			err = sendWithoutWaitInvalidMetadataWrongType(t, ctx, apex, usr, userReceiver,
				cardanofw.ChainIDVector, cardanofw.ChainIDPrime, sendAmount, feeAmount)
			require.NoError(t, err)
		}

		fmt.Printf("\nAll invalid txs with invalid metadata sent\n")
	}()

	// signal goroutine
	go func() {
		fmt.Printf("\nWaiting for all routines to finish sending txs...\n")
		wgTest.Wait() // wait for the 3 test routines

		fmt.Printf("\nWaiting for receivers on to receive their bridged funds...\n")

		for _, chain := range chains {
			key := chainUserKey{src: chain.src, dst: chain.dst, user: defundUser}

			// expectedAmount = initial + (parallelInstances * sequentialInstances * sendAmount)
			expectedAmount := new(big.Int).Add(userInitialAmounts[key], new(big.Int).SetUint64(parallelInstances*sequentialInstances*sendAmount))

			err := apex.WaitForExactAmount(ctx, userReceiver, chain.src, chain.dst, expectedAmount, 50, 200)
			require.NoError(t, err)
		}

		fmt.Printf("\nAll receivers received their bridged funds...\n")

		close(doneCh) // tell fundDefund to shut down
	}()

	// run fund/defund process
	wgWithFund.Add(1)

	go func(ctx context.Context) {
		defer wgWithFund.Done()
		defer fmt.Println("exiting fund/defund routine...")

		var (
			executeFund = true

			defundPrevAmounts     = make(map[chainStageKey]*big.Int, len(chains))
			defundExpectedAmounts = make(map[chainStageKey]*big.Int, len(chains))
			defundReceivers       = make(map[chainStageKey]*cardanofw.TestApexUser, len(chains))
		)

		for {
			select {
			case <-doneCh:
				// other routines finished - stop the fund/defund process
				return
			default:
				select {
				case <-ctx.Done():
					return
				case <-time.After(fundDefundTimeDelay):
				}

				if executeFund {
					executeFund = false

					fundWallets(t, ctx, apex, []string{cardanofw.ChainIDPrime, cardanofw.ChainIDVector}, fundDefundAmount)
					require.NoError(t, err)
				} else {
					executeFund = true

					for _, chain := range chains {
						chainKey := chainStageKey{chain: chain.src, destChain: chain.dst, receiver: 0}

						balance, err := apex.GetBalance(ctx, defundUser, chain.src)
						require.NoError(t, err)

						defundPrevAmounts[chainKey] = balance[infrawallet.AdaTokenName]

						defundExpectedAmounts[chainKey] = cardanofw.ApexToDfm(fundDefundAmount)

						defundReceivers[chainKey] = defundUser
					}

					defundWallets(t, ctx, apex, []string{cardanofw.ChainIDPrime, cardanofw.ChainIDVector}, defundUser, fundDefundAmount, defundPrevAmounts,
						defundExpectedAmounts, defundReceivers)

					defundCount++
				}
			}
		}
	}(ctx)

	fmt.Printf("\nWaiting for sender users to receive their refunds...\n")

	for i, usr := range apex.Users[1 : parallelInstances+1] {
		key := chainUserKey{src: cardanofw.ChainIDPrime, dst: cardanofw.ChainIDVector, user: usr}

		// minExpectedAmount = initial - (2*sendAmount+sendAmount1+3*feeAmount)
		minExpectedAmount := new(big.Int).Sub(userInitialAmounts[key], new(big.Int).SetUint64(2*sendAmount+hugeSendAmount+3*feeAmount))
		maxExpectedAmount := new(big.Int).Sub(userInitialAmounts[key], new(big.Int).SetUint64(sendAmount+feeAmount))

		fmt.Printf("\nWaiting for sender %d to receive his refunds...\n\tMin expected amount: %d", i, minExpectedAmount)
		fmt.Printf("\n\tMax expected amount: %d\n", maxExpectedAmount)

		err = apex.WaitForAmountInRange(ctx, usr, cardanofw.ChainIDPrime, cardanofw.ChainIDVector, minExpectedAmount,
			maxExpectedAmount, 20, time.Second*30)
		require.NoError(t, err)

		actualAmount, err := apex.GetBalance(ctx, usr, cardanofw.ChainIDPrime)
		require.NoError(t, err)

		fmt.Printf("\nSender %d:\n\tActual amount: %+v\n", i, actualAmount)

		fmt.Printf("\nSender %d received his refunds\n", i)
	}

	fmt.Printf("\nAll sender users received their refunds...\n")

	fmt.Printf("\nWaiting for defund users to receive their funds...\n")

	for _, chain := range chains {
		key := chainUserKey{src: chain.src, dst: chain.dst, user: defundUser}

		// expectedAmount = initial + (defundCount * fundDefundAmount * 1_000_000)
		expectedAmount := new(big.Int).Add(userInitialAmounts[key], new(big.Int).Mul(new(big.Int).SetUint64(defundCount), cardanofw.ApexToDfm(fundDefundAmount)))

		err = apex.WaitForExactAmount(ctx, defundUser, chain.src, chain.dst, expectedAmount, 50, 200)
		require.NoError(t, err)
	}

	fmt.Printf("\nAll defund users received their funds...\n")

	wgWithFund.Wait()
}
