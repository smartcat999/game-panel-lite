package artifact

import (
	"net/http"

	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	apiruntime "github.com/smartcat999/game-panel-cloud/pkg/apiserver/runtime"
	artifactmodel "github.com/smartcat999/game-panel-cloud/pkg/models/registry/artifact"
)

func init() {
	apiruntime.Register("registry.gamepanel.io/v1/artifacts", AddToContainer)
}

func AddToContainer(deps apiruntime.Dependencies, c *apiruntime.Container) {
	h := &handler{op: artifactmodel.NewOperator(deps.DB, deps.MQ, deps.Cache), mq: deps.MQ}
	ws := apiruntime.NewWebService(registryv1.GroupVersion, registryv1.KindArtifact)

	ws.Route(ws.GET("/artifacts").To(h.ListArtifacts).Doc("List Registry Artifacts").
		Param(ws.QueryParameter("watch", "Watch for real-time changes").DataType("boolean")).
		WithListOptions().Writes(metav1.List[*registryv1.Artifact]{}).
		Returns(http.StatusOK, "OK", metav1.List[*registryv1.Artifact]{}))

	ws.Route(ws.POST("/artifacts").To(h.CreateArtifact).Doc("Create a Registry Artifact").
		Reads(registryv1.Artifact{}).Writes(registryv1.Artifact{}).
		Returns(http.StatusCreated, "Created", registryv1.Artifact{}))

	ws.Route(ws.GET("/artifacts/{name}").To(h.GetArtifact).Doc("Get a Registry Artifact by <repository>.<package>.<version>").
		Param(ws.PathParameter("name", "Artifact coordinate")).
		Writes(registryv1.Artifact{}).
		Returns(http.StatusOK, "OK", registryv1.Artifact{}))

	ws.Route(ws.PUT("/artifacts/{name}").To(h.UpdateArtifact).Doc("Replace a Registry Artifact spec").
		Param(ws.PathParameter("name", "Artifact coordinate")).Reads(registryv1.Artifact{}).Writes(registryv1.Artifact{}).
		Returns(http.StatusOK, "OK", registryv1.Artifact{}))

	ws.Route(ws.PATCH("/artifacts/{name}").To(h.PatchArtifact).Doc("Patch a Registry Artifact").
		Param(ws.PathParameter("name", "Artifact coordinate")).Reads(registryv1.Artifact{}).Writes(registryv1.Artifact{}).
		Returns(http.StatusOK, "OK", registryv1.Artifact{}))

	ws.Route(ws.DELETE("/artifacts/{name}").To(h.DeleteArtifact).Doc("Delete a Registry Artifact").
		Param(ws.PathParameter("name", "Artifact coordinate")).Returns(http.StatusAccepted, "Accepted", nil))

	registerStatusRoutes(ws, h)
	registerLogRoutes(ws, h)
	c.Add(ws)
}

func registerLogRoutes(ws *apiruntime.WebService, h *handler) {
	ws.Route(ws.GET("/artifacts/{name}/log").To(h.StreamArtifactLogs).Doc("Stream Artifact sync logs").
		Param(ws.PathParameter("name", "Artifact coordinate")).
		Param(ws.QueryParameter("follow", "Follow log stream in real time").DataType("boolean")).
		Param(ws.QueryParameter("tailLines", "Number of historical lines to return").DataType("integer")).
		Returns(http.StatusOK, "OK", nil))

	ws.Route(ws.POST("/artifacts/{name}/log").To(h.AppendArtifactLogs).Doc("Append Artifact sync logs").
		Param(ws.PathParameter("name", "Artifact coordinate")).
		Reads(registryv1.ArtifactLogChunk{}).
		Returns(http.StatusNoContent, "No Content", nil))
}

func registerStatusRoutes(ws *apiruntime.WebService, h *handler) {
	ws.Route(ws.GET("/artifacts/{name}/status").To(h.GetArtifactStatus).Doc("Get Artifact status").
		Param(ws.PathParameter("name", "Artifact coordinate")).Writes(registryv1.Artifact{}).
		Returns(http.StatusOK, "OK", registryv1.Artifact{}))

	ws.Route(ws.PUT("/artifacts/{name}/status").To(h.UpdateArtifactStatus).Doc("Replace Artifact status").
		Param(ws.PathParameter("name", "Artifact coordinate")).Reads(registryv1.Artifact{}).Writes(registryv1.Artifact{}).
		Returns(http.StatusOK, "OK", registryv1.Artifact{}))

	ws.Route(ws.PATCH("/artifacts/{name}/status").To(h.UpdateArtifactStatus).Doc("Patch Artifact status").
		Param(ws.PathParameter("name", "Artifact coordinate")).Reads(registryv1.Artifact{}).Writes(registryv1.Artifact{}).
		Returns(http.StatusOK, "OK", registryv1.Artifact{}))
}
