package e2e

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/e2e-polybft/cardanofw"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/e2ehelper"
	infracommon "github.com/Ethernal-Tech/cardano-infrastructure/common"
	cardanowallet "github.com/Ethernal-Tech/cardano-infrastructure/wallet"
	"github.com/Ethernal-Tech/ethgo"
	"github.com/stretchr/testify/require"
)

var (
	timeoutConfig = e2ehelper.NewTimeoutConfig(
		e2ehelper.WithBridgingNumRetries(defaultDstNumRetries),
		e2ehelper.WithBridgingRetryWaitTime(defaultDstWaitTime),
	)
	bridgingOpts = []e2ehelper.ExecuteBridgingOption{e2ehelper.WithTimeoutConfig(timeoutConfig)}
)

const (
	defaultDstNumRetries = 120
	defaultDstWaitTime   = 30 * time.Second
)

type BridgingRequest struct {
	src  string
	dest string
}

// This is called manually when needed. It is not called on every run.
func Test_E2E_TestnetDistributeFromPrimeToFunderWallets(t *testing.T) {
	const (
		apexAmountToBridge = 10_000
	)

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	apex, err := cardanofw.SetupRemoteApexBridge(t, cardanofw.GetTestnetApexBridgeConfig())
	require.NoError(t, err)

	require.NotNil(t, apex.FunderUser)

	balances := getUserLovelaceBalances(ctx, apex, nil)
	printUserBalances(apex, nil, balances)

	sendAmountDfm := cardanofw.ApexToDfm(new(big.Int).SetUint64(apexAmountToBridge))

	if IsVectorEnabled(apex) {
		fmt.Printf("bridging %v apex to vector\n", apexAmountToBridge)

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, apex.FunderUser, apex.FunderUser, cardanofw.ChainIDPrime, cardanofw.ChainIDVector,
			sendAmountDfm, cardanofw.AP3XTokenID, true, bridgingOpts...)
	}

	fmt.Printf("bridging %v apex to nexus\n", apexAmountToBridge)
	e2ehelper.ExecuteSingleBridging(
		t, ctx, apex, apex.FunderUser, apex.FunderUser, cardanofw.ChainIDPrime, cardanofw.ChainIDNexus,
		sendAmountDfm, cardanofw.AP3XTokenID, true, bridgingOpts...)

	balances = getUserLovelaceBalances(ctx, apex, nil)
	printUserBalances(apex, nil, balances)
}
func Test_E2E_TestnetDefund(t *testing.T) {
	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	apex, err := cardanofw.SetupRemoteApexBridge(t, cardanofw.GetTestnetApexBridgeConfig())
	require.NoError(t, err)

	require.NotNil(t, apex.FunderUser)

	var wg sync.WaitGroup

	balances := getUserLovelaceBalances(ctx, apex, apex.Users)
	printUserBalances(apex, apex.Users, balances)

	fmt.Printf("defunding the wallets\n")

	chains := getEnabledChains(apex)

	chainInfo := map[string]struct {
		info        *cardanofw.CardanoChainInfo
		networkType cardanowallet.CardanoNetworkType
	}{
		cardanofw.ChainIDPrime:  {info: &apex.PrimeInfo, networkType: apex.Config.PrimeConfig.NetworkType},
		cardanofw.ChainIDVector: {info: &apex.VectorInfo, networkType: apex.Config.VectorConfig.NetworkType},
	}

	protParamsCached := map[string][]byte{}

	for _, user := range apex.Users {
		for _, chain := range chains {
			addr := user.GetAddress(chain)

			var (
				change         *big.Int
				balanceAtleast *big.Int
			)

			if chain == cardanofw.ChainIDNexus {
				change = new(big.Int).SetUint64(cardanofw.PotentialFee)
				balanceAtleast = new(big.Int).Set(change)
			} else {
				txProvider, err := chainInfo[chain].info.GetTxProvider()
				require.NoError(t, err)

				if _, exist := protParamsCached[chain]; !exist {
					protParamsCached[chain], err = infracommon.ExecuteWithRetry(ctx, func(ctx context.Context) ([]byte, error) {
						return txProvider.GetProtocolParameters(ctx)
					})
					require.NoError(t, err)
				}

				utxos, err := infracommon.ExecuteWithRetry(
					ctx, func(ctx context.Context) ([]cardanowallet.Utxo, error) {
						return txProvider.GetUtxos(ctx, addr)
					},
				)
				require.NoError(t, err)

				balance := cardanowallet.GetUtxosSum(utxos)

				tokens, err := cardanowallet.GetTokensFromSumMap(balance)
				require.NoError(t, err)

				txBuilder, err := cardanowallet.NewTxBuilder(cardanowallet.ResolveCardanoCliBinary(chainInfo[chain].networkType))
				require.NoError(t, err)
				defer txBuilder.Dispose()

				minUtxo, err := txBuilder.SetProtocolParameters(protParamsCached[chain]).CalculateMinUtxo(cardanowallet.TxOutputWithRefScript{
					TxOutput: cardanowallet.TxOutput{
						Addr:   addr,
						Tokens: tokens,
					},
				})
				require.NoError(t, err)

				change = new(big.Int).SetUint64(max(minUtxo, cardanofw.MinUTxODefaultValue) + cardanofw.PotentialFee)
				balanceAtleast = big.NewInt(0).Add(new(big.Int).SetUint64(cardanofw.MinUTxODefaultValue), change)
			}

			balance, exists := balances[addr]
			if !exists || balance.Cmp(balanceAtleast) != 1 {
				continue
			}

			toDefund := big.NewInt(0).Sub(balance, change)

			wg.Add(1)

			go func(user *cardanofw.TestApexUser, chain string) {
				defer wg.Done()

				fmt.Printf("Defunding %s address: %s\n", chain, addr)

				_, err := apex.SubmitTx(ctx, chain, user, apex.FunderUser.GetAddress(chain), toDefund, nil, nil, nil)
				if err != nil {
					fmt.Printf("error while defunding %s address: %s, err: %v\n", chain, addr, err)
				}
			}(user, chain)
		}
	}

	wg.Wait()

	balances = getUserLovelaceBalances(ctx, apex, apex.Users)
	printUserBalances(apex, apex.Users, balances)

	fmt.Printf("done\n")
}

func Test_E2E_TestnetFund(t *testing.T) {
	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	apex, err := cardanofw.SetupRemoteApexBridge(t, cardanofw.GetTestnetApexBridgeConfig())
	require.NoError(t, err)

	const (
		apexToFund = 100
	)

	require.NotNil(t, apex.FunderUser)

	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		addrErrs = make(map[string]error)
	)

	balances := getUserLovelaceBalances(ctx, apex, apex.Users)
	printUserBalances(apex, apex.Users, balances)

	fmt.Printf("funding the wallets\n")

	chains := getEnabledChains(apex)

	for _, user := range apex.Users {
		fmt.Printf("-----------------------------\n")

		for _, chain := range chains {
			wg.Add(1)

			go func(user *cardanofw.TestApexUser, chain string) {
				defer wg.Done()

				addr := user.GetAddress(chain)

				fmt.Printf("Funding %s address: %s\n", chain, addr)

				// resubmit the transaction in case of error because of a possible rollback
				_, err := infracommon.ExecuteWithRetry(ctx, func(ctx context.Context) (string, error) {
					txHash, err := apex.SubmitTx(ctx, chain, apex.FunderUser, addr, cardanofw.ApexToDfm(big.NewInt(apexToFund)), nil, nil, nil)
					if errors.Is(err, infracommon.ErrRetryTimeout) {
						return "", infracommon.ErrRetryTryAgain
					}

					return txHash, err
				})
				if err != nil {
					fmt.Printf("error while funding %s address: %s, err: %v\n", chain, addr, err)

					mu.Lock()
					addrErrs[addr] = err
					mu.Unlock()
				}
			}(user, chain)
		}

		wg.Wait()
	}

	balances = getUserLovelaceBalances(ctx, apex, apex.Users)
	printUserBalances(apex, apex.Users, balances)

	errs := make([]error, 0, len(addrErrs))
	for _, err := range addrErrs {
		errs = append(errs, err)
	}

	err = errors.Join(errs...)
	require.NoError(t, err)

	fmt.Printf("done\n")
}

func Test_E2E_SanityCheck(t *testing.T) {
	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	apex, err := cardanofw.SetupRemoteApexBridge(t, cardanofw.GetTestnetApexBridgeConfig())
	require.NoError(t, err)

	var (
		user             = apex.Users[0]
		sendAmount       = cardanofw.ApexToDfm(big.NewInt(1))
		bridgingRequests = getEnabledDirections(apex)
	)

	for _, dir := range bridgingRequests {
		fmt.Printf("bridging from %s to %s\n", dir.src, dir.dest)

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, dir.src, dir.dest, sendAmount, cardanofw.AP3XTokenID, true, bridgingOpts...)
	}
}

func TestE2E_ApexTestnetBridge_ValidScenarios(t *testing.T) {
	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	apex, err := cardanofw.SetupRemoteApexBridge(t, cardanofw.GetTestnetApexBridgeConfig())
	require.NoError(t, err)

	if IsVectorEnabled(apex) {
		t.Run("From Prime to Vector sequential and parallel with max receivers", func(t *testing.T) {
			const (
				sequentialInstances = 3
				parallelInstances   = 10
			)

			PrimeToVectorSequentialAndParallelWithMaxReceivers(t, ctx, apex, sequentialInstances, parallelInstances, bridgingOpts...)
		})

		t.Run("Prime and Vector both directions sequential and parallel", func(t *testing.T) {
			const (
				sequentialInstances = 3
				parallelInstances   = 6
			)

			receiverUser := apex.Users[parallelInstances]

			PrimeVectorBothDirectionsSequentialAndParallel(t, ctx, apex, receiverUser, sequentialInstances, parallelInstances, bridgingOpts...)
		})

		t.Run("Vector and Nexus both directions sequential and parallel", func(t *testing.T) {
			const (
				sequentialInstances = 3
				parallelInstances   = 6
			)

			receiverUser := apex.Users[parallelInstances]
			sendAmountDfm := cardanofw.WeiToDfm(ethgo.Ether(1))

			DstNexusBothDirectionsSequentialAndParallel(
				t, ctx, apex, cardanofw.ChainIDVector, receiverUser, sequentialInstances, parallelInstances, sendAmountDfm, bridgingOpts...)
		})
	}

	t.Run("From Prime to Nexus sequential and parallel with max receivers", func(t *testing.T) {
		const (
			sequentialInstances = 3
			parallelInstances   = 10
		)

		sendAmountDfm := cardanofw.WeiToDfm(ethgo.Ether(1))

		DstNexusSequentialAndParallelWithMaxReceivers(t, ctx, apex, cardanofw.ChainIDPrime, sequentialInstances, parallelInstances, sendAmountDfm, bridgingOpts...)
	})

	t.Run("Prime and Nexus both directions sequential and parallel", func(t *testing.T) {
		const (
			sequentialInstances = 3
			parallelInstances   = 6
		)

		receiverUser := apex.Users[parallelInstances]
		sendAmountDfm := cardanofw.WeiToDfm(ethgo.Ether(1))

		DstNexusBothDirectionsSequentialAndParallel(
			t, ctx, apex, cardanofw.ChainIDPrime, receiverUser, sequentialInstances, parallelInstances, sendAmountDfm, bridgingOpts...)
	})

	t.Run("From Nexus to Prime sequential and parallel max receivers", func(t *testing.T) {
		const (
			sequentialInstances = 3
			parallelInstances   = 10
		)

		sendAmountDfm := cardanofw.WeiToDfm(ethgo.Ether(1))

		SrcNexusSequentialAndParallelWithMaxReceivers(t, ctx, apex, cardanofw.ChainIDPrime, sequentialInstances, parallelInstances, sendAmountDfm, bridgingOpts...)
	})
}

func TestE2E_ApexTestnetBridge_InvalidScenarios(t *testing.T) {
	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	apex, err := cardanofw.SetupRemoteApexBridge(t, cardanofw.GetTestnetApexBridgeConfig())
	require.NoError(t, err)

	const (
		requestStateTimeoutSec = 1500
		retryDelaySec          = 5
	)

	primeTestConfig := newTestConfig(
		t, apex, apex.Config.PrimeConfig, &apex.PrimeInfo, cardanofw.ChainIDVector, cardanofw.AP3XTokenID)

	srcChain := cardanofw.ChainIDPrime

	if IsVectorEnabled(apex) {
		t.Run("1. Prime to Vector mismatch submitted and receiver amounts", func(t *testing.T) {
			executeInvalidMismatchSendLovelaceAmount(
				t, ctx, apex, primeTestConfig, apex.Users[0], requestStateTimeoutSec, retryDelaySec, true, 0)
		})

		t.Run("2. Prime to Vector submitted invalid metadata - sliced off", func(t *testing.T) {
			PrimeToVectorInvalidMetadataSlicedOff(t, ctx, apex, apex.Users[1])
		})

		t.Run("3. Prime to Vector submitted invalid metadata - wrong type", func(t *testing.T) {
			executeInvalidMetadataType(
				t, ctx, apex, primeTestConfig, apex.Users[2], requestStateTimeoutSec, retryDelaySec, true, 0)
		})

		t.Run("4. Prime to Vector submitted invalid metadata - invalid destination", func(t *testing.T) {
			executeInvalidDestination(
				t, ctx, apex, primeTestConfig, apex.Users[3], requestStateTimeoutSec, retryDelaySec, true, 0)
		})

		t.Run("5. Prime to Vector submitted invalid metadata - invalid sender", func(t *testing.T) {
			executeInvalidMetadataInvalidSender(
				t, ctx, apex, primeTestConfig, apex.Users[4], requestStateTimeoutSec, 0)
		})

		t.Run("6. Prime to Vector submitted invalid metadata - empty receivers", func(t *testing.T) {
			executeInvalidEmptyReceivers(
				t, ctx, apex, primeTestConfig, apex.Users[5], requestStateTimeoutSec, retryDelaySec, false, 0)
		})
	}

	t.Run("Prime to Nexus submitter not enough funds", func(t *testing.T) {
		sendAmountDfm := cardanofw.WeiToDfm(ethgo.Ether(500_000))

		DstNexusSubmitterNotEnoughFunds(t, ctx, apex, srcChain, apex.Users[6], sendAmountDfm)
	})

	t.Run("Prime to Nexus submitted invalid metadata - sliced off", func(t *testing.T) {
		sendAmountDfm := cardanofw.WeiToDfm(ethgo.Ether(1))

		DstNexusInvalidMetadataSlicedOff(t, ctx, apex, srcChain, apex.Users[7], sendAmountDfm)
	})

	t.Run("Prime to Nexus submitted invalid metadata - wrong type", func(t *testing.T) {
		sendAmountDfm := cardanofw.WeiToDfm(ethgo.Ether(1))

		DstNexusInvalidMetadataWrongType(t, ctx, apex, srcChain, apex.Users[8], requestStateTimeoutSec, sendAmountDfm)
	})

	t.Run("Prime to Nexus submitted invalid metadata - invalid destination", func(t *testing.T) {
		sendAmountDfm := cardanofw.WeiToDfm(ethgo.Ether(1))

		DstNexusInvalidMetadataInvalidDestination(t, ctx, apex, srcChain, apex.Users[9], requestStateTimeoutSec, sendAmountDfm)
	})

	t.Run("Prime to Nexus submitted invalid metadata - invalid sender", func(t *testing.T) {
		sendAmountDfm := cardanofw.WeiToDfm(ethgo.Ether(1))

		DstNexusInvalidMetadataInvalidSender(t, ctx, apex, srcChain, apex.Users[0], requestStateTimeoutSec, sendAmountDfm)
	})

	t.Run("Prime to Nexus submitted invalid metadata - empty tx", func(t *testing.T) {
		sendAmountDfm := cardanofw.WeiToDfm(ethgo.Ether(1))

		DstNexusInvalidMetadataInvalidTransactions(t, ctx, apex, srcChain, apex.Users[1], requestStateTimeoutSec, sendAmountDfm)
	})

	t.Run("Nexus to Prime submitter not enough funds", func(t *testing.T) {
		SrcNexusSubmitterNotEnoughFunds(t, ctx, apex, srcChain)
	})
}

func Test_E2E_TestnetPrintBalances(t *testing.T) {
	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	apex, err := cardanofw.SetupRemoteApexBridge(t, cardanofw.GetTestnetApexBridgeConfig())
	require.NoError(t, err)

	balances := getUserLovelaceBalances(ctx, apex, apex.Users)
	printUserBalances(apex, apex.Users, balances)
}

func printUserBalances(apex *cardanofw.ApexSystem, users []*cardanofw.TestApexUser, balances map[string]*big.Int) {
	allUsers := append([]*cardanofw.TestApexUser{apex.FunderUser}, users...)

	for i, user := range allUsers {
		fmt.Printf("=============================\n")
		fmt.Printf("user: %d\n", i)

		chains := getEnabledChains(apex)

		for _, chain := range chains {
			var (
				addr       = user.GetAddress(chain)
				balanceStr = "No data"
			)

			if balance, exists := balances[addr]; exists {
				balanceStr = balance.String()
			}

			fmt.Printf("%s addr: %s, balance: %s\n", chain, addr, balanceStr)
		}

		fmt.Printf("=============================\n")
	}
}

func getUserLovelaceBalances(
	ctx context.Context, apex *cardanofw.ApexSystem,
	users []*cardanofw.TestApexUser,
) map[string]*big.Int {
	chains := getEnabledChains(apex)
	balances, _ := cardanofw.GetUsersBalances(ctx, apex, chains, users)
	result := make(map[string]*big.Int, len(balances))

	for addr, balances := range balances {
		result[addr] = balances[cardanowallet.AdaTokenName]
	}

	return result
}

func IsVectorEnabled(apex *cardanofw.ApexSystem) bool {
	return apex.Config.VectorConfig != nil && apex.Config.VectorConfig.IsEnabled
}

func getEnabledChains(apex *cardanofw.ApexSystem) []string {
	chains := []string{
		cardanofw.ChainIDPrime,
		cardanofw.ChainIDNexus,
	}

	if IsVectorEnabled(apex) {
		chains = append(chains, cardanofw.ChainIDVector)
	}

	return chains
}

func getEnabledDirections(apex *cardanofw.ApexSystem) []BridgingRequest {
	directions := []BridgingRequest{
		{src: cardanofw.ChainIDNexus, dest: cardanofw.ChainIDPrime},
		{src: cardanofw.ChainIDPrime, dest: cardanofw.ChainIDNexus},
	}

	if IsVectorEnabled(apex) {
		directions = append(directions, []BridgingRequest{
			{src: cardanofw.ChainIDPrime, dest: cardanofw.ChainIDVector},
			{src: cardanofw.ChainIDVector, dest: cardanofw.ChainIDPrime},
		}...)
	}

	return directions
}
