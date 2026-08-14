package bigfft

import (
	"fmt"
	"math/big"
	"math/rand"
	"testing"
)

// Micro-benchmarks for the internal primitives of the Schönhage-Strassen
// multiplier: fermat.Shift/ShiftHalf/Add/Sub/Mul, poly.Transform,
// polValues.InvTransform and polValues.Mul.
//
// Sizes are not invented: they are derived at run time from the same
// production planner. For an input of B bits per operand, selectFFTPlan yields
// the FFT length k, chunk size m, and coefficient length n used by poly.Mul.
// So every (n, k) pair below is the pair the FFT path really uses for that
// input size.
//
// The representative inputs are ~100kb, ~1Mb and ~5Mb per operand, which on
// a 64-bit platform come out as:
//
//	100kb -> k=8,  m=13, n=27   (basicMul branch; see the caveat below)
//	1Mb   -> k=10, m=31, n=64   (big.Int branch)
//	5Mb   -> k=12, m=39, n=80   (big.Int branch)
//
// Caveat on the 100kb row: 100 kbit is 1562 words on 64-bit, below
// fftThreshold (1800), so the public Mul dispatches that size to math/big and
// never builds this configuration. It is reached only by mulFFT / poly.Mul,
// as these benchmarks do. TestFermatBasicMulThresholdReachable in
// threshold_test.go enumerates what Mul itself can reach.
//
// On a 32-bit platform the numbers differ, which is why they are computed
// rather than hard-coded. Subtest names are "n=<n>/k=<k>" so benchstat
// output stays greppable and stable across commits.

// Package-level sinks, so that benchmarked calls cannot be optimised away.
var (
	benchSinkFermat fermat
	benchSinkPoly   poly
	benchSinkValues polValues
)

// benchRnd is a dedicated, fixed-seed source so that benchmark inputs are
// identical from run to run and independent of test execution order.
var benchRnd = rand.New(rand.NewSource(0x1bf7c0ffee13))

// benchRndNat returns n random words, in the style of rndNat in fft_test.go.
func benchRndNat(n int) nat {
	x := make(nat, n)
	for i := range x {
		x[i] = big.Word(benchRnd.Int63()<<1 + benchRnd.Int63n(2))
	}
	return x
}

// benchRndFermat returns a valid, normalised element of Z/(b^n+1)Z:
// n random words plus a zero high word.
func benchRndFermat(n int) fermat {
	f := make(fermat, n+1)
	copy(f, benchRndNat(n))
	return f
}

// benchSize describes one representative FFT configuration.
type benchSize struct {
	bits int  // bits per operand of the original multiplication
	k    uint // FFT length is 1<<k
	m    int  // words per polynomial chunk
	n    int  // words per fermat coefficient (modulus 2^(n*_W)+1)
}

func (s benchSize) name() string { return fmt.Sprintf("n=%d/k=%d", s.n, s.k) }

// benchSizes derives the (n, k) table exactly as fftmul does.
var benchSizes = func() []benchSize {
	inputs := []int{100e3, 1e6, 5e6}
	sizes := make([]benchSize, 0, len(inputs))
	for _, bits := range inputs {
		w := bits / _W
		plan := selectFFTPlan(make(nat, w), make(nat, w))
		sizes = append(sizes, benchSize{bits: bits, k: plan.k, m: plan.m, n: plan.n})
	}
	return sizes
}()

// benchShift returns a shift amount that is not a whole number of words, so
// that both the kw (word) and the kb (bit) paths of fermat.Shift are taken.
// It stays below n*_W, i.e. it exercises the non-negating branch.
func benchShift(n int) int { return n*_W/2 + 13 }

func BenchmarkFermatShift(b *testing.B) {
	for _, s := range benchSizes {
		b.Run(s.name(), func(b *testing.B) {
			x := benchRndFermat(s.n)
			z := make(fermat, s.n+1)
			k := benchShift(s.n)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				z.Shift(x, k)
			}
			benchSinkFermat = z
		})
	}
}

// BenchmarkFermatShiftHalf_Even measures the cheap half of ShiftHalf: an even
// k is a single fermat.Shift.
func BenchmarkFermatShiftHalf_Even(b *testing.B) {
	for _, s := range benchSizes {
		b.Run(s.name(), func(b *testing.B) {
			x := benchRndFermat(s.n)
			z := make(fermat, s.n+1)
			tmp := make(fermat, s.n+1)
			k := 2 * benchShift(s.n)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				z.ShiftHalf(x, k, tmp)
			}
			benchSinkFermat = z
		})
	}
}

// BenchmarkFermatShiftHalf_Odd measures the expensive half: an odd k is a
// multiplication by sqrt(2), i.e. two fermat.Shift calls plus a fermat.Sub.
// This is the hottest single path in the library (~22% of runtime) and the
// target of the upcoming optimisation, so it is kept strictly separate from
// the even case.
func BenchmarkFermatShiftHalf_Odd(b *testing.B) {
	for _, s := range benchSizes {
		b.Run(s.name(), func(b *testing.B) {
			x := benchRndFermat(s.n)
			z := make(fermat, s.n+1)
			tmp := make(fermat, s.n+1)
			k := 2*benchShift(s.n) + 1
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				z.ShiftHalf(x, k, tmp)
			}
			benchSinkFermat = z
		})
	}
}

func BenchmarkFermatAdd(b *testing.B) {
	for _, s := range benchSizes {
		b.Run(s.name(), func(b *testing.B) {
			x := benchRndFermat(s.n)
			y := benchRndFermat(s.n)
			z := make(fermat, s.n+1)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				benchSinkFermat = z.Add(x, y)
			}
		})
	}
}

func BenchmarkFermatSub(b *testing.B) {
	for _, s := range benchSizes {
		b.Run(s.name(), func(b *testing.B) {
			x := benchRndFermat(s.n)
			y := benchRndFermat(s.n)
			z := make(fermat, s.n+1)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				benchSinkFermat = z.Sub(x, y)
			}
		})
	}
}

// BenchmarkFermatMul covers both sides of the fermatBasicMulThreshold branch in fermat.Mul:
// basicMul for small coefficients, big.Int.Mul (Karatsuba) for large ones.
// The derived sizes above already contain at least one of each on 64-bit;
// n=16 is added so the basicMul side is covered on every platform.
func BenchmarkFermatMul(b *testing.B) {
	type mulCase struct {
		name string
		n    int
	}
	cases := []mulCase{{name: "n=16/k=0", n: 16}}
	for _, s := range benchSizes {
		cases = append(cases, mulCase{name: s.name(), n: s.n})
	}
	for _, c := range cases {
		branch := "bigint"
		if c.n < fermatBasicMulThreshold {
			branch = "basicMul"
		}
		b.Run(c.name+"/"+branch, func(b *testing.B) {
			x := benchRndFermat(c.n)
			y := benchRndFermat(c.n)
			// polValues.Mul uses a scratch buffer of 8*n words.
			buf := make(fermat, 8*c.n)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				benchSinkFermat = buf.Mul(x, y)
			}
		})
	}
}

// benchPolys builds a pair of polynomials the way fftmul does, and returns
// the coefficient size n that poly.Mul would use for them.
func benchPolys(s benchSize) (p, q poly, n int) {
	w := s.bits / _W
	x := benchRndNat(w)
	y := benchRndNat(w)
	plan := selectFFTPlan(x, y)
	return polyFromNat(x, plan.k, plan.m), polyFromNat(y, plan.k, plan.m), plan.n
}

func BenchmarkTransform(b *testing.B) {
	for _, s := range benchSizes {
		b.Run(s.name(), func(b *testing.B) {
			p, _, n := benchPolys(s)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				benchSinkValues = p.Transform(n)
			}
		})
	}
}

func BenchmarkInvTransform(b *testing.B) {
	for _, s := range benchSizes {
		b.Run(s.name(), func(b *testing.B) {
			p, _, n := benchPolys(s)
			pv := p.Transform(n)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				benchSinkPoly = pv.InvTransform()
			}
		})
	}
}

// BenchmarkPolValuesMul measures the pointwise product loop, which is about
// 35% of total runtime and the target of the upcoming parallelisation.
func BenchmarkPolValuesMul(b *testing.B) {
	for _, s := range benchSizes {
		b.Run(s.name(), func(b *testing.B) {
			p, q, n := benchPolys(s)
			pv := p.Transform(n)
			qv := q.Transform(n)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				benchSinkValues = pv.Mul(&qv)
			}
		})
	}
}
