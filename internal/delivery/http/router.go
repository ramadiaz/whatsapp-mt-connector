package http

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/ramadiaz/money-wa-bot/internal/delivery/http/handler"
)

func NewRouter(webhookHandler *handler.WebhookHandler, healthHandler *handler.HealthHandler) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))
	r.Use(correlationIDMiddleware)
	r.Use(requestLoggerMiddleware)

	r.Get("/healthz", healthHandler.Healthz)
	r.Get("/readyz", healthHandler.Readyz)

	r.Post("/webhooks/gowa", webhookHandler.Handle)

	return r
}
