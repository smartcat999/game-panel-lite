package pkg

import (
	"context"
	"errors"
	"fmt"
	"strings"

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

func nonNilDeps(in []registryv1.PackageDependency) []registryv1.PackageDependency {
	if len(in) == 0 {
		return []registryv1.PackageDependency{}
	}
	return append([]registryv1.PackageDependency(nil), in...)
}

func nonNilConfigSchema(in []registryv1.AddonConfigField) []registryv1.AddonConfigField {
	if len(in) == 0 {
		return []registryv1.AddonConfigField{}
	}
	return append([]registryv1.AddonConfigField(nil), in...)
}

func toRecord(in *registryv1.Package, rv int64) registryv1.PackageRecord {
	return registryv1.PackageRecord{
		Name: in.Metadata.Name, UID: in.Metadata.UID,
		Repository: in.Repository, Package: in.Package,
		Title: in.Title, Summary: in.Summary, Description: in.Description,
		Author: in.Author, IconURL: in.IconURL,
		GalleryURLs:     pgutil.NonNilStrings(in.GalleryURLs),
		SourceURL:       in.SourceURL,
		Downloads:       in.Downloads,
		SizeBytes:       in.SizeBytes,
		LatestVersion:   in.LatestVersion,
		UpdatedAtStr:    in.UpdatedAt,
		Tags:            pgutil.NonNilStrings(in.Tags),
		ModIDs:          pgutil.NonNilStrings(in.ModIDs),
		MapFolders:      pgutil.NonNilStrings(in.MapFolders),
		Dependencies:    nonNilDeps(in.Dependencies),
		ConfigSchema:    nonNilConfigSchema(in.ConfigSchema),
		ResourceVersion: rv, Generation: in.Metadata.Generation,
		CreatedAt: in.Metadata.CreationTimestamp, DeletedAt: in.Metadata.DeletionTimestamp,
		Finalizers: pgutil.NonNilStrings(in.Metadata.Finalizers),
		Labels:     pgutil.NonNilMap(in.Metadata.Labels), Annotations: pgutil.NonNilMap(in.Metadata.Annotations),
	}
}

func fromRecord(rec registryv1.PackageRecord) *registryv1.Package {
	out := &registryv1.Package{
		TypeMeta: metav1.TypeMeta{APIVersion: registryv1.GroupVersion, Kind: registryv1.KindPackage},
		Metadata: metav1.ObjectMeta{
			Name: rec.Name, UID: rec.UID, ResourceVersion: pgutil.FormatVersion(rec.ResourceVersion),
			Generation: rec.Generation, CreationTimestamp: rec.CreatedAt, DeletionTimestamp: rec.DeletedAt,
		},
		Repository: rec.Repository, Package: rec.Package, Title: rec.Title,
		Summary: rec.Summary, Description: rec.Description, Author: rec.Author,
		IconURL: rec.IconURL, SourceURL: rec.SourceURL,
		Downloads: rec.Downloads, SizeBytes: rec.SizeBytes,
		LatestVersion: rec.LatestVersion, UpdatedAt: rec.UpdatedAtStr,
	}
	copyPackageSlices(out, rec)
	return out
}

func copyPackageSlices(out *registryv1.Package, rec registryv1.PackageRecord) {
	if len(rec.GalleryURLs) > 0 {
		out.GalleryURLs = append([]string(nil), rec.GalleryURLs...)
	}
	if len(rec.Tags) > 0 {
		out.Tags = append([]string(nil), rec.Tags...)
	}
	if len(rec.ModIDs) > 0 {
		out.ModIDs = append([]string(nil), rec.ModIDs...)
	}
	if len(rec.MapFolders) > 0 {
		out.MapFolders = append([]string(nil), rec.MapFolders...)
	}
	if len(rec.Dependencies) > 0 {
		out.Dependencies = append([]registryv1.PackageDependency(nil), rec.Dependencies...)
	}
	if len(rec.ConfigSchema) > 0 {
		out.ConfigSchema = append([]registryv1.AddonConfigField(nil), rec.ConfigSchema...)
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
}

func (s *DB) Create(ctx context.Context, in *registryv1.Package) (*registryv1.Package, error) {
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
	res := s.orm.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "name"}}, DoNothing: true}).Create(&rec)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, db.ErrAlreadyExists
	}
	return fromRecord(rec), nil
}

func (s *DB) Get(ctx context.Context, name string) (*registryv1.Package, error) {
	if s == nil || s.orm == nil {
		return nil, errors.New("db is nil")
	}
	var rec registryv1.PackageRecord
	if err := s.orm.WithContext(ctx).Where("name = ?", name).Take(&rec).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, db.ErrNotFound
		}
		return nil, err
	}
	return fromRecord(rec), nil
}

func (s *DB) List(ctx context.Context, opts db.ListOptions) (*metav1.List[*registryv1.Package], error) {
	if s == nil || s.orm == nil {
		return nil, errors.New("db is nil")
	}
	tx := s.orm.WithContext(ctx).Model(&registryv1.PackageRecord{})
	for _, field := range pgutil.SortedFieldKeys(opts.FieldSelector) {
		val := opts.FieldSelector[field]
		switch field {
		case "metadata.name":
			tx = tx.Where("name = ?", val)
		case "metadata.name.in":
			tx = db.ApplyNameInWhere(tx, "name", val)
		case "repository":
			tx = tx.Where("repository = ?", val)
		case "package":
			tx = tx.Where("package = ?", val)
		case "q":
			q := "%" + strings.TrimSpace(val) + "%"
			tx = tx.Where("(title ILIKE ? OR package ILIKE ? OR summary ILIKE ?)", q, q, q)
		default:
			return nil, fmt.Errorf("unsupported package fieldSelector %q", field)
		}
	}
	tx = db.ApplyLabelSelector(tx, opts.LabelSelector)
	rows, total, err := pgutil.Paginate[registryv1.PackageRecord](tx, "downloads DESC, created_at DESC, name ASC", opts)
	if err != nil {
		return nil, err
	}
	items := make([]*registryv1.Package, 0, len(rows))
	for _, row := range rows {
		items = append(items, fromRecord(row))
	}
	return &metav1.List[*registryv1.Package]{
		TypeMeta: metav1.TypeMeta{APIVersion: registryv1.GroupVersion, Kind: "PackageList"},
		Metadata: metav1.ListMeta{
			Continue:   pgutil.NextContinue(opts.Offset, len(items), total),
			TotalCount: total,
		},
		Items: items,
	}, nil
}

func (s *DB) Update(ctx context.Context, in *registryv1.Package) (*registryv1.Package, error) {
	return s.updatePackage(ctx, in, true)
}

// HydrateMetadata refreshes cached upstream metadata fields without advancing desired generation.
func (s *DB) HydrateMetadata(ctx context.Context, in *registryv1.Package) (*registryv1.Package, error) {
	return s.updatePackage(ctx, in, false)
}

func (s *DB) updatePackage(ctx context.Context, in *registryv1.Package, bumpGeneration bool) (*registryv1.Package, error) {
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
	if (cp.Metadata.UID != "" && cp.Metadata.UID != existing.Metadata.UID) ||
		(cp.Metadata.ResourceVersion != "" && cp.Metadata.ResourceVersion != existing.Metadata.ResourceVersion) {
		return nil, db.ErrConflict
	}
	oldRV, _ := pgutil.ParseVersion(existing.Metadata.ResourceVersion)
	newRV := oldRV + 1
	cp.Metadata.UID = existing.Metadata.UID
	cp.Metadata.CreationTimestamp = existing.Metadata.CreationTimestamp
	cp.Metadata.Generation = existing.Metadata.Generation
	if bumpGeneration {
		cp.Metadata.Generation++
	}
	return s.commitUpdate(ctx, &cp, oldRV, newRV)
}

func (s *DB) commitUpdate(ctx context.Context, cp *registryv1.Package, oldRV, newRV int64) (*registryv1.Package, error) {
	rec := toRecord(cp, newRV)
	galJSON, _ := pgutil.MarshalJSONCol(rec.GalleryURLs)
	tagsJSON, _ := pgutil.MarshalJSONCol(rec.Tags)
	modsJSON, _ := pgutil.MarshalJSONCol(rec.ModIDs)
	mapsJSON, _ := pgutil.MarshalJSONCol(rec.MapFolders)
	depsJSON, _ := pgutil.MarshalJSONCol(rec.Dependencies)
	schemaJSON, _ := pgutil.MarshalJSONCol(rec.ConfigSchema)
	finJSON, _ := pgutil.MarshalJSONCol(rec.Finalizers)
	lblJSON, _ := pgutil.MarshalJSONCol(rec.Labels)
	annJSON, _ := pgutil.MarshalJSONCol(rec.Annotations)
	tx := s.orm.WithContext(ctx).Model(&registryv1.PackageRecord{}).
		Where("name = ? AND uid = ? AND resource_version = ?", rec.Name, rec.UID, oldRV)
	res := tx.Updates(map[string]any{
		"title": rec.Title, "summary": rec.Summary, "description": rec.Description,
		"author": rec.Author, "icon_url": rec.IconURL, "gallery_urls": galJSON,
		"source_url": rec.SourceURL, "downloads": rec.Downloads, "size_bytes": rec.SizeBytes,
		"latest_version": rec.LatestVersion, "updated_at_str": rec.UpdatedAtStr,
		"tags": tagsJSON, "mod_ids": modsJSON, "map_folders": mapsJSON, "dependencies": depsJSON,
		"config_schema":    schemaJSON,
		"resource_version": newRV, "generation": rec.Generation, "deleted_at": rec.DeletedAt,
		"finalizers": finJSON, "labels": lblJSON, "annotations": annJSON,
	})
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, db.ErrConflict
	}
	return s.Get(ctx, rec.Name)
}

func (s *DB) Delete(ctx context.Context, name string) error {
	if s == nil {
		return errors.New("db is nil")
	}
	return pgutil.DeleteByCondition(ctx, s.orm, "registry_packages", "name = ?", []any{name}, "")
}

func (s *DB) CountByRepository(ctx context.Context, repository string) (int64, error) {
	if s == nil || s.orm == nil {
		return 0, errors.New("db is nil")
	}
	var cnt int64
	err := s.orm.WithContext(ctx).Model(&registryv1.PackageRecord{}).Where("repository = ?", repository).Count(&cnt).Error
	return cnt, err
}
