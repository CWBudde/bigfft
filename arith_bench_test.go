package bigfft

import (
	"fmt"
	"math/bits"
	"testing"
)

func BenchmarkArithPrimitives(b *testing.B) {
	for _, n := range []int{16, 80, 384} {
		x, y := arithInputs(n, int64(5000+n))
		z := make([]Word, n)

		benchArithPair(b, "addVV", n,
			func() { addVV(z, x, y) },
			func() { addVVGo(z, x, y) })
		benchArithPair(b, "subVV", n,
			func() { subVV(z, x, y) },
			func() { subVVGo(z, x, y) })
		benchArithPair(b, "shlVU", n,
			func() { shlVU(z, x, 13) },
			func() { lshVUGo(z, x, 13) })
		copy(z, y)
		benchArithPair(b, "addMulVVW", n,
			func() { addMulVVW(z, x, Word(0xfedcba9)) },
			func() { addMulVVWGo(z, x, Word(0xfedcba9)) })
	}
}

func benchArithPair(b *testing.B, name string, n int, asm, goImpl func()) {
	b.Helper()
	for _, impl := range []struct {
		name string
		fn   func()
	}{
		{name: "selected", fn: asm},
		{name: "go", fn: goImpl},
	} {
		b.Run(fmt.Sprintf("%s/n=%d/impl=%s", name, n, impl.name), func(b *testing.B) {
			b.SetBytes(int64(n * (bits.UintSize / 8)))
			b.ReportAllocs()
			for range b.N {
				impl.fn()
			}
		})
	}
}
