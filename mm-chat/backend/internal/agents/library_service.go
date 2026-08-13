package agents

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

var (
	ErrRepositoryUnavailable = errors.New("assistant repository unavailable")
	ErrLibraryNotFound       = errors.New("assistant library entry not found")
	ErrLibraryConflict       = errors.New("assistant is already installed")
	ErrRevisionConflict      = errors.New("assistant revision changed")
	ErrAdmissionNotFound     = errors.New("assistant is not admitted")
	ErrAdministratorRequired = errors.New("assistant administrator access is required")
	ErrMarketChanged         = errors.New("assistant market detail changed")
	ErrMarketUnavailable     = errors.New("assistant market unavailable")
)

var uuidRE = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func (service *Service) IsAdministrator(userID string) bool {
	if service == nil {
		return false
	}
	userID = strings.TrimSpace(userID)
	administratorUserID := strings.TrimSpace(service.administratorUserID)
	return userID != "" && administratorUserID != "" &&
		strings.EqualFold(userID, administratorUserID)
}

func (service *Service) ListLibrary(ctx context.Context, userID string) ([]LibraryEntry, error) {
	if service == nil || service.repository == nil {
		return nil, ErrRepositoryUnavailable
	}
	entries, err := service.repository.ListLibrary(ctx, strings.TrimSpace(userID))
	if err != nil {
		return nil, err
	}
	for index := range entries {
		if entries[index].Source != SourceLobeHub {
			continue
		}
		admission, admissionErr := service.repository.GetAdmission(ctx, entries[index].SourceIdentifier)
		entries[index].UpdateAvailable = admissionErr == nil &&
			admission.Status == AdmissionAdmitted &&
			admission.ContentFingerprint != entries[index].ContentFingerprint
	}
	return entries, nil
}

func (service *Service) GetLibrary(ctx context.Context, userID, entryID string) (LibraryEntry, error) {
	if service == nil || service.repository == nil {
		return LibraryEntry{}, ErrRepositoryUnavailable
	}
	if !validUUID(entryID) {
		return LibraryEntry{}, validationError("INVALID_ASSISTANT_ID", "assistant id is invalid")
	}
	return service.repository.GetLibrary(ctx, strings.TrimSpace(userID), strings.TrimSpace(entryID))
}

func (service *Service) CreateCustom(ctx context.Context, userID string, input CreateCustomInput) (LibraryEntry, error) {
	if service == nil || service.repository == nil {
		return LibraryEntry{}, ErrRepositoryUnavailable
	}
	snapshot, err := normalizeCustomSnapshot(input)
	if err != nil {
		return LibraryEntry{}, err
	}
	return service.repository.CreateCustom(ctx, strings.TrimSpace(userID), snapshot)
}

func (service *Service) UpdateCustom(
	ctx context.Context,
	userID, entryID string,
	input UpdateCustomInput,
) (LibraryEntry, error) {
	if service == nil || service.repository == nil {
		return LibraryEntry{}, ErrRepositoryUnavailable
	}
	if !validUUID(entryID) || input.ExpectedRevision < 1 {
		return LibraryEntry{}, validationError("INVALID_ASSISTANT_UPDATE", "assistant update is invalid")
	}
	snapshot, err := normalizeCustomSnapshot(input.CreateCustomInput)
	if err != nil {
		return LibraryEntry{}, err
	}
	current, err := service.repository.GetLibrary(ctx, strings.TrimSpace(userID), entryID)
	if err != nil {
		return LibraryEntry{}, err
	}
	if current.Source != SourceCustom || current.Revision != input.ExpectedRevision {
		return LibraryEntry{}, ErrRevisionConflict
	}
	snapshot.RequiredTools = normalizeStringTags(current.RequiredTools)
	snapshot.ContentFingerprint = fingerprintSnapshot(snapshot)
	return service.repository.UpdateCustom(ctx, strings.TrimSpace(userID), entryID, input.ExpectedRevision, snapshot)
}

func (service *Service) DeleteCustom(ctx context.Context, userID, entryID string, expectedRevision int64) error {
	if service == nil || service.repository == nil {
		return ErrRepositoryUnavailable
	}
	if !validUUID(entryID) || expectedRevision < 1 {
		return validationError("INVALID_ASSISTANT_DELETE", "assistant delete is invalid")
	}
	return service.repository.DeleteCustom(ctx, strings.TrimSpace(userID), entryID, expectedRevision)
}

func (service *Service) Install(
	ctx context.Context,
	userID, identifier, expectedFingerprint string,
) (LibraryEntry, error) {
	if service == nil || service.repository == nil {
		return LibraryEntry{}, ErrRepositoryUnavailable
	}
	identifier = strings.TrimSpace(identifier)
	if !validIdentifier(identifier) || !validFingerprint(expectedFingerprint) {
		return LibraryEntry{}, validationError("INVALID_ASSISTANT_INSTALL", "assistant install is invalid")
	}
	admission, err := service.repository.GetAdmission(ctx, identifier)
	if err != nil || admission.Status != AdmissionAdmitted {
		return LibraryEntry{}, ErrAdmissionNotFound
	}
	if admission.ContentFingerprint != expectedFingerprint {
		return LibraryEntry{}, ErrMarketChanged
	}
	return service.repository.Install(ctx, strings.TrimSpace(userID), admission.Snapshot)
}

func (service *Service) UpdateInstalled(
	ctx context.Context,
	userID, entryID string,
	expectedRevision int64,
) (LibraryEntry, error) {
	if service == nil || service.repository == nil {
		return LibraryEntry{}, ErrRepositoryUnavailable
	}
	if !validUUID(entryID) || expectedRevision < 1 {
		return LibraryEntry{}, validationError("INVALID_ASSISTANT_UPDATE", "assistant update is invalid")
	}
	current, err := service.repository.GetLibrary(ctx, userID, entryID)
	if err != nil {
		return LibraryEntry{}, err
	}
	if current.Source != SourceLobeHub || current.Revision != expectedRevision {
		return LibraryEntry{}, ErrRevisionConflict
	}
	admission, err := service.repository.GetAdmission(ctx, current.SourceIdentifier)
	if err != nil || admission.Status != AdmissionAdmitted {
		return LibraryEntry{}, ErrAdmissionNotFound
	}
	return service.repository.UpdateInstalled(ctx, userID, entryID, expectedRevision, admission.Snapshot)
}

func (service *Service) Uninstall(ctx context.Context, userID, entryID string, expectedRevision int64) error {
	if service == nil || service.repository == nil {
		return ErrRepositoryUnavailable
	}
	if !validUUID(entryID) || expectedRevision < 1 {
		return validationError("INVALID_ASSISTANT_UNINSTALL", "assistant uninstall is invalid")
	}
	return service.repository.Uninstall(ctx, strings.TrimSpace(userID), entryID, expectedRevision)
}

func (service *Service) CopyToCustom(
	ctx context.Context,
	userID, entryID string,
	expectedRevision int64,
) (LibraryEntry, error) {
	if expectedRevision < 1 {
		return LibraryEntry{}, validationError("INVALID_ASSISTANT_COPY", "assistant copy is invalid")
	}
	current, err := service.GetLibrary(ctx, userID, entryID)
	if err != nil {
		return LibraryEntry{}, err
	}
	if current.Revision != expectedRevision {
		return LibraryEntry{}, ErrRevisionConflict
	}
	snapshot, err := normalizeCustomSnapshot(CreateCustomInput{
		Avatar: current.Avatar, Title: current.Title, Description: current.Description,
		Category: current.Category, Tags: current.Tags, SystemPrompt: current.SystemPrompt,
	})
	if err != nil {
		return LibraryEntry{}, err
	}
	snapshot.RequiredTools = normalizeStringTags(current.RequiredTools)
	snapshot.ContentFingerprint = fingerprintSnapshot(snapshot)
	return service.repository.CreateCustom(ctx, userID, snapshot)
}

func (service *Service) ReviewMarket(
	ctx context.Context,
	userID, identifier, status, expectedFingerprint string,
	locale Locale,
) error {
	if service == nil || service.repository == nil {
		return ErrRepositoryUnavailable
	}
	if !service.IsAdministrator(userID) {
		return ErrAdministratorRequired
	}
	if status != AdmissionAdmitted && status != AdmissionRejected {
		return validationError("INVALID_ASSISTANT_REVIEW", "assistant review is invalid")
	}
	agent, err := service.getMarketAgentDetail(ctx, identifier, locale)
	if err != nil {
		return err
	}
	snapshot, err := snapshotFromAgent(agent)
	if err != nil {
		return err
	}
	if !validFingerprint(expectedFingerprint) || snapshot.ContentFingerprint != expectedFingerprint {
		return ErrMarketChanged
	}
	return service.repository.UpsertAdmission(ctx, userID, Admission{Snapshot: snapshot, Status: status})
}

func normalizeCustomSnapshot(input CreateCustomInput) (Snapshot, error) {
	snapshot := Snapshot{
		Avatar:        trimText(input.Avatar, maxAgentAvatarChars),
		Title:         trimText(input.Title, maxAgentTitleChars),
		Description:   trimText(input.Description, maxAgentDescriptionChars),
		Category:      trimText(input.Category, maxAgentCategoryChars),
		Tags:          normalizeStringTags(input.Tags),
		SystemPrompt:  trimText(input.SystemPrompt, maxAgentSystemRoleChars),
		RequiredTools: []string{},
	}
	if snapshot.Avatar == "" {
		snapshot.Avatar = "🤖"
	}
	if snapshot.Category == "" {
		snapshot.Category = "general"
	}
	if snapshot.Title == "" || snapshot.SystemPrompt == "" {
		return Snapshot{}, validationError("INVALID_ASSISTANT", "assistant name and system prompt are required")
	}
	snapshot.ContentFingerprint = fingerprintSnapshot(snapshot)
	return snapshot, nil
}

func snapshotFromAgent(agent Agent) (Snapshot, error) {
	systemPrompt := agent.Meta.SystemRole
	if agent.Config != nil && strings.TrimSpace(agent.Config.SystemRole) != "" {
		systemPrompt = agent.Config.SystemRole
	}
	snapshot := Snapshot{
		SourceIdentifier: trimText(agent.Identifier, maxAgentIdentifierChars),
		Avatar:           trimText(agent.Meta.Avatar, maxAgentAvatarChars),
		Title:            trimText(agent.Meta.Title, maxAgentTitleChars),
		Description:      trimText(agent.Meta.Description, maxAgentDescriptionChars),
		Category:         trimText(agent.Meta.Category, maxAgentCategoryChars),
		Tags:             normalizeStringTags(agent.Meta.Tags),
		SystemPrompt:     trimText(systemPrompt, maxAgentSystemRoleChars),
		Author:           trimText(agent.Author, maxAgentAuthorChars),
		Homepage:         trimText(agent.Homepage, maxAgentHomepageChars),
		SourceVersion:    trimText(agent.Version, 80),
		SourceUpdatedAt:  trimText(agent.UpdatedAt, 80),
		RequiredTools:    normalizeStringTags(agent.RequiredTools),
	}
	if !validIdentifier(snapshot.SourceIdentifier) || snapshot.Title == "" || snapshot.SystemPrompt == "" {
		return Snapshot{}, ErrInvalidRegistryEntry
	}
	if snapshot.Avatar == "" {
		snapshot.Avatar = "🤖"
	}
	if snapshot.Category == "" {
		snapshot.Category = "general"
	}
	snapshot.ContentFingerprint = fingerprintSnapshot(snapshot)
	return snapshot, nil
}

func fingerprintSnapshot(snapshot Snapshot) string {
	canonical := struct {
		SourceIdentifier string   `json:"sourceIdentifier"`
		Avatar           string   `json:"avatar"`
		Title            string   `json:"title"`
		Description      string   `json:"description"`
		Category         string   `json:"category"`
		Tags             []string `json:"tags"`
		SystemPrompt     string   `json:"systemPrompt"`
		Author           string   `json:"author"`
		Homepage         string   `json:"homepage"`
		SourceVersion    string   `json:"sourceVersion"`
		SourceUpdatedAt  string   `json:"sourceUpdatedAt"`
		RequiredTools    []string `json:"requiredTools"`
	}{snapshot.SourceIdentifier, snapshot.Avatar, snapshot.Title, snapshot.Description,
		snapshot.Category, snapshot.Tags, snapshot.SystemPrompt, snapshot.Author,
		snapshot.Homepage, snapshot.SourceVersion, snapshot.SourceUpdatedAt,
		snapshot.RequiredTools}
	encoded, _ := json.Marshal(canonical)
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func agentFromSnapshot(snapshot Snapshot, admitted bool) Agent {
	return Agent{Identifier: snapshot.SourceIdentifier, Meta: AgentMeta{
		Avatar: snapshot.Avatar, Description: snapshot.Description, Tags: snapshot.Tags,
		Title: snapshot.Title, Category: snapshot.Category, SystemRole: snapshot.SystemPrompt,
	}, UpdatedAt: snapshot.SourceUpdatedAt, Homepage: snapshot.Homepage, Author: snapshot.Author,
		Config: &AgentConfig{SystemRole: snapshot.SystemPrompt}, Version: snapshot.SourceVersion,
		Fingerprint: snapshot.ContentFingerprint, RequiredTools: snapshot.RequiredTools,
		Admitted: admitted}
}

func normalizeStringTags(values []string) []string {
	tags := make([]string, 0, min(len(values), maxAgentTags))
	seen := map[string]struct{}{}
	for _, value := range values {
		tag := trimText(value, maxAgentTagChars)
		key := strings.ToLower(tag)
		if tag == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		tags = append(tags, tag)
		if len(tags) == maxAgentTags {
			break
		}
	}
	return tags
}

func trimText(value string, maxRunes int) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) > maxRunes {
		value = string(runes[:maxRunes])
	}
	return value
}

func validFingerprint(value string) bool {
	return len(value) == 64 && strings.Trim(value, "0123456789abcdef") == ""
}

func validUUID(value string) bool {
	return uuidRE.MatchString(strings.ToLower(strings.TrimSpace(value)))
}

func validSearchText(value string, maxRunes int) bool {
	if !utf8.ValidString(value) || len([]rune(value)) > maxRunes {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func pageCount(total, pageSize int) int {
	if total <= 0 || pageSize <= 0 {
		return 0
	}
	return (total + pageSize - 1) / pageSize
}
