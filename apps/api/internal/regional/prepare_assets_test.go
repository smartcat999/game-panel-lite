package regional_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/assetfiles"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/assets"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/regional"
)

type contentSource func(context.Context, instances.RevisionAvailable, assets.PublishedVersion) (io.ReadCloser, error)

func (f contentSource) Open(ctx context.Context, e instances.RevisionAvailable, v assets.PublishedVersion) (io.ReadCloser, error) {
	return f(ctx, e, v)
}

func assetSnapshot() regional.RevisionSnapshot {
	e := instances.RevisionAvailable{SchemaVersion: 1, EventID: "event", OperationID: "operation", OrganizationID: "tenant", ServerID: "server", RevisionID: "revision", RegionID: "east", PlacementEpoch: 1, SpecGeneration: 1}
	s := regional.RevisionSnapshot{Event: e, CurrentSpecGeneration: 1, DesiredState: "stopped", IntentVersion: 1, Revision: instances.Revision{ID: e.RevisionID, ServerID: e.ServerID, SpecGeneration: 1, Specification: instances.Specification{ProviderKey: "test", GameVersion: "1", ConfigSchemaVersion: 1, Resources: instances.Resources{CPU: 1, MemoryMB: 128}, Configuration: instances.ProtectedConfiguration{KeyID: "test", Ciphertext: []byte("opaque")}}}}
	for _, id := range []string{"first", "second"} {
		sum := sha256.Sum256([]byte(id))
		s.Assets = append(s.Assets, assets.PublishedVersion{OrganizationID: "tenant", AssetID: id, Version: "v1", SHA256: hex.EncodeToString(sum[:]), SizeBytes: int64(len(id))})
		s.Revision.Specification.Assets = append(s.Revision.Specification.Assets, instances.AssetVersion{AssetID: id, Version: "v1"})
	}
	return s
}

func TestAssetPreparationRetriesRequireSourceAuthorization(t *testing.T) {
	files, err := assetfiles.New(t.TempDir(), 1024)
	if err != nil {
		t.Fatal(err)
	}
	defer files.Close()
	snapshot := assetSnapshot()
	calls := 0
	corrupt := true
	denied := false
	denial := errors.New("current regional access revoked")
	source := contentSource(func(ctx context.Context, e instances.RevisionAvailable, v assets.PublishedVersion) (io.ReadCloser, error) {
		calls++
		if e != snapshot.Event {
			t.Fatal("source lost revision authorization context")
		}
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("source has no deadline")
		}
		if denied {
			return nil, denial
		}
		body := v.AssetID
		if corrupt && body == "second" {
			body = "broken"
		}
		return io.NopCloser(strings.NewReader(body)), nil
	})
	p := regional.AssetPreparer{RegionID: "east", Source: source, Files: files, MaxFiles: 2, MaxFileBytes: 10, MaxTotalBytes: 11, Timeout: time.Second}
	if err := p.Prepare(context.Background(), snapshot); err == nil {
		t.Fatal("partially prepared revision succeeded")
	}
	if f, err := files.Open(context.Background(), snapshot.Assets[0]); err != nil {
		t.Fatal(err)
	} else {
		f.Close()
	}
	if f, err := files.Open(context.Background(), snapshot.Assets[1]); err == nil {
		f.Close()
		t.Fatal("corrupt file published")
	}
	corrupt = false
	if err := p.Prepare(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	if calls != 4 {
		t.Fatal("retry bypassed source authorization")
	}
	denied = true
	if err := p.Prepare(context.Background(), snapshot); !errors.Is(err, denial) {
		t.Fatal("cached bytes bypassed revoked access")
	}
}

func TestAssetPreparationRejectsBeforeIO(t *testing.T) {
	files, err := assetfiles.New(t.TempDir(), 1024)
	if err != nil {
		t.Fatal(err)
	}
	defer files.Close()
	source := contentSource(func(context.Context, instances.RevisionAvailable, assets.PublishedVersion) (io.ReadCloser, error) {
		t.Fatal("invalid preparation reached source")
		return nil, nil
	})
	base := regional.AssetPreparer{RegionID: "east", Source: source, Files: files, MaxFiles: 2, MaxFileBytes: 10, MaxTotalBytes: 11, Timeout: time.Second}
	for _, mutate := range []func(*regional.AssetPreparer){func(p *regional.AssetPreparer) { p.RegionID = "west" }, func(p *regional.AssetPreparer) { p.MaxFiles = 1 }, func(p *regional.AssetPreparer) { p.MaxFileBytes = 5 }, func(p *regional.AssetPreparer) { p.MaxTotalBytes = 10 }} {
		p := base
		mutate(&p)
		if err := p.Prepare(context.Background(), assetSnapshot()); err == nil {
			t.Fatal("invalid limits accepted")
		}
	}
	snapshot := assetSnapshot()
	snapshot.Assets = nil
	if err := base.Prepare(context.Background(), snapshot); err == nil {
		t.Fatal("missing manifest accepted")
	}
	snapshot = assetSnapshot()
	snapshot.Assets[0].SizeBytes = math.MaxInt64
	snapshot.Assets[1].SizeBytes = 1
	base.MaxFileBytes = math.MaxInt64
	base.MaxTotalBytes = math.MaxInt64
	if err := base.Prepare(context.Background(), snapshot); err == nil {
		t.Fatal("aggregate size overflow accepted")
	}
}

func TestAssetPreparationDeadlineClosesBlockedStream(t *testing.T) {
	files, err := assetfiles.New(t.TempDir(), 1024)
	if err != nil {
		t.Fatal(err)
	}
	defer files.Close()
	reader, writer := io.Pipe()
	defer writer.Close()
	p := regional.AssetPreparer{RegionID: "east", Files: files, MaxFiles: 2, MaxFileBytes: 10, MaxTotalBytes: 11, Timeout: 50 * time.Millisecond, Source: contentSource(func(context.Context, instances.RevisionAvailable, assets.PublishedVersion) (io.ReadCloser, error) {
		return reader, nil
	})}
	if err := p.Prepare(context.Background(), assetSnapshot()); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("preparation timeout: %v", err)
	}
}
