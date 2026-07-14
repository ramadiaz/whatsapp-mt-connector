package transaction

import "context"

type InboundRepository interface {
	Insert(ctx context.Context, deviceID, messageID, chatID, senderNumber, msgType, rawPayload string) (string, error)
	MarkProcessing(ctx context.Context, id string) error
	MarkDone(ctx context.Context, id string) error
	MarkFailed(ctx context.Context, id string, reason string) error
}

type PendingTransactionRepository interface {
	Insert(ctx context.Context, pt *PendingTransactionInsert) (string, error)
	FindActiveByChat(ctx context.Context, chatID string) (*PendingTransactionRow, error)
	MarkConfirmed(ctx context.Context, id string) error
	MarkCancelled(ctx context.Context, id string) error
	ExpireStale(ctx context.Context) error
}

type SubmissionRepository interface {
	Insert(ctx context.Context, s *SubmissionInsert) (string, error)
	UpdateSuccess(ctx context.Context, id string, mtTxID, responseJSON string) error
	UpdateFailed(ctx context.Context, id string, errMsg string) error
}

type CategoryCacheRepository interface {
	Upsert(ctx context.Context, userUUID string, categories []Category) error
	List(ctx context.Context, userUUID string) ([]Category, error)
}

type AccountCacheRepository interface {
	Upsert(ctx context.Context, userUUID string, accounts []Account) error
	List(ctx context.Context, userUUID string) ([]Account, error)
}

type PendingTransactionInsert struct {
	UserUUID        string
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
	UUID            string
	UserUUID        string
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
	PendingTransactionUUID string
	RequestSnapshotJSON    string
}

