package regionopsapi

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/regional"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/serviceauth"
)

type nodeReaderFixture struct{ calls int }

func (f *nodeReaderFixture) RegionID() string { return "east" }
func (f *nodeReaderFixture) ListRegionalNodeOperations(_ context.Context, after string, limit int, _ time.Duration) (regional.NodeOperationsPage, error) {
	f.calls++
	if after != "cursor" || limit != 10 {
		return regional.NodeOperationsPage{}, regional.ErrInvalidNodeOperations
	}
	return regional.NodeOperationsPage{RegionID: "east", ObservedAtMS: 1, Nodes: []regional.NodeOperations{}}, nil
}
func (f *nodeReaderFixture) ListRegionalDeploymentOperations(_ context.Context, after string, limit int) (regional.DeploymentOperationsPage, error) {
	f.calls++
	if after != "cursor" || limit != 10 {
		return regional.DeploymentOperationsPage{}, regional.ErrInvalidDeploymentOperations
	}
	return regional.DeploymentOperationsPage{RegionID: "east", ObservedAtMS: 1, Deployments: []regional.DeploymentOperations{}}, nil
}

func TestNodeOperationsRequiresGlobalControlIdentity(t *testing.T) {
	identity := "spiffe://gamepanel/global-control"
	auth, err := serviceauth.NewGlobalControls([]string{identity})
	if err != nil {
		t.Fatal(err)
	}
	reader := &nodeReaderFixture{}
	handler, err := NewHandler(reader, auth, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	plain := httptest.NewRecorder()
	handler.ServeHTTP(plain, httptest.NewRequest(http.MethodGet, "/internal/operations/nodes?after=cursor&limit=10", nil))
	if plain.Code != http.StatusUnauthorized || reader.calls != 0 {
		t.Fatal("unauthenticated request reached regional store")
	}
	uri, _ := url.Parse(identity)
	leaf := &x509.Certificate{URIs: []*url.URL{uri}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Minute)}
	request := httptest.NewRequest(http.MethodGet, "/internal/operations/nodes?after=cursor&limit=10", nil)
	request.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{leaf}, VerifiedChains: [][]*x509.Certificate{{leaf}}}
	record := httptest.NewRecorder()
	handler.ServeHTTP(record, request)
	if record.Code != http.StatusOK || reader.calls != 1 || record.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("authenticated operations response: %d %s", record.Code, record.Body.String())
	}
	request = httptest.NewRequest(http.MethodGet, "/internal/operations/nodes?limit=10&limit=11", nil)
	request.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{leaf}, VerifiedChains: [][]*x509.Certificate{{leaf}}}
	record = httptest.NewRecorder()
	handler.ServeHTTP(record, request)
	if record.Code != http.StatusBadRequest || reader.calls != 1 {
		t.Fatal("ambiguous query reached regional store")
	}
	request = httptest.NewRequest(http.MethodGet, "/internal/operations/deployments?after=cursor&limit=10", nil)
	request.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{leaf}, VerifiedChains: [][]*x509.Certificate{{leaf}}}
	record = httptest.NewRecorder()
	handler.ServeHTTP(record, request)
	if record.Code != http.StatusOK || reader.calls != 2 {
		t.Fatalf("deployment operations response: %d %s", record.Code, record.Body.String())
	}
}
