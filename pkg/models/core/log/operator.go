package log

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/config"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
)

type QueryOptions struct {
	ListOptions  db.ListOptions
	Scope        string
	Service      string
	Level        string
	Tenant       string
	TenantScoped bool
	Instance     string
	Window       string
	Since        string
	Query        string
}

type normalizedQuery struct {
	scope        string
	service      string
	level        string
	tenant       string
	tenantScoped bool
	instance     string
	cutoff       time.Time
	query        string
	limit        int
	offset       int
}

type Operator interface {
	List(ctx context.Context, opts QueryOptions) (*metav1.List[*corev1.LogEntry], error)
}

type operator struct {
	restyClient *resty.Client
}

func NewOperator() Operator {
	return &operator{
		restyClient: resty.New().SetTimeout(3 * time.Second),
	}
}

func (o *operator) List(ctx context.Context, qOpts QueryOptions) (*metav1.List[*corev1.LogEntry], error) {
	nq := normalizeLogQueryOptions(qOpts)
	if isInvalidTenantScopedQuery(nq) {
		return &metav1.List[*corev1.LogEntry]{
			TypeMeta: metav1.TypeMeta{APIVersion: corev1.GroupVersion, Kind: "LogEntryList"},
			Metadata: metav1.ListMeta{TotalCount: 0},
			Items:    []*corev1.LogEntry{},
		}, nil
	}
	items := o.queryVictoriaLogs(ctx, nq, nq.query, nq.limit)
	if nq.limit > 0 && len(items) > nq.limit {
		items = items[:nq.limit]
	}
	if items == nil {
		items = []*corev1.LogEntry{}
	}
	total := max(o.countVictoriaLogs(ctx, nq, nq.query), nq.offset+len(items))
	var nextContinue string
	if total > nq.offset+len(items) && len(items) > 0 {
		nextContinue = strconv.Itoa(nq.offset + len(items))
	}
	return &metav1.List[*corev1.LogEntry]{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.GroupVersion, Kind: "LogEntryList"},
		Metadata: metav1.ListMeta{TotalCount: total, Continue: nextContinue},
		Items:    items,
	}, nil
}

func isInvalidTenantScopedQuery(nq normalizedQuery) bool {
	if !nq.tenantScoped {
		return false
	}
	t := strings.TrimSpace(nq.tenant)
	return t == "" || strings.EqualFold(t, corev1.LogScopeAll) || t == corev1.LogTenantPlatform
}

func normalizeLogQueryOptions(qOpts QueryOptions) normalizedQuery {
	opts := qOpts.ListOptions
	serviceFilter := firstNonEmpty(qOpts.Service, opts.FieldSelector["spec.service"], opts.FieldSelector["service"])
	if strings.EqualFold(serviceFilter, corev1.LogScopeAll) {
		serviceFilter = ""
	}
	levelFilter := strings.ToUpper(firstNonEmpty(qOpts.Level, opts.FieldSelector["spec.level"], opts.FieldSelector["level"]))
	if strings.EqualFold(levelFilter, corev1.LogScopeAll) {
		levelFilter = ""
	}
	var tenantFilter string
	if qOpts.TenantScoped {
		tenantFilter = strings.TrimSpace(qOpts.Tenant)
	} else {
		tenantFilter = firstNonEmpty(qOpts.Tenant, opts.Tenant, opts.FieldSelector["tenant"])
		if strings.EqualFold(tenantFilter, corev1.LogScopeAll) {
			tenantFilter = ""
		}
	}
	instanceFilter := firstNonEmpty(qOpts.Instance, opts.FieldSelector["instance"], opts.FieldSelector["name"])
	if strings.EqualFold(instanceFilter, corev1.LogScopeAll) {
		instanceFilter = ""
	}
	sinceVal := firstNonEmpty(qOpts.Since, opts.FieldSelector["since"])
	limit := opts.Limit
	if limit <= 0 {
		limit = 100
	} else if limit > 1000 {
		limit = 1000
	}
	return normalizedQuery{
		scope:        normalizeLogScope(qOpts.Scope, serviceFilter),
		service:      serviceFilter,
		level:        levelFilter,
		tenant:       tenantFilter,
		tenantScoped: qOpts.TenantScoped,
		instance:     instanceFilter,
		cutoff:       resolveLogCutoff(qOpts.Window, sinceVal),
		query:        strings.TrimSpace(qOpts.Query),
		limit:        limit,
		offset:       max(opts.Offset, 0),
	}
}

func normalizeLogScope(rawScope, serviceFilter string) string {
	switch strings.ToLower(strings.TrimSpace(rawScope)) {
	case corev1.LogScopeInstance, "instances":
		return corev1.LogScopeInstance
	case corev1.LogScopeComponents, "component", "platform":
		return corev1.LogScopeComponents
	case corev1.LogScopeAll:
		return corev1.LogScopeAll
	}
	if strings.EqualFold(serviceFilter, corev1.LogScopeInstance) {
		return corev1.LogScopeInstance
	}
	return corev1.LogScopeAll
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}

func resolveLogCutoff(window, since string) time.Time {
	if s := strings.TrimSpace(since); s != "" {
		for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
			if t, err := time.Parse(layout, s); err == nil {
				return t.UTC()
			}
		}
	}
	w := strings.ToLower(strings.TrimSpace(window))
	if w == "" || w == corev1.LogScopeAll {
		return time.Time{}
	}
	if d, ok := corev1.ParseWindowDuration(w); ok && d > 0 {
		return time.Now().UTC().Add(-d)
	}
	return time.Time{}
}

func (o *operator) queryVictoriaLogs(ctx context.Context, nq normalizedQuery, queryText string, limit int) []*corev1.LogEntry {
	vlURL := strings.TrimSpace(config.Get().Observability.VictoriaLogsURL)
	if vlURL == "" {
		return nil
	}
	logsQL := buildVictoriaLogsQuery(nq, queryText)
	qURL := fmt.Sprintf("%s/select/logsql/query?query=%s&limit=%d", strings.TrimRight(vlURL, "/"), url.QueryEscape(logsQL), limit)
	resp, err := o.restyClient.R().
		SetContext(ctx).
		Get(qURL)
	if err != nil || resp == nil || resp.StatusCode() != http.StatusOK {
		return nil
	}
	return parseVictoriaLogsStream(bufio.NewScanner(bytes.NewReader(resp.Body())))
}

func (o *operator) countVictoriaLogs(ctx context.Context, nq normalizedQuery, queryText string) int {
	vlURL := strings.TrimSpace(config.Get().Observability.VictoriaLogsURL)
	if vlURL == "" {
		return 0
	}
	logsQL := buildVictoriaLogsFilter(nq, queryText) + " | stats count() as total"
	qURL := fmt.Sprintf("%s/select/logsql/query?query=%s", strings.TrimRight(vlURL, "/"), url.QueryEscape(logsQL))
	resp, err := o.restyClient.R().
		SetContext(ctx).
		Get(qURL)
	if err != nil || resp == nil || resp.StatusCode() != http.StatusOK {
		return 0
	}
	return parseVictoriaLogsTotal(resp.Body())
}

func parseVictoriaLogsTotal(body []byte) int {
	for _, line := range bytes.Split(body, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var raw map[string]any
		if json.Unmarshal(line, &raw) != nil {
			continue
		}
		switch v := raw["total"].(type) {
		case string:
			if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n >= 0 {
				return n
			}
		case float64:
			if v >= 0 {
				return int(v)
			}
		}
	}
	return 0
}

func buildVictoriaLogsQuery(nq normalizedQuery, queryText string) string {
	q := buildVictoriaLogsFilter(nq, queryText) + " | sort by (_time) desc"
	if nq.offset > 0 {
		q += fmt.Sprintf(" | offset %d", nq.offset)
	}
	return q
}

func buildVictoriaLogsFilter(nq normalizedQuery, queryText string) string {
	var parts []string
	switch {
	case nq.scope == corev1.LogScopeInstance:
		parts = append(parts, fmt.Sprintf(`service:%q`, corev1.LogScopeInstance))
	case nq.service != "":
		parts = append(parts, fmt.Sprintf(`service:%q`, nq.service))
	case nq.scope == corev1.LogScopeComponents:
		parts = append(parts, fmt.Sprintf(`NOT service:%q`, corev1.LogScopeInstance))
	}
	if nq.level != "" {
		upper := strings.ToUpper(nq.level)
		lower := strings.ToLower(nq.level)
		if upper == corev1.LogLevelWarn {
			parts = append(parts, `(level:"WARN" OR level:"warn" OR level:"WARNING" OR level:"warning")`)
		} else {
			parts = append(parts, fmt.Sprintf(`(level:%q OR level:%q)`, upper, lower))
		}
	}
	if nq.tenantScoped {
		parts = append(parts, fmt.Sprintf(`tenant:%q`, nq.tenant))
	} else if nq.tenant == corev1.LogTenantPlatform {
		parts = append(parts, fmt.Sprintf(`NOT service:%q AND NOT tenant:*`, corev1.LogScopeInstance))
	} else if nq.tenant != "" {
		parts = append(parts, fmt.Sprintf(`tenant:%q`, nq.tenant))
	}
	if nq.instance != "" {
		pat := "(?i)" + regexp.QuoteMeta(nq.instance)
		parts = append(parts, fmt.Sprintf(`(instance:~%q OR container:~%q OR service:~%q OR name:~%q OR resourceName:~%q)`,
			pat, pat, pat, pat, pat))
	}
	if !nq.cutoff.IsZero() {
		parts = append(parts, fmt.Sprintf(`_time:>=%s`, nq.cutoff.UTC().Format(time.RFC3339)))
	}
	if queryText != "" {
		pat := "(?i)" + regexp.QuoteMeta(queryText)
		parts = append(parts, fmt.Sprintf(`(_msg:~%q OR requestId:~%q OR name:~%q OR instance:~%q OR tenant:~%q OR resource:~%q OR containerId:~%q OR container:~%q OR error:~%q OR err:~%q)`,
			pat, pat, pat, pat, pat, pat, pat, pat, pat, pat))
	}
	if len(parts) == 0 {
		return "*"
	}
	return strings.Join(parts, " AND ")
}

func parseVictoriaLogsStream(scanner *bufio.Scanner) []*corev1.LogEntry {
	var items []*corev1.LogEntry
	idx := 0
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var raw map[string]any
		if json.Unmarshal(line, &raw) != nil {
			continue
		}
		items = append(items, buildLogEntryFromRaw(raw, &idx))
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].Timestamp.After(items[j].Timestamp)
	})
	return items
}

func buildLogEntryFromRaw(raw map[string]any, idx *int) *corev1.LogEntry {
	tsStr, _ := raw["_time"].(string)
	ts, err := time.Parse(time.RFC3339Nano, tsStr)
	if err != nil {
		ts = time.Now().UTC()
	}
	msg, _ := raw["_msg"].(string)
	svc, _ := raw["service"].(string)
	lvl, _ := raw["level"].(string)
	fields := make(map[string]string)
	for k, v := range raw {
		if !strings.HasPrefix(k, "_") && k != "service" && k != "level" && k != "caller" {
			fields[k] = fmt.Sprintf("%v", v)
		}
	}
	*idx++
	name := fmt.Sprintf("log-%d-%d", ts.UnixNano(), *idx)
	entry := &corev1.LogEntry{
		Metadata:  metav1.ObjectMeta{Name: name, UID: name, CreationTimestamp: ts},
		Level:     strings.ToUpper(lvl),
		Service:   svc,
		Message:   msg,
		Fields:    fields,
		Timestamp: ts,
	}
	entry.DefaultMeta()
	return entry
}
