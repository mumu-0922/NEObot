package websearch

import (
	"context"
	"encoding/json"
)

const firecrawlBaseURL = "https://api.firecrawl.dev"

type firecrawlProvider struct{ client providerClient }

func newFirecrawlProvider(config Config) (*firecrawlProvider, error) {
	client, err := newProviderClient(
		ProviderFirecrawl, config, firecrawlBaseURL, "/v2/search", false,
	)
	if err != nil {
		return nil, err
	}
	return &firecrawlProvider{client: client}, nil
}

func (p *firecrawlProvider) ID() ProviderID { return ProviderFirecrawl }

func (p *firecrawlProvider) Search(ctx context.Context, input Request) (Result, error) {
	input, err := normalizeRequest(input)
	if err != nil {
		return Result{}, err
	}
	body := struct {
		Query         string         `json:"query"`
		Limit         int            `json:"limit"`
		Sources       []string       `json:"sources"`
		TimeRange     string         `json:"tbs"`
		ScrapeOptions map[string]any `json:"scrapeOptions"`
		Timeout       int            `json:"timeout"`
	}{
		Query: input.Query, Limit: input.MaxResults,
		Sources: []string{"web", "images"}, TimeRange: "qdr:w",
		ScrapeOptions: map[string]any{
			"formats": []map[string]string{{"type": "markdown"}},
		},
		Timeout: 25_000,
	}
	var response struct {
		Data json.RawMessage `json:"data"`
	}
	if err := p.client.postJSON(
		ctx, body, bearerHeaders(p.client.apiKey), &response,
	); err != nil {
		return Result{}, err
	}
	return normalizeFirecrawlData(response.Data, input.MaxResults), nil
}

type firecrawlItem struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Markdown    string `json:"markdown"`
	Description string `json:"description"`
	Snippet     string `json:"snippet"`
	ImageURL    string `json:"imageUrl"`
}

func normalizeFirecrawlData(data json.RawMessage, limit int) Result {
	var object struct {
		Web    []firecrawlItem `json:"web"`
		Images []firecrawlItem `json:"images"`
	}
	if err := json.Unmarshal(data, &object); err != nil || object.Web == nil {
		var items []firecrawlItem
		if json.Unmarshal(data, &items) == nil {
			object.Web = items
		}
	}
	sources := make([]Source, 0, len(object.Web))
	for _, item := range object.Web {
		content := item.Markdown
		if content == "" {
			content = item.Description
		}
		if content == "" {
			content = item.Snippet
		}
		if content == "" {
			content = item.Title
		}
		sources = append(sources, Source{
			Title: item.Title, URL: item.URL, Content: content,
		})
	}
	images := make([]Image, 0, len(object.Images))
	for _, item := range object.Images {
		images = append(images, Image{URL: item.ImageURL, Description: item.Title})
	}
	return normalizeResult(sources, images, limit)
}
