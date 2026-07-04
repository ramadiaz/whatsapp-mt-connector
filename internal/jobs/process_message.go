package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/hibiken/asynq"
	"github.com/ramadiaz/money-wa-bot/internal/domain/transaction"
	"github.com/ramadiaz/money-wa-bot/internal/integration/gowa"
	"github.com/ramadiaz/money-wa-bot/internal/service"
	apperrors "github.com/ramadiaz/money-wa-bot/internal/shared/errors"
	"github.com/ramadiaz/money-wa-bot/internal/shared/logger"
	"github.com/ramadiaz/money-wa-bot/internal/shared/money"
	"github.com/ramadiaz/money-wa-bot/internal/shared/timeutil"
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
	parserSvc       *service.ParserService
	txSvc           *service.TransactionService
	confirmationSvc *service.ConfirmationService
	gowaClient      gowa.WhatsAppGateway
	deviceID        string
}

func NewProcessMessageHandler(
	inboundRepo transaction.InboundRepository,
	parserSvc *service.ParserService,
	txSvc *service.TransactionService,
	confirmationSvc *service.ConfirmationService,
	gowaClient gowa.WhatsAppGateway,
	deviceID string,
) *ProcessMessageHandler {
	return &ProcessMessageHandler{
		inboundRepo:     inboundRepo,
		parserSvc:       parserSvc,
		txSvc:           txSvc,
		confirmationSvc: confirmationSvc,
		gowaClient:      gowaClient,
		deviceID:        deviceID,
	}
}

func (h *ProcessMessageHandler) ProcessTask(ctx context.Context, t *asynq.Task) error {
	var p ProcessMessagePayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("unmarshal payload: %w", err)
	}

	log := logger.WithCorrelationID(p.CorrelationID)
	_ = h.inboundRepo.MarkProcessing(ctx, p.InboundID)

	if h.confirmationSvc.IsConfirmationCommand(p.Body) {
		err := h.confirmationSvc.Handle(ctx, p.ChatID, p.Body, p.MessageID, p.CorrelationID)
		if err != nil {
			log.Error().Err(err).Msg("confirmation handle failed")
			_ = h.inboundRepo.MarkFailed(ctx, p.InboundID, err.Error())
			return err
		}
		_ = h.inboundRepo.MarkDone(ctx, p.InboundID)
		return nil
	}

	var aiResult interface{ GetCategoryHint() *string }
	var parseErr error

	if p.Type == "image" {
		phone := p.SenderNumber + "@s.whatsapp.net"
		result, err := h.parserSvc.ParseImage(ctx, p.MessageID, phone, p.Caption)
		if err != nil {
			return h.handleParseError(ctx, p, err)
		}
		aiResult = nil
		_ = aiResult

		pendingID, err := h.txSvc.CreatePending(ctx, p.ChatID, p.MessageID, result)
		if err != nil {
			return h.handleParseError(ctx, p, err)
		}

		_ = pendingID
		return h.sendConfirmationPrompt(ctx, p, result.Amount, result.CategoryHint, result.Date, result.Remark, p.MessageID)
	}

	text := p.Body
	result, err := h.parserSvc.ParseText(ctx, text)
	if err != nil {
		parseErr = err
	}

	if parseErr != nil {
		return h.handleParseError(ctx, p, parseErr)
	}

	if result.Intent == "help" {
		_ = h.gowaClient.SendText(ctx, h.deviceID, p.ChatID, helpText(), p.MessageID)
		_ = h.inboundRepo.MarkDone(ctx, p.InboundID)
		return nil
	}

	if result.Intent == "clarification" || result.Intent == "unsupported" {
		msg := "Maaf, saya tidak mengerti. Coba format: *catat kopi 25k* atau ketik *bantuan* untuk panduan."
		_ = h.gowaClient.SendText(ctx, h.deviceID, p.ChatID, msg, p.MessageID)
		_ = h.inboundRepo.MarkDone(ctx, p.InboundID)
		return nil
	}

	if len(result.MissingFields) > 0 || result.Amount == nil {
		msg := "Transaksi belum lengkap. Mohon sertakan *jumlah*, *kategori*, dan *tanggal*."
		_ = h.gowaClient.SendText(ctx, h.deviceID, p.ChatID, msg, p.MessageID)
		_ = h.inboundRepo.MarkDone(ctx, p.InboundID)
		return nil
	}

	_, err = h.txSvc.CreatePending(ctx, p.ChatID, p.MessageID, result)
	if err != nil {
		return h.handleParseError(ctx, p, err)
	}

	if err := h.sendConfirmationPrompt(ctx, p, result.Amount, result.CategoryHint, result.Date, result.Remark, p.MessageID); err != nil {
		log.Error().Err(err).Msg("send confirmation failed")
	}

	_ = h.inboundRepo.MarkDone(ctx, p.InboundID)
	return nil
}

func (h *ProcessMessageHandler) handleParseError(ctx context.Context, p ProcessMessagePayload, err error) error {
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

func (h *ProcessMessageHandler) sendConfirmationPrompt(ctx context.Context, p ProcessMessagePayload, amountPtr *float64, catHint, datePtr, remarkPtr *string, replyToID string) error {
	amount := decimal.Zero
	if amountPtr != nil {
		amount = decimal.NewFromFloat(*amountPtr)
	}

	cat := ""
	if catHint != nil {
		cat = *catHint
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

	msg := fmt.Sprintf(`Saya baca transaksi berikut:

%s: %s
Kategori: %s
Tanggal: %s
Catatan: %s

Balas "ya" untuk simpan atau "batal" untuk membatalkan.`,
		txTypeLabel,
		money.FormatRupiah(amount),
		cat,
		date,
		remark,
	)

	return h.gowaClient.SendText(ctx, h.deviceID, p.ChatID, msg, replyToID)
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
