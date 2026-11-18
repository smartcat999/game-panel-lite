package lago

import (
	"encoding/json"
	"fmt"
	"time"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/models/core/metering/driver"
)

func init() {
	driver.RegisterExporter(&Exporter{})
}

type lagoEvent struct {
	TransactionID          string         `json:"transaction_id"`
	ExternalSubscriptionID string         `json:"external_subscription_id"`
	Code                   string         `json:"code"`
	Timestamp              int64          `json:"timestamp"`
	Properties             map[string]any `json:"properties"`
}

// Exporter serializes TenantMetering usage records as a Lago batch ingestion payload.
type Exporter struct{}

// Formats returns the canonical "lago" format identifier.
func (*Exporter) Formats() []string {
	return []string{corev1.MeteringFormatLago}
}

// Export renders all instance usage records in list into a Lago {"events": [...]} JSON response.
func (*Exporter) Export(list *metav1.List[*corev1.TenantMetering], now time.Time) (*driver.ExportResult, error) {
	events := make([]lagoEvent, 0)
	ts := now.UTC().Unix()
	if list != nil {
		for _, tm := range list.Items {
			if tm == nil {
				continue
			}
			for _, rec := range tm.Instances {
				events = append(events, buildLagoEvent(tm, rec, ts))
			}
		}
	}
	raw, err := json.Marshal(map[string]any{"events": events})
	if err != nil {
		return nil, err
	}
	return &driver.ExportResult{
		ContentType: "application/json; charset=utf-8",
		Body:        append(raw, '\n'),
	}, nil
}

func buildLagoEvent(tm *corev1.TenantMetering, rec corev1.InstanceMeteringItem, unixSec int64) lagoEvent {
	return lagoEvent{
		TransactionID:          fmt.Sprintf("lago-%s-%s-%s-%d", tm.Metadata.Name, rec.Name, tm.Window, unixSec),
		ExternalSubscriptionID: tm.Metadata.Name,
		Code:                   "gamepanel_instance_usage",
		Timestamp:              unixSec,
		Properties: map[string]any{
			"instance":          rec.Name,
			"runtime_class":     rec.RuntimeClassName,
			"window":            tm.Window,
			"running_hours":     fmt.Sprintf("%.2f", rec.RunningHours),
			"cpu_core_hours":    fmt.Sprintf("%.2f", rec.CPUCoreHours),
			"memory_gib_hours":  fmt.Sprintf("%.2f", rec.MemoryGiBHours),
			"storage_gib_hours": fmt.Sprintf("%.2f", rec.StorageGiBHours),
		},
	}
}
