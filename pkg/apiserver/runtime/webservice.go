package runtime

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	restful "github.com/emicklei/go-restful/v3"

	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
)

// Parameter is an alias for go-restful's *restful.Parameter.
type Parameter = *restful.Parameter

// RouteBuilder wraps go-restful's *restful.RouteBuilder while adapting standard http.HandlerFunc
// and recording OpenAPI tag metadata.
type RouteBuilder struct {
	builder *restful.RouteBuilder
	tag     string
}

func (b *RouteBuilder) To(handler http.HandlerFunc) *RouteBuilder {
	b.builder.To(func(req *restful.Request, resp *restful.Response) {
		for k, v := range req.PathParameters() {
			req.Request.SetPathValue(k, v)
		}
		handler(resp.ResponseWriter, req.Request)
	})
	return b
}

func (b *RouteBuilder) Doc(summary string) *RouteBuilder {
	b.builder.Doc(summary)
	return b
}

func (b *RouteBuilder) Param(p Parameter) *RouteBuilder {
	b.builder.Param(p)
	return b
}

func (b *RouteBuilder) Reads(sample any) *RouteBuilder {
	b.builder.Reads(sample)
	return b
}

func (b *RouteBuilder) Writes(sample any) *RouteBuilder {
	b.builder.Writes(sample)
	return b
}

func (b *RouteBuilder) Returns(code int, description string, model any) *RouteBuilder {
	b.builder.Returns(code, description, model)
	return b
}

// ListParameters returns the standard query parameters for list operations across all API resources.
func ListParameters() []Parameter {
	return []Parameter{
		restful.QueryParameter("labelSelector", "Selector to filter resources by labels (e.g. env=prod)").DataType("string"),
		restful.QueryParameter("fieldSelector", "Selector to filter resources by fields (e.g. spec.nodeName=node-01)").DataType("string"),
		restful.QueryParameter("limit", "Maximum number of items to return (default: 100, max: 1000)").DataType("integer").DefaultValue("100"),
		restful.QueryParameter("offset", "Number of items to skip (default: 0)").DataType("integer").DefaultValue("0"),
		restful.QueryParameter("continue", "Continue token (decimal offset cursor) for retrieving the next page").DataType("string"),
	}
}

// WithListOptions appends standard list query parameters (labelSelector, limit, offset) to the route.
func (b *RouteBuilder) WithListOptions() *RouteBuilder {
	for _, p := range ListParameters() {
		b.builder.Param(p)
	}
	return b
}

// WithListParams is an alias for WithListOptions.
func (b *RouteBuilder) WithListParams() *RouteBuilder {
	return b.WithListOptions()
}

// Params appends multiple Parameters to the RouteBuilder.
func (b *RouteBuilder) Params(params ...Parameter) *RouteBuilder {
	for _, p := range params {
		b.builder.Param(p)
	}
	return b
}

// WebService wraps go-restful's *restful.WebService for a specific API groupVersion and resource tag
// (modeled after KubeSphere pkg/apiserver/runtime.NewWebService).
type WebService struct {
	groupVersion string
	rootPath     string
	tag          string
	ws           *restful.WebService
	builders     []*restful.RouteBuilder
}

func NewWebService(groupVersion string, tag string) *WebService {
	rootPath := "/apis/" + strings.Trim(groupVersion, "/")
	ws := new(restful.WebService)
	ws.Path(rootPath).
		Consumes(restful.MIME_JSON, "*/*").
		Produces(restful.MIME_JSON, "application/x-ndjson", "text/plain", "*/*")
	return &WebService{
		groupVersion: groupVersion,
		rootPath:     rootPath,
		tag:          tag,
		ws:           ws,
	}
}

func (w *WebService) RootPath() string { return w.rootPath }

func (w *WebService) Route(b *RouteBuilder) *WebService {
	if b.tag != "" {
		b.builder.Metadata("tag", b.tag)
	}
	w.builders = append(w.builders, b.builder)
	return w
}

func (w *WebService) GET(subPath string) *RouteBuilder {
	return &RouteBuilder{builder: w.ws.GET(subPath), tag: w.tag}
}

func (w *WebService) POST(subPath string) *RouteBuilder {
	return &RouteBuilder{builder: w.ws.POST(subPath), tag: w.tag}
}

func (w *WebService) PUT(subPath string) *RouteBuilder {
	return &RouteBuilder{builder: w.ws.PUT(subPath), tag: w.tag}
}

func (w *WebService) PATCH(subPath string) *RouteBuilder {
	return &RouteBuilder{builder: w.ws.PATCH(subPath), tag: w.tag}
}

func (w *WebService) DELETE(subPath string) *RouteBuilder {
	return &RouteBuilder{builder: w.ws.DELETE(subPath), tag: w.tag}
}

func (w *WebService) PathParameter(name, description string) Parameter {
	return restful.PathParameter(name, description).Required(true)
}

func (w *WebService) QueryParameter(name, description string) Parameter {
	return restful.QueryParameter(name, description)
}

// Container wraps go-restful's *restful.Container and manages GroupVersion WebServices.
type Container struct {
	mux              *http.ServeMux
	restfulContainer *restful.Container
	byRootPath       map[string]*restful.WebService
}

func NewContainer(mux *http.ServeMux) *Container {
	if mux == nil {
		mux = http.NewServeMux()
	}
	rc := restful.NewContainer()
	rc.ServeMux = mux
	rc.Router(restful.CurlyRouter{})
	return &Container{
		mux:              mux,
		restfulContainer: rc,
		byRootPath:       make(map[string]*restful.WebService),
	}
}

func (c *Container) ServeMux() *http.ServeMux {
	return c.mux
}

func (c *Container) RestfulContainer() *restful.Container {
	return c.restfulContainer
}

// Add registers all routes of a resource WebService into the underlying go-restful Container,
// grouping routes that share the same GroupVersion rootPath into a single *restful.WebService.
func (c *Container) Add(ws *WebService) *Container {
	sharedWS, ok := c.byRootPath[ws.rootPath]
	if !ok {
		sharedWS = new(restful.WebService)
		sharedWS.Path(ws.rootPath).
			Consumes(restful.MIME_JSON, "*/*").
			Produces(restful.MIME_JSON, "application/x-ndjson", "text/plain", "*/*")
		c.byRootPath[ws.rootPath] = sharedWS
		for _, b := range ws.builders {
			sharedWS.Route(b)
		}
		c.restfulContainer.Add(sharedWS)
		return c
	}
	for _, b := range ws.builders {
		sharedWS.Route(b)
	}
	return c
}

// ListParameters returns standard query parameters from the WebService.
func (w *WebService) ListParameters() []Parameter {
	return ListParameters()
}

// ParseListOptions extracts and validates standard query parameters (?labelSelector=, ?fieldSelector=, ?limit=, ?offset=, ?continue=)
// and extracts the tenant path/query parameter if present.
// Bounds: 1 <= limit <= 1000 (default 100), offset/continue >= 0 (default 0).
func ParseListOptions(r *http.Request) (db.ListOptions, error) {
	opts := db.ListOptions{Limit: 100, Offset: 0}
	if r == nil {
		return opts, nil
	}
	q := r.URL.Query()
	if t := r.PathValue("tenant"); t != "" {
		opts.Tenant = t
	} else if t := q.Get("tenant"); t != "" {
		opts.Tenant = t
	}
	if err := parsePaginationParams(q, &opts); err != nil {
		return opts, err
	}
	if selectorStr := strings.TrimSpace(q.Get("labelSelector")); selectorStr != "" {
		selector, err := metav1.ParseLabelSelector(selectorStr)
		if err != nil {
			return opts, fmt.Errorf("invalid labelSelector parameter: %w", err)
		}
		opts.LabelSelector = selector
	}
	if fieldSelectorStr := strings.TrimSpace(q.Get("fieldSelector")); fieldSelectorStr != "" {
		fs, err := parseFieldSelectorQuery(fieldSelectorStr)
		if err != nil {
			return opts, err
		}
		opts.FieldSelector = fs
	}
	return opts, nil
}

func parsePaginationParams(q url.Values, opts *db.ListOptions) error {
	if limitStr := q.Get("limit"); limitStr != "" {
		v, err := strconv.Atoi(limitStr)
		if err != nil {
			return fmt.Errorf("invalid limit parameter %q: must be an integer", limitStr)
		}
		if v < 1 || v > 1000 {
			return fmt.Errorf("limit must be between 1 and 1000, got %d", v)
		}
		opts.Limit = v
	}
	hasOffset := false
	if offsetStr := q.Get("offset"); offsetStr != "" {
		v, err := strconv.Atoi(offsetStr)
		if err != nil {
			return fmt.Errorf("invalid offset parameter %q: must be an integer", offsetStr)
		}
		if v < 0 {
			return fmt.Errorf("offset must be greater than or equal to 0, got %d", v)
		}
		opts.Offset = v
		hasOffset = true
	}
	if contStr := strings.TrimSpace(q.Get("continue")); contStr != "" {
		v, err := strconv.Atoi(contStr)
		if err != nil {
			return fmt.Errorf("invalid continue parameter %q: must be a non-negative integer", contStr)
		}
		if v < 0 {
			return fmt.Errorf("continue must be greater than or equal to 0, got %d", v)
		}
		if hasOffset && v != opts.Offset {
			return fmt.Errorf("conflicting offset (%d) and continue (%d) parameters", opts.Offset, v)
		}
		opts.Offset = v
	}
	return nil
}

// fieldSelectorSetSuffix marks set-based selector keys (e.g. metadata.name.in=a,b,c)
// whose comma-separated values continue across top-level term separators.
const fieldSelectorSetSuffix = ".in"

func parseFieldSelectorQuery(fieldSelectorStr string) (map[string]string, error) {
	fs := make(map[string]string)
	setKey := ""
	for _, part := range strings.Split(fieldSelectorStr, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if !strings.Contains(part, "=") && setKey != "" {
			fs[setKey] += "," + part
			continue
		}
		k, v, err := parseFieldSelectorTerm(part)
		if err != nil {
			return nil, err
		}
		fs[k] = v
		setKey = ""
		if strings.HasSuffix(k, fieldSelectorSetSuffix) {
			setKey = k
		}
	}
	if len(fs) == 0 {
		return nil, nil
	}
	return fs, nil
}

func parseFieldSelectorTerm(part string) (string, string, error) {
	var k, v string
	switch {
	case strings.Contains(part, "!="):
		kv := strings.SplitN(part, "!=", 2)
		k, v = strings.TrimSpace(kv[0])+"!=", strings.TrimSpace(kv[1])
	case strings.Contains(part, "=="):
		kv := strings.SplitN(part, "==", 2)
		k, v = strings.TrimSpace(kv[0]), strings.TrimSpace(kv[1])
	case strings.Contains(part, "="):
		kv := strings.SplitN(part, "=", 2)
		k, v = strings.TrimSpace(kv[0]), strings.TrimSpace(kv[1])
	default:
		return "", "", fmt.Errorf("invalid fieldSelector term %q: expected key=value", part)
	}
	if strings.TrimSuffix(k, "!=") == "" {
		return "", "", fmt.Errorf("invalid fieldSelector term %q: empty key", part)
	}
	return k, v, nil
}
