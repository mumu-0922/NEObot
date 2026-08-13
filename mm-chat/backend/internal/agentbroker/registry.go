package agentbroker

import (
	"encoding/json"
	"sort"
	"time"
)

var childForbiddenTools = map[string]struct{}{
	"delegate_task": {}, "cron_manage": {}, "grant_manage": {},
	"secret_manage": {}, "runtime_manage": {},
}

func BuildRegistry(catalog []ToolDefinition, requested []string, grant CapabilityGrant, now time.Time) (ToolRegistry, error) {
	if validateGrant(grant, now) != nil || len(catalog) > 512 || len(requested) > 128 {
		return ToolRegistry{}, ErrGrantDenied
	}
	if grant.Budget.MaxToolCalls == 0 {
		return ToolRegistry{}, ErrBudgetExhausted
	}
	requestSet := make(map[string]struct{}, len(requested))
	for _, identity := range requested {
		if !validIdentifier(identity) {
			return ToolRegistry{}, ErrInvalidInput
		}
		if _, duplicate := requestSet[identity]; duplicate {
			return ToolRegistry{}, ErrReplayDetected
		}
		requestSet[identity] = struct{}{}
	}
	catalogSet := map[string]ToolDefinition{}
	for _, definition := range catalog {
		if !validIdentifier(definition.Identity) || !validIdentifier(definition.Capability) ||
			!member(definition.Classification, ClassificationRead, ClassificationMutable, ClassificationUnknown) {
			return ToolRegistry{}, ErrInvalidInput
		}
		actions, ok := uniqueSorted(definition.Actions, 32)
		if !ok || definition.Classification != ClassificationRead && definition.Idempotent {
			return ToolRegistry{}, ErrInvalidInput
		}
		definition.Actions = actions
		if _, duplicate := catalogSet[definition.Identity]; duplicate {
			return ToolRegistry{}, ErrReplayDetected
		}
		catalogSet[definition.Identity] = definition
	}
	registry := ToolRegistry{SchemaVersion: RegistryVersion, Subject: grant.Subject,
		RunID: grant.Run.RunID, Depth: grant.Run.Depth, GrantID: grant.GrantID,
		PackageFingerprint: grant.PackageFingerprint, RuntimeBundleFingerprint: grant.RuntimeBundleFingerprint,
		ExpiresAt: grant.ExpiresAt.UTC(), Egress: grant.Egress,
		Secrets: append([]SecretGrant(nil), grant.Secrets...), Budget: grant.Budget, Tools: []RegistryTool{}}
	for identity := range requestSet {
		if grant.Run.Depth == 1 {
			if _, forbidden := childForbiddenTools[identity]; forbidden {
				continue
			}
		}
		definition, ok := catalogSet[identity]
		if !ok {
			return ToolRegistry{}, ErrGrantDenied
		}
		matched := false
		for _, capability := range grant.Capabilities {
			if capability.Capability != definition.Capability || capability.Approval == ApprovalDenied || capability.MaxCalls == 0 {
				continue
			}
			actions := intersection(definition.Actions, capability.Actions)
			if len(actions) == 0 {
				continue
			}
			resources := Selector{Kind: capability.Resources.Kind, Values: append([]string(nil), capability.Resources.Values...)}
			sort.Strings(resources.Values)
			registry.Tools = append(registry.Tools, RegistryTool{Identity: identity,
				Capability: definition.Capability, Actions: actions, Resources: resources,
				Approval: capability.Approval, MaxCalls: capability.MaxCalls,
				Classification: definition.Classification, Idempotent: definition.Idempotent})
			matched = true
			break
		}
		if !matched {
			return ToolRegistry{}, ErrGrantDenied
		}
	}
	sort.Slice(registry.Tools, func(i, j int) bool { return registry.Tools[i].Identity < registry.Tools[j].Identity })
	fingerprintValue, err := registryFingerprint(registry)
	if err != nil {
		return ToolRegistry{}, err
	}
	registry.Fingerprint = fingerprintValue
	return registry, nil
}

func registryFingerprint(registry ToolRegistry) (string, error) {
	encoded, err := json.Marshal(struct {
		SchemaVersion            string         `json:"schemaVersion"`
		Subject                  Subject        `json:"subject"`
		RunID                    string         `json:"runId"`
		Depth                    int            `json:"depth"`
		GrantID                  string         `json:"grantId"`
		PackageFingerprint       string         `json:"packageFingerprint"`
		RuntimeBundleFingerprint string         `json:"runtimeBundleFingerprint"`
		ExpiresAt                string         `json:"expiresAt"`
		Egress                   EgressPolicy   `json:"egress"`
		Secrets                  []SecretGrant  `json:"secrets"`
		Budget                   Budget         `json:"budget"`
		Tools                    []RegistryTool `json:"tools"`
	}{registry.SchemaVersion, registry.Subject, registry.RunID, registry.Depth, registry.GrantID,
		registry.PackageFingerprint, registry.RuntimeBundleFingerprint,
		registry.ExpiresAt.UTC().Format(time.RFC3339Nano), registry.Egress, registry.Secrets,
		registry.Budget, registry.Tools})
	if err != nil {
		return "", ErrInvalidInput
	}
	return fingerprint("neo-tool-registry-v1", encoded), nil
}

func intersection(left, right []string) []string {
	set := make(map[string]struct{}, len(right))
	for _, value := range right {
		set[value] = struct{}{}
	}
	result := make([]string, 0, len(left))
	for _, value := range left {
		if _, ok := set[value]; ok {
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}

func RegistryToolFor(registry ToolRegistry, identity, action, resource string) (RegistryTool, error) {
	expected, err := registryFingerprint(registry)
	if err != nil || registry.SchemaVersion != RegistryVersion || expected != registry.Fingerprint ||
		registry.Subject.UserID == "" || !validateSubject(registry.Subject) ||
		!validID(registry.RunID, "run") || !validID(registry.GrantID, "grant") ||
		registry.Depth < 0 || registry.Depth > 1 || len(registry.Tools) > 128 {
		return RegistryTool{}, ErrSnapshotMismatch
	}
	for index, tool := range registry.Tools {
		if !validIdentifier(tool.Identity) || !validIdentifier(tool.Capability) ||
			index > 0 && registry.Tools[index-1].Identity >= tool.Identity {
			return RegistryTool{}, ErrSnapshotMismatch
		}
	}
	if registry.Depth == 1 {
		for _, tool := range registry.Tools {
			if _, forbidden := childForbiddenTools[tool.Identity]; forbidden {
				return RegistryTool{}, ErrGrantDenied
			}
		}
	}
	for _, tool := range registry.Tools {
		if tool.Identity != identity {
			continue
		}
		for _, allowed := range tool.Actions {
			if allowed == action && resourceAllowed(tool.Resources, resource) {
				return tool, nil
			}
		}
		return RegistryTool{}, ErrGrantDenied
	}
	return RegistryTool{}, ErrGrantDenied
}
