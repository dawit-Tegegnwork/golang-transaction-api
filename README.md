# golang-transaction-api

Synthetic demo REST API for user accounts, deposits, withdrawals, transfers, and transaction history. Built with Go and PostgreSQL.

[![Tests](https://github.com/dawit-Tegegnwork/golang-transaction-api/actions/workflows/test.yml/badge.svg)](https://github.com/dawit-Tegegnwork/golang-transaction-api/actions/workflows/test.yml)

## Screenshot

![API overview](docs/screenshots/api-overview.png)

## Stack

- **Go 1.22** — HTTP API with `net/http` ServeMux
- **PostgreSQL 16** — runtime persistence (Docker Compose)
- **SQLite** — in-memory file DB for unit tests
- **Docker Compose** — local development

## Quick start

```bash
docker compose up --build
curl http://localhost:8080/health
```

## API overview

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Liveness + DB ping |
| POST | `/users` | Create user |
| GET | `/users/{id}` | Get user |
| POST | `/users/{id}/accounts` | Create account |
| GET | `/accounts/{id}` | Get account balance |
| POST | `/accounts/{id}/deposit` | Deposit funds |
| POST | `/accounts/{id}/withdraw` | Withdraw funds |
| POST | `/accounts/{id}/transfer` | Transfer to another account |
| GET | `/accounts/{id}/transactions` | List transaction history |

Mutating money endpoints accept an optional `Idempotency-Key` header. See [docs/idempotency.md](docs/idempotency.md).

Amounts are integer **cents** (e.g. `1500` = $15.00). All data is synthetic demo content.

## curl examples

Create a user:

```bash
curl -s -X POST http://localhost:8080/users \
  -H 'Content-Type: application/json' \
  -d '{"email":"alice@demo.test","name":"Alice Demo"}'
```

Create an account (replace `USER_ID`):

```bash
curl -s -X POST http://localhost:8080/users/USER_ID/accounts \
  -H 'Content-Type: application/json' \
  -d '{"currency":"USD"}'
```

Deposit with idempotency:

```bash
curl -s -X POST http://localhost:8080/accounts/ACCOUNT_ID/deposit \
  -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: dep-001' \
  -d '{"amount":10000,"reference":"demo deposit"}'
```

Withdraw:

```bash
curl -s -X POST http://localhost:8080/accounts/ACCOUNT_ID/withdraw \
  -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: wd-001' \
  -d '{"amount":2500,"reference":"demo withdraw"}'
```

Transfer:

```bash
curl -s -X POST http://localhost:8080/accounts/FROM_ACCOUNT_ID/transfer \
  -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: xfer-001' \
  -d '{"to_account_id":"TO_ACCOUNT_ID","amount":1500,"reference":"demo transfer"}'
```

Transaction history:

```bash
curl -s http://localhost:8080/accounts/ACCOUNT_ID/transactions
```

## Development

```bash
# Start Postgres only
docker compose up db -d

export DATABASE_URL='postgres://txn:txn@localhost:5432/txn?sslmode=disable'
go run .

# Tests (SQLite, no Docker required)
go test ./...
```

## Documentation

- [API reference](docs/api.md)
- [Database schema](docs/schema.md)
- [Idempotency](docs/idempotency.md)

## Design notes

- Money movement runs inside DB transactions with `SELECT ... FOR UPDATE` row locks.
- Transfers lock both accounts in sorted ID order to reduce deadlock risk.
- Every user/account creation and money movement writes an audit log row.
- This project uses **synthetic demo data only** — not for production financial use.
