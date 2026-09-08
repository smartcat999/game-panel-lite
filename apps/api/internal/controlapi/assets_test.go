package controlapi

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/assets"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/controlclient"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/regional"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/serviceauth"
)

type assetReaderFunc func(context.Context, string, instances.RevisionAvailable, instances.AssetVersion) (assets.PublishedVersion, error)

func (f assetReaderFunc) ResolveRegionalAsset(ctx context.Context, r string, e instances.RevisionAvailable, a instances.AssetVersion) (assets.PublishedVersion, error) {
	return f(ctx, r, e, a)
}

func testAssetAPI(t *testing.T, auth *serviceauth.Regions, serverTLS *tls.Config, pool *x509.CertPool, east, west tls.Certificate) {
	t.Helper()
	event := instances.RevisionAvailable{SchemaVersion: 1, EventID: "event", OperationID: "operation", OrganizationID: "org", ServerID: "server", RevisionID: "revision", RegionID: "east", PlacementEpoch: 1, SpecGeneration: 1}
	asset := assets.PublishedVersion{AssetID: "world", OrganizationID: "org", Version: "v1", SHA256: strings.Repeat("a", 64), SizeBytes: 1}
	h, err := NewAssetHandler(assetReaderFunc(func(_ context.Context, r string, e instances.RevisionAvailable, ref instances.AssetVersion) (assets.PublishedVersion, error) {
		if r != "east" {
			t.Fatal("reader got wrong service identity")
		}
		if ref.AssetID == "failure" {
			return assets.PublishedVersion{}, errors.New("private detail")
		}
		if e != event || ref.AssetID != asset.AssetID || ref.Version != asset.Version {
			return assets.PublishedVersion{}, regional.ErrRevisionUnavailable
		}
		return asset, nil
	}), auth, 1024)
	if err != nil {
		t.Fatal(err)
	}
	plain := httptest.NewRequest(http.MethodPost, "/internal/region/assets/resolve?assetId=world&version=v1", strings.NewReader("{}"))
	plain.Header.Set("X-Region-ID", "east")
	record := httptest.NewRecorder()
	h.ServeHTTP(record, plain)
	if record.Code != 401 {
		t.Fatal("asset header spoof accepted")
	}
	server := httptest.NewUnstartedServer(h)
	server.TLS = serverTLS.Clone()
	server.StartTLS()
	defer server.Close()
	client, err := controlclient.New(controlclient.Options{Endpoint: server.URL, RegionID: "east", Certificate: east, ServerCAs: pool, Timeout: time.Second, MaxResponseBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if got, err := client.ResolveAsset(context.Background(), event, instances.AssetVersion{AssetID: "world", Version: "v1"}); err != nil || got != asset {
		t.Fatalf("asset mTLS roundtrip: %v", err)
	}
	body, _ := json.Marshal(event)
	for _, tc := range []struct {
		query, body string
		cert        tls.Certificate
		status      int
	}{
		{"assetId=world&version=v1", string(body), west, 403},
		{"assetId=world&assetId=other&version=v1", string(body), east, 400},
		{"assetId=world&version=v1&extra=x", string(body), east, 400},
		{"assetId=world&version=v1", string(body) + strings.Repeat(" ", 1024), east, 413},
		{"assetId=world&version=v1", strings.TrimSuffix(string(body), "}") + `,"REGIONID":"east"}`, east, 400},
		{"assetId=unknown&version=v1", string(body), east, 404},
		{"assetId=failure&version=v1", string(body), east, 503},
	} {
		transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: pool, Certificates: []tls.Certificate{tc.cert}}}
		httpClient := &http.Client{Transport: transport, Timeout: time.Second}
		r, _ := http.NewRequest(http.MethodPost, server.URL+"/internal/region/assets/resolve?"+tc.query, strings.NewReader(tc.body))
		r.Header.Set("Content-Type", "application/json")
		response, err := httpClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		failure, _ := io.ReadAll(response.Body)
		response.Body.Close()
		transport.CloseIdleConnections()
		if response.StatusCode != tc.status || response.Header.Get("Cache-Control") != "no-store" || strings.Contains(string(failure), "private detail") {
			t.Fatalf("asset API status: %d want %d", response.StatusCode, tc.status)
		}
	}
}
