package event

import (
	"net/http"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	apiruntime "github.com/smartcat999/game-panel-cloud/pkg/apiserver/runtime"
	eventmodel "github.com/smartcat999/game-panel-cloud/pkg/models/core/event"
)

func init() {
	apiruntime.Register("core.gamepanel.io/v1/events", AddToContainer)
}

func AddToContainer(deps apiruntime.Dependencies, c *apiruntime.Container) {
	h := newHandler(eventmodel.NewOperator(deps.DB))
	ws := apiruntime.NewWebService(corev1.GroupVersion, "Event")

	ws.Route(ws.GET("/events").
		To(h.ListClusterEvents).
		Doc("List operational audit Events across all tenants").
		Param(ws.QueryParameter("tenant", "Filter by tenant workspace name")).
		Param(ws.QueryParameter("window", "Relative lookback window (15m, 1h, 6h, 24h, 7d, 30d)")).
		Param(ws.QueryParameter("since", "RFC3339 lower-bound timestamp")).
		Param(ws.QueryParameter("resource", "Filter by target resource type (instances, nodes, tenants, users, apps, ...)")).
		Param(ws.QueryParameter("verb", "Filter by audit action verb (create, update, patch, delete)")).
		Param(ws.QueryParameter("actor", "Filter by acting user or subject name")).
		WithListOptions().
		Writes(metav1.List[*corev1.Event]{}).
		Returns(http.StatusOK, "OK", metav1.List[*corev1.Event]{}))

	ws.Route(ws.GET("/tenants/{tenant}/events").
		To(h.ListTenantEvents).
		Doc("List operational audit Events in a Tenant").
		Param(ws.PathParameter("tenant", "Tenant name")).
		Param(ws.QueryParameter("window", "Relative lookback window (15m, 1h, 6h, 24h, 7d, 30d)")).
		Param(ws.QueryParameter("since", "RFC3339 lower-bound timestamp")).
		Param(ws.QueryParameter("resource", "Filter by target resource type (instances, tenants, invites, ...)")).
		Param(ws.QueryParameter("verb", "Filter by audit action verb (create, update, patch, delete)")).
		Param(ws.QueryParameter("actor", "Filter by acting user or subject name")).
		WithListOptions().
		Writes(metav1.List[*corev1.Event]{}).
		Returns(http.StatusOK, "OK", metav1.List[*corev1.Event]{}))

	c.Add(ws)
}
