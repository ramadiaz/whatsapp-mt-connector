package transaction

import "context"

type Service interface {
	ProcessText(ctx context.Context, chatID, senderNumber, text, messageID string) error
	ProcessImage(ctx context.Context, chatID, senderNumber, imageMessageID, caption string, inboundID int64) error
	ProcessConfirmation(ctx context.Context, chatID, senderNumber, text, messageID string) error
}
