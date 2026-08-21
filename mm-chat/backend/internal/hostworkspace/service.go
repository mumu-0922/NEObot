package hostworkspace

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"neo-chat/mm-chat/backend/internal/agenthost"
)

const (
	maxNameBytes         = 200
	maxSystemPromptBytes = 256 << 10
	maxFilesBytes        = 1 << 20
	maxFiles             = 128
	maxColorBytes        = 64
)

var sha256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
var runnerIDPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{2,63}$`)

type Service struct {
	repository Repository
	resolver   PathResolver
	now        func() time.Time
}

func NewService(repository Repository, resolver PathResolver) *Service {
	return &Service{repository: repository, resolver: resolver, now: time.Now}
}

func (service *Service) HostStatus(ctx context.Context) HostStatus {
	status := HostStatus{Status: "disabled", Features: agenthost.HostFeatures{
		PermissionModes: []agenthost.PermissionMode{},
	}}
	if service == nil || service.resolver == nil {
		return status
	}
	status.Enabled = true
	status.Status = "unavailable"
	resolver, ok := service.resolver.(capabilityResolver)
	if !ok {
		return status
	}
	capabilities, err := resolver.Capabilities(ctx)
	if err != nil {
		return status
	}
	status.Status = "ready"
	status.RunnerID = capabilities.RunnerID
	status.Platform = capabilities.Platform
	status.Architecture = capabilities.Architecture
	status.Features = capabilities.Features
	if status.Features.PermissionModes == nil {
		status.Features.PermissionModes = []agenthost.PermissionMode{}
	}
	return status
}

func (service *Service) BrowseDirectories(
	ctx context.Context,
	path string,
) (agenthost.DirectoryBrowseResponse, error) {
	if service == nil || service.resolver == nil {
		return agenthost.DirectoryBrowseResponse{}, ErrDisabled
	}
	if path != "" && !validPath(path) {
		return agenthost.DirectoryBrowseResponse{}, ErrInvalid
	}
	browser, ok := service.resolver.(directoryBrowser)
	if !ok {
		return agenthost.DirectoryBrowseResponse{}, ErrDisabled
	}
	return browser.BrowseDirectories(ctx, path)
}

func (service *Service) PickNativeDirectory(
	ctx context.Context,
) (agenthost.NativeDirectoryPickResponse, error) {
	if service == nil || service.resolver == nil {
		return agenthost.NativeDirectoryPickResponse{}, ErrDisabled
	}
	picker, ok := service.resolver.(nativeDirectoryPicker)
	if !ok {
		return agenthost.NativeDirectoryPickResponse{}, ErrDisabled
	}
	return picker.PickNativeDirectory(ctx)
}

func (service *Service) List(ctx context.Context) ([]Workspace, error) {
	if service == nil || service.repository == nil {
		return nil, ErrDisabled
	}
	return service.repository.List(ctx)
}

func (service *Service) Get(ctx context.Context, workspaceID string) (Workspace, error) {
	if service == nil || service.repository == nil {
		return Workspace{}, ErrDisabled
	}
	if !validUUID(workspaceID) {
		return Workspace{}, ErrInvalid
	}
	return service.repository.Get(ctx, workspaceID)
}

func (service *Service) ImportLegacy(
	ctx context.Context,
	workspaceID string,
	settings Settings,
) (Workspace, error) {
	if service == nil || service.repository == nil {
		return Workspace{}, ErrDisabled
	}
	if !validUUID(workspaceID) || validateSettings(settings) != nil {
		return Workspace{}, ErrInvalid
	}
	return service.repository.ImportLegacy(ctx, workspaceID, normalizeSettings(settings), service.now().UTC())
}

func (service *Service) UpdateSettings(
	ctx context.Context,
	workspaceID string,
	expectedRevision int64,
	settings Settings,
) (Workspace, error) {
	if service == nil || service.repository == nil {
		return Workspace{}, ErrDisabled
	}
	if !validUUID(workspaceID) || expectedRevision < 1 || validateSettings(settings) != nil {
		return Workspace{}, ErrInvalid
	}
	return service.repository.UpdateSettings(
		ctx, workspaceID, expectedRevision, normalizeSettings(settings),
	)
}

func (service *Service) Bind(
	ctx context.Context,
	workspaceID string,
	expectedRevision int64,
	path string,
) (Workspace, error) {
	if service == nil || service.repository == nil {
		return Workspace{}, ErrDisabled
	}
	if !validUUID(workspaceID) || expectedRevision < 1 || !validPath(path) {
		return Workspace{}, ErrInvalid
	}
	existing, err := service.repository.Get(ctx, workspaceID)
	if err != nil {
		return Workspace{}, err
	}
	if existing.Revision != expectedRevision {
		return Workspace{}, ErrRevisionConflict
	}
	if existing.Bound() {
		return Workspace{}, ErrAlreadyBound
	}
	if service.resolver == nil {
		return Workspace{}, ErrDisabled
	}
	descriptor, err := service.resolver.ResolveWorkspace(ctx, path)
	if err != nil {
		return Workspace{}, err
	}
	if !runnerIDPattern.MatchString(service.resolver.RunnerID()) ||
		!strings.HasPrefix(descriptor.CanonicalPath, "/") ||
		!validPath(descriptor.CanonicalPath) || !validPath(descriptor.DisplayPath) ||
		(descriptor.PathKind != "wsl" && descriptor.PathKind != "windows-mounted") ||
		!strings.HasPrefix(descriptor.DirectoryFingerprint, "sha256:") ||
		!sha256Pattern.MatchString(strings.TrimPrefix(descriptor.DirectoryFingerprint, "sha256:")) {
		return Workspace{}, ErrInvalid
	}
	return service.repository.Bind(
		ctx, workspaceID, expectedRevision, descriptor,
		service.resolver.RunnerID(), service.now().UTC(),
	)
}

func (service *Service) Delete(
	ctx context.Context,
	workspaceID string,
	expectedRevision int64,
) error {
	if service == nil || service.repository == nil {
		return ErrDisabled
	}
	if !validUUID(workspaceID) || expectedRevision < 1 {
		return ErrInvalid
	}
	return service.repository.Delete(ctx, workspaceID, expectedRevision, service.now().UTC())
}

func (service *Service) SetConversationWorkspace(
	ctx context.Context,
	conversationID string,
	workspaceID string,
) error {
	if service == nil || service.repository == nil {
		return ErrDisabled
	}
	if !validUUID(conversationID) || !validUUID(workspaceID) {
		return ErrInvalid
	}
	return service.repository.SetConversationWorkspace(ctx, conversationID, workspaceID)
}

func (service *Service) ClearConversationWorkspace(
	ctx context.Context,
	conversationID string,
	workspaceID string,
) error {
	if service == nil || service.repository == nil {
		return ErrDisabled
	}
	if !validUUID(conversationID) || !validUUID(workspaceID) {
		return ErrInvalid
	}
	return service.repository.ClearConversationWorkspace(ctx, conversationID, workspaceID)
}

func (service *Service) LockConversationExecutionWorkspace(
	ctx context.Context,
	conversationID string,
	workspaceID string,
) (ExecutionBinding, error) {
	if service == nil || service.repository == nil {
		return ExecutionBinding{}, ErrDisabled
	}
	if !validUUID(conversationID) || !validUUID(workspaceID) {
		return ExecutionBinding{}, ErrInvalid
	}
	return service.repository.LockConversationExecutionWorkspace(
		ctx, conversationID, workspaceID, service.now().UTC(),
	)
}

func validateSettings(settings Settings) error {
	name := strings.TrimSpace(settings.Name)
	if name == "" || len(name) > maxNameBytes || !utf8.ValidString(name) ||
		len(settings.SystemPrompt) > maxSystemPromptBytes || !utf8.ValidString(settings.SystemPrompt) ||
		len(settings.Files) > maxFiles || len(settings.Color) > maxColorBytes ||
		!utf8.ValidString(settings.Color) || strings.ContainsAny(settings.Color, "\x00\r\n") {
		return ErrInvalid
	}
	encodedFiles, err := json.Marshal(settings.Files)
	if err != nil || len(encodedFiles) > maxFilesBytes {
		return ErrInvalid
	}
	for _, file := range settings.Files {
		if strings.TrimSpace(file.ID) == "" || strings.TrimSpace(file.FileName) == "" ||
			len(file.ID) > 256 || len(file.FileName) > 1024 || len(file.MimeType) > 255 ||
			file.Size < 0 || (file.SHA256 != "" && !sha256Pattern.MatchString(file.SHA256)) {
			return ErrInvalid
		}
	}
	return nil
}

func normalizeSettings(settings Settings) Settings {
	settings.Name = strings.TrimSpace(settings.Name)
	settings.Color = strings.TrimSpace(settings.Color)
	if settings.Files == nil {
		settings.Files = []WorkspaceFile{}
	}
	return settings
}

func validUUID(value string) bool {
	parsed, err := uuid.Parse(value)
	return err == nil && parsed.String() == strings.ToLower(value)
}

func validPath(value string) bool {
	if value == "" || len(value) > 4096 || !utf8.ValidString(value) {
		return false
	}
	for _, char := range value {
		if char < 0x20 || char == 0x7f {
			return false
		}
	}
	return true
}
