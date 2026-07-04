package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ramadiaz/money-wa-bot/internal/domain/transaction"
	apperrors "github.com/ramadiaz/money-wa-bot/internal/shared/errors"
)

type InboundRepository struct {
	db *pgxpool.Pool
}

func NewInboundRepository(db *pgxpool.Pool) *InboundRepository {
	return &InboundRepository{db: db}
}

func (r *InboundRepository) Insert(ctx context.Context, deviceID, messageID, chatID, senderNumber, msgType, rawPayload string) (int64, error) {
	var id int64
	err := r.db.QueryRow(ctx, `
		INSERT INTO inbound_messages
			(gowa_device_id, gowa_message_id, chat_id, sender_number, message_type, raw_payload_json, received_at, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, 'pending')
		RETURNING id`,
		deviceID, messageID, chatID, senderNumber, msgType, rawPayload, time.Now(),
	).Scan(&id)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return 0, apperrors.ErrDuplicateMessage
		}
		return 0, fmt.Errorf("inbound insert: %w", err)
	}
	return id, nil
}

func (r *InboundRepository) MarkProcessing(ctx context.Context, id int64) error {
	_, err := r.db.Exec(ctx, `UPDATE inbound_messages SET status='processing' WHERE id=$1`, id)
	return err
}

func (r *InboundRepository) MarkDone(ctx context.Context, id int64) error {
	now := time.Now()
	_, err := r.db.Exec(ctx, `UPDATE inbound_messages SET status='done', processed_at=$1 WHERE id=$2`, now, id)
	return err
}

func (r *InboundRepository) MarkFailed(ctx context.Context, id int64, reason string) error {
	now := time.Now()
	_, err := r.db.Exec(ctx, `UPDATE inbound_messages SET status='failed', processed_at=$1 WHERE id=$2`, now, id)
	_ = reason
	return err
}

var _ transaction.InboundRepository = (*InboundRepository)(nil)

func isNoRows(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}
