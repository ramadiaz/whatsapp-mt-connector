package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/shopspring/decimal"
	"github.com/ramadiaz/money-wa-bot/internal/domain/transaction"
	"github.com/ramadiaz/money-wa-bot/internal/integration/gowa"
	"github.com/ramadiaz/money-wa-bot/internal/shared/money"
	apperrors "github.com/ramadiaz/money-wa-bot/internal/shared/errors"
	"github.com/ramadiaz/money-wa-bot/internal/shared/logger"
)

type ConfirmationService struct {
	pendingRepo transaction.PendingTransactionRepository
	txService   *TransactionService
	gowaClient  gowa.WhatsAppGateway
	deviceID    string
}

func NewConfirmationService(
	pendingRepo transaction.PendingTransactionRepository,
	txService *TransactionService,
	gowaClient gowa.WhatsAppGateway,
	deviceID string,
) *ConfirmationService {
	return &ConfirmationService{
		pendingRepo: pendingRepo,
		txService:   txService,
		gowaClient:  gowaClient,
		deviceID:    deviceID,
	}
}

func (s *ConfirmationService) Handle(ctx context.Context, chatID, text, replyToID, correlationID string) error {
	log := logger.WithCorrelationID(correlationID)
	cmd := strings.ToLower(strings.TrimSpace(text))

	switch cmd {
	case "ya", "yes", "confirm":
		return s.confirm(ctx, chatID, replyToID, correlationID)
	case "batal", "cancel", "tidak", "no":
		return s.cancel(ctx, chatID, replyToID)
	case "bantuan", "help", "/help":
		return s.gowaClient.SendText(ctx, s.deviceID, chatID, confirmHelpText(), replyToID)
	}
	_ = log
	return nil
}

func (s *ConfirmationService) IsConfirmationCommand(text string) bool {
	cmd := strings.ToLower(strings.TrimSpace(text))
	switch cmd {
	case "ya", "yes", "confirm", "batal", "cancel", "tidak", "no", "bantuan", "help", "/help":
		return true
	}
	return false
}

func (s *ConfirmationService) confirm(ctx context.Context, chatID, replyToID, correlationID string) error {
	log := logger.WithCorrelationID(correlationID)

	pending, err := s.pendingRepo.FindActiveByChat(ctx, chatID)
	if err != nil {
		if errors.Is(err, apperrors.ErrNoPendingTransaction) {
			return s.gowaClient.SendText(ctx, s.deviceID, chatID, "Tidak ada transaksi yang menunggu konfirmasi.", replyToID)
		}
		return err
	}

	created, err := s.txService.Commit(ctx, pending.ID, pending)
	if err != nil {
		log.Error().Err(err).Msg("commit failed")
		if errors.Is(err, apperrors.ErrMoneyTrackerRejected) {
			return s.gowaClient.SendText(ctx, s.deviceID, chatID, "Gagal menyimpan transaksi ke Money Tracker. Coba lagi nanti.", replyToID)
		}
		return err
	}

	amount, _ := decimal.NewFromString(pending.Amount)
	msg := fmt.Sprintf("✅ Transaksi berhasil disimpan!\n\nID: %s\nJumlah: %s\nTanggal: %s",
		created.ID,
		money.FormatRupiah(amount),
		pending.TransactionDate,
	)
	return s.gowaClient.SendText(ctx, s.deviceID, chatID, msg, replyToID)
}

func (s *ConfirmationService) cancel(ctx context.Context, chatID, replyToID string) error {
	pending, err := s.pendingRepo.FindActiveByChat(ctx, chatID)
	if err != nil {
		if errors.Is(err, apperrors.ErrNoPendingTransaction) {
			return s.gowaClient.SendText(ctx, s.deviceID, chatID, "Tidak ada transaksi yang perlu dibatalkan.", replyToID)
		}
		return err
	}

	_ = s.pendingRepo.MarkCancelled(ctx, pending.ID)
	return s.gowaClient.SendText(ctx, s.deviceID, chatID, "❌ Transaksi dibatalkan.", replyToID)
}

func confirmHelpText() string {
	return `🤖 *Money Tracker Bot*

*Format input:*
• catat kopi susu 25k
• makan siang 45 ribu tadi
• transport 80k tanggal 2 juli
• income freelance 1.500.000
• belanja shopee 230k pakai BCA

*Format manual:*
• expense | 25000 | food | kopi susu | 2026-07-03
• income | 1500000 | salary | freelance | 2026-07-03

*Konfirmasi:*
• Balas *ya* untuk menyimpan
• Balas *batal* untuk membatalkan`
}
