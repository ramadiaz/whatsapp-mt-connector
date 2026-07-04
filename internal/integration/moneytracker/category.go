package moneytracker

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/ramadiaz/whatsapp-mt-connector/internal/domain/transaction"
)

func (c *Client) GetCategories(ctx context.Context) ([]transaction.Category, error) {
	result, err := c.post(ctx, "/getCategories", url.Values{})
	if err != nil {
		return nil, fmt.Errorf("moneytracker get categories: %w", err)
	}
	if result.Status != 1 {
		return nil, fmt.Errorf("moneytracker get categories: %s", result.Msg)
	}

	var raw []struct {
		ID    string      `json:"id"`
		Title string      `json:"title"`
		Type  json.Number `json:"type"`
	}
	if err := json.Unmarshal(result.Data, &raw); err != nil {
		return nil, fmt.Errorf("moneytracker get categories decode: %w", err)
	}

	now := time.Now()
	cats := make([]transaction.Category, len(raw))
	for i, r := range raw {
		t, _ := strconv.Atoi(r.Type.String())
		cats[i] = transaction.Category{
			CategoryID:  r.ID,
			Title:       r.Title,
			Type:        t,
			RefreshedAt: now,
		}
	}
	return cats, nil
}
