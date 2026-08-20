package chat

import (
	"context"
	"sync"
	"time"
)

const (
	activeRunStreamRetention = 30 * time.Second
	maxFinishedRunStreams    = 64
)

type activeRunEntry struct {
	cancel         context.CancelFunc
	userID         string
	conversationID string
	stream         *activeRunStream
	finishedAt     time.Time
}

type activeRunRegistry struct {
	mu      sync.Mutex
	entries map[string]*activeRunEntry
}

func newActiveRunRegistry() *activeRunRegistry {
	return &activeRunRegistry{entries: map[string]*activeRunEntry{}}
}

func (r *activeRunRegistry) register(runID string, cancel context.CancelFunc) func() {
	return r.registerStream(runID, cancel, "", "", nil)
}

func (r *activeRunRegistry) registerStream(
	runID string,
	cancel context.CancelFunc,
	userID string,
	conversationID string,
	stream *activeRunStream,
) func() {
	if r == nil || runID == "" || cancel == nil {
		return func() {}
	}

	r.mu.Lock()
	r.entries[runID] = &activeRunEntry{
		cancel: cancel, userID: userID, conversationID: conversationID, stream: stream,
	}
	r.mu.Unlock()

	return func() {
		r.finish(runID, stream)
	}
}

func (r *activeRunRegistry) cancel(runID string) bool {
	if r == nil || runID == "" {
		return false
	}

	r.mu.Lock()
	entry := r.entries[runID]
	var cancel context.CancelFunc
	if entry != nil {
		cancel = entry.cancel
	}
	r.mu.Unlock()

	if cancel == nil {
		return false
	}

	cancel()
	return true
}

func (r *activeRunRegistry) streamForUser(runID, userID string) (*activeRunStream, string) {
	if r == nil || runID == "" || userID == "" {
		return nil, ""
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	entry := r.entries[runID]
	if entry == nil || entry.userID != userID || entry.stream == nil {
		return nil, ""
	}
	return entry.stream, entry.conversationID
}

func (r *activeRunRegistry) finish(runID string, stream *activeRunStream) {
	if r == nil || runID == "" {
		return
	}
	if stream != nil {
		stream.close()
	}
	r.mu.Lock()
	entry := r.entries[runID]
	if entry == nil || entry.stream != stream {
		r.mu.Unlock()
		return
	}
	if stream == nil {
		delete(r.entries, runID)
		r.mu.Unlock()
		return
	}
	entry.cancel = nil
	entry.finishedAt = time.Now()
	r.pruneFinishedLocked()
	r.mu.Unlock()
	time.AfterFunc(activeRunStreamRetention, func() {
		r.mu.Lock()
		current := r.entries[runID]
		if current == entry && !current.finishedAt.IsZero() &&
			time.Since(current.finishedAt) >= activeRunStreamRetention {
			delete(r.entries, runID)
		}
		r.mu.Unlock()
	})
}

func (r *activeRunRegistry) pruneFinishedLocked() {
	for {
		finishedCount := 0
		oldestRunID := ""
		var oldest time.Time
		for runID, entry := range r.entries {
			if entry.finishedAt.IsZero() {
				continue
			}
			finishedCount++
			if oldestRunID == "" || entry.finishedAt.Before(oldest) {
				oldestRunID, oldest = runID, entry.finishedAt
			}
		}
		if finishedCount <= maxFinishedRunStreams || oldestRunID == "" {
			return
		}
		delete(r.entries, oldestRunID)
	}
}
