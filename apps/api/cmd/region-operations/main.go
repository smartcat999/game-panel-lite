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

	"github.com/smartcat999/game-panel-lite/apps/api/internal/regionopsapi"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/serviceauth"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
)

func main() {
	region := flag.String("region", "", "identity bound to this regional database")
	address := flag.String("listen", "127.0.0.1:9443", "mutual TLS operations listener")
	certificate := flag.String("certificate", "", "server certificate PEM file")
	key := flag.String("key", "", "server private key PEM file")
	clientCA := flag.String("client-ca", "", "trusted global control client CA PEM file")
	identities := flag.String("global-identities", "", "JSON array of allowed global control URI SANs")
	timeout := flag.Duration("request-timeout", 10*time.Second, "maximum operations request time")
	freshness := flag.Duration("heartbeat-freshness", time.Minute, "maximum age considered online")
	flag.Parse()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := run(ctx, *region, *address, *certificate, *key, *clientCA, *identities, *timeout, *freshness); err != nil {
		slog.Error("regional operations stopped", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, region, address, certificateFile, keyFile, caFile, identitiesFile string, timeout, freshness time.Duration) error {
	if os.Getenv("GAMEPANEL_REGIONAL_DATABASE_URL") == "" || timeout <= 0 {
		return fmt.Errorf("regional database endpoint and positive request timeout are required")
	}
	certificate, err := tls.LoadX509KeyPair(certificateFile, keyFile)
	if err != nil {
		return errors.New("cannot load regional operations certificate")
	}
	caPEM, err := os.ReadFile(caFile)
	if err != nil {
		return errors.New("cannot load global control client CA")
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return errors.New("global control client CA contains no certificates")
	}
	encoded, err := os.ReadFile(identitiesFile)
	if err != nil {
		return errors.New("cannot load global control identity list")
	}
	var allowed []string
	if err := json.Unmarshal(encoded, &allowed); err != nil {
		return errors.New("invalid global control identity JSON")
	}
	identities, err := serviceauth.NewGlobalControls(allowed)
	if err != nil {
		return err
	}
	tlsConfig, err := serviceauth.ServerTLS(certificate, pool)
	if err != nil {
		return err
	}
	db, err := store.OpenRegionalPostgres(os.Getenv("GAMEPANEL_REGIONAL_DATABASE_URL"), region, 4)
	if err != nil {
		return err
	}
	defer db.Close()
	handler, err := regionopsapi.NewHandler(db, identities, freshness)
	if err != nil {
		return err
	}
	server := &http.Server{Addr: address, TLSConfig: tlsConfig, Handler: http.TimeoutHandler(handler, timeout, "request timed out"), ReadHeaderTimeout: timeout, ReadTimeout: timeout, WriteTimeout: timeout, IdleTimeout: time.Minute}
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
