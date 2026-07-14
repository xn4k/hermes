package main

import (
	"context"
	"fmt"
	"github.com/mmcdole/gofeed"
	"sort"
	"strings"
	"sync"
	"time"
)

type NewsCategory string

const (
	CategoryGermany    NewsCategory = "deutschland"
	CategoryWorld      NewsCategory = "welt"
	CategoryPolitics   NewsCategory = "politik"
	CategoryTech       NewsCategory = "tech"
	CategorySecurity   NewsCategory = "security"
	CategoryScience    NewsCategory = "wissenschaft"
	CategoryCulture    NewsCategory = "kultur"
	CategoryMusic      NewsCategory = "musik"
	CategoryLiterature NewsCategory = "literatur"
	CategoryEconomy    NewsCategory = "wirtschaft"
	CategorySports     NewsCategory = "sport"
	CategoryWeather    NewsCategory = "wetter-klima"
)

type NewsSource struct {
	ID       string       `json:"id"`
	Name     string       `json:"name"`
	FeedURL  string       `json:"feed_url"`
	Category NewsCategory `json:"category"`
	Enabled  bool         `json:"enabled"`
}

type NewsArticle struct {
	ID          string       `json:"id"`
	SourceID    string       `json:"sourceId"`
	SourceName  string       `json:"sourceName"`
	Category    NewsCategory `json:"category"`
	Title       string       `json:"title"`
	URL         string       `json:"url"`
	Summary     string       `json:"summary"`
	PublishedAt time.Time    `json:"publishedAt"`
}

type NewsCache struct {
	Articles    []NewsArticle `json:"articles"`
	RefreshedAt time.Time     `json:"refreshedAt"`
}

type NewsRefreshWarning struct {
	SourceID   string `json:"sourceId"`
	SourceName string `json:"sourceName"`
	Message    string `json:"message"`
}

type NewsService struct {
	sources  []NewsSource
	cache    NewsCache
	cacheTTL time.Duration
	mu       sync.RWMutex
}

var newsSources = []NewsSource{
	{
		ID:       "tagesschau-deutschland",
		Name:     "Tagesschau",
		FeedURL:  "https://www.tagesschau.de/inland/index~rss2.xml",
		Category: CategoryGermany,
		Enabled:  true,
	},
	{
		ID:       "tagesschau-welt",
		Name:     "Tagesschau",
		FeedURL:  "https://www.tagesschau.de/ausland/index~rss2.xml",
		Category: CategoryWorld,
		Enabled:  true,
	},
	{
		ID:       "deutschlandfunk-politik",
		Name:     "Deutschlandfunk",
		FeedURL:  "https://www.deutschlandfunk.de/politikportal-100.rss",
		Category: CategoryPolitics,
		Enabled:  true,
	},
	{
		ID:       "tagesschau-tech",
		Name:     "Tagesschau",
		FeedURL:  "https://www.tagesschau.de/wissen/technologie/index~rss2.xml",
		Category: CategoryTech,
		Enabled:  true,
	},
	{
		ID:       "bsi-cert-bund",
		Name:     "BSI / CERT-Bund",
		FeedURL:  "https://www.bsi.bund.de/SiteGlobals/Functions/RSSFeed/RSSNewsfeed/RSSNewsfeed_Presse_Veranstaltungen.xml",
		Category: CategorySecurity,
		Enabled:  true,
	},
	{
		ID:       "deutschlandfunk-wissenschaft",
		Name:     "Deutschlandfunk",
		FeedURL:  "https://www.deutschlandfunk.de/wissen-106.rss",
		Category: CategoryScience,
		Enabled:  true,
	},
	{
		ID:       "deutschlandfunk-kultur",
		Name:     "Deutschlandfunk",
		FeedURL:  "https://www.deutschlandfunk.de/kulturportal-100.rss",
		Category: CategoryCulture,
		Enabled:  true,
	},
	{
		ID:       "deutschlandfunk-musik",
		Name:     "Deutschlandfunk Kultur",
		FeedURL:  "https://www.deutschlandfunkkultur.de/musikportal-100.rss",
		Category: CategoryMusic,
		Enabled:  true,
	},
	{
		ID:       "deutschlandfunk-literatur",
		Name:     "Deutschlandfunk Kultur",
		FeedURL:  "https://www.deutschlandfunkkultur.de/buecher-108.rss",
		Category: CategoryLiterature,
		Enabled:  true,
	},
	{
		ID:       "deutschlandfunk-wirtschaft",
		Name:     "Deutschlandfunk",
		FeedURL:  "https://www.deutschlandfunk.de/wirtschaft-106.rss",
		Category: CategoryEconomy,
		Enabled:  true,
	},
	{
		ID:       "deutschlandfunk-sport",
		Name:     "Deutschlandfunk",
		FeedURL:  "https://www.deutschlandfunk.de/sportportal-100.rss",
		Category: CategorySports,
		Enabled:  true,
	},
	{
		ID:       "tagesschau-klima",
		Name:     "Tagesschau",
		FeedURL:  "https://www.tagesschau.de/wissen/klima/index~rss2.xml",
		Category: CategoryWeather,
		Enabled:  true,
	},
}

func NewNewsService(sources []NewsSource) *NewsService {
	return &NewsService{
		sources:  sources,
		cacheTTL: 15 * time.Minute,
	}
}

func (s *NewsService) getCachedArticles() ([]NewsArticle, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.cache.RefreshedAt.IsZero() ||
		time.Since(s.cache.RefreshedAt) >= s.cacheTTL {
		return nil, false
	}

	articles := make([]NewsArticle, len(s.cache.Articles))
	copy(articles, s.cache.Articles)

	return articles, true
}

func (s *NewsService) setCachedArticles(articles []NewsArticle) {
	copiedArticles := make([]NewsArticle, len(articles))
	copy(copiedArticles, articles)

	s.mu.Lock()
	defer s.mu.Unlock()

	s.cache = NewsCache{
		Articles:    copiedArticles,
		RefreshedAt: time.Now(),
	}
}

func (s *NewsService) fetchSource(ctx context.Context, source NewsSource) ([]NewsArticle, error) {
	parser := gofeed.NewParser()

	feed, err := parser.ParseURLWithContext(source.FeedURL, ctx)
	if err != nil {
		return nil, fmt.Errorf("feed %s laden: %w", source.ID, err)
	}

	articles := make([]NewsArticle, 0, len(feed.Items))

	for _, item := range feed.Items {
		articleID := item.GUID
		if articleID == "" {
			articleID = item.Link
		}

		publishedAt := time.Time{}
		if item.PublishedParsed != nil {
			publishedAt = *item.PublishedParsed
		} else if item.UpdatedParsed != nil {
			publishedAt = *item.UpdatedParsed
		}

		articles = append(articles, NewsArticle{
			ID:          source.ID + ":" + articleID,
			SourceID:    source.ID,
			SourceName:  source.Name,
			Category:    source.Category,
			Title:       item.Title,
			URL:         item.Link,
			Summary:     item.Description,
			PublishedAt: publishedAt,
		})
	}

	return articles, nil
}

func normalizeArticles(articles []NewsArticle) []NewsArticle {
	seen := make(map[string]bool)
	normalized := make([]NewsArticle, 0, len(articles))

	for _, article := range articles {
		key := strings.TrimSpace(article.URL)
		if key == "" {
			key = strings.TrimSpace(article.ID)
		}

		if key == "" || seen[key] {
			continue
		}

		seen[key] = true
		normalized = append(normalized, article)
	}

	sort.SliceStable(normalized, func(i, j int) bool {
		return normalized[i].PublishedAt.After(normalized[j].PublishedAt)
	})

	return normalized
}

func (s *NewsService) refresh(ctx context.Context) ([]NewsArticle, []NewsRefreshWarning, error) {
	var allArticles []NewsArticle
	var warnings []NewsRefreshWarning

	for _, source := range s.sources {
		if !source.Enabled {
			continue
		}

		articles, err := s.fetchSource(ctx, source)
		if err != nil {
			warnings = append(warnings, NewsRefreshWarning{
				SourceID:   source.ID,
				SourceName: source.Name,
				Message:    err.Error(),
			})

			continue
		}

		allArticles = append(allArticles, articles...)
	}

	normalizedArticles := normalizeArticles(allArticles)

	if len(normalizedArticles) == 0 && len(warnings) > 0 {
		return nil, warnings, fmt.Errorf("all news sources failed")
	}

	s.setCachedArticles(normalizedArticles)

	return normalizedArticles, warnings, nil
}
