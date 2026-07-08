package browserimport

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func TestPackageReaderValidatesChatOnlyImport(t *testing.T) {
	pkg, issues, err := PackageReader{Now: fixedNow}.Read(bytes.NewReader(testImportZip(t, validManifest())))
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if len(filterIssues(issues, "error")) != 0 {
		t.Fatalf("errors = %#v, want none", issues)
	}
	if pkg.Manifest.IdempotencyKey != "import-key-1" {
		t.Fatalf("idempotencyKey = %q", pkg.Manifest.IdempotencyKey)
	}
	if pkg.PackageHash == "" || pkg.ManifestHash == "" {
		t.Fatalf("hashes are blank: %#v", pkg)
	}
}

func TestPackageReaderValidatesFileImportPackage(t *testing.T) {
	blob := []byte("hello")
	manifest := validFileManifest()

	pkg, issues, err := PackageReader{Now: fixedNow}.Read(bytes.NewReader(testImportZip(
		t,
		manifest,
		zipEntry{name: manifest.Files[0].BlobPath, body: blob},
	)))
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if len(filterIssues(issues, "error")) != 0 {
		t.Fatalf("errors = %#v, want none", issues)
	}
	got := pkg.Blobs[manifest.Files[0].BlobPath]
	if got.SHA256 != manifest.Files[0].SHA256 || !bytes.Equal(got.Data, blob) {
		t.Fatalf("blob = %#v, want sha/data preserved", got)
	}
}

func TestPackageReaderRejectsUnsupportedFileAttachmentFields(t *testing.T) {
	manifest := validFileManifest()
	manifest.Files[0].Source = "http"
	manifest.Files[0].OriginalURL = "https://example.test/file.txt"
	manifest.Messages[0].Attachments[0].Purpose = "bad"

	_, issues, err := PackageReader{Now: fixedNow}.Read(bytes.NewReader(testImportZip(
		t,
		manifest,
		zipEntry{name: manifest.Files[0].BlobPath, body: []byte("hello")},
	)))
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	assertIssue(t, issues, "INVALID_IMPORT_PAYLOAD", "files[0].source")
	assertIssue(t, issues, "INVALID_IMPORT_PAYLOAD", "files[0].originalUrl")
	assertIssue(t, issues, "INVALID_IMPORT_PAYLOAD", "messages[0].attachments[0].purpose")
}

func TestPackageReaderRejectsDuplicateFileAttachmentOnSameMessage(t *testing.T) {
	manifest := validFileManifest()
	manifest.Messages[0].Attachments = append(manifest.Messages[0].Attachments, ImportAttachment{
		ClientAttachmentID: "attachment-client-2",
		Source:             "file",
		ClientFileID:       "file-client-1",
		FileName:           "hello-copy.txt",
		MimeType:           "text/plain",
		Size:               int64(len("hello")),
		SHA256:             manifest.Files[0].SHA256,
		Purpose:            "input",
	})

	_, issues, err := PackageReader{Now: fixedNow}.Read(bytes.NewReader(testImportZip(
		t,
		manifest,
		zipEntry{name: manifest.Files[0].BlobPath, body: []byte("hello")},
	)))
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	assertIssue(t, issues, "INVALID_IMPORT_PAYLOAD", "messages[0].attachments[1].clientFileId")
}

func TestPackageReaderRejectsSecretsInImportFileMetadata(t *testing.T) {
	manifest := validFileManifest()
	manifest.Files[0].OriginalURL = "opfs://neo-chat/files/access_token"

	_, issues, err := PackageReader{Now: fixedNow}.Read(bytes.NewReader(testImportZip(
		t,
		manifest,
		zipEntry{name: manifest.Files[0].BlobPath, body: []byte("hello")},
	)))
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	assertIssueCode(t, issues, "FORBIDDEN_SECRET_FIELD")
}

func TestPackageReaderRejectsDiagnosticStoreEntries(t *testing.T) {
	archive := testImportZip(t, validManifest(), zipEntry{name: "stores/chat.json", body: []byte(`{}`)})

	_, _, err := PackageReader{Now: fixedNow}.Read(bytes.NewReader(archive))
	if err == nil {
		t.Fatal("Read() error = nil, want invalid package")
	}
	assertValidationCode(t, err, "INVALID_IMPORT_PACKAGE")
}

func TestPackageReaderRejectsMessageTreeAndSecretFields(t *testing.T) {
	manifest := validManifest()
	manifest.Conversations[0].Config = map[string]any{"apiKey": "secret"}
	manifest.Messages[1].SequenceNo = 3

	_, issues, err := PackageReader{Now: fixedNow}.Read(bytes.NewReader(testImportZip(t, manifest)))
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	assertIssueCode(t, issues, "FORBIDDEN_SECRET_FIELD")
	assertIssueCode(t, issues, "INVALID_MESSAGE_TREE")
}

func TestPackageReaderRejectsTopLevelInvalidTimestamps(t *testing.T) {
	tests := []struct {
		name     string
		mutate   func(*Manifest)
		wantPath string
	}{
		{
			name:     "missing generatedAt",
			mutate:   func(m *Manifest) { m.GeneratedAt = "" },
			wantPath: "generatedAt",
		},
		{
			name:     "non UTC generatedAt",
			mutate:   func(m *Manifest) { m.GeneratedAt = "2026-07-07T00:00:00+08:00" },
			wantPath: "generatedAt",
		},
		{
			name:     "future generatedAt",
			mutate:   func(m *Manifest) { m.GeneratedAt = "2026-07-09T00:00:00Z" },
			wantPath: "generatedAt",
		},
		{
			name:     "invalid exportedAt",
			mutate:   func(m *Manifest) { m.ExportedAt = "bad" },
			wantPath: "exportedAt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manifest := validManifest()
			tt.mutate(&manifest)
			_, issues, err := PackageReader{Now: fixedNow}.Read(bytes.NewReader(testImportZip(t, manifest)))
			if err != nil {
				t.Fatalf("Read() error = %v", err)
			}
			assertIssue(t, issues, "INVALID_IMPORT_PAYLOAD", tt.wantPath)
		})
	}
}

func TestPackageReaderRejectsSecretsInPersistedImportFields(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Manifest)
	}{
		{
			name: "outputBlocks",
			mutate: func(m *Manifest) {
				m.Messages[1].OutputBlocks = []any{map[string]any{"apiKey": "secret"}}
			},
		},
		{
			name: "remote attachment url token",
			mutate: func(m *Manifest) {
				m.Messages[0].Attachments = []ImportAttachment{{
					ClientAttachmentID: "attachment-client-1",
					Source:             "remote",
					FileName:           "remote.txt",
					MimeType:           "text/plain",
					URL:                "https://example.test/file.txt?access_token=secret",
				}}
			},
		},
		{
			name: "remote attachment userinfo secret",
			mutate: func(m *Manifest) {
				m.Messages[0].Attachments = []ImportAttachment{{
					ClientAttachmentID: "attachment-client-1",
					Source:             "remote",
					FileName:           "remote.txt",
					MimeType:           "text/plain",
					URL:                "https://user:password@example.test/file.txt",
				}}
			},
		},
		{
			name: "remote attachment fragment token",
			mutate: func(m *Manifest) {
				m.Messages[0].Attachments = []ImportAttachment{{
					ClientAttachmentID: "attachment-client-1",
					Source:             "remote",
					FileName:           "remote.txt",
					MimeType:           "text/plain",
					URL:                "https://example.test/file.txt#access_token=secret",
				}}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manifest := validManifest()
			tt.mutate(&manifest)
			_, issues, err := PackageReader{Now: fixedNow}.Read(bytes.NewReader(testImportZip(t, manifest)))
			if err != nil {
				t.Fatalf("Read() error = %v", err)
			}
			assertIssueCode(t, issues, "FORBIDDEN_SECRET_FIELD")
		})
	}
}

func TestPackageReaderValidatesFileBlobHash(t *testing.T) {
	blob := []byte("hello")
	manifest := validManifest()
	manifest.Counts.Files = 1
	manifest.Counts.Bytes = int64(len(blob))
	manifest.Files = []ImportFile{{
		ClientFileID:        "file-client-1",
		Source:              "opfs",
		SourceAttachmentIDs: []string{"attachment-client-1"},
		FileName:            "hello.txt",
		MimeType:            "text/plain",
		Size:                int64(len(blob)),
		SHA256:              "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824",
		BlobPath:            "files/sha256/2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824",
		Purpose:             "chat",
	}}

	_, issues, err := PackageReader{Now: fixedNow}.Read(bytes.NewReader(testImportZip(t, manifest, zipEntry{name: manifest.Files[0].BlobPath, body: []byte("bad")})))
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	assertIssueCode(t, issues, "FILE_HASH_MISMATCH")
}

func TestPackageReaderRejectsOrphanBlob(t *testing.T) {
	hash := "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	_, issues, err := PackageReader{Now: fixedNow}.Read(bytes.NewReader(testImportZip(
		t,
		validManifest(),
		zipEntry{name: "files/sha256/" + hash, body: []byte("hello")},
	)))
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	assertIssueCode(t, issues, "INVALID_IMPORT_PACKAGE")
}

func TestPackageReaderRejectsSymlinkEntry(t *testing.T) {
	archive := testCustomZip(t, func(writer *zip.Writer) {
		header := &zip.FileHeader{Name: manifestPath}
		header.SetMode(os.ModeSymlink | 0o644)
		entry, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatalf("create symlink entry: %v", err)
		}
		if _, err := entry.Write([]byte("target")); err != nil {
			t.Fatalf("write symlink entry: %v", err)
		}
	})
	_, _, err := PackageReader{Now: fixedNow}.Read(bytes.NewReader(archive))
	if err == nil {
		t.Fatal("Read() error = nil, want invalid package")
	}
	assertValidationCode(t, err, "INVALID_IMPORT_PACKAGE")
}

func fixedNow() time.Time {
	return time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC)
}

func validManifest() Manifest {
	return Manifest{
		Format:         FormatName,
		SchemaVersion:  SchemaVersion,
		StorageVersion: 4,
		GeneratedAt:    "2026-07-07T00:00:00Z",
		IdempotencyKey: "import-key-1",
		Source:         ImportSource{App: "neo-chat", Origin: "http://localhost:3000"},
		Counts:         ImportCounts{Conversations: 1, Messages: 2, Files: 0, Bytes: 0},
		OPFS:           OPFSSummary{},
		Conversations: []ImportConversation{{
			ClientID:  "conversation-client-1",
			Title:     "Imported",
			UpdatedAt: "2026-07-07T00:00:00Z",
		}},
		Messages: []ImportMessage{
			{
				ClientID:             "message-client-1",
				ConversationClientID: "conversation-client-1",
				SequenceNo:           0,
				Role:                 "user",
				Status:               "completed",
				Content:              "hello",
				CreatedAt:            "2026-07-07T00:00:01Z",
			},
			{
				ClientID:             "message-client-2",
				ConversationClientID: "conversation-client-1",
				ParentClientID:       "message-client-1",
				SequenceNo:           1,
				Role:                 "assistant",
				Status:               "completed",
				Content:              "hi",
				CreatedAt:            "2026-07-07T00:00:02Z",
				CompletedAt:          "2026-07-07T00:00:03Z",
			},
		},
		Files: []ImportFile{},
	}
}

func validFileManifest() Manifest {
	manifest := validManifest()
	sha := "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	manifest.Counts.Files = 1
	manifest.Counts.Bytes = int64(len("hello"))
	manifest.Files = []ImportFile{{
		ClientFileID:        "file-client-1",
		Source:              "opfs",
		OriginalURL:         "opfs://neo-chat/files/file-client-1",
		SourceAttachmentIDs: []string{"attachment-client-1"},
		FileName:            "hello.txt",
		MimeType:            "text/plain",
		Size:                int64(len("hello")),
		SHA256:              sha,
		BlobPath:            "files/sha256/" + sha,
		Purpose:             "chat",
	}}
	manifest.Messages[0].Attachments = []ImportAttachment{{
		ClientAttachmentID: "attachment-client-1",
		Source:             "file",
		ClientFileID:       "file-client-1",
		FileName:           "hello.txt",
		MimeType:           "text/plain",
		Size:               int64(len("hello")),
		SHA256:             sha,
		Purpose:            "input",
	}}
	return manifest
}

type zipEntry struct {
	name string
	body []byte
}

func testImportZip(t *testing.T, manifest Manifest, extra ...zipEntry) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	manifestWriter, err := writer.Create(manifestPath)
	if err != nil {
		t.Fatalf("create manifest entry: %v", err)
	}
	if err := json.NewEncoder(manifestWriter).Encode(manifest); err != nil {
		t.Fatalf("encode manifest: %v", err)
	}
	for _, entry := range extra {
		entryWriter, err := writer.Create(entry.name)
		if err != nil {
			t.Fatalf("create entry %s: %v", entry.name, err)
		}
		if _, err := entryWriter.Write(entry.body); err != nil {
			t.Fatalf("write entry %s: %v", entry.name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buffer.Bytes()
}

func testCustomZip(t *testing.T, build func(*zip.Writer)) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	build(writer)
	if err := writer.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buffer.Bytes()
}

func assertValidationCode(t *testing.T, err error, code string) {
	t.Helper()
	validationError, ok := err.(ValidationError)
	if !ok {
		t.Fatalf("error type = %T, want ValidationError", err)
	}
	if validationError.Code != code {
		t.Fatalf("code = %q, want %q", validationError.Code, code)
	}
}

func assertIssueCode(t *testing.T, issues []Issue, code string) {
	t.Helper()
	for _, issue := range issues {
		if issue.Code == code {
			return
		}
	}
	parts := make([]string, 0, len(issues))
	for _, issue := range issues {
		parts = append(parts, issue.Code+":"+issue.Path)
	}
	t.Fatalf("issues missing code %s; got %s", code, strings.Join(parts, ", "))
}

func assertIssue(t *testing.T, issues []Issue, code string, path string) {
	t.Helper()
	for _, issue := range issues {
		if issue.Code == code && issue.Path == path {
			return
		}
	}
	t.Fatalf("issues missing %s at %s; got %#v", code, path, issues)
}
