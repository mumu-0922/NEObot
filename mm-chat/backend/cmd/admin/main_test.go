package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"neo-chat/mm-chat/backend/internal/knowledge"
	"neo-chat/mm-chat/backend/internal/runtimeconfig"
)

type fakeGovernanceDisableService struct {
	legacyCalls int
	exactCalls  int
	processor   string
	endpointID  string
	modelID     string
	legacyErr   error
}

func (s *fakeGovernanceDisableService) Disable(
	_ context.Context,
	processor, endpointID string,
) (knowledge.ProcessorGovernanceHead, error) {
	s.legacyCalls++
	s.processor, s.endpointID = processor, endpointID
	return knowledge.ProcessorGovernanceHead{
		Processor: processor, EndpointID: endpointID, ModelID: "only-model",
	}, s.legacyErr
}

func (s *fakeGovernanceDisableService) DisableModel(
	_ context.Context,
	processor, endpointID, modelID string,
) (knowledge.ProcessorGovernanceHead, error) {
	s.exactCalls++
	s.processor, s.endpointID, s.modelID = processor, endpointID, modelID
	return knowledge.ProcessorGovernanceHead{
		Processor: processor, EndpointID: endpointID, ModelID: modelID,
	}, nil
}

func TestReadPasswordLinePreservesSpacesAndRemovesLineEnding(t *testing.T) {
	password, err := readPasswordLine(strings.NewReader("  password with spaces  \r\n"))
	if err != nil {
		t.Fatalf("readPasswordLine() error = %v", err)
	}
	if password != "  password with spaces  " {
		t.Fatalf("password = %q, want spaces preserved", password)
	}
}

func TestReadPasswordLineRejectsMultipleLinesAndOversize(t *testing.T) {
	for name, input := range map[string]string{
		"multiple": "first\nsecond\n",
		"oversize": strings.Repeat("x", 1025),
		"empty":    "\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := readPasswordLine(strings.NewReader(input)); err == nil {
				t.Fatal("readPasswordLine() error = nil, want error")
			}
		})
	}
}

func TestAdminRunRequiresExplicitCommandArguments(t *testing.T) {
	for _, args := range [][]string{
		nil,
		{"unknown"},
		{"bootstrap-identity"},
		{"bootstrap-identity", "--email", "owner@example.test"},
		{"disable-account"},
		{"disable-account", "--user-id"},
		{"governance-apply"},
		{"governance-disable", "--processor", "mineru"},
		{"governance-disable", "--processor", "mineru", "--endpoint-id", "default", "--model-id", ""},
		{"provider-secrets-rewrite", "--execute"},
		{"provider-secrets-rewrite", "--expected-plan-sha256", strings.Repeat("a", 64)},
		{"provider-secrets-rewrite", "--confirmed-backup-sha256", strings.Repeat("b", 64)},
		{"memory-deletions-export", "--output", "out.mm-memory-deletions"},
		{"memory-deletions-replay", "--input", "in.mm-memory-deletions", "--passphrase-stdin"},
		{"backup-retention"},
		{"backup-retention", "--backup-root", "/backup", "--execute"},
	} {
		if err := run(args, strings.NewReader("password\n"), &strings.Builder{}); err == nil {
			t.Fatalf("run(%v) error = nil, want usage error", args)
		}
	}
}

func TestParseProviderSecretRewriteArgsRequiresExactDryRunAndBackupProof(t *testing.T) {
	dryRun, err := parseProviderSecretRewriteArgs(nil)
	if err != nil || dryRun.rewrite.Execute || dryRun.rewrite.ExpectedPlanSHA256 != "" {
		t.Fatalf("dry run = %#v, %v", dryRun, err)
	}

	plan := strings.Repeat("a", 64)
	backup := strings.Repeat("b", 64)
	execute, err := parseProviderSecretRewriteArgs([]string{
		"--execute",
		"--expected-plan-sha256", plan,
		"--confirmed-backup-sha256", backup,
	})
	if err != nil || !execute.rewrite.Execute ||
		execute.rewrite.ExpectedPlanSHA256 != plan ||
		execute.confirmedBackupSHA256 != backup {
		t.Fatalf("execute = %#v, %v", execute, err)
	}

	for _, args := range [][]string{
		{"--execute", "--expected-plan-sha256", "bad", "--confirmed-backup-sha256", backup},
		{"--execute", "--expected-plan-sha256", plan, "--confirmed-backup-sha256", "bad"},
		{"--execute", "--expected-plan-sha256", plan},
		{"--execute", "--confirmed-backup-sha256", backup},
		{"extra"},
	} {
		if _, err := parseProviderSecretRewriteArgs(args); err == nil {
			t.Fatalf("parseProviderSecretRewriteArgs(%v) error = nil", args)
		}
	}
}

func TestWriteProviderSecretRewriteResultIsRedacted(t *testing.T) {
	result := runtimeconfig.ProviderSecretRewriteResult{
		TotalRows: 6, SecretRows: 5, ChangedRows: 4, LegacyRows: 1,
		EnvRows: 1, RotatedRows: 2, CurrentRows: 1, EmptyRows: 1, BlockedRows: 1,
		PlanSHA256: strings.Repeat("c", 64), Executed: true,
	}
	var output strings.Builder
	if err := writeProviderSecretRewriteResult(&output, result); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	for _, want := range []string{
		"mode=executed", "changed_rows=4", "legacy_rows=1",
		"env_rows=1", "rotated_rows=2", "blocked_rows=1",
		"plan_sha256=" + strings.Repeat("c", 64),
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("output %q missing %q", text, want)
		}
	}
	for _, forbidden := range []string{"api_key", "ciphertext", "backup_sha256", "secret="} {
		if strings.Contains(strings.ToLower(text), forbidden) {
			t.Fatalf("output contains %q: %q", forbidden, text)
		}
	}
}

func TestReadGovernanceManifestIsStrictAndBounded(t *testing.T) {
	valid := `{"processor":"mineru","endpointId":"hosted-main","modelId":"model-stable-20260712","modelApiVersion":"api-20260623",` +
		`"allowedPurposes":["parse"],"allowedDataTypes":["application/pdf"],` +
		`"region":"global","retentionPolicy":"none","deletionContract":"delete",` +
		`"trainingUse":"disabled"}`
	manifest, err := readGovernanceManifest(strings.NewReader(valid))
	if err != nil || manifest.Processor != "mineru" ||
		manifest.ModelID != "model-stable-20260712" {
		t.Fatalf("manifest = %#v, err=%v", manifest, err)
	}
	legacy := strings.Replace(valid, `"modelId":"model-stable-20260712",`, "", 1)
	legacyManifest, err := readGovernanceManifest(strings.NewReader(legacy))
	if err != nil || legacyManifest.ModelID != "" {
		t.Fatalf("legacy manifest = %#v, err=%v", legacyManifest, err)
	}
	for name, input := range map[string]string{
		"unknown":      strings.TrimSuffix(valid, "}") + `,"apiKey":"secret"}`,
		"duplicate":    strings.TrimSuffix(valid, "}") + `,"processor":"other"}`,
		"case variant": strings.Replace(valid, `"processor"`, `"Processor"`, 1),
		"trailing":     valid + `{}`,
		"empty":        "",
		"oversize":     valid + strings.Repeat(" ", 64<<10),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := readGovernanceManifest(strings.NewReader(input)); err == nil {
				t.Fatal("readGovernanceManifest() error = nil")
			}
		})
	}
}

func TestBlockedGovernanceManifestCannotApply(t *testing.T) {
	payload, err := os.ReadFile(filepath.Join(
		"..", "..", "..", "docs", "deployment",
		"governance-mineru.blocked.json",
	))
	if err != nil {
		t.Fatal(err)
	}

	var output strings.Builder
	err = run(
		[]string{"governance-apply", "--manifest-stdin"},
		bytes.NewReader(payload),
		&output,
	)
	if !errors.Is(err, errProviderWireContractNotFrozen) ||
		err.Error() != providerWireContractNotFrozenErrorCode {
		t.Fatalf("governance-apply error = %v", err)
	}
	if output.Len() != 0 {
		t.Fatalf("governance-apply output = %q, want empty", output.String())
	}
	if _, err := readGovernanceManifest(bytes.NewReader(payload)); err == nil {
		t.Fatal("blocked governance manifest passed the strict parser")
	}
}

func TestProviderWireContractApplyAllowedRequiresExplicitDraftProfile(t *testing.T) {
	t.Setenv(ragProviderProfileEnv, "")
	t.Setenv(ragProviderProfileDraftAcceptedEnv, "")
	if providerWireContractApplyAllowed() {
		t.Fatal("providerWireContractApplyAllowed() = true without explicit draft acceptance")
	}

	t.Setenv(ragProviderProfileEnv, ragDraftAcceptedProviderProfile)
	if providerWireContractApplyAllowed() {
		t.Fatal("providerWireContractApplyAllowed() = true without draft acceptance")
	}

	t.Setenv(ragProviderProfileEnv, "disabled")
	t.Setenv(ragProviderProfileDraftAcceptedEnv, "true")
	if providerWireContractApplyAllowed() {
		t.Fatal("providerWireContractApplyAllowed() = true for disabled profile")
	}

	t.Setenv(ragProviderProfileEnv, ragDraftAcceptedProviderProfile)
	t.Setenv(ragProviderProfileDraftAcceptedEnv, "true")
	if !providerWireContractApplyAllowed() {
		t.Fatal("providerWireContractApplyAllowed() = false for accepted BGE provider profile")
	}
}

func TestJinaGovernanceManifestCannotApply(t *testing.T) {
	t.Setenv(ragProviderProfileEnv, ragDraftAcceptedProviderProfile)
	t.Setenv(ragProviderProfileDraftAcceptedEnv, "true")
	payload := `{"processor":"Jina-AI","endpointId":"hosted-main",` +
		`"modelId":"jina-embeddings-v4","modelApiVersion":"api-20260623",` +
		`"allowedPurposes":["query_embedding"],` +
		`"allowedDataTypes":["text/plain"],"region":"global",` +
		`"retentionPolicy":"none","deletionContract":"delete",` +
		`"trainingUse":"disabled"}`

	var output strings.Builder
	err := run(
		[]string{"governance-apply", "--manifest-stdin"},
		strings.NewReader(payload),
		&output,
	)
	if !errors.Is(err, errJinaRuntimeRetired) ||
		err.Error() != jinaRuntimeRetiredErrorCode {
		t.Fatalf("governance-apply error = %v", err)
	}
	if output.Len() != 0 {
		t.Fatalf("governance-apply output = %q, want empty", output.String())
	}
}

func TestDisableGovernanceUsesExactModelWhenProvided(t *testing.T) {
	service := &fakeGovernanceDisableService{}
	head, err := disableGovernance(
		context.Background(), service, "jina", "hosted", "embed-a",
	)
	if err != nil {
		t.Fatal(err)
	}
	if service.exactCalls != 1 || service.legacyCalls != 0 ||
		service.processor != "jina" || service.endpointID != "hosted" ||
		service.modelID != "embed-a" || head.ModelID != "embed-a" {
		t.Fatalf("service = %#v, head = %#v", service, head)
	}
}

func TestDisableGovernanceKeepsLegacyUniqueModelCompatibility(t *testing.T) {
	service := &fakeGovernanceDisableService{}
	head, err := disableGovernance(
		context.Background(), service, "mineru", "default", "",
	)
	if err != nil {
		t.Fatal(err)
	}
	if service.legacyCalls != 1 || service.exactCalls != 0 ||
		head.ModelID != "only-model" {
		t.Fatalf("service = %#v, head = %#v", service, head)
	}
}

func TestDisableGovernanceLegacyCallFailsClosedOnAmbiguousModel(t *testing.T) {
	service := &fakeGovernanceDisableService{
		legacyErr: knowledge.ErrGovernanceIdentityAmbiguous,
	}
	_, err := disableGovernance(
		context.Background(), service, "jina", "hosted", "",
	)
	if err != knowledge.ErrGovernanceIdentityAmbiguous ||
		service.legacyCalls != 1 || service.exactCalls != 0 {
		t.Fatalf("service = %#v, err = %v", service, err)
	}
}

func TestUsageDocumentsGovernanceModelID(t *testing.T) {
	if usage := usageError().Error(); !strings.Contains(usage, "[--model-id <id>]") {
		t.Fatalf("usage = %q", usage)
	}
}

func TestGovernanceSuccessOutputIncludesExactModelWithoutCredentials(t *testing.T) {
	head := knowledge.ProcessorGovernanceHead{
		Processor:                "jina",
		EndpointID:               "hosted",
		ModelID:                  "embed-a",
		ActiveProfileID:          "profile-id",
		ActiveGovernanceRevision: 2,
		HeadRevision:             3,
	}
	tests := []struct {
		name  string
		write func(*strings.Builder, knowledge.ProcessorGovernanceHead) error
		want  string
	}{
		{
			name: "apply",
			write: func(output *strings.Builder, value knowledge.ProcessorGovernanceHead) error {
				return writeGovernanceApplyResult(output, value)
			},
			want: "governance active processor=jina endpoint=hosted model=embed-a profile=profile-id governance_revision=2 head_revision=3\n",
		},
		{
			name: "disable",
			write: func(output *strings.Builder, value knowledge.ProcessorGovernanceHead) error {
				return writeGovernanceDisableResult(output, value)
			},
			want: "governance disabled processor=jina endpoint=hosted model=embed-a head_revision=3\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output strings.Builder
			if err := test.write(&output, head); err != nil {
				t.Fatal(err)
			}
			if got := output.String(); got != test.want {
				t.Fatalf("output = %q, want %q", got, test.want)
			}
			for _, credential := range []string{"api_key", "password", "secret", "token"} {
				if strings.Contains(strings.ToLower(output.String()), credential) {
					t.Fatalf("output contains credential field %q: %q", credential, output.String())
				}
			}
		})
	}
}
