package mcpclient

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/google/uuid"
)

const (
	maxToolNameBytes        = 256
	maxToolDescriptionBytes = 4096
	maxToolSchemaBytes      = 32 * 1024
	maxProviderAliasBytes   = 64
)

var aliasUnsafePattern = regexp.MustCompile(`[^a-zA-Z0-9_]`)

func validUUID(value string) bool {
	_, err := uuid.Parse(strings.TrimSpace(value))
	return err == nil
}

func validHeaderName(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 128 {
		return false
	}
	for _, char := range value {
		if !(char >= 'A' && char <= 'Z') && !(char >= 'a' && char <= 'z') &&
			!(char >= '0' && char <= '9') && !strings.ContainsRune("!#$%&'*+-.^_`|~", char) {
			return false
		}
	}
	return http.CanonicalHeaderKey(value) != ""
}

func normalizeStrings(values []string, maximum int, maxBytes int) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || len(value) > maxBytes {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
		if len(result) == maximum {
			break
		}
	}
	return result
}

func toolAlias(ref ServerRef, name string) string {
	digest := sha256.Sum256([]byte(ref.Key()))
	prefix := hex.EncodeToString(digest[:6])
	base := aliasUnsafePattern.ReplaceAllString(strings.TrimSpace(name), "_")
	base = strings.Trim(base, "_")
	if base == "" || !unicode.IsLetter(rune(base[0])) {
		base = "tool_" + base
	}
	nameDigest := sha256.Sum256([]byte(name))
	suffix := hex.EncodeToString(nameDigest[:4])
	maximumBase := maxProviderAliasBytes - len("mcp__") - len(prefix) - len(suffix)
	if maximumBase < 1 {
		maximumBase = 1
	}
	if len(base) > maximumBase {
		base = base[:maximumBase]
	}
	return fmt.Sprintf("mcp_%s_%s_%s", prefix, base, suffix)
}

func normalizeTool(ref ServerRef, name, title, description string, schema any, classification string) Tool {
	name = strings.TrimSpace(name)
	title = strings.TrimSpace(title)
	description = strings.TrimSpace(description)
	if ref.Source == SourcePrivate {
		classification = ClassificationUnknown
	}
	tool := Tool{
		ServerRef:      ref,
		Name:           name,
		Alias:          toolAlias(ref, name),
		Title:          title,
		Description:    description,
		Classification: normalizeClassification(classification),
		Supported:      true,
	}
	if name == "" || len(name) > maxToolNameBytes {
		tool.Supported = false
		tool.Unsupported = "invalid_name"
		return tool
	}
	if len(description) > maxToolDescriptionBytes {
		tool.Supported = false
		tool.Unsupported = "description_too_large"
		return tool
	}
	encoded, err := json.Marshal(schema)
	if err != nil || len(encoded) > maxToolSchemaBytes {
		tool.Supported = false
		tool.Unsupported = "schema_too_large"
		return tool
	}
	var normalized map[string]any
	if err := json.Unmarshal(encoded, &normalized); err != nil || normalized == nil {
		tool.Supported = false
		tool.Unsupported = "schema_not_object"
		return tool
	}
	if err := validateToolSchema(normalized, 0); err != nil {
		tool.Supported = false
		tool.Unsupported = "schema_unsupported"
		return tool
	}
	if _, err := resolveToolSchema(normalized); err != nil {
		tool.Supported = false
		tool.Unsupported = "schema_invalid"
		return tool
	}
	tool.InputSchema = normalized
	return tool
}

func ValidateToolArguments(tool Tool, arguments map[string]any) error {
	if !tool.Supported || tool.InputSchema == nil {
		return ErrToolUnsupported
	}
	if arguments == nil {
		arguments = map[string]any{}
	}
	resolved, err := resolveToolSchema(tool.InputSchema)
	if err != nil {
		return ErrToolUnsupported
	}
	if err := resolved.Validate(arguments); err != nil {
		return ErrToolArgumentsInvalid
	}
	return nil
}

func resolveToolSchema(schema map[string]any) (*jsonschema.Resolved, error) {
	encoded, err := json.Marshal(schema)
	if err != nil {
		return nil, err
	}
	var parsed jsonschema.Schema
	if err := json.Unmarshal(encoded, &parsed); err != nil {
		return nil, err
	}
	if parsed.Type != "object" {
		return nil, ErrToolUnsupported
	}
	return parsed.Resolve(nil)
}

func validateToolSchema(schema map[string]any, depth int) error {
	if depth > 12 {
		return ErrToolUnsupported
	}
	if schemaType, ok := schema["type"].(string); !ok || (depth == 0 && schemaType != "object") {
		return ErrToolUnsupported
	}
	for _, keyword := range []string{"$ref", "$dynamicRef", "oneOf", "anyOf", "allOf", "not", "if", "then", "else"} {
		if _, exists := schema[keyword]; exists {
			return ErrToolUnsupported
		}
	}
	properties, hasProperties := schema["properties"]
	if !hasProperties {
		return nil
	}
	propertyMap, ok := properties.(map[string]any)
	if !ok || len(propertyMap) > 128 {
		return ErrToolUnsupported
	}
	for key, raw := range propertyMap {
		if strings.TrimSpace(key) == "" || len(key) > 256 {
			return ErrToolUnsupported
		}
		child, ok := raw.(map[string]any)
		if !ok {
			return ErrToolUnsupported
		}
		if childType, _ := child["type"].(string); childType == "array" {
			if items, ok := child["items"].(map[string]any); ok {
				if err := validateToolSchema(items, depth+1); err != nil {
					return err
				}
			}
		}
		if childType, _ := child["type"].(string); childType == "object" {
			if err := validateToolSchema(child, depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}

func normalizeClassification(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case ClassificationRead:
		return ClassificationRead
	case ClassificationWrite:
		return ClassificationWrite
	default:
		return ClassificationUnknown
	}
}

func sortedTools(tools []Tool) []Tool {
	result := append([]Tool(nil), tools...)
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].ServerRef.Key() != result[j].ServerRef.Key() {
			return result[i].ServerRef.Key() < result[j].ServerRef.Key()
		}
		return result[i].Name < result[j].Name
	})
	return result
}
