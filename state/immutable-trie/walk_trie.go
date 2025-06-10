package itrie

import (
	"bytes"
	"encoding/hex"
	"fmt"

	"github.com/0xPolygon/polygon-edge/crypto"

	"github.com/0xPolygon/polygon-edge/state"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/fastrlp"
)

type (
	AccountWithHash struct {
		state.Account
		Hash []byte
	}

	NodeWalkedCallback    func(nodeHash []byte, node Node, data []byte, account *AccountWithHash) error
	AccountWalkedCallback func(valueNode *ValueNode, account *AccountWithHash) error
)

var EmptyCodeHash = crypto.Keccak256(nil)

func GetAccount(storage Storage, rootHash []byte, addr types.Address) (*state.Account, bool, error) {
	key := crypto.Keccak256(addr.Bytes())

	data, err := Lookup(storage, rootHash, key)
	if err != nil {
		return nil, false, err
	}

	if len(data) == 0 {
		return nil, false, err
	}

	var account state.Account
	if err := account.UnmarshalRlp(data); err != nil {
		return nil, false, err
	}

	return &account, true, nil
}

func Lookup(storage Storage, rootHash []byte, key []byte) ([]byte, error) {
	rootNode, _, err := getCustomNode(rootHash, storage)
	if err != nil {
		return nil, err
	}

	_, res := lookup(storage, rootNode, bytesToHexNibbles(key))

	return res, nil
}

func lookup(storage Storage, node interface{}, key []byte) (Node, []byte) {
	switch n := node.(type) {
	case nil:
		return nil, nil

	case *ValueNode:
		if n.hash {
			nc, ok, err := GetNode(n.buf, storage)
			if err != nil {
				panic(err) //nolint:gocritic
			}

			if !ok {
				return nil, nil
			}

			_, res := lookup(storage, nc, key)

			return nc, res
		}

		if len(key) == 0 {
			return nil, n.buf
		} else {
			return nil, nil
		}

	case *ShortNode:
		plen := len(n.key)
		if plen > len(key) || !bytes.Equal(key[:plen], n.key) {
			return nil, nil
		}

		child, res := lookup(storage, n.child, key[plen:])

		if child != nil {
			n.child = child
		}

		return nil, res

	case *FullNode:
		if len(key) == 0 {
			return lookup(storage, n.value, key)
		}

		child, res := lookup(storage, n.getEdge(key[0]), key[1:])

		if child != nil {
			n.children[key[0]] = child
		}

		return nil, res

	default:
		panic(fmt.Sprintf("unknown node type %v", n)) //nolint:gocritic
	}
}

func WalkTrie(
	nodeHash []byte, storage Storage, agg []byte, isStorage bool, account *AccountWithHash,
	onNode NodeWalkedCallback, onAccount AccountWalkedCallback,
) error {
	return walkTrieHash(nodeHash, storage, agg, isStorage, account, onNode, onAccount)
}

func walkTrieHash(
	nodeHash []byte, storage Storage, agg []byte, isStorage bool, account *AccountWithHash,
	onNode NodeWalkedCallback, onAccount AccountWalkedCallback,
) error {
	node, data, err := getCustomNode(nodeHash, storage)
	if err != nil {
		return err
	}

	if err = onNode(nodeHash, node, data, account); err != nil {
		return err
	}

	return walkTrieNode(node, storage, agg, isStorage, account, onNode, onAccount)
}

func walkTrieNode(
	node Node, storage Storage, agg []byte, isStorage bool, account *AccountWithHash,
	onNode NodeWalkedCallback, onAccount AccountWalkedCallback,
) error {
	switch n := node.(type) {
	case nil:
		return nil
	case *FullNode:
		if len(n.hash) > 0 {
			return walkTrieHash(n.hash, storage, agg, isStorage, account, onNode, onAccount)
		}

		for i := range n.children {
			if n.children[i] == nil {
				continue
			}

			err := walkTrieNode(
				n.children[i], storage, append(agg, uint8(i)), isStorage, account, onNode, onAccount)
			if err != nil {
				return err
			}
		}

	case *ValueNode:
		// if node represens stored value, then we need to copy it
		if n.hash {
			return walkTrieHash(n.buf, storage, agg, isStorage, account, onNode, onAccount)
		}

		if !isStorage {
			var (
				account     state.Account
				accountHash = encodeCompact(agg)
			)

			if err := account.UnmarshalRlp(n.buf); err != nil {
				return fmt.Errorf("can't parse account %s: %w", hex.EncodeToString(accountHash), err)
			} else {
				accountWithHash := &AccountWithHash{Account: account, Hash: accountHash}

				if err := onAccount(n, accountWithHash); err != nil {
					return err
				}

				if account.Root != types.EmptyRootHash {
					return walkTrieHash(account.Root[:], storage, nil, true, accountWithHash, onNode, nil)
				}
			}
		}

	case *ShortNode:
		if len(n.hash) > 0 {
			return walkTrieHash(n.hash, storage, agg, isStorage, account, onNode, onAccount)
		}

		return walkTrieNode(n.child, storage, append(agg, n.key...), isStorage, account, onNode, onAccount)
	}

	return nil
}

func getCustomNode(hash []byte, storage Storage) (Node, []byte, error) {
	data, ok, err := storage.Get(hash)
	if err != nil || !ok {
		return nil, nil, err
	}

	// NOTE: We dont need to make copies of the bytes because the nodes
	// take the reference from data itself which is a safe copy.
	p := parserPool.Get()
	defer parserPool.Put(p)

	v, err := p.Parse(data)
	if err != nil {
		return nil, nil, err
	}

	if v.Type() != fastrlp.TypeArray {
		return nil, nil, fmt.Errorf("storage item should be an array")
	}

	n, err := decodeNode(v, storage)

	return n, data, err
}
