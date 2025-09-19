package cardanofw

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/0xPolygon/polygon-edge/contracts"
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
		require.NoError(t, err)
	case SystemIDSkyline:
		apexSystem, err = NewSkylineSystem(bridgeDataDir, opts...)
		require.NoError(t, err)
	default:
		t.Fatalf("unknown system ID: %s", system)
	}

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

	bridgeSmartContractsUpgrades(t, apexSystem, filepath.Join("..", "..", "apex-bridge-smartcontracts"))

	fmt.Printf("Wallets have been created.\n")

	require.NoError(t, apexSystem.RegisterChains())

	fmt.Printf("Chains have been registered\n")

	require.NoError(t, apexSystem.InitContracts(ctx))

	fmt.Printf("Contracts have been set up\n")

	require.NoError(t, apexSystem.UpdateBridgingAddressCounts(ctx))

	fmt.Printf("Bridging address counts have been updated\n")

	require.NoError(t, apexSystem.CreateAddresses())

	fmt.Printf("Multisig addresses have been created\n")

	require.NoError(t, apexSystem.FinishConfiguring(t))

	fmt.Printf("Configuration has been set up\n")

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

func bridgeSmartContractsUpgrades(t *testing.T, apexSystem *ApexSystem, bridgeSmartContractsDirPath string) {
	t.Helper()

	dir, err := filepath.Abs(bridgeSmartContractsDirPath)
	require.NoError(t, err)

	bridgingAddressesContractAddr, err := apexSystem.DeploySmartContract(
		dir, "BridgingAddresses", []string{contracts.Bridge.String(),
			contracts.Claims.String(), contracts.ApexBridgeAdmin.String()})
	require.NoError(t, err)

	contractParams := []ContractParams{
		{
			contractName:    "Admin",
			contractAddress: contracts.ApexBridgeAdmin.String(),
			functionName:    "setBridgingAddrsDependency",
			functionArgs:    []string{bridgingAddressesContractAddr},
		},
		{
			contractName:    "Bridge",
			contractAddress: contracts.Bridge.String(),
			functionName:    "setBridgingAddrsDependencyAndSync",
			functionArgs:    []string{bridgingAddressesContractAddr},
		},
		{
			contractName:    "Claims",
			contractAddress: contracts.Claims.String(),
			functionName:    "setBridgingAddrsDependencyAndSync",
			functionArgs:    []string{bridgingAddressesContractAddr},
		},
	}

	require.NoError(t, apexSystem.UpgradeSmartContract(&UpgradeSCParams{
		contractsDir:   dir,
		contractParams: contractParams,
		gasLimit:       7_000_000,
	}))
}
