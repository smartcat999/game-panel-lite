package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/internal/workload"
)

func regionalTLS(t *testing.T, handler http.Handler) (*regionalClient, *httptest.Server) {
	t.Helper()
	now := time.Now()
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, template, template, pub, key)
	if err != nil {
		t.Fatal(err)
	}
	ca, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	issue := func(serial int64, client bool) tls.Certificate {
		t.Helper()
		public, private, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		cert := &x509.Certificate{SerialNumber: big.NewInt(serial), NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature}
		if client {
			cert.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
			identity, _ := url.Parse("spiffe://test/node/a")
			cert.URIs = []*url.URL{identity}
		} else {
			cert.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
			cert.IPAddresses = []net.IP{net.ParseIP("127.0.0.1")}
		}
		encoded, err := x509.CreateCertificate(rand.Reader, cert, ca, public, key)
		if err != nil {
			t.Fatal(err)
		}
		return tls.Certificate{Certificate: [][]byte{encoded}, PrivateKey: private}
	}
	server := httptest.NewUnstartedServer(handler)
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{issue(2, false)}, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: roots}
	server.StartTLS()
	t.Cleanup(server.Close)
	client, err := newRegionalClient(server.URL, issue(3, true), roots, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	return client, server
}

func TestRegionalClient(t *testing.T) {
	var code atomic.Int64
	code.Store(200)
	var body atomic.Value
	body.Store(`{"epoch":7}`)
	var redirected atomic.Int64
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { redirected.Add(1) }))
	defer target.Close()
	client, _ := regionalTLS(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil || len(r.TLS.VerifiedChains) == 0 {
			t.Error("client identity not verified")
		}
		status := int(code.Load())
		if status == -1 {
			<-r.Context().Done()
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Location", target.URL)
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body.Load().(string))
	}))
	t.Setenv("HTTPS_PROXY", target.URL)
	session, err := client.Start(context.Background())
	if err != nil || session.Epoch != 7 {
		t.Fatal("session", err)
	}
	for _, raw := range []string{`{"epoch":0}`, `{"epoch":1,"epoch":2}`, `{"Epoch":1}`, `{"epoch":1} {}`, strings.Repeat(" ", 1025) + `{"epoch":1}`} {
		body.Store(raw)
		if _, err := client.Start(context.Background()); !errors.Is(err, errRegionalResponse) {
			t.Fatal("invalid session accepted", raw)
		}
	}
	body.Store(`{"epoch":7}`)
	for _, status := range []int64{401, 403, 404} {
		code.Store(status)
		if _, err := client.Start(context.Background()); !errors.Is(err, errRegionalIdentity) {
			t.Fatal("identity failure accepted")
		}
	}
	code.Store(307)
	if _, err := client.Start(context.Background()); err == nil {
		t.Fatal("redirect accepted")
	}
	if redirected.Load() != 0 {
		t.Fatal("credential followed redirect or proxy")
	}
	code.Store(409)
	if err := client.Heartbeat(context.Background(), workload.NodeHeartbeat{}); !errors.Is(err, errRegionalSession) {
		t.Fatal("session replacement ignored")
	}
	code.Store(200)
	if err := client.Heartbeat(context.Background(), workload.NodeHeartbeat{}); err == nil {
		t.Fatal("unexpected heartbeat success accepted")
	}
	code.Store(204)
	if err := client.Heartbeat(context.Background(), workload.NodeHeartbeat{}); err != nil {
		t.Fatal(err)
	}
	for _, endpoint := range []string{"http://example.test", "https://u:p@example.test", "https://example.test/path", "https://example.test?x=1", "https://example.test#fragment"} {
		cfg := client.transport.TLSClientConfig
		if c, err := newRegionalClient(endpoint, cfg.Certificates[0], cfg.RootCAs, time.Second); err == nil {
			c.Close()
			t.Fatal("invalid endpoint accepted")
		}
	}
	code.Store(-1)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := client.Start(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("cancellation not propagated", err)
	}
}

type regionalInfoFunc func(context.Context) (workload.RuntimeInfo, error)

func (f regionalInfoFunc) Info(ctx context.Context) (workload.RuntimeInfo, error) { return f(ctx) }

func TestRegionalHeartbeatLoop(t *testing.T) {
	var sessions atomic.Int64
	observations := make(chan workload.NodeHeartbeat, 3)
	client, _ := regionalTLS(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/internal/node/session" {
			sessions.Add(1)
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"epoch":5}`)
			return
		}
		if r.URL.Path != "/internal/node/heartbeat" {
			t.Error("unexpected legacy endpoint")
		}
		var h workload.NodeHeartbeat
		if json.NewDecoder(r.Body).Decode(&h) != nil {
			t.Error("invalid heartbeat")
		}
		observations <- h
		switch h.Sequence {
		case 1:
			w.WriteHeader(204)
		case 2:
			w.WriteHeader(503)
		default:
			w.WriteHeader(409)
		}
	}))
	calls := 0
	runtime := regionalInfoFunc(func(context.Context) (workload.RuntimeInfo, error) {
		calls++
		if calls == 1 || calls == 3 {
			return workload.RuntimeInfo{}, errors.New("runtime down")
		}
		return workload.RuntimeInfo{Architecture: "x86_64"}, nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := runRegionalHeartbeats(ctx, client, runtime, slog.New(slog.NewTextHandler(io.Discard, nil)), time.Millisecond, 100*time.Millisecond)
	if !errors.Is(err, errRegionalSession) || sessions.Load() != 1 {
		t.Fatal("replaced session re-enrolled", err)
	}
	for i, ready := range []bool{true, false, true} {
		select {
		case h := <-observations:
			if h.SessionEpoch != 5 || h.Sequence != int64(i+1) || h.Architecture != "amd64" || h.RuntimeReady != ready {
				t.Fatal("incorrect runtime observation", h)
			}
		default:
			t.Fatal("missing observation")
		}
	}
}
