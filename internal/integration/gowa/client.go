package gowa

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type WhatsAppGateway interface {
	SendText(ctx context.Context, deviceID, chatID, message string, replyToID string) error
	DownloadMessageMedia(ctx context.Context, deviceID, messageID, phone string) ([]byte, string, error)
	Health(ctx context.Context) error
}

type Client struct {
	baseURL    string
	username   string
	httpClient *http.Client
	password   string
}

func NewClient(baseURL, username, password string, timeout time.Duration) *Client {
	return &Client{
		baseURL:  strings.TrimRight(baseURL, "/"),
		username: username,
		password: password,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

func (c *Client) newRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(c.username, c.password)
	return req, nil
}

func (c *Client) SendText(ctx context.Context, deviceID, chatID, message string, replyToID string) error {
	payload := map[string]interface{}{
		"phone":   chatID,
		"message": message,
	}
	if replyToID != "" {
		payload["reply_message_id"] = replyToID
	}

	b, _ := json.Marshal(payload)
	req, err := c.newRequest(ctx, http.MethodPost, "/send/message", strings.NewReader(string(b)))
	if err != nil {
		return fmt.Errorf("gowa send text: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Device-Id", deviceID)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("gowa send text: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("gowa send text: http %d", resp.StatusCode)
	}
	return nil
}

func (c *Client) DownloadMessageMedia(ctx context.Context, deviceID, messageID, phone string) ([]byte, string, error) {
	path := fmt.Sprintf("/message/%s/download?phone=%s", messageID, phone)
	req, err := c.newRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, "", fmt.Errorf("gowa download media: %w", err)
	}
	req.Header.Set("X-Device-Id", deviceID)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("gowa download media: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, "", fmt.Errorf("gowa download media: http %d", resp.StatusCode)
	}

	var result struct {
		Results struct {
			FileURL   string `json:"file_url"`
			MediaType string `json:"media_type"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, "", fmt.Errorf("gowa download media decode: %w", err)
	}

	if result.Results.FileURL == "" {
		return nil, "", fmt.Errorf("gowa download media: empty file_url")
	}

	fileReq, err := c.newRequest(ctx, http.MethodGet, "", nil)
	if err != nil {
		return nil, "", err
	}
	fileReq.URL, err = fileReq.URL.Parse(result.Results.FileURL)
	if err != nil {
		return nil, "", fmt.Errorf("gowa parse file url: %w", err)
	}

	fileResp, err := c.httpClient.Do(fileReq)
	if err != nil {
		return nil, "", fmt.Errorf("gowa fetch file: %w", err)
	}
	defer fileResp.Body.Close()

	data, err := io.ReadAll(fileResp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("gowa read file: %w", err)
	}

	mimeType := fileResp.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = result.Results.MediaType
	}

	return data, mimeType, nil
}

func (c *Client) Health(ctx context.Context) error {
	req, err := c.newRequest(ctx, http.MethodGet, "/health", nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("gowa health: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("gowa health: http %d", resp.StatusCode)
	}
	return nil
}
