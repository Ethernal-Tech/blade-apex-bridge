package cardanofw

import (
	"encoding/hex"
	"fmt"

	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/types"
	cardanowallet "github.com/Ethernal-Tech/cardano-infrastructure/wallet"
)

type ApexNetworkTypes struct {
	Prime            cardanowallet.CardanoNetworkType
	Vector           cardanowallet.CardanoNetworkType
	IsVectorEnabled  bool
	Cardano          cardanowallet.CardanoNetworkType
	IsCardanoEnabled bool
	IsNexusEnabled   bool
}

func NewApexNetworkTypes(
	primeConfig, vectorConfig, cardanoConfig *TestCardanoChainConfig,
	nexusConfig *TestEVMChainConfig,
) *ApexNetworkTypes {
	var (
		vectorNetworkType, cardanoNetworkType             cardanowallet.CardanoNetworkType
		vectorIsEnabled, cardanoIsEnabled, nexusIsEnabled bool
	)

	if vectorConfig != nil {
		vectorNetworkType = vectorConfig.NetworkType
		vectorIsEnabled = vectorConfig.IsEnabled
	}

	if cardanoConfig != nil {
		cardanoNetworkType = cardanoConfig.NetworkType
		cardanoIsEnabled = cardanoConfig.IsEnabled
	}

	if nexusConfig != nil {
		nexusIsEnabled = nexusConfig.IsEnabled
	}

	return &ApexNetworkTypes{
		Prime:            primeConfig.NetworkType,
		Vector:           vectorNetworkType,
		IsVectorEnabled:  vectorIsEnabled,
		Cardano:          cardanoNetworkType,
		IsCardanoEnabled: cardanoIsEnabled,
		IsNexusEnabled:   nexusIsEnabled,
	}
}

func NewApexNetworkTypesFromSystem(apex *ApexSystem) *ApexNetworkTypes {
	return NewApexNetworkTypes(
		apex.Config.PrimeConfig, apex.Config.VectorConfig, apex.Config.CardanoConfig, apex.Config.NexusConfig)
}

type apexUserWallets struct {
	Prime   *cardanowallet.Wallet
	Vector  *cardanowallet.Wallet
	Nexus   *crypto.ECDSAKey
	Cardano *cardanowallet.Wallet
}

type TestApexUser struct {
	PrimeWallet  *cardanowallet.Wallet
	PrimeAddress *cardanowallet.CardanoAddress

	HasVectorWallet bool
	VectorWallet    *cardanowallet.Wallet
	VectorAddress   *cardanowallet.CardanoAddress

	HasCardanoWallet bool
	CardanoWallet    *cardanowallet.Wallet
	CardanoAddress   *cardanowallet.CardanoAddress

	HasNexusWallet bool
	NexusWallet    *crypto.ECDSAKey
	NexusAddress   types.Address
}

func NewTestApexUser(
	networks *ApexNetworkTypes,
) (*TestApexUser, error) {
	var (
		vectorWallet       *cardanowallet.Wallet         = nil
		vectorUserAddress  *cardanowallet.CardanoAddress = nil
		cardanoWallet      *cardanowallet.Wallet         = nil
		cardanoUserAddress *cardanowallet.CardanoAddress = nil
		nexusWallet        *crypto.ECDSAKey              = nil
		nexusUserAddress                                 = types.Address{}
	)

	primeWallet, err := cardanowallet.GenerateWallet(false)
	if err != nil {
		return nil, err
	}

	primeUserAddress, err := GetAddress(networks.Prime, primeWallet)
	if err != nil {
		return nil, err
	}

	if networks.IsVectorEnabled {
		vectorWallet, err = cardanowallet.GenerateWallet(false)
		if err != nil {
			return nil, err
		}

		vectorUserAddress, err = GetAddress(networks.Vector, vectorWallet)
		if err != nil {
			return nil, err
		}
	}

	if networks.IsCardanoEnabled {
		cardanoWallet, err = cardanowallet.GenerateWallet(false)
		if err != nil {
			return nil, err
		}

		cardanoUserAddress, err = GetAddress(networks.Cardano, cardanoWallet)
		if err != nil {
			return nil, err
		}
	}

	if networks.IsNexusEnabled {
		nexusWallet, err = crypto.GenerateECDSAKey()
		if err != nil {
			return nil, err
		}

		nexusUserAddress = nexusWallet.Address()
	}

	return &TestApexUser{
		PrimeWallet:      primeWallet,
		PrimeAddress:     primeUserAddress,
		VectorWallet:     vectorWallet,
		VectorAddress:    vectorUserAddress,
		HasVectorWallet:  networks.IsVectorEnabled,
		CardanoWallet:    cardanoWallet,
		CardanoAddress:   cardanoUserAddress,
		HasCardanoWallet: networks.IsCardanoEnabled,
		NexusWallet:      nexusWallet,
		NexusAddress:     nexusUserAddress,
		HasNexusWallet:   networks.IsNexusEnabled,
	}, nil
}

func NewExistingTestApexUser(
	wallets *apexUserWallets,
	networks *ApexNetworkTypes,
) (*TestApexUser, error) {
	var (
		vectorUserAddress, cardanoUserAddress *cardanowallet.CardanoAddress
		nexusUserAddress                      types.Address
	)

	primeUserAddress, err := GetAddress(networks.Prime, wallets.Prime)
	if err != nil {
		return nil, err
	}

	if wallets.Vector != nil && networks.IsVectorEnabled {
		vectorUserAddress, err = GetAddress(networks.Vector, wallets.Vector)
		if err != nil {
			return nil, err
		}
	}

	if wallets.Nexus != nil && networks.IsNexusEnabled {
		nexusUserAddress = wallets.Nexus.Address()
	}

	if wallets.Cardano != nil && networks.IsCardanoEnabled {
		cardanoUserAddress, err = GetAddress(networks.Cardano, wallets.Cardano)
		if err != nil {
			return nil, err
		}
	}

	return &TestApexUser{
		PrimeWallet:      wallets.Prime,
		PrimeAddress:     primeUserAddress,
		VectorWallet:     wallets.Vector,
		VectorAddress:    vectorUserAddress,
		HasVectorWallet:  wallets.Vector != nil,
		NexusWallet:      wallets.Nexus,
		NexusAddress:     nexusUserAddress,
		HasNexusWallet:   wallets.Nexus != nil,
		CardanoWallet:    wallets.Cardano,
		CardanoAddress:   cardanoUserAddress,
		HasCardanoWallet: wallets.Cardano != nil,
	}, nil
}

func NewApexUserTesting(addr string) (*TestApexUser, error) {
	address, err := cardanowallet.NewCardanoAddressFromString(addr)

	return &TestApexUser{
		HasCardanoWallet: true,
		CardanoAddress:   address,
	}, err
}

func (u *TestApexUser) GetCardanoWallet(chain ChainID) (
	*cardanowallet.Wallet, *cardanowallet.CardanoAddress,
) {
	switch chain {
	case ChainIDPrime:
		return u.PrimeWallet, u.PrimeAddress
	case ChainIDVector:
		return u.VectorWallet, u.VectorAddress
	case ChainIDCardano:
		return u.CardanoWallet, u.CardanoAddress
	}

	return nil, nil
}

func (u *TestApexUser) GetEvmWallet(chain ChainID) (
	*crypto.ECDSAKey, types.Address,
) {
	if chain == ChainIDNexus {
		return u.NexusWallet, u.NexusAddress
	}

	return nil, types.Address{}
}

func (u *TestApexUser) GetAddress(chain ChainID) string {
	switch chain {
	case ChainIDPrime:
		return u.PrimeAddress.String()
	case ChainIDVector:
		if u.HasVectorWallet {
			return u.VectorAddress.String()
		}

		return ""
	case ChainIDCardano:
		if u.HasCardanoWallet {
			return u.CardanoAddress.String()
		}

		return ""
	case ChainIDNexus:
		if u.HasNexusWallet {
			return u.NexusAddress.String()
		}

		return ""
	}

	return ""
}

func (u *TestApexUser) GetPrivateKey(chain ChainID) (string, error) {
	switch chain {
	case ChainIDPrime:
		return ToCardanoPrivateKeyString(u.PrimeWallet.SigningKey, u.PrimeWallet.StakeSigningKey), nil
	case ChainIDVector:
		if u.HasVectorWallet {
			return ToCardanoPrivateKeyString(u.VectorWallet.SigningKey, u.VectorWallet.StakeSigningKey), nil
		}

		return "", fmt.Errorf("user doesn't have a vector wallet")
	case ChainIDCardano:
		if u.HasCardanoWallet {
			return ToCardanoPrivateKeyString(u.CardanoWallet.SigningKey, u.CardanoWallet.StakeSigningKey), nil
		}

		return "", fmt.Errorf("user doesn't have a cardano wallet")
	case ChainIDNexus:
		if u.HasNexusWallet {
			pkBytes, err := u.NexusWallet.MarshallPrivateKey()
			if err != nil {
				return "", err
			}

			return hex.EncodeToString(pkBytes), nil
		}

		return "", fmt.Errorf("user doesn't have a nexus wallet")
	}

	return "", nil
}
