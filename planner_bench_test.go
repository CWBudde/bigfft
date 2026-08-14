package bigfft

import (
	"fmt"
	"math/big"
	"testing"
)

var (
	plannerSinkNat nat
	plannerSinkInt *big.Int
)

var plannerPlateauChunks = []int{
	55, 56, 60, 62, 63, 64, 65,
	79, 80, 87, 88, 92, 94, 95,
	111, 112, 119, 120,
}

// forcedFFTConfig derives the same chunk and coefficient sizes as fftSize and
// poly.Mul, but for a caller-selected transform length. The boolean excludes
// configurations for which valueSize's K/4 root-of-unity requirement cannot
// be expressed or either input would need more than K coefficients.
func forcedFFTConfig(x, y nat, k uint) (m, n int, ok bool) {
	if k < 2 || k > uint(len(fftSizeThreshold)+1) {
		return 0, 0, false
	}
	K := 1 << k
	m = natWordCount(x, y)>>k + 1
	if m <= 0 || len(x)/m+1 > K || len(y)/m+1 > K {
		return 0, 0, false
	}
	return m, valueSize(k, m, 2), true
}

// mulFFTForced is fftmul with only its planner decision forced. Everything
// after that decision is the production path: polynomial slicing, valueSize,
// the scratch arena, both transforms, pointwise multiplication, inverse
// transform, and integer reconstruction.
func mulFFTForced(x, y nat, k uint) nat {
	m, n, ok := forcedFFTConfig(x, y, k)
	if !ok {
		panic("invalid forced FFT configuration")
	}
	xp := polyFromNat(x, k, m)
	yp := polyFromNat(y, k, m)
	rp := xp.mul(&yp, newFFTScratch(k, n))
	return rp.Int()
}

// BenchmarkFFTPlannerPlateaus measures the incumbent plan and its valid k-1
// and k+1 neighbours around the recurring coefficient-padding steps at k=12.
// The points cover each selected window plus both neighboring controls; m=64,
// for example, is where valueSize jumps from n=128 to n=144 on 64-bit systems.
//
// Names use benchstat key=value dimensions, in particular /plan, so a run can
// be pivoted with:
//
//	benchstat -col /plan results.txt
//
// Besides time and allocations, workwords/op exposes the transform working
// set and padbits/op exposes the coefficient bits bought only by rounding.
func BenchmarkFFTPlannerPlateaus(b *testing.B) {
	const incumbentK = uint(12)
	for _, incumbentM := range plannerPlateauChunks {
		// fftSize computes m=(totalWords>>k)+1. Choose the first balanced
		// input point producing incumbentM at incumbentK.
		wordsPerOperand := ((incumbentM - 1) << incumbentK) / 2
		x := benchRndNat(wordsPerOperand)
		y := benchRndNat(wordsPerOperand)
		gotK, gotM := fftSize(x, y)
		if gotK != incumbentK || gotM != incumbentM {
			b.Fatalf("constructed plateau point selected k=%d m=%d, want k=%d m=%d",
				gotK, gotM, incumbentK, incumbentM)
		}

		for _, candidate := range []struct {
			plan string
			k    uint
		}{{"minus", gotK - 1}, {"incumbent", gotK}, {"plus", gotK + 1}} {
			m, n, ok := forcedFFTConfig(x, y, candidate.k)
			if !ok {
				continue
			}
			K := 1 << candidate.k
			neededBits := 2*m*_W + int(candidate.k)
			name := fmt.Sprintf("size=%dkb/m0=%d/plan=%s",
				wordsPerOperand*_W/1000, incumbentM, candidate.plan)
			b.Run(name, func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(int64(2 * wordsPerOperand * (_W / 8)))
				b.ResetTimer()
				for range b.N {
					plannerSinkNat = mulFFTForced(x, y, candidate.k)
				}
				b.ReportMetric(float64(candidate.k), "k/op")
				b.ReportMetric(float64(m), "m/op")
				b.ReportMetric(float64(n), "n/op")
				b.ReportMetric(float64(K), "points/op")
				b.ReportMetric(float64((n+1)*K), "workwords/op")
				b.ReportMetric(float64(n*_W-neededBits), "padbits/op")
			})
		}
	}
}

// BenchmarkMulPlannerPlateaus is the end-to-end counterpart to the forced
// grid. It is intentionally separate so binaries built before and after a
// planner change can be interleaved with the usual pinned A/B protocol.
func BenchmarkMulPlannerPlateaus(b *testing.B) {
	const incumbentK = uint(12)
	for _, incumbentM := range plannerPlateauChunks {
		wordsPerOperand := ((incumbentM - 1) << incumbentK) / 2
		xWords := benchRndNat(wordsPerOperand)
		yWords := benchRndNat(wordsPerOperand)
		var x, y big.Int
		x.SetBits(xWords)
		y.SetBits(yWords)
		gotK, gotM := fftSize(xWords, yWords)
		if gotK != incumbentK || gotM != incumbentM {
			b.Fatalf("constructed plateau point selected k=%d m=%d, want k=%d m=%d",
				gotK, gotM, incumbentK, incumbentM)
		}

		b.Run(fmt.Sprintf("size=%dkb/m0=%d", wordsPerOperand*_W/1000, incumbentM), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(2 * wordsPerOperand * (_W / 8)))
			for range b.N {
				plannerSinkInt = Mul(&x, &y)
			}
		})
	}
}

func TestMulFFTForcedMatchesBig(t *testing.T) {
	x := benchRndNat(37)
	y := benchRndNat(53)
	incumbent, _ := fftSize(x, y)

	var xi, yi, want big.Int
	xi.SetBits(x)
	yi.SetBits(y)
	want.Mul(&xi, &yi)
	for _, k := range []uint{incumbent - 1, incumbent, incumbent + 1} {
		if _, _, ok := forcedFFTConfig(x, y, k); !ok {
			continue
		}
		got := new(big.Int).SetBits(mulFFTForced(x, y, k))
		if got.Cmp(&want) != 0 {
			t.Fatalf("forced k=%d produced a wrong product", k)
		}
	}
}
