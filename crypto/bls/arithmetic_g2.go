package bls

import (
	"math/big"
)

var (
	two256 = new(big.Int).Lsh(big.NewInt(1), 256)
	// pPrime is a prime over which we form a basic field: 36u⁴+36u³+24u²+6u+1.
	pPrime, _ = new(big.Int).SetString(
		"21888242871839275222246405745257275088696311157297823662689037894645226208583", 10)
)

func maskToUint256(x *big.Int) *big.Int {
	if x == nil {
		return new(big.Int)
	}

	r := new(big.Int).Mod(x, two256)
	if r.Sign() < 0 {
		r.Add(r, two256)
	}

	return r
}

// fp2SubAddmod implements Solidity's addmod(a, N - b, N) for Fp2 subtraction.
func fp2SubAddmod(a0, a1, b0, b1 *big.Int) (c0, c1 *big.Int) {
	t0 := new(big.Int).Sub(pPrime, b0)
	t0.Add(t0, a0)
	t0.Mod(t0, pPrime)

	t1 := new(big.Int).Sub(pPrime, b1)
	t1.Add(t1, a1)
	t1.Mod(t1, pPrime)

	return maskToUint256(t0), maskToUint256(t1)
}

func fp2AddInts(a0, a1, b0, b1 *big.Int) (c0, c1 *big.Int) {
	c0 = new(big.Int).Add(a0, b0)
	c0.Mod(c0, pPrime)
	c1 = new(big.Int).Add(a1, b1)
	c1.Mod(c1, pPrime)
	return maskToUint256(c0), maskToUint256(c1)
}

func fp2MulInts(a0, a1, b0, b1 *big.Int) (c0, c1 *big.Int) {
	// (a0 + a1*u)(b0 + b1*u) = (a0*b0 - a1*b1) + (a0*b1 + a1*b0)*u
	t1 := new(big.Int).Mul(a0, b0) // a0*b0
	t2 := new(big.Int).Mul(a1, b1) // a1*b1
	c0 = new(big.Int).Sub(t1, t2)  // a0*b0 - a1*b1
	c0.Mod(c0, pPrime)

	t3 := new(big.Int).Mul(a0, b1) // a0*b1
	t4 := new(big.Int).Mul(a1, b0) // a1*b0
	c1 = new(big.Int).Add(t3, t4)  // a0*b1 + a1*b0
	c1.Mod(c1, pPrime)

	return maskToUint256(c0), maskToUint256(c1)
}

func fp2InvInts(a0, a1 *big.Int) (c0, c1 *big.Int) {
	// (a0 + a1*u)^{-1} = (a0 - a1*u) / (a0² + a1²)
	t1 := new(big.Int).Mul(a0, a0)
	t2 := new(big.Int).Mul(a1, a1)
	denom := new(big.Int).Add(t1, t2) // a0² + a1²
	denom.Mod(denom, pPrime)

	inv := new(big.Int).ModInverse(denom, pPrime)
	if inv == nil {
		return nil, nil
	}

	c0 = new(big.Int).Mul(a0, inv) // a0 / (a0² + a1²)
	c0.Mod(c0, pPrime)

	negA1 := new(big.Int).Neg(a1)     //  -a1
	c1 = new(big.Int).Mul(negA1, inv) // -a1 / (a0² + a1²)
	c1.Mod(c1, pPrime)

	return maskToUint256(c0), maskToUint256(c1)
}

// _doubleG2Point is a 1:1 translation of the Solidity _doubleG2Point
// Input/output format: [x0, x1, y0, y1]
func _doubleG2Point(p [4]*big.Int) [4]*big.Int {
	// (uint256 a0, a1) = _fp2Mul(p[0], p[1], p[0], p[1]);
	a0, a1 := fp2MulInts(p[0], p[1], p[0], p[1])
	// (uint256 b0, b1) = _fp2Add(a0, a1, a0, a1);
	b0, b1 := fp2AddInts(a0, a1, a0, a1)
	// (a0, a1) = _fp2Add(b0, b1, a0, a1);
	a0, a1 = fp2AddInts(b0, b1, a0, a1)
	// (b0, b1) = _fp2Add(p[2], p[3], p[2], p[3]);
	b0, b1 = fp2AddInts(p[2], p[3], p[2], p[3])
	// (b0, b1) = _fp2Inv(b0, b1);
	ib0, ib1 := fp2InvInts(b0, b1)
	if ib0 == nil {
		return zeroArr()
	}

	b0, b1 = ib0, ib1
	// (a0, a1) = _fp2Mul(a0, a1, b0, b1);
	a0, a1 = fp2MulInts(a0, a1, b0, b1)

	// (b0, b1) = _fp2Mul(a0, a1, a0, a1);
	b0, b1 = fp2MulInts(a0, a1, a0, a1)
	// (b0, b1) = _fp2Sub(b0, b1, p[0], p[1]);
	b0, b1 = fp2SubAddmod(b0, b1, p[0], p[1])
	// (b0, b1) = _fp2Sub(b0, b1, p[0], p[1]);
	b0, b1 = fp2SubAddmod(b0, b1, p[0], p[1])

	// (uint256 c0, c1) = _fp2Sub(p[0], p[1], b0, b1);
	c0, c1 := fp2SubAddmod(p[0], p[1], b0, b1)
	// (c0, c1) = _fp2Mul(a0, a1, c0, c1);
	c0, c1 = fp2MulInts(a0, a1, c0, c1)
	// (c0, c1) = _fp2Sub(c0, c1, p[2], p[3]);
	c0, c1 = fp2SubAddmod(c0, c1, p[2], p[3])

	return [4]*big.Int{
		maskToUint256(b0), maskToUint256(b1),
		maskToUint256(c0), maskToUint256(c1),
	}
}

// _addG2Points is a 1:1 translation of the Solidity _addG2Points function.
func _addG2Points(p [4]*big.Int, q [4]*big.Int) [4]*big.Int {
	// Check for point at infinity
	if p[0].Sign() == 0 && p[1].Sign() == 0 &&
		p[2].Sign() == 0 && p[3].Sign() == 0 {
		return q
	}

	if q[0].Sign() == 0 && q[1].Sign() == 0 &&
		q[2].Sign() == 0 && q[3].Sign() == 0 {
		return p
	}

	// Check if x coordinates are equal
	if p[0].Cmp(q[0]) == 0 && p[1].Cmp(q[1]) == 0 {
		if p[2].Cmp(q[2]) == 0 && p[3].Cmp(q[3]) == 0 {
			// Same point: perform doubling
			if p[2].Sign() == 0 && p[3].Sign() == 0 {
				return zeroArr()
			}

			return _doubleG2Point(p)
		} else {
			// P + (-P) = point at infinity
			return zeroArr()
		}
	}

	// General case: lambda = (qy - py) / (qx - px)
	a0, a1 := fp2SubAddmod(q[2], q[3], p[2], p[3])
	b0, b1 := fp2SubAddmod(q[0], q[1], p[0], p[1])

	ib0, ib1 := fp2InvInts(b0, b1)
	if ib0 == nil {
		return zeroArr()
	}

	a0, a1 = fp2MulInts(a0, a1, ib0, ib1)

	// rx = lambda^2 - px - qx
	b0, b1 = fp2MulInts(a0, a1, a0, a1)
	b0, b1 = fp2SubAddmod(b0, b1, p[0], p[1])
	b0, b1 = fp2SubAddmod(b0, b1, q[0], q[1])

	// ry = lambda * (px - rx) - py
	c0, c1 := fp2SubAddmod(p[0], p[1], b0, b1)
	c0, c1 = fp2MulInts(a0, a1, c0, c1)
	c0, c1 = fp2SubAddmod(c0, c1, p[2], p[3])

	return [4]*big.Int{
		maskToUint256(b0), maskToUint256(b1),
		maskToUint256(c0), maskToUint256(c1),
	}
}

func zeroArr() [4]*big.Int {
	return [4]*big.Int{new(big.Int), new(big.Int), new(big.Int), new(big.Int)}
}
