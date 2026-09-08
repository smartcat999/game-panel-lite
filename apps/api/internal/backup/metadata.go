package backup

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"github.com/smartcat999/game-panel-lite/internal/archive"
	"io"
)

const metadataPath = archive.MetadataPath
const maxMetadataBytes = archive.MaxMetadataBytes

type Metadata = archive.Metadata

// WithMetadata returns an independent service configured for archive creation.
func (s *Service) WithMetadata(metadata Metadata) *Service {
	return &Service{dataDir: s.dataDir, metadata: &metadata}
}

func validateMetadata(metadata Metadata) error { return metadata.Validate() }
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
