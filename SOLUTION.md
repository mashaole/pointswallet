# SOLUTION.md

## Overview

Single Go HTTP service implementing a loyalty points wallet with PostgreSQL persistence, layered architecture, JWT session auth, and async batch CSV processing.

## Architecture

**Pattern:** Router → Controller → Service → DAO → Models (ports/adapters at DAO boundary).

- **Controllers** decode/validate HTTP, map errors to status codes
- **Services** hold business rules (idempotency, balance checks, auth flows)
- **DAOs** own SQL; `ApplyTransaction` uses `SELECT … FOR UPDATE` + ledger INSERT + balance UPDATE in one transaction

**Why:** Clear test boundaries; batch and API share `wallet.Service.ApplyTransaction` for one idempotency implementation.

## Points storage

- DB columns: `balance_points`, `ledger_entries.points` (point-cents, scale ×100)
- API/CSV: whole integer `points` (assignment-compatible)
- Go: `type Points int64` with `PointsFromWhole` / `WholePoints`

Never use floats. Fractional points can be added later by extending API parsing without schema changes.

## Auth

- HS256 JWT with claims: `sub` (account_id), `role`, `jti`, `exp`
- Session row per login; middleware validates JWT + active session
- Single active session: login revokes prior sessions
- Password reset revokes all sessions

## Concurrency

Per-account row lock serializes concurrent earns/spends on the same account. Batch worker pool processes rows in parallel; different accounts proceed concurrently.

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

## Threat model (STRIDE-lite)

- **Spoofing:** JWT + session validation; bcrypt passwords
- **Tampering:** Parameterized SQL; RBAC middleware
- **Repudiation:** Immutable ledger + batch audit events
- **DoS:** Rate limits, body size caps, gzip decompressed size cap
- **Elevation:** Admin-only routes; role in JWT + DB

## AI prompts used

All prompts below were used with Cursor during planning and implementation. The list starts from the assignment brief and plan; each entry notes what it changed in the design or code.

### 1 — Plan from assignment PDF

> @Sanlam+_+Senior+Software+Engineer+Assignment.pdf this is requirments , lets create a logical plan for it, and lets start with user flows before database schema

**Effect:** Plan structured as user flows → data model → API; three assignment tasks mapped explicitly.

### 2 — Postgres + cents + Compose

> store money as cents , lets use postgres and ets use composer for it

**Effect:** PostgreSQL via Docker Compose; integer point-cents storage (no floats).

### 3 — Email validation

> lets makesure the emails are unique and also validate in the backend and databse that email follows a valid email format

**Effect:** Go `net/mail` validation + Postgres `UNIQUE` + `CHECK` on normalized email.

### 4 — Ports/adapters + connection pool

> for dependecies lets use a adpter and ports pattern in case we want to change the database or logger in future , and lets use pooling for database connections but limit the the pooled connections

**Effect:** DAO interfaces in `internal/dao/`; Postgres impl behind them; `SetMaxOpenConns` / `SetMaxIdleConns` from env.

### 5 — Layered architecture

> lets separate the router controller models and doa,functionality has to be easily maintainable and reusable and testtable

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

> add unit tests to make sure units and mocks function as expected , for integration tests lets make use of the postman test run and variablesto test all end points and some of their edge cases , this must be negative and postuve test cases that i can import into postaman

**Effect:** Go unit tests with hand-written mock DAOs; Postman collection with positive/negative folders and collection variables.

### 10 — Request body sanitization

> make sure that request body is sterelized

**Effect:** Strict JSON decode (`DisallowUnknownFields`) → `Sanitize()` → `Validate()` before service layer.

### 11 — Compression

> compress payload

**Effect:** Gzip response middleware; optional gzip request decompression with decompressed size cap.

### 12 — Async batch + concurrency + atomicity

> the batching processing chould be asynch unless each row depends on the next , else this will effect the effeciency of running batches, we can make sure of concurrency here , also apply transactions where atomicity is required else rollback

**Effect:** `POST /batch/transactions` → 202; worker pool; per-row DB transaction with rollback on failure.

### 13 — Global idempotency

> each transaction is idemponent wether batch or single transation , to ensure theres no duplicated transactions

**Effect:** Global `UNIQUE(ref)` on `ledger_entries`; single `wallet.Service.ApplyTransaction` for API + batch.

### 14 — Gap analysis

> have i missed anything in this spec? @Sanlam+_+Senior+Software+Engineer+Assignment.pdf

**Effect:** Identified batch audit (all rows), async poll flow, stale job recovery, assignment-compatible README examples.

### 15 — Close plan gaps

> lets cover the missed gaps except the video , and the soultion md will be on going , and skip submission logistics

**Effect:** Audit all batch rows; `RecoverStaleJobs()` on startup; ongoing `SOLUTION.md`; no video deliverable.

### 16 — Admin creates member vs admin

> there should be a way for a admin to create a new admin and to distinguish betwen when creatig a memeber or admin

**Effect:** `POST /accounts` with `role: "member" | "admin"` (default `member`); admin-only route; tests for each role.

### 17 — Consistent naming

> theres tables that have different names for the same thing , we using points unless we using amounts somehwere lets rename that column to points so that its easier to document and follow

**Effect:** Columns `points`, `balance_points`, `balance_after_points` (replacing mixed `amount_cents` / `balance_cents` names).

### 18 — Point-cents storage confirmed

> we still store the points in cents right incase of decimales etc

**Effect:** Values stored as point-cents (×100 scale); API/CSV use whole integer `points`; conversion at boundary via `PointsFromWhole` / `WholePoints`.

### 19 — Implementation

> go implement

**Effect:** Full codebase scaffolded and built from the plan.

### 20 — Single Postman file

> the postman should be a single file not seperste so thatt i import once

**Effect:** One `postman/PointsWallet.postman_collection.json` with collection-scoped variables; no separate environment file.

---

**How I used AI:** I shared the requirments PDF with cursor and asked for a flow-first plan (prompt 1), then iterated on architecture, security, batch, and naming before `go implement`. I reviewed each plan diff before execution and kept the stack explainable (stdlib HTTP, explicit layers, no ORM).
