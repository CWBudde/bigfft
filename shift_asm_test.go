//go:build arm64 && !purego

package bigfft

import (
	"math/rand"
	"slices"
	"testing"
)

func TestShiftAsmMatchesGo(t *testing.T) {
	rng := rand.New(rand.NewSource(0x5a1f7))
	for _, n := range []int{1, 2, 3, 4, 5, 16, 17, 27, 31, 32, 64, 80} {
		N := n * _W
		shifts := []int{
			0, 1, _W - 1, _W, _W + 1,
			(n / 2) * _W, (n-1)*_W - 1, (n - 1) * _W,
			N - 1, N, N + 1, N + _W, N + (n/2)*_W, 2*N - 1,
		}
		values := specialFermats(n)
		for range 20 {
			x := make(fermat, n+1)
			for i := 0; i < n; i++ {
				x[i] = Word(rng.Uint64())
			}
			values = append(values, x)
		}
		for xi, x := range values {
			for _, k := range shifts {
				want := make(fermat, n+1)
				want.Shift(x, k)
				const guard = Word(0x5a1f7bad)
				buf := make([]Word, n+3)
				buf[0], buf[n+2] = guard, guard
				got := fermat(buf[1 : n+2])
				q := k % (2 * N)
				if q < 0 {
					q += 2 * N
				}
				shiftMod(&got[0], &x[0], uintptr(n), uintptr(q))
				if !slices.Equal(got, want) {
					t.Fatalf("n=%d x=%d k=%d: got %v, want %v", n, xi, k, []Word(got), []Word(want))
				}
				if buf[0] != guard || buf[n+2] != guard {
					t.Fatalf("n=%d x=%d k=%d: assembly wrote outside destination", n, xi, k)
				}
			}
		}
	}
}
