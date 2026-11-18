package globalrolebinding

import (
	"net/http"

	iamv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/iam/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	apiruntime "github.com/smartcat999/game-panel-cloud/pkg/apiserver/runtime"
	globalrolebindingmodel "github.com/smartcat999/game-panel-cloud/pkg/models/iam/globalrolebinding"
)

func init() {
	apiruntime.Register("iam.gamepanel.io/v1/globalrolebindings", AddToContainer)
}

func AddToContainer(deps apiruntime.Dependencies, c *apiruntime.Container) {
	h := newHandler(globalrolebindingmodel.NewOperator(deps.DB, deps.MQ), deps.MQ)
	ws := apiruntime.NewWebService(iamv1.GroupVersion, "GlobalRoleBinding")

	ws.Route(ws.GET("/globalrolebindings").
		To(h.ListGlobalRoleBindings).
		Doc("List cluster-scoped GlobalRoleBindings").
		Param(ws.QueryParameter("watch", "Watch for real-time resource changes (true/false)").DataType("boolean")).
		WithListOptions().
		Writes(metav1.List[*iamv1.GlobalRoleBinding]{}).
		Returns(http.StatusOK, "OK", metav1.List[*iamv1.GlobalRoleBinding]{}))

	ws.Route(ws.POST("/globalrolebindings").
		To(h.CreateGlobalRoleBinding).
		Doc("Create a cluster-scoped GlobalRoleBinding").
		Reads(iamv1.GlobalRoleBinding{}).
		Writes(iamv1.GlobalRoleBinding{}).
		Returns(http.StatusCreated, "Created", iamv1.GlobalRoleBinding{}))

	ws.Route(ws.GET("/globalrolebindings/{name}").
		To(h.GetGlobalRoleBinding).
		Doc("Get a GlobalRoleBinding by name").
		Param(ws.PathParameter("name", "GlobalRoleBinding name")).
		Writes(iamv1.GlobalRoleBinding{}).
		Returns(http.StatusOK, "OK", iamv1.GlobalRoleBinding{}))

	ws.Route(ws.PUT("/globalrolebindings/{name}").
		To(h.UpdateGlobalRoleBinding).
		Doc("Replace a GlobalRoleBinding by name").
		Param(ws.PathParameter("name", "GlobalRoleBinding name")).
		Reads(iamv1.GlobalRoleBinding{}).
		Writes(iamv1.GlobalRoleBinding{}).
		Returns(http.StatusOK, "OK", iamv1.GlobalRoleBinding{}))

	ws.Route(ws.PATCH("/globalrolebindings/{name}").
		To(h.PatchGlobalRoleBinding).
		Doc("Partially update a GlobalRoleBinding (JSON Merge Patch)").
		Param(ws.PathParameter("name", "GlobalRoleBinding name")).
		Reads(iamv1.GlobalRoleBinding{}).
		Writes(iamv1.GlobalRoleBinding{}).
		Returns(http.StatusOK, "OK", iamv1.GlobalRoleBinding{}))

	ws.Route(ws.DELETE("/globalrolebindings/{name}").
		To(h.DeleteGlobalRoleBinding).
		Doc("Delete a GlobalRoleBinding by name").
		Param(ws.PathParameter("name", "GlobalRoleBinding name")).
		Returns(http.StatusNoContent, "No Content", nil))

	c.Add(ws)
}
