#!/bin/sh

set -eu

# Change to repo root.
cd "$(dirname "$0")/../"

BENCHMARK_PACKAGE="./server/router/api/v1/test"
BENCHMARK_NAME="BenchmarkSearchMemosSemanticPostgres10k"
BENCHTIME="${BENCHTIME:-30x}"
COUNT="${COUNT:-1}"

if [ "${DRIVER:-postgres}" != "postgres" ]; then
  echo "error: semantic benchmark requires DRIVER=postgres"
  exit 1
fi

OUTPUT_FILE="$(mktemp)"
cleanup() {
  rm -f "$OUTPUT_FILE"
}
trap cleanup EXIT INT TERM

DRIVER=postgres go test "$BENCHMARK_PACKAGE" \
  -run '^$' \
  -bench "^${BENCHMARK_NAME}$" \
  -benchtime "$BENCHTIME" \
  -count "$COUNT" "$@" | tee "$OUTPUT_FILE"

RESULT_LINE="$(grep "$BENCHMARK_NAME" "$OUTPUT_FILE" | tail -n 1 || true)"
if [ -z "$RESULT_LINE" ]; then
  echo "error: benchmark output does not contain ${BENCHMARK_NAME}"
  exit 1
fi

extract_metric() {
  label="$1"
  value="$(echo "$RESULT_LINE" | awk -v label="$label" '{for (i = 1; i <= NF; i++) if ($i == label) {print $(i - 1); exit}}')"
  if [ -z "$value" ]; then
    echo "n/a"
    return
  fi
  echo "$value"
}

NS_PER_OP="$(extract_metric "ns/op")"
P50_MS="$(extract_metric "p50_ms")"
P95_MS="$(extract_metric "p95_ms")"
P99_MS="$(extract_metric "p99_ms")"

echo ""
echo "Semantic search benchmark summary:"
echo "  benchmark: $BENCHMARK_NAME"
echo "  benchtime: $BENCHTIME"
echo "  count:     $COUNT"
echo "  ns/op:     $NS_PER_OP"
echo "  p50_ms:    $P50_MS"
echo "  p95_ms:    $P95_MS"
echo "  p99_ms:    $P99_MS"
