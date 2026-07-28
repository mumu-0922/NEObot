package redisstate

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"sync"

	"github.com/redis/go-redis/v9"
)

const MemoryOutboxChannel = "mm-chat:memory:outbox:v1"

var memoryEventIDRE = regexp.MustCompile(
	`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-5][0-9a-fA-F]{3}-` +
		`[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`,
)

func (c *Client) PublishMemoryWake(ctx context.Context, eventID string) error {
	if c == nil || c.rdb == nil {
		return errors.New("redis memory wake publisher is not initialized")
	}
	eventID = strings.TrimSpace(eventID)
	if !memoryEventIDRE.MatchString(eventID) {
		return errors.New("memory wake event id must be a UUID")
	}
	return c.rdb.Publish(ctx, MemoryOutboxChannel, eventID).Err()
}

type MemoryWakeSubscription struct {
	pubsub    *redis.PubSub
	messages  chan string
	closeOnce sync.Once
}

func (c *Client) SubscribeMemoryWake(
	ctx context.Context,
) (*MemoryWakeSubscription, error) {
	if c == nil || c.rdb == nil {
		return nil, errors.New("redis memory wake subscriber is not initialized")
	}
	pubsub := c.rdb.Subscribe(ctx, MemoryOutboxChannel)
	if _, err := pubsub.Receive(ctx); err != nil {
		_ = pubsub.Close()
		return nil, err
	}
	subscription := &MemoryWakeSubscription{
		pubsub:   pubsub,
		messages: make(chan string, 1),
	}
	go subscription.forward(ctx)
	return subscription, nil
}

func (s *MemoryWakeSubscription) C() <-chan string {
	if s == nil {
		return nil
	}
	return s.messages
}

func (s *MemoryWakeSubscription) Close() error {
	if s == nil {
		return nil
	}
	var closeErr error
	s.closeOnce.Do(func() {
		closeErr = s.pubsub.Close()
	})
	return closeErr
}

func (s *MemoryWakeSubscription) forward(ctx context.Context) {
	defer close(s.messages)
	channel := s.pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			_ = s.Close()
			return
		case message, ok := <-channel:
			if !ok {
				return
			}
			eventID := strings.TrimSpace(message.Payload)
			if !memoryEventIDRE.MatchString(eventID) {
				continue
			}
			select {
			case s.messages <- eventID:
			default:
			}
		}
	}
}
