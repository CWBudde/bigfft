// Copyright 2010 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build amd64 && !purego

package bigfft

//go:noescape
func addVV(z, x, y []Word) (c Word)

//go:noescape
func subVV(z, x, y []Word) (c Word)

//go:noescape
func lshVU(z, x []Word, s uint) (c Word)

//go:noescape
func addMulVVW(z, x []Word, y Word) (c Word)
