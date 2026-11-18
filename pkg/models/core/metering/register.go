package metering

import (
	"fmt"
	"time"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/models/core/metering/driver"

	// Self-register built-in TenantMetering export format drivers.
	_ "github.com/smartcat999/game-panel-cloud/pkg/models/core/metering/providers/cloudevents"
	_ "github.com/smartcat999/game-panel-cloud/pkg/models/core/metering/providers/json"
	_ "github.com/smartcat999/game-panel-cloud/pkg/models/core/metering/providers/lago"
)

type (
	// Exporter serializes a TenantMetering list into a billing or observability wire format.
	Exporter = driver.Exporter
	// ExportResult holds the serialized response body and Content-Type header produced by an Exporter.
	ExportResult = driver.ExportResult
)

// RegisterExporter registers exp under each format identifier returned by exp.Formats().
func RegisterExporter(exp Exporter) {
	driver.RegisterExporter(exp)
}

// LookupExporter returns the Exporter registered for format (empty string resolves to "json").
func LookupExporter(format string) (Exporter, bool) {
	return driver.LookupExporter(format)
}

// Export renders list using the Exporter registered for format (defaulting to JSON for unknown/empty formats).
func Export(list *metav1.List[*corev1.TenantMetering], format string) (*ExportResult, error) {
	exp, ok := driver.ResolveExporter(format)
	if !ok {
		return nil, fmt.Errorf("no metering exporter registered for format %q", format)
	}
	return exp.Export(list, time.Now().UTC())
}
