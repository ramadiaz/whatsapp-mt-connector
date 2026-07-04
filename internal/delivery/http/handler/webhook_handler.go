package handler

import (
	"errors"
	"io"
	"net/http"

	"github.com/ramadiaz/whatsapp-mt-connector/internal/service"
	apperrors "github.com/ramadiaz/whatsapp-mt-connector/internal/shared/errors"
	"github.com/ramadiaz/whatsapp-mt-connector/internal/shared/logger"
)

type WebhookHandler struct {
	webhookSvc *service.WebhookService
}

func NewWebhookHandler(webhookSvc *service.WebhookService) *WebhookHandler {
	return &WebhookHandler{webhookSvc: webhookSvc}
}

func (h *WebhookHandler) Handle(w http.ResponseWriter, r *http.Request) {
	correlationID := r.Header.Get("X-Request-Id")
	log := logger.WithCorrelationID(correlationID)
	log.Info().Str("method", r.Method).Str("path", r.URL.Path).Msg("received webhook request")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Error().Err(err).Msg("failed to read request body")
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	signature := r.Header.Get("X-Hub-Signature-256")
	log.Debug().Int("body_len", len(body)).Str("signature", signature).Msg("verifying webhook signature")
	if !h.webhookSvc.VerifySignature(signature, body) {
		log.Warn().Msg("unauthorized webhook signature mismatch")
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	log.Info().Msg("signature verified successfully, invoking webhook service")
	err = h.webhookSvc.Handle(r.Context(), correlationID, body)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrUnauthorizedSender):
			log.Warn().Err(err).Msg("sender unauthorized, accepted without further processing")
			w.WriteHeader(http.StatusAccepted)
			return
		case errors.Is(err, apperrors.ErrUnsupportedMessageType):
			log.Warn().Err(err).Msg("message type unsupported, accepted without further processing")
			w.WriteHeader(http.StatusAccepted)
			return
		default:
			log.Error().Err(err).Msg("internal error handling webhook")
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
	}

	log.Info().Msg("webhook request accepted and enqueued")
	w.WriteHeader(http.StatusAccepted)
}
