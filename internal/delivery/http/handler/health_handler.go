package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ramadiaz/money-wa-bot/internal/integration/gowa"
)

type HealthHandler struct {
	db         *pgxpool.Pool
	gowaClient gowa.WhatsAppGateway
}

func NewHealthHandler(db *pgxpool.Pool, gowaClient gowa.WhatsAppGateway) *HealthHandler {
	return &HealthHandler{db: db, gowaClient: gowaClient}
}

func (h *HealthHandler) Healthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (h *HealthHandler) Readyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	status := map[string]string{}
	httpStatus := http.StatusOK

	if err := h.db.Ping(ctx); err != nil {
		status["postgres"] = "down: " + err.Error()
		httpStatus = http.StatusServiceUnavailable
	} else {
		status["postgres"] = "ok"
	}

	if err := h.gowaClient.Health(ctx); err != nil {
		status["gowa"] = "down: " + err.Error()
	} else {
		status["gowa"] = "ok"
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus)
	json.NewEncoder(w).Encode(status)
}
