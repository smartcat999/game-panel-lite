package blob

import (
	"net/http"

	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	apiruntime "github.com/smartcat999/game-panel-cloud/pkg/apiserver/runtime"
	blobmodel "github.com/smartcat999/game-panel-cloud/pkg/models/registry/blob"
)

func init() {
	apiruntime.Register("registry.gamepanel.io/v1/blobs", AddToContainer)
}

func AddToContainer(deps apiruntime.Dependencies, c *apiruntime.Container) {
	h := &handler{op: blobmodel.NewOperator(deps.DB, nil)}
	ws := apiruntime.NewWebService(registryv1.GroupVersion, registryv1.KindBlob)

	ws.Route(ws.GET("/blobs").To(h.ListBlobs).Doc("List content-addressed Blobs").
		WithListOptions().Writes(metav1.List[*registryv1.Blob]{}).
		Returns(http.StatusOK, "OK", metav1.List[*registryv1.Blob]{}))

	ws.Route(ws.POST("/blobs").To(h.RegisterBlob).Doc("Register a content-addressed Blob metadata record").
		Reads(registryv1.Blob{}).Writes(registryv1.Blob{}).
		Returns(http.StatusCreated, "Created", registryv1.Blob{}))

	ws.Route(ws.GET("/blobs/{name}").To(h.GetBlob).Doc("Redirect (307) to S3/CDN presigned Blob URL or inspect metadata").
		Param(ws.PathParameter("name", "SHA-256 digest (sha256:<hex>)")).
		Param(ws.QueryParameter("metadata", "Return JSON metadata instead of 307 redirect").DataType("boolean")).
		Writes(registryv1.Blob{}).
		Returns(http.StatusTemporaryRedirect, "Temporary Redirect", nil).
		Returns(http.StatusOK, "OK", registryv1.Blob{}))

	ws.Route(ws.DELETE("/blobs/{name}").To(h.DeleteBlob).Doc("Delete an unreferenced Blob").
		Param(ws.PathParameter("name", "SHA-256 digest (sha256:<hex>)")).
		Returns(http.StatusNoContent, "No Content", nil))

	c.Add(ws)
}
