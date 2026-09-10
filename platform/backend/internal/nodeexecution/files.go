package nodeexecution

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

var ErrPathOutsideRoot = errors.New("path escapes scoped root")

type ScopedRoot struct {
	path string
}

func NewScopedRoot(path string) (ScopedRoot, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return ScopedRoot{}, err
	}
	if err := os.MkdirAll(absolute, 0o750); err != nil {
		return ScopedRoot{}, err
	}
	return ScopedRoot{path: filepath.Clean(absolute)}, nil
}

func (r ScopedRoot) Resolve(relative string) (string, error) {
	if relative == "" || filepath.IsAbs(relative) {
		return "", ErrPathOutsideRoot
	}
	clean := filepath.Clean(relative)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", ErrPathOutsideRoot
	}
	resolved := filepath.Join(r.path, clean)
	rel, err := filepath.Rel(r.path, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", ErrPathOutsideRoot
	}
	current := r.path
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			break
		}
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", ErrPathOutsideRoot
		}
	}
	return resolved, nil
}

type TransferResult struct {
	SizeBytes int64
	Checksum  string
}

type ObjectTransfer interface {
	Upload(context.Context, string, io.Reader) error
	Download(context.Context, string) (io.ReadCloser, error)
}

type BackupJob struct {
	SourceRelative  string
	SignedUploadURL string
}

type RestoreJob struct {
	TargetRelative    string
	SignedDownloadURL string
}

func (r ScopedRoot) Backup(ctx context.Context, transfer ObjectTransfer, job BackupJob) (TransferResult, error) {
	source, err := r.Resolve(job.SourceRelative)
	if err != nil {
		return TransferResult{}, err
	}
	digest := sha256.New()
	counter := &byteCounter{}
	reader, writer := io.Pipe()
	done := make(chan error, 1)
	go func() {
		err := writeArchive(source, io.MultiWriter(writer, digest, counter))
		_ = writer.CloseWithError(err)
		done <- err
	}()
	uploadErr := transfer.Upload(ctx, job.SignedUploadURL, reader)
	if uploadErr != nil {
		_ = reader.CloseWithError(uploadErr)
	}
	archiveErr := <-done
	if uploadErr != nil {
		return TransferResult{}, uploadErr
	}
	if archiveErr != nil {
		return TransferResult{}, archiveErr
	}
	return TransferResult{SizeBytes: counter.total, Checksum: hex.EncodeToString(digest.Sum(nil))}, nil
}

func writeArchive(source string, destination io.Writer) error {
	gzipWriter := gzip.NewWriter(destination)
	tarWriter := tar.NewWriter(gzipWriter)
	err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return ErrPathOutsideRoot
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return nil
		}
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(relative)
		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(tarWriter, file)
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
	if err != nil {
		_ = tarWriter.Close()
		_ = gzipWriter.Close()
		return err
	}
	if err := tarWriter.Close(); err != nil {
		_ = gzipWriter.Close()
		return err
	}
	return gzipWriter.Close()
}

type byteCounter struct {
	total int64
}

func (c *byteCounter) Write(data []byte) (int, error) {
	c.total += int64(len(data))
	return len(data), nil
}

func (r ScopedRoot) Restore(ctx context.Context, transfer ObjectTransfer, job RestoreJob) error {
	target, err := r.Resolve(job.TargetRelative)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(target, 0o750); err != nil {
		return err
	}
	stream, err := transfer.Download(ctx, job.SignedDownloadURL)
	if err != nil {
		return err
	}
	defer stream.Close()
	gzipReader, err := gzip.NewReader(stream)
	if err != nil {
		return err
	}
	defer gzipReader.Close()
	reader := tar.NewReader(gzipReader)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		archivePath := filepath.Clean(filepath.FromSlash(header.Name))
		if filepath.IsAbs(archivePath) || archivePath == ".." || strings.HasPrefix(archivePath, ".."+string(filepath.Separator)) {
			return ErrPathOutsideRoot
		}
		destination, err := r.Resolve(filepath.Join(job.TargetRelative, archivePath))
		if err != nil {
			return err
		}
		if destination == target {
			continue
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(destination, 0o750); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(destination), 0o750); err != nil {
				return err
			}
			file, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o640)
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(file, reader)
			closeErr := file.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
		default:
			return ErrPathOutsideRoot
		}
	}
}
