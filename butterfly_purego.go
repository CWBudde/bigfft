//go:build (!amd64 && !arm64) || purego

package bigfft

func butterflyWords(sum, diff, x, y []Word) (carry, borrow Word) {
	return butterflyWordsGo(sum, diff, x, y)
}
