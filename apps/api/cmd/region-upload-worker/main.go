package main

import (
	"context"
	"crypto/x509"
	"errors"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/assetfiles"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/backup"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/s3archive"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
)

type options struct {
	region, dsn, directory, storageID, endpoint, signingRegion, bucket, ca string
	accessKey, secretKey, sessionToken                                     string
	lease, transferTimeout, taskTimeout, retry, poll                       time.Duration
	maxBytes                                                               int64
}

func main() {
	var o options
	flag.StringVar(&o.region, "region", "", "Region owning this worker")
	flag.StringVar(&o.directory, "archive-directory", "", "private directory containing verified assetfiles archives")
	flag.StringVar(&o.storageID, "storage-id", "", "configured regional destination storage identity")
	flag.StringVar(&o.endpoint, "s3-endpoint", "", "S3 HTTPS origin")
	flag.StringVar(&o.signingRegion, "s3-signing-region", "", "S3 signing region")
	flag.StringVar(&o.bucket, "s3-bucket", "", "archive bucket")
	flag.StringVar(&o.ca, "s3-ca", "", "trusted S3 CA PEM file")
	flag.DurationVar(&o.lease, "lease", 5*time.Minute, "database upload claim lifetime")
	flag.DurationVar(&o.transferTimeout, "transfer-timeout", 2*time.Minute, "maximum recovery or upload attempt")
	flag.DurationVar(&o.taskTimeout, "task-timeout", 3*time.Minute, "maximum claim, transfer and database completion time")
	flag.DurationVar(&o.retry, "retry-delay", 10*time.Second, "persistent retry delay")
	flag.DurationVar(&o.poll, "poll-interval", time.Second, "delay when idle or unsuccessful")
	flag.Int64Var(&o.maxBytes, "max-archive-bytes", 512<<20, "maximum individual archive size")
	flag.Parse()
	o.dsn = os.Getenv("GAMEPANEL_REGIONAL_DATABASE_URL")
	o.accessKey = os.Getenv("GAMEPANEL_S3_ACCESS_KEY_ID")
	o.secretKey = os.Getenv("GAMEPANEL_S3_SECRET_ACCESS_KEY")
	o.sessionToken = os.Getenv("GAMEPANEL_S3_SESSION_TOKEN")
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := run(ctx, o); err != nil {
		slog.Error("regional upload worker stopped", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, o options) error {
	if strings.TrimSpace(o.region) == "" || o.region != strings.TrimSpace(o.region) || o.dsn == "" || o.directory == "" || o.accessKey == "" || o.secretKey == "" || o.transferTimeout <= 0 || o.taskTimeout <= o.transferTimeout || o.lease <= o.taskTimeout || o.lease > time.Hour || o.retry < time.Millisecond || o.retry > 24*time.Hour || o.poll < time.Millisecond || o.poll > time.Hour {
		return errors.New("invalid regional upload settings")
	}
	pem, err := os.ReadFile(o.ca)
	if err != nil {
		return errors.New("cannot load archive storage CA")
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return errors.New("archive storage CA contains no certificates")
	}
	archive, err := s3archive.New(s3archive.Options{StorageID: o.storageID, Endpoint: o.endpoint, SigningRegion: o.signingRegion, Bucket: o.bucket, RootCAs: pool, Timeout: o.transferTimeout, MaxBytes: o.maxBytes, Credentials: aws.CredentialsProviderFunc(func(context.Context) (aws.Credentials, error) {
		return aws.Credentials{AccessKeyID: o.accessKey, SecretAccessKey: o.secretKey, SessionToken: o.sessionToken}, nil
	})})
	if err != nil {
		return errors.New("invalid archive storage settings")
	}
	defer archive.Close()
	files, err := assetfiles.New(o.directory, o.maxBytes)
	if err != nil {
		return errors.New("cannot open prepared archive directory")
	}
	defer files.Close()
	db, err := store.OpenRegionalPostgres(o.dsn, o.region, 2)
	if err != nil {
		return errors.New("regional database unavailable")
	}
	defer db.Close()
	worker := backup.UploadWorker{Tasks: db, Files: files, Archives: archive, StorageID: o.storageID, Lease: o.lease, Timeout: o.transferTimeout, RetryDelay: o.retry}
	for ctx.Err() == nil {
		taskCtx, cancel := context.WithTimeout(ctx, o.taskTimeout)
		done, err := worker.RunOnce(taskCtx)
		cancel()
		if ctx.Err() != nil {
			return nil
		}
		if err != nil {
			slog.Warn("regional archive upload unsuccessful", "region", o.region, "storage_id", o.storageID)
		}
		if done {
			continue
		}
		timer := time.NewTimer(o.poll)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
	return nil
}
