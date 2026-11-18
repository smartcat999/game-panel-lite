package globalrole

import (
	"net/http"

	iamv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/iam/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	apiruntime "github.com/smartcat999/game-panel-cloud/pkg/apiserver/runtime"
	globalrolemodel "github.com/smartcat999/game-panel-cloud/pkg/models/iam/globalrole"
)

func init() {
	apiruntime.Register("iam.gamepanel.io/v1/globalroles", AddToContainer)
}

func AddToContainer(deps apiruntime.Dependencies, c *apiruntime.Container) {
	h := newHandler(globalrolemodel.NewOperator(deps.DB, deps.MQ), deps.MQ)
	ws := apiruntime.NewWebService(iamv1.GroupVersion, "GlobalRole")

	ws.Route(ws.GET("/globalroles").
		To(h.ListGlobalRoles).
		Doc("List cluster-scoped GlobalRoles").
		Param(ws.QueryParameter("watch", "Watch for real-time resource changes (true/false)").DataType("boolean")).
		WithListOptions().
		Writes(metav1.List[*iamv1.GlobalRole]{}).
		Returns(http.StatusOK, "OK", metav1.List[*iamv1.GlobalRole]{}))

	ws.Route(ws.POST("/globalroles").
		To(h.CreateGlobalRole).
		Doc("Create a cluster-scoped GlobalRole").
		Reads(iamv1.GlobalRole{}).
		Writes(iamv1.GlobalRole{}).
		Returns(http.StatusCreated, "Created", iamv1.GlobalRole{}))

	ws.Route(ws.GET("/globalroles/{name}").
		To(h.GetGlobalRole).
		Doc("Get a GlobalRole by name").
		Param(ws.PathParameter("name", "GlobalRole name")).
		Writes(iamv1.GlobalRole{}).
		Returns(http.StatusOK, "OK", iamv1.GlobalRole{}))

	ws.Route(ws.PUT("/globalroles/{name}").
		To(h.UpdateGlobalRole).
		Doc("Replace a GlobalRole by name").
		Param(ws.PathParameter("name", "GlobalRole name")).
		Reads(iamv1.GlobalRole{}).
		Writes(iamv1.GlobalRole{}).
		Returns(http.StatusOK, "OK", iamv1.GlobalRole{}))

	ws.Route(ws.PATCH("/globalroles/{name}").
		To(h.PatchGlobalRole).
		Doc("Partially update a GlobalRole (JSON Merge Patch)").
		Param(ws.PathParameter("name", "GlobalRole name")).
		Reads(iamv1.GlobalRole{}).
		Writes(iamv1.GlobalRole{}).
		Returns(http.StatusOK, "OK", iamv1.GlobalRole{}))

	ws.Route(ws.DELETE("/globalroles/{name}").
		To(h.DeleteGlobalRole).
		Doc("Delete a GlobalRole by name").
		Param(ws.PathParameter("name", "GlobalRole name")).
		Returns(http.StatusNoContent, "No Content", nil))

	c.Add(ws)
}
