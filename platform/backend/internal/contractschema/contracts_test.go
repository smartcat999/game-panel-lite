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
	if parsed != 7 {
		t.Fatalf("expected 7 versioned contract documents, parsed %d", parsed)
	}
}

func TestOpenAPIV1CompatibilitySurface(t *testing.T) {
	expected := map[string]map[string]string{
		"control-plane.openapi.json": {
			"GET /v1/session":                                         "getSession",
			"GET /v1/user-preferences":                                "getUserPreferences",
			"PATCH /v1/user-preferences":                              "updateUserPreferences",
			"POST /v1/workspace-selection":                            "selectWorkspace",
			"GET /v1/workspaces":                                      "listWorkspaces",
			"GET /v1/workspaces/{workspaceId}/members":                "listWorkspaceMembers",
			"POST /v1/instances":                                      "createLogicalInstance",
			"GET /v1/instances/{logicalInstanceId}":                   "getLogicalInstance",
			"GET /v1/regions":                                         "listRegions",
			"GET /v1/plans":                                           "listPlanVersions",
			"GET /v1/workspaces/{workspaceId}/instances":              "listWorkspaceInstances",
			"GET /v1/workspaces/{workspaceId}/instances/{instanceId}": "getWorkspaceInstance",
			"GET /v1/workspaces/{workspaceId}/orders":                 "listWorkspaceOrders",
			"GET /v1/workspaces/{workspaceId}/backups":                "listWorkspaceBackups",
			"POST /v1/workspaces/{workspaceId}/backups":               "createBackupRequest",
		},
		"platform-operations.openapi.json": {
			"GET /v1/platform/regions":                                                  "listPlatformRegions",
			"GET /v1/platform/regions/{regionId}":                                       "getPlatformRegion",
			"PATCH /v1/platform/regions/{regionId}":                                     "updatePlatformRegionOperations",
			"GET /v1/platform/workspaces":                                               "listPlatformWorkspaces",
			"GET /v1/platform/plans":                                                    "listPlatformPlans",
			"GET /v1/platform/orders":                                                   "listPlatformOrders",
			"GET /v1/platform/instances":                                                "listPlatformInstances",
			"POST /v1/platform/payments/verified":                                       "activateVerifiedPayment",
			"GET /v1/regions/{regionId}/overview":                                       "getRegionExecutionOverview",
			"GET /v1/regions/{regionId}/nodes":                                          "listRegionNodes",
			"GET /v1/regions/{regionId}/deployments":                                    "listRegionalDeployments",
			"GET /v1/regions/{regionId}/tasks":                                          "listRegionalTasks",
			"GET /v1/regions/{regionId}/capacity":                                       "getRegionCapacity",
			"GET /v1/regions/{regionId}/storage":                                        "getRegionStorage",
			"GET /v1/regions/{regionId}/monitoring":                                     "getRegionMonitoring",
			"POST /v1/regions/{regionId}/deployments/{deploymentId}/placement-override": "overrideRegionalPlacement",
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
		"deployment-desired.schema.json":  {"workspaceId", "logicalInstanceId", "regionId", "placementVersion", "instanceRevisionId", "desiredState", "gameKey", "cpuUnits", "memoryMegabytes"},
		"entitlement-changed.schema.json": {"workspaceId", "logicalInstanceId", "entitlementId", "status", "effectiveAt", "expiresAt"},
		"deployment-observed.schema.json": {"logicalInstanceId", "regionalDeploymentId", "regionId", "sequence", "observedState", "observedAt"},
		"backup-observed.schema.json":     {"backupRequestId", "logicalInstanceId", "regionId", "sequence", "status", "observedAt"},
		"backup-requested.schema.json":    {"backupRequestId", "logicalInstanceId", "regionId", "kind", "objectKey", "transferUrl", "relativePath"},
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
