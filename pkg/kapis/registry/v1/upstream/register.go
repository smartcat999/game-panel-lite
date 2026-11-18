package upstream

import (
	"net/http"

	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	apiruntime "github.com/smartcat999/game-panel-cloud/pkg/apiserver/runtime"
	upstreammodel "github.com/smartcat999/game-panel-cloud/pkg/models/registry/upstream"
)

func init() {
	apiruntime.Register("registry.gamepanel.io/v1/upstreams", AddToContainer)
}

func AddToContainer(deps apiruntime.Dependencies, c *apiruntime.Container) {
	h := &handler{op: upstreammodel.NewOperator(deps.DB)}
	ws := apiruntime.NewWebService(registryv1.GroupVersion, registryv1.KindUpstream)

	ws.Route(ws.GET("/upstreams").To(h.ListUpstreams).Doc("List all Registry Upstreams").
		WithListOptions().Writes(metav1.List[*registryv1.Upstream]{}).
		Returns(http.StatusOK, "OK", metav1.List[*registryv1.Upstream]{}))

	ws.Route(ws.POST("/upstreams").To(h.CreateUpstream).Doc("Create a Registry Upstream").
		Reads(registryv1.Upstream{}).Writes(registryv1.Upstream{}).
		Returns(http.StatusCreated, "Created", registryv1.Upstream{}))

	ws.Route(ws.GET("/upstreams/{name}").To(h.GetUpstream).Doc("Get a Registry Upstream by name").
		Param(ws.PathParameter("name", "Upstream name")).Writes(registryv1.Upstream{}).
		Returns(http.StatusOK, "OK", registryv1.Upstream{}))

	ws.Route(ws.PUT("/upstreams/{name}").To(h.UpdateUpstream).Doc("Replace a Registry Upstream by name").
		Param(ws.PathParameter("name", "Upstream name")).Reads(registryv1.Upstream{}).Writes(registryv1.Upstream{}).
		Returns(http.StatusOK, "OK", registryv1.Upstream{}))

	ws.Route(ws.PATCH("/upstreams/{name}").To(h.PatchUpstream).Doc("Patch a Registry Upstream").
		Param(ws.PathParameter("name", "Upstream name")).Reads(registryv1.Upstream{}).Writes(registryv1.Upstream{}).
		Returns(http.StatusOK, "OK", registryv1.Upstream{}))

	ws.Route(ws.DELETE("/upstreams/{name}").To(h.DeleteUpstream).Doc("Delete a Registry Upstream").
		Param(ws.PathParameter("name", "Upstream name")).Returns(http.StatusNoContent, "No Content", nil))

	c.Add(ws)
}
