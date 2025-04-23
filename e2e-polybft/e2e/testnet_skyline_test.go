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

			funderWallet, _ := apex.FunderUser.GetCardanoWallet(chain)

			tokens := cardanofw.GetAllTokensForChainWithAmounts(t, apex, chain, skylineChains, tokensToFund)

			info, networkType := apex.PrimeInfo, apex.Config.PrimeConfig.NetworkType
			if chain == cardanofw.ChainIDCardano {
				info, networkType = apex.CardanoInfo, apex.Config.CardanoConfig.NetworkType
			}

			txProvider, err := info.GetTxProvider()
			require.NoError(t, err)

			for _, user := range apex.Users {
				receiverAddr := user.GetAddress(chain)

				fmt.Printf("Funding %s address: %s\n", chain, receiverAddr)

				_, err := cardanofw.SendTxWithTokens(
					ctx, chain, networkType, txProvider, funderWallet, receiverAddr, tokensToFund, tokens, nil)
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
			senderWallet, senderAddr := user.GetCardanoWallet(chain)

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

				_, err := cardanofw.SendTxWithTokens(
					ctx, chain, networkType, txProvider, senderWallet, funderReceiverAddr,
					refundAmountLovelace, tokens, nil)
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
	testConfigPrime := newTestConfig(t, apex.Config.PrimeConfig, &apex.PrimeInfo, cardanofw.ChainIDCardano)
	testConfigCardano := newTestConfig(t, apex.Config.CardanoConfig, &apex.CardanoInfo, cardanofw.ChainIDPrime)
	testConfigs := []*testConfig{testConfigPrime, testConfigCardano}
	transactionTypes := map[sendtx.BridgingType]string{
		sendtx.BridgingTypeCurrencyOnSource:    "BridgingTypeCurrencyOnSource",
		sendtx.BridgingTypeNativeTokenOnSource: "BridgingTypeNativeTokenOnSource",
	}

	t.Run("Prime -> Cardano - currency on src", func(t *testing.T) {
		sendAmountDfm := big.NewInt(1_500_000)

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDPrime, cardanofw.ChainIDCardano, sendAmountDfm,
			sendtx.BridgingTypeCurrencyOnSource)
	})

	t.Run("Cardano -> Prime - native on src", func(t *testing.T) {
		sendAmountDfm := big.NewInt(1_500_000)

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDCardano, cardanofw.ChainIDPrime, sendAmountDfm,
			sendtx.BridgingTypeNativeTokenOnSource)
	})

	for _, cfg := range testConfigs {
		for txType, txTypeString := range transactionTypes {
			t.Run(fmt.Sprintf("%s -> %s sequential %s", cfg.srcChainID, cfg.dstChainID, txTypeString), func(t *testing.T) {
				const (
					sendAmount = uint64(1_000_000)
					instances  = 4
				)

				e2ehelper.ExecuteBridgingWaitAfterSubmits(
					t, ctx, apex, instances, user, cfg.srcChainID, cfg.dstChainID,
					new(big.Int).SetUint64(sendAmount), txType, bridgingOpts...)
			})
		}
	}

	for txType, txTypeString := range transactionTypes {
		t.Run(fmt.Sprintf("Both directions sequential and parallel %s multiple receivers", txTypeString), func(t *testing.T) {
			const (
				sendAmount          = uint64(1_000_000)
				sequentialInstances = 4
				parallelInstances   = 5
				receiversCnt        = 4
			)

			options := slices.Clone(bridgingOpts)
			options = append(options, e2ehelper.WithWaitForUnexpectedBridges(true))

			e2ehelper.ExecuteBridging(
				t, ctx, apex, sequentialInstances,
				apex.Users[:parallelInstances],
				apex.Users[len(apex.Users)-receiversCnt:],
				[]string{cardanofw.ChainIDPrime, cardanofw.ChainIDCardano},
				map[string][]string{
					cardanofw.ChainIDPrime:   {cardanofw.ChainIDCardano},
					cardanofw.ChainIDCardano: {cardanofw.ChainIDPrime},
				},
				txType,
				new(big.Int).SetUint64(sendAmount),
				options...)
		})
	}
}

func TestE2E_SkylineTestnetBridge_InvalidScenarios(t *testing.T) {
	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	apex, err := cardanofw.SetupSkylineRemoteBridge(t, cardanofw.GetTestnetSkylineBridgeConfig())
	require.NoError(t, err)

	const (
		requestStateTimeoutSec = 600
		bridgingFee            = uint64(1_000_010)
		operationFee           = uint64(0)
	)

	t.Run("Mismatch submitted and receiver amounts", func(t *testing.T) {
		executeInvalidMismatchSendLovelaceAmount(
			t, ctx, apex, cardanofw.ChainIDPrime, cardanofw.ChainIDCardano, bridgingFee, operationFee, requestStateTimeoutSec)
	})

	t.Run("Submitted invalid metadata - wrong type", func(t *testing.T) {
		executeInvalidMetadataType(
			t, ctx, apex, cardanofw.ChainIDPrime, cardanofw.ChainIDCardano, bridgingFee, operationFee, requestStateTimeoutSec)
	})

	t.Run("Submitted invalid metadata - invalid sender", func(t *testing.T) {
		executeInvalidMetadataSender(
			t, ctx, apex, cardanofw.ChainIDPrime, cardanofw.ChainIDCardano, bridgingFee, operationFee, requestStateTimeoutSec)
	})

	t.Run("Submitted invalid metadata - invalid bridging fee", func(t *testing.T) {
		executeInvalidBridgingFee(
			t, ctx, apex, cardanofw.ChainIDPrime, cardanofw.ChainIDCardano, bridgingFee, operationFee, requestStateTimeoutSec)
	})

	t.Run("Submitted invalid metadata - empty receivers", func(t *testing.T) {
		executeInvalidEmptyReceivers(
			t, ctx, apex, cardanofw.ChainIDPrime, cardanofw.ChainIDCardano, bridgingFee, operationFee, requestStateTimeoutSec)
	})

	t.Run("Submitted invalid metadata - invalid destination", func(t *testing.T) {
		executeInvalidDestination(
			t, ctx, apex, cardanofw.ChainIDPrime, cardanofw.ChainIDCardano, bridgingFee, operationFee, requestStateTimeoutSec)
	})

	t.Run("Submitted invalid metadata - invalid fee receiver address - token on source", func(t *testing.T) {
		executeInvalidFeeReceiverAddr(
			t, ctx, apex, cardanofw.ChainIDPrime, cardanofw.ChainIDCardano, bridgingFee, operationFee, requestStateTimeoutSec)
	})

	t.Run("Submitted with unknown tokens to bridging addr", func(t *testing.T) {
		srcChain, dstChain := cardanofw.ChainIDPrime, cardanofw.ChainIDCardano
		sendAmount := uint64(1_500_000)
		user := apex.Users[len(apex.Users)-1]

		srcInfo := apex.GetCardanoInfo(srcChain)
		txProviderSrc, err := srcInfo.GetTxProvider()
		require.NoError(t, err)

		networkTypeSrc := apex.Config.PrimeConfig.NetworkType
		if srcChain == cardanofw.ChainIDCardano {
			networkTypeSrc = apex.Config.CardanoConfig.NetworkType
		}

		minterWallet, _ := user.GetCardanoWallet(srcChain)

		tokensFunded, err := cardanofw.FundUserWithToken(
			ctx, srcChain, networkTypeSrc, txProviderSrc,
			minterWallet, user, uint64(1_500_000), uint64(1_000_000))
		require.NoError(t, err)

		executeInvalidSendUnknownToken(
			t, ctx, apex, user, srcChain, dstChain,
			bridgingFee, operationFee, sendAmount, *tokensFunded, requestStateTimeoutSec)
	})

	t.Run("Submitted invalid metadata - invalid send amount - token on source", func(t *testing.T) {
		srcChain, dstChain := cardanofw.ChainIDPrime, cardanofw.ChainIDCardano

		token, err := cardanowallet.NewTokenWithFullNameTry(apex.GetTokenNameForChains(srcChain, dstChain))
		require.NoError(t, err)

		tokenAmount := &cardanowallet.TokenAmount{
			Amount: 1_123_000,
			Token:  token,
		}

		executeInvalidMismatchSendNativeTokenAmount(
			t, ctx, apex, apex.Users[len(apex.Users)-1], srcChain, dstChain,
			bridgingFee, operationFee, *tokenAmount, requestStateTimeoutSec)
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
