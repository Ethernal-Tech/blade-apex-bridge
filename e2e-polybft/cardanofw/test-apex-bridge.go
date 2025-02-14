package cardanofw

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type SystemID = string

const (
	SystemIDReactor SystemID = "reactor"
	SystemIDSkyline SystemID = "skyline"
)

func SetupAndRunReactorBridge(
	t *testing.T,
	ctx context.Context,
	opts ...ApexSystemOptions,
) *ApexSystem {
	t.Helper()

	return SetupAndRunApexBridge(t, ctx, SystemIDReactor, opts...)
}
func SetupAndRunSkylineBridge(
	t *testing.T,
	ctx context.Context,
	opts ...ApexSystemOptions,
) *ApexSystem {
	t.Helper()

	return SetupAndRunApexBridge(t, ctx, SystemIDSkyline, opts...)
}

func SetupAndRunApexBridge(
	t *testing.T,
	ctx context.Context,
	system SystemID,
	opts ...ApexSystemOptions,
) *ApexSystem {
	t.Helper()

	bridgeDataDir := filepath.Join("..", "..", "e2e-bridge-data-tmp-"+t.Name())

	os.RemoveAll(bridgeDataDir)

	var (
		apexSystem *ApexSystem
		err        error
	)

	switch system {
	case SystemIDReactor:
		apexSystem, err = NewApexSystem(bridgeDataDir, opts...)
	case SystemIDSkyline:
		apexSystem, err = NewSkylineSystem(bridgeDataDir, opts...)
	default:
		err = fmt.Errorf("unknown system ID: %s", system)
	}

	require.NoError(t, err)

	fmt.Printf("Starting chains...\n")

	// stop all chains and the bridge
	t.Cleanup(func() {
		assert.NoError(t, apexSystem.StopAll())
	})

	require.NoError(t, apexSystem.StartChains(t))

	fmt.Printf("Chains have been started. Starting bridge chain...\n")

	apexSystem.StartBridgeChain(t)

	fmt.Printf("Bridge chain has been started. Validators are ready\n")

	require.NoError(t, apexSystem.CreateWallets())

	fmt.Printf("Wallets have been created.\n")

	require.NoError(t, apexSystem.RegisterChains())

	fmt.Printf("Chains have been registered\n")

	require.NoError(t, apexSystem.CreateAddresses())

	fmt.Printf("Multisig addresses have been created\n")

	require.NoError(t, apexSystem.InitContracts(ctx))
	require.NoError(t, apexSystem.FinishConfiguring(t))

	fmt.Printf("Contracts have been set up\n")

	require.NoError(t, apexSystem.FundWallets(ctx))

	fmt.Printf("Wallets have been funded\n")

	require.NoError(t, apexSystem.GenerateConfigs())

	fmt.Printf("Configs have been generated\n")

	require.NoError(t, apexSystem.StartValidatorComponents(ctx))

	fmt.Printf("Validator components started\n")

	require.NoError(t, apexSystem.StartRelayer(ctx))

	fmt.Printf("Relayer started. Apex bridge setup done\n")

	return apexSystem
}
