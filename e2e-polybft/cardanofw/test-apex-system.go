package cardanofw

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	cardanowallet "github.com/Ethernal-Tech/cardano-infrastructure/wallet"
	"github.com/stretchr/testify/require"
)

type ApexSystem struct {
	PrimeCluster  *TestCardanoCluster
	VectorCluster *TestCardanoCluster
	Nexus         *TestEVMBridge
	Bridge        *TestCardanoBridge

	Config *ApexSystemConfig
}

type ApexSystemConfig struct {
	// Apex
	VectorEnabled bool

	ApiValidatorID int // -1 all validators
	ApiPortStart   int
	ApiKey         string

	VectorTTLInc                uint64
	VectorSlotRoundingThreshold uint64
	PrimeTTLInc                 uint64
	PrimeSlotRoundingThreshold  uint64

	TelemetryConfig               string
	TargetOneCardanoClusterServer bool

	CardanoNodesNum     int
	BladeValidatorCount int

	// Nexus EVM
	NexusEnabled bool

	NexusValidatorCount int
	NexusStartingPort   int64
}

type ApexSystemOptions func(*ApexSystemConfig)

func WithAPIValidatorID(apiValidatorID int) ApexSystemOptions {
	return func(h *ApexSystemConfig) {
		h.ApiValidatorID = apiValidatorID
	}
}

func WithAPIPortStart(apiPortStart int) ApexSystemOptions {
	return func(h *ApexSystemConfig) {
		h.ApiPortStart = apiPortStart
	}
}

func WithAPIKey(apiKey string) ApexSystemOptions {
	return func(h *ApexSystemConfig) {
		h.ApiKey = apiKey
	}
}

func WithVectorTTL(threshold, ttlInc uint64) ApexSystemOptions {
	return func(h *ApexSystemConfig) {
		h.VectorSlotRoundingThreshold = threshold
		h.VectorTTLInc = ttlInc
	}
}

func WithPrimeTTL(threshold, ttlInc uint64) ApexSystemOptions {
	return func(h *ApexSystemConfig) {
		h.PrimeSlotRoundingThreshold = threshold
		h.PrimeTTLInc = ttlInc
	}
}

func WithTelemetryConfig(tc string) ApexSystemOptions {
	return func(h *ApexSystemConfig) {
		h.TelemetryConfig = tc // something like "0.0.0.0:5001,localhost:8126"
	}
}

func WithTargetOneCardanoClusterServer(targetOneCardanoClusterServer bool) ApexSystemOptions {
	return func(h *ApexSystemConfig) {
		h.TargetOneCardanoClusterServer = targetOneCardanoClusterServer
	}
}

func WithVectorEnabled(enabled bool) ApexSystemOptions {
	return func(h *ApexSystemConfig) {
		h.VectorEnabled = enabled
	}
}

func WithNexusEnabled(enabled bool) ApexSystemOptions {
	return func(h *ApexSystemConfig) {
		h.NexusEnabled = enabled
	}
}

func WithNexusStartintPort(port int64) ApexSystemOptions {
	return func(h *ApexSystemConfig) {
		h.NexusStartingPort = port
	}
}

func (as *ApexSystemConfig) ServiceCount() int {
	// Prime
	count := 1

	if as.VectorEnabled {
		count++
	}

	if as.NexusEnabled {
		count++
	}

	return count
}

func newApexSystemConfig(opts ...ApexSystemOptions) *ApexSystemConfig {
	config := &ApexSystemConfig{
		ApiValidatorID: 1,
		ApiPortStart:   40000,
		ApiKey:         "test_api_key",

		CardanoNodesNum:     4,
		BladeValidatorCount: 4,

		NexusValidatorCount: 4,
		NexusStartingPort:   int64(30400),

		VectorEnabled: true,
		NexusEnabled:  false,
	}

	for _, opt := range opts {
		opt(config)
	}

	return config
}

func RunApexBridge(
	t *testing.T, ctx context.Context,
	apexOptions ...ApexSystemOptions,
) *ApexSystem {
	t.Helper()

	apexConfig := newApexSystemConfig(apexOptions...)
	apexSystem := &ApexSystem{Config: apexConfig}
	wg := &sync.WaitGroup{}
	serviceCount := apexConfig.ServiceCount()
	errorsContainer := make([]error, serviceCount)

	wg.Add(serviceCount)

	go func() {
		defer wg.Done()

		apexSystem.PrimeCluster, errorsContainer[0] = RunCardanoCluster(
			t, ctx, 0, apexConfig.CardanoNodesNum, cardanowallet.TestNetNetwork)
	}()

	if apexConfig.VectorEnabled {
		go func() {
			defer wg.Done()

			apexSystem.VectorCluster, errorsContainer[1] = RunCardanoCluster(
				t, ctx, 1, apexConfig.CardanoNodesNum, cardanowallet.VectorTestNetNetwork)
		}()
	}

	if apexConfig.NexusEnabled {
		go func() {
			defer wg.Done()

			apexSystem.Nexus, errorsContainer[2] = RunEVMChain(
				t, apexConfig)
		}()
	}

	t.Cleanup(func() {
		fmt.Println("Stopping chains...")

		if apexSystem.PrimeCluster != nil {
			wg.Add(1)

			go func() {
				defer wg.Done()

				errorsContainer[0] = apexSystem.PrimeCluster.Stop()
			}()
		}

		if apexSystem.VectorCluster != nil {
			wg.Add(1)

			go func() {
				defer wg.Done()

				errorsContainer[1] = apexSystem.VectorCluster.Stop()
			}()
		}

		if apexSystem.Nexus != nil {
			wg.Add(1)

			go func() {
				defer wg.Done()

				apexSystem.Nexus.Cluster.Stop()
			}()
		}

		if apexSystem.Bridge != nil {
			wg.Add(1)

			go func() {
				defer wg.Done()

				fmt.Printf("Cleaning up apex bridge\n")
				apexSystem.Bridge.StopValidators()
				fmt.Printf("Done cleaning up apex bridge\n")
			}()
		}

		wg.Wait()

		fmt.Printf("Chains has been stopped...%v\n", errors.Join(errorsContainer[:]...))
	})

	fmt.Println("Starting chains...")

	wg.Wait()

	fmt.Println("Chains have been started...")

	require.NoError(t, errors.Join(errorsContainer[:]...))

	apexSystem.Bridge = SetupAndRunApexBridge(
		t, ctx,
		// path.Join(path.Dir(primeCluster.Config.TmpDir), "bridge"),
		"../../e2e-bridge-data-tmp-"+t.Name(),
		apexSystem,
	)

	if apexConfig.NexusEnabled {
		SetupAndRunNexusBridge(
			t, ctx,
			apexSystem,
		)
	}

	apexSystem.SetupAndRunValidatorsAndRelayer(t, ctx)

	fmt.Printf("Apex bridge setup done\n")

	return apexSystem
}
