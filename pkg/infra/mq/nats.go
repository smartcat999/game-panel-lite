package mq

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/smartcat999/game-panel-cloud/pkg/infra/log"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/metrics"
)

const (
	natsJetStreamName       = "GAMEPANEL_EVENTS"
	natsJetStreamMaxAge     = 72 * time.Hour
	natsJetStreamMaxMsgs    = 100_000
	natsJetStreamMaxBytes   = 256 * 1024 * 1024
	natsJetStreamDuplicates = 2 * time.Minute
	natsJetStreamMaxDeliver = 10
	natsJetStreamAckWait    = 30 * time.Second
	natsHandlerAttempts     = 4
)

var natsStreamSubjects = []string{
	"core.v1.>",
	"iam.v1.>",
	"billing.v1.>",
}

func init() {
	Register("nats", func(_ context.Context, cfg *Config) (Interface, error) {
		opts := []nats.Option{
			nats.Name("gamepanel-mq"),
			nats.Timeout(5 * time.Second),
		}
		if cfg.Password != "" {
			opts = append(opts, nats.Token(cfg.Password))
		}
		nc, err := nats.Connect(cfg.Addr, opts...)
		if err != nil {
			return nil, fmt.Errorf("nats connect (%s): %w", cfg.Addr, err)
		}
		q := &natsQueue{nc: nc}
		q.initJetStream()
		return q, nil
	})
}

type natsQueue struct {
	nc        *nats.Conn
	js        nats.JetStreamContext
	jsEnabled bool
}

func defaultJetStreamConfig(storage nats.StorageType) *nats.StreamConfig {
	return &nats.StreamConfig{
		Name:       natsJetStreamName,
		Subjects:   slices.Clone(natsStreamSubjects),
		Retention:  nats.LimitsPolicy,
		Discard:    nats.DiscardOld,
		Storage:    storage,
		MaxAge:     natsJetStreamMaxAge,
		MaxMsgs:    natsJetStreamMaxMsgs,
		MaxBytes:   natsJetStreamMaxBytes,
		Duplicates: natsJetStreamDuplicates,
	}
}

func (n *natsQueue) initJetStream() {
	if n.nc == nil {
		return
	}
	js, err := n.nc.JetStream()
	if err != nil {
		log.Warn("nats: failed to initialize JetStream context; falling back to Core NATS", "error", err)
		return
	}
	info, err := js.StreamInfo(natsJetStreamName)
	if err != nil {
		if _, addErr := js.AddStream(defaultJetStreamConfig(nats.FileStorage)); addErr != nil {
			// Try memory storage if file storage is disallowed on the server
			if _, memErr := js.AddStream(defaultJetStreamConfig(nats.MemoryStorage)); memErr != nil {
				log.Warn("nats: JetStream not available on server; falling back to Core NATS + Resync", "error", addErr)
				return
			}
		}
	} else if _, upErr := js.UpdateStream(defaultJetStreamConfig(info.Config.Storage)); upErr != nil {
		log.Warn("nats: failed to update JetStream stream config", "stream", natsJetStreamName, "error", upErr)
	}
	n.js = js
	n.jsEnabled = true
}

func (n *natsQueue) Publish(ctx context.Context, topic string, event Event) error {
	if err := ctx.Err(); err != nil {
		recordPublishFailure("nats", topic, event, err)
		return err
	}
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
		recordPublishFailure("nats", topic, event, err)
		return err
	}
	if n.jsEnabled && n.js != nil && isJetStreamSubject(topic) {
		if _, err := n.js.Publish(topic, raw, nats.MsgId(event.ID), nats.Context(ctx)); err != nil {
			recordPublishFailure("nats-jetstream", topic, event, err)
			return err
		}
		metrics.MQEventsPublishedTotal.WithLabelValues(topic, string(event.Type)).Inc()
		return nil
	}
	if err := n.nc.Publish(topic, raw); err != nil {
		recordPublishFailure("nats", topic, event, err)
		return err
	}
	metrics.MQEventsPublishedTotal.WithLabelValues(topic, string(event.Type)).Inc()
	return nil
}

func (n *natsQueue) Subscribe(ctx context.Context, topic, group string, handler HandlerFunc) (Subscription, error) {
	if group == "" {
		group = "gamepanel-controllers"
	}
	isEphemeral := strings.HasPrefix(group, "watch-")
	if n.jsEnabled && n.js != nil && !isEphemeral && isJetStreamSubject(topic) {
		if sub, err := n.subscribeJetStream(ctx, topic, group, handler); err == nil {
			return sub, nil
		} else {
			log.Warn("nats jetstream: QueueSubscribe failed; falling back to Core NATS", "topic", topic, "group", group, "error", err)
		}
	}
	return n.nc.QueueSubscribe(topic, group, func(msg *nats.Msg) {
		var ev Event
		if err := json.Unmarshal(msg.Data, &ev); err != nil {
			log.Warn("nats: dropping undecodable event", "topic", topic, "error", err)
			return
		}
		var err error
		for attempt := range natsHandlerAttempts {
			if err = handler(ctx, ev); err == nil || ctx.Err() != nil {
				return
			}
			time.Sleep(time.Duration(50<<attempt) * time.Millisecond)
		}
		log.Warn("nats: event handler failed, relying on resync", "topic", topic, "type", ev.Type, "tenant", ev.Tenant, "name", ev.Name, "error", err)
	})
}

func (n *natsQueue) subscribeJetStream(ctx context.Context, topic, group string, handler HandlerFunc) (*nats.Subscription, error) {
	durable := sanitizeDurableName(group + "-" + topic)
	return n.js.QueueSubscribe(
		topic,
		durable,
		func(msg *nats.Msg) {
			if ctx.Err() != nil {
				_ = msg.Nak()
				return
			}
			var ev Event
			if err := json.Unmarshal(msg.Data, &ev); err != nil {
				log.Warn("nats jetstream: terminating undecodable event", "topic", topic, "group", group, "error", err)
				_ = msg.Term()
				return
			}
			if err := handler(ctx, ev); err != nil {
				log.Warn("nats jetstream: handler failed, naking for redelivery", "topic", topic, "group", group, "type", ev.Type, "tenant", ev.Tenant, "name", ev.Name, "error", err)
				_ = msg.NakWithDelay(500 * time.Millisecond)
				return
			}
			_ = msg.Ack()
		},
		nats.BindStream(natsJetStreamName),
		nats.Durable(durable),
		nats.ManualAck(),
		nats.AckExplicit(),
		nats.AckWait(natsJetStreamAckWait),
		nats.MaxDeliver(natsJetStreamMaxDeliver),
	)
}

func isJetStreamSubject(topic string) bool {
	return strings.HasPrefix(topic, "core.v1.") ||
		strings.HasPrefix(topic, "iam.v1.") ||
		strings.HasPrefix(topic, "billing.v1.")
}

var durableSanitizer = strings.NewReplacer(".", "_", ":", "_", " ", "_", "/", "_", "*", "all", ">", "all")

func sanitizeDurableName(s string) string {
	return durableSanitizer.Replace(s)
}

func (n *natsQueue) Ping(_ context.Context) error {
	if n.nc == nil || !n.nc.IsConnected() {
		return errors.New("nats connection is not active")
	}
	return nil
}

func (n *natsQueue) Close() error {
	n.nc.Close()
	return nil
}
