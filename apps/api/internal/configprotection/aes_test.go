package configprotection

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"sync"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
)

func TestAuthenticatedConfiguration(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	keys := map[string][]byte{"old": key, "alias": key}
	p, err := New("old", keys, 1024)
	if err != nil {
		t.Fatal(err)
	}
	binding := instances.ConfigurationBinding{OrganizationID: "tenant", ServerID: "server", RevisionID: "revision", SpecGeneration: 1, ProviderKey: "test", ConfigSchemaVersion: 1}
	ctx := context.Background()
	plaintext := []byte(`{"password":"temporary-test"}`)
	one, err := p.Seal(ctx, binding, plaintext)
	if err != nil {
		t.Fatal(err)
	}
	two, err := p.Seal(ctx, binding, plaintext)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(one.Ciphertext, two.Ciphertext) || bytes.Contains(one.Ciphertext, plaintext) {
		t.Fatal("encryption is deterministic or exposes plaintext")
	}
	clear(key)
	delete(keys, "old")
	opened, err := p.Open(ctx, binding, one)
	if err != nil || !bytes.Equal(opened, plaintext) {
		t.Fatalf("roundtrip or key snapshot: %v", err)
	}
	for _, mutate := range []func(*instances.ConfigurationBinding){
		func(b *instances.ConfigurationBinding) { b.OrganizationID = "other" }, func(b *instances.ConfigurationBinding) { b.ServerID = "other" }, func(b *instances.ConfigurationBinding) { b.RevisionID = "other" }, func(b *instances.ConfigurationBinding) { b.SpecGeneration++ }, func(b *instances.ConfigurationBinding) { b.ProviderKey = "other" }, func(b *instances.ConfigurationBinding) { b.ConfigSchemaVersion++ },
	} {
		wrong := binding
		mutate(&wrong)
		if result, err := p.Open(ctx, wrong, one); !errors.Is(err, ErrInvalidConfiguration) || result != nil {
			t.Fatal("binding substitution accepted")
		}
	}
	for i := range one.Ciphertext {
		bad := one
		bad.Ciphertext = bytes.Clone(one.Ciphertext)
		bad.Ciphertext[i] ^= 1
		if result, err := p.Open(ctx, binding, bad); !errors.Is(err, ErrInvalidConfiguration) || result != nil {
			t.Fatalf("tampering accepted at %d", i)
		}
	}
	for _, id := range []string{"missing", "alias"} {
		bad := one
		bad.KeyID = id
		if result, err := p.Open(ctx, binding, bad); !errors.Is(err, ErrInvalidConfiguration) || result != nil {
			t.Fatal("key substitution accepted")
		}
	}
	if _, err := p.Seal(ctx, binding, make([]byte, 1025)); !errors.Is(err, ErrInvalidConfiguration) {
		t.Fatal("size limit ignored")
	}
	ctx, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := p.Seal(ctx, binding, plaintext); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation lost")
	}
}

func TestRotationAndConcurrentUse(t *testing.T) {
	oldKey, newKey := make([]byte, 32), make([]byte, 32)
	if _, err := rand.Read(oldKey); err != nil {
		t.Fatal(err)
	}
	if _, err := rand.Read(newKey); err != nil {
		t.Fatal(err)
	}
	old, err := New("old", map[string][]byte{"old": oldKey}, 100)
	if err != nil {
		t.Fatal(err)
	}
	rotated, err := New("new", map[string][]byte{"old": oldKey, "new": newKey}, 100)
	if err != nil {
		t.Fatal(err)
	}
	binding := instances.ConfigurationBinding{OrganizationID: "tenant", ServerID: "server", RevisionID: "revision", SpecGeneration: 1, ProviderKey: "test", ConfigSchemaVersion: 1}
	ctx := context.Background()
	prior, err := old.Seal(ctx, binding, []byte("secret"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rotated.Open(ctx, binding, prior); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sealed, err := rotated.Seal(ctx, binding, []byte("secret"))
			if err != nil {
				t.Error(err)
				return
			}
			if sealed.KeyID != "new" {
				t.Error("old key used for new ciphertext")
			}
			if _, err := rotated.Open(ctx, binding, sealed); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
}
