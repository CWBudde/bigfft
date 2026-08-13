package bigfft

import (
	"math/big"
	"math/rand"
	"testing"
)

var shRnd = rand.New(rand.NewSource(0x5f3759df1234beef))

// rndFermat returns a random, normalized fermat of n+1 words.
func rndFermat(n int) fermat {
	f := make(fermat, n+1)
	for i := range n {
		f[i] = Word(shRnd.Uint64())
	}
	f.norm()
	return f
}

// specialFermats returns interesting edge-case values modulo 2^(n*_W)+1.
func specialFermats(n int) []fermat {
	zero := make(fermat, n+1)
	one := make(fermat, n+1)
	one[0] = 1
	ones := make(fermat, n+1) // 2^N-1
	for i := range n {
		ones[i] = ^Word(0)
	}
	minusOne := make(fermat, n+1) // 2^N ≡ -1
	minusOne[n] = 1
	half := make(fermat, n+1) // 2^(N/2)
	half[n/2] = 1
	if n%2 != 0 {
		half[n/2] = 1 << uint(_W/2)
	}
	topOnly := make(fermat, n+1) // high half all ones
	for i := n / 2; i < n; i++ {
		topOnly[i] = ^Word(0)
	}
	lowOnly := make(fermat, n+1) // low half all ones
	for i := range n / 2 {
		lowOnly[i] = ^Word(0)
	}
	return []fermat{zero, one, ones, minusOne, half, topOnly, lowOnly}
}

// shiftHalfTestKs returns a varied list of odd shift amounts for a given n,
// including negative ones, ones beyond 2N and ones close to multiples of N.
func shiftHalfTestKs(n int) []int {
	N := n * _W
	ks := []int{1, -1, 3, -3, 5, 7, 9, 11, 13, 15, 63, 65, 127, 129}
	// 2*N is the period of ShiftHalf's argument (k/2 has period 2N bits).
	for _, base := range []int{0, N / 2, N, 3 * N / 2, 2 * N, 3 * N, 4 * N, 5 * N} {
		for _, d := range []int{-5, -3, -1, 1, 3, 5} {
			k := base + d
			if k%2 == 0 {
				k++
			}
			ks = append(ks, k, -k)
		}
	}
	for range 64 {
		k := 2*shRnd.Intn(8*N+1) + 1
		if shRnd.Intn(2) == 0 {
			k = -k
		}
		ks = append(ks, k)
	}
	return ks
}

func fermatEqual(a, b fermat) bool {
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

// TestShiftHalfDoubleValue checks the actual value computed by ShiftHalf
// (not just agreement with the old code) by applying it twice: shifting
// twice by k/2 must be a shift by k.
func TestShiftHalfDoubleValue(t *testing.T) {
	for _, n := range []int{2, 3, 4, 5, 16, 17} {
		N := uint(n * _W)
		mod := new(big.Int).Lsh(big.NewInt(1), N)
		mod.Add(mod, big.NewInt(1))

		z := make(fermat, n+1)
		z2 := make(fermat, n+1)
		tmp := make(fermat, n+1)
		tmp2 := make(fermat, n+1)
		for _, x := range append(specialFermats(n), rndFermat(n), rndFermat(n), rndFermat(n)) {
			for _, k := range shiftHalfTestKs(n) {
				z.ShiftHalf(x, k, tmp)
				z2.ShiftHalf(z, k, tmp2)

				// 2^(2N) ≡ 1 mod 2^N+1.
				kk := k % (2 * int(N))
				if kk < 0 {
					kk += 2 * int(N)
				}
				want := new(big.Int).Lsh(new(big.Int).SetBits(x), uint(kk))
				want.Mod(want, mod)
				got := new(big.Int).SetBits(z2)
				if got.Cmp(want) != 0 {
					t.Fatalf("n=%d k=%d x=%x: double shift gives %x, want %x",
						n, k, nat(x), got, want)
				}
			}
		}
	}
}

// TestShiftHalfAliasFreeTmp checks that the fast path leaves tmp usable as a
// scratch buffer only, i.e. that callers may share a single tmp across calls.
func TestShiftHalfAliasFreeTmp(t *testing.T) {
	const n = 16
	x := rndFermat(n)
	y := rndFermat(n)
	tmp := make(fermat, n+1)
	a := make(fermat, n+1)
	b := make(fermat, n+1)
	ref := make(fermat, n+1)
	for _, k := range []int{1, 7, 12345, -9} {
		a.ShiftHalf(x, k, tmp)
		b.ShiftHalf(y, k, tmp)
		ref.ShiftHalf(x, k, make(fermat, n+1))
		if !fermatEqual(a, ref) {
			t.Fatalf("k=%d: shared tmp changed the result", k)
		}
		ref.ShiftHalf(y, k, make(fermat, n+1))
		if !fermatEqual(b, ref) {
			t.Fatalf("k=%d: shared tmp changed the second result", k)
		}
	}
}
