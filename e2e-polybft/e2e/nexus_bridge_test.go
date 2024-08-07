package e2e

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/e2e-polybft/cardanofw"
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

	sendAmount := uint64(1)

	expectedAmount := ethgo.Ether(sendAmount).Uint64()

	userPrime := apex.CreateAndFundUser(t, ctx, uint64(500_000_000), apex.PrimeCluster.NetworkConfig(), apex.VectorCluster.NetworkConfig())
	require.NotNil(t, userPrime)

	user := apex.CreateAndFundNexusUser(t, ctx, sendAmount)
	require.NotNil(t, user)

	ethBalance, err := cardanofw.GetEthAmount(ctx, apex.Nexus, user)
	fmt.Printf("ETH Amount %d\n", ethBalance)
	require.NoError(t, err)
	require.NotZero(t, ethBalance)

	err = cardanofw.WaitForEthAmount(context.Background(), apex.Nexus, user, func(val uint64) bool {
		return val == expectedAmount
	}, 10, 10)
	require.NoError(t, err)

	///////////////////////////////////////
	//////////// SEND TX //////////////////
	///////////////////////////////////////

	txProviderPrime := apex.GetPrimeTxProvider()

	nexusAddress := user.Address()

	// receiverAddr := "0x537Fc153530Cd06f99FEf66474e5Ba1F8Cb69E03"
	receiverAddr := user.Address().String()
	fmt.Printf("ETH receiver Addr: %s\n", receiverAddr)

	ethBalanceBefore, err := cardanofw.GetEthAmount(ctx, apex.Nexus, user)
	fmt.Printf("ETH Amount BEFORE TX %d\n", ethBalanceBefore)
	require.NoError(t, err)

	txHash := userPrime.BridgeNexusAmount(t, ctx, txProviderPrime, apex.Bridge.PrimeMultisigAddr,
		nexusAddress.String(), sendAmount, apex.PrimeCluster.NetworkConfig(), receiverAddr)

	ethBalanceAfter, err := cardanofw.GetEthAmount(ctx, apex.Nexus, user)
	fmt.Printf("ETH Amount AFTER  TX %d\n", ethBalanceAfter)
	require.NoError(t, err)

	fmt.Printf("Tx sent. hash: %s\n", txHash)

	err = cardanofw.WaitForEthAmount(context.Background(), apex.Nexus, user, func(val uint64) bool {
		ethBalanceAfter, err := cardanofw.GetEthAmount(ctx, apex.Nexus, user)
		require.NoError(t, err)

		return ethBalanceBefore != ethBalanceAfter
	}, 30, time.Second*30)
	require.NoError(t, err)

	ethBalanceAfter, err = cardanofw.GetEthAmount(ctx, apex.Nexus, user)
	fmt.Printf("ETH Amount AFTER AFTER TX %d\n", ethBalanceAfter)
	require.NoError(t, err)

	///////////////////////////////////////
	//////////// END OF TEST //////////////
	///////////////////////////////////////

	signalChannel := make(chan os.Signal, 1)
	// Notify the signalChannel when the interrupt signal is received (Ctrl+C)
	signal.Notify(signalChannel, os.Interrupt, syscall.SIGTERM)

	<-signalChannel
	fmt.Println("END OF THE TEST")
}
