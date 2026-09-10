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

func TestJoinRuleOnlyInspectsSQLLiterals(t *testing.T) {
	files := []SourceFile{
		{Path: "/platform/backend/internal/example/model.go", Content: "package example\nconst purpose = `join`"},
		{Path: "/platform/backend/internal/example/query.go", Content: "package example\nconst query = `SELECT * FROM instances JOIN nodes ON nodes.id = instances.node_id`"},
	}
	violations := Check(files)
	if len(violations) != 1 || violations[0].File != files[1].Path {
		t.Fatalf("expected only the SQL literal to fail, got %#v", violations)
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

func TestWorldRuleAllowsProviderRuntimePaths(t *testing.T) {
	files := []SourceFile{{Path: "/platform/backend/internal/gameprovider/provider.go", Content: "package gameprovider\nconst dataPath = `/data/Worlds`"}}
	if violations := CheckRebaseline(files); len(violations) != 0 {
		t.Fatalf("provider runtime path was mistaken for a World HTTP route: %#v", violations)
	}
}

func TestRebaselineNewSourcePassesRules(t *testing.T) {
	_, currentFile, _, _ := runtime.Caller(0)
	backendRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", ".."))
	var files []SourceFile
	for _, relative := range []string{"internal/accessapi", "internal/authentication", "internal/authorization", "internal/billing", "internal/billingapi", "internal/deliveryapi", "internal/deliverycontrol", "internal/deliveryworker", "internal/eventtransport", "internal/gameprovider", "internal/httpfilter", "internal/instanceaction", "internal/instanceconfiguration", "internal/instanceobservability", "internal/instanceprovisioning", "internal/messaging", "internal/nodeworkload", "internal/productioncatalog", "internal/productionseed", "internal/providercontract", "internal/regionaldelivery", "internal/regionaltask", "internal/runtimeprovider", "internal/workspaceapi", "migrations/global/0005_identity_authorization_rebaseline.sql", "migrations/global/0006_resource_pricing_wallet.sql", "migrations/global/0007_async_delivery.sql", "migrations/global/0008_provider_driven_operation.sql", "migrations/region/0004_async_delivery.sql", "migrations/region/0005_provider_driven_operation.sql", "migrations/region/0006_node_task_ownership.sql"} {
		path := filepath.Join(backendRoot, relative)
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if !info.IsDir() {
			content, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			files = append(files, SourceFile{Path: path, Content: string(content)})
			continue
		}
		err = filepath.WalkDir(path, func(sourcePath string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || strings.HasSuffix(sourcePath, "_test.go") || !strings.HasSuffix(sourcePath, ".go") {
				return nil
			}
			content, readErr := os.ReadFile(sourcePath)
			if readErr != nil {
				return readErr
			}
			files = append(files, SourceFile{Path: sourcePath, Content: string(content)})
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if violations := CheckRebaseline(files); len(violations) > 0 {
		t.Fatalf("rebaseline architecture violations:\n%s", formatViolations(violations))
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

func TestProviderDrivenFrontendHasNoGameSpecificBranches(t *testing.T) {
	_, currentFile, _, _ := runtime.Caller(0)
	frontendRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "..", "..", "frontend"))
	for _, relative := range []string{"features/prototype/workspace-pages.tsx", "lib/prototype-store.tsx"} {
		path := filepath.Join(frontendRoot, relative)
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, branch := range []string{`.game ===`, `.game !==`, `supportsMods`, `provider ===`, `provider !==`, `"Terraria" | "tModLoader"`} {
			if strings.Contains(string(content), branch) {
				t.Errorf("%s contains game-specific branch or duplicate capability %q", path, branch)
			}
		}
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
