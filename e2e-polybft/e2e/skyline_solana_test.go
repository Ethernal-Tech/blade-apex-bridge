package e2e

import (
	"context"
	"fmt"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/e2e-polybft/cardanofw"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/e2ehelper"
	cardanowallet "github.com/Ethernal-Tech/cardano-infrastructure/wallet"
	solanawallet "github.com/Ethernal-Tech/solana-infrastructure/wallet"
	"github.com/stretchr/testify/require"
)

// To run solana tests be sure to have necessary tools installed:
// https://solana.com/docs/intro/installation

func Test_SkylineSolana_AllDirections(t *testing.T) {
	const (
		apiKey = "test_api_key"
	)

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	primeConfig, cardanoConfig := cardanofw.NewPrimeChainConfig(), cardanofw.NewCardanoChainConfig(true)
	vectorConfig := cardanofw.NewVectorChainConfig(map[uint16]string{
		cardanofw.ASOLTokenID:  cardanofw.ASOLTokenName,
		cardanofw.USDTTokenID:  cardanofw.USDTTokenName,
		cardanofw.SAP3XTokenID: cardanofw.SAP3XTokenName,
	})
	nexusConfig := cardanofw.NewNexusChainConfig(true)

	solanaConfig := cardanofw.NewSolanaChainConfig(true)

	apex := cardanofw.SetupAndRunSkylineBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithCardanoConfig(cardanoConfig),
		cardanofw.WithPrimeConfig(primeConfig),
		cardanofw.WithVectorConfig(vectorConfig),
		cardanofw.WithSolanaConfig(solanaConfig),
		cardanofw.WithNexusConfig(nexusConfig),
		cardanofw.WithUserCnt(1),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	fmt.Println("solana user addr: ", apex.Users[0].SolanaAddress)
	balance, err := apex.GetBalance(ctx, apex.Users[0], cardanofw.ChainIDSolana)
	require.NoError(t, err)
	fmt.Println("solana user SOL balance: ", balance)
	time.Sleep(1 * time.Second)

	balance, err = apex.GetBalanceWithTokenName(ctx, apex.Users[0], cardanofw.ChainIDSolana, cardanofw.WSOLMintAddress)
	require.NoError(t, err)
	fmt.Println("solana user wSOL balance: ", balance)

	t.Run("Min amount bridging tests", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, apex.Users[0], apex.Users[0], cardanofw.ChainIDSolana, cardanofw.ChainIDVector, cardanofw.LamportToWei(solanaConfig.MinTokenBridgingAmount),
			cardanofw.WSOLTokenID, true)

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, apex.Users[0], apex.Users[0], cardanofw.ChainIDVector, cardanofw.ChainIDSolana, cardanofw.LamportToWei(solanaConfig.MinTokenBridgingAmount),
			cardanofw.ASOLTokenID, true)
	})

	t.Run("SOL -> Vector", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, apex.Users[0], apex.Users[0], cardanofw.ChainIDSolana, cardanofw.ChainIDVector, cardanofw.SolanaToWei(big.NewInt(1)),
			cardanofw.WSOLTokenID, true)
	})
	t.Run("Vector -> SOL", func(t *testing.T) {
		relayerUser := &cardanofw.TestApexUser{
			HasSolanaWallet: true,
			SolanaAddress:   apex.SolanaInfo.RelayerAddress,
		}
		relayerBalance, err := apex.GetBalance(ctx, relayerUser, cardanofw.ChainIDSolana)
		require.NoError(t, err)

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, apex.Users[0], apex.Users[0], cardanofw.ChainIDVector, cardanofw.ChainIDSolana, cardanofw.SolanaToWei(big.NewInt(1)),
			cardanofw.ASOLTokenID, true)

		relayerBalanceAfter, err := apex.GetBalance(ctx, relayerUser, cardanofw.ChainIDSolana)
		require.NoError(t, err)

		require.True(t, relayerBalanceAfter["lovelace"].Cmp(relayerBalance["lovelace"]) > 0)
	})

	t.Run("SOL -> Nexus", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, apex.Users[0], apex.Users[0], cardanofw.ChainIDSolana, cardanofw.ChainIDNexus, cardanofw.SolanaToWei(big.NewInt(1)),
			cardanofw.WSOLTokenID, true)
	})

	t.Run("Nexus -> SOL", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, apex.Users[0], apex.Users[0], cardanofw.ChainIDNexus, cardanofw.ChainIDSolana, cardanofw.SolanaToWei(big.NewInt(1)),
			cardanofw.ASOLTokenID, true)
	})

	t.Run("Vector AP3X -> Solana sAP3X", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, apex.Users[0], apex.Users[0], cardanofw.ChainIDVector, cardanofw.ChainIDSolana, cardanofw.ApexToWei(big.NewInt(1)),
			cardanofw.AP3XTokenID, true)
	})

	t.Run("Solana sAP3X -> Vector AP3X", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, apex.Users[0], apex.Users[0], cardanofw.ChainIDSolana, cardanofw.ChainIDVector, cardanofw.ApexToWei(big.NewInt(1)),
			cardanofw.SAP3XTokenID, true)
	})

	t.Run("Nexus AP3X -> Solana sAP3X", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, apex.Users[0], apex.Users[0], cardanofw.ChainIDNexus, cardanofw.ChainIDSolana, cardanofw.ApexToWei(big.NewInt(1)),
			cardanofw.AP3XTokenID, true)
	})

	t.Run("Solana sAP3X -> Nexus AP3X", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, apex.Users[0], apex.Users[0], cardanofw.ChainIDSolana, cardanofw.ChainIDNexus, cardanofw.ApexToWei(big.NewInt(1)),
			cardanofw.SAP3XTokenID, true)
	})

	// mint VS and NS tokens to the user
	_, err = cardanofw.FundUserWithToken(ctx, apex, cardanofw.ChainIDVector,
		apex.VectorInfo.GenesisWallet, apex.Users[0], cardanofw.VSTokenName,
		cardanofw.ApexToWei(big.NewInt(400_000_000)), cardanofw.ApexToWei(big.NewInt(1)), cardanofw.ApexToWei(big.NewInt(400_000_000)))
	require.NoError(t, err)

	nexusChain := apex.GetChainMust(t, cardanofw.ChainIDNexus).(*cardanofw.TestEVMChain)
	err = nexusChain.FundUsersWithToken(apex.Users[0].GetAddress(cardanofw.ChainIDNexus), cardanofw.DfmToWei(big.NewInt(400_000_000)), cardanofw.NSTokenID)
	require.NoError(t, err)

	userBalance, err := apex.GetBalanceWithTokenName(ctx, apex.Users[0], cardanofw.ChainIDVector, apex.VectorInfo.Tokens[cardanofw.VSTokenID].ChainSpecific)
	require.NoError(t, err)
	fmt.Println("vector user VS balance: ", userBalance)
	userBalance, err = apex.GetBalanceWithTokenName(ctx, apex.Users[0], cardanofw.ChainIDNexus, apex.NexusInfo.Tokens[cardanofw.NSTokenID].ChainSpecific)
	require.NoError(t, err)
	fmt.Println("nexus user NS balance: ", userBalance)

	t.Run("Vector VS -> Solana VS", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, apex.Users[0], apex.Users[0], cardanofw.ChainIDVector, cardanofw.ChainIDSolana, cardanofw.ApexToWei(big.NewInt(1)),
			cardanofw.VSTokenID, true)
	})

	t.Run("Solana VS -> Vector VS", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, apex.Users[0], apex.Users[0], cardanofw.ChainIDSolana, cardanofw.ChainIDVector, cardanofw.ApexToWei(big.NewInt(1)),
			cardanofw.VSTokenID, true)
	})

	t.Run("Nexus NS -> Solana NS", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, apex.Users[0], apex.Users[0], cardanofw.ChainIDNexus, cardanofw.ChainIDSolana, cardanofw.ApexToWei(big.NewInt(1)),
			cardanofw.NSTokenID, true)
	})

	t.Run("Solana NS -> Nexus NS", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, apex.Users[0], apex.Users[0], cardanofw.ChainIDSolana, cardanofw.ChainIDNexus, cardanofw.ApexToWei(big.NewInt(1)),
			cardanofw.NSTokenID, true)
	})
}

func Test_SkylineSolana_ForceFullBatch(t *testing.T) {
	const (
		apiKey = "test_api_key"
	)

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	primeConfig, cardanoConfig := cardanofw.NewPrimeChainConfig(), cardanofw.NewCardanoChainConfig(true)
	vectorConfig := cardanofw.NewVectorChainConfig(map[uint16]string{
		cardanofw.ASOLTokenID:  cardanofw.ASOLTokenName,
		cardanofw.USDTTokenID:  cardanofw.USDTTokenName,
		cardanofw.SAP3XTokenID: cardanofw.SAP3XTokenName,
	})
	nexusConfig := cardanofw.NewNexusChainConfig(true)

	solanaConfig := cardanofw.NewSolanaChainConfig(true)

	apex := cardanofw.SetupAndRunSkylineBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithCardanoConfig(cardanoConfig),
		cardanofw.WithPrimeConfig(primeConfig),
		cardanofw.WithVectorConfig(vectorConfig),
		cardanofw.WithSolanaConfig(solanaConfig),
		cardanofw.WithNexusConfig(nexusConfig),
		cardanofw.WithUserCnt(2),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	t.Run("SOL -> Vector", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, apex.Users[0], apex.Users[0], cardanofw.ChainIDSolana, cardanofw.ChainIDVector, cardanofw.SolanaToWei(big.NewInt(5)),
			cardanofw.WSOLTokenID, true)
	})

	// mint VS and NS tokens to the user
	_, err := cardanofw.FundUserWithToken(ctx, apex, cardanofw.ChainIDVector,
		apex.VectorInfo.GenesisWallet, apex.Users[0], cardanofw.VSTokenName,
		cardanofw.ApexToWei(big.NewInt(500_000_000)), cardanofw.ApexToWei(big.NewInt(1)), cardanofw.ApexToWei(big.NewInt(500_000_000)))
	require.NoError(t, err)

	nexusChain := apex.GetChainMust(t, cardanofw.ChainIDNexus).(*cardanofw.TestEVMChain)
	err = nexusChain.FundUsersWithToken(apex.Users[0].GetAddress(cardanofw.ChainIDNexus), cardanofw.DfmToWei(big.NewInt(500_000_000)), cardanofw.NSTokenID)
	require.NoError(t, err)

	userBalance, err := apex.GetBalanceWithTokenName(ctx, apex.Users[0], cardanofw.ChainIDVector, apex.VectorInfo.Tokens[cardanofw.VSTokenID].ChainSpecific)
	require.NoError(t, err)
	fmt.Println("vector user VS balance: ", userBalance)
	userBalance, err = apex.GetBalanceWithTokenName(ctx, apex.Users[0], cardanofw.ChainIDNexus, apex.NexusInfo.Tokens[cardanofw.NSTokenID].ChainSpecific)
	require.NoError(t, err)
	fmt.Println("nexus user NS balance: ", userBalance)

	solanaReceivers := make([]*cardanofw.TestApexUser, 4)
	for i := range len(solanaReceivers) {
		solanaReceivers[i], err = cardanofw.NewTestApexUser(cardanofw.NewApexNetworkTypes(cardanofw.ApexNetworkTypesParams{
			PrimeConfig:   primeConfig,
			VectorConfig:  vectorConfig,
			CardanoConfig: cardanoConfig,
			NexusConfig:   nexusConfig,
			SolanaConfig:  solanaConfig,
		}))
		require.NoError(t, err)
	}

	relayerUser := &cardanofw.TestApexUser{
		HasSolanaWallet: true,
		SolanaAddress:   apex.SolanaInfo.RelayerAddress,
	}

	for range 5 {
		t.Run("TEST SOLANA BRIDGING", func(t *testing.T) {
			wg := sync.WaitGroup{}
			wg.Add(4)

			relayerBalance, err := apex.GetBalance(ctx, relayerUser, cardanofw.ChainIDSolana)
			require.NoError(t, err)

			go func() {
				defer wg.Done()
				e2ehelper.ExecuteSingleBridging(
					t, ctx, apex, apex.Users[0], solanaReceivers[0], cardanofw.ChainIDVector, cardanofw.ChainIDSolana, cardanofw.ApexToWei(big.NewInt(1)),
					cardanofw.VSTokenID, false, e2ehelper.WithTimeoutConfig(
						e2ehelper.NewTimeoutConfig(
							e2ehelper.WithBridgingRetryWaitTime(10*time.Second),
							e2ehelper.WithBridgingNumRetries(150),
						)),
				)
			}()

			go func() {
				defer wg.Done()
				e2ehelper.ExecuteSingleBridging(
					t, ctx, apex, apex.Users[1], solanaReceivers[1], cardanofw.ChainIDVector, cardanofw.ChainIDSolana, cardanofw.ApexToWei(big.NewInt(1)),
					cardanofw.AP3XTokenID, false, e2ehelper.WithTimeoutConfig(
						e2ehelper.NewTimeoutConfig(
							e2ehelper.WithBridgingRetryWaitTime(10*time.Second),
							e2ehelper.WithBridgingNumRetries(150),
						)),
				)
			}()

			go func() {
				defer wg.Done()
				e2ehelper.ExecuteSingleBridging(
					t, ctx, apex, apex.Users[0], solanaReceivers[2], cardanofw.ChainIDNexus, cardanofw.ChainIDSolana, cardanofw.ApexToWei(big.NewInt(1)),
					cardanofw.NSTokenID, false, e2ehelper.WithTimeoutConfig(
						e2ehelper.NewTimeoutConfig(
							e2ehelper.WithBridgingRetryWaitTime(10*time.Second),
							e2ehelper.WithBridgingNumRetries(150),
						)),
				)
			}()

			go func() {
				defer wg.Done()
				e2ehelper.ExecuteSingleBridging(
					t, ctx, apex, apex.Users[0], solanaReceivers[3], cardanofw.ChainIDVector, cardanofw.ChainIDSolana, cardanofw.SolanaToWei(big.NewInt(1)),
					cardanofw.ASOLTokenID, false, e2ehelper.WithTimeoutConfig(
						e2ehelper.NewTimeoutConfig(
							e2ehelper.WithBridgingRetryWaitTime(10*time.Second),
							e2ehelper.WithBridgingNumRetries(150),
						)),
				)
			}()

			wg.Wait()

			relayerBalanceAfter, err := apex.GetBalance(ctx, relayerUser, cardanofw.ChainIDSolana)
			require.NoError(t, err)

			diff := new(big.Int).Sub(relayerBalanceAfter["lovelace"], relayerBalance["lovelace"])
			fee := new(big.Int).Sub(cardanofw.LamportToWei(big.NewInt(6_000_000*4)), diff)
			fmt.Println("diff: ", cardanofw.WeiToLamport(diff))
			fmt.Println("fee: ", cardanofw.WeiToLamport(fee))
		})
	}
}

func Test_SkylineSolana_OpFeeNotSet(t *testing.T) {
	const (
		apiKey = "test_api_key"
	)

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	primeConfig, cardanoConfig := cardanofw.NewPrimeChainConfig(), cardanofw.NewCardanoChainConfig(true)
	vectorConfig := cardanofw.NewVectorChainConfig(map[uint16]string{
		cardanofw.ASOLTokenID:  cardanofw.ASOLTokenName,
		cardanofw.USDTTokenID:  cardanofw.USDTTokenName,
		cardanofw.SAP3XTokenID: cardanofw.SAP3XTokenName,
	})
	nexusConfig := cardanofw.NewNexusChainConfig(true)

	solanaConfig := cardanofw.NewSolanaChainConfig(true)
	solanaConfig.MinOperationFee = big.NewInt(0)

	apex := cardanofw.SetupAndRunSkylineBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithCardanoConfig(cardanoConfig),
		cardanofw.WithPrimeConfig(primeConfig),
		cardanofw.WithVectorConfig(vectorConfig),
		cardanofw.WithSolanaConfig(solanaConfig),
		cardanofw.WithNexusConfig(nexusConfig),
		cardanofw.WithUserCnt(1),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	t.Run("SOL -> Vector", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, apex.Users[0], apex.Users[0], cardanofw.ChainIDSolana, cardanofw.ChainIDVector, cardanofw.SolanaToWei(big.NewInt(1)),
			cardanofw.WSOLTokenID, false)
	})

	t.Run("SOL -> Nexus", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, apex.Users[0], apex.Users[0], cardanofw.ChainIDSolana, cardanofw.ChainIDNexus, cardanofw.SolanaToWei(big.NewInt(1)),
			cardanofw.WSOLTokenID, false)
	})
}

func Test_SkylineSolana_UpgradeAndUpdates(t *testing.T) {
	if cardanofw.ShouldSkipE2RRedundantTests() {
		t.Skip()
	}

	const (
		apiKey = "test_api_key"
	)

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	primeConfig, cardanoConfig := cardanofw.NewPrimeChainConfig(), cardanofw.NewCardanoChainConfig(true)
	vectorConfig := cardanofw.NewVectorChainConfig(map[uint16]string{
		cardanofw.ASOLTokenID:  cardanofw.ASOLTokenName,
		cardanofw.USDTTokenID:  cardanofw.USDTTokenName,
		cardanofw.SAP3XTokenID: cardanofw.SAP3XTokenName,
	})
	nexusConfig := cardanofw.NewNexusChainConfig(true)

	solanaConfig := cardanofw.NewSolanaChainConfig(true)

	apex := cardanofw.SetupAndRunSkylineBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithCardanoConfig(cardanoConfig),
		cardanofw.WithPrimeConfig(primeConfig),
		cardanofw.WithVectorConfig(vectorConfig),
		cardanofw.WithSolanaConfig(solanaConfig),
		cardanofw.WithNexusConfig(nexusConfig),
		cardanofw.WithUserCnt(1),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	nexusChain := apex.GetChainMust(t, cardanofw.ChainIDNexus).(*cardanofw.TestEVMChain)
	err := nexusChain.FundUsersWithToken(apex.Users[0].GetAddress(cardanofw.ChainIDNexus), cardanofw.DfmToWei(big.NewInt(400_000_000)), cardanofw.NSTokenID)
	require.NoError(t, err)

	solanaChain := apex.GetChainMust(t, cardanofw.ChainIDSolana).(*cardanofw.TestSolanaChain)
	version, err := solanaChain.GetProgramVersion(ctx)
	require.NoError(t, err)
	require.Equal(t, "0.1.0", version)

	testFunc := func() {
		t.Run("Bridging after changes", func(t *testing.T) {
			t.Run("SOL -> Vector", func(t *testing.T) {
				e2ehelper.ExecuteSingleBridging(
					t, ctx, apex, apex.Users[0], apex.Users[0], cardanofw.ChainIDSolana, cardanofw.ChainIDVector, cardanofw.SolanaToWei(big.NewInt(1)),
					cardanofw.WSOLTokenID, true)
			})

			t.Run("Vector -> SOL", func(t *testing.T) {
				e2ehelper.ExecuteSingleBridging(
					t, ctx, apex, apex.Users[0], apex.Users[0], cardanofw.ChainIDVector, cardanofw.ChainIDSolana, cardanofw.SolanaToWei(big.NewInt(1)),
					cardanofw.ASOLTokenID, true)
			})

			t.Run("Nexus NS -> Solana NS", func(t *testing.T) {
				e2ehelper.ExecuteSingleBridging(
					t, ctx, apex, apex.Users[0], apex.Users[0], cardanofw.ChainIDNexus, cardanofw.ChainIDSolana, cardanofw.ApexToWei(big.NewInt(1)),
					cardanofw.NSTokenID, true)
			})

			t.Run("Solana NS -> Nexus NS", func(t *testing.T) {
				e2ehelper.ExecuteSingleBridging(
					t, ctx, apex, apex.Users[0], apex.Users[0], cardanofw.ChainIDSolana, cardanofw.ChainIDNexus, cardanofw.ApexToWei(big.NewInt(1)),
					cardanofw.NSTokenID, true)
			})
		})
	}

	testFunc()

	t.Run("Update min bridging amount", func(t *testing.T) {
		minTokenBridgingAmount := solanaConfig.MinTokenBridgingAmount
		newMinTokenBridgingAmount := big.NewInt(2000)

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, apex.Users[0], apex.Users[0], cardanofw.ChainIDSolana, cardanofw.ChainIDVector,
			cardanofw.LamportToWei(minTokenBridgingAmount),
			cardanofw.WSOLTokenID, true)

		solanaChain := apex.GetChainMust(t, cardanofw.ChainIDSolana).(*cardanofw.TestSolanaChain)
		err := solanaChain.UpdateMinBridgingAmount(ctx, cardanofw.WSOLTokenID, newMinTokenBridgingAmount)
		require.NoError(t, err)

		_, err = solanaChain.BridgingRequest(cardanofw.BridgingRequestParams{
			Ctx:            ctx,
			DestChainID:    cardanofw.ChainIDVector,
			PrivateKey:     apex.Users[0].SolanaWallet.PrivateKey.String(),
			ChainIDsConfig: "",
			Receivers: map[string]cardanofw.ReceiverAmount{
				apex.Users[0].GetAddress(cardanofw.ChainIDVector): {
					TokenID: cardanofw.WSOLTokenID,
					Amount:  cardanofw.LamportToWei(minTokenBridgingAmount),
				},
			},
			FeeAmount:      solanaConfig.MinBridgingFee,
			OperationFee:   solanaConfig.MinOperationFee,
			IsCurrencySrc:  false,
			IsCurrencyDest: false,
		})
		require.Error(t, err)
		require.ErrorContains(t, err, "BridgingAmountTooLow")

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, apex.Users[0], apex.Users[0], cardanofw.ChainIDSolana, cardanofw.ChainIDVector,
			cardanofw.LamportToWei(newMinTokenBridgingAmount),
			cardanofw.WSOLTokenID, true)
	})

	t.Run("Upgrade program", func(t *testing.T) {
		solanaChain := apex.GetChainMust(t, cardanofw.ChainIDSolana).(*cardanofw.TestSolanaChain)
		err := solanaChain.UpgradeProgram(ctx)
		require.NoError(t, err)

		version, err := solanaChain.GetProgramVersion(ctx)
		require.NoError(t, err)
		require.Equal(t, "0.2.0", version)
	})

	testFunc()

	newOpFee := big.NewInt(0).Add(big.NewInt(50000000), solanaConfig.MinOperationFee)

	t.Run("Update fee config - min operation fee", func(t *testing.T) {
		solanaChain := apex.GetChainMust(t, cardanofw.ChainIDSolana).(*cardanofw.TestSolanaChain)
		err := solanaChain.UpdateFeeConfig(ctx, cardanofw.UpdateFeeConfigDto{
			MinOperationFee: newOpFee,
			BridgeFee:       solanaConfig.MinBridgingFee,
		})
		require.NoError(t, err)

		treasuryUser := &cardanofw.TestApexUser{
			HasSolanaWallet: true,
			SolanaAddress:   solanaChain.GetTreasuryAddress(),
		}
		treasuryBalance, err := apex.GetBalance(ctx, treasuryUser, cardanofw.ChainIDSolana)
		require.NoError(t, err)

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, apex.Users[0], apex.Users[0], cardanofw.ChainIDSolana, cardanofw.ChainIDVector, cardanofw.SolanaToWei(big.NewInt(1)),
			cardanofw.WSOLTokenID, true)

		treasuryBalanceAfter, err := apex.GetBalance(ctx, treasuryUser, cardanofw.ChainIDSolana)
		require.NoError(t, err)

		diff := new(big.Int).Sub(treasuryBalanceAfter["lovelace"], treasuryBalance["lovelace"])
		require.True(t, diff.Cmp(cardanofw.LamportToWei(newOpFee)) == 0)
	})

	newBridgeFee := big.NewInt(0).Add(big.NewInt(50000000), solanaConfig.MinBridgingFee)

	t.Run("Update fee config - update bridging fee", func(t *testing.T) {
		solanaChain := apex.GetChainMust(t, cardanofw.ChainIDSolana).(*cardanofw.TestSolanaChain)
		err := solanaChain.UpdateFeeConfig(ctx, cardanofw.UpdateFeeConfigDto{
			BridgeFee:       newBridgeFee,
			MinOperationFee: newOpFee,
		})
		require.NoError(t, err)

		relayerUser := &cardanofw.TestApexUser{
			HasSolanaWallet: true,
			SolanaAddress:   apex.SolanaInfo.RelayerAddress,
		}
		relayerBalance, err := apex.GetBalance(ctx, relayerUser, cardanofw.ChainIDSolana)
		require.NoError(t, err)

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, apex.Users[0], apex.Users[0], cardanofw.ChainIDSolana, cardanofw.ChainIDVector, cardanofw.SolanaToWei(big.NewInt(1)),
			cardanofw.WSOLTokenID, true)

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, apex.Users[0], apex.Users[0], cardanofw.ChainIDVector, cardanofw.ChainIDSolana, cardanofw.SolanaToWei(big.NewInt(1)),
			cardanofw.ASOLTokenID, true)

		relayerBalanceAfter, err := apex.GetBalance(ctx, relayerUser, cardanofw.ChainIDSolana)
		require.NoError(t, err)

		diff := new(big.Int).Sub(relayerBalanceAfter["lovelace"], relayerBalance["lovelace"])
		require.True(t, diff.Cmp(cardanofw.LamportToWei(solanaConfig.MinBridgingFee)) == -1)
	})

	t.Run("Update fee config - update treasury address", func(t *testing.T) {
		newTreasuryWallet, err := solanawallet.NewWallet()
		require.NoError(t, err)

		solanaChain := apex.GetChainMust(t, cardanofw.ChainIDSolana).(*cardanofw.TestSolanaChain)
		err = solanaChain.UpdateFeeConfig(ctx, cardanofw.UpdateFeeConfigDto{
			MinOperationFee: newOpFee,
			BridgeFee:       newBridgeFee,
			UpdateTreasury:  true,
			TreasuryAddress: newTreasuryWallet.PublicKey.String(),
		})
		require.NoError(t, err)

		treasuryUser := &cardanofw.TestApexUser{
			HasSolanaWallet: true,
			SolanaAddress:   newTreasuryWallet.PublicKey.String(),
		}
		treasuryBalance, err := apex.GetBalance(ctx, treasuryUser, cardanofw.ChainIDSolana)
		require.NoError(t, err)

		require.True(t, treasuryBalance["lovelace"].Cmp(big.NewInt(0)) == 0)

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, apex.Users[0], apex.Users[0], cardanofw.ChainIDSolana, cardanofw.ChainIDVector, cardanofw.SolanaToWei(big.NewInt(1)),
			cardanofw.WSOLTokenID, true)

		treasuryBalanceAfter, err := apex.GetBalance(ctx, treasuryUser, cardanofw.ChainIDSolana)
		require.NoError(t, err)
		require.True(t, treasuryBalanceAfter["lovelace"].Cmp(cardanofw.LamportToWei(newOpFee)) == 0)
	})
}

func Test_SkylineSolana_LockUnlockTokenFlow(t *testing.T) {
	const (
		apiKey = "test_api_key"
	)

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	primeConfig, cardanoConfig := cardanofw.NewPrimeChainConfig(), cardanofw.NewCardanoChainConfig(true)
	vectorConfig := cardanofw.NewVectorChainConfig(map[uint16]string{
		cardanofw.ASOLTokenID:  cardanofw.ASOLTokenName,
		cardanofw.USDTTokenID:  cardanofw.USDTTokenName,
		cardanofw.SAP3XTokenID: cardanofw.SAP3XTokenName,
	})
	nexusConfig := cardanofw.NewNexusChainConfig(true)

	solanaConfig := cardanofw.NewSolanaChainConfig(true)
	solanaConfig.LockUnlockTokens[cardanofw.SAP3XTokenID] = cardanofw.SAP3XTokenName
	delete(solanaConfig.MintableTokens, cardanofw.SAP3XTokenID)

	apex := cardanofw.SetupAndRunSkylineBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithCardanoConfig(cardanoConfig),
		cardanofw.WithPrimeConfig(primeConfig),
		cardanofw.WithVectorConfig(vectorConfig),
		cardanofw.WithSolanaConfig(solanaConfig),
		cardanofw.WithNexusConfig(nexusConfig),
		cardanofw.WithUserCnt(1),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	t.Run("Vector AP3X -> Solana sAP3X", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, apex.Users[0], apex.Users[0], cardanofw.ChainIDVector, cardanofw.ChainIDSolana, cardanofw.ApexToWei(big.NewInt(1)),
			cardanofw.AP3XTokenID, true, e2ehelper.WithTimeoutConfig(
				e2ehelper.NewTimeoutConfig(
					e2ehelper.WithBridgingRetryWaitTime(10*time.Second),
					e2ehelper.WithBridgingNumRetries(150),
				),
			))
	})

	t.Run("Solana sAP3X -> Vector AP3X", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, apex.Users[0], apex.Users[0], cardanofw.ChainIDSolana, cardanofw.ChainIDVector, cardanofw.ApexToWei(big.NewInt(1)),
			cardanofw.SAP3XTokenID, true)
	})

	t.Run("Nexus AP3X -> Solana sAP3X", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, apex.Users[0], apex.Users[0], cardanofw.ChainIDNexus, cardanofw.ChainIDSolana, cardanofw.ApexToWei(big.NewInt(1)),
			cardanofw.AP3XTokenID, true)
	})

	t.Run("Solana sAP3X -> Nexus AP3X", func(t *testing.T) {
		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, apex.Users[0], apex.Users[0], cardanofw.ChainIDSolana, cardanofw.ChainIDNexus, cardanofw.ApexToWei(big.NewInt(1)),
			cardanofw.SAP3XTokenID, true)
	})
}

func Test_SkylineSolana_ValidScenarios(t *testing.T) {
	if cardanofw.ShouldSkipE2RRedundantTests() {
		t.Skip()
	}

	type bridgingRequest struct {
		src             string
		dest            string
		sender          *cardanofw.TestApexUser
		srcTokenID      uint16
		isValid         bool
		srcMinterWallet *cardanowallet.Wallet
	}

	const (
		apiKey  = "test_api_key"
		userCnt = 9
	)

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	primeConfig, cardanoConfig := cardanofw.NewPrimeChainConfig(), cardanofw.NewCardanoChainConfig(true)
	vectorConfig := cardanofw.NewVectorChainConfig(map[uint16]string{
		cardanofw.ASOLTokenID:  cardanofw.ASOLTokenName,
		cardanofw.USDTTokenID:  cardanofw.USDTTokenName,
		cardanofw.SAP3XTokenID: cardanofw.SAP3XTokenName,
	})
	nexusConfig := cardanofw.NewNexusChainConfig(true)

	solanaConfig := cardanofw.NewSolanaChainConfig(true)
	tokenFundAmountSol := cardanofw.LamportToWei(cardanofw.SolanaToLamport(big.NewInt(10000)))
	solanaConfig.FundAmount = tokenFundAmountSol

	apex := cardanofw.SetupAndRunSkylineBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithCardanoConfig(cardanoConfig),
		cardanofw.WithPrimeConfig(primeConfig),
		cardanofw.WithVectorConfig(vectorConfig),
		cardanofw.WithSolanaConfig(solanaConfig),
		cardanofw.WithNexusConfig(nexusConfig),
		cardanofw.WithUserCnt(userCnt),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	fundingPassed := t.Run("funding stage", func(t *testing.T) {
		// fund tokens that cannot be minted directly on the solana chain
		tokenAmount := cardanofw.ApexToWei(new(big.Int).Mul(big.NewInt(int64(userCnt)), big.NewInt(400_000_000)))
		bridgeAmount := cardanofw.ApexToWei(big.NewInt(1_000_000))
		bridgeAmountSol := cardanofw.SolanaToWei(big.NewInt(2000))

		var wg sync.WaitGroup

		errCh := make(chan error, 3)

		wg.Add(2)

		go func() {
			defer wg.Done()

			_, err := cardanofw.FundUserWithToken(ctx, apex, cardanofw.ChainIDVector,
				apex.VectorInfo.GenesisWallet, apex.Users[0], cardanofw.VSTokenName,
				tokenAmount, cardanofw.ApexToWei(big.NewInt(20)), tokenAmount)
			if err != nil {
				errCh <- fmt.Errorf("vector funding failed: %w", err)
			}
		}()

		go func() {
			defer wg.Done()

			nexusChain := apex.GetChainMust(t, cardanofw.ChainIDNexus).(*cardanofw.TestEVMChain)

			err := nexusChain.FundUsersWithToken(apex.Users[0].GetAddress(cardanofw.ChainIDNexus), tokenAmount, cardanofw.NSTokenID)
			if err != nil {
				errCh <- fmt.Errorf("nexus funding failed: %w", err)
			}
		}()

		wg.Wait()
		close(errCh)

		for err := range errCh {
			require.NoError(t, err)
		}

		lenUsers := len(apex.Users) - 1

		for i, user := range apex.Users {
			wg.Add(4)

			go func() {
				defer wg.Done()

				e2ehelper.ExecuteSingleBridging(
					t, ctx, apex, apex.Users[0], user, cardanofw.ChainIDVector, cardanofw.ChainIDSolana, bridgeAmount,
					cardanofw.VSTokenID, false, e2ehelper.WithTimeoutConfig(
						e2ehelper.NewTimeoutConfig(
							e2ehelper.WithBridgingRetryWaitTime(10*time.Second),
							e2ehelper.WithBridgingNumRetries(150),
						)),
				)
			}()

			go func() {
				defer wg.Done()

				e2ehelper.ExecuteSingleBridging(
					t, ctx, apex, apex.Users[0], user, cardanofw.ChainIDNexus, cardanofw.ChainIDSolana, bridgeAmount,
					cardanofw.NSTokenID, false, e2ehelper.WithTimeoutConfig(
						e2ehelper.NewTimeoutConfig(
							e2ehelper.WithBridgingRetryWaitTime(10*time.Second),
							e2ehelper.WithBridgingNumRetries(150),
						)),
				)
			}()

			go func() {
				defer wg.Done()

				e2ehelper.ExecuteSingleBridging(
					t, ctx, apex, apex.Users[i], apex.Users[i], cardanofw.ChainIDSolana, cardanofw.ChainIDVector, bridgeAmountSol,
					cardanofw.WSOLTokenID, false, e2ehelper.WithTimeoutConfig(
						e2ehelper.NewTimeoutConfig(
							e2ehelper.WithBridgingRetryWaitTime(10*time.Second),
							e2ehelper.WithBridgingNumRetries(150),
						)),
				)
			}()

			go func() {
				defer wg.Done()

				e2ehelper.ExecuteSingleBridging(
					t, ctx, apex, apex.Users[lenUsers-i], apex.Users[lenUsers-i], cardanofw.ChainIDSolana, cardanofw.ChainIDNexus, bridgeAmountSol,
					cardanofw.WSOLTokenID, false, e2ehelper.WithTimeoutConfig(
						e2ehelper.NewTimeoutConfig(
							e2ehelper.WithBridgingRetryWaitTime(10*time.Second),
							e2ehelper.WithBridgingNumRetries(150),
						)),
				)
			}()

			wg.Wait()

			fmt.Println("Fund round ", i+1, " completed")
		}
	})

	if !fundingPassed {
		fmt.Println("funding stage failed, stoping the test")
		t.FailNow()
	}

	var (
		bridgingRequests = []bridgingRequest{
			{src: cardanofw.ChainIDSolana, dest: cardanofw.ChainIDVector, sender: apex.Users[1], srcTokenID: cardanofw.WSOLTokenID, isValid: true},
			{src: cardanofw.ChainIDVector, dest: cardanofw.ChainIDSolana, sender: apex.Users[2], srcTokenID: cardanofw.ASOLTokenID, isValid: true},
			{src: cardanofw.ChainIDSolana, dest: cardanofw.ChainIDNexus, sender: apex.Users[3], srcTokenID: cardanofw.WSOLTokenID, isValid: true},
			{src: cardanofw.ChainIDNexus, dest: cardanofw.ChainIDSolana, sender: apex.Users[4], srcTokenID: cardanofw.ASOLTokenID, isValid: true},
			{src: cardanofw.ChainIDSolana, dest: cardanofw.ChainIDVector, sender: apex.Users[5], srcTokenID: cardanofw.VSTokenID, isValid: true},
			{src: cardanofw.ChainIDVector, dest: cardanofw.ChainIDSolana, sender: apex.Users[6], srcTokenID: cardanofw.VSTokenID, isValid: true, srcMinterWallet: apex.VectorInfo.GenesisWallet},
			{src: cardanofw.ChainIDSolana, dest: cardanofw.ChainIDNexus, sender: apex.Users[7], srcTokenID: cardanofw.NSTokenID, isValid: true},
			{src: cardanofw.ChainIDNexus, dest: cardanofw.ChainIDSolana, sender: apex.Users[8], srcTokenID: cardanofw.NSTokenID, isValid: true},
		}
	)

	bridgingAmount := cardanofw.ApexToWei(big.NewInt(1))
	//nolint:dupl
	t.Run("1. Wait for each submit", func(t *testing.T) {
		for idx, br := range bridgingRequests {
			fmt.Printf("1.%d %s -> %s - tokenID: %d\n", idx+1, br.src, br.dest, br.srcTokenID)

			t.Cleanup(func() {
				apex.ResetIndexers()
			})

			const (
				instances = 5
			)

			fundAmount := new(big.Int).Mul(bridgingAmount, big.NewInt(int64(instances)))

			if br.src == cardanofw.ChainIDVector && br.srcTokenID == cardanofw.VSTokenID {
				_, err := cardanofw.FundUserWithToken(ctx, apex, cardanofw.ChainIDVector,
					apex.VectorInfo.GenesisWallet, br.sender, cardanofw.VSTokenName,
					fundAmount, cardanofw.ApexToWei(big.NewInt(1)), fundAmount)
				require.NoError(t, err)
			}

			if br.src == cardanofw.ChainIDNexus && br.srcTokenID == cardanofw.NSTokenID {
				nexusChain := apex.GetChainMust(t, cardanofw.ChainIDNexus).(*cardanofw.TestEVMChain)
				err := nexusChain.FundUsersWithToken(br.sender.GetAddress(cardanofw.ChainIDNexus), fundAmount, br.srcTokenID)
				require.NoError(t, err)
			}

			e2ehelper.ExecuteBridgingOneByOneWaitOnOtherSide(
				t, ctx, apex, instances, br.sender, br.src, br.dest, bridgingAmount,
				br.srcTokenID)
		}
	})

	//nolint:dupl
	t.Run("2. One by one", func(t *testing.T) {
		for idx, br := range bridgingRequests {
			fmt.Printf("2.%d %s -> %s - tokenID: %d\n", idx+1, br.src, br.dest, br.srcTokenID)

			t.Cleanup(func() {
				apex.ResetIndexers()
			})

			const (
				instances = 5
			)

			fundAmount := new(big.Int).Mul(bridgingAmount, big.NewInt(int64(instances)))

			if br.src == cardanofw.ChainIDVector && br.srcTokenID == cardanofw.VSTokenID {
				_, err := cardanofw.FundUserWithToken(ctx, apex, cardanofw.ChainIDVector,
					apex.VectorInfo.GenesisWallet, br.sender, cardanofw.VSTokenName,
					fundAmount, cardanofw.ApexToWei(big.NewInt(1)), fundAmount)
				require.NoError(t, err)
			}

			if br.src == cardanofw.ChainIDNexus && br.srcTokenID == cardanofw.NSTokenID {
				nexusChain := apex.GetChainMust(t, cardanofw.ChainIDNexus).(*cardanofw.TestEVMChain)
				err := nexusChain.FundUsersWithToken(br.sender.GetAddress(cardanofw.ChainIDNexus), fundAmount, br.srcTokenID)
				require.NoError(t, err)
			}

			e2ehelper.ExecuteBridgingWaitAfterSubmits(
				t, ctx, apex, instances, br.sender, br.src, br.dest, bridgingAmount,
				br.srcTokenID)
		}
	})

	user := apex.Users[len(apex.Users)-1]

	t.Run("3. Parallel bridging tests", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() || !fundingPassed {
			t.Skip()
		}

		const (
			instances        = 5
			txCountPerSender = 1
		)

		for idx, br := range bridgingRequests {
			fmt.Printf("3.%d %s -> %s - srcTokenID: %d\n", idx+1, br.src, br.dest, br.srcTokenID)

			fundAmount := new(big.Int).Mul(bridgingAmount, big.NewInt(int64(txCountPerSender)))

			if br.src == cardanofw.ChainIDVector && br.srcTokenID == cardanofw.VSTokenID {
				for i := range instances {
					_, err := cardanofw.FundUserWithToken(ctx, apex, cardanofw.ChainIDVector,
						apex.VectorInfo.GenesisWallet, apex.Users[i], cardanofw.VSTokenName,
						fundAmount, cardanofw.ApexToWei(big.NewInt(1)), fundAmount)
					require.NoError(t, err)
				}
			}

			if br.src == cardanofw.ChainIDNexus && br.srcTokenID == cardanofw.NSTokenID {
				nexusChain := apex.GetChainMust(t, cardanofw.ChainIDNexus).(*cardanofw.TestEVMChain)
				for i := range instances {
					err := nexusChain.FundUsersWithToken(apex.Users[i].GetAddress(cardanofw.ChainIDNexus), fundAmount, br.srcTokenID)
					require.NoError(t, err)
				}
			}

			e2ehelper.ExecuteBridging(
				t, ctx, apex, txCountPerSender, apex.Users[:instances], []*cardanofw.TestApexUser{user},
				[]string{br.src},
				map[string][]string{
					br.src: {br.dest},
				},
				map[e2ehelper.SrcDstChainPair]uint16{
					e2ehelper.NewChainPair(br.src, br.dest): br.srcTokenID,
				},
				bridgingAmount)
		}
	})

	t.Run("4. Sequential bridging tests", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() || !fundingPassed {
			t.Skip()
		}

		const (
			sequentialInstances = 5
			receivers           = 1
		)

		for idx, br := range bridgingRequests {
			fmt.Printf("4.%d %s -> %s - srcTokenID: %d\n", idx+1, br.src, br.dest, br.srcTokenID)

			fundAmount := new(big.Int).Mul(bridgingAmount, big.NewInt(int64(sequentialInstances)))

			if br.src == cardanofw.ChainIDVector && br.srcTokenID == cardanofw.VSTokenID {
				for i := range sequentialInstances {
					_, err := cardanofw.FundUserWithToken(ctx, apex, cardanofw.ChainIDVector,
						apex.VectorInfo.GenesisWallet, apex.Users[i], cardanofw.VSTokenName,
						fundAmount, cardanofw.ApexToWei(big.NewInt(1)), fundAmount)
					require.NoError(t, err)
				}
			}

			if br.src == cardanofw.ChainIDNexus && br.srcTokenID == cardanofw.NSTokenID {
				nexusChain := apex.GetChainMust(t, cardanofw.ChainIDNexus).(*cardanofw.TestEVMChain)
				for i := range sequentialInstances {
					err := nexusChain.FundUsersWithToken(apex.Users[i].GetAddress(cardanofw.ChainIDNexus), fundAmount, br.srcTokenID)
					require.NoError(t, err)
				}
			}

			e2ehelper.ExecuteBridging(
				t, ctx, apex, sequentialInstances,
				apex.Users[:sequentialInstances],
				apex.Users[:receivers],
				[]string{br.src},
				map[string][]string{
					br.src: {br.dest},
				},
				map[e2ehelper.SrcDstChainPair]uint16{
					e2ehelper.NewChainPair(br.src, br.dest): br.srcTokenID,
				},
				bridgingAmount,
			)
		}
	})

	getBridgingRequests := func(bridgingRequests []bridgingRequest) []e2ehelper.BridgingDirectionConfig {
		res := make([]e2ehelper.BridgingDirectionConfig, len(bridgingRequests))
		for i, br := range bridgingRequests {
			res[i] = e2ehelper.BridgingDirectionConfig{
				SrcChain:   br.src,
				DstChain:   br.dest,
				SrcTokenID: br.srcTokenID,
			}
		}

		return res
	}

	t.Run("5. All directions parallel", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() || !fundingPassed {
			t.Skip()
		}

		const (
			instances        = 5
			txCountPerSender = 1
		)

		fundAmount := new(big.Int).Mul(bridgingAmount, big.NewInt(int64(txCountPerSender)))
		for i := range instances {
			_, err := cardanofw.FundUserWithToken(ctx, apex, cardanofw.ChainIDVector,
				apex.VectorInfo.GenesisWallet, apex.Users[i], cardanofw.VSTokenName,
				fundAmount, cardanofw.ApexToWei(big.NewInt(1)), fundAmount)
			require.NoError(t, err)
		}

		nexusChain := apex.GetChainMust(t, cardanofw.ChainIDNexus).(*cardanofw.TestEVMChain)
		for i := range instances {
			err := nexusChain.FundUsersWithToken(apex.Users[i].GetAddress(cardanofw.ChainIDNexus), fundAmount, cardanofw.NSTokenID)
			require.NoError(t, err)
		}

		e2ehelper.ExecuteBridgingExtended(
			t, ctx, apex, txCountPerSender,
			apex.Users[:instances],
			[]*cardanofw.TestApexUser{user},
			getBridgingRequests(bridgingRequests),
			bridgingAmount,
			e2ehelper.WithTimeoutConfig(e2ehelper.NewTimeoutConfig(
				e2ehelper.WithBridgingNumRetries(100),
				e2ehelper.WithBridgingRetryWaitTime(20*time.Second),
				e2ehelper.WithUnexpectedBridgesNumRetries(12),
				e2ehelper.WithUnexpectedBridgesRetryWaitTime(10*time.Second),
			)),
		)
	})

	t.Run("6. All directions sequential and parallel", func(t *testing.T) {
		const (
			instances = 5
		)

		fundAmount := new(big.Int).Mul(bridgingAmount, big.NewInt(int64(instances)))
		for i := range instances {
			_, err := cardanofw.FundUserWithToken(ctx, apex, cardanofw.ChainIDVector,
				apex.VectorInfo.GenesisWallet, apex.Users[i], cardanofw.VSTokenName,
				fundAmount, cardanofw.ApexToWei(big.NewInt(1)), fundAmount)
			require.NoError(t, err)
		}

		nexusChain := apex.GetChainMust(t, cardanofw.ChainIDNexus).(*cardanofw.TestEVMChain)
		for i := range instances {
			err := nexusChain.FundUsersWithToken(apex.Users[i].GetAddress(cardanofw.ChainIDNexus), fundAmount, cardanofw.NSTokenID)
			require.NoError(t, err)
		}

		e2ehelper.ExecuteBridgingExtended(
			t, ctx, apex, instances,
			apex.Users[:instances],
			[]*cardanofw.TestApexUser{user},
			getBridgingRequests(bridgingRequests),
			bridgingAmount,
			e2ehelper.WithTimeoutConfig(e2ehelper.NewTimeoutConfig(
				e2ehelper.WithBridgingNumRetries(120),
				e2ehelper.WithBridgingRetryWaitTime(30*time.Second),
				e2ehelper.WithUnexpectedBridgesNumRetries(12),
				e2ehelper.WithUnexpectedBridgesRetryWaitTime(10*time.Second),
			)),
		)
	})
}

func Test_SkylineSolana_InvalidScenarios(t *testing.T) {
	const (
		apiKey           = "test_api_key"
		maxWaitTimeSec   = 600
		retryIntervalSec = 10
	)

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	primeConfig, cardanoConfig := cardanofw.NewPrimeChainConfig(), cardanofw.NewCardanoChainConfig(true)
	vectorConfig := cardanofw.NewVectorChainConfig(map[uint16]string{
		cardanofw.ASOLTokenID: cardanofw.ASOLTokenName,
		cardanofw.USDTTokenID: cardanofw.USDTTokenName,
	})
	nexusConfig := cardanofw.NewNexusChainConfig(true)

	solanaConfig := cardanofw.NewSolanaChainConfig(true)

	apex := cardanofw.SetupAndRunSkylineBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithCardanoConfig(cardanoConfig),
		cardanofw.WithPrimeConfig(primeConfig),
		cardanofw.WithVectorConfig(vectorConfig),
		cardanofw.WithSolanaConfig(solanaConfig),
		cardanofw.WithNexusConfig(nexusConfig),
		cardanofw.WithUserCnt(1),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	user := apex.Users[0]

	solanaChain := apex.GetChainMust(t, cardanofw.ChainIDSolana).(*cardanofw.TestSolanaChain)

	wallet, err := solanawallet.NewWallet()
	require.NoError(t, err)

	sendAmount := cardanofw.LamportToWei(solanaConfig.MinBridgingAmount)

	nowSOLUser := &cardanofw.TestApexUser{
		HasSolanaWallet: true,
		SolanaAddress:   wallet.PublicKey.String(),
		SolanaWallet:    wallet,
	}

	_, err = solanaChain.SendTx(ctx, user.SolanaWallet.PrivateKey.String(), []byte{}, []cardanofw.GenericTxReceiver{
		{
			Addr:         nowSOLUser.SolanaAddress,
			Amount:       cardanofw.SolanaToWei(big.NewInt(5)),
			NativeTokens: nil,
		},
	}, 0)
	require.NoError(t, err)

	t.Run("1. user has no wSOL", func(t *testing.T) {
		_, err := solanaChain.BridgingRequest(cardanofw.BridgingRequestParams{
			Ctx:            ctx,
			DestChainID:    cardanofw.ChainIDVector,
			PrivateKey:     nowSOLUser.SolanaWallet.PrivateKey.String(),
			ChainIDsConfig: "",
			Receivers: map[string]cardanofw.ReceiverAmount{
				user.GetAddress(cardanofw.ChainIDVector): {
					TokenID: cardanofw.WSOLTokenID,
					Amount:  sendAmount,
				},
			},
			FeeAmount:      solanaConfig.MinBridgingFee,
			OperationFee:   solanaConfig.MinOperationFee,
			IsCurrencySrc:  false,
			IsCurrencyDest: false,
		})
		require.Error(t, err)
		require.ErrorContains(t, err, "AccountNotInitialized")
	})

	t.Run("2. amount is less than min bridging amount", func(t *testing.T) {
		_, err := solanaChain.BridgingRequest(cardanofw.BridgingRequestParams{
			Ctx:            ctx,
			DestChainID:    cardanofw.ChainIDVector,
			PrivateKey:     user.SolanaWallet.PrivateKey.String(),
			ChainIDsConfig: "",
			Receivers: map[string]cardanofw.ReceiverAmount{
				user.GetAddress(cardanofw.ChainIDVector): {
					TokenID: cardanofw.WSOLTokenID,
					Amount:  cardanofw.LamportToWei(new(big.Int).Sub(solanaConfig.MinBridgingAmount, big.NewInt(1))),
				},
			},
			FeeAmount:      solanaConfig.MinBridgingFee,
			OperationFee:   solanaConfig.MinOperationFee,
			IsCurrencySrc:  false,
			IsCurrencyDest: false,
		})
		require.Error(t, err)
		require.ErrorContains(t, err, "BridgingAmountTooLow")
	})

	t.Run("3. insufficient fee", func(t *testing.T) {
		_, err := solanaChain.BridgingRequest(cardanofw.BridgingRequestParams{
			Ctx:            ctx,
			DestChainID:    cardanofw.ChainIDVector,
			PrivateKey:     user.SolanaWallet.PrivateKey.String(),
			ChainIDsConfig: "",
			Receivers: map[string]cardanofw.ReceiverAmount{
				user.GetAddress(cardanofw.ChainIDVector): {
					TokenID: cardanofw.WSOLTokenID,
					Amount:  cardanofw.SolanaToWei(big.NewInt(1)),
				},
			},
			FeeAmount:      new(big.Int).Sub(solanaConfig.MinBridgingFee, big.NewInt(1)),
			OperationFee:   solanaConfig.MinOperationFee,
			IsCurrencySrc:  false,
			IsCurrencyDest: false,
		})
		require.Error(t, err)
		require.ErrorContains(t, err, "InsufficientFee")
	})

	t.Run("4. insufficient operation fee", func(t *testing.T) {
		_, err := solanaChain.BridgingRequest(cardanofw.BridgingRequestParams{
			Ctx:            ctx,
			DestChainID:    cardanofw.ChainIDVector,
			PrivateKey:     user.SolanaWallet.PrivateKey.String(),
			ChainIDsConfig: "",
			Receivers: map[string]cardanofw.ReceiverAmount{
				user.GetAddress(cardanofw.ChainIDVector): {
					TokenID: cardanofw.WSOLTokenID,
					Amount:  sendAmount,
				},
			},
			FeeAmount:      solanaConfig.MinBridgingFee,
			OperationFee:   new(big.Int).Sub(solanaConfig.MinOperationFee, big.NewInt(1)),
			IsCurrencySrc:  false,
			IsCurrencyDest: false,
		})
		require.Error(t, err)
		require.ErrorContains(t, err, "InsufficientFee")
	})

	t.Run("5. refund - invalid destination chain ID", func(t *testing.T) {
		userWSolBalance, err := apex.GetBalanceWithTokenName(ctx, user, cardanofw.ChainIDSolana, cardanofw.WSOLMintAddress)
		require.NoError(t, err)
		userSolBalance, err := apex.GetBalance(ctx, user, cardanofw.ChainIDSolana)
		require.NoError(t, err)
		fmt.Println("user SOL balance: ", cardanofw.WeiToLamport(userSolBalance[cardanowallet.AdaTokenName]))
		fmt.Println("user WSOL balance: ", cardanofw.WeiToLamport(userWSolBalance[cardanofw.WSOLMintAddress]))

		txSig, err := solanaChain.BridgingRequest(cardanofw.BridgingRequestParams{
			Ctx:            ctx,
			DestChainID:    "invalid",
			PrivateKey:     user.SolanaWallet.PrivateKey.String(),
			ChainIDsConfig: "",
			Receivers: map[string]cardanofw.ReceiverAmount{
				user.GetAddress(cardanofw.ChainIDVector): {
					TokenID: cardanofw.WSOLTokenID,
					Amount:  sendAmount,
				},
			},
			FeeAmount:      solanaConfig.MinBridgingFee,
			OperationFee:   solanaConfig.MinOperationFee,
			IsCurrencySrc:  false,
			IsCurrencyDest: false,
		})
		require.NoError(t, err)

		tokensInfo, err := apex.GetBridgingTokensInfo(cardanofw.ChainIDSolana, cardanofw.ChainIDVector, cardanofw.WSOLTokenID)
		require.NoError(t, err)

		waitForInvalidTestResultSol(t, ctx, apex, cardanofw.ChainIDSolana, tokensInfo, user, txSig, userWSolBalance, true, maxWaitTimeSec*2, retryIntervalSec)
		require.NoError(t, err)

		userWSolBalanceAfter, err := apex.GetBalanceWithTokenName(ctx, user, cardanofw.ChainIDSolana, cardanofw.WSOLMintAddress)
		require.NoError(t, err)
		userSolBalanceAfter, err := apex.GetBalance(ctx, user, cardanofw.ChainIDSolana)
		require.NoError(t, err)
		fmt.Println("user SOL balance after: ", cardanofw.WeiToLamport(userSolBalanceAfter[cardanowallet.AdaTokenName]))
		fmt.Println("user WSOL balance after: ", cardanofw.WeiToLamport(userWSolBalanceAfter[cardanofw.WSOLMintAddress]))
	})

	//nolint:dupl
	t.Run("6. refund - invalid destination address", func(t *testing.T) {
		userWSolBalance, err := apex.GetBalanceWithTokenName(ctx, user, cardanofw.ChainIDSolana, cardanofw.WSOLMintAddress)
		require.NoError(t, err)
		userSolBalance, err := apex.GetBalance(ctx, user, cardanofw.ChainIDSolana)
		require.NoError(t, err)
		fmt.Println("user SOL balance: ", cardanofw.WeiToLamport(userSolBalance[cardanowallet.AdaTokenName]))
		fmt.Println("user WSOL balance: ", cardanofw.WeiToLamport(userWSolBalance[cardanofw.WSOLMintAddress]))

		txSig, err := solanaChain.BridgingRequest(cardanofw.BridgingRequestParams{
			Ctx:            ctx,
			DestChainID:    cardanofw.ChainIDVector,
			PrivateKey:     user.SolanaWallet.PrivateKey.String(),
			ChainIDsConfig: "",
			Receivers: map[string]cardanofw.ReceiverAmount{
				user.GetAddress(cardanofw.ChainIDNexus): {
					TokenID: cardanofw.WSOLTokenID,
					Amount:  sendAmount,
				},
			},
			FeeAmount:      solanaConfig.MinBridgingFee,
			OperationFee:   solanaConfig.MinOperationFee,
			IsCurrencySrc:  false,
			IsCurrencyDest: false,
		})
		require.NoError(t, err)

		tokensInfo, err := apex.GetBridgingTokensInfo(cardanofw.ChainIDSolana, cardanofw.ChainIDVector, cardanofw.WSOLTokenID)
		require.NoError(t, err)

		waitForInvalidTestResultSol(t, ctx, apex, cardanofw.ChainIDSolana, tokensInfo, user, txSig, userWSolBalance, true, maxWaitTimeSec, retryIntervalSec)
		require.NoError(t, err)

		userWSolBalanceAfter, err := apex.GetBalanceWithTokenName(ctx, user, cardanofw.ChainIDSolana, cardanofw.WSOLMintAddress)
		require.NoError(t, err)
		userSolBalanceAfter, err := apex.GetBalance(ctx, user, cardanofw.ChainIDSolana)
		require.NoError(t, err)
		fmt.Println("user SOL balance after: ", cardanofw.WeiToLamport(userSolBalanceAfter[cardanowallet.AdaTokenName]))
		fmt.Println("user WSOL balance after: ", cardanofw.WeiToLamport(userWSolBalanceAfter[cardanofw.WSOLMintAddress]))
	})

	//nolint:dupl
	t.Run("7. refund - chain ID not in directions", func(t *testing.T) {
		userWSolBalance, err := apex.GetBalanceWithTokenName(ctx, user, cardanofw.ChainIDSolana, cardanofw.WSOLMintAddress)
		require.NoError(t, err)
		userSolBalance, err := apex.GetBalance(ctx, user, cardanofw.ChainIDSolana)
		require.NoError(t, err)
		fmt.Println("user SOL balance: ", cardanofw.WeiToLamport(userSolBalance[cardanowallet.AdaTokenName]))
		fmt.Println("user WSOL balance: ", cardanofw.WeiToLamport(userWSolBalance[cardanofw.WSOLMintAddress]))

		txSig, err := solanaChain.BridgingRequest(cardanofw.BridgingRequestParams{
			Ctx:            ctx,
			DestChainID:    cardanofw.ChainIDCardano,
			PrivateKey:     user.SolanaWallet.PrivateKey.String(),
			ChainIDsConfig: "",
			Receivers: map[string]cardanofw.ReceiverAmount{
				user.GetAddress(cardanofw.ChainIDCardano): {
					TokenID: cardanofw.WSOLTokenID,
					Amount:  sendAmount,
				},
			},
			FeeAmount:      solanaConfig.MinBridgingFee,
			OperationFee:   solanaConfig.MinOperationFee,
			IsCurrencySrc:  false,
			IsCurrencyDest: false,
		})
		require.NoError(t, err)

		tokensInfo, err := apex.GetBridgingTokensInfo(cardanofw.ChainIDSolana, cardanofw.ChainIDVector, cardanofw.WSOLTokenID)
		require.NoError(t, err)

		waitForInvalidTestResultSol(t, ctx, apex, cardanofw.ChainIDSolana, tokensInfo, user, txSig, userWSolBalance, true, maxWaitTimeSec, retryIntervalSec)
		require.NoError(t, err)

		userWSolBalanceAfter, err := apex.GetBalanceWithTokenName(ctx, user, cardanofw.ChainIDSolana, cardanofw.WSOLMintAddress)
		require.NoError(t, err)
		userSolBalanceAfter, err := apex.GetBalance(ctx, user, cardanofw.ChainIDSolana)
		require.NoError(t, err)
		fmt.Println("user SOL balance after: ", cardanofw.WeiToLamport(userSolBalanceAfter[cardanowallet.AdaTokenName]))
		fmt.Println("user WSOL balance after: ", cardanofw.WeiToLamport(userWSolBalanceAfter[cardanofw.WSOLMintAddress]))
	})

	t.Run("8. refund - wrong token for chain ID", func(t *testing.T) {
		nexusChain := apex.GetChainMust(t, cardanofw.ChainIDNexus).(*cardanofw.TestEVMChain)
		err = nexusChain.FundUsersWithToken(user.GetAddress(cardanofw.ChainIDNexus), cardanofw.DfmToWei(big.NewInt(400_000_000)), cardanofw.NSTokenID)
		require.NoError(t, err)

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, user, user, cardanofw.ChainIDNexus, cardanofw.ChainIDSolana, cardanofw.ApexToWei(big.NewInt(1)),
			cardanofw.NSTokenID, true)

		nstokenMint, ok := apex.SolanaInfo.Tokens[cardanofw.NSTokenID]
		require.True(t, ok)

		userNSBalanceBefore, err := apex.GetBalanceWithTokenName(ctx, user, cardanofw.ChainIDSolana, nstokenMint.ChainSpecific)
		require.NoError(t, err)
		fmt.Println("user NS balance before: ", userNSBalanceBefore)

		txSig, err := solanaChain.BridgingRequest(cardanofw.BridgingRequestParams{
			Ctx:            ctx,
			DestChainID:    cardanofw.ChainIDVector,
			PrivateKey:     user.SolanaWallet.PrivateKey.String(),
			ChainIDsConfig: "",
			Receivers: map[string]cardanofw.ReceiverAmount{
				user.GetAddress(cardanofw.ChainIDVector): {
					TokenID: cardanofw.NSTokenID,
					Amount:  sendAmount,
				},
			},
			FeeAmount:      solanaConfig.MinBridgingFee,
			OperationFee:   solanaConfig.MinOperationFee,
			IsCurrencySrc:  false,
			IsCurrencyDest: false,
		})
		require.NoError(t, err)

		userNSBalanceAfter, err := apex.GetBalanceWithTokenName(ctx, user, cardanofw.ChainIDSolana, nstokenMint.ChainSpecific)
		require.NoError(t, err)
		fmt.Println("user NS balance after: ", userNSBalanceAfter)

		tokensInfo, err := apex.GetBridgingTokensInfo(cardanofw.ChainIDSolana, cardanofw.ChainIDNexus, cardanofw.NSTokenID)
		require.NoError(t, err)

		waitForInvalidTestResultSol(t, ctx, apex, cardanofw.ChainIDSolana, tokensInfo, user, txSig, userNSBalanceBefore, true, maxWaitTimeSec, retryIntervalSec)
		require.NoError(t, err)
	})
}

func waitForInvalidTestResultSol(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, srcChainID cardanofw.ChainID,
	tokensInfo *cardanofw.BridgingTokensInfo, user *cardanofw.TestApexUser,
	txHash string, beforeSendingAmount map[string]*big.Int,
	refundEnabled bool, maxWaitTimeSec, retryIntervalSec uint,
) {
	t.Helper()

	retryIntervalSec = max(retryIntervalSec, 1)
	numRetries := max(1, int(maxWaitTimeSec/retryIntervalSec))

	if refundEnabled {
		fmt.Printf("Tx sent. hash: %s, beforeSendingAmount: %+v\n", txHash,
			beforeSendingAmount)

		err := apex.WaitForExactAmount(ctx, user, srcChainID, beforeSendingAmount[tokensInfo.SrcTokenName],
			numRetries, time.Second*time.Duration(retryIntervalSec), tokensInfo.SrcTokenName)
		require.NoError(t, err)
	} else {
		cardanofw.WaitForInvalidState(t, ctx, apex, srcChainID, txHash, apex.Config.APIKey, maxWaitTimeSec)
	}
}
