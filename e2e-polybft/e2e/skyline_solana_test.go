package e2e

import (
	"context"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/e2e-polybft/cardanofw"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/e2ehelper"
	solanawallet "github.com/Ethernal-Tech/solana-infrastructure/wallet"
	"github.com/stretchr/testify/require"
)

// To run solana tests be sure to have necessary tools installed:
// https://solana.com/docs/intro/installation

func Test_SkylineSolana_AllDirections(t *testing.T) {
	const (
		apiKey = "test_api_key"
	)

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	primeConfig, cardanoConfig := cardanofw.NewPrimeChainConfig(), cardanofw.NewCardanoChainConfig(true)
	vectorConfig := cardanofw.NewVectorChainConfig(map[uint16]string{
		cardanofw.ASOLTokenID:  cardanofw.ASOLTokenName,
		cardanofw.USDTTokenID:  cardanofw.USDTTokenName,
		cardanofw.SAP3XTokenID: cardanofw.SAP3XTokenName,
	})
	nexusConfig := cardanofw.NewNexusChainConfig(true)

	solanaConfig := cardanofw.NewSolanaChainConfig(true)

	apex := cardanofw.SetupAndRunSkylineBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithCardanoConfig(cardanoConfig),
		cardanofw.WithPrimeConfig(primeConfig),
		cardanofw.WithVectorConfig(vectorConfig),
		cardanofw.WithSolanaConfig(solanaConfig),
		cardanofw.WithNexusConfig(nexusConfig),
		cardanofw.WithUserCnt(1),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	fmt.Println("solana user addr: ", apex.Users[0].SolanaAddress)
	balance, err := apex.GetBalance(ctx, apex.Users[0], cardanofw.ChainIDSolana)
	require.NoError(t, err)
	fmt.Println("solana user SOL balance: ", balance)
	time.Sleep(1 * time.Second)

	balance, err = apex.GetBalanceWithTokenName(ctx, apex.Users[0], cardanofw.ChainIDSolana, cardanofw.WSOLMintAddress)
	require.NoError(t, err)
	fmt.Println("solana user wSOL balance: ", balance)

	t.Run("SOL -> Vector", func(t *testing.T) {
		relayerUser := &cardanofw.TestApexUser{
			HasSolanaWallet: true,
			SolanaAddress:   apex.SolanaInfo.RelayerAddress,
		}
		relayerBalance, err := apex.GetBalance(ctx, relayerUser, cardanofw.ChainIDSolana)
		require.NoError(t, err)

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, apex.Users[0], apex.Users[0], cardanofw.ChainIDSolana, cardanofw.ChainIDVector, cardanofw.SolanaToWei(big.NewInt(1)),
			cardanofw.WSOLTokenID, true)

		relayerBalanceAfter, err := apex.GetBalance(ctx, relayerUser, cardanofw.ChainIDSolana)
		require.NoError(t, err)

		diff := new(big.Int).Sub(relayerBalanceAfter["lovelace"], relayerBalance["lovelace"])
		require.True(t, diff.Cmp(cardanofw.LamportToWei(solanaConfig.MinBridgingFee)) == 0)
	})

	t.Run("Vector -> SOL", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, apex.Users[0], apex.Users[0], cardanofw.ChainIDVector, cardanofw.ChainIDSolana, cardanofw.SolanaToWei(big.NewInt(1)),
			cardanofw.ASOLTokenID, true)
	})

	t.Run("SOL -> Nexus", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, apex.Users[0], apex.Users[0], cardanofw.ChainIDSolana, cardanofw.ChainIDNexus, cardanofw.SolanaToWei(big.NewInt(1)),
			cardanofw.WSOLTokenID, true)
	})

	t.Run("Nexus -> SOL", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, apex.Users[0], apex.Users[0], cardanofw.ChainIDNexus, cardanofw.ChainIDSolana, cardanofw.SolanaToWei(big.NewInt(1)),
			cardanofw.ASOLTokenID, true)
	})

	t.Run("Vector AP3X -> Solana sAP3X", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, apex.Users[0], apex.Users[0], cardanofw.ChainIDVector, cardanofw.ChainIDSolana, cardanofw.ApexToWei(big.NewInt(1)),
			cardanofw.AP3XTokenID, true)
	})

	t.Run("Solana sAP3X -> Vector AP3X", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, apex.Users[0], apex.Users[0], cardanofw.ChainIDSolana, cardanofw.ChainIDVector, cardanofw.ApexToWei(big.NewInt(1)),
			cardanofw.SAP3XTokenID, true)
	})

	t.Run("Nexus AP3X -> Solana sAP3X", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, apex.Users[0], apex.Users[0], cardanofw.ChainIDNexus, cardanofw.ChainIDSolana, cardanofw.ApexToWei(big.NewInt(1)),
			cardanofw.AP3XTokenID, true)
	})

	t.Run("Solana sAP3X -> Nexus AP3X", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, apex.Users[0], apex.Users[0], cardanofw.ChainIDSolana, cardanofw.ChainIDNexus, cardanofw.ApexToWei(big.NewInt(1)),
			cardanofw.SAP3XTokenID, true)
	})

	// mint VS and NS tokens to the user
	_, err = cardanofw.FundUserWithToken(ctx, apex, cardanofw.ChainIDVector,
		apex.VectorInfo.GenesisWallet, apex.Users[0], cardanofw.VSTokenName,
		cardanofw.ApexToWei(big.NewInt(400_000_000)), cardanofw.ApexToWei(big.NewInt(1)), cardanofw.ApexToWei(big.NewInt(400_000_000)))
	require.NoError(t, err)

	nexusChain := apex.GetChainMust(t, cardanofw.ChainIDNexus).(*cardanofw.TestEVMChain)
	err = nexusChain.FundUsersWithToken(apex.Users[0].GetAddress(cardanofw.ChainIDNexus), cardanofw.DfmToWei(big.NewInt(400_000_000)), cardanofw.NSTokenID)
	require.NoError(t, err)

	userBalance, err := apex.GetBalanceWithTokenName(ctx, apex.Users[0], cardanofw.ChainIDVector, apex.VectorInfo.Tokens[cardanofw.VSTokenID].ChainSpecific)
	require.NoError(t, err)
	fmt.Println("vector user VS balance: ", userBalance)
	userBalance, err = apex.GetBalanceWithTokenName(ctx, apex.Users[0], cardanofw.ChainIDNexus, apex.NexusInfo.Tokens[cardanofw.NSTokenID].ChainSpecific)
	require.NoError(t, err)
	fmt.Println("nexus user NS balance: ", userBalance)

	t.Run("Vector VS -> Solana VS", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, apex.Users[0], apex.Users[0], cardanofw.ChainIDVector, cardanofw.ChainIDSolana, cardanofw.ApexToWei(big.NewInt(1)),
			cardanofw.VSTokenID, true)
	})

	t.Run("Solana VS -> Vector VS", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, apex.Users[0], apex.Users[0], cardanofw.ChainIDSolana, cardanofw.ChainIDVector, cardanofw.ApexToWei(big.NewInt(1)),
			cardanofw.VSTokenID, true)
	})

	t.Run("Nexus NS -> Solana NS", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, apex.Users[0], apex.Users[0], cardanofw.ChainIDNexus, cardanofw.ChainIDSolana, cardanofw.ApexToWei(big.NewInt(1)),
			cardanofw.NSTokenID, true)
	})

	t.Run("Solana NS -> Nexus NS", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, apex.Users[0], apex.Users[0], cardanofw.ChainIDSolana, cardanofw.ChainIDNexus, cardanofw.ApexToWei(big.NewInt(1)),
			cardanofw.NSTokenID, true)
	})
}

func Test_SkylineSolana_InvalidScenarios(t *testing.T) {
	const (
		apiKey           = "test_api_key"
		maxWaitTimeSec   = 600
		retryIntervalSec = 10
	)

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	primeConfig, cardanoConfig := cardanofw.NewPrimeChainConfig(), cardanofw.NewCardanoChainConfig(true)
	vectorConfig := cardanofw.NewVectorChainConfig(map[uint16]string{
		cardanofw.ASOLTokenID: cardanofw.ASOLTokenName,
		cardanofw.USDTTokenID: cardanofw.USDTTokenName,
	})
	nexusConfig := cardanofw.NewNexusChainConfig(true)

	solanaConfig := cardanofw.NewSolanaChainConfig(true)

	apex := cardanofw.SetupAndRunSkylineBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithCardanoConfig(cardanoConfig),
		cardanofw.WithPrimeConfig(primeConfig),
		cardanofw.WithVectorConfig(vectorConfig),
		cardanofw.WithSolanaConfig(solanaConfig),
		cardanofw.WithNexusConfig(nexusConfig),
		cardanofw.WithUserCnt(1),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	user := apex.Users[0]

	solanaChain := apex.GetChainMust(t, cardanofw.ChainIDSolana).(*cardanofw.TestSolanaChain)

	wallet, err := solanawallet.NewWallet()
	require.NoError(t, err)

	sendAmount := cardanofw.LamportToWei(solanaConfig.MinBridgingAmount)

	nowSOLUser := &cardanofw.TestApexUser{
		HasSolanaWallet: true,
		SolanaAddress:   wallet.PublicKey.String(),
		SolanaWallet:    wallet,
	}

	_, err = solanaChain.SendTx(ctx, user.SolanaWallet.PrivateKey.String(), []byte{}, []cardanofw.GenericTxReceiver{
		{
			Addr:         nowSOLUser.SolanaAddress,
			Amount:       cardanofw.SolanaToWei(big.NewInt(5)),
			NativeTokens: nil,
		},
	}, 0)
	require.NoError(t, err)

	t.Run("1. user has no wSOL", func(t *testing.T) {
		_, err := solanaChain.BridgingRequest(cardanofw.BridgingRequestParams{
			Ctx:            ctx,
			DestChainID:    cardanofw.ChainIDVector,
			PrivateKey:     nowSOLUser.SolanaWallet.PrivateKey.String(),
			ChainIDsConfig: "",
			Receivers: map[string]cardanofw.ReceiverAmount{
				user.GetAddress(cardanofw.ChainIDVector): {
					TokenID: cardanofw.WSOLTokenID,
					Amount:  sendAmount,
				},
			},
			FeeAmount:      solanaConfig.MinBridgingFee,
			OperationFee:   solanaConfig.MinOperationFee,
			IsCurrencySrc:  false,
			IsCurrencyDest: false,
		})
		require.Error(t, err)
		require.ErrorContains(t, err, "AccountNotInitialized")
	})

	t.Run("2. amount is less than min bridging amount", func(t *testing.T) {
		_, err := solanaChain.BridgingRequest(cardanofw.BridgingRequestParams{
			Ctx:            ctx,
			DestChainID:    cardanofw.ChainIDVector,
			PrivateKey:     user.SolanaWallet.PrivateKey.String(),
			ChainIDsConfig: "",
			Receivers: map[string]cardanofw.ReceiverAmount{
				user.GetAddress(cardanofw.ChainIDVector): {
					TokenID: cardanofw.WSOLTokenID,
					Amount:  cardanofw.LamportToWei(new(big.Int).Sub(solanaConfig.MinBridgingAmount, big.NewInt(1))),
				},
			},
			FeeAmount:      solanaConfig.MinBridgingFee,
			OperationFee:   solanaConfig.MinOperationFee,
			IsCurrencySrc:  false,
			IsCurrencyDest: false,
		})
		require.Error(t, err)
		require.ErrorContains(t, err, "BridgingAmountTooLow")
	})

	t.Run("3. insufficient fee", func(t *testing.T) {
		_, err := solanaChain.BridgingRequest(cardanofw.BridgingRequestParams{
			Ctx:            ctx,
			DestChainID:    cardanofw.ChainIDVector,
			PrivateKey:     user.SolanaWallet.PrivateKey.String(),
			ChainIDsConfig: "",
			Receivers: map[string]cardanofw.ReceiverAmount{
				user.GetAddress(cardanofw.ChainIDVector): {
					TokenID: cardanofw.WSOLTokenID,
					Amount:  cardanofw.SolanaToWei(big.NewInt(1)),
				},
			},
			FeeAmount:      new(big.Int).Sub(solanaConfig.MinBridgingFee, big.NewInt(1)),
			OperationFee:   solanaConfig.MinOperationFee,
			IsCurrencySrc:  false,
			IsCurrencyDest: false,
		})
		require.Error(t, err)
		require.ErrorContains(t, err, "InsufficientFee")
	})

	t.Run("4. insufficient operation fee", func(t *testing.T) {
		_, err := solanaChain.BridgingRequest(cardanofw.BridgingRequestParams{
			Ctx:            ctx,
			DestChainID:    cardanofw.ChainIDVector,
			PrivateKey:     user.SolanaWallet.PrivateKey.String(),
			ChainIDsConfig: "",
			Receivers: map[string]cardanofw.ReceiverAmount{
				user.GetAddress(cardanofw.ChainIDVector): {
					TokenID: cardanofw.WSOLTokenID,
					Amount:  sendAmount,
				},
			},
			FeeAmount:      solanaConfig.MinBridgingFee,
			OperationFee:   new(big.Int).Sub(solanaConfig.MinOperationFee, big.NewInt(1)),
			IsCurrencySrc:  false,
			IsCurrencyDest: false,
		})
		require.Error(t, err)
		require.ErrorContains(t, err, "InsufficientFee")
	})

	t.Run("5. refund - invalid destination chain ID", func(t *testing.T) {
		userWSolBalance, err := apex.GetBalanceWithTokenName(ctx, user, cardanofw.ChainIDSolana, cardanofw.WSOLMintAddress)
		require.NoError(t, err)

		txSig, err := solanaChain.BridgingRequest(cardanofw.BridgingRequestParams{
			Ctx:            ctx,
			DestChainID:    "invalid",
			PrivateKey:     user.SolanaWallet.PrivateKey.String(),
			ChainIDsConfig: "",
			Receivers: map[string]cardanofw.ReceiverAmount{
				user.GetAddress(cardanofw.ChainIDVector): {
					TokenID: cardanofw.WSOLTokenID,
					Amount:  sendAmount,
				},
			},
			FeeAmount:      solanaConfig.MinBridgingFee,
			OperationFee:   solanaConfig.MinOperationFee,
			IsCurrencySrc:  false,
			IsCurrencyDest: false,
		})
		require.NoError(t, err)

		tokensInfo, err := apex.GetBridgingTokensInfo(cardanofw.ChainIDSolana, cardanofw.ChainIDVector, cardanofw.WSOLTokenID)
		require.NoError(t, err)

		waitForInvalidTestResultSol(t, ctx, apex, cardanofw.ChainIDSolana, tokensInfo, user, txSig, userWSolBalance, sendAmount, true, maxWaitTimeSec, retryIntervalSec)
		require.NoError(t, err)
	})

	//nolint:dupl
	t.Run("6. refund - invalid destination address", func(t *testing.T) {
		userWSolBalance, err := apex.GetBalanceWithTokenName(ctx, user, cardanofw.ChainIDSolana, cardanofw.WSOLMintAddress)
		require.NoError(t, err)

		txSig, err := solanaChain.BridgingRequest(cardanofw.BridgingRequestParams{
			Ctx:            ctx,
			DestChainID:    cardanofw.ChainIDVector,
			PrivateKey:     user.SolanaWallet.PrivateKey.String(),
			ChainIDsConfig: "",
			Receivers: map[string]cardanofw.ReceiverAmount{
				user.GetAddress(cardanofw.ChainIDNexus): {
					TokenID: cardanofw.WSOLTokenID,
					Amount:  sendAmount,
				},
			},
			FeeAmount:      solanaConfig.MinBridgingFee,
			OperationFee:   solanaConfig.MinOperationFee,
			IsCurrencySrc:  false,
			IsCurrencyDest: false,
		})
		require.NoError(t, err)

		tokensInfo, err := apex.GetBridgingTokensInfo(cardanofw.ChainIDSolana, cardanofw.ChainIDVector, cardanofw.WSOLTokenID)
		require.NoError(t, err)

		waitForInvalidTestResultSol(t, ctx, apex, cardanofw.ChainIDSolana, tokensInfo, user, txSig, userWSolBalance, sendAmount, true, maxWaitTimeSec, retryIntervalSec)
		require.NoError(t, err)
	})

	//nolint:dupl
	t.Run("7. refund - chain ID not in directions", func(t *testing.T) {
		userWSolBalance, err := apex.GetBalanceWithTokenName(ctx, user, cardanofw.ChainIDSolana, cardanofw.WSOLMintAddress)
		require.NoError(t, err)

		txSig, err := solanaChain.BridgingRequest(cardanofw.BridgingRequestParams{
			Ctx:            ctx,
			DestChainID:    cardanofw.ChainIDCardano,
			PrivateKey:     user.SolanaWallet.PrivateKey.String(),
			ChainIDsConfig: "",
			Receivers: map[string]cardanofw.ReceiverAmount{
				user.GetAddress(cardanofw.ChainIDCardano): {
					TokenID: cardanofw.WSOLTokenID,
					Amount:  sendAmount,
				},
			},
			FeeAmount:      solanaConfig.MinBridgingFee,
			OperationFee:   solanaConfig.MinOperationFee,
			IsCurrencySrc:  false,
			IsCurrencyDest: false,
		})
		require.NoError(t, err)

		tokensInfo, err := apex.GetBridgingTokensInfo(cardanofw.ChainIDSolana, cardanofw.ChainIDVector, cardanofw.WSOLTokenID)
		require.NoError(t, err)

		waitForInvalidTestResultSol(t, ctx, apex, cardanofw.ChainIDSolana, tokensInfo, user, txSig, userWSolBalance, sendAmount, true, maxWaitTimeSec, retryIntervalSec)
		require.NoError(t, err)
	})

	t.Run("8. refund - wrong token for chain ID", func(t *testing.T) {
		nexusChain := apex.GetChainMust(t, cardanofw.ChainIDNexus).(*cardanofw.TestEVMChain)
		err = nexusChain.FundUsersWithToken(user.GetAddress(cardanofw.ChainIDNexus), cardanofw.DfmToWei(big.NewInt(400_000_000)), cardanofw.NSTokenID)
		require.NoError(t, err)

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDNexus, cardanofw.ChainIDSolana, cardanofw.ApexToWei(big.NewInt(1)),
			cardanofw.NSTokenID, true)

		nstokenMint, ok := apex.SolanaInfo.Tokens[cardanofw.NSTokenID]
		require.True(t, ok)

		userNSBalanceBefore, err := apex.GetBalanceWithTokenName(ctx, user, cardanofw.ChainIDSolana, nstokenMint.ChainSpecific)
		require.NoError(t, err)
		fmt.Println("user NS balance before: ", userNSBalanceBefore)

		txSig, err := solanaChain.BridgingRequest(cardanofw.BridgingRequestParams{
			Ctx:            ctx,
			DestChainID:    cardanofw.ChainIDVector,
			PrivateKey:     user.SolanaWallet.PrivateKey.String(),
			ChainIDsConfig: "",
			Receivers: map[string]cardanofw.ReceiverAmount{
				user.GetAddress(cardanofw.ChainIDVector): {
					TokenID: cardanofw.NSTokenID,
					Amount:  sendAmount,
				},
			},
			FeeAmount:      solanaConfig.MinBridgingFee,
			OperationFee:   solanaConfig.MinOperationFee,
			IsCurrencySrc:  false,
			IsCurrencyDest: false,
		})
		require.NoError(t, err)

		userNSBalanceAfter, err := apex.GetBalanceWithTokenName(ctx, user, cardanofw.ChainIDSolana, nstokenMint.ChainSpecific)
		require.NoError(t, err)
		fmt.Println("user NS balance after: ", userNSBalanceAfter)

		tokensInfo, err := apex.GetBridgingTokensInfo(cardanofw.ChainIDSolana, cardanofw.ChainIDNexus, cardanofw.NSTokenID)
		require.NoError(t, err)

		waitForInvalidTestResultSol(t, ctx, apex, cardanofw.ChainIDSolana, tokensInfo, user, txSig, userNSBalanceBefore, sendAmount, true, maxWaitTimeSec, retryIntervalSec)
		require.NoError(t, err)
	})
}

func waitForInvalidTestResultSol(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, srcChainID cardanofw.ChainID,
	tokensInfo *cardanofw.BridgingTokensInfo, user *cardanofw.TestApexUser,
	txHash string, beforeSendingAmount map[string]*big.Int, sentAmount *big.Int,
	refundEnabled bool, maxWaitTimeSec, retryIntervalSec uint,
) {
	t.Helper()

	retryIntervalSec = max(retryIntervalSec, 1)
	numRetries := max(1, int(maxWaitTimeSec/retryIntervalSec))

	if refundEnabled {
		lowerBoundary := new(big.Int).Sub(
			beforeSendingAmount[tokensInfo.SrcTokenName], sentAmount)

		fmt.Printf("Tx sent. hash: %s, lowerBoundary: %+v, higherBoundary: %+v\n", txHash, lowerBoundary,
			beforeSendingAmount)

		err := apex.WaitForAmountInRange(ctx, user, srcChainID, lowerBoundary,
			beforeSendingAmount[tokensInfo.SrcTokenName], numRetries,
			time.Second*time.Duration(retryIntervalSec), tokensInfo.SrcTokenName)
		require.NoError(t, err)
	} else {
		cardanofw.WaitForInvalidState(t, ctx, apex, srcChainID, txHash, apex.Config.APIKey, maxWaitTimeSec)
	}
}
