package session

import (
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"strconv"
	"strings"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	iamv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/iam/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/apiserver/request"
	apiruntime "github.com/smartcat999/game-panel-cloud/pkg/apiserver/runtime"
	sessionmodel "github.com/smartcat999/game-panel-cloud/pkg/models/iam/session"
)

const (
	AnnotationSessionUserAgent = iamv1.AnnotationSessionUserAgent
	AnnotationSessionClientIP  = iamv1.AnnotationSessionClientIP
	AnnotationSessionCurrent   = iamv1.AnnotationSessionCurrent
)

type handler struct {
	op   sessionmodel.Operator
	canI apiruntime.AccessChecker
}

func newHandler(op sessionmodel.Operator, canI apiruntime.AccessChecker) *handler {
	return &handler{op: op, canI: canI}
}

// BootstrapNodePrincipal is the identity of the shared node bootstrap token (GP_AGENT_TOKEN).
// It may only be exchanged for a per-node session; it carries no other permission.
const BootstrapNodePrincipal = iamv1.PrincipalNodeAgent

// nodeTokenDenied decides who may mint a Node session token for nodeName:
//   - a User allowed to create nodes (platform administrators);
//   - the shared bootstrap token, exchanging itself for a per-node identity at agent start-up;
//   - a Node renewing its own token.
//
// Ordinary users must never be able to mint a token and impersonate a node.
func (h *handler) nodeTokenDenied(r *http.Request, nodeName string) string {
	u, ok := request.UserFrom(r.Context())
	if !ok || u.IsAnonymous() {
		return "issuing a Node session token requires authentication"
	}
	switch u.Kind {
	case iamv1.SubjectKindNode:
		if u.Name == BootstrapNodePrincipal || u.Name == nodeName {
			return ""
		}
		return "a node may only renew its own session token"
	case iamv1.SubjectKindUser:
		if h.canI != nil && h.canI(r.Context(), corev1.GroupName, "nodes", iamv1.VerbCreate, nodeName) {
			return ""
		}
	}
	return "issuing a Node session token requires a platform administrator"
}

func (h *handler) CreateSession(w http.ResponseWriter, r *http.Request) {
	var req iamv1.Session
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apiruntime.WriteError(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}
	if req.Kind == iamv1.SubjectKindNode {
		h.handleCreateNodeSession(w, r, req.Identifier)
		return
	}
	if req.Metadata.Annotations == nil {
		req.Metadata.Annotations = make(map[string]string)
	}
	if ua := strings.TrimSpace(r.UserAgent()); ua != "" {
		req.Metadata.Annotations[AnnotationSessionUserAgent] = ua
	}
	if ip := extractClientIP(r); ip != "" {
		req.Metadata.Annotations[AnnotationSessionClientIP] = ip
	}
	session, err := h.op.Create(r.Context(), &req)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	apiruntime.WriteJSON(w, http.StatusCreated, session)
}

func (h *handler) handleCreateNodeSession(w http.ResponseWriter, r *http.Request, identifier string) {
	if identifier == "" {
		apiruntime.WriteError(w, http.StatusBadRequest, "identifier (node name) is required when kind is Node")
		return
	}
	if reason := h.nodeTokenDenied(r, identifier); reason != "" {
		apiruntime.WriteError(w, http.StatusForbidden, reason)
		return
	}
	var (
		sess *iamv1.Session
		err  error
	)
	if u, ok := request.UserFrom(r.Context()); ok && u.Kind == iamv1.SubjectKindUser {
		sess, err = h.op.RotateNode(r.Context(), identifier)
	} else {
		sess, err = h.op.Issue(r.Context(), iamv1.SubjectKindNode, identifier, "")
	}
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	apiruntime.WriteJSON(w, http.StatusCreated, sess)
}

func (h *handler) ListSessions(w http.ResponseWriter, r *http.Request) {
	u, ok := requireUserSessionCaller(w, r)
	if !ok {
		return
	}
	opts, err := apiruntime.ParseListOptions(r)
	if err != nil {
		apiruntime.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validateSessionFieldSelector(opts.FieldSelector); err != nil {
		apiruntime.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	list, err := h.op.ListByUser(r.Context(), u.Name)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	hasCurrent := false
	for _, item := range list.Items {
		if item != nil && item.Metadata.Name == u.SessionID {
			hasCurrent = true
			markCurrentSession(item)
		}
	}
	if !hasCurrent && u.SessionID != "" {
		if cur, gErr := h.op.Get(r.Context(), u.SessionID); gErr == nil && cur != nil && cur.User == u.Name {
			markCurrentSession(cur)
			list.Items = append([]*iamv1.Session{cur}, list.Items...)
		}
	}
	filtered := filterSessions(list.Items, opts.FieldSelector, opts.LabelSelector)
	list.Metadata.TotalCount = len(filtered)
	list.Items, list.Metadata.Continue = paginateSessions(filtered, opts.Offset, opts.Limit)
	apiruntime.WriteJSON(w, http.StatusOK, list)
}

func validateSessionFieldSelector(fs map[string]string) error {
	for k, v := range fs {
		if v == "" {
			return fmt.Errorf("invalid fieldSelector term %q: empty value", k)
		}
		switch k {
		case "metadata.name", "metadata.name!=", "user", "kind":
		default:
			return fmt.Errorf("unsupported session fieldSelector %q", k)
		}
	}
	return nil
}

func filterSessions(items []*iamv1.Session, fs map[string]string, ls *metav1.LabelSelector) []*iamv1.Session {
	filtered := make([]*iamv1.Session, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		if ls != nil && !ls.Matches(item.Metadata.Labels) {
			continue
		}
		if !matchesSessionFieldSelector(item, fs) {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}

func matchesSessionFieldSelector(item *iamv1.Session, fs map[string]string) bool {
	for k, v := range fs {
		switch k {
		case "metadata.name":
			if item.Metadata.Name != v {
				return false
			}
		case "metadata.name!=":
			if item.Metadata.Name == v {
				return false
			}
		case "user":
			if item.User != v {
				return false
			}
		case "kind":
			if item.Kind != v {
				return false
			}
		}
	}
	return true
}

func paginateSessions(items []*iamv1.Session, offset, limit int) ([]*iamv1.Session, string) {
	total := len(items)
	if offset >= total {
		return []*iamv1.Session{}, ""
	}
	end := total
	if limit > 0 && offset+limit < total {
		end = offset + limit
	}
	var cont string
	if end < total {
		cont = strconv.Itoa(end)
	}
	return items[offset:end], cont
}

func markCurrentSession(s *iamv1.Session) {
	s.Metadata.Annotations = maps.Clone(s.Metadata.Annotations)
	if s.Metadata.Annotations == nil {
		s.Metadata.Annotations = make(map[string]string)
	}
	s.Metadata.Annotations[AnnotationSessionCurrent] = "true"
}

func (h *handler) GetSession(w http.ResponseWriter, r *http.Request) {
	sid := strings.TrimSpace(r.PathValue("name"))
	if sid == "" {
		apiruntime.WriteError(w, http.StatusBadRequest, "session name is required")
		return
	}
	session, err := h.op.Get(r.Context(), sid)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	if !h.canAccessSession(r, session, iamv1.VerbGet) {
		apiruntime.WriteError(w, http.StatusForbidden, "cannot inspect another user's session")
		return
	}
	if u, ok := request.UserFrom(r.Context()); ok && session.Metadata.Name == u.SessionID {
		markCurrentSession(session)
	}
	apiruntime.WriteJSON(w, http.StatusOK, session)
}

func (h *handler) DeleteSessions(w http.ResponseWriter, r *http.Request) {
	u, ok := requireUserSessionCaller(w, r)
	if !ok {
		return
	}
	eqName, neqName, err := parseSessionFieldSelector(r.URL.Query().Get("fieldSelector"))
	if err != nil {
		apiruntime.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if eqName != "" {
		sess, gErr := h.op.Get(r.Context(), eqName)
		if gErr != nil {
			apiruntime.WriteModelError(w, gErr)
			return
		}
		if !h.canAccessSession(r, sess, iamv1.VerbDelete) {
			apiruntime.WriteError(w, http.StatusForbidden, "cannot revoke another user's session")
			return
		}
		err = h.op.Delete(r.Context(), eqName)
	} else if neqName != "" {
		err = h.op.RevokeOthersByUser(r.Context(), u.Name, neqName)
	} else {
		err = h.op.RevokeByUser(r.Context(), u.Name)
	}
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) DeleteSession(w http.ResponseWriter, r *http.Request) {
	sid := strings.TrimSpace(r.PathValue("name"))
	if sid == "" {
		apiruntime.WriteError(w, http.StatusBadRequest, "session name is required")
		return
	}
	sess, err := h.op.Get(r.Context(), sid)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	if !h.canAccessSession(r, sess, iamv1.VerbDelete) {
		apiruntime.WriteError(w, http.StatusForbidden, "cannot revoke another user's session")
		return
	}
	if err := h.op.Delete(r.Context(), sid); err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func requireUserSessionCaller(w http.ResponseWriter, r *http.Request) (request.UserInfo, bool) {
	u, ok := request.UserFrom(r.Context())
	if !ok || u.IsAnonymous() || u.Name == "" {
		apiruntime.WriteError(w, http.StatusUnauthorized, "authentication required")
		return request.UserInfo{}, false
	}
	if u.Kind == iamv1.SubjectKindNode {
		apiruntime.WriteError(w, http.StatusForbidden, "node principals cannot manage user login sessions")
		return request.UserInfo{}, false
	}
	return u, true
}

func parseSessionFieldSelector(raw string) (eqName, neqName string, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", nil
	}
	for _, term := range strings.Split(raw, ",") {
		term = strings.TrimSpace(term)
		if term == "" {
			continue
		}
		var key, val string
		var isNeq bool
		switch {
		case strings.Contains(term, "!="):
			parts := strings.SplitN(term, "!=", 2)
			key, val, isNeq = strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), true
		case strings.Contains(term, "=="):
			parts := strings.SplitN(term, "==", 2)
			key, val = strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		case strings.Contains(term, "="):
			parts := strings.SplitN(term, "=", 2)
			key, val = strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		default:
			return "", "", fmt.Errorf("invalid fieldSelector term %q: expected metadata.name=value or metadata.name!=value", term)
		}
		if key != "metadata.name" || val == "" {
			return "", "", fmt.Errorf("unsupported fieldSelector term %q: only metadata.name is supported", term)
		}
		if isNeq {
			neqName = val
		} else {
			eqName = val
		}
	}
	return eqName, neqName, nil
}

func (h *handler) canAccessSession(r *http.Request, sess *iamv1.Session, verb string) bool {
	u, ok := request.UserFrom(r.Context())
	if !ok || u.IsAnonymous() || sess == nil {
		return false
	}
	if u.SessionID != "" && u.SessionID == sess.Metadata.Name {
		return true
	}
	if u.Kind != iamv1.SubjectKindNode && sess.Kind != iamv1.SubjectKindNode && u.Name != "" && u.Name == sess.User {
		return true
	}
	return h.canI != nil && h.canI(r.Context(), iamv1.GroupName, "sessions", verb, sess.Metadata.Name)
}

func extractClientIP(r *http.Request) string {
	return request.ClientIP(r)
}
