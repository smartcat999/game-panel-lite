package session

import (
	"net/http"

	iamv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/iam/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	apiruntime "github.com/smartcat999/game-panel-cloud/pkg/apiserver/runtime"
	sessionmodel "github.com/smartcat999/game-panel-cloud/pkg/models/iam/session"
)

func init() {
	apiruntime.Register("iam.gamepanel.io/v1/sessions", AddToContainer)
}

func AddToContainer(deps apiruntime.Dependencies, c *apiruntime.Container) {
	op := deps.Sessions
	if op == nil {
		op = sessionmodel.NewOperator(deps.DB, deps.Cache, nil)
	}
	h := newHandler(op, deps.CanI)
	ws := apiruntime.NewWebService(iamv1.GroupVersion, "Session")

	ws.Route(ws.GET("/sessions").
		To(h.ListSessions).
		Doc("List active login sessions for the authenticated user").
		WithListOptions().
		Writes(metav1.List[*iamv1.Session]{}).
		Returns(http.StatusOK, "OK", metav1.List[*iamv1.Session]{}))

	ws.Route(ws.POST("/sessions").
		To(h.CreateSession).
		Doc("Authenticate user credentials, persist session in cache, and issue a JWT token").
		Reads(iamv1.Session{}).
		Writes(iamv1.Session{}).
		Returns(http.StatusCreated, "Created", iamv1.Session{}))

	ws.Route(ws.DELETE("/sessions").
		To(h.DeleteSessions).
		Doc("Delete active login Sessions for the authenticated user").
		Param(ws.QueryParameter("fieldSelector", "Selector to filter sessions (e.g. metadata.name!=<session-name>)").DataType("string")).
		Returns(http.StatusNoContent, "No Content", nil))

	ws.Route(ws.GET("/sessions/{name}").
		To(h.GetSession).
		Doc("Get an active Session by name").
		Param(ws.PathParameter("name", "Session name")).
		Writes(iamv1.Session{}).
		Returns(http.StatusOK, "OK", iamv1.Session{}))

	ws.Route(ws.DELETE("/sessions/{name}").
		To(h.DeleteSession).
		Doc("Delete an active Session by name").
		Param(ws.PathParameter("name", "Session name")).
		Returns(http.StatusNoContent, "No Content", nil))

	c.Add(ws)
}
