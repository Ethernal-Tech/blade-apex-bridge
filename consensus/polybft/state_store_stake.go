package polybft

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/big"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/types"
	bolt "go.etcd.io/bbolt"
)

var (
	// bucket to store full validator set
	validatorSetBucket = []byte("fullValidatorSetBucket")
	// "root" bucket to store all unprocessed staking (StakeAdded/StakeRemoved) events
	stakingBucket = []byte("staking")
	// sub-bucket to store all unprocessed stake (StakeAdded) events
	stakeSubBucket = []byte("stake")
	// sub-bucket to store all unprocessed unstake (StakeRemoved) events
	unstakeSubBucket = []byte("unstake")
	// key of the full validator set in bucket
	fullValidatorSetKey = []byte("fullValidatorSet")
	// error returned if full validator set does not exists in db
	errNoFullValidatorSet = errors.New("full validator set not in db")
)

type StakeStore struct {
	db *bolt.DB
}

// initialize creates necessary buckets in DB if they don't already exist
func (s *StakeStore) initialize(tx *bolt.Tx) error {
	if _, err := tx.CreateBucketIfNotExists(validatorSetBucket); err != nil {
		return fmt.Errorf("failed to create bucket=%s: %w", string(epochsBucket), err)
	}

	buc, err := tx.CreateBucketIfNotExists(stakingBucket)
	if err != nil {
		return fmt.Errorf("failed to create staking bucket=%s: %w", string(stakingBucket), err)
	}

	if _, err := buc.CreateBucketIfNotExists(stakeSubBucket); err != nil {
		return fmt.Errorf("failed to create stake sub-bucket=%s: %w", string(stakeSubBucket), err)
	}

	if _, err := buc.CreateBucketIfNotExists(unstakeSubBucket); err != nil {
		return fmt.Errorf("failed to create unstake sub-bucket=%s: %w", string(unstakeSubBucket), err)
	}

	return nil
}

// insertFullValidatorSet inserts full validator set to its bucket (or updates it if exists)
// If the passed tx is already open (not nil), it will use it to insert full validator set
// If the passed tx is not open (it is nil), it will open a new transaction on db and insert full validator set
func (s *StakeStore) insertFullValidatorSet(fullValidatorSet validatorSetState, dbTx *bolt.Tx) error {
	insertFn := func(tx *bolt.Tx) error {
		raw, err := fullValidatorSet.Marshal()
		if err != nil {
			return err
		}

		return tx.Bucket(validatorSetBucket).Put(fullValidatorSetKey, raw)
	}

	if dbTx == nil {
		return s.db.Update(func(tx *bolt.Tx) error {
			return insertFn(tx)
		})
	}

	return insertFn(dbTx)
}

// getFullValidatorSet returns full validator set from its bucket if exists
// If the passed tx is already open (not nil), it will use it to get full validator set
// If the passed tx is not open (it is nil), it will open a new transaction on db and get full validator set
func (s *StakeStore) getFullValidatorSet(dbTx *bolt.Tx) (validatorSetState, error) {
	var (
		fullValidatorSet validatorSetState
		err              error
	)

	getFn := func(tx *bolt.Tx) error {
		raw := tx.Bucket(validatorSetBucket).Get(fullValidatorSetKey)
		if raw == nil {
			return errNoFullValidatorSet
		}

		return fullValidatorSet.Unmarshal(raw)
	}

	if dbTx == nil {
		err = s.db.View(func(tx *bolt.Tx) error {
			return getFn(tx)
		})
	} else {
		err = getFn(dbTx)
	}

	return fullValidatorSet, err
}

// stakingEventMarshalFormat is intended only for internal use (serialization).
type stakingEventMarshalFormat struct {
	Counter   int
	Validator types.Address
	Amount    *big.Int
}

// insertStakingEvent inserts a staking-related (StakeAdded/StakeRemoved) event into the store.
func (s *StakeStore) insertStakingEvent(event contractsapi.EventAbi, dbTx *bolt.Tx) error {
	if event == nil {
		return nil
	}

	var err error

	insertFn := func(tx *bolt.Tx) error {
		var bucket *bolt.Bucket
		stakingEvent := stakingEventMarshalFormat{Counter: 1}

		switch v := event.(type) {
		case *contractsapi.StakeAddedEvent:
			bucket = tx.Bucket(stakingBucket).Bucket(stakeSubBucket)
			stakingEvent.Validator = v.Validator
			stakingEvent.Amount = v.Amount
		case *contractsapi.StakeRemovedEvent:
			bucket = tx.Bucket(stakingBucket).Bucket(unstakeSubBucket)
			stakingEvent.Validator = v.Validator
			stakingEvent.Amount = v.Amount
		}

		raw := bucket.Get(stakingEvent.Validator.Bytes())
		if raw != nil {
			if err := json.Unmarshal(raw, &stakingEvent); err != nil {
				return err
			}

			stakingEvent.Counter++
		}

		raw, err := json.Marshal(stakingEvent)
		if err != nil {
			return err
		}

		err = bucket.Put(stakingEvent.Validator.Bytes(), raw)
		if err != nil {
			return err
		}

		return nil
	}

	if dbTx == nil {
		err = s.db.Update(insertFn)
	} else {
		err = insertFn(dbTx)
	}

	return err
}

func getStakingEventHelperFn(
	addr types.Address,
	bucket *bolt.Bucket) (*stakingEventMarshalFormat, error) {
	raw := bucket.Get(addr.Bytes())
	if raw == nil {
		return nil, nil
	}

	var stakingEvent stakingEventMarshalFormat

	if err := json.Unmarshal(raw, &stakingEvent); err != nil {
		return nil, err
	}

	stakingEvent.Counter--

	if stakingEvent.Counter == 0 {
		if err := bucket.Delete(addr.Bytes()); err != nil {
			return nil, err
		}

		return &stakingEvent, nil
	}

	raw, err := json.Marshal(stakingEvent)
	if err != nil {
		return nil, err
	}

	err = bucket.Put(addr.Bytes(), raw)
	if err != nil {
		return nil, err
	}

	return &stakingEvent, nil
}

// getStakeAddedEvent retrieves and removes a StakeAdded event from the store for the given address.
// If the event does not exist, it returns nil.
func (s *StakeStore) getStakeAddedEvent(
	addr types.Address,
	dbTx *bolt.Tx) (*contractsapi.StakeAddedEvent, error) {

	var (
		event *contractsapi.StakeAddedEvent
		err   error
	)

	getFn := func(tx *bolt.Tx) error {
		stakingEvent, err := getStakingEventHelperFn(addr, tx.Bucket(stakingBucket).Bucket(stakeSubBucket))
		if err != nil {
			return err
		}

		if stakingEvent == nil {
			return nil
		}

		event = &contractsapi.StakeAddedEvent{
			Validator: stakingEvent.Validator,
			Amount:    stakingEvent.Amount,
		}

		return nil
	}

	if dbTx == nil {
		err = s.db.Update(getFn)
	} else {
		err = getFn(dbTx)
	}

	return event, err
}

// getStakeRemovedEvent retrieves and removes a StakeRemoved event from the store for the given address.
// If the event does not exist, it returns nil.
func (s *StakeStore) getStakeRemovedEvent(
	addr types.Address,
	dbTx *bolt.Tx) (*contractsapi.StakeRemovedEvent, error) {

	var (
		event *contractsapi.StakeRemovedEvent
		err   error
	)

	getFn := func(tx *bolt.Tx) error {
		stakingEvent, err := getStakingEventHelperFn(addr, tx.Bucket(stakingBucket).Bucket(unstakeSubBucket))
		if err != nil {
			return err
		}

		if stakingEvent == nil {
			return nil
		}

		event = &contractsapi.StakeRemovedEvent{
			Validator: stakingEvent.Validator,
			Amount:    stakingEvent.Amount,
		}

		return nil
	}

	if dbTx == nil {
		err = s.db.Update(getFn)
	} else {
		err = getFn(dbTx)
	}

	return event, err
}
