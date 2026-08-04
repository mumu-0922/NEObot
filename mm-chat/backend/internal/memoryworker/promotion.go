package memoryworker

import (
	"context"
	"log/slog"
	"strings"
)

func (w *Worker) promoteCaptureCandidates(
	ctx context.Context,
	job Job,
) (string, bool, error) {
	summary, err := w.repository.PromoteCandidates(ctx, job)
	if err != nil {
		return classifyPromotionError(err), terminalPromotionError(err), err
	}
	w.logger.InfoContext(
		ctx,
		"memory_capture_promotion_completed",
		slog.String("job_id", job.JobID),
		slog.Int("promoted_count", summary.PromotedCount),
		slog.Int("review_count", summary.ReviewCount),
		slog.Int("rejected_count", summary.RejectedCount),
	)
	return "", false, nil
}

func classifyPromotionError(err error) string {
	value := strings.ToUpper(err.Error())
	switch {
	case strings.Contains(value, "SOURCE_DRIFT") ||
		strings.Contains(value, "SOURCE_TOMBSTONED") ||
		strings.Contains(value, "VISIBILITY_EPOCH_DRIFT"):
		return errorSourceDrift
	case strings.Contains(value, "PROFILE_DRIFT"):
		return errorProfileDrift
	default:
		return errorPromotionFailed
	}
}

func terminalPromotionError(err error) bool {
	code := classifyPromotionError(err)
	return code == errorSourceDrift || code == errorProfileDrift
}
