package fetcher

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// BiliBiliConfig holds configuration for the BiliBili fetcher.
type BiliBiliConfig struct{}

// BiliBiliFetcher implements Fetcher for Bilibili (B站) content.
// It supports video list (popular/space) and dynamic feed APIs.
type BiliBiliFetcher struct {
	client *http.Client
}

// NewBiliBiliFetcher creates a new BiliBiliFetcher.
func NewBiliBiliFetcher(client *http.Client, cfg BiliBiliConfig) *BiliBiliFetcher {
	return &BiliBiliFetcher{
		client: httpClient(client),
	}
}

func (f *BiliBiliFetcher) Fetch(ctx context.Context, source Source) ([]Item, error) {
	if source.Type != SourceTypeBiliBili {
		return nil, errors.New("bilibili fetcher requires bilibili source")
	}

	if isDynamicURL(source.URL) {
		return f.fetchDynamics(ctx, source)
	}
	return f.fetchVideos(ctx, source)
}

// fetchVideos handles video list APIs (popular, space/search, ranking).
func (f *BiliBiliFetcher) fetchVideos(ctx context.Context, source Source) ([]Item, error) {
	body, err := fetchBody(ctx, f.client, source.URL)
	if err != nil {
		return nil, err
	}
	defer body.Close()

	var resp biliVideoListResponse
	if err := json.NewDecoder(body).Decode(&resp); err != nil {
		return nil, fmt.Errorf("parse bilibili response: %w", err)
	}
	if err := checkBiliCode(resp.Code, resp.Message); err != nil {
		if errors.Is(err, errBiliTakedown) {
			return nil, nil // takedowns are filtered out, not a failure
		}
		return nil, err
	}

	videos, err := parseVideoList(resp.Data.List)
	if err != nil {
		return nil, err
	}
	if len(videos) == 0 {
		return []Item{}, nil
	}

	seen := make(map[string]struct{})
	items := make([]Item, 0, len(videos))
	for _, v := range videos {
		if v.Title == "" {
			continue
		}
		if _, exists := seen[v.BVID]; exists {
			continue
		}
		seen[v.BVID] = struct{}{}

		published := time.Unix(v.PubDate, 0).UTC()
		snippet := strings.TrimSpace(v.Desc)
		if snippet == "" {
			snippet = strings.TrimSpace(v.Description)
		}

		item := Item{
			Title:       strings.TrimSpace(v.Title),
			URL:         fmt.Sprintf("https://www.bilibili.com/video/%s", v.BVID),
			ExternalID:  v.BVID,
			PublishedAt: &published,
			Snippet:     snippet,
			Score:       v.Stat.View,
			Descendants: v.Stat.Reply,
		}
		items = append(items, item)
	}
	return items, nil
}

// fetchDynamics handles dynamic feed APIs.
func (f *BiliBiliFetcher) fetchDynamics(ctx context.Context, source Source) ([]Item, error) {
	body, err := fetchBody(ctx, f.client, source.URL)
	if err != nil {
		return nil, err
	}
	defer body.Close()

	var resp biliDynamicResponse
	if err := json.NewDecoder(body).Decode(&resp); err != nil {
		return nil, fmt.Errorf("parse bilibili dynamic response: %w", err)
	}
	if err := checkBiliCode(resp.Code, resp.Message); err != nil {
		if errors.Is(err, errBiliTakedown) {
			return nil, nil // takedowns are filtered out, not a failure
		}
		return nil, err
	}

	items := make([]Item, 0, len(resp.Data.Items))
	for _, d := range resp.Data.Items {
		desc := d.Modules.ModuleDynamic.Desc
		if desc == nil || strings.TrimSpace(desc.Text) == "" {
			continue
		}
		published := time.Unix(d.Modules.ModuleAuthor.PubTs, 0).UTC()

		text := strings.TrimSpace(desc.Text)
		item := Item{
			Title:       truncate(text, 80),
			URL:         fmt.Sprintf("https://www.bilibili.com/dynamic/%s", d.IDStr),
			ExternalID:  fmt.Sprintf("dyn_%s", d.IDStr),
			PublishedAt: &published,
			Snippet:     text,
		}
		items = append(items, item)
	}
	return items, nil
}

// isDynamicURL checks if the URL targets a dynamic/feed API.
func isDynamicURL(rawURL string) bool {
	return strings.Contains(rawURL, "/polymer/web-dynamic/")
}

// errBiliTakedown is returned when the Bilibili API reports content unavailable (-404).
// Callers should treat this as an empty result (filter out), not a hard failure.
var errBiliTakedown = errors.New("bilibili content unavailable")

// checkBiliCode checks the Bilibili API response code.
// -404: content unavailable/takedown → returns errBiliTakedown
// -412: rate limit → returns error
// Other non-zero: generic error
func checkBiliCode(code int, message string) error {
	switch {
	case code == 0:
		return nil
	case code == -404:
		return errBiliTakedown
	case code == -412:
		return fmt.Errorf("bilibili rate limit: %s", message)
	default:
		return fmt.Errorf("bilibili API error %d: %s", code, message)
	}
}

// parseVideoList handles two Bilibili API response formats:
// - Popular API: data.list is a direct array of videos
// - Space API: data.list is an object with a "vlist" array
func parseVideoList(raw json.RawMessage) ([]biliVideo, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}

	// Try as direct array first (popular API)
	var direct []biliVideo
	if err := json.Unmarshal(raw, &direct); err == nil {
		return direct, nil
	}

	// Try as object with vlist (space API)
	var wrapper struct {
		Vlist []biliVideo `json:"vlist"`
	}
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		return nil, fmt.Errorf("parse bilibili video list: %w", err)
	}
	return wrapper.Vlist, nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "…"
}

// --- Bilibili API response types ---

type biliVideoListResponse struct {
	Code    int                `json:"code"`
	Message string             `json:"message"`
	Data    biliVideoListData  `json:"data"`
}

type biliVideoListData struct {
	List json.RawMessage `json:"list"`
}

type biliVideo struct {
	BVID        string   `json:"bvid"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Desc        string   `json:"desc"`
	Author      string   `json:"author"`
	PubDate     int64    `json:"pubdate"`
	Stat        biliStat `json:"stat"`
}

type biliStat struct {
	View  int `json:"view"`
	Like  int `json:"like"`
	Coin  int `json:"coin"`
	Share int `json:"share"`
	Reply int `json:"reply"`
}

type biliDynamicResponse struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    biliDynamicData `json:"data"`
}

type biliDynamicData struct {
	Items []biliDynamicItem `json:"items"`
}

type biliDynamicItem struct {
	IDStr   string             `json:"id_str"`
	Type    string             `json:"type"`
	Modules biliDynamicModules `json:"modules"`
}

type biliDynamicModules struct {
	ModuleDynamic biliModuleDynamic `json:"module_dynamic"`
	ModuleAuthor  biliModuleAuthor  `json:"module_author"`
}

type biliModuleDynamic struct {
	Desc *biliDesc `json:"desc"`
}

type biliDesc struct {
	Text string `json:"text"`
}

type biliModuleAuthor struct {
	Name  string `json:"name"`
	PubTs int64  `json:"pub_ts"`
}
