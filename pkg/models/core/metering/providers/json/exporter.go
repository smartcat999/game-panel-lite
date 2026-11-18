package json

import (
	stdjson "encoding/json"
	"time"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/models/core/metering/driver"
)

func init() {
	driver.RegisterExporter(&Exporter{})
}

// Exporter serializes TenantMetering lists using the default declarative JSON list schema.
type Exporter struct{}

// Formats returns the default "json" format identifier.
func (*Exporter) Formats() []string {
	return []string{corev1.MeteringFormatJSON}
}

// Export marshals list to UTF-8 JSON with a trailing newline matching net/http json.Encoder.
func (*Exporter) Export(list *metav1.List[*corev1.TenantMetering], _ time.Time) (*driver.ExportResult, error) {
	raw, err := stdjson.Marshal(list)
	if err != nil {
		return nil, err
	}
	return &driver.ExportResult{
		ContentType: "application/json; charset=utf-8",
		Body:        append(raw, '\n'),
	}, nil
}
