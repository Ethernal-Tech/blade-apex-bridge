package solanafw

import (
	"fmt"
	"io"
	"strconv"

	"github.com/0xPolygon/polygon-edge/e2e-polybft/framework"
)

const hostIP = "127.0.0.1"

type TestSolanaServerConfig struct {
	ID       int
	Port     int
	WSPort   int
	SlotTime int
	LogsDir  string
	StdOut   io.Writer
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
		"--log-level", "error",
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
	return fmt.Sprintf("localhost:%d", t.config.Port)
}
