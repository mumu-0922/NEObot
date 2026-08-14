package agentdelegation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"neo-chat/mm-chat/backend/internal/agentbroker"
	"neo-chat/mm-chat/backend/internal/agentorchestrator"
)

type Service struct {
	repository Repository
	reaper     Reaper
	now        func() time.Time
	newID      func(string) string
}

func NewService(repository Repository, reaper Reaper) *Service {
	return &Service{repository: repository, reaper: reaper, now: time.Now,
		newID: func(prefix string) string { return prefix + "_" + strings.ReplaceAll(uuid.NewString(), "-", "") }}
}

func (service *Service) RegisterRoot(ctx context.Context, input RegisterRootInput) (Authority, bool, error) {
	if service == nil || service.repository == nil {
		return Authority{}, false, ErrDatabaseRequired
	}
	if input.RunID == "" || input.UserID == "" || input.Grant.Subject.UserID != input.UserID ||
		input.Grant.Run.Depth != 0 || input.Grant.Run.RunID != input.RunID ||
		input.Grant.Run.ParentRunID != "" || input.Registry.Depth != 0 || input.Registry.RunID != input.RunID ||
		input.Registry.Subject != input.Grant.Subject || input.Registry.GrantID != input.Grant.GrantID ||
		input.Registry.PackageFingerprint != input.Grant.PackageFingerprint ||
		input.Registry.RuntimeBundleFingerprint != input.Grant.RuntimeBundleFingerprint ||
		input.Model.Provider == "" || input.Model.ModelID == "" {
		return Authority{}, false, ErrInvalidInput
	}
	now := service.now().UTC()
	if err := agentbroker.ValidateGrant(input.Grant, now); err != nil {
		return Authority{}, false, ErrInvalidInput
	}
	grantFingerprint, err := agentbroker.GrantFingerprint(input.Grant)
	if err != nil || input.Registry.Fingerprint == "" {
		return Authority{}, false, ErrInvalidInput
	}
	if _, err := agentbroker.RegistryToolFor(input.Registry, "__authority_probe__", "probe", "probe"); err != nil && err != agentbroker.ErrGrantDenied {
		return Authority{}, false, ErrInvalidInput
	}
	if !registryMatchesGrant(input.Registry, input.Grant) {
		return Authority{}, false, ErrInvalidInput
	}
	authority := Authority{RunID: input.RunID, RootRunID: input.RunID, Depth: 0,
		UserID: input.UserID, Subject: input.Grant.Subject, Model: input.Model,
		PackageFingerprint:       input.Grant.PackageFingerprint,
		RuntimeBundleFingerprint: input.Grant.RuntimeBundleFingerprint,
		Grant:                    input.Grant, GrantFingerprint: grantFingerprint, Registry: input.Registry,
		RegistryFingerprint: input.Registry.Fingerprint, ExpiresAt: input.Grant.ExpiresAt.UTC(),
		Budget: input.Grant.Budget, State: "active", CreatedAt: now}
	return service.repository.RegisterRoot(ctx, authority)
}

func (service *Service) EnqueueChild(ctx context.Context, proposal ChildProposal) (EnqueueResult, error) {
	if service == nil || service.repository == nil {
		return EnqueueResult{}, ErrDatabaseRequired
	}
	if proposal.UserID == "" || proposal.IdempotencyKey == "" || proposal.ParentRunID == "" || len(proposal.Steps) == 0 ||
		proposal.ParentAttempt.AttemptID == "" || proposal.ParentAttempt.LeaseToken == "" {
		return EnqueueResult{}, ErrInvalidInput
	}
	parent, err := service.repository.GetAuthority(ctx, proposal.UserID, proposal.ParentRunID)
	if err != nil {
		return EnqueueResult{}, err
	}
	derivation, err := service.deriveChild(parent, proposal)
	if err != nil {
		return EnqueueResult{}, err
	}
	authority, created, err := service.repository.EnqueueChild(ctx, derivation, proposal)
	return EnqueueResult{Authority: authority, Created: created}, err
}

func (service *Service) deriveChild(parent Authority, proposal ChildProposal) (Derivation, error) {
	now := service.now().UTC()
	if parent.Depth != 0 || parent.ParentRunID != "" {
		return Derivation{}, ErrDepthExceeded
	}
	if parent.UserID != proposal.UserID || parent.RunID != proposal.ParentRunID ||
		parent.State != "active" || !now.Before(parent.ExpiresAt) {
		return Derivation{}, ErrParentInvalid
	}
	childRunID := service.newID("run")
	childGrant := proposal.Grant
	childGrant.GrantID = service.newID("grant")
	childGrant.Run = agentbroker.RunBinding{RunID: childRunID, ParentRunID: parent.RunID, Depth: 1}
	childGrant.IssuedAt = parent.Grant.IssuedAt
	if childGrant.Subject != parent.Subject || childGrant.PackageFingerprint != parent.PackageFingerprint ||
		childGrant.RuntimeBundleFingerprint != parent.RuntimeBundleFingerprint || proposal.Model != parent.Model ||
		childGrant.IssuedAt.Before(parent.Grant.IssuedAt) || childGrant.ExpiresAt.After(parent.ExpiresAt) ||
		!budgetContained(childGrant.Budget, parent.Budget) ||
		!capabilitiesContained(childGrant.Capabilities, parent.Grant.Capabilities) ||
		!egressContained(childGrant.Egress, parent.Grant.Egress) || !secretsContained(childGrant.Secrets, parent.Grant.Secrets) {
		return Derivation{}, ErrSubsetViolation
	}
	if err := agentbroker.ValidateGrant(childGrant, now); err != nil {
		return Derivation{}, ErrSubsetViolation
	}
	grantFingerprint, err := agentbroker.GrantFingerprint(childGrant)
	if err != nil {
		return Derivation{}, ErrInvalidInput
	}
	forbidden := agentbroker.ChildForbiddenIdentities()
	catalogByIdentity := make(map[string]agentbroker.ToolDefinition, len(proposal.Catalog))
	for _, definition := range proposal.Catalog {
		catalogByIdentity[definition.Identity] = definition
	}
	for _, identity := range proposal.RequestedTools {
		definition, ok := catalogByIdentity[identity]
		if !ok {
			continue
		}
		if _, identityForbidden := forbidden[identity]; !identityForbidden {
			if _, capabilityForbidden := forbidden[definition.Capability]; capabilityForbidden {
				return Derivation{}, ErrSubsetViolation
			}
		}
	}
	registry, err := agentbroker.BuildRegistry(proposal.Catalog, proposal.RequestedTools, childGrant, now)
	if err != nil {
		return Derivation{}, ErrSubsetViolation
	}
	if !registryContained(registry, parent.Registry) {
		return Derivation{}, ErrSubsetViolation
	}
	snapshot := struct {
		SchemaVersion            string             `json:"schemaVersion"`
		RootRunID                string             `json:"rootRunId"`
		ParentRunID              string             `json:"parentRunId"`
		Depth                    int                `json:"depth"`
		Model                    ModelBinding       `json:"model"`
		PackageFingerprint       string             `json:"packageFingerprint"`
		RuntimeBundleFingerprint string             `json:"runtimeBundleFingerprint"`
		GrantFingerprint         string             `json:"grantFingerprint"`
		RegistryFingerprint      string             `json:"registryFingerprint"`
		Budget                   agentbroker.Budget `json:"budget"`
	}{"neo.agent-snapshot/v1", parent.RootRunID, parent.RunID, 1, parent.Model,
		parent.PackageFingerprint, parent.RuntimeBundleFingerprint, grantFingerprint, registry.Fingerprint, childGrant.Budget}
	canonicalSnapshot, _ := json.Marshal(snapshot)
	snapshotFingerprint := fingerprint("neo-agent-snapshot-v1", canonicalSnapshot)
	requestBody, _ := json.Marshal(struct {
		Parent  string
		Attempt ParentAttempt
		Model   ModelBinding
		Grant   agentbroker.CapabilityGrant
		Tools   []string
		Steps   []string
	}{
		parent.RunID, proposal.ParentAttempt, proposal.Model, proposal.Grant,
		append([]string(nil), proposal.RequestedTools...), stepKinds(proposal.Steps)})
	steps := append([]agentorchestrator.StepPlan(nil), proposal.Steps...)
	for index := range steps {
		if steps[index].ID == "" {
			steps[index].ID = service.newID("step")
		}
	}
	events := make([]string, 3+2*len(steps))
	for index := range events {
		events[index] = service.newID("event")
	}
	scopeKeys := []string{"global:*", "scheduler:*", "user:" + parent.UserID, "run:" + childRunID,
		"project:" + parent.Subject.ProjectID, "skill:" + parent.PackageFingerprint}
	sort.Strings(scopeKeys)
	return Derivation{Parent: parent, ChildRunID: childRunID, ChildSnapshotID: service.newID("snapshot"),
		ChildSnapshot: canonicalSnapshot, ChildSnapshotFingerprint: snapshotFingerprint,
		ChildGrant: childGrant, ChildGrantFingerprint: grantFingerprint, ChildRegistry: registry,
		Reservation: childGrant.Budget, Steps: steps, ScopeKeys: scopeKeys, EventIDs: events,
		RequestFingerprint: fingerprint("neo-agent-child-enqueue-v1", requestBody)}, nil
}

func (service *Service) AdmitLaunch(ctx context.Context, input LaunchAdmissionInput) error {
	if service == nil || service.repository == nil {
		return ErrDatabaseRequired
	}
	digest := sha256.Sum256([]byte(input.LeaseToken))
	return service.repository.AdmitLaunch(ctx, input, hex.EncodeToString(digest[:]))
}

func (service *Service) Settle(ctx context.Context, input SettleInput) (bool, error) {
	if service == nil || service.repository == nil {
		return false, ErrDatabaseRequired
	}
	if input.UserID == "" || input.ChildRunID == "" ||
		input.Usage.MaxWallSeconds < 0 || input.Usage.MaxModelTokens < 0 ||
		input.Usage.MaxToolCalls < 0 || input.Usage.MaxArtifactBytes < 0 ||
		!contains([]string{"succeeded", "failed", "canceled", "killed", "outcome_unknown"}, input.Outcome) {
		return false, ErrInvalidInput
	}
	return service.repository.Settle(ctx, input, service.newID("settlement"))
}

func (service *Service) Cascade(ctx context.Context, input CascadeInput) ([]ReapTarget, error) {
	if service == nil || service.repository == nil || service.reaper == nil {
		return nil, ErrDatabaseRequired
	}
	if input.UserID == "" || input.ParentRunID == "" ||
		!contains([]string{"cancel", "kill"}, input.Mode) ||
		!contains([]string{"user", "orchestrator", "runner", "scheduler", "operator"}, input.ActorType) ||
		strings.TrimSpace(input.ActorID) != input.ActorID || input.ActorID == "" || len(input.ActorID) > 128 ||
		!validReasonCode(input.ReasonCode) {
		return nil, ErrInvalidInput
	}
	targets, err := service.repository.Cascade(ctx, input)
	if err != nil {
		return nil, err
	}
	for _, target := range targets {
		reapErr := service.reaper.Reap(ctx, target)
		code := ""
		if reapErr != nil {
			code = "RUNTIME_UNAVAILABLE"
		}
		if completeErr := service.repository.CompleteReap(ctx, target.ReapID, reapErr == nil, code); completeErr != nil {
			return targets, completeErr
		}
	}
	return targets, nil
}

func (service *Service) Reconcile(ctx context.Context, limit int) (int, error) {
	if service == nil || service.repository == nil || service.reaper == nil {
		return 0, ErrDatabaseRequired
	}
	if limit < 1 || limit > 1000 {
		return 0, ErrInvalidInput
	}
	if _, err := service.repository.Recover(ctx, limit); err != nil {
		return 0, err
	}
	targets, err := service.repository.ListPendingReaps(ctx, limit)
	if err != nil {
		return 0, err
	}
	completed := 0
	for _, target := range targets {
		reapErr := service.reaper.Reap(ctx, target)
		code := ""
		if reapErr != nil {
			code = "RUNTIME_UNAVAILABLE"
		}
		if err := service.repository.CompleteReap(ctx, target.ReapID, reapErr == nil, code); err != nil {
			return completed, err
		}
		if reapErr == nil {
			completed++
		}
	}
	return completed, nil
}

func budgetContained(child, parent agentbroker.Budget) bool {
	return child.MaxWallSeconds <= parent.MaxWallSeconds && child.MaxModelTokens <= parent.MaxModelTokens &&
		child.MaxToolCalls <= parent.MaxToolCalls && child.MaxArtifactBytes <= parent.MaxArtifactBytes
}

func capabilitiesContained(child, parent []agentbroker.Capability) bool {
	parents := map[string]agentbroker.Capability{}
	for _, value := range parent {
		parents[value.Capability] = value
	}
	for _, value := range child {
		if _, denied := agentbroker.ChildForbiddenIdentities()[value.Capability]; denied {
			return false
		}
		base, ok := parents[value.Capability]
		if !ok || value.MaxCalls > base.MaxCalls || approvalRank(value.Approval) > approvalRank(base.Approval) ||
			!stringsContained(value.Actions, base.Actions) || !selectorContained(value.Resources, base.Resources) {
			return false
		}
	}
	return true
}

func selectorContained(child, parent agentbroker.Selector) bool {
	for _, candidate := range child.Values {
		allowed := false
		for _, base := range parent.Values {
			if parent.Kind == "prefix" && strings.HasPrefix(candidate, base) || parent.Kind == "exact" && child.Kind == "exact" && candidate == base {
				allowed = true
				break
			}
		}
		if !allowed {
			return false
		}
	}
	return child.Kind == "exact" || parent.Kind == "prefix"
}

func egressContained(child, parent agentbroker.EgressPolicy) bool {
	if egressRank(child.Mode) > egressRank(parent.Mode) {
		return false
	}
	for _, rule := range child.Rules {
		matched := false
		for _, base := range parent.Rules {
			if rule.Scheme == base.Scheme && rule.Host == base.Host && intsContained(rule.Ports, base.Ports) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

func secretsContained(child, parent []agentbroker.SecretGrant) bool {
	parents := map[string]agentbroker.SecretGrant{}
	for _, value := range parent {
		parents[value.Slot] = value
	}
	for _, value := range child {
		base, ok := parents[value.Slot]
		if !ok || value.BrokerRef != base.BrokerRef || value.TTLSeconds > base.TTLSeconds || !stringsContained(value.Actions, base.Actions) {
			return false
		}
	}
	return true
}

func registryMatchesGrant(registry agentbroker.ToolRegistry, grant agentbroker.CapabilityGrant) bool {
	if registry.SchemaVersion != agentbroker.RegistryVersion || registry.Subject != grant.Subject ||
		registry.RunID != grant.Run.RunID || registry.Depth != grant.Run.Depth || registry.GrantID != grant.GrantID ||
		registry.PackageFingerprint != grant.PackageFingerprint ||
		registry.RuntimeBundleFingerprint != grant.RuntimeBundleFingerprint ||
		!registry.ExpiresAt.Equal(grant.ExpiresAt) || registry.Budget != grant.Budget ||
		!reflect.DeepEqual(registry.Egress, grant.Egress) || !reflect.DeepEqual(registry.Secrets, grant.Secrets) {
		return false
	}
	capabilities := make(map[string]agentbroker.Capability, len(grant.Capabilities))
	for _, capability := range grant.Capabilities {
		capabilities[capability.Capability] = capability
	}
	for _, tool := range registry.Tools {
		capability, ok := capabilities[tool.Capability]
		if !ok || tool.MaxCalls < 1 || tool.MaxCalls > capability.MaxCalls ||
			approvalRank(tool.Approval) > approvalRank(capability.Approval) ||
			!stringsContained(tool.Actions, capability.Actions) || !selectorContained(tool.Resources, capability.Resources) ||
			!contains([]string{agentbroker.ClassificationRead, agentbroker.ClassificationMutable, agentbroker.ClassificationUnknown}, tool.Classification) ||
			(tool.Idempotent && tool.Classification != agentbroker.ClassificationRead) {
			return false
		}
	}
	return true
}

func registryContained(child, parent agentbroker.ToolRegistry) bool {
	parentTools := make(map[string]agentbroker.RegistryTool, len(parent.Tools))
	for _, tool := range parent.Tools {
		parentTools[tool.Identity] = tool
	}
	forbidden := agentbroker.ChildForbiddenIdentities()
	for _, tool := range child.Tools {
		base, ok := parentTools[tool.Identity]
		_, identityForbidden := forbidden[tool.Identity]
		_, capabilityForbidden := forbidden[tool.Capability]
		if !ok || identityForbidden || capabilityForbidden || tool.Capability != base.Capability ||
			tool.MaxCalls > base.MaxCalls || approvalRank(tool.Approval) > approvalRank(base.Approval) ||
			!stringsContained(tool.Actions, base.Actions) || !selectorContained(tool.Resources, base.Resources) ||
			tool.Classification != base.Classification || tool.Idempotent != base.Idempotent {
			return false
		}
	}
	return true
}

func stringsContained(child, parent []string) bool {
	set := map[string]struct{}{}
	for _, value := range parent {
		set[value] = struct{}{}
	}
	for _, value := range child {
		if _, ok := set[value]; !ok {
			return false
		}
	}
	return true
}
func intsContained(child, parent []int) bool {
	set := map[int]struct{}{}
	for _, value := range parent {
		set[value] = struct{}{}
	}
	for _, value := range child {
		if _, ok := set[value]; !ok {
			return false
		}
	}
	return true
}
func approvalRank(value string) int {
	switch value {
	case agentbroker.ApprovalDenied:
		return 0
	case agentbroker.ApprovalPerCommit:
		return 1
	case agentbroker.ApprovalOnce:
		return 2
	case agentbroker.ApprovalAutomatic:
		return 3
	}
	return 99
}
func egressRank(value string) int {
	switch value {
	case "none":
		return 0
	case "allowlist":
		return 1
	case "brokered":
		return 2
	}
	return 99
}
func contains(values []string, candidate string) bool {
	for _, value := range values {
		if value == candidate {
			return true
		}
	}
	return false
}

func validReasonCode(value string) bool {
	if value == "" || len(value) > 64 || value[0] < 'A' || value[0] > 'Z' {
		return false
	}
	for _, char := range value[1:] {
		if char != '_' && (char < 'A' || char > 'Z') && (char < '0' || char > '9') {
			return false
		}
	}
	return true
}
func fingerprint(domain string, value []byte) string {
	digest := sha256.Sum256(append(append([]byte(nil), []byte(domain)...), append([]byte{0}, value...)...))
	return "sha256:" + hex.EncodeToString(digest[:])
}
func stepKinds(steps []agentorchestrator.StepPlan) []string {
	result := make([]string, len(steps))
	for index := range steps {
		result[index] = steps[index].Kind
	}
	return result
}
