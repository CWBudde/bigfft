//go:build (amd64 || arm64) && !purego

package bigfft

import "testing"

func BenchmarkButterflyImplementations(b *testing.B) {
	for _, s := range benchSizes {
		x, y := benchRndFermat(s.n), benchRndFermat(s.n)
		b.Run(s.name()+"/separate", func(b *testing.B) {
			sum, diff := make(fermat, s.n+1), make(fermat, s.n+1)
			for b.Loop() {
				diff.Sub(x, y)
				sum.Add(x, y)
			}
			benchSinkFermat = sum
		})
		b.Run(s.name()+"/fused", func(b *testing.B) {
			sum, diff := make(fermat, s.n+1), make(fermat, s.n+1)
			for b.Loop() {
				butterfly(sum, diff, x, y)
			}
			benchSinkFermat = sum
		})
	}
}
