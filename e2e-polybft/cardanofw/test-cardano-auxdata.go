package cardanofw

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/fxamacker/cbor/v2"
	"golang.org/x/crypto/blake2b"
)

// AlonzoAuxiliaryDataTag is the cbor tag wrapping auxiliary_data from alonzo onwards.
const AlonzoAuxiliaryDataTag = 259

// Babbage accepts three auxiliary_data encodings, and the transaction builder is the
// one that picks - none of them means the transaction is malformed:
//
//	metadata                                                        ; shelley
//	[ metadata, [* native_script] ]                                 ; shelley-ma
//	#6.259({ ?0: metadata, ?1: [* native_script], ?2: .., ?3: .. }) ; alonzo and later
//
// where metadata is the { label => metadatum } map. cardano-cli only ever emits the
// alonzo one, so a transaction built through it cannot exercise the others, yet the
// oracle has to read all three the same way - prime mainnet has carried at least one
// bridging request built outside our tooling with the shelley envelope.
//
// The helpers below rewrite the auxiliary_data of an already built transaction. That
// works because witnesses are created from the raw transaction afterwards, so as long
// as auxiliary_data_hash in the body is fixed up to match, the result is a transaction
// the node accepts and signs normally.

// txAuxDataIndex is the position of auxiliary_data in a serialized transaction,
// which is [ body, witness_set, is_valid, auxiliary_data ].
const (
	txElementCount     = 4
	txBodyIndex        = 0
	txAuxDataIndex     = 3
	bodyAuxDataHashKey = 7
	auxDataMetadataKey = 0
	auxDataScriptsKey  = 1
)

func auxDataEncMode() (cbor.EncMode, error) {
	// cardano-cli emits canonical cbor, and the body has to be re-encoded the same way
	// it came in so that only auxiliary_data_hash changes
	return cbor.CanonicalEncOptions().EncMode()
}

// auxDataDecMode raises the nesting limit past fxamacker's default of 32. These helpers
// deliberately build transactions carrying native scripts deeper than that, and the
// nesting counter is per message, so the default would make the helpers choke on their
// own output long before the code under test ever saw it.
var auxDataDecMode = sync.OnceValues(func() (cbor.DecMode, error) {
	return cbor.DecOptions{MaxNestedLevels: 65535}.DecMode()
})

// auxDataUnmarshal decodes cbor with the nesting limit above.
func auxDataUnmarshal(data []byte, v interface{}) error {
	decMode, err := auxDataDecMode()
	if err != nil {
		return err
	}

	return decMode.Unmarshal(data, v)
}

// AuxiliaryDataEnvelopeName names which of the encodings babbage accepts an
// auxiliary_data blob uses, so a test can assert on the envelope it actually produced
// rather than assume one.
func AuxiliaryDataEnvelopeName(auxData []byte) string {
	var asTag cbor.RawTag
	if err := auxDataUnmarshal(auxData, &asTag); err == nil && asTag.Number == AlonzoAuxiliaryDataTag {
		return "alonzo"
	}

	var asArray []cbor.RawMessage
	if err := auxDataUnmarshal(auxData, &asArray); err == nil {
		return "shelley-ma"
	}

	return "shelley"
}

// TxAuxiliaryData returns the raw auxiliary_data of a built transaction.
func TxAuxiliaryData(txRaw []byte) ([]byte, error) {
	var tx []cbor.RawMessage
	if err := auxDataUnmarshal(txRaw, &tx); err != nil {
		return nil, fmt.Errorf("failed to unmarshal tx: %w", err)
	}

	if len(tx) != txElementCount {
		return nil, fmt.Errorf("expected %d tx elements, got %d", txElementCount, len(tx))
	}

	return tx[txAuxDataIndex], nil
}

// TxHashFromRawTx recomputes a transaction's hash, which is blake2b-256 of its body.
// Rewriting auxiliary_data changes the body, so the hash the builder reported no
// longer applies.
func TxHashFromRawTx(txRaw []byte) (string, error) {
	var tx []cbor.RawMessage
	if err := auxDataUnmarshal(txRaw, &tx); err != nil {
		return "", fmt.Errorf("failed to unmarshal tx: %w", err)
	}

	if len(tx) != txElementCount {
		return "", fmt.Errorf("expected %d tx elements, got %d", txElementCount, len(tx))
	}

	hash := blake2b.Sum256(tx[txBodyIndex])

	return hex.EncodeToString(hash[:]), nil
}

// RewriteTxAuxiliaryDataToShelleyMA re-wraps a built transaction's auxiliary_data from
// the alonzo envelope cardano-cli emits into the shelley-ma [ metadata, [scripts] ]
// form, leaving the metadata map itself untouched, and fixes up auxiliary_data_hash.
// Passing no scripts yields an empty script list, which is what a transaction that only
// carries metadata looks like in this envelope.
func RewriteTxAuxiliaryDataToShelleyMA(txRaw []byte, scripts []cbor.RawMessage) ([]byte, error) {
	encMode, err := auxDataEncMode()
	if err != nil {
		return nil, err
	}

	var tx []cbor.RawMessage
	if err := auxDataUnmarshal(txRaw, &tx); err != nil {
		return nil, fmt.Errorf("failed to unmarshal tx: %w", err)
	}

	if len(tx) != txElementCount {
		return nil, fmt.Errorf("expected %d tx elements, got %d", txElementCount, len(tx))
	}

	var tagged cbor.RawTag
	if err := auxDataUnmarshal(tx[txAuxDataIndex], &tagged); err != nil {
		return nil, fmt.Errorf("failed to unmarshal auxiliary_data: %w", err)
	}

	if tagged.Number != AlonzoAuxiliaryDataTag {
		return nil, fmt.Errorf("unexpected auxiliary_data tag: %d", tagged.Number)
	}

	var fields map[uint64]cbor.RawMessage
	if err := auxDataUnmarshal(tagged.Content, &fields); err != nil {
		return nil, fmt.Errorf("failed to unmarshal auxiliary_data fields: %w", err)
	}

	metadataMap, exists := fields[auxDataMetadataKey]
	if !exists {
		return nil, fmt.Errorf("auxiliary_data carries no metadata")
	}

	if scripts == nil {
		scripts = []cbor.RawMessage{}
	}

	scriptsRaw, err := encMode.Marshal(scripts)
	if err != nil {
		return nil, err
	}

	auxData, err := encMode.Marshal([]cbor.RawMessage{metadataMap, scriptsRaw})
	if err != nil {
		return nil, err
	}

	return replaceTxAuxiliaryData(encMode, tx, auxData)
}

// RewriteTxAuxiliaryDataScripts puts native scripts into a built transaction's alonzo
// auxiliary_data, keeping the envelope, and fixes up auxiliary_data_hash. cardano-cli
// has no flag for attaching auxiliary scripts to a metadata-only transaction.
func RewriteTxAuxiliaryDataScripts(txRaw []byte, scripts []cbor.RawMessage) ([]byte, error) {
	encMode, err := auxDataEncMode()
	if err != nil {
		return nil, err
	}

	var tx []cbor.RawMessage
	if err := auxDataUnmarshal(txRaw, &tx); err != nil {
		return nil, fmt.Errorf("failed to unmarshal tx: %w", err)
	}

	if len(tx) != txElementCount {
		return nil, fmt.Errorf("expected %d tx elements, got %d", txElementCount, len(tx))
	}

	var tagged cbor.RawTag
	if err := auxDataUnmarshal(tx[txAuxDataIndex], &tagged); err != nil {
		return nil, fmt.Errorf("failed to unmarshal auxiliary_data: %w", err)
	}

	if tagged.Number != AlonzoAuxiliaryDataTag {
		return nil, fmt.Errorf("unexpected auxiliary_data tag: %d", tagged.Number)
	}

	var fields map[uint64]cbor.RawMessage
	if err := auxDataUnmarshal(tagged.Content, &fields); err != nil {
		return nil, fmt.Errorf("failed to unmarshal auxiliary_data fields: %w", err)
	}

	if fields[auxDataScriptsKey], err = encMode.Marshal(scripts); err != nil {
		return nil, err
	}

	content, err := encMode.Marshal(fields)
	if err != nil {
		return nil, err
	}

	auxData, err := encMode.Marshal(cbor.RawTag{
		Number:  AlonzoAuxiliaryDataTag,
		Content: content,
	})
	if err != nil {
		return nil, err
	}

	return replaceTxAuxiliaryData(encMode, tx, auxData)
}

// replaceTxAuxiliaryData swaps in new auxiliary_data and updates auxiliary_data_hash so
// the body still commits to what the transaction carries.
func replaceTxAuxiliaryData(encMode cbor.EncMode, tx []cbor.RawMessage, auxData []byte) ([]byte, error) {
	var body map[uint64]cbor.RawMessage
	if err := auxDataUnmarshal(tx[txBodyIndex], &body); err != nil {
		return nil, fmt.Errorf("failed to unmarshal tx body: %w", err)
	}

	hash := blake2b.Sum256(auxData)

	var err error
	if body[bodyAuxDataHashKey], err = encMode.Marshal(hash[:]); err != nil {
		return nil, err
	}

	if tx[txBodyIndex], err = encMode.Marshal(body); err != nil {
		return nil, err
	}

	tx[txAuxDataIndex] = auxData

	return encMode.Marshal(tx)
}

// NestedNativeScript builds an `all` native script nested the given number of levels
// deep around a single signature script. native_script is recursive and cardano-ledger
// puts no depth counter on it, so the only bound is transaction size - a decoder with a
// nesting cap rejects scripts the node itself accepts.
func NestedNativeScript(depth int, keyHash []byte) (cbor.RawMessage, error) {
	const (
		scriptPubkey = 0
		scriptAll    = 1
	)

	encMode, err := auxDataEncMode()
	if err != nil {
		return nil, err
	}

	script, err := encMode.Marshal([]interface{}{scriptPubkey, keyHash})
	if err != nil {
		return nil, err
	}

	for range depth {
		if script, err = encMode.Marshal([]interface{}{
			scriptAll, []cbor.RawMessage{script},
		}); err != nil {
			return nil, err
		}
	}

	return script, nil
}

// AddNestedMetadatumLabel attaches an unrelated metadata label holding a metadatum
// nested the given number of levels deep to already built metadata json. The bridging
// request under label 1 is left alone: the point is that a neighbouring label cannot
// decide whether it is readable.
func AddNestedMetadatumLabel(metadataJSON []byte, label uint64, depth int) ([]byte, error) {
	var labels map[string]json.RawMessage
	if err := json.Unmarshal(metadataJSON, &labels); err != nil {
		return nil, fmt.Errorf("failed to unmarshal metadata json: %w", err)
	}

	if _, taken := labels[fmt.Sprint(label)]; taken {
		return nil, fmt.Errorf("metadata label %d is already in use", label)
	}

	nested := make([]byte, 0, 2*depth+1)
	for range depth {
		nested = append(nested, '[')
	}

	nested = append(nested, '1')

	for range depth {
		nested = append(nested, ']')
	}

	labels[fmt.Sprint(label)] = nested

	return json.Marshal(labels)
}
