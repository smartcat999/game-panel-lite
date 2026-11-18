package blob_test

import (
	"context"
	"errors"
	"os"
	"testing"

	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/repo/registry/blob"
	"github.com/smartcat999/game-panel-cloud/tests/testpg"
)

func TestMain(m *testing.M) {
	code := m.Run()
	testpg.Stop()
	os.Exit(code)
}

func TestBlobAcquireReleaseRefAndUnderflow(t *testing.T) {
	fix := testpg.New(t, nil)
	store := blob.NewDB(fix.Pool)
	ctx := context.Background()
	const digest = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

	created, err := store.CreateOrGet(ctx, &registryv1.Blob{Digest: digest, SizeBytes: 128})
	if err != nil {
		t.Fatalf("CreateOrGet: %v", err)
	}
	if created.RefCount != 0 {
		t.Fatalf("initial RefCount = %d, want 0", created.RefCount)
	}
	if _, err := store.DecrementRef(ctx, digest); !errors.Is(err, db.ErrConflict) {
		t.Fatalf("DecrementRef at 0 err = %v, want db.ErrConflict", err)
	}

	b, err := store.AcquireRef(ctx, digest, "uid-a")
	if err != nil || b.RefCount != 1 {
		t.Fatalf("first AcquireRef uid-a: blob=%+v err=%v", b, err)
	}
	if b, err = store.AcquireRef(ctx, digest, "uid-a"); err != nil || b.RefCount != 1 {
		t.Fatalf("duplicate AcquireRef uid-a: blob=%+v err=%v", b, err)
	}
	if b, err = store.AcquireRef(ctx, digest, "uid-b"); err != nil || b.RefCount != 2 {
		t.Fatalf("AcquireRef uid-b: blob=%+v err=%v", b, err)
	}

	if b, err = store.ReleaseRef(ctx, digest, "uid-a"); err != nil || b.RefCount != 1 {
		t.Fatalf("first ReleaseRef uid-a: blob=%+v err=%v", b, err)
	}
	if b, err = store.ReleaseRef(ctx, digest, "uid-a"); err != nil || b.RefCount != 1 {
		t.Fatalf("duplicate ReleaseRef uid-a: blob=%+v err=%v", b, err)
	}
	if b, err = store.ReleaseRef(ctx, digest, "uid-b"); err != nil || b.RefCount != 0 {
		t.Fatalf("ReleaseRef uid-b: blob=%+v err=%v", b, err)
	}
	if b, err = store.ReleaseRef(ctx, digest, "uid-unknown"); err != nil || b.RefCount != 0 {
		t.Fatalf("ReleaseRef unknown holder at 0: blob=%+v err=%v", b, err)
	}
	if _, err := store.DecrementRef(ctx, digest); !errors.Is(err, db.ErrConflict) {
		t.Fatalf("DecrementRef after ReleaseRef err = %v, want db.ErrConflict", err)
	}
}
