package resourceorchestrator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"neo-chat/mm-chat/backend/internal/mcpclient"
	"neo-chat/mm-chat/backend/internal/skillsupply"
)

const (
	KindSkill            = "skill"
	KindMCP              = "mcp"
	MaxItems             = 5
	maxSkillSearchPages  = 5
	mutationAuditTimeout = 3 * time.Second
)

type SkillSource interface {
	ListLibrary(context.Context, string) ([]skillsupply.Installation, error)
	ListStore(context.Context, int, int) (skillsupply.StoreResult, error)
	GetStoreItem(context.Context, string) (skillsupply.Candidate, error)
	Install(context.Context, string, string, string) (skillsupply.Installation, error)
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

func (service *Service) InstallExplicit(
	ctx context.Context,
	request InstallRequest,
) (InstallResult, error) {
	if service == nil || !service.mutationEnabled {
		return InstallResult{}, ErrDisabled
	}
	request.Kind = strings.ToLower(strings.TrimSpace(request.Kind))
	request.ID = strings.TrimSpace(request.ID)
	request.Version = strings.TrimSpace(request.Version)
	request.ExactRevision = strings.TrimSpace(request.ExactRevision)
	request.UserID = strings.TrimSpace(request.UserID)
	request.ConversationID = strings.TrimSpace(request.ConversationID)
	request.EntryPoint = strings.TrimSpace(request.EntryPoint)
	if request.ID == "" || request.ExactRevision == "" || request.UserID == "" ||
		request.ConversationID == "" {
		return InstallResult{}, ErrInvalidQuery
	}
	result, err := service.installExplicit(ctx, request)
	if service.auditor == nil {
		return result, err
	}
	auditID := service.newAuditID()
	audit := MutationAudit{
		ID: auditID, UserID: request.UserID, ConversationID: request.ConversationID,
		EntryPoint: request.EntryPoint, Kind: request.Kind, CandidateID: request.ID,
		Version: request.Version, ExactRevision: request.ExactRevision,
		Outcome: MutationOutcomeSuccess, OccurredAt: service.now().UTC(),
	}
	if err != nil {
		audit.Outcome = MutationOutcomeFailed
		audit.ErrorCode = mutationErrorCode(err)
	} else {
		audit.ResultResourceID = result.ID
	}
	auditCtx, cancelAudit := context.WithTimeout(context.WithoutCancel(ctx), mutationAuditTimeout)
	defer cancelAudit()
	if auditErr := service.auditor.RecordMutation(auditCtx, audit); auditErr != nil {
		if err != nil {
			return result, err
		}
		return result, ErrAuditUnavailable
	}
	result.MutationAuditID = auditID
	return result, err
}

func (service *Service) installExplicit(
	ctx context.Context,
	request InstallRequest,
) (InstallResult, error) {
	switch request.Kind {
	case KindSkill:
		if service.skills == nil {
			return InstallResult{}, ErrUnavailable
		}
		candidate, err := service.skills.GetStoreItem(ctx, request.ID)
		if err != nil {
			return InstallResult{}, err
		}
		if candidate.Package.PackageFingerprint != request.ExactRevision ||
			(request.Version != "" && candidate.Package.Version != request.Version) {
			return InstallResult{}, ErrRevisionChanged
		}
		if existing, found, err := service.installedSkill(
			ctx, request.UserID, candidate.ID, request.ExactRevision,
		); err != nil {
			return InstallResult{}, err
		} else if found {
			return skillInstallResult(existing), nil
		}
		installed, err := service.skills.Install(
			ctx, request.UserID, candidate.ID, request.ExactRevision,
		)
		if err != nil {
			if errors.Is(err, skillsupply.ErrInstallationConflict) {
				if existing, found, listErr := service.installedSkill(
					ctx, request.UserID, candidate.ID, request.ExactRevision,
				); listErr == nil && found {
					return skillInstallResult(existing), nil
				}
			}
			return InstallResult{}, err
		}
		return skillInstallResult(installed), nil
	case KindMCP:
		if service.mcp == nil {
			return InstallResult{}, ErrUnavailable
		}
		if request.ConversationID == "" || request.Version == "" {
			return InstallResult{}, ErrInvalidQuery
		}
		detail, err := service.mcp.MarketplaceItem(
			ctx, request.UserID, request.ID, request.Version,
		)
		if err != nil {
			return InstallResult{}, err
		}
		var deployment *mcpclient.MarketplaceDeployment
		for index := range detail.Deployments {
			if detail.Deployments[index].Hash == request.ExactRevision {
				deployment = &detail.Deployments[index]
				break
			}
		}
		if deployment == nil {
			return InstallResult{}, ErrRevisionChanged
		}
		if deployment.Compatibility != mcpclient.MarketplaceCompatibilityInstallable ||
			len(deployment.SecretFields) > 0 || deployment.InstallMode != "direct" {
			return InstallResult{}, ErrConfigurationRequired
		}
		selection, err := service.mcp.GetSelection(
			ctx, request.UserID, request.ConversationID,
		)
		if err != nil {
			return InstallResult{}, err
		}
		installed, err := service.mcp.InstallMarketplaceItem(
			ctx, request.UserID, mcpclient.MarketplaceInstallInput{
				Identifier: request.ID, Version: request.Version,
				ConversationID:    request.ConversationID,
				SelectionRevision: selection.Revision, EnableForConversation: true,
				DeploymentHash: request.ExactRevision,
			},
		)
		if err != nil {
			return InstallResult{}, err
		}
		if installed.ValidationError != "" || !installed.Enabled {
			return InstallResult{}, ErrConfigurationRequired
		}
		return InstallResult{
			Kind: KindMCP, ID: installed.Server.Ref.Key(), Name: installed.Server.Name,
			Revision: request.ExactRevision, Status: "installed", RefreshRequired: true,
		}, nil
	default:
		return InstallResult{}, ErrInvalidQuery
	}
}

func (service *Service) installedSkill(
	ctx context.Context,
	userID string,
	admissionID string,
	packageFingerprint string,
) (skillsupply.Installation, bool, error) {
	library, err := service.skills.ListLibrary(ctx, userID)
	if err != nil {
		return skillsupply.Installation{}, false, err
	}
	for _, installed := range library {
		if installed.AdmissionID == admissionID &&
			installed.PackageFingerprint == packageFingerprint {
			return installed, true, nil
		}
	}
	return skillsupply.Installation{}, false, nil
}

func skillInstallResult(installed skillsupply.Installation) InstallResult {
	return InstallResult{
		Kind: KindSkill, ID: installed.ID, Name: installed.Name,
		Revision: installed.PackageFingerprint, Status: "installed", RefreshRequired: true,
	}
}

func mutationErrorCode(err error) string {
	switch {
	case errors.Is(err, ErrInvalidQuery):
		return "invalid_request"
	case errors.Is(err, ErrRevisionChanged):
		return "revision_changed"
	case errors.Is(err, ErrConfigurationRequired):
		return "configuration_required"
	case errors.Is(err, ErrDisabled):
		return "disabled"
	case errors.Is(err, ErrUnavailable):
		return "unavailable"
	default:
		return "mutation_failed"
	}
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
