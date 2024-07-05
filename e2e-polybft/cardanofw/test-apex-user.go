package cardanofw

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Ethernal-Tech/cardano-infrastructure/wallet"
	"github.com/stretchr/testify/require"
)

type TestApexUser struct {
	PrimeWallet   wallet.IWallet
	VectorWallet  wallet.IWallet
	PrimeAddress  string
	VectorAddress string
}

type BridgingRequestMetadataTransaction struct {
	Address []string `cbor:"a" json:"a"`
	Amount  uint64   `cbor:"m" json:"m"`
}

func NewTestApexUser(t *testing.T, primeNetworkType wallet.CardanoNetworkType, vectorNetworkType wallet.CardanoNetworkType) *TestApexUser {
	t.Helper()

	primeWallet, err := wallet.GenerateWallet(false)
	require.NoError(t, err)

	vectorWallet, err := wallet.GenerateWallet(false)
	require.NoError(t, err)

	primeUserAddress, err := GetAddress(primeNetworkType, primeWallet)
	require.NoError(t, err)

	vectorUserAddress, err := GetAddress(vectorNetworkType, vectorWallet)
	require.NoError(t, err)

	return &TestApexUser{
		PrimeWallet:   primeWallet,
		VectorWallet:  vectorWallet,
		PrimeAddress:  primeUserAddress.String(),
		VectorAddress: vectorUserAddress.String(),
	}
}

func NewTestApexUserWithExistingWallets(t *testing.T, primePrivateKey, vectorPrivateKey string,
	primeNetworkId wallet.CardanoNetworkType, vectorNetworkId wallet.CardanoNetworkType,
) *TestApexUser {
	t.Helper()

	primePrivateKeyBytes, err := wallet.GetKeyBytes(primePrivateKey)
	require.NoError(t, err)

	vectorPrivateKeyBytes, err := wallet.GetKeyBytes(vectorPrivateKey)
	require.NoError(t, err)

	primeWallet := wallet.NewWallet(
		wallet.GetVerificationKeyFromSigningKey(primePrivateKeyBytes), primePrivateKeyBytes)
	vectorWallet := wallet.NewWallet(
		wallet.GetVerificationKeyFromSigningKey(vectorPrivateKeyBytes), vectorPrivateKeyBytes)

	primeUserAddress, err := GetAddress(primeNetworkId, primeWallet)
	require.NoError(t, err)

	vectorUserAddress, err := GetAddress(vectorNetworkId, vectorWallet)
	require.NoError(t, err)

	return &TestApexUser{
		PrimeWallet:   primeWallet,
		VectorWallet:  vectorWallet,
		PrimeAddress:  primeUserAddress.String(),
		VectorAddress: vectorUserAddress.String(),
	}
}

func (u *TestApexUser) SendToUser(
	t *testing.T,
	ctx context.Context, txProvider wallet.ITxProvider,
	sender wallet.IWallet, sendAmount uint64,
	networkConfig TestCardanoNetworkConfig,
) {
	t.Helper()

	addr := u.PrimeAddress
	if !networkConfig.IsPrime() {
		addr = u.VectorAddress
	}

	prevAmount, err := GetTokenAmount(ctx, txProvider, addr)
	require.NoError(t, err)

	_, err = SendTx(ctx, txProvider, sender,
		sendAmount, addr, networkConfig, []byte{})
	require.NoError(t, err)

	err = wallet.WaitForAmount(
		context.Background(), txProvider, addr, func(val uint64) bool {
			return val == prevAmount+sendAmount
		}, 60, time.Second*2, IsRecoverableError)
	require.NoError(t, err)
}

func (u *TestApexUser) SendToAddress(
	t *testing.T,
	ctx context.Context, txProvider wallet.ITxProvider,
	sender wallet.IWallet, sendAmount uint64,
	receiver string, networkConfig TestCardanoNetworkConfig,
) {
	t.Helper()

	prevAmount, err := GetTokenAmount(ctx, txProvider, receiver)
	require.NoError(t, err)

	_, err = SendTx(ctx, txProvider, sender,
		sendAmount, receiver, networkConfig, []byte{})
	require.NoError(t, err)

	err = wallet.WaitForAmount(
		context.Background(), txProvider, receiver, func(val uint64) bool {
			return val == prevAmount+sendAmount
		}, 60, time.Second*2, IsRecoverableError)
	require.NoError(t, err)
}

func (u *TestApexUser) BridgeAmount(
	t *testing.T, ctx context.Context,
	txProvider wallet.ITxProvider,
	multisigAddr, feeAddr string, sendAmount uint64,
	networkConfig TestCardanoNetworkConfig,
) string {
	t.Helper()

	sender := u.PrimeWallet
	receiverAddr := u.VectorAddress
	if !networkConfig.IsPrime() {
		sender = u.VectorWallet
		receiverAddr = u.PrimeAddress
	}

	txHash := BridgeAmountFull(t, ctx, txProvider, networkConfig,
		multisigAddr, feeAddr, sender, receiverAddr, sendAmount)

	return txHash
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

func BridgeAmountFull(
	t *testing.T, ctx context.Context, txProvider wallet.ITxProvider,
	networkConfig TestCardanoNetworkConfig, multisigAddr, feeAddr string, sender wallet.IWallet,
	receiverAddr string, sendAmount uint64,
) string {
	t.Helper()

	return BridgeAmountFullMultipleReceivers(
		t, ctx, txProvider, networkConfig, multisigAddr, feeAddr, sender,
		[]string{receiverAddr}, sendAmount,
	)
}

func BridgeAmountFullMultipleReceivers(
	t *testing.T, ctx context.Context, txProvider wallet.ITxProvider, networkConfig TestCardanoNetworkConfig,
	multisigAddr, feeAddr string, sender wallet.IWallet,
	receiverAddrs []string, sendAmount uint64,
) string {
	t.Helper()

	require.Greater(t, len(receiverAddrs), 0)
	require.Less(t, len(receiverAddrs), 5)

	const feeAmount = 1_100_000

	senderAddr, err := GetAddress(networkConfig.NetworkType, sender)
	require.NoError(t, err)

	receivers := make(map[string]uint64, len(receiverAddrs)+1)
	receivers[feeAddr] = feeAmount

	for _, receiverAddr := range receiverAddrs {
		receivers[receiverAddr] = sendAmount
	}

	bridgingRequestMetadata, err := CreateMetaData(senderAddr.String(), receivers, GetDestinationChainID(networkConfig))
	require.NoError(t, err)

	txHash, err := SendTx(ctx, txProvider, sender,
		uint64(len(receiverAddrs))*sendAmount+feeAmount, multisigAddr, networkConfig, bridgingRequestMetadata)
	require.NoError(t, err)

	err = wallet.WaitForTxHashInUtxos(
		context.Background(), txProvider, multisigAddr, txHash, 60, time.Second*2, IsRecoverableError)
	require.NoError(t, err)

	return txHash
}
