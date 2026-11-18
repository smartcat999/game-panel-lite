package config

import "testing"

func TestObservabilityConfigPrometheusURLValidate(t *testing.T) {
	cfg := New()
	if err := cfg.Observability.Validate(); err != nil {
		t.Fatalf("expected empty prometheusURL to validate cleanly, got %v", err)
	}
	cfg.Observability.PrometheusURL = "http://prometheus:9090"
	if err := cfg.Observability.Validate(); err != nil {
		t.Fatalf("expected valid prometheusURL to validate cleanly, got %v", err)
	}
	cfg.Observability.PrometheusURL = "not-a-url"
	if err := cfg.Observability.Validate(); err == nil {
		t.Fatalf("expected invalid prometheusURL to return error")
	}
}

func TestAgentConfigReservedCPUAndMemory(t *testing.T) {
	def := New()
	if def.Agent.ReservedResources.CPUMillicores != 500 || def.Agent.ReservedResources.MemoryBytes != 512<<20 {
		t.Fatalf("unexpected default reservation: %+v", def.Agent.ReservedResources)
	}
	cfg := New()
	cfg.Agent.ReservedCPU = "2"
	cfg.Agent.ReservedMemory = "8Gi"
	cfg.Agent.ReservedStorage = "10Gi"
	cfg.Default()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid reserved quantities, got %v", err)
	}
	if cfg.Agent.ReservedResources.CPUMillicores != 2000 || cfg.Agent.ReservedResources.MemoryBytes != 8<<30 || cfg.Agent.ReservedResources.StorageBytes != 10<<30 {
		t.Fatalf("unexpected parsed reservation: %+v", cfg.Agent.ReservedResources)
	}
	zero := New()
	zero.Agent.ReservedCPU = "0"
	zero.Agent.ReservedMemory = "0"
	zero.Agent.ReservedStorage = "0"
	zero.Default()
	if err := zero.Validate(); err != nil {
		t.Fatalf("expected explicit zero to validate, got %v", err)
	}
	if zero.Agent.ReservedResources.CPUMillicores != 0 || zero.Agent.ReservedResources.MemoryBytes != 0 || zero.Agent.ReservedResources.StorageBytes != 0 {
		t.Fatalf("expected explicit zero reservation, got %+v", zero.Agent.ReservedResources)
	}
	bad := New()
	bad.Agent.ReservedMemory = "-1Gi"
	if err := bad.Validate(); err == nil {
		t.Fatalf("expected invalid reservedMemory to fail validation")
	}
}
