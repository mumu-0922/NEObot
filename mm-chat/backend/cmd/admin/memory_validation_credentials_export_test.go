package main

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"neo-chat/mm-chat/backend/internal/memorycapture"
	"neo-chat/mm-chat/backend/internal/runtimeconfig"
)

const (
	testMemoryValidationBGESecret  = "test-only-bge-credential"
	testMemoryValidationLunaSecret = "test-only-luna-credential"
)

type fakeMemoryValidationCredentialResolver struct {
	bge       string
	bgeErr    error
	luna      runtimeconfig.ResolvedProvider
	lunaErr   error
	ragInputs []string
	lunaCalls int
}

func (resolver *fakeMemoryValidationCredentialResolver) ResolveRAGProviderCredential(
	_ context.Context,
	providerID string,
) (string, error) {
	resolver.ragInputs = append(resolver.ragInputs, providerID)
	return resolver.bge, resolver.bgeErr
}

func (resolver *fakeMemoryValidationCredentialResolver) ResolveServerDefaultProvider(
	context.Context,
) (runtimeconfig.ResolvedProvider, error) {
	resolver.lunaCalls++
	return resolver.luna, resolver.lunaErr
}

type failingMemoryValidationOutput struct{}

func (failingMemoryValidationOutput) Write([]byte) (int, error) {
	return 0, io.ErrClosedPipe
}

func TestParseMemoryValidationCredentialExportArgsRequiresExactOneRunApproval(t *testing.T) {
	bgePath := filepath.Join(t.TempDir(), "bge.key")
	lunaPath := filepath.Join(t.TempDir(), "luna.key")
	valid := []string{
		"--bge-output", bgePath,
		"--luna-output", lunaPath,
		"--approval", memoryValidationCredentialExportApproval,
	}
	options, err := parseMemoryValidationCredentialExportArgs(valid)
	if err != nil || options.bgeOutput != bgePath || options.lunaOutput != lunaPath {
		t.Fatalf("parse options = %#v, %v", options, err)
	}

	for name, args := range map[string][]string{
		"missing approval": {
			"--bge-output", bgePath, "--luna-output", lunaPath,
		},
		"wrong approval": {
			"--bge-output", bgePath, "--luna-output", lunaPath,
			"--approval", "approved",
		},
		"arbitrary provider": append(append([]string{}, valid...), "--provider-id", "CUSTOM"),
		"duplicate output":   append(append([]string{}, valid...), "--bge-output", bgePath+".other"),
		"extra argument":     append(append([]string{}, valid...), "extra"),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseMemoryValidationCredentialExportArgs(args); err == nil {
				t.Fatal("parse error = nil")
			}
		})
	}
}

func TestValidateMemoryValidationCredentialOutputsRejectsUnsafeTargets(t *testing.T) {
	private := t.TempDir()
	if err := os.Chmod(private, 0o700); err != nil {
		t.Fatal(err)
	}
	bgePath := filepath.Join(private, "bge.key")
	lunaPath := filepath.Join(private, "luna.key")
	if err := validateMemoryValidationCredentialOutputs(memoryValidationCredentialExportOptions{
		bgeOutput: bgePath, lunaOutput: lunaPath,
	}); err != nil {
		t.Fatalf("valid targets error = %v", err)
	}

	existing := filepath.Join(private, "existing.key")
	if err := os.WriteFile(existing, []byte("preserve"), 0o600); err != nil {
		t.Fatal(err)
	}
	symlink := filepath.Join(private, "symlink.key")
	if err := os.Symlink(existing, symlink); err != nil {
		t.Fatal(err)
	}
	publicParent := filepath.Join(private, "public")
	if err := os.Mkdir(publicParent, 0o755); err != nil {
		t.Fatal(err)
	}
	symlinkParent := filepath.Join(private, "linked")
	if err := os.Symlink(private, symlinkParent); err != nil {
		t.Fatal(err)
	}

	for name, options := range map[string]memoryValidationCredentialExportOptions{
		"same":       {bgeOutput: bgePath, lunaOutput: bgePath},
		"relative":   {bgeOutput: "bge.key", lunaOutput: lunaPath},
		"unclean":    {bgeOutput: private + "/nested/../bge.key", lunaOutput: lunaPath},
		"existing":   {bgeOutput: existing, lunaOutput: lunaPath},
		"symlink":    {bgeOutput: symlink, lunaOutput: lunaPath},
		"public dir": {bgeOutput: filepath.Join(publicParent, "bge.key"), lunaOutput: lunaPath},
		"linked dir": {bgeOutput: filepath.Join(symlinkParent, "bge.key"), lunaOutput: lunaPath},
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateMemoryValidationCredentialOutputs(options); !errors.Is(
				err, errMemoryValidationCredentialOutputRejected,
			) {
				t.Fatalf("validation error = %v", err)
			}
		})
	}
}

func TestExportMemoryValidationCredentialsCreatesExactPrivatePair(t *testing.T) {
	root := t.TempDir()
	options := memoryValidationCredentialExportOptions{
		bgeOutput:  filepath.Join(root, "bge.key"),
		lunaOutput: filepath.Join(root, "luna.key"),
	}
	resolver := validMemoryValidationCredentialResolver()
	var stdout strings.Builder
	if err := exportMemoryValidationCredentials(
		context.Background(), resolver, options, &stdout,
	); err != nil {
		t.Fatalf("export error = %v", err)
	}
	if stdout.String() != memoryValidationCredentialExportSuccess ||
		strings.Contains(stdout.String(), testMemoryValidationBGESecret) ||
		strings.Contains(stdout.String(), testMemoryValidationLunaSecret) {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if len(resolver.ragInputs) != 1 || resolver.ragInputs[0] != "siliconflow" ||
		resolver.lunaCalls != 1 {
		t.Fatalf("resolver calls = %#v/%d", resolver.ragInputs, resolver.lunaCalls)
	}
	for path, want := range map[string]string{
		options.bgeOutput:  testMemoryValidationBGESecret,
		options.lunaOutput: testMemoryValidationLunaSecret,
	} {
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 ||
			info.Mode().Perm() != 0o600 {
			t.Fatalf("output %s info = %#v, %v", path, info, err)
		}
		body, err := os.ReadFile(path)
		if err != nil || string(body) != want {
			t.Fatalf("output %s = %q, %v", path, body, err)
		}
	}
}

func TestExportMemoryValidationCredentialsRejectsAuthorityDriftWithoutFiles(t *testing.T) {
	valid := validMemoryValidationCredentialResolver()
	for name, mutate := range map[string]func(*fakeMemoryValidationCredentialResolver){
		"BGE unavailable": func(value *fakeMemoryValidationCredentialResolver) {
			value.bgeErr = errors.New("vault unavailable: " + testMemoryValidationBGESecret)
		},
		"BGE empty": func(value *fakeMemoryValidationCredentialResolver) {
			value.bge = ""
		},
		"Luna unavailable": func(value *fakeMemoryValidationCredentialResolver) {
			value.lunaErr = errors.New("database contains " + testMemoryValidationLunaSecret)
		},
		"Luna ID": func(value *fakeMemoryValidationCredentialResolver) {
			value.luna.ID = "CUSTOM"
		},
		"Luna type": func(value *fakeMemoryValidationCredentialResolver) {
			value.luna.Type = runtimeconfig.ProviderTypeOpenAI
		},
		"Luna base URL": func(value *fakeMemoryValidationCredentialResolver) {
			value.luna.BaseURL = "https://other.example/v1"
		},
		"Luna model": func(value *fakeMemoryValidationCredentialResolver) {
			value.luna.Models = []string{"gpt-other"}
		},
		"equal credentials": func(value *fakeMemoryValidationCredentialResolver) {
			value.luna.APIKey = value.bge
		},
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			resolver := *valid
			resolver.ragInputs = nil
			resolver.lunaCalls = 0
			resolver.luna.Models = append([]string(nil), valid.luna.Models...)
			mutate(&resolver)
			options := memoryValidationCredentialExportOptions{
				bgeOutput:  filepath.Join(root, "bge.key"),
				lunaOutput: filepath.Join(root, "luna.key"),
			}
			var stdout strings.Builder
			err := exportMemoryValidationCredentials(
				context.Background(), &resolver, options, &stdout,
			)
			if !errors.Is(err, errMemoryValidationCredentialAuthorityUnavailable) {
				t.Fatalf("export error = %v", err)
			}
			if stdout.Len() != 0 || strings.Contains(err.Error(), testMemoryValidationBGESecret) ||
				strings.Contains(err.Error(), testMemoryValidationLunaSecret) {
				t.Fatalf("secret-bearing result = %q / %v", stdout.String(), err)
			}
			for _, path := range []string{options.bgeOutput, options.lunaOutput} {
				if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("unexpected output %s: %v", path, err)
				}
			}
		})
	}
}

func TestExportMemoryValidationCredentialsCleansPartialAndOutputFailure(t *testing.T) {
	t.Run("second output exists", func(t *testing.T) {
		root := t.TempDir()
		bgePath := filepath.Join(root, "bge.key")
		lunaPath := filepath.Join(root, "luna.key")
		if err := os.WriteFile(lunaPath, []byte("preserve"), 0o600); err != nil {
			t.Fatal(err)
		}
		err := exportMemoryValidationCredentials(
			context.Background(), validMemoryValidationCredentialResolver(),
			memoryValidationCredentialExportOptions{bgeOutput: bgePath, lunaOutput: lunaPath},
			io.Discard,
		)
		if !errors.Is(err, errMemoryValidationCredentialOutputRejected) {
			t.Fatalf("export error = %v", err)
		}
		if _, err := os.Lstat(bgePath); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("partial BGE output remains: %v", err)
		}
		body, err := os.ReadFile(lunaPath)
		if err != nil || string(body) != "preserve" {
			t.Fatalf("existing Luna output = %q, %v", body, err)
		}
	})

	t.Run("stdout failure", func(t *testing.T) {
		root := t.TempDir()
		options := memoryValidationCredentialExportOptions{
			bgeOutput:  filepath.Join(root, "bge.key"),
			lunaOutput: filepath.Join(root, "luna.key"),
		}
		err := exportMemoryValidationCredentials(
			context.Background(), validMemoryValidationCredentialResolver(),
			options, failingMemoryValidationOutput{},
		)
		if !errors.Is(err, errMemoryValidationCredentialOutputRejected) {
			t.Fatalf("export error = %v", err)
		}
		for _, path := range []string{options.bgeOutput, options.lunaOutput} {
			if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("output remains after stdout failure %s: %v", path, err)
			}
		}
	})
}

func validMemoryValidationCredentialResolver() *fakeMemoryValidationCredentialResolver {
	return &fakeMemoryValidationCredentialResolver{
		bge: testMemoryValidationBGESecret,
		luna: runtimeconfig.ResolvedProvider{
			ID:      memorycapture.FixedMemoryJudgeProviderID,
			Type:    runtimeconfig.ProviderTypeOpenAICompatible,
			BaseURL: memorycapture.FixedMemoryJudgeBaseURL + "/#",
			APIKey:  testMemoryValidationLunaSecret,
			Models:  []string{"other", memorycapture.FixedMemoryJudgeModelID},
		},
	}
}
