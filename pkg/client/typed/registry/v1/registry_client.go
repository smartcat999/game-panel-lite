package v1

import (
	"github.com/smartcat999/game-panel-cloud/pkg/client/rest"
)

// RegistryV1Interface provides typed access to registry.gamepanel.io/v1 resources.
type RegistryV1Interface interface {
	Upstreams() UpstreamInterface
	Repositories() RepositoryInterface
	Packages() PackageInterface
	Artifacts() ArtifactInterface
	Blobs() BlobInterface
	BlobUploads() BlobUploadInterface
	ReplicationPolicies() ReplicationPolicyInterface
}

// RegistryV1Client implements RegistryV1Interface.
type RegistryV1Client struct {
	rest rest.Interface
}

// New constructs a new RegistryV1Client backed by a generic REST client.
func New(r rest.Interface) *RegistryV1Client {
	return &RegistryV1Client{rest: r}
}

func (c *RegistryV1Client) Upstreams() UpstreamInterface {
	return newUpstreams(c.rest)
}

func (c *RegistryV1Client) Repositories() RepositoryInterface {
	return newRepositories(c.rest)
}

func (c *RegistryV1Client) Packages() PackageInterface {
	return newPackages(c.rest)
}

func (c *RegistryV1Client) Artifacts() ArtifactInterface {
	return newArtifacts(c.rest)
}

func (c *RegistryV1Client) Blobs() BlobInterface {
	return newBlobs(c.rest)
}

func (c *RegistryV1Client) BlobUploads() BlobUploadInterface {
	return newBlobUploads(c.rest)
}

func (c *RegistryV1Client) ReplicationPolicies() ReplicationPolicyInterface {
	return newReplicationPolicies(c.rest)
}
