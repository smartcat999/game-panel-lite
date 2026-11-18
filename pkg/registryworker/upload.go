package registryworker

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
)

const maxUploadPartAttempts = 3

func (w *Worker) ensureBlobUploaded(ctx context.Context, rep *syncReporter, p *builtPayload) (string, error) {
	canonDigest, _, err := registryv1.NormalizeDigest("sha256:" + p.SHA256Hex)
	if err != nil {
		return "", err
	}
	if existing, gErr := w.cli.RegistryV1().Blobs().GetMetadata(ctx, canonDigest); gErr == nil && existing != nil {
		rep.Transfer(registryv1.ArtifactStageUploading, 96, p.SizeBytes, p.SizeBytes, 0, "命中去重对象存储缓存")
		rep.Logf("命中去重 Blob: %s (跳过上传)", canonDigest)
		return canonDigest, nil
	}
	rep.Stage(registryv1.ArtifactStageUploading, 74, "准备上传至对象存储")
	rep.Logf("创建对象存储上传会话 (SHA-256: %s, 大小: %d 字节)", canonDigest, p.SizeBytes)
	session, err := w.cli.RegistryV1().BlobUploads().Create(ctx, &registryv1.BlobUpload{
		Digest:    canonDigest,
		SizeBytes: p.SizeBytes,
		MediaType: p.MediaType,
		FileName:  p.FileName,
	})
	if err != nil {
		return "", fmt.Errorf("create blob upload session: %w", err)
	}
	if len(session.Parts) > 0 {
		completedParts, uErr := w.uploadMultipartFile(ctx, rep, p.LocalPath, session)
		if uErr != nil {
			_ = w.cli.RegistryV1().BlobUploads().Delete(ctx, session.Metadata.Name)
			return "", uErr
		}
		session.Parts = completedParts
	} else {
		if uErr := w.uploadSingleFile(ctx, rep, p.LocalPath, p.SizeBytes, p.MediaType, session.PresignedURL); uErr != nil {
			_ = w.cli.RegistryV1().BlobUploads().Delete(ctx, session.Metadata.Name)
			return "", uErr
		}
	}
	session.Digest = canonDigest
	rep.Stage(registryv1.ArtifactStageUploading, 98, "正在校验并提交对象存储")
	if _, err := w.cli.RegistryV1().BlobUploads().Complete(ctx, session); err != nil {
		return "", fmt.Errorf("complete blob upload: %w", err)
	}
	rep.Logf("对象存储上传并校验通过: %s", canonDigest)
	return canonDigest, nil
}

func shouldRetryUploadStatus(code int) bool {
	return code >= http.StatusInternalServerError || code == http.StatusTooManyRequests
}

func uploadRetryBackoff(ctx context.Context, base time.Duration, attempt int) error {
	if base <= 0 {
		base = 500 * time.Millisecond
	}
	timer := time.NewTimer(time.Duration(attempt) * base)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (w *Worker) uploadSingleFile(ctx context.Context, rep *syncReporter, path string, size int64, mediaType, putURL string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	rep.Transfer(registryv1.ArtifactStageUploading, 75, 0, size, 0, "正在上传至对象存储")
	for attempt := 1; attempt <= maxUploadPartAttempts; attempt++ {
		retryable, uErr := w.doSingleUploadAttempt(ctx, rep, f, size, mediaType, putURL)
		if uErr == nil {
			return nil
		}
		if !retryable || attempt == maxUploadPartAttempts {
			return uErr
		}
		rep.Logf("对象存储上传尝试 %d/%d 失败 (%v)，准备重试...", attempt, maxUploadPartAttempts, uErr)
		if wErr := uploadRetryBackoff(ctx, w.uploadRetryDelay, attempt); wErr != nil {
			return wErr
		}
	}
	return nil
}

func (w *Worker) doSingleUploadAttempt(ctx context.Context, rep *syncReporter, f io.ReaderAt, size int64, mediaType, putURL string) (bool, error) {
	pr := &progressReader{
		r:     io.NewSectionReader(f, 0, size),
		total: size,
		from:  75,
		to:    96,
		speed: &byteSpeedTracker{},
		onStep: func(pct int32, done, total, spd int64) {
			rep.Transfer(registryv1.ArtifactStageUploading, pct, done, total, spd, "正在上传至对象存储")
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, putURL, pr)
	if err != nil {
		return false, err
	}
	req.ContentLength = size
	if mediaType != "" {
		req.Header.Set("Content-Type", mediaType)
	}
	resp, err := w.uploadHC.Do(req)
	if err != nil {
		return true, fmt.Errorf("put s3 blob: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return shouldRetryUploadStatus(resp.StatusCode), fmt.Errorf("put s3 blob HTTP %d: %s", resp.StatusCode, string(body))
	}
	return false, nil
}

func (w *Worker) uploadMultipartFile(ctx context.Context, rep *syncReporter, path string, session *registryv1.BlobUpload) ([]registryv1.BlobUploadPart, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	speed := &byteSpeedTracker{}
	completed := make([]registryv1.BlobUploadPart, 0, len(session.Parts))
	for i, part := range session.Parts {
		offset := int64(part.PartNumber-1) * session.PartSize
		partLen := min(session.PartSize, session.SizeBytes-offset)
		etag, uErr := w.uploadMultipartPartWithRetry(ctx, rep, f, speed, i, len(session.Parts), session.SizeBytes, offset, partLen, part.URL)
		if uErr != nil {
			return nil, fmt.Errorf("upload part %d: %w", part.PartNumber, uErr)
		}
		completed = append(completed, registryv1.BlobUploadPart{PartNumber: part.PartNumber, ETag: etag})
		rep.Logf("完成分片上传 %d/%d", i+1, len(session.Parts))
	}
	return completed, nil
}

func (w *Worker) uploadMultipartPartWithRetry(
	ctx context.Context,
	rep *syncReporter,
	f io.ReaderAt,
	speed *byteSpeedTracker,
	partIdx, totalParts int,
	totalBytes, offset, partLen int64,
	putURL string,
) (string, error) {
	for attempt := 1; attempt <= maxUploadPartAttempts; attempt++ {
		pr := &progressReader{
			r:     io.NewSectionReader(f, offset, partLen),
			total: totalBytes,
			base:  offset,
			from:  75,
			to:    96,
			speed: speed,
			onStep: func(pct int32, done, total, spd int64) {
				rep.Transfer(registryv1.ArtifactStageUploading, pct, done, total, spd, fmt.Sprintf("分片上传 %d/%d", partIdx+1, totalParts))
			},
		}
		etag, retryable, err := w.doMultipartChunkAttempt(ctx, pr, partLen, offset, putURL)
		if err == nil {
			return etag, nil
		}
		if !retryable || attempt == maxUploadPartAttempts {
			return "", err
		}
		rep.Logf("分片 %d/%d 上传重试 (%d/%d): %v", partIdx+1, totalParts, attempt, maxUploadPartAttempts, err)
		if wErr := uploadRetryBackoff(ctx, w.uploadRetryDelay, attempt); wErr != nil {
			return "", wErr
		}
	}
	return "", fmt.Errorf("exhausted upload part retries")
}

func (w *Worker) doMultipartChunkAttempt(ctx context.Context, body io.Reader, partLen, offset int64, putURL string) (string, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, putURL, body)
	if err != nil {
		return "", false, err
	}
	req.ContentLength = partLen
	resp, err := w.uploadHC.Do(req)
	if err != nil {
		return "", true, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", shouldRetryUploadStatus(resp.StatusCode), fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(raw))
	}
	etag := resp.Header.Get("ETag")
	if etag == "" {
		etag = fmt.Sprintf(`"part-%d"`, offset)
	}
	return etag, false, nil
}
