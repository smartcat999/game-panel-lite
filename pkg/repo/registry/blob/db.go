package blob

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

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

func toRecord(in *registryv1.Blob, rv int64) registryv1.BlobRecord {
	return registryv1.BlobRecord{
		Name: in.Metadata.Name, UID: in.Metadata.UID,
		Digest: in.Digest, SizeBytes: in.SizeBytes,
		MediaType: in.MediaType, ObjectKey: in.ObjectKey, RefCount: in.RefCount,
		ResourceVersion: rv, Generation: in.Metadata.Generation,
		CreatedAt: in.Metadata.CreationTimestamp, DeletedAt: in.Metadata.DeletionTimestamp,
		Finalizers: pgutil.NonNilStrings(in.Metadata.Finalizers),
		Labels:     pgutil.NonNilMap(in.Metadata.Labels), Annotations: pgutil.NonNilMap(in.Metadata.Annotations),
	}
}

func fromRecord(rec registryv1.BlobRecord) *registryv1.Blob {
	out := &registryv1.Blob{
		TypeMeta: metav1.TypeMeta{APIVersion: registryv1.GroupVersion, Kind: registryv1.KindBlob},
		Metadata: metav1.ObjectMeta{
			Name: rec.Name, UID: rec.UID, ResourceVersion: pgutil.FormatVersion(rec.ResourceVersion),
			Generation: rec.Generation, CreationTimestamp: rec.CreatedAt, DeletionTimestamp: rec.DeletedAt,
		},
		Digest: rec.Digest, SizeBytes: rec.SizeBytes,
		MediaType: rec.MediaType, ObjectKey: rec.ObjectKey, RefCount: rec.RefCount,
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

func (s *DB) CreateOrGet(ctx context.Context, in *registryv1.Blob) (*registryv1.Blob, error) {
	if s == nil || s.orm == nil {
		return nil, errors.New("db is nil")
	}
	cp := *in
	cp.DefaultMeta()
	if err := cp.Validate(); err != nil {
		return nil, err
	}
	pgutil.InitCreateMeta(&cp.Metadata)
	rec := toRecord(&cp, 1)
	res := s.orm.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "digest"}}, DoNothing: true}).Create(&rec)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return s.Get(ctx, cp.Digest)
	}
	return fromRecord(rec), nil
}

func (s *DB) Get(ctx context.Context, rawDigest string) (*registryv1.Blob, error) {
	if s == nil || s.orm == nil {
		return nil, errors.New("db is nil")
	}
	digest, _, err := registryv1.NormalizeDigest(rawDigest)
	if err != nil {
		return nil, db.ErrNotFound
	}
	var rec registryv1.BlobRecord
	if err := s.orm.WithContext(ctx).Where("digest = ?", digest).Take(&rec).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, db.ErrNotFound
		}
		return nil, err
	}
	return fromRecord(rec), nil
}

func (s *DB) List(ctx context.Context, opts db.ListOptions) (*metav1.List[*registryv1.Blob], error) {
	if s == nil || s.orm == nil {
		return nil, errors.New("db is nil")
	}
	tx := s.orm.WithContext(ctx).Model(&registryv1.BlobRecord{})
	for _, field := range pgutil.SortedFieldKeys(opts.FieldSelector) {
		val := opts.FieldSelector[field]
		switch field {
		case "metadata.name", "digest":
			tx = tx.Where("digest = ?", val)
		case "refCount":
			tx = tx.Where("ref_count = ?", val)
		default:
			return nil, fmt.Errorf("unsupported blob fieldSelector %q", field)
		}
	}
	tx = db.ApplyLabelSelector(tx, opts.LabelSelector)
	rows, total, err := pgutil.Paginate[registryv1.BlobRecord](tx, "created_at DESC, name ASC", opts)
	if err != nil {
		return nil, err
	}
	items := make([]*registryv1.Blob, 0, len(rows))
	for _, row := range rows {
		items = append(items, fromRecord(row))
	}
	return &metav1.List[*registryv1.Blob]{
		TypeMeta: metav1.TypeMeta{APIVersion: registryv1.GroupVersion, Kind: "BlobList"},
		Metadata: metav1.ListMeta{TotalCount: total},
		Items:    items,
	}, nil
}

// AcquireRef idempotently records a reference from holderUID to the blob digest and increments ref_count once.
func (s *DB) AcquireRef(ctx context.Context, rawDigest, holderUID string) (*registryv1.Blob, error) {
	if s == nil || s.orm == nil {
		return nil, errors.New("db is nil")
	}
	digest, _, err := registryv1.NormalizeDigest(rawDigest)
	if err != nil {
		return nil, err
	}
	holderUID = strings.TrimSpace(holderUID)
	if holderUID == "" {
		return nil, errors.New("holder UID is required")
	}
	tx := s.orm.WithContext(ctx).Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}
	defer tx.Rollback()
	if err := tx.Where("digest = ?", digest).Take(&registryv1.BlobRecord{}).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, db.ErrNotFound
		}
		return nil, err
	}
	ref := registryv1.BlobRefRecord{Digest: digest, HolderUID: holderUID, CreatedAt: time.Now().UTC()}
	ins := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "digest"}, {Name: "holder_uid"}}, DoNothing: true}).Create(&ref)
	if ins.Error != nil {
		return nil, ins.Error
	}
	if ins.RowsAffected == 1 {
		upd := tx.Exec(`UPDATE registry_blobs SET ref_count = ref_count + 1, resource_version = resource_version + 1 WHERE digest = ?`, digest)
		if upd.Error != nil {
			return nil, upd.Error
		}
		if upd.RowsAffected == 0 {
			return nil, db.ErrNotFound
		}
	}
	if err := tx.Commit().Error; err != nil {
		return nil, err
	}
	return s.Get(ctx, digest)
}

// ReleaseRef idempotently removes holderUID's reference to the blob digest and decrements ref_count once.
func (s *DB) ReleaseRef(ctx context.Context, rawDigest, holderUID string) (*registryv1.Blob, error) {
	if s == nil || s.orm == nil {
		return nil, errors.New("db is nil")
	}
	digest, _, err := registryv1.NormalizeDigest(rawDigest)
	if err != nil {
		return nil, err
	}
	holderUID = strings.TrimSpace(holderUID)
	if holderUID == "" {
		return nil, errors.New("holder UID is required")
	}
	tx := s.orm.WithContext(ctx).Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}
	defer tx.Rollback()
	del := tx.Exec(`DELETE FROM registry_blob_refs WHERE digest = ? AND holder_uid = ?`, digest, holderUID)
	if del.Error != nil {
		return nil, del.Error
	}
	if del.RowsAffected == 1 {
		upd := tx.Exec(`UPDATE registry_blobs SET ref_count = ref_count - 1, resource_version = resource_version + 1 WHERE digest = ? AND ref_count > 0`, digest)
		if upd.Error != nil {
			return nil, upd.Error
		}
		if upd.RowsAffected == 0 {
			return nil, db.ErrConflict
		}
	}
	if err := tx.Commit().Error; err != nil {
		return nil, err
	}
	return s.Get(ctx, digest)
}

func (s *DB) IncrementRef(ctx context.Context, rawDigest string) (*registryv1.Blob, error) {
	if s == nil || s.orm == nil {
		return nil, errors.New("db is nil")
	}
	digest, _, err := registryv1.NormalizeDigest(rawDigest)
	if err != nil {
		return nil, err
	}
	res := s.orm.WithContext(ctx).Exec(
		`UPDATE registry_blobs SET ref_count = ref_count + 1, resource_version = resource_version + 1 WHERE digest = ?`,
		digest,
	)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, db.ErrNotFound
	}
	return s.Get(ctx, digest)
}

func (s *DB) DecrementRef(ctx context.Context, rawDigest string) (*registryv1.Blob, error) {
	if s == nil || s.orm == nil {
		return nil, errors.New("db is nil")
	}
	digest, _, err := registryv1.NormalizeDigest(rawDigest)
	if err != nil {
		return nil, err
	}
	res := s.orm.WithContext(ctx).Exec(
		`UPDATE registry_blobs SET ref_count = ref_count - 1, resource_version = resource_version + 1 WHERE digest = ? AND ref_count > 0`,
		digest,
	)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		if _, gErr := s.Get(ctx, digest); gErr != nil {
			return nil, gErr
		}
		return nil, db.ErrConflict
	}
	return s.Get(ctx, digest)
}

func (s *DB) DeleteUnreferenced(ctx context.Context, rawDigest string) error {
	if s == nil || s.orm == nil {
		return errors.New("db is nil")
	}
	digest, _, err := registryv1.NormalizeDigest(rawDigest)
	if err != nil {
		return db.ErrNotFound
	}
	res := s.orm.WithContext(ctx).Exec(`DELETE FROM registry_blobs WHERE digest = ? AND ref_count = 0`, digest)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		if _, gErr := s.Get(ctx, digest); gErr != nil {
			return gErr
		}
		return db.ErrConflict
	}
	return nil
}
