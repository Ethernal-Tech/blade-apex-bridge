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
	cardanowallet "github.com/Ethernal-Tech/cardano-infrastructure/wallet"
	"github.com/stretchr/testify/require"
)

var skylineChains = []cardanofw.ChainID{cardanofw.ChainIDPrime, cardanofw.ChainIDVector, cardanofw.ChainIDCardano, cardanofw.ChainIDNexus}
var fundableTokensPerChain = map[cardanofw.ChainID][]uint16{
	cardanofw.ChainIDPrime:   {},
	cardanofw.ChainIDVector:  {cardanofw.XADATokenID},
	cardanofw.ChainIDCardano: {cardanofw.CAP3XTokenID},
	cardanofw.ChainIDNexus:   {cardanofw.USDTTokenID},
}

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

			tokens := func() []cardanowallet.TokenAmount {
				if chain == cardanofw.ChainIDNexus {
					chainInfo := apex.GetEvmInfo(chain)
					tokens := make([]cardanowallet.TokenAmount, len(fundableTokensPerChain[chain]))

					for i, tokenID := range fundableTokensPerChain[chain] {
						tokens[i] = cardanowallet.TokenAmount{
							Token:  cardanowallet.Token{PolicyID: chainInfo.Tokens[tokenID].ChainSpecific},
							Amount: tokensToFund,
						}
					}

					return tokens
				}

				chainInfo := apex.GetCardanoInfo(chain)
				tokens := make([]cardanowallet.TokenAmount, len(fundableTokensPerChain[chain]))

				for i, tokenID := range fundableTokensPerChain[chain] {
					token, err := cardanowallet.NewTokenWithFullNameTry(chainInfo.Tokens[tokenID].ChainSpecific)
					require.NoError(t, err)

					tokens[i] = cardanowallet.TokenAmount{
						Token:  token,
						Amount: tokensToFund,
					}
				}

				return tokens
			}()

			for _, user := range apex.Users {
				receiverAddr := user.GetAddress(chain)

				fmt.Printf("Funding %s address: %s\n", chain, receiverAddr)

				// resubmit the transaction in case of error because of a possible rollback
				_, err := common.ExecuteWithRetry(ctx, func(ctx context.Context) (string, error) {
					txHash, err := apex.SubmitTx(ctx, chain, apex.FunderUser, receiverAddr,
						tokensToFundBigInt, tokens, nil)
					if errors.Is(err, common.ErrRetryTimeout) {
						return "", common.ErrRetryTryAgain
					}

					return txHash, err
				})
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
		if chain == cardanofw.ChainIDNexus {
			info := apex.NexusInfo

			for _, user := range apex.Users {
				balance, err := apex.GetBalance(ctx, user, chain)
				require.NoError(t, err)

				for _, token := range info.Tokens {
					tokenBalance, err := apex.GetBalanceWithTokenName(ctx, user, chain, token.ChainSpecific)
					require.NoError(t, err)

					if tokenBalance[token.ChainSpecific].Cmp(big.NewInt(0)) > 0 {
						balance[token.ChainSpecific] = tokenBalance[token.ChainSpecific]
					}
				}

				// 1. Compute change in DFM (PotentialFee is already in DFM units)
				changeDfm := new(big.Int).Mul(
					new(big.Int).SetUint64(cardanofw.PotentialFee),
					new(big.Int).SetUint64(uint64(len(balance))),
				)

				if balance[cardanowallet.AdaTokenName].Cmp(changeDfm) <= 0 {
					continue
				}

				// 2. Refund amount in DFM
				refundAmountDfm := new(big.Int).Sub(balance[cardanowallet.AdaTokenName], changeDfm)

				tokens := make([]cardanowallet.TokenAmount, 0, len(balance)-1)

				// 3. Token refunds
				for token, amount := range balance {
					if token == cardanowallet.AdaTokenName {
						continue
					}

					tokens = append(tokens, cardanowallet.TokenAmount{
						Token:  cardanowallet.Token{PolicyID: token},
						Amount: amount.Uint64(),
					})
				}

				wg.Add(1)

				go func(user *cardanofw.TestApexUser, chain string) {
					defer wg.Done()

					fmt.Printf("Defunding %s address: %s\n", chain, user.GetAddress(chain))

					_, err := apex.SubmitTx(ctx, chain, user, apex.FunderUser.GetAddress(chain),
						refundAmountDfm, tokens, nil)

					if err != nil {
						mu.Lock()
						addrErrs = append(addrErrs, fmt.Errorf("error while defunding addr %s: %w", user.GetAddress(chain), err))
						mu.Unlock()
					}
				}(user, chain)
			}

			continue
		}

		info, networkType := apex.PrimeInfo, apex.Config.PrimeConfig.NetworkType
		if chain == cardanofw.ChainIDCardano {
			info, networkType = apex.CardanoInfo, apex.Config.CardanoConfig.NetworkType
		}

		if chain == cardanofw.ChainIDVector {
			info, networkType = apex.VectorInfo, apex.Config.VectorConfig.NetworkType
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

			receiverMinUtxo, err := txBuilder.SetProtocolParameters(protParams).CalculateMinUtxo(cardanowallet.TxOutputWithRefScript{
				TxOutput: cardanowallet.TxOutput{
					Addr:   senderAddr.String(),
					Tokens: tokens,
				},
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
		src          string
		dest         string
		bridgingType cardanofw.BridgingType
		tokenID      uint16
	}{
		{src: cardanofw.ChainIDPrime, dest: cardanofw.ChainIDCardano, bridgingType: cardanofw.BridgingTypeCurrencyOnSource, tokenID: cardanofw.CAP3XTokenID},
		{src: cardanofw.ChainIDCardano, dest: cardanofw.ChainIDPrime, bridgingType: cardanofw.BridgingTypeWrappedTokenOnSource, tokenID: cardanofw.ADATokenID},
		{src: cardanofw.ChainIDVector, dest: cardanofw.ChainIDCardano, bridgingType: cardanofw.BridgingTypeWrappedTokenOnSource, tokenID: cardanofw.XADATokenID},
		{src: cardanofw.ChainIDCardano, dest: cardanofw.ChainIDVector, bridgingType: cardanofw.BridgingTypeCurrencyOnSource, tokenID: cardanofw.ADATokenID},
		//// order is important here
		{src: cardanofw.ChainIDCardano, dest: cardanofw.ChainIDNexus, bridgingType: cardanofw.BridgingTypeCurrencyOnSource, tokenID: cardanofw.ADATokenID},
		{src: cardanofw.ChainIDNexus, dest: cardanofw.ChainIDCardano, bridgingType: cardanofw.BridgingTypeColoredCoinOnSource, tokenID: cardanofw.XADATokenID},
		{src: cardanofw.ChainIDVector, dest: cardanofw.ChainIDNexus, bridgingType: cardanofw.BridgingTypeColoredCoinOnSource, tokenID: cardanofw.XADATokenID},
		{src: cardanofw.ChainIDNexus, dest: cardanofw.ChainIDVector, bridgingType: cardanofw.BridgingTypeColoredCoinOnSource, tokenID: cardanofw.XADATokenID},
		{src: cardanofw.ChainIDNexus, dest: cardanofw.ChainIDVector, bridgingType: cardanofw.BridgingTypeColoredCoinOnSource, tokenID: cardanofw.USDTTokenID},
		{src: cardanofw.ChainIDVector, dest: cardanofw.ChainIDNexus, bridgingType: cardanofw.BridgingTypeColoredCoinOnSource, tokenID: cardanofw.USDTTokenID},
	}

	for _, dir := range bridgingRequests {
		fmt.Printf("bridging from %s to %s, %s\n", dir.src, dir.dest, dir.bridgingType.String())

		options := append([]e2ehelper.ExecuteBridgingOption{}, bridgingOpts...)
		options = append(options, e2ehelper.WithColoredCoins([]uint16{dir.tokenID}))

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, dir.src, dir.dest, sendAmount, dir.bridgingType, options...)
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
			sendAmountDfm, cardanofw.BridgingTypeCurrencyOnSource)
	})

	t.Run("Cardano -> Prime - native token on src", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDCardano, cardanofw.ChainIDPrime,
			sendAmountDfm, cardanofw.BridgingTypeWrappedTokenOnSource)
	})

	t.Run("Prime -> Cardano sequential currency on source", func(t *testing.T) {
		e2ehelper.ExecuteBridgingWaitAfterSubmits(
			t, ctx, apex, numOfInstanceForSequentialTests, user,
			cardanofw.ChainIDPrime, cardanofw.ChainIDCardano,
			sendAmountDfm, cardanofw.BridgingTypeCurrencyOnSource, bridgingOpts...)
	})

	t.Run("Vector -> Cardano sequential native token on source", func(t *testing.T) {
		e2ehelper.ExecuteBridgingWaitAfterSubmits(
			t, ctx, apex, numOfInstanceForSequentialTests, user,
			cardanofw.ChainIDVector, cardanofw.ChainIDCardano,
			sendAmountDfm, cardanofw.BridgingTypeWrappedTokenOnSource, bridgingOpts...)
	})

	t.Run("Cardano -> Vector sequential currency on source", func(t *testing.T) {
		e2ehelper.ExecuteBridgingWaitAfterSubmits(
			t, ctx, apex, numOfInstanceForSequentialTests, user,
			cardanofw.ChainIDCardano, cardanofw.ChainIDVector,
			sendAmountDfm, cardanofw.BridgingTypeCurrencyOnSource, bridgingOpts...)
	})

	t.Run("Cardano -> Prime sequential native token on source", func(t *testing.T) {
		e2ehelper.ExecuteBridgingWaitAfterSubmits(
			t, ctx, apex, numOfInstanceForSequentialTests, user,
			cardanofw.ChainIDCardano, cardanofw.ChainIDPrime,
			sendAmountDfm, cardanofw.BridgingTypeWrappedTokenOnSource, bridgingOpts...)
	})

	executeAllDirectionsMulReceiversTest := func(t *testing.T, chainsDst map[string][]string, txTypes map[e2ehelper.SrcDstChainPair]cardanofw.BridgingType) {
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
			chainsDst,
			txTypes,
			sendAmountDfm, options...)
	}

	t.Run("Both directions sequential and parallel multiple receivers currency on source", func(t *testing.T) {
		executeAllDirectionsMulReceiversTest(t, map[string][]string{
			cardanofw.ChainIDPrime:   {cardanofw.ChainIDCardano},
			cardanofw.ChainIDCardano: {cardanofw.ChainIDVector},
		}, map[e2ehelper.SrcDstChainPair]cardanofw.BridgingType{
			e2ehelper.NewChainPair(cardanofw.ChainIDPrime, cardanofw.ChainIDCardano):  cardanofw.BridgingTypeCurrencyOnSource,
			e2ehelper.NewChainPair(cardanofw.ChainIDCardano, cardanofw.ChainIDVector): cardanofw.BridgingTypeCurrencyOnSource,
		})
	})

	t.Run("Both directions sequential and parallel multiple receivers with cardano as a source", func(t *testing.T) {
		executeAllDirectionsMulReceiversTest(t, map[string][]string{
			cardanofw.ChainIDCardano: {cardanofw.ChainIDPrime, cardanofw.ChainIDVector},
		}, map[e2ehelper.SrcDstChainPair]cardanofw.BridgingType{
			e2ehelper.NewChainPair(cardanofw.ChainIDCardano, cardanofw.ChainIDPrime):  cardanofw.BridgingTypeWrappedTokenOnSource,
			e2ehelper.NewChainPair(cardanofw.ChainIDCardano, cardanofw.ChainIDVector): cardanofw.BridgingTypeCurrencyOnSource,
		})
	})
}

func TestE2E_SkylineTestnetBridge_ValidScenarios_ColoredCoins(t *testing.T) {
	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	apex, err := cardanofw.SetupSkylineRemoteBridge(t, cardanofw.GetTestnetSkylineBridgeConfig())
	require.NoError(t, err)

	user := apex.Users[6]
	sendAmountDfm := big.NewInt(1_050_000)

	const numOfInstanceForSequentialTests = 3

	t.Run("1. Cardano -> Vector -> Nexus -> Cardano - ADA/xADA", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDCardano, cardanofw.ChainIDVector, sendAmountDfm,
			cardanofw.BridgingTypeCurrencyOnSource)

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDVector, cardanofw.ChainIDNexus, sendAmountDfm,
			cardanofw.BridgingTypeWrappedTokenOnSource)

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDNexus, cardanofw.ChainIDCardano, sendAmountDfm,
			cardanofw.BridgingTypeColoredCoinOnSource, e2ehelper.WithColoredCoins([]uint16{cardanofw.XADATokenID}))
	})

	t.Run("2. Cardano -> Nexus -> Vector -> Cardano - ADA/xADA", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDCardano, cardanofw.ChainIDNexus, sendAmountDfm,
			cardanofw.BridgingTypeCurrencyOnSource)

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDNexus, cardanofw.ChainIDVector, sendAmountDfm,
			cardanofw.BridgingTypeColoredCoinOnSource, e2ehelper.WithColoredCoins([]uint16{cardanofw.XADATokenID}))

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDVector, cardanofw.ChainIDCardano, sendAmountDfm,
			cardanofw.BridgingTypeWrappedTokenOnSource)
	})

	t.Run("3. Nexus -> Vector sequential USDT", func(t *testing.T) {
		options := append([]e2ehelper.ExecuteBridgingOption{}, bridgingOpts...)
		options = append(options, e2ehelper.WithColoredCoins([]uint16{cardanofw.USDTTokenID}))

		e2ehelper.ExecuteBridgingWaitAfterSubmits(
			t, ctx, apex, numOfInstanceForSequentialTests, user,
			cardanofw.ChainIDNexus, cardanofw.ChainIDVector,
			sendAmountDfm, cardanofw.BridgingTypeColoredCoinOnSource, options...)
	})

	t.Run("4. Vector -> Nexus sequential USDT", func(t *testing.T) {
		options := append([]e2ehelper.ExecuteBridgingOption{}, bridgingOpts...)
		options = append(options, e2ehelper.WithColoredCoins([]uint16{cardanofw.USDTTokenID}))

		e2ehelper.ExecuteBridgingWaitAfterSubmits(
			t, ctx, apex, numOfInstanceForSequentialTests, user,
			cardanofw.ChainIDVector, cardanofw.ChainIDNexus,
			sendAmountDfm, cardanofw.BridgingTypeColoredCoinOnSource, options...)
	})

	t.Run("5. Cardano -> Nexus sequential ADA", func(t *testing.T) {
		e2ehelper.ExecuteBridgingWaitAfterSubmits(
			t, ctx, apex, numOfInstanceForSequentialTests, user,
			cardanofw.ChainIDCardano, cardanofw.ChainIDNexus,
			sendAmountDfm, cardanofw.BridgingTypeCurrencyOnSource, bridgingOpts...)
	})

	t.Run("6. Nexus -> Cardano sequential xADA", func(t *testing.T) {
		options := append([]e2ehelper.ExecuteBridgingOption{}, bridgingOpts...)
		options = append(options, e2ehelper.WithColoredCoins([]uint16{cardanofw.XADATokenID}))

		e2ehelper.ExecuteBridgingWaitAfterSubmits(
			t, ctx, apex, numOfInstanceForSequentialTests, user,
			cardanofw.ChainIDNexus, cardanofw.ChainIDCardano,
			sendAmountDfm, cardanofw.BridgingTypeColoredCoinOnSource, options...)
	})

	fundingSuccessfull := t.Run("7. Vector -> Nexus sequential xADA", func(t *testing.T) {
		options := append([]e2ehelper.ExecuteBridgingOption{}, bridgingOpts...)
		options = append(options, e2ehelper.WithColoredCoins([]uint16{cardanofw.XADATokenID}))

		e2ehelper.ExecuteBridgingWaitAfterSubmits(
			t, ctx, apex, numOfInstanceForSequentialTests, user,
			cardanofw.ChainIDVector, cardanofw.ChainIDNexus,
			sendAmountDfm, cardanofw.BridgingTypeColoredCoinOnSource, options...)
	})

	t.Run("8. Nexus -> Vector and Cardano -> Vector in parallel - sequential xADA", func(t *testing.T) {
		if !fundingSuccessfull {
			t.Skip()
		}

		bridgingDirections := []e2ehelper.ExecuteBridgingConfig{
			{SrcChain: cardanofw.ChainIDNexus, DstChain: cardanofw.ChainIDVector, BridgingType: cardanofw.BridgingTypeColoredCoinOnSource, TokenID: cardanofw.XADATokenID, SendAmountDfm: sendAmountDfm},
			{SrcChain: cardanofw.ChainIDCardano, DstChain: cardanofw.ChainIDVector, BridgingType: cardanofw.BridgingTypeCurrencyOnSource, SendAmountDfm: sendAmountDfm},
		}

		e2ehelper.ExecuteBridgingWaitAfterSubmitsExtended(
			t, ctx, apex, numOfInstanceForSequentialTests, user,
			bridgingDirections,
			bridgingOpts...)

		returnAmountDfm := new(big.Int).Mul(sendAmountDfm, big.NewInt(int64(numOfInstanceForSequentialTests*len(bridgingDirections))))

		// return all the xADA to Cardano
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDVector, cardanofw.ChainIDCardano,
			returnAmountDfm, cardanofw.BridgingTypeWrappedTokenOnSource, bridgingOpts...)
	})

	t.Run("9. Nexus -> Cardano and Vector -> Cardano in parallel - sequential xADA", func(t *testing.T) {
		bridgingDirections := []e2ehelper.ExecuteBridgingConfig{
			{SrcChain: cardanofw.ChainIDNexus, DstChain: cardanofw.ChainIDCardano, BridgingType: cardanofw.BridgingTypeColoredCoinOnSource, TokenID: cardanofw.XADATokenID, SendAmountDfm: sendAmountDfm},
			{SrcChain: cardanofw.ChainIDVector, DstChain: cardanofw.ChainIDCardano, BridgingType: cardanofw.BridgingTypeWrappedTokenOnSource, SendAmountDfm: sendAmountDfm},
		}

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDCardano, cardanofw.ChainIDVector, new(big.Int).Mul(sendAmountDfm, big.NewInt(int64(numOfInstanceForSequentialTests*len(bridgingDirections)))),
			cardanofw.BridgingTypeCurrencyOnSource)

		options := append([]e2ehelper.ExecuteBridgingOption{}, bridgingOpts...)
		options = append(options, e2ehelper.WithColoredCoins([]uint16{cardanofw.XADATokenID}))

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDVector, cardanofw.ChainIDNexus, new(big.Int).Mul(sendAmountDfm, big.NewInt(int64(numOfInstanceForSequentialTests))),
			cardanofw.BridgingTypeColoredCoinOnSource, options...)

		e2ehelper.ExecuteBridgingWaitAfterSubmitsExtended(
			t, ctx, apex, numOfInstanceForSequentialTests, user,
			bridgingDirections, bridgingOpts...)
	})

	const (
		sequentialInstances = 2
		parallelInstances   = 3
		receiversCnt        = 2
	)

	executeAllDirectionsMulReceiversTest := func(t *testing.T, bridgingDirections []e2ehelper.BridgingDirectionConfig) {
		t.Helper()

		options := append(slices.Clone(bridgingOpts), e2ehelper.WithWaitForUnexpectedBridges(true))
		senders := apex.Users[len(apex.Users)-parallelInstances:]
		receivers := apex.Users[:receiversCnt]

		e2ehelper.ExecuteBridgingExtended(
			t, ctx, apex, sequentialInstances, senders, receivers,
			bridgingDirections,
			sendAmountDfm, options...)
	}

	t.Run("10. Nexus <-> Vector USDT both directions parallel", func(t *testing.T) {
		fundAmount := new(big.Int).Mul(sendAmountDfm, big.NewInt(sequentialInstances*parallelInstances))

		options := append([]e2ehelper.ExecuteBridgingOption{}, bridgingOpts...)
		options = append(options, e2ehelper.WithColoredCoins([]uint16{cardanofw.USDTTokenID}))

		for _, user := range apex.Users[:parallelInstances] {
			e2ehelper.ExecuteSingleBridging(
				t, ctx, apex, user, user, cardanofw.ChainIDNexus, cardanofw.ChainIDVector, fundAmount,
				cardanofw.BridgingTypeColoredCoinOnSource, options...)
		}

		bridgingDirections := []e2ehelper.BridgingDirectionConfig{
			{SrcChain: cardanofw.ChainIDNexus, DstChain: cardanofw.ChainIDVector, BridgingType: cardanofw.BridgingTypeColoredCoinOnSource, TokenID: cardanofw.USDTTokenID},
			{SrcChain: cardanofw.ChainIDVector, DstChain: cardanofw.ChainIDNexus, BridgingType: cardanofw.BridgingTypeColoredCoinOnSource, TokenID: cardanofw.USDTTokenID},
		}

		executeAllDirectionsMulReceiversTest(t, bridgingDirections)
	})

	t.Run("11. Nexus <-> Vector xADA both directions parallel", func(t *testing.T) {
		fundAmount := new(big.Int).Mul(sendAmountDfm, big.NewInt(sequentialInstances*parallelInstances))

		for _, user := range apex.Users[:parallelInstances] {
			e2ehelper.ExecuteSingleBridging(
				t, ctx, apex, user, user, cardanofw.ChainIDCardano, cardanofw.ChainIDNexus, fundAmount,
				cardanofw.BridgingTypeCurrencyOnSource)
		}

		bridgingDirections := []e2ehelper.BridgingDirectionConfig{
			{SrcChain: cardanofw.ChainIDNexus, DstChain: cardanofw.ChainIDVector, BridgingType: cardanofw.BridgingTypeColoredCoinOnSource, TokenID: cardanofw.XADATokenID},
			{SrcChain: cardanofw.ChainIDVector, DstChain: cardanofw.ChainIDNexus, BridgingType: cardanofw.BridgingTypeColoredCoinOnSource, TokenID: cardanofw.XADATokenID},
		}

		executeAllDirectionsMulReceiversTest(t, bridgingDirections)
	})

	t.Run("12. Nexus <-> Cardano xADA both directions parallel", func(t *testing.T) {
		fundAmount := new(big.Int).Mul(sendAmountDfm, big.NewInt(sequentialInstances*parallelInstances))

		for _, user := range apex.Users[:parallelInstances] {
			e2ehelper.ExecuteSingleBridging(
				t, ctx, apex, user, user, cardanofw.ChainIDCardano, cardanofw.ChainIDNexus, fundAmount,
				cardanofw.BridgingTypeCurrencyOnSource)
		}

		bridgingDirections := []e2ehelper.BridgingDirectionConfig{
			{SrcChain: cardanofw.ChainIDNexus, DstChain: cardanofw.ChainIDCardano, BridgingType: cardanofw.BridgingTypeColoredCoinOnSource, TokenID: cardanofw.XADATokenID},
			{SrcChain: cardanofw.ChainIDCardano, DstChain: cardanofw.ChainIDNexus, BridgingType: cardanofw.BridgingTypeCurrencyOnSource},
		}

		executeAllDirectionsMulReceiversTest(t, bridgingDirections)
	})
}

func TestE2E_SkylineTestnetBridge_InvalidScenarios(t *testing.T) {
	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	apex, err := cardanofw.SetupSkylineRemoteBridge(t, cardanofw.GetTestnetSkylineBridgeConfig())
	require.NoError(t, err)

	const (
		requestStateTimeoutSec = 1500
		retryIntervalSec       = 5
	)

	tokensInfo := apex.GetBridgingTokensInfo(cardanofw.ChainIDVector, cardanofw.ChainIDCardano, cardanofw.BridgingTypeWrappedTokenOnSource)
	require.NotNil(t, tokensInfo)

	vectorCardanoTokenName := tokensInfo.SrcTokenName

	primeCardanoTestConfig := newTestConfig(
		t, apex.Config.PrimeConfig, &apex.PrimeInfo, cardanofw.ChainIDCardano, "")
	cardanoVectorTestConfig := newTestConfig(
		t, apex.Config.CardanoConfig, &apex.CardanoInfo, cardanofw.ChainIDVector, "")
	vectorCardanoTestConfig := newTestConfig(
		t, apex.Config.VectorConfig, &apex.VectorInfo, cardanofw.ChainIDCardano, vectorCardanoTokenName)
	bridgingType := cardanofw.BridgingTypeCurrencyOnSource

	t.Run("1. Mismatch submitted and receiver amounts", func(t *testing.T) {
		executeInvalidMismatchSendLovelaceAmount(
			t, ctx, apex, primeCardanoTestConfig, apex.Users[0], requestStateTimeoutSec, retryIntervalSec, bridgingType, true, 0)
	})

	t.Run("2. Submitted invalid metadata - invalid bridging fee", func(t *testing.T) {
		executeInvalidBridgingFee(
			t, ctx, apex, primeCardanoTestConfig, requestStateTimeoutSec, retryIntervalSec, bridgingType, true, 0)
	})

	t.Run("3. Submitted invalid metadata - empty receivers", func(t *testing.T) {
		executeInvalidEmptyReceivers(
			t, ctx, apex, primeCardanoTestConfig, apex.Users[2], requestStateTimeoutSec, retryIntervalSec, bridgingType, true, 0)
	})

	t.Run("4. Submitted invalid metadata - wrong type", func(t *testing.T) {
		executeInvalidMetadataType(
			t, ctx, apex, primeCardanoTestConfig, apex.Users[1], requestStateTimeoutSec, retryIntervalSec, bridgingType, true, 0)
	})

	t.Run("5. Submitted invalid metadata - invalid destination", func(t *testing.T) {
		executeInvalidDestination(
			t, ctx, apex, cardanoVectorTestConfig, apex.Users[3], requestStateTimeoutSec, retryIntervalSec, bridgingType, true, 0)
	})

	t.Run("6. Submitted invalid metadata - invalid sender", func(t *testing.T) {
		executeInvalidMetadataInvalidSender(
			t, ctx, apex, cardanoVectorTestConfig, apex.Users[1], requestStateTimeoutSec, bridgingType, 0)
	})

	t.Run("7. Submitted invalid metadata - invalid fee receiver address - token on source", func(t *testing.T) {
		executeInvalidFeeReceiverAddr(
			t, ctx, apex, cardanoVectorTestConfig, requestStateTimeoutSec, retryIntervalSec, bridgingType, true, 0)
	})

	t.Run("8. Submitted with unknown tokens to bridging addr", func(t *testing.T) {
		user := apex.Users[len(apex.Users)-1]

		minterWallet, _ := user.GetCardanoWallet(cardanofw.ChainIDVector)

		tokensFunded, err := cardanofw.FundUserWithToken(
			ctx, apex, cardanofw.ChainIDVector,
			minterWallet, user,
			cardanofw.DefaultTokenName, cardanofw.DefaultTokenMintAmount,
			uint64(1_500_000), uint64(1_000_000))
		require.NoError(t, err)

		executeInvalidSendNativeToken(t, ctx, apex, user, vectorCardanoTestConfig, *tokensFunded, requestStateTimeoutSec, retryIntervalSec, true, 0, cardanofw.BridgingTypeWrappedTokenOnSource)
	})

	t.Run("9. Submitted invalid metadata - invalid send amount - token on source", func(t *testing.T) {
		user := apex.Users[len(apex.Users)-1]

		tokenInfo := apex.GetBridgingTokensInfo(cardanofw.ChainIDVector, cardanofw.ChainIDCardano, cardanofw.BridgingTypeWrappedTokenOnSource)
		require.NotNil(t, tokenInfo)
		token, err := cardanowallet.NewTokenWithFullNameTry(tokenInfo.SrcTokenName)
		require.NoError(t, err)

		tokenAmount := &cardanowallet.TokenAmount{
			Amount: 1_000_000,
			Token:  token,
		}

		executeInvalidMismatchSendNativeTokenAmount(
			t, ctx, apex, user, vectorCardanoTestConfig, *tokenAmount, requestStateTimeoutSec, retryIntervalSec, true, 0)
	})
}

func TestE2E_SkylineTestnetBridge_InvalidScenarios_NexusSrc(t *testing.T) {
	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	apex, err := cardanofw.SetupSkylineRemoteBridge(t, cardanofw.GetTestnetSkylineBridgeConfig())
	require.NoError(t, err)

	user := apex.Users[5]

	tokenInfo := apex.GetBridgingTokensInfo(cardanofw.ChainIDNexus, cardanofw.ChainIDVector, cardanofw.BridgingTypeColoredCoinOnSource, cardanofw.USDTTokenID)
	require.NotNil(t, tokenInfo)

	sendAmount := cardanofw.DfmToWei(big.NewInt(1_000_000))

	//nolint:dupl
	t.Run("1. Invalid destination in bridging request", func(t *testing.T) {
		t.Run("1. Destination is Nexus", func(t *testing.T) {
			err := executeInvalidNexusBridgingRequest(t, ctx, apex, user, InvalidNexusBridgingRequest{
				dstChainID: cardanofw.ChainIDToInt(cardanofw.ChainIDNexus),
				sender:     user,
				receivers: map[string]cardanofw.ReceiverAmount{
					user.GetAddress(cardanofw.ChainIDVector): {
						TokenID: cardanofw.USDTTokenID,
						Amount:  sendAmount,
					},
				},
				operationFee: big.NewInt(0),
				tokenInfo:    tokenInfo,
			})
			require.NoError(t, err)
		})

		t.Run("2. Destination is unregistered", func(t *testing.T) {
			err := executeInvalidNexusBridgingRequest(t, ctx, apex, user, InvalidNexusBridgingRequest{
				dstChainID: 99,
				sender:     user,
				receivers: map[string]cardanofw.ReceiverAmount{
					user.GetAddress(cardanofw.ChainIDVector): {
						TokenID: cardanofw.USDTTokenID,
						Amount:  sendAmount,
					},
				},
				operationFee: big.NewInt(0),
				tokenInfo:    tokenInfo,
			})
			require.NoError(t, err)
		})
	})

	t.Run("2. Invalid destination in receiver", func(t *testing.T) {
		err := executeInvalidNexusBridgingRequest(t, ctx, apex, user, InvalidNexusBridgingRequest{
			dstChainID: cardanofw.ChainIDToInt(cardanofw.ChainIDVector),
			sender:     user,
			receivers: map[string]cardanofw.ReceiverAmount{
				user.GetAddress(cardanofw.ChainIDNexus): {
					TokenID: cardanofw.USDTTokenID,
					Amount:  sendAmount,
				},
			},
			operationFee: big.NewInt(0),
			tokenInfo:    tokenInfo,
		})
		require.NoError(t, err)
	})

	t.Run("4. 0 receivers in bridging request", func(t *testing.T) {
		err := executeInvalidNexusBridgingRequest(t, ctx, apex, user, InvalidNexusBridgingRequest{
			dstChainID:   cardanofw.ChainIDToInt(cardanofw.ChainIDVector),
			sender:       user,
			operationFee: big.NewInt(0),
			tokenInfo:    tokenInfo,
		})
		require.NoError(t, err)
	})

	t.Run("5. Too many receivers in bridging request", func(t *testing.T) {
		receivers := make(map[string]cardanofw.ReceiverAmount)
		for i := range 6 {
			receivers[apex.Users[i].GetAddress(cardanofw.ChainIDVector)] = cardanofw.ReceiverAmount{
				TokenID: cardanofw.USDTTokenID,
				Amount:  sendAmount,
			}
		}

		err := executeInvalidNexusBridgingRequest(t, ctx, apex, user, InvalidNexusBridgingRequest{
			dstChainID:   cardanofw.ChainIDToInt(cardanofw.ChainIDVector),
			sender:       user,
			receivers:    receivers,
			operationFee: big.NewInt(0),
			tokenInfo:    tokenInfo,
		})
		require.NoError(t, err)
	})

	t.Run("6. Invalid receiver address", func(t *testing.T) {
		err := executeInvalidNexusBridgingRequest(t, ctx, apex, user, InvalidNexusBridgingRequest{
			dstChainID: cardanofw.ChainIDToInt(cardanofw.ChainIDVector),
			sender:     user,
			receivers: map[string]cardanofw.ReceiverAmount{
				"addr_test1invalidaddress": {
					TokenID: cardanofw.USDTTokenID,
					Amount:  sendAmount,
				},
			},
			operationFee: big.NewInt(0),
			tokenInfo:    tokenInfo,
		})
		require.NoError(t, err)
	})

	t.Run("7. Fee address in receivers", func(t *testing.T) {
		err := executeInvalidNexusBridgingRequest(t, ctx, apex, user, InvalidNexusBridgingRequest{
			dstChainID: cardanofw.ChainIDToInt(cardanofw.ChainIDVector),
			sender:     user,
			receivers: map[string]cardanofw.ReceiverAmount{
				apex.VectorInfo.FeeAddr: {
					TokenID: cardanofw.USDTTokenID,
					Amount:  sendAmount,
				},
			},
			operationFee: big.NewInt(0),
			tokenInfo:    tokenInfo,
		})
		require.NoError(t, err)
	})

	t.Run("8. Less than allowed to bridge", func(t *testing.T) {
		err := executeInvalidNexusBridgingRequest(t, ctx, apex, user, InvalidNexusBridgingRequest{
			dstChainID: cardanofw.ChainIDToInt(cardanofw.ChainIDVector),
			sender:     user,
			receivers: map[string]cardanofw.ReceiverAmount{
				user.GetAddress(cardanofw.ChainIDVector): {
					TokenID: cardanofw.USDTTokenID,
					Amount:  big.NewInt(0),
				},
			},
			operationFee: big.NewInt(0),
			tokenInfo:    tokenInfo,
		})
		require.Error(t, err)
	})

	t.Run("9. Negative amount in receivers", func(t *testing.T) {
		err := executeInvalidNexusBridgingRequest(t, ctx, apex, user, InvalidNexusBridgingRequest{
			dstChainID: cardanofw.ChainIDToInt(cardanofw.ChainIDVector),
			sender:     user,
			receivers: map[string]cardanofw.ReceiverAmount{
				user.GetAddress(cardanofw.ChainIDVector): {
					TokenID: cardanofw.USDTTokenID,
					Amount:  sendAmount,
				},
				apex.Users[1].GetAddress(cardanofw.ChainIDVector): {
					TokenID: cardanofw.USDTTokenID,
					Amount:  big.NewInt(-1),
				},
			},
			operationFee: big.NewInt(0),
			tokenInfo:    tokenInfo,
		})
		require.Error(t, err)
	})

	t.Run("10. Incorrect token id in receivers", func(t *testing.T) {
		req := InvalidNexusBridgingRequest{
			dstChainID: cardanofw.ChainIDToInt(cardanofw.ChainIDVector),
			sender:     user,
			receivers: map[string]cardanofw.ReceiverAmount{
				user.GetAddress(cardanofw.ChainIDVector): {
					TokenID: 0,
					Amount:  sendAmount,
				},
			},
			operationFee: big.NewInt(0),
			tokenInfo:    tokenInfo,
		}

		err := executeInvalidNexusBridgingRequest(t, ctx, apex, user, req)
		require.Error(t, err)
		require.ErrorContains(t, err, "transaction receipt status is unsuccessful")
	})

	t.Run("11. Insufficient balance", func(t *testing.T) {
		err := executeInvalidNexusBridgingRequest(t, ctx, apex, user, InvalidNexusBridgingRequest{
			dstChainID: cardanofw.ChainIDToInt(cardanofw.ChainIDVector),
			sender:     user,
			receivers: map[string]cardanofw.ReceiverAmount{
				user.GetAddress(cardanofw.ChainIDVector): {
					TokenID: cardanofw.USDTTokenID,
					Amount:  new(big.Int).Mul(sendAmount, big.NewInt(1000000000000000000)),
				},
			},
			operationFee: big.NewInt(0),
			tokenInfo:    tokenInfo,
		})
		require.Error(t, err)
		require.ErrorContains(t, err, "transaction receipt status is unsuccessful")
	})

	t.Run("12. Insufficient fee", func(t *testing.T) {
		err := executeInvalidNexusBridgingRequest(t, ctx, apex, user, InvalidNexusBridgingRequest{
			dstChainID: cardanofw.ChainIDToInt(cardanofw.ChainIDVector),
			sender:     user,
			receivers: map[string]cardanofw.ReceiverAmount{
				user.GetAddress(cardanofw.ChainIDVector): {
					TokenID: cardanofw.USDTTokenID,
					Amount:  sendAmount,
				},
			},
			operationFee: big.NewInt(0),
			feeAmount:    big.NewInt(1000000000),
			tokenInfo:    tokenInfo,
		})
		require.Error(t, err)
		require.ErrorContains(t, err, "transaction receipt status is unsuccessful")
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

	allUsers := append([]*cardanofw.TestApexUser{apex.FunderUser}, users...)

	balanceToString := func(tokenID uint16, balance *big.Int) {
		if balance == nil {
			return
		}

		if balance.Cmp(big.NewInt(0)) == 0 {
			return
		}

		tokenName := apex.EcosystemTokens[tokenID]
		fmt.Printf("  %s = %s\n", tokenName, balance.String())
	}

	for i, user := range allUsers {
		fmt.Printf("=============================\n")
		fmt.Printf("user: %d\n", i)

		for _, chain := range skylineChains {
			addr := user.GetAddress(chain)

			if balance, exists := balances[addr]; !exists {
				fmt.Printf("%s addr: %s, balance: No data\n", chain, addr)
			} else {
				fmt.Printf("%s addr: %s\n", chain, addr)

				if chain == cardanofw.ChainIDNexus {
					info := apex.NexusInfo
					for tokenID, token := range info.Tokens {
						balanceToString(tokenID, balance[token.ChainSpecific])
					}
				} else {
					info := apex.GetCardanoInfo(chain)
					for tokenID, token := range info.Tokens {
						balanceToString(tokenID, balance[token.ChainSpecific])
					}
				}
			}
		}

		fmt.Printf("=============================\n")
	}
}
