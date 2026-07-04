package jobs

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hibiken/asynq"
	"github.com/ramadiaz/whatsapp-mt-connector/internal/domain/transaction"
	"github.com/ramadiaz/whatsapp-mt-connector/internal/integration/moneytracker"
	"github.com/ramadiaz/whatsapp-mt-connector/internal/shared/logger"
)

const TypeRefreshMTCache = "cache:refresh"

type RefreshMTCacheHandler struct {
	mtClient      moneytracker.MoneyTrackerClient
	catCacheRepo  transaction.CategoryCacheRepository
	accCacheRepo  transaction.AccountCacheRepository
}

func NewRefreshMTCacheHandler(
	mtClient moneytracker.MoneyTrackerClient,
	catCacheRepo transaction.CategoryCacheRepository,
	accCacheRepo transaction.AccountCacheRepository,
) *RefreshMTCacheHandler {
	return &RefreshMTCacheHandler{
		mtClient:     mtClient,
		catCacheRepo: catCacheRepo,
		accCacheRepo: accCacheRepo,
	}
}

func (h *RefreshMTCacheHandler) ProcessTask(ctx context.Context, _ *asynq.Task) error {
	log := logger.Log

	categories, err := h.mtClient.GetCategories(ctx)
	if err != nil {
		log.Error().Err(err).Msg("refresh: get categories failed")
		return fmt.Errorf("get categories: %w", err)
	}

	if err := h.catCacheRepo.Upsert(ctx, categories); err != nil {
		log.Error().Err(err).Msg("refresh: upsert categories failed")
		return err
	}

	accounts, err := h.mtClient.GetAccounts(ctx)
	if err != nil {
		log.Error().Err(err).Msg("refresh: get accounts failed")
		return fmt.Errorf("get accounts: %w", err)
	}

	if err := h.accCacheRepo.Upsert(ctx, accounts); err != nil {
		log.Error().Err(err).Msg("refresh: upsert accounts failed")
		return err
	}

	log.Info().Int("categories", len(categories)).Int("accounts", len(accounts)).Msg("MT cache refreshed")
	return nil
}

func NewRefreshCacheTask() *asynq.Task {
	payload, _ := json.Marshal(map[string]string{})
	return asynq.NewTask(TypeRefreshMTCache, payload)
}
