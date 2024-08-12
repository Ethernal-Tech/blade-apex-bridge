package e2e

import (
	"context"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/e2e-polybft/cardanofw"
	"github.com/Ethernal-Tech/cardano-infrastructure/wallet"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"
)

func Test_E2E_Nexus(t *testing.T) {
	const (
		apiKey = "test_api_key"
	)

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	apex := cardanofw.RunApexBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithNexusEnabled(true),
	)

	t.Run("Sanity check", func(t *testing.T) {
		sendAmount := uint64(1)
		expectedAmount := ethgo.Ether(sendAmount)

		user := apex.CreateAndFundNexusUser(t, ctx, sendAmount)
		require.NotNil(t, user)

		ethBalance, err := cardanofw.GetEthAmount(ctx, apex.Nexus, user)
		require.NoError(t, err)
		require.NotZero(t, ethBalance)

		err = cardanofw.WaitForEthAmount(context.Background(), apex.Nexus, user, func(val *big.Int) bool {
			return val.Cmp(expectedAmount) == 0
		}, 10, 10)
		require.NoError(t, err)
	})

	t.Run("From Nexus to Prime", func(t *testing.T) {
		user := apex.CreateAndFundUser(t, ctx, uint64(20_000_000_000), apex.PrimeCluster.NetworkConfig(), apex.VectorCluster.NetworkConfig())

		txProviderPrime := apex.GetPrimeTxProvider()

		// create and fund wallet on nexus
		evmUser := apex.CreateAndFundNexusUser(t, ctx, 1)
		pkBytes, err := evmUser.Ecdsa.MarshallPrivateKey()
		require.NoError(t, err)

		// create cardano wallet on prime
		destinationWallet, err := wallet.GenerateWallet(false)
		require.NoError(t, err)
		walletAddress, err := cardanofw.GetAddress(apex.PrimeCluster.NetworkConfig().NetworkType, destinationWallet)
		require.NoError(t, err)

		prevAmount, err := cardanofw.GetTokenAmount(ctx, apex.GetPrimeTxProvider(), walletAddress.String())
		require.NoError(t, err)

		sendAmount := uint64(1)

		// call SendTx command
		err = apex.Nexus.SendTxEvm(string(pkBytes), walletAddress.String(), 1)
		require.NoError(t, err)

		// check expected amount cardano
		expectedAmountOnPrime := prevAmount + sendAmount // * wei?
		err = cardanofw.WaitForAmount(context.Background(), txProviderPrime, user.PrimeAddress, func(val uint64) bool {
			return val == expectedAmountOnPrime
		}, 100, time.Second*10)
		require.NoError(t, err)

		newAmountOnPrime, err := cardanofw.GetTokenAmount(ctx, txProviderPrime, user.PrimeAddress)
		require.NoError(t, err)
		require.NotZero(t, newAmountOnPrime)
	})

	t.Run("From Prime to Nexus", func(t *testing.T) {
		sendAmount := uint64(1)
		sendAmountDfm := new(big.Int).SetUint64(sendAmount)
		exp := new(big.Int).Exp(big.NewInt(10), big.NewInt(6), nil)
		sendAmountDfm.Mul(sendAmountDfm, exp)

		expectedAmount := ethgo.Ether(sendAmount)

		userPrime := apex.CreateAndFundUser(t, ctx, uint64(500_000_000), apex.PrimeCluster.NetworkConfig(), apex.VectorCluster.NetworkConfig())
		require.NotNil(t, userPrime)

		user := apex.CreateAndFundNexusUser(t, ctx, sendAmount)
		require.NotNil(t, user)

		ethBalance, err := cardanofw.GetEthAmount(ctx, apex.Nexus, user)
		fmt.Printf("ETH Amount %d\n", ethBalance)
		require.NoError(t, err)
		require.NotZero(t, ethBalance)

		err = cardanofw.WaitForEthAmount(context.Background(), apex.Nexus, user, func(val *big.Int) bool {
			return val.Cmp(expectedAmount) == 0
		}, 10, 10)
		require.NoError(t, err)

		///////////////////////////////////////
		//////////// SEND TX //////////////////
		///////////////////////////////////////

		txProviderPrime := apex.GetPrimeTxProvider()

		nexusAddress := user.Address()

		receiverAddr := user.Address().String()
		fmt.Printf("ETH receiver Addr: %s\n", receiverAddr)

		ethBalanceBefore, err := cardanofw.GetEthAmount(ctx, apex.Nexus, user)
		fmt.Printf("ETH Amount BEFORE TX %d\n", ethBalanceBefore)
		require.NoError(t, err)

		txHash := userPrime.BridgeNexusAmount(t, ctx, txProviderPrime, apex.Bridge.PrimeMultisigAddr,
			nexusAddress.String(), sendAmountDfm.Uint64(), apex.PrimeCluster.NetworkConfig(), receiverAddr)

		ethBalanceAfter, err := cardanofw.GetEthAmount(ctx, apex.Nexus, user)
		fmt.Printf("ETH Amount AFTER  TX %d\n", ethBalanceAfter)
		require.NoError(t, err)

		fmt.Printf("Tx sent. hash: %s\n", txHash)

		err = cardanofw.WaitForEthAmount(context.Background(), apex.Nexus, user, func(val *big.Int) bool {
			ethBalanceAfter, err := cardanofw.GetEthAmount(ctx, apex.Nexus, user)
			require.NoError(t, err)

			return ethBalanceBefore.Cmp(ethBalanceAfter) != 0
		}, 30, time.Second*30)
		require.NoError(t, err)

		ethBalanceAfter, err = cardanofw.GetEthAmount(ctx, apex.Nexus, user)
		fmt.Printf("ETH Amount AFTER AFTER TX %d\n", ethBalanceAfter)
		require.NoError(t, err)
	})
}
