package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"time"

	"github.com/hibiken/asynq"
	"github.com/ramadiaz/whatsapp-mt-connector/internal/domain/transaction"
	"github.com/ramadiaz/whatsapp-mt-connector/internal/integration/gowa"
	"github.com/ramadiaz/whatsapp-mt-connector/internal/integration/moneytracker"
	"github.com/ramadiaz/whatsapp-mt-connector/internal/integration/ninerouter"
	"github.com/ramadiaz/whatsapp-mt-connector/internal/persistence/postgres"
	"github.com/ramadiaz/whatsapp-mt-connector/internal/service"
	apperrors "github.com/ramadiaz/whatsapp-mt-connector/internal/shared/errors"
	"github.com/ramadiaz/whatsapp-mt-connector/internal/shared/logger"
	"github.com/ramadiaz/whatsapp-mt-connector/internal/shared/money"
	"github.com/ramadiaz/whatsapp-mt-connector/internal/shared/timeutil"
	"github.com/shopspring/decimal"
)

const TypeProcessMessage = "process:message"

type ProcessMessagePayload struct {
	InboundID     int64  `json:"inbound_id"`
	ChatID        string `json:"chat_id"`
	SenderNumber  string `json:"sender_number"`
	MessageID     string `json:"message_id"`
	Type          string `json:"type"`
	Body          string `json:"body"`
	Caption       string `json:"caption"`
	DeviceID      string `json:"device_id"`
	CorrelationID string `json:"correlation_id"`
}

type ProcessMessageHandler struct {
	inboundRepo     transaction.InboundRepository
	userRepo        *postgres.UserRepository
	parserSvc       *service.ParserService
	txSvc           *service.TransactionService
	confirmationSvc *service.ConfirmationService
	gowaClient      gowa.WhatsAppGateway
	deviceID        string
	adminNumbers    []string
	defaultAPIKey   string
	mtHost          string
}

func NewProcessMessageHandler(
	inboundRepo transaction.InboundRepository,
	userRepo *postgres.UserRepository,
	parserSvc *service.ParserService,
	txSvc *service.TransactionService,
	confirmationSvc *service.ConfirmationService,
	gowaClient gowa.WhatsAppGateway,
	deviceID string,
	adminNumbers []string,
	defaultAPIKey string,
	mtHost string,
) *ProcessMessageHandler {
	return &ProcessMessageHandler{
		inboundRepo:     inboundRepo,
		userRepo:        userRepo,
		parserSvc:       parserSvc,
		txSvc:           txSvc,
		confirmationSvc: confirmationSvc,
		gowaClient:      gowaClient,
		deviceID:        deviceID,
		adminNumbers:    adminNumbers,
		defaultAPIKey:   defaultAPIKey,
		mtHost:          mtHost,
	}
}

func (h *ProcessMessageHandler) ProcessTask(ctx context.Context, t *asynq.Task) error {
	var p ProcessMessagePayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("unmarshal payload: %w", err)
	}

	log := logger.WithCorrelationID(p.CorrelationID)
	log.Info().Int64("inbound_id", p.InboundID).Str("type", p.Type).Msg("processing inbound message task")

	_ = h.inboundRepo.MarkProcessing(ctx, p.InboundID)

	role := "customer"
	apiKey := ""
	for _, admin := range h.adminNumbers {
		if p.SenderNumber == admin {
			role = "admin"
			apiKey = h.defaultAPIKey
			break
		}
	}

	user, err := h.userRepo.GetOrCreateByPhoneNumber(ctx, p.SenderNumber, role, apiKey)
	if err != nil {
		log.Error().Err(err).Msg("failed resolving user")
		_ = h.inboundRepo.MarkFailed(ctx, p.InboundID, err.Error())
		return err
	}

	bodyText := strings.TrimSpace(p.Body)
	if strings.HasPrefix(strings.ToLower(bodyText), "key ") {
		newKey := strings.TrimSpace(bodyText[4:])
		if newKey == "" {
			_ = h.gowaClient.SendText(ctx, h.deviceID, p.ChatID, "Format salah. Kirim: key <MT_API_KEY>", p.MessageID)
			_ = h.inboundRepo.MarkDone(ctx, p.InboundID)
			return nil
		}
		if err := h.userRepo.UpdateAPIKey(ctx, user.ID, newKey); err != nil {
			log.Error().Err(err).Msg("failed updating api key")
			_ = h.inboundRepo.MarkFailed(ctx, p.InboundID, err.Error())
			return err
		}

		log.Info().Int64("user_id", user.ID).Msg("triggering immediate cache sync for newly registered user")
		mtClient := moneytracker.NewClient(h.mtHost, newKey, 30*time.Second)
		if categories, err := mtClient.GetCategories(ctx); err == nil {
			_ = h.parserSvc.CategoryCacheRepo().Upsert(ctx, user.ID, categories)
		} else {
			log.Error().Err(err).Msg("failed fetching categories for new user")
		}
		if accounts, err := mtClient.GetAccounts(ctx); err == nil {
			_ = h.parserSvc.AccountCacheRepo().Upsert(ctx, user.ID, accounts)
		} else {
			log.Error().Err(err).Msg("failed fetching accounts for new user")
		}

		_ = h.gowaClient.SendText(ctx, h.deviceID, p.ChatID, "API Key berhasil diregister! Anda sekarang bisa mencatat transaksi.", p.MessageID)
		_ = h.inboundRepo.MarkDone(ctx, p.InboundID)
		return nil
	}

	if user.MTAPIKey == "" {
		msg := "Nomor WhatsApp Anda belum terdaftar. Silakan kirim API Key Anda terlebih dahulu dengan format:\n\n*key [MT_API_KEY]*\n\nContoh:\n*key eyJ0eXAiOiJKV1Qi...*"
		_ = h.gowaClient.SendText(ctx, h.deviceID, p.ChatID, msg, p.MessageID)
		_ = h.inboundRepo.MarkDone(ctx, p.InboundID)
		return nil
	}

	userMTClient := moneytracker.NewClient(h.mtHost, user.MTAPIKey, 30*time.Second)

	log.Debug().Str("body", p.Body).Msg("checking if text is confirmation command")
	if h.confirmationSvc.IsConfirmationCommand(p.Body) {
		log.Info().Str("command", p.Body).Msg("confirmation command detected, invoking confirmation handler")
		err := h.confirmationSvc.Handle(ctx, p.ChatID, p.Body, p.MessageID, p.CorrelationID, userMTClient)
		if err != nil {
			log.Error().Err(err).Msg("confirmation handling failed")
			_ = h.inboundRepo.MarkFailed(ctx, p.InboundID, err.Error())
			return err
		}
		log.Info().Msg("confirmation command processed successfully")
		_ = h.inboundRepo.MarkDone(ctx, p.InboundID)
		return nil
	}

	var parseErr error

	if p.Type == "image" {
		log.Info().Str("message_id", p.MessageID).Msg("image message detected, parsing image media payload")
		phone := p.SenderNumber + "@s.whatsapp.net"
		caption := p.Caption
		if caption == "" {
			caption = p.Body
		}
		result, err := h.parserSvc.ParseImage(ctx, user.ID, p.MessageID, phone, caption, userMTClient)
		if err != nil {
			log.Error().Err(err).Msg("parsing image media payload failed")
			return h.handleParseError(ctx, p, err)
		}

		missing := h.getMissingFields(result)
		if len(missing) > 0 {
			log.Info().Interface("missing_fields", missing).Msg("missing crucial transaction fields, prompt verification")
			msg := fmt.Sprintf("Transaksi belum lengkap. Mohon sertakan %s.", strings.Join(missing, ", "))
			_ = h.gowaClient.SendText(ctx, h.deviceID, p.ChatID, msg, p.MessageID)
			_ = h.inboundRepo.MarkDone(ctx, p.InboundID)
			return nil
		}

		log.Info().Msg("image parsed successfully, creating pending transaction record")
		pendingID, err := h.txSvc.CreatePending(ctx, user.ID, p.ChatID, p.MessageID, result)
		if err != nil {
			log.Error().Err(err).Msg("creating pending transaction record from image failed")
			return h.handleParseError(ctx, p, err)
		}

		log.Info().Int64("pending_id", pendingID).Msg("sending confirmation prompt to user")
		return h.sendConfirmationPrompt(ctx, p, result.Amount, result.Type, result.CategoryHint, result.AccountHint, result.Date, result.Remark, p.MessageID)
	}

	text := p.Body
	log.Info().Str("text", text).Msg("parsing text message payload")
	result, err := h.parserSvc.ParseText(ctx, user.ID, text, userMTClient)
	if err != nil {
		log.Error().Err(err).Msg("parsing text message payload failed")
		parseErr = err
	}

	if parseErr != nil {
		return h.handleParseError(ctx, p, parseErr)
	}

	log.Info().Str("intent", result.Intent).Msg("checking extracted intent result")
	if result.Intent == "help" {
		log.Info().Msg("intent help detected, sending help text")
		_ = h.gowaClient.SendText(ctx, h.deviceID, p.ChatID, helpText(), p.MessageID)
		_ = h.inboundRepo.MarkDone(ctx, p.InboundID)
		return nil
	}

	if result.Intent == "clarification" || result.Intent == "unsupported" {
		log.Info().Str("intent", result.Intent).Msg("intent clarification or unsupported detected, sending retry format prompt")
		msg := "Maaf, saya tidak mengerti. Coba format: *catat kopi 25k* atau ketik *bantuan* untuk panduan."
		_ = h.gowaClient.SendText(ctx, h.deviceID, p.ChatID, msg, p.MessageID)
		_ = h.inboundRepo.MarkDone(ctx, p.InboundID)
		return nil
	}

	missing := h.getMissingFields(result)
	if len(missing) > 0 {
		log.Info().Interface("missing_fields", missing).Msg("missing crucial transaction fields, prompt verification")
		msg := fmt.Sprintf("Transaksi belum lengkap. Mohon sertakan %s.", strings.Join(missing, ", "))
		_ = h.gowaClient.SendText(ctx, h.deviceID, p.ChatID, msg, p.MessageID)
		_ = h.inboundRepo.MarkDone(ctx, p.InboundID)
		return nil
	}

	log.Info().Msg("creating pending transaction from parsed text")
	pendingID, err := h.txSvc.CreatePending(ctx, user.ID, p.ChatID, p.MessageID, result)
	if err != nil {
		log.Error().Err(err).Msg("creating pending transaction from parsed text failed")
		return h.handleParseError(ctx, p, err)
	}

	log.Info().Int64("pending_id", pendingID).Msg("sending confirmation prompt to user")
	if err := h.sendConfirmationPrompt(ctx, p, result.Amount, result.Type, result.CategoryHint, result.AccountHint, result.Date, result.Remark, p.MessageID); err != nil {
		log.Error().Err(err).Msg("send confirmation prompt failed")
	}

	log.Info().Int64("inbound_id", p.InboundID).Msg("completed message processing task successfully")
	_ = h.inboundRepo.MarkDone(ctx, p.InboundID)
	return nil
}

func (h *ProcessMessageHandler) handleParseError(ctx context.Context, p ProcessMessagePayload, err error) error {
	log := logger.WithCorrelationID(p.CorrelationID)
	log.Warn().Err(err).Msg("error encountered during processing task, sending user notification")

	var msg string
	switch {
	case errors.Is(err, apperrors.ErrMediaTooLarge):
		msg = "Ukuran foto terlalu besar (maks 5MB). Coba kompres foto terlebih dahulu."
	case errors.Is(err, apperrors.ErrUnsupportedMessageType):
		msg = "Format foto tidak didukung. Gunakan JPEG, PNG, atau WebP."
	case errors.Is(err, apperrors.ErrUnknownCategory):
		msg = "Kategori tidak ditemukan. Coba tulis ulang dengan kata kunci yang lebih jelas (contoh: makan, transport, belanja)."
	case errors.Is(err, apperrors.ErrAIUnavailable):
		msg = "Layanan AI sedang tidak tersedia. Coba beberapa saat lagi."
	default:
		msg = "Gagal memproses pesan. Coba format: *expense | 25000 | food | kopi susu | 2026-07-03*"
	}

	_ = h.gowaClient.SendText(ctx, h.deviceID, p.ChatID, msg, p.MessageID)
	_ = h.inboundRepo.MarkFailed(ctx, p.InboundID, err.Error())
	return nil
}

func (h *ProcessMessageHandler) sendConfirmationPrompt(ctx context.Context, p ProcessMessagePayload, amountPtr *float64, txType, catHint, accHint, datePtr, remarkPtr *string, replyToID string) error {
	amount := decimal.Zero
	if amountPtr != nil {
		amount = decimal.NewFromFloat(*amountPtr)
	}

	cat := ""
	if catHint != nil {
		cat = *catHint
	}

	acc := ""
	if accHint != nil {
		acc = *accHint
	}

	date := timeutil.TodayJakarta()
	if datePtr != nil && *datePtr != "" {
		date = *datePtr
	}

	remark := ""
	if remarkPtr != nil {
		remark = *remarkPtr
	}

	txTypeLabel := "Pengeluaran"
	if txType != nil && *txType == "income" {
		txTypeLabel = "Pemasukan"
	}

	msg := fmt.Sprintf(`Saya baca transaksi berikut:

%s: %s
Kategori: %s
Akun: %s
Tanggal: %s
Catatan: %s

Balas "ya" untuk simpan atau "batal" untuk membatalkan.`,
		txTypeLabel,
		money.FormatRupiah(amount),
		cat,
		acc,
		date,
		remark,
	)

	return h.gowaClient.SendText(ctx, h.deviceID, p.ChatID, msg, replyToID)
}

func (h *ProcessMessageHandler) getMissingFields(result *ninerouter.AIExtractionResult) []string {
	var missing []string
	if result.Amount == nil || *result.Amount <= 0 {
		missing = append(missing, "*jumlah*")
	}
	if result.CategoryHint == nil || *result.CategoryHint == "" {
		missing = append(missing, "*kategori*")
	}
	if result.AccountHint == nil || *result.AccountHint == "" {
		missing = append(missing, "*akun*")
	}
	if result.Date == nil || *result.Date == "" {
		missing = append(missing, "*tanggal*")
	}
	if result.Remark == nil || *result.Remark == "" {
		missing = append(missing, "*catatan*")
	}
	return missing
}

func helpText() string {
	return `🤖 *Money Tracker Bot*

*Format input:*
• catat kopi susu 25k
• makan siang 45 ribu tadi
• transport 80k tanggal 2 juli
• income freelance 1.500.000

*Konfirmasi:*
• Balas *ya* untuk menyimpan
• Balas *batal* untuk membatalkan`
}
