package e2ehelper

import (
	"context"
	"fmt"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/e2e-polybft/cardanofw"
	"github.com/Ethernal-Tech/cardano-infrastructure/sendtx"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"
)

type SubmittedTxData struct {
	SrcChainID, DstChainID cardanofw.ChainID
	TxHash                 string
	SendAmountDfm          *big.Int
}

type TimeoutConfig struct {
	bridgingRetryWaitTime          time.Duration
	bridgingNumRetries             int
	unexpectedBridgesRetryWaitTime time.Duration
	unexpectedBridgesNumRetries    int
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

func WithUnexpectedBridgesRetryWaitTime(unexpectedBridgesRetryWaitTime time.Duration) TimeoutOption {
	return func(cfg *TimeoutConfig) {
		cfg.unexpectedBridgesRetryWaitTime = unexpectedBridgesRetryWaitTime
	}
}

func WithUnexpectedBridgesNumRetries(retries int) TimeoutOption {
	return func(cfg *TimeoutConfig) {
		cfg.unexpectedBridgesNumRetries = retries
	}
}

func NewTimeoutConfig(options ...TimeoutOption) TimeoutConfig {
	cfg := TimeoutConfig{
		bridgingRetryWaitTime:          10 * time.Second,
		bridgingNumRetries:             100,
		unexpectedBridgesRetryWaitTime: 10 * time.Second,
		unexpectedBridgesNumRetries:    12,
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

type SendTxStrategyFn func(
	t *testing.T, ctx context.Context, apex IApexSystem, chainsDst map[string][]string,
	senders, receivers []*cardanofw.TestApexUser, sendAmountDfm *big.Int, txCountPerSender int,
	bridgingType sendtx.BridgingType) []*SubmittedTxData

type RestartValidatorStrategyFn func(
	t *testing.T, ctx context.Context, apex IApexSystem, configs []RestartValidatorsConfig)

type executeBridgingConfig struct {
	waitForUnexpectedBridges bool
	restartValidatorsConfigs []RestartValidatorsConfig
	sendTxStrategy           SendTxStrategyFn
	restartValidatorStrategy RestartValidatorStrategyFn
	timeoutConfig            TimeoutConfig
	logger                   hclog.Logger
}

func newExecuteBridgingConfig(opts ...ExecuteBridgingOption) *executeBridgingConfig {
	config := &executeBridgingConfig{
		sendTxStrategy:           defaultSendTxStrategy,
		restartValidatorStrategy: defaultRestartValidatorStrategy,
		timeoutConfig:            NewTimeoutConfig(),
		logger:                   hclog.NewNullLogger(),
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

func WithLogger(logger hclog.Logger) ExecuteBridgingOption {
	return func(cfg *executeBridgingConfig) {
		cfg.logger = logger
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
		t *testing.T, ctx context.Context, apex IApexSystem, chainsDst map[string][]string,
		senders, receivers []*cardanofw.TestApexUser, sendAmountDfm *big.Int, txCountPerSender int,
		bridgingType sendtx.BridgingType) []*SubmittedTxData {
		t.Helper()

		var (
			wg              sync.WaitGroup
			mu              sync.Mutex
			submittedTxData []*SubmittedTxData
		)

		for i, sender := range senders {
			for srcChain, dstChains := range chainsDst {
				wg.Add(1)

				go func(idx int, senderUser *cardanofw.TestApexUser, srcChain string, dstChains []string) {
					defer wg.Done()

					for j := 0; j < txCountPerSender; j++ {
						for _, dstChain := range dstChains {
							txHash := apex.SubmitBridgingRequest(
								t, ctx, srcChain, dstChain, senderUser, sendAmountDfm,
								bridgingType, receivers...)

							fmt.Printf("Sender: %d. run: %d. %s->%s tx sent: %s\n",
								idx+1, j+1, srcChain, dstChain, txHash)

							mu.Lock()
							submittedTxData = append(submittedTxData, &SubmittedTxData{
								SrcChainID:    srcChain,
								DstChainID:    dstChain,
								TxHash:        txHash,
								SendAmountDfm: sendAmountDfm,
							})
							mu.Unlock()
						}
					}
				}(i, sender, srcChain, dstChains)
			}
		}

		wg.Wait()

		return submittedTxData
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
