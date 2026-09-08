package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/controlapi"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/regional"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/serviceauth"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
)

func TestFetcherPostgresMutualTLS(t *testing.T) {
	dsn := os.Getenv("GAMEPANEL_TEST_POSTGRES_URL")
	if dsn == "" {
		t.Skip("requires dedicated PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	newSchema := func() (string, string) {
		t.Helper()
		name := "fetch_cli_" + strings.ReplaceAll(uuid.NewString(), "-", "")
		if _, err := admin.ExecContext(ctx, "CREATE SCHEMA "+name); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			db, err := sql.Open("pgx", dsn)
			if err != nil {
				t.Error(err)
				return
			}
			defer db.Close()
			if _, err := db.Exec("DROP SCHEMA " + name + " CASCADE"); err != nil {
				t.Error(err)
			}
		})
		u, err := url.Parse(dsn)
		if err != nil {
			t.Fatal(err)
		}
		q := u.Query()
		q.Set("search_path", name)
		u.RawQuery = q.Encode()
		return u.String(), name
	}
	globalDSN, _ := newSchema()
	regionDSN, schema := newSchema()
	if err := store.MigratePostgres(ctx, globalDSN); err != nil {
		t.Fatal(err)
	}
	if err := store.MigrateRegionalPostgres(ctx, regionDSN, "east"); err != nil {
		t.Fatal(err)
	}
	global, err := store.OpenConfigured("", globalDSN, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer global.Close()
	region, err := store.OpenRegionalPostgres(regionDSN, "east", 2)
	if err != nil {
		t.Fatal(err)
	}
	defer region.Close()
	org := domain.Organization{ID: "org", Slug: "org"}
	if err := global.CreateOrganization(ctx, &org, "owner"); err != nil {
		t.Fatal(err)
	}
	_, err = global.CreateGlobalServer(ctx, "owner", instances.CreateRequest{OrganizationID: org.ID, Name: "server", RegionID: "east", IdempotencyKey: "create", Specification: instances.Specification{ProviderKey: "test", GameVersion: "1", ConfigSchemaVersion: 1, Resources: instances.Resources{CPU: 1, MemoryMB: 256}, Configuration: instances.ProtectedConfiguration{KeyID: "test", Ciphertext: []byte("opaque")}}})
	if err != nil {
		t.Fatal(err)
	}
	messages, err := global.ClaimOutbox(ctx, "east", 1, time.Minute)
	if err != nil || len(messages) != 1 {
		t.Fatalf("outbox: %v", err)
	}
	var event instances.RevisionAvailable
	if err := json.Unmarshal([]byte(messages[0].Payload), &event); err != nil {
		t.Fatal(err)
	}
	if err := region.RecordRevisionNotification(ctx, event); err != nil {
		t.Fatal(err)
	}
	serverCert, clientCert, ca := testCertificates(t)
	pool := x509.NewCertPool()
	pool.AddCert(ca)
	identities, err := serviceauth.NewRegions(map[string]string{"spiffe://test/region/east": "east"})
	if err != nil {
		t.Fatal(err)
	}
	handler, err := controlapi.NewRevisionHandler(global, identities, 65536)
	if err != nil {
		t.Fatal(err)
	}
	var available atomic.Bool
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !available.Load() {
			http.Error(w, "temporarily unavailable", 503)
			return
		}
		handler.ServeHTTP(w, r)
	}))
	server.TLS, err = serviceauth.ServerTLS(serverCert, pool)
	if err != nil {
		t.Fatal(err)
	}
	server.StartTLS()
	defer server.Close()
	dir := t.TempDir()
	write := func(name string, data []byte) string {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	key, err := x509.MarshalPKCS8PrivateKey(clientCert.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	o := options{region: "east", endpoint: server.URL, dsn: regionDSN, lease: 5 * time.Second, requestTimeout: time.Second, taskTimeout: 2 * time.Second, retry: 50 * time.Millisecond, poll: 10 * time.Millisecond, maxBytes: 65536, certificate: write("client.crt", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: clientCert.Certificate[0]})), key: write("client.key", pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: key})), ca: write("ca.crt", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.Raw}))}
	start := func() (context.CancelFunc, chan error) {
		workerCtx, stop := context.WithCancel(ctx)
		done := make(chan error, 1)
		go func() { done <- run(workerCtx, o) }()
		return stop, done
	}
	stop, done := start()
	defer stop()
	waitFor := func(check func() bool) {
		t.Helper()
		for !check() {
			select {
			case <-ctx.Done():
				t.Fatal("timed out waiting for task")
			case <-time.After(10 * time.Millisecond):
			}
		}
	}
	var status, snapshot string
	var attempts int
	var nextAttempt int64
	read := func() bool {
		err := admin.QueryRowContext(ctx, "SELECT status,COALESCE(snapshot,''),attempts,next_attempt_ms FROM "+schema+".regional_revision_tasks WHERE operation_id=$1", event.OperationID).Scan(&status, &snapshot, &attempts, &nextAttempt)
		if err != nil {
			t.Fatal(err)
		}
		return true
	}
	waitFor(func() bool { read(); return attempts > 0 && nextAttempt > 0 })
	stop()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("worker failed to stop")
	}
	if status != "awaiting_revision" {
		t.Fatal("failed request completed task")
	}
	available.Store(true)
	stop, done = start()
	defer stop()
	waitFor(func() bool { read(); return status == "revision_fetched" })
	stop()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("restarted worker failed to stop")
	}
	var saved regional.RevisionSnapshot
	if err := json.Unmarshal([]byte(snapshot), &saved); err != nil || saved.ValidateFor(event) != nil {
		t.Fatalf("invalid saved snapshot: %v", err)
	}
}

func testCertificates(t *testing.T) (tls.Certificate, tls.Certificate, *x509.Certificate) {
	t.Helper()
	now := time.Now()
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign, NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, template, template, pub, key)
	if err != nil {
		t.Fatal(err)
	}
	ca, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	issue := func(client bool) tls.Certificate {
		pub, private, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		leaf := &x509.Certificate{SerialNumber: big.NewInt(2), NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature}
		if client {
			leaf.SerialNumber = big.NewInt(3)
			leaf.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
			uri, _ := url.Parse("spiffe://test/region/east")
			leaf.URIs = []*url.URL{uri}
		} else {
			leaf.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
			leaf.IPAddresses = []net.IP{net.ParseIP("127.0.0.1")}
		}
		encoded, err := x509.CreateCertificate(rand.Reader, leaf, ca, pub, key)
		if err != nil {
			t.Fatal(err)
		}
		return tls.Certificate{Certificate: [][]byte{encoded}, PrivateKey: private}
	}
	return issue(false), issue(true), ca
}
