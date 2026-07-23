package moneytracker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ramadiaz/whatsapp-mt-connector/internal/domain/transaction"
)

type MoneyTrackerClient interface {
	GetCategories(ctx context.Context) ([]transaction.Category, error)
	GetAccounts(ctx context.Context) ([]transaction.Account, error)
	AddTransaction(ctx context.Context, req transaction.CreateTransactionRequest) (*transaction.CreatedTransaction, error)
	GetTransactions(ctx context.Context, limit int) ([]transaction.MTTransaction, error)
	GetTransactionsDateRange(ctx context.Context, startDate, endDate string) ([]transaction.MTTransaction, error)
}

type mtResponse struct {
	Status int             `json:"status"`
	Msg    string          `json:"msg"`
	Data   json.RawMessage `json:"data"`
}

type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

func NewClient(baseURL, apiKey string, timeout time.Duration) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

func (c *Client) post(ctx context.Context, path string, form url.Values) (*mtResponse, error) {
	form.Set("token", c.apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("moneytracker post %s: %w", path, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("moneytracker read body: %w", err)
	}

	var result mtResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("moneytracker decode: %w", err)
	}

	return &result, nil
}
