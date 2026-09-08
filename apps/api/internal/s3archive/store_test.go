package s3archive

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/assets"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/backup"
)

func TestS3ArchiveProtocol(t *testing.T) {
	ctx := context.Background()
	data := []byte("archive bytes")
	digest := sha256.Sum256(data)
	version := assets.PublishedVersion{OrganizationID: "org", AssetID: "backup", Version: "one", SHA256: fmt.Sprintf("%x", digest), SizeBytes: int64(len(data))}
	var mu sync.Mutex
	var stored []byte
	calls := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		calls++
		if !strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 ") || r.URL.Path != "/archives/object-one" {
			t.Errorf("unexpected signed request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(400)
			return
		}
		switch r.Method {
		case "PUT":
			if r.Header.Get("If-None-Match") != "*" || r.Header.Get("X-Amz-Checksum-Sha256") != base64.StdEncoding.EncodeToString(digest[:]) {
				t.Error("missing create-only/checksum headers")
				w.WriteHeader(400)
				return
			}
			if stored != nil {
				w.WriteHeader(412)
				return
			}
			var err error
			stored, err = io.ReadAll(r.Body)
			if err != nil || !bytes.Equal(stored, data) {
				t.Errorf("upload bytes %q, %v", stored, err)
				w.WriteHeader(400)
				return
			}
			w.Header().Set("X-Amz-Version-Id", "backend-version")
			w.WriteHeader(200)
		case "GET":
			if r.URL.Query().Get("versionId") != "backend-version" {
				t.Error("version lost")
				w.WriteHeader(400)
				return
			}
			w.Header().Set("Content-Length", fmt.Sprint(len(stored)))
			w.Header().Set("X-Amz-Version-Id", "backend-version")
			_, _ = w.Write(stored)
		default:
			w.WriteHeader(405)
		}
	}))
	defer server.Close()
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	opts := Options{StorageID: "regional-storage", Endpoint: server.URL, SigningRegion: "local-region", Bucket: "archives", RootCAs: roots, Timeout: time.Second * 3, MaxBytes: 1024,
		Credentials: aws.CredentialsProviderFunc(func(context.Context) (aws.Credentials, error) {
			return aws.Credentials{AccessKeyID: "test-access", SecretAccessKey: "test-secret"}, nil
		})}
	s, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ref, err := s.Upload(ctx, "object-one", version, bytes.NewReader(data))
	if err != nil || ref.ObjectVersion != "backend-version" || ref.Asset != version {
		t.Fatalf("upload %+v %v", ref, err)
	}
	stream, err := s.Open(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(stream)
	stream.Close()
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("download %q %v", got, err)
	}
	if _, err := s.Upload(ctx, "object-one", version, bytes.NewReader(data)); err == nil {
		t.Fatal("existing key overwritten")
	}
	mu.Lock()
	before := calls
	mu.Unlock()
	for _, payload := range [][]byte{[]byte("wrong"), append(append([]byte(nil), data...), 'x')} {
		if _, err := s.Upload(ctx, "object-two", version, bytes.NewReader(payload)); err == nil {
			t.Fatal("bad source uploaded")
		}
	}
	bad := ref
	bad.StorageID = "other"
	if _, err := s.Open(ctx, bad); err == nil {
		t.Fatal("wrong storage accepted")
	}
	bad = ref
	bad.ObjectKey = "../object-one"
	if _, err := s.Open(ctx, bad); err == nil {
		t.Fatal("path key accepted")
	}
	mu.Lock()
	after := calls
	mu.Unlock()
	if after != before {
		t.Fatal("invalid input reached backend")
	}
	opts.Endpoint = "http://localhost:9000"
	if _, err := New(opts); err == nil {
		t.Fatal("plaintext storage accepted")
	}
}

func TestS3ReadRejectsWrongVersionAndBoundsStream(t *testing.T) {
	for _, mode := range []string{"wrong-version", "wrong-size", "blocked", "redirect"} {
		t.Run(mode, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if mode == "redirect" {
					w.Header().Set("Location", "https://example.invalid/secret")
					w.WriteHeader(307)
					return
				}
				v, size := "exact", "4"
				if mode == "wrong-version" {
					v = "other"
				}
				if mode == "wrong-size" {
					size = "5"
				}
				w.Header().Set("X-Amz-Version-Id", v)
				w.Header().Set("Content-Length", size)
				w.WriteHeader(200)
				w.(http.Flusher).Flush()
				if mode == "blocked" {
					<-r.Context().Done()
					return
				}
				_, _ = io.WriteString(w, "data")
			}))
			defer server.Close()
			roots := x509.NewCertPool()
			roots.AddCert(server.Certificate())
			s, err := New(Options{StorageID: "store", Endpoint: server.URL, SigningRegion: "local", Bucket: "archives", RootCAs: roots, Timeout: 500 * time.Millisecond, MaxBytes: 1024,
				Credentials: aws.CredentialsProviderFunc(func(context.Context) (aws.Credentials, error) {
					return aws.Credentials{AccessKeyID: "test", SecretAccessKey: "secret"}, nil
				})})
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			ref := backup.StoredArchive{StorageID: "store", ObjectKey: "object", ObjectVersion: "exact", Asset: assets.PublishedVersion{OrganizationID: "org", AssetID: "backup", Version: "one", SHA256: fmt.Sprintf("%x", sha256.Sum256([]byte("data"))), SizeBytes: 4}}
			body, err := s.Open(context.Background(), ref)
			if mode != "blocked" {
				if err == nil {
					body.Close()
					t.Fatal("invalid response accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			defer body.Close()
			if _, err := io.ReadAll(body); err == nil {
				t.Fatal("blocked stream succeeded")
			}
		})
	}
}
