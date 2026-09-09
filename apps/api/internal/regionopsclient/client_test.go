package regionopsclient

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/regional"
)

func TestDirectoryUsesMutualTLSAndChecksRegionResponse(t *testing.T) {
	ca, caKey, roots := testCA(t)
	serverCertificate := testCertificate(t, ca, caKey, true, "", []net.IP{net.ParseIP("127.0.0.1")})
	clientCertificate := testCertificate(t, ca, caKey, false, "spiffe://gamepanel/global-control", nil)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil || len(r.TLS.VerifiedChains) == 0 || r.URL.Query().Get("limit") != "25" {
			http.Error(w, "invalid request", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/internal/operations/nodes" && r.URL.Query().Get("after") == "oversized" {
			_, _ = w.Write([]byte(`{"regionId":"east","observedAtMs":1,"nodes":[]}` + strings.Repeat(" ", 5000)))
			return
		}
		if r.URL.Query().Get("after") != "node-a" {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		switch r.URL.Path {
		case "/internal/operations/nodes":
			_ = json.NewEncoder(w).Encode(regional.NodeOperationsPage{RegionID: "east", ObservedAtMS: 1, Nodes: []regional.NodeOperations{}})
		case "/internal/operations/deployments":
			_ = json.NewEncoder(w).Encode(regional.DeploymentOperationsPage{RegionID: "east", ObservedAtMS: 2, Deployments: []regional.DeploymentOperations{}})
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{serverCertificate}, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: roots}
	server.StartTLS()
	defer server.Close()
	directory, err := NewDirectory(map[string]string{"east": server.URL}, clientCertificate, roots, time.Second, 4096)
	if err != nil {
		t.Fatal(err)
	}
	defer directory.Close()
	page, err := directory.ListRegionalNodes(context.Background(), "east", "node-a", 25)
	if err != nil || page.RegionID != "east" || page.ObservedAtMS != 1 {
		t.Fatalf("mutual TLS operations read: %+v %v", page, err)
	}
	if _, err := directory.ListRegionalNodes(context.Background(), "west", "", 25); err != ErrUnavailable {
		t.Fatalf("unknown Region endpoint: %v", err)
	}
	deployments, err := directory.ListRegionalDeployments(context.Background(), "east", "node-a", 25)
	if err != nil || deployments.ObservedAtMS != 2 {
		t.Fatalf("deployment operations read: %+v %v", deployments, err)
	}
	if _, err := directory.ListRegionalNodes(context.Background(), "east", "oversized", 25); err != ErrInvalidResponse {
		t.Fatalf("oversized response accepted: %v", err)
	}
}

func testCA(t *testing.T) (*x509.Certificate, *ecdsa.PrivateKey, *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test-ca"}, NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certificate, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(certificate)
	return certificate, key, roots
}

func testCertificate(t *testing.T, ca *x509.Certificate, caKey *ecdsa.PrivateKey, server bool, uri string, ips []net.IP) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	usage := x509.ExtKeyUsageClientAuth
	if server {
		usage = x509.ExtKeyUsageServerAuth
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(now.UnixNano()), Subject: pkix.Name{CommonName: "test-peer"}, NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{usage}, IPAddresses: ips}
	if uri != "" {
		parsed, err := url.Parse(uri)
		if err != nil {
			t.Fatal(err)
		}
		template.URIs = []*url.URL{parsed}
	}
	der, err := x509.CreateCertificate(rand.Reader, template, ca, &key.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der, ca.Raw}, PrivateKey: key}
}
