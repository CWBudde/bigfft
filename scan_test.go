package bigfft

import (
	"math/big"
	"testing"
	"time"
)

func TestScan(t *testing.T) {
	for size := 10; size <= 1e5; size += 191 {
		s := rndStr(size)
		x, ok := new(big.Int).SetString(s, 10)
		if !ok {
			t.Fatal("cannot parse", s)
		}
		t0 := time.Now()
		y := FromDecimalString(s)
		if x.Cmp(y) != 0 {
			t.Errorf("failed at size %d", size)
		} else {
			t.Logf("OK for size %d in %s", size, time.Since(t0))
		}
	}
}

func BenchmarkScanFast1k(b *testing.B)   { benchmarkScanFast(1e3, b) }
func BenchmarkScanFast10k(b *testing.B)  { benchmarkScanFast(10e3, b) }
func BenchmarkScanFast100k(b *testing.B) { benchmarkScanFast(100e3, b) }
func BenchmarkScanFast1M(b *testing.B)   { benchmarkScanFast(1e6, b) }
func BenchmarkScanFast2M(b *testing.B)   { benchmarkScanFast(2e6, b) }
func BenchmarkScanFast5M(b *testing.B)   { benchmarkScanFast(5e6, b) }
func BenchmarkScanFast10M(b *testing.B)  { benchmarkScanFast(10e6, b) }

// func BenchmarkScanFast100M(b *testing.B) { benchmarkScanFast(100e6, b) }

func benchmarkScanFast(n int, b *testing.B) {
	s := rndStr(n)
	var x *big.Int
	for i := 0; i < b.N; i++ {
		x = FromDecimalString(s)
	}
	_ = x
}

func BenchmarkScanBig1k(b *testing.B)   { benchmarkScanBig(1e3, b) }
func BenchmarkScanBig10k(b *testing.B)  { benchmarkScanBig(10e3, b) }
func BenchmarkScanBig100k(b *testing.B) { benchmarkScanBig(100e3, b) }
func BenchmarkScanBig1M(b *testing.B)   { benchmarkScanBig(1e6, b) }
func BenchmarkScanBig2M(b *testing.B)   { benchmarkScanBig(2e6, b) }
func BenchmarkScanBig5M(b *testing.B)   { benchmarkScanBig(5e6, b) }
func BenchmarkScanBig10M(b *testing.B)  { benchmarkScanBig(10e6, b) }

func benchmarkScanBig(n int, b *testing.B) {
	s := rndStr(n)
	var x big.Int
	for i := 0; i < b.N; i++ {
		x.SetString(s, 10)
	}
}

func rndStr(n int) string {
	x := make([]byte, n)
	for i := 0; i < n; i++ {
		x[i] = '0' + byte(rnd.Intn(10))
	}
	return string(x)
}

// TestScanPowerTable checks the invariants init relies on: the chain halves, the
// last entry is small enough to hand to SetString, and every entry really is
// 10^digits[d]. The table is built by repeated squaring with a correction, so a
// single wrong correction would silently produce a wrong number at one size
// only — the kind of bug a spot-check at one length would miss.
func TestScanPowerTable(t *testing.T) {
	for _, size := range []int{2500, 10000, 99999, 1e6} {
		var s scanner
		s.init(size)
		if len(s.digits) == 0 {
			t.Fatalf("size %d: empty table", size)
		}
		if got := s.digits[len(s.digits)-1]; got > quadraticScanThreshold {
			t.Errorf("size %d: deepest chunk %d exceeds the threshold %d",
				size, got, quadraticScanThreshold)
		}
		for d := range s.digits {
			if d > 0 && s.digits[d] != s.digits[d-1]/2 {
				t.Errorf("size %d: digits[%d]=%d is not half of digits[%d]=%d",
					size, d, s.digits[d], d-1, s.digits[d-1])
			}
			want := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(s.digits[d])), nil)
			if s.pow[d].Cmp(want) != 0 {
				t.Errorf("size %d: pow[%d] != 10^%d", size, d, s.digits[d])
			}
		}
	}
}

// TestScanThresholds runs the whole scan at thresholds that are not multiples of
// 14, which the previous (10^14)^(threshold/14) base made illegal, and at sizes
// that sit exactly on the recursion's split boundaries.
func TestScanThresholds(t *testing.T) {
	old := quadraticScanThreshold
	defer func() { quadraticScanThreshold = old }()

	for _, thr := range []int{100, 137, 1000, 1232, 2001} {
		quadraticScanThreshold = thr
		for _, size := range []int{
			thr - 1, thr, thr + 1, 2*thr - 1, 2 * thr, 2*thr + 1,
			4*thr + 3, 8*thr - 5, 13 * thr, 100*thr + 7,
		} {
			if size <= 0 {
				continue
			}
			str := rndStr(size)
			want, ok := new(big.Int).SetString(str, 10)
			if !ok {
				t.Fatalf("cannot parse a %d-digit string", size)
			}
			if got := FromDecimalString(str); got.Cmp(want) != 0 {
				t.Errorf("threshold %d, size %d: wrong result", thr, size)
			}
		}
	}
}
