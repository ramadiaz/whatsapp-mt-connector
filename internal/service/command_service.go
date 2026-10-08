package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"image"
	_ "image/jpeg"
	_ "image/png"

	"github.com/fogleman/gg"
	"github.com/golang/freetype/truetype"
	"github.com/ramadiaz/whatsapp-mt-connector/internal/assets"
	"github.com/skip2/go-qrcode"

	"github.com/ramadiaz/whatsapp-mt-connector/internal/domain/blacklist"
	gowaintegration "github.com/ramadiaz/whatsapp-mt-connector/internal/integration/gowa"
	"github.com/ramadiaz/whatsapp-mt-connector/internal/integration/ninerouter"
	"github.com/ramadiaz/whatsapp-mt-connector/internal/persistence/postgres"
	"github.com/ramadiaz/whatsapp-mt-connector/internal/shared/logger"
	"gorm.io/gorm"
)

type ParsedCommand struct {
	Name  string
	Args  []string
	Flags map[string]string
}

type CommandService struct {
	adminNumbers   []string
	blacklistRepo  blacklist.Repository
	userRepo       *postgres.UserRepository
	db             *gorm.DB
	gowaClient     gowaintegration.WhatsAppGateway
	deviceID       string
	gopayClientURL string
	httpClient     *http.Client
	nineRouter     *ninerouter.Client
}

func NewCommandService(
	adminNumbers []string,
	blacklistRepo blacklist.Repository,
	userRepo *postgres.UserRepository,
	db *gorm.DB,
	gowaClient gowaintegration.WhatsAppGateway,
	deviceID string,
	gopayClientURL string,
	nineRouter *ninerouter.Client,
) *CommandService {
	if gopayClientURL == "" {
		gopayClientURL = "http://localhost:8085"
	}
	return &CommandService{
		adminNumbers:   adminNumbers,
		blacklistRepo:  blacklistRepo,
		userRepo:       userRepo,
		db:             db,
		gowaClient:     gowaClient,
		deviceID:       deviceID,
		gopayClientURL: strings.TrimRight(gopayClientURL, "/"),
		httpClient:     &http.Client{Timeout: 30 * time.Second},
		nineRouter:     nineRouter,
	}
}

func (s *CommandService) IsAdmin(senderNumber string) bool {
	for _, admin := range s.adminNumbers {
		if senderNumber == admin {
			return true
		}
	}
	return false
}

func ParseCommandString(input string) *ParsedCommand {
	input = strings.TrimSpace(input)
	if !strings.HasPrefix(input, "/") {
		return nil
	}
	rawTokens := strings.Fields(input[1:])
	if len(rawTokens) == 0 {
		return nil
	}

	cmdName := strings.ToLower(rawTokens[0])
	args := make([]string, 0)
	flags := make(map[string]string)

	for i := 1; i < len(rawTokens); i++ {
		token := rawTokens[i]
		if strings.HasPrefix(token, "--") {
			parts := strings.SplitN(token[2:], "=", 2)
			if len(parts) == 2 {
				flags[strings.ToLower(parts[0])] = parts[1]
			} else if i+1 < len(rawTokens) && !strings.HasPrefix(rawTokens[i+1], "-") {
				flags[strings.ToLower(parts[0])] = rawTokens[i+1]
				i++
			} else {
				flags[strings.ToLower(parts[0])] = "true"
			}
		} else if strings.HasPrefix(token, "-") {
			parts := strings.SplitN(token[1:], "=", 2)
			if len(parts) == 2 {
				flags[strings.ToLower(parts[0])] = parts[1]
			} else if i+1 < len(rawTokens) && !strings.HasPrefix(rawTokens[i+1], "-") {
				flags[strings.ToLower(parts[0])] = rawTokens[i+1]
				i++
			} else {
				flags[strings.ToLower(parts[0])] = "true"
			}
		} else {
			args = append(args, token)
		}
	}

	return &ParsedCommand{
		Name:  cmdName,
		Args:  args,
		Flags: flags,
	}
}

func (c *ParsedCommand) HasHelpFlag() bool {
	if c == nil {
		return false
	}
	_, hasHelp := c.Flags["help"]
	_, hasH := c.Flags["h"]
	return hasHelp || hasH
}

func (s *CommandService) HandleCommand(ctx context.Context, senderNumber, chatID, body, messageID string) error {
	if !s.IsAdmin(senderNumber) {
		msg := "Akses ditolak. Command hanya bisa dijalankan oleh nomor admin."
		return s.gowaClient.SendText(ctx, s.deviceID, chatID, msg, messageID)
	}

	cmd := ParseCommandString(body)
	if cmd == nil {
		msg := "Format command tidak valid."
		return s.gowaClient.SendText(ctx, s.deviceID, chatID, msg, messageID)
	}

	switch cmd.Name {
	case "blacklist":
		return s.handleBlacklistCommand(ctx, senderNumber, chatID, cmd, messageID)
	case "users", "user":
		return s.handleUsersCommand(ctx, chatID, cmd, messageID)
	case "stats", "status":
		return s.handleStatsCommand(ctx, chatID, cmd, messageID)
	case "payment", "pay", "tagih":
		return s.handlePaymentCommand(ctx, senderNumber, chatID, body, cmd, messageID)
	case "contacts", "contact", "kontak":
		return s.handleContactsCommand(ctx, senderNumber, chatID, cmd, messageID)
	case "help":
		return s.handleHelpCommand(ctx, chatID, cmd, messageID)
	default:
		msg := fmt.Sprintf("Command `/%s` tidak ditemukan. Gunakan `/help` untuk melihat daftar command.", cmd.Name)
		return s.gowaClient.SendText(ctx, s.deviceID, chatID, msg, messageID)
	}
}

func (s *CommandService) handleHelpCommand(ctx context.Context, chatID string, cmd *ParsedCommand, messageID string) error {
	msg := "Daftar Command:\n- `/payment` : Tagih utang via QRIS dinamis GoPay Merchant (`/payment tagih 50rb ke 62813xxxxx a/n Rama, bayar kopi`)\n- `/contacts` : Kelola kontak tersimpan (`/contacts` / `/contacts delete <nama>`)\n- `/cal` : Hitung kalori dari teks / gambar makanan\n- `/blacklist` : Kelola nomor diblokir (`add`, `remove`, `list`)\n- `/users` : Lihat daftar pengguna terdaftar\n- `/stats` : Lihat statistik & status sistem\n- `/help` : Tampilkan bantuan\n\nTips: Tambahkan `--help` atau `-h` pada command untuk melihat opsi."
	return s.gowaClient.SendText(ctx, s.deviceID, chatID, msg, messageID)
}

func (s *CommandService) handleUsersCommand(ctx context.Context, chatID string, cmd *ParsedCommand, messageID string) error {
	if cmd.HasHelpFlag() {
		msg := "Penggunaan command users:\n- `/users` : Tampilkan daftar semua pengguna terdaftar\n\nFlag:\n  --help, -h : Tampilkan pesan bantuan"
		return s.gowaClient.SendText(ctx, s.deviceID, chatID, msg, messageID)
	}

	users, err := s.userRepo.ListAll(ctx)
	if err != nil {
		return s.gowaClient.SendText(ctx, s.deviceID, chatID, "Gagal mengambil daftar pengguna.", messageID)
	}
	if len(users) == 0 {
		return s.gowaClient.SendText(ctx, s.deviceID, chatID, "Belum ada pengguna terdaftar.", messageID)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Daftar Pengguna (%d):\n", len(users)))
	for idx, u := range users {
		keyStatus := "terdaftar"
		if u.MTAPIKey == "" {
			keyStatus = "belum set API key"
		}
		sb.WriteString(fmt.Sprintf("%d. %s [%s] (%s)\n", idx+1, u.PhoneNumber, u.Role, keyStatus))
	}
	return s.gowaClient.SendText(ctx, s.deviceID, chatID, sb.String(), messageID)
}

func (s *CommandService) handleStatsCommand(ctx context.Context, chatID string, cmd *ParsedCommand, messageID string) error {
	if cmd.HasHelpFlag() {
		msg := "Penggunaan command stats:\n- `/stats` : Tampilkan ringkasan statistik sistem\n\nFlag:\n  --help, -h : Tampilkan pesan bantuan"
		return s.gowaClient.SendText(ctx, s.deviceID, chatID, msg, messageID)
	}

	var userCount int64
	var blacklistedCount int64
	var inboundCount int64
	var pendingTxCount int64

	_ = s.db.WithContext(ctx).Model(&postgres.User{}).Count(&userCount).Error
	_ = s.db.WithContext(ctx).Model(&postgres.Blacklist{}).Count(&blacklistedCount).Error
	_ = s.db.WithContext(ctx).Model(&postgres.InboundMessage{}).Count(&inboundCount).Error
	_ = s.db.WithContext(ctx).Model(&postgres.PendingTransaction{}).Where("status = ?", "pending").Count(&pendingTxCount).Error

	msg := fmt.Sprintf("Statistik Sistem:\n- Total User: %d\n- Total Blacklist: %d\n- Inbound Messages: %d\n- Pending Transactions: %d", userCount, blacklistedCount, inboundCount, pendingTxCount)
	return s.gowaClient.SendText(ctx, s.deviceID, chatID, msg, messageID)
}

func (s *CommandService) handleBlacklistCommand(ctx context.Context, senderNumber, chatID string, cmd *ParsedCommand, messageID string) error {
	if cmd.HasHelpFlag() || len(cmd.Args) == 0 {
		msg := "Penggunaan command blacklist:\n- `/blacklist add <nomor> [--reason=...]`\n- `/blacklist remove <nomor>`\n- `/blacklist list`\n\nFlag:\n  --help, -h : Tampilkan pesan bantuan\n  --reason : Alasan pemblokiran"
		return s.gowaClient.SendText(ctx, s.deviceID, chatID, msg, messageID)
	}

	subCmd := strings.ToLower(cmd.Args[0])
	reason := cmd.Flags["reason"]

	switch subCmd {
	case "list":
		list, err := s.blacklistRepo.List(ctx)
		if err != nil {
			return s.gowaClient.SendText(ctx, s.deviceID, chatID, "Gagal mengambil daftar blacklist.", messageID)
		}
		if len(list) == 0 {
			return s.gowaClient.SendText(ctx, s.deviceID, chatID, "Daftar blacklist kosong.", messageID)
		}
		var sb strings.Builder
		sb.WriteString("Daftar Nomor Blacklist:\n")
		for idx, num := range list {
			sb.WriteString(fmt.Sprintf("%d. %s\n", idx+1, num))
		}
		return s.gowaClient.SendText(ctx, s.deviceID, chatID, sb.String(), messageID)

	case "remove", "delete", "del", "unblacklist":
		if len(cmd.Args) < 2 {
			return s.gowaClient.SendText(ctx, s.deviceID, chatID, "Sebutkan nomor yang ingin dihapus dari blacklist.", messageID)
		}
		targetNum := cmd.Args[1]
		err := s.blacklistRepo.Remove(ctx, targetNum)
		if err != nil {
			return s.gowaClient.SendText(ctx, s.deviceID, chatID, "Gagal menghapus nomor dari blacklist.", messageID)
		}
		return s.gowaClient.SendText(ctx, s.deviceID, chatID, fmt.Sprintf("Nomor %s berhasil dihapus dari blacklist.", targetNum), messageID)

	case "add":
		if len(cmd.Args) < 2 {
			return s.gowaClient.SendText(ctx, s.deviceID, chatID, "Sebutkan nomor yang ingin diblacklist.", messageID)
		}
		targetNum := cmd.Args[1]
		err := s.blacklistRepo.Add(ctx, targetNum, reason, senderNumber)
		if err != nil {
			return s.gowaClient.SendText(ctx, s.deviceID, chatID, "Gagal menambahkan nomor ke blacklist.", messageID)
		}
		return s.gowaClient.SendText(ctx, s.deviceID, chatID, fmt.Sprintf("Nomor %s berhasil ditambahkan ke blacklist.", targetNum), messageID)

	default:
		targetNum := cmd.Args[0]
		err := s.blacklistRepo.Add(ctx, targetNum, reason, senderNumber)
		if err != nil {
			return s.gowaClient.SendText(ctx, s.deviceID, chatID, "Gagal menambahkan nomor ke blacklist.", messageID)
		}
		return s.gowaClient.SendText(ctx, s.deviceID, chatID, fmt.Sprintf("Nomor %s berhasil ditambahkan ke blacklist.", targetNum), messageID)
	}
}

// Payment Command Handler
func (s *CommandService) handlePaymentCommand(ctx context.Context, senderNumber, chatID, rawBody string, cmd *ParsedCommand, messageID string) error {
	if cmd.HasHelpFlag() || (len(cmd.Args) > 0 && (cmd.Args[0] == "help" || cmd.Args[0] == "h")) {
		msg := "Penggunaan command payment:\n" +
			"• `/payment tagih <nominal> ke <nomor> <keterangan>`\n" +
			"  Contoh: `/payment tagih 50rb ke 6281312345678 bayar kopi`\n" +
			"  Variasi nominal: `50rb`, `50k`, `50.000`, `1.5jt`\n" +
			"• `/payment status <order_id>` : Cek status pembayaran tagihan\n\n" +
			"Bot akan membuat QRIS Dinamis resmi dari GoPay Merchant, mengirimkan gambar QRIS ke target, dan mengonfirmasi otomatis saat lunas."
		return s.gowaClient.SendText(ctx, s.deviceID, chatID, msg, messageID)
	}

	// Subcommand: status
	if len(cmd.Args) >= 2 && strings.ToLower(cmd.Args[0]) == "status" {
		orderID := cmd.Args[1]
		return s.checkPaymentStatus(ctx, chatID, orderID, messageID)
	}

	// Parse billing parameters using 9Router AI with rule-based fallback
	req, err := s.parsePaymentBillingWithAI(ctx, rawBody)
	if err != nil {
		msg := fmt.Sprintf("⚠️ Format tagihan salah: %v\n\nContoh yang benar:\n`/payment tagih 50rb ke 62813xxxxx a/n Rama, bayar kopi`\natau jika kontak sudah tersimpan:\n`/payment tagih 50rb ke rama, bayar kopi`", err)
		return s.gowaClient.SendText(ctx, s.deviceID, chatID, msg, messageID)
	}

	var targetPhone string
	var recipientName string

	if req.TargetPhone != "" {
		cleanPhone := cleanPhoneNumber(req.TargetPhone)
		var existingContact postgres.Contact
		err := s.db.WithContext(ctx).Where("phone_number = ?", cleanPhone).First(&existingContact).Error
		if err == nil {
			targetPhone = cleanPhone
			recipientName = existingContact.Name
			if req.TargetName != "" && req.TargetName != existingContact.Name {
				existingContact.Name = req.TargetName
				existingContact.NormalizedName = strings.ToLower(strings.TrimSpace(req.TargetName))
				_ = s.db.WithContext(ctx).Save(&existingContact)
				recipientName = req.TargetName
			}
		} else {
			if req.TargetName != "" {
				newContact := postgres.Contact{
					Name:           req.TargetName,
					NormalizedName: strings.ToLower(strings.TrimSpace(req.TargetName)),
					PhoneNumber:    cleanPhone,
					CreatedBy:      senderNumber,
				}
				_ = s.db.WithContext(ctx).Create(&newContact)
				targetPhone = cleanPhone
				recipientName = req.TargetName
			} else {
				msg := fmt.Sprintf(
					"⚠️ Nomor *%s* belum terdaftar di kontak.\n\n"+
						"Silakan sertakan nama tujuan agar nomor tersimpan otomatis ke database:\n"+
						"`/payment tagih %s ke %s a/n <Nama Tujuan>, %s`\n\n"+
						"Contoh:\n"+
						"`/payment tagih %s ke %s a/n Rama, %s`",
					cleanPhone,
					formatRupiah(req.Amount), cleanPhone, req.Description,
					formatRupiah(req.Amount), cleanPhone, req.Description,
				)
				return s.gowaClient.SendText(ctx, s.deviceID, chatID, msg, messageID)
			}
		}
	} else if req.TargetName != "" {
		normName := strings.ToLower(strings.TrimSpace(req.TargetName))
		var existingContact postgres.Contact
		err := s.db.WithContext(ctx).Where("normalized_name = ? OR LOWER(name) = ?", normName, normName).First(&existingContact).Error
		if err != nil {
			err = s.db.WithContext(ctx).Where("normalized_name LIKE ?", normName+"%").First(&existingContact).Error
		}
		if err == nil {
			targetPhone = existingContact.PhoneNumber
			recipientName = existingContact.Name
		} else {
			msg := fmt.Sprintf(
				"⚠️ Kontak *%s* belum terdaftar di database.\n\n"+
					"Untuk transaksi pertama kali, gunakan nomor telepon dan sertakan nama tujuan:\n"+
					"`/payment tagih %s ke <nomor_hp> a/n %s, %s`\n\n"+
					"Contoh:\n"+
					"`/payment tagih %s ke 08123456789 a/n %s, %s`",
				req.TargetName,
				formatRupiah(req.Amount), req.TargetName, req.Description,
				formatRupiah(req.Amount), req.TargetName, req.Description,
			)
			return s.gowaClient.SendText(ctx, s.deviceID, chatID, msg, messageID)
		}
	} else {
		msg := "⚠️ Nomor tujuan atau nama kontak belum ditentukan."
		return s.gowaClient.SendText(ctx, s.deviceID, chatID, msg, messageID)
	}

	// Call GoPay Client API
	endpoint := fmt.Sprintf("%s/api/v1/bills", s.gopayClientURL)
	payload := map[string]any{
		"amount":         req.Amount,
		"description":    req.Description,
		"payer_phone":    targetPhone,
		"admin_phone":    senderNumber,
		"expiry_minutes": 360,
	}

	b, err := json.Marshal(payload)
	if err != nil {
		return s.gowaClient.SendText(ctx, s.deviceID, chatID, "Gagal memproses data tagihan.", messageID)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(b))
	if err != nil {
		return s.gowaClient.SendText(ctx, s.deviceID, chatID, "Gagal membuat request ke service GoPay.", messageID)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		msg := fmt.Sprintf("❌ Gagal terhubung ke GoPay Client Service: %v\nPastikan gopay-client sedang berjalan.", err)
		return s.gowaClient.SendText(ctx, s.deviceID, chatID, msg, messageID)
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		msg := fmt.Sprintf("❌ Gagal membuat tagihan GoPay (HTTP %d): %s", resp.StatusCode, string(respBytes))
		return s.gowaClient.SendText(ctx, s.deviceID, chatID, msg, messageID)
	}

	var apiResp struct {
		Success bool `json:"success"`
		Data    struct {
			ID          string  `json:"id"`
			OrderID     string  `json:"order_id"`
			Amount      float64 `json:"amount"`
			Description string  `json:"description"`
			Status      string  `json:"status"`
			QRISString  string  `json:"qris_string"`
			ExpiresAt   string  `json:"expires_at"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBytes, &apiResp); err != nil {
		return s.gowaClient.SendText(ctx, s.deviceID, chatID, "Gagal membaca respons dari GoPay Client.", messageID)
	}

	displayRecipient := targetPhone
	if recipientName != "" {
		displayRecipient = fmt.Sprintf("%s (%s)", recipientName, targetPhone)
	}

	successMsg := fmt.Sprintf(
		"🧾 *TAGIHAN QRIS BERHASIL DIBUAT*\n\n"+
			"• *Penerima:* %s\n"+
			"• *Nominal:* Rp %s\n"+
			"• *Keperluan:* %s\n"+
			"• *Order ID:* `%s`\n"+
			"• *Status:* Menunggu Pembayaran (6 jam)\n\n"+
			"Kode QRIS Dinamis dan rincian invoice telah dikirimkan ke WhatsApp %s.\n"+
			"_Sistem akan otomatis memberi tahu Anda begitu tagihan lunas._",
		displayRecipient,
		formatRupiah(req.Amount),
		req.Description,
		apiResp.Data.OrderID,
		displayRecipient,
	)

	// SEND THE ACTUAL QRIS TO TARGET
	go func() {
		targetChatID := targetPhone + "@s.whatsapp.net"
		greeting := "Halo!"
		if recipientName != "" {
			greeting = fmt.Sprintf("Halo *%s*!", recipientName)
		}
		targetMsg := fmt.Sprintf(
			"%s Anda menerima tagihan baru dari *GoPay Merchant*\n\n" +
			"• *Nominal:* Rp %s\n" +
			"• *Keperluan:* %s\n" +
			"• *Order ID:* %s\n\n" +
			"Silakan scan kode QRIS di bawah ini menggunakan aplikasi M-Banking atau E-Wallet Anda. Tagihan ini akan otomatis kedaluwarsa dalam 6 jam.",
			greeting,
			formatRupiah(req.Amount), req.Description, apiResp.Data.OrderID,
		)
		
		// Generate QRIS with official template natively
		expiryTime := time.Now().Add(6 * time.Hour)
		if apiResp.Data.ExpiresAt != "" {
			if t, err := time.Parse(time.RFC3339, apiResp.Data.ExpiresAt); err == nil {
				expiryTime = t
			}
		}
		finalBytes, err := s.generateCompositeQRIS(apiResp.Data.QRISString, "SanySoft", formatRupiah(req.Amount), expiryTime)
		if err == nil {
			// Send Image
			_ = s.gowaClient.SendImage(context.Background(), s.deviceID, targetChatID, targetMsg, finalBytes, "qris.png", "")
		} else {
			_ = s.gowaClient.SendText(context.Background(), s.deviceID, targetChatID, targetMsg + "\n\n(Gambar QRIS gagal dimuat secara internal, silakan hubungi admin)", "")
		}
	}()

	return s.gowaClient.SendText(ctx, s.deviceID, chatID, successMsg, messageID)
}

func (s *CommandService) checkPaymentStatus(ctx context.Context, chatID, orderID, messageID string) error {
	endpoint := fmt.Sprintf("%s/api/v1/bills/%s", s.gopayClientURL, orderID)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return s.gowaClient.SendText(ctx, s.deviceID, chatID, "Gagal membuat request status.", messageID)
	}

	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		return s.gowaClient.SendText(ctx, s.deviceID, chatID, fmt.Sprintf("Gagal cek status: %v", err), messageID)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return s.gowaClient.SendText(ctx, s.deviceID, chatID, fmt.Sprintf("Tagihan dengan Order ID `%s` tidak ditemukan.", orderID), messageID)
	}

	var apiResp struct {
		Data struct {
			OrderID     string  `json:"order_id"`
			Amount      float64 `json:"amount"`
			Description string  `json:"description"`
			PayerPhone  string  `json:"payer_phone"`
			Status      string  `json:"status"`
			ExpiresAt   string  `json:"expires_at"`
		} `json:"data"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&apiResp)

	statusIndo := apiResp.Data.Status
	switch apiResp.Data.Status {
	case "settlement":
		statusIndo = "✅ Lunas (Settlement)"
	case "pending":
		statusIndo = "⏳ Menunggu Pembayaran"
	case "expired":
		statusIndo = "❌ Kedaluwarsa"
	}

	msg := fmt.Sprintf(
		"📋 *STATUS TAGIHAN*\n\n"+
			"• *Order ID:* `%s`\n"+
			"• *Target:* %s\n"+
			"• *Nominal:* Rp %s\n"+
			"• *Keperluan:* %s\n"+
			"• *Status:* %s",
		apiResp.Data.OrderID,
		apiResp.Data.PayerPhone,
		formatRupiah(apiResp.Data.Amount),
		apiResp.Data.Description,
		statusIndo,
	)
	return s.gowaClient.SendText(ctx, s.deviceID, chatID, msg, messageID)
}

type PaymentBillingInput struct {
	Amount      float64 `json:"amount"`
	TargetPhone string  `json:"target_phone"`
	TargetName  string  `json:"target_name"`
	Description string  `json:"description"`
}

const paymentBillingSystemPrompt = `You are an AI payment billing parser for an Indonesian WhatsApp merchant bot.
Your job is to parse a billing / invoice command from the user into JSON.
Examples of user commands:
- "/payment tagih 5rb ke 62812345678 a/n Rama, patungan sesuatu"
- "/payment tagih 50rb ke 0812345678 a.n. Rama Diaz, makan siang"
- "/payment tagih 10k ke 62812345678 an Budi, kas kantor"
- "/payment tagih 5rb ke 62812345678, patungan sesuatu"
- "/payment tagih 5rb ke rama, patungan sesuatu"
- "/payment tagih 15rb ke Budi Santoso, makan siang"
- "/payment tagih 50rb ke 6281312345678 bayar kopi"

Output must be strict valid JSON only, without markdown fences or additional text:
{
  "amount": <number in IDR, e.g. 5000, 50000>,
  "target_phone": "<phone number starting with 628 without +, spaces, or dashes, or empty string if no phone number was given>",
  "target_name": "<name of the recipient if given via a/n, atas nama, or directly after ke, otherwise empty string>",
  "description": "<purpose/remark of the bill, default to 'Tagihan Pembayaran' if omitted>"
}`

func (s *CommandService) parsePaymentBillingWithAI(ctx context.Context, rawInput string) (*PaymentBillingInput, error) {
	if s.nineRouter != nil {
		messages := []ninerouter.Message{
			{
				Role:    "user",
				Content: rawInput,
			},
		}

		logger.Log.Info().Str("input", rawInput).Msg("calling 9Router AI for payment billing parsing")
		rawJSON, err := s.nineRouter.Complete(ctx, s.nineRouter.Model(), paymentBillingSystemPrompt, messages, 300)
		if err == nil {
			cleaned := strings.TrimSpace(rawJSON)
			if strings.HasPrefix(cleaned, "```") {
				lines := strings.Split(cleaned, "\n")
				var inner []string
				for _, line := range lines {
					if strings.HasPrefix(line, "```") {
						continue
					}
					inner = append(inner, line)
				}
				cleaned = strings.Join(inner, "\n")
			}
			cleaned = strings.TrimSpace(cleaned)

			var aiResult PaymentBillingInput
			if err := json.Unmarshal([]byte(cleaned), &aiResult); err == nil && aiResult.Amount > 0 {
				if aiResult.TargetPhone != "" {
					aiResult.TargetPhone = cleanPhoneNumber(aiResult.TargetPhone)
				}
				aiResult.TargetName = strings.TrimSpace(aiResult.TargetName)
				aiResult.Description = strings.TrimSpace(aiResult.Description)
				if aiResult.Description == "" {
					aiResult.Description = "Tagihan Pembayaran"
				}
				logger.Log.Info().Interface("ai_result", aiResult).Msg("9Router AI billing parsing succeeded")
				return &aiResult, nil
			} else if err != nil {
				logger.Log.Warn().Err(err).Str("raw_json", cleaned).Msg("failed to unmarshal 9Router AI billing response")
			}
		} else {
			logger.Log.Warn().Err(err).Msg("9Router AI billing completion failed, falling back to rule-based parser")
		}
	}

	return parsePaymentBilling(rawInput)
}

func parsePaymentBilling(rawInput string) (*PaymentBillingInput, error) {
	rawInput = strings.TrimSpace(rawInput)
	idx := strings.Index(rawInput, " ")
	if idx == -1 {
		return nil, fmt.Errorf("parameter tagihan tidak lengkap")
	}
	content := strings.TrimSpace(rawInput[idx:])
	tokens := strings.Fields(content)
	if len(tokens) == 0 {
		return nil, fmt.Errorf("parameter tagihan kosong")
	}

	if strings.ToLower(tokens[0]) == "tagih" {
		tokens = tokens[1:]
	}
	if len(tokens) == 0 {
		return nil, fmt.Errorf("nominal dan nomor tujuan belum ditentukan")
	}

	var amount float64
	var targetPhone string
	var targetName string

	var remainingTokens []string
	for _, token := range tokens {
		if amount == 0 {
			if parsed, ok := parseCurrency(token); ok && parsed > 0 {
				amount = parsed
				continue
			}
		}
		remainingTokens = append(remainingTokens, token)
	}

	if amount <= 0 {
		return nil, fmt.Errorf("nominal tagihan tidak valid")
	}
	if len(remainingTokens) == 0 {
		return nil, fmt.Errorf("nomor tujuan atau nama kontak belum ditentukan")
	}

	remStr := strings.Join(remainingTokens, " ")

	var targetClause string
	var descClause string
	hasComma := false
	if commaIdx := strings.Index(remStr, ","); commaIdx != -1 {
		targetClause = strings.TrimSpace(remStr[:commaIdx])
		descClause = strings.TrimSpace(remStr[commaIdx+1:])
		hasComma = true
	} else {
		targetClause = remStr
	}

	targetTokens := strings.Fields(targetClause)

	anIndex := -1
	anLen := 1
	for idx, tok := range targetTokens {
		cleanTok := strings.Trim(strings.ToLower(tok), ".,")
		if cleanTok == "a/n" || cleanTok == "a.n" || cleanTok == "an" {
			anIndex = idx
			anLen = 1
			break
		}
		if cleanTok == "atas" && idx+1 < len(targetTokens) && strings.Trim(strings.ToLower(targetTokens[idx+1]), ".,") == "nama" {
			anIndex = idx
			anLen = 2
			break
		}
	}

	if anIndex != -1 {
		nameTokens := targetTokens[anIndex+anLen:]
		if hasComma {
			targetName = strings.Trim(strings.Join(nameTokens, " "), ",. ")
		} else {
			if len(nameTokens) > 0 {
				targetName = strings.Trim(nameTokens[0], ",. ")
				if descClause == "" && len(nameTokens) > 1 {
					descClause = strings.Join(nameTokens[1:], " ")
				}
			}
		}

		beforeTokens := targetTokens[:anIndex]
		for idx, tok := range beforeTokens {
			if strings.ToLower(tok) == "ke" && idx+1 < len(beforeTokens) {
				candidate := beforeTokens[idx+1]
				if isPhoneNumber(candidate) {
					targetPhone = cleanPhoneNumber(candidate)
				}
				continue
			}
			if targetPhone == "" && isPhoneNumber(tok) {
				targetPhone = cleanPhoneNumber(tok)
			}
		}
	} else {
		keIndex := -1
		for idx, tok := range targetTokens {
			if strings.ToLower(tok) == "ke" {
				keIndex = idx
				break
			}
		}

		if keIndex != -1 && keIndex+1 < len(targetTokens) {
			candidate := targetTokens[keIndex+1]
			if isPhoneNumber(candidate) {
				targetPhone = cleanPhoneNumber(candidate)
				if descClause == "" && len(targetTokens) > keIndex+2 {
					descClause = strings.Join(targetTokens[keIndex+2:], " ")
				}
			} else {
				if hasComma {
					targetName = strings.Trim(strings.Join(targetTokens[keIndex+1:], " "), ",. ")
				} else {
					targetName = candidate
					if descClause == "" && len(targetTokens) > keIndex+2 {
						descClause = strings.Join(targetTokens[keIndex+2:], " ")
					}
				}
			}
		} else {
			for idx, tok := range targetTokens {
				if isPhoneNumber(tok) {
					targetPhone = cleanPhoneNumber(tok)
					if descClause == "" && len(targetTokens) > idx+1 {
						descClause = strings.Join(targetTokens[idx+1:], " ")
					}
					break
				}
			}
		}
	}

	if targetPhone == "" && targetName == "" {
		return nil, fmt.Errorf("nomor tujuan atau nama kontak belum ditentukan")
	}

	descClause = strings.TrimSpace(descClause)
	if descClause == "" {
		descClause = "Tagihan Pembayaran"
	}

	return &PaymentBillingInput{
		Amount:      amount,
		TargetPhone: targetPhone,
		TargetName:  targetName,
		Description: descClause,
	}, nil
}

func parseCurrency(s string) (float64, bool) {
	s = strings.TrimSpace(strings.ToLower(s))
	s = strings.TrimPrefix(s, "rp")
	s = strings.TrimPrefix(s, "rp.")
	s = strings.TrimSpace(s)

	multiplier := 1.0
	if strings.HasSuffix(s, "ribu") {
		multiplier = 1000.0
		s = strings.TrimSuffix(s, "ribu")
	} else if strings.HasSuffix(s, "rb") {
		multiplier = 1000.0
		s = strings.TrimSuffix(s, "rb")
	} else if strings.HasSuffix(s, "k") {
		multiplier = 1000.0
		s = strings.TrimSuffix(s, "k")
	} else if strings.HasSuffix(s, "juta") {
		multiplier = 1000000.0
		s = strings.TrimSuffix(s, "juta")
	} else if strings.HasSuffix(s, "jt") {
		multiplier = 1000000.0
		s = strings.TrimSuffix(s, "jt")
	} else if strings.HasSuffix(s, "m") {
		multiplier = 1000000.0
		s = strings.TrimSuffix(s, "m")
	}

	// Handle thousand dot separators like 50.000
	parts := strings.Split(s, ".")
	if len(parts) > 1 && len(parts[len(parts)-1]) == 3 && multiplier == 1.0 {
		s = strings.ReplaceAll(s, ".", "")
	} else {
		s = strings.ReplaceAll(s, ",", ".")
	}

	val, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return val * multiplier, true
}

func cleanPhoneNumber(phone string) string {
	phone = strings.TrimSpace(phone)
	phone = strings.ReplaceAll(phone, "+", "")
	phone = strings.ReplaceAll(phone, "-", "")
	phone = strings.ReplaceAll(phone, " ", "")
	phone = strings.ReplaceAll(phone, "(", "")
	phone = strings.ReplaceAll(phone, ")", "")
	if strings.HasPrefix(phone, "0") {
		phone = "62" + phone[1:]
	}
	return phone
}

func isPhoneNumber(s string) bool {
	cleaned := cleanPhoneNumber(s)
	if strings.HasPrefix(cleaned, "628") && len(cleaned) >= 10 && len(cleaned) <= 15 {
		_, err := strconv.ParseInt(cleaned, 10, 64)
		return err == nil
	}
	return false
}

func formatRupiah(amount float64) string {
	intVal := int64(amount)
	s := fmt.Sprintf("%d", intVal)
	var res []string
	for len(s) > 3 {
		res = append([]string{s[len(s)-3:]}, res...)
		s = s[:len(s)-3]
	}
	if len(s) > 0 {
		res = append([]string{s}, res...)
	}
	return strings.Join(res, ".")
}

func (s *CommandService) generateCompositeQRIS(qrisString, storeName, nominal string, expiry time.Time) ([]byte, error) {
	templateImg, _, err := image.Decode(bytes.NewReader(assets.QRISTemplate))
	if err != nil {
		return nil, fmt.Errorf("decode template: %w", err)
	}

	qr, err := qrcode.New(qrisString, qrcode.Medium)
	if err != nil {
		return nil, fmt.Errorf("create qr: %w", err)
	}
	qr.DisableBorder = true
	qrImg := qr.Image(662)

	dc := gg.NewContextForImage(templateImg)

	// Draw QR Code centered at (552, 825)
	dc.DrawImageAnchored(qrImg, 552, 825, 0.5, 0.5)

	fontRobotoBold, err := truetype.Parse(assets.RobotoBold)
	if err != nil {
		return nil, fmt.Errorf("parse RobotoBold: %w", err)
	}

	fontRobotoReg, err := truetype.Parse(assets.RobotoRegular)
	if err != nil {
		return nil, fmt.Errorf("parse RobotoRegular: %w", err)
	}

	fontZilla, err := truetype.Parse(assets.ZillaSlabBold)
	if err != nil {
		return nil, fmt.Errorf("parse ZillaSlabBold: %w", err)
	}

	// 1. Draw Nominal ("Rp 40.000")
	nominalClean := strings.TrimPrefix(strings.TrimSpace(nominal), "Rp")
	nominalClean = strings.TrimSpace(nominalClean)

	faceRp := truetype.NewFace(fontRobotoBold, &truetype.Options{Size: 34})
	faceNominal := truetype.NewFace(fontZilla, &truetype.Options{Size: 58})

	dc.SetFontFace(faceRp)
	wRp, _ := dc.MeasureString("Rp")
	gap := 10.0

	dc.SetFontFace(faceNominal)
	wNominal, _ := dc.MeasureString(nominalClean)

	totalNominalW := wRp + gap + wNominal
	startX := 552.0 - (totalNominalW / 2.0)
	yBaseNominal := 338.0

	// Draw "Rp" (top aligned with cap height of the numerals)
	dc.SetFontFace(faceRp)
	dc.SetHexColor("#616E7A")
	dc.DrawString("Rp", startX, yBaseNominal-14)

	// Draw nominal amount
	dc.SetFontFace(faceNominal)
	dc.SetHexColor("#1E2225")
	dc.DrawString(nominalClean, startX+wRp+gap, yBaseNominal)

	// 2. Draw Expired Time ("hingga 07 Oct 2026 21:12 WIB")
	if expiry.IsZero() {
		expiry = time.Now().Add(6 * time.Hour)
	}
	locWIB := time.FixedZone("WIB", 7*3600)
	expiryFormatted := expiry.In(locWIB).Format("02 Jan 2006 15:04 WIB")

	faceExpReg := truetype.NewFace(fontRobotoReg, &truetype.Options{Size: 33})
	faceExpBold := truetype.NewFace(fontRobotoBold, &truetype.Options{Size: 33})

	textPart1 := "hingga "
	dc.SetFontFace(faceExpReg)
	w1, _ := dc.MeasureString(textPart1)

	dc.SetFontFace(faceExpBold)
	w2, _ := dc.MeasureString(expiryFormatted)

	totalExpW := w1 + w2
	startExpX := 552.0 - (totalExpW / 2.0)
	yBaseExp := 1365.0

	dc.SetFontFace(faceExpReg)
	dc.SetHexColor("#526270")
	dc.DrawString(textPart1, startExpX, yBaseExp)

	dc.SetFontFace(faceExpBold)
	dc.SetHexColor("#526270")
	dc.DrawString(expiryFormatted, startExpX+w1, yBaseExp)

	buf := new(bytes.Buffer)
	if err := dc.EncodePNG(buf); err != nil {
		return nil, fmt.Errorf("encode composite png: %w", err)
	}

	return buf.Bytes(), nil
}

func (s *CommandService) handleContactsCommand(ctx context.Context, senderNumber, chatID string, cmd *ParsedCommand, messageID string) error {
	if cmd.HasHelpFlag() || (len(cmd.Args) > 0 && (cmd.Args[0] == "help" || cmd.Args[0] == "h")) {
		msg := "Penggunaan command contacts:\n" +
			"• `/contacts` atau `/contacts list` : Lihat daftar semua kontak terdaftar\n" +
			"• `/contacts delete <nama/nomor>` : Hapus kontak dari database\n" +
			"\nTips: Kontak otomatis tersimpan saat Anda menagih dengan format:\n`/payment tagih <nominal> ke <nomor> a/n <nama>, <ket>`"
		return s.gowaClient.SendText(ctx, s.deviceID, chatID, msg, messageID)
	}

	if len(cmd.Args) >= 2 && (strings.ToLower(cmd.Args[0]) == "delete" || strings.ToLower(cmd.Args[0]) == "del" || strings.ToLower(cmd.Args[0]) == "remove") {
		target := strings.TrimSpace(cmd.Args[1])
		norm := strings.ToLower(target)
		err := s.db.WithContext(ctx).Where("phone_number = ? OR normalized_name = ? OR LOWER(name) = ?", target, norm, norm).Delete(&postgres.Contact{}).Error
		if err != nil {
			return s.gowaClient.SendText(ctx, s.deviceID, chatID, "Gagal menghapus kontak.", messageID)
		}
		return s.gowaClient.SendText(ctx, s.deviceID, chatID, fmt.Sprintf("Kontak *%s* berhasil dihapus.", target), messageID)
	}

	var contacts []postgres.Contact
	if err := s.db.WithContext(ctx).Order("name ASC").Find(&contacts).Error; err != nil {
		return s.gowaClient.SendText(ctx, s.deviceID, chatID, "Gagal mengambil daftar kontak.", messageID)
	}

	if len(contacts) == 0 {
		return s.gowaClient.SendText(ctx, s.deviceID, chatID, "Belum ada kontak tersimpan.\n\nKontak akan otomatis tersimpan saat Anda menagih dengan format:\n`/payment tagih 50rb ke 628xxxx a/n Nama, ket`", messageID)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📖 *DAFTAR KONTAK TERDAFTAR (%d)*\n\n", len(contacts)))
	for idx, c := range contacts {
		sb.WriteString(fmt.Sprintf("%d. *%s* - `%s`\n", idx+1, c.Name, c.PhoneNumber))
	}
	sb.WriteString("\n_Gunakan `/payment tagih <nominal> ke <nama>, <ket>` untuk menagih langsung._")

	return s.gowaClient.SendText(ctx, s.deviceID, chatID, sb.String(), messageID)
}

