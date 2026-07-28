package usermemory

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"neo-chat/mm-chat/backend/internal/auth"
)

const (
	PortabilityPlanTokenVersion = 1
	PortabilityPlanTTL          = 10 * time.Minute
	maximumPlanTokenBytes       = 4096
	portabilityPlanDomain       = "mm-chat.memory-portability-plan.v1\n"
)

var portabilityTokenEncoding = base64.RawURLEncoding.Strict()

type PortabilityPlanKeyring struct {
	ActiveKeyID string
	Keys        map[string][]byte
}

type PortabilityPlanCodec struct {
	activeKeyID string
	keys        map[string][]byte
}

type PortabilityPlanToken struct {
	Version            int    `json:"v"`
	KeyID              string `json:"k"`
	UserID             string `json:"u"`
	ImportID           string `json:"i"`
	PackageSHA256      string `json:"p"`
	ManifestSHA256     string `json:"m"`
	MappingsSHA256     string `json:"g"`
	PlanSHA256         string `json:"h"`
	AuthorityStateHash string `json:"s"`
	IssuedAt           int64  `json:"a"`
	ExpiresAt          int64  `json:"e"`
}

type ImportMappings struct {
	Projects      map[string]ImportProjectMapping      `json:"projects"`
	Conversations map[string]ImportConversationMapping `json:"conversations"`
}

type ImportProjectMapping struct {
	Mode      string `json:"mode"`
	ProjectID string `json:"projectId,omitempty"`
}

type ImportConversationMapping struct {
	Mode           string `json:"mode"`
	ConversationID string `json:"conversationId,omitempty"`
	ProjectID      string `json:"projectId,omitempty"`
	ProjectRef     string `json:"projectRef,omitempty"`
}

type ResolvedImportScope struct {
	Type            string
	ProjectID       string
	ConversationID  string
	ScopeGeneration int64
	Skipped         bool
}

type ImportMemoryResolutionInput struct {
	NormalizedContent string
	SubjectKey        string
	FactKey           string
	Scope             ResolvedImportScope
}

type ImportMemoryResolution struct {
	Result             string
	ReasonCode         string
	CurrentMemoryID    string
	CurrentRevision    int64
	CurrentContentHash string
}

type PortabilityApplyMetadata struct {
	ImportID           string
	PackageSHA256      string
	ManifestSHA256     string
	MappingsSHA256     string
	PlanSHA256         string
	AuthorityStateHash string
	ProjectCount       int
	MemoryCount        int
	RevisionCount      int
}

type PortabilityApplyProject struct {
	ID              string
	Name            string
	Description     string
	LifecycleStatus string
}

type PortabilityApplyMemory struct {
	ID     string
	Record PortableMemoryRecord
	Scope  ResolvedImportScope
}

type PortabilityApplyRevision struct {
	MemoryID             string
	Record               PortableRevisionRecord
	Scope                ResolvedImportScope
	SupersededByMemoryID string
}

type PortabilityApplyFinalState struct {
	MemoryID             string
	LifecycleStatus      string
	SupersededByMemoryID string
}

type PortabilityApplySink interface {
	CreateProject(PortabilityApplyProject) error
	AddMemory(PortabilityApplyMemory) error
	AddRevision(PortabilityApplyRevision) error
	FinalizeMemory(PortabilityApplyFinalState) error
}

type PortabilityApplyResult struct {
	ImportID      string `json:"importId"`
	Status        string `json:"status"`
	AddedProjects int    `json:"addedProjects"`
	AddedMemories int    `json:"addedMemories"`
	ImportedAt    int64  `json:"importedAt"`
}

type PortabilityRepository interface {
	PortabilityAuthorityState(context.Context) (string, error)
	ResolveImportProject(context.Context, string) (MemoryProject, error)
	ResolveImportConversation(context.Context, string) (ConversationMemoryPolicy, error)
	ResolveImportMemory(context.Context, ImportMemoryResolutionInput) (ImportMemoryResolution, error)
	CompletedPortabilityImport(
		context.Context,
		PortabilityApplyMetadata,
	) (PortabilityApplyResult, bool, error)
	ApplyPortabilityImport(
		context.Context,
		PortabilityApplyMetadata,
		func(PortabilityApplySink) error,
	) (PortabilityApplyResult, error)
}

type ImportPlanItem struct {
	Ordinal         int    `json:"ordinal"`
	MemoryRef       string `json:"memoryRef"`
	RecordHash      string `json:"recordHash"`
	Result          string `json:"result"`
	ReasonCode      string `json:"reasonCode"`
	CurrentHash     string `json:"currentHash,omitempty"`
	CurrentMemoryID string `json:"-"`
}

type ImportScopeRequirement struct {
	Kind        string `json:"kind"`
	PortableRef string `json:"portableRef"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
}

type ImportDryRunResult struct {
	ImportID           string                   `json:"importId"`
	PackageSHA256      string                   `json:"packageSha256"`
	ManifestSHA256     string                   `json:"manifestSha256"`
	PlanSHA256         string                   `json:"planSha256"`
	PlanToken          string                   `json:"planToken"`
	ExpiresAt          int64                    `json:"expiresAt"`
	Counts             map[string]int           `json:"counts"`
	Items              []ImportPlanItem         `json:"items"`
	ScopeRequirements  []ImportScopeRequirement `json:"scopeRequirements"`
	SettingsSuggestion *Settings                `json:"settingsSuggestion,omitempty"`
}

type importPlanHashPayload struct {
	ImportID           string                   `json:"importId"`
	PackageSHA256      string                   `json:"packageSha256"`
	ManifestSHA256     string                   `json:"manifestSha256"`
	MappingsSHA256     string                   `json:"mappingsSha256"`
	AuthorityStateHash string                   `json:"authorityStateHash"`
	Counts             PortableRecordCounts     `json:"counts"`
	Items              []ImportPlanItem         `json:"items"`
	ScopeRequirements  []ImportScopeRequirement `json:"scopeRequirements"`
}

func NewPortabilityPlanCodec(ring PortabilityPlanKeyring) (*PortabilityPlanCodec, error) {
	activeKeyID := strings.TrimSpace(ring.ActiveKeyID)
	if activeKeyID == "" || len(ring.Keys) == 0 {
		return nil, errors.New("memory portability plan keyring is required")
	}
	keys := make(map[string][]byte, len(ring.Keys))
	for keyID, key := range ring.Keys {
		keyID = strings.TrimSpace(keyID)
		if keyID == "" || len(key) < sha256.Size {
			return nil, errors.New("memory portability plan keys must have ids and at least 32 bytes")
		}
		keys[keyID] = append([]byte(nil), key...)
	}
	if _, ok := keys[activeKeyID]; !ok {
		return nil, errors.New("memory portability active plan key is not configured")
	}
	return &PortabilityPlanCodec{activeKeyID: activeKeyID, keys: keys}, nil
}

func (c *PortabilityPlanCodec) Encode(token PortabilityPlanToken) (string, error) {
	if c == nil {
		return "", errors.New("memory portability plan codec is required")
	}
	if token.Version == 0 {
		token.Version = PortabilityPlanTokenVersion
	}
	if token.KeyID == "" {
		token.KeyID = c.activeKeyID
	}
	if err := validatePortabilityPlanToken(token); err != nil {
		return "", err
	}
	if token.KeyID != c.activeKeyID {
		return "", errors.New("memory portability token must use the active key")
	}
	payload, err := json.Marshal(token)
	if err != nil {
		return "", fmt.Errorf("marshal memory portability plan token: %w", err)
	}
	signature, err := c.sign(token.KeyID, payload)
	if err != nil {
		return "", err
	}
	encoded := portabilityTokenEncoding.EncodeToString(payload) + "." +
		portabilityTokenEncoding.EncodeToString(signature)
	if len(encoded) > maximumPlanTokenBytes {
		return "", errors.New("memory portability plan token is too large")
	}
	return encoded, nil
}

func (c *PortabilityPlanCodec) Decode(encoded string) (PortabilityPlanToken, error) {
	var token PortabilityPlanToken
	if c == nil {
		return token, errors.New("memory portability plan codec is required")
	}
	encoded = strings.TrimSpace(encoded)
	if encoded == "" || len(encoded) > maximumPlanTokenBytes {
		return token, validation("MEMORY_IMPORT_PLAN_TOKEN_INVALID", "memory import plan token is invalid")
	}
	left, right, ok := strings.Cut(encoded, ".")
	if !ok || left == "" || right == "" || strings.Contains(right, ".") {
		return token, validation("MEMORY_IMPORT_PLAN_TOKEN_INVALID", "memory import plan token is invalid")
	}
	payload, err := portabilityTokenEncoding.DecodeString(left)
	if err != nil {
		return token, validation("MEMORY_IMPORT_PLAN_TOKEN_INVALID", "memory import plan token is invalid")
	}
	signature, err := portabilityTokenEncoding.DecodeString(right)
	if err != nil {
		return token, validation("MEMORY_IMPORT_PLAN_TOKEN_INVALID", "memory import plan token is invalid")
	}
	if err := json.Unmarshal(payload, &token); err != nil {
		return token, validation("MEMORY_IMPORT_PLAN_TOKEN_INVALID", "memory import plan token is invalid")
	}
	key, ok := c.keys[token.KeyID]
	if !ok {
		return token, validation("MEMORY_IMPORT_PLAN_TOKEN_INVALID", "memory import plan token is invalid")
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(portabilityPlanDomain))
	_, _ = mac.Write(payload)
	if subtle.ConstantTimeCompare(signature, mac.Sum(nil)) != 1 ||
		validatePortabilityPlanToken(token) != nil {
		return token, validation("MEMORY_IMPORT_PLAN_TOKEN_INVALID", "memory import plan token is invalid")
	}
	return token, nil
}

func (c *PortabilityPlanCodec) sign(keyID string, payload []byte) ([]byte, error) {
	key, ok := c.keys[keyID]
	if !ok {
		return nil, errors.New("memory portability signing key is not configured")
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(portabilityPlanDomain))
	_, _ = mac.Write(payload)
	return mac.Sum(nil), nil
}

func validatePortabilityPlanToken(token PortabilityPlanToken) error {
	if token.Version != PortabilityPlanTokenVersion || strings.TrimSpace(token.KeyID) == "" ||
		!uuidRE.MatchString(token.UserID) || !uuidRE.MatchString(token.ImportID) ||
		!isLowerSHA256(token.PackageSHA256) || !isLowerSHA256(token.ManifestSHA256) ||
		!isLowerSHA256(token.MappingsSHA256) || !isLowerSHA256(token.PlanSHA256) ||
		!isLowerSHA256(token.AuthorityStateHash) || token.IssuedAt <= 0 ||
		token.ExpiresAt <= token.IssuedAt {
		return errors.New("memory portability plan token fields are invalid")
	}
	return nil
}

func normalizeImportMappings(input ImportMappings) (ImportMappings, error) {
	result := ImportMappings{
		Projects:      make(map[string]ImportProjectMapping, len(input.Projects)),
		Conversations: make(map[string]ImportConversationMapping, len(input.Conversations)),
	}
	for ref, mapping := range input.Projects {
		ref = strings.TrimSpace(ref)
		mapping.Mode = strings.ToLower(strings.TrimSpace(mapping.Mode))
		mapping.ProjectID = strings.TrimSpace(mapping.ProjectID)
		if !portableRef(ref, "project") ||
			(mapping.Mode != "existing" && mapping.Mode != "create" && mapping.Mode != "skip") ||
			(mapping.Mode == "existing" && !uuidRE.MatchString(mapping.ProjectID)) ||
			(mapping.Mode != "existing" && mapping.ProjectID != "") {
			return ImportMappings{}, validation("MEMORY_IMPORT_MAPPING_INVALID", "memory import project mapping is invalid")
		}
		if _, duplicate := result.Projects[ref]; duplicate {
			return ImportMappings{}, validation("MEMORY_IMPORT_MAPPING_INVALID", "memory import project mapping is duplicated")
		}
		result.Projects[ref] = mapping
	}
	for ref, mapping := range input.Conversations {
		ref = strings.TrimSpace(ref)
		mapping.Mode = strings.ToLower(strings.TrimSpace(mapping.Mode))
		mapping.ConversationID = strings.TrimSpace(mapping.ConversationID)
		mapping.ProjectID = strings.TrimSpace(mapping.ProjectID)
		mapping.ProjectRef = strings.TrimSpace(mapping.ProjectRef)
		if !portableRef(ref, "conversation") {
			return ImportMappings{}, validation("MEMORY_IMPORT_MAPPING_INVALID", "memory import conversation mapping is invalid")
		}
		valid := false
		switch mapping.Mode {
		case "existing":
			valid = uuidRE.MatchString(mapping.ConversationID) && mapping.ProjectID == "" && mapping.ProjectRef == ""
		case "global", "skip":
			valid = mapping.ConversationID == "" && mapping.ProjectID == "" && mapping.ProjectRef == ""
		case "project":
			valid = mapping.ConversationID == "" &&
				((uuidRE.MatchString(mapping.ProjectID) && mapping.ProjectRef == "") ||
					(mapping.ProjectID == "" && portableRef(mapping.ProjectRef, "project")))
		}
		if !valid {
			return ImportMappings{}, validation("MEMORY_IMPORT_MAPPING_INVALID", "memory import conversation mapping is invalid")
		}
		if _, duplicate := result.Conversations[ref]; duplicate {
			return ImportMappings{}, validation("MEMORY_IMPORT_MAPPING_INVALID", "memory import conversation mapping is duplicated")
		}
		result.Conversations[ref] = mapping
	}
	return result, nil
}

func canonicalMappingsHash(mappings ImportMappings) (string, error) {
	normalized, err := normalizeImportMappings(mappings)
	if err != nil {
		return "", err
	}
	return hashCanonicalJSON(normalized)
}

func hashSeekable(source io.ReadSeeker) (string, error) {
	if source == nil {
		return "", errors.New("memory package is required")
	}
	if _, err := source.Seek(0, io.SeekStart); err != nil {
		return "", fmt.Errorf("seek memory package: %w", err)
	}
	hasher := sha256.New()
	if _, err := io.Copy(hasher, source); err != nil {
		return "", fmt.Errorf("hash memory package: %w", err)
	}
	if _, err := source.Seek(0, io.SeekStart); err != nil {
		return "", fmt.Errorf("rewind memory package: %w", err)
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func deterministicImportUUID(importID, portableRef, kind string) (string, error) {
	if !uuidRE.MatchString(importID) || strings.TrimSpace(portableRef) == "" || strings.TrimSpace(kind) == "" {
		return "", errors.New("deterministic import UUID input is invalid")
	}
	sum := sha256.Sum256([]byte("mm-chat.memory-import-id.v1\n" + kind + "\n" + importID + "\n" + portableRef))
	value := append([]byte(nil), sum[:16]...)
	value[6] = (value[6] & 0x0f) | 0x50
	value[8] = (value[8] & 0x3f) | 0x80
	return fmt.Sprintf(
		"%08x-%04x-%04x-%04x-%012x",
		value[0:4], value[4:6], value[6:8], value[8:10], value[10:16],
	), nil
}

func currentPortabilityUserID(ctx context.Context) (string, error) {
	user, ok := auth.UserFromContext(ctx)
	if !ok || !uuidRE.MatchString(user.ID) {
		return "", validation("MEMORY_IMPORT_USER_REQUIRED", "authenticated memory import user is required")
	}
	return user.ID, nil
}
