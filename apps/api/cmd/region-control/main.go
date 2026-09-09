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

	"github.com/smartcat999/game-panel-lite/apps/api/internal/nodeapi"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/serviceauth"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
)

func main() {
	region := flag.String("region", "", "identity bound to this regional database")
	address := flag.String("listen", "127.0.0.1:8443", "mutual TLS control listener")
	certificate := flag.String("certificate", "", "server certificate PEM file")
	key := flag.String("key", "", "server private key PEM file")
	clientCA := flag.String("client-ca", "", "trusted node client CA PEM file")
	identities := flag.String("node-identities", "", "JSON URI SAN to node ID mapping file")
	timeout := flag.Duration("request-timeout", 10*time.Second, "maximum node request time")
	maxBytes := flag.Int64("max-request-bytes", 65536, "maximum event request bytes")
	execution := executionOptions{}
	flag.StringVar(&execution.endpoint, "global-control-endpoint", "", "global control HTTPS origin")
	flag.StringVar(&execution.certificate, "global-client-certificate", "", "regional client certificate PEM file")
	flag.StringVar(&execution.key, "global-client-key", "", "regional client private key PEM file")
	flag.StringVar(&execution.ca, "global-server-ca", "", "trusted global server CA PEM file")
	flag.StringVar(&execution.configurationKeys, "configuration-keys", "", "private JSON keyring for protected global revisions")
	flag.StringVar(&execution.catalog, "provider-catalog", "", "provider catalog shared with global configuration validation")
	flag.IntVar(&execution.maxConfigurationBytes, "max-configuration-bytes", 65536, "maximum plaintext provider configuration bytes")
	flag.Int64Var(&execution.maxResponseBytes, "max-global-response-bytes", 4<<20, "maximum global control response bytes")
	flag.DurationVar(&execution.leaseTTL, "execution-lease", 2*time.Minute, "short-lived Node execution authority")
	flag.DurationVar(&execution.maxHeartbeatAge, "max-heartbeat-age", 30*time.Second, "maximum Node heartbeat age for execution authority")
	flag.Parse()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := run(ctx, *region, *address, *certificate, *key, *clientCA, *identities, *timeout, *maxBytes, &execution); err != nil {
		slog.Error("regional control stopped", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, region, address, certificateFile, keyFile, caFile, identitiesFile string, timeout time.Duration, maxBytes int64, execution *executionOptions) error {
	if os.Getenv("GAMEPANEL_REGIONAL_DATABASE_URL") == "" || timeout <= 0 || maxBytes < 1 {
		return fmt.Errorf("regional database endpoint and positive request limits are required")
	}
	certificate, err := tls.LoadX509KeyPair(certificateFile, keyFile)
	if err != nil {
		return errors.New("cannot load control server certificate")
	}
	caPEM, err := os.ReadFile(caFile)
	if err != nil {
		return errors.New("cannot load node client CA")
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return errors.New("node client CA contains no certificates")
	}
	encoded, err := os.ReadFile(identitiesFile)
	if err != nil {
		return errors.New("cannot load node identity mapping")
	}
	var mapping map[string]string
	if err := json.Unmarshal(encoded, &mapping); err != nil {
		return errors.New("invalid node identity mapping JSON")
	}
	identities, err := serviceauth.NewNodes(region, mapping)
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
	var handler http.Handler
	if execution == nil {
		handler, err = nodeapi.NewHandler(db, identities, maxBytes)
	} else {
		authorizer, closeExecution, buildErr := buildExecutionAuthorizer(region, db, timeout, *execution)
		if buildErr != nil {
			return buildErr
		}
		defer closeExecution()
		handler, err = nodeapi.NewExecutionHandler(db, authorizer, identities, maxBytes)
	}
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
