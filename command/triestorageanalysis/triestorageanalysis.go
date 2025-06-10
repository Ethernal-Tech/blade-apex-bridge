package triestorageanalysis

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"math/big"
	"path/filepath"
	"strings"

	"github.com/0xPolygon/polygon-edge/blockchain"
	"github.com/0xPolygon/polygon-edge/blockchain/storagev2"
	v2Pebble "github.com/0xPolygon/polygon-edge/blockchain/storagev2/pebble"
	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/helper/common"
	itrie "github.com/0xPolygon/polygon-edge/state/immutable-trie"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/cockroachdb/pebble"
	"github.com/hashicorp/go-hclog"
	"github.com/spf13/cobra"
)

/*
	./blade trie-storage-analysis\
		--data-path /home/user/blade\
		--block-num-from 1000\
		--block-num-to 1000\
		--acc-storage-only\
		--verbose\
		--acc 0xABEF000000000000000000000000000000000000:Bridge\
		--acc 0xABEF000000000000000000000000000000000001:ClaimsHelper\
		--acc 0xABEF000000000000000000000000000000000002:Claims\
		--acc 0xABEF000000000000000000000000000000000003:SignedBatches\
		--acc 0xABEF000000000000000000000000000000000004:Slots\
		--acc 0xABEF000000000000000000000000000000000005:Validators\
		--acc 0xABEF000000000000000000000000000000000006:ApexBridgeAdmin
*/
func TrieStorageAnalysisCMD() *cobra.Command {
	saCmd := &cobra.Command{
		Use:   "trie-storage-analysis",
		Short: "Shows trie storage breakdown by account/hash",
	}

	saCmd.Flags().StringVar(
		&params.DataPath,
		"data-path",
		"",
		"the directory of blade data",
	)
	saCmd.Flags().StringVar(
		&params.DBEngine,
		"db-engine",
		common.Pebble,
		"trie database, possible values: 'pebble' (default) and 'leveldb'",
	)
	saCmd.Flags().Uint64Var(
		&params.BlockNumFrom,
		"block-num-from",
		uint64(0),
		"block number from which to analyze",
	)
	saCmd.Flags().Uint64Var(
		&params.BlockNumTo,
		"block-num-to",
		uint64(0),
		"block number up to which to analyze",
	)
	saCmd.Flags().StringSliceVar(
		&params.Accs,
		"acc",
		nil,
		"predefined accounts",
	)
	saCmd.Flags().BoolVar(
		&params.AccStorageOnly,
		"acc-storage-only",
		false,
		"walk only the account storage tries",
	)
	saCmd.Flags().BoolVar(
		&params.Verbose,
		"verbose",
		false,
		"show verbose output",
	)

	outputter := command.InitializeOutputter(saCmd)
	defer outputter.WriteOutput()

	saCmd.PreRun = func(cmd *cobra.Command, args []string) {
		if params.DataPath == "" {
			outputter.SetError(fmt.Errorf("data path not specified"))

			return
		}

		if params.DBEngine != common.Pebble && params.DBEngine != common.LevelDB {
			outputter.SetError(fmt.Errorf("wrong database engine"))

			return
		}
	}

	saCmd.Run = func(cmd *cobra.Command, args []string) {
		targetAccounts := make([]account, len(params.Accs))

		for idx, addrWho := range params.Accs {
			var (
				who string
				ss  = strings.Split(addrWho, ":")
			)

			if len(ss) == 0 || len(ss) > 2 {
				outputter.SetError(fmt.Errorf("invalid acc %s", addrWho))
				outputter.WriteOutput()

				return
			}

			if len(ss) == 2 {
				who = ss[1]
			}

			targetAccounts[idx] = account{
				addr: ss[0],
				who:  who,
			}
		}

		storagePerAcc, nonAccStorage, err := calculateStorage(
			params.DataPath, params.DBEngine,
			walkSettings{
				onlyAccounts:   params.AccStorageOnly,
				targetAccounts: targetAccounts,
				startBlockNum:  params.BlockNumFrom,
				endBlockNum:    params.BlockNumTo,
			},
			verboseOutputter{outputter: outputter, verbose: params.Verbose})
		if err != nil {
			outputter.SetError(err)
			outputter.WriteOutput()

			return
		}

		hashToAcc := make(map[types.Hash]account, len(targetAccounts))
		for _, targetAcc := range targetAccounts {
			hashToAcc[types.BytesToHash(crypto.Keccak256(types.StringToAddress(targetAcc.addr).Bytes()))] = targetAcc
		}

		var sb strings.Builder
		if nonAccStorage.BitLen() > 0 {
			sb.WriteString(fmt.Sprintf("%-16v - Non account storage", nonAccStorage))
		}

		for _, targetAcc := range targetAccounts {
			storage, found := storagePerAcc[types.BytesToHash(crypto.Keccak256(types.StringToAddress(targetAcc.addr).Bytes()))]
			if found {
				sb.WriteString(
					fmt.Sprintf(
						"\n%-16v - %s",
						storage, fmt.Sprintf("Addr: %s, Who: %s", targetAcc.addr, targetAcc.who),
					),
				)
			}
		}

		for accHash, storage := range storagePerAcc {
			if _, found := hashToAcc[accHash]; !found {
				sb.WriteString(fmt.Sprintf("\n%-16v - %s", storage, "Hash: "+hex.EncodeToString(accHash.Bytes())))
			}
		}

		outputter.WriteCommandResult(&TrieStorageAnalysisResult{Message: sb.String()})
	}

	return saCmd
}

type TrieStorageAnalysisResult struct {
	Message string `json:"message"`
}

func (r *TrieStorageAnalysisResult) GetOutput() string {
	var buffer bytes.Buffer

	buffer.WriteString("\n[Trie storage analysis SUCCESS]\n")
	buffer.WriteString(r.Message)

	return buffer.String()
}

func calculateStorage(
	path, dbEngine string, settings walkSettings, o verboseOutputter,
) (map[types.Hash]*big.Int, *big.Int, error) {
	trieStorage, err := openStorage(path, dbEngine, true)
	if err != nil {
		return nil, nil, fmt.Errorf("open trie db error:%w", err)
	}
	defer trieStorage.Close()

	blockchainStorage, err := openBlockchainStorage(path, dbEngine)
	if err != nil {
		return nil, nil, fmt.Errorf("open blockchain db error:%w", err)
	}
	defer blockchainStorage.Close()

	bc, err := blockchain.NewBlockchain(
		hclog.NewNullLogger(),
		blockchainStorage, nil, nil, nil, crypto.NewSigner(chain.ForksInTime{Berlin: true}, uint64(1)),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("open blockchain db error:%w", err)
	}
	defer bc.Close()

	var (
		processedNodes    = newProcessingCache()
		processedAccCodes = newProcessingCache()
		nonAccStorage     = big.NewInt(0)
		storagePerAcc     = map[types.Hash]*big.Int{}
		addToResult       = func(accountHashRaw []byte, storageSize *big.Int) {
			if accountHashRaw == nil {
				nonAccStorage.Add(nonAccStorage, storageSize)

				return
			}

			accountHash := types.BytesToHash(accountHashRaw)

			if storageUsed, found := storagePerAcc[accountHash]; found {
				storageUsed.Add(storageUsed, storageSize)

				return
			}

			storagePerAcc[accountHash] = new(big.Int).Set(storageSize)
		}
	)

	walk := &walkProgress{
		processedNodes:    processedNodes,
		processedAccCodes: processedAccCodes,
		addToResult:       addToResult,
	}

	for blockNum := settings.startBlockNum; blockNum <= settings.endBlockNum; blockNum++ {
		block, ok := bc.GetBlockByNumber(blockNum, true)
		if !ok {
			return nil, nil, fmt.Errorf("error getting block %d", blockNum)
		}

		var err error

		if settings.onlyAccounts {
			o.write("===================================================================\n")
			o.write(fmt.Sprintf("starting account storage tries walk for block: %d\n", block.Number()))
			err = walkOnlyAccountStorageTries(trieStorage, block, settings.targetAccounts, walk, o)
			o.write("===================================================================\n")
		} else {
			o.write("===================================================================\n")
			o.write(fmt.Sprintf("starting world state trie walk for block: %d\n", block.Number()))
			err = walkTrie(trieStorage, block, block.Header.StateRoot, false, nil, walk, o)
			o.write("===================================================================\n")
		}

		if err != nil {
			return nil, nil, err
		}

		processedNodes.Next()
		processedAccCodes.Next()
	}

	return storagePerAcc, nonAccStorage, nil
}

func walkOnlyAccountStorageTries(
	trieStorage itrie.Storage, block *types.Block, targetAccounts []account, walk *walkProgress, o verboseOutputter,
) error {
	for _, targetAcc := range targetAccounts {
		acc, ok, err := itrie.GetAccount(trieStorage, block.Header.StateRoot.Bytes(), types.StringToAddress(targetAcc.addr))
		if err != nil {
			return fmt.Errorf("failed to get account for %s - %s. err: %w", targetAcc.addr, targetAcc.who, err)
		}

		o.write("\n-------------------------------------------------------------------\n")
		o.write(fmt.Sprintf("account: %s - %s", targetAcc.addr, targetAcc.who))

		if !ok {
			o.write(" - no data\n")

			continue
		}

		err = walkTrie(
			trieStorage, block, acc.Root, true,
			&itrie.AccountWithHash{Account: *acc, Hash: crypto.Keccak256(types.StringToAddress(targetAcc.addr).Bytes())},
			walk, o)
		if err != nil {
			return fmt.Errorf(
				"error while walking account storage trie for block: %d, addr: %s, who: %s. err: %w",
				block.Number(), targetAcc.addr, targetAcc.who, err)
		}
	}

	return nil
}

func walkTrie(
	trieStorage itrie.Storage, block *types.Block, root types.Hash, isStorage bool, account *itrie.AccountWithHash,
	walk *walkProgress, o verboseOutputter,
) error {
	var nodesWalked, accsWalked uint64

	o.write("\n")

	err := itrie.WalkTrie(
		root.Bytes(), trieStorage, nil, isStorage, account,
		func(nodeHash []byte, node itrie.Node, data []byte, account *itrie.AccountWithHash) error {
			nodesWalked++
			o.write(fmt.Sprintf("\rnodes walked: %-20d, accs walked: %-20d", nodesWalked, accsWalked))

			if walk.processedNodes.AlreadyProcessed(types.BytesToHash(nodeHash)) {
				return nil
			}

			storageSize := new(big.Int).Add(
				big.NewInt(int64(len(nodeHash))),
				big.NewInt(int64(len(data))))

			if account != nil {
				walk.addToResult(account.Hash, storageSize)
			} else {
				walk.addToResult(nil, storageSize)
			}

			walk.processedNodes.SetProcessed(types.BytesToHash(nodeHash))

			return nil
		},
		func(valueNode *itrie.ValueNode, account *itrie.AccountWithHash) error {
			accsWalked++
			o.write(fmt.Sprintf("\rnodes walked: %-20d, accs walked: %-20d", nodesWalked, accsWalked))

			if account.CodeHash != nil && bytes.Equal(account.CodeHash, itrie.EmptyCodeHash) == false {
				if walk.processedAccCodes.AlreadyProcessed(types.BytesToHash(account.CodeHash)) {
					return nil
				}

				hash := types.BytesToHash(account.CodeHash)

				code, ok := trieStorage.GetCode(hash)
				if ok {
					walk.addToResult(
						account.Hash,
						new(big.Int).Add(
							big.NewInt(int64(len(itrie.GetCodeKey(hash)))),
							big.NewInt(int64(len(code))),
						),
					)
				} else {
					return fmt.Errorf("can't find code %s", hex.EncodeToString(account.CodeHash))
				}

				walk.processedAccCodes.SetProcessed(types.BytesToHash(account.CodeHash))
			}

			return nil
		},
	)

	o.write("\n")

	if err != nil {
		return err
	}

	return nil
}

func openBlockchainStorage(path, dbEngine string) (*storagev2.Storage, error) {
	switch dbEngine {
	case common.Pebble:
		return v2Pebble.NewPebbleDBStorage(filepath.Join(path, "blockchain"), hclog.NewNullLogger())
	default:
		return nil, fmt.Errorf("invalid blockchain database engine %s", dbEngine)
	}
}

func openStorage(path, dbEngine string, isReadOnly bool) (itrie.Storage, error) {
	switch dbEngine {
	case common.Pebble:
		opts := &pebble.Options{Logger: itrie.PebbleLogger{}, ReadOnly: isReadOnly}

		db, err := pebble.Open(filepath.Join(path, "trie"), opts)
		if err != nil {
			return nil, err
		}

		return itrie.NewPebble(db), nil
	default:
		return nil, fmt.Errorf("invalid database engine %s", dbEngine)
	}
}

type account struct {
	addr string
	who  string
}

type verboseOutputter struct {
	outputter command.OutputFormatter
	verbose   bool
}

func (o verboseOutputter) write(output string) {
	if o.verbose {
		_, _ = o.outputter.Write([]byte(output))
	}
}

type walkSettings struct {
	startBlockNum  uint64
	endBlockNum    uint64
	onlyAccounts   bool
	targetAccounts []account
}

type walkProgress struct {
	processedNodes    processingCache
	processedAccCodes processingCache
	addToResult       func(accountHashRaw []byte, storageSize *big.Int)
}

type processingCache interface {
	Next()
	SetProcessed(hash types.Hash)
	AlreadyProcessed(hash types.Hash) bool
}

type processingCacheStrat struct {
	prevCache map[types.Hash]struct{}
	cache     map[types.Hash]struct{}
}

func newProcessingCache() processingCache {
	return &processingCacheStrat{
		prevCache: map[types.Hash]struct{}{},
		cache:     map[types.Hash]struct{}{},
	}
}

// Next implements processingCache.
func (p *processingCacheStrat) Next() {
	p.prevCache = p.cache
	p.cache = map[types.Hash]struct{}{}
}

// SetProcessed implements processingCache.
func (p *processingCacheStrat) SetProcessed(hash types.Hash) {
}

// AlreadyProcessed implements processingCache.
func (p *processingCacheStrat) AlreadyProcessed(hash types.Hash) bool {
	_, found := p.prevCache[hash]
	p.cache[hash] = struct{}{}

	return found
}

var _ processingCache = (*processingCacheStrat)(nil)
