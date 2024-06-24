package cardanofw

import (
	"context"
	"encoding/json"
	"math/big"
	"os"
	"testing"
	"time"

	"github.com/Ethernal-Tech/cardano-infrastructure/wallet"
	"github.com/stretchr/testify/require"
)

type TestApexUser struct {
	basePath string

	primeNetworkMagic  uint
	vectorNetworkMagic uint
	PrimeWallet        wallet.IWallet
	VectorWallet       wallet.IWallet
	PrimeAddress       string
	VectorAddress      string
}

type BridgingRequestMetadataTransaction struct {
	Address []string `cbor:"a" json:"a"`
	Amount  uint64   `cbor:"m" json:"m"`
}

func NewTestApexUser(t *testing.T, primeNetworkMagic, vectorNetworkMagic uint) *TestApexUser {
	t.Helper()

	basePath, err := os.MkdirTemp("/tmp", "apex-user")
	require.NoError(t, err)

	primeWallet, err := wallet.GenerateWallet(false)
	require.NoError(t, err)

	vectorWallet, err := wallet.GenerateWallet(false)
	require.NoError(t, err)

	primeUserAddress, err := GetAddress(true, primeWallet)
	require.NoError(t, err)

	vectorUserAddress, err := GetAddress(false, vectorWallet)
	require.NoError(t, err)

	return &TestApexUser{
		basePath:           basePath,
		PrimeWallet:        primeWallet,
		VectorWallet:       vectorWallet,
		PrimeAddress:       primeUserAddress.String(),
		VectorAddress:      vectorUserAddress.String(),
		primeNetworkMagic:  primeNetworkMagic,
		vectorNetworkMagic: vectorNetworkMagic,
	}
}

func NewTestApexUserWithExistingWallets(
	t *testing.T, primePrivateKey, vectorPrivateKey string, primeNetworkMagic, vectorNetworkMagic uint,
) *TestApexUser {
	t.Helper()

	basePath, err := os.MkdirTemp("/tmp", "apex-user")
	require.NoError(t, err)

	primePrivateKeyBytes, err := (wallet.Key{Hex: primePrivateKey}).GetKeyBytes()
	require.NoError(t, err)

	vectorPrivateKeyBytes, err := (wallet.Key{Hex: vectorPrivateKey}).GetKeyBytes()
	require.NoError(t, err)

	primeWallet := wallet.NewWallet(
		wallet.GetVerificationKeyFromSigningKey(primePrivateKeyBytes), primePrivateKeyBytes)
	vectorWallet := wallet.NewWallet(
		wallet.GetVerificationKeyFromSigningKey(vectorPrivateKeyBytes), vectorPrivateKeyBytes)

	primeUserAddress, err := GetAddress(true, primeWallet)
	require.NoError(t, err)

	vectorUserAddress, err := GetAddress(false, vectorWallet)
	require.NoError(t, err)

	return &TestApexUser{
		basePath:           basePath,
		PrimeWallet:        primeWallet,
		VectorWallet:       vectorWallet,
		PrimeAddress:       primeUserAddress.String(),
		VectorAddress:      vectorUserAddress.String(),
		primeNetworkMagic:  primeNetworkMagic,
		vectorNetworkMagic: vectorNetworkMagic,
	}
}

func (u *TestApexUser) SendToUser(
	t *testing.T,
	ctx context.Context, txProvider wallet.ITxProvider,
	sender wallet.IWallet, sendAmount uint64,
	isPrime bool,
) {
	t.Helper()

	networkMagic := u.primeNetworkMagic
	addr := u.PrimeAddress

	if !isPrime {
		networkMagic = u.vectorNetworkMagic
		addr = u.VectorAddress
	}

	prevAmount, err := GetTokenAmount(ctx, txProvider, addr)
	require.NoError(t, err)

	_, err = SendTx(ctx, txProvider, sender,
		sendAmount, addr, int(networkMagic), isPrime, []byte{})
	require.NoError(t, err)

	err = wallet.WaitForAmount(
		context.Background(), txProvider, addr, func(val *big.Int) bool {
			return val.Cmp(prevAmount) > 0
		}, 60, time.Second*2, IsRecoverableError)
	require.NoError(t, err)
}

func (u *TestApexUser) SendToAddress(
	t *testing.T,
	ctx context.Context, txProvider wallet.ITxProvider,
	sender wallet.IWallet, sendAmount uint64,
	receiver string, isPrime bool,
) {
	t.Helper()

	networkMagic := u.primeNetworkMagic

	if !isPrime {
		networkMagic = u.vectorNetworkMagic
	}

	prevAmount, err := GetTokenAmount(ctx, txProvider, receiver)
	require.NoError(t, err)

	_, err = SendTx(ctx, txProvider, sender,
		sendAmount, receiver, int(networkMagic), isPrime, []byte{})
	require.NoError(t, err)

	err = wallet.WaitForAmount(
		context.Background(), txProvider, receiver, func(val *big.Int) bool {
			return val.Cmp(new(big.Int).SetUint64(prevAmount.Uint64()+sendAmount)) == 0
		}, 60, time.Second*2, IsRecoverableError)
	require.NoError(t, err)
}

func (u *TestApexUser) BridgeAmount(
	t *testing.T, ctx context.Context,
	txProvider wallet.ITxProvider,
	multisigAddr, feeAddr string, sendAmount uint64, isPrime bool,
) string {
	t.Helper()

	networkMagic := u.primeNetworkMagic
	sender := u.PrimeWallet
	receiverAddr := u.VectorAddress

	if !isPrime {
		networkMagic = u.vectorNetworkMagic
		sender = u.VectorWallet
		receiverAddr = u.PrimeAddress
	}

	txHash := BridgeAmountFull(t, ctx, txProvider, networkMagic, isPrime,
		multisigAddr, feeAddr, sender, receiverAddr, sendAmount, GetDestinationChainID(isPrime))

	return txHash
}

func (u *TestApexUser) Dispose() {
	_ = os.RemoveAll(u.basePath)
	_ = os.Remove(u.basePath)
}

func CreateMetaData(sender string, receivers map[string]uint64, destinationChainID string) ([]byte, error) {
	var transactions = make([]BridgingRequestMetadataTransaction, 0, len(receivers))
	for addr, amount := range receivers {
		transactions = append(transactions, BridgingRequestMetadataTransaction{
			Address: SplitString(addr, 40),
			Amount:  amount,
		})
	}

	metadata := map[string]interface{}{
		"1": map[string]interface{}{
			"t":  "bridge",
			"d":  destinationChainID,
			"s":  SplitString(sender, 40),
			"tx": transactions,
		},
	}

	return json.Marshal(metadata)
}

func GetDestinationChainID(isSourcePrime bool) string {
	if isSourcePrime {
		return "vector"
	} else {
		return "prime"
	}
}

func GetNetworkID(isSourcePrime bool) wallet.CardanoNetworkType {
	if isSourcePrime {
		return wallet.MainNetNetwork
	} else {
		return wallet.VectorMainNetNetwork
	}
}

func GetAddress(isSourcePrime bool, cardanoWallet wallet.IWallet) (wallet.CardanoAddress, error) {
	return wallet.NewEnterpriseAddress(GetNetworkID(isSourcePrime), cardanoWallet.GetVerificationKey())
}

func BridgeAmountFull(
	t *testing.T, ctx context.Context, txProvider wallet.ITxProvider, networkMagic uint,
	isPrime bool, multisigAddr, feeAddr string, sender wallet.IWallet,
	receiverAddr string, sendAmount uint64, destinationChainID string,
) string {
	t.Helper()

	return BridgeAmountFullMultipleReceivers(
		t, ctx, txProvider, networkMagic, isPrime, multisigAddr, feeAddr, sender,
		[]string{receiverAddr}, sendAmount, destinationChainID,
	)
}

func BridgeAmountFullMultipleReceivers(
	t *testing.T, ctx context.Context, txProvider wallet.ITxProvider, networkMagic uint, isPrime bool,
	multisigAddr, feeAddr string, sender wallet.IWallet,
	receiverAddrs []string, sendAmount uint64, destinationChainID string,
) string {
	t.Helper()

	require.Greater(t, len(receiverAddrs), 0)
	require.Less(t, len(receiverAddrs), 5)

	const feeAmount = 1_100_000

	senderAddr, err := GetAddress(true, sender)
	require.NoError(t, err)

	receivers := make(map[string]uint64, len(receiverAddrs)+1)
	receivers[feeAddr] = feeAmount

	for _, receiverAddr := range receiverAddrs {
		receivers[receiverAddr] = sendAmount
	}

	bridgingRequestMetadata, err := CreateMetaData(senderAddr.String(), receivers, destinationChainID)
	require.NoError(t, err)

	txHash, err := SendTx(ctx, txProvider, sender,
		uint64(len(receiverAddrs))*sendAmount+feeAmount, multisigAddr, int(networkMagic), isPrime, bridgingRequestMetadata)
	require.NoError(t, err)

	err = wallet.WaitForTxHashInUtxos(
		context.Background(), txProvider, multisigAddr, txHash, 60, time.Second*2, IsRecoverableError)
	require.NoError(t, err)

	return txHash
}
