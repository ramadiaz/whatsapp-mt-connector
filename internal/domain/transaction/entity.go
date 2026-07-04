package transaction

import (
	"time"

	"github.com/shopspring/decimal"
)

type TransactionType int

const (
	TypeIncome  TransactionType = 1
	TypeExpense TransactionType = 2
)

type SubmissionStatus string

const (
	SubmissionPending   SubmissionStatus = "pending"
	SubmissionSucceeded SubmissionStatus = "succeeded"
	SubmissionFailed    SubmissionStatus = "failed"
)

type CreateTransactionRequest struct {
	Type         TransactionType
	Amount       decimal.Decimal
	CategoryID   string
	AccountID    string
	Date         string
	Remark       string
	CurrencyCode string
}

type CreatedTransaction struct {
	ID string
}

type Submission struct {
	ID                        int64
	PendingTransactionID      int64
	MoneyTrackerTransactionID string
	RequestSnapshotJSON       string
	ResponseSnapshotJSON      string
	Status                    SubmissionStatus
	AttemptCount              int
	LastError                 string
	CreatedAt                 time.Time
	UpdatedAt                 time.Time
}

type Category struct {
	CategoryID  string
	Title       string
	Type        int
	RefreshedAt time.Time
}

type Account struct {
	AccountID    string
	Name         string
	CurrencyCode string
	RefreshedAt  time.Time
}
