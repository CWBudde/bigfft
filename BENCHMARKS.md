# Benchmarks

All numbers below were measured on:

|     |                                                                         |
| --- | ----------------------------------------------------------------------- |
| CPU | 12th Gen Intel Core i7-1255U (4 P-cores @ 4.7 GHz, 8 E-cores @ 3.5 GHz) |
| OS  | Linux 6.8, `GOARCH=amd64`                                               |
| Go  | 1.26.1                                                                  |

Historical results from upstream (2012 Core 2 Quad, 2016 Core i5-4590) are preserved in
[docs/historical-benchmarks.md](docs/historical-benchmarks.md).

## How to reproduce — read this first

This machine is a hybrid P/E-core laptop, and naive benchmarking on it is not merely
imprecise, it is wrong by more than any optimization being measured. `FermatShift/n=80`
measures **1057 ns** unpinned and **66 ns** pinned to a P-core: a 16x error caused purely
by the scheduler migrating the benchmark between core types.

The protocol, in full, is documented in [PLAN.md](PLAN.md#measurement-discipline-read-this-before-touching-performance).
The short version:

```sh
# Serial measurements: pin to one P-core.
taskset -c 0 go test -run=XXX -bench=. -count=10 .

# Parallel measurements: pin to the P-cores only, and match GOMAXPROCS,
# so the number reflects parallel speedup and not P-vs-E scheduling luck.
taskset -c 0-3 env GOMAXPROCS=4 go test -run=XXX -bench=. -count=10 .
```

For A/B comparisons, build two test binaries with `go test -c` and run them
**interleaved**, alternating, at least 8-10 repetitions each — never measure "before",
change the code, then measure "after". Compare with `benchstat` and report p-values.
Treat p > 0.05 as no measured difference.

Nothing else may run on the machine during measurement. A baseline captured while two
builds were running reported `MulFFT_5Mb` at 136 ms against a true quiet value of 27 ms,
and a change that looked like a 67% improvement against it turned out to be −3% (p > 0.3).

## bigfft vs math/big

`Mul` at default settings (parallelism enabled, `GOMAXPROCS=12`), 6 repetitions.
This is the number a user sees.

| Operand size | `math/big` | `bigfft` | Speedup |
| ------------ | ---------: | -------: | ------: |
| 200 kb       |   996.1 µs | 765.4 µs |    1.3x |
| 500 kb       |   4.041 ms | 1.513 ms |    2.7x |
| 1 Mb         |   12.46 ms | 2.740 ms |    4.5x |
| 2 Mb         |   35.23 ms | 5.152 ms |    6.8x |
| 5 Mb         |   151.1 ms | 12.81 ms |   11.8x |
| 10 Mb        |   455.3 ms | 27.73 ms |   16.4x |

For scale, upstream's 2012 measurements reported −73% at 1 Mb and −89% at 10 Mb; the
corresponding figures here are −78% and −94%.

Below roughly 120 kbit `math/big` wins and `Mul` dispatches to it automatically — see
[Threshold calibration](#threshold-calibration).

## Parallel speedup

Interleaved A/B against the pre-parallelization commit, 16 repetitions, all p < 0.001.

Pinned to the four P-cores with `GOMAXPROCS=4` — the representative figure, since the
cores are homogeneous:

| Operand size |   Serial | Parallel | Change |
| ------------ | -------: | -------: | -----: |
| 200 kb       | 699.6 µs | 614.3 µs | −12.2% |
| 500 kb       | 1.567 ms | 1.215 ms | −22.5% |
| 1 Mb         | 3.498 ms | 2.444 ms | −30.1% |
| 5 Mb         | 22.11 ms | 12.58 ms | −43.1% |
| 10 Mb        | 53.86 ms | 28.01 ms | −48.0% |

Whole machine, `taskset -c 0-11` with `GOMAXPROCS=12`:

| Operand size |   Serial | Parallel | Change |
| ------------ | -------: | -------: | -----: |
| 200 kb       | 1.065 ms | 942.8 µs | −11.4% |
| 500 kb       | 2.391 ms | 1.721 ms | −28.0% |
| 1 Mb         | 5.340 ms | 2.929 ms | −45.2% |
| 5 Mb         | 28.85 ms | 12.54 ms | −56.5% |
| 10 Mb        | 68.56 ms | 25.59 ms | −62.7% |

Note that at 200 kb the four-P-core configuration (614 µs) beats the whole machine
(943 µs): once the E-cores join, the slowest shard sets the pace and small transforms do
not have enough work to hide the imbalance.

**Plausibility check.** The parallelizable fraction is about 80% (pointwise multiply ~35%,
transforms ~45%), so Amdahl's law caps speedup at ~2.5x on four cores; the measured 5 Mb
figure is 1.76x. On 4 P + 8 E cores (~7 P-equivalents) the ceiling is ~3.2x and the
measured 10 Mb figure is 2.68x. Both sit comfortably below their ceilings, which is the
check that earlier apparent wins on this project failed.

Serial throughput costs about 1–2% at 5 Mb from the added indirection. Set
`SetMaxParallelism(1)` to opt out entirely.

## Allocations

Introducing a per-`fftmul` scratch arena, `MulFFT_1Mb`:

|        |      B/op | allocs/op |
| ------ | --------: | --------: |
| before | 3,681,869 |        30 |
| after  | 2,018,977 |        12 |

Allocation counts are deterministic and unaffected by timing noise, which makes them the
one metric on this machine that can be trusted without the pinning protocol. The
wall-clock effect of this change alone was within noise: the library is arithmetic-bound,
not allocation-bound.

## Threshold calibration

Four constants decide which algorithm runs at which size, and all four were calibrated on
a Core 2 Quad around 2012, against a `math/big` whose Karatsuba and assembly kernels have
improved substantially since:

| Constant                  | Where          | Decides                                     |
| ------------------------- | -------------- | ------------------------------------------- |
| `fftThreshold`            | `fft.go`       | `math/big` vs FFT                           |
| `fftSizeThreshold`        | `fft.go`       | the FFT length `1<<k` for a given size       |
| `fermatBasicMulThreshold` | `fermat.go`    | schoolbook vs `big.Int.Mul` for coefficients |
| `quadraticScanThreshold`  | `scan.go`      | `SetString` vs recursive decimal scanning    |

`just calibrate` runs all four sweeps; `just calibrate-fft`, `just calibrate-fermat` and
`just calibrate-scan` run them individually. Run serially and pinned
(`taskset -c 0`, `GOMAXPROCS=1`) on an otherwise idle machine, so the resulting constants
stay correct for callers who disable parallelism.

Two of the sweeps deliberately publish a flat grid rather than bisecting to a single
number. Bisection is how the 2012 constants were produced, and it cannot distinguish a
crossover from an oscillation — which is exactly what re-measuring `fftThreshold` found.
The decision rule used throughout, fixed before looking at any data:

> A constant changes only if the sweep shows a monotone crossover rather than oscillation,
> the candidate beats the incumbent by ≥5% on an interleaved A/B of the *public*
> benchmarks with p < 0.05, and nothing on the other side of the crossover regresses by
> more than 2%.

**Measured and left unchanged** is a first-class outcome here, and three of the four
entries below are exactly that.

### `fftThreshold` — `math/big` vs FFT

```
120,600 bits: 1.01      133,756 bits: 0.75      166,048 bits: 1.11
126,580 bits: 0.99      150,500 bits: 0.90      180,400 bits: 1.10
128,374 bits: 1.02      155,284 bits: 0.94      210,300 bits: 1.13
```

The speedup oscillates between 0.75 and 1.11 across the crossover region with no clean
transition. The existing `fftThreshold = 1800` words (115.2 kbit) sits at the low edge of
that band, so **the constant was left unchanged**: the 2012 value is still approximately
right on modern hardware, and picking a new number from this data would be fitting noise.

One consequence worth knowing: with parallelism enabled the FFT wins earlier than 1800
words, so the default dispatch switches slightly later than optimal. A separate, lower
threshold for the parallel case is listed as future work in [PLAN.md](PLAN.md).

### `fermatBasicMulThreshold` — schoolbook vs `big.Int.Mul`

Before timing anything, `TestFermatBasicMulThresholdReachable` (always on, not gated
behind `-calibrate`) settles how much this constant can possibly matter. The coefficient
size `n` is not a free parameter: `fftSize` picks `k` from `fftSizeThreshold`, `k` and the
word count give `m`, and `n` is `valueSize(k, m, 2)`. So the set of `n` that `fermat.Mul`
ever sees is a function of the size table, and it is small:

| Word size          | Reachable configurations | On the `basicMul` side              |
| ------------------ | -----------------------: | ----------------------------------- |
| 64-bit             |                      319 | 5 (`n` = 20, 22, 24, 26, 28)        |
| 32-bit (`GOARCH=386`) |                   476 | **0**                               |

On 64-bit those five occupy a single window, **131.2 kbit to 211.4 kbit per operand** — the
bottom of the `k=9` bracket, immediately above where `fftThreshold` hands work to the FFT
at all. Above 211.4 kbit the branch is never taken again, because every higher `k` bracket
starts at `n ≥ 36`. On 32-bit the branch is unreachable through the public `Mul` entirely;
it exists there only for direct `mulFFT` / `poly.Mul` callers, which is why
`BenchmarkFermatMul` hard-codes a synthetic `n=16` case to keep that side covered on every
platform.

This is rule 5 of the measurement discipline applied before the fact rather than after:
whatever the timings say, this constant can only move an 80 kbit-wide sliver of one word
size.

## Benchmark inventory

- `fft_test.go` — `BenchmarkMulBig_*` (math/big baseline), `BenchmarkMulFFT_*` (FFT
  forced), `BenchmarkMul_*` (threshold-dispatched, i.e. what users get), plus unbalanced
  operand sizes.
- `fermat_bench_test.go` — micro-benchmarks for `Shift`, `ShiftHalf` (even and odd paths
  separately), `Add`, `Sub`, `Mul` (both sides of the `n < 30` branch), `Transform`,
  `InvTransform`, `polValues.Mul`. Sizes are derived at run time from the real
  `fftSize`/`valueSize` path rather than hard-coded, so they remain correct on 32-bit.
- `scan_test.go` — `BenchmarkScanFast*` / `BenchmarkScanBig*` for `FromDecimalString`.

`scripts/run_benchmarks.sh` captures a run; `scripts/bench_compare.sh` compares it against
a committed baseline with a `benchstat` regression gate. Note that this gate is a plain
threshold check and does **not** implement canary-gated measurement windows; on a noisy or
thermally-limited host it can still be fooled. Treat CI benchmark output as informational
and reproduce anything surprising locally under the protocol above.
