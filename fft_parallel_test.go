package bigfft

import (
	"fmt"
	"math/big"
	"math/rand"
	"runtime"
	"testing"
)

// parSizes are operand sizes chosen to land on a particular FFT size.
// fftSize picks k from the total bit length of the product, so the sizes are
// given in bits and converted to words at run time: expressing them directly
// in words would select a different k on 32-bit platforms, where the same
// word count is half the bits.
//
//	1.28 Mbit per operand -> 2.44 Mbit product -> k=10
//	2.56 Mbit per operand -> 4.88 Mbit product -> k=11
//	4.48 Mbit per operand -> 8.54 Mbit product -> k=12
//
// The table is checked against fftSize in TestParallelCoversLargeK rather
// than trusted, so a change to fftSizeThreshold cannot silently turn these
// tests into small-k tests.
type parSize struct {
	bits int
	k    uint
}

// words returns the operand size in machine words on the current platform.
func (ps parSize) words() int { return ps.bits / _W }

var parSizes = []parSize{
	{20000 * 64, 10},
	{40000 * 64, 11},
	{70000 * 64, 12},
}

// withParallelism sets the worker cap for the duration of the test and
// restores the previous setting afterwards.
func withParallelism(t *testing.T, n int) {
	t.Helper()
	old := int(maxParallelism.Load())
	t.Cleanup(func() { maxParallelism.Store(int64(old)) })
	SetMaxParallelism(n)
}

// TestParallelCoversLargeK verifies the premise of the tests below: the sizes
// in parSizes really do select k=10..12, and at those sizes the library
// really does choose to run in parallel. Without this, a threshold change
// could make every other test in this file exercise the serial path only.
func TestParallelCoversLargeK(t *testing.T) {
	withParallelism(t, 0)
	if MaxParallelism() < 2 {
		t.Skipf("GOMAXPROCS=%d, nothing to parallelise", runtime.GOMAXPROCS(0))
	}
	for _, ps := range parSizes {
		x, y := make(nat, ps.words()), make(nat, ps.words())
		k, m := fftSize(x, y)
		if k != ps.k {
			t.Errorf("%d words: fftSize gave k=%d, want %d", ps.words(), k, ps.k)
		}
		n := valueSize(k, m, 2)
		if w := parallelWorkers(k, n); w < 2 {
			t.Errorf("%d words (k=%d, n=%d): parallelWorkers=%d, the parallel "+
				"path is not being exercised", ps.words(), k, n, w)
		}
	}
}

// TestParallelBitIdentical is the correctness bar for the whole feature: for
// many random operand pairs at k=10..12, the product must be bit-for-bit the
// same serially and in parallel, and must agree with math/big.
func TestParallelBitIdentical(t *testing.T) {
	iters := 4
	sizes := parSizes
	if testing.Short() {
		iters = 1
		sizes = parSizes[:1]
	}
	rng := rand.New(rand.NewSource(0xfeed5))
	for _, ps := range sizes {
		for it := 0; it < iters; it++ {
			xb := arenaRandNat(rng, ps.words())
			yb := arenaRandNat(rng, ps.words()+it) // vary the second operand
			x, y := natToInt(xb), natToInt(yb)

			withParallelism(t, 1)
			serial := mulFFT(x, y)
			serialMul := Mul(x, y)

			withParallelism(t, 0)
			par := mulFFT(x, y)
			parMul := Mul(x, y)

			want := new(big.Int).Mul(x, y)
			for _, c := range []struct {
				name string
				got  *big.Int
			}{
				{"serial mulFFT", serial},
				{"serial Mul", serialMul},
				{"parallel mulFFT", par},
				{"parallel Mul", parMul},
			} {
				if c.got.Cmp(want) != 0 {
					t.Fatalf("k=%d iter %d: %s disagrees with math/big", ps.k, it, c.name)
				}
			}
			if !bitsEqual(serial.Bits(), par.Bits()) {
				t.Fatalf("k=%d iter %d: parallel result is not bit-identical to serial", ps.k, it)
			}
			// Operands must survive untouched.
			if natToInt(xb).Cmp(x) != 0 || natToInt(yb).Cmp(y) != 0 {
				t.Fatalf("k=%d iter %d: operands clobbered", ps.k, it)
			}
		}
	}
}

// TestParallelBitIdenticalAcrossWorkerCounts checks that the answer does not
// depend on how the work happened to be sharded: a worker count that does not
// divide the number of coefficients, and one that exceeds GOMAXPROCS, must
// give the same words as the serial run.
func TestParallelBitIdenticalAcrossWorkerCounts(t *testing.T) {
	ps := parSizes[0]
	rng := rand.New(rand.NewSource(0xc0ffee))
	xb, yb := arenaRandNat(rng, ps.words()), arenaRandNat(rng, ps.words())
	x, y := natToInt(xb), natToInt(yb)

	withParallelism(t, 1)
	want := mulFFT(x, y)

	counts := []int{2, 3, 5, 7, 8, 12, 17}
	if testing.Short() {
		counts = []int{3, 7}
	}
	for _, w := range counts {
		withParallelism(t, w)
		if got := mulFFT(x, y); !bitsEqual(got.Bits(), want.Bits()) {
			t.Errorf("worker count %d: result differs from serial", w)
		}
	}
}

// TestParallelSaturatedOperands repeats the bit-identity check on all-ones
// operands, the worst case for carry propagation in the modular arithmetic.
func TestParallelSaturatedOperands(t *testing.T) {
	sizes := parSizes
	if testing.Short() {
		sizes = parSizes[:1]
	}
	for _, ps := range sizes {
		xb := make(nat, ps.words())
		for i := range xb {
			xb[i] = ^big.Word(0)
		}
		x := natToInt(xb)

		withParallelism(t, 1)
		serial := mulFFT(x, x)
		withParallelism(t, 0)
		par := mulFFT(x, x)

		if want := new(big.Int).Mul(x, x); par.Cmp(want) != 0 {
			t.Fatalf("k=%d: parallel saturated product is wrong", ps.k)
		}
		if !bitsEqual(serial.Bits(), par.Bits()) {
			t.Fatalf("k=%d: parallel saturated product differs from serial", ps.k)
		}
	}
}

// TestParallelConcurrentMuls runs several multiplications at once, each of
// them internally parallel. Nothing in the arena may be shared between calls;
// under -race this is what would catch a stray package-level buffer.
func TestParallelConcurrentMuls(t *testing.T) {
	withParallelism(t, 0)
	ps := parSizes[0]
	rng := rand.New(rand.NewSource(0x1234abcd))
	const n = 4
	xs := make([]*big.Int, n)
	ys := make([]*big.Int, n)
	want := make([]*big.Int, n)
	for i := range xs {
		xs[i] = natToInt(arenaRandNat(rng, ps.words()))
		ys[i] = natToInt(arenaRandNat(rng, ps.words()+i))
		want[i] = new(big.Int).Mul(xs[i], ys[i])
	}
	got := make([]*big.Int, n)
	done := make(chan int, n)
	for i := range xs {
		go func(i int) {
			got[i] = mulFFT(xs[i], ys[i])
			done <- i
		}(i)
	}
	for range xs {
		<-done
	}
	for i := range got {
		if got[i].Cmp(want[i]) != 0 {
			t.Errorf("concurrent multiplication %d is wrong", i)
		}
	}
}

// TestSetMaxParallelism covers the knob itself.
func TestSetMaxParallelism(t *testing.T) {
	withParallelism(t, 0)
	if got, want := MaxParallelism(), runtime.GOMAXPROCS(0); got != want {
		t.Errorf("default MaxParallelism = %d, want GOMAXPROCS = %d", got, want)
	}
	for _, n := range []int{-3, -1, 0} {
		SetMaxParallelism(n)
		if got, want := MaxParallelism(), runtime.GOMAXPROCS(0); got != want {
			t.Errorf("SetMaxParallelism(%d): MaxParallelism = %d, want %d", n, got, want)
		}
	}
	for _, n := range []int{1, 2, 5, 64} {
		SetMaxParallelism(n)
		if got := MaxParallelism(); got != n {
			t.Errorf("SetMaxParallelism(%d): MaxParallelism = %d", n, got)
		}
	}
	// Serial mode must not merely cap the workers, it must switch the
	// library to the single-buffer layout.
	SetMaxParallelism(1)
	s := newFFTScratch(12, 128)
	if s.w != 1 || len(s.fts) != 1 || len(s.mbufs) != 1 {
		t.Errorf("SetMaxParallelism(1): arena has w=%d, %d temp pairs, %d bufs",
			s.w, len(s.fts), len(s.mbufs))
	}
}

// TestParallelWorkersThreshold checks the size threshold: small transforms
// stay serial whatever the knob says, large ones do not.
func TestParallelWorkersThreshold(t *testing.T) {
	withParallelism(t, 8)
	// Well below the threshold: (8+1)<<6 = 576 words.
	if w := parallelWorkers(6, 8); w != 1 {
		t.Errorf("parallelWorkers(6, 8) = %d, want 1 (below threshold)", w)
	}
	// Well above: (128+1)<<10 = 132096 words.
	if w := parallelWorkers(10, 128); w != 8 {
		t.Errorf("parallelWorkers(10, 128) = %d, want 8", w)
	}
	// Never more workers than coefficients.
	if w := parallelWorkers(2, 1<<16); w > 4 {
		t.Errorf("parallelWorkers(2, 1<<16) = %d, want at most 4 coefficients' worth", w)
	}
	withParallelism(t, 1)
	if w := parallelWorkers(10, 128); w != 1 {
		t.Errorf("with the knob at 1, parallelWorkers = %d, want 1", w)
	}
}

// TestParallelArenaCapacitiesBounded extends TestArenaCapacitiesBounded to
// the per-worker regions: they must be capacity bounded too, and no two
// workers' buffers may overlap, or the pointwise products would race and
// math/big would stop reusing the destination buffers.
func TestParallelArenaCapacitiesBounded(t *testing.T) {
	withParallelism(t, 6)
	s := newFFTScratch(10, 128)
	if s.w != 6 {
		t.Fatalf("arena built for %d workers, want 6", s.w)
	}
	var regions []arenaRegion
	for i := range s.fts {
		regions = append(regions,
			arenaRegion{fmt.Sprintf("fts[%d].tmp", i), s.fts[i].tmp},
			arenaRegion{fmt.Sprintf("fts[%d].tmp2", i), s.fts[i].tmp2})
	}
	for i := range s.mbufs {
		regions = append(regions, arenaRegion{fmt.Sprintf("mbufs[%d]", i), s.mbufs[i]})
	}
	regions = append(regions,
		arenaRegion{"a", s.a}, arenaRegion{"b", s.b},
		arenaRegion{"c", s.c}, arenaRegion{"u", s.u})

	for _, r := range regions {
		if cap(r.w) != len(r.w) {
			t.Errorf("%s: cap %d != len %d, arena sub-slice is not bounded",
				r.name, cap(r.w), len(r.w))
		}
	}
	for i := range regions {
		for j := i + 1; j < len(regions); j++ {
			if overlaps(regions[i].w, regions[j].w) {
				t.Errorf("worker regions %s and %s overlap", regions[i].name, regions[j].name)
			}
		}
	}
	// The property math/big cares about, for every worker's buffer.
	for w := range s.mbufs {
		for i := range s.hP {
			if capEnd(s.mbufs[w]) == capEnd(s.hP[i]) || capEnd(s.mbufs[w]) == capEnd(s.hQ[i]) {
				t.Fatalf("mbufs[%d] shares a capacity end with value %d", w, i)
			}
		}
	}
	// ft and mbuf must still name worker 0's storage.
	if !sameSlice(s.ft.tmp, s.fts[0].tmp) || !sameSlice(s.mbuf, s.mbufs[0]) {
		t.Error("ft/mbuf no longer alias worker 0's regions")
	}
}

func sameSlice(a, b []big.Word) bool {
	return len(a) == len(b) && (len(a) == 0 || &a[0] == &b[0])
}

func bitsEqual(a, b []big.Word) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// benchmarkParallelSizes are operand bit sizes straddling the region where
// parallelism starts to pay off.
var benchmarkParallelSizes = []int{5e4, 1e5, 1.5e5, 2e5, 5e5, 1e6, 2e6, 5e6, 10e6}

// BenchmarkMulFFTParallelSweep measures the serial and the parallel path at
// each size, in one binary, so the two are directly comparable. It is how
// parallelWordThreshold was chosen: the crossover is the smallest size at
// which the "par" variant is reliably faster than the "seq" one.
func BenchmarkMulFFTParallelSweep(b *testing.B) {
	old := int(maxParallelism.Load())
	b.Cleanup(func() { maxParallelism.Store(int64(old)) })

	for _, bits := range benchmarkParallelSizes {
		x := natToInt(rndNat(bits / _W))
		y := natToInt(rndNat(bits / _W))
		for _, mode := range []struct {
			name    string
			workers int
		}{{"seq", 1}, {"par", 0}} {
			// The name is in benchstat's key=value form so that the two
			// modes can be pivoted into columns: benchstat -col /mode.
			b.Run(fmt.Sprintf("size=%dkb/mode=%s", bits/1000, mode.name), func(b *testing.B) {
				SetMaxParallelism(mode.workers)
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					_ = mulFFT(x, y)
				}
			})
		}
	}
}
