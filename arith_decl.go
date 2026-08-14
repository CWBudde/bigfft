// Copyright 2010 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bigfft

import (
	"math/big"
	"math/bits"
)

// Word is an alias for big.Word, the unsigned machine word type used by
// math/big to store the magnitude of an integer. It is exported so callers can
// name the element type of the slices returned by (*big.Int).Bits.
type Word = big.Word

// The word-vector helpers below are adapted from Go's math/big package. The
// architecture-specific versions of addVV, subVV, lshVU, and addMulVVW live
// in arith_$GOARCH.s; purego and other architectures use arith_generic.go.

func addVVGo(z, x, y []Word) (c Word) {
	if len(x) != len(z) || len(y) != len(z) {
		panic("addVV len")
	}

	for i := range z {
		zi, cc := bits.Add(uint(x[i]), uint(y[i]), uint(c))
		z[i] = Word(zi)
		c = Word(cc)
	}
	return c
}

func subVVGo(z, x, y []Word) (c Word) {
	if len(x) != len(z) || len(y) != len(z) {
		panic("subVV len")
	}

	for i := range z {
		zi, cc := bits.Sub(uint(x[i]), uint(y[i]), uint(c))
		z[i] = Word(zi)
		c = Word(cc)
	}
	return c
}

// addVW sets z = x + y, returning the final carry. If z is empty, the
// unconsumed word y is the carry.
func addVW(z, x []Word, y Word) (c Word) {
	if len(x) != len(z) {
		panic("addVW len")
	}
	if len(z) == 0 {
		return y
	}

	zi, cc := bits.Add(uint(x[0]), uint(y), 0)
	z[0] = Word(zi)
	if cc == 0 {
		if &z[0] != &x[0] {
			copy(z[1:], x[1:])
		}
		return 0
	}
	for i := 1; i < len(z); i++ {
		xi := x[i]
		if xi != ^Word(0) {
			z[i] = xi + 1
			if &z[0] != &x[0] {
				copy(z[i+1:], x[i+1:])
			}
			return 0
		}
		z[i] = 0
	}
	return 1
}

// subVW sets z = x - y, returning the final borrow. If z is empty, the
// unconsumed word y is the borrow.
func subVW(z, x []Word, y Word) (c Word) {
	if len(x) != len(z) {
		panic("subVW len")
	}
	if len(z) == 0 {
		return y
	}

	zi, cc := bits.Sub(uint(x[0]), uint(y), 0)
	z[0] = Word(zi)
	if cc == 0 {
		if &z[0] != &x[0] {
			copy(z[1:], x[1:])
		}
		return 0
	}
	for i := 1; i < len(z); i++ {
		xi := x[i]
		if xi != 0 {
			z[i] = xi - 1
			if &z[0] != &x[0] {
				copy(z[i+1:], x[i+1:])
			}
			return 0
		}
		z[i] = ^Word(0)
	}
	return 1
}

// shlVU sets z = x << s and returns the bits shifted out of the high word.
// A zero shift is a copy; lshVU's assembly contract is 1 <= s < _W.
func shlVU(z, x []Word, s uint) (c Word) {
	if len(x) != len(z) {
		panic("shlVU len")
	}
	if s == 0 {
		copy(z, x)
		return 0
	}
	return lshVU(z, x, s)
}

func lshVUGo(z, x []Word, s uint) (c Word) {
	if len(x) != len(z) {
		panic("lshVU len")
	}
	if s == 0 {
		copy(z, x)
		return 0
	}
	if len(z) == 0 {
		return 0
	}

	s &= bits.UintSize - 1
	reverse := uint(bits.UintSize) - s
	reverse &= bits.UintSize - 1
	c = x[len(z)-1] >> reverse
	for i := len(z) - 1; i > 0; i-- {
		z[i] = x[i]<<s | x[i-1]>>reverse
	}
	z[0] = x[0] << s
	return c
}

func addMulVVWGo(z, x []Word, y Word) (c Word) {
	if len(x) != len(z) {
		panic("addMulVVW len")
	}

	for i := range z {
		hi, lo := bits.Mul(uint(x[i]), uint(y))
		lo, cc := bits.Add(lo, uint(z[i]), 0)
		hi += cc
		lo, cc3 := bits.Add(lo, uint(c), 0)
		hi += cc3
		z[i] = Word(lo)
		c = Word(hi)
	}
	return c
}
