# money-wa-bot

Production-ready Go service that bridges WhatsApp (via GOWA) with Money Tracker. Receives GOWA webhooks, extracts transactions via 9Router AI, then writes to Money Tracker with user confirmation.

## Flow

```
WhatsApp → GOWA → signed webhook → money-wa-bot
  → validate HMAC + sender + idempotency
  → enqueue background job (Asynq)
  → parse text / download receipt image
  → AI extraction via 9Router
  → validate output
  → match category/account from MT cache
  → send confirmation to user
  → user replies "ya"
  → POST addTransaction to Money Tracker (form-urlencoded)
  → send success reply
```

## Prerequisites

- Go 1.23+
- PostgreSQL 14+
- Redis 7+
- GOWA instance running and configured
- 9Router instance with OpenAI-compatible API
- Money Tracker account with API key

## Setup

```bash
cd money-wa-bot
cp .env.example .env
# Edit .env with your values
```

## Run locally

```bash
make tidy
make migrate
make run
```

## Run with Docker (dev)

```bash
docker compose -f deploy/docker-compose.yml up -d
```

## Run in production

```bash
# Build image
make docker-build

# Deploy (with external DB/Redis)
docker compose -f deploy/docker-compose.prod.yml up -d
```

## Endpoints

| Method | Path | Description |
|--------|------|-------------|
| POST | `/webhooks/gowa` | GOWA webhook receiver |
| GET | `/healthz` | Liveness probe |
| GET | `/readyz` | Readiness probe (checks DB + GOWA) |

## GOWA webhook setup

Configure GOWA to send webhooks to:
```
https://your-domain.com/webhooks/gowa
```

Set `GOWA_WEBHOOK_SECRET` to match `WHATSAPP_WEBHOOK_SECRET` in GOWA.

## Message formats

```
catat kopi susu 25k
makan siang 45 ribu tadi
transport 80k tanggal 2 juli
income freelance 1.500.000
belanja shopee 230k pakai BCA
```

Manual fallback format:
```
expense | 25000 | food | kopi susu | 2026-07-03
income | 1500000 | salary | freelance | 2026-07-03
```

## Confirmation flow

1. Send transaction → bot replies with summary
2. Reply `ya` → transaction saved to Money Tracker
3. Reply `batal` → transaction cancelled
4. Pending transactions expire after 15 minutes

## Environment variables

See [.env.example](.env.example) for all required variables.

## Database migrations

```bash
make migrate
```

Uses [goose](https://github.com/pressly/goose) format. Migrations in `migrations/`.

## Tests

```bash
make test
```

## Security

- GOWA webhooks validated via HMAC-SHA256
- Only messages from `WHATSAPP_ALLOWED_NUMBER` processed
- API keys never appear in logs
- Receipt images kept in memory only, never persisted
- Non-root Docker user
