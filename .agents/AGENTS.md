# WhatsApp-MT-Connector Agent Guidelines & Rules

## Project Overview
This service connects GOWA (WhatsApp Gateway) with Money Tracker. It receives signed webhooks, parses transaction descriptions or receipt images using 9Router (AI API), requests user confirmation, and uploads transactions to Money Tracker.

## Technology Stack
- **Language**: Go 1.25.7
- **Router**: Go-Chi/v5
- **ORM**: GORM + pgx/v5 (Postgres)
- **Queue**: Redis + Asynq
- **Logger**: Zerolog

## Coding Standards (Strict Rules)
- **No Comments**: NEVER write comments on code.
- **Naming**: English variables and functions only.
- **Modification Rule**: Check project-wide usage before modifying functions. If reused, write a new one.
- **No Nesting**: Avoid layered nested functions unless reusable.
- **Config**: Always check existing configurations before changing defaults.

## Architectural Structure
- `cmd/whatsapp-mt-connector/main.go`: App entry.
- `internal/app/application.go`: Dependency injection and server boots.
- `internal/config/config.go`: Configuration environment variables.
- `internal/domain/`: Domain entities and repository interfaces.
- `internal/persistence/postgres/`: GORM database repositories.
- `internal/persistence/redis/`: Asynq redis client/scheduler setup.
- `internal/service/`: Business services (webhook, parser, transactions).
- `internal/delivery/http/`: Webhook and health endpoints.
- `internal/jobs/`: Async queue worker tasks.

## Key Logic Requirements
1. **Webhook Security**: Validate `X-Hub-Signature-256` signature using HMAC SHA-256 raw request body. Reject unauthorized senders.
2. **Data Submission**: Send URL form-encoded data to Money Tracker API. JSON requests are not allowed.
3. **No Key Leaks**: Never log API keys, basic auth passwords, tokens, or raw receipt images.
4. **Transaction Flows**: Transactions expire after 15 minutes. Must require user confirmation unless confidence is high and AUTO_COMMIT is enabled.
