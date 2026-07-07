package solanafw

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/e2e-polybft/framework"
	"github.com/0xPolygon/polygon-edge/helper/common"
)

const (
	DefaultPort     = 8899
	DefaultWSPort   = 8900
	DefaultSlotTime = 64
)

type TestSolanaClusterConfig struct {
	t *testing.T

	ID         int
	NodesCount int
	Port       int
	WSPort     int
	SlotTime   int
	TmpDir     string
	LogsDir    string
}

func (cfg *TestSolanaClusterConfig) Dir(name string) string {
	return filepath.Join(cfg.TmpDir, name)
}

func (cfg *TestSolanaClusterConfig) initLogsDir(t *testing.T) error {
	t.Helper()

	logsDir := path.Join("../..", fmt.Sprintf("e2e-logs-%d%s", time.Now().UTC().Unix(), "solana"), t.Name())

	if err := common.CreateDirSafe(logsDir, 0755); err != nil {
		return err
	}

	cfg.LogsDir = logsDir

	return nil
}

type TestSolanaCluster struct {
	Config  *TestSolanaClusterConfig
	Servers []*TestSolanaServer

	once         sync.Once
	failCh       chan struct{}
	executionErr error
}

type SolanaClusterOption func(*TestSolanaClusterConfig)

func WithPort(port int) SolanaClusterOption {
	return func(h *TestSolanaClusterConfig) {
		h.Port = port
	}
}

func WithWSPort(port int) SolanaClusterOption {
	return func(h *TestSolanaClusterConfig) {
		h.WSPort = port
	}
}

func NewSolanaTestCluster(t *testing.T, opts ...SolanaClusterOption) (*TestSolanaCluster, error) {
	t.Helper()

	config := &TestSolanaClusterConfig{
		t:          t,
		NodesCount: 1,
		Port:       DefaultPort,
		WSPort:     DefaultWSPort,
		SlotTime:   DefaultSlotTime,
	}

	var err error

	err = config.initLogsDir(t)
	if err != nil {
		return nil, err
	}

	for _, opt := range opts {
		opt(config)
	}

	config.TmpDir, err = os.MkdirTemp("", "solana-")
	if err != nil {
		return nil, err
	}

	cluster := &TestSolanaCluster{
		Config:  config,
		Servers: []*TestSolanaServer{},
		failCh:  make(chan struct{}),
		once:    sync.Once{},
	}

	for i := 0; i < cluster.Config.NodesCount; i++ {
		err = cluster.NewTestServer(i+1, config.Port+i, config.WSPort+i)
		if err != nil {
			return nil, err
		}
	}

	return cluster, nil
}

func (c *TestSolanaCluster) NewTestServer(id int, port int, wsPort int) error {
	srv, err := NewSolanaTestServer(&TestSolanaServerConfig{
		ID:       id,
		Port:     port,
		WSPort:   wsPort,
		SlotTime: c.Config.SlotTime,
		// StdOut:   c.Config.GetStdout(fmt.Sprintf("solana-node-%d", id)),
		LogsDir: c.Config.LogsDir,
	})
	if err != nil {
		return err
	}

	// watch the server for stop signals. It is important to fix the specific
	// 'node' reference since 'TestServer' creates a new one if restarted.
	go func(node *framework.Node) {
		<-node.Wait()

		if !node.ExitResult().Signaled {
			c.Fail(fmt.Errorf("server id = %d, port = %d has stopped unexpectedly", id, port))
		}
	}(srv.node)

	c.Servers = append(c.Servers, srv)

	return nil
}

func (c *TestSolanaCluster) Fail(err error) {
	c.once.Do(func() {
		c.executionErr = err
		close(c.failCh)
	})
}

func (c *TestSolanaCluster) Stop() error {
	wg := sync.WaitGroup{}
	errs := []error(nil)

	for _, srv := range c.Servers {
		if srv.IsRunning() {
			wg.Add(1)

			go func(s *TestSolanaServer) {
				defer wg.Done()

				fmt.Printf("terminating solana node: cluster=%d, node port=%d\n", c.Config.ID, s.Port())

				errs = append(errs, s.Stop())

				fmt.Printf("solana node has been terminated: cluster=%d, node port=%d\n", c.Config.ID, s.Port())
			}(srv)
		}
	}

	wg.Wait()

	return errors.Join(errs...)
}

func (c *TestSolanaCluster) NetworkAddress() string {
	return fmt.Sprintf("localhost:%d", c.Config.Port)
}
