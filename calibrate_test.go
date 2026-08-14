// Usage: go test -run=TestCalibrate -calibrate

package bigfft

import (
	"flag"
	"fmt"
	"math"
	"math/big"
	"testing"
	"time"
)

var (
	calibrate = flag.Bool("calibrate", false, "run calibration test")

	// calibrateMaxBits bounds the fftSizeThreshold sweep. The top of the
	// shipped table is 600 Mbit, and sweeping past its boundary means
	// gigabit operands; on most machines those points swap rather than
	// measure. Points above the limit are reported as skipped, not
	// extrapolated.
	calibrateMaxBits = flag.Int64("calibrate-max-bits", 3<<30,
		"skip fftSizeThreshold calibration points above this many bits per operand")

	// calibrateMaxDigits bounds the quadraticScanThreshold sweep the same way.
	calibrateMaxDigits = flag.Int("calibrate-max-digits", 10e6,
		"skip quadraticScanThreshold calibration points above this many digits")
)

// benchMin runs f as a benchmark three times and returns the fastest ns/op.
//
// Minima, not means: on a machine that occasionally migrates or throttles a
// benchmark, the mean absorbs every transient while the minimum tracks the
// quiet-machine cost. That matters here because a calibration sweep compares
// two configurations point by point rather than aggregating with benchstat.
func benchMin(f func(*testing.B)) time.Duration {
	const reps = 3
	best := time.Duration(math.MaxInt64)
	for range reps {
		if d := time.Duration(testing.Benchmark(f).NsPerOp()); d < best {
			best = d
		}
	}
	return best
}

// measureMul benchmarks math/big versus FFT for a given input size
// (in bits).
func measureMul(th int) (tBig, tFFT time.Duration) {
	bigLoad := func(b *testing.B) { benchmarkMulBig(b, th, th) }
	fftLoad := func(b *testing.B) { benchmarkMulFFT(b, th, th) }

	res1 := testing.Benchmark(bigLoad)
	res2 := testing.Benchmark(fftLoad)
	tBig = time.Duration(res1.NsPerOp())
	tFFT = time.Duration(res2.NsPerOp())
	return
}

func roundDur(d time.Duration) time.Duration {
	if d > 100*time.Millisecond {
		return d / time.Millisecond * time.Millisecond
	} else {
		return d / time.Microsecond * time.Microsecond
	}
}

func TestCalibrateThreshold(t *testing.T) {
	if !*calibrate {
		t.Log("not calibrating, use -calibrate to do so.")
		return
	}

	lower := int(1e3)   // math/big is faster at this size.
	upper := int(300e3) // FFT is faster at this size.

	var sizes [9]int
	var speedups [9]float64
	for i := 0; i < 3; i++ {
		for idx := 1; idx <= 9; idx++ {
			sz := ((10-idx)*lower + idx*upper) / 10
			big, fft := measureMul(sz)
			spd := float64(big) / float64(fft)
			sizes[idx-1] = sz
			speedups[idx-1] = spd
			fmt.Printf("speedup of FFT over math/big at size %d bits: %.2f (%s vs %s)\n",
				sz, spd, roundDur(big), roundDur(fft))
		}
		narrow := false
		for idx, s := range speedups {
			if s < .98 {
				lower = sizes[idx]
				narrow = true
			} else {
				break
			}
		}
		for idx := range speedups {
			if speedups[8-idx] > 1.02 {
				upper = sizes[8-idx]
				narrow = true
			} else {
				break
			}
		}
		if lower >= upper {
			panic("impossible")
		}
		if !narrow || (upper-lower) <= 10 {
			break
		}
	}
	fmt.Printf("sizes: %d\n", sizes)
	fmt.Printf("speedups: %.2f\n", speedups)
}

func measureFFTSize(w int, k uint) time.Duration {
	load := func(b *testing.B) {
		x := rndNat(w)
		y := rndNat(w)
		for i := 0; i < b.N; i++ {
			m := (w+w)>>k + 1
			xp := polyFromNat(x, k, m)
			yp := polyFromNat(y, k, m)
			rp := xp.Mul(&yp)
			_ = rp.Int()
		}
	}
	res := testing.Benchmark(load)
	return time.Duration(res.NsPerOp())
}

func TestCalibrateFFT(t *testing.T) {
	if !*calibrate {
		t.Log("not calibrating, use -calibrate to do so.")
		return
	}

	lows := [...]int{
		10, 10, 10, 10,
		20, 50, 100, 200, 500, // 8
		1000, 2000, 5000, 10000, // 12
		20000, 50000, 100e3, 200e3, // 16
	}
	his := [...]int{
		100, 100, 100, 200,
		500, 1000, 2000, 5000, 10000, // 8
		50e3, 100e3, 200e3, 800e3, // 12
		2e6, 5e6, 10e6, 20e6, // 16
	}
	for k := uint(3); k <= 16; k++ {
		// Measure the speedup between k and k+1
		low := lows[k] // FFT of size 1<<k known to be faster
		hi := his[k]   // FFT of size 2<<k known to be faster
		var sizes [9]int
		var speedups [9]float64
		for i := 0; i < 3; i++ {
			for idx := 1; idx <= 9; idx++ {
				sz := ((10-idx)*low + idx*hi) / 10
				t1, t2 := measureFFTSize(sz, k), measureFFTSize(sz, k+1)
				spd := float64(t1) / float64(t2)
				sizes[idx-1] = sz
				speedups[idx-1] = spd
				fmt.Printf("speedup of %d vs %d at size %d words: %.2f (%s vs %s)\n",
					k+1, k, sz, spd, roundDur(t1), roundDur(t2))
			}
			narrow := false
			for idx, s := range speedups {
				if s < .98 {
					low = sizes[idx]
					narrow = true
				} else {
					break
				}
			}
			for idx := range speedups {
				if speedups[8-idx] > 1.02 {
					hi = sizes[8-idx]
					narrow = true
				} else {
					break
				}
			}
			if low >= hi {
				panic("impossible")
			}
			if !narrow || (hi-low) <= 10 {
				break
			}
		}
		fmt.Printf("sizes: %d\n", sizes)
		fmt.Printf("speedups: %.2f\n", speedups)
	}
}

// ---------------------------------------------------------------------------
// Threshold recalibration (PLAN.md item 2).
//
// The three sweeps below deliberately do *not* bisect. TestCalibrateThreshold
// and TestCalibrateFFT above narrow towards a single number, which is exactly
// how the 2012 constants were produced — and when fftThreshold was re-measured
// that way in 2026 the "crossover" turned out to be an oscillation between 0.75
// and 1.11 with no clean transition. A flat grid publishes the whole curve, so
// a reader can see whether there is a crossing at all before anyone edits a
// constant. See BENCHMARKS.md.
// ---------------------------------------------------------------------------

// fftGridFactors straddle each incumbent fftSizeThreshold boundary.
var fftGridFactors = []float64{0.5, 0.7, 0.85, 1.0, 1.2, 1.5, 2.0}

// measureFFTSizeMin is measureFFTSize with transient noise filtered out.
func measureFFTSizeMin(w int, k uint) time.Duration {
	return benchMin(func(b *testing.B) {
		x := rndNat(w)
		y := rndNat(w)
		for i := 0; i < b.N; i++ {
			m := (w+w)>>k + 1
			xp := polyFromNat(x, k, m)
			yp := polyFromNat(y, k, m)
			rp := xp.Mul(&yp)
			_ = rp.Int()
		}
	})
}

// TestCalibrateFFTTable sweeps a flat grid around every boundary in
// fftSizeThreshold, comparing FFT length 1<<k against 1<<(k+1).
//
// A ratio above 1.0 means k+1 is the better choice at that size, so the
// incumbent boundary is right if the ratios cross 1.0 near factor 1.0 and stay
// above it. Anything else — no crossing, or ratios that wander back and forth
// across 1.0 — means the boundary is not measurable on this machine and must be
// left alone.
//
// Note the units: fftSize compares fftSizeThreshold against the *total* bit
// size of x*y, while measureFFTSize takes words per operand. The boundary in
// per-operand words is therefore threshold/(2*_W).
func TestCalibrateFFTTable(t *testing.T) {
	if !*calibrate {
		t.Log("not calibrating, use -calibrate to do so.")
		return
	}
	fmt.Printf("fftSizeThreshold calibration: ratio = t(k) / t(k+1); >1 means k+1 wins.\n")
	fmt.Printf("max %d bits/operand (-calibrate-max-bits)\n", *calibrateMaxBits)

	for k := uint(3); k < uint(len(fftSizeThreshold)); k++ {
		boundary := fftSizeThreshold[k]
		wb := int(boundary / int64(2*_W)) // words per operand at the boundary
		fmt.Printf("\n--- k=%d vs k=%d, incumbent boundary %d bits total = %d words/operand\n",
			k, k+1, boundary, wb)

		var lastRatio float64
		firstWin := -1 // smallest w at which k+1 wins
		lastLoss := -1 // largest w at which k still wins
		monotone := true
		measured := 0
		for _, f := range fftGridFactors {
			w := int(float64(wb) * f)
			if w < 1 {
				continue
			}
			if int64(w)*int64(_W) > *calibrateMaxBits {
				fmt.Printf("  f=%.2f  w=%-9d skipped (%d bits/operand exceeds limit)\n",
					f, w, int64(w)*int64(_W))
				continue
			}
			t1 := measureFFTSizeMin(w, k)
			t2 := measureFFTSizeMin(w, k+1)
			ratio := float64(t1) / float64(t2)
			fmt.Printf("  f=%.2f  w=%-9d k=%-2d %-12s k=%-2d %-12s ratio %.3f\n",
				f, w, k, roundDur(t1), k+1, roundDur(t2), ratio)

			if measured > 0 && ratio < lastRatio-0.02 {
				monotone = false
			}
			lastRatio = ratio
			measured++
			if ratio >= 1.0 {
				if firstWin < 0 {
					firstWin = w
				}
			} else {
				lastLoss = w
				firstWin = -1 // a later loss invalidates an earlier win
			}
		}

		switch {
		case measured == 0:
			fmt.Printf("  verdict: not measured\n")
		case firstWin < 0:
			fmt.Printf("  verdict: k=%d never wins in this range; boundary may be too low\n", k+1)
		case lastLoss < 0:
			fmt.Printf("  verdict: k=%d already wins at the low end; boundary may be too high\n", k+1)
		default:
			fmt.Printf("  verdict: crossover between w=%d and w=%d (%d..%d bits total); "+
				"incumbent %d bits; ratios %s\n",
				lastLoss, firstWin, 2*lastLoss*_W, 2*firstWin*_W, boundary,
				map[bool]string{true: "monotone", false: "NON-MONOTONE, do not fit"}[monotone])
		}
	}
}

// measureFermatMul times fermat.Mul at coefficient size n, forcing one branch
// or the other by moving fermatBasicMulThreshold around n.
//
// Two asymmetries worth knowing before reading the numbers:
//
//   - The big.Int branch has a short-product early return that basicMul does
//     not. benchRndFermat produces full-width random coefficients, whose
//     product is ~2n words, so that path essentially never fires here — but it
//     would bias the comparison for sparse inputs, which is why calibration
//     uses random full-width values only.
//   - Destination sizing is not a constraint either way: polValues.mul hands
//     fermat.Mul an 8*n-word buffer, and both branches need at most 2n+2. Any
//     cutoff is memory-safe.
func measureFermatMul(n int, forceBasic bool) time.Duration {
	x := benchRndFermat(n)
	y := benchRndFermat(n)
	buf := make(fermat, 8*n)

	old := fermatBasicMulThreshold
	defer func() { fermatBasicMulThreshold = old }()
	if forceBasic {
		fermatBasicMulThreshold = n + 1
	} else {
		fermatBasicMulThreshold = 0
	}

	return benchMin(func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			benchSinkFermat = buf.Mul(x, y)
		}
	})
}

// TestCalibrateFermatMul sweeps the basicMul / big.Int.Mul crossover in
// fermat.Mul.
//
// Rows are labelled reachable or informational. Only the reachable ones may
// drive the constant: n is not a free parameter on the production path, it is
// valueSize(k, m, 2) for the (k, m) that fftSize picked, so most of the sweep
// describes configurations that Mul can never produce. See
// TestFermatBasicMulThresholdReachable in threshold_test.go.
func TestCalibrateFermatMul(t *testing.T) {
	if !*calibrate {
		t.Log("not calibrating, use -calibrate to do so.")
		return
	}

	reachable := make(map[int]bool)
	for _, s := range reachableFermatSizes(64 << 20) {
		reachable[s.n] = true
	}

	// Every reachable n up to 96, plus a dense informational grid around the
	// incumbent cutoff.
	seen := make(map[int]bool)
	var ns []int
	for n := 8; n <= 96; n++ {
		if reachable[n] || (n >= 8 && n <= 72) {
			if !seen[n] {
				seen[n] = true
				ns = append(ns, n)
			}
		}
	}

	fmt.Printf("fermat.Mul basicMul/big.Int calibration "+
		"(incumbent fermatBasicMulThreshold = %d)\n", fermatBasicMulThreshold)
	fmt.Printf("ratio = t(bigint) / t(basicMul); >1 means basicMul wins.\n\n")

	for _, n := range ns {
		// The branch must not change the value.
		x := benchRndFermat(n)
		y := benchRndFermat(n)
		gotBasic := append(fermat(nil), make(fermat, 8*n).Mul(x, y)...)
		saved := fermatBasicMulThreshold
		fermatBasicMulThreshold = 0
		gotBig := append(fermat(nil), make(fermat, 8*n).Mul(x, y)...)
		fermatBasicMulThreshold = n + 1
		gotBasic = append(gotBasic[:0], make(fermat, 8*n).Mul(x, y)...)
		fermatBasicMulThreshold = saved
		if len(gotBasic) != len(gotBig) {
			t.Fatalf("n=%d: branch lengths differ: %d vs %d", n, len(gotBasic), len(gotBig))
		}
		for i := range gotBasic {
			if gotBasic[i] != gotBig[i] {
				t.Fatalf("n=%d: branches disagree at word %d", n, i)
			}
		}

		tBasic := measureFermatMul(n, true)
		tBig := measureFermatMul(n, false)
		label := "informational (unreachable via Mul)"
		if reachable[n] {
			label = "reachable"
		}
		// Printed unrounded: at these sizes fermat.Mul costs tens to hundreds
		// of nanoseconds, which roundDur would flatten to "0s".
		fmt.Printf("n=%-4d basicMul %-10v bigint %-10v ratio %.3f  %s\n",
			n, tBasic, tBig, float64(tBig)/float64(tBasic), label)
	}
}

// scanThresholdCandidates are the quadraticScanThreshold values to sweep.
//
// They are all multiples of 14 for continuity with the 2026 sweep, which ran
// when power(0) was built as (10^14)^(threshold/14) and no other value was
// legal. The balanced-split rewrite builds the base by binary exponentiation,
// so that constraint is gone and any value may be added here.
var scanThresholdCandidates = []int{
	280, 560, 840, 1120, 1232, 1400, 1680, 2240, 2800, 3360, 4480, 6720,
}

var scanDigitSizes = []int{1e3, 3e3, 1e4, 3e4, 1e5, 3e5, 1e6, 3e6, 1e7}

// TestCalibrateScan sweeps quadraticScanThreshold, and first answers the
// prior question of whether FromDecimalString beats big.Int.SetString at all.
//
// That question matters more than the constant. 1232 digits is about 4093 bits,
// roughly 64 words — far below fftThreshold (1800 words, ~34,700 digits). So
// below ~35k digits the whole recursion is math/big calling math/big, competing
// against Go's own subquadratic SetString, and the FFT never engages.
//
// One confound is reported rather than corrected: FromDecimalString builds a
// fresh scanner per call, so the whole power table is rebuilt inside the
// measured region. A larger threshold makes that setup dearer while reducing
// recursion depth, which entangles this constant with the "reuse scan
// temporaries" item in PLAN.md.
func TestCalibrateScan(t *testing.T) {
	if !*calibrate {
		t.Log("not calibrating, use -calibrate to do so.")
		return
	}

	old := quadraticScanThreshold
	defer func() { quadraticScanThreshold = old }()

	for _, size := range scanDigitSizes {
		if size > *calibrateMaxDigits {
			fmt.Printf("\n=== %d digits: skipped (-calibrate-max-digits)\n", size)
			continue
		}
		s := rndStr(size)
		want, ok := new(big.Int).SetString(s, 10)
		if !ok {
			t.Fatalf("cannot parse a %d-digit random string", size)
		}

		tBig := benchMin(func(b *testing.B) {
			var z big.Int
			for i := 0; i < b.N; i++ {
				z.SetString(s, 10)
			}
		})
		fmt.Printf("\n=== %d digits: big.Int.SetString %s\n", size, roundDur(tBig))

		for _, thr := range scanThresholdCandidates {
			quadraticScanThreshold = thr
			// A wrong threshold must fail as a wrong answer, not a fast one.
			if got := FromDecimalString(s); got.Cmp(want) != 0 {
				t.Fatalf("threshold %d: FromDecimalString is wrong at %d digits", thr, size)
			}
			tFast := benchMin(func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					_ = FromDecimalString(s)
				}
			})
			mark := ""
			if thr == old {
				mark = "  <- incumbent"
			}
			fmt.Printf("  threshold %-6d %-12s speedup vs SetString %.3f%s\n",
				thr, roundDur(tFast), float64(tBig)/float64(tFast), mark)
		}
		quadraticScanThreshold = old
	}
}

// scanSplitPerMille are candidate top-chunk lengths for the scan recursion,
// in per mille of half the input. 1000 is the shipped exact halving.
var scanSplitPerMille = []int{900, 930, 950, 960, 970, 975, 980, 985, 990, 995, 1000}

// TestCalibrateScanSplit sweeps where scan splits its input.
//
// The question it answers is whether the recursion should split exactly in half,
// or shade the split so that the children land higher inside a valueSize
// plateau. valueSize rounds a coefficient up to a multiple of 1<<(k-2) bits, so
// Mul cost is a step function of operand size with plateaus up to 25% wide: at
// k=12 every m from 56 to 63 costs the same ~33 ms, and m=64 jumps to ~43 ms for
// 3% more data. Shrinking the chunk can therefore drop a child a whole step.
//
// The 2026 answer was no: timings are flat from 970 to 1000 and degrade sharply
// below that (at 10M digits, 986 ms at 1000 against 1.85 s at 900), because the
// imbalance compounds down the recursion faster than the alignment pays. The
// sweep is kept so the verdict can be rechecked if fftSizeThreshold moves, since
// the plateau boundaries move with it.
func TestCalibrateScanSplit(t *testing.T) {
	if !*calibrate {
		t.Log("not calibrating, use -calibrate to do so.")
		return
	}
	for _, size := range scanDigitSizes {
		if size <= 4*quadraticScanThreshold || size > *calibrateMaxDigits {
			continue
		}
		s := rndStr(size)
		want, ok := new(big.Int).SetString(s, 10)
		if !ok {
			t.Fatalf("cannot parse a %d-digit random string", size)
		}
		fmt.Printf("\n=== %d digits\n", size)
		for _, pm := range scanSplitPerMille {
			top := (size / 2) * pm / 1000
			run := func() *big.Int {
				var sc scanner
				sc.initAt(size, top)
				z := new(big.Int)
				sc.scan(z, s, 0)
				return z
			}
			// A bad split must fail as a wrong answer, not a slow one.
			if got := run(); got.Cmp(want) != 0 {
				t.Fatalf("split %d/1000: scan is wrong at %d digits", pm, size)
			}
			d := benchMin(func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					run()
				}
			})
			mark := ""
			if pm == 1000 {
				mark = "  <- shipped (exact half)"
			}
			fmt.Printf("  split %4d/1000  top=%-9d %s%s\n", pm, top, roundDur(d), mark)
		}
	}
}
