package cardanofw

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"sort"
	"sync"
	"testing"
	"time"

	infracommon "github.com/Ethernal-Tech/cardano-infrastructure/common"
	infrawallet "github.com/Ethernal-Tech/cardano-infrastructure/wallet"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

const (
	safeSlotOffset    = 100
	RedColorPrefix    = "\033[31m"
	ColorSuffix       = "\033[0m"
	GreenColorPrefix  = "\033[32m"
	YellowColorPrefix = "\033[33m"

	SlotsInEpoch         = 600
	slotLengthInSeconds  = 0.1
	EpochLengthInSeconds = SlotsInEpoch * slotLengthInSeconds
)

type IStakingComponent interface {
	GetGlobalExchangeRate() float32
	ChooseStakeAddressForStaking(stakeId string, amount uint64) string
	ChooseStakeAddressForUnstaking(amount uint64) map[string]uint64
}

var _ IStakingComponent = &StakingComponent{}

type StakingRequest struct {
	ID     string
	Amount uint64
}

type UnstakingRequest struct {
	Address string
	Amount  uint64
}

type StakingComponent struct {
	chain *TestCardanoChain
	ctx   context.Context
	t     *testing.T

	StakingWallet         *infrawallet.Wallet
	PaymentStakingAddress string
	StakingAddress        string
	FeePayerWallet        *infrawallet.Wallet
	FeePayerAddress       string

	TotalStakedAmount  uint64 // sum of total funds on wallets + rewards on wallets
	TotalWalletAmount  uint64
	TotalStAdaAmount   uint64
	TotalRewardBalance uint64
	globalExchangeRate float32
	PreviousEpoch      uint64

	Addresses []*StakingAddress
	mu        sync.RWMutex

	stakingRequestChan chan StakingRequest
	stakingRequest     map[string]uint64

	unstakingRequestChan chan UnstakingRequest
	unstakingRequest     map[string]bool
}

func NewStakingComponent(
	t *testing.T,
	ctx context.Context,
	chain *TestCardanoChain,
	addresses []*StakingAddress,
	globalExchangeRate float32,
	feePayer *infrawallet.Wallet,
	networkType infrawallet.CardanoNetworkType) *StakingComponent {
	stakingWallet, err := infrawallet.GenerateWallet(true)
	require.NoError(t, err)

	cliUtils := infrawallet.NewCliUtils(ResolveCardanoCliBinary(chain.config.NetworkType))
	paymentStakingAddress, stakingAddress, err := cliUtils.GetWalletAddress(stakingWallet.VerificationKey, stakingWallet.StakeVerificationKey, GetNetworkMagic(chain.config.NetworkType, chain.config.ChainType))
	require.NoError(t, err)

	feePayerAddress, _, err := cliUtils.GetWalletAddress(feePayer.VerificationKey, feePayer.StakeVerificationKey, GetNetworkMagic(chain.config.NetworkType, chain.config.ChainType))
	require.NoError(t, err)

	return &StakingComponent{
		chain: chain,
		t:     t,
		ctx:   ctx,

		StakingWallet:         stakingWallet,
		StakingAddress:        stakingAddress,
		PaymentStakingAddress: paymentStakingAddress,
		Addresses:             addresses,

		stakingRequestChan: make(chan StakingRequest, 10),
		stakingRequest:     make(map[string]uint64),

		unstakingRequestChan: make(chan UnstakingRequest, 10),
		unstakingRequest:     make(map[string]bool),

		FeePayerWallet:  feePayer,
		FeePayerAddress: feePayerAddress,

		TotalStakedAmount:  0,
		TotalWalletAmount:  0,
		TotalStAdaAmount:   0,
		TotalRewardBalance: 0,
		globalExchangeRate: 1,
		PreviousEpoch:      0,
	}
}

func (s *StakingComponent) StartStakingComponent() {
	txProvider, err := s.chain.GetTxProvider()
	require.NoError(s.t, err)

	go s.StartGlobalExchangeRateUpdater(s.ctx, txProvider)
	go s.WaitForStakingRequest(s.t, s.ctx, txProvider)
	go s.ProcessStakingRequest(s.t, s.ctx, s.chain)

	go s.WaitForUnstakingRequest(s.t, s.ctx, s.chain)
}

// ChooseStakePoolForStaking implements IStakingComponent.
func (s *StakingComponent) ChooseStakeAddressForStaking(stakeId string, amount uint64) string {
	// Choose the address with the lowest exchange rate
	// If there are multiple addresses with the same exchange rate, choose the one with the least staked amount
	minStakedAmount := uint64(math.MaxUint64)
	chosenAddress := ""
	for _, address := range s.Addresses {
		if address.TotalStakedAmount < minStakedAmount {
			minStakedAmount = address.TotalStakedAmount
			chosenAddress = address.PaymentAddress
		}
	}

	return chosenAddress
}

// ChooseStakeAddressForUnstaking implements IStakingComponent.
func (s *StakingComponent) ChooseStakeAddressForUnstaking(amount uint64) map[string]uint64 {
	sort.Slice(s.Addresses, func(i, j int) bool { return s.Addresses[i].TotalStakedAmount > s.Addresses[j].TotalStakedAmount })

	unstakedAmounts, remainingAmount := s.getUnstakeAmounts(amount)

	if remainingAmount > 0 {
		fmt.Println("unstakedAmounts", unstakedAmounts)
		fmt.Println("We have remaining amount after selecting addresses to unstake from: ", remainingAmount)

		// Iteratevly claim rewards for each address until amount is 0
		for _, address := range s.Addresses {
			if unstakedAmounts[address.PaymentAddress] > 0 {
				fmt.Println("Initiate withdraw from address: ", address.ID)
				err := address.WithdrawRewards(s.ctx, s.chain)
				if err != nil {
					fmt.Printf("%sERROR: failed to witdraw rewards for address %d: %e%s\n", RedColorPrefix, address.ID, err, ColorSuffix)
				}
			}

			unstakedAmounts, remainingAmount = s.getUnstakeAmounts(amount)

			if remainingAmount == 0 {
				break
			}
		}
	}

	return unstakedAmounts
}

func (s *StakingComponent) getUnstakeAmounts(amount uint64) (map[string]uint64, uint64) {
	unstakedAmounts := make(map[string]uint64)
	remainingAmount := amount
	for _, address := range s.Addresses {
		// We either have exactly enough amount to unstake from the address in its wallet
		// or we have more than enough to cover the unstaking
		// or we have enough to cover it with the rewards that we need to withdraw
		fmt.Println("Address: ", address.ID, "Wallet token amount: ", address.WalletTokenAmount, "Total staked amount: ", address.TotalStakedAmount, "Remaining amount: ", remainingAmount)
		if address.WalletTokenAmount == remainingAmount ||
			address.WalletTokenAmount > remainingAmount+MinUTxODefaultValue {
			unstakedAmounts[address.PaymentAddress] = address.WalletTokenAmount
			remainingAmount -= unstakedAmounts[address.PaymentAddress]
		} else if address.TotalStakedAmount == remainingAmount ||
			address.TotalStakedAmount > remainingAmount+MinUTxODefaultValue {
			unstakedAmounts[address.PaymentAddress] = address.WalletTokenAmount
			remainingAmount -= unstakedAmounts[address.PaymentAddress]
		}

		if remainingAmount == 0 {
			break
		}
	}

	return unstakedAmounts, remainingAmount
}

// GetGlobalExchangeRate implements IStakingComponent.
func (s *StakingComponent) GetGlobalExchangeRate() float32 {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.globalExchangeRate
}

func (s *StakingComponent) StartGlobalExchangeRateUpdater(ctx context.Context, txProvider infrawallet.ITxProvider) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second * 1):
				tip, err := infracommon.ExecuteWithRetry(ctx, func(ctx context.Context) (infrawallet.QueryTipData, error) {
					return txProvider.GetTip(ctx)
				})
				if err != nil {
					fmt.Printf("%sERROR: failed to retrieve chain tip, exchange rate not updated %e%s\n", RedColorPrefix, err, ColorSuffix)
					continue
				}

				// TODO: Hack, disscuss the correct way of implementation
				if s.PreviousEpoch == tip.Slot/SlotsInEpoch {
					continue
				}

				s.mu.Lock()
				s.globalExchangeRate, err = s.CalculateGlobalExchangeRate(ctx, txProvider)
				if err != nil {
					fmt.Printf("%sERROR: failed to calculate global exchange rate, exchange rate not updated %e%s\n", RedColorPrefix, err, ColorSuffix)
					s.mu.Unlock()
					continue
				}

				fmt.Printf("%sUpdated global exchange rate to: %f%s\n", YellowColorPrefix, s.globalExchangeRate, ColorSuffix)
				s.PreviousEpoch = tip.Slot / SlotsInEpoch
				s.mu.Unlock()
			}
		}

	}()
}

func (s *StakingComponent) CalculateGlobalExchangeRate(ctx context.Context, txProvider infrawallet.ITxProvider) (float32, error) {
	fmt.Println("Total staked amount: ", s.TotalStakedAmount)
	fmt.Println("Total stAda amount: ", s.TotalStAdaAmount)

	if s.TotalStAdaAmount == 0 {
		return 1.0, nil
	}

	var err error

	s.TotalRewardBalance, err = s.getTotalRewardsBalance(ctx, txProvider)
	if err != nil {
		return 0, err
	}

	s.TotalWalletAmount, err = s.getTotalWalletsBalance(ctx, txProvider)
	if err != nil {
		return 0, err
	}

	fmt.Println("Total wallets amount: ", s.TotalWalletAmount)
	fmt.Println("Total reward balance: ", s.TotalRewardBalance)
	s.TotalStakedAmount = s.TotalWalletAmount + s.TotalRewardBalance

	totalStakedAmount := decimal.NewFromUint64(s.TotalStakedAmount)
	totalStAdaAmount := decimal.NewFromUint64(s.TotalStAdaAmount)

	// Round it to 6 decimals
	exchangeRate, _ := totalStakedAmount.Div(totalStAdaAmount).Float64()
	return float32(exchangeRate), nil
}

func (s *StakingComponent) getTotalRewardsBalance(ctx context.Context, txProvider infrawallet.ITxProvider) (uint64, error) {
	totalRewardBalance := uint64(0)
	for _, address := range s.Addresses {
		rewardBalance, err := address.GetRewardBalance(ctx, txProvider)
		if err != nil {
			return 0, err
		}

		totalRewardBalance += rewardBalance
	}

	return totalRewardBalance, nil
}

func (s *StakingComponent) getTotalWalletsBalance(ctx context.Context, txProvider infrawallet.ITxProvider) (uint64, error) {
	totalWalletsBalance := uint64(0)
	for _, address := range s.Addresses {
		walletBalance, err := address.GetWalletTokenAmount(ctx, txProvider)
		if err != nil {
			return 0, err
		}

		totalWalletsBalance += walletBalance
	}

	return totalWalletsBalance, nil
}

func (s *StakingComponent) WaitForStakingRequest(t *testing.T, ctx context.Context, txProvider infrawallet.ITxProvider) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Millisecond * 300):
			utxos, err := txProvider.GetUtxos(ctx, s.PaymentStakingAddress)
			require.NoError(t, err)

			for _, utxo := range utxos {
				if _, ok := s.stakingRequest[utxo.Hash]; ok {
					continue
				}

				s.stakingRequest[utxo.Hash] = 0
				stakingRequest := StakingRequest{
					ID:     utxo.Hash,
					Amount: utxo.Amount,
				}

				fmt.Printf("%sStaking request for %d Ada, hash: %s initiated%s\n", GreenColorPrefix, stakingRequest.Amount, stakingRequest.ID, ColorSuffix)
				s.stakingRequestChan <- stakingRequest
			}
		}
	}
}

func (s *StakingComponent) NewUnstakingRequest(address string, adaAmount uint64) {
	unstakingRequest := UnstakingRequest{
		Address: address,
		Amount:  adaAmount,
	}

	fmt.Printf("%sNew unstaking request for %d stAda for address: %s initiated%s\n", RedColorPrefix, unstakingRequest.Amount, unstakingRequest.Address, ColorSuffix)
	s.unstakingRequest[address] = false
	s.unstakingRequestChan <- unstakingRequest
}

func (s *StakingComponent) WaitForUnstakingRequest(t *testing.T, ctx context.Context, chain *TestCardanoChain) {
	for {
		select {
		case <-ctx.Done():
			return
		case unstakingRequest := <-s.unstakingRequestChan:
			fmt.Println("Received unstaking request", unstakingRequest)
			amount := uint64(float32(unstakingRequest.Amount) * s.GetGlobalExchangeRate())
			adresses := s.ChooseStakeAddressForUnstaking(amount)
			fmt.Println("Adresses and amounts to unstake from: ", adresses)

			senderPrivateKeys := make([][]byte, len(adresses)+1)
			senderStakePrivateKeys := make([][]byte, len(adresses)+1)
			senderAmounts := make([]uint64, len(adresses)+1)
			for address, amount := range adresses {
				for i, stakingAddress := range s.Addresses {
					if stakingAddress.PaymentAddress == address {
						senderPrivateKeys[i] = stakingAddress.Wallet.SigningKey
						senderStakePrivateKeys[i] = stakingAddress.Wallet.StakeSigningKey
						senderAmounts[i] = amount
					}
				}
			}
			senderPrivateKeys[len(adresses)] = s.FeePayerWallet.SigningKey
			senderStakePrivateKeys[len(adresses)] = s.FeePayerWallet.StakeSigningKey
			senderAmounts[len(adresses)] = 0

			fmt.Println("Unstaking from addresses: ", adresses)
			for address := range adresses {
				for _, stakingAddress := range s.Addresses {
					if stakingAddress.PaymentAddress == address {
						fmt.Println("Unstaking from address: ", stakingAddress.ID)
						stakingAddress.NewUnstakingRequest(ctx, address, unstakingRequest.Amount, amount)
					}
				}
			}

			_, err := chain.SendTxWithFeePayer(ctx, senderPrivateKeys, senderStakePrivateKeys, senderAmounts, []string{unstakingRequest.Address}, []uint64{amount}, nil, s.FeePayerAddress)
			require.NoError(s.t, err)

			s.unstakingRequest[unstakingRequest.Address] = true

			fmt.Printf("%sUnstaking request for %d stAda for address: %s processed%s\n", RedColorPrefix, unstakingRequest.Amount, unstakingRequest.Address, ColorSuffix)
			s.TotalStAdaAmount -= unstakingRequest.Amount
			s.TotalStakedAmount -= uint64(float32(unstakingRequest.Amount) * s.GetGlobalExchangeRate())
		}
	}
}

func (s *StakingComponent) ProcessStakingRequest(t *testing.T, ctx context.Context, chain *TestCardanoChain) {
	for {
		select {
		case <-ctx.Done():
			return
		case stakingRequest := <-s.stakingRequestChan:
			time.Sleep(time.Second * 5)

			fmt.Println("Received staking request", stakingRequest)
			addressToStakeTo := s.ChooseStakeAddressForStaking(stakingRequest.ID, stakingRequest.Amount)
			fmt.Println("Chosen address to stake to", addressToStakeTo)
			txHash, err := chain.SendTxWithFeePayer(
				ctx,
				[][]byte{s.StakingWallet.SigningKey, s.FeePayerWallet.SigningKey},
				[][]byte{s.StakingWallet.StakeSigningKey, s.FeePayerWallet.StakeSigningKey},
				[]uint64{stakingRequest.Amount, 0},
				[]string{addressToStakeTo},
				[]uint64{stakingRequest.Amount},
				nil,
				s.FeePayerAddress,
			)
			require.NoError(t, err)

			txProvider, err := chain.GetTxProvider()
			require.NoError(t, err)

			exchangeRate := s.GetGlobalExchangeRate()
			done := false
			for {
				utxos, err := txProvider.GetUtxos(ctx, addressToStakeTo)
				require.NoError(t, err)
				for _, utxo := range utxos {
					if utxo.Hash == txHash {
						for _, stakingAddress := range s.Addresses {
							if stakingAddress.PaymentAddress == addressToStakeTo {
								stakingAddress.NewStakingRequest(ctx, stakingRequest.Amount, exchangeRate)
								done = true
								break
							}
						}
						break
					}
				}

				if done {
					break
				}

				time.Sleep(time.Millisecond * 500)
			}

			s.stakingRequest[stakingRequest.ID] = uint64(float32(stakingRequest.Amount) * exchangeRate)
			fmt.Printf("%sStaking request for %d Ada, hash: %s processed%s\n", GreenColorPrefix, stakingRequest.Amount, stakingRequest.ID, ColorSuffix)
			s.TotalStAdaAmount += stakingRequest.Amount
			s.TotalStakedAmount += uint64(float32(stakingRequest.Amount) * s.GetGlobalExchangeRate())
		}
	}
}

func (s *StakingComponent) GetStAdaAmount(t *testing.T, ctx context.Context, chain *TestCardanoChain, stakingRequest string) (stAdaAmount uint64) {
	return s.stakingRequest[stakingRequest]
}

func (s *StakingComponent) CheckIfUnstakingRequestIsProcessed(address string) bool {
	return s.unstakingRequest[address]
}

func (s *StakingComponent) PrintStakingComponentState() {
	fmt.Println("-------------------------------- Staking component --------------------------------")
	fmt.Println("Total staked amount:", s.TotalStakedAmount)
	fmt.Println("Total wallets amount:", s.TotalWalletAmount)
	fmt.Println("Total reward balance:", s.TotalRewardBalance)
	fmt.Println("Total stAda amount:", s.TotalStAdaAmount)
	fmt.Println("Exchange rate:", s.GetGlobalExchangeRate())
	fmt.Println("------------------------------------------------------------------------------------------")
	for _, address := range s.Addresses {
		address.PrintStakingAddressState()
	}
}

type StakingAddress struct {
	mu             sync.RWMutex
	ID             int
	Wallet         *infrawallet.Wallet
	StakeAddress   string
	PaymentAddress string

	TotalStakedAmount     uint64
	TotalStAdaAmount      uint64
	PreviousRewardBalance uint64
	WalletTokenAmount     uint64
}

func NewStakingAddress(t *testing.T, id int, wallet *infrawallet.Wallet, networkType infrawallet.CardanoNetworkType, chainType ChainID) *StakingAddress {
	cliUtils := infrawallet.NewCliUtils(ResolveCardanoCliBinary(networkType))
	paymentAddress, stakingAddress, err := cliUtils.GetWalletAddress(wallet.VerificationKey, wallet.StakeVerificationKey, GetNetworkMagic(networkType, chainType))
	require.NoError(t, err)

	return &StakingAddress{
		ID:             id,
		Wallet:         wallet,
		StakeAddress:   stakingAddress,
		PaymentAddress: paymentAddress,

		TotalStakedAmount:     0,
		TotalStAdaAmount:      0,
		PreviousRewardBalance: 0,
		WalletTokenAmount:     0,
	}
}

func (s *StakingAddress) PrintStakingAddressState() {
	fmt.Println("----------------- Staking address:", s.StakeAddress, "-----------------")
	fmt.Println("ID:", s.ID)
	fmt.Println("Total staked amount:", s.TotalStakedAmount)
	fmt.Println("Total stAda amount:", s.TotalStAdaAmount)
	fmt.Println("Previous reward balance:", s.PreviousRewardBalance)
	fmt.Println("Wallet token amount:", s.WalletTokenAmount)
	fmt.Println("------------------------------------------------------------------------------------------")
}

func (s *StakingAddress) GetRewardBalance(ctx context.Context, txProvider infrawallet.ITxProvider) (uint64, error) {
	stakeInfo, err := txProvider.GetStakeAddressInfo(ctx, s.StakeAddress)
	if err != nil {
		return 0, err
	}

	s.PreviousRewardBalance = stakeInfo.RewardAccountBalance
	s.TotalStakedAmount = stakeInfo.RewardAccountBalance + s.WalletTokenAmount
	return stakeInfo.RewardAccountBalance, nil
}

func (s *StakingAddress) NewStakingRequest(ctx context.Context, adaAmount uint64, globalExchangeRate float32) (stAdaAmount uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.TotalStakedAmount += adaAmount
	s.WalletTokenAmount += adaAmount
	stAdaAmount = uint64(float32(adaAmount) / globalExchangeRate)
	s.TotalStAdaAmount += stAdaAmount
	fmt.Printf("%sStaking address %d: Received %d Ada, total staked amount: %d, total stAda amount: %d, wallet token amount: %d%s\n", GreenColorPrefix, s.ID, adaAmount, s.TotalStakedAmount, s.TotalStAdaAmount, s.WalletTokenAmount, ColorSuffix)

	return stAdaAmount
}

func (s *StakingAddress) NewUnstakingRequest(ctx context.Context, address string, stAdaAmount uint64, adaAmount uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.TotalStAdaAmount = s.TotalStAdaAmount - stAdaAmount
	s.TotalStakedAmount -= adaAmount
	s.WalletTokenAmount -= adaAmount
	fmt.Printf("%sStaking address %d: Unstaking %d stAda, total staked amount: %d, total stAda amount: %d, wallet token amount: %d%s\n", RedColorPrefix, s.ID, stAdaAmount, s.TotalStakedAmount, s.TotalStAdaAmount, s.WalletTokenAmount, ColorSuffix)
}

func (s *StakingAddress) WithdrawRewards(ctx context.Context, chain *TestCardanoChain, receiverAddress ...string) error {
	txProvider, err := chain.GetTxProvider()
	if err != nil {
		return fmt.Errorf("failed to instantiate tx provider: %e", err)
	}

	// In some cases, the reward balance get's updated before the tx is sent
	// so we need to get the reward balance again after the tx is sent
	_, err = s.GetRewardBalance(ctx, txProvider)
	if err != nil {
		return fmt.Errorf("failed to get reward balance during withdraw process: %e", err)
	}

	if s.PreviousRewardBalance == 0 {
		return fmt.Errorf("no rewards to withdraw for address: %d", s.ID)
	}

	txHash, err := chain.SendWithdrawRewardsTx(ctx, s.Wallet.SigningKey, s.Wallet.StakeSigningKey, s.StakeAddress, s.PreviousRewardBalance, 0, receiverAddress...)
	if err != nil {
		return fmt.Errorf("failed to execute withdrawal tx: %e", err)
	}

	receiver := s.PaymentAddress
	if len(receiverAddress) > 0 {
		receiver = receiverAddress[0]
	}

	utxos, err := txProvider.GetUtxos(ctx, receiver)
	if err != nil {
		return fmt.Errorf("failed to query utxos after withdrawal tx: %e", err)
	}

	for _, utxo := range utxos {
		if utxo.Hash == txHash {
			fmt.Printf("%sWithdraw rewards tx hash: %s, withdraw amount: %d%s\n", YellowColorPrefix, txHash, utxo.Amount, ColorSuffix)
			break
		}
	}

	_, err = s.GetWalletTokenAmount(ctx, txProvider)
	if err != nil {
		return fmt.Errorf("failed to update wallet state after withdrawal tx: %e", err)
	}
	_, err = s.GetRewardBalance(ctx, txProvider)
	if err != nil {
		return fmt.Errorf("failed to updatereward state after withdrawal tx: %e", err)
	}

	s.TotalStakedAmount = s.WalletTokenAmount + s.PreviousRewardBalance
	return nil
}

func (s *StakingAddress) GetWalletTokenAmount(ctx context.Context, txProvider infrawallet.ITxProvider) (uint64, error) {
	utxos, err := txProvider.GetUtxos(ctx, s.PaymentAddress)
	if err != nil {
		return 0, err
	}

	s.WalletTokenAmount = infrawallet.GetUtxosSum(utxos)[infrawallet.AdaTokenName]
	return s.WalletTokenAmount, nil
}

type User struct {
	ID                        int
	Wallet                    *infrawallet.Wallet
	Address                   string
	TimeToWaitBeforeUnstaking time.Duration

	WalletAmount        uint64
	StakedAmount        uint64
	ReceivedStAdaAmount uint64

	NumberOfLifecycles int
}

func GenerateUser(t *testing.T, ctx context.Context, id int, sim *TestSimulation, stakingComponent *StakingComponent, numberOfLifecycles int, timeToWaitBeforeUnstaking time.Duration) *User {
	user := NewUser(t, sim.Chain.config.NetworkType, id, numberOfLifecycles, timeToWaitBeforeUnstaking)
	user.WalletAmount = 5200000 + uint64(rand.Intn(5000000))
	_, err := sim.Chain.SendSimpleTx(ctx, sim.AdminWallet.SigningKey, sim.AdminWallet.StakeSigningKey, []string{user.Address}, []uint64{user.WalletAmount}, nil, 0, false)
	require.NoError(t, err)

	return user
}

func NewUser(t *testing.T, networkType infrawallet.CardanoNetworkType, id int, numberOfLifecycles int, timeToWaitBeforeUnstaking time.Duration) *User {
	wallet, err := infrawallet.GenerateWallet(false)
	require.NoError(t, err)
	address, err := GetAddress(networkType, wallet)
	require.NoError(t, err)
	return &User{Wallet: wallet, Address: address.String(), ID: id, NumberOfLifecycles: numberOfLifecycles, TimeToWaitBeforeUnstaking: timeToWaitBeforeUnstaking}
}

func (u *User) GetAddress(t *testing.T, networkType infrawallet.CardanoNetworkType) string {
	address, err := GetAddress(networkType, u.Wallet)
	require.NoError(t, err)
	return address.String()
}

func (u *User) StartUserLifecycle(t *testing.T, ctx context.Context, chain *TestCardanoChain, stakingComponent *StakingComponent) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Millisecond * 200):
			if u.NumberOfLifecycles == 0 {
				fmt.Printf("User %d completed all lifecycles\n", u.ID)
				return
			}

			fmt.Printf("%sUser %d that has %d Ada sending staking request%s\n", GreenColorPrefix, u.ID, u.WalletAmount, ColorSuffix)

			definedFee := uint64(180_000)
			txHash, err := chain.SendSimpleTx(
				ctx,
				u.Wallet.SigningKey,
				nil,
				[]string{stakingComponent.PaymentStakingAddress},
				[]uint64{u.WalletAmount},
				nil,
				definedFee,
				true,
			)
			require.NoError(t, err)

			// Substract fee that staking address doesn't receive
			u.StakedAmount = u.WalletAmount - definedFee
			for {
				time.Sleep(time.Second * 1)
				stAdaAmount := stakingComponent.GetStAdaAmount(t, ctx, chain, txHash)
				if stAdaAmount > 0 {
					u.ReceivedStAdaAmount = stAdaAmount
					break
				}
			}
			fmt.Printf("User %d staked amount: %d, received stAda amount: %d\n", u.ID, u.StakedAmount, u.ReceivedStAdaAmount)

			time.Sleep(time.Duration(u.TimeToWaitBeforeUnstaking))

			txProvider, err := chain.GetTxProvider()
			require.NoError(t, err)

			stakingComponent.NewUnstakingRequest(u.Address, u.ReceivedStAdaAmount)

			expectedWalletBalance := uint64(stakingComponent.GetGlobalExchangeRate() * float32(u.ReceivedStAdaAmount))

			for {
				time.Sleep(time.Second * 2)

				if !stakingComponent.CheckIfUnstakingRequestIsProcessed(u.Address) {
					continue
				}

				utxos, err := txProvider.GetUtxos(ctx, u.Address)
				require.NoError(t, err)
				newWalletBalance := infrawallet.GetUtxosSum(utxos)["lovelace"]
				if newWalletBalance >= expectedWalletBalance {
					u.WalletAmount = newWalletBalance
					break
				}
			}

			if u.WalletAmount > u.StakedAmount {
				fmt.Printf("User %d unstaked amount: %d, received Ada amount: %d, earned Ada amount: %d\n", u.ID, u.ReceivedStAdaAmount, u.WalletAmount, u.WalletAmount-u.StakedAmount)
			} else if u.WalletAmount < u.StakedAmount {
				fmt.Printf("User %d unstaked amount: %d, received Ada amount: %d, and lost funds %d\n", u.ID, u.ReceivedStAdaAmount, u.WalletAmount, u.StakedAmount-u.WalletAmount)
			} else {
				fmt.Printf("User %d unstaked amount: %d, received Ada amount: %d, nothing changed\n", u.ID, u.ReceivedStAdaAmount, u.WalletAmount)
			}

			u.NumberOfLifecycles--
		}
	}
}
