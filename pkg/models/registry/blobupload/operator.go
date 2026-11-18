package blobupload

import (
	"context"
	"errors"
	"fmt"
	"time"

	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/config"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/s3"
	"github.com/smartcat999/game-panel-cloud/pkg/models"
	"github.com/smartcat999/game-panel-cloud/pkg/repo"
	blobdb "github.com/smartcat999/game-panel-cloud/pkg/repo/registry/blob"
	blobuploaddb "github.com/smartcat999/game-panel-cloud/pkg/repo/registry/blobupload"
)

const (
	defaultUploadPartSize = 32 << 20 // 32 MiB
	maxMultipartParts     = 10000
)

type Operator interface {
	List(ctx context.Context, opts db.ListOptions) (*metav1.List[*registryv1.BlobUpload], error)
	Get(ctx context.Context, name string) (*registryv1.BlobUpload, error)
	Create(ctx context.Context, in *registryv1.BlobUpload, publicHost string) (*registryv1.BlobUpload, error)
	Complete(ctx context.Context, name string, in *registryv1.BlobUpload) (*registryv1.BlobUpload, error)
	Delete(ctx context.Context, name string) error
}

type operator struct {
	db     *blobuploaddb.DB
	blobDB *blobdb.DB
	s3     s3.Interface
}

func NewOperator(reg *repo.Registry, s3Cli s3.Interface) Operator {
	op := &operator{s3: s3Cli}
	if reg != nil {
		op.db = blobuploaddb.NewDB(reg.Pool())
		op.blobDB = blobdb.NewDB(reg.Pool())
	}
	return op
}

func (o *operator) s3Client() s3.Interface {
	if o.s3 != nil {
		return o.s3
	}
	return s3.Default()
}

func (o *operator) List(ctx context.Context, opts db.ListOptions) (*metav1.List[*registryv1.BlobUpload], error) {
	return o.db.List(ctx, opts)
}

func (o *operator) Get(ctx context.Context, name string) (*registryv1.BlobUpload, error) {
	return o.db.Get(ctx, name)
}

func (o *operator) Create(ctx context.Context, in *registryv1.BlobUpload, publicHost string) (*registryv1.BlobUpload, error) {
	cli := o.s3Client()
	if cli == nil || !cli.IsConfigured() {
		return nil, fmt.Errorf("%w: s3 storage is not configured", models.ErrBadRequest)
	}
	cp := *in
	if cp.Metadata.Name == "" {
		cp.Metadata.Name = db.NewUID()
	}
	cp.DefaultMeta()
	if err := cp.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", models.ErrBadRequest, err)
	}
	if err := o.prepareUploadSession(ctx, cli, &cp, publicHost); err != nil {
		return nil, err
	}
	return o.db.Create(ctx, &cp)
}

func (o *operator) prepareUploadSession(ctx context.Context, cli s3.Interface, cp *registryv1.BlobUpload, publicHost string) error {
	if cp.Digest != "" {
		objKey, err := registryv1.BlobObjectKey(cp.Digest)
		if err != nil {
			return fmt.Errorf("%w: %v", models.ErrBadRequest, err)
		}
		cp.ObjectKey = objKey
	} else {
		cp.ObjectKey = "uploads/" + cp.Metadata.Name
	}
	ttl := config.Get().Registry.BlobPresignTTL
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	partSize := cp.PartSize
	if partSize <= 0 && cp.SizeBytes > defaultUploadPartSize {
		partSize = defaultUploadPartSize
	}
	if partSize > 0 && cp.SizeBytes > partSize {
		return o.presignMultipart(ctx, cli, cp, partSize, ttl, publicHost)
	}
	putURL, exp, err := cli.PresignPut(ctx, cp.ObjectKey, s3.PresignOptions{PublicHostOverride: publicHost, Expires: ttl})
	if err != nil {
		return fmt.Errorf("%w: %v", models.ErrBadGateway, err)
	}
	cp.PresignedURL = putURL
	cp.ExpiresAt = &exp
	cp.UploadState = registryv1.BlobUploadStateUploading
	return nil
}

func (o *operator) presignMultipart(ctx context.Context, cli s3.Interface, cp *registryv1.BlobUpload, partSize int64, ttl time.Duration, publicHost string) error {
	numParts := int((cp.SizeBytes + partSize - 1) / partSize)
	if numParts > maxMultipartParts {
		return fmt.Errorf("%w: multipart upload exceeds %d parts", models.ErrBadRequest, maxMultipartParts)
	}
	uploadID, err := cli.CreateMultipartUpload(ctx, cp.ObjectKey, cp.MediaType)
	if err != nil {
		return fmt.Errorf("%w: %v", models.ErrBadGateway, err)
	}
	parts := make([]registryv1.BlobUploadPart, 0, numParts)
	var latestExp time.Time
	for i := 1; i <= numParts; i++ {
		u, exp, pErr := cli.PresignUploadPart(ctx, cp.ObjectKey, uploadID, i, s3.PresignOptions{PublicHostOverride: publicHost, Expires: ttl})
		if pErr != nil {
			_ = cli.AbortMultipartUpload(ctx, cp.ObjectKey, uploadID)
			return fmt.Errorf("%w: %v", models.ErrBadGateway, pErr)
		}
		parts = append(parts, registryv1.BlobUploadPart{PartNumber: i, URL: u})
		latestExp = exp
	}
	cp.UploadID = uploadID
	cp.PartSize = partSize
	cp.Parts = parts
	cp.ExpiresAt = &latestExp
	cp.UploadState = registryv1.BlobUploadStateUploading
	return nil
}

func (o *operator) Complete(ctx context.Context, name string, in *registryv1.BlobUpload) (*registryv1.BlobUpload, error) {
	existing, err := o.db.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	if existing.UploadState == registryv1.BlobUploadStateCompleted {
		return existing, nil
	}
	cli := o.s3Client()
	if cli == nil || !cli.IsConfigured() {
		return nil, fmt.Errorf("%w: s3 storage is not configured", models.ErrBadRequest)
	}
	digestRaw := in.Digest
	if digestRaw == "" {
		digestRaw = existing.Digest
	}
	canonDigest, _, err := registryv1.NormalizeDigest(digestRaw)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", models.ErrBadRequest, err)
	}
	if err := o.finalizeS3Object(ctx, cli, existing, in.Parts, canonDigest); err != nil {
		return nil, err
	}
	targetKey, _ := registryv1.BlobObjectKey(canonDigest)
	stat, err := cli.Stat(ctx, targetKey)
	if err != nil {
		return nil, fmt.Errorf("%w: uploaded blob verification failed: %v", models.ErrBadRequest, err)
	}
	sizeBytes := stat.SizeBytes
	if sizeBytes <= 0 {
		sizeBytes = existing.SizeBytes
	}
	if _, err := o.blobDB.CreateOrGet(ctx, &registryv1.Blob{
		Digest:    canonDigest,
		SizeBytes: sizeBytes,
		MediaType: existing.MediaType,
		ObjectKey: targetKey,
	}); err != nil {
		return nil, err
	}
	existing.Digest = canonDigest
	existing.SizeBytes = sizeBytes
	existing.ObjectKey = targetKey
	existing.PresignedURL = ""
	existing.Parts = nil
	existing.UploadState = registryv1.BlobUploadStateCompleted
	return o.db.Update(ctx, existing)
}

func (o *operator) finalizeS3Object(ctx context.Context, cli s3.Interface, existing *registryv1.BlobUpload, parts []registryv1.BlobUploadPart, canonDigest string) error {
	if existing.UploadID != "" {
		completed := make([]s3.CompletedPart, 0, len(parts))
		for _, p := range parts {
			if p.PartNumber <= 0 || p.ETag == "" {
				return fmt.Errorf("%w: multipart completion requires partNumber and etag", models.ErrBadRequest)
			}
			completed = append(completed, s3.CompletedPart{PartNumber: p.PartNumber, ETag: p.ETag})
		}
		if err := cli.CompleteMultipartUpload(ctx, existing.ObjectKey, existing.UploadID, completed); err != nil {
			return fmt.Errorf("%w: complete multipart upload failed: %v", models.ErrBadGateway, err)
		}
	}
	targetKey, _ := registryv1.BlobObjectKey(canonDigest)
	if existing.ObjectKey == targetKey {
		return nil
	}
	if err := cli.Copy(ctx, existing.ObjectKey, targetKey); err != nil {
		if errors.Is(err, s3.ErrNotFound) {
			return fmt.Errorf("%w: staging object %q not found: %v", models.ErrBadRequest, existing.ObjectKey, err)
		}
		return fmt.Errorf("%w: promote staging blob failed: %v", models.ErrBadGateway, err)
	}
	_ = cli.Delete(ctx, existing.ObjectKey)
	return nil
}

func (o *operator) Delete(ctx context.Context, name string) error {
	existing, err := o.db.Get(ctx, name)
	if err != nil {
		return err
	}
	if cli := o.s3Client(); cli != nil && cli.IsConfigured() && existing.UploadState == registryv1.BlobUploadStateUploading {
		if existing.UploadID != "" && existing.ObjectKey != "" {
			_ = cli.AbortMultipartUpload(ctx, existing.ObjectKey, existing.UploadID)
		}
	}
	return o.db.Delete(ctx, name)
}
