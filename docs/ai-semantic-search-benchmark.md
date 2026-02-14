# AI Semantic Search Benchmark (10k)

Last updated: 2026-02-14
Owner: @raythunder

## 1. Goal

Validate semantic search latency for the current MVP implementation at `10k memos` scale.

Target from plan:

- `P95 < 500ms` for semantic retrieval on single instance baseline.

## 2. Benchmark Setup

- Database: `PostgreSQL` (`DRIVER=postgres`)
- API path: `SearchMemosSemantic`
- Corpus size: `10,000` memos
- Embedding dimension (benchmark synthetic vectors): `64`
- Query page size: `20`
- Execution count: `30` fixed iterations (`-benchtime=30x`)
- Host:
  - `goos=darwin`
  - `goarch=arm64`
  - `cpu=Apple M4 Pro`

## 3. Command

```bash
DRIVER=postgres scripts/benchmark-semantic-search.sh
```

Raw command (equivalent):

```bash
DRIVER=postgres go test ./server/router/api/v1/test -run '^$' -bench '^BenchmarkSearchMemosSemanticPostgres10k$' -benchtime=30x -count=1
```

## 4. Result

Run date: 2026-02-14

- `ns/op`: `147030754` (~147.0ms mean)
- `p50_ms`: `146.5`
- `p95_ms`: `152.4`
- `p99_ms`: `154.2`

Conclusion:

- Current semantic retrieval path meets latency target with margin (`152.4ms < 500ms`).

## 5. Decision

- Keep current app-layer cosine ranking implementation for this phase (KISS/YAGNI).
- Do not introduce `pgvector` index yet.
- Re-open optimization only when one of the following occurs:
  - corpus grows beyond current target (`>50k` memos), or
  - benchmark/regression p95 exceeds `500ms`.

## 6. Local Runbook

1. Ensure Docker is running (for postgres test container).
2. Execute `DRIVER=postgres scripts/benchmark-semantic-search.sh` in repo root.
3. Record outputs (`ns/op`, `p50_ms`, `p95_ms`, `p99_ms`) to tracker.
4. If `p95_ms >= 500`, open performance task to evaluate `pgvector` index path.
