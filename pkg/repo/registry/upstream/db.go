package upstream

import (
	"context"
	"errors"
	"fmt"
	"reflect"

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

func toRecord(in *registryv1.Upstream, rv int64) registryv1.UpstreamRecord {
	return registryv1.UpstreamRecord{
		Name:                in.Metadata.Name,
		UID:                 in.Metadata.UID,
		Provider:            in.Provider,
		Endpoint:            in.Endpoint,
		ProxyURL:            in.ProxyURL,
		SteamAppID:          in.SteamAppID,
		ModrinthLoader:      in.ModrinthLoader,
		ModrinthProjectType: in.ModrinthProjectType,
		APIKey:              in.APIKey,
		RateLimitRPS:        in.RateLimitRPS,
		ResourceVersion:     rv,
		Generation:          in.Metadata.Generation,
		CreatedAt:           in.Metadata.CreationTimestamp,
		DeletedAt:           in.Metadata.DeletionTimestamp,
		Finalizers:          pgutil.NonNilStrings(in.Metadata.Finalizers),
		Labels:              pgutil.NonNilMap(in.Metadata.Labels),
		Annotations:         pgutil.NonNilMap(in.Metadata.Annotations),
	}
}

func fromRecord(rec registryv1.UpstreamRecord) *registryv1.Upstream {
	out := &registryv1.Upstream{
		TypeMeta: metav1.TypeMeta{APIVersion: registryv1.GroupVersion, Kind: registryv1.KindUpstream},
		Metadata: metav1.ObjectMeta{
			Name:              rec.Name,
			UID:               rec.UID,
			ResourceVersion:   pgutil.FormatVersion(rec.ResourceVersion),
			Generation:        rec.Generation,
			CreationTimestamp: rec.CreatedAt,
			DeletionTimestamp: rec.DeletedAt,
		},
		Provider:            rec.Provider,
		Endpoint:            rec.Endpoint,
		ProxyURL:            rec.ProxyURL,
		SteamAppID:          rec.SteamAppID,
		ModrinthLoader:      rec.ModrinthLoader,
		ModrinthProjectType: rec.ModrinthProjectType,
		APIKey:              rec.APIKey,
		RateLimitRPS:        rec.RateLimitRPS,
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

func (s *DB) Create(ctx context.Context, in *registryv1.Upstream) (*registryv1.Upstream, error) {
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

func (s *DB) Get(ctx context.Context, name string) (*registryv1.Upstream, error) {
	if s == nil || s.orm == nil {
		return nil, errors.New("db is nil")
	}
	var rec registryv1.UpstreamRecord
	if err := s.orm.WithContext(ctx).Where("name = ?", name).Take(&rec).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, db.ErrNotFound
		}
		return nil, err
	}
	return fromRecord(rec), nil
}

func (s *DB) List(ctx context.Context, opts db.ListOptions) (*metav1.List[*registryv1.Upstream], error) {
	if s == nil || s.orm == nil {
		return nil, errors.New("db is nil")
	}
	tx := s.orm.WithContext(ctx).Model(&registryv1.UpstreamRecord{})
	for _, field := range pgutil.SortedFieldKeys(opts.FieldSelector) {
		val := opts.FieldSelector[field]
		switch field {
		case "metadata.name":
			tx = tx.Where("name = ?", val)
		case "metadata.name.in":
			tx = db.ApplyNameInWhere(tx, "name", val)
		case "provider":
			tx = tx.Where("provider = ?", val)
		default:
			return nil, fmt.Errorf("unsupported upstream fieldSelector %q", field)
		}
	}
	tx = db.ApplyLabelSelector(tx, opts.LabelSelector)
	rows, total, err := pgutil.Paginate[registryv1.UpstreamRecord](tx, "created_at DESC, name ASC", opts)
	if err != nil {
		return nil, err
	}
	items := make([]*registryv1.Upstream, 0, len(rows))
	for _, row := range rows {
		items = append(items, fromRecord(row))
	}
	return &metav1.List[*registryv1.Upstream]{
		TypeMeta: metav1.TypeMeta{APIVersion: registryv1.GroupVersion, Kind: "UpstreamList"},
		Metadata: metav1.ListMeta{TotalCount: total},
		Items:    items,
	}, nil
}

func (s *DB) Update(ctx context.Context, in *registryv1.Upstream) (*registryv1.Upstream, error) {
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
	if !sameUpstreamBody(existing, &cp) {
		cp.Metadata.Generation++
	}
	return s.commitUpdate(ctx, &cp, oldRV, newRV)
}

func sameUpstreamBody(a, b *registryv1.Upstream) bool {
	return a.Provider == b.Provider && a.Endpoint == b.Endpoint && a.ProxyURL == b.ProxyURL &&
		a.SteamAppID == b.SteamAppID && a.ModrinthLoader == b.ModrinthLoader &&
		a.ModrinthProjectType == b.ModrinthProjectType && a.APIKey == b.APIKey &&
		a.RateLimitRPS == b.RateLimitRPS && reflect.DeepEqual(a.Metadata.Finalizers, b.Metadata.Finalizers)
}

func (s *DB) commitUpdate(ctx context.Context, cp *registryv1.Upstream, oldRV, newRV int64) (*registryv1.Upstream, error) {
	rec := toRecord(cp, newRV)
	finJSON, _ := pgutil.MarshalJSONCol(rec.Finalizers)
	lblJSON, _ := pgutil.MarshalJSONCol(rec.Labels)
	annJSON, _ := pgutil.MarshalJSONCol(rec.Annotations)
	tx := s.orm.WithContext(ctx).Model(&registryv1.UpstreamRecord{}).
		Where("name = ? AND uid = ? AND resource_version = ?", rec.Name, rec.UID, oldRV)
	res := tx.Updates(map[string]any{
		"provider": rec.Provider, "endpoint": rec.Endpoint, "proxy_url": rec.ProxyURL,
		"steam_app_id": rec.SteamAppID, "modrinth_loader": rec.ModrinthLoader,
		"modrinth_project_type": rec.ModrinthProjectType, "api_key": rec.APIKey,
		"rate_limit_rps": rec.RateLimitRPS, "resource_version": newRV, "generation": rec.Generation,
		"deleted_at": rec.DeletedAt, "finalizers": finJSON, "labels": lblJSON, "annotations": annJSON,
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
	return pgutil.DeleteByCondition(ctx, s.orm, "registry_upstreams", "name = ?", []any{name}, "")
}
