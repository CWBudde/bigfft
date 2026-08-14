package bigfft

import (
	"math/big"
	"testing"
)

func TestMakeFFTPlanInvariants(t *testing.T) {
	for k := uint(2); k <= 16; k++ {
		K := 1 << k
		for _, words := range []int{1, K - 1, K, K + 1, 3*K - 1, 3 * K, 3*K + 1} {
			p := makeFFTPlan(words, k)
			if p.m*K <= words {
				t.Errorf("words=%d k=%d: m*K=%d does not exceed the input", words, k, p.m*K)
			}
			neededBits := 2*p.m*_W + int(k)
			if p.n*_W <= neededBits {
				t.Errorf("words=%d k=%d: modulus has %d bits, need more than %d",
					words, k, p.n*_W, neededBits)
			}
			quarterK := K / 4
			if (p.n*_W)%quarterK != 0 {
				t.Errorf("words=%d k=%d: %d coefficient bits not divisible by K/4=%d",
					words, k, p.n*_W, quarterK)
			}
		}
	}
}

func TestMakeFFTPlanRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name  string
		words int
		k     uint
	}{
		{name: "negative words", words: -1, k: 12},
		{name: "small k", words: 1, k: 1},
		{name: "large k", words: 1, k: uint(len(fftSizeThreshold) + 2)},
		{name: "coefficient overflow", words: int(^uint(0) >> 1), k: 2},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("makeFFTPlan did not reject an invalid input")
				}
			}()
			_ = makeFFTPlan(test.words, test.k)
		})
	}
}

func TestSelectFFTPlanPlateaus(t *testing.T) {
	if !plateauPlannerEnabled {
		words := int((fftSizeThreshold[11] + int64(_W) - 1) / int64(_W))
		incumbentK, _ := fftSizeWords(words)
		if incumbentK != 12 {
			t.Fatalf("constructed disabled-policy point has incumbent k=%d, want k=12", incumbentK)
		}
		if got := selectFFTPlanWords(words); got.k != incumbentK {
			t.Fatalf("disabled plateau planner selected k=%d, want incumbent k=%d", got.k, incumbentK)
		}
		return
	}
	// k=12 owns m=33..120 in the shipped threshold table. These are the
	// measured windows where k=13's coefficient length is less than 4/7 of
	// the incumbent's; at the neighboring equality points it does not win.
	wantNext := map[int]bool{}
	for _, bounds := range [][2]int{{56, 62}, {80, 94}, {112, 120}} {
		for m := bounds[0]; m <= bounds[1]; m++ {
			wantNext[m] = true
		}
	}

	for m := 33; m <= 120; m++ {
		words := (m - 1) << 12
		incumbentK, incumbentM := fftSizeWords(words)
		if incumbentK != 12 || incumbentM != m {
			t.Fatalf("constructed point has k=%d m=%d, want k=12 m=%d", incumbentK, incumbentM, m)
		}
		got := selectFFTPlanWords(words)
		if wantNext[m] {
			if got.k != 13 || 7*got.n >= 4*makeFFTPlan(words, 12).n {
				t.Errorf("m=%d: got %+v, want the measured k=13 plan", m, got)
			}
		} else if got.k != 12 || got.m != m {
			t.Errorf("m=%d: got %+v, want the k=12 incumbent", m, got)
		}
	}
}

func TestSelectedFFTPlanMatchesBig(t *testing.T) {
	if !plateauPlannerEnabled {
		t.Skip("plateau-aware production policy is currently calibrated only for amd64")
	}

	tests := []struct {
		name       string
		m          int
		unbalanced bool
		allOnes    bool
	}{
		{name: "first window", m: 56},
		{name: "middle window all ones", m: 80, allOnes: true},
		{name: "last window unbalanced", m: 112, unbalanced: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			totalWords := (test.m - 1) << 12
			xWords, yWords := totalWords/2, totalWords-totalWords/2
			if test.unbalanced {
				xWords, yWords = totalWords-1, 1
			}
			x := make(nat, xWords)
			y := make(nat, yWords)
			for i := range x {
				x[i] = Word(uint64(i)*0x9e3779b97f4a7c15 + 1)
				if test.allOnes {
					x[i] = ^Word(0)
				}
			}
			for i := range y {
				y[i] = Word(uint64(i)*0xd1b54a32d192ed03 + 3)
				if test.allOnes {
					y[i] = ^Word(0)
				}
			}
			if got := selectFFTPlan(x, y); got.k != 13 {
				t.Fatalf("selected k=%d, want the measured k=13 plan", got.k)
			}

			var xi, yi, want big.Int
			xi.SetBits(x)
			yi.SetBits(y)
			want.Mul(&xi, &yi)
			got := new(big.Int).SetBits(fftmul(x, y))
			if got.Cmp(&want) != 0 {
				t.Fatal("plateau-aware FFT plan produced a wrong product")
			}
		})
	}
}

func TestSelectFFTPlanLeavesOtherLengthsAlone(t *testing.T) {
	for _, k := range []uint{8, 9, 10, 11, 13, 14, 15} {
		threshold := fftSizeThreshold[k]
		words := int((threshold - 1) / int64(_W))
		incumbentK, _ := fftSizeWords(words)
		if incumbentK != k {
			t.Fatalf("threshold point selected k=%d, want %d", incumbentK, k)
		}
		if got := selectFFTPlanWords(words); got.k != k {
			t.Errorf("incumbent k=%d unexpectedly changed to k=%d", k, got.k)
		}
	}
}
