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
		assert.NoError(t, apexSystem.CheckAndTerminateAPIProcess())
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

	require.NoError(t, apexSystem.DeployCardanoContracts())

	fmt.Printf("Cardano contracts have been deployed\n")

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

	claimsProcessorAddress, err := apexSystem.DeploySmartContract(
		dir, "ClaimsProcessor", []string{})
	require.NoError(t, err)

	registrationAddress, err := apexSystem.DeploySmartContract(
		dir, "Registration", []string{})
	require.NoError(t, err)

	chainTokensContractAddr, err := apexSystem.DeploySmartContract(
		dir, "ChainTokens", []string{contracts.ApexBridgeAdmin.String(), contracts.Bridge.String(),
			contracts.Claims.String(), claimsProcessorAddress, registrationAddress})
	require.NoError(t, err)

	bridgingAddressesContractAddr, err := apexSystem.DeploySmartContract(
		dir, "BridgingAddresses", []string{contracts.Bridge.String(),
			contracts.Claims.String(), contracts.ApexBridgeAdmin.String()})
	require.NoError(t, err)

	require.NoError(t, apexSystem.SetDependencies(&SetDependenciesSCParams{
		contractsDir: dir,
		contractName: "ClaimsProcessor",
		dependencies: []string{contracts.ApexBridgeAdmin.String(), contracts.Bridge.String(), chainTokensContractAddr,
			contracts.Claims.String(), contracts.ClaimsHelper.String(), registrationAddress, contracts.Validators.String()},
		proxyAddress: claimsProcessorAddress,
	}))

	require.NoError(t, apexSystem.SetDependencies(&SetDependenciesSCParams{
		contractsDir: dir,
		contractName: "Registration",
		dependencies: []string{contracts.Bridge.String(), bridgingAddressesContractAddr, chainTokensContractAddr,
			contracts.Claims.String(), contracts.ClaimsHelper.String(), contracts.Validators.String()},
		proxyAddress: registrationAddress,
	}))

	contractParams := []ContractParams{
		{
			contractName:    "Admin",
			contractAddress: contracts.ApexBridgeAdmin.String(),
			functionName:    "setAdditionalDependenciesAndSync",
			functionArgs:    []string{bridgingAddressesContractAddr, chainTokensContractAddr, "true"},
		},
		{
			contractName:    "Bridge",
			contractAddress: contracts.Bridge.String(),
			functionName:    "setAdditionalDependenciesAndSync",
			functionArgs:    []string{bridgingAddressesContractAddr, chainTokensContractAddr, contracts.ClaimsHelper.String(), registrationAddress, "true"},
		},
		{
			contractName:    "BridgingAddresses",
			contractAddress: bridgingAddressesContractAddr,
			functionName:    "setAdditionalDependenciesAndSync",
			functionArgs:    []string{claimsProcessorAddress, registrationAddress},
		},
		{
			contractName:    "Claims",
			contractAddress: contracts.Claims.String(),
			functionName:    "setAdditionalDependenciesAndSync",
			functionArgs:    []string{bridgingAddressesContractAddr, chainTokensContractAddr, claimsProcessorAddress, registrationAddress, "true"},
		},
		{
			contractName:    "ClaimsHelper",
			contractAddress: contracts.ClaimsHelper.String(),
			functionName:    "setAdditionalDependenciesAndSync",
			functionArgs:    []string{claimsProcessorAddress, registrationAddress},
		},
		{
			contractName:    "Validators",
			contractAddress: contracts.Validators.String(),
			functionName:    "setAdditionalDependenciesAndSync",
			functionArgs:    []string{registrationAddress},
		},
	}

	require.NoError(t, apexSystem.UpgradeSmartContract(&UpgradeSCParams{
		contractsDir:   dir,
		contractParams: contractParams,
		gasLimit:       7_000_000,
	}))
}
