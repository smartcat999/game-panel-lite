package providercontract

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
)

type PostgresStore struct {
	database *sql.DB
}

func NewPostgresStore(database *sql.DB) *PostgresStore {
	return &PostgresStore{database: database}
}

func (s *PostgresStore) Put(ctx context.Context, manifest Manifest) error {
	encoded, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	result, err := s.database.ExecContext(ctx, "INSERT INTO provider_releases (id,game_key,display_name,release_version,manifest,manifest_digest,published_at) VALUES ($1,$2,$3,$4,$5,$6,$7) ON CONFLICT (id) DO NOTHING", manifest.ProviderReleaseID, manifest.GameKey, manifest.DisplayName, manifest.ReleaseVersion, encoded, manifest.ManifestDigest, manifest.PublishedAt)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil || changed == 1 {
		return err
	}
	var stored []byte
	if err := s.database.QueryRowContext(ctx, "SELECT manifest FROM provider_releases WHERE id=$1", manifest.ProviderReleaseID).Scan(&stored); err != nil {
		return err
	}
	var existing Manifest
	if err := json.Unmarshal(stored, &existing); err != nil {
		return err
	}
	canonical, err := json.Marshal(existing)
	if err != nil {
		return err
	}
	if !bytes.Equal(canonical, encoded) {
		return ErrImmutableRelease
	}
	return nil
}

func (s *PostgresStore) ByID(ctx context.Context, id string) (Manifest, error) {
	var encoded []byte
	if err := s.database.QueryRowContext(ctx, "SELECT manifest FROM provider_releases WHERE id=$1", id).Scan(&encoded); err != nil {
		return Manifest{}, err
	}
	var manifest Manifest
	if err := json.Unmarshal(encoded, &manifest); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func (s *PostgresStore) List(ctx context.Context, limit int) ([]Manifest, error) {
	rows, err := s.database.QueryContext(ctx, "SELECT manifest FROM provider_releases ORDER BY game_key,release_version,id LIMIT $1", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Manifest
	for rows.Next() {
		var encoded []byte
		if err := rows.Scan(&encoded); err != nil {
			return nil, err
		}
		var manifest Manifest
		if err := json.Unmarshal(encoded, &manifest); err != nil {
			return nil, err
		}
		result = append(result, manifest)
	}
	return result, rows.Err()
}
