package jobs

import (
	"context"
	"encoding/json"
	"time"

	"github.com/hibiken/asynq"
	"github.com/ramadiaz/whatsapp-mt-connector/internal/domain/transaction"
	"github.com/ramadiaz/whatsapp-mt-connector/internal/integration/moneytracker"
	"github.com/ramadiaz/whatsapp-mt-connector/internal/persistence/postgres"
	"github.com/ramadiaz/whatsapp-mt-connector/internal/shared/logger"
	"gorm.io/gorm"
)

const TypeRefreshMTCache = "cache:refresh"

type RefreshMTCacheHandler struct {
	db            *gorm.DB
	userRepo      *postgres.UserRepository
	catCacheRepo  transaction.CategoryCacheRepository
	accCacheRepo  transaction.AccountCacheRepository
	mtHost        string
	defaultAPIKey string
}

func NewRefreshMTCacheHandler(
	db *gorm.DB,
	userRepo *postgres.UserRepository,
	catCacheRepo transaction.CategoryCacheRepository,
	accCacheRepo transaction.AccountCacheRepository,
	mtHost string,
	defaultAPIKey string,
) *RefreshMTCacheHandler {
	return &RefreshMTCacheHandler{
		db:            db,
		userRepo:      userRepo,
		catCacheRepo:  catCacheRepo,
		accCacheRepo:  accCacheRepo,
		mtHost:        mtHost,
		defaultAPIKey: defaultAPIKey,
	}
}

func (h *RefreshMTCacheHandler) ProcessTask(ctx context.Context, _ *asynq.Task) error {
	log := logger.Log

	var users []postgres.User
	if err := h.db.WithContext(ctx).Find(&users).Error; err != nil {
		log.Error().Err(err).Msg("refresh: list users failed")
		return err
	}

	for _, u := range users {
		key := u.MTAPIKey
		if key == "" {
			continue
		}
		log.Info().Str("user_uuid", u.UUID).Msg("refreshing cache for user")
		mtClient := moneytracker.NewClient(h.mtHost, key, 30*time.Second)

		categories, err := mtClient.GetCategories(ctx)
		if err != nil {
			log.Error().Err(err).Str("user_uuid", u.UUID).Msg("refresh: get categories failed")
			continue
		}

		if err := h.catCacheRepo.Upsert(ctx, u.UUID, categories); err != nil {
			log.Error().Err(err).Str("user_uuid", u.UUID).Msg("refresh: upsert categories failed")
			continue
		}

		accounts, err := mtClient.GetAccounts(ctx)
		if err != nil {
			log.Error().Err(err).Str("user_uuid", u.UUID).Msg("refresh: get accounts failed")
			continue
		}

		if err := h.accCacheRepo.Upsert(ctx, u.UUID, accounts); err != nil {
			log.Error().Err(err).Str("user_uuid", u.UUID).Msg("refresh: upsert accounts failed")
			continue
		}
	}

	log.Info().Msg("MT cache refreshed for all users")
	return nil
}

func NewRefreshCacheTask() *asynq.Task {
	payload, _ := json.Marshal(map[string]string{})
	return asynq.NewTask(TypeRefreshMTCache, payload)
}
