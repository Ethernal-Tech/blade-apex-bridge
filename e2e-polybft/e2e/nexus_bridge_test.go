package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"math/rand"
	"os"
	"sync"
	"testing"
	"time"

	eth_wallet "github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/cardanofw"
	"github.com/Ethernal-Tech/cardano-infrastructure/wallet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"
)

func TestE2E_ApexBridge_Nexus(t *testing.T) {
	const (
		apiKey = "test_api_key"
	)

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	apex := cardanofw.RunApexBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithVectorEnabled(false),
		cardanofw.WithNexusEnabled(true),
	)

	t.Run("Sanity check", func(t *testing.T) {
		sendAmount := uint64(1)
		expectedAmount := ethgo.Ether(sendAmount)

		user, err := apex.CreateAndFundNexusUser(ctx, sendAmount)
		require.NoError(t, err)

		ethBalance, err := cardanofw.GetEthAmount(ctx, apex.Nexus, user)
		require.NoError(t, err)
		require.NotZero(t, ethBalance)

		err = cardanofw.WaitForEthAmount(context.Background(), apex.Nexus, user, func(val *big.Int) bool {
			return val.Cmp(expectedAmount) == 0
		}, 10, 10)
		require.NoError(t, err)
	})

	t.Run("From Nexus to Prime", func(t *testing.T) {
		user := apex.CreateAndFundUser(t, ctx, uint64(5_000_000))

		txProviderPrime := apex.GetPrimeTxProvider()

		// create and fund wallet on nexus
		evmUser, err := apex.CreateAndFundNexusUser(ctx, 10)
		require.NoError(t, err)
		pkBytes, err := evmUser.Ecdsa.MarshallPrivateKey()
		require.NoError(t, err)

		prevAmount, err := cardanofw.GetTokenAmount(ctx, apex.GetPrimeTxProvider(), user.PrimeAddress)
		require.NoError(t, err)

		sendAmountEth := uint64(1)
		sendAmountDfm := new(big.Int).SetUint64(sendAmountEth)
		expDfm := new(big.Int).Exp(big.NewInt(10), big.NewInt(6), nil)
		sendAmountDfm.Mul(sendAmountDfm, expDfm)

		sendAmountWei := ethgo.Ether(sendAmountEth)

		// call SendTx command
		err = apex.Nexus.SendTxEvm(string(pkBytes), user.PrimeAddress, sendAmountWei)
		require.NoError(t, err)

		// check expected amount cardano
		expectedAmountOnPrime := prevAmount + sendAmountDfm.Uint64()
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

		userPrime := apex.CreateAndFundUser(t, ctx, uint64(500_000_000))
		require.NotNil(t, userPrime)

		user, err := apex.CreateAndFundNexusUser(ctx, sendAmount)
		require.NoError(t, err)

		ethBalance, err := cardanofw.GetEthAmount(ctx, apex.Nexus, user)
		fmt.Printf("ETH Amount %d\n", ethBalance)
		require.NoError(t, err)
		require.NotZero(t, ethBalance)

		err = cardanofw.WaitForEthAmount(context.Background(), apex.Nexus, user, func(val *big.Int) bool {
			return val.Cmp(expectedAmount) == 0
		}, 10, 10)
		require.NoError(t, err)

		txProviderPrime := apex.GetPrimeTxProvider()

		nexusAddress := user.Address()

		receiverAddr := user.Address().String()
		fmt.Printf("ETH receiver Addr: %s\n", receiverAddr)

		ethBalanceBefore, err := cardanofw.GetEthAmount(ctx, apex.Nexus, user)
		fmt.Printf("ETH Amount BEFORE TX %d\n", ethBalanceBefore)
		require.NoError(t, err)

		txHash, err := userPrime.BridgeNexusAmount(t, ctx, txProviderPrime, apex.Bridge.PrimeMultisigAddr,
			nexusAddress.String(), sendAmountDfm.Uint64(), apex.PrimeCluster.NetworkConfig(), receiverAddr)
		require.NoError(t, err)

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

func TestE2E_ApexBridgeWithNexus_ValidScenarios(t *testing.T) {
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

	userPrime := apex.CreateAndFundUser(t, ctx, uint64(500_000_000))
	require.NotNil(t, userPrime)

	txProviderPrime := apex.GetPrimeTxProvider()

	startAmountNexus := uint64(1)
	expectedAmountNexus := ethgo.Ether(startAmountNexus)

	userNexus, err := apex.CreateAndFundNexusUser(ctx, startAmountNexus)
	require.NoError(t, err)

	err = cardanofw.WaitForEthAmount(context.Background(), apex.Nexus, userNexus, func(val *big.Int) bool {
		return val.Cmp(expectedAmountNexus) == 0
	}, 10, 10)
	require.NoError(t, err)

	fmt.Println("Nexus user created and funded")

	receiverAddrNexus := userNexus.Address().String()
	fmt.Printf("Nexus receiver Addr: %s\n", receiverAddrNexus)

	t.Run("From Prime to Nexus", func(t *testing.T) {
		sendAmount := uint64(1)
		sendAmountDfm := new(big.Int).SetUint64(sendAmount)
		exp := new(big.Int).Exp(big.NewInt(10), big.NewInt(6), nil)
		sendAmountDfm.Mul(sendAmountDfm, exp)

		ethBalanceBefore, err := cardanofw.GetEthAmount(ctx, apex.Nexus, userNexus)
		fmt.Printf("ETH Amount before Tx %d\n", ethBalanceBefore)
		require.NoError(t, err)

		txHash, err := userPrime.BridgeNexusAmount(t, ctx, txProviderPrime, apex.Bridge.PrimeMultisigAddr,
			receiverAddrNexus, sendAmountDfm.Uint64(), apex.PrimeCluster.NetworkConfig(), receiverAddrNexus)
		require.NoError(t, err)

		fmt.Printf("Tx sent. hash: %s\n", txHash)

		err = cardanofw.WaitForEthAmount(context.Background(), apex.Nexus, userNexus, func(val *big.Int) bool {
			return val.Cmp(ethBalanceBefore) != 0
		}, 30, 30*time.Second)
		require.NoError(t, err)

		ethBalanceAfter, err := cardanofw.GetEthAmount(ctx, apex.Nexus, userNexus)
		require.NoError(t, err)
		fmt.Printf("ETH Amount after Tx %d\n", ethBalanceAfter)
	})

	t.Run("From Prime to Nexus one by one - wait for other side", func(t *testing.T) {
		sendAmount := uint64(1)
		sendAmountDfm := new(big.Int).SetUint64(sendAmount)
		exp := new(big.Int).Exp(big.NewInt(10), big.NewInt(6), nil)
		sendAmountDfm.Mul(sendAmountDfm, exp)

		expectedAmount := ethgo.Ether(sendAmount)

		instances := 5

		for i := 0; i < instances; i++ {
			ethBalanceBefore, err := cardanofw.GetEthAmount(ctx, apex.Nexus, userNexus)
			fmt.Printf("ETH Amount before Tx %d\n", ethBalanceBefore)
			require.NoError(t, err)

			txHash, err := userPrime.BridgeNexusAmount(t, ctx, txProviderPrime, apex.Bridge.PrimeMultisigAddr,
				receiverAddrNexus, sendAmountDfm.Uint64(), apex.PrimeCluster.NetworkConfig(), receiverAddrNexus)
			require.NoError(t, err)

			fmt.Printf("Tx sent. hash: %s\n", txHash)

			ethExpectedBalance := big.NewInt(0).Add(ethBalanceBefore, expectedAmount)

			err = cardanofw.WaitForEthAmount(context.Background(), apex.Nexus, userNexus, func(val *big.Int) bool {
				return val.Cmp(ethExpectedBalance) == 0
			}, 30, 30*time.Second)
			require.NoError(t, err)
		}

		ethBalanceAfter, err := cardanofw.GetEthAmount(ctx, apex.Nexus, userNexus)
		fmt.Printf("ETH Amount after Tx %d\n", ethBalanceAfter)
		require.NoError(t, err)
	})

	t.Run("From Prime to Nexus one by one - don't wait for other side", func(t *testing.T) {
		sendAmount := uint64(1)
		sendAmountDfm := new(big.Int).SetUint64(sendAmount)
		exp := new(big.Int).Exp(big.NewInt(10), big.NewInt(6), nil)
		sendAmountDfm.Mul(sendAmountDfm, exp)

		expectedAmount := ethgo.Ether(sendAmount)

		ethBalanceBefore, err := cardanofw.GetEthAmount(ctx, apex.Nexus, userNexus)
		fmt.Printf("ETH Amount before Tx %d\n", ethBalanceBefore)
		require.NoError(t, err)

		instances := 5

		for i := 0; i < instances; i++ {
			txHash, err := userPrime.BridgeNexusAmount(t, ctx, txProviderPrime, apex.Bridge.PrimeMultisigAddr,
				receiverAddrNexus, sendAmountDfm.Uint64(), apex.PrimeCluster.NetworkConfig(), receiverAddrNexus)
			require.NoError(t, err)

			fmt.Printf("Tx sent. hash: %s\n", txHash)
		}

		transferedAmount := new(big.Int).SetInt64(int64(instances))
		transferedAmount.Mul(transferedAmount, expectedAmount)
		ethExpectedBalance := big.NewInt(0).Add(ethBalanceBefore, transferedAmount)

		err = cardanofw.WaitForEthAmount(context.Background(), apex.Nexus, userNexus, func(val *big.Int) bool {
			return val.Cmp(ethExpectedBalance) == 0
		}, 30, 30*time.Second)
		require.NoError(t, err)

		ethBalanceAfter, err := cardanofw.GetEthAmount(ctx, apex.Nexus, userNexus)
		fmt.Printf("ETH Amount after Tx %d\n", ethBalanceAfter)
		require.NoError(t, err)
	})

	t.Run("From Prime to Nexus parallel", func(t *testing.T) {
		sendAmount := uint64(1)
		sendAmountDfm := new(big.Int).SetUint64(sendAmount)
		exp := new(big.Int).Exp(big.NewInt(10), big.NewInt(6), nil)
		sendAmountDfm.Mul(sendAmountDfm, exp)

		expectedAmount := ethgo.Ether(sendAmount)

		ethBalanceBefore, err := cardanofw.GetEthAmount(ctx, apex.Nexus, userNexus)
		fmt.Printf("ETH Amount before Tx %d\n", ethBalanceBefore)
		require.NoError(t, err)

		// Fund wallets
		instances := 5
		primeUsers := make([]*cardanofw.TestApexUser, instances)

		for i := 0; i < instances; i++ {
			primeUsers[i] = apex.CreateAndFundUser(t, ctx, uint64(10_000_000))
			require.NotNil(t, primeUsers[i])
		}

		var wg sync.WaitGroup
		for i := 0; i < instances; i++ {
			wg.Add(1)

			go func(idx int) {
				defer wg.Done()

				txHash, err := primeUsers[idx].BridgeNexusAmount(t, ctx, txProviderPrime, apex.Bridge.PrimeMultisigAddr,
					receiverAddrNexus, sendAmountDfm.Uint64(), apex.PrimeCluster.NetworkConfig(), receiverAddrNexus)
				require.NoError(t, err)

				fmt.Printf("Tx %v sent. hash: %s\n", idx+1, txHash)
			}(i)
		}

		wg.Wait()

		transferedAmount := new(big.Int).SetInt64(int64(instances))
		transferedAmount.Mul(transferedAmount, expectedAmount)
		ethExpectedBalance := big.NewInt(0).Add(ethBalanceBefore, transferedAmount)

		err = cardanofw.WaitForEthAmount(context.Background(), apex.Nexus, userNexus, func(val *big.Int) bool {
			return val.Cmp(ethExpectedBalance) == 0
		}, 30, 30*time.Second)
		require.NoError(t, err)

		ethBalanceAfter, err := cardanofw.GetEthAmount(ctx, apex.Nexus, userNexus)
		fmt.Printf("ETH Amount after Tx %d\n", ethBalanceAfter)
		require.NoError(t, err)
	})

	t.Run("From Prime to Nexus sequential and parallel", func(t *testing.T) {
		sendAmount := uint64(1)
		sendAmountDfm := new(big.Int).SetUint64(sendAmount)
		exp := new(big.Int).Exp(big.NewInt(10), big.NewInt(6), nil)
		sendAmountDfm.Mul(sendAmountDfm, exp)

		expectedAmount := ethgo.Ether(sendAmount)

		ethBalanceBefore, err := cardanofw.GetEthAmount(ctx, apex.Nexus, userNexus)
		fmt.Printf("ETH Amount before Tx %d\n", ethBalanceBefore)
		require.NoError(t, err)

		sequentialInstances := 5
		parallelInstances := 10

		for sequenceIdx := 0; sequenceIdx < sequentialInstances; sequenceIdx++ {
			primeUsers := make([]*cardanofw.TestApexUser, parallelInstances)

			for i := 0; i < parallelInstances; i++ {
				primeUsers[i] = apex.CreateAndFundUser(t, ctx, uint64(5_000_000))
				require.NotNil(t, primeUsers[i])
			}

			var wg sync.WaitGroup
			for i := 0; i < parallelInstances; i++ {
				wg.Add(1)

				go func(sequence, idx int) {
					defer wg.Done()

					txHash, err := primeUsers[idx].BridgeNexusAmount(t, ctx, txProviderPrime, apex.Bridge.PrimeMultisigAddr,
						receiverAddrNexus, sendAmountDfm.Uint64(), apex.PrimeCluster.NetworkConfig(), receiverAddrNexus)
					require.NoError(t, err)

					fmt.Printf("Seq: %v, Tx %v sent. hash: %s\n", sequence+1, idx+1, txHash)
				}(sequenceIdx, i)
			}

			wg.Wait()
		}

		transferedAmount := new(big.Int).SetInt64(int64(parallelInstances * sequentialInstances))
		transferedAmount.Mul(transferedAmount, expectedAmount)
		ethExpectedBalance := big.NewInt(0).Add(ethBalanceBefore, transferedAmount)
		fmt.Printf("Expected ETH Amount after Txs %d\n", ethExpectedBalance)

		err = cardanofw.WaitForEthAmount(context.Background(), apex.Nexus, userNexus, func(val *big.Int) bool {
			return val.Cmp(ethExpectedBalance) == 0
		}, 60, 30*time.Second)
		require.NoError(t, err)

		ethBalanceAfter, err := cardanofw.GetEthAmount(ctx, apex.Nexus, userNexus)
		fmt.Printf("ETH Amount after Tx %d\n", ethBalanceAfter)
		require.NoError(t, err)
	})

	t.Run("From Prime to Nexus sequential and parallel with max receivers", func(t *testing.T) {
		const (
			sequentialInstances = 5
			parallelInstances   = 10
			receivers           = 4
		)

		startAmountNexus := uint64(1)
		startAmountNexusEth := ethgo.Ether(startAmountNexus)

		sendAmount := uint64(1)
		sendAmountDfm := new(big.Int).SetUint64(sendAmount)
		exp := new(big.Int).Exp(big.NewInt(10), big.NewInt(6), nil)
		sendAmountDfm.Mul(sendAmountDfm, exp)

		sendAmountEth := ethgo.Ether(sendAmount)

		ethBalanceBefore, err := cardanofw.GetEthAmount(ctx, apex.Nexus, userNexus)
		fmt.Printf("ETH Amount before Tx %d\n", ethBalanceBefore)
		require.NoError(t, err)

		nexusUsers := make([]*eth_wallet.Account, receivers)
		receiverAddresses := make([]string, receivers)

		// Create receivers
		for i := 0; i < receivers; i++ {
			user, err := apex.CreateAndFundNexusUser(ctx, startAmountNexus)
			require.NoError(t, err)

			err = cardanofw.WaitForEthAmount(context.Background(), apex.Nexus, user, func(val *big.Int) bool {
				return val.Cmp(startAmountNexusEth) == 0
			}, 30, 30*time.Second)
			require.NoError(t, err)

			receiverAddresses[i] = user.Address().String()
			nexusUsers[i] = user
		}

		fmt.Println("Nexus user created and funded")

		for sequenceIdx := 0; sequenceIdx < sequentialInstances; sequenceIdx++ {
			primeUsers := make([]*cardanofw.TestApexUser, parallelInstances)

			for i := 0; i < parallelInstances; i++ {
				primeUsers[i] = apex.CreateAndFundUser(t, ctx, uint64(10_000_000))
				require.NotNil(t, primeUsers[i])
			}

			var wg sync.WaitGroup
			for i := 0; i < parallelInstances; i++ {
				wg.Add(1)

				go func(sequence, idx int) {
					defer wg.Done()

					txHash, err := cardanofw.BridgeAmountFullMultipleReceiversNexus(t, ctx, txProviderPrime,
						apex.PrimeCluster.NetworkConfig(), apex.Bridge.PrimeMultisigAddr, receiverAddrNexus,
						primeUsers[idx].PrimeWallet, receiverAddresses, sendAmountDfm.Uint64())
					require.NoError(t, err)

					fmt.Printf("Seq: %v, Tx %v sent. hash: %s\n", sequence+1, idx+1, txHash)
				}(sequenceIdx, i)
			}

			wg.Wait()
		}

		fmt.Printf("Waiting for %v TXs\n", sequentialInstances*parallelInstances)

		var wgResults sync.WaitGroup

		transferedAmount := new(big.Int).SetInt64(int64(parallelInstances * sequentialInstances))
		transferedAmount.Mul(transferedAmount, sendAmountEth)
		ethExpectedBalance := big.NewInt(0).Add(startAmountNexusEth, transferedAmount)
		fmt.Printf("Expected ETH Amount after Txs %d\n", ethExpectedBalance)

		for i := 0; i < receivers; i++ {
			wgResults.Add(1)

			go func(receiverIdx int) {
				defer wgResults.Done()

				err = cardanofw.WaitForEthAmount(context.Background(), apex.Nexus, nexusUsers[receiverIdx],
					func(val *big.Int) bool {
						return val.Cmp(ethExpectedBalance) == 0
					}, 60, 30*time.Second)
				require.NoError(t, err)
				fmt.Printf("%v receiver, %v TXs confirmed\n", receiverIdx, sequentialInstances*parallelInstances)
			}(i)
		}

		wgResults.Wait()
	})

	t.Run("From Prime to Nexus sequential and parallel - one node goes off in the midle", func(t *testing.T) {
		const (
			sequentialInstances  = 5
			parallelInstances    = 6
			stopAfter            = time.Second * 60
			validatorStoppingIdx = 1
		)

		sendAmount := uint64(1)
		sendAmountDfm := new(big.Int).SetUint64(sendAmount)
		exp := new(big.Int).Exp(big.NewInt(10), big.NewInt(6), nil)
		sendAmountDfm.Mul(sendAmountDfm, exp)

		expectedAmount := ethgo.Ether(sendAmount)

		ethBalanceBefore, err := cardanofw.GetEthAmount(ctx, apex.Nexus, userNexus)
		fmt.Printf("ETH Amount before Tx %d\n", ethBalanceBefore)
		require.NoError(t, err)

		go func() {
			select {
			case <-ctx.Done():
				return
			case <-time.After(stopAfter):
				require.NoError(t, apex.Bridge.GetValidator(t, validatorStoppingIdx).Stop())
			}
		}()

		for sequenceIdx := 0; sequenceIdx < sequentialInstances; sequenceIdx++ {
			primeUsers := make([]*cardanofw.TestApexUser, parallelInstances)

			for i := 0; i < parallelInstances; i++ {
				primeUsers[i] = apex.CreateAndFundUser(t, ctx, uint64(5_000_000))
				require.NotNil(t, primeUsers[i])
			}

			var wg sync.WaitGroup
			for i := 0; i < parallelInstances; i++ {
				wg.Add(1)

				go func(sequence, idx int) {
					defer wg.Done()

					txHash, err := primeUsers[idx].BridgeNexusAmount(t, ctx, txProviderPrime, apex.Bridge.PrimeMultisigAddr,
						receiverAddrNexus, sendAmountDfm.Uint64(), apex.PrimeCluster.NetworkConfig(), receiverAddrNexus)
					require.NoError(t, err)

					fmt.Printf("Seq: %v, Tx %v sent. hash: %s\n", sequence+1, idx+1, txHash)
				}(sequenceIdx, i)
			}

			wg.Wait()
		}

		transferedAmount := new(big.Int).SetInt64(int64(parallelInstances * sequentialInstances))
		transferedAmount.Mul(transferedAmount, expectedAmount)
		ethExpectedBalance := big.NewInt(0).Add(ethBalanceBefore, transferedAmount)
		fmt.Printf("Expected ETH Amount after Txs %d\n", ethExpectedBalance)

		err = cardanofw.WaitForEthAmount(context.Background(), apex.Nexus, userNexus, func(val *big.Int) bool {
			return val.Cmp(ethExpectedBalance) == 0
		}, 60, 30*time.Second)
		require.NoError(t, err)

		fmt.Printf("TXs on Nexus expected amount received, err: %v\n", err)

		// nothing else should be bridged for 2 minutes
		err = cardanofw.WaitForEthAmount(context.Background(), apex.Nexus, userNexus, func(val *big.Int) bool {
			return val.Cmp(ethExpectedBalance) > 0
		}, 60, 30*time.Second)
		assert.ErrorIs(t, err, wallet.ErrWaitForTransactionTimeout, "more tokens than expected are on prime")

		fmt.Printf("TXs on prime finished with success: %v\n", err != nil)
	})
}

func TestE2E_ApexBridgeWithNexus_InvalidScenarios(t *testing.T) {
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

	userPrime := apex.CreateAndFundUser(t, ctx, uint64(10_000_000))
	require.NotNil(t, userPrime)

	txProviderPrime := apex.GetPrimeTxProvider()

	userNexus, err := apex.CreateAndFundNexusUser(ctx, 0)
	require.NoError(t, err)

	fmt.Println("Nexus user created and funded")

	receiverAddrNexus := userNexus.Address().String()
	fmt.Printf("Nexus receiver Addr: %s\n", receiverAddrNexus)

	t.Run("Submitter not enough funds", func(t *testing.T) {
		sendAmount := uint64(100)
		sendAmountDfm := new(big.Int).SetUint64(sendAmount)
		exp := new(big.Int).Exp(big.NewInt(10), big.NewInt(6), nil)
		sendAmountDfm.Mul(sendAmountDfm, exp)

		ethBalanceBefore, err := cardanofw.GetEthAmount(ctx, apex.Nexus, userNexus)
		fmt.Printf("ETH Amount before Tx %d\n", ethBalanceBefore)
		require.NoError(t, err)

		_, err = userPrime.BridgeNexusAmount(t, ctx, txProviderPrime, apex.Bridge.PrimeMultisigAddr,
			receiverAddrNexus, sendAmountDfm.Uint64(), apex.PrimeCluster.NetworkConfig(), receiverAddrNexus)
		require.ErrorContains(t, err, "not enough funds")
	})

	t.Run("Multiple submitters - not enough funds", func(t *testing.T) {
		submitters := 5

		for i := 0; i < submitters; i++ {
			sendAmount := uint64(100)
			sendAmountDfm := new(big.Int).SetUint64(sendAmount)
			exp := new(big.Int).Exp(big.NewInt(10), big.NewInt(6), nil)
			sendAmountDfm.Mul(sendAmountDfm, exp)

			ethBalanceBefore, err := cardanofw.GetEthAmount(ctx, apex.Nexus, userNexus)
			fmt.Printf("ETH Amount before Tx %d\n", ethBalanceBefore)
			require.NoError(t, err)

			_, err = userPrime.BridgeNexusAmount(t, ctx, txProviderPrime, apex.Bridge.PrimeMultisigAddr,
				receiverAddrNexus, sendAmountDfm.Uint64(), apex.PrimeCluster.NetworkConfig(), receiverAddrNexus)
			require.ErrorContains(t, err, "not enough funds")
		}
	})

	t.Run("Multiple submitters - not enough funds parallel", func(t *testing.T) {
		sendAmount := uint64(100)
		sendAmountDfm := new(big.Int).SetUint64(sendAmount)
		exp := new(big.Int).Exp(big.NewInt(10), big.NewInt(6), nil)
		sendAmountDfm.Mul(sendAmountDfm, exp)

		ethBalanceBefore, err := cardanofw.GetEthAmount(ctx, apex.Nexus, userNexus)
		fmt.Printf("ETH Amount before Tx %d\n", ethBalanceBefore)
		require.NoError(t, err)

		// Fund wallets
		instances := 5
		primeUsers := make([]*cardanofw.TestApexUser, instances)

		for i := 0; i < instances; i++ {
			primeUsers[i] = apex.CreateAndFundUser(t, ctx, uint64(1_000_000))
			require.NotNil(t, primeUsers[i])
		}

		var wg sync.WaitGroup
		for i := 0; i < instances; i++ {
			wg.Add(1)

			go func(idx int) {
				defer wg.Done()

				_, err := primeUsers[idx].BridgeNexusAmount(t, ctx, txProviderPrime, apex.Bridge.PrimeMultisigAddr,
					receiverAddrNexus, sendAmountDfm.Uint64(), apex.PrimeCluster.NetworkConfig(), receiverAddrNexus)
				require.ErrorContains(t, err, "not enough funds")
			}(i)
		}

		wg.Wait()
	})

	t.Run("Submitted invalid metadata", func(t *testing.T) {
		sendAmount := uint64(1)
		sendAmountDfm := new(big.Int).SetUint64(sendAmount)
		exp := new(big.Int).Exp(big.NewInt(10), big.NewInt(6), nil)
		sendAmountDfm.Mul(sendAmountDfm, exp)

		const feeAmount = 1_100_000

		receivers := map[string]uint64{
			receiverAddrNexus: sendAmount * 10, // 10Ada
		}

		bridgingRequestMetadata, err := cardanofw.CreateMetaData(
			userPrime.PrimeAddress, receivers, cardanofw.ChainIDNexus, feeAmount)
		require.NoError(t, err)

		// Send only half bytes of metadata making it invalid
		bridgingRequestMetadata = bridgingRequestMetadata[0 : len(bridgingRequestMetadata)/2]

		_, err = cardanofw.SendTx(ctx, txProviderPrime, userPrime.PrimeWallet,
			sendAmountDfm.Uint64()+feeAmount, apex.Bridge.PrimeMultisigAddr,
			apex.PrimeCluster.NetworkConfig(), bridgingRequestMetadata)
		require.Error(t, err)
	})

	t.Run("Submitted invalid metadata - wrong type", func(t *testing.T) {
		sendAmount := uint64(1)
		sendAmountDfm := new(big.Int).SetUint64(sendAmount)
		exp := new(big.Int).Exp(big.NewInt(10), big.NewInt(6), nil)
		sendAmountDfm.Mul(sendAmountDfm, exp)

		const feeAmount = 1_100_000

		receivers := map[string]uint64{
			receiverAddrNexus: sendAmount * 10, // 10Ada
		}

		var transactions = make([]cardanofw.BridgingRequestMetadataTransaction, 0, len(receivers))
		for addr, amount := range receivers {
			transactions = append(transactions, cardanofw.BridgingRequestMetadataTransaction{
				Address: cardanofw.SplitString(addr, 40),
				Amount:  amount,
			})
		}

		metadata := map[string]interface{}{
			"1": map[string]interface{}{
				"t":  "transaction", // should be "bridge"
				"d":  cardanofw.ChainIDNexus,
				"s":  cardanofw.SplitString(userPrime.PrimeAddress, 40),
				"tx": transactions,
				"fa": feeAmount,
			},
		}

		bridgingRequestMetadata, err := json.Marshal(metadata)
		require.NoError(t, err)

		txHash, err := cardanofw.SendTx(ctx, txProviderPrime, userPrime.PrimeWallet,
			sendAmountDfm.Uint64()+feeAmount, apex.Bridge.PrimeMultisigAddr,
			apex.PrimeCluster.NetworkConfig(), bridgingRequestMetadata)
		require.NoError(t, err)

		apiURL, err := apex.Bridge.GetBridgingAPI()
		require.NoError(t, err)

		requestURL := fmt.Sprintf(
			"%s/api/BridgingRequestState/Get?chainId=%s&txHash=%s", apiURL, "prime", txHash)

		_, err = cardanofw.WaitForRequestState("InvalidState", ctx, requestURL, apiKey, 60)
		require.Error(t, err)
		require.ErrorContains(t, err, "Timeout")
	})

	t.Run("Submitted invalid metadata - invalid destination", func(t *testing.T) {
		sendAmount := uint64(1)
		sendAmountDfm := new(big.Int).SetUint64(sendAmount)
		exp := new(big.Int).Exp(big.NewInt(10), big.NewInt(6), nil)
		sendAmountDfm.Mul(sendAmountDfm, exp)

		const feeAmount = 1_100_000

		receivers := map[string]uint64{
			receiverAddrNexus: sendAmount * 10, // 10Ada
		}

		var transactions = make([]cardanofw.BridgingRequestMetadataTransaction, 0, len(receivers))
		for addr, amount := range receivers {
			transactions = append(transactions, cardanofw.BridgingRequestMetadataTransaction{
				Address: cardanofw.SplitString(addr, 40),
				Amount:  amount,
			})
		}

		metadata := map[string]interface{}{
			"1": map[string]interface{}{
				"t":  "bridge",
				"d":  "", // should be destination chain address
				"s":  cardanofw.SplitString(userPrime.PrimeAddress, 40),
				"tx": transactions,
				"fa": feeAmount,
			},
		}

		bridgingRequestMetadata, err := json.Marshal(metadata)
		require.NoError(t, err)

		txHash, err := cardanofw.SendTx(ctx, txProviderPrime, userPrime.PrimeWallet,
			sendAmountDfm.Uint64()+feeAmount, apex.Bridge.PrimeMultisigAddr,
			apex.PrimeCluster.NetworkConfig(), bridgingRequestMetadata)
		require.NoError(t, err)

		apiURL, err := apex.Bridge.GetBridgingAPI()
		require.NoError(t, err)

		requestURL := fmt.Sprintf(
			"%s/api/BridgingRequestState/Get?chainId=%s&txHash=%s", apiURL, "prime", txHash)

		_, err = cardanofw.WaitForRequestState("InvalidState", ctx, requestURL, apiKey, 60)
		require.Error(t, err)
		require.ErrorContains(t, err, "Timeout")
	})

	t.Run("Submitted invalid metadata - invalid sender", func(t *testing.T) {
		sendAmount := uint64(1)
		sendAmountDfm := new(big.Int).SetUint64(sendAmount)
		exp := new(big.Int).Exp(big.NewInt(10), big.NewInt(6), nil)
		sendAmountDfm.Mul(sendAmountDfm, exp)

		const feeAmount = 1_100_000

		receivers := map[string]uint64{
			receiverAddrNexus: sendAmount * 10, // 10Ada
		}

		var transactions = make([]cardanofw.BridgingRequestMetadataTransaction, 0, len(receivers))
		for addr, amount := range receivers {
			transactions = append(transactions, cardanofw.BridgingRequestMetadataTransaction{
				Address: cardanofw.SplitString(addr, 40),
				Amount:  amount,
			})
		}

		metadata := map[string]interface{}{
			"1": map[string]interface{}{
				"t":  "bridge",
				"d":  cardanofw.ChainIDNexus,
				"s":  "", // should be sender address (max len 40)
				"tx": transactions,
				"fa": feeAmount,
			},
		}

		bridgingRequestMetadata, err := json.Marshal(metadata)
		require.NoError(t, err)

		txHash, err := cardanofw.SendTx(ctx, txProviderPrime, userPrime.PrimeWallet,
			sendAmountDfm.Uint64()+feeAmount, apex.Bridge.PrimeMultisigAddr,
			apex.PrimeCluster.NetworkConfig(), bridgingRequestMetadata)
		require.NoError(t, err)

		apiURL, err := apex.Bridge.GetBridgingAPI()
		require.NoError(t, err)

		requestURL := fmt.Sprintf(
			"%s/api/BridgingRequestState/Get?chainId=%s&txHash=%s", apiURL, "prime", txHash)

		_, err = cardanofw.WaitForRequestState("InvalidState", ctx, requestURL, apiKey, 60)
		require.Error(t, err)
		require.ErrorContains(t, err, "Timeout")
	})

	t.Run("Submitted invalid metadata - emty tx", func(t *testing.T) {
		sendAmount := uint64(0)
		sendAmountDfm := new(big.Int).SetUint64(sendAmount)
		exp := new(big.Int).Exp(big.NewInt(10), big.NewInt(6), nil)
		sendAmountDfm.Mul(sendAmountDfm, exp)

		const feeAmount = 1_100_000

		var transactions = make([]cardanofw.BridgingRequestMetadataTransaction, 0)

		metadata := map[string]interface{}{
			"1": map[string]interface{}{
				"t":  "bridge",
				"d":  cardanofw.ChainIDNexus,
				"s":  cardanofw.SplitString(userPrime.PrimeAddress, 40),
				"tx": transactions, // should not be empty
				"fa": feeAmount,
			},
		}

		bridgingRequestMetadata, err := json.Marshal(metadata)
		require.NoError(t, err)

		txHash, err := cardanofw.SendTx(ctx, txProviderPrime, userPrime.PrimeWallet,
			sendAmountDfm.Uint64()+feeAmount, apex.Bridge.PrimeMultisigAddr,
			apex.PrimeCluster.NetworkConfig(), bridgingRequestMetadata)
		require.NoError(t, err)

		apiURL, err := apex.Bridge.GetBridgingAPI()
		require.NoError(t, err)

		requestURL := fmt.Sprintf(
			"%s/api/BridgingRequestState/Get?chainId=%s&txHash=%s", apiURL, "prime", txHash)

		_, err = cardanofw.WaitForRequestState("InvalidState", ctx, requestURL, apiKey, 60)
		require.Error(t, err)
		require.ErrorContains(t, err, "Timeout")
	})
}

func TestE2E_ApexBridgeWithNexus_ValidScenarios_BigTest(t *testing.T) {
	if shouldRun := os.Getenv("RUN_E2E_BIG_TEST"); shouldRun != "true" {
		t.Skip()
	}

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

	userPrime := apex.CreateAndFundUser(t, ctx, uint64(500_000_000))
	require.NotNil(t, userPrime)

	txProviderPrime := apex.GetPrimeTxProvider()

	startAmountNexus := uint64(1)
	expectedAmountNexus := ethgo.Ether(startAmountNexus)

	userNexus, err := apex.CreateAndFundNexusUser(ctx, startAmountNexus)
	require.NoError(t, err)

	err = cardanofw.WaitForEthAmount(context.Background(), apex.Nexus, userNexus, func(val *big.Int) bool {
		return val.Cmp(expectedAmountNexus) == 0
	}, 10, 10)
	require.NoError(t, err)

	fmt.Println("Nexus user created and funded")

	receiverAddrNexus := userNexus.Address().String()
	fmt.Printf("Nexus receiver Addr: %s\n", receiverAddrNexus)

	//nolint:dupl
	t.Run("From Prime to Nexus 200x 5min 90%", func(t *testing.T) {
		sendAmount := uint64(1)
		sendAmountDfm := new(big.Int).SetUint64(sendAmount)
		exp := new(big.Int).Exp(big.NewInt(10), big.NewInt(6), nil)
		sendAmountDfm.Mul(sendAmountDfm, exp)

		sendAmountEth := ethgo.Ether(sendAmount)

		ethBalanceBefore, err := cardanofw.GetEthAmount(ctx, apex.Nexus, userNexus)
		fmt.Printf("ETH Amount before Tx %d\n", ethBalanceBefore)
		require.NoError(t, err)

		instances := 200
		primeUsers := make([]*cardanofw.TestApexUser, instances)

		maxWaitTime := 300
		successChance := 90 // 90%
		succeededCount := int64(0)

		fmt.Printf("Funding %v Wallets\n", instances)

		for i := 0; i < instances; i++ {
			primeUsers[i] = apex.CreateAndFundUser(t, ctx, uint64(10_000_000))
			require.NotNil(t, primeUsers[i])
		}

		fmt.Printf("Funding complete\n")
		fmt.Printf("Sending transactions\n")

		var wg sync.WaitGroup
		for i := 0; i < instances; i++ {
			wg.Add(1)

			go func(idx int) {
				defer wg.Done()

				if successChance > rand.Intn(100) {
					succeededCount++
					sleepTime := rand.Intn(maxWaitTime)
					time.Sleep(time.Second * time.Duration(sleepTime))

					txHash, err := primeUsers[idx].BridgeNexusAmount(t, ctx, txProviderPrime, apex.Bridge.PrimeMultisigAddr,
						receiverAddrNexus, sendAmountDfm.Uint64(), apex.PrimeCluster.NetworkConfig(), receiverAddrNexus)
					require.NoError(t, err)
					fmt.Printf("Tx %v sent. hash: %s\n", idx+1, txHash)
				} else {
					const feeAmount = 1_100_000

					receivers := map[string]uint64{
						receiverAddrNexus: sendAmount * 10, // 10Ada
					}

					bridgingRequestMetadata, err := cardanofw.CreateMetaData(
						primeUsers[idx].PrimeAddress, receivers, cardanofw.ChainIDNexus, feeAmount)
					require.NoError(t, err)

					txHash, err := cardanofw.SendTx(ctx, txProviderPrime, primeUsers[idx].PrimeWallet,
						sendAmountDfm.Uint64()+feeAmount, apex.Bridge.PrimeMultisigAddr,
						apex.PrimeCluster.NetworkConfig(), bridgingRequestMetadata)
					require.NoError(t, err)

					fmt.Printf("Tx %v sent without waiting for confirmation. hash: %s\n", idx+1, txHash)
				}
			}(i)
		}

		wg.Wait()

		fmt.Printf("All tx sent, waiting for confirmation.\n")

		var newAmount *big.Int

		expectedAmount := new(big.Int).SetInt64(succeededCount)
		expectedAmount.Mul(expectedAmount, sendAmountEth)
		expectedAmount.Add(expectedAmount, ethBalanceBefore)

		for i := 0; i < instances; i++ {
			newAmount, err = cardanofw.GetEthAmount(ctx, apex.Nexus, userNexus)
			require.NoError(t, err)
			fmt.Printf("Current Amount Nexus: %v. Expected: %v\n", newAmount, expectedAmount)

			if newAmount.Cmp(expectedAmount) >= 0 {
				break
			}

			time.Sleep(time.Second * 10)
		}

		fmt.Printf("Success count: %v. prevAmount: %v. newAmount: %v. expectedAmount: %v\n", succeededCount, ethBalanceBefore, newAmount, expectedAmount)
	})

	//nolint:dupl
	t.Run("From Prime to Nexus 1000x 20min 90%", func(t *testing.T) {
		sendAmount := uint64(1)
		sendAmountDfm := new(big.Int).SetUint64(sendAmount)
		exp := new(big.Int).Exp(big.NewInt(10), big.NewInt(6), nil)
		sendAmountDfm.Mul(sendAmountDfm, exp)

		sendAmountEth := ethgo.Ether(sendAmount)

		ethBalanceBefore, err := cardanofw.GetEthAmount(ctx, apex.Nexus, userNexus)
		fmt.Printf("ETH Amount before Tx %d\n", ethBalanceBefore)
		require.NoError(t, err)

		instances := 1000
		primeUsers := make([]*cardanofw.TestApexUser, instances)

		maxWaitTime := 1200
		successChance := 90 // 90%
		succeededCount := int64(0)

		fmt.Printf("Funding %v Wallets\n", instances)

		for i := 0; i < instances; i++ {
			primeUsers[i] = apex.CreateAndFundUser(t, ctx, uint64(10_000_000))
			require.NotNil(t, primeUsers[i])
		}

		fmt.Printf("Funding complete\n")
		fmt.Printf("Sending transactions\n")

		var wg sync.WaitGroup
		for i := 0; i < instances; i++ {
			wg.Add(1)

			go func(idx int) {
				defer wg.Done()

				if successChance > rand.Intn(100) {
					succeededCount++
					sleepTime := rand.Intn(maxWaitTime)
					time.Sleep(time.Second * time.Duration(sleepTime))

					txHash, err := primeUsers[idx].BridgeNexusAmount(t, ctx, txProviderPrime, apex.Bridge.PrimeMultisigAddr,
						receiverAddrNexus, sendAmountDfm.Uint64(), apex.PrimeCluster.NetworkConfig(), receiverAddrNexus)
					require.NoError(t, err)
					fmt.Printf("Tx %v sent. hash: %s\n", idx+1, txHash)
				} else {
					const feeAmount = 1_100_000

					receivers := map[string]uint64{
						receiverAddrNexus: sendAmount * 10, // 10Ada
					}

					bridgingRequestMetadata, err := cardanofw.CreateMetaData(
						primeUsers[idx].PrimeAddress, receivers, cardanofw.ChainIDNexus, feeAmount)
					require.NoError(t, err)

					txHash, err := cardanofw.SendTx(ctx, txProviderPrime, primeUsers[idx].PrimeWallet,
						sendAmountDfm.Uint64()+feeAmount, apex.Bridge.PrimeMultisigAddr,
						apex.PrimeCluster.NetworkConfig(), bridgingRequestMetadata)
					require.NoError(t, err)

					fmt.Printf("Tx %v sent without waiting for confirmation. hash: %s\n", idx+1, txHash)
				}
			}(i)
		}

		wg.Wait()

		fmt.Printf("All tx sent, waiting for confirmation.\n")

		var newAmount *big.Int

		expectedAmount := new(big.Int).SetInt64(succeededCount)
		expectedAmount.Mul(expectedAmount, sendAmountEth)
		expectedAmount.Add(expectedAmount, ethBalanceBefore)

		for i := 0; i < instances; i++ {
			newAmount, err = cardanofw.GetEthAmount(ctx, apex.Nexus, userNexus)
			require.NoError(t, err)
			fmt.Printf("Current Amount Nexus: %v. Expected: %v\n", newAmount, expectedAmount)

			if newAmount.Cmp(expectedAmount) >= 0 {
				break
			}

			time.Sleep(time.Second * 10)
		}

		fmt.Printf("Success count: %v. prevAmount: %v. newAmount: %v. expectedAmount: %v\n", succeededCount, ethBalanceBefore, newAmount, expectedAmount)
	})
}
