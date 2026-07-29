package memoryauthor

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/memoryeval"
	"neo-chat/mm-chat/backend/internal/strictjson"
)

func TestFreezeAndOneShotHoldoutEndToEnd(t *testing.T) {
	root := newTestPoolRoot(t)
	seedTestOnlyDecisions(t, root)
	frozenAt := time.Date(2026, time.July, 29, 2, 0, 0, 0, time.UTC)
	frozen, err := Freeze(root, FreezeInput{
		HoldoutRunID: testHoldoutID, Clock: func() time.Time { return frozenAt },
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(frozen.Golden.Cases) != 500 || len(frozen.Fixtures.Fixtures) != 500 {
		t.Fatalf("frozen cases/fixtures = %d/%d", len(frozen.Golden.Cases), len(frozen.Fixtures.Fixtures))
	}
	if err := memoryeval.ValidateGoldenAdmission(frozen.Golden); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadFrozen(root)
	if err != nil || loaded.Manifest.GoldenContentSHA256 != frozen.Manifest.GoldenContentSHA256 {
		t.Fatalf("LoadFrozen() = %+v, %v", loaded.Manifest, err)
	}
	status, err := CurrentStatus(root)
	if err != nil || !status.Frozen || status.HoldoutState != "sealed" {
		t.Fatalf("sealed status = %+v, %v", status, err)
	}
	if server, err := StartReviewServer(ReviewServerOptions{Root: root, ReviewerID: testReviewerID}); err == nil {
		_ = server.Close(context.Background())
		t.Fatal("review server started after freeze")
	}

	preexisting := filepath.Join(root, HoldoutDirectory, "already.json")
	if err := os.WriteFile(preexisting, []byte("exists\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := BeginHoldout(root, HoldoutInput{
		HoldoutRunID: testHoldoutID, OutputPath: preexisting,
		Clock: func() time.Time { return frozenAt.Add(time.Hour) },
	}); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("preflight existing output error = %v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, HoldoutDirectory, HoldoutUseFile)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("preflight failure consumed Holdout: %v", err)
	}

	output := filepath.Join(root, HoldoutDirectory, "run-input.json")
	use, err := BeginHoldout(root, HoldoutInput{
		HoldoutRunID: testHoldoutID, OutputPath: output,
		Clock: func() time.Time { return frozenAt.Add(time.Hour) },
	})
	if err != nil || use.Ordinal != 1 || use.State != "consumed" {
		t.Fatalf("BeginHoldout() = %+v, %v", use, err)
	}
	body, err := readSecureArtifact(output)
	if err != nil {
		t.Fatal(err)
	}
	var bundle HoldoutBundle
	if err := strictjson.Decode(body, maximumArtifactBytes, &bundle); err != nil {
		t.Fatal(err)
	}
	if err := validateHoldoutBundle(bundle); err != nil || len(bundle.Cases) != 100 {
		t.Fatalf("Holdout bundle = %d cases, %v", len(bundle.Cases), err)
	}
	status, err = CurrentStatus(root)
	if err != nil || status.HoldoutState != "consumed" {
		t.Fatalf("consumed status = %+v, %v", status, err)
	}
	secondOutput := filepath.Join(root, HoldoutDirectory, "second.json")
	if _, err := BeginHoldout(root, HoldoutInput{
		HoldoutRunID: testHoldoutID, OutputPath: secondOutput,
		Clock: func() time.Time { return frozenAt.Add(2 * time.Hour) },
	}); err == nil || !strings.Contains(err.Error(), "permanently refused") {
		t.Fatalf("second Holdout error = %v", err)
	}
}

func TestFreezeRejectsIncompleteReviewAndFrozenTamper(t *testing.T) {
	root := newTestPoolRoot(t)
	if _, err := Freeze(root, FreezeInput{HoldoutRunID: testHoldoutID}); err == nil ||
		!strings.Contains(err.Error(), "exactly 500 accepted") {
		t.Fatalf("incomplete freeze error = %v", err)
	}

	root = newTestPoolRoot(t)
	seedTestOnlyDecisions(t, root)
	if _, err := Freeze(root, FreezeInput{
		HoldoutRunID: testHoldoutID,
		Clock:        func() time.Time { return time.Date(2026, time.July, 29, 2, 0, 0, 0, time.UTC) },
	}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, FrozenDirectory, FreezeManifestFile)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	body = bytes.Replace(body, []byte(testHoldoutID), []byte("33333333-3333-4333-8333-333333333333"), 1)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFrozen(root); err == nil {
		t.Fatal("tampered freeze manifest was accepted")
	}
}
