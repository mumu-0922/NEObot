package agenthost

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type stubWindowsConverter struct {
	path string
	err  error
}

func (converter stubWindowsConverter) ToWSL(context.Context, string) (string, error) {
	return converter.path, converter.err
}

func newTestResolver(t *testing.T) *LocalWorkspaceResolver {
	t.Helper()
	resolver, err := NewLocalWorkspaceResolver("wsl-test-runner")
	if err != nil {
		t.Fatalf("NewLocalWorkspaceResolver() error = %v", err)
	}
	resolver.Converter = nil
	return resolver
}

func TestLocalWorkspaceResolverCanonicalizesSymlinkAliases(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	alias := filepath.Join(root, "project-alias")
	if err := os.Mkdir(project, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(project, alias); err != nil {
		t.Fatal(err)
	}
	resolver := newTestResolver(t)
	canonical, err := resolver.ResolveWorkspace(context.Background(), project)
	if err != nil {
		t.Fatalf("resolve canonical: %v", err)
	}
	aliased, err := resolver.ResolveWorkspace(context.Background(), alias)
	if err != nil {
		t.Fatalf("resolve alias: %v", err)
	}
	if canonical != aliased {
		t.Fatalf("alias descriptor drifted:\ncanonical=%+v\nalias=%+v", canonical, aliased)
	}
	if canonical.PathKind != "wsl" || !strings.HasPrefix(canonical.DirectoryFingerprint, "sha256:") {
		t.Fatalf("unexpected descriptor: %+v", canonical)
	}
}

func TestLocalWorkspaceResolverConvertsWindowsPath(t *testing.T) {
	project := t.TempDir()
	resolver := newTestResolver(t)
	resolver.Converter = stubWindowsConverter{path: project}
	descriptor, err := resolver.ResolveWorkspace(context.Background(), `D:\projects\neo-chat`)
	if err != nil {
		t.Fatalf("ResolveWorkspace() error = %v", err)
	}
	if descriptor.CanonicalPath != project || descriptor.DisplayPath != `D:\projects\neo-chat` ||
		descriptor.PathKind != "windows-mounted" {
		t.Fatalf("unexpected descriptor: %+v", descriptor)
	}
}

func TestLocalWorkspaceResolverRejectsInvalidPaths(t *testing.T) {
	resolver := newTestResolver(t)
	filePath := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(filePath, []byte("test"), 0o600); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(t.TempDir(), "missing", "private")
	tests := []struct {
		name string
		path string
		want error
	}{
		{"empty", "", ErrWorkspacePathInvalid},
		{"relative", "relative/project", ErrWorkspacePathInvalid},
		{"control", "/tmp/project\nprivate", ErrWorkspacePathInvalid},
		{"file", filePath, ErrWorkspacePathUnavailable},
		{"missing", missing, ErrWorkspacePathUnavailable},
		{"windows unavailable", `C:\private`, ErrWindowsInteropUnavailable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := resolver.ResolveWorkspace(context.Background(), test.path)
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
			if err != nil && test.path != "" && strings.Contains(err.Error(), test.path) {
				t.Fatal("resolver error disclosed Host path")
			}
		})
	}
}

func TestLocalWorkspaceResolverPropagatesConverterFailure(t *testing.T) {
	resolver := newTestResolver(t)
	resolver.Converter = stubWindowsConverter{err: ErrWorkspacePathInvalid}
	_, err := resolver.ResolveWorkspace(context.Background(), `D:\private\missing`)
	if !errors.Is(err, ErrWorkspacePathInvalid) {
		t.Fatalf("error = %v, want %v", err, ErrWorkspacePathInvalid)
	}
}

func TestExecWindowsPathConverterRejectsUnavailableExecutable(t *testing.T) {
	converter := &ExecWindowsPathConverter{Executable: "wslpath"}
	if converter.Available() {
		t.Fatal("relative executable must not be accepted")
	}
	if _, err := converter.ToWSL(context.Background(), `D:\private`); !errors.Is(err, ErrWindowsInteropUnavailable) {
		t.Fatalf("error = %v, want %v", err, ErrWindowsInteropUnavailable)
	}
}
