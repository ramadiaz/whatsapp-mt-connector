package service

import (
	"context"
	"fmt"
	"strings"

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
	adminNumbers  []string
	blacklistRepo blacklist.Repository
	userRepo      *postgres.UserRepository
	db            *gorm.DB
	gowaClient    gowaintegration.WhatsAppGateway
	deviceID      string
}

func NewCommandService(
	adminNumbers []string,
	blacklistRepo blacklist.Repository,
	userRepo *postgres.UserRepository,
	db *gorm.DB,
	gowaClient gowaintegration.WhatsAppGateway,
	deviceID string,
) *CommandService {
	return &CommandService{
		adminNumbers:  adminNumbers,
		blacklistRepo: blacklistRepo,
		userRepo:      userRepo,
		db:            db,
		gowaClient:    gowaClient,
		deviceID:      deviceID,
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
	case "help":
		return s.handleHelpCommand(ctx, chatID, cmd, messageID)
	default:
		msg := fmt.Sprintf("Command `/%s` tidak ditemukan. Gunakan `/help` untuk melihat daftar command.", cmd.Name)
		return s.gowaClient.SendText(ctx, s.deviceID, chatID, msg, messageID)
	}
}

func (s *CommandService) handleHelpCommand(ctx context.Context, chatID string, cmd *ParsedCommand, messageID string) error {
	msg := "Daftar Admin Command:\n- `/blacklist` : Kelola nomor diblokir (`add`, `remove`, `list`)\n- `/users` : Lihat daftar pengguna terdaftar\n- `/stats` : Lihat statistik & status sistem\n- `/help` : Tampilkan bantuan\n\nTips: Tambahkan `--help` atau `-h` pada command untuk melihat opsi."
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
