package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/e2e-polybft/cardanofw"
	infrawallet "github.com/Ethernal-Tech/cardano-infrastructure/wallet"
	"github.com/stretchr/testify/require"
)

func TestE2E_Apex_Bridge_Refund_ValidScenarios(t *testing.T) {
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
		if cardanofw.ShouldSkipE2RRedundantTests() {
			t.Skip()
		}

		const (
			sendAmount = uint64(100_600_000_000)
			feeAmount  = uint64(1_100_000)
			instances  = 1
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

		fmt.Printf("Tx sent. hash: %s\nlowerBoundaryDfm: %d, higherBoundaryDfm: %d\n", txHash, lowerBoundaryDfm, beforeSendingAmountDfm)

		err = apex.WaitForAmountInRange(ctx, user, cardanofw.ChainIDPrime, lowerBoundaryDfm, beforeSendingAmountDfm,
			20, time.Second*30)
		require.NoError(t, err)

		newAmountDfm, err := apex.GetBalance(ctx, user, cardanofw.ChainIDPrime)
		require.NoError(t, err)

		fmt.Println("newAmountDfm", newAmountDfm)
	})

	t.Run("Submitted invalid metadata - wrong type", func(t *testing.T) {
		sendAmount := uint64(1_000_000)
		feeAmount := uint64(1_100_000)

		transactions := []cardanofw.BridgingRequestMetadataTransaction{
			cardanofw.BridgingRequestMetadataTransaction{
				Address: cardanofw.AddrToMetaDataAddr(user.GetAddress(cardanofw.ChainIDVector)),
				Amount:  sendAmount,
			},
		}

		metadata := map[string]interface{}{
			"1": map[string]interface{}{
				"t":  "bridga", // should be "bridge"
				"d":  cardanofw.ChainIDVector,
				"s":  cardanofw.AddrToMetaDataAddr(user.GetAddress(cardanofw.ChainIDPrime)),
				"tx": transactions,
				"fa": feeAmount,
			},
		}

		bridgingRequestMetadata, err := json.Marshal(metadata)
		require.NoError(t, err)

		beforeSendingAmountDfm, err := apex.GetBalance(ctx, user, cardanofw.ChainIDPrime)
		require.NoError(t, err)

		txHash, err := cardanofw.SendTx(
			ctx, txProviderPrime, user.PrimeWallet, sendAmount+feeAmount, apex.PrimeInfo.MultisigAddr,
			apex.Config.PrimeConfig.NetworkType, bridgingRequestMetadata)
		require.NoError(t, err)

		lowerBoundaryDfm := new(big.Int).Sub(beforeSendingAmountDfm, new(big.Int).SetUint64(sendAmount+feeAmount))

		fmt.Printf("Tx sent. hash: %s\nlowerBoundaryDfm: %d, higherBoundaryDfm: %d\n", txHash, lowerBoundaryDfm, beforeSendingAmountDfm)

		err = apex.WaitForAmountInRange(ctx, user, cardanofw.ChainIDPrime, lowerBoundaryDfm, beforeSendingAmountDfm,
			20, time.Second*30)
		require.NoError(t, err)

		newAmountDfm, err := apex.GetBalance(ctx, user, cardanofw.ChainIDPrime)
		require.NoError(t, err)

		fmt.Println("newAmountDfm", newAmountDfm)
	})

	t.Run("Submitted invalid metadata - invalid destination", func(t *testing.T) {
		sendAmount := uint64(1_000_000)
		feeAmount := uint64(1_100_000)

		transactions := []cardanofw.BridgingRequestMetadataTransaction{
			cardanofw.BridgingRequestMetadataTransaction{
				Address: cardanofw.AddrToMetaDataAddr(user.GetAddress(cardanofw.ChainIDVector)),
				Amount:  sendAmount,
			},
		}

		metadata := map[string]interface{}{
			"1": map[string]interface{}{
				"t":  "bridge",
				"d":  "", // should be destination chain address
				"s":  cardanofw.AddrToMetaDataAddr(user.GetAddress(cardanofw.ChainIDPrime)),
				"tx": transactions,
				"fa": feeAmount,
			},
		}

		bridgingRequestMetadata, err := json.Marshal(metadata)
		require.NoError(t, err)

		beforeSendingAmountDfm, err := apex.GetBalance(ctx, user, cardanofw.ChainIDPrime)
		require.NoError(t, err)

		txHash, err := cardanofw.SendTx(
			ctx, txProviderPrime, user.PrimeWallet, sendAmount+feeAmount, apex.PrimeInfo.MultisigAddr,
			apex.Config.PrimeConfig.NetworkType, bridgingRequestMetadata)
		require.NoError(t, err)

		lowerBoundaryDfm := new(big.Int).Sub(beforeSendingAmountDfm, new(big.Int).SetUint64(sendAmount+feeAmount))

		fmt.Printf("Tx sent. hash: %s\nlowerBoundaryDfm: %d, higherBoundaryDfm: %d\n", txHash, lowerBoundaryDfm, beforeSendingAmountDfm)

		err = apex.WaitForAmountInRange(ctx, user, cardanofw.ChainIDPrime, lowerBoundaryDfm, beforeSendingAmountDfm,
			20, time.Second*30)
		require.NoError(t, err)

		newAmountDfm, err := apex.GetBalance(ctx, user, cardanofw.ChainIDPrime)
		require.NoError(t, err)

		fmt.Println("newAmountDfm", newAmountDfm)
	})

	t.Run("Submitted invalid metadata - empty tx", func(t *testing.T) {
		sendAmount := uint64(1_000_000)
		feeAmount := uint64(1_100_000)

		metadata := map[string]interface{}{
			"1": map[string]interface{}{
				"t":  "bridge",
				"d":  cardanofw.ChainIDVector,
				"s":  cardanofw.AddrToMetaDataAddr(user.GetAddress(cardanofw.ChainIDPrime)),
				"tx": []cardanofw.BridgingRequestMetadataTransaction{}, // should not be empty
				"fa": feeAmount,
			},
		}

		bridgingRequestMetadata, err := json.Marshal(metadata)
		require.NoError(t, err)

		beforeSendingAmountDfm, err := apex.GetBalance(ctx, user, cardanofw.ChainIDPrime)
		require.NoError(t, err)

		txHash, err := cardanofw.SendTx(
			ctx, txProviderPrime, user.PrimeWallet, sendAmount, apex.PrimeInfo.MultisigAddr,
			apex.Config.PrimeConfig.NetworkType, bridgingRequestMetadata)
		require.NoError(t, err)

		lowerBoundaryDfm := new(big.Int).Sub(beforeSendingAmountDfm, new(big.Int).SetUint64(sendAmount+feeAmount))

		fmt.Printf("Tx sent. hash: %s\nlowerBoundaryDfm: %d, higherBoundaryDfm: %d\n", txHash, lowerBoundaryDfm, beforeSendingAmountDfm)

		err = apex.WaitForAmountInRange(ctx, user, cardanofw.ChainIDPrime, lowerBoundaryDfm, beforeSendingAmountDfm,
			20, time.Second*30)
		require.NoError(t, err)

		newAmountDfm, err := apex.GetBalance(ctx, user, cardanofw.ChainIDPrime)
		require.NoError(t, err)

		fmt.Println("newAmountDfm", newAmountDfm)
	})

	t.Run("Submitted invalid metadata - invalid sender", func(t *testing.T) {
		sendAmount := uint64(1_000_000)
		feeAmount := uint64(1_100_000)

		transactions := []cardanofw.BridgingRequestMetadataTransaction{
			cardanofw.BridgingRequestMetadataTransaction{
				Address: cardanofw.AddrToMetaDataAddr(user.GetAddress(cardanofw.ChainIDVector)),
				Amount:  sendAmount,
			},
		}

		metadata := map[string]interface{}{
			"1": map[string]interface{}{
				"t":  "brideg",     // incorrect value to trigger refund
				"s":  []string{""}, // should be sender address (max len 40)
				"d":  cardanofw.ChainIDVector,
				"tx": transactions,
				"fa": feeAmount,
			},
		}

		bridgingRequestMetadata, err := json.Marshal(metadata)
		require.NoError(t, err)

		txHash, err := cardanofw.SendTx(
			ctx, txProviderPrime, user.PrimeWallet, sendAmount+feeAmount, apex.PrimeInfo.MultisigAddr,
			apex.Config.PrimeConfig.NetworkType, bridgingRequestMetadata)
		require.NoError(t, err)

		fmt.Printf("Tx sent. hash: %s\n", txHash)

		cardanofw.WaitForInvalidState(t, ctx, apex, cardanofw.ChainIDPrime, txHash, apex.Config.APIKey, cardanofw.DefaultRequestStateTimeoutSec)
	})

	t.Run("Submitted not enough funds - invalid", func(t *testing.T) {
		sendAmount := uint64(800_000)
		feeAmount := uint64(1_100_000)

		beforeSendingAmountDfm, err := apex.GetBalance(ctx, user, cardanofw.ChainIDPrime)
		require.NoError(t, err)

		fmt.Println("beforeSendingAmountDfm", beforeSendingAmountDfm)

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

		fmt.Printf("Tx sent. hash: %s\n", txHash)

		cardanofw.WaitForInvalidState(t, ctx, apex, cardanofw.ChainIDPrime, txHash, apex.Config.APIKey, cardanofw.DefaultRequestStateTimeoutSec)
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

		fmt.Printf("Tx sent. hash: %s\nlowerBoundaryDfm: %d, higherBoundaryDfm: %d\n", txHash, lowerBoundaryDfm, beforeSendingAmountDfm)

		err = apex.WaitForAmountInRange(ctx, brSubmitterUser, cardanofw.ChainIDPrime, lowerBoundaryDfm, beforeSendingAmountDfm,
			20, time.Second*30)
		require.NoError(t, err)

		newAmountDfm, err := apex.GetBalance(ctx, brSubmitterUser, cardanofw.ChainIDPrime)
		require.NoError(t, err)

		fmt.Println("newAmountDfm", newAmountDfm)
	})
}

func TestE2E_Apex_Bridge_Refund_BatchRecreated(t *testing.T) {
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

	fmt.Printf("Tx sent. hash: %s\nlowerBoundaryDfm: %d, higherBoundaryDfm: %d\n", txHash, lowerBoundaryDfm, beforeSendingAmountDfm)

	err = apex.WaitForAmountInRange(ctx, brSubmitterUser, cardanofw.ChainIDPrime, lowerBoundaryDfm, beforeSendingAmountDfm,
		60, time.Second*30)
	require.NoError(t, err)

	newAmountDfm, err := apex.GetBalance(ctx, brSubmitterUser, cardanofw.ChainIDPrime)
	require.NoError(t, err)

	fmt.Println("newAmountDfm", newAmountDfm)
}
