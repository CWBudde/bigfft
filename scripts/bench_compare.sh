#!/usr/bin/env bash
#
# Compare a benchmark run against a committed baseline and fail if anything
# regressed by more than BENCH_THRESHOLD_PCT (default 10).
#
# Usage: bench_compare.sh [baseline.txt] [current.txt]
set -euo pipefail

baseline_path="${1:-benchmarks/baseline.txt}"
current_path="${2:-benchmarks/latest.txt}"
threshold="${BENCH_THRESHOLD_PCT:-10}"

if [[ ! -f "$baseline_path" ]]; then
  echo "bench_compare: baseline file not found: $baseline_path" >&2
  exit 0
fi

if [[ ! -f "$current_path" ]]; then
  echo "bench_compare: current benchmark file not found: $current_path" >&2
  exit 1
fi

if ! command -v benchstat >/dev/null 2>&1; then
  echo "bench_compare: benchstat not found in PATH" >&2
  exit 1
fi

mkdir -p benchmarks

# Human-readable report.
benchstat "$baseline_path" "$current_path" | tee benchmarks/benchstat.txt

# The gate parses the CSV form rather than the text table. benchstat's text
# layout is not parseable: it strips the "Benchmark" prefix from names, scales
# each row's unit independently (ns, µs, ms), and carries the unit in a column
# header rather than on the row. An earlier version of this script grepped for
# '^Benchmark.*ns/op' and therefore matched nothing at all, silently passing
# every run.
#
# In CSV the "vs base" column is "~" when the difference is not statistically
# significant, so any numeric value there is already significant at benchstat's
# default alpha.
benchstat -format=csv "$baseline_path" "$current_path" >benchmarks/benchstat.csv 2>/dev/null

awk -v threshold="$threshold" -F, '
  # A header row names the metric for the table that follows.
  $2 == "sec/op" || $2 == "B/op" || $2 == "allocs/op" { metric = $2; next }

  # Gate on wall-clock only; allocation changes are reported but not enforced.
  metric != "sec/op" { next }
  NF < 6 { next }

  {
    name = $1
    delta = $6
    if (name == "" || delta == "") next
    seen++
    if (delta == "~") next
    if (delta !~ /%$/) next

    pct = delta
    gsub(/[%+]/, "", pct)
    if (pct + 0 > threshold + 0) {
      printf("bench_compare: %s regressed by %s (threshold %s%%)\n",
             name, delta, threshold) > "/dev/stderr"
      bad++
    }
  }

  END {
    # Silence must not be mistaken for success: if the format changes again and
    # nothing parses, say so instead of reporting a clean run.
    if (seen == 0) {
      print "bench_compare: no sec/op comparison rows found in benchstat CSV;" \
            " the regression gate did not actually run" > "/dev/stderr"
      exit 2
    }
    if (bad > 0) {
      printf("bench_compare: %d benchmark(s) regressed\n", bad) > "/dev/stderr"
      exit 1
    }
    printf("bench_compare: %d benchmark(s) compared, none regressed by more than %s%%\n",
           seen, threshold)
  }
' benchmarks/benchstat.csv
