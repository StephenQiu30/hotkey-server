// Package preset owns the server-side source preset catalog and resolves a
// selected preset into a typed SourceConnection. Endpoint templates and fixed
// policy settings never need to be duplicated by clients.
package preset

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
)

type Cost string

const (
	CostFree         Cost = "free"
	CostPaid         Cost = "paid"
	CostCredentialed Cost = "credentialed"
)

type Input struct {
	Key         string
	Label       string
	Placeholder string
	Required    bool
	MaxLength   int
}

type Definition struct {
	ID                 string
	Label              string
	Description        string
	SourceType         domain.SourceType
	AuthLabel          string
	Cost               Cost
	CredentialRequired bool
	Inputs             []Input
}

type resolver func(values map[string]string, config domain.SourceConfig) (string, error)

type configuredPreset struct {
	definition     Definition
	authType       domain.AuthType
	enabled        bool
	termsPolicyURL string
	resolve        resolver
}

var (
	youtubeChannelIDPattern = regexp.MustCompile(`^UC[A-Za-z0-9_-]{22}$`)
	githubRepositoryPattern = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,38})/[A-Za-z0-9._-]{1,100}$`)
	mastodonInstancePattern = regexp.MustCompile(`(?i)^(?:[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?\.)+[A-Za-z]{2,63}$`)
	mastodonUsernamePattern = regexp.MustCompile(`^[A-Za-z0-9_]{1,64}$`)
	mastodonHashtagPattern  = regexp.MustCompile(`^[^\s/#?]{1,100}$`)
)

var presets = []configuredPreset{
	{
		definition: Definition{ID: "rss_custom", Label: "RSS / Atom 地址", Description: "连接公开的 HTTPS RSS 或 Atom Feed。", SourceType: domain.SourceTypeRSS, AuthLabel: "无需授权", Cost: CostFree, Inputs: []Input{{Key: "endpoint", Label: "Feed 地址", Placeholder: "https://example.com/feed.xml", Required: true, MaxLength: 2048}}},
		authType:   domain.AuthTypeNone, enabled: true,
		resolve: valueResolver("endpoint"),
	},
	{
		definition: Definition{ID: "youtube_channel", Label: "YouTube 频道（免费）", Description: "使用 YouTube 官方公开频道 Feed。", SourceType: domain.SourceTypeRSS, AuthLabel: "无需密钥", Cost: CostFree, Inputs: []Input{{Key: "youtube_channel_id", Label: "YouTube 频道 ID", Placeholder: "UC…（以 UC 开头）", Required: true, MaxLength: 24}}},
		authType:   domain.AuthTypeNone, enabled: true,
		resolve: func(values map[string]string, _ domain.SourceConfig) (string, error) {
			channelID := strings.TrimSpace(values["youtube_channel_id"])
			if !youtubeChannelIDPattern.MatchString(channelID) {
				return "", fmt.Errorf("invalid YouTube channel ID")
			}
			return "https://www.youtube.com/feeds/videos.xml?channel_id=" + channelID, nil
		},
	},
	{
		definition: Definition{ID: "github_releases", Label: "GitHub Releases（免费）", Description: "使用 GitHub 仓库官方 Releases Atom Feed。", SourceType: domain.SourceTypeRSS, AuthLabel: "无需密钥", Cost: CostFree, Inputs: []Input{{Key: "github_repository", Label: "GitHub 仓库", Placeholder: "owner/repository", Required: true, MaxLength: 140}}},
		authType:   domain.AuthTypeNone, enabled: true,
		resolve: func(values map[string]string, _ domain.SourceConfig) (string, error) {
			repository := strings.TrimSpace(values["github_repository"])
			if !githubRepositoryPattern.MatchString(repository) {
				return "", fmt.Errorf("invalid GitHub repository")
			}
			return "https://github.com/" + repository + "/releases.atom", nil
		},
	},
	{
		definition: Definition{ID: "arxiv_search", Label: "arXiv 关键词（免费）", Description: "使用 arXiv 官方 Atom API 订阅关键词。", SourceType: domain.SourceTypeRSS, AuthLabel: "无需密钥", Cost: CostFree, Inputs: []Input{{Key: "arxiv_query", Label: "arXiv 关键词", Placeholder: "例如：large language model", Required: true, MaxLength: 200}}},
		authType:   domain.AuthTypeNone, enabled: true,
		resolve: func(values map[string]string, _ domain.SourceConfig) (string, error) {
			query := strings.TrimSpace(values["arxiv_query"])
			if query == "" || len([]rune(query)) > 200 {
				return "", fmt.Errorf("invalid arXiv query")
			}
			encoded := strings.ReplaceAll(url.QueryEscape(query), "+", "%20")
			return "https://export.arxiv.org/api/query?search_query=all%3A" + encoded + "&start=0&max_results=100", nil
		},
	},
	{
		definition: Definition{ID: "mastodon_account", Label: "Mastodon 账号（免费）", Description: "使用实例公开的账号 RSS Feed。", SourceType: domain.SourceTypeRSS, AuthLabel: "无需密钥", Cost: CostFree, Inputs: mastodonInputs("Mastodon 用户名", "username")},
		authType:   domain.AuthTypeNone, enabled: true, resolve: mastodonResolver(true),
	},
	{
		definition: Definition{ID: "mastodon_hashtag", Label: "Mastodon 标签（免费）", Description: "使用实例公开的标签 RSS Feed。", SourceType: domain.SourceTypeRSS, AuthLabel: "无需密钥", Cost: CostFree, Inputs: mastodonInputs("Mastodon 标签", "opensource")},
		authType:   domain.AuthTypeNone, enabled: true, resolve: mastodonResolver(false),
	},
	{
		definition: Definition{ID: "hacker_news", Label: "Hacker News", Description: "使用 Hacker News 官方 Firebase API。", SourceType: domain.SourceTypeHackerNews, AuthLabel: "无需授权", Cost: CostFree},
		authType:   domain.AuthTypeNone, enabled: true, resolve: fixedResolver(domain.HackerNewsEndpoint),
	},
	{
		definition: Definition{ID: "x", Label: "X / Twitter（官方付费）", Description: "X Recent Search 官方 API；需要 Bearer Token 和可用额度。", SourceType: domain.SourceTypeX, AuthLabel: "Bearer Token", Cost: CostPaid, CredentialRequired: true},
		authType:   domain.AuthTypeBearer, enabled: false, resolve: fixedResolver(domain.XRecentSearchEndpoint),
	},
	{
		definition: Definition{ID: "bing_grounding", Label: "Bing Grounding", Description: "已授权的 Foundry Web Search Toolbox MCP。", SourceType: domain.SourceTypeBingGrounding, AuthLabel: "Bearer Token", Cost: CostCredentialed, CredentialRequired: true, Inputs: []Input{{Key: "endpoint", Label: "接口地址", Placeholder: "https://…/api/projects/…/toolboxes/…/versions/…/mcp?api-version=v1", Required: true, MaxLength: 2048}}},
		authType:   domain.AuthTypeBearer, enabled: false, termsPolicyURL: "https://learn.microsoft.com/en-us/azure/ai-foundry/agents/how-to/tools/web-search?view=foundry", resolve: valueResolver("endpoint"),
	},
	{
		definition: Definition{ID: "bilibili", Label: "Bilibili", Description: "哔哩哔哩开放平台官方接口。", SourceType: domain.SourceTypeBilibili, AuthLabel: "OAuth 2.0", Cost: CostCredentialed, CredentialRequired: true},
		authType:   domain.AuthTypeOAuth2, enabled: false, termsPolicyURL: "https://openhome.bilibili.com/agreement/privacy-policy", resolve: fixedResolver(domain.BilibiliOpenEndpoint),
	},
	{
		definition: Definition{ID: "weibo", Label: "Weibo", Description: "微博开放平台官方接口。", SourceType: domain.SourceTypeWeibo, AuthLabel: "Bearer Token", Cost: CostCredentialed, CredentialRequired: true},
		authType:   domain.AuthTypeBearer, enabled: false, termsPolicyURL: domain.WeiboDeveloperTerms, resolve: fixedResolver(domain.WeiboCLIApiEndpoint),
	},
	{
		definition: Definition{ID: "google_agent_search", Label: "Google Agent Search", Description: "Google Discovery Engine 官方接口。", SourceType: domain.SourceTypeGoogleAgentSearch, AuthLabel: "Bearer Token", Cost: CostCredentialed, CredentialRequired: true},
		authType:   domain.AuthTypeBearer, enabled: false, termsPolicyURL: domain.GoogleCloudTerms,
		resolve: func(_ map[string]string, config domain.SourceConfig) (string, error) {
			switch config.GoogleLocation {
			case "global":
				return domain.GoogleAgentSearchGlobalEndpoint, nil
			case "us":
				return domain.GoogleAgentSearchUSEndpoint, nil
			case "eu":
				return domain.GoogleAgentSearchEUEndpoint, nil
			default:
				return "", fmt.Errorf("invalid Google location")
			}
		},
	},
}

func Catalog() []Definition {
	result := make([]Definition, 0, len(presets))
	for _, item := range presets {
		definition := item.definition
		definition.Inputs = append([]Input(nil), item.definition.Inputs...)
		result = append(result, definition)
	}
	return result
}

func Resolve(id, name string, values map[string]string, config domain.SourceConfig) (domain.SourceConnection, error) {
	for _, item := range presets {
		if item.definition.ID != strings.TrimSpace(id) {
			continue
		}
		if err := validateValues(item.definition.Inputs, values); err != nil {
			return domain.SourceConnection{}, err
		}
		endpoint, err := item.resolve(values, config)
		if err != nil {
			return domain.SourceConnection{}, err
		}
		config = presetConfig(item.definition.SourceType, config)
		return domain.SourceConnection{SourceType: item.definition.SourceType, Name: name, Endpoint: endpoint, AuthType: item.authType, Config: config, Enabled: item.enabled, TermsPolicyURL: item.termsPolicyURL}, nil
	}
	return domain.SourceConnection{}, fmt.Errorf("unknown source preset")
}

func validateValues(inputs []Input, values map[string]string) error {
	allowed := make(map[string]Input, len(inputs))
	for _, input := range inputs {
		allowed[input.Key] = input
		if input.Required && strings.TrimSpace(values[input.Key]) == "" {
			return fmt.Errorf("missing preset value %q", input.Key)
		}
	}
	for key, value := range values {
		input, ok := allowed[key]
		if !ok {
			return fmt.Errorf("unknown preset value %q", key)
		}
		if input.MaxLength > 0 && len([]rune(value)) > input.MaxLength {
			return fmt.Errorf("preset value %q is too long", key)
		}
	}
	return nil
}

func presetConfig(sourceType domain.SourceType, config domain.SourceConfig) domain.SourceConfig {
	switch sourceType {
	case domain.SourceTypeBingGrounding:
		config.AllowBodyStorage, config.RequiresAttribution, config.MaxPagesPerRun = true, true, 1
	case domain.SourceTypeBilibili, domain.SourceTypeWeibo:
		config.AllowBodyStorage, config.RequiresAttribution, config.RequiresDeletionSync = true, true, true
	case domain.SourceTypeGoogleAgentSearch:
		config.AllowBodyStorage, config.RequiresAttribution = true, true
	}
	return config
}

func valueResolver(key string) resolver {
	return func(values map[string]string, _ domain.SourceConfig) (string, error) {
		value := strings.TrimSpace(values[key])
		if value == "" {
			return "", fmt.Errorf("missing preset value %q", key)
		}
		return value, nil
	}
}

func fixedResolver(endpoint string) resolver {
	return func(map[string]string, domain.SourceConfig) (string, error) { return endpoint, nil }
}

func mastodonInputs(valueLabel, valuePlaceholder string) []Input {
	return []Input{
		{Key: "mastodon_instance", Label: "Mastodon 实例", Placeholder: "mastodon.social", Required: true, MaxLength: 253},
		{Key: "mastodon_value", Label: valueLabel, Placeholder: valuePlaceholder, Required: true, MaxLength: 100},
	}
}

func mastodonResolver(account bool) resolver {
	return func(values map[string]string, _ domain.SourceConfig) (string, error) {
		instance := strings.ToLower(strings.TrimSpace(values["mastodon_instance"]))
		value := strings.TrimSpace(values["mastodon_value"])
		if !mastodonInstancePattern.MatchString(instance) {
			return "", fmt.Errorf("invalid Mastodon instance")
		}
		if account {
			value = strings.TrimPrefix(value, "@")
			if !mastodonUsernamePattern.MatchString(value) {
				return "", fmt.Errorf("invalid Mastodon username")
			}
			return "https://" + instance + "/@" + value + ".rss", nil
		}
		value = strings.TrimPrefix(value, "#")
		if !mastodonHashtagPattern.MatchString(value) {
			return "", fmt.Errorf("invalid Mastodon hashtag")
		}
		return "https://" + instance + "/tags/" + url.PathEscape(value) + ".rss", nil
	}
}
