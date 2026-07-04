package handler

import (
	"io"
	"net/http"

	"github.com/ramadiaz/money-wa-bot/internal/service"
	apperrors "github.com/ramadiaz/money-wa-bot/internal/shared/errors"
	"github.com/ramadiaz/money-wa-bot/internal/shared/logger"
	"errors"
)

type WebhookHandler struct {
	webhookSvc *service.WebhookService
}

func NewWebhookHandler(webhookSvc *service.WebhookService) *WebhookHandler {
	return &WebhookHandler{webhookSvc: webhookSvc}
}

func (h *WebhookHandler) Handle(w http.ResponseWriter, r *http.Request) {
	correlationID := r.Header.Get("X-Request-Id")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	signature := r.Header.Get("X-Hub-Signature-256")
	log := logger.WithCorrelationID(correlationID)
	if !h.webhookSvc.VerifySignature(signature, body) {
		l := log
		l.Warn().Msg("invalid webhook signature")
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	err = h.webhookSvc.Handle(r.Context(), correlationID, body)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrUnauthorizedSender):
			w.WriteHeader(http.StatusAccepted)
			return
		case errors.Is(err, apperrors.ErrUnsupportedMessageType):
			w.WriteHeader(http.StatusAccepted)
			return
		default:
			l := log
			l.Error().Err(err).Msg("webhook handle error")
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
	}

	w.WriteHeader(http.StatusAccepted)
}
