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
	"github.com/stretchr/testify/require"
)

func TestE2E_SkylineBridge_InvalidScenarios_RefundEnabled(t *testing.T) {
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

	primeToken, err := cardanofw.FundUserWithToken(
		ctx, apex, cardanofw.ChainIDPrime,
		apex.PrimeInfo.GenesisWallet, user,
		cardanofw.DefaultTokenName, cardanofw.DefaultTokenMintAmount,
		uint64(10_000_000), cardanofw.DefaultTokenMintAmount)
	require.NoError(t, err)

	cardanoToken, err := cardanofw.FundUserWithToken(
		ctx, apex, cardanofw.ChainIDCardano,
		apex.CardanoInfo.GenesisWallet, user,
		cardanofw.DefaultTokenName, cardanofw.DefaultTokenMintAmount,
		uint64(10_000_000), cardanofw.DefaultTokenMintAmount)
	require.NoError(t, err)

	primeTestConfig := newTestConfig(t, apex.Config.PrimeConfig, &apex.PrimeInfo, cardanofw.ChainIDCardano, bridgingFee, operationFee, primeToken.TokenName())
	cardanoTestConfig := newTestConfig(t, apex.Config.CardanoConfig, &apex.CardanoInfo, cardanofw.ChainIDPrime, bridgingFee, operationFee, cardanoToken.TokenName())

	transactionTypes := []sendtx.BridgingType{
		sendtx.BridgingTypeCurrencyOnSource, sendtx.BridgingTypeNativeTokenOnSource,
	}

	t.Run("1.1 Prime -> Cardano - Mismatch submitted and receiver amounts", func(t *testing.T) {
		for _, txType := range transactionTypes {
			executeInvalidMismatchSendLovelaceAmount(t, ctx, apex, primeTestConfig, 0, txType, true)
		}
	})

	t.Run("1.2 Cardano -> Prime - Mismatch submitted and receiver amounts", func(t *testing.T) {
		for _, txType := range transactionTypes {
			executeInvalidMismatchSendLovelaceAmount(t, ctx, apex, cardanoTestConfig, 0, txType, true)
		}
	})

	t.Run("2.1 Prime -> Cardano Multiple submitters mismatch submitted and receiver amounts", func(t *testing.T) {
		for _, txType := range transactionTypes {
			executeInvalidMismatchSendAmountMultipleInstances(t, ctx, apex, user, primeTestConfig, txType)
		}
	})

	t.Run("2.2 Cardano -> Prime Multiple submitters mismatch submitted and receiver amounts", func(t *testing.T) {
		for _, txType := range transactionTypes {
			executeInvalidMismatchSendAmountMultipleInstances(t, ctx, apex, user, cardanoTestConfig, txType)
		}
	})

	t.Run("3.1 Prime -> Cardano - Multiple submitters mismatch submitted and receiver amounts parallel", func(t *testing.T) {
		for _, txType := range transactionTypes {
			executeInvalidMismatchSendAndReceiveAmountParallel(t, ctx, apex, primeTestConfig, 0, txType, true)
		}
	})

	t.Run("3.2 Cardano -> Prime - Multiple submitters mismatch submitted and receiver amounts parallel", func(t *testing.T) {
		for _, txType := range transactionTypes {
			executeInvalidMismatchSendAndReceiveAmountParallel(t, ctx, apex, cardanoTestConfig, 0, txType, true)
		}
	})

	t.Run("4.1 Prime -> Cardano - Submitted invalid metadata - sliced off", func(t *testing.T) {
		for _, txType := range transactionTypes {
			executeInvalidMetadataSlicedOff(t, ctx, apex, primeTestConfig, 0, txType, true)
		}
	})

	t.Run("4.2 Cardano -> Prime - Submitted invalid metadata - sliced off", func(t *testing.T) {
		for _, txType := range transactionTypes {
			executeInvalidMetadataSlicedOff(t, ctx, apex, cardanoTestConfig, 0, txType, true)
		}
	})

	t.Run("5.1 Prime -> Cardano - Submitted invalid metadata - wrong type", func(t *testing.T) {
		for _, txType := range transactionTypes {
			executeInvalidMetadataType(t, ctx, apex, primeTestConfig, 60, txType, true)
		}
	})

	t.Run("5.2 Cardano -> Prime - Submitted invalid metadata - wrong type", func(t *testing.T) {
		for _, txType := range transactionTypes {
			executeInvalidMetadataType(t, ctx, apex, cardanoTestConfig, 60, txType, true)
		}
	})

	t.Run("6.1 Prime -> Cardano - Submitted invalid metadata - invalid destination", func(t *testing.T) {
		for _, txType := range transactionTypes {
			executeInvalidDestination(t, ctx, apex, primeTestConfig, 0, txType, true)
		}
	})

	t.Run("6.2 Cardano -> Prime - Submitted invalid metadata - invalid destination", func(t *testing.T) {
		for _, txType := range transactionTypes {
			executeInvalidDestination(t, ctx, apex, cardanoTestConfig, 0, txType, true)
		}
	})

	t.Run("7.1 Prime -> Cardano - Submitted invalid metadata - invalid sender", func(t *testing.T) {
		for _, txType := range transactionTypes {
			executeInvalidMetadataSender(t, ctx, apex, primeTestConfig, 0, txType, true)
		}
	})

	t.Run("7.2 Cardano -> Prime - Submitted invalid metadata - invalid sender", func(t *testing.T) {
		for _, txType := range transactionTypes {
			executeInvalidMetadataSender(t, ctx, apex, cardanoTestConfig, 0, txType, true)
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
		for _, txType := range transactionTypes {
			executeInvalidFeeReceiverAddr(t, ctx, apex, primeTestConfig, 0, txType, true)
		}
	})

	t.Run("9.2 Cardano -> Prime - Submitted invalid metadata - invalid fee receiver address - token on source", func(t *testing.T) {
		for _, txType := range transactionTypes {
			executeInvalidFeeReceiverAddr(t, ctx, apex, cardanoTestConfig, 0, txType, true)
		}
	})

	t.Run("10.1 Prime -> Cardano - Submitted invalid metadata - empty receivers", func(t *testing.T) {
		for _, txType := range transactionTypes {
			executeInvalidEmptyReceivers(t, ctx, apex, primeTestConfig, 0, txType, true)
		}
	})

	t.Run("10.2 Cardano -> Prime - Submitted invalid metadata - empty receivers", func(t *testing.T) {
		for _, txType := range transactionTypes {
			executeInvalidEmptyReceivers(t, ctx, apex, cardanoTestConfig, 0, txType, true)
		}
	})

	t.Run("11. Submitted with unknown tokens to bridging addr", func(t *testing.T) {
		sendAmount := uint64(1_500_000)
		user := apex.Users[userCnt-1]
		minterWallet, _ := user.GetCardanoWallet(cardanofw.ChainIDPrime)

		tokensFunded, err := cardanofw.FundUserWithToken(
			ctx, apex, cardanofw.ChainIDPrime,
			minterWallet, user,
			cardanofw.DefaultTokenName, cardanofw.DefaultTokenMintAmount,
			uint64(1_500_000), uint64(1_000_000))
		require.NoError(t, err)

		executeInvalidSendUnknownToken(t, ctx, apex, user, primeTestConfig, sendAmount, *tokensFunded, 0, true)
	})

	t.Run("12. Submitted invalid metadata - invalid send amount - token on source", func(t *testing.T) {
		user, err := cardanofw.NewTestApexUser(cardanofw.NewApexNetworkTypesFromSystem(apex))
		require.NoError(t, err)

		tokensFunded, err := cardanofw.FundUserWithToken(
			ctx, apex, cardanofw.ChainIDPrime,
			apex.PrimeInfo.GenesisWallet, user,
			cardanofw.DefaultTokenName, cardanofw.DefaultTokenMintAmount,
			uint64(10_000_000), uint64(1_123_000))
		require.NoError(t, err)

		executeInvalidMismatchSendNativeTokenAmount(t, ctx, apex, user, primeTestConfig, *tokensFunded, 0, true)
	})
}

func TestE2E_SkylineBridge_Over_Max_Allowed_To_Bridge_RefundEnabled(t *testing.T) {
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

			fmt.Printf("Tx sent. hash: %s, lowerBoundaryDfm: %d, higherBoundaryDfm: %d\n", txHashes[idx], lowerBoundaryDfm, beforeSendingAmountDfm)

			err := apex.WaitForAmountInRange(ctx, user, br.dest, br.src, lowerBoundaryDfm, beforeSendingAmountDfm[idx]["lovelace"],
				20, time.Second*30)
			require.NoError(t, err)
		}()
	}

	wg.Wait()
}
