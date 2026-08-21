package agenthost

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadClientCredentialsAcceptsReadOnlyMountedFiles(t *testing.T) {
	directory := t.TempDir()
	tokenPath := filepath.Join(directory, "token")
	runnerPath := filepath.Join(directory, "runner-id")
	if err := os.WriteFile(tokenPath, []byte(testToken+"\n"), 0o444); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(runnerPath, []byte("wsl-test-runner\n"), 0o444); err != nil {
		t.Fatal(err)
	}
	token, runnerID, err := LoadClientCredentials(tokenPath, runnerPath)
	if err != nil {
		t.Fatalf("LoadClientCredentials() error = %v", err)
	}
	if token != testToken || runnerID != "wsl-test-runner" {
		t.Fatalf("credentials = %q / %q", token, runnerID)
	}
}

func TestLoadClientCredentialsRejectsWritableOrSymlinkedFiles(t *testing.T) {
	directory := t.TempDir()
	tokenPath := filepath.Join(directory, "token")
	runnerPath := filepath.Join(directory, "runner-id")
	if err := os.WriteFile(tokenPath, []byte(testToken), 0o666); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(tokenPath, 0o666); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(runnerPath, []byte("wsl-test-runner"), 0o444); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadClientCredentials(tokenPath, runnerPath); err == nil {
		t.Fatal("writable token was accepted")
	}
	if err := os.Chmod(tokenPath, 0o444); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(directory, "runner-alias")
	if err := os.Symlink(runnerPath, alias); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadClientCredentials(tokenPath, alias); err == nil {
		t.Fatal("symlinked Runner id was accepted")
	}
}
