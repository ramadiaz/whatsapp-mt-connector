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
	InboundID     string `json:"inbound_id"`
	ChatID        string `json:"chat_id"`
	SenderNumber  string `json:"sender_number"`
	MessageID     string `json:"message_id"`
	Type          string `json:"type"`
	Body          string `json:"body"`
	Caption       string `json:"caption"`
	DeviceID      string `json:"device_id"`
	CorrelationID string `json:"correlation_id"`
	QuotedBody    string `json:"quoted_body"`
	RepliedToID   string `json:"replied_to_id"`
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
	log.Info().Str("inbound_uuid", p.InboundID).Str("type", p.Type).Msg("processing inbound message task")

	_ = h.inboundRepo.MarkProcessing(ctx, p.InboundID)
	_ = h.gowaClient.SendChatPresence(ctx, h.deviceID, p.ChatID, "start")
	defer h.gowaClient.SendChatPresence(ctx, h.deviceID, p.ChatID, "stop") //nolint:errcheck

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
			_ = h.gowaClient.SendText(ctx, h.deviceID, p.ChatID, "A-a-ano... (kenapa aku selalu gagal jelasin hal simpel) ...formatnya kayaknya kurang tepat... 😰 Coba kirim: *key <MT_API_KEY>* ...please?", p.MessageID)
			_ = h.inboundRepo.MarkDone(ctx, p.InboundID)
			return nil
		}
		if err := h.userRepo.UpdateAPIKey(ctx, user.UUID, newKey); err != nil {
			log.Error().Err(err).Msg("failed updating api key")
			_ = h.inboundRepo.MarkFailed(ctx, p.InboundID, err.Error())
			return err
		}

		log.Info().Str("user_uuid", user.UUID).Msg("triggering immediate cache sync for newly registered user")
		mtClient := moneytracker.NewClient(h.mtHost, newKey, 30*time.Second)
		if categories, err := mtClient.GetCategories(ctx); err == nil {
			_ = h.parserSvc.CategoryCacheRepo().Upsert(ctx, user.UUID, categories)
		} else {
			log.Error().Err(err).Msg("failed fetching categories for new user")
		}
		if accounts, err := mtClient.GetAccounts(ctx); err == nil {
			_ = h.parserSvc.AccountCacheRepo().Upsert(ctx, user.UUID, accounts)
		} else {
			log.Error().Err(err).Msg("failed fetching accounts for new user")
		}

		_ = h.gowaClient.SendText(ctx, h.deviceID, p.ChatID, "...b-berhasil. (aku nggak nyangka aku bisa lakuin ini) API Key-nya udah terdaftar... 🎸 Sekarang kita bisa mulai catat bareng... k-kalau kamu mau.", p.MessageID)
		_ = h.inboundRepo.MarkDone(ctx, p.InboundID)
		return nil
	}

	if user.MTAPIKey == "" {
		msg := "...a-ano... (ini canggung banget) ...nomor kamu kayaknya belum terdaftar... 😶 Um, kalau nggak keberatan... coba daftarin API Key dulu ya?\n\n*Cara dapetin API Key:*\n1. Download app Money Tracker di xann.my.id/s/money-tracker\n2. Buka *Profile → Settings → API (Developer Tools)*\n3. Copy Authorization Token-nya\n\nTerus kirim:\n*key [MT_API_KEY]*\n\nContoh:\n*key eyJ0eXAiOiJKV1Qi...*\n\n...(maaf ya ngerepotin)"
		_ = h.gowaClient.SendText(ctx, h.deviceID, p.ChatID, msg, p.MessageID)
		_ = h.inboundRepo.MarkDone(ctx, p.InboundID)
		return nil
	}

	userMTClient := moneytracker.NewClient(h.mtHost, user.MTAPIKey, 30*time.Second)

	cats, _ := h.parserSvc.CategoryCacheRepo().List(ctx, user.UUID)
	if len(cats) == 0 {
		log.Info().Str("user_uuid", user.UUID).Msg("category cache is empty, trying sync categories")
		categories, err := userMTClient.GetCategories(ctx)
		if err == nil {
			_ = h.parserSvc.CategoryCacheRepo().Upsert(ctx, user.UUID, categories)
			cats = categories
		}
		accounts, err := userMTClient.GetAccounts(ctx)
		if err == nil {
			_ = h.parserSvc.AccountCacheRepo().Upsert(ctx, user.UUID, accounts)
		}
	}

	if len(cats) == 0 {
		_ = h.gowaClient.SendText(ctx, h.deviceID, p.ChatID, "...um... (semoga ini nggak lama) ...kategorinya lagi disinkronisasi... Coba lagi dalam 10 detik ya... m-maaf ya nunggu-nungguin. 🙏", p.MessageID)
		_ = h.inboundRepo.MarkDone(ctx, p.InboundID)
		return nil
	}

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

	activePendings, _ := h.txSvc.GetActivePendings(ctx, p.ChatID)

	var isQuotedImage bool
	var quotedImageMessageID string
	var quotedImageCaption string

	if p.RepliedToID != "" {
		if rawPayload, err := h.inboundRepo.GetRawPayloadByMessageID(ctx, p.RepliedToID); err == nil && rawPayload != "" {
			var quotedPayload struct {
				MediaType string `json:"media_type"`
				Type      string `json:"type"`
				Image     any    `json:"image"`
				Caption   string `json:"caption"`
				Body      string `json:"body"`
			}
			if err := json.Unmarshal([]byte(rawPayload), &quotedPayload); err == nil {
				if quotedPayload.MediaType == "image" || quotedPayload.Image != nil || quotedPayload.Type == "image" {
					isQuotedImage = true
					quotedImageMessageID = p.RepliedToID
					quotedImageCaption = p.Body
				} else if p.QuotedBody == "" {
					if quotedPayload.Body != "" {
						p.QuotedBody = quotedPayload.Body
					} else if quotedPayload.Caption != "" {
						p.QuotedBody = quotedPayload.Caption
					}
				}
			}
		}
	}

	var parseErr error

	if p.Type == "image" || isQuotedImage {
		log.Info().Str("message_id", p.MessageID).Msg("image message detected, parsing image media payload")
		phone := p.SenderNumber + "@s.whatsapp.net"
		targetMessageID := p.MessageID
		caption := p.Caption
		if caption == "" {
			caption = p.Body
		}
		if isQuotedImage {
			targetMessageID = quotedImageMessageID
			caption = quotedImageCaption
		}
		result, err := h.parserSvc.ParseImage(ctx, user.UUID, targetMessageID, phone, caption, p.QuotedBody, activePendings, userMTClient)
		if err != nil {
			log.Error().Err(err).Msg("parsing image media payload failed")
			return h.handleParseError(ctx, p, err)
		}

		return h.handleExtractionResult(ctx, p, user, result)
	}

	text := p.Body
	log.Info().Str("text", text).Msg("parsing text message payload")
	result, err := h.parserSvc.ParseText(ctx, user.UUID, text, p.QuotedBody, activePendings, userMTClient)
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
		msg := "...a-aku... (ini memalukan tapi aku beneran nggak ngerti) ...nggak bisa baca pesannya... 😶 Coba format kayak: *catat kopi 25k* ...atau ketik *bantuan* kalau mau lihat panduannya. ...maaf ya."
		_ = h.gowaClient.SendText(ctx, h.deviceID, p.ChatID, msg, p.MessageID)
		_ = h.inboundRepo.MarkDone(ctx, p.InboundID)
		return nil
	}

	return h.handleExtractionResult(ctx, p, user, result)
}

func (h *ProcessMessageHandler) handleExtractionResult(ctx context.Context, p ProcessMessagePayload, user *postgres.User, result *ninerouter.AIExtractionResult) error {
	log := logger.WithCorrelationID(p.CorrelationID)

	type itemMissingInfo struct {
		index   int
		remark  string
		missing []string
	}

	var incomplete []itemMissingInfo
	for i, item := range result.Transactions {
		missing := h.getItemMissingFields(item)
		if len(missing) > 0 {
			remark := ""
			if item.Remark != nil {
				remark = *item.Remark
			}
			incomplete = append(incomplete, itemMissingInfo{
				index:   i + 1,
				remark:  remark,
				missing: missing,
			})
		}
	}

	if len(incomplete) > 0 {
		log.Info().Interface("incomplete_items", incomplete).Msg("missing crucial transaction fields in extracted items")
		var sb strings.Builder
		sb.WriteString("...h-hmm... (aku takut salah ngomong ini tapi) ...ada transaksi yang parameternya kurang... 😶\n\n")
		for _, inc := range incomplete {
			if inc.remark != "" {
				sb.WriteString(fmt.Sprintf("• Transaksi %d (\"%s\"): kurang %s\n", inc.index, inc.remark, strings.Join(inc.missing, ", ")))
			} else {
				sb.WriteString(fmt.Sprintf("• Transaksi %d: kurang %s\n", inc.index, strings.Join(inc.missing, ", ")))
			}
		}
		sb.WriteString("\nUm, bisa dilengkapi dulu? ...maaf ya.")
		_ = h.gowaClient.SendText(ctx, h.deviceID, p.ChatID, sb.String(), p.MessageID)
		_ = h.inboundRepo.MarkDone(ctx, p.InboundID)
		return nil
	}

	for _, item := range result.Transactions {
		_, err := h.txSvc.CreatePendingItem(ctx, user.UUID, p.ChatID, p.MessageID, &item)
		if err != nil {
			log.Error().Err(err).Msg("creating pending transaction item failed")
			return h.handleParseError(ctx, p, err)
		}
	}

	log.Info().Int("count", len(result.Transactions)).Msg("sending confirmation prompt batch to user")
	if err := h.sendConfirmationPromptBatch(ctx, p, result.Transactions, p.MessageID); err != nil {
		log.Error().Err(err).Msg("send confirmation prompt batch failed")
	}

	log.Info().Str("inbound_uuid", p.InboundID).Msg("completed message processing task successfully")
	_ = h.inboundRepo.MarkDone(ctx, p.InboundID)
	return nil
}

func (h *ProcessMessageHandler) getItemMissingFields(item ninerouter.TransactionItem) []string {
	var missing []string
	if item.Amount == nil || *item.Amount <= 0 {
		missing = append(missing, "*jumlah*")
	}
	if item.CategoryHint == nil || *item.CategoryHint == "" {
		missing = append(missing, "*kategori*")
	}
	if item.AccountHint == nil || *item.AccountHint == "" {
		missing = append(missing, "*akun*")
	}
	if item.Date == nil || *item.Date == "" {
		missing = append(missing, "*tanggal*")
	}
	if item.Remark == nil || *item.Remark == "" {
		missing = append(missing, "*catatan*")
	}
	return missing
}

func (h *ProcessMessageHandler) handleParseError(ctx context.Context, p ProcessMessagePayload, err error) error {
	log := logger.WithCorrelationID(p.CorrelationID)
	log.Warn().Err(err).Msg("error encountered during processing task, sending user notification")

	var msg string
	switch {
	case errors.Is(err, apperrors.ErrMediaTooLarge):
		msg = "...um, fotonya terlalu besar... (aku harus bilangin ini tapi nggak mau nyakitin) ...maksimal 5MB ya... Bisa dikompres dulu? ...maaf. 😔"
	case errors.Is(err, apperrors.ErrUnsupportedMessageType):
		msg = "...a-ano... (susah jelasinnya) ...format fotonya belum bisa aku baca... 😶 Coba JPEG, PNG, atau WebP ya... kalau nggak keberatan."
	case errors.Is(err, apperrors.ErrUnknownCategory):
		msg = "...h-hmm... kategorinya nggak aku temuin... (aku udah coba, beneran) ...Coba tulis lebih jelas? Contoh: makan, transport, belanja... 💦"
	case errors.Is(err, apperrors.ErrAIUnavailable):
		msg = "...AI-nya lagi... nggak bisa dihubungin... (bukan salahku, tapi aku tetep merasa bersalah) ...Coba lagi nanti ya. ...maaf. 😴"
	default:
		msg = "...g-gagal... (kenapa selalu kayak gini) ...Coba format: *expense | 25000 | food | kopi susu | 2026-07-03* ...semoga berhasil. 🥺"
	}

	_ = h.gowaClient.SendText(ctx, h.deviceID, p.ChatID, msg, p.MessageID)
	_ = h.inboundRepo.MarkFailed(ctx, p.InboundID, err.Error())
	return nil
}

func (h *ProcessMessageHandler) sendConfirmationPrompt(ctx context.Context, p ProcessMessagePayload, item *ninerouter.TransactionItem, replyToID string) error {
	amount := decimal.Zero
	if item.Amount != nil {
		amount = decimal.NewFromFloat(*item.Amount)
	}

	cat := ""
	if item.CategoryHint != nil {
		cat = *item.CategoryHint
	}

	acc := ""
	if item.AccountHint != nil {
		acc = *item.AccountHint
	}

	date := timeutil.TodayJakarta()
	if item.Date != nil && *item.Date != "" {
		date = *item.Date
	}

	remark := ""
	if item.Remark != nil {
		remark = *item.Remark
	}

	txTypeLabel := "Pengeluaran"
	isExpense := item.Type == nil || *item.Type == "expense"
	if item.Type != nil && *item.Type == "income" {
		txTypeLabel = "Pemasukan"
		isExpense = false
	}

	if isExpense && item.IsWasteful != nil && *item.IsWasteful {
		log := logger.WithCorrelationID(p.CorrelationID)
		reason := ""
		if item.WastefulReason != nil {
			reason = *item.WastefulReason
		}
		log.Info().Str("reason", reason).Msg("wasteful spending detected, sending warning bubble")
		warningMsg := fmt.Sprintf("...a-ano... (ini agak canggung buat aku bilang tapi) ...kayaknya transaksi ini masuk kategori boros deh... 😶\n\n_%s_\n\n...(aku cuma mau ngingetin aja. Keuanganmu penting. ...maaf kalau lancang)", reason)
		_ = h.gowaClient.SendText(ctx, h.deviceID, p.ChatID, warningMsg, replyToID)
	}

	msg := fmt.Sprintf(`...o-oh, aku nangkep transaksinya... (semoga bener) 🎸

%s: %s
Kategori: %s
Akun: %s
Tanggal: %s
Catatan: %s

...um, balas *ya* kalau mau disimpan... atau *batal* kalau nggak jadi. ...aku tunggu. 🙏`,

		txTypeLabel,
		money.FormatRupiah(amount),
		cat,
		acc,
		date,
		remark,
	)

	return h.gowaClient.SendText(ctx, h.deviceID, p.ChatID, msg, replyToID)
}

func (h *ProcessMessageHandler) sendConfirmationPromptBatch(ctx context.Context, p ProcessMessagePayload, items []ninerouter.TransactionItem, replyToID string) error {
	for _, item := range items {
		isExpense := item.Type == nil || *item.Type == "expense"
		if isExpense && item.IsWasteful != nil && *item.IsWasteful {
			reason := ""
			if item.WastefulReason != nil {
				reason = *item.WastefulReason
			}
			warningMsg := fmt.Sprintf("...a-ano... (ini agak canggung buat aku bilang tapi) ...kayaknya transaksi ini masuk kategori boros deh... 😶\n\n_%s_\n\n...(aku cuma mau ngingetin aja. Keuanganmu penting. ...maaf kalau lancang)", reason)
			_ = h.gowaClient.SendText(ctx, h.deviceID, p.ChatID, warningMsg, replyToID)
		}
	}

	if len(items) == 1 {
		return h.sendConfirmationPrompt(ctx, p, &items[0], replyToID)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("...o-oh, aku nangkep %d transaksi... (semoga bener) 🎸\n\n", len(items)))
	for i, item := range items {
		amount := decimal.Zero
		if item.Amount != nil {
			amount = decimal.NewFromFloat(*item.Amount)
		}
		cat := ""
		if item.CategoryHint != nil {
			cat = *item.CategoryHint
		}
		acc := ""
		if item.AccountHint != nil {
			acc = *item.AccountHint
		}
		date := timeutil.TodayJakarta()
		if item.Date != nil && *item.Date != "" {
			date = *item.Date
		}
		remark := ""
		if item.Remark != nil {
			remark = *item.Remark
		}
		txTypeLabel := "Pengeluaran"
		if item.Type != nil && *item.Type == "income" {
			txTypeLabel = "Pemasukan"
		}

		sb.WriteString(fmt.Sprintf("%d. %s: %s | Catatan: %s | Kategori: %s | Akun: %s | Tanggal: %s\n",
			i+1,
			txTypeLabel,
			money.FormatRupiah(amount),
			remark,
			cat,
			acc,
			date,
		))
	}
	sb.WriteString("\n...um, balas *ya* kalau mau disimpan... atau *batal* kalau nggak jadi. ...aku tunggu. 🙏")

	return h.gowaClient.SendText(ctx, h.deviceID, p.ChatID, sb.String(), replyToID)
}

func helpText() string {
	return `...a-ano... (aku nggak biasa ngomong depan orang tapi ini penting) ...aku Money Tracker Bot... 🎸

(Deep breath) ...ini cara pakainya:

*Format input:*
• catat kopi susu 25k
• makan siang 45 ribu tadi
• transport 80k tanggal 2 juli
• income freelance 1.500.000

*Konfirmasi:*
• Balas *ya* untuk menyimpan
• Balas *batal* untuk membatalkan

...s-semoga membantu. (aku nggak tau gimana caranya terlihat lebih ramah dari ini)`
}
