package driver

import (
	"time"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
)

// ExportResult holds the serialized response payload and MIME content type produced by an Exporter.
type ExportResult struct {
	ContentType string
	Body        []byte
}

// Exporter serializes a TenantMetering list into a billing or observability wire format.
type Exporter interface {
	Formats() []string
	Export(list *metav1.List[*corev1.TenantMetering], now time.Time) (*ExportResult, error)
}
