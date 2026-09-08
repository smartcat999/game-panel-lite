// Package rabbitmq implements confirmed publication to one region's broker.
package rabbitmq

import (
	"context"
	"errors"
	"net"
	"strings"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/delivery"
)

var (
	ErrUnavailable    = errors.New("RabbitMQ publication unavailable or unconfirmed")
	ErrUnroutable     = errors.New("RabbitMQ publication has no destination")
	ErrInvalidMessage = errors.New("invalid regional publication")
)

type Options struct {
	URL, RegionID, Queue string
	Timeout              time.Duration
	MaxPayloadBytes      int
}

// Publisher owns its connection. One in-flight message per instance makes a
// returned mandatory message unambiguous; use multiple instances for parallelism.
type Publisher struct {
	options    Options
	gate       chan struct{}
	closed     bool
	raw        net.Conn
	connection *amqp.Connection
	channel    *amqp.Channel
	returned   chan amqp.Return
}

var _ delivery.Publisher = (*Publisher)(nil)

func NewPublisher(options Options) (*Publisher, error) {
	if options.URL == "" {
		return nil, errors.New("RabbitMQ endpoint is required")
	}
	if _, err := amqp.ParseURI(options.URL); err != nil {
		return nil, errors.New("invalid RabbitMQ endpoint")
	}
	if options.RegionID == "" || options.RegionID != strings.TrimSpace(options.RegionID) || options.Queue == "" || len(options.Queue) > 255 || options.Queue != strings.TrimSpace(options.Queue) || strings.HasPrefix(options.Queue, "amq.") ||
		options.Timeout <= 0 || options.Timeout > time.Hour || options.MaxPayloadBytes < 1 {
		return nil, errors.New("invalid RabbitMQ publisher options")
	}
	return &Publisher{options: options, gate: make(chan struct{}, 1)}, nil
}

func (p *Publisher) discard() {
	if p.raw != nil {
		_ = p.raw.Close()
	}
	p.raw = nil
	p.connection = nil
	p.channel = nil
	p.returned = nil
}

func (p *Publisher) Close() error {
	p.gate <- struct{}{}
	defer func() { <-p.gate }()
	p.closed = true
	p.discard()
	return nil
}

func (p *Publisher) Publish(parent context.Context, message delivery.Message) (result error) {
	if message.ID == "" || len(message.ID) > 255 || message.RegionID != p.options.RegionID || len(message.Payload) == 0 || len(message.Payload) > p.options.MaxPayloadBytes {
		return ErrInvalidMessage
	}
	ctx, cancel := context.WithTimeout(parent, p.options.Timeout)
	defer cancel()
	select {
	case p.gate <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-p.gate }()
	if p.closed {
		return ErrUnavailable
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	deadline, _ := ctx.Deadline()
	var stop func() bool
	var stopped chan struct{}
	arm := func(conn net.Conn) error {
		if err := conn.SetDeadline(deadline); err != nil {
			return err
		}
		stopped = make(chan struct{})
		stop = context.AfterFunc(ctx, func() { _ = conn.Close(); close(stopped) })
		return nil
	}
	defer func() {
		// Wait for any active cancellation callback before releasing the gate.
		if stop != nil && !stop() {
			<-stopped
		}
		if ctx.Err() != nil {
			result = ctx.Err()
		}
		if result != nil {
			p.discard()
			return
		}
		if err := p.raw.SetDeadline(time.Time{}); err != nil {
			p.discard()
			result = ErrUnavailable
		}
	}()
	if p.connection == nil || p.connection.IsClosed() {
		p.discard()
		conn, err := amqp.DialConfig(p.options.URL, amqp.Config{Dial: func(network, address string) (net.Conn, error) {
			raw, err := (&net.Dialer{}).DialContext(ctx, network, address)
			if err != nil {
				return nil, err
			}
			p.raw = raw
			if err := arm(raw); err != nil {
				return nil, err
			}
			return raw, nil
		}})
		if err != nil {
			return ErrUnavailable
		}
		p.connection = conn
		p.channel, err = conn.Channel()
		if err != nil {
			return ErrUnavailable
		}
		// A mismatching existing queue fails setup; do not silently fall back to
		// a transient or classic queue. Broker policies remain operator-managed.
		if _, err = p.channel.QueueDeclare(p.options.Queue, true, false, false, false, amqp.Table{"x-queue-type": "quorum"}); err != nil {
			return ErrUnavailable
		}
		if err = p.channel.Confirm(false); err != nil {
			return ErrUnavailable
		}
		p.returned = p.channel.NotifyReturn(make(chan amqp.Return, 1))
	} else if err := arm(p.raw); err != nil {
		return ErrUnavailable
	}
	confirmation, err := p.channel.PublishWithDeferredConfirmWithContext(ctx, "", p.options.Queue, true, false, amqp.Publishing{DeliveryMode: amqp.Persistent, ContentType: "application/json", MessageId: message.ID, Body: []byte(message.Payload)})
	if err != nil || confirmation == nil {
		return ErrUnavailable
	}
	confirmed, err := confirmation.WaitContext(ctx)
	if err != nil || !confirmed {
		return ErrUnavailable
	}
	// RabbitMQ sends basic.return before basic.ack; the client dispatches the
	// return to this buffered channel before resolving the deferred confirmation.
	select {
	case _, ok := <-p.returned:
		if !ok {
			return ErrUnavailable
		}
		return ErrUnroutable
	default:
	}
	return nil
}
