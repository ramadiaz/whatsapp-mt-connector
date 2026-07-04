package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/ramadiaz/money-wa-bot/internal/domain/transaction"
	apperrors "github.com/ramadiaz/money-wa-bot/internal/shared/errors"
	"gorm.io/gorm"
)

type InboundRepository struct {
	db *gorm.DB
}

func NewInboundRepository(db *gorm.DB) *InboundRepository {
	return &InboundRepository{db: db}
}

func (r *InboundRepository) Insert(ctx context.Context, deviceID, messageID, chatID, senderNumber, msgType, rawPayload string) (int64, error) {
	msg := InboundMessage{
		GowaDeviceID:   deviceID,
		GowaMessageID:  messageID,
		ChatID:         chatID,
		SenderNumber:   senderNumber,
		MessageType:    msgType,
		RawPayloadJSON: rawPayload,
		ReceivedAt:     time.Now(),
		Status:         "pending",
	}
	err := r.db.WithContext(ctx).Create(&msg).Error
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return 0, apperrors.ErrDuplicateMessage
		}
		return 0, fmt.Errorf("inbound insert: %w", err)
	}
	return msg.ID, nil
}

func (r *InboundRepository) MarkProcessing(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Model(&InboundMessage{}).Where("id = ?", id).Update("status", "processing").Error
}

func (r *InboundRepository) MarkDone(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Model(&InboundMessage{}).Where("id = ?", id).Updates(map[string]interface{}{
		"status":      "done",
		"processed_at": time.Now(),
	}).Error
}

func (r *InboundRepository) MarkFailed(ctx context.Context, id int64, reason string) error {
	return r.db.WithContext(ctx).Model(&InboundMessage{}).Where("id = ?", id).Updates(map[string]interface{}{
		"status":      "failed",
		"processed_at": time.Now(),
	}).Error
}

var _ transaction.InboundRepository = (*InboundRepository)(nil)
