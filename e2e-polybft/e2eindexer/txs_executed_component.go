package e2eindexer

import (
	"sync"

	"github.com/Ethernal-Tech/cardano-infrastructure/indexer"
	"github.com/Ethernal-Tech/cardano-infrastructure/indexer/gouroboros"
	"github.com/hashicorp/go-hclog"
)

type TxsExecutedCallback interface {
	Forward(executed []string)
	Rollback(failed []string)
}

type TxsInfo struct {
	Desired  []string
	Executed []string
	Failed   []string
}

func (ti TxsInfo) IsEverythingProcessed() bool {
	return len(ti.Desired) > 0 && len(ti.Desired) == len(ti.Executed)+len(ti.Failed)
}

type TxsExecutedComponent interface {
	GetTxs() TxsInfo
	Add(txs ...string)
	Close() error
}

type TxsExecutedComponentDummy struct {
	lock sync.RWMutex
	txs  []string
}

func NewTxsExecutedComponentDummy() *TxsExecutedComponentDummy {
	return &TxsExecutedComponentDummy{
		lock: sync.RWMutex{},
	}
}

// Add implements TxsExecutedComponent.
func (t *TxsExecutedComponentDummy) Add(txs ...string) {
	t.lock.Lock()
	defer t.lock.Unlock()

	t.txs = append(t.txs, txs...)
}

// Close implements TxsExecutedComponent.
func (t *TxsExecutedComponentDummy) Close() error {
	return nil
}

// GetTxs implements TxsExecutedComponent.
func (t *TxsExecutedComponentDummy) GetTxs() TxsInfo {
	t.lock.RLock()
	defer t.lock.RUnlock()

	return TxsInfo{
		Desired:  t.txs,
		Executed: t.txs,
	}
}

var _, _ TxsExecutedComponent = (*TxsExecutedComponentCardano)(nil), (*TxsExecutedComponentDummy)(nil)

type blockData struct {
	indexer.BlockPoint
	txs []string
}

// TxsExecutedComponentCardano tracks count of transactions that are not rolled back
type TxsExecutedComponentCardano struct {
	lock        sync.RWMutex
	blocks      []blockData
	desiredTxs  map[string]struct{}
	failedTxs   map[string]struct{}
	executedTxs map[string]struct{}
	callback    TxsExecutedCallback

	syncer indexer.BlockSyncer
	logger hclog.Logger
}

var _ indexer.BlockSyncerHandler = (*TxsExecutedComponentCardano)(nil)

// NewTxsExecutedComponentCardano creates TxsExecutedComponent
func NewTxsExecutedComponentCardano(
	config *gouroboros.BlockSyncerConfig, startingBlockPoint indexer.BlockPoint,
	callback TxsExecutedCallback, logger hclog.Logger,
) (*TxsExecutedComponentCardano, error) {
	component := &TxsExecutedComponentCardano{
		lock:        sync.RWMutex{},
		desiredTxs:  map[string]struct{}{},
		executedTxs: map[string]struct{}{},
		failedTxs:   map[string]struct{}{},
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
func (b *TxsExecutedComponentCardano) GetTxs() TxsInfo {
	b.lock.RLock()
	defer b.lock.RUnlock()

	desired := make([]string, 0, len(b.desiredTxs))
	executed := make([]string, 0, len(b.executedTxs))
	failed := make([]string, 0, len(b.failedTxs))

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
func (b *TxsExecutedComponentCardano) Add(txs ...string) {
	b.lock.Lock()
	defer b.lock.Unlock()

	for _, tx := range txs {
		b.desiredTxs[tx] = struct{}{}
	}
}

// Close closes the syncer
func (b *TxsExecutedComponentCardano) Close() error {
	return b.syncer.Close()
}

// Reset implements indexer.BlockSyncerHandler.
func (b *TxsExecutedComponentCardano) Reset() (indexer.BlockPoint, error) {
	b.lock.RLock()
	defer b.lock.RUnlock()

	return b.blocks[len(b.blocks)-1].BlockPoint, nil
}

// RollBackward implements indexer.BlockSyncerHandler.
func (b *TxsExecutedComponentCardano) RollBackward(point indexer.BlockPoint) error {
	b.lock.Lock()
	defer b.lock.Unlock()

	failedBlockInd := 0

	for i := len(b.blocks) - 1; i >= 0; i-- {
		if block := b.blocks[i]; block.BlockHash == point.BlockHash && block.BlockSlot == point.BlockSlot {
			failedBlockInd = i + 1

			break
		}
	}

	var failedTxs []string

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
func (b *TxsExecutedComponentCardano) RollForward(
	blockHeader indexer.BlockHeader, txsRetriver indexer.BlockTxsRetriever,
) error {
	b.lock.Lock()
	defer b.lock.Unlock()

	allRetrievedTxs, err := txsRetriver.GetBlockTransactions(blockHeader)
	if err != nil {
		return err
	}

	//nolint:prealloc
	var txs []string

	for _, txFromBlock := range allRetrievedTxs {
		txHashStr := txFromBlock.Hash.String()

		if _, exists := b.desiredTxs[txHashStr]; !exists {
			continue
		}

		b.executedTxs[txHashStr] = struct{}{}
		delete(b.failedTxs, txHashStr)

		txs = append(txs, txHashStr)
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
