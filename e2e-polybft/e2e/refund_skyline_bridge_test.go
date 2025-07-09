package e2e

import (
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

func TestE2E_SkylineRefund_ValidScenarios(t *testing.T) {
	const (
		apiKey  = "test_api_key"
		userCnt = 15

		bridgingFee  = uint64(1_000_010)
		operationFee = uint64(0)
	)

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	primeConfig, cardanoConfig := cardanofw.NewPrimeChainConfig(), cardanofw.NewCardanoChainConfig(true)
	primeConfig.FundTokenAmount = 1_000_000_000
	cardanoConfig.FundTokenAmount = 1_000_000_000

	apex := cardanofw.SetupAndRunSkylineBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithUserCnt(userCnt),
		cardanofw.WithCardanoConfig(cardanoConfig),
		cardanofw.WithPrimeConfig(primeConfig),
		cardanofw.WithCustomConfigHandlers(func(_ *cardanofw.ApexSystem, mp map[string]interface{}) {
			tryCountLimitsSettings := cardanofw.GetMapFromInterfaceKey(mp, "tryCountLimits")
			tryCountLimitsSettings["maxBatchTryCount"] = 1
			tryCountLimitsSettings["maxSubmitTryCount"] = 2
		}, nil),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	user := apex.Users[userCnt-1]
	fmt.Println("prime user addr: ", user.PrimeAddress)
	fmt.Println("cardano user addr: ", user.CardanoAddress)
	fmt.Println("prime multisig addr: ", apex.PrimeInfo.MultisigAddr)
	fmt.Println("prime fee addr: ", apex.PrimeInfo.FeeAddr)
	fmt.Printf("prime socket path: %s\n", apex.PrimeInfo.SocketPath)
	fmt.Println("cardano multisig addr: ", apex.CardanoInfo.MultisigAddr)
	fmt.Println("cardano fee addr: ", apex.CardanoInfo.FeeAddr)
	fmt.Printf("cardano socket path: %s\n", apex.CardanoInfo.SocketPath)

	var (
		primeToken   *wallet.TokenAmount
		cardanoToken *wallet.TokenAmount
		err          error
	)

	for i := 0; i < userCnt; i++ {
		primeToken, err = cardanofw.FundUserWithToken(
			ctx, apex, cardanofw.ChainIDPrime,
			apex.PrimeInfo.GenesisWallet, apex.Users[i],
			cardanofw.DefaultTokenName, cardanofw.DefaultTokenMintAmount,
			uint64(10_000_000), cardanofw.DefaultTokenMintAmount)
		require.NoError(t, err)

		cardanoToken, err = cardanofw.FundUserWithToken(
			ctx, apex, cardanofw.ChainIDCardano,
			apex.CardanoInfo.GenesisWallet, apex.Users[i],
			cardanofw.DefaultTokenName, cardanofw.DefaultTokenMintAmount,
			uint64(10_000_000), cardanofw.DefaultTokenMintAmount)
		require.NoError(t, err)
	}

	primeTestConfig := newTestConfig(t, apex.Config.PrimeConfig, &apex.PrimeInfo, cardanofw.ChainIDCardano, bridgingFee,
		operationFee, primeToken.TokenName())
	cardanoTestConfig := newTestConfig(t, apex.Config.CardanoConfig, &apex.CardanoInfo, cardanofw.ChainIDPrime,
		bridgingFee, operationFee, cardanoToken.TokenName())

	fmt.Printf("Prime test config: %+v\n", primeToken)
	fmt.Printf("Cardano test config: %+v\n", cardanoToken)

	transactionTypes := []sendtx.BridgingType{
		sendtx.BridgingTypeCurrencyOnSource,
		sendtx.BridgingTypeNativeTokenOnSource,
	}

	t.Run("1.1 Prime -> Cardano - Mismatch submitted and receiver amounts", func(t *testing.T) {
		for _, txType := range transactionTypes {
			executeInvalidMismatchSendLovelaceAmount(t, ctx, apex, primeTestConfig, user, 0, txType, true)
		}
	})

	t.Run("1.2 Cardano -> Prime - Mismatch submitted and receiver amounts", func(t *testing.T) {
		for _, txType := range transactionTypes {
			executeInvalidMismatchSendLovelaceAmount(t, ctx, apex, cardanoTestConfig, user, 0, txType, true)
		}
	})

	t.Run("2.1 Prime -> Cardano - Multiple submitters mismatch submitted and receiver amounts", func(t *testing.T) {
		for _, txType := range transactionTypes {
			executeInvalidMismatchSendAmountMultipleInstances(t, ctx, apex, primeTestConfig, 0, txType, true)
		}
	})

	t.Run("2.2 Cardano -> Prime - Multiple submitters mismatch submitted and receiver amounts", func(t *testing.T) {
		for _, txType := range transactionTypes {
			executeInvalidMismatchSendAmountMultipleInstances(t, ctx, apex, cardanoTestConfig, 0, txType, true)
		}
	})

	t.Run("3.1 Prime -> Cardano - Multiple submitters mismatch submitted and receiver amounts parallel", func(t *testing.T) {
		for _, txType := range transactionTypes {
			executeInvalidMismatchSendAmountMultipleInstancesParalel(t, ctx, apex, primeTestConfig, 0, txType, true)
		}
	})

	t.Run("3.2 Cardano -> Prime - Multiple submitters mismatch submitted and receiver amounts parallel", func(t *testing.T) {
		for _, txType := range transactionTypes {
			executeInvalidMismatchSendAmountMultipleInstancesParalel(t, ctx, apex, cardanoTestConfig, 0, txType, true)
		}
	})

	t.Run("4.1 Prime -> Cardano - Submitted invalid metadata - sliced off", func(t *testing.T) {
		for _, txType := range transactionTypes {
			executeInvalidMetadataSlicedOff(t, ctx, apex, primeTestConfig, txType)
		}
	})

	t.Run("4.2 Cardano -> Prime - Submitted invalid metadata - sliced off", func(t *testing.T) {
		for _, txType := range transactionTypes {
			executeInvalidMetadataSlicedOff(t, ctx, apex, cardanoTestConfig, txType)
		}
	})

	t.Run("5.1 Prime -> Cardano - Submitted invalid metadata - wrong type", func(t *testing.T) {
		for _, txType := range transactionTypes {
			executeInvalidMetadataType(t, ctx, apex, primeTestConfig, user, 0, txType, true)
		}
	})

	t.Run("5.2 Cardano -> Prime - Submitted invalid metadata - wrong type", func(t *testing.T) {
		for _, txType := range transactionTypes {
			executeInvalidMetadataType(t, ctx, apex, cardanoTestConfig, user, 0, txType, true)
		}
	})

	t.Run("6.1 Prime -> Cardano - Submitted invalid metadata - invalid destination", func(t *testing.T) {
		for _, txType := range transactionTypes {
			executeInvalidDestination(t, ctx, apex, primeTestConfig, user, 0, txType, true)
		}
	})

	t.Run("6.2 Cardano -> Prime - Submitted invalid metadata - invalid destination", func(t *testing.T) {
		for _, txType := range transactionTypes {
			executeInvalidDestination(t, ctx, apex, cardanoTestConfig, user, 0, txType, true)
		}
	})

	t.Run("7.1 Prime -> Cardano - Submitted invalid metadata - invalid sender", func(t *testing.T) {
		for _, txType := range transactionTypes {
			executeInvalidMetadataInvalidSender(t, ctx, apex, primeTestConfig, user, 0, txType)
		}
	})

	t.Run("7.2 Cardano -> Prime - Submitted invalid metadata - invalid sender", func(t *testing.T) {
		for _, txType := range transactionTypes {
			executeInvalidMetadataInvalidSender(t, ctx, apex, cardanoTestConfig, user, 0, txType)
		}
	})

	t.Run("8.1 Prime -> Cardano - Submitted invalid metadata - invalid bridging fee", func(t *testing.T) {
		for _, txType := range transactionTypes {
			executeInvalidBridgingFee(t, ctx, apex, primeTestConfig, 0, txType, true)
		}
	})

	t.Run("8.2 Cardano -> Prime - Submitted invalid metadata - invalid bridging fee", func(t *testing.T) {
		for _, txType := range transactionTypes {
			executeInvalidBridgingFee(t, ctx, apex, cardanoTestConfig, 0, txType, true)
		}
	})

	t.Run("9.1 Prime -> Cardano - Submitted invalid metadata - invalid fee receiver address - token on source", func(t *testing.T) {
		executeInvalidFeeReceiverAddr(t, ctx, apex, primeTestConfig, 0, sendtx.BridgingTypeNativeTokenOnSource, true)
	})

	t.Run("9.2 Cardano -> Prime - Submitted invalid metadata - invalid fee receiver address - token on source", func(t *testing.T) {
		executeInvalidFeeReceiverAddr(t, ctx, apex, cardanoTestConfig, 0, sendtx.BridgingTypeNativeTokenOnSource, true)
	})

	t.Run("10.1 Prime -> Cardano - Submitted invalid metadata - empty receivers", func(t *testing.T) {
		for _, txType := range transactionTypes {
			executeInvalidEmptyReceivers(t, ctx, apex, primeTestConfig, user, 0, txType, true)
		}
	})

	t.Run("10.2 Cardano -> Prime - Submitted invalid metadata - empty receivers", func(t *testing.T) {
		for _, txType := range transactionTypes {
			executeInvalidEmptyReceivers(t, ctx, apex, cardanoTestConfig, user, 0, txType, true)
		}
	})

	t.Run("11. Submitted with unknown tokens to bridging addr", func(t *testing.T) {
		user := apex.Users[0]
		minterWallet, _ := user.GetCardanoWallet(cardanofw.ChainIDPrime)

		tokensFunded, err := cardanofw.FundUserWithToken(
			ctx, apex, cardanofw.ChainIDPrime,
			minterWallet, user,
			cardanofw.DefaultTokenName, cardanofw.DefaultTokenMintAmount,
			uint64(1_500_000), uint64(1_000_000))
		require.NoError(t, err)

		executeInvalidSendUnknownToken(t, ctx, apex, user, primeTestConfig, *tokensFunded, 0, true)
	})

	t.Run("12. Submitted invalid metadata - invalid send amount - token on source", func(t *testing.T) {
		user, err := cardanofw.NewTestApexUser(cardanofw.NewApexNetworkTypesFromSystem(apex))
		require.NoError(t, err)

		_, err = cardanofw.FundUserWithToken(
			ctx, apex, cardanofw.ChainIDPrime,
			apex.PrimeInfo.GenesisWallet, user,
			cardanofw.DefaultTokenName, cardanofw.DefaultTokenMintAmount,
			uint64(10_000_000), uint64(1_123_000))
		require.NoError(t, err)

		executeInvalidMismatchSendNativeTokenAmount(t, ctx, apex, user, primeTestConfig, 0, true)
	})
}

func TestE2E_SkylineRefund_Over_Max_Allowed_To_Bridge(t *testing.T) {
	if cardanofw.ShouldSkipE2RRedundantTests() {
		t.Skip()
	}

	const (
		apiKey = "test_api_key"
	)

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	primeConfig, cardanoConfig := cardanofw.NewPrimeChainConfig(), cardanofw.NewCardanoChainConfig(true)
	primeConfig.FundTokenAmount = 1_000_000_000
	cardanoConfig.FundTokenAmount = 1_000_000_000

	apex := cardanofw.SetupAndRunSkylineBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithUserCnt(1),
		cardanofw.WithCardanoConfig(cardanoConfig),
		cardanofw.WithPrimeConfig(primeConfig),
		cardanofw.WithCustomConfigHandlers(func(_ *cardanofw.ApexSystem, mp map[string]interface{}) {
			setting := cardanofw.GetMapFromInterfaceKey(mp, "bridgingSettings")
			setting["maxAmountAllowedToBridge"] = new(big.Int).SetUint64(5_000_000)
		}, nil),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	var (
		user             = apex.Users[0]
		feeAmount        = uint64(1_000_010)
		apexSendAmount   = cardanofw.ApexToDfm(big.NewInt(10))
		bridgingRequests = []struct {
			src    string
			dest   string
			sender *cardanofw.TestApexUser
		}{
			{src: cardanofw.ChainIDPrime, dest: cardanofw.ChainIDCardano, sender: apex.Users[0]},
			{src: cardanofw.ChainIDCardano, dest: cardanofw.ChainIDPrime, sender: apex.Users[0]},
		}
		txHashes = make([]string, len(bridgingRequests))
	)

	var wg sync.WaitGroup

	beforeSendingAmountDfm := make([]map[string]*big.Int, len(bridgingRequests))

	for idx, br := range bridgingRequests {
		wg.Add(1)

		go func(i int, src string, dest string, sender *cardanofw.TestApexUser) {
			defer wg.Done()

			var err error

			beforeSendingAmountDfm[idx], err = apex.GetBalance(ctx, user, src)
			require.NoError(t, err)

			txHashes[i] = apex.SubmitBridgingRequest(t, ctx, src, dest, sender, apexSendAmount, sendtx.BridgingTypeCurrencyOnSource,
				user)
			fmt.Printf("Bridging request: %v to %v sent. hash: %s\n", src, dest, txHashes[i])
		}(idx, br.src, br.dest, br.sender)
	}

	wg.Wait()

	for idx, br := range bridgingRequests {
		wg.Add(1)

		go func() {
			defer wg.Done()

			lowerBoundaryDfm := new(big.Int).Sub(beforeSendingAmountDfm[idx]["lovelace"], new(big.Int).Add(apexSendAmount, new(big.Int).SetUint64(feeAmount)))

			fmt.Printf("Tx sent. hash: %s, lowerBoundaryDfm: %d, higherBoundaryDfm: %+v\n", txHashes[idx], lowerBoundaryDfm, beforeSendingAmountDfm)

			err := apex.WaitForAmountInRange(ctx, user, br.dest, br.src, lowerBoundaryDfm, beforeSendingAmountDfm[idx]["lovelace"],
				20, time.Second*30)
			require.NoError(t, err)
		}()
	}

	wg.Wait()
}

func TestE2E_SkylineRefund_Over_Max_Tokens_Allowed_To_Bridge(t *testing.T) {
	if cardanofw.ShouldSkipE2RRedundantTests() {
		t.Skip()
	}

	const (
		apiKey = "test_api_key"
	)

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	primeConfig, cardanoConfig := cardanofw.NewPrimeChainConfig(), cardanofw.NewCardanoChainConfig(true)
	primeConfig.FundTokenAmount = 1_000_000_000
	cardanoConfig.FundTokenAmount = 1_000_000_000

	apex := cardanofw.SetupAndRunSkylineBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithUserCnt(1),
		cardanofw.WithCardanoConfig(cardanoConfig),
		cardanofw.WithPrimeConfig(primeConfig),
		cardanofw.WithCustomConfigHandlers(func(_ *cardanofw.ApexSystem, mp map[string]interface{}) {
			setting := cardanofw.GetMapFromInterfaceKey(mp, "bridgingSettings")
			setting["maxTokenAmountAllowedToBridge"] = new(big.Int).SetUint64(5_000_000)
		}, nil),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	var (
		user             = apex.Users[0]
		apexSendAmount   = cardanofw.ApexToDfm(big.NewInt(10))
		bridgingRequests = []struct {
			src    string
			dest   string
			sender *cardanofw.TestApexUser
		}{
			{src: cardanofw.ChainIDPrime, dest: cardanofw.ChainIDCardano, sender: apex.Users[0]},
			{src: cardanofw.ChainIDCardano, dest: cardanofw.ChainIDPrime, sender: apex.Users[0]},
		}
		initialBalances = map[string]map[string]*big.Int{}
	)

	_, err := cardanofw.FundUserWithToken(
		ctx, apex, cardanofw.ChainIDPrime,
		apex.PrimeInfo.GenesisWallet, apex.Users[0],
		cardanofw.DefaultTokenName, cardanofw.DefaultTokenMintAmount,
		uint64(5_000_000), uint64(1_000_000_000))
	require.NoError(t, err)

	_, err = cardanofw.FundUserWithToken(
		ctx, apex, cardanofw.ChainIDCardano,
		apex.CardanoInfo.GenesisWallet, apex.Users[0],
		cardanofw.DefaultTokenName, cardanofw.DefaultTokenMintAmount,
		uint64(5_000_000), uint64(1_000_000_000))
	require.NoError(t, err)

	var wg sync.WaitGroup

	for idx, br := range bridgingRequests {
		wg.Add(1)

		go func(i int, src, dest string, sender *cardanofw.TestApexUser) {
			defer wg.Done()

			initialBalances[src], err = apex.GetBalance(ctx, sender, src)
			require.NoError(t, err)

			txHash := apex.SubmitBridgingRequest(
				t, ctx, src, dest, sender, apexSendAmount, sendtx.BridgingTypeNativeTokenOnSource, user)
			fmt.Printf("Bridging request: %v to %v sent. hash: %s\n", src, dest, txHash)
		}(idx, br.src, br.dest, br.sender)
	}

	wg.Wait()

	for _, br := range bridgingRequests {
		wg.Add(1)

		go func(src, dest string, sender *cardanofw.TestApexUser) {
			defer wg.Done()

			tokenName := apex.GetTokenNameForChains(src, dest)
			tokenBalance := initialBalances[br.src][tokenName]

			err := apex.WaitForExactAmount(ctx, br.sender, br.src, br.dest, tokenBalance, 30, 30*time.Second, true)
			require.NoError(t, err)
		}(br.src, br.dest, br.sender)
	}

	wg.Wait()
}

func TestE2E_SkylineRefund_DisabledDirection(t *testing.T) {
	if cardanofw.ShouldSkipE2RRedundantTests() {
		t.Skip()
	}

	type bridgingRequest struct {
		src         string
		dest        string
		sender      *cardanofw.TestApexUser
		requestType sendtx.BridgingType
		isValid     bool
	}

	const apiKey = "test_api_key"

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	primeConfig, cardanoConfig := cardanofw.NewPrimeChainConfig(), cardanofw.NewCardanoChainConfig(true)
	primeConfig.FundTokenAmount = 0 // very important otherwise HWIC wont work

	apex := cardanofw.SetupAndRunSkylineBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithUserCnt(3),
		cardanofw.WithCardanoConfig(cardanoConfig),
		cardanofw.WithPrimeConfig(primeConfig),
		cardanofw.WithCustomConfigHandlers(func(_ *cardanofw.ApexSystem, mp map[string]interface{}) {
			primeSettings := cardanofw.GetMapFromInterfaceKey(mp, "cardanoChains", "prime")
			primeSettings["nativeTokens"] = nil
		}, nil),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	var (
		user             = apex.Users[0]
		sendAmount       = cardanofw.ApexToDfm(big.NewInt(2))
		bridgingRequests = []bridgingRequest{
			{src: cardanofw.ChainIDPrime, dest: cardanofw.ChainIDCardano, sender: apex.Users[1], requestType: sendtx.BridgingTypeCurrencyOnSource, isValid: true},
			{src: cardanofw.ChainIDPrime, dest: cardanofw.ChainIDCardano, sender: apex.Users[2], requestType: sendtx.BridgingTypeNativeTokenOnSource, isValid: false},
			{src: cardanofw.ChainIDCardano, dest: cardanofw.ChainIDPrime, sender: apex.Users[1], requestType: sendtx.BridgingTypeCurrencyOnSource, isValid: false},
			{src: cardanofw.ChainIDCardano, dest: cardanofw.ChainIDPrime, sender: apex.Users[2], requestType: sendtx.BridgingTypeNativeTokenOnSource, isValid: true},
		}
		txHashes = make([]string, len(bridgingRequests))

		feeAmount = new(big.Int).SetUint64(cardanoConfig.MinBridgingFee)

		// map that contains initial balances of users that will receive refunds, per chains
		initialBalance = map[string]map[string]*big.Int{}

		err error
	)

	var wg sync.WaitGroup

	for idx, br := range bridgingRequests {
		wg.Add(1)

		go func(i int, br bridgingRequest) {
			defer wg.Done()

			if br.requestType == sendtx.BridgingTypeNativeTokenOnSource {
				token, err := cardanofw.FundUserWithToken(
					ctx, apex, br.src,
					apex.GetCardanoInfo(br.src).GenesisWallet, br.sender,
					cardanofw.DefaultTokenName, cardanofw.DefaultTokenMintAmount,
					uint64(10_000_000), uint64(100_000_000))
				require.NoError(t, err)

				fmt.Printf("Added new token for chain: %s. Token: %s\n", br.src, token.TokenName())
			}

			if !br.isValid {
				initialBalance[br.sender.GetAddress(br.src)], err = apex.GetBalance(ctx, br.sender, br.src)
				require.NoError(t, err)
			}

			txHashes[i] = apex.SubmitBridgingRequest(t, ctx, br.src, br.dest, br.sender, sendAmount, br.requestType, user)
			fmt.Printf("Bridging request: %v to %v sent %v. hash: %s\n", br.src, br.dest, br.requestType, txHashes[i])
		}(idx, br)
	}

	wg.Wait()

	for idx, br := range bridgingRequests {
		wg.Add(1)

		go func(br bridgingRequest, txHash string) {
			defer wg.Done()

			if !br.isValid {
				tokenName := wallet.AdaTokenName
				isNativeToken := br.requestType != sendtx.BridgingTypeCurrencyOnSource
				userSpending := new(big.Int).Set(sendAmount)
				addr := br.sender.GetAddress(br.src)

				if br.requestType == sendtx.BridgingTypeCurrencyOnSource {
					userSpending.Add(userSpending, feeAmount)
				} else {
					tokenName = apex.GetTokenNameForChains(br.src, br.dest)
				}

				initialAmount := initialBalance[addr][tokenName]

				// minExpected = initial - (sendAmount + feeAmount)
				minExpectedAmount := new(big.Int).Sub(initialAmount, userSpending)

				require.NoError(t,
					apex.WaitForAmountInRange(ctx, br.sender, br.src, br.dest, minExpectedAmount, initialAmount, 20, 30*time.Second, isNativeToken))
			} else {
				state, timeout := "ExecutedOnDestination", uint(60*8)

				_, err := cardanofw.WaitForRequestStates(ctx, apex, br.src, txHash, apiKey, []string{state}, timeout)
				require.NoError(t, err)

				fmt.Printf("%s is %s\n", txHash, state)
			}
		}(br, txHashes[idx])
	}

	wg.Wait()
}
