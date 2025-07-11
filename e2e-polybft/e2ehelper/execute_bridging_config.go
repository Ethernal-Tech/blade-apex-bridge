package e2ehelper

import (
	"context"
	"fmt"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/e2e-polybft/cardanofw"
	"github.com/stretchr/testify/require"
)

type TimeoutConfig struct {
	bridgingRetryWaitTime time.Duration
	bridgingNumRetries    int
}

type TimeoutOption func(*TimeoutConfig)

func WithBridgingRetryWaitTime(waitTime time.Duration) TimeoutOption {
	return func(cfg *TimeoutConfig) {
		cfg.bridgingRetryWaitTime = waitTime
	}
}

func WithBridgingNumRetries(retries int) TimeoutOption {
	return func(cfg *TimeoutConfig) {
		cfg.bridgingNumRetries = retries
	}
}

func NewTimeoutConfig(options ...TimeoutOption) TimeoutConfig {
	cfg := TimeoutConfig{
		bridgingRetryWaitTime: 10 * time.Second,
		bridgingNumRetries:    100,
	}

	for _, opt := range options {
		opt(&cfg)
	}

	return cfg
}

type ExecutableType int

const (
	Oracle ExecutableType = iota
	Blade
	BladeAndOracle
)

type RestartValidatorsConfig struct {
	WaitTime   time.Duration
	StartIndxs []int
	StopIndxs  []int

	ExecutableOption ExecutableType
}

// returns map that contains hashes of transactions that were sent and source chain ID and receiver user as a key
type SendTxStrategyFn func(
	t *testing.T, ctx context.Context, apex IApexSystem, chains []srcDstChainPair,
	senders, receivers []*cardanofw.TestApexUser, sendAmountDfm *big.Int, txCountPerSender int) map[string]map[*cardanofw.TestApexUser][]string

type RestartValidatorStrategyFn func(
	t *testing.T, ctx context.Context, apex IApexSystem, configs []RestartValidatorsConfig)

type executeBridgingConfig struct {
	waitForUnexpectedBridges bool
	runIndexerInstance       bool
	restartValidatorsConfigs []RestartValidatorsConfig
	sendTxStrategy           SendTxStrategyFn
	restartValidatorStrategy RestartValidatorStrategyFn
	timeoutConfig            TimeoutConfig
}

func newExecuteBridgingConfig(opts ...ExecuteBridgingOption) *executeBridgingConfig {
	config := &executeBridgingConfig{
		sendTxStrategy:           defaultSendTxStrategy,
		restartValidatorStrategy: defaultRestartValidatorStrategy,
		timeoutConfig:            NewTimeoutConfig(),
	}

	for _, x := range opts {
		x(config)
	}

	return config
}

type ExecuteBridgingOption func(config *executeBridgingConfig)

func WithWaitForUnexpectedBridges(waitForUnexpectedBridges bool) ExecuteBridgingOption {
	return func(config *executeBridgingConfig) {
		config.waitForUnexpectedBridges = waitForUnexpectedBridges
	}
}

func WithRunIndexer(runIndexerInstance bool) ExecuteBridgingOption {
	return func(config *executeBridgingConfig) {
		config.runIndexerInstance = runIndexerInstance
	}
}

func WithRestartValidatorsConfig(restartValidatorsConfigs []RestartValidatorsConfig) ExecuteBridgingOption {
	return func(config *executeBridgingConfig) {
		config.restartValidatorsConfigs = restartValidatorsConfigs
	}
}

func WithSendTxStrategy(strategy SendTxStrategyFn) ExecuteBridgingOption {
	return func(config *executeBridgingConfig) {
		config.sendTxStrategy = strategy
	}
}

func WithTimeoutConfig(tc TimeoutConfig) ExecuteBridgingOption {
	return func(cfg *executeBridgingConfig) {
		cfg.timeoutConfig = tc
	}
}

var (
	defaultSendTxStrategy SendTxStrategyFn = func(
		t *testing.T, ctx context.Context, apex IApexSystem, chains []srcDstChainPair,
		senders, receivers []*cardanofw.TestApexUser, sendAmountDfm *big.Int, txCountPerSender int) map[string]map[*cardanofw.TestApexUser][]string {
		t.Helper()

		var (
			wg sync.WaitGroup
			mu sync.Mutex

			txHashes = make(map[string]map[*cardanofw.TestApexUser][]string, len(chains))
		)

		for i, sender := range senders {
			for _, chainPair := range chains {
				wg.Add(1)

				go func(idx int, senderUser *cardanofw.TestApexUser, chainPair srcDstChainPair) {
					defer wg.Done()

					for j := 0; j < txCountPerSender; j++ {
						txHash := apex.SubmitBridgingRequest(
							t, ctx, chainPair.srcChain, chainPair.dstChain, senderUser, sendAmountDfm, receivers...)

						fmt.Printf("Sender: %d. run: %d. %s->%s tx sent: %s\n",
							idx+1, j+1, chainPair.srcChain, chainPair.dstChain, txHash)

						for _, receiver := range receivers {
							mu.Lock()
							if _, ok := txHashes[chainPair.srcChain]; !ok {
								txHashes[chainPair.srcChain] = make(map[*cardanofw.TestApexUser][]string)
							}
							txHashes[chainPair.srcChain][receiver] = append(txHashes[chainPair.srcChain][receiver], txHash)
							mu.Unlock()
						}

						fmt.Printf("Added txHash: %s\n Tx Hashes for chain %s: %v\n", txHash, chainPair.srcChain, txHashes[chainPair.srcChain])
					}
				}(i, sender, chainPair)
			}
		}

		wg.Wait()
		return txHashes
	}
	defaultRestartValidatorStrategy RestartValidatorStrategyFn = func(
		t *testing.T, ctx context.Context, apex IApexSystem, configs []RestartValidatorsConfig) {
		t.Helper()

		for _, cfg := range configs {
			go func() {
				select {
				case <-ctx.Done():
					return
				case <-time.After(cfg.WaitTime):
					for _, idx := range cfg.StopIndxs {
						if cfg.ExecutableOption == Oracle || cfg.ExecutableOption == BladeAndOracle {
							require.NoError(t, apex.GetValidator(t, idx).Stop())
						}

						if cfg.ExecutableOption == Blade || cfg.ExecutableOption == BladeAndOracle {
							fmt.Printf("Stoping Blade node idx: %d\n", idx)
							require.NoError(t, apex.GetBridgeNode(t, idx).Stop())
						}
					}

					for _, idx := range cfg.StartIndxs {
						if cfg.ExecutableOption == Oracle || cfg.ExecutableOption == BladeAndOracle {
							require.NoError(t, apex.GetValidator(t, idx).Start(ctx, false))
						}

						if cfg.ExecutableOption == Blade || cfg.ExecutableOption == BladeAndOracle {
							fmt.Printf("Starting Blade node idx: %d\n", idx)

							require.NoError(t, apex.GetBridgeNode(t, idx).Start())
						}
					}
				}
			}()
		}
	}
)
