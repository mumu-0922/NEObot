package usermemory

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	MemoryPackageFormat        = "mm-memory"
	MemoryPackageFormatVersion = 1
	MemoryPackageSchemaVersion = 1
	MaxMemoryPackageBytes      = 256 << 20
	MaxMemoryPackageProjects   = 1000
	MaxMemoryPackageMemories   = 50000
	MaxMemoryPackageRevisions  = 200000
	maxMemoryPackageLineBytes  = 64 << 10
)

var portableRefRE = regexp.MustCompile(
	`^(project|conversation|memory)-[0-9]{6}$`,
)

type PortableRecordCounts struct {
	Settings  int `json:"settings"`
	Projects  int `json:"projects"`
	Memories  int `json:"memories"`
	Revisions int `json:"revisions"`
}

type MemoryPackageManifest struct {
	Kind            string               `json:"kind"`
	Format          string               `json:"format"`
	FormatVersion   int                  `json:"formatVersion"`
	SchemaVersion   int                  `json:"schemaVersion"`
	CreatedAt       string               `json:"createdAt"`
	ExporterRelease string               `json:"exporterRelease"`
	IncludeHistory  bool                 `json:"includeHistory"`
	Counts          PortableRecordCounts `json:"counts"`
	RecordsSHA256   string               `json:"recordsSha256"`
}

type PortableSettingsRecord struct {
	Kind     string   `json:"kind"`
	Settings Settings `json:"settings"`
}

type PortableProjectRecord struct {
	Kind            string `json:"kind"`
	Ref             string `json:"ref"`
	Name            string `json:"name"`
	Description     string `json:"description"`
	LifecycleStatus string `json:"lifecycleStatus"`
}

type PortableMemoryScope struct {
	Type            string `json:"type"`
	ProjectRef      string `json:"projectRef,omitempty"`
	ConversationRef string `json:"conversationRef,omitempty"`
}

type PortableMemoryRecord struct {
	Kind              string              `json:"kind"`
	Ref               string              `json:"ref"`
	Revision          int64               `json:"revision"`
	Type              string              `json:"type"`
	Content           string              `json:"content"`
	ContentHash       string              `json:"contentHash"`
	Importance        int                 `json:"importance"`
	Tags              []string            `json:"tags"`
	OriginalAuthority string              `json:"originalAuthority"`
	Enabled           bool                `json:"enabled"`
	Scope             PortableMemoryScope `json:"scope"`
	LifecycleStatus   string              `json:"lifecycleStatus"`
	SupersededByRef   string              `json:"supersededByRef,omitempty"`
	Sensitivity       string              `json:"sensitivity"`
	SubjectKey        string              `json:"subjectKey,omitempty"`
	FactKey           string              `json:"factKey,omitempty"`
	Confidence        float64             `json:"confidence"`
	ObservedAt        string              `json:"observedAt"`
	ValidFrom         string              `json:"validFrom,omitempty"`
	ValidTo           string              `json:"validTo,omitempty"`
	ExpiresAt         string              `json:"expiresAt,omitempty"`
	CreatedAt         string              `json:"createdAt"`
	UpdatedAt         string              `json:"updatedAt"`
}

type PortableMemorySnapshot struct {
	Type            string              `json:"type"`
	Content         string              `json:"content"`
	ContentHash     string              `json:"contentHash"`
	Importance      int                 `json:"importance"`
	Tags            []string            `json:"tags"`
	Enabled         bool                `json:"enabled"`
	Scope           PortableMemoryScope `json:"scope"`
	LifecycleStatus string              `json:"lifecycleStatus"`
	SupersededByRef string              `json:"supersededByRef,omitempty"`
	Sensitivity     string              `json:"sensitivity"`
	SubjectKey      string              `json:"subjectKey,omitempty"`
	FactKey         string              `json:"factKey,omitempty"`
	Confidence      float64             `json:"confidence"`
	ObservedAt      string              `json:"observedAt"`
	ValidFrom       string              `json:"validFrom,omitempty"`
	ValidTo         string              `json:"validTo,omitempty"`
	ExpiresAt       string              `json:"expiresAt,omitempty"`
}

type PortableRevisionRecord struct {
	Kind           string                  `json:"kind"`
	MemoryRef      string                  `json:"memoryRef"`
	Revision       int64                   `json:"revision"`
	Operation      string                  `json:"operation"`
	OldContentHash string                  `json:"oldContentHash"`
	NewContentHash string                  `json:"newContentHash"`
	Prior          *PortableMemorySnapshot `json:"prior,omitempty"`
	Purged         bool                    `json:"purged"`
	CreatedAt      string                  `json:"createdAt"`
}

type ParsedMemoryPackage struct {
	Manifest       MemoryPackageManifest
	ManifestHash   string
	DecryptedBytes int64
}

type MemoryPackageVisitor interface {
	VisitSettings(int, PortableSettingsRecord) error
	VisitProject(int, PortableProjectRecord) error
	VisitMemory(int, PortableMemoryRecord) error
	VisitRevision(int, PortableRevisionRecord) error
}

type MemoryPackageVisitorFuncs struct {
	Settings func(int, PortableSettingsRecord) error
	Project  func(int, PortableProjectRecord) error
	Memory   func(int, PortableMemoryRecord) error
	Revision func(int, PortableRevisionRecord) error
}

func (v MemoryPackageVisitorFuncs) VisitSettings(ordinal int, record PortableSettingsRecord) error {
	if v.Settings == nil {
		return nil
	}
	return v.Settings(ordinal, record)
}

func (v MemoryPackageVisitorFuncs) VisitProject(ordinal int, record PortableProjectRecord) error {
	if v.Project == nil {
		return nil
	}
	return v.Project(ordinal, record)
}

func (v MemoryPackageVisitorFuncs) VisitMemory(ordinal int, record PortableMemoryRecord) error {
	if v.Memory == nil {
		return nil
	}
	return v.Memory(ordinal, record)
}

func (v MemoryPackageVisitorFuncs) VisitRevision(ordinal int, record PortableRevisionRecord) error {
	if v.Revision == nil {
		return nil
	}
	return v.Revision(ordinal, record)
}

type memoryRevisionChain struct {
	currentRevision int64
	currentHash     string
	nextRevision    int64
	previousNewHash string
	seen            int
	supersededByRef string
}

type countingReader struct {
	reader io.Reader
	count  int64
}

func (r *countingReader) Read(value []byte) (int, error) {
	read, err := r.reader.Read(value)
	r.count += int64(read)
	if r.count > MaxMemoryPackageBytes {
		return read, validation(
			"MEMORY_PACKAGE_TOO_LARGE",
			"decrypted memory package exceeds 256 MiB",
		)
	}
	return read, err
}

func ParseEncryptedMemoryPackage(
	source io.Reader,
	passphrase string,
	visitor MemoryPackageVisitor,
) (ParsedMemoryPackage, error) {
	plaintext, err := DecryptPortabilityStream(source, passphrase)
	if err != nil {
		return ParsedMemoryPackage{}, err
	}
	return ParseMemoryPackage(plaintext, visitor)
}

func ParseMemoryPackage(
	plaintext io.Reader,
	visitor MemoryPackageVisitor,
) (ParsedMemoryPackage, error) {
	if plaintext == nil {
		return ParsedMemoryPackage{}, errors.New("memory package plaintext is required")
	}
	if visitor == nil {
		visitor = MemoryPackageVisitorFuncs{}
	}
	counter := &countingReader{reader: plaintext}
	scanner := bufio.NewScanner(counter)
	scanner.Buffer(make([]byte, 4096), maxMemoryPackageLineBytes)

	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return ParsedMemoryPackage{}, mapMemoryPackageReadError(err)
		}
		return ParsedMemoryPackage{}, validation(
			"MEMORY_PACKAGE_MANIFEST_REQUIRED",
			"memory package manifest must be the first line",
		)
	}
	manifestLine := append([]byte(nil), scanner.Bytes()...)
	var manifest MemoryPackageManifest
	if err := decodeCanonicalJSONLine(manifestLine, &manifest); err != nil {
		return ParsedMemoryPackage{}, err
	}
	if err := validateMemoryManifest(manifest); err != nil {
		return ParsedMemoryPackage{}, err
	}

	manifestSum := sha256.Sum256(append(append([]byte(nil), manifestLine...), '\n'))
	recordHasher := sha256.New()
	counts := PortableRecordCounts{}
	phase := 0
	ordinal := 0
	projects := make(map[string]struct{}, manifest.Counts.Projects)
	memories := make(map[string]*memoryRevisionChain, manifest.Counts.Memories)
	lastProjectRef := ""
	lastMemoryRef := ""
	lastRevisionMemoryRef := ""
	lastRevision := int64(0)

	for scanner.Scan() {
		line := append([]byte(nil), scanner.Bytes()...)
		ordinal++
		_, _ = recordHasher.Write(line)
		_, _ = recordHasher.Write([]byte{'\n'})
		kind, err := memoryPackageRecordKind(line)
		if err != nil {
			return ParsedMemoryPackage{}, err
		}
		switch kind {
		case "settings":
			if phase > 0 || counts.Settings == 1 {
				return ParsedMemoryPackage{}, packageOrderError()
			}
			var record PortableSettingsRecord
			if err := decodeCanonicalJSONLine(line, &record); err != nil {
				return ParsedMemoryPackage{}, err
			}
			if err := validatePortableSettingsRecord(record); err != nil {
				return ParsedMemoryPackage{}, err
			}
			counts.Settings++
			if err := visitor.VisitSettings(ordinal, record); err != nil {
				return ParsedMemoryPackage{}, err
			}
		case "project":
			if phase > 1 {
				return ParsedMemoryPackage{}, packageOrderError()
			}
			phase = 1
			var record PortableProjectRecord
			if err := decodeCanonicalJSONLine(line, &record); err != nil {
				return ParsedMemoryPackage{}, err
			}
			if err := validatePortableProjectRecord(record); err != nil {
				return ParsedMemoryPackage{}, err
			}
			if _, exists := projects[record.Ref]; exists ||
				(lastProjectRef != "" && record.Ref <= lastProjectRef) {
				return ParsedMemoryPackage{}, validation(
					"MEMORY_PACKAGE_PROJECT_ORDER_INVALID",
					"project refs must be unique and strictly ordered",
				)
			}
			projects[record.Ref] = struct{}{}
			lastProjectRef = record.Ref
			counts.Projects++
			if counts.Projects > MaxMemoryPackageProjects {
				return ParsedMemoryPackage{}, packageCountError("projects")
			}
			if err := visitor.VisitProject(ordinal, record); err != nil {
				return ParsedMemoryPackage{}, err
			}
		case "memory":
			if phase > 2 {
				return ParsedMemoryPackage{}, packageOrderError()
			}
			phase = 2
			var record PortableMemoryRecord
			if err := decodeCanonicalJSONLine(line, &record); err != nil {
				return ParsedMemoryPackage{}, err
			}
			if err := validatePortableMemoryRecordStructure(record, projects); err != nil {
				return ParsedMemoryPackage{}, err
			}
			if _, exists := memories[record.Ref]; exists ||
				(lastMemoryRef != "" && record.Ref <= lastMemoryRef) {
				return ParsedMemoryPackage{}, validation(
					"MEMORY_PACKAGE_MEMORY_ORDER_INVALID",
					"memory refs must be unique and strictly ordered",
				)
			}
			memories[record.Ref] = &memoryRevisionChain{
				currentRevision: record.Revision,
				currentHash:     record.ContentHash,
				nextRevision:    2,
				supersededByRef: record.SupersededByRef,
			}
			lastMemoryRef = record.Ref
			counts.Memories++
			if counts.Memories > MaxMemoryPackageMemories {
				return ParsedMemoryPackage{}, packageCountError("memories")
			}
			if err := visitor.VisitMemory(ordinal, record); err != nil {
				return ParsedMemoryPackage{}, err
			}
		case "revision":
			if !manifest.IncludeHistory || phase < 2 {
				return ParsedMemoryPackage{}, packageOrderError()
			}
			phase = 3
			var record PortableRevisionRecord
			if err := decodeCanonicalJSONLine(line, &record); err != nil {
				return ParsedMemoryPackage{}, err
			}
			chain, ok := memories[record.MemoryRef]
			if !ok {
				return ParsedMemoryPackage{}, validation(
					"MEMORY_PACKAGE_REVISION_MEMORY_UNKNOWN",
					"revision references an unknown memory",
				)
			}
			if lastRevisionMemoryRef != "" &&
				(record.MemoryRef < lastRevisionMemoryRef ||
					(record.MemoryRef == lastRevisionMemoryRef && record.Revision <= lastRevision)) {
				return ParsedMemoryPackage{}, validation(
					"MEMORY_PACKAGE_REVISION_ORDER_INVALID",
					"revisions must be strictly ordered by memory ref and revision",
				)
			}
			if err := validatePortableRevisionRecord(record, chain, projects); err != nil {
				return ParsedMemoryPackage{}, err
			}
			lastRevisionMemoryRef = record.MemoryRef
			lastRevision = record.Revision
			counts.Revisions++
			if counts.Revisions > MaxMemoryPackageRevisions {
				return ParsedMemoryPackage{}, packageCountError("revisions")
			}
			if err := visitor.VisitRevision(ordinal, record); err != nil {
				return ParsedMemoryPackage{}, err
			}
		default:
			return ParsedMemoryPackage{}, validation(
				"MEMORY_PACKAGE_RECORD_KIND_UNSUPPORTED",
				"memory package record kind is unsupported",
			)
		}
	}
	if err := scanner.Err(); err != nil {
		return ParsedMemoryPackage{}, mapMemoryPackageReadError(err)
	}
	if counts != manifest.Counts {
		return ParsedMemoryPackage{}, validation(
			"MEMORY_PACKAGE_COUNT_MISMATCH",
			"memory package record counts do not match the manifest",
		)
	}
	if counts.Settings > 1 || counts.Settings < 0 ||
		counts.Projects > MaxMemoryPackageProjects ||
		counts.Memories > MaxMemoryPackageMemories ||
		counts.Revisions > MaxMemoryPackageRevisions {
		return ParsedMemoryPackage{}, packageCountError("records")
	}
	if !manifest.IncludeHistory && counts.Revisions != 0 {
		return ParsedMemoryPackage{}, validation(
			"MEMORY_PACKAGE_HISTORY_INVALID",
			"memory package history flag does not match revision records",
		)
	}
	for memoryRef, chain := range memories {
		if chain.currentRevision == 1 && chain.seen != 0 {
			return ParsedMemoryPackage{}, revisionChainError()
		}
		if chain.currentRevision > 1 &&
			(chain.seen != int(chain.currentRevision-1) ||
				chain.previousNewHash != chain.currentHash) {
			return ParsedMemoryPackage{}, revisionChainError()
		}
		if chain.supersededByRef != "" {
			if _, ok := memories[chain.supersededByRef]; !ok {
				return ParsedMemoryPackage{}, validation(
					"MEMORY_PACKAGE_SUPERSESSION_INVALID",
					"superseded memory references an unknown package memory",
				)
			}
			if chain.supersededByRef == memoryRef {
				return ParsedMemoryPackage{}, validation(
					"MEMORY_PACKAGE_SUPERSESSION_INVALID",
					"memory cannot supersede itself",
				)
			}
		}
	}
	if got := hex.EncodeToString(recordHasher.Sum(nil)); got != manifest.RecordsSHA256 {
		return ParsedMemoryPackage{}, validation(
			"MEMORY_PACKAGE_HASH_MISMATCH",
			"memory package records hash does not match the manifest",
		)
	}
	return ParsedMemoryPackage{
		Manifest:       manifest,
		ManifestHash:   hex.EncodeToString(manifestSum[:]),
		DecryptedBytes: counter.count,
	}, nil
}

func WriteMemoryPackage(
	destination io.Writer,
	manifest MemoryPackageManifest,
	records []any,
) error {
	if destination == nil {
		return errors.New("memory package destination is required")
	}
	hasher := sha256.New()
	encodedRecords := make([][]byte, 0, len(records))
	for _, record := range records {
		encoded, err := json.Marshal(record)
		if err != nil {
			return fmt.Errorf("marshal memory package record: %w", err)
		}
		encodedRecords = append(encodedRecords, encoded)
		_, _ = hasher.Write(encoded)
		_, _ = hasher.Write([]byte{'\n'})
	}
	manifest.Kind = "manifest"
	manifest.Format = MemoryPackageFormat
	manifest.FormatVersion = MemoryPackageFormatVersion
	manifest.SchemaVersion = MemoryPackageSchemaVersion
	manifest.RecordsSHA256 = hex.EncodeToString(hasher.Sum(nil))
	manifestLine, err := json.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("marshal memory package manifest: %w", err)
	}
	if _, err := destination.Write(append(manifestLine, '\n')); err != nil {
		return fmt.Errorf("write memory package manifest: %w", err)
	}
	for _, encoded := range encodedRecords {
		if _, err := destination.Write(append(encoded, '\n')); err != nil {
			return fmt.Errorf("write memory package record: %w", err)
		}
	}
	return nil
}

func memoryPackageRecordKind(line []byte) (string, error) {
	var header struct {
		Kind string `json:"kind"`
	}
	if err := decodeStrictJSONObject(line, map[string]struct{}{"kind": {}}, true, &header); err != nil {
		return "", err
	}
	return strings.TrimSpace(header.Kind), nil
}

func decodeCanonicalJSONLine(line []byte, target any) error {
	if len(line) == 0 || len(line) > maxMemoryPackageLineBytes || !utf8.Valid(line) {
		return validation(
			"MEMORY_PACKAGE_JSON_INVALID",
			"memory package line is empty, oversized, or not UTF-8",
		)
	}
	allowed, err := jsonFieldNames(target)
	if err != nil {
		return err
	}
	if err := decodeStrictJSONObject(line, allowed, false, target); err != nil {
		return err
	}
	canonical, err := json.Marshal(target)
	if err != nil || !bytes.Equal(canonical, line) {
		return validation(
			"MEMORY_PACKAGE_JSON_NOT_CANONICAL",
			"memory package JSONL must use canonical field order and encoding",
		)
	}
	return nil
}

func decodeStrictJSONObject(
	payload []byte,
	allowed map[string]struct{},
	allowUnknown bool,
	target any,
) error {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return validation("MEMORY_PACKAGE_JSON_INVALID", "memory package line must be one JSON object")
	}
	seen := make(map[string]struct{}, len(allowed))
	for decoder.More() {
		token, err = decoder.Token()
		if err != nil {
			return validation("MEMORY_PACKAGE_JSON_INVALID", "memory package object key is invalid")
		}
		key, ok := token.(string)
		if !ok {
			return validation("MEMORY_PACKAGE_JSON_INVALID", "memory package object key is invalid")
		}
		if _, duplicate := seen[key]; duplicate {
			return validation("MEMORY_PACKAGE_JSON_DUPLICATE", "memory package object has a duplicate field")
		}
		if _, permitted := allowed[key]; !allowUnknown && !permitted {
			return validation("MEMORY_PACKAGE_JSON_UNKNOWN_FIELD", "memory package object has an unknown field")
		}
		seen[key] = struct{}{}
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return validation("MEMORY_PACKAGE_JSON_INVALID", "memory package field value is invalid")
		}
	}
	if token, err = decoder.Token(); err != nil || token != json.Delim('}') {
		return validation("MEMORY_PACKAGE_JSON_INVALID", "memory package object is not closed")
	}
	if token, err = decoder.Token(); !errors.Is(err, io.EOF) || token != nil {
		return validation("MEMORY_PACKAGE_JSON_TRAILING", "memory package line contains trailing JSON")
	}
	strict := json.NewDecoder(bytes.NewReader(payload))
	if !allowUnknown {
		strict.DisallowUnknownFields()
	}
	if err := strict.Decode(target); err != nil {
		return validation("MEMORY_PACKAGE_JSON_INVALID", "memory package object shape is invalid")
	}
	return nil
}

func jsonFieldNames(target any) (map[string]struct{}, error) {
	encoded, err := json.Marshal(target)
	if err != nil {
		return nil, fmt.Errorf("inspect memory package JSON fields: %w", err)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &object); err != nil {
		return nil, fmt.Errorf("inspect memory package JSON shape: %w", err)
	}
	fields := make(map[string]struct{}, len(object)+16)
	for name := range object {
		fields[name] = struct{}{}
	}
	// Omitted optional fields are not present in a zero-value marshal.
	for _, name := range []string{
		"projectRef", "conversationRef", "subjectKey", "factKey",
		"validFrom", "validTo", "expiresAt", "prior", "supersededByRef",
		"purgedAt",
	} {
		fields[name] = struct{}{}
	}
	return fields, nil
}

func validateMemoryManifest(manifest MemoryPackageManifest) error {
	if manifest.Kind != "manifest" || manifest.Format != MemoryPackageFormat ||
		manifest.FormatVersion != MemoryPackageFormatVersion ||
		manifest.SchemaVersion != MemoryPackageSchemaVersion {
		return validation(
			"MEMORY_PACKAGE_VERSION_UNSUPPORTED",
			"memory package format or schema version is unsupported",
		)
	}
	createdAt, err := time.Parse(time.RFC3339Nano, manifest.CreatedAt)
	if err != nil || createdAt.Location() != time.UTC ||
		strings.TrimSpace(manifest.ExporterRelease) == "" ||
		len(manifest.ExporterRelease) > 128 || !isLowerSHA256(manifest.RecordsSHA256) {
		return validation("MEMORY_PACKAGE_MANIFEST_INVALID", "memory package manifest is invalid")
	}
	if manifest.Counts.Settings < 0 || manifest.Counts.Settings > 1 ||
		manifest.Counts.Projects < 0 || manifest.Counts.Projects > MaxMemoryPackageProjects ||
		manifest.Counts.Memories < 0 || manifest.Counts.Memories > MaxMemoryPackageMemories ||
		manifest.Counts.Revisions < 0 || manifest.Counts.Revisions > MaxMemoryPackageRevisions ||
		(!manifest.IncludeHistory && manifest.Counts.Revisions != 0) {
		return packageCountError("manifest")
	}
	return nil
}

func validatePortableSettingsRecord(record PortableSettingsRecord) error {
	if record.Kind != "settings" ||
		!validMemoryPolicyMode(record.Settings.L2Mode) ||
		!validMemoryPolicyMode(record.Settings.L3Mode) {
		return validation("MEMORY_PACKAGE_SETTINGS_INVALID", "memory settings suggestion is invalid")
	}
	return nil
}

func validatePortableProjectRecord(record PortableProjectRecord) error {
	if record.Kind != "project" || !portableRef(record.Ref, "project") ||
		utf8.RuneCountInString(strings.TrimSpace(record.Name)) < 1 ||
		utf8.RuneCountInString(record.Name) > MaxProjectNameChars ||
		utf8.RuneCountInString(record.Description) > MaxProjectDescriptionChars ||
		(record.LifecycleStatus != "active" && record.LifecycleStatus != "archived") {
		return validation("MEMORY_PACKAGE_PROJECT_INVALID", "memory package project is invalid")
	}
	return nil
}

func validatePortableMemoryRecordStructure(
	record PortableMemoryRecord,
	projects map[string]struct{},
) error {
	if record.Kind != "memory" || !portableRef(record.Ref, "memory") ||
		record.Revision < 1 || !isLowerSHA256(record.ContentHash) ||
		record.ContentHash != contentSHA256(record.Content) {
		return validation("MEMORY_PACKAGE_MEMORY_INVALID", "memory package memory identity or hash is invalid")
	}
	if err := validatePortableScope(record.Scope, projects); err != nil {
		return err
	}
	return nil
}

func ValidatePortableMemoryCandidate(record PortableMemoryRecord) error {
	if _, ok := memoryTypes[record.Type]; !ok ||
		strings.TrimSpace(record.Content) == "" ||
		utf8.RuneCountInString(record.Content) > MaxContentChars ||
		record.Importance < 1 || record.Importance > 5 ||
		len(record.Tags) > MaxTags ||
		(record.OriginalAuthority != "manual" &&
			record.OriginalAuthority != "direct_user" &&
			record.OriginalAuthority != "confirmed" &&
			record.OriginalAuthority != "import" &&
			record.OriginalAuthority != "auto") ||
		(record.LifecycleStatus != "active" && record.LifecycleStatus != "superseded" &&
			record.LifecycleStatus != "expired" && record.LifecycleStatus != "rejected") ||
		(record.Sensitivity != SensitivityNormal && record.Sensitivity != SensitivitySensitive) ||
		record.Confidence < 0 || record.Confidence > 1 {
		return validation("MEMORY_PACKAGE_MEMORY_INVALID", "memory package memory fields are invalid")
	}
	for _, key := range []string{record.SubjectKey, record.FactKey} {
		if key != "" && (strings.TrimSpace(key) != key || len(key) > 256) {
			return validation("MEMORY_PACKAGE_MEMORY_INVALID", "memory package memory fields are invalid")
		}
	}
	if (record.LifecycleStatus == "superseded" &&
		!portableRef(record.SupersededByRef, "memory")) ||
		(record.LifecycleStatus != "superseded" && record.SupersededByRef != "") {
		return validation("MEMORY_PACKAGE_MEMORY_INVALID", "memory supersession reference is invalid")
	}
	for _, tag := range record.Tags {
		if strings.TrimSpace(tag) == "" || utf8.RuneCountInString(tag) > MaxTagChars {
			return validation("MEMORY_PACKAGE_MEMORY_INVALID", "memory package memory tags are invalid")
		}
	}
	for _, value := range []string{
		record.ObservedAt, record.ValidFrom, record.ValidTo, record.ExpiresAt,
		record.CreatedAt, record.UpdatedAt,
	} {
		if value != "" && !validPortableTime(value) {
			return validation("MEMORY_PACKAGE_MEMORY_INVALID", "memory package memory time is invalid")
		}
	}
	return nil
}

func validatePortableRevisionRecord(
	record PortableRevisionRecord,
	chain *memoryRevisionChain,
	projects map[string]struct{},
) error {
	if record.Kind != "revision" || record.Revision != chain.nextRevision ||
		record.Revision > chain.currentRevision ||
		!isLowerSHA256(record.OldContentHash) || !isLowerSHA256(record.NewContentHash) ||
		(record.Operation != "update" && record.Operation != "merge" &&
			record.Operation != "supersede" && record.Operation != "delete" &&
			record.Operation != "restore" && record.Operation != "move") ||
		!validPortableTime(record.CreatedAt) || (record.Purged == (record.Prior != nil)) {
		return revisionChainError()
	}
	if chain.seen > 0 && record.OldContentHash != chain.previousNewHash {
		return revisionChainError()
	}
	if record.Prior != nil {
		if record.Prior.ContentHash != record.OldContentHash ||
			record.Prior.ContentHash != contentSHA256(record.Prior.Content) ||
			validatePortableScope(record.Prior.Scope, projects) != nil {
			return revisionChainError()
		}
		candidate := PortableMemoryRecord{
			Type: record.Prior.Type, Content: record.Prior.Content,
			Importance: record.Prior.Importance, Tags: record.Prior.Tags,
			OriginalAuthority: "import", LifecycleStatus: record.Prior.LifecycleStatus,
			Sensitivity: record.Prior.Sensitivity, Confidence: record.Prior.Confidence,
			SupersededByRef: record.Prior.SupersededByRef,
			SubjectKey:      record.Prior.SubjectKey,
			FactKey:         record.Prior.FactKey,
			ObservedAt:      record.Prior.ObservedAt, ValidFrom: record.Prior.ValidFrom,
			ValidTo: record.Prior.ValidTo, ExpiresAt: record.Prior.ExpiresAt,
		}
		if err := ValidatePortableMemoryCandidate(candidate); err != nil {
			return revisionChainError()
		}
	}
	chain.previousNewHash = record.NewContentHash
	chain.nextRevision++
	chain.seen++
	return nil
}

func validatePortableScope(scope PortableMemoryScope, projects map[string]struct{}) error {
	switch scope.Type {
	case "global":
		if scope.ProjectRef != "" || scope.ConversationRef != "" {
			return validation("MEMORY_PACKAGE_SCOPE_INVALID", "global scope cannot have a portable ref")
		}
	case "project":
		if scope.ConversationRef != "" || !portableRef(scope.ProjectRef, "project") {
			return validation("MEMORY_PACKAGE_SCOPE_INVALID", "project scope requires one project ref")
		}
		if _, ok := projects[scope.ProjectRef]; !ok {
			return validation("MEMORY_PACKAGE_SCOPE_INVALID", "project scope references an unknown project")
		}
	case "conversation":
		if scope.ProjectRef != "" || !portableRef(scope.ConversationRef, "conversation") {
			return validation("MEMORY_PACKAGE_SCOPE_INVALID", "conversation scope requires one conversation ref")
		}
	default:
		return validation("MEMORY_PACKAGE_SCOPE_INVALID", "memory package scope is invalid")
	}
	return nil
}

func contentSHA256(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func portableRef(value, prefix string) bool {
	match := portableRefRE.FindStringSubmatch(value)
	return len(match) == 2 && match[1] == prefix
}

func validPortableTime(value string) bool {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	return err == nil && parsed.Location() == time.UTC
}

func validMemoryPolicyMode(value string) bool {
	_, ok := memoryPolicyModes[value]
	return ok
}

func isLowerSHA256(value string) bool {
	if len(value) != sha256.Size*2 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func packageOrderError() error {
	return validation(
		"MEMORY_PACKAGE_RECORD_ORDER_INVALID",
		"memory package record order is invalid",
	)
}

func packageCountError(kind string) error {
	return validation(
		"MEMORY_PACKAGE_COUNT_LIMIT",
		fmt.Sprintf("memory package %s count exceeds the supported limit", kind),
	)
}

func revisionChainError() error {
	return validation(
		"MEMORY_PACKAGE_REVISION_CHAIN_INVALID",
		"memory package revision history is not a continuous authenticated hash chain",
	)
}

func mapMemoryPackageReadError(err error) error {
	var validationErr ValidationError
	if errors.As(err, &validationErr) {
		return err
	}
	if strings.Contains(err.Error(), "token too long") {
		return validation("MEMORY_PACKAGE_LINE_TOO_LARGE", "memory package line exceeds 64 KiB")
	}
	return validation(
		"MEMORY_PACKAGE_AUTHENTICATION_FAILED",
		"memory package is truncated or failed authenticated decryption",
	)
}

func sortedStringKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func hashCanonicalJSON(value any) (string, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

func writeCanonicalRecord(hasher hash.Hash, writer io.Writer, record any) error {
	payload, err := json.Marshal(record)
	if err != nil {
		return err
	}
	if hasher != nil {
		_, _ = hasher.Write(payload)
		_, _ = hasher.Write([]byte{'\n'})
	}
	if writer != nil {
		if _, err := writer.Write(append(payload, '\n')); err != nil {
			return err
		}
	}
	return nil
}
