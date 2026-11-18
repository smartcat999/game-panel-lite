package event

import (
	"fmt"
	"maps"
	"net/http"
	"strings"
	"time"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	apiruntime "github.com/smartcat999/game-panel-cloud/pkg/apiserver/runtime"
	"github.com/smartcat999/game-panel-cloud/pkg/config"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	eventmodel "github.com/smartcat999/game-panel-cloud/pkg/models/core/event"
)

type handler struct {
	op eventmodel.Operator
}

func newHandler(op eventmodel.Operator) *handler {
	return &handler{op: op}
}

func (h *handler) ListClusterEvents(w http.ResponseWriter, r *http.Request) {
	opts, err := parseEventOptions(r)
	if err != nil {
		apiruntime.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	list, err := h.op.List(r.Context(), opts)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	w.Header().Set("X-GamePanel-Audit-Level", resolveConfiguredAuditLevel())
	apiruntime.WriteJSON(w, http.StatusOK, list)
}

func (h *handler) ListTenantEvents(w http.ResponseWriter, r *http.Request) {
	opts, err := parseEventOptions(r)
	if err != nil {
		apiruntime.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	opts.Tenant = r.PathValue("tenant")
	list, err := h.op.List(r.Context(), opts)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	w.Header().Set("X-GamePanel-Audit-Level", resolveConfiguredAuditLevel())
	apiruntime.WriteJSON(w, http.StatusOK, list)
}

func resolveConfiguredAuditLevel() string {
	return corev1.NormalizeAuditLevel(config.Get().Observability.AuditLevel)
}

func parseEventOptions(r *http.Request) (db.ListOptions, error) {
	opts, err := apiruntime.ParseListOptions(r)
	if err != nil {
		return opts, err
	}
	if strings.EqualFold(strings.TrimSpace(opts.Tenant), "all") {
		opts.Tenant = ""
	}
	q := r.URL.Query()
	since, err := resolveEventSince(q.Get("window"), q.Get("since"))
	if err != nil {
		return opts, err
	}
	opts.FieldSelector = maps.Clone(opts.FieldSelector)
	setEventField(&opts, "since", since)
	setEventField(&opts, "resource", q.Get("resource"))
	setEventField(&opts, "verb", q.Get("verb"))
	setEventField(&opts, "actor", q.Get("actor"))
	return opts, nil
}

func setEventField(opts *db.ListOptions, key, raw string) {
	v := strings.TrimSpace(raw)
	if v == "" || strings.EqualFold(v, "all") {
		return
	}
	if opts.FieldSelector == nil {
		opts.FieldSelector = make(map[string]string)
	}
	opts.FieldSelector[key] = v
}

func resolveEventSince(window, since string) (string, error) {
	if s := strings.TrimSpace(since); s != "" {
		for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
			if t, err := time.Parse(layout, s); err == nil {
				return t.UTC().Format(time.RFC3339Nano), nil
			}
		}
		return "", fmt.Errorf("invalid since timestamp %q", s)
	}
	w := strings.ToLower(strings.TrimSpace(window))
	if w == "" || w == "all" {
		return "", nil
	}
	d, ok := corev1.ParseWindowDuration(w)
	if !ok {
		return "", fmt.Errorf("invalid window %q", window)
	}
	return time.Now().UTC().Add(-d).Format(time.RFC3339Nano), nil
}
