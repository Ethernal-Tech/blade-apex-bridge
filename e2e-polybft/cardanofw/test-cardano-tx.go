package cardanofw

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Ethernal-Tech/cardano-infrastructure/wallet"
)

const (
	potentialFee     = 250_000
	ttlSlotNumberInc = 500
)

func SendTx(ctx context.Context,
	txProvider wallet.ITxProvider,
	cardanoWallet wallet.IWallet,
	amount uint64,
	receiver string,
	networkType wallet.CardanoNetworkType,
	metadata []byte,
) (res string, err error) {
	err = ExecuteWithRetryIfNeeded(ctx, func() error {
		res, err = sendTx(ctx, txProvider, cardanoWallet, amount, receiver, networkType, metadata)

		return err
	})

	return res, err
}

func sendTx(ctx context.Context,
	txProvider wallet.ITxProvider,
	cardanoWallet wallet.IWallet,
	amount uint64,
	receiver string,
	networkType wallet.CardanoNetworkType,
	metadata []byte,
) (string, error) {
	caddr, err := GetAddress(networkType, cardanoWallet)
	if err != nil {
		return "", err
	}

	cardanoWalletAddr := caddr.String()
	networkTestMagic := GetNetworkMagic(networkType)
	cardanoCliBinary := ResolveCardanoCliBinary(networkType)

	protocolParams, err := txProvider.GetProtocolParameters(ctx)
	if err != nil {
		return "", err
	}

	qtd, err := txProvider.GetTip(ctx)
	if err != nil {
		return "", err
	}

	outputs := []wallet.TxOutput{
		{
			Addr:   receiver,
			Amount: amount,
		},
	}
	desiredSum := amount + potentialFee + wallet.MinUTxODefaultValue

	inputs, err := wallet.GetUTXOsForAmount(
		ctx, txProvider, cardanoWalletAddr, desiredSum, desiredSum)
	if err != nil {
		return "", err
	}

	rawTx, txHash, err := CreateTx(
		cardanoCliBinary,
		networkTestMagic, protocolParams,
		qtd.Slot+ttlSlotNumberInc, metadata,
		outputs, inputs, cardanoWalletAddr)
	if err != nil {
		return "", err
	}

	witness, err := wallet.CreateTxWitness(txHash, cardanoWallet)
	if err != nil {
		return "", err
	}

	signedTx, err := AssembleTxWitnesses(cardanoCliBinary, rawTx, [][]byte{witness})
	if err != nil {
		return "", err
	}

	return txHash, txProvider.SubmitTx(ctx, signedTx)
}

func GetGenesisWalletFromCluster(
	dirPath string,
	keyID uint,
) (wallet.IWallet, error) {
	keyFileName := strings.Join([]string{"utxo", fmt.Sprint(keyID)}, "")

	sKey, err := wallet.NewKey(filepath.Join(dirPath, "utxo-keys", fmt.Sprintf("%s.skey", keyFileName)))
	if err != nil {
		return nil, err
	}

	sKeyBytes, err := sKey.GetKeyBytes()
	if err != nil {
		return nil, err
	}

	vKey, err := wallet.NewKey(filepath.Join(dirPath, "utxo-keys", fmt.Sprintf("%s.vkey", keyFileName)))
	if err != nil {
		return nil, err
	}

	vKeyBytes, err := vKey.GetKeyBytes()
	if err != nil {
		return nil, err
	}

	return wallet.NewWallet(vKeyBytes, sKeyBytes), nil
}

// CreateTx creates tx and returns cbor of raw transaction data, tx hash and error
func CreateTx(
	cardanoCliBinary string,
	testNetMagic uint,
	protocolParams []byte,
	timeToLive uint64,
	metadataBytes []byte,
	outputs []wallet.TxOutput,
	inputs wallet.TxInputs,
	changeAddress string,
) ([]byte, string, error) {
	outputsSum := wallet.GetOutputsSum(outputs)

	builder, err := wallet.NewTxBuilder(cardanoCliBinary)
	if err != nil {
		return nil, "", err
	}

	defer builder.Dispose()

	if len(metadataBytes) != 0 {
		builder.SetMetaData(metadataBytes)
	}

	builder.SetProtocolParameters(protocolParams).SetTimeToLive(timeToLive).
		SetTestNetMagic(testNetMagic).
		AddInputs(inputs.Inputs...).
		AddOutputs(outputs...).AddOutputs(wallet.TxOutput{Addr: changeAddress})

	fee, err := builder.CalculateFee(0)
	if err != nil {
		return nil, "", err
	}

	change := inputs.Sum - outputsSum - fee
	// handle overflow or insufficient amount
	if change > inputs.Sum || (change > 0 && change < wallet.MinUTxODefaultValue) {
		return []byte{}, "", fmt.Errorf("insufficient amount %d for %d or min utxo not satisfied",
			inputs.Sum, outputsSum+fee)
	}

	if change == 0 {
		builder.RemoveOutput(-1)
	} else {
		builder.UpdateOutputAmount(-1, change)
	}

	builder.SetFee(fee)

	return builder.Build()
}

// CreateTxWitness creates cbor of vkey+signature pair of tx hash
func CreateTxWitness(txHash string, key wallet.ISigner) ([]byte, error) {
	return wallet.CreateTxWitness(txHash, key)
}

// AssembleTxWitnesses assembles all witnesses in final cbor of signed tx
func AssembleTxWitnesses(cardanoCliBinary string, txRaw []byte, witnesses [][]byte) ([]byte, error) {
	builder, err := wallet.NewTxBuilder(cardanoCliBinary)
	if err != nil {
		return nil, err
	}

	defer builder.Dispose()

	return builder.AssembleTxWitnesses(txRaw, witnesses)
}
