# API reference

Base URL: `http://localhost:8080`

All request/response bodies are JSON. Amounts are integer **cents**.

## Health

### GET /health

**200** — database reachable

```json
{"status":"ok"}
```

**503** — database unavailable

---

## Users

### POST /users

Create a demo user.

**Body**

```json
{"email":"bob@demo.test","name":"Bob Demo"}
```

**201** — user object

### GET /users/{id}

**200** — user object  
**404** — not found

---

## Accounts

### POST /users/{id}/accounts

**Body**

```json
{"currency":"USD"}
```

**201** — account object (`balance` starts at 0)

### GET /accounts/{id}

**200** — account with current balance  
**404** — not found

---

## Money operations

Support optional header: `Idempotency-Key: <token>`

### POST /accounts/{id}/deposit

**Body**

```json
{"amount":10000,"reference":"optional note"}
```

**200** — transaction record  
**400** — invalid amount  
**404** — account not found

### POST /accounts/{id}/withdraw

**Body**

```json
{"amount":2500,"reference":"optional note"}
```

**200** — transaction record  
**400** — invalid amount  
**404** — account not found  
**422** — insufficient funds

### POST /accounts/{id}/transfer

**Body**

```json
{"to_account_id":"DEST_UUID","amount":1500,"reference":"optional note"}
```

**200** — array of two transactions (`transfer_out`, `transfer_in`)  
**400** — invalid amount or same account  
**404** — account not found  
**422** — insufficient funds

### GET /accounts/{id}/transactions

**200** — array of transactions (newest first)

---

## Error shape

```json
{"error":"human-readable message"}
```

## Transaction object

```json
{
  "id": "uuid",
  "account_id": "uuid",
  "counterparty_id": "uuid-or-null",
  "type": "deposit|withdraw|transfer_in|transfer_out",
  "amount": 1500,
  "balance_after": 8500,
  "reference": "optional",
  "created_at": "2026-06-29T12:00:00Z"
}
```
