# AI Semantic Search Operations Runbook

Last updated: 2026-02-14
Owner: @raythunder

## 1. Scope

This runbook covers the current semantic-search MVP only:

- storage driver: `postgres`
- embedding provider: `OpenAI`
- API path: `SearchMemosSemantic`
- indexing mode: async refresh on memo create/update/delete

Out of scope (for now):

- multi-provider runtime switching
- `pgvector` index migration before the defined performance gate

## 2. Runtime Config Priority

Semantic embedding config resolves in this order:

1. Admin UI (`Settings -> AI`)
   - `openai_base_url`
   - `openai_embedding_model`
   - `openai_api_key` (stored as encrypted value)
2. Environment fallback (when UI field is empty)
   - `MEMOS_OPENAI_BASE_URL`
   - `MEMOS_OPENAI_EMBEDDING_MODEL`
   - `MEMOS_OPENAI_API_KEY`

Security notes:

- API key is stored in `system_setting` as ciphertext (`enc:v1:*` payload format).
- `GetInstanceSetting` never returns plaintext API keys.

## 3. API Key Rotation

Use this process when rotating OpenAI credentials:

1. Login as admin and open `Settings -> AI`.
2. Replace API key and save.
3. Verify:
   - semantic query succeeds from UI/API;
   - DB row `system_setting.name='AI'` still stores `openaiApiKeyEncrypted` with `enc:v1:` prefix;
   - plaintext key is not returned from API responses.
4. Keep the previous key available until the new key is confirmed.

Server secret dependency:

- encrypted values depend on server secret (`s.Secret`).
- if server secret is rotated, old ciphertext cannot be decrypted.
- recovery path: re-enter API key in `Settings -> AI` so it can be re-encrypted with the new secret.

## 4. Failure Triage

### `semantic search only supports postgres driver`

- Cause: non-postgres runtime (`sqlite`/`mysql`).
- Action: run semantic workloads with `MEMOS_DRIVER=postgres`.

### `semantic search is not configured`

- Cause: no valid OpenAI API key/base URL/model available from UI or env fallback.
- Action: set values in `Settings -> AI` first; use env fallback only for bootstrap.

### `failed to generate query embedding`

- Cause: upstream OpenAI request failed (auth/network/model/rate-limit).
- Action:
  - verify key and model;
  - verify network egress from server to OpenAI base URL;
  - inspect API error message in server logs.

### Background sync warnings (`failed to refresh memo embedding`)

- Cause: embedding refresh failure in async indexing path.
- Action:
  - inspect warn logs by memo ID;
  - update memo content (or re-save) to trigger re-index;
  - verify API key/model configuration.

## 5. Performance Gate and Benchmark

Latency target (10k corpus baseline):

- `p95 < 500ms`

Run benchmark from repo root:

```bash
DRIVER=postgres scripts/benchmark-semantic-search.sh
```

Optional knobs:

- `BENCHTIME` (default: `30x`)
- `COUNT` (default: `1`)

Example:

```bash
DRIVER=postgres BENCHTIME=50x COUNT=3 scripts/benchmark-semantic-search.sh
```

Escalation rule:

- keep current app-layer ranking unless either:
  - corpus size grows beyond `50k` memos, or
  - benchmark `p95 >= 500ms`.
- when triggered, open a task to evaluate `pgvector` index path.

## 6. Weekly Ops Checklist

1. Run staging benchmark with production-like corpus distribution:
   `DRIVER=postgres NOTE="staging weekly run" scripts/benchmark-semantic-search-trend.sh`.
2. Confirm new row is appended in `docs/ai-semantic-search-benchmark-trend.md`.
3. Record `ns/op`, `p50_ms`, `p95_ms`, `p99_ms` in tracker.
4. Compare trend against last run and flag regressions.
5. If gate is breached, create optimization task and link to benchmark evidence.
