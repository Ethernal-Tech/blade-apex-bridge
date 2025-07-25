package e2eindexer

import (
	"errors"
	"fmt"
	"sync"

	"github.com/Ethernal-Tech/cardano-infrastructure/common"
	"github.com/Ethernal-Tech/cardano-infrastructure/indexer"
	"github.com/Ethernal-Tech/cardano-infrastructure/indexer/gouroboros"
	"github.com/hashicorp/go-hclog"
)

const blocksQueueSize = 35

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
	GetFailedTxs() []string
	Add(txs ...string)
	SetCallback(callback TxsExecutedCallback)
	ResetData()
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

	txs := append([]string(nil), t.txs...)

	return TxsInfo{
		Desired:  txs,
		Executed: txs,
	}
}

// GetFailedTxsMap implements TxsExecutedComponent.
func (t *TxsExecutedComponentDummy) GetFailedTxs() []string {
	t.lock.RLock()
	defer t.lock.RUnlock()

	return nil
}

// ResetData implements TxsExecutedComponent.
func (t *TxsExecutedComponentDummy) ResetData() {
	t.lock.Lock()
	defer t.lock.Unlock()

	t.txs = nil
}

// SetCallback implements TxsExecutedComponent.
func (t *TxsExecutedComponentDummy) SetCallback(callback TxsExecutedCallback) {
}

var _, _ TxsExecutedComponent = (*TxsExecutedComponentCardano)(nil), (*TxsExecutedComponentDummy)(nil)

type blockData struct {
	indexer.BlockPoint
	txs []string
}

// TxsExecutedComponentCardano tracks count of transactions that are not rolled back
type TxsExecutedComponentCardano struct {
	lock           sync.RWMutex
	confirmedPoint indexer.BlockPoint
	blocks         common.CircularQueue[*blockData]
	desiredTxs     map[string]struct{}
	failedTxs      map[string]struct{}
	executedTxs    map[string]struct{}
	callback       TxsExecutedCallback

	syncer indexer.BlockSyncer
	logger hclog.Logger
}

var _ indexer.BlockSyncerHandler = (*TxsExecutedComponentCardano)(nil)

// NewTxsExecutedComponentCardano creates TxsExecutedComponent
func NewTxsExecutedComponentCardano(
	config *gouroboros.BlockSyncerConfig, startingBlockPoint indexer.BlockPoint, logger hclog.Logger,
) (*TxsExecutedComponentCardano, error) {
	component := &TxsExecutedComponentCardano{
		lock:           sync.RWMutex{},
		desiredTxs:     map[string]struct{}{},
		executedTxs:    map[string]struct{}{},
		failedTxs:      map[string]struct{}{},
		blocks:         common.NewCircularQueue[*blockData](blocksQueueSize),
		confirmedPoint: startingBlockPoint,
		logger:         logger,
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

func (b *TxsExecutedComponentCardano) GetFailedTxs() []string {
	b.lock.RLock()
	defer b.lock.RUnlock()

	result := make([]string, 0, len(b.failedTxs))

	for hash := range b.failedTxs {
		result = append(result, hash)
	}

	return result
}

// Add must be called before adding submit actual tx
func (b *TxsExecutedComponentCardano) Add(txs ...string) {
	b.lock.Lock()
	defer b.lock.Unlock()

	for _, tx := range txs {
		b.desiredTxs[tx] = struct{}{}
	}
}

func (b *TxsExecutedComponentCardano) ResetData() {
	b.lock.Lock()
	defer b.lock.Unlock()

	b.desiredTxs = map[string]struct{}{}
	b.executedTxs = map[string]struct{}{}
	b.failedTxs = map[string]struct{}{}
	// clear old txs but keep blocks in queue
	for _, blck := range b.blocks.ToList() {
		blck.txs = nil
	}
}

// Close closes the syncer
func (b *TxsExecutedComponentCardano) Close() error {
	return b.syncer.Close()
}

func (b *TxsExecutedComponentCardano) SetCallback(callback TxsExecutedCallback) {
	b.lock.Lock()
	defer b.lock.Unlock()

	b.callback = callback
}

// Reset implements indexer.BlockSyncerHandler.
func (b *TxsExecutedComponentCardano) Reset() (indexer.BlockPoint, error) {
	b.lock.RLock()
	defer b.lock.RUnlock()

	b.blocks.ClearFrom(0)

	return b.confirmedPoint, nil
}

// RollBackward implements indexer.BlockSyncerHandler.
func (b *TxsExecutedComponentCardano) RollBackward(point indexer.BlockPoint) error {
	b.lock.Lock()
	defer b.lock.Unlock()

	indx := b.blocks.Find(func(blck *blockData) bool {
		return blck.BlockSlot == point.BlockSlot && blck.BlockHash == point.BlockHash
	})
	//nolint
	if indx != -1 {
		indx++
	} else if b.confirmedPoint.BlockSlot == point.BlockSlot && b.confirmedPoint.BlockHash == point.BlockHash {
		// everything is ok -> we are reverting to the latest confirmed block
		indx = 0
	} else {
		// we have confirmed a block that should NOT have been confirmed!
		// recovering from this error is difficult and requires manual database changes
		return errors.Join(indexer.ErrBlockIndexerFatal,
			fmt.Errorf("roll backward block not found. new = (%d, %s) vs latest = (%d, %s)",
				point.BlockSlot, point.BlockHash, b.confirmedPoint.BlockSlot, &b.confirmedPoint.BlockHash))
	}

	var failedTxs []string

	// remove all executed transactions from removed blocks and add those txs to failed txs map
	for _, innerBlock := range b.blocks.ToList()[indx:] {
		failedTxs = append(failedTxs, innerBlock.txs...)

		for _, txHash := range innerBlock.txs {
			delete(b.executedTxs, txHash)
			b.failedTxs[txHash] = struct{}{}
		}
	}

	b.blocks.ClearFrom(indx) // remove all in memory blocks from indx

	if len(failedTxs) > 0 {
		b.logger.Warn("roll backward happened, some txs are lost", "txs", failedTxs)
	}

	if b.callback != nil {
		b.callback.Rollback(failedTxs)
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

	if len(txs) > 0 {
		b.logger.Info("roll forward happened, some txs are executed", "txs", txs)
	}

	if b.callback != nil {
		b.callback.Forward(txs)
	}

	if b.blocks.IsFull() {
		b.confirmedPoint = b.blocks.Pop().BlockPoint
	}

	return b.blocks.Push(&blockData{
		BlockPoint: indexer.BlockPoint{
			BlockSlot: blockHeader.Slot,
			BlockHash: blockHeader.Hash,
		},
		txs: txs,
	})
}
