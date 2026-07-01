package e2e

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"math/big"
	"math/rand"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/cardanofw"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/e2ehelper"
	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/jsonrpc"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
	infracommon "github.com/Ethernal-Tech/cardano-infrastructure/common"
	"github.com/Ethernal-Tech/cardano-infrastructure/sendtx"
	infrawallet "github.com/Ethernal-Tech/cardano-infrastructure/wallet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Download Cardano executables from https://github.com/IntersectMBO/cardano-node/releases/tag/8.7.3 and unpack tar.gz file
// Add directory where unpacked files are located to the $PATH (in example bellow `~/Apps/cardano`)
// eq add line `export PATH=$PATH:~/Apps/cardano` to  `~/.bashrc`
// cd e2e-polybft/e2e
// ONLY_RUN_APEX_BRIDGE=true go test -v -timeout 0 -run ^Test_OnlyRunApexBridge_WithNexusAndVector$ github.com/0xPolygon/polygon-edge/e2e-polybft/e2e
func Test_OnlyRunApexBridge_WithNexusAndVector(t *testing.T) {
	if !cardanofw.IsEnvVarTrue("ONLY_RUN_APEX_BRIDGE") {
		t.Skip()
	}

	const (
		apiKey = "test_api_key"
	)

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	apex := cardanofw.SetupAndRunReactorBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithNexusEnabled(true),
		cardanofw.WithTelemetryConfig(cardanofw.PrometheusTelemetry),
		cardanofw.WithUserCnt(1),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	oracleAPI, err := apex.GetBridgingAPI()
	require.NoError(t, err)

	fmt.Printf("oracle API: %s\n", oracleAPI)
	fmt.Printf("oracle API key: %s\n", apiKey)

	fmt.Printf("prime network url: %s\n", apex.PrimeInfo.NetworkAddress)
	fmt.Printf("prime ogmios url: %s\n", apex.PrimeInfo.OgmiosURL)
	fmt.Printf("prime bridging addr: %s\n", apex.PrimeInfo.MultisigAddr)
	fmt.Printf("prime fee addr: %s\n", apex.PrimeInfo.FeeAddr)
	fmt.Printf("prime socket path: %s\n", apex.PrimeInfo.SocketPath)

	fmt.Printf("vector network url: %s\n", apex.VectorInfo.NetworkAddress)
	fmt.Printf("vector ogmios url: %s\n", apex.VectorInfo.OgmiosURL)
	fmt.Printf("vector bridging addr: %s\n", apex.VectorInfo.MultisigAddr)
	fmt.Printf("vector fee addr: %s\n", apex.VectorInfo.FeeAddr)
	fmt.Printf("vector socket path: %s\n", apex.VectorInfo.SocketPath)

	user := apex.Users[0]
	userPrimeSK, err := user.GetPrivateKey(cardanofw.ChainIDPrime)
	require.NoError(t, err)
	userVectorSK, err := user.GetPrivateKey(cardanofw.ChainIDVector)
	require.NoError(t, err)
	userNexusPK, err := user.GetPrivateKey(cardanofw.ChainIDNexus)
	require.NoError(t, err)

	nexusAdminKeyRaw, err := apex.NexusInfo.AdminKey.MarshallPrivateKey()
	require.NoError(t, err)

	fmt.Printf("user prime addr: %s\n", user.GetAddress(cardanofw.ChainIDPrime))
	fmt.Printf("user prime signing key hex: %s\n", userPrimeSK)
	fmt.Printf("user vector addr: %s\n", user.GetAddress(cardanofw.ChainIDVector))
	fmt.Printf("user vector signing key hex: %s\n", userVectorSK)

	jsonRPCClient, err := cardanofw.JSONRPCClient(apex.NexusInfo.JSONRPCAddr)
	require.NoError(t, err)

	nexusChainID, err := jsonRPCClient.ChainID()
	require.NoError(t, err)

	fmt.Printf("nexus user addr: %s\n", user.GetAddress(cardanofw.ChainIDNexus))
	fmt.Printf("nexus user signing key: %s\n", userNexusPK)
	fmt.Printf("nexus url: %s\n", apex.NexusInfo.JSONRPCAddr)
	fmt.Printf("nexus gateway sc addr: %s\n", apex.NexusInfo.GatewayAddress)
	fmt.Printf("nexus chainID: %v\n", nexusChainID)
	fmt.Printf("nexus admin key: %v\n", hex.EncodeToString(nexusAdminKeyRaw))

	proxyAdminPrivateKeyRaw, err := apex.GetBridgeProxyAdmin().MarshallPrivateKey()
	require.NoError(t, err)

	privateKeyRaw, err := apex.GetBridgeAdmin().MarshallPrivateKey()
	require.NoError(t, err)

	fmt.Printf("bridge url: %s\n", apex.GetBridgeDefaultJSONRPCAddr())
	fmt.Printf("bridge admin key: %s\n", hex.EncodeToString(privateKeyRaw))
	fmt.Printf("bridge admin address: %s\n", apex.GetBridgeAdmin().Address())
	fmt.Printf("bridge proxy admin key: %s\n", hex.EncodeToString(proxyAdminPrivateKeyRaw))
	fmt.Printf("bridge proxy admin address: %s\n", apex.GetBridgeProxyAdmin().Address())

	for i := 0; i < apex.GetValidatorsCount(); i++ {
		fmt.Printf("validator %d `--telemetry` flag telemetry url(s): %s\n",
			i+1, apex.Config.GetTelemetryForValidatorIdx(i))
	}

	signalChannel := make(chan os.Signal, 1)
	// Notify the signalChannel when the interrupt signal is received (Ctrl+C)
	signal.Notify(signalChannel, os.Interrupt, syscall.SIGTERM)

	<-signalChannel
}

func TestE2E_ApexBridge_UpdateApexBridgeSmartContract(t *testing.T) {
	if cardanofw.ShouldSkipE2RRedundantTests() {
		t.Skip()
	}

	currentWorkingDir, err := os.Getwd()
	require.NoError(t, err)

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	apex := cardanofw.SetupAndRunReactorBridge(
		t, ctx,
		cardanofw.WithAPIValidatorID(-2),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	txRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithClient(apex.BridgeCluster.Servers[0].JSONRPC()))
	require.NoError(t, err)

	privateKeyRaw, err := apex.GetBridgeProxyAdmin().MarshallPrivateKey()
	require.NoError(t, err)

	tmpPath, err := os.MkdirTemp("", "TestE2E_ApexBridge_UpdateApexBridgeSmartContract")
	require.NoError(t, err)

	defer os.RemoveAll(tmpPath)

	baseRepoFilePath := filepath.Join(tmpPath, "apex-bridge-smartcontracts")
	bridgeSolFilePath := filepath.Join(baseRepoFilePath, "contracts", "Bridge.sol")

	getVersion := func(t *testing.T) string {
		t.Helper()

		fn := contractsapi.ApexBridgeContracts.Bridge.Abi.GetMethod("version")

		fncall, err := fn.Encode([]any{})
		require.NoError(t, err)

		response, err := txRelayer.Call(types.ZeroAddress, contracts.Bridge, fncall)
		require.NoError(t, err)

		byteResponse, err := hex.DecodeString(strings.TrimPrefix(response, "0x"))
		require.NoError(t, err)

		decoded, err := fn.Outputs.Decode(byteResponse)
		require.NoError(t, err)

		mp, _ := decoded.(map[string]any)

		return mp["0"].(string)
	}

	var (
		stdOutBuffer   bytes.Buffer
		desiredVersion = "190843934374.0323.2371283182"
		oldVersion     = getVersion(t)
	)

	require.NoError(t, cardanofw.RunCommand("git", []string{"submodule"}, &stdOutBuffer))
	fmt.Printf("git submodule output:\n%s\n", stdOutBuffer.String())

	re := regexp.MustCompile(`(?m)[- ]?([a-f0-9]{40})\s+(?:\./|\.\./)*` + regexp.QuoteMeta("apex-bridge-smartcontracts") + `(?:\s+\(.*\))?`)

	match := re.FindStringSubmatch(stdOutBuffer.String())
	require.Greater(t, len(match), 1)

	branchName := match[1]
	require.Greater(t, len(branchName), 0)

	fmt.Printf("apex-bridge-smartcontracts branchName: %s\n", branchName)

	// first upgrade just to clone repository
	require.NoError(t, cardanofw.RunCommand(cardanofw.ResolveApexBridgeBinary(), []string{
		"deploy-evm", "upgrade",
		"--url", apex.GetBridgeDefaultJSONRPCAddr(),
		"--key", hex.EncodeToString(privateKeyRaw),
		"--dir", tmpPath,
		"--clone",
		"--branch", branchName,
		"--repo", "https://github.com/Ethernal-Tech/apex-bridge-smartcontracts",
		"--contract", "Bridge:" + contracts.Bridge.String(),
	}, os.Stdout))

	content, err := os.ReadFile(bridgeSolFilePath)
	require.NoError(t, err)

	// Regular expression to match the version function and its return string
	// This pattern matches the function declaration and captures the string to replace
	pattern := `(function version\(\) public pure returns \(string memory\)\s*\{\s*return ")([^"]+)(";)`
	re = regexp.MustCompile(pattern)

	// Replace the string
	replacePattern := fmt.Sprintf("${1}%s${3}", desiredVersion)
	newContent := re.ReplaceAll(content, []byte(replacePattern))

	require.NoError(t, os.WriteFile(bridgeSolFilePath, newContent, 0660))

	require.NoError(t, os.Chdir(baseRepoFilePath))
	// must compile hardhat script(s) again
	require.NoError(t, cardanofw.RunCommand("npx", []string{"hardhat", "compile"}, os.Stdout))
	require.NoError(t, os.Chdir(currentWorkingDir))

	// second upgrade upgrades changed contract
	require.NoError(t, cardanofw.RunCommand(cardanofw.ResolveApexBridgeBinary(), []string{
		"deploy-evm", "upgrade",
		"--url", apex.GetBridgeDefaultJSONRPCAddr(),
		"--key", hex.EncodeToString(privateKeyRaw),
		"--dir", tmpPath,
		"--contract", "Bridge:" + contracts.Bridge.String(),
	}, os.Stdout))

	newVersion := getVersion(t)

	require.NotEqual(t, oldVersion, newVersion)
	require.Equal(t, desiredVersion, newVersion)

	// send bridging tx should work after upgrading
	e2ehelper.ExecuteSingleBridging(
		t, ctx, apex, apex.Users[0], apex.Users[0], cardanofw.ChainIDPrime, cardanofw.ChainIDVector,
		cardanofw.ApexToWei(big.NewInt(1)), cardanofw.AP3XTokenID, false)
}

func TestE2E_ApexBridge_CardanoOracleState(t *testing.T) {
	if cardanofw.ShouldSkipE2RRedundantTests() {
		t.Skip()
	}

	const apiKey = "my_api_key"

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	apex := cardanofw.SetupAndRunReactorBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithAPIValidatorID(-1),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	apiURLs, err := apex.GetBridgingAPIs()
	require.NoError(t, err)

	require.Equal(t, apex.GetValidatorsCount(), len(apiURLs))

	ticker := time.NewTicker(time.Second * 4)
	defer ticker.Stop()

	goodOraclesCount := 0

	for goodOraclesCount < apex.GetValidatorsCount() {
		select {
		case <-ctx.Done():
			t.Fatal("timeout")
		case <-ticker.C:
		}

		goodOraclesCount = 0

	outerLoop:
		for _, apiURL := range apiURLs {
			for _, chainID := range []string{cardanofw.ChainIDVector, cardanofw.ChainIDPrime} {
				requestURL := fmt.Sprintf("%s/api/OracleState/Get?chainId=%s", apiURL, chainID)

				currentState, err := cardanofw.GetOracleState(ctx, requestURL, apiKey)
				if err != nil || currentState == nil {
					break outerLoop
				}

				multisigAddr, feeAddr := "", ""
				sumMultisig, sumFee, desiredAmount := uint64(0), uint64(0), uint64(0)

				switch chainID {
				case cardanofw.ChainIDPrime:
					multisigAddr, feeAddr = apex.PrimeInfo.MultisigAddr[0], apex.PrimeInfo.FeeAddr
					desiredAmount = apex.Config.PrimeConfig.FundAmount
				case cardanofw.ChainIDVector:
					multisigAddr, feeAddr = apex.VectorInfo.MultisigAddr[0], apex.VectorInfo.FeeAddr
					desiredAmount = apex.Config.VectorConfig.FundAmount
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

				if sumMultisig != desiredAmount || sumFee != desiredAmount || currentState.BlockSlot == 0 {
					break outerLoop
				} else {
					goodOraclesCount++
				}
			}
		}
	}
}

func TestE2E_ApexBridge_SingleBridgingWithMultisig(t *testing.T) {
	const privateKeysCount = 5

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	srcChain, dstChain := cardanofw.ChainIDPrime, cardanofw.ChainIDVector
	sendAmount := cardanofw.ApexToWei(big.NewInt(1))
	primeConfig, vectorConfig := cardanofw.NewPrimeChainConfig(), cardanofw.NewVectorChainConfig()
	primeConfig.PremineAmount = 500_000_000
	vectorConfig.PremineAmount = 500_000_000

	apex := cardanofw.SetupAndRunReactorBridge(
		t, ctx,
		cardanofw.WithUserCnt(1),
		cardanofw.WithPrimeConfig(primeConfig),
		cardanofw.WithVectorConfig(vectorConfig),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	wallets := make([]*infrawallet.Wallet, privateKeysCount)
	keyHashes := make([]string, privateKeysCount)

	for i := range privateKeysCount {
		var err error

		wallets[i], err = infrawallet.GenerateWallet(true)
		require.NoError(t, err)

		keyHashes[i], err = infrawallet.GetKeyHash(wallets[i].VerificationKey)
		require.NoError(t, err)
	}

	quorumCount := (len(keyHashes)*2)/3 + 1
	policyScript := infrawallet.NewPolicyScript(keyHashes, quorumCount)

	multisigAddr, err := infrawallet.NewCliUtils(cardanofw.ResolveCardanoCliBinary(cardanofw.ChainIDCardano)).
		GetPolicyScriptEnterpriseAddress(primeConfig.NetworkMagic, policyScript)
	require.NoError(t, err)

	// fund multsig addr
	txHashFund, err := apex.SubmitTx(ctx, srcChain, apex.Users[0], multisigAddr, cardanofw.ApexToWei(big.NewInt(10)), nil, nil, nil)
	require.NoError(t, err)

	fmt.Printf("multsig addr %s funded: %s\n", multisigAddr, txHashFund)

	policyScriptBytes, err := policyScript.GetBytesJSON()
	require.NoError(t, err)

	var senderUserBuilder strings.Builder

	senderUserBuilder.WriteString("ps")
	senderUserBuilder.WriteString(hex.EncodeToString(policyScriptBytes))

	for _, w := range wallets {
		senderUserBuilder.WriteRune('_')
		senderUserBuilder.WriteString(hex.EncodeToString(w.SigningKey))
	}

	balance, err := apex.GetBalance(ctx, apex.Users[0], dstChain)
	require.NoError(t, err)

	prevAmount := cardanofw.SetOrDefault(balance[infrawallet.AdaTokenName], big.NewInt(0))
	expectedAmount := new(big.Int).Add(prevAmount, sendAmount)

	tokensInfo, err := apex.GetBridgingTokensInfo(srcChain, dstChain, cardanofw.AP3XTokenID)
	require.NoError(t, err)

	receiversMap := make(map[string]cardanofw.ReceiverAmount, 1)
	receiversMap[apex.Users[0].VectorAddress.String()] = cardanofw.ReceiverAmount{
		TokenID: tokensInfo.SrcTokenID,
		Amount:  sendAmount,
	}

	txHash, err := apex.GetChainMust(t, srcChain).BridgingRequest(
		cardanofw.BridgingRequestParams{
			Ctx:            ctx,
			DestChainID:    dstChain,
			PrivateKey:     senderUserBuilder.String(),
			ChainIDsConfig: apex.GetChainIDsConfig(),
			Receivers:      receiversMap,
			FeeAmount:      apex.GetMinBridgingFee(cardanofw.ChainIDPrime, false),
			OperationFee:   big.NewInt(0),
			IsCurrencySrc:  true,
			IsCurrencyDest: true,
		},
	)
	require.NoError(t, err)

	fmt.Printf("Tx sent. hash: %s\n", txHash)

	err = apex.WaitForExactAmount(
		ctx, apex.Users[0], dstChain, expectedAmount, 48, time.Second*10, tokensInfo.DstTokenName)
	require.NoError(t, err)
}

func TestE2E_ApexBridge_BatchRecreated(t *testing.T) {
	if cardanofw.ShouldSkipE2RRedundantTests() {
		t.Skip()
	}

	const (
		apiKey = "test_api_key"
	)

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	primeConfig, vectorConfig := cardanofw.NewPrimeChainConfig(), cardanofw.NewVectorChainConfig()
	primeConfig.FundAmount = 500_000_000
	vectorConfig.FundAmount = 500_000_000
	primeConfig.TTLInc, primeConfig.SlotRoundingThreshold = 1, 20
	vectorConfig.TTLInc, vectorConfig.SlotRoundingThreshold = 1, 30

	apex := cardanofw.SetupAndRunReactorBridge(
		t, ctx,
		cardanofw.WithPrimeConfig(primeConfig),
		cardanofw.WithVectorConfig(vectorConfig),
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithUserCnt(1),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	user := apex.Users[0]

	sendAmount := cardanofw.ApexToWei(big.NewInt(1))

	// Initiate bridging PRIME -> VECTOR
	txHash, err := apex.SubmitBridgingRequest(
		cardanofw.SubmitBridgingRequestData{
			Context:          ctx,
			SourceChain:      cardanofw.ChainIDPrime,
			DestinationChain: cardanofw.ChainIDVector,
			Sender:           user,
			WeiAmount:        sendAmount,
			SrcTokenID:       cardanofw.AP3XTokenID,
			Receivers:        []*cardanofw.TestApexUser{user},
		},
	)
	require.NoError(t, err)

	_, timeout := cardanofw.WaitForBatchState(
		ctx, apex, cardanofw.ChainIDPrime, txHash, apiKey, false, true,
		cardanofw.BatchStateIncludedInBatch, cardanofw.BatchStateSubmittedToDestination)

	require.False(t, timeout)
}

func TestE2E_ApexBridge_Over_Max_Allowed_To_Bridge(t *testing.T) {
	if cardanofw.ShouldSkipE2RRedundantTests() {
		t.Skip()
	}

	const (
		apiKey = "test_api_key"
	)

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	apex := cardanofw.SetupAndRunReactorBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithUserCnt(1),
		cardanofw.WithNexusEnabled(true),
		cardanofw.WithCustomConfigHandlers(func(_ *cardanofw.ApexSystem, mp map[string]interface{}) {
			setting := cardanofw.GetMapFromInterfaceKey(mp, "bridgingSettings")
			setting["maxAmountAllowedToBridge"] = cardanofw.ApexToWei(big.NewInt(5))
		}, nil, nil, nil),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	var (
		user             = apex.Users[0]
		sendAmount       = cardanofw.ApexToWei(big.NewInt(10))
		bridgingRequests = []struct {
			src    string
			dest   string
			sender *cardanofw.TestApexUser
		}{
			{src: cardanofw.ChainIDPrime, dest: cardanofw.ChainIDVector, sender: apex.Users[0]},
			{src: cardanofw.ChainIDVector, dest: cardanofw.ChainIDPrime, sender: apex.Users[0]},
			{src: cardanofw.ChainIDNexus, dest: cardanofw.ChainIDPrime, sender: apex.Users[0]},
		}
		txHashes            = make([]string, len(bridgingRequests))
		beforeSendingAmount = make([]map[string]*big.Int, len(bridgingRequests))
	)

	var wg sync.WaitGroup

	for idx, br := range bridgingRequests {
		wg.Add(1)

		go func(i int, src string, dest string, sender *cardanofw.TestApexUser) {
			defer wg.Done()

			var err error

			tokensInfo, err := apex.GetBridgingTokensInfo(br.src, br.dest, cardanofw.AP3XTokenID)
			require.NoError(t, err)

			beforeSendingAmount[i], err = apex.GetBalanceWithTokenName(ctx, user, src, tokensInfo.SrcTokenName)
			require.NoError(t, err)

			txHashes[i], err = apex.SubmitBridgingRequest(cardanofw.SubmitBridgingRequestData{
				Context:          ctx,
				SourceChain:      src,
				DestinationChain: dest,
				Sender:           sender,
				WeiAmount:        sendAmount,
				SrcTokenID:       tokensInfo.SrcTokenID,
				TokensInfo:       tokensInfo,
				Receivers:        []*cardanofw.TestApexUser{user},
			})
			require.NoError(t, err)

			fmt.Printf("Bridging request: %v to %v sent. hash: %s\n", src, dest, txHashes[i])
		}(idx, br.src, br.dest, br.sender)
	}

	wg.Wait()

	for idx, br := range bridgingRequests {
		wg.Add(1)

		go func() {
			defer wg.Done()

			lowerBoundary := new(big.Int).Sub(
				beforeSendingAmount[idx][infrawallet.AdaTokenName],
				new(big.Int).Add(sendAmount, apex.GetMinBridgingFee(br.src, false)))

			fmt.Printf("Tx hash: %s, lowerBoundaryDfm: %d, higherBoundaryDfm: %+v\n", txHashes[idx], lowerBoundary, beforeSendingAmount[idx])

			err := apex.WaitForAmountInRange(ctx, apex.Users[0], br.src, lowerBoundary, beforeSendingAmount[idx][infrawallet.AdaTokenName],
				60, time.Second*30, infrawallet.AdaTokenName)
			require.NoError(t, err)
		}()
	}

	wg.Wait()
}

func TestE2E_FundAmount(t *testing.T) {
	if cardanofw.ShouldSkipE2RRedundantTests() {
		t.Skip()
	}

	const (
		apiKey           = "test_api_key"
		userCnt          = 10
		fundAmountPrime  = 100_000_000
		fundAmountVector = 100_000_000
	)

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	primeConfig, vectorConfig := cardanofw.NewPrimeChainConfig(), cardanofw.NewVectorChainConfig()
	primeConfig.FundAmount = 1_000_000
	vectorConfig.FundAmount = 1_000_000

	apex := cardanofw.SetupAndRunReactorBridge(
		t, ctx,
		cardanofw.WithPrimeConfig(primeConfig),
		cardanofw.WithVectorConfig(vectorConfig),
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithUserCnt(userCnt),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	user := apex.Users[userCnt-1]

	testCases := []struct {
		name       string
		sendAmount *big.Int
		fromChain  cardanofw.ChainID
		toChain    cardanofw.ChainID
		fundAmount int64
	}{
		{
			name:       "From prime to vector - not enough funds",
			sendAmount: cardanofw.ApexToWei(new(big.Int).SetUint64(5)),
			fromChain:  cardanofw.ChainIDPrime,
			toChain:    cardanofw.ChainIDVector,
			fundAmount: fundAmountVector,
		},
		{
			name:       "From vector to prime - not enough funds",
			sendAmount: cardanofw.ApexToWei(new(big.Int).SetUint64(15)),
			fromChain:  cardanofw.ChainIDVector,
			toChain:    cardanofw.ChainIDPrime,
			fundAmount: fundAmountPrime,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			balance, err := apex.GetBalance(ctx, user, tc.toChain)
			require.NoError(t, err)

			tokensInfo, err := apex.GetBridgingTokensInfo(tc.toChain, tc.fromChain, cardanofw.AP3XTokenID)
			require.NoError(t, err)

			prevAmount := balance[infrawallet.AdaTokenName]

			fmt.Printf("prevAmount %v\n", prevAmount)

			expectedAmount := new(big.Int).Set(tc.sendAmount)
			expectedAmount.Add(expectedAmount, prevAmount)

			txHash, err := apex.SubmitBridgingRequest(
				cardanofw.SubmitBridgingRequestData{
					Context:          ctx,
					SourceChain:      tc.fromChain,
					DestinationChain: tc.toChain,
					Sender:           user,
					WeiAmount:        tc.sendAmount,
					SrcTokenID:       tokensInfo.SrcTokenID,
					TokensInfo:       tokensInfo,
					Receivers:        []*cardanofw.TestApexUser{user},
				},
			)
			require.NoError(t, err)

			fmt.Printf("Tx sent. hash: %s. %v - expectedAmount\n", txHash, expectedAmount)

			err = apex.WaitForExactAmount(ctx, user, tc.toChain, expectedAmount, 20, time.Second*10, tokensInfo.DstTokenName)
			require.Error(t, err)

			require.NoError(t, apex.FundChainHotWallet(ctx, tc.toChain, cardanofw.DfmToWei(big.NewInt(tc.fundAmount))))

			txHash, err = apex.SubmitBridgingRequest(
				cardanofw.SubmitBridgingRequestData{
					Context:          ctx,
					SourceChain:      tc.fromChain,
					DestinationChain: tc.toChain,
					Sender:           user,
					WeiAmount:        tc.sendAmount,
					SrcTokenID:       tokensInfo.SrcTokenID,
					TokensInfo:       tokensInfo,
					Receivers:        []*cardanofw.TestApexUser{user},
				},
			)
			require.NoError(t, err)

			fmt.Printf("Tx sent. hash: %s. %v - expectedAmount\n", txHash, expectedAmount)

			err = apex.WaitForExactAmount(ctx, user, tc.toChain, expectedAmount, 20, time.Second*10, tokensInfo.DstTokenName)
			require.NoError(t, err)
		})
	}
}

func TestE2E_ApexBridge_InvalidScenarios(t *testing.T) {
	const (
		apiKey  = "test_api_key"
		userCnt = 15
	)

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	primeConfig, vectorConfig := cardanofw.NewPrimeChainConfig(), cardanofw.NewVectorChainConfig()
	primeConfig.PremineAmount = 500_000_000
	vectorConfig.PremineAmount = 500_000_000

	apex := cardanofw.SetupAndRunReactorBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithUserCnt(userCnt),
		cardanofw.WithPrimeConfig(primeConfig),
		cardanofw.WithVectorConfig(vectorConfig),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	user := apex.Users[0]

	t.Run("Submitted invalid metadata - wrong label", func(t *testing.T) {
		executeInvalidMetadataWrongLabel(t, ctx, apex, user)
	})

	t.Run("Submitted invalid metadata - sliced off", func(t *testing.T) {
		PrimeToVectorInvalidMetadataSlicedOff(t, ctx, apex, user)
	})

	t.Run("Submitted not enough funds - invalid send amount", func(t *testing.T) {
		sendAmount := cardanofw.DfmToWei(big.NewInt(800_000))

		operationFee := apex.GetMinOperationFee(cardanofw.ChainIDPrime)
		minBridgingFee := apex.GetMinBridgingFee(cardanofw.ChainIDPrime, false)

		beforeSendingAmountDfm, err := apex.GetBalance(ctx, user, cardanofw.ChainIDPrime)
		require.NoError(t, err)

		fmt.Println("beforeSendingAmountDfm", beforeSendingAmountDfm)

		tokensInfo, err := apex.GetBridgingTokensInfo(
			cardanofw.ChainIDPrime, cardanofw.ChainIDVector, cardanofw.AP3XTokenID)
		require.NoError(t, err)

		metadata, err := apex.GetChainMust(t, cardanofw.ChainIDPrime).CreateMetadata(
			user.GetAddress(cardanofw.ChainIDPrime), cardanofw.ChainIDVector,
			[]sendtx.BridgingTxReceiver{
				{
					Addr:    user.GetAddress(cardanofw.ChainIDVector),
					Amount:  cardanofw.WeiToDfm(new(big.Int).Sub(sendAmount, minBridgingFee)).Uint64(),
					TokenID: tokensInfo.SrcTokenID,
				},
			}, minBridgingFee,
			operationFee)
		require.NoError(t, err)

		txHash, err := apex.SubmitTx(ctx, cardanofw.ChainIDPrime, user, apex.PrimeInfo.MultisigAddr[0],
			sendAmount, nil, metadata, nil)
		require.NoError(t, err)

		fmt.Printf("Tx sent. hash: %s\n", txHash)
		cardanofw.WaitForInvalidState(t, ctx, apex, cardanofw.ChainIDPrime, txHash, apex.Config.APIKey, cardanofw.DefaultRequestStateTimeoutSec)
	})
}

func TestE2E_ApexBridge_InvalidScenarios_RefundDisabled(t *testing.T) {
	const (
		apiKey  = "test_api_key"
		userCnt = 15

		requestStateTimeoutSec = 600
		retryDelaySec          = 5
	)

	ctx, cncl := context.WithCancel(context.Background())

	defer cncl()

	primeConfig, vectorConfig := cardanofw.NewPrimeChainConfig(), cardanofw.NewVectorChainConfig()
	primeConfig.PremineAmount = 500_000_000
	vectorConfig.PremineAmount = 500_000_000
	primeConfig.UseIndexer = true
	vectorConfig.UseIndexer = true

	apex := cardanofw.SetupAndRunReactorBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithUserCnt(userCnt),
		cardanofw.WithPrimeConfig(primeConfig),
		cardanofw.WithVectorConfig(vectorConfig),
		cardanofw.WithCustomConfigHandlers(func(_ *cardanofw.ApexSystem, mp map[string]interface{}) {
			mp["refundEnabled"] = false
		}, nil, nil, nil),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	user := apex.Users[0]

	primeTestConfig := newTestConfig(t, apex, apex.Config.PrimeConfig, &apex.PrimeInfo, cardanofw.ChainIDVector, cardanofw.AP3XTokenID)

	t.Run("1 .Mismatch submitted and receiver amounts", func(t *testing.T) {
		executeInvalidMismatchSendLovelaceAmount(t, ctx, apex, primeTestConfig, user, requestStateTimeoutSec, retryDelaySec, false, 0)
	})

	t.Run("2. Multiple submitters mismatch submitted and receiver amounts", func(t *testing.T) {
		executeInvalidMismatchSendAmountMultipleInstances(t, ctx, apex, primeTestConfig, requestStateTimeoutSec, retryDelaySec, false, 0)
	})

	t.Run("3. Multiple submitters mismatch submitted and receiver amounts parallel", func(t *testing.T) {
		executeInvalidMismatchSendAmountMultipleInstancesParalel(t, ctx, apex, primeTestConfig, requestStateTimeoutSec, retryDelaySec, false, 0)
	})

	t.Run("4. Submitted invalid metadata - wrong type", func(t *testing.T) {
		executeInvalidMetadataType(
			t, ctx, apex, primeTestConfig, user, requestStateTimeoutSec, retryDelaySec, false, 0)
	})

	t.Run("5. Submitted invalid metadata - invalid destination", func(t *testing.T) {
		executeInvalidDestination(
			t, ctx, apex, primeTestConfig, user, requestStateTimeoutSec, retryDelaySec, false, 0)
	})

	t.Run("6. Submitted invalid metadata - invalid sender", func(t *testing.T) {
		executeInvalidMetadataInvalidSender(
			t, ctx, apex, primeTestConfig, user, requestStateTimeoutSec, 0)
	})

	t.Run("7. Submitted invalid metadata - empty receivers", func(t *testing.T) {
		executeInvalidEmptyReceivers(
			t, ctx, apex, primeTestConfig, user, requestStateTimeoutSec, retryDelaySec, false, 0)
	})

	t.Run("8. Submitted with tokens to bridging addr", func(t *testing.T) {
		sendAmount := cardanofw.ApexToWei(big.NewInt(5))

		operationFee := apex.GetMinOperationFee(cardanofw.ChainIDPrime)
		minBridgingFee := apex.GetMinBridgingFee(cardanofw.ChainIDPrime, true)

		minterUser := apex.Users[userCnt-1]

		brSubmitterUser, err := cardanofw.NewTestApexUser(cardanofw.NewApexNetworkTypesFromSystem(apex))
		require.NoError(t, err)

		minterWallet, _ := minterUser.GetCardanoWallet(cardanofw.ChainIDPrime)

		tokensFunded, err := cardanofw.FundUserWithToken(
			ctx, apex, cardanofw.ChainIDPrime,
			minterWallet, brSubmitterUser,
			cardanofw.DefaultTokenName, cardanofw.DefaultTokenMintAmount,
			cardanofw.ApexToWei(big.NewInt(10)), cardanofw.ApexToWei(big.NewInt(1)))
		require.NoError(t, err)

		tokensInfo, err := apex.GetBridgingTokensInfo(
			cardanofw.ChainIDPrime, cardanofw.ChainIDVector, cardanofw.AP3XTokenID)
		require.NoError(t, err)

		metadata, err := apex.GetChainMust(t, cardanofw.ChainIDPrime).CreateMetadata(
			user.GetAddress(cardanofw.ChainIDPrime), cardanofw.ChainIDVector,
			[]sendtx.BridgingTxReceiver{
				{
					Addr:    user.GetAddress(cardanofw.ChainIDVector),
					Amount:  cardanofw.WeiToDfm(new(big.Int).Sub(sendAmount, minBridgingFee)).Uint64(),
					TokenID: tokensInfo.SrcTokenID,
				},
			}, minBridgingFee, operationFee)
		require.NoError(t, err)

		txHash, err := apex.SubmitTx(ctx, cardanofw.ChainIDPrime, brSubmitterUser, apex.PrimeInfo.MultisigAddr[0],
			sendAmount, []cardanofw.GenericTokenAmount{*tokensFunded}, metadata, nil)
		require.NoError(t, err)

		cardanofw.WaitForInvalidState(t, ctx, apex, cardanofw.ChainIDPrime, txHash, apiKey, 0)
	})
}

func TestE2E_ApexBridge_ValidScenarios(t *testing.T) {
	const (
		apiKey  = "test_api_key"
		userCnt = 40
	)

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	primerConfig, vectorConfig := cardanofw.NewPrimeChainConfig(), cardanofw.NewVectorChainConfig()
	primerConfig.UseIndexer = true
	vectorConfig.UseIndexer = true

	apex := cardanofw.SetupAndRunReactorBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithUserCnt(userCnt),
		cardanofw.WithPrimeConfig(primerConfig),
		cardanofw.WithVectorConfig(vectorConfig),
		cardanofw.WithCustomConfigHandlers(func(_ *cardanofw.ApexSystem, mp map[string]interface{}) {
			mp["refundEnabled"] = false
		}, nil, nil, nil),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	user := apex.Users[userCnt-1]

	fmt.Println("prime user addr: ", user.PrimeAddress)
	fmt.Println("vector user addr: ", user.VectorAddress)
	fmt.Println("prime multisig addr: ", apex.PrimeInfo.MultisigAddr)
	fmt.Println("prime fee addr: ", apex.PrimeInfo.FeeAddr)
	fmt.Printf("prime socket path: %s\n", apex.PrimeInfo.SocketPath)
	fmt.Println("vector multisig addr: ", apex.VectorInfo.MultisigAddr)
	fmt.Println("vector fee addr: ", apex.VectorInfo.FeeAddr)
	fmt.Printf("vector socket path: %s\n", apex.VectorInfo.SocketPath)

	t.Run("Submitter has tokens", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() {
			t.Skip()
		}

		t.Cleanup(func() {
			apex.ResetIndexers()
		})

		sendAmount := cardanofw.ApexToWei(big.NewInt(5))

		minterUser := apex.Users[userCnt-2]

		brSubmitterUser, err := cardanofw.NewTestApexUser(cardanofw.NewApexNetworkTypesFromSystem(apex))
		require.NoError(t, err)

		minterWallet, _ := minterUser.GetCardanoWallet(cardanofw.ChainIDPrime)

		_, err = cardanofw.FundUserWithToken(
			ctx, apex, cardanofw.ChainIDPrime,
			minterWallet, brSubmitterUser,
			cardanofw.DefaultTokenName, cardanofw.DefaultTokenMintAmount,
			cardanofw.ApexToWei(big.NewInt(20)), cardanofw.ApexToWei(big.NewInt(1)))
		require.NoError(t, err)

		e2ehelper.ExecuteSingleBridging(
			t, ctx, apex, brSubmitterUser, user, cardanofw.ChainIDPrime, cardanofw.ChainIDVector, sendAmount,
			cardanofw.AP3XTokenID, false)
	})

	t.Run("Submitted with tokens to bridging addr - confirming that batcher functions", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() {
			t.Skip()
		}

		t.Cleanup(func() {
			apex.ResetIndexers()
		})

		sendAmount := cardanofw.ApexToWei(big.NewInt(5))

		operationFee := apex.GetMinOperationFee(cardanofw.ChainIDPrime)
		feeAmount := apex.GetMinBridgingFee(cardanofw.ChainIDPrime, true)

		minterUser := apex.Users[userCnt-3]

		brSubmitterUser, err := cardanofw.NewTestApexUser(cardanofw.NewApexNetworkTypesFromSystem(apex))
		require.NoError(t, err)

		minterWallet, _ := minterUser.GetCardanoWallet(cardanofw.ChainIDPrime)

		tokensFunded, err := cardanofw.FundUserWithToken(
			ctx, apex, cardanofw.ChainIDPrime,
			minterWallet, brSubmitterUser,
			cardanofw.DefaultTokenName, cardanofw.DefaultTokenMintAmount,
			cardanofw.ApexToWei(big.NewInt(10)), cardanofw.ApexToWei(big.NewInt(1)))
		require.NoError(t, err)

		tokensInfo, err := apex.GetBridgingTokensInfo(
			cardanofw.ChainIDPrime, cardanofw.ChainIDVector, cardanofw.AP3XTokenID)
		require.NoError(t, err)

		metadata, err := apex.GetChainMust(t, cardanofw.ChainIDPrime).CreateMetadata(
			user.GetAddress(cardanofw.ChainIDPrime), cardanofw.ChainIDVector,
			[]sendtx.BridgingTxReceiver{
				{
					Addr:    user.GetAddress(cardanofw.ChainIDVector),
					Amount:  cardanofw.WeiToDfm(new(big.Int).Sub(sendAmount, feeAmount)).Uint64(),
					TokenID: tokensInfo.SrcTokenID,
				},
			}, feeAmount, operationFee)
		require.NoError(t, err)

		txHash, err := apex.SubmitTx(ctx, cardanofw.ChainIDPrime, brSubmitterUser, apex.PrimeInfo.MultisigAddr[0],
			sendAmount, []cardanofw.GenericTokenAmount{*tokensFunded}, metadata, nil)
		require.NoError(t, err)

		cardanofw.WaitForInvalidState(t, ctx, apex, cardanofw.ChainIDPrime, txHash, apiKey, 0)

		const (
			instances = 10
		)

		sendAmountVec := cardanofw.ApexToWei(big.NewInt(1))

		e2ehelper.ExecuteBridgingWaitAfterSubmits(
			t, ctx, apex, instances, minterUser,
			cardanofw.ChainIDVector, cardanofw.ChainIDPrime, sendAmountVec, cardanofw.AP3XTokenID)
	})

	t.Run("From prime to vector wait for each submit", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() {
			t.Skip()
		}

		t.Cleanup(func() {
			apex.ResetIndexers()
		})

		const (
			instances = 5
		)

		sendAmount := cardanofw.ApexToWei(big.NewInt(1))

		e2ehelper.ExecuteBridgingOneByOneWaitOnOtherSide(
			t, ctx, apex, instances, user, cardanofw.ChainIDPrime, cardanofw.ChainIDVector, sendAmount,
			cardanofw.AP3XTokenID)
	})

	t.Run("From prime to vector one by one", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() {
			t.Skip()
		}

		t.Cleanup(func() {
			apex.ResetIndexers()
		})

		const (
			instances = 5
		)

		sendAmount := cardanofw.DfmToWei(big.NewInt(1_000_005))

		e2ehelper.ExecuteBridgingWaitAfterSubmits(
			t, ctx, apex, instances, user, cardanofw.ChainIDPrime, cardanofw.ChainIDVector, sendAmount,
			cardanofw.AP3XTokenID)
	})

	t.Run("From prime to vector parallel", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() {
			t.Skip()
		}

		t.Cleanup(func() {
			apex.ResetIndexers()
		})

		const (
			instances = 5
		)

		sendAmount := cardanofw.ApexToWei(big.NewInt(1))

		e2ehelper.ExecuteBridging(
			t, ctx, apex, 1, apex.Users[:instances], []*cardanofw.TestApexUser{user},
			[]string{cardanofw.ChainIDPrime},
			map[string][]string{
				cardanofw.ChainIDPrime: {cardanofw.ChainIDVector},
			},
			map[e2ehelper.SrcDstChainPair]uint16{
				e2ehelper.NewChainPair(cardanofw.ChainIDPrime, cardanofw.ChainIDVector): cardanofw.AP3XTokenID,
			},
			sendAmount)
	})

	t.Run("From vector to prime one by one", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() {
			t.Skip()
		}

		t.Cleanup(func() {
			apex.ResetIndexers()
		})

		const (
			instances = 5
		)

		sendAmount := cardanofw.ApexToWei(big.NewInt(1))

		e2ehelper.ExecuteBridgingWaitAfterSubmits(
			t, ctx, apex, instances, user, cardanofw.ChainIDVector, cardanofw.ChainIDPrime, sendAmount,
			cardanofw.AP3XTokenID)
	})

	t.Run("From vector to prime parallel", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() {
			t.Skip()
		}

		t.Cleanup(func() {
			apex.ResetIndexers()
		})

		const (
			instances = 5
		)

		sendAmount := cardanofw.ApexToWei(big.NewInt(1))

		e2ehelper.ExecuteBridging(
			t, ctx, apex, 1, apex.Users[:instances], []*cardanofw.TestApexUser{user},
			[]string{cardanofw.ChainIDVector},
			map[string][]string{
				cardanofw.ChainIDVector: {cardanofw.ChainIDPrime},
			},
			map[e2ehelper.SrcDstChainPair]uint16{
				e2ehelper.NewChainPair(cardanofw.ChainIDVector, cardanofw.ChainIDPrime): cardanofw.AP3XTokenID,
			},
			sendAmount)
	})

	t.Run("From prime to vector sequential and parallel", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() {
			t.Skip()
		}

		t.Cleanup(func() {
			apex.ResetIndexers()
		})

		const (
			sequentialInstances = 5
			parallelInstances   = 10
		)

		PrimeToVectorSequentialAndParallelWithMaxReceivers(t, ctx, apex, sequentialInstances, parallelInstances)
	})

	t.Run("From prime to vector sequential and parallel with max receivers", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() {
			t.Skip()
		}

		t.Cleanup(func() {
			apex.ResetIndexers()
		})

		const (
			sequentialInstances = 5
			parallelInstances   = 10
		)

		PrimeToVectorSequentialAndParallelWithMaxReceivers(t, ctx, apex, sequentialInstances, parallelInstances)
	})

	t.Run("Both directions sequential", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() {
			t.Skip()
		}

		t.Cleanup(func() {
			apex.ResetIndexers()
		})

		const (
			instances = 5
		)

		var (
			sendAmount = cardanofw.ApexToWei(big.NewInt(1))
		)

		e2ehelper.ExecuteBridging(
			t, ctx, apex, instances,
			[]*cardanofw.TestApexUser{apex.Users[0]},
			[]*cardanofw.TestApexUser{user},
			[]string{cardanofw.ChainIDPrime, cardanofw.ChainIDVector},
			map[string][]string{
				cardanofw.ChainIDPrime:  {cardanofw.ChainIDVector},
				cardanofw.ChainIDVector: {cardanofw.ChainIDPrime},
			},
			map[e2ehelper.SrcDstChainPair]uint16{
				e2ehelper.NewChainPair(cardanofw.ChainIDPrime, cardanofw.ChainIDVector): cardanofw.AP3XTokenID,
				e2ehelper.NewChainPair(cardanofw.ChainIDVector, cardanofw.ChainIDPrime): cardanofw.AP3XTokenID,
			},
			sendAmount)
	})

	t.Run("Both directions sequential and parallel", func(t *testing.T) {
		const (
			sequentialInstances = 5
			parallelInstances   = 6
		)

		t.Cleanup(func() {
			apex.ResetIndexers()
		})

		PrimeVectorBothDirectionsSequentialAndParallel(t, ctx, apex, user, sequentialInstances, parallelInstances)
	})

	t.Run("Both directions sequential and parallel - one node goes offline midway", func(t *testing.T) {
		const (
			sequentialInstances  = 5
			parallelInstances    = 6
			stopAfter            = time.Second * 60
			validatorStoppingIdx = 1
		)

		t.Cleanup(func() {
			apex.ResetIndexers()

			_ = apex.GetValidator(t, validatorStoppingIdx).Stop() // make sure it was stopped
			require.NoError(t, apex.GetValidator(t, validatorStoppingIdx).Start(ctx, false))
		})

		PrimeVectorBothDirectionsSequentialAndParallel(
			t, ctx, apex, user, sequentialInstances, parallelInstances,
			e2ehelper.WithRestartValidatorsConfig([]e2ehelper.RestartValidatorsConfig{
				{WaitTime: stopAfter, StopIndxs: []int{validatorStoppingIdx}},
			}))
	})

	t.Run("Both directions sequential and parallel — two nodes go offline midway, one node recovers", func(t *testing.T) {
		const (
			sequentialInstances   = 5
			parallelInstances     = 10
			stopAfter             = time.Second * 60
			startAgainAfter       = time.Second * 120
			validatorStoppingIdx1 = 1
			validatorStoppingIdx2 = 2
		)

		var (
			sendAmount = cardanofw.ApexToWei(big.NewInt(1))
		)

		t.Cleanup(func() {
			apex.ResetIndexers()

			_ = apex.GetValidator(t, validatorStoppingIdx2).Stop() // make sure it was stopped
			require.NoError(t, apex.GetValidator(t, validatorStoppingIdx2).Start(ctx, false))
		})

		e2ehelper.ExecuteBridging(
			t, ctx, apex, sequentialInstances,
			apex.Users[:parallelInstances],
			[]*cardanofw.TestApexUser{user},
			[]string{cardanofw.ChainIDPrime, cardanofw.ChainIDVector},
			map[string][]string{
				cardanofw.ChainIDPrime:  {cardanofw.ChainIDVector},
				cardanofw.ChainIDVector: {cardanofw.ChainIDPrime},
			},
			map[e2ehelper.SrcDstChainPair]uint16{
				e2ehelper.NewChainPair(cardanofw.ChainIDPrime, cardanofw.ChainIDVector): cardanofw.AP3XTokenID,
				e2ehelper.NewChainPair(cardanofw.ChainIDVector, cardanofw.ChainIDPrime): cardanofw.AP3XTokenID,
			},
			sendAmount,
			e2ehelper.WithWaitForUnexpectedBridges(true),
			e2ehelper.WithRestartValidatorsConfig([]e2ehelper.RestartValidatorsConfig{
				{WaitTime: stopAfter, StopIndxs: []int{validatorStoppingIdx1, validatorStoppingIdx2}},
				{WaitTime: startAgainAfter, StartIndxs: []int{validatorStoppingIdx1}},
			}))
	})

	t.Run("Both directions sequential and parallel - 4 blade nodes goes off in the middle", func(t *testing.T) {
		if cardanofw.ShouldSkipE2RRedundantTests() {
			t.Skip()
		}

		t.Cleanup(func() {
			apex.ResetIndexers()
		})

		const (
			sequentialInstances   = 5
			parallelInstances     = 10
			stopAfter             = time.Second * 120
			restartAfter          = time.Second * 800
			startAgainAfter       = time.Second * 1000
			validatorStoppingIdx1 = 1
			validatorStoppingIdx2 = 2
		)

		sendAmount := cardanofw.ApexToWei(big.NewInt(1))

		e2ehelper.ExecuteBridging(
			t, ctx, apex, sequentialInstances,
			apex.Users[:parallelInstances],
			[]*cardanofw.TestApexUser{user},
			[]string{cardanofw.ChainIDPrime, cardanofw.ChainIDVector},
			map[string][]string{
				cardanofw.ChainIDPrime:  {cardanofw.ChainIDVector},
				cardanofw.ChainIDVector: {cardanofw.ChainIDPrime},
			},
			map[e2ehelper.SrcDstChainPair]uint16{
				e2ehelper.NewChainPair(cardanofw.ChainIDPrime, cardanofw.ChainIDVector): cardanofw.AP3XTokenID,
				e2ehelper.NewChainPair(cardanofw.ChainIDVector, cardanofw.ChainIDPrime): cardanofw.AP3XTokenID,
			},
			sendAmount,
			e2ehelper.WithWaitForUnexpectedBridges(true),
			e2ehelper.WithTimeoutConfig(e2ehelper.NewTimeoutConfig(
				e2ehelper.WithBridgingNumRetries(500),
				e2ehelper.WithBridgingRetryWaitTime(10*time.Second),
			)),
			e2ehelper.WithRestartValidatorsConfig([]e2ehelper.RestartValidatorsConfig{
				{WaitTime: stopAfter, StopIndxs: []int{0, 1}, ExecutableOption: e2ehelper.Blade},
				{WaitTime: restartAfter, StopIndxs: []int{2, 3}, StartIndxs: []int{0, 1, 2, 3}, ExecutableOption: e2ehelper.Blade},
			}))
	})
}

func TestE2E_ApexBridge_Fund_Defund(t *testing.T) {
	if cardanofw.ShouldSkipE2RRedundantTests() {
		t.Skip()
	}

	const (
		apiKey  = "test_api_key"
		userCnt = 10
	)

	var (
		chains = []string{cardanofw.ChainIDPrime, cardanofw.ChainIDVector, cardanofw.ChainIDNexus}
	)

	type bridingRequest struct {
		src         string
		dest        string
		sender      *cardanofw.TestApexUser
		amount      *big.Int
		receiverIdx uint
	}

	createBridgingData := func(ctx context.Context, apex *cardanofw.ApexSystem,
		bridgingRequests []*bridingRequest, receivers map[uint]*cardanofw.TestApexUser,
		defundReceiver *cardanofw.TestApexUser, defundAmount *big.Int) (
		map[chainStageKey]*big.Int, map[chainStageKey]*big.Int,
		map[chainStageKey]*cardanofw.TestApexUser,
		map[chainStageKey]*big.Int, map[chainStageKey]*big.Int,
		map[chainStageKey]*cardanofw.TestApexUser,
	) {
		var (
			chainPrevAmounts     = make(map[chainStageKey]*big.Int)
			chainExpectedAmounts = make(map[chainStageKey]*big.Int)
			chainReceivers       = make(map[chainStageKey]*cardanofw.TestApexUser)

			defundReceiversPrevAmount     = make(map[chainStageKey]*big.Int)
			defundReceiversExpectedAmount = make(map[chainStageKey]*big.Int)
			defundReceivers               = make(map[chainStageKey]*cardanofw.TestApexUser)
		)

		for _, br := range bridgingRequests {
			tokensInfo, err := apex.GetBridgingTokensInfo(br.src, br.dest, cardanofw.AP3XTokenID)
			require.NoError(t, err)

			key := chainStageKey{dstChain: br.dest, dstTokenName: tokensInfo.DstTokenName, receiver: br.receiverIdx}
			if _, exists := chainPrevAmounts[key]; !exists {
				tokensInfo, err := apex.GetBridgingTokensInfo(br.src, br.dest, cardanofw.AP3XTokenID)
				require.NoError(t, err)

				balance, err := apex.GetBalanceWithTokenName(ctx, receivers[br.receiverIdx], br.dest, tokensInfo.DstTokenName)
				require.NoError(t, err)

				prevAmount := balance[infrawallet.AdaTokenName]

				chainPrevAmounts[key] = prevAmount
			}

			if _, exists := chainExpectedAmounts[key]; !exists {
				chainExpectedAmounts[key] = big.NewInt(0)
			}

			chainExpectedAmounts[key].Add(chainExpectedAmounts[key], cardanofw.ApexToWei(br.amount))

			if _, exists := chainReceivers[key]; !exists {
				chainReceivers[key] = receivers[br.receiverIdx]
			}

			if defundAmount != nil && defundReceiver != nil {
				if _, exists := defundReceiversPrevAmount[key]; !exists {
					balance, err := apex.GetBalance(ctx, defundReceiver, br.dest)
					require.NoError(t, err)

					prevAmount := balance[infrawallet.AdaTokenName]

					defundReceiversPrevAmount[key] = prevAmount
				}

				if _, exist := defundReceiversExpectedAmount[key]; !exist {
					defundReceiversExpectedAmount[key] = big.NewInt(0)
				}

				defundReceiversExpectedAmount[key].Add(defundReceiversExpectedAmount[key], cardanofw.ApexToWei(defundAmount))

				if _, exists := defundReceivers[key]; !exists {
					defundReceivers[key] = defundReceiver
				}
			}
		}

		return chainPrevAmounts, chainExpectedAmounts, chainReceivers, defundReceiversPrevAmount, defundReceiversExpectedAmount, defundReceivers
	}

	//nolint:dupl
	bridgeTransactions := func(ctx context.Context, apex *cardanofw.ApexSystem,
		bridgingRequests []*bridingRequest, receivers map[uint]*cardanofw.TestApexUser,
	) {
		var wg sync.WaitGroup

		for _, br := range bridgingRequests {
			wg.Add(1)

			go func(src string, dest string, sender *cardanofw.TestApexUser, receiver *cardanofw.TestApexUser, sendAmount *big.Int) {
				defer wg.Done()

				txHash, err := apex.SubmitBridgingRequest(cardanofw.SubmitBridgingRequestData{
					Context:          ctx,
					SourceChain:      src,
					DestinationChain: dest,
					Sender:           sender,
					WeiAmount:        sendAmount,
					SrcTokenID:       cardanofw.AP3XTokenID,
					Receivers:        []*cardanofw.TestApexUser{receiver},
				})
				require.NoError(t, err)

				fmt.Printf("Bridging request: %v to %v sent. hash: %s\n", src, dest, txHash)
			}(br.src, br.dest, br.sender, receivers[br.receiverIdx], cardanofw.ApexToWei(br.amount))
		}

		wg.Wait()
	}

	t.Run("Fund_Parallel_Send_BRs_Then_Full_Fund", func(t *testing.T) {
		ctx, cncl := context.WithCancel(context.Background())
		defer cncl()

		primeConfig, vectorConfig, nexusConfig := cardanofw.NewPrimeChainConfig(),
			cardanofw.NewVectorChainConfig(), cardanofw.NewNexusChainConfig(true)
		primeConfig.FundAmount = 0
		vectorConfig.FundAmount = 0
		nexusConfig.FundAmount = big.NewInt(0)

		apex := cardanofw.SetupAndRunReactorBridge(
			t, ctx,
			cardanofw.WithAPIKey(apiKey),
			cardanofw.WithUserCnt(userCnt),
			cardanofw.WithPrimeConfig(primeConfig),
			cardanofw.WithVectorConfig(vectorConfig),
			cardanofw.WithNexusConfig(nexusConfig),
		)

		defer require.True(t, apex.ApexBridgeProcessesRunning())

		var (
			bridgingRequests = []*bridingRequest{
				{src: cardanofw.ChainIDPrime, dest: cardanofw.ChainIDVector, sender: apex.Users[0], amount: big.NewInt(1), receiverIdx: 0},
				{src: cardanofw.ChainIDPrime, dest: cardanofw.ChainIDNexus, sender: apex.Users[1], amount: big.NewInt(1), receiverIdx: 0},
				{src: cardanofw.ChainIDVector, dest: cardanofw.ChainIDPrime, sender: apex.Users[0], amount: big.NewInt(1), receiverIdx: 0},
				{src: cardanofw.ChainIDNexus, dest: cardanofw.ChainIDPrime, sender: apex.Users[0], amount: big.NewInt(1), receiverIdx: 0},
			}

			receivers = map[uint]*cardanofw.TestApexUser{
				0: apex.Users[userCnt-1],
			}
		)

		chainPrevAmounts, chainExpectedAmounts, chainReceivers, _, _, _ := createBridgingData(ctx, apex, bridgingRequests, receivers, nil, nil)

		bridgeTransactions(ctx, apex, bridgingRequests, receivers)

		fmt.Printf("Confirming that bridging requests will not be processed\n")

		errsPerChain := waitOnDestination(ctx, apex, chainPrevAmounts, chainExpectedAmounts, chainReceivers, 30, time.Second*10)
		for chainKey, err := range errsPerChain {
			require.Error(t, err)
			fmt.Printf("As intended, %v TXs on %v not yet arrived\n", chainExpectedAmounts[chainKey], chainKey.dstChain)
		}

		fundWallets(t, ctx, apex, chains, big.NewInt(100))

		errsPerChain = waitOnDestination(ctx, apex, chainPrevAmounts, chainExpectedAmounts, chainReceivers, 200, time.Second*10)
		for chainKey, err := range errsPerChain {
			require.NoError(t, err)
			fmt.Printf("%v TXs on %v confirmed\n", chainExpectedAmounts[chainKey], chainKey)
		}
	})

	t.Run("Fund_Parallel_Send_BRs_Then_Fund_Twice", func(t *testing.T) {
		ctx, cncl := context.WithCancel(context.Background())
		defer cncl()

		primeConfig, vectorConfig, nexusConfig := cardanofw.NewPrimeChainConfig(),
			cardanofw.NewVectorChainConfig(), cardanofw.NewNexusChainConfig(true)
		primeConfig.FundAmount = 0
		vectorConfig.FundAmount = 0
		nexusConfig.FundAmount = big.NewInt(0)

		apex := cardanofw.SetupAndRunReactorBridge(
			t, ctx,
			cardanofw.WithAPIKey(apiKey),
			cardanofw.WithUserCnt(userCnt),
			cardanofw.WithPrimeConfig(primeConfig),
			cardanofw.WithVectorConfig(vectorConfig),
			cardanofw.WithNexusConfig(nexusConfig),
		)

		defer require.True(t, apex.ApexBridgeProcessesRunning())

		var (
			bridgingRequests = []*bridingRequest{
				{src: cardanofw.ChainIDPrime, dest: cardanofw.ChainIDVector, sender: apex.Users[0], amount: big.NewInt(1), receiverIdx: 0},
				{src: cardanofw.ChainIDPrime, dest: cardanofw.ChainIDVector, sender: apex.Users[1], amount: big.NewInt(100), receiverIdx: 1},
				{src: cardanofw.ChainIDPrime, dest: cardanofw.ChainIDNexus, sender: apex.Users[2], amount: big.NewInt(1), receiverIdx: 0},
				{src: cardanofw.ChainIDPrime, dest: cardanofw.ChainIDNexus, sender: apex.Users[3], amount: big.NewInt(100), receiverIdx: 1},
				{src: cardanofw.ChainIDVector, dest: cardanofw.ChainIDPrime, sender: apex.Users[0], amount: big.NewInt(1), receiverIdx: 0},
				{src: cardanofw.ChainIDVector, dest: cardanofw.ChainIDPrime, sender: apex.Users[1], amount: big.NewInt(100), receiverIdx: 1},
				{src: cardanofw.ChainIDNexus, dest: cardanofw.ChainIDPrime, sender: apex.Users[0], amount: big.NewInt(1), receiverIdx: 0},
				{src: cardanofw.ChainIDNexus, dest: cardanofw.ChainIDPrime, sender: apex.Users[1], amount: big.NewInt(100), receiverIdx: 1},
			}

			receivers = map[uint]*cardanofw.TestApexUser{
				0: apex.Users[userCnt-1],
				1: apex.Users[userCnt-2],
			}
		)

		chainPrevAmounts, chainExpectedAmounts, chainReceivers, _, _, _ := createBridgingData(ctx, apex, bridgingRequests, receivers, nil, nil)

		bridgeTransactions(ctx, apex, bridgingRequests, receivers)

		fmt.Printf("Confirming that bridging requests will not be processed\n")

		errsPerChain := waitOnDestination(ctx, apex, chainPrevAmounts, chainExpectedAmounts, chainReceivers, 30, time.Second*10)
		for chainKey, err := range errsPerChain {
			require.Error(t, err)
			fmt.Printf("As intended, %v TXs on %v not yet arrived\n", chainExpectedAmounts[chainKey], chainKey.dstChain)
		}

		fundWallets(t, ctx, apex, chains, big.NewInt(10))

		errsPerChain = waitOnDestination(ctx, apex, chainPrevAmounts, chainExpectedAmounts, chainReceivers, 30, time.Second*10)
		for chainKey, err := range errsPerChain {
			if chainKey.receiver == 1 {
				require.Error(t, err)
				fmt.Printf("As intended, %v TXs on %v not yet arrived\n", chainExpectedAmounts[chainKey], chainKey)
			} else {
				require.NoError(t, err)
				fmt.Printf("%v TXs on %v confirmed\n", chainExpectedAmounts[chainKey], chainKey.dstChain)
			}
		}

		fundWallets(t, ctx, apex, chains, big.NewInt(1000))

		errsPerChain = waitOnDestination(ctx, apex, chainPrevAmounts, chainExpectedAmounts, chainReceivers, 200, time.Second*10)
		for chainKey, err := range errsPerChain {
			require.NoError(t, err)
			fmt.Printf("%v TXs on %v confirmed\n", chainExpectedAmounts[chainKey], chainKey.dstChain)
		}
	})

	t.Run("Basic defund test", func(t *testing.T) {
		ctx, cncl := context.WithCancel(context.Background())
		defer cncl()

		initialFundInDfm := cardanofw.ApexToDfm(big.NewInt(100)).Uint64()

		primeConfig, vectorConfig, nexusConfig := cardanofw.NewPrimeChainConfig(),
			cardanofw.NewVectorChainConfig(), cardanofw.NewNexusChainConfig(true)
		primeConfig.FundAmount = initialFundInDfm
		vectorConfig.FundAmount = initialFundInDfm
		nexusConfig.FundAmount = cardanofw.ApexToWei(big.NewInt(100))

		apex := cardanofw.SetupAndRunReactorBridge(
			t, ctx,
			cardanofw.WithAPIKey(apiKey),
			cardanofw.WithUserCnt(userCnt),
			cardanofw.WithPrimeConfig(primeConfig),
			cardanofw.WithVectorConfig(vectorConfig),
			cardanofw.WithNexusConfig(nexusConfig),
		)

		defer require.True(t, apex.ApexBridgeProcessesRunning())

		// give time for oracles to submit hot wallet increment claims for initial fundings
		select {
		case <-ctx.Done():
			return
		case <-time.After(90 * time.Second):
		}

		var (
			defundReceiver          = apex.Users[userCnt-2]
			apexDefundAndFundAmount = big.NewInt(70)
			apexSendAmount          = big.NewInt(50)

			bridgingRequests = []*bridingRequest{
				{src: cardanofw.ChainIDPrime, dest: cardanofw.ChainIDVector, sender: apex.Users[0], amount: apexSendAmount, receiverIdx: 0},
				{src: cardanofw.ChainIDPrime, dest: cardanofw.ChainIDNexus, sender: apex.Users[1], amount: apexSendAmount, receiverIdx: 0},
				{src: cardanofw.ChainIDNexus, dest: cardanofw.ChainIDPrime, sender: apex.Users[0], amount: apexSendAmount, receiverIdx: 0},
			}

			receivers = map[uint]*cardanofw.TestApexUser{
				0: apex.Users[userCnt-1],
			}
		)

		chainPrevAmounts, chainExpectedAmounts, chainReceivers,
			defundReceiversPrevAmount, defundReceiversExpectedAmount, defundReceivers :=
			createBridgingData(ctx, apex, bridgingRequests, receivers, defundReceiver, apexDefundAndFundAmount)

		defundWallets(t, ctx, apex, chains, defundReceiver, apexDefundAndFundAmount,
			defundReceiversPrevAmount, defundReceiversExpectedAmount, defundReceivers)

		bridgeTransactions(ctx, apex, bridgingRequests, receivers)

		fmt.Printf("Confirming that bridging requests will not be processed\n")

		errsPerChain := waitOnDestination(ctx, apex, chainPrevAmounts, chainExpectedAmounts, chainReceivers, 30, time.Second*10)
		for chain, err := range errsPerChain {
			require.Error(t, err)
			fmt.Printf("As intended, %v TXs on %v not yet arrived\n", chainExpectedAmounts[chain], chain)
		}

		fundWallets(t, ctx, apex, chains, apexDefundAndFundAmount)

		errsPerChain = waitOnDestination(ctx, apex, chainPrevAmounts, chainExpectedAmounts, chainReceivers, 200, time.Second*10)
		for chain, err := range errsPerChain {
			require.NoError(t, err)
			fmt.Printf("%v TXs on %v confirmed\n", chainExpectedAmounts[chain], chain)
		}
	})

	t.Run("Defund after bridging request is sent", func(t *testing.T) {
		ctx, cncl := context.WithCancel(context.Background())
		defer cncl()

		initialFundInDfm := cardanofw.ApexToDfm(big.NewInt(100)).Uint64()

		primeConfig, vectorConfig, nexusConfig := cardanofw.NewPrimeChainConfig(),
			cardanofw.NewVectorChainConfig(), cardanofw.NewNexusChainConfig(true)
		primeConfig.FundAmount = initialFundInDfm
		vectorConfig.FundAmount = initialFundInDfm
		nexusConfig.FundAmount = cardanofw.ApexToWei(big.NewInt(100))

		apex := cardanofw.SetupAndRunReactorBridge(
			t, ctx,
			cardanofw.WithAPIKey(apiKey),
			cardanofw.WithUserCnt(userCnt),
			cardanofw.WithPrimeConfig(primeConfig),
			cardanofw.WithVectorConfig(vectorConfig),
			cardanofw.WithNexusConfig(nexusConfig),
			// increase confirmation block count for prime chain to avoid race conditions with defunding in smart contract
			cardanofw.WithCustomConfigHandlers(func(_ *cardanofw.ApexSystem, mp map[string]interface{}) {
				setting := cardanofw.GetMapFromInterfaceKey(mp, "cardanoChains", "prime")
				setting["confirmationBlockCount"] = 20
			}, nil, nil, nil),
		)

		defer require.True(t, apex.ApexBridgeProcessesRunning())

		// give time for oracles to submit hot wallet increment claims for initial fundings
		select {
		case <-ctx.Done():
			return
		case <-time.After(90 * time.Second):
		}

		var (
			defundReceiver          = apex.Users[userCnt-2]
			apexDefundAndFundAmount = big.NewInt(70)
			apexSendAmount          = big.NewInt(50)

			bridgingRequests = []*bridingRequest{
				{src: cardanofw.ChainIDPrime, dest: cardanofw.ChainIDVector, sender: apex.Users[0], amount: apexSendAmount, receiverIdx: 0},
				{src: cardanofw.ChainIDPrime, dest: cardanofw.ChainIDNexus, sender: apex.Users[1], amount: apexSendAmount, receiverIdx: 0},
				{src: cardanofw.ChainIDNexus, dest: cardanofw.ChainIDPrime, sender: apex.Users[0], amount: big.NewInt(150), receiverIdx: 0},
			}

			receivers = map[uint]*cardanofw.TestApexUser{
				0: apex.Users[userCnt-1],
			}
		)

		chainPrevAmounts, chainExpectedAmounts, chainReceivers, _, _, _ :=
			createBridgingData(ctx, apex, bridgingRequests, receivers, defundReceiver, apexDefundAndFundAmount)

		for _, request := range bridgingRequests {
			bridgeTransactions(ctx, apex, []*bridingRequest{request}, receivers)

			require.NoError(t, apex.DefundHotWallet(
				request.dest, defundReceiver.GetAddress(request.dest), cardanofw.ApexToWei(apexDefundAndFundAmount), big.NewInt(0)))
		}

		fmt.Printf("Confirming that bridging requests will not be processed\n")

		errsPerChain := waitOnDestination(ctx, apex, chainPrevAmounts, chainExpectedAmounts, chainReceivers, 30, time.Second*10)
		for chainKey, err := range errsPerChain {
			require.Error(t, err)
			fmt.Printf("As intended, %v TX on %v not yet arrived\n", chainExpectedAmounts[chainKey], chainKey.dstChain)
		}

		fundWallets(t, ctx, apex, chains, apexDefundAndFundAmount)

		errsPerChain = waitOnDestination(ctx, apex, chainPrevAmounts, chainExpectedAmounts, chainReceivers, 200, time.Second*10)
		for chainKey, err := range errsPerChain {
			require.NoError(t, err)
			fmt.Printf("%v TX on %v confirmed\n", chainExpectedAmounts[chainKey], chainKey.dstChain)
		}
	})
}

func TestE2E_ApexBridge_ValidScenarios_BigTests_AllDirections(t *testing.T) {
	if !cardanofw.IsEnvVarTrue("RUN_E2E_BIG_TESTS") {
		t.Skip()
	}

	const (
		apiKey  = "test_api_key"
		userCnt = 2010 // max 1000 parallel instances, userCnot >= 2 * instances + 1
	)

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	primeConfig, vectorConfig, nexusConfig := cardanofw.NewPrimeChainConfig(), cardanofw.NewVectorChainConfig(), cardanofw.NewNexusChainConfig(true)
	primeConfig.PremineAmount = 30_000_000_000
	vectorConfig.PremineAmount = 30_000_000_000
	nexusConfig.PremineAmount = cardanofw.ApexToWei(new(big.Int).SetUint64(30_000_000_000))

	apex := cardanofw.SetupAndRunReactorBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithUserCnt(userCnt),
		cardanofw.WithPrimeConfig(primeConfig),
		cardanofw.WithVectorConfig(vectorConfig),
		cardanofw.WithNexusConfig(nexusConfig),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	user := apex.Users[userCnt-1]

	fmt.Println("prime user addr: ", user.PrimeAddress)
	fmt.Println("vector user addr: ", user.VectorAddress)
	fmt.Println("prime multisig addr: ", apex.PrimeInfo.MultisigAddr)
	fmt.Println("prime fee addr: ", apex.PrimeInfo.FeeAddr)
	fmt.Println("vector multisig addr: ", apex.VectorInfo.MultisigAddr)
	fmt.Println("vector fee addr: ", apex.VectorInfo.FeeAddr)

	t.Run("Both directions 1000x 60min 90%", func(t *testing.T) {
		const (
			instances     = 1000
			maxWaitTime   = 3600
			successChance = 90 // 90%

			// wait for tx timeout
			numRetries = 500
			waitTime   = time.Second * 10
		)

		sendAmount := cardanofw.ApexToWei(new(big.Int).SetInt64(1))

		bridgingRequests := []struct {
			src            cardanofw.ChainID
			dest           cardanofw.ChainID
			firstSenderIdx int
		}{
			{src: cardanofw.ChainIDPrime, dest: cardanofw.ChainIDVector, firstSenderIdx: 0},
			{src: cardanofw.ChainIDPrime, dest: cardanofw.ChainIDNexus, firstSenderIdx: instances},
			{src: cardanofw.ChainIDVector, dest: cardanofw.ChainIDPrime, firstSenderIdx: 0},
			{src: cardanofw.ChainIDNexus, dest: cardanofw.ChainIDPrime, firstSenderIdx: 0},
		}

		seed := rand.Int63n(1_000_000_000)
		r := rand.New(rand.NewSource(seed)) // New seeded random number generator

		fmt.Printf("Test seed: %v\n", seed)

		fmt.Printf("Sending %v transactions in %v seconds\n", instances*len(bridgingRequests), maxWaitTime)

		prevAmounts := make(map[cardanofw.ChainID]*big.Int)
		expectedAmounts := make(map[cardanofw.ChainID]*big.Int)

		var wg sync.WaitGroup

		for j, br := range bridgingRequests {
			succeededCount := int64(0)

			if _, ok := prevAmounts[br.dest]; !ok {
				balance, err := apex.GetBalance(ctx, user, br.dest)
				require.NoError(t, err)

				prevAmounts[br.dest] = balance[infrawallet.AdaTokenName]
				expectedAmounts[br.dest] = new(big.Int).Set(prevAmounts[br.dest])
			}

			for i := 0; i < instances; i++ {
				success := successChance > r.Intn(100)
				if success {
					succeededCount++
				}

				wg.Add(1)

				go func(idx int, brIdx int, src, dest cardanofw.ChainID, valid bool) {
					defer wg.Done()

					if valid {
						time.Sleep(time.Second * time.Duration(r.Intn(maxWaitTime)))

						_, err := apex.SubmitBridgingRequest(cardanofw.SubmitBridgingRequestData{
							Context:          ctx,
							SourceChain:      src,
							DestinationChain: dest,
							Sender:           apex.Users[idx],
							WeiAmount:        sendAmount,
							SrcTokenID:       cardanofw.AP3XTokenID,
							Receivers:        []*cardanofw.TestApexUser{user},
						})
						require.NoError(t, err)
					} else if src != cardanofw.ChainIDNexus {
						submitInvalidSendAmountTransaction(t, ctx, apex, src, dest, apex.Users[idx], sendAmount, user.GetAddress(dest))
					}
				}(br.firstSenderIdx+i, j, br.src, br.dest, success)
			}

			totalSent := new(big.Int).Mul(sendAmount, big.NewInt(succeededCount))
			expectedAmounts[br.dest].Add(expectedAmounts[br.dest], totalSent)
		}

		wg.Wait()

		fmt.Printf("All tx sent, waiting for confirmation.\n")

		for _, destChain := range []cardanofw.ChainID{cardanofw.ChainIDPrime, cardanofw.ChainIDVector, cardanofw.ChainIDNexus} {
			wg.Add(1)

			go func(dest string) {
				defer wg.Done()

				succeededCount := new(big.Int).Sub(expectedAmounts[dest], prevAmounts[dest]).Uint64() / sendAmount.Uint64()

				fmt.Printf("Waiting for %+v TXs on %s, prevAmount: %v, expectedAmount: %v\n",
					succeededCount, dest, prevAmounts[dest], expectedAmounts[dest])

				err := apex.WaitForExactAmount(ctx, user, dest, expectedAmounts[dest], numRetries, waitTime, infrawallet.AdaTokenName)
				require.NoError(t, err)

				fmt.Printf("TXs on %s confirmed\n", dest)
			}(destChain)
		}

		wg.Wait()
	})
}

type chainStageKey struct {
	dstChain     string
	dstTokenName string
	receiver     uint
}

func waitOnDestination(
	ctx context.Context, apex *cardanofw.ApexSystem,
	chainPrevAmounts map[chainStageKey]*big.Int, chainExpectedAmounts map[chainStageKey]*big.Int,
	chainReceivers map[chainStageKey]*cardanofw.TestApexUser, numRetries int, waitTime time.Duration,
) map[chainStageKey]error {
	var (
		wg           sync.WaitGroup
		errsPerChain = make(map[chainStageKey]error, len(chainPrevAmounts))
		mu           sync.Mutex
	)

	for chainKey, prevAmount := range chainPrevAmounts {
		wg.Add(1)

		go func() {
			defer wg.Done()

			fmt.Printf("Waiting for %v Amount on %v\n", chainExpectedAmounts[chainKey], chainKey.dstChain)

			expectedAmount := new(big.Int).Set(chainExpectedAmounts[chainKey])
			expectedAmount.Add(expectedAmount, prevAmount)

			err := apex.WaitForExactAmount(
				ctx, chainReceivers[chainKey], chainKey.dstChain, expectedAmount, numRetries, waitTime, chainKey.dstTokenName)

			mu.Lock()
			defer mu.Unlock()

			errsPerChain[chainKey] = err
		}()
	}

	wg.Wait()

	return errsPerChain
}

func fundWallets(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, chains []string,
	fundAmountApex *big.Int,
) {
	t.Helper()

	fmt.Printf("Funding hot wallets\n")

	for _, chain := range chains {
		require.NoError(t,
			apex.FundChainHotWallet(ctx, chain, cardanofw.ApexToWei(fundAmountApex)),
		)
	}

	fmt.Printf("Hot wallets have been funded\n")
}

func defundWallets(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, chains []string,
	defundReceiver *cardanofw.TestApexUser, defundAmountApex *big.Int,
	defundReceiverPrevAmounts map[chainStageKey]*big.Int, defundReceiverExpectedAmounts map[chainStageKey]*big.Int,
	defundReceivers map[chainStageKey]*cardanofw.TestApexUser,
) {
	t.Helper()

	fmt.Printf("Defunding hot wallets\n")

	defundAmount := cardanofw.ApexToWei(defundAmountApex)

	for _, chain := range chains {
		require.NoError(t,
			apex.DefundHotWallet(chain, defundReceiver.GetAddress(chain), defundAmount, big.NewInt(0)))
	}

	errsPerChain := waitOnDestination(ctx, apex,
		defundReceiverPrevAmounts, defundReceiverExpectedAmounts, defundReceivers,
		300, time.Second*10)
	for chain, err := range errsPerChain {
		require.NoError(t, err)
		fmt.Printf("Defund on %v confirmed\n", chain)
	}
}

func sendWithoutWaitInvalidMetadataWrongType(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, sender, receiver *cardanofw.TestApexUser,
	originChainID, destinationChainID string, sendAmount *big.Int,
) error {
	t.Helper()

	tokensInfo, err := apex.GetBridgingTokensInfo(originChainID, destinationChainID, cardanofw.AP3XTokenID)
	require.NoError(t, err)

	receivers := []sendtx.BridgingTxReceiver{
		{
			Addr:    receiver.GetAddress(destinationChainID),
			Amount:  cardanofw.WeiToDfm(sendAmount).Uint64(),
			TokenID: tokensInfo.SrcTokenID,
		},
	}

	metadata, feeAmount := createMetadata(t, ctx, apex, originChainID, destinationChainID,
		apex.GetMinBridgingFee(originChainID, false),
		big.NewInt(0), sender, receivers, false)
	metadata = bytes.Replace(metadata, []byte("bridge"), []byte("xxxxx"), 1)

	multisigAddress, err := apex.GetChainMust(t, originChainID).GetAddressToBridgeTo(ctx, false)
	if err != nil {
		return err
	}

	_, err = apex.SubmitTx(ctx, originChainID, sender, multisigAddress, new(big.Int).Add(sendAmount, feeAmount), nil,
		metadata, nil)
	if err != nil {
		return err
	}

	return nil
}

func TestE2E_ApexBridge_UTxOConsolidation(t *testing.T) {
	if cardanofw.ShouldSkipE2RRedundantTests() {
		t.Skip()
	}

	const (
		fundUtxoCount   = 63
		maxFeeUtxoCount = 4
		maxUtxoCount    = 50
	)

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	vectorConfig := cardanofw.NewVectorChainConfig()
	minUtxoDfm := cardanofw.WeiToDfm(cardanofw.MinUTxODefaultValue).Uint64()
	vectorConfig.FundUTxOCount = fundUtxoCount
	vectorConfig.FundAmount = minUtxoDfm * fundUtxoCount
	vectorConfig.InitialHotWalletAmount = new(big.Int).SetUint64(vectorConfig.FundAmount)
	sendAmount := cardanofw.DfmToWei(big.NewInt(0).SetUint64(vectorConfig.FundAmount - minUtxoDfm*3))

	// adding indexer because there are many funding transactions
	vectorConfig.UseIndexer = true

	var (
		initialUtxos []map[string]any
		tipData      infrawallet.QueryTipData
		lock         sync.Mutex
	)

	apex := cardanofw.SetupAndRunApexBridge(
		t, ctx, cardanofw.SystemIDReactor,
		cardanofw.WithUserCnt(1),
		cardanofw.WithVectorConfig(vectorConfig),
		cardanofw.WithCustomConfigHandlers(func(a *cardanofw.ApexSystem, mp map[string]any) {
			t.Helper()

			lock.Lock()
			defer lock.Unlock()

			// retrieve only once for all validators
			if len(initialUtxos) == 0 {
				initialUtxos, tipData = getInitialUtxosAndTip(
					t, ctx, a.VectorInfo, a.VectorInfo.MultisigAddr, a.VectorInfo.FeeAddr)
			}

			// Vector indexer should start after multisig funding is done
			vcCfg := cardanofw.GetMapFromInterfaceKey(mp, "cardanoChains", cardanofw.ChainIDVector)
			vcCfg["startBlockHash"] = tipData.Hash
			vcCfg["startSlot"] = tipData.Slot
			vcCfg["initialUtxos"] = initialUtxos
			vcCfg["maxFeeUtxoCount"] = maxFeeUtxoCount
			vcCfg["maxUtxoCount"] = maxUtxoCount
		}, nil, nil, nil),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	txRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithClient(apex.BridgeCluster.Servers[0].JSONRPC()))
	require.NoError(t, err)

	txProviderVector, err := apex.VectorInfo.GetTxProvider()
	require.NoError(t, err)

	getLastConfirmedBatchID := func(chainID string) uint64 {
		input, err := contractsapi.ApexBridgeContracts.SignedBatches.Abi.GetMethod("getConfirmedBatchId").
			Encode([]any{cardanofw.ChainIDToInt(chainID)})
		require.NoError(t, err)

		response, err := txRelayer.Call(types.ZeroAddress, contracts.SignedBatches, input)
		require.NoError(t, err)

		val, err := common.ParseUint64orHex(&response)
		require.NoError(t, err)

		return val
	}

	require.Equal(t, uint64(0), getLastConfirmedBatchID(cardanofw.ChainIDVector))

	utxos, err := infracommon.ExecuteWithRetry(
		ctx, func(ctx context.Context) ([]infrawallet.Utxo, error) {
			return txProviderVector.GetUtxos(ctx, apex.VectorInfo.MultisigAddr[0])
		},
	)
	require.NoError(t, err)

	require.Len(t, utxos, vectorConfig.FundUTxOCount)

	e2ehelper.ExecuteSingleBridging(
		t, ctx, apex, apex.Users[0], apex.Users[0],
		cardanofw.ChainIDPrime, cardanofw.ChainIDVector, sendAmount, cardanofw.AP3XTokenID, false)

	require.Equal(t, uint64(2), getLastConfirmedBatchID(cardanofw.ChainIDVector))

	utxos, err = infracommon.ExecuteWithRetry(
		ctx, func(ctx context.Context) ([]infrawallet.Utxo, error) {
			return txProviderVector.GetUtxos(ctx, apex.VectorInfo.MultisigAddr[0])
		},
	)
	require.NoError(t, err)

	require.Len(t, utxos, 1)
}

func TestE2E_ApexBridge_UTxOConsolidationWithBothDirections(t *testing.T) {
	if cardanofw.ShouldSkipE2RRedundantTests() {
		t.Skip()
	}

	const (
		fundUtxoCount                 = 8
		maxFeeUtxoCount               = 1
		maxUtxoCount                  = 4
		sequentialInstances           = 3
		parallelInstances             = 6
		minimumExpectedConsolidations = 3
	)

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	primeConfig, vectorConfig := cardanofw.NewPrimeChainConfig(), cardanofw.NewVectorChainConfig()
	vectorConfig.FundUTxOCount = fundUtxoCount
	vectorConfig.FundAmount = cardanofw.WeiToDfm(cardanofw.MinUTxODefaultValue).Uint64() * parallelInstances * sequentialInstances
	vectorConfig.InitialHotWalletAmount = new(big.Int).SetUint64(vectorConfig.FundAmount)
	vectorConfig.UseIndexer = true
	primeConfig.FundUTxOCount = fundUtxoCount
	primeConfig.FundAmount = cardanofw.WeiToDfm(cardanofw.MinUTxODefaultValue).Uint64() * parallelInstances * sequentialInstances
	primeConfig.InitialHotWalletAmount = new(big.Int).SetUint64(primeConfig.FundAmount)
	primeConfig.UseIndexer = true
	sendAmount := cardanofw.MinUTxODefaultValue

	var (
		initialUtxosVector, initialUtxosPrime []map[string]any
		tipDataVector, tipDataPrime           infrawallet.QueryTipData
		lock                                  sync.Mutex
	)

	apex := cardanofw.SetupAndRunApexBridge(
		t, ctx, cardanofw.SystemIDReactor,
		cardanofw.WithUserCnt(parallelInstances+1),
		cardanofw.WithVectorConfig(vectorConfig),
		cardanofw.WithPrimeConfig(primeConfig),
		cardanofw.WithCustomConfigHandlers(func(a *cardanofw.ApexSystem, mp map[string]any) {
			t.Helper()

			lock.Lock()
			defer lock.Unlock()

			// retrieve only once for all validators
			if len(initialUtxosVector) == 0 {
				initialUtxosVector, tipDataVector = getInitialUtxosAndTip(
					t, ctx, a.VectorInfo, a.VectorInfo.MultisigAddr, a.VectorInfo.FeeAddr)
				initialUtxosPrime, tipDataPrime = getInitialUtxosAndTip(
					t, ctx, a.PrimeInfo, a.PrimeInfo.MultisigAddr, a.PrimeInfo.FeeAddr)
			}

			// Both chains indexers should start after multisig funding is done
			vcCfg := cardanofw.GetMapFromInterfaceKey(mp, "cardanoChains", cardanofw.ChainIDVector)
			vcCfg["startBlockHash"] = tipDataVector.Hash
			vcCfg["startSlot"] = tipDataVector.Slot
			vcCfg["initialUtxos"] = initialUtxosVector
			vcCfg["maxFeeUtxoCount"] = maxFeeUtxoCount
			vcCfg["maxUtxoCount"] = maxUtxoCount
			vcCfg = cardanofw.GetMapFromInterfaceKey(mp, "cardanoChains", cardanofw.ChainIDPrime)
			vcCfg["startBlockHash"] = tipDataPrime.Hash
			vcCfg["startSlot"] = tipDataPrime.Slot
			vcCfg["initialUtxos"] = initialUtxosPrime
			vcCfg["maxFeeUtxoCount"] = maxFeeUtxoCount
			vcCfg["maxUtxoCount"] = maxUtxoCount
		}, nil, nil, nil),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	getCntConsolidationMap, _ := checkConsolidationBatchCounts(
		t, ctx,
		apex.BridgeCluster.Servers[0].JSONRPC(),
		[]string{cardanofw.ChainIDPrime, cardanofw.ChainIDVector},
		map[string]uint64{cardanofw.ChainIDPrime: 0, cardanofw.ChainIDVector: 0},
	)

	e2ehelper.ExecuteBridging(
		t, ctx, apex, sequentialInstances,
		apex.Users[:parallelInstances],
		[]*cardanofw.TestApexUser{apex.Users[parallelInstances]},
		[]string{cardanofw.ChainIDPrime, cardanofw.ChainIDVector},
		map[string][]string{
			cardanofw.ChainIDPrime:  {cardanofw.ChainIDVector},
			cardanofw.ChainIDVector: {cardanofw.ChainIDPrime},
		},
		map[e2ehelper.SrcDstChainPair]uint16{
			e2ehelper.NewChainPair(cardanofw.ChainIDPrime, cardanofw.ChainIDVector): cardanofw.AP3XTokenID,
			e2ehelper.NewChainPair(cardanofw.ChainIDVector, cardanofw.ChainIDPrime): cardanofw.AP3XTokenID,
		},
		sendAmount,
		e2ehelper.WithWaitForUnexpectedBridges(true),
		e2ehelper.WithTimeoutConfig(e2ehelper.NewTimeoutConfig(
			e2ehelper.WithBridgingNumRetries(200),
			e2ehelper.WithBridgingRetryWaitTime(10*time.Second),
		)))

	for _, cnt := range getCntConsolidationMap() {
		assert.GreaterOrEqual(t, cnt, minimumExpectedConsolidations)
	}
}

func submitInvalidSendAmountTransaction(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, src, dest cardanofw.ChainID, senderUser *cardanofw.TestApexUser, sendAmount *big.Int,
	receiverUserAddr string,
) {
	t.Helper()

	operationFee := apex.GetMinOperationFee(src)
	feeAmount := apex.GetMinBridgingFee(src, false)

	srcTestChain := apex.GetChainMust(t, src)

	tokensInfo, err := apex.GetBridgingTokensInfo(src, dest, cardanofw.AP3XTokenID)
	require.NoError(t, err)

	bridgingRequestMetadata, err := srcTestChain.CreateMetadata(
		senderUser.GetAddress(src), dest, []sendtx.BridgingTxReceiver{
			{
				Addr:    receiverUserAddr,
				Amount:  cardanofw.WeiToDfm(sendAmount).Uint64() * 10,
				TokenID: tokensInfo.SrcTokenID,
			},
		}, feeAmount, operationFee)
	require.NoError(t, err)

	_, err = apex.SubmitTx(ctx, src, senderUser, srcTestChain.GetHotWalletAddresses()[0],
		new(big.Int).Add(sendAmount, feeAmount), nil, bridgingRequestMetadata, nil)
	require.NoError(t, err)
}

func PrimeToVectorInvalidMetadataSlicedOff(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, user *cardanofw.TestApexUser,
) {
	t.Helper()

	sendAmount := cardanofw.ApexToWei(big.NewInt(1))

	operationFee := apex.GetMinOperationFee(cardanofw.ChainIDPrime)
	minBridgingFee := apex.GetMinBridgingFee(cardanofw.ChainIDPrime, false)

	tokensInfo, err := apex.GetBridgingTokensInfo(cardanofw.ChainIDPrime, cardanofw.ChainIDVector, cardanofw.AP3XTokenID)
	require.NoError(t, err)

	receivers := []sendtx.BridgingTxReceiver{
		{
			Addr:    user.GetAddress(cardanofw.ChainIDVector),
			Amount:  cardanofw.WeiToDfm(sendAmount).Uint64(),
			TokenID: tokensInfo.SrcTokenID,
		},
	}

	metadata, err := apex.GetChainMust(t, cardanofw.ChainIDPrime).CreateMetadata(
		user.GetAddress(cardanofw.ChainIDPrime), cardanofw.ChainIDVector,
		receivers, sendAmount, operationFee)
	require.NoError(t, err)

	// Send only half bytes of metadata making it invalid
	metadata = metadata[0 : len(metadata)/2]

	txHash, err := apex.SubmitTx(
		ctx, cardanofw.ChainIDPrime, user,
		apex.PrimeInfo.MultisigAddr[0], new(big.Int).Add(sendAmount, minBridgingFee), nil, metadata, nil)
	require.Error(t, err)

	fmt.Printf("Tx sent. hash: %s\n", txHash)
}

func PrimeToVectorSequentialAndParallelWithMaxReceivers(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem,
	sequentialInstances, parallelInstances int, options ...e2ehelper.ExecuteBridgingOption,
) {
	t.Helper()

	const (
		receivers = 4
	)

	sendAmount := cardanofw.ApexToWei(big.NewInt(1))

	e2ehelper.ExecuteBridging(
		t, ctx, apex, sequentialInstances,
		apex.Users[:parallelInstances],
		apex.Users[:receivers],
		[]string{cardanofw.ChainIDPrime},
		map[string][]string{
			cardanofw.ChainIDPrime: {cardanofw.ChainIDVector},
		},
		map[e2ehelper.SrcDstChainPair]uint16{
			e2ehelper.NewChainPair(cardanofw.ChainIDPrime, cardanofw.ChainIDVector): cardanofw.AP3XTokenID,
		},
		sendAmount,
		options...)
}

func PrimeVectorBothDirectionsSequentialAndParallel(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem,
	receiverUser *cardanofw.TestApexUser, sequentialInstances, parallelInstances int, options ...e2ehelper.ExecuteBridgingOption,
) {
	t.Helper()

	sendAmount := cardanofw.ApexToWei(big.NewInt(1))

	options = append(
		options,
		e2ehelper.WithWaitForUnexpectedBridges(true),
	)

	e2ehelper.ExecuteBridging(
		t, ctx, apex, sequentialInstances,
		apex.Users[:parallelInstances],
		[]*cardanofw.TestApexUser{receiverUser},
		[]string{cardanofw.ChainIDPrime, cardanofw.ChainIDVector},
		map[string][]string{
			cardanofw.ChainIDPrime:  {cardanofw.ChainIDVector},
			cardanofw.ChainIDVector: {cardanofw.ChainIDPrime},
		},
		map[e2ehelper.SrcDstChainPair]uint16{
			e2ehelper.NewChainPair(cardanofw.ChainIDPrime, cardanofw.ChainIDVector): cardanofw.AP3XTokenID,
			e2ehelper.NewChainPair(cardanofw.ChainIDVector, cardanofw.ChainIDPrime): cardanofw.AP3XTokenID,
		},
		sendAmount,
		options...)
}

func getInitialUtxosAndTip(
	t *testing.T, ctx context.Context, chainInfo cardanofw.CardanoChainInfo, multisigAddresses []string, feeAddr string,
) ([]map[string]any, infrawallet.QueryTipData) {
	t.Helper()

	txProvider, err := chainInfo.GetTxProvider()
	require.NoError(t, err)

	addrUtxos := make(map[string][]infrawallet.Utxo, len(multisigAddresses))
	multisigUtxosCnt := 0

	for idx, addr := range multisigAddresses {
		multisigUtxos, err := infracommon.ExecuteWithRetry(
			ctx, func(ctx context.Context) ([]infrawallet.Utxo, error) {
				return txProvider.GetUtxos(ctx, addr)
			},
		)
		require.NoError(t, err)

		fmt.Printf("\nMultisig addr: %s[%d]: %v\n", addr, idx, multisigUtxos)

		addrUtxos[addr] = append(addrUtxos[addr], multisigUtxos...)
		multisigUtxosCnt += len(multisigUtxos)
	}

	feeUtxos, err := infracommon.ExecuteWithRetry(
		ctx, func(ctx context.Context) ([]infrawallet.Utxo, error) {
			return txProvider.GetUtxos(ctx, feeAddr)
		},
	)
	require.NoError(t, err)

	fmt.Printf("\nFee addr: %s: %v\n", feeAddr, feeUtxos)

	tipData, err := infracommon.ExecuteWithRetry(
		ctx, func(ctx context.Context) (infrawallet.QueryTipData, error) {
			return txProvider.GetTip(ctx)
		},
	)
	require.NoError(t, err)

	initialUtxos := make([]map[string]any, 0, multisigUtxosCnt+len(feeUtxos))

	utxoToMap := func(utxo infrawallet.Utxo, addr string) map[string]any {
		bytes, _ := hex.DecodeString(utxo.Hash)

		return map[string]any{
			"id":      [32]byte(bytes),
			"index":   utxo.Index,
			"address": addr,
			"amount":  utxo.Amount,
			"tokens":  utxo.Tokens,
			"slot":    tipData.Slot,
		}
	}

	for _, addr := range multisigAddresses {
		for _, utxo := range addrUtxos[addr] {
			initialUtxos = append(initialUtxos, utxoToMap(utxo, addr))
		}
	}

	for _, utxo := range feeUtxos {
		initialUtxos = append(initialUtxos, utxoToMap(utxo, feeAddr))
	}

	return initialUtxos, tipData
}

func checkConsolidationBatchCounts(
	t *testing.T, ctx context.Context, bridgeJSONRPC *jsonrpc.EthClient, chainIDs []string,
	lastBatchIDs map[string]uint64,
) (func() map[string]int, map[string]uint64) {
	t.Helper()

	const pullTimeBatchInfo = time.Second * 10

	txRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithClient(bridgeJSONRPC))
	require.NoError(t, err)

	var (
		lock                sync.Mutex
		consolidationCntMap = map[string]int{}
	)

	getConfirmedBatchFn := contractsapi.ApexBridgeContracts.SignedBatches.Abi.GetMethod("getConfirmedBatch")

	for _, chainID := range chainIDs {
		go func(chainID string) {
			var lastBatchID = lastBatchIDs[chainID]

			for {
				select {
				case <-time.After(pullTimeBatchInfo):
					input, err := getConfirmedBatchFn.Encode([]any{cardanofw.ChainIDToInt(chainID)})
					require.NoError(t, err)

					response, err := txRelayer.Call(types.ZeroAddress, contracts.SignedBatches, input)
					if err != nil {
						return // we assume this error is because the bridge is stopped already
					}

					bytes, err := hex.DecodeString(strings.TrimPrefix(response, "0x"))
					require.NoError(t, err)

					decoded, err := getConfirmedBatchFn.Outputs.Decode(bytes)
					require.NoError(t, err)

					batchInfo := decoded.(map[string]any)["_batch"].(map[string]any)
					id := batchInfo["id"].(uint64)
					batchType := batchInfo["batchType"].(uint8)

					if lastBatchID <= id {
						lastBatchID = id

						if batchType == uint8(cardanofw.BatchTypeConsolidation) {
							lock.Lock()
							lastBatchIDs[chainID] = id
							consolidationCntMap[chainID]++
							lock.Unlock()
						}
					}
				case <-ctx.Done():
					return
				}
			}
		}(chainID)
	}

	return func() map[string]int {
		lock.Lock()
		defer lock.Unlock()

		res := make(map[string]int, len(consolidationCntMap))
		for k, v := range consolidationCntMap {
			res[k] = v
		}

		return res
	}, lastBatchIDs
}
