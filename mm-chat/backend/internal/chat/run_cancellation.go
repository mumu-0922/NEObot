package chat

import (
	"context"
	"time"
)

const redisCancelPollInterval = 250 * time.Millisecond

type RunCancellationStore interface {
	MarkRunCancelled(ctx context.Context, runID string) error
	IsRunCancelled(ctx context.Context, runID string) (bool, error)
	ClearRunCancelled(ctx context.Context, runID string) error
}

func watchRunCancellation(
	parent context.Context,
	store RunCancellationStore,
	runID string,
	cancel context.CancelFunc,
) func() {
	if store == nil || runID == "" || cancel == nil {
		return func() {}
	}

	ctx, stop := context.WithCancel(parent)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(redisCancelPollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				cancelled, err := store.IsRunCancelled(ctx, runID)
				if err != nil {
					continue
				}
				if cancelled {
					cancel()
					return
				}
			}
		}
	}()

	return func() {
		stop()
		<-done
	}
}
