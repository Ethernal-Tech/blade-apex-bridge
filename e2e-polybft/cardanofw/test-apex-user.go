package cardanofw

import (
	"encoding/hex"
	"fmt"

	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/types"
	cardanowallet "github.com/Ethernal-Tech/cardano-infrastructure/wallet"
	solanawallet "github.com/Ethernal-Tech/solana-infrastructure/wallet"
)

type ApexNetworkTypes struct {
	Prime            cardanowallet.CardanoNetworkType
	Vector           cardanowallet.CardanoNetworkType
	IsVectorEnabled  bool
	Cardano          cardanowallet.CardanoNetworkType
	IsCardanoEnabled bool
	IsNexusEnabled   bool
	IsPolygonEnabled bool
	IsSolanaEnabled  bool
}

type ApexNetworkTypesParams struct {
	PrimeConfig   *TestCardanoChainConfig
	VectorConfig  *TestCardanoChainConfig
	CardanoConfig *TestCardanoChainConfig

	NexusConfig   *TestEVMChainConfig
	PolygonConfig *TestEVMChainConfig

	SolanaConfig *TestSolanaChainConfig
}

func NewApexNetworkTypes(p ApexNetworkTypesParams) *ApexNetworkTypes {
	var (
		vectorNetworkType, cardanoNetworkType                                                cardanowallet.CardanoNetworkType
		vectorIsEnabled, cardanoIsEnabled, nexusIsEnabled, polygonIsEnabled, solanaIsEnabled bool
	)

	if p.VectorConfig != nil {
		vectorNetworkType = p.VectorConfig.NetworkType
		vectorIsEnabled = p.VectorConfig.IsEnabled
	}

	if p.CardanoConfig != nil {
		cardanoNetworkType = p.CardanoConfig.NetworkType
		cardanoIsEnabled = p.CardanoConfig.IsEnabled
	}

	if p.NexusConfig != nil {
		nexusIsEnabled = p.NexusConfig.IsEnabled
	}

	if p.PolygonConfig != nil {
		polygonIsEnabled = p.PolygonConfig.IsEnabled
	}

	if p.SolanaConfig != nil {
		solanaIsEnabled = p.SolanaConfig.IsEnabled
	}

	return &ApexNetworkTypes{
		Prime:            p.PrimeConfig.NetworkType,
		Vector:           vectorNetworkType,
		IsVectorEnabled:  vectorIsEnabled,
		Cardano:          cardanoNetworkType,
		IsCardanoEnabled: cardanoIsEnabled,
		IsNexusEnabled:   nexusIsEnabled,
		IsPolygonEnabled: polygonIsEnabled,
		IsSolanaEnabled:  solanaIsEnabled,
	}
}

func NewApexNetworkTypesFromSystem(apex *ApexSystem) *ApexNetworkTypes {
	return NewApexNetworkTypes(ApexNetworkTypesParams{
		PrimeConfig:   apex.Config.PrimeConfig,
		VectorConfig:  apex.Config.VectorConfig,
		CardanoConfig: apex.Config.CardanoConfig,
		NexusConfig:   apex.Config.NexusConfig,
		PolygonConfig: apex.Config.PolygonConfig,
		SolanaConfig:  apex.Config.SolanaConfig,
	})
}

type apexUserWallets struct {
	Prime   *cardanowallet.Wallet
	Vector  *cardanowallet.Wallet
	Nexus   *crypto.ECDSAKey
	Cardano *cardanowallet.Wallet
	Polygon *crypto.ECDSAKey
	Solana  *solanawallet.Wallet
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

	HasPolygonWallet bool
	PolygonWallet    *crypto.ECDSAKey
	PolygonAddress   types.Address

	HasSolanaWallet bool
	SolanaWallet    *solanawallet.Wallet
	SolanaAddress   string
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
		polygonWallet      *crypto.ECDSAKey              = nil
		polygonUserAddress                               = types.Address{}
		solanaWallet       *solanawallet.Wallet          = nil
		solanaUserAddress                                = ""
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

	if networks.IsPolygonEnabled {
		polygonWallet, err = crypto.GenerateECDSAKey()
		if err != nil {
			return nil, err
		}

		polygonUserAddress = polygonWallet.Address()
	}

	if networks.IsSolanaEnabled {
		solanaWallet, err = solanawallet.NewWallet()
		if err != nil {
			return nil, err
		}

		solanaUserAddress = solanaWallet.PublicKey.String()
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
		PolygonWallet:    polygonWallet,
		PolygonAddress:   polygonUserAddress,
		HasPolygonWallet: networks.IsPolygonEnabled,
		HasSolanaWallet:  networks.IsSolanaEnabled,
		SolanaWallet:     solanaWallet,
		SolanaAddress:    solanaUserAddress,
	}, nil
}

func NewExistingTestApexUser(
	wallets *apexUserWallets,
	networks *ApexNetworkTypes,
) (*TestApexUser, error) {
	var (
		vectorUserAddress, cardanoUserAddress *cardanowallet.CardanoAddress
		nexusUserAddress, polygonUserAddress  types.Address
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

	if wallets.Polygon != nil && networks.IsPolygonEnabled {
		polygonUserAddress = wallets.Polygon.Address()
	}

	var solanaUserAddress string
	if wallets.Solana != nil && networks.IsSolanaEnabled {
		solanaUserAddress = wallets.Solana.PublicKey.String()
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
		PolygonWallet:    wallets.Polygon,
		PolygonAddress:   polygonUserAddress,
		HasPolygonWallet: wallets.Polygon != nil,
		HasSolanaWallet:  wallets.Solana != nil,
		SolanaWallet:     wallets.Solana,
		SolanaAddress:    solanaUserAddress,
	}, nil
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
	switch chain {
	case ChainIDNexus:
		return u.NexusWallet, u.NexusAddress
	case ChainIDPolygon:
		return u.PolygonWallet, u.PolygonAddress
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
	case ChainIDPolygon:
		if u.HasPolygonWallet {
			return u.PolygonAddress.String()
		}

		return ""
	case ChainIDSolana:
		if u.HasSolanaWallet {
			return u.SolanaAddress
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
	case ChainIDPolygon:
		if u.HasPolygonWallet {
			pkBytes, err := u.PolygonWallet.MarshallPrivateKey()
			if err != nil {
				return "", err
			}

			return hex.EncodeToString(pkBytes), nil
		}

		return "", fmt.Errorf("user doesn't have a polygon wallet")
	case ChainIDSolana:
		if u.HasSolanaWallet {
			return u.SolanaWallet.PrivateKey.String(), nil
		}

		return "", fmt.Errorf("user doesn't have a solana wallet")
	}

	return "", nil
}
