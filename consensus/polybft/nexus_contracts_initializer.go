package polybft

// import (
// 	"fmt"

// 	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
// 	"github.com/0xPolygon/polygon-edge/contracts"
// 	"github.com/0xPolygon/polygon-edge/state"
// 	"github.com/0xPolygon/polygon-edge/types"
// )

// func initNexus(transition *state.Transition, admin types.Address,
// 	polyBFTConfig PolyBFTConfig) (err error) {
// 	// Initialize Nexus proxies
// 	if err = initNexusProxies(transition, admin,
// 		contracts.GetNexusProxyImplementationMapping(), polyBFTConfig); err != nil {
// 		return err
// 	}

// 	// Initialize Nexus contracts

// 	if err = initTestContract(transition); err != nil {
// 		return err
// 	}

// 	return nil
// }

// // initProxies initializes proxy contracts, that allow upgradeability of contracts implementation
// func initNexusProxies(transition *state.Transition, admin types.Address,
// 	proxyToImplMap map[types.Address]types.Address, polyBFTConfig PolyBFTConfig) error {
// 	for proxyAddress, implAddress := range proxyToImplMap {
// 		protectSetupProxyFn := &contractsapi.ProtectSetUpProxyGenesisProxyFn{Initiator: contracts.SystemCaller}

// 		proxyInput, err := protectSetupProxyFn.EncodeAbi()
// 		if err != nil {
// 			return fmt.Errorf("GenesisProxy.protectSetUpProxy params encoding failed: %w", err)
// 		}

// 		err = callContract(contracts.SystemCaller, proxyAddress, proxyInput, "GenesisProxy.protectSetUpProxy", transition)
// 		if err != nil {
// 			return err
// 		}

// 		_ = polyBFTConfig
// 		data, err := getDataForNexusContract(proxyAddress)
// 		if err != nil {
// 			return fmt.Errorf("initialize encoding for %v proxy failed: %w", proxyAddress, err)
// 		}

// 		setUpproxyFn := &contractsapi.SetUpProxyGenesisProxyFn{
// 			Logic: implAddress,
// 			Admin: admin,
// 			Data:  data,
// 		}

// 		proxyInput, err = setUpproxyFn.EncodeAbi()
// 		if err != nil {
// 			return fmt.Errorf("apex GenesisProxy.setUpProxy params encoding failed: %w", err)
// 		}

// 		err = callContract(contracts.SystemCaller, proxyAddress, proxyInput, "GenesisProxy.setUpProxy", transition)
// 		if err != nil {
// 			return err
// 		}
// 	}

// 	return nil
// }

// func getDataForNexusContract(contract types.Address) ([]byte, error) {
// 	switch contract {
// 	case contracts.TestContract:
// 		return (&contractsapi.InitializeBridgeFn{}).EncodeAbi()
// 	default:
// 		return nil, fmt.Errorf("no contract defined at address %v", contract)
// 	}
// }

// // Nexus smart contracts initialization

// // initBridge initializes Bridge and it's proxy SC
// func initTestContract(transition *state.Transition) error {
// 	setDependenciesFn := &contractsapi.SetDependenciesBridgeFn{
// 		ClaimsAddress:        contracts.Claims,
// 		SignedBatchesAddress: contracts.SignedBatches,
// 		SlotsAddress:         contracts.Slots,
// 		ValidatorsAddress:    contracts.Validators,
// 	}

// 	input, err := setDependenciesFn.EncodeAbi()
// 	if err != nil {
// 		return fmt.Errorf("Bridge.setDependencies params encoding failed: %w", err)
// 	}

// 	return callContract(contracts.SystemCaller,
// 		contracts.Bridge, input, "Bridge.setDependencies", transition)
// }
