package backup

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
)

const metadataPath = ".gamepanel-backup.json"
const maxMetadataBytes = 16 << 10

// Metadata travels with the archive. It describes compatibility, not authenticity.
type Metadata struct {
	FormatVersion int    `json:"formatVersion"`
	GameKey       string `json:"gameKey"`
	ProviderKey   string `json:"providerKey"`
	ConfigVersion int    `json:"configVersion"`
}

// WithMetadata returns an independent service configured for archive creation.
func (s *Service) WithMetadata(metadata Metadata) *Service {
	return &Service{dataDir: s.dataDir, metadata: &metadata}
}

func validateMetadata(metadata Metadata) error {
	if metadata.FormatVersion != 1 || metadata.ConfigVersion < 1 || metadata.ProviderKey == "" || metadata.GameKey == "" {
		return fmt.Errorf("invalid or unsupported backup metadata")
	}
	return nil
}
func writeMetadata(writer *zip.Writer, metadata Metadata) error {
	if err := validateMetadata(metadata); err != nil {
		return err
	}
	payload, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	if len(payload) > maxMetadataBytes {
		return fmt.Errorf("backup metadata exceeds size limit")
	}
	file, err := writer.Create(metadataPath)
	if err != nil {
		return err
	}
	_, err = file.Write(payload)
	return err
}
func readMetadata(files []*zip.File) (Metadata, error) {
	// Legacy archives have no source identity and use original configuration format.
	metadata := Metadata{ConfigVersion: 1}
	found := false
	for _, file := range files {
		if file.Name != metadataPath {
			continue
		}
		if found {
			return Metadata{}, fmt.Errorf("duplicate backup metadata")
		}
		found = true
		if file.UncompressedSize64 > maxMetadataBytes {
			return Metadata{}, fmt.Errorf("backup metadata exceeds size limit")
		}
		reader, err := file.Open()
		if err != nil {
			return Metadata{}, err
		}
		payload, err := io.ReadAll(io.LimitReader(reader, maxMetadataBytes+1))
		closeErr := reader.Close()
		if err != nil {
			return Metadata{}, err
		}
		if closeErr != nil {
			return Metadata{}, closeErr
		}
		if len(payload) > maxMetadataBytes {
			return Metadata{}, fmt.Errorf("backup metadata exceeds size limit")
		}
		metadata = Metadata{}
		if err := json.Unmarshal(payload, &metadata); err != nil {
			return Metadata{}, err
		}
		if err := validateMetadata(metadata); err != nil {
			return Metadata{}, err
		}
	}
	return metadata, nil
}
