package s3archive

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/assetfiles"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/assets"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/backup"
)

// Opt-in: creates only an isolated loopback container and temporary data.
// The pinned image must already have been pulled; production endpoints are not accepted.
func TestMinIOArchiveIntegration(t *testing.T) {
	if os.Getenv("GAMEPANEL_TEST_MINIO") != "1" {
		t.Skip("set GAMEPANEL_TEST_MINIO=1 for isolated Docker integration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*2)
	defer cancel()
	opts := startArchiveMinIO(t, ctx)
	store, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	_, err = store.client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(opts.Bucket)})
	if err != nil {
		t.Fatalf("create test bucket: %v", err)
	}
	_, err = store.client.PutBucketVersioning(ctx, &s3.PutBucketVersioningInput{Bucket: aws.String(opts.Bucket), VersioningConfiguration: &types.VersioningConfiguration{Status: types.BucketVersioningStatusEnabled}})
	if err != nil {
		t.Fatalf("enable test versioning: %v", err)
	}
	work := t.TempDir()
	source := filepath.Join(work, "worlds")
	if err := os.Mkdir(source, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "save.dat"), []byte("consistent stopped world"), 0600); err != nil {
		t.Fatal(err)
	}
	metadata := backup.Metadata{FormatVersion: 1, GameKey: "fixture", ProviderKey: "fixture", ConfigVersion: 1}
	archivePath, size, err := backup.NewService(work).WithMetadata(metadata).Create("instance", source)
	if err != nil {
		t.Fatal(err)
	}
	archive, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	version := assets.PublishedVersion{OrganizationID: "tenant", AssetID: "backup", Version: "one", SizeBytes: size, SHA256: fmt.Sprintf("%x", sha256.Sum256(archive))}
	testArchiveWorker(t, ctx, store, version, archive)
	ref, err := store.Upload(ctx, "backup-one", version, bytes.NewReader(archive))
	if err != nil || ref.ObjectVersion == "" || ref.ObjectVersion == "null" {
		t.Fatalf("versioned upload: %+v %v", ref, err)
	}
	if _, err := store.Upload(ctx, "backup-one", version, bytes.NewReader(archive)); err == nil {
		t.Fatal("conditional upload overwrote existing object")
	}
	// Forget the upload result as a crashed publisher would. Reopen the adapter
	// and recover only from the durable task key and expected archive manifest.
	reopened, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	recovered, err := reopened.ResolveUpload(ctx, "backup-one", version)
	if err != nil || recovered != ref {
		t.Fatalf("lost receipt recovery: %+v %v", recovered, err)
	}
	// Simulate an administrative overwrite outside our create-only adapter. The
	// recorded backend version must still retrieve the original immutable backup.
	_, err = store.client.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(opts.Bucket), Key: aws.String(ref.ObjectKey), Body: strings.NewReader("replacement")})
	if err != nil {
		t.Fatal(err)
	}
	stream, err := store.Open(ctx, ref)
	if _, recoverErr := reopened.ResolveUpload(ctx, "backup-one", version); recoverErr == nil {
		t.Fatal("recovery adopted a replacement object with different bytes")
	}
	if err != nil {
		t.Fatal(err)
	}
	files, err := assetfiles.New(t.TempDir(), 1<<20)
	if err != nil {
		stream.Close()
		t.Fatal(err)
	}
	defer files.Close()
	if err := files.Put(ctx, version, stream); err != nil {
		t.Fatalf("stage verified backup: %v", err)
	}
	staged, err := files.Open(ctx, version)
	if err != nil {
		t.Fatal(err)
	}
	reader, ok := staged.(io.ReaderAt)
	if !ok {
		staged.Close()
		t.Fatal("local verified archive does not support seekable restore")
	}
	defer staged.Close()
	target := t.TempDir()
	committed := false
	err = backup.RestoreArchiveChecked(reader, size, target, backup.RestoreHooks{
		Validate: func(got backup.Metadata) error {
			if got != metadata {
				return fmt.Errorf("metadata mismatch")
			}
			return nil
		},
		Commit: func() error { committed = true; return nil },
	})
	if err != nil || !committed {
		t.Fatalf("restore: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(target, "save.dat"))
	if err != nil || string(got) != "consistent stopped world" {
		t.Fatalf("restored bytes %q %v", got, err)
	}
	missing := ref
	missing.ObjectVersion = "00000000-0000-0000-0000-000000000000"
	if body, err := store.Open(ctx, missing); err == nil {
		body.Close()
		t.Fatal("missing version accepted")
	}
	// The actual service must validate the supplied SHA-256, not merely echo it.
	_, err = store.client.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(opts.Bucket), Key: aws.String("bad-checksum"), Body: strings.NewReader("bad"), ChecksumSHA256: aws.String("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=")})
	if err == nil {
		t.Fatal("storage accepted incorrect checksum")
	}
	badOpts := opts
	badOpts.Credentials = aws.CredentialsProviderFunc(func(context.Context) (aws.Credentials, error) {
		return aws.Credentials{AccessKeyID: "invalid-access", SecretAccessKey: "invalid-secret"}, nil
	})
	unauthorized, err := New(badOpts)
	if err != nil {
		t.Fatal(err)
	}
	defer unauthorized.Close()
	if body, err := unauthorized.Open(ctx, ref); err == nil {
		body.Close()
		t.Fatal("invalid credentials accepted")
	}
}

func startArchiveMinIO(t *testing.T, ctx context.Context) Options {
	t.Helper()
	dir := t.TempDir()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	cert := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "gamepanel isolated storage test"}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour), IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, cert, cert, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "public.crt"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "private.key"), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), 0600); err != nil {
		t.Fatal(err)
	}
	parsed, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(parsed)
	access, secret := "test"+strings.ToLower(rand.Text()), rand.Text()+rand.Text()
	name := "gamepanel-s3-test-" + strings.ToLower(rand.Text())
	const image = "minio/minio:RELEASE.2025-04-22T22-12-26Z"
	args := []string{"run", "--pull=never", "--rm", "-d", "--name", name, "-p", "127.0.0.1::9000", "-v", dir + ":/certs:ro", "-e", "MINIO_ROOT_USER=" + access, "-e", "MINIO_ROOT_PASSWORD=" + secret, image, "server", "/data", "--certs-dir", "/certs"}
	if output, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput(); err != nil {
		t.Fatalf("start isolated storage: %v %s", err, output)
	}
	t.Cleanup(func() {
		cleanCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if output, err := exec.CommandContext(cleanCtx, "docker", "stop", name).CombinedOutput(); err != nil {
			t.Errorf("remove isolated storage: %v %s", err, output)
		}
	})
	output, err := exec.CommandContext(ctx, "docker", "port", name, "9000/tcp").Output()
	if err != nil {
		t.Fatal(err)
	}
	address := strings.TrimSpace(string(output))
	host, _, err := net.SplitHostPort(address)
	if err != nil || host != "127.0.0.1" {
		t.Fatalf("unexpected test address %q", address)
	}
	opts := Options{StorageID: "region-test", Endpoint: "https://" + address, SigningRegion: "us-east-1", Bucket: "gamepanel-archives", RootCAs: pool, Timeout: 10 * time.Second, MaxBytes: 1 << 20,
		Credentials: aws.CredentialsProviderFunc(func(context.Context) (aws.Credentials, error) {
			return aws.Credentials{AccessKeyID: access, SecretAccessKey: secret}, nil
		})}
	probe, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	defer probe.Close()
	readyCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	for {
		_, err := probe.client.ListBuckets(readyCtx, &s3.ListBucketsInput{})
		if err == nil {
			break
		}
		select {
		case <-readyCtx.Done():
			t.Fatalf("storage did not become ready: %v", err)
		case <-time.After(100 * time.Millisecond):
		}
	}
	t.Logf("isolated S3 compatibility target: %s; TLS; versioning enabled by test", image)
	return opts
}

// The task port is an explicit fixture here; PostgreSQL lease/Outbox behavior is
// covered in store tests. This exercises actual local files, Worker and S3 I/O.
type archiveWorkerTask struct {
	claim          *backup.UploadClaim
	receipt        backup.StoredArchive
	failCompletion bool
}

func (t *archiveWorkerTask) PrepareArchiveUpload(context.Context, backup.UploadPlan) error {
	return nil
}
func (t *archiveWorkerTask) ClaimArchiveUpload(context.Context, time.Duration) (*backup.UploadClaim, error) {
	return t.claim, nil
}
func (t *archiveWorkerTask) RetryArchiveUpload(context.Context, backup.UploadClaim, time.Duration) error {
	return nil
}
func (t *archiveWorkerTask) CompleteArchiveUpload(_ context.Context, _ backup.UploadClaim, r backup.StoredArchive) error {
	if t.failCompletion {
		return fmt.Errorf("injected lost completion")
	}
	t.receipt = r
	t.claim = nil
	return nil
}

func testArchiveWorker(t *testing.T, ctx context.Context, storage *Store, version assets.PublishedVersion, data []byte) {
	t.Helper()
	files, err := assetfiles.New(t.TempDir(), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	defer files.Close()
	if err := files.Put(ctx, version, io.NopCloser(bytes.NewReader(data))); err != nil {
		t.Fatal(err)
	}
	plan := backup.UploadPlan{ID: "worker-upload", OperationID: "worker-operation", RequestEventID: "worker-request", RegionID: "east", ServerID: "server", DeploymentID: "deployment", NodeID: "node", SnapshotID: "snapshot", PlacementEpoch: 1, StorageID: storage.storageID, ObjectKey: "worker-archive", Asset: version}
	task := &archiveWorkerTask{claim: &backup.UploadClaim{Token: "fixture", Plan: plan}, failCompletion: true}
	worker := backup.UploadWorker{Tasks: task, Files: files, Archives: storage, StorageID: storage.storageID, Lease: time.Minute, Timeout: 20 * time.Second, RetryDelay: time.Second}
	if ok, err := worker.RunOnce(ctx); ok || err == nil {
		t.Fatal("completion failure ignored")
	}
	if err := files.Close(); err != nil {
		t.Fatal(err)
	}
	task.failCompletion = false
	if ok, err := worker.RunOnce(ctx); !ok || err != nil {
		t.Fatal("S3 receipt recovery failed", err)
	}
	if task.receipt.ValidateFor(plan) != nil || task.receipt.ObjectVersion == "" {
		t.Fatal("invalid recovered S3 receipt")
	}
}
