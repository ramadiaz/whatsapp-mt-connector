package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/shopspring/decimal"
	"github.com/ramadiaz/whatsapp-mt-connector/internal/domain/transaction"
	"github.com/ramadiaz/whatsapp-mt-connector/internal/integration/gowa"
	"github.com/ramadiaz/whatsapp-mt-connector/internal/shared/money"
	apperrors "github.com/ramadiaz/whatsapp-mt-connector/internal/shared/errors"
	"github.com/ramadiaz/whatsapp-mt-connector/internal/shared/logger"
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

	log.Info().Str("command", cmd).Str("chat_id", chatID).Msg("processing confirmation command")
	switch cmd {
	case "ya", "yes", "confirm":
		log.Info().Msg("confirmation input matches yes, proceeding to confirm transaction")
		return s.confirm(ctx, chatID, replyToID, correlationID)
	case "batal", "cancel", "tidak", "no":
		log.Info().Msg("confirmation input matches cancel, proceeding to cancel transaction")
		return s.cancel(ctx, chatID, replyToID, correlationID)
	case "bantuan", "help", "/help":
		log.Info().Msg("confirmation input matches help, sending confirmation help text")
		return s.gowaClient.SendText(ctx, s.deviceID, chatID, confirmHelpText(), replyToID)
	}
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

	log.Info().Str("chat_id", chatID).Msg("searching for active pending transaction")
	pending, err := s.pendingRepo.FindActiveByChat(ctx, chatID)
	if err != nil {
		if errors.Is(err, apperrors.ErrNoPendingTransaction) {
			log.Warn().Str("chat_id", chatID).Msg("no active pending transaction found for confirmation request")
			return s.gowaClient.SendText(ctx, s.deviceID, chatID, "Tidak ada transaksi yang menunggu konfirmasi.", replyToID)
		}
		log.Error().Err(err).Str("chat_id", chatID).Msg("failed finding active pending transaction")
		return err
	}

	log.Info().Int64("pending_id", pending.ID).Msg("found active pending transaction, committing to money tracker")
	created, err := s.txService.Commit(ctx, pending.ID, pending)
	if err != nil {
		log.Error().Err(err).Int64("pending_id", pending.ID).Msg("transaction commit failed")
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
	log.Info().Int64("pending_id", pending.ID).Str("mt_tx_id", created.ID).Msg("transaction committed successfully, sending success reply")
	return s.gowaClient.SendText(ctx, s.deviceID, chatID, msg, replyToID)
}

func (s *ConfirmationService) cancel(ctx context.Context, chatID, replyToID, correlationID string) error {
	log := logger.WithCorrelationID(correlationID)

	log.Info().Str("chat_id", chatID).Msg("searching for active pending transaction to cancel")
	pending, err := s.pendingRepo.FindActiveByChat(ctx, chatID)
	if err != nil {
		if errors.Is(err, apperrors.ErrNoPendingTransaction) {
			log.Warn().Str("chat_id", chatID).Msg("no active pending transaction found for cancellation request")
			return s.gowaClient.SendText(ctx, s.deviceID, chatID, "Tidak ada transaksi yang perlu dibatalkan.", replyToID)
		}
		log.Error().Err(err).Str("chat_id", chatID).Msg("failed finding active pending transaction")
		return err
	}

	log.Info().Int64("pending_id", pending.ID).Msg("found active pending transaction, marking cancelled in database")
	_ = s.pendingRepo.MarkCancelled(ctx, pending.ID)
	log.Info().Int64("pending_id", pending.ID).Msg("transaction cancelled successfully, sending cancel reply")
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
