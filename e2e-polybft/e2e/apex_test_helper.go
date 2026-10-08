package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math/big"
	"net/http"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/e2e-polybft/cardanofw"
	"github.com/0xPolygon/polygon-edge/helper/hex"
	infracommon "github.com/Ethernal-Tech/cardano-infrastructure/common"
	"github.com/Ethernal-Tech/cardano-infrastructure/sendtx"
	"github.com/Ethernal-Tech/cardano-infrastructure/wallet"
	"github.com/stretchr/testify/require"
)

type WaitOption int

const (
	WaitRefundDisabled WaitOption = iota
	WaitRefundEnabled
	WaitTimeoutRefundDisabled
	NoWait
)

const (
	bridgingMetaDataType sendtx.BridgingRequestType = "bridge"
	metadataMapKey       int                        = 1
)

type colCoinInvalidOpts struct {
	receivers        []sendtx.BridgingTxReceiver
	amount           *big.Int
	waitOption       WaitOption
	metadataModifier func([]byte) []byte
}

// backward compatibility
type BridgingRequestMetadataTransactionBC struct {
	Address                     []string `cbor:"a" json:"a"`
	IsNativeTokenOnSrc_Obsolete byte     `cbor:"nt" json:"nt"` //nolint:stylecheck
	Amount                      *big.Int `cbor:"m" json:"m"`
	TokenID                     uint16   `cbor:"t" json:"t"`
}

// backward compatibility
type BridgingRequestMetadataBC struct {
	BridgingTxType     sendtx.BridgingRequestType             `cbor:"t" json:"t"`
	DestinationChainID string                                 `cbor:"d" json:"d"`
	SenderAddr         []string                               `cbor:"s" json:"s"`
	Transactions       []BridgingRequestMetadataTransactionBC `cbor:"tx" json:"tx"`
	BridgingFee        *big.Int                               `cbor:"fa" json:"fa"`
	OperationFee       *big.Int                               `cbor:"of" json:"of"`
}

type testConfig struct {
	srcChainID cardanofw.ChainID
	dstChainID cardanofw.ChainID

	srcMinterWallet *wallet.Wallet
	srcNetworkType  wallet.CardanoNetworkType
	srcTxProvider   wallet.ITxProvider
	srcMultiSigAddr string
	tokenID         uint16
	tokensInfo      *cardanofw.BridgingTokensInfo
	isCurrency      bool
}

var (
	defaultSendAmount = cardanofw.ApexToWei(big.NewInt(1))
)

func newTestConfig(
	t *testing.T, apex *cardanofw.ApexSystem, config *cardanofw.TestCardanoChainConfig,
	info *cardanofw.CardanoChainInfo, dstChainID cardanofw.ChainID, srcTokenID uint16,
) *testConfig {
	t.Helper()

	txProvider, err := info.GetTxProvider()
	require.NoError(t, err)

	tokenInfo, err := apex.GetBridgingTokensInfo(config.ChainType, dstChainID, srcTokenID)
	require.NoError(t, err)

	currencyID, err := apex.GetChainCurrencyID(config.ChainType)
	require.NoError(t, err)

	return &testConfig{
		srcChainID:      config.ChainType,
		dstChainID:      dstChainID,
		srcNetworkType:  config.NetworkType,
		srcTxProvider:   txProvider,
		srcMinterWallet: info.GenesisWallet,
		srcMultiSigAddr: info.MultisigAddr[0],
		tokenID:         srcTokenID,
		tokensInfo:      tokenInfo,
		isCurrency:      currencyID == srcTokenID,
	}
}

// Util methods
func WaitForInvalidTestResult(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, config *testConfig, user *cardanofw.TestApexUser,
	txHash string, beforeSendingAmount map[string]*big.Int, sentAmount *big.Int,
	refundEnabled bool, maxWaitTimeSec, retryIntervalSec uint,
) {
	t.Helper()

	retryIntervalSec = max(retryIntervalSec, 1)
	numRetries := max(1, int(maxWaitTimeSec/retryIntervalSec))

	if refundEnabled {
		lowerBoundary := new(big.Int).Sub(
			beforeSendingAmount[config.tokensInfo.SrcTokenName], sentAmount)

		fmt.Printf("Tx sent. hash: %s, lowerBoundary: %+v, higherBoundary: %+v\n", txHash, lowerBoundary,
			beforeSendingAmount)

		err := apex.WaitForAmountInRange(ctx, user, config.srcChainID, lowerBoundary,
			beforeSendingAmount[config.tokensInfo.SrcTokenName], numRetries,
			time.Second*time.Duration(retryIntervalSec), config.tokensInfo.SrcTokenName)
		require.NoError(t, err)
	} else {
		cardanofw.WaitForInvalidState(t, ctx, apex, config.srcChainID, txHash, apex.Config.APIKey, maxWaitTimeSec)
	}
}

// Test methods
func submitMismatchAndWait(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, config *testConfig, user *cardanofw.TestApexUser,
	metadata []byte, lovelaceAmount *big.Int, sentTokenAmount []cardanofw.GenericTokenAmount, waitForAmount *big.Int,
	waitOption WaitOption, maxWaitTimeSec, retryIntervalSec uint, addrIndex uint8,
	operationFee *big.Int, validateTreasury bool,
) {
	t.Helper()

	beforeSendingAmount, err := apex.GetBalance(ctx, user, config.srcChainID)
	require.NoError(t, err)

	initialTreasuryBalance, err := apex.GetTreasuryAddressBalance(ctx, t, config.srcChainID)
	require.NoError(t, err)

	txHash, err := apex.SubmitTx(
		ctx, config.srcChainID, user, apex.GetCardanoInfo(config.srcChainID).MultisigAddr[addrIndex],
		lovelaceAmount, sentTokenAmount, metadata, operationFee)
	require.NoError(t, err)

	if waitOption == WaitTimeoutRefundDisabled {
		_, err = cardanofw.WaitForRequestStates(ctx, apex, config.srcChainID, txHash, apex.Config.APIKey, nil, maxWaitTimeSec)
		require.Error(t, err)
		require.ErrorContains(t, err, "timeout")

		return
	}

	if waitOption != NoWait {
		WaitForInvalidTestResult(t, ctx, apex, config, user, txHash, beforeSendingAmount, waitForAmount,
			waitOption == WaitRefundEnabled, maxWaitTimeSec, retryIntervalSec)
	}

	if validateTreasury && initialTreasuryBalance != nil && operationFee.Cmp(big.NewInt(0)) > 0 {
		err = apex.ValidateTreasuryAddressBalance(ctx, t, config.srcChainID, initialTreasuryBalance, 1)
		require.NoError(t, err)
	}
}

func submitColCoinsMismatchAndWait(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, config *testConfig, user *cardanofw.TestApexUser,
	receivers []sendtx.BridgingTxReceiver, amount *big.Int,
	maxWaitTimeSec, retryIntervalSec uint, addrIndex uint8,
	waitOption WaitOption, metadataModifier func([]byte) []byte,
	validateTreasury bool,
) {
	t.Helper()

	operationFee := apex.GetMinOperationFee(config.srcChainID)

	metadata, feeAmount := createMetadata(t, ctx, apex, config.srcChainID, config.dstChainID,
		apex.GetMinBridgingFee(config.srcChainID, !config.isCurrency),
		operationFee, user, receivers, config.isCurrency)

	if metadataModifier != nil {
		metadata = metadataModifier(metadata)
	}

	waitForAmount := amount
	weiAmount := new(big.Int).Add(feeAmount, operationFee)

	token, err := wallet.NewTokenWithFullName(config.tokensInfo.SrcTokenName, true)
	require.NoError(t, err)

	sentTokenAmount := []cardanofw.GenericTokenAmount{cardanofw.NewGenericTokenAmount(token, amount)}

	submitMismatchAndWait(t, ctx, apex, config, user, metadata, weiAmount, sentTokenAmount, waitForAmount,
		waitOption, maxWaitTimeSec, retryIntervalSec, addrIndex, operationFee, validateTreasury)
}

func executeInvalidMismatchSendLovelaceAmount(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, config *testConfig, user *cardanofw.TestApexUser,
	maxWaitTimeSec, retryIntervalSec uint, refundEnabled bool, addrIndex uint8,
) {
	t.Helper()

	receivers := createReceivers(
		apex,
		1,
		config.dstChainID,
		new(big.Int).Mul(defaultSendAmount, big.NewInt(10)),
		config.tokenID,
	)

	operationFee := apex.GetMinOperationFee(config.srcChainID)

	metadata, feeAmount := createMetadata(t, ctx, apex, config.srcChainID, config.dstChainID,
		apex.GetMinBridgingFee(config.srcChainID, !config.isCurrency),
		operationFee,
		user, receivers, config.isCurrency)

	defaultAmount, sentTokenAmount, waitForAmount := getDefaultSendAmounts(
		t, config, feeAmount, operationFee)

	waitOption := WaitRefundDisabled
	if refundEnabled {
		waitOption = WaitRefundEnabled
	}

	submitMismatchAndWait(t, ctx, apex, config, user, metadata, defaultAmount, sentTokenAmount, waitForAmount,
		waitOption, maxWaitTimeSec, retryIntervalSec, addrIndex, operationFee, true)
}

func executeInvalidColCoin(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, config *testConfig, user *cardanofw.TestApexUser,
	maxWaitTimeSec, retryIntervalSec uint, addrIndex uint8, opts colCoinInvalidOpts, validateTreasury bool,
) {
	t.Helper()

	submitColCoinsMismatchAndWait(
		t, ctx, apex, config, user, opts.receivers, opts.amount,
		maxWaitTimeSec, retryIntervalSec, addrIndex, opts.waitOption, opts.metadataModifier, validateTreasury)
}

func executeInvalidMismatchSendColCoinsMultipleInstancesParalel(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, config *testConfig, amount *big.Int,
	instances int, maxWaitTimeSec, retryIntervalSec uint, refundEnabled bool,
	addrIndex uint8,
) {
	t.Helper()

	var wg sync.WaitGroup

	for i := range instances {
		wg.Add(1)

		go func(idx int) {
			defer wg.Done()

			receivers := createReceivers(apex, 1, config.dstChainID,
				new(big.Int).Mul(amount, big.NewInt(10)), config.tokenID)

			waitOption := WaitRefundDisabled
			if refundEnabled {
				waitOption = WaitRefundEnabled
			}

			submitColCoinsMismatchAndWait(t, ctx, apex, config, apex.Users[idx], receivers, amount,
				maxWaitTimeSec, retryIntervalSec, addrIndex, waitOption, nil, false)
		}(i)
	}

	wg.Wait()
}

func executeInvalidMismatchSendAmountMultipleInstances(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, config *testConfig,
	maxWaitTimeSec, retryIntervalSec uint, refundEnabled bool, addrIndex uint8,
) {
	t.Helper()

	const instances = 5

	for i := 0; i < instances; i++ {
		receivers := createReceivers(apex, 1, config.dstChainID,
			new(big.Int).Mul(defaultSendAmount, big.NewInt(10)), config.tokenID)

		operationFee := apex.GetMinOperationFee(config.srcChainID)

		metadata, feeAmount := createMetadata(t, ctx, apex, config.srcChainID, config.dstChainID,
			apex.GetMinBridgingFee(config.srcChainID, !config.isCurrency),
			operationFee,
			apex.Users[i], receivers, config.isCurrency)

		beforeSendingAmount, err := apex.GetBalance(ctx, apex.Users[i], config.srcChainID)
		require.NoError(t, err)

		defaultAmount, sentTokenAmount, waitForAmount := getDefaultSendAmounts(
			t, config, feeAmount, operationFee)

		txHash, err := apex.SubmitTx(
			ctx, config.srcChainID, apex.Users[i],
			apex.GetCardanoInfo(config.srcChainID).MultisigAddr[addrIndex],
			defaultAmount, sentTokenAmount, metadata, operationFee)
		require.NoError(t, err)

		WaitForInvalidTestResult(t, ctx, apex, config, apex.Users[i], txHash, beforeSendingAmount, waitForAmount,
			refundEnabled, maxWaitTimeSec, retryIntervalSec)
	}
}

func executeInvalidMismatchSendAmountMultipleInstancesParalel(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, config *testConfig,
	maxWaitTimeSec, retryIntervalSec uint, refundEnabled bool,
	addrIndex uint8,
) {
	t.Helper()

	instances := 5

	var wg sync.WaitGroup

	for i := 0; i < instances; i++ {
		wg.Add(1)

		go func(idx int) {
			defer wg.Done()

			receivers := createReceivers(apex, 1, config.dstChainID,
				new(big.Int).Mul(defaultSendAmount, big.NewInt(10)), config.tokenID)

			operationFee := apex.GetMinOperationFee(config.srcChainID)

			metadata, feeAmount := createMetadata(t, ctx, apex, config.srcChainID, config.dstChainID,
				apex.GetMinBridgingFee(config.srcChainID, !config.isCurrency),
				operationFee,
				apex.Users[i], receivers, config.isCurrency)

			beforeSendingAmount, err := apex.GetBalance(ctx, apex.Users[idx], config.srcChainID)
			require.NoError(t, err)

			defaultAmount, sentTokenAmount, waitForAmount := getDefaultSendAmounts(
				t, config, feeAmount, operationFee)

			txHashe, err := apex.SubmitTx(
				ctx, config.srcChainID, apex.Users[idx],
				apex.GetCardanoInfo(config.srcChainID).MultisigAddr[addrIndex],
				defaultAmount, sentTokenAmount, metadata, operationFee)
			require.NoError(t, err)

			WaitForInvalidTestResult(t, ctx, apex, config, apex.Users[idx], txHashe, beforeSendingAmount, waitForAmount,
				refundEnabled, maxWaitTimeSec, retryIntervalSec)
		}(i)
	}

	wg.Wait()
}

func executeInvalidMetadataType(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, config *testConfig, user *cardanofw.TestApexUser,
	maxWaitTimeSec, retryIntervalSec uint, refundEnabled bool, addrIndex uint8,
) {
	t.Helper()

	receivers := createReceivers(apex, 1, config.dstChainID, defaultSendAmount, config.tokenID)

	operationFee := apex.GetMinOperationFee(config.srcChainID)

	metadata, feeAmount := createMetadata(t, ctx, apex, config.srcChainID, config.dstChainID,
		apex.GetMinBridgingFee(config.srcChainID, !config.isCurrency),
		operationFee, user, receivers, config.isCurrency)
	metadata = bytes.Replace(metadata, []byte("bridge"), []byte("xxxxx"), 1)

	beforeSendingAmount, err := apex.GetBalance(ctx, user, config.srcChainID)
	require.NoError(t, err)

	defaultAmount, sentTokenAmount, waitForAmount := getDefaultSendAmounts(
		t, config, feeAmount, operationFee)

	initialTreasuryBalance, err := apex.GetTreasuryAddressBalance(ctx, t, config.srcChainID)
	require.NoError(t, err)

	txHash, err := apex.SubmitTx(
		ctx, config.srcChainID, user, apex.GetCardanoInfo(config.srcChainID).MultisigAddr[addrIndex],
		defaultAmount, sentTokenAmount, metadata, operationFee)
	require.NoError(t, err)

	if refundEnabled {
		WaitForInvalidTestResult(t, ctx, apex, config, user, txHash, beforeSendingAmount, waitForAmount,
			refundEnabled, maxWaitTimeSec, retryIntervalSec)
	} else {
		_, err = cardanofw.WaitForRequestStates(ctx, apex, config.srcChainID, txHash, apex.Config.APIKey, nil, maxWaitTimeSec)
		require.Error(t, err)
		require.ErrorContains(t, err, "timeout")
	}

	if initialTreasuryBalance != nil && operationFee.Cmp(big.NewInt(0)) > 0 {
		err = apex.ValidateTreasuryAddressBalance(ctx, t, config.srcChainID, initialTreasuryBalance, 1)
		require.NoError(t, err)
	}
}

func executeObsoleteMetadata(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, config *testConfig, user *cardanofw.TestApexUser,
	maxWaitTimeSec, retryIntervalSec uint, addrIndex uint8,
) {
	t.Helper()

	receivers := createReceivers(apex, 1, config.dstChainID, defaultSendAmount, config.tokenID)

	operationFee := new(big.Int).Add(apex.GetMinOperationFee(config.srcChainID), big.NewInt(500_000))

	metadata, feeAmount := createObsoleteMetadata(t, ctx, apex, config.srcChainID, config.dstChainID,
		apex.GetMinBridgingFee(config.srcChainID, !config.isCurrency),
		operationFee, user, receivers, config.isCurrency)

	balance, err := apex.GetBalanceWithTokenName(
		ctx, apex.Users[len(apex.Users)-1], config.dstChainID, config.tokensInfo.DstTokenName)
	fmt.Printf("Receiver balance: %+v\n", balance)
	require.NoError(t, err)

	lovelaceAmount, sentTokenAmount, _ := getDefaultSendAmounts(
		t, config, feeAmount, operationFee)

	txHash, err := apex.SubmitTx(
		ctx, config.srcChainID, user, apex.GetCardanoInfo(config.srcChainID).MultisigAddr[addrIndex],
		lovelaceAmount, sentTokenAmount, metadata, operationFee)
	require.NoError(t, err)

	require.NoError(t, err)

	fmt.Printf("Tx sent. hash: %s\n", txHash)

	currentAmount, ok := balance[config.tokensInfo.DstTokenName]
	if !ok {
		currentAmount = big.NewInt(0)
	}

	expectedAmount := new(big.Int).Add(currentAmount, defaultSendAmount)

	numRetries := max(1, int(maxWaitTimeSec/retryIntervalSec))

	err = apex.WaitForExactAmount(ctx, user, config.dstChainID, expectedAmount,
		numRetries, time.Second*time.Duration(retryIntervalSec), config.tokensInfo.DstTokenName)

	require.NoError(t, err)
}

func executeInvalidDestination(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, config *testConfig, user *cardanofw.TestApexUser,
	maxWaitTimeSec, retryIntervalSec uint, refundEnabled bool, addrIndex uint8,
) {
	t.Helper()

	receivers := createReceivers(apex, 0, config.dstChainID, defaultSendAmount, config.tokenID)
	receiversForFeeCalculation := []sendtx.BridgingTxReceiver{
		{
			Addr:    user.GetAddress(config.dstChainID),
			Amount:  cardanofw.WeiToDfm(defaultSendAmount).Uint64(),
			TokenID: config.tokensInfo.SrcTokenID,
		},
	}

	srcTestChain := apex.GetChainMust(t, config.srcChainID)

	operationFee := apex.GetMinOperationFee(config.srcChainID)

	feeAmount, err := srcTestChain.GetBridgingFee(
		ctx, config.dstChainID, receiversForFeeCalculation,
		apex.GetMinBridgingFee(config.srcChainID, !config.isCurrency),
		operationFee, config.srcMultiSigAddr)
	require.NoError(t, err)

	metadata, err := srcTestChain.CreateMetadata(
		user.GetAddress(config.srcChainID), config.dstChainID, receivers, feeAmount,
		operationFee,
	)
	require.NoError(t, err)

	metadata = bytes.Replace(metadata, fmt.Appendf(nil, "\"%s\"", config.dstChainID), []byte("\"unknown\""), 1)

	beforeSendingAmount, err := apex.GetBalanceWithTokenName(
		ctx, user, config.srcChainID, config.tokensInfo.SrcTokenName)
	require.NoError(t, err)

	weiAmount, sentTokenAmount, waitForAmount := getDefaultSendAmounts(
		t, config, feeAmount, operationFee)

	initialTreasuryBalance, err := apex.GetTreasuryAddressBalance(ctx, t, config.srcChainID)
	require.NoError(t, err)

	txHash, err := apex.SubmitTx(
		ctx, config.srcChainID, user, apex.GetCardanoInfo(config.srcChainID).MultisigAddr[addrIndex],
		weiAmount, sentTokenAmount, metadata, operationFee)
	require.NoError(t, err)

	WaitForInvalidTestResult(t, ctx, apex, config, user, txHash, beforeSendingAmount, waitForAmount,
		refundEnabled, maxWaitTimeSec, retryIntervalSec)

	if initialTreasuryBalance != nil && operationFee.Cmp(big.NewInt(0)) > 0 {
		err = apex.ValidateTreasuryAddressBalance(ctx, t, config.srcChainID, initialTreasuryBalance, 1)
		require.NoError(t, err)
	}
}

func executeInvalidMetadataInvalidSender(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, config *testConfig, user *cardanofw.TestApexUser,
	maxWaitTimeSec uint, addrIndex uint8,
) {
	t.Helper()

	receivers := createReceivers(apex, 1, config.dstChainID, defaultSendAmount, config.tokenID)

	srcTestChain := apex.GetChainMust(t, config.srcChainID)

	operationFee := apex.GetMinOperationFee(config.srcChainID)

	feeAmount, err := srcTestChain.GetBridgingFee(
		ctx, config.dstChainID, receivers,
		apex.GetMinBridgingFee(config.srcChainID, !config.isCurrency),
		operationFee, config.srcMultiSigAddr)
	require.NoError(t, err)

	metadata, err := srcTestChain.CreateMetadata(
		"dummy", config.dstChainID, receivers, feeAmount,
		operationFee,
	)
	require.NoError(t, err)

	// remove this after we make correct validation on oracle!
	metadata = bytes.Replace(metadata, []byte("[\"dummy\"]"), []byte("\"\""), 1)

	defaultAmount, sentTokenAmount, _ := getDefaultSendAmounts(t, config, feeAmount, operationFee)

	initialTreasuryBalance, err := apex.GetTreasuryAddressBalance(ctx, t, config.srcChainID)
	require.NoError(t, err)

	txHash, err := apex.SubmitTx(
		ctx, config.srcChainID, user, apex.GetCardanoInfo(config.srcChainID).MultisigAddr[addrIndex],
		defaultAmount, sentTokenAmount, metadata, operationFee)
	require.NoError(t, err)

	fmt.Printf("Tx sent. hash: %s\n", txHash)

	cardanofw.WaitForInvalidState(t, ctx, apex, config.srcChainID, txHash, apex.Config.APIKey, maxWaitTimeSec)

	if initialTreasuryBalance != nil && operationFee.Cmp(big.NewInt(0)) > 0 {
		err = apex.ValidateTreasuryAddressBalance(ctx, t, config.srcChainID, initialTreasuryBalance, 1)
		require.NoError(t, err)
	}
}

func executeInvalidEmptyReceivers(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, config *testConfig, user *cardanofw.TestApexUser,
	maxWaitTimeSec, retryIntervalSec uint, refundEnabled bool, addrIndex uint8,
) {
	t.Helper()

	receivers := []sendtx.BridgingTxReceiver{}

	receiversForFeeCalculation := []sendtx.BridgingTxReceiver{
		{
			Addr:    user.GetAddress(config.dstChainID),
			Amount:  defaultSendAmount.Uint64(),
			TokenID: config.tokensInfo.SrcTokenID,
		},
	}

	srcTestChain := apex.GetChainMust(t, config.srcChainID)

	operationFee := apex.GetMinOperationFee(config.srcChainID)

	feeAmount, err := srcTestChain.GetBridgingFee(
		ctx, config.dstChainID, receiversForFeeCalculation,
		apex.GetMinBridgingFee(config.srcChainID, !config.isCurrency),
		operationFee, config.srcMultiSigAddr)
	require.NoError(t, err)

	metadata, err := srcTestChain.CreateMetadata(
		user.GetAddress(config.srcChainID), config.dstChainID, receivers, feeAmount, operationFee)
	require.NoError(t, err)

	beforeSendingAmount, err := apex.GetBalance(ctx, user, config.srcChainID)
	require.NoError(t, err)

	defaultAmount, sentTokenAmount, waitForAmount := getDefaultSendAmounts(
		t, config, feeAmount, operationFee)

	initialTreasuryBalance, err := apex.GetTreasuryAddressBalance(ctx, t, config.srcChainID)
	require.NoError(t, err)

	txHash, err := apex.SubmitTx(
		ctx, config.srcChainID, user, apex.GetCardanoInfo(config.srcChainID).MultisigAddr[addrIndex],
		defaultAmount, sentTokenAmount, metadata, operationFee)
	require.NoError(t, err)

	WaitForInvalidTestResult(t, ctx, apex, config, user, txHash, beforeSendingAmount, waitForAmount,
		refundEnabled, maxWaitTimeSec, retryIntervalSec)

	if initialTreasuryBalance != nil && operationFee.Cmp(big.NewInt(0)) > 0 {
		err = apex.ValidateTreasuryAddressBalance(ctx, t, config.srcChainID, initialTreasuryBalance, 1)
		require.NoError(t, err)
	}
}

func executeInvalidTokenDirection(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, config *testConfig,
	invalidTokenID uint16, user *cardanofw.TestApexUser,
	maxWaitTimeSec, retryIntervalSec uint, refundEnabled bool, addrIndex uint8,
) {
	t.Helper()

	receivers := createReceivers(apex, 1, config.dstChainID, defaultSendAmount, invalidTokenID)

	operationFee := apex.GetMinOperationFee(config.srcChainID)

	metadata, feeAmount := createMetadata(t, ctx, apex, config.srcChainID, config.dstChainID,
		apex.GetMinBridgingFee(config.srcChainID, false),
		operationFee, user, receivers, config.isCurrency)

	beforeSendingAmount, err := apex.GetBalance(ctx, user, config.srcChainID)
	require.NoError(t, err)

	defaultAmount, sentTokenAmount, waitForAmount := getDefaultSendAmounts(
		t, config, feeAmount, operationFee)

	txHash, err := apex.SubmitTx(
		ctx, config.srcChainID, user, apex.GetCardanoInfo(config.srcChainID).MultisigAddr[addrIndex],
		defaultAmount, sentTokenAmount, metadata, operationFee)
	require.NoError(t, err)

	WaitForInvalidTestResult(t, ctx, apex, config, user, txHash, beforeSendingAmount, waitForAmount,
		refundEnabled, maxWaitTimeSec, retryIntervalSec)
}

func getDefaultSendAmounts(
	t *testing.T, config *testConfig,
	feeAmount *big.Int, operationFee *big.Int,
) (*big.Int, []cardanofw.GenericTokenAmount, *big.Int) {
	t.Helper()

	amount := new(big.Int).Add(feeAmount, defaultSendAmount)

	waitForAmount := new(big.Int)

	var tokens []cardanofw.GenericTokenAmount

	if config.isCurrency {
		waitForAmount.Set(new(big.Int).Add(amount, operationFee))
	} else {
		waitForAmount.Set(defaultSendAmount)
		amount.Set(feeAmount)

		token, err := wallet.NewTokenWithFullName(config.tokensInfo.SrcTokenName, true)
		require.NoError(t, err)

		tokens = []cardanofw.GenericTokenAmount{cardanofw.NewGenericTokenAmount(token, defaultSendAmount)}
	}

	return amount, tokens, waitForAmount
}

func createMetadata(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem,
	srcChain, dstChain cardanofw.ChainID, bridgingFee, operationFee *big.Int,
	sender *cardanofw.TestApexUser, receivers []sendtx.BridgingTxReceiver,
	isCurrency bool,
) ([]byte, *big.Int) {
	t.Helper()

	srcTestChain := apex.GetChainMust(t, srcChain)

	multisig, err := srcTestChain.GetAddressToBridgeTo(ctx, !isCurrency)
	require.NoError(t, err)

	feeAmount, err := srcTestChain.GetBridgingFee(ctx, dstChain, receivers, bridgingFee, operationFee, multisig)
	require.NoError(t, err)

	metadata, err := srcTestChain.CreateMetadata(sender.GetAddress(srcChain), dstChain, receivers, feeAmount, operationFee)
	require.NoError(t, err)

	return metadata, feeAmount
}

func createObsoleteMetadata(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem,
	srcChain, dstChain cardanofw.ChainID, bridgingFee, operationFee *big.Int,
	sender *cardanofw.TestApexUser, receivers []sendtx.BridgingTxReceiver, isCurrency bool,
) ([]byte, *big.Int) {
	t.Helper()

	srcTestChain := apex.GetChainMust(t, srcChain)

	multisig, err := srcTestChain.GetAddressToBridgeTo(ctx, !isCurrency)
	require.NoError(t, err)

	feeAmount, err := srcTestChain.GetBridgingFee(ctx, dstChain, receivers, bridgingFee, operationFee, multisig)
	require.NoError(t, err)

	txs := make([]BridgingRequestMetadataTransactionBC, len(receivers))
	boolToByte := map[bool]byte{true: 1, false: 0}

	for i, x := range receivers {
		txs[i] = BridgingRequestMetadataTransactionBC{
			Address:                     sendtx.AddrToMetaDataAddr(x.Addr),
			IsNativeTokenOnSrc_Obsolete: boolToByte[!isCurrency],
			Amount:                      new(big.Int).SetUint64(x.Amount),
			TokenID:                     0,
		}
	}

	metadata := BridgingRequestMetadataBC{
		BridgingTxType:     bridgingMetaDataType,
		DestinationChainID: dstChain,
		SenderAddr:         sendtx.AddrToMetaDataAddr(sender.GetAddress(srcChain)),
		Transactions:       txs,
		BridgingFee:        cardanofw.WeiToDfm(feeAmount),
		OperationFee:       cardanofw.WeiToDfm(operationFee),
	}

	metadataBytes, err := json.Marshal(map[int]BridgingRequestMetadataBC{
		metadataMapKey: metadata,
	})
	require.NoError(t, err)

	return metadataBytes, feeAmount
}

func createReceivers(
	apex *cardanofw.ApexSystem, receiversCount int, dstChain string, sendAmount *big.Int, tokenID uint16,
) []sendtx.BridgingTxReceiver {
	receivers := make([]sendtx.BridgingTxReceiver, receiversCount)

	for i := range receivers {
		receivers[i] = sendtx.BridgingTxReceiver{
			Addr:    apex.Users[len(apex.Users)-1-i].GetAddress(dstChain),
			Amount:  cardanofw.WeiToDfm(sendAmount).Uint64(),
			TokenID: tokenID,
		}
	}

	return receivers
}

// executeBridgingWithAuxDataVariant submits a bridging request whose metadata json and
// whose built transaction are each passed through an optional hook, then requires it to
// bridge through successfully.
//
// The point of every caller is the same: what rides alongside the bridging request in
// auxiliary_data - which of the three envelopes babbage accepts wraps it, native
// scripts, other metadata labels - must not decide whether the request is readable. The
// oracle sees the auxiliary_data cbor exactly as it sits in the block, so a decoder
// stricter than the ledger silently drops requests the chain accepted.
func executeBridgingWithAuxDataVariant(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, config *testConfig,
	user *cardanofw.TestApexUser, maxWaitTimeSec, retryIntervalSec uint, addrIndex uint8,
	mutateMetadata func(metadataJSON []byte) ([]byte, error),
	mutateRawTx func(txRaw []byte) ([]byte, error),
) {
	t.Helper()

	receivers := createReceivers(apex, 1, config.dstChainID, defaultSendAmount, config.tokenID)
	receiver := apex.Users[len(apex.Users)-1]

	operationFee := apex.GetMinOperationFee(config.srcChainID)

	metadata, feeAmount := createMetadata(t, ctx, apex, config.srcChainID, config.dstChainID,
		apex.GetMinBridgingFee(config.srcChainID, !config.isCurrency),
		operationFee, user, receivers, config.isCurrency)

	if mutateMetadata != nil {
		var err error

		metadata, err = mutateMetadata(metadata)
		require.NoError(t, err)
	}

	balanceBefore, err := apex.GetBalanceWithTokenName(
		ctx, receiver, config.dstChainID, config.tokensInfo.DstTokenName)
	require.NoError(t, err)

	lovelaceAmount, sentTokenAmount, _ := getDefaultSendAmounts(t, config, feeAmount, operationFee)

	// the envelope the oracle will have to read is decided here, after the tx is built
	var submittedAuxData []byte

	rewrite := mutateRawTx
	if rewrite != nil {
		rewrite = func(txRaw []byte) ([]byte, error) {
			rewritten, err := mutateRawTx(txRaw)
			if err != nil {
				return nil, err
			}

			if submittedAuxData, err = cardanofw.TxAuxiliaryData(rewritten); err != nil {
				return nil, err
			}

			return rewritten, nil
		}
	}

	txHash, err := apex.SubmitTxWithRawTxMutator(
		ctx, config.srcChainID, user, apex.GetCardanoInfo(config.srcChainID).MultisigAddr[addrIndex],
		lovelaceAmount, sentTokenAmount, metadata, operationFee, rewrite)
	require.NoError(t, err)

	if submittedAuxData != nil {
		fmt.Printf("Tx sent. hash: %s, auxiliary_data envelope: %s\n",
			txHash, cardanofw.AuxiliaryDataEnvelopeName(submittedAuxData))
	} else {
		fmt.Printf("Tx sent. hash: %s\n", txHash)
	}

	currentAmount, ok := balanceBefore[config.tokensInfo.DstTokenName]
	if !ok {
		currentAmount = big.NewInt(0)
	}

	expectedAmount := new(big.Int).Add(currentAmount, defaultSendAmount)

	numRetries := max(1, int(maxWaitTimeSec/retryIntervalSec))

	require.NoError(t, apex.WaitForExactAmount(ctx, receiver, config.dstChainID, expectedAmount,
		numRetries, time.Second*time.Duration(retryIntervalSec), config.tokensInfo.DstTokenName))
}

// executePhase2InvalidTxs has attacker send every source's bridging address, one scenario
// at a time, a script tx that fails phase-2, then has controlSender send each of them the
// same tx with a policy that succeeds. See TestE2E_SkylineBridge_Phase2InvalidTxs for why.
//
// It requires the bridge to act as if the failed txs had never been sent - no bridging
// request state for them, none of their declared outputs among the bridge utxos, nothing
// paid out to victim and nothing refunded to attacker - and the control txs to bridge
// normally. Sources run in parallel; attacker must hold every token its source bridges.
func executePhase2InvalidTxs(
	t *testing.T, ctx context.Context, apex *cardanofw.ApexSystem, sources []*testConfig,
	attacker, victim, controlSender, controlReceiver *cardanofw.TestApexUser,
	maxWaitTimeSec, retryIntervalSec uint,
) {
	t.Helper()

	type scenario struct {
		name string
		// metadata is what the tx carries, given the bridging request a valid tx from the
		// same source would carry. nil means no metadata at all.
		metadata           func(cfg *testConfig, bridgingRequest []byte) []byte
		collateralToBridge bool
		mutateRawTx        func(txRaw []byte) ([]byte, error)
	}

	type sentTx struct {
		scenario string
		info     *cardanofw.ScriptTxInfo
	}

	// collateral comes from the smallest pure ada utxo able to carry it. Setting a small one
	// aside, for the first scenario to take, caps what returning collateral to the bridging
	// address costs.
	for _, cfg := range sources {
		_, err := apex.SubmitTx(ctx, cfg.srcChainID, attacker, attacker.GetAddress(cfg.srcChainID),
			cardanofw.ApexToWei(big.NewInt(3)), nil, nil, nil)
		require.NoError(t, err)
	}

	receiversFor := func(
		cfg *testConfig, receiver *cardanofw.TestApexUser, amount *big.Int,
	) []sendtx.BridgingTxReceiver {
		return []sendtx.BridgingTxReceiver{
			{
				Addr:    receiver.GetAddress(cfg.dstChainID),
				Amount:  cardanofw.WeiToDfm(amount).Uint64(),
				TokenID: cfg.tokenID,
			},
		}
	}

	bridgingRequestFor := func(
		cfg *testConfig, sender, receiver *cardanofw.TestApexUser, amount *big.Int,
	) ([]byte, *big.Int) {
		return createMetadata(t, ctx, apex, cfg.srcChainID, cfg.dstChainID,
			apex.GetMinBridgingFee(cfg.srcChainID, !cfg.isCurrency), apex.GetMinOperationFee(cfg.srcChainID),
			sender, receiversFor(cfg, receiver, amount), cfg.isCurrency)
	}

	metadataOfType := func(fields map[string]interface{}) []byte {
		metadata, err := json.Marshal(map[int]map[string]interface{}{metadataMapKey: fields})
		require.NoError(t, err)

		return metadata
	}

	asIs := func(_ *testConfig, bridgingRequest []byte) []byte {
		return bridgingRequest
	}

	scenarios := []scenario{
		{
			// first, so that it takes the small utxo set aside above
			name:               "bridging request, collateral returned to the bridging address",
			metadata:           asIs,
			collateralToBridge: true,
		},
		{
			name:     "bridging request",
			metadata: asIs,
		},
		{
			name:     "bridging request in shelley-ma envelope",
			metadata: asIs,
			mutateRawTx: func(txRaw []byte) ([]byte, error) {
				return cardanofw.RewriteTxAuxiliaryDataToShelleyMA(txRaw, nil)
			},
		},
		{
			name: "bridging request in obsolete metadata format",
			metadata: func(cfg *testConfig, _ []byte) []byte {
				metadata, _ := createObsoleteMetadata(t, ctx, apex, cfg.srcChainID, cfg.dstChainID,
					apex.GetMinBridgingFee(cfg.srcChainID, !cfg.isCurrency), apex.GetMinOperationFee(cfg.srcChainID),
					attacker, receiversFor(cfg, victim, defaultSendAmount), cfg.isCurrency)

				return metadata
			},
		},
		{
			// invalid as a bridging request, so a valid tx like it would be refunded
			name: "bridging request for more than is sent",
			metadata: func(cfg *testConfig, _ []byte) []byte {
				metadata, _ := bridgingRequestFor(
					cfg, attacker, victim, new(big.Int).Mul(defaultSendAmount, big.NewInt(10)))

				return metadata
			},
		},
		{
			name: "refund request",
			metadata: func(cfg *testConfig, _ []byte) []byte {
				return metadataOfType(map[string]interface{}{
					"t": "refund",
					"s": sendtx.AddrToMetaDataAddr(attacker.GetAddress(cfg.srcChainID)),
					"d": cfg.dstChainID,
				})
			},
		},
		{
			// the oracle hands txs of a type it does not know to the refund processor
			name: "unknown tx type",
			metadata: func(_ *testConfig, _ []byte) []byte {
				return metadataOfType(map[string]interface{}{"t": "unknown"})
			},
		},
		{
			name: "batch execution",
			metadata: func(_ *testConfig, _ []byte) []byte {
				return metadataOfType(map[string]interface{}{"t": "batch", "n": 1})
			},
		},
		{
			name: "hot wallet funding",
			metadata: func(_ *testConfig, _ []byte) []byte {
				return metadataOfType(map[string]interface{}{"t": "fund"})
			},
		},
		{
			// the oracle hands txs without metadata to the hot wallet funding processor
			name: "no metadata",
		},
	}

	var (
		lovelaces       = make([]*big.Int, len(sources))
		tokens          = make([][]cardanofw.GenericTokenAmount, len(sources))
		metadatas       = make([][][]byte, len(sources))
		attackerBalance = make([]map[string]*big.Int, len(sources))
		victimBalance   = make([]map[string]*big.Int, len(sources))
	)

	for i, cfg := range sources {
		bridgingRequest, feeAmount := bridgingRequestFor(cfg, attacker, victim, defaultSendAmount)

		// every tx sends what the valid bridging request asks for, whatever its metadata says
		lovelaces[i], tokens[i], _ = getDefaultSendAmounts(t, cfg, feeAmount, apex.GetMinOperationFee(cfg.srcChainID))

		for _, sc := range scenarios {
			var metadata []byte
			if sc.metadata != nil {
				metadata = sc.metadata(cfg, bridgingRequest)
			}

			metadatas[i] = append(metadatas[i], metadata)
		}

		var err error

		attackerBalance[i], err = apex.GetBalance(ctx, attacker, cfg.srcChainID)
		require.NoError(t, err)

		victimBalance[i], err = apex.GetBalance(ctx, victim, cfg.dstChainID)
		require.NoError(t, err)
	}

	var (
		sent = make([][]sentTx, len(sources))
		errs = make([]error, len(sources))
		wg   sync.WaitGroup
	)

	for i, cfg := range sources {
		wg.Add(1)

		go func(i int, cfg *testConfig) {
			defer wg.Done()

			// one at a time: every tx takes the collateral the previous one returned
			for j, sc := range scenarios {
				returnAddr := ""
				if sc.collateralToBridge {
					returnAddr = cfg.srcMultiSigAddr
				}

				info, err := apex.SubmitScriptTx(
					ctx, cfg.srcChainID, attacker, cfg.srcMultiSigAddr, lovelaces[i], tokens[i], metadatas[i][j],
					apex.GetMinOperationFee(cfg.srcChainID), cardanofw.ScriptTxConfig{
						ScriptFails:          true,
						CollateralReturnAddr: returnAddr,
						MutateRawTx:          sc.mutateRawTx,
					})
				if err != nil {
					errs[i] = fmt.Errorf("%s -> %s, %s: %w", cfg.srcChainID, cfg.dstChainID, sc.name, err)

					return
				}

				fmt.Printf("%s -> %s, %s: failed tx %s is on chain\n", cfg.srcChainID, cfg.dstChainID, sc.name, info.TxHash)

				sent[i] = append(sent[i], sentTx{scenario: sc.name, info: info})
			}
		}(i, cfg)
	}

	wg.Wait()

	require.NoError(t, errors.Join(errs...))

	var (
		controls        = make([]*cardanofw.ScriptTxInfo, len(sources))
		controlBalances = make([]*big.Int, len(sources))
	)

	for i, cfg := range sources {
		metadata, feeAmount := bridgingRequestFor(cfg, controlSender, controlReceiver, defaultSendAmount)
		lovelace, sentTokens, _ := getDefaultSendAmounts(t, cfg, feeAmount, apex.GetMinOperationFee(cfg.srcChainID))

		balance, err := apex.GetBalanceWithTokenName(ctx, controlReceiver, cfg.dstChainID, cfg.tokensInfo.DstTokenName)
		require.NoError(t, err)

		controlBalances[i] = balance[cfg.tokensInfo.DstTokenName]
		if controlBalances[i] == nil {
			controlBalances[i] = big.NewInt(0)
		}

		controls[i], err = apex.SubmitScriptTx(
			ctx, cfg.srcChainID, controlSender, cfg.srcMultiSigAddr, lovelace, sentTokens, metadata,
			apex.GetMinOperationFee(cfg.srcChainID), cardanofw.ScriptTxConfig{})
		require.NoError(t, err)

		fmt.Printf("%s -> %s, control tx %s is on chain\n", cfg.srcChainID, cfg.dstChainID, controls[i].TxHash)
	}

	apiKey := apex.Config.APIKey

	apiURLs, err := apex.GetBridgingAPIs()
	require.NoError(t, err)

	// checked before the control txs are waited on to execute: a bridge that took the failed
	// txs in tries to spend their outputs and stalls, which would hide why
	for _, apiURL := range apiURLs {
		for i, cfg := range sources {
			// the control tx came after all the failed ones, so an oracle that has it has
			// already read every one of them
			waitForRequestState(t, ctx, apiURL, apiKey, cfg.srcChainID, controls[i].TxHash)

			oracleState, err := cardanofw.GetOracleState(
				ctx, fmt.Sprintf("%s/api/OracleState/Get?chainId=%s", apiURL, cfg.srcChainID), apiKey)
			require.NoError(t, err)

			for _, tx := range sent[i] {
				_, err := cardanofw.GetBridgingRequestState(
					ctx, requestStateURL(apiURL, cfg.srcChainID, tx.info.TxHash), apiKey)
				require.ErrorContains(t, err, fmt.Sprintf("code is %d", http.StatusNotFound),
					"%s: %s has a bridging request state for failed tx %s", tx.scenario, apiURL, tx.info.TxHash)

				for _, utxo := range oracleState.Utxos {
					// the collateral return is the one output a failed tx really creates
					if hex.EncodeToString(utxo.Hash[:]) == tx.info.TxHash {
						require.Equal(t, tx.info.OutputsCount, utxo.Index,
							"%s: %s holds output %d of failed tx %s among the bridge utxos",
							tx.scenario, apiURL, utxo.Index, tx.info.TxHash)
					}
				}
			}
		}
	}

	numRetries := max(1, int(maxWaitTimeSec/retryIntervalSec))

	for i, cfg := range sources {
		_, err := cardanofw.WaitForRequestStates(ctx, apex, cfg.srcChainID, controls[i].TxHash, apiKey,
			[]string{"ExecutedOnDestination"}, maxWaitTimeSec)
		require.NoError(t, err)

		require.NoError(t, apex.WaitForExactAmount(ctx, controlReceiver, cfg.dstChainID,
			new(big.Int).Add(controlBalances[i], defaultSendAmount), numRetries, time.Second*time.Duration(retryIntervalSec),
			cfg.tokensInfo.DstTokenName))
	}

	for _, apiURL := range apiURLs {
		for i, cfg := range sources {
			hasFailed, err := cardanofw.GetHasTxFailed(ctx, hasTxFailedURL(apiURL, cfg.srcChainID, controls[i]), apiKey)
			require.NoError(t, err)
			require.False(t, hasFailed.Failed, "%s reports control tx %s as failed", apiURL, controls[i].TxHash)

			for _, tx := range sent[i] {
				waitForHasTxFailed(t, ctx, apiURL, apiKey, cfg.srcChainID, tx.info,
					numRetries, time.Second*time.Duration(retryIntervalSec))
			}
		}
	}

	for i, cfg := range sources {
		lost := big.NewInt(0)

		for _, tx := range sent[i] {
			loss := tx.info.TotalCollateral
			if tx.info.CollateralReturnAddr != attacker.GetAddress(cfg.srcChainID) {
				loss = tx.info.CollateralAmount
			}

			lost.Add(lost, new(big.Int).SetUint64(loss))
		}

		expected := maps.Clone(attackerBalance[i])
		expected[wallet.AdaTokenName] = new(big.Int).Sub(expected[wallet.AdaTokenName], cardanofw.DfmToWei(lost))

		balance, err := apex.GetBalance(ctx, attacker, cfg.srcChainID)
		require.NoError(t, err)

		// the collateral is all the failed txs cost, and nothing came back as a refund
		requireSameBalances(t, expected, balance, fmt.Sprintf("attacker on %s", cfg.srcChainID))

		balance, err = apex.GetBalance(ctx, victim, cfg.dstChainID)
		require.NoError(t, err)

		requireSameBalances(t, victimBalance[i], balance, fmt.Sprintf("receiver on %s", cfg.dstChainID))
	}
}

// waitForRequestState waits for the oracle behind apiURL to have a bridging request state,
// any state, for txHash.
func waitForRequestState(
	t *testing.T, ctx context.Context, apiURL, apiKey string, chainID cardanofw.ChainID, txHash string,
) {
	t.Helper()

	_, err := infracommon.ExecuteWithRetry(ctx, func(ctx context.Context) (bool, error) {
		if _, err := cardanofw.GetBridgingRequestState(ctx, requestStateURL(apiURL, chainID, txHash), apiKey); err != nil {
			return false, infracommon.ErrRetryTryAgain
		}

		return true, nil
	}, infracommon.WithRetryCount(60), infracommon.WithRetryWaitTime(time.Second*2))
	require.NoError(t, err, "%s has no bridging request state for tx %s", apiURL, txHash)
}

func requestStateURL(apiURL string, chainID cardanofw.ChainID, txHash string) string {
	return fmt.Sprintf("%s/api/BridgingRequestState/Get?chainId=%s&txHash=%s", apiURL, chainID, txHash)
}

// waitForHasTxFailed waits for the oracle behind apiURL to report tx as failed, which it
// does once it has indexed past the tx's ttl without having seen the tx. The ttl is a
// slot count, so on a chain with long slots - vector's are ten times prime's - that
// takes minutes.
func waitForHasTxFailed(
	t *testing.T, ctx context.Context, apiURL, apiKey string, chainID cardanofw.ChainID, tx *cardanofw.ScriptTxInfo,
	numRetries int, retryWaitTime time.Duration,
) {
	t.Helper()

	_, err := infracommon.ExecuteWithRetry(ctx, func(ctx context.Context) (bool, error) {
		hasFailed, err := cardanofw.GetHasTxFailed(ctx, hasTxFailedURL(apiURL, chainID, tx), apiKey)
		if err != nil {
			return false, err
		}

		if !hasFailed.Failed {
			return false, infracommon.ErrRetryTryAgain
		}

		return true, nil
	}, infracommon.WithRetryCount(numRetries), infracommon.WithRetryWaitTime(retryWaitTime))
	require.NoError(t, err, "%s does not report failed tx %s as failed", apiURL, tx.TxHash)
}

func hasTxFailedURL(apiURL string, chainID cardanofw.ChainID, tx *cardanofw.ScriptTxInfo) string {
	return fmt.Sprintf("%s/api/OracleState/GetHasTxFailed?chainId=%s&txHash=%s&ttl=%d",
		apiURL, chainID, tx.TxHash, tx.TTL)
}

// requireSameBalances compares balances token by token, a missing token counting as zero.
func requireSameBalances(t *testing.T, expected, actual map[string]*big.Int, whose string) {
	t.Helper()

	tokenNames := slices.Collect(maps.Keys(expected))

	for tokenName := range actual {
		if _, exists := expected[tokenName]; !exists {
			tokenNames = append(tokenNames, tokenName)
		}
	}

	for _, tokenName := range tokenNames {
		expectedAmount, actualAmount := expected[tokenName], actual[tokenName]
		if expectedAmount == nil {
			expectedAmount = big.NewInt(0)
		}

		if actualAmount == nil {
			actualAmount = big.NewInt(0)
		}

		require.Zero(t, expectedAmount.Cmp(actualAmount),
			"%s %s: expected %s, got %s", whose, tokenName, expectedAmount, actualAmount)
	}
}
