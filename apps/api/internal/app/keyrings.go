package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/configprotection"
)

const maxKeyringBytes = 1 << 20

var errKeyringUnavailable = errors.New("logical instance keyring unavailable")

type keyringFile struct {
	Active string            `json:"active"`
	Keys   map[string][]byte `json:"keys"`
}

func loadConfigurationProtector(path string, maxPlaintextBytes int) (*configprotection.Protector, error) {
	keyring, err := loadKeyring(path)
	if err != nil {
		return nil, err
	}
	defer clearKeyring(keyring)
	protector, err := configprotection.New(keyring.Active, keyring.Keys, maxPlaintextBytes)
	if err != nil {
		return nil, errKeyringUnavailable
	}
	return protector, nil
}

func loadRequestFingerprinter(path string) (*configprotection.Fingerprinter, error) {
	keyring, err := loadKeyring(path)
	if err != nil {
		return nil, err
	}
	defer clearKeyring(keyring)
	fingerprinter, err := configprotection.NewFingerprinter(keyring.Active, keyring.Keys)
	if err != nil {
		return nil, errKeyringUnavailable
	}
	return fingerprinter, nil
}

func loadKeyring(path string) (keyringFile, error) {
	if path == "" {
		return keyringFile{}, errKeyringUnavailable
	}
	file, err := os.Open(path)
	if err != nil {
		return keyringFile{}, errKeyringUnavailable
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return keyringFile{}, errKeyringUnavailable
	}
	raw, err := io.ReadAll(io.LimitReader(file, maxKeyringBytes+1))
	if err != nil || len(raw) > maxKeyringBytes {
		clear(raw)
		return keyringFile{}, errKeyringUnavailable
	}
	defer clear(raw)
	var keyring keyringFile
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&keyring) != nil || decoder.Decode(new(any)) != io.EOF {
		clearKeyring(keyring)
		return keyringFile{}, errKeyringUnavailable
	}
	return keyring, nil
}

func clearKeyring(keyring keyringFile) {
	for _, key := range keyring.Keys {
		clear(key)
	}
}
