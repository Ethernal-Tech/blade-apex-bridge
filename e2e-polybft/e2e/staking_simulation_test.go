package e2e

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/e2e-polybft/cardanofw"
	"github.com/Ethernal-Tech/cardano-infrastructure/sendtx"
	infrawallet "github.com/Ethernal-Tech/cardano-infrastructure/wallet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// go test -timeout 0 -run ^Test_Simulation$ github.com/0xPolygon/polygon-edge/e2e-polybft/e2e -v
func Test_Simulation(t *testing.T) {
	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	sim := cardanofw.SetupAndRunSimulation(t, ctx)

	stakingComponent := cardanofw.CreateAndStartStakingComponent(t, ctx, sim.Chain, sim.StakingAddresses, sim.FeePayer, cardanofw.ConfigNetworkType)
	fmt.Println("Staking component started")

	userCount := 1
	var wg sync.WaitGroup

	for i := range userCount {
		wg.Add(1)
		user := cardanofw.GenerateUser(t, ctx, i, sim, stakingComponent, 1, time.Second*cardanofw.EpochLengthInSeconds*5)

		// Start user lifecycle with wait group
		go func(u *cardanofw.User) {
			defer wg.Done()
			u.StartUserLifecycle(t, ctx, sim.Chain, stakingComponent)
		}(user)

		time.Sleep(time.Second * time.Duration(rand.Intn(100)))
	}

	// Wait for all users to complete their lifecycles
	fmt.Println("Waiting for all users to complete their lifecycles...")
	wg.Wait()
	fmt.Println("All users completed their lifecycles")

	// Wait a bit to make sure all unstaking requests are processed
	// and all values are updated
	time.Sleep(time.Second * 5)
	stakingComponent.PrintStakingComponentState()
}

/*
func Test_Transactions(t *testing.T) {
	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	chain := cardanofw.CreateCardanoChain()

	err := chain.RunChain(t)
	require.NoError(t, err)

	t.Cleanup(func() {
		assert.NoError(t, chain.Stop())
	})

	// Create 2 addresses and fund one
	wallet1, err := infrawallet.GenerateWallet(false)
	require.NoError(t, err)
	wallet2, err := infrawallet.GenerateWallet(false)
	require.NoError(t, err)

	cliUtils := infrawallet.NewCliUtils(cardanofw.ResolveCardanoCliBinary(infrawallet.TestNetNetwork))
	address1, _, err := cliUtils.GetWalletAddress(wallet1.VerificationKey, wallet1.StakeVerificationKey, cardanofw.GetNetworkMagic(infrawallet.TestNetNetwork, cardanofw.ChainIDCardano))
	require.NoError(t, err)
	address2, _, err := cliUtils.GetWalletAddress(wallet2.VerificationKey, wallet2.StakeVerificationKey, cardanofw.GetNetworkMagic(infrawallet.TestNetNetwork, cardanofw.ChainIDCardano))
	require.NoError(t, err)

	chain.FundAddress(ctx, address1, 10_000_000)

	// 1 -> 1, sending partial amount
	_, err = chain.SendSimpleTx(ctx, wallet1.SigningKey, wallet1.StakeSigningKey, []string{address2}, []uint64{1_000_000}, nil, 0, false)
	require.NoError(t, err)

	// 1->1, sending full amount
	balance1, err := chain.GetAddressBalance(ctx, address1)
	require.NoError(t, err)
	_, err = chain.SendSimpleTx(ctx, wallet1.SigningKey, wallet1.StakeSigningKey, []string{address2}, []uint64{balance1[infrawallet.AdaTokenName].Uint64()}, nil, 0, true)
	require.NoError(t, err)

	balance1, err = chain.GetAddressBalance(ctx, address1)
	require.NoError(t, err)
	require.Equal(t, 0, len(balance1))

	balance2, err := chain.GetAddressBalance(ctx, address2)
	require.NoError(t, err)
	require.LessOrEqual(t, uint64(9_500_000), balance2[infrawallet.AdaTokenName].Uint64())

	addresses := []string{}
	wallet3, err := infrawallet.GenerateWallet(false)
	require.NoError(t, err)
	address3, _, err := cliUtils.GetWalletAddress(wallet3.VerificationKey, wallet3.StakeVerificationKey, cardanofw.GetNetworkMagic(infrawallet.TestNetNetwork, cardanofw.ChainIDCardano))
	require.NoError(t, err)
	addresses = append(addresses, address3)

	wallet4, err := infrawallet.GenerateWallet(false)
	require.NoError(t, err)
	address4, _, err := cliUtils.GetWalletAddress(wallet4.VerificationKey, wallet4.StakeVerificationKey, cardanofw.GetNetworkMagic(infrawallet.TestNetNetwork, cardanofw.ChainIDCardano))
	require.NoError(t, err)
	addresses = append(addresses, address4)

	wallet5, err := infrawallet.GenerateWallet(false)
	require.NoError(t, err)
	address5, _, err := cliUtils.GetWalletAddress(wallet5.VerificationKey, wallet5.StakeVerificationKey, cardanofw.GetNetworkMagic(infrawallet.TestNetNetwork, cardanofw.ChainIDCardano))
	require.NoError(t, err)
	addresses = append(addresses, address5)

	// 1 -> N, sending partial amount
	_, err = chain.SendSimpleTx(ctx, wallet2.SigningKey, wallet2.StakeSigningKey, addresses, []uint64{1_000_000, 1_000_000, 1_000_000}, nil, 0, false)
	require.NoError(t, err)

	for _, address := range addresses {
		balance, err := chain.GetAddressBalance(ctx, address)
		require.NoError(t, err)
		require.Equal(t, uint64(1_000_000), balance[infrawallet.AdaTokenName].Uint64())
	}

	// 1 -> N, sending full amount
	balance2, err = chain.GetAddressBalance(ctx, address2)
	require.NoError(t, err)
	require.LessOrEqual(t, uint64(6_000_000), balance2[infrawallet.AdaTokenName].Uint64())

	amountPerAddress := balance2[infrawallet.AdaTokenName].Uint64() / 3
	remainder := balance2[infrawallet.AdaTokenName].Uint64() % 3

	_, err = chain.SendSimpleTx(ctx, wallet2.SigningKey, wallet2.StakeSigningKey, addresses, []uint64{amountPerAddress, amountPerAddress, amountPerAddress + remainder}, nil, 0, true)
	require.NoError(t, err)
}

func Test_ComplexTransactions(t *testing.T) {
	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	chain := cardanofw.CreateCardanoChain()

	err := chain.RunChain(t)
	require.NoError(t, err)

	t.Cleanup(func() {
		assert.NoError(t, chain.Stop())
	})

	cliUtils := infrawallet.NewCliUtils(cardanofw.ResolveCardanoCliBinary(infrawallet.TestNetNetwork))

	// Create fee payer address and fund it
	feeWallet, err := infrawallet.GenerateWallet(false)
	require.NoError(t, err)
	feeAddress, _, err := cliUtils.GetWalletAddress(feeWallet.VerificationKey, feeWallet.StakeVerificationKey, cardanofw.GetNetworkMagic(infrawallet.TestNetNetwork, cardanofw.ChainIDCardano))
	require.NoError(t, err)

	err = chain.FundAddress(ctx, feeAddress, 10_000_000)
	require.NoError(t, err)

	// Create 2 addresses and fund one
	wallet1, err := infrawallet.GenerateWallet(false)
	require.NoError(t, err)
	wallet2, err := infrawallet.GenerateWallet(false)
	require.NoError(t, err)

	address1, _, err := cliUtils.GetWalletAddress(wallet1.VerificationKey, wallet1.StakeVerificationKey, cardanofw.GetNetworkMagic(infrawallet.TestNetNetwork, cardanofw.ChainIDCardano))
	require.NoError(t, err)
	address2, _, err := cliUtils.GetWalletAddress(wallet2.VerificationKey, wallet2.StakeVerificationKey, cardanofw.GetNetworkMagic(infrawallet.TestNetNetwork, cardanofw.ChainIDCardano))
	require.NoError(t, err)

	err = chain.FundAddress(ctx, address1, 10_000_000)
	require.NoError(t, err)
	balance1, err := chain.GetAddressBalance(ctx, address1)
	require.NoError(t, err)
	require.Equal(t, uint64(10_000_000), balance1[infrawallet.AdaTokenName].Uint64())
	require.NoError(t, err)

	// 1 -> 1, partial amount
	_, err = chain.SendTxWithFeePayer(ctx, [][]byte{wallet1.SigningKey, feeWallet.SigningKey}, [][]byte{wallet1.StakeSigningKey, feeWallet.StakeSigningKey}, []uint64{1_000_000, 0}, []string{address2}, []uint64{1_000_000}, feeAddress)
	require.NoError(t, err)

	balance2, err := chain.GetAddressBalance(ctx, address2)
	require.NoError(t, err)
	require.Equal(t, uint64(1_000_000), balance2[infrawallet.AdaTokenName].Uint64())

	balance1, err = chain.GetAddressBalance(ctx, address1)
	require.NoError(t, err)
	require.Equal(t, uint64(9_000_000), balance1[infrawallet.AdaTokenName].Uint64())

	addresses := []string{}
	wallet3, err := infrawallet.GenerateWallet(false)
	require.NoError(t, err)
	address3, _, err := cliUtils.GetWalletAddress(wallet3.VerificationKey, wallet3.StakeVerificationKey, cardanofw.GetNetworkMagic(infrawallet.TestNetNetwork, cardanofw.ChainIDCardano))
	require.NoError(t, err)
	addresses = append(addresses, address3)

	wallet4, err := infrawallet.GenerateWallet(false)
	require.NoError(t, err)
	address4, _, err := cliUtils.GetWalletAddress(wallet4.VerificationKey, wallet4.StakeVerificationKey, cardanofw.GetNetworkMagic(infrawallet.TestNetNetwork, cardanofw.ChainIDCardano))
	require.NoError(t, err)
	addresses = append(addresses, address4)

	wallet5, err := infrawallet.GenerateWallet(false)
	require.NoError(t, err)
	address5, _, err := cliUtils.GetWalletAddress(wallet5.VerificationKey, wallet5.StakeVerificationKey, cardanofw.GetNetworkMagic(infrawallet.TestNetNetwork, cardanofw.ChainIDCardano))
	require.NoError(t, err)
	addresses = append(addresses, address5)

	// N -> N, sending partial and full amount
	_, err = chain.SendTxWithFeePayer(ctx, [][]byte{wallet1.SigningKey, wallet2.SigningKey, feeWallet.SigningKey}, [][]byte{wallet1.StakeSigningKey, wallet2.StakeSigningKey, feeWallet.StakeSigningKey}, []uint64{2_000_000, 1_000_000, 0}, addresses, []uint64{1_000_000, 1_000_000, 1_000_000}, feeAddress)
	require.NoError(t, err)

	for _, address := range addresses {
		balance, err := chain.GetAddressBalance(ctx, address)
		require.NoError(t, err)
		require.Equal(t, uint64(1_000_000), balance[infrawallet.AdaTokenName].Uint64())
	}

	balance2, err = chain.GetAddressBalance(ctx, address2)
	require.NoError(t, err)
	require.Empty(t, balance2)

	balance1, err = chain.GetAddressBalance(ctx, address1)
	require.NoError(t, err)
	fmt.Println(balance1)
	require.Equal(t, uint64(7_000_000), balance1[infrawallet.AdaTokenName].Uint64())

	// 1 -> 1, full amount
	_, err = chain.SendTxWithFeePayer(ctx, [][]byte{wallet1.SigningKey, feeWallet.SigningKey}, [][]byte{wallet1.StakeSigningKey, feeWallet.StakeSigningKey}, []uint64{7_000_000, 0}, []string{address2}, []uint64{7_000_000}, feeAddress)
	require.NoError(t, err)

	balance2, err = chain.GetAddressBalance(ctx, address2)
	require.NoError(t, err)
	require.Equal(t, uint64(7_000_000), balance2[infrawallet.AdaTokenName].Uint64())

	balance1, err = chain.GetAddressBalance(ctx, address1)
	require.NoError(t, err)
	require.Empty(t, balance1)
}
*/

func Test_StakeMultipleMultisigAddressesInSignleTx(t *testing.T) {
	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	chain := cardanofw.CreateCardanoChain()

	err := chain.RunChain(t)
	require.NoError(t, err)

	t.Cleanup(func() {
		assert.NoError(t, chain.Stop())
	})

	cliUtils := infrawallet.NewCliUtils(cardanofw.ResolveCardanoCliBinary(infrawallet.TestNetNetwork))

	feeWallet, err := infrawallet.GenerateWallet(false)
	require.NoError(t, err)
	paymentAddress, _, err := cliUtils.GetWalletAddress(feeWallet.VerificationKey, feeWallet.StakeVerificationKey, cardanofw.GetNetworkMagic(infrawallet.TestNetNetwork, cardanofw.ChainIDCardano))
	require.NoError(t, err)
	err = chain.FundAddress(ctx, paymentAddress, 100_000_000)
	require.NoError(t, err)

	txProvider, err := chain.GetTxProvider()
	require.NoError(t, err)

	stakePools, err := txProvider.GetStakePools(ctx)
	require.NoError(t, err)
	require.Equal(t, 4, len(stakePools))

	numOfStakeAddresses := 25
	certs := make([]*sendtx.CertificatesWithScript, 0, numOfStakeAddresses)
	stakeAddresses := make([]string, 0, numOfStakeAddresses)
	allWallets := make([]*infrawallet.Wallet, 0, numOfStakeAddresses*numOfStakeAddresses)

	// Generate 4 multisig addresses:
	// 1. generate 4 wallets
	// 2. create policy scripts
	// 3. create certs
	for i := range numOfStakeAddresses {
		wallets := make([]*infrawallet.Wallet, 0, 4)
		//paymentKeyHashes := make([]string, 0, 4)
		stakeKeyHashes := make([]string, 0, 4)
		for range 4 {
			stakeWallet, err := infrawallet.GenerateWallet(true)
			require.NoError(t, err)
			wallets = append(wallets, stakeWallet)

			//paymentKeyHash, err := infrawallet.GetKeyHash(stakeWallet.VerificationKey)
			require.NoError(t, err)
			//paymentKeyHashes = append(paymentKeyHashes, paymentKeyHash)
			stakeKeyHash, err := infrawallet.GetKeyHash(stakeWallet.StakeVerificationKey)
			require.NoError(t, err)
			stakeKeyHashes = append(stakeKeyHashes, stakeKeyHash)
		}
		allWallets = append(allWallets, wallets...)

		//policyScriptPaymentMultiSig := infrawallet.NewPolicyScript(paymentKeyHashes, len(paymentKeyHashes)*2/3+1)
		policyScriptStakeMultiSig := infrawallet.NewPolicyScript(stakeKeyHashes, len(stakeKeyHashes)*2/3+1)

		// multisigPaymentPolicyID, err := cliUtils.GetPolicyID(policyScriptPaymentMultiSig)
		// require.NoError(t, err)

		multisigStakePolicyID, err := cliUtils.GetPolicyID(policyScriptStakeMultiSig)
		require.NoError(t, err)

		multiSigStakeAddr, err := infrawallet.NewPolicyScriptRewardAddress(infrawallet.TestNetNetwork, multisigStakePolicyID)
		require.NoError(t, err)
		stakeAddresses = append(stakeAddresses, multiSigStakeAddr.String())

		stakeRegistrationCert, err := cliUtils.CreateRegistrationCertificate(multiSigStakeAddr.String(), 2000000)
		require.NoError(t, err)

		stakeDelegationCert, err := cliUtils.CreateDelegationCertificate(multiSigStakeAddr.String(), stakePools[i%len(stakePools)])
		require.NoError(t, err)

		certs = append(certs, &sendtx.CertificatesWithScript{
			Certificates: []infrawallet.ICertificate{stakeRegistrationCert, stakeDelegationCert},
			PolicyScript: policyScriptStakeMultiSig,
		})
	}

	// txHash, err := chain.SendSimpleTx(
	// 	ctx,
	// 	wallets[0].SigningKey,
	// 	wallets[0].StakeSigningKey,
	// 	nil,
	// 	nil,
	// 	certs,
	// 	0,
	// 	false,
	// )

	txHash, err := chain.CreateRegAndDelegTx(
		ctx,
		allWallets,
		certs,
		feeWallet,
	)
	require.NoError(t, err)
	fmt.Println("addresses registrated and delegated in tx: ", txHash)

	for i, stakeAddress := range stakeAddresses {
		info, err := txProvider.GetStakeAddressInfo(ctx, stakeAddress)
		require.NoError(t, err)

		fmt.Println(info)
		require.Equal(t, stakePools[i%len(stakePools)], info.StakeDelegation)
	}

	fmt.Println(txProvider.GetUtxos(ctx, paymentAddress))
}

func Test_Withdraw(t *testing.T) {
	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	chain := cardanofw.CreateCardanoChain()

	err := chain.RunChain(t)
	require.NoError(t, err)

	t.Cleanup(func() {
		assert.NoError(t, chain.Stop())
	})

	cliUtils := infrawallet.NewCliUtils(cardanofw.ResolveCardanoCliBinary(infrawallet.TestNetNetwork))

	wallets := make([]*infrawallet.Wallet, 0, 4)
	stakedAddresses := make([]string, 0, 4)
	for range 4 {
		stakeWallet, err := infrawallet.GenerateWallet(true)
		require.NoError(t, err)
		wallets = append(wallets, stakeWallet)
		paymentAddress, stakeAddress, err := cliUtils.GetWalletAddress(stakeWallet.VerificationKey, stakeWallet.StakeVerificationKey, cardanofw.GetNetworkMagic(infrawallet.TestNetNetwork, cardanofw.ChainIDCardano))
		require.NoError(t, err)
		err = chain.FundAddress(ctx, paymentAddress, 100_000_000)
		require.NoError(t, err)
		stakedAddresses = append(stakedAddresses, stakeAddress)
	}

	feeWallet, err := infrawallet.GenerateWallet(false)
	require.NoError(t, err)
	paymentAddress, _, err := cliUtils.GetWalletAddress(feeWallet.VerificationKey, feeWallet.StakeVerificationKey, cardanofw.GetNetworkMagic(infrawallet.TestNetNetwork, cardanofw.ChainIDCardano))
	require.NoError(t, err)
	err = chain.FundAddress(ctx, paymentAddress, 10_000_000)
	require.NoError(t, err)

	txProvider, err := chain.GetTxProvider()
	require.NoError(t, err)

	stakePools, err := txProvider.GetStakePools(ctx)
	require.NoError(t, err)
	require.Equal(t, 4, len(stakePools))

	err = cardanofw.RegisterAndDelegateStakeAddress(ctx, chain, wallets, stakePools, feeWallet)
	require.NoError(t, err)

	// Check if addresses are staked to right pools:
	for i, stakeAddress := range stakedAddresses {
		info, err := txProvider.GetStakeAddressInfo(ctx, stakeAddress)
		require.NoError(t, err)

		require.Equal(t, stakePools[i], info.StakeDelegation)
	}
	/*
	   stakeBalance, err := chain.GetAddressBalance(ctx, paymentAddress)
	   require.NoError(t, err)
	   previousBalance := stakeBalance[infrawallet.AdaTokenName].Uint64()

	   	// Send rewards to stake address - tx fee
	   	rewardAmount := uint64(0)
	   	for {
	   		info, err := txProvider.GetStakeAddressInfo(ctx, stakeAddress)
	   		require.NoError(t, err)

	   		fmt.Println(info)
	   		if info.RewardAccountBalance > cardanofw.MinUTxODefaultValue {
	   			rewardAmount = info.RewardAccountBalance
	   			break
	   		}

	   		time.Sleep(time.Second * cardanofw.EpochLengthInSeconds)
	   	}

	   	_, err = chain.SendWithdrawRewardsTx(ctx, stakeWallet.SigningKey, stakeWallet.StakeSigningKey, stakeAddress, rewardAmount, 0)
	   	require.NoError(t, err)

	   	stakeBalance, err = chain.GetAddressBalance(ctx, paymentAddress)
	   	require.NoError(t, err)
	   	// Had to pay fee so the values are slightly off
	   	require.GreaterOrEqual(t, previousBalance+rewardAmount, stakeBalance[infrawallet.AdaTokenName].Uint64())

	   	wallet, err := infrawallet.GenerateWallet(false)
	   	require.NoError(t, err)
	   	address, _, err := cliUtils.GetWalletAddress(wallet.VerificationKey, wallet.StakeVerificationKey, cardanofw.GetNetworkMagic(infrawallet.TestNetNetwork, cardanofw.ChainIDCardano))
	   	require.NoError(t, err)

	   	// Send rewards to some other address
	   	rewardAmount = uint64(0)
	   	for {
	   		info, err := txProvider.GetStakeAddressInfo(ctx, stakeAddress)
	   		require.NoError(t, err)

	   		fmt.Println(info)
	   		if info.RewardAccountBalance > cardanofw.MinUTxODefaultValue {
	   			rewardAmount = info.RewardAccountBalance
	   			break
	   		}

	   		time.Sleep(time.Second * cardanofw.EpochLengthInSeconds / 2)
	   	}

	   	_, err = chain.SendWithdrawRewardsTx(ctx, stakeWallet.SigningKey, stakeWallet.StakeSigningKey, stakeAddress, rewardAmount, 0, address)
	   	require.NoError(t, err)
	   	balance, err := chain.GetAddressBalance(ctx, address)
	   	require.NoError(t, err)
	   	require.Equal(t, rewardAmount, balance[infrawallet.AdaTokenName].Uint64())
	*/
}
