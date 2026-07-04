package moneytracker

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/ramadiaz/whatsapp-mt-connector/internal/domain/transaction"
	apperrors "github.com/ramadiaz/whatsapp-mt-connector/internal/shared/errors"
)

func (c *Client) AddTransaction(ctx context.Context, req transaction.CreateTransactionRequest) (*transaction.CreatedTransaction, error) {
	form := url.Values{}
	form.Set("type", fmt.Sprintf("%d", int(req.Type)))
	form.Set("amount", req.Amount.String())
	form.Set("category_id", req.CategoryID)
	form.Set("date", req.Date)

	if req.AccountID != "" {
		form.Set("account_id", req.AccountID)
	}
	if req.Remark != "" {
		form.Set("remark", req.Remark)
	}
	if req.CurrencyCode != "" {
		form.Set("currency_code", req.CurrencyCode)
	}

	result, err := c.post(ctx, "/addTransaction", form)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", apperrors.ErrMoneyTrackerUnavailable, err.Error())
	}

	if result.Status != 1 {
		return nil, fmt.Errorf("%w: %s", apperrors.ErrMoneyTrackerRejected, result.Msg)
	}

	var data struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(result.Data, &data); err != nil {
		return nil, fmt.Errorf("moneytracker add transaction decode: %w", err)
	}

	return &transaction.CreatedTransaction{ID: data.ID}, nil
}
