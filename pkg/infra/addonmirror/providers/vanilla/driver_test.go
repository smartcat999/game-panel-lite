package vanilla

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/smartcat999/game-panel-cloud/pkg/infra/addonmirror/driver"
)

type stubRuntime struct {
	driver.Runtime
	client *http.Client
}

func (s *stubRuntime) DoGetJSON(ctx context.Context, endpoint string, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return driver.ErrNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("upstream status %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(dst)
}

func newMirrorTestServer(t *testing.T, serverURL, sha1 string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/mc/game/version_manifest_v2.json", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"latest":{"release":"26.3"},"versions":[{"id":"26.3","type":"release","url":"https://piston-meta.mojang.com/v1/packages/abc/26.3.json"}]}`))
	})
	mux.HandleFunc("/v1/packages/abc/26.3.json", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintf(w, `{"releaseTime":"2026-11-18T12:00:00Z","downloads":{"server":{"url":%q,"sha1":%q}}}`, serverURL, sha1)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestResolveFallsBackToMirrorWhenPrimaryFails(t *testing.T) {
	const officialJar = "https://piston-data.mojang.com/v1/objects/abc/server.jar"
	const wantSHA1 = "33680f5f2ac32864d6d7cf5e56a705fdb3e05f4c"

	primarySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "eof", http.StatusBadGateway)
	}))
	defer primarySrv.Close()
	mirrorSrv := newMirrorTestServer(t, officialJar, wantSHA1)

	const fastMirrorBase = "https://fastmirror.example"
	rt := &stubRuntime{client: http.DefaultClient}
	rel, err := resolveWithBases(context.Background(), rt, "", "26.3", primarySrv.URL+"/mc/game/version_manifest_v2.json", mirrorSrv.URL, fastMirrorBase)
	if err != nil {
		t.Fatalf("expected mirror fallback to succeed, got error: %v", err)
	}
	wantFastJar := fastMirrorBase + "/download/Vanilla/26.3/release"
	if rel.DownloadURL != wantFastJar {
		t.Fatalf("DownloadURL = %q, want %q", rel.DownloadURL, wantFastJar)
	}
	wantBMCLJar := mirrorSrv.URL + "/version/26.3/server"
	if len(rel.FallbackDownloadURLs) != 2 || rel.FallbackDownloadURLs[0] != wantBMCLJar || rel.FallbackDownloadURLs[1] != officialJar {
		t.Fatalf("FallbackDownloadURLs = %v, want [%q, %q]", rel.FallbackDownloadURLs, wantBMCLJar, officialJar)
	}
	if rel.Build != wantSHA1 {
		t.Fatalf("Build SHA1 = %q, want %q", rel.Build, wantSHA1)
	}
}

func TestResolvePrimaryPopulatesMirrorFallback(t *testing.T) {
	const officialJar = "https://piston-data.mojang.com/v1/objects/abc/server.jar"
	const wantSHA1 = "33680f5f2ac32864d6d7cf5e56a705fdb3e05f4c"

	var base string
	mux := http.NewServeMux()
	mux.HandleFunc("/mc/game/version_manifest_v2.json", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintf(w, `{"latest":{"release":"26.3"},"versions":[{"id":"26.3","type":"release","url":"%s/v1/packages/abc/26.3.json"}]}`, base)
	})
	mux.HandleFunc("/v1/packages/abc/26.3.json", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintf(w, `{"releaseTime":"2026-11-18T12:00:00Z","downloads":{"server":{"url":%q,"sha1":%q}}}`, officialJar, wantSHA1)
	})
	primarySrv := httptest.NewServer(mux)
	base = primarySrv.URL
	defer primarySrv.Close()

	rt := &stubRuntime{client: http.DefaultClient}
	rel, err := resolveWithBases(context.Background(), rt, "", "26.3", primarySrv.URL+"/mc/game/version_manifest_v2.json", "https://bmclapi2.bangbang93.com", "https://download.fastmirror.net")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rel.DownloadURL != officialJar {
		t.Fatalf("DownloadURL = %q, want %q", rel.DownloadURL, officialJar)
	}
	wantFast := "https://download.fastmirror.net/download/Vanilla/26.3/release"
	wantBMCL := "https://bmclapi2.bangbang93.com/version/26.3/server"
	if len(rel.FallbackDownloadURLs) != 2 || rel.FallbackDownloadURLs[0] != wantFast || rel.FallbackDownloadURLs[1] != wantBMCL {
		t.Fatalf("FallbackDownloadURLs = %v, want [%q, %q]", rel.FallbackDownloadURLs, wantFast, wantBMCL)
	}
}

func TestExplicitEndpointDoesNotFallbackSilently(t *testing.T) {
	var mirrorHits atomic.Int32
	mirrorSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mirrorHits.Add(1)
		http.Error(w, "unexpected", http.StatusInternalServerError)
	}))
	defer mirrorSrv.Close()

	badExplicit := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "down", http.StatusBadGateway)
	}))
	defer badExplicit.Close()

	rt := &stubRuntime{client: http.DefaultClient}
	_, err := resolveWithBases(context.Background(), rt, badExplicit.URL+"/manifest.json", "26.3", badExplicit.URL+"/unused", mirrorSrv.URL, "https://unused-fast.example")
	if err == nil || errors.Is(err, driver.ErrNotFound) {
		t.Fatalf("expected explicit endpoint failure, got %v", err)
	}
	if mirrorHits.Load() != 0 {
		t.Fatalf("explicit endpoint unexpectedly hit mirror %d times", mirrorHits.Load())
	}
}

func TestExplicitCustomMirrorRewritesVersionMetaOrigin(t *testing.T) {
	const officialJar = "https://piston-data.mojang.com/v1/objects/abc/server.jar"
	customMirror := newMirrorTestServer(t, officialJar, "0123456789012345678901234567890123456789")

	rt := &stubRuntime{client: http.DefaultClient}
	rel, err := resolveWithBases(context.Background(), rt, customMirror.URL+"/mc/game/version_manifest_v2.json", "26.3", "https://unused.example", "https://unused-mirror.example", "https://unused-fast.example")
	if err != nil {
		t.Fatalf("custom mirror failed: %v", err)
	}
	if rel.DownloadURL != officialJar || len(rel.FallbackDownloadURLs) != 0 {
		t.Fatalf("unexpected custom endpoint result: url=%q fallbacks=%v", rel.DownloadURL, rel.FallbackDownloadURLs)
	}
}
