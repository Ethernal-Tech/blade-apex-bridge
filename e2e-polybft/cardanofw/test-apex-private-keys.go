package cardanofw

import (
	"encoding/hex"
	"fmt"

	"github.com/0xPolygon/polygon-edge/crypto"
	wallet "github.com/Ethernal-Tech/cardano-infrastructure/wallet"
	solanawallet "github.com/Ethernal-Tech/solana-infrastructure/wallet"
)

type ApexPrivateKeys struct {
	PrimePaymentSigningKeyCborHex   string `json:"primePaymentSKCborHex"`
	PrimeStakeSigningKeyCborHex     string `json:"primeStakeSKCborHex"`
	VectorPaymentSigningKeyCborHex  string `json:"vectorPaymentSKCborHex"`
	VectorStakeSigningKeyCborHex    string `json:"vectorStakeSKCborHex"`
	NexusPrivateKey                 string `json:"nexusPK"`
	CardanoPaymentSigningKeyCborHex string `json:"cardanoPaymentSKCborHex"`
	CardanoStakeSigningKeyCborHex   string `json:"cardanoStakeSKCborHex"`
	PolygonPrivateKey               string `json:"polygonPK"`
	SolanaPrivateKey                string `json:"solanaPK"`
}

func (keys *ApexPrivateKeys) Wallets() (*apexUserWallets, error) {
	prime, err := newCardanoWalletFromCborHex(
		keys.PrimePaymentSigningKeyCborHex, keys.PrimeStakeSigningKeyCborHex)
	if err != nil {
		return nil, err
	}

	var (
		vector, cardano *wallet.Wallet
		nexus, polygon  *crypto.ECDSAKey
		solana          *solanawallet.Wallet
	)

	if len(keys.VectorPaymentSigningKeyCborHex) > 0 {
		vector, err = newCardanoWalletFromCborHex(keys.VectorPaymentSigningKeyCborHex, keys.VectorStakeSigningKeyCborHex)
		if err != nil {
			return nil, err
		}
	}

	if len(keys.NexusPrivateKey) > 0 {
		nexus, err = newEvmWallet(keys.NexusPrivateKey)
		if err != nil {
			return nil, err
		}
	}

	if len(keys.CardanoPaymentSigningKeyCborHex) > 0 {
		cardano, err = newCardanoWalletFromCborHex(
			keys.CardanoPaymentSigningKeyCborHex, keys.CardanoStakeSigningKeyCborHex)
		if err != nil {
			return nil, err
		}
	}

	if len(keys.PolygonPrivateKey) > 0 {
		polygon, err = newEvmWallet(keys.PolygonPrivateKey)
		if err != nil {
			return nil, err
		}
	}

	if len(keys.SolanaPrivateKey) > 0 {
		solana, err = newSolanaWalletFromBase58(keys.SolanaPrivateKey)
		if err != nil {
			return nil, err
		}
	}

	return &apexUserWallets{
		Prime:   prime,
		Vector:  vector,
		Nexus:   nexus,
		Cardano: cardano,
		Polygon: polygon,
		Solana:  solana,
	}, nil
}

func (keys *ApexPrivateKeys) User(
	networks *ApexNetworkTypes,
) (*TestApexUser, error) {
	wallets, err := keys.Wallets()
	if err != nil {
		return nil, err
	}

	return NewExistingTestApexUser(wallets, networks)
}

func newEvmWallet(privateKey string) (*crypto.ECDSAKey, error) {
	if len(privateKey) == 0 {
		return nil, fmt.Errorf("empty private key")
	}

	pkBytes, err := hex.DecodeString(privateKey)
	if err != nil {
		return nil, err
	}

	return crypto.NewECDSAKeyFromRawPrivECDSA(pkBytes)
}

func newCardanoWalletFromCborHex(paymentKey, stakeKey string) (*wallet.Wallet, error) {
	paymentBytes, err := wallet.GetKeyBytes(paymentKey)
	if err != nil {
		return nil, err
	}

	var stakeBytes []byte
	if len(stakeKey) > 0 {
		stakeBytes, err = wallet.GetKeyBytes(stakeKey)
		if err != nil {
			return nil, err
		}
	}

	return wallet.NewWallet(paymentBytes, stakeBytes), nil
}

func newSolanaWalletFromBase58(privateKey string) (*solanawallet.Wallet, error) {
	if len(privateKey) == 0 {
		return nil, fmt.Errorf("empty private key")
	}

	return solanawallet.NewWalletFromPrivateKey(privateKey)
}
