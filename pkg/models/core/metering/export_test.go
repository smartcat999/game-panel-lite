package metering

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
)

func sampleMeteringList() *metav1.List[*corev1.TenantMetering] {
	tm := &corev1.TenantMetering{
		Window: Window24h,
		Instances: []corev1.InstanceMeteringItem{{
			Name:             "mc-01",
			RuntimeClassName: "docker-standard",
			Phase:            string(corev1.InstancePhaseRunning),
			CPU:              "2",
			Memory:           "4Gi",
			Storage:          "20Gi",
			RunningHours:     12.5,
			CPUCoreHours:     25.0,
			MemoryGiBHours:   50.0,
			StorageGiBHours:  250.0,
		}},
	}
	tm.Metadata.Name = "admin"
	tm.DefaultMeta()
	return &metav1.List[*corev1.TenantMetering]{
		Items: []*corev1.TenantMetering{tm},
	}
}

func TestMeteringExportersDefaultAndCloudEvents(t *testing.T) {
	list := sampleMeteringList()

	for _, fmtName := range []string{"", corev1.MeteringFormatJSON, "unknown-fallback"} {
		res, err := Export(list, fmtName)
		if err != nil {
			t.Fatalf("Export(%q): %v", fmtName, err)
		}
		if res.ContentType != "application/json; charset=utf-8" {
			t.Fatalf("unexpected default content-type: %q", res.ContentType)
		}
		var decoded metav1.List[*corev1.TenantMetering]
		if err := json.Unmarshal(res.Body, &decoded); err != nil || len(decoded.Items) != 1 {
			t.Fatalf("unexpected default JSON decode err=%v items=%d", err, len(decoded.Items))
		}
	}

	for _, fmtName := range []string{corev1.MeteringFormatCloudEvents, corev1.MeteringFormatOpenMeter} {
		res, err := Export(list, fmtName)
		if err != nil {
			t.Fatalf("Export(%q): %v", fmtName, err)
		}
		if res.ContentType != "application/cloudevents-batch+json; charset=utf-8" {
			t.Fatalf("unexpected cloudevents content-type: %q", res.ContentType)
		}
		var events []cloudevents.Event
		if err := json.Unmarshal(res.Body, &events); err != nil || len(events) != 1 {
			t.Fatalf("unexpected cloudevents decode err=%v len=%d", err, len(events))
		}
		if events[0].Subject() != "admin" || events[0].Type() != "io.gamepanel.metering.instance.usage.v1" {
			t.Fatalf("unexpected event metadata: %+v", events[0])
		}
	}
}

func TestMeteringLagoAndCustomExporter(t *testing.T) {
	list := sampleMeteringList()

	res, err := Export(list, corev1.MeteringFormatLago)
	if err != nil {
		t.Fatalf("Export lago: %v", err)
	}
	if res.ContentType != "application/json; charset=utf-8" {
		t.Fatalf("unexpected lago content-type: %q", res.ContentType)
	}
	var payload struct {
		Events []struct {
			ExternalSubscriptionID string            `json:"external_subscription_id"`
			Code                   string            `json:"code"`
			Properties             map[string]string `json:"properties"`
		} `json:"events"`
	}
	if err := json.Unmarshal(res.Body, &payload); err != nil || len(payload.Events) != 1 {
		t.Fatalf("unexpected lago decode err=%v len=%d", err, len(payload.Events))
	}
	if payload.Events[0].Code != "gamepanel_instance_usage" || payload.Events[0].Properties["cpu_core_hours"] != "25.00" {
		t.Fatalf("unexpected lago event: %+v", payload.Events[0])
	}

	RegisterExporter(&testCustomExporter{})
	customRes, err := Export(list, "csv-test")
	if err != nil || customRes.ContentType != "text/csv; charset=utf-8" || !strings.Contains(string(customRes.Body), "admin,mc-01") {
		t.Fatalf("unexpected custom exporter result: res=%+v err=%v", customRes, err)
	}
}

type testCustomExporter struct{}

func (*testCustomExporter) Formats() []string { return []string{"csv-test"} }
func (*testCustomExporter) Export(list *metav1.List[*corev1.TenantMetering], _ time.Time) (*ExportResult, error) {
	return &ExportResult{
		ContentType: "text/csv; charset=utf-8",
		Body:        []byte(list.Items[0].Metadata.Name + "," + list.Items[0].Instances[0].Name + "\n"),
	}, nil
}
