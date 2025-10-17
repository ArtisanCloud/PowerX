#!/usr/bin/env bash

set -euo pipefail

OUT_FILE="${BENCHMARK_OUTPUT:-reports/capability_registry_bench.txt}"
PKG="./internal/tests/perf/capability_registry"

mkdir -p "$(dirname "${OUT_FILE}")"

echo "Running capability registry benchmarks..."
go test -run=^$ -bench=. -benchmem "${PKG}" | tee "${OUT_FILE}"

echo "Benchmark results saved to ${OUT_FILE}"
