package main

import (
	"context"
	"strings"
	"testing"

	"neo-chat/mm-chat/backend/internal/knowledge"
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
	} {
		if err := run(args, strings.NewReader("password\n"), &strings.Builder{}); err == nil {
			t.Fatalf("run(%v) error = nil, want usage error", args)
		}
	}
}

func TestReadGovernanceManifestIsStrictAndBounded(t *testing.T) {
	valid := `{"processor":"mineru","endpointId":"default","modelId":"model-v1","modelApiVersion":"v1",` +
		`"allowedPurposes":["parse"],"allowedDataTypes":["application/pdf"],` +
		`"region":"global","retentionPolicy":"none","deletionContract":"delete",` +
		`"trainingUse":"disabled"}`
	manifest, err := readGovernanceManifest(strings.NewReader(valid))
	if err != nil || manifest.Processor != "mineru" || manifest.ModelID != "model-v1" {
		t.Fatalf("manifest = %#v, err=%v", manifest, err)
	}
	legacy := strings.Replace(valid, `"modelId":"model-v1",`, "", 1)
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
