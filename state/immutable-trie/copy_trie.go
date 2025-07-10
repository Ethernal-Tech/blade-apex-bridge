package itrie

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/cockroachdb/pebble"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/umbracle/fastrlp"
)

func onNode(batchWriter Batch) NodeWalkedCallback {
	return func(nodeHash []byte, _ Node, data []byte, _ *AccountWithHash) error {
		// copy whole bytes of nodes
		batchWriter.Put(nodeHash, data)

		return nil
	}
}

func onAccount(batchWriter Batch, storage Storage) AccountWalkedCallback {
	return func(_ *ValueNode, account *AccountWithHash) error {
		if account.CodeHash != nil && bytes.Equal(account.CodeHash, EmptyCodeHash) == false {
			hash := types.BytesToHash(account.CodeHash)

			code, ok := storage.GetCode(hash)
			if ok {
				batchWriter.Put(GetCodeKey(hash), code)
			} else {
				return fmt.Errorf("can't find code %s", hex.EncodeToString(account.CodeHash))
			}
		}

		return nil
	}
}

func CopyTrie(nodeHash []byte, storage Storage, newStorage Storage, agg []byte, isStorage bool) error {
	batchWriter := newStorage.Batch()

	err := walkTrieHash(
		nodeHash, storage, agg, isStorage, nil,
		onNode(batchWriter), onAccount(batchWriter, storage))

	if err != nil {
		return nil
	}

	return batchWriter.Write()
}

func HashChecker(stateRoot []byte, storage Storage) (types.Hash, error) {
	node, _, err := GetNode(stateRoot, storage)
	if err != nil {
		return types.Hash{}, err
	}

	h, ok := hasherPool.Get().(*hasher)
	if !ok {
		return types.Hash{}, errors.New("can't get hasher")
	}

	arena, _ := h.AcquireArena()

	val, err := hashChecker(node, h, arena, 0, storage)
	if err != nil {
		return types.Hash{}, err
	}

	if val == nil {
		return emptyStateHash, nil
	}

	h.ReleaseArenas(0)
	hasherPool.Put(h)

	return types.BytesToHash(val.Raw()), nil
}

func hashChecker(node Node, h *hasher, a *fastrlp.Arena, d int, storage Storage) (*fastrlp.Value, error) {
	var (
		val *fastrlp.Value
		aa  *fastrlp.Arena
		idx int
	)

	switch n := node.(type) {
	case nil:
		return nil, nil
	case *ValueNode:
		if n.hash {
			nd, _, err := GetNode(n.buf, storage)
			if err != nil {
				return nil, err
			}

			return hashChecker(nd, h, a, d, storage)
		}

		return a.NewCopyBytes(n.buf), nil

	case *ShortNode:
		child, err := hashChecker(n.child, h, a, d+1, storage)
		if err != nil {
			return nil, err
		}

		val = a.NewArray()
		val.Set(a.NewBytes(encodeCompact(n.key)))
		val.Set(child)

	case *FullNode:
		val = a.NewArray()

		aa, idx = h.AcquireArena()

		for _, i := range n.children {
			if i == nil {
				val.Set(a.NewNull())
			} else {
				v, err := hashChecker(i, h, aa, d+1, storage)
				if err != nil {
					return nil, err
				}

				val.Set(v)
			}
		}

		// Add the value
		if n.value == nil {
			val.Set(a.NewNull())
		} else {
			v, err := hashChecker(n.value, h, a, d+1, storage)
			if err != nil {
				return nil, err
			}

			val.Set(v)
		}

	default:
		return nil, fmt.Errorf("unknown node type %T", node)
	}

	if val.Len() < 32 {
		return val, nil
	}

	// marshal RLP value
	h.buf = val.MarshalTo(h.buf[:0])

	if aa != nil {
		h.ReleaseArenas(idx)
	}

	tmp := h.Hash(h.buf)
	hh := node.SetHash(tmp)

	return a.NewCopyBytes(hh), nil
}

func NewKV(db *leveldb.DB) *KVStorage {
	return &KVStorage{db: db}
}

func NewPebble(db *pebble.DB) *pebbleStorage {
	return &pebbleStorage{db: db}
}

func NewTrieWithRoot(root Node) *Trie {
	return &Trie{
		root: root,
	}
}
