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

	log.Info().Msg("starting money-wa-bot application")
	log.Info().Str("env", cfg.AppEnv).Msg("loading configurations")

	log.Info().Str("dsn", cfg.DatabaseURL).Msg("connecting to postgres via gorm")
	db, err := gorm.Open(gormpostgres.Open(cfg.DatabaseURL), &gorm.Config{})
	if err != nil {
		return fmt.Errorf("connect postgres: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("get sql db: %w", err)
	}
	defer sqlDB.Close()

	log.Info().Msg("pinging database connection")
	if err := sqlDB.Ping(); err != nil {
		return fmt.Errorf("ping postgres: %w", err)
	}

	log.Info().Msg("running database auto migration")
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
	log.Info().Msg("database auto migration successful")

	log.Info().Str("host", cfg.GOWAHost).Msg("initializing gowa client")
	gowaTimeout := time.Duration(cfg.AITimeoutSeconds) * time.Second
	gowaClient := gowaintegration.NewClient(cfg.GOWAHost, cfg.GOWAUsername, cfg.GOWAPassword, gowaTimeout)

	log.Info().Str("host", cfg.MTHost).Msg("initializing moneytracker client")
	mtClient := moneytracker.NewClient(cfg.MTHost, cfg.MTAPIKey, 30*time.Second)

	log.Info().Str("endpoint", cfg.NineRouterEndpoint).Str("model", cfg.NineRouterModel).Msg("initializing 9router client")
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

	log.Info().Str("host", cfg.RedisHost).Str("port", cfg.RedisPort).Msg("connecting to redis asynq client")
	asynqClient := redisqueue.NewAsynqClient(cfg.RedisHost, cfg.RedisPort, cfg.RedisPassword, cfg.RedisDB)
	defer asynqClient.Close()

	webhookSvc := service.NewWebhookService(cfg.GOWAWebhookSecret, cfg.AllowedNumbers, cfg.GOWADeviceID, inboundRepo, asynqClient)
	parserSvc := service.NewParserService(gowaClient, nineClient, catCacheRepo, accCacheRepo, cfg.GOWADeviceID, cfg.MaxMediaBytes, cfg.MaxAIRetries)
	txSvc := service.NewTransactionService(mtClient, catCacheRepo, accCacheRepo, pendingRepo, submissionRepo)
	confirmationSvc := service.NewConfirmationService(pendingRepo, txSvc, gowaClient, cfg.GOWADeviceID)

	processHandler := jobs.NewProcessMessageHandler(inboundRepo, parserSvc, txSvc, confirmationSvc, gowaClient, cfg.GOWADeviceID)
	refreshHandler := jobs.NewRefreshMTCacheHandler(mtClient, catCacheRepo, accCacheRepo)

	asynqServer := redisqueue.NewAsynqServer(cfg.RedisHost, cfg.RedisPort, cfg.RedisPassword, cfg.RedisDB)
	mux := asynq.NewServeMux()
	mux.HandleFunc(jobs.TypeProcessMessage, processHandler.ProcessTask)
	mux.HandleFunc(jobs.TypeRefreshMTCache, refreshHandler.ProcessTask)

	log.Info().Msg("registering cache refresh schedule")
	scheduler := redisqueue.NewAsynqScheduler(cfg.RedisHost, cfg.RedisPort, cfg.RedisPassword, cfg.RedisDB)
	ttl := fmt.Sprintf("@every %dm", cfg.MTCacheTTLMinutes)
	_, err = scheduler.Register(ttl, jobs.NewRefreshCacheTask())
	if err != nil {
		log.Warn().Err(err).Msg("register scheduler")
	}

	log.Info().Msg("triggering initial mt cache refresh")
	if err := refreshHandler.ProcessTask(context.Background(), nil); err != nil {
		log.Warn().Err(err).Msg("initial cache refresh failed")
	} else {
		log.Info().Msg("initial mt cache refresh successful")
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

	log.Info().Msg("shutting down application")

	shutCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	asynqServer.Shutdown()
	scheduler.Shutdown()

	log.Info().Msg("graceful shutdown completed")
	return srv.Shutdown(shutCtx)
}
