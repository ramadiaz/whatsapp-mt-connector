package jobs

import (
	"context"
	"encoding/json"
	"time"

	"github.com/hibiken/asynq"
	"github.com/ramadiaz/whatsapp-mt-connector/internal/integration/gowa"
	"github.com/ramadiaz/whatsapp-mt-connector/internal/integration/moneytracker"
	"github.com/ramadiaz/whatsapp-mt-connector/internal/persistence/postgres"
	"github.com/ramadiaz/whatsapp-mt-connector/internal/shared/logger"
)

const TypeDailyReminder = "reminder:daily"

type DailyReminderHandler struct {
	userRepo   *postgres.UserRepository
	gowaClient gowa.WhatsAppGateway
	deviceID   string
	mtHost     string
}

func NewDailyReminderHandler(
	userRepo *postgres.UserRepository,
	gowaClient gowa.WhatsAppGateway,
	deviceID string,
	mtHost string,
) *DailyReminderHandler {
	return &DailyReminderHandler{
		userRepo:   userRepo,
		gowaClient: gowaClient,
		deviceID:   deviceID,
		mtHost:     mtHost,
	}
}

func (h *DailyReminderHandler) ProcessTask(ctx context.Context, _ *asynq.Task) error {
	log := logger.Log

	users, err := h.userRepo.FindUsersWithAPIKey(ctx)
	if err != nil {
		log.Error().Err(err).Msg("daily reminder: fetch users failed")
		return err
	}

	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		loc = time.FixedZone("WIB", 7*3600)
	}
	today := time.Now().In(loc).Format("2006-01-02")

	for _, user := range users {
		if user.MTAPIKey == "" {
			continue
		}

		mtClient := moneytracker.NewClient(h.mtHost, user.MTAPIKey, 30*time.Second)
		txs, err := mtClient.GetTransactionsDateRange(ctx, today, today)
		if err != nil {
			log.Error().Err(err).Str("user_uuid", user.UUID).Msg("daily reminder: get transactions failed")
			continue
		}

		if len(txs) == 0 {
			msg := "...a-ano... (aku gemetaran nulis ini) ...hari ini belum ada catatan transaksi... 😶 K-kalau sempat... jangan lupa catat pengeluaranmu ya... m-maaf ya ngingetin. 🙏"
			if err := h.gowaClient.SendText(ctx, h.deviceID, user.PhoneNumber, msg, ""); err != nil {
				log.Error().Err(err).Str("user_uuid", user.UUID).Msg("daily reminder: send message failed")
			} else {
				log.Info().Str("user_uuid", user.UUID).Msg("daily reminder message sent")
			}
		}
	}

	return nil
}

func NewDailyReminderTask() *asynq.Task {
	payload, _ := json.Marshal(map[string]string{})
	return asynq.NewTask(TypeDailyReminder, payload)
}
