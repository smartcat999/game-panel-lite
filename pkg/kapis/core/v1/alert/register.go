package alert

import (
	"net/http"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	apiruntime "github.com/smartcat999/game-panel-cloud/pkg/apiserver/runtime"
	alertmodel "github.com/smartcat999/game-panel-cloud/pkg/models/core/alert"
)

func init() {
	apiruntime.Register("core.gamepanel.io/v1/alerts", AddToContainer)
}

func AddToContainer(deps apiruntime.Dependencies, c *apiruntime.Container) {
	h := newHandler(alertmodel.NewOperator(deps.DB))
	ws := apiruntime.NewWebService(corev1.GroupVersion, "Alert")

	ws.Route(ws.GET("/alerts").
		To(h.ListClusterAlerts).
		Doc("List active platform and tenant Alerts").
		WithListOptions().
		Writes(metav1.List[*corev1.Alert]{}).
		Returns(http.StatusOK, "OK", metav1.List[*corev1.Alert]{}))

	ws.Route(ws.GET("/tenants/{tenant}/alerts").
		To(h.ListTenantAlerts).
		Doc("List active Alerts in a Tenant").
		Param(ws.PathParameter("tenant", "Tenant name")).
		WithListOptions().
		Writes(metav1.List[*corev1.Alert]{}).
		Returns(http.StatusOK, "OK", metav1.List[*corev1.Alert]{}))

	c.Add(ws)
}
