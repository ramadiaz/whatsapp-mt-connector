package confirmation

import (
	"time"

	"github.com/shopspring/decimal"
)

type Status string

const (
	StatusPending   Status = "pending"
	StatusConfirmed Status = "confirmed"
	StatusCancelled Status = "cancelled"
	StatusExpired   Status = "expired"
)

const PendingTTLMinutes = 15

type PendingTransaction struct {
	ID              int64
	ChatID          string
	SourceMessageID string
	Type            string
	Amount          decimal.Decimal
	CurrencyCode    string
	CategoryHint    string
	CategoryID      string
	AccountHint     string
	AccountID       string
	TransactionDate string
	Remark          string
	Confidence      float64
	Status          Status
	ExpiresAt       time.Time
	CreatedAt       time.Time
	ConfirmedAt     *time.Time
	CancelledAt     *time.Time
}

func (p *PendingTransaction) IsExpired() bool {
	return time.Now().After(p.ExpiresAt)
}

func (p *PendingTransaction) IsActive() bool {
	return p.Status == StatusPending && !p.IsExpired()
}
