package v1

import (
	"github.com/smartcat999/game-panel-cloud/pkg/client/rest"
)

// IAMV1Interface provides typed access to iam.gamepanel.io/v1 resources.
type IAMV1Interface interface {
	Sessions() SessionInterface
	Users() UserInterface
	GlobalRoles() GlobalRoleInterface
	GlobalRoleBindings() GlobalRoleBindingInterface
	TenantRoles(tenant string) TenantRoleInterface
	TenantRoleBindings(tenant string) TenantRoleBindingInterface
}

// IAMV1Client implements IAMV1Interface.
type IAMV1Client struct {
	rest rest.Interface
}

// New constructs a new IAMV1Client backed by a generic REST client.
func New(r rest.Interface) *IAMV1Client {
	return &IAMV1Client{rest: r}
}

func (c *IAMV1Client) Sessions() SessionInterface {
	return newSessions(c.rest)
}

func (c *IAMV1Client) Users() UserInterface {
	return newUsers(c.rest)
}

func (c *IAMV1Client) GlobalRoles() GlobalRoleInterface {
	return newGlobalRoles(c.rest)
}

func (c *IAMV1Client) GlobalRoleBindings() GlobalRoleBindingInterface {
	return newGlobalRoleBindings(c.rest)
}

func (c *IAMV1Client) TenantRoles(tenant string) TenantRoleInterface {
	return newTenantRoles(c.rest, tenant)
}

func (c *IAMV1Client) TenantRoleBindings(tenant string) TenantRoleBindingInterface {
	return newTenantRoleBindings(c.rest, tenant)
}
