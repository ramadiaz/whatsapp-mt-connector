package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/ramadiaz/whatsapp-mt-connector/internal/domain/transaction"
	apperrors "github.com/ramadiaz/whatsapp-mt-connector/internal/shared/errors"
	"gorm.io/gorm"
)

type InboundRepository struct {
	db *gorm.DB
}

func NewInboundRepository(db *gorm.DB) *InboundRepository {
	return &InboundRepository{db: db}
}

func (r *InboundRepository) Insert(ctx context.Context, deviceID, messageID, chatID, senderNumber, msgType, rawPayload string) (string, error) {
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
			return "", apperrors.ErrDuplicateMessage
		}
		return "", fmt.Errorf("inbound insert: %w", err)
	}
	return msg.UUID, nil
}

func (r *InboundRepository) MarkProcessing(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Model(&InboundMessage{}).Where("uuid = ?", id).Update("status", "processing").Error
}

func (r *InboundRepository) MarkDone(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Model(&InboundMessage{}).Where("uuid = ?", id).Updates(map[string]interface{}{
		"status":       "done",
		"processed_at": time.Now(),
	}).Error
}

func (r *InboundRepository) MarkFailed(ctx context.Context, id string, reason string) error {
	return r.db.WithContext(ctx).Model(&InboundMessage{}).Where("uuid = ?", id).Updates(map[string]interface{}{
		"status":       "failed",
		"processed_at": time.Now(),
	}).Error
}

func (r *InboundRepository) GetRawPayloadByMessageID(ctx context.Context, messageID string) (string, error) {
	var msg InboundMessage
	err := r.db.WithContext(ctx).Where("gowa_message_id = ?", messageID).First(&msg).Error
	if err != nil {
		return "", err
	}
	return msg.RawPayloadJSON, nil
}

var _ transaction.InboundRepository = (*InboundRepository)(nil)

