package cardanofw

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

const FundTokenAmount = uint64(100_000_000_000)

func SetupAndRunApexBridge(
	t *testing.T,
	ctx context.Context,
	opts ...ApexSystemOptions,
) *ApexSystem {
	t.Helper()

	bridgeDataDir := "../../e2e-bridge-data-tmp-" + t.Name() // path.Join(path.Dir(primeCluster.Config.TmpDir), "bridge")

	os.RemoveAll(bridgeDataDir)

	apexSystem := NewApexSystem(bridgeDataDir, opts...)

	fmt.Printf("Starting chains...\n")

	apexSystem.StartChains(t)

	fmt.Printf("Chains have been started. Starting bridge chain...\n")

	apexSystem.StartBridgeChain(t)

	fmt.Printf("Bridge chain has been started\n")

	require.NoError(t, apexSystem.CreateWallets(false))

	fmt.Printf("Validators are ready\n")

	require.NoError(t, apexSystem.RegisterChains(FundTokenAmount))

	fmt.Printf("Chains have been registered\n")

	require.NoError(t, apexSystem.CreateCardanoMultisigAddresse())

	fmt.Printf("Cardano Multisig addresses have been created\n")

	require.NoError(t, apexSystem.FundCardanoMultisigAddresses(ctx, FundTokenAmount))

	if apexSystem.config.NexusEnabled {
		apexSystem.Nexus.SetupChain(t, apexSystem.BridgeCluster.Servers[0].JSONRPCAddr(),
			apexSystem.GetBridgeAdmin(), apexSystem.GetRelayerWalletAddr())
	}

	require.NoError(t, apexSystem.GenerateConfigs(
		apexSystem.PrimeCluster,
		apexSystem.VectorCluster,
		apexSystem.Nexus,
	))

	fmt.Printf("Configs have been generated\n")

	require.NoError(t, apexSystem.StartValidatorComponents(ctx))

	fmt.Printf("Validator components started\n")

	require.NoError(t, apexSystem.StartRelayer(ctx))

	fmt.Printf("Relayer started. Apex bridge setup done\n")

	return apexSystem
}
