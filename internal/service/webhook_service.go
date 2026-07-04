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
	log.Debug().Msg("parsing webhook event json payload")

	var event gowa.WebhookEvent
	if err := json.Unmarshal(body, &event); err != nil {
		log.Error().Err(err).Msg("unmarshal webhook event payload failed")
		return apperrors.ErrInvalidAIResponse
	}

	if event.Event != "message" {
		log.Debug().Str("event", event.Event).Msg("ignored non-message event type")
		return nil
	}

	if event.Payload.IsFromMe {
		log.Debug().Str("message_id", event.Payload.ID).Msg("ignored outgoing message from current device")
		return nil
	}

	if event.Payload.IsGroup {
		log.Debug().Str("message_id", event.Payload.ID).Msg("ignored incoming group message")
		return nil
	}

	senderNumber := gowa.NormalizeSenderJID(event.Payload.From)
	log.Info().Str("sender", senderNumber).Str("message_id", event.Payload.ID).Msg("verifying sender authorization")
	if !s.isAllowed(senderNumber) {
		log.Warn().Str("sender", senderNumber).Msg("sender phone number is not on authorized list")
		return apperrors.ErrUnauthorizedSender
	}

	msgType := inbound.MessageTypeText
	if event.Payload.MediaType == "image" || event.Payload.Image != nil {
		msgType = inbound.MessageTypeImage
	} else if event.Payload.Type != "text" && event.Payload.MediaType != "" {
		msgType = inbound.MessageTypeOther
	}

	log.Info().Str("type", string(msgType)).Msg("categorizing inbound message type")
	if msgType == inbound.MessageTypeOther {
		log.Debug().Str("type", event.Payload.Type).Msg("ignoring unsupported message payload format")
		return apperrors.ErrUnsupportedMessageType
	}

	rawJSON, _ := json.Marshal(event.Payload)
	log.Info().Str("message_id", event.Payload.ID).Msg("inserting inbound message into database")
	id, err := s.inboundRepo.Insert(ctx, s.deviceID, event.Payload.ID, event.Payload.ChatID, senderNumber, string(msgType), string(rawJSON))
	if err != nil {
		if errors.Is(err, apperrors.ErrDuplicateMessage) {
			log.Warn().Str("message_id", event.Payload.ID).Msg("duplicate message received and ignored")
			return nil
		}
		log.Error().Err(err).Msg("failed database insertion of inbound message")
		return err
	}

	log.Info().Int64("inbound_id", id).Str("type", string(msgType)).Msg("enqueuing message processing task to queue")

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
		log.Error().Err(err).Int64("inbound_id", id).Msg("enqueuing process task failed")
		return fmt.Errorf("enqueue message: %w", err)
	}

	log.Info().Int64("inbound_id", id).Msg("enqueued process task successfully")
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
