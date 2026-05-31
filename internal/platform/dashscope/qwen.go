package dashscope

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	servicereport "github.com/StephenQiu30/hotkey-server/internal/service/report"
)

type QwenConfig struct {
	APIKey  string
	BaseURL string
	Model   string
}

type QwenClient struct {
	cfg    QwenConfig
	client *http.Client
}

func NewQwenClient(cfg QwenConfig, client *http.Client) *QwenClient {
	if client == nil {
		client = http.DefaultClient
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://dashscope.aliyuncs.com/compatible-mode/v1"
	}
	if cfg.Model == "" {
		cfg.Model = "qwen-plus"
	}
	return &QwenClient{cfg: cfg, client: client}
}

func (c *QwenClient) GenerateReport(ctx context.Context, prompt string) (string, error) {
	if strings.TrimSpace(c.cfg.APIKey) == "" {
		return "", servicereport.ErrFailedConfig
	}
	payload := qwenRequest{
		Model: c.cfg.Model,
		Messages: []qwenMessage{
			{Role: "system", Content: "你是可靠的中文日报编辑，只能基于来源证据输出中文。"},
			{Role: "user", Content: prompt},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	endpoint := strings.TrimRight(c.cfg.BaseURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("dashscope status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	var parsed qwenResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return "", err
	}
	if len(parsed.Choices) == 0 || strings.TrimSpace(parsed.Choices[0].Message.Content) == "" {
		return "", errors.New("dashscope empty response")
	}
	return parsed.Choices[0].Message.Content, nil
}

type qwenRequest struct {
	Model    string        `json:"model"`
	Messages []qwenMessage `json:"messages"`
}

type qwenMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type qwenResponse struct {
	Choices []struct {
		Message qwenMessage `json:"message"`
	} `json:"choices"`
}
