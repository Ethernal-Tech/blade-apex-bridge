package cardanofw

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	infracommon "github.com/Ethernal-Tech/cardano-infrastructure/common"
	infrawallet "github.com/Ethernal-Tech/cardano-infrastructure/wallet"
	"github.com/fxamacker/cbor/v2"
	"golang.org/x/crypto/blake2b"
)

// A transaction that runs a plutus script is validated in two phases. Phase-1 covers
// everything but the scripts - signatures, balance, fee, collateral - and a transaction
// failing it never reaches a block. Phase-2 runs the scripts, and a transaction whose
// script fails there still goes on chain, flagged is_valid=false: the ledger takes its
// collateral and nothing else happens. Its inputs stay unspent and the outputs it
// declares are never created, so an indexer that reads it like any other transaction
// sees a payment that did not happen.
//
// The helpers below send transactions minting through one of two PlutusV2 policies -
// the newest version every test chain has a cost model for - that differ only in
// whether they fail:
//
//	alwaysFailsPolicy    (program 1.0.0 (lam redeemer (error)))
//	alwaysSucceedsPolicy (program 1.0.0 (lam redeemer (lam ctx (con unit ()))))
//
// cborHex is the text envelope encoding: the flat program, cbor-wrapped twice.
const (
	alwaysFailsPolicyCborHex    = "46450100002601"
	alwaysSucceedsPolicyCborHex = "4746010000224981"

	plutusV2LanguageTag = 2
	policyIDSize        = 28

	scriptTxMintTokenName = "phase2"

	// far above what either policy spends, far below any chain's per-tx limit
	scriptTxExUnitsSteps  = 2_000_000
	scriptTxExUnitsMemory = 20_000

	// added to the min fee cardano-cli reports, which it computes on a draft whose fee
	// and collateral fields encode shorter than the final ones
	scriptTxFeeMargin = 50_000

	// collateral is taken from the smallest pure ada utxo holding at least this much -
	// enough for the collateral itself and the collateral return output
	scriptTxMinCollateralUtxo = 2_000_000

	// a tx running a plutus script has its validity bounds translated to time for the
	// script context, which the node can only do up to 3k/f slots past its tip - 300 on
	// prime, whose securityParam is 10. Past that the tx is rejected as PastHorizon.
	scriptTxTTLSlotInc = 200
)

// ScriptTxConfig decides on which side of phase-2 a script tx lands.
type ScriptTxConfig struct {
	// ScriptFails mints through alwaysFailsPolicy and flags the tx is_valid=false, the
	// only way a tx whose script fails is let on chain. Otherwise the tx mints through
	// alwaysSucceedsPolicy - the same tx in every other respect, which goes through.
	ScriptFails bool
	// CollateralReturnAddr receives what is left of the collateral input, the sender
	// when empty. If the script fails, this is the only output the tx creates.
	CollateralReturnAddr string
	// MutateRawTx rewrites the built tx before it is signed
	MutateRawTx func(txRaw []byte) ([]byte, error)
}

// ScriptTxInfo tells a test where a script tx left its traces.
type ScriptTxInfo struct {
	TxHash string
	// OutputsCount is the number of outputs the tx declares. If its script fails the
	// collateral return is created instead of them, at this index.
	OutputsCount uint32
	// TTL is the tx's invalid-hereafter slot
	TTL uint64
	// CollateralAmount is the lovelace in the collateral input. TotalCollateral is the
	// part of it the ledger keeps if the script fails, the rest goes to
	// CollateralReturnAddr.
	CollateralAmount     uint64
	TotalCollateral      uint64
	CollateralReturnAddr string
}

// SendScriptTx pays receivers what SendTx would, laid out the way sendtx lays out a
// bridging request, but has the tx also mint one token through a plutus policy - see
// ScriptTxConfig. All sender utxos are spent and the change, minted token included,
// returns to the sender. It returns once the tx is on chain.
func (ec *TestCardanoChain) SendScriptTx(
	ctx context.Context, privateKey string, metadata []byte, receivers []GenericTxReceiver,
	config ScriptTxConfig,
) (*ScriptTxInfo, error) {
	if len(receivers) == 0 {
		return nil, fmt.Errorf("cardano SendScriptTx supports one or multiple receivers but got zero")
	}

	wallets, policyScript, senderAddr, err := FromCardanoPrivateKeyString(
		privateKey, ec.config.NetworkType, ec.config.NetworkMagic)
	if err != nil {
		return nil, err
	}

	if policyScript != nil {
		return nil, fmt.Errorf("script txs from multisig senders are not supported")
	}

	txProvider, err := ec.GetTxProvider()
	if err != nil {
		return nil, err
	}

	protocolParams, err := infracommon.ExecuteWithRetry(ctx, func(ctx context.Context) ([]byte, error) {
		return txProvider.GetProtocolParameters(ctx)
	})
	if err != nil {
		return nil, err
	}

	utxos, err := infracommon.ExecuteWithRetry(ctx, func(ctx context.Context) ([]infrawallet.Utxo, error) {
		return txProvider.GetUtxos(ctx, senderAddr)
	})
	if err != nil {
		return nil, err
	}

	tip, err := infracommon.ExecuteWithRetry(ctx, func(ctx context.Context) (infrawallet.QueryTipData, error) {
		return txProvider.GetTip(ctx)
	})
	if err != nil {
		return nil, err
	}

	collateral, err := pickCollateralUtxo(utxos)
	if err != nil {
		return nil, fmt.Errorf("sender %s: %w", senderAddr, err)
	}

	policyCborHex := alwaysSucceedsPolicyCborHex
	if config.ScriptFails {
		policyCborHex = alwaysFailsPolicyCborHex
	}

	policyID, err := plutusV2PolicyID(policyCborHex)
	if err != nil {
		return nil, err
	}

	minted := infrawallet.NewTokenAmount(infrawallet.NewToken(policyID, scriptTxMintTokenName), 1)

	outputs := make([]infrawallet.TxOutput, 0, len(receivers)+1)

	for _, r := range receivers {
		outputs = append(outputs, infrawallet.NewTxOutput(r.Addr, r.Amount.Uint64(), r.NativeTokens...))
	}

	returnAddr := config.CollateralReturnAddr
	if returnAddr == "" {
		returnAddr = senderAddr
	}

	baseDir, err := os.MkdirTemp("", "cardano-script-tx")
	if err != nil {
		return nil, err
	}

	defer os.RemoveAll(baseDir)

	var (
		cliBinary    = ResolveCardanoCliBinary(ec.config.NetworkType)
		ppFilePath   = filepath.Join(baseDir, "protocol-parameters.json")
		policyPath   = filepath.Join(baseDir, "policy.plutus")
		metadataPath = filepath.Join(baseDir, "metadata.json")
		txFilePath   = filepath.Join(baseDir, "tx.raw")
		ttl          = tip.Slot + scriptTxTTLSlotInc
	)

	policyEnvelope, err := json.Marshal(map[string]string{
		"type":        "PlutusScriptV2",
		"description": "",
		"cborHex":     policyCborHex,
	})
	if err != nil {
		return nil, err
	}

	files := map[string][]byte{ppFilePath: protocolParams, policyPath: policyEnvelope}
	if metadata != nil {
		files[metadataPath] = metadata
	}

	for path, content := range files {
		if err := os.WriteFile(path, content, 0600); err != nil {
			return nil, err
		}
	}

	build := func(fee, totalCollateral uint64) error {
		change, err := scriptTxChange(senderAddr, utxos, outputs, fee, minted)
		if err != nil {
			return fmt.Errorf("sender %s: %w", senderAddr, err)
		}

		args := []string{
			infrawallet.DefaultEra, "transaction", "build-raw",
			"--protocol-params-file", ppFilePath,
			"--fee", strconv.FormatUint(fee, 10),
			"--invalid-hereafter", strconv.FormatUint(ttl, 10),
			"--out-file", txFilePath,
		}

		for _, utxo := range utxos {
			args = append(args, "--tx-in", infrawallet.NewTxInput(utxo.Hash, utxo.Index).String())
		}

		args = append(args,
			"--tx-in-collateral", infrawallet.NewTxInput(collateral.Hash, collateral.Index).String(),
			"--tx-total-collateral", strconv.FormatUint(totalCollateral, 10),
			"--tx-out-return-collateral", infrawallet.NewTxOutput(returnAddr, collateral.Amount-totalCollateral).String(),
		)

		for _, output := range outputs {
			args = append(args, "--tx-out", output.String())
		}

		args = append(args,
			"--tx-out", change.String(),
			"--mint", minted.String(),
			"--mint-script-file", policyPath,
			"--mint-redeemer-value", "0",
			"--mint-execution-units", fmt.Sprintf("(%d,%d)", scriptTxExUnitsSteps, scriptTxExUnitsMemory),
		)

		if metadata != nil {
			args = append(args, "--metadata-json-file", metadataPath)
		}

		if config.ScriptFails {
			args = append(args, "--script-invalid")
		}

		return RunCommand(cliBinary, args, nil)
	}

	if err := build(0, 0); err != nil {
		return nil, err
	}

	minFee, err := scriptTxMinFee(
		cliBinary, txFilePath, ppFilePath, len(utxos), len(outputs)+1, len(wallets), ec.config.NetworkMagic)
	if err != nil {
		return nil, err
	}

	var pp struct {
		CollateralPercentage uint64 `json:"collateralPercentage"`
	}

	if err := json.Unmarshal(protocolParams, &pp); err != nil {
		return nil, fmt.Errorf("failed to read collateral percentage: %w", err)
	}

	fee := minFee + scriptTxFeeMargin
	totalCollateral := (fee*pp.CollateralPercentage + 99) / 100

	if totalCollateral >= collateral.Amount {
		return nil, fmt.Errorf("collateral utxo %s#%d holds %d, less than the %d needed",
			collateral.Hash, collateral.Index, collateral.Amount, totalCollateral)
	}

	if err := build(fee, totalCollateral); err != nil {
		return nil, err
	}

	txRaw, err := readTxEnvelope(txFilePath)
	if err != nil {
		return nil, err
	}

	if config.MutateRawTx != nil {
		if txRaw, err = config.MutateRawTx(txRaw); err != nil {
			return nil, fmt.Errorf("failed to mutate raw script tx: %w", err)
		}
	}

	txHash, err := TxHashFromRawTx(txRaw)
	if err != nil {
		return nil, err
	}

	info := &ScriptTxInfo{
		TxHash:               txHash,
		OutputsCount:         uint32(len(outputs) + 1),
		TTL:                  ttl,
		CollateralAmount:     collateral.Amount,
		TotalCollateral:      totalCollateral,
		CollateralReturnAddr: returnAddr,
	}

	// only the outputs on the side of phase-2 the tx is meant to land on will ever show up
	waitAddr := receivers[0].Addr
	if config.ScriptFails {
		waitAddr = returnAddr
	}

	if _, err := ec.submitTx(ctx, txRaw, txHash, waitAddr, wallets); err != nil {
		return nil, err
	}

	if err := ec.checkScriptTxOutcome(ctx, txProvider, info, receivers[0].Addr, config.ScriptFails); err != nil {
		return nil, err
	}

	return info, nil
}

// checkScriptTxOutcome makes sure the ledger settled the tx the way it was meant to, so
// a test never mistakes a tx that went through for one whose script failed.
func (ec *TestCardanoChain) checkScriptTxOutcome(
	ctx context.Context, txProvider infrawallet.ITxProvider, info *ScriptTxInfo, firstReceiverAddr string,
	scriptFails bool,
) error {
	findOutput := func(addr string) (*infrawallet.Utxo, error) {
		utxos, err := infracommon.ExecuteWithRetry(ctx, func(ctx context.Context) ([]infrawallet.Utxo, error) {
			return txProvider.GetUtxos(ctx, addr)
		})
		if err != nil {
			return nil, err
		}

		for _, utxo := range utxos {
			if utxo.Hash == info.TxHash {
				return &utxo, nil
			}
		}

		return nil, nil
	}

	receiverOutput, err := findOutput(firstReceiverAddr)
	if err != nil {
		return err
	}

	if !scriptFails {
		if receiverOutput == nil || receiverOutput.Index != 0 {
			return fmt.Errorf("script tx %s is on chain but its first output is not", info.TxHash)
		}

		return nil
	}

	returnOutput, err := findOutput(info.CollateralReturnAddr)
	if err != nil {
		return err
	}

	switch {
	case returnOutput == nil || returnOutput.Index != info.OutputsCount:
		return fmt.Errorf("script tx %s is on chain but not as a failed one: no collateral return at index %d",
			info.TxHash, info.OutputsCount)
	case firstReceiverAddr != info.CollateralReturnAddr && receiverOutput != nil:
		return fmt.Errorf("script tx %s failed, yet its first output exists", info.TxHash)
	}

	return nil
}

// plutusV2PolicyID is the hash a PlutusV2 script is known by: blake2b-224 over the
// language tag followed by the script, cbor-wrapped once.
func plutusV2PolicyID(cborHex string) (string, error) {
	envelope, err := hex.DecodeString(cborHex)
	if err != nil {
		return "", err
	}

	var script []byte
	if err := cbor.Unmarshal(envelope, &script); err != nil {
		return "", fmt.Errorf("failed to unwrap plutus script: %w", err)
	}

	hasher, err := blake2b.New(policyIDSize, nil)
	if err != nil {
		return "", err
	}

	hasher.Write([]byte{plutusV2LanguageTag})
	hasher.Write(script)

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// pickCollateralUtxo takes the smallest pure ada utxo that can carry the collateral, so
// whoever the collateral return goes to, the sender risks as little as possible.
func pickCollateralUtxo(utxos []infrawallet.Utxo) (infrawallet.Utxo, error) {
	var (
		best  infrawallet.Utxo
		found bool
	)

	for _, utxo := range utxos {
		if len(utxo.Tokens) > 0 || utxo.Amount < scriptTxMinCollateralUtxo {
			continue
		}

		if !found || utxo.Amount < best.Amount {
			best, found = utxo, true
		}
	}

	if !found {
		return best, fmt.Errorf("no pure ada utxo of at least %d lovelace to use as collateral",
			scriptTxMinCollateralUtxo)
	}

	return best, nil
}

// scriptTxChange is what is left of utxos once outputs and fee are paid, minted token added.
func scriptTxChange(
	senderAddr string, utxos []infrawallet.Utxo, outputs []infrawallet.TxOutput,
	fee uint64, minted infrawallet.TokenAmount,
) (*infrawallet.TxOutput, error) {
	inputsSum := infrawallet.GetUtxosSum(utxos)
	tokens := map[string]infrawallet.TokenAmount{}

	for _, utxo := range utxos {
		for _, token := range utxo.Tokens {
			tokens[token.TokenName()] = infrawallet.NewTokenAmount(token.Token, inputsSum[token.TokenName()])
		}
	}

	spent := map[string]uint64{infrawallet.AdaTokenName: fee}

	for _, output := range outputs {
		spent[infrawallet.AdaTokenName] += output.Amount

		for _, token := range output.Tokens {
			spent[token.TokenName()] += token.Amount
		}
	}

	for name, amount := range spent {
		if inputsSum[name] < amount {
			return nil, fmt.Errorf("insufficient %s: have %d, need %d", name, inputsSum[name], amount)
		}
	}

	changeTokens := []infrawallet.TokenAmount{minted}

	for name, token := range tokens {
		if left := token.Amount - spent[name]; left > 0 {
			changeTokens = append(changeTokens, infrawallet.NewTokenAmount(token.Token, left))
		}
	}

	change := infrawallet.NewTxOutput(
		senderAddr, inputsSum[infrawallet.AdaTokenName]-spent[infrawallet.AdaTokenName], changeTokens...)

	return &change, nil
}

func scriptTxMinFee(
	cliBinary, txFilePath, ppFilePath string, inputsCount, outputsCount, witnessCount int, networkMagic uint,
) (uint64, error) {
	var out bytes.Buffer

	args := append([]string{
		infrawallet.DefaultEra, "transaction", "calculate-min-fee",
		"--tx-body-file", txFilePath,
		"--tx-in-count", strconv.Itoa(inputsCount),
		"--tx-out-count", strconv.Itoa(outputsCount),
		"--witness-count", strconv.Itoa(witnessCount),
		"--protocol-params-file", ppFilePath,
	}, GetTestNetMagicArgs(networkMagic)...)

	if err := RunCommand(cliBinary, args, &out); err != nil {
		return 0, err
	}

	// newer clis print json, older ones "<fee> Lovelace"
	var asJSON struct {
		Fee uint64 `json:"fee"`
	}

	if err := json.Unmarshal(out.Bytes(), &asJSON); err == nil {
		return asJSON.Fee, nil
	}

	fields := strings.Fields(out.String())
	if len(fields) == 0 {
		return 0, fmt.Errorf("empty calculate-min-fee output")
	}

	return strconv.ParseUint(fields[0], 10, 64)
}

func readTxEnvelope(path string) ([]byte, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var envelope struct {
		CborHex string `json:"cborHex"`
	}

	if err := json.Unmarshal(content, &envelope); err != nil {
		return nil, fmt.Errorf("failed to read tx envelope %s: %w", path, err)
	}

	return hex.DecodeString(envelope.CborHex)
}
