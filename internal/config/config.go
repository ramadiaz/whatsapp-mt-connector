package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	MTHost    string
	MTAPIKey  string

	GOWAHost     string
	GOWAUsername string
	GOWAPassword string
	GOWAWebhookSecret string
	GOWADeviceID string

	AllowedNumbers []string

	NineRouterEndpoint    string
	NineRouterAPIKey      string
	NineRouterModel       string
	NineRouterVisionModel string

	AppEnv      string
	AppHost     string
	AppPort     string
	AppTimezone string

	DatabaseURL string
	RedisURL    string

	ConfirmationMode       string
	AutoCommitMinConfidence float64
	MaxMediaBytes          int64
	MaxAIRetries           int
	AITimeoutSeconds       int
	MTCacheTTLMinutes      int
}

func Load() (*Config, error) {
	cfg := &Config{
		MTHost:   mustGet("MT_HOST"),
		MTAPIKey: mustGet("MT_API_KEY"),

		GOWAHost:          mustGet("GOWA_HOST"),
		GOWAUsername:      mustGet("GOWA_USERNAME"),
		GOWAPassword:      mustGet("GOWA_PASSWORD"),
		GOWAWebhookSecret: mustGet("GOWA_WEBHOOK_SECRET"),
		GOWADeviceID:      mustGet("GOWA_DEVICE_ID"),

		NineRouterEndpoint:    mustGet("9ROUTER_ENDPOINT"),
		NineRouterAPIKey:      mustGet("9ROUTER_API_KEY"),
		NineRouterModel:       mustGet("9ROUTER_MODEL"),
		NineRouterVisionModel: getEnv("9ROUTER_VISION_MODEL", ""),

		AppEnv:      getEnv("APP_ENV", "production"),
		AppHost:     getEnv("APP_HOST", "0.0.0.0"),
		AppPort:     getEnv("APP_PORT", "8080"),
		AppTimezone: getEnv("APP_TIMEZONE", "Asia/Jakarta"),

		DatabaseURL: mustGet("DATABASE_URL"),
		RedisURL:    mustGet("REDIS_URL"),

		ConfirmationMode:        getEnv("CONFIRMATION_MODE", "always"),
		AutoCommitMinConfidence: parseFloat(getEnv("AUTO_COMMIT_MIN_CONFIDENCE", "0.98")),
		MaxMediaBytes:           parseInt64(getEnv("MAX_MEDIA_BYTES", "5242880")),
		MaxAIRetries:            parseInt(getEnv("MAX_AI_RETRIES", "1")),
		AITimeoutSeconds:        parseInt(getEnv("AI_TIMEOUT_SECONDS", "45")),
		MTCacheTTLMinutes:       parseInt(getEnv("MT_CACHE_TTL_MINUTES", "60")),
	}

	raw := mustGet("WHATSAPP_ALLOWED_NUMBER")
	for _, n := range strings.Split(raw, ",") {
		n = strings.TrimSpace(n)
		if n != "" {
			cfg.AllowedNumbers = append(cfg.AllowedNumbers, n)
		}
	}

	if len(cfg.AllowedNumbers) == 0 {
		return nil, fmt.Errorf("WHATSAPP_ALLOWED_NUMBER must not be empty")
	}

	return cfg, nil
}

func mustGet(key string) string {
	v := os.Getenv(key)
	if v == "" {
		panic(fmt.Sprintf("required env var %s is not set", key))
	}
	return v
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func parseInt(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

func parseInt64(s string) int64 {
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}

func parseFloat(s string) float64 {
	f, _ := strconv.ParseFloat(s, 64)
	return f
}
