package skillsupply

import (
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

const (
	maxSkillMarkdownBytes       = int64(1 << 20)
	maxSkillMetadataEntries     = 64
	maxSkillMetadataKeyBytes    = 128
	maxSkillMetadataValueBytes  = 1024
	maxSkillMetadataOpaqueDepth = 16
	maxSkillMetadataOpaqueNodes = 4096
	maxSkillMetadataScalarBytes = 4096
)

type skillFrontmatter struct {
	Name          string
	Description   string
	License       string
	Compatibility string
	Metadata      map[string]string
	AllowedTools  string
}

func parseSkillMarkdown(files []packageFile) (SkillMetadata, error) {
	var body []byte
	for _, file := range files {
		if file.path == "SKILL.md" {
			body = file.data
			break
		}
	}
	if len(body) == 0 || int64(len(body)) > maxSkillMarkdownBytes || !utf8.Valid(body) {
		return SkillMetadata{}, ErrManifestInvalid
	}
	text := string(body)
	if !strings.HasPrefix(text, "---\n") && !strings.HasPrefix(text, "---\r\n") {
		return SkillMetadata{}, ErrManifestInvalid
	}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	end := strings.Index(text[4:], "\n---\n")
	if end < 0 {
		return SkillMetadata{}, ErrManifestInvalid
	}
	frontmatterBytes := []byte(text[4 : 4+end])
	var document yaml.Node
	if err := yaml.Unmarshal(frontmatterBytes, &document); err != nil || len(document.Content) != 1 {
		return SkillMetadata{}, ErrManifestInvalid
	}
	frontmatter, err := decodeFrontmatter(document.Content[0])
	if err != nil {
		return SkillMetadata{}, err
	}
	if !skillNamePattern.MatchString(frontmatter.Name) || len(frontmatter.Name) > 64 ||
		frontmatter.Description == "" || len([]rune(frontmatter.Description)) > 1024 ||
		len([]rune(frontmatter.License)) > 512 || len([]rune(frontmatter.Compatibility)) > 500 ||
		len(frontmatter.Metadata) > maxSkillMetadataEntries || len(frontmatter.AllowedTools) > 4096 ||
		!validPlainText(frontmatter.Description) || !validPlainText(frontmatter.License) ||
		!validPlainText(frontmatter.Compatibility) {
		return SkillMetadata{}, ErrManifestInvalid
	}
	for key, value := range frontmatter.Metadata {
		if strings.TrimSpace(key) == "" || len(key) > maxSkillMetadataKeyBytes ||
			len(value) > maxSkillMetadataValueBytes ||
			!validPlainText(key) || !validPlainText(value) {
			return SkillMetadata{}, ErrManifestInvalid
		}
	}
	allowedTools, err := parseAllowedTools(frontmatter.AllowedTools)
	if err != nil {
		return SkillMetadata{}, err
	}
	return SkillMetadata{Name: frontmatter.Name, Description: frontmatter.Description,
		License: frontmatter.License, Compatibility: frontmatter.Compatibility,
		Metadata: frontmatter.Metadata, AllowedTools: allowedTools}, nil
}

func decodeFrontmatter(node *yaml.Node) (skillFrontmatter, error) {
	if node == nil || node.Kind != yaml.MappingNode || len(node.Content)%2 != 0 {
		return skillFrontmatter{}, ErrManifestInvalid
	}
	result := skillFrontmatter{Metadata: map[string]string{}}
	seen := map[string]struct{}{}
	for index := 0; index < len(node.Content); index += 2 {
		keyNode, valueNode := node.Content[index], node.Content[index+1]
		key := keyNode.Value
		if keyNode.Kind != yaml.ScalarNode {
			return result, ErrManifestInvalid
		}
		if _, duplicate := seen[key]; duplicate {
			return result, ErrManifestInvalid
		}
		seen[key] = struct{}{}
		switch key {
		case "name":
			result.Name = scalarString(valueNode)
		case "description":
			result.Description = scalarString(valueNode)
		case "license":
			result.License = scalarString(valueNode)
		case "compatibility":
			result.Compatibility = scalarString(valueNode)
		case "allowed-tools":
			result.AllowedTools = scalarString(valueNode)
		case "metadata":
			metadata, err := projectSkillMetadata(valueNode)
			if err != nil {
				return result, err
			}
			result.Metadata = metadata
		default:
			// Unknown standard extensions remain untrusted package bytes and are
			// deliberately ignored rather than becoming server authority.
		}
		if key != "metadata" && (key == "name" || key == "description" || key == "license" ||
			key == "compatibility" || key == "allowed-tools") && !isStringScalar(valueNode) {
			return result, ErrManifestInvalid
		}
	}
	return result, nil
}

func scalarString(node *yaml.Node) string {
	if !isStringScalar(node) {
		return ""
	}
	return strings.TrimSpace(node.Value)
}

func isStringScalar(node *yaml.Node) bool {
	return node != nil && node.Kind == yaml.ScalarNode && node.Tag != "!!null" &&
		(node.Tag == "!!str" || node.Tag == "")
}

func projectSkillMetadata(node *yaml.Node) (map[string]string, error) {
	if node == nil || node.Kind != yaml.MappingNode || len(node.Content)%2 != 0 ||
		len(node.Content)/2 > maxSkillMetadataEntries {
		return nil, ErrManifestInvalid
	}
	result := map[string]string{}
	seen := map[string]struct{}{}
	state := skillMetadataValidationState{}
	for index := 0; index < len(node.Content); index += 2 {
		key, keyOK := strictScalarString(node.Content[index])
		if !keyOK || key == "" || len(key) > maxSkillMetadataKeyBytes || !validPlainText(key) {
			return nil, ErrManifestInvalid
		}
		if _, duplicate := seen[key]; duplicate {
			return nil, ErrManifestInvalid
		}
		seen[key] = struct{}{}

		valueNode := node.Content[index+1]
		if value, valueOK := strictScalarString(valueNode); valueOK {
			if len(value) > maxSkillMetadataValueBytes || !validPlainText(value) {
				return nil, ErrManifestInvalid
			}
			result[key] = value
			continue
		}
		if valueNode == nil || valueNode.Kind == yaml.ScalarNode ||
			state.validateOpaque(valueNode, 1) != nil {
			return nil, ErrManifestInvalid
		}
	}
	return result, nil
}

type skillMetadataValidationState struct {
	nodes int
}

// validateOpaque accepts ecosystem-specific metadata only as bounded inert
// structure. Nothing visited here is projected into runtime authority.
func (state *skillMetadataValidationState) validateOpaque(node *yaml.Node, depth int) error {
	if node == nil || depth > maxSkillMetadataOpaqueDepth {
		return ErrManifestInvalid
	}
	state.nodes++
	if state.nodes > maxSkillMetadataOpaqueNodes {
		return ErrManifestInvalid
	}
	switch node.Kind {
	case yaml.ScalarNode:
		if len(node.Value) > maxSkillMetadataScalarBytes {
			return ErrManifestInvalid
		}
		return nil
	case yaml.SequenceNode:
		for _, child := range node.Content {
			if err := state.validateOpaque(child, depth+1); err != nil {
				return err
			}
		}
		return nil
	case yaml.MappingNode:
		if len(node.Content)%2 != 0 {
			return ErrManifestInvalid
		}
		seen := map[string]struct{}{}
		for index := 0; index < len(node.Content); index += 2 {
			key, ok := strictScalarString(node.Content[index])
			if !ok || key == "" || len(key) > maxSkillMetadataKeyBytes || !validPlainText(key) {
				return ErrManifestInvalid
			}
			if _, duplicate := seen[key]; duplicate {
				return ErrManifestInvalid
			}
			seen[key] = struct{}{}
			state.nodes++
			if state.nodes > maxSkillMetadataOpaqueNodes {
				return ErrManifestInvalid
			}
			if err := state.validateOpaque(node.Content[index+1], depth+1); err != nil {
				return err
			}
		}
		return nil
	default:
		// Alias and document nodes are deliberately not dereferenced.
		return ErrManifestInvalid
	}
}

func strictScalarString(node *yaml.Node) (string, bool) {
	if node == nil || node.Kind != yaml.ScalarNode || node.Tag != "!!str" {
		return "", false
	}
	return strings.TrimSpace(node.Value), true
}

func parseAllowedTools(value string) ([]string, error) {
	fields := strings.Fields(value)
	if len(fields) > 64 {
		return nil, ErrManifestInvalid
	}
	for _, field := range fields {
		if len(field) > 128 || !validPlainText(field) {
			return nil, ErrManifestInvalid
		}
	}
	return splitAllowedTools(strings.Join(fields, " ")), nil
}

func splitAllowedTools(value any) []string {
	var text string
	switch typed := value.(type) {
	case string:
		text = typed
	case []string:
		return append([]string(nil), typed...)
	default:
		return []string{}
	}
	fields := strings.Fields(text)
	if len(fields) > 64 {
		fields = fields[:64]
	}
	result := make([]string, 0, len(fields))
	seen := map[string]struct{}{}
	for _, field := range fields {
		if len(field) > 128 || !validPlainText(field) {
			continue
		}
		if _, exists := seen[field]; exists {
			continue
		}
		seen[field] = struct{}{}
		result = append(result, field)
	}
	return result
}
