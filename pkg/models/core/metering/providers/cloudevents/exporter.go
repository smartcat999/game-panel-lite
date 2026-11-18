package cloudevents

import (
	"encoding/json"
	"fmt"
	"time"

	cesdk "github.com/cloudevents/sdk-go/v2"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/models/core/metering/driver"
)

func init() {
	driver.RegisterExporter(&Exporter{})
}

// Exporter serializes TenantMetering usage records as a CNCF CloudEvents 1.0 batch (OpenMeter compatible).
type Exporter struct{}

// Formats returns both "cloudevents" and "openmeter" format aliases handled by this exporter.
func (*Exporter) Formats() []string {
	return []string{corev1.MeteringFormatCloudEvents, corev1.MeteringFormatOpenMeter}
}

// Export renders all instance usage records in list into a CloudEvents batch JSON response.
func (*Exporter) Export(list *metav1.List[*corev1.TenantMetering], now time.Time) (*driver.ExportResult, error) {
	events := make([]cesdk.Event, 0)
	if list != nil {
		ts := now.UTC()
		for _, tm := range list.Items {
			if tm == nil {
				continue
			}
			for _, rec := range tm.Instances {
				events = append(events, buildEvent(tm, rec, ts))
			}
		}
	}
	raw, err := json.Marshal(events)
	if err != nil {
		return nil, err
	}
	return &driver.ExportResult{
		ContentType: "application/cloudevents-batch+json; charset=utf-8",
		Body:        append(raw, '\n'),
	}, nil
}

func buildEvent(tm *corev1.TenantMetering, rec corev1.InstanceMeteringItem, now time.Time) cesdk.Event {
	ce := cesdk.NewEvent(cesdk.VersionV1)
	ce.SetID(fmt.Sprintf("om-%s-%s-%s-%d", tm.Metadata.Name, rec.Name, tm.Window, now.Unix()))
	ce.SetSource("urn:gamepanel:apiserver:metering")
	ce.SetType("io.gamepanel.metering.instance.usage.v1")
	ce.SetSubject(tm.Metadata.Name)
	ce.SetTime(now)
	_ = ce.SetData(cesdk.ApplicationJSON, map[string]any{
		"tenant":           tm.Metadata.Name,
		"instance":         rec.Name,
		"runtimeClassName": rec.RuntimeClassName,
		"phase":            rec.Phase,
		"window":           tm.Window,
		"cpu":              rec.CPU,
		"memory":           rec.Memory,
		"storage":          rec.Storage,
		"runningHours":     rec.RunningHours,
		"cpuCoreHours":     rec.CPUCoreHours,
		"memoryGiBHours":   rec.MemoryGiBHours,
		"storageGiBHours":  rec.StorageGiBHours,
	})
	return ce
}
