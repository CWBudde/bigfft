package bigfft

import (
	"fmt"
	"strconv"
	"testing"
)

// Always-on guards for the tuning constants, plus the machinery that derives
// which coefficient sizes the production Mul path can actually reach.
//
// These are deliberately not gated behind -calibrate: they are invariants, not
// measurements. The calibration sweeps themselves live in calibrate_test.go.

// TestFFTSizeThresholdMonotone checks that fftSizeThreshold is strictly
// increasing after its leading zeros.
//
// fftSize scans for the *first* entry greater than the bit size, so a
// non-monotone table silently selects a smaller k than intended, with no other
// test failing. The leading zeros are load-bearing in the opposite direction:
// a zero entry is never > bits, which is what makes k = 0, 1, 2 unselectable.
func TestFFTSizeThresholdMonotone(t *testing.T) {
	const leadingZeros = 3
	for i := range leadingZeros {
		if fftSizeThreshold[i] != 0 {
			t.Errorf("fftSizeThreshold[%d] = %d, want 0: entries below k=%d must be "+
				"unselectable", i, fftSizeThreshold[i], leadingZeros)
		}
	}
	for i := leadingZeros; i < len(fftSizeThreshold); i++ {
		if fftSizeThreshold[i] <= 0 {
			t.Errorf("fftSizeThreshold[%d] = %d, want > 0", i, fftSizeThreshold[i])
			continue
		}
		if i > leadingZeros && fftSizeThreshold[i] <= fftSizeThreshold[i-1] {
			t.Errorf("fftSizeThreshold not increasing: [%d] = %d <= [%d] = %d",
				i, fftSizeThreshold[i], i-1, fftSizeThreshold[i-1])
		}
	}
}

// fermatSize is one (k, m, n) configuration that the public Mul path reaches,
// together with the range of per-operand bit sizes that reach it.
type fermatSize struct {
	k                uint
	m, n             int
	minBits, maxBits int64 // per operand
}

func (f fermatSize) String() string {
	return fmt.Sprintf("k=%2d m=%4d n=%4d  reached for %s..%s bits/operand",
		f.k, f.m, f.n, siBits(f.minBits), siBits(f.maxBits))
}

func siBits(b int64) string {
	switch {
	case b >= 1e9:
		return fmt.Sprintf("%.2fG", float64(b)/1e9)
	case b >= 1e6:
		return fmt.Sprintf("%.2fM", float64(b)/1e6)
	case b >= 1e3:
		return fmt.Sprintf("%.1fk", float64(b)/1e3)
	default:
		return strconv.FormatInt(b, 10)
	}
}

// reachableFermatSizes returns every (k, m, n) that balanced operands can drive
// fermat.Mul to on the current platform, for per-operand sizes from just above
// fftThreshold up to 1 Gbit.
//
// This is the mechanical version of a fact that is otherwise easy to get wrong:
// n is not a free parameter. The planner starts from fftSizeThreshold and may
// choose a measured adjacent k; that k and the word count give m, and
// valueSize(k, m, 2) gives n. So the set of n values that fermat.Mul ever sees
// re-derives itself if either the threshold table or planner policy changes.
//
// Only balanced operands are swept. Mul dispatches to math/big unless *both*
// operands exceed fftThreshold words (fft.go), so unbalanced pairs reach a
// subset of the same configurations at a larger total size.
func reachableFermatSizes(maxBits int64) []fermatSize {
	minBits := int64(fftThreshold+1) * int64(_W)
	byConfig := make(map[[3]int]*fermatSize)
	var order [][3]int

	// selectFFTPlan only reads len(x) and len(y), so one buffer sliced to length
	// serves the whole sweep. Allocating a fresh nat per point would mean
	// hundreds of multi-megabyte allocations for no benefit.
	buf := make(nat, maxBits/int64(_W)+1)

	// A 1% geometric step is fine enough that no k bracket is missed: the
	// narrowest bracket in the shipped table spans a factor of two.
	for bits := minBits; bits <= maxBits; bits = max(bits*101/100, bits+1) {
		w := int(bits / int64(_W))
		if w <= fftThreshold {
			continue
		}
		plan := selectFFTPlan(buf[:w], buf[:w])
		key := [3]int{int(plan.k), plan.m, plan.n}
		if f, ok := byConfig[key]; ok {
			f.maxBits = bits
			continue
		}
		byConfig[key] = &fermatSize{
			k: plan.k, m: plan.m, n: plan.n, minBits: bits, maxBits: bits,
		}
		order = append(order, key)
	}

	out := make([]fermatSize, 0, len(order))
	for _, key := range order {
		out = append(out, *byConfig[key])
	}
	return out
}

// TestFermatBasicMulThresholdReachable records which coefficient sizes the
// production path actually reaches, and checks that fermatBasicMulThreshold
// still separates two non-empty sets of them.
//
// The point is rule 5 of PLAN.md's measurement discipline: before tuning a
// branch, count how often it is taken. If no reachable n falls below the
// threshold, the basicMul branch is dead code on this platform and tuning the
// constant is pointless — the test says so rather than leaving it to be
// rediscovered.
func TestFermatBasicMulThresholdReachable(t *testing.T) {
	// 64 Mbit per operand is far past the point where n stops being small;
	// the calibration harness sweeps further.
	sizes := reachableFermatSizes(64 << 20)
	if len(sizes) == 0 {
		t.Fatal("no reachable fermat sizes: reachableFermatSizes is broken")
	}

	var basic, bigint []fermatSize
	// One line per k rather than per configuration: there are hundreds of the
	// latter, and the k brackets are what the table controls.
	prevK := ^uint(0)
	for _, s := range sizes {
		if s.k != prevK {
			t.Logf("k=%2d starts at %s bits/operand: m=%d n=%d",
				s.k, siBits(s.minBits), s.m, s.n)
			prevK = s.k
		}
		if s.n < fermatBasicMulThreshold {
			basic = append(basic, s)
		} else {
			bigint = append(bigint, s)
		}
	}

	t.Logf("fermatBasicMulThreshold = %d: %d of %d reachable configurations take "+
		"the basicMul branch", fermatBasicMulThreshold, len(basic), len(sizes))
	for _, s := range basic {
		t.Logf("  basicMul: %s", s)
	}

	if len(bigint) == 0 {
		t.Errorf("fermatBasicMulThreshold = %d puts every reachable configuration on "+
			"the basicMul side; the big.Int branch is dead code", fermatBasicMulThreshold)
	}
	if len(basic) == 0 {
		t.Logf("NOTE: no reachable configuration takes the basicMul branch on this "+
			"platform (_W = %d). The branch is exercised only by direct calls to "+
			"fermat.Mul, as in BenchmarkFermatMul; calibrating "+
			"fermatBasicMulThreshold against the public Mul path is therefore moot "+
			"here.", _W)
	}
}

// TestParallelDispatchOverlap records where the two dispatch gates sit relative
// to each other, mechanically, so the relationship cannot drift unnoticed.
//
// Mul sends operands to the FFT above fftThreshold words, and fftmul runs the
// transform in parallel once one transform array reaches parallelWordThreshold
// words. If parallelism started paying at a smaller operand size than
// fftThreshold admits, there would be a band where a parallel FFT beats
// Karatsuba but Mul never selects it. Capturing that band with a second, lower
// threshold for the parallel path was a to-do item until it was measured: the
// band is eight words wide, and a parallel FFT does not overtake math/big until
// 115 kbit, which is fftThreshold itself. See PLAN.md § Tried and rejected.
//
// This test exists so that a future change to fftSizeThreshold or fftThreshold
// cannot silently reopen that band without saying so.
func TestParallelDispatchOverlap(t *testing.T) {
	// One allocation, re-sliced, so the scan can step by a single word: the
	// answer is a specific boundary and a geometric scan would step over it.
	const maxWords = 1 << 20
	buf := make(nat, maxWords)
	first := 0
	for w := 1; w <= maxWords; w++ {
		x := buf[:w]
		plan := selectFFTPlan(x, x)
		if arr := (plan.n + 1) << plan.k; arr >= parallelWordThreshold {
			first = w
			break
		}
	}
	if first == 0 {
		t.Fatalf("parallelWordThreshold = %d is never reached below 2^20 words per "+
			"operand; the parallel path is dead through the public Mul",
			parallelWordThreshold)
	}

	t.Logf("fftThreshold = %d words/operand (%s bits); parallelism engages at %d "+
		"words/operand (%s bits)", fftThreshold, siBits(int64(fftThreshold)*int64(_W)),
		first, siBits(int64(first)*int64(_W)))

	switch {
	case first > fftThreshold:
		t.Logf("NOTE: %d words of the FFT range run serially. Sizes in "+
			"[%d, %d) words take the FFT without parallelism.",
			first-fftThreshold, fftThreshold, first)
	case first < fftThreshold:
		t.Logf("NOTE: parallelism now engages %d words BELOW fftThreshold. "+
			"Operands in [%d, %d) words would run a parallel FFT, but Mul sends "+
			"them to Karatsuba instead. PLAN.md item 1 (a separate threshold for "+
			"the parallel path) has room again; re-measure the crossover.",
			fftThreshold-first, first, fftThreshold)
	default:
		t.Logf("the two gates coincide exactly; PLAN.md item 1 has no room")
	}
}
