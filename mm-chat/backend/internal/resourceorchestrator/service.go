package resourceorchestrator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"neo-chat/mm-chat/backend/internal/mcpclient"
	"neo-chat/mm-chat/backend/internal/skillsupply"
)

const (
	KindSkill            = "skill"
	KindMCP              = "mcp"
	ActionInstall        = "install"
	ActionRemove         = "remove"
	ActionEnable         = "enable"
	ActionDisable        = "disable"
	MaxItems             = 5
	maxSkillSearchPages  = 5
	mutationAuditTimeout = 3 * time.Second
)

type SkillSource interface {
	ListLibrary(context.Context, string) ([]skillsupply.Installation, error)
	ListStore(context.Context, int, int) (skillsupply.StoreResult, error)
	GetStoreItem(context.Context, string) (skillsupply.Candidate, error)
	Install(context.Context, string, string, string) (skillsupply.Installation, error)
	Uninstall(context.Context, string, string, int64) error
}

type MCPSource interface {
	ListServers(context.Context, string, string) ([]mcpclient.Server, error)
	GetSelection(context.Context, string, string) (mcpclient.Selection, error)
	SearchMarketplace(context.Context, mcpclient.MarketplaceSearchInput) (mcpclient.MarketplaceSearchResult, error)
	MarketplaceItem(context.Context, string, string, string) (mcpclient.MarketplaceItemDetail, error)
	InstallMarketplaceItem(
		context.Context,
		string,
		mcpclient.MarketplaceInstallInput,
	) (mcpclient.MarketplaceInstallResult, error)
	ReplaceSelection(context.Context, string, mcpclient.Selection) (mcpclient.Selection, error)
	DeletePrivateServer(context.Context, string, string) error
}

type Service struct {
	skills          SkillSource
	mcp             MCPSource
	mutationEnabled bool
	auditor         MutationAuditor
	newAuditID      func() string
	now             func() time.Time
}

type ServiceOption func(*Service)

func WithMutationEnabled(enabled bool) ServiceOption {
	return func(service *Service) { service.mutationEnabled = enabled }
}

func WithMutationAuditor(auditor MutationAuditor) ServiceOption {
	return func(service *Service) { service.auditor = auditor }
}

func NewService(skills SkillSource, mcp MCPSource, options ...ServiceOption) *Service {
	service := &Service{
		skills: skills, mcp: mcp, mutationEnabled: true,
		newAuditID: defaultMutationAuditID, now: time.Now,
	}
	for _, option := range options {
		if option != nil {
			option(service)
		}
	}
	return service
}

type SkillResource struct {
	InstallationID     string   `json:"installationId"`
	Name               string   `json:"name"`
	Version            string   `json:"version"`
	PackageFingerprint string   `json:"packageFingerprint"`
	Description        string   `json:"description"`
	AllowedTools       []string `json:"allowedTools"`
	Revision           int64    `json:"revision"`
}

type MCPResource struct {
	Ref              mcpclient.ServerRef `json:"ref"`
	Name             string              `json:"name"`
	Description      string              `json:"description,omitempty"`
	AuthType         string              `json:"authType"`
	Status           string              `json:"status"`
	CredentialStatus string              `json:"credentialStatus"`
	ToolCount        int                 `json:"toolCount"`
	UnsupportedCount int                 `json:"unsupportedToolCount"`
	Selected         bool                `json:"selected"`
	DisabledTools    []string            `json:"disabledTools"`
	CanManage        bool                `json:"canManage"`
}

type Catalog struct {
	Revision      string          `json:"revision"`
	SelectionMode string          `json:"selectionMode"`
	Skills        []SkillResource `json:"skills"`
	MCPServers    []MCPResource   `json:"mcpServers"`
}

type SearchItem struct {
	Kind             string   `json:"kind"`
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	Description      string   `json:"description"`
	Version          string   `json:"version,omitempty"`
	ExactRevision    string   `json:"exactRevision"`
	Status           string   `json:"status"`
	Source           string   `json:"source"`
	AuthType         string   `json:"authType,omitempty"`
	PermissionScopes []string `json:"permissionScopes"`
}

type SearchResult struct {
	Kind  string       `json:"kind"`
	Query string       `json:"query"`
	Items []SearchItem `json:"items"`
}

type InstallRequest struct {
	Kind           string
	ID             string
	Version        string
	ExactRevision  string
	UserID         string
	ConversationID string
	EntryPoint     string
}

type InstallResult struct {
	Kind            string `json:"kind"`
	ID              string `json:"id"`
	Name            string `json:"name"`
	Revision        string `json:"revision"`
	Status          string `json:"status"`
	RefreshRequired bool   `json:"refreshRequired"`
	MutationAuditID string `json:"mutationAuditId,omitempty"`
}

type MutationRequest struct {
	Kind             string
	Action           string
	ID               string
	ExpectedRevision int64
	UserID           string
	ConversationID   string
	EntryPoint       string
}

type MutationResult struct {
	Kind            string `json:"kind"`
	Action          string `json:"action"`
	ID              string `json:"id"`
	Name            string `json:"name"`
	Revision        int64  `json:"revision"`
	Status          string `json:"status"`
	RefreshRequired bool   `json:"refreshRequired"`
	MutationAuditID string `json:"mutationAuditId,omitempty"`
}

func (service *Service) Catalog(
	ctx context.Context,
	userID string,
	conversationID string,
) (Catalog, error) {
	catalog := Catalog{Skills: []SkillResource{}, MCPServers: []MCPResource{}}
	if service != nil && service.skills != nil {
		installed, err := service.skills.ListLibrary(ctx, userID)
		if err != nil {
			return Catalog{}, err
		}
		for _, skill := range installed {
			catalog.Skills = append(catalog.Skills, SkillResource{
				InstallationID: skill.ID,
				Name:           skill.Name, Version: skill.Version,
				PackageFingerprint: skill.PackageFingerprint,
				Description:        skill.Description,
				AllowedTools:       append([]string(nil), skill.AllowedTools...),
				Revision:           skill.Revision,
			})
		}
	}
	if service != nil && service.mcp != nil {
		servers, err := service.mcp.ListServers(ctx, userID, conversationID)
		if err != nil {
			return Catalog{}, err
		}
		selection, err := service.mcp.GetSelection(ctx, userID, conversationID)
		if err != nil {
			return Catalog{}, err
		}
		catalog.SelectionMode = selection.Mode
		selected := make(map[string]mcpclient.SelectionServer, len(selection.Servers))
		for _, item := range selection.Servers {
			selected[item.Ref.Key()] = item
		}
		for _, server := range servers {
			selectionServer, isSelected := selected[server.Ref.Key()]
			credentialStatus := "not_required"
			if server.AuthType != mcpclient.AuthNone {
				credentialStatus = "required"
				if server.HasCredential {
					credentialStatus = "configured"
				}
			}
			catalog.MCPServers = append(catalog.MCPServers, MCPResource{
				Ref: server.Ref, Name: server.Name, Description: server.Description,
				AuthType: server.AuthType, Status: server.Status,
				CredentialStatus: credentialStatus, ToolCount: server.ToolCount,
				UnsupportedCount: server.UnsupportedCount,
				Selected:         isSelected, DisabledTools: append([]string(nil), selectionServer.DisabledTools...),
				CanManage: server.CanManage,
			})
		}
	}
	sort.Slice(catalog.Skills, func(i, j int) bool {
		return catalog.Skills[i].Name < catalog.Skills[j].Name
	})
	sort.Slice(catalog.MCPServers, func(i, j int) bool {
		return catalog.MCPServers[i].Ref.Key() < catalog.MCPServers[j].Ref.Key()
	})
	catalog.Revision = revision(catalog)
	return catalog, nil
}

func (service *Service) Search(ctx context.Context, userID, kind, query string) (SearchResult, error) {
	kind = strings.ToLower(strings.TrimSpace(kind))
	query = strings.Join(strings.Fields(query), " ")
	if linkedIdentifier, supported := supportedResourceLinkIdentifier(kind, query); supported {
		query = linkedIdentifier
	}
	result := SearchResult{Kind: kind, Query: query, Items: []SearchItem{}}
	switch kind {
	case KindSkill:
		if service == nil || service.skills == nil {
			return SearchResult{}, ErrUnavailable
		}
		for page := 1; page <= maxSkillSearchPages && len(result.Items) < MaxItems; page++ {
			store, err := service.skills.ListStore(ctx, page, 20)
			if err != nil {
				return SearchResult{}, err
			}
			for _, candidate := range store.Items {
				if !matches(query, candidate.ID, candidate.Package.Name, candidate.Package.Description) {
					continue
				}
				permissions := append([]string(nil), candidate.Package.AllowedTools...)
				result.Items = append(result.Items, SearchItem{
					Kind: KindSkill, ID: candidate.ID, Name: candidate.Package.Name,
					Description: candidate.Package.Description, Version: candidate.Package.Version,
					ExactRevision: candidate.Package.PackageFingerprint,
					Status:        candidate.Status, Source: candidate.SourceType,
					PermissionScopes: permissions,
				})
				if len(result.Items) == MaxItems {
					break
				}
			}
			if page >= store.TotalPages || len(store.Items) == 0 {
				break
			}
		}
	case KindMCP:
		if service == nil || service.mcp == nil {
			return SearchResult{}, ErrUnavailable
		}
		marketplace, err := service.mcp.SearchMarketplace(ctx, mcpclient.MarketplaceSearchInput{
			Query: query, Page: 1, PageSize: MaxItems,
		})
		if err != nil {
			return SearchResult{}, err
		}
		for _, item := range marketplace.Items {
			detail, detailErr := service.mcp.MarketplaceItem(ctx, userID, item.Identifier, "")
			if detailErr != nil {
				continue
			}
			status := "incompatible"
			exactRevision := ""
			authType := ""
			for _, deployment := range detail.Deployments {
				if deployment.Recommended || exactRevision == "" {
					status = deployment.Compatibility
					exactRevision = deployment.Hash
					authType = marketplaceAuthType(deployment.InstallMode)
				}
				if deployment.Recommended {
					break
				}
			}
			permissions := make([]string, 0, len(detail.Tools))
			for _, tool := range detail.Tools {
				permissions = append(permissions, tool.Name)
			}
			result.Items = append(result.Items, SearchItem{
				Kind: KindMCP, ID: item.Identifier, Name: item.Name,
				Description: item.Description, Version: detail.Version,
				ExactRevision: exactRevision, Status: status, Source: marketplace.Source,
				AuthType: authType, PermissionScopes: permissions,
			})
		}
	default:
		return SearchResult{}, ErrInvalidQuery
	}
	return result, nil
}

func marketplaceAuthType(installMode string) string {
	switch strings.TrimSpace(installMode) {
	case "direct":
		return mcpclient.AuthNone
	case "header":
		return mcpclient.AuthHeader
	case "oauth":
		return mcpclient.AuthOAuth
	case "runner_env":
		return mcpclient.AuthEnv
	default:
		return ""
	}
}

func matches(query string, values ...string) bool {
	if query == "" {
		return true
	}
	haystack := strings.ToLower(strings.Join(values, " "))
	for _, term := range strings.Fields(strings.ToLower(query)) {
		if !strings.Contains(haystack, term) {
			return false
		}
	}
	return true
}

func revision(value any) string {
	encoded, _ := json.Marshal(value)
	digest := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(digest[:])
}
