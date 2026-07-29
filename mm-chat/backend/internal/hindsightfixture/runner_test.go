package hindsightfixture

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"neo-chat/mm-chat/backend/internal/memoryeval"
)

type fakeAdapter struct {
	configured  []Mode
	retained    []RetainItem
	deletedDoc  []string
	deletedBank []string
	results     map[string][]string
	failConfig  error
}

func (fake *fakeAdapter) ConfigureBank(_ context.Context, _ string, mode Mode) error {
	fake.configured = append(fake.configured, mode)
	return fake.failConfig
}

func (fake *fakeAdapter) Retain(_ context.Context, _ string, item RetainItem) error {
	fake.retained = append(fake.retained, item)
	return nil
}

func (fake *fakeAdapter) Recall(_ context.Context, _ string, query string, _ RecallScope) ([]string, error) {
	return append([]string(nil), fake.results[query]...), nil
}

func (fake *fakeAdapter) DeleteDocument(_ context.Context, _ string, documentID string) error {
	fake.deletedDoc = append(fake.deletedDoc, documentID)
	return nil
}

func (fake *fakeAdapter) DeleteBank(_ context.Context, bankID string) error {
	fake.deletedBank = append(fake.deletedBank, bankID)
	return nil
}

func TestRunnerDualModesRejectForbiddenInputsAndDeleteEveryBank(t *testing.T) {
	manifest, golden, goldenHash := checkedInFixture(t)
	for _, mode := range []Mode{ModeEndToEnd, ModeRetrievalOnly} {
		t.Run(string(mode), func(t *testing.T) {
			fake := &fakeAdapter{results: make(map[string][]string)}
			for _, item := range golden.Cases {
				fake.results[item.Query] = append([]string(nil), item.ExpectedRelevantMemoryIDs...)
			}
			runner, err := NewRunner(fake, strings.Repeat("k", 32))
			if err != nil {
				t.Fatal(err)
			}
			report := runner.Run(context.Background(), manifest, golden, goldenHash, mode)
			if !report.Passed || report.PromotionEligible || report.Profile.RemoteProviderCalls != 0 {
				t.Fatalf("report = %#v", report)
			}
			for _, result := range report.Cases {
				if result.CandidateMemoryIDs == nil || result.FinalMemoryIDs == nil ||
					result.PersistedMemoryIDs == nil || result.ProviderSentMemoryIDs == nil {
					t.Fatalf("case result contains a null ID list: %#v", result)
				}
			}
			if len(fake.configured) != len(manifest.Fixtures) ||
				len(fake.deletedBank) != len(manifest.Fixtures) || len(fake.deletedDoc) != 1 {
				t.Fatalf("config/delete counts = %d/%d/%d", len(fake.configured), len(fake.deletedBank), len(fake.deletedDoc))
			}
			for _, bankID := range fake.deletedBank {
				if !strings.HasPrefix(bankID, "neo-") || strings.Contains(bankID, "fixture") {
					t.Fatalf("non-opaque bank ID = %q", bankID)
				}
			}
			for _, retained := range fake.retained {
				if retained.Metadata["neo_memory_id"] == "memory-credential-sentinel" ||
					retained.Metadata["neo_memory_id"] == "memory-assistant-claim" ||
					retained.Metadata["neo_memory_id"] == "memory-quoted-claim" {
					t.Fatalf("rejected Memory was retained: %#v", retained)
				}
				memory := findMemory(t, manifest, retained.Metadata["neo_memory_id"])
				want := memory.CanonicalContent
				if mode == ModeEndToEnd {
					want = memory.RawEventContent
				}
				if retained.Content != want {
					t.Fatalf("retained content = %q, want mode content %q", retained.Content, want)
				}
			}
			body, err := json.Marshal(report)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(body, []byte(golden.Cases[0].Query)) ||
				bytes.Contains(body, []byte(manifest.Fixtures[0].Memories[0].CanonicalContent)) {
				t.Fatal("report leaked fixture/query plaintext")
			}
			for _, bankID := range fake.deletedBank {
				if bytes.Contains(body, []byte(bankID)) {
					t.Fatal("report leaked an opaque bank ID")
				}
			}
		})
	}
}

func TestRunnerFailsClosedOnCrossBankResultAndStillDeletesBanks(t *testing.T) {
	manifest, golden, goldenHash := checkedInFixture(t)
	fake := &fakeAdapter{results: make(map[string][]string)}
	for _, item := range golden.Cases {
		fake.results[item.Query] = append([]string(nil), item.ExpectedRelevantMemoryIDs...)
		if item.ID == "draft-scope" {
			fake.results[item.Query] = []string{"memory-owned-by-user-b"}
		}
	}
	runner, _ := NewRunner(fake, strings.Repeat("k", 32))
	report := runner.Run(context.Background(), manifest, golden, goldenHash, ModeRetrievalOnly)
	if report.Passed || len(fake.deletedBank) != len(manifest.Fixtures) {
		t.Fatalf("report passed/deleted = %v/%d", report.Passed, len(fake.deletedBank))
	}
	for _, result := range report.Cases {
		if result.CaseID == "draft-scope" && result.ErrorCode != "cross_bank_result" {
			t.Fatalf("scope result = %#v", result)
		}
	}
}

func TestRunnerSetupFailureIsSanitizedAndCleaned(t *testing.T) {
	manifest, golden, goldenHash := checkedInFixture(t)
	fake := &fakeAdapter{failConfig: &Fault{Code: "unauthorized"}}
	runner, _ := NewRunner(fake, strings.Repeat("k", 32))
	report := runner.Run(context.Background(), manifest, golden, goldenHash, ModeEndToEnd)
	if report.Passed || report.ErrorCode != "unauthorized" ||
		len(fake.deletedBank) != len(manifest.Fixtures) {
		t.Fatalf("report/cleanup = %#v/%d", report, len(fake.deletedBank))
	}
}

func checkedInFixture(t *testing.T) (Manifest, memoryeval.GoldenSet, string) {
	t.Helper()
	manifestFile, err := os.Open("../../../docs/contracts/memory-hindsight-fixture-draft.json")
	if err != nil {
		t.Fatal(err)
	}
	defer manifestFile.Close()
	manifest, err := DecodeManifest(manifestFile)
	if err != nil {
		t.Fatal(err)
	}
	goldenBody, err := os.ReadFile("../../../docs/contracts/memory-benchmark-golden-draft-template.json")
	if err != nil {
		t.Fatal(err)
	}
	golden, err := memoryeval.DecodeGoldenSet(bytes.NewReader(goldenBody))
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(goldenBody)
	return manifest, golden, hex.EncodeToString(digest[:])
}

func findMemory(t *testing.T, manifest Manifest, memoryID string) Memory {
	t.Helper()
	for _, fixture := range manifest.Fixtures {
		for _, memory := range fixture.Memories {
			if memory.ID == memoryID {
				return memory
			}
		}
	}
	t.Fatalf("Memory %q not found", memoryID)
	return Memory{}
}
