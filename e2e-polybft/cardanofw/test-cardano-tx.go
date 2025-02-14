package cardanofw

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	infracommon "github.com/Ethernal-Tech/cardano-infrastructure/common"
	"github.com/Ethernal-Tech/cardano-infrastructure/wallet"
)

const (
	PotentialFee     = 500_000
	maxInputs        = 40
	ttlSlotNumberInc = 500
)

func SendTx(ctx context.Context,
	txProvider wallet.ITxProvider,
	senderWallet *wallet.Wallet,
	amount uint64,
	receiver string,
	networkType wallet.CardanoNetworkType,
	metadata []byte,
) (string, error) {
	return infracommon.ExecuteWithRetry(ctx, func(ctx context.Context) (string, error) {
		txBuilder, err := wallet.NewTxBuilder(ResolveCardanoCliBinary(networkType))
		if err != nil {
			return "", err
		}

		defer txBuilder.Dispose()

		caddr, err := GetAddress(networkType, senderWallet)
		if err != nil {
			return "", err
		}

		txRaw, txHash, err := createTx(
			ctx, txBuilder, txProvider, caddr.String(), amount, receiver, networkType, metadata)
		if err != nil {
			return "", err
		}

		signedTx, err := txBuilder.SignTx(txRaw, []wallet.ITxSigner{senderWallet})
		if err != nil {
			return "", err
		}

		return txHash, txProvider.SubmitTx(ctx, signedTx)
	})
}

func createTx(
	ctx context.Context,
	txBuilder *wallet.TxBuilder,
	txProvider wallet.ITxProvider,
	senderAddr string,
	amount uint64,
	receiverAddr string,
	networkType wallet.CardanoNetworkType,
	metadata []byte,
) ([]byte, string, error) {
	allUtxos, err := txProvider.GetUtxos(ctx, senderAddr)
	if err != nil {
		return nil, "", err
	}

	// utxos without tokens should come first
	sort.Slice(allUtxos, func(i, j int) bool {
		return len(allUtxos[i].Tokens) < len(allUtxos[j].Tokens)
	})

	if err := txBuilder.SetProtocolParametersAndTTL(ctx, txProvider, ttlSlotNumberInc); err != nil {
		return nil, "", err
	}

	txBuilder.SetTestNetMagic(GetNetworkMagic(networkType))

	if len(metadata) != 0 {
		txBuilder.SetMetaData(metadata)
	}

	minUtxoLovelace, err := wallet.GetTokenCostSum(txBuilder, senderAddr, allUtxos)
	if err != nil {
		return nil, "", err
	}

	desiredLovelacle := amount + PotentialFee + max(minUtxoLovelace, MinUTxODefaultValue)

	inputs, err := wallet.GetUTXOsForAmount(allUtxos, wallet.AdaTokenName, desiredLovelacle, maxInputs)
	if err != nil {
		return nil, "", err
	}

	senderTokens, err := wallet.GetTokensFromSumMap(inputs.Sum)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create tokens from sum map. err: %w", err)
	}

	txBuilder.AddInputs(inputs.Inputs...).AddOutputs(wallet.TxOutput{
		Addr:   receiverAddr,
		Amount: amount,
	}, wallet.TxOutput{
		Addr:   senderAddr,
		Tokens: senderTokens,
	})

	fee, err := txBuilder.CalculateFee(1)
	if err != nil {
		return nil, "", err
	}

	changeTxOutput, err := wallet.CreateTxOutputChange(wallet.TxOutput{
		Addr: senderAddr,
	}, inputs.Sum, map[string]uint64{
		wallet.AdaTokenName: amount + fee,
	})
	if err != nil {
		return nil, "", err
	}

	if changeTxOutput.Amount > 0 || len(changeTxOutput.Tokens) > 0 {
		txBuilder.ReplaceOutput(-1, changeTxOutput)
	} else {
		txBuilder.RemoveOutput(-1)
	}

	txBuilder.SetFee(fee)

	return txBuilder.Build()
}

func GetGenesisWalletFromCluster(
	dirPath string,
	keyID uint,
) (*wallet.Wallet, error) {
	keyFileName := strings.Join([]string{"utxo", fmt.Sprint(keyID)}, "")

	sKey, err := wallet.NewKey(filepath.Join(dirPath, "utxo-keys", fmt.Sprintf("%s.skey", keyFileName)))
	if err != nil {
		return nil, err
	}

	sKeyBytes, err := sKey.GetKeyBytes()
	if err != nil {
		return nil, err
	}

	return wallet.NewWallet(sKeyBytes, nil), nil
}

