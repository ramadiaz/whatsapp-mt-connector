---
name: whatsapp_mt_connector_development
description: Guidelines and reference knowledge for developing or debugging the whatsapp-mt-connector repository
---

# Repository Architecture & Domain Reference

## Database Models & Table Schema
Entities are defined in `internal/persistence/postgres/models.go`:
- `User`: Handles UUID, phone numbers, roles, and user-specific Money Tracker API Keys.
- `InboundMessage`: Captures GOWA device and message IDs with unique constraints.
- `PendingTransaction`: Temporarily stores parsed transaction data awaiting user confirmation.
- `TransactionSubmission`: Tracks actual submissions to Money Tracker and errors.
- `CategoryCache` / `AccountCache`: Local caches of Money Tracker categories and accounts.

## Integration Protocols
- **GOWA (WhatsApp)**: Interacts via authenticated HTTP. Webhooks are sent to `/webhooks/gowa` containing message objects. Media download requires fetching binary data using GOWA message IDs.
- **Money Tracker**: Submissions must be URL form-encoded (`application/x-www-form-urlencoded`). Never send JSON bodies.
- **9Router**: Formulates OpenAI-compatible chat completions using structured system prompts for text and receipt image extraction.

## Common Operations
- **Category Synonym Matching**: Matches text hints against cached titles, substring containment, and pre-defined synonym mappings (`internal/service/category_matcher.go`).
- **Asynq Jobs**: Tasks are processed asynchronously:
  - `jobs.TypeProcessMessage`: Processes incoming messages.
  - `jobs.TypeRefreshMTCache`: Syncs category and account caches from Money Tracker.
  - `jobs.TypeDailyReminder`: Notifies users daily.
