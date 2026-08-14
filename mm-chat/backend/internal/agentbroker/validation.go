package agentbroker

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"sort"
	"strings"
	"time"

	"neo-chat/mm-chat/backend/internal/strictjson"
)

const maxArgumentsBytes = 256 << 10

var (
	identifierPattern  = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[._-][a-z0-9]+)*$`)
	prefixedIDPattern  = regexp.MustCompile(`^(run|step|attempt|grant|intent|approval|cancellation|request|artifact|user|project|assistant)_[a-z0-9]{8,64}$`)
	fingerprintPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
	uuidPattern        = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	leasePattern       = regexp.MustCompile(`^lease_[A-Za-z0-9_-]{24,128}$`)
	revisionPattern    = regexp.MustCompile(`^(?:rev_[A-Za-z0-9_-]{8,128}|sha256:[a-f0-9]{64})$`)
	secretRefPattern   = regexp.MustCompile(`^secret_ref_[a-z0-9]{16,64}$`)
	reasonPattern      = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,63}$`)
)

func canonicalJSON(body []byte, maximum int) ([]byte, error) {
	var value any
	if err := strictjson.Decode(body, maximum, &value); err != nil {
		return nil, ErrInvalidInput
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return nil, ErrInvalidInput
	}
	return canonical, nil
}

func fingerprint(domain string, value []byte) string {
	digest := sha256.Sum256(append(append([]byte(nil), []byte(domain)...), append([]byte{0}, value...)...))
	return "sha256:" + hex.EncodeToString(digest[:])
}

func tokenDigest(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func randomID(prefix string, bytesCount int) (string, error) {
	value := make([]byte, bytesCount)
	if _, err := rand.Read(value); err != nil {
		return "", ErrExecutorUnavailable
	}
	return prefix + "_" + strings.ToLower(hex.EncodeToString(value)), nil
}

func randomCommitKey() (string, error) {
	value := make([]byte, 24)
	if _, err := rand.Read(value); err != nil {
		return "", ErrExecutorUnavailable
	}
	return "commit_" + base64.RawURLEncoding.EncodeToString(value), nil
}

func validID(value, prefix string) bool {
	return strings.HasPrefix(value, prefix+"_") && prefixedIDPattern.MatchString(value)
}

func validIdentifier(value string) bool {
	return len(value) <= 64 && identifierPattern.MatchString(value)
}

func validFingerprint(value string) bool { return fingerprintPattern.MatchString(value) }

func member(value string, candidates ...string) bool {
	for _, candidate := range candidates {
		if value == candidate {
			return true
		}
	}
	return false
}

func uniqueSorted(values []string, limit int) ([]string, bool) {
	if len(values) == 0 || len(values) > limit {
		return nil, false
	}
	result := append([]string(nil), values...)
	sort.Strings(result)
	for index, value := range result {
		if !validIdentifier(value) || index > 0 && result[index-1] == value {
			return nil, false
		}
	}
	return result, true
}

func validateAttempt(value AttemptAuthority) error {
	if !uuidPattern.MatchString(value.UserID) || !validID(value.RunID, "run") ||
		!validID(value.StepID, "step") || !validID(value.AttemptID, "attempt") ||
		value.Generation < 1 || value.LeaseOwner == "" || len(value.LeaseOwner) > 128 ||
		!leasePattern.MatchString(value.LeaseToken) || !validFingerprint(value.SnapshotFingerprint) ||
		!validFingerprint(value.GrantFingerprint) || !validFingerprint(value.RegistryFingerprint) ||
		value.KillSwitchEpoch < 0 {
		return ErrInvalidInput
	}
	return nil
}

func validateSubject(value Subject) bool {
	return uuidPattern.MatchString(value.UserID) && validID(value.ProjectID, "project") &&
		validID(value.AssistantID, "assistant")
}

func validateGrant(value CapabilityGrant, now time.Time) error {
	if value.SchemaVersion != GrantVersion || !validID(value.GrantID, "grant") ||
		!validateSubject(value.Subject) || !validID(value.Run.RunID, "run") ||
		value.Run.Depth < 0 || value.Run.Depth > 1 ||
		(value.Run.Depth == 0 && value.Run.ParentRunID != "") ||
		(value.Run.Depth == 1 && !validID(value.Run.ParentRunID, "run")) ||
		!validFingerprint(value.PackageFingerprint) || !validFingerprint(value.RuntimeBundleFingerprint) ||
		value.IssuedAt.IsZero() || value.IssuedAt.After(now) || !value.ExpiresAt.After(value.IssuedAt) || !now.Before(value.ExpiresAt) ||
		len(value.Capabilities) == 0 || len(value.Capabilities) > 128 ||
		value.Budget.MaxWallSeconds < 0 || value.Budget.MaxWallSeconds > 86400 ||
		value.Budget.MaxModelTokens < 0 || value.Budget.MaxModelTokens > 10000000 ||
		value.Budget.MaxToolCalls < 0 || value.Budget.MaxToolCalls > 10000 ||
		value.Budget.MaxArtifactBytes < 0 || value.Budget.MaxArtifactBytes > 10<<30 ||
		!member(value.Egress.Mode, "none", "allowlist", "brokered") || len(value.Egress.Rules) > 64 ||
		len(value.Secrets) > 32 {
		return ErrGrantDenied
	}
	seen := map[string]struct{}{}
	for capabilityIndex, capability := range value.Capabilities {
		if capabilityIndex > 0 && value.Capabilities[capabilityIndex-1].Capability >= capability.Capability {
			return ErrGrantDenied
		}
		if !validIdentifier(capability.Capability) || capability.MaxCalls < 0 || capability.MaxCalls > 10000 ||
			!member(capability.Approval, ApprovalAutomatic, ApprovalOnce, ApprovalPerCommit, ApprovalDenied) {
			return ErrGrantDenied
		}
		actions, ok := uniqueSorted(capability.Actions, 32)
		if !ok || len(capability.Resources.Values) == 0 || len(capability.Resources.Values) > 128 ||
			!member(capability.Resources.Kind, "exact", "prefix") {
			return ErrGrantDenied
		}
		for index := range actions {
			if actions[index] != capability.Actions[index] {
				return ErrGrantDenied
			}
		}
		resources := append([]string(nil), capability.Resources.Values...)
		for index, resource := range resources {
			if resource == "" || len(resource) > 256 || strings.ContainsAny(resource, "\x00\r\n") ||
				index > 0 && resources[index-1] >= resource ||
				(capability.Resources.Kind == "prefix" && index > 0 && strings.HasPrefix(resource, resources[index-1])) {
				return ErrGrantDenied
			}
		}
		if _, duplicate := seen[capability.Capability]; duplicate {
			return ErrGrantDenied
		}
		seen[capability.Capability] = struct{}{}
	}
	if value.Egress.Mode == "none" && len(value.Egress.Rules) != 0 {
		return ErrGrantDenied
	}
	for index, rule := range value.Egress.Rules {
		host := strings.ToLower(strings.TrimSuffix(rule.Host, "."))
		if !member(rule.Scheme, "https", "wss") || host != rule.Host || !validEgressHost(host) ||
			len(rule.Ports) == 0 || len(rule.Ports) > 8 {
			return ErrGrantDenied
		}
		if index > 0 {
			previous := value.Egress.Rules[index-1]
			previousKey, key := previous.Scheme+"\x00"+previous.Host, rule.Scheme+"\x00"+rule.Host
			if previousKey >= key {
				return ErrGrantDenied
			}
		}
		for portIndex, port := range rule.Ports {
			if port < 1 || port > 65535 || portIndex > 0 && rule.Ports[portIndex-1] >= port {
				return ErrGrantDenied
			}
		}
	}
	secretSlots := map[string]struct{}{}
	for index, secret := range value.Secrets {
		if !validIdentifier(secret.Slot) || !secretRefPattern.MatchString(secret.BrokerRef) ||
			secret.TTLSeconds < 1 || secret.TTLSeconds > 3600 {
			return ErrGrantDenied
		}
		if index > 0 && value.Secrets[index-1].Slot >= secret.Slot {
			return ErrGrantDenied
		}
		actions, ok := uniqueSorted(secret.Actions, 32)
		if !ok {
			return ErrGrantDenied
		}
		for actionIndex := range actions {
			if actions[actionIndex] != secret.Actions[actionIndex] {
				return ErrGrantDenied
			}
		}
		if _, duplicate := secretSlots[secret.Slot]; duplicate {
			return ErrGrantDenied
		}
		secretSlots[secret.Slot] = struct{}{}
	}
	return nil
}

func validEgressHost(host string) bool {
	if host == "" || len(host) > 253 || strings.ContainsAny(host, "\x00\r\n:/[]@") ||
		host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, char := range label {
			if char != '-' && (char < 'a' || char > 'z') && (char < '0' || char > '9') {
				return false
			}
		}
	}
	return true
}

func canonicalGrantFingerprint(grant CapabilityGrant) (string, error) {
	encoded, err := json.Marshal(grant)
	if err != nil {
		return "", ErrInvalidInput
	}
	canonical, err := canonicalJSON(encoded, 512<<10)
	if err != nil {
		return "", err
	}
	return fingerprint("neo-capability-grant-v1", canonical), nil
}

func resourceAllowed(selector Selector, resource string) bool {
	if resource == "" || len(resource) > 256 || strings.ContainsAny(resource, "\x00\r\n") {
		return false
	}
	for _, value := range selector.Values {
		if value == "" || len(value) > 256 {
			continue
		}
		if selector.Kind == "exact" && resource == value ||
			selector.Kind == "prefix" && strings.HasPrefix(resource, value) {
			return true
		}
	}
	return false
}
