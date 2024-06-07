package types

import (
	"errors"
	"fmt"
	"math/big"
	"strings"
	"unicode"

	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/helper/keccak"
	"github.com/Ethernal-Tech/merkle-tree"
)

const (
	HashLength    = 32
	AddressLength = 20

	SignatureSize = 4
)

var (
	// ZeroAddress is the default zero address
	ZeroAddress = Address{}

	// ZeroHash is the default zero hash
	ZeroHash = Hash{}

	// ZeroNonce is the default empty nonce
	ZeroNonce = Nonce{}

	// EmptyRootHash is the root when there are no transactions
	EmptyRootHash = StringToHash("0x56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421")

	// EmptyUncleHash is the root when there are no uncles
	EmptyUncleHash = StringToHash("0x1dcc4de8dec75d7aab85b567b6ccd41ad312451b948a7413f0a142fd40d49347")

	// EmptyCodeHash is the root where there is no code.
	// Equivalent of: `types.BytesToHash(crypto.Keccak256(nil))`
	EmptyCodeHash = StringToHash("0xc5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a470")

	// ErrTxTypeNotSupported denotes that transaction is not supported
	ErrTxTypeNotSupported = errors.New("transaction type not supported")

	// ErrInsufficientFunds denotes that account has insufficient funds for transaction execution
	ErrInsufficientFunds = errors.New("insufficient funds for execution")
)

type Hash [HashLength]byte

type Address [AddressLength]byte

func min(i, j int) int {
	if i < j {
		return i
	}

	return j
}

func BytesToHash(b []byte) Hash {
	var h Hash

	size := len(b)
	min := min(size, HashLength)

	copy(h[HashLength-min:], b[len(b)-min:])

	return h
}

func (h Hash) Bytes() []byte {
	return h[:]
}

func (h Hash) String() string {
	return hex.EncodeToHex(h[:])
}

// checksumEncode returns the checksummed address with 0x prefix, as by EIP-55
// https://github.com/ethereum/EIPs/blob/master/EIPS/eip-55.md
func (a Address) checksumEncode() string {
	addrBytes := a.Bytes() // 20 bytes

	// Encode to hex without the 0x prefix
	lowercaseHex := hex.EncodeToHex(addrBytes)[2:]
	hashedAddress := hex.EncodeToHex(keccak.Keccak256(nil, []byte(lowercaseHex)))[2:]

	result := make([]rune, len(lowercaseHex))
	// Iterate over each character in the lowercase hex address
	for idx, ch := range lowercaseHex {
		if ch >= '0' && ch <= '9' || hashedAddress[idx] >= '0' && hashedAddress[idx] <= '7' {
			// Numbers in range [0, 9] are ignored (as well as hashed values [0, 7]),
			// because they can't be uppercased
			result[idx] = ch
		} else {
			// The current character / hashed character is in the range [8, f]
			result[idx] = unicode.ToUpper(ch)
		}
	}

	return "0x" + string(result)
}

func (a Address) Ptr() *Address {
	return &a
}

func (a Address) String() string {
	return a.checksumEncode()
}

func (a Address) Bytes() []byte {
	return a[:]
}

func StringToHash(str string) Hash {
	return BytesToHash(StringToBytes(str))
}

func StringToAddress(str string) Address {
	return BytesToAddress(StringToBytes(str))
}

func AddressToString(address Address) string {
	return string(address[:])
}

func BytesToAddress(b []byte) Address {
	var a Address

	size := len(b)
	min := min(size, AddressLength)

	copy(a[AddressLength-min:], b[len(b)-min:])

	return a
}

func StringToBytes(str string) []byte {
	str = strings.TrimPrefix(str, "0x")
	if len(str)%2 == 1 {
		str = "0" + str
	}

	b, _ := hex.DecodeString(str)

	return b
}

// FromTypesToMerkleHash fills array of merkle.Hash from array types.Hash
func FromTypesToMerkleHash(hashes []Hash) []merkle.Hash {
	merkleHashes := make([]merkle.Hash, 0, len(hashes))
	for _, hash := range hashes {
		merkleHashes = append(merkleHashes, merkle.Hash(hash))
	}

	return merkleHashes
}

// FromMerkleToTypesHash fills array of types.Hash from array merkle.Hash
func FromMerkleToTypesHash(merkleHashes []merkle.Hash) []Hash {
	hashes := make([]Hash, 0, len(merkleHashes))
	for _, merkleHash := range merkleHashes {
		hashes = append(hashes, Hash(merkleHash))
	}

	return hashes
}

// IsValidAddress checks if provided string is a valid Ethereum address
func IsValidAddress(address string, zeroAddressAllowed bool) (Address, error) {
	// remove 0x prefix if it exists
	address = strings.TrimPrefix(address, "0x")

	// decode the address
	decodedAddress, err := hex.DecodeString(address)
	if err != nil {
		return ZeroAddress, fmt.Errorf("address %s contains invalid characters", address)
	}

	// check if the address has the correct length
	if len(decodedAddress) != AddressLength {
		return ZeroAddress, fmt.Errorf("address %s has invalid length", string(decodedAddress))
	}

	addr := StringToAddress(address)
	if !zeroAddressAllowed && addr == ZeroAddress {
		return ZeroAddress, errors.New("zero address is not allowed")
	}

	return addr, nil
}

// UnmarshalText parses a hash in hex syntax.
func (h *Hash) UnmarshalText(input []byte) error {
	*h = BytesToHash(StringToBytes(string(input)))

	return nil
}

// UnmarshalText parses an address in hex syntax.
func (a *Address) UnmarshalText(input []byte) error {
	buf := StringToBytes(string(input))
	if len(buf) != AddressLength {
		return fmt.Errorf("incorrect length")
	}

	*a = BytesToAddress(buf)

	return nil
}

func (h Hash) MarshalText() ([]byte, error) {
	return []byte(h.String()), nil
}

func (a Address) MarshalText() ([]byte, error) {
	return []byte(a.String()), nil
}

type Proof struct {
	Data     []Hash // the proof himself
	Metadata map[string]interface{}
}

type OverrideAccount struct {
	Nonce     *uint64
	Code      []byte
	Balance   *big.Int
	State     map[Hash]Hash
	StateDiff map[Hash]Hash
}

type StateOverride map[Address]OverrideAccount

// MixedcaseAddress retains the original string, which may or may not be
// correctly checksummed
type MixedcaseAddress struct {
	addr     Address
	original string
}

// NewMixedcaseAddress constructor (mainly for testing)
func NewMixedcaseAddress(addr Address) *MixedcaseAddress {
	return &MixedcaseAddress{addr: addr, original: addr.String()}
}

// NewMixedcaseAddressFromString is mainly meant for unit-testing
func NewMixedcaseAddressFromString(hexaddr string) (*MixedcaseAddress, error) {
	var addr Address
	addr, err := IsValidAddress(hexaddr, false)
	if err != nil {
		return nil, errors.New("invalid address")
	}

	return &MixedcaseAddress{addr: addr, original: addr.String()}, nil
}

// Address returns the address
func (ma *MixedcaseAddress) Address() Address {
	return ma.addr
}

// String implements fmt.Stringer
func (ma *MixedcaseAddress) String() string {
	if ma.ValidChecksum() {
		return fmt.Sprintf("%s [chksum ok]", ma.original)
	}

	return fmt.Sprintf("%s [chksum INVALID]", ma.original)
}

// ValidChecksum returns true if the address has valid checksum
func (ma *MixedcaseAddress) ValidChecksum() bool {
	return ma.original == ma.addr.String()
}

// Original returns the mixed-case input string
func (ma *MixedcaseAddress) Original() string {
	return ma.original
}
