//go:build arm64 && !purego

package bigfft

//go:noescape
func shiftMod(z, x *Word, n, shift uintptr)
