# PLAN

Working notes for this fork: what has been done, what was tried and rejected, and what
is worth doing next. Upstream is
[remyoudompheng/bigfft](https://github.com/remyoudompheng/bigfft); this fork adds modern
tooling and pursues performance work the original explicitly left as a proof of concept.

## Measurement discipline (read this before touching performance)

Nearly every wrong conclusion on this project so far came from bad measurement, not bad
code. The rules below are not optional.

1. **Pin to a P-core.** The development machine (12th Gen i7-1255U) has four P-cores
   (CPU 0-3, 4.7 GHz) and eight E-cores (CPU 4-11, 3.5 GHz). Unpinned, the Go scheduler
   migrates benchmarks between core types: `FermatShift/n=80` measured **1057 ns**
   unpinned and **66 ns** under `taskset -c 0`. That is a 16x measurement error, larger
   than any optimization discussed here.
   - Serial benchmarks: `taskset -c 0`.
   - Parallel benchmarks: `taskset -c 0-3` with `GOMAXPROCS=4`, so the number reflects
     parallel speedup rather than luck in P-core assignment.
2. **Never benchmark while anything else runs.** A baseline captured while two agents
   were compiling reported `MulFFT_5Mb` at 136 ms; the true quiet value was 27 ms. A
   later "-67% improvement" measured against that baseline evaporated to -3% (p>0.3)
   when re-measured properly.
3. **Interleave A/B runs.** Build two test binaries with `go test -c` and alternate them,
   at least 8-10 repetitions each. Do not measure "before", change the code, then measure
   "after" — thermal drift alone will invent a result.
4. **Use benchstat and report p-values.** Treat p > 0.05 as "no measured difference" and
   say so plainly.
5. **Check that the code you optimized actually runs.** Before optimizing a function,
   count how often it is called on the target workload. See the `ShiftHalf` entry under
   "Tried and rejected".
6. **Sanity-check large wins before believing them.** Apply Amdahl's law against the
   known profile split. If a change to something that is 5% of runtime appears to give a
   40% speedup, the measurement is wrong.

Profile of `Mul` at 5 Mb operands (single core, pinned), for reference:

| Component | Share |
| --- | ---: |
| `fourier` transforms | ~45% |
| — of which `fermat.Shift` | 22% |
| — of which `fermat.Add` | 14.5% |
| — of which `fermat.Sub` | 8.5% |
| pointwise `polValues.Mul` | ~35% |
| `runtime.memclrNoHeapPointers` | ~6.6% |

## Done

### Tooling and CI

- Module renamed to `github.com/cwbudde/bigfft`; Go floor raised to 1.25 (was 1.12).
- `.golangci.toml` — golangci-lint v2, `default = all`, every disable carrying a written
  rationale. `revive`'s `exported` rule is on: the public surface is small and documented.
- `treefmt.toml` (gofumpt + gci for Go, markdownlint + prettier for Markdown),
  `.editorconfig`, `.markdownlint.json`, `.gitignore` (the repo previously had none).
- `justfile` mirroring the sibling `algo-fft` repo's recipe names.
- Six GitHub Actions workflows in a reusable `workflow_call` layout: unit tests with
  race detector and coverage, lint, format check, cross-architecture matrix, nightly
  benchmarks, and a dispatcher.
- `scripts/run_benchmarks.sh`, `scripts/bench_compare.sh` (benchstat + regression gate).
- `README` renamed to `README.md` with badges; the 2012/2016 tables preserved in
  `docs/historical-benchmarks.md`.

The cross-architecture matrix matters more here than in a typical Go library:
`arith_decl.go` reaches into `math/big` internals via `//go:linkname`, and `fermat.go` is
full of `_W`-dependent word arithmetic. `GOARCH=386` is verified to vet and test cleanly.

### Benchmarks

- `fermat_bench_test.go` — micro-benchmarks for `Shift`, `ShiftHalf` (even and odd paths
  measured separately), `Add`, `Sub`, `Mul` (both sides of the `n < 30` branch),
  `Transform`, `InvTransform`, `polValues.Mul`. Sizes are derived at run time from the
  real `fftSize`/`valueSize` path rather than hard-coded, so they stay correct on 32-bit.

### Performance

- **Scratch arena** (`fftScratch`): one allocation set per `fftmul` call, replacing
  per-stage `make` calls in five places. `MulFFT_1Mb` went from 30 allocations and
  3.68 MB per operation to 12 allocations and 2.02 MB — 60% fewer allocations, 45% less
  memory. Wall-clock effect was within noise; this library is arithmetic-bound, not
  allocation-bound. Kept anyway: the allocation win is real and deterministic, and
  per-worker buffers are a prerequisite for parallelism.
  - Trap encountered and documented in-code: `math/big`'s internal `alias` check compares
    the address of the **last word of capacity**, so naive sub-slices of one backing array
    all look mutually aliasing. That made `big.Int.Mul` allocate defensively for every one
    of 1024 pointwise products (1038 allocs/op, worse than no arena). Every arena
    sub-slice therefore uses three-index slicing so `cap == len`.
    `TestArenaCapacitiesBounded` guards this and has been verified to fail when reverted.
- **Word-aligned shift guard**: `fermat.Shift` ended with an unconditional
  `shlVU(z, z, kb)`, which for `kb == 0` is a full-length copy of a buffer onto itself.
  Instrumentation showed 41% (5 Mb) to 71% (1 Mb) of `Shift` calls are word-aligned, and
  `lshVU` is ~11% of total runtime. Now guarded. Costs one predictable branch on the
  unaligned path.
- **Test coverage gap closed**: `TestFermatShiftHalf` only ever exercised `n = 3`, so the
  even-`n` (word-aligned halves) case was entirely untested. `fermat_shifthalf_test.go`
  adds value-based checks against `big.Int` across even and odd `n`, negative `k`,
  `k > 2N`, and `k` near multiples of `N`.

## Tried and rejected

### Fused odd-`k` `ShiftHalf` — correct, faster, and pointless

For odd `k`, `ShiftHalf` computed `2^a·x - 2^b·x` as two full `Shift`s plus a `Sub`.
Since `a - b` is always exactly `N/2`, this factors as `(2^(N/2) - 1)·t` with `t = 2^b·x`,
and multiplying by `2^(N/2)` mod `2^N+1` is a word-aligned half rotation with a sign flip.
Writing `t = (t_lo, t_hi)` gives `z_lo = -(t_hi + t_lo)`, `z_hi = t_lo - t_hi`: one general
`Shift` plus one half-length fused pass.

It worked — 25-35% faster on the odd path, differential-tested bit-exact against the
original across ~9500 cases. It was **reverted anyway**, because instrumenting the call
sites showed the odd path barely runs:

```
bits=1e6   ShiftHalf calls: 13824   odd: 0      (0.00%)
bits=5e6   ShiftHalf calls: 67584   odd: 3072   (4.55%)
bits=2e7   ShiftHalf calls: 147456  odd: 6144   (4.17%)
```

`ω2shift = (4·n·_W) >> size`, and `n·_W` is a multiple of `2^(k-2)`, so `ω2shift` is odd
on at most one recursion level — roughly `1/(2k)` of all calls. At 1 Mb and 2 Mb it never
executes at all. End-to-end: p = 0.97. Sixty lines of subtle carry logic for an
unmeasurable gain. The tests were kept; the code was not.

The general lesson is rule 5 above: this was proposed on a plausible reading of the
profile (`fermat.Shift` at 22%) without checking which *path* through `Shift` that 22%
represented. It was the even path.

## Next

Roughly in expected-value order.

### 1. Parallelism

The library is strictly single-threaded on machines that are not. Two independent
opportunities:

- **Pointwise multiply** (~35% of runtime): `polValues.mul` is a flat loop of K
  independent `fermat.Mul` calls. Embarrassingly parallel; each worker needs its own `8n`
  scratch buffer.
- **`fourier` recursion** (~45%): the two recursive calls are independent subtrees.
  Parallelize the top few levels only, staying serial below a depth cutoff so deep levels
  remain cache-resident. Each concurrent branch needs its own `fourierTemps` — which is
  why `fourierTmp` already takes them as a parameter.

Requires a `SetMaxParallelism` knob (default `GOMAXPROCS`, `1` = serial) and a size
threshold below which small multiplies stay serial. Results must be bit-identical to the
serial path; only scheduling changes.

Amdahl bound: with ~80% of runtime parallelizable, four P-cores cap the speedup at ~2.5x.
Do not expect more, and be suspicious of any measurement that claims it.

### 2. Threshold recalibration

`fftThreshold = 1800`, the `fftSizeThreshold` table, `quadraticScanThreshold = 1232`, and
`fermat.Mul`'s `n < 30` basicMul cutoff were all calibrated on a Core 2 Quad around 2012,
against a `math/big` whose Karatsuba and assembly kernels have improved substantially
since. `calibrate_test.go` already implements the measurement behind a `-calibrate` flag
(`just calibrate`); it needs to be run under the pinning protocol above and the constants
updated, with the measured values and machine recorded in `BENCHMARKS.md`.

Cheap, zero-risk, and most valuable near the crossover sizes where the stale constants
are most wrong.

### 3. Plan 9 assembly

The headline long-term item, and the reason to care about it is not only speed:
`arith_decl.go` pulls seven unexported symbols out of `math/big` via `//go:linkname`.
Those have no compatibility guarantee, and Go has already renamed some of them
(`shlVU` → `lshVU`, `addMulVVW` → `addMulVVWW`); they survive today only because the
toolchain keeps compatibility shims for known linkname users. Owning the kernels removes
the single largest fragility in this repository.

Targets, in order:

1. A **fused butterfly kernel** for amd64: `ShiftHalf` + `Add`/`Sub` in one pass with ADC
   /SBB carry chains. The current butterfly makes three passes over an `n`-word buffer
   through a `tmp` intermediate; one pass computing sum and difference together should be
   a clear win, and unlike the pure-Go version it is not competing against `math/big`'s
   hand-written assembly for the individual `addVV`/`subVV` steps. (A pure-Go fusion was
   considered and dropped for exactly that reason: a hand-rolled Go carry loop loses to
   two calls into assembly.)
2. A **shift-mod-2^n+1 kernel** (`fermat.Shift`), the single hottest primitive at 22%.
3. Then arm64.

Plumbing required, modelled on the sibling `algo-fft` repository
(`internal/asm/arch_*.go` and its `test-arch.yaml`):

- A `purego` build tag with a pure-Go fallback, built **and tested** on every matrix
  entry — the fallback is a supported configuration, not dead code.
- `go vet` per GOARCH in CI (`asmdecl` checks assembly frame sizes and FP references
  against the Go declarations; this is what catches decl↔asm drift).
- Assembly-vs-Go differential tests, in the style of the `ShiftHalf` differential test.

### 4. Smaller items

- **Cache-blocked / six-step transform** for sizes whose working set exceeds L2. At 5 Mb
  the transform is memory-bound, which is also why fusing passes is the recurring theme
  above.
- **Recursive Schönhage–Strassen for the pointwise products** instead of delegating to
  `big.Int.Mul`, once the values are large enough for that to pay.
- **`sync.Pool` for arenas**, now that a per-call arena exists. Worth it only if a
  workload does many multiplies; measure before adding global state.
- **Reuse scan temporaries** — `scan.go` carries a `// FIXME: reuse temporaries.` and
  allocates a fresh `big.Int` per recursion level.
- **Wisdom-style persisted auto-tuning** to replace the hand-run `-calibrate` flag.
- **Fuzz targets** for `Mul` and `FromDecimalString`, wired to a time-budgeted CI job.
- **Big-endian coverage**. 32-bit is already covered by the `GOARCH=386` leg.
