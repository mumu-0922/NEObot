package websearch

func NewProvider(id ProviderID, config Config) (Provider, error) {
	switch id {
	case ProviderTavily:
		return newTavilyProvider(config)
	case ProviderFirecrawl:
		return newFirecrawlProvider(config)
	case ProviderExa:
		return newExaProvider(config)
	case ProviderBocha:
		return newBochaProvider(config)
	default:
		return nil, ErrInvalidConfig
	}
}

func bearerHeaders(apiKey string) map[string]string {
	if apiKey == "" {
		return nil
	}
	return map[string]string{"Authorization": "Bearer " + apiKey}
}
