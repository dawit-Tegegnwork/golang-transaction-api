# Idempotency

Money endpoints (`deposit`, `withdraw`, `transfer`) support the `Idempotency-Key` request header so clients can safely retry on network failures.

## Header

```
Idempotency-Key: <unique-client-token>
```

- Optional but recommended for mutating calls
- Should be unique per logical operation (UUIDs work well)
- Scoped to the **request path** — the same key on different paths is treated independently

## Behavior

1. If no header is sent, the request is processed normally (no replay cache).
2. If a matching `(key, path)` exists in `idempotency_keys`, the stored HTTP status and JSON body are returned unchanged.
3. Replayed responses include `X-Idempotent-Replay: true`.
4. On first success (or non-400 error), the response is persisted for future replays.

## Example

First request:

```bash
curl -i -X POST http://localhost:8080/accounts/ACC_ID/deposit \
  -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: dep-demo-42' \
  -d '{"amount":5000}'
```

Retry (same key, same path):

```bash
curl -i -X POST http://localhost:8080/accounts/ACC_ID/deposit \
  -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: dep-demo-42' \
  -d '{"amount":5000}'
```

The retry returns the original transaction JSON and does **not** double-credit the account.

## Limitations (demo)

- Idempotency records are not expired automatically.
- Request body is not hashed; clients should send the same payload on retry.
- Only deposit, withdraw, and transfer are idempotent-aware.
