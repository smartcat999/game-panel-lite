package messaging

import (
	"context"
	"fmt"
	"sync"
	"time"

	contract "github.com/smartcat999/game-panel-lite/platform/backend/internal/contracts/v1"
)

type OutboxMessage struct {
	SchemaVersion  int                     `json:"schemaVersion"`
	ID             contract.EventID        `json:"id"`
	MessageType    string                  `json:"messageType"`
	IdempotencyKey contract.IdempotencyKey `json:"idempotencyKey"`
	Payload        any                     `json:"payload"`
	CreatedAt      time.Time               `json:"createdAt"`
}

type Module struct {
	mu     sync.RWMutex
	outbox []OutboxMessage
	inbox  map[contract.EventID]time.Time
	nextID int
}

func New() *Module {
	return &Module{inbox: make(map[contract.EventID]time.Time)}
}

func (m *Module) Publish(messageType string, key contract.IdempotencyKey, payload any, now time.Time) OutboxMessage {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextID++
	message := OutboxMessage{
		SchemaVersion: 1, ID: contract.EventID(fmt.Sprintf("evt_%06d", m.nextID)), MessageType: messageType,
		IdempotencyKey: key, Payload: payload, CreatedAt: now,
	}
	m.outbox = append(m.outbox, message)
	return message
}

func (m *Module) Outbox(_ context.Context) []OutboxMessage {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]OutboxMessage(nil), m.outbox...)
}

func (m *Module) Count(messageType string) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	count := 0
	for _, message := range m.outbox {
		if message.MessageType == messageType {
			count++
		}
	}
	return count
}

func (m *Module) HandleOnce(messageID contract.EventID, now time.Time, handler func() error) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, handled := m.inbox[messageID]; handled {
		return false, nil
	}
	if err := handler(); err != nil {
		return false, err
	}
	m.inbox[messageID] = now
	return true, nil
}
