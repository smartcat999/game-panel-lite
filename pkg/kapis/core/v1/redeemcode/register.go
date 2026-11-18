package redeemcode

import (
	"net/http"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	apiruntime "github.com/smartcat999/game-panel-cloud/pkg/apiserver/runtime"
	"github.com/smartcat999/game-panel-cloud/pkg/models/core/subscription"
)

func init() {
	apiruntime.Register("core.gamepanel.io/v1/redeemcodes", AddToContainer)
}

// AddToContainer registers cluster-scoped RedeemCode routes into the API container.
func AddToContainer(deps apiruntime.Dependencies, c *apiruntime.Container) {
	h := newHandler(subscription.NewService(deps.DB, deps.MQ))
	ws := apiruntime.NewWebService(corev1.GroupVersion, corev1.KindRedeemCode)

	ws.Route(ws.GET("/redeemcodes").
		To(h.ListRedeemCodes).
		Doc("List masked subscription redeem codes").
		Param(ws.QueryParameter("batchName", "Optional batch name filter")).
		WithListOptions().
		Writes(metav1.List[corev1.RedeemCode]{}).
		Returns(http.StatusOK, "OK", metav1.List[corev1.RedeemCode]{}))

	ws.Route(ws.POST("/redeemcodes").
		To(h.CreateRedeemCodeBatch).
		Doc("Generate a batch of single-use subscription redeem codes").
		Reads(corev1.CreateRedeemCodeBatchRequest{}).
		Writes(corev1.RedeemCodeBatch{}).
		Returns(http.StatusCreated, "Created", corev1.RedeemCodeBatch{}))

	c.Add(ws)
}
