package session

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	iamv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/iam/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/config"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/cache"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/models"
	usermodel "github.com/smartcat999/game-panel-cloud/pkg/models/iam/user"
	"github.com/smartcat999/game-panel-cloud/pkg/repo"
	userdb "github.com/smartcat999/game-panel-cloud/pkg/repo/iam/user"
)

const (
	cacheKeyPrefix        = "gp:iam:session:"
	userSessionsPrefix    = "gp:iam:user-sessions:"
	nodeBootstrapTokenTTL = 365 * 24 * time.Hour
)

// Config is an alias for config.AuthConfig managed centrally in pkg/config.
type Config = config.AuthConfig

func NewConfig() *Config {
	return config.NewAuthConfig()
}

// Principal represents a verified caller identity resolved from a valid, cache-backed session token.
type Principal struct {
	SessionID string
	Kind      string
	Name      string
}

// Operator encapsulates Session creation, cache-backed persistence (memory/redis),
// JWT token issuance, stateful validity verification, and session revocation.
type Operator interface {
	Create(ctx context.Context, req *iamv1.Session) (*iamv1.Session, error)
	Get(ctx context.Context, sessionID string) (*iamv1.Session, error)
	ListByUser(ctx context.Context, username string) (*metav1.List[*iamv1.Session], error)
	Delete(ctx context.Context, sessionID string) error
	RevokeByUser(ctx context.Context, username string) error
	RevokeOthersByUser(ctx context.Context, username, keepSessionID string) error
	Issue(ctx context.Context, kind, name, defaultTenant string) (*iamv1.Session, error)
	RotateNode(ctx context.Context, nodeName string) (*iamv1.Session, error)
	Verify(ctx context.Context, rawToken string) (Principal, error)
}

type sessionClaims struct {
	jwt.RegisteredClaims
	SID  string `json:"sid"`
	Kind string `json:"kind"`
	// IssuedAtNano is the issue time with nanosecond precision, compared against the user's
	// revocation watermark (jwt "iat" only has second precision).
	IssuedAtNano int64 `json:"ian,omitempty"`
}

// userRevokedPrefix and nodeRevokedPrefix store the UnixNano time of the last revocation
// or credential rotation. Any earlier token is rejected immediately.
const (
	userRevokedPrefix = "gp:iam:revoked:"
	nodeRevokedPrefix = "gp:iam:node-revoked:"
)

type operator struct {
	db     *userdb.DB
	cache  cache.Interface
	secret []byte
	ttl    time.Duration
}

func NewOperator(reg *repo.Registry, cacheStore cache.Interface, cfg *Config) Operator {
	if cfg == nil {
		cfg = &config.Get().Auth
	}
	cfg.Default()
	op := &operator{
		cache:  cacheStore,
		secret: []byte(cfg.JWTSecret),
		ttl:    cfg.TokenTTL,
	}
	if reg != nil {
		op.db = userdb.NewDB(reg.Pool())
	}
	return op
}

func (o *operator) Create(ctx context.Context, req *iamv1.Session) (*iamv1.Session, error) {
	ident := strings.TrimSpace(req.Identifier)
	if ident == "" || req.Password == "" {
		return nil, fmt.Errorf("%w: identifier and password are required", models.ErrBadRequest)
	}

	var matched *iamv1.User
	if u, err := o.db.Get(ctx, ident); err == nil && u != nil {
		matched = u
	} else if err != nil && !errors.Is(err, db.ErrNotFound) {
		return nil, err
	} else if strings.Contains(ident, "@") {
		users, err := o.db.List(ctx, db.ListOptions{
			FieldSelector: map[string]string{"spec.email": ident},
			Limit:         1,
		})
		if err != nil {
			return nil, err
		}
		if len(users.Items) > 0 {
			matched = users.Items[0]
		}
	}
	if matched == nil || matched.Spec.State == iamv1.UserStateDisabled || !usermodel.VerifyPassword(matched.Spec.PasswordHash, req.Password) {
		return nil, models.ErrUnauthorized
	}

	now := time.Now().UTC()
	matched.Metadata.StatusVersion = ""
	matched.Status.LastLoginTime = &now
	if matched.Status.DefaultTenant == "" {
		matched.Status.DefaultTenant = matched.Metadata.Name
	}
	_, _ = o.db.UpdateStatus(ctx, matched)

	sess, err := o.Issue(ctx, iamv1.SubjectKindUser, matched.Metadata.Name, matched.Status.DefaultTenant)
	if err != nil {
		return nil, err
	}
	if len(req.Metadata.Annotations) > 0 {
		sess.Metadata.Annotations = maps.Clone(req.Metadata.Annotations)
		stored := *sess
		stored.Token = ""
		if raw, mErr := json.Marshal(&stored); mErr == nil {
			_ = o.cache.Set(ctx, cacheKeyPrefix+sess.Metadata.Name, string(raw), o.ttl)
		}
		o.evictSameDeviceSessions(ctx, matched.Metadata.Name, sess.Metadata.Name, req.Metadata.Annotations)
	}
	return sess, nil
}

func (o *operator) withUserIndexLock(ctx context.Context, username string, fn func()) {
	if o == nil || o.cache == nil {
		return
	}
	if unlock, ok := cache.LockOn(ctx, o.cache, "sess:idx:lock:"+username, 5*time.Second, 500*time.Millisecond); ok {
		defer unlock()
	}
	fn()
}

func (o *operator) evictSameDeviceSessions(ctx context.Context, username, keepSID string, ann map[string]string) {
	ua := strings.TrimSpace(ann[iamv1.AnnotationSessionUserAgent])
	ip := strings.TrimSpace(ann[iamv1.AnnotationSessionClientIP])
	if ua == "" {
		return
	}
	o.withUserIndexLock(ctx, username, func() {
		o.evictSameDeviceUnlocked(ctx, username, keepSID, ua, ip)
	})
}

func (o *operator) evictSameDeviceUnlocked(ctx context.Context, username, keepSID, ua, ip string) {
	indexKey := userSessionsPrefix + username
	rawList, err := o.cache.Get(ctx, indexKey)
	if err != nil {
		return
	}
	var sids []string
	if json.Unmarshal([]byte(rawList), &sids) != nil {
		return
	}
	var active []string
	for _, sid := range sids {
		if sid == keepSID {
			active = append(active, sid)
			continue
		}
		old, gErr := o.Get(ctx, sid)
		if gErr != nil || old == nil {
			continue
		}
		oldUA := strings.TrimSpace(old.Metadata.Annotations[iamv1.AnnotationSessionUserAgent])
		oldIP := strings.TrimSpace(old.Metadata.Annotations[iamv1.AnnotationSessionClientIP])
		if oldUA == ua && (ip == "" || oldIP == "" || oldIP == ip) {
			_ = o.cache.Delete(ctx, cacheKeyPrefix+sid)
			continue
		}
		active = append(active, sid)
	}
	if encoded, mErr := json.Marshal(active); mErr == nil {
		_ = o.cache.Set(ctx, indexKey, string(encoded), o.ttl)
	}
}

func (o *operator) Issue(ctx context.Context, kind, name, defaultTenant string) (*iamv1.Session, error) {
	return o.issueAt(ctx, kind, name, defaultTenant, time.Now().UTC())
}

func (o *operator) RotateNode(ctx context.Context, nodeName string) (*iamv1.Session, error) {
	trimmed := strings.TrimSpace(nodeName)
	if trimmed == "" {
		return nil, fmt.Errorf("%w: node name is required", models.ErrBadRequest)
	}
	now := time.Now().UTC()
	watermark := now.UnixNano() - 1
	if err := o.cache.Set(ctx, nodeRevokedPrefix+trimmed, strconv.FormatInt(watermark, 10), nodeBootstrapTokenTTL+time.Minute); err != nil {
		return nil, err
	}
	return o.issueAtWithTTL(ctx, iamv1.SubjectKindNode, trimmed, "", now, nodeBootstrapTokenTTL)
}

func (o *operator) issueAt(ctx context.Context, kind, name, defaultTenant string, now time.Time) (*iamv1.Session, error) {
	return o.issueAtWithTTL(ctx, kind, name, defaultTenant, now, o.ttl)
}

func (o *operator) issueAtWithTTL(ctx context.Context, kind, name, defaultTenant string, now time.Time, ttl time.Duration) (*iamv1.Session, error) {
	if kind == "" {
		kind = iamv1.SubjectKindUser
	}
	if ttl <= 0 {
		ttl = o.ttl
	}
	exp := now.Add(ttl)
	sid := fmt.Sprintf("%s-%s", name, randomHex(8))
	jti := sid
	if kind == iamv1.SubjectKindNode && name != "" {
		sid = "node-" + name
		jti = fmt.Sprintf("%s-%s", sid, randomHex(4))
	}

	claims := sessionClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ID: jti, Subject: name, IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(exp),
		},
		SID: sid, Kind: kind, IssuedAtNano: now.UnixNano(),
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(o.secret)
	if err != nil {
		return nil, err
	}
	sess := &iamv1.Session{
		TypeMeta:      metav1.TypeMeta{APIVersion: iamv1.GroupVersion, Kind: "Session"},
		Metadata:      metav1.ObjectMeta{Name: sid, CreationTimestamp: now},
		Kind:          kind,
		Identifier:    name,
		Token:         token,
		User:          name,
		DefaultTenant: defaultTenant,
		ExpiresAt:     exp,
	}
	stored := *sess
	stored.Token = ""
	raw, err := json.Marshal(&stored)
	if err != nil {
		return nil, err
	}
	if err := o.cache.Set(ctx, cacheKeyPrefix+sid, string(raw), ttl); err != nil {
		return nil, err
	}
	if kind == iamv1.SubjectKindNode && name != "" {
		_ = o.cache.Set(ctx, iamv1.CacheKeyPrefixNodeToken+name, token, nodeBootstrapTokenTTL)
	}
	if kind == iamv1.SubjectKindUser && name != "" {
		o.indexUserSession(ctx, name, sid)
	}
	return sess, nil
}

func (o *operator) indexUserSession(ctx context.Context, username, sid string) {
	o.withUserIndexLock(ctx, username, func() {
		o.indexUserSessionUnlocked(ctx, username, sid)
	})
}

func (o *operator) indexUserSessionUnlocked(ctx context.Context, username, sid string) {
	indexKey := userSessionsPrefix + username
	var active []string
	if rawList, err := o.cache.Get(ctx, indexKey); err == nil {
		var existing []string
		if json.Unmarshal([]byte(rawList), &existing) == nil {
			for _, id := range existing {
				if id == sid {
					continue
				}
				if exists, err := o.cache.Exists(ctx, cacheKeyPrefix+id); err == nil && exists {
					active = append(active, id)
				}
			}
		}
	}
	active = append(active, sid)
	if encoded, err := json.Marshal(active); err == nil {
		_ = o.cache.Set(ctx, indexKey, string(encoded), o.ttl)
	}
}

func (o *operator) Get(ctx context.Context, sessionID string) (*iamv1.Session, error) {
	raw, err := o.cache.Get(ctx, cacheKeyPrefix+sessionID)
	if err != nil {
		if errors.Is(err, cache.ErrNotFound) {
			return nil, models.ErrNotFound
		}
		return nil, err
	}
	var sess iamv1.Session
	if err := json.Unmarshal([]byte(raw), &sess); err != nil {
		return nil, err
	}
	// Do not leak raw token on GET inspection; only return session metadata/status summary.
	sess.Token = ""
	return &sess, nil
}

func (o *operator) ListByUser(ctx context.Context, username string) (*metav1.List[*iamv1.Session], error) {
	username = strings.TrimSpace(username)
	now := time.Now().UTC()
	var items []*iamv1.Session
	seen := make(map[string]bool)
	indexKey := userSessionsPrefix + username
	if rawList, err := o.cache.Get(ctx, indexKey); err == nil {
		var sids []string
		if json.Unmarshal([]byte(rawList), &sids) == nil {
			for _, sid := range sids {
				if sid == "" || seen[sid] {
					continue
				}
				sess, gErr := o.Get(ctx, sid)
				if gErr != nil || sess == nil || sess.User != username || !sess.ExpiresAt.After(now) {
					continue
				}
				ua := strings.TrimSpace(sess.Metadata.Annotations[iamv1.AnnotationSessionUserAgent])
				if ua == "" || strings.HasPrefix(strings.ToLower(ua), "curl/") || strings.HasPrefix(strings.ToLower(ua), "python") {
					continue
				}
				seen[sid] = true
				items = append(items, sess)
			}
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].Metadata.CreationTimestamp.After(items[j].Metadata.CreationTimestamp)
	})
	return &metav1.List[*iamv1.Session]{
		TypeMeta: metav1.TypeMeta{APIVersion: iamv1.GroupVersion, Kind: "SessionList"},
		Metadata: metav1.ListMeta{TotalCount: len(items)},
		Items:    items,
	}, nil
}

func (o *operator) RevokeOthersByUser(ctx context.Context, username, keepSessionID string) error {
	username = strings.TrimSpace(username)
	if username == "" {
		return nil
	}
	o.withUserIndexLock(ctx, username, func() {
		o.revokeOthersByUserUnlocked(ctx, username, keepSessionID)
	})
	return nil
}

func (o *operator) revokeOthersByUserUnlocked(ctx context.Context, username, keepSessionID string) {
	indexKey := userSessionsPrefix + username
	var remaining []string
	if rawList, err := o.cache.Get(ctx, indexKey); err == nil {
		var sids []string
		if json.Unmarshal([]byte(rawList), &sids) == nil {
			for _, sid := range sids {
				if sid == keepSessionID {
					remaining = append(remaining, sid)
					continue
				}
				_ = o.cache.Delete(ctx, cacheKeyPrefix+sid)
			}
		}
	}
	if len(remaining) == 0 && keepSessionID != "" {
		remaining = []string{keepSessionID}
	}
	if encoded, err := json.Marshal(remaining); err == nil {
		_ = o.cache.Set(ctx, indexKey, string(encoded), o.ttl)
	}
}

func (o *operator) Delete(ctx context.Context, sessionID string) error {
	exists, err := o.cache.Exists(ctx, cacheKeyPrefix+sessionID)
	if err != nil {
		return err
	}
	if !exists {
		return models.ErrNotFound
	}
	return o.cache.Delete(ctx, cacheKeyPrefix+sessionID)
}

func (o *operator) RevokeByUser(ctx context.Context, username string) error {
	username = strings.TrimSpace(username)
	if username == "" {
		return nil
	}
	// Watermark first: it invalidates every earlier token even if the index below missed some
	// sessions (concurrent logins race on the read-modify-write of that list).
	if err := o.cache.Set(ctx, userRevokedPrefix+username, strconv.FormatInt(time.Now().UTC().UnixNano(), 10), o.ttl+time.Minute); err != nil {
		return err
	}
	o.withUserIndexLock(ctx, username, func() {
		o.revokeByUserUnlocked(ctx, username)
	})
	return nil
}

func (o *operator) revokeByUserUnlocked(ctx context.Context, username string) {
	indexKey := userSessionsPrefix + username
	if rawList, err := o.cache.Get(ctx, indexKey); err == nil {
		var sids []string
		if json.Unmarshal([]byte(rawList), &sids) == nil {
			for _, sid := range sids {
				_ = o.cache.Delete(ctx, cacheKeyPrefix+sid)
			}
		}
	}
	_ = o.cache.Delete(ctx, indexKey)
}

func (o *operator) Verify(ctx context.Context, rawToken string) (Principal, error) {
	var claims sessionClaims
	parsed, err := jwt.ParseWithClaims(
		rawToken,
		&claims,
		func(token *jwt.Token) (any, error) {
			return o.secret, nil
		},
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithStrictDecoding(),
	)
	if err != nil || !parsed.Valid {
		return Principal{}, errors.New("invalid or expired token")
	}
	sid := claims.SID
	if sid == "" {
		sid = claims.ID
	}
	if sid == "" {
		return Principal{}, errors.New("token missing session id")
	}

	// Verify session existence in cache (memory or redis) to guarantee unrevoked & active token validity.
	exists, err := o.cache.Exists(ctx, cacheKeyPrefix+sid)
	if err != nil {
		return Principal{}, err
	}
	if !exists {
		return Principal{}, errors.New("session expired or revoked")
	}

	kind := claims.Kind
	if kind == "" {
		kind = iamv1.SubjectKindUser
	}
	if kind == iamv1.SubjectKindUser && o.isRevokedByWatermark(ctx, userRevokedPrefix+claims.Subject, claims) {
		return Principal{}, errors.New("session expired or revoked")
	}
	if kind == iamv1.SubjectKindNode && o.isRevokedByWatermark(ctx, nodeRevokedPrefix+claims.Subject, claims) {
		return Principal{}, errors.New("session expired or revoked")
	}
	return Principal{
		SessionID: sid,
		Kind:      kind,
		Name:      claims.Subject,
	}, nil
}

func (o *operator) isRevokedByWatermark(ctx context.Context, key string, claims sessionClaims) bool {
	raw, err := o.cache.Get(ctx, key)
	if err != nil {
		return false
	}
	revokedAt, _ := strconv.ParseInt(raw, 10, 64)
	issuedAt := claims.IssuedAtNano
	if issuedAt == 0 && claims.IssuedAt != nil {
		issuedAt = claims.IssuedAt.UnixNano()
	}
	return revokedAt > 0 && issuedAt <= revokedAt
}

func randomHex(bytesLen int) string {
	b := make([]byte, bytesLen)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
