# Points Wallet

Loyalty points wallet service (Go + PostgreSQL).

## Features

- Accounts with earn/spend, balance, global idempotent `ref`, no negative balance
- API idempotency via **`Idempotency-Key` header** (falls back to JSON `ref`); batch CSV still uses `ref` column
- JWT + DB-backed tokens (`auth_tokens`), `member` / `admin` RBAC
- Admin creates members or admins via `POST /accounts` with `role`
- Admin list/update/soft-delete accounts; members update/delete own profile
- Ledger exposes **`actor_account_id`** (who performed the action) and **`direction`** (`credit` | `debit`) on every entry
- Integer **point-cents** storage (×100 scale); API uses whole integer `points`

## Documentation

| Document | Contents |
|----------|----------|
| **[PLAN.md](PLAN.md)** | **Implementation plan** (flow-first design, updated to match codebase): flows, schema, API, business rules, status |
| **[SOLUTION.md](SOLUTION.md)** | Design rationale, tradeoffs, threat model, prompt history |
| **[postman/README.md](postman/README.md)** | Postman setup and test walkthrough |

## Prerequisites

Install and verify these before starting:

| Tool | Version | Check |
|------|---------|--------|
| **Go** | 1.22 or later | `go version` |
| **Docker** | Recent (Docker Desktop or Engine) | `docker --version` |
| **Docker Compose** | v2 (included with Docker Desktop) | `docker compose version` |
| **Git** | Any recent | `git clone` this repo |
| **curl** | Optional, for API examples | `curl --version` |
| **jq** | Optional, pretty JSON output | `jq --version` |
| **Postman** | Optional, for collection import | — |

**Ports:** PostgreSQL uses **5432** and the API uses **8080** on localhost. Ensure nothing else is bound to those ports.

**OS:** macOS, Linux, or Windows with WSL2 + Docker.

---

## Getting started (step by step)

Run these from the **repository root** (`pointswallet/`). The server loads migrations from `./migrations/` relative to where you run the command.

### Step 1 — Clone the repository

```bash
git clone <your-repo-url>
cd pointswallet
```

### Step 2 — Configure environment

```bash
cp .env.example .env
```

Defaults work for local development. The server **auto-loads `.env`** from the repo root on startup (no need to `export` vars manually). To change the bootstrap admin or JWT secret, edit `.env`:

- `ADMIN_EMAIL` / `ADMIN_PASSWORD` / `ADMIN_ACCOUNT_ID` — bootstrap admin (seeded on server start; see below)
- `JWT_SECRET` — change before any non-local use
- `SINGLE_ACTIVE_SESSION` — `true` (default) keeps one active login per user; set `false` to allow multiple devices/sessions
- `DATABASE_URL` — only if Postgres is not on `localhost:5432`

### Step 3 — Start PostgreSQL

```bash
docker compose up -d
```

Wait until Postgres is healthy:

```bash
docker compose ps
```

You should see `postgres` with state **running** (health: healthy).

### Step 4 — Install Go dependencies

```bash
go mod download
```

### Step 5 — Run the server

```bash
go run ./cmd/server
```

On success you will see a log line like `server starting` with `addr` `:8080`.

The app will:

1. Connect to Postgres using `DATABASE_URL`
2. Apply SQL migrations in `migrations/` (`001_init.sql`, `002_accounts_soft_delete.sql`)
3. Seed the **bootstrap admin** from env (if `ADMIN_ACCOUNT_ID` is not already in `accounts`)

**Docker does not seed data.** Only `go run ./cmd/server` runs migrations and admin seed. Test members are created via `POST /accounts` (Postman **Create Member** or curl below).

After changing Go code, restart the server so fixes take effect (`Ctrl+C`, then `go run ./cmd/server` again).

### Step 6 — Verify the service

```bash
curl -s http://localhost:8080/health
```

Expected: `{"status":"ok"}`

**Admin login:**

```bash
curl -s -X POST http://localhost:8080/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"admin@example.com","password":"admin123"}'
```

Expected: JSON with `data.access_token`.

### Step 7 — Run unit tests (optional)

From the repo root — **no Docker or running server required**:

```bash
go test ./...
```

Verbose output:

```bash
go test ./... -v
```

Coverage:

```bash
go test ./... -cover
```

| Package | What is tested |
|---------|----------------|
| `internal/models` | Point-cents conversion and arithmetic |
| `internal/models/dto` | Idempotency ref resolution (header vs body) |
| `internal/service/wallet` | Insufficient balance, duplicate `ref` (mock DAOs) |
| `internal/router/middleware` | Gzip response compression |

Integration / end-to-end testing uses Postman with the server running (steps 3–5).

### Step 8 — Postman (full walkthrough)

See **[postman/README.md](postman/README.md)** for step-by-step import, run order, batch CSV attachment, Collection Runner, and troubleshooting.

Quick version:

1. Import `postman/PointsWallet.postman_collection.json` (one file, no environment)
2. Start server (`go run ./cmd/server`) — admin is seeded automatically
3. Run **Health** → **Admin Login** → **Create Member** → **Member Login** → **Earn Points** → **My Balance**
4. Run **06 Negative Cases** after earn (skip **Logout** until the end — logout revokes tokens)
5. For batch: attach `postman/sample-batch.csv` on **Upload CSV**, then poll **Get Batch Job Status**

---

## Stop / reset

**Stop the API:** `Ctrl+C` in the terminal running `go run`.

**Stop Postgres:**

```bash
docker compose down
```

**Stop Postgres and delete all data** (fresh DB on next start):

```bash
docker compose down -v
```

---

## Quick start (curl)

**Login as admin:**

```bash
curl -s -X POST http://localhost:8080/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"admin@example.com","password":"admin123"}' | jq
```

**Create member** (409 if `member-1` or email already exists):

```bash
TOKEN=<access_token from login>

curl -s -X POST http://localhost:8080/accounts \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "account_id": "member-1",
    "name": "Rina",
    "email": "rina@example.com",
    "password": "changeme1",
    "role": "member"
  }' | jq
```

**Logout** (same endpoint for admin or member; returns **204** with empty body):

```bash
curl -s -o /dev/null -w "%{http_code}\n" -X POST http://localhost:8080/auth/logout \
  -H "Authorization: Bearer $TOKEN"
```

**Create admin:**

```bash
curl -s -X POST http://localhost:8080/accounts \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "account_id": "admin-2",
    "name": "Sam",
    "email": "sam@example.com",
    "password": "changeme1",
    "role": "admin"
  }' | jq
```

**Member earn** (login as member first; idempotency key in header):

```bash
curl -s -X POST http://localhost:8080/transactions \
  -H "Authorization: Bearer $MEMBER_TOKEN" \
  -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: tx-001' \
  -d '{
    "kind": "earn",
    "direction": "credit",
    "points": 150,
    "occurred_at": "2024-06-01T10:00:00Z"
  }' | jq
```

Legacy body `ref` still works when the header is omitted.

**Balance:**

```bash
curl -s http://localhost:8080/accounts/me/balance \
  -H "Authorization: Bearer $MEMBER_TOKEN" | jq
```

**Negative examples** (member token required):

```bash
# Duplicate idempotency key → 409 duplicate_ref (replay same header as earn)
curl -s -X POST http://localhost:8080/transactions \
  -H "Authorization: Bearer $MEMBER_TOKEN" \
  -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: tx-001' \
  -d '{"kind":"earn","direction":"credit","points":10,"occurred_at":"2024-06-01T10:00:00Z"}' | jq

# Insufficient balance → 422 insufficient_balance
curl -s -X POST http://localhost:8080/transactions \
  -H "Authorization: Bearer $MEMBER_TOKEN" \
  -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: tx-overdraw' \
  -d '{"kind":"spend","direction":"debit","points":999999,"occurred_at":"2024-06-01T10:00:00Z"}' | jq
```

## Batch CSV (async)

Upload returns **202**; poll job status until `completed`.

```bash
curl -s -X POST http://localhost:8080/batch/transactions \
  -H "Authorization: Bearer $TOKEN" \
  -F 'file=@batch.csv' | jq

curl -s http://localhost:8080/batch/jobs/<batch_job_id> \
  -H "Authorization: Bearer $TOKEN" | jq
```

CSV header: `ref,account_id,kind,points,occurred_at`

## Troubleshooting

| Issue | Fix |
|-------|-----|
| `ping db: connection refused` | Run `docker compose up -d` and wait for healthy Postgres |
| `run migrations: no such file` | Run `go run ./cmd/server` from the repo root, not `cmd/server/` |
| Port 5432 or 8080 in use | Stop the other process or change ports in `.env` / `docker-compose.yml` |
| `401` on protected routes | Login again; tokens expire per `JWT_TTL` (default 24h) |
| `401` after logout or Collection Runner | Logout revokes the token — run **Member Login** again before wallet/negative tests |
| `500` on duplicate Create Member | Restart server after pulling latest code (duplicate PK/email should be **409**) |
| Docker daemon not running | Start Docker Desktop / OrbStack, then retry `docker compose up -d` |

| Garbled / binary JSON on ledger | Restart server (gzip fix); or remove `Accept-Encoding: gzip` from request |

## Architecture

Router → Controller → Service → DAO → PostgreSQL

See **[PLAN.md](PLAN.md)** for the implementation plan and **[SOLUTION.md](SOLUTION.md)** for design decisions and tradeoffs.

## API summary

| Method | Path | Role | Notes |
|--------|------|------|-------|
| GET | `/health` | Public | Liveness |
| POST | `/auth/login` | Public | Returns `access_token` |
| POST | `/auth/logout` | Admin or member | Revokes token; **204** empty body |
| POST | `/auth/forgot-password` | Public | Dev returns reset token in response |
| POST | `/auth/reset-password` | Public | Revokes all sessions for account |
| POST | `/accounts` | Admin | Create member or admin (`role`) |
| GET | `/accounts` | Admin | List active accounts (paginated) |
| GET | `/accounts/{id}` | Admin | |
| PATCH | `/accounts/{id}` | Admin | Update name, email, role |
| DELETE | `/accounts/{id}` | Admin | Soft delete (204); preserves ledger |
| PATCH | `/accounts/me` | Member | Update name and email |
| DELETE | `/accounts/me` | Member | Soft delete own account (204) |
| GET | `/accounts/{id}/balance` | Admin | |
| GET | `/accounts/{id}/ledger` | Admin | Paginated; each entry has `actor_account_id` |
| POST | `/accounts/{id}/transactions` | Admin | Adjust any account; `kind: adjustment` with required `direction` (`credit` \| `debit`); `Idempotency-Key` or body `ref` |
| GET | `/accounts/me/balance` | Member | |
| GET | `/accounts/me/ledger` | Member | Paginated; each entry has `actor_account_id` |
| POST | `/transactions` | Member | Earn/spend; required `direction` (`credit` for earn, `debit` for spend); `Idempotency-Key` or body `ref` |
| POST | `/batch/transactions` | Admin | CSV upload → **202** |
| GET | `/batch/jobs/{id}` | Admin | Poll job status |
| GET | `/batch/jobs/{id}/audit` | Admin | Paginated per-row audit |

Common error codes: `duplicate_ref` (409), `insufficient_balance` (422), `forbidden` (403), `unauthorized` (401), `account_already_exists` / `email_already_exists` (409), `last_admin` (409).

Full API design in [PLAN.md](PLAN.md). Error mapping and tradeoffs in [SOLUTION.md](SOLUTION.md).
