package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
	"github.com/ramadiaz/whatsapp-mt-connector/internal/domain/transaction"
	"github.com/ramadiaz/whatsapp-mt-connector/internal/integration/moneytracker"
	"github.com/ramadiaz/whatsapp-mt-connector/internal/persistence/postgres"
	"github.com/ramadiaz/whatsapp-mt-connector/internal/service"
	"github.com/ramadiaz/whatsapp-mt-connector/internal/shared/logger"
)

const TypeRetryTransaction = "retry:transaction"

type RetryTransactionPayload struct {
	PendingTransactionUUID string `json:"pending_transaction_uuid"`
	CorrelationID          string `json:"correlation_id"`
}

type RetryTransactionHandler struct {
	pendingRepo transaction.PendingTransactionRepository
	userRepo    *postgres.UserRepository
	txSvc       *service.TransactionService
	mtHost      string
}

func NewRetryTransactionHandler(
	pendingRepo transaction.PendingTransactionRepository,
	userRepo *postgres.UserRepository,
	txSvc *service.TransactionService,
	mtHost string,
) *RetryTransactionHandler {
	return &RetryTransactionHandler{
		pendingRepo: pendingRepo,
		userRepo:    userRepo,
		txSvc:       txSvc,
		mtHost:      mtHost,
	}
}

func (h *RetryTransactionHandler) ProcessTask(ctx context.Context, t *asynq.Task) error {
	var p RetryTransactionPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("unmarshal retry payload: %w", err)
	}

	log := logger.WithCorrelationID(p.CorrelationID)
	log.Info().Str("pending_uuid", p.PendingTransactionUUID).Msg("retrying pending transaction")

	pending, err := h.pendingRepo.FindActiveByChat(ctx, p.PendingTransactionUUID)
	if err != nil {
		log.Error().Err(err).Str("pending_uuid", p.PendingTransactionUUID).Msg("retry: find pending failed")
		return err
	}

	user, err := h.userRepo.FindByUUID(ctx, pending.UserUUID)
	if err != nil {
		log.Error().Err(err).Str("pending_uuid", p.PendingTransactionUUID).Msg("find user for retry failed")
		return err
	}

	mtClient := moneytracker.NewClient(h.mtHost, user.MTAPIKey, 30*time.Second)
	_, err = h.txSvc.Commit(ctx, pending.UUID, pending, mtClient)
	if err != nil {
		log.Error().Err(err).Str("pending_uuid", pending.UUID).Msg("retry commit failed")
		return err
	}

	log.Info().Str("pending_uuid", pending.UUID).Msg("retry transaction committed successfully")
	return nil
}

func NewRetryTransactionTask(pendingTransactionUUID string, correlationID string) (*asynq.Task, error) {
	payload, err := json.Marshal(RetryTransactionPayload{
		PendingTransactionUUID: pendingTransactionUUID,
		CorrelationID:          correlationID,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal retry payload: %w", err)
	}
	return asynq.NewTask(TypeRetryTransaction, payload), nil
}
