package cardanofw

import (
	"context"
	"errors"
	"fmt"

	"github.com/Ethernal-Tech/cardano-infrastructure/common"
	"github.com/Ethernal-Tech/cardano-infrastructure/sendtx"
	cardanowallet "github.com/Ethernal-Tech/cardano-infrastructure/wallet"
)

const (
	defaultTokenName       = "test1"
	DefaultTokenMintAmount = uint64(1_000_000_000)
)

func FundUserWithToken(ctx context.Context, chain ChainID,
	networkType cardanowallet.CardanoNetworkType, txProvider cardanowallet.ITxProvider,
	minterWallet *cardanowallet.Wallet, userToFund *TestApexUser, lovelaceFundAmount uint64, tokenFundAmount uint64,
) (*cardanowallet.TokenAmount, error) {
	return FundAddressWithToken(
		ctx, chain, networkType, txProvider, minterWallet,
		userToFund.GetAddress(chain), lovelaceFundAmount, tokenFundAmount)
}

func FundAddressWithToken(ctx context.Context, chain ChainID,
	networkType cardanowallet.CardanoNetworkType, txProvider cardanowallet.ITxProvider,
	minterWallet *cardanowallet.Wallet, addrToFund string,
	lovelaceFundAmount uint64, tokenFundAmount uint64,
) (*cardanowallet.TokenAmount, error) {
	token, policy, err := GetTokenAndPolicyForVerificationKey(
		chain, networkType, minterWallet.VerificationKey, defaultTokenName)
	if err != nil {
		return nil, err
	}

	var (
		fundTokenAmountObj = cardanowallet.NewTokenAmount(token, tokenFundAmount)
		tokens             []cardanowallet.TokenAmount
	)

	if tokenFundAmount > 0 {
		mintToken := cardanowallet.NewTokenAmount(token, DefaultTokenMintAmount)

		txHash, err := MintTokens(
			ctx, chain, networkType, txProvider, minterWallet, lovelaceFundAmount,
			[]cardanowallet.TokenAmount{mintToken}, []cardanowallet.IPolicyScript{policy},
		)
		if err != nil {
			return nil, err
		}

		tokens = append(tokens, fundTokenAmountObj)

		fmt.Printf("Done minting tokens: %d. txHash: %s\n", tokenFundAmount, txHash)
	}

	txHash, err := SendTxWithTokens(
		ctx, chain, networkType, txProvider, minterWallet, addrToFund, lovelaceFundAmount, tokens, nil)
	if err != nil {
		return nil, err
	}

	fmt.Printf("Funded %s with lovelace: %d, native tokens: %s. txHash: %s\n",
		addrToFund, lovelaceFundAmount, fundTokenAmountObj, txHash)

	return &fundTokenAmountObj, nil
}

func SendTxWithTokens(
	ctx context.Context,
	chainID ChainID,
	networkType cardanowallet.CardanoNetworkType,
	txProvider cardanowallet.ITxProvider,
	senderWallet *cardanowallet.Wallet,
	receiverAddr string,
	lovelaceAmount uint64,
	tokens []cardanowallet.TokenAmount,
	metadata []byte,
) (string, error) {
	txRaw, txHash, err := createNativeTokenTx(
		ctx, chainID, networkType, txProvider, senderWallet, receiverAddr, lovelaceAmount, tokens, metadata)
	if err != nil {
		return "", err
	}

	err = submitTokenTx(ctx, txProvider, txRaw, txHash, receiverAddr)
	if err != nil {
		return "", err
	}

	return txHash, nil
}

func MintTokens(
	ctx context.Context,
	chainID ChainID,
	networkType cardanowallet.CardanoNetworkType,
	txProvider cardanowallet.ITxProvider,
	wallet *cardanowallet.Wallet,
	lovelaceAmount uint64,
	tokens []cardanowallet.TokenAmount,
	tokenPolicyScripts []cardanowallet.IPolicyScript,
) (string, error) {
	if len(tokens) == 0 || len(tokenPolicyScripts) == 0 {
		return "", errors.New("no tokens or policy scripts")
	}

	walletAddr, err := GetAddress(networkType, wallet)
	if err != nil {
		return "", err
	}

	txRaw, txHash, err := createMintTx(
		ctx, chainID, networkType, txProvider, wallet, lovelaceAmount,
		tokens, tokenPolicyScripts,
	)
	if err != nil {
		return "", err
	}

	err = submitTokenTx(ctx, txProvider, txRaw, txHash, walletAddr.String())
	if err != nil {
		return "", err
	}

	return txHash, nil
}

func createNativeTokenTx(
	ctx context.Context,
	chainID ChainID,
	networkType cardanowallet.CardanoNetworkType,
	txProvider cardanowallet.ITxProvider,
	senderWallet *cardanowallet.Wallet,
	receiverAddr string,
	lovelaceAmount uint64,
	tokens []cardanowallet.TokenAmount,
	metadata []byte,
) ([]byte, string, error) {
	senderWalletAddr, err := GetAddress(networkType, senderWallet)
	if err != nil {
		return nil, "", err
	}

	senderAddr := senderWalletAddr.String()

	builder, err := cardanowallet.NewTxBuilder(ResolveCardanoCliBinary(networkType))
	if err != nil {
		return nil, "", err
	}

	defer builder.Dispose()

	builder.SetTestNetMagic(GetNetworkMagic(networkType, chainID))

	if err := builder.SetProtocolParametersAndTTL(ctx, txProvider, 0); err != nil {
		return nil, "", err
	}

	if len(metadata) != 0 {
		builder.SetMetaData(metadata)
	}

	allUtxos, err := common.ExecuteWithRetry(ctx, func(ctx context.Context) ([]cardanowallet.Utxo, error) {
		return txProvider.GetUtxos(ctx, senderAddr)
	})
	if err != nil {
		return nil, "", err
	}

	minUtxoLovelace, err := cardanowallet.GetMinUtxoForSumMap(
		builder,
		senderWalletAddr.String(),
		cardanowallet.SubtractSumMaps(
			cardanowallet.GetUtxosSum(allUtxos),
			cardanowallet.GetTokensSumMap(tokens...),
		))
	if err != nil {
		return nil, "", err
	}

	receiverOutput := cardanowallet.TxOutput{
		Addr:   receiverAddr,
		Amount: lovelaceAmount,
		Tokens: tokens,
	}
	desiredLovelaceAmount := PotentialFee + lovelaceAmount + max(minUtxoLovelace, MinUTxODefaultValue)

	conditions := map[string]uint64{
		cardanowallet.AdaTokenName: desiredLovelaceAmount,
	}
	for _, token := range tokens {
		conditions[token.Token.String()] = token.Amount
	}

	inputs, err := sendtx.GetUTXOsForAmounts(allUtxos, conditions, maxInputs, 1)
	if err != nil {
		return nil, "", err
	}

	senderTokens, err := cardanowallet.GetTokensFromSumMap(inputs.Sum)
	if err != nil {
		return nil, "", err
	}

	builder.AddInputs(inputs.Inputs...)
	builder.AddOutputs(receiverOutput, cardanowallet.TxOutput{
		Addr:   senderAddr,
		Tokens: senderTokens,
	})

	fee, err := builder.CalculateFee(1)
	if err != nil {
		return nil, "", err
	}

	outputsSumMap := cardanowallet.GetOutputsSum([]cardanowallet.TxOutput{receiverOutput})
	outputsSumMap[cardanowallet.AdaTokenName] += fee

	changeTxOutput, err := cardanowallet.CreateTxOutputChange(cardanowallet.TxOutput{
		Addr: senderAddr,
	}, inputs.Sum, outputsSumMap)
	if err != nil {
		return nil, "", err
	}

	if changeTxOutput.Amount > 0 || len(changeTxOutput.Tokens) > 0 {
		builder.ReplaceOutput(-1, changeTxOutput)
	} else {
		builder.RemoveOutput(-1)
	}

	builder.SetFee(fee)

	txRaw, txHash, err := builder.Build()
	if err != nil {
		return nil, "", err
	}

	txSigned, err := builder.SignTx(txRaw, []cardanowallet.ITxSigner{senderWallet})
	if err != nil {
		return nil, "", err
	}

	return txSigned, txHash, nil
}

func createMintTx(
	ctx context.Context,
	chainID ChainID,
	networkType cardanowallet.CardanoNetworkType,
	txProvider cardanowallet.ITxProvider,
	wallet *cardanowallet.Wallet,
	lovelaceAmount uint64,
	tokens []cardanowallet.TokenAmount,
	tokenPolicyScripts []cardanowallet.IPolicyScript,
) ([]byte, string, error) {
	walletAddr, err := GetAddress(networkType, wallet)
	if err != nil {
		return nil, "", err
	}

	senderAddr := walletAddr.String()

	builder, err := cardanowallet.NewTxBuilder(ResolveCardanoCliBinary(networkType))
	if err != nil {
		return nil, "", err
	}

	defer builder.Dispose()

	builder.SetTestNetMagic(GetNetworkMagic(networkType, chainID))

	if err := builder.SetProtocolParametersAndTTL(ctx, txProvider, 0); err != nil {
		return nil, "", err
	}

	allUtxos, err := txProvider.GetUtxos(ctx, senderAddr)
	if err != nil {
		return nil, "", err
	}

	minUtxoLovelace, err := cardanowallet.GetMinUtxoForSumMap(
		builder,
		senderAddr,
		cardanowallet.AddSumMaps(
			cardanowallet.GetUtxosSum(allUtxos),
			cardanowallet.GetTokensSumMap(tokens...),
		))
	if err != nil {
		return nil, "", err
	}

	desiredLovelaceAmount := PotentialFee + lovelaceAmount + max(minUtxoLovelace, MinUTxODefaultValue)

	inputs, err := cardanowallet.GetUTXOsForAmount(
		allUtxos, cardanowallet.AdaTokenName, desiredLovelaceAmount, maxInputs)
	if err != nil {
		return nil, "", err
	}

	senderTokens, err := cardanowallet.GetTokensFromSumMap(inputs.Sum)
	if err != nil {
		return nil, "", err
	}

	txOutput := cardanowallet.TxOutput{
		Addr:   senderAddr,
		Amount: lovelaceAmount,
		Tokens: append(senderTokens, tokens...),
	}

	builder.AddInputs(inputs.Inputs...).AddTokenMints(tokenPolicyScripts, tokens)
	builder.AddOutputs(txOutput, cardanowallet.TxOutput{
		Addr: senderAddr,
	})

	fee, err := builder.CalculateFee(1)
	if err != nil {
		return nil, "", err
	}

	outputsSumMap := cardanowallet.GetOutputsSum([]cardanowallet.TxOutput{txOutput})
	outputsSumMap[cardanowallet.AdaTokenName] += fee

	lovelaceInputAmount := inputs.Sum[cardanowallet.AdaTokenName]

	change := lovelaceInputAmount - lovelaceAmount - fee
	// handle overflow or insufficient amount
	if change > lovelaceInputAmount || change < max(minUtxoLovelace, MinUTxODefaultValue) {
		return []byte{}, "", fmt.Errorf("insufficient amount: %d", change)
	}

	if change > 0 {
		builder.UpdateOutputAmount(-1, change)
	} else {
		builder.RemoveOutput(-1)
	}

	builder.SetFee(fee)

	txRaw, txHash, err := builder.Build()
	if err != nil {
		return nil, "", err
	}

	txSigned, err := builder.SignTx(txRaw, []cardanowallet.ITxSigner{wallet})
	if err != nil {
		return nil, "", err
	}

	return txSigned, txHash, nil
}

func submitTokenTx(
	ctx context.Context,
	txProvider cardanowallet.ITxProvider,
	txRaw []byte,
	txHash string,
	receiverAddr string,
) error {
	if err := txProvider.SubmitTx(ctx, txRaw); err != nil {
		return fmt.Errorf("error while submitting tx %s: %w", txHash, err)
	}

	fmt.Println("transaction has been submitted. hash =", txHash)

	newAmounts, err := common.ExecuteWithRetry(ctx, func(ctx context.Context) (map[string]uint64, error) {
		utxos, err := txProvider.GetUtxos(ctx, receiverAddr)
		if err != nil {
			return nil, err
		}

		for _, x := range utxos {
			if x.Hash == txHash {
				return cardanowallet.GetUtxosSum(utxos), nil
			}
		}

		return nil, common.ErrRetryTryAgain
	}, common.WithRetryCount(60))
	if err != nil {
		return fmt.Errorf("error while waiting for tx %s to be included in a block: %w", txHash, err)
	}

	fmt.Printf("transaction has been included in block. hash = %s, balance = %v\n", txHash, newAmounts)

	return nil
}
