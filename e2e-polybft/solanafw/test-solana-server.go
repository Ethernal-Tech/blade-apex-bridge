package solanafw

import (
	"fmt"
	"io"
	"math/big"
	"strconv"

	"github.com/0xPolygon/polygon-edge/e2e-polybft/framework"
)

const hostIP = "127.0.0.1"

type TestSolanaServerConfig struct {
	ID            int
	Port          int
	WSPort        int
	SlotTime      int
	LogsDir       string
	Premine       []string
	PremineAmount *big.Int
	StdOut        io.Writer
}

type TestSolanaServer struct {
	config *TestSolanaServerConfig
	node   *framework.Node
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

	return nil
}

func (t *TestSolanaServer) Start() error {
	fmt.Println("Starting Solana server with logs directory: ", t.config.LogsDir)
	// Build arguments
	args := []string{
		"start",
		"--port", strconv.Itoa(t.config.Port),
		"--ws-port", strconv.Itoa(t.config.WSPort),
		"--host", hostIP,
		"--slot-time", strconv.Itoa(t.config.SlotTime),
		"--no-tui",    // Display streams of logs instead of terminal UI dashboard
		"--no-studio", // Disable studio
	}

	if t.config.LogsDir != "" {
		args = append(args, "--log-path", t.config.LogsDir)
	}

	if len(t.config.Premine) > 0 {
		for _, premine := range t.config.Premine {
			args = append(args, "--airdrop", premine)
		}
	}

	if t.config.PremineAmount != nil {
		args = append(args, "--airdrop-amount", t.config.PremineAmount.String())
	}

	binary := ResolveSurfPoolBinary()

	node, err := framework.NewNode(binary, args, t.config.StdOut)
	if err != nil {
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
