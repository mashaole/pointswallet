# Points Wallet

Loyalty points wallet service (Go + PostgreSQL) for the take-home assignment.

## Features

- Accounts with earn/spend, balance, global idempotent `ref`, no negative balance
- JWT + DB sessions, `member` / `admin` RBAC
- Admin creates members or admins via `POST /accounts` with `role`
- Async CSV batch ingestion with per-row audit trail
- Integer **point-cents** storage (×100 scale); API uses whole integer `points`

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

Defaults work for local development. To change the bootstrap admin or JWT secret, edit `.env`:

- `ADMIN_EMAIL` / `ADMIN_PASSWORD` — first admin login
- `JWT_SECRET` — change before any non-local use
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
2. Apply `migrations/001_init.sql`
3. Seed the bootstrap admin from env (if not already present)

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

### Step 7 — Run tests (optional)

```bash
go test ./...
```

Unit tests do not require Docker. Integration testing via Postman requires the server running (steps 3–5).

### Step 8 — Import Postman collection (optional)

1. Open Postman → **Import**
2. Select `postman/PointsWallet.postman_collection.json` (single file; no environment needed)
3. Run **Admin Login** first, then **Create Member**, **Member Login**, etc.

For batch upload, attach `postman/sample-batch.csv` on the **Upload CSV** request.

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

**Create member:**

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

**Member earn (login as member first):**

```bash
curl -s -X POST http://localhost:8080/transactions \
  -H "Authorization: Bearer $MEMBER_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "ref": "tx-001",
    "kind": "earn",
    "points": 150,
    "occurred_at": "2024-06-01T10:00:00Z"
  }' | jq
```

**Balance:**

```bash
curl -s http://localhost:8080/accounts/me/balance \
  -H "Authorization: Bearer $MEMBER_TOKEN" | jq
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
| `401` on admin routes | Login again; tokens expire per `JWT_TTL` (default 24h) |
| Docker daemon not running | Start Docker Desktop / OrbStack, then retry `docker compose up -d` |

## Architecture

Router → Controller → Service → DAO → PostgreSQL

See [SOLUTION.md](SOLUTION.md) for design decisions and tradeoffs.

## API summary

| Method | Path | Role |
|--------|------|------|
| POST | `/auth/login` | Public |
| POST | `/auth/logout` | Authed |
| POST | `/accounts` | Admin |
| GET | `/accounts/me/balance` | Member |
| POST | `/transactions` | Member |
| POST | `/accounts/{id}/transactions` | Admin |
| POST | `/batch/transactions` | Admin |
| GET | `/batch/jobs/{id}` | Admin |

Full matrix in SOLUTION.md.
