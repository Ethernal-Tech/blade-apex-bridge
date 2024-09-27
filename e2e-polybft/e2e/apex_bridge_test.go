package e2e

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/e2e-polybft/cardanofw"
	"github.com/Ethernal-Tech/cardano-infrastructure/wallet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Download Cardano executables from https://github.com/IntersectMBO/cardano-node/releases/tag/8.7.3 and unpack tar.gz file
// Add directory where unpacked files are located to the $PATH (in example bellow `~/Apps/cardano`)
// eq add line `export PATH=$PATH:~/Apps/cardano` to  `~/.bashrc`
// cd e2e-polybft/e2e
// ONLY_RUN_APEX_BRIDGE=true go test -v -timeout 0 -run ^Test_OnlyRunApexBridge_WithNexusAndVector$ github.com/0xPolygon/polygon-edge/e2e-polybft/e2e
func Test_OnlyRunApexBridge_WithNexusAndVector(t *testing.T) {
	if shouldRun := os.Getenv("ONLY_RUN_APEX_BRIDGE"); shouldRun != "true" {
		t.Skip()
	}

	const (
		apiKey = "test_api_key"
	)

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	apex := cardanofw.RunApexBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithVectorEnabled(true),
		cardanofw.WithNexusEnabled(true),
	)

	oracleAPI, err := apex.Bridge.GetBridgingAPI()
	require.NoError(t, err)

	fmt.Printf("oracle API: %s\n", oracleAPI)
	fmt.Printf("oracle API key: %s\n", apiKey)

	fmt.Printf("prime network url: %s\n", apex.PrimeCluster.NetworkAddress())
	fmt.Printf("prime ogmios url: %s\n", apex.PrimeCluster.OgmiosURL())
	fmt.Printf("prime bridging addr: %s\n", apex.Bridge.PrimeMultisigAddr)
	fmt.Printf("prime fee addr: %s\n", apex.Bridge.PrimeMultisigFeeAddr)

	fmt.Printf("vector network url: %s\n", apex.VectorCluster.NetworkAddress())
	fmt.Printf("vector ogmios url: %s\n", apex.VectorCluster.OgmiosURL())
	fmt.Printf("vector bridging addr: %s\n", apex.Bridge.VectorMultisigAddr)
	fmt.Printf("vector fee addr: %s\n", apex.Bridge.VectorMultisigFeeAddr)

	user := apex.CreateAndFundUser(t, ctx, uint64(500_000_000))

	primeUserSKHex := hex.EncodeToString(user.PrimeWallet.GetSigningKey())
	vectorUserSKHex := hex.EncodeToString(user.VectorWallet.GetSigningKey())

	fmt.Printf("user prime addr: %s\n", user.PrimeAddress)
	fmt.Printf("user prime signing key hex: %s\n", primeUserSKHex)
	fmt.Printf("user vector addr: %s\n", user.VectorAddress)
	fmt.Printf("user vector signing key hex: %s\n", vectorUserSKHex)

	evmUser, err := apex.CreateAndFundNexusUser(ctx, 10_000)
	require.NoError(t, err)
	pkBytes, err := evmUser.Ecdsa.MarshallPrivateKey()
	require.NoError(t, err)

	chainID, err := apex.Nexus.Cluster.Servers[0].JSONRPC().ChainID()
	require.NoError(t, err)

	fmt.Printf("nexus user addr: %s\n", evmUser.Address())
	fmt.Printf("nexus user signing key: %s\n", hex.EncodeToString(pkBytes))
	fmt.Printf("nexus url: %s\n", apex.Nexus.Cluster.Servers[0].JSONRPCAddr())
	fmt.Printf("nexus gateway sc addr: %s\n", apex.Nexus.GetGatewayAddress().String())
	fmt.Printf("nexus chainID: %v\n", chainID)

	fmt.Printf("birdge url: %s\n", apex.Bridge.GetFirstServer().JSONRPCAddr())

	signalChannel := make(chan os.Signal, 1)
	// Notify the signalChannel when the interrupt signal is received (Ctrl+C)
	signal.Notify(signalChannel, os.Interrupt, syscall.SIGTERM)

	<-signalChannel
}

func TestE2E_ApexBridge_DoNothingWithSpecificUser(t *testing.T) {
	t.Skip()

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	apex := cardanofw.RunApexBridge(t, ctx)
	user := apex.CreateAndFundExistingUser(
		t, ctx,
		"58201825bce09711e1563fc1702587da6892d1d869894386323bd4378ea5e3d6cba0",
		"5820ccdae0d1cd3fa9be16a497941acff33b9aa20bdbf2f9aa5715942d152988e083", uint64(500_000_000),
		apex.PrimeCluster.NetworkConfig(), apex.VectorCluster.NetworkConfig())

	fmt.Println("Prime multisig", apex.Bridge.PrimeMultisigAddr)
	fmt.Println("Prime fee", apex.Bridge.PrimeMultisigFeeAddr)
	fmt.Println("Prime User", user.PrimeAddress)
	fmt.Println("Prime testnet", apex.PrimeCluster.Config.NetworkMagic)
	fmt.Println("Prime ogmios", apex.PrimeCluster.OgmiosServer.Port())
	fmt.Println("Vector multisig", apex.Bridge.VectorMultisigAddr)
	fmt.Println("Vector fee", apex.Bridge.VectorMultisigFeeAddr)
	fmt.Println("Vector User", user.VectorAddress)
	fmt.Println("Vector testnet", apex.VectorCluster.Config.NetworkMagic)
	fmt.Println("Vector ogmios", apex.VectorCluster.OgmiosServer.Port())

	time.Sleep(time.Second * 60 * 60) // one hour sleep :)
}

func TestE2E_ApexBridge_CardanoOracleState(t *testing.T) {
	const (
		apiKey = "my_api_key"
	)

	ctx, cncl := context.WithTimeout(context.Background(), time.Second*180)
	defer cncl()

	apex := cardanofw.RunApexBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithAPIValidatorID(-1),
	)

	apiURLs, err := apex.Bridge.GetBridgingAPIs()
	require.NoError(t, err)

	require.Equal(t, apex.Bridge.GetValidatorsCount(), len(apiURLs))

	ticker := time.NewTicker(time.Second * 4)
	defer ticker.Stop()

	goodOraclesCount := 0

	for goodOraclesCount < apex.Bridge.GetValidatorsCount() {
		select {
		case <-ctx.Done():
			t.Fatal("timeout")
		case <-ticker.C:
		}

		goodOraclesCount = 0

	outerLoop:
		for _, apiURL := range apiURLs {
			for _, chainID := range []string{"vector", "prime"} {
				requestURL := fmt.Sprintf("%s/api/OracleState/Get?chainId=%s", apiURL, chainID)

				currentState, err := cardanofw.GetOracleState(ctx, requestURL, apiKey)
				if err != nil || currentState == nil {
					break outerLoop
				}

				multisigAddr, feeAddr := "", ""
				sumMultisig, sumFee := uint64(0), uint64(0)

				switch chainID {
				case "prime":
					multisigAddr, feeAddr = apex.Bridge.PrimeMultisigAddr, apex.Bridge.PrimeMultisigFeeAddr
				case "vector":
					multisigAddr, feeAddr = apex.Bridge.VectorMultisigAddr, apex.Bridge.VectorMultisigFeeAddr
				}

				for _, utxo := range currentState.Utxos {
					switch utxo.Address {
					case multisigAddr:
						sumMultisig += utxo.Amount
					case feeAddr:
						sumFee += utxo.Amount
					}
				}

				if sumMultisig != 0 || sumFee != 0 {
					fmt.Printf("%s sums: %d, %d\n", requestURL, sumMultisig, sumFee)
				}

				if sumMultisig != cardanofw.FundTokenAmount || sumFee != cardanofw.FundTokenAmount ||
					currentState.BlockSlot == 0 {
					break outerLoop
				} else {
					goodOraclesCount++
				}
			}
		}
	}
}

func TestE2E_ApexBridge(t *testing.T) {
	ctx, cncl := context.WithTimeout(context.Background(), time.Second*180)
	defer cncl()

	apex := cardanofw.RunApexBridge(t, ctx)
	user := apex.CreateAndFundUser(t, ctx, uint64(5_000_000))

	txProviderPrime := apex.GetPrimeTxProvider()
	txProviderVector := apex.GetVectorTxProvider()

	// Initiate bridging PRIME -> VECTOR
	sendAmount := uint64(1_000_000)
	prevAmount, err := cardanofw.GetTokenAmount(ctx, txProviderVector, user.VectorAddress)
	require.NoError(t, err)

	expectedAmount := prevAmount + sendAmount

	user.BridgeAmount(t, ctx, txProviderPrime, apex.Bridge.PrimeMultisigAddr,
		apex.Bridge.VectorMultisigFeeAddr, sendAmount, apex.PrimeCluster.NetworkConfig())

	err = cardanofw.WaitForAmount(ctx, txProviderVector, user.VectorAddress, func(val uint64) bool {
		return val == expectedAmount
	}, 15, time.Second*10)
	require.NoError(t, err)
}

func TestE2E_ApexBridge_BatchRecreated(t *testing.T) {
	const (
		apiKey = "test_api_key"
	)

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	apex := cardanofw.RunApexBridge(
		t, ctx,
		cardanofw.WithPrimeTTL(20, 1),
		cardanofw.WithVectorTTL(30, 1),
		cardanofw.WithAPIKey(apiKey),
	)
	user := apex.CreateAndFundUser(t, ctx, uint64(5_000_000))

	txProviderPrime := apex.GetPrimeTxProvider()

	// Initiate bridging PRIME -> VECTOR
	sendAmount := uint64(1_000_000)

	txHash := user.BridgeAmount(t, ctx, txProviderPrime, apex.Bridge.PrimeMultisigAddr,
		apex.Bridge.VectorMultisigFeeAddr, sendAmount, apex.PrimeCluster.NetworkConfig())

	timeoutTimer := time.NewTimer(time.Second * 300)
	defer timeoutTimer.Stop()

	var (
		wentFromFailedOnDestinationToIncludedInBatch bool
		prevStatus                                   string
		currentStatus                                string
	)

	apiURL, err := apex.Bridge.GetBridgingAPI()
	require.NoError(t, err)

	requestURL := fmt.Sprintf(
		"%s/api/BridgingRequestState/Get?chainId=%s&txHash=%s", apiURL, "prime", txHash)

	fmt.Printf("Bridging request txHash = %s\n", txHash)

for_loop:
	for {
		select {
		case <-timeoutTimer.C:
			fmt.Printf("Timeout\n")

			break for_loop
		case <-ctx.Done():
			fmt.Printf("Done\n")

			break for_loop
		case <-time.After(time.Millisecond * 500):
		}

		currentState, err := cardanofw.GetBridgingRequestState(ctx, requestURL, apiKey)
		if err != nil || currentState == nil {
			continue
		}

		prevStatus = currentStatus
		currentStatus = currentState.Status

		if prevStatus != currentStatus {
			fmt.Printf("currentStatus = %s\n", currentStatus)
		}

		if prevStatus == "FailedToExecuteOnDestination" &&
			(currentStatus == "IncludedInBatch" || currentStatus == "SubmittedToDestination") {
			wentFromFailedOnDestinationToIncludedInBatch = true

			break for_loop
		}
	}

	require.True(t, wentFromFailedOnDestinationToIncludedInBatch)
}

func TestE2E_ApexBridge_InvalidScenarios(t *testing.T) {
	const (
		apiKey = "test_api_key"
	)

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	apex := cardanofw.RunApexBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
	)
	user := apex.CreateAndFundUser(t, ctx, uint64(50_000_000))

	txProviderPrime := apex.GetPrimeTxProvider()

	t.Run("Submitter not enough funds", func(t *testing.T) {
		sendAmount := uint64(1_000_000)
		feeAmount := uint64(1_100_000)

		receivers := map[string]uint64{
			user.VectorAddress: sendAmount * 10, // 10Ada
		}

		bridgingRequestMetadata, err := cardanofw.CreateMetaData(
			user.PrimeAddress, receivers, cardanofw.GetDestinationChainID(apex.PrimeCluster.NetworkConfig()), feeAmount)
		require.NoError(t, err)

		txHash, err := cardanofw.SendTx(
			ctx, txProviderPrime, user.PrimeWallet, sendAmount+feeAmount, apex.Bridge.PrimeMultisigAddr,
			apex.PrimeCluster.NetworkConfig(), bridgingRequestMetadata)
		require.NoError(t, err)

		apiURL, err := apex.Bridge.GetBridgingAPI()
		require.NoError(t, err)
		cardanofw.WaitForInvalidState(t, ctx, apiURL, apiKey, txHash)
	})

	t.Run("Multiple submitters don't have enough funds", func(t *testing.T) {
		for i := 0; i < 5; i++ {
			sendAmount := uint64(1_000_000)
			feeAmount := uint64(1_100_000)

			receivers := map[string]uint64{
				user.VectorAddress: sendAmount * 10, // 10Ada
			}

			bridgingRequestMetadata, err := cardanofw.CreateMetaData(
				user.PrimeAddress, receivers, cardanofw.GetDestinationChainID(apex.PrimeCluster.NetworkConfig()), feeAmount)
			require.NoError(t, err)

			txHash, err := cardanofw.SendTx(
				ctx, txProviderPrime, user.PrimeWallet, sendAmount+feeAmount, apex.Bridge.PrimeMultisigAddr,
				apex.PrimeCluster.NetworkConfig(), bridgingRequestMetadata)
			require.NoError(t, err)

			apiURL, err := apex.Bridge.GetBridgingAPI()
			require.NoError(t, err)
			cardanofw.WaitForInvalidState(t, ctx, apiURL, apiKey, txHash)
		}
	})

	t.Run("Multiple submitters don't have enough funds parallel", func(t *testing.T) {
		var err error

		instances := 5
		walletKeys := make([]wallet.IWallet, instances)
		txHashes := make([]string, instances)
		primeGenesisWallet := apex.GetPrimeGenesisWallet(t)

		for i := 0; i < instances; i++ {
			walletKeys[i], err = wallet.GenerateWallet(false)
			require.NoError(t, err)

			walletAddress, err := cardanofw.GetAddress(apex.PrimeCluster.NetworkConfig().NetworkType, walletKeys[i])
			require.NoError(t, err)

			fundSendAmount := uint64(5_000_000)
			txHash, err := cardanofw.SendTx(ctx, txProviderPrime, primeGenesisWallet,
				fundSendAmount, walletAddress.String(), apex.PrimeCluster.NetworkConfig(), []byte{})
			require.NoError(t, err)
			err = wallet.WaitForTxHashInUtxos(ctx, txProviderPrime, walletAddress.String(), txHash,
				60, time.Second*2, cardanofw.IsRecoverableError)
			require.NoError(t, err)
		}

		sendAmount := uint64(1_000_000)
		feeAmount := uint64(1_100_000)

		var wg sync.WaitGroup

		for i := 0; i < instances; i++ {
			idx := i
			receivers := map[string]uint64{
				user.VectorAddress: sendAmount * 10, // 10Ada
			}

			bridgingRequestMetadata, err := cardanofw.CreateMetaData(
				user.PrimeAddress, receivers, cardanofw.GetDestinationChainID(apex.PrimeCluster.NetworkConfig()), feeAmount)
			require.NoError(t, err)

			wg.Add(1)

			go func() {
				defer wg.Done()

				txHashes[idx], err = cardanofw.SendTx(
					ctx, txProviderPrime, walletKeys[idx], sendAmount+feeAmount, apex.Bridge.PrimeMultisigAddr,
					apex.PrimeCluster.NetworkConfig(), bridgingRequestMetadata)
				require.NoError(t, err)
			}()
		}

		wg.Wait()

		for i := 0; i < instances; i++ {
			apiURL, err := apex.Bridge.GetBridgingAPI()
			require.NoError(t, err)
			cardanofw.WaitForInvalidState(t, ctx, apiURL, apiKey, txHashes[i])
		}
	})

	t.Run("Submitted invalid metadata", func(t *testing.T) {
		sendAmount := uint64(1_000_000)
		feeAmount := uint64(1_100_000)

		receivers := map[string]uint64{
			user.VectorAddress: sendAmount,
		}

		bridgingRequestMetadata, err := cardanofw.CreateMetaData(
			user.PrimeAddress, receivers, cardanofw.GetDestinationChainID(apex.PrimeCluster.NetworkConfig()), feeAmount)
		require.NoError(t, err)

		// Send only half bytes of metadata making it invalid
		bridgingRequestMetadata = bridgingRequestMetadata[0 : len(bridgingRequestMetadata)/2]

		_, err = cardanofw.SendTx(
			ctx, txProviderPrime, user.PrimeWallet, sendAmount+feeAmount, apex.Bridge.PrimeMultisigAddr,
			apex.PrimeCluster.NetworkConfig(), bridgingRequestMetadata)
		require.Error(t, err)
	})

	t.Run("Submitted invalid metadata - wrong type", func(t *testing.T) {
		sendAmount := uint64(1_000_000)
		feeAmount := uint64(1_100_000)

		receivers := map[string]uint64{
			user.VectorAddress:                sendAmount,
			apex.Bridge.VectorMultisigFeeAddr: feeAmount,
		}

		var transactions = make([]cardanofw.BridgingRequestMetadataTransaction, 0, len(receivers))
		for addr, amount := range receivers {
			transactions = append(transactions, cardanofw.BridgingRequestMetadataTransaction{
				Address: cardanofw.SplitString(addr, 40),
				Amount:  amount,
			})
		}

		metadata := map[string]interface{}{
			"1": map[string]interface{}{
				"t":  "transaction", // should be "bridge"
				"d":  cardanofw.GetDestinationChainID(apex.PrimeCluster.NetworkConfig()),
				"s":  cardanofw.SplitString(user.PrimeAddress, 40),
				"tx": transactions,
			},
		}

		bridgingRequestMetadata, err := json.Marshal(metadata)
		require.NoError(t, err)

		txHash, err := cardanofw.SendTx(
			ctx, txProviderPrime, user.PrimeWallet, sendAmount+feeAmount, apex.Bridge.PrimeMultisigAddr,
			apex.PrimeCluster.NetworkConfig(), bridgingRequestMetadata)
		require.NoError(t, err)

		apiURL, err := apex.Bridge.GetBridgingAPI()
		require.NoError(t, err)

		requestURL := fmt.Sprintf(
			"%s/api/BridgingRequestState/Get?chainId=%s&txHash=%s", apiURL, "prime", txHash)

		_, err = cardanofw.WaitForRequestState("InvalidState", ctx, requestURL, apiKey, 60)
		require.Error(t, err)
		require.ErrorContains(t, err, "Timeout")
	})

	t.Run("Submitted invalid metadata - invalid destination", func(t *testing.T) {
		sendAmount := uint64(1_000_000)
		feeAmount := uint64(1_100_000)

		receivers := map[string]uint64{
			user.VectorAddress:                sendAmount,
			apex.Bridge.VectorMultisigFeeAddr: feeAmount,
		}

		var transactions = make([]cardanofw.BridgingRequestMetadataTransaction, 0, len(receivers))
		for addr, amount := range receivers {
			transactions = append(transactions, cardanofw.BridgingRequestMetadataTransaction{
				Address: cardanofw.SplitString(addr, 40),
				Amount:  amount,
			})
		}

		metadata := map[string]interface{}{
			"1": map[string]interface{}{
				"t":  "bridge",
				"d":  "", // should be destination chain address
				"s":  cardanofw.SplitString(user.PrimeAddress, 40),
				"tx": transactions,
			},
		}

		bridgingRequestMetadata, err := json.Marshal(metadata)
		require.NoError(t, err)

		txHash, err := cardanofw.SendTx(
			ctx, txProviderPrime, user.PrimeWallet, sendAmount+feeAmount, apex.Bridge.PrimeMultisigAddr,
			apex.PrimeCluster.NetworkConfig(), bridgingRequestMetadata)
		require.NoError(t, err)

		apiURL, err := apex.Bridge.GetBridgingAPI()
		require.NoError(t, err)
		cardanofw.WaitForInvalidState(t, ctx, apiURL, apiKey, txHash)
	})

	t.Run("Submitted invalid metadata - invalid sender", func(t *testing.T) {
		sendAmount := uint64(1_000_000)
		feeAmount := uint64(1_100_000)

		receivers := map[string]uint64{
			user.VectorAddress:                sendAmount,
			apex.Bridge.VectorMultisigFeeAddr: feeAmount,
		}

		var transactions = make([]cardanofw.BridgingRequestMetadataTransaction, 0, len(receivers))
		for addr, amount := range receivers {
			transactions = append(transactions, cardanofw.BridgingRequestMetadataTransaction{
				Address: cardanofw.SplitString(addr, 40),
				Amount:  amount,
			})
		}

		metadata := map[string]interface{}{
			"1": map[string]interface{}{
				"t":  "bridge",
				"d":  cardanofw.GetDestinationChainID(apex.PrimeCluster.NetworkConfig()),
				"s":  "", // should be sender address (max len 40)
				"tx": transactions,
			},
		}

		bridgingRequestMetadata, err := json.Marshal(metadata)
		require.NoError(t, err)

		txHash, err := cardanofw.SendTx(
			ctx, txProviderPrime, user.PrimeWallet, sendAmount+feeAmount, apex.Bridge.PrimeMultisigAddr,
			apex.PrimeCluster.NetworkConfig(), bridgingRequestMetadata)
		require.NoError(t, err)

		apiURL, err := apex.Bridge.GetBridgingAPI()
		require.NoError(t, err)
		cardanofw.WaitForInvalidState(t, ctx, apiURL, apiKey, txHash)
	})

	t.Run("Submitted invalid metadata - empty tx", func(t *testing.T) {
		var transactions = make([]cardanofw.BridgingRequestMetadataTransaction, 0)

		metadata := map[string]interface{}{
			"1": map[string]interface{}{
				"t":  "bridge",
				"d":  cardanofw.GetDestinationChainID(apex.PrimeCluster.NetworkConfig()),
				"s":  cardanofw.SplitString(user.PrimeAddress, 40),
				"tx": transactions, // should not be empty
			},
		}

		bridgingRequestMetadata, err := json.Marshal(metadata)
		require.NoError(t, err)

		txHash, err := cardanofw.SendTx(
			ctx, txProviderPrime, user.PrimeWallet, 1_000_000, apex.Bridge.PrimeMultisigAddr,
			apex.PrimeCluster.NetworkConfig(), bridgingRequestMetadata)
		require.NoError(t, err)

		apiURL, err := apex.Bridge.GetBridgingAPI()
		require.NoError(t, err)
		cardanofw.WaitForInvalidState(t, ctx, apiURL, apiKey, txHash)
	})
}

func TestE2E_ApexBridge_ValidScenarios(t *testing.T) {
	const (
		apiKey               = "test_api_key"
		maxParallelInstances = 50
		fundAmount           = uint64(50_000_000_000)
	)

	var (
		err                error
		walletKeysPrime    = make([]wallet.IWallet, maxParallelInstances)
		walletKeysVector   = make([]wallet.IWallet, maxParallelInstances)
		primeClusterConfig = &cardanofw.RunCardanoClusterConfig{
			ID:                 0,
			NodesCount:         4,
			NetworkType:        wallet.TestNetNetwork,
			InitialFundsAmount: fundAmount,
			InitialFundsKeys:   make([]string, maxParallelInstances),
		}
		vectorClusterConfig = &cardanofw.RunCardanoClusterConfig{
			ID:                 1,
			NodesCount:         4,
			NetworkType:        wallet.VectorTestNetNetwork,
			InitialFundsAmount: fundAmount,
			InitialFundsKeys:   make([]string, maxParallelInstances),
		}
	)

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	for i := range walletKeysPrime {
		walletKeysPrime[i], err = wallet.GenerateWallet(false)
		require.NoError(t, err)

		walletKeysVector[i], err = wallet.GenerateWallet(false)
		require.NoError(t, err)

		addrPrime, err := wallet.NewEnterpriseAddress(primeClusterConfig.NetworkType, walletKeysPrime[i].GetVerificationKey())
		require.NoError(t, err)

		addrVec, err := wallet.NewEnterpriseAddress(vectorClusterConfig.NetworkType, walletKeysVector[i].GetVerificationKey())
		require.NoError(t, err)

		primeClusterConfig.InitialFundsKeys[i] = hex.EncodeToString(addrPrime.Bytes())
		vectorClusterConfig.InitialFundsKeys[i] = hex.EncodeToString(addrVec.Bytes())
	}

	apex := cardanofw.RunApexBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithPrimeClusterConfig(primeClusterConfig),
		cardanofw.WithVectorClusterConfig(vectorClusterConfig),
	)
	user := apex.CreateAndFundUser(t, ctx, uint64(20_000_000_000))

	txProviderPrime := apex.GetPrimeTxProvider()
	txProviderVector := apex.GetVectorTxProvider()

	primeGenesisWallet := apex.GetPrimeGenesisWallet(t)
	vectorGenesisWallet := apex.GetVectorGenesisWallet(t)

	primeGenesisAddress, err := cardanofw.GetAddress(apex.PrimeCluster.NetworkConfig().NetworkType, primeGenesisWallet)
	require.NoError(t, err)

	vectorGenesisAddress, err := cardanofw.GetAddress(apex.VectorCluster.NetworkConfig().NetworkType, vectorGenesisWallet)
	require.NoError(t, err)

	fmt.Println("prime genesis addr: ", primeGenesisAddress)
	fmt.Println("vector genesis addr: ", vectorGenesisAddress)
	fmt.Println("prime user addr: ", user.PrimeAddress)
	fmt.Println("vector user addr: ", user.VectorAddress)
	fmt.Println("prime multisig addr: ", apex.Bridge.PrimeMultisigAddr)
	fmt.Println("prime fee addr: ", apex.Bridge.PrimeMultisigFeeAddr)
	fmt.Println("vector multisig addr: ", apex.Bridge.VectorMultisigAddr)
	fmt.Println("vector fee addr: ", apex.Bridge.VectorMultisigFeeAddr)

	t.Run("From prime to vector one by one - wait for other side", func(t *testing.T) {
		if shouldRun := os.Getenv("RUN_E2E_REDUNDANT_TESTS"); shouldRun != "true" {
			t.Skip()
		}

		const (
			sendAmount = uint64(1_000_000)
			instances  = 5
		)

		for i := 0; i < instances; i++ {
			prevAmount, err := cardanofw.GetTokenAmount(ctx, txProviderVector, user.VectorAddress)
			require.NoError(t, err)

			fmt.Printf("%v - prevAmount %v\n", i+1, prevAmount)

			txHash := user.BridgeAmount(t, ctx, txProviderPrime, apex.Bridge.PrimeMultisigAddr,
				apex.Bridge.VectorMultisigFeeAddr, sendAmount, apex.PrimeCluster.NetworkConfig())

			fmt.Printf("%v - Tx sent. hash: %s\n", i+1, txHash)

			expectedAmount := prevAmount + sendAmount
			fmt.Printf("%v - expectedAmount %v\n", i+1, expectedAmount)

			err = cardanofw.WaitForAmount(ctx, txProviderVector, user.VectorAddress, func(val uint64) bool {
				return val == expectedAmount
			}, 20, time.Second*10)
			require.NoError(t, err)
		}
	})

	t.Run("From prime to vector one by one", func(t *testing.T) {
		if shouldRun := os.Getenv("RUN_E2E_REDUNDANT_TESTS"); shouldRun != "true" {
			t.Skip()
		}

		const (
			sendAmount = uint64(1_000_000)
			instances  = 5
		)

		prevAmount, err := cardanofw.GetTokenAmount(ctx, txProviderVector, user.VectorAddress)
		require.NoError(t, err)

		for i := 0; i < instances; i++ {
			txHash := user.BridgeAmount(t, ctx, txProviderPrime, apex.Bridge.PrimeMultisigAddr,
				apex.Bridge.VectorMultisigFeeAddr, sendAmount, apex.PrimeCluster.NetworkConfig())

			fmt.Printf("Tx %v sent. hash: %s\n", i+1, txHash)
		}

		expectedAmount := prevAmount + uint64(instances)*sendAmount
		err = cardanofw.WaitForAmount(ctx, txProviderVector, user.VectorAddress, func(val uint64) bool {
			return val == expectedAmount
		}, 20, time.Second*10)
		require.NoError(t, err)
	})

	//nolint:dupl
	t.Run("From prime to vector parallel", func(t *testing.T) {
		if shouldRun := os.Getenv("RUN_E2E_REDUNDANT_TESTS"); shouldRun != "true" {
			t.Skip()
		}

		const (
			sendAmount = uint64(1_000_000)
			instances  = 5
		)

		prevAmount, err := cardanofw.GetTokenAmount(ctx, txProviderVector, user.VectorAddress)
		require.NoError(t, err)

		var wg sync.WaitGroup
		for i := 0; i < instances; i++ {
			wg.Add(1)

			go func(idx int) {
				defer wg.Done()

				txHash := cardanofw.BridgeAmountFull(t, ctx, txProviderPrime, apex.PrimeCluster.NetworkConfig(),
					apex.Bridge.PrimeMultisigAddr, apex.Bridge.VectorMultisigFeeAddr, walletKeysPrime[idx],
					user.VectorAddress, sendAmount)
				fmt.Printf("Tx %v sent. hash: %s\n", idx+1, txHash)
			}(i)
		}

		wg.Wait()

		expectedAmount := prevAmount + uint64(instances)*sendAmount
		err = cardanofw.WaitForAmount(ctx, txProviderVector, user.VectorAddress, func(val uint64) bool {
			return val == expectedAmount
		}, 100, time.Second*10)
		require.NoError(t, err)
		fmt.Printf("%v TXs confirmed\n", instances)
	})

	t.Run("From vector to prime one by one", func(t *testing.T) {
		if shouldRun := os.Getenv("RUN_E2E_REDUNDANT_TESTS"); shouldRun != "true" {
			t.Skip()
		}

		const (
			sendAmount = uint64(1_000_000)
			instances  = 5
		)

		prevAmount, err := cardanofw.GetTokenAmount(ctx, txProviderPrime, user.PrimeAddress)
		require.NoError(t, err)

		for i := 0; i < instances; i++ {
			txHash := user.BridgeAmount(t, ctx, txProviderVector, apex.Bridge.VectorMultisigAddr,
				apex.Bridge.PrimeMultisigFeeAddr, sendAmount, apex.VectorCluster.NetworkConfig())

			fmt.Printf("Tx %v sent. hash: %s\n", i+1, txHash)
		}

		expectedAmount := prevAmount + uint64(instances)*sendAmount
		err = cardanofw.WaitForAmount(ctx, txProviderPrime, user.PrimeAddress, func(val uint64) bool {
			return val == expectedAmount
		}, 20, time.Second*10)
		require.NoError(t, err)
	})

	//nolint:dupl
	t.Run("From vector to prime parallel", func(t *testing.T) {
		if shouldRun := os.Getenv("RUN_E2E_REDUNDANT_TESTS"); shouldRun != "true" {
			t.Skip()
		}

		const (
			instances  = 5
			sendAmount = uint64(1_000_000)
		)

		prevAmount, err := cardanofw.GetTokenAmount(ctx, txProviderPrime, user.PrimeAddress)
		require.NoError(t, err)

		var wg sync.WaitGroup
		for i := 0; i < instances; i++ {
			wg.Add(1)

			go func(idx int) {
				defer wg.Done()

				txHash := cardanofw.BridgeAmountFull(t, ctx, txProviderVector, apex.VectorCluster.NetworkConfig(),
					apex.Bridge.VectorMultisigAddr, apex.Bridge.PrimeMultisigFeeAddr, walletKeysVector[idx],
					user.PrimeAddress, sendAmount)
				fmt.Printf("Tx %v sent. hash: %s\n", idx+1, txHash)
			}(i)
		}

		wg.Wait()

		expectedAmount := prevAmount + uint64(instances)*sendAmount
		err = cardanofw.WaitForAmount(ctx, txProviderPrime, user.PrimeAddress, func(val uint64) bool {
			return val == expectedAmount
		}, 100, time.Second*10)
		require.NoError(t, err)
		fmt.Printf("%v TXs confirmed\n", instances)
	})

	t.Run("From prime to vector sequential and parallel", func(t *testing.T) {
		if shouldRun := os.Getenv("RUN_E2E_REDUNDANT_TESTS"); shouldRun != "true" {
			t.Skip()
		}

		const (
			sequentialInstances = 5
			parallelInstances   = 10
			sendAmount          = uint64(1_000_000)
		)

		var wg sync.WaitGroup

		prevAmount, err := cardanofw.GetTokenAmount(ctx, txProviderVector, user.VectorAddress)
		require.NoError(t, err)

		for j := 0; j < sequentialInstances; j++ {
			for i := 0; i < parallelInstances; i++ {
				wg.Add(1)

				go func(run, idx int) {
					defer wg.Done()

					txHash := cardanofw.BridgeAmountFull(t, ctx, txProviderPrime, apex.PrimeCluster.NetworkConfig(),
						apex.Bridge.PrimeMultisigAddr, apex.Bridge.VectorMultisigFeeAddr, walletKeysPrime[idx],
						user.VectorAddress, sendAmount)
					fmt.Printf("run: %v. Tx %v sent. hash: %s\n", run+1, idx+1, txHash)
				}(j, i)
			}

			wg.Wait()
		}

		fmt.Printf("Waiting for %v TXs\n", sequentialInstances*parallelInstances)

		expectedAmount := prevAmount + uint64(sequentialInstances)*uint64(parallelInstances)*sendAmount
		err = cardanofw.WaitForAmount(ctx, txProviderVector, user.VectorAddress, func(val uint64) bool {
			return val == expectedAmount
		}, 100, time.Second*10)
		require.NoError(t, err)
		fmt.Printf("%v TXs confirmed\n", sequentialInstances*parallelInstances)
	})

	t.Run("From prime to vector sequential and parallel with max receivers", func(t *testing.T) {
		const (
			sequentialInstances = 5
			parallelInstances   = 10
			receivers           = 4
			sendAmount          = uint64(1_000_000)
		)

		var (
			wg                           sync.WaitGroup
			destinationWalletKeys        = make([]wallet.IWallet, receivers)
			destinationWalletAddresses   = make([]string, receivers)
			destinationWalletPrevAmounts = make([]uint64, receivers)
		)

		for i := 0; i < receivers; i++ {
			destinationWalletKeys[i], err = wallet.GenerateWallet(false)
			require.NoError(t, err)

			walletAddress, err := cardanofw.GetAddress(apex.VectorCluster.NetworkConfig().NetworkType, destinationWalletKeys[i])
			require.NoError(t, err)

			destinationWalletAddresses[i] = walletAddress.String()

			destinationWalletPrevAmounts[i], err = cardanofw.GetTokenAmount(ctx, txProviderVector, destinationWalletAddresses[i])
			require.NoError(t, err)
		}

		for j := 0; j < sequentialInstances; j++ {
			for i := 0; i < parallelInstances; i++ {
				wg.Add(1)

				go func(run, idx int) {
					defer wg.Done()

					txHash := cardanofw.BridgeAmountFullMultipleReceivers(t, ctx, txProviderPrime, apex.PrimeCluster.NetworkConfig(),
						apex.Bridge.PrimeMultisigAddr, apex.Bridge.VectorMultisigFeeAddr, walletKeysPrime[idx],
						destinationWalletAddresses, sendAmount)
					fmt.Printf("run: %v. Tx %v sent. hash: %s\n", run+1, idx+1, txHash)
				}(j, i)
			}

			wg.Wait()
		}

		fmt.Printf("Waiting for %v TXs\n", sequentialInstances*parallelInstances)

		var wgResult sync.WaitGroup

		for i := 0; i < receivers; i++ {
			wgResult.Add(1)

			go func(receiverIdx int) {
				defer wgResult.Done()

				expectedAmount := destinationWalletPrevAmounts[receiverIdx] + uint64(sequentialInstances)*uint64(parallelInstances)*sendAmount
				err = cardanofw.WaitForAmount(ctx, txProviderVector, destinationWalletAddresses[receiverIdx], func(val uint64) bool {
					return val == expectedAmount
				}, 100, time.Second*10)
				require.NoError(t, err)
				fmt.Printf("%v receiver, %v TXs confirmed\n", receiverIdx, sequentialInstances*parallelInstances)
			}(i)
		}

		wgResult.Wait()
	})

	t.Run("Both directions sequential", func(t *testing.T) {
		if shouldRun := os.Getenv("RUN_E2E_REDUNDANT_TESTS"); shouldRun != "true" {
			t.Skip()
		}

		const (
			instances  = 5
			sendAmount = uint64(1_000_000)
		)

		for i := 0; i < instances; i++ {
			primeTxHash := user.BridgeAmount(t, ctx, txProviderPrime, apex.Bridge.PrimeMultisigAddr,
				apex.Bridge.VectorMultisigFeeAddr, sendAmount, apex.PrimeCluster.NetworkConfig())

			fmt.Printf("prime tx %v sent. hash: %s\n", i+1, primeTxHash)

			vectorTxHash := user.BridgeAmount(t, ctx, txProviderVector, apex.Bridge.VectorMultisigAddr,
				apex.Bridge.PrimeMultisigFeeAddr, sendAmount, apex.VectorCluster.NetworkConfig())

			fmt.Printf("vector tx %v sent. hash: %s\n", i+1, vectorTxHash)
		}

		prevAmountOnVector, err := cardanofw.GetTokenAmount(ctx, txProviderVector, user.VectorAddress)
		require.NoError(t, err)
		prevAmountOnPrime, err := cardanofw.GetTokenAmount(ctx, txProviderPrime, user.PrimeAddress)
		require.NoError(t, err)

		fmt.Printf("Waiting for %v TXs on vector\n", instances)
		expectedAmountOnVector := prevAmountOnVector + uint64(instances)*sendAmount
		err = cardanofw.WaitForAmount(ctx, txProviderVector, user.VectorAddress, func(val uint64) bool {
			return val == expectedAmountOnVector
		}, 100, time.Second*10)
		require.NoError(t, err)

		fmt.Printf("Waiting for %v TXs on prime\n", instances)
		expectedAmountOnPrime := prevAmountOnPrime + uint64(instances)*sendAmount
		err = cardanofw.WaitForAmount(ctx, txProviderPrime, user.PrimeAddress, func(val uint64) bool {
			return val == expectedAmountOnPrime
		}, 100, time.Second*10)
		require.NoError(t, err)
	})

	t.Run("Both directions sequential and parallel - one node goes off in the middle", func(t *testing.T) {
		const (
			sequentialInstances  = 5
			parallelInstances    = 6
			stopAfter            = time.Second * 60
			validatorStoppingIdx = 1
			sendAmount           = uint64(1_000_000)
		)

		prevAmountOnVector, err := cardanofw.GetTokenAmount(ctx, txProviderVector, user.VectorAddress)
		require.NoError(t, err)
		prevAmountOnPrime, err := cardanofw.GetTokenAmount(ctx, txProviderPrime, user.PrimeAddress)
		require.NoError(t, err)

		go func() {
			select {
			case <-ctx.Done():
				return
			case <-time.After(stopAfter):
				require.NoError(t, apex.Bridge.GetValidator(t, validatorStoppingIdx).Stop())
			}
		}()

		var wg sync.WaitGroup

		for i := 0; i < parallelInstances; i++ {
			wg.Add(2)

			go func(idx int) {
				defer wg.Done()

				for j := 0; j < sequentialInstances; j++ {
					txHash := cardanofw.BridgeAmountFull(t, ctx, txProviderPrime, apex.PrimeCluster.NetworkConfig(),
						apex.Bridge.PrimeMultisigAddr, apex.Bridge.VectorMultisigFeeAddr, walletKeysPrime[idx],
						user.VectorAddress, sendAmount)
					fmt.Printf("run: %d. Prime tx %d sent. hash: %s\n", idx+1, j+1, txHash)
				}
			}(i)

			go func(idx int) {
				defer wg.Done()

				for j := 0; j < sequentialInstances; j++ {
					txHash := cardanofw.BridgeAmountFull(t, ctx, txProviderVector, apex.VectorCluster.NetworkConfig(),
						apex.Bridge.VectorMultisigAddr, apex.Bridge.PrimeMultisigFeeAddr, walletKeysVector[idx],
						user.PrimeAddress, sendAmount)
					fmt.Printf("run: %d. Vector tx %d sent. hash: %s\n", idx+1, j+1, txHash)
				}
			}(i)
		}

		wg.Wait()

		wg.Add(2)

		//nolint:dupl
		go func() {
			defer wg.Done()

			fmt.Printf("Waiting for %v TXs on vector:\n", sequentialInstances*parallelInstances)

			expectedAmountOnVector := prevAmountOnVector + uint64(sequentialInstances)*uint64(parallelInstances)*sendAmount
			err := cardanofw.WaitForAmount(ctx, txProviderVector, user.VectorAddress, func(val uint64) bool {
				return val == expectedAmountOnVector
			}, 100, time.Second*10)
			assert.NoError(t, err)

			fmt.Printf("TXs on vector expected amount received: %v\n", err)

			// nothing else should be bridged for 2 minutes
			err = cardanofw.WaitForAmount(ctx, txProviderVector, user.VectorAddress, func(val uint64) bool {
				return val > expectedAmountOnVector
			}, 12, time.Second*10)
			assert.ErrorIs(t, err, wallet.ErrWaitForTransactionTimeout, "more tokens than expected are on vector")

			fmt.Printf("TXs on vector finished with success: %v\n", err != nil)
		}()

		//nolint:dupl
		go func() {
			defer wg.Done()

			fmt.Printf("Waiting for %v TXs on prime\n", sequentialInstances*parallelInstances)

			expectedAmountOnPrime := prevAmountOnPrime + uint64(sequentialInstances)*uint64(parallelInstances)*sendAmount
			err := cardanofw.WaitForAmount(ctx, txProviderPrime, user.PrimeAddress, func(val uint64) bool {
				return val == expectedAmountOnPrime
			}, 100, time.Second*10)
			assert.NoError(t, err)

			fmt.Printf("TXs on prime expected amount received: %v\n", err)

			// nothing else should be bridged for 2 minutes
			err = cardanofw.WaitForAmount(ctx, txProviderPrime, user.PrimeAddress, func(val uint64) bool {
				return val > expectedAmountOnPrime
			}, 12, time.Second*10)
			assert.ErrorIs(t, err, wallet.ErrWaitForTransactionTimeout, "more tokens than expected are on prime")

			fmt.Printf("TXs on prime finished with success: %v\n", err != nil)
		}()

		wg.Wait()
	})

	t.Run("Both directions sequential and parallel - two nodes goes off in the middle and then one comes back", func(t *testing.T) {
		const (
			sequentialInstances   = 5
			parallelInstances     = 10
			stopAfter             = time.Second * 60
			startAgainAfter       = time.Second * 120
			validatorStoppingIdx1 = 1
			validatorStoppingIdx2 = 2
			sendAmount            = uint64(1_000_000)
		)

		prevAmountOnVector, err := cardanofw.GetTokenAmount(ctx, txProviderVector, user.VectorAddress)
		require.NoError(t, err)
		prevAmountOnPrime, err := cardanofw.GetTokenAmount(ctx, txProviderPrime, user.PrimeAddress)
		require.NoError(t, err)

		go func() {
			select {
			case <-ctx.Done():
				return
			case <-time.After(stopAfter):
				require.NoError(t, apex.Bridge.GetValidator(t, validatorStoppingIdx1).Stop())
				require.NoError(t, apex.Bridge.GetValidator(t, validatorStoppingIdx2).Stop())
			}

			select {
			case <-ctx.Done():
				return
			case <-time.After(startAgainAfter):
				require.NoError(t, apex.Bridge.GetValidator(t, validatorStoppingIdx1).Start(ctx, false))
			}
		}()

		var wg sync.WaitGroup

		for i := 0; i < parallelInstances; i++ {
			wg.Add(2)

			go func(idx int) {
				defer wg.Done()

				for j := 0; j < sequentialInstances; j++ {
					txHash := cardanofw.BridgeAmountFull(t, ctx, txProviderPrime, apex.PrimeCluster.NetworkConfig(),
						apex.Bridge.PrimeMultisigAddr, apex.Bridge.VectorMultisigFeeAddr, walletKeysPrime[idx],
						user.VectorAddress, sendAmount)
					fmt.Printf("run: %d. Prime tx %d sent. hash: %s\n", idx+1, j+1, txHash)
				}
			}(i)

			go func(idx int) {
				defer wg.Done()

				for j := 0; j < sequentialInstances; j++ {
					txHash := cardanofw.BridgeAmountFull(t, ctx, txProviderVector, apex.VectorCluster.NetworkConfig(),
						apex.Bridge.VectorMultisigAddr, apex.Bridge.PrimeMultisigFeeAddr, walletKeysVector[idx],
						user.PrimeAddress, sendAmount)
					fmt.Printf("run: %d. Vector tx %d sent. hash: %s\n", idx+1, j+1, txHash)
				}
			}(i)
		}

		wg.Wait()

		wg.Add(2)

		//nolint:dupl
		go func() {
			defer wg.Done()

			fmt.Printf("Waiting for %v TXs on vector:\n", sequentialInstances*parallelInstances)

			expectedAmountOnVector := prevAmountOnVector + uint64(sequentialInstances)*uint64(parallelInstances)*sendAmount
			err := cardanofw.WaitForAmount(ctx, txProviderVector, user.VectorAddress, func(val uint64) bool {
				return val == expectedAmountOnVector
			}, 100, time.Second*10)
			assert.NoError(t, err)

			fmt.Printf("TXs on vector expected amount received: %v\n", err)

			if err != nil {
				return
			}

			// nothing else should be bridged for 2 minutes
			err = cardanofw.WaitForAmount(ctx, txProviderVector, user.VectorAddress, func(val uint64) bool {
				return val > expectedAmountOnVector
			}, 12, time.Second*10)
			assert.ErrorIs(t, err, wallet.ErrWaitForTransactionTimeout, "more tokens than expected are on vector")

			fmt.Printf("TXs on vector finished with success: %v\n", err != nil)
		}()

		//nolint:dupl
		go func() {
			defer wg.Done()

			fmt.Printf("Waiting for %v TXs on prime\n", sequentialInstances*parallelInstances)

			expectedAmountOnPrime := prevAmountOnPrime + uint64(sequentialInstances)*uint64(parallelInstances)*sendAmount
			err := cardanofw.WaitForAmount(ctx, txProviderPrime, user.PrimeAddress, func(val uint64) bool {
				return val == expectedAmountOnPrime
			}, 100, time.Second*10)
			assert.NoError(t, err)

			fmt.Printf("TXs on prime expected amount received: %v\n", err)

			if err != nil {
				return
			}

			// nothing else should be bridged for 2 minutes
			err = cardanofw.WaitForAmount(ctx, txProviderPrime, user.PrimeAddress, func(val uint64) bool {
				return val > expectedAmountOnPrime
			}, 12, time.Second*10)
			assert.ErrorIs(t, err, wallet.ErrWaitForTransactionTimeout, "more tokens than expected are on prime")

			fmt.Printf("TXs on prime finished with success: %v\n", err != nil)
		}()

		wg.Wait()
	})

	t.Run("Both directions sequential and parallel", func(t *testing.T) {
		const (
			sequentialInstances = 5
			parallelInstances   = 6
		)

		prevAmountOnVector, err := cardanofw.GetTokenAmount(ctx, txProviderVector, user.VectorAddress)
		require.NoError(t, err)
		prevAmountOnPrime, err := cardanofw.GetTokenAmount(ctx, txProviderPrime, user.PrimeAddress)
		require.NoError(t, err)

		sendAmount := uint64(1_000_000)

		for j := 0; j < sequentialInstances; j++ {
			var wg sync.WaitGroup

			for i := 0; i < parallelInstances; i++ {
				wg.Add(1)

				go func(run, idx int) {
					defer wg.Done()

					txHash := cardanofw.BridgeAmountFull(t, ctx, txProviderPrime, apex.PrimeCluster.NetworkConfig(),
						apex.Bridge.PrimeMultisigAddr, apex.Bridge.VectorMultisigFeeAddr, walletKeysPrime[idx],
						user.VectorAddress, sendAmount)
					fmt.Printf("run: %v. Prime tx %v sent. hash: %s\n", run+1, idx+1, txHash)
				}(j, i)
			}

			wg.Wait()

			for i := 0; i < parallelInstances; i++ {
				wg.Add(1)

				go func(run, idx int) {
					defer wg.Done()

					txHash := cardanofw.BridgeAmountFull(t, ctx, txProviderVector, apex.VectorCluster.NetworkConfig(),
						apex.Bridge.VectorMultisigAddr, apex.Bridge.PrimeMultisigFeeAddr, walletKeysVector[idx],
						user.PrimeAddress, sendAmount)
					fmt.Printf("run: %v. Vector tx %v sent. hash: %s\n", run+1, idx+1, txHash)
				}(j, i)
			}

			wg.Wait()
		}

		fmt.Printf("Waiting for %v TXs on vector:\n", sequentialInstances*parallelInstances)

		expectedAmountOnVector := prevAmountOnVector + uint64(sequentialInstances)*uint64(parallelInstances)*sendAmount
		err = cardanofw.WaitForAmount(ctx, txProviderVector, user.VectorAddress, func(val uint64) bool {
			return val == expectedAmountOnVector
		}, 100, time.Second*10)
		require.NoError(t, err)
		fmt.Printf("%v TXs on vector confirmed\n", sequentialInstances*parallelInstances)

		fmt.Printf("Waiting for %v TXs on prime\n", sequentialInstances*parallelInstances)

		expectedAmountOnPrime := prevAmountOnPrime + uint64(sequentialInstances)*uint64(parallelInstances)*sendAmount
		err = cardanofw.WaitForAmount(ctx, txProviderPrime, user.PrimeAddress, func(val uint64) bool {
			return val == expectedAmountOnPrime
		}, 100, time.Second*10)
		require.NoError(t, err)
		fmt.Printf("%v TXs on prime confirmed\n", sequentialInstances*parallelInstances)
	})
}

func TestE2E_ApexBridge_ValidScenarios_BigTests(t *testing.T) {
	if shouldRun := os.Getenv("RUN_E2E_BIG_TESTS"); shouldRun != "true" {
		t.Skip()
	}

	const (
		apiKey = "test_api_key"
	)

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	apex := cardanofw.RunApexBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
	)
	user := apex.CreateAndFundUser(t, ctx, uint64(20_000_000_000))

	txProviderPrime := apex.GetPrimeTxProvider()
	txProviderVector := apex.GetVectorTxProvider()

	primeGenesisWallet := apex.GetPrimeGenesisWallet(t)
	vectorGenesisWallet := apex.GetVectorGenesisWallet(t)

	primeGenesisAddress, err := cardanofw.GetAddress(apex.PrimeCluster.NetworkConfig().NetworkType, primeGenesisWallet)
	require.NoError(t, err)

	vectorGenesisAddress, err := cardanofw.GetAddress(apex.VectorCluster.NetworkConfig().NetworkType, vectorGenesisWallet)
	require.NoError(t, err)

	fmt.Println("prime genesis addr: ", primeGenesisAddress)
	fmt.Println("vector genesis addr: ", vectorGenesisAddress)
	fmt.Println("prime user addr: ", user.PrimeAddress)
	fmt.Println("vector user addr: ", user.VectorAddress)
	fmt.Println("prime multisig addr: ", apex.Bridge.PrimeMultisigAddr)
	fmt.Println("prime fee addr: ", apex.Bridge.PrimeMultisigFeeAddr)
	fmt.Println("vector multisig addr: ", apex.Bridge.VectorMultisigAddr)
	fmt.Println("vector fee addr: ", apex.Bridge.VectorMultisigFeeAddr)

	//nolint:dupl
	t.Run("From prime to vector 200x 5min 90%", func(t *testing.T) {
		instances := 200
		maxWaitTime := 300
		fundSendAmount := uint64(5_000_000)
		sendAmount := uint64(1_000_000)
		successChance := 90 // 90%
		succeededCount := 0
		walletKeys := make([]wallet.IWallet, instances)

		prevAmount, err := cardanofw.GetTokenAmount(ctx, txProviderVector, user.VectorAddress)
		require.NoError(t, err)

		fmt.Printf("Funding %v Wallets\n", instances)

		for i := 0; i < instances; i++ {
			if (i+1)%100 == 0 {
				fmt.Printf("Funded %v..%v\n", i-99, i)
			}

			walletKeys[i], err = wallet.GenerateWallet(false)
			require.NoError(t, err)

			walletAddress, err := cardanofw.GetAddress(apex.PrimeCluster.NetworkConfig().NetworkType, walletKeys[i])
			require.NoError(t, err)
			require.NoError(t, err)

			user.SendToAddress(t, ctx, txProviderPrime, primeGenesisWallet, fundSendAmount, walletAddress.String(), apex.PrimeCluster.NetworkConfig())
		}

		fmt.Printf("Funding Complete\n")
		fmt.Printf("Sending %v transactions in %v seconds\n", instances, maxWaitTime)

		var wg sync.WaitGroup
		for i := 0; i < instances; i++ {
			wg.Add(1)

			go func(idx int) {
				defer wg.Done()

				if successChance > rand.Intn(100) {
					succeededCount++
					sleepTime := rand.Intn(maxWaitTime)
					time.Sleep(time.Second * time.Duration(sleepTime))

					cardanofw.BridgeAmountFull(t, ctx, txProviderPrime, apex.PrimeCluster.NetworkConfig(),
						apex.Bridge.PrimeMultisigAddr, apex.Bridge.VectorMultisigFeeAddr, walletKeys[idx], user.VectorAddress, sendAmount)
				} else {
					feeAmount := uint64(1_100_000)
					receivers := map[string]uint64{
						user.VectorAddress: sendAmount * 10, // 10Ada
					}

					bridgingRequestMetadata, err := cardanofw.CreateMetaData(
						user.PrimeAddress, receivers, cardanofw.GetDestinationChainID(apex.PrimeCluster.NetworkConfig()), feeAmount)
					require.NoError(t, err)

					_, err = cardanofw.SendTx(
						ctx, txProviderPrime, walletKeys[idx], sendAmount+feeAmount, apex.Bridge.PrimeMultisigAddr,
						apex.PrimeCluster.NetworkConfig(), bridgingRequestMetadata)
					require.NoError(t, err)
				}
			}(i)
		}

		wg.Wait()

		fmt.Printf("All tx sent, waiting for confirmation.\n")

		expectedAmount := prevAmount + uint64(succeededCount)*sendAmount

		var newAmount uint64
		for i := 0; i < instances; i++ {
			newAmount, err = cardanofw.GetTokenAmount(ctx, txProviderVector, user.VectorAddress)
			require.NoError(t, err)
			fmt.Printf("Current Amount Prime: %v. Expected: %v\n", newAmount, expectedAmount)

			if newAmount == expectedAmount {
				break
			}

			time.Sleep(time.Second * 10)
		}

		fmt.Printf("Success count: %v. prevAmount: %v. newAmount: %v. expectedAmount: %v\n", succeededCount, prevAmount, newAmount, expectedAmount)
	})

	//nolint:dupl
	t.Run("From prime to vector 1000x 20min 90%", func(t *testing.T) {
		instances := 1000
		maxWaitTime := 1200
		fundSendAmount := uint64(5_000_000)
		sendAmount := uint64(1_000_000)
		successChance := 90 // 90%
		succeededCount := 0
		walletKeys := make([]wallet.IWallet, instances)

		prevAmount, err := cardanofw.GetTokenAmount(ctx, txProviderVector, user.VectorAddress)
		require.NoError(t, err)

		fmt.Printf("Funding %v Wallets\n", instances)

		for i := 0; i < instances; i++ {
			if (i+1)%100 == 0 {
				fmt.Printf("Funded %v..%v\n", i-99, i)
			}

			walletKeys[i], err = wallet.GenerateWallet(false)
			require.NoError(t, err)

			walletAddress, err := cardanofw.GetAddress(apex.PrimeCluster.NetworkConfig().NetworkType, walletKeys[i])
			require.NoError(t, err)

			user.SendToAddress(t, ctx, txProviderPrime, primeGenesisWallet, fundSendAmount, walletAddress.String(), apex.PrimeCluster.NetworkConfig())
		}

		fmt.Printf("Funding Complete\n")
		fmt.Printf("Sending %v transactions in %v seconds\n", instances, maxWaitTime)

		var wg sync.WaitGroup
		for i := 0; i < instances; i++ {
			wg.Add(1)

			go func(idx int) {
				defer wg.Done()

				if successChance > rand.Intn(100) {
					succeededCount++
					sleepTime := rand.Intn(maxWaitTime)
					time.Sleep(time.Second * time.Duration(sleepTime))

					cardanofw.BridgeAmountFull(t, ctx, txProviderPrime, apex.PrimeCluster.NetworkConfig(),
						apex.Bridge.PrimeMultisigAddr, apex.Bridge.VectorMultisigFeeAddr, walletKeys[idx], user.VectorAddress, sendAmount)
				} else {
					feeAmount := uint64(1_100_000)
					receivers := map[string]uint64{
						user.VectorAddress: sendAmount * 10, // 10Ada+
					}

					bridgingRequestMetadata, err := cardanofw.CreateMetaData(
						user.PrimeAddress, receivers, cardanofw.GetDestinationChainID(apex.PrimeCluster.NetworkConfig()), feeAmount)
					require.NoError(t, err)

					_, err = cardanofw.SendTx(
						ctx, txProviderPrime, walletKeys[idx], sendAmount+feeAmount, apex.Bridge.PrimeMultisigAddr,
						apex.PrimeCluster.NetworkConfig(), bridgingRequestMetadata)
					require.NoError(t, err)
				}
			}(i)
		}

		wg.Wait()

		fmt.Printf("All tx sent, waiting for confirmation.\n")

		expectedAmount := prevAmount + uint64(succeededCount)*sendAmount

		var newAmount uint64
		for i := 0; i < instances; i++ {
			newAmount, err = cardanofw.GetTokenAmount(ctx, txProviderVector, user.VectorAddress)
			require.NoError(t, err)
			fmt.Printf("Current Amount Prime: %v. Expected: %v\n", newAmount, expectedAmount)

			if newAmount == expectedAmount {
				break
			}

			time.Sleep(time.Second * 10)
		}

		fmt.Printf("Success count: %v. prevAmount: %v. newAmount: %v. expectedAmount: %v\n", succeededCount, prevAmount, newAmount, expectedAmount)
	})

	t.Run("Both directions 1000x 60min 90%", func(t *testing.T) {
		instances := 1000
		maxWaitTime := 3600
		fundSendAmount := uint64(5_000_000)
		sendAmount := uint64(1_000_000)
		successChance := 90 // 90%
		succeededCountPrime := 0
		succeededCountVector := 0
		walletKeysPrime := make([]wallet.IWallet, instances)
		walletKeysVector := make([]wallet.IWallet, instances)

		prevAmountPrime, err := cardanofw.GetTokenAmount(ctx, txProviderVector, user.VectorAddress)
		require.NoError(t, err)
		prevAmountVector, err := cardanofw.GetTokenAmount(ctx, txProviderPrime, user.PrimeAddress)
		require.NoError(t, err)

		fmt.Printf("Funding %v Wallets\n", instances*2)

		for i := 0; i < instances; i++ {
			if (i+1)%100 == 0 {
				fmt.Printf("Funded %v..%v on prime\n", i-99, i)
			}

			walletKeysPrime[i], err = wallet.GenerateWallet(false)
			require.NoError(t, err)

			walletAddress, err := cardanofw.GetAddress(apex.PrimeCluster.NetworkConfig().NetworkType, walletKeysPrime[i])
			require.NoError(t, err)

			user.SendToAddress(t, ctx, txProviderPrime, primeGenesisWallet, fundSendAmount, walletAddress.String(), apex.PrimeCluster.NetworkConfig())
		}

		for i := 0; i < instances; i++ {
			if (i+1)%100 == 0 {
				fmt.Printf("Funded %v..%v on vector\n", i-99, i)
			}

			walletKeysVector[i], err = wallet.GenerateWallet(false)
			require.NoError(t, err)

			walletAddress, err := cardanofw.GetAddress(apex.VectorCluster.NetworkConfig().NetworkType, walletKeysVector[i])
			require.NoError(t, err)

			user.SendToAddress(t, ctx, txProviderVector, vectorGenesisWallet, fundSendAmount, walletAddress.String(), apex.VectorCluster.NetworkConfig())
		}

		fmt.Printf("Funding Complete\n")
		fmt.Printf("Sending %v transactions in %v seconds\n", instances*2, maxWaitTime)

		var wg sync.WaitGroup
		for i := 0; i < instances; i++ {
			wg.Add(2)

			//nolint:dupl
			go func(idx int) {
				defer wg.Done()

				if successChance > rand.Intn(100) {
					succeededCountPrime++
					sleepTime := rand.Intn(maxWaitTime)
					time.Sleep(time.Second * time.Duration(sleepTime))

					cardanofw.BridgeAmountFull(t, ctx, txProviderPrime, apex.PrimeCluster.NetworkConfig(),
						apex.Bridge.PrimeMultisigAddr, apex.Bridge.VectorMultisigFeeAddr, walletKeysPrime[idx], user.VectorAddress, sendAmount)
				} else {
					feeAmount := uint64(1_100_000)
					receivers := map[string]uint64{
						user.VectorAddress: sendAmount * 10, // 10Ada+
					}

					bridgingRequestMetadata, err := cardanofw.CreateMetaData(
						user.PrimeAddress, receivers, cardanofw.GetDestinationChainID(apex.PrimeCluster.NetworkConfig()), feeAmount)
					require.NoError(t, err)

					_, err = cardanofw.SendTx(
						ctx, txProviderPrime, walletKeysPrime[idx], sendAmount+feeAmount, apex.Bridge.PrimeMultisigAddr,
						apex.PrimeCluster.NetworkConfig(), bridgingRequestMetadata)
					require.NoError(t, err)
				}
			}(i)

			//nolint:dupl
			go func(idx int) {
				defer wg.Done()

				if successChance > rand.Intn(100) {
					succeededCountVector++
					sleepTime := rand.Intn(maxWaitTime)
					time.Sleep(time.Second * time.Duration(sleepTime))

					cardanofw.BridgeAmountFull(t, ctx, txProviderVector, apex.VectorCluster.NetworkConfig(),
						apex.Bridge.VectorMultisigAddr, apex.Bridge.PrimeMultisigFeeAddr, walletKeysVector[idx], user.PrimeAddress, sendAmount)
				} else {
					feeAmount := uint64(1_100_000)
					receivers := map[string]uint64{
						user.PrimeAddress: sendAmount * 10, // 10Ada+
					}

					bridgingRequestMetadata, err := cardanofw.CreateMetaData(
						user.VectorAddress, receivers, cardanofw.GetDestinationChainID(apex.VectorCluster.NetworkConfig()), feeAmount)
					require.NoError(t, err)

					_, err = cardanofw.SendTx(
						ctx, txProviderVector, walletKeysVector[idx], sendAmount+feeAmount, apex.Bridge.VectorMultisigAddr,
						apex.VectorCluster.NetworkConfig(), bridgingRequestMetadata)
					require.NoError(t, err)
				}
			}(i)
		}

		wg.Wait()

		fmt.Printf("All tx sent, waiting for confirmation.\n")

		expectedAmountPrime := prevAmountPrime + uint64(succeededCountPrime)*sendAmount
		expectedAmountVector := prevAmountVector + uint64(succeededCountVector)*sendAmount

		for i := 0; i < instances; i++ {
			newAmountPrime, err := cardanofw.GetTokenAmount(ctx, txProviderVector, user.VectorAddress)
			require.NoError(t, err)
			fmt.Printf("Current Amount Prime: %v. Expected: %v\n", newAmountPrime, expectedAmountPrime)

			newAmountVector, err := cardanofw.GetTokenAmount(ctx, txProviderPrime, user.PrimeAddress)
			require.NoError(t, err)
			fmt.Printf("Current Amount Vector: %v. Expected: %v\n", newAmountVector, expectedAmountVector)

			if newAmountPrime == expectedAmountPrime && newAmountVector == expectedAmountVector {
				break
			}

			time.Sleep(time.Second * 10)
		}

		newAmountPrime, err := cardanofw.GetTokenAmount(ctx, txProviderVector, user.VectorAddress)
		require.NoError(t, err)
		fmt.Printf("Success count: %v. prevAmount: %v. newAmount: %v. expectedAmount: %v\n", succeededCountPrime, prevAmountPrime, newAmountPrime, expectedAmountPrime)

		newAmountVector, err := cardanofw.GetTokenAmount(ctx, txProviderPrime, user.PrimeAddress)
		require.NoError(t, err)
		fmt.Printf("Success count: %v. prevAmount: %v. newAmount: %v. expectedAmount: %v\n", succeededCountVector, prevAmountVector, newAmountVector, expectedAmountVector)
	})
}
