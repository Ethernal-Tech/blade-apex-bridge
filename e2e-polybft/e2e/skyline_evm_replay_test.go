package e2e

import (
	"context"
	"fmt"
	"math/big"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/cardanofw"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/e2ehelper"
	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/jsonrpc"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/Ethernal-Tech/ethgo"
	"github.com/Ethernal-Tech/ethgo/abi"
	"github.com/stretchr/testify/require"
)

// ABI fragments of the apex-evm-gateway contracts used by the replay test.
var (
	replayGatewayDepositMethod     = abi.MustNewMethod("function deposit(bytes _signature, uint256 _bitmap, bytes _data)")
	replayGatewayVersionMethod     = abi.MustNewMethod("function version() view returns (string v)")
	replayGatewayPredicateMethod   = abi.MustNewMethod("function nativeTokenPredicate() view returns (address addr)")
	replayGatewayCurrencyIDMethod  = abi.MustNewMethod("function currencyTokenId() view returns (uint16 id)")
	replayPredicateLastBatchMethod = abi.MustNewMethod("function lastBatchId() view returns (uint64 id)")
	replayPredicateTokenInfoMethod = abi.MustNewMethod(
		"function getTokenInfo(uint16 _tokenId) view returns (tuple(address addr, bool isLockUnlock) info)")

	replayDepositEvent = abi.MustNewEvent("event Deposit(bytes data)")

	replayWrongDestinationSelector     = crypto.Keccak256([]byte("WrongDestination()"))[:4]
	replayBatchAlreadyExecutedSelector = crypto.Keccak256([]byte("BatchAlreadyExecuted()"))[:4]

	// _data passed to Gateway.deposit is abi.encode(Deposits), a single dynamic tuple
	replayEVMBatchType = abi.MustNewType("tuple(tuple(uint64 batchId, uint64 ttlExpired, uint256 feeAmount, " +
		"tuple(address receiver, uint256 amount, uint16 tokenId)[] receivers, uint256 chainId, address gateway) deposits)")
)

const expectedGatewayVersion = "1.0.3"

type replayBatchReceiver struct {
	Receiver ethgo.Address `abi:"receiver"`
	Amount   *big.Int      `abi:"amount"`
	TokenID  uint16        `abi:"tokenId"`
}

type replayBatch struct {
	BatchID    uint64                `abi:"batchId"`
	TTLExpired uint64                `abi:"ttlExpired"`
	FeeAmount  *big.Int              `abi:"feeAmount"`
	Receivers  []replayBatchReceiver `abi:"receivers"`
	ChainID    *big.Int              `abi:"chainId"`
	Gateway    ethgo.Address         `abi:"gateway"`
}

// replaySignedBatch is one Gateway.deposit call as the relayer sent it on the source EVM chain.
type replaySignedBatch struct {
	txHash    types.Hash
	signature []byte
	bitmap    *big.Int
	data      []byte
	batch     replayBatch
}

// TestE2E_SkylineBridge_EVMBatchCrossChainReplay is the regression test for audit finding AD-1196 #1
// (cross-chain replay of a signed EVM batch).
//
// The test bridges USDT Vector -> Nexus, takes the (signature, bitmap, data) of the relayer's
// Gateway.deposit tx on Nexus and:
//
//	B) submits the same calldata to the Polygon Gateway from an unrelated account; it must revert
//	   with WrongDestination and move no funds and no lastBatchId;
//	C) submits it again to the Nexus Gateway; it must revert with BatchAlreadyExecuted.
//
// Environment:
//
//	APEX_BRIDGE_BINARY               apex-bridge binary built with the destination fields
//	SKIP_GATEWAY_VERSION_CHECK=true  skip the Gateway version() == "1.0.3" precondition
//
// cd e2e-polybft/e2e
// go test -v -timeout 0 -run ^TestE2E_SkylineBridge_EVMBatchCrossChainReplay$ github.com/0xPolygon/polygon-edge/e2e-polybft/e2e
func TestE2E_SkylineBridge_EVMBatchCrossChainReplay(t *testing.T) {
	const (
		apiKey  = "test_api_key"
		userCnt = 2
	)

	ctx, cncl := context.WithCancel(context.Background())
	defer cncl()

	vectorConfig := cardanofw.NewVectorChainConfig(map[uint16]string{cardanofw.USDTTokenID: cardanofw.USDTTokenName})
	nexusConfig := cardanofw.NewNexusChainConfig(true)
	polygonConfig := cardanofw.NewPolygonChainConfig(true)

	apex := cardanofw.SetupAndRunSkylineBridge(
		t, ctx,
		cardanofw.WithAPIKey(apiKey),
		cardanofw.WithUserCnt(userCnt),
		cardanofw.WithVectorConfig(vectorConfig),
		cardanofw.WithNexusConfig(nexusConfig),
		cardanofw.WithPolygonConfig(polygonConfig),
		cardanofw.WithBridgingAddrCnt(cardanofw.ChainIDPrime, bridgeAddrCnt),
	)

	defer require.True(t, apex.ApexBridgeProcessesRunning())

	user := apex.Users[0]
	nexusChain := apex.GetChainMust(t, cardanofw.ChainIDNexus).(*cardanofw.TestEVMChain)
	polygonChain := apex.GetChainMust(t, cardanofw.ChainIDPolygon).(*cardanofw.TestEVMChain)

	nexusRPC, err := nexusChain.JSONRPC()
	require.NoError(t, err)

	polygonRPC, err := polygonChain.JSONRPC()
	require.NoError(t, err)

	nexusGateway := apex.NexusInfo.GatewayAddress
	polygonGateway := apex.PolygonInfo.GatewayAddress
	polygonRPCAddr := apex.PolygonInfo.JSONRPCAddr
	nexusRPCAddr := apex.NexusInfo.JSONRPCAddr

	// S1: the contracts under test must be the fixed ones.
	if os.Getenv("SKIP_GATEWAY_VERSION_CHECK") != "true" {
		for name, c := range map[string]struct {
			rpc     *jsonrpc.EthClient
			gateway types.Address
		}{"Nexus": {nexusRPC, nexusGateway}, "Polygon": {polygonRPC, polygonGateway}} {
			version := replayCallString(t, c.rpc, c.gateway, replayGatewayVersionMethod)
			require.Equal(t, expectedGatewayVersion, version,
				"%s Gateway version is %q: the e2e must deploy the fixed apex-evm-gateway "+
					"(deploy-evm --branch in cardanofw InitContracts) "+
					"(or set SKIP_GATEWAY_VERSION_CHECK=true to run against the old contracts)",
				name, version)
		}
	}

	// S7: log both chain ids. They are equal in the e2e (F14), so only the Gateway binding is proven here.
	nexusChainID, err := nexusRPC.ChainID()
	require.NoError(t, err)

	polygonChainID, err := polygonRPC.ChainID()
	require.NoError(t, err)

	t.Logf("eth_chainId: Nexus=%s Polygon=%s equal=%t", nexusChainID, polygonChainID,
		nexusChainID.Cmp(polygonChainID) == 0)

	// 1. Produce a legitimate EVM batch on Nexus that pays USDT.
	// Nexus -> Vector first, so the Nexus NativeTokenWallet holds USDT it can unlock later.
	fundAmount := cardanofw.DfmToWei(big.NewInt(100))
	bridgeAmount := cardanofw.DfmToWei(big.NewInt(5))

	require.NoError(t, nexusChain.FundUsersWithToken(
		user.GetAddress(cardanofw.ChainIDNexus), fundAmount, cardanofw.USDTTokenID))

	e2ehelper.ExecuteSingleBridging(
		t, ctx, apex, user, user, cardanofw.ChainIDNexus, cardanofw.ChainIDVector,
		new(big.Int).Mul(bridgeAmount, big.NewInt(2)), cardanofw.USDTTokenID, false)

	nexusFromBlock, err := nexusRPC.BlockNumber()
	require.NoError(t, err)

	// S2: the receiver gets the USDT on Nexus (ExecuteSingleBridging waits for and checks the balance).
	e2ehelper.ExecuteSingleBridging(
		t, ctx, apex, user, user, cardanofw.ChainIDVector, cardanofw.ChainIDNexus,
		bridgeAmount, cardanofw.USDTTokenID, false)

	// 2. Read the relayer's Gateway.deposit call from Nexus.
	receiverOnNexus := types.StringToAddress(user.GetAddress(cardanofw.ChainIDNexus))
	signed := findReplaySignedBatch(t, nexusRPC, nexusGateway, nexusFromBlock, receiverOnNexus, cardanofw.USDTTokenID)

	t.Logf("Nexus batch: tx=%s id=%d ttl=%d fee=%s bitmap=%s receivers=%+v chainId=%s gateway=%s",
		signed.txHash, signed.batch.BatchID, signed.batch.TTLExpired, signed.batch.FeeAmount,
		signed.bitmap, signed.batch.Receivers, signed.batch.ChainID, signed.batch.Gateway)

	require.Equal(t, nexusGateway, types.Address(signed.batch.Gateway),
		"signed batch must carry the Nexus Gateway address")
	require.Equal(t, 0, nexusChainID.Cmp(signed.batch.ChainID),
		"signed batch must carry the Nexus eth_chainId %s, got %s", nexusChainID, signed.batch.ChainID)

	// 3. Preconditions on Polygon: every token in the batch must be payable there and
	// the batch id must still be ahead of Polygon's lastBatchId.
	polygonPredicate := replayCallAddress(t, polygonRPC, polygonGateway, replayGatewayPredicateMethod)
	polygonCurrencyID := replayCallUint16(t, polygonRPC, polygonGateway, replayGatewayCurrencyIDMethod)

	polygonTokenAddrs := map[uint16]types.Address{}

	for _, r := range signed.batch.Receivers {
		if r.TokenID == polygonCurrencyID {
			continue
		}

		tokenAddr := replayTokenAddress(t, polygonRPC, polygonPredicate, r.TokenID)
		require.NotEqual(t, types.ZeroAddress, tokenAddr,
			"precondition: token id %d from the Nexus batch is not registered on Polygon", r.TokenID)

		polygonTokenAddrs[r.TokenID] = tokenAddr
	}

	polygonLastBatchBefore := replayCallUint64(t, polygonRPC, polygonPredicate, replayPredicateLastBatchMethod)
	require.Less(t, polygonLastBatchBefore, signed.batch.BatchID,
		"precondition: Polygon lastBatchId must be lower than the Nexus batch id")

	t.Run("B. Replaying the Nexus batch on the Polygon Gateway reverts with WrongDestination", func(t *testing.T) {
		attacker := newFundedReplayAttacker(t, polygonChain, polygonRPC, polygonRPCAddr)

		balancesBefore := replayReceiverBalances(t, ctx, polygonChain, polygonRPC, signed.batch, polygonTokenAddrs)

		requireReplayCallRevertsWith(t, polygonRPC, attacker.Address(), polygonGateway, signed,
			replayWrongDestinationSelector, "WrongDestination")

		receipt, sendErr := trySendReplayDeposit(polygonRPCAddr, attacker, polygonGateway, signed)
		requireReplayReverted(t, receipt, sendErr, replayWrongDestinationSelector, "WrongDestination")

		balancesAfter := replayReceiverBalances(t, ctx, polygonChain, polygonRPC, signed.batch, polygonTokenAddrs)

		for _, r := range signed.batch.Receivers {
			key := replayBalanceKey(r)
			require.Equal(t, 0, balancesAfter[key].Cmp(balancesBefore[key]),
				"receiver %s token %d balance on Polygon changed: %s -> %s",
				r.Receiver, r.TokenID, balancesBefore[key], balancesAfter[key])
		}

		require.Equal(t, polygonLastBatchBefore,
			replayCallUint64(t, polygonRPC, polygonPredicate, replayPredicateLastBatchMethod),
			"Polygon lastBatchId must not move")
	})

	t.Run("C. Second submission on the Nexus Gateway reverts with BatchAlreadyExecuted", func(t *testing.T) {
		attacker := newFundedReplayAttacker(t, nexusChain, nexusRPC, nexusRPCAddr)

		nexusPredicate := replayCallAddress(t, nexusRPC, nexusGateway, replayGatewayPredicateMethod)
		lastBatchBefore := replayCallUint64(t, nexusRPC, nexusPredicate, replayPredicateLastBatchMethod)

		requireReplayCallRevertsWith(t, nexusRPC, attacker.Address(), nexusGateway, signed,
			replayBatchAlreadyExecutedSelector, "BatchAlreadyExecuted")

		receipt, sendErr := trySendReplayDeposit(nexusRPCAddr, attacker, nexusGateway, signed)
		requireReplayReverted(t, receipt, sendErr, replayBatchAlreadyExecutedSelector, "BatchAlreadyExecuted")

		require.Equal(t, lastBatchBefore,
			replayCallUint64(t, nexusRPC, nexusPredicate, replayPredicateLastBatchMethod),
			"Nexus lastBatchId must not move")
	})
}

// requireReplayReverted accepts either a failed send/estimate (revert data or name in the error)
// or a receipt with a failed status; a successful receipt fails the test.
func requireReplayReverted(
	t *testing.T, receipt *ethgo.Receipt, sendErr error, selector []byte, name string,
) {
	t.Helper()

	if sendErr != nil {
		msg := strings.ToLower(sendErr.Error())
		t.Logf("%s: send failed: %v", name, sendErr)

		// the error is only informative: some nodes do not return revert data
		if !strings.Contains(msg, strings.ToLower(name)) && !strings.Contains(msg, hex.EncodeToString(selector)) {
			t.Logf("%s: error does not name the custom error; relying on unchanged state", name)
		}

		return
	}

	require.NotNil(t, receipt)
	require.NotEqual(t, uint64(types.ReceiptSuccess), receipt.Status,
		"%s expected, but the tx succeeded (tx %s)", name, receipt.TransactionHash)
	t.Logf("%s: tx %s failed with status %d", name, receipt.TransactionHash, receipt.Status)
}

// requireReplayCallRevertsWith simulates the replay with eth_call from the attacker and requires the
// revert data to carry the given custom error selector. Blade returns the revert data in the JSON-RPC
// error (error.data), which the client includes in the error text, so the reason is checked exactly
// and a revert for any other reason (InvalidSignature, out of gas, ...) fails the test.
func requireReplayCallRevertsWith(
	t *testing.T, rpc *jsonrpc.EthClient, from, gateway types.Address, signed *replaySignedBatch,
	selector []byte, name string,
) {
	t.Helper()

	input, err := replayGatewayDepositMethod.Encode([]interface{}{signed.signature, signed.bitmap, signed.data})
	require.NoError(t, err)

	_, callErr := rpc.Call(&jsonrpc.CallMsg{From: from, To: &gateway, Data: input}, jsonrpc.LatestBlockNumber, nil)
	require.Error(t, callErr, "eth_call of the replay must revert with %s", name)

	selectorHex := strings.TrimPrefix(hex.EncodeToHex(selector), "0x")
	require.Contains(t, strings.ToLower(callErr.Error()), selectorHex,
		"eth_call of the replay must revert with %s (selector 0x%s), got: %v", name, selectorHex, callErr)
	t.Logf("%s: eth_call reverted with selector 0x%s", name, selectorHex)
}

func findReplaySignedBatch(
	t *testing.T, rpc *jsonrpc.EthClient, gateway types.Address, fromBlock uint64,
	receiver types.Address, tokenID uint16,
) *replaySignedBatch {
	t.Helper()

	from := ethgo.BlockNumber(fromBlock)
	to := ethgo.Latest
	depositTopic := replayDepositEvent.ID()

	logs, err := rpc.GetLogs(&ethgo.LogFilter{
		Address: []ethgo.Address{ethgo.Address(gateway)},
		Topics:  [][]*ethgo.Hash{{&depositTopic}},
		From:    &from,
		To:      &to,
	})
	require.NoError(t, err)

	for i := len(logs) - 1; i >= 0; i-- {
		tx, err := rpc.GetTransactionByHash(types.Hash(logs[i].TransactionHash))
		require.NoError(t, err)
		require.NotNil(t, tx)

		signed, err := decodeReplayDepositCall(tx.Input())
		if err != nil {
			t.Logf("skipping Deposit log in tx %s: %v", logs[i].TransactionHash, err)

			continue
		}

		signed.txHash = tx.Hash()

		for _, r := range signed.batch.Receivers {
			if types.Address(r.Receiver) == receiver && r.TokenID == tokenID {
				return signed
			}
		}
	}

	require.FailNow(t, fmt.Sprintf("no Gateway.deposit on Nexus since block %d pays token %d to %s",
		fromBlock, tokenID, receiver))

	return nil
}

func decodeReplayDepositCall(input []byte) (*replaySignedBatch, error) {
	methodID := replayGatewayDepositMethod.ID()
	if len(input) < len(methodID) || !strings.EqualFold(hex.EncodeToHex(input[:4]), hex.EncodeToHex(methodID)) {
		return nil, fmt.Errorf("tx input is not a Gateway.deposit call")
	}

	args, err := replayGatewayDepositMethod.Inputs.Decode(input[4:])
	if err != nil {
		return nil, fmt.Errorf("decode deposit args: %w", err)
	}

	argsMap, ok := args.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected deposit args type %T", args)
	}

	signature, _ := argsMap["_signature"].([]byte)
	bitmap, _ := argsMap["_bitmap"].(*big.Int)
	data, _ := argsMap["_data"].([]byte)

	if signature == nil || bitmap == nil || data == nil {
		return nil, fmt.Errorf("missing deposit args: %+v", argsMap)
	}

	var wrapper struct {
		Deposits replayBatch `abi:"deposits"`
	}

	if err := replayEVMBatchType.DecodeStruct(data, &wrapper); err != nil {
		return nil, fmt.Errorf("decode batch data: %w", err)
	}

	return &replaySignedBatch{
		signature: signature,
		bitmap:    bitmap,
		data:      data,
		batch:     wrapper.Deposits,
	}, nil
}

func replayCall(
	t *testing.T, rpc *jsonrpc.EthClient, to types.Address, method *abi.Method, args ...interface{},
) map[string]interface{} {
	t.Helper()

	if args == nil {
		args = []interface{}{}
	}

	input, err := method.Encode(args)
	require.NoError(t, err)

	outHex, err := rpc.Call(&jsonrpc.CallMsg{To: &to, Data: input}, jsonrpc.LatestBlockNumber, nil)
	require.NoError(t, err, "call %s on %s", method.Name, to)

	out, err := hex.DecodeHex(outHex)
	require.NoError(t, err)

	res, err := method.Decode(out)
	require.NoError(t, err, "decode %s output %s", method.Name, outHex)

	return res
}

func replayCallAddress(t *testing.T, rpc *jsonrpc.EthClient, to types.Address, method *abi.Method) types.Address {
	t.Helper()

	addr, ok := replayCall(t, rpc, to, method)["addr"].(ethgo.Address)
	require.True(t, ok)

	return types.Address(addr)
}

func replayCallUint64(t *testing.T, rpc *jsonrpc.EthClient, to types.Address, method *abi.Method) uint64 {
	t.Helper()

	val, ok := replayCall(t, rpc, to, method)["id"].(uint64)
	require.True(t, ok)

	return val
}

func replayCallUint16(t *testing.T, rpc *jsonrpc.EthClient, to types.Address, method *abi.Method) uint16 {
	t.Helper()

	val, ok := replayCall(t, rpc, to, method)["id"].(uint16)
	require.True(t, ok)

	return val
}

func replayTokenAddress(
	t *testing.T, rpc *jsonrpc.EthClient, predicate types.Address, tokenID uint16,
) types.Address {
	t.Helper()

	info, ok := replayCall(t, rpc, predicate, replayPredicateTokenInfoMethod, tokenID)["info"].(map[string]interface{})
	require.True(t, ok)

	addr, ok := info["addr"].(ethgo.Address)
	require.True(t, ok)

	return types.Address(addr)
}

func replayCallString(t *testing.T, rpc *jsonrpc.EthClient, to types.Address, method *abi.Method) string {
	t.Helper()

	val, ok := replayCall(t, rpc, to, method)["v"].(string)
	require.True(t, ok)

	return val
}

func replayBalanceKey(r replayBatchReceiver) string {
	return fmt.Sprintf("%s/%d", types.Address(r.Receiver), r.TokenID)
}

// replayReceiverBalances returns the Polygon balance of every (receiver, token) pair of the batch.
func replayReceiverBalances(
	t *testing.T, ctx context.Context, chain *cardanofw.TestEVMChain, rpc *jsonrpc.EthClient,
	batch replayBatch, tokenAddrs map[uint16]types.Address,
) map[string]*big.Int {
	t.Helper()

	res := make(map[string]*big.Int, len(batch.Receivers))

	for _, r := range batch.Receivers {
		receiver := types.Address(r.Receiver)

		tokenAddr, isToken := tokenAddrs[r.TokenID]
		if !isToken {
			balance, err := rpc.GetBalance(receiver, jsonrpc.LatestBlockNumberOrHash)
			require.NoError(t, err)

			res[replayBalanceKey(r)] = balance

			continue
		}

		balances, err := chain.GetAddressBalanceWithTokenName(ctx, receiver.String(), tokenAddr.String())
		require.NoError(t, err)

		res[replayBalanceKey(r)] = cardanofw.SetOrDefault(balances[tokenAddr.String()], big.NewInt(0))
	}

	return res
}

// newFundedReplayAttacker creates a fresh account, unrelated to the bridge, with gas money on the chain.
func newFundedReplayAttacker(
	t *testing.T, chain *cardanofw.TestEVMChain, rpc *jsonrpc.EthClient, rpcAddr string,
) *crypto.ECDSAKey {
	t.Helper()

	attacker, err := crypto.GenerateECDSAKey()
	require.NoError(t, err)

	adminKeyHex, err := chain.GetAdminPrivateKey()
	require.NoError(t, err)

	adminECDSA, err := crypto.HexToECDSA(adminKeyHex)
	require.NoError(t, err)

	admin := crypto.NewECDSAKey(adminECDSA)
	attackerAddr := attacker.Address()

	relayer := newReplayTxRelayer(t, rpcAddr)

	receipt, err := relayer.SendTransaction(types.NewTx(types.NewLegacyTx(
		types.WithFrom(admin.Address()),
		types.WithTo(&attackerAddr),
		types.WithValue(cardanofw.ApexToWei(big.NewInt(10))),
	)), admin)
	require.NoError(t, err)
	require.Equal(t, uint64(types.ReceiptSuccess), receipt.Status)

	balance, err := rpc.GetBalance(attackerAddr, jsonrpc.LatestBlockNumberOrHash)
	require.NoError(t, err)
	require.Positive(t, balance.Sign())

	return attacker
}

func newReplayTxRelayer(t *testing.T, rpcAddr string) txrelayer.TxRelayer {
	t.Helper()

	relayer, err := txrelayer.NewTxRelayer(
		txrelayer.WithIPAddress(rpcAddr),
		txrelayer.WithReceiptsTimeout(time.Minute),
		txrelayer.WithEstimateGasFallback(),
	)
	require.NoError(t, err)

	return relayer
}

func trySendReplayDeposit(
	rpcAddr string, attacker *crypto.ECDSAKey, gateway types.Address, signed *replaySignedBatch,
) (*ethgo.Receipt, error) {
	input, err := replayGatewayDepositMethod.Encode([]interface{}{signed.signature, signed.bitmap, signed.data})
	if err != nil {
		return nil, err
	}

	relayer, err := txrelayer.NewTxRelayer(
		txrelayer.WithIPAddress(rpcAddr),
		txrelayer.WithReceiptsTimeout(time.Minute),
		txrelayer.WithEstimateGasFallback(),
	)
	if err != nil {
		return nil, err
	}

	return relayer.SendTransaction(types.NewTx(types.NewLegacyTx(
		types.WithFrom(attacker.Address()),
		types.WithTo(&gateway),
		types.WithInput(input),
	)), attacker)
}

func sendReplayDeposit(
	t *testing.T, rpcAddr string, attacker *crypto.ECDSAKey, gateway types.Address,
	signed *replaySignedBatch,
) *ethgo.Receipt {
	t.Helper()

	receipt, err := trySendReplayDeposit(rpcAddr, attacker, gateway, signed)
	require.NoError(t, err)

	return receipt
}
