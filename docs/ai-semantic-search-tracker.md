# AI Semantic Search Tracker

Last updated: 2026-02-14

## 1. Status Board

| Milestone | Status | Target Date | Owner | Notes |
| --- | --- | --- | --- | --- |
| M0 Contracts and skeleton | DONE | 2026-02-16 | @raythunder | proto + service skeleton |
| M1 Storage + embedding pipeline | DONE | 2026-02-19 | @raythunder | postgres migration + async embedding jobs + tests |
| M2 Semantic search API | DONE | 2026-02-22 | @raythunder | retrieval + ACL filtering + integration tests |
| M3 Frontend integration | DONE | 2026-02-24 | @raythunder | semantic mode/hook/error states + admin AI settings |
| M4 Performance hardening | IN_PROGRESS | 2026-02-26 | @raythunder | 10k benchmark baseline done (p95 152.4ms), tuning gate defined |

Status enum:

- `TODO`
- `IN_PROGRESS`
- `BLOCKED`
- `DONE`

## 2. Current Sprint Focus

- Sprint goal: Complete performance baseline and operational documentation for semantic search.
- Sprint scope:
  - add reproducible 10k benchmark harness
  - capture p95 baseline and optimization gate
  - keep README/plan/tracker docs synchronized

## 3. Task Checklist

### Backend

- [x] Add `SearchMemosSemantic` RPC in `proto/api/v1/memo_service.proto`
- [x] Regenerate protobuf code (`proto/gen`, `web/src/types/proto`)
- [x] Add API handler in `server/router/api/v1/memo_service.go`
- [x] Add store model and methods for embedding rows
- [x] Add PostgreSQL migration scripts

### Embedding Pipeline

- [x] Add OpenAI embedding client (HTTP + timeout + retry)
- [x] Add queue/runner for async embedding update
- [x] Wire create/update/delete memo hooks
- [x] Add content hash dedupe

### Frontend

- [x] Add admin AI setting page (`Settings -> AI`) for OpenAI config
- [x] Add semantic mode in `SearchBar`
- [x] Add semantic query hook in `web/src/hooks/useMemoQueries.ts`
- [x] Add semantic result rendering and fallback states

### Testing

- [x] Service tests for ACL and visibility
- [x] Service tests for AI setting security (admin-only + API key no echo)
- [x] Store tests for embedding CRUD
- [x] Integration smoke tests for semantic endpoint

### Performance

- [x] Add 10k semantic search benchmark (`BenchmarkSearchMemosSemanticPostgres10k`)
- [x] Record baseline metrics (`p50/p95/p99`) in local benchmark doc
- [x] Document optimization gate for future `pgvector` adoption

## 4. Decision Log

| Date | Decision | Rationale | Impact |
| --- | --- | --- | --- |
| 2026-02-14 | Provider uses OpenAI | Fastest delivery for current scope | Requires API key management |
| 2026-02-14 | Primary DB uses PostgreSQL | Matches production target | Enables better vector scaling path |
| 2026-02-14 | Keep keyword and semantic APIs separate | Reduce coupling and regression risk | Adds one new endpoint |
| 2026-02-14 | AI config managed from frontend and encrypted at rest | Improve operability and secret safety | Adds `instance/settings/AI` contract and crypto helpers |
| 2026-02-14 | Add injectable embedding client factory in API service | Improve testability without real OpenAI dependency | Enables deterministic semantic integration tests |
| 2026-02-14 | Keep app-layer ranking for now; postpone pgvector | Current 10k baseline p95 is within target with margin | Avoids premature complexity; keep clear trigger for optimization |

## 5. Iteration Log

Use one entry per working session.

### Template

Date:
Owner:
What changed:
Files:
Verification:
Risks/blockers:
Next step:

### Entries

#### 2026-02-14

- Owner: @raythunder + Codex
- What changed: confirmed technical constraints and created local planning/tracking docs.
- Files:
  - `docs/ai-semantic-search-plan.md`
  - `docs/ai-semantic-search-tracker.md`
  - `README.md` (link section)
- Verification: document structure reviewed.
- Risks/blockers: none.
- Next step: start M0 (`proto` contract + backend skeleton).

#### 2026-02-14 (Implementation)

- Owner: @raythunder + Codex
- What changed:
  - Added `SearchMemosSemantic` RPC contract and generated protobuf/connect code.
  - Implemented semantic retrieval endpoint in API service.
  - Added Postgres `memo_embedding` storage access methods in store layer.
  - Added async embedding indexing hooks on memo create/update/delete.
  - Added Postgres migration and `LATEST.sql` table definition for memo embeddings.
  - Added API test for unsupported driver behavior.
- Files:
  - `proto/api/v1/memo_service.proto`
  - `server/router/api/v1/memo_semantic_service.go`
  - `server/router/api/v1/memo_list_helpers.go`
  - `server/router/api/v1/memo_service.go`
  - `store/memo_embedding.go`
  - `store/migration/postgres/0.26/03__memo_embedding.sql`
  - `store/migration/postgres/LATEST.sql`
- Verification:
  - `cd proto && buf generate`
  - `go test ./server/router/api/v1/... ./store/...`
  - `cd web && pnpm lint`
- Risks/blockers:
  - Semantic retrieval currently computes cosine ranking in app layer; performance tuning for larger datasets pending M4.
  - OpenAI API key is required for indexing and semantic query runtime.
- Next step:
  - Implement frontend semantic search mode and hook wiring (M3).

#### 2026-02-14 (AI config + encrypted key)

- Owner: @raythunder + Codex
- What changed:
  - Added `instance/settings/AI` in store/API proto contracts.
  - Added backend encryption/decryption helper for sensitive values.
  - Added admin-only AI setting read/write flow in instance service.
  - Changed semantic embedding config loading: frontend setting first, env fallback second.
  - Added settings page section `AI` to configure OpenAI base URL/model/API key.
  - Added tests for AI setting auth/no-echo/encryption behaviors.
- Files:
  - `proto/store/instance_setting.proto`
  - `proto/api/v1/instance_service.proto`
  - `store/instance_setting.go`
  - `server/router/api/v1/instance_service.go`
  - `server/router/api/v1/semantic_embedding_openai.go`
  - `server/router/api/v1/secret_crypto.go`
  - `server/router/api/v1/secret_crypto_test.go`
  - `server/router/api/v1/test/instance_service_test.go`
  - `web/src/pages/Setting.tsx`
  - `web/src/components/Settings/AISettings.tsx`
  - `web/src/contexts/InstanceContext.tsx`
- Verification:
  - `cd proto && buf generate`
  - `go test ./server/router/api/v1/... ./store/...`
  - `cd web && pnpm lint`
  - Browser manual test: login as admin -> `Settings -> AI` -> save/clear key
  - DB check: `system_setting.name='AI'` contains `openaiApiKeyEncrypted` with `enc:v1:` prefix
- Risks/blockers:
  - Key encryption currently depends on server secret; secret rotation policy should be documented in ops phase.
- Next step:
  - Implement semantic search mode in frontend search flow and connect to `SearchMemosSemantic`.

#### 2026-02-14 (Frontend semantic mode + hook wiring)

- Owner: @raythunder + Codex
- What changed:
  - Added `keyword/semantic` mode toggle to `SearchBar`.
  - Added `semanticSearch` filter factor in filter context and filter chips.
  - Added semantic infinite query hook (`searchMemosSemantic`) in memo query hooks.
  - Switched `PagedMemoList` data source by search mode:
    - keyword mode -> `ListMemos`
    - semantic mode -> `SearchMemosSemantic`
  - Kept semantic result ordering from backend relevance rank.
  - Added explicit list error state rendering (semantic/keyword query failures).
- Files:
  - `web/src/components/SearchBar.tsx`
  - `web/src/contexts/MemoFilterContext.tsx`
  - `web/src/hooks/useMemoFilters.ts`
  - `web/src/hooks/useMemoQueries.ts`
  - `web/src/components/PagedMemoList/PagedMemoList.tsx`
  - `web/src/components/MemoFilters.tsx`
  - `web/src/locales/en.json`
  - `web/src/locales/zh-Hans.json`
- Verification:
  - `cd web && pnpm lint`
  - `go test ./server/router/api/v1/... ./store/...`
  - Browser manual test:
    - keyword/semantic mode toggle visible
    - semantic search adds filter chip and switches list source
    - switching back to keyword clears semantic filter
  - Screenshot: `.tmp/dev-run/semantic-search-mode.png`
- Risks/blockers:
  - Local smoke test used SQLite, semantic API returns expected failed precondition (`semantic search only supports postgres driver`).
- Next step:
  - Run full semantic e2e with PostgreSQL + valid OpenAI key and add error-state UX message for provider/driver failures.

#### 2026-02-14 (Store embedding tests)

- Owner: @raythunder + Codex
- What changed:
  - Added store test coverage for `memo_embedding`:
    - non-postgres unsupported-driver behavior
    - postgres CRUD (upsert/update/list/delete/content_hash)
    - input validation (`nil` and empty vector)
- Files:
  - `store/test/memo_embedding_test.go`
- Verification:
  - `DRIVER=sqlite go test -v ./store/test/... -run TestMemoEmbeddingStore`
  - `DRIVER=postgres go test -v ./store/test/... -run TestMemoEmbeddingStore`
  - `go test ./server/router/api/v1/... ./store/...`
  - `cd web && pnpm lint`
- Risks/blockers:
  - semantic e2e for real relevance quality still depends on PostgreSQL + valid OpenAI key in runtime.
- Next step:
  - add integration smoke test for semantic endpoint in API layer (postgres path).

#### 2026-02-14 (Semantic endpoint integration tests)

- Owner: @raythunder + Codex
- What changed:
  - Added semantic endpoint integration tests on PostgreSQL path:
    - ranking correctness by cosine similarity
    - visibility isolation (other user's private memo not leaked)
    - configuration failure path returns `FailedPrecondition`
  - Added injectable embedding client factory in API service for deterministic tests.
  - Updated API test helper to respect `DRIVER` environment variable.
- Files:
  - `server/router/api/v1/semantic_embedding_openai.go`
  - `server/router/api/v1/memo_semantic_service.go`
  - `server/router/api/v1/v1.go`
  - `server/router/api/v1/test/test_helper.go`
  - `server/router/api/v1/test/memo_semantic_service_test.go`
- Verification:
  - `go test ./server/router/api/v1/... ./store/...`
  - `DRIVER=postgres go test -v ./server/router/api/v1/test -run TestSearchMemosSemanticPostgres`
- Risks/blockers:
  - still need production-like relevance evaluation with real OpenAI vectors on 10k dataset.
- Next step:
  - run benchmark and profiling on 10k memos (M4) and decide whether to switch to pgvector index.

#### 2026-02-14 (Performance baseline + benchmark harness)

- Owner: @raythunder + Codex
- What changed:
  - Refactored test helpers to use `testing.TB` so benchmark and tests share one initialization path.
  - Added `BenchmarkSearchMemosSemanticPostgres10k` with deterministic synthetic vectors.
  - Added benchmark percentile metrics output (`p50_ms`, `p95_ms`, `p99_ms`).
  - Added local benchmark runbook and result doc.
- Files:
  - `store/test/store.go`
  - `store/test/containers.go`
  - `server/router/api/v1/test/test_helper.go`
  - `server/router/api/v1/test/memo_semantic_benchmark_test.go`
  - `docs/ai-semantic-search-benchmark.md`
  - `README.md`
- Verification:
  - `go test ./server/router/api/v1/... -count=1`
  - `go test ./store/test/... -run TestMemoEmbeddingStore -count=1`
  - `DRIVER=postgres go test ./server/router/api/v1/test -run TestSearchMemosSemanticPostgres -count=1`
  - `DRIVER=postgres go test ./server/router/api/v1/test -run '^$' -bench '^BenchmarkSearchMemosSemanticPostgres10k$' -benchtime=30x -count=1`
- Risks/blockers:
  - benchmark currently uses synthetic vectors; online OpenAI vector distribution may vary slightly.
- Next step:
  - run the same benchmark in staging with production-like content distribution and track trend over time.

## 6. Local Manual Test Account

This account is for local development verification only.

- Username: `admin`
- Password: `Passw0rd!`
- Data directory: `.tmp/memos-dev`

Startup commands:

```bash
# backend (repo root)
MEMOS_DATA="$(pwd)/.tmp/memos-dev" go run ./cmd/memos --port 8081

# frontend (new terminal)
cd web
pnpm dev --host 127.0.0.1 --port 5173
```

Manual verification URL:

- `http://127.0.0.1:5173`

Smoke test evidence (2026-02-14):

- Registration/login succeeded with the account above.
- Create memo succeeded.
- Search query `agent-browser` returned the created memo.
- Screenshot: `.tmp/dev-run/agent-browser-home.png`
- Admin AI settings save/clear succeeded on `http://127.0.0.1:5173/setting#ai`.
- Screenshot: `.tmp/dev-run/ai-settings-page.png`
