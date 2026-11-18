package instance

import (
	"net/http"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	apiruntime "github.com/smartcat999/game-panel-cloud/pkg/apiserver/runtime"
	instancemodel "github.com/smartcat999/game-panel-cloud/pkg/models/core/instance"
)

func init() {
	apiruntime.Register("core.gamepanel.io/v1/instances", AddToContainer)
}

func AddToContainer(deps apiruntime.Dependencies, c *apiruntime.Container) {
	h := newHandler(instancemodel.NewOperator(deps.DB, deps.Drivers, deps.MQ), deps.MQ)
	ws := apiruntime.NewWebService(corev1.GroupVersion, "Instance")
	registerInstanceCRUDRoutes(ws, h)
	registerInstanceSubresourceRoutes(ws, h)
	registerInstanceActionRoutes(ws, h)
	c.Add(ws)
}

func registerInstanceCRUDRoutes(ws *apiruntime.WebService, h *handler) {
	ws.Route(ws.GET("/instances").To(h.ListClusterInstances).
		Doc("List all Instances across the cluster (optionally filtered by nodeName)").
		Param(ws.QueryParameter("nodeName", "Filter instances scheduled on a specific Node")).
		Param(ws.QueryParameter("watch", "Watch for real-time resource changes (true/false)")).
		WithListOptions().Writes(metav1.List[*corev1.Instance]{}).
		Returns(http.StatusOK, "OK", metav1.List[*corev1.Instance]{}))

	ws.Route(ws.GET("/tenants/{tenant}/instances").To(h.ListInstances).
		Doc("List Instances in a Tenant").Param(ws.PathParameter("tenant", "Tenant name")).
		Param(ws.QueryParameter("watch", "Watch for real-time resource changes (true/false)")).
		WithListOptions().Writes(metav1.List[*corev1.Instance]{}).
		Returns(http.StatusOK, "OK", metav1.List[*corev1.Instance]{}))

	ws.Route(ws.POST("/tenants/{tenant}/instances").To(h.CreateInstance).
		Doc("Create an Instance in a Tenant").Param(ws.PathParameter("tenant", "Tenant name")).
		Reads(corev1.Instance{}).Writes(corev1.Instance{}).
		Returns(http.StatusCreated, "Created", corev1.Instance{}))

	ws.Route(ws.GET("/tenants/{tenant}/instances/{name}").To(h.GetInstance).
		Doc("Get an Instance by name").Param(ws.PathParameter("tenant", "Tenant name")).
		Param(ws.PathParameter("name", "Instance name")).Writes(corev1.Instance{}).
		Returns(http.StatusOK, "OK", corev1.Instance{}))

	ws.Route(ws.PUT("/tenants/{tenant}/instances/{name}").To(h.UpdateInstance).
		Doc("Replace an Instance spec by name").Param(ws.PathParameter("tenant", "Tenant name")).
		Param(ws.PathParameter("name", "Instance name")).Reads(corev1.Instance{}).Writes(corev1.Instance{}).
		Returns(http.StatusOK, "OK", corev1.Instance{}))

	ws.Route(ws.PATCH("/tenants/{tenant}/instances/{name}").To(h.PatchInstance).
		Doc("Partially update an Instance (JSON Merge Patch)").Param(ws.PathParameter("tenant", "Tenant name")).
		Param(ws.PathParameter("name", "Instance name")).Reads(corev1.Instance{}).Writes(corev1.Instance{}).
		Returns(http.StatusOK, "OK", corev1.Instance{}))

	ws.Route(ws.DELETE("/tenants/{tenant}/instances/{name}").To(h.DeleteInstance).
		Doc("Request asynchronous Instance deletion; cleanup and resource release are reconciled").
		Param(ws.PathParameter("tenant", "Tenant name")).Param(ws.PathParameter("name", "Instance name")).
		Returns(http.StatusAccepted, "Deletion request persisted", nil))
}

func registerInstanceSubresourceRoutes(ws *apiruntime.WebService, h *handler) {
	ws.Route(ws.GET("/tenants/{tenant}/instances/{name}/status").To(h.GetInstance).
		Doc("Get an Instance status subresource").Param(ws.PathParameter("tenant", "Tenant name")).
		Param(ws.PathParameter("name", "Instance name")).Writes(corev1.Instance{}).
		Returns(http.StatusOK, "OK", corev1.Instance{}))
	ws.Route(ws.PUT("/tenants/{tenant}/instances/{name}/status").To(h.UpdateInstanceStatus).
		Doc("Replace an Instance status subresource").Param(ws.PathParameter("tenant", "Tenant name")).
		Param(ws.PathParameter("name", "Instance name")).Reads(corev1.Instance{}).Writes(corev1.Instance{}).
		Returns(http.StatusOK, "OK", corev1.Instance{}))
	ws.Route(ws.PATCH("/tenants/{tenant}/instances/{name}/status").To(h.UpdateInstanceStatus).
		Doc("Patch an Instance status subresource").Param(ws.PathParameter("tenant", "Tenant name")).
		Param(ws.PathParameter("name", "Instance name")).Reads(corev1.Instance{}).Writes(corev1.Instance{}).
		Returns(http.StatusOK, "OK", corev1.Instance{}))
	ws.Route(ws.GET("/tenants/{tenant}/instances/{name}/state").To(h.GetInstanceState).
		Doc("Get an Instance desired state subresource").Param(ws.PathParameter("tenant", "Tenant name")).
		Param(ws.PathParameter("name", "Instance name")).Writes(corev1.InstanceState{}).
		Returns(http.StatusOK, "OK", corev1.InstanceState{}))
	ws.Route(ws.PUT("/tenants/{tenant}/instances/{name}/state").To(h.UpdateInstanceState).
		Doc("Replace an Instance desired state subresource (Running | Stopped)").Param(ws.PathParameter("tenant", "Tenant name")).
		Param(ws.PathParameter("name", "Instance name")).Reads(corev1.InstanceState{}).Writes(corev1.Instance{}).
		Returns(http.StatusOK, "OK", corev1.Instance{}))
	ws.Route(ws.PATCH("/tenants/{tenant}/instances/{name}/state").To(h.UpdateInstanceState).
		Doc("Patch an Instance desired state subresource (Running | Stopped)").Param(ws.PathParameter("tenant", "Tenant name")).
		Param(ws.PathParameter("name", "Instance name")).Reads(corev1.InstanceState{}).Writes(corev1.Instance{}).
		Returns(http.StatusOK, "OK", corev1.Instance{}))
}

func registerInstanceActionRoutes(ws *apiruntime.WebService, h *handler) {
	ws.Route(ws.GET("/tenants/{tenant}/instances/{name}/log").To(h.StreamInstanceLogs).
		Doc("Stream standard output or task logs from an Instance (HTTP or WebSocket upgrade)").
		Param(ws.PathParameter("tenant", "Tenant name")).Param(ws.PathParameter("name", "Instance name")).
		Param(ws.QueryParameter("taskID", "Optional backup, restore, or scheduled task log identifier")).
		Param(ws.QueryParameter("follow", "Follow log stream in real time (boolean)")).
		Param(ws.QueryParameter("tailLines", "Number of lines from end of log (integer)")).
		Returns(http.StatusOK, "OK", ""))
	ws.Route(ws.GET("/tenants/{tenant}/instances/{name}/exec").To(h.ExecInstance).
		Doc("Interactive WebSocket command or PTY execution inside a running Instance").
		Param(ws.PathParameter("tenant", "Tenant name")).Param(ws.PathParameter("name", "Instance name")).
		Param(ws.QueryParameter("tty", "Allocate a full-duplex PTY terminal session (true/false)")).
		Param(ws.QueryParameter("cols", "Initial PTY width in columns when tty=true")).
		Param(ws.QueryParameter("rows", "Initial PTY height in rows when tty=true")).
		Param(ws.QueryParameter("command", "Optional command argument(s) to execute upon WebSocket connect")).
		Writes(corev1.InstanceExec{}).Returns(http.StatusSwitchingProtocols, "Switching Protocols", corev1.InstanceExec{}))
	ws.Route(ws.POST("/tenants/{tenant}/instances/{name}/exec").To(h.ExecInstance).
		Doc("Execute a command inside a running Instance (HTTP POST or WebSocket upgrade)").
		Param(ws.PathParameter("tenant", "Tenant name")).Param(ws.PathParameter("name", "Instance name")).
		Reads(corev1.InstanceExec{}).Writes(corev1.InstanceExec{}).Returns(http.StatusOK, "OK", corev1.InstanceExec{}))
	ws.Route(ws.POST("/tenants/{tenant}/instances/{name}/files").To(h.FilesInstance).
		Doc("Issue a direct-to-node signed file ticket or trigger S3 Pre-signed URL streaming backup/restore").
		Param(ws.PathParameter("tenant", "Tenant name")).Param(ws.PathParameter("name", "Instance name")).
		Reads(corev1.InstanceFiles{}).Writes(corev1.InstanceFiles{}).Returns(http.StatusOK, "OK", corev1.InstanceFiles{}))
	ws.Route(ws.GET("/tenants/{tenant}/instances/{name}/metrics").To(h.GetInstanceMetrics).
		Doc("Get time-series resource usage metrics subresource for an Instance").
		Param(ws.PathParameter("tenant", "Tenant name")).Param(ws.PathParameter("name", "Instance name")).
		Param(ws.QueryParameter("window", "Time window duration (e.g. 5m, 15m, 1h)")).
		Writes(corev1.InstanceMetrics{}).Returns(http.StatusOK, "OK", corev1.InstanceMetrics{}))
}
