package e2ehelper

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/0xPolygon/polygon-edge/e2e-polybft/cardanofw"
	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/Ethernal-Tech/cardano-infrastructure/indexer"
	indexerDb "github.com/Ethernal-Tech/cardano-infrastructure/indexer/db"
	"github.com/Ethernal-Tech/cardano-infrastructure/indexer/gouroboros"
	"github.com/hashicorp/go-hclog"
)

const (
	confirmationBlockCount = 10

	indexerRestartDelay   = time.Second * 5
	indexerKeepAlive      = true
	indexerSyncStartTries = math.MaxInt

	defaultObservingWaitTime = 10 * time.Second
)

type CardanoTxObserverImpl struct {
	ctx         context.Context
	indexerDB   indexer.Database
	syncer      indexer.BlockSyncer
	chainConfig *cardanofw.TestCardanoChainConfig
	isClosed    uint32
	txChan      chan channelMsg
}

func NewCardanoTxObserver(
	ctx context.Context,
	chainConfig *cardanofw.TestCardanoChainConfig,
	chainInfo *cardanofw.CardanoChainInfo,
	indexerDB indexer.Database,
) (*CardanoTxObserverImpl, error) {
	indexerConfig, syncerConfig := loadSyncerConfigs(chainConfig, chainInfo)

	err := initOracleState(indexerDB, chainConfig.StartBlockHash, chainConfig.StartSlot,
		chainConfig.InitialUtxos)
	if err != nil {
		return nil, err
	}

	txChan := make(chan channelMsg, 1000)

	confirmedBlockHandler := func(txChan chan channelMsg, chainID int) func(block *indexer.CardanoBlock, blockTxs []*indexer.Tx) error {
		return func(block *indexer.CardanoBlock, blockTxs []*indexer.Tx) error {
			// fmt.Println("Confirmed Block Handler invoked",
			// 	"block", block.Hash, "slot", block.Slot, "block txs", len(blockTxs))

			// do not rely only on blockTx, instead retrieve all unprocessed transactions from the database
			// to account for any previous errors
			txs, err := indexerDB.GetUnprocessedConfirmedTxs(0)
			if err != nil {
				return err
			}

			// Process confirmed Txs
			fmt.Printf("\nUnprocessedConfirmedTxs:\n")
			for _, tx := range txs {
				msg := channelMsg{
					chainID: chainID,
					txHash:  tx.Hash,
				}

				select {
				case <-ctx.Done():
					return nil
				case txChan <- msg:
					fmt.Printf("Channel message with chain ID: %d and txHash: %s is successfully sent over the channel", msg.chainID, msg.txHash)
				default:
					fmt.Printf("ERROR: Channel message with chain ID: %d and txHash: %s failed to be sent over the channel", msg.chainID, msg.txHash)
				}
			}
			// Send transaction hashes through the channel, signal to the test that transactions are not rolled back
			err = indexerDB.MarkConfirmedTxsProcessed(txs)
			if err != nil {
				return err
			}

			return nil
		}
	}(txChan, chainConfig.ID)

	blockIndexer := indexer.NewBlockIndexer(indexerConfig, confirmedBlockHandler, indexerDB, hclog.NewNullLogger())
	syncer := gouroboros.NewBlockSyncer(syncerConfig, blockIndexer, hclog.NewNullLogger())

	return &CardanoTxObserverImpl{
		ctx:         ctx,
		indexerDB:   indexerDB,
		syncer:      syncer,
		chainConfig: chainConfig,
		txChan:      txChan,
	}, nil
}

func (ctxo CardanoTxObserverImpl) Start() error {
	bp, err := ctxo.indexerDB.GetLatestBlockPoint()
	if err == nil && bp != nil {
		fmt.Println("Started...", "hash", bp.BlockHash, "slot", bp.BlockSlot)
	}

	go func() {
		common.RetryForever(ctxo.ctx, 5*time.Second, func(context.Context) (err error) {
			err = ctxo.syncer.Sync()
			if err != nil {
				fmt.Println("Failed to Start syncer while starting CardanoChainObserver. Retrying...",
					"chainId", ctxo.chainConfig.ID, "err", err)
			}

			return err
		})

		for {
			select {
			case <-ctxo.ctx.Done():
				return
			case err, ok := <-ctxo.syncer.ErrorCh():
				if !ok {
					return
				}

				fmt.Println("Syncer fatal error", "chainID", ctxo.chainConfig.ID, "err", err)

				if err := ctxo.Dispose(); err != nil {
					fmt.Println("cardano chain observer dispose failed", "err", err)
				}
			}
		}
	}()

	return nil
}

func (ctxo *CardanoTxObserverImpl) TxChan() <-chan channelMsg {
	return ctxo.txChan
}

func (ctxo CardanoTxObserverImpl) Dispose() error {
	if atomic.CompareAndSwapUint32(&ctxo.isClosed, 0, 1) {
		if err := ctxo.syncer.Close(); err != nil {
			fmt.Println("Failed to close syncer", "err", err)
		}

		if err := ctxo.indexerDB.Close(); err != nil {
			fmt.Println("Failed to close indexerDB", "err", err)
		}
	}

	return nil
}

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

func initOracleState(
	db indexer.Database,
	blockHashStr string, blockSlot uint64, utxos []cardanofw.CardanoChainConfigUtxo,
) error {
	blockHash := indexer.NewHashFromHexString(blockHashStr)
	if blockHash == (indexer.Hash{}) {
		fmt.Println("Configuration block hash is zero hash", "slot", blockSlot)

		return nil
	}

	latestBlockPoint, err := db.GetLatestBlockPoint()
	if err != nil {
		return fmt.Errorf("could not retrieve latest block point while initializing utxos: %w", err)
	}

	currentBlockSlot := uint64(0)
	if latestBlockPoint != nil {
		currentBlockSlot = latestBlockPoint.BlockSlot
	}

	// in oracle we already have more recent block
	if currentBlockSlot >= blockSlot {
		fmt.Println("Oracle database contains more recent block",
			"hash", currentBlockSlot, "slot", currentBlockSlot)

		return nil
	}

	return db.OpenTx().DeleteAllTxOutputsPhysically().SetLatestBlockPoint(&indexer.BlockPoint{
		BlockSlot: blockSlot,
		BlockHash: blockHash,
	}).AddTxOutputs(convertUtxos(utxos)).Execute()
}

func convertUtxos(input []cardanofw.CardanoChainConfigUtxo) (output []*indexer.TxInputOutput) {
	output = make([]*indexer.TxInputOutput, len(input))
	for i, inp := range input {
		output[i] = &indexer.TxInputOutput{
			Input: indexer.TxInput{
				Hash:  inp.Hash,
				Index: inp.Index,
			},
			Output: indexer.TxOutput{
				Address: inp.Address,
				Amount:  inp.Amount,
				Slot:    inp.Slot,
			},
		}
	}

	return output
}

func initIndexerDBs(chains []string) (map[string]indexer.Database, error) {
	baseDBPath := "../../tmp/test-dbs"

	os.RemoveAll(baseDBPath)

	if err := common.CreateDirSafe(baseDBPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create base directory: %w", err)
	}

	cardanoIndexerDbs := make(map[string]indexer.Database, len(chains))

	for _, chain := range chains {
		indexerDB, err := indexerDb.NewDatabaseInit("",
			filepath.Join(baseDBPath, chain+".db"))
		if err != nil {
			return nil, fmt.Errorf("failed to open oracle indexer db for `%s`: %w", chain, err)
		}

		cardanoIndexerDbs[chain] = indexerDB
	}

	return cardanoIndexerDbs, nil
}
