package jobs

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hibiken/asynq"
	"github.com/ramadiaz/money-wa-bot/internal/domain/transaction"
	"github.com/ramadiaz/money-wa-bot/internal/service"
	"github.com/ramadiaz/money-wa-bot/internal/shared/logger"
)

const TypeRetryTransaction = "retry:transaction"

type RetryTransactionPayload struct {
	PendingTransactionID int64 `json:"pending_transaction_id"`
}

type RetryTransactionHandler struct {
	pendingRepo transaction.PendingTransactionRepository
	txSvc       *service.TransactionService
}

func NewRetryTransactionHandler(
	pendingRepo transaction.PendingTransactionRepository,
	txSvc *service.TransactionService,
) *RetryTransactionHandler {
	return &RetryTransactionHandler{
		pendingRepo: pendingRepo,
		txSvc:       txSvc,
	}
}

func (h *RetryTransactionHandler) ProcessTask(ctx context.Context, t *asynq.Task) error {
	var p RetryTransactionPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("unmarshal retry payload: %w", err)
	}

	log := logger.Log.With().Int64("pending_id", p.PendingTransactionID).Logger()

	pending, err := h.pendingRepo.FindActiveByChat(ctx, "")
	if err != nil {
		log.Error().Err(err).Msg("retry: find pending failed")
		return err
	}

	_, err = h.txSvc.Commit(ctx, pending.ID, pending)
	if err != nil {
		log.Error().Err(err).Msg("retry: commit failed")
		return err
	}

	log.Info().Msg("retry: transaction committed successfully")
	return nil
}

func NewRetryTransactionTask(pendingTransactionID int64) *asynq.Task {
	payload, _ := json.Marshal(RetryTransactionPayload{PendingTransactionID: pendingTransactionID})
	return asynq.NewTask(TypeRetryTransaction, payload)
}
