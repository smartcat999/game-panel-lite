package resourceplan

import (
	"net/http"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	apiruntime "github.com/smartcat999/game-panel-cloud/pkg/apiserver/runtime"
	resourceplanmodel "github.com/smartcat999/game-panel-cloud/pkg/models/core/resourceplan"
)

func init() {
	apiruntime.Register("core.gamepanel.io/v1/resourceplans", AddToContainer)
}

// AddToContainer registers cluster-scoped ResourcePlan routes into the API container.
func AddToContainer(deps apiruntime.Dependencies, c *apiruntime.Container) {
	h := newHandler(resourceplanmodel.NewOperator(deps.DB, deps.MQ), deps.MQ)
	ws := apiruntime.NewWebService(corev1.GroupVersion, corev1.KindResourcePlan)

	ws.Route(ws.GET("/resourceplans").
		To(h.ListResourcePlans).
		Doc("List cluster-scoped ResourcePlans").
		Param(ws.QueryParameter("watch", "Watch for real-time resource changes (true/false)").DataType("boolean")).
		WithListOptions().
		Writes(metav1.List[*corev1.ResourcePlan]{}).
		Returns(http.StatusOK, "OK", metav1.List[*corev1.ResourcePlan]{}))

	ws.Route(ws.POST("/resourceplans").
		To(h.CreateResourcePlan).
		Doc("Create a cluster-scoped ResourcePlan").
		Reads(corev1.ResourcePlan{}).
		Writes(corev1.ResourcePlan{}).
		Returns(http.StatusCreated, "Created", corev1.ResourcePlan{}))

	ws.Route(ws.GET("/resourceplans/{resourceplan}").
		To(h.GetResourcePlan).
		Doc("Get a ResourcePlan by name").
		Param(ws.PathParameter("resourceplan", "ResourcePlan name")).
		Writes(corev1.ResourcePlan{}).
		Returns(http.StatusOK, "OK", corev1.ResourcePlan{}))

	ws.Route(ws.PUT("/resourceplans/{resourceplan}").
		To(h.UpdateResourcePlan).
		Doc("Replace a ResourcePlan by name").
		Param(ws.PathParameter("resourceplan", "ResourcePlan name")).
		Reads(corev1.ResourcePlan{}).
		Writes(corev1.ResourcePlan{}).
		Returns(http.StatusOK, "OK", corev1.ResourcePlan{}))

	ws.Route(ws.PATCH("/resourceplans/{resourceplan}").
		To(h.PatchResourcePlan).
		Doc("Partially update a ResourcePlan (JSON Merge Patch)").
		Param(ws.PathParameter("resourceplan", "ResourcePlan name")).
		Reads(corev1.ResourcePlan{}).
		Writes(corev1.ResourcePlan{}).
		Returns(http.StatusOK, "OK", corev1.ResourcePlan{}))

	ws.Route(ws.DELETE("/resourceplans/{resourceplan}").
		To(h.DeleteResourcePlan).
		Doc("Delete a ResourcePlan by name").
		Param(ws.PathParameter("resourceplan", "ResourcePlan name")).
		Returns(http.StatusNoContent, "No Content", nil))

	c.Add(ws)
}
