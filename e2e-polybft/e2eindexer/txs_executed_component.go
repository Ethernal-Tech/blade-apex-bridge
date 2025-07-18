package e2eindexer

import (
	"sync"

	"github.com/Ethernal-Tech/cardano-infrastructure/indexer"
	"github.com/Ethernal-Tech/cardano-infrastructure/indexer/gouroboros"
	"github.com/hashicorp/go-hclog"
)

type TxsExecutedCallback interface {
	Forward(executed []indexer.Hash)
	Rollback(failed []indexer.Hash)
}

type TxsInfo struct {
	Desired  []indexer.Hash
	Executed []indexer.Hash
	Failed   []indexer.Hash
}

func (ti TxsInfo) IsEverythingProcessed() bool {
	return len(ti.Desired) > 0 && len(ti.Desired) == len(ti.Executed)+len(ti.Failed)
}

type blockData struct {
	indexer.BlockPoint
	txs []indexer.Hash
}

// TxsExecutedComponent tracks count of transactions that are not rolled back
type TxsExecutedComponent struct {
	lock        sync.RWMutex
	blocks      []blockData
	desiredTxs  map[indexer.Hash]struct{}
	failedTxs   map[indexer.Hash]struct{}
	executedTxs map[indexer.Hash]struct{}
	callback    TxsExecutedCallback

	syncer indexer.BlockSyncer
	logger hclog.Logger
}

var _ indexer.BlockSyncerHandler = (*TxsExecutedComponent)(nil)

// NewTxsExecutedComponent creates TxsExecutedComponent
func NewTxsExecutedComponent(
	config *gouroboros.BlockSyncerConfig, startingBlockPoint indexer.BlockPoint,
	callback TxsExecutedCallback, logger hclog.Logger,
) (*TxsExecutedComponent, error) {
	component := &TxsExecutedComponent{
		lock:        sync.RWMutex{},
		desiredTxs:  map[indexer.Hash]struct{}{},
		executedTxs: map[indexer.Hash]struct{}{},
		failedTxs:   map[indexer.Hash]struct{}{},
		blocks: []blockData{
			{
				BlockPoint: startingBlockPoint,
			},
		},
		callback: callback,
		logger:   logger,
	}

	component.syncer = gouroboros.NewBlockSyncer(config, component, logger)

	if err := component.syncer.Sync(); err != nil {
		return nil, err
	}

	return component, nil
}

// GetTxs returns TxsInfo
func (b *TxsExecutedComponent) GetTxs() TxsInfo {
	b.lock.RLock()
	defer b.lock.RUnlock()

	desired := make([]indexer.Hash, 0, len(b.desiredTxs))
	executed := make([]indexer.Hash, 0, len(b.executedTxs))
	failed := make([]indexer.Hash, 0, len(b.failedTxs))

	for hash := range b.desiredTxs {
		desired = append(desired, hash)
	}

	for hash := range b.executedTxs {
		executed = append(executed, hash)
	}

	for hash := range b.failedTxs {
		failed = append(failed, hash)
	}

	return TxsInfo{
		Desired:  desired,
		Executed: executed,
		Failed:   failed,
	}
}

// Add must be called before adding submit actual tx
func (b *TxsExecutedComponent) Add(txs ...indexer.Hash) {
	b.lock.Lock()
	defer b.lock.Unlock()

	for _, tx := range txs {
		b.desiredTxs[tx] = struct{}{}
	}
}

// Close closes the syncer
func (b *TxsExecutedComponent) Close() error {
	return b.syncer.Close()
}

// Reset implements indexer.BlockSyncerHandler.
func (b *TxsExecutedComponent) Reset() (indexer.BlockPoint, error) {
	b.lock.RLock()
	defer b.lock.RUnlock()

	return b.blocks[len(b.blocks)-1].BlockPoint, nil
}

// RollBackward implements indexer.BlockSyncerHandler.
func (b *TxsExecutedComponent) RollBackward(point indexer.BlockPoint) error {
	b.lock.Lock()
	defer b.lock.Unlock()

	failedBlockInd := 0

	for i := len(b.blocks) - 1; i >= 0; i-- {
		if block := b.blocks[i]; block.BlockHash == point.BlockHash && block.BlockSlot == point.BlockSlot {
			failedBlockInd = i + 1

			break
		}
	}

	var failedTxs []indexer.Hash

	// remove all executed transactions from subsequent blocks and them to failed map
	for _, innerBlock := range b.blocks[failedBlockInd:] {
		failedTxs = append(failedTxs, innerBlock.txs...)

		for _, txHash := range innerBlock.txs {
			delete(b.executedTxs, txHash)
			b.failedTxs[txHash] = struct{}{}
		}
	}

	b.blocks = b.blocks[:failedBlockInd] // keep all blocks until point

	if b.callback != nil {
		b.callback.Rollback(failedTxs)
	}

	if failedBlockInd == 0 {
		b.logger.Error("roll backward to non existing block point", "point", point)

		b.blocks = append(b.blocks, blockData{
			BlockPoint: point,
		})
	}

	return nil
}

// RollForward implements indexer.BlockSyncerHandler.
func (b *TxsExecutedComponent) RollForward(
	blockHeader indexer.BlockHeader, txsRetriver indexer.BlockTxsRetriever,
) error {
	b.lock.Lock()
	defer b.lock.Unlock()

	allRetrievedTxs, err := txsRetriver.GetBlockTransactions(blockHeader)
	if err != nil {
		return err
	}

	//nolint:prealloc
	var txs []indexer.Hash

	for _, txFromBlock := range allRetrievedTxs {
		if _, exists := b.desiredTxs[txFromBlock.Hash]; !exists {
			continue
		}

		b.executedTxs[txFromBlock.Hash] = struct{}{}
		delete(b.failedTxs, txFromBlock.Hash)

		txs = append(txs, txFromBlock.Hash)
	}

	if b.callback != nil {
		b.callback.Forward(txs)
	}

	b.blocks = append(b.blocks, blockData{
		BlockPoint: indexer.BlockPoint{
			BlockSlot: blockHeader.Slot,
			BlockHash: blockHeader.Hash,
		},
		txs: txs,
	})

	return nil
}
