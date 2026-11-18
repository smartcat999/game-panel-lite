package pkg

import (
	"net/http"

	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	apiruntime "github.com/smartcat999/game-panel-cloud/pkg/apiserver/runtime"
	pkgmodel "github.com/smartcat999/game-panel-cloud/pkg/models/registry/pkg"
)

func init() {
	apiruntime.Register("registry.gamepanel.io/v1/packages", AddToContainer)
}

func AddToContainer(deps apiruntime.Dependencies, c *apiruntime.Container) {
	h := &handler{op: pkgmodel.NewOperator(deps.DB, deps.Cache)}
	ws := apiruntime.NewWebService(registryv1.GroupVersion, registryv1.KindPackage)

	ws.Route(ws.GET("/packages").To(h.ListPackages).Doc("List or search Registry Packages").
		Param(ws.QueryParameter("repository", "Filter or proxy-search within a Repository").DataType("string")).
		Param(ws.QueryParameter("q", "Search keyword").DataType("string")).
		Param(ws.QueryParameter("sort", "Catalog sort order").DataType("string")).
		Param(ws.QueryParameter("gameVersion", "Target game version filter").DataType("string")).
		Param(ws.QueryParameter("resolve", "Resolve direct URL, Workshop Collection URL, or IDs").DataType("string")).
		WithListOptions().Writes(metav1.List[*registryv1.Package]{}).
		Returns(http.StatusOK, "OK", metav1.List[*registryv1.Package]{}))

	ws.Route(ws.POST("/packages").To(h.CreatePackage).Doc("Create a Registry Package").
		Reads(registryv1.Package{}).Writes(registryv1.Package{}).
		Returns(http.StatusCreated, "Created", registryv1.Package{}))

	ws.Route(ws.GET("/packages/{name}").To(h.GetPackage).Doc("Get a Registry Package by <repository>.<package>").
		Param(ws.PathParameter("name", "Package coordinate")).Writes(registryv1.Package{}).
		Returns(http.StatusOK, "OK", registryv1.Package{}))

	ws.Route(ws.PUT("/packages/{name}").To(h.UpdatePackage).Doc("Replace a Registry Package").
		Param(ws.PathParameter("name", "Package coordinate")).Reads(registryv1.Package{}).Writes(registryv1.Package{}).
		Returns(http.StatusOK, "OK", registryv1.Package{}))

	ws.Route(ws.PATCH("/packages/{name}").To(h.PatchPackage).Doc("Patch a Registry Package").
		Param(ws.PathParameter("name", "Package coordinate")).Reads(registryv1.Package{}).Writes(registryv1.Package{}).
		Returns(http.StatusOK, "OK", registryv1.Package{}))

	ws.Route(ws.DELETE("/packages/{name}").To(h.DeletePackage).Doc("Delete a Registry Package").
		Param(ws.PathParameter("name", "Package coordinate")).Returns(http.StatusNoContent, "No Content", nil))

	ws.Route(ws.GET("/packages/{name}/icon").To(h.GetPackageIcon).Doc("Stream HMAC-signed cached package thumbnail").
		Param(ws.PathParameter("name", "Package coordinate")).
		Param(ws.QueryParameter("u", "Base64url upstream image URL").DataType("string")).
		Param(ws.QueryParameter("sig", "HMAC-SHA256 signature").DataType("string")).
		Returns(http.StatusOK, "OK", nil))

	c.Add(ws)
}
