package eventtransport

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	contract "github.com/smartcat999/game-panel-lite/platform/backend/internal/contracts/v1"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/messaging"
)

type JetStream struct {
	connection *nats.Conn
	context    nats.JetStreamContext
	mu         sync.Mutex
	consumers  map[string]*nats.Subscription
}

func Connect(url string, timeout time.Duration) (*JetStream, error) {
	connection, err := nats.Connect(url, nats.Name("gamepanel-platform"), nats.Timeout(timeout), nats.MaxReconnects(-1), nats.ReconnectWait(time.Second))
	if err != nil {
		return nil, err
	}
	stream, err := connection.JetStream(nats.PublishAsyncMaxPending(256))
	if err != nil {
		connection.Close()
		return nil, err
	}
	return &JetStream{connection: connection, context: stream, consumers: make(map[string]*nats.Subscription)}, nil
}

func (j *JetStream) EnsureStreams() error {
	configs := []*nats.StreamConfig{
		{Name: "GAMEPANEL_REGION", Subjects: []string{"gamepanel.region.*.>"}, Storage: nats.FileStorage, Retention: nats.LimitsPolicy, MaxAge: 7 * 24 * time.Hour, Duplicates: 10 * time.Minute},
		{Name: "GAMEPANEL_GLOBAL", Subjects: []string{"gamepanel.global.>"}, Storage: nats.FileStorage, Retention: nats.LimitsPolicy, MaxAge: 7 * 24 * time.Hour, Duplicates: 10 * time.Minute},
	}
	for _, config := range configs {
		if _, err := j.context.AddStream(config); err != nil && err != nats.ErrStreamNameAlreadyInUse {
			if _, updateErr := j.context.UpdateStream(config); updateErr != nil {
				return updateErr
			}
		}
	}
	return nil
}

func (j *JetStream) Publish(ctx context.Context, subject string, messageID contract.EventID, payload []byte) error {
	message := &nats.Msg{Subject: subject, Data: payload, Header: nats.Header{}}
	message.Header.Set(nats.MsgIdHdr, string(messageID))
	_, err := j.context.PublishMsg(message, nats.Context(ctx))
	return err
}

func (j *JetStream) Close() error {
	if err := j.connection.Drain(); err != nil {
		j.connection.Close()
		return err
	}
	return nil
}

func (j *JetStream) ConsumeOne(ctx context.Context, stream, durable, filter string, handler func([]byte) error) (bool, error) {
	subscription, err := j.consumer(stream, durable, filter)
	if err != nil {
		return false, err
	}
	messages, err := subscription.Fetch(1, nats.Context(ctx))
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) || errors.Is(err, nats.ErrTimeout) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if len(messages) == 0 {
		return false, nil
	}
	message := messages[0]
	if err := handler(message.Data); err != nil {
		var permanent messaging.PermanentError
		if errors.As(err, &permanent) {
			_ = message.Term()
			return true, err
		}
		_ = message.Nak()
		return true, err
	}
	return true, message.AckSync(nats.Context(ctx))
}

func (j *JetStream) consumer(stream, durable, filter string) (*nats.Subscription, error) {
	key := stream + "\x00" + durable + "\x00" + filter
	j.mu.Lock()
	defer j.mu.Unlock()
	if subscription := j.consumers[key]; subscription != nil {
		return subscription, nil
	}
	subscription, err := j.context.PullSubscribe(filter, durable, nats.BindStream(stream), nats.ManualAck(), nats.AckExplicit())
	if err != nil {
		return nil, err
	}
	j.consumers[key] = subscription
	return subscription, nil
}
