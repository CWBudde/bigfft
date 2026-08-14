# bigfft WASM demo

A minimal browser interface for multiplying decimal integers with `bigfft`
compiled to WebAssembly. It also includes a small browser-local benchmark of
`bigfft.Mul` against `math/big.Int.Mul`.

## Build and run

```bash
just run-wasm-demo
```

This builds the demo into `./dist` and serves it at
<http://localhost:8090>. To build without starting a server:

```bash
just build-wasm-demo
```

The page must be opened through an HTTP server rather than directly from the
filesystem because it fetches `bigfft.wasm` during startup.

## Bridge

The Go program publishes `globalThis.bigfft.multiply({ left, right })` and
`globalThis.bigfft.benchmark({ digits, iterations })`. Interactive operands are
parsed with `bigfft.FromDecimalString` and multiplied with `bigfft.Mul`. The
bridge catches panics before they escape into the Go WASM runtime, where they
would otherwise terminate the instance.

The benchmark uses deterministic operands and reports average multiplication
time for `bigfft.Mul` and `math/big.Int.Mul`. Operand generation, parsing,
correctness checking and decimal formatting are outside the timed region. Its
numbers characterize the current browser's portable WASM path, not bigfft's
native assembly paths. Generated benchmark operands range from 10,000 to
1,000,000 decimal digits; the largest presets may keep the browser tab busy for
several seconds.

The interactive multiplier limits each operand to 100,000 decimal digits to
keep accidental inputs from exhausting a browser tab. The generated benchmark
has a separate 1,000,000-digit limit. The “FFT-sized sample” button loads two
40,000-digit values.
