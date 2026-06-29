package cardanofw

import "math/big"

const (
	DfmDecimals     = 6
	WeiDecimals     = 18
	LamportDecimals = 9
)

func WeiToChainNativeTokenAmount(chainID string, weiAmount *big.Int) *big.Int {
	if IsCardanoChain(chainID) {
		return WeiToDfm(weiAmount)
	}

	return weiAmount
}

func ChainNativeTokenAmountToWei(chainID string, nativeTokenAmount *big.Int) *big.Int {
	if IsCardanoChain(chainID) {
		return DfmToWei(nativeTokenAmount)
	}

	return nativeTokenAmount
}

func ApexToDfm(apex *big.Int) *big.Int {
	dfm := new(big.Int).Set(apex)
	base := big.NewInt(10)

	return dfm.Mul(dfm, base.Exp(base, big.NewInt(DfmDecimals), nil))
}

func DfmToApex(dfm *big.Int) *big.Int {
	apex := new(big.Int).Set(dfm)
	base := big.NewInt(10)

	return apex.Div(apex, base.Exp(base, big.NewInt(DfmDecimals), nil))
}

func ApexToWei(apex *big.Int) *big.Int {
	wei := new(big.Int).Set(apex)
	base := big.NewInt(10)

	return wei.Mul(wei, base.Exp(base, big.NewInt(WeiDecimals), nil))
}

func DfmToWei(dfm *big.Int) *big.Int {
	wei := new(big.Int).Set(dfm)
	base := big.NewInt(10)

	return wei.Mul(wei, base.Exp(base, big.NewInt(WeiDecimals-DfmDecimals), nil))
}

// LamportToWei converts lamports (9 decimals) to wei (18 decimals): 1 SOL = 10^9 lamports = 10^18 wei.
func LamportToWei(lamport *big.Int) *big.Int {
	wei := new(big.Int).Set(lamport)
	base := big.NewInt(10)

	return wei.Mul(wei, base.Exp(base, big.NewInt(WeiDecimals-LamportDecimals), nil))
}

// WeiToLamport converts wei (18 decimals) to lamports (9 decimals).
func WeiToLamport(wei *big.Int) *big.Int {
	lamport := new(big.Int).Set(wei)
	base := big.NewInt(10)

	return lamport.Div(lamport, base.Exp(base, big.NewInt(WeiDecimals-LamportDecimals), nil))
}

// SolanaToLamport converts SOL (human unit) to lamports: 1 SOL = 10^9 lamports.
func SolanaToLamport(solana *big.Int) *big.Int {
	out := new(big.Int).Set(solana)
	base := big.NewInt(10)

	return out.Mul(out, base.Exp(base, big.NewInt(LamportDecimals), nil))
}

// LamportToSolana converts lamports to SOL: 10^9 lamports = 1 SOL.
func LamportToSolana(lamport *big.Int) *big.Int {
	out := new(big.Int).Set(lamport)
	base := big.NewInt(10)

	return out.Div(out, base.Exp(base, big.NewInt(LamportDecimals), nil))
}

func SolanaToWei(solana *big.Int) *big.Int {
	out := new(big.Int).Set(solana)
	base := big.NewInt(10)

	return out.Mul(out, base.Exp(base, big.NewInt(WeiDecimals), nil))
}

func WeiToSolana(wei *big.Int) *big.Int {
	out := new(big.Int).Set(wei)
	base := big.NewInt(10)

	return out.Div(out, base.Exp(base, big.NewInt(WeiDecimals), nil))
}

func WeiToDfm(wei *big.Int) *big.Int {
	dfm := new(big.Int).Set(wei)
	base := big.NewInt(10)
	dfm.Div(dfm, base.Exp(base, big.NewInt(WeiDecimals-DfmDecimals), nil))

	return dfm
}

func WeiToDfmCeil(wei *big.Int) *big.Int {
	dfm := new(big.Int).Set(wei)
	base := big.NewInt(10)
	mod := new(big.Int)
	dfm.DivMod(dfm, base.Exp(base, big.NewInt(WeiDecimals-DfmDecimals), nil), mod)

	if mod.BitLen() > 0 { // for zero big.Int BitLen() == 0
		dfm.Add(dfm, big.NewInt(1))
	}

	return dfm
}
