package source

import "context"

// SystemSource defines a built-in news source that the system seeds on startup.
type SystemSource struct {
	Name             string
	Type             SourceType
	URL              string
	ComplianceNote   string
	FetchIntervalMin int
	RateLimitPerHour int
}

// systemSources is the list of built-in news sources seeded by SeedSources.
var systemSources = []SystemSource{
	{
		Name:             "Hacker News - Top Stories",
		Type:             SourceTypeHackerNews,
		URL:              "https://hacker-news.firebaseio.com/v0/topstories.json",
		FetchIntervalMin: 60,
		RateLimitPerHour: 30,
	},
	{
		Name:             "Hacker News - Best Stories",
		Type:             SourceTypeHackerNews,
		URL:              "https://hacker-news.firebaseio.com/v0/beststories.json",
		FetchIntervalMin: 120,
		RateLimitPerHour: 20,
	},
	{
		Name:             "TechCrunch",
		Type:             SourceTypeRSS,
		URL:              "https://techcrunch.com/feed/",
		FetchIntervalMin: 60,
		RateLimitPerHour: 60,
	},
	{
		Name:             "The Verge",
		Type:             SourceTypeRSS,
		URL:              "https://www.theverge.com/rss/index.xml",
		FetchIntervalMin: 60,
		RateLimitPerHour: 60,
	},
	{
		Name:             "Ars Technica",
		Type:             SourceTypeRSS,
		URL:              "https://feeds.arstechnica.com/arstechnica/index",
		FetchIntervalMin: 60,
		RateLimitPerHour: 60,
	},
	{
		Name:             "36氪",
		Type:             SourceTypePublicPage,
		URL:              "https://36kr.com/",
		FetchIntervalMin: 60,
		RateLimitPerHour: 30,
		ComplianceNote:   "system-seeded public page, respect robots.txt",
	},
}

// SeedSources creates built-in system news sources that don't already exist.
// Returns the number of newly created sources.
func (s *Service) SeedSources(ctx context.Context) (int, error) {
	existing, err := s.repo.ListSources(ctx)
	if err != nil {
		return 0, err
	}

	existingURLs := make(map[string]struct{}, len(existing))
	for _, src := range existing {
		existingURLs[src.URL] = struct{}{}
	}

	created := 0
	for _, sys := range systemSources {
		if _, exists := existingURLs[sys.URL]; exists {
			continue
		}
		complianceNote := sys.ComplianceNote
		if sys.Type == SourceTypePublicPage && complianceNote == "" {
			complianceNote = "system-seeded public page"
		}
		if _, err := s.CreateSource(ctx, CreateSourceInput{
			Name:             sys.Name,
			Type:             sys.Type,
			URL:              sys.URL,
			ComplianceNote:   complianceNote,
			FetchIntervalMin: sys.FetchIntervalMin,
			RateLimitPerHour: sys.RateLimitPerHour,
		}); err != nil {
			continue
		}
		created++
	}
	return created, nil
}
