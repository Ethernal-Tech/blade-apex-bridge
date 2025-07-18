package e2ehelper

import (
	"math"
	"strings"
	"time"

	"github.com/0xPolygon/polygon-edge/e2e-polybft/cardanofw"
	"github.com/Ethernal-Tech/cardano-infrastructure/indexer"
	"github.com/Ethernal-Tech/cardano-infrastructure/indexer/gouroboros"
)

const (
	confirmationBlockCount = 10

	indexerRestartDelay   = time.Second * 5
	indexerKeepAlive      = true
	indexerSyncStartTries = math.MaxInt
)

func loadSyncerConfigs(
	config *cardanofw.TestCardanoChainConfig,
	chainInfo *cardanofw.CardanoChainInfo,
) (*indexer.BlockIndexerConfig, *gouroboros.BlockSyncerConfig) {
	networkAddress := strings.TrimPrefix(
		strings.TrimPrefix(chainInfo.NetworkAddress, "http://"),
		"https://")

	addressesOfInterest := []string{
		chainInfo.MultisigAddr,
		chainInfo.FeeAddr,
	}

	indexerConfig := &indexer.BlockIndexerConfig{
		StartingBlockPoint: &indexer.BlockPoint{
			BlockSlot: config.StartSlot,
			BlockHash: indexer.NewHashFromHexString(config.StartBlockHash),
		},
		AddressCheck:           indexer.AddressCheckAll,
		ConfirmationBlockCount: confirmationBlockCount,
		AddressesOfInterest:    addressesOfInterest,
	}
	syncerConfig := &gouroboros.BlockSyncerConfig{
		NetworkMagic:   uint32(cardanofw.GetNetworkMagic(config.NetworkType)),
		NodeAddress:    networkAddress,
		RestartOnError: true, // always try to restart on non-fatal errors
		RestartDelay:   indexerRestartDelay,
		KeepAlive:      indexerKeepAlive,
		SyncStartTries: indexerSyncStartTries,
	}

	return indexerConfig, syncerConfig
}
