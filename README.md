# bigfft

[![tests](https://github.com/cwbudde/bigfft/actions/workflows/test.yaml/badge.svg)](https://github.com/cwbudde/bigfft/actions/workflows/test.yaml)
[![codecov](https://codecov.io/gh/cwbudde/bigfft/branch/master/graph/badge.svg)](https://codecov.io/gh/cwbudde/bigfft)
[![Go Reference](https://pkg.go.dev/badge/github.com/cwbudde/bigfft.svg)](https://pkg.go.dev/github.com/cwbudde/bigfft)

Fast multiplication of very large integers in Go, using the Schönhage-Strassen
algorithm.

## What this is

Schönhage-Strassen multiplies two _N_-bit integers in _O(N log N log log N)_
time by treating each operand as a polynomial, evaluating both polynomials with
a fast Fourier transform, multiplying the values pointwise, and transforming
back. The transform is carried out in the ring of integers modulo 2^n+1 (a
"Fermat ring"), where the roots of unity are powers of two — so every twiddle
multiplication is a shift, and the whole FFT runs in exact integer arithmetic
with no rounding error.

The asymptotics only pay off once the numbers are large. Go's `math/big` uses
schoolbook multiplication for small operands and Karatsuba above a threshold,
and Karatsuba's _O(N^1.585)_ stays ahead until roughly 100k-200k bits per
operand. Below that, `Mul` in this package simply forwards to
`(*big.Int).Mul`; above it, the FFT path takes over and the gap widens rapidly
with size — at tens of megabits it is an order of magnitude faster.

The same idea makes decimal parsing subquadratic: `FromDecimalString` splits
the digit string in half recursively and recombines with FFT multiplication
instead of the linear digit-by-digit accumulation `big.Int.SetString` performs.

If your numbers are smaller than about 100k bits, use `math/big` directly —
this package will not help you.

## Installation

```sh
go get github.com/cwbudde/bigfft
```

## Usage

### Multiplication

`Mul` is a drop-in replacement for `(*big.Int).Mul`, and picks the faster of the
FFT and `math/big` paths based on operand size.

```go
package main

import (
    "fmt"
    "math/big"

    "github.com/cwbudde/bigfft"
)

func main() {
    // Two large operands, e.g. 2^1000000 - 1 and 3^500000.
    x := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 1_000_000), big.NewInt(1))
    y := new(big.Int).Exp(big.NewInt(3), big.NewInt(500_000), nil)

    z := bigfft.Mul(x, y)
    fmt.Println(z.BitLen())
}
```

### Scanning decimal strings

`FromDecimalString` parses the base-10 representation of a non-negative integer
with subquadratic complexity.

```go
digits := strings.Repeat("1234567890", 1_000_000) // 10 million digits
n := bigfft.FromDecimalString(digits)
fmt.Println(n.BitLen())
```

## Performance

Benchmark results live in [BENCHMARKS.md](BENCHMARKS.md).

The original 2012/2016-era measurements from upstream (Core 2 Quad and Core
i5-4590) are preserved in
[docs/historical-benchmarks.md](docs/historical-benchmarks.md).

The FFT/Karatsuba crossover is a single tunable, `fftThreshold` in `fft.go`. To
re-measure it on your own hardware, run `just calibrate`.

## Development

The repository uses [just](https://github.com/casey/just) as its task runner.

| Recipe             | What it does                                                         |
| ------------------ | -------------------------------------------------------------------- |
| `just build`       | Build the package                                                    |
| `just test`        | Run the full suite with the race detector                            |
| `just bench`       | Run all benchmarks                                                   |
| `just bench-quick` | Run benchmarks with a short time budget                              |
| `just lint`        | Run golangci-lint                                                    |
| `just lint-fix`    | Run golangci-lint with `--fix`                                       |
| `just fmt`         | Format everything via treefmt (gofumpt, gci, markdownlint, prettier) |
| `just fmt-check`   | Fail if anything is unformatted                                      |
| `just cover`       | Produce `coverage.txt` and `coverage.html`                           |
| `just check`       | test + lint + cover                                                  |
| `just vet-arch`    | `go vet` under GOARCH amd64, 386 and arm64                           |
| `just calibrate`   | Re-measure the FFT-vs-`math/big` crossover point                     |
| `just fix`         | `lint-fix` then `fmt`                                                |
| `just clean`       | Remove coverage, benchmark and test-binary artifacts                 |

Cross-architecture coverage matters more here than in most pure-Go packages:
`arith_decl.go` reaches into `math/big`'s internal word routines with
`//go:linkname`, and the arithmetic in `fermat.go` is written in terms of the
machine word size `_W`. Both 32-bit and 64-bit words are exercised in CI.

## Relationship to upstream

This is a fork of [github.com/remyoudompheng/bigfft](https://github.com/remyoudompheng/bigfft)
by Rémy Oudompheng, who wrote the original implementation and all of the
algorithmic work. This fork modernizes the tooling (Go module path, linting,
formatting, CI matrix) and continues performance work on top of it. All credit
for the algorithm implementation belongs to the original author.

Upstream describes the library as a proof-of-concept rather than a production
component. That characterization still applies: if you are reaching for
Schönhage-Strassen, examine first whether your problem really needs
multi-megabit integer multiplication.

## License

BSD 3-Clause. See [LICENSE](LICENSE).
