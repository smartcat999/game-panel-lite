package v1

import (
	"github.com/smartcat999/game-panel-cloud/pkg/client/rest"
)

// CoreV1Interface provides typed access to core.gamepanel.io/v1 resources.
type CoreV1Interface interface {
	Tenants() TenantInterface
	RuntimeClasses() RuntimeClassInterface
	Apps() AppInterface
	PrewarmJobs() PrewarmJobInterface
	Nodes() NodeInterface
	Instances(tenant string) InstanceInterface
	Metering(tenant string) MeteringInterface
}

// CoreV1Client implements CoreV1Interface.
type CoreV1Client struct {
	rest rest.Interface
}

// New constructs a new CoreV1Client backed by a generic REST client.
func New(r rest.Interface) *CoreV1Client {
	return &CoreV1Client{rest: r}
}

func (c *CoreV1Client) Tenants() TenantInterface {
	return newTenants(c.rest)
}

func (c *CoreV1Client) RuntimeClasses() RuntimeClassInterface {
	return newRuntimeClasses(c.rest)
}

func (c *CoreV1Client) Apps() AppInterface {
	return newApps(c.rest)
}

func (c *CoreV1Client) PrewarmJobs() PrewarmJobInterface {
	return newPrewarmJobs(c.rest)
}

func (c *CoreV1Client) Nodes() NodeInterface {
	return newNodes(c.rest)
}

func (c *CoreV1Client) Instances(tenant string) InstanceInterface {
	return newInstances(c.rest, tenant)
}

func (c *CoreV1Client) Metering(tenant string) MeteringInterface {
	return newMetering(c.rest, tenant)
}
