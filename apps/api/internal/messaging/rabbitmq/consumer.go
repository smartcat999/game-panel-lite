package rabbitmq

import (
	"context"
	"errors"
	"net"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/regional"
)

type Handler interface {
	Handle(context.Context, regional.Notification) error
}
type Consumer struct {
	Options                    Options
	HandlerTimeout, RetryDelay time.Duration
}

// Run holds at most one unacknowledged message. Transient handler failures retry
// locally without a broker requeue loop. The caller supervises failed sessions.
// The result counts ACK writes; AMQP does not confirm receipt of consumer ACKs.
func (c Consumer) Run(ctx context.Context, handler Handler) (int, error) {
	if err := c.Options.Validate(); err != nil {
		return 0, err
	}
	if handler == nil || c.HandlerTimeout <= 0 || c.RetryDelay < time.Millisecond {
		return 0, errors.New("invalid consumer settings")
	}
	setupCtx, cancel := context.WithTimeout(ctx, c.Options.Timeout)
	defer cancel()
	var raw net.Conn
	var stopSetup func() bool
	setupStopped := make(chan struct{})
	defer func() {
		if stopSetup != nil && !stopSetup() {
			<-setupStopped
		}
		if raw != nil {
			_ = raw.Close()
		}
	}()
	conn, err := amqp.DialConfig(c.Options.URL, amqp.Config{Dial: func(network, address string) (net.Conn, error) {
		var err error
		raw, err = (&net.Dialer{}).DialContext(setupCtx, network, address)
		if err == nil {
			stopSetup = context.AfterFunc(setupCtx, func() { _ = raw.Close(); close(setupStopped) })
		}
		return raw, err
	}})
	if err != nil {
		return 0, ErrUnavailable
	}
	channel, err := conn.Channel()
	if err != nil {
		return 0, ErrUnavailable
	}
	if err := declareTopology(channel, c.Options); err != nil {
		return 0, ErrUnavailable
	}
	if err := channel.Qos(1, 0, false); err != nil {
		return 0, ErrUnavailable
	}
	deliveries, err := channel.ConsumeWithContext(ctx, c.Options.Queue, "", false, false, false, false, nil)
	if err != nil {
		return 0, ErrUnavailable
	}
	if !stopSetup() {
		<-setupStopped
		stopSetup = nil
		return 0, ErrUnavailable
	}
	stopSetup = nil
	if setupCtx.Err() != nil {
		return 0, ErrUnavailable
	}
	cancel()
	stopped := make(chan struct{})
	stop := context.AfterFunc(ctx, func() { _ = raw.Close(); close(stopped) })
	defer func() {
		if !stop() {
			<-stopped
		}
	}()
	closed := channel.NotifyClose(make(chan *amqp.Error, 1))
	acked := 0
	for {
		select {
		case <-ctx.Done():
			return acked, ctx.Err()
		case <-closed:
			return acked, ErrUnavailable
		case message, ok := <-deliveries:
			if !ok {
				return acked, ErrUnavailable
			}
			for {
				workCtx, stopWork := context.WithTimeout(ctx, c.HandlerTimeout)
				err := handler.Handle(workCtx, regional.Notification{ID: message.MessageId, ContentType: message.ContentType, Type: message.Type, Body: message.Body})
				stopWork()
				if ctx.Err() != nil {
					return acked, ctx.Err()
				}
				permanent := errors.Is(err, regional.ErrInvalidNotification) || errors.Is(err, regional.ErrNotificationConflict)
				if err == nil || permanent {
					if err := raw.SetWriteDeadline(time.Now().Add(c.Options.Timeout)); err != nil {
						return acked, ErrUnavailable
					}
					var ackErr error
					if permanent {
						ackErr = message.Reject(false)
					} else {
						ackErr = message.Ack(false)
					}
					if ackErr != nil {
						return acked, ErrUnavailable
					}
					if err := raw.SetWriteDeadline(time.Time{}); err != nil {
						return acked, ErrUnavailable
					}
					if !permanent {
						acked++
					}
					break
				}
				timer := time.NewTimer(c.RetryDelay)
				select {
				case <-ctx.Done():
					timer.Stop()
					return acked, ctx.Err()
				case <-closed:
					timer.Stop()
					return acked, ErrUnavailable
				case <-timer.C:
				}
			}
		}
	}
}
