# bigfft WASM demo

A minimal browser interface for multiplying decimal integers with `bigfft`
compiled to WebAssembly.

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

The Go program publishes `globalThis.bigfft.multiply({ left, right })`. Both
operands are parsed with `bigfft.FromDecimalString` and multiplied with
`bigfft.Mul`. The bridge catches panics before they escape into the Go WASM
runtime, where they would otherwise terminate the instance.

The demo limits each operand to 100,000 decimal digits to keep accidental
inputs from exhausting a browser tab. The “FFT-sized sample” button loads two
40,000-digit values.
