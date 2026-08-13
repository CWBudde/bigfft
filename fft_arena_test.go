package bigfft

import (
	"math/big"
	"math/rand"
	"testing"
	"unsafe"
)

// arenaSizes are operand sizes in words. They straddle fftThreshold (1800)
// and reach far enough to exercise FFT sizes k=10 and k=11: an operand of
// 20000 words gives 40000*_W bits of product, which selects k=10 on a
// 64-bit machine, and 31000 words selects k=11.
var arenaSizes = []int{1, 2, 17, 64, 1799, 1800, 1801, 1900, 2500, 4000, 8000, 12000, 20000, 31000}

func natToInt(x nat) *big.Int {
	z := new(big.Int)
	z.SetBits(append(nat(nil), x...))
	return z
}

// TestArenaMulExact multiplies many random operand pairs of assorted sizes
// and requires bit-exact agreement with math/big, both through the public
// Mul entry point and through mulFFT, which forces the FFT path even below
// the threshold.
func TestArenaMulExact(t *testing.T) {
	sizes := arenaSizes
	iters := 3
	if testing.Short() {
		sizes = arenaSizes[:len(arenaSizes)-3]
		iters = 1
	}
	rng := rand.New(rand.NewSource(0x5eed1234))
	for _, sx := range sizes {
		for _, sy := range sizes {
			for it := 0; it < iters; it++ {
				xb, yb := arenaRandNat(rng, sx), arenaRandNat(rng, sy)
				x, y := natToInt(xb), natToInt(yb)
				want := new(big.Int).Mul(x, y)

				if got := Mul(x, y); got.Cmp(want) != 0 {
					t.Fatalf("Mul mismatch for sizes %d x %d (iter %d)", sx, sy, it)
				}
				if got := mulFFT(x, y); got.Cmp(want) != 0 {
					t.Fatalf("mulFFT mismatch for sizes %d x %d (iter %d)", sx, sy, it)
				}
				// The operands must not have been modified.
				if natToInt(xb).Cmp(x) != 0 || natToInt(yb).Cmp(y) != 0 {
					t.Fatalf("operands clobbered for sizes %d x %d", sx, sy)
				}
			}
		}
	}
}

// TestArenaMulSaturated repeats the check with all-ones operands, the worst
// case for carries in the modular arithmetic.
func TestArenaMulSaturated(t *testing.T) {
	sizes := arenaSizes
	if testing.Short() {
		sizes = arenaSizes[:len(arenaSizes)-3]
	}
	for _, sx := range sizes {
		for _, sy := range sizes {
			xb, yb := make(nat, sx), make(nat, sy)
			for i := range xb {
				xb[i] = ^big.Word(0)
			}
			for i := range yb {
				yb[i] = ^big.Word(0)
			}
			x, y := natToInt(xb), natToInt(yb)
			want := new(big.Int).Mul(x, y)
			if got := mulFFT(x, y); got.Cmp(want) != 0 {
				t.Fatalf("mulFFT mismatch for saturated sizes %d x %d", sx, sy)
			}
		}
	}
}

func arenaRandNat(rng *rand.Rand, n int) nat {
	x := make(nat, n)
	for i := range x {
		x[i] = big.Word(rng.Uint64())
	}
	// Avoid a zero top word so the operand really has n words.
	if n > 0 && x[n-1] == 0 {
		x[n-1] = 1
	}
	return x
}

// fftmulScribble mirrors fftmul but keeps hold of the scratch arena and
// fills it with garbage once the result has been built. If the returned
// nat aliased the arena in any way the comparison in the caller would fail.
func fftmulScribble(x, y nat) (nat, *fftScratch, bool) {
	k, m := fftSize(x, y)
	xp := polyFromNat(x, k, m)
	yp := polyFromNat(y, k, m)
	s := newFFTScratch(k, valueSize(k, m, 2))
	rp := xp.mul(&yp, s)
	// The reconstructed polynomial is expected to live inside the arena;
	// that is what makes the scribbling below a meaningful check.
	coeffsInArena := overlaps(rp.a[0], s.a)
	z := rp.Int()
	for _, region := range arenaRegions(s) {
		for i := range region.w {
			region.w[i] = ^big.Word(0)
		}
	}
	return z, s, coeffsInArena
}

// arenaRegion is one named storage region of a scratch arena.
type arenaRegion struct {
	name string
	w    []big.Word
}

// arenaRegions lists every distinct storage region of s.
func arenaRegions(s *fftScratch) []arenaRegion {
	return []arenaRegion{
		{"a", s.a},
		{"b", s.b},
		{"c", s.c},
		{"u", s.u},
		{"tmp", s.ft.tmp},
		{"tmp2", s.ft.tmp2},
		{"mbuf", s.mbuf},
	}
}

// overlaps reports whether the two word slices share any storage.
func overlaps(a, b []big.Word) bool {
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	aLo := uintptr(unsafe.Pointer(&a[0]))
	aHi := aLo + uintptr(len(a))*unsafe.Sizeof(a[0])
	bLo := uintptr(unsafe.Pointer(&b[0]))
	bHi := bLo + uintptr(len(b))*unsafe.Sizeof(b[0])
	return aLo < bHi && bLo < aHi
}

// TestArenaResultDoesNotAlias checks in two independent ways that the nat
// returned by fftmul is disjoint from the scratch arena: by scribbling over
// the whole arena after the fact and re-checking the product, and by
// comparing the backing address ranges.
func TestArenaResultDoesNotAlias(t *testing.T) {
	sizes := []int{2000, 20000}
	if testing.Short() {
		sizes = []int{2000}
	}
	rng := rand.New(rand.NewSource(0xa11a5))
	for _, size := range sizes {
		xb, yb := arenaRandNat(rng, size), arenaRandNat(rng, size)
		x, y := natToInt(xb), natToInt(yb)
		want := new(big.Int).Mul(x, y)

		zb, s, coeffsInArena := fftmulScribble(xb, yb)
		if !coeffsInArena {
			t.Fatalf("size %d: poly coefficients are not in the arena, "+
				"the scribble check would be vacuous", size)
		}
		got := new(big.Int)
		got.SetBits(zb)
		if got.Cmp(want) != 0 {
			t.Fatalf("size %d: product changed after scribbling the arena; result aliases it", size)
		}
		for _, region := range arenaRegions(s) {
			if overlaps(zb, region.w) {
				t.Errorf("size %d: result overlaps arena region %s", size, region.name)
			}
		}
	}
}

// capEnd returns the address just past the capacity of x.
func capEnd(x []big.Word) uintptr {
	full := x[:cap(x)]
	return uintptr(unsafe.Pointer(&full[0])) + uintptr(cap(x))*unsafe.Sizeof(x[0])
}

// TestArenaCapacitiesBounded guards the three-index slicing in
// newFFTScratch. math/big decides whether it may reuse a destination buffer
// by comparing the address of the last word of the destination's capacity
// with that of its operands. If the arena's sub-slices were not capacity
// bounded they would all run to the end of the backing array, mbuf would
// look like an alias of the values it multiplies, and big.Int.Mul would
// allocate a fresh buffer for each of the 1<<k pointwise products.
//
// The invariant is checked structurally rather than with
// testing.AllocsPerRun because the race detector perturbs allocation counts.
func TestArenaCapacitiesBounded(t *testing.T) {
	s := newFFTScratch(6, 64)
	named := append(arenaRegions(s), []arenaRegion{
		{"hIn[0]", s.hIn[0]},
		{"hP[0]", s.hP[0]},
		{"hQ[0]", s.hQ[0]},
		{"hR[3]", s.hR[3]},
		{"hP[3]", s.hP[3]},
	}...)
	for _, r := range named {
		if cap(r.w) != len(r.w) {
			t.Errorf("%s: cap %d != len %d, arena sub-slice is not bounded",
				r.name, cap(r.w), len(r.w))
		}
	}
	// The property that actually matters to math/big.
	for i := range s.hP {
		if capEnd(s.mbuf) == capEnd(s.hP[i]) || capEnd(s.mbuf) == capEnd(s.hQ[i]) {
			t.Fatalf("mbuf shares a capacity end with value %d; "+
				"math/big will refuse to reuse it", i)
		}
	}
}

// TestArenaRegionsDisjoint documents the intended arena layout: the three
// large regions must not overlap each other.
func TestArenaRegionsDisjoint(t *testing.T) {
	s := newFFTScratch(6, 8)
	regions := arenaRegions(s)
	for i := range regions {
		for j := i + 1; j < len(regions); j++ {
			if overlaps(regions[i].w, regions[j].w) {
				t.Errorf("regions %s and %s overlap", regions[i].name, regions[j].name)
			}
		}
	}
	// The product values deliberately share storage with p's values.
	if !overlaps(s.hP[0], s.hR[0]) {
		t.Error("hR is expected to alias hP")
	}
	// The inverse transform destination must not alias the values it reads.
	if overlaps(s.hIn[0], s.hP[0]) || overlaps(s.hIn[0], s.hQ[0]) {
		t.Error("hIn must be disjoint from hP and hQ")
	}
}
