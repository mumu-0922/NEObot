package browserimport

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	manifestPath           = "manifest.json"
	blobPathPrefix         = "files/sha256/"
	defaultMaxPackageBytes = int64(50 << 20)
	maxManifestBytes       = int64(10 << 20)
)

var sha256HexPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

type PackageReader struct {
	MaxPackageBytes int64
	Now             func() time.Time
}

type blobRecord struct {
	Size   int64
	SHA256 string
}

func (r PackageReader) Read(reader io.Reader) (Package, []Issue, error) {
	maxBytes := r.MaxPackageBytes
	if maxBytes <= 0 {
		maxBytes = defaultMaxPackageBytes
	}
	payload, err := readLimited(reader, maxBytes)
	if err != nil {
		return Package{}, nil, err
	}
	packageSum := sha256.Sum256(payload)

	zipReader, err := zip.NewReader(bytes.NewReader(payload), int64(len(payload)))
	if err != nil {
		return Package{}, nil, newValidationError("INVALID_IMPORT_PACKAGE", "import package must be a valid ZIP archive")
	}

	manifestBytes, blobs, err := readZipEntries(zipReader, maxBytes)
	if err != nil {
		return Package{}, nil, err
	}
	if len(manifestBytes) == 0 {
		return Package{}, nil, newValidationError("INVALID_IMPORT_PACKAGE", "import package must contain manifest.json")
	}
	manifestSum := sha256.Sum256(manifestBytes)

	var manifest Manifest
	decoder := json.NewDecoder(bytes.NewReader(manifestBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return Package{}, nil, newValidationError("INVALID_IMPORT_PAYLOAD", "manifest.json is invalid")
	}
	var trailing struct{}
	if err := decoder.Decode(&trailing); err != io.EOF {
		return Package{}, nil, newValidationError("INVALID_IMPORT_PAYLOAD", "manifest.json must contain a single JSON object")
	}

	pkg := Package{
		Manifest:     manifest,
		PackageHash:  hex.EncodeToString(packageSum[:]),
		ManifestHash: hex.EncodeToString(manifestSum[:]),
	}
	issues := validateManifest(pkg, blobs, r.now())
	pkg.Warnings = filterIssues(issues, "warning")

	return pkg, issues, nil
}

func readLimited(reader io.Reader, maxBytes int64) ([]byte, error) {
	if reader == nil {
		return nil, newValidationError("INVALID_IMPORT_PAYLOAD", "package is required")
	}
	limited := io.LimitReader(reader, maxBytes+1)
	payload, err := io.ReadAll(limited)
	if err != nil {
		return nil, newValidationError("INVALID_IMPORT_PAYLOAD", "package could not be read")
	}
	if int64(len(payload)) > maxBytes {
		return nil, newValidationError("FILE_TOO_LARGE", "import package exceeds upload limit")
	}
	return payload, nil
}

func readZipEntries(zipReader *zip.Reader, maxTotalBytes int64) ([]byte, map[string]blobRecord, error) {
	seen := map[string]struct{}{}
	blobs := map[string]blobRecord{}
	var manifest []byte
	var totalUncompressed int64

	for _, entry := range zipReader.File {
		name := entry.Name
		if _, ok := seen[name]; ok {
			return nil, nil, newValidationError("INVALID_IMPORT_PACKAGE", "duplicate ZIP entry is not allowed")
		}
		seen[name] = struct{}{}

		if err := validateEntryPath(name, entry); err != nil {
			return nil, nil, err
		}
		if entry.FileInfo().IsDir() {
			continue
		}
		if entry.UncompressedSize64 > uint64(maxTotalBytes) {
			return nil, nil, newValidationError("FILE_TOO_LARGE", "ZIP entry exceeds import limit")
		}
		totalUncompressed += int64(entry.UncompressedSize64)
		if totalUncompressed > maxTotalBytes {
			return nil, nil, newValidationError("FILE_TOO_LARGE", "import package uncompressed size exceeds limit")
		}

		switch {
		case name == manifestPath:
			data, err := readZipFile(entry, maxManifestBytes)
			if err != nil {
				return nil, nil, err
			}
			manifest = data
		case strings.HasPrefix(name, blobPathPrefix):
			data, err := readZipFile(entry, int64(entry.UncompressedSize64))
			if err != nil {
				return nil, nil, err
			}
			sum := sha256.Sum256(data)
			blobs[name] = blobRecord{Size: int64(len(data)), SHA256: hex.EncodeToString(sum[:])}
		default:
			return nil, nil, newValidationError("INVALID_IMPORT_PACKAGE", "ZIP entry is not allowed")
		}
	}

	return manifest, blobs, nil
}

func readZipFile(entry *zip.File, maxBytes int64) ([]byte, error) {
	reader, err := entry.Open()
	if err != nil {
		return nil, newValidationError("INVALID_IMPORT_PACKAGE", "ZIP entry could not be opened")
	}
	defer reader.Close()
	if maxBytes <= 0 {
		maxBytes = int64(entry.UncompressedSize64)
	}
	return readLimited(reader, maxBytes)
}

func validateEntryPath(name string, entry *zip.File) error {
	if name == "" || !utf8.ValidString(name) || strings.Contains(name, "\\") {
		return newValidationError("INVALID_IMPORT_PACKAGE", "ZIP entry path is invalid")
	}
	trimmed := strings.TrimSuffix(name, "/")
	if trimmed == "" || strings.HasPrefix(trimmed, "/") || strings.Contains(trimmed, "//") {
		return newValidationError("INVALID_IMPORT_PACKAGE", "ZIP entry path is invalid")
	}
	if path.Clean(trimmed) != trimmed || strings.HasPrefix(trimmed, "../") || strings.Contains(trimmed, "/../") {
		return newValidationError("INVALID_IMPORT_PACKAGE", "ZIP entry path must be normalized")
	}
	if entry.Flags&0x1 != 0 {
		return newValidationError("INVALID_IMPORT_PACKAGE", "encrypted ZIP entries are not supported")
	}
	if entry.FileInfo().Mode()&os.ModeSymlink != 0 {
		return newValidationError("INVALID_IMPORT_PACKAGE", "ZIP symlinks are not supported")
	}

	if entry.FileInfo().IsDir() {
		if name == "files/" || name == "files/sha256/" {
			return nil
		}
		return newValidationError("INVALID_IMPORT_PACKAGE", "ZIP directory entry is not allowed")
	}
	if name == manifestPath {
		return nil
	}
	if strings.HasPrefix(name, blobPathPrefix) {
		hash := strings.TrimPrefix(name, blobPathPrefix)
		if strings.Contains(hash, "/") || !sha256HexPattern.MatchString(hash) {
			return newValidationError("INVALID_IMPORT_PACKAGE", "file blob path must be files/sha256/{sha256}")
		}
		return nil
	}

	return newValidationError("INVALID_IMPORT_PACKAGE", "ZIP entry is not allowed")
}

func validateManifest(pkg Package, blobs map[string]blobRecord, now time.Time) []Issue {
	manifest := pkg.Manifest
	issues := []Issue{}
	addError := func(code, path, message string) {
		issues = append(issues, Issue{Code: code, Path: path, Message: message, Severity: "error"})
	}

	if manifest.Format != FormatName {
		addError("INVALID_IMPORT_PAYLOAD", "format", "format must be neo-chat-browser-import")
	}
	parseImportTime(manifest.GeneratedAt, now, "generatedAt", &issues)
	if manifest.ExportedAt != "" {
		parseImportTime(manifest.ExportedAt, now, "exportedAt", &issues)
	}
	if manifest.SchemaVersion != SchemaVersion {
		addError("UNSUPPORTED_SCHEMA_VERSION", "schemaVersion", "schemaVersion is not supported")
	}
	if strings.TrimSpace(manifest.IdempotencyKey) == "" {
		addError("INVALID_IMPORT_PAYLOAD", "idempotencyKey", "idempotencyKey is required")
	}
	if manifest.Source.App != "neo-chat" {
		addError("INVALID_IMPORT_PAYLOAD", "source.app", "source.app must be neo-chat")
	}
	if manifest.Counts.Conversations != len(manifest.Conversations) {
		addError("INVALID_IMPORT_PAYLOAD", "counts.conversations", "conversation count does not match manifest")
	}
	if manifest.Counts.Messages != len(manifest.Messages) {
		addError("INVALID_IMPORT_PAYLOAD", "counts.messages", "message count does not match manifest")
	}
	if manifest.Counts.Files != len(manifest.Files) {
		addError("INVALID_IMPORT_PAYLOAD", "counts.files", "file count does not match manifest")
	}

	conversationIDs := map[string]ImportConversation{}
	for i, conversation := range manifest.Conversations {
		base := fmt.Sprintf("conversations[%d]", i)
		conversation.ClientID = strings.TrimSpace(conversation.ClientID)
		if conversation.ClientID == "" {
			addError("INVALID_IMPORT_PAYLOAD", base+".clientId", "conversation clientId is required")
		} else if _, ok := conversationIDs[conversation.ClientID]; ok {
			addError("DUPLICATE_CLIENT_ID", base+".clientId", "conversation clientId is duplicated")
		} else {
			conversationIDs[conversation.ClientID] = conversation
		}
		status := normalizeConversationStatus(conversation.Status)
		if status == "" {
			addError("INVALID_IMPORT_PAYLOAD", base+".status", "conversation status is unsupported")
		}
		updatedAt, updatedOK := parseImportTime(conversation.UpdatedAt, now, base+".updatedAt", &issues)
		if conversation.CreatedAt != "" {
			createdAt, createdOK := parseImportTime(conversation.CreatedAt, now, base+".createdAt", &issues)
			if createdOK && updatedOK && updatedAt.Before(createdAt) {
				addError("INVALID_IMPORT_PAYLOAD", base+".updatedAt", "updatedAt must be greater than or equal to createdAt")
			}
		}
		if containsForbiddenSecret(conversation.Config) {
			addError("FORBIDDEN_SECRET_FIELD", base+".config", "conversation config contains forbidden secret-like fields")
		}
	}

	fileIDs := map[string]ImportFile{}
	var declaredBytes int64
	for i, file := range manifest.Files {
		base := fmt.Sprintf("files[%d]", i)
		if strings.TrimSpace(file.ClientFileID) == "" {
			addError("INVALID_IMPORT_PAYLOAD", base+".clientFileId", "file clientFileId is required")
		} else if _, ok := fileIDs[file.ClientFileID]; ok {
			addError("DUPLICATE_CLIENT_ID", base+".clientFileId", "file clientFileId is duplicated")
		} else {
			fileIDs[file.ClientFileID] = file
		}
		declaredBytes += file.Size
		if !sha256HexPattern.MatchString(file.SHA256) {
			addError("INVALID_IMPORT_PAYLOAD", base+".sha256", "file sha256 must be lowercase hex")
		}
		if !strings.HasPrefix(file.BlobPath, blobPathPrefix) {
			addError("INVALID_IMPORT_PAYLOAD", base+".blobPath", "file blobPath must be under files/sha256")
			continue
		}
		blob, ok := blobs[file.BlobPath]
		if !ok {
			addError("MISSING_FILE_BLOB", base+".blobPath", "referenced ZIP blob is missing")
			continue
		}
		if blob.Size != file.Size || blob.SHA256 != file.SHA256 || strings.TrimPrefix(file.BlobPath, blobPathPrefix) != file.SHA256 {
			addError("FILE_HASH_MISMATCH", base+".blobPath", "file size or SHA-256 does not match ZIP blob")
		}
	}
	if manifest.Counts.Bytes != declaredBytes {
		addError("INVALID_IMPORT_PAYLOAD", "counts.bytes", "byte count does not match manifest files")
	}
	if len(manifest.Files) > 0 {
		addError("INVALID_IMPORT_PAYLOAD", "files", "file object import is deferred to the attachment phase")
	}
	if len(blobs) > 0 && len(manifest.Files) == 0 {
		addError("INVALID_IMPORT_PACKAGE", "files/sha256", "unreferenced file blobs are not allowed")
	}

	messagesByID := map[string]ImportMessage{}
	messagesByConversation := map[string][]ImportMessage{}
	for i, message := range manifest.Messages {
		base := fmt.Sprintf("messages[%d]", i)
		message.ClientID = strings.TrimSpace(message.ClientID)
		if message.ClientID == "" {
			addError("INVALID_IMPORT_PAYLOAD", base+".clientId", "message clientId is required")
		} else if _, ok := messagesByID[message.ClientID]; ok {
			addError("DUPLICATE_CLIENT_ID", base+".clientId", "message clientId is duplicated")
		} else {
			messagesByID[message.ClientID] = message
		}
		if _, ok := conversationIDs[message.ConversationClientID]; !ok {
			addError("INVALID_IMPORT_PAYLOAD", base+".conversationClientId", "message references an unknown conversation")
		} else {
			messagesByConversation[message.ConversationClientID] = append(messagesByConversation[message.ConversationClientID], message)
		}
		if !isValidMessageRole(message.Role) {
			addError("INVALID_IMPORT_PAYLOAD", base+".role", "message role is unsupported")
		}
		if normalizeMessageStatus(message.Status) == "" {
			addError("INVALID_IMPORT_PAYLOAD", base+".status", "message status is unsupported")
		}
		if strings.TrimSpace(message.Content) == "" && len(message.Attachments) == 0 && len(message.OutputBlocks) == 0 {
			addError("INVALID_IMPORT_PAYLOAD", base+".content", "empty message content requires attachments or outputBlocks")
		}
		createdAt, createdOK := parseImportTime(message.CreatedAt, now, base+".createdAt", &issues)
		if message.CompletedAt != "" {
			completedAt, completedOK := parseImportTime(message.CompletedAt, now, base+".completedAt", &issues)
			if createdOK && completedOK && completedAt.Before(createdAt) {
				addError("INVALID_IMPORT_PAYLOAD", base+".completedAt", "completedAt must be greater than or equal to createdAt")
			}
		}
		if containsForbiddenSecret(message.Metadata) {
			addError("FORBIDDEN_SECRET_FIELD", base+".metadata", "message metadata contains forbidden secret-like fields")
		}
		if containsForbiddenSecret(message.OutputBlocks) {
			addError("FORBIDDEN_SECRET_FIELD", base+".outputBlocks", "message outputBlocks contain forbidden secret-like fields")
		}
		for j, attachment := range message.Attachments {
			attachmentPath := fmt.Sprintf("%s.attachments[%d]", base, j)
			if attachmentContainsSecret(attachment) {
				addError("FORBIDDEN_SECRET_FIELD", attachmentPath, "attachment contains forbidden secret-like fields")
			}
			source := strings.ToLower(strings.TrimSpace(attachment.Source))
			switch source {
			case "remote", "knowledge_ref":
			case "file":
				if _, ok := fileIDs[attachment.ClientFileID]; !ok {
					addError("MISSING_FILE_BLOB", attachmentPath+".clientFileId", "file attachment references an unknown file")
				} else {
					addError("INVALID_IMPORT_PAYLOAD", attachmentPath, "file attachments are deferred to the attachment phase")
				}
			default:
				addError("INVALID_IMPORT_PAYLOAD", attachmentPath+".source", "attachment source is unsupported")
			}
		}
	}

	for conversationClientID, messages := range messagesByConversation {
		sort.Slice(messages, func(i, j int) bool { return messages[i].SequenceNo < messages[j].SequenceNo })
		seenSeq := map[int]string{}
		seenMessage := map[string]int{}
		for index, message := range messages {
			if message.SequenceNo != index {
				addError("INVALID_MESSAGE_TREE", "messages", "sequenceNo must be gap-free per conversation")
			}
			if _, ok := seenSeq[message.SequenceNo]; ok {
				addError("INVALID_MESSAGE_TREE", "messages", "sequenceNo must be unique per conversation")
			}
			seenSeq[message.SequenceNo] = message.ClientID
			seenMessage[message.ClientID] = message.SequenceNo
		}
		for _, message := range messages {
			if message.ParentClientID == "" {
				continue
			}
			parentSeq, ok := seenMessage[message.ParentClientID]
			if !ok {
				addError("INVALID_MESSAGE_TREE", "messages", "parentClientId must reference a message in the same conversation")
				continue
			}
			if parentSeq >= message.SequenceNo {
				addError("INVALID_MESSAGE_TREE", "messages", "parentClientId must reference an earlier message")
			}
		}
		_ = conversationClientID
	}

	return issues
}

func attachmentContainsSecret(attachment ImportAttachment) bool {
	if isForbiddenSecretKey(attachment.ClientAttachmentID) ||
		isForbiddenSecretKey(attachment.ClientFileID) ||
		isForbiddenSecretKey(attachment.FileName) {
		return true
	}
	if attachment.URL == "" {
		return false
	}
	parsed, err := url.Parse(attachment.URL)
	if err != nil {
		return false
	}
	if parsed.User != nil {
		return true
	}
	if isForbiddenSecretKey(parsed.Fragment) {
		return true
	}
	for key := range parsed.Query() {
		if isForbiddenSecretKey(key) {
			return true
		}
		for _, value := range parsed.Query()[key] {
			if isForbiddenSecretKey(value) {
				return true
			}
		}
	}
	for _, segment := range strings.Split(parsed.Path, "/") {
		if isForbiddenSecretKey(segment) {
			return true
		}
	}
	return false
}

func parseImportTime(value string, now time.Time, issuePath string, issues *[]Issue) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" || !strings.HasSuffix(value, "Z") {
		*issues = append(*issues, Issue{Code: "INVALID_IMPORT_PAYLOAD", Path: issuePath, Message: "timestamp must be UTC RFC3339", Severity: "error"})
		return time.Time{}, false
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		*issues = append(*issues, Issue{Code: "INVALID_IMPORT_PAYLOAD", Path: issuePath, Message: "timestamp must be valid RFC3339", Severity: "error"})
		return time.Time{}, false
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if parsed.After(now) {
		*issues = append(*issues, Issue{Code: "INVALID_IMPORT_PAYLOAD", Path: issuePath, Message: "timestamp is too far in the future", Severity: "error"})
		return time.Time{}, false
	}
	return parsed.UTC(), true
}

func normalizeConversationStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "", "active":
		return "active"
	case "archived":
		return "archived"
	default:
		return ""
	}
}

func normalizeMessageStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "", "completed":
		return "completed"
	case "failed", "cancelled":
		return strings.ToLower(strings.TrimSpace(status))
	default:
		return ""
	}
}

func isValidMessageRole(role string) bool {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "system", "user", "assistant", "tool":
		return true
	default:
		return false
	}
}

func filterIssues(issues []Issue, severity string) []Issue {
	filtered := make([]Issue, 0)
	for _, issue := range issues {
		if issue.Severity == severity {
			filtered = append(filtered, issue)
		}
	}
	return filtered
}

func containsForbiddenSecret(value any) bool {
	switch typed := value.(type) {
	case nil:
		return false
	case map[string]any:
		for key, child := range typed {
			if isForbiddenSecretKey(key) || containsForbiddenSecret(child) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if containsForbiddenSecret(child) {
				return true
			}
		}
	}
	return false
}

func isForbiddenSecretKey(key string) bool {
	key = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(key), "_", ""))
	for _, fragment := range []string{"apikey", "accesstoken", "authorization", "bearertoken", "cookie", "secret", "password", "token"} {
		if strings.Contains(key, fragment) {
			return true
		}
	}
	return false
}

func (r PackageReader) now() time.Time {
	if r.Now != nil {
		return r.Now().UTC()
	}
	return time.Now().UTC()
}
