package bootstrap

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"time"
)

type HealthServer struct {
	Name string
	Addr string
}

func (s HealthServer) Run() error {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.health)
	mux.HandleFunc("GET /health/live", s.health)
	mux.HandleFunc("GET /health/ready", s.health)

	server := &http.Server{
		Addr:              s.Addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	slog.Info("health server listening", "process", s.Name, "addr", s.Addr)
	return server.ListenAndServe()
}

func (s HealthServer) health(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"process": s.Name,
		"status":  "ok",
	})
}

func Address(defaultAddress string) string {
	if value := os.Getenv("GAMEPANEL_HTTP_ADDR"); value != "" {
		return value
	}
	return defaultAddress
}
