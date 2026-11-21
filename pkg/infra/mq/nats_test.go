package mq

import (
	"slices"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
)

func TestDefaultJetStreamConfigBounds(t *testing.T) {
	cfg := defaultJetStreamConfig(nats.MemoryStorage)
	if cfg.Name != natsJetStreamName {
		t.Fatalf("unexpected stream name: %q", cfg.Name)
	}
	if cfg.Retention != nats.LimitsPolicy || cfg.Discard != nats.DiscardOld {
		t.Fatalf("unexpected retention/discard: %v / %v", cfg.Retention, cfg.Discard)
	}
	if cfg.Storage != nats.MemoryStorage {
		t.Fatalf("expected MemoryStorage preserved, got %v", cfg.Storage)
	}
	if cfg.MaxAge != 72*time.Hour || cfg.MaxMsgs != 100_000 || cfg.MaxBytes != 256*1024*1024 || cfg.Duplicates != 2*time.Minute {
		t.Fatalf("unexpected bounds: maxAge=%v maxMsgs=%d maxBytes=%d dup=%v", cfg.MaxAge, cfg.MaxMsgs, cfg.MaxBytes, cfg.Duplicates)
	}
	if !slices.Equal(cfg.Subjects, natsStreamSubjects) {
		t.Fatalf("unexpected subjects: %v", cfg.Subjects)
	}
	cfg.Subjects[0] = "mutated.>"
	if natsStreamSubjects[0] == "mutated.>" {
		t.Fatal("expected Subjects slice to be cloned")
	}
}
