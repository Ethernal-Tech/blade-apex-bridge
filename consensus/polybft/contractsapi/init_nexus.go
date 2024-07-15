package contractsapi

import (
	"github.com/0xPolygon/polygon-edge/contracts"
)

var (
	// Proxy contracts
	ERC1967Proxy *contracts.Artifact

	// Nexus smart contracts
	ClaimsTest *contracts.Artifact
)

func initNexusContracts() (
	claimsTest *contracts.Artifact,
	erc1967ProxyContract *contracts.Artifact,
	err error) {
	erc1967ProxyContract, err = contracts.DecodeArtifact([]byte(ERC1967ProxyArtifact))
	if err != nil {
		return
	}

	claimsTest, err = contracts.DecodeArtifact([]byte(ClaimsTestArtifact))
	if err != nil {
		return
	}

	return
}
