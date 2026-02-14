# AI Semantic Search Plan (Memos)

Last updated: 2026-02-14
Owner: @raythunder

## 1. Scope and Constraints

This plan is fixed by current decisions:

- Embedding provider: `OpenAI`
- Data size target: `10k memos`
- Privacy policy: `external API allowed`
- Primary database: `PostgreSQL`

Out of scope for this phase (YAGNI):

- Multi-provider abstraction for more than one production provider on day 1
- Reranker and hybrid BM25 + vector orchestration
- LLM answer generation over search results

## 2. Iteration Goal

Add semantic retrieval for fragmented notes while preserving existing keyword search behavior.

Measurable outcomes:

- Semantic search API returns relevant top-N memo results for natural language queries.
- P95 search latency < 500ms on 10k memos (single instance baseline).
- Existing `ListMemos` behavior is unchanged.

## 3. Technical Design (MVP)

### 3.1 Retrieval Pipeline

1. User sends semantic query text.
2. Backend generates query embedding through OpenAI.
3. Backend retrieves top-K candidates from `memo_embedding` in PostgreSQL.
4. Backend applies existing visibility/permission constraints.
5. Backend returns memo results sorted by similarity score.

### 3.2 Storage Design

Add table `memo_embedding`:

- `memo_id` (FK to `memo.id`, unique)
- `model` (text)
- `dimension` (int)
- `embedding` (float vector storage; MVP can use `float4[]` in Postgres)
- `content_hash` (text, dedupe by content)
- `updated_ts` (bigint)

Indexing strategy:

- Start simple with sequential score calc at 10k scale.
- Evaluate `pgvector` index in Phase 2 if latency needs optimization.

### 3.3 Write Path

Embedding update is async (KISS + stability):

- On create/update content: enqueue embedding refresh task.
- On delete memo: remove embedding row.
- Use `content_hash` to skip duplicate re-embedding.

### 3.4 API Design

Add dedicated endpoint (do not overload `ListMemos`):

- `SearchMemosSemantic(request)`
- Request fields:
  - `query`
  - `page_size`
  - `page_token`
  - optional `filter`
- Response type:
  - Reuse `ListMemosResponse` for MVP (`memos`, `next_page_token`)

Reasoning:

- Keeps keyword and semantic responsibilities separated (SRP).
- Avoids breaking existing clients (OCP).
- Avoids introducing extra response schema during MVP (KISS).

### 3.5 Runtime Configuration

Current implementation supports two configuration paths:

1. Admin frontend setting page (`Settings -> AI`)
   - `openai_base_url`
   - `openai_embedding_model`
   - `openai_api_key` (write-only in API, encrypted before persistence)
2. Environment variable fallback (backward compatibility):
   - `MEMOS_OPENAI_API_KEY`
   - `MEMOS_OPENAI_EMBEDDING_MODEL` (default: `text-embedding-3-small`)
   - `MEMOS_OPENAI_BASE_URL` (default: `https://api.openai.com/v1`)

Security note:

- OpenAI API key is persisted as ciphertext in `system_setting` using server-side encryption (`enc:v1:*` payload format).
- API key is never returned in plaintext from `GetInstanceSetting`.

## 4. Milestones and Checkpoints

### M0 - Baseline and Contracts

Deliverables:

- `.proto` API contract for semantic search
- service/interface skeleton
- plan/tracker docs in repo

Acceptance:

- `buf generate` passes
- backend compiles without feature behavior changes

### M1 - Storage and Embedding Pipeline

Deliverables:

- PostgreSQL migration for `memo_embedding`
- store methods for upsert/delete/list candidate embeddings
- OpenAI embedding client (stdlib HTTP only)
- async job processing for embedding updates

Acceptance:

- create/update/delete memo triggers correct embedding side effects
- retry + error log behavior is verifiable

### M2 - Semantic Search Endpoint

Deliverables:

- `SearchMemosSemantic` backend implementation
- permission/visibility filtering integrated with existing rules
- pagination + deterministic ordering

Acceptance:

- top-N semantic results are stable
- no visibility leakage across users

### M3 - Frontend Integration

Deliverables:

- search mode switch: `keyword` / `semantic`
- semantic query UI and result rendering
- loading/error/empty states

Acceptance:

- user can run semantic search from current app UI
- keyword mode remains unchanged

### M4 - Performance and Hardening

Deliverables:

- baseline performance report on 10k memos
- optional `pgvector` optimization decision record
- operational configs and runbook

Acceptance:

- p95 target reached or optimization decision documented

Current baseline (2026-02-14):

- `DRIVER=postgres` local benchmark reports `p95=152.4ms` at 10k corpus.
- Detailed command/result: `docs/ai-semantic-search-benchmark.md`.
- Trend history: `docs/ai-semantic-search-benchmark-trend.md`.
- Operations runbook: `docs/ai-semantic-search-operations.md`.

## 5. Engineering Principles Mapping

- KISS: introduce one dedicated semantic endpoint and one embedding table.
- YAGNI: no premature multi-provider/reranker/agent features.
- DRY: reuse existing auth/visibility/filter paths and memo conversion.
- SOLID:
  - SRP: split embed client, index updater, and search handler.
  - OCP: provider implementation behind interface.
  - DIP: service depends on embedding interface, not concrete SDK.

## 6. Risks and Mitigations

- OpenAI API latency/rate limits:
  - async indexing + retry with backoff.
- Cost growth:
  - hash-based skip re-embedding; batch backfill with throttling.
- Permission leakage risk:
  - apply existing memo visibility checks before response.
- Embedding model drift:
  - persist `model` and `dimension` in table for migration-safe upgrades.

## 7. Definition of Done (Current Epic)

- Semantic search available in production path.
- Docs in sync (`README` + this plan + tracker).
- Tests added for:
  - service-level permission constraints
  - embedding lifecycle on memo CRUD
  - search relevance smoke cases
