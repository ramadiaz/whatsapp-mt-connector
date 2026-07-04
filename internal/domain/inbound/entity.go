package inbound

import "time"

type Status string

const (
	StatusPending    Status = "pending"
	StatusProcessing Status = "processing"
	StatusDone       Status = "done"
	StatusFailed     Status = "failed"
	StatusIgnored    Status = "ignored"
)

type MessageType string

const (
	MessageTypeText  MessageType = "text"
	MessageTypeImage MessageType = "image"
	MessageTypeOther MessageType = "other"
)

type Message struct {
	ID             int64
	GowaDeviceID   string
	GowaMessageID  string
	ChatID         string
	SenderNumber   string
	MessageType    MessageType
	RawPayloadJSON string
	ReceivedAt     time.Time
	ProcessedAt    *time.Time
	Status         Status
}
