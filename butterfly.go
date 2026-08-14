package bigfft

import (
	"math/big"
	"math/bits"
)

// butterfly computes sum = x+t and diff = x-t modulo 2^(n*_W)+1.
// sum may alias x and diff may alias x; t must not alias either destination.
//
// The low n words are computed by butterflyWords in one pass. Keeping the two
// carry chains together saves one load of each operand compared with separate
// addVV/subVV calls. The high words and modular corrections are deliberately
// kept here: they are constant work and sharing the normalization code with
// fermat.Add/Sub makes the assembly contract small enough for asmdecl to check.
func butterfly(sum, diff, x, t fermat) {
	if len(sum) != len(x) || len(diff) != len(x) || len(t) != len(x) {
		panic("butterfly: length mismatch")
	}
	n := len(x) - 1
	xn, tn := x[n], t[n]
	carry, borrow := butterflyWords(sum[:n], diff[:n], x[:n], t[:n])

	sum[n] = xn + tn + carry
	sum.norm()

	// x-t borrowed borrow from the implicit high word and must also subtract
	// t[n]. Subtracting b*2^(n*_W) is adding b modulo 2^(n*_W)+1.
	borrow += tn
	diff[n] = xn
	if diff[0] <= ^big.Word(0)-borrow {
		diff[0] += borrow
	} else {
		addVW(diff, diff, borrow)
	}
	diff.norm()
}

// butterflyWordsGo is the architecture-independent reference implementation
// kept in every build for assembly differential tests.
func butterflyWordsGo(sum, diff, x, y []Word) (carry, borrow Word) {
	if len(sum) != len(x) || len(diff) != len(x) || len(y) != len(x) {
		panic("butterflyWordsGo: length mismatch")
	}
	for i := range x {
		s, c := bits.Add(uint(x[i]), uint(y[i]), uint(carry))
		d, b := bits.Sub(uint(x[i]), uint(y[i]), uint(borrow))
		sum[i], diff[i] = Word(s), Word(d)
		carry, borrow = Word(c), Word(b)
	}
	return carry, borrow
}
