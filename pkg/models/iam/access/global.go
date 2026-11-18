// Package access provides model-layer RBAC checks for sensitive operations that
// require global privileges even when a tenant role permits the HTTP request.
package access

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/sync/singleflight"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	iamv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/iam/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/models"
	"github.com/smartcat999/game-panel-cloud/pkg/repo"
	tenantdb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/tenant"
	globalroledb "github.com/smartcat999/game-panel-cloud/pkg/repo/iam/globalrole"
	globalrolebindingdb "github.com/smartcat999/game-panel-cloud/pkg/repo/iam/globalrolebinding"
	userdb "github.com/smartcat999/game-panel-cloud/pkg/repo/iam/user"
)

var globalAccessFlight singleflight.Group

// SubjectFilter builds the comma-separated subject selector ("*,<user>,<group1>,...")
// used by the indexed subjects column in GlobalRoleBinding and TenantRoleBinding queries.
func SubjectFilter(u iamv1.UserInfo) string {
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

// SubjectMatches reports whether any subject in subjects matches user.
func SubjectMatches(subjects []iamv1.Subject, user iamv1.UserInfo) bool {
	for _, subject := range subjects {
		if subject.Kind == iamv1.SubjectKindGroup {
			for _, group := range user.Groups {
				if subject.Name == "*" || subject.Name == group {
					return true
				}
			}
			continue
		}
		if (subject.Kind == "" || subject.Kind == user.Kind) && (subject.Name == "*" || subject.Name == user.Name) {
			return true
		}
	}
	return false
}

// AllowsGlobal evaluates only GlobalRoleBindings and GlobalRoles. Tenant role
// wildcards cannot grant a platform-owned privilege. Missing stores fail closed.
func AllowsGlobal(ctx context.Context, reg *repo.Registry, user iamv1.UserInfo, apiGroup, resource, verb, name string, labels map[string]string) (bool, error) {
	if user.IsAnonymous() || reg == nil || reg.Pool() == nil {
		return false, nil
	}
	bindings, err := loadUserGlobalBindings(ctx, reg, user)
	if err != nil {
		return false, err
	}
	seenRoles := make(map[string]struct{}, len(bindings))
	for _, binding := range bindings {
		if binding == nil || !SubjectMatches(binding.Subjects, user) {
			continue
		}
		roleName := strings.TrimSpace(binding.RoleRef.Name)
		if roleName == "" {
			continue
		}
		if _, seen := seenRoles[roleName]; seen {
			continue
		}
		seenRoles[roleName] = struct{}{}
		role, err := loadGlobalRole(ctx, reg, roleName)
		if err != nil {
			return false, err
		}
		if role == nil {
			continue
		}
		for _, rule := range role.Rules {
			if rule.Matches(apiGroup, resource, verb, name, labels) {
				return true, nil
			}
		}
	}
	return false, nil
}

func loadUserGlobalBindings(ctx context.Context, reg *repo.Registry, user iamv1.UserInfo) ([]*iamv1.GlobalRoleBinding, error) {
	filter := SubjectFilter(user)
	key := fmt.Sprintf("grb:%p:%s", reg.Pool(), filter)
	val, err, _ := globalAccessFlight.Do(key, func() (any, error) {
		bindingDB := globalrolebindingdb.NewDB(reg.Pool())
		list, lErr := bindingDB.List(ctx, db.ListOptions{
			FieldSelector: map[string]string{"subject": filter},
		})
		if lErr != nil {
			return nil, fmt.Errorf("list global role bindings: %w", lErr)
		}
		if list == nil {
			return ([]*iamv1.GlobalRoleBinding)(nil), nil
		}
		return list.Items, nil
	})
	if err != nil {
		return nil, err
	}
	items, _ := val.([]*iamv1.GlobalRoleBinding)
	return items, nil
}

func loadGlobalRole(ctx context.Context, reg *repo.Registry, roleName string) (*iamv1.GlobalRole, error) {
	key := fmt.Sprintf("gr:%p:%s", reg.Pool(), roleName)
	val, err, _ := globalAccessFlight.Do(key, func() (any, error) {
		roleDB := globalroledb.NewDB(reg.Pool())
		role, gErr := roleDB.Get(ctx, roleName)
		if errors.Is(gErr, db.ErrNotFound) {
			return (*iamv1.GlobalRole)(nil), nil
		}
		if gErr != nil {
			return nil, fmt.Errorf("get global role %q: %w", roleName, gErr)
		}
		return role, nil
	})
	if err != nil {
		return nil, err
	}
	role, _ := val.(*iamv1.GlobalRole)
	return role, nil
}

// ActiveTenant returns the named Tenant if it exists and is not terminating.
func ActiveTenant(ctx context.Context, reg *repo.Registry, tenantName string) (*corev1.Tenant, error) {
	name := strings.TrimSpace(tenantName)
	if name == "" {
		return nil, fmt.Errorf("%w: tenant is required", models.ErrBadRequest)
	}
	if reg == nil || reg.Pool() == nil {
		return nil, fmt.Errorf("%w: tenant %q not found", models.ErrNotFound, name)
	}
	tenantDB := tenantdb.NewDB(reg.Pool())
	tenant, err := tenantDB.Get(ctx, name)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil, fmt.Errorf("%w: tenant %q not found", models.ErrNotFound, name)
		}
		return nil, err
	}
	if tenant.Metadata.DeletionTimestamp != nil || tenant.Status.Phase == corev1.TenantPhaseTerminating {
		return nil, fmt.Errorf("%w: tenant %q is terminating", models.ErrConflict, name)
	}
	return tenant, nil
}

// ValidateActiveUsers verifies that every explicit User subject exists and is not terminating or disabled.
func ValidateActiveUsers(ctx context.Context, reg *repo.Registry, subjects []iamv1.Subject) error {
	if reg == nil || reg.Pool() == nil {
		return nil
	}
	userDB := userdb.NewDB(reg.Pool())
	for _, s := range subjects {
		if s.Kind != "" && s.Kind != iamv1.SubjectKindUser {
			continue
		}
		name := strings.TrimSpace(s.Name)
		if name == "" || name == "*" {
			continue
		}
		u, err := userDB.Get(ctx, name)
		if err != nil {
			if errors.Is(err, db.ErrNotFound) {
				return fmt.Errorf("%w: user %q not found", models.ErrNotFound, name)
			}
			return err
		}
		if u.Metadata.DeletionTimestamp != nil || u.Spec.State == iamv1.UserStateDisabled {
			return fmt.Errorf("%w: user %q is disabled or terminating", models.ErrConflict, name)
		}
	}
	return nil
}
