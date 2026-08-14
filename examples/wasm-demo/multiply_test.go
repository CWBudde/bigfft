package main

import (
	"strings"
	"testing"
)

func TestCalculate(t *testing.T) {
	t.Parallel()

	got, err := calculate("-12345678901234567890", "+987654321")
	if err != nil {
		t.Fatalf("calculate: %v", err)
	}
	if want := "-12193263112482853211126352690"; got.product != want {
		t.Fatalf("product = %q, want %q", got.product, want)
	}
	if got.leftDigits != 20 || got.rightDigits != 9 || got.productDigits != 29 {
		t.Fatalf("digit counts = (%d, %d, %d), want (20, 9, 29)", got.leftDigits, got.rightDigits, got.productDigits)
	}
}

func TestParseDecimalRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	tests := []string{"", "-", "12.3", "1 2", strings.Repeat("1", maxInputDigits+1)}
	for _, input := range tests {
		if _, _, err := parseDecimal("operand", input); err == nil {
			t.Errorf("parseDecimal(%q) unexpectedly succeeded", input)
		}
	}
}
