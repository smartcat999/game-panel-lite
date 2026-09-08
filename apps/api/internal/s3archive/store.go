// Package s3archive implements regional archive storage using the S3 protocol.
package s3archive

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/assets"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/backup"
)

var ErrStorage = errors.New("archive storage operation failed")

type Options struct {
	StorageID, Endpoint, SigningRegion, Bucket string
	Credentials                                aws.CredentialsProvider
	RootCAs                                    *x509.CertPool
	Timeout                                    time.Duration
	MaxBytes                                   int64
}

type Store struct {
	client            *s3.Client
	transport         *http.Transport
	storageID, bucket string
	timeout           time.Duration
	maxBytes          int64
}

var _ backup.ArchiveStore = (*Store)(nil)

func New(o Options) (*Store, error) {
	u, err := url.Parse(o.Endpoint)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || (u.Path != "" && u.Path != "/") || !validKey(o.StorageID) || o.SigningRegion == "" || !validBucket(o.Bucket) || o.Credentials == nil || o.Timeout <= 0 || o.MaxBytes < 1 || o.MaxBytes > 5<<30 {
		return nil, ErrStorage
	}
	var roots *x509.CertPool
	if o.RootCAs != nil {
		roots = o.RootCAs.Clone()
	}
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.Proxy = nil
	tr.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots}
	tr.ResponseHeaderTimeout = o.Timeout
	client := &http.Client{Transport: tr, Timeout: o.Timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	sdk := s3.New(s3.Options{
		BaseEndpoint: aws.String(o.Endpoint), Region: o.SigningRegion,
		Credentials: o.Credentials, HTTPClient: client, UsePathStyle: true,
		RetryMaxAttempts:           1,
		RequestChecksumCalculation: aws.RequestChecksumCalculationWhenRequired,
		ResponseChecksumValidation: aws.ResponseChecksumValidationWhenRequired,
	})
	return &Store{client: sdk, transport: tr, storageID: o.StorageID, bucket: o.Bucket, timeout: o.Timeout, maxBytes: o.MaxBytes}, nil
}

func (s *Store) Close() { s.transport.CloseIdleConnections() }

// Upload uses create-only PUT; an existing key is never overwritten, even on
// retry. Ambiguous outcomes are left to the durable task's reconciliation.
func (s *Store) Upload(ctx context.Context, key string, version assets.PublishedVersion, source io.ReaderAt) (backup.StoredArchive, error) {
	if !validKey(key) || version.Validate() != nil || version.SizeBytes > s.maxBytes || source == nil {
		return backup.StoredArchive{}, ErrStorage
	}
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	hash := sha256.New()
	n, err := io.Copy(hash, checkedReader{ctx, io.NewSectionReader(source, 0, version.SizeBytes)})
	if err != nil || n != version.SizeBytes || hex.EncodeToString(hash.Sum(nil)) != version.SHA256 {
		return backup.StoredArchive{}, ErrStorage
	}
	var extra [1]byte
	nn, endErr := source.ReadAt(extra[:], version.SizeBytes)
	if nn != 0 || endErr != io.EOF || ctx.Err() != nil {
		return backup.StoredArchive{}, ErrStorage
	}
	result, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket), Key: aws.String(key), IfNoneMatch: aws.String("*"),
		Body: io.NewSectionReader(source, 0, version.SizeBytes), ContentLength: aws.Int64(version.SizeBytes),
		ContentType: aws.String("application/zip"), ChecksumSHA256: aws.String(base64.StdEncoding.EncodeToString(hash.Sum(nil))),
	})
	if err != nil || ctx.Err() != nil {
		return backup.StoredArchive{}, ErrStorage
	}
	return backup.StoredArchive{StorageID: s.storageID, ObjectKey: key, ObjectVersion: aws.ToString(result.VersionId), Asset: version}, nil
}

func (s *Store) Open(ctx context.Context, ref backup.StoredArchive) (io.ReadCloser, error) {
	if ref.StorageID != s.storageID || !validKey(ref.ObjectKey) || ref.Asset.Validate() != nil || ref.Asset.SizeBytes > s.maxBytes || len(ref.ObjectVersion) > 1024 || strings.ContainsAny(ref.ObjectVersion, "\x00\r\n") {
		return nil, ErrStorage
	}
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	in := &s3.GetObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(ref.ObjectKey)}
	if ref.ObjectVersion != "" {
		in.VersionId = aws.String(ref.ObjectVersion)
	}
	out, err := s.client.GetObject(ctx, in)
	if err != nil {
		cancel()
		return nil, ErrStorage
	}
	if out.Body == nil || out.ContentLength == nil || *out.ContentLength != ref.Asset.SizeBytes || (ref.ObjectVersion != "" && aws.ToString(out.VersionId) != ref.ObjectVersion) {
		if out.Body != nil {
			_ = out.Body.Close()
		}
		cancel()
		return nil, ErrStorage
	}
	return &body{ReadCloser: out.Body, cancel: cancel}, nil
}

type body struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (b *body) Close() error { b.cancel(); return b.ReadCloser.Close() }

type checkedReader struct {
	ctx context.Context
	r   io.Reader
}

func (r checkedReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}

func validKey(s string) bool {
	if len(s) < 1 || len(s) > 128 {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z') && !(c >= '0' && c <= '9') && c != '-' && c != '_' {
			return false
		}
	}
	return true
}
func validBucket(s string) bool {
	if len(s) < 3 || len(s) > 63 || s[0] == '-' || s[len(s)-1] == '-' {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z') && !(c >= '0' && c <= '9') && c != '-' {
			return false
		}
	}
	return true
}
