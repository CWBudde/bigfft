package bigfft

import (
	"math/big"
	"math/rand"
	"slices"
	"testing"
)

func TestButterflyWordsMatchesGo(t *testing.T) {
	rng := rand.New(rand.NewSource(0xb077e2f1))
	for _, n := range []int{0, 1, 2, 3, 4, 5, 7, 8, 9, 16, 17, 31, 64, 80, 127} {
		for iter := 0; iter < 100; iter++ {
			x := make([]Word, n)
			y := make([]Word, n)
			for i := range x {
				x[i] = Word(rng.Uint64())
				y[i] = Word(rng.Uint64())
			}
			if iter == 0 {
				for i := range x {
					x[i], y[i] = ^Word(0), ^Word(0)
				}
			}

			wantSum, wantDiff := make([]Word, n), make([]Word, n)
			wantCarry, wantBorrow := butterflyWordsGo(wantSum, wantDiff, x, y)

			const guard = Word(0xd1ff3a5b)
			sumBuf, diffBuf := make([]Word, n+2), make([]Word, n+2)
			sumBuf[0], sumBuf[n+1] = guard, guard
			diffBuf[0], diffBuf[n+1] = guard, guard
			sum, diff := sumBuf[1:n+1], diffBuf[1:n+1]
			carry, borrow := butterflyWords(sum, diff, x, y)

			if carry != wantCarry || borrow != wantBorrow ||
				!slices.Equal(sum, wantSum) || !slices.Equal(diff, wantDiff) {
				t.Fatalf("n=%d iter=%d: got (%x,%x,%x,%x), want (%x,%x,%x,%x)",
					n, iter, sum, diff, carry, borrow,
					wantSum, wantDiff, wantCarry, wantBorrow)
			}
			if sumBuf[0] != guard || sumBuf[n+1] != guard ||
				diffBuf[0] != guard || diffBuf[n+1] != guard {
				t.Fatalf("n=%d iter=%d: assembly wrote outside a destination", n, iter)
			}
		}
	}
}

func TestButterflyMatchesAddSub(t *testing.T) {
	rng := rand.New(rand.NewSource(0xf37a47))
	for _, n := range []int{1, 2, 3, 4, 5, 16, 17, 27, 64, 80} {
		values := specialFermats(n)
		for range 20 {
			x := make(fermat, n+1)
			for i := 0; i < n; i++ {
				x[i] = big.Word(rng.Uint64())
			}
			values = append(values, x)
		}

		for xi, x := range values {
			for yi, y := range values {
				wantSum, wantDiff := make(fermat, n+1), make(fermat, n+1)
				wantSum.Add(x, y)
				wantDiff.Sub(x, y)

				// The production alias pattern: the sum overwrites x while the
				// difference is a separate destination.
				sum, diff := append(fermat(nil), x...), make(fermat, n+1)
				butterfly(sum, diff, sum, y)
				if !slices.Equal(sum, wantSum) || !slices.Equal(diff, wantDiff) {
					t.Fatalf("n=%d x=%d y=%d: got sum=%x diff=%x, want sum=%x diff=%x",
						n, xi, yi, sum, diff, wantSum, wantDiff)
				}

				// Also pin the exact diff==y alias supported by the word loop.
				sum = make(fermat, n+1)
				diff = append(fermat(nil), y...)
				butterfly(sum, diff, x, diff)
				if !slices.Equal(sum, wantSum) || !slices.Equal(diff, wantDiff) {
					t.Fatalf("n=%d x=%d y=%d diff-alias mismatch", n, xi, yi)
				}
			}
		}
	}
}
