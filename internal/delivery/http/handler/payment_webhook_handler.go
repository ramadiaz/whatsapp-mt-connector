package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	gowaintegration "github.com/ramadiaz/whatsapp-mt-connector/internal/integration/gowa"
	"github.com/ramadiaz/whatsapp-mt-connector/internal/persistence/postgres"
	"github.com/ramadiaz/whatsapp-mt-connector/internal/shared/logger"
	"gorm.io/gorm"
)

type PaymentNotificationPayload struct {
	ID          string    `json:"id"`
	TrxID       string    `json:"trx_id"`
	Amount      float64   `json:"amount"`
	Status      string    `json:"status"`
	AdminPhone  string    `json:"admin_phone"`
	PayerPhone  string    `json:"payer_phone"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type PaymentWebhookHandler struct {
	gowaClient gowaintegration.WhatsAppGateway
	deviceID   string
	db         *gorm.DB
}

func NewPaymentWebhookHandler(gowaClient gowaintegration.WhatsAppGateway, deviceID string, db *gorm.DB) *PaymentWebhookHandler {
	return &PaymentWebhookHandler{
		gowaClient: gowaClient,
		deviceID:   deviceID,
		db:         db,
	}
}

func (h *PaymentWebhookHandler) Handle(w http.ResponseWriter, r *http.Request) {
	correlationID := r.Header.Get("X-Request-Id")
	log := logger.WithCorrelationID(correlationID)
	log.Info().Str("method", r.Method).Str("path", r.URL.Path).Msg("received payment webhook notification")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Error().Err(err).Msg("failed to read payment webhook body")
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var payload PaymentNotificationPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		log.Error().Err(err).Str("body", string(body)).Msg("failed to decode payment webhook payload")
		http.Error(w, "invalid json payload", http.StatusBadRequest)
		return
	}

	log.Info().
		Str("order_id", payload.ID).
		Str("status", payload.Status).
		Float64("amount", payload.Amount).
		Str("payer", payload.PayerPhone).
		Str("admin", payload.AdminPhone).
		Msg("processing payment notification")

	// If status is SETTLEMENT, send WhatsApp confirmation messages
	if strings.ToUpper(payload.Status) == "SETTLEMENT" {
		h.handleSettlement(r.Context(), payload)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (h *PaymentWebhookHandler) handleSettlement(ctx context.Context, p PaymentNotificationPayload) {
	cleanPayer := cleanPhone(p.PayerPhone)
	cleanAdmin := cleanPhone(p.AdminPhone)

	payerName := cleanPayer
	payerDisplayName := cleanPayer
	if h.db != nil && cleanPayer != "" {
		var contact postgres.Contact
		if err := h.db.WithContext(ctx).Where("phone_number = ?", cleanPayer).First(&contact).Error; err == nil && contact.Name != "" {
			payerDisplayName = contact.Name
			payerName = fmt.Sprintf("%s (%s)", contact.Name, cleanPayer)
		}
	}

	desc := p.Description
	if desc == "" {
		desc = "Tagihan Pembayaran"
	}

	nominalStr := formatRupiahAmount(p.Amount)

	// 1. Send confirmation message to Admin
	if cleanAdmin != "" {
		adminChatID := cleanAdmin + "@s.whatsapp.net"
		adminMsg := fmt.Sprintf(
			"🧾 *PEMBAYARAN DITERIMA (LUNAS)*\n\n"+
				"• *Order ID:* `%s`\n"+
				"• *Pembayar:* %s\n"+
				"• *Nominal:* Rp %s\n"+
				"• *Keperluan:* %s\n"+
				"• *Status:* ✅ Lunas (Settlement)\n\n"+
				"_Pembayaran telah berhasil diverifikasi oleh GoPay Merchant._",
			p.ID,
			payerName,
			nominalStr,
			desc,
		)

		if err := h.gowaClient.SendText(ctx, h.deviceID, adminChatID, adminMsg, ""); err != nil {
			logger.Log.Error().Err(err).Str("admin", cleanAdmin).Msg("failed to send settlement notification to admin")
		} else {
			logger.Log.Info().Str("admin", cleanAdmin).Msg("settlement notification sent to admin successfully")
		}
	}

	// 2. Send thank-you confirmation message to Payer
	if cleanPayer != "" {
		payerChatID := cleanPayer + "@s.whatsapp.net"
		payerMsg := fmt.Sprintf(
			"Yth. %s,\n\n"+
				"Sistem telah mendeteksi kembalinya sejumlah Rp%s. Dana telah berhasil kembali ke habitat asalnya dan tagihan Anda resmi dinyatakan:\n\n"+
				"LUNAS\n\n"+
				"Terima kasih telah mengembalikan aset tersebut dengan selamat.\n"+
				"Hubungan pertemanan Anda kini kembali berada dalam kondisi sehat dan stabil.\n\n"+
				"ID: %s",
			payerDisplayName,
			nominalStr,
			p.ID,
		)

		if err := h.gowaClient.SendText(ctx, h.deviceID, payerChatID, payerMsg, ""); err != nil {
			logger.Log.Error().Err(err).Str("payer", cleanPayer).Msg("failed to send payment confirmation to payer")
		} else {
			logger.Log.Info().Str("payer", cleanPayer).Msg("payment confirmation sent to payer successfully")
		}
	}
}

func cleanPhone(phone string) string {
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

func formatRupiahAmount(amount float64) string {
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
