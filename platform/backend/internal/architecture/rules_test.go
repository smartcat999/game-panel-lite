package architecture

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPlatformProductionSourcePassesRules(t *testing.T) {
	_, currentFile, _, _ := runtime.Caller(0)
	backendRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", ".."))
	files, err := productionFiles(backendRoot)
	if err != nil {
		t.Fatal(err)
	}
	if violations := Check(files); len(violations) > 0 {
		t.Fatalf("architecture violations:\n%s", formatViolations(violations))
	}
}

func TestRulesRejectDeliberateViolations(t *testing.T) {
	files := []SourceFile{
		{Path: "/platform/backend/internal/identity/service.go", Content: "package identity\nimport _ \"example.test/apps/api/internal/model\""},
		{Path: "/platform/backend/internal/workspace/service.go", Content: "package workspace\nimport _ \"example.test/platform/backend/internal/identity/repository\""},
		{Path: "/platform/backend/internal/commerce/query.sql", Content: "SELECT * FROM orders JOIN payments ON payments.order_id = orders.id"},
	}
	violations := Check(files)
	if len(violations) != 3 {
		t.Fatalf("expected all three deliberate violations, got %d:\n%s", len(violations), formatViolations(violations))
	}
}

func TestRebaselineRulesRejectDeliberateViolations(t *testing.T) {
	files := []SourceFile{
		{Path: "/platform/backend/internal/example/model.go", Content: "package example\ntype Session struct { IsAdmin bool }\ntype Plan struct{}"},
		{Path: "/platform/backend/internal/example/handler.go", Content: "package example\nfunc handle() { authorize() }\nfunc authorize() {}"},
		{Path: "/platform/backend/internal/example/repository.go", Content: "package example\nfunc load(db DB, ids []string) { for range ids { db.QueryContext() } }\ntype DB interface { QueryContext() }"},
		{Path: "/platform/backend/internal/example/routes.go", Content: "package example\nconst route = `/v1/workspaces/{workspaceId}/worlds`"},
	}
	violations := CheckRebaseline(files)
	if len(violations) != 5 {
		t.Fatalf("expected all five rebaseline violations, got %d:\n%s", len(violations), formatViolations(violations))
	}
}

func TestPlatformFrontendDoesNotReferenceLegacyFrontend(t *testing.T) {
	_, currentFile, _, _ := runtime.Caller(0)
	frontendRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "..", "..", "frontend"))
	err := filepath.WalkDir(frontendRoot, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == "node_modules" || entry.Name() == ".next" || entry.Name() == "test-results" {
				return filepath.SkipDir
			}
			return nil
		}
		extension := filepath.Ext(path)
		if extension != ".ts" && extension != ".tsx" && extension != ".css" {
			return nil
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		for _, legacyReference := range []string{"apps/web", "@/../apps", "../../../apps"} {
			if strings.Contains(string(content), legacyReference) {
				t.Errorf("%s references legacy frontend path %q", path, legacyReference)
			}
		}
		for _, forbidden := range []string{"is_admin", "IsAdmin", "PlatformOperator", "pending_payment", "/plans", "/orders", "/payments", "/entitlements", "/worlds"} {
			if strings.Contains(string(content), forbidden) {
				t.Errorf("%s contains forbidden rebaseline frontend term %q", path, forbidden)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func productionFiles(root string) ([]SourceFile, error) {
	var files []SourceFile
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == "testdata" || entry.Name() == "architecture" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, "_test.go") || (!strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, ".sql")) {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files = append(files, SourceFile{Path: path, Content: string(content)})
		return nil
	})
	return files, err
}

func formatViolations(violations []Violation) string {
	lines := make([]string, 0, len(violations))
	for _, violation := range violations {
		lines = append(lines, violation.Error())
	}
	return strings.Join(lines, "\n")
}
