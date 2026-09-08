package backup_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/assetfiles"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/assets"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/backup"
)

type uploadTasks struct {
	claim                *backup.UploadClaim
	receipt              backup.StoredArchive
	retries, completions int
	completeErr          error
}

func (t *uploadTasks) ClaimArchiveUpload(context.Context, time.Duration) (*backup.UploadClaim, error) {
	return t.claim, nil
}
func (t *uploadTasks) RetryArchiveUpload(context.Context, backup.UploadClaim, time.Duration) error {
	t.retries++
	return nil
}
func (t *uploadTasks) CompleteArchiveUpload(_ context.Context, c backup.UploadClaim, r backup.StoredArchive) error {
	t.completions++
	if t.completeErr != nil {
		return t.completeErr
	}
	t.receipt = r
	t.claim = nil
	return nil
}

type uploadArchive struct {
	receipt           backup.StoredArchive
	body              []byte
	uploads, resolves int
	uploadErr         error
	wrong             bool
	block             bool
}

func (s *uploadArchive) ResolveUpload(ctx context.Context, _ string, _ assets.PublishedVersion) (backup.StoredArchive, error) {
	s.resolves++
	if s.block {
		<-ctx.Done()
		return backup.StoredArchive{}, ctx.Err()
	}
	if s.body == nil {
		return backup.StoredArchive{}, errors.New("not found")
	}
	r := s.receipt
	if s.wrong {
		r.ObjectKey = "other"
	}
	return r, nil
}
func (s *uploadArchive) Upload(_ context.Context, key string, v assets.PublishedVersion, r io.ReaderAt) (backup.StoredArchive, error) {
	s.uploads++
	if s.uploadErr != nil {
		return backup.StoredArchive{}, s.uploadErr
	}
	s.body = make([]byte, v.SizeBytes)
	if _, err := r.ReadAt(s.body, 0); err != nil {
		return backup.StoredArchive{}, err
	}
	return s.receipt, nil
}
func (s *uploadArchive) Open(context.Context, backup.StoredArchive) (io.ReadCloser, error) {
	return nil, errors.New("unused")
}

func TestUploadWorkerRecovery(t *testing.T) {
	ctx := context.Background()
	data := []byte("immutable prepared archive")
	v := assets.PublishedVersion{AssetID: "asset", OrganizationID: "tenant", Version: "v1", SHA256: fmt.Sprintf("%x", sha256.Sum256(data)), SizeBytes: int64(len(data))}
	files, err := assetfiles.New(t.TempDir(), 1024)
	if err != nil {
		t.Fatal(err)
	}
	defer files.Close()
	if err := files.Put(ctx, v, io.NopCloser(bytes.NewReader(data))); err != nil {
		t.Fatal(err)
	}
	plan := backup.UploadPlan{OperationID: "operation", RequestEventID: "request", ID: "upload", RegionID: "east", ServerID: "server", DeploymentID: "deployment", NodeID: "node", SnapshotID: "snapshot", PlacementEpoch: 1, StorageID: "storage", ObjectKey: "object", Asset: v}
	receipt := backup.StoredArchive{StorageID: plan.StorageID, ObjectKey: plan.ObjectKey, ObjectVersion: "version", Asset: v}
	tasks := &uploadTasks{claim: &backup.UploadClaim{Token: "token", Plan: plan}, completeErr: errors.New("database completion lost")}
	archives := &uploadArchive{receipt: receipt}
	worker := backup.UploadWorker{Tasks: tasks, Files: files, Archives: archives, StorageID: "storage", Lease: time.Second, Timeout: 100 * time.Millisecond, RetryDelay: time.Second}
	if ok, err := worker.RunOnce(ctx); ok || !errors.Is(err, tasks.completeErr) {
		t.Fatal("completion failure ignored", err)
	}
	if archives.uploads != 1 || !bytes.Equal(archives.body, data) || tasks.retries != 0 {
		t.Fatal("wrong initial upload")
	}
	// Recovery must work even after the local file adapter becomes unavailable.
	if err := files.Close(); err != nil {
		t.Fatal(err)
	}
	tasks.completeErr = nil
	if ok, err := worker.RunOnce(ctx); !ok || err != nil {
		t.Fatal("lost receipt not recovered", err)
	}
	if archives.uploads != 1 || archives.resolves != 2 || tasks.receipt != receipt {
		t.Fatal("recovery reuploaded or changed receipt")
	}
	if ok, err := worker.RunOnce(ctx); ok || err != nil {
		t.Fatal("empty queue not idle")
	}
	for _, tc := range []struct {
		name  string
		setup func(*uploadArchive, *backup.UploadWorker)
	}{
		{"wrong receipt", func(a *uploadArchive, w *backup.UploadWorker) { a.wrong = true }},
		{"wrong storage", func(a *uploadArchive, w *backup.UploadWorker) { w.StorageID = "other" }},
		{"timeout", func(a *uploadArchive, w *backup.UploadWorker) { a.block = true; w.Timeout = 10 * time.Millisecond }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			task := &uploadTasks{claim: &backup.UploadClaim{Token: "token", Plan: plan}}
			archive := &uploadArchive{receipt: receipt, body: data}
			w := worker
			w.Tasks = task
			w.Archives = archive
			tc.setup(archive, &w)
			if ok, err := w.RunOnce(ctx); ok || err == nil {
				t.Fatal("invalid upload succeeded")
			}
			if task.retries != 1 || task.completions != 0 || archive.uploads != 0 {
				t.Fatal("invalid result published")
			}
		})
	}
	fresh, err := assetfiles.New(t.TempDir(), 1024)
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Close()
	if err := fresh.Put(ctx, v, io.NopCloser(bytes.NewReader(data))); err != nil {
		t.Fatal(err)
	}
	failed := &uploadTasks{claim: &backup.UploadClaim{Token: "token", Plan: plan}}
	archive := &uploadArchive{receipt: receipt, uploadErr: errors.New("secret credentials")}
	worker.Tasks = failed
	worker.Files = fresh
	worker.Archives = archive
	if ok, err := worker.RunOnce(ctx); ok || err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatal("unsafe failure", err)
	}
	if failed.retries != 1 || failed.completions != 0 {
		t.Fatal("failed upload completed")
	}
}
