package cardanofw

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/big"
	"os"

	cardanowallet "github.com/Ethernal-Tech/cardano-infrastructure/wallet"
)

const (
	defaultTokenName       = "test1"
	defaultTokenMintAmount = uint64(1_000_000_000)
)

func FundUserWithToken(
	ctx context.Context, apex *ApexSystem, chainID ChainID,
	minterWallet *cardanowallet.Wallet, userToFund *TestApexUser,
	tokenName string, mintAmount uint64,
	lovelaceFundAmount uint64, tokenFundAmount uint64,
) (*cardanowallet.TokenAmount, error) {
	chain, err := apex.getChain(chainID)
	if err != nil {
		return nil, err
	}

	cardanoChain, ok := chain.(*TestCardanoChain)
	if !ok {
		return nil, fmt.Errorf("failed to cast the chain to cardano chain")
	}

	return FundAddressWithToken(
		ctx, cardanoChain, minterWallet, userToFund.GetAddress(chain.ChainID()),
		tokenName, mintAmount, lovelaceFundAmount, tokenFundAmount)
}

func FundAddressWithToken(
	ctx context.Context, chain *TestCardanoChain,
	minterWallet *cardanowallet.Wallet, addrToFund string,
	tokenName string, mintAmount uint64,
	lovelaceFundAmount uint64, tokenFundAmount uint64,
) (*cardanowallet.TokenAmount, error) {
	if lovelaceFundAmount == 0 {
		return nil, fmt.Errorf("lovelace amount must be greater than zero")
	}

	if mintAmount > 0 {
		if err := MintToken(chain, minterWallet, tokenName, mintAmount); err != nil {
			return nil, err
		}
	}

	token, _, err := GetTokenAndPolicyForVerificationKey(
		chain.ChainID(), chain.config.NetworkType, minterWallet.VerificationKey, tokenName)
	if err != nil {
		return nil, err
	}

	tokenAmount := cardanowallet.NewTokenAmount(token, tokenFundAmount)

	minterAddr, err := GetAddress(chain.config.NetworkType, minterWallet)
	if err != nil {
		return nil, err
	}

	if minterAddr.String() == addrToFund {
		return &tokenAmount, nil
	}

	return FundAddressesWithToken(
		ctx, chain, minterWallet, []string{addrToFund}, tokenName, lovelaceFundAmount, tokenFundAmount)
}

func FundAddressesWithToken(
	ctx context.Context, chain *TestCardanoChain,
	sender *cardanowallet.Wallet, addrs []string,
	tokenName string, lovelaceFundAmount uint64, tokenFundAmount uint64,
) (*cardanowallet.TokenAmount, error) {
	token, _, err := GetTokenAndPolicyForVerificationKey(
		chain.ChainID(), chain.config.NetworkType, sender.VerificationKey, tokenName)
	if err != nil {
		return nil, err
	}

	tokenAmount := cardanowallet.NewTokenAmount(token, tokenFundAmount)
	privateKey := ToCardanoPrivateKeyString(sender.SigningKey, sender.StakeSigningKey)
	receivers := make([]GenericTxReceiver, len(addrs))

	for i, addr := range addrs {
		receivers[i] = GenericTxReceiver{
			Addr:   addr,
			Amount: new(big.Int).SetUint64(lovelaceFundAmount),
			NativeTokens: []cardanowallet.TokenAmount{
				tokenAmount,
			},
		}
	}

	txHash, err := chain.SendTx(ctx, privateKey, nil, receivers)
	if err != nil {
		return nil, err
	}

	fmt.Printf("Funded %s with lovelace: %d, native tokens: %s. txHash: %s\n",
		addrs, lovelaceFundAmount, tokenAmount, txHash)

	return &tokenAmount, nil
}

func MintToken(
	chain *TestCardanoChain, minterWallet *cardanowallet.Wallet, tokenName string, mintAmount uint64,
) error {
	args := []string{
		"bridge-admin", "mint-native-token",
		"--key", hex.EncodeToString(minterWallet.SigningKey),
		"--ogmios", chain.ogmiosURL,
		"--network-id", fmt.Sprintf("%v", chain.config.NetworkType),
		"--testnet-magic", fmt.Sprintf("%v", chain.config.NetworkMagic),
		"--token-name", tokenName,
		"--amount", fmt.Sprintf("%v", mintAmount),
	}

	if len(minterWallet.StakeSigningKey) > 0 {
		args = append(args, "--stake-key", hex.EncodeToString(minterWallet.StakeSigningKey))
	}

	return RunCommand(ResolveApexBridgeBinary(), args, os.Stdout)
}
