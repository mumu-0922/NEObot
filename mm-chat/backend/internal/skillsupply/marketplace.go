package skillsupply

import (
	"context"
	"encoding/json"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	maxSkillMarketplaceJSONBytes = int64(2 << 20)
	maxMarketplaceQueryRunes     = 200
	maxMarketplacePageSize       = 50
	maxMarketplaceCategories     = 512
	maxMarketplaceResources      = 512
)

type MarketplaceSearchInput struct {
	Query    string
	Category string
	Locale   string
	Sort     string
	Page     int
	PageSize int
}

type MarketplaceCategory struct {
	Category string `json:"category"`
	Count    int    `json:"count"`
}

type MarketplaceSkillSummary struct {
	Identifier    string  `json:"identifier"`
	Name          string  `json:"name"`
	Description   string  `json:"description"`
	Version       string  `json:"version"`
	Category      string  `json:"category,omitempty"`
	Author        string  `json:"author,omitempty"`
	Icon          string  `json:"icon,omitempty"`
	License       string  `json:"license,omitempty"`
	Homepage      string  `json:"homepage,omitempty"`
	RepositoryURL string  `json:"repositoryUrl,omitempty"`
	InstallCount  int     `json:"installCount"`
	Rating        float64 `json:"rating"`
	Official      bool    `json:"official"`
	Validated     bool    `json:"validated"`
	Featured      bool    `json:"featured"`
	ResourceCount int     `json:"resourceCount"`
}

type MarketplaceSearchResult struct {
	Items      []MarketplaceSkillSummary `json:"items"`
	Categories []MarketplaceCategory     `json:"categories"`
	Page       int                       `json:"page"`
	PageSize   int                       `json:"pageSize"`
	TotalCount int                       `json:"totalCount"`
	TotalPages int                       `json:"totalPages"`
	Source     string                    `json:"source"`
	SourceURL  string                    `json:"sourceUrl"`
}

type MarketplaceSkillResource struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

type MarketplaceSkillVersion struct {
	Version     string `json:"version"`
	Latest      bool   `json:"latest"`
	Validated   bool   `json:"validated"`
	CreatedAt   string `json:"createdAt"`
	VersionRank int    `json:"versionRank"`
}

type MarketplaceSkillDetail struct {
	MarketplaceSkillSummary
	ManifestName     string                     `json:"manifestName"`
	Summary          string                     `json:"summary,omitempty"`
	Permissions      []string                   `json:"permissions"`
	Resources        []MarketplaceSkillResource `json:"resources"`
	Versions         []MarketplaceSkillVersion  `json:"versions"`
	Source           string                     `json:"source"`
	SourceURL        string                     `json:"sourceUrl"`
	Installed        bool                       `json:"installed"`
	packageSourceURL string
}

type lobeSkillListResponse struct {
	Items []struct {
		Identifier     string   `json:"identifier"`
		Name           string   `json:"name"`
		Description    string   `json:"description"`
		Version        string   `json:"version"`
		Category       string   `json:"category"`
		Author         string   `json:"author"`
		Icon           string   `json:"icon"`
		Logo           string   `json:"logo"`
		License        string   `json:"license"`
		Homepage       string   `json:"homepage"`
		InstallCount   int      `json:"installCount"`
		RatingAverage  float64  `json:"ratingAvg"`
		Official       bool     `json:"isOfficial"`
		Validated      bool     `json:"isValidated"`
		Featured       bool     `json:"isFeatured"`
		ResourcesCount int      `json:"resourcesCount"`
		Tags           []string `json:"tags"`
		GitHub         struct {
			URL string `json:"url"`
		} `json:"github"`
	} `json:"items"`
	CurrentPage int `json:"currentPage"`
	PageSize    int `json:"pageSize"`
	TotalCount  int `json:"totalCount"`
	TotalPages  int `json:"totalPages"`
}

type lobeSkillDetail struct {
	Identifier    string  `json:"identifier"`
	Name          string  `json:"name"`
	Description   string  `json:"description"`
	Version       string  `json:"version"`
	Category      string  `json:"category"`
	Icon          string  `json:"icon"`
	Logo          string  `json:"logo"`
	Homepage      string  `json:"homepage"`
	InstallCount  int     `json:"installCount"`
	RatingAverage float64 `json:"ratingAverage"`
	Official      bool    `json:"isOfficial"`
	Validated     bool    `json:"isValidated"`
	Featured      bool    `json:"isFeatured"`
	Repository    string  `json:"repository"`
	Author        struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	} `json:"author"`
	GitHub struct {
		URL string `json:"url"`
	} `json:"github"`
	License struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	} `json:"license"`
	Manifest struct {
		Name        string   `json:"name"`
		SourceURL   string   `json:"sourceUrl"`
		Description string   `json:"description"`
		License     string   `json:"license"`
		Repository  string   `json:"repository"`
		Permissions []string `json:"permissions"`
	} `json:"manifest"`
	Overview struct {
		Summary string `json:"summary"`
	} `json:"overview"`
	Resources map[string]struct {
		FileHash string `json:"fileHash"`
		Size     int64  `json:"size"`
	} `json:"resources"`
	Versions []struct {
		Version       string `json:"version"`
		Latest        bool   `json:"isLatest"`
		Validated     bool   `json:"isValidated"`
		CreatedAt     string `json:"createdAt"`
		VersionNumber int    `json:"versionNumber"`
	} `json:"versions"`
}

func (service *Service) SearchMarketplace(
	ctx context.Context,
	input MarketplaceSearchInput,
) (MarketplaceSearchResult, error) {
	if service == nil || service.lobeMarketplace == nil {
		return MarketplaceSearchResult{}, ErrUnavailable
	}
	input.Query, input.Category = strings.TrimSpace(input.Query), strings.TrimSpace(input.Category)
	if input.Page == 0 {
		input.Page = 1
	}
	if input.PageSize == 0 {
		input.PageSize = 20
	}
	locale, localeOK := normalizeMarketplaceLocale(input.Locale)
	sortField, sortOK := normalizeMarketplaceSort(input.Sort)
	if input.Page < 1 || input.Page > 10000 || input.PageSize < 1 ||
		input.PageSize > maxMarketplacePageSize || len([]rune(input.Query)) > maxMarketplaceQueryRunes ||
		len([]rune(input.Category)) > 128 || !validPlainText(input.Query) ||
		!validPlainText(input.Category) ||
		(input.Category != "" && !isCuratedMarketplaceCategory(input.Category)) || !localeOK || !sortOK {
		return MarketplaceSearchResult{}, validationError("INVALID_SKILL_MARKETPLACE_QUERY", "Skill Marketplace query is invalid")
	}
	query := url.Values{
		"locale":   []string{locale},
		"page":     []string{strconv.Itoa(input.Page)},
		"pageSize": []string{strconv.Itoa(input.PageSize)},
		"sort":     []string{sortField},
		"order":    []string{"desc"},
	}
	if input.Query != "" {
		query.Set("q", input.Query)
	}
	if input.Category != "" {
		query.Set("category", input.Category)
	}
	data, err := service.lobeMarketplace.FetchSkillMarketJSON(
		ctx, "/api/v1/skills?"+query.Encode(), maxSkillMarketplaceJSONBytes,
	)
	if err != nil {
		return MarketplaceSearchResult{}, ErrSourceUnavailable
	}
	var raw lobeSkillListResponse
	if json.Unmarshal(data, &raw) != nil || raw.CurrentPage < 1 || raw.PageSize < 1 ||
		raw.TotalCount < 0 || raw.TotalPages < 0 || len(raw.Items) > input.PageSize {
		return MarketplaceSearchResult{}, ErrSourceUnavailable
	}
	result := MarketplaceSearchResult{
		Items: make([]MarketplaceSkillSummary, 0, len(raw.Items)), Categories: []MarketplaceCategory{},
		Page: raw.CurrentPage, PageSize: raw.PageSize, TotalCount: raw.TotalCount,
		TotalPages: raw.TotalPages, Source: SourceLobeHub,
		SourceURL: "https://lobehub.com/skills",
	}
	for _, item := range raw.Items {
		if normalized, ok := normalizeMarketplaceSummary(item.Identifier, item.Name,
			item.Description, item.Version, item.Category, item.Author, firstValue(item.Icon, item.Logo),
			item.License, item.Homepage, item.GitHub.URL, item.InstallCount, item.RatingAverage,
			item.Official, item.Validated, item.Featured, item.ResourcesCount); ok {
			result.Items = append(result.Items, normalized)
		}
	}
	if categories, categoryErr := service.marketplaceCategories(ctx, input.Query, locale); categoryErr == nil {
		result.Categories = categories
	}
	return result, nil
}

func (service *Service) marketplaceCategories(
	ctx context.Context,
	search string,
	locale string,
) ([]MarketplaceCategory, error) {
	query := url.Values{"locale": []string{locale}}
	if search != "" {
		query.Set("q", search)
	}
	data, err := service.lobeMarketplace.FetchSkillMarketJSON(
		ctx, "/api/v1/skills/categories?"+query.Encode(), maxSkillMarketplaceJSONBytes,
	)
	if err != nil {
		return nil, err
	}
	var raw []MarketplaceCategory
	if json.Unmarshal(data, &raw) != nil || len(raw) > maxMarketplaceCategories {
		return nil, ErrSourceUnavailable
	}
	counts := make(map[string]int, len(curatedMarketplaceCategories))
	for _, item := range raw {
		item.Category = strings.TrimSpace(item.Category)
		if item.Category == "" || len([]rune(item.Category)) > 128 || item.Count < 0 ||
			!validPlainText(item.Category) || !isCuratedMarketplaceCategory(item.Category) {
			continue
		}
		if _, exists := counts[item.Category]; !exists {
			counts[item.Category] = item.Count
		}
	}
	result := make([]MarketplaceCategory, 0, len(counts))
	for _, category := range curatedMarketplaceCategories {
		if count, exists := counts[category]; exists {
			result = append(result, MarketplaceCategory{Category: category, Count: count})
		}
	}
	return result, nil
}

func (service *Service) GetMarketplaceSkill(
	ctx context.Context,
	userID, identifier, version string,
) (MarketplaceSkillDetail, error) {
	return service.GetMarketplaceSkillLocalized(ctx, userID, identifier, version, "")
}

func (service *Service) GetMarketplaceSkillLocalized(
	ctx context.Context,
	userID, identifier, version, locale string,
) (MarketplaceSkillDetail, error) {
	if service == nil || service.lobeMarketplace == nil || service.repository == nil {
		return MarketplaceSkillDetail{}, ErrUnavailable
	}
	userID, identifier, version = strings.TrimSpace(userID), strings.TrimSpace(identifier), strings.TrimSpace(version)
	locale, localeOK := normalizeMarketplaceLocale(locale)
	if !validUserID(userID) || !lobeIdentifierPattern.MatchString(identifier) ||
		(version != "" && !semverPattern.MatchString(version)) || !localeOK {
		return MarketplaceSkillDetail{}, validationError("INVALID_SKILL_MARKETPLACE_ITEM", "Skill Marketplace item is invalid")
	}
	query := url.Values{"locale": []string{locale}}
	if version != "" {
		query.Set("version", version)
	}
	data, err := service.lobeMarketplace.FetchPublicSkillDetailJSON(ctx,
		"/api/v1/skills/"+url.PathEscape(identifier)+"?"+query.Encode(), maxSkillMarketplaceJSONBytes)
	if err != nil {
		return MarketplaceSkillDetail{}, ErrSourceUnavailable
	}
	var raw lobeSkillDetail
	if json.Unmarshal(data, &raw) != nil || raw.Identifier != identifier ||
		!semverPattern.MatchString(strings.TrimSpace(raw.Version)) ||
		(version != "" && raw.Version != version) {
		return MarketplaceSkillDetail{}, ErrPackageChanged
	}
	summary, ok := normalizeMarketplaceSummary(raw.Identifier, raw.Name, raw.Description,
		raw.Version, raw.Category, raw.Author.Name, firstValue(raw.Icon, raw.Logo),
		firstValue(raw.License.Name, raw.Manifest.License), raw.Homepage,
		firstValue(raw.Repository, raw.GitHub.URL, raw.Manifest.Repository), raw.InstallCount,
		raw.RatingAverage, raw.Official, raw.Validated, raw.Featured, len(raw.Resources))
	if !ok || !skillNamePattern.MatchString(strings.TrimSpace(raw.Manifest.Name)) {
		return MarketplaceSkillDetail{}, ErrSourceUnavailable
	}
	detail := MarketplaceSkillDetail{
		MarketplaceSkillSummary: summary, ManifestName: strings.TrimSpace(raw.Manifest.Name),
		Summary:     boundedMarketplaceString(raw.Overview.Summary, 4096),
		Permissions: []string{}, Resources: []MarketplaceSkillResource{},
		Versions: []MarketplaceSkillVersion{}, Source: SourceLobeHub,
		SourceURL: "https://lobehub.com/skills/" + url.PathEscape(identifier),
	}
	detail.packageSourceURL = validMarketplaceGitHubSourceURL(
		raw.Manifest.SourceURL, detail.ManifestName,
	)
	for _, permission := range raw.Manifest.Permissions {
		permission = boundedMarketplaceString(permission, 256)
		if permission != "" && len(detail.Permissions) < 64 {
			detail.Permissions = append(detail.Permissions, permission)
		}
	}
	paths := make([]string, 0, len(raw.Resources))
	for resourcePath := range raw.Resources {
		paths = append(paths, resourcePath)
	}
	sort.Strings(paths)
	for _, resourcePath := range paths {
		resource := raw.Resources[resourcePath]
		if len(detail.Resources) >= maxMarketplaceResources || resource.Size < 0 ||
			len(resourcePath) > 512 || strings.TrimSpace(resource.FileHash) == "" {
			continue
		}
		detail.Resources = append(detail.Resources, MarketplaceSkillResource{
			Path: resourcePath, SHA256: strings.TrimSpace(resource.FileHash), Size: resource.Size,
		})
	}
	for _, item := range raw.Versions {
		if !semverPattern.MatchString(strings.TrimSpace(item.Version)) || item.VersionNumber < 0 ||
			!validOptionalTimestamp(item.CreatedAt) || len(detail.Versions) >= 100 {
			continue
		}
		detail.Versions = append(detail.Versions, MarketplaceSkillVersion{
			Version: item.Version, Latest: item.Latest, Validated: item.Validated,
			CreatedAt: item.CreatedAt, VersionRank: item.VersionNumber,
		})
	}
	detail.Installed = service.marketplaceSkillInstalled(ctx, userID, identifier, raw.Version)
	return detail, nil
}

func (service *Service) InstallMarketplaceSkill(
	ctx context.Context,
	userID, identifier, version string,
) (Installation, error) {
	detail, err := service.GetMarketplaceSkill(ctx, userID, identifier, version)
	if err != nil {
		return Installation{}, err
	}
	if detail.Version != strings.TrimSpace(version) {
		return Installation{}, ErrPackageChanged
	}
	if detail.packageSourceURL == "" {
		return Installation{}, ErrInvalidSource
	}
	source, err := service.direct.fetchGitHubSubtree(
		ctx, detail.packageSourceURL, detail.ManifestName,
	)
	if err != nil {
		return Installation{}, err
	}
	source.Type = SourceLobeHub
	source.Ref = "lobehub:" + identifier + "@" + detail.Version
	source.Identifier = identifier
	source.Version = detail.Version
	source.ExpectedName = detail.ManifestName
	candidate, err := service.ingest(ctx, strings.TrimSpace(userID), source)
	if err != nil {
		return Installation{}, err
	}
	return service.installPrivateCandidate(ctx, strings.TrimSpace(userID), candidate)
}

func (service *Service) InstallSkillLink(
	ctx context.Context,
	userID, rawURL string,
) (Installation, error) {
	if identifier, err := ParseLobeHubSkillURL(rawURL); err == nil {
		detail, detailErr := service.GetMarketplaceSkill(ctx, userID, identifier, "")
		if detailErr != nil {
			return Installation{}, detailErr
		}
		return service.InstallMarketplaceSkill(ctx, userID, identifier, detail.Version)
	}
	if coordinate, err := ParseGitHubSkillURL(rawURL); err == nil {
		return service.InstallDirectSkillLink(ctx, userID, rawURL, coordinate.ExpectedName)
	}
	return Installation{}, ErrInvalidSource
}

func (service *Service) marketplaceSkillInstalled(
	ctx context.Context,
	userID, identifier, version string,
) bool {
	candidate, err := service.repository.GetCandidateBySource(
		ctx, SourceLobeHub, "lobehub:"+identifier+"@"+version, userID,
	)
	if err != nil {
		return false
	}
	library, err := service.repository.ListLibrary(ctx, userID)
	if err != nil {
		return false
	}
	for _, item := range library {
		if item.AdmissionID == candidate.ID && item.PackageFingerprint == candidate.Package.PackageFingerprint {
			return true
		}
	}
	return false
}

func normalizeMarketplaceSummary(
	identifier, name, description, version, category, author, icon, license,
	homepage, repository string,
	installCount int,
	rating float64,
	official, validated, featured bool,
	resourceCount int,
) (MarketplaceSkillSummary, bool) {
	identifier, name, version = strings.TrimSpace(identifier), strings.TrimSpace(name), strings.TrimSpace(version)
	if !lobeIdentifierPattern.MatchString(identifier) || name == "" || len([]rune(name)) > 256 ||
		!semverPattern.MatchString(version) || installCount < 0 || resourceCount < 0 ||
		rating < 0 || rating > 5 || !validPlainText(name) {
		return MarketplaceSkillSummary{}, false
	}
	return MarketplaceSkillSummary{
		Identifier: identifier, Name: name,
		Description: boundedMarketplaceString(description, 2048), Version: version,
		Category: boundedMarketplaceString(category, 128), Author: boundedMarketplaceString(author, 256),
		Icon: boundedMarketplaceIcon(icon), License: boundedMarketplaceString(license, 128),
		Homepage: boundedMarketplaceHTTPSURL(homepage), RepositoryURL: boundedMarketplaceHTTPSURL(repository),
		InstallCount: installCount, Rating: rating, Official: official, Validated: validated,
		Featured: featured, ResourceCount: resourceCount,
	}, true
}

func boundedMarketplaceString(value string, maximum int) string {
	value = strings.TrimSpace(value)
	if value == "" || len([]rune(value)) > maximum || !validPlainText(value) {
		return ""
	}
	return value
}

func boundedMarketplaceHTTPSURL(value string) string {
	value = strings.TrimSpace(value)
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil ||
		len(value) > 2048 {
		return ""
	}
	return parsed.String()
}

func boundedMarketplaceIcon(value string) string {
	if parsed := boundedMarketplaceHTTPSURL(value); parsed != "" {
		return parsed
	}
	return boundedMarketplaceString(value, 16)
}

func firstValue(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func validOptionalTimestamp(value string) bool {
	if strings.TrimSpace(value) == "" {
		return true
	}
	_, err := time.Parse(time.RFC3339, value)
	return err == nil
}

func normalizeMarketplaceLocale(value string) (string, bool) {
	switch strings.TrimSpace(value) {
	case "", "zh", "zh-CN":
		return "zh-CN", true
	case "en", "en-US":
		return "en-US", true
	case "ja", "ja-JP":
		return "ja-JP", true
	default:
		return "", false
	}
}

func normalizeMarketplaceSort(value string) (string, bool) {
	switch strings.TrimSpace(value) {
	case "", "relevance":
		return "relevance", true
	case "recommended", "installCount", "ratingAverage", "updatedAt":
		return strings.TrimSpace(value), true
	default:
		return "", false
	}
}
