package agentlearning

import (
	"context"
	"regexp"
	"sort"
	"strings"
	"time"
)

var (
	promptOverridePattern   = regexp.MustCompile(`(?i)(ignore (all |any )?(previous|prior) instructions|reveal (the )?system prompt|override (the )?system instructions|<\s*system\s*>)`)
	secretMaterialPattern   = regexp.MustCompile(`(?i)(-----BEGIN [A-Z ]*PRIVATE KEY-----|AKIA[0-9A-Z]{16}|sk-[A-Za-z0-9_-]{20,}|(?:api[_-]?key|password|access[_-]?token)\s*[:=]\s*['\"]?[A-Za-z0-9_./+:-]{12,})`)
	evaluationGamingPattern = regexp.MustCompile(`(?i)(t\.skip\s*\(|pytest\.skip\s*\(|@pytest\.mark\.skip|describe\.only\s*\(|it\.only\s*\(|hardcod(?:e|ed).*(?:score|pass)|bypass.*(?:evaluation|test))`)
)

type StaticChecker struct{}

func (StaticChecker) Check(_ context.Context, input CheckInput) (CheckResult, error) {
	started := time.Now()
	files, err := extractArchive(input.Archive)
	if err != nil {
		return CheckResult{}, err
	}
	var scanned int64
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		body := files[path]
		if !textFile(body) {
			continue
		}
		scanned += int64(len(body))
		if scanned > 8<<20 {
			return staticResult(CheckFailed, "STATIC_SCAN_LIMIT", started, scanned), nil
		}
		text := string(body)
		if promptOverridePattern.MatchString(text) {
			return staticResult(CheckFailed, "PROMPT_INJECTION", started, scanned), nil
		}
		if secretMaterialPattern.MatchString(text) {
			return staticResult(CheckFailed, "SECRET_COPY", started, scanned), nil
		}
		if strings.HasPrefix(path, "tests/") && evaluationGamingPattern.MatchString(text) {
			return staticResult(CheckFailed, "EVALUATION_GAMING", started, scanned), nil
		}
	}
	if err := validateEvidenceCoverage(input.Draft.Spec.Evidence,
		input.Draft.Spec.BasePackageFingerprint, input.Draft.Spec.ChangedPaths); err != nil {
		return staticResult(CheckFailed, "SOURCE_LAUNDERING", started, scanned), nil
	}
	return staticResult(CheckPassed, "STATIC_PASSED", started, scanned), nil
}

func staticResult(status, reason string, started time.Time, scanned int64) CheckResult {
	return CheckResult{Status: status, ReasonCode: reason,
		SuiteFingerprint: fingerprint("neo-agent-learning-static-suite-v1", []byte("policy-v1")),
		DurationMillis:   max(time.Since(started).Milliseconds(), 0),
		Metrics:          map[string]int64{"scannedBytes": scanned}}
}

type unavailableChecker struct{ kind string }

func (checker unavailableChecker) Check(context.Context, CheckInput) (CheckResult, error) {
	return CheckResult{}, ErrCheckUnavailable
}
