package service

import (
	"context"
	"encoding/base64"
	"fmt"
	"sort"
	"strings"

	"github.com/ramadiaz/whatsapp-mt-connector/internal/domain/transaction"
	"github.com/ramadiaz/whatsapp-mt-connector/internal/integration/gowa"
	"github.com/ramadiaz/whatsapp-mt-connector/internal/integration/moneytracker"
	"github.com/ramadiaz/whatsapp-mt-connector/internal/integration/ninerouter"
	apperrors "github.com/ramadiaz/whatsapp-mt-connector/internal/shared/errors"
	"github.com/ramadiaz/whatsapp-mt-connector/internal/shared/logger"
	"github.com/ramadiaz/whatsapp-mt-connector/internal/shared/timeutil"
)

type ParserService struct {
	gowaClient    gowa.WhatsAppGateway
	nineRouter    *ninerouter.Client
	catCacheRepo  transaction.CategoryCacheRepository
	accCacheRepo  transaction.AccountCacheRepository
	mtClient      moneytracker.MoneyTrackerClient
	deviceID      string
	maxMediaBytes int64
	maxRetries    int
}

func (s *ParserService) CategoryCacheRepo() transaction.CategoryCacheRepository {
	return s.catCacheRepo
}

func (s *ParserService) AccountCacheRepo() transaction.AccountCacheRepository {
	return s.accCacheRepo
}

func NewParserService(
	gowaClient gowa.WhatsAppGateway,
	nineRouter *ninerouter.Client,
	catCacheRepo transaction.CategoryCacheRepository,
	accCacheRepo transaction.AccountCacheRepository,
	mtClient moneytracker.MoneyTrackerClient,
	deviceID string,
	maxMediaBytes int64,
	maxRetries int,
) *ParserService {
	return &ParserService{
		gowaClient:    gowaClient,
		nineRouter:    nineRouter,
		catCacheRepo:  catCacheRepo,
		accCacheRepo:  accCacheRepo,
		mtClient:      mtClient,
		deviceID:      deviceID,
		maxMediaBytes: maxMediaBytes,
		maxRetries:    maxRetries,
	}
}

func (s *ParserService) ParseText(ctx context.Context, userUUID string, text string, mtClient moneytracker.MoneyTrackerClient) (*ninerouter.AIExtractionResult, error) {
	logger.Log.Info().Msg("fetching category list from database cache")
	categories, err := s.catCacheRepo.List(ctx, userUUID)
	if err != nil {
		return nil, fmt.Errorf("load categories: %w", err)
	}
	logger.Log.Info().Int("count", len(categories)).Msg("categories retrieved successfully")

	logger.Log.Info().Msg("fetching account list from database cache")
	accounts, err := s.accCacheRepo.List(ctx, userUUID)
	if err != nil {
		return nil, fmt.Errorf("load accounts: %w", err)
	}
	logger.Log.Info().Int("count", len(accounts)).Msg("accounts retrieved successfully")

	today := timeutil.TodayJakarta()
	catLabels := CategoryLabels(categories)
	accLabels := AccountLabels(accounts)

	userContext := s.buildUserContext(ctx, categories, mtClient)

	prompt := ninerouter.BuildTextPrompt(text, today, catLabels, accLabels, userContext)
	logger.Log.Debug().Str("text", text).Int("prompt_len", len(prompt)).Msg("prepared AI prompt for text parsing")

	return s.callAI(ctx, s.nineRouter.Model(), prompt, nil)
}

func (s *ParserService) ParseImage(ctx context.Context, userUUID string, messageID, phone, caption string, mtClient moneytracker.MoneyTrackerClient) (*ninerouter.AIExtractionResult, error) {
	logger.Log.Info().Str("message_id", messageID).Str("phone", phone).Msg("downloading message media via gowa client")
	imgBytes, mimeType, err := s.gowaClient.DownloadMessageMedia(ctx, s.deviceID, messageID, phone)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", apperrors.ErrGOWAUnavailable, err.Error())
	}

	logger.Log.Info().Str("mime_type", mimeType).Int("size_bytes", len(imgBytes)).Msg("message media downloaded successfully")
	if err := gowa.ValidateMediaMIME(mimeType); err != nil {
		return nil, apperrors.ErrUnsupportedMessageType
	}

	if int64(len(imgBytes)) > s.maxMediaBytes {
		return nil, apperrors.ErrMediaTooLarge
	}

	logger.Log.Info().Msg("fetching category list from database cache")
	categories, err := s.catCacheRepo.List(ctx, userUUID)
	if err != nil {
		return nil, fmt.Errorf("load categories: %w", err)
	}
	logger.Log.Info().Int("count", len(categories)).Msg("categories retrieved successfully")

	logger.Log.Info().Msg("fetching account list from database cache")
	accounts, err := s.accCacheRepo.List(ctx, userUUID)
	if err != nil {
		return nil, fmt.Errorf("load accounts: %w", err)
	}
	logger.Log.Info().Int("count", len(accounts)).Msg("accounts retrieved successfully")

	today := timeutil.TodayJakarta()
	catLabels := CategoryLabels(categories)
	accLabels := AccountLabels(accounts)

	userContext := s.buildUserContext(ctx, categories, mtClient)

	prompt := ninerouter.BuildImagePrompt(caption, today, catLabels, accLabels, userContext)
	logger.Log.Debug().Str("caption", caption).Int("prompt_len", len(prompt)).Msg("prepared AI vision prompt for image parsing")

	b64 := base64.StdEncoding.EncodeToString(imgBytes)
	dataURL := fmt.Sprintf("data:%s;base64,%s", mimeType, b64)

	imageContent := []ninerouter.Message{
		{
			Role: "user",
			Content: []interface{}{
				ninerouter.ImageContent{
					Type:     "image_url",
					ImageURL: ninerouter.ImageURL{URL: dataURL},
				},
			},
		},
	}

	return s.callAI(ctx, s.nineRouter.VisionModel(), prompt, imageContent)
}

func (s *ParserService) buildUserContext(ctx context.Context, categories []transaction.Category, mtClient moneytracker.MoneyTrackerClient) string {
	txs, err := mtClient.GetTransactions(ctx, 200)
	if err != nil {
		logger.Log.Warn().Err(err).Msg("failed to load transactions history for user context")
		return ""
	}
	if len(txs) == 0 {
		return ""
	}
	catMap := make(map[string]string)
	for _, cat := range categories {
		catMap[cat.CategoryID] = cat.Title
	}
	type remarkCat struct {
		remark string
		cat    string
	}
	counts := make(map[remarkCat]int)
	remarkTotal := make(map[string]int)
	for _, tx := range txs {
		r := strings.ToLower(strings.TrimSpace(tx.Remark))
		if r == "" {
			continue
		}
		catTitle := catMap[tx.IncomeExpenditureCategoryID]
		if catTitle == "" {
			continue
		}
		pair := remarkCat{remark: r, cat: catTitle}
		counts[pair]++
		remarkTotal[r]++
	}
	bestCat := make(map[string]string)
	bestCount := make(map[string]int)
	for pair, count := range counts {
		if count > bestCount[pair.remark] {
			bestCount[pair.remark] = count
			bestCat[pair.remark] = pair.cat
		}
	}
	type remarkFreq struct {
		remark string
		freq   int
	}
	var freqs []remarkFreq
	for r, tot := range remarkTotal {
		freqs = append(freqs, remarkFreq{remark: r, freq: tot})
	}
	sort.Slice(freqs, func(i, j int) bool {
		return freqs[i].freq > freqs[j].freq
	})
	limit := 50
	if len(freqs) < limit {
		limit = len(freqs)
	}
	var sb strings.Builder
	sb.WriteString("User categorization history/habits:\n")
	for i := 0; i < limit; i++ {
		r := freqs[i].remark
		cat := bestCat[r]
		sb.WriteString(fmt.Sprintf("- When user mentions \"%s\", categorize it under \"%s\"\n", r, cat))
	}
	return sb.String()
}

func (s *ParserService) callAI(ctx context.Context, model, systemPrompt string, extraMessages []ninerouter.Message) (*ninerouter.AIExtractionResult, error) {
	var messages []ninerouter.Message
	if extraMessages != nil {
		messages = extraMessages
	} else {
		messages = []ninerouter.Message{
			{Role: "user", Content: systemPrompt},
		}
	}

	var lastErr error
	for attempt := 0; attempt <= s.maxRetries; attempt++ {
		logger.Log.Info().Str("model", model).Int("attempt", attempt).Msg("calling ninerouter AI completion API")
		raw, err := s.nineRouter.Complete(ctx, model, systemPrompt, messages, 0)
		if err != nil {
			logger.Log.Warn().Err(err).Int("attempt", attempt).Msg("ninerouter AI completion API call failed, retrying")
			lastErr = fmt.Errorf("%w: %s", apperrors.ErrAIUnavailable, err.Error())
			continue
		}

		logger.Log.Debug().Str("raw_response", raw).Msg("ninerouter raw response received, starting validation")
		result, err := ninerouter.ParseAndValidate(raw)
		if err != nil {
			logger.Log.Warn().Err(err).Int("attempt", attempt).Msg("parsing and validation of raw AI response failed, retrying")
			lastErr = err
			continue
		}

		logger.Log.Info().Interface("extraction_result", result).Msg("AI extraction parsed and validated successfully")
		return result, nil
	}

	logger.Log.Error().Err(lastErr).Msg("all retries for AI completion failed")
	return nil, lastErr
}

func (s *ParserService) AnalyzeWasteful(ctx context.Context, remark, category string, amount float64) (bool, string, error) {
	prompt := ninerouter.BuildWastefulPrompt(remark, category, amount)
	messages := []ninerouter.Message{
		{Role: "user", Content: prompt},
	}

	raw, err := s.nineRouter.Complete(ctx, s.nineRouter.Model(), prompt, messages, 200)
	if err != nil {
		logger.Log.Warn().Err(err).Msg("wasteful analysis AI call failed")
		return false, "", err
	}

	result, err := ninerouter.ParseWasteful(raw)
	if err != nil {
		logger.Log.Warn().Err(err).Str("raw", raw).Msg("parse wasteful AI response failed")
		return false, "", err
	}

	return result.Wasteful, result.Reason, nil
}

