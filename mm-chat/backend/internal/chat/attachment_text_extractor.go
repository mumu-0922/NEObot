package chat

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

func directAttachmentKind(fileName string, mimeType string) string {
	extension := strings.ToLower(filepath.Ext(strings.TrimSpace(fileName)))
	mimeType = normalizedAttachmentMIMEType(mimeType)

	switch extension {
	case ".pdf":
		return "pdf"
	case ".docx":
		return "docx"
	case ".pptx":
		return "pptx"
	case ".xlsx":
		return "xlsx"
	case ".doc", ".ppt", ".xls":
		return "unsupported"
	}

	switch mimeType {
	case "application/pdf":
		return "pdf"
	case "application/vnd.openxmlformats-officedocument.wordprocessingml.document":
		return "docx"
	case "application/vnd.openxmlformats-officedocument.presentationml.presentation":
		return "pptx"
	case "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":
		return "xlsx"
	case "application/msword", "application/vnd.ms-powerpoint", "application/vnd.ms-excel":
		return "unsupported"
	}

	if strings.HasPrefix(mimeType, "text/") || supportedTextMIMEType(mimeType) ||
		supportedTextExtension(extension) {
		return "text"
	}
	return "unsupported"
}

func normalizedAttachmentMIMEType(mimeType string) string {
	mimeType = strings.ToLower(strings.TrimSpace(mimeType))
	if semicolon := strings.IndexByte(mimeType, ';'); semicolon >= 0 {
		mimeType = strings.TrimSpace(mimeType[:semicolon])
	}
	return mimeType
}

func supportedTextMIMEType(mimeType string) bool {
	switch mimeType {
	case "application/json",
		"application/ld+json",
		"application/xml",
		"application/xhtml+xml",
		"application/javascript",
		"application/typescript",
		"application/x-yaml",
		"application/yaml",
		"application/sql",
		"application/graphql",
		"application/x-sh",
		"application/x-httpd-php",
		"application/toml":
		return true
	default:
		return false
	}
}

func supportedTextExtension(extension string) bool {
	switch extension {
	case ".txt", ".text", ".md", ".markdown", ".csv", ".tsv",
		".json", ".jsonl", ".xml", ".html", ".htm", ".yaml", ".yml",
		".log", ".sql", ".graphql", ".gql", ".toml", ".ini", ".conf",
		".env", ".go", ".rs", ".py", ".rb", ".php", ".java", ".kt",
		".kts", ".c", ".h", ".cc", ".cpp", ".cxx", ".hpp", ".cs",
		".js", ".jsx", ".mjs", ".cjs", ".ts", ".tsx", ".css", ".scss",
		".sass", ".less", ".sh", ".bash", ".zsh", ".fish", ".ps1",
		".dockerfile", ".gradle", ".properties":
		return true
	default:
		return false
	}
}

func extractUTF8AttachmentText(
	attachment ProviderAttachment,
) (extractedAttachmentText, error) {
	data := bytes.TrimPrefix(attachment.Data, []byte{0xef, 0xbb, 0xbf})
	if bytes.IndexByte(data, 0) >= 0 || !utf8.Valid(data) {
		return extractedAttachmentText{}, newValidationError(
			"ATTACHMENT_ENCODING_UNSUPPORTED",
			fmt.Sprintf(
				"attachment %q is not valid UTF-8 text",
				attachmentDisplayName(attachment),
			),
		)
	}
	return boundedExtractedText(string(data), false, attachment)
}
