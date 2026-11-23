package user

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	iamv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/iam/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/config"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/mq"
	"github.com/smartcat999/game-panel-cloud/pkg/models"
	tenantmodel "github.com/smartcat999/game-panel-cloud/pkg/models/core/tenant"
	"github.com/smartcat999/game-panel-cloud/pkg/models/iam/tenantrole"
	"github.com/smartcat999/game-panel-cloud/pkg/repo"
	tenantdb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/tenant"
	globalroledb "github.com/smartcat999/game-panel-cloud/pkg/repo/iam/globalrole"
	globalrolebindingdb "github.com/smartcat999/game-panel-cloud/pkg/repo/iam/globalrolebinding"
	tenantrolebindingdb "github.com/smartcat999/game-panel-cloud/pkg/repo/iam/tenantrolebinding"
	userdb "github.com/smartcat999/game-panel-cloud/pkg/repo/iam/user"
)

const noPersonalTenant = "absent"

func builtInAdminName() string {
	if n := strings.TrimSpace(config.Get().Bootstrap.AdminUser); n != "" {
		return n
	}
	return "admin"
}

// SessionRevoker revokes active cached sessions for a given username.
type SessionRevoker interface {
	RevokeByUser(ctx context.Context, username string) error
}

type Operator interface {
	List(ctx context.Context, opts db.ListOptions) (*metav1.List[*iamv1.User], error)
	Get(ctx context.Context, name string) (*iamv1.User, error)
	Create(ctx context.Context, user *iamv1.User) (*iamv1.User, error)
	Update(ctx context.Context, name string, incoming *iamv1.User) (*iamv1.User, error)
	UpdateStatus(ctx context.Context, name string, incoming *iamv1.User) (*iamv1.User, error)
	Delete(ctx context.Context, name string) error
	// ChangePassword verifies currentPassword against the stored bcrypt hash of the named User,
	// then hashes and persists newPassword and revokes all active sessions of that User.
	ChangePassword(ctx context.Context, name, currentPassword, newPassword string) error
}

// MinPasswordLength is the minimum accepted length for a self-service password change.
const MinPasswordLength = 8

var (
	ErrPasswordRequired  = fmt.Errorf("%w: currentPassword and newPassword are required", models.ErrBadRequest)
	ErrPasswordTooShort  = fmt.Errorf("%w: newPassword must be at least %d characters", models.ErrBadRequest, MinPasswordLength)
	ErrPasswordUnchanged = fmt.Errorf("%w: newPassword must differ from currentPassword", models.ErrBadRequest)
	ErrPasswordIncorrect = fmt.Errorf("%w: current password is incorrect", models.ErrForbidden)
)

type operator struct {
	db       *userdb.DB
	tenantDB *tenantdb.DB
	grDB     *globalroledb.DB
	grbDB    *globalrolebindingdb.DB
	trbDB    *tenantrolebindingdb.DB
	reg      *repo.Registry
	revoker  SessionRevoker
	mq       mq.Interface
}

func NewOperator(reg *repo.Registry, bus mq.Interface) Operator {
	return NewOperatorWithRevoker(reg, nil, bus)
}

func NewOperatorWithRevoker(reg *repo.Registry, revoker SessionRevoker, bus mq.Interface) Operator {
	op := &operator{reg: reg, revoker: revoker, mq: bus}
	if reg != nil {
		pool := reg.Pool()
		op.db = userdb.NewDB(pool)
		op.tenantDB = tenantdb.NewDB(pool)
		op.grDB = globalroledb.NewDB(pool)
		op.grbDB = globalrolebindingdb.NewDB(pool)
		op.trbDB = tenantrolebindingdb.NewDB(pool)
	}
	return op
}

func (o *operator) List(ctx context.Context, opts db.ListOptions) (*metav1.List[*iamv1.User], error) {
	list, err := o.db.List(ctx, opts)
	if err != nil {
		return nil, err
	}
	for i := range list.Items {
		list.Items[i] = sanitizeUser(list.Items[i])
	}
	return list, nil
}

func (o *operator) Get(ctx context.Context, name string) (*iamv1.User, error) {
	u, err := o.db.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	return sanitizeUser(u), nil
}

func (o *operator) ensureUniqueEmail(ctx context.Context, selfName, email string) error {
	email = strings.TrimSpace(email)
	if email == "" {
		return nil
	}
	list, err := o.db.List(ctx, db.ListOptions{
		FieldSelector: map[string]string{"spec.email": email},
		Limit:         2,
	})
	if err != nil {
		return err
	}
	for _, u := range list.Items {
		if u != nil && u.Metadata.Name != selfName && strings.EqualFold(u.Spec.Email, email) {
			return fmt.Errorf("%w: email %q is already registered to another user", models.ErrConflict, email)
		}
	}
	return nil
}

func (o *operator) Create(ctx context.Context, user *iamv1.User) (*iamv1.User, error) {
	if _, err := o.db.Get(ctx, user.Metadata.Name); err == nil {
		return nil, db.ErrAlreadyExists
	} else if !errors.Is(err, db.ErrNotFound) {
		return nil, err
	}
	if o.tenantDB != nil {
		if _, err := o.tenantDB.Get(ctx, user.Metadata.Name); err == nil {
			return nil, fmt.Errorf("%w: a workspace already reserves the user's name", models.ErrConflict)
		} else if !errors.Is(err, db.ErrNotFound) {
			return nil, err
		}
	}
	resetUserLifecycleForCreate(user)
	if err := o.ensureUniqueEmail(ctx, user.Metadata.Name, user.Spec.Email); err != nil {
		return nil, err
	}
	if user.Spec.Password != "" {
		hash, err := HashPassword(user.Spec.Password)
		if err != nil {
			return nil, err
		}
		user.Spec.PasswordHash = hash
		user.Spec.Password = ""
	}
	if user.Status.DefaultTenant == "" && !PersonalTenantDisabled(user) {
		user.Status.DefaultTenant = user.Metadata.Name
	}
	created, err := o.db.Create(ctx, user)
	if err != nil {
		return nil, err
	}
	if o.mq != nil {
		_ = o.mq.Publish(ctx, mq.TopicUser, mq.NewEvent(mq.EventCreated, iamv1.GroupVersion, "User", "", created.Metadata.Name, created.Metadata.Generation, created.Metadata.ResourceVersion))
	}
	return sanitizeUser(created), nil
}

func subjectFilterForUser(u iamv1.UserInfo) string {
	parts := []string{"*"}
	if u.Name != "" {
		parts = append(parts, u.Name)
	}
	for _, g := range u.Groups {
		if g != "" {
			parts = append(parts, g)
		}
	}
	return strings.Join(parts, ",")
}

func (o *operator) canManageAllUsers(ctx context.Context, u iamv1.UserInfo) bool {
	if o.grbDB == nil || o.grDB == nil {
		return true
	}
	grbs, err := o.grbDB.List(ctx, db.ListOptions{
		FieldSelector: map[string]string{"spec.subject": subjectFilterForUser(u)},
	})
	if err != nil || grbs == nil {
		return false
	}
	for _, b := range grbs.Items {
		if b == nil || !subjectMatchesUser(b.Subjects, u) {
			continue
		}
		role, err := o.grDB.Get(ctx, b.RoleRef.Name)
		if err != nil || role == nil {
			continue
		}
		for _, rule := range role.Rules {
			if rule.Matches(iamv1.GroupName, "users", "update", "*", nil) {
				return true
			}
		}
	}
	return false
}

func subjectMatchesUser(subjects []iamv1.Subject, u iamv1.UserInfo) bool {
	for _, s := range subjects {
		if s.Kind == iamv1.SubjectKindGroup {
			for _, g := range u.Groups {
				if s.Name == "*" || s.Name == g {
					return true
				}
			}
			continue
		}
		if (s.Kind == "" || s.Kind == u.Kind) && (s.Name == "*" || s.Name == u.Name) {
			return true
		}
	}
	return false
}

func (o *operator) Update(ctx context.Context, name string, incoming *iamv1.User) (*iamv1.User, error) {
	incoming.Metadata.Name = name
	if name == builtInAdminName() && incoming.Spec.State == iamv1.UserStateDisabled {
		return nil, fmt.Errorf("%w: cannot disable built-in administrator user %q", models.ErrConflict, name)
	}
	existing, err := o.db.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	if existing.Metadata.DeletionTimestamp != nil {
		return nil, fmt.Errorf("%w: terminating user is immutable", models.ErrConflict)
	}
	if (incoming.Metadata.UID != "" && incoming.Metadata.UID != existing.Metadata.UID) ||
		(incoming.Metadata.ResourceVersion != "" && incoming.Metadata.ResourceVersion != existing.Metadata.ResourceVersion) {
		return nil, db.ErrConflict
	}
	revokeSessions, err := o.validateAndApplyPasswordAndEmailUpdate(ctx, name, existing, incoming)
	if err != nil {
		return nil, err
	}
	preserveUserLifecycle(existing, incoming)
	updated, err := o.db.Update(ctx, incoming)
	if err != nil {
		return nil, err
	}
	if revokeSessions && o.revoker != nil {
		_ = o.revoker.RevokeByUser(ctx, name)
	}
	if o.mq != nil {
		_ = o.mq.Publish(ctx, mq.TopicUser, mq.NewEvent(mq.EventUpdated, iamv1.GroupVersion, "User", "", updated.Metadata.Name, updated.Metadata.Generation, updated.Metadata.ResourceVersion))
	}
	return sanitizeUser(updated), nil
}

func (o *operator) validateAndApplyPasswordAndEmailUpdate(ctx context.Context, name string, existing, incoming *iamv1.User) (bool, error) {
	isSelfUpdate := false
	if u, hasUser := iamv1.UserFrom(ctx); hasUser && u.Kind == iamv1.SubjectKindUser && !o.canManageAllUsers(ctx, u) {
		isSelfUpdate = true
		if incoming.Spec.State != "" && incoming.Spec.State != existing.Spec.State {
			return false, fmt.Errorf("%w: only platform administrators may modify user state", models.ErrForbidden)
		}
		incoming.Spec.State = existing.Spec.State
		incoming.Spec.PasswordHash = ""
	}
	if incoming.Spec.CurrentPassword != "" || (isSelfUpdate && incoming.Spec.Password != "" && existing.Spec.PasswordHash != "") {
		if incoming.Spec.CurrentPassword == "" {
			return false, ErrPasswordRequired
		}
		if !VerifyPassword(existing.Spec.PasswordHash, incoming.Spec.CurrentPassword) {
			return false, ErrPasswordIncorrect
		}
		if incoming.Spec.Password != "" && incoming.Spec.CurrentPassword == incoming.Spec.Password {
			return false, ErrPasswordUnchanged
		}
	}
	incoming.Spec.CurrentPassword = ""
	if !strings.EqualFold(existing.Spec.Email, incoming.Spec.Email) {
		if err := o.ensureUniqueEmail(ctx, name, incoming.Spec.Email); err != nil {
			return false, err
		}
	}
	revokeSessions := incoming.Spec.State == iamv1.UserStateDisabled ||
		incoming.Spec.Password != "" ||
		(incoming.Spec.PasswordHash != "" && incoming.Spec.PasswordHash != existing.Spec.PasswordHash)
	if incoming.Spec.Password != "" {
		if len(incoming.Spec.Password) < MinPasswordLength {
			return false, ErrPasswordTooShort
		}
		hash, err := HashPassword(incoming.Spec.Password)
		if err != nil {
			return false, err
		}
		incoming.Spec.PasswordHash = hash
		incoming.Spec.Password = ""
	} else if incoming.Spec.PasswordHash == "" {
		incoming.Spec.PasswordHash = existing.Spec.PasswordHash
	}
	return revokeSessions, nil
}

func (o *operator) UpdateStatus(ctx context.Context, name string, incoming *iamv1.User) (*iamv1.User, error) {
	incoming.Metadata.Name = name
	current, err := o.db.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	if current.Metadata.DeletionTimestamp != nil {
		return nil, fmt.Errorf("%w: terminating user status is owned by cleanup", models.ErrConflict)
	}
	if (incoming.Metadata.UID != "" && incoming.Metadata.UID != current.Metadata.UID) ||
		(incoming.Metadata.ResourceVersion != "" && incoming.Metadata.ResourceVersion != current.Metadata.ResourceVersion) {
		return nil, db.ErrConflict
	}
	preserveUserLifecycle(current, incoming)
	updated, err := o.db.UpdateStatus(ctx, incoming)
	if err != nil {
		return nil, err
	}
	if o.mq != nil {
		_ = o.mq.Publish(ctx, mq.TopicUser, mq.NewEvent(mq.EventUpdated, iamv1.GroupVersion, "User", "", updated.Metadata.Name, updated.Metadata.Generation, updated.Metadata.ResourceVersion))
	}
	return sanitizeUser(updated), nil
}

func withoutUser(subjects []iamv1.Subject, user string) ([]iamv1.Subject, bool) {
	out := make([]iamv1.Subject, 0, len(subjects))
	changed := false
	for _, s := range subjects {
		if (s.Kind == iamv1.SubjectKindUser || s.Kind == "") && s.Name == user {
			changed = true
			continue
		}
		out = append(out, s)
	}
	return out, changed
}

func (o *operator) publishBinding(ctx context.Context, topic, kind, tenant, name string, typ mq.EventType) {
	if o.mq != nil {
		_ = o.mq.Publish(ctx, topic, mq.NewEvent(typ, iamv1.GroupVersion, kind, tenant, name, 0, ""))
	}
}

func (o *operator) ChangePassword(ctx context.Context, name, currentPassword, newPassword string) error {
	if currentPassword == "" || newPassword == "" {
		return ErrPasswordRequired
	}
	if len(newPassword) < MinPasswordLength {
		return ErrPasswordTooShort
	}
	if currentPassword == newPassword {
		return ErrPasswordUnchanged
	}
	existing, err := o.db.Get(ctx, name)
	if err != nil {
		return err
	}
	if existing.Metadata.DeletionTimestamp != nil {
		return fmt.Errorf("%w: terminating user credentials are immutable", models.ErrConflict)
	}
	if !VerifyPassword(existing.Spec.PasswordHash, currentPassword) {
		return ErrPasswordIncorrect
	}
	hash, err := HashPassword(newPassword)
	if err != nil {
		return err
	}
	existing.Spec.Password = ""
	existing.Spec.PasswordHash = hash
	updated, err := o.db.Update(ctx, existing)
	if err != nil {
		return err
	}
	if o.revoker != nil {
		_ = o.revoker.RevokeByUser(ctx, name)
	}
	if o.mq != nil {
		_ = o.mq.Publish(ctx, mq.TopicUser, mq.NewEvent(mq.EventUpdated, iamv1.GroupVersion, "User", "", updated.Metadata.Name, updated.Metadata.Generation, updated.Metadata.ResourceVersion))
	}
	return nil
}

func sanitizeUser(u *iamv1.User) *iamv1.User {
	if u == nil {
		return nil
	}
	cloned := *u
	cloned.Spec.CurrentPassword = ""
	cloned.Spec.Password = ""
	cloned.Spec.PasswordHash = ""
	return &cloned
}

// HashPassword hashes a plaintext password with bcrypt.
func HashPassword(plain string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.MinCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// VerifyPassword compares a bcrypt password hash against plaintext.
func VerifyPassword(hash, plain string) bool {
	if hash == "" || plain == "" {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}

// PersonalTenantDisabled reports whether automatic personal workspace provisioning is disabled for user.
func PersonalTenantDisabled(user *iamv1.User) bool {
	if user == nil || len(user.Metadata.Annotations) == 0 {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(user.Metadata.Annotations[iamv1.AnnotationDisablePersonalTenant]), "true")
}

func EnsurePersonalTenant(ctx context.Context, reg *repo.Registry, username string) error {
	current, err := userdb.NewDB(reg.Pool()).Get(ctx, username)
	if err != nil {
		return err
	}
	return EnsurePersonalTenantForUID(ctx, reg, username, current.Metadata.UID)
}

func EnsurePersonalTenantForUID(ctx context.Context, reg *repo.Registry, username, expectedUID string) error {
	current, err := currentUser(ctx, reg, username, expectedUID)
	if err != nil {
		return err
	}
	if current.Metadata.DeletionTimestamp != nil {
		return fmt.Errorf("%w: user is terminating", models.ErrConflict)
	}
	if PersonalTenantDisabled(current) {
		return ensureAuthenticatedRole(ctx, reg)
	}
	tenantDB := tenantdb.NewDB(reg.Pool())
	userDB := userdb.NewDB(reg.Pool())
	tenant, err := tenantDB.Get(ctx, username)
	if errors.Is(err, db.ErrNotFound) {
		if current.Metadata.Annotations[iamv1.AnnotationPersonalTenantUID] != "" {
			return fmt.Errorf("%w: the user's bound personal tenant is missing", models.ErrConflict)
		}
		tenant, err = createPersonalTenantRecord(ctx, tenantDB, username, expectedUID)
	}
	if err != nil {
		return err
	}
	if !personalTenantOwnedBy(tenant, username, expectedUID) || tenant.Metadata.DeletionTimestamp != nil {
		return fmt.Errorf("%w: personal tenant belongs to another identity or is terminating", models.ErrConflict)
	}
	if err := syncPersonalTenantBinding(ctx, userDB, current, tenant.Metadata.UID); err != nil {
		return err
	}
	if err := ensureAuthenticatedRole(ctx, reg); err != nil {
		return err
	}
	return tenantrole.EnsureBuiltInRoles(ctx, reg, username, username)
}

func createPersonalTenantRecord(ctx context.Context, tenantDB *tenantdb.DB, username, expectedUID string) (*corev1.Tenant, error) {
	allUnlimited := []corev1.ResourceName{
		corev1.ResourceCPU,
		corev1.ResourceMemory,
		corev1.ResourceStorage,
		corev1.ResourceInstances,
		corev1.ResourceBackups,
		corev1.ResourceBackupStorage,
	}
	candidate := &corev1.Tenant{
		Metadata: metav1.ObjectMeta{Name: username, Annotations: map[string]string{iamv1.AnnotationOwnerUID: expectedUID}},
		Spec:     corev1.TenantSpec{DisplayName: username + " Personal", Type: corev1.TenantTypePersonal, Owner: username, UnlimitedQuota: allUnlimited},
	}
	tenant, err := tenantDB.Create(ctx, candidate)
	if errors.Is(err, db.ErrAlreadyExists) {
		return tenantDB.Get(ctx, username)
	}
	return tenant, err
}

func syncPersonalTenantBinding(ctx context.Context, userDB *userdb.DB, current *iamv1.User, tenantUID string) error {
	bound := current.Metadata.Annotations[iamv1.AnnotationPersonalTenantUID]
	if bound != "" {
		if bound != tenantUID {
			return fmt.Errorf("%w: personal tenant identity changed", models.ErrConflict)
		}
		return nil
	}
	setUserAnnotation(current, iamv1.AnnotationPersonalTenantUID, tenantUID)
	_, err := userDB.Update(ctx, current)
	return err
}

func personalTenantOwnedBy(tenant *corev1.Tenant, username, userUID string) bool {
	return tenant != nil && tenant.Metadata.UID != "" && tenant.Metadata.Name == username && tenant.Spec.Type == corev1.TenantTypePersonal && tenant.Spec.Owner == username && tenant.Metadata.Annotations[iamv1.AnnotationOwnerUID] == userUID
}

func ensureAuthenticatedRole(ctx context.Context, reg *repo.Registry) error {
	if reg == nil || reg.Pool() == nil {
		return nil
	}
	grDB := globalroledb.NewDB(reg.Pool())
	grbDB := globalrolebindingdb.NewDB(reg.Pool())
	if _, err := grDB.Get(ctx, "authenticated-user"); errors.Is(err, db.ErrNotFound) {
		_, err = grDB.Create(ctx, &iamv1.GlobalRole{
			Metadata:    metav1.ObjectMeta{Name: "authenticated-user"},
			Description: "Baseline authenticated user access",
			Rules: []iamv1.PolicyRule{
				{APIGroups: []string{corev1.GroupName}, Resources: []string{"tenants"}, Verbs: []string{iamv1.VerbList, iamv1.VerbCreate}},
				{APIGroups: []string{corev1.GroupName}, Resources: []string{"runtimeclasses", "apps"}, Verbs: []string{iamv1.VerbGet, iamv1.VerbList}},
			},
		})
		if err != nil && !errors.Is(err, db.ErrAlreadyExists) {
			return err
		}
	} else if err != nil {
		return err
	}
	if _, err := grbDB.Get(ctx, "system-authenticated-users"); errors.Is(err, db.ErrNotFound) {
		_, err = grbDB.Create(ctx, &iamv1.GlobalRoleBinding{Metadata: metav1.ObjectMeta{Name: "system-authenticated-users"}, RoleRef: iamv1.RoleRef{Name: "authenticated-user"}, Subjects: []iamv1.Subject{{Kind: iamv1.SubjectKindUser, Name: "*"}}})
		if err != nil && !errors.Is(err, db.ErrAlreadyExists) {
			return err
		}
	} else if err != nil {
		return err
	}
	return nil
}

type userDeletionStore interface {
	Get(ctx context.Context, name string) (*iamv1.User, error)
	UpdateObject(ctx context.Context, in *iamv1.User) (*iamv1.User, error)
	UpdateStatus(ctx context.Context, in *iamv1.User) (*iamv1.User, error)
}

func (o *operator) Delete(ctx context.Context, name string) error {
	if name == builtInAdminName() {
		return fmt.Errorf("%w: cannot delete built-in administrator user %q", models.ErrConflict, name)
	}
	var store userDeletionStore
	if o.db != nil {
		store = o.db
	}
	current, err := markUserDeleting(ctx, store, name)
	if err != nil {
		return err
	}
	if o.revoker != nil {
		var revokeErr error
		if o.reg != nil {
			revokeErr = revokeDeletingUser(ctx, o.reg, current, o.revoker)
		} else {
			revokeErr = revokeDeletingUserWithStore(ctx, store, current, o.revoker)
		}
		if revokeErr != nil {
			o.publishUser(ctx, mq.EventUpdated, current)
			return revokeErr
		}
	}
	o.publishUser(ctx, mq.EventUpdated, current)
	return nil
}

func markUserDeleting(ctx context.Context, store userDeletionStore, name string) (*iamv1.User, error) {
	if store == nil {
		return nil, errors.New("user store is nil")
	}
	var expectedUID string
	for attempt := range 4 {
		current, err := store.Get(ctx, name)
		if err != nil {
			return nil, err
		}
		if attempt == 0 {
			expectedUID = current.Metadata.UID
		} else if expectedUID == "" || current.Metadata.UID != expectedUID {
			return nil, fmt.Errorf("%w: user identity changed", db.ErrConflict)
		}
		if current.Metadata.DeletionTimestamp != nil && slices.Contains(current.Metadata.Finalizers, iamv1.FinalizerUserCleanup) && current.Spec.State == iamv1.UserStateDisabled {
			return current, nil
		}
		if current.Metadata.DeletionTimestamp == nil {
			now := time.Now().UTC()
			current.Metadata.DeletionTimestamp = &now
		}
		if !slices.Contains(current.Metadata.Finalizers, iamv1.FinalizerUserCleanup) {
			current.Metadata.Finalizers = append(current.Metadata.Finalizers, iamv1.FinalizerUserCleanup)
		}
		current.Spec.State = iamv1.UserStateDisabled
		current.Status.Phase = iamv1.UserStateDisabled
		updated, uErr := store.UpdateObject(ctx, current)
		if errors.Is(uErr, db.ErrConflict) {
			continue
		}
		if uErr != nil {
			return nil, uErr
		}
		updated.Status.Phase = iamv1.UserStateDisabled
		if stUpdated, sErr := store.UpdateStatus(ctx, updated); sErr == nil {
			updated = stUpdated
		}
		return updated, nil
	}
	return nil, db.ErrConflict
}

func currentUser(ctx context.Context, reg *repo.Registry, name, expectedUID string) (*iamv1.User, error) {
	current, err := userdb.NewDB(reg.Pool()).Get(ctx, name)
	if err != nil {
		return nil, err
	}
	if expectedUID == "" || current.Metadata.UID != expectedUID {
		return nil, fmt.Errorf("%w: user identity changed", db.ErrConflict)
	}
	return current, nil
}

func setUserAnnotation(user *iamv1.User, key, value string) {
	if user.Metadata.Annotations == nil {
		user.Metadata.Annotations = make(map[string]string)
	}
	user.Metadata.Annotations[key] = value
}

func resetUserLifecycleForCreate(user *iamv1.User) {
	user.Metadata.UID = ""
	user.Metadata.ResourceVersion = ""
	user.Metadata.DeletionTimestamp = nil
	user.Metadata.Finalizers = slices.DeleteFunc(slices.Clone(user.Metadata.Finalizers), func(value string) bool { return value == iamv1.FinalizerUserCleanup })
	user.Metadata.Annotations = maps.Clone(user.Metadata.Annotations)
	for _, key := range []string{iamv1.AnnotationPersonalTenantUID, iamv1.AnnotationSessionsRevoked} {
		delete(user.Metadata.Annotations, key)
	}
}

func preserveUserLifecycle(current, incoming *iamv1.User) {
	incoming.Metadata.UID = current.Metadata.UID
	incoming.Metadata.ResourceVersion = current.Metadata.ResourceVersion
	incoming.Metadata.DeletionTimestamp = current.Metadata.DeletionTimestamp
	incoming.Metadata.Finalizers = slices.Clone(current.Metadata.Finalizers)
	incoming.Metadata.Annotations = maps.Clone(incoming.Metadata.Annotations)
	for _, key := range []string{iamv1.AnnotationPersonalTenantUID, iamv1.AnnotationSessionsRevoked} {
		delete(incoming.Metadata.Annotations, key)
		if value := current.Metadata.Annotations[key]; value != "" {
			setUserAnnotation(incoming, key, value)
		}
	}
}

func revokeDeletingUser(ctx context.Context, reg *repo.Registry, user *iamv1.User, revoker SessionRevoker) error {
	var store userDeletionStore
	if reg != nil && reg.Pool() != nil {
		store = userdb.NewDB(reg.Pool())
	}
	return revokeDeletingUserWithStore(ctx, store, user, revoker)
}

func revokeDeletingUserWithStore(ctx context.Context, store userDeletionStore, user *iamv1.User, revoker SessionRevoker) error {
	if user == nil {
		return nil
	}
	if user.Metadata.UID != "" && user.Metadata.Annotations[iamv1.AnnotationSessionsRevoked] == user.Metadata.UID {
		return nil
	}
	if revoker == nil {
		return fmt.Errorf("session revoker is required for user cleanup")
	}
	if err := revoker.RevokeByUser(ctx, user.Metadata.Name); err != nil {
		return err
	}
	if store == nil {
		return errors.New("user store is nil")
	}
	for range 4 {
		current, err := store.Get(ctx, user.Metadata.Name)
		if errors.Is(err, db.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if user.Metadata.UID == "" || current.Metadata.UID != user.Metadata.UID {
			return fmt.Errorf("%w: user identity changed", db.ErrConflict)
		}
		if current.Metadata.DeletionTimestamp == nil {
			return db.ErrConflict
		}
		if current.Metadata.Annotations[iamv1.AnnotationSessionsRevoked] == current.Metadata.UID {
			return nil
		}
		setUserAnnotation(current, iamv1.AnnotationSessionsRevoked, current.Metadata.UID)
		_, err = store.UpdateObject(ctx, current)
		if err == nil {
			return nil
		}
		if !errors.Is(err, db.ErrConflict) {
			return err
		}
	}
	return db.ErrConflict
}

func ReconcileDeletion(ctx context.Context, reg *repo.Registry, bus mq.Interface, name, expectedUID string, revoker SessionRevoker) error {
	o := NewOperatorWithRevoker(reg, revoker, bus).(*operator)
	current, err := currentUser(ctx, reg, name, expectedUID)
	if errors.Is(err, db.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if current.Metadata.DeletionTimestamp == nil || current.Spec.State != iamv1.UserStateDisabled {
		return fmt.Errorf("%w: user deletion intent is not committed", models.ErrConflict)
	}
	if err := revokeDeletingUser(ctx, reg, current, revoker); err != nil {
		return err
	}
	current, err = currentUser(ctx, reg, name, expectedUID)
	if err != nil {
		return err
	}
	boundUID, done, err := o.reconcilePersonalTenantDuringDeletion(ctx, current, name, expectedUID)
	if err != nil || done {
		return err
	}
	return o.finalizeUserCleanupAndDelete(ctx, name, expectedUID, boundUID)
}

func (o *operator) reconcilePersonalTenantDuringDeletion(ctx context.Context, current *iamv1.User, name, expectedUID string) (string, bool, error) {
	boundUID := current.Metadata.Annotations[iamv1.AnnotationPersonalTenantUID]
	tenant, tenantErr := o.tenantDB.Get(ctx, name)
	if tenantErr != nil && !errors.Is(tenantErr, db.ErrNotFound) {
		return "", true, tenantErr
	}
	if boundUID == "" {
		boundUID = noPersonalTenant
		if tenantErr == nil {
			if !personalTenantOwnedBy(tenant, name, expectedUID) {
				return "", true, fmt.Errorf("%w: personal tenant ownership does not match deleting user", models.ErrConflict)
			}
			boundUID = tenant.Metadata.UID
		}
		setUserAnnotation(current, iamv1.AnnotationPersonalTenantUID, boundUID)
		_, err := o.db.UpdateObject(ctx, current)
		return boundUID, true, err
	}
	if tenantErr == nil {
		if boundUID == noPersonalTenant || tenant.Metadata.UID != boundUID || !personalTenantOwnedBy(tenant, name, expectedUID) {
			return "", true, fmt.Errorf("%w: same-name tenant is not the user's captured personal tenant", models.ErrConflict)
		}
		return boundUID, true, tenantmodel.RequestDeletion(ctx, o.reg, o.mq, name, boundUID)
	}
	return boundUID, false, nil
}

func (o *operator) finalizeUserCleanupAndDelete(ctx context.Context, name, expectedUID, boundUID string) error {
	if err := o.removeUserFromBindings(ctx, name); err != nil {
		return err
	}
	if o.grDB != nil {
		selfRole := "user-self-" + name
		role, err := o.grDB.Get(ctx, selfRole)
		if err != nil && !errors.Is(err, db.ErrNotFound) {
			return err
		}
		if err == nil {
			if err := o.grDB.DeleteIfUID(ctx, selfRole, role.Metadata.UID); err != nil && !errors.Is(err, db.ErrNotFound) {
				return err
			}
			o.publishBinding(ctx, mq.TopicGlobalRole, "GlobalRole", "", selfRole, mq.EventDeleted)
		}
	}
	current, err := currentUser(ctx, o.reg, name, expectedUID)
	if err != nil {
		return err
	}
	if current.Metadata.DeletionTimestamp == nil || current.Metadata.Annotations[iamv1.AnnotationPersonalTenantUID] != boundUID {
		return db.ErrConflict
	}
	if slices.Contains(current.Metadata.Finalizers, iamv1.FinalizerUserCleanup) {
		current.Metadata.Finalizers = slices.DeleteFunc(current.Metadata.Finalizers, func(value string) bool { return value == iamv1.FinalizerUserCleanup })
		current, err = o.db.UpdateObject(ctx, current)
		if err != nil {
			return err
		}
	}
	if len(current.Metadata.Finalizers) != 0 {
		return nil
	}
	if err := o.db.DeleteIfUID(ctx, name, expectedUID); err != nil && !errors.Is(err, db.ErrNotFound) {
		return err
	}
	o.publishUser(ctx, mq.EventDeleted, current)
	return nil
}

func (o *operator) removeUserFromBindings(ctx context.Context, user string) error {
	opts := db.ListOptions{FieldSelector: map[string]string{"subject": user}}
	global, err := o.grbDB.List(ctx, opts)
	if err != nil {
		return err
	}
	for _, binding := range global.Items {
		if binding == nil {
			continue
		}
		rest, changed := withoutUser(binding.Subjects, user)
		if !changed {
			continue
		}
		typ := mq.EventDeleted
		if len(rest) == 0 {
			err = o.grbDB.DeleteIfUID(ctx, binding.Metadata.Name, binding.Metadata.UID)
		} else {
			binding.Subjects = rest
			_, err = o.grbDB.Update(ctx, binding)
			typ = mq.EventUpdated
		}
		if err != nil && !errors.Is(err, db.ErrNotFound) {
			return err
		}
		o.publishBinding(ctx, mq.TopicGlobalRoleBinding, "GlobalRoleBinding", "", binding.Metadata.Name, typ)
	}
	tenant, err := o.trbDB.List(ctx, opts)
	if err != nil {
		return err
	}
	for _, binding := range tenant.Items {
		if binding == nil {
			continue
		}
		rest, changed := withoutUser(binding.Subjects, user)
		if !changed {
			continue
		}
		typ := mq.EventDeleted
		if len(rest) == 0 {
			err = o.trbDB.DeleteIfUID(ctx, binding.Metadata.Tenant, binding.Metadata.Name, binding.Metadata.UID)
		} else {
			binding.Subjects = rest
			_, err = o.trbDB.Update(ctx, binding)
			typ = mq.EventUpdated
		}
		if err != nil && !errors.Is(err, db.ErrNotFound) {
			return err
		}
		o.publishBinding(ctx, mq.TopicTenantRoleBinding, "TenantRoleBinding", binding.Metadata.Tenant, binding.Metadata.Name, typ)
	}
	return nil
}

func (o *operator) publishUser(ctx context.Context, typ mq.EventType, user *iamv1.User) {
	if o.mq != nil {
		event := mq.NewEvent(typ, iamv1.GroupVersion, "User", "", user.Metadata.Name, user.Metadata.Generation, user.Metadata.ResourceVersion)
		event.Attributes = map[string]string{"uid": user.Metadata.UID}
		_ = o.mq.Publish(ctx, mq.TopicUser, event)
	}
}
