package blobupload

import (
	"net/http"

	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	apiruntime "github.com/smartcat999/game-panel-cloud/pkg/apiserver/runtime"
	blobuploadmodel "github.com/smartcat999/game-panel-cloud/pkg/models/registry/blobupload"
)

func init() {
	apiruntime.Register("registry.gamepanel.io/v1/blobuploads", AddToContainer)
}

func AddToContainer(deps apiruntime.Dependencies, c *apiruntime.Container) {
	h := &handler{op: blobuploadmodel.NewOperator(deps.DB, nil)}
	ws := apiruntime.NewWebService(registryv1.GroupVersion, registryv1.KindBlobUpload)

	ws.Route(ws.GET("/blobuploads").To(h.ListBlobUploads).Doc("List active and completed BlobUploads").
		WithListOptions().Writes(metav1.List[*registryv1.BlobUpload]{}).
		Returns(http.StatusOK, "OK", metav1.List[*registryv1.BlobUpload]{}))

	ws.Route(ws.POST("/blobuploads").To(h.CreateBlobUpload).Doc("Initiate a direct S3 presigned BlobUpload session").
		Reads(registryv1.BlobUpload{}).Writes(registryv1.BlobUpload{}).
		Returns(http.StatusCreated, "Created", registryv1.BlobUpload{}))

	ws.Route(ws.GET("/blobuploads/{name}").To(h.GetBlobUpload).Doc("Get a BlobUpload session").
		Param(ws.PathParameter("name", "BlobUpload session name")).Writes(registryv1.BlobUpload{}).
		Returns(http.StatusOK, "OK", registryv1.BlobUpload{}))

	ws.Route(ws.PUT("/blobuploads/{name}").To(h.CompleteBlobUpload).Doc("Complete a BlobUpload and commit its immutable Blob").
		Param(ws.PathParameter("name", "BlobUpload session name")).
		Reads(registryv1.BlobUpload{}).Writes(registryv1.BlobUpload{}).
		Returns(http.StatusOK, "OK", registryv1.BlobUpload{}))

	ws.Route(ws.DELETE("/blobuploads/{name}").To(h.DeleteBlobUpload).Doc("Abort and delete a BlobUpload session").
		Param(ws.PathParameter("name", "BlobUpload session name")).
		Returns(http.StatusNoContent, "No Content", nil))

	c.Add(ws)
}
