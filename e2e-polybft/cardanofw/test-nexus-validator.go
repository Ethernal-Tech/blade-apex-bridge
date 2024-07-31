package cardanofw

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/0xPolygon/polygon-edge/types"
	bn256 "github.com/Ethernal-Tech/bn256"
	secretsCardano "github.com/Ethernal-Tech/cardano-infrastructure/secrets"
	secretsHelper "github.com/Ethernal-Tech/cardano-infrastructure/secrets/helper"
)

type EthTxWallet struct {
	BN256 *bn256.PrivateKey
}

// TODO:Nexus - parse to get the address
func (w *EthTxWallet) Address() types.Address {
	return types.ZeroAddress
}

type TestNexusValidator struct {
	ID          int
	APIPort     int
	Wallet      *EthTxWallet
	dataDirPath string
}

func NewTestNexusValidator(
	dataDirPath string,
	id int,
) *TestNexusValidator {
	return &TestNexusValidator{
		dataDirPath: filepath.Join(dataDirPath, fmt.Sprintf("validator_%d", id)),
		ID:          id,
	}
}

func (cv *TestNexusValidator) NexusWalletCreate(walletType string) error {
	return RunCommand(ResolveApexBridgeBinary(), []string{
		"wallet-create",
		"--chain", "nexus",
		"--validator-data-dir", cv.dataDirPath,
		"--type", walletType,
	}, os.Stdout)
}

func (cv *TestNexusValidator) GetNexusWallet(keyType string) (*bn256.PrivateKey, error) {
	secretsMngr, err := secretsHelper.CreateSecretsManager(&secretsCardano.SecretsManagerConfig{
		Path: cv.dataDirPath,
		Type: secretsCardano.Local,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to load wallet: %w", err)
	}

	keyName := fmt.Sprintf("%s%s_%s", secretsCardano.OtherKeyLocalPrefix, "nexus", keyType)

	bytes, err := secretsMngr.GetSecret(keyName)
	if err != nil {
		return nil, fmt.Errorf("failed to load wallet: %w", err)
	}

	bn256, err := bn256.UnmarshalPrivateKey(bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal wallet: %w", err)
	}

	return bn256, nil
}
