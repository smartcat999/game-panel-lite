package tenant

import (
	"net/http"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	apiruntime "github.com/smartcat999/game-panel-cloud/pkg/apiserver/runtime"
	tenantmodel "github.com/smartcat999/game-panel-cloud/pkg/models/core/tenant"
)

func init() {
	apiruntime.Register("core.gamepanel.io/v1/tenants", AddToContainer)
}

func AddToContainer(deps apiruntime.Dependencies, c *apiruntime.Container) {
	h := newHandler(tenantmodel.NewOperator(deps.DB, deps.MQ), deps.DB, deps.MQ)
	h.sessions = deps.Sessions
	ws := apiruntime.NewWebService(corev1.GroupVersion, "Tenant")

	ws.Route(ws.GET("/tenants").To(h.ListTenants).Doc("List all Tenants").
		Param(ws.QueryParameter("watch", "Watch for real-time resource changes (true/false)")).
		WithListOptions().Writes(metav1.List[*corev1.Tenant]{}).
		Returns(http.StatusOK, "OK", metav1.List[*corev1.Tenant]{}))

	ws.Route(ws.POST("/tenants").To(h.CreateTenant).Doc("Create a Tenant and seed built-in TenantRoles").
		Reads(corev1.Tenant{}).Writes(corev1.Tenant{}).Returns(http.StatusCreated, "Created", corev1.Tenant{}))

	ws.Route(ws.GET("/tenants/{tenant}").To(h.GetTenant).Doc("Get a Tenant by name").
		Param(ws.PathParameter("tenant", "Tenant name")).Writes(corev1.Tenant{}).
		Returns(http.StatusOK, "OK", corev1.Tenant{}))

	ws.Route(ws.PUT("/tenants/{tenant}").To(h.UpdateTenant).Doc("Replace a Tenant spec by name").
		Param(ws.PathParameter("tenant", "Tenant name")).Reads(corev1.Tenant{}).Writes(corev1.Tenant{}).
		Returns(http.StatusOK, "OK", corev1.Tenant{}))

	ws.Route(ws.PATCH("/tenants/{tenant}").To(h.PatchTenant).Doc("Partially update a Tenant (JSON Merge Patch)").
		Param(ws.PathParameter("tenant", "Tenant name")).Reads(corev1.Tenant{}).Writes(corev1.Tenant{}).
		Returns(http.StatusOK, "OK", corev1.Tenant{}))

	ws.Route(ws.DELETE("/tenants/{tenant}").To(h.DeleteTenant).
		Doc("Request tenant deletion; children and resources drain asynchronously").
		Param(ws.PathParameter("tenant", "Tenant name")).Returns(http.StatusAccepted, "Accepted", nil))

	ws.Route(ws.GET("/tenants/{tenant}/status").To(h.GetTenant).Doc("Get a Tenant status subresource").
		Param(ws.PathParameter("tenant", "Tenant name")).Writes(corev1.Tenant{}).
		Returns(http.StatusOK, "OK", corev1.Tenant{}))

	ws.Route(ws.PUT("/tenants/{tenant}/status").To(h.UpdateTenantStatus).Doc("Replace a Tenant status subresource").
		Param(ws.PathParameter("tenant", "Tenant name")).Reads(corev1.Tenant{}).Writes(corev1.Tenant{}).
		Returns(http.StatusOK, "OK", corev1.Tenant{}))

	ws.Route(ws.PATCH("/tenants/{tenant}/status").To(h.UpdateTenantStatus).Doc("Patch a Tenant status subresource").
		Param(ws.PathParameter("tenant", "Tenant name")).Reads(corev1.Tenant{}).Writes(corev1.Tenant{}).
		Returns(http.StatusOK, "OK", corev1.Tenant{}))

	addTenantSubresourceRoutes(ws, h)
	c.Add(ws)
}

func addTenantSubresourceRoutes(ws *apiruntime.WebService, h *handler) {
	ws.Route(ws.POST("/tenants/{tenant}/invites").To(h.CreateTenantInvite).Doc("Create a signed workspace invitation token").
		Param(ws.PathParameter("tenant", "Tenant name")).Reads(corev1.TenantInvite{}).Writes(corev1.TenantInvite{}).
		Returns(http.StatusCreated, "Created", corev1.TenantInvite{}))

	ws.Route(ws.GET("/tenants/{tenant}/invites/{token}").To(h.GetTenantInvite).Doc("Inspect a signed workspace invitation token").
		Param(ws.PathParameter("tenant", "Tenant name")).Param(ws.PathParameter("token", "Signed invite token")).
		Writes(corev1.TenantInvite{}).Returns(http.StatusOK, "OK", corev1.TenantInvite{}))

	ws.Route(ws.POST("/tenants/{tenant}/invites/{token}").To(h.AcceptTenantInvite).Doc("Accept a signed workspace invitation token").
		Param(ws.PathParameter("tenant", "Tenant name")).Param(ws.PathParameter("token", "Signed invite token")).
		Reads(corev1.TenantInvite{}).Writes(corev1.TenantInvite{}).Returns(http.StatusOK, "OK", corev1.TenantInvite{}))
}
