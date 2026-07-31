package http

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	stdhttp "net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	workshopsvc "github.com/smartcat999/game-panel-lite/apps/api/internal/workshop"
)

type staticWorkshopResolver struct {
	collection workshopsvc.Collection
	err        error
}

func (r staticWorkshopResolver) ResolveCollection(context.Context, domain.ProviderKey, string) (workshopsvc.Collection, error) {
	return r.collection, r.err
}

func TestWorkshopCollectionPreviewMarksExistingItemsAndCachesMetadata(t *testing.T) {
	_, db, _ := newTestRouter(t)
	existing := domain.ModFile{
		ID:          "existing",
		InstanceID:  "unassigned",
		ProviderKey: domain.ProviderTerrariaTModLoader,
		Source:      "workshop",
		WorkshopID:  "100",
		FileName:    "workshop-100",
		Enabled:     true,
		CreatedAt:   time.Now(),
	}
	if err := db.CreateMod(context.Background(), &existing); err != nil {
		t.Fatal(err)
	}
	handler := &Handler{
		store: db,
		workshopResolver: staticWorkshopResolver{collection: workshopsvc.Collection{
			ID: "900",
			Items: []workshopsvc.Item{
				{WorkshopID: "100", Title: "Existing"},
				{WorkshopID: "200", Title: "New Mod", FileSize: 2048, Tags: []string{"New Content"}},
			},
		}},
		workshopPreviews: map[string]cachedWorkshopPreview{},
	}
	body := bytes.NewBufferString(`{"providerKey":"terraria-tmodloader","value":"900"}`)
	request := httptest.NewRequest(stdhttp.MethodPost, "/api/mods/workshop/preview", body)
	recorder := httptest.NewRecorder()
	handler.previewWorkshopCollection(recorder, request)
	if recorder.Code != stdhttp.StatusOK {
		t.Fatalf("expected preview 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var response workshopPreviewResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Summary.Total != 2 || response.Summary.New != 1 || response.Summary.InLibrary != 1 {
		t.Fatalf("unexpected summary: %+v", response.Summary)
	}
	if response.PreviewID == "" || len(response.Items) != 2 {
		t.Fatalf("unexpected response: %+v", response)
	}
	selected, err := handler.workshopPreviewItems(response.PreviewID, domain.ProviderTerrariaTModLoader, "", []string{"200"})
	if err != nil {
		t.Fatal(err)
	}
	if selected["200"].Title != "New Mod" {
		t.Fatalf("expected cached server metadata, got %+v", selected)
	}
	if _, err := handler.workshopPreviewItems(response.PreviewID, domain.ProviderTerrariaTModLoader, "", []string{"999"}); err == nil {
		t.Fatal("expected non-preview item to be rejected")
	}
}

func TestCreateModPackFromWorkshopCollectionImportsAndReusesLibraryMods(t *testing.T) {
	_, db, _ := newTestRouter(t)
	existing := domain.ModFile{
		ID:          "existing",
		InstanceID:  "unassigned",
		GameKey:     domain.GameTerraria,
		ProviderKey: domain.ProviderTerrariaTModLoader,
		Source:      "workshop",
		WorkshopID:  "100",
		FileName:    "workshop-100",
		Enabled:     true,
		CreatedAt:   time.Now(),
	}
	if err := db.CreateMod(context.Background(), &existing); err != nil {
		t.Fatal(err)
	}
	previewID := "preview-1"
	handler := &Handler{
		store:  db,
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		workshopPreviews: map[string]cachedWorkshopPreview{
			previewID: {
				ProviderKey: domain.ProviderTerrariaTModLoader,
				Collection: workshopsvc.Collection{
					ID: "900",
					Items: []workshopsvc.Item{
						{WorkshopID: "100", Title: "Existing Mod", FileSize: 1024},
						{WorkshopID: "200", Title: "New Mod", FileSize: 2048},
					},
				},
				ExpiresAt: time.Now().Add(time.Minute),
			},
		},
	}
	body := bytes.NewBufferString(`{"name":"Collection Pack","description":"Imported from Steam","providerKey":"terraria-tmodloader","previewId":"preview-1","workshopIds":["100","200"]}`)
	recorder := httptest.NewRecorder()
	handler.createModPackFromWorkshopCollection(recorder, httptest.NewRequest(stdhttp.MethodPost, "/api/mod-packs/workshop", body))
	if recorder.Code != stdhttp.StatusCreated {
		t.Fatalf("expected collection pack create 201, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var created modPackResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Name != "Collection Pack" || len(created.Mods) != 2 || len(created.ModIDs) != 2 {
		t.Fatalf("unexpected imported mod pack: %+v", created)
	}
	if created.ModIDs[0] != existing.ID {
		t.Fatalf("expected existing library mod to be reused, got %+v", created.ModIDs)
	}
	if created.Mods[0].Title != "Existing Mod" || created.Mods[1].Title != "New Mod" {
		t.Fatalf("expected collection metadata to be preserved, got %+v", created.Mods)
	}
	library, err := db.ListMods(context.Background(), "unassigned")
	if err != nil {
		t.Fatal(err)
	}
	if len(library) != 2 {
		t.Fatalf("expected exactly two library mods after import, got %+v", library)
	}
}
