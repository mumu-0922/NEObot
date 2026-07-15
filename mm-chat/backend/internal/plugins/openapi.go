package plugins

import (
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
)

const (
	maxOpenAPIPaths             = 200
	maxParametersPerFunction    = 50
	maxPluginFunctions          = 20
	maxPluginDescriptionChars   = 1024
	maxPluginFunctionNameChars  = 128
	defaultCustomPluginIDPrefix = "custom"
)

var supportedOpenAPIMethods = map[string]struct{}{
	"get":    {},
	"post":   {},
	"put":    {},
	"patch":  {},
	"delete": {},
}

var functionNameUnsafeRE = regexp.MustCompile(`[^a-zA-Z0-9_]`)

type pluginBaseMetadata struct {
	ID              string
	Title           string
	Description     string
	LogoURL         string
	ManifestURL     string
	ExternalDocsURL string
	Category        string
	Categories      []string
	Added           string
}

func convertOpenAPISpecToPlugin(spec map[string]any, base pluginBaseMetadata, manifestURL string) (Plugin, error) {
	paths, ok := asRecord(spec["paths"])
	if !ok {
		return Plugin{}, ErrPluginManifestInvalid
	}
	baseURL, err := openAPIBaseURL(spec, manifestURL)
	if err != nil {
		return Plugin{}, err
	}
	if err := validatePluginBaseURL(baseURL); err != nil {
		return Plugin{}, err
	}

	functions := make([]PluginFunction, 0, maxPluginFunctions)
	pathCount := 0
	for rawPath, rawMethods := range paths {
		pathCount++
		if pathCount > maxOpenAPIPaths || len(functions) >= maxPluginFunctions {
			break
		}
		path := normalizeOpenAPIPath(rawPath)
		methods, ok := asRecord(rawMethods)
		if path == "" || !ok {
			continue
		}
		for rawMethod, rawOperation := range methods {
			method := strings.ToLower(rawMethod)
			if _, ok := supportedOpenAPIMethods[method]; !ok {
				continue
			}
			operation, ok := asRecord(rawOperation)
			if !ok {
				continue
			}
			description := firstString(operation["summary"], operation["description"])
			if description == "" {
				continue
			}
			operationID := stringValue(operation["operationId"])
			fallbackName := fmt.Sprintf("fn_%s_%d", method, len(functions)+1)
			functionName := sanitizeFunctionName(firstNonEmpty(operationID, method+path), fallbackName)
			parameters := openAPIParameters(operation)

			functions = append(functions, PluginFunction{
				Name:        functionName,
				Description: truncate(description, maxPluginDescriptionChars),
				Parameters:  parameters,
				Path:        path,
				Method:      strings.ToUpper(method),
			})
			if len(functions) >= maxPluginFunctions {
				break
			}
		}
	}
	if len(functions) == 0 {
		return Plugin{}, ErrPluginManifestInvalid
	}

	info, _ := asRecord(spec["info"])
	plugin := Plugin{
		ID:              firstNonEmpty(base.ID, defaultCustomPluginIDPrefix),
		Title:           firstNonEmpty(base.Title, stringValue(info["title"]), "Unknown Plugin"),
		Description:     firstNonEmpty(base.Description, stringValue(info["description"]), ""),
		LogoURL:         base.LogoURL,
		ManifestURL:     firstNonEmpty(base.ManifestURL, manifestURL),
		ExternalDocsURL: base.ExternalDocsURL,
		BaseURL:         baseURL,
		Functions:       functions,
		Category:        base.Category,
		Categories:      base.Categories,
		Added:           base.Added,
	}
	if auth := openAPIAuth(spec); auth != nil {
		plugin.Auth = auth
	}
	return plugin, nil
}

func openAPIBaseURL(spec map[string]any, manifestURL string) (string, error) {
	if servers, ok := spec["servers"].([]any); ok {
		for _, item := range servers {
			server, ok := asRecord(item)
			if !ok {
				continue
			}
			candidate := strings.TrimSpace(regexp.MustCompile(`\{[^}]+\}`).ReplaceAllString(stringValue(server["url"]), ""))
			if candidate == "" {
				continue
			}
			if manifestURL != "" {
				base, err := url.Parse(manifestURL)
				if err != nil {
					return "", ErrPluginManifestInvalid
				}
				resolved, err := base.Parse(candidate)
				if err != nil {
					return "", ErrPluginManifestInvalid
				}
				return resolved.String(), nil
			}
			return candidate, nil
		}
	}

	host := strings.TrimSpace(stringValue(spec["host"]))
	if host != "" {
		scheme := "https"
		if schemes, ok := spec["schemes"].([]any); ok {
			for _, candidate := range schemes {
				if stringValue(candidate) == "http" {
					scheme = "http"
					break
				}
			}
		}
		return scheme + "://" + host + stringValue(spec["basePath"]), nil
	}
	return "", ErrPluginManifestInvalid
}

func validatePluginBaseURL(value string) error {
	parsed, err := url.Parse(value)
	if err != nil || parsed == nil || parsed.Hostname() == "" {
		return ErrPluginManifestInvalid
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return ErrPluginURLBlocked
	}
	if parsed.User != nil {
		return ErrPluginURLBlocked
	}
	host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	if strings.ContainsAny(host, "\r\n") {
		return ErrPluginURLBlocked
	}
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return ErrPluginURLBlocked
	}
	if ip := net.ParseIP(host); ip != nil && isBlockedIP(ip) {
		return ErrPluginURLBlocked
	}
	return nil
}

func normalizeOpenAPIPath(path string) string {
	if !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") {
		return ""
	}
	return truncate(path, 1024)
}

func openAPIParameters(operation map[string]any) map[string]any {
	properties := map[string]any{}
	required := []any{}
	parameters, _ := operation["parameters"].([]any)
	for index, raw := range parameters {
		if index >= maxParametersPerFunction {
			break
		}
		param, ok := asRecord(raw)
		if !ok {
			continue
		}
		location := stringValue(param["in"])
		if location != "path" && location != "query" {
			continue
		}
		name := sanitizeParameterName(stringValue(param["name"]))
		if name == "" {
			continue
		}
		schema, _ := asRecord(param["schema"])
		properties[name] = map[string]any{
			"type":        mapOpenAPIType(firstNonEmpty(stringValue(schema["type"]), stringValue(param["type"]))),
			"description": firstNonEmpty(stringValue(param["description"]), stringValue(param["name"])),
		}
		if requiredValue, ok := param["required"].(bool); ok && requiredValue {
			required = append(required, name)
		}
	}
	result := map[string]any{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		result["required"] = required
	}
	return result
}

func openAPIAuth(spec map[string]any) *PluginAuth {
	var schemes map[string]any
	if components, ok := asRecord(spec["components"]); ok {
		schemes, _ = asRecord(components["securitySchemes"])
	}
	if schemes == nil {
		schemes, _ = asRecord(spec["securityDefinitions"])
	}
	for _, raw := range schemes {
		scheme, ok := asRecord(raw)
		if !ok {
			continue
		}
		switch stringValue(scheme["type"]) {
		case "apiKey":
			return &PluginAuth{Type: "apiKey", Name: stringValue(scheme["name"]), In: stringValue(scheme["in"])}
		case "oauth2":
			return &PluginAuth{Type: "bearer"}
		case "http":
			if stringValue(scheme["scheme"]) == "bearer" {
				return &PluginAuth{Type: "bearer"}
			}
		}
	}
	return nil
}

func sanitizeFunctionName(value string, fallback string) string {
	cleaned := functionNameUnsafeRE.ReplaceAllString(value, "_")
	cleaned = truncate(cleaned, maxPluginFunctionNameChars)
	if cleaned == "" {
		cleaned = fallback
	}
	if cleaned[0] >= '0' && cleaned[0] <= '9' {
		return "fn_" + cleaned
	}
	return cleaned
}

func sanitizeParameterName(value string) string {
	return functionNameUnsafeRE.ReplaceAllString(value, "_")
}

func mapOpenAPIType(value string) string {
	switch value {
	case "string", "number", "integer", "boolean", "array", "object":
		return value
	default:
		return "string"
	}
}

func asRecord(value any) (map[string]any, bool) {
	record, ok := value.(map[string]any)
	return record, ok
}

func stringValue(value any) string {
	if value, ok := value.(string); ok {
		return value
	}
	return ""
}

func firstString(values ...any) string {
	for _, value := range values {
		if text := stringValue(value); strings.TrimSpace(text) != "" {
			return text
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func truncate(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max]
}
