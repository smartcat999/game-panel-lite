package node

import (
	"net/http"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	apiruntime "github.com/smartcat999/game-panel-cloud/pkg/apiserver/runtime"
	nodemodel "github.com/smartcat999/game-panel-cloud/pkg/models/core/node"
)

func init() {
	apiruntime.Register("core.gamepanel.io/v1/nodes", AddToContainer)
}

func AddToContainer(deps apiruntime.Dependencies, c *apiruntime.Container) {
	h := newHandler(nodemodel.NewOperator(deps.DB, deps.MQ, nodemodel.WithDrivers(deps.Drivers)), deps.MQ)
	ws := apiruntime.NewWebService(corev1.GroupVersion, "Node")

	ws.Route(ws.GET("/nodes").To(h.ListNodes).Doc("List cluster-scoped compute Nodes").
		Param(ws.QueryParameter("watch", "Watch for real-time resource changes (true/false)")).
		WithListOptions().Writes(metav1.List[*corev1.Node]{}).
		Returns(http.StatusOK, "OK", metav1.List[*corev1.Node]{}))

	ws.Route(ws.POST("/nodes").To(h.CreateNode).Doc("Register a compute Node").
		Reads(corev1.Node{}).Writes(corev1.Node{}).Returns(http.StatusCreated, "Created", corev1.Node{}))

	ws.Route(ws.GET("/nodes/{name}").To(h.GetNode).Doc("Get a compute Node by name").
		Param(ws.PathParameter("name", "Node name")).Writes(corev1.Node{}).
		Returns(http.StatusOK, "OK", corev1.Node{}))

	ws.Route(ws.PUT("/nodes/{name}").To(h.UpdateNode).Doc("Replace a compute Node spec by name").
		Param(ws.PathParameter("name", "Node name")).Reads(corev1.Node{}).Writes(corev1.Node{}).
		Returns(http.StatusOK, "OK", corev1.Node{}))

	ws.Route(ws.PATCH("/nodes/{name}").To(h.PatchNode).Doc("Partially update a Node (JSON Merge Patch)").
		Param(ws.PathParameter("name", "Node name")).Reads(corev1.Node{}).Writes(corev1.Node{}).
		Returns(http.StatusOK, "OK", corev1.Node{}))

	ws.Route(ws.DELETE("/nodes/{name}").To(h.DeleteNode).Doc("Delete a compute Node by name").
		Param(ws.PathParameter("name", "Node name")).Returns(http.StatusNoContent, "No Content", nil))

	ws.Route(ws.GET("/nodes/{name}/status").To(h.GetNode).Doc("Get a compute Node status subresource").
		Param(ws.PathParameter("name", "Node name")).Writes(corev1.Node{}).
		Returns(http.StatusOK, "OK", corev1.Node{}))

	ws.Route(ws.PUT("/nodes/{name}/status").To(h.UpdateNodeStatus).Doc("Replace a compute Node status subresource").
		Param(ws.PathParameter("name", "Node name")).Reads(corev1.Node{}).Writes(corev1.Node{}).
		Returns(http.StatusOK, "OK", corev1.Node{}))

	ws.Route(ws.PATCH("/nodes/{name}/status").To(h.UpdateNodeStatus).Doc("Patch a compute Node status subresource").
		Param(ws.PathParameter("name", "Node name")).Reads(corev1.Node{}).Writes(corev1.Node{}).
		Returns(http.StatusOK, "OK", corev1.Node{}))

	ws.Route(ws.GET("/nodes/{name}/metrics").To(h.GetNodeMetrics).
		Doc("Get time-series physical host resource usage metrics subresource for a Node").
		Param(ws.PathParameter("name", "Node name")).
		Param(ws.QueryParameter("window", "Time window duration (e.g. 15m, 1h, 6h)")).
		Writes(corev1.NodeMetrics{}).Returns(http.StatusOK, "OK", corev1.NodeMetrics{}))

	ws.Route(ws.GET("/nodes/{name}/exec").To(h.ExecNode).
		Doc("Interactive WebSocket PTY terminal session on a compute Node").
		Param(ws.PathParameter("name", "Node name")).
		Param(ws.QueryParameter("tty", "Allocate a full-duplex PTY terminal session (true/false)")).
		Param(ws.QueryParameter("cols", "Initial PTY width in columns")).
		Param(ws.QueryParameter("rows", "Initial PTY height in rows")).
		Param(ws.QueryParameter("command", "Optional command argument(s) to execute upon WebSocket connect")).
		Returns(http.StatusSwitchingProtocols, "Switching Protocols", nil))

	c.Add(ws)
}
