package bigfft

import (
	"math/big"
)

// FromDecimalString converts the base 10 string
// representation of a natural (non-negative) number
// into a *big.Int.
// Its asymptotic complexity is less than quadratic.
func FromDecimalString(s string) *big.Int {
	z := new(big.Int)
	if len(s) <= quadraticScanThreshold {
		// Below the threshold there is nothing to split, and building a
		// scanner to discover that costs more than the scan itself.
		z.SetString(s, 10)
		return z
	}
	var sc scanner
	sc.init(len(s))
	sc.scan(z, s, 0)
	return z
}

// scanner holds the table of powers of ten used to reassemble the chunks a
// decimal string is split into.
//
// The split is balanced: at every level the low chunk is half the digits of
// that level, so both halves of the recursive Mul are the same size. That
// requires a power of ten per level rather than per binary order of magnitude,
// which is why the table is built for one specific input length in init.
type scanner struct {
	// digits[d] is the number of decimal digits split off the right at
	// recursion depth d, and pow[d] is 10^digits[d]. Both are strictly
	// decreasing; digits[len(digits)-1] <= quadraticScanThreshold.
	digits []int
	pow    []*big.Int

	// tmp[d] holds the scaled left half while the right half at depth d is
	// scanned into its destination. Calls at the same depth are sequential,
	// so one reusable multiplication destination per depth is sufficient.
	tmp []big.Int
}

// init builds the power table for an input of size decimal digits. It is O(size)
// digits of storage and costs one multiplication per level, dominated by the
// topmost squaring of a size/2-digit number.
func (s *scanner) init(size int) {
	s.initAt(size, size/2)
}

// initAt is init with an explicit top chunk length, so the calibration harness
// can sweep it. Every lower level follows from it by halving.
func (s *scanner) initAt(size, top int) {
	// The chain of lengths is top, top/2, top/4, ... Halving with floor is
	// what makes the table cheap to build: digits[d] = 2*digits[d+1] + (0 or 1),
	// so each entry is the square of the next, times ten at most once.
	if size > quadraticScanThreshold {
		s.digits = append(s.digits, top)
		for n := top; n > quadraticScanThreshold; n /= 2 {
			s.digits = append(s.digits, n/2)
		}
	}
	if len(s.digits) == 0 {
		return
	}
	s.pow = make([]*big.Int, len(s.digits))
	s.tmp = make([]big.Int, len(s.digits))
	last := len(s.digits) - 1
	s.pow[last] = new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(s.digits[last])), nil)
	for d := last - 1; d >= 0; d-- {
		z := new(big.Int).Mul(s.pow[d+1], s.pow[d+1])
		if s.digits[d]&1 != 0 {
			z.Mul(z, big.NewInt(10))
		}
		s.pow[d] = z
	}
}

func (s *scanner) scan(z *big.Int, str string, depth int) {
	// Floor halving lets the high chunk carry up to one digit more than the
	// level was sized for, so by the deepest level the string can exceed the
	// threshold by as much as the depth — a handful of digits. SetString
	// absorbs that; recursing past the table would not.
	if len(str) <= quadraticScanThreshold || depth >= len(s.digits) {
		z.SetString(str, 10)
		return
	}
	sz := s.digits[depth]
	if len(str) <= sz {
		// Nothing to split off at this level; drop to the next one.
		s.scan(z, str, depth+1)
		return
	}
	// Scan the left half.
	s.scan(z, str[:len(str)-sz], depth+1)
	left := &s.tmp[depth]
	mulInto(left, z, s.pow[depth])
	// Scan the right half.
	s.scan(z, str[len(str)-sz:], depth+1)
	z.Add(z, left)
}

// quadraticScanThreshold is the number of digits
// below which big.Int.SetString is more efficient
// than subquadratic algorithms.
// 1232 digits fit in 4096 bits.
//
// That 4096 used to matter for more than tidiness: when chunks were
// threshold<<k digits they were all just under 2^(12+k) bits, which happened to
// land them near the top of a valueSize plateau. Balanced splitting gives that
// up, which costs a few percent above 5M digits and wins more below. See
// BENCHMARKS.md § Balanced splitting in decimal scanning.
//
// It is a var only so that the calibration harness can sweep it; production code
// never assigns to it. Any value is legal: the power table is built by binary
// exponentiation, so unlike the earlier (10^14)^(threshold/14) construction this
// no longer requires a multiple of 14.
var quadraticScanThreshold = 1232
