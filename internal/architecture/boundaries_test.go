package architecture

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

const api = "github.com/smartcat999/game-panel-lite/apps/api/internal/"

func TestBackendImportBoundaries(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate repository")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "../.."))
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if path != root && filepath.Dir(path) == root && entry.Name() != "apps" && entry.Name() != "internal" {
				return filepath.SkipDir
			}
			if entry.Name() == "node_modules" || entry.Name() == ".next" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, spec := range parsed.Imports {
			imported, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return err
			}
			reason := forbiddenImport(relative, imported)
			if reason == "" {
				continue
			}
			t.Errorf("%s imports %s: %s", relative, imported, reason)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

}

func forbiddenImport(file, imported string) string {
	if strings.HasPrefix(file, "internal/archive/") && (strings.Contains(imported, ".") || imported == "os" || strings.HasPrefix(imported, "database/")) {
		return "shared archive writer must use scoped filesystem and standard-library contracts only"
	}
	under := func(dir string) bool { return strings.HasPrefix(file, "apps/api/internal/"+dir+"/") }
	if under("entitlements") && (strings.Contains(imported, ".") || imported == "os" || imported == "net/http" || strings.HasPrefix(imported, "database/")) {
		return "entitlements must use standard-library domain rules only"
	}
	if under("backupingress") && ((strings.Contains(imported, ".") && imported != api+"backup" && imported != api+"regional") || imported == "os" || strings.HasPrefix(imported, "database/")) {
		return "backup ingress must depend on consumer-owned persistence ports and wire models"
	}
	if strings.HasPrefix(imported, "github.com/aws/") && !under("s3archive") && !strings.HasPrefix(file, "apps/api/cmd/") {
		return "object storage SDK belongs in its adapter or composition root"
	}
	if under("s3archive") && ((strings.Contains(imported, ".") && !strings.HasPrefix(imported, "github.com/aws/") && imported != api+"assets" && imported != api+"backup") || imported == "os" || strings.HasPrefix(imported, "database/")) {
		return "S3 archive adapter must use backup contracts, not persistence or runtime"
	}
	if under("transferauth") && ((strings.Contains(imported, ".") && imported != api+"instances" && imported != api+"regional") || imported == "net/http" || imported == "os" || strings.HasPrefix(imported, "database/")) {
		return "transfer authorization must use source-resolution contracts and local keys, not concrete adapters"
	}
	if under("assetfiles") && ((strings.Contains(imported, ".") && imported != api+"assets") || imported == "net/http" || strings.HasPrefix(imported, "database/")) {
		return "regional asset files must depend on authorized manifest contracts, not transport or persistence"
	}
	if under("assets") && (strings.Contains(imported, ".") || imported == "net/http" || imported == "os" || strings.HasPrefix(imported, "database/")) {
		return "global asset contracts must not depend on regional storage or persistence adapters"
	}
	if under("regions") && (strings.Contains(imported, ".") || imported == "net/http" || imported == "os" || strings.HasPrefix(imported, "database/")) {
		return "global region directory models must not depend on regional infrastructure or adapters"
	}
	if under("instanceapp") && ((strings.Contains(imported, ".") && imported != api+"instances") || imported == "os" || imported == "net/http" || strings.HasPrefix(imported, "database/")) {
		return "instance application orchestration must depend on consumer-owned ports and intent contracts"
	}
	if under("configprotection") && ((strings.Contains(imported, ".") && imported != api+"instances") || imported == "os" || imported == "net/http" || strings.HasPrefix(imported, "database/")) {
		return "configuration cryptography must depend on immutable identity contracts, not storage, transport or process secrets"
	}
	if under("serviceauth") && (strings.Contains(imported, ".") || strings.HasPrefix(imported, "database/") || imported == "os") {
		return "service identity verification must not depend on persistence or process configuration"
	}
	if under("controlclient") && ((strings.Contains(imported, ".") && imported != api+"instances" && imported != api+"regional" && imported != api+"assets" && imported != api+"backup" && imported != api+"entitlements") || imported == "os" || strings.HasPrefix(imported, "database/")) {
		return "regional control client must depend on wire models, not persistence, runtime or process configuration"
	}
	if under("nodeapi") && strings.Contains(imported, ".") && imported != "github.com/go-chi/chi/v5" && imported != api+"regional" && imported != api+"serviceauth" {
		return "node API must depend on authenticated identity and consumer-owned ports, not persistence or runtime"
	}
	if under("controlapi") && strings.Contains(imported, ".") && imported != "github.com/go-chi/chi/v5" && imported != api+"instances" && imported != api+"regional" && imported != api+"serviceauth" && imported != api+"assets" && imported != api+"backup" && imported != api+"backupingress" && imported != api+"entitlements" {
		return "control HTTP endpoints must use consumer-owned ports rather than concrete persistence or runtime"
	}
	if under("instances") && (strings.Contains(imported, ".") || imported == "net/http" || strings.HasPrefix(imported, "database/") || imported == "os") {
		return "global instance models must not depend on legacy domain, persistence or runtime"
	}
	if under("delivery") && (strings.Contains(imported, ".") || imported == "net/http" || strings.HasPrefix(imported, "database/") || imported == "os") {
		return "delivery orchestration must depend on consumer-owned interfaces, not persistence or broker adapters"
	}
	if under("regional") && ((strings.Contains(imported, ".") && imported != api+"instances" && imported != api+"assets" && imported != "github.com/smartcat999/game-panel-lite/internal/workload") || imported == "net/http" || imported == "os" || strings.HasPrefix(imported, "database/")) {
		return "regional intake must depend on message contracts and consumer-owned ports, not concrete adapters"
	}
	if under("scheduling") && (strings.Contains(imported, ".") || imported == "net/http" || imported == "os" || strings.HasPrefix(imported, "database/")) {
		return "placement rules must be independent of domain persistence, transport and runtime adapters"
	}
	if strings.HasPrefix(file, "internal/workload/") && (strings.Contains(imported, ".") || imported == "net/http") {
		return "shared workload protocol must be infrastructure independent"
	}
	if strings.HasPrefix(file, "internal/worker/") && (strings.HasPrefix(imported, api) || strings.Contains(imported, "/internal/runtime/") || imported == "net/http") {
		return "worker coordinates runtime interfaces without transport or adapter dependencies"
	}
	if strings.HasPrefix(imported, "github.com/smartcat999/game-panel-lite/internal/runtime/") && !under("app") && !under("runtime/docker") && file != "apps/agent/main.go" && !strings.HasPrefix(file, "internal/runtime/") {
		return "concrete shared runtime belongs in an adapter or composition root"
	}
	if strings.HasPrefix(file, "internal/runtime/") && strings.HasPrefix(imported, api) {
		return "shared runtimes cannot depend on control-plane internals"
	}

	if strings.HasPrefix(imported, "github.com/docker/") && !under("runtime/docker") && !strings.HasPrefix(file, "internal/runtime/docker/") {
		return "Docker SDK belongs to the Docker RuntimeAdapter"
	}
	if strings.HasPrefix(imported, "gorm.io/") && !under("store") {
		return "GORM belongs to persistence adapters"
	}
	if imported == "github.com/rabbitmq/amqp091-go" && !under("messaging/rabbitmq") {
		return "RabbitMQ client belongs to its messaging adapter"
	}
	if (file == "apps/api/internal/http/mod_config_handlers.go" || file == "apps/api/internal/http/mod_library_handlers.go") && (imported == "os" || imported == "path/filepath" || imported == api+"safety") {
		return "mod configuration file operations belong to modruntime"
	}
	if (under("modcatalog") || under("modruntime") || under("modlibrary") || under("gameconfig")) && (imported == "net/http" || strings.HasPrefix(imported, api+"http") || strings.HasPrefix(imported, api+"server") || strings.HasPrefix(imported, api+"store") || strings.HasPrefix(imported, api+"runtime")) {
		return "shared configuration and mod rules must not depend on transport, lifecycle or concrete persistence/runtime"
	}
	if under("domain") && (strings.HasPrefix(imported, api) || imported == "net/http" || strings.HasPrefix(imported, "github.com/go-chi/")) {
		return "domain must not depend on transport, services or infrastructure"
	}
	if strings.HasPrefix(imported, api+"provider/") && imported != api+"provider/runtimecatalog" &&
		!under("provider") && !under("app") && file != "apps/api/cmd/region-scheduler/main.go" {
		return "concrete game providers belong in provider implementations or the composition root"
	}
	if under("runtime") && (strings.HasPrefix(imported, api+"provider") || strings.HasPrefix(imported, api+"http")) {
		return "runtime must not depend on providers or HTTP handlers"
	}
	return ""
}

func TestImportRules(t *testing.T) {
	for _, tc := range []struct {
		file, imported string
		forbidden      bool
	}{
		{"apps/api/internal/instances/models.go", "encoding/json", false},
		{"apps/api/internal/instances/models.go", api + "domain", true},
		{"apps/api/internal/instances/models.go", "gorm.io/gorm", true},
		{"apps/api/internal/scheduling/capacity.go", "math", false},
		{"apps/api/internal/scheduling/capacity.go", api + "store", true},
		{"apps/api/internal/scheduling/capacity.go", "net/http", true},
		{"apps/api/internal/scheduling/capacity.go", "database/sql", true},
		{"apps/api/internal/http/mod_config_handlers.go", "os", true},
		{"apps/api/internal/http/mod_config_handlers.go", api + "modruntime", false},
		{"apps/api/internal/gameconfig/payload.go", api + "store", true},
		{"apps/api/internal/modlibrary/service.go", api + "store", true},
		{"apps/api/internal/modlibrary/service.go", "net/http", true},
		{"apps/api/internal/gameconfig/payload.go", api + "provider", false},
		{"apps/api/internal/modcatalog/metadata.go", api + "store", true},
		{"apps/api/internal/modruntime/dependencies.go", api + "server", true},
		{"apps/api/internal/modruntime/dependencies.go", api + "modcatalog", false},
		{"apps/api/internal/modcatalog/metadata.go", api + "domain", false},
		{"apps/api/internal/http/new.go", api + "provider/terraria", true},
		{"apps/api/cmd/region-scheduler/main.go", api + "provider/terraria", false},
		{"apps/api/cmd/region-scheduler/worker.go", api + "provider/terraria", true},
		{"apps/agent/main.go", "github.com/smartcat999/game-panel-lite/internal/runtime/docker", false},
		{"apps/agent/reconcile.go", "github.com/docker/docker/client", true},
		{"internal/runtime/docker/adapter.go", "github.com/docker/docker/client", false},
		{"internal/runtime/docker/adapter.go", api + "store", true},
		{"internal/worker/reconciler.go", "github.com/smartcat999/game-panel-lite/internal/runtime/docker", true},
		{"internal/workload/protocol.go", "net/http", true},
		{"apps/api/internal/http/new.go", "github.com/smartcat999/game-panel-lite/internal/runtime/docker", true},
		{"apps/api/internal/http/world_handlers.go", api + "provider/minecraft", true},
		{"apps/api/internal/app/app.go", api + "provider/terraria", false},
		{"apps/api/internal/runtime/docker/adapter.go", "github.com/docker/docker/client", false},
		{"apps/api/internal/server/new.go", "github.com/docker/docker/client", true},
		{"apps/api/internal/domain/new.go", "net/http", true},
		{"apps/api/internal/domain/new.go", api + "store", true},
		{"apps/api/internal/server/new.go", "gorm.io/gorm", true},
		{"apps/api/internal/runtime/new.go", api + "provider", true},
	} {
		if got := forbiddenImport(tc.file, tc.imported) != ""; got != tc.forbidden {
			t.Errorf("%s -> %s: forbidden=%v, want %v", tc.file, tc.imported, got, tc.forbidden)
		}
	}
}
