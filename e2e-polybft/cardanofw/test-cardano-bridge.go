package cardanofw

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math/big"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/framework"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/Ethernal-Tech/cardano-infrastructure/wallet"
	"github.com/stretchr/testify/require"
)

const (
	ChainIDPrime  = "prime"
	ChainIDVector = "vector"
	ChainIDNexus  = "nexus"

	BridgeSCAddr = "0xABEF000000000000000000000000000000000000"

	RunRelayerOnValidatorID = 1
)

type CardanoBridgeOption func(*TestCardanoBridge)

type TestCardanoBridge struct {
	dataDirPath string

	validators  []*TestCardanoValidator
	relayerNode *framework.Node

	primeMultisigKeys     []string
	primeMultisigFeeKeys  []string
	vectorMultisigKeys    []string
	vectorMultisigFeeKeys []string

	PrimeMultisigAddr     string
	PrimeMultisigFeeAddr  string
	VectorMultisigAddr    string
	VectorMultisigFeeAddr string

	relayerWallet *crypto.ECDSAKey

	cluster *framework.TestCluster

	config *ApexSystemConfig
}

func NewTestCardanoBridge(
	dataDirPath string, apexSystemConfig *ApexSystemConfig,
) *TestCardanoBridge {

	validators := make([]*TestCardanoValidator, apexSystemConfig.BladeValidatorCount)

	for i := 0; i < apexSystemConfig.BladeValidatorCount; i++ {
		validators[i] = NewTestCardanoValidator(dataDirPath, i+1)
	}

	bridge := &TestCardanoBridge{
		dataDirPath: dataDirPath,
		validators:  validators,
		config:      apexSystemConfig,
	}

	return bridge
}

func (cb *TestCardanoBridge) CardanoCreateWalletsAndAddresses(
	primeNetworkConfig TestCardanoNetworkConfig, vectorNetworkConfig TestCardanoNetworkConfig,
) error {
	if err := cb.cardanoCreateWallets(); err != nil {
		return err
	}

	if err := cb.cardanoPrepareKeys(); err != nil {
		return err
	}

	return cb.cardanoCreateAddresses(primeNetworkConfig, vectorNetworkConfig)
}

func (cb *TestCardanoBridge) StartValidators(t *testing.T, epochSize int) {
	t.Helper()

	cb.cluster = framework.NewTestCluster(t, cb.config.BladeValidatorCount,
		framework.WithEpochSize(epochSize),
	)

	for idx, validator := range cb.validators {
		require.NoError(t, validator.SetClusterAndServer(cb.cluster, cb.cluster.Servers[idx]))
	}

	err := cb.nexusCreateWalletsAndAddresses()
	require.NoError(t, err)
}

func (ec *TestCardanoBridge) nexusCreateWalletsAndAddresses() (err error) {
	if !ec.config.NexusEnabled || len(ec.validators) == 0 {
		return nil
	}

	// relayer is on the first validator only
	relayerValidator := ec.validators[0]

	if err = relayerValidator.createSpecificWallet("relayer-evm"); err != nil {
		return err
	}

	ec.relayerWallet, err = relayerValidator.getRelayerWallet()
	if err != nil {
		return err
	}

	for _, validator := range ec.validators {
		if err = validator.createSpecificWallet("batcher-evm"); err != nil {
			return err
		}

		validator.BatcherBN256PrivateKey, err = validator.getBatcherWallet()
		if err != nil {
			return err
		}
	}

	return err
}

func (cb *TestCardanoBridge) GetRelayerWalletAddr() types.Address {
	return cb.relayerWallet.Address()
}

func (cb *TestCardanoBridge) GetValidator(t *testing.T, idx int) *TestCardanoValidator {
	t.Helper()

	require.True(t, idx >= 0 && idx < len(cb.validators))

	return cb.validators[idx]
}

func (cb *TestCardanoBridge) WaitForValidatorsReady(t *testing.T) {
	t.Helper()

	cb.cluster.WaitForReady(t)
}

func (cb *TestCardanoBridge) StopValidators() {
	if cb.cluster != nil {
		cb.cluster.Stop()
	}
}

func (cb *TestCardanoBridge) GetValidatorsCount() int {
	return len(cb.validators)
}

func (cb *TestCardanoBridge) RegisterChains(
	primeTokenSupply *big.Int,
	vectorTokenSupply *big.Int,
	nexusTokenSupply *big.Int,
	apex *ApexSystem,
) error {
	errs := make([]error, len(cb.validators))
	wg := sync.WaitGroup{}

	wg.Add(len(cb.validators))

	for i, validator := range cb.validators {
		go func(validator *TestCardanoValidator, indx int) {
			defer wg.Done()

			errs[indx] = validator.RegisterChain(
				ChainIDPrime, cb.PrimeMultisigAddr, cb.PrimeMultisigFeeAddr, primeTokenSupply, ChainTypeCardano)
			if errs[indx] != nil {
				return
			}

			if cb.config.VectorEnabled {
				errs[indx] = validator.RegisterChain(
					ChainIDVector, cb.VectorMultisigAddr, cb.VectorMultisigFeeAddr, vectorTokenSupply, ChainTypeCardano)
				if errs[indx] != nil {
					return
				}
			}

			if cb.config.NexusEnabled {
				errs[indx] = validator.RegisterChain(
					ChainIDNexus,
					apex.Nexus.contracts.gateway.String(), cb.GetRelayerWalletAddr().String(),
					nexusTokenSupply, ChainTypeEVM)
				if errs[indx] != nil {
					return
				}
			}
		}(validator, i)
	}

	wg.Wait()

	return errors.Join(errs...)
}

func (cb *TestCardanoBridge) GenerateConfigs(
	primeCluster *TestCardanoCluster,
	vectorCluster *TestCardanoCluster,
	nexus *TestEVMBridge,
) error {
	errs := make([]error, len(cb.validators))
	wg := sync.WaitGroup{}

	wg.Add(len(cb.validators))

	for i, validator := range cb.validators {
		go func(validator *TestCardanoValidator, indx int) {
			defer wg.Done()

			telemetryConfig := ""
			if indx == 0 {
				telemetryConfig = cb.config.TelemetryConfig
			}

			var (
				primeNetworkURL    string
				vectorNetworkURL   string
				primeNetworkMagic  uint
				vectorNetworkMagic uint
				primeNetworkID     uint
				vectorNetworkID    uint

				nexusContractAddr  string
				nexusRelayerWallet string
				nexusNodeURL       string
			)

			if cb.config.TargetOneCardanoClusterServer {
				primeNetworkURL = primeCluster.NetworkURL()
				vectorNetworkURL = vectorCluster.NetworkURL()
				primeNetworkMagic = primeCluster.Config.NetworkMagic
				vectorNetworkMagic = vectorCluster.Config.NetworkMagic
				primeNetworkID = uint(primeCluster.Config.NetworkType)
				vectorNetworkID = uint(vectorCluster.Config.NetworkType)
			} else {
				primeServer := primeCluster.Servers[indx%len(primeCluster.Servers)]
				vectorServer := vectorCluster.Servers[indx%len(vectorCluster.Servers)]
				primeNetworkURL = primeServer.NetworkURL()
				vectorNetworkURL = vectorServer.NetworkURL()
				primeNetworkMagic = primeServer.config.NetworkMagic
				vectorNetworkMagic = vectorServer.config.NetworkMagic
				primeNetworkID = uint(primeServer.config.NetworkID)
				vectorNetworkID = uint(vectorServer.config.NetworkID)
			}

			if cb.config.NexusEnabled {
				nexusContractAddr = nexus.contracts.gateway.String()
				nexusRelayerWallet = cb.GetRelayerWalletAddr().String()

				nexusNodeURLIndx := 0
				if cb.config.TargetOneCardanoClusterServer {
					nexusNodeURLIndx = indx % len(nexus.Cluster.Servers)
				}

				nexusNodeURL = nexus.Cluster.Servers[nexusNodeURLIndx].JSONRPCAddr()
			} else {
				nexusContractAddr = types.ZeroAddress.String()
				nexusRelayerWallet = types.ZeroAddress.String()
				nexusNodeURL = "localhost:5500"
			}

			errs[indx] = validator.GenerateConfigs(
				primeNetworkURL,
				primeNetworkMagic,
				primeNetworkID,
				primeCluster.OgmiosURL(),
				cb.config.PrimeSlotRoundingThreshold,
				cb.config.PrimeTTLInc,
				vectorNetworkURL,
				vectorNetworkMagic,
				vectorNetworkID,
				vectorCluster.OgmiosURL(),
				cb.config.VectorSlotRoundingThreshold,
				cb.config.VectorTTLInc,
				cb.config.APIPortStart+indx,
				cb.config.APIKey,
				telemetryConfig,
				nexusContractAddr,
				nexusRelayerWallet,
				nexusNodeURL,
			)
		}(validator, i)
	}

	wg.Wait()

	return errors.Join(errs...)
}

func (cb *TestCardanoBridge) StartValidatorComponents(ctx context.Context) (err error) {
	for _, validator := range cb.validators {
		hasAPI := cb.config.APIValidatorID == -1 || validator.ID == cb.config.APIValidatorID

		if err = validator.Start(ctx, hasAPI); err != nil {
			return err
		}
	}

	return err
}

func (cb *TestCardanoBridge) StartRelayer(ctx context.Context) (err error) {
	for _, validator := range cb.validators {
		if RunRelayerOnValidatorID != validator.ID {
			continue
		}

		cb.relayerNode, err = framework.NewNodeWithContext(ctx, ResolveApexBridgeBinary(), []string{
			"run-relayer",
			"--config", validator.GetRelayerConfig(),
		}, os.Stdout)
		if err != nil {
			return err
		}
	}

	return nil
}

func (cb TestCardanoBridge) StopRelayer() error {
	if cb.relayerNode == nil {
		return errors.New("relayer not started")
	}

	return cb.relayerNode.Stop()
}

func (cb *TestCardanoBridge) GetBridgingAPI() (string, error) {
	apis, err := cb.GetBridgingAPIs()
	if err != nil {
		return "", err
	}

	return apis[0], nil
}

func (cb *TestCardanoBridge) GetBridgingAPIs() (res []string, err error) {
	for _, validator := range cb.validators {
		hasAPI := cb.config.APIValidatorID == -1 || validator.ID == cb.config.APIValidatorID

		if hasAPI {
			if validator.APIPort == 0 {
				return nil, fmt.Errorf("api port not defined")
			}

			res = append(res, fmt.Sprintf("http://localhost:%d", validator.APIPort))
		}
	}

	if len(res) == 0 {
		return nil, fmt.Errorf("not running API")
	}

	return res, nil
}

func (cb *TestCardanoBridge) cardanoCreateWallets() (err error) {
	for _, validator := range cb.validators {
		err = validator.CardanoWalletCreate(ChainIDPrime)
		if err != nil {
			return err
		}

		err = validator.CardanoWalletCreate(ChainIDVector)
		if err != nil {
			return err
		}
	}

	return err
}

func (cb *TestCardanoBridge) cardanoPrepareKeys() (err error) {
	validatorCount := cb.config.BladeValidatorCount
	cb.primeMultisigKeys = make([]string, validatorCount)
	cb.primeMultisigFeeKeys = make([]string, validatorCount)
	cb.vectorMultisigKeys = make([]string, validatorCount)
	cb.vectorMultisigFeeKeys = make([]string, validatorCount)

	for idx, validator := range cb.validators {
		primeWallet, err := validator.GetCardanoWallet(ChainIDPrime)
		if err != nil {
			return err
		}

		cb.primeMultisigKeys[idx] = hex.EncodeToString(primeWallet.Multisig.GetVerificationKey())
		cb.primeMultisigFeeKeys[idx] = hex.EncodeToString(primeWallet.MultisigFee.GetVerificationKey())

		vectorWallet, err := validator.GetCardanoWallet(ChainIDVector)
		if err != nil {
			return err
		}

		cb.vectorMultisigKeys[idx] = hex.EncodeToString(vectorWallet.Multisig.GetVerificationKey())
		cb.vectorMultisigFeeKeys[idx] = hex.EncodeToString(vectorWallet.MultisigFee.GetVerificationKey())
	}

	return err
}

func (cb *TestCardanoBridge) cardanoCreateAddresses(
	primeNetworkConfig TestCardanoNetworkConfig, vectorNetworkConfig TestCardanoNetworkConfig,
) error {
	errs := make([]error, 4)
	wg := sync.WaitGroup{}

	wg.Add(4)

	go func() {
		defer wg.Done()

		cb.PrimeMultisigAddr, errs[0] = cb.cardanoCreateAddress(primeNetworkConfig.NetworkType, cb.primeMultisigKeys)
	}()

	go func() {
		defer wg.Done()

		cb.PrimeMultisigFeeAddr, errs[1] = cb.cardanoCreateAddress(primeNetworkConfig.NetworkType, cb.primeMultisigFeeKeys)
	}()

	go func() {
		defer wg.Done()

		cb.VectorMultisigAddr, errs[2] = cb.cardanoCreateAddress(vectorNetworkConfig.NetworkType, cb.vectorMultisigKeys)
	}()

	go func() {
		defer wg.Done()

		cb.VectorMultisigFeeAddr, errs[3] = cb.cardanoCreateAddress(vectorNetworkConfig.NetworkType, cb.vectorMultisigFeeKeys)
	}()

	wg.Wait()

	return errors.Join(errs...)
}

func (cb *TestCardanoBridge) cardanoCreateAddress(network wallet.CardanoNetworkType, keys []string) (string, error) {
	args := []string{
		"create-address",
		"--network-id", fmt.Sprint(network),
	}

	for _, key := range keys {
		args = append(args, "--key", key)
	}

	var outb bytes.Buffer

	err := RunCommand(ResolveApexBridgeBinary(), args, io.MultiWriter(os.Stdout, &outb))
	if err != nil {
		return "", err
	}

	result := strings.TrimSpace(strings.ReplaceAll(outb.String(), "Address = ", ""))

	return result, nil
}
