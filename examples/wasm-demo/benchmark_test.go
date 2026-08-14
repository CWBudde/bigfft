package main

import "testing"

func TestBenchmarkMultiplication(t *testing.T) {
	t.Parallel()

	got, err := benchmarkMultiplication(minBenchmarkDigits, 2)
	if err != nil {
		t.Fatalf("benchmarkMultiplication: %v", err)
	}
	if got.digits != minBenchmarkDigits || got.iterations != 2 {
		t.Fatalf("benchmark configuration = (%d, %d), want (%d, 2)", got.digits, got.iterations, minBenchmarkDigits)
	}
	if !got.resultsVerified {
		t.Fatal("benchmark products differ")
	}
	if got.inputBits == 0 || got.bigfftMillis < 0 || got.standardMillis < 0 {
		t.Fatalf("invalid benchmark measurements: %+v", got)
	}
}

func TestBenchmarkMultiplicationRejectsInvalidConfiguration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		digits     int
		iterations int
	}{
		{minBenchmarkDigits - 1, 1},
		{maxBenchmarkDigits + 1, 1},
		{minBenchmarkDigits, 0},
		{minBenchmarkDigits, maxBenchmarkIterations + 1},
	}
	for _, test := range tests {
		if _, err := benchmarkMultiplication(test.digits, test.iterations); err == nil {
			t.Errorf("benchmarkMultiplication(%d, %d) unexpectedly succeeded", test.digits, test.iterations)
		}
	}
}

func TestRepeatedDigits(t *testing.T) {
	t.Parallel()

	if got, want := repeatedDigits("123", 8), "12312312"; got != want {
		t.Fatalf("repeatedDigits = %q, want %q", got, want)
	}
}
