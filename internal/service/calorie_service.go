package service

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/ramadiaz/whatsapp-mt-connector/internal/integration/gowa"
	"github.com/ramadiaz/whatsapp-mt-connector/internal/integration/ninerouter"
	"github.com/ramadiaz/whatsapp-mt-connector/internal/shared/logger"
)

const calorieSystemPrompt = `You are a professional nutritionist and calorie counter assistant.
Your task is to analyze the food items described in the user text or shown in the image (including nutrition fact tables or food photos).
Provide a clear, helpful, and concise breakdown in Indonesian.

Response structure guidelines:
- Title / header identifying the food item(s)
- Breakdown per item (estimated portion/weight, calories, protein, carbs, fat)
- Total summary (Total Kalori, Total Protein, Total Karbohidrat, Total Lemak)
- Brief health note or recommendation if relevant

Keep formatting clean and readable for WhatsApp (use *bold* for emphasis, bullet points, emoji appropriately).
If the input cannot be identified as food, nutrition facts, or meal, politely inform the user.`

type CalorieService struct {
	gowaClient    gowa.WhatsAppGateway
	nineRouter    *ninerouter.Client
	deviceID      string
	maxMediaBytes int64
}

func NewCalorieService(
	gowaClient gowa.WhatsAppGateway,
	nineRouter *ninerouter.Client,
	deviceID string,
	maxMediaBytes int64,
) *CalorieService {
	return &CalorieService{
		gowaClient:    gowaClient,
		nineRouter:    nineRouter,
		deviceID:      deviceID,
		maxMediaBytes: maxMediaBytes,
	}
}

func (s *CalorieService) CalculateFromText(ctx context.Context, query string) (string, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return "Sebutkan makanan yang ingin dihitung kalorinya.\n\nContoh: `/cal indomie goreng + telur ceplok`", nil
	}

	messages := []ninerouter.Message{
		{
			Role:    "user",
			Content: fmt.Sprintf("Hitung kalori dan nutrisi untuk makanan berikut:\n%s", query),
		},
	}

	return s.nineRouter.Complete(ctx, s.nineRouter.Model(), calorieSystemPrompt, messages, 1000)
}

func (s *CalorieService) CalculateFromImage(ctx context.Context, messageID, phone, query string) (string, error) {
	imgBytes, mimeType, err := s.gowaClient.DownloadMessageMedia(ctx, s.deviceID, messageID, phone)
	if err != nil {
		return "", fmt.Errorf("download media: %w", err)
	}

	if int64(len(imgBytes)) > s.maxMediaBytes {
		return "", fmt.Errorf("image size exceeds maximum allowed limit")
	}

	b64Data := base64.StdEncoding.EncodeToString(imgBytes)
	dataURI := fmt.Sprintf("data:%s;base64,%s", mimeType, b64Data)

	userPrompt := "Analisis makanan / tabel informasi nilai gizi pada gambar ini dan hitung rincian kalori serta makronutrisinya."
	if strings.TrimSpace(query) != "" {
		userPrompt = fmt.Sprintf("%s\nCatatan tambahan dari user: %s", userPrompt, strings.TrimSpace(query))
	}

	userMsg := ninerouter.Message{
		Role: "user",
		Content: []interface{}{
			ninerouter.ImageContent{
				Type: "image_url",
				ImageURL: ninerouter.ImageURL{
					URL: dataURI,
				},
			},
			ninerouter.TextContent{
				Type: "text",
				Text: userPrompt,
			},
		},
	}

	return s.nineRouter.Complete(ctx, s.nineRouter.VisionModel(), calorieSystemPrompt, []ninerouter.Message{userMsg}, 1000)
}

func (s *CalorieService) HandleCalorieQuery(ctx context.Context, chatID, messageID, phone, query string, isImage bool, targetImageID string) error {
	logger.Log.Info().Str("chat_id", chatID).Bool("is_image", isImage).Str("query", query).Msg("processing calorie calculation query")

	var result string
	var err error

	if isImage {
		result, err = s.CalculateFromImage(ctx, targetImageID, phone, query)
	} else {
		result, err = s.CalculateFromText(ctx, query)
	}

	if err != nil {
		logger.Log.Error().Err(err).Msg("failed calculating calories")
		errorMsg := "Maaf, terjadi kendala saat menghitung kalori. Silakan coba beberapa saat lagi."
		return s.gowaClient.SendText(ctx, s.deviceID, chatID, errorMsg, messageID)
	}

	return s.gowaClient.SendText(ctx, s.deviceID, chatID, result, messageID)
}
