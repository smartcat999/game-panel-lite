package apiserver

import (
	"context"
	"crypto/subtle"
	"net/http"
	"strings"
	"time"

	iamv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/iam/v1"
	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/apiserver/authentication"
	"github.com/smartcat999/game-panel-cloud/pkg/apiserver/authorization"
	"github.com/smartcat999/game-panel-cloud/pkg/apiserver/filters"
	"github.com/smartcat999/game-panel-cloud/pkg/apiserver/request"
	apiruntime "github.com/smartcat999/game-panel-cloud/pkg/apiserver/runtime"
	"github.com/smartcat999/game-panel-cloud/pkg/config"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/cache"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/log"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/metrics"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/mq"
	promclient "github.com/smartcat999/game-panel-cloud/pkg/infra/prometheus"
	drvruntime "github.com/smartcat999/game-panel-cloud/pkg/infra/runtime"
	_ "github.com/smartcat999/game-panel-cloud/pkg/kapis/core/v1"
	_ "github.com/smartcat999/game-panel-cloud/pkg/kapis/iam/v1"
	_ "github.com/smartcat999/game-panel-cloud/pkg/kapis/registry/v1"
	sessionmodel "github.com/smartcat999/game-panel-cloud/pkg/models/iam/session"
	"github.com/smartcat999/game-panel-cloud/pkg/repo"
)

// ServerConfig is an alias for config.ServerConfig managed centrally in pkg/config.
type ServerConfig = config.ServerConfig

func NewServerConfig() *ServerConfig {
	return config.NewServerConfig()
}

// Server is the Kubernetes/KubeSphere-style declarative HTTP API Server.
type Server struct {
	srvCfg        *ServerConfig
	reg           *repo.Registry
	cache         cache.Interface
	mq            mq.Interface
	drivers       *drvruntime.Registry
	sessions      sessionmodel.Operator
	authenticator authentication.Request
	authorizer    authorization.Authorizer
	container     *apiruntime.Container
	handlerChain  http.Handler
}

func NewServer(srvCfg *ServerConfig, authCfg *AuthConfig, reg *repo.Registry, cacheStore cache.Interface, drivers *drvruntime.Registry, bus mq.Interface) *Server {
	if srvCfg == nil {
		srvCfg = &config.Get().Server
	}
	srvCfg.Default()
	if authCfg == nil {
		authCfg = &config.Get().Auth
	}
	authCfg.Default()
	if cacheStore != nil {
		cache.SetDefault(cacheStore)
		promclient.SetDefaultCache(cacheStore, 5*time.Second)
	}
	sessions := sessionmodel.NewOperator(reg, cacheStore, authCfg)
	authn := newServerAuthenticator(sessions)
	authz := authorization.NewUnionAuthorizer(
		authorization.NewPathAuthorizer(),
		authorization.NewRBACAuthorizer(reg, drivers, bus),
	)
	resolver := request.NewRequestInfoResolver()
	container := apiruntime.NewContainer(http.NewServeMux())
	s := &Server{
		srvCfg: srvCfg, reg: reg, cache: cacheStore, mq: bus, drivers: drivers,
		sessions: sessions, authenticator: authn, authorizer: authz, container: container,
	}
	s.installRoutes()
	s.handlerChain = DefaultBuildHandlerChain(s.container.ServeMux(), resolver, s.authenticator, s.authorizer, s.cache, s.mq, s.reg)
	return s
}

func newServerAuthenticator(sessions sessionmodel.Operator) authentication.Request {
	return authentication.NewUnionAuthenticator(
		authentication.NewBearerTokenAuthenticator(authentication.TokenVerifierFunc(func(ctx context.Context, rawToken string) (request.UserInfo, error) {
			p, err := sessions.Verify(ctx, rawToken)
			if err == nil {
				return request.UserInfo{SessionID: p.SessionID, Kind: p.Kind, Name: p.Name, Groups: []string{request.GroupAuthenticated}}, nil
			}
			agentToken := strings.TrimSpace(config.Get().Agent.Token)
			if agentToken != "" && subtle.ConstantTimeCompare([]byte(rawToken), []byte(agentToken)) == 1 {
				return request.UserInfo{SessionID: "agent-static-token", Kind: request.KindNode, Name: iamv1.PrincipalNodeAgent, Groups: []string{request.GroupAuthenticated}}, nil
			}
			workerToken := strings.TrimSpace(config.Get().Registry.WorkerToken)
			if workerToken != "" && subtle.ConstantTimeCompare([]byte(rawToken), []byte(workerToken)) == 1 {
				return request.UserInfo{SessionID: "registry-worker-static-token", Kind: request.KindSystem, Name: registryv1.PrincipalRegistryWorker, Groups: []string{request.GroupAuthenticated}}, nil
			}
			return request.UserInfo{}, err
		})),
		authentication.NewAnonymousAuthenticator(),
	)
}

// DefaultBuildHandlerChain wraps the API mux with Kubernetes/KubeSphere-style HTTP filters
// (PanicRecovery -> MaxRequestBodyBytes -> RequestInfo -> Authentication -> RateLimit -> RequestLog -> Authorization -> Mux).
func DefaultBuildHandlerChain(
	apiHandler http.Handler,
	resolver request.RequestInfoResolver,
	authn authentication.Request,
	authz authorization.Authorizer,
	cacheStores ...any,
) http.Handler {
	var cacheStore cache.Interface
	for _, dep := range cacheStores {
		if v, ok := dep.(cache.Interface); ok {
			cacheStore = v
		}
	}
	handler := apiHandler
	handler = filters.WithAuthorization(handler, authz)
	handler = filters.WithRequestLog(handler)
	handler = filters.WithDistributedRateLimit(handler, cacheStore)
	handler = filters.WithAuthentication(handler, authn)
	handler = filters.WithRequestInfo(handler, resolver)
	handler = filters.WithMaxRequestBodyBytes(handler, filters.DefaultMaxRequestBodyBytes)
	handler = filters.WithPanicRecovery(handler)
	return handler
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.handlerChain.ServeHTTP(w, r)
}

func (s *Server) Sessions() sessionmodel.Operator {
	return s.sessions
}

func (s *Server) MQ() mq.Interface {
	return s.mq
}

// Run starts the HTTP server using its internal ServerConfig and shuts down gracefully when ctx is cancelled.
func (s *Server) Run(ctx context.Context) error {
	httpServer := &http.Server{
		Addr:         s.srvCfg.Addr,
		Handler:      s,
		ReadTimeout:  s.srvCfg.ReadTimeout,
		WriteTimeout: s.srvCfg.WriteTimeout,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), s.srvCfg.ShutdownTimeout)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
	}()

	log.Info("starting apiserver", "addr", s.srvCfg.Addr)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (s *Server) installRoutes() {
	mux := s.container.ServeMux()
	livenessHandler := func(w http.ResponseWriter, _ *http.Request) {
		apiruntime.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
	mux.HandleFunc("GET /healthz", livenessHandler)
	mux.HandleFunc("GET /livez", livenessHandler)
	mux.HandleFunc("GET /readyz", s.handleReadyz)
	mux.Handle("GET /metrics", metrics.Handler())

	apiruntime.InstallAll(apiruntime.Dependencies{
		DB:       s.reg,
		Cache:    s.cache,
		MQ:       s.mq,
		Drivers:  s.drivers,
		Sessions: s.sessions,
		CanI:     s.canI,
	}, s.container)

	openAPISpec := s.container.BuildOpenAPISpec()
	mux.HandleFunc("GET /openapi/v3", func(w http.ResponseWriter, _ *http.Request) {
		apiruntime.WriteJSON(w, http.StatusOK, openAPISpec)
	})
}

// canI runs the full authorizer chain for the caller on ctx against a synthetic resource request.
func (s *Server) canI(ctx context.Context, apiGroup, resource, verb, name string) bool {
	u, ok := request.UserFrom(ctx)
	if !ok || u.IsAnonymous() {
		return false
	}
	decision, _, err := s.authorizer.Authorize(ctx, authorization.AttributesRecord{
		User:              u,
		IsResourceRequest: true,
		Verb:              verb,
		APIGroup:          apiGroup,
		APIVersion:        "v1",
		Resource:          resource,
		Name:              name,
	})
	return err == nil && decision == authorization.DecisionAllow
}

func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	checks := map[string]string{
		"storage": "ok",
		"cache":   "ok",
		"mq":      "ok",
	}
	ready := true
	if err := s.reg.Ping(ctx); err != nil {
		checks["storage"] = err.Error()
		ready = false
	}
	if err := s.cache.Ping(ctx); err != nil {
		checks["cache"] = err.Error()
		ready = false
	}
	if err := s.mq.Ping(ctx); err != nil {
		checks["mq"] = err.Error()
		ready = false
	}
	status := http.StatusOK
	state := "ready"
	if !ready {
		status = http.StatusServiceUnavailable
		state = "not_ready"
	}
	apiruntime.WriteJSON(w, status, map[string]any{
		"status": state,
		"checks": checks,
	})
}
