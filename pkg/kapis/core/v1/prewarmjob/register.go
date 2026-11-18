package prewarmjob

import (
	"net/http"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	apiruntime "github.com/smartcat999/game-panel-cloud/pkg/apiserver/runtime"
	prewarmjobmodel "github.com/smartcat999/game-panel-cloud/pkg/models/core/prewarmjob"
)

func init() {
	apiruntime.Register("core.gamepanel.io/v1/prewarmjobs", AddToContainer)
}

func AddToContainer(deps apiruntime.Dependencies, c *apiruntime.Container) {
	h := newHandler(prewarmjobmodel.NewOperator(deps.DB, deps.MQ), deps.MQ)
	ws := apiruntime.NewWebService(corev1.GroupVersion, corev1.KindPrewarmJob)

	ws.Route(ws.GET("/prewarmjobs").To(h.ListPrewarmJobs).Doc("List cluster-scoped PrewarmJobs").
		Param(ws.QueryParameter("watch", "Watch for real-time resource changes (true/false)")).
		WithListOptions().Writes(metav1.List[*corev1.PrewarmJob]{}).
		Returns(http.StatusOK, "OK", metav1.List[*corev1.PrewarmJob]{}))

	ws.Route(ws.POST("/prewarmjobs").To(h.CreatePrewarmJob).Doc("Create a cluster-scoped PrewarmJob").
		Reads(corev1.PrewarmJob{}).Writes(corev1.PrewarmJob{}).Returns(http.StatusCreated, "Created", corev1.PrewarmJob{}))

	ws.Route(ws.GET("/prewarmjobs/{name}").To(h.GetPrewarmJob).Doc("Get a PrewarmJob by name").
		Param(ws.PathParameter("name", "PrewarmJob name")).Writes(corev1.PrewarmJob{}).
		Returns(http.StatusOK, "OK", corev1.PrewarmJob{}))

	ws.Route(ws.PUT("/prewarmjobs/{name}").To(h.UpdatePrewarmJob).Doc("Replace a PrewarmJob spec by name").
		Param(ws.PathParameter("name", "PrewarmJob name")).Reads(corev1.PrewarmJob{}).Writes(corev1.PrewarmJob{}).
		Returns(http.StatusOK, "OK", corev1.PrewarmJob{}))

	ws.Route(ws.DELETE("/prewarmjobs/{name}").To(h.DeletePrewarmJob).Doc("Delete a PrewarmJob by name").
		Param(ws.PathParameter("name", "PrewarmJob name")).Returns(http.StatusNoContent, "No Content", nil))

	ws.Route(ws.GET("/prewarmjobs/{name}/status").To(h.GetPrewarmJob).Doc("Get a PrewarmJob status subresource").
		Param(ws.PathParameter("name", "PrewarmJob name")).Writes(corev1.PrewarmJob{}).
		Returns(http.StatusOK, "OK", corev1.PrewarmJob{}))

	ws.Route(ws.PUT("/prewarmjobs/{name}/status").To(h.UpdatePrewarmJobStatus).Doc("Update a PrewarmJob status subresource").
		Param(ws.PathParameter("name", "PrewarmJob name")).Reads(corev1.PrewarmJob{}).Writes(corev1.PrewarmJob{}).
		Returns(http.StatusOK, "OK", corev1.PrewarmJob{}))

	ws.Route(ws.PATCH("/prewarmjobs/{name}/status").To(h.UpdatePrewarmJobStatus).Doc("Patch a PrewarmJob status subresource").
		Param(ws.PathParameter("name", "PrewarmJob name")).Reads(corev1.PrewarmJob{}).Writes(corev1.PrewarmJob{}).
		Returns(http.StatusOK, "OK", corev1.PrewarmJob{}))

	c.Add(ws)
}
