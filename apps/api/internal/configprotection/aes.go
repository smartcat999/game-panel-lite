// Package configprotection implements authenticated configuration encryption.
// Key provisioning and permission to decrypt belong to the calling application.
package configprotection

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/json"
	"errors"
	"strings"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
)

var ErrInvalidConfiguration = errors.New("configuration protection rejected input")

const envelopeVersion byte = 1

type Protector struct {
	active   string
	keys     map[string]cipher.AEAD
	maxBytes int
}

// New snapshots external AES-256 keys. The active key seals new revisions;
// retained keys only open historical revisions. Issuers must rotate each key
// before 2^32 encryptions across all processes, as required by Go's random GCM.
func New(active string, keys map[string][]byte, maxBytes int) (*Protector, error) {
	if active == "" || maxBytes < 1 || uint64(maxBytes) > ((1<<32)-2)*16 {
		return nil, ErrInvalidConfiguration
	}
	p := &Protector{active: active, keys: make(map[string]cipher.AEAD, len(keys)), maxBytes: maxBytes}
	for id, key := range keys {
		if id == "" || strings.TrimSpace(id) != id || len(key) != 32 {
			return nil, ErrInvalidConfiguration
		}
		block, err := aes.NewCipher(key)
		if err != nil {
			return nil, ErrInvalidConfiguration
		}
		aead, err := cipher.NewGCMWithRandomNonce(block)
		if err != nil {
			return nil, ErrInvalidConfiguration
		}
		p.keys[id] = aead
	}
	if p.keys[active] == nil {
		return nil, ErrInvalidConfiguration
	}
	return p, nil
}

func associatedData(id string, binding instances.ConfigurationBinding) ([]byte, error) {
	if binding.Validate() != nil {
		return nil, ErrInvalidConfiguration
	}
	return json.Marshal(struct {
		Purpose string                         `json:"purpose"`
		Version byte                           `json:"version"`
		KeyID   string                         `json:"keyId"`
		Binding instances.ConfigurationBinding `json:"binding"`
	}{"gamepanel.server-configuration", envelopeVersion, id, binding})
}

func (p *Protector) Seal(ctx context.Context, binding instances.ConfigurationBinding, plaintext []byte) (instances.ProtectedConfiguration, error) {
	if err := ctx.Err(); err != nil {
		return instances.ProtectedConfiguration{}, err
	}
	if len(plaintext) == 0 || len(plaintext) > p.maxBytes {
		return instances.ProtectedConfiguration{}, ErrInvalidConfiguration
	}
	aad, err := associatedData(p.active, binding)
	if err != nil {
		return instances.ProtectedConfiguration{}, err
	}
	encoded := p.keys[p.active].Seal([]byte{envelopeVersion}, nil, plaintext, aad)
	if err := ctx.Err(); err != nil {
		return instances.ProtectedConfiguration{}, err
	}
	return instances.ProtectedConfiguration{KeyID: p.active, Ciphertext: encoded}, nil
}

// Open authenticates the envelope and immutable binding before returning any
// plaintext. Possessing a decryptor is not evidence of a current execution grant.
func (p *Protector) Open(ctx context.Context, binding instances.ConfigurationBinding, protected instances.ProtectedConfiguration) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	aead := p.keys[protected.KeyID]
	if aead == nil || len(protected.Ciphertext) < 1+aead.Overhead() || protected.Ciphertext[0] != envelopeVersion || len(protected.Ciphertext)-1-aead.Overhead() > p.maxBytes {
		return nil, ErrInvalidConfiguration
	}
	aad, err := associatedData(protected.KeyID, binding)
	if err != nil {
		return nil, err
	}
	plaintext, err := aead.Open(nil, nil, protected.Ciphertext[1:], aad)
	if err != nil || len(plaintext) == 0 {
		return nil, ErrInvalidConfiguration
	}
	if err := ctx.Err(); err != nil {
		clear(plaintext)
		return nil, err
	}
	return plaintext, nil
}
