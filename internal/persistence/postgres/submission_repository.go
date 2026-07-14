package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/ramadiaz/whatsapp-mt-connector/internal/domain/transaction"
	"gorm.io/gorm"
)

type SubmissionRepository struct {
	db *gorm.DB
}

func NewSubmissionRepository(db *gorm.DB) *SubmissionRepository {
	return &SubmissionRepository{db: db}
}

func (r *SubmissionRepository) Insert(ctx context.Context, s *transaction.SubmissionInsert) (string, error) {
	now := time.Now()
	sub := TransactionSubmission{
		PendingTransactionUUID: s.PendingTransactionUUID,
		RequestSnapshotJSON:    &s.RequestSnapshotJSON,
		Status:                 "pending",
		AttemptCount:           1,
		CreatedAt:              now,
		UpdatedAt:              now,
	}

	err := r.db.WithContext(ctx).Create(&sub).Error
	if err != nil {
		return "", fmt.Errorf("submission insert: %w", err)
	}
	return sub.UUID, nil
}

func (r *SubmissionRepository) UpdateSuccess(ctx context.Context, id string, mtTxID, responseJSON string) error {
	return r.db.WithContext(ctx).Model(&TransactionSubmission{}).Where("uuid = ?", id).Updates(map[string]interface{}{
		"status":                       "succeeded",
		"money_tracker_transaction_id": mtTxID,
		"response_snapshot_json":       responseJSON,
		"updated_at":                   time.Now(),
	}).Error
}

func (r *SubmissionRepository) UpdateFailed(ctx context.Context, id string, errMsg string) error {
	return r.db.WithContext(ctx).Model(&TransactionSubmission{}).Where("uuid = ?", id).Updates(map[string]interface{}{
		"status":        "failed",
		"last_error":    errMsg,
		"attempt_count": gorm.Expr("attempt_count + 1"),
		"updated_at":    time.Now(),
	}).Error
}

var _ transaction.SubmissionRepository = (*SubmissionRepository)(nil)

