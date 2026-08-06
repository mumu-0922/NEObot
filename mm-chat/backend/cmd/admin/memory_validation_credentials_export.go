package main

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"

	"neo-chat/mm-chat/backend/internal/auth"
	"neo-chat/mm-chat/backend/internal/config"
	"neo-chat/mm-chat/backend/internal/database"
	"neo-chat/mm-chat/backend/internal/memorycapture"
	"neo-chat/mm-chat/backend/internal/providersecrets"
	"neo-chat/mm-chat/backend/internal/runtimeconfig"
)

const (
	memoryValidationCredentialExportApproval = "I_UNDERSTAND_THIS_EXPORTS_ACTIVE_MEMORY_VALIDATION_CREDENTIALS"
	memoryValidationCredentialExportSuccess  = "memory validation credentials exported\n"
)

var (
	errMemoryValidationCredentialExportNotAuthorized = errors.New(
		"MEMORY_VALIDATION_CREDENTIAL_EXPORT_NOT_AUTHORIZED",
	)
	errMemoryValidationCredentialAuthorityUnavailable = errors.New(
		"MEMORY_VALIDATION_CREDENTIAL_AUTHORITY_UNAVAILABLE",
	)
	errMemoryValidationCredentialOutputRejected = errors.New(
		"MEMORY_VALIDATION_CREDENTIAL_OUTPUT_REJECTED",
	)
	errMemoryValidationCredentialCleanupFailed = errors.New(
		"MEMORY_VALIDATION_CREDENTIAL_CLEANUP_FAILED",
	)
)

type memoryValidationCredentialExportOptions struct {
	bgeOutput  string
	lunaOutput string
}

type memoryValidationCredentialResolver interface {
	ResolveRAGProviderCredential(context.Context, string) (string, error)
	ResolveServerDefaultProvider(context.Context) (runtimeconfig.ResolvedProvider, error)
}

type createdCredentialFile struct {
	path string
	file *os.File
	info os.FileInfo
}

func runMemoryValidationCredentialsExport(args []string, stdout io.Writer) error {
	options, err := parseMemoryValidationCredentialExportArgs(args)
	if err != nil {
		return err
	}
	if err := validateMemoryValidationCredentialOutputs(options); err != nil {
		return err
	}

	cfg := config.Load()
	resolver, closeDatabase, err := openMemoryValidationCredentialResolver(cfg)
	if err != nil {
		return errMemoryValidationCredentialAuthorityUnavailable
	}
	defer closeDatabase()

	ownerContext := auth.WithUser(context.Background(), auth.User{
		ID:          cfg.Auth.BootstrapUserID,
		DisplayName: cfg.Auth.BootstrapDisplayName,
		Role:        "user",
	})
	ctx, cancel := context.WithTimeout(ownerContext, adminCommandTimeout)
	defer cancel()
	return exportMemoryValidationCredentials(ctx, resolver, options, stdout)
}

func parseMemoryValidationCredentialExportArgs(
	args []string,
) (memoryValidationCredentialExportOptions, error) {
	flags := flag.NewFlagSet("memory-validation-credentials-export", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var bgeOutput, lunaOutput, approval string
	flags.StringVar(&bgeOutput, "bge-output", "", "new fixed-BGE credential file")
	flags.StringVar(&lunaOutput, "luna-output", "", "new fixed-Luna credential file")
	flags.StringVar(&approval, "approval", "", "exact one-run export approval")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 ||
		flagCount(args, "bge-output") != 1 || flagCount(args, "luna-output") != 1 ||
		flagCount(args, "approval") != 1 {
		return memoryValidationCredentialExportOptions{}, usageError()
	}
	if approval != memoryValidationCredentialExportApproval {
		return memoryValidationCredentialExportOptions{}, errMemoryValidationCredentialExportNotAuthorized
	}
	options := memoryValidationCredentialExportOptions{
		bgeOutput: strings.TrimSpace(bgeOutput), lunaOutput: strings.TrimSpace(lunaOutput),
	}
	if options.bgeOutput == "" || options.lunaOutput == "" {
		return memoryValidationCredentialExportOptions{}, usageError()
	}
	return options, nil
}

func flagCount(args []string, name string) int {
	prefix := "--" + name
	count := 0
	for _, value := range args {
		if value == prefix || strings.HasPrefix(value, prefix+"=") {
			count++
		}
	}
	return count
}

func validateMemoryValidationCredentialOutputs(
	options memoryValidationCredentialExportOptions,
) error {
	if options.bgeOutput == options.lunaOutput {
		return errMemoryValidationCredentialOutputRejected
	}
	for _, path := range []string{options.bgeOutput, options.lunaOutput} {
		if !filepath.IsAbs(path) || filepath.Clean(path) != path {
			return errMemoryValidationCredentialOutputRejected
		}
		if _, err := os.Lstat(path); err == nil || !errors.Is(err, os.ErrNotExist) {
			return errMemoryValidationCredentialOutputRejected
		}
		parent := filepath.Dir(path)
		info, err := os.Lstat(parent)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 ||
			info.Mode().Perm()&0o077 != 0 {
			return errMemoryValidationCredentialOutputRejected
		}
		resolvedParent, err := filepath.EvalSymlinks(parent)
		if err != nil || filepath.Clean(resolvedParent) != parent {
			return errMemoryValidationCredentialOutputRejected
		}
	}
	return nil
}

func openMemoryValidationCredentialResolver(
	cfg config.Config,
) (memoryValidationCredentialResolver, func(), error) {
	if strings.TrimSpace(cfg.DatabaseURL) == "" ||
		strings.TrimSpace(cfg.ProviderSecrets.KeyringFile) == "" ||
		strings.TrimSpace(cfg.Auth.BootstrapUserID) == "" {
		return nil, func() {}, errMemoryValidationCredentialAuthorityUnavailable
	}
	vault, err := providersecrets.LoadVaultFile(cfg.ProviderSecrets.KeyringFile)
	if err != nil {
		return nil, func() {}, errMemoryValidationCredentialAuthorityUnavailable
	}
	ctx, cancel := context.WithTimeout(context.Background(), adminCommandTimeout)
	defer cancel()
	db, err := database.Open(ctx, cfg)
	if err != nil || db == nil || db.SQL() == nil {
		if db != nil {
			_ = db.Close()
		}
		return nil, func() {}, errMemoryValidationCredentialAuthorityUnavailable
	}
	service := runtimeconfig.NewService(
		cfg,
		runtimeconfig.WithProviderConfigRepository(
			runtimeconfig.NewPostgresProviderConfigRepository(db.SQL()),
		),
		runtimeconfig.WithProviderSecretVault(vault),
	)
	return service, func() { _ = db.Close() }, nil
}

func exportMemoryValidationCredentials(
	ctx context.Context,
	resolver memoryValidationCredentialResolver,
	options memoryValidationCredentialExportOptions,
	stdout io.Writer,
) error {
	if resolver == nil || stdout == nil {
		return errMemoryValidationCredentialAuthorityUnavailable
	}
	bgeCredential, err := resolver.ResolveRAGProviderCredential(ctx, "siliconflow")
	if err != nil || strings.TrimSpace(bgeCredential) == "" {
		return errMemoryValidationCredentialAuthorityUnavailable
	}
	bgeBytes := []byte(strings.TrimSpace(bgeCredential))
	bgeCredential = ""
	defer clear(bgeBytes)

	lunaProvider, err := resolver.ResolveServerDefaultProvider(ctx)
	if err != nil || !validMemoryValidationLunaAuthority(lunaProvider) {
		lunaProvider.APIKey = ""
		return errMemoryValidationCredentialAuthorityUnavailable
	}
	lunaBytes := []byte(strings.TrimSpace(lunaProvider.APIKey))
	lunaProvider.APIKey = ""
	defer clear(lunaBytes)
	if len(bgeBytes) == len(lunaBytes) &&
		subtle.ConstantTimeCompare(bgeBytes, lunaBytes) == 1 {
		return errMemoryValidationCredentialAuthorityUnavailable
	}

	created := make([]*createdCredentialFile, 0, 2)
	cleanup := func() error {
		failed := false
		for index := len(created) - 1; index >= 0; index-- {
			if err := wipeCreatedCredentialFile(created[index]); err != nil {
				failed = true
			}
		}
		if failed {
			return errMemoryValidationCredentialCleanupFailed
		}
		return nil
	}
	fail := func(result error) error {
		if cleanup() != nil {
			return errMemoryValidationCredentialCleanupFailed
		}
		return result
	}

	bgeFile, err := createCredentialFileExclusive(options.bgeOutput, bgeBytes)
	if bgeFile != nil {
		created = append(created, bgeFile)
	}
	if err != nil {
		return fail(errMemoryValidationCredentialOutputRejected)
	}
	lunaFile, err := createCredentialFileExclusive(options.lunaOutput, lunaBytes)
	if lunaFile != nil {
		created = append(created, lunaFile)
	}
	if err != nil {
		return fail(errMemoryValidationCredentialOutputRejected)
	}
	if os.SameFile(bgeFile.info, lunaFile.info) {
		return fail(errMemoryValidationCredentialOutputRejected)
	}
	for _, output := range created {
		if err := verifyAndCloseCreatedCredentialFile(output); err != nil {
			return fail(errMemoryValidationCredentialOutputRejected)
		}
	}
	if _, err := io.WriteString(stdout, memoryValidationCredentialExportSuccess); err != nil {
		return fail(errMemoryValidationCredentialOutputRejected)
	}
	return nil
}

func validMemoryValidationLunaAuthority(provider runtimeconfig.ResolvedProvider) bool {
	if strings.TrimSpace(provider.ID) != memorycapture.FixedMemoryJudgeProviderID ||
		provider.Type != runtimeconfig.ProviderTypeOpenAICompatible ||
		strings.TrimSpace(provider.APIKey) == "" {
		return false
	}
	baseURL := strings.TrimSuffix(strings.TrimSpace(provider.BaseURL), "#")
	baseURL = strings.TrimRight(baseURL, "/")
	digest := sha256.Sum256([]byte(baseURL))
	if hex.EncodeToString(digest[:]) != memorycapture.FixedMemoryJudgeBaseURLSHA256 {
		return false
	}
	for _, modelID := range provider.Models {
		if strings.TrimSpace(modelID) == memorycapture.FixedMemoryJudgeModelID {
			return true
		}
	}
	return false
}

func createCredentialFileExclusive(path string, credential []byte) (*createdCredentialFile, error) {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, err
	}
	created := &createdCredentialFile{path: path, file: file}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return created, errMemoryValidationCredentialOutputRejected
	}
	created.info = info
	if err := file.Chmod(0o600); err != nil {
		return created, err
	}
	if _, err := file.Write(credential); err != nil {
		return created, err
	}
	if err := file.Sync(); err != nil {
		return created, err
	}
	info, err = file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		return created, errMemoryValidationCredentialOutputRejected
	}
	created.info = info
	return created, nil
}

func verifyAndCloseCreatedCredentialFile(created *createdCredentialFile) error {
	if created == nil || created.file == nil || created.info == nil {
		return errMemoryValidationCredentialOutputRejected
	}
	pathInfo, err := os.Lstat(created.path)
	if err != nil || !pathInfo.Mode().IsRegular() || pathInfo.Mode()&os.ModeSymlink != 0 ||
		pathInfo.Mode().Perm() != 0o600 || !os.SameFile(created.info, pathInfo) {
		return errMemoryValidationCredentialOutputRejected
	}
	if err := created.file.Close(); err != nil {
		return errMemoryValidationCredentialOutputRejected
	}
	created.file = nil
	return nil
}

func wipeCreatedCredentialFile(created *createdCredentialFile) error {
	if created == nil {
		return nil
	}
	failed := false
	file := created.file
	if file == nil {
		opened, err := os.OpenFile(created.path, os.O_RDWR, 0)
		if err == nil {
			openedInfo, statErr := opened.Stat()
			if statErr == nil && created.info != nil && os.SameFile(created.info, openedInfo) {
				file = opened
			} else {
				_ = opened.Close()
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			failed = true
		}
	}
	if file != nil {
		currentInfo, statErr := file.Stat()
		if statErr != nil || !currentInfo.Mode().IsRegular() {
			failed = true
		}
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			failed = true
		} else {
			remaining := int64(0)
			if statErr == nil {
				remaining = currentInfo.Size()
			}
			zeroes := make([]byte, 64<<10)
			for remaining > 0 {
				chunk := int64(len(zeroes))
				if remaining < chunk {
					chunk = remaining
				}
				written, err := file.Write(zeroes[:chunk])
				if err != nil || int64(written) != chunk {
					failed = true
					break
				}
				remaining -= chunk
			}
			clear(zeroes)
		}
		if err := file.Sync(); err != nil {
			failed = true
		}
		if err := file.Truncate(0); err != nil {
			failed = true
		}
		if err := file.Close(); err != nil {
			failed = true
		}
		created.file = nil
	}
	pathInfo, err := os.Lstat(created.path)
	if err == nil {
		if created.info == nil || !os.SameFile(created.info, pathInfo) {
			failed = true
		} else if err := os.Remove(created.path); err != nil {
			failed = true
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		failed = true
	}
	if failed {
		return errMemoryValidationCredentialCleanupFailed
	}
	return nil
}
