package repository

import (
	"net/http"

	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	apiruntime "github.com/smartcat999/game-panel-cloud/pkg/apiserver/runtime"
	repositorymodel "github.com/smartcat999/game-panel-cloud/pkg/models/registry/repository"
)

func init() {
	apiruntime.Register("registry.gamepanel.io/v1/repositories", AddToContainer)
}

func AddToContainer(deps apiruntime.Dependencies, c *apiruntime.Container) {
	h := &handler{op: repositorymodel.NewOperator(deps.DB, deps.MQ), mq: deps.MQ}
	ws := apiruntime.NewWebService(registryv1.GroupVersion, registryv1.KindRepository)

	ws.Route(ws.GET("/repositories").To(h.ListRepositories).Doc("List all Registry Repositories").
		Param(ws.QueryParameter("watch", "Watch for real-time changes").DataType("boolean")).
		WithListOptions().Writes(metav1.List[*registryv1.Repository]{}).
		Returns(http.StatusOK, "OK", metav1.List[*registryv1.Repository]{}))

	ws.Route(ws.POST("/repositories").To(h.CreateRepository).Doc("Create a Registry Repository").
		Reads(registryv1.Repository{}).Writes(registryv1.Repository{}).
		Returns(http.StatusCreated, "Created", registryv1.Repository{}))

	ws.Route(ws.GET("/repositories/{name}").To(h.GetRepository).Doc("Get a Registry Repository by name").
		Param(ws.PathParameter("name", "Repository name")).Writes(registryv1.Repository{}).
		Returns(http.StatusOK, "OK", registryv1.Repository{}))

	ws.Route(ws.PUT("/repositories/{name}").To(h.UpdateRepository).Doc("Replace a Registry Repository spec").
		Param(ws.PathParameter("name", "Repository name")).Reads(registryv1.Repository{}).Writes(registryv1.Repository{}).
		Returns(http.StatusOK, "OK", registryv1.Repository{}))

	ws.Route(ws.PATCH("/repositories/{name}").To(h.PatchRepository).Doc("Patch a Registry Repository").
		Param(ws.PathParameter("name", "Repository name")).Reads(registryv1.Repository{}).Writes(registryv1.Repository{}).
		Returns(http.StatusOK, "OK", registryv1.Repository{}))

	ws.Route(ws.DELETE("/repositories/{name}").To(h.DeleteRepository).Doc("Delete a Registry Repository").
		Param(ws.PathParameter("name", "Repository name")).Returns(http.StatusAccepted, "Accepted", nil))

	ws.Route(ws.GET("/repositories/{name}/status").To(h.GetRepository).Doc("Get Repository status").
		Param(ws.PathParameter("name", "Repository name")).Writes(registryv1.Repository{}).
		Returns(http.StatusOK, "OK", registryv1.Repository{}))

	ws.Route(ws.PUT("/repositories/{name}/status").To(h.UpdateRepositoryStatus).Doc("Replace Repository status").
		Param(ws.PathParameter("name", "Repository name")).Reads(registryv1.Repository{}).Writes(registryv1.Repository{}).
		Returns(http.StatusOK, "OK", registryv1.Repository{}))

	ws.Route(ws.PATCH("/repositories/{name}/status").To(h.UpdateRepositoryStatus).Doc("Patch Repository status").
		Param(ws.PathParameter("name", "Repository name")).Reads(registryv1.Repository{}).Writes(registryv1.Repository{}).
		Returns(http.StatusOK, "OK", registryv1.Repository{}))

	c.Add(ws)
}
