package bls

import (
	"crypto/rand"
	"math/big"
	"testing"

	bn256eth "github.com/Ethernal-Tech/bn256"
	bn256 "github.com/Ethernal-Tech/bn256/cloudflare"
	"github.com/stretchr/testify/require"
)

// Test_AddG2Points_Fuzzy generates random G2 public keys (including duplicates)
// and verifies that Aggregate() and aggregateAffine() produce identical results.
func Test_AddG2Points_Fuzzy(t *testing.T) {
	t.Parallel()

	const (
		iterations = 1000
		maxKeys    = 12
		minKeys    = 4
	)

	for i := range iterations {
		nBig, err := rand.Int(rand.Reader, big.NewInt(maxKeys+1-minKeys))
		require.NoError(t, err, "rand.Int failed")

		g2s := make([]*bn256.G2, nBig.Int64()+minKeys)

		for j := range g2s {
			dupRand, err := rand.Int(rand.Reader, big.NewInt(100))
			require.NoError(t, err, "rand.Int failed")
			// 10%: duplicate a previous one when available
			if j > 0 && dupRand.Int64() < 10 {
				idx, err := rand.Int(rand.Reader, big.NewInt(int64(j)))
				require.NoError(t, err, "rand.Int failed")

				g2s[j] = g2s[idx.Int64()]
			} else {
				_, g2s[j], err = bn256.RandomG2(rand.Reader)
				require.NoError(t, err, "RandomG2 failed")
			}
		}

		pks := make(bn256eth.PublicKeys, len(g2s))
		for j, g2 := range g2s {
			pks[j], err = bn256eth.UnmarshalPublicKey(g2.Marshal())
			require.NoError(t, err)
		}

		a := pks.Aggregate()

		b, err := aggregateAffine(pks)
		require.NoError(t, err)

		ab, bb := a.Marshal(), b.Marshal()
		require.Equal(t, ab, bb, "mismatch on iter %d keys=%d", i, len(pks))
	}
}

func aggregateAffine(pks bn256eth.PublicKeys) (*bn256eth.PublicKey, error) {
	acc := [4]*big.Int{new(big.Int), new(big.Int), new(big.Int), new(big.Int)}

	for _, pk := range pks {
		acc = _addG2Points(acc, pk.ToBigInt())
	}

	return bn256eth.UnmarshalPublicKeyFromBigInt(acc)
}
