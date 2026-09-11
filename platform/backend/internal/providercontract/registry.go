package providercontract

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"
)

type Store interface {
	Put(context.Context, Manifest) error
	ByID(context.Context, string) (Manifest, error)
	ByIDs(context.Context, []string) ([]Manifest, error)
	List(context.Context, int) ([]Manifest, error)
}

type Registry struct {
	store Store
	key   []byte
}

func NewRegistry(store Store, signingKey []byte) *Registry {
	return &Registry{store: store, key: append([]byte(nil), signingKey...)}
}

func (r *Registry) Publish(ctx context.Context, manifest Manifest, now time.Time) (Manifest, error) {
	manifest.ManifestDigest = ""
	manifest.Signature = ""
	manifest.PublishedAt = now.UTC()
	if err := validateManifest(manifest); err != nil {
		return Manifest{}, err
	}
	if manifest.ModCatalog != nil {
		catalog := *manifest.ModCatalog
		catalog.Digest, catalog.Signature = "", ""
		digest, signature, err := sign(catalog, r.key)
		if err != nil {
			return Manifest{}, err
		}
		catalog.Digest, catalog.Signature = digest, signature
		manifest.ModCatalog = &catalog
	}
	digest, signature, err := sign(unsignedManifest(manifest), r.key)
	if err != nil {
		return Manifest{}, err
	}
	manifest.ManifestDigest, manifest.Signature = digest, signature
	if err := r.store.Put(ctx, manifest); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func (r *Registry) Verified(ctx context.Context, id string) (Manifest, error) {
	manifest, err := r.store.ByID(ctx, id)
	if err != nil {
		return Manifest{}, err
	}
	if err := verifyManifest(manifest, r.key); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func (r *Registry) VerifiedByIDs(ctx context.Context, ids []string) (map[string]Manifest, error) {
	unique := make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if id == "" {
			return nil, ErrInvalidManifest
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		unique = append(unique, id)
	}
	if len(unique) > 100 {
		return nil, ErrInvalidManifest
	}
	items, err := r.store.ByIDs(ctx, unique)
	if err != nil {
		return nil, err
	}
	result := make(map[string]Manifest, len(items))
	for _, item := range items {
		if err := verifyManifest(item, r.key); err != nil {
			return nil, err
		}
		result[item.ProviderReleaseID] = item
	}
	if len(result) != len(unique) {
		return nil, ErrInvalidManifest
	}
	return result, nil
}

func (r *Registry) List(ctx context.Context, limit int) ([]Manifest, error) {
	if limit < 1 || limit > 100 {
		limit = 100
	}
	items, err := r.store.List(ctx, limit)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if err := verifyManifest(item, r.key); err != nil {
			return nil, err
		}
	}
	return items, nil
}

func verifyManifest(manifest Manifest, key []byte) error {
	if err := validateManifest(manifest); err != nil {
		return err
	}
	digest, signature, err := sign(unsignedManifest(manifest), key)
	if err != nil {
		return err
	}
	if digest != manifest.ManifestDigest || !hmac.Equal([]byte(signature), []byte(manifest.Signature)) {
		return ErrInvalidSignature
	}
	if manifest.ModCatalog != nil {
		catalog := *manifest.ModCatalog
		wantDigest, wantSignature := catalog.Digest, catalog.Signature
		catalog.Digest, catalog.Signature = "", ""
		digest, signature, err = sign(catalog, key)
		if err != nil || digest != wantDigest || !hmac.Equal([]byte(signature), []byte(wantSignature)) {
			return ErrInvalidSignature
		}
	}
	return nil
}

func sign(value any, key []byte) (string, string, error) {
	if len(key) < 32 {
		return "", "", ErrInvalidSignature
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", "", err
	}
	digest := sha256.Sum256(encoded)
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(digest[:])
	return "sha256:" + hex.EncodeToString(digest[:]), hex.EncodeToString(mac.Sum(nil)), nil
}

func unsignedManifest(manifest Manifest) Manifest {
	manifest.ManifestDigest = ""
	manifest.Signature = ""
	return manifest
}

func IsImmutable(err error) bool { return errors.Is(err, ErrImmutableRelease) }
