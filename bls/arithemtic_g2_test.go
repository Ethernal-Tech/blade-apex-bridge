package bls

import (
	"crypto/rand"
	"encoding/hex"
	"math/big"
	mrand "math/rand"
	"testing"

	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/stretchr/testify/require"
	bn256 "github.com/umbracle/go-eth-bn256"
)

// TestAggregateAffineSeeded is a deterministic version of TestAggregateAffineFuzzy
// that uses a fixed seed so failures are reproducible.
func TestAggregateAffineSeeded(t *testing.T) {
	const (
		iterations = 1000
		maxKeys    = 12
		minKeys    = 4
		seed       = 42
	)

	rng := mrand.New(mrand.NewSource(seed)) //nolint:gosec

	for i := 0; i < iterations; i++ {
		nKeys := minKeys + rng.Intn(maxKeys-minKeys+1)
		pks := make(PublicKeys, nKeys)

		for j := range pks {
			var g2 *bn256.G2

			r := rng.Intn(100)
			if r < 5 {
				g2 = new(bn256.G2)
				g2.Marshal()
			} else if j > 0 && rng.Intn(100) < 10 {
				idx := rng.Intn(j)
				g2 = pks[idx].g2
			} else {
				scalar := big.NewInt(rng.Int63() + 1)
				g2 = new(bn256.G2).ScalarBaseMult(scalar)
			}

			pks[j] = &PublicKey{g2: g2}
		}

		a := pks.Aggregate()
		b := pks.AggregateAffine()

		if string(a.Marshal()) != string(b.Marshal()) {
			// Trace the failing iteration step by step.
			t.Logf("FAIL iter=%d keys=%d — tracing:", i, nKeys)
			bn256Acc := new(bn256.G2)
			affineAcc := [4]*big.Int{new(big.Int), new(big.Int), new(big.Int), new(big.Int)}

			for step, pk := range pks {
				buf := pk.g2.Marshal()
				var cur [4]*big.Int
				if len(buf) == 128 {
					cur = [4]*big.Int{
						new(big.Int).SetBytes(buf[32:64]),
						new(big.Int).SetBytes(buf[0:32]),
						new(big.Int).SetBytes(buf[96:128]),
						new(big.Int).SetBytes(buf[64:96]),
					}
				} else {
					cur = [4]*big.Int{new(big.Int), new(big.Int), new(big.Int), new(big.Int)}
				}

				bn256Acc.Add(bn256Acc, pk.g2)
				affineAcc = _addG2Points(affineAcc, cur)

				// Compare at this step.
				bn256Buf := bn256Acc.Marshal()
				var bn256Coords [4]*big.Int
				if len(bn256Buf) == 128 {
					bn256Coords = [4]*big.Int{
						new(big.Int).SetBytes(bn256Buf[32:64]),
						new(big.Int).SetBytes(bn256Buf[0:32]),
						new(big.Int).SetBytes(bn256Buf[96:128]),
						new(big.Int).SetBytes(bn256Buf[64:96]),
					}
				} else {
					bn256Coords = [4]*big.Int{new(big.Int), new(big.Int), new(big.Int), new(big.Int)}
				}

				match := bn256Coords[0].Cmp(affineAcc[0]) == 0 &&
					bn256Coords[1].Cmp(affineAcc[1]) == 0 &&
					bn256Coords[2].Cmp(affineAcc[2]) == 0 &&
					bn256Coords[3].Cmp(affineAcc[3]) == 0
				t.Logf("  step %2d: len(marshal)=%d match=%v", step, len(buf), match)
				if !match {
					t.Logf("    key:         x0=%x x1=%x y0=%x y1=%x", cur[0], cur[1], cur[2], cur[3])
					t.Logf("    affineAcc:   x0=%x x1=%x y0=%x y1=%x", affineAcc[0], affineAcc[1], affineAcc[2], affineAcc[3])
					t.Logf("    bn256 after: x0=%x x1=%x y0=%x y1=%x", bn256Coords[0], bn256Coords[1], bn256Coords[2], bn256Coords[3])
					t.Logf("    affine after:x0=%x x1=%x y0=%x y1=%x", affineAcc[0], affineAcc[1], affineAcc[2], affineAcc[3])
					break
				}
			}
		}

		require.Equal(t, a.Marshal(), b.Marshal(),
			"mismatch on iter %d keys=%d", i, nKeys)
	}
}

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

// mustHexToBigInt is a test helper that parses a hex string (without 0x prefix) into *big.Int.
// Panics on invalid input so test failures are immediately obvious.
func mustHexToBigInt(s string) *big.Int {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic("mustHexToBigInt: invalid hex string: " + s + ": " + err.Error())
	}

	return new(big.Int).SetBytes(b)
}

// assertBigIntArrayEqual compares two [4]*big.Int arrays element by element,
// printing the expected and actual hex values for each component on failure.
func assertBigIntArrayEqual(t *testing.T, expected, actual [4]*big.Int) {
	t.Helper()

	labels := []string{"x0", "x1", "y0", "y1"}

	for i := 0; i < 4; i++ {
		exp := expected[i]
		got := actual[i]

		if got == nil {
			got = new(big.Int)
		}

		require.Equal(t, 0, exp.Cmp(got),
			"component %s mismatch:\n  expected: %064x\n  actual:   %064x",
			labels[i], exp, got,
		)
	}
}

func TestAddG2Points(t *testing.T) {
	// ---------------------------------------------------------------------------
	// Validator 0 public key (TypeScript validatorsData[0].key)
	// [x0, x1, y0, y1] in Solidity/affine Fp2 format
	// ---------------------------------------------------------------------------
	// 0x2e6ae79fad74905b4391c4258eee0ed911a83ea8b2ee4b830fe8cbcdd50645fe
	// 0x2036180907f4381ffafc6def06892e3265c0fc9d8f5cb8eaf5a43386a2c22943
	// 0x2045e6ab998872fd13c0d9a62761ef9b99452e1d54dc0958c9c7d19afec6efb0
	// 0x29e8001b664baeaf0cc9fb1e9332806fed04794c5c6aaf9937051d40725aeeb5
	p1 := [4]*big.Int{
		mustHexToBigInt("2e6ae79fad74905b4391c4258eee0ed911a83ea8b2ee4b830fe8cbcdd50645fe"),
		mustHexToBigInt("2036180907f4381ffafc6def06892e3265c0fc9d8f5cb8eaf5a43386a2c22943"),
		mustHexToBigInt("2045e6ab998872fd13c0d9a62761ef9b99452e1d54dc0958c9c7d19afec6efb0"),
		mustHexToBigInt("29e8001b664baeaf0cc9fb1e9332806fed04794c5c6aaf9937051d40725aeeb5"),
	}

	// ---------------------------------------------------------------------------
	// Validator 1 public key (TypeScript validatorsData[1].key)
	// ---------------------------------------------------------------------------
	// 0x12e36a5325e37611faeb493d583dab3bf396a75910272d44667f85360501b54d
	// 0x1072541de938896039f580f1a7fcb7eb0542962fcca5691cd4933de5502d2746
	// 0x20fbb3b94c9ce874bf868e6958ad049bc60c7474ab49f7f06cf76c03b5e66e09
	// 0x2f2a1a81bf575571a67ae3aef8bef9f2cafdbcdc03cdaa35566d81686d709295
	p2 := [4]*big.Int{
		mustHexToBigInt("12e36a5325e37611faeb493d583dab3bf396a75910272d44667f85360501b54d"),
		mustHexToBigInt("1072541de938896039f580f1a7fcb7eb0542962fcca5691cd4933de5502d2746"),
		mustHexToBigInt("20fbb3b94c9ce874bf868e6958ad049bc60c7474ab49f7f06cf76c03b5e66e09"),
		mustHexToBigInt("2f2a1a81bf575571a67ae3aef8bef9f2cafdbcdc03cdaa35566d81686d709295"),
	}

	// ---------------------------------------------------------------------------
	// Expected result from TypeScript blsVerifierTest.addG2Points(p1, p2):
	// x0: 050c69b8424ed7897c638d852b6a01523d6d31cbf17c0e4c23c5947e72419792
	// x1: 04c2b1da74e2a1c5139431ff0b96ba6a6301adfa7a0c472dc2e0f39ef099442a
	// y0: 250b9887c1a5a5ff5c4ec5ea08d3ad20146d1cdebd9feaf3e7665294a59e17b8
	// y1: 21c0ed5230eda60e3f9d75f60d03c36251881e7523009b84a2bf5657820ac3b8
	// ---------------------------------------------------------------------------
	expected := [4]*big.Int{
		mustHexToBigInt("050c69b8424ed7897c638d852b6a01523d6d31cbf17c0e4c23c5947e72419792"),
		mustHexToBigInt("04c2b1da74e2a1c5139431ff0b96ba6a6301adfa7a0c472dc2e0f39ef099442a"),
		mustHexToBigInt("250b9887c1a5a5ff5c4ec5ea08d3ad20146d1cdebd9feaf3e7665294a59e17b8"),
		mustHexToBigInt("21c0ed5230eda60e3f9d75f60d03c36251881e7523009b84a2bf5657820ac3b8"),
	}

	res := _addG2Points(p1, p2)

	assertBigIntArrayEqual(t, expected, res)
}

func TestDoubleG2Point(t *testing.T) {
	// Helper: compare _doubleG2Point(p) against bn256.G2.Add(g2raw, g2raw).
	checkDouble := func(t *testing.T, g2raw *bn256.G2, p [4]*big.Int) {
		t.Helper()
		doubled := new(bn256.G2).Add(g2raw, g2raw)
		dbuf := doubled.Marshal()
		require.Len(t, dbuf, 128, "doubled point should be 128 bytes")
		expected := [4]*big.Int{
			new(big.Int).SetBytes(dbuf[32:64]),
			new(big.Int).SetBytes(dbuf[0:32]),
			new(big.Int).SetBytes(dbuf[96:128]),
			new(big.Int).SetBytes(dbuf[64:96]),
		}
		got := _doubleG2Point(p)
		if got[0].Cmp(expected[0]) != 0 || got[1].Cmp(expected[1]) != 0 ||
			got[2].Cmp(expected[2]) != 0 || got[3].Cmp(expected[3]) != 0 {
			t.Logf("_doubleG2Point mismatch:")
			t.Logf("  input  x0=%x", p[0])
			t.Logf("  input  x1=%x", p[1])
			t.Logf("  input  y0=%x", p[2])
			t.Logf("  input  y1=%x", p[3])
			t.Logf("  expect x0=%x", expected[0])
			t.Logf("  expect x1=%x", expected[1])
			t.Logf("  expect y0=%x", expected[2])
			t.Logf("  expect y1=%x", expected[3])
			t.Logf("  got    x0=%x", got[0])
			t.Logf("  got    x1=%x", got[1])
			t.Logf("  got    y0=%x", got[2])
			t.Logf("  got    y1=%x", got[3])
		}
		assertBigIntArrayEqual(t, expected, got)
	}

	unmarshalG2 := func(t *testing.T, p [4]*big.Int) *bn256.G2 {
		t.Helper()
		g := new(bn256.G2)
		_, err := g.Unmarshal(append(
			common.PadLeftOrTrim(p[1].Bytes(), 32),
			append(
				common.PadLeftOrTrim(p[0].Bytes(), 32),
				append(
					common.PadLeftOrTrim(p[3].Bytes(), 32),
					common.PadLeftOrTrim(p[2].Bytes(), 32)...,
				)...,
			)...,
		))
		require.NoError(t, err)
		return g
	}

	// Case 1: validator 0 key (known-good baseline).
	p1 := [4]*big.Int{
		mustHexToBigInt("2e6ae79fad74905b4391c4258eee0ed911a83ea8b2ee4b830fe8cbcdd50645fe"),
		mustHexToBigInt("2036180907f4381ffafc6def06892e3265c0fc9d8f5cb8eaf5a43386a2c22943"),
		mustHexToBigInt("2045e6ab998872fd13c0d9a62761ef9b99452e1d54dc0958c9c7d19afec6efb0"),
		mustHexToBigInt("29e8001b664baeaf0cc9fb1e9332806fed04794c5c6aaf9937051d40725aeeb5"),
	}
	checkDouble(t, unmarshalG2(t, p1), p1)

	// Case 2: exact key from TestAggregateAffineSeeded failure (seed=42, iter=2, step=1).
	p2 := [4]*big.Int{
		mustHexToBigInt("2f4e735b7f1d818bc926f385488fec1af1d8071294b47e210a3371d5488f08ad"),
		mustHexToBigInt("12de23ae132f3f684643db2c41eeb239d78e211835d1b3407c14dfab78edcb7e"),
		mustHexToBigInt("2414a6b714afab98b028f2117b5bbb01f631f369b770cad161be4e82b9fd7f42"),
		mustHexToBigInt("2ff8fe0cfec5748d6e70737c4428cea0a4b5b37164c1c74d38f9fb5060e96c5e"),
	}
	checkDouble(t, unmarshalG2(t, p2), p2)

	// Case 2: 50 random keys via deterministic seed — catches the bug that
	// _doubleG2Point produces wrong results for some inputs.
	rng := mrand.New(mrand.NewSource(42)) //nolint:gosec
	for k := 0; k < 50; k++ {
		scalar := big.NewInt(rng.Int63() + 1)
		g2raw := new(bn256.G2).ScalarBaseMult(scalar)
		buf := g2raw.Marshal()
		require.Len(t, buf, 128)
		p := [4]*big.Int{
			new(big.Int).SetBytes(buf[32:64]),
			new(big.Int).SetBytes(buf[0:32]),
			new(big.Int).SetBytes(buf[96:128]),
			new(big.Int).SetBytes(buf[64:96]),
		}
		checkDouble(t, g2raw, p)
	}
}

// TestChainAddG2Points verifies that chaining multiple _addG2Points calls
// matches bn256 native addition.
// TestAddG2PointsDuplicate replicates iter=2, steps 0-1 from TestAggregateAffineSeeded
// to isolate whether _addG2Points handles the identity→P→P+P sequence correctly.
func TestAddG2PointsDuplicate(t *testing.T) {
	P := [4]*big.Int{
		mustHexToBigInt("2f4e735b7f1d818bc926f385488fec1af1d8071294b47e210a3371d5488f08ad"),
		mustHexToBigInt("12de23ae132f3f684643db2c41eeb239d78e211835d1b3407c14dfab78edcb7e"),
		mustHexToBigInt("2414a6b714afab98b028f2117b5bbb01f631f369b770cad161be4e82b9fd7f42"),
		mustHexToBigInt("2ff8fe0cfec5748d6e70737c4428cea0a4b5b37164c1c74d38f9fb5060e96c5e"),
	}

	// Build expected 2P via bn256.
	g2p := new(bn256.G2)
	_, err := g2p.Unmarshal(append(
		common.PadLeftOrTrim(P[1].Bytes(), 32),
		append(
			common.PadLeftOrTrim(P[0].Bytes(), 32),
			append(
				common.PadLeftOrTrim(P[3].Bytes(), 32),
				common.PadLeftOrTrim(P[2].Bytes(), 32)...,
			)...,
		)...,
	))
	require.NoError(t, err)
	dbuf := new(bn256.G2).Add(g2p, g2p).Marshal()
	expected2P := [4]*big.Int{
		new(big.Int).SetBytes(dbuf[32:64]),
		new(big.Int).SetBytes(dbuf[0:32]),
		new(big.Int).SetBytes(dbuf[96:128]),
		new(big.Int).SetBytes(dbuf[64:96]),
	}

	// Step 0: accumulate identity + P.
	acc := [4]*big.Int{new(big.Int), new(big.Int), new(big.Int), new(big.Int)}
	acc = _addG2Points(acc, P)

	// Step 1: accumulate P + P (duplicate, fresh big.Int copies with same values).
	P2 := [4]*big.Int{
		new(big.Int).Set(P[0]),
		new(big.Int).Set(P[1]),
		new(big.Int).Set(P[2]),
		new(big.Int).Set(P[3]),
	}
	result := _addG2Points(acc, P2)
	assertBigIntArrayEqual(t, expected2P, result)
}

func TestChainAddG2Points(t *testing.T) {
	makeCoords := func(buf []byte) [4]*big.Int {
		return [4]*big.Int{
			new(big.Int).SetBytes(buf[32:64]),
			new(big.Int).SetBytes(buf[0:32]),
			new(big.Int).SetBytes(buf[96:128]),
			new(big.Int).SetBytes(buf[64:96]),
		}
	}

	// Use validator 0 and 1 plus a third point (their known sum) as inputs.
	p1 := [4]*big.Int{
		mustHexToBigInt("2e6ae79fad74905b4391c4258eee0ed911a83ea8b2ee4b830fe8cbcdd50645fe"),
		mustHexToBigInt("2036180907f4381ffafc6def06892e3265c0fc9d8f5cb8eaf5a43386a2c22943"),
		mustHexToBigInt("2045e6ab998872fd13c0d9a62761ef9b99452e1d54dc0958c9c7d19afec6efb0"),
		mustHexToBigInt("29e8001b664baeaf0cc9fb1e9332806fed04794c5c6aaf9937051d40725aeeb5"),
	}
	p2 := [4]*big.Int{
		mustHexToBigInt("12e36a5325e37611faeb493d583dab3bf396a75910272d44667f85360501b54d"),
		mustHexToBigInt("1072541de938896039f580f1a7fcb7eb0542962fcca5691cd4933de5502d2746"),
		mustHexToBigInt("20fbb3b94c9ce874bf868e6958ad049bc60c7474ab49f7f06cf76c03b5e66e09"),
		mustHexToBigInt("2f2a1a81bf575571a67ae3aef8bef9f2cafdbcdc03cdaa35566d81686d709295"),
	}

	unmarshalG2 := func(c [4]*big.Int) *bn256.G2 {
		g := new(bn256.G2)
		_, err := g.Unmarshal(append(
			common.PadLeftOrTrim(c[1].Bytes(), 32),
			append(
				common.PadLeftOrTrim(c[0].Bytes(), 32),
				append(
					common.PadLeftOrTrim(c[3].Bytes(), 32),
					common.PadLeftOrTrim(c[2].Bytes(), 32)...,
				)...,
			)...,
		))
		require.NoError(t, err)
		return g
	}

	g2p1, g2p2 := unmarshalG2(p1), unmarshalG2(p2)

	// Generate a third random G2 point deterministically (3 * generator).
	g2p3 := new(bn256.G2).ScalarBaseMult(big.NewInt(12345678))
	p3 := makeCoords(g2p3.Marshal())

	// bn256 native: P1 + P2 + P3
	sum := new(bn256.G2).Add(g2p1, g2p2)
	sum = new(bn256.G2).Add(sum, g2p3)
	wantBuf := sum.Marshal()
	require.Len(t, wantBuf, 128)
	want := makeCoords(wantBuf)

	// Affine accumulation
	acc := [4]*big.Int{new(big.Int), new(big.Int), new(big.Int), new(big.Int)}
	acc = _addG2Points(acc, p1)
	acc = _addG2Points(acc, p2)
	acc = _addG2Points(acc, p3)

	assertBigIntArrayEqual(t, want, acc)
}

func TestAddG2Points1(t *testing.T) {
	// ---------------------------------------------------------------------------
	// Validator 0 public key (TypeScript validatorsData[0].key)
	// [x0, x1, y0, y1] in Solidity/affine Fp2 format
	// ---------------------------------------------------------------------------
	// 0x2e6ae79fad74905b4391c4258eee0ed911a83ea8b2ee4b830fe8cbcdd50645fe
	// 0x2036180907f4381ffafc6def06892e3265c0fc9d8f5cb8eaf5a43386a2c22943
	// 0x2045e6ab998872fd13c0d9a62761ef9b99452e1d54dc0958c9c7d19afec6efb0
	// 0x29e8001b664baeaf0cc9fb1e9332806fed04794c5c6aaf9937051d40725aeeb5
	p1 := [4]*big.Int{
		mustHexToBigInt("2e6ae79fad74905b4391c4258eee0ed911a83ea8b2ee4b830fe8cbcdd50645fe"),
		mustHexToBigInt("2036180907f4381ffafc6def06892e3265c0fc9d8f5cb8eaf5a43386a2c22943"),
		mustHexToBigInt("2045e6ab998872fd13c0d9a62761ef9b99452e1d54dc0958c9c7d19afec6efb0"),
		mustHexToBigInt("29e8001b664baeaf0cc9fb1e9332806fed04794c5c6aaf9937051d40725aeeb5"),
	}

	// ---------------------------------------------------------------------------
	// Validator 1 public key (TypeScript validatorsData[1].key)
	// ---------------------------------------------------------------------------
	// 0x12e36a5325e37611faeb493d583dab3bf396a75910272d44667f85360501b54d
	// 0x1072541de938896039f580f1a7fcb7eb0542962fcca5691cd4933de5502d2746
	// 0x20fbb3b94c9ce874bf868e6958ad049bc60c7474ab49f7f06cf76c03b5e66e09
	// 0x2f2a1a81bf575571a67ae3aef8bef9f2cafdbcdc03cdaa35566d81686d709295
	p2 := [4]*big.Int{
		mustHexToBigInt("12e36a5325e37611faeb493d583dab3bf396a75910272d44667f85360501b54d"),
		mustHexToBigInt("1072541de938896039f580f1a7fcb7eb0542962fcca5691cd4933de5502d2746"),
		mustHexToBigInt("20fbb3b94c9ce874bf868e6958ad049bc60c7474ab49f7f06cf76c03b5e66e09"),
		mustHexToBigInt("2f2a1a81bf575571a67ae3aef8bef9f2cafdbcdc03cdaa35566d81686d709295"),
	}

	// ---------------------------------------------------------------------------
	// Expected result from TypeScript blsVerifierTest.addG2Points(p1, p2):
	// x0: 050c69b8424ed7897c638d852b6a01523d6d31cbf17c0e4c23c5947e72419792
	// x1: 04c2b1da74e2a1c5139431ff0b96ba6a6301adfa7a0c472dc2e0f39ef099442a
	// y0: 250b9887c1a5a5ff5c4ec5ea08d3ad20146d1cdebd9feaf3e7665294a59e17b8
	// y1: 21c0ed5230eda60e3f9d75f60d03c36251881e7523009b84a2bf5657820ac3b8
	// ---------------------------------------------------------------------------
	expected := [4]*big.Int{
		mustHexToBigInt("050c69b8424ed7897c638d852b6a01523d6d31cbf17c0e4c23c5947e72419792"),
		mustHexToBigInt("04c2b1da74e2a1c5139431ff0b96ba6a6301adfa7a0c472dc2e0f39ef099442a"),
		mustHexToBigInt("250b9887c1a5a5ff5c4ec5ea08d3ad20146d1cdebd9feaf3e7665294a59e17b8"),
		mustHexToBigInt("21c0ed5230eda60e3f9d75f60d03c36251881e7523009b84a2bf5657820ac3b8"),
	}
	// make a *bn256.G2 based on p1
	a := new(bn256.G2)
	_, err := a.Unmarshal(append(
		common.PadLeftOrTrim(p1[1].Bytes(), 32),
		append(
			common.PadLeftOrTrim(p1[0].Bytes(), 32),
			append(
				common.PadLeftOrTrim(p1[3].Bytes(), 32),
				common.PadLeftOrTrim(p1[2].Bytes(), 32)...,
			)...,
		)...,
	))
	require.NoError(t, err, "failed to unmarshal p1")

	// make a *bn256.G2 based on p2
	b := new(bn256.G2)
	_, err = b.Unmarshal(append(
		common.PadLeftOrTrim(p2[1].Bytes(), 32),
		append(
			common.PadLeftOrTrim(p2[0].Bytes(), 32),
			append(
				common.PadLeftOrTrim(p2[3].Bytes(), 32),
				common.PadLeftOrTrim(p2[2].Bytes(), 32)...,
			)...,
		)...,
	))
	require.NoError(t, err, "failed to unmarshal p2")

	// add the two points using the bn256 library
	res := new(bn256.G2).Add(a, b)

	// expected result as *bn256.G2
	expectedG2 := new(bn256.G2)
	_, err = expectedG2.Unmarshal(append(
		common.PadLeftOrTrim(expected[1].Bytes(), 32),
		append(
			common.PadLeftOrTrim(expected[0].Bytes(), 32),
			append(
				common.PadLeftOrTrim(expected[3].Bytes(), 32),
				common.PadLeftOrTrim(expected[2].Bytes(), 32)...,
			)...,
		)...,
	))
	require.NoError(t, err, "failed to unmarshal expected result")

	// verify the result matches expected
	require.Equal(t, expectedG2.Marshal(), res.Marshal(), "result does not match expected")

}
