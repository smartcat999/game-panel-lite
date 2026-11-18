package filters

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-redis/redis_rate/v10"
	"github.com/redis/go-redis/v9"

	iamv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/iam/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/apiserver/request"
	apiruntime "github.com/smartcat999/game-panel-cloud/pkg/apiserver/runtime"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/cache"
)

type redisClientProvider interface {
	RedisClient() *redis.Client
}

func extractClientIP(req *http.Request) string {
	return request.ClientIP(req)
}

// WithDistributedRateLimit enforces distributed GCRA rate limiting via github.com/go-redis/redis_rate/v10.
// It protects authentication endpoints (/sessions) against brute-force attacks and mutating
// endpoints against request floods while exempting node-agent control-plane traffic.
func WithDistributedRateLimit(handler http.Handler, store cache.Interface) http.Handler {
	var redisLimiter *redis_rate.Limiter
	if p, ok := store.(redisClientProvider); ok && p.RedisClient() != nil {
		redisLimiter = redis_rate.NewLimiter(p.RedisClient())
	}
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		default:
			handler.ServeHTTP(w, req)
			return
		}
		user, _ := request.UserFrom(req.Context())
		if user.Kind == request.KindNode || strings.HasPrefix(user.Name, "system:node") {
			handler.ServeHTTP(w, req)
			return
		}
		bucketKey, perMin, burst := resolveRateLimitBucket(req, user)
		if !checkRateLimitAllowed(w, req, redisLimiter, bucketKey, perMin, burst) {
			return
		}
		handler.ServeHTTP(w, req)
	})
}

func resolveRateLimitBucket(req *http.Request, user request.UserInfo) (string, int, int) {
	info, _ := request.RequestInfoFrom(req.Context())
	clientIP := extractClientIP(req)
	if info != nil && info.Resource == "sessions" && info.Verb == iamv1.VerbCreate {
		return "rl:login:" + clientIP, 60, 60
	}
	actor := user.Name
	if actor == "" || user.IsAnonymous() {
		actor = clientIP
	}
	return "rl:write:" + actor, 600, 200
}

func checkRateLimitAllowed(
	w http.ResponseWriter, req *http.Request,
	redisLimiter *redis_rate.Limiter,
	bucketKey string, perMin, burst int,
) bool {
	if redisLimiter == nil {
		return true
	}
	res, err := redisLimiter.Allow(req.Context(), bucketKey, redis_rate.Limit{Rate: perMin, Burst: burst, Period: time.Minute})
	if err == nil && res.Allowed == 0 {
		retrySec := max(1, int(res.RetryAfter.Seconds()))
		w.Header().Set("Retry-After", strconv.Itoa(retrySec))
		apiruntime.WriteError(w, http.StatusTooManyRequests, fmt.Sprintf("请求过于频繁，请 %d 秒后再试", retrySec))
		return false
	}
	return true
}
