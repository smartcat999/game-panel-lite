package gameconfig

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider"
)

var ErrInvalidLogicalConfiguration = errors.New("invalid logical server configuration")

// LogicalNormalizer has no persistence or runtime dependency. Only providers
// declaring the logical configuration contract may prepare global revisions.
type LogicalNormalizer struct {
	Providers interface {
		Get(domain.ProviderKey) (provider.GameProvider, bool)
	}
	MaxBytes int
}

func (n LogicalNormalizer) Normalize(ctx context.Context, key, gameVersion string, schema int, raw []byte) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if n.Providers == nil || n.MaxBytes < 1 || len(raw) == 0 || len(raw) > n.MaxBytes || !utf8.Valid(raw) || schema < 1 {
		return nil, ErrInvalidLogicalConfiguration
	}
	p, ok := n.Providers.Get(domain.ProviderKey(key))
	if !ok || provider.CheckConfigVersion(p, schema) != nil {
		return nil, ErrInvalidLogicalConfiguration
	}
	knownVersion := false
	for _, version := range p.Versions() {
		if version == gameVersion {
			knownVersion = true
			break
		}
	}
	logical, ok := p.(provider.LogicalConfigProvider)
	if !knownVersion || !ok {
		return nil, ErrInvalidLogicalConfiguration
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		return nil, ErrInvalidLogicalConfiguration
	}
	input := map[string]any{}
	seen := map[string]bool{}
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, ErrInvalidLogicalConfiguration
		}
		field, ok := token.(string)
		if !ok || seen[strings.ToLower(field)] {
			return nil, ErrInvalidLogicalConfiguration
		}
		seen[strings.ToLower(field)] = true
		var value any
		if decoder.Decode(&value) != nil {
			return nil, ErrInvalidLogicalConfiguration
		}
		input[field] = value
	}
	if token, err := decoder.Token(); err != nil || token != json.Delim('}') {
		return nil, ErrInvalidLogicalConfiguration
	}
	if decoder.Decode(new(any)) != io.EOF {
		return nil, ErrInvalidLogicalConfiguration
	}
	normalized, err := logical.NormalizeLogicalConfig(input)
	if err != nil || normalized == nil {
		return nil, ErrInvalidLogicalConfiguration
	}
	encoded, err := json.Marshal(normalized)
	if err != nil || len(encoded) > n.MaxBytes {
		return nil, ErrInvalidLogicalConfiguration
	}
	if err := ctx.Err(); err != nil {
		clear(encoded)
		return nil, err
	}
	return encoded, nil
}
