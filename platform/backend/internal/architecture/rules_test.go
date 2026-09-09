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
