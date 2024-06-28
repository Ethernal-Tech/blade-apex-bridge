package cardanofw

// import (
// 	"bytes"
// 	"fmt"
// 	"io"
// 	"os"
// 	"path"
// 	"path/filepath"
// 	"strconv"
// 	"strings"
// 	"sync"
// 	"testing"
// 	"time"

// 	"github.com/0xPolygon/polygon-edge/e2e-polybft/framework"
// 	"github.com/0xPolygon/polygon-edge/helper/common"
// 	cardano_wallet "github.com/Ethernal-Tech/cardano-infrastructure/wallet"
// )

// func resolveVectorNodeBinary() string {
// 	bin := os.Getenv("CARDANO_NODE_BINARY")
// 	if bin != "" {
// 		return bin
// 	}
// 	// fallback
// 	return "cardano-node"

// 	// ./..location../bin/cardano-node
// }

// func resolveVectorCliBinary() string {
// 	bin := os.Getenv("CARDANO_CLI_BINARY")
// 	if bin != "" {
// 		return bin
// 	}
// 	// fallback
// 	return "cardano-cli"

// 	// ./..location../bin/cardano-cli
// }

// type VectorTestnetNodeConfig struct {
// 	t *testing.T

// 	ID            int
// 	NetworkMagic  int
// 	SecurityParam int
// 	Port          int

// 	WithLogs   bool
// 	WithStdout bool
// 	LogsDir    string
// 	TmpDir     string
// 	NodeDir    string

// 	CliBinary  string // cardano-cli
// 	NodeBinary string // cardano-node
// 	ConfigFile string
// 	SocketPath string

// 	TxProvider cardano_wallet.ITxProvider
// 	StdOut     io.Writer

// 	logsDirOnce sync.Once
// }

// func (c *VectorTestnetNodeConfig) Dir(name string) string {
// 	return filepath.Join(c.TmpDir, name)
// }

// func (c *VectorTestnetNodeConfig) GetStdout(name string, custom ...io.Writer) io.Writer {
// 	writers := []io.Writer{}

// 	if c.WithLogs {
// 		c.logsDirOnce.Do(func() {
// 			if err := c.initLogsDir(); err != nil {
// 				c.t.Fatal("GetStdout init logs dir", "err", err)
// 			}
// 		})

// 		f, err := os.OpenFile(filepath.Join(c.LogsDir, name+".log"), os.O_RDWR|os.O_APPEND|os.O_CREATE, 0600)
// 		if err != nil {
// 			c.t.Log("GetStdout open file error", "err", err)
// 		} else {
// 			writers = append(writers, f)

// 			c.t.Cleanup(func() {
// 				if err := f.Close(); err != nil {
// 					c.t.Log("GetStdout close file error", "err", err)
// 				}
// 			})
// 		}
// 	}

// 	if c.WithStdout {
// 		writers = append(writers, os.Stdout)
// 	}

// 	if len(custom) > 0 {
// 		writers = append(writers, custom...)
// 	}

// 	if len(writers) == 0 {
// 		return io.Discard
// 	}

// 	return io.MultiWriter(writers...)
// }

// func (c *VectorTestnetNodeConfig) initLogsDir() error {
// 	if c.LogsDir == "" {
// 		logsDir := path.Join("../..", fmt.Sprintf("e2e-logs-cardano-%d", time.Now().UTC().Unix()), c.t.Name())
// 		if err := common.CreateDirSafe(logsDir, 0750); err != nil {
// 			return err
// 		}

// 		c.t.Logf("logs enabled for e2e test: %s", logsDir)
// 		c.LogsDir = logsDir
// 	}

// 	return nil
// }

// type VectorTestnetNode struct {
// 	t *testing.T

// 	Config *VectorTestnetNodeConfig
// 	Node   *framework.Node
// 	// OgmiosServer *TestOgmiosServer

// 	once         sync.Once
// 	failCh       chan struct{}
// 	executionErr error
// }

// type VectorTestnetNodeOption func(*VectorTestnetNodeConfig)

// // func WithStartNodeID(startNodeID int) VectorClusterOption { // COM: Should this be removed?
// // 	return func(h *TestVectorClusterConfig) {
// // 		h.StartNodeID = startNodeID
// // 	}
// // }

// // func WithPort(port int) VectorClusterOption {
// // 	return func(h *TestVectorClusterConfig) {
// // 		h.Port = port
// // 	}
// // }

// // func WithLogsDir(logsDir string) VectorClusterOption {
// // 	return func(h *TestVectorClusterConfig) {
// // 		h.LogsDir = logsDir
// // 	}
// // }

// // func WithNetworkMagic(networkMagic int) VectorClusterOption {
// // 	return func(h *TestVectorClusterConfig) {
// // 		h.NetworkMagic = networkMagic
// // 	}
// // }

// // func WithID(id int) VectorClusterOption {
// // 	return func(h *TestVectorClusterConfig) {
// // 		h.ID = id
// // 	}
// // }

// func NewVectorTestnetNode(t *testing.T, opts ...VectorTestnetNodeOption) (*VectorTestnetNode, error) {
// 	t.Helper()

// 	var err error

// 	config := &VectorTestnetNodeConfig{
// 		t:          t,
// 		WithLogs:   true, // strings.ToLower(os.Getenv(e)) == "true"
// 		WithStdout: true, // strings.ToLower(os.Getenv(envStdoutEnabled)) == "true"
// 		CliBinary:  resolveVectorCliBinary(),
// 		NodeBinary: resolveCardanoNodeBinary(),

// 		NetworkMagic:  1127,
// 		SecurityParam: 216,
// 		Port:          7522,
// 	}

// 	for _, opt := range opts {
// 		opt(config)
// 	}

// 	config.TmpDir, err = os.MkdirTemp("/tmp", "vector-testnet-")
// 	if err != nil {
// 		return nil, err
// 	}

// 	cluster := &VectorTestnetNode{
// 		t:      t,
// 		Node:   nil,
// 		Config: config,
// 		failCh: make(chan struct{}),
// 		once:   sync.Once{},
// 	}

// 	// mkdir vector && cd vector
// 	if err := cluster.CreateDirectory(); err != nil {
// 		return nil, err
// 	}

// 	// wget -P $DIR https://artifacts.apexfusion.org/vector-node-beta-8.9.3.0.0.0.0.5-linux.tar.gz
// 	if err := cluster.DownloadVectorNode("https://artifacts.apexfusion.org/vector-node-beta-8.9.3.0.0.0.0.5-linux.tar.gz"); err != nil {
// 		return nil, err
// 	}

// 	// tar xvzf vector-node-beta-8.9.3.0.0.0.0.5-linux.tar.gz -C $DIR
// 	if err := cluster.UnpackVectorNode("vector-node-beta-8.9.3.0.0.0.0.5-linux.tar.gz"); err != nil {
// 		return nil, err
// 	}

// 	return cluster, nil
// }

// func (c *VectorTestnetNode) Start() error {
// 	// Node arguments
// 	args := []string{
// 		"run",
// 		"--topology", fmt.Sprintf("%s/share/vector_testnet/topology.json", c.Config.NodeDir),
// 		"--database-path", fmt.Sprintf("%s/db", c.Config.NodeDir),
// 		"--socket-path", c.SocketPath(),
// 		// "--shelley-kes-key", fmt.Sprintf("%s/kes.skey", t.config.NodeDir),
// 		// "--shelley-vrf-key", fmt.Sprintf("%s/vrf.skey", t.config.NodeDir),
// 		// "--byron-delegation-certificate", fmt.Sprintf("%s/byron-delegation.cert", t.config.NodeDir),
// 		// "--byron-signing-key", fmt.Sprintf("%s/byron-delegate.key", t.config.NodeDir),
// 		// "--shelley-operational-certificate", fmt.Sprintf("%s/opcert.cert", t.config.NodeDir),
// 		"--config", fmt.Sprintf("%s/share/vector_testnet/config.json", c.Config.NodeDir),
// 		"--port", strconv.Itoa(c.Config.Port),
// 	}

// 	node, err := framework.NewNode(c.Config.NodeBinary, args, c.Config.StdOut)
// 	if err != nil {
// 		return err
// 	}

// 	c.Node = node
// 	c.Node.SetShouldForceStop(true)

// 	return nil
// }

// func (c *VectorTestnetNode) Fail(err error) {
// 	return
// }

// func (c *VectorTestnetNode) Stop() error {
// 	return nil
// }

// func (c *VectorTestnetNode) NetworkURL() string {
// 	return fmt.Sprintf("http://localhost:%d", c.Config.Port)
// }

// func (c *VectorTestnetNode) Stats() ([]*TestCardanoStats, bool, error) {
// 	blocks := make([]*TestCardanoStats, len(c.Servers))
// 	ready := make([]bool, len(c.Servers))
// 	errors := make([]error, len(c.Servers))
// 	wg := sync.WaitGroup{}

// 	for i := range c.Servers {
// 		id, srv := i, c.Servers[i]
// 		if !srv.IsRunning() {
// 			ready[id] = true

// 			continue
// 		}

// 		wg.Add(1)

// 		go func() {
// 			defer wg.Done()

// 			var b bytes.Buffer

// 			stdOut := c.Config.GetStdout(fmt.Sprintf("cardano-stats-%d", srv.ID()), &b)
// 			args := []string{
// 				"query", "tip",
// 				"--testnet-magic", strconv.Itoa(c.Config.NetworkMagic),
// 				"--socket-path", srv.SocketPath(),
// 			}

// 			if err := RunCommand(c.Config.Binary, args, stdOut); err != nil {
// 				if strings.Contains(err.Error(), "Network.Socket.connect") &&
// 					strings.Contains(err.Error(), "does not exist (No such file or directory)") {
// 					c.Config.t.Log("socket error", "path", srv.SocketPath(), "err", err)

// 					return
// 				}

// 				ready[id], errors[id] = true, err

// 				return
// 			}

// 			stat, err := NewTestCardanoStats(b.Bytes())
// 			if err != nil {
// 				ready[id], errors[id] = true, err
// 			}

// 			ready[id], blocks[id] = true, stat
// 		}()
// 	}

// 	wg.Wait()

// 	for i, err := range errors {
// 		if err != nil {
// 			return nil, true, err
// 		} else if !ready[i] {
// 			return nil, false, nil
// 		}
// 	}

// 	return blocks, true, nil
// }

// func (c *VectorTestnetNode) WaitUntil(timeout, frequency time.Duration, handler func() (bool, error)) error {
// 	ticker := time.NewTicker(frequency)
// 	defer ticker.Stop()

// 	timer := time.NewTimer(timeout)
// 	defer timer.Stop()

// 	for {
// 		select {
// 		case <-timer.C:
// 			return fmt.Errorf("timeout")
// 		case <-c.failCh:
// 			return c.executionErr
// 		case <-ticker.C:
// 		}

// 		finish, err := handler()
// 		if err != nil {
// 			return err
// 		} else if finish {
// 			return nil
// 		}
// 	}
// }

// func (c *VectorTestnetNode) WaitForReady(timeout time.Duration) error {
// 	return c.WaitUntil(timeout, time.Second*2, func() (bool, error) {
// 		_, ready, err := c.Stats()

// 		return ready, err
// 	})
// }

// func (c *VectorTestnetNode) WaitForBlock(
// 	n uint64, timeout time.Duration, frequency time.Duration,
// ) error {
// 	return c.WaitUntil(timeout, frequency, func() (bool, error) {
// 		tips, ready, err := c.Stats()
// 		if err != nil {
// 			return false, err
// 		} else if !ready {
// 			return false, nil
// 		}

// 		c.Config.t.Log("WaitForBlock", "tips", tips)

// 		for _, tip := range tips {
// 			if tip.Block < n {
// 				return false, nil
// 			}
// 		}

// 		return true, nil
// 	})
// }

// func (c *VectorTestnetNode) WaitForBlockWithState(
// 	n uint64, timeout time.Duration,
// ) error {
// 	servers := c.Servers
// 	blockState := make(map[uint64]map[int]string, len(c.Servers))

// 	return c.WaitUntil(timeout, time.Millisecond*200, func() (bool, error) {
// 		tips, ready, err := c.Stats()
// 		if err != nil {
// 			return false, err
// 		} else if !ready {
// 			return false, nil
// 		}

// 		fmt.Print("WaitForBlockWithState", "tips", tips)

// 		for i, bn := range tips {
// 			serverID := servers[i].ID()
// 			// bn == nil -> server is stopped + dont remember smaller than n blocks
// 			if bn.Block < n {
// 				continue
// 			}

// 			if mp, exists := blockState[bn.Block]; exists {
// 				mp[serverID] = bn.Hash
// 			} else {
// 				blockState[bn.Block] = map[int]string{
// 					serverID: bn.Hash,
// 				}
// 			}
// 		}

// 		// for all running servers there must be at least one block >= n
// 		// that all servers have with same hash
// 		for _, mp := range blockState {
// 			if len(mp) != len(c.Servers) {
// 				continue
// 			}

// 			hash, ok := "", true

// 			for _, h := range mp {
// 				if hash == "" {
// 					hash = h
// 				} else if h != hash {
// 					ok = false

// 					break
// 				}
// 			}

// 			if ok {
// 				return true, nil
// 			}
// 		}

// 		return false, nil
// 	})
// }

// func (c *VectorTestnetNode) CreateDirectory() error {
// 	dir := c.Config.Dir("vector-testnet")
// 	c.Config.NodeDir = dir

// 	return nil
// }

// func (c *VectorTestnetNode) DownloadVectorNode(url string) error {
// 	var b bytes.Buffer
// 	stdOut := c.Config.GetStdout("vector-testnet-download-vector-node", &b)

// 	args := []string{
// 		"-P", c.Config.NodeDir,
// 		url,
// 	}

// 	err := RunCommand("wget", args, stdOut)

// 	return err
// }

// func (c *VectorTestnetNode) UnpackVectorNode(fileName string) error {
// 	var b bytes.Buffer
// 	stdOut := c.Config.GetStdout("vector-testnet-unpack-vector-node", &b)

// 	args := []string{
// 		"xvzf",
// 		fileName,
// 		"-C", c.Config.NodeDir,
// 	}

// 	err := RunCommand("tar", args, stdOut)

// 	return err
// }

// func (c *VectorTestnetNode) RunningServersCount() int {
// 	if c.Node != nil {
// 		return 1
// 	}
// 	return 0
// }

// func (c *VectorTestnetNode) SocketPath() string {
// 	// socketPath handle for windows \\.\pipe\
// 	return fmt.Sprintf("%s/node.socket", c.Config.TmpDir)
// }
