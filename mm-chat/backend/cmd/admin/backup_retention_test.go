package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBackupRetentionDryRunExecuteAndOrphanPreservation(t *testing.T) {
	root := newBackupRoot(t)
	now := time.Date(2026, 7, 28, 0, 0, 0, 0, time.UTC)
	expiredID := "20260714T000000Z-expired"
	recentID := "20260727T000000Z-recent"
	writeBackupSet(t, root, expiredID, "daily", now.Add(-dailyRetention))
	writeBackupSet(t, root, recentID, "weekly", now.Add(-24*time.Hour))
	orphan := filepath.Join(root, "postgres", "orphan.dump")
	writeOwnerFile(t, orphan, []byte("orphan"))

	plan, planSHA, err := buildBackupRetentionPlan(root, now)
	if err != nil {
		t.Fatalf("buildBackupRetentionPlan() error = %v", err)
	}
	if len(plan.Sets) != 1 || plan.Sets[0].SetID != expiredID ||
		len(plan.Sets[0].Files) != 6 || !sha256Pattern.MatchString(planSHA) {
		t.Fatalf("plan=%#v sha=%q", plan, planSHA)
	}
	again, againSHA, err := buildBackupRetentionPlan(root, now)
	if err != nil || againSHA != planSHA || len(again.Sets) != 1 {
		t.Fatalf("second plan=%#v sha=%q err=%v", again, againSHA, err)
	}

	dryRun, _, err := applyBackupRetention(root, now, false, "")
	if err != nil || dryRun.Executed || dryRun.DeletedFiles != 0 {
		t.Fatalf("dry run=%#v err=%v", dryRun, err)
	}
	executed, _, err := applyBackupRetention(root, now, true, planSHA)
	if err != nil || executed.DeletedSets != 1 || executed.DeletedFiles != 6 {
		t.Fatalf("execute=%#v err=%v", executed, err)
	}
	for _, relative := range plan.Sets[0].Files {
		if _, err := os.Lstat(filepath.Join(root, filepath.FromSlash(relative))); !os.IsNotExist(err) {
			t.Fatalf("expired path still exists: %s (%v)", relative, err)
		}
	}
	if _, err := os.Stat(orphan); err != nil {
		t.Fatalf("orphan was deleted: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "sets", recentID+".json")); err != nil {
		t.Fatalf("recent set was deleted: %v", err)
	}
}

func TestBackupRetentionFailsClosedOnDriftChecksumAndSymlink(t *testing.T) {
	now := time.Date(2026, 7, 28, 0, 0, 0, 0, time.UTC)

	t.Run("plan drift", func(t *testing.T) {
		root := newBackupRoot(t)
		firstID := "20260701T000000Z-first"
		writeBackupSet(t, root, firstID, "daily", now.Add(-30*24*time.Hour))
		_, firstSHA, err := buildBackupRetentionPlan(root, now)
		if err != nil {
			t.Fatal(err)
		}
		writeBackupSet(t, root, "20260702T000000Z-second", "daily", now.Add(-29*24*time.Hour))
		result, _, err := applyBackupRetention(root, now, true, firstSHA)
		if err == nil || result.DeletedFiles != 0 {
			t.Fatalf("plan drift result=%#v err=%v", result, err)
		}
		if _, err := os.Stat(filepath.Join(root, "sets", firstID+".json")); err != nil {
			t.Fatalf("drift deleted the old set: %v", err)
		}
	})

	t.Run("checksum", func(t *testing.T) {
		root := newBackupRoot(t)
		setID := "20260701T000000Z-checksum"
		writeBackupSet(t, root, setID, "daily", now.Add(-30*24*time.Hour))
		writeOwnerFile(t, filepath.Join(root, "postgres", "postgres-"+setID+".dump"), []byte("tampered"))
		if _, _, err := buildBackupRetentionPlan(root, now); err == nil {
			t.Fatal("tampered artifact was accepted")
		}
	})

	t.Run("symlink", func(t *testing.T) {
		root := newBackupRoot(t)
		setID := "20260701T000000Z-symlink"
		writeBackupSet(t, root, setID, "daily", now.Add(-30*24*time.Hour))
		artifact := filepath.Join(root, "minio", "minio-"+setID+".tar.gz")
		if err := os.Remove(artifact); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join(root, "postgres", "postgres-"+setID+".dump"), artifact); err != nil {
			t.Fatal(err)
		}
		if _, _, err := buildBackupRetentionPlan(root, now); err == nil {
			t.Fatal("symlink artifact was accepted")
		}
	})
}

func TestBackupRetentionStrictManifestAndPartialDeleteReporting(t *testing.T) {
	now := time.Date(2026, 7, 28, 0, 0, 0, 0, time.UTC)

	t.Run("duplicate field", func(t *testing.T) {
		var manifest backupSetManifest
		payload := []byte(`{"version":1,"version":1}`)
		if err := decodeStrictBackupManifest(payload, &manifest); err == nil {
			t.Fatal("duplicate field was accepted")
		}
	})

	t.Run("partial delete", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("directory write denial cannot be proved as root")
		}
		root := newBackupRoot(t)
		setID := "20260701T000000Z-partial"
		writeBackupSet(t, root, setID, "daily", now.Add(-30*24*time.Hour))
		_, planSHA, err := buildBackupRetentionPlan(root, now)
		if err != nil {
			t.Fatal(err)
		}
		minioDir := filepath.Join(root, "minio")
		if err := os.Chmod(minioDir, 0o500); err != nil {
			t.Fatal(err)
		}
		defer func() { _ = os.Chmod(minioDir, 0o700) }()
		result, _, err := applyBackupRetention(root, now, true, planSHA)
		if err == nil || result.DeletedFiles != 2 ||
			!strings.Contains(err.Error(), "after deleting 2 files") {
			t.Fatalf("partial result=%#v err=%v", result, err)
		}
	})
}

func newBackupRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, directory := range []string{"sets", "postgres", "minio"} {
		if err := os.Mkdir(filepath.Join(root, directory), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func writeBackupSet(
	t *testing.T,
	root, setID, class string,
	createdAt time.Time,
) {
	t.Helper()
	artifacts := []backupSetArtifact{
		{
			Kind: "postgres", Path: "postgres/postgres-" + setID + ".dump",
			ChecksumPath: "postgres/postgres-" + setID + ".dump.sha256",
		},
		{
			Kind: "minio", Path: "minio/minio-" + setID + ".tar.gz",
			ChecksumPath: "minio/minio-" + setID + ".tar.gz.sha256",
		},
	}
	for index := range artifacts {
		artifactPath := filepath.Join(root, filepath.FromSlash(artifacts[index].Path))
		payload := []byte(artifacts[index].Kind + "-" + setID)
		writeOwnerFile(t, artifactPath, payload)
		artifacts[index].SHA256 = testSHA256(payload)
		checksum := artifacts[index].SHA256 + "  " + filepath.Base(artifactPath) + "\n"
		writeOwnerFile(
			t, filepath.Join(root, filepath.FromSlash(artifacts[index].ChecksumPath)),
			[]byte(checksum),
		)
	}
	manifest := backupSetManifest{
		Version: backupSetManifestVersion, SetID: setID, Class: class,
		CreatedAt:               createdAt.UTC().Format(time.RFC3339Nano),
		ContainsMemoryPlaintext: true, Artifacts: artifacts,
	}
	payload, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(root, "sets", setID+".json")
	writeOwnerFile(t, manifestPath, append(payload, '\n'))
	manifestHash := testSHA256(append(payload, '\n'))
	writeOwnerFile(
		t, manifestPath+".sha256",
		[]byte(manifestHash+"  "+filepath.Base(manifestPath)+"\n"),
	)
}

func writeOwnerFile(t *testing.T, path string, payload []byte) {
	t.Helper()
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}
}

func testSHA256(payload []byte) string {
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}
