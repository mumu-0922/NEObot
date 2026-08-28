package mcpclient

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"golang.org/x/sync/singleflight"
)

const (
	maxMarketplaceSearchBytes = int64(1 << 20)
	maxMarketplaceDetailBytes = int64(2 << 20)
	maxMarketplaceTokenBytes  = int64(64 << 10)
	maxNPMMetadataBytes       = int64(64 << 10)
	marketplaceTokenLeeway    = time.Minute
	defaultNPMRegistryURL     = "https://registry.npmjs.org"
	maxMarketplaceSkillBytes  = int64(32 << 20)
)

var exactSkillVersionPattern = regexp.MustCompile(`^(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)(?:-[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$`)

// FetchAgentMarketJSON exposes the already-authenticated, cached LobeHub
// Market transport to the Assistant Store adapter. Keeping token exchange in
// one owner avoids a second M2M implementation and a second token cache.
func (m *LobeHubMarketplace) FetchAgentMarketJSON(
	ctx context.Context,
	path string,
	maximum int64,
) ([]byte, error) {
	if m == nil || (path != "/api/v1/agents" &&
		!strings.HasPrefix(path, "/api/v1/agents?") &&
		!strings.HasPrefix(path, "/api/v1/agents/")) ||
		maximum < 1 || maximum > maxMarketplaceDetailBytes {
		return nil, ErrMarketplaceUnavailable
	}
	return m.authorizedGET(ctx, path, maximum)
}

// FetchSkillMarketJSON exposes only the authenticated Skill list and category
// endpoints through the existing Marketplace M2M token owner. Exact Skill
// detail is public upstream and deliberately has a separate method so an
// Authorization header cannot leak onto that route.
func (m *LobeHubMarketplace) FetchSkillMarketJSON(
	ctx context.Context,
	requestPath string,
	maximum int64,
) ([]byte, error) {
	if m == nil || maximum < 1 || maximum > maxMarketplaceDetailBytes ||
		!validSkillMarketListJSONPath(requestPath) {
		return nil, ErrMarketplaceUnavailable
	}
	return m.authorizedGET(ctx, requestPath, maximum)
}

func validSkillMarketListJSONPath(requestPath string) bool {
	parsed, err := url.ParseRequestURI(strings.TrimSpace(requestPath))
	if err != nil || parsed.IsAbs() || parsed.Host != "" || parsed.Fragment != "" {
		return false
	}
	switch parsed.EscapedPath() {
	case "/api/v1/skills", "/api/v1/skills/categories":
		return true
	}
	return false
}

// FetchPublicSkillDetailJSON reads one exact public Skill detail while keeping
// the same bounded origin, redirect, timeout, and response-size policy as the
// rest of the Marketplace client. Only locale and an exact SemVer may cross
// this unauthenticated boundary.
func (m *LobeHubMarketplace) FetchPublicSkillDetailJSON(
	ctx context.Context,
	requestPath string,
	maximum int64,
) ([]byte, error) {
	if m == nil || maximum < 1 || maximum > maxMarketplaceDetailBytes ||
		!validPublicSkillDetailJSONPath(requestPath) {
		return nil, ErrMarketplaceUnavailable
	}
	return m.publicGET(ctx, requestPath, maximum)
}

func validPublicSkillDetailJSONPath(requestPath string) bool {
	parsed, err := url.ParseRequestURI(strings.TrimSpace(requestPath))
	if err != nil || parsed.IsAbs() || parsed.Host != "" || parsed.Fragment != "" {
		return false
	}
	prefix := "/api/v1/skills/"
	if !strings.HasPrefix(parsed.EscapedPath(), prefix) {
		return false
	}
	segment := strings.TrimPrefix(parsed.EscapedPath(), prefix)
	if segment == "" || strings.Contains(segment, "/") ||
		segment == "categories" || segment == "download" {
		return false
	}
	identifier, err := url.PathUnescape(segment)
	if err != nil || strings.Contains(identifier, "/") ||
		url.PathEscape(identifier) != segment || !validMarketplaceIdentifier(identifier) {
		return false
	}
	query := parsed.Query()
	for key, values := range query {
		if len(values) != 1 || (key != "locale" && key != "version") {
			return false
		}
	}
	if locale := query.Get("locale"); locale != "" &&
		locale != "zh-CN" && locale != "en-US" && locale != "ja-JP" {
		return false
	}
	if version := query.Get("version"); version != "" &&
		!exactSkillVersionPattern.MatchString(version) {
		return false
	}
	return true
}

// FetchSkillPackage downloads one exact Skill version through the existing
// Marketplace M2M token owner. Skill archives are deliberately not cached:
// the supply-chain repository must observe and reject immutable-source drift.
func (m *LobeHubMarketplace) FetchSkillPackage(
	ctx context.Context,
	identifier string,
	version string,
	maximum int64,
) ([]byte, error) {
	identifier, version = strings.TrimSpace(identifier), strings.TrimSpace(version)
	if m == nil || !validMarketplaceIdentifier(identifier) ||
		!exactSkillVersionPattern.MatchString(version) || maximum < 1 ||
		maximum > maxMarketplaceSkillBytes {
		return nil, ErrMarketplaceUnavailable
	}
	query := url.Values{"version": []string{version}}
	path := "/api/v1/skills/" + url.PathEscape(identifier) + "/download?" + query.Encode()
	return m.authorizedDownload(ctx, path, maximum)
}

type LobeHubMarketplaceConfig struct {
	BaseURL        string
	NPMRegistryURL string
	ClientID       string
	ClientSecret   string
	Timeout        time.Duration
	CacheTTL       time.Duration
	HTTPClient     *http.Client
	Now            func() time.Time
}

type marketplaceCacheEntry struct {
	data      []byte
	expiresAt time.Time
}

type LobeHubMarketplace struct {
	baseURL        *url.URL
	npmRegistryURL *url.URL
	clientID       string
	clientSecret   string
	timeout        time.Duration
	cacheTTL       time.Duration
	httpClient     *http.Client
	now            func() time.Time

	tokenMu     sync.Mutex
	token       string
	tokenExpiry time.Time
	tokenGroup  singleflight.Group
	cacheMu     sync.Mutex
	cache       map[string]marketplaceCacheEntry
}

func NewLobeHubMarketplace(config LobeHubMarketplaceConfig) (*LobeHubMarketplace, error) {
	baseURL, err := url.Parse(strings.TrimSpace(config.BaseURL))
	if err != nil || baseURL.Scheme != "https" || baseURL.Host == "" || baseURL.User != nil ||
		baseURL.RawQuery != "" || baseURL.Fragment != "" {
		return nil, ErrMarketplaceUnavailable
	}
	baseURL.Path = strings.TrimRight(baseURL.Path, "/")
	npmRegistryRawURL := strings.TrimSpace(config.NPMRegistryURL)
	if npmRegistryRawURL == "" {
		npmRegistryRawURL = defaultNPMRegistryURL
	}
	npmRegistryURL, err := url.Parse(npmRegistryRawURL)
	if err != nil || npmRegistryURL.Scheme != "https" || npmRegistryURL.Host == "" ||
		npmRegistryURL.User != nil || npmRegistryURL.RawQuery != "" || npmRegistryURL.Fragment != "" {
		return nil, ErrMarketplaceUnavailable
	}
	npmRegistryURL.Path = strings.TrimRight(npmRegistryURL.Path, "/")
	if strings.TrimSpace(config.ClientID) == "" || len(config.ClientID) > 2048 ||
		strings.TrimSpace(config.ClientSecret) == "" || len(config.ClientSecret) > 4096 {
		return nil, ErrMarketplaceUnavailable
	}
	if config.Timeout <= 0 || config.Timeout > 30*time.Second ||
		config.CacheTTL < 10*time.Second || config.CacheTTL > time.Hour {
		return nil, ErrMarketplaceUnavailable
	}
	now := config.Now
	if now == nil {
		now = time.Now
	}
	client := config.HTTPClient
	if client == nil {
		baseOrigin := baseURL.Scheme + "://" + baseURL.Host
		client = &http.Client{
			Timeout: config.Timeout,
			CheckRedirect: func(request *http.Request, via []*http.Request) error {
				if len(via) >= 3 || request.URL.Scheme+"://"+request.URL.Host != baseOrigin {
					return ErrMarketplaceUnavailable
				}
				return nil
			},
		}
	}
	return &LobeHubMarketplace{
		baseURL:        baseURL,
		npmRegistryURL: npmRegistryURL,
		clientID:       strings.TrimSpace(config.ClientID),
		clientSecret:   strings.TrimSpace(config.ClientSecret),
		timeout:        config.Timeout,
		cacheTTL:       config.CacheTTL,
		httpClient:     client,
		now:            now,
		cache:          map[string]marketplaceCacheEntry{},
	}, nil
}

func (m *LobeHubMarketplace) Search(
	ctx context.Context,
	input MarketplaceSearchInput,
) (MarketplaceSearchResult, error) {
	ctx, cancel := context.WithTimeout(ctx, m.timeout)
	defer cancel()
	query := url.Values{}
	query.Set("locale", "zh-CN")
	query.Set("page", strconv.Itoa(input.Page))
	query.Set("pageSize", strconv.Itoa(input.PageSize))
	query.Set("sort", "relevance")
	if input.Query != "" {
		query.Set("q", input.Query)
	}
	if input.Category != "" {
		query.Set("category", input.Category)
	}
	path := "/api/v1/plugins?" + query.Encode()
	data, err := m.authorizedGET(ctx, path, maxMarketplaceSearchBytes)
	if err != nil {
		return MarketplaceSearchResult{}, err
	}
	var raw lobePluginListResponse
	if err := json.Unmarshal(data, &raw); err != nil {
		return MarketplaceSearchResult{}, ErrMarketplaceUnavailable
	}
	if raw.CurrentPage < 1 || raw.PageSize < 1 || raw.TotalCount < 0 || raw.TotalPages < 0 ||
		len(raw.Items) > input.PageSize {
		return MarketplaceSearchResult{}, ErrMarketplaceUnavailable
	}
	result := MarketplaceSearchResult{
		Items:      make([]MarketplaceItem, 0, len(raw.Items)),
		Categories: []MarketplaceCategory{},
		Page:       raw.CurrentPage,
		PageSize:   raw.PageSize,
		TotalCount: raw.TotalCount,
		TotalPages: raw.TotalPages,
		Source:     marketplaceProviderLobeHub,
		SourceURL:  m.baseURL.String(),
	}
	for _, item := range raw.Items {
		if !validMarketplaceIdentifier(strings.TrimSpace(item.Identifier)) || strings.TrimSpace(item.Name) == "" {
			continue
		}
		result.Items = append(result.Items, MarketplaceItem{
			Identifier:          boundedMarketplaceText(item.Identifier, 256),
			Name:                boundedMarketplaceText(item.Name, 256),
			Description:         boundedMarketplaceText(item.Description, 2048),
			Icon:                boundedMarketplaceIcon(item.Icon),
			Category:            boundedMarketplaceText(item.Category, 128),
			Author:              boundedMarketplaceText(item.Author, 256),
			ConnectionType:      boundedMarketplaceText(item.ConnectionType, 32),
			InstallationMethods: boundedMarketplaceText(item.InstallationMethods, 128),
			ToolCount:           boundedNonNegative(item.ToolsCount, 10000),
			InstallCount:        boundedNonNegative(item.InstallCount, 1_000_000_000),
			Stars:               boundedNonNegative(item.GitHub.Stars, 1_000_000_000),
			Rating:              boundedRating(item.RatingAverage),
			Official:            item.IsOfficial,
			Validated:           item.IsValidated,
		})
	}
	if categories, categoryErr := m.categories(ctx, input.Query); categoryErr == nil {
		result.Categories = categories
	}
	return result, nil
}

func (m *LobeHubMarketplace) categories(
	ctx context.Context,
	searchQuery string,
) ([]MarketplaceCategory, error) {
	query := url.Values{"locale": []string{"zh-CN"}}
	if searchQuery != "" {
		query.Set("q", searchQuery)
	}
	data, err := m.authorizedGET(
		ctx,
		"/api/v1/plugins/categories?"+query.Encode(),
		maxMarketplaceSearchBytes,
	)
	if err != nil {
		return nil, err
	}
	var raw []struct {
		Category string `json:"category"`
		Count    int    `json:"count"`
	}
	if json.Unmarshal(data, &raw) != nil || len(raw) > 256 {
		return nil, ErrMarketplaceUnavailable
	}
	result := make([]MarketplaceCategory, 0, len(raw))
	for _, item := range raw {
		category := strings.TrimSpace(item.Category)
		if !validMarketplaceCategory(category) || item.Count < 0 {
			continue
		}
		result = append(result, MarketplaceCategory{
			Category: category,
			Count:    boundedNonNegative(item.Count, 1_000_000_000),
		})
	}
	return result, nil
}

func (m *LobeHubMarketplace) GetItem(
	ctx context.Context,
	identifier string,
	version string,
) (MarketplaceItemDetail, error) {
	ctx, cancel := context.WithTimeout(ctx, m.timeout)
	defer cancel()
	query := url.Values{"locale": []string{"zh-CN"}}
	path := "/api/v1/plugins/" + url.PathEscape(identifier) + "?" + query.Encode()
	data, err := m.publicGET(ctx, path, maxMarketplaceDetailBytes)
	if err != nil {
		return MarketplaceItemDetail{}, err
	}
	var raw lobePluginDetail
	if err := json.Unmarshal(data, &raw); err != nil {
		return MarketplaceItemDetail{}, ErrMarketplaceUnavailable
	}
	if raw.Identifier != identifier || !validMarketplaceIdentifier(raw.Identifier) ||
		strings.TrimSpace(raw.Name) == "" || strings.TrimSpace(raw.Version) == "" {
		return MarketplaceItemDetail{}, ErrMarketplaceUnavailable
	}
	if version != "" && raw.Version != version {
		return MarketplaceItemDetail{}, ErrMarketplaceChanged
	}
	author := raw.Author.Name
	if author == "" {
		author = raw.AuthorText
	}
	detail := MarketplaceItemDetail{
		MarketplaceItem: MarketplaceItem{
			Identifier:     boundedMarketplaceText(raw.Identifier, 256),
			Name:           boundedMarketplaceText(raw.Name, 256),
			Description:    boundedMarketplaceText(raw.Description, 2048),
			Icon:           boundedMarketplaceIcon(raw.Icon),
			Category:       boundedMarketplaceText(raw.Category, 128),
			Author:         boundedMarketplaceText(author, 256),
			ConnectionType: boundedMarketplaceText(raw.ConnectionType, 32),
			ToolCount:      boundedNonNegative(raw.ToolsCount, 10000),
			InstallCount:   boundedNonNegative(raw.InstallCount, 1_000_000_000),
			Stars:          boundedNonNegative(raw.GitHub.Stars, 1_000_000_000),
			Rating:         boundedRating(raw.RatingAverage),
			Official:       raw.IsOfficial,
			Validated:      raw.IsValidated,
		},
		Version:       boundedMarketplaceText(raw.Version, 128),
		Summary:       boundedMarketplaceText(raw.Overview.Summary, 4096),
		Homepage:      boundedHTTPSURL(raw.Homepage),
		RepositoryURL: boundedHTTPSURL(raw.GitHub.URL),
		Source:        marketplaceProviderLobeHub,
		SourceURL:     m.baseURL.String(),
		Tools:         []MarketplaceToolPreview{},
		Deployments:   []MarketplaceDeployment{},
	}
	for index, tool := range raw.Tools {
		if index >= 64 {
			break
		}
		if strings.TrimSpace(tool.Name) == "" {
			continue
		}
		detail.Tools = append(detail.Tools, MarketplaceToolPreview{
			Name:        boundedMarketplaceText(tool.Name, 256),
			Description: boundedMarketplaceText(tool.Description, 1024),
		})
	}
	for index, option := range raw.DeploymentOptions {
		if index >= 32 {
			break
		}
		deployment := normalizeLobeDeployment(raw.Identifier, raw.Version, option)
		if deployment.ConnectionType == "stdio" && deployment.InstallationMethod == "npm" &&
			deployment.Command == "npx" && deployment.PackageName != "" {
			packageSpec, resolveErr := m.resolveNPMPackageSpec(
				ctx,
				deployment.PackageName,
				npmPackageSelector(deployment.PackageName, deployment.Args, raw.Version),
			)
			if resolveErr != nil {
				deployment.Compatibility = MarketplaceCompatibilityIncompatible
				deployment.CompatibilityReason = "The exact npm package version is unavailable"
			} else {
				deployment.PackageSpec = packageSpec
			}
		}
		detail.Deployments = append(detail.Deployments, deployment)
	}
	return detail, nil
}

func (m *LobeHubMarketplace) resolveNPMPackageSpec(
	ctx context.Context,
	packageName string,
	selector string,
) (string, error) {
	packageName = npmPackageBase(packageName)
	selector = strings.TrimSpace(selector)
	if m == nil || m.npmRegistryURL == nil || packageName == "" ||
		!validMarketplaceVersion(selector) {
		return "", ErrMarketplaceUnavailable
	}
	cacheKey := "npm-package:" + packageName + "@" + selector
	if data, ok := m.cached(cacheKey); ok {
		return parseNPMVersionMetadata(data, packageName)
	}
	requestURL := strings.TrimRight(m.npmRegistryURL.String(), "/") + "/" +
		url.PathEscape(packageName) + "/" + url.PathEscape(selector)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return "", ErrMarketplaceUnavailable
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "Neo-Chat-MCP-Marketplace/1")
	response, err := m.httpClient.Do(request)
	if err != nil {
		return "", ErrMarketplaceUnavailable
	}
	defer response.Body.Close()
	data, err := readMarketplaceBody(response.Body, maxNPMMetadataBytes)
	if err != nil || response.StatusCode != http.StatusOK {
		return "", ErrMarketplaceUnavailable
	}
	packageSpec, err := parseNPMVersionMetadata(data, packageName)
	if err != nil {
		return "", err
	}
	m.storeCache(cacheKey, data)
	return packageSpec, nil
}

func parseNPMVersionMetadata(data []byte, expectedPackageName string) (string, error) {
	var metadata struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	if json.Unmarshal(data, &metadata) != nil || npmPackageBase(metadata.Name) != expectedPackageName ||
		!exactNPMVersionPattern.MatchString(metadata.Version) {
		return "", ErrMarketplaceUnavailable
	}
	return expectedPackageName + "@" + metadata.Version, nil
}

func (m *LobeHubMarketplace) publicGET(
	ctx context.Context,
	path string,
	maximum int64,
) ([]byte, error) {
	if data, ok := m.cached(path); ok {
		return data, nil
	}
	data, status, err := m.get(ctx, path, "", maximum)
	if err != nil {
		return nil, err
	}
	switch status {
	case http.StatusOK:
		m.storeCache(path, data)
		return data, nil
	case http.StatusNotFound:
		return nil, ErrMarketplaceNotFound
	default:
		return nil, ErrMarketplaceUnavailable
	}
}

func (m *LobeHubMarketplace) authorizedGET(
	ctx context.Context,
	path string,
	maximum int64,
) ([]byte, error) {
	if data, ok := m.cached(path); ok {
		return data, nil
	}
	token, err := m.accessToken(ctx)
	if err != nil {
		return nil, err
	}
	data, status, err := m.get(ctx, path, token, maximum)
	if err != nil {
		return nil, err
	}
	if status == http.StatusUnauthorized {
		m.clearToken(token)
		token, err = m.accessToken(ctx)
		if err != nil {
			return nil, err
		}
		data, status, err = m.get(ctx, path, token, maximum)
		if err != nil {
			return nil, err
		}
	}
	switch status {
	case http.StatusOK:
		m.storeCache(path, data)
		return data, nil
	case http.StatusNotFound:
		return nil, ErrMarketplaceNotFound
	default:
		return nil, ErrMarketplaceUnavailable
	}
}

func (m *LobeHubMarketplace) get(
	ctx context.Context,
	path string,
	token string,
	maximum int64,
) ([]byte, int, error) {
	return m.getWithAccept(ctx, path, token, maximum, "application/json")
}

func (m *LobeHubMarketplace) getWithAccept(
	ctx context.Context,
	path string,
	token string,
	maximum int64,
	accept string,
) ([]byte, int, error) {
	requestURL, err := url.Parse(strings.TrimRight(m.baseURL.String(), "/") + path)
	if err != nil || requestURL.Scheme != m.baseURL.Scheme || requestURL.Host != m.baseURL.Host {
		return nil, 0, ErrMarketplaceUnavailable
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL.String(), nil)
	if err != nil {
		return nil, 0, ErrMarketplaceUnavailable
	}
	request.Header.Set("Accept", accept)
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	request.Header.Set("User-Agent", "Neo-Chat-MCP-Marketplace/1")
	response, err := m.httpClient.Do(request)
	if err != nil {
		return nil, 0, ErrMarketplaceUnavailable
	}
	defer response.Body.Close()
	data, err := readMarketplaceBody(response.Body, maximum)
	if err != nil {
		return nil, response.StatusCode, err
	}
	return data, response.StatusCode, nil
}

func (m *LobeHubMarketplace) authorizedDownload(
	ctx context.Context,
	path string,
	maximum int64,
) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, m.timeout)
	defer cancel()
	token, err := m.accessToken(ctx)
	if err != nil {
		return nil, err
	}
	data, status, err := m.getWithAccept(ctx, path, token, maximum, "application/zip")
	if err != nil {
		return nil, err
	}
	if status == http.StatusUnauthorized {
		m.clearToken(token)
		token, err = m.accessToken(ctx)
		if err != nil {
			return nil, err
		}
		data, status, err = m.getWithAccept(ctx, path, token, maximum, "application/zip")
		if err != nil {
			return nil, err
		}
	}
	switch status {
	case http.StatusOK:
		return data, nil
	case http.StatusNotFound:
		return nil, ErrMarketplaceNotFound
	default:
		return nil, ErrMarketplaceUnavailable
	}
}

func (m *LobeHubMarketplace) accessToken(ctx context.Context) (string, error) {
	m.tokenMu.Lock()
	if m.token != "" && m.now().Add(marketplaceTokenLeeway).Before(m.tokenExpiry) {
		token := m.token
		m.tokenMu.Unlock()
		return token, nil
	}
	m.tokenMu.Unlock()
	value, err, _ := m.tokenGroup.Do("m2m", func() (any, error) {
		m.tokenMu.Lock()
		if m.token != "" && m.now().Add(marketplaceTokenLeeway).Before(m.tokenExpiry) {
			token := m.token
			m.tokenMu.Unlock()
			return token, nil
		}
		m.tokenMu.Unlock()
		token, expiry, err := m.fetchToken(ctx)
		if err != nil {
			return "", err
		}
		m.tokenMu.Lock()
		m.token, m.tokenExpiry = token, expiry
		m.tokenMu.Unlock()
		return token, nil
	})
	if err != nil {
		return "", err
	}
	token, ok := value.(string)
	if !ok || token == "" {
		return "", ErrMarketplaceUnavailable
	}
	return token, nil
}

func (m *LobeHubMarketplace) fetchToken(ctx context.Context) (string, time.Time, error) {
	tokenURL := *m.baseURL
	tokenURL.Path = strings.TrimRight(m.baseURL.Path, "/") + "/oauth/token"
	tokenURL.RawQuery = ""
	assertion, err := m.clientAssertion(tokenURL.String())
	if err != nil {
		return "", time.Time{}, ErrMarketplaceUnavailable
	}
	form := url.Values{
		"grant_type":            []string{"client_credentials"},
		"client_assertion_type": []string{"urn:ietf:params:oauth:client-assertion-type:jwt-bearer"},
		"client_assertion":      []string{assertion},
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL.String(), strings.NewReader(form.Encode()))
	if err != nil {
		return "", time.Time{}, ErrMarketplaceUnavailable
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "Neo-Chat-MCP-Marketplace/1")
	response, err := m.httpClient.Do(request)
	if err != nil {
		return "", time.Time{}, ErrMarketplaceUnavailable
	}
	defer response.Body.Close()
	data, err := readMarketplaceBody(response.Body, maxMarketplaceTokenBytes)
	if err != nil || response.StatusCode != http.StatusOK {
		return "", time.Time{}, ErrMarketplaceUnavailable
	}
	var token struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
		TokenType   string `json:"token_type"`
	}
	if json.Unmarshal(data, &token) != nil || strings.TrimSpace(token.AccessToken) == "" ||
		len(token.AccessToken) > 64<<10 || token.ExpiresIn < 60 || token.ExpiresIn > 24*60*60 ||
		(token.TokenType != "" && !strings.EqualFold(token.TokenType, "Bearer")) {
		return "", time.Time{}, ErrMarketplaceUnavailable
	}
	return token.AccessToken, m.now().Add(time.Duration(token.ExpiresIn) * time.Second), nil
}

func (m *LobeHubMarketplace) clientAssertion(audience string) (string, error) {
	now := m.now().UTC()
	jti := make([]byte, 18)
	if _, err := rand.Read(jti); err != nil {
		return "", err
	}
	header, _ := json.Marshal(map[string]string{"alg": "HS256", "typ": "JWT"})
	claims, err := json.Marshal(map[string]any{
		"iss": m.clientID,
		"sub": m.clientID,
		"aud": audience,
		"jti": base64.RawURLEncoding.EncodeToString(jti),
		"iat": now.Unix(),
		"exp": now.Add(5 * time.Minute).Unix(),
	})
	if err != nil {
		return "", err
	}
	unsigned := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(claims)
	mac := hmac.New(sha256.New, []byte(m.clientSecret))
	_, _ = mac.Write([]byte(unsigned))
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func (m *LobeHubMarketplace) clearToken(expected string) {
	m.tokenMu.Lock()
	if m.token == expected {
		m.token, m.tokenExpiry = "", time.Time{}
	}
	m.tokenMu.Unlock()
}

func (m *LobeHubMarketplace) cached(key string) ([]byte, bool) {
	m.cacheMu.Lock()
	defer m.cacheMu.Unlock()
	entry, found := m.cache[key]
	if !found || !m.now().Before(entry.expiresAt) {
		delete(m.cache, key)
		return nil, false
	}
	return append([]byte(nil), entry.data...), true
}

func (m *LobeHubMarketplace) storeCache(key string, data []byte) {
	m.cacheMu.Lock()
	defer m.cacheMu.Unlock()
	if len(m.cache) >= 256 {
		for candidate := range m.cache {
			delete(m.cache, candidate)
			break
		}
	}
	m.cache[key] = marketplaceCacheEntry{data: append([]byte(nil), data...), expiresAt: m.now().Add(m.cacheTTL)}
}

func readMarketplaceBody(reader io.Reader, maximum int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, maximum+1))
	if err != nil || int64(len(data)) > maximum {
		return nil, ErrMarketplaceUnavailable
	}
	return data, nil
}

func normalizeLobeDeployment(
	identifier string,
	version string,
	option lobeDeploymentOption,
) MarketplaceDeployment {
	deployment := MarketplaceDeployment{
		ConnectionType:      boundedMarketplaceText(option.Connection.Type, 16),
		InstallationMethod:  boundedMarketplaceText(option.InstallationMethod, 64),
		Recommended:         option.IsRecommended,
		Compatibility:       MarketplaceCompatibilityIncompatible,
		CompatibilityReason: "Unsupported connection",
	}
	switch option.Connection.Type {
	case "http":
		secretFields := lobeRequiredStringFields(option.Connection.ConfigSchema, false)
		deployment.EndpointURL = boundedMarketplaceEndpointTemplate(option.Connection.URL)
		if deployment.EndpointURL == "" {
			deployment.CompatibilityReason = "A public HTTPS endpoint is required"
		} else if len(secretFields) > 0 {
			if headerName, ok := approvedMarketplaceHeader(identifier, deployment.EndpointURL, secretFields); ok {
				deployment.SecretFields = secretFields
				deployment.InstallMode = "header"
				deployment.HeaderName = headerName
				deployment.Compatibility = MarketplaceCompatibilityNeedsConfig
				deployment.CompatibilityReason = "An API key is required"
			} else {
				deployment.Compatibility = MarketplaceCompatibilityIncompatible
				deployment.CompatibilityReason = "The Marketplace does not declare an approved API-key header"
			}
		} else {
			deployment.InstallMode = "direct"
			deployment.Compatibility = MarketplaceCompatibilityInstallable
			deployment.CompatibilityReason = "Public HTTPS Streamable HTTP"
		}
	case "stdio":
		deployment.SecretFields = lobeRequiredStringFields(option.Connection.ConfigSchema, true)
		deployment.Command = boundedMarketplaceCommand(option.Connection.Command, 256)
		deployment.Args = boundedMarketplaceArgs(option.Connection.Args)
		deployment.PackageName = boundedMarketplaceCommand(option.InstallationDetails.PackageName, 512)
		if deployment.Command == "" || deployment.Args == nil {
			deployment.CompatibilityReason = "The local command metadata is invalid"
		} else if len(deployment.SecretFields) > 0 {
			deployment.InstallMode = "runner_env"
			deployment.Compatibility = MarketplaceCompatibilityNeedsConfig
			deployment.CompatibilityReason = "Additional Runner configuration is required"
		} else {
			deployment.Compatibility = MarketplaceCompatibilityRequiresRunner
			deployment.CompatibilityReason = "An approved shared Runner artifact is required"
		}
	case "sse":
		deployment.CompatibilityReason = "Legacy SSE transport is not supported"
	}
	if deployment.EndpointURL != "" || deployment.Command != "" {
		hash, err := marketplaceDeploymentHash(identifier, version, deployment)
		if err == nil {
			deployment.Hash = hash
		}
	}
	return deployment
}

func approvedMarketplaceHeader(identifier, endpoint string, secretFields []string) (string, bool) {
	if identifier == "tavily-ai-tavily-mcp" && endpoint == "https://mcp.tavily.com/mcp/" &&
		len(secretFields) == 1 && secretFields[0] == "TAVILY_API_KEY" {
		return "Authorization", true
	}
	return "", false
}

func boundedMarketplaceCommand(value string, maximum int) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > maximum || !utf8.ValidString(value) ||
		strings.ContainsAny(value, "\x00\r\n\t") {
		return ""
	}
	return value
}

func boundedMarketplaceArgs(values []string) []string {
	if len(values) > maxManifestArgs {
		return nil
	}
	result := make([]string, len(values))
	for index, value := range values {
		value = boundedMarketplaceCommand(value, maxManifestString)
		if value == "" {
			return nil
		}
		result[index] = value
	}
	return result
}

func lobeConfigRequiresInput(schema json.RawMessage) bool {
	if len(schema) == 0 || string(schema) == "null" || string(schema) == "{}" {
		return false
	}
	var object struct {
		Required []string `json:"required"`
	}
	return json.Unmarshal(schema, &object) != nil || len(object.Required) > 0
}

func lobeRequiredStringFields(schema json.RawMessage, environmentOnly bool) []string {
	if len(schema) == 0 || string(schema) == "null" || string(schema) == "{}" {
		return nil
	}
	var object struct {
		Required   []string `json:"required"`
		Properties map[string]struct {
			Type string `json:"type"`
		} `json:"properties"`
	}
	if json.Unmarshal(schema, &object) != nil || len(object.Required) > 8 {
		return nil
	}
	result := make([]string, 0, len(object.Required))
	for _, field := range object.Required {
		field = strings.TrimSpace(field)
		property, found := object.Properties[field]
		if field == "" || len(field) > 128 ||
			strings.ContainsAny(field, "\x00\r\n\t") {
			return nil
		}
		if found && property.Type != "" && property.Type != "string" {
			return nil
		}
		if environmentOnly && (!environmentPattern.MatchString(field) || !safeCommandEnvironmentName(field)) {
			return nil
		}
		result = append(result, field)
	}
	sort.Strings(result)
	return result
}

func boundedMarketplaceEndpointTemplate(value string) string {
	value = boundedMarketplaceText(value, 2048)
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return ""
	}
	query := parsed.Query()
	for key, values := range query {
		for _, candidate := range values {
			if strings.Contains(candidate, "{{") || strings.Contains(candidate, "}}") {
				query.Del(key)
				break
			}
		}
	}
	parsed.RawQuery = query.Encode()
	if strings.Contains(parsed.String(), "{{") || strings.Contains(parsed.String(), "}}") {
		return ""
	}
	return parsed.String()
}

func boundedMarketplaceText(value string, maximum int) string {
	value = strings.TrimSpace(value)
	if len(value) <= maximum {
		return value
	}
	value = value[:maximum]
	for !utf8.ValidString(value) && len(value) > 0 {
		value = value[:len(value)-1]
	}
	return value
}

func boundedHTTPSURL(value string) string {
	value = boundedMarketplaceText(value, 2048)
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return ""
	}
	return parsed.String()
}

func boundedMarketplaceIcon(value string) string {
	value = strings.TrimSpace(value)
	if httpsURL := boundedHTTPSURL(value); httpsURL != "" {
		return httpsURL
	}
	if value == "" || len(value) > 32 || !utf8.ValidString(value) ||
		utf8.RuneCountInString(value) > 4 || strings.ContainsAny(value, "\r\n\t") {
		return ""
	}
	return value
}

func boundedNonNegative(value int, maximum int) int {
	if value < 0 {
		return 0
	}
	if value > maximum {
		return maximum
	}
	return value
}

func boundedRating(value float64) float64 {
	if value < 0 || value > 5 {
		return 0
	}
	return value
}

type lobePluginListResponse struct {
	CurrentPage int              `json:"currentPage"`
	Items       []lobePluginItem `json:"items"`
	PageSize    int              `json:"pageSize"`
	TotalCount  int              `json:"totalCount"`
	TotalPages  int              `json:"totalPages"`
}

type lobePluginItem struct {
	Identifier          string  `json:"identifier"`
	Name                string  `json:"name"`
	Description         string  `json:"description"`
	Icon                string  `json:"icon"`
	Category            string  `json:"category"`
	Author              string  `json:"author"`
	ConnectionType      string  `json:"connectionType"`
	InstallationMethods string  `json:"installationMethods"`
	ToolsCount          int     `json:"toolsCount"`
	InstallCount        int     `json:"installCount"`
	RatingAverage       float64 `json:"ratingAverage"`
	IsOfficial          bool    `json:"isOfficial"`
	IsValidated         bool    `json:"isValidated"`
	GitHub              struct {
		Stars int    `json:"stars"`
		URL   string `json:"url"`
	} `json:"github"`
}

type lobePluginDetail struct {
	lobePluginItem
	Version    string `json:"version"`
	Homepage   string `json:"homepage"`
	AuthorText string
	Author     struct {
		Name string `json:"name"`
	} `json:"author"`
	Overview struct {
		Summary string `json:"summary"`
	} `json:"overview"`
	Tools []struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	} `json:"tools"`
	DeploymentOptions []lobeDeploymentOption `json:"deploymentOptions"`
}

func (d *lobePluginDetail) UnmarshalJSON(data []byte) error {
	type detailAlias lobePluginDetail
	var auxiliary struct {
		detailAlias
		Author json.RawMessage `json:"author"`
	}
	if err := json.Unmarshal(data, &auxiliary); err != nil {
		return err
	}
	*d = lobePluginDetail(auxiliary.detailAlias)
	if len(auxiliary.Author) == 0 || string(auxiliary.Author) == "null" {
		return nil
	}
	if auxiliary.Author[0] == '"' {
		return json.Unmarshal(auxiliary.Author, &d.AuthorText)
	}
	return json.Unmarshal(auxiliary.Author, &d.Author)
}

type lobeDeploymentOption struct {
	Connection struct {
		Type         string          `json:"type"`
		URL          string          `json:"url"`
		ConfigSchema json.RawMessage `json:"configSchema"`
		Command      string          `json:"command"`
		Args         []string        `json:"args"`
	} `json:"connection"`
	InstallationDetails struct {
		PackageName string `json:"packageName"`
	} `json:"installationDetails"`
	InstallationMethod string `json:"installationMethod"`
	IsRecommended      bool   `json:"isRecommended"`
}

var _ Marketplace = (*LobeHubMarketplace)(nil)
