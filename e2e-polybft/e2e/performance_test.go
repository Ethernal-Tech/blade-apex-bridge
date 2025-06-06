package e2e

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/framework"
	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/require"
)

type confirmedBatch struct {
	BatchID    *big.Int `abi:"batchID"`
	Bitmap     *big.Int `abi:"bitmap"`
	Counter    *big.Int `abi:"counter"`
	Signatures [][]byte `abi:"signatures"`
}

func newConfirmedBatch(mp map[string]interface{}) *confirmedBatch {
	return &confirmedBatch{
		BatchID:    mp["batchID"].(*big.Int),
		Bitmap:     mp["bitmap"].(*big.Int),
		Counter:    mp["counter"].(*big.Int),
		Signatures: mp["signatures"].([][]byte),
	}
}

func (cb confirmedBatch) String() string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("ID      = %s\n", cb.BatchID))
	sb.WriteString(fmt.Sprintf("Bmp     = %s\n", cb.Bitmap))
	sb.WriteString(fmt.Sprintf("Counter = %s\n", cb.Counter))

	for i, x := range cb.Signatures {
		sb.WriteString(fmt.Sprintf("Sign(%d) = %s\n", i, hex.EncodeToString(x)))
	}

	return sb.String()
}

type memoryStats struct {
	validatorID int
	memoryUsage []int64 // in KB
	timestamps  []time.Time
}

func monitorMemoryUsage(ctx context.Context, t *testing.T, validatorIDs []int) []*memoryStats {
	t.Helper()

	stats := make([]*memoryStats, len(validatorIDs))
	for i, id := range validatorIDs {
		stats[i] = &memoryStats{
			validatorID: id,
			memoryUsage: make([]int64, 0),
			timestamps:  make([]time.Time, 0),
		}
	}

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	go func() {
		for {
			select {
			case <-ticker.C:
				for i, id := range validatorIDs {
					cmd := fmt.Sprintf("ps -o rss= -p $(pgrep -f 'test-chain-%d')", id)
					output, err := exec.Command("bash", "-c", cmd).Output()
					if err != nil {
						t.Logf("Failed to get memory usage for validator %d: %v", id, err)
						continue
					}

					// Split output into lines and sum up memory usage
					lines := strings.Split(strings.TrimSpace(string(output)), "\n")
					var totalMemory int64
					for _, line := range lines {
						if line == "" {
							continue
						}
						memoryKB, err := strconv.ParseInt(strings.TrimSpace(line), 10, 64)
						if err != nil {
							t.Logf("Failed to parse memory line '%s' for validator %d: %v", line, id, err)
							continue
						}
						totalMemory += memoryKB
					}

					if totalMemory > 0 {
						stats[i].memoryUsage = append(stats[i].memoryUsage, totalMemory)
						stats[i].timestamps = append(stats[i].timestamps, time.Now())
					}
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// Wait for context to be done
	<-ctx.Done()

	// Print memory usage statistics
	for _, stat := range stats {
		if len(stat.memoryUsage) == 0 {
			continue
		}

		var min, max, sum int64
		min = stat.memoryUsage[0]
		max = stat.memoryUsage[0]
		sum = 0

		for _, usage := range stat.memoryUsage {
			if usage < min {
				min = usage
			}
			if usage > max {
				max = usage
			}
			sum += usage
		}

		avg := float64(sum) / float64(len(stat.memoryUsage))
		t.Logf("Validator %d Memory Usage (KB):", stat.validatorID)
		t.Logf("  Min: %d", min)
		t.Logf("  Max: %d", max)
		t.Logf("  Avg: %.2f", avg)
	}

	return stats
}

func TestE2E_ApexBridge_TestPerformance(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const (
		quorumCnt                          = 5
		checkBatchID                       = true
		deleteTemporaryMappingsAfterQuorum = true
	)

	admin, err := wallet.GenerateAccount()
	require.NoError(t, err)

	cluster := framework.NewTestCluster(
		t, 4, framework.WithBladeAdmin(admin.Address().String()))

	defer cluster.Stop()

	cluster.WaitForReady(t)

	// Start memory monitoring
	validatorIDs := []int{1, 2, 3, 4}
	go monitorMemoryUsage(ctx, t, validatorIDs)

	txRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithClient(cluster.Servers[0].JSONRPC()))
	require.NoError(t, err)

	input, err := contractsapi.TestPerformance.Abi.Constructor.Inputs.Encode([]interface{}{
		big.NewInt(quorumCnt), checkBatchID, deleteTemporaryMappingsAfterQuorum,
	})
	require.NoError(t, err)

	// deploy contract
	receipt, err := txRelayer.SendTransaction(
		types.NewTx(types.NewLegacyTx(
			types.WithFrom(admin.Ecdsa.Address()),
			types.WithInput(append(contractsapi.TestPerformance.Bytecode, input...)),
		)),
		admin.Ecdsa)
	require.NoError(t, err)

	contractAddr := types.Address(receipt.ContractAddress)

	rndBytes := func(size int) []byte {
		token := make([]byte, size)
		_, _ = rand.Read(token)

		return token
	}

	submitBatch := func(t *testing.T, validatorID uint8, batchID uint64, counter uint64, signature []byte) {
		t.Helper()

		signedBatch := []interface{}{
			new(big.Int).SetUint64(batchID),
			new(big.Int).SetUint64(counter),
			new(big.Int).SetUint64(uint64(validatorID)),
			signature,
		}

		fn := contractsapi.TestPerformance.Abi.GetMethod("submitSignedBatch")
		input, err := fn.Encode([]interface{}{signedBatch})
		require.NoError(t, err)

		txn := types.NewTx(types.NewLegacyTx(
			types.WithFrom(admin.Address()),
			types.WithTo(&contractAddr),
			types.WithInput(input),
		))

		receipt, err = txRelayer.SendTransaction(txn, admin.Ecdsa)
		require.NoError(t, err)
		require.Equal(t, uint64(types.ReceiptSuccess), receipt.Status)
	}

	getConfirmedBatches := func(t *testing.T) []*confirmedBatch {
		t.Helper()

		fn := contractsapi.TestPerformance.Abi.GetMethod("getConfirmedBatches")
		input, err := fn.Encode([]interface{}{})
		require.NoError(t, err)

		response, err := txRelayer.Call(types.ZeroAddress, contractAddr, input)
		require.NoError(t, err)

		byteResponse, err := hex.DecodeHex(response)
		require.NoError(t, err)

		decoded, err := fn.Outputs.Decode(byteResponse)
		require.NoError(t, err)

		base := decoded.(map[string]interface{})
		if len(base) == 0 {
			return nil
		}

		items := base["0"].([]map[string]interface{})
		result := make([]*confirmedBatch, len(items))

		for i, x := range items {
			result[i] = newConfirmedBatch(x)
		}

		return result
	}

	getHashesCount := func(t *testing.T) uint64 {
		t.Helper()

		fn := contractsapi.TestPerformance.Abi.GetMethod("getHashesCount")
		input, err := fn.Encode([]interface{}{})
		require.NoError(t, err)

		response, err := txRelayer.Call(types.ZeroAddress, contractAddr, input)
		require.NoError(t, err)

		result, err := common.ParseUint64orHex(&response)
		require.NoError(t, err)

		return result
	}

	getLastBatchID := func(t *testing.T) uint64 {
		t.Helper()

		fn := contractsapi.TestPerformance.Abi.GetMethod("getLastBatchID")
		input, err := fn.Encode([]interface{}{})
		require.NoError(t, err)

		response, err := txRelayer.Call(types.ZeroAddress, contractAddr, input)
		require.NoError(t, err)

		result, err := common.ParseUint64orHex(&response)
		require.NoError(t, err)

		return result
	}

	require.Equal(t, uint64(0), getLastBatchID(t))

	submitBatch(t, 1, 1, 100, rndBytes(64))
	submitBatch(t, 2, 1, 100, rndBytes(64))
	submitBatch(t, 3, 1, 100, rndBytes(64))
	submitBatch(t, 4, 1, 100, rndBytes(64))
	submitBatch(t, 5, 1, 100, rndBytes(64))

	submitBatch(t, 1, 2, 100, rndBytes(64))
	submitBatch(t, 2, 2, 100, rndBytes(64))
	submitBatch(t, 3, 2, 100, rndBytes(64))
	submitBatch(t, 4, 2, 100, rndBytes(64))
	submitBatch(t, 5, 2, 100, rndBytes(64))

	submitBatch(t, 5, 3, 100, rndBytes(64))
	submitBatch(t, 3, 3, 200, rndBytes(64))
	submitBatch(t, 6, 1, 100, rndBytes(64))

	confirmedBatches := getConfirmedBatches(t)

	require.Len(t, confirmedBatches, 2)
	require.Equal(t, uint64(4), getHashesCount(t))
	require.Equal(t, uint64(2), getLastBatchID(t))

	for _, x := range confirmedBatches {
		fmt.Println(x)
	}
}

type CardanoBlock struct {
	BlockSlot uint64   `abi:"blockSlot"`
	BlockHash [32]byte `abi:"blockHash"`
}

func TestE2E_ApexBridge_TestUpdateBlocks(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	admin, err := wallet.GenerateAccount()
	require.NoError(t, err)

	cluster := framework.NewTestCluster(
		t, 4, framework.WithBlockGasLimit(16_000_000), framework.WithBladeAdmin(admin.Address().String()))

	defer cluster.Stop()

	cluster.WaitForReady(t)

	// Start memory monitoring
	validatorIDs := []int{1, 2, 3, 4}
	_ = monitorMemoryUsage(ctx, t, validatorIDs)

	txRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithClient(cluster.Servers[0].JSONRPC()))
	require.NoError(t, err)

	numOfAccounts := 4
	input, err := contractsapi.TestPerformance.Abi.Constructor.Inputs.Encode([]interface{}{
		big.NewInt(int64(numOfAccounts)), true, true,
	})
	require.NoError(t, err)

	// Deploy the contract
	receipt, err := txRelayer.SendTransaction(
		types.NewTx(types.NewLegacyTx(
			types.WithFrom(admin.Ecdsa.Address()),
			types.WithInput(append(contractsapi.TestPerformance.Bytecode, input...)),
			types.WithGas(8_242_880),
		)),
		admin.Ecdsa)
	require.NoError(t, err)

	contractAddr := types.Address(receipt.ContractAddress)

	getTotalTrieSize := func(t *testing.T) (total int64) {
		t.Helper()

		triePath := filepath.Join(cluster.Config.TmpDir, "test-chain-1", "trie")

		err := filepath.Walk(triePath, func(_ string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if !info.IsDir() {
				total += info.Size()
			}

			return nil
		})

		require.NoError(t, err)

		return total
	}

	fmt.Println("total trie size before: ", getTotalTrieSize(t))

	getLastObservedBlock(t, txRelayer, contractAddr)

	// Create accounts for each node
	accounts := make([]*wallet.Account, numOfAccounts)
	for i := range numOfAccounts {
		accounts[i], err = wallet.GenerateAccount()
		require.NoError(t, err)
	}

	var wg sync.WaitGroup
	for i := range numOfAccounts {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			updateBlocks(t, txRelayer, contractAddr, accounts[i], i)
		}(i)
	}
	wg.Wait()

	time.Sleep(10 * time.Second)

	// Get last observed block
	fn := contractsapi.TestPerformance.Abi.GetMethod("getLastObservedBlock")
	input, err = fn.Encode([]interface{}{uint8(1)})
	require.NoError(t, err)

	response, err := txRelayer.Call(types.ZeroAddress, contractAddr, input)
	require.NoError(t, err)

	fmt.Println(response)
	fmt.Println("total trie size after: ", getTotalTrieSize(t))
}

func updateBlocks(t *testing.T, txRelayer txrelayer.TxRelayer, contractAddr types.Address, account *wallet.Account, id int) {
	for i := 0; i < 10; i++ {
		t.Helper()

		getLastObservedBlock(t, txRelayer, contractAddr)

		blocks := []CardanoBlock{}
		for j := 0 + i*20; j < 20+i*20; j++ {
			blocks = append(blocks, CardanoBlock{
				BlockSlot: uint64(j),
				BlockHash: [32]byte{byte(j)},
			})
		}

		fn := contractsapi.TestPerformance.Abi.GetMethod("updateBlocks")
		input, err := fn.Encode([]interface{}{
			uint8(1),
			blocks,
			account.Address(),
			uint8(id),
		})
		require.NoError(t, err)

		txn := types.NewTx(types.NewLegacyTx(
			types.WithFrom(account.Address()),
			types.WithTo(&contractAddr),
			types.WithInput(input),
		))

		receipt, err := txRelayer.SendTransaction(txn, account.Ecdsa)
		require.NoError(t, err)

		require.Equal(t, uint64(types.ReceiptSuccess), receipt.Status)
		fmt.Printf("%d submited blocks %d - %d\n", id, i*20, i*20+20)

		time.Sleep(3 * time.Second)
	}
}

func getLastObservedBlock(t *testing.T, txRelayer txrelayer.TxRelayer, contractAddr types.Address) {
	fn := contractsapi.TestPerformance.Abi.GetMethod("getLastObservedBlock")
	input, err := fn.Encode([]interface{}{uint8(1)})
	require.NoError(t, err)

	response, err := txRelayer.Call(types.ZeroAddress, contractAddr, input)
	require.NoError(t, err)

	fmt.Println("last observed block: ", response)
}
