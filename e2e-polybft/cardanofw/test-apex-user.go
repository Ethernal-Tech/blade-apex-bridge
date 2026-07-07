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
	Prime             cardanowallet.CardanoNetworkType
	Vector            cardanowallet.CardanoNetworkType
	IsVectorEnabled   bool
	Cardano           cardanowallet.CardanoNetworkType
	IsCardanoEnabled  bool
	IsNexusEnabled    bool
	IsPolygonEnabled  bool
	IsEthereumEnabled bool
	IsKatanaEnabled   bool
	IsSeiEnabled      bool
	IsArbitrumEnabled bool
	IsScrollEnabled   bool
	IsUnichainEnabled bool
	IsSolanaEnabled   bool
}

type ApexNetworkTypesParams struct {
	PrimeConfig   *TestCardanoChainConfig
	VectorConfig  *TestCardanoChainConfig
	CardanoConfig *TestCardanoChainConfig

	NexusConfig    *TestEVMChainConfig
	PolygonConfig  *TestEVMChainConfig
	EthereumConfig *TestEVMChainConfig
	KatanaConfig   *TestEVMChainConfig
	SeiConfig      *TestEVMChainConfig
	ArbitrumConfig *TestEVMChainConfig
	ScrollConfig   *TestEVMChainConfig
	UnichainConfig *TestEVMChainConfig

	SolanaConfig *TestSolanaChainConfig
}

func NewApexNetworkTypes(p ApexNetworkTypesParams) *ApexNetworkTypes {
	var (
		vectorNetworkType, cardanoNetworkType              cardanowallet.CardanoNetworkType
		vectorIsEnabled, cardanoIsEnabled, solanaIsEnabled bool
	)

	if p.VectorConfig != nil {
		vectorNetworkType = p.VectorConfig.NetworkType
		vectorIsEnabled = p.VectorConfig.IsEnabled
	}

	if p.CardanoConfig != nil {
		cardanoNetworkType = p.CardanoConfig.NetworkType
		cardanoIsEnabled = p.CardanoConfig.IsEnabled
	}

	if p.SolanaConfig != nil {
		solanaIsEnabled = p.SolanaConfig.IsEnabled
	}

	return &ApexNetworkTypes{
		Prime:             p.PrimeConfig.NetworkType,
		Vector:            vectorNetworkType,
		IsVectorEnabled:   vectorIsEnabled,
		Cardano:           cardanoNetworkType,
		IsCardanoEnabled:  cardanoIsEnabled,
		IsNexusEnabled:    p.NexusConfig != nil && p.NexusConfig.IsEnabled,
		IsPolygonEnabled:  p.PolygonConfig != nil && p.PolygonConfig.IsEnabled,
		IsEthereumEnabled: p.EthereumConfig != nil && p.EthereumConfig.IsEnabled,
		IsKatanaEnabled:   p.KatanaConfig != nil && p.KatanaConfig.IsEnabled,
		IsSeiEnabled:      p.SeiConfig != nil && p.SeiConfig.IsEnabled,
		IsArbitrumEnabled: p.ArbitrumConfig != nil && p.ArbitrumConfig.IsEnabled,
		IsScrollEnabled:   p.ScrollConfig != nil && p.ScrollConfig.IsEnabled,
		IsUnichainEnabled: p.UnichainConfig != nil && p.UnichainConfig.IsEnabled,
		IsSolanaEnabled:   solanaIsEnabled,
	}
}

func NewApexNetworkTypesFromSystem(apex *ApexSystem) *ApexNetworkTypes {
	return NewApexNetworkTypes(ApexNetworkTypesParams{
		PrimeConfig:    apex.Config.PrimeConfig,
		VectorConfig:   apex.Config.VectorConfig,
		CardanoConfig:  apex.Config.CardanoConfig,
		NexusConfig:    apex.Config.NexusConfig,
		PolygonConfig:  apex.Config.PolygonConfig,
		EthereumConfig: apex.Config.EthereumConfig,
		KatanaConfig:   apex.Config.KatanaConfig,
		SeiConfig:      apex.Config.SeiConfig,
		ArbitrumConfig: apex.Config.ArbitrumConfig,
		ScrollConfig:   apex.Config.ScrollConfig,
		UnichainConfig: apex.Config.UnichainConfig,
		SolanaConfig:   apex.Config.SolanaConfig,
	})
}

type apexUserWallets struct {
	Prime    *cardanowallet.Wallet
	Vector   *cardanowallet.Wallet
	Nexus    *crypto.ECDSAKey
	Cardano  *cardanowallet.Wallet
	Polygon  *crypto.ECDSAKey
	Ethereum *crypto.ECDSAKey
	Katana   *crypto.ECDSAKey
	Sei      *crypto.ECDSAKey
	Arbitrum *crypto.ECDSAKey
	Scroll   *crypto.ECDSAKey
	Unichain *crypto.ECDSAKey
	Solana   *solanawallet.Wallet
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

	HasEthereumWallet bool
	EthereumWallet    *crypto.ECDSAKey
	EthereumAddress   types.Address

	HasKatanaWallet bool
	KatanaWallet    *crypto.ECDSAKey
	KatanaAddress   types.Address

	HasSeiWallet bool
	SeiWallet    *crypto.ECDSAKey
	SeiAddress   types.Address

	HasArbitrumWallet bool
	ArbitrumWallet    *crypto.ECDSAKey
	ArbitrumAddress   types.Address

	HasScrollWallet bool
	ScrollWallet    *crypto.ECDSAKey
	ScrollAddress   types.Address

	HasUnichainWallet bool
	UnichainWallet    *crypto.ECDSAKey
	UnichainAddress   types.Address

	HasSolanaWallet bool
	SolanaWallet    *solanawallet.Wallet
	SolanaAddress   string
}

func NewTestApexUser(
	networks *ApexNetworkTypes,
) (*TestApexUser, error) {
	var (
		vectorWallet        *cardanowallet.Wallet         = nil
		vectorUserAddress   *cardanowallet.CardanoAddress = nil
		cardanoWallet       *cardanowallet.Wallet         = nil
		cardanoUserAddress  *cardanowallet.CardanoAddress = nil
		nexusWallet         *crypto.ECDSAKey              = nil
		nexusUserAddress                                  = types.Address{}
		polygonWallet       *crypto.ECDSAKey              = nil
		polygonUserAddress                                = types.Address{}
		ethereumWallet      *crypto.ECDSAKey              = nil
		ethereumUserAddress                               = types.Address{}
		katanaWallet        *crypto.ECDSAKey              = nil
		katanaUserAddress                                 = types.Address{}
		seiWallet           *crypto.ECDSAKey              = nil
		seiUserAddress                                    = types.Address{}
		arbitrumWallet      *crypto.ECDSAKey              = nil
		arbitrumUserAddress                               = types.Address{}
		scrollWallet        *crypto.ECDSAKey              = nil
		scrollUserAddress                                 = types.Address{}
		unichainWallet      *crypto.ECDSAKey              = nil
		unichainUserAddress                               = types.Address{}
		solanaWallet        *solanawallet.Wallet          = nil
		solanaUserAddress                                 = ""
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

	if networks.IsEthereumEnabled {
		ethereumWallet, err = crypto.GenerateECDSAKey()
		if err != nil {
			return nil, err
		}

		ethereumUserAddress = ethereumWallet.Address()
	}

	if networks.IsKatanaEnabled {
		katanaWallet, err = crypto.GenerateECDSAKey()
		if err != nil {
			return nil, err
		}

		katanaUserAddress = katanaWallet.Address()
	}

	if networks.IsSeiEnabled {
		seiWallet, err = crypto.GenerateECDSAKey()
		if err != nil {
			return nil, err
		}

		seiUserAddress = seiWallet.Address()
	}

	if networks.IsArbitrumEnabled {
		arbitrumWallet, err = crypto.GenerateECDSAKey()
		if err != nil {
			return nil, err
		}

		arbitrumUserAddress = arbitrumWallet.Address()
	}

	if networks.IsScrollEnabled {
		scrollWallet, err = crypto.GenerateECDSAKey()
		if err != nil {
			return nil, err
		}

		scrollUserAddress = scrollWallet.Address()
	}

	if networks.IsUnichainEnabled {
		unichainWallet, err = crypto.GenerateECDSAKey()
		if err != nil {
			return nil, err
		}

		unichainUserAddress = unichainWallet.Address()
	}

	if networks.IsSolanaEnabled {
		solanaWallet, err = solanawallet.NewWallet()
		if err != nil {
			return nil, err
		}

		solanaUserAddress = solanaWallet.PublicKey.String()
	}

	return &TestApexUser{
		PrimeWallet:       primeWallet,
		PrimeAddress:      primeUserAddress,
		VectorWallet:      vectorWallet,
		VectorAddress:     vectorUserAddress,
		HasVectorWallet:   networks.IsVectorEnabled,
		CardanoWallet:     cardanoWallet,
		CardanoAddress:    cardanoUserAddress,
		HasCardanoWallet:  networks.IsCardanoEnabled,
		NexusWallet:       nexusWallet,
		NexusAddress:      nexusUserAddress,
		HasNexusWallet:    networks.IsNexusEnabled,
		PolygonWallet:     polygonWallet,
		PolygonAddress:    polygonUserAddress,
		HasPolygonWallet:  networks.IsPolygonEnabled,
		EthereumWallet:    ethereumWallet,
		EthereumAddress:   ethereumUserAddress,
		HasEthereumWallet: networks.IsEthereumEnabled,
		KatanaWallet:      katanaWallet,
		KatanaAddress:     katanaUserAddress,
		HasKatanaWallet:   networks.IsKatanaEnabled,
		SeiWallet:         seiWallet,
		SeiAddress:        seiUserAddress,
		HasSeiWallet:      networks.IsSeiEnabled,
		ArbitrumWallet:    arbitrumWallet,
		ArbitrumAddress:   arbitrumUserAddress,
		HasArbitrumWallet: networks.IsArbitrumEnabled,
		ScrollWallet:      scrollWallet,
		ScrollAddress:     scrollUserAddress,
		HasScrollWallet:   networks.IsScrollEnabled,
		UnichainWallet:    unichainWallet,
		UnichainAddress:   unichainUserAddress,
		HasUnichainWallet: networks.IsUnichainEnabled,
		HasSolanaWallet:   networks.IsSolanaEnabled,
		SolanaWallet:      solanaWallet,
		SolanaAddress:     solanaUserAddress,
	}, nil
}

func NewExistingTestApexUser(
	wallets *apexUserWallets,
	networks *ApexNetworkTypes,
) (*TestApexUser, error) {
	var (
		vectorUserAddress, cardanoUserAddress *cardanowallet.CardanoAddress
		nexusUserAddress, polygonUserAddress, ethereumUserAddress,
		katanaUserAddress, seiUserAddress, arbitrumUserAddress,
		scrollUserAddress, unichainUserAddress types.Address
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

	if wallets.Ethereum != nil && networks.IsEthereumEnabled {
		ethereumUserAddress = wallets.Ethereum.Address()
	}

	if wallets.Katana != nil && networks.IsKatanaEnabled {
		katanaUserAddress = wallets.Katana.Address()
	}

	if wallets.Sei != nil && networks.IsSeiEnabled {
		seiUserAddress = wallets.Sei.Address()
	}

	if wallets.Arbitrum != nil && networks.IsArbitrumEnabled {
		arbitrumUserAddress = wallets.Arbitrum.Address()
	}

	if wallets.Scroll != nil && networks.IsScrollEnabled {
		scrollUserAddress = wallets.Scroll.Address()
	}

	if wallets.Unichain != nil && networks.IsUnichainEnabled {
		unichainUserAddress = wallets.Unichain.Address()
	}

	var solanaUserAddress string
	if wallets.Solana != nil && networks.IsSolanaEnabled {
		solanaUserAddress = wallets.Solana.PublicKey.String()
	}

	return &TestApexUser{
		PrimeWallet:       wallets.Prime,
		PrimeAddress:      primeUserAddress,
		VectorWallet:      wallets.Vector,
		VectorAddress:     vectorUserAddress,
		HasVectorWallet:   wallets.Vector != nil,
		NexusWallet:       wallets.Nexus,
		NexusAddress:      nexusUserAddress,
		HasNexusWallet:    wallets.Nexus != nil,
		CardanoWallet:     wallets.Cardano,
		CardanoAddress:    cardanoUserAddress,
		HasCardanoWallet:  wallets.Cardano != nil,
		PolygonWallet:     wallets.Polygon,
		PolygonAddress:    polygonUserAddress,
		HasPolygonWallet:  wallets.Polygon != nil,
		EthereumWallet:    wallets.Ethereum,
		EthereumAddress:   ethereumUserAddress,
		HasEthereumWallet: wallets.Ethereum != nil,
		KatanaWallet:      wallets.Katana,
		KatanaAddress:     katanaUserAddress,
		HasKatanaWallet:   wallets.Katana != nil,
		SeiWallet:         wallets.Sei,
		SeiAddress:        seiUserAddress,
		HasSeiWallet:      wallets.Sei != nil,
		ArbitrumWallet:    wallets.Arbitrum,
		ArbitrumAddress:   arbitrumUserAddress,
		HasArbitrumWallet: wallets.Arbitrum != nil,
		ScrollWallet:      wallets.Scroll,
		ScrollAddress:     scrollUserAddress,
		HasScrollWallet:   wallets.Scroll != nil,
		UnichainWallet:    wallets.Unichain,
		UnichainAddress:   unichainUserAddress,
		HasUnichainWallet: wallets.Unichain != nil,
		HasSolanaWallet:   wallets.Solana != nil,
		SolanaWallet:      wallets.Solana,
		SolanaAddress:     solanaUserAddress,
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
	case ChainIDEthereum:
		return u.EthereumWallet, u.EthereumAddress
	case ChainIDKatana:
		return u.KatanaWallet, u.KatanaAddress
	case ChainIDSei:
		return u.SeiWallet, u.SeiAddress
	case ChainIDArbitrum:
		return u.ArbitrumWallet, u.ArbitrumAddress
	case ChainIDScroll:
		return u.ScrollWallet, u.ScrollAddress
	case ChainIDUnichain:
		return u.UnichainWallet, u.UnichainAddress
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
	case ChainIDEthereum:
		if u.HasEthereumWallet {
			return u.EthereumAddress.String()
		}

		return ""
	case ChainIDKatana:
		if u.HasKatanaWallet {
			return u.KatanaAddress.String()
		}

		return ""
	case ChainIDSei:
		if u.HasSeiWallet {
			return u.SeiAddress.String()
		}

		return ""
	case ChainIDArbitrum:
		if u.HasArbitrumWallet {
			return u.ArbitrumAddress.String()
		}

		return ""
	case ChainIDScroll:
		if u.HasScrollWallet {
			return u.ScrollAddress.String()
		}

		return ""
	case ChainIDUnichain:
		if u.HasUnichainWallet {
			return u.UnichainAddress.String()
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
	case ChainIDEthereum:
		if u.HasEthereumWallet {
			pkBytes, err := u.EthereumWallet.MarshallPrivateKey()
			if err != nil {
				return "", err
			}

			return hex.EncodeToString(pkBytes), nil
		}

		return "", fmt.Errorf("user doesn't have a ethereum wallet")
	case ChainIDKatana:
		if u.HasKatanaWallet {
			pkBytes, err := u.KatanaWallet.MarshallPrivateKey()
			if err != nil {
				return "", err
			}

			return hex.EncodeToString(pkBytes), nil
		}

		return "", fmt.Errorf("user doesn't have a katana wallet")
	case ChainIDSei:
		if u.HasSeiWallet {
			pkBytes, err := u.SeiWallet.MarshallPrivateKey()
			if err != nil {
				return "", err
			}

			return hex.EncodeToString(pkBytes), nil
		}

		return "", fmt.Errorf("user doesn't have a sei wallet")
	case ChainIDArbitrum:
		if u.HasArbitrumWallet {
			pkBytes, err := u.ArbitrumWallet.MarshallPrivateKey()
			if err != nil {
				return "", err
			}

			return hex.EncodeToString(pkBytes), nil
		}

		return "", fmt.Errorf("user doesn't have a arbitrum wallet")
	case ChainIDScroll:
		if u.HasScrollWallet {
			pkBytes, err := u.ScrollWallet.MarshallPrivateKey()
			if err != nil {
				return "", err
			}

			return hex.EncodeToString(pkBytes), nil
		}

		return "", fmt.Errorf("user doesn't have a scroll wallet")
	case ChainIDUnichain:
		if u.HasUnichainWallet {
			pkBytes, err := u.UnichainWallet.MarshallPrivateKey()
			if err != nil {
				return "", err
			}

			return hex.EncodeToString(pkBytes), nil
		}

		return "", fmt.Errorf("user doesn't have a unichain wallet")
	case ChainIDSolana:
		if u.HasSolanaWallet {
			return u.SolanaWallet.PrivateKey.String(), nil
		}

		return "", fmt.Errorf("user doesn't have a solana wallet")
	}

	return "", nil
}

func (u *TestApexUser) HasWallet(chain ChainID) bool {
	return map[ChainID]bool{
		ChainIDPrime:    true,
		ChainIDVector:   u.HasVectorWallet,
		ChainIDCardano:  u.HasCardanoWallet,
		ChainIDNexus:    u.HasNexusWallet,
		ChainIDPolygon:  u.HasPolygonWallet,
		ChainIDEthereum: u.HasEthereumWallet,
		ChainIDKatana:   u.HasKatanaWallet,
		ChainIDSei:      u.HasSeiWallet,
		ChainIDArbitrum: u.HasArbitrumWallet,
		ChainIDScroll:   u.HasScrollWallet,
		ChainIDUnichain: u.HasUnichainWallet,
		ChainIDSolana:   u.HasSolanaWallet,
	}[chain]
}
