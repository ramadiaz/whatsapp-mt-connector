package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ramadiaz/whatsapp-mt-connector/internal/domain/transaction"
	"github.com/ramadiaz/whatsapp-mt-connector/internal/integration/gowa"
	"github.com/ramadiaz/whatsapp-mt-connector/internal/integration/moneytracker"
	apperrors "github.com/ramadiaz/whatsapp-mt-connector/internal/shared/errors"
	"github.com/ramadiaz/whatsapp-mt-connector/internal/shared/logger"
	"github.com/ramadiaz/whatsapp-mt-connector/internal/shared/money"
	"github.com/shopspring/decimal"
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

func (s *ConfirmationService) Handle(ctx context.Context, chatID, text, replyToID, correlationID string, mtClient moneytracker.MoneyTrackerClient) error {
	log := logger.WithCorrelationID(correlationID)
	cmd := strings.ToLower(strings.TrimSpace(text))

	log.Info().Str("command", cmd).Str("chat_id", chatID).Msg("processing confirmation command")
	switch cmd {
	case "ya", "yes", "confirm":
		log.Info().Msg("confirmation input matches yes, proceeding to confirm transaction")
		return s.confirm(ctx, chatID, replyToID, correlationID, mtClient)
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

func (s *ConfirmationService) confirm(ctx context.Context, chatID, replyToID, correlationID string, mtClient moneytracker.MoneyTrackerClient) error {
	log := logger.WithCorrelationID(correlationID)

	log.Info().Str("chat_id", chatID).Msg("searching for active pending transactions")
	pendings, err := s.pendingRepo.FindAllActiveByChat(ctx, chatID)
	if err != nil {
		if errors.Is(err, apperrors.ErrNoPendingTransaction) {
			log.Warn().Str("chat_id", chatID).Msg("no active pending transaction found for confirmation request")
			return s.gowaClient.SendText(ctx, s.deviceID, chatID, "...e-eh... (aku mesti bilang apa) ...kayaknya nggak ada transaksi yang lagi nunggu konfirmasi... 😶", replyToID)
		}
		log.Error().Err(err).Str("chat_id", chatID).Msg("failed finding active pending transactions")
		return err
	}

	latestSourceMsgID := pendings[len(pendings)-1].SourceMessageID
	var toConfirm []*transaction.PendingTransactionRow
	for _, pending := range pendings {
		if pending.SourceMessageID == latestSourceMsgID {
			toConfirm = append(toConfirm, pending)
		} else {
			log.Info().Str("pending_uuid", pending.UUID).Msg("cancelling older unconfirmed pending transaction")
			_ = s.pendingRepo.MarkCancelled(ctx, pending.UUID)
		}
	}

	var committed []*transaction.CreatedTransaction
	var committedPendings []*transaction.PendingTransactionRow
	for _, pending := range toConfirm {
		log.Info().Str("pending_uuid", pending.UUID).Msg("found active pending transaction, committing to money tracker")
		created, err := s.txService.Commit(ctx, pending.UUID, pending, mtClient)
		if err != nil {
			log.Error().Err(err).Str("pending_uuid", pending.UUID).Msg("transaction commit failed")
			if errors.Is(err, apperrors.ErrMoneyTrackerRejected) {
				return s.gowaClient.SendText(ctx, s.deviceID, chatID, "...a-ada yang salah waktu nyimpen ke Money Tracker... (aku udah coba tapi tetep gagal) ...m-maaf ya. Coba lagi nanti... 😔", replyToID)
			}
			return err
		}
		committed = append(committed, created)
		committedPendings = append(committedPendings, pending)
	}

	var msg string
	if len(committed) == 1 {
		amount, _ := decimal.NewFromString(committedPendings[0].Amount)
		msg = fmt.Sprintf("...b-berhasil disimpan... (aku nggak nyangka selancar ini) 🎸\n\nID: %s\nJumlah: %s\nTanggal: %s\n\n...makasih udah percaya aku.",
			committed[0].ID,
			money.FormatRupiah(amount),
			committedPendings[0].TransactionDate,
		)
	} else {
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("...b-berhasil disimpan %d transaksi... (aku nggak nyangka selancar ini) 🎸\n\n", len(committed)))
		for i, c := range committed {
			p := committedPendings[i]
			amount, _ := decimal.NewFromString(p.Amount)
			sb.WriteString(fmt.Sprintf("• ID: %s | %s (%s)\n", c.ID, money.FormatRupiah(amount), p.Remark))
		}
		sb.WriteString("\n...makasih udah percaya aku.")
		msg = sb.String()
	}

	log.Info().Int("count", len(committed)).Msg("transactions committed successfully, sending success reply")
	return s.gowaClient.SendText(ctx, s.deviceID, chatID, msg, replyToID)
}

func (s *ConfirmationService) cancel(ctx context.Context, chatID, replyToID, correlationID string) error {
	log := logger.WithCorrelationID(correlationID)

	log.Info().Str("chat_id", chatID).Msg("searching for active pending transactions to cancel")
	pendings, err := s.pendingRepo.FindAllActiveByChat(ctx, chatID)
	if err != nil {
		if errors.Is(err, apperrors.ErrNoPendingTransaction) {
			log.Warn().Str("chat_id", chatID).Msg("no active pending transaction found for cancellation request")
			return s.gowaClient.SendText(ctx, s.deviceID, chatID, "...e-eh... (aku mesti bilang apa) ...kayaknya nggak ada transaksi yang perlu dibatalin... 😶", replyToID)
		}
		log.Error().Err(err).Str("chat_id", chatID).Msg("failed finding active pending transactions")
		return err
	}

	for _, pending := range pendings {
		log.Info().Str("pending_uuid", pending.UUID).Msg("found active pending transaction, marking cancelled in database")
		_ = s.pendingRepo.MarkCancelled(ctx, pending.UUID)
	}
	log.Info().Msg("transactions cancelled successfully, sending cancel reply")
	return s.gowaClient.SendText(ctx, s.deviceID, chatID, "...o-oke... transaksinya udah dibatalin... (ini bukan berarti gagal, cuma dibatalin) ...nggak apa-apa kok. 🙏", replyToID)
}

func confirmHelpText() string {
	return `...a-ano... (aku nggak biasa ngomong depan orang tapi ini penting) ...aku Money Tracker Bot... 🎸

(Deep breath) ...ini cara pakainya:

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
• Balas *batal* untuk membatalkan

...s-semoga membantu. (aku nggak tau gimana caranya terlihat lebih ramah dari ini)`
}
