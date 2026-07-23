package ninerouter

import (
	"encoding/json"
	"fmt"
	"strings"

	apperrors "github.com/ramadiaz/whatsapp-mt-connector/internal/shared/errors"
)

type TransactionItem struct {
	Type              *string  `json:"type"`
	Amount            *float64 `json:"amount"`
	CurrencyCode      string   `json:"currency_code"`
	CategoryHint      *string  `json:"category_hint"`
	AccountHint       *string  `json:"account_hint"`
	Date              *string  `json:"date"`
	Remark            *string  `json:"remark"`
	Confidence        float64  `json:"confidence"`
	NeedsConfirmation bool     `json:"needs_confirmation"`
	MissingFields     []string `json:"missing_fields"`
	IsWasteful        *bool    `json:"is_wasteful"`
	WastefulReason    *string  `json:"wasteful_reason"`
}

type AIExtractionResult struct {
	Intent            string            `json:"intent"`
	Transactions      []TransactionItem `json:"transactions"`
	Type              *string           `json:"type,omitempty"`
	Amount            *float64          `json:"amount,omitempty"`
	CurrencyCode      string            `json:"currency_code,omitempty"`
	CategoryHint      *string           `json:"category_hint,omitempty"`
	AccountHint       *string           `json:"account_hint,omitempty"`
	Date              *string           `json:"date,omitempty"`
	Remark            *string           `json:"remark,omitempty"`
	Confidence        float64           `json:"confidence,omitempty"`
	NeedsConfirmation bool              `json:"needs_confirmation,omitempty"`
	MissingFields     []string          `json:"missing_fields,omitempty"`
	IsWasteful        *bool             `json:"is_wasteful,omitempty"`
	WastefulReason    *string           `json:"wasteful_reason,omitempty"`
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

	if len(result.Transactions) == 0 {
		if result.Type != nil || result.Amount != nil || result.CategoryHint != nil || result.Intent == "create_transaction" {
			singleItem := TransactionItem{
				Type:              result.Type,
				Amount:            result.Amount,
				CurrencyCode:      result.CurrencyCode,
				CategoryHint:      result.CategoryHint,
				AccountHint:       result.AccountHint,
				Date:              result.Date,
				Remark:            result.Remark,
				Confidence:        result.Confidence,
				NeedsConfirmation: result.NeedsConfirmation,
				MissingFields:     result.MissingFields,
				IsWasteful:        result.IsWasteful,
				WastefulReason:    result.WastefulReason,
			}
			result.Transactions = []TransactionItem{singleItem}
		}
	}

	for i := range result.Transactions {
		tx := &result.Transactions[i]
		if tx.Type != nil && *tx.Type != "" && !allowedTypes[*tx.Type] {
			return nil, fmt.Errorf("%w: unknown type %q", apperrors.ErrInvalidAIResponse, *tx.Type)
		}
		if result.Intent == "create_transaction" {
			if tx.Amount == nil || *tx.Amount <= 0 {
				return nil, fmt.Errorf("%w: amount must be > 0", apperrors.ErrInvalidAIResponse)
			}
		}
	}

	return &result, nil
}

