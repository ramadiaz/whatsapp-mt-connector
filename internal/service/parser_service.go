package service

import (
	"context"
	"encoding/base64"
	"fmt"

	"github.com/ramadiaz/money-wa-bot/internal/domain/transaction"
	"github.com/ramadiaz/money-wa-bot/internal/integration/gowa"
	"github.com/ramadiaz/money-wa-bot/internal/integration/ninerouter"
	apperrors "github.com/ramadiaz/money-wa-bot/internal/shared/errors"
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
	categories, err := s.catCacheRepo.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("load categories: %w", err)
	}
	accounts, err := s.accCacheRepo.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("load accounts: %w", err)
	}

	today := timeutil.TodayJakarta()
	catLabels := CategoryLabels(categories)
	accLabels := AccountLabels(accounts)

	prompt := ninerouter.BuildTextPrompt(text, today, catLabels, accLabels)

	return s.callAI(ctx, s.nineRouter.Model(), prompt, nil)
}

func (s *ParserService) ParseImage(ctx context.Context, messageID, phone, caption string) (*ninerouter.AIExtractionResult, error) {
	imgBytes, mimeType, err := s.gowaClient.DownloadMessageMedia(ctx, s.deviceID, messageID, phone)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", apperrors.ErrGOWAUnavailable, err.Error())
	}

	if err := gowa.ValidateMediaMIME(mimeType); err != nil {
		return nil, apperrors.ErrUnsupportedMessageType
	}

	if int64(len(imgBytes)) > s.maxMediaBytes {
		return nil, apperrors.ErrMediaTooLarge
	}

	categories, err := s.catCacheRepo.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("load categories: %w", err)
	}
	accounts, err := s.accCacheRepo.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("load accounts: %w", err)
	}

	today := timeutil.TodayJakarta()
	catLabels := CategoryLabels(categories)
	accLabels := AccountLabels(accounts)

	prompt := ninerouter.BuildImagePrompt(caption, today, catLabels, accLabels)

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
		raw, err := s.nineRouter.Complete(ctx, model, systemPrompt, messages, 512)
		if err != nil {
			lastErr = fmt.Errorf("%w: %s", apperrors.ErrAIUnavailable, err.Error())
			continue
		}

		result, err := ninerouter.ParseAndValidate(raw)
		if err != nil {
			lastErr = err
			continue
		}

		return result, nil
	}

	return nil, lastErr
}
