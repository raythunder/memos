# Supabase Backend Migration (Postgres + Hybrid Auth)

This guide describes how to run Memos backend on Supabase Postgres while keeping the existing Memos web sign-in flow and adding backend support for Supabase JWT.

## Scope

- Keep existing Memos Auth (`SignIn`, refresh cookie, PAT).
- Add Supabase JWT verification in backend auth middleware.
- Resolve Supabase identity to local Memos user via `user_external_identity`.

## Prerequisites

- Supabase project created.
- Postgres connection uses **Session Pooler** (`5432`).
- `Enable automatic RLS` remains disabled for this backend-direct model.

## Environment Variables

Required:

- `MEMOS_DRIVER=postgres`
- `MEMOS_DSN=postgres://postgres.<project-ref>:<password>@aws-0-<region>.pooler.supabase.com:5432/postgres?sslmode=require`

Optional but required for Supabase JWT auth:

- `MEMOS_SUPABASE_PROJECT_URL=https://<project-ref>.supabase.co`
- `MEMOS_SUPABASE_JWT_AUDIENCE=authenticated` (default: `authenticated`)

## Startup

```bash
MEMOS_DRIVER=postgres \
MEMOS_DSN="postgres://postgres.<project-ref>:<password>@aws-0-<region>.pooler.supabase.com:5432/postgres?sslmode=require" \
MEMOS_SUPABASE_PROJECT_URL="https://<project-ref>.supabase.co" \
go run ./cmd/memos --port 8081
```

Expected startup output includes:

- `Database driver: postgres`

## Identity Mapping Rules

Provider is fixed to `supabase`.

On first request authenticated by Supabase JWT:

1. Try mapping by `(provider=supabase, subject=sub)`.
2. If not found and email uniquely matches a local user, bind it.
3. If still not found, auto-create a local user and bind:
   - username: deterministic 32-char UID from `sub`
   - nickname: email local-part or `Supabase User`
   - email: from token claim when available
   - role: `ADMIN` only when system has no users; otherwise `USER`
4. If registration is disabled and this is not first-user bootstrap, auto-create is denied.

## Verification Checklist

1. Start backend with Supabase DSN and project URL.
2. Call a protected API with no token -> `Unauthenticated`.
3. Call protected API with valid Supabase JWT:
   - first call creates/binds local user.
   - second call reuses existing mapping.
4. Access private attachment endpoint with Supabase JWT.

## Rollback

To roll back to previous database backend behavior:

1. Remove `MEMOS_SUPABASE_PROJECT_URL` / `MEMOS_SUPABASE_JWT_AUDIENCE`.
2. Point `MEMOS_DSN` back to previous Postgres.
3. Restart service.
