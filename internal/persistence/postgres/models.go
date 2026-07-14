package postgres

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type User struct {
	ID          int64          `gorm:"primaryKey;autoIncrement;index;column:id"`
	UUID        string         `gorm:"uniqueIndex;index;column:uuid;not null"`
	PhoneNumber string         `gorm:"column:phone_number;uniqueIndex;not null"`
	Role        string         `gorm:"column:role;default:customer;not null"`
	MTAPIKey    string         `gorm:"column:mt_api_key"`
	CreatedAt   time.Time      `gorm:"column:created_at;default:now()"`
	UpdatedAt   time.Time      `gorm:"column:updated_at;default:now()"`
	DeletedAt   gorm.DeletedAt `gorm:"column:deleted_at;index"`
}

func (u *User) BeforeCreate(tx *gorm.DB) error {
	if u.UUID == "" {
		u.UUID = uuid.NewString()
	}
	return nil
}

func (User) TableName() string {
	return "users"
}

type InboundMessage struct {
	ID             int64          `gorm:"primaryKey;autoIncrement;index;column:id"`
	UUID           string         `gorm:"uniqueIndex;index;column:uuid;not null"`
	GowaDeviceID   string         `gorm:"column:gowa_device_id;uniqueIndex:uq_inbound_device_message"`
	GowaMessageID  string         `gorm:"column:gowa_message_id;uniqueIndex:uq_inbound_device_message"`
	ChatID         string         `gorm:"column:chat_id"`
	SenderNumber   string         `gorm:"column:sender_number"`
	MessageType    string         `gorm:"column:message_type"`
	RawPayloadJSON string         `gorm:"column:raw_payload_json;type:jsonb"`
	ReceivedAt     time.Time      `gorm:"column:received_at;default:now();index:idx_inbound_received_at"`
	ProcessedAt    *time.Time     `gorm:"column:processed_at"`
	Status         string         `gorm:"column:status;default:pending;index:idx_inbound_status"`
	CreatedAt      time.Time      `gorm:"column:created_at;default:now()"`
	UpdatedAt      time.Time      `gorm:"column:updated_at;default:now()"`
	DeletedAt      gorm.DeletedAt `gorm:"column:deleted_at;index"`
}

func (m *InboundMessage) BeforeCreate(tx *gorm.DB) error {
	if m.UUID == "" {
		m.UUID = uuid.NewString()
	}
	return nil
}

func (InboundMessage) TableName() string {
	return "inbound_messages"
}

type PendingTransaction struct {
	ID              int64           `gorm:"primaryKey;autoIncrement;index;column:id"`
	UUID            string          `gorm:"uniqueIndex;index;column:uuid;not null"`
	UserUUID        string          `gorm:"column:user_uuid;index:idx_pending_user"`
	ChatID          string          `gorm:"column:chat_id;index:idx_pending_chat_status"`
	SourceMessageID string          `gorm:"column:source_message_id"`
	Type            string          `gorm:"column:type"`
	Amount          decimal.Decimal `gorm:"column:amount;type:numeric(20,4)"`
	CurrencyCode    string          `gorm:"column:currency_code;default:IDR"`
	CategoryHint    string          `gorm:"column:category_hint"`
	CategoryID      string          `gorm:"column:category_id"`
	AccountHint     string          `gorm:"column:account_hint"`
	AccountID       string          `gorm:"column:account_id"`
	TransactionDate string          `gorm:"column:transaction_date"`
	Remark          string          `gorm:"column:remark"`
	Confidence      float64         `gorm:"column:confidence;default:0"`
	Status          string          `gorm:"column:status;default:pending;index:idx_pending_chat_status"`
	ExpiresAt       time.Time       `gorm:"column:expires_at;index:idx_pending_expires_at"`
	CreatedAt       time.Time       `gorm:"column:created_at;default:now()"`
	UpdatedAt       time.Time       `gorm:"column:updated_at;default:now()"`
	ConfirmedAt     *time.Time      `gorm:"column:confirmed_at"`
	CancelledAt     *time.Time      `gorm:"column:cancelled_at"`
	DeletedAt       gorm.DeletedAt  `gorm:"column:deleted_at;index"`
}

func (p *PendingTransaction) BeforeCreate(tx *gorm.DB) error {
	if p.UUID == "" {
		p.UUID = uuid.NewString()
	}
	return nil
}

func (PendingTransaction) TableName() string {
	return "pending_transactions"
}

type TransactionSubmission struct {
	ID                        int64          `gorm:"primaryKey;autoIncrement;index;column:id"`
	UUID                      string         `gorm:"uniqueIndex;index;column:uuid;not null"`
	PendingTransactionUUID    string         `gorm:"column:pending_transaction_uuid;index:idx_submission_pending_uuid"`
	MoneyTrackerTransactionID string         `gorm:"column:money_tracker_transaction_id"`
	RequestSnapshotJSON       *string        `gorm:"column:request_snapshot_json;type:jsonb"`
	ResponseSnapshotJSON      *string        `gorm:"column:response_snapshot_json;type:jsonb"`
	Status                    string         `gorm:"column:status;default:pending;index:idx_submission_status"`
	AttemptCount              int            `gorm:"column:attempt_count;default:1"`
	LastError                 string         `gorm:"column:last_error"`
	CreatedAt                 time.Time      `gorm:"column:created_at;default:now()"`
	UpdatedAt                 time.Time      `gorm:"column:updated_at;default:now()"`
	DeletedAt                 gorm.DeletedAt `gorm:"column:deleted_at;index"`
}

func (s *TransactionSubmission) BeforeCreate(tx *gorm.DB) error {
	if s.UUID == "" {
		s.UUID = uuid.NewString()
	}
	return nil
}

func (TransactionSubmission) TableName() string {
	return "transaction_submissions"
}

type CategoryCache struct {
	UserUUID    string    `gorm:"primaryKey;column:user_uuid"`
	CategoryID  string    `gorm:"primaryKey;column:category_id"`
	Title       string    `gorm:"column:title"`
	Type        int       `gorm:"column:type"`
	RefreshedAt time.Time `gorm:"column:refreshed_at;default:now()"`
}

func (CategoryCache) TableName() string {
	return "money_tracker_categories_cache"
}

type AccountCache struct {
	UserUUID     string    `gorm:"primaryKey;column:user_uuid"`
	AccountID    string    `gorm:"primaryKey;column:account_id"`
	Name         string    `gorm:"column:name"`
	CurrencyCode string    `gorm:"column:currency_code;default:IDR"`
	RefreshedAt  time.Time `gorm:"column:refreshed_at;default:now()"`
}

func (AccountCache) TableName() string {
	return "money_tracker_accounts_cache"
}
