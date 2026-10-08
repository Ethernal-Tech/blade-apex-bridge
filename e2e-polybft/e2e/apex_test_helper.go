package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/e2e-polybft/cardanofw"
	"github.com/0xPolygon/polygon-edge/helper/hex"
	infracommon "github.com/Ethernal-Tech/cardano-infrastructure/common"
	"github.com/Ethernal-Tech/cardano-infrastructure/sendtx"
	"github.com/Ethernal-Tech/cardano-infrastructure/wallet"
	"github.com/stretchr/testify/require"
)

type testConfig struct {
	srcChainID cardanofw.ChainID
	dstChainID cardanofw.ChainID

	srcNetworkType  wallet.CardanoNetworkType
	srcTxProvider   wallet.ITxProvider
	srcMultiSigAddr string

	bridgingFee uint64
}

const (
	defaultLovelaceAmount = uint64(1_000_000)

	metadataMapKey = 1
)

func newTestConfig(
	t *testing.T, config *cardanofw.TestCardanoChainConfig, info *cardanofw.CardanoChainInfo, dstChainID cardanofw.ChainID,
	brFee uint64,
) *testConfig {
	t.Helper()

	txProvider, err := info.GetTxProvider()
	require.NoError(t, err)

	return &testConfig{
		srcChainID:      config.ChainID,
		dstChainID:      dstChainID,
		srcNetworkType:  config.NetworkType,
		srcTxProvider:   txProvider,
		srcMultiSigAddr: info.MultisigAddr,
		bridgingFee:     brFee,
	}
}

// Util methods
func WaitForTestResult(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, config *testConfig, user *cardanofw.TestApexUser,
	txHash string, beforeSendingAmountDfm *big.Int, sentAmount uint64,
	refundEnabled bool, maxWaitTimeSec, retryIntervalSec uint,
) {
	t.Helper()

	retryIntervalSec = max(retryIntervalSec, 1)
	numRetries := max(1, int(maxWaitTimeSec/retryIntervalSec))

	if refundEnabled {
		lowerBoundaryDfm := new(big.Int).Sub(beforeSendingAmountDfm, new(big.Int).SetUint64(sentAmount))

		fmt.Printf("Tx sent. hash: %s, lowerBoundaryDfm: %d, higherBoundaryDfm: %+v\n", txHash, lowerBoundaryDfm,
			beforeSendingAmountDfm)

		err := apex.WaitForAmountInRange(ctx, user, config.srcChainID, lowerBoundaryDfm,
			beforeSendingAmountDfm, numRetries, time.Second*time.Duration(retryIntervalSec))
		require.NoError(t, err)
	} else {
		cardanofw.WaitForInvalidState(t, ctx, apex, config.srcChainID, txHash, apex.Config.APIKey, maxWaitTimeSec)
	}
}

// Test methods
func executeInvalidMismatchSendLovelaceAmount(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, config *testConfig, user *cardanofw.TestApexUser,
	maxWaitTimeSec, retryIntervalSec uint, refundEnabled bool,
) {
	t.Helper()

	receivers := createReceivers(apex, 1, config.dstChainID, defaultLovelaceAmount*10)

	metadata, _ := createMetadata(t, ctx, apex, config.srcChainID, config.dstChainID, config.bridgingFee,
		user, receivers)

	beforeSendingAmountDfm, err := apex.GetBalance(ctx, user, config.srcChainID)
	require.NoError(t, err)

	lovelaceAmount, waitForAmount := getDefaultSendAmounts(t, config)

	txHash, err := apex.SubmitTx(
		ctx, config.srcChainID, user, apex.GetCardanoInfo(config.srcChainID).MultisigAddr,
		lovelaceAmount, nil, metadata)
	require.NoError(t, err)

	WaitForTestResult(t, ctx, apex, config, user, txHash, beforeSendingAmountDfm, waitForAmount,
		refundEnabled, maxWaitTimeSec, retryIntervalSec)

	require.NoError(t, err)
}

func executeInvalidMismatchSendAmountMultipleInstances(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, config *testConfig,
	maxWaitTimeSec, retryIntervalSec uint, refundEnabled bool,
) {
	t.Helper()

	const instances = 5

	for i := 0; i < instances; i++ {
		receivers := createReceivers(apex, 1, config.dstChainID, defaultLovelaceAmount*10)

		metadata, _ := createMetadata(t, ctx, apex, config.srcChainID, config.dstChainID, config.bridgingFee,
			apex.Users[i], receivers)

		beforeSendingAmountDfm, err := apex.GetBalance(ctx, apex.Users[i], config.srcChainID)
		require.NoError(t, err)

		lovelaceAmount, waitForAmount := getDefaultSendAmounts(t, config)

		txHash, err := apex.SubmitTx(
			ctx, config.srcChainID, apex.Users[i],
			apex.GetCardanoInfo(config.srcChainID).MultisigAddr, lovelaceAmount, nil, metadata)
		require.NoError(t, err)

		WaitForTestResult(t, ctx, apex, config, apex.Users[i], txHash, beforeSendingAmountDfm, waitForAmount,
			refundEnabled, maxWaitTimeSec, retryIntervalSec)
	}
}

func executeInvalidMismatchSendAmountMultipleInstancesParalel(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, config *testConfig,
	maxWaitTimeSec, retryIntervalSec uint, refundEnabled bool,
) {
	t.Helper()

	instances := 5

	var wg sync.WaitGroup

	for i := 0; i < instances; i++ {
		wg.Add(1)

		go func(idx int) {
			defer wg.Done()

			receivers := createReceivers(apex, 1, config.dstChainID, defaultLovelaceAmount*10)

			metadata, _ := createMetadata(t, ctx, apex, config.srcChainID, config.dstChainID, config.bridgingFee,
				apex.Users[i], receivers)

			beforeSendingAmountDfm, err := apex.GetBalance(ctx, apex.Users[idx], config.srcChainID)
			require.NoError(t, err)

			lovelaceAmount, waitForAmount := getDefaultSendAmounts(t, config)

			txHashe, err := apex.SubmitTx(
				ctx, config.srcChainID, apex.Users[idx],
				apex.GetCardanoInfo(config.srcChainID).MultisigAddr, lovelaceAmount, nil, metadata)
			require.NoError(t, err)

			WaitForTestResult(t, ctx, apex, config, apex.Users[idx], txHashe, beforeSendingAmountDfm, waitForAmount,
				refundEnabled, maxWaitTimeSec, retryIntervalSec)
		}(i)
	}

	wg.Wait()
}

func executeInvalidMetadataType(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, config *testConfig, user *cardanofw.TestApexUser,
	maxWaitTimeSec, retryIntervalSec uint, refundEnabled bool,
) {
	t.Helper()

	receivers := createReceivers(apex, 1, config.dstChainID, defaultLovelaceAmount)

	metadata, _ := createMetadata(t, ctx, apex, config.srcChainID, config.dstChainID, config.bridgingFee,
		user, receivers)
	metadata = bytes.Replace(metadata, []byte("bridge"), []byte("xxxxx"), 1)

	beforeSendingAmountDfm, err := apex.GetBalance(ctx, user, config.srcChainID)
	require.NoError(t, err)

	lovelaceAmount, waitForAmount := getDefaultSendAmounts(t, config)

	txHash, err := apex.SubmitTx(
		ctx, config.srcChainID, user, apex.GetCardanoInfo(config.srcChainID).MultisigAddr,
		lovelaceAmount, nil, metadata)
	require.NoError(t, err)

	if refundEnabled {
		WaitForTestResult(t, ctx, apex, config, user, txHash, beforeSendingAmountDfm, waitForAmount,
			refundEnabled, maxWaitTimeSec, retryIntervalSec)
	} else {
		_, err = cardanofw.WaitForRequestStates(ctx, apex, config.srcChainID, txHash, apex.Config.APIKey, nil, maxWaitTimeSec)
		require.Error(t, err)
		require.ErrorContains(t, err, "timeout")
	}
}

func executeInvalidDestination(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, config *testConfig, user *cardanofw.TestApexUser,
	maxWaitTimeSec, retryIntervalSec uint, refundEnabled bool,
) {
	t.Helper()

	receivers := createReceivers(apex, 0, config.dstChainID, defaultLovelaceAmount)
	receiversForFeeCalculation := []sendtx.BridgingTxReceiver{
		{
			Addr:   user.GetAddress(config.dstChainID),
			Amount: defaultLovelaceAmount,
		},
	}

	srcTestChain := apex.GetChainMust(t, config.srcChainID)

	feeAmount, err := srcTestChain.GetBridgingFee(
		ctx, config.dstChainID, receiversForFeeCalculation, config.bridgingFee, config.srcMultiSigAddr)
	require.NoError(t, err)

	metadata, err := srcTestChain.CreateMetadata(
		user.GetAddress(config.srcChainID), config.dstChainID, receivers, feeAmount)
	require.NoError(t, err)

	metadata = bytes.Replace(metadata, fmt.Appendf([]byte("\"%s\""), config.dstChainID), []byte("\"unknown\""), 1)

	beforeSendingAmountDfm, err := apex.GetBalance(ctx, user, config.srcChainID)
	require.NoError(t, err)

	lovelaceAmount, waitForAmount := getDefaultSendAmounts(t, config)

	txHash, err := apex.SubmitTx(
		ctx, config.srcChainID, user, apex.GetCardanoInfo(config.srcChainID).MultisigAddr,
		lovelaceAmount, nil, metadata)
	require.NoError(t, err)

	WaitForTestResult(t, ctx, apex, config, user, txHash, beforeSendingAmountDfm, waitForAmount,
		refundEnabled, maxWaitTimeSec, retryIntervalSec)
}

func executeInvalidMetadataInvalidSender(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, config *testConfig, user *cardanofw.TestApexUser,
	maxWaitTimeSec uint,
) {
	t.Helper()

	receivers := createReceivers(apex, 1, config.dstChainID, defaultLovelaceAmount)

	srcTestChain := apex.GetChainMust(t, config.srcChainID)

	feeAmount, err := srcTestChain.GetBridgingFee(
		ctx, config.dstChainID, receivers, config.bridgingFee, config.srcMultiSigAddr)
	require.NoError(t, err)

	metadata, err := srcTestChain.CreateMetadata(
		"dummy", config.dstChainID, receivers, feeAmount)
	require.NoError(t, err)

	// remove this after we make correct validation on oracle!
	metadata = bytes.Replace(metadata, []byte("[\"dummy\"]"), []byte("\"\""), 1)

	lovelaceAmount, _ := getDefaultSendAmounts(t, config)

	txHash, err := apex.SubmitTx(
		ctx, config.srcChainID, user, apex.GetCardanoInfo(config.srcChainID).MultisigAddr,
		lovelaceAmount, nil, metadata)
	require.NoError(t, err)

	fmt.Printf("Tx sent. hash: %s\n", txHash)

	cardanofw.WaitForInvalidState(t, ctx, apex, config.srcChainID, txHash, apex.Config.APIKey, maxWaitTimeSec)
}

func executeInvalidEmptyReceivers(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, config *testConfig, user *cardanofw.TestApexUser,
	maxWaitTimeSec, retryIntervalSec uint, refundEnabled bool,
) {
	t.Helper()

	receivers := []sendtx.BridgingTxReceiver{}

	receiversForFeeCalculation := []sendtx.BridgingTxReceiver{
		{
			Addr:   user.GetAddress(config.dstChainID),
			Amount: defaultLovelaceAmount,
		},
	}

	srcTestChain := apex.GetChainMust(t, config.srcChainID)

	feeAmount, err := srcTestChain.GetBridgingFee(
		ctx, config.dstChainID, receiversForFeeCalculation, config.bridgingFee, config.srcMultiSigAddr)
	require.NoError(t, err)

	metadata, err := srcTestChain.CreateMetadata(
		user.GetAddress(config.srcChainID), config.dstChainID, receivers, feeAmount)
	require.NoError(t, err)

	beforeSendingAmountDfm, err := apex.GetBalance(ctx, user, config.srcChainID)
	require.NoError(t, err)

	lovelaceAmount, waitForAmount := getDefaultSendAmounts(t, config)

	txHash, err := apex.SubmitTx(
		ctx, config.srcChainID, user, apex.GetCardanoInfo(config.srcChainID).MultisigAddr,
		lovelaceAmount, nil, metadata)
	require.NoError(t, err)

	WaitForTestResult(t, ctx, apex, config, user, txHash, beforeSendingAmountDfm, waitForAmount,
		refundEnabled, maxWaitTimeSec, retryIntervalSec)
}

func getDefaultSendAmounts(t *testing.T, config *testConfig,
) (*big.Int, uint64) {
	t.Helper()

	lovelaceAmount := defaultLovelaceAmount + config.bridgingFee
	waitForAmount := lovelaceAmount

	return new(big.Int).SetUint64(lovelaceAmount), waitForAmount
}

func createMetadata(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem,
	srcChain, dstChain cardanofw.ChainID, bridgingFee uint64,
	sender *cardanofw.TestApexUser, receivers []sendtx.BridgingTxReceiver,
) ([]byte, uint64) {
	t.Helper()

	srcTestChain := apex.GetChainMust(t, srcChain)

	multisig := srcTestChain.GetHotWalletAddress()

	feeAmount, err := srcTestChain.GetBridgingFee(ctx, dstChain, receivers, bridgingFee, multisig)
	require.NoError(t, err)

	metadata, err := srcTestChain.CreateMetadata(sender.GetAddress(srcChain), dstChain, receivers, feeAmount)
	require.NoError(t, err)

	return metadata, feeAmount
}

func createReceivers(
	apex *cardanofw.ApexSystem, receiversCount int, dstChain string, sendAmount uint64,
) []sendtx.BridgingTxReceiver {
	receivers := make([]sendtx.BridgingTxReceiver, receiversCount)

	for i := range receivers {
		receivers[i] = sendtx.BridgingTxReceiver{
			Addr:   apex.Users[len(apex.Users)-1-i].GetAddress(dstChain),
			Amount: sendAmount,
		}
	}

	return receivers
}

// executeBridgingWithAuxDataVariant submits a bridging request whose metadata json and
// whose built transaction are each passed through an optional hook, then requires it to
// bridge through successfully.
//
// The point of every caller is the same: what rides alongside the bridging request in
// auxiliary_data - which of the three envelopes babbage accepts wraps it, native
// scripts, other metadata labels - must not decide whether the request is readable. The
// oracle sees the auxiliary_data cbor exactly as it sits in the block, so a decoder
// stricter than the ledger silently drops requests the chain accepted.
func executeBridgingWithAuxDataVariant(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, config *testConfig,
	user *cardanofw.TestApexUser, maxWaitTimeSec, retryIntervalSec uint,
	mutateMetadata func(metadataJSON []byte) ([]byte, error),
	mutateRawTx func(txRaw []byte) ([]byte, error),
) {
	t.Helper()

	receivers := createReceivers(apex, 1, config.dstChainID, defaultLovelaceAmount)
	receiver := apex.Users[len(apex.Users)-1]

	metadata, feeAmount := createMetadata(t, ctx, apex, config.srcChainID, config.dstChainID, config.bridgingFee,
		user, receivers)

	if mutateMetadata != nil {
		var err error

		metadata, err = mutateMetadata(metadata)
		require.NoError(t, err)
	}

	balanceBefore, err := apex.GetBalance(ctx, receiver, config.dstChainID)
	require.NoError(t, err)

	lovelaceAmount := new(big.Int).SetUint64(defaultLovelaceAmount + feeAmount)

	// the envelope the oracle will have to read is decided here, after the tx is built
	var submittedAuxData []byte

	rewrite := mutateRawTx
	if rewrite != nil {
		rewrite = func(txRaw []byte) ([]byte, error) {
			rewritten, err := mutateRawTx(txRaw)
			if err != nil {
				return nil, err
			}

			if submittedAuxData, err = cardanofw.TxAuxiliaryData(rewritten); err != nil {
				return nil, err
			}

			return rewritten, nil
		}
	}

	txHash, err := apex.SubmitTxWithRawTxMutator(
		ctx, config.srcChainID, user, apex.GetCardanoInfo(config.srcChainID).MultisigAddr,
		lovelaceAmount, nil, metadata, rewrite)
	require.NoError(t, err)

	if submittedAuxData != nil {
		fmt.Printf("Tx sent. hash: %s, auxiliary_data envelope: %s\n",
			txHash, cardanofw.AuxiliaryDataEnvelopeName(submittedAuxData))
	} else {
		fmt.Printf("Tx sent. hash: %s\n", txHash)
	}

	expectedAmount := new(big.Int).Add(balanceBefore, new(big.Int).SetUint64(defaultLovelaceAmount))

	numRetries := max(1, int(maxWaitTimeSec/retryIntervalSec))

	require.NoError(t, apex.WaitForExactAmount(ctx, receiver, config.dstChainID, expectedAmount,
		numRetries, time.Second*time.Duration(retryIntervalSec)))
}

// executePhase2InvalidTxs has attacker send every source's bridging address, one scenario
// at a time, a script tx that fails phase-2, then has controlSender send each of them the
// same tx with a policy that succeeds. See TestE2E_ApexBridge_Phase2InvalidTxs for why.
//
// It requires the bridge to act as if the failed txs had never been sent - no bridging
// request state for them, none of their declared outputs among the bridge utxos, nothing
// paid out to victim and nothing refunded to attacker - and the control txs to bridge
// normally. Sources run in parallel.
func executePhase2InvalidTxs(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, sources []*testConfig,
	attacker, victim, controlSender, controlReceiver *cardanofw.TestApexUser,
	maxWaitTimeSec, retryIntervalSec uint,
) {
	t.Helper()

	type scenario struct {
		name string
		// metadata is what the tx carries, given the bridging request a valid tx from the
		// same source would carry. nil means no metadata at all.
		metadata           func(cfg *testConfig, bridgingRequest []byte) []byte
		collateralToBridge bool
		mutateRawTx        func(txRaw []byte) ([]byte, error)
	}

	type sentTx struct {
		scenario string
		info     *cardanofw.ScriptTxInfo
	}

	// collateral comes from the smallest pure ada utxo able to carry it. Setting a small one
	// aside, for the first scenario to take, caps what returning collateral to the bridging
	// address costs.
	for _, cfg := range sources {
		_, err := apex.SubmitTx(ctx, cfg.srcChainID, attacker, attacker.GetAddress(cfg.srcChainID),
			cardanofw.ApexToDfm(big.NewInt(3)), nil, nil)
		require.NoError(t, err)
	}

	bridgingRequestFor := func(
		cfg *testConfig, sender, receiver *cardanofw.TestApexUser, amount uint64,
	) ([]byte, uint64) {
		return createMetadata(t, ctx, apex, cfg.srcChainID, cfg.dstChainID, cfg.bridgingFee, sender,
			[]sendtx.BridgingTxReceiver{
				{
					Addr:   receiver.GetAddress(cfg.dstChainID),
					Amount: amount,
				},
			})
	}

	metadataOfType := func(fields map[string]interface{}) []byte {
		metadata, err := json.Marshal(map[int]map[string]interface{}{metadataMapKey: fields})
		require.NoError(t, err)

		return metadata
	}

	asIs := func(_ *testConfig, bridgingRequest []byte) []byte {
		return bridgingRequest
	}

	scenarios := []scenario{
		{
			// first, so that it takes the small utxo set aside above
			name:               "bridging request, collateral returned to the bridging address",
			metadata:           asIs,
			collateralToBridge: true,
		},
		{
			name:     "bridging request",
			metadata: asIs,
		},
		{
			name:     "bridging request in shelley-ma envelope",
			metadata: asIs,
			mutateRawTx: func(txRaw []byte) ([]byte, error) {
				return cardanofw.RewriteTxAuxiliaryDataToShelleyMA(txRaw, nil)
			},
		},
		{
			// invalid as a bridging request, so a valid tx like it would be refunded
			name: "bridging request for more than is sent",
			metadata: func(cfg *testConfig, _ []byte) []byte {
				metadata, _ := bridgingRequestFor(cfg, attacker, victim, defaultLovelaceAmount*10)

				return metadata
			},
		},
		{
			name: "refund request",
			metadata: func(cfg *testConfig, _ []byte) []byte {
				return metadataOfType(map[string]interface{}{
					"t": "refund",
					"s": cardanofw.AddrToMetaDataAddr(attacker.GetAddress(cfg.srcChainID)),
				})
			},
		},
		{
			// the oracle hands txs of a type it does not know to the refund processor
			name: "unknown tx type",
			metadata: func(_ *testConfig, _ []byte) []byte {
				return metadataOfType(map[string]interface{}{"t": "unknown"})
			},
		},
		{
			name: "batch execution",
			metadata: func(_ *testConfig, _ []byte) []byte {
				return metadataOfType(map[string]interface{}{"t": "batch", "n": 1})
			},
		},
		{
			name: "hot wallet funding",
			metadata: func(_ *testConfig, _ []byte) []byte {
				return metadataOfType(map[string]interface{}{"t": "fund"})
			},
		},
		{
			// the oracle hands txs without metadata to the hot wallet funding processor
			name: "no metadata",
		},
	}

	var (
		lovelaces       = make([]*big.Int, len(sources))
		metadatas       = make([][][]byte, len(sources))
		attackerBalance = make([]*big.Int, len(sources))
		victimBalance   = make([]*big.Int, len(sources))
	)

	for i, cfg := range sources {
		bridgingRequest, feeAmount := bridgingRequestFor(cfg, attacker, victim, defaultLovelaceAmount)

		// every tx sends what the valid bridging request asks for, whatever its metadata says
		lovelaces[i] = new(big.Int).SetUint64(defaultLovelaceAmount + feeAmount)

		for _, sc := range scenarios {
			var metadata []byte
			if sc.metadata != nil {
				metadata = sc.metadata(cfg, bridgingRequest)
			}

			metadatas[i] = append(metadatas[i], metadata)
		}

		var err error

		attackerBalance[i], err = apex.GetBalance(ctx, attacker, cfg.srcChainID)
		require.NoError(t, err)

		victimBalance[i], err = apex.GetBalance(ctx, victim, cfg.dstChainID)
		require.NoError(t, err)
	}

	var (
		sent = make([][]sentTx, len(sources))
		errs = make([]error, len(sources))
		wg   sync.WaitGroup
	)

	for i, cfg := range sources {
		wg.Add(1)

		go func(i int, cfg *testConfig) {
			defer wg.Done()

			// one at a time: every tx takes the collateral the previous one returned
			for j, sc := range scenarios {
				returnAddr := ""
				if sc.collateralToBridge {
					returnAddr = cfg.srcMultiSigAddr
				}

				info, err := apex.SubmitScriptTx(
					ctx, cfg.srcChainID, attacker, cfg.srcMultiSigAddr, lovelaces[i], nil, metadatas[i][j],
					cardanofw.ScriptTxConfig{
						ScriptFails:          true,
						CollateralReturnAddr: returnAddr,
						MutateRawTx:          sc.mutateRawTx,
					})
				if err != nil {
					errs[i] = fmt.Errorf("%s -> %s, %s: %w", cfg.srcChainID, cfg.dstChainID, sc.name, err)

					return
				}

				fmt.Printf("%s -> %s, %s: failed tx %s is on chain\n", cfg.srcChainID, cfg.dstChainID, sc.name, info.TxHash)

				sent[i] = append(sent[i], sentTx{scenario: sc.name, info: info})
			}
		}(i, cfg)
	}

	wg.Wait()

	require.NoError(t, errors.Join(errs...))

	var (
		controls        = make([]*cardanofw.ScriptTxInfo, len(sources))
		controlBalances = make([]*big.Int, len(sources))
	)

	for i, cfg := range sources {
		metadata, feeAmount := bridgingRequestFor(cfg, controlSender, controlReceiver, defaultLovelaceAmount)

		var err error

		controlBalances[i], err = apex.GetBalance(ctx, controlReceiver, cfg.dstChainID)
		require.NoError(t, err)

		controls[i], err = apex.SubmitScriptTx(
			ctx, cfg.srcChainID, controlSender, cfg.srcMultiSigAddr,
			new(big.Int).SetUint64(defaultLovelaceAmount+feeAmount), nil, metadata, cardanofw.ScriptTxConfig{})
		require.NoError(t, err)

		fmt.Printf("%s -> %s, control tx %s is on chain\n", cfg.srcChainID, cfg.dstChainID, controls[i].TxHash)
	}

	apiKey := apex.Config.APIKey

	apiURLs, err := apex.GetBridgingAPIs()
	require.NoError(t, err)

	// checked before the control txs are waited on to execute: a bridge that took the failed
	// txs in tries to spend their outputs and stalls, which would hide why
	for _, apiURL := range apiURLs {
		for i, cfg := range sources {
			// the control tx came after all the failed ones, so an oracle that has it has
			// already read every one of them
			waitForRequestState(t, ctx, apiURL, apiKey, cfg.srcChainID, controls[i].TxHash)

			oracleState, err := cardanofw.GetOracleState(
				ctx, fmt.Sprintf("%s/api/OracleState/Get?chainId=%s", apiURL, cfg.srcChainID), apiKey)
			require.NoError(t, err)

			for _, tx := range sent[i] {
				_, err := cardanofw.GetBridgingRequestState(
					ctx, requestStateURL(apiURL, cfg.srcChainID, tx.info.TxHash), apiKey)
				require.ErrorContains(t, err, fmt.Sprintf("code is %d", http.StatusNotFound),
					"%s: %s has a bridging request state for failed tx %s", tx.scenario, apiURL, tx.info.TxHash)

				for _, utxo := range oracleState.Utxos {
					// the collateral return is the one output a failed tx really creates
					if hex.EncodeToString(utxo.Hash[:]) == tx.info.TxHash {
						require.Equal(t, tx.info.OutputsCount, utxo.Index,
							"%s: %s holds output %d of failed tx %s among the bridge utxos",
							tx.scenario, apiURL, utxo.Index, tx.info.TxHash)
					}
				}
			}
		}
	}

	numRetries := max(1, int(maxWaitTimeSec/retryIntervalSec))

	for i, cfg := range sources {
		_, err := cardanofw.WaitForRequestStates(ctx, apex, cfg.srcChainID, controls[i].TxHash, apiKey,
			[]string{cardanofw.BatchStateExecuted}, maxWaitTimeSec)
		require.NoError(t, err)

		require.NoError(t, apex.WaitForExactAmount(ctx, controlReceiver, cfg.dstChainID,
			new(big.Int).Add(controlBalances[i], new(big.Int).SetUint64(defaultLovelaceAmount)),
			numRetries, time.Second*time.Duration(retryIntervalSec)))
	}

	for _, apiURL := range apiURLs {
		for i, cfg := range sources {
			hasFailed, err := cardanofw.GetHasTxFailed(ctx, hasTxFailedURL(apiURL, cfg.srcChainID, controls[i]), apiKey)
			require.NoError(t, err)
			require.False(t, hasFailed.Failed, "%s reports control tx %s as failed", apiURL, controls[i].TxHash)

			for _, tx := range sent[i] {
				waitForHasTxFailed(t, ctx, apiURL, apiKey, cfg.srcChainID, tx.info,
					numRetries, time.Second*time.Duration(retryIntervalSec))
			}
		}
	}

	for i, cfg := range sources {
		lost := big.NewInt(0)

		for _, tx := range sent[i] {
			loss := tx.info.TotalCollateral
			if tx.info.CollateralReturnAddr != attacker.GetAddress(cfg.srcChainID) {
				loss = tx.info.CollateralAmount
			}

			lost.Add(lost, new(big.Int).SetUint64(loss))
		}

		expected := new(big.Int).Sub(attackerBalance[i], lost)

		balance, err := apex.GetBalance(ctx, attacker, cfg.srcChainID)
		require.NoError(t, err)

		// the collateral is all the failed txs cost, and nothing came back as a refund
		require.Zero(t, expected.Cmp(balance),
			"attacker on %s: expected %s, got %s", cfg.srcChainID, expected, balance)

		balance, err = apex.GetBalance(ctx, victim, cfg.dstChainID)
		require.NoError(t, err)

		require.Zero(t, victimBalance[i].Cmp(balance),
			"receiver on %s: expected %s, got %s", cfg.dstChainID, victimBalance[i], balance)
	}
}

// waitForRequestState waits for the oracle behind apiURL to have a bridging request state,
// any state, for txHash.
func waitForRequestState(
	t *testing.T, ctx context.Context, apiURL, apiKey string, chainID cardanofw.ChainID, txHash string,
) {
	t.Helper()

	_, err := infracommon.ExecuteWithRetry(ctx, func(ctx context.Context) (bool, error) {
		if _, err := cardanofw.GetBridgingRequestState(ctx, requestStateURL(apiURL, chainID, txHash), apiKey); err != nil {
			return false, infracommon.ErrRetryTryAgain
		}

		return true, nil
	}, infracommon.WithRetryCount(60), infracommon.WithRetryWaitTime(time.Second*2))
	require.NoError(t, err, "%s has no bridging request state for tx %s", apiURL, txHash)
}

func requestStateURL(apiURL string, chainID cardanofw.ChainID, txHash string) string {
	return fmt.Sprintf("%s/api/BridgingRequestState/Get?chainId=%s&txHash=%s", apiURL, chainID, txHash)
}

// waitForHasTxFailed waits for the oracle behind apiURL to report tx as failed, which it
// does once it has indexed past the tx's ttl without having seen the tx. The ttl is a
// slot count, so on a chain with long slots - vector's are ten times prime's - that
// takes minutes.
func waitForHasTxFailed(
	t *testing.T, ctx context.Context, apiURL, apiKey string, chainID cardanofw.ChainID, tx *cardanofw.ScriptTxInfo,
	numRetries int, retryWaitTime time.Duration,
) {
	t.Helper()

	_, err := infracommon.ExecuteWithRetry(ctx, func(ctx context.Context) (bool, error) {
		hasFailed, err := cardanofw.GetHasTxFailed(ctx, hasTxFailedURL(apiURL, chainID, tx), apiKey)
		if err != nil {
			return false, err
		}

		if !hasFailed.Failed {
			return false, infracommon.ErrRetryTryAgain
		}

		return true, nil
	}, infracommon.WithRetryCount(numRetries), infracommon.WithRetryWaitTime(retryWaitTime))
	require.NoError(t, err, "%s does not report failed tx %s as failed", apiURL, tx.TxHash)
}

func hasTxFailedURL(apiURL string, chainID cardanofw.ChainID, tx *cardanofw.ScriptTxInfo) string {
	return fmt.Sprintf("%s/api/OracleState/GetHasTxFailed?chainId=%s&txHash=%s&ttl=%d",
		apiURL, chainID, tx.TxHash, tx.TTL)
}
