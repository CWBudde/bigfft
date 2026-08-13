# Build the library
build:
    go build -v ./...

# Run all tests
# -timeout is a backstop, not a target: the race detector roughly triples the
# cost of the multi-megabit multiplication tests, which are already the
# slowest thing in the suite. Go's 10m default leaves no headroom.
test:
    go test -v -race -count=1 -timeout=20m ./...

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

# Re-measure the FFT-vs-math/big crossover point.
# Prints the measured speedups so fftThreshold in fft.go can be retuned; see
# TestCalibrate in calibrate_test.go.
calibrate:
    go test -v -run=TestCalibrate -timeout=60m -calibrate

# Vet on every supported word size / architecture. arith_decl.go's
# //go:linkname pulls into math/big and the _W-dependent word arithmetic in
# fermat.go are exactly what breaks on 32-bit, so 386 is not optional here.
vet-arch:
    GOARCH=amd64 go vet ./...
    GOARCH=386 go vet ./...
    GOARCH=arm64 go vet ./...

# Apply every automatic fix (lint then format)
fix:
    just lint-fix
    just fmt

# Default target
default: build
