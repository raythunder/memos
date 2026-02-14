# AI Semantic Search Tracker

Last updated: 2026-02-14

## 1. Status Board

| Milestone | Status | Target Date | Owner | Notes |
| --- | --- | --- | --- | --- |
| M0 Contracts and skeleton | TODO | 2026-02-16 | @raythunder | proto + service skeleton |
| M1 Storage + embedding pipeline | TODO | 2026-02-19 | @raythunder | postgres migration + async jobs |
| M2 Semantic search API | TODO | 2026-02-22 | @raythunder | retrieval + ACL filtering |
| M3 Frontend integration | TODO | 2026-02-24 | @raythunder | search mode switch |
| M4 Performance hardening | TODO | 2026-02-26 | @raythunder | 10k benchmark + tuning |

Status enum:

- `TODO`
- `IN_PROGRESS`
- `BLOCKED`
- `DONE`

## 2. Current Sprint Focus

- Sprint goal: Establish backend contract and storage foundation for semantic search.
- Sprint scope:
  - define proto
  - add postgres migration
  - add embedding task flow

## 3. Task Checklist

### Backend

- [ ] Add `SearchMemosSemantic` RPC in `proto/api/v1/memo_service.proto`
- [ ] Regenerate protobuf code (`proto/gen`, `web/src/types/proto`)
- [ ] Add API handler in `server/router/api/v1/memo_service.go`
- [ ] Add store model and methods for embedding rows
- [ ] Add PostgreSQL migration scripts

### Embedding Pipeline

- [ ] Add OpenAI embedding client (HTTP + timeout + retry)
- [ ] Add queue/runner for async embedding update
- [ ] Wire create/update/delete memo hooks
- [ ] Add content hash dedupe

### Frontend

- [ ] Add semantic mode in `SearchBar`
- [ ] Add semantic query hook in `web/src/hooks/useMemoQueries.ts`
- [ ] Add semantic result rendering and fallback states

### Testing

- [ ] Service tests for ACL and visibility
- [ ] Store tests for embedding CRUD
- [ ] Integration smoke tests for semantic endpoint

## 4. Decision Log

| Date | Decision | Rationale | Impact |
| --- | --- | --- | --- |
| 2026-02-14 | Provider uses OpenAI | Fastest delivery for current scope | Requires API key management |
| 2026-02-14 | Primary DB uses PostgreSQL | Matches production target | Enables better vector scaling path |
| 2026-02-14 | Keep keyword and semantic APIs separate | Reduce coupling and regression risk | Adds one new endpoint |

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
