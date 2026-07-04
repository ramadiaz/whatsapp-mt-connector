package ninerouter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	endpoint   string
	apiKey     string
	model      string
	visionModel string
	httpClient *http.Client
}

func NewClient(endpoint, apiKey, model, visionModel string, timeout time.Duration) (*Client, error) {
	c := &Client{
		endpoint:    strings.TrimRight(endpoint, "/"),
		apiKey:      apiKey,
		model:       model,
		visionModel: visionModel,
		httpClient:  &http.Client{Timeout: timeout},
	}

	if err := c.validateModel(context.Background(), model); err != nil {
		return nil, fmt.Errorf("9router model validation: %w", err)
	}

	return c, nil
}

func (c *Client) validateModel(ctx context.Context, model string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint+"/models", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetch models: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode models: %w", err)
	}

	for _, m := range result.Data {
		if m.ID == model {
			return nil
		}
	}
	return fmt.Errorf("model %q not found in 9Router", model)
}

type Message struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"`
}

type TextContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type ImageContent struct {
	Type     string    `json:"type"`
	ImageURL ImageURL  `json:"image_url"`
}

type ImageURL struct {
	URL string `json:"url"`
}

func (c *Client) Complete(ctx context.Context, model, systemPrompt string, userMessages []Message, maxTokens int) (string, error) {
	messages := []Message{
		{Role: "system", Content: systemPrompt},
	}
	messages = append(messages, userMessages...)

	body := map[string]interface{}{
		"model":    model,
		"messages": messages,
		"stream":   false,
	}
	if maxTokens > 0 {
		body["max_tokens"] = maxTokens
	}

	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+"/chat/completions", bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("9router complete: %w", err)
	}
	defer resp.Body.Close()

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("9router read: %w", err)
	}

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("9router http %d: %s", resp.StatusCode, string(rawBody))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(rawBody, &result); err != nil {
		return "", fmt.Errorf("9router decode: %w", err)
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("9router: no choices in response")
	}

	return result.Choices[0].Message.Content, nil
}

func (c *Client) Model() string {
	return c.model
}

func (c *Client) VisionModel() string {
	if c.visionModel != "" {
		return c.visionModel
	}
	return c.model
}
