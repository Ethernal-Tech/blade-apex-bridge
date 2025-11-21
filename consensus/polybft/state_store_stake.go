package polybft

import (
	"errors"
	"fmt"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/validator"
	bolt "go.etcd.io/bbolt"
)

var (
	// bucket to store full validator set
	validatorSetBucket = []byte("fullValidatorSetBucket")
	// key of the full validator set in bucket
	fullValidatorSetKey = []byte("fullValidatorSet")
	// last delta bucket
	lastDeltaBucket = []byte("lastDeltaBucket")
	// last delta
	lastDeltaKey = []byte("lastDelta")
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

	if _, err := tx.CreateBucketIfNotExists(lastDeltaBucket); err != nil {
		return fmt.Errorf("failed to create bucket=%s: %w", string(lastDeltaBucket), err)
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

func (s *StakeStore) getLastDelta(dbTx *bolt.Tx) (*validator.ValidatorSetDelta, error) {
	var (
		delta = &validator.ValidatorSetDelta{}
		err   error
	)

	getFn := func(tx *bolt.Tx) error {
		raw := tx.Bucket(lastDeltaBucket).Get(lastDeltaKey)
		if raw == nil {
			delta = &validator.ValidatorSetDelta{}

			return nil
		}

		return delta.Unmarshal(raw)
	}

	if dbTx == nil {
		err = s.db.View(func(tx *bolt.Tx) error {
			return getFn(tx)
		})
	} else {
		err = getFn(dbTx)
	}

	return delta, err
}

func (s *StakeStore) insertLastDelta(delta *validator.ValidatorSetDelta, dbTx *bolt.Tx) error {
	insertFn := func(tx *bolt.Tx) error {
		raw, err := delta.Marshal()
		if err != nil {
			return err
		}

		return tx.Bucket(lastDeltaBucket).Put(lastDeltaKey, raw)
	}

	if dbTx == nil {
		return s.db.Update(func(tx *bolt.Tx) error {
			return insertFn(tx)
		})
	}

	return insertFn(dbTx)
}
