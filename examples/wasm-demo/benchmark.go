package main

import (
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/cwbudde/bigfft"
)

const (
	minBenchmarkDigits     = 1_000
	maxBenchmarkDigits     = 1_000_000
	maxBenchmarkIterations = 10
)

const (
	benchmarkLeftPattern  = "314159265358979323846264338327950288419716939937510"
	benchmarkRightPattern = "271828182845904523536028747135266249775724709369995"
)

type benchmarkResult struct {
	digits          int
	iterations      int
	inputBits       int
	bigfftMillis    float64
	standardMillis  float64
	speedup         float64
	resultsVerified bool
}

func benchmarkMultiplication(digits, iterations int) (benchmarkResult, error) {
	if err := validateBenchmarkConfiguration(digits, iterations); err != nil {
		return benchmarkResult{}, err
	}

	x := bigfft.FromDecimalString(repeatedDigits(benchmarkLeftPattern, digits))
	y := bigfft.FromDecimalString(repeatedDigits(benchmarkRightPattern, digits))

	// Warm both paths once. Parsing and decimal formatting deliberately stay
	// outside the timed region; this compares multiplication only.
	bigfftWarm := bigfft.Mul(x, y)
	standardWarm := new(big.Int).Mul(x, y)
	if bigfftWarm.Cmp(standardWarm) != 0 {
		return benchmarkResult{}, errors.New("warm-up products differ")
	}

	var bigfftElapsed, standardElapsed time.Duration
	var bigfftProduct, standardProduct *big.Int
	for iteration := range iterations {
		// Alternate the order to reduce systematic bias from whichever path
		// happens to run first in a browser event-loop turn.
		if iteration%2 == 0 {
			bigfftElapsed, bigfftProduct = timeProduct(bigfftElapsed, func() *big.Int {
				return bigfft.Mul(x, y)
			})
			standardElapsed, standardProduct = timeProduct(standardElapsed, func() *big.Int {
				return new(big.Int).Mul(x, y)
			})
			continue
		}

		standardElapsed, standardProduct = timeProduct(standardElapsed, func() *big.Int {
			return new(big.Int).Mul(x, y)
		})
		bigfftElapsed, bigfftProduct = timeProduct(bigfftElapsed, func() *big.Int {
			return bigfft.Mul(x, y)
		})
	}

	verified := bigfftProduct.Cmp(standardProduct) == 0
	bigfftMillis := durationMillis(bigfftElapsed) / float64(iterations)
	standardMillis := durationMillis(standardElapsed) / float64(iterations)
	speedup := 0.0
	if bigfftMillis > 0 {
		speedup = standardMillis / bigfftMillis
	}

	return benchmarkResult{
		digits:          digits,
		iterations:      iterations,
		inputBits:       x.BitLen(),
		bigfftMillis:    bigfftMillis,
		standardMillis:  standardMillis,
		speedup:         speedup,
		resultsVerified: verified,
	}, nil
}

func validateBenchmarkConfiguration(digits, iterations int) error {
	if digits < minBenchmarkDigits || digits > maxBenchmarkDigits {
		return fmt.Errorf(
			"benchmark size must be between %d and %d digits",
			minBenchmarkDigits,
			maxBenchmarkDigits,
		)
	}
	if iterations < 1 || iterations > maxBenchmarkIterations {
		return fmt.Errorf(
			"benchmark iterations must be between 1 and %d",
			maxBenchmarkIterations,
		)
	}
	return nil
}

func repeatedDigits(pattern string, digits int) string {
	repeats := digits/len(pattern) + 1
	return strings.Repeat(pattern, repeats)[:digits]
}

func timeProduct(elapsed time.Duration, multiply func() *big.Int) (time.Duration, *big.Int) {
	start := time.Now()
	product := multiply()
	return elapsed + time.Since(start), product
}

func durationMillis(duration time.Duration) float64 {
	return float64(duration) / float64(time.Millisecond)
}
