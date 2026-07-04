package errors

import "errors"

var (
	ErrInvalidWebhookSignature      = errors.New("invalid_webhook_signature")
	ErrUnauthorizedSender           = errors.New("unauthorized_sender")
	ErrDuplicateMessage             = errors.New("duplicate_message")
	ErrUnsupportedMessageType       = errors.New("unsupported_message_type")
	ErrMediaTooLarge                = errors.New("media_too_large")
	ErrInvalidAIResponse            = errors.New("invalid_ai_response")
	ErrMissingTransactionData       = errors.New("missing_transaction_data")
	ErrUnknownCategory              = errors.New("unknown_category")
	ErrUnknownAccount               = errors.New("unknown_account")
	ErrMoneyTrackerRejected         = errors.New("money_tracker_rejected")
	ErrMoneyTrackerUnavailable      = errors.New("money_tracker_unavailable")
	ErrGOWAUnavailable              = errors.New("gowa_unavailable")
	ErrAIUnavailable                = errors.New("ai_unavailable")
	ErrPendingTransactionExpired    = errors.New("pending_transaction_expired")
	ErrDuplicateTransactionSubmission = errors.New("duplicate_transaction_submission")
	ErrNoPendingTransaction         = errors.New("no_pending_transaction")
)
