package main

import (
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/cwbudde/bigfft"
)

const maxInputDigits = 100_000

type multiplicationResult struct {
	product        string
	leftDigits     int
	rightDigits    int
	productDigits  int
	leftBits       int
	rightBits      int
	productBits    int
	parseMillis    float64
	multiplyMillis float64
	formatMillis   float64
}

func calculate(left, right string) (multiplicationResult, error) {
	parseStart := time.Now()
	x, leftDigits, err := parseDecimal("left operand", left)
	if err != nil {
		return multiplicationResult{}, err
	}
	y, rightDigits, err := parseDecimal("right operand", right)
	if err != nil {
		return multiplicationResult{}, err
	}
	parseMillis := elapsedMillis(parseStart)

	multiplyStart := time.Now()
	z := bigfft.Mul(x, y)
	multiplyMillis := elapsedMillis(multiplyStart)

	formatStart := time.Now()
	product := z.String()
	formatMillis := elapsedMillis(formatStart)

	return multiplicationResult{
		product:        product,
		leftDigits:     leftDigits,
		rightDigits:    rightDigits,
		productDigits:  countDigits(product),
		leftBits:       x.BitLen(),
		rightBits:      y.BitLen(),
		productBits:    z.BitLen(),
		parseMillis:    parseMillis,
		multiplyMillis: multiplyMillis,
		formatMillis:   formatMillis,
	}, nil
}

func parseDecimal(name, value string) (*big.Int, int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, 0, fmt.Errorf("%s is empty", name)
	}

	value, negative := splitSign(value)
	if err := validateDigits(name, value); err != nil {
		return nil, 0, err
	}

	z := bigfft.FromDecimalString(value)
	if negative && z.Sign() != 0 {
		z.Neg(z)
	}
	return z, len(value), nil
}

func splitSign(value string) (string, bool) {
	if strings.HasPrefix(value, "-") {
		return value[1:], true
	}
	return strings.TrimPrefix(value, "+"), false
}

func validateDigits(name, value string) error {
	if value == "" {
		return fmt.Errorf("%s has no digits", name)
	}
	if len(value) > maxInputDigits {
		return fmt.Errorf("%s exceeds the %d digit demo limit", name, maxInputDigits)
	}
	for i := range len(value) {
		digit := value[i]
		if digit < '0' || digit > '9' {
			return fmt.Errorf("%s must be a base-10 integer", name)
		}
	}
	return nil
}

func countDigits(value string) int {
	if strings.HasPrefix(value, "-") {
		return len(value) - 1
	}
	return len(value)
}

func elapsedMillis(start time.Time) float64 {
	return float64(time.Since(start).Microseconds()) / 1000
}
