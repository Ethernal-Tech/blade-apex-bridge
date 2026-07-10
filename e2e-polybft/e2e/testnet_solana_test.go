package e2e

import (
	"context"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/e2e-polybft/cardanofw"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/e2ehelper"
	"github.com/stretchr/testify/require"
)

const (
	splTokenSendAmountWei = 100000000000000000  // 0.1 wSOL
	apexSendAmountWei     = 1000000000000000000 // 1 APEX
)

// returns the send amount in wei for a given token ID
// if the token ID is a SPL token, returns the splTokenSendAmountWei
// otherwise, returns the apexSendAmountWei
func sendAmountValueForToken(isSPLToken bool) *big.Int {
	if isSPLToken {
		return big.NewInt(splTokenSendAmountWei)
	}

	return big.NewInt(apexSendAmountWei)
}

func isSPLToken(tokenID uint16) bool {
	return tokenID == cardanofw.WSOLTokenID || tokenID == cardanofw.ASOLTokenID
}

func Test_E2E_SkylineSolanaSanityCheck(t *testing.T) {
	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	apex, err := cardanofw.SetupSkylineRemoteBridge(t, cardanofw.GetTestnetSkylineBridgeConfig())
	require.NoError(t, err)

	apex.Users = apex.Users[skylineTestsUserCnt:]

	user := apex.Users[0]

	bridgingRequests := []struct {
		src        string
		dest       string
		srcTokenID uint16
	}{
		{src: cardanofw.ChainIDSolana, dest: cardanofw.ChainIDVector, srcTokenID: cardanofw.WSOLTokenID},
		{src: cardanofw.ChainIDVector, dest: cardanofw.ChainIDSolana, srcTokenID: cardanofw.ASOLTokenID},
		{src: cardanofw.ChainIDVector, dest: cardanofw.ChainIDSolana, srcTokenID: cardanofw.AP3XTokenID},
		{src: cardanofw.ChainIDSolana, dest: cardanofw.ChainIDVector, srcTokenID: cardanofw.SAP3XTokenID},
	}

	for _, dir := range bridgingRequests {
		fmt.Printf("bridging from %s to %s, srcTokenID: %d\n", dir.src, dir.dest, dir.srcTokenID)

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, dir.src, dir.dest, sendAmountValueForToken(isSPLToken(dir.srcTokenID)), dir.srcTokenID, false, bridgingOpts...)
	}
}

func Test_E2E_SkylineSolanaTestnetPrintBalances(t *testing.T) {
	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	apex, err := cardanofw.SetupSkylineRemoteBridge(t, cardanofw.GetTestnetSkylineBridgeConfig())
	require.NoError(t, err)

	users := apex.Users[skylineTestsUserCnt:]

	balances, _ := cardanofw.GetUsersBalances(ctx, apex, skylineChains, users)
	printSkylineUserBalances(t, apex, users, balances)
}

func TestE2E_SkylineSolanaTestnetBridge_ValidScenarios(t *testing.T) {
	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	apex, err := cardanofw.SetupSkylineRemoteBridge(t, cardanofw.GetTestnetSkylineBridgeConfig())
	require.NoError(t, err)

	apex.Users = apex.Users[skylineTestsUserCnt:]

	user := apex.Users[0]

	const numOfInstanceForSequentialTests = 3

	// rpc cooldown
	time.Sleep(10 * time.Second)

	t.Run("Solana -> Vector - wrapped token on src", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDSolana, cardanofw.ChainIDVector, sendAmountValueForToken(true), cardanofw.WSOLTokenID, false)
	})

	// rpc cooldown
	time.Sleep(5 * time.Second)

	t.Run("Vector -> Solana - wrapped token on dest", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDVector, cardanofw.ChainIDSolana, sendAmountValueForToken(true), cardanofw.ASOLTokenID, false)
	})

	// rpc cooldown
	time.Sleep(5 * time.Second)

	t.Run("Vector -> Solana - mint token on dest", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDVector, cardanofw.ChainIDSolana, sendAmountValueForToken(false), cardanofw.AP3XTokenID, false)
	})

	// rpc cooldown
	time.Sleep(5 * time.Second)

	t.Run("Solana -> Vector - mint token on src", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDSolana, cardanofw.ChainIDVector, sendAmountValueForToken(false), cardanofw.SAP3XTokenID, false)
	})

	time.Sleep(10 * time.Second)

	t.Run("Solana -> Vector sequential wrapped token on source", func(t *testing.T) {
		e2ehelper.ExecuteBridgingWaitAfterSubmits(
			t, ctx, apex, numOfInstanceForSequentialTests, user,
			cardanofw.ChainIDSolana, cardanofw.ChainIDVector,
			sendAmountValueForToken(true), cardanofw.WSOLTokenID, bridgingOpts...)
	})

	time.Sleep(10 * time.Second)

	t.Run("Solana -> Vector sequential mint token on source", func(t *testing.T) {
		e2ehelper.ExecuteBridgingWaitAfterSubmits(
			t, ctx, apex, numOfInstanceForSequentialTests, user,
			cardanofw.ChainIDSolana, cardanofw.ChainIDVector,
			sendAmountValueForToken(false), cardanofw.SAP3XTokenID, bridgingOpts...)
	})

	// rpc cooldown
	time.Sleep(20 * time.Second)

	t.Run("return src tokens to original src chains", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDVector, cardanofw.ChainIDSolana, new(big.Int).Mul(sendAmountValueForToken(true), big.NewInt(3)), cardanofw.ASOLTokenID, false)

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDSolana, cardanofw.ChainIDVector, new(big.Int).Mul(sendAmountValueForToken(false), big.NewInt(2)), cardanofw.SAP3XTokenID, false)
	})
}

func TestE2E_SkylineSolanaTestnetBridge_InvalidScenarios(t *testing.T) {
	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	apex, err := cardanofw.SetupSkylineRemoteBridge(t, cardanofw.GetTestnetSkylineBridgeConfig())
	require.NoError(t, err)

	apex.Users = apex.Users[skylineTestsUserCnt:]

	user := apex.Users[0]

	bridgingFee := apex.GetMinBridgingFee(cardanofw.ChainIDSolana, false)

	solanaChain := apex.GetChainMust(t, cardanofw.ChainIDSolana).(*cardanofw.TestSolanaChain)

	// rpc cooldown
	time.Sleep(10 * time.Second)

	t.Run("amount is less than min bridging amount", func(t *testing.T) {
		_, err := solanaChain.BridgingRequest(cardanofw.BridgingRequestParams{
			Ctx:            ctx,
			DestChainID:    cardanofw.ChainIDVector,
			PrivateKey:     user.SolanaWallet.PrivateKey.String(),
			ChainIDsConfig: "",
			Receivers: map[string]cardanofw.ReceiverAmount{
				user.GetAddress(cardanofw.ChainIDVector): {
					TokenID: cardanofw.WSOLTokenID,
					Amount:  cardanofw.LamportToWei(big.NewInt(1)),
				},
			},
			FeeAmount:      bridgingFee,
			OperationFee:   apex.GetMinOperationFee(cardanofw.ChainIDSolana),
			IsCurrencySrc:  false,
			IsCurrencyDest: false,
		})
		require.Error(t, err)
		require.ErrorContains(t, err, "BridgingAmountTooLow")
	})

	t.Run("insufficient fee", func(t *testing.T) {
		_, err := solanaChain.BridgingRequest(cardanofw.BridgingRequestParams{
			Ctx:            ctx,
			DestChainID:    cardanofw.ChainIDVector,
			PrivateKey:     user.SolanaWallet.PrivateKey.String(),
			ChainIDsConfig: "",
			Receivers: map[string]cardanofw.ReceiverAmount{
				user.GetAddress(cardanofw.ChainIDVector): {
					TokenID: cardanofw.WSOLTokenID,
					Amount:  sendAmountValueForToken(true),
				},
			},
			FeeAmount:      new(big.Int).Sub(bridgingFee, big.NewInt(1)),
			OperationFee:   apex.GetMinOperationFee(cardanofw.ChainIDSolana),
			IsCurrencySrc:  false,
			IsCurrencyDest: false,
		})
		require.Error(t, err)
		require.ErrorContains(t, err, "InsufficientFee")
	})

	const (
		maxWaitTimeSec   = 1500
		retryIntervalSec = 5
	)

	t.Run("invalid destination chain ID", func(t *testing.T) {
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
					Amount:  sendAmountValueForToken(true),
				},
			},
			FeeAmount:      bridgingFee,
			OperationFee:   apex.GetMinOperationFee(cardanofw.ChainIDSolana),
			IsCurrencySrc:  false,
			IsCurrencyDest: false,
		})
		require.NoError(t, err)

		tokensInfo, err := apex.GetBridgingTokensInfo(cardanofw.ChainIDSolana, cardanofw.ChainIDVector, cardanofw.WSOLTokenID)
		require.NoError(t, err)

		waitForInvalidTestResultSol(t, ctx, apex, cardanofw.ChainIDSolana, tokensInfo, user, txSig, userWSolBalance, true, maxWaitTimeSec, retryIntervalSec)
		require.NoError(t, err)
	})

	//nolint:dupl
	t.Run("invalid destination address", func(t *testing.T) {
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
					Amount:  sendAmountValueForToken(true),
				},
			},
			FeeAmount:      apex.GetMinBridgingFee(cardanofw.ChainIDSolana, false),
			OperationFee:   apex.GetMinOperationFee(cardanofw.ChainIDSolana),
			IsCurrencySrc:  false,
			IsCurrencyDest: false,
		})
		require.NoError(t, err)

		tokensInfo, err := apex.GetBridgingTokensInfo(cardanofw.ChainIDSolana, cardanofw.ChainIDVector, cardanofw.WSOLTokenID)
		require.NoError(t, err)

		waitForInvalidTestResultSol(t, ctx, apex, cardanofw.ChainIDSolana, tokensInfo, user, txSig, userWSolBalance, true, maxWaitTimeSec, retryIntervalSec)
		require.NoError(t, err)
	})

	//nolint:dupl
	t.Run("chain ID not in directions", func(t *testing.T) {
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
					Amount:  sendAmountValueForToken(true),
				},
			},
			FeeAmount:      apex.GetMinBridgingFee(cardanofw.ChainIDSolana, false),
			OperationFee:   apex.GetMinOperationFee(cardanofw.ChainIDSolana),
			IsCurrencySrc:  false,
			IsCurrencyDest: false,
		})
		require.NoError(t, err)

		tokensInfo, err := apex.GetBridgingTokensInfo(cardanofw.ChainIDSolana, cardanofw.ChainIDVector, cardanofw.WSOLTokenID)
		require.NoError(t, err)

		waitForInvalidTestResultSol(t, ctx, apex, cardanofw.ChainIDSolana, tokensInfo, user, txSig, userWSolBalance, true, maxWaitTimeSec, retryIntervalSec)
		require.NoError(t, err)
	})
}
