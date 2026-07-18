package websearch

import "context"

const bochaBaseURL = "https://api.bochaai.com"

type bochaProvider struct{ client providerClient }

func newBochaProvider(config Config) (*bochaProvider, error) {
	client, err := newProviderClient(
		ProviderBocha, config, bochaBaseURL, "/v1/web-search", true,
	)
	if err != nil {
		return nil, err
	}
	return &bochaProvider{client: client}, nil
}

func (p *bochaProvider) ID() ProviderID { return ProviderBocha }

func (p *bochaProvider) Search(ctx context.Context, input Request) (Result, error) {
	input, err := normalizeRequest(input)
	if err != nil {
		return Result{}, err
	}
	body := struct {
		Query     string `json:"query"`
		Freshness string `json:"freshness"`
		Summary   bool   `json:"summary"`
		Count     int    `json:"count"`
	}{Query: input.Query, Freshness: "noLimit", Summary: true, Count: input.MaxResults}
	var response struct {
		Data struct {
			WebPages struct {
				Value []struct {
					Name    string `json:"name"`
					URL     string `json:"url"`
					Summary string `json:"summary"`
					Snippet string `json:"snippet"`
				} `json:"value"`
			} `json:"webPages"`
			Images struct {
				Value []struct {
					Name        string `json:"name"`
					ContentURL  string `json:"contentUrl"`
					HostPageURL string `json:"hostPageUrl"`
				} `json:"value"`
			} `json:"images"`
		} `json:"data"`
	}
	if err := p.client.postJSON(
		ctx, body, bearerHeaders(p.client.apiKey), &response,
	); err != nil {
		return Result{}, err
	}
	sources := make([]Source, 0, len(response.Data.WebPages.Value))
	titles := make(map[string]string, len(response.Data.WebPages.Value))
	for _, item := range response.Data.WebPages.Value {
		content := item.Summary
		if content == "" {
			content = item.Snippet
		}
		sources = append(sources, Source{
			Title: item.Name, URL: item.URL, Content: content,
		})
		titles[item.URL] = item.Name
	}
	images := make([]Image, 0, len(response.Data.Images.Value))
	for _, item := range response.Data.Images.Value {
		description := item.Name
		if description == "" {
			description = titles[item.HostPageURL]
		}
		images = append(images, Image{URL: item.ContentURL, Description: description})
	}
	return normalizeResult(sources, images, input.MaxResults), nil
}
