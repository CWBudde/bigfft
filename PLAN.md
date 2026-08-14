# PLAN

Working notes for this fork: what has been done, what was tried and rejected, and what
is worth doing next. Upstream is
[remyoudompheng/bigfft](https://github.com/remyoudompheng/bigfft); this fork adds modern
tooling and pursues performance work the original explicitly left as a proof of concept.

## Status at a glance

| Area                    | Progress                                                 |
| ----------------------- | -------------------------------------------------------- |
| Tooling and CI          | done                                                     |
| Benchmarks              | done                                                     |
| Parallelism             | done; separate parallel threshold measured, no room      |
| Allocation / arena      | done                                                     |
| Threshold recalibration | done: all four measured; one changed, three pinned       |
| Decimal scanning        | done: balanced split, -4% serial / -8% parallel          |
| Plan 9 assembly         | done; linknames gone, fused Add/Sub tails on amd64/arm64 |
| Coefficient planning    | done for measured amd64 k=12 plateaus, up to -19%        |
| Follow-up performance   | pool + scan reuse done; cache/recursive measured out     |

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

| Component                      | Share |
| ------------------------------ | ----: |
| `fourier` transforms           |  ~45% |
| — of which `fermat.Shift`      |   22% |
| — of which `fermat.Add`        | 14.5% |
| — of which `fermat.Sub`        |  8.5% |
| pointwise `polValues.Mul`      |  ~35% |
| `runtime.memclrNoHeapPointers` | ~6.6% |

## Done

### Tooling and CI

- [x] Module renamed to `github.com/cwbudde/bigfft`; Go floor raised to 1.25 (was 1.12).
- [x] `.golangci.toml` — golangci-lint v2, `default = all`, every disable carrying a
      written rationale. `revive`'s `exported` rule is on: the public surface is small and
      documented.
- [x] `treefmt.toml` (gofumpt + gci for Go, markdownlint + prettier for Markdown),
      `.editorconfig`, `.markdownlint.json`, `.gitignore` (the repo previously had none).
- [x] `justfile` mirroring the sibling `algo-fft` repo's recipe names.
- [x] Six GitHub Actions workflows in a reusable `workflow_call` layout: unit tests with
      race detector and coverage, lint, format check, cross-architecture matrix, nightly
      benchmarks, and a dispatcher.
- [x] `scripts/run_benchmarks.sh`, `scripts/bench_compare.sh` (benchstat + regression gate).
- [x] `README` renamed to `README.md` with badges; the 2012/2016 tables preserved in
      `docs/historical-benchmarks.md`.

The cross-architecture matrix matters more here than in a typical Go library: the owned
arithmetic kernels have architecture-specific assembly and `fermat.go` is full of
`_W`-dependent word arithmetic. `GOARCH=386` is verified to vet and test cleanly.

### Benchmarks

- [x] `fermat_bench_test.go` — micro-benchmarks for `Shift`, `ShiftHalf` (even and odd
      paths measured separately), `Add`, `Sub`, `Mul` (both sides of the
      `fermatBasicMulThreshold` branch), `Transform`, `InvTransform`, `polValues.Mul`.
      Sizes are derived at run time from the real `fftSize`/`valueSize` path rather than
      hard-coded, so they stay correct on 32-bit.

### Performance

- [x] **Parallelism** (`parallel.go`): the pointwise multiply (~35% of runtime), the
      `fourier` recursion and its butterfly loops (~45%) are sharded across up to
      `GOMAXPROCS` workers via a `parRange` helper that runs the last shard inline and
      spawns only `w-1` goroutines. `SetMaxParallelism`/`MaxParallelism` control it; below
      `parallelWordThreshold` (8192 words of transform array, measured) it stays serial.
      Results are bit-identical to the serial path — only scheduling changes.
  - Measured on four pinned P-cores: −22.5% at 500 kb, −30.1% at 1 Mb, −43.1% at 5 Mb,
    −48.0% at 10 Mb (all p < 0.001). Whole machine at 10 Mb: −62.7%. Serial cost ~1–2%.
  - The two forward transforms were **left sequential**: both stage through arena region
    `a`, so running them concurrently needs a fourth `N*K` staging region (+33% working
    set) to buy at most 2x, whereas the intra-transform sharding needs no extra storage
    and scales further.
  - Trap: passing a closure to `parRange` from inside the recursion heap-allocated one
    closure per node (10 → 791 allocs/op). Extracting `butterflies()` and calling it
    directly in the serial case fixed it.
  - Race coverage was **verified, not assumed**: deliberately pointing two workers at a
    shared buffer produces `DATA RACE` reports in both `polValues.mul` and the butterfly
    loop. A green `-race` run proves nothing if the parallel path never ran.
- [x] **Scratch arena** (`fftScratch`): one allocation set per `fftmul` call, replacing
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
- [x] **Bounded `sync.Pool` reuse for scratch arenas.** Repeated multiplies with the same
      plan now reuse exact `(k, n, worker-count)` arena shapes. Mismatches are discarded,
      and arenas above 16 MiB bypass the pool entirely so a one-off huge multiplication
      cannot leave its workspace in global state. Poisoned-reuse and concurrent race tests
      guard the zeroing and ownership assumptions.
  - Ten interleaved serial repetitions: allocated bytes fell **84.9% at 200 kbit, 85.7%
    at 1 Mbit and 85.9% at 5 Mbit**; allocation counts fell 23-29%. Wall time improved
    **11.7% at 200 kbit (p=0.023)** and **9.7% at 1 Mbit (p=0.009)**, and was flat at
    5 Mbit (p=0.143). A 20 Mbit control exceeds the retention cap and was exactly flat in
    bytes/allocations, with wall time p=0.842.
  - On four P-cores, time was flat at 1/5 Mbit (p=1.000/0.353), while bytes still fell
    about 77%. The library remains arithmetic-bound once parallel work dominates.
- [x] **Word-aligned shift guard**: `fermat.Shift` ended with an unconditional
      `shlVU(z, z, kb)`, which for `kb == 0` is a full-length copy of a buffer onto itself.
      Instrumentation showed 41% (5 Mb) to 71% (1 Mb) of `Shift` calls are word-aligned,
      and `lshVU` is ~11% of total runtime. Now guarded. Costs one predictable branch on
      the unaligned path.
- [x] **Test coverage gap closed**: `TestFermatShiftHalf` only ever exercised `n = 3`, so
      the even-`n` (word-aligned halves) case was entirely untested.
      `fermat_shifthalf_test.go` adds value-based checks against `big.Int` across even and
      odd `n`, negative `k`, `k > 2N`, and `k` near multiples of `N`.
- [x] **Threshold recalibration.** All four 2012-era constants re-measured under the
      protocol above; one changed, three pinned with the data published in BENCHMARKS.md.
      `quadraticScanThreshold` and `fermat.Mul`'s cutoff are now `var`s so the harness can
      sweep them; production never assigns to them.
  - **`fftSizeThreshold[8]`: `1<<18` → `1<<19`** — the only change. At the old boundary an
    FFT of length `1<<8` beat the `1<<9` the table switched to by 20%, monotonically, with
    parity only at twice the boundary. Interleaved A/B on the public `Mul`: **−21.7% at
    150 kbit, −11.0% at 200 kbit, −8.1% at 250 kbit** serial (−24.1 / −13.8 / −12.4% on
    four P-cores), all p < 0.001, with `Mul_100kb` (below `fftThreshold`, never enters
    `fftmul`) flat as a control and no regression at 500 kbit or 1 Mb.
  - Entries 3–7 show the identical shape but lie entirely below `fftThreshold`, so the
    public `Mul` cannot select them and no end-to-end benchmark can confirm a change.
    **Left unchanged** deliberately. Entries 9–13 oscillate across 1.0 — the `fftThreshold`
    signature — and 14–15 were not measured (629 Mbit operands exceed available memory).
  - **`fermat.Mul`'s cutoff: left at 30.** From `n=14` to `n=48` schoolbook and
    `big.Int.Mul` are within 1.00 ± 3% of each other with no crossover; the two have simply
    converged. And the table change moved the 131–211 kbit window from `k=9` to `k=8`,
    so `basicMul` is now **unreachable through the public `Mul` on both word sizes** (it
    already was on 386). `TestFermatBasicMulThresholdReachable` enumerates this mechanically
    and will say so again if the table moves.
  - **`quadraticScanThreshold`: left at 1232.** The spread across candidates is not noise
    but recursion-tree quantization — thresholds a power of two apart time identically to
    within 1%, families differ by up to 22%, and the ranking inverts between input sizes.
    That quantization is what prompted the balanced-split rewrite, done next and recorded
    below; the threshold itself is still 1232 after it.
  - Two always-on guards were added and **verified to fail when reverted**:
    `TestFFTSizeThresholdMonotone` (`fftSize` takes the first entry `> bits`, so a
    non-monotone table silently picks the wrong `k` with nothing else failing) and
    `TestFermatBasicMulThresholdReachable`.
  - Method note: the new sweeps publish a flat grid instead of bisecting. Bisection is how
    the 2012 constants were produced and it cannot tell a crossover from an oscillation —
    which is precisely what it did to `fftThreshold`. Every constant that moved here had a
    monotone curve _and_ an interleaved A/B on the public API behind it.

- [x] **Balanced splitting in `scan.go`.** `FromDecimalString` now splits exactly in half
      and builds a per-input power table (one entry per depth, each the square of the next,
      times ten for odd lengths) instead of splitting off the largest cached
      `quadraticScanThreshold << k` chunk. Interleaved A/B: **−12.2% at 10k digits, −6.5%
      at 100k, −8.8% at 1M, −6.9% at 2M** serial, all p < 0.001; **−12.5 / −9.5 / −10.4%**
      on four P-cores. Geomean −4.0% serial, −8.3% parallel.
  - 5M–10M is flat in the ten-repetition run but measured **+1.8% and +3.7%** in a tighter
    twelve-repetition one, and that regression is real. The old `1232 << k` chunk length is
    about `2^(12+k)` bits (1232 digits = 4092.8, just under 4096), so every chunk landed
    near the top of a `valueSize` plateau by construction. Exact halving lands wherever the
    input falls. Reported rather than hidden.
  - Recovering the alignment by shading the split **does not work**, and
    `TestCalibrateScanSplit` keeps the evidence: from 97% to 100% of half the timings are
    flat, and below that they degrade sharply (10M digits: 986 ms at exact half, 1.18 s at
    95%, 1.85 s at 90%) because the imbalance compounds down the recursion faster than the
    alignment pays.
  - Incidental: the table base is now built by binary exponentiation, so
    `quadraticScanThreshold` no longer has to be a multiple of 14 — any value is legal, and
    the sweep in `TestCalibrateScan` may be widened accordingly. Inputs at or below the
    threshold bypass the scanner entirely, which removed a 3.4% regression at 1k digits
    that the first version introduced.
  - `TestScanPowerTable` and `TestScanThresholds` are always-on guards, **verified to fail
    when the odd-length correction is removed**.
- [x] **Reused decimal-scan multiplication destinations.** The scanner owns one `big.Int`
      temporary per recursion depth, and the internal multiply path can evaluate both
      `math/big` and FFT products into caller-owned result storage. `Mul` still returns a
      fresh `*big.Int`; this only changes internal ownership.
  - Ten interleaved serial repetitions cut bytes/op by **28.9% at 10k digits, 28.7% at
    100k, 20.0% at 1M and 15.9% at 2M**. Allocations fell **32.5%, 54.7%, 58.3% and
    58.5%** respectively, all p=0.000. Time improved 4.2% at 100k (p=0.035) and had no
    measured difference at the other sizes (p=0.165-0.280).

- [x] **Owned Plan 9 arithmetic and fused Add/Sub butterfly tails.** The six unexported
      `math/big` linknames are gone. `addVV`, `subVV`, `lshVU`, and `addMulVVW` are now
      local amd64 and arm64 Plan 9 assembly; `addVW`/`subVW` and every `purego` or
      other-architecture build use owned Go implementations. This removes the repository's
      largest toolchain compatibility hazard, including the renamed `shlVU` and
      `addMulVVW` shims.
  - After `ShiftHalf` produces the twiddle product in `tmp`, the fused butterfly tail
    computes the low-word sum and difference together. amd64 uses two-word blocks with
    independent saved ADC/SBB chains; arm64 uses four-word ADCS/SBCS blocks. Exact
    differential tests cover carries, borrows, aliases, odd lengths, and guard words.
  - The arithmetic kernels are 12-62% faster than their always-available Go oracles at
    representative lengths. This is a fallback comparison, not an end-to-end claim: the
    old build already reached `math/big` assembly through linkname.
  - Interleaved end-to-end A/B, ten repetitions, single P-core: transform `n=27` is
    **-6.0%** (p=0.002); transforms `n=64`/`n=80` and `MulFFT` at 1/5/10 Mb show no
    measured difference (all p=0.53-0.97). Geomean -1.9%; allocations unchanged.
  - `purego` is tested on every CI architecture, and default plus purego builds are vetted
    per GOARCH so asmdecl checks every assembly declaration/frame pair.
  - A complete arm64 shift-mod kernel and same-binary benchmark are present but deliberately
    not dispatched until the native ARM machine is available. QEMU validates it bit-exactly
    but cannot supply meaningful performance data.

- [x] **Plateau-aware FFT planning on amd64.** `valueSize` padding makes an incumbent
      `k=12` plan abruptly expensive at repeatable coefficient boundaries. A forced-plan
      grid measured the incumbent and valid `k±1` plans through the full multiplication
      path, then a separate-binary public `Mul` A/B validated the production selector.
  - The selector recomputes both `m` and `n` for `k+1` and uses it only when its coefficient
    length is less than 4/7 of the incumbent. This admits the measured winning windows and
    rejects the neighboring padding steps, where `k+1` is 17-55% slower.
  - Serial wins are -5.2% to -19.3% (p=0.000-0.009); four-P-core wins are -5.9% to -18.1%
    at the stable points. Boundary controls are flat. Selected plans use 2-11% more bytes
    per operation; allocation counts are effectively flat.
  - The policy is enabled only for default amd64 builds because that is the implementation
    measured. `purego` and other architectures keep the threshold-table incumbent until
    their own results justify enabling it.

## Tried and rejected

### ~~amd64 shift-mod dispatch~~ — the hot aligned primitive did not move `MulFFT`

Two amd64 shift kernels were measured. A faithful monolithic port was 40-80% slower than
the existing composition of copy/subtract/`lshVU`. A specialized one-pass negacyclic word
rotation did win on aligned positive shifts (about 27% at `n=80`) and was roughly flat on
aligned negative shifts. That was the right path — 41-71% of production shifts are word
aligned — but dispatching it at the `Shift` boundary made unaligned calls pay for selection,
while dispatching only at the butterfly call site merely broke even.

Ten interleaved repetitions against the fused-Add/Sub build: geomean -0.43%, every
transform and `MulFFT_1Mb`/`5Mb`/`10Mb` comparison p=0.06-0.97. No measurable end-to-end
gain, so the amd64 shift kernel and dispatch were removed. The owned `lshVU` kernel remains:
it removes the linkname and is the faster building block on the mixed shift workload.

### ~~Fused odd-`k` `ShiftHalf`~~ — correct, faster, and pointless

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
profile (`fermat.Shift` at 22%) without checking which _path_ through `Shift` that 22%
represented. It was the even path.

### ~~Cache-blocked / fused transform levels~~ — no end-to-end win

A cache-pass-fusion prototype combined two adjacent reconstruction levels once a subtree
exceeded 1.25 MiB. Four coefficients stayed hot and one intermediate child-array
read/write pass disappeared. Instrumentation confirmed that production used it once per
transform at 5 Mbit and at the root plus four child subtrees at 10 Mbit. Differential
forward/inverse tests forced the fused path at every eligible level, including the parallel
schedule, and race, 386 and WebAssembly tests passed.

It did not improve the public multiplication. Ten interleaved serial repetitions with ten
operations per sample:

| Workload      | Baseline |    Fused | Result             |
| ------------- | -------: | -------: | ------------------ |
| `MulFFT_2Mb`  | 8.405 ms | 8.080 ms | p=0.190 control    |
| `MulFFT_5Mb`  | 24.86 ms | 23.85 ms | -4.1%, **p=0.052** |
| `MulFFT_10Mb` | 57.44 ms | 60.55 ms | +5.4%, **p=0.063** |

Neither gated workload crossed the significance threshold, their directions disagree, and
the geomean across the control and gated cases was -0.9%. The refactor incidentally removed
six small closure allocations per multiplication, but not allocated bytes; bounded arena
pooling removes much more allocation without coupling adjacent FFT levels. The fusion was
therefore not retained.

### ~~Recursive Schönhage–Strassen pointwise products~~ — crossover is impractically large

On the measured amd64 path, real transformed-value distributions are dense but small:
median/p99/max were all 64 words
at 1 Mbit, 80 at 5 Mbit and 384 at 100 Mbit. Even the largest existing public benchmark,
1 Gbit per operand, plans `k=16` with 65,536 pointwise products of at most 1,024 words,
below the 1,800-word FFT crossover. `TestRecursivePointwiseMulBenchmarkReachability`
records this mechanically on 64-bit builds; 32-bit reaches larger word counts but cannot
fit the multi-gigabyte outer workload needed for an end-to-end crossover measurement.

The first plan above that crossover has 2,048-word values at about 1.877 Gbit per operand,
requiring roughly 4.16 GB of active memory. A forced recursive prototype was still flat/
slower there (444.5 vs 487.5 µs, p=0.280). Its first significant micro-benchmark win was
at 4,096 words (-23.8%, p=0.001), reached around 4.024 Gbit per operand with about 8.46 GB
active memory. The naive nested FFT also allocated 608,880 bytes per pointwise product —
about 40 GB cumulatively across 65,536 points — so a production version would first need
nested scratch and header reuse. Those workloads cannot be calibrated end to end on this
machine, and no current reachable workload benefits, so no dispatch was added.

### ~~A separate threshold for the parallel path~~ — the crossover barely moves

The idea, carried at the top of the to-do list since the parallel path landed: `fftThreshold`
was calibrated with parallelism disabled, so with four cores the FFT should start beating
`math/big` well below 1800 words, and a second lower threshold selected when parallelism is
active would pick up the band between the two crossovers.

The band is essentially empty. `BenchmarkMulDispatchCrossover` times Karatsuba against both
FFT modes across the region, 30 interleaved repetitions:

```
operand    big vs serial FFT    big vs parallel FFT
 90 kbit        +3.64%                 +7.38%
105 kbit        +7.89%                 +8.28%
115 kbit        +8.46%              ~  (p=0.051)
120 kbit        -2.99%                 -8.29%
150 kbit       -15.05%                -26.01%
```

The parallel FFT breaks even at **115 kbit**; `fftThreshold = 1800` words is **115.2 kbit**.
Four cores move the dispatch crossover by about 3 kbit — some 60 words, under 3% — not the
substantial shift the item assumed. There is no range to pick up, and a second threshold
would add a mode to the dispatch logic in exchange for nothing.

Two things came out of the measurement that were worth having anyway:

- The transform-array sizes documented for `parallelWordThreshold` were **stale**. They
  predated `fftSizeThreshold[8]` going from `1<<18` to `1<<19`, which moved 150 kbit
  operands from `k=9` to `k=8` and from 11776 to 10240 words. The re-measurement also
  overturned one of its data points: 100 kbit was recorded as `p=0.70`, "no difference", and
  is actually −2.05% at 30 repetitions. The crossover is between 5632 and 7168 words, not
  between 7168 and 11776.
- `parallelWordThreshold` was nonetheless **left at 8192**, because 8192 words of transform
  array is 1792 words per operand against an `fftThreshold` of 1800. Everything a lower gate
  would admit lies below the point where `Mul` enters the FFT, so lowering it would change
  nothing reachable through the public API.

`TestParallelDispatchOverlap` now records the relationship between the two gates
mechanically and will say so if a future table change opens a band that today is eight words
wide. The measurements are **provisional**: taken at load average 2.3–3.6 rather than idle,
at the user's direction, with dispersion of ±0–2%, every p at 0.000, and monotone curves. A
confirming run on a quiet machine is still owed, and they are marked as such in
BENCHMARKS.md.

The lesson is a variant of rule 5: the premise had been sitting in BENCHMARKS.md as a plain
assertion ("with parallelism enabled the FFT wins earlier than 1800 words") long enough to
look like a finding. It had never been measured.

## To do

Roughly in expected-value order.

### 1. Smaller items

- [ ] **Wisdom-style persisted auto-tuning** to replace the hand-run `-calibrate` flag.
- [ ] **Fuzz targets** for `Mul` and `FromDecimalString`, wired to a time-budgeted CI job.
- [ ] **Big-endian coverage**. 32-bit is already covered by the `GOARCH=386` leg.
