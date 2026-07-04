package moneytracker

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"github.com/ramadiaz/money-wa-bot/internal/domain/transaction"
)

func (c *Client) GetAccounts(ctx context.Context) ([]transaction.Account, error) {
	result, err := c.post(ctx, "/getAccounts", url.Values{})
	if err != nil {
		return nil, fmt.Errorf("moneytracker get accounts: %w", err)
	}
	if result.Status != 1 {
		return nil, fmt.Errorf("moneytracker get accounts: %s", result.Msg)
	}

	var raw []struct {
		ID           string `json:"id"`
		Name         string `json:"name"`
		CurrencyCode string `json:"currency_code"`
	}
	if err := json.Unmarshal(result.Data, &raw); err != nil {
		return nil, fmt.Errorf("moneytracker get accounts decode: %w", err)
	}

	now := time.Now()
	accounts := make([]transaction.Account, len(raw))
	for i, r := range raw {
		accounts[i] = transaction.Account{
			AccountID:    r.ID,
			Name:         r.Name,
			CurrencyCode: r.CurrencyCode,
			RefreshedAt:  now,
		}
	}
	return accounts, nil
}
