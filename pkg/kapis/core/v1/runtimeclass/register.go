package runtimeclass

import (
	"net/http"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	apiruntime "github.com/smartcat999/game-panel-cloud/pkg/apiserver/runtime"
	runtimeclassmodel "github.com/smartcat999/game-panel-cloud/pkg/models/core/runtimeclass"
)

func init() {
	apiruntime.Register("core.gamepanel.io/v1/runtimeclasses", AddToContainer)
}

func AddToContainer(deps apiruntime.Dependencies, c *apiruntime.Container) {
	h := newHandler(runtimeclassmodel.NewOperator(deps.DB, deps.MQ), deps.MQ)
	ws := apiruntime.NewWebService(corev1.GroupVersion, "RuntimeClass")

	ws.Route(ws.GET("/runtimeclasses").
		To(h.ListRuntimeClasses).
		Doc("List cluster-scoped RuntimeClasses").
		Param(ws.QueryParameter("watch", "Watch for real-time resource changes (true/false)").DataType("boolean")).
		WithListOptions().
		Writes(metav1.List[*corev1.RuntimeClass]{}).
		Returns(http.StatusOK, "OK", metav1.List[*corev1.RuntimeClass]{}))

	ws.Route(ws.POST("/runtimeclasses").
		To(h.CreateRuntimeClass).
		Doc("Create a cluster-scoped RuntimeClass").
		Reads(corev1.RuntimeClass{}).
		Writes(corev1.RuntimeClass{}).
		Returns(http.StatusCreated, "Created", corev1.RuntimeClass{}))

	ws.Route(ws.GET("/runtimeclasses/{name}").
		To(h.GetRuntimeClass).
		Doc("Get a RuntimeClass by name").
		Param(ws.PathParameter("name", "RuntimeClass name")).
		Writes(corev1.RuntimeClass{}).
		Returns(http.StatusOK, "OK", corev1.RuntimeClass{}))

	ws.Route(ws.PUT("/runtimeclasses/{name}").
		To(h.UpdateRuntimeClass).
		Doc("Replace a RuntimeClass by name").
		Param(ws.PathParameter("name", "RuntimeClass name")).
		Reads(corev1.RuntimeClass{}).
		Writes(corev1.RuntimeClass{}).
		Returns(http.StatusOK, "OK", corev1.RuntimeClass{}))

	ws.Route(ws.PATCH("/runtimeclasses/{name}").
		To(h.PatchRuntimeClass).
		Doc("Partially update a RuntimeClass (JSON Merge Patch)").
		Param(ws.PathParameter("name", "RuntimeClass name")).
		Reads(corev1.RuntimeClass{}).
		Writes(corev1.RuntimeClass{}).
		Returns(http.StatusOK, "OK", corev1.RuntimeClass{}))

	ws.Route(ws.DELETE("/runtimeclasses/{name}").
		To(h.DeleteRuntimeClass).
		Doc("Delete a RuntimeClass by name").
		Param(ws.PathParameter("name", "RuntimeClass name")).
		Returns(http.StatusNoContent, "No Content", nil))

	c.Add(ws)
}
