package user

import (
	"net/http"

	iamv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/iam/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	apiruntime "github.com/smartcat999/game-panel-cloud/pkg/apiserver/runtime"
	usermodel "github.com/smartcat999/game-panel-cloud/pkg/models/iam/user"
)

func init() {
	apiruntime.Register("iam.gamepanel.io/v1/users", AddToContainer)
}

func AddToContainer(deps apiruntime.Dependencies, c *apiruntime.Container) {
	h := newHandler(usermodel.NewOperatorWithRevoker(deps.DB, deps.Sessions, deps.MQ), deps.MQ)
	ws := apiruntime.NewWebService(iamv1.GroupVersion, "User")

	ws.Route(ws.GET("/users").To(h.ListUsers).Doc("List all platform Users").
		Param(ws.QueryParameter("watch", "Watch for real-time resource changes (true/false)").DataType("boolean")).
		WithListOptions().Writes(metav1.List[*iamv1.User]{}).
		Returns(http.StatusOK, "OK", metav1.List[*iamv1.User]{}))

	ws.Route(ws.POST("/users").To(h.CreateUser).Doc("Create a User and provision its Personal Tenant").
		Reads(iamv1.User{}).Writes(iamv1.User{}).Returns(http.StatusCreated, "Created", iamv1.User{}))

	ws.Route(ws.GET("/users/{name}").To(h.GetUser).Doc("Get a User by name").
		Param(ws.PathParameter("name", "User name")).Writes(iamv1.User{}).Returns(http.StatusOK, "OK", iamv1.User{}))

	ws.Route(ws.PUT("/users/{name}").To(h.UpdateUser).Doc("Replace a User spec by name").
		Param(ws.PathParameter("name", "User name")).Reads(iamv1.User{}).Writes(iamv1.User{}).
		Returns(http.StatusOK, "OK", iamv1.User{}))

	ws.Route(ws.PATCH("/users/{name}").To(h.PatchUser).Doc("Partially update a User (JSON Merge Patch)").
		Param(ws.PathParameter("name", "User name")).Reads(iamv1.User{}).Writes(iamv1.User{}).
		Returns(http.StatusOK, "OK", iamv1.User{}))

	ws.Route(ws.DELETE("/users/{name}").To(h.DeleteUser).
		Doc("Request asynchronous User and Personal Tenant deletion").
		Param(ws.PathParameter("name", "User name")).Returns(http.StatusAccepted, "Accepted", nil))

	ws.Route(ws.GET("/users/{name}/status").To(h.GetUser).Doc("Get a User status subresource").
		Param(ws.PathParameter("name", "User name")).Writes(iamv1.User{}).Returns(http.StatusOK, "OK", iamv1.User{}))

	ws.Route(ws.PUT("/users/{name}/status").To(h.UpdateUserStatus).Doc("Replace a User status subresource").
		Param(ws.PathParameter("name", "User name")).Reads(iamv1.User{}).Writes(iamv1.User{}).
		Returns(http.StatusOK, "OK", iamv1.User{}))

	ws.Route(ws.PATCH("/users/{name}/status").To(h.UpdateUserStatus).Doc("Patch a User status subresource").
		Param(ws.PathParameter("name", "User name")).Reads(iamv1.User{}).Writes(iamv1.User{}).
		Returns(http.StatusOK, "OK", iamv1.User{}))

	c.Add(ws)
}
