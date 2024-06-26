package cardanofw

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"

	"github.com/0xPolygon/polygon-edge/e2e-polybft/framework"
	"github.com/0xPolygon/polygon-edge/helper/common"
	secretsCardano "github.com/Ethernal-Tech/cardano-infrastructure/secrets"
	secretsHelper "github.com/Ethernal-Tech/cardano-infrastructure/secrets/helper"
	cardanoWallet "github.com/Ethernal-Tech/cardano-infrastructure/wallet"
)

const (
	BridgingConfigsDir = "bridging-configs"
	BridgingLogsDir    = "bridging-logs"
	BridgingDBsDir     = "bridging-dbs"

	ValidatorComponentsConfigFileName = "vc_config.json"
	RelayerConfigFileName             = "relayer_config.json"
)

type CardanoWallet struct {
	Multisig    *cardanoWallet.Wallet `json:"multisig"`
	MultisigFee *cardanoWallet.Wallet `json:"fee"`
}

type TestCardanoValidator struct {
	ID          int
	APIPort     int
	dataDirPath string
	cluster     *framework.TestCluster
	server      *framework.TestServer
	node        *framework.Node
}

func NewTestCardanoValidator(
	dataDirPath string,
	id int,
) *TestCardanoValidator {
	return &TestCardanoValidator{
		dataDirPath: filepath.Join(dataDirPath, fmt.Sprintf("validator_%d", id)),
		ID:          id,
	}
}

func (cv *TestCardanoValidator) SetClusterAndServer(
	cluster *framework.TestCluster, server *framework.TestServer,
) error {
	cv.cluster = cluster
	cv.server = server
	// move wallets files
	srcPath := filepath.Join(cv.dataDirPath, secretsCardano.CardanoFolderLocal)
	dstPath := filepath.Join(cv.server.DataDir(), secretsCardano.CardanoFolderLocal)

	if err := common.CreateDirSafe(dstPath, 0750); err != nil {
		return fmt.Errorf("failed to create dst directory: %w", err)
	}

	files, err := os.ReadDir(srcPath)
	if err != nil {
		return fmt.Errorf("failed to read source directory: %w", err)
	}

	for _, file := range files {
		sourcePath := filepath.Join(srcPath, file.Name())
		destPath := filepath.Join(dstPath, file.Name())
		// Move the file
		if err := os.Rename(sourcePath, destPath); err != nil {
			return fmt.Errorf("failed to move file %s: %w", file.Name(), err)
		}
	}

	return nil
}

func (cv *TestCardanoValidator) GetBridgingConfigsDir() string {
	return filepath.Join(cv.dataDirPath, BridgingConfigsDir)
}

func (cv *TestCardanoValidator) GetValidatorComponentsConfig() string {
	return filepath.Join(cv.GetBridgingConfigsDir(), ValidatorComponentsConfigFileName)
}

func (cv *TestCardanoValidator) GetRelayerConfig() string {
	return filepath.Join(cv.GetBridgingConfigsDir(), RelayerConfigFileName)
}

func (cv *TestCardanoValidator) CardanoWalletCreate(chainID string) error {
	return RunCommand(ResolveApexBridgeBinary(), []string{
		"wallet-create",
		"--chain", chainID,
		"--validator-data-dir", cv.dataDirPath,
	}, os.Stdout)
}

func (cv *TestCardanoValidator) GetCardanoWallet(chainID string) (*CardanoWallet, error) {
	secretsMngr, err := secretsHelper.CreateSecretsManager(&secretsCardano.SecretsManagerConfig{
		Path: cv.dataDirPath,
		Type: secretsCardano.Local,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to load wallet: %w", err)
	}

	keyName := fmt.Sprintf("%s%s_key", secretsCardano.CardanoKeyLocalPrefix, chainID)

	bytes, err := secretsMngr.GetSecret(keyName)
	if err != nil {
		return nil, fmt.Errorf("failed to load wallet: %w", err)
	}

	var cardanoWallet *CardanoWallet

	if err := json.Unmarshal(bytes, &cardanoWallet); err != nil {
		return nil, fmt.Errorf("failed to load wallet: %w", err)
	}

	return cardanoWallet, nil
}

func (cv *TestCardanoValidator) RegisterChain(
	chainID string,
	multisigAddr string,
	multisigFeeAddr string,
	tokenSupply *big.Int,
	ogmiosURL string,
) error {
	return RunCommand(ResolveApexBridgeBinary(), []string{
		"register-chain",
		"--chain", chainID,
		"--validator-data-dir", cv.server.DataDir(),
		"--addr", multisigAddr,
		"--addr-fee", multisigFeeAddr,
		"--token-supply", fmt.Sprint(tokenSupply),
		"--ogmios", ogmiosURL,
		"--bridge-url", cv.server.JSONRPCAddr(),
		"--bridge-addr", BridgeSCAddr,
	}, os.Stdout)
}

func (cv *TestCardanoValidator) GenerateConfigs(
	primeNetworkAddress string,
	primeNetworkMagic uint,
	primeNetworkID uint,
	primeOgmiosURL string,
	primeTTLInc uint64,
	vectorNetworkAddress string,
	vectorNetworkMagic uint,
	vectorNetworkID uint,
	vectorOgmiosURL string,
	vectorTTLInc uint64,
	apiPort int,
	apiKey string,
	telemetryConfig string,
) error {
	cv.APIPort = apiPort

	args := []string{
		"generate-configs",
		"--validator-data-dir", cv.server.DataDir(),
		"--output-dir", cv.GetBridgingConfigsDir(),
		"--output-validator-components-file-name", ValidatorComponentsConfigFileName,
		"--output-relayer-file-name", RelayerConfigFileName,
		"--prime-network-address", primeNetworkAddress,
		"--prime-network-magic", fmt.Sprint(primeNetworkMagic),
		"--prime-network-id", fmt.Sprint(primeNetworkID),
		"--prime-ogmios-url", primeOgmiosURL,
		"--vector-network-address", vectorNetworkAddress,
		"--vector-network-magic", fmt.Sprint(vectorNetworkMagic),
		"--vector-network-id", fmt.Sprint(vectorNetworkID),
		"--vector-ogmios-url", vectorOgmiosURL,
		"--bridge-node-url", cv.server.JSONRPCAddr(),
		"--bridge-sc-address", BridgeSCAddr,
		"--logs-path", filepath.Join(cv.dataDirPath, BridgingLogsDir),
		"--dbs-path", filepath.Join(cv.dataDirPath, BridgingDBsDir),
		"--api-port", fmt.Sprint(apiPort),
		"--api-keys", apiKey,
		"--telemetry", telemetryConfig,
	}

	if primeTTLInc > 0 {
		args = append(args,
			"--prime-ttl-slot-inc", fmt.Sprint(primeTTLInc),
		)
	}

	if vectorTTLInc > 0 {
		args = append(args,
			"--vector-ttl-slot-inc", fmt.Sprint(vectorTTLInc),
		)
	}

	return RunCommand(ResolveApexBridgeBinary(), args, os.Stdout)
}

func (cv *TestCardanoValidator) Start(ctx context.Context, runAPI bool) (err error) {
	args := []string{
		"run-validator-components",
		"--config", cv.GetValidatorComponentsConfig(),
	}

	if runAPI {
		args = append(args, "--run-api")
	}

	cv.node, err = framework.NewNodeWithContext(ctx, ResolveApexBridgeBinary(), args, os.Stdout)

	return err
}

func (cv *TestCardanoValidator) Stop() error {
	if cv.node == nil {
		return errors.New("validator not started")
	}

	return cv.node.Stop()
}
