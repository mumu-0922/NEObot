package chat

import (
	"context"
	"sync"
)

type activeRunRegistry struct {
	mu      sync.Mutex
	cancels map[string]context.CancelFunc
}

func newActiveRunRegistry() *activeRunRegistry {
	return &activeRunRegistry{cancels: map[string]context.CancelFunc{}}
}

func (r *activeRunRegistry) register(runID string, cancel context.CancelFunc) func() {
	if r == nil || runID == "" || cancel == nil {
		return func() {}
	}

	r.mu.Lock()
	r.cancels[runID] = cancel
	r.mu.Unlock()

	return func() {
		r.mu.Lock()
		delete(r.cancels, runID)
		r.mu.Unlock()
	}
}

func (r *activeRunRegistry) cancel(runID string) bool {
	if r == nil || runID == "" {
		return false
	}

	r.mu.Lock()
	cancel := r.cancels[runID]
	r.mu.Unlock()

	if cancel == nil {
		return false
	}

	cancel()
	return true
}
