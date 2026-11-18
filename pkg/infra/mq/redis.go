package mq

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/log"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/smartcat999/game-panel-cloud/pkg/infra/metrics"
)

const redisStreamPrefix = "gp:mq:stream:"

func init() {
	Register("redis", func(ctx context.Context, cfg *Config) (Interface, error) {
		client := redis.NewClient(&redis.Options{
			Addr:         cfg.Addr,
			Password:     cfg.Password,
			DB:           cfg.DB,
			DialTimeout:  5 * time.Second,
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 3 * time.Second,
		})
		if err := client.Ping(ctx).Err(); err != nil {
			_ = client.Close()
			return nil, fmt.Errorf("redis mq ping (%s): %w", cfg.Addr, err)
		}
		return &redisQueue{client: client}, nil
	})
}

type redisQueue struct {
	client *redis.Client
}

func (r *redisQueue) Publish(ctx context.Context, topic string, event Event) error {
	if event.ID == "" {
		event.ID = randomID()
	}
	if event.Topic == "" {
		event.Topic = topic
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}
	raw, err := json.Marshal(event)
	if err != nil {
		recordPublishFailure("redis", topic, event, err)
		return err
	}
	if err := r.client.XAdd(ctx, &redis.XAddArgs{
		Stream: redisStreamPrefix + topic,
		MaxLen: 10000,
		Approx: true,
		Values: map[string]any{"event": string(raw)},
	}).Err(); err != nil {
		recordPublishFailure("redis", topic, event, err)
		return err
	}
	metrics.MQEventsPublishedTotal.WithLabelValues(topic, string(event.Type)).Inc()
	return nil
}

func (r *redisQueue) Subscribe(ctx context.Context, topic, group string, handler HandlerFunc) (Subscription, error) {
	if group == "" {
		group = "gamepanel-controllers"
	}
	stream := redisStreamPrefix + topic
	isEphemeral := strings.HasPrefix(group, "watch-")
	startID := "0"
	if isEphemeral {
		startID = "$"
	}
	if err := r.client.XGroupCreateMkStream(ctx, stream, group, startID).Err(); err != nil {
		if !strings.Contains(err.Error(), "BUSYGROUP") {
			return nil, err
		}
	}
	consumer := fmt.Sprintf("consumer-%s", randomID())
	subCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go r.consumeStreamLoop(subCtx, done, stream, group, consumer, isEphemeral, handler)
	return &redisSub{
		client:      r.client,
		stream:      stream,
		group:       group,
		isEphemeral: isEphemeral,
		cancel:      cancel,
		done:        done,
	}, nil
}

func (r *redisQueue) consumeStreamLoop(subCtx context.Context, done chan struct{}, stream, group, consumer string, isEphemeral bool, handler HandlerFunc) {
	defer close(done)
	lastAutoClaim := time.Now()
	for {
		if subCtx.Err() != nil {
			return
		}
		if !isEphemeral && time.Since(lastAutoClaim) >= 5*time.Second {
			lastAutoClaim = time.Now()
			r.reclaimStalledMessages(subCtx, stream, group, consumer, handler)
		}
		streams, err := r.client.XReadGroup(subCtx, &redis.XReadGroupArgs{
			Group:    group,
			Consumer: consumer,
			Streams:  []string{stream, ">"},
			Count:    16,
			Block:    200 * time.Millisecond,
		}).Result()
		if err != nil {
			if !errors.Is(err, redis.Nil) && subCtx.Err() == nil {
				time.Sleep(50 * time.Millisecond)
			}
			continue
		}
		for _, st := range streams {
			for _, msg := range st.Messages {
				r.processStreamMessage(subCtx, stream, group, msg, handler)
			}
		}
	}
}

func (r *redisQueue) reclaimStalledMessages(subCtx context.Context, stream, group, consumer string, handler HandlerFunc) {
	claimed, _, err := r.client.XAutoClaim(subCtx, &redis.XAutoClaimArgs{
		Stream:   stream,
		Group:    group,
		Consumer: consumer,
		MinIdle:  15 * time.Second,
		Start:    "0-0",
		Count:    16,
	}).Result()
	if err != nil {
		return
	}
	for _, msg := range claimed {
		if !r.deadLetterIfExhausted(subCtx, stream, group, msg) {
			r.processStreamMessage(subCtx, stream, group, msg, handler)
		}
	}
}

func (r *redisQueue) processStreamMessage(subCtx context.Context, stream, group string, msg redis.XMessage, handler HandlerFunc) {
	rawStr, _ := msg.Values["event"].(string)
	if rawStr == "" {
		_ = r.client.XAck(subCtx, stream, group, msg.ID).Err()
		return
	}
	var ev Event
	if err := json.Unmarshal([]byte(rawStr), &ev); err != nil {
		_ = r.client.XAck(subCtx, stream, group, msg.ID).Err()
		return
	}
	if err := handler(subCtx, ev); err == nil {
		_ = r.client.XAck(subCtx, stream, group, msg.ID).Err()
	}
}

// redisMaxDeliveries caps how often a pending entry is redelivered before it is parked in the
// "<stream>:dlq" dead-letter stream, so a poison message cannot be retried forever.
const redisMaxDeliveries = 10

func (r *redisQueue) deadLetterIfExhausted(ctx context.Context, stream, group string, msg redis.XMessage) bool {
	pending, err := r.client.XPendingExt(ctx, &redis.XPendingExtArgs{
		Stream: stream, Group: group, Start: msg.ID, End: msg.ID, Count: 1,
	}).Result()
	if err != nil || len(pending) == 0 || pending[0].RetryCount < redisMaxDeliveries {
		return false
	}
	values := map[string]any{"sourceId": msg.ID, "group": group, "deliveries": pending[0].RetryCount}
	for k, v := range msg.Values {
		values[k] = v
	}
	if err := r.client.XAdd(ctx, &redis.XAddArgs{Stream: stream + ":dlq", Values: values}).Err(); err != nil {
		return false
	}
	_ = r.client.XAck(ctx, stream, group, msg.ID).Err()
	log.Warn("redis mq: moved poison message to dead-letter stream", "stream", stream, "group", group, "id", msg.ID, "deliveries", pending[0].RetryCount)
	return true
}

func (r *redisQueue) Ping(ctx context.Context) error {
	return r.client.Ping(ctx).Err()
}

func (r *redisQueue) Close() error {
	return r.client.Close()
}

type redisSub struct {
	client      *redis.Client
	stream      string
	group       string
	isEphemeral bool
	cancel      context.CancelFunc
	done        chan struct{}
	once        sync.Once
}

func (s *redisSub) Unsubscribe() error {
	s.once.Do(func() {
		s.cancel()
		<-s.done
		if s.isEphemeral && s.client != nil {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = s.client.XGroupDestroy(cleanupCtx, s.stream, s.group).Err()
		}
	})
	return nil
}
