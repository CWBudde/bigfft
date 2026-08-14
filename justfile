# Build the library
build:
    go build -v ./...

# Run all tests
# -timeout is a backstop, not a target: the race detector roughly triples the
# cost of the multi-megabit multiplication tests, which are already the
# slowest thing in the suite. Go's 10m default leaves no headroom.
test:
    go test -v -race -count=1 -timeout=20m ./...

# Run the supported pure-Go fallback (no architecture assembly).
test-purego:
    go test -tags "purego" -v -count=1 -timeout=20m ./...

# Run benchmarks
bench:
    go test -bench=. -benchmem -run=^$ -timeout=60m ./...

# Run a quick benchmark pass (one iteration, small time budget)
bench-quick:
    go test -bench=. -benchmem -benchtime=10x -run=^$ -timeout=20m ./...

# Run linters
lint:
    golangci-lint run

# Run linters and fix issues
lint-fix:
    golangci-lint run --fix

# Format code using treefmt
fmt:
    treefmt . --allow-missing-formatter

# Check if code is formatted
fmt-check:
    treefmt --allow-missing-formatter --fail-on-change

# Generate coverage report
cover:
    go test -coverprofile=coverage.txt -covermode=atomic ./...
    go tool cover -html=coverage.txt -o coverage.html

# Run all checks (test, lint, coverage)
check: test lint cover

# Clean build artifacts
clean:
    rm -f coverage.txt coverage.html
    rm -f benchmarks/latest-*.txt benchmarks/benchstat.txt
    rm -rf dist/
    find . -type f \( -name '*.test' -o -name '*.prof' \) -delete

# Re-measure every tuning constant: fftThreshold and the fftSizeThreshold
# table (fft.go), fermatBasicMulThreshold (fermat.go) and
# quadraticScanThreshold (scan.go). See calibrate_test.go, and BENCHMARKS.md
# for the results and the pinning protocol — run these on a quiet machine
# under `taskset -c 0` with GOMAXPROCS=1, or the numbers are fiction.
calibrate:
    go test -v -run=TestCalibrate -timeout=600m -calibrate

# The FFT-vs-math/big crossover (fftThreshold) and the FFT size table.
calibrate-fft:
    go test -v -run='TestCalibrateThreshold|TestCalibrateFFT' -timeout=600m -calibrate

# The basicMul / big.Int.Mul crossover in fermat.Mul.
calibrate-fermat:
    go test -v -run=TestCalibrateFermatMul -timeout=60m -calibrate

# The decimal scanning threshold, and FromDecimalString vs big.Int.SetString.
calibrate-scan:
    go test -v -run=TestCalibrateScan -timeout=120m -calibrate

# The serial/parallel crossover (parallelWordThreshold). Unlike the four above
# this is a benchmark, not a -calibrate test: BenchmarkMulFFTParallelSweep runs
# both modes in one binary, with the gate under measurement disabled on the
# parallel side. Pin it differently too — `taskset -c 0-3` with GOMAXPROCS=4,
# not the single core the others use — and pivot with `benchstat -col /mode`.
calibrate-parallel:
    go test -run=XXX -bench=MulFFTParallelSweep -count=14 -timeout=120m

# Vet assembly declarations on every supported assembly architecture and keep
# the _W-dependent pure-Go arithmetic honest on 386.
vet-arch:
    GOARCH=amd64 go vet ./...
    GOARCH=386 go vet ./...
    GOARCH=arm64 go vet ./...
    go vet -tags "purego" ./...

# Apply every automatic fix (lint then format)
fix:
    just lint-fix
    just fmt

# Default target
default: build
