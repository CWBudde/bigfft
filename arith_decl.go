// Copyright 2010 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bigfft

import (
	"math/big"
	_ "unsafe"
)

// Word is an alias for big.Word, the unsigned machine word type used by
// math/big to store the magnitude of an integer. It is exported so callers can
// name the element type of the slices returned by (*big.Int).Bits.
type Word = big.Word

//go:linkname addVV math/big.addVV
func addVV(z, x, y []Word) (c Word)

//go:linkname subVV math/big.subVV
func subVV(z, x, y []Word) (c Word)

//go:linkname addVW math/big.addVW
func addVW(z, x []Word, y Word) (c Word)

//go:linkname subVW math/big.subVW
func subVW(z, x []Word, y Word) (c Word)

//go:linkname shlVU math/big.shlVU
func shlVU(z, x []Word, s uint) (c Word)

//go:linkname addMulVVW math/big.addMulVVW
func addMulVVW(z, x []Word, y Word) (c Word)
