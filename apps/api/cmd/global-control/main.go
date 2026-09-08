package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/controlapi"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/serviceauth"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
)

func main() {
	address := flag.String("listen", "127.0.0.1:8443", "mutual TLS control listener")
	certificate := flag.String("certificate", "", "server certificate PEM file")
	key := flag.String("key", "", "server private key PEM file")
	clientCA := flag.String("client-ca", "", "trusted regional client CA PEM file")
	identities := flag.String("region-identities", "", "JSON URI SAN to region mapping file")
	timeout := flag.Duration("request-timeout", 10*time.Second, "maximum revision request time")
	maxBytes := flag.Int64("max-request-bytes", 65536, "maximum event request bytes")
	flag.Parse()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := run(ctx, *address, *certificate, *key, *clientCA, *identities, *timeout, *maxBytes); err != nil {
		slog.Error("global control stopped", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, address, certificateFile, keyFile, caFile, identitiesFile string, timeout time.Duration, maxBytes int64) error {
	if os.Getenv("GAMEPANEL_DATABASE_URL") == "" || timeout <= 0 || maxBytes < 1 {
		return fmt.Errorf("global database endpoint and positive request limits are required")
	}
	certificate, err := tls.LoadX509KeyPair(certificateFile, keyFile)
	if err != nil {
		return errors.New("cannot load control server certificate")
	}
	caPEM, err := os.ReadFile(caFile)
	if err != nil {
		return errors.New("cannot load regional client CA")
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return errors.New("regional client CA contains no certificates")
	}
	encoded, err := os.ReadFile(identitiesFile)
	if err != nil {
		return errors.New("cannot load regional identity mapping")
	}
	var mapping map[string]string
	if err := json.Unmarshal(encoded, &mapping); err != nil {
		return errors.New("invalid regional identity mapping JSON")
	}
	identities, err := serviceauth.NewRegions(mapping)
	if err != nil {
		return err
	}
	tlsConfig, err := serviceauth.ServerTLS(certificate, pool)
	if err != nil {
		return err
	}
	db, err := store.OpenConfigured("", os.Getenv("GAMEPANEL_DATABASE_URL"), 4)
	if err != nil {
		return err
	}
	defer db.Close()
	handler, err := controlapi.NewHandler(db, identities, maxBytes)
	if err != nil {
		return err
	}
	backups, err := controlapi.NewBackupHandler(db, identities, maxBytes)
	if err != nil {
		return err
	}
	entitlements, err := controlapi.NewEntitlementHandler(db, identities, maxBytes)
	if err != nil {
		return err
	}
	mux := http.NewServeMux()
	mux.Handle("/internal/region/entitlements/", entitlements)
	mux.Handle("/internal/region/backups/", backups)
	mux.Handle("/", handler)
	server := &http.Server{Addr: address, TLSConfig: tlsConfig, Handler: http.TimeoutHandler(mux, timeout, "request timed out"), ReadHeaderTimeout: timeout, ReadTimeout: timeout, WriteTimeout: timeout, IdleTimeout: time.Minute}
	finished := make(chan error, 1)
	go func() { finished <- server.ListenAndServeTLS("", "") }()
	select {
	case err := <-finished:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			_ = server.Close()
			return err
		}
		<-finished
		return nil
	}
}
