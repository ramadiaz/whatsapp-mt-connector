package transaction

import "context"

type InboundRepository interface {
	Insert(ctx context.Context, deviceID, messageID, chatID, senderNumber, msgType, rawPayload string) (int64, error)
	MarkProcessing(ctx context.Context, id int64) error
	MarkDone(ctx context.Context, id int64) error
	MarkFailed(ctx context.Context, id int64, reason string) error
}

type PendingTransactionRepository interface {
	Insert(ctx context.Context, pt *PendingTransactionInsert) (int64, error)
	FindActiveByChat(ctx context.Context, chatID string) (*PendingTransactionRow, error)
	MarkConfirmed(ctx context.Context, id int64) error
	MarkCancelled(ctx context.Context, id int64) error
	ExpireStale(ctx context.Context) error
}

type SubmissionRepository interface {
	Insert(ctx context.Context, s *SubmissionInsert) (int64, error)
	UpdateSuccess(ctx context.Context, id int64, mtTxID, responseJSON string) error
	UpdateFailed(ctx context.Context, id int64, errMsg string) error
}

type CategoryCacheRepository interface {
	Upsert(ctx context.Context, userID int64, categories []Category) error
	List(ctx context.Context, userID int64) ([]Category, error)
}

type AccountCacheRepository interface {
	Upsert(ctx context.Context, userID int64, accounts []Account) error
	List(ctx context.Context, userID int64) ([]Account, error)
}

type PendingTransactionInsert struct {
	UserID          int64
	ChatID          string
	SourceMessageID string
	Type            string
	Amount          string
	CurrencyCode    string
	CategoryHint    string
	CategoryID      string
	AccountHint     string
	AccountID       string
	TransactionDate string
	Remark          string
	Confidence      float64
}

type PendingTransactionRow struct {
	ID              int64
	UserID          int64
	ChatID          string
	SourceMessageID string
	Type            string
	Amount          string
	CurrencyCode    string
	CategoryHint    string
	CategoryID      string
	AccountHint     string
	AccountID       string
	TransactionDate string
	Remark          string
	Confidence      float64
	Status          string
	ExpiresAt       string
}

type SubmissionInsert struct {
	PendingTransactionID int64
	RequestSnapshotJSON  string
}
