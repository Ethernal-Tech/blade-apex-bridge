package genesis

import (
	"github.com/0xPolygon/polygon-edge/chain"
)

const (
	nexusBridgeFlag             = "nexus"
	nexusBridgeFlagDefaultValue = false
	nexusBridgeDescriptionFlag  = "settings needed for nexus bridge"
)

func (p *genesisParams) processConfigNexus(chainConfig *chain.Chain) {
	if !p.nexusBridge {
		return
	}

	chainConfig.Params.Forks.
		RemoveFork(chain.Governance).
		// RemoveFork(chain.London).
		RemoveFork(chain.EIP3855).
		RemoveFork(chain.Berlin).
		RemoveFork(chain.EIP3607)
	// chainConfig.Params.BurnContract = nil
}
