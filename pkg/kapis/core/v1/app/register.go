package app

import (
	"net/http"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	apiruntime "github.com/smartcat999/game-panel-cloud/pkg/apiserver/runtime"
	appmodel "github.com/smartcat999/game-panel-cloud/pkg/models/core/app"
)

func init() {
	apiruntime.Register("core.gamepanel.io/v1/apps", AddToContainer)
}

func AddToContainer(deps apiruntime.Dependencies, c *apiruntime.Container) {
	h := newHandler(appmodel.NewOperator(deps.DB, deps.MQ, deps.Cache), deps.MQ)
	ws := apiruntime.NewWebService(corev1.GroupVersion, "App")

	ws.Route(ws.GET("/apps").
		To(h.ListApps).
		Doc("List cluster-scoped game Apps in the marketplace").
		Param(ws.QueryParameter("watch", "Watch for real-time resource changes (true/false)").DataType("boolean")).
		WithListOptions().
		Writes(metav1.List[*corev1.App]{}).
		Returns(http.StatusOK, "OK", metav1.List[*corev1.App]{}))

	ws.Route(ws.POST("/apps").
		To(h.CreateApp).
		Doc("Create a cluster-scoped game App in the marketplace").
		Reads(corev1.App{}).
		Writes(corev1.App{}).
		Returns(http.StatusCreated, "Created", corev1.App{}))

	ws.Route(ws.GET("/apps/{name}").
		To(h.GetApp).
		Doc("Get a game App by name").
		Param(ws.PathParameter("name", "App name")).
		Writes(corev1.App{}).
		Returns(http.StatusOK, "OK", corev1.App{}))

	ws.Route(ws.PUT("/apps/{name}").
		To(h.UpdateApp).
		Doc("Replace a game App by name").
		Param(ws.PathParameter("name", "App name")).
		Reads(corev1.App{}).
		Writes(corev1.App{}).
		Returns(http.StatusOK, "OK", corev1.App{}))

	ws.Route(ws.PATCH("/apps/{name}").
		To(h.PatchApp).
		Doc("Partially update a game App (JSON Merge Patch)").
		Param(ws.PathParameter("name", "App name")).
		Reads(corev1.App{}).
		Writes(corev1.App{}).
		Returns(http.StatusOK, "OK", corev1.App{}))

	ws.Route(ws.DELETE("/apps/{name}").
		To(h.DeleteApp).
		Doc("Delete a game App by name").
		Param(ws.PathParameter("name", "App name")).
		Returns(http.StatusNoContent, "No Content", nil))

	c.Add(ws)
}
