//go:build arm64 && !purego

package bigfft

import "testing"

// BenchmarkShiftARM64Implementations is intentionally same-binary: QEMU is
// useful for correctness but meaningless for timing, so this is ready for the
// next native arm64 run without conflating different build layouts.
func BenchmarkShiftARM64Implementations(b *testing.B) {
	for _, s := range benchSizes {
		for _, tc := range []struct {
			name string
			k    int
		}{
			{name: "unaligned", k: benchShift(s.n)},
			{name: "aligned", k: (s.n / 2) * _W},
			{name: "negative", k: s.n*_W + benchShift(s.n)},
		} {
			x := benchRndFermat(s.n)
			b.Run(s.name()+"/"+tc.name+"/go", func(b *testing.B) {
				z := make(fermat, s.n+1)
				for b.Loop() {
					z.Shift(x, tc.k)
				}
				benchSinkFermat = z
			})
			b.Run(s.name()+"/"+tc.name+"/asm", func(b *testing.B) {
				z := make(fermat, s.n+1)
				k := tc.k % (2 * s.n * _W)
				if k < 0 {
					k += 2 * s.n * _W
				}
				for b.Loop() {
					shiftMod(&z[0], &x[0], uintptr(s.n), uintptr(k))
				}
				benchSinkFermat = z
			})
		}
	}
}
