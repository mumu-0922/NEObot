package resourceorchestrator

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"

	"neo-chat/mm-chat/backend/internal/mcpclient"
	"neo-chat/mm-chat/backend/internal/skillsupply"
)

func (service *Service) MutateExplicit(
	ctx context.Context,
	request MutationRequest,
) (MutationResult, error) {
	if service == nil || !service.mutationEnabled {
		return MutationResult{}, ErrDisabled
	}
	request.Kind = strings.ToLower(strings.TrimSpace(request.Kind))
	request.Action = strings.ToLower(strings.TrimSpace(request.Action))
	request.ID = strings.TrimSpace(request.ID)
	request.UserID = strings.TrimSpace(request.UserID)
	request.ConversationID = strings.TrimSpace(request.ConversationID)
	request.EntryPoint = strings.TrimSpace(request.EntryPoint)
	if request.ID == "" || request.UserID == "" || request.ConversationID == "" ||
		!validMutationAction(request.Kind, request.Action) {
		return MutationResult{}, ErrInvalidQuery
	}
	result, err := service.mutateExplicit(ctx, request)
	audit := MutationAudit{
		UserID: request.UserID, ConversationID: request.ConversationID,
		EntryPoint: request.EntryPoint, Kind: request.Kind, Action: request.Action,
		CandidateID:   request.ID,
		ExactRevision: mutationRevision(request.ExpectedRevision),
	}
	auditID, auditErr := service.recordMutation(ctx, audit, result.ID, err)
	result.MutationAuditID = auditID
	if auditErr != nil {
		return result, auditErr
	}
	return result, err
}

func (service *Service) mutateExplicit(
	ctx context.Context,
	request MutationRequest,
) (MutationResult, error) {
	switch request.Kind {
	case KindSkill:
		return service.removeSkill(ctx, request)
	case KindMCP:
		return service.mutateMCP(ctx, request)
	default:
		return MutationResult{}, ErrInvalidQuery
	}
}

func (service *Service) removeSkill(
	ctx context.Context,
	request MutationRequest,
) (MutationResult, error) {
	if service.skills == nil || request.Action != ActionRemove || request.ExpectedRevision < 1 {
		return MutationResult{}, ErrInvalidQuery
	}
	installed, found, err := service.installedSkillByID(ctx, request.UserID, request.ID)
	if err != nil {
		return MutationResult{}, err
	}
	if !found {
		return MutationResult{}, ErrForbidden
	}
	if installed.Revision != request.ExpectedRevision {
		return MutationResult{}, ErrRevisionChanged
	}
	if err := service.skills.Uninstall(
		ctx, request.UserID, request.ID, request.ExpectedRevision,
	); err != nil {
		if errors.Is(err, skillsupply.ErrRevisionConflict) {
			return MutationResult{}, ErrRevisionChanged
		}
		return MutationResult{}, err
	}
	return MutationResult{
		Kind: KindSkill, Action: ActionRemove, ID: installed.ID, Name: installed.Name,
		Revision: request.ExpectedRevision, Status: "removed", RefreshRequired: true,
	}, nil
}

func (service *Service) mutateMCP(
	ctx context.Context,
	request MutationRequest,
) (MutationResult, error) {
	if service.mcp == nil {
		return MutationResult{}, ErrUnavailable
	}
	ref, valid := parseMCPServerRef(request.ID)
	if !valid {
		return MutationResult{}, ErrInvalidQuery
	}
	servers, err := service.mcp.ListServers(ctx, request.UserID, request.ConversationID)
	if err != nil {
		return MutationResult{}, err
	}
	server, found := findMCPServer(servers, ref)
	if !found {
		return MutationResult{}, ErrForbidden
	}
	if request.Action == ActionRemove {
		return service.removeMCP(ctx, request, server)
	}
	return service.toggleMCP(ctx, request, server)
}

func (service *Service) removeMCP(
	ctx context.Context,
	request MutationRequest,
	server mcpclient.Server,
) (MutationResult, error) {
	if server.Ref.Source != mcpclient.SourcePrivate || !server.CanManage {
		return MutationResult{}, ErrForbidden
	}
	if err := service.mcp.DeletePrivateServer(ctx, request.UserID, server.Ref.ID); err != nil {
		return MutationResult{}, err
	}
	return MutationResult{
		Kind: KindMCP, Action: ActionRemove, ID: server.Ref.Key(), Name: server.Name,
		Status: "removed", RefreshRequired: true,
	}, nil
}

func (service *Service) toggleMCP(
	ctx context.Context,
	request MutationRequest,
	server mcpclient.Server,
) (MutationResult, error) {
	selection, err := service.mcp.GetSelection(ctx, request.UserID, request.ConversationID)
	if err != nil {
		return MutationResult{}, err
	}
	if selection.Revision != request.ExpectedRevision {
		return MutationResult{}, ErrRevisionChanged
	}
	selected := append([]mcpclient.SelectionServer(nil), selection.Servers...)
	if selection.Mode != mcpclient.SelectionModeCustom {
		selected = []mcpclient.SelectionServer{}
	}
	status := "disabled"
	if request.Action == ActionEnable {
		if server.Status != mcpclient.ServerStatusReady {
			return MutationResult{}, ErrConfigurationRequired
		}
		if !selectionContains(selected, server.Ref) {
			selected = append(selected, mcpclient.SelectionServer{Ref: server.Ref})
		}
		status = "enabled"
	} else {
		selected = removeSelection(selected, server.Ref)
	}
	updated, err := service.mcp.ReplaceSelection(ctx, request.UserID, mcpclient.Selection{
		ConversationID: request.ConversationID, Mode: mcpclient.SelectionModeCustom,
		Revision: request.ExpectedRevision, Servers: selected,
	})
	if err != nil {
		return MutationResult{}, err
	}
	return MutationResult{
		Kind: KindMCP, Action: request.Action, ID: server.Ref.Key(), Name: server.Name,
		Revision: updated.Revision, Status: status, RefreshRequired: true,
	}, nil
}

func (service *Service) recordMutation(
	ctx context.Context,
	audit MutationAudit,
	resultResourceID string,
	mutationErr error,
) (string, error) {
	if service.auditor == nil {
		return "", mutationErr
	}
	audit.ID = service.newAuditID()
	audit.Outcome = MutationOutcomeSuccess
	audit.OccurredAt = service.now().UTC()
	audit.ResultResourceID = resultResourceID
	if mutationErr != nil {
		audit.Outcome = MutationOutcomeFailed
		audit.ErrorCode = mutationErrorCode(mutationErr)
	}
	auditCtx, cancelAudit := context.WithTimeout(context.WithoutCancel(ctx), mutationAuditTimeout)
	defer cancelAudit()
	if err := service.auditor.RecordMutation(auditCtx, audit); err != nil {
		slog.WarnContext(auditCtx, "resource_mutation_audit_failed",
			slog.String("failure_code", mutationAuditFailureCode(err)),
			slog.String("error_type", fmt.Sprintf("%T", err)),
		)
		if mutationErr != nil {
			return "", mutationErr
		}
		return "", ErrAuditUnavailable
	}
	return audit.ID, mutationErr
}

func mutationAuditFailureCode(err error) string {
	var pgError *pgconn.PgError
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "deadline_exceeded"
	case errors.Is(err, context.Canceled):
		return "cancelled"
	case errors.As(err, &pgError):
		return "postgres_" + pgError.Code
	default:
		return "internal"
	}
}

func (service *Service) installedSkillByID(
	ctx context.Context,
	userID string,
	installationID string,
) (skillsupply.Installation, bool, error) {
	library, err := service.skills.ListLibrary(ctx, userID)
	if err != nil {
		return skillsupply.Installation{}, false, err
	}
	for _, installed := range library {
		if installed.ID == installationID {
			return installed, true, nil
		}
	}
	return skillsupply.Installation{}, false, nil
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
	case errors.Is(err, ErrForbidden):
		return "forbidden"
	default:
		return "mutation_failed"
	}
}

func validMutationAction(kind, action string) bool {
	switch kind {
	case KindSkill:
		return action == ActionRemove
	case KindMCP:
		return action == ActionEnable || action == ActionDisable || action == ActionRemove
	default:
		return false
	}
}

func mutationRevision(revision int64) string {
	if revision < 1 {
		return ""
	}
	return fmt.Sprintf("cas:%d", revision)
}

func parseMCPServerRef(value string) (mcpclient.ServerRef, bool) {
	parts := strings.SplitN(strings.TrimSpace(value), ":", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[1]) == "" {
		return mcpclient.ServerRef{}, false
	}
	source := strings.TrimSpace(parts[0])
	if source != mcpclient.SourceCatalog && source != mcpclient.SourceManifest &&
		source != mcpclient.SourcePrivate {
		return mcpclient.ServerRef{}, false
	}
	return mcpclient.ServerRef{Source: source, ID: strings.TrimSpace(parts[1])}, true
}

func findMCPServer(
	servers []mcpclient.Server,
	ref mcpclient.ServerRef,
) (mcpclient.Server, bool) {
	for _, server := range servers {
		if server.Ref == ref {
			return server, true
		}
	}
	return mcpclient.Server{}, false
}

func selectionContains(items []mcpclient.SelectionServer, ref mcpclient.ServerRef) bool {
	for _, item := range items {
		if item.Ref == ref {
			return true
		}
	}
	return false
}

func removeSelection(
	items []mcpclient.SelectionServer,
	ref mcpclient.ServerRef,
) []mcpclient.SelectionServer {
	result := make([]mcpclient.SelectionServer, 0, len(items))
	for _, item := range items {
		if item.Ref != ref {
			result = append(result, item)
		}
	}
	return result
}
