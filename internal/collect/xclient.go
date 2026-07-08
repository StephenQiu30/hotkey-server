package collect

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Tweet represents a parsed tweet from the X API stream.
type Tweet struct {
	ID           string `json:"id"`
	Text         string `json:"text"`
	AuthorID     string `json:"author_id"`
	AuthorName   string `json:"author_name,omitempty"`
	AuthorHandle string `json:"author_handle,omitempty"`
	CreatedAt    string `json:"created_at,omitempty"`
}

// StreamRule represents a Filtered Stream rule.
type StreamRule struct {
	ID    string `json:"id,omitempty"`
	Value string `json:"value"`
	Tag   string `json:"tag,omitempty"`
}

// XClient manages the X API v2 Filtered Stream connection.
type XClient struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewXClient creates an X API client.
func NewXClient(baseURL, token string) *XClient {
	return &XClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		token:      token,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// SetRules replaces all Filtered Stream rules with the given ones.
func (c *XClient) SetRules(ctx context.Context, rules []StreamRule) error {
	// Delete existing rules
	existing, err := c.getRules(ctx)
	if err != nil {
		return fmt.Errorf("get existing rules: %w", err)
	}
	if len(existing) > 0 {
		ids := make([]string, len(existing))
		for i, r := range existing {
			ids[i] = r.ID
		}
		delPayload, _ := json.Marshal(map[string]interface{}{
			"delete": map[string]interface{}{
				"ids": ids,
			},
		})
		req, err := http.NewRequestWithContext(ctx, http.MethodPost,
			c.baseURL+"/2/tweets/search/stream/rules",
			strings.NewReader(string(delPayload)),
		)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+c.token)
		req.Header.Set("Content-Type", "application/json")
		resp, err := c.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("delete rules: %w", err)
		}
		resp.Body.Close()
	}

	// Add new rules
	if len(rules) == 0 {
		return nil
	}
	addPayload, _ := json.Marshal(map[string]interface{}{
		"add": rules,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/2/tweets/search/stream/rules",
		strings.NewReader(string(addPayload)),
	)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("add rules: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("add rules failed: status=%d body=%s", resp.StatusCode, string(body))
	}
	return nil
}

// ConnectStream establishes a Filtered Stream connection.
func (c *XClient) ConnectStream(ctx context.Context) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.baseURL+"/2/tweets/search/stream?expansions=author_id&tweet.fields=created_at&user.fields=name,username",
		nil,
	)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("connect stream: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("stream connection failed: status=%d", resp.StatusCode)
	}
	return resp.Body, nil
}

// ParseTweet parses a single JSON line from the stream into a Tweet.
func ParseTweet(data []byte) (*Tweet, error) {
	var raw struct {
		Data struct {
			ID        string `json:"id"`
			Text      string `json:"text"`
			AuthorID  string `json:"author_id"`
			CreatedAt string `json:"created_at"`
		} `json:"data"`
		Includes struct {
			Users []struct {
				ID       string `json:"id"`
				Name     string `json:"name"`
				Username string `json:"username"`
			} `json:"users"`
		} `json:"includes"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("unmarshal tweet: %w", err)
	}
	if raw.Data.ID == "" {
		return nil, fmt.Errorf("empty tweet data")
	}
	tweet := &Tweet{
		ID:        raw.Data.ID,
		Text:      raw.Data.Text,
		AuthorID:  raw.Data.AuthorID,
		CreatedAt: raw.Data.CreatedAt,
	}
	for _, u := range raw.Includes.Users {
		if u.ID == raw.Data.AuthorID {
			tweet.AuthorName = u.Name
			tweet.AuthorHandle = u.Username
			break
		}
	}
	return tweet, nil
}

// ReadStream reads one tweet line from the scanner. Empty lines are keepalives.
func ReadStream(scanner *bufio.Scanner) (*Tweet, error) {
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		return ParseTweet([]byte(line))
	}
	return nil, scanner.Err()
}

func (c *XClient) getRules(ctx context.Context) ([]StreamRule, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.baseURL+"/2/tweets/search/stream/rules", nil,
	)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Data []StreamRule `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, nil
	}
	return result.Data, nil
}
