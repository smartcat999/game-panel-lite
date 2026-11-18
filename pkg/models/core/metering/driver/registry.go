package driver

import (
	"strings"
	"sync"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
)

var (
	exportersMu sync.RWMutex
	exporters   = make(map[string]Exporter)
)

// RegisterExporter registers exp under each format identifier returned by exp.Formats().
func RegisterExporter(exp Exporter) {
	if exp == nil {
		return
	}
	exportersMu.Lock()
	defer exportersMu.Unlock()
	for _, raw := range exp.Formats() {
		if key := normalizeFormat(raw); key != "" {
			exporters[key] = exp
		}
	}
}

// LookupExporter returns the Exporter registered for format (empty string resolves to "json").
func LookupExporter(format string) (Exporter, bool) {
	key := normalizeFormat(format)
	if key == "" {
		key = corev1.MeteringFormatJSON
	}
	exportersMu.RLock()
	defer exportersMu.RUnlock()
	exp, ok := exporters[key]
	return exp, ok
}

// ResolveExporter returns the Exporter matching format, falling back to the default JSON exporter.
func ResolveExporter(format string) (Exporter, bool) {
	if exp, ok := LookupExporter(format); ok {
		return exp, true
	}
	return LookupExporter(corev1.MeteringFormatJSON)
}

func normalizeFormat(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}
