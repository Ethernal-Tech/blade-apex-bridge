package evm

import (
	"math/big"
	"testing"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/state/runtime"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func newMockContract(value *big.Int, gas uint64, code []byte) *runtime.Contract {
	return runtime.NewContract(
		1,
		types.ZeroAddress,
		types.ZeroAddress,
		types.ZeroAddress,
		value,
		gas,
		code,
	)
}

var _ runtime.Host = (*mockHost)(nil)

// mockHost is a struct which meets the requirements of runtime.Host interface but throws panic in each methods
// we don't test all opcodes in this test
type mockHost struct {
	mock.Mock

	tracer     runtime.VMTracer
	accessList *runtime.AccessList
}

func (m *mockHost) AccountExists(addr types.Address) bool {
	panic("Not implemented in tests") //nolint:gocritic
}

func (m *mockHost) GetStorage(addr types.Address, key types.Hash) types.Hash {
	args := m.Called()

	return args.Get(0).(types.Hash)
}

func (m *mockHost) SetState(
	addr types.Address,
	key types.Hash,
	value types.Hash,
) {
	panic("Not implemented in tests") //nolint:gocritic
}

func (m *mockHost) SetStorage(
	addr types.Address,
	key types.Hash,
	value types.Hash,
	config *chain.ForksInTime,
) runtime.StorageStatus {
	args := m.Called()

	return args.Get(0).(runtime.StorageStatus)
}

func (m *mockHost) SetNonPayable(bool) {
	panic("Not implemented in tests") //nolint:gocritic
}

func (m *mockHost) GetBalance(addr types.Address) *big.Int {
	args := m.Called(addr)

	return args.Get(0).(*big.Int)
}

func (m *mockHost) GetCodeSize(addr types.Address) int {
	args := m.Called(addr)

	return args.Int(0)
}

func (m *mockHost) GetCodeHash(addr types.Address) types.Hash {
	args := m.Called(addr)

	return types.StringToHash(args.String(0))
}

func (m *mockHost) GetCode(addr types.Address) []byte {
	args := m.Called(addr)

	return types.StringToBytes(args.String(0))
}

func (m *mockHost) Selfdestruct(addr types.Address, beneficiary types.Address) {
	m.Called()
}

func (m *mockHost) GetTxContext() runtime.TxContext {
	args := m.Called()

	return args.Get(0).(runtime.TxContext)
}

func (m *mockHost) GetBlockHash(number int64) types.Hash {
	args := m.Called(number)

	return args.Get(0).(types.Hash)
}

func (m *mockHost) EmitLog(addr types.Address, topics []types.Hash, data []byte) {
	m.Called()
}

func (m *mockHost) Callx(*runtime.Contract, runtime.Host) *runtime.ExecutionResult {
	panic("Not implemented in tests") //nolint:gocritic
}

func (m *mockHost) Empty(addr types.Address) bool {
	args := m.Called(addr)

	return args.Bool(0)
}

func (m *mockHost) GetNonce(addr types.Address) uint64 {
	panic("Not implemented in tests") //nolint:gocritic
}

func (m *mockHost) Transfer(from types.Address, to types.Address, amount *big.Int) error {
	panic("Not implemented in tests") //nolint:gocritic
}

func (m *mockHost) GetTracer() runtime.VMTracer {
	return m.tracer
}

func (m *mockHost) GetRefund() uint64 {
	panic("Not implemented in tests") //nolint:gocritic
}

func (m *mockHost) AddSlotToAccessList(addr types.Address, slot types.Hash) {
	m.accessList.AddSlot(addr, slot)
}

func (m *mockHost) AddAddressToAccessList(addr types.Address) {
	m.accessList.AddAddress(addr)
}

func (m *mockHost) ContainsAccessListAddress(addr types.Address) bool {
	return m.accessList.ContainsAddress(addr)
}

func (m *mockHost) ContainsAccessListSlot(addr types.Address, slot types.Hash) (bool, bool) {
	return m.accessList.Contains(addr, slot)
}

func (m *mockHost) DeleteAccessListAddress(addr types.Address) {
	m.accessList.DeleteAddress(addr)
}

func (m *mockHost) DeleteAccessListSlot(addr types.Address, slot types.Hash) {
	m.accessList.DeleteSlot(addr, slot)
}

func (m *mockHost) ActivePrecompiles() []types.Address {
	panic("Not implemented in tests") //nolint:gocritic
}

func TestRun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		value    *big.Int
		gas      uint64
		code     []byte
		config   *chain.ForksInTime
		expected *runtime.ExecutionResult
	}{
		{
			name:  "should succeed because of no codes",
			value: big.NewInt(0),
			gas:   5000,
			code:  []byte{},
			expected: &runtime.ExecutionResult{
				ReturnValue: nil,
				GasLeft:     5000,
			},
		},
		{
			name:  "should succeed and return result",
			value: big.NewInt(0),
			gas:   5000,
			code: []byte{
				PUSH1, 0x01, PUSH1, 0x02, ADD,
				PUSH1, 0x00, MSTORE8,
				PUSH1, 0x01, PUSH1, 0x00, RETURN,
			},
			expected: &runtime.ExecutionResult{
				ReturnValue: []uint8{0x03},
				GasLeft:     4976,
				GasUsed:     24,
			},
		},
		{
			name:  "should fail and consume all gas by error",
			value: big.NewInt(0),
			gas:   5000,
			// ADD will be failed by stack underflow
			code: []byte{ADD},
			expected: &runtime.ExecutionResult{
				ReturnValue: nil,
				GasLeft:     0,
				GasUsed:     5000,
				Err:         &runtime.StackUnderflowError{StackLen: 0, Required: 2},
			},
		},
		{
			name:  "should fail by REVERT and return remaining gas at that time",
			value: big.NewInt(0),
			gas:   5000,
			// Stack size and offset for return value first
			code: []byte{PUSH1, 0x00, PUSH1, 0x00, REVERT},
			config: &chain.ForksInTime{
				Byzantium: true,
			},
			expected: &runtime.ExecutionResult{
				ReturnValue: nil,
				GasUsed:     6,
				// gas consumed for 2 push1 ops
				GasLeft: 4994,
				Err:     errRevert,
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			evm := NewEVM()
			contract := newMockContract(tt.value, tt.gas, tt.code)
			host := &mockHost{}
			config := tt.config

			if config == nil {
				config = &chain.ForksInTime{}
			}

			res := evm.Run(contract, host, config)
			assert.Equal(t, tt.expected, res)
		})
	}
}

type mockCall struct {
	name string
	args map[string]interface{}
}

// m *mockTracer
type mockTracer struct {
	calls []mockCall
}

func (m *mockTracer) CaptureState(
	memory []byte,
	stack []*big.Int,
	opCode int,
	contractAddress types.Address,
	sp int,
	_host runtime.Host,
	_state runtime.VMState,
) {
	m.calls = append(m.calls, mockCall{
		name: "CaptureState",
		args: map[string]interface{}{
			"memory":          memory,
			"stack":           stack,
			"opCode":          opCode,
			"contractAddress": contractAddress,
			"sp":              sp,
		},
	})
}

func (m *mockTracer) ExecuteState(
	contractAddress types.Address,
	ip uint64,
	opcode int,
	availableGas uint64,
	cost uint64,
	lastReturnData []byte,
	depth int,
	err error,
	_host runtime.Host,
) {
	m.calls = append(m.calls, mockCall{
		name: "ExecuteState",
		args: map[string]interface{}{
			"contractAddress": contractAddress,
			"ip":              ip,
			"opcode":          opcode,
			"availableGas":    availableGas,
			"cost":            cost,
			"lastReturnData":  lastReturnData,
			"depth":           depth,
			"err":             err,
		},
	})
}

func (m *mockTracer) CaptureStateBre(
	opCode, depth int,
	ip, gas, cost uint64,
	returnData []byte,
	scope *runtime.ScopeContext,
	host runtime.Host,
	state runtime.VMState,
	err error,
) {

}

func TestRunWithTracer(t *testing.T) {
	t.Parallel()

	contractAddress := types.StringToAddress("1")

	tests := []struct {
		name     string
		value    *big.Int
		gas      uint64
		code     []byte
		config   *chain.ForksInTime
		expected []mockCall
	}{
		{
			name:  "should call CaptureState and ExecuteState",
			value: big.NewInt(0),
			gas:   5000,
			code: []byte{
				PUSH1,
				0x1,
			},
			expected: []mockCall{
				{
					name: "CaptureState",
					args: map[string]interface{}{
						"memory":          []byte{},
						"stack":           []*big.Int{},
						"opCode":          int(PUSH1),
						"contractAddress": contractAddress,
						"sp":              0,
					},
				},
				{
					name: "ExecuteState",
					args: map[string]interface{}{
						"contractAddress": contractAddress,
						"ip":              uint64(0),
						"opcode":          OpCodeToString[PUSH1],
						"availableGas":    uint64(5000),
						"cost":            uint64(3),
						"lastReturnData":  []byte{},
						"depth":           1,
						"err":             (error)(nil),
					},
				},
				{
					name: "CaptureState",
					args: map[string]interface{}{
						"memory": []byte{},
						"stack": []*big.Int{
							big.NewInt(1),
						},
						"opCode":          int(0),
						"contractAddress": contractAddress,
						"sp":              1,
					},
				},
			},
		},
		{
			name:  "should exit with error",
			value: big.NewInt(0),
			gas:   5000,
			code: []byte{
				POP,
			},
			expected: []mockCall{
				{
					name: "CaptureState",
					args: map[string]interface{}{
						"memory":          []byte{},
						"stack":           []*big.Int{},
						"opCode":          int(POP),
						"contractAddress": contractAddress,
						"sp":              0,
					},
				},
				{
					name: "ExecuteState",
					args: map[string]interface{}{
						"contractAddress": contractAddress,
						"ip":              uint64(0),
						"opcode":          OpCodeToString[POP],
						"availableGas":    uint64(5000),
						"cost":            uint64(2),
						"lastReturnData":  []byte{},
						"depth":           1,
						"err":             &runtime.StackUnderflowError{StackLen: 0, Required: 1},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			contract := newMockContract(tt.value, tt.gas, tt.code)
			contract.Address = contractAddress
			tracer := &mockTracer{}
			host := &mockHost{
				tracer: tracer,
			}

			config := tt.config
			if config == nil {
				config = &chain.ForksInTime{}
			}

			state := acquireState()
			state.resetReturnData()
			state.msg = contract
			state.code = contract.Code
			state.gas = contract.Gas
			state.host = host
			state.config = config

			// make sure stack, memory, and returnData are empty
			state.stack = make([]*big.Int, 0)
			state.memory = make([]byte, 0)
			state.returnData = make([]byte, 0)

			_, _ = state.Run()

			assert.Equal(t, tt.expected, tracer.calls)
		})
	}
}
