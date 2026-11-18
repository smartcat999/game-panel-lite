package blobupload

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/repo/pgutil"
)

type DB struct{ orm *gorm.DB }

func NewDB(pool *pgxpool.Pool) *DB {
	if pool == nil {
		return nil
	}
	return &DB{orm: db.OpenGORM(pool)}
}

func nonNilParts(in []registryv1.BlobUploadPart) []registryv1.BlobUploadPart {
	if len(in) == 0 {
		return []registryv1.BlobUploadPart{}
	}
	return append([]registryv1.BlobUploadPart(nil), in...)
}

func toRecord(in *registryv1.BlobUpload, rv int64) registryv1.BlobUploadRecord {
	return registryv1.BlobUploadRecord{
		Name: in.Metadata.Name, UID: in.Metadata.UID,
		Digest: in.Digest, SizeBytes: in.SizeBytes,
		MediaType: in.MediaType, FileName: in.FileName,
		ObjectKey: in.ObjectKey, UploadID: in.UploadID, PartSize: in.PartSize,
		Parts: nonNilParts(in.Parts), PresignedURL: in.PresignedURL,
		ExpiresAt: in.ExpiresAt, UploadState: string(in.UploadState),
		ResourceVersion: rv, Generation: in.Metadata.Generation,
		CreatedAt: in.Metadata.CreationTimestamp, DeletedAt: in.Metadata.DeletionTimestamp,
		Finalizers: pgutil.NonNilStrings(in.Metadata.Finalizers),
		Labels:     pgutil.NonNilMap(in.Metadata.Labels), Annotations: pgutil.NonNilMap(in.Metadata.Annotations),
	}
}

func fromRecord(rec registryv1.BlobUploadRecord) *registryv1.BlobUpload {
	out := &registryv1.BlobUpload{
		TypeMeta: metav1.TypeMeta{APIVersion: registryv1.GroupVersion, Kind: registryv1.KindBlobUpload},
		Metadata: metav1.ObjectMeta{
			Name: rec.Name, UID: rec.UID, ResourceVersion: pgutil.FormatVersion(rec.ResourceVersion),
			Generation: rec.Generation, CreationTimestamp: rec.CreatedAt, DeletionTimestamp: rec.DeletedAt,
		},
		Digest: rec.Digest, SizeBytes: rec.SizeBytes, MediaType: rec.MediaType,
		FileName: rec.FileName, ObjectKey: rec.ObjectKey, UploadID: rec.UploadID,
		PartSize: rec.PartSize, PresignedURL: rec.PresignedURL,
		ExpiresAt: rec.ExpiresAt, UploadState: registryv1.BlobUploadState(rec.UploadState),
	}
	if len(rec.Parts) > 0 {
		out.Parts = append([]registryv1.BlobUploadPart(nil), rec.Parts...)
	}
	if len(rec.Finalizers) > 0 {
		out.Metadata.Finalizers = append([]string(nil), rec.Finalizers...)
	}
	if len(rec.Labels) > 0 {
		out.Metadata.Labels = pgutil.NonNilMap(rec.Labels)
	}
	if len(rec.Annotations) > 0 {
		out.Metadata.Annotations = pgutil.NonNilMap(rec.Annotations)
	}
	return out
}

func (s *DB) Create(ctx context.Context, in *registryv1.BlobUpload) (*registryv1.BlobUpload, error) {
	if s == nil || s.orm == nil {
		return nil, errors.New("db is nil")
	}
	cp := *in
	if cp.Metadata.Name == "" {
		cp.Metadata.Name = db.NewUID()
	}
	cp.DefaultMeta()
	if err := cp.Validate(); err != nil {
		return nil, err
	}
	pgutil.InitCreateMeta(&cp.Metadata)
	rec := toRecord(&cp, 1)
	res := s.orm.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "name"}}, DoNothing: true}).Create(&rec)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, db.ErrAlreadyExists
	}
	return fromRecord(rec), nil
}

func (s *DB) Get(ctx context.Context, name string) (*registryv1.BlobUpload, error) {
	if s == nil || s.orm == nil {
		return nil, errors.New("db is nil")
	}
	var rec registryv1.BlobUploadRecord
	if err := s.orm.WithContext(ctx).Where("name = ?", name).Take(&rec).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, db.ErrNotFound
		}
		return nil, err
	}
	return fromRecord(rec), nil
}

func (s *DB) List(ctx context.Context, opts db.ListOptions) (*metav1.List[*registryv1.BlobUpload], error) {
	if s == nil || s.orm == nil {
		return nil, errors.New("db is nil")
	}
	tx := s.orm.WithContext(ctx).Model(&registryv1.BlobUploadRecord{})
	for _, field := range pgutil.SortedFieldKeys(opts.FieldSelector) {
		val := opts.FieldSelector[field]
		switch field {
		case "metadata.name":
			tx = tx.Where("name = ?", val)
		case "uploadState":
			tx = tx.Where("upload_state = ?", val)
		default:
			return nil, fmt.Errorf("unsupported blobUpload fieldSelector %q", field)
		}
	}
	tx = db.ApplyLabelSelector(tx, opts.LabelSelector)
	rows, total, err := pgutil.Paginate[registryv1.BlobUploadRecord](tx, "created_at DESC, name ASC", opts)
	if err != nil {
		return nil, err
	}
	items := make([]*registryv1.BlobUpload, 0, len(rows))
	for _, row := range rows {
		items = append(items, fromRecord(row))
	}
	return &metav1.List[*registryv1.BlobUpload]{
		TypeMeta: metav1.TypeMeta{APIVersion: registryv1.GroupVersion, Kind: "BlobUploadList"},
		Metadata: metav1.ListMeta{TotalCount: total},
		Items:    items,
	}, nil
}

func (s *DB) Update(ctx context.Context, in *registryv1.BlobUpload) (*registryv1.BlobUpload, error) {
	if s == nil || s.orm == nil {
		return nil, errors.New("db is nil")
	}
	cp := *in
	cp.DefaultMeta()
	if err := cp.Validate(); err != nil {
		return nil, err
	}
	existing, err := s.Get(ctx, cp.Metadata.Name)
	if err != nil {
		return nil, err
	}
	if cp.Metadata.ResourceVersion != "" && cp.Metadata.ResourceVersion != existing.Metadata.ResourceVersion {
		return nil, db.ErrConflict
	}
	oldRV, _ := pgutil.ParseVersion(existing.Metadata.ResourceVersion)
	newRV := oldRV + 1
	partsJSON, _ := pgutil.MarshalJSONCol(nonNilParts(cp.Parts))
	tx := s.orm.WithContext(ctx).Model(&registryv1.BlobUploadRecord{}).
		Where("name = ? AND uid = ? AND resource_version = ?", existing.Metadata.Name, existing.Metadata.UID, oldRV)
	res := tx.Updates(map[string]any{
		"digest": cp.Digest, "size_bytes": cp.SizeBytes, "media_type": cp.MediaType,
		"file_name": cp.FileName, "object_key": cp.ObjectKey, "upload_id": cp.UploadID,
		"part_size": cp.PartSize, "parts": partsJSON, "presigned_url": cp.PresignedURL,
		"expires_at": cp.ExpiresAt, "upload_state": string(cp.UploadState),
		"resource_version": newRV, "generation": existing.Metadata.Generation + 1,
	})
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, db.ErrConflict
	}
	return s.Get(ctx, existing.Metadata.Name)
}

func (s *DB) Delete(ctx context.Context, name string) error {
	if s == nil {
		return errors.New("db is nil")
	}
	return pgutil.DeleteByCondition(ctx, s.orm, "registry_blob_uploads", "name = ?", []any{name}, "")
}
