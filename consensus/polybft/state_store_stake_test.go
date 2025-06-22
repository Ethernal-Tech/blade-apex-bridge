package polybft

import (
	"math/big"
	"testing"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/validator"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestState_Insert_And_Get_FullValidatorSet(t *testing.T) {
	state := newTestState(t)

	t.Run("No full validator set", func(t *testing.T) {
		_, err := state.StakeStore.getFullValidatorSet(nil)

		require.ErrorIs(t, err, errNoFullValidatorSet)
	})

	t.Run("Insert validator set", func(t *testing.T) {
		validators := validator.NewTestValidators(t, 5).GetPublicIdentities()

		assert.NoError(t, state.StakeStore.insertFullValidatorSet(validatorSetState{
			BlockNumber: 100,
			EpochID:     10,
			Validators:  newValidatorStakeMap(validators),
		}, nil))

		fullValidatorSet, err := state.StakeStore.getFullValidatorSet(nil)
		require.NoError(t, err)
		assert.Equal(t, uint64(100), fullValidatorSet.BlockNumber)
		assert.Equal(t, uint64(10), fullValidatorSet.EpochID)
		assert.Len(t, fullValidatorSet.Validators, len(validators))
	})

	t.Run("Update validator set", func(t *testing.T) {
		validators := validator.NewTestValidators(t, 10).GetPublicIdentities()

		assert.NoError(t, state.StakeStore.insertFullValidatorSet(validatorSetState{
			BlockNumber: 40,
			EpochID:     4,
			Validators:  newValidatorStakeMap(validators),
		}, nil))

		fullValidatorSet, err := state.StakeStore.getFullValidatorSet(nil)
		require.NoError(t, err)
		assert.Len(t, fullValidatorSet.Validators, len(validators))
		assert.Equal(t, uint64(40), fullValidatorSet.BlockNumber)
		assert.Equal(t, uint64(4), fullValidatorSet.EpochID)
	})
}

func TestState_Insert_And_Get_StakingEvent(t *testing.T) {
	state := newTestState(t)

	addr := types.StringToAddress("0xDA01")

	sae := contractsapi.StakeAddedEvent{
		Validator: addr,
		Amount:    big.NewInt(1),
	}

	// Insert a StakeAddedEvent, but try to retrieve a StakeRemovedEvent.
	// Expecting nil since no such event was inserted.
	require.NoError(t, state.StakeStore.insertStakingEvent(&sae, nil))
	retRem, err := state.StakeStore.getStakeRemovedEvent(addr, nil)
	require.NoError(t, err)
	require.Nil(t, retRem)

	// Try to retrieve the previously inserted StakeAddedEvent.
	// Should return the event successfully.
	retAdd, err := state.StakeStore.getStakeAddedEvent(addr, nil)
	require.NoError(t, err)
	require.NotNil(t, retAdd)
	require.EqualValues(t, addr, retAdd.Validator)
	require.EqualValues(t, big.NewInt(1), retAdd.Amount)

	// Attempt to retrieve it again.
	// Should return nil since the event was already taken.
	retAdd, err = state.StakeStore.getStakeAddedEvent(addr, nil)
	require.NoError(t, err)
	require.Nil(t, retAdd)

	sre := contractsapi.StakeRemovedEvent{
		Validator: addr,
		Amount:    big.NewInt(1),
	}

	retAdd = nil
	retAdd = nil
	err = nil

	// Insert two StakeRemovedEvent, then attempt to retrieve a StakeAddedEvent.
	// Should return nil since no such event exists.
	require.NoError(t, state.StakeStore.insertStakingEvent(&sre, nil))
	require.NoError(t, state.StakeStore.insertStakingEvent(&sre, nil))

	retAdd, err = state.StakeStore.getStakeAddedEvent(addr, nil)
	require.NoError(t, err)
	require.Nil(t, retAdd)

	// Retrieve the StakeRemovedEvent.
	// Should succeed (one more remains in the store).
	retRem, err = state.StakeStore.getStakeRemovedEvent(addr, nil)
	require.NoError(t, err)
	require.NotNil(t, retRem)
	require.EqualValues(t, addr, retRem.Validator)
	require.EqualValues(t, big.NewInt(1), retRem.Amount)

	// Retrieve the second StakeRemovedEvent.
	// Should also succeed.
	retRem, err = state.StakeStore.getStakeRemovedEvent(addr, nil)
	require.NoError(t, err)
	require.NotNil(t, retRem)
	require.EqualValues(t, addr, retRem.Validator)
	require.EqualValues(t, big.NewInt(1), retRem.Amount)

	// Try retrieving StakeRemovedEvent again after both events have been taken.
	// Should return nil.
	retRem, err = state.StakeStore.getStakeRemovedEvent(addr, nil)
	require.NoError(t, err)
	require.Nil(t, retRem)
}
