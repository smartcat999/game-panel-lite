package tenantrole

import (
	"net/http"

	iamv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/iam/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	apiruntime "github.com/smartcat999/game-panel-cloud/pkg/apiserver/runtime"
	tenantrolemodel "github.com/smartcat999/game-panel-cloud/pkg/models/iam/tenantrole"
)

func init() {
	apiruntime.Register("iam.gamepanel.io/v1/tenantroles", AddToContainer)
}

func AddToContainer(deps apiruntime.Dependencies, c *apiruntime.Container) {
	h := newHandler(tenantrolemodel.NewOperator(deps.DB, deps.MQ), deps.MQ)
	ws := apiruntime.NewWebService(iamv1.GroupVersion, "TenantRole")

	ws.Route(ws.GET("/tenantroles").To(h.ListAllTenantRoles).Doc("List all TenantRoles across the cluster").
		Param(ws.QueryParameter("watch", "Watch for real-time resource changes (true/false)").DataType("boolean")).
		WithListOptions().Writes(metav1.List[*iamv1.TenantRole]{}).
		Returns(http.StatusOK, "OK", metav1.List[*iamv1.TenantRole]{}))

	ws.Route(ws.GET("/tenants/{tenant}/tenantroles").To(h.ListTenantRoles).Doc("List TenantRoles in a Tenant").
		Param(ws.PathParameter("tenant", "Tenant name")).
		Param(ws.QueryParameter("watch", "Watch for real-time resource changes (true/false)").DataType("boolean")).
		WithListOptions().Writes(metav1.List[*iamv1.TenantRole]{}).
		Returns(http.StatusOK, "OK", metav1.List[*iamv1.TenantRole]{}))

	ws.Route(ws.POST("/tenants/{tenant}/tenantroles").To(h.CreateTenantRole).Doc("Create a TenantRole in a Tenant").
		Param(ws.PathParameter("tenant", "Tenant name")).Reads(iamv1.TenantRole{}).Writes(iamv1.TenantRole{}).
		Returns(http.StatusCreated, "Created", iamv1.TenantRole{}))

	ws.Route(ws.GET("/tenants/{tenant}/tenantroles/{name}").To(h.GetTenantRole).Doc("Get a TenantRole by name").
		Param(ws.PathParameter("tenant", "Tenant name")).Param(ws.PathParameter("name", "TenantRole name")).
		Writes(iamv1.TenantRole{}).Returns(http.StatusOK, "OK", iamv1.TenantRole{}))

	ws.Route(ws.PUT("/tenants/{tenant}/tenantroles/{name}").To(h.UpdateTenantRole).Doc("Replace a TenantRole by name").
		Param(ws.PathParameter("tenant", "Tenant name")).Param(ws.PathParameter("name", "TenantRole name")).
		Reads(iamv1.TenantRole{}).Writes(iamv1.TenantRole{}).Returns(http.StatusOK, "OK", iamv1.TenantRole{}))

	ws.Route(ws.PATCH("/tenants/{tenant}/tenantroles/{name}").To(h.PatchTenantRole).
		Doc("Partially update a TenantRole (JSON Merge Patch)").
		Param(ws.PathParameter("tenant", "Tenant name")).Param(ws.PathParameter("name", "TenantRole name")).
		Reads(iamv1.TenantRole{}).Writes(iamv1.TenantRole{}).Returns(http.StatusOK, "OK", iamv1.TenantRole{}))

	ws.Route(ws.DELETE("/tenants/{tenant}/tenantroles/{name}").To(h.DeleteTenantRole).Doc("Delete a TenantRole by name").
		Param(ws.PathParameter("tenant", "Tenant name")).Param(ws.PathParameter("name", "TenantRole name")).
		Returns(http.StatusNoContent, "No Content", nil))

	c.Add(ws)
}
