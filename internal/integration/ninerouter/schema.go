package ninerouter

import (
	"encoding/json"
	"fmt"
	"strings"

	apperrors "github.com/ramadiaz/whatsapp-mt-connector/internal/shared/errors"
)

type AIExtractionResult struct {
	Intent           string   `json:"intent"`
	Type             *string  `json:"type"`
	Amount           *float64 `json:"amount"`
	CurrencyCode     string   `json:"currency_code"`
	CategoryHint     *string  `json:"category_hint"`
	AccountHint      *string  `json:"account_hint"`
	Date             *string  `json:"date"`
	Remark           *string  `json:"remark"`
	Confidence       float64  `json:"confidence"`
	NeedsConfirmation bool    `json:"needs_confirmation"`
	MissingFields    []string `json:"missing_fields"`
}

var allowedIntents = map[string]bool{
	"create_transaction": true,
	"clarification":      true,
	"help":               true,
	"unsupported":        true,
}

var allowedTypes = map[string]bool{
	"income":  true,
	"expense": true,
}

func ParseAndValidate(raw string) (*AIExtractionResult, error) {
	raw = strings.TrimSpace(raw)

	if strings.HasPrefix(raw, "```") {
		lines := strings.Split(raw, "\n")
		var inner []string
		for _, line := range lines {
			if strings.HasPrefix(line, "```") {
				continue
			}
			inner = append(inner, line)
		}
		raw = strings.Join(inner, "\n")
	}

	dec := json.NewDecoder(strings.NewReader(raw))
	dec.DisallowUnknownFields()

	var result AIExtractionResult
	if err := dec.Decode(&result); err != nil {
		return nil, fmt.Errorf("%w: %s", apperrors.ErrInvalidAIResponse, err.Error())
	}

	if !allowedIntents[result.Intent] {
		return nil, fmt.Errorf("%w: unknown intent %q", apperrors.ErrInvalidAIResponse, result.Intent)
	}

	if result.Type != nil && *result.Type != "" && !allowedTypes[*result.Type] {
		return nil, fmt.Errorf("%w: unknown type %q", apperrors.ErrInvalidAIResponse, *result.Type)
	}

	if result.Intent == "create_transaction" {
		if result.Amount == nil || *result.Amount <= 0 {
			return nil, fmt.Errorf("%w: amount must be > 0", apperrors.ErrInvalidAIResponse)
		}
	}

	return &result, nil
}

type WastefulAnalysisResult struct {
	Wasteful bool   `json:"wasteful"`
	Reason   string `json:"reason"`
}

func ParseWasteful(raw string) (*WastefulAnalysisResult, error) {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "```") {
		lines := strings.Split(raw, "\n")
		var inner []string
		for _, line := range lines {
			if strings.HasPrefix(line, "```") {
				continue
			}
			inner = append(inner, line)
		}
		raw = strings.Join(inner, "\n")
	}

	var result WastefulAnalysisResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return nil, fmt.Errorf("parse wasteful: %w", err)
	}
	return &result, nil
}

