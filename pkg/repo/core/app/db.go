package app

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/repo/pgutil"
)

// DB manages persistence for App marketplace templates (apps).
type DB struct{ orm *gorm.DB }

func NewDB(pool *pgxpool.Pool) *DB {
	if pool == nil {
		return nil
	}
	return &DB{orm: db.OpenGORM(pool)}
}

func nonNilSlice[T any](in []T) []T {
	if len(in) == 0 {
		return []T{}
	}
	return append([]T(nil), in...)
}

func nonNilResources(in corev1.ResourceList) corev1.ResourceList {
	if in == nil {
		return corev1.ResourceList{}
	}
	cp := make(corev1.ResourceList, len(in))
	for k, v := range in {
		cp[k] = v
	}
	return cp
}

func toAppRecord(in *corev1.App, rv int64) corev1.AppRecord {
	return corev1.AppRecord{
		Name: in.Metadata.Name, UID: in.Metadata.UID, DisplayName: in.DisplayName,
		Tagline: in.Tagline, Summary: in.Summary, Category: in.Category, CoverImage: in.CoverImage,
		Image: in.Image, ImageTemplate: in.ImageTemplate, Versions: pgutil.NonNilStrings(in.Versions),
		RecommendedVersion: in.RecommendedVersion, ServerBinding: in.ServerBinding, AddonBinding: in.AddonBinding,
		RuntimeClassName: in.RuntimeClassName, PreStartCommand: in.PreStartCommand, StartupCommand: in.StartupCommand, PreStopCommand: in.PreStopCommand,
		Command: pgutil.NonNilStrings(in.Command), WorkingDir: in.WorkingDir, Ports: nonNilSlice(in.Ports),
		Env: pgutil.NonNilMap(in.Env), RecommendedPlan: in.RecommendedPlan, Resources: nonNilResources(in.Resources),
		ConfigSchema: nonNilSlice(in.ConfigSchema), ConfigPresets: nonNilSlice(in.ConfigPresets),
		FileTemplates: nonNilSlice(in.FileTemplates), ConsoleDelivery: in.ConsoleDelivery,
		ConsoleTargets: nonNilSlice(in.ConsoleTargets), ConsoleActions: nonNilSlice(in.ConsoleActions),
		StoragePolicy: in.StoragePolicy, Metrics: nonNilSlice(in.Metrics),
		Capabilities: pgutil.NonNilStrings(in.Capabilities), Published: in.Published, BuiltIn: in.BuiltIn,
		ResourceVersion: rv, Generation: in.Metadata.Generation, CreatedAt: in.Metadata.CreationTimestamp,
		DeletedAt: in.Metadata.DeletionTimestamp, Finalizers: pgutil.NonNilStrings(in.Metadata.Finalizers),
		Labels: pgutil.NonNilMap(in.Metadata.Labels), Annotations: pgutil.NonNilMap(in.Metadata.Annotations),
	}
}

func fromAppRecord(rec corev1.AppRecord) *corev1.App {
	out := &corev1.App{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.GroupVersion, Kind: "App"},
		Metadata: metav1.ObjectMeta{
			Name: rec.Name, UID: rec.UID, ResourceVersion: pgutil.FormatVersion(rec.ResourceVersion),
			Generation: rec.Generation, CreationTimestamp: rec.CreatedAt, DeletionTimestamp: rec.DeletedAt,
		},
		DisplayName: rec.DisplayName, Tagline: rec.Tagline, Summary: rec.Summary,
		Category: rec.Category, CoverImage: rec.CoverImage, Image: rec.Image, ImageTemplate: rec.ImageTemplate,
		Versions: append([]string(nil), rec.Versions...), RecommendedVersion: rec.RecommendedVersion,
		ServerBinding: rec.ServerBinding, AddonBinding: rec.AddonBinding, RuntimeClassName: rec.RuntimeClassName,
		PreStartCommand: rec.PreStartCommand, StartupCommand: rec.StartupCommand, PreStopCommand: rec.PreStopCommand,
		Command: append([]string(nil), rec.Command...), WorkingDir: rec.WorkingDir,
		Ports: append([]corev1.InstancePort(nil), rec.Ports...), RecommendedPlan: rec.RecommendedPlan,
		ConfigSchema:    append([]corev1.AppConfigField(nil), rec.ConfigSchema...),
		ConfigPresets:   append([]corev1.AppConfigPreset(nil), rec.ConfigPresets...),
		FileTemplates:   append([]corev1.AppFileTemplate(nil), rec.FileTemplates...),
		ConsoleDelivery: rec.ConsoleDelivery, ConsoleTargets: append([]corev1.AppConsoleTarget(nil), rec.ConsoleTargets...),
		ConsoleActions: append([]corev1.AppConsoleAction(nil), rec.ConsoleActions...),
		StoragePolicy:  rec.StoragePolicy,
		Metrics:        append([]corev1.AppMetricSpec(nil), rec.Metrics...),
		Capabilities:   append([]string(nil), rec.Capabilities...), Published: rec.Published, BuiltIn: rec.BuiltIn,
	}
	hydrateAppOptionalFields(out, rec)
	return out
}

func hydrateAppOptionalFields(out *corev1.App, rec corev1.AppRecord) {
	if len(rec.Env) > 0 {
		out.Env = pgutil.NonNilMap(rec.Env)
	}
	if len(rec.Resources) > 0 {
		out.Resources = nonNilResources(rec.Resources)
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

func (s *DB) Create(ctx context.Context, in *corev1.App) (*corev1.App, error) {
	if s == nil || s.orm == nil {
		return nil, errors.New("db is nil")
	}
	cp := *in
	cp.DefaultMeta()
	if err := cp.Validate(); err != nil {
		return nil, err
	}
	pgutil.InitCreateMeta(&cp.Metadata)
	rec := toAppRecord(&cp, 1)
	res := s.orm.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "name"}}, DoNothing: true}).Create(&rec)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, db.ErrAlreadyExists
	}
	return fromAppRecord(rec), nil
}

func (s *DB) Get(ctx context.Context, name string) (*corev1.App, error) {
	if s == nil || s.orm == nil {
		return nil, errors.New("db is nil")
	}
	var rec corev1.AppRecord
	if err := s.orm.WithContext(ctx).Where("name = ?", name).Take(&rec).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, db.ErrNotFound
		}
		return nil, err
	}
	return fromAppRecord(rec), nil
}

func (s *DB) List(ctx context.Context, opts db.ListOptions) (*metav1.List[*corev1.App], error) {
	if s == nil || s.orm == nil {
		return nil, errors.New("db is nil")
	}
	tx := s.orm.WithContext(ctx).Model(&corev1.AppRecord{})
	for _, field := range pgutil.SortedFieldKeys(opts.FieldSelector) {
		val := opts.FieldSelector[field]
		switch field {
		case "metadata.name":
			tx = tx.Where("name = ?", val)
		case "metadata.name.in":
			tx = db.ApplyNameInWhere(tx, "name", val)
		case "category", "spec.category":
			tx = tx.Where("category = ?", val)
		case "published", "spec.published":
			b, err := strconv.ParseBool(val)
			if err != nil {
				return nil, fmt.Errorf("invalid boolean fieldSelector %s=%q", field, val)
			}
			tx = tx.Where("published = ?", b)
		default:
			return nil, fmt.Errorf("unsupported app fieldSelector %q", field)
		}
	}
	tx = db.ApplyLabelSelectorCol(tx, "labels", opts.LabelSelector)
	rows, total, err := pgutil.Paginate[corev1.AppRecord](tx, "created_at DESC, name ASC", opts)
	if err != nil {
		return nil, err
	}
	items := make([]*corev1.App, 0, len(rows))
	for _, row := range rows {
		item := fromAppRecord(row)
		if opts.LabelSelector != nil && !opts.LabelSelector.Matches(item.Metadata.Labels) {
			continue
		}
		items = append(items, item)
	}
	return &metav1.List[*corev1.App]{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.GroupVersion, Kind: "AppList"},
		Metadata: metav1.ListMeta{TotalCount: total},
		Items:    items,
	}, nil
}

func (s *DB) Update(ctx context.Context, in *corev1.App) (*corev1.App, error) {
	return s.updateTemplate(ctx, in, false)
}

func (s *DB) UpdateObject(ctx context.Context, in *corev1.App) (*corev1.App, error) {
	return s.updateTemplate(ctx, in, true)
}

func (s *DB) updateTemplate(ctx context.Context, in *corev1.App, requireCAS bool) (*corev1.App, error) {
	if s == nil || s.orm == nil {
		return nil, errors.New("db is nil")
	}
	if requireCAS && (in.Metadata.UID == "" || in.Metadata.ResourceVersion == "") {
		return nil, db.ErrConflict
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
	if appTemplateChanged(existing, &cp) {
		cp.Metadata.Generation++
	}
	return s.commitTemplateUpdate(ctx, &cp, oldRV, newRV, requireCAS || in.Metadata.ResourceVersion != "")
}

func appTemplateChanged(a, b *corev1.App) bool {
	ca, cb := *a, *b
	ca.Metadata, cb.Metadata = a.Metadata, a.Metadata
	return !reflect.DeepEqual(ca, cb)
}

func marshalOptionalJSON(v any, nonNil bool) any {
	if !nonNil {
		return nil
	}
	raw, _ := pgutil.MarshalJSONCol(v)
	return raw
}

func appUpdateColumns(rec corev1.AppRecord, newRV int64) map[string]any {
	verJSON, _ := pgutil.MarshalJSONCol(rec.Versions)
	cmdJSON, _ := pgutil.MarshalJSONCol(rec.Command)
	portsJSON, _ := pgutil.MarshalJSONCol(rec.Ports)
	envJSON, _ := pgutil.MarshalJSONCol(rec.Env)
	resJSON, _ := pgutil.MarshalJSONCol(rec.Resources)
	schemaJSON, _ := pgutil.MarshalJSONCol(rec.ConfigSchema)
	presetsJSON, _ := pgutil.MarshalJSONCol(rec.ConfigPresets)
	tplJSON, _ := pgutil.MarshalJSONCol(rec.FileTemplates)
	targetsJSON, _ := pgutil.MarshalJSONCol(rec.ConsoleTargets)
	actionsJSON, _ := pgutil.MarshalJSONCol(rec.ConsoleActions)
	metricsJSON, _ := pgutil.MarshalJSONCol(rec.Metrics)
	capsJSON, _ := pgutil.MarshalJSONCol(rec.Capabilities)
	finJSON, _ := pgutil.MarshalJSONCol(rec.Finalizers)
	lblJSON, _ := pgutil.MarshalJSONCol(rec.Labels)
	annJSON, _ := pgutil.MarshalJSONCol(rec.Annotations)
	return map[string]any{
		"display_name": rec.DisplayName, "tagline": rec.Tagline, "summary": rec.Summary,
		"category": rec.Category, "cover_image": rec.CoverImage, "image": rec.Image,
		"image_template": rec.ImageTemplate, "versions": verJSON, "recommended_version": rec.RecommendedVersion,
		"server_binding":     marshalOptionalJSON(rec.ServerBinding, rec.ServerBinding != nil),
		"addon_binding":      marshalOptionalJSON(rec.AddonBinding, rec.AddonBinding != nil),
		"runtime_class_name": rec.RuntimeClassName, "pre_start_command": rec.PreStartCommand,
		"startup_command": rec.StartupCommand, "pre_stop_command": rec.PreStopCommand, "command": cmdJSON, "working_dir": rec.WorkingDir,
		"ports": portsJSON, "env": envJSON, "recommended_plan": rec.RecommendedPlan, "resources": resJSON,
		"config_schema": schemaJSON, "config_presets": presetsJSON, "file_templates": tplJSON,
		"console_delivery": rec.ConsoleDelivery, "console_targets": targetsJSON, "console_actions": actionsJSON,
		"storage_policy": marshalOptionalJSON(rec.StoragePolicy, rec.StoragePolicy != nil),
		"metrics":        metricsJSON, "capabilities": capsJSON, "published": rec.Published, "built_in": rec.BuiltIn,
		"resource_version": newRV, "generation": rec.Generation, "deleted_at": rec.DeletedAt,
		"finalizers": finJSON, "labels": lblJSON, "annotations": annJSON,
	}
}

func (s *DB) commitTemplateUpdate(ctx context.Context, cp *corev1.App, oldRV, newRV int64, checkRV bool) (*corev1.App, error) {
	rec := toAppRecord(cp, newRV)
	tx := s.orm.WithContext(ctx).Model(&corev1.AppRecord{}).Where("name = ? AND uid = ?", rec.Name, rec.UID)
	if checkRV {
		tx = tx.Where("resource_version = ?", oldRV)
	}
	res := tx.Updates(appUpdateColumns(rec, newRV))
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, db.ErrConflict
	}
	return s.Get(ctx, rec.Name)
}

func (s *DB) Delete(ctx context.Context, name string) error {
	return s.DeleteIfUID(ctx, name, "")
}

func (s *DB) DeleteIfUID(ctx context.Context, name, expectedUID string) error {
	if s == nil {
		return errors.New("db is nil")
	}
	return pgutil.DeleteByCondition(ctx, s.orm, "apps", "name = ?", []any{name}, expectedUID)
}
