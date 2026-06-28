# Postman — setup and test walkthrough

One collection file, no separate environment. All variables live on the collection.

**File to import:** `postman/PointsWallet.postman_collection.json`

**Related docs:** [PLAN.md](../PLAN.md) (API plan) · [README.md](../README.md) (setup) · [SOLUTION.md](../SOLUTION.md) (design notes)

---

## Part 1 — Start the API (terminal)

Do this once before using Postman. Run every command from the **repo root** (`pointswallet/`).

### 1. Install prerequisites

- Go 1.22+
- Docker Desktop (or Docker Engine + Compose)
- [Postman](https://www.postman.com/downloads/) desktop app

Check:

```bash
go version
docker compose version
```

### 2. Configure env

```bash
cp .env.example .env
```

Default admin (used by the collection):

| Variable | Default |
|----------|---------|
| `ADMIN_EMAIL` | `admin@example.com` |
| `ADMIN_PASSWORD` | `admin123` |

### 3. Start PostgreSQL

```bash
docker compose up -d
docker compose ps
```

Wait until `postgres` shows **healthy**.

### 4. Start the server

In a **separate terminal** (keep it running):

```bash
go mod download
go run ./cmd/server
```

You should see JSON log: `"msg":"server starting","addr":":8080"`.

On startup the server applies migrations and seeds the **bootstrap admin** from `.env` (not Docker). Members are created via **Create Member** in Postman.

### 5. Quick sanity check

In Postman or browser: `GET http://localhost:8080/health` → `{"status":"ok"}`

---

## Part 2 — Import the collection

1. Open **Postman**
2. Click **Import** (top left)
3. Drag in **`PointsWallet.postman_collection.json`** or browse to `postman/`
4. Click **Import**
5. In the left sidebar, open **Collections** → **Points Wallet API**

No environment file needed.

### Collection variables (pre-filled)

Open the collection → **Variables** tab:

| Variable | Purpose |
|----------|---------|
| `baseUrl` | `http://localhost:8080` |
| `adminEmail` / `adminPassword` | Bootstrap admin |
| `memberEmail` / `memberPassword` | Test member |
| `memberAccountId` | `member-1` |
| `adminToken` | Set automatically by **Admin Login** |
| `memberToken` | Set automatically by **Member Login** |
| `batchJobId` | Set automatically by **Upload CSV** |
| `idempotencyKey` | Auto-set by pre-request on spend/admin (`{{$guid}}`) |

Leave token variables empty before the first run; scripts fill them after login.

**Idempotency:** API transactions use the **`Idempotency-Key` header** (maps to ledger `ref`). Earn and duplicate tests use fixed `tx-001`; spend/admin auto-generate a UUID per request.

---

## Part 3 — Run requests in order (manual)

Run **one request at a time** top to bottom. Recommended full path:

**Health** → **01 Admin Login** → **02 Create Member** → **01 Member Login** → **03 Wallet** → **04 Admin Adjust** → **05 Batch** → **06 Negative Cases** → **01 Logout** (last)

### Folder: **Health**

| Request | Expected |
|---------|----------|
| GET /health | **200** |

### Folder: **01 Auth**

| # | Request | Expected | Notes |
|---|---------|----------|-------|
| 1 | **Admin Login** | **200** | Saves `adminToken` |
| 2 | **Member Login** | **200** | Run after **Create Member**; saves `memberToken` |
| 3 | **Logout (Admin)** / **Logout (Member)** | **204** | Same `POST /auth/logout` — **run last**; revokes token |
| — | Forgot / Reset Password | — | Optional; skip until happy path works |

**Logout:** one endpoint for all roles. Success is **204** (empty body), not 200.

### Folder: **02 Accounts (Admin)**

Requires **Admin Login** first.

| # | Request | Expected |
|---|---------|----------|
| 1 | **Create Member** | **201** or **409** | **409** = member already exists; continue to login |
| 2 | **Create Admin** | **201** (optional) |
| 3 | **List Accounts** | **200** | Paginated; active accounts only |
| 4 | **Get Account** | **200** |
| 5 | **Update Account (Admin)** | **200** | Name, email, role |
| 6 | **Delete Account (Admin)** | **204** | Soft delete (optional — skip if testing member flow) |
| 7 | **Get Account Balance** | **200**, `balance_points: 0` |
| 8 | **Get Account Ledger** | **200** | Each row includes `actor_account_id` |

**409 codes:** `account_already_exists` (duplicate `account_id`) or `email_already_exists` — not an error for your flow; go to **Member Login**.

### Folder: **03 Wallet (Member)**

Requires **Member Login** (folder **01**) first.

| # | Request | Expected |
|---|---------|----------|
| 1 | **Update My Account** | **200** | Member: name + email only |
| 2 | **Delete My Account** | **204** | Optional — soft-deletes member; skip if continuing tests |
| 3 | **My Balance** | **200** |
| 4 | **Earn Points** | **201** (or **409** if `Idempotency-Key: tx-001` already used) | Body must include `"direction": "credit"` |
| 5 | **Spend Points** | **201** | `"direction": "debit"`; pre-request sets new `Idempotency-Key` |
| 6 | **My Ledger** | **200** | Check `actor_account_id` matches member for self-service rows |

### Folder: **04 Admin Transactions**

| # | Request | Expected |
|---|---------|----------|
| 1 | **Admin Adjust Account (credit)** | **201** | `actor_account_id` = admin; `direction=credit`; `account_id` = member |
| 2 | **Admin Adjust Debit** | **201** | `direction: debit`; fails **422** if balance too low |

### Folder: **05 Batch (Admin)**

| # | Request | Expected | Notes |
|---|---------|----------|-------|
| 1 | **Upload CSV** | **202** | Attach file (see below) |
| 2 | **Get Batch Job Status** | **200** | Poll until `status: completed` |
| 3 | **Get Batch Audit** | **200** | One row per CSV line; `data` is an array |

Batch audit response shape: `{ "data": [ {...}, ... ], "pagination": {...} }` (not `data.items`).

#### Sample CSV files

Use **`member-1`** (created by **Create Member** in Postman). Run **Member Login** before batch if the collection was reset.

| File | Purpose | Expected job summary (typical) |
|------|---------|--------------------------------|
| `sample-batch-success.csv` | Video/demo: **all rows accepted** | 3 accepted, 0 rejected, 0 duplicates |
| `sample-batch-rejects.csv` | Video/demo: **success + duplicate + insufficient balance** | 2 accepted, 1 rejected, 1 duplicate |
| `sample-batch.csv` | Original mixed file (earn, spend, duplicate `ref` in same file) | 2 accepted, 0 rejected, 1 duplicate |

**`sample-batch-success.csv`** — three unique refs: earn 50, earn 25, spend 10 (spend succeeds if balance ≥ 10 after earns).

**`sample-batch-rejects.csv`** — row by row:

| Row | ref | Expected audit |
|-----|-----|----------------|
| 1 | `batch-dup-001` earn 20 | `accepted` / `ok` |
| 2 | `batch-dup-001` earn 20 | `rejected` / `duplicate_ref` |
| 3 | `batch-over-001` spend 999999 | `rejected` / `insufficient_balance` |
| 4 | `batch-ok-004` earn 5 | `accepted` / `ok` |

**Video order:** upload **success** first (show clean audit), then **rejects** (show duplicate and overdraw in audit + job counts).

#### Attach CSV for Upload

1. Open **Upload CSV**
2. **Body** → **form-data**
3. Key `file` → type **File** → select one of:
   - `postman/sample-batch-success.csv` — all accepted
   - `postman/sample-batch-rejects.csv` — duplicate + insufficient balance
   - `postman/sample-batch.csv` — original mixed example
4. Send

The test script saves `batchJobId`. If status is `queued` or `processing`, send **Get Batch Job Status** again after 1–2 seconds.

### Folder: **06 Negative Cases**

Run **after** **Earn Points** (`tx-001` must exist). Run **before** **Logout**.

Automated tests assert status + `error.code`. **An error JSON here is success** — not a failed API.

| Request | Expected | Meaning |
|---------|----------|---------|
| Duplicate Ref (409) | **409** `duplicate_ref` | Replays `Idempotency-Key: tx-001` from **Earn Points** |
| Insufficient Balance (422) | **422** `insufficient_balance` | Spend more than balance |
| Member Create Account (403) | **403** `forbidden` | Member cannot create accounts |

---

## Part 4 — Run the whole collection (Collection Runner)

1. Click **Points Wallet API** collection → **Run**
2. Select folders **Health**, **01–06**
3. **Uncheck** **Logout (Admin)** and **Logout (Member)** in folder **01** (or run logout only at the end manually)
4. **Run Points Wallet API**

**Important for batch:**

- Collection Runner may **not** attach `sample-batch.csv` automatically.
- Run **Upload CSV** manually once with the file attached, or add the file path in Runner if prompted.

**Order tip:** Runner uses folder order: **Admin Login** → **Create Member** → **Member Login** → wallet → negatives. Do not run logout mid-collection.

**Restart server** after pulling code changes (`Ctrl+C`, `go run ./cmd/server`) — e.g. duplicate Create Member must return **409**, not **500**.

---

## Part 5 — Fresh database (start over)

If members already exist or you want a clean slate:

```bash
# Stop API (Ctrl+C), then:
docker compose down -v
docker compose up -d
go run ./cmd/server
```

In Postman, clear `adminToken`, `memberToken`, and `batchJobId` on the collection **Variables** tab (or re-import the collection).

---

## Troubleshooting

| Problem | Fix |
|---------|-----|
| Could not send request / connect | Server not running — `go run ./cmd/server` |
| `401` on protected routes | Run **Admin Login** or **Member Login** again |
| `401` on negative tests (`unauthorized`) | **Logout** ran first and revoked `memberToken` — login again, skip logout until end |
| `401` after second login | `SINGLE_ACTIVE_SESSION=true` revokes old tokens — use latest login |
| `409` on Create Member | Member exists — skip to **Member Login** or reset DB (Part 5) |
| `500` on Create Member duplicate | Restart server with latest code (should be **409**) |
| Negative test shows correct error but “failed” | Re-import collection — tests assert **409/422/403**, not 200 |
| Batch stays `queued` | Wait and poll **Get Batch Job Status** again |
| Upload CSV **400** | Ensure **form-data** key is `file` and a CSV is attached |
| Wrong port | Set collection variable `baseUrl` to your `HTTP_ADDR` |
| Garbled / binary ledger response | Fixed in gzip middleware — restart server; or remove `Accept-Encoding: gzip` from request headers as a workaround |

---

## Assignment coverage (via Postman)

| Task | Requests to run |
|------|-----------------|
| Wallet | Earn, Spend, My Balance, My Ledger (`actor_account_id`), Duplicate Ref, Insufficient Balance |
| Access control | Admin Login, Create Member, Member Login, Admin Adjust, Member Create Account (403) |
| Accounts | List, Update (Admin), Update My Account, soft delete (optional) |
| Batch | Upload CSV → poll Job Status → Audit; re-upload for duplicates |

See [PLAN.md](../PLAN.md) for full API design, [README.md](../README.md) for curl equivalents, and [SOLUTION.md](../SOLUTION.md) for design notes.
