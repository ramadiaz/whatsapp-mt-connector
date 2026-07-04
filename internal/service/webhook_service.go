package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/hibiken/asynq"
	"github.com/ramadiaz/money-wa-bot/internal/domain/inbound"
	"github.com/ramadiaz/money-wa-bot/internal/domain/transaction"
	"github.com/ramadiaz/money-wa-bot/internal/integration/gowa"
	apperrors "github.com/ramadiaz/money-wa-bot/internal/shared/errors"
	"github.com/ramadiaz/money-wa-bot/internal/shared/logger"
)

type WebhookService struct {
	secret         string
	allowedNumbers []string
	deviceID       string
	inboundRepo    transaction.InboundRepository
	asynqClient    *asynq.Client
}

func NewWebhookService(
	secret string,
	allowedNumbers []string,
	deviceID string,
	inboundRepo transaction.InboundRepository,
	asynqClient *asynq.Client,
) *WebhookService {
	return &WebhookService{
		secret:         secret,
		allowedNumbers: allowedNumbers,
		deviceID:       deviceID,
		inboundRepo:    inboundRepo,
		asynqClient:    asynqClient,
	}
}

func (s *WebhookService) VerifySignature(signature string, body []byte) bool {
	if signature == "" {
		return false
	}
	sig := strings.TrimPrefix(signature, "sha256=")
	mac := hmac.New(sha256.New, []byte(s.secret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(sig), []byte(expected))
}

func (s *WebhookService) Handle(ctx context.Context, correlationID string, body []byte) error {
	log := logger.WithCorrelationID(correlationID)

	var event gowa.WebhookEvent
	if err := json.Unmarshal(body, &event); err != nil {
		return apperrors.ErrInvalidAIResponse
	}

	if event.Event != "message" {
		log.Debug().Str("event", event.Event).Msg("ignored non-message event")
		return nil
	}

	if event.Payload.IsFromMe {
		log.Debug().Msg("ignored is_from_me message")
		return nil
	}

	if event.Payload.IsGroup {
		log.Debug().Msg("ignored group message")
		return nil
	}

	senderNumber := gowa.NormalizeSenderJID(event.Payload.From)
	if !s.isAllowed(senderNumber) {
		log.Warn().Str("sender", senderNumber).Msg("unauthorized sender ignored")
		return apperrors.ErrUnauthorizedSender
	}

	msgType := inbound.MessageTypeText
	if event.Payload.MediaType == "image" {
		msgType = inbound.MessageTypeImage
	} else if event.Payload.Type != "text" && event.Payload.MediaType != "" {
		msgType = inbound.MessageTypeOther
	}

	if msgType == inbound.MessageTypeOther {
		log.Debug().Str("type", event.Payload.Type).Msg("unsupported message type ignored")
		return apperrors.ErrUnsupportedMessageType
	}

	rawJSON, _ := json.Marshal(event.Payload)
	id, err := s.inboundRepo.Insert(ctx, s.deviceID, event.Payload.ID, event.Payload.ChatID, senderNumber, string(msgType), string(rawJSON))
	if err != nil {
		if errors.Is(err, apperrors.ErrDuplicateMessage) {
			log.Warn().Str("message_id", event.Payload.ID).Msg("duplicate message ignored")
			return nil
		}
		return err
	}

	log.Info().Int64("inbound_id", id).Str("type", string(msgType)).Msg("enqueuing message")

	payload, _ := json.Marshal(map[string]interface{}{
		"inbound_id":    id,
		"chat_id":       event.Payload.ChatID,
		"sender_number": senderNumber,
		"message_id":    event.Payload.ID,
		"type":          string(msgType),
		"body":          event.Payload.Body,
		"caption":       event.Payload.Caption,
		"device_id":     s.deviceID,
		"correlation_id": correlationID,
	})

	task := asynq.NewTask("process:message", payload, asynq.Queue("default"))
	_, err = s.asynqClient.EnqueueContext(ctx, task)
	if err != nil {
		return fmt.Errorf("enqueue message: %w", err)
	}

	return nil
}

func (s *WebhookService) isAllowed(number string) bool {
	for _, allowed := range s.allowedNumbers {
		if number == allowed {
			return true
		}
	}
	return false
}
