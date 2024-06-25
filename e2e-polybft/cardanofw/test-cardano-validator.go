package cardanofw

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path"

	"github.com/0xPolygon/polygon-edge/e2e-polybft/framework"
	"github.com/Ethernal-Tech/cardano-infrastructure/wallet"
)

const (
	CardanoWalletsDir  = "cardano-wallet"
	BridgingConfigsDir = "bridging-configs"
	BridgingLogsDir    = "bridging-logs"
	BridgingDBsDir     = "bridging-dbs"

	ValidatorComponentsConfigFileName = "vc_config.json"
	RelayerConfigFileName             = "relayer_config.json"
)

type CardanoWallet struct {
	Multisig    wallet.IWallet
	MultisigFee wallet.IWallet
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
		dataDirPath: path.Join(dataDirPath, fmt.Sprintf("validator_%d", id)),
		ID:          id,
	}
}

func (cv *TestCardanoValidator) SetClusterAndServer(
	cluster *framework.TestCluster, server *framework.TestServer) {
	cv.cluster = cluster
	cv.server = server
}

func (cv *TestCardanoValidator) GetCardanoWalletsDir() string {
	return path.Join(cv.dataDirPath, CardanoWalletsDir)
}

func (cv *TestCardanoValidator) GetBridgingConfigsDir() string {
	return path.Join(cv.dataDirPath, BridgingConfigsDir)
}

func (cv *TestCardanoValidator) GetValidatorComponentsConfig() string {
	return path.Join(cv.GetBridgingConfigsDir(), ValidatorComponentsConfigFileName)
}

func (cv *TestCardanoValidator) GetRelayerConfig() string {
	return path.Join(cv.GetBridgingConfigsDir(), RelayerConfigFileName)
}

func (cv *TestCardanoValidator) CardanoWalletCreate(chainID string) error {
	return RunCommand(ResolveApexBridgeBinary(), []string{
		"wallet-create",
		"--chain", chainID,
		"--dir", cv.GetCardanoWalletsDir(),
	}, os.Stdout)
}

func (cv *TestCardanoValidator) GetCardanoWallet(chainID string) (*CardanoWallet, error) {
	wm := wallet.NewWalletManager()

	multiSig, err := wm.Load(path.Join(
		cv.GetCardanoWalletsDir(), chainID, "multisig"))
	if err != nil {
		return nil, err
	}

	multiSigFee, err := wm.Load(path.Join(
		cv.GetCardanoWalletsDir(), chainID, "multisigfee"))
	if err != nil {
		return nil, err
	}

	return &CardanoWallet{
		Multisig:    multiSig,
		MultisigFee: multiSigFee,
	}, nil
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
		"--keys-dir", cv.GetCardanoWalletsDir(),
		"--bridge-validator-data-dir", cv.server.DataDir(),
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
		"--output-dir", cv.GetBridgingConfigsDir(),
		"--output-validator-components-file-name", ValidatorComponentsConfigFileName,
		"--output-relayer-file-name", RelayerConfigFileName,
		"--prime-keys-dir", path.Join(cv.GetCardanoWalletsDir(), ChainIDPrime),
		"--prime-network-address", primeNetworkAddress,
		"--prime-network-magic", fmt.Sprint(primeNetworkMagic),
		"--prime-network-id", fmt.Sprint(primeNetworkID),
		"--prime-ogmios-url", primeOgmiosURL,
		"--vector-keys-dir", path.Join(cv.GetCardanoWalletsDir(), ChainIDVector),
		"--vector-network-address", vectorNetworkAddress,
		"--vector-network-magic", fmt.Sprint(vectorNetworkMagic),
		"--vector-network-id", fmt.Sprint(vectorNetworkID),
		"--vector-ogmios-url", vectorOgmiosURL,
		"--bridge-node-url", cv.server.JSONRPCAddr(),
		"--bridge-sc-address", BridgeSCAddr,
		"--logs-path", path.Join(cv.dataDirPath, BridgingLogsDir),
		"--dbs-path", path.Join(cv.dataDirPath, BridgingDBsDir),
		"--bridge-validator-data-dir", cv.server.DataDir(),
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