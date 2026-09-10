package providercontract

import (
	"context"
	"database/sql"
	"reflect"
	"sort"
	"sync"
)

type MemoryStore struct {
	mu    sync.RWMutex
	items map[string]Manifest
}

func NewMemoryStore() *MemoryStore { return &MemoryStore{items: make(map[string]Manifest)} }

func (s *MemoryStore) Put(_ context.Context, manifest Manifest) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if previous, exists := s.items[manifest.ProviderReleaseID]; exists {
		if reflect.DeepEqual(previous, manifest) {
			return nil
		}
		return ErrImmutableRelease
	}
	s.items[manifest.ProviderReleaseID] = manifest
	return nil
}

func (s *MemoryStore) ByID(_ context.Context, id string) (Manifest, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.items[id]
	if !ok {
		return Manifest{}, sql.ErrNoRows
	}
	return item, nil
}

func (s *MemoryStore) List(_ context.Context, limit int) ([]Manifest, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := make([]string, 0, len(s.items))
	for id := range s.items {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	if len(ids) > limit {
		ids = ids[:limit]
	}
	items := make([]Manifest, len(ids))
	for index, id := range ids {
		items[index] = s.items[id]
	}
	return items, nil
}
