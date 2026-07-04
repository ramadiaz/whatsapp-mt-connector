package app

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hibiken/asynq"
	"github.com/joho/godotenv"
	"github.com/ramadiaz/money-wa-bot/internal/config"
	deliveryhttp "github.com/ramadiaz/money-wa-bot/internal/delivery/http"
	"github.com/ramadiaz/money-wa-bot/internal/delivery/http/handler"
	gowaintegration "github.com/ramadiaz/money-wa-bot/internal/integration/gowa"
	"github.com/ramadiaz/money-wa-bot/internal/integration/moneytracker"
	"github.com/ramadiaz/money-wa-bot/internal/integration/ninerouter"
	"github.com/ramadiaz/money-wa-bot/internal/jobs"
	"github.com/ramadiaz/money-wa-bot/internal/persistence/postgres"
	redisqueue "github.com/ramadiaz/money-wa-bot/internal/persistence/redis"
	"github.com/ramadiaz/money-wa-bot/internal/service"
	"github.com/ramadiaz/money-wa-bot/internal/shared/logger"
	gormpostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func Run() error {
	_ = godotenv.Load()

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logger.Init(cfg.AppEnv)
	log := logger.Log

	db, err := gorm.Open(gormpostgres.Open(cfg.DatabaseURL), &gorm.Config{})
	if err != nil {
		return fmt.Errorf("connect postgres: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("get sql db: %w", err)
	}
	defer sqlDB.Close()

	if err := sqlDB.Ping(); err != nil {
		return fmt.Errorf("ping postgres: %w", err)
	}

	err = db.AutoMigrate(
		&postgres.InboundMessage{},
		&postgres.PendingTransaction{},
		&postgres.TransactionSubmission{},
		&postgres.CategoryCache{},
		&postgres.AccountCache{},
	)
	if err != nil {
		return fmt.Errorf("auto migrate: %w", err)
	}

	gowaTimeout := time.Duration(cfg.AITimeoutSeconds) * time.Second
	gowaClient := gowaintegration.NewClient(cfg.GOWAHost, cfg.GOWAUsername, cfg.GOWAPassword, gowaTimeout)

	mtClient := moneytracker.NewClient(cfg.MTHost, cfg.MTAPIKey, 30*time.Second)

	aiTimeout := time.Duration(cfg.AITimeoutSeconds) * time.Second
	nineClient, err := ninerouter.NewClient(cfg.NineRouterEndpoint, cfg.NineRouterAPIKey, cfg.NineRouterModel, cfg.NineRouterVisionModel, aiTimeout)
	if err != nil {
		return fmt.Errorf("init 9router: %w", err)
	}

	inboundRepo := postgres.NewInboundRepository(db)
	pendingRepo := postgres.NewPendingTransactionRepository(db)
	submissionRepo := postgres.NewSubmissionRepository(db)
	catCacheRepo := postgres.NewCategoryCacheRepository(db)
	accCacheRepo := postgres.NewAccountCacheRepository(db)

	asynqClient := redisqueue.NewAsynqClient(cfg.RedisURL)
	defer asynqClient.Close()

	webhookSvc := service.NewWebhookService(cfg.GOWAWebhookSecret, cfg.AllowedNumbers, cfg.GOWADeviceID, inboundRepo, asynqClient)
	parserSvc := service.NewParserService(gowaClient, nineClient, catCacheRepo, accCacheRepo, cfg.GOWADeviceID, cfg.MaxMediaBytes, cfg.MaxAIRetries)
	txSvc := service.NewTransactionService(mtClient, catCacheRepo, accCacheRepo, pendingRepo, submissionRepo)
	confirmationSvc := service.NewConfirmationService(pendingRepo, txSvc, gowaClient, cfg.GOWADeviceID)

	processHandler := jobs.NewProcessMessageHandler(inboundRepo, parserSvc, txSvc, confirmationSvc, gowaClient, cfg.GOWADeviceID)
	refreshHandler := jobs.NewRefreshMTCacheHandler(mtClient, catCacheRepo, accCacheRepo)

	asynqServer := redisqueue.NewAsynqServer(cfg.RedisURL)
	mux := asynq.NewServeMux()
	mux.HandleFunc(jobs.TypeProcessMessage, processHandler.ProcessTask)
	mux.HandleFunc(jobs.TypeRefreshMTCache, refreshHandler.ProcessTask)

	scheduler := redisqueue.NewAsynqScheduler(cfg.RedisURL)
	ttl := fmt.Sprintf("@every %dm", cfg.MTCacheTTLMinutes)
	_, err = scheduler.Register(ttl, jobs.NewRefreshCacheTask())
	if err != nil {
		log.Warn().Err(err).Msg("register scheduler")
	}

	if err := refreshHandler.ProcessTask(context.Background(), nil); err != nil {
		log.Warn().Err(err).Msg("initial cache refresh failed")
	}

	webhookH := handler.NewWebhookHandler(webhookSvc)
	healthH := handler.NewHealthHandler(db, gowaClient)

	router := deliveryhttp.NewRouter(webhookH, healthH)

	addr := fmt.Sprintf("%s:%s", cfg.AppHost, cfg.AppPort)
	srv := &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Info().Str("addr", addr).Msg("HTTP server starting")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("HTTP server error")
		}
	}()

	go func() {
		log.Info().Msg("Asynq worker starting")
		if err := asynqServer.Run(mux); err != nil {
			log.Fatal().Err(err).Msg("Asynq worker error")
		}
	}()

	go func() {
		log.Info().Msg("Asynq scheduler starting")
		if err := scheduler.Run(); err != nil {
			log.Fatal().Err(err).Msg("Asynq scheduler error")
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info().Msg("shutting down")

	shutCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	asynqServer.Shutdown()
	scheduler.Shutdown()

	return srv.Shutdown(shutCtx)
}
