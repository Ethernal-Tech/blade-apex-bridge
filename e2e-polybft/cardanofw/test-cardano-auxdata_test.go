package cardanofw

import (
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/fxamacker/cbor/v2"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/blake2b"
)

// A babbage transaction built by cardano-cli 8.17 carrying bridging metadata under
// label 1 and a nested metadatum under label 2. It is a fixture rather than something
// built here so the envelope under test is the one cardano-cli really emits: the
// auxiliary_data starts d9 01 03, the alonzo #6.259 tag, which is the only envelope
// cardano-cli can produce.
const (
	cliBuiltTxHex = "84a50081825820000000000000000000000000000000000000000000000000000000000000000000018" +
		"182581d60e24badd574bcb61c71dc1dd843b68e00abed782c7de6a0b9b452bf991a000f4240021a00030d40031a0001" +
		"86a00758203694bb55fa3c573d686187a7d9659cb8e68b407a13bd44bc49d3683d83eed9c6a0f5d90103a100a201a66" +
		"164656e657875736266611a000f4240626f6600617381646164647261746662726964676562747881a36161816178616" +
		"d1864617400028181818181818181818181818181818181818181818181818101"

	// as reported by `cardano-cli transaction txid` for the fixture above
	cliBuiltTxHash = "4ef9237e712a34a18a4b823b71c32454d366bda4482fc99fbbd1a856037c8239"

	// as reported by `cardano-cli transaction txid` after the shelley-ma rewrite
	shelleyMARewrittenTxHash = "9a34e236309bf4d0748911b708858d1efe62e5d05460f7428a8b314c100faab8"
)

func cliBuiltTx(t *testing.T) []byte {
	t.Helper()

	txRaw, err := hex.DecodeString(cliBuiltTxHex)
	require.NoError(t, err)

	return txRaw
}

// auxDataHashInBody returns what the body commits auxiliary_data to be.
func auxDataHashInBody(t *testing.T, txRaw []byte) []byte {
	t.Helper()

	var tx []cbor.RawMessage

	require.NoError(t, auxDataUnmarshal(txRaw, &tx))

	var body map[uint64]cbor.RawMessage

	require.NoError(t, auxDataUnmarshal(tx[txBodyIndex], &body))

	var hash []byte

	require.NoError(t, auxDataUnmarshal(body[bodyAuxDataHashKey], &hash))

	return hash
}

// metadataMapOf resolves the { label => metadatum } map out of any of the three
// envelopes, the way the oracle has to.
func metadataMapOf(t *testing.T, auxData []byte) map[uint64]cbor.RawMessage {
	t.Helper()

	metadataMap := cbor.RawMessage(auxData)

	var tagged cbor.RawTag
	if err := auxDataUnmarshal(auxData, &tagged); err == nil && tagged.Number == AlonzoAuxiliaryDataTag {
		var fields map[uint64]cbor.RawMessage

		require.NoError(t, auxDataUnmarshal(tagged.Content, &fields))

		metadataMap = fields[auxDataMetadataKey]
	} else {
		var asArray []cbor.RawMessage
		if err := auxDataUnmarshal(auxData, &asArray); err == nil {
			metadataMap = asArray[0]
		}
	}

	var labels map[uint64]cbor.RawMessage

	require.NoError(t, auxDataUnmarshal(metadataMap, &labels))

	return labels
}

func TestTxHashFromRawTx(t *testing.T) {
	// the builder reports a hash, but after a rewrite the body is ours, so the hash has
	// to be recomputed the way the ledger does it
	hash, err := TxHashFromRawTx(cliBuiltTx(t))
	require.NoError(t, err)
	require.Equal(t, cliBuiltTxHash, hash)
}

func TestAuxiliaryDataEnvelopeName(t *testing.T) {
	auxData, err := TxAuxiliaryData(cliBuiltTx(t))
	require.NoError(t, err)

	require.Equal(t, "alonzo", AuxiliaryDataEnvelopeName(auxData))
	require.Equal(t, byte(0xd9), auxData[0], "cardano-cli must still emit the tagged envelope")
}

func TestRewriteTxAuxiliaryDataToShelleyMA(t *testing.T) {
	txRaw := cliBuiltTx(t)

	before := metadataMapOf(t, mustTxAuxData(t, txRaw))

	rewritten, err := RewriteTxAuxiliaryDataToShelleyMA(txRaw, nil)
	require.NoError(t, err)

	auxData := mustTxAuxData(t, rewritten)
	require.Equal(t, "shelley-ma", AuxiliaryDataEnvelopeName(auxData))

	// [ metadata, [] ] - metadata first, then an empty native script list
	var asArray []cbor.RawMessage

	require.NoError(t, auxDataUnmarshal(auxData, &asArray))
	require.Len(t, asArray, 2)

	var scripts []cbor.RawMessage

	require.NoError(t, auxDataUnmarshal(asArray[1], &scripts))
	require.Empty(t, scripts)

	// the envelope changed but not a byte of what it carries
	require.Equal(t, before, metadataMapOf(t, auxData))

	// the body has to commit to the new auxiliary_data, or the node rejects the tx
	hash := blake2b.Sum256(auxData)
	require.Equal(t, hash[:], auxDataHashInBody(t, rewritten))

	// and the transaction is now a different transaction
	txHash, err := TxHashFromRawTx(rewritten)
	require.NoError(t, err)
	require.Equal(t, shelleyMARewrittenTxHash, txHash)
	require.NotEqual(t, cliBuiltTxHash, txHash)
}

func TestRewriteTxAuxiliaryDataScripts(t *testing.T) {
	txRaw := cliBuiltTx(t)

	before := metadataMapOf(t, mustTxAuxData(t, txRaw))

	script, err := NestedNativeScript(40, make([]byte, 28))
	require.NoError(t, err)

	rewritten, err := RewriteTxAuxiliaryDataScripts(txRaw, []cbor.RawMessage{script})
	require.NoError(t, err)

	auxData := mustTxAuxData(t, rewritten)
	require.Equal(t, "alonzo", AuxiliaryDataEnvelopeName(auxData))
	require.Equal(t, before, metadataMapOf(t, auxData))

	var tagged cbor.RawTag

	require.NoError(t, auxDataUnmarshal(auxData, &tagged))

	var fields map[uint64]cbor.RawMessage

	require.NoError(t, auxDataUnmarshal(tagged.Content, &fields))
	require.Contains(t, fields, uint64(auxDataScriptsKey))

	hash := blake2b.Sum256(auxData)
	require.Equal(t, hash[:], auxDataHashInBody(t, rewritten))
}

func TestNestedNativeScript(t *testing.T) {
	script, err := NestedNativeScript(3, make([]byte, 28))
	require.NoError(t, err)

	// unwrap the three `all` levels and land on the signature script
	current := script

	for range 3 {
		var level []cbor.RawMessage

		require.NoError(t, auxDataUnmarshal(current, &level))
		require.Len(t, level, 2)

		var kind uint64

		require.NoError(t, auxDataUnmarshal(level[0], &kind))
		require.EqualValues(t, 1, kind)

		var inner []cbor.RawMessage

		require.NoError(t, auxDataUnmarshal(level[1], &inner))
		require.Len(t, inner, 1)

		current = inner[0]
	}

	var leaf []cbor.RawMessage

	require.NoError(t, auxDataUnmarshal(current, &leaf))

	var kind uint64

	require.NoError(t, auxDataUnmarshal(leaf[0], &kind))
	require.EqualValues(t, 0, kind)
}

func TestAddNestedMetadatumLabel(t *testing.T) {
	metadata := []byte(`{"1":{"t":"bridge","d":"nexus"}}`)

	withNested, err := AddNestedMetadatumLabel(metadata, 2, 40)
	require.NoError(t, err)

	var labels map[string]json.RawMessage

	require.NoError(t, json.Unmarshal(withNested, &labels))
	require.Contains(t, labels, "1")
	require.Contains(t, labels, "2")

	// the bridging request is handed through untouched
	require.JSONEq(t, `{"t":"bridge","d":"nexus"}`, string(labels["1"]))

	// and label 2 really is 40 levels deep
	depth, value := 0, labels["2"]

	for {
		var nested []json.RawMessage
		if err := json.Unmarshal(value, &nested); err != nil {
			break
		}

		require.Len(t, nested, 1)

		depth++
		value = nested[0]
	}

	require.Equal(t, 40, depth)

	_, err = AddNestedMetadatumLabel(withNested, 2, 10)
	require.Error(t, err, "must refuse to clobber a label that is already there")
}

func mustTxAuxData(t *testing.T, txRaw []byte) []byte {
	t.Helper()

	auxData, err := TxAuxiliaryData(txRaw)
	require.NoError(t, err)

	return auxData
}
