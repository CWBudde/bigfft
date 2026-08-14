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

**Stale row.** The 200 kb figures predate the `fftSizeThreshold[8]` change below, which is
worth −11% to −14% at that size, so the 1.3x understates current performance. The row was
deliberately **not** patched in place: a spot re-measurement put `MulBig_200kb` — code the
change does not touch — at 756 µs against the 996 µs recorded here, which shows the machine
is in a different state than when this table was captured. Splicing one fresh number into
it would produce a table whose rows are not mutually comparable, which is the
before/after error the protocol forbids. The whole table needs regenerating in one quiet
session; until then, the interleaved A/B under
[`fftSizeThreshold`](#fftsizethreshold--the-fft-length-for-a-given-size) is the sound
measurement of that change.

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

## Plan 9 assembly

The default amd64 and arm64 builds now own the word-vector primitives previously reached
through six unsupported `math/big` linknames. `addVV`, `subVV`, `lshVU`, and `addMulVVW`
are local Plan 9 assembly; `addVW` and `subVW` are local Go. `-tags purego` selects Go
oracles for all six and is built, vetted, and tested on every CI architecture.

Same-binary microbenchmarks against those Go oracles show why the assembly implementations
are retained, independent of the compatibility win. At `n=80`, approximate medians:

| Primitive   |      Go | Assembly | Change |
| ----------- | ------: | -------: | -----: |
| `addVV`     | 62.7 ns |  24.7 ns |   -61% |
| `subVV`     | 61.9 ns |  24.6 ns |   -60% |
| `lshVU`     | 43.7 ns |  25.4 ns |   -42% |
| `addMulVVW` | 76.4 ns |  61.6 ns |   -19% |

The old default already reached `math/big` assembly, so these are fallback comparisons,
not the end-to-end A/B. The user-visible performance change comes from the fused Add/Sub
tail of each butterfly. After `ShiftHalf` produces the twiddle product, this tail computes
sum and difference in one payload pass (two-word ADC/SBB blocks on amd64, four-word
ADCS/SBCS blocks on arm64).

Interleaved old/default A/B, ten repetitions, `taskset -c 0`, `GOMAXPROCS=1`:

| Benchmark             |   Before |    After |      Change / p |
| --------------------- | -------: | -------: | --------------: |
| `Transform/n=27/k=8`  | 75.84 µs | 71.29 µs | -6.00%, p=0.002 |
| `Transform/n=64/k=10` | 617.9 µs | 615.8 µs |      ~, p=0.971 |
| `Transform/n=80/k=12` | 4.087 ms | 4.060 ms |      ~, p=0.971 |
| `MulFFT_1Mb`          | 3.343 ms | 3.317 ms |      ~, p=0.853 |
| `MulFFT_5Mb`          | 21.80 ms | 21.39 ms |      ~, p=0.684 |
| `MulFFT_10Mb`         | 52.66 ms | 51.84 ms |      ~, p=0.529 |

Geomean -1.88%; bytes and allocations are unchanged. The correct claim is therefore one
small-transform win and no measured difference on the large public workloads, alongside
removal of the linkname compatibility hazard.

The attempted amd64 shift-mod dispatch was not retained. A specialized aligned kernel won
its positive micro-path by about 27% at `n=80`, but the final call-site A/B was flat:
geomean -0.43%, every transform and `MulFFT` comparison p=0.06-0.97. The complete arm64
shift kernel remains differential-tested and has a same-binary benchmark ready for native
hardware; QEMU timings are deliberately not reported.

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

## Balanced splitting in decimal scanning

`FromDecimalString` splits its input, converts each half, and reassembles them with a
multiplication by a power of ten. The old code split off `quadraticScanThreshold << (pow-1)`
digits, the largest cached power of ten that fits — so the split ratio was set by
`frac(log2(size/threshold))` and swung between roughly balanced and maximally lopsided
depending on the input length. `scan.go` now splits exactly in half and builds a power
table sized for that specific input, one entry per recursion depth, each the square of the
next (times ten when the length is odd).

Interleaved A/B, `taskset -c 0` with `GOMAXPROCS=1`, ten repetitions each:

| Digits | 1k     | 10k     | 100k   | 1M     | 2M     | 5M    | 10M   |
| ------ | ------ | ------- | ------ | ------ | ------ | ----- | ----- |
| Δ      | ~      | -12.24% | -6.48% | -8.78% | -6.92% | ~     | ~     |
| p      | (0.85) | 0.000   | 0.000  | 0.000  | 0.000  | 0.165 | 0.165 |

And on four P-cores (`taskset -c 0-3`, `GOMAXPROCS=4`), eight repetitions:

| Digits | 10k     | 100k   | 1M      | 10M   |
| ------ | ------- | ------ | ------- | ----- |
| Δ      | -12.51% | -9.54% | -10.43% | ~     |
| p      | 0.000   | 0.000  | 0.007   | 0.878 |

Geomean -4.0% serial, -8.3% parallel.

**The 5M–10M columns are the honest caveat.** They are flat here, but a tighter run
(twelve repetitions, ±1–2% instead of ±4–7%) measured **+1.8% and +3.7%** against the old
code. The cause is worth recording because it is not about scanning at all:

`valueSize` rounds a coefficient up to a multiple of `1 << (k-2)` bits, so `Mul` cost is a
**step function of operand size with plateaus up to 25% wide**. Measured at k=12:

```
m=56..62  n=128   33-37 ms      <- 3% more data...
m=64      n=144   43 ms         <- ...costs 30% more time
m=72..78  n=160   48-52 ms
```

The old chunk length of `1232 << k` digits is about `2^(12+k)` bits, because 1232 digits is
4092.8 bits — just under 4096. Every chunk therefore landed near the _top_ of a plateau by
construction. Exact halving lands wherever the input happens to fall.

Shading the split to recover that alignment does not work, and `TestCalibrateScanSplit`
keeps the evidence: sweeping the top chunk from 90% to 100% of half, timings are flat from
97% upward and degrade sharply below, because the imbalance compounds down the recursion
faster than the alignment pays.

```
10M digits:  split 1000/1000  986 ms      975/1000  1002 ms
             split  950/1000  1177 ms      900/1000  1845 ms
```

A cost model over the recursion tree agreed that near-exact halving is optimal, but
predicted only a 1.7% gain from perfect alignment where the direct measurement shows 30%
per node — the model treats cost as proportional to `n`, and the real function punishes an
unlucky `n` far harder. That gap is itself the finding: **the packing waste in `valueSize`
is worth up to 25% to every caller of `Mul`, not just to scanning**, and is listed as new
work in [PLAN.md](PLAN.md).

Two incidental results: the base of the power table is now built by binary exponentiation
rather than `(10^14)^(threshold/14)`, so `quadraticScanThreshold` is no longer required to
be a multiple of 14 — any value is legal. And inputs at or below the threshold skip the
scanner entirely, which removed a 3.4% regression at 1,000 digits that the first version
introduced.

## Threshold calibration

Four constants decide which algorithm runs at which size, and all four were calibrated on
a Core 2 Quad around 2012, against a `math/big` whose Karatsuba and assembly kernels have
improved substantially since:

| Constant                  | Where       | Decides                                      |
| ------------------------- | ----------- | -------------------------------------------- |
| `fftThreshold`            | `fft.go`    | `math/big` vs FFT                            |
| `fftSizeThreshold`        | `fft.go`    | the FFT length `1<<k` for a given size       |
| `fermatBasicMulThreshold` | `fermat.go` | schoolbook vs `big.Int.Mul` for coefficients |
| `quadraticScanThreshold`  | `scan.go`   | `SetString` vs recursive decimal scanning    |

A fifth, `parallelWordThreshold` (`parallel.go`), decides serial vs parallel transforms. It
is not a 2012 constant and is measured by benchmark rather than by a `-calibrate` sweep, but
it is recorded in the same section below because it interacts with `fftThreshold`.

`just calibrate` runs all four sweeps; `just calibrate-fft`, `just calibrate-fermat` and
`just calibrate-scan` run them individually. Run serially and pinned
(`taskset -c 0`, `GOMAXPROCS=1`) on an otherwise idle machine, so the resulting constants
stay correct for callers who disable parallelism.

Two of the sweeps deliberately publish a flat grid rather than bisecting to a single
number. Bisection is how the 2012 constants were produced, and it cannot distinguish a
crossover from an oscillation — which is exactly what re-measuring `fftThreshold` found.
The decision rule used throughout, fixed before looking at any data:

> A constant changes only if the sweep shows a monotone crossover rather than oscillation,
> the candidate beats the incumbent by ≥5% on an interleaved A/B of the _public_
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

This entry used to claim that with parallelism enabled the FFT wins earlier than 1800
words, and that a separate lower threshold for the parallel case was therefore worth
having. **That was wrong**, and the correction is in
[§ `parallelWordThreshold`](#parallelwordthreshold--serial-vs-parallel-transforms) below:
parallelism does not engage at all until a transform array reaches 8192 words, which
operands only do at 1792 words each — eight words below `fftThreshold` itself. There is no
band to pick up.

### `fftSizeThreshold` — the FFT length for a given size

`TestCalibrateFFTTable` sweeps a flat grid around every boundary, comparing FFT length
`1<<k` against `1<<(k+1)` at the same input size. A ratio above 1.0 means `k+1` is the
better choice there, so a well-placed boundary shows the ratios crossing 1.0 near factor
1.0 and staying above it.

The result splits into two regimes. Entries 3 to 8 are monotone and all say the same
thing — `k+1` never wins anywhere in its own bracket, reaching parity only at twice the
incumbent boundary. Entry 8, the lowest one the public `Mul` can select:

```
--- k=8 vs k=9, incumbent boundary 262144 bits total = 2048 words/operand
  f=0.50  w=1024   k=8 225µs   k=9 351µs   ratio 0.642
  f=0.70  w=1433   k=8 272µs   k=9 416µs   ratio 0.655
  f=0.85  w=1740   k=8 320µs   k=9 399µs   ratio 0.801
  f=1.00  w=2048   k=8 384µs   k=9 484µs   ratio 0.793   <- incumbent boundary
  f=1.20  w=2457   k=8 471µs   k=9 560µs   ratio 0.840
  f=1.50  w=3072   k=8 562µs   k=9 628µs   ratio 0.895
  f=2.00  w=4096   k=8 803µs   k=9 804µs   ratio 0.998
```

Entries 9 to 13 are the opposite — non-monotone, oscillating across 1.0. `k=9` vs `k=10`
gives 0.745, 0.753, **1.019**, 0.941, 0.900, **1.027**, 1.126: the `fftThreshold`
signature again, and not actionable. `k=12` vs `k=13` leans the other way (`k=13` wins
from `f=0.5` up, hinting that boundary is too high) but is likewise non-monotone. Entries
14 and 15 were **not measured**: their brackets need operands up to 629 Mbit, past the
memory available on this machine. They are recorded as a gap rather than extrapolated.

**Entry 8 was changed, `1<<18` → `1<<19`.** Interleaved A/B, 10 repetitions each,
`BenchmarkMul_*` (the threshold-dispatched path users actually get):

| Benchmark   | serial, `taskset -c 0` |            | 4 P-cores, `GOMAXPROCS=4` |            |
| ----------- | ---------------------: | ---------: | ------------------------: | ---------: |
| `Mul_100kb` |    239.2 µs → 238.2 µs | ~ (p=0.97) |       244.8 µs → 247.2 µs | ~ (p=0.53) |
| `Mul_150kb` |    564.6 µs → 442.0 µs | **−21.7%** |       523.1 µs → 396.9 µs | **−24.1%** |
| `Mul_200kb` |    634.9 µs → 564.9 µs | **−11.0%** |       559.8 µs → 482.6 µs | **−13.8%** |
| `Mul_250kb` |    805.5 µs → 740.5 µs |  **−8.1%** |       657.2 µs → 575.6 µs | **−12.4%** |
| `Mul_500kb` |    1.454 ms → 1.453 ms | ~ (p=0.53) |       1.039 ms → 1.019 ms | ~ (p=0.12) |
| `Mul_1Mb`   |    3.074 ms → 3.055 ms | ~ (p=0.25) |       2.115 ms → 2.102 ms | ~ (p=0.68) |

All changes p < 0.001. `Mul_100kb` is the control: it is below `fftThreshold`, never
enters `fftmul`, and does not move. The far side does not regress. The gradient across
the window — largest just above the old boundary, decaying as `k=9` becomes competitive —
is the shape the sweep predicted, measured through an independent code path.

Entries 3 to 7 show the identical shape but their whole range lies below `fftThreshold`
(32.8 kbit per operand at most, against `fftThreshold`'s 115.2 kbit), so the public `Mul`
can never select them and no end-to-end benchmark can confirm a change there. **They were
left unchanged**: the data is real, but a constant that only internal callers reach,
adjusted on evidence that cannot be validated through the public API, is how the 2012
numbers earned their reputation.

The physical reading is the premise of this whole exercise confirmed: for small operands
fewer, larger coefficients now beat more, smaller ones, because `big.Int.Mul`'s Karatsuba
and assembly kernels have improved far more since 2012 than the butterfly loops have.

### `fermatBasicMulThreshold` — schoolbook vs `big.Int.Mul`

Before timing anything, `TestFermatBasicMulThresholdReachable` (always on, not gated
behind `-calibrate`) settles how much this constant can possibly matter. The coefficient
size `n` is not a free parameter: `fftSize` picks `k` from `fftSizeThreshold`, `k` and the
word count give `m`, and `n` is `valueSize(k, m, 2)`. So the set of `n` that `fermat.Mul`
ever sees is a function of the size table, and it is small:

| Word size             | Reachable configurations | On the `basicMul` side |
| --------------------- | -----------------------: | ---------------------- |
| 64-bit, before        |                      319 | 5 (`n` = 20…28)        |
| 64-bit, after         |                      327 | **0**                  |
| 32-bit (`GOARCH=386`) |                      492 | **0**                  |

Before the `fftSizeThreshold` change, five configurations took the branch on 64-bit, in a
single window from 131.2 to 211.4 kbit per operand — the bottom of the `k=9` bracket.
Raising entry 8 moved exactly that window to `k=8`, where `n ≥ 31`. **The basicMul branch
is now unreachable through the public `Mul` on both word sizes.** It survives only for
direct `mulFFT` / `poly.Mul` callers, which is why `BenchmarkFermatMul` hard-codes a
synthetic `n=16` case to keep that side covered everywhere.

The timings say the same thing independently. `TestCalibrateFermatMul`, `n` = 8…96:

```
n=27  basicMul 374ns  bigint 369ns  ratio 0.987
n=28  basicMul 405ns  bigint 399ns  ratio 0.985
n=29  basicMul 443ns  bigint 439ns  ratio 0.991
n=30  basicMul 477ns  bigint 476ns  ratio 0.998   <- incumbent cutoff
n=33  basicMul 496ns  bigint 493ns  ratio 0.994
n=36  basicMul 611ns  bigint 626ns  ratio 1.025
```

From `n=14` to `n=48` the ratio sits at 1.00 ± 3% with no crossover anywhere; the scattered
1.18–1.25 readings are single lucky minima, not a trend. `basicMul` has a real edge only at
`n` = 8–10 (~1.1–1.2x), and `big.Int.Mul` pulls ahead from about `n=60` (0.72–0.90 at
`n` = 68…96). Schoolbook and Karatsuba-plus-assembly have simply converged over the range
where the cutoff sits.

**Left unchanged at 30**, doubly determined: there is no crossing to find near 30, and the
branch has no reachable decision to make on the public path regardless.

### `quadraticScanThreshold` — decimal scanning

`TestCalibrateScan` sweeps twelve candidate thresholds across seven input sizes, and value-checks every candidate against
`big.Int.SetString` so a wrong threshold fails as a wrong answer rather than a fast one.

First, the prior question — whether `FromDecimalString` is worth using at all. Speedup over
`big.Int.SetString` at the incumbent threshold:

| Digits | 1k   | 3k   | 10k  | 30k  | 100k | 300k | 1M    | 3M    |
| ------ | ---- | ---- | ---- | ---- | ---- | ---- | ----- | ----- |
| ×      | 0.93 | 0.87 | 1.18 | 1.94 | 3.18 | 7.15 | 15.82 | 30.61 |

The crossover is near 5,000 digits and the win grows without bound after it. The two
sub-crossover figures are a measurement artifact, not a regression: they compare against a
`SetString` loop that reuses its destination, while `FromDecimalString` must allocate a
fresh `*big.Int` to return. Like for like at 1,000 digits — `new(big.Int).SetString`
4.343 µs against `FromDecimalString` 4.231 µs — it is 2.6% _faster_. Below the threshold it
degenerates to `SetString` with no measurable penalty.

Second, the threshold itself. The spread across candidates is **not noise**: the timings
cluster into tight families of thresholds related by exact powers of two.

```
1,000,000 digits                    3,000,000 digits
 280  49.05    560  48.82            280  234ms   560  231ms
1120  48.59   2240  49.13   family A 1120  235ms  2240  234ms
4480  49.27                          4480  237ms
---------------------------         ---------------------------
 840  59.77   1680  59.44   family B  840  204ms  1680  203ms
3360  59.22   6720  60.22            3360  205ms  6720  206ms
---------------------------         ---------------------------
1400  53.07   2800  53.04   family C 1400  279ms  2800  279ms
```

Within a family the timings agree to within 1%; between families they differ by up to 22%
— and the ranking **inverts** between the two input sizes (A best at 1M, B best at 3M).

The cause is structural, and this sweep is what prompted the balanced-split rewrite above.
At the time, `chunkSize` split at `quadraticScanThreshold << (pow-1)`, so only
`frac(log2(size/threshold))` affects the recursion tree, which is why thresholds a power of
two apart are indistinguishable. When that fraction is near 0.8 the split is close to
balanced (0.43 / 0.57) and the recursive `Mul` is cheapest; as it approaches 0 the split
degenerates towards maximally unbalanced. Since the fraction depends on the _input_ size,
no fixed threshold can be right for every input.

**Left unchanged at 1232.** There is no crossover to fit — only a quantization artifact —
and 1232 is at or near the best candidate in every column where the function is worth using
at all (best at 30k and 300k digits, tied at 1M, within 6% of best at 100k). Picking the
winner at 3M digits would be fitting one input size. The real fix was balanced splitting,
independent of this constant — see § Balanced splitting in decimal scanning above, which
supersedes the split behaviour described here. The threshold itself was re-checked after
that rewrite and remains at 1232.

### `parallelWordThreshold` — serial vs parallel transforms

A fifth constant, added with the parallel path rather than inherited from 2012, and
measured differently from the four above: `just calibrate-parallel` runs
`BenchmarkMulFFTParallelSweep`, which times both modes in one binary with the gate under
measurement disabled on the parallel side, pinned to four P-cores
(`taskset -c 0-3`, `GOMAXPROCS=4`) rather than to one.

> **Provisional.** Both tables below were taken with the machine under a load average of
> 2.3–3.6 (browser, editor, desktop session), not idle, at the user's direction. Dispersion
> came out at ±0–2% with every p at 0.000, and the curves are monotone, so the shape is not
> in doubt — but rule 2 of PLAN.md § Measurement discipline is not satisfied and a
> confirming run on a quiet machine is still owed.

First, the gate's own crossover — parallel vs serial transforms, 30 interleaved
repetitions:

| operand  | transform array | parallel vs serial |
| -------- | --------------: | -----------------: |
| 50 kbit  |          4096 w |  **+9.13%** slower |
| 75 kbit  |          5632 w |  **+3.14%** slower |
| 100 kbit |          7168 w |  **−2.05%** faster |
| 125 kbit |          8704 w |             −5.59% |
| 150 kbit |         10240 w |            −12.03% |
| 200 kbit |         13312 w |            −16.05% |

Two corrections to what this file and `parallel.go` previously recorded. The transform-array
sizes for 150 and 200 kbit were **stale**: they were measured before `fftSizeThreshold[8]`
went from `1<<18` to `1<<19`, which moved 150 kbit operands from `k=9` to `k=8` and with them
from 11776 to 10240 words. And the original run put 100 kbit at `p=0.70`, "no difference";
with 30 repetitions it is a small but unambiguous **−2.05%**. The crossover is therefore
between 5632 and 7168 words, not between 7168 and 11776.

That would argue for lowering the gate from 8192 to 7168 — except that it would change
nothing anyone can observe. A transform array of 8192 words corresponds to 1792 words per
operand, and `fftThreshold` is 1800: the entire band the change would unlock lies _below_
the threshold at which `Mul` enters the FFT at all, and is reachable only by calling
`mulFFT` directly. **Left unchanged at 8192**, which the second table shows is where it
belongs anyway.

Second, the comparison that decides whether the band is worth unlocking — `math/big`
Karatsuba against both FFT modes, from `BenchmarkMulDispatchCrossover`, 30 repetitions:

| operand      | `big`   | serial FFT |    parallel FFT |
| ------------ | ------- | ---------: | --------------: |
| 90 kbit      | 262.8µs |     +3.64% |          +7.38% |
| 100 kbit     | 254.1µs |    +26.11% |         +26.47% |
| 105 kbit     | 296.3µs |     +7.89% |          +8.28% |
| 110 kbit     | 322.4µs |     +6.45% |          +3.66% |
| **115 kbit** | 349.7µs |     +8.46% | **~ (p=0.051)** |
| 120 kbit     | 388.8µs |     −2.99% |          −8.29% |
| 130 kbit     | 387.3µs |     −1.48% |          −6.11% |
| 150 kbit     | 581.2µs |    −15.05% |         −26.01% |

**The parallel FFT breaks even against Karatsuba at 115 kbit, and `fftThreshold = 1800`
words is 115.2 kbit.** Parallelism moves the dispatch crossover by roughly 3 kbit — about
60 words, under 3% — where PLAN.md item 1 assumed it moved it enough to be worth a second
threshold. It does not. The item is closed; see PLAN.md § Tried and rejected.

Two reading notes. The `big` column is not monotone: 100 kbit (254.1µs) beats 90 kbit
(262.8µs), because Karatsuba has size steps of its own, so the +26% in that row is a
Karatsuba sweet spot rather than an FFT weakness — which is why the crossover has to be read
off the trend and not off any single row. And the `big` column's dispersion (±2–8%) is
several times the FFT columns' (±0–2%), the load showing up where the code is least
regular.

## Benchmark inventory

- `fft_test.go` — `BenchmarkMulBig_*` (math/big baseline), `BenchmarkMulFFT_*` (FFT
  forced), `BenchmarkMul_*` (threshold-dispatched, i.e. what users get), plus unbalanced
  operand sizes. `Mul_150kb` / `Mul_200kb` / `Mul_250kb` bracket the `fftSizeThreshold[8]`
  boundary.
- `fermat_bench_test.go` — micro-benchmarks for `Shift`, `ShiftHalf` (even and odd paths
  separately), `Add`, `Sub`, `Mul` (both sides of the `fermatBasicMulThreshold` branch),
  `Transform`, `InvTransform`, `polValues.Mul`. Sizes are derived at run time from the real
  `fftSize`/`valueSize` path rather than hard-coded, so they remain correct on 32-bit.
- `scan_test.go` — `BenchmarkScanFast*` / `BenchmarkScanBig*` for `FromDecimalString`, plus
  `TestScanPowerTable` and `TestScanThresholds`. Those two are always-on guards on the
  balanced-split power table: the table is built by repeated squaring with a correction for
  odd lengths, so one wrong correction would produce a wrong number at one input size only.
  `TestScanThresholds` also exercises thresholds that are not multiples of 14, which the
  old power-of-ten base made illegal. Both were verified to fail when the correction is
  removed.
- `calibrate_test.go` — the `-calibrate`-gated sweeps: `TestCalibrateThreshold` and
  `TestCalibrateFFT` (the original bisecting harnesses), plus `TestCalibrateFFTTable`,
  `TestCalibrateFermatMul`, `TestCalibrateScan` and `TestCalibrateScanSplit` (flat grids).
  The flat grids report
  minima of three runs rather than means, and label every `fermat.Mul` row reachable or
  informational so unreachable configurations cannot drive a constant.
- `threshold_test.go` — always-on invariants, not gated behind `-calibrate`:
  `TestFFTSizeThresholdMonotone` (a non-monotone table would silently select the wrong `k`)
  and `TestFermatBasicMulThresholdReachable`, which enumerates every `(k, m, n)` the public
  `Mul` can reach and reports which side of the cutoff each falls on, and
  `TestParallelDispatchOverlap`, which finds the exact operand size at which parallelism
  engages and compares it against `fftThreshold`. All three have been verified to fail when
  the invariant is deliberately broken.
- `fft_parallel_test.go` — `BenchmarkMulFFTParallelSweep` (serial vs parallel transforms at
  each size, both modes in one binary, gate disabled on the parallel side) and
  `BenchmarkMulDispatchCrossover` (`math/big` against both FFT modes across `fftThreshold`).
  The first calibrates `parallelWordThreshold`; only the second can answer whether `Mul`
  should have chosen the FFT at all. Run both pinned to four P-cores, not one, and pivot
  with `benchstat -col /mode`.

`scripts/run_benchmarks.sh` captures a run; `scripts/bench_compare.sh` compares it against
a committed baseline with a `benchstat` regression gate. Note that this gate is a plain
threshold check and does **not** implement canary-gated measurement windows; on a noisy or
thermally-limited host it can still be fooled. Treat CI benchmark output as informational
and reproduce anything surprising locally under the protocol above.
