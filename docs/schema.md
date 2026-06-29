# Database schema

Synthetic demo schema for the transaction API. Amounts are stored as integer cents.

## users

| Column | Type | Notes |
|--------|------|-------|
| id | TEXT PK | UUID |
| email | TEXT UNIQUE | Demo email |
| name | TEXT | Display name |
| created_at | TIMESTAMPTZ | UTC |

## accounts

| Column | Type | Notes |
|--------|------|-------|
| id | TEXT PK | UUID |
| user_id | TEXT FK → users.id | Owner |
| currency | TEXT | Default `USD` |
| balance | BIGINT | Non-negative cents |
| created_at | TIMESTAMPTZ | UTC |

## transactions

Ledger entries for each account movement.

| Column | Type | Notes |
|--------|------|-------|
| id | TEXT PK | UUID |
| account_id | TEXT FK → accounts.id | Affected account |
| counterparty_id | TEXT FK → accounts.id | Optional other account |
| type | TEXT | `deposit`, `withdraw`, `transfer_in`, `transfer_out` |
| amount | BIGINT | Positive cents moved |
| balance_after | BIGINT | Balance after this entry |
| reference | TEXT | Optional client reference |
| created_at | TIMESTAMPTZ | UTC |

## idempotency_keys

Caches HTTP responses for safe retries.

| Column | Type | Notes |
|--------|------|-------|
| key | TEXT | Client `Idempotency-Key` header |
| path | TEXT | Request path (composite PK with key) |
| status_code | INTEGER | HTTP status to replay |
| response_body | BLOB | JSON body to replay |
| created_at | TIMESTAMPTZ | UTC |

## audit_log

Append-only audit trail.

| Column | Type | Notes |
|--------|------|-------|
| id | TEXT PK | UUID |
| actor | TEXT | e.g. `system` or user id |
| action | TEXT | e.g. `account.deposit` |
| resource | TEXT | Target resource id |
| details | TEXT | Free-form context |
| created_at | TIMESTAMPTZ | UTC |

## Indexes

- `transactions(account_id)`
- `accounts(user_id)`
- `audit_log(created_at)`

## Concurrency

Transfers and withdrawals use:

```sql
SELECT balance FROM accounts WHERE id = $1 FOR UPDATE;
```

inside a single database transaction. Transfer operations lock both source and destination accounts (ordered by id) before updating balances and inserting ledger rows.
