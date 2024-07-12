package contracts

import "github.com/0xPolygon/polygon-edge/types"

var (
	// Nexus contracts

	// Address of Bridge proxy
	TestContract     = types.StringToAddress("0xABEF000000000000000000000000000000000100")
	TestContractAddr = types.StringToAddress("0xABEF000000000000000000000000000000000110")
)

func GetNexusProxyImplementationMapping() map[types.Address]types.Address {
	return map[types.Address]types.Address{
		TestContract: TestContractAddr,
	}
}
