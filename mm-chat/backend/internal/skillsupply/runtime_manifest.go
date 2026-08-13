package skillsupply

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"neo-chat/mm-chat/backend/internal/strictjson"
)

const maxManifestBytes = 256 << 10

var (
	skillNamePattern   = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	semverPattern      = regexp.MustCompile(`^(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)(?:-[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$`)
	identifierPattern  = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[._-][a-z0-9]+)*$`)
	digestPattern      = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
	imageDigestPattern = regexp.MustCompile(`^[^@\s]+@sha256:[a-f0-9]{64}$`)
)

func parseRuntimeManifest(files []packageFile, skillName string) (*RuntimeManifest, error) {
	var body []byte
	for _, file := range files {
		if file.path == "neo.runtime.json" {
			body = file.data
			break
		}
	}
	if body == nil {
		return nil, nil
	}
	var manifest RuntimeManifest
	if err := validateManifestRequiredKeys(body); err != nil ||
		strictjson.Decode(body, maxManifestBytes, &manifest) != nil ||
		!validRuntimeManifest(manifest, skillName) {
		return nil, ErrManifestInvalid
	}
	return &manifest, nil
}

func validateManifestRequiredKeys(body []byte) error {
	type rawManifest struct {
		Package            json.RawMessage   `json:"package"`
		Runtime            json.RawMessage   `json:"runtime"`
		Entrypoints        []json.RawMessage `json:"entrypoints"`
		Dependencies       []json.RawMessage `json:"dependencies"`
		CapabilityRequests []json.RawMessage `json:"capabilityRequests"`
		EgressRequests     []json.RawMessage `json:"egressRequests"`
		SecretSlots        []json.RawMessage `json:"secretSlots"`
		Resources          json.RawMessage   `json:"resources"`
		Limits             json.RawMessage   `json:"limits"`
	}
	if err := strictjson.RequireExactKeys(body, []string{"schemaVersion", "package", "runtime",
		"entrypoints", "dependencies", "capabilityRequests", "egressRequests", "secretSlots",
		"resources", "limits"}); err != nil {
		return err
	}
	var raw rawManifest
	if err := json.Unmarshal(body, &raw); err != nil {
		return err
	}
	checks := []struct {
		body json.RawMessage
		keys []string
	}{
		{raw.Package, []string{"name", "version"}},
		{raw.Runtime, []string{"kind", "image", "platform", "user"}},
		{raw.Resources, []string{"cpuMillis", "memoryMiB", "pids", "wallSeconds"}},
		{raw.Limits, []string{"maxPackageFiles", "maxPackageBytes", "maxExpandedBytes",
			"maxStdoutBytes", "maxStderrBytes", "maxArtifactBytes"}},
	}
	var runtime struct {
		User json.RawMessage `json:"user"`
	}
	if err := json.Unmarshal(raw.Runtime, &runtime); err != nil {
		return err
	}
	checks = append(checks, struct {
		body json.RawMessage
		keys []string
	}{runtime.User, []string{"uid", "gid"}})
	for _, item := range raw.Entrypoints {
		checks = append(checks, struct {
			body json.RawMessage
			keys []string
		}{item, []string{"name", "argv", "workingDirectory"}})
	}
	for _, item := range raw.Dependencies {
		checks = append(checks, struct {
			body json.RawMessage
			keys []string
		}{item, []string{"name", "digest"}})
	}
	for _, item := range raw.CapabilityRequests {
		checks = append(checks, struct {
			body json.RawMessage
			keys []string
		}{item, []string{"capability", "actions", "reason"}})
	}
	for _, item := range raw.EgressRequests {
		checks = append(checks, struct {
			body json.RawMessage
			keys []string
		}{item, []string{"id", "mode", "schemes", "hosts", "ports", "reason"}})
	}
	for _, item := range raw.SecretSlots {
		checks = append(checks, struct {
			body json.RawMessage
			keys []string
		}{item, []string{"name", "purpose", "delivery", "required"}})
	}
	for _, check := range checks {
		if err := strictjson.RequireExactKeys(check.body, check.keys); err != nil {
			return err
		}
	}
	return nil
}

func validRuntimeManifest(manifest RuntimeManifest, skillName string) bool {
	if manifest.SchemaVersion != "neo.skill-runtime/v1" || manifest.Package.Name != skillName ||
		!skillNamePattern.MatchString(manifest.Package.Name) || !semverPattern.MatchString(manifest.Package.Version) ||
		manifest.Runtime.Kind != "rootless_oci" || !imageDigestPattern.MatchString(manifest.Runtime.Image) ||
		(manifest.Runtime.Platform != "linux/amd64" && manifest.Runtime.Platform != "linux/arm64") ||
		manifest.Runtime.User.UID < 1 || manifest.Runtime.User.UID > 2147483647 ||
		manifest.Runtime.User.GID < 1 || manifest.Runtime.User.GID > 2147483647 ||
		len(manifest.Entrypoints) < 1 || len(manifest.Entrypoints) > 16 ||
		len(manifest.Dependencies) > 128 || len(manifest.CapabilityRequests) > 64 ||
		len(manifest.EgressRequests) > 32 || len(manifest.SecretSlots) > 32 {
		return false
	}
	seen := map[string]struct{}{}
	for _, entrypoint := range manifest.Entrypoints {
		if !validIdentifier(entrypoint.Name) || len(entrypoint.Argv) < 1 || len(entrypoint.Argv) > 32 ||
			(entrypoint.WorkingDirectory != "/workspace" && entrypoint.WorkingDirectory != "/scratch") {
			return false
		}
		if _, exists := seen[entrypoint.Name]; exists {
			return false
		}
		seen[entrypoint.Name] = struct{}{}
		for _, argument := range entrypoint.Argv {
			if argument == "" || len(argument) > 512 || strings.ContainsAny(argument, "\x00\r\n") {
				return false
			}
		}
	}
	seen = map[string]struct{}{}
	for _, dependency := range manifest.Dependencies {
		if !validIdentifier(dependency.Name) || !digestPattern.MatchString(dependency.Digest) {
			return false
		}
		if _, exists := seen[dependency.Name]; exists {
			return false
		}
		seen[dependency.Name] = struct{}{}
	}
	seen = map[string]struct{}{}
	for _, request := range manifest.CapabilityRequests {
		if !validIdentifier(request.Capability) || len(request.Actions) < 1 || len(request.Actions) > 32 ||
			!validReason(request.Reason) || !uniqueIdentifiers(request.Actions) {
			return false
		}
		if _, exists := seen[request.Capability]; exists {
			return false
		}
		seen[request.Capability] = struct{}{}
	}
	seen = map[string]struct{}{}
	for _, request := range manifest.EgressRequests {
		if !validIdentifier(request.ID) || (request.Mode != "allowlist" && request.Mode != "brokered") ||
			len(request.Schemes) < 1 || len(request.Schemes) > 2 || len(request.Hosts) < 1 ||
			len(request.Hosts) > 32 || len(request.Ports) < 1 || len(request.Ports) > 8 || !validReason(request.Reason) {
			return false
		}
		if _, exists := seen[request.ID]; exists || !uniqueStrings(request.Schemes) ||
			!uniqueStrings(request.Hosts) || !uniqueInts(request.Ports) {
			return false
		}
		seen[request.ID] = struct{}{}
		for _, scheme := range request.Schemes {
			if scheme != "https" && scheme != "wss" {
				return false
			}
		}
		for _, host := range request.Hosts {
			if !validDNSName(host) {
				return false
			}
		}
		for _, port := range request.Ports {
			if port < 1 || port > 65535 {
				return false
			}
		}
	}
	seen = map[string]struct{}{}
	for _, slot := range manifest.SecretSlots {
		if !validIdentifier(slot.Name) || !validReason(slot.Purpose) || slot.Delivery != "broker_handle" {
			return false
		}
		if _, exists := seen[slot.Name]; exists {
			return false
		}
		seen[slot.Name] = struct{}{}
	}
	resources, limits := manifest.Resources, manifest.Limits
	return resources.CPUMillis >= 100 && resources.CPUMillis <= 8000 &&
		resources.MemoryMiB >= 64 && resources.MemoryMiB <= 16384 &&
		resources.PIDs >= 8 && resources.PIDs <= 1024 &&
		resources.WallSecond >= 1 && resources.WallSecond <= 86400 &&
		limits.MaxPackageFiles >= 1 && limits.MaxPackageFiles <= 10000 &&
		limits.MaxPackageBytes >= 1 && limits.MaxPackageBytes <= 1073741824 &&
		limits.MaxExpandedBytes >= 1 && limits.MaxExpandedBytes <= 4294967296 &&
		limits.MaxStdoutBytes >= 1 && limits.MaxStdoutBytes <= 16777216 &&
		limits.MaxStderrBytes >= 1 && limits.MaxStderrBytes <= 16777216 &&
		limits.MaxArtifactBytes >= 1 && limits.MaxArtifactBytes <= 1073741824
}

func AdmissionEligible(source ArchiveSource, validated ValidatedPackage) bool {
	if !validated.Package.HasRuntime {
		return true
	}
	manifest := validated.Package.Manifest
	if !source.ExecutableAllowed || source.Type != SourceOfficial || manifest == nil ||
		len(manifest.EgressRequests) != 0 || len(manifest.SecretSlots) != 0 || len(manifest.CapabilityRequests) == 0 {
		return false
	}
	for _, request := range manifest.CapabilityRequests {
		if request.Capability != "workspace.read" {
			return false
		}
		for _, action := range request.Actions {
			if action != "list" && action != "read" {
				return false
			}
		}
	}
	return true
}

func validIdentifier(value string) bool {
	return len(value) <= 64 && identifierPattern.MatchString(value)
}

func validReason(value string) bool {
	return value != "" && len([]rune(value)) <= 280 && validPlainText(value)
}

func validPlainText(value string) bool {
	if !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) && character != '\t' {
			return false
		}
	}
	return true
}

func uniqueIdentifiers(values []string) bool {
	seen := map[string]struct{}{}
	for _, value := range values {
		if !validIdentifier(value) {
			return false
		}
		if _, exists := seen[value]; exists {
			return false
		}
		seen[value] = struct{}{}
	}
	return true
}

func uniqueStrings(values []string) bool {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if _, exists := seen[value]; exists {
			return false
		}
		seen[value] = struct{}{}
	}
	return true
}

func uniqueInts(values []int) bool {
	seen := make(map[int]struct{}, len(values))
	for _, value := range values {
		if _, exists := seen[value]; exists {
			return false
		}
		seen[value] = struct{}{}
	}
	return true
}

func validDNSName(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" || len(value) > 253 || value == "localhost" || netIPLike(value) {
		return false
	}
	for _, label := range strings.Split(value, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, char := range label {
			if !(char >= 'a' && char <= 'z') && !(char >= '0' && char <= '9') && char != '-' {
				return false
			}
		}
	}
	return true
}

func netIPLike(value string) bool {
	parts := strings.Split(value, ".")
	if len(parts) != 4 {
		return strings.Contains(value, ":")
	}
	for _, part := range parts {
		if _, err := strconv.Atoi(part); err != nil {
			return false
		}
	}
	return true
}
