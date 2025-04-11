package e2e

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"testing"

	"github.com/0xPolygon/polygon-edge/e2e-polybft/cardanofw"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/e2ehelper"
	"github.com/Ethernal-Tech/cardano-infrastructure/sendtx"
	cardanowallet "github.com/Ethernal-Tech/cardano-infrastructure/wallet"
	"github.com/stretchr/testify/require"
)

var skylineChains = []cardanofw.ChainID{cardanofw.ChainIDPrime, cardanofw.ChainIDCardano}

func Test_E2E_SkylineTestnetFund(t *testing.T) {
	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	apex, err := cardanofw.SetupSkylineRemoteBridge(t, cardanofw.GetTestnetSkylineBridgeConfig())
	require.NoError(t, err)

	const tokensToFund = 100

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
		tokens := cardanofw.GetAllTokensForChainWithAmounts(t, apex, chain, skylineChains, tokensToFund)

		info, networkType := apex.PrimeInfo, apex.Config.PrimeConfig.NetworkType
		if chain == cardanofw.ChainIDCardano {
			info, networkType = apex.CardanoInfo, apex.Config.CardanoConfig.NetworkType
		}

		txProvider, err := info.GetTxProvider()
		require.NoError(t, err)

		funderWallet, _ := apex.FunderUser.GetCardanoWallet(chain)

		for _, user := range apex.Users {
			wg.Add(1)

			go func(user *cardanofw.TestApexUser, chain string) {
				defer wg.Done()

				receiverAddr := user.GetAddress(chain)

				fmt.Printf("Funding %s address: %s\n", chain, receiverAddr)

				_, err := cardanofw.SendTxWithTokens(
					ctx, chain, networkType, txProvider, funderWallet, receiverAddr, tokensToFund, tokens, nil)
				if err != nil {
					fmt.Printf("error while funding %s address: %s, err: %v\n", chain, receiverAddr, err)

					mu.Lock()
					addrErrs = append(addrErrs, fmt.Errorf("addr %s: %w", receiverAddr, err))
					mu.Unlock()
				}
			}(user, chain)
		}

		wg.Wait()
	}

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
		tokens := cardanofw.GetAllTokensForChainWithAmounts(t, apex, chain, skylineChains, 0)

		info, networkType := apex.PrimeInfo, apex.Config.PrimeConfig.NetworkType
		if chain == cardanofw.ChainIDCardano {
			info, networkType = apex.CardanoInfo, apex.Config.CardanoConfig.NetworkType
		}

		txProvider, err := info.GetTxProvider()
		require.NoError(t, err)

		funderReceiverAddr := apex.FunderUser.GetAddress(chain)

		for _, user := range apex.Users {
			senderWallet, senderAddr := user.GetCardanoWallet(chain)

			change := new(big.Int).SetUint64(cardanofw.MinUTxODefaultValue + cardanofw.PotentialFee)
			balanceAtleast := big.NewInt(0).Add(new(big.Int).SetUint64(cardanofw.MinUTxODefaultValue), change)

			balance, exists := balances[senderAddr.String()]
			if !exists {
				continue
			}

			lovelaceBalance := balance[cardanowallet.AdaTokenName]

			if lovelaceBalance.Cmp(balanceAtleast) != 1 {
				continue
			}

			toDefundLovelace := big.NewInt(0).Sub(lovelaceBalance, change)

			for i, token := range tokens {
				for tokenName, amount := range balance {
					if token.TokenName() == tokenName {
						tokens[i].Amount = amount.Uint64()
					}
				}
			}

			wg.Add(1)

			go func(user *cardanofw.TestApexUser, chain string) {
				defer wg.Done()

				fmt.Printf("Defunding %s address: %s\n", chain, senderAddr)

				_, err := cardanofw.SendTxWithTokens(
					ctx, chain, networkType, txProvider, senderWallet, funderReceiverAddr,
					toDefundLovelace.Uint64(), tokens, nil)
				if err != nil {
					fmt.Printf("error while funding %s address: %s, err: %v\n", chain, senderAddr, err)

					mu.Lock()
					addrErrs = append(addrErrs, fmt.Errorf("addr %s: %w", senderAddr, err))
					mu.Unlock()
				}
			}(user, chain)
		}
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

	t.Run("1. Mismatch submitted and receiver amounts", func(t *testing.T) {
		executeSkylineMismatchedAndReceivedAmounts(
			t, ctx, apex, cardanofw.ChainIDPrime, cardanofw.ChainIDCardano,
			bridgingFee, operationFee, requestStateTimeoutSec)
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
