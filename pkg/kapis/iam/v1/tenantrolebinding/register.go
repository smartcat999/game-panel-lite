package tenantrolebinding

import (
	"net/http"

	iamv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/iam/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	apiruntime "github.com/smartcat999/game-panel-cloud/pkg/apiserver/runtime"
	tenantrolebindingmodel "github.com/smartcat999/game-panel-cloud/pkg/models/iam/tenantrolebinding"
)

func init() {
	apiruntime.Register("iam.gamepanel.io/v1/tenantrolebindings", AddToContainer)
}

func AddToContainer(deps apiruntime.Dependencies, c *apiruntime.Container) {
	h := newHandler(tenantrolebindingmodel.NewOperator(deps.DB, deps.MQ), deps.MQ)
	ws := apiruntime.NewWebService(iamv1.GroupVersion, "TenantRoleBinding")

	ws.Route(ws.GET("/tenantrolebindings").To(h.ListAllTenantRoleBindings).
		Doc("List all TenantRoleBindings across the cluster").
		Param(ws.QueryParameter("watch", "Watch for real-time resource changes (true/false)").DataType("boolean")).
		WithListOptions().Writes(metav1.List[*iamv1.TenantRoleBinding]{}).
		Returns(http.StatusOK, "OK", metav1.List[*iamv1.TenantRoleBinding]{}))

	ws.Route(ws.GET("/tenants/{tenant}/tenantrolebindings").To(h.ListTenantRoleBindings).
		Doc("List TenantRoleBindings in a Tenant").Param(ws.PathParameter("tenant", "Tenant name")).
		Param(ws.QueryParameter("watch", "Watch for real-time resource changes (true/false)").DataType("boolean")).
		WithListOptions().Writes(metav1.List[*iamv1.TenantRoleBinding]{}).
		Returns(http.StatusOK, "OK", metav1.List[*iamv1.TenantRoleBinding]{}))

	ws.Route(ws.POST("/tenants/{tenant}/tenantrolebindings").To(h.CreateTenantRoleBinding).
		Doc("Create a TenantRoleBinding in a Tenant").Param(ws.PathParameter("tenant", "Tenant name")).
		Reads(iamv1.TenantRoleBinding{}).Writes(iamv1.TenantRoleBinding{}).
		Returns(http.StatusCreated, "Created", iamv1.TenantRoleBinding{}))

	ws.Route(ws.GET("/tenants/{tenant}/tenantrolebindings/{name}").To(h.GetTenantRoleBinding).
		Doc("Get a TenantRoleBinding by name").
		Param(ws.PathParameter("tenant", "Tenant name")).Param(ws.PathParameter("name", "TenantRoleBinding name")).
		Writes(iamv1.TenantRoleBinding{}).Returns(http.StatusOK, "OK", iamv1.TenantRoleBinding{}))

	ws.Route(ws.PUT("/tenants/{tenant}/tenantrolebindings/{name}").To(h.UpdateTenantRoleBinding).
		Doc("Replace a TenantRoleBinding by name").
		Param(ws.PathParameter("tenant", "Tenant name")).Param(ws.PathParameter("name", "TenantRoleBinding name")).
		Reads(iamv1.TenantRoleBinding{}).Writes(iamv1.TenantRoleBinding{}).
		Returns(http.StatusOK, "OK", iamv1.TenantRoleBinding{}))

	ws.Route(ws.PATCH("/tenants/{tenant}/tenantrolebindings/{name}").To(h.PatchTenantRoleBinding).
		Doc("Partially update a TenantRoleBinding (JSON Merge Patch)").
		Param(ws.PathParameter("tenant", "Tenant name")).Param(ws.PathParameter("name", "TenantRoleBinding name")).
		Reads(iamv1.TenantRoleBinding{}).Writes(iamv1.TenantRoleBinding{}).
		Returns(http.StatusOK, "OK", iamv1.TenantRoleBinding{}))

	ws.Route(ws.DELETE("/tenants/{tenant}/tenantrolebindings/{name}").To(h.DeleteTenantRoleBinding).
		Doc("Delete a TenantRoleBinding by name").
		Param(ws.PathParameter("tenant", "Tenant name")).Param(ws.PathParameter("name", "TenantRoleBinding name")).
		Returns(http.StatusNoContent, "No Content", nil))

	c.Add(ws)
}
