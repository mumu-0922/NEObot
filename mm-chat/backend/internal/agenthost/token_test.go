package agenthost

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTokenFile(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(testToken+"\n"), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

func TestLoadTokenFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent-host-token")
	writeTokenFile(t, path, 0o600)
	token, err := LoadTokenFile(path)
	if err != nil {
		t.Fatalf("LoadTokenFile() error = %v", err)
	}
	if token != testToken {
		t.Fatalf("token = %q", token)
	}
}

func TestLoadTokenFileRejectsUnsafeFiles(t *testing.T) {
	root := t.TempDir()
	valid := filepath.Join(root, "valid")
	writeTokenFile(t, valid, 0o600)
	symlink := filepath.Join(root, "symlink")
	if err := os.Symlink(valid, symlink); err != nil {
		t.Fatal(err)
	}
	linkedDirectory := filepath.Join(root, "linked-directory")
	targetDirectory := filepath.Join(root, "target-directory")
	if err := os.Mkdir(targetDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(targetDirectory, linkedDirectory); err != nil {
		t.Fatal(err)
	}
	viaLinkedDirectory := filepath.Join(linkedDirectory, "token")
	writeTokenFile(t, filepath.Join(targetDirectory, "token"), 0o600)
	wrongMode := filepath.Join(root, "wrong-mode")
	writeTokenFile(t, wrongMode, 0o640)
	short := filepath.Join(root, "short")
	if err := os.WriteFile(short, []byte("short"), 0o600); err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(root, "directory")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		path string
	}{
		{"relative", "token"},
		{"missing", filepath.Join(root, "missing")},
		{"symlink", symlink},
		{"symlink component", viaLinkedDirectory},
		{"wrong mode", wrongMode},
		{"short", short},
		{"directory", directory},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := LoadTokenFile(test.path); err == nil {
				t.Fatalf("LoadTokenFile(%q) succeeded", test.path)
			} else if strings.Contains(err.Error(), testToken) {
				t.Fatal("token error disclosed token material")
			}
		})
	}
}

func TestLoadTokenFileRejectsUnexpectedOwner(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent-host-token")
	writeTokenFile(t, path, 0o600)
	_, err := loadTokenFile(path, os.Geteuid()+1)
	if err == nil || !strings.Contains(err.Error(), "owner") {
		t.Fatalf("error = %v, want owner failure", err)
	}
}

func TestValidateTokenRejectsControlAndBounds(t *testing.T) {
	for _, token := range []string{
		"short",
		strings.Repeat("a", maxTokenBytes+1),
		strings.Repeat("a", minTokenBytes) + "\n",
	} {
		if err := validateToken(token); err == nil {
			t.Fatalf("validateToken(%q) succeeded", token)
		} else if errors.Is(err, os.ErrPermission) {
			t.Fatalf("unexpected error = %v", err)
		}
	}
}
