package contractschema

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

type document map[string]any

func TestContractDocumentsParse(t *testing.T) {
	root := contractRoot(t)
	var parsed int
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".json") {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var value document
		if err := json.Unmarshal(content, &value); err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		parsed++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if parsed != 11 {
		t.Fatalf("expected 11 versioned contract documents, parsed %d", parsed)
	}
}

func TestOpenAPIV1CompatibilitySurface(t *testing.T) {
	expected := map[string]map[string]string{
		"control-plane.openapi.json": {
			"POST /v1/auth/github/start":                                                                           "startGitHubSignIn",
			"POST /v1/auth/password/sign-in":                                                                       "signInWithPassword",
			"GET /v1/session":                                                                                      "getSession",
			"POST /v1/invitations/{invitationToken}:redeem":                                                        "redeemInvitation",
			"GET /v1/workspaces":                                                                                   "listWorkspaces",
			"GET /v1/regions/{regionId}/catalog":                                                                   "getRegionCatalog",
			"GET /v1/providers/releases/{providerReleaseId}/manifest":                                              "getProviderManifest",
			"GET /v1/providers/releases/{providerReleaseId}/mods":                                                  "getProviderModCatalog",
			"GET /v1/workspaces/{workspaceId}/wallet":                                                              "getWorkspaceWallet",
			"POST /v1/workspaces/{workspaceId}/quotes":                                                             "createResourceQuote",
			"POST /v1/workspaces/{workspaceId}/instances":                                                          "createLogicalInstance",
			"GET /v1/workspaces/{workspaceId}/instances/{logicalInstanceId}":                                       "getWorkspaceInstance",
			"POST /v1/workspaces/{workspaceId}/instances/{logicalInstanceId}:start":                                "startLogicalInstance",
			"POST /v1/workspaces/{workspaceId}/instances/{logicalInstanceId}:stop":                                 "stopLogicalInstance",
			"POST /v1/workspaces/{workspaceId}/instances/{logicalInstanceId}:restart":                              "restartLogicalInstance",
			"POST /v1/workspaces/{workspaceId}/instances/{logicalInstanceId}/configuration-drafts":                 "createConfigurationDraft",
			"PUT /v1/workspaces/{workspaceId}/instances/{logicalInstanceId}/configuration-drafts/{draftId}":        "saveConfigurationDraft",
			"POST /v1/workspaces/{workspaceId}/instances/{logicalInstanceId}/configuration-drafts/{draftId}:apply": "applyConfigurationDraft",
			"GET /v1/workspaces/{workspaceId}/instances/{logicalInstanceId}/revisions/{revisionId}":                "getInstanceRevision",
			"GET /v1/workspaces/{workspaceId}/instances/{logicalInstanceId}/logs":                                  "getInstanceLogs",
			"GET /v1/workspaces/{workspaceId}/instances/{logicalInstanceId}/metrics":                               "getInstanceMetrics",
			"POST /v1/workspaces/{workspaceId}/instances/{logicalInstanceId}/console-commands":                     "sendGameConsoleCommand",
			"GET /v1/workspaces/{workspaceId}/backups":                                                             "listWorkspaceBackups",
			"POST /v1/workspaces/{workspaceId}/backups/{backupId}:restore":                                         "restoreBackup",
			"GET /v1/operations/{operationId}":                                                                     "getOperation",
		},
		"platform-operations.openapi.json": {
			"GET /v1/platform/users":                                                    "listPlatformUsers",
			"POST /v1/platform/credit-grants":                                           "grantPromotionalCredit",
			"GET /v1/platform/price-books":                                              "listPriceBooks",
			"GET /v1/platform/instances":                                                "listPlatformInstances",
			"GET /v1/platform/regions":                                                  "listPlatformRegions",
			"GET /v1/regions/{regionId}/health":                                         "getRegionHealth",
			"GET /v1/regions/{regionId}/nodes":                                          "listRegionNodes",
			"GET /v1/regions/{regionId}/deployments":                                    "listRegionalDeployments",
			"GET /v1/regions/{regionId}/tasks":                                          "listRegionalTasks",
			"POST /v1/regions/{regionId}/tasks/{taskId}:retry":                          "retryRegionalTask",
			"POST /v1/regions/{regionId}/deployments/{deploymentId}:override-placement": "overrideRegionalPlacement",
		},
	}
	for filename, operations := range expected {
		doc := readDocument(t, filepath.Join(contractRoot(t), "openapi", "v1", filename))
		if doc["openapi"] != "3.1.0" {
			t.Fatalf("%s must remain OpenAPI 3.1.0", filename)
		}
		paths := object(t, doc["paths"], filename+" paths")
		for signature, operationID := range operations {
			method, path, _ := strings.Cut(signature, " ")
			pathItem := object(t, paths[path], signature)
			operation := object(t, pathItem[strings.ToLower(method)], signature)
			if operation["operationId"] != operationID {
				t.Errorf("%s: expected operationId %q, got %v", signature, operationID, operation["operationId"])
			}
		}
	}
}

func TestEventV1CompatibilitySurface(t *testing.T) {
	expectedPayloadFields := map[string][]string{
		"deployment-desired.schema.json":        {"workspaceId", "logicalInstanceId", "regionId", "placementVersion", "instanceRevisionId", "operationId", "desiredState", "providerReleaseId", "gameVersion", "applyBehavior", "resourceSpec", "configuration", "modLock", "listenerRequirements", "authorityGrant"},
		"deployment-observed.schema.json":       {"workspaceId", "logicalInstanceId", "regionalDeploymentId", "runtimeAttemptId", "regionId", "placementVersion", "sequence", "observedState", "endpointBindings", "observedAt"},
		"usage-observed.schema.json":            {"workspaceId", "logicalInstanceId", "runtimeAttemptId", "regionId", "resourceKind", "quantity", "intervalStart", "intervalEnd"},
		"wallet-exhausted.schema.json":          {"workspaceId", "ledgerSequence", "exhaustedAt"},
		"backup-observed.schema.json":           {"workspaceId", "backupRequestId", "operationId", "logicalInstanceId", "regionId", "sequence", "status", "providerReleaseId", "gameVersion", "configurationRevisionId", "modLock", "checksums", "observedAt"},
		"backup-requested.schema.json":          {"workspaceId", "backupRequestId", "operationId", "logicalInstanceId", "regionId", "kind", "objectKey", "transferUrl", "dataScope", "authorityGrant"},
		"instance-observed.schema.json":         {"workspaceId", "logicalInstanceId", "regionId", "regionalDeploymentId", "runtimeAttemptId", "sequence", "logs", "metrics", "observedAt"},
		"instance-action-observed.schema.json":  {"workspaceId", "operationId", "logicalInstanceId", "regionId", "kind", "status", "observedAt"},
		"console-command-requested.schema.json": {"workspaceId", "logicalInstanceId", "regionId", "operationId", "command", "authorityGrant"},
	}
	envelopeFields := []string{"schemaVersion", "messageId", "messageType", "occurredAt", "idempotencyKey", "payload"}
	for filename, payloadFields := range expectedPayloadFields {
		doc := readDocument(t, filepath.Join(contractRoot(t), "events", "v1", filename))
		assertContainsAll(t, stringSlice(t, doc["required"], filename+" required"), envelopeFields, filename+" envelope")
		properties := object(t, doc["properties"], filename+" properties")
		payload := object(t, properties["payload"], filename+" payload")
		assertContainsAll(t, stringSlice(t, payload["required"], filename+" payload required"), payloadFields, filename+" payload")
	}
}

func TestProviderConfigurationTypesFailClosed(t *testing.T) {
	doc := readDocument(t, filepath.Join(contractRoot(t), "openapi", "v1", "control-plane.openapi.json"))
	components := object(t, doc["components"], "components")
	schemas := object(t, components["schemas"], "schemas")
	field := object(t, schemas["ConfigurationFieldSchema"], "ConfigurationFieldSchema")
	properties := object(t, field["properties"], "ConfigurationFieldSchema properties")
	fieldType := object(t, properties["type"], "ConfigurationFieldSchema type")
	got := stringSlice(t, fieldType["enum"], "ConfigurationFieldSchema type enum")
	want := []string{"string", "integer", "number", "boolean", "enum", "secret", "string-list"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("configuration field types must be an explicit fail-closed allowlist: got %v", got)
	}
	required := stringSlice(t, field["required"], "ConfigurationFieldSchema required")
	assertContainsAll(t, required, []string{"applyBehavior"}, "ConfigurationFieldSchema")
	applyBehavior := object(t, properties["applyBehavior"], "ConfigurationFieldSchema applyBehavior")
	wantApply := []string{"hot-reload", "restart-required", "recreate-required", "create-only"}
	if got := stringSlice(t, applyBehavior["enum"], "apply behavior enum"); strings.Join(got, ",") != strings.Join(wantApply, ",") {
		t.Fatalf("apply behavior must fail closed: got %v", got)
	}
	localizations := object(t, properties["localizations"], "ConfigurationFieldSchema localizations")
	localizedValues := object(t, localizations["additionalProperties"], "ConfigurationFieldSchema localized values")
	if localizedValues["$ref"] != "#/components/schemas/ConfigurationFieldLocalization" {
		t.Fatalf("configuration localizations must use the versioned localization schema: %v", localizedValues)
	}
}

func TestAuthorizationContractSupportsScopedHundredResourceBatch(t *testing.T) {
	doc := readDocument(t, filepath.Join(contractRoot(t), "openapi", "v1", "control-plane.openapi.json"))
	components := object(t, doc["components"], "components")
	schemas := object(t, components["schemas"], "schemas")

	role := object(t, schemas["BuiltInRole"], "BuiltInRole")
	wantRoles := []string{"platform.admin", "region.operator", "workspace.owner", "workspace.operator", "workspace.viewer"}
	if got := stringSlice(t, role["enum"], "BuiltInRole enum"); strings.Join(got, ",") != strings.Join(wantRoles, ",") {
		t.Fatalf("unexpected built-in roles: %v", got)
	}

	scope := object(t, schemas["AuthorizationScope"], "AuthorizationScope")
	scopeProperties := object(t, scope["properties"], "AuthorizationScope properties")
	scopeType := object(t, scopeProperties["type"], "AuthorizationScope type")
	wantScopes := []string{"platform", "region", "workspace"}
	if got := stringSlice(t, scopeType["enum"], "AuthorizationScope type enum"); strings.Join(got, ",") != strings.Join(wantScopes, ",") {
		t.Fatalf("unexpected authorization scopes: %v", got)
	}

	batch := object(t, schemas["AuthorizationBatchRequest"], "AuthorizationBatchRequest")
	batchProperties := object(t, batch["properties"], "AuthorizationBatchRequest properties")
	checks := object(t, batchProperties["checks"], "AuthorizationBatchRequest checks")
	if checks["maxItems"] != float64(100) {
		t.Fatalf("authorization batch must support exactly 100 resources, got %v", checks["maxItems"])
	}

	principalID := "usr_example"
	bindings := []struct {
		PrincipalID string
		Role        string
		Scope       string
	}{
		{PrincipalID: principalID, Role: "platform.admin", Scope: "platform"},
		{PrincipalID: principalID, Role: "region.operator", Scope: "region"},
		{PrincipalID: principalID, Role: "workspace.viewer", Scope: "workspace"},
	}
	if len(bindings) != 3 {
		t.Fatal("authorization fixture must cover Platform, Region, and Workspace bindings for one Principal")
	}
	resourceIDs := make([]string, 100)
	for index := range resourceIDs {
		resourceIDs[index] = fmt.Sprintf("lin_%03d", index)
	}
	if len(resourceIDs) != int(checks["maxItems"].(float64)) {
		t.Fatalf("authorization fixture must exercise the full batch, got %d resources", len(resourceIDs))
	}
}

func TestCommerceRebaselineSchemasArePresent(t *testing.T) {
	doc := readDocument(t, filepath.Join(contractRoot(t), "openapi", "v1", "control-plane.openapi.json"))
	components := object(t, doc["components"], "components")
	schemas := object(t, components["schemas"], "schemas")
	for _, name := range []string{"RegionCatalog", "PriceBook", "Quote", "CapacityHold", "Wallet", "LedgerEntry", "UsageRecord", "FundingAuthorization"} {
		if _, ok := schemas[name]; !ok {
			t.Errorf("missing rebaseline commerce schema %s", name)
		}
	}
	for _, legacy := range []string{"Plan", "PlanVersion", "Order", "Payment", "Entitlement"} {
		if _, ok := schemas[legacy]; ok {
			t.Errorf("legacy commerce schema %s must not remain in v1", legacy)
		}
	}
}

func contractRoot(t *testing.T) string {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate contract test")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "..", "..", "contracts"))
}

func readDocument(t *testing.T, path string) document {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var value document
	if err := json.Unmarshal(content, &value); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return value
}

func object(t *testing.T, value any, label string) document {
	t.Helper()
	result, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("%s must be an object, got %T", label, value)
	}
	return result
}

func stringSlice(t *testing.T, value any, label string) []string {
	t.Helper()
	items, ok := value.([]any)
	if !ok {
		t.Fatalf("%s must be an array, got %T", label, value)
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		text, ok := item.(string)
		if !ok {
			t.Fatalf("%s must contain strings, got %T", label, item)
		}
		result = append(result, text)
	}
	return result
}

func assertContainsAll(t *testing.T, got, expected []string, label string) {
	t.Helper()
	available := make(map[string]bool, len(got))
	for _, item := range got {
		available[item] = true
	}
	var missing []string
	for _, item := range expected {
		if !available[item] {
			missing = append(missing, item)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		t.Errorf("%s removed required v1 fields: %v", label, missing)
	}
}
