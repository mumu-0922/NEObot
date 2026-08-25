package resourceorchestrator

import (
	"context"
	"errors"
	"strings"

	"neo-chat/mm-chat/backend/internal/mcpclient"
	"neo-chat/mm-chat/backend/internal/skillsupply"
)

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
	audit := MutationAudit{
		UserID: request.UserID, ConversationID: request.ConversationID,
		EntryPoint: request.EntryPoint, Kind: request.Kind, Action: ActionInstall,
		CandidateID: request.ID,
		Version:     request.Version, ExactRevision: request.ExactRevision,
	}
	auditID, auditErr := service.recordMutation(ctx, audit, result.ID, err)
	result.MutationAuditID = auditID
	if auditErr != nil {
		return result, auditErr
	}
	return result, err
}

func (service *Service) CompleteConfiguredMCP(
	ctx context.Context,
	request InstallRequest,
	resourceID string,
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
	resourceID = strings.TrimSpace(resourceID)
	if request.Kind != KindMCP || request.ID == "" || request.Version == "" ||
		request.ExactRevision == "" || request.UserID == "" ||
		request.ConversationID == "" || resourceID == "" {
		return InstallResult{}, ErrInvalidQuery
	}
	result, err := service.completeConfiguredMCP(ctx, request, resourceID)
	audit := MutationAudit{
		UserID: request.UserID, ConversationID: request.ConversationID,
		EntryPoint: request.EntryPoint, Kind: request.Kind, Action: ActionInstall,
		CandidateID: request.ID, Version: request.Version,
		ExactRevision: request.ExactRevision,
	}
	auditID, auditErr := service.recordMutation(ctx, audit, result.ID, err)
	result.MutationAuditID = auditID
	if auditErr != nil {
		return result, auditErr
	}
	return result, err
}

func (service *Service) installExplicit(
	ctx context.Context,
	request InstallRequest,
) (InstallResult, error) {
	switch request.Kind {
	case KindSkill:
		return service.installSkill(ctx, request)
	case KindMCP:
		return service.installMCP(ctx, request)
	default:
		return InstallResult{}, ErrInvalidQuery
	}
}

func (service *Service) installSkill(
	ctx context.Context,
	request InstallRequest,
) (InstallResult, error) {
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
}

func (service *Service) installMCP(
	ctx context.Context,
	request InstallRequest,
) (InstallResult, error) {
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
	if deployment.Compatibility != mcpclient.MarketplaceCompatibilityInstallable &&
		deployment.Compatibility != mcpclient.MarketplaceCompatibilityNeedsConfig {
		return InstallResult{}, ErrConfigurationRequired
	}
	selection, err := service.mcp.GetSelection(ctx, request.UserID, request.ConversationID)
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
		return InstallResult{
			Kind: KindMCP, ID: installed.Server.Ref.Key(), Name: installed.Server.Name,
			Revision: request.ExactRevision, Status: "configuration_required",
		}, ErrConfigurationRequired
	}
	return InstallResult{
		Kind: KindMCP, ID: installed.Server.Ref.Key(), Name: installed.Server.Name,
		Revision: request.ExactRevision, Status: "installed", RefreshRequired: true,
	}, nil
}

func (service *Service) completeConfiguredMCP(
	ctx context.Context,
	request InstallRequest,
	resourceID string,
) (InstallResult, error) {
	if service.mcp == nil {
		return InstallResult{}, ErrUnavailable
	}
	detail, err := service.mcp.MarketplaceItem(ctx, request.UserID, request.ID, request.Version)
	if err != nil {
		return InstallResult{}, err
	}
	if !hasExactDeployment(detail.Deployments, request.ExactRevision) {
		return InstallResult{}, ErrRevisionChanged
	}
	ref, valid := parseMCPServerRef(resourceID)
	if !valid || ref.Source != mcpclient.SourcePrivate {
		return InstallResult{}, ErrForbidden
	}
	servers, err := service.mcp.ListServers(ctx, request.UserID, request.ConversationID)
	if err != nil {
		return InstallResult{}, err
	}
	server, found := findMCPServer(servers, ref)
	if !found || !server.CanManage || !marketplaceMetadataMatches(
		server.Metadata, request.ID, request.Version, request.ExactRevision,
	) {
		return InstallResult{}, ErrForbidden
	}
	pending := InstallResult{
		Kind: KindMCP, ID: ref.Key(), Name: server.Name,
		Revision: request.ExactRevision, Status: "configuration_required",
	}
	if server.Status != mcpclient.ServerStatusReady ||
		(server.AuthType != mcpclient.AuthNone && !server.HasCredential) {
		return pending, ErrConfigurationRequired
	}
	if err := service.enableConfiguredMCP(ctx, request, ref); err != nil {
		return InstallResult{}, err
	}
	return InstallResult{
		Kind: KindMCP, ID: ref.Key(), Name: server.Name,
		Revision: request.ExactRevision, Status: "installed", RefreshRequired: true,
	}, nil
}

func (service *Service) enableConfiguredMCP(
	ctx context.Context,
	request InstallRequest,
	ref mcpclient.ServerRef,
) error {
	selection, err := service.mcp.GetSelection(ctx, request.UserID, request.ConversationID)
	if err != nil || selectionContains(selection.Servers, ref) {
		return err
	}
	selected := append([]mcpclient.SelectionServer(nil), selection.Servers...)
	if selection.Mode != mcpclient.SelectionModeCustom {
		selected = []mcpclient.SelectionServer{}
	}
	selected = append(selected, mcpclient.SelectionServer{Ref: ref})
	_, err = service.mcp.ReplaceSelection(ctx, request.UserID, mcpclient.Selection{
		ConversationID: request.ConversationID, Mode: mcpclient.SelectionModeCustom,
		Revision: selection.Revision, Servers: selected,
	})
	return err
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

func hasExactDeployment(deployments []mcpclient.MarketplaceDeployment, exactRevision string) bool {
	for _, deployment := range deployments {
		if deployment.Hash == exactRevision {
			return true
		}
	}
	return false
}

func marketplaceMetadataMatches(
	metadata map[string]any,
	identifier string,
	version string,
	deploymentHash string,
) bool {
	marketplace, ok := metadata["marketplace"].(map[string]any)
	if !ok {
		return false
	}
	return strings.TrimSpace(stringMetadataField(marketplace, "identifier")) == identifier &&
		strings.TrimSpace(stringMetadataField(marketplace, "version")) == version &&
		strings.TrimSpace(stringMetadataField(marketplace, "deploymentHash")) == deploymentHash
}

func stringMetadataField(metadata map[string]any, key string) string {
	value, _ := metadata[key].(string)
	return value
}
