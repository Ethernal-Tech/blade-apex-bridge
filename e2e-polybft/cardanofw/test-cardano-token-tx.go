package cardanofw

import (
	"context"
	"errors"
	"fmt"

	"github.com/Ethernal-Tech/cardano-infrastructure/common"
	cardanowallet "github.com/Ethernal-Tech/cardano-infrastructure/wallet"
)

const (
	defaultTokenName       = "test1"
	defaultTokenMintAmount = uint64(1_000_000_000)
)

func FundUserWithToken(ctx context.Context, chain ChainID,
	networkType cardanowallet.CardanoNetworkType, networkMagic uint, txProvider cardanowallet.ITxProvider,
	minterUser *TestApexUser, userToFund *TestApexUser, lovelaceFundAmount uint64, tokenFundAmount uint64,
) (*cardanowallet.TokenAmount, error) {
	minterWallet, _ := minterUser.GetCardanoWallet(chain)

	keyHash, err := cardanowallet.GetKeyHash(minterWallet.VerificationKey)
	if err != nil {
		return nil, err
	}

	policyScript := &cardanowallet.PolicyScript{
		Type:    cardanowallet.PolicyScriptSigType,
		KeyHash: keyHash,
	}

	cardanoCliBinary := cardanowallet.ResolveCardanoCliBinary(networkType)

	pid, err := cardanowallet.NewCliUtils(cardanoCliBinary).GetPolicyID(policyScript)
	if err != nil {
		return nil, err
	}

	mintToken := cardanowallet.NewTokenAmount(pid, defaultTokenName, defaultTokenMintAmount)

	txHash, err := MintTokens(
		ctx, networkType, networkMagic, txProvider, minterWallet, lovelaceFundAmount,
		[]cardanowallet.TokenAmount{mintToken}, []cardanowallet.IPolicyScript{policyScript},
	)
	if err != nil {
		return nil, err
	}

	fmt.Printf("Done minting tokens. txHash: %s\n", txHash)

	userToFundAddr := userToFund.GetAddress(chain)
	fundToken := cardanowallet.NewTokenAmount(pid, defaultTokenName, tokenFundAmount)

	txHash, err = SendTxWithTokens(
		ctx, networkType, networkMagic, txProvider, minterWallet, userToFundAddr, lovelaceFundAmount,
		[]cardanowallet.TokenAmount{fundToken}, nil,
	)
	if err != nil {
		return nil, err
	}

	fmt.Printf("Funded user %s with lovelace + native token %s. txHash: %s\n",
		userToFundAddr, fundToken.TokenName(), txHash)

	return &fundToken, nil
}

func SendTxWithTokens(
	ctx context.Context,
	networkType cardanowallet.CardanoNetworkType,
	networkMagic uint,
	txProvider cardanowallet.ITxProvider,
	senderWallet *cardanowallet.Wallet,
	receiverAddr string,
	lovelaceAmount uint64,
	tokens []cardanowallet.TokenAmount,
	metadata []byte,
) (string, error) {
	if len(tokens) == 0 {
		return "", errors.New("no tokens")
	}

	txRaw, txHash, err := createNativeTokenTx(
		ctx, networkType, networkMagic, txProvider, senderWallet, receiverAddr, lovelaceAmount, tokens, metadata)
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
	networkType cardanowallet.CardanoNetworkType,
	networkMagic uint,
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
		ctx, networkType, networkMagic, txProvider, wallet, lovelaceAmount,
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
	networkType cardanowallet.CardanoNetworkType,
	networkMagic uint,
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

	builder.SetTestNetMagic(networkMagic)

	if err := builder.SetProtocolParametersAndTTL(ctx, txProvider, 0); err != nil {
		return nil, "", err
	}

	if len(metadata) != 0 {
		builder.SetMetaData(metadata)
	}

	allUtxos, err := txProvider.GetUtxos(ctx, senderAddr)
	if err != nil {
		return nil, "", err
	}

	minUtxoLovelace, err := cardanowallet.GetTokenCostSum(builder, senderWalletAddr.String(), allUtxos)
	if err != nil {
		return nil, "", err
	}

	receiverOutput := cardanowallet.TxOutput{
		Addr:   receiverAddr,
		Amount: lovelaceAmount,
		Tokens: tokens,
	}
	desiredLovelaceAmount := PotentialFee + lovelaceAmount + max(minUtxoLovelace, MinUTxODefaultValue)

	// This is a hacky way to get inputs for both lovelace and tokens
	// will do it this way, until skyline is merged to main
	// after that, this can be removed
	inputs, err := getInputs(allUtxos, desiredLovelaceAmount, tokens)
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

func getInputs(
	allUtxos []cardanowallet.Utxo,
	desiredLovelaceAmount uint64,
	tokens []cardanowallet.TokenAmount,
) (*cardanowallet.TxInputs, error) {
	inputsMap := make(map[string]cardanowallet.TxInput)

	inputsLovelace, err := cardanowallet.GetUTXOsForAmount(
		allUtxos, cardanowallet.AdaTokenName, desiredLovelaceAmount, maxInputs)
	if err != nil {
		return nil, err
	}

	for _, input := range inputsLovelace.Inputs {
		inputsMap[input.String()] = input
	}

	for _, token := range tokens {
		inputsToken, err := cardanowallet.GetUTXOsForAmount(
			allUtxos, token.TokenName(), token.Amount, maxInputs)
		if err != nil {
			return nil, err
		}

		for _, input := range inputsToken.Inputs {
			inputsMap[input.String()] = input
		}
	}

	utxoMap := make(map[string]cardanowallet.Utxo, len(allUtxos))
	for _, utxo := range allUtxos {
		utxoMap[fmt.Sprintf("%s#%d", utxo.Hash, utxo.Index)] = utxo
	}

	inputs := cardanowallet.TxInputs{
		Inputs: make([]cardanowallet.TxInput, 0, len(inputsMap)),
		Sum:    make(map[string]uint64),
	}
	for _, input := range inputsMap {
		inputs.Inputs = append(inputs.Inputs, input)

		utxo, exists := utxoMap[input.String()]
		if !exists {
			return nil, fmt.Errorf("can not find utxo for input %v", input.String())
		}

		inputs.Sum[cardanowallet.AdaTokenName] += utxo.Amount
		for _, token := range utxo.Tokens {
			inputs.Sum[token.TokenName()] += token.Amount
		}
	}

	return &inputs, nil
}

func createMintTx(
	ctx context.Context,
	networkType cardanowallet.CardanoNetworkType,
	networkMagic uint,
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

	builder.SetTestNetMagic(networkMagic)

	if err := builder.SetProtocolParametersAndTTL(ctx, txProvider, 0); err != nil {
		return nil, "", err
	}

	allUtxos, err := txProvider.GetUtxos(ctx, senderAddr)
	if err != nil {
		return nil, "", err
	}

	minUtxoLovelace, err := cardanowallet.GetTokenCostSum(builder, senderAddr, allUtxos)
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
		Tokens: tokens,
	}

	builder.AddInputs(inputs.Inputs...).AddTokenMints(tokenPolicyScripts, tokens)
	builder.AddOutputs(txOutput, cardanowallet.TxOutput{
		Addr:   walletAddr.String(),
		Tokens: senderTokens,
	})

	fee, err := builder.CalculateFee(1)
	if err != nil {
		return nil, "", err
	}

	outputsSumMap := cardanowallet.GetOutputsSum([]cardanowallet.TxOutput{txOutput})
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
		return err
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
		return err
	}

	fmt.Printf("transaction has been included in block. hash = %s, balance = %v\n", txHash, newAmounts)

	return nil
}
