package usermemory

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"neo-chat/mm-chat/backend/internal/auth"
)

const portabilityHandlerTestUserID = "00000000-0000-4000-8000-000000000001"

type portabilityMultipartPart struct {
	name     string
	filename string
	value    []byte
}

func TestMemoryExportHandlerStrictJSONHeadersAndTemporaryCleanup(t *testing.T) {
	temporaryRoot := t.TempDir()
	t.Setenv("TMPDIR", temporaryRoot)
	repository := &portabilityExportTestRepository{
		fakeRepository: &fakeRepository{},
		records: []any{
			PortableSettingsRecord{Kind: "settings", Settings: DefaultSettings()},
			validPortableMemory(1),
		},
	}
	handler := NewHandler(NewService(repository, WithPortabilityRelease("handler-test")))

	response := serveAuthenticatedPortabilityRequest(
		t,
		handler,
		httptest.NewRequest(
			http.MethodPost,
			memoryExportPath,
			strings.NewReader(`{"passphrase":"fixture-passphrase","includeHistory":false}`),
		),
	)
	if response.Code != http.StatusOK {
		t.Fatalf("export status = %d body=%s", response.Code, response.Body.String())
	}
	if response.Header().Get("Content-Type") != "application/octet-stream" ||
		response.Header().Get("Cache-Control") != "no-store" ||
		response.Header().Get("X-Content-Type-Options") != "nosniff" ||
		response.Header().Get("Content-Length") == "" {
		t.Fatalf("export headers = %#v", response.Header())
	}
	disposition := response.Header().Get("Content-Disposition")
	if !strings.HasPrefix(disposition, `attachment; filename="memory-`) ||
		!strings.HasSuffix(disposition, `.mm-memory"`) {
		t.Fatalf("Content-Disposition = %q", disposition)
	}
	parsed, err := ParseEncryptedMemoryPackage(
		bytes.NewReader(response.Body.Bytes()),
		"fixture-passphrase",
		MemoryPackageVisitorFuncs{},
	)
	if err != nil || parsed.Manifest.ExporterRelease != "handler-test" {
		t.Fatalf("downloaded export parsed=%#v error=%v", parsed, err)
	}
	assertDirectoryEmpty(t, temporaryRoot)

	for name, body := range map[string]string{
		"duplicate field": `{"passphrase":"fixture-passphrase","passphrase":"replacement-passphrase","includeHistory":false}`,
		"unknown field":   `{"passphrase":"fixture-passphrase","includeHistory":false,"secret":true}`,
		"oversize body":   `{"passphrase":"` + strings.Repeat("x", maxRequestBytes) + `","includeHistory":false}`,
	} {
		t.Run(name, func(t *testing.T) {
			response := serveAuthenticatedPortabilityRequest(
				t,
				handler,
				httptest.NewRequest(http.MethodPost, memoryExportPath, strings.NewReader(body)),
			)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
			}
		})
	}
	assertDirectoryEmpty(t, temporaryRoot)
}

func TestMemoryImportHandlerRejectsMultipartBoundaryViolationsAndCleansTemporaryFile(t *testing.T) {
	temporaryRoot := t.TempDir()
	t.Setenv("TMPDIR", temporaryRoot)
	handler := NewHandler(newPortabilityHandlerTestService(t, &portabilityTestRepository{
		fakeRepository: &fakeRepository{},
		state:          strings.Repeat("a", 64),
		resolution:     ImportMemoryResolution{Result: "ADD", ReasonCode: "NEW_MEMORY"},
	}))
	encrypted := encryptedTestMemoryPackage(t, validPortableMemory(1), false)
	mappings := []byte(`{"projects":{"project-000001":{"mode":"skip"}},"conversations":{}}`)

	tests := []struct {
		name  string
		parts []portabilityMultipartPart
	}{
		{
			name: "duplicate field",
			parts: []portabilityMultipartPart{
				{name: "package", filename: "memory.mm-memory", value: encrypted},
				{name: "passphrase", value: []byte("fixture-passphrase")},
				{name: "passphrase", value: []byte("replacement-passphrase")},
				{name: "mappings", value: mappings},
			},
		},
		{
			name: "unknown field after package",
			parts: []portabilityMultipartPart{
				{name: "package", filename: "memory.mm-memory", value: encrypted},
				{name: "passphrase", value: []byte("fixture-passphrase")},
				{name: "mappings", value: mappings},
				{name: "plaintext", value: []byte("must-not-be-accepted")},
			},
		},
		{
			name: "oversize bounded field",
			parts: []portabilityMultipartPart{
				{name: "package", filename: "memory.mm-memory", value: encrypted},
				{name: "passphrase", value: bytes.Repeat([]byte("p"), MaxPortabilityPassphraseBytes+1)},
				{name: "mappings", value: mappings},
			},
		},
		{
			name: "duplicate nested mapping key",
			parts: []portabilityMultipartPart{
				{name: "package", filename: "memory.mm-memory", value: encrypted},
				{name: "passphrase", value: []byte("fixture-passphrase")},
				{name: "mappings", value: []byte(`{"projects":{},"projects":{},"conversations":{}}`)},
			},
		},
		{
			name: "unknown mapping field",
			parts: []portabilityMultipartPart{
				{name: "package", filename: "memory.mm-memory", value: encrypted},
				{name: "passphrase", value: []byte("fixture-passphrase")},
				{name: "mappings", value: []byte(`{"projects":{},"conversations":{},"authority":"external"}`)},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := newPortabilityMultipartRequest(t, memoryImportDryRunPath, test.parts)
			response := serveAuthenticatedPortabilityRequest(t, handler, request)
			if response.Code != http.StatusBadRequest ||
				!strings.Contains(response.Body.String(), "MEMORY_IMPORT_MULTIPART_INVALID") {
				t.Fatalf("response = %d %s", response.Code, response.Body.String())
			}
			assertDirectoryEmpty(t, temporaryRoot)
		})
	}
}

func TestMemoryImportHandlerAuthenticationAndConfirmConflictResponses(t *testing.T) {
	temporaryRoot := t.TempDir()
	t.Setenv("TMPDIR", temporaryRoot)
	repository := &portabilityTestRepository{
		fakeRepository: &fakeRepository{},
		state:          strings.Repeat("b", 64),
		resolution:     ImportMemoryResolution{Result: "ADD", ReasonCode: "NEW_MEMORY"},
	}
	handler := NewHandler(newPortabilityHandlerTestService(t, repository))
	encrypted := encryptedTestMemoryPackage(t, validPortableMemory(1), false)
	mappings := []byte(`{"projects":{"project-000001":{"mode":"skip"}},"conversations":{}}`)

	for name, test := range map[string]struct {
		packageBytes []byte
		passphrase   string
		wantCode     string
	}{
		"wrong passphrase": {
			packageBytes: encrypted,
			passphrase:   "incorrect-passphrase",
			wantCode:     "MEMORY_PORTABILITY_DECRYPT_FAILED",
		},
		"tampered ciphertext": {
			packageBytes: tamperLastByte(encrypted),
			passphrase:   "fixture-passphrase",
			wantCode:     "MEMORY_PACKAGE_AUTHENTICATION_FAILED",
		},
	} {
		t.Run(name, func(t *testing.T) {
			request := newPortabilityMultipartRequest(t, memoryImportDryRunPath, []portabilityMultipartPart{
				{name: "package", filename: "memory.mm-memory", value: test.packageBytes},
				{name: "passphrase", value: []byte(test.passphrase)},
				{name: "mappings", value: mappings},
			})
			response := serveAuthenticatedPortabilityRequest(t, handler, request)
			assertPortabilityError(t, response, http.StatusBadRequest, test.wantCode)
			assertDirectoryEmpty(t, temporaryRoot)
		})
	}

	dryRunRequest := newPortabilityMultipartRequest(t, memoryImportDryRunPath, []portabilityMultipartPart{
		{name: "package", filename: "memory.mm-memory", value: encrypted},
		{name: "passphrase", value: []byte("fixture-passphrase")},
		{name: "mappings", value: mappings},
	})
	dryRunResponse := serveAuthenticatedPortabilityRequest(t, handler, dryRunRequest)
	if dryRunResponse.Code != http.StatusOK {
		t.Fatalf("dry-run response = %d %s", dryRunResponse.Code, dryRunResponse.Body.String())
	}
	var dryRun ImportDryRunResult
	if err := json.NewDecoder(dryRunResponse.Body).Decode(&dryRun); err != nil {
		t.Fatalf("decode dry-run: %v", err)
	}
	if dryRun.PlanToken == "" {
		t.Fatal("dry-run omitted plan token")
	}

	for name, test := range map[string]struct {
		packageBytes []byte
		token        string
		wantCode     string
	}{
		"tampered token": {
			packageBytes: encrypted,
			token:        tamperLastCharacter(dryRun.PlanToken),
			wantCode:     "MEMORY_IMPORT_PLAN_TOKEN_INVALID",
		},
		"package drift": {
			packageBytes: tamperLastByte(encrypted),
			token:        dryRun.PlanToken,
			wantCode:     "MEMORY_IMPORT_PLAN_STALE",
		},
	} {
		t.Run(name, func(t *testing.T) {
			request := newPortabilityMultipartRequest(t, memoryImportConfirmPath, []portabilityMultipartPart{
				{name: "package", filename: "memory.mm-memory", value: test.packageBytes},
				{name: "passphrase", value: []byte("fixture-passphrase")},
				{name: "mappings", value: mappings},
				{name: "planToken", value: []byte(test.token)},
			})
			response := serveAuthenticatedPortabilityRequest(t, handler, request)
			assertPortabilityError(t, response, http.StatusConflict, test.wantCode)
			assertDirectoryEmpty(t, temporaryRoot)
		})
	}

	repository.state = strings.Repeat("c", 64)
	stateDriftRequest := newPortabilityMultipartRequest(t, memoryImportConfirmPath, []portabilityMultipartPart{
		{name: "package", filename: "memory.mm-memory", value: encrypted},
		{name: "passphrase", value: []byte("fixture-passphrase")},
		{name: "mappings", value: mappings},
		{name: "planToken", value: []byte(dryRun.PlanToken)},
	})
	stateDriftResponse := serveAuthenticatedPortabilityRequest(t, handler, stateDriftRequest)
	assertPortabilityError(t, stateDriftResponse, http.StatusConflict, "MEMORY_IMPORT_PLAN_STALE")
	assertDirectoryEmpty(t, temporaryRoot)
}

func newPortabilityHandlerTestService(t *testing.T, repository *portabilityTestRepository) *Service {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("rand.Read() error = %v", err)
	}
	codec, err := NewPortabilityPlanCodec(PortabilityPlanKeyring{
		ActiveKeyID: "handler-test",
		Keys:        map[string][]byte{"handler-test": key},
	})
	if err != nil {
		t.Fatalf("NewPortabilityPlanCodec() error = %v", err)
	}
	return NewService(repository, WithPortabilityPlanCodec(codec))
}

func newPortabilityMultipartRequest(
	t *testing.T,
	path string,
	parts []portabilityMultipartPart,
) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for _, part := range parts {
		var destination interface{ Write([]byte) (int, error) }
		var err error
		if part.filename != "" {
			destination, err = writer.CreateFormFile(part.name, part.filename)
		} else {
			destination, err = writer.CreateFormField(part.name)
		}
		if err != nil {
			t.Fatalf("create multipart field %q: %v", part.name, err)
		}
		if _, err := destination.Write(part.value); err != nil {
			t.Fatalf("write multipart field %q: %v", part.name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, path, &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	return request
}

func serveAuthenticatedPortabilityRequest(
	t *testing.T,
	handler http.Handler,
	request *http.Request,
) *httptest.ResponseRecorder {
	t.Helper()
	request = request.WithContext(auth.WithUser(request.Context(), auth.User{
		ID: portabilityHandlerTestUserID,
	}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func assertPortabilityError(
	t *testing.T,
	response *httptest.ResponseRecorder,
	status int,
	code string,
) {
	t.Helper()
	var payload ErrorResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("decode error response: %v body=%s", err, response.Body.String())
	}
	if response.Code != status || payload.Error.Code != code {
		t.Fatalf("response = %d/%s, want %d/%s", response.Code, payload.Error.Code, status, code)
	}
}

func assertDirectoryEmpty(t *testing.T, directory string) {
	t.Helper()
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("read temporary directory: %v", err)
	}
	if len(entries) != 0 {
		paths := make([]string, 0, len(entries))
		for _, entry := range entries {
			paths = append(paths, filepath.Join(directory, entry.Name()))
		}
		t.Fatalf("temporary files were not removed: %v", paths)
	}
}

func tamperLastByte(value []byte) []byte {
	tampered := append([]byte(nil), value...)
	tampered[len(tampered)-1] ^= 1
	return tampered
}

func tamperLastCharacter(value string) string {
	if strings.HasSuffix(value, "A") {
		return value[:len(value)-1] + "B"
	}
	return value[:len(value)-1] + "A"
}
