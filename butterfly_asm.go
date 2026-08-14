//go:build (amd64 || arm64) && !purego

package bigfft

//go:noescape
func butterflyWords(sum, diff, x, y []Word) (carry, borrow Word)
