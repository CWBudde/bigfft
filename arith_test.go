package bigfft

import (
	"math/bits"
	"math/rand"
	"slices"
	"testing"
)

const arithGuardWords = 2

var arithGuard = ^Word(0) ^ Word(0x5a5a)

func guardedWords(n int) (raw, words []Word) {
	raw = make([]Word, n+2*arithGuardWords)
	for i := range arithGuardWords {
		raw[i] = arithGuard
		raw[len(raw)-1-i] = arithGuard
	}
	return raw, raw[arithGuardWords : arithGuardWords+n]
}

func checkArithGuards(t *testing.T, raw []Word) {
	t.Helper()
	for i := range arithGuardWords {
		if raw[i] != arithGuard || raw[len(raw)-1-i] != arithGuard {
			t.Fatalf("guard overwritten: %x", raw)
		}
	}
}

func arithInputs(n int, seed int64) (x, y []Word) {
	rng := rand.New(rand.NewSource(seed))
	x = make([]Word, n)
	y = make([]Word, n)
	for i := range n {
		x[i] = Word(rng.Uint64())
		y[i] = Word(rng.Uint64())
	}
	return x, y
}

func TestArithVVMatchesGo(t *testing.T) {
	lengths := []int{0, 1, 2, 3, 4, 5, 7, 8, 9, 15, 16, 17, 31, 32, 33, 80}
	for _, n := range lengths {
		x, y := arithInputs(n, int64(1000+n))
		if n > 0 {
			x[0], y[0] = ^Word(0), 1
			x[n-1], y[n-1] = ^Word(0), ^Word(0)
		}
		xOriginal, yOriginal := slices.Clone(x), slices.Clone(y)

		wantAdd := make([]Word, n)
		wantAddCarry := addVVGo(wantAdd, x, y)
		addRaw, gotAdd := guardedWords(n)
		if gotCarry := addVV(gotAdd, x, y); gotCarry != wantAddCarry || !slices.Equal(gotAdd, wantAdd) {
			t.Fatalf("addVV n=%d: got (%x, %x), want (%x, %x)", n, gotAdd, gotCarry, wantAdd, wantAddCarry)
		}
		checkArithGuards(t, addRaw)

		wantSub := make([]Word, n)
		wantSubCarry := subVVGo(wantSub, x, y)
		subRaw, gotSub := guardedWords(n)
		if gotCarry := subVV(gotSub, x, y); gotCarry != wantSubCarry || !slices.Equal(gotSub, wantSub) {
			t.Fatalf("subVV n=%d: got (%x, %x), want (%x, %x)", n, gotSub, gotCarry, wantSub, wantSubCarry)
		}
		checkArithGuards(t, subRaw)

		if !slices.Equal(x, xOriginal) || !slices.Equal(y, yOriginal) {
			t.Fatalf("n=%d: out-of-place operation changed an input", n)
		}

		for _, alias := range []string{"x", "y"} {
			addAliasRaw, addAlias := guardedWords(n)
			subAliasRaw, subAlias := guardedWords(n)
			if alias == "x" {
				copy(addAlias, x)
				copy(subAlias, x)
				if c := addVV(addAlias, addAlias, y); c != wantAddCarry || !slices.Equal(addAlias, wantAdd) {
					t.Fatalf("addVV n=%d z==x: got (%x, %x), want (%x, %x)", n, addAlias, c, wantAdd, wantAddCarry)
				}
				if c := subVV(subAlias, subAlias, y); c != wantSubCarry || !slices.Equal(subAlias, wantSub) {
					t.Fatalf("subVV n=%d z==x: got (%x, %x), want (%x, %x)", n, subAlias, c, wantSub, wantSubCarry)
				}
			} else {
				copy(addAlias, y)
				copy(subAlias, y)
				if c := addVV(addAlias, x, addAlias); c != wantAddCarry || !slices.Equal(addAlias, wantAdd) {
					t.Fatalf("addVV n=%d z==y: got (%x, %x), want (%x, %x)", n, addAlias, c, wantAdd, wantAddCarry)
				}
				if c := subVV(subAlias, x, subAlias); c != wantSubCarry || !slices.Equal(subAlias, wantSub) {
					t.Fatalf("subVV n=%d z==y: got (%x, %x), want (%x, %x)", n, subAlias, c, wantSub, wantSubCarry)
				}
			}
			checkArithGuards(t, addAliasRaw)
			checkArithGuards(t, subAliasRaw)
		}
	}
}

func TestArithCarryChains(t *testing.T) {
	for _, n := range []int{1, 2, 3, 4, 5, 7, 8, 9, 31, 32, 33, 80} {
		allMax := make([]Word, n)
		zero := make([]Word, n)
		one := make([]Word, n)
		for i := range allMax {
			allMax[i] = ^Word(0)
		}
		one[0] = 1

		addRaw, addDst := guardedWords(n)
		copy(addDst, allMax)
		if c := addVV(addDst, addDst, one); c != 1 || !slices.Equal(addDst, zero) {
			t.Fatalf("addVV n=%d full carry: got (%x, %x), want (%x, 1)", n, addDst, c, zero)
		}
		checkArithGuards(t, addRaw)

		subRaw, subDst := guardedWords(n)
		if c := subVV(subDst, subDst, one); c != 1 || !slices.Equal(subDst, allMax) {
			t.Fatalf("subVV n=%d full borrow: got (%x, %x), want (%x, 1)", n, subDst, c, allMax)
		}
		checkArithGuards(t, subRaw)

		addVWRaw, addVWDst := guardedWords(n)
		copy(addVWDst, allMax)
		if c := addVW(addVWDst, addVWDst, 1); c != 1 || !slices.Equal(addVWDst, zero) {
			t.Fatalf("addVW n=%d full carry: got (%x, %x), want (%x, 1)", n, addVWDst, c, zero)
		}
		checkArithGuards(t, addVWRaw)

		subVWRaw, subVWDst := guardedWords(n)
		if c := subVW(subVWDst, subVWDst, 1); c != 1 || !slices.Equal(subVWDst, allMax) {
			t.Fatalf("subVW n=%d full borrow: got (%x, %x), want (%x, 1)", n, subVWDst, c, allMax)
		}
		checkArithGuards(t, subVWRaw)

		wantMul := slices.Clone(allMax)
		wantMulCarry := addMulVVWGo(wantMul, allMax, ^Word(0))
		mulRaw, mulDst := guardedWords(n)
		copy(mulDst, allMax)
		if c := addMulVVW(mulDst, allMax, ^Word(0)); c != wantMulCarry || !slices.Equal(mulDst, wantMul) {
			t.Fatalf("addMulVVW n=%d full carry: got (%x, %x), want (%x, %x)", n, mulDst, c, wantMul, wantMulCarry)
		}
		checkArithGuards(t, mulRaw)
	}
}

func TestArithVWMatchesReference(t *testing.T) {
	lengths := []int{0, 1, 2, 3, 4, 5, 8, 9, 32}
	words := []Word{0, 1, 2, ^Word(0)}
	for _, n := range lengths {
		x, _ := arithInputs(n, int64(2000+n))
		for _, y := range words {
			wantAdd := make([]Word, n)
			wantAddCarry := addVWReference(wantAdd, x, y)
			addRaw, gotAdd := guardedWords(n)
			if c := addVW(gotAdd, x, y); c != wantAddCarry || !slices.Equal(gotAdd, wantAdd) {
				t.Fatalf("addVW n=%d y=%x: got (%x, %x), want (%x, %x)", n, y, gotAdd, c, wantAdd, wantAddCarry)
			}
			checkArithGuards(t, addRaw)

			wantSub := make([]Word, n)
			wantSubCarry := subVWReference(wantSub, x, y)
			subRaw, gotSub := guardedWords(n)
			if c := subVW(gotSub, x, y); c != wantSubCarry || !slices.Equal(gotSub, wantSub) {
				t.Fatalf("subVW n=%d y=%x: got (%x, %x), want (%x, %x)", n, y, gotSub, c, wantSub, wantSubCarry)
			}
			checkArithGuards(t, subRaw)

			addAliasRaw, addAlias := guardedWords(n)
			copy(addAlias, x)
			if c := addVW(addAlias, addAlias, y); c != wantAddCarry || !slices.Equal(addAlias, wantAdd) {
				t.Fatalf("addVW n=%d y=%x z==x: got (%x, %x), want (%x, %x)", n, y, addAlias, c, wantAdd, wantAddCarry)
			}
			checkArithGuards(t, addAliasRaw)

			subAliasRaw, subAlias := guardedWords(n)
			copy(subAlias, x)
			if c := subVW(subAlias, subAlias, y); c != wantSubCarry || !slices.Equal(subAlias, wantSub) {
				t.Fatalf("subVW n=%d y=%x z==x: got (%x, %x), want (%x, %x)", n, y, subAlias, c, wantSub, wantSubCarry)
			}
			checkArithGuards(t, subAliasRaw)
		}
	}
}

func addVWReference(z, x []Word, y Word) (c Word) {
	if len(z) == 0 {
		return y
	}
	zi, carry := bits.Add(uint(x[0]), uint(y), 0)
	z[0] = Word(zi)
	for i := 1; i < len(z); i++ {
		zi, carry = bits.Add(uint(x[i]), 0, carry)
		z[i] = Word(zi)
	}
	return Word(carry)
}

func subVWReference(z, x []Word, y Word) (c Word) {
	if len(z) == 0 {
		return y
	}
	zi, borrow := bits.Sub(uint(x[0]), uint(y), 0)
	z[0] = Word(zi)
	for i := 1; i < len(z); i++ {
		zi, borrow = bits.Sub(uint(x[i]), 0, borrow)
		z[i] = Word(zi)
	}
	return Word(borrow)
}

func TestArithShiftMatchesGo(t *testing.T) {
	lengths := []int{0, 1, 2, 3, 4, 5, 7, 8, 9, 16, 17, 80}
	shifts := []uint{0, 1, uint(bits.UintSize / 2), uint(bits.UintSize - 1)}
	for _, n := range lengths {
		x, _ := arithInputs(n, int64(3000+n))
		for _, shift := range shifts {
			want := make([]Word, n)
			wantCarry := lshVUGo(want, x, shift)
			raw, got := guardedWords(n)
			if c := shlVU(got, x, shift); c != wantCarry || !slices.Equal(got, want) {
				t.Fatalf("shlVU n=%d s=%d: got (%x, %x), want (%x, %x)", n, shift, got, c, want, wantCarry)
			}
			checkArithGuards(t, raw)

			aliasRaw, alias := guardedWords(n)
			copy(alias, x)
			if c := shlVU(alias, alias, shift); c != wantCarry || !slices.Equal(alias, want) {
				t.Fatalf("shlVU n=%d s=%d z==x: got (%x, %x), want (%x, %x)", n, shift, alias, c, want, wantCarry)
			}
			checkArithGuards(t, aliasRaw)
		}
	}
}

func TestArithAddMulMatchesGo(t *testing.T) {
	lengths := []int{0, 1, 2, 3, 4, 5, 7, 8, 9, 16, 17, 32}
	multipliers := []Word{0, 1, 2, ^Word(0)}
	for _, n := range lengths {
		z, x := arithInputs(n, int64(4000+n))
		for _, multiplier := range multipliers {
			want := slices.Clone(z)
			wantCarry := addMulVVWGo(want, x, multiplier)
			raw, got := guardedWords(n)
			copy(got, z)
			xOriginal := slices.Clone(x)
			if c := addMulVVW(got, x, multiplier); c != wantCarry || !slices.Equal(got, want) {
				t.Fatalf("addMulVVW n=%d y=%x: got (%x, %x), want (%x, %x)", n, multiplier, got, c, want, wantCarry)
			}
			if !slices.Equal(x, xOriginal) {
				t.Fatalf("addMulVVW n=%d y=%x: changed x", n, multiplier)
			}
			checkArithGuards(t, raw)

			aliasWant := slices.Clone(x)
			aliasCarry := addMulVVWGo(aliasWant, slices.Clone(x), multiplier)
			aliasRaw, alias := guardedWords(n)
			copy(alias, x)
			if c := addMulVVW(alias, alias, multiplier); c != aliasCarry || !slices.Equal(alias, aliasWant) {
				t.Fatalf("addMulVVW n=%d y=%x z==x: got (%x, %x), want (%x, %x)", n, multiplier, alias, c, aliasWant, aliasCarry)
			}
			checkArithGuards(t, aliasRaw)
		}
	}
}
