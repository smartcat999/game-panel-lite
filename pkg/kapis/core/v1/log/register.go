package log

import (
	"net/http"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	apiruntime "github.com/smartcat999/game-panel-cloud/pkg/apiserver/runtime"
	logmodel "github.com/smartcat999/game-panel-cloud/pkg/models/core/log"
)

func init() {
	apiruntime.Register("core.gamepanel.io/v1/logs", AddToContainer)
}

func AddToContainer(_ apiruntime.Dependencies, c *apiruntime.Container) {
	h := newHandler(logmodel.NewOperator())
	ws := apiruntime.NewWebService(corev1.GroupVersion, "LogEntry")

	ws.Route(ws.GET("/logs").
		To(h.ListPlatformLogs).
		Doc("List structured platform component or aggregated container LogEntries").
		Param(ws.QueryParameter("scope", "Log scope (components, instance, all)")).
		Param(ws.QueryParameter("service", "Filter by service component (apiserver, controller-manager, node-agent, registry-worker, instance)")).
		Param(ws.QueryParameter("level", "Filter by log severity level (ERROR, WARN, INFO)")).
		Param(ws.QueryParameter("tenant", "Filter by tenant workspace name")).
		Param(ws.QueryParameter("instance", "Filter by server instance name")).
		Param(ws.QueryParameter("window", "Relative lookback window (15m, 1h, 6h, 24h, 7d, 30d)")).
		Param(ws.QueryParameter("since", "RFC3339 lower-bound timestamp")).
		Param(ws.QueryParameter("q", "Full-text search keyword or request ID")).
		WithListOptions().
		Writes(metav1.List[*corev1.LogEntry]{}).
		Returns(http.StatusOK, "OK", metav1.List[*corev1.LogEntry]{}))

	ws.Route(ws.GET("/tenants/{tenant}/logs").
		To(h.ListTenantLogs).
		Doc("List aggregated game instance container LogEntries strictly scoped to a Tenant").
		Param(ws.PathParameter("tenant", "Tenant workspace name")).
		Param(ws.QueryParameter("instance", "Filter by server instance name")).
		Param(ws.QueryParameter("level", "Filter by log severity level (ERROR, WARN, INFO)")).
		Param(ws.QueryParameter("window", "Relative lookback window (15m, 1h, 6h, 24h, 7d, 30d)")).
		Param(ws.QueryParameter("since", "RFC3339 lower-bound timestamp")).
		Param(ws.QueryParameter("q", "Full-text search keyword")).
		WithListOptions().
		Writes(metav1.List[*corev1.LogEntry]{}).
		Returns(http.StatusOK, "OK", metav1.List[*corev1.LogEntry]{}))

	c.Add(ws)
}
