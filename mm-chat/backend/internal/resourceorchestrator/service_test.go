package resourceorchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/mcpclient"
	"neo-chat/mm-chat/backend/internal/skillsupply"
)

type fakeMutationAuditor struct {
	audits []MutationAudit
	err    error
}

func (auditor *fakeMutationAuditor) RecordMutation(_ context.Context, audit MutationAudit) error {
	auditor.audits = append(auditor.audits, audit)
	return auditor.err
}

type fakeSkills struct {
	library []skillsupply.Installation
	store   skillsupply.StoreResult
}

type countingSkills struct {
	fakeSkills
	installCalls int
}

type mutationSkills struct {
	fakeSkills
	owner          string
	uninstallCalls []MutationRequest
}

func (skills *mutationSkills) ListLibrary(
	_ context.Context,
	userID string,
) ([]skillsupply.Installation, error) {
	if userID != skills.owner {
		return []skillsupply.Installation{}, nil
	}
	return skills.library, nil
}

func (skills *mutationSkills) Uninstall(
	_ context.Context,
	userID string,
	installationID string,
	revision int64,
) error {
	skills.uninstallCalls = append(skills.uninstallCalls, MutationRequest{
		UserID: userID, ID: installationID, ExpectedRevision: revision,
	})
	return nil
}

func (skills *countingSkills) Install(
	ctx context.Context,
	userID string,
	id string,
	fingerprint string,
) (skillsupply.Installation, error) {
	skills.installCalls++
	return skills.fakeSkills.Install(ctx, userID, id, fingerprint)
}

func (fake fakeSkills) ListLibrary(context.Context, string) ([]skillsupply.Installation, error) {
	return fake.library, nil
}

func (fake fakeSkills) ListStore(context.Context, int, int) (skillsupply.StoreResult, error) {
	return fake.store, nil
}

func (fake fakeSkills) GetStoreItem(_ context.Context, id string) (skillsupply.Candidate, error) {
	for _, candidate := range fake.store.Items {
		if candidate.ID == id {
			return candidate, nil
		}
	}
	return skillsupply.Candidate{}, skillsupply.ErrCandidateNotFound
}

func (fake fakeSkills) Install(_ context.Context, _ string, id string, fingerprint string) (skillsupply.Installation, error) {
	return skillsupply.Installation{
		ID: "installed-" + id, Name: "office-xlsx", PackageFingerprint: fingerprint,
	}, nil
}

func (fake fakeSkills) Uninstall(context.Context, string, string, int64) error { return nil }

type fakeMCP struct {
	servers       []mcpclient.Server
	selection     mcpclient.Selection
	search        mcpclient.MarketplaceSearchResult
	details       map[string]mcpclient.MarketplaceItemDetail
	installResult mcpclient.MarketplaceInstallResult
	installErr    error
}

type mutationMCP struct {
	fakeMCP
	replacements []mcpclient.Selection
	deletions    []string
}

func (mcp *mutationMCP) ReplaceSelection(
	_ context.Context,
	_ string,
	selection mcpclient.Selection,
) (mcpclient.Selection, error) {
	mcp.replacements = append(mcp.replacements, selection)
	selection.Revision++
	return selection, nil
}

func (mcp *mutationMCP) DeletePrivateServer(
	_ context.Context,
	_ string,
	serverID string,
) error {
	mcp.deletions = append(mcp.deletions, serverID)
	return nil
}

func (fake fakeMCP) ListServers(context.Context, string, string) ([]mcpclient.Server, error) {
	return fake.servers, nil
}

func (fake fakeMCP) GetSelection(context.Context, string, string) (mcpclient.Selection, error) {
	return fake.selection, nil
}

func (fake fakeMCP) SearchMarketplace(context.Context, mcpclient.MarketplaceSearchInput) (mcpclient.MarketplaceSearchResult, error) {
	return fake.search, nil
}

func (fake fakeMCP) MarketplaceItem(_ context.Context, _ string, identifier string, _ string) (mcpclient.MarketplaceItemDetail, error) {
	return fake.details[identifier], nil
}

func (fake fakeMCP) InstallMarketplaceItem(context.Context, string, mcpclient.MarketplaceInstallInput) (mcpclient.MarketplaceInstallResult, error) {
	if fake.installErr != nil || fake.installResult.Server.Ref.ID != "" ||
		fake.installResult.ValidationError != "" {
		return fake.installResult, fake.installErr
	}
	return mcpclient.MarketplaceInstallResult{Enabled: true}, nil
}

func (fake fakeMCP) ReplaceSelection(
	_ context.Context,
	_ string,
	selection mcpclient.Selection,
) (mcpclient.Selection, error) {
	selection.Revision++
	return selection, nil
}

func (fake fakeMCP) DeletePrivateServer(context.Context, string, string) error { return nil }

func TestCatalogIsSanitizedDeterministicAndSelectionAware(t *testing.T) {
	serverRef := mcpclient.ServerRef{Source: mcpclient.SourcePrivate, ID: "server-id"}
	service := NewService(fakeSkills{library: []skillsupply.Installation{{
		ID: "installation-id", Name: "office-xlsx", Version: "1.0.0",
		PackageFingerprint: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Description:        "Create spreadsheets", AllowedTools: []string{"write"}, Revision: 2,
	}}}, fakeMCP{
		servers: []mcpclient.Server{{
			Ref: serverRef, Name: "DeepWiki", AuthType: mcpclient.AuthHeader,
			Status: mcpclient.ServerStatusReady, HasCredential: true,
			ToolCount: 3, OwnerUserID: "must-not-leak",
		}},
		selection: mcpclient.Selection{
			ConversationID: "conversation-id", Mode: mcpclient.SelectionModeCustom,
			Revision: 4, Servers: []mcpclient.SelectionServer{{Ref: serverRef}},
		},
	})

	first, err := service.Catalog(context.Background(), "user-id", "conversation-id")
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Catalog(context.Background(), "user-id", "conversation-id")
	if err != nil {
		t.Fatal(err)
	}
	if first.Revision == "" || first.Revision != second.Revision || len(first.Skills) != 1 ||
		len(first.MCPServers) != 1 || !first.MCPServers[0].Selected ||
		first.MCPServers[0].CredentialStatus != "configured" {
		t.Fatalf("catalog=%#v", first)
	}
}

func TestSearchFiltersSkillAndBoundsMCPResults(t *testing.T) {
	service := NewService(fakeSkills{store: skillsupply.StoreResult{Items: []skillsupply.Candidate{
		{ID: "one", Status: skillsupply.StatusAdmitted, SourceType: skillsupply.SourceOfficial,
			Package: skillsupply.PackageVersion{Name: "office-xlsx", Version: "1.0.0", Description: "Excel reports", PackageFingerprint: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}},
		{ID: "two", Status: skillsupply.StatusAdmitted, SourceType: skillsupply.SourceOfficial,
			Package: skillsupply.PackageVersion{Name: "translator", Version: "1.0.0", Description: "Translate prose", PackageFingerprint: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}},
	}}}, fakeMCP{search: mcpclient.MarketplaceSearchResult{
		Source: "lobehub", Items: []mcpclient.MarketplaceItem{{Identifier: "deepwiki", Name: "DeepWiki", Validated: true}},
	}})

	skills, err := service.Search(context.Background(), "user-id", KindSkill, "excel")
	if err != nil || len(skills.Items) != 1 || skills.Items[0].Name != "office-xlsx" {
		t.Fatalf("skills=%#v error=%v", skills, err)
	}
	mcp, err := service.Search(context.Background(), "user-id", KindMCP, "wiki")
	if err != nil || len(mcp.Items) != 1 || mcp.Items[0].ID != "deepwiki" {
		t.Fatalf("mcp=%#v error=%v", mcp, err)
	}
	if _, err := service.Search(context.Background(), "user-id", "forged", "x"); err != ErrInvalidQuery {
		t.Fatalf("invalid kind error=%v", err)
	}
}

func TestInstallKillSwitchFailsClosedWithoutMutating(t *testing.T) {
	skills := fakeSkills{store: skillsupply.StoreResult{Items: []skillsupply.Candidate{{
		ID: "candidate-id", Status: skillsupply.StatusAdmitted,
		Package: skillsupply.PackageVersion{
			Name: "office-xlsx", Version: "1.0.0", PackageFingerprint: "sha256:exact",
		},
	}}}}
	service := NewService(skills, fakeMCP{}, WithMutationEnabled(false))
	_, err := service.InstallExplicit(context.Background(), InstallRequest{
		Kind: KindSkill, ID: "candidate-id", Version: "1.0.0",
		ExactRevision: "sha256:exact", UserID: "user-id", ConversationID: "conversation-id",
	})
	if err != ErrDisabled {
		t.Fatalf("error=%v want=%v", err, ErrDisabled)
	}
}

func TestSkillInstallIsIdempotentForExactExistingRevision(t *testing.T) {
	fingerprint := "sha256:" + strings.Repeat("a", 64)
	skills := &countingSkills{fakeSkills: fakeSkills{
		library: []skillsupply.Installation{{
			ID: "installation-id", AdmissionID: "candidate-id", Name: "office-xlsx",
			Version: "1.0.0", PackageFingerprint: fingerprint,
		}},
		store: skillsupply.StoreResult{Items: []skillsupply.Candidate{{
			ID: "candidate-id", Status: skillsupply.StatusAdmitted,
			Package: skillsupply.PackageVersion{
				Name: "office-xlsx", Version: "1.0.0", PackageFingerprint: fingerprint,
			},
		}}},
	}}
	service := NewService(skills, fakeMCP{})

	result, err := service.InstallExplicit(context.Background(), InstallRequest{
		Kind: KindSkill, ID: "candidate-id", Version: "1.0.0",
		ExactRevision: fingerprint, UserID: "user-id", ConversationID: "conversation-id",
	})
	if err != nil || result.ID != "installation-id" || skills.installCalls != 0 {
		t.Fatalf("result=%#v error=%v calls=%d", result, err, skills.installCalls)
	}
}

func TestInstallRecordsSanitizedMutationAudit(t *testing.T) {
	fingerprint := "sha256:" + strings.Repeat("a", 64)
	auditor := &fakeMutationAuditor{}
	service := NewService(fakeSkills{store: skillsupply.StoreResult{Items: []skillsupply.Candidate{{
		ID: "candidate-id", Status: skillsupply.StatusAdmitted,
		Package: skillsupply.PackageVersion{
			Name: "office-xlsx", Version: "1.0.0", PackageFingerprint: fingerprint,
		},
	}}}}, fakeMCP{}, WithMutationAuditor(auditor))
	service.newAuditID = func() string { return "00000000-0000-4000-8000-000000000099" }
	service.now = func() time.Time { return time.Date(2026, 8, 26, 2, 0, 0, 0, time.UTC) }

	result, err := service.InstallExplicit(context.Background(), InstallRequest{
		Kind: KindSkill, ID: "candidate-id", Version: "1.0.0",
		ExactRevision: fingerprint, UserID: "user-id", ConversationID: "conversation-id",
		EntryPoint: "agent_tool",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.MutationAuditID != "00000000-0000-4000-8000-000000000099" || len(auditor.audits) != 1 {
		t.Fatalf("result=%#v audits=%#v", result, auditor.audits)
	}
	audit := auditor.audits[0]
	if audit.Action != ActionInstall || audit.Outcome != MutationOutcomeSuccess ||
		audit.EntryPoint != "agent_tool" ||
		audit.ExactRevision != fingerprint || audit.ResultResourceID != result.ID || audit.ErrorCode != "" {
		t.Fatalf("audit=%#v", audit)
	}
}

func TestSkillRemoveUsesOwnerCASAuthorityAndMutationAudit(t *testing.T) {
	skills := &mutationSkills{
		owner: "owner-id",
		fakeSkills: fakeSkills{library: []skillsupply.Installation{{
			ID: "installation-id", Name: "office-xlsx", Revision: 4,
		}}},
	}
	auditor := &fakeMutationAuditor{}
	service := NewService(skills, fakeMCP{}, WithMutationAuditor(auditor))

	result, err := service.MutateExplicit(context.Background(), MutationRequest{
		Kind: KindSkill, Action: ActionRemove, ID: "installation-id",
		ExpectedRevision: 4, UserID: "owner-id", ConversationID: "conversation-id",
		EntryPoint: "control_plane",
	})
	if err != nil || result.Status != "removed" || len(skills.uninstallCalls) != 1 ||
		len(auditor.audits) != 1 || auditor.audits[0].Action != ActionRemove ||
		auditor.audits[0].ExactRevision != "cas:4" {
		t.Fatalf("result=%#v error=%v calls=%#v audits=%#v", result, err, skills.uninstallCalls, auditor.audits)
	}

	_, err = service.MutateExplicit(context.Background(), MutationRequest{
		Kind: KindSkill, Action: ActionRemove, ID: "installation-id",
		ExpectedRevision: 3, UserID: "owner-id", ConversationID: "conversation-id",
	})
	if !errors.Is(err, ErrRevisionChanged) || len(skills.uninstallCalls) != 1 ||
		len(auditor.audits) != 2 || auditor.audits[1].ErrorCode != "revision_changed" {
		t.Fatalf("error=%v calls=%#v audits=%#v", err, skills.uninstallCalls, auditor.audits)
	}

	_, err = service.MutateExplicit(context.Background(), MutationRequest{
		Kind: KindSkill, Action: ActionRemove, ID: "installation-id",
		ExpectedRevision: 4, UserID: "other-user", ConversationID: "conversation-id",
	})
	if !errors.Is(err, ErrForbidden) || len(skills.uninstallCalls) != 1 ||
		len(auditor.audits) != 3 || auditor.audits[2].ErrorCode != "forbidden" {
		t.Fatalf("error=%v calls=%#v audits=%#v", err, skills.uninstallCalls, auditor.audits)
	}
}

func TestMCPEnableDisableAndRemoveUseCASAuthorityAndMutationAudit(t *testing.T) {
	sharedRef := mcpclient.ServerRef{Source: mcpclient.SourceCatalog, ID: "deepwiki"}
	privateRef := mcpclient.ServerRef{Source: mcpclient.SourcePrivate, ID: "private-id"}
	mcp := &mutationMCP{fakeMCP: fakeMCP{
		servers: []mcpclient.Server{
			{Ref: sharedRef, Name: "DeepWiki", Status: mcpclient.ServerStatusReady},
			{Ref: privateRef, Name: "Private", Status: mcpclient.ServerStatusReady, CanManage: true},
		},
		selection: mcpclient.Selection{
			ConversationID: "conversation-id", Mode: mcpclient.SelectionModeCustom, Revision: 3,
		},
	}}
	auditor := &fakeMutationAuditor{}
	service := NewService(fakeSkills{}, mcp, WithMutationAuditor(auditor))

	enabled, err := service.MutateExplicit(context.Background(), MutationRequest{
		Kind: KindMCP, Action: ActionEnable, ID: sharedRef.Key(), ExpectedRevision: 3,
		UserID: "owner-id", ConversationID: "conversation-id", EntryPoint: "control_plane",
	})
	if err != nil || enabled.Status != "enabled" || enabled.Revision != 4 ||
		len(mcp.replacements) != 1 || !selectionContains(mcp.replacements[0].Servers, sharedRef) ||
		len(auditor.audits) != 1 || auditor.audits[0].Action != ActionEnable {
		t.Fatalf("result=%#v error=%v replacements=%#v audits=%#v", enabled, err, mcp.replacements, auditor.audits)
	}

	_, err = service.MutateExplicit(context.Background(), MutationRequest{
		Kind: KindMCP, Action: ActionDisable, ID: sharedRef.Key(), ExpectedRevision: 2,
		UserID: "owner-id", ConversationID: "conversation-id",
	})
	if !errors.Is(err, ErrRevisionChanged) || len(mcp.replacements) != 1 ||
		len(auditor.audits) != 2 || auditor.audits[1].ErrorCode != "revision_changed" {
		t.Fatalf("error=%v replacements=%#v audits=%#v", err, mcp.replacements, auditor.audits)
	}

	removed, err := service.MutateExplicit(context.Background(), MutationRequest{
		Kind: KindMCP, Action: ActionRemove, ID: privateRef.Key(),
		UserID: "owner-id", ConversationID: "conversation-id",
	})
	if err != nil || removed.Status != "removed" || len(mcp.deletions) != 1 ||
		mcp.deletions[0] != privateRef.ID || len(auditor.audits) != 3 ||
		auditor.audits[2].Action != ActionRemove {
		t.Fatalf("result=%#v error=%v deletions=%#v audits=%#v", removed, err, mcp.deletions, auditor.audits)
	}
}

func TestStaleInstallRecordsFailureWithoutSecretMaterial(t *testing.T) {
	auditor := &fakeMutationAuditor{}
	service := NewService(fakeSkills{store: skillsupply.StoreResult{Items: []skillsupply.Candidate{{
		ID: "candidate-id", Status: skillsupply.StatusAdmitted,
		Package: skillsupply.PackageVersion{
			Name: "office-xlsx", Version: "1.0.0", PackageFingerprint: "sha256:current",
		},
	}}}}, fakeMCP{}, WithMutationAuditor(auditor))

	_, err := service.InstallExplicit(context.Background(), InstallRequest{
		Kind: KindSkill, ID: "candidate-id", Version: "1.0.0",
		ExactRevision: "sha256:stale", UserID: "user-id", ConversationID: "conversation-id",
		EntryPoint: "control_plane",
	})
	if err != ErrRevisionChanged || len(auditor.audits) != 1 ||
		auditor.audits[0].Outcome != MutationOutcomeFailed ||
		auditor.audits[0].ErrorCode != "revision_changed" {
		t.Fatalf("error=%v audits=%#v", err, auditor.audits)
	}
}

func TestSuccessfulInstallFailsClosedWhenAuditCannotBePersisted(t *testing.T) {
	fingerprint := "sha256:" + strings.Repeat("a", 64)
	auditor := &fakeMutationAuditor{err: errors.New("audit offline")}
	service := NewService(fakeSkills{store: skillsupply.StoreResult{Items: []skillsupply.Candidate{{
		ID: "candidate-id", Status: skillsupply.StatusAdmitted,
		Package: skillsupply.PackageVersion{
			Name: "office-xlsx", Version: "1.0.0", PackageFingerprint: fingerprint,
		},
	}}}}, fakeMCP{}, WithMutationAuditor(auditor))

	result, err := service.InstallExplicit(context.Background(), InstallRequest{
		Kind: KindSkill, ID: "candidate-id", Version: "1.0.0",
		ExactRevision: fingerprint, UserID: "user-id", ConversationID: "conversation-id",
	})
	if err != ErrAuditUnavailable || result.ID == "" || result.MutationAuditID != "" || len(auditor.audits) != 1 {
		t.Fatalf("result=%#v error=%v audits=%#v", result, err, auditor.audits)
	}
}

func TestMCPConfigurationHandoffCreatesBoundDraftAndAuditsTransition(t *testing.T) {
	deploymentHash := "sha256:" + strings.Repeat("b", 64)
	privateRef := mcpclient.ServerRef{Source: mcpclient.SourcePrivate, ID: "private-id"}
	auditor := &fakeMutationAuditor{}
	service := NewService(fakeSkills{}, fakeMCP{
		selection: mcpclient.Selection{ConversationID: "conversation-id", Revision: 2},
		details: map[string]mcpclient.MarketplaceItemDetail{"deepwiki": {
			MarketplaceItem: mcpclient.MarketplaceItem{Identifier: "deepwiki", Name: "DeepWiki"},
			Version:         "1.0.0",
			Deployments: []mcpclient.MarketplaceDeployment{{
				Hash: deploymentHash, Compatibility: mcpclient.MarketplaceCompatibilityNeedsConfig,
				InstallMode: "header", SecretFields: []string{"DEEPWIKI_TOKEN"},
			}},
		}},
		installResult: mcpclient.MarketplaceInstallResult{
			Server:          mcpclient.Server{Ref: privateRef, Name: "DeepWiki"},
			ValidationError: "credential_required",
		},
	}, WithMutationAuditor(auditor))

	result, err := service.InstallExplicit(context.Background(), InstallRequest{
		Kind: KindMCP, ID: "deepwiki", Version: "1.0.0", ExactRevision: deploymentHash,
		UserID: "owner-id", ConversationID: "conversation-id", EntryPoint: "agent_tool",
	})
	if !errors.Is(err, ErrConfigurationRequired) || result.ID != privateRef.Key() ||
		result.Status != "configuration_required" || result.MutationAuditID == "" ||
		len(auditor.audits) != 1 || auditor.audits[0].ErrorCode != "configuration_required" ||
		auditor.audits[0].ResultResourceID != privateRef.Key() {
		t.Fatalf("result=%#v error=%v audits=%#v", result, err, auditor.audits)
	}
}

func TestCompleteConfiguredMCPRevalidatesProvenanceReadinessAndSelection(t *testing.T) {
	deploymentHash := "sha256:" + strings.Repeat("c", 64)
	privateRef := mcpclient.ServerRef{Source: mcpclient.SourcePrivate, ID: "private-id"}
	mcp := &mutationMCP{fakeMCP: fakeMCP{
		servers: []mcpclient.Server{{
			Ref: privateRef, Name: "DeepWiki", Status: mcpclient.ServerStatusReady,
			AuthType: mcpclient.AuthHeader, HasCredential: true, CanManage: true,
			Metadata: map[string]any{"marketplace": map[string]any{
				"identifier": "deepwiki", "version": "1.0.0", "deploymentHash": deploymentHash,
			}},
		}},
		selection: mcpclient.Selection{
			ConversationID: "conversation-id", Mode: mcpclient.SelectionModeCustom, Revision: 5,
		},
		details: map[string]mcpclient.MarketplaceItemDetail{"deepwiki": {
			MarketplaceItem: mcpclient.MarketplaceItem{Identifier: "deepwiki", Name: "DeepWiki"},
			Version:         "1.0.0", Deployments: []mcpclient.MarketplaceDeployment{{Hash: deploymentHash}},
		}},
	}}
	auditor := &fakeMutationAuditor{}
	service := NewService(fakeSkills{}, mcp, WithMutationAuditor(auditor))

	result, err := service.CompleteConfiguredMCP(context.Background(), InstallRequest{
		Kind: KindMCP, ID: "deepwiki", Version: "1.0.0", ExactRevision: deploymentHash,
		UserID: "owner-id", ConversationID: "conversation-id",
		EntryPoint: "agent_configuration_resume",
	}, privateRef.Key())
	if err != nil || result.Status != "installed" || !result.RefreshRequired ||
		result.MutationAuditID == "" || len(mcp.replacements) != 1 ||
		!selectionContains(mcp.replacements[0].Servers, privateRef) || len(auditor.audits) != 1 ||
		auditor.audits[0].EntryPoint != "agent_configuration_resume" {
		t.Fatalf("result=%#v error=%v replacements=%#v audits=%#v", result, err, mcp.replacements, auditor.audits)
	}

	mcp.servers[0].HasCredential = false
	_, err = service.CompleteConfiguredMCP(context.Background(), InstallRequest{
		Kind: KindMCP, ID: "deepwiki", Version: "1.0.0", ExactRevision: deploymentHash,
		UserID: "owner-id", ConversationID: "conversation-id",
	}, privateRef.Key())
	if !errors.Is(err, ErrConfigurationRequired) || len(mcp.replacements) != 1 {
		t.Fatalf("error=%v replacements=%#v", err, mcp.replacements)
	}
}

func TestMCPSearchNeverExposesCredentialOrDeploymentMaterial(t *testing.T) {
	service := NewService(fakeSkills{}, fakeMCP{
		search: mcpclient.MarketplaceSearchResult{
			Source: "lobehub",
			Items:  []mcpclient.MarketplaceItem{{Identifier: "deepwiki", Name: "DeepWiki"}},
		},
		details: map[string]mcpclient.MarketplaceItemDetail{
			"deepwiki": {
				MarketplaceItem: mcpclient.MarketplaceItem{Identifier: "deepwiki", Name: "DeepWiki"},
				Version:         "1.0.0",
				Deployments: []mcpclient.MarketplaceDeployment{{
					Recommended: true, Compatibility: mcpclient.MarketplaceCompatibilityNeedsConfig,
					Hash: strings.Repeat("b", 64), InstallMode: "header",
					EndpointURL: "https://private.example.test/mcp", SecretFields: []string{"API_KEY"},
				}},
			},
		},
	})
	result, err := service.Search(context.Background(), "user-id", KindMCP, "deepwiki")
	if err != nil || len(result.Items) != 1 || result.Items[0].AuthType != mcpclient.AuthHeader {
		t.Fatalf("result=%#v error=%v", result, err)
	}
	encoded, _ := json.Marshal(result)
	for _, forbidden := range []string{"private.example.test", "API_KEY", "secretFields", "endpointUrl"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("search response leaked %q: %s", forbidden, encoded)
		}
	}
}

func TestUnavailableSourcesFailWithoutPanicking(t *testing.T) {
	service := NewService(nil, nil)
	if _, err := service.Search(context.Background(), "user-id", KindSkill, "excel"); err != ErrUnavailable {
		t.Fatalf("Skill search error=%v", err)
	}
	if _, err := service.Search(context.Background(), "user-id", KindMCP, "wiki"); err != ErrUnavailable {
		t.Fatalf("MCP search error=%v", err)
	}
}
