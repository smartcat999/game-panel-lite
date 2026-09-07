package http

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/modlibrary"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
	"github.com/smartcat999/game-panel-lite/internal/workload"
)

func TestAgentArtifactDownloadIsBoundToAssignment(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	ctx := context.Background()
	for _, id := range []string{"download-a", "download-b"} {
		node := domain.ComputeNode{ID: id, Token: id}
		if err := db.CreateComputeNode(ctx, &node); err != nil {
			t.Fatal(err)
		}
	}
	org := domain.Organization{ID: "download-space", Slug: "download-space"}
	if err := db.CreateOrganization(ctx, &org, "owner"); err != nil {
		t.Fatal(err)
	}
	target := domain.GameServer{ID: "download-server", OrganizationID: org.ID, NodeID: "download-a", ProviderKey: domain.ProviderTerrariaTModLoader, Spec: domain.ServerSpec{Generation: 1, DesiredState: domain.DesiredRunning}}
	payload := []byte{0, 255, 128, 42}
	sum := sha256.Sum256(payload)
	item := domain.ModFile{ID: "download-source", OrganizationID: org.ID, InstanceID: "unassigned", ProviderKey: target.ProviderKey, Source: "upload", FileName: "source.tmod", ContentHash: hex.EncodeToString(sum[:]), SizeBytes: int64(len(payload))}
	files := newTestModService(t, cfg.DataDir)
	if _, err := files.PutLibrary(ctx, item, bytes.NewReader(payload), 100); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateOwnedLibraryMod(ctx, "owner", &item); err != nil {
		t.Fatal(err)
	}
	assignment := domain.WorkloadAssignment{ID: "download-assignment", UID: "download-uid", NodeID: target.NodeID, ServerID: target.ID, Generation: 1, DesiredState: domain.DesiredRunning, Spec: workload.Spec{Options: workload.Options{Artifacts: []workload.Artifact{{ID: item.ID, Path: "Mods/source.tmod", SizeBytes: item.SizeBytes, SHA256: item.ContentHash}}}}}
	if err := db.Transaction(ctx, func(tx *store.Store) error {
		if err := tx.CreateGameServer(ctx, &target); err != nil {
			return err
		}
		return tx.PublishWorkloadAssignment(ctx, target, &assignment)
	}); err != nil {
		t.Fatal(err)
	}
	request := func(token, id, generation string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, "/api/agent/assignments/download-uid/artifacts/"+id+"?generation="+generation, nil)
		r.Header.Set("X-Node-Token", token)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	for _, tc := range []struct {
		token, id, gen string
		status         int
	}{{"", item.ID, "1", 401}, {"download-b", item.ID, "1", 404}, {"download-a", item.ID, "2", 404}, {"download-a", "other-source", "1", 404}, {"download-a", item.ID, "0", 400}} {
		got := request(tc.token, tc.id, tc.gen)
		if got.Code != tc.status {
			t.Fatalf("download authorization: %+v: %d %s", tc, got.Code, got.Body.String())
		}
		if bytes.Equal(got.Body.Bytes(), payload) {
			t.Fatal("unauthorized body leaked")
		}
	}
	got := request("download-a", item.ID, "1")
	if got.Code != 200 || !bytes.Equal(got.Body.Bytes(), payload) || got.Header().Get("Content-Length") != strconv.Itoa(len(payload)) || got.Header().Get("ETag") != strconv.Quote(item.ContentHash) || got.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("download: %d %v %v", got.Code, got.Header(), got.Body.Bytes())
	}

	node, err := db.GetComputeNodeByToken(ctx, "download-a")
	if err != nil {
		t.Fatal(err)
	}
	var opened *os.File
	revoking := artifactDownloadFilesFunc(func(item domain.ModFile) (*os.File, error) {
		node.Token = "rotated-token"
		if err := db.UpdateComputeNode(ctx, &node); err != nil {
			t.Fatal(err)
		}
		file, err := files.OpenLibrary(item)
		opened = file
		return file, err
	})
	handler := &Handler{store: db, modDelivery: modlibrary.NewDelivery(db, revoking)}
	isolated := chi.NewRouter()
	isolated.Get("/api/agent/assignments/{uid}/artifacts/{artifactId}", handler.downloadAgentArtifact)
	revokedRequest := httptest.NewRequest(http.MethodGet, "/api/agent/assignments/download-uid/artifacts/"+item.ID+"?generation=1", nil)
	revokedRequest.Header.Set("X-Node-Token", "download-a")
	denied := httptest.NewRecorder()
	isolated.ServeHTTP(denied, revokedRequest)
	if denied.Code != 401 || bytes.Equal(denied.Body.Bytes(), payload) {
		t.Fatalf("rotated token received bytes: %d", denied.Code)
	}
	if opened == nil {
		t.Fatal("did not exercise revocation after open")
	}
	if _, err := opened.Stat(); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("revoked download leaked file: %v", err)
	}
	node.Token = "download-a"
	if err := db.UpdateComputeNode(ctx, &node); err != nil {
		t.Fatal(err)
	}
	if err := files.RemoveLibrary(item); err != nil {
		t.Fatal(err)
	}
	if got := request("download-a", item.ID, "1"); got.Code != 503 {
		t.Fatalf("missing bytes: %d %s", got.Code, got.Body.String())
	}
	target.Spec.Generation++
	if err := db.SaveGameServer(ctx, &target); err != nil {
		t.Fatal(err)
	}
	if got := request("download-a", item.ID, "1"); got.Code != 404 {
		t.Fatalf("old desired generation still authorized: %d", got.Code)
	}
}

type artifactDownloadFilesFunc func(domain.ModFile) (*os.File, error)

func (f artifactDownloadFilesFunc) OpenLibrary(item domain.ModFile) (*os.File, error) { return f(item) }
