package bls

import (
	"crypto/rand"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	bn256 "github.com/umbracle/go-eth-bn256"
)

// TestAggregateAffineFuzzy generates random G2 public keys (including duplicates
// and occasional infinity points) and verifies that Aggregate() and
// AggregateAffine() produce identical results.
func TestAggregateAffineFuzzy(t *testing.T) {
	const (
		iterations = 5000
		maxKeys    = 12
		minKeys    = 4
	)

	for i := 0; i < iterations; i++ {
		// choose number of keys [0..maxKeys]
		nBig, err := rand.Int(rand.Reader, big.NewInt(maxKeys+1-minKeys))
		require.NoError(t, err, "rand.Int failed")

		pks := make(PublicKeys, nBig.Int64()+minKeys)

		// sometimes include duplicate or infinity
		for j := range pks {
			var g2 *bn256.G2

			r, err := rand.Int(rand.Reader, big.NewInt(100))
			require.NoError(t, err, "rand.Int failed")
			// 5%: insert infinity
			if r.Int64() < 5 {
				// must use Marshal to get a valid infinity point
				g2 = new(bn256.G2)

				// zero value is infinity - we need to create &twistPoint{} somehow to get a valid infinity point
				g2.Marshal()
			} else {
				dupRand, err := rand.Int(rand.Reader, big.NewInt(100))
				require.NoError(t, err, "rand.Int failed")
				// 10%: duplicate a previous one when available
				if j > 0 && dupRand.Int64() < 10 {
					idx, err := rand.Int(rand.Reader, big.NewInt(int64(j)))
					require.NoError(t, err, "rand.Int failed")

					g2 = pks[int(idx.Int64())].g2
				} else {
					_, g2, err = bn256.RandomG2(rand.Reader)
					require.NoError(t, err, "RandomG2 failed")
				}
			}

			pks[j] = &PublicKey{g2: g2}
		}

		a := pks.Aggregate()
		b := pks.AggregateAffine()

		ab, bb := a.Marshal(), b.Marshal()
		require.Equal(t, ab, bb, "mismatch on iter %d keys=%d", i, len(pks))
	}
}
