package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"github.com/skip2/go-qrcode"
	"strconv"
	"strings"
	"time"

	"github.com/ramadiaz/whatsapp-mt-connector/internal/domain/blacklist"
	gowaintegration "github.com/ramadiaz/whatsapp-mt-connector/internal/integration/gowa"
	"github.com/ramadiaz/whatsapp-mt-connector/internal/persistence/postgres"
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
}

func NewCommandService(
	adminNumbers []string,
	blacklistRepo blacklist.Repository,
	userRepo *postgres.UserRepository,
	db *gorm.DB,
	gowaClient gowaintegration.WhatsAppGateway,
	deviceID string,
	gopayClientURL string,
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
	case "help":
		return s.handleHelpCommand(ctx, chatID, cmd, messageID)
	default:
		msg := fmt.Sprintf("Command `/%s` tidak ditemukan. Gunakan `/help` untuk melihat daftar command.", cmd.Name)
		return s.gowaClient.SendText(ctx, s.deviceID, chatID, msg, messageID)
	}
}

func (s *CommandService) handleHelpCommand(ctx context.Context, chatID string, cmd *ParsedCommand, messageID string) error {
	msg := "Daftar Command:\n- `/payment` : Tagih utang via QRIS dinamis GoPay Merchant (`/payment tagih 50rb ke 62813xxxxx bayar kopi`)\n- `/cal` : Hitung kalori dari teks / gambar makanan\n- `/blacklist` : Kelola nomor diblokir (`add`, `remove`, `list`)\n- `/users` : Lihat daftar pengguna terdaftar\n- `/stats` : Lihat statistik & status sistem\n- `/help` : Tampilkan bantuan\n\nTips: Tambahkan `--help` atau `-h` pada command untuk melihat opsi."
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

	// Parse billing parameters
	req, err := parsePaymentBilling(rawBody)
	if err != nil {
		msg := fmt.Sprintf("⚠️ Format tagihan salah: %v\n\nContoh yang benar:\n`/payment tagih 50rb ke 62813xxxxx bayar kopi`", err)
		return s.gowaClient.SendText(ctx, s.deviceID, chatID, msg, messageID)
	}

	// Call GoPay Client API
	endpoint := fmt.Sprintf("%s/api/v1/bills", s.gopayClientURL)
	payload := map[string]any{
		"amount":         req.Amount,
		"description":    req.Description,
		"payer_phone":    req.TargetPhone,
		"admin_phone":    senderNumber,
		"expiry_minutes": 15,
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
		} `json:"data"`
	}
	if err := json.Unmarshal(respBytes, &apiResp); err != nil {
		return s.gowaClient.SendText(ctx, s.deviceID, chatID, "Gagal membaca respons dari GoPay Client.", messageID)
	}

	successMsg := fmt.Sprintf(
		"🧾 *TAGIHAN QRIS BERHASIL DIBUAT*\n\n"+
			"• *Penerima:* %s\n"+
			"• *Nominal:* Rp %s\n"+
			"• *Keperluan:* %s\n"+
			"• *Order ID:* `%s`\n"+
			"• *Status:* Menunggu Pembayaran (15 menit)\n\n"+
			"Kode QRIS Dinamis dan rincian invoice telah dikirimkan ke WhatsApp %s.\n"+
			"_Sistem akan otomatis memberi tahu Anda begitu tagihan lunas._",
		req.TargetPhone,
		formatRupiah(req.Amount),
		req.Description,
		apiResp.Data.OrderID,
		req.TargetPhone,
	)

	// SEND THE ACTUAL QRIS TO TARGET
	go func() {
		targetChatID := req.TargetPhone + "@s.whatsapp.net"
		targetMsg := fmt.Sprintf(
			"Halo! Anda menerima tagihan baru dari *GoPay Merchant*\n\n" +
			"• *Nominal:* Rp %s\n" +
			"• *Keperluan:* %s\n" +
			"• *Order ID:* %s\n\n" +
			"Silakan scan kode QRIS di bawah ini menggunakan aplikasi M-Banking atau E-Wallet Anda. Tagihan ini akan otomatis kedaluwarsa dalam 15 menit.",
			formatRupiah(req.Amount), req.Description, apiResp.Data.OrderID,
		)
		
		// Generate QRIS code natively using go-qrcode
		pngBytes, err := qrcode.Encode(apiResp.Data.QRISString, qrcode.Medium, 300)
		if err == nil {
			// Send Image
			_ = s.gowaClient.SendImage(context.Background(), s.deviceID, targetChatID, targetMsg, pngBytes, "qris.png", "")
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
	Amount      float64
	TargetPhone string
	Description string
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
	var descTokens []string

	i := 0
	for i < len(tokens) {
		token := tokens[i]
		lower := strings.ToLower(token)

		if lower == "ke" && i+1 < len(tokens) {
			targetPhone = cleanPhoneNumber(tokens[i+1])
			i += 2
			continue
		}

		if targetPhone == "" && isPhoneNumber(token) {
			targetPhone = cleanPhoneNumber(token)
			i++
			continue
		}

		if amount == 0 {
			if parsed, ok := parseCurrency(token); ok && parsed > 0 {
				amount = parsed
				i++
				continue
			}
		}

		descTokens = append(descTokens, token)
		i++
	}

	if amount <= 0 {
		return nil, fmt.Errorf("nominal tagihan tidak valid")
	}
	if targetPhone == "" {
		return nil, fmt.Errorf("nomor tujuan belum ditentukan (format: 'ke 62813xxxx')")
	}

	desc := strings.Join(descTokens, " ")
	if desc == "" {
		desc = "Tagihan Pembayaran"
	}

	return &PaymentBillingInput{
		Amount:      amount,
		TargetPhone: targetPhone,
		Description: desc,
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
