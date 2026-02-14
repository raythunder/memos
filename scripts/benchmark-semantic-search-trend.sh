#!/bin/sh

set -eu

# Change to repo root.
cd "$(dirname "$0")/../"

TREND_FILE="${TREND_FILE:-docs/ai-semantic-search-benchmark-trend.md}"
BENCHTIME="${BENCHTIME:-30x}"
COUNT="${COUNT:-1}"
NOTE="${NOTE:-}"

TMP_OUTPUT="$(mktemp)"
cleanup() {
  rm -f "$TMP_OUTPUT"
}
trap cleanup EXIT INT TERM

DRIVER=postgres BENCHTIME="$BENCHTIME" COUNT="$COUNT" scripts/benchmark-semantic-search.sh "$@" | tee "$TMP_OUTPUT"

NS_PER_OP="$(awk '/^  ns\/op:/ {print $2}' "$TMP_OUTPUT" | tail -n 1)"
P50_MS="$(awk '/^  p50_ms:/ {print $2}' "$TMP_OUTPUT" | tail -n 1)"
P95_MS="$(awk '/^  p95_ms:/ {print $2}' "$TMP_OUTPUT" | tail -n 1)"
P99_MS="$(awk '/^  p99_ms:/ {print $2}' "$TMP_OUTPUT" | tail -n 1)"

if [ -z "${NS_PER_OP}" ] || [ -z "${P50_MS}" ] || [ -z "${P95_MS}" ] || [ -z "${P99_MS}" ]; then
  echo "error: failed to parse benchmark summary from scripts/benchmark-semantic-search.sh output"
  exit 1
fi

RUN_AT_UTC="$(date -u '+%Y-%m-%d %H:%M:%S UTC')"
GIT_SHA="$(git rev-parse --short HEAD)"

if [ ! -f "$TREND_FILE" ]; then
  cat > "$TREND_FILE" <<EOF
# AI Semantic Search Benchmark Trend

Last updated: ${RUN_AT_UTC}

| Run At (UTC) | Commit | Benchtime | Count | ns/op | p50_ms | p95_ms | p99_ms | Note |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
EOF
fi

if rg -q '^Last updated:' "$TREND_FILE"; then
  # Keep a single Last updated line at top.
  TMP_TREND="$(mktemp)"
  awk -v updated="$RUN_AT_UTC" '
    BEGIN { replaced = 0 }
    {
      if (!replaced && $0 ~ /^Last updated:/) {
        print "Last updated: " updated
        replaced = 1
      } else {
        print $0
      }
    }
  ' "$TREND_FILE" > "$TMP_TREND"
  mv "$TMP_TREND" "$TREND_FILE"
fi

printf '| %s | `%s` | `%s` | `%s` | `%s` | `%s` | `%s` | `%s` | %s |\n' \
  "$RUN_AT_UTC" \
  "$GIT_SHA" \
  "$BENCHTIME" \
  "$COUNT" \
  "$NS_PER_OP" \
  "$P50_MS" \
  "$P95_MS" \
  "$P99_MS" \
  "${NOTE:-n/a}" >> "$TREND_FILE"

echo "Appended trend record to $TREND_FILE"
