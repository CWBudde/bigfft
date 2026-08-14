// Copyright 2010 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build (!amd64 && !arm64) || purego

package bigfft

func addVV(z, x, y []Word) Word {
	return addVVGo(z, x, y)
}

func subVV(z, x, y []Word) Word {
	return subVVGo(z, x, y)
}

func lshVU(z, x []Word, s uint) Word {
	return lshVUGo(z, x, s)
}

func addMulVVW(z, x []Word, y Word) Word {
	return addMulVVWGo(z, x, y)
}
