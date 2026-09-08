package controlapi

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"errors"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/controlclient"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/regional"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/serviceauth"
)

type readerFunc func(context.Context, string, instances.RevisionAvailable) (regional.RevisionSnapshot, error)

func (f readerFunc) GetRegionalRevision(ctx context.Context, region string, event instances.RevisionAvailable) (regional.RevisionSnapshot, error) {
	return f(ctx, region, event)
}

func TestRevisionAPIWithMutualTLS(t *testing.T) {
	now := time.Now()
	caPublic, caKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	caTemplate := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test CA"}, IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour)}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, caPublic, caKey)
	if err != nil {
		t.Fatal(err)
	}
	ca, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(ca)
	serial := int64(2)
	issue := func(identity string, client, expired bool) tls.Certificate {
		t.Helper()
		public, key, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		template := &x509.Certificate{SerialNumber: big.NewInt(serial), NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature}
		serial++
		if expired {
			template.NotAfter = now.Add(-time.Minute)
		}
		if client {
			template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
			uri, _ := url.Parse(identity)
			template.URIs = []*url.URL{uri}
		} else {
			template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
			template.IPAddresses = []net.IP{net.ParseIP("127.0.0.1")}
		}
		der, err := x509.CreateCertificate(rand.Reader, template, ca, public, caKey)
		if err != nil {
			t.Fatal(err)
		}
		return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
	}
	identities := map[string]string{"spiffe://test/region/east": "east", "spiffe://test/region/west": "west"}
	auth, err := serviceauth.NewRegions(identities)
	if err != nil {
		t.Fatal(err)
	}
	identities["spiffe://test/region/east"] = "west"
	event := instances.RevisionAvailable{SchemaVersion: 1, EventID: "event", OperationID: "operation", OrganizationID: "owner", ServerID: "server", RevisionID: "revision", RegionID: "east", PlacementEpoch: 1, SpecGeneration: 1}
	handler, err := NewRevisionHandler(readerFunc(func(_ context.Context, region string, got instances.RevisionAvailable) (regional.RevisionSnapshot, error) {
		if got.EventID == "unavailable" {
			return regional.RevisionSnapshot{}, errors.New("private database detail")
		}
		if region != "east" || got != event {
			return regional.RevisionSnapshot{}, regional.ErrRevisionUnavailable
		}
		return regional.RevisionSnapshot{Event: got, CurrentSpecGeneration: 1, IntentVersion: 1, DesiredState: "running", Revision: instances.Revision{ID: got.RevisionID, ServerID: got.ServerID, SpecGeneration: got.SpecGeneration, Specification: instances.Specification{ProviderKey: "test", GameVersion: "1", ConfigSchemaVersion: 1, Configuration: instances.ProtectedConfiguration{KeyID: "test", Ciphertext: []byte("opaque")}, Resources: instances.Resources{CPU: 1, MemoryMB: 256}}}}, nil
	}), auth, 1024)
	if err != nil {
		t.Fatal(err)
	}
	plain := httptest.NewRequest(http.MethodPost, "/internal/region/revisions/resolve", strings.NewReader("{}"))
	plain.Header.Set("X-Region-ID", "east")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, plain)
	if w.Code != http.StatusUnauthorized {
		t.Fatal("header spoof bypassed TLS")
	}
	server := httptest.NewUnstartedServer(handler)
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	server.TLS, err = serviceauth.ServerTLS(issue("", false, false), pool)
	if err != nil {
		t.Fatal(err)
	}
	server.StartTLS()
	defer server.Close()
	remote, err := controlclient.New(controlclient.Options{Endpoint: server.URL, RegionID: "east", Certificate: issue("spiffe://test/region/east", true, false), ServerCAs: pool, Timeout: time.Second, MaxResponseBytes: 4096})
	if err != nil {
		t.Fatal(err)
	}
	defer remote.Close()
	testAssetAPI(t, auth, server.TLS, pool, issue("spiffe://test/region/east", true, false), issue("spiffe://test/region/west", true, false))
	testBackupAPI(t, auth, server.TLS, pool, issue("spiffe://test/region/east", true, false), issue("spiffe://test/region/west", true, false))
	if snapshot, err := remote.GetRevision(context.Background(), event); err != nil || snapshot.Event != event {
		t.Fatalf("regional client round trip: %v", err)
	}
	body, _ := json.Marshal(event)
	for _, test := range []struct {
		name, identity   string
		expired, missing bool
		status           int
		body             string
	}{
		{"east", "spiffe://test/region/east", false, false, http.StatusOK, ""},
		{"west", "spiffe://test/region/west", false, false, http.StatusForbidden, ""},
		{"unknown", "spiffe://test/region/unknown", false, false, http.StatusUnauthorized, ""},
		{"expired", "spiffe://test/region/east", true, false, 0, ""},
		{"missing", "", false, true, 0, ""},
		{"duplicate", "spiffe://test/region/east", false, false, http.StatusBadRequest, strings.TrimSuffix(string(body), "}") + `,"REGIONID":"east"}`},
		{"oversized", "spiffe://test/region/east", false, false, http.StatusRequestEntityTooLarge, string(body) + strings.Repeat(" ", 1024)},
		{"not found", "spiffe://test/region/east", false, false, http.StatusNotFound, strings.Replace(string(body), `"event"`, `"absent"`, 1)},
		{"backend error", "spiffe://test/region/east", false, false, http.StatusServiceUnavailable, strings.Replace(string(body), `"event"`, `"unavailable"`, 1)},
	} {
		t.Run(test.name, func(t *testing.T) {
			config := &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS13}
			if !test.missing {
				config.Certificates = []tls.Certificate{issue(test.identity, true, test.expired)}
			}
			transport := &http.Transport{TLSClientConfig: config}
			defer transport.CloseIdleConnections()
			client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
			payload := body
			if test.body != "" {
				payload = []byte(test.body)
			}
			request, _ := http.NewRequest(http.MethodPost, server.URL+"/internal/region/revisions/resolve", bytes.NewReader(payload))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("X-Region-ID", "east")
			response, err := client.Do(request)
			if test.status == 0 {
				if err == nil {
					response.Body.Close()
					t.Fatal("invalid certificate accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if response.StatusCode != test.status || response.Header.Get("Cache-Control") != "no-store" {
				t.Fatalf("status=%d wanted=%d", response.StatusCode, test.status)
			}
			if test.status == http.StatusOK {
				var snapshot regional.RevisionSnapshot
				if err := json.NewDecoder(response.Body).Decode(&snapshot); err != nil || snapshot.Event != event {
					t.Fatalf("snapshot changed: %v", err)
				}
			} else {
				failure, _ := io.ReadAll(response.Body)
				if strings.Contains(string(failure), "private database detail") {
					t.Fatal("backend details exposed")
				}
			}
		})
	}
}
