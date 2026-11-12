package solanafw

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"github.com/0xPolygon/polygon-edge/e2e-polybft/framework"
)

const hostIP = "127.0.0.1"

// sbpfV0V1V2DeploymentDisabledFeature is the SIMD-0500 feature gate ("Disable deployment of
// SBPF v0, v1 and v2 programs"). solana-test-validator activates every known feature by default,
// so without deactivating it only SBPFv3 binaries can be deployed locally, while devnet/mainnet
// still accept older ones. Deactivating it keeps the local validator in line with the clusters
// and lets the upgrade tests deploy an older program build before upgrading to the latest one.
const sbpfV0V1V2DeploymentDisabledFeature = "B8JJXCy5amZyWG9r7EnUYLwzXSXTxG7GZ1qZ1qggo83g"

// ValidatorLogFileName is the name of the file where solana-test-validator logs are written when LogsDir is set.
const ValidatorLogFileName = "validator.log"

type TestSolanaServerConfig struct {
	ID       int
	Port     int
	WSPort   int
	SlotTime int
	LogsDir  string
	StdOut   io.Writer
}

type TestSolanaServer struct {
	config  *TestSolanaServerConfig
	node    *framework.Node
	logFile *os.File
}

func NewSolanaTestServer(config *TestSolanaServerConfig) (*TestSolanaServer, error) {
	srv := &TestSolanaServer{
		config: config,
	}

	return srv, srv.Start()
}

func (t *TestSolanaServer) IsRunning() bool {
	return t.node != nil
}

func (t *TestSolanaServer) Stop(removeDB ...bool) error {
	if err := t.node.Stop(); err != nil {
		return err
	}

	t.node = nil

	if t.logFile != nil {
		_ = t.logFile.Close()
		t.logFile = nil
	}

	// Remove the ledger directory after the server is stopped to free disk space.
	if t.config.LogsDir != "" {
		ledgerDir := filepath.Join(t.config.LogsDir, "ledger")
		if err := os.RemoveAll(ledgerDir); err != nil {
			fmt.Printf("warning: failed to remove ledger dir %s: %v\n", ledgerDir, err)
		}
	}

	return nil
}

func (t *TestSolanaServer) Start() error {
	// solana-test-validator requires --ledger (required). RPC/WS ports and bind address
	// are set via --rpc-port and --bind-address.
	ledgerDir := "test-ledger"
	if t.config.LogsDir != "" {
		ledgerDir = filepath.Join(t.config.LogsDir, "ledger")
	}

	fmt.Println("Starting Solana test validator with ledger directory: ", ledgerDir)

	metaplexProgramID := "metaqbxxUerdq28cj1RbAWkYQm3ybzjb6a8bt518x1s"
	metadataProgramSoPath := filepath.Join("..", "..", "skyline-solana-programs", "program_build", "mpl_token_metadata.so")

	args := []string{
		"--ledger", ledgerDir,
		"--rpc-port", strconv.Itoa(t.config.Port),
		"--bind-address", hostIP,
		"--bpf-program", metaplexProgramID, metadataProgramSoPath,
		"--limit-ledger-size", "200000000", // ensure no cleanup is done on ledger
		"--deactivate-feature", sbpfV0V1V2DeploymentDisabledFeature,
		"--log", // stream validator log to stdout (we redirect to file when LogsDir is set)
	}

	if t.config.SlotTime > 0 {
		args = append(args, "--ticks-per-slot", strconv.Itoa(t.config.SlotTime))
	}

	binary := ResolveSolanaTestValidatorBinary()

	stdout := t.config.StdOut
	if stdout == nil {
		stdout = os.Stdout
	}

	// When LogsDir is set, write validator logs only to a file (no stdout) so the terminal is not flooded.
	if t.config.LogsDir != "" {
		logPath := filepath.Join(t.config.LogsDir, ValidatorLogFileName)

		logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND|os.O_TRUNC, 0600)
		if err != nil {
			return fmt.Errorf("open validator log file %s: %w", logPath, err)
		}

		t.logFile = logFile
		stdout = logFile

		fmt.Println("Solana test validator logs will be written to: ", logPath)
	}

	node, err := framework.NewNode(binary, args, stdout)
	if err != nil {
		if t.logFile != nil {
			_ = t.logFile.Close()
			t.logFile = nil
		}

		return err
	}

	t.node = node
	t.node.SetShouldForceStop(true)

	return nil
}

func (t *TestSolanaServer) Stat() (bool, error) {
	return true, nil
}

func (t TestSolanaServer) ID() int {
	return t.config.ID
}

func (t TestSolanaServer) Port() int {
	return t.config.Port
}

func (t *TestSolanaServer) NetworkAddress() string {
	return fmt.Sprintf("http://%s:%d", hostIP, t.config.Port)
}
