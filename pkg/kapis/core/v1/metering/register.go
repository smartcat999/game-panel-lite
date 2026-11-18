package metering

import (
	"net/http"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	apiruntime "github.com/smartcat999/game-panel-cloud/pkg/apiserver/runtime"
	meteringmodel "github.com/smartcat999/game-panel-cloud/pkg/models/core/metering"
)

func init() {
	apiruntime.Register("core.gamepanel.io/v1/metering", AddToContainer)
}

func AddToContainer(deps apiruntime.Dependencies, c *apiruntime.Container) {
	h := newHandler(meteringmodel.NewOperator(deps.DB))
	ws := apiruntime.NewWebService(corev1.GroupVersion, "TenantMetering")

	ws.Route(ws.GET("/metering").
		To(h.ListClusterMetering).
		Doc("List resource usage metering across all Tenants (supports ?format=openmeter|cloudevents|lago)").
		Param(ws.QueryParameter("window", "Metering time window (24h, 7d, 30d)").DataType("string")).
		Param(ws.QueryParameter("format", "Output format: default (TenantMeteringList), openmeter / cloudevents (CNCF CloudEvents 1.0 batch), or lago (Lago events batch)").DataType("string")).
		WithListOptions().
		Writes(metav1.List[*corev1.TenantMetering]{}).
		Returns(http.StatusOK, "OK", metav1.List[*corev1.TenantMetering]{}))

	ws.Route(ws.GET("/tenants/{tenant}/metering").
		To(h.ListTenantMetering).
		Doc("List resource usage metering for a specific Tenant (supports ?format=openmeter|cloudevents|lago)").
		Param(ws.PathParameter("tenant", "Tenant name")).
		Param(ws.QueryParameter("window", "Metering time window (24h, 7d, 30d)").DataType("string")).
		Param(ws.QueryParameter("format", "Output format: default (TenantMeteringList), openmeter / cloudevents (CNCF CloudEvents 1.0 batch), or lago (Lago events batch)").DataType("string")).
		WithListOptions().
		Writes(metav1.List[*corev1.TenantMetering]{}).
		Returns(http.StatusOK, "OK", metav1.List[*corev1.TenantMetering]{}))

	c.Add(ws)
}
