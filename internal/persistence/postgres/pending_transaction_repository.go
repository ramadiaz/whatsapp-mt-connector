package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ramadiaz/money-wa-bot/internal/domain/transaction"
	apperrors "github.com/ramadiaz/money-wa-bot/internal/shared/errors"
)

type PendingTransactionRepository struct {
	db *pgxpool.Pool
}

func NewPendingTransactionRepository(db *pgxpool.Pool) *PendingTransactionRepository {
	return &PendingTransactionRepository{db: db}
}

func (r *PendingTransactionRepository) Insert(ctx context.Context, pt *transaction.PendingTransactionInsert) (int64, error) {
	expiresAt := time.Now().Add(15 * time.Minute)
	var id int64
	err := r.db.QueryRow(ctx, `
		INSERT INTO pending_transactions
			(chat_id, source_message_id, type, amount, currency_code, category_hint, category_id,
			 account_hint, account_id, transaction_date, remark, confidence, status, expires_at, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,'pending',$13,$14)
		RETURNING id`,
		pt.ChatID, pt.SourceMessageID, pt.Type, pt.Amount, pt.CurrencyCode,
		pt.CategoryHint, pt.CategoryID, pt.AccountHint, pt.AccountID,
		pt.TransactionDate, pt.Remark, pt.Confidence, expiresAt, time.Now(),
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("pending tx insert: %w", err)
	}
	return id, nil
}

func (r *PendingTransactionRepository) FindActiveByChat(ctx context.Context, chatID string) (*transaction.PendingTransactionRow, error) {
	row := &transaction.PendingTransactionRow{}
	err := r.db.QueryRow(ctx, `
		SELECT id, chat_id, source_message_id, type, amount, currency_code,
		       category_hint, category_id, account_hint, account_id,
		       transaction_date, remark, confidence, status, expires_at
		FROM pending_transactions
		WHERE chat_id=$1 AND status='pending' AND expires_at > NOW()
		ORDER BY created_at DESC
		LIMIT 1`, chatID,
	).Scan(
		&row.ID, &row.ChatID, &row.SourceMessageID, &row.Type, &row.Amount, &row.CurrencyCode,
		&row.CategoryHint, &row.CategoryID, &row.AccountHint, &row.AccountID,
		&row.TransactionDate, &row.Remark, &row.Confidence, &row.Status, &row.ExpiresAt,
	)
	if err != nil {
		if isNoRows(err) {
			return nil, apperrors.ErrNoPendingTransaction
		}
		return nil, fmt.Errorf("pending tx find: %w", err)
	}
	return row, nil
}

func (r *PendingTransactionRepository) MarkConfirmed(ctx context.Context, id int64) error {
	_, err := r.db.Exec(ctx,
		`UPDATE pending_transactions SET status='confirmed', confirmed_at=$1 WHERE id=$2`,
		time.Now(), id,
	)
	return err
}

func (r *PendingTransactionRepository) MarkCancelled(ctx context.Context, id int64) error {
	_, err := r.db.Exec(ctx,
		`UPDATE pending_transactions SET status='cancelled', cancelled_at=$1 WHERE id=$2`,
		time.Now(), id,
	)
	return err
}

func (r *PendingTransactionRepository) ExpireStale(ctx context.Context) error {
	_, err := r.db.Exec(ctx,
		`UPDATE pending_transactions SET status='expired' WHERE status='pending' AND expires_at <= NOW()`,
	)
	return err
}

var _ transaction.PendingTransactionRepository = (*PendingTransactionRepository)(nil)
