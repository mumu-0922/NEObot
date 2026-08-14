package agentcron

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
	_ "time/tzdata"

	"github.com/robfig/cron/v3"
)

const (
	maxCatchupWindow = 24 * time.Hour
	onTimeWindow     = time.Minute
	maxScheduleScan  = 2000
)

var strictParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)

var (
	fingerprintPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
	uuidPattern        = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	prefixedIDPattern  = regexp.MustCompile(`^[a-z][a-z0-9_]*_[a-z0-9]{8,64}$`)
	inputRefPattern    = regexp.MustCompile(`^input_ref_[a-z0-9]{16,64}$`)
	reasonPattern      = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,63}$`)
	identifierPattern  = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[._-][a-z0-9]+)*$`)
)

type parsedSchedule struct {
	schedule cron.Schedule
	location *time.Location
}

func parseSchedule(binding ScheduleBinding) (parsedSchedule, string, error) {
	fields := strings.Fields(binding.Expression)
	if len(fields) != 5 || strings.HasPrefix(binding.Expression, "TZ=") ||
		strings.HasPrefix(binding.Expression, "CRON_TZ=") || strings.HasPrefix(binding.Expression, "@") {
		return parsedSchedule{}, "", ErrInvalidInput
	}
	canonical := strings.Join(fields, " ")
	if len(canonical) > 256 || binding.Calculator != CalculatorVersion ||
		(binding.Timezone != "UTC" && binding.Timezone != "Etc/UTC" && !strings.Contains(binding.Timezone, "/")) ||
		binding.Timezone == "Local" || len(binding.Timezone) > 128 {
		return parsedSchedule{}, "", ErrInvalidInput
	}
	location, err := time.LoadLocation(binding.Timezone)
	if err != nil {
		return parsedSchedule{}, "", ErrInvalidInput
	}
	schedule, err := strictParser.Parse(canonical)
	if err != nil {
		return parsedSchedule{}, "", ErrInvalidInput
	}
	if spec, ok := schedule.(*cron.SpecSchedule); ok {
		spec.Location = location
	}
	return parsedSchedule{schedule: schedule, location: location}, canonical, nil
}

func (schedule parsedSchedule) next(after time.Time) time.Time {
	return schedule.schedule.Next(after.UTC()).UTC()
}

func fingerprint(domain string, body []byte) string {
	digest := sha256.Sum256(append(append([]byte(nil), []byte(domain)...), append([]byte{0}, body...)...))
	return "sha256:" + hex.EncodeToString(digest[:])
}

func occurrenceFingerprint(templateID string, revision int64, scheduledFor time.Time) string {
	identity := fmt.Sprintf("%s\x00%d\x00%s", templateID, revision, scheduledFor.UTC().Format(time.RFC3339Nano))
	return fingerprint("neo-cron-occurrence-v1", []byte(identity))
}

func scopeKeys(spec TemplateSpec) []string {
	values := []string{
		"global:*",
		"scheduler:*",
		"user:" + spec.Owner.UserID,
		"project:" + spec.Owner.ProjectID,
		"skill:" + spec.Skill.PackageFingerprint,
		"admission:" + spec.Skill.AdmissionID,
	}
	for _, secret := range spec.Secrets {
		values = append(values, "secret:"+secret.BrokerRef)
	}
	sort.Strings(values)
	return values
}

func planOccurrences(claim DueClaim, observedAt time.Time) ([]OccurrenceDecision, time.Time, error) {
	parsed, _, err := parseSchedule(claim.Spec.Schedule)
	if err != nil || claim.NextTriggerAt.IsZero() || observedAt.IsZero() || claim.NextTriggerAt.After(observedAt) {
		return nil, time.Time{}, ErrInvalidInput
	}
	observedAt = observedAt.UTC()
	cursor := claim.NextTriggerAt.UTC()
	cutoff := observedAt.Add(-time.Duration(claim.Spec.Policies.CatchupWindowSeconds) * time.Second)
	decisions := make([]OccurrenceDecision, 0, claim.Spec.Policies.MaxCatchupRuns+2)
	truncated := false
	if cursor.Before(cutoff) {
		decisions = append(decisions, OccurrenceDecision{
			ScheduledFor: cursor, State: TriggerSkipped, ReasonCode: "MISSED_WINDOW",
			MissedCount: 1, CountTruncated: true,
		})
		truncated = true
		cursor = parsed.next(cutoff.Add(-time.Nanosecond))
	}
	due := make([]time.Time, 0, 64)
	for !cursor.IsZero() && !cursor.After(observedAt) {
		due = append(due, cursor)
		if len(due) > maxScheduleScan {
			return nil, time.Time{}, ErrInvalidInput
		}
		cursor = parsed.next(cursor)
	}
	if cursor.IsZero() {
		return nil, time.Time{}, ErrInvalidInput
	}
	selected := map[int]struct{}{}
	switch claim.Spec.Policies.Missed {
	case MissedSkip:
		if len(due) > 0 && observedAt.Sub(due[len(due)-1]) < onTimeWindow {
			selected[len(due)-1] = struct{}{}
		}
	case MissedFireOnce:
		if len(due) > 0 {
			selected[len(due)-1] = struct{}{}
		}
	case MissedCatchUp:
		start := len(due) - claim.Spec.Policies.MaxCatchupRuns
		if start < 0 {
			start = 0
		}
		for index := start; index < len(due); index++ {
			selected[index] = struct{}{}
		}
	default:
		return nil, time.Time{}, ErrInvalidInput
	}
	missedStart := -1
	missedCount := 0
	flushMissed := func() {
		if missedStart < 0 {
			return
		}
		decisions = append(decisions, OccurrenceDecision{
			ScheduledFor: due[missedStart], State: TriggerSkipped, ReasonCode: "MISSED_RUN",
			MissedCount: missedCount, CountTruncated: truncated,
		})
		missedStart, missedCount = -1, 0
	}
	for index, scheduledFor := range due {
		if _, ok := selected[index]; ok {
			flushMissed()
			decisions = append(decisions, OccurrenceDecision{
				ScheduledFor: scheduledFor, State: TriggerPending, ReasonCode: "SCHEDULED",
			})
			continue
		}
		if missedStart < 0 {
			missedStart = index
		}
		missedCount++
	}
	flushMissed()
	return decisions, cursor, nil
}

func validID(value, prefix string) bool {
	return strings.HasPrefix(value, prefix+"_") && prefixedIDPattern.MatchString(value)
}

func validReason(value string) bool { return reasonPattern.MatchString(value) }

func validActor(actorType, actorID string, approval bool) bool {
	if approval && actorType != "user" && actorType != "operator" {
		return false
	}
	if !approval && actorType != "user" && actorType != "operator" && actorType != "scheduler" {
		return false
	}
	return actorID != "" && strings.TrimSpace(actorID) == actorID && len(actorID) <= 128
}

func validModel(value ModelBinding) bool {
	return identifierPattern.MatchString(value.Provider) && len(value.Provider) <= 64 &&
		value.ModelID != "" && strings.TrimSpace(value.ModelID) == value.ModelID && len(value.ModelID) <= 128 &&
		!strings.ContainsAny(value.ModelID, "\x00\r\n")
}
