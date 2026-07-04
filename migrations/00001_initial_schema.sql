-- +goose Up

CREATE TABLE IF NOT EXISTS inbound_messages (
    id               BIGSERIAL PRIMARY KEY,
    gowa_device_id   TEXT NOT NULL,
    gowa_message_id  TEXT NOT NULL,
    chat_id          TEXT NOT NULL,
    sender_number    TEXT NOT NULL,
    message_type     TEXT NOT NULL,
    raw_payload_json JSONB NOT NULL DEFAULT '{}',
    received_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    processed_at     TIMESTAMPTZ,
    status           TEXT NOT NULL DEFAULT 'pending',
    CONSTRAINT uq_inbound_device_message UNIQUE (gowa_device_id, gowa_message_id)
);

CREATE INDEX IF NOT EXISTS idx_inbound_status ON inbound_messages(status);
CREATE INDEX IF NOT EXISTS idx_inbound_received_at ON inbound_messages(received_at);

CREATE TABLE IF NOT EXISTS pending_transactions (
    id               BIGSERIAL PRIMARY KEY,
    chat_id          TEXT NOT NULL,
    source_message_id TEXT NOT NULL,
    type             TEXT NOT NULL,
    amount           NUMERIC(20,4) NOT NULL,
    currency_code    TEXT NOT NULL DEFAULT 'IDR',
    category_hint    TEXT,
    category_id      TEXT,
    account_hint     TEXT,
    account_id       TEXT,
    transaction_date TEXT NOT NULL,
    remark           TEXT,
    confidence       DOUBLE PRECISION NOT NULL DEFAULT 0,
    status           TEXT NOT NULL DEFAULT 'pending',
    expires_at       TIMESTAMPTZ NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    confirmed_at     TIMESTAMPTZ,
    cancelled_at     TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_pending_chat_status ON pending_transactions(chat_id, status);
CREATE INDEX IF NOT EXISTS idx_pending_expires_at ON pending_transactions(expires_at);

CREATE TABLE IF NOT EXISTS transaction_submissions (
    id                            BIGSERIAL PRIMARY KEY,
    pending_transaction_id        BIGINT NOT NULL REFERENCES pending_transactions(id),
    money_tracker_transaction_id  TEXT,
    request_snapshot_json         JSONB,
    response_snapshot_json        JSONB,
    status                        TEXT NOT NULL DEFAULT 'pending',
    attempt_count                 INT NOT NULL DEFAULT 1,
    last_error                    TEXT,
    created_at                    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_submission_pending_id ON transaction_submissions(pending_transaction_id);
CREATE INDEX IF NOT EXISTS idx_submission_status ON transaction_submissions(status);

CREATE TABLE IF NOT EXISTS money_tracker_categories_cache (
    category_id  TEXT PRIMARY KEY,
    title        TEXT NOT NULL,
    type         INT NOT NULL,
    refreshed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS money_tracker_accounts_cache (
    account_id    TEXT PRIMARY KEY,
    name          TEXT NOT NULL,
    currency_code TEXT NOT NULL DEFAULT 'IDR',
    refreshed_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- +goose Down

DROP TABLE IF EXISTS money_tracker_accounts_cache;
DROP TABLE IF EXISTS money_tracker_categories_cache;
DROP TABLE IF EXISTS transaction_submissions;
DROP TABLE IF EXISTS pending_transactions;
DROP TABLE IF EXISTS inbound_messages;
