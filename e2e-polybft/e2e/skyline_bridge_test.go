package e2e

import (
	"context"
	"fmt"
	"math/big"
	"testing"

	"github.com/0xPolygon/polygon-edge/e2e-polybft/cardanofw"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/e2ehelper"
	"github.com/stretchr/testify/require"
)

func TestE2E_SkylineBridge_ValidScenarios(t *testing.T) {
	const (
		apiKey  = "test_api_key"
		userCnt = 10
	)

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	apex := cardanofw.SetupAndRunSkylineBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithUserCnt(userCnt),
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

	t.Run("prime -> cardano - currency on src", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() {
			t.Skip()
		}

		sendAmountDfm := big.NewInt(1_500_000)

		brSubmitterUser, err := cardanofw.NewTestApexUserSkyline(
			apex.Config.PrimeConfig.NetworkType, false, 0, false, 0, false)
		require.NoError(t, err)

		txProviderPrime := apex.PrimeInfo.GetTxProvider()

		minterUser := apex.Users[userCnt-2]

		_, err = cardanofw.FundUserWithToken(
			ctx, cardanofw.ChainIDPrime, apex.Config.PrimeConfig.NetworkType, txProviderPrime,
			minterUser, brSubmitterUser, uint64(1_100_000_000), uint64(0))
		require.NoError(t, err)

		e2ehelper.ExecuteSingleBridgingSkyline(
			t, ctx, apex, brSubmitterUser, user, cardanofw.ChainIDPrime, cardanofw.ChainIDCardano, sendAmountDfm)
	})
	t.Run("prime -> cardano - native token on src", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() {
			t.Skip()
		}

		sendAmountDfm := big.NewInt(1_500_000)

		brSubmitterUser, err := cardanofw.NewTestApexUserSkyline(
			apex.Config.PrimeConfig.NetworkType, false, 0, false, 0, false)
		require.NoError(t, err)

		txProviderPrime := apex.PrimeInfo.GetTxProvider()

		minterUser := apex.Users[userCnt-2]

		_, err = cardanofw.FundUserWithToken(
			ctx, cardanofw.ChainIDPrime, apex.Config.PrimeConfig.NetworkType, txProviderPrime,
			minterUser, brSubmitterUser, uint64(1_100_000_000), uint64(2_500_000))
		require.NoError(t, err)

		e2ehelper.ExecuteSingleBridgingSkylineNativeTokens(
			t, ctx, apex, brSubmitterUser, user, cardanofw.ChainIDPrime, cardanofw.ChainIDCardano, sendAmountDfm)
	})

	t.Run("cardano -> prime - currency on src", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() {
			t.Skip()
		}

		sendAmountDfm := big.NewInt(1_500_000)

		brSubmitterUser, err := cardanofw.NewTestApexUserSkyline(
			apex.Config.PrimeConfig.NetworkType, false, 0, true, apex.Config.CardanoConfig.NetworkType, false)
		require.NoError(t, err)

		txProviderCardano := apex.CardanoInfo.GetTxProvider()

		minterUser := apex.Users[userCnt-2]

		_, err = cardanofw.FundUserWithToken(
			ctx, cardanofw.ChainIDCardano, apex.Config.CardanoConfig.NetworkType, txProviderCardano,
			minterUser, brSubmitterUser, uint64(1_100_000_000), uint64(0))
		require.NoError(t, err)

		e2ehelper.ExecuteSingleBridgingSkyline(
			t, ctx, apex, brSubmitterUser, user, cardanofw.ChainIDCardano, cardanofw.ChainIDPrime, sendAmountDfm)
	})

	t.Run("cardano -> prime - native token on src", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() {
			t.Skip()
		}

		sendAmountDfm := big.NewInt(1_500_000)

		brSubmitterUser, err := cardanofw.NewTestApexUserSkyline(
			apex.Config.PrimeConfig.NetworkType, false, 0, true, apex.Config.CardanoConfig.NetworkType, false)
		require.NoError(t, err)

		txProviderCardano := apex.CardanoInfo.GetTxProvider()

		minterUser := apex.Users[userCnt-2]

		_, err = cardanofw.FundUserWithToken(
			ctx, cardanofw.ChainIDCardano, apex.Config.CardanoConfig.NetworkType, txProviderCardano,
			minterUser, brSubmitterUser, uint64(1_100_000_000), uint64(2_500_000))
		require.NoError(t, err)

		e2ehelper.ExecuteSingleBridgingSkylineNativeTokens(
			t, ctx, apex, brSubmitterUser, user, cardanofw.ChainIDCardano, cardanofw.ChainIDPrime, sendAmountDfm)
	})
}
