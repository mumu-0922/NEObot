package agenthost

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
)

func secureSocketDirectory(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	if err := os.Chmod(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	return directory
}

func TestListenUnixCreatesAndRemovesPrivateSocket(t *testing.T) {
	path := filepath.Join(secureSocketDirectory(t), "agent-host.sock")
	listener, err := ListenUnix(path)
	if err != nil {
		t.Fatalf("ListenUnix() error = %v", err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSocket == 0 || info.Mode().Perm() != 0o600 {
		t.Fatalf("socket mode = %v", info.Mode())
	}
	if err := listener.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("socket remains after close: %v", err)
	}
}

func TestListenUnixRejectsActiveSocket(t *testing.T) {
	path := filepath.Join(secureSocketDirectory(t), "agent-host.sock")
	listener, err := ListenUnix(path)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if _, err := ListenUnix(path); !errors.Is(err, ErrSocketAlreadyActive) {
		t.Fatalf("error = %v, want %v", err, ErrSocketAlreadyActive)
	}
}

func TestListenUnixReplacesOwnedStaleSocket(t *testing.T) {
	path := filepath.Join(secureSocketDirectory(t), "agent-host.sock")
	raw, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	raw.SetUnlinkOnClose(false)
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}
	listener, err := ListenUnix(path)
	if err != nil {
		t.Fatalf("ListenUnix() error = %v", err)
	}
	defer listener.Close()
	if connection, err := net.Dial("unix", path); err != nil {
		t.Fatalf("dial replacement: %v", err)
	} else {
		_ = connection.Close()
	}
}

func TestListenUnixRejectsUnsafePathAndDirectory(t *testing.T) {
	root := secureSocketDirectory(t)
	regular := filepath.Join(root, "regular.sock")
	if err := os.WriteFile(regular, []byte("do not remove"), 0o600); err != nil {
		t.Fatal(err)
	}
	symlink := filepath.Join(root, "symlink.sock")
	if err := os.Symlink(regular, symlink); err != nil {
		t.Fatal(err)
	}
	wrongModeDirectory := filepath.Join(root, "wrong-mode")
	if err := os.Mkdir(wrongModeDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	linkedTarget := filepath.Join(root, "linked-target")
	if err := os.Mkdir(linkedTarget, 0o700); err != nil {
		t.Fatal(err)
	}
	linkedDirectory := filepath.Join(root, "linked")
	if err := os.Symlink(linkedTarget, linkedDirectory); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		path string
		want error
	}{
		{"relative", "agent-host.sock", ErrSocketInvalid},
		{"regular file", regular, ErrSocketInvalid},
		{"symlink", symlink, ErrSocketInvalid},
		{"wrong directory mode", filepath.Join(wrongModeDirectory, "agent-host.sock"), ErrSocketDirectory},
		{"symlink directory", filepath.Join(linkedDirectory, "agent-host.sock"), ErrSocketDirectory},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := ListenUnix(test.path); !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
		})
	}
	data, err := os.ReadFile(regular)
	if err != nil || string(data) != "do not remove" {
		t.Fatalf("regular path was altered: data=%q err=%v", data, err)
	}
}

func TestUnixListenerCloseDoesNotRemoveReplacement(t *testing.T) {
	path := filepath.Join(secureSocketDirectory(t), "agent-host.sock")
	listener, err := ListenUnix(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("replacement"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := listener.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "replacement" {
		t.Fatalf("replacement was removed: data=%q err=%v", data, err)
	}
}
