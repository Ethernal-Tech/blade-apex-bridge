package e2e

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"testing"
	"time"

	"slices"

	"github.com/0xPolygon/polygon-edge/e2e-polybft/cardanofw"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/e2ehelper"
	"github.com/0xPolygon/polygon-edge/jsonrpc"
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

	tokensToFundApex := big.NewInt(100)
	tokensToFund := cardanofw.ApexToWei(tokensToFundApex)

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

			tokens := func() []cardanofw.GenericTokenAmount {
				if chain == cardanofw.ChainIDNexus {
					chainInfo := apex.GetEvmInfo(chain)
					tokens := make([]cardanofw.GenericTokenAmount, len(fundableTokensPerChain[chain]))

					for i, tokenID := range fundableTokensPerChain[chain] {
						tokens[i] = cardanofw.NewGenericTokenAmount(
							cardanowallet.Token{PolicyID: chainInfo.Tokens[tokenID].ChainSpecific}, tokensToFund)
					}

					return tokens
				}

				chainInfo := apex.GetCardanoInfo(chain)
				tokens := make([]cardanofw.GenericTokenAmount, len(fundableTokensPerChain[chain]))

				for i, tokenID := range fundableTokensPerChain[chain] {
					token, err := cardanowallet.NewTokenWithFullNameTry(chainInfo.Tokens[tokenID].ChainSpecific)
					require.NoError(t, err)

					tokens[i] = cardanofw.NewGenericTokenAmount(token, tokensToFund)
				}

				return tokens
			}()

			for _, user := range apex.Users {
				receiverAddr := user.GetAddress(chain)

				fmt.Printf("Funding %s address: %s\n", chain, receiverAddr)

				// resubmit the transaction in case of error because of a possible rollback
				_, err := common.ExecuteWithRetry(ctx, func(ctx context.Context) (string, error) {
					txHash, err := apex.SubmitTx(ctx, chain, apex.FunderUser, receiverAddr,
						tokensToFund, tokens, nil, nil)
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

				// 1. Compute change in Wei (PotentialFee is in Wei units)
				change := new(big.Int).Mul(
					cardanofw.PotentialFee, new(big.Int).SetUint64(uint64(len(balance))))

				if balance[cardanowallet.AdaTokenName].Cmp(change) <= 0 {
					continue
				}

				// 2. Refund amount in Wei
				refundAmount := new(big.Int).Sub(balance[cardanowallet.AdaTokenName], change)

				tokens := make([]cardanofw.GenericTokenAmount, 0, len(balance)-1)

				// 3. Token refunds
				for token, amount := range balance {
					if token == cardanowallet.AdaTokenName {
						continue
					}

					tokens = append(tokens, cardanofw.NewGenericTokenAmount(cardanowallet.Token{PolicyID: token}, amount))
				}

				wg.Add(1)

				go func(user *cardanofw.TestApexUser, chain string) {
					defer wg.Done()

					fmt.Printf("Defunding %s address: %s\n", chain, user.GetAddress(chain))

					_, err := apex.SubmitTx(ctx, chain, user, apex.FunderUser.GetAddress(chain),
						refundAmount, tokens, nil, nil)

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

			balance, exists := balances[senderAddr.String()]
			if !exists {
				continue
			}

			balanceDfm := make(map[string]uint64, len(balance))
			for tokenName, amount := range balance {
				balanceDfm[tokenName] = cardanofw.WeiToDfm(amount).Uint64()
			}

			// bring back all tokens from user to funderReceiverAddr
			tokens, err := cardanowallet.GetTokensFromSumMap(balanceDfm)
			require.NoError(t, err)

			receiverMinUtxoDfm, err := txBuilder.SetProtocolParameters(protParams).CalculateMinUtxo(cardanowallet.TxOutputWithRefScript{
				TxOutput: cardanowallet.TxOutput{
					Addr:   senderAddr.String(),
					Tokens: tokens,
				},
			})
			require.NoError(t, err)

			changePlusPotentialFee := new(big.Int).Add(cardanofw.MinUTxODefaultValue, cardanofw.PotentialFee)
			balanceAtLeast := new(big.Int).Add(cardanofw.DfmToWei(new(big.Int).SetUint64(receiverMinUtxoDfm)), changePlusPotentialFee)

			lovelaceBalance := balance[cardanowallet.AdaTokenName]
			if lovelaceBalance.Cmp(balanceAtLeast) < 0 {
				continue
			}

			refundAmountLovelace := new(big.Int).Sub(lovelaceBalance, changePlusPotentialFee)

			genericTokens := make([]cardanofw.GenericTokenAmount, 0, len(tokens))
			for _, t := range tokens {
				genericTokens = append(genericTokens, cardanofw.NewGenericTokenAmount(t.Token, cardanofw.DfmToWei(new(big.Int).SetUint64(t.Amount))))
			}

			wg.Add(1)

			go func(user *cardanofw.TestApexUser, chain string) {
				defer wg.Done()

				fmt.Printf("Defunding %s address: %s\n", chain, senderAddr)

				_, err := apex.SubmitTx(ctx, chain, user, funderReceiverAddr,
					refundAmountLovelace, genericTokens, nil, nil)

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
	sendAmount := cardanofw.ApexToWei(big.NewInt(1))
	bridgingRequests := []struct {
		src        string
		dest       string
		srcTokenID uint16
	}{
		{src: cardanofw.ChainIDPrime, dest: cardanofw.ChainIDCardano, srcTokenID: cardanofw.AP3XTokenID},
		{src: cardanofw.ChainIDCardano, dest: cardanofw.ChainIDPrime, srcTokenID: cardanofw.CAP3XTokenID},
		{src: cardanofw.ChainIDVector, dest: cardanofw.ChainIDCardano, srcTokenID: cardanofw.XADATokenID},
		{src: cardanofw.ChainIDCardano, dest: cardanofw.ChainIDVector, srcTokenID: cardanofw.ADATokenID},
		//// order is important here
		{src: cardanofw.ChainIDCardano, dest: cardanofw.ChainIDNexus, srcTokenID: cardanofw.ADATokenID},
		{src: cardanofw.ChainIDNexus, dest: cardanofw.ChainIDCardano, srcTokenID: cardanofw.XADATokenID},
		{src: cardanofw.ChainIDVector, dest: cardanofw.ChainIDNexus, srcTokenID: cardanofw.XADATokenID},
		{src: cardanofw.ChainIDNexus, dest: cardanofw.ChainIDVector, srcTokenID: cardanofw.XADATokenID},
		{src: cardanofw.ChainIDNexus, dest: cardanofw.ChainIDVector, srcTokenID: cardanofw.USDTTokenID},
		{src: cardanofw.ChainIDVector, dest: cardanofw.ChainIDNexus, srcTokenID: cardanofw.USDTTokenID},
	}

	for _, dir := range bridgingRequests {
		fmt.Printf("bridging from %s to %s, srcTokenID: %d\n", dir.src, dir.dest, dir.srcTokenID)

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, dir.src, dir.dest, sendAmount, dir.srcTokenID, false, bridgingOpts...)
	}
}

func TestE2E_SkylineTestnetBridge_ValidScenarios(t *testing.T) {
	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	apex, err := cardanofw.SetupSkylineRemoteBridge(t, cardanofw.GetTestnetSkylineBridgeConfig())
	require.NoError(t, err)

	user := apex.Users[0]
	sendAmount := cardanofw.DfmToWei(big.NewInt(1_050_000))

	const numOfInstanceForSequentialTests = 3

	t.Run("Prime -> Cardano - currency on src", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDPrime, cardanofw.ChainIDCardano,
			sendAmount, cardanofw.AP3XTokenID, false)
	})

	t.Run("Cardano -> Prime - native token on src", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDCardano, cardanofw.ChainIDPrime,
			sendAmount, cardanofw.CAP3XTokenID, false)
	})

	t.Run("Prime -> Cardano sequential currency on source", func(t *testing.T) {
		e2ehelper.ExecuteBridgingWaitAfterSubmits(
			t, ctx, apex, numOfInstanceForSequentialTests, user,
			cardanofw.ChainIDPrime, cardanofw.ChainIDCardano,
			sendAmount, cardanofw.AP3XTokenID, bridgingOpts...)
	})

	t.Run("Vector -> Cardano sequential native token on source", func(t *testing.T) {
		e2ehelper.ExecuteBridgingWaitAfterSubmits(
			t, ctx, apex, numOfInstanceForSequentialTests, user,
			cardanofw.ChainIDVector, cardanofw.ChainIDCardano,
			sendAmount, cardanofw.XADATokenID, bridgingOpts...)
	})

	t.Run("Cardano -> Vector sequential currency on source", func(t *testing.T) {
		e2ehelper.ExecuteBridgingWaitAfterSubmits(
			t, ctx, apex, numOfInstanceForSequentialTests, user,
			cardanofw.ChainIDCardano, cardanofw.ChainIDVector,
			sendAmount, cardanofw.ADATokenID, bridgingOpts...)
	})

	t.Run("Cardano -> Prime sequential native token on source", func(t *testing.T) {
		e2ehelper.ExecuteBridgingWaitAfterSubmits(
			t, ctx, apex, numOfInstanceForSequentialTests, user,
			cardanofw.ChainIDCardano, cardanofw.ChainIDPrime,
			sendAmount, cardanofw.CAP3XTokenID, bridgingOpts...)
	})

	executeAllDirectionsMulReceiversTest := func(t *testing.T, chainsDst map[string][]string, txTypes map[e2ehelper.SrcDstChainPair]uint16) {
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
			sendAmount, options...)
	}

	t.Run("Both directions sequential and parallel multiple receivers currency on source", func(t *testing.T) {
		executeAllDirectionsMulReceiversTest(t, map[string][]string{
			cardanofw.ChainIDPrime:   {cardanofw.ChainIDCardano},
			cardanofw.ChainIDCardano: {cardanofw.ChainIDVector},
		}, map[e2ehelper.SrcDstChainPair]uint16{
			e2ehelper.NewChainPair(cardanofw.ChainIDPrime, cardanofw.ChainIDCardano):  cardanofw.AP3XTokenID,
			e2ehelper.NewChainPair(cardanofw.ChainIDCardano, cardanofw.ChainIDVector): cardanofw.ADATokenID,
		})
	})

	t.Run("Both directions sequential and parallel multiple receivers with cardano as a source", func(t *testing.T) {
		executeAllDirectionsMulReceiversTest(t, map[string][]string{
			cardanofw.ChainIDCardano: {cardanofw.ChainIDPrime, cardanofw.ChainIDVector},
		}, map[e2ehelper.SrcDstChainPair]uint16{
			e2ehelper.NewChainPair(cardanofw.ChainIDCardano, cardanofw.ChainIDPrime):  cardanofw.CAP3XTokenID,
			e2ehelper.NewChainPair(cardanofw.ChainIDCardano, cardanofw.ChainIDVector): cardanofw.ADATokenID,
		})
	})
}

func TestE2E_SkylineTestnetBridge_ValidScenarios_ColoredCoins(t *testing.T) {
	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	apex, err := cardanofw.SetupSkylineRemoteBridge(t, cardanofw.GetTestnetSkylineBridgeConfig())
	require.NoError(t, err)

	user := apex.Users[6]
	sendAmount := cardanofw.DfmToWei(big.NewInt(1_050_000))

	const numOfInstanceForSequentialTests = 3

	t.Run("1. Cardano -> Vector -> Nexus -> Cardano - ADA/xADA", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDCardano, cardanofw.ChainIDVector, sendAmount,
			cardanofw.ADATokenID, false)

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDVector, cardanofw.ChainIDNexus, sendAmount,
			cardanofw.XADATokenID, false)

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDNexus, cardanofw.ChainIDCardano, sendAmount,
			cardanofw.XADATokenID, false)
	})

	t.Run("2. Cardano -> Nexus -> Vector -> Cardano - ADA/xADA", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDCardano, cardanofw.ChainIDNexus, sendAmount,
			cardanofw.ADATokenID, false)

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDNexus, cardanofw.ChainIDVector, sendAmount,
			cardanofw.XADATokenID, false)

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDVector, cardanofw.ChainIDCardano, sendAmount,
			cardanofw.XADATokenID, false)
	})

	t.Run("3. Nexus -> Vector sequential USDT", func(t *testing.T) {
		e2ehelper.ExecuteBridgingWaitAfterSubmits(
			t, ctx, apex, numOfInstanceForSequentialTests, user,
			cardanofw.ChainIDNexus, cardanofw.ChainIDVector,
			sendAmount, cardanofw.USDTTokenID, bridgingOpts...)
	})

	t.Run("4. Vector -> Nexus sequential USDT", func(t *testing.T) {
		e2ehelper.ExecuteBridgingWaitAfterSubmits(
			t, ctx, apex, numOfInstanceForSequentialTests, user,
			cardanofw.ChainIDVector, cardanofw.ChainIDNexus,
			sendAmount, cardanofw.USDTTokenID, bridgingOpts...)
	})

	t.Run("5. Cardano -> Nexus sequential ADA", func(t *testing.T) {
		e2ehelper.ExecuteBridgingWaitAfterSubmits(
			t, ctx, apex, numOfInstanceForSequentialTests, user,
			cardanofw.ChainIDCardano, cardanofw.ChainIDNexus,
			sendAmount, cardanofw.ADATokenID, bridgingOpts...)
	})

	t.Run("6. Nexus -> Cardano sequential xADA", func(t *testing.T) {
		e2ehelper.ExecuteBridgingWaitAfterSubmits(
			t, ctx, apex, numOfInstanceForSequentialTests, user,
			cardanofw.ChainIDNexus, cardanofw.ChainIDCardano,
			sendAmount, cardanofw.XADATokenID, bridgingOpts...)
	})

	fundingSuccessfull := t.Run("7. Vector -> Nexus sequential xADA", func(t *testing.T) {
		e2ehelper.ExecuteBridgingWaitAfterSubmits(
			t, ctx, apex, numOfInstanceForSequentialTests, user,
			cardanofw.ChainIDVector, cardanofw.ChainIDNexus,
			sendAmount, cardanofw.XADATokenID, bridgingOpts...)
	})

	t.Run("8. Nexus -> Vector and Cardano -> Vector in parallel - sequential xADA", func(t *testing.T) {
		if !fundingSuccessfull {
			t.Skip()
		}

		bridgingDirections := []e2ehelper.ExecuteBridgingConfig{
			{SrcChain: cardanofw.ChainIDNexus, DstChain: cardanofw.ChainIDVector, SrcTokenID: cardanofw.XADATokenID, SendAmountWei: sendAmount},
			{SrcChain: cardanofw.ChainIDCardano, DstChain: cardanofw.ChainIDVector, SrcTokenID: cardanofw.ADATokenID, SendAmountWei: sendAmount},
		}

		e2ehelper.ExecuteBridgingWaitAfterSubmitsExtended(
			t, ctx, apex, numOfInstanceForSequentialTests, user,
			bridgingDirections,
			bridgingOpts...)

		returnAmount := new(big.Int).Mul(sendAmount, big.NewInt(int64(numOfInstanceForSequentialTests*len(bridgingDirections))))

		// return all the xADA to Cardano
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDVector, cardanofw.ChainIDCardano,
			returnAmount, cardanofw.XADATokenID, false, bridgingOpts...)
	})

	t.Run("9. Nexus -> Cardano and Vector -> Cardano in parallel - sequential xADA", func(t *testing.T) {
		bridgingDirections := []e2ehelper.ExecuteBridgingConfig{
			{SrcChain: cardanofw.ChainIDNexus, DstChain: cardanofw.ChainIDCardano, SrcTokenID: cardanofw.XADATokenID, SendAmountWei: sendAmount},
			{SrcChain: cardanofw.ChainIDVector, DstChain: cardanofw.ChainIDCardano, SrcTokenID: cardanofw.XADATokenID, SendAmountWei: sendAmount},
		}

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDCardano, cardanofw.ChainIDVector, new(big.Int).Mul(sendAmount, big.NewInt(int64(numOfInstanceForSequentialTests*len(bridgingDirections)))),
			cardanofw.ADATokenID, false)

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDVector, cardanofw.ChainIDNexus, new(big.Int).Mul(sendAmount, big.NewInt(int64(numOfInstanceForSequentialTests))),
			cardanofw.XADATokenID, false, bridgingOpts...)

		e2ehelper.ExecuteBridgingWaitAfterSubmitsExtended(
			t, ctx, apex, numOfInstanceForSequentialTests, user,
			bridgingDirections, bridgingOpts...)
	})

	const (
		sequentialInstances = 2
		parallelInstances   = 3
		receiversCnt        = 2
	)

	senders := apex.Users[len(apex.Users)-parallelInstances:]

	executeAllDirectionsMulReceiversTest := func(t *testing.T, bridgingDirections []e2ehelper.BridgingDirectionConfig) {
		t.Helper()

		options := append(slices.Clone(bridgingOpts), e2ehelper.WithWaitForUnexpectedBridges(true))
		receivers := apex.Users[:receiversCnt]

		e2ehelper.ExecuteBridgingExtended(
			t, ctx, apex, sequentialInstances, senders, receivers,
			bridgingDirections,
			sendAmount, options...)
	}

	t.Run("10. Nexus <-> Vector USDT both directions parallel", func(t *testing.T) {
		fundAmount := new(big.Int).Mul(sendAmount, big.NewInt(sequentialInstances*parallelInstances))

		for _, user := range senders {
			e2ehelper.ExecuteSingleBridging(
				t, ctx, apex, user, user, cardanofw.ChainIDNexus, cardanofw.ChainIDVector, fundAmount,
				cardanofw.USDTTokenID, false, bridgingOpts...)
		}

		bridgingDirections := []e2ehelper.BridgingDirectionConfig{
			{SrcChain: cardanofw.ChainIDNexus, DstChain: cardanofw.ChainIDVector, SrcTokenID: cardanofw.USDTTokenID},
			{SrcChain: cardanofw.ChainIDVector, DstChain: cardanofw.ChainIDNexus, SrcTokenID: cardanofw.USDTTokenID},
		}

		executeAllDirectionsMulReceiversTest(t, bridgingDirections)
	})

	t.Run("11. Nexus <-> Vector xADA both directions parallel", func(t *testing.T) {
		fundAmount := new(big.Int).Mul(sendAmount, big.NewInt(sequentialInstances*parallelInstances))

		for _, user := range senders {
			e2ehelper.ExecuteSingleBridging(
				t, ctx, apex, user, user, cardanofw.ChainIDCardano, cardanofw.ChainIDNexus, fundAmount,
				cardanofw.ADATokenID, false)
		}

		bridgingDirections := []e2ehelper.BridgingDirectionConfig{
			{SrcChain: cardanofw.ChainIDNexus, DstChain: cardanofw.ChainIDVector, SrcTokenID: cardanofw.XADATokenID},
			{SrcChain: cardanofw.ChainIDVector, DstChain: cardanofw.ChainIDNexus, SrcTokenID: cardanofw.XADATokenID},
		}

		executeAllDirectionsMulReceiversTest(t, bridgingDirections)
	})

	t.Run("12. Nexus <-> Cardano xADA both directions parallel", func(t *testing.T) {
		fundAmount := new(big.Int).Mul(sendAmount, big.NewInt(sequentialInstances*parallelInstances))

		for _, user := range senders {
			e2ehelper.ExecuteSingleBridging(
				t, ctx, apex, user, user, cardanofw.ChainIDCardano, cardanofw.ChainIDNexus, fundAmount,
				cardanofw.ADATokenID, false)
		}

		bridgingDirections := []e2ehelper.BridgingDirectionConfig{
			{SrcChain: cardanofw.ChainIDNexus, DstChain: cardanofw.ChainIDCardano, SrcTokenID: cardanofw.XADATokenID},
			{SrcChain: cardanofw.ChainIDCardano, DstChain: cardanofw.ChainIDNexus, SrcTokenID: cardanofw.ADATokenID},
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

	primeCardanoTestConfig := newTestConfig(
		t, apex, apex.Config.PrimeConfig, &apex.PrimeInfo, cardanofw.ChainIDCardano, cardanofw.AP3XTokenID)
	cardanoVectorTestConfig := newTestConfig(
		t, apex, apex.Config.CardanoConfig, &apex.CardanoInfo, cardanofw.ChainIDVector, cardanofw.ADATokenID)
	vectorCardanoTestConfig := newTestConfig(
		t, apex, apex.Config.VectorConfig, &apex.VectorInfo, cardanofw.ChainIDCardano, cardanofw.XADATokenID)

	t.Run("1. Mismatch submitted and receiver amounts", func(t *testing.T) {
		executeInvalidMismatchSendLovelaceAmount(
			t, ctx, apex, primeCardanoTestConfig, apex.Users[0], requestStateTimeoutSec, retryIntervalSec, true, 0)
	})

	t.Run("2. Submitted invalid metadata - invalid bridging fee", func(t *testing.T) {
		executeInvalidBridgingFee(
			t, ctx, apex, primeCardanoTestConfig, requestStateTimeoutSec, retryIntervalSec, true, 0)
	})

	t.Run("3. Submitted invalid metadata - empty receivers", func(t *testing.T) {
		executeInvalidEmptyReceivers(
			t, ctx, apex, primeCardanoTestConfig, apex.Users[2], requestStateTimeoutSec, retryIntervalSec, true, 0)
	})

	t.Run("4. Submitted invalid metadata - wrong type", func(t *testing.T) {
		executeInvalidMetadataType(
			t, ctx, apex, primeCardanoTestConfig, apex.Users[1], requestStateTimeoutSec, retryIntervalSec, true, 0)
	})

	t.Run("5. Submitted invalid metadata - invalid destination", func(t *testing.T) {
		executeInvalidDestination(
			t, ctx, apex, cardanoVectorTestConfig, apex.Users[3], requestStateTimeoutSec, retryIntervalSec, true, 0)
	})

	t.Run("6. Submitted invalid metadata - invalid sender", func(t *testing.T) {
		executeInvalidMetadataInvalidSender(
			t, ctx, apex, cardanoVectorTestConfig, apex.Users[1], requestStateTimeoutSec, 0)
	})

	t.Run("7. Submitted invalid metadata - invalid fee receiver address - token on source", func(t *testing.T) {
		executeInvalidFeeReceiverAddr(
			t, ctx, apex, cardanoVectorTestConfig, cardanofw.CAP3XTokenID, requestStateTimeoutSec, retryIntervalSec, true, 0)
	})

	t.Run("8. Submitted with unknown tokens to bridging addr", func(t *testing.T) {
		user := apex.Users[len(apex.Users)-1]

		minterWallet, _ := user.GetCardanoWallet(cardanofw.ChainIDVector)

		tokensFunded, err := cardanofw.FundUserWithToken(
			ctx, apex, cardanofw.ChainIDVector,
			minterWallet, user,
			cardanofw.DefaultTokenName, cardanofw.DefaultTokenMintAmount,
			cardanofw.DfmToWei(big.NewInt(1_500_000)), cardanofw.ApexToWei(big.NewInt(1)))
		require.NoError(t, err)

		executeInvalidSendNativeToken(t, ctx, apex, user, vectorCardanoTestConfig, *tokensFunded, requestStateTimeoutSec, retryIntervalSec, true, 0)
	})

	t.Run("9. Submitted invalid metadata - invalid send amount - token on source", func(t *testing.T) {
		user := apex.Users[len(apex.Users)-1]

		token, err := cardanowallet.NewTokenWithFullNameTry(vectorCardanoTestConfig.tokensInfo.SrcTokenName)
		require.NoError(t, err)

		tokenAmount := &cardanofw.GenericTokenAmount{
			Amount: cardanofw.ApexToWei(big.NewInt(1)),
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

	tokenInfo, err := apex.GetBridgingTokensInfo(cardanofw.ChainIDNexus, cardanofw.ChainIDVector, cardanofw.USDTTokenID)
	require.NoError(t, err)

	sendAmount := cardanofw.ApexToWei(big.NewInt(1)) // 1*10^18

	operationFee := apex.GetMinOperationFee(cardanofw.ChainIDNexus)

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
				operationFee: operationFee,
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
				operationFee: operationFee,
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
			operationFee: operationFee,
			tokenInfo:    tokenInfo,
		})
		require.NoError(t, err)
	})

	t.Run("4. 0 receivers in bridging request", func(t *testing.T) {
		err := executeInvalidNexusBridgingRequest(t, ctx, apex, user, InvalidNexusBridgingRequest{
			dstChainID:   cardanofw.ChainIDToInt(cardanofw.ChainIDVector),
			sender:       user,
			operationFee: operationFee,
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
			operationFee: operationFee,
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
			operationFee: operationFee,
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
			operationFee: operationFee,
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
			operationFee: operationFee,
			tokenInfo:    tokenInfo,
		})
		require.Error(t, err)
		require.ErrorContains(t, err, "transaction receipt status is unsuccessful")
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
		require.ErrorContains(t, err, "transaction receipt status is unsuccessful")
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
			operationFee: operationFee,
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
			operationFee: operationFee,
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

// This test is necessary to start manually.
func TestE2E_SkylineTestnetBridge_NexusSrcGasPrice_NonDecreasing(t *testing.T) {
	t.Skip("Skipping manual test")

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	const numOfInstanceForSequentialTests = 10

	config := cardanofw.GetTestnetSkylineBridgeConfig()

	apex, err := cardanofw.SetupSkylineRemoteBridge(t, config)
	require.NoError(t, err)

	sendAmount := cardanofw.ApexToWei(big.NewInt(1))

	client, err := jsonrpc.NewEthClient(config.EVMChains[cardanofw.ChainIDNexus].Info.JSONRPCAddr)
	require.NoError(t, err)

	// 1. Measure Before
	gasPriceBefore, err := client.GasPrice()
	require.NoError(t, err)

	t.Logf("GasPrice Before: %d", gasPriceBefore)

	bridgingDirections := []e2ehelper.ExecuteBridgingConfig{
		{SrcChain: cardanofw.ChainIDNexus, DstChain: cardanofw.ChainIDVector, SrcTokenID: cardanofw.USDTTokenID, SendAmountWei: sendAmount},
	}

	var wg sync.WaitGroup

	wg.Add(len(apex.Users))

	// 2. Execute Load (Spam)
	t.Log("Starting bridge spam to raise gas price...")

	for _, user := range apex.Users {
		go func(u *cardanofw.TestApexUser) {
			defer wg.Done()
			e2ehelper.ExecuteBridgingWaitAfterSubmitsExtended(
				t, ctx, apex, numOfInstanceForSequentialTests, u,
				bridgingDirections,
				bridgingOpts...)
		}(user)
		time.Sleep(2 * time.Second)
	}

	wg.Wait()

	// 3. Measure Peak (Immediately after load)
	gasPricePeak, err := client.GasPrice()
	require.NoError(t, err)
	t.Logf("GasPrice Peak (after load): %d", gasPricePeak)

	// 4. Wait 5 Minutes (with monitoring)
	t.Log("Waiting 5 minutes to verify GasPrice does NOT decrease...")

	time.Sleep(5 * time.Minute)

	// 5. Measure Final
	gasPriceFinal, err := client.GasPrice()
	require.NoError(t, err)

	t.Logf("GasPrice Summary -> Start: %d, Peak: %d, Final: %d",
		gasPriceBefore, gasPricePeak, gasPriceFinal)
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
