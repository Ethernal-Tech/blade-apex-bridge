package e2e

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"testing"

	"slices"

	"github.com/0xPolygon/polygon-edge/e2e-polybft/cardanofw"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/e2ehelper"
	"github.com/Ethernal-Tech/cardano-infrastructure/common"
	"github.com/Ethernal-Tech/cardano-infrastructure/sendtx"
	cardanowallet "github.com/Ethernal-Tech/cardano-infrastructure/wallet"
	"github.com/stretchr/testify/require"
)

var skylineChains = []cardanofw.ChainID{cardanofw.ChainIDPrime, cardanofw.ChainIDCardano}

func Test_E2E_SkylineTestnetFund(t *testing.T) {
	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	tokensToFundBigInt := cardanofw.ApexToDfm(big.NewInt(100))
	tokensToFund := tokensToFundBigInt.Uint64()

	apex, err := cardanofw.SetupSkylineRemoteBridge(t, cardanofw.GetTestnetSkylineBridgeConfig())
	require.NoError(t, err)

	require.NotNil(t, apex.FunderUser)

	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		addrErrs []error
	)

	balances, _ := cardanofw.GetUsersBalances(ctx, apex, skylineChains, apex.Users)
	printSkylineUserBalances(t, apex, apex.Users, balances)

	fmt.Printf("funding the wallets\n")

	for _, chain := range skylineChains {
		wg.Add(1)

		go func(chain string) {
			defer wg.Done()

			tokens := cardanofw.GetAllTokensForChainWithAmounts(t, apex, chain, skylineChains, tokensToFund)

			for _, user := range apex.Users {
				receiverAddr := user.GetAddress(chain)

				fmt.Printf("Funding %s address: %s\n", chain, receiverAddr)

				_, err := apex.SubmitTx(ctx, chain, apex.FunderUser, receiverAddr,
					tokensToFundBigInt, tokens, nil)
				if err != nil {
					mu.Lock()
					addrErrs = append(addrErrs, fmt.Errorf("error while funding %s addr %s: %w", chain, receiverAddr, err))
					mu.Unlock()
				}
			}
		}(chain)
	}

	wg.Wait()

	require.NoError(t, errors.Join(addrErrs...))

	balances, _ = cardanofw.GetUsersBalances(ctx, apex, skylineChains, apex.Users)
	printSkylineUserBalances(t, apex, apex.Users, balances)

	fmt.Printf("done\n")
}

func Test_E2E_SkylineTestnetDefund(t *testing.T) {
	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	apex, err := cardanofw.SetupSkylineRemoteBridge(t, cardanofw.GetTestnetSkylineBridgeConfig())
	require.NoError(t, err)

	require.NotNil(t, apex.FunderUser)

	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		addrErrs []error
	)

	balances, _ := cardanofw.GetUsersBalances(ctx, apex, skylineChains, apex.Users)
	printSkylineUserBalances(t, apex, apex.Users, balances)

	fmt.Printf("defunding the wallets\n")

	for _, chain := range skylineChains {
		info, networkType := apex.PrimeInfo, apex.Config.PrimeConfig.NetworkType
		if chain == cardanofw.ChainIDCardano {
			info, networkType = apex.CardanoInfo, apex.Config.CardanoConfig.NetworkType
		}

		txProvider, err := info.GetTxProvider()
		require.NoError(t, err)

		protParams, err := common.ExecuteWithRetry(ctx, func(ctx context.Context) ([]byte, error) {
			return txProvider.GetProtocolParameters(ctx)
		})
		require.NoError(t, err)

		funderReceiverAddr := apex.FunderUser.GetAddress(chain)

		txBuilder, err := cardanowallet.NewTxBuilder(cardanowallet.ResolveCardanoCliBinary(networkType))
		require.NoError(t, err)

		for _, user := range apex.Users {
			_, senderAddr := user.GetCardanoWallet(chain)

			balanceBigInt, exists := balances[senderAddr.String()]
			if !exists {
				continue
			}

			balance := make(map[string]uint64, len(balanceBigInt))
			for tokenName, amount := range balanceBigInt {
				balance[tokenName] = amount.Uint64()
			}

			// bring back all tokens from user to funderReceiverAddr
			tokens, err := cardanowallet.GetTokensFromSumMap(balance)
			require.NoError(t, err)

			receiverMinUtxo, err := txBuilder.SetProtocolParameters(protParams).CalculateMinUtxo(cardanowallet.TxOutput{
				Addr:   senderAddr.String(),
				Tokens: tokens,
			})
			require.NoError(t, err)

			changePlusPotentialFee := cardanofw.MinUTxODefaultValue + cardanofw.PotentialFee
			balanceAtLeast := receiverMinUtxo + changePlusPotentialFee

			lovelaceBalance := balance[cardanowallet.AdaTokenName]
			if lovelaceBalance < balanceAtLeast {
				continue
			}

			refundAmountLovelace := lovelaceBalance - changePlusPotentialFee

			wg.Add(1)

			go func(user *cardanofw.TestApexUser, chain string) {
				defer wg.Done()

				fmt.Printf("Defunding %s address: %s\n", chain, senderAddr)

				_, err := apex.SubmitTx(ctx, chain, user, funderReceiverAddr,
					new(big.Int).SetUint64(refundAmountLovelace), tokens, nil)

				if err != nil {
					mu.Lock()
					addrErrs = append(addrErrs, fmt.Errorf("error while defunding addr %s: %w", senderAddr, err))
					mu.Unlock()
				}
			}(user, chain)
		}

		txBuilder.Dispose()
	}

	wg.Wait()

	require.NoError(t, errors.Join(addrErrs...))

	balances, _ = cardanofw.GetUsersBalances(ctx, apex, skylineChains, apex.Users)
	printSkylineUserBalances(t, apex, apex.Users, balances)

	fmt.Printf("done\n")
}

func Test_E2E_SkylineSanityCheck(t *testing.T) {
	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	apex, err := cardanofw.SetupSkylineRemoteBridge(t, cardanofw.GetTestnetSkylineBridgeConfig())
	require.NoError(t, err)

	user := apex.Users[0]
	sendAmount := cardanofw.ApexToDfm(big.NewInt(1))
	bridgingRequests := []struct {
		src  string
		dest string
	}{
		{src: cardanofw.ChainIDPrime, dest: cardanofw.ChainIDCardano},
		{src: cardanofw.ChainIDCardano, dest: cardanofw.ChainIDPrime},
	}

	for _, dir := range bridgingRequests {
		fmt.Printf("bridging from %s to %s native token on source\n", dir.src, dir.dest)

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, dir.src, dir.dest, sendAmount, sendtx.BridgingTypeNativeTokenOnSource, bridgingOpts...)

		fmt.Printf("bridging from %s to %s currency on source\n", dir.src, dir.dest)

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, dir.src, dir.dest, sendAmount, sendtx.BridgingTypeCurrencyOnSource, bridgingOpts...)
	}
}

func TestE2E_SkylineTestnetBridge_ValidScenarios(t *testing.T) {
	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	apex, err := cardanofw.SetupSkylineRemoteBridge(t, cardanofw.GetTestnetSkylineBridgeConfig())
	require.NoError(t, err)

	user := apex.Users[0]
	sendAmountDfm := big.NewInt(1_050_000)

	const numOfInstanceForSequentialTests = 3

	t.Run("Prime -> Cardano - currency on src", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDPrime, cardanofw.ChainIDCardano,
			sendAmountDfm, sendtx.BridgingTypeCurrencyOnSource)
	})

	t.Run("Cardano -> Prime - native on src", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDCardano, cardanofw.ChainIDPrime,
			sendAmountDfm, sendtx.BridgingTypeNativeTokenOnSource)
	})

	t.Run("Prime -> Cardano sequential currency on source", func(t *testing.T) {
		e2ehelper.ExecuteBridgingWaitAfterSubmits(
			t, ctx, apex, numOfInstanceForSequentialTests, user,
			cardanofw.ChainIDPrime, cardanofw.ChainIDCardano,
			sendAmountDfm, sendtx.BridgingTypeCurrencyOnSource, bridgingOpts...)
	})

	t.Run("Prime -> Cardano sequential native token on source", func(t *testing.T) {
		e2ehelper.ExecuteBridgingWaitAfterSubmits(
			t, ctx, apex, numOfInstanceForSequentialTests, user,
			cardanofw.ChainIDPrime, cardanofw.ChainIDCardano,
			sendAmountDfm, sendtx.BridgingTypeNativeTokenOnSource, bridgingOpts...)
	})

	t.Run("Cardano -> Prime sequential currency on source", func(t *testing.T) {
		e2ehelper.ExecuteBridgingWaitAfterSubmits(
			t, ctx, apex, numOfInstanceForSequentialTests, user,
			cardanofw.ChainIDCardano, cardanofw.ChainIDPrime,
			sendAmountDfm, sendtx.BridgingTypeCurrencyOnSource, bridgingOpts...)
	})

	t.Run("Cardano -> Prime sequential native token on source", func(t *testing.T) {
		e2ehelper.ExecuteBridgingWaitAfterSubmits(
			t, ctx, apex, numOfInstanceForSequentialTests, user,
			cardanofw.ChainIDCardano, cardanofw.ChainIDPrime,
			sendAmountDfm, sendtx.BridgingTypeNativeTokenOnSource, bridgingOpts...)
	})

	executeBothDirectionsMulReceiversTest := func(t *testing.T, txType sendtx.BridgingType) {
		t.Helper()

		const (
			sequentialInstances = 2
			parallelInstances   = 3
			receiversCnt        = 2
		)

		options := append(slices.Clone(bridgingOpts), e2ehelper.WithWaitForUnexpectedBridges(true))
		senders := apex.Users[:parallelInstances]
		receivers := apex.Users[len(apex.Users)-receiversCnt:]

		e2ehelper.ExecuteBridging(
			t, ctx, apex, sequentialInstances, senders, receivers,
			[]string{cardanofw.ChainIDPrime, cardanofw.ChainIDCardano},
			map[string][]string{
				cardanofw.ChainIDPrime:   {cardanofw.ChainIDCardano},
				cardanofw.ChainIDCardano: {cardanofw.ChainIDPrime},
			},
			txType, sendAmountDfm, options...)
	}

	t.Run("Both directions sequential and parallel multiple receivers currency on source", func(t *testing.T) {
		executeBothDirectionsMulReceiversTest(t, sendtx.BridgingTypeCurrencyOnSource)
	})

	t.Run("Both directions sequential and parallel multiple receivers native token on source", func(t *testing.T) {
		executeBothDirectionsMulReceiversTest(t, sendtx.BridgingTypeNativeTokenOnSource)
	})
}

func TestE2E_SkylineTestnetBridge_InvalidScenarios(t *testing.T) {
	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	apex, err := cardanofw.SetupSkylineRemoteBridge(t, cardanofw.GetTestnetSkylineBridgeConfig())
	require.NoError(t, err)

	const (
		requestStateTimeoutSec = 1500
		bridgingFee            = uint64(1_000_010)
		operationFee           = uint64(0)
	)

	primeTestConfig := newTestConfig(t, apex.Config.PrimeConfig, &apex.PrimeInfo, cardanofw.ChainIDCardano, bridgingFee, operationFee, "")
	bridgingType := sendtx.BridgingTypeCurrencyOnSource

	t.Run("1. Mismatch submitted and receiver amounts", func(t *testing.T) {
		executeInvalidMismatchSendLovelaceAmount(
			t, ctx, apex, primeTestConfig, apex.Users[0], requestStateTimeoutSec, bridgingType, true)
	})

	t.Run("2. Submitted invalid metadata - invalid bridging fee", func(t *testing.T) {
		executeInvalidBridgingFee(
			t, ctx, apex, primeTestConfig, requestStateTimeoutSec, bridgingType, true)
	})

	t.Run("3. Submitted invalid metadata - empty receivers", func(t *testing.T) {
		executeInvalidEmptyReceivers(
			t, ctx, apex, primeTestConfig, apex.Users[2], requestStateTimeoutSec, bridgingType, true)
	})

	t.Run("4. Submitted invalid metadata - wrong type", func(t *testing.T) {
		executeInvalidMetadataType(
			t, ctx, apex, primeTestConfig, apex.Users[1], requestStateTimeoutSec, bridgingType, true)
	})

	t.Run("5. Submitted invalid metadata - invalid destination", func(t *testing.T) {
		executeInvalidDestination(
			t, ctx, apex, primeTestConfig, apex.Users[3], requestStateTimeoutSec, bridgingType, true)
	})

	t.Run("6. Submitted invalid metadata - invalid sender", func(t *testing.T) {
		executeInvalidMetadataInvalidSender(
			t, ctx, apex, primeTestConfig, apex.Users[1], requestStateTimeoutSec, bridgingType)
	})

	t.Run("7. Submitted invalid metadata - invalid fee receiver address - token on source", func(t *testing.T) {
		executeInvalidFeeReceiverAddr(
			t, ctx, apex, primeTestConfig, requestStateTimeoutSec, bridgingType, true)
	})

	t.Run("8. Submitted with unknown tokens to bridging addr", func(t *testing.T) {
		user := apex.Users[len(apex.Users)-1]

		tokensFunded, err := cardanofw.FundUserWithToken(
			ctx, apex, cardanofw.ChainIDPrime,
			primeTestConfig.srcMinterWallet, user,
			cardanofw.DefaultTokenName, cardanofw.DefaultTokenMintAmount,
			uint64(1_500_000), uint64(1_000_000))
		require.NoError(t, err)

		executeInvalidSendUnknownToken(t, ctx, apex, user, primeTestConfig, *tokensFunded, requestStateTimeoutSec, true)
	})

	t.Run("9. Submitted invalid metadata - invalid send amount - token on source", func(t *testing.T) {
		executeInvalidMismatchSendNativeTokenAmount(t, ctx, apex, apex.Users[len(apex.Users)-1], primeTestConfig, requestStateTimeoutSec, true)
	})
}

func Test_E2E_SkylineTestnetPrintBalances(t *testing.T) {
	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	apex, err := cardanofw.SetupSkylineRemoteBridge(t, cardanofw.GetTestnetSkylineBridgeConfig())
	require.NoError(t, err)

	balances, _ := cardanofw.GetUsersBalances(ctx, apex, skylineChains, apex.Users)
	printSkylineUserBalances(t, apex, apex.Users, balances)
}

func printSkylineUserBalances(
	t *testing.T, apex *cardanofw.ApexSystem, users []*cardanofw.TestApexUser, balances map[string]map[string]*big.Int,
) {
	t.Helper()

	chainTokensMap := map[cardanofw.ChainID][]string{}

	for _, chain := range skylineChains {
		chainTokensMap[chain] = append(chainTokensMap[chain], cardanowallet.AdaTokenName)

		for _, token := range cardanofw.GetAllTokensForChainWithAmounts(t, apex, chain, skylineChains, 0) {
			chainTokensMap[chain] = append(chainTokensMap[chain], token.TokenName())
		}
	}

	allUsers := append([]*cardanofw.TestApexUser{apex.FunderUser}, users...)

	for i, user := range allUsers {
		fmt.Printf("=============================\n")
		fmt.Printf("user: %d\n", i)

		for _, chain := range skylineChains {
			addr := user.GetAddress(chain)

			if balance, exists := balances[addr]; !exists {
				fmt.Printf("%s addr: %s, balance: No data\n", chain, addr)
			} else {
				fmt.Printf("%s addr: %s\n", chain, addr)

				for _, tokenName := range chainTokensMap[chain] {
					fmt.Printf("  %s = %s\n", tokenName, balance[tokenName])
				}
			}
		}

		fmt.Printf("=============================\n")
	}
}
