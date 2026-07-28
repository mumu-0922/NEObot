package redisstate

import (
	"context"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/config"
)

func TestMemoryWakeRejectsInvalidEventID(t *testing.T) {
	client := &Client{}
	if err := client.PublishMemoryWake(context.Background(), "not-an-id"); err == nil {
		t.Fatal("PublishMemoryWake() error = nil, want initialization or UUID error")
	}
}

func TestMemoryWakeIntegration(t *testing.T) {
	redisURL := testRedisURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := Open(ctx, config.RedisConfig{URL: redisURL})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer client.Close()

	subscription, err := client.SubscribeMemoryWake(ctx)
	if err != nil {
		t.Fatalf("SubscribeMemoryWake() error = %v", err)
	}
	defer subscription.Close()

	const eventID = "11111111-1111-4111-8111-111111111111"
	if err := client.PublishMemoryWake(ctx, eventID); err != nil {
		t.Fatalf("PublishMemoryWake() error = %v", err)
	}
	select {
	case got := <-subscription.C():
		if got != eventID {
			t.Fatalf("wake event = %q, want %q", got, eventID)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for memory wake")
	}
}
