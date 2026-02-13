package solanafw

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/0xPolygon/polygon-edge/e2e-polybft/framework"
)

const (
	DefaultPort     = 8899
	DefaultWSPort   = 8900
	DefaultSlotTime = 400
)

type TestSolanaClusterConfig struct {
	ID         int
	NodesCount int
	Port       int
	WSPort     int
	SlotTime   int
	TmpDir     string
}

func (c *TestSolanaClusterConfig) Dir(name string) string {
	return filepath.Join(c.TmpDir, name)
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

func NewSolanaTestCluster(opts ...SolanaClusterOption) (*TestSolanaCluster, error) {
	config := &TestSolanaClusterConfig{
		NodesCount: 1,
		Port:       DefaultPort,
		WSPort:     DefaultWSPort,
		SlotTime:   DefaultSlotTime,
	}

	for _, opt := range opts {
		opt(config)
	}

	var err error

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
