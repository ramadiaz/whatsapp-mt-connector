package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ramadiaz/money-wa-bot/internal/domain/transaction"
)

type SubmissionRepository struct {
	db *pgxpool.Pool
}

func NewSubmissionRepository(db *pgxpool.Pool) *SubmissionRepository {
	return &SubmissionRepository{db: db}
}

func (r *SubmissionRepository) Insert(ctx context.Context, s *transaction.SubmissionInsert) (int64, error) {
	var id int64
	now := time.Now()
	err := r.db.QueryRow(ctx, `
		INSERT INTO transaction_submissions
			(pending_transaction_id, request_snapshot_json, status, attempt_count, created_at, updated_at)
		VALUES ($1,$2,'pending',1,$3,$4)
		RETURNING id`,
		s.PendingTransactionID, s.RequestSnapshotJSON, now, now,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("submission insert: %w", err)
	}
	return id, nil
}

func (r *SubmissionRepository) UpdateSuccess(ctx context.Context, id int64, mtTxID, responseJSON string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE transaction_submissions
		SET status='succeeded', money_tracker_transaction_id=$1, response_snapshot_json=$2, updated_at=$3
		WHERE id=$4`,
		mtTxID, responseJSON, time.Now(), id,
	)
	return err
}

func (r *SubmissionRepository) UpdateFailed(ctx context.Context, id int64, errMsg string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE transaction_submissions
		SET status='failed', last_error=$1, attempt_count=attempt_count+1, updated_at=$2
		WHERE id=$3`,
		errMsg, time.Now(), id,
	)
	return err
}

var _ transaction.SubmissionRepository = (*SubmissionRepository)(nil)
