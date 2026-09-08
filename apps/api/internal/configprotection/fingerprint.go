package configprotection

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strings"
)

// Fingerprinter uses separate externally provisioned keys from encryption.
// Keep old keys while operations referencing them can still be retried.
type Fingerprinter struct {
	active string
	keys   map[string][]byte
}

func NewFingerprinter(active string, keys map[string][]byte) (*Fingerprinter, error) {
	f := &Fingerprinter{active: active, keys: make(map[string][]byte, len(keys))}
	for id, key := range keys {
		if id == "" || strings.TrimSpace(id) != id || len(key) != 32 {
			return nil, ErrInvalidConfiguration
		}
		f.keys[id] = append([]byte(nil), key...)
	}
	if f.keys[active] == nil {
		return nil, ErrInvalidConfiguration
	}
	return f, nil
}

func (f *Fingerprinter) digest(id string, input []byte) []byte {
	h := hmac.New(sha256.New, f.keys[id])
	_, _ = h.Write([]byte("gamepanel.create-request.v1\x00"))
	_, _ = h.Write(input)
	return h.Sum(nil)
}
func (f *Fingerprinter) Sum(input []byte) (string, error) {
	return "h1." + base64.RawURLEncoding.EncodeToString([]byte(f.active)) + "." + hex.EncodeToString(f.digest(f.active, input)), nil
}
func (f *Fingerprinter) Matches(stored string, input []byte) (bool, error) {
	parts := strings.Split(stored, ".")
	if len(parts) != 3 || parts[0] != "h1" {
		return false, nil
	}
	id, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || f.keys[string(id)] == nil {
		return false, ErrInvalidConfiguration
	}
	digest, err := hex.DecodeString(parts[2])
	if err != nil || len(digest) != sha256.Size {
		return false, ErrInvalidConfiguration
	}
	return hmac.Equal(digest, f.digest(string(id), input)), nil
}
