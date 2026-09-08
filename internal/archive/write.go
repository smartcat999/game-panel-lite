// Package archive writes game-independent archives from a caller-scoped filesystem.
package archive

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
)

const MetadataPath = ".gamepanel-backup.json"
const MaxMetadataBytes = 16 << 10

// Metadata describes compatibility, not authenticity or execution authority.
type Metadata struct {
	FormatVersion int    `json:"formatVersion"`
	GameKey       string `json:"gameKey"`
	ProviderKey   string `json:"providerKey"`
	ConfigVersion int    `json:"configVersion"`
}

func (m Metadata) Validate() error {
	if m.FormatVersion != 1 || m.ConfigVersion < 1 || m.ProviderKey == "" || m.GameKey == "" {
		return fmt.Errorf("invalid or unsupported backup metadata")
	}
	return nil
}

// Write preserves subtree paths relative to source. The caller must stabilize
// source, enforce authority and discard destination on any error. Neither source
// nor destination is closed here; opened files are closed before returning.
func Write(ctx context.Context, destination io.Writer, source fs.FS, subtree string, metadata *Metadata) (err error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if destination == nil || source == nil || !fs.ValidPath(subtree) {
		return fmt.Errorf("invalid archive source")
	}
	zipper := zip.NewWriter(destination)
	defer func() {
		err = errors.Join(err, zipper.Close())
		if err == nil {
			err = ctx.Err()
		}
	}()
	if metadata != nil {
		if err := metadata.Validate(); err != nil {
			return err
		}
		payload, err := json.Marshal(metadata)
		if err != nil {
			return err
		}
		if len(payload) > MaxMetadataBytes {
			return fmt.Errorf("backup metadata exceeds size limit")
		}
		file, err := zipper.Create(MetadataPath)
		if err != nil {
			return err
		}
		if _, err := file.Write(payload); err != nil {
			return err
		}
	}
	return fs.WalkDir(source, subtree, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("backup source contains a non-regular file")
		}
		if path == MetadataPath {
			return fmt.Errorf("backup source uses reserved metadata filename")
		}
		in, err := source.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		info, err := in.Stat()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("backup source is not a regular file")
		}
		stop := context.AfterFunc(ctx, func() { _ = in.Close() })
		defer stop()
		out, err := zipper.Create(path)
		if err != nil {
			return err
		}
		_, err = io.Copy(out, contextReader{ctx: ctx, source: in})
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	})
}

type contextReader struct {
	ctx    context.Context
	source io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.source.Read(p)
}
