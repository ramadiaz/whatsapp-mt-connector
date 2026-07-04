package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
	"github.com/ramadiaz/whatsapp-mt-connector/internal/domain/transaction"
	apperrors "github.com/ramadiaz/whatsapp-mt-connector/internal/shared/errors"
	"gorm.io/gorm"
)

type PendingTransactionRepository struct {
	db *gorm.DB
}

func NewPendingTransactionRepository(db *gorm.DB) *PendingTransactionRepository {
	return &PendingTransactionRepository{db: db}
}

func (r *PendingTransactionRepository) Insert(ctx context.Context, pt *transaction.PendingTransactionInsert) (int64, error) {
	amount, err := decimal.NewFromString(pt.Amount)
	if err != nil {
		return 0, fmt.Errorf("invalid amount decimal: %w", err)
	}

	expiresAt := time.Now().Add(15 * time.Minute)
	pending := PendingTransaction{
		ChatID:          pt.ChatID,
		SourceMessageID: pt.SourceMessageID,
		Type:            pt.Type,
		Amount:          amount,
		CurrencyCode:    pt.CurrencyCode,
		CategoryHint:    pt.CategoryHint,
		CategoryID:      pt.CategoryID,
		AccountHint:     pt.AccountHint,
		AccountID:       pt.AccountID,
		TransactionDate: pt.TransactionDate,
		Remark:          pt.Remark,
		Confidence:      pt.Confidence,
		Status:          "pending",
		ExpiresAt:       expiresAt,
		CreatedAt:       time.Now(),
	}

	err = r.db.WithContext(ctx).Create(&pending).Error
	if err != nil {
		return 0, fmt.Errorf("pending tx insert: %w", err)
	}
	return pending.ID, nil
}

func (r *PendingTransactionRepository) FindActiveByChat(ctx context.Context, chatID string) (*transaction.PendingTransactionRow, error) {
	var pt PendingTransaction
	err := r.db.WithContext(ctx).
		Where("chat_id = ? AND status = ? AND expires_at > ?", chatID, "pending", time.Now()).
		Order("created_at DESC").
		First(&pt).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrNoPendingTransaction
		}
		return nil, fmt.Errorf("pending tx find: %w", err)
	}

	return &transaction.PendingTransactionRow{
		ID:              pt.ID,
		ChatID:          pt.ChatID,
		SourceMessageID: pt.SourceMessageID,
		Type:            pt.Type,
		Amount:          pt.Amount.String(),
		CurrencyCode:    pt.CurrencyCode,
		CategoryHint:    pt.CategoryHint,
		CategoryID:      pt.CategoryID,
		AccountHint:     pt.AccountHint,
		AccountID:       pt.AccountID,
		TransactionDate: pt.TransactionDate,
		Remark:          pt.Remark,
		Confidence:      pt.Confidence,
		Status:          pt.Status,
		ExpiresAt:       pt.ExpiresAt.Format(time.RFC3339),
	}, nil
}

func (r *PendingTransactionRepository) MarkConfirmed(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Model(&PendingTransaction{}).Where("id = ?", id).Updates(map[string]interface{}{
		"status":       "confirmed",
		"confirmed_at": time.Now(),
	}).Error
}

func (r *PendingTransactionRepository) MarkCancelled(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Model(&PendingTransaction{}).Where("id = ?", id).Updates(map[string]interface{}{
		"status":       "cancelled",
		"cancelled_at": time.Now(),
	}).Error
}

func (r *PendingTransactionRepository) ExpireStale(ctx context.Context) error {
	return r.db.WithContext(ctx).Model(&PendingTransaction{}).
		Where("status = ? AND expires_at <= ?", "pending", time.Now()).
		Update("status", "expired").Error
}

var _ transaction.PendingTransactionRepository = (*PendingTransactionRepository)(nil)
