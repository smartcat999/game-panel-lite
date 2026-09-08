package s3archive

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/assets"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/backup"
)

func TestResolveUploadRequiresVerifiedContent(t *testing.T) {
	for _, mode := range []string{"valid", "wrong-content", "truncated", "blocked", "missing"} {
		t.Run(mode, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" || r.URL.Path != "/archives/task-key" || r.URL.Query().Get("versionId") != "" {
					t.Error("unexpected recovery request")
				}
				if mode == "missing" {
					w.WriteHeader(404)
					return
				}
				w.Header().Set("X-Amz-Version-Id", "observed-version")
				w.Header().Set("Content-Length", "4")
				w.WriteHeader(200)
				switch mode {
				case "wrong-content":
					_, _ = io.WriteString(w, "evil")
				case "truncated":
					_, _ = io.WriteString(w, "da")
				case "blocked":
					w.(http.Flusher).Flush()
					<-r.Context().Done()
				default:
					_, _ = io.WriteString(w, "data")
				}
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
			version := assets.PublishedVersion{OrganizationID: "org", AssetID: "backup", Version: "one", SHA256: fmt.Sprintf("%x", sha256.Sum256([]byte("data"))), SizeBytes: 4}
			ref, err := s.ResolveUpload(context.Background(), "task-key", version)
			if mode == "valid" {
				if err != nil || ref.Asset != version || ref.ObjectVersion != "observed-version" || ref.StorageID != "store" || ref.ObjectKey != "task-key" {
					t.Fatalf("recovered %+v %v", ref, err)
				}
			} else if err == nil || ref != (backup.StoredArchive{}) {
				t.Fatalf("unverified receipt %+v %v", ref, err)
			}
		})
	}
}
