package cardanofw

import (
	"context"
	"fmt"
	"math/big"
	"testing"
	"time"

	infracommon "github.com/Ethernal-Tech/cardano-infrastructure/common"
	sendtx "github.com/Ethernal-Tech/cardano-infrastructure/sendtx"
	infrawallet "github.com/Ethernal-Tech/cardano-infrastructure/wallet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	retryCount    = 90
	retryWaitTime = time.Second * 2

	ConfigNetworkType = infrawallet.TestNetNetwork
	ConfigChainType   = ChainIDCardano
)

type TestSimulation struct {
	Chain            *TestCardanoChain
	AdminWallet      *infrawallet.Wallet
	StakingAddresses []*StakingAddress
	FeePayer         *infrawallet.Wallet
}

func SetupAndRunSimulation(t *testing.T, ctx context.Context) *TestSimulation {
	chain := CreateCardanoChain()

	err := chain.RunChain(t)
	require.NoError(t, err)

	t.Cleanup(func() {
		assert.NoError(t, chain.Stop())
	})

	txProvider, err := chain.GetTxProvider()
	require.NoError(t, err)

	adminWallet, err := infrawallet.GenerateWallet(false)
	assert.NoError(t, err)
	// adminAddress, err := GetAddress(ConfigNetworkType, adminWallet)
	// assert.NoError(t, err)

	// 1. Create 4 wallets that will mimic 4 multisig staking wallets
	// 2. Fund wallets
	// 3. Register all wallets for stakeing
	stakingAddresses := []*StakingAddress{}
	amount := uint64(int64(defaultPremineAmount / 5))

	// Create and fund fee payer
	feePayer, err := infrawallet.GenerateWallet(false)
	require.NoError(t, err)
	feePayerAddress, err := GetAddress(ConfigNetworkType, feePayer)
	assert.NoError(t, err)
	err = chain.FundAddress(ctx, feePayerAddress.String(), amount)
	require.NoError(t, err)

	//cliUtils := infrawallet.NewCliUtils(ResolveCardanoCliBinary(chain.config.NetworkType))

	// for i := range 4 {
	// 	wallet, err := infrawallet.GenerateWallet(true)
	// 	assert.NoError(t, err)
	// 	stakingAddress := NewStakingAddress(t, i, wallet, ConfigNetworkType, ConfigChainType)
	// 	stakingAddresses = append(stakingAddresses, stakingAddress)
	//
	// 	paymentAddress, _, err := cliUtils.GetWalletAddress(wallet.VerificationKey, wallet.StakeVerificationKey, GetNetworkMagic(chain.config.NetworkType, chain.config.ChainType))
	// 	assert.NoError(t, err)
	//
	// 	err = chain.FundAddress(ctx, paymentAddress, amount)
	// 	require.NoError(t, err)
	//
	// 	err = RegisterAndDelegateStakeAddress(ctx, chain, wallet)
	// 	require.NoError(t, err)
	// }

	// 4. Get 4 existing stake pools of the network
	pools, err := txProvider.GetStakePools(ctx)
	require.NoError(t, err)
	require.Len(t, pools, 4)

	// 5. Delegate every address to some pool
	// 6. Return the rest of the funds to the admin
	wallets := make([]*infrawallet.Wallet, 0, len(stakingAddresses))
	for _, stakeAddress := range stakingAddresses {
		wallets = append(wallets, stakeAddress.Wallet)
	}

	err = RegisterAndDelegateStakeAddress(ctx, chain, wallets, pools, feePayer)
	assert.NoError(t, err)

	// address, err := GetAddress(ConfigNetworkType, stakingAddresses[i].Wallet)
	// assert.NoError(t, err)
	// utxos, err := txProvider.GetUtxos(ctx, address.String())
	// require.NoError(t, err)
	//
	// _, err = chain.SendSimpleTx(
	// 	ctx,
	// 	stakingAddresses[i].Wallet.SigningKey,
	// 	stakingAddresses[i].Wallet.StakeSigningKey,
	// 	[]string{adminAddress.String()},
	// 	[]uint64{infrawallet.GetUtxosSum(utxos)[infrawallet.AdaTokenName]},
	// 	nil,
	// 	0,
	// 	true,
	// )
	// require.NoError(t, err)
	//}

	return &TestSimulation{
		Chain:            chain,
		AdminWallet:      adminWallet,
		StakingAddresses: stakingAddresses,
		FeePayer:         feePayer,
	}
}

func CreateCardanoChain() *TestCardanoChain {
	config := &TestCardanoChainConfig{
		IsEnabled:              true,
		ID:                     0,
		NetworkType:            infrawallet.TestNetNetwork,
		NodesCount:             4,
		InitialHotWalletAmount: big.NewInt(0),
		PremineAmount:          defaultPremineAmount,
		FundAmount:             defaultFundTokenAmount,
		FundFeeAmount:          defaultFundTokenAmount,
		FundUTxOCount:          1,
		FundFeeUTxOCount:       1,
		ChainType:              ChainIDCardano,
		MinOperationFee:        200000,
	}

	ogmiosUrl := "http://localhost:1337"
	txSenderConfig := sendtx.ChainConfig{
		CardanoCliBinary: ResolveCardanoCliBinary(config.NetworkType),
		TxProvider:       infrawallet.NewTxProviderOgmios(ogmiosUrl),
		TestNetMagic:     GetNetworkMagic(config.NetworkType, config.ChainType),
		TTLSlotNumberInc: ttlSlotNumberInc,
		MinUtxoValue:     MinUTxODefaultValue,
		PotentialFee:     200_000,
	}

	return &TestCardanoChain{
		config:    config,
		ogmiosURL: ogmiosUrl,
		txSender:  sendtx.NewTxSender(map[string]sendtx.ChainConfig{config.ChainType: txSenderConfig}),
	}
}

func CreateAndStartStakingComponent(t *testing.T, ctx context.Context, chain *TestCardanoChain, stakingAddresses []*StakingAddress, feePayer *infrawallet.Wallet, networkType infrawallet.CardanoNetworkType) *StakingComponent {
	stakingComponent := NewStakingComponent(t, ctx, chain, stakingAddresses, 1.0, feePayer, networkType)
	stakingComponent.StartStakingComponent()
	return stakingComponent
}

/*
	func RegisterStakeAddress(ctx context.Context, chain *TestCardanoChain, wallet *infrawallet.Wallet) error {
		cliUtils := infrawallet.NewCliUtils(ResolveCardanoCliBinary(chain.config.NetworkType))
		_, stakeAddress, err := cliUtils.GetWalletAddress(wallet.VerificationKey, wallet.StakeVerificationKey, GetNetworkMagic(chain.config.NetworkType, chain.config.ChainType))
		if err != nil {
			return err
		}

		stakeRegistrationCert, err := cliUtils.CreateRegistrationCertificate(stakeAddress, 0)
		if err != nil {
			return err
		}

		// Ogmios doesn't help here since it can't recognize that address is registered
		// txProvider, err := chain.GetTxProvider()
		txProvider, err := infrawallet.NewTxProviderCli(
			GetNetworkMagic(chain.config.NetworkType, chain.config.ChainType),
			chain.cluster.OgmiosServer.SocketPath(),
			ResolveCardanoCliBinary(chain.config.NetworkType),
		)
		if err != nil {
			return err
		}

		txHash, err := chain.SendSimpleTx(
			ctx,
			wallet.SigningKey,
			wallet.StakeSigningKey,
			nil,
			nil,
			*stakeRegistrationCert,
			0,
			false,
		)
		if err != nil {
			return err
		}

		_, err = infracommon.ExecuteWithRetry(ctx, func(ctx context.Context) (string, error) {
			_, err := txProvider.GetStakeAddressInfo(ctx, stakeAddress)
			if err != nil {
				return "", infracommon.ErrRetryTryAgain
			}

			return txHash, nil
		}, infracommon.WithRetryCount(retryCount), infracommon.WithRetryWaitTime(retryWaitTime))
		if err != nil {
			fmt.Printf("failed to register stake address: %s", stakeAddress)
			return err
		}

		fmt.Printf("Stake address %s registered in tx: %s\n", stakeAddress, txHash)
		return nil
	}

	func DelegateStakeAddressToPool(ctx context.Context, chain *TestCardanoChain, wallet *infrawallet.Wallet, poolId string) error {
		cliUtils := infrawallet.NewCliUtils(ResolveCardanoCliBinary(chain.config.NetworkType))
		_, stakeAddress, err := cliUtils.GetWalletAddress(wallet.VerificationKey, wallet.StakeVerificationKey, GetNetworkMagic(chain.config.NetworkType, chain.config.ChainType))
		if err != nil {
			return err
		}

		stakeDelegationCert, err := cliUtils.CreateDelegationCertificate(stakeAddress, poolId)
		if err != nil {
			return err
		}

		txHash, err := chain.SendSimpleTx(
			ctx,
			wallet.SigningKey,
			wallet.StakeSigningKey,
			nil,
			nil,
			stakeDelegationCert,
			0,
			false,
		)
		if err != nil {
			return err
		}

		txProvider, err := chain.GetTxProvider()
		if err != nil {
			return err
		}

		_, err = infracommon.ExecuteWithRetry(ctx, func(ctx context.Context) (string, error) {
			res, err := txProvider.GetStakeAddressInfo(ctx, stakeAddress)
			if err != nil {
				return "", infracommon.ErrRetryTryAgain
			} else if res.StakeDelegation != poolId {
				return "", infracommon.ErrRetryTryAgain
			}

			return txHash, nil
		}, infracommon.WithRetryCount(retryCount), infracommon.WithRetryWaitTime(retryWaitTime))
		if err != nil {
			fmt.Printf("failed to delegate funds from %s to pool %s", stakeAddress, poolId)
			return err
		}

		fmt.Printf("Stake delegated to pool in tx: %s, address: %s\n", txHash, stakeAddress)
		return nil
	}
*/
/*
func RegisterAndDelegateStakeAddress(ctx context.Context, chain *TestCardanoChain, wallet *infrawallet.Wallet, poolId string) error {
	cliUtils := infrawallet.NewCliUtils(ResolveCardanoCliBinary(chain.config.NetworkType))
	_, stakeAddress, err := cliUtils.GetWalletAddress(wallet.VerificationKey, wallet.StakeVerificationKey, GetNetworkMagic(chain.config.NetworkType, chain.config.ChainType))
	if err != nil {
		return err
	}

	stakeRegistrationCert, err := cliUtils.CreateRegistrationCertificate(stakeAddress, 0)
	if err != nil {
		return err
	}

	stakeDelegationCert, err := cliUtils.CreateDelegationCertificate(stakeAddress, poolId)
	if err != nil {
		return err
	}

	txHash, err := chain.SendSimpleTx(
		ctx,
		wallet.SigningKey,
		wallet.StakeSigningKey,
		nil,
		nil,
		&sendtx.CertificatesWithScript{
			Certificates: []infrawallet.ICertificate{stakeRegistrationCert, stakeDelegationCert},
			PolicyScript: nil,
		},
		0,
		false,
	)
	if err != nil {
		return err
	}

	txProvider, err := chain.GetTxProvider()
	if err != nil {
		return err
	}

	_, err = infracommon.ExecuteWithRetry(ctx, func(ctx context.Context) (string, error) {
		res, err := txProvider.GetStakeAddressInfo(ctx, stakeAddress)
		if err != nil {
			return "", infracommon.ErrRetryTryAgain
		} else if res.StakeDelegation != poolId {
			return "", infracommon.ErrRetryTryAgain
		}

		return txHash, nil
	}, infracommon.WithRetryCount(retryCount), infracommon.WithRetryWaitTime(retryWaitTime))
	if err != nil {
		fmt.Printf("failed to delegate funds from %s to pool %s", stakeAddress, poolId)
		return err
	}

	fmt.Printf("Stake delegated to pool in tx: %s, address: %s\n", txHash, stakeAddress)
	return nil
}
*/

func RegisterAndDelegateStakeAddress(ctx context.Context, chain *TestCardanoChain, wallets []*infrawallet.Wallet, poolIDs []string, feeWallet *infrawallet.Wallet) error {
	cliUtils := infrawallet.NewCliUtils(ResolveCardanoCliBinary(chain.config.NetworkType))
	certs := make([]*sendtx.CertificatesWithScript, 0, len(poolIDs))
	stakeAddresses := make([]string, 0, len(poolIDs))

	for i, wallet := range wallets {
		_, stakeAddress, err := cliUtils.GetWalletAddress(wallet.VerificationKey, wallet.StakeVerificationKey, GetNetworkMagic(chain.config.NetworkType, chain.config.ChainType))
		if err != nil {
			return err
		}
		stakeAddresses = append(stakeAddresses, stakeAddress)

		stakeRegistrationCert, err := cliUtils.CreateRegistrationCertificate(stakeAddress, 0)
		if err != nil {
			return err
		}

		stakeDelegationCert, err := cliUtils.CreateDelegationCertificate(stakeAddress, poolIDs[i])
		if err != nil {
			return err
		}

		certs = append(certs, &sendtx.CertificatesWithScript{
			Certificates: []infrawallet.ICertificate{stakeRegistrationCert, stakeDelegationCert},
			PolicyScript: nil,
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
		wallets,
		certs,
		feeWallet,
	)
	if err != nil {
		return err
	}

	txProvider, err := chain.GetTxProvider()
	if err != nil {
		return err
	}

	for i, stakeAddress := range stakeAddresses {
		_, err = infracommon.ExecuteWithRetry(ctx, func(ctx context.Context) (string, error) {
			res, err := txProvider.GetStakeAddressInfo(ctx, stakeAddress)
			fmt.Println(res)
			if err != nil {
				return "", infracommon.ErrRetryTryAgain
			} else if res.StakeDelegation != poolIDs[i] {
				return "", infracommon.ErrRetryTryAgain
			}

			return txHash, nil
		}, infracommon.WithRetryCount(retryCount), infracommon.WithRetryWaitTime(retryWaitTime))
		if err != nil {
			fmt.Printf("failed to delegate funds from %s to pool %s", stakeAddress, poolIDs[i])
			return err
		}

		fmt.Printf("Stake delegated to pool in tx: %s, address: %s\n", txHash, stakeAddress)
	}

	return nil
}
