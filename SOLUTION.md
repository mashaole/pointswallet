# SOLUTION.md

Design rationale, tradeoffs, threat model, and AI prompt history for Points Wallet.

**Implementation plan:** [PLAN.md](PLAN.md) — flow-first design document, updated to reflect the current codebase (routes, schema, tests, status).

## Overview

Single Go HTTP service implementing a loyalty points wallet with PostgreSQL persistence, layered architecture, JWT session auth, and async batch CSV processing.

## Plan status (assignment tasks)

| Task | Status | How to verify |
|------|--------|---------------|
| **1 — Wallet** | Done | Postman **03 Wallet** + **06 Negative Cases**; `go test ./internal/service/wallet/...` |
| **2 — Auth & RBAC** | Done | Admin/member login, logout (204), admin-only `POST /accounts`, member **403** |
| **3 — Batch CSV** | Done | Postman **05 Batch**; audit lists all rows including rejects |
| **Account CRUD** | Done | List/update/soft-delete; `PATCH`/`DELETE` admin + member |
| **Header idempotency** | Done | `Idempotency-Key` on API transactions |
| **Ledger audit** | Done | `actor_account_id`, `direction`, and `kind` on ledger/transaction responses; Postman **04 Admin Transactions** |
| **Unit tests** | Done | `go test ./...` |
| **Postman collection** | Done | Single import file + `postman/README.md` |
| **Docs** | Done | `PLAN.md`, `README.md`, `SOLUTION.md` |
| **Code hygiene** | Done | `staticcheck ./...` clean; unused DAO/helpers removed |

## Architecture

**Pattern:** Router → Controller → Service → DAO → Models (ports/adapters at DAO boundary).

- **Controllers** decode/validate HTTP, map errors to status codes
- **Services** hold auth flows and input validation; wallet service delegates financial writes to DAO
- **DAOs** own SQL and business invariants on the write path: `ApplyTransaction` uses `SELECT … FOR UPDATE`, duplicate-`ref` check, **non-negative balance on spend**, ledger INSERT, and balance UPDATE in one DB transaction

**Why:** Clear test boundaries; batch and API share `wallet.Service.ApplyTransaction` → `WalletDAO.ApplyTransaction` for one idempotency and balance implementation.

## Balance & spend rules

Every earn/spend/adjustment goes through `WalletDAO.ApplyTransaction` (API, admin adjust, and batch CSV).

| `kind` | Effect | Overdraft check |
|--------|--------|-----------------|
| `earn` | Adds points | N/A |
| `adjustment` | Adds or subtracts points (admin) | **Debit** rejected if balance insufficient |
| `spend` | Subtracts points | **Rejected** if `balance - points < 0` |

**Spend enforcement (application):** After locking the account row, spend computes `current.Sub(delta)`; if the result is negative, returns `ErrInsufficientBalance` **before** any ledger insert or balance update. The DB transaction rolls back — no partial write.

**Spend enforcement (database):** `accounts.balance_points` has `CHECK (balance_points >= 0)` as defense in depth.

**API:** `insufficient_balance` → **422**. Postman **06 Negative Cases → Insufficient Balance** covers this.

Spending the **exact** balance (result `0`) is allowed. `kind: spend` and `adjustment` with `direction: debit` can reduce balance.

**API `direction` (required):** Every transaction request must include `direction`. Must match kind: `earn` → `credit`, `spend` → `debit`, `adjustment` → `credit` or `debit`. Omission → **400** `validation_error`.

**Ledger audit fields:** Every entry stores `kind`, **`direction`** (`credit` | `debit`), and **`actor_account_id`**. For admin adjustments, `account_id` is the member wallet and `actor_account_id` is the admin (JWT `sub`). Look up admin name/email via `GET /accounts/{actor_account_id}` if needed — not duplicated on the ledger row.

## Points storage

- DB columns: `balance_points`, `ledger_entries.points` (point-cents, scale ×100)
- API/CSV: whole integer `points` (assignment-compatible)
- Go: `type Points int64` with `PointsFromWhole` / `WholePoints`

Never use floats. Fractional points can be added later by extending API parsing without schema changes.

## Auth

- HS256 JWT with claims: `sub` (account_id), `role`, `jti`, `exp`
- **`auth_tokens` table** stores one row per login: `account_id`, `token` (JWT `jti`), `expires_at`, optional `revoked_at`
- Multiple rows per user are allowed by schema; set `SINGLE_ACTIVE_SESSION=false` to permit concurrent sessions
- Default `SINGLE_ACTIVE_SESSION=true` revokes other tokens on login; password reset always revokes all tokens
- **`POST /auth/logout`** — single endpoint for admin and member; validates bearer token, sets `revoked_at`, returns **204 No Content**

## Database bootstrap & seeding

| When | What runs | Data created |
|------|-----------|--------------|
| `docker compose up -d` | Postgres container only | Empty database (no schema) |
| `go run ./cmd/server` startup | `RunMigrations` → `SeedAdminIfMissing` | Schema + bootstrap **admin** only |

- Admin credentials from env: `ADMIN_ACCOUNT_ID`, `ADMIN_EMAIL`, `ADMIN_PASSWORD` (see `.env.example`)
- Seed is **idempotent**: if `account_id` already exists, skip insert
- **Members are not seeded** — created at runtime via `POST /accounts` (admin-only)
- Config loads `.env` from repo root automatically (`config.Load` → `loadEnvFile`)

## API errors

Standard envelope: `{ "error": { "code", "message", "status" } }`.

| Code | HTTP | When |
|------|------|------|
| `duplicate_ref` | 409 | Global `ref` already in ledger |
| `insufficient_balance` | 422 | Spend exceeds balance |
| `account_already_exists` | 409 | Duplicate `account_id` on create |
| `email_already_exists` | 409 | Duplicate email on create |
| `forbidden` | 403 | Wrong role (e.g. member calling admin route) |
| `unauthorized` | 401 | Missing/invalid/revoked token |
| `validation_error` | 400 | DTO sanitize/validate failure |
| `not_found` | 404 | Unknown account or batch job |
| `last_admin` | 409 | Cannot delete or demote the last admin |

Mapped in `controller.MapDomainError`; Postgres unique violations normalized in `internal/dao/postgres/pgerrors.go`.

## API idempotency (REST)

| Source | Used for |
|--------|----------|
| `Idempotency-Key` header | Preferred for `POST /transactions` and admin adjust |
| JSON `ref` | Fallback when header omitted (backwards compatible) |
| CSV `ref` column | Batch uploads only |

Resolution (`dto.ResolveTransactionRef`): if both header and body are set, they must match; stored value is always the ledger `ref`. Postman generates UUIDs via pre-request script on spend/admin; earn and duplicate-negative tests use fixed `tx-001`.

## Account lifecycle (soft delete)

- **`deleted_at`** column marks removed accounts; rows stay in DB so ledger, batch audit, and FK references are never orphaned.
- Active queries filter `deleted_at IS NULL`. Deleted accounts return **404** on login and API access.
- On delete: email anonymized (`deleted+{id}+…@deleted.invalid`) to free the address; all sessions revoked.
- **Last admin** cannot be deleted or demoted to member (**409** `last_admin`).

| Action | Admin | Member |
|--------|-------|--------|
| Update name/email | `PATCH /accounts/{id}` | `PATCH /accounts/me` |
| Update role | `PATCH /accounts/{id}` | — |
| Delete | `DELETE /accounts/{id}` | `DELETE /accounts/me` |

**Financial audit (ledger):** Every earn/spend/adjustment stores `account_id` (wallet) and `actor_account_id` (who ran it). Ledger and transaction responses expose both — use `kind: adjustment` for admin point changes; `actor_account_id` differs from `account_id` when an admin adjusts a member.

## Concurrency

Per-account row lock (`SELECT … FOR UPDATE`) serializes concurrent earns/spends on the same account. Duplicate `ref` is checked inside the same transaction (not via a separate `RefExists` fast path). Batch worker pool processes rows in parallel; different accounts proceed concurrently.

## Batch

- `POST /batch/transactions` → 202 + `batch_job_id`
- Worker pool (`BATCH_WORKER_COUNT`, default 8)
- Every row logged to `audit_events` (accepted + rejected)
- Startup: `processing` jobs → `failed` (safe re-upload via idempotent refs)

## Tradeoffs

| Choice | Rationale |
|--------|-----------|
| PostgreSQL + Compose | Durable, row-level locking, realistic for prod |
| Stdlib HTTP | Transparent, no framework magic |
| Async batch | Large files don't block HTTP; client polls rather than websockets as its a short process |
| Denormalized `balance_points` | Fast reads; ledger is system of record |
| Balance check in DAO | Single transactional path; `FOR UPDATE` + in-txn check avoids race with concurrent spends |
| `direction` on ledger + API | Explicit credit/debit for audit; required on REST body; stored in `ledger_entries.direction` (`003` migration) |
| `actor_account_id` only (no denormalized actor profile) | Admin identity is a FK-style id on the ledger row; name/email resolved via `GET /accounts/{id}` when accounting needs it |
| `auth_tokens` not `sessions` | Same semantics; name matches migration table |

## Threat model (STRIDE-lite)

- **Spoofing:** JWT + session validation; bcrypt passwords
- **Tampering:** Parameterized SQL; RBAC middleware
- **Repudiation:** Immutable ledger (`actor_account_id` on every entry, exposed in API) + batch audit events
- **DoS:** Rate limits, body size caps, gzip decompressed size cap
- **Elevation:** Admin-only routes; role in JWT + DB

## AI prompts used

All prompts below were used with Cursor during planning and implementation. The list starts from the assignment brief and plan; each entry notes what it changed in the design or code.

### 1 — Plan from requirements

> Project requirements PDF — create a logical plan; start with user flows before database schema

**Effect:** Plan structured as user flows → data model → API; three assignment tasks mapped explicitly.

### 2 — Postgres + cents + Compose

> store money as cents , lets use postgres and ets use composer for it

**Effect:** PostgreSQL via Docker Compose; integer point-cents storage (no floats).

### 3 — Email validation

> lets make sure the emails are unique and also validate in the backend and databse that email follows a valid email format

**Effect:** Go `net/mail` validation + Postgres `UNIQUE` + `CHECK` on normalized email.

### 4 — Ports/adapters + connection pool

> for dependencies lets use a adapter and ports pattern in case we want to change the database or logger in future , and lets use pooling for database connections but limit the the pooled connections

**Effect:** DAO interfaces in `internal/dao/`; Postgres impl behind them; `SetMaxOpenConns` / `SetMaxIdleConns` from env.

### 5 — Layered architecture

> lets separate the router controller models and dao,functionality has to be easily maintainable and reusable and testable

**Effect:** Router → Controller → Service → DAO → Models; services reused by API and batch.

### 6 — Immutable ledger

> there should be a immutable ledger that has all the actions for each transaction

**Effect:** Append-only `ledger_entries`; INSERT-only DAO; no UPDATE/DELETE on ledger rows.

### 7 — Middleware + validation + errors

> lets add rate limiting on endpoints , and only allows specific methods , also add a middleware across necessary endpoints to check token and check role as some endpoints should only be authrized for admin etc and add validation before going into the rest of the logic of the endpoint , i would also prefer standardized status codes with clear message

**Effect:** Rate limit, method allowlist, JWT + RBAC middleware, DTO sanitize/validate in controller, `{ error: { code, message, status } }` envelope.

### 8 — Pagination

> any of the list apis should have pagination with a default limit of 20 unless limit is specified to be more

**Effect:** `limit` default 20, max 100; `{ data, pagination }` on ledger and audit lists.

### 9 — Testing strategy

> add unit tests to make sure units and mocks function as expected , for integration tests lets make use of the postman test run and variables to test all end points and some of their edge cases , this must be negative and postive test cases that i can import into postman

**Effect:** Go unit tests with hand-written mock DAOs; Postman collection with positive/negative folders, collection variables, and test scripts on login and negative cases.

### 10 — Request body sanitization

> make sure that request body is sanitized

**Effect:** Strict JSON decode (`DisallowUnknownFields`) → `Sanitize()` → `Validate()` before service layer.

### 11 — Compression

> compress payload so that siz e of payload is not large

**Effect:** Gzip response middleware; optional gzip request decompression with decompressed size cap.

### 12 — Async batch + concurrency + atomicity

> the batching processing should be asynch unless each row depends on the next , else this will effect the effeciency of running batches, we can make us of concurrency here , also apply transactions where atomicity is required else rollback

**Effect:** `POST /batch/transactions` → 202; worker pool; per-row DB transaction with rollback on failure.

### 13 — Global idempotency

> each transaction is idemponent wether batch or single transation , to ensure theres no duplicated transactions

**Effect:** Global `UNIQUE(ref)` on `ledger_entries`; single `wallet.Service.ApplyTransaction` for API + batch.

### 14 — Header idempotency key

> use Idempotency-Key header for API transactions; Postman auto-generates UUID; duplicate still testable

**Effect:** `Idempotency-Key` header maps to ledger `ref` (header preferred; body `ref` fallback; **400** if both differ). Postman: fixed `tx-001` on earn + duplicate test; `{{$guid}}` pre-request on spend/admin. Batch CSV unchanged.

### 15 — Gap analysis

> Have I missed anything in the requirements spec?

**Effect:** Identified batch audit (all rows), async poll flow, stale job recovery, assignment-compatible README examples.

### 16 — Close plan gaps

> lets cover the missed gaps except the video , and the soultion md will be on going , and skip submission logistics

**Effect:** Audit all batch rows; `RecoverStaleJobs()` on startup; ongoing `SOLUTION.md`; no video deliverable.

### 17 — Admin creates member vs admin

> there should be a way for a admin to create a new admin and to distinguish betwen when creating a memeber or admin

**Effect:** `POST /accounts` with `role: "member" | "admin"` (default `member`); admin-only route; tests for each role.

### 18 — Consistent naming

> theres tables that have different names for the same thing , we using points unless we using amounts somehwere lets rename that column to points so that its easier to document and follow

**Effect:** Columns `points`, `balance_points`, `balance_after_points` (replacing mixed `amount_cents` / `balance_cents` names).

### 19 — Point-cents storage confirmed

> we still store the points in cents right incase of decimales etc

**Effect:** Values stored as point-cents (×100 scale); API/CSV use whole integer `points`; conversion at boundary via `PointsFromWhole` / `WholePoints`.

### 20 — Implementation

> go implement

**Effect:** Full codebase scaffolded and built from the plan.

### 21 — Single Postman file

> the postman should be a single file not separate so thatt i import once

**Effect:** One `postman/PointsWallet.postman_collection.json` with collection-scoped variables; no separate environment file.

### 22 — Postman walkthrough doc

> add postman readme with full setup and test order

**Effect:** `postman/README.md` — terminal setup, import, folder run order, batch CSV attachment, Collection Runner tips, troubleshooting.

### 23 — Implementation fixes (manual testing)

Issues found during Postman/curl testing and fixed in code:

| Issue | Fix |
|-------|-----|
| Admin login **400** `validation_error: email` | `decodeAndValidateJSON` now validates **after** JSON decode (not on empty struct) |
| `.env` ignored when not exported | `config.Load` reads `.env` from repo root via `loadEnvFile` |
| Duplicate **Create Member** → **500** | Map Postgres `23505` on `accounts_pkey` / `accounts_email_unique` → **409** (`pgerrors.go`) |
| Logout looked admin-only in Postman | Split into **Logout (Admin)** and **Logout (Member)**; same `POST /auth/logout` |
| Negative tests showed **401** in Runner | Document: logout revokes token; run negative folder after earn, **before** logout |
| Negative tests “failed” on correct errors | Postman test scripts assert expected **409/422/403** and `error.code` |

### 24 — Verification

> run unit tests and e2e smoke test

**Effect:** `go test ./...`; curl/Postman happy path + negative cases confirmed.

### 25 — Account update and soft delete

> admin/member edit accounts; soft delete to avoid orphaned ledger data

**Effect:** `PATCH`/`DELETE` on `/accounts/{id}` (admin) and `/accounts/me` (member); `deleted_at` migration; last-admin guard; email anonymized on delete.

### 26 — List accounts (admin)

> admin endpoint to see all accounts

**Effect:** `GET /accounts` with pagination; active accounts only.

### 27 — Ledger actor exposure

> ledger-only audit first — show who made adjustments

**Effect:** `actor_account_id` in ledger/transaction JSON; admin adjust uses `kind: adjustment`.

### 28 — Gzip middleware fix

> garbled ledger response in Postman

**Effect:** Buffer response before compressing so `Content-Encoding: gzip` is set correctly.

### 29 — Remove dead code

> remove any redundant or unused code

**Effect:** Removed unused `LedgerDAO.RefExists`, `BatchDAO.FailJob`, unused error sentinels/helpers, duplicate `writeError` wrapper; `staticcheck ./...` clean.

### 30 — Admin adjustment credit and debit

> I want adjustment to support adjust in and out (credit and debit) with a clear audit trail on the ledger, without breaking existing clients.

**Effect:** Kept single `kind: adjustment` (assignment only defines `earn` | `spend`; `adjustment` is our extension). Added **`direction: credit | debit`** on the API and **`ledger_entries.direction`** (`migrations/003_ledger_direction.sql`). Credit adds points; debit subtracts with the same insufficient-balance rules as spend. Batch CSV: optional 6th column `direction`; earn/spend rows still infer direction from kind in 5-column files.

**Why not two kinds (`adjustment_credit` / `adjustment_debit`)?** Would alter the `kind` CHECK and break existing `adjustment` rows. **Why not negative `points`?** Conflicts with `CHECK (points > 0)` and assignment-style positive integers.

### 31 — Record which admin performed an adjustment

> For auditing and accounting we need to know which admin did a specific adjustment.

**Effect:** Already satisfied by **`actor_account_id`** on every ledger row — set from JWT `sub` on `POST /accounts/{id}/transactions` (admin route) and from the uploading admin on batch. **`account_id`** = member wallet affected; **`actor_account_id`** = who acted. Members cannot POST `kind: adjustment` on `POST /transactions` (**403**).

**Rejected:** Duplicating admin `name` / `email` on each ledger row or nested `actor` object in JSON — `actor_account_id` is enough; lookup via accounts API when needed.

### 32 — Require `direction` on transaction endpoints

> Make `direction` required on the endpoint from now on.

**Effect:** `TransactionRequest` validation requires `direction` on **`POST /transactions`** and **`POST /accounts/{id}/transactions`**. Rules: `earn` → `credit`, `spend` → `debit`, `adjustment` → `credit` or `debit`; mismatch or omission → **400** `validation_error`. Postman collection and README curl examples updated. Unit tests in `internal/models/direction_test.go` and `internal/service/wallet/service_test.go`.

**Breaking change:** Clients that omit `direction` must send it explicitly. Restart server so migration **003** runs before testing.

---

## Testing summary

| Layer | Command / tool | Scope | Status |
|-------|----------------|-------|--------|
| Unit | `go test ./...` | Points math; DTO idempotency; **direction validation**; wallet duplicate ref + insufficient balance + adjustment debit; gzip middleware | Partial (auth/batch/controller mocks not yet extracted) |
| Static analysis | `staticcheck ./...` | Unused code, common bugs | Done |
| Integration | Postman collection | Full API: auth, RBAC, wallet, accounts, batch, negative cases | Done |
| Manual | curl examples in README | Same flows without Postman | Done |

Postman **04 Admin Transactions** asserts `actor_account_id`, `kind: adjustment`, and `direction` on admin credit. **06 Negative Cases** expects **Earn Points** first with `Idempotency-Key: tx-001` and `"direction": "credit"`; **Duplicate Ref** replays the same header.

---

**How I used AI:** I shared the requirements PDF with Cursor and asked for a flow-first plan (prompt 1), captured in [PLAN.md](PLAN.md) and kept in sync as the codebase evolved. I iterated on architecture, security, batch, and naming before `go implement`, reviewed diffs before execution, validated with Postman and curl, and kept the stack explainable (stdlib HTTP, explicit layers, no ORM).
