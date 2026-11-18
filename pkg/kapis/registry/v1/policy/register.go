package policy

import (
	"net/http"

	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	apiruntime "github.com/smartcat999/game-panel-cloud/pkg/apiserver/runtime"
	policymodel "github.com/smartcat999/game-panel-cloud/pkg/models/registry/policy"
)

func init() {
	apiruntime.Register("registry.gamepanel.io/v1/replicationpolicies", AddToContainer)
}

func AddToContainer(deps apiruntime.Dependencies, c *apiruntime.Container) {
	h := &handler{op: policymodel.NewOperator(deps.DB, deps.MQ)}
	ws := apiruntime.NewWebService(registryv1.GroupVersion, registryv1.KindReplicationPolicy)

	ws.Route(ws.GET("/replicationpolicies").To(h.ListPolicies).Doc("List all ReplicationPolicies").
		WithListOptions().Writes(metav1.List[*registryv1.ReplicationPolicy]{}).
		Returns(http.StatusOK, "OK", metav1.List[*registryv1.ReplicationPolicy]{}))

	ws.Route(ws.POST("/replicationpolicies").To(h.CreatePolicy).Doc("Create a ReplicationPolicy").
		Reads(registryv1.ReplicationPolicy{}).Writes(registryv1.ReplicationPolicy{}).
		Returns(http.StatusCreated, "Created", registryv1.ReplicationPolicy{}))

	ws.Route(ws.GET("/replicationpolicies/{name}").To(h.GetPolicy).Doc("Get a ReplicationPolicy by name").
		Param(ws.PathParameter("name", "Policy name")).Writes(registryv1.ReplicationPolicy{}).
		Returns(http.StatusOK, "OK", registryv1.ReplicationPolicy{}))

	ws.Route(ws.PUT("/replicationpolicies/{name}").To(h.UpdatePolicy).Doc("Replace a ReplicationPolicy").
		Param(ws.PathParameter("name", "Policy name")).Reads(registryv1.ReplicationPolicy{}).Writes(registryv1.ReplicationPolicy{}).
		Returns(http.StatusOK, "OK", registryv1.ReplicationPolicy{}))

	ws.Route(ws.PATCH("/replicationpolicies/{name}").To(h.PatchPolicy).Doc("Patch a ReplicationPolicy").
		Param(ws.PathParameter("name", "Policy name")).Reads(registryv1.ReplicationPolicy{}).Writes(registryv1.ReplicationPolicy{}).
		Returns(http.StatusOK, "OK", registryv1.ReplicationPolicy{}))

	ws.Route(ws.DELETE("/replicationpolicies/{name}").To(h.DeletePolicy).Doc("Delete a ReplicationPolicy").
		Param(ws.PathParameter("name", "Policy name")).Returns(http.StatusNoContent, "No Content", nil))

	c.Add(ws)
}
