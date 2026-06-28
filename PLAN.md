---
name: Points Wallet Plan
overview: Build a stdlib-first Go loyalty points wallet with layered architecture, PostgreSQL (Docker Compose), immutable ledger, JWT + auth_tokens, middleware hardening, Go unit tests with mocks, and an importable Postman collection for integration testing.
todos:
  - id: scaffold
    content: go.mod, docker-compose, migrations, models, DAO interfaces + postgres impl with pool caps, logger adapter, router/controller wiring in main
    status: completed
  - id: wallet-service
    content: service/wallet business logic (Points, ValidateEmail, FOR UPDATE via DAO) with mock-DAO unit tests
    status: completed
  - id: wallet-http
    content: controllers, router, middleware chain (compress, rate limit, methods, auth, RBAC), standardized errors, README curl examples
    status: completed
  - id: auth-sessions
    content: auth service + AuthController, JWT + auth_tokens, router middleware RBAC, single-session rule
    status: completed
  - id: auth-reset
    content: forgot/reset via AuthController; revoke all auth_tokens on reset
    status: completed
  - id: batch-csv
    content: Async batch worker pool, per-row DB transactions, BatchController 202 + GET job status, audit DAO
    status: completed
  - id: unit-tests
    content: Table-driven unit tests per layer with hand-written mocks (DAO, service); verify mocks and isolation with go test ./...
    status: completed
  - id: postman
    content: Importable Postman collection (collection-scoped variables); positive/negative cases; README for Collection Runner
    status: completed
  - id: docs
    content: README (run, async batch flow, assignment JSON examples, go test, Postman); SOLUTION.md ongoing as built
    status: completed
  - id: assignment-gaps
    content: Batch audit all rows accepted+rejected; startup recovery for stale processing jobs; README assignment-compat examples
    status: completed
isProject: false
---

> **Living plan** — started as the flow-first Cursor design doc; **updated to match the current codebase** (routes, schema, Postman, tests). For design rationale, threat model, and AI prompt history, see [SOLUTION.md](SOLUTION.md). For setup and run instructions, see [README.md](README.md).

# Loyalty Points Wallet — Implementation Plan

## Scope (from assignment)

Three tasks in one service, runnable locally:

| Task | Must deliver |
|------|----------------|
| **1 — Wallet** | Accounts, earn/spend, balance, idempotent `ref`, no negative balance, concurrency-safe |
| **2 — Access control** | `member` + `admin` roles; members scoped to own account; admins view/adjust any account |
| **3 — Batch** | HTTP CSV upload; safe per-account concurrency; summary + audit trail |

**Your choices (locked in):**

- **Auth:** JWT (role + account/user id in claims) **plus** DB session rows (not stateless). Single active session; revoke on logout, password reset, and forgot-password flows. **Admins create accounts** via `POST /accounts` with explicit **`role: member | admin`** (default `member`).
- **Batch:** HTTP multipart CSV upload (admin-only); **async processing** with worker pool; poll job status for summary.
- **Points storage:** Values persisted as **integer point-cents** (`BIGINT`) — smallest unit, fixed scale (**100 centipoints = 1.00 point**). Column names use **`points`** / **`balance_points`**; API/CSV use assignment-style **whole integer points** with conversion at the boundary. No floats anywhere.
- **DB:** **PostgreSQL** via `database/sql` + `github.com/jackc/pgx/v5/stdlib` driver.
- **Local dev:** **Docker Compose** — Postgres container with a named volume so data survives restarts.
- **Email:** Required on every account; **globally unique**; validated in **Go (backend)** and **PostgreSQL (CHECK + UNIQUE)**. Login and forgot-password use email.
- **Architecture:** **Layered + ports/adapters** — Router → Controller → Service → DAO → Models. Each layer has a single job; dependencies point inward. DAO interfaces allow swapping Postgres without touching services or controllers.
- **Ledger:** **Immutable append-only ledger** — every accepted earn/spend/adjustment appends one `ledger_entries` row; no UPDATE/DELETE ever. Each row stores **`direction`** (`credit` | `debit`), **`actor_account_id`** (who acted), and **`kind`** (including admin-only **`adjustment`**).
- **Admin adjustments:** `POST /accounts/{id}/transactions` with `kind: adjustment` and required **`direction`** (`credit` adds, `debit` subtracts). **`actor_account_id`** = admin JWT `sub`; **`account_id`** = member wallet. Members cannot POST `adjustment` (**403**). Assignment spec only defines `earn` | `spend`; `adjustment` is our extension for admin audit (see [SOLUTION.md](SOLUTION.md) prompts 30–32).
- **Transaction `direction` (API):** Required on **`POST /transactions`** and **`POST /accounts/{id}/transactions`**. Must match kind: `earn` → `credit`, `spend` → `debit`, `adjustment` → `credit` or `debit`. Omission → **400** `validation_error`. Stored in `ledger_entries.direction` (`migrations/003_ledger_direction.sql`).
- **Batch audit (assignment Task 3):** **`audit_events` logs every batch row** — accepted **and** rejected — with `status`, `reason`, and `timestamp`. Ledger remains the financial record for accepted rows; audit is the per-attempt trail.
- **Idempotency:** **`ref` is the global idempotency key** for every transaction — single API call or batch CSV row. API transactions accept **`Idempotency-Key` header** (preferred) or JSON body `ref` (fallback); both must match if both are sent. Batch CSV uses the `ref` column. Same `ref` never produces two ledger entries or two balance changes (enforced in service + DB).
- **Account lifecycle:** Soft delete via `deleted_at` (`migrations/002_accounts_soft_delete.sql`); admin list/update/delete; members update/delete own profile; **last-admin guard** (`409 last_admin`).
- **Session storage:** JWT + DB-backed **`auth_tokens`** table (not stateless); JWT `jti` claim stored in `auth_tokens.token`.
- **Connection pool:** `database/sql` pool via pgx with **explicit caps** from env.
- **HTTP hardening:** Rate limiting, method allowlists, middleware chain (auth + RBAC), **gzip payload compression** (responses + optional compressed request bodies), strict JSON deserialize + sanitization, standardized error responses.
- **List APIs:** Cursor-free **offset pagination** — default `limit=20`; client may request a higher limit up to `PAGINATION_MAX_LIMIT` (default 100).

**Dependencies:** stdlib (`log/slog`, `net/http`, `database/sql`) + `pgx` + `golang.org/x/crypto/bcrypt` + `golang.org/x/time/rate` (rate limiter). No Gin, no ORM.

**Repo deliverables (assignment + testing):**

1. Source code (layered Go service)
2. `README.md` — run locally, **assignment-compatible JSON examples**, async batch flow (202 → poll), `go test`, Postman import/run
3. `SOLUTION.md` — **ongoing** design/tradeoffs/AI prompts (updated as you build; auth token docs included)
4. `postman/` — single importable collection with collection-scoped variables (positive/negative integration tests)

---

## Part 1 — User flows (before schema)

Flows define *who* does *what*, *when*, and *what happens*. Schema and endpoints are derived from these.

### Actors

```mermaid
flowchart LR
  subgraph actors [Actors]
    Admin[Admin]
    Member[Member]
    System[System]
  end
  subgraph service [PointsWalletService]
    API[HTTP_API]
    DB[(PostgreSQL)]
  end
  Admin --> API
  Member --> API
  API --> DB
  System --> API
```

### Flow A — Bootstrap (first run)

1. Developer runs `docker compose up -d` → Postgres starts, volume persists data across restarts.
2. App starts → connect via `DATABASE_URL` → run SQL migrations → seed **one admin user** (env: `ADMIN_EMAIL`, `ADMIN_PASSWORD`, optional `ADMIN_ACCOUNT_ID`).
3. Admin logs in with **email + password** → receives JWT + session row created.
4. Admin creates accounts — **`role: "member"`** (default) or **`role: "admin"`** — only admins can call `POST /accounts`.

**Note:** Compose gives durable Postgres locally; row-level locking suits production workloads better than SQLite for concurrent writes.

### Flow B — Authentication (session-backed JWT)

```mermaid
sequenceDiagram
  participant Client
  participant API
  participant DB
  participant AuthTokens as auth_tokens

  Client->>API: POST /auth/login email password
  API->>API: Validate email format
  API->>DB: Verify credentials by email (active account only)
  API->>AuthTokens: Revoke all tokens for account_id
  API->>AuthTokens: Insert new row id + token=jti + expires_at
  API->>Client: JWT access_token (claims: sub, role, jti, exp)

  Client->>API: GET /accounts/me/balance Authorization Bearer JWT
  API->>API: Verify JWT signature and claims
  API->>AuthTokens: Lookup token=jti active and not expired
  alt token valid
  API->>DB: Read balance_points scoped to account_id
  API->>Client: 200 balance_points
  else revoked or expired
    API->>Client: 401 Unauthorized
  end

  Client->>API: POST /auth/logout
  API->>AuthTokens: Revoke token by jti
  API->>Client: 204 No Content
```

**Single-session rule:** On every successful login, revoke existing `auth_tokens` for that `account_id` before inserting the new one.

**Password reset / forgot password:**

- `POST /auth/forgot-password` — body `{email}`; validate format; lookup account; issue reset token (hashed + expiry; **revoke all auth_tokens**). Return generic 200 even if email not found (no enumeration).
- `POST /auth/reset-password` — validate reset token, set new password hash, **revoke all auth_tokens** again.
- Email delivery stubbed for local dev (token in response only).

### Flow C — Admin: manage accounts

| Step | Action | Auth | Outcome |
|------|--------|------|---------|
| C1 | `POST /accounts` body `{account_id, name, email, password, role}` | Admin JWT | Account created with `role: member` or `role: admin`; email unique + valid; balance 0 |
| C1b | Admin omits `role` | Admin JWT | Defaults to `member` |
| C1c | Member attempts `POST /accounts` | Member JWT | **403 Forbidden** |
| C1d | Admin creates another admin | Admin JWT | `role: "admin"` — only existing admins may create admins |
| C2 | `GET /accounts/{account_id}` | Admin JWT | Account metadata (active accounts only) |
| C3 | `GET /accounts/{account_id}/balance` | Admin JWT | `balance_points` |
| C4 | `GET /accounts?limit&offset` | Admin JWT | Paginated list of active accounts |
| C5 | `PATCH /accounts/{account_id}` body `{name, email, role}` | Admin JWT | Update profile/role; **409 `last_admin`** if demoting sole remaining admin |
| C6 | `DELETE /accounts/{account_id}` | Admin JWT | Soft delete: `deleted_at`, anonymized email, revoke all tokens; **409 `last_admin`** for sole admin |

### Flow D — Member: wallet operations

| Step | Action | Auth | Outcome |
|------|--------|------|---------|
| D1 | `GET /accounts/me/balance` | Member JWT | Own balance only |
| D2 | `POST /transactions` `{kind, direction, points, occurred_at}` + **`Idempotency-Key` header** (or body `ref`) | Member JWT | Earn/spend on **own** account only (`earn`/`spend` kinds; **`adjustment` forbidden**) |
| D3 | Member targets another account | Member JWT | **403 Forbidden** |
| D4 | `PATCH /accounts/me` `{name, email}` | Member JWT | Update own profile |
| D5 | `DELETE /accounts/me` | Member JWT | Soft delete own account; revoke auth_tokens |

**Transaction rules (all write paths):**

```mermaid
flowchart TD
  Start[POST transaction] --> AuthZ{Authorized for account?}
  AuthZ -->|No| Forbidden[403]
  AuthZ -->|Yes| BeginTx[BEGIN txn]
  BeginTx --> LockRow["SELECT balance_points FROM accounts WHERE id = $1 FOR UPDATE"]
  LockRow --> DupCheck{ref exists?}
  DupCheck -->|Yes| RejectDup[Reject duplicate reason]
  DupCheck -->|No| Kind{kind + direction?}
  Kind -->|earn credit| AppendLedger[Append ledger entry update balance_points]
  Kind -->|spend debit| BalanceOK{balance_points minus points >= 0?}
  Kind -->|adjustment debit| BalanceOK
  Kind -->|adjustment credit| AppendLedger
  Kind -->|missing/wrong direction| RejectVal[400 validation_error]
  BalanceOK -->|No| RejectNeg[Reject insufficient funds]
  BalanceOK -->|Yes| AppendLedger
  AppendLedger --> Commit[COMMIT]
  RejectDup --> Rollback[ROLLBACK audit log]
  RejectNeg --> Rollback
  Commit --> Success[201 Accepted]
```

### Flow E — Admin: adjustment

- `POST /accounts/{account_id}/transactions` — admin may target any account (admin JWT + `RequireAdmin`).
- Body: **`kind: adjustment`** with required **`direction: credit | debit`**; **`points`** always positive; same idempotency rules as other transactions.
- **`actor_account_id`** on ledger row = admin’s `account_id` (JWT `sub`). **`account_id`** on ledger row = member wallet in path.
- Credit adds points; debit subtracts with same insufficient-balance check as `spend`.
- Members cannot use `kind: adjustment` on `POST /transactions` (**403** `forbidden`).
- Batch CSV: uploading admin is `actor_account_id` for accepted rows. Optional 6th column **`direction`**; 5-column files infer direction from `kind` for `earn`/`spend` only — **`adjustment` rows require explicit direction** in CSV.

**Example (admin credit on member wallet):**

```json
{
  "kind": "adjustment",
  "direction": "credit",
  "points": 25,
  "occurred_at": "2024-06-01T12:00:00Z"
}
```

**Response / ledger entry includes:** `kind`, `direction`, `actor_account_id`, `account_id`, `points`, `balance_after_points`, `source`.

### Flow F — Admin: CSV batch ingestion (async + concurrent)

CSV rows are **independent** — no row depends on the previous row’s outcome. Processing is **async** so the HTTP request returns immediately; rows run concurrently subject to per-account DB locking.

```mermaid
sequenceDiagram
  participant Admin
  participant API
  participant BatchSvc
  participant Pool as WorkerPool
  participant Wallet as WalletService
  participant DB

  Admin->>API: POST /batch/transactions CSV file
  API->>BatchSvc: Parse + validate CSV rows in memory
  BatchSvc->>DB: INSERT batch_jobs status=queued
  API->>Admin: 202 Accepted batch_job_id

  BatchSvc->>Pool: Enqueue rows async

  par concurrent workers N
    Pool->>Wallet: ApplyTransaction row
    Wallet->>DB: BEGIN per row FOR UPDATE
    alt accepted
      Wallet->>DB: INSERT ledger + UPDATE balance COMMIT
      Pool->>DB: INSERT audit_event status=accepted reason=ok
    else rejected
      Wallet->>DB: ROLLBACK
      Pool->>DB: INSERT audit_event status=rejected reason=duplicate_or_insufficient
    end
  end

  Pool->>DB: UPDATE batch_jobs status=completed summary atomic
  Admin->>API: GET /batch/jobs/{id}
  API->>Admin: 200 summary accepted rejected duplicates
```

**Why async:** A 10k-row CSV must not block the HTTP connection for minutes. Upload validates and queues; workers drain the queue in the background.

**Why concurrent:** Rows targeting **different** `account_id` values run in parallel (`BATCH_WORKER_COUNT`, default 8). Rows targeting the **same** account serialize safely via `SELECT ... FOR UPDATE` inside each row’s own transaction — no application-level ordering required unless business rules change.

**Row independence:** Each CSV row is a separate financial action with its own `ref`. Failure on row 50 does **not** roll back rows 1–49.

### Flow F2 — Transaction boundaries (atomicity + rollback)

| Unit of work | Transaction scope | On failure |
|--------------|-------------------|------------|
| **Single CSV row** (accepted path) | One DB txn: lock account → check dup → insert ledger → update balance | **ROLLBACK** — no ledger row, balance unchanged |
| **Single CSV row** (rejected path) | No ledger txn; optional audit INSERT in own short txn | Audit row still recorded |
| **Batch job completion** | One DB txn: update `batch_jobs.status`, counts, `completed_at` | Retry or mark `failed` |
| **Entire CSV file** | **Not** one giant transaction | Partial success is valid — summary reports accepted + rejected |

**Never** wrap the whole batch in a single transaction — that would undo all progress on one bad row and kills concurrency.

**Rollback rule:** Only the **current row’s** database transaction rolls back. Already-committed rows from the same batch remain durable (assignment-consistent partial batch outcomes).

### Flow F3 — Batch job recovery after process restart

Assignment requires data durable across restarts. On app startup (after migrations):

1. Find `batch_jobs` where `status = 'processing'`.
2. Mark them **`failed`** with `error_message = 'interrupted by process restart'`.
3. Do **not** re-run rows automatically — ledger + idempotent `ref` make **re-upload safe**; admin re-uploads CSV if needed.

**Summary:** Committed ledger rows survive restart; in-flight jobs fail closed; reprocessing the file is safe because refs are idempotent.

### Flow J — Global idempotency (`ref`) — API and batch identical rules

Every earn/spend/adjustment — whether `POST /transactions`, admin adjustment, or a CSV row — **must** carry a client-supplied **`ref`**. The system treats `ref` as a **unique business idempotency key**, not an auto-generated DB id.

**API resolution (`dto.ResolveTransactionRef`):**

| Input | Stored ledger `ref` |
|-------|---------------------|
| `Idempotency-Key` header only | Header value |
| JSON body `ref` only | Body value |
| Both header and body | Must be identical; else **400** `validation_error` |
| Neither | **400** `validation_error` |

Batch CSV unchanged: `ref` column only (no HTTP header).

```mermaid
flowchart TD
  Submit[Submit transaction with ref] --> Sanitize[Sanitize + validate ref]
  Sanitize --> ServiceCheck{Service: ref already in ledger?}
  ServiceCheck -->|yes| RejectEarly[Reject duplicate_ref]
  ServiceCheck -->|no| BeginTxn[BEGIN DB transaction]
  BeginTxn --> Lock[FOR UPDATE account row]
  Lock --> DupDB{INSERT ledger UNIQUE ref}
  DupDB -->|conflict 23505| Rollback[ROLLBACK no balance change]
  DupDB -->|ok| Commit[COMMIT ledger + balance]
  Rollback --> RejectEarly
  Commit --> Success[Accepted once]
```

**Single rule, three entry points — same code path:**

| Entry point | Idempotency behavior |
|-------------|----------------------|
| `POST /transactions` (member) | `wallet.Service.ApplyTransaction` → duplicate → **409** `duplicate_ref` |
| `POST /accounts/{id}/transactions` (admin) | Same service method → **409** `duplicate_ref` |
| Batch CSV row | Same service method → row **rejected**, `duplicate_count++`, audit `duplicate_ref` |
| Batch re-upload same file | All previously accepted refs → duplicates, **zero** extra balance impact |
| Concurrent duplicate submits | Two requests same `ref` → one COMMIT, one UNIQUE violation → **409** / duplicate |

**No special batch idempotency** — batch rows call the **exact same** `ApplyTransaction` as the REST API. No duplicated logic.

**Layered enforcement (defense in depth):**

1. **DTO / sanitize** — `ref` or `Idempotency-Key` required for API txs; trimmed, max length; empty → **400**.
2. **Service** — optional fast path: `LedgerDAO.RefExists(ref)` before opening write txn (friendly error without lock contention when obvious duplicate).
3. **Database** — `ledger_entries.ref UNIQUE` — final guarantee even under concurrent API + batch workers; map Postgres `23505` → `models.ErrDuplicateRef`.
4. **Immutable ledger** — accepted refs never UPDATE/DELETE; replaying the same ref never mutates balance twice.

**Safe retry semantics:**

- Client retries `POST /transactions` with same `ref` after timeout → **409** (or idempotent **200** with original result if you store ref→response — **plan choice: 409** for simplicity and clear “already processed” signal; document in SOLUTION.md).
- CSV row with `tx-001` accepted; same file reprocessed → `tx-001` counted as duplicate, balance unchanged.

**CSV format unchanged:** `ref,account_id,kind,points,occurred_at` — `ref` column is the idempotency key per assignment.

### Flow I — Immutable ledger + batch audit trail

```mermaid
flowchart LR
  subgraph writes [Append only]
    API[API earn/spend]
    Batch[Batch CSV row]
    Admin[Admin adjustment]
  end
  subgraph ledger [ledger_entries]
    L1[accepted entry]
    L2[accepted entry]
  end
  subgraph audit [audit_events batch rows]
    A1[accepted reason ok]
    A2[rejected duplicate_ref]
    A3[rejected insufficient_balance]
  end
  API --> L1
  Admin --> L2
  Batch --> L2
  Batch --> A1
  Batch -.->|rejected| A2
  Batch -.->|rejected| A3
```

**Rules:**

- Every **accepted** transaction → exactly one **INSERT** into `ledger_entries` (never UPDATE/DELETE).
- Ledger row: `ref`, `account_id`, `kind`, **`direction`**, `points`, `balance_after_points`, `occurred_at`, `recorded_at`, **`actor_account_id`**, `source`.
- **`accounts.balance_points`** updated in the same DB txn as ledger INSERT.
- **Every batch row attempt** → one **`audit_events`** row (`status`: `accepted` | `rejected`, `reason`, `timestamp`) — assignment Task 3.
- **API-only** txs: ledger sufficient; no `audit_events` required. **Batch path** always writes audit for every row.
- **Rejected** attempts → no ledger row; batch rejections still get `audit_events`.
- **Idempotency:** global `UNIQUE(ref)`; duplicate never mutates balance twice.

- **Read ledger (paginated):** responses include **`direction`**, **`kind`**, and **`actor_account_id`** (who performed the action; for admin adjustments, differs from `account_id`).
- Member: `GET /accounts/me/ledger?limit=20&offset=0` — own entries, newest-first.
- Admin: `GET /accounts/{id}/ledger?limit=20&offset=0` — any account.
- **Default:** `limit=20` when query param omitted; `offset=0` when omitted.
- **Override:** client may pass a higher `limit` (e.g. `limit=50`) up to server max; values above max → **400** `validation_error`.

### Flow G — Error / edge cases (explicit test targets)

| Scenario | Expected HTTP | Audit |
|----------|---------------|-------|
| Duplicate `ref` (API or CSV) | 409 or duplicate count in batch summary | `audit_events` rejected `duplicate_ref` |
| Batch row accepted | — | `audit_events` accepted `ok` |
| Spend > balance (spend or adjustment debit) | 422 | `insufficient_balance` |
| Missing or invalid `direction` | 400 | `validation_error` |
| `direction` mismatches kind (e.g. earn + debit) | 400 | `validation_error` |
| Member POST `kind: adjustment` | 403 | — |
| Invalid kind / negative points | 400 | `validation_error` |
| Float in JSON points field | 400 | `validation_error` |
| Member reads other account | 403 | — |
| Revoked JWT / missing token | 401 | — |
| Soft-deleted account login | 401 | — |
| Demote/delete last admin | 409 `last_admin` | — |
| Wrong role (member hits admin route) | 403 | — |
| Wrong HTTP method (GET on POST-only route) | 405 | — |
| Rate limit exceeded | 429 | — |
| Validation failed (body/params) | 400 | — |
| Second login while first session active | First token invalid on next request | — |
| Invalid role on create account | 400 | — |
| Member creates account (any role) | 403 | — |
| Invalid email format (API) | 400 Bad Request | — |
| Invalid email format (DB bypass attempt) | Insert fails; map to 400/500 | — |

### Flow H — Email validation (defense in depth)

```mermaid
flowchart TD
  Request[Incoming email field] --> Normalize["Normalize: strings.TrimSpace + strings.ToLower"]
  Normalize --> BackendCheck["Backend: domain.ValidateEmail via net/mail.ParseAddress"]
  BackendCheck -->|invalid| Reject400[400 invalid_email]
  BackendCheck -->|valid| Service[service layer CreateAccount / Login]
  Service --> DBInsert[INSERT into accounts]
  DBInsert --> DBCheck["DB: CHECK constraint regex + UNIQUE on email"]
  DBCheck -->|duplicate| Reject409[409 email_already_exists]
  DBCheck -->|invalid| RejectDB[Reject constraint violation]
  DBCheck -->|ok| Success[201 Created]
```

**Normalization rule:** Always persist lowercase trimmed email so `User@Example.com` and `user@example.com` cannot both exist.

---

## Part 2 — Data model (derived from flows)

### Integer points naming + cent storage

**Naming:** use **`points`** consistently in column and JSON field names — no parallel `amount` / `amount_cents` names.

**Storage:** every persisted value is **point-cents** (fixed scale, 2 decimal places). Never store floats.

| Concept | DB column | Stored value (example) | Go type | JSON field (API) |
|---------|-----------|------------------------|---------|------------------|
| Current account balance | `accounts.balance_points` | `15000` (= 150.00 pts) | `Points` | `balance_points`: **150** (whole points) |
| Transaction delta | `ledger_entries.points` | `5000` (= 50.00 pts) | `Points` | `points`: **50** (whole points) |
| Balance after entry | `ledger_entries.balance_after_points` | `15000` | `Points` | `balance_after_points`: **150** |

**Scale rule:** `stored = whole_points × 100`. Assignment CSV/JSON use **integer whole points only** (no decimals in v1) — service converts at the edge:

```go
// internal/models/points.go
type Points int64 // point-cents in DB and domain math

const PointsScale int64 = 100

func PointsFromWhole(whole int64) Points { return Points(whole * PointsScale) }
func (p Points) WholePoints() int64       { return int64(p) / PointsScale }
```

| Layer | Rule | Example |
|-------|------|---------|
| **Database** | `BIGINT` point-cents | `balance_points = 15000` |
| **Go domain** | `Points int64` — all earn/spend math in point-cents | `PointsFromWhole(150)` → `15000` |
| **JSON request (assignment)** | integer `points` = whole points | `"points": 150` → store `15000` |
| **JSON response** | whole points for assignment compat | DB `15000` → `"balance_points": 150` |
| **CSV batch** | `points` column = whole points | same conversion as API |

**Why point-cents:** Avoids floating-point rounding; supports fractional points later (e.g. `"150.50"`) without schema changes — only API parsing would extend. Column is named `points`; stored values are fixed-scale integer cents.

**Assignment compat:** Request/response shapes unchanged — integers only. Internally always point-cents.

**No `amount` column:** Ledger delta column is `points` (stores cent-scaled value). Account running total is `balance_points` (same unit).

### Immutable ledger design

| Principle | Implementation |
|-----------|----------------|
| **Append-only** | DAO exposes `AppendLedgerEntry` only — no Update/Delete methods on ledger |
| **DB enforcement** | Migration grants INSERT+SELECT on `ledger_entries`; app DB user has no UPDATE/DELETE (document in README) |
| **Idempotency** | `UNIQUE(ref)` — same ref cannot produce two ledger rows |
| **Audit trail** | Each row is a permanent record of one financial action |
| **Balance integrity** | `balance_after_points` written atomically with account `balance_points` update |

**Principle:** The ledger is the system of record; account balance is a cache. In a dispute you replay ledger entries — rows are never edited or deleted.

### Entity relationship

```mermaid
erDiagram
  accounts ||--o{ auth_tokens : has
  accounts ||--o{ ledger_entries : owns
  accounts ||--o{ audit_events : relates
  batch_jobs ||--o{ audit_events : produces

  accounts {
    string account_id
    string email
    string name
    string role
    string password_hash
    int balance_points
    datetime created_at
    datetime deleted_at
  }
  ledger_entries {
    int id
    string ref
    string account_id
    string kind
    string direction
    int points
    int balance_after_points
    datetime occurred_at
    datetime recorded_at
    string actor_account_id
    string source
  }
  auth_tokens {
    string id
    string account_id
    string token
    datetime expires_at
    datetime revoked_at
    datetime created_at
  }
  audit_events {
    int id
    string batch_id
    string ref
    string account_id
    string status
    string reason
    datetime created_at
  }
  batch_jobs {
    string id
    string status
    int total_rows
    int accepted_count
    int rejected_count
    int duplicate_count
    datetime created_at
    datetime started_at
    datetime completed_at
    string error_message
  }
```

**Keys & indexes (not shown in diagram — Mermaid allows only `type name` per line):**
- `accounts.account_id` PK; partial unique on `email` where `deleted_at IS NULL`
- `ledger_entries.ref` UNIQUE globally; `account_id` FK; `direction` CHECK (`credit` | `debit`)
- `auth_tokens.token` UNIQUE (JWT `jti`); `account_id` FK
- `audit_events.batch_id` FK → `batch_jobs.id`

`audit_events.status`: `accepted` | `rejected`. `reason`: e.g. `ok`, `duplicate_ref`, `insufficient_balance`, `validation_error`.

### Email constraints (PostgreSQL)

```sql
-- migrations/001_init.sql (accounts table excerpt)
email TEXT NOT NULL,
CONSTRAINT accounts_email_format CHECK (
  email ~ '^[a-z0-9._%+-]+@[a-z0-9.-]+\.[a-z]{2,}$'
)

-- migrations/002_accounts_soft_delete.sql
deleted_at TIMESTAMPTZ,
CREATE UNIQUE INDEX accounts_email_active_unique ON accounts (email) WHERE deleted_at IS NULL;
```

- **Partial unique index** — active accounts only (`deleted_at IS NULL`); soft-deleted rows get anonymized email so the original address can be reused.
- **`CHECK`** — regex on **normalized** (lowercase) values; app must normalize before insert so CHECK and UNIQUE stay consistent.
- **Index:** partial unique index — O(log n) lookup by email at login among active accounts.

**Backend mirror** (`internal/models/email.go`):

```go
func NormalizeEmail(raw string) (string, error)
func ValidateEmail(raw string) error  // net/mail.ParseAddress after normalize
```

Validate on: `POST /accounts`, `POST /auth/login`, `POST /auth/forgot-password`. Map Postgres `23505` (unique violation) → `409 email_already_exists`; `23514` (check violation) → `400 invalid_email`.

**Accounts role constraint:**

```sql
role TEXT NOT NULL DEFAULT 'member' CHECK (role IN ('member', 'admin'))
```

Only **`POST /accounts`** (admin-only route) may set `role`. Seeded bootstrap admin has `role = 'admin'`.

**Accounts points column:**

```sql
balance_points BIGINT NOT NULL DEFAULT 0 CHECK (balance_points >= 0)  -- point-cents
```

### Ledger schema (PostgreSQL)

```sql
CREATE TABLE ledger_entries (
  id                  BIGSERIAL PRIMARY KEY,
  ref                 TEXT NOT NULL UNIQUE,
  account_id          TEXT NOT NULL REFERENCES accounts(account_id),
  kind                TEXT NOT NULL CHECK (kind IN ('earn', 'spend', 'adjustment')),
  direction           TEXT NOT NULL CHECK (direction IN ('credit', 'debit')),
  points              BIGINT NOT NULL CHECK (points > 0),              -- point-cents
  balance_after_points BIGINT NOT NULL CHECK (balance_after_points >= 0), -- point-cents
  occurred_at         TIMESTAMPTZ NOT NULL,
  recorded_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
  actor_account_id    TEXT NOT NULL REFERENCES accounts(account_id),
  source              TEXT NOT NULL CHECK (source IN ('api', 'batch'))
);

CREATE INDEX idx_ledger_account_recorded ON ledger_entries (account_id, recorded_at DESC);
```

- **`ref` UNIQUE** — global idempotency key (assignment requirement).
- **`direction`** — `credit` (add) or `debit` (subtract); required on API; stored on every ledger row. Migration **`003_ledger_direction.sql`** adds column and backfills existing rows (`earn`/`adjustment` → credit, `spend` → debit).
- **`balance_after_points`** — running balance snapshot after this entry.
- **`actor_account_id`** — who triggered the action (member self-serve, admin adjusting a member, or uploading admin for batch). Admin identity is **id only** on the ledger — resolve name/email via `GET /accounts/{actor_account_id}` when needed.
- **`source`** — distinguishes API vs batch ingestion for audit.
- **Immutability:** no UPDATE/DELETE triggers or app methods; optional migration comment: `REVOKE UPDATE, DELETE ON ledger_entries FROM wallet_app`.

### Table notes

- **`ledger_entries`** — immutable system of record; INSERT-only via DAO.
- **`accounts.role`** — `member` | `admin`; set at create time via `POST /accounts`; CHECK constraint in DB; copied into JWT on login.
- **`accounts.balance_points`** — denormalized cache; must equal latest `balance_after_points` for that account after each commit.
- **`batch_jobs`** — async job tracker: `queued` → `processing` → `completed` | `failed`; summary counts updated atomically on completion.
- **`audit_events`** — **every batch row attempt** (accepted + rejected) with reason + timestamp; `GET /batch/jobs/{id}/audit` paginated for reviewers.
- **`accounts.email`** — unique among active accounts; normalized lowercase; validated at app + DB; anonymized on soft delete.
- **`accounts.deleted_at`** — NULL = active; set on soft delete; all active queries filter `deleted_at IS NULL`.
- **`auth_tokens.token`** — stores JWT `jti` claim; middleware checks `revoked_at IS NULL AND expires_at > now()`.

### Batch jobs schema (PostgreSQL)

```sql
CREATE TABLE batch_jobs (
  id               UUID PRIMARY KEY,
  status           TEXT NOT NULL CHECK (status IN ('queued','processing','completed','failed')),
  total_rows       INT NOT NULL DEFAULT 0,
  accepted_count   INT NOT NULL DEFAULT 0,
  rejected_count   INT NOT NULL DEFAULT 0,
  duplicate_count  INT NOT NULL DEFAULT 0,
  created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  started_at       TIMESTAMPTZ,
  completed_at     TIMESTAMPTZ,
  error_message    TEXT
);
```

### Async batch processor (`service/batch/processor.go`)

```go
// Pseudocode — worker pool pattern (stdlib sync + context)
func (s *Service) ProcessAsync(ctx context.Context, jobID uuid.UUID, rows []models.BatchRow) {
    s.dao.UpdateJobStatus(ctx, jobID, "processing")
    rowCh := make(chan models.BatchRow, len(rows))
    var wg sync.WaitGroup
    var mu sync.Mutex
    summary := models.BatchSummary{}

    for i := 0; i < s.workerCount; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            for row := range rowCh {
                err := s.wallet.ApplyTransaction(ctx, row.ToLedgerEntry(jobID))
                mu.Lock()
                summary.Record(err)
                mu.Unlock()
                s.audit.LogRow(ctx, jobID, row, err) // always: accepted → reason ok; rejected → duplicate_ref etc.
            }
        }()
    }
    for _, r := range rows { rowCh <- r }
    close(rowCh)
    wg.Wait()
    s.dao.CompleteJob(ctx, jobID, summary) // single atomic UPDATE
}
```

- **`BATCH_WORKER_COUNT`** (default **8**) — tunable via env; cap to pool size (do not exceed `DB_MAX_OPEN_CONNS` recklessly).
- **Reprocess safety:** Same `ref` → `UNIQUE` on `ledger_entries.ref` → duplicate counted, not double-applied.
- **Graceful shutdown:** `context.Cancel` on server shutdown; in-flight row txns finish or rollback; job marked `failed` if interrupted.
- **Startup recovery:** `RecoverStaleJobs()` marks `processing` → `failed` on boot (see Flow F3).

### Concurrency strategy (PostgreSQL)

```sql
BEGIN;
SELECT balance_points FROM accounts WHERE account_id = $1 FOR UPDATE;
-- check duplicate ref in ledger_entries
-- check spend would not go negative
INSERT INTO ledger_entries (ref, account_id, kind, direction, points, balance_after_points, occurred_at, actor_account_id, source)
  VALUES (...);
-- kind=direction rules: earn→credit, spend→debit; adjustment→credit|debit (debit checks balance)
UPDATE accounts SET balance_points = $balance_after WHERE account_id = $1;
COMMIT;
-- rejected paths: INSERT audit_events only, ROLLBACK or skip ledger insert
```

**Concurrency:** Row-level lock on account serializes concurrent spends/earns; Postgres handles cross-request safety better than SQLite for overlapping writes.

---

## Part 3 — Docker Compose (local dev)

```
pointswallet/
├── docker-compose.yml
├── migrations/
│   ├── 001_init.sql
│   ├── 002_accounts_soft_delete.sql
│   └── 003_ledger_direction.sql
├── .env.example
├── cmd/server/main.go              # composition root: wire DAO → services → controllers → router
├── postman/
│   ├── PointsWallet.postman_collection.json   # single file; collection-scoped variables
│   ├── sample-batch.csv
│   ├── sample-batch-success.csv
│   ├── sample-batch-rejects.csv
│   └── README.md                   # import + Collection Runner instructions
├── internal/
│   ├── testutil/
│   │   └── mocks/                  # hand-written mock DAOs/services for unit tests
│   ├── models/                     # domain models, DTOs, validation, sentinel errors
│   │   ├── account.go
│   │   ├── transaction.go
│   │   ├── ledger_entry.go
│   │   ├── points.go
│   │   ├── email.go
│   │   ├── errors.go
│   │   └── dto/                    # HTTP request/response + PaginationQuery
│   ├── dao/                        # data access interfaces (ports)
│   │   ├── wallet.go               # WalletDAO + LedgerDAO interfaces
│   │   ├── auth.go
│   │   ├── audit.go
│   │   └── postgres/
│   │       ├── pool.go
│   │       ├── wallet_dao.go
│   │       ├── ledger_dao.go         # AppendLedgerEntry, ListByAccount (INSERT-only)
│   │       ├── batch_dao.go          # batch_jobs async status
│   │       ├── auth_dao.go
│   │       └── audit_dao.go
│   ├── service/                    # business logic (reusable, no HTTP/SQL)
│   │   ├── wallet/service.go
│   │   ├── auth/service.go
│   │   └── batch/
│   │       ├── service.go
│   │       └── processor.go        # async worker pool + summary aggregation
│   ├── controller/                 # HTTP controllers (thin: parse → service → respond)
│   │   ├── account_controller.go
│   │   ├── auth_controller.go
│   │   ├── transaction_controller.go
│   │   ├── ledger_controller.go      # GET ledger history
│   │   ├── batch_controller.go
│   │   ├── response.go             # writeJSON, writeError, MapDomainError
│   │   └── decode.go               # decodeAndSanitizeJSON — shared body pipeline
│   ├── router/                     # route table + middleware chain only
│   │   ├── router.go
│   │   └── middleware/
│   │       ├── chain.go
│   │       ├── logging.go
│   │       ├── bodylimit.go        # MaxBytesReader — cap request body size
│   │       ├── decompress.go       # gunzip request when Content-Encoding: gzip
│   │       ├── compress.go         # gzip response when Accept-Encoding: gzip
│   │       ├── contenttype.go      # require application/json on JSON routes
│   │       ├── ratelimit.go
│   │       ├── methods.go
│   │       ├── auth.go
│   │       └── rbac.go
│   └── adapters/
│       └── logger/
│           └── slog.go             # Logger interface + slog impl
├── migrations/
│   ├── 001_init.sql
│   ├── 002_accounts_soft_delete.sql
│   └── 003_ledger_direction.sql
├── README.md
├── SOLUTION.md
└── go.mod
```

### `docker-compose.yml` (minimal)

| Service | Image | Purpose |
|---------|-------|---------|
| `postgres` | `postgres:16-alpine` | Primary datastore |
| `migrate` (optional) | same or app image | Run migrations on startup |

**Volume:** `postgres_data:/var/lib/postgresql/data` — satisfies assignment “persisted across restarts”.

**Env (`.env.example`):**

```
DATABASE_URL=postgres://wallet:wallet@localhost:5432/pointswallet?sslmode=disable
DB_MAX_OPEN_CONNS=10
DB_MAX_IDLE_CONNS=5
DB_CONN_MAX_LIFETIME=30m
DB_CONN_MAX_IDLE_TIME=5m
JWT_SECRET=change-me-in-dev
ADMIN_EMAIL=admin@example.com
ADMIN_PASSWORD=admin123
ADMIN_ACCOUNT_ID=admin
HTTP_ADDR=:8080
MAX_REQUEST_BODY_BYTES=1048576
MAX_DECOMPRESSED_BODY_BYTES=1048576
GZIP_MIN_RESPONSE_BYTES=1024
LOG_LEVEL=info
RATE_LIMIT_RPS=10
RATE_LIMIT_BURST=20
AUTH_RATE_LIMIT_RPS=5
PAGINATION_DEFAULT_LIMIT=20
PAGINATION_MAX_LIMIT=100
BATCH_WORKER_COUNT=8
```

**Run flow:**

1. `docker compose up -d postgres`
2. App runs migrations on start (or `make migrate`)
3. `go run ./cmd/server`
4. `docker compose down` — data kept in volume; `docker compose up -d` again restores same balances

```mermaid
flowchart TB
  subgraph compose [DockerCompose]
    PG[(postgres:16)]
    Vol[postgres_data volume]
    PG --- Vol
  end
  App[Go server] -->|DATABASE_URL| PG
```

---

## Part 4 — Layered architecture (Router → Controller → Service → DAO → Models)

Each layer has **one responsibility**. Dependencies flow **inward only**.

```mermaid
flowchart TB
  subgraph outer [HTTP Layer]
    Router[router.Router]
    MW[router/middleware]
    Ctrl[controller]
  end
  subgraph biz [Business Layer]
    WalletSvc[service/wallet]
    AuthSvc[service/auth]
    BatchSvc[service/batch]
  end
  subgraph data [Data Layer]
    WalletDAO[dao.WalletDAO]
    AuthDAO[dao.AuthDAO]
    AuditDAO[dao.AuditDAO]
  end
  subgraph core [Models]
    ModelsPkg[models + dto]
  end
  subgraph infra [Infrastructure]
    PG[dao/postgres]
    Slog[adapters/logger]
    Pool[(sql.DB pool)]
  end
  Router --> MW --> Ctrl
  Ctrl --> WalletSvc
  Ctrl --> AuthSvc
  Ctrl --> BatchSvc
  WalletSvc --> WalletDAO
  AuthSvc --> AuthDAO
  BatchSvc --> WalletDAO
  BatchSvc --> AuditDAO
  WalletSvc --> ModelsPkg
  WalletDAO -.->|implements| PG
  AuthDAO -.->|implements| PG
  AuditDAO -.->|implements| PG
  PG --> Pool --> PostgresDB[(PostgreSQL)]
  WalletSvc --> Slog
```

### Layer responsibilities

| Layer | Package | Does | Must NOT do |
|-------|---------|------|-------------|
| **Router** | `internal/router` | Register routes, attach middleware | Parse bodies, SQL, business rules |
| **Controller** | `internal/controller` | HTTP decode/encode, call service, map errors to status | SQL, balance rules, idempotency |
| **Service** | `internal/service/*` | Business rules, orchestration, RBAC helpers | HTTP types, SQL strings |
| **DAO** | `internal/dao` + `postgres/` | Persist/load models, parameterized SQL | HTTP, JWT, earn/spend rules |
| **Models** | `internal/models` | Entities, value objects, validation, DTOs, domain errors | Import service, dao, controller |

### Request flow (member spend)

```mermaid
sequenceDiagram
  participant R as Router
  participant M as AuthMiddleware
  participant C as TransactionController
  participant S as WalletService
  participant D as WalletDAO
  participant DB as PostgreSQL

  R->>M: POST /transactions
  M->>C: inject claims
  C->>S: Spend(ctx, accountID, tx)
  S->>D: ApplyTransaction(ctx, models.LedgerEntry)
  D->>DB: BEGIN + FOR UPDATE + INSERT ledger_entries + UPDATE balance
  D-->>S: nil or models.ErrDuplicateRef
  S-->>C: result or domain error
  C-->>R: JSON + HTTP status
```

### Models (`internal/models/`)

- **Entities:** `Account`, `LedgerEntry`, `AuthToken`, `AuditEvent`
- **Value objects:** `Points`, `Email` with `Validate()` / `Normalize()`
- **Domain errors:** `ErrDuplicateRef`, `ErrInsufficientBalance`, `ErrInvalidEmail`
- **DTOs** (`models/dto/`): HTTP request/response structs — keeps services transport-agnostic

### DAO interfaces (`internal/dao/`)

| Interface | Responsibility | Postgres impl |
|-----------|----------------|---------------|
| `WalletDAO` | `CreateAccount`, `GetAccount`, `ListAccounts`, `UpdateProfile`, `UpdateProfileRole`, `SoftDeleteAccount`, `CountActiveAdmins`, `GetBalance`, `ApplyTransaction` | `wallet_dao.go` |
| `LedgerDAO` | `ListByAccount`, `RefExists` | `ledger_dao.go` |
| `AuthDAO` | credentials, `auth_tokens`, reset tokens, `SeedAdminIfMissing`, `IsAccountActive` | `auth_dao.go` |
| `BatchDAO` | `CreateJob`, `UpdateJobStatus`, `CompleteJob`, `GetJob` | `batch_dao.go` |
| `AuditDAO` | `InsertAuditEvent`, `ListByBatchID` (all statuses) | `audit_dao.go` |

`ApplyTransaction` atomically INSERTs into `ledger_entries` and UPDATEs `accounts.balance_points` — services call one method.

### Service layer (`internal/service/`)

- **Reusable:** `batch.Service` and `TransactionController` both call **`wallet.Service.ApplyTransaction`** — single idempotency implementation for API + batch.
- **Idempotent by `ref`:** duplicate detection and `ErrDuplicateRef` handled once in `ApplyTransaction`; never duplicated in batch processor or controllers.
- **Testable:** mock DAOs in unit tests.
- Never imports `controller`, `router`, or `dao/postgres`.

### Controller layer (`internal/controller/`)

| Controller | Handles |
|------------|---------|
| `AuthController` | login, logout, forgot/reset password |
| `AccountController` | create, list, get, update, delete (admin); my balance, update, delete (member) |
| `TransactionController` | member earn/spend; admin adjustment; required `direction`; member **`adjustment` → 403**; resolves `Idempotency-Key` / body `ref` |
| `LedgerController` | GET ledger history (member own / admin any) |
| `BatchController` | CSV upload → **202**; GET job status; GET paginated audit |

`response.go` — standardized success/error envelopes (see Part 5b below).

**Controller request pipeline:** Every JSON handler uses shared `decodeAndSanitizeJSON` — **deserialize → sanitize → validate** — before calling service. No service call until all three pass.

**Tests:** `httptest` + mock service — no DB.

### Request body: deserialize + sanitize (`controller/decode.go`)

All POST/PUT JSON endpoints share one pipeline. Order is fixed:

```mermaid
flowchart LR
  Body[Request body] --> Limit[MaxBytesReader]
  Limit --> CT[Content-Type check]
  CT --> JSON[json.Decoder strict decode]
  JSON --> Sanitize[dto.Sanitize]
  Sanitize --> Validate[dto.Validate]
  Validate --> Service[Service layer]
```

**1. Size limit (middleware `bodylimit.go`)**

- Wrap `r.Body` with `http.MaxBytesReader(w, r.Body, cfg.MaxRequestBodyBytes)` — default **1 MiB** (`MAX_REQUEST_BODY_BYTES=1048576`).
- Exceed limit → **413** `payload_too_large` (or **400** if preferred; document one choice consistently).

**2. Content-Type (middleware `contenttype.go`)**

- JSON routes require `Content-Type: application/json` (allow `application/json; charset=utf-8`).
- Wrong/missing type → **415** `unsupported_media_type` or **400** `validation_error`.

**3. Strict JSON deserialize (`controller/decode.go`)**

```go
func decodeAndSanitizeJSON(w http.ResponseWriter, r *http.Request, dst dto.SanitizerValidator) error {
    dec := json.NewDecoder(r.Body)
    dec.DisallowUnknownFields() // reject extra keys — prevents mass-assignment surprises
    if err := dec.Decode(&dst); err != nil {
        return models.ErrInvalidJSON(err)
    }
    if err := dec.Decode(&struct{}{}); err != io.EOF {
        return models.ErrInvalidJSON("multiple JSON values") // single object only
    }
    dst.Sanitize()
    return dst.Validate()
}
```

- Decode into typed DTO structs only — **never** `map[string]any` for user input.
- Integer fields (`points`, `balance_points`) use `int64` — JSON floats rejected at decode (`json: cannot unmarshal number 1.5 into Go struct field ...`).
- Reject `null` for required fields via pointer checks or validate after decode.

**4. Sanitize (`dto.Sanitize()` on every request DTO)**

| Field type | Sanitization |
|------------|--------------|
| Strings | `strings.TrimSpace`; strip `\x00` and other ASCII control chars |
| `email` | `models.NormalizeEmail` (lowercase, trim) |
| `account_id`, `ref` | trim; max length (e.g. 64); regex `^[a-zA-Z0-9._-]+$` |
| `name` | trim; max length (e.g. 128) |
| `password` | trim outer whitespace only; no HTML entity encoding (stored hashed) |
| `kind` | trim; lowercase; allowlist `earn` \| `spend` \| `adjustment` |
| `direction` | trim; lowercase; **required**; allowlist `credit` \| `debit`; must match kind (see Flow E) |
| `occurred_at` | parse RFC3339; reject zero time |

**5. Validate (`dto.Validate()`)** — business rules after sanitization (required fields, email format, positive points, etc.).

**Interface:**

```go
// models/dto/sanitizer.go
type SanitizerValidator interface {
    Sanitize()
    Validate() error
}
```

**CSV/batch upload:** multipart file body — separate path; sanitize each CSV cell (trim, reject control chars) before building ledger DTO; row-level validation before `wallet.Service.ApplyTransaction`.

**Pipeline:** Deserialize strictly into typed DTOs, sanitize strings before validation, then validate — never pass raw request maps into the service layer.

### Router layer (`internal/router/`)

Route table wires **middleware chains per route**. Go 1.22+ `ServeMux` method patterns ensure only declared method+path match; extra guard via `middleware.AllowMethods`.

### Middleware chain (request pipeline)

Every request passes through layers **in this order** before controller logic:

```mermaid
flowchart TD
  Req[Incoming request] --> Log[LoggingMiddleware]
  Log --> Decompress{Content-Encoding gzip?}
  Decompress -->|yes| Gunzip[DecompressMiddleware gunzip body]
  Decompress -->|no| Method
  Gunzip --> Method[AllowMethodsMiddleware]
  Method -->|wrong method| R405[405 Method Not Allowed]
  Method --> Rate[RateLimitMiddleware]
  Rate -->|over limit| R429[429 Too Many Requests]
  Rate --> BodyLimit[MaxBodyBytesMiddleware]
  BodyLimit -->|too large| R413[413 Payload Too Large]
  BodyLimit --> CT{JSON route?}
  CT -->|yes| ContentType[ContentTypeMiddleware]
  ContentType -->|wrong type| R415[415 Unsupported Media Type]
  ContentType --> AuthCheck{Route requires auth?}
  AuthCheck -->|public| Ctrl[Controller]
  AuthCheck -->|protected| Auth[AuthMiddleware JWT + session]
  Auth -->|invalid| R401[401 Unauthorized]
  Auth --> RBAC[RBACMiddleware role check]
  RBAC -->|wrong role| R403[403 Forbidden]
  RBAC --> Ctrl
  Ctrl --> Decode[decodeAndSanitizeJSON]
  Decode -->|bad JSON/sanitize/validate| R400[400 validation_error]
  Decode --> Service[Service layer]
  Service --> Compress{Accept-Encoding gzip?}
  Compress -->|yes and body large enough| GzipResp[CompressMiddleware gzip response]
  Compress -->|no| Client[Client]
  GzipResp --> Client
```

**Note:** Response compression wraps the handler chain outward — registered so it runs after handler writes (classic `gzip.ResponseWriter` pattern).

```mermaid
flowchart TD
  subgraph mwTable [Middleware summary]
    direction TB
    M1[Logging]
    M2[Decompress request gzip]
    M3[AllowMethods]
    M4[RateLimit]
    M5[MaxBodyBytes]
    M6[ContentType JSON routes]
    M7[Auth + RBAC]
    M8[Compress response gzip]
  end
```

| Middleware | Scope | Behavior |
|------------|-------|----------|
| `LoggingMiddleware` | All routes | Request ID, method, path, status, latency |
| `Decompress` | Requests with `Content-Encoding: gzip` | Gunzip body; cap decompressed size (`MAX_DECOMPRESSED_BODY_BYTES`) — zip-bomb guard |
| `AllowMethods` | Per route | Wrong verb → **405** `method_not_allowed` |
| `RateLimit` | All routes (stricter on `/auth/*`) | Token bucket per IP; **429** + `Retry-After` |
| `MaxBodyBytes` | All routes | Cap compressed upload size → **413** |
| `ContentType` | JSON routes | Require `application/json` → **415** |
| `Auth` | Protected routes | JWT + DB session → **401** |
| `RBAC` | Role routes | `RequireAdmin` / `RequireMember` → **403** |
| `Compress` | Response path (outer wrap) | `Accept-Encoding: gzip` + response ≥ min size → gzip JSON |

**Implementation:** stdlib `compress/gzip` only — no third-party compression libs.

### Response compression (`middleware/compress.go`)

- Wrap `http.ResponseWriter` with a gzip writer when client sends `Accept-Encoding: gzip` (or `gzip, deflate` — support **gzip** only for simplicity).
- Skip compression when:
  - Response smaller than `GZIP_MIN_RESPONSE_BYTES` (default **1024** bytes) — avoids overhead on tiny errors
  - Handler already set `Content-Encoding`
  - Status is `204 No Content`
- Set headers: `Content-Encoding: gzip`, `Vary: Accept-Encoding`; remove `Content-Length` (gzip body size unknown until complete).
- **Implementation:** buffer the full handler response first, then gzip if status + size warrant it — avoids garbled JSON from streaming gzip writers on small/medium payloads.

### Request decompression (`middleware/decompress.go`)

- When client sends `Content-Encoding: gzip`, replace `r.Body` with `gzip.NewReader` wrapped in `io.LimitReader(..., MAX_DECOMPRESSED_BODY_BYTES)`.
- Reject unsupported encodings (e.g. `br`, `deflate`) → **415** `unsupported_media_type`.
- **Zip-bomb guard:** limit applies to **decompressed** bytes read, not just compressed upload size.
- After decompress, downstream `bodylimit` + `decodeAndSanitizeJSON` see plain JSON.

**Order matters:** `Decompress` → `MaxBodyBytes` (on compressed stream) → `ContentType` → handler.

### `decodeAndSanitizeJSON` unchanged

Runs on already-decompressed body; no gzip awareness in controller.

**Design:** Gzip is middleware — compress responses for large ledger pages; accept gzip request bodies with a decompressed size cap to prevent zip bombs.

```go
// Global outer wrap: compress responses + decompress requests (all routes)
handler := middleware.Compress(middleware.Decompress(mux))

// Public — POST only
mux.Handle("POST /auth/login", chain(logging, allowPOST, authRateLimit, ctrl.Auth.Login))

// Member — ledger benefits from Accept-Encoding: gzip on large pages
mux.Handle("GET /accounts/me/ledger", chain(logging, allowGET, rateLimit, auth, requireMember, ctrl.Ledger.MyLedger))

// Admin
mux.Handle("POST /batch/transactions", chain(logging, allowPOST, rateLimit, auth, requireAdmin, ctrl.Batch.Upload))
```

**Rate limit config:**

| Env | Default | Notes |
|-----|---------|-------|
| `RATE_LIMIT_RPS` | `10` | Sustained requests/sec per IP |
| `RATE_LIMIT_BURST` | `20` | Burst allowance |
| `AUTH_RATE_LIMIT_RPS` | `5` | Stricter on login/forgot-password (brute-force mitigation) |

In-memory limiter is fine for local dev; SOLUTION.md notes Redis for multi-instance prod.

### Connection pool (`dao/postgres/pool.go`)

```go
db.SetMaxOpenConns(cfg.MaxOpenConns)       // default 10
db.SetMaxIdleConns(cfg.MaxIdleConns)       // default 5
db.SetConnMaxLifetime(cfg.ConnMaxLifetime)
db.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)
```

Graceful shutdown: drain HTTP, then `db.Close()`.

### Composition root (`cmd/server/main.go`)

Config → pool → **`batch.RecoverStaleJobs()`** → DAOs → services → controllers → router → `http.Server`.

### Testability matrix

| Layer | Test | Mock/replace |
|-------|------|--------------|
| Models | Unit | none |
| Service | Unit | mock DAO |
| Controller | httptest | mock service |
| DAO | Postman + unit mocks | real Postgres exercised via HTTP collection |

**Pattern rationale:** Separating router/controller/service/DAO/models gives clear test boundaries and a maintainable structure. Business rules live once in services; SQL lives only in DAO; HTTP concerns never leak inward. Monolithic `handlers.go` rejected — hard to test and reuse (e.g. batch vs API both need same spend logic).

---

## Part 5 — API surface (minimal)

### Endpoint matrix (method + role enforced)

| Method | Path | Middleware | Role | Purpose |
|--------|------|------------|------|---------|
| POST | `/auth/login` | rate-limit(auth) | Public | Login |
| POST | `/auth/logout` | auth | Any authed | Logout → **204** |
| POST | `/auth/forgot-password` | rate-limit(auth) | Public | Forgot password |
| POST | `/auth/reset-password` | rate-limit(auth) | Public | Reset password |
| GET | `/accounts` | auth, admin | Admin | Paginated list of active accounts |
| POST | `/accounts` | auth, admin | Admin | Create account `{ ..., role: "member" \| "admin" }` |
| PATCH | `/accounts/{id}` | auth, admin | Admin | Update name/email/role |
| DELETE | `/accounts/{id}` | auth, admin | Admin | Soft delete account |
| GET | `/accounts/{id}` | auth, admin | Admin | Account info |
| GET | `/accounts/{id}/balance` | auth, admin | Admin | Any balance |
| GET | `/accounts/{id}/ledger` | auth, admin | Admin | Paginated ledger (`limit`, `offset`) |
| GET | `/accounts/me/balance` | auth, member | Member | Own balance |
| PATCH | `/accounts/me` | auth, member | Member | Update own name/email |
| DELETE | `/accounts/me` | auth, member | Member | Soft delete own account |
| GET | `/accounts/me/ledger` | auth, member | Member | Paginated own ledger |
| POST | `/transactions` | auth, member | Member | Earn/spend own; **`Idempotency-Key` or body `ref`** |
| POST | `/accounts/{id}/transactions` | auth, admin | Admin | Adjust any account; **`Idempotency-Key` or body `ref`** |
| POST | `/batch/transactions` | auth, admin | Admin | Upload CSV → **202** `{ batch_job_id }` |
| GET | `/batch/jobs/{id}` | auth, admin | Admin | Poll status + summary when `completed` |
| GET | `/batch/jobs/{id}/audit` | auth, admin | Admin | Paginated per-row audit (accepted + rejected) |
| GET | `/health` | — | Public | Liveness check |

Unlisted methods on these paths → **405**. No route registered for unknown paths → **404**.

**List endpoints** (ledger + accounts; same pagination pattern):

| Query param | Default | Max | Notes |
|-------------|---------|-----|-------|
| `limit` | `20` | `100` (`PAGINATION_MAX_LIMIT`) | Omit → 20; specify higher up to max |
| `offset` | `0` | — | Must be `>= 0`; negative → **400** |

DAO uses `LIMIT $limit OFFSET $offset` with a separate `COUNT(*)` for `total_count` (or window function in one query if preferred).

---

## Part 5b — Standardized responses and status codes

### Success envelope (single resource)

```json
{
  "data": { ... }
}
```

### Success envelope (paginated list)

```json
{
  "data": [
    {
      "ref": "tx-001",
      "account_id": "member-123",
      "kind": "earn",
      "direction": "credit",
      "points": 150,
      "balance_after_points": 150,
      "occurred_at": "2024-06-01T10:00:00Z",
      "recorded_at": "2024-06-01T10:00:01Z",
      "actor_account_id": "member-123",
      "source": "api"
    }
  ],
  "pagination": {
    "limit": 20,
    "offset": 0,
    "total_count": 45,
    "has_more": true
  }
}
```

- **`has_more`:** `(offset + len(data)) < total_count` — client knows whether to fetch next page.
- Sort order: **`recorded_at DESC`** (newest first) for ledger lists.

### Pagination validation (`models/dto/pagination.go`)

```go
type PaginationQuery struct {
    Limit  int
    Offset int
}

func (p PaginationQuery) WithDefaults(defaultLimit, maxLimit int) PaginationQuery
func (p PaginationQuery) Validate(maxLimit int) error  // limit 1..maxLimit, offset >= 0
```

Parsed in **controller** from `r.URL.Query()` before calling service — invalid values → **400** `validation_error`.

### Error envelope (all non-2xx)

```json
{
  "error": {
    "code": "insufficient_balance",
    "message": "Spend would exceed available balance",
    "status": 422
  }
}
```

Implemented in `controller/response.go` — single `writeError(w, apiErr)` used by all controllers.

### Status code contract

| HTTP | `error.code` | When |
|------|--------------|------|
| **413** | `payload_too_large` | Request body exceeds `MAX_REQUEST_BODY_BYTES` |
| **415** | `unsupported_media_type` | Missing or wrong `Content-Type` on JSON routes |
| **400** | `validation_error` / `invalid_role` | Bad body/query; `role` not member or admin |
| **401** | `unauthorized` | Missing token, invalid JWT, expired/revoked session |
| **403** | `forbidden` | Valid token but wrong role, or member accessing another account |
| **404** | `not_found` | Account or resource not found |
| **405** | `method_not_allowed` | HTTP method not allowed on this path |
| **409** | `duplicate_ref` | Same `ref` already accepted (API retry, batch reprocess, or concurrent duplicate) |
| **409** | `email_already_exists` | Unique email constraint |
| **422** | `insufficient_balance` | Spend or adjustment debit would drive balance below zero |
| **429** | `rate_limit_exceeded` | Too many requests; include `Retry-After` header |
| **500** | `internal_error` | Unexpected failure; generic message to client, details in server logs |

### Validation before business logic

1. **Middleware** — method, rate limit, body size, content-type (JSON routes), auth, role.
2. **Controller** — `decodeAndSanitizeJSON` **or** parse/validate query params (`PaginationQuery`) → if invalid, return **4xx** immediately (never call service).
3. **Service** — domain/business rules → return typed `models.APIError` or sentinel errors.
4. **Controller** — map service errors to status codes via central `MapDomainError(err) APIError`.

**DTO validation** (`models/dto/*.go`):

```go
func (r *CreateAccountRequest) Sanitize() {
    r.AccountID = sanitizeID(r.AccountID)
    r.Name = sanitizeText(r.Name, 128)
    if e, err := models.NormalizeEmail(r.Email); err == nil { r.Email = e }
    r.Password = strings.TrimSpace(r.Password)
    r.Role = strings.ToLower(strings.TrimSpace(r.Role))
    if r.Role == "" { r.Role = "member" }
}

func (r CreateAccountRequest) Validate() error {
    if r.AccountID == "" { return models.ErrFieldRequired("account_id") }
    if err := models.ValidateEmail(r.Email); err != nil { return err }
    if r.Role != "member" && r.Role != "admin" { return models.ErrInvalidRole }
    // password strength checks...
    return nil
}
```

**Service rule (`service/auth` or `service/wallet`):** `CreateAccount` persists `accounts.role` from request. Caller must already be admin (enforced by middleware); no bootstrap path for non-admin to set `role: admin`.

**Validation order:** Validation runs in the controller before the service — middleware handles cross-cutting auth; business rules stay in the service layer.

**Create request (admin-only; role distinguishes member vs admin):**

```json
{
  "account_id": "member-123",
  "name": "Rina",
  "email": "rina@example.com",
  "password": "changeme",
  "role": "member"
}
```

```json
{
  "account_id": "admin-2",
  "name": "Sam",
  "email": "sam@example.com",
  "password": "changeme",
  "role": "admin"
}
```

- **`role`** — required enum: `"member"` | `"admin"`. If omitted, defaults to **`member`**.
- Only an authenticated **admin** may create accounts; only **`role: "admin"`** creates another admin (no self-registration of admins).
- Response includes `role` so caller can confirm what was created:

```json
{
  "data": {
    "account_id": "member-123",
    "name": "Rina",
    "email": "rina@example.com",
    "role": "member",
    "balance_points": 0
  }
}
```

Note in README: assignment minimal account JSON is `{ account_id, name }`; we extend with `email`, `password`, and **`role`** for auth and RBAC.

**Example balance response:**

```json
{
  "account_id": "member-123",
  "email": "rina@example.com",
  "balance_points": 150
}
```

**Example ledger entry (response):**

```json
{
  "ref": "tx-001",
  "account_id": "member-123",
  "kind": "earn",
  "direction": "credit",
  "points": 150,
  "balance_after_points": 150,
  "occurred_at": "2024-06-01T10:00:00Z",
  "recorded_at": "2024-06-01T10:00:01Z",
  "actor_account_id": "member-123",
  "source": "api"
}
```

**Example admin adjustment (response):**

```json
{
  "ref": "adj-001",
  "account_id": "member-123",
  "kind": "adjustment",
  "direction": "credit",
  "points": 25,
  "balance_after_points": 175,
  "occurred_at": "2024-06-01T12:00:00Z",
  "recorded_at": "2024-06-01T12:00:01Z",
  "actor_account_id": "admin",
  "source": "api"
}
```

**Example transaction request (member earn):**

```json
{
  "kind": "earn",
  "direction": "credit",
  "points": 150,
  "occurred_at": "2024-06-01T10:00:00Z"
}
```

**Example transaction request (member spend):**

```json
{
  "kind": "spend",
  "direction": "debit",
  "points": 50,
  "occurred_at": "2024-06-01T11:00:00Z"
}
```

Assignment PDF omits `direction`; our REST API **requires** it. (`points`: 150 → `15000` point-cents in DB.)

**Example batch upload response (202 Accepted):**

```json
{
  "data": {
    "batch_job_id": "550e8400-e29b-41d4-a716-446655440000",
    "status": "queued",
    "total_rows": 150
  }
}
```

**Example batch job status (200 OK when completed):**

```json
{
  "data": {
    "batch_job_id": "550e8400-e29b-41d4-a716-446655440000",
    "status": "completed",
    "total_rows": 150,
    "accepted": 140,
    "rejected": 5,
    "duplicates": 5,
    "started_at": "2024-06-01T10:00:01Z",
    "completed_at": "2024-06-01T10:00:05Z"
  }
}
```

---

## Part 5c — README requirements (assignment alignment)

README must include **example requests/responses for every endpoint** (Task 1 requirement). Highlight assignment-compatible shapes:

### Account (assignment minimum + extensions)

**Assignment shape:**

```json
{ "account_id": "member-123", "name": "Rina" }
```

**Create request (extension for auth + role):**

```json
{
  "account_id": "member-123",
  "name": "Rina",
  "email": "rina@example.com",
  "password": "changeme",
  "role": "member"
}
```

```json
{
  "account_id": "admin-2",
  "name": "Sam",
  "email": "sam@example.com",
  "password": "changeme",
  "role": "admin"
}
```

Note in README: assignment minimal account JSON is `{ account_id, name }`; extensions are `email`, `password`, and **`role`** (`member` | `admin`, default `member`).

### Transaction (assignment vs API)

**Assignment shape (admin / CSV — no direction in PDF):**

```json
{
  "ref": "tx-001",
  "account_id": "member-123",
  "kind": "earn",
  "points": 150,
  "occurred_at": "2024-06-01T10:00:00Z"
}
```

**Our API extension (direction required on REST):**

```json
{
  "kind": "earn",
  "direction": "credit",
  "points": 150,
  "occurred_at": "2024-06-01T10:00:00Z"
}
```

**Member `POST /transactions`** — body `{kind, direction, points, occurred_at}`; **`Idempotency-Key` header** preferred (body `ref` optional fallback); `account_id` inferred from JWT `sub`. **`adjustment` forbidden** for members.

**Admin `POST /accounts/{id}/transactions`** — `account_id` in path; **`kind: adjustment`** with **`direction: credit | debit`**; **`actor_account_id`** in response = admin.

**CSV** — minimum header: `ref,account_id,kind,points,occurred_at` (direction inferred for `earn`/`spend`). Optional 6th column: **`direction`** (required for `adjustment` rows).

### Async batch flow (document clearly)

```text
1. POST /batch/transactions  → 202 { batch_job_id, status: "queued", total_rows }
2. GET  /batch/jobs/{id}     → poll until status is "completed" or "failed"
3. Read summary: total_rows, accepted, rejected, duplicates
4. GET  /batch/jobs/{id}/audit?limit=20  → paginated per-row audit (accepted + rejected)
```

Include curl examples for upload, poll loop, and audit list.

### Auth (Task 2 — document in README summary + detail in SOLUTION.md)

- Header: `Authorization: Bearer <access_token>`
- JWT claims: `sub` (account_id), `role` (`member`|`admin`), `jti`, `exp`
- Session validated against DB (`auth_tokens` table) — not stateless; JWT `jti` stored in `auth_tokens.token`
- Logout returns **204 No Content**
- Point to **SOLUTION.md** for full token lifecycle (ongoing doc)

---

## Part 5d — SOLUTION.md (ongoing)

Maintain **while building** — not a one-shot at the end. Sections to fill incrementally:

| Section | When to write |
|---------|----------------|
| Architecture diagram | After scaffold |
| Auth: token shape, storage, validation, RBAC | After auth middleware |
| Idempotency + ledger + audit split | After wallet/batch |
| Async batch + restart recovery | After batch processor |
| Tradeoffs (Postgres vs SQLite, 202 async, integer points, extras) | As decisions are made |
| AI prompts / what you accepted or edited | Ongoing during build |

Task 2 explicitly requires documenting credential shape and enforcement — keep that section current in SOLUTION.md.

---

## Part 6 — Implementation order

| Step | Item | Status |
|------|------|--------|
| 1 | Scaffold — compose, migrations (`001`, `002`, **`003`**), models, DAOs, pool, **`RecoverStaleJobs()` on startup** | ✅ Done |
| 2 | Services — wallet/auth/batch with unit tests | ✅ Done |
| 3 | Controllers + router + middleware — standardized errors; README curl examples | ✅ Done |
| 4 | Auth — JWT, `auth_tokens`, RBAC; SOLUTION.md auth section | ✅ Done |
| 5 | Batch — async processor; audit every row; 202 + GET job + GET audit; restart recovery | ✅ Done |
| 6 | Account CRUD — list/update/soft-delete; last-admin guard | ✅ Done |
| 7 | API idempotency — `Idempotency-Key` header + body `ref` fallback | ✅ Done |
| 11 | Admin adjustment audit — `direction` on ledger + API; required on REST; `actor_account_id`; member `adjustment` → 403 | ✅ Done |
| 12 | Unit tests — wallet service, dto, points, **direction**, compress middleware | ✅ Done (partial vs full matrix below) |
| 13 | Postman collection — single file; admin adjust credit/debit; direction on all txs | ✅ Done |
| 14 | README + SOLUTION.md | ✅ Done |

---

## Part 7 — Testing strategy

Two layers: **Go unit tests** (fast, mocked) and **Postman integration tests** (full HTTP stack, importable).

### 7a — Unit tests (mocks per layer)

**Principle:** Each layer tested in isolation with **hand-written mocks** implementing DAO/service interfaces — no code generation, stdlib `testing` only, table-driven where useful.

**Mock location:** inline mocks in `*_test.go` files (e.g. `service/wallet/service_test.go`). Planned `internal/testutil/mocks/` package not yet extracted.

| Package | What to test | Mock | Status |
|---------|--------------|------|--------|
| `models/dto` | `ResolveTransactionRef`, validation | none | ✅ `requests_test.go` |
| `models` | Points conversion; **`ResolveTransactionDirection`** | none | ✅ `points_test.go`, `direction_test.go` |
| `service/wallet` | Idempotency, insufficient balance, **adjustment debit** | inline `mockWalletDAO` | ✅ `service_test.go` |
| `service/auth` | Single session, login/logout flow | `MockAuthDAO` | ⏳ Not yet |
| `service/batch` | CSV row outcomes, summary counts | mocks | ⏳ Not yet |
| `controller/*` | HTTP status mapping, validation | mock service | ⏳ Not yet |
| `router/middleware` | gzip buffer, decompress | stub handler | ✅ `compress_test.go` (partial) |

**Example pattern:**

```go
func TestWalletService_Spend_InsufficientBalance(t *testing.T) {
    dao := mocks.NewMockWalletDAO()
    dao.Balance = models.PointsFromWhole(100) // 100 whole points = 10000 point-cents
    svc := wallet.NewService(dao, testLogger)
    err := svc.Spend(ctx, "member-1", spendTx(200))
    if !errors.Is(err, models.ErrInsufficientBalance) { t.Fatal(...) }
    if dao.ApplyTransactionCalls != 0 { t.Fatal("DAO must not be called on pre-check failure") }
}
```

**Run:** `go test ./... -count=1` (README documents this).

### 7b — Postman integration tests (importable)

Deliverables in `postman/` — **import single collection file** (no separate environment required).

**Files:**

| File | Purpose |
|------|---------|
| `PointsWallet.postman_collection.json` | All requests, test scripts, **collection-scoped variables** |
| `sample-batch*.csv` | Demo CSVs (success, rejects, original) |
| `README.md` | Import steps, Collection Runner order, negative test notes |

**Collection variables** (defaults in collection; updated by test scripts):

| Variable | Set by | Used for |
|----------|--------|----------|
| `baseUrl` | collection default | `http://localhost:8080` |
| `adminEmail`, `adminPassword` | collection default | Admin login |
| `memberAccountId`, `memberEmail`, `memberPassword` | collection default / create | Member flows |
| `adminToken`, `memberToken` | login test scripts | `Authorization: Bearer {{token}}` |
| `adminAccountId` | Admin Login test script | Assert admin `actor_account_id` on adjustments |
| `idempotencyKey` | pre-request script (spend/admin) | `Idempotency-Key` header |
| `batchJobId` | batch upload test | Poll + audit follow-up |
| `resetToken` | forgot-password test | Reset password flow |

**Collection folder structure (as built):**

```
Points Wallet API
├── 01 Auth
│   ├── Admin Login (saves adminToken)
│   ├── Member Login (saves memberToken)
│   ├── Logout (Admin) / Logout (Member) → 204
│   ├── Forgot Password / Reset Password
├── 02 Accounts (Admin)
│   ├── Create Member / Create Admin
│   ├── List Accounts
│   ├── Get / Update / Delete Account
│   ├── Get Account Balance / Ledger
├── 03 Wallet (Member)
│   ├── My Balance / Update My Account / Delete My Account
│   ├── Earn Points (direction credit; Idempotency-Key: tx-001)
│   ├── Spend Points (direction debit; pre-request UUID idempotencyKey)
│   └── My Ledger
├── 04 Admin Transactions
│   ├── Admin Adjust Account (adjustment + direction credit; asserts actor_account_id)
│   └── Admin Adjust Debit (adjustment + direction debit)
├── 05 Batch (Admin)
│   ├── Upload CSV → 202
│   ├── Get Batch Job Status
│   └── Get Batch Audit
├── 06 Negative Cases
│   ├── Duplicate Ref (409) — replays tx-001
│   ├── Insufficient Balance (422)
│   └── Member Create Account (403)
└── Health
    └── GET /health
```

**Run order:** Admin Login → Create Member → Member Login → Earn → (negatives before logout) → Batch → Logout last. See `postman/README.md` for Collection Runner notes (logout revokes member token).

**How to run:**

1. **Postman UI:** Import `PointsWallet.postman_collection.json` → **Collection Runner** → run folders in order.
2. **Newman CLI (optional):** `newman run postman/PointsWallet.postman_collection.json` — no `-e` flag needed.

**Prerequisite in README:** `docker compose up -d` + `go run ./cmd/server` before Postman run.

### 7c — Test checklist (quick reference)

| Test | Proves |
|------|--------|
| `TestMockWalletDAO_ImplementsInterface` | Mock compiles and behaves as expected |
| `TestWalletService_Spend_InsufficientBalance` | Mock DAO; service rule isolated |
| `TestMiddleware_CompressResponse` | Accept-Encoding gzip + large JSON → Content-Encoding gzip |
| `TestMiddleware_DecompressRequest` | gzip request body decoded correctly |
| `TestMiddleware_DecompressZipBomb` | oversized decompressed stream → 413 |
| `TestDecode_UnknownJSONFields` | Extra keys → 400 |
| `TestDecode_FloatPointsRejected` | `"points": 1.5` → 400 at decode |
| `TestDTO_Sanitize_TrimsAndStripsControlChars` | Sanitize normalizes input |
| `TestMiddleware_MaxBodyBytes` | Oversized body → 413 |
| `TestController_ValidationBeforeService` | Mock service never called on bad input |
| `TestCreateAccount_AdminCreatesMember` | role=member default or explicit → 201 |
| `TestCreateAccount_AdminCreatesAdmin` | role=admin → 201; new admin login works |
| `TestCreateAccount_InvalidRole` | role=superuser → 400 |
| `TestCreateAccount_MemberForbidden` | member token on POST /accounts → 403 |
| `TestMiddleware_RequireAdmin` | Member token → 403 on admin route |
| `TestMiddleware_AllowMethods` | GET on POST-only → 405 |
| `TestLedger_ListPagination_DefaultLimit` | Default limit 20 |
| `TestApplyTransaction_DuplicateRef_API` | Second POST same ref → 409, balance unchanged |
| `TestApplyTransaction_DuplicateRef_BatchRow` | Duplicate ref in CSV → duplicate_count++ |
| `TestApplyTransaction_DuplicateRef_Concurrent` | Two goroutines same ref → one wins |
| `TestApplyTransaction_DuplicateRef_CrossEntry` | API ref then batch same ref → rejected |
| `TestBatch_AsyncReturns202` | Upload returns job id before processing finishes |
| `TestBatch_ConcurrentDifferentAccounts` | Parallel rows on different accounts all commit |
| `TestBatch_ConcurrentSameAccount` | Parallel rows same account — balance stays correct |
| `TestBatch_RowFailureRollback` | Failed row rolls back; prior committed rows kept |
| `TestBatch_AuditLogsAcceptedRows` | Accepted batch row → audit_events status accepted reason ok |
| `TestBatch_AuditLogsRejectedRows` | Rejected row → audit_events with reason |
| `TestBatch_RecoverStaleJobsOnStartup` | processing job → failed after simulated restart |
| `TestBatch_ReprocessCSV` | Re-upload → duplicates; balance unchanged |
| Postman: upload then poll GET /batch/jobs/{id} | Full async flow |

---

## Part 8 — SOLUTION.md talking points (ongoing) + demo prep

**SOLUTION.md** is updated continuously during build (see Part 5d). Key decisions to explain:

**Decisions to explain:**

- **Integer point-cents, not floats** — fixed-scale storage (×100); assignment API uses whole integer points; ready for fractional points later without schema change.
- **Postgres + Compose** — durable, production-scale backend; row locks vs SQLite.
- **JWT + DB `auth_tokens`** — revocation, single session, reset invalidates all tokens.
- **Soft delete + last-admin guard** — preserve ledger FK integrity; anonymize email; block orphaning the system.
- **`Idempotency-Key` header** — REST-friendly idempotency without requiring body `ref` on every request.
- **Admin `adjustment` + required `direction`** — credit/debit audit on ledger; `actor_account_id` identifies admin without denormalizing profile fields.
- **Ledger `actor_account_id` + `direction` in API** — who performed each action and whether points were credited or debited.
- **Batch audit trail** — every batch row in `audit_events` (accepted + rejected); ledger for accepted financial record.
- **Async batch + restart** — 202 poll flow; stale `processing` jobs → `failed` on boot; re-upload safe via idempotent refs.
- **Immutable ledger** — append-only `ledger_entries`; balance is a cache.
- **Denormalized balance_points + ledger** — fast reads + permanent audit trail for every accepted action.
- **Stdlib HTTP** — transparent, readable request handling without framework magic.
- **Email validation at two layers** — backend gives friendly errors; DB CHECK + UNIQUE is the safety net (defense in depth).
- **Layered architecture** — Router/Controller/Service/DAO/Models; each layer independently testable.
- **Ports at DAO boundary** — swap Postgres without touching services or controllers.
- **Global idempotency on `ref`** — one `ApplyTransaction` path for API + batch; DB UNIQUE + service check; safe retries and CSV reprocess.
- **Async batch processing** — 202 + worker pool; rows concurrent across accounts; per-row DB txn with rollback on failure.
- **Gzip compression** — stdlib `compress/gzip`; compress large responses; accept gzip requests with decompressed size cap.
- **Request body pipeline** — strict JSON deserialize, sanitize, validate before service.
- **Middleware pipeline** — method allowlist, rate limiting, body size, content-type, auth, RBAC.
- **Paginated list APIs** — default `limit=20`, configurable up to max; `{ data, pagination }` response shape.
- **Standardized errors** — consistent JSON `{ error: { code, message, status } }` across all endpoints.
- **Two-tier testing** — Go unit tests with hand-written mocks (fast, per-layer); Postman collection (importable, full-stack positive/negative).
- **Capped connection pool** — explicit `MaxOpenConns`/`MaxIdleConns` instead of Go defaults (unlimited open).

**Threat model (STRIDE-lite):** Bearer token theft → short expiry + HTTPS in prod; bcrypt passwords; admin-only batch; parameterized SQL only; forgot-password returns generic response to avoid email enumeration; rate limiting on auth endpoints mitigates credential stuffing; RBAC middleware enforces least privilege per route.

---

## Out of scope (explicitly defer)

- Kubernetes, production CI/CD
- Frontend UI
- Real email for forgot-password
- Multi-tenant org hierarchy

---

## Implementation status (current)

| Area | Status |
|------|--------|
| Assignment Tasks 1–3 (wallet, auth/RBAC, batch) | ✅ Complete |
| Account list / update / soft delete | ✅ Complete |
| `Idempotency-Key` header | ✅ Complete |
| Admin adjustment (`kind` + `direction` + `actor_account_id`) | ✅ Complete |
| Migration `003_ledger_direction.sql` | ✅ Complete |
| Postman single-file collection | ✅ Complete |
| Unit tests (wallet, dto, points, direction, compress) | ✅ Partial |
| Static analysis (`staticcheck`) | ✅ Done |

See [SOLUTION.md](SOLUTION.md) for rationale, threat model, and full AI prompt log.
