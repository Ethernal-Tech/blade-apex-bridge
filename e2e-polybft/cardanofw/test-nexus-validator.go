package cardanofw

import (
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/0xPolygon/polygon-edge/e2e-polybft/framework"
	secretsCardano "github.com/Ethernal-Tech/cardano-infrastructure/secrets"
	secretsHelper "github.com/Ethernal-Tech/cardano-infrastructure/secrets/helper"
	"github.com/ethereum/go-ethereum/common"
)

type EthTxWallet struct {
	Addres     common.Address
	PrivateKey *ecdsa.PrivateKey
}

type TestNexusValidator struct {
	ID          int
	APIPort     int
	dataDirPath string
	cluster     *framework.TestCluster
	server      *framework.TestServer
	node        *framework.Node
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

// func (cv *TestCardanoValidator) SetClusterAndServer(
// 	cluster *framework.TestCluster, server *framework.TestServer,
// ) error {
// 	cv.cluster = cluster
// 	cv.server = server
// 	// move wallets files
// 	srcPath := filepath.Join(cv.dataDirPath, secretsCardano.CardanoFolderLocal)
// 	dstPath := filepath.Join(cv.server.DataDir(), secretsCardano.CardanoFolderLocal)

// 	if err := common.CreateDirSafe(dstPath, 0750); err != nil {
// 		return fmt.Errorf("failed to create dst directory: %w", err)
// 	}

// 	files, err := os.ReadDir(srcPath)
// 	if err != nil {
// 		return fmt.Errorf("failed to read source directory: %w", err)
// 	}

// 	for _, file := range files {
// 		sourcePath := filepath.Join(srcPath, file.Name())
// 		destPath := filepath.Join(dstPath, file.Name())
// 		// Move the file
// 		if err := os.Rename(sourcePath, destPath); err != nil {
// 			return fmt.Errorf("failed to move file %s: %w", file.Name(), err)
// 		}
// 	}

// 	return nil
// }

func (cv *TestNexusValidator) NexusWalletCreate(walletType string) error {
	return RunCommand(ResolveApexBridgeBinary(), []string{
		"wallet-create",
		"--chain", "nexus",
		"--validator-data-dir", cv.dataDirPath,
		"--type", walletType,
	}, os.Stdout)
}

func (cv *TestNexusValidator) GetNexusWallet(keyType string) (*EthTxWallet, error) {
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

	var wallet *EthTxWallet

	if err := json.Unmarshal(bytes, &wallet); err != nil {
		return nil, fmt.Errorf("failed to load wallet: %w", err)
	}

	return wallet, nil
}

// func (cv *TestCardanoValidator) Start(ctx context.Context, runAPI bool) (err error) {
// 	args := []string{
// 		"run-validator-components",
// 		"--config", cv.GetValidatorComponentsConfig(),
// 	}

// 	if runAPI {
// 		args = append(args, "--run-api")
// 	}

// 	cv.node, err = framework.NewNodeWithContext(ctx, ResolveApexBridgeBinary(), args, os.Stdout)

// 	return err
// }

// func (cv *TestCardanoValidator) Stop() error {
// 	if cv.node == nil {
// 		return errors.New("validator not started")
// 	}

// 	return cv.node.Stop()
// }
