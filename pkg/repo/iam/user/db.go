package user

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	iamv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/iam/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/repo/pgutil"
)

// DB manages persistence for User across users (spec/metadata) and user_states (runtime status).
type DB struct{ orm *gorm.DB }

func NewDB(pool *pgxpool.Pool) *DB {
	if pool == nil {
		return nil
	}
	return &DB{orm: db.OpenGORM(pool)}
}

func toRecords(in *iamv1.User, rv, sv int64) (iamv1.UserRecord, iamv1.UserStateRecord) {
	phase := in.Status.Phase
	if phase == "" {
		phase = in.Spec.State
	}
	rec := iamv1.UserRecord{
		Name: in.Metadata.Name, UID: in.Metadata.UID, Email: in.Spec.Email,
		DisplayName: in.Spec.DisplayName, PasswordHash: in.Spec.PasswordHash, State: string(in.Spec.State),
		ResourceVersion: rv, Generation: in.Metadata.Generation,
		CreatedAt: in.Metadata.CreationTimestamp, DeletedAt: in.Metadata.DeletionTimestamp,
		Finalizers: pgutil.NonNilStrings(in.Metadata.Finalizers),
		Labels:     pgutil.NonNilMap(in.Metadata.Labels), Annotations: pgutil.NonNilMap(in.Metadata.Annotations),
	}
	st := iamv1.UserStateRecord{
		UserUID: in.Metadata.UID, Version: sv, ObservedGeneration: in.Status.ObservedGeneration,
		Phase: string(phase), DefaultTenant: in.Status.DefaultTenant, LastLoginTime: in.Status.LastLoginTime,
		Conditions: pgutil.NonNilConditions(in.Status.Conditions), UpdatedAt: time.Now().UTC(),
	}
	return rec, st
}

func fromRecords(rec iamv1.UserRecord, st iamv1.UserStateRecord, hasState bool) *iamv1.User {
	out := &iamv1.User{
		TypeMeta: metav1.TypeMeta{APIVersion: iamv1.GroupVersion, Kind: "User"},
		Metadata: metav1.ObjectMeta{
			Name: rec.Name, UID: rec.UID, ResourceVersion: pgutil.FormatVersion(rec.ResourceVersion),
			Generation: rec.Generation, CreationTimestamp: rec.CreatedAt, DeletionTimestamp: rec.DeletedAt,
		},
		Spec: iamv1.UserSpec{
			Email: rec.Email, DisplayName: rec.DisplayName, PasswordHash: rec.PasswordHash,
			State: iamv1.UserState(rec.State),
		},
		Status: iamv1.UserStatus{Phase: iamv1.UserState(rec.State)},
	}
	if hasState {
		out.Metadata.StatusVersion = pgutil.FormatVersion(st.Version)
		out.Status = iamv1.UserStatus{
			ObservedGeneration: st.ObservedGeneration,
			Phase:              iamv1.UserState(st.Phase),
			DefaultTenant:      st.DefaultTenant,
			LastLoginTime:      st.LastLoginTime,
			Conditions:         append([]metav1.Condition(nil), st.Conditions...),
		}
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

func (s *DB) Create(ctx context.Context, in *iamv1.User) (*iamv1.User, error) {
	if s == nil || s.orm == nil {
		return nil, errors.New("db is nil")
	}
	cp := *in
	cp.DefaultMeta()
	if err := cp.Validate(); err != nil {
		return nil, err
	}
	pgutil.InitCreateMeta(&cp.Metadata)
	cp.Metadata.StatusVersion = "1"
	rec, st := toRecords(&cp, 1, 1)
	tx := s.orm.WithContext(ctx).Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}
	defer tx.Rollback()
	res := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "name"}}, DoNothing: true}).Create(&rec)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, db.ErrAlreadyExists
	}
	if err := tx.Create(&st).Error; err != nil {
		return nil, err
	}
	if err := tx.Commit().Error; err != nil {
		return nil, err
	}
	return fromRecords(rec, st, true), nil
}

func (s *DB) Get(ctx context.Context, name string) (*iamv1.User, error) {
	if s == nil || s.orm == nil {
		return nil, errors.New("db is nil")
	}
	var rec iamv1.UserRecord
	if err := s.orm.WithContext(ctx).Where("name = ?", name).Take(&rec).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, db.ErrNotFound
		}
		return nil, err
	}
	var st iamv1.UserStateRecord
	err := s.orm.WithContext(ctx).Where("user_uid = ?", rec.UID).Take(&st).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	return fromRecords(rec, st, err == nil), nil
}

func (s *DB) List(ctx context.Context, opts db.ListOptions) (*metav1.List[*iamv1.User], error) {
	if s == nil || s.orm == nil {
		return nil, errors.New("db is nil")
	}
	tx := s.orm.WithContext(ctx).Model(&iamv1.UserRecord{})
	for _, field := range pgutil.SortedFieldKeys(opts.FieldSelector) {
		val := opts.FieldSelector[field]
		switch field {
		case "metadata.name":
			tx = tx.Where("users.name = ?", val)
		case "metadata.name.in":
			tx = db.ApplyNameInWhere(tx, "users.name", val)
		case "spec.email":
			tx = tx.Where("LOWER(users.email) = LOWER(?)", val)
		case "spec.state":
			tx = tx.Where("users.state = ?", val)
		case "status.phase":
			tx = tx.Joins("JOIN user_states ON user_states.user_uid = users.uid").Where("user_states.phase = ?", val)
		default:
			return nil, fmt.Errorf("unsupported user fieldSelector %q", field)
		}
	}
	tx = db.ApplyLabelSelectorCol(tx, "users.labels", opts.LabelSelector)
	rows, total, err := pgutil.Paginate[iamv1.UserRecord](tx, "users.created_at DESC, users.name ASC", opts)
	if err != nil {
		return nil, err
	}
	return s.hydrateList(ctx, rows, total, opts.LabelSelector)
}

func (s *DB) hydrateList(ctx context.Context, rows []iamv1.UserRecord, total int, sel *metav1.LabelSelector) (*metav1.List[*iamv1.User], error) {
	stateMap := make(map[string]iamv1.UserStateRecord, len(rows))
	if len(rows) > 0 {
		uids := make([]string, len(rows))
		for i, r := range rows {
			uids[i] = r.UID
		}
		var states []iamv1.UserStateRecord
		if err := s.orm.WithContext(ctx).Where("user_uid IN ?", uids).Find(&states).Error; err != nil {
			return nil, err
		}
		for _, st := range states {
			stateMap[st.UserUID] = st
		}
	}
	items := make([]*iamv1.User, 0, len(rows))
	for _, row := range rows {
		st, ok := stateMap[row.UID]
		item := fromRecords(row, st, ok)
		if sel != nil && !sel.Matches(item.Metadata.Labels) {
			continue
		}
		items = append(items, item)
	}
	return &metav1.List[*iamv1.User]{
		TypeMeta: metav1.TypeMeta{APIVersion: iamv1.GroupVersion, Kind: "UserList"},
		Metadata: metav1.ListMeta{TotalCount: total},
		Items:    items,
	}, nil
}

func (s *DB) Delete(ctx context.Context, name string) error {
	return s.DeleteIfUID(ctx, name, "")
}

func (s *DB) DeleteIfUID(ctx context.Context, name, expectedUID string) error {
	if s == nil {
		return errors.New("db is nil")
	}
	return pgutil.DeleteByCondition(ctx, s.orm, "users", "name = ?", []any{name}, expectedUID)
}
