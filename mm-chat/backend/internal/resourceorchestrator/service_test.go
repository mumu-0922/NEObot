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

type fakeMCP struct {
	servers   []mcpclient.Server
	selection mcpclient.Selection
	search    mcpclient.MarketplaceSearchResult
	details   map[string]mcpclient.MarketplaceItemDetail
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
	return mcpclient.MarketplaceInstallResult{Enabled: true}, nil
}

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
	if audit.Outcome != MutationOutcomeSuccess || audit.EntryPoint != "agent_tool" ||
		audit.ExactRevision != fingerprint || audit.ResultResourceID != result.ID || audit.ErrorCode != "" {
		t.Fatalf("audit=%#v", audit)
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
