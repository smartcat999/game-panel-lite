package authorization

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
)

type MemoryStore struct {
	mu       sync.RWMutex
	bindings []RoleBinding
}

func NewMemoryStore(bindings []RoleBinding) (*MemoryStore, error) {
	store := &MemoryStore{}
	for _, binding := range bindings {
		if err := store.Put(context.Background(), binding); err != nil {
			return nil, err
		}
	}
	return store, nil
}

func (s *MemoryStore) Put(_ context.Context, binding RoleBinding) error {
	if err := ValidateBinding(binding); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for index, current := range s.bindings {
		if current.ID == binding.ID {
			s.bindings[index] = binding
			return nil
		}
	}
	s.bindings = append(s.bindings, binding)
	return nil
}

func (s *MemoryStore) BindingsForPrincipal(_ context.Context, principalID PrincipalID) ([]RoleBinding, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]RoleBinding, 0, len(s.bindings))
	for _, binding := range s.bindings {
		if binding.PrincipalID == principalID {
			result = append(result, binding)
		}
	}
	return result, nil
}

type PostgresStore struct {
	database *sql.DB
}

func NewPostgresStore(database *sql.DB) *PostgresStore {
	return &PostgresStore{database: database}
}

func (s *PostgresStore) BindingsForPrincipal(ctx context.Context, principalID PrincipalID) ([]RoleBinding, error) {
	rows, err := s.database.QueryContext(ctx, `
		SELECT id, principal_id, role, scope_type, scope_id
		FROM authorization_role_bindings
		WHERE principal_id = $1
		ORDER BY id
		LIMIT 1001`, principalID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []RoleBinding
	for rows.Next() {
		var binding RoleBinding
		if err := rows.Scan(&binding.ID, &binding.PrincipalID, &binding.Role, &binding.Scope.Type, &binding.Scope.ID); err != nil {
			return nil, err
		}
		result = append(result, binding)
		if len(result) > 1000 {
			return nil, ErrTooManyBindings
		}
	}
	return result, rows.Err()
}

func (s *PostgresStore) Put(ctx context.Context, binding RoleBinding) error {
	if err := ValidateBinding(binding); err != nil {
		return err
	}
	_, err := s.database.ExecContext(ctx, `
		INSERT INTO authorization_role_bindings (id, principal_id, role, scope_type, scope_id, created_at)
		VALUES ($1, $2, $3, $4, $5, now())
		ON CONFLICT (principal_id, role, scope_type, scope_id) DO NOTHING`, binding.ID, binding.PrincipalID, binding.Role, binding.Scope.Type, binding.Scope.ID)
	if err != nil {
		return fmt.Errorf("put role binding: %w", err)
	}
	return nil
}
