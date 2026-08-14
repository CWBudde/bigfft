#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
DEMO_DIR="$ROOT_DIR/examples/wasm-demo"
OUT_DIR="${1:-$ROOT_DIR/dist}"

mkdir -p "$OUT_DIR"

GOOS=js GOARCH=wasm go build -o "$OUT_DIR/bigfft.wasm" "$DEMO_DIR"

shopt -s nullglob
assets=("$DEMO_DIR"/*.html "$DEMO_DIR"/*.css "$DEMO_DIR"/*.js)
shopt -u nullglob

if [ ${#assets[@]} -eq 0 ]; then
    echo "error: no static assets found in $DEMO_DIR" >&2
    exit 1
fi

cp "${assets[@]}" "$OUT_DIR/"
cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" "$OUT_DIR/"

printf 'WASM demo built at %s (%d static assets)\n' "$OUT_DIR" "${#assets[@]}"
