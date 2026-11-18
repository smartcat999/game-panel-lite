package log

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/config"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
)

func startMockVictoriaLogsServer(t *testing.T, records []map[string]any) *string {
	t.Helper()
	var mu sync.Mutex
	var lastQuery string
	offsetRe := regexp.MustCompile(`\|\s*offset\s+(\d+)`)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/select/logsql/query" {
			http.NotFound(w, r)
			return
		}
		query := r.URL.Query().Get("query")
		w.Header().Set("Content-Type", "application/stream+json")
		matched := filterMockVLRecords(records, query)
		if strings.Contains(query, "| stats count() as total") {
			_, _ = fmt.Fprintf(w, "{\"total\":%q}\n", strconv.Itoa(len(matched)))
			return
		}
		mu.Lock()
		lastQuery = query
		mu.Unlock()
		start := 0
		if m := offsetRe.FindStringSubmatch(query); len(m) == 2 {
			if parsed, err := strconv.Atoi(m[1]); err == nil && parsed > 0 {
				start = parsed
			}
		}
		limit := len(matched)
		if lStr := r.URL.Query().Get("limit"); lStr != "" {
			if parsed, err := strconv.Atoi(lStr); err == nil && parsed > 0 {
				limit = parsed
			}
		}
		end := min(len(matched), start+limit)
		if start < end {
			for _, rec := range matched[start:end] {
				if raw, err := json.Marshal(rec); err == nil {
					_, _ = w.Write(append(raw, '\n'))
				}
			}
		}
	}))
	origVL := config.Get().Observability.VictoriaLogsURL
	config.Get().Observability.VictoriaLogsURL = srv.URL
	t.Cleanup(func() {
		config.Get().Observability.VictoriaLogsURL = origVL
		srv.Close()
	})
	return &lastQuery
}

func filterMockVLRecords(records []map[string]any, rawQuery string) []map[string]any {
	filterExpr := strings.TrimSpace(strings.SplitN(rawQuery, "|", 2)[0])
	if filterExpr == "" || filterExpr == "*" {
		return records
	}
	var out []map[string]any
	for _, rec := range records {
		if mockMatchesFilter(rec, filterExpr) {
			out = append(out, rec)
		}
	}
	return out
}

func mockMatchesFilter(rec map[string]any, expr string) bool {
	expr = strings.TrimSpace(expr)
	if expr == "" || expr == "*" {
		return true
	}
	if strings.HasPrefix(expr, "(") && strings.HasSuffix(expr, ")") && isBalancedParens(expr[1:len(expr)-1]) {
		return mockMatchesFilter(rec, expr[1:len(expr)-1])
	}
	if terms := splitTopLevelLogical(expr, " AND "); len(terms) > 1 {
		for _, term := range terms {
			if !mockMatchesFilter(rec, term) {
				return false
			}
		}
		return true
	}
	if branches := splitTopLevelLogical(expr, " OR "); len(branches) > 1 {
		for _, br := range branches {
			if mockMatchesFilter(rec, br) {
				return true
			}
		}
		return false
	}
	if strings.HasPrefix(expr, "NOT ") {
		return !mockMatchesFilter(rec, expr[4:])
	}
	return mockMatchesAtom(rec, expr)
}

func mockMatchesAtom(rec map[string]any, atom string) bool {
	if strings.HasPrefix(atom, "_time:>=") {
		cutoff, err1 := time.Parse(time.RFC3339Nano, strings.TrimPrefix(atom, "_time:>="))
		ts, err2 := time.Parse(time.RFC3339Nano, fmt.Sprint(rec["_time"]))
		return err1 == nil && err2 == nil && !ts.Before(cutoff)
	}
	field, rhs, ok := strings.Cut(atom, ":")
	if !ok {
		return true
	}
	val, exists := rec[field]
	valStr := ""
	if exists && val != nil {
		valStr = fmt.Sprint(val)
	}
	if rhs == "*" {
		return strings.TrimSpace(valStr) != ""
	}
	if strings.HasPrefix(rhs, "~") {
		pat, err := strconv.Unquote(rhs[1:])
		if err != nil {
			return false
		}
		re, err := regexp.Compile(pat)
		return err == nil && re.MatchString(valStr)
	}
	want, err := strconv.Unquote(rhs)
	return err == nil && valStr == want
}

func splitTopLevelLogical(expr, sep string) []string {
	var parts []string
	depth, inQuote, escaped, start := 0, false, false, 0
	for i := 0; i < len(expr); i++ {
		ch := expr[i]
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' && inQuote {
			escaped = true
			continue
		}
		if ch == '"' {
			inQuote = !inQuote
			continue
		}
		if !inQuote {
			if ch == '(' {
				depth++
			} else if ch == ')' && depth > 0 {
				depth--
			} else if depth == 0 && strings.HasPrefix(expr[i:], sep) {
				parts = append(parts, expr[start:i])
				start = i + len(sep)
				i += len(sep) - 1
			}
		}
	}
	parts = append(parts, expr[start:])
	return parts
}

func isBalancedParens(s string) bool {
	depth, inQuote, escaped := 0, false, false
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' && inQuote {
			escaped = true
			continue
		}
		if ch == '"' {
			inQuote = !inQuote
		} else if !inQuote {
			switch ch {
			case '(':
				depth++
			case ')':
				depth--
				if depth < 0 {
					return false
				}
			}
		}
	}
	return depth == 0
}

func componentSeedLogs() []map[string]any {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return []map[string]any{
		{"_time": now, "service": "apiserver", "level": "INFO", "_msg": "instance started", "tenant": "alpha", "instance": "mc-1", "container": "gamepanel-apiserver", "requestId": "req-1"},
		{"_time": now, "service": "node-agent", "level": "ERROR", "_msg": "container dial failed", "tenant": "alpha", "instance": "mc-2", "container": "gamepanel-node-agent", "error": "dial tcp timeout"},
		{"_time": now, "service": "controller-manager", "level": "WARN", "_msg": "reconcile retrying", "tenant": "beta", "resourceName": "dst-1", "container": "gamepanel-controller-manager"},
	}
}

func TestOperatorListQueriesVictoriaLogsAndFilters(t *testing.T) {
	lastQuery := startMockVictoriaLogsServer(t, componentSeedLogs())
	op := NewOperator()
	ctx := context.Background()

	cases := []struct {
		name      string
		opts      QueryOptions
		wantCount int
		wantSvc   string
	}{
		{name: "filter by service", opts: QueryOptions{Service: "node-agent"}, wantCount: 1, wantSvc: "node-agent"},
		{name: "filter by level", opts: QueryOptions{Level: "WARN"}, wantCount: 1, wantSvc: "controller-manager"},
		{name: "filter by tenant", opts: QueryOptions{Tenant: "alpha"}, wantCount: 2},
		{name: "filter by instance via resourceName", opts: QueryOptions{Instance: "dst-1"}, wantCount: 1, wantSvc: "controller-manager"},
		{name: "filter by query in field", opts: QueryOptions{Query: "req-1"}, wantCount: 1, wantSvc: "apiserver"},
		{name: "filter by query in message", opts: QueryOptions{Query: "dial failed"}, wantCount: 1, wantSvc: "node-agent"},
		{name: "recent window keeps fresh entries", opts: QueryOptions{Window: "15m", ListOptions: db.ListOptions{Limit: 10}}, wantCount: 3},
	}

	for _, tc := range cases {
		list, err := op.List(ctx, tc.opts)
		if err != nil {
			t.Fatalf("%s: List returned error: %v", tc.name, err)
		}
		if len(list.Items) != tc.wantCount {
			t.Fatalf("%s: expected %d items, got %d (lastQuery=%q)", tc.name, tc.wantCount, len(list.Items), *lastQuery)
		}
		if tc.wantSvc != "" && list.Items[0].Service != tc.wantSvc {
			t.Fatalf("%s: expected service %q, got %q", tc.name, tc.wantSvc, list.Items[0].Service)
		}
	}
}

func TestResolveLogCutoff(t *testing.T) {
	now := time.Now().UTC()
	for _, tc := range []struct {
		window   string
		expected time.Duration
	}{
		{window: "15m", expected: 15 * time.Minute},
		{window: "1h", expected: time.Hour},
		{window: "7d", expected: 7 * 24 * time.Hour},
	} {
		cutoff := resolveLogCutoff(tc.window, "")
		if cutoff.IsZero() {
			t.Fatalf("window %q returned zero cutoff", tc.window)
		}
		diff := now.Sub(cutoff) - tc.expected
		if diff < -5*time.Second || diff > 5*time.Second {
			t.Fatalf("window %q cutoff off by %v", tc.window, diff)
		}
	}
	if got := resolveLogCutoff("invalid", ""); !got.IsZero() {
		t.Fatalf("expected zero cutoff for invalid window, got %v", got)
	}
	if got := resolveLogCutoff("all", ""); !got.IsZero() {
		t.Fatalf("expected zero cutoff for all window, got %v", got)
	}
	explicit := "2026-11-15T08:00:00Z"
	want, _ := time.Parse(time.RFC3339, explicit)
	if got := resolveLogCutoff("15m", explicit); !got.Equal(want) {
		t.Fatalf("expected explicit since %v, got %v", want, got)
	}
}

func TestOperatorListWindowCutoff(t *testing.T) {
	now := time.Now().UTC()
	startMockVictoriaLogsServer(t, []map[string]any{
		{"_time": now.Add(-5 * time.Minute).Format(time.RFC3339Nano), "service": "apiserver", "level": "INFO", "_msg": "five min"},
		{"_time": now.Add(-45 * time.Minute).Format(time.RFC3339Nano), "service": "apiserver", "level": "WARN", "_msg": "fortyfive min"},
		{"_time": now.Add(-48 * time.Hour).Format(time.RFC3339Nano), "service": "node-agent", "level": "ERROR", "_msg": "two days"},
	})
	op := NewOperator()
	ctx := context.Background()
	for _, tc := range []struct {
		window string
		want   int
	}{
		{window: "15m", want: 1},
		{window: "1h", want: 2},
		{window: "7d", want: 3},
		{window: "invalid", want: 3},
	} {
		res, err := op.List(ctx, QueryOptions{Window: tc.window})
		if err != nil {
			t.Fatalf("window %q returned error: %v", tc.window, err)
		}
		if len(res.Items) != tc.want {
			t.Fatalf("window %q: expected %d entries, got %d", tc.window, tc.want, len(res.Items))
		}
	}
}

func TestOperatorScopeSeparatesInstancesAndComponents(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	lastQuery := startMockVictoriaLogsServer(t, []map[string]any{
		{"_time": now, "service": "apiserver", "level": "INFO", "_msg": "apiserver started", "instance": "gamepanel-apiserver", "container": "gamepanel-apiserver"},
		{"_time": now, "service": "instance", "level": "ERROR", "_msg": "palworld crash", "tenant": "alpha", "instance": "pal-1", "container": "gp-uid-pal-1"},
		{"_time": now, "service": "instance", "level": "INFO", "_msg": "rust started", "tenant": "beta", "instance": "rust-2", "container": "gp-uid-rust-2"},
	})
	op := NewOperator()
	ctx := context.Background()

	allLogs, err := op.List(ctx, QueryOptions{})
	if err != nil || len(allLogs.Items) != 3 {
		t.Fatalf("expected default scope all to return 3 logs, got err=%v items=%+v", err, allLogs)
	}
	wsLogs, err := op.List(ctx, QueryOptions{Scope: corev1.LogScopeInstance, Tenant: "alpha"})
	if err != nil || len(wsLogs.Items) != 1 || wsLogs.Items[0].Service != corev1.LogScopeInstance || wsLogs.Items[0].Fields["instance"] != "pal-1" {
		t.Fatalf("expected only alpha instance log, got err=%v items=%+v", err, wsLogs)
	}
	platLogs, err := op.List(ctx, QueryOptions{Tenant: corev1.LogTenantPlatform})
	if err != nil || len(platLogs.Items) != 1 || platLogs.Items[0].Service != "apiserver" {
		t.Fatalf("expected only platform log, got err=%v items=%+v", err, platLogs)
	}
	if !strings.Contains(*lastQuery, `NOT service:"instance" AND NOT tenant:*`) {
		t.Fatalf("expected LogsQL platform filter, got %q", *lastQuery)
	}
	subLogs, err := op.List(ctx, QueryOptions{Instance: "pal"})
	if err != nil || len(subLogs.Items) != 1 || subLogs.Items[0].Fields["container"] != "gp-uid-pal-1" {
		t.Fatalf("expected substring match on instance/container, got err=%v items=%+v", err, subLogs)
	}
}

func TestTenantScopedLogIsolationRejectsSpecialTenantAliases(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	lastQuery := startMockVictoriaLogsServer(t, []map[string]any{
		{"_time": now, "service": corev1.LogScopeInstance, "level": corev1.LogLevelInfo, "_msg": "alpha msg", "tenant": "alpha", "instance": "pal-1"},
		{"_time": now, "service": "apiserver", "level": corev1.LogLevelInfo, "_msg": "platform msg", "instance": "gamepanel-apiserver"},
	})
	op := NewOperator()
	ctx := context.Background()

	for _, alias := range []string{corev1.LogScopeAll, corev1.LogTenantPlatform, ""} {
		res, err := op.List(ctx, QueryOptions{
			Scope:        corev1.LogScopeInstance,
			Service:      corev1.LogScopeInstance,
			Tenant:       alias,
			TenantScoped: true,
		})
		if err != nil {
			t.Fatalf("List failed for tenant alias %q: %v", alias, err)
		}
		if len(res.Items) != 0 || res.Metadata.TotalCount != 0 {
			t.Fatalf("expected 0 items/total when TenantScoped=true with tenant=%q, got items=%+v meta=%+v", alias, res.Items, res.Metadata)
		}
		if *lastQuery != "" {
			t.Fatalf("expected no VictoriaLogs query for rejected alias %q, got %q", alias, *lastQuery)
		}
	}

	valid, err := op.List(ctx, QueryOptions{
		Scope:        corev1.LogScopeInstance,
		Service:      corev1.LogScopeInstance,
		Tenant:       "alpha",
		TenantScoped: true,
	})
	if err != nil || len(valid.Items) != 1 || valid.Items[0].Fields["tenant"] != "alpha" {
		t.Fatalf("expected alpha tenant log, got err=%v items=%+v", err, valid)
	}
	if !strings.Contains(*lastQuery, `tenant:"alpha"`) {
		t.Fatalf("expected strict LogsQL tenant filter, got %q", *lastQuery)
	}
}

func TestOperatorListReturnsTotalCountAndOffsetPagination(t *testing.T) {
	base := time.Now().UTC()
	var records []map[string]any
	for i := range 5 {
		records = append(records, map[string]any{
			"_time":   base.Add(-time.Duration(i) * time.Second).Format(time.RFC3339Nano),
			"service": "apiserver",
			"level":   "INFO",
			"_msg":    fmt.Sprintf("entry-%d", i),
		})
	}
	lastQuery := startMockVictoriaLogsServer(t, records)
	op := NewOperator()
	ctx := context.Background()

	first, err := op.List(ctx, QueryOptions{ListOptions: db.ListOptions{Limit: 2}})
	if err != nil {
		t.Fatalf("page 1 error: %v", err)
	}
	if first.Metadata.TotalCount != 5 || first.Metadata.Continue != "2" || len(first.Items) != 2 {
		t.Fatalf("unexpected page 1 metadata/items: meta=%+v len=%d", first.Metadata, len(first.Items))
	}

	second, err := op.List(ctx, QueryOptions{ListOptions: db.ListOptions{Limit: 2, Offset: 2}})
	if err != nil {
		t.Fatalf("page 2 error: %v", err)
	}
	if !strings.Contains(*lastQuery, "| offset 2") {
		t.Fatalf("expected VictoriaLogs query to include offset pipe, got %q", *lastQuery)
	}
	if second.Metadata.TotalCount != 5 || second.Metadata.Continue != "4" || len(second.Items) != 2 {
		t.Fatalf("unexpected page 2 metadata/items: meta=%+v len=%d", second.Metadata, len(second.Items))
	}
	if second.Items[0].Message != "entry-2" {
		t.Fatalf("expected entry-2 at offset 2, got %q", second.Items[0].Message)
	}

	last, err := op.List(ctx, QueryOptions{ListOptions: db.ListOptions{Limit: 2, Offset: 4}})
	if err != nil {
		t.Fatalf("last page error: %v", err)
	}
	if last.Metadata.TotalCount != 5 || last.Metadata.Continue != "" || len(last.Items) != 1 {
		t.Fatalf("unexpected last page metadata/items: meta=%+v len=%d", last.Metadata, len(last.Items))
	}
}
