# Deploy to Render (free tier)

[![Deploy to Render](https://render.com/images/deploy-to-render-button.svg)](https://render.com/deploy?repo=https://github.com/dawit-Tegegnwork/golang-transaction-api)

## One-click deploy

1. Click **Deploy to Render** above.
2. Render provisions a **free PostgreSQL** database and web service from [`render.yaml`](../render.yaml).
3. After deploy, open `https://<your-service>.onrender.com/` for the demo landing page.
4. Demo wallets and a sample deposit are seeded on startup.

## Quick test

```bash
curl https://<your-service>.onrender.com/health
curl https://<your-service>.onrender.com/audit?limit=5
```

## Environment variables

| Variable | Source | Notes |
|----------|--------|-------|
| `DATABASE_URL` | Linked Postgres | Set automatically by blueprint |
| `HTTP_ADDR` | default `:8080` | Render sets `PORT`; map in Dockerfile if needed |

## Production notes

- Free tier cold start ~30s after idle.
- Synthetic ledger data only — not a financial product.
- Idempotency keys required on transfers — see [docs/idempotency.md](idempotency.md).

## Health check

Expected: `{"status":"ok"}`
