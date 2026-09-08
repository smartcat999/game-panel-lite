package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/configprotection"
)

func loadConfigurationKeys(path string, maxBytes int) (*configprotection.Protector, error) {
	rejected := errors.New("cannot load configuration keyring")
	file, err := os.Open(path)
	if err != nil {
		return nil, rejected
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil, rejected
	}
	raw, err := io.ReadAll(io.LimitReader(file, (1<<20)+1))
	defer clear(raw)
	if err != nil || len(raw) > 1<<20 {
		return nil, rejected
	}
	var config struct {
		Active string            `json:"active"`
		Keys   map[string][]byte `json:"keys"`
	}
	defer func() {
		for _, key := range config.Keys {
			clear(key)
		}
	}()
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&config) != nil {
		return nil, rejected
	}
	if decoder.Decode(new(any)) != io.EOF {
		return nil, rejected
	}
	protector, err := configprotection.New(config.Active, config.Keys, maxBytes)
	if err != nil {
		return nil, rejected
	}
	return protector, nil
}
