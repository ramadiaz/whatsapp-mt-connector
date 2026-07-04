package service

import (
	"context"
	"encoding/base64"
	"fmt"

	"github.com/ramadiaz/money-wa-bot/internal/domain/transaction"
	"github.com/ramadiaz/money-wa-bot/internal/integration/gowa"
	"github.com/ramadiaz/money-wa-bot/internal/integration/ninerouter"
	apperrors "github.com/ramadiaz/money-wa-bot/internal/shared/errors"
	"github.com/ramadiaz/money-wa-bot/internal/shared/logger"
	"github.com/ramadiaz/money-wa-bot/internal/shared/timeutil"
)

type ParserService struct {
	gowaClient    gowa.WhatsAppGateway
	nineRouter    *ninerouter.Client
	catCacheRepo  transaction.CategoryCacheRepository
	accCacheRepo  transaction.AccountCacheRepository
	deviceID      string
	maxMediaBytes int64
	maxRetries    int
}

func NewParserService(
	gowaClient gowa.WhatsAppGateway,
	nineRouter *ninerouter.Client,
	catCacheRepo transaction.CategoryCacheRepository,
	accCacheRepo transaction.AccountCacheRepository,
	deviceID string,
	maxMediaBytes int64,
	maxRetries int,
) *ParserService {
	return &ParserService{
		gowaClient:    gowaClient,
		nineRouter:    nineRouter,
		catCacheRepo:  catCacheRepo,
		accCacheRepo:  accCacheRepo,
		deviceID:      deviceID,
		maxMediaBytes: maxMediaBytes,
		maxRetries:    maxRetries,
	}
}

func (s *ParserService) ParseText(ctx context.Context, text string) (*ninerouter.AIExtractionResult, error) {
	logger.Log.Info().Msg("fetching category list from database cache")
	categories, err := s.catCacheRepo.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("load categories: %w", err)
	}
	logger.Log.Info().Int("count", len(categories)).Msg("categories retrieved successfully")

	logger.Log.Info().Msg("fetching account list from database cache")
	accounts, err := s.accCacheRepo.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("load accounts: %w", err)
	}
	logger.Log.Info().Int("count", len(accounts)).Msg("accounts retrieved successfully")

	today := timeutil.TodayJakarta()
	catLabels := CategoryLabels(categories)
	accLabels := AccountLabels(accounts)

	prompt := ninerouter.BuildTextPrompt(text, today, catLabels, accLabels)
	logger.Log.Debug().Str("text", text).Int("prompt_len", len(prompt)).Msg("prepared AI prompt for text parsing")

	return s.callAI(ctx, s.nineRouter.Model(), prompt, nil)
}

func (s *ParserService) ParseImage(ctx context.Context, messageID, phone, caption string) (*ninerouter.AIExtractionResult, error) {
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
	categories, err := s.catCacheRepo.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("load categories: %w", err)
	}
	logger.Log.Info().Int("count", len(categories)).Msg("categories retrieved successfully")

	logger.Log.Info().Msg("fetching account list from database cache")
	accounts, err := s.accCacheRepo.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("load accounts: %w", err)
	}
	logger.Log.Info().Int("count", len(accounts)).Msg("accounts retrieved successfully")

	today := timeutil.TodayJakarta()
	catLabels := CategoryLabels(categories)
	accLabels := AccountLabels(accounts)

	prompt := ninerouter.BuildImagePrompt(caption, today, catLabels, accLabels)
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
