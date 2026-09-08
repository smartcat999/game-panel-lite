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
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/regional"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
)

func TestRegionalNodeControl(t *testing.T) {
	dsn := os.Getenv("GAMEPANEL_TEST_POSTGRES_URL")
	if dsn == "" {
		t.Skip("set GAMEPANEL_TEST_POSTGRES_URL for actual regional control integration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	schema := "node_control_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := admin.ExecContext(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec("DROP SCHEMA " + schema + " CASCADE")
	endpoint, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	q := endpoint.Query()
	q.Set("search_path", schema)
	endpoint.RawQuery = q.Encode()
	regionalDSN := endpoint.String()
	t.Setenv("GAMEPANEL_REGIONAL_DATABASE_URL", regionalDSN)
	if err := store.MigrateRegionalPostgres(ctx, regionalDSN, "east"); err != nil {
		t.Fatal(err)
	}
	db, err := store.OpenRegionalPostgres(regionalDSN, "east", 2)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	config := regional.NodeConfiguration{ID: "node-a", Name: "node-a", Architecture: "amd64", CPU: 4, MemoryMB: 4096}
	if _, err := db.ConfigureRegionalNode(ctx, config, 0); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	public, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	caTemplate := &x509.Certificate{SerialNumber: big.NewInt(1), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour)}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, public, key)
	if err != nil {
		t.Fatal(err)
	}
	ca, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	write := func(name string, data []byte) string {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	caPath := write("ca.pem", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}))
	issue := func(serial int64, client bool) (string, string) {
		t.Helper()
		pub, private, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		template := &x509.Certificate{SerialNumber: big.NewInt(serial), NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature}
		if client {
			template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
			identity, _ := url.Parse("spiffe://test/node/a")
			template.URIs = []*url.URL{identity}
		} else {
			template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
			template.IPAddresses = []net.IP{net.ParseIP("127.0.0.1")}
		}
		der, err := x509.CreateCertificate(rand.Reader, template, ca, pub, key)
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := x509.MarshalPKCS8PrivateKey(private)
		if err != nil {
			t.Fatal(err)
		}
		return write(fmt.Sprintf("%d.crt", serial), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})), write(fmt.Sprintf("%d.key", serial), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: encoded}))
	}
	certPath, keyPath := issue(2, false)
	clientCert, clientKey := issue(3, true)
	identities := write("identities.json", []byte(`{"spiffe://test/node/a":"node-a"}`))
	certificate, err := tls.LoadX509KeyPair(clientCert, clientKey)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: roots, Certificates: []tls.Certificate{certificate}}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: time.Second}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	listener.Close()
	runCtx, stop := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		done <- run(runCtx, "east", address, certPath, keyPath, caPath, identities, 2*time.Second, 1024)
	}()
	defer func() {
		stop()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(3 * time.Second):
			t.Error("regional control did not stop")
		}
	}()
	var session regional.NodeSession
	for {
		request, _ := http.NewRequestWithContext(ctx, "POST", "https://"+address+"/internal/node/session", nil)
		response, err := client.Do(request)
		if err == nil {
			decodeErr := json.NewDecoder(response.Body).Decode(&session)
			response.Body.Close()
			if response.StatusCode != 200 || decodeErr != nil || session.Epoch < 1 {
				t.Fatal("invalid node session response")
			}
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("regional control did not become ready")
		case <-time.After(20 * time.Millisecond):
		}
	}
	body := fmt.Sprintf(`{"sessionEpoch":%d,"sequence":1,"architecture":"amd64","runtimeReady":true}`, session.Epoch)
	request, _ := http.NewRequestWithContext(ctx, "POST", "https://"+address+"/internal/node/heartbeat", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 204 {
		t.Fatal("heartbeat not recorded")
	}
	var epoch, sequence, lastSeen int64
	var ready bool
	if err := admin.QueryRowContext(ctx, "SELECT epoch,sequence,last_seen_ms,runtime_ready FROM "+schema+".regional_node_sessions WHERE node_id=$1", "node-a").Scan(&epoch, &sequence, &lastSeen, &ready); err != nil {
		t.Fatal(err)
	}
	if epoch != session.Epoch || sequence != 1 || lastSeen <= 0 || !ready {
		t.Fatal("heartbeat did not reach regional database")
	}
	nodes, err := db.ListRegionalNodes(ctx, "", 1)
	if err != nil || len(nodes) != 1 || nodes[0].NodeConfiguration != config || nodes[0].Version != 1 {
		t.Fatal("node observation changed configuration")
	}
}
