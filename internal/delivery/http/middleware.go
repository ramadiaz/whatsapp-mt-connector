package http

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/ramadiaz/money-wa-bot/internal/shared/logger"
)

func correlationIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		correlationID := r.Header.Get("X-Request-Id")
		if correlationID == "" {
			correlationID = uuid.New().String()
		}
		r = r.WithContext(context.WithValue(r.Context(), ctxKeyCorrelationID, correlationID))
		w.Header().Set("X-Request-Id", correlationID)
		next.ServeHTTP(w, r)
	})
}

func requestLoggerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		correlationID, _ := r.Context().Value(ctxKeyCorrelationID).(string)
		log := logger.WithCorrelationID(correlationID)

		next.ServeHTTP(w, r)

		log.Info().
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Dur("latency", time.Since(start)).
			Msg("request")
	})
}

type contextKey string

const ctxKeyCorrelationID contextKey = "correlation_id"
