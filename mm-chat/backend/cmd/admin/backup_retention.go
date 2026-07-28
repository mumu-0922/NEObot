package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	backupSetManifestVersion = 1
	dailyRetention           = 14 * 24 * time.Hour
	longRetention            = 56 * 24 * time.Hour
	maxBackupManifestBytes   = 64 << 10
)

var (
	backupSetIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
	sha256Pattern      = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

type backupSetArtifact struct {
	Kind         string `json:"kind"`
	Path         string `json:"path"`
	ChecksumPath string `json:"checksumPath"`
	SHA256       string `json:"sha256"`
}

type backupSetManifest struct {
	Version                 int                 `json:"version"`
	SetID                   string              `json:"setId"`
	Class                   string              `json:"class"`
	CreatedAt               string              `json:"createdAt"`
	ContainsMemoryPlaintext bool                `json:"containsMemoryPlaintext"`
	Artifacts               []backupSetArtifact `json:"artifacts"`
}

type backupRetentionSet struct {
	SetID     string   `json:"setId"`
	Class     string   `json:"class"`
	CreatedAt string   `json:"createdAt"`
	Files     []string `json:"files"`
}

type backupRetentionPlan struct {
	Version    int                  `json:"version"`
	BackupRoot string               `json:"backupRoot"`
	Sets       []backupRetentionSet `json:"sets"`
}

type backupRetentionResult struct {
	PlanSHA256   string
	ExpiredSets  int
	Files        int
	DeletedSets  int
	DeletedFiles int
	Executed     bool
}

func runBackupRetention(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("backup-retention", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var backupRoot, expectedPlanSHA256 string
	var execute bool
	flags.StringVar(&backupRoot, "backup-root", "", "fixed backup root")
	flags.BoolVar(&execute, "execute", false, "delete the freshly verified plan")
	flags.StringVar(&expectedPlanSHA256, "expected-plan-sha256", "", "dry-run plan SHA-256")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 ||
		strings.TrimSpace(backupRoot) == "" ||
		(execute && !sha256Pattern.MatchString(expectedPlanSHA256)) ||
		(!execute && expectedPlanSHA256 != "") {
		return usageError()
	}
	result, plan, err := applyBackupRetention(
		strings.TrimSpace(backupRoot), time.Now().UTC(), execute, expectedPlanSHA256,
	)
	if writeErr := writeBackupRetentionResult(stdout, result, plan); writeErr != nil {
		return writeErr
	}
	return err
}

func applyBackupRetention(
	backupRoot string,
	now time.Time,
	execute bool,
	expectedPlanSHA256 string,
) (backupRetentionResult, backupRetentionPlan, error) {
	plan, planSHA256, err := buildBackupRetentionPlan(backupRoot, now)
	if err != nil {
		return backupRetentionResult{}, backupRetentionPlan{}, err
	}
	result := backupRetentionResult{
		PlanSHA256: planSHA256, ExpiredSets: len(plan.Sets), Executed: execute,
	}
	for _, set := range plan.Sets {
		result.Files += len(set.Files)
	}
	if !execute {
		return result, plan, nil
	}
	if expectedPlanSHA256 != planSHA256 {
		return result, plan, errors.New("backup retention plan changed; run dry-run again")
	}
	for _, set := range plan.Sets {
		for _, relative := range set.Files {
			fullPath := filepath.Join(plan.BackupRoot, filepath.FromSlash(relative))
			if err := validateExistingPath(fullPath, false); err != nil {
				return result, plan, fmt.Errorf(
					"backup retention stopped after deleting %d files: %w",
					result.DeletedFiles, err,
				)
			}
			if err := os.Remove(fullPath); err != nil {
				return result, plan, fmt.Errorf(
					"backup retention stopped after deleting %d files: %w",
					result.DeletedFiles, err,
				)
			}
			result.DeletedFiles++
		}
		result.DeletedSets++
	}
	return result, plan, nil
}

func buildBackupRetentionPlan(
	backupRoot string,
	now time.Time,
) (backupRetentionPlan, string, error) {
	root, err := filepath.Abs(backupRoot)
	if err != nil {
		return backupRetentionPlan{}, "", fmt.Errorf("resolve backup root: %w", err)
	}
	root = filepath.Clean(root)
	if err := validateExistingPath(root, true); err != nil {
		return backupRetentionPlan{}, "", fmt.Errorf("validate backup root: %w", err)
	}
	setsDirectory := filepath.Join(root, "sets")
	if err := validateExistingPath(setsDirectory, true); err != nil {
		return backupRetentionPlan{}, "", fmt.Errorf("validate backup sets directory: %w", err)
	}
	entries, err := os.ReadDir(setsDirectory)
	if err != nil {
		return backupRetentionPlan{}, "", fmt.Errorf("read backup sets directory: %w", err)
	}
	plan := backupRetentionPlan{Version: 1, BackupRoot: root}
	claimedPaths := make(map[string]string)
	for _, directoryEntry := range entries {
		name := directoryEntry.Name()
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		setID := strings.TrimSuffix(name, ".json")
		if !backupSetIDPattern.MatchString(setID) {
			return backupRetentionPlan{}, "", fmt.Errorf("backup set manifest name is invalid: %s", name)
		}
		manifestRelative := path.Join("sets", name)
		manifestChecksumRelative := manifestRelative + ".sha256"
		manifestPath, err := verifiedBackupPath(root, manifestRelative, false)
		if err != nil {
			return backupRetentionPlan{}, "", err
		}
		manifestChecksumPath, err := verifiedBackupPath(root, manifestChecksumRelative, false)
		if err != nil {
			return backupRetentionPlan{}, "", err
		}
		manifestBytes, err := readBoundedFile(manifestPath, maxBackupManifestBytes)
		if err != nil {
			return backupRetentionPlan{}, "", err
		}
		var manifest backupSetManifest
		if err := decodeStrictBackupManifest(manifestBytes, &manifest); err != nil {
			return backupRetentionPlan{}, "", fmt.Errorf("decode backup set %s: %w", setID, err)
		}
		if err := validateBackupSetManifest(manifest, setID, now); err != nil {
			return backupRetentionPlan{}, "", err
		}
		manifestHash, err := fileSHA256(manifestPath)
		if err != nil {
			return backupRetentionPlan{}, "", err
		}
		if err := verifyChecksumPair(manifestChecksumPath, manifestPath, manifestHash); err != nil {
			return backupRetentionPlan{}, "", err
		}
		setFiles := make([]string, 0, len(manifest.Artifacts)*2+2)
		for _, artifact := range manifest.Artifacts {
			if owner, duplicate := claimedPaths[artifact.Path]; duplicate {
				return backupRetentionPlan{}, "", fmt.Errorf(
					"backup path %s is claimed by both %s and %s", artifact.Path, owner, setID,
				)
			}
			if owner, duplicate := claimedPaths[artifact.ChecksumPath]; duplicate {
				return backupRetentionPlan{}, "", fmt.Errorf(
					"backup path %s is claimed by both %s and %s", artifact.ChecksumPath, owner, setID,
				)
			}
			claimedPaths[artifact.Path] = setID
			claimedPaths[artifact.ChecksumPath] = setID
			artifactPath, err := verifiedBackupPath(root, artifact.Path, false)
			if err != nil {
				return backupRetentionPlan{}, "", err
			}
			checksumPath, err := verifiedBackupPath(root, artifact.ChecksumPath, false)
			if err != nil {
				return backupRetentionPlan{}, "", err
			}
			actualHash, err := fileSHA256(artifactPath)
			if err != nil {
				return backupRetentionPlan{}, "", err
			}
			if actualHash != artifact.SHA256 {
				return backupRetentionPlan{}, "", fmt.Errorf("backup artifact checksum mismatch: %s", artifact.Path)
			}
			if err := verifyChecksumPair(checksumPath, artifactPath, actualHash); err != nil {
				return backupRetentionPlan{}, "", err
			}
			setFiles = append(setFiles, artifact.Path, artifact.ChecksumPath)
		}
		claimedPaths[manifestRelative] = setID
		claimedPaths[manifestChecksumRelative] = setID
		createdAt, _ := time.Parse(time.RFC3339Nano, manifest.CreatedAt)
		retention := dailyRetention
		if manifest.Class != "daily" {
			retention = longRetention
		}
		if !createdAt.Add(retention).After(now) {
			setFiles = append(setFiles, manifestChecksumRelative, manifestRelative)
			plan.Sets = append(plan.Sets, backupRetentionSet{
				SetID: setID, Class: manifest.Class,
				CreatedAt: manifest.CreatedAt, Files: setFiles,
			})
		}
	}
	sort.Slice(plan.Sets, func(left, right int) bool {
		if plan.Sets[left].CreatedAt == plan.Sets[right].CreatedAt {
			return plan.Sets[left].SetID < plan.Sets[right].SetID
		}
		return plan.Sets[left].CreatedAt < plan.Sets[right].CreatedAt
	})
	encoded, err := json.Marshal(plan)
	if err != nil {
		return backupRetentionPlan{}, "", fmt.Errorf("encode backup retention plan: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return plan, hex.EncodeToString(sum[:]), nil
}

func validateBackupSetManifest(
	manifest backupSetManifest,
	setID string,
	now time.Time,
) error {
	createdAt, err := time.Parse(time.RFC3339Nano, manifest.CreatedAt)
	if err != nil || createdAt.Location() != time.UTC || createdAt.After(now) ||
		manifest.Version != backupSetManifestVersion || manifest.SetID != setID ||
		!manifest.ContainsMemoryPlaintext ||
		(manifest.Class != "daily" && manifest.Class != "weekly" && manifest.Class != "pre-deploy") ||
		len(manifest.Artifacts) != 2 {
		return fmt.Errorf("backup set manifest is invalid: %s", setID)
	}
	expected := []backupSetArtifact{
		{
			Kind: "postgres", Path: "postgres/postgres-" + setID + ".dump",
			ChecksumPath: "postgres/postgres-" + setID + ".dump.sha256",
		},
		{
			Kind: "minio", Path: "minio/minio-" + setID + ".tar.gz",
			ChecksumPath: "minio/minio-" + setID + ".tar.gz.sha256",
		},
	}
	for index, artifact := range manifest.Artifacts {
		if artifact.Kind != expected[index].Kind || artifact.Path != expected[index].Path ||
			artifact.ChecksumPath != expected[index].ChecksumPath ||
			!sha256Pattern.MatchString(artifact.SHA256) {
			return fmt.Errorf("backup set artifact is invalid: %s", setID)
		}
	}
	return nil
}

func verifiedBackupPath(root, relative string, directory bool) (string, error) {
	if relative == "" || strings.Contains(relative, "\\") || path.Clean(relative) != relative ||
		path.IsAbs(relative) || relative == "." || relative == ".." ||
		strings.HasPrefix(relative, "../") {
		return "", fmt.Errorf("backup path is unsafe: %s", relative)
	}
	fullPath := filepath.Join(root, filepath.FromSlash(relative))
	relativeToRoot, err := filepath.Rel(root, fullPath)
	if err != nil || relativeToRoot == ".." || strings.HasPrefix(relativeToRoot, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("backup path escapes root: %s", relative)
	}
	if err := validateExistingPath(fullPath, directory); err != nil {
		return "", err
	}
	return fullPath, nil
}

func validateExistingPath(value string, directory bool) error {
	absolute, err := filepath.Abs(value)
	if err != nil {
		return err
	}
	volume := filepath.VolumeName(absolute)
	current := volume + string(filepath.Separator)
	relative := strings.TrimPrefix(absolute, current)
	for _, component := range strings.Split(relative, string(filepath.Separator)) {
		if component == "" {
			continue
		}
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if err != nil {
			return fmt.Errorf("inspect backup path %s: %w", current, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("backup path contains a symlink: %s", current)
		}
	}
	info, err := os.Lstat(absolute)
	if err != nil {
		return err
	}
	if directory && !info.IsDir() {
		return fmt.Errorf("backup path is not a directory: %s", absolute)
	}
	if !directory && !info.Mode().IsRegular() {
		return fmt.Errorf("backup path is not a regular file: %s", absolute)
	}
	return nil
}

func readBoundedFile(filePath string, maximum int64) ([]byte, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	payload, err := io.ReadAll(io.LimitReader(file, maximum+1))
	if err != nil {
		return nil, err
	}
	if int64(len(payload)) > maximum {
		return nil, fmt.Errorf("backup metadata is too large: %s", filePath)
	}
	return payload, nil
}

func fileSHA256(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func verifyChecksumPair(checksumPath, artifactPath, expectedHash string) error {
	payload, err := readBoundedFile(checksumPath, 1024)
	if err != nil {
		return err
	}
	expected := expectedHash + "  " + filepath.Base(artifactPath) + "\n"
	if string(payload) != expected {
		return fmt.Errorf("backup checksum file does not match artifact: %s", checksumPath)
	}
	return nil
}

func decodeStrictBackupManifest(payload []byte, target *backupSetManifest) error {
	if len(payload) == 0 || !json.Valid(payload) {
		return errors.New("backup set manifest must be valid JSON")
	}
	duplicateDecoder := json.NewDecoder(bytes.NewReader(payload))
	if err := consumeStrictJSONValue(duplicateDecoder); err != nil {
		return err
	}
	if token, err := duplicateDecoder.Token(); !errors.Is(err, io.EOF) || token != nil {
		return errors.New("backup set manifest contains trailing JSON")
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return errors.New("backup set manifest shape is invalid")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("backup set manifest contains trailing JSON")
	}
	return nil
}

func consumeStrictJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return errors.New("backup set manifest JSON is invalid")
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			key, keyOK := keyToken.(string)
			if err != nil || !keyOK {
				return errors.New("backup set manifest object key is invalid")
			}
			if _, duplicate := seen[key]; duplicate {
				return errors.New("backup set manifest contains a duplicate field")
			}
			seen[key] = struct{}{}
			if err := consumeStrictJSONValue(decoder); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim('}') {
			return errors.New("backup set manifest object is not closed")
		}
	case '[':
		for decoder.More() {
			if err := consumeStrictJSONValue(decoder); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim(']') {
			return errors.New("backup set manifest array is not closed")
		}
	default:
		return errors.New("backup set manifest JSON is invalid")
	}
	return nil
}

func writeBackupRetentionResult(
	writer io.Writer,
	result backupRetentionResult,
	plan backupRetentionPlan,
) error {
	mode := "dry-run"
	if result.Executed {
		mode = "executed"
	}
	buffer := bufio.NewWriter(writer)
	if _, err := fmt.Fprintf(
		buffer,
		"backup retention mode=%s expired_sets=%d files=%d deleted_sets=%d deleted_files=%d plan_sha256=%s\n",
		mode, result.ExpiredSets, result.Files, result.DeletedSets,
		result.DeletedFiles, result.PlanSHA256,
	); err != nil {
		return err
	}
	for _, set := range plan.Sets {
		if _, err := fmt.Fprintf(
			buffer, "backup retention set=%s class=%s created_at=%s\n",
			set.SetID, set.Class, set.CreatedAt,
		); err != nil {
			return err
		}
	}
	return buffer.Flush()
}
