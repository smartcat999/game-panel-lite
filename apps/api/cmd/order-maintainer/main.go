package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
)

type options struct {
	batch         int
	timeout, poll time.Duration
	once          bool
}

func (o options) validate() error {
	if o.batch < 1 || o.batch > 200 || o.timeout < time.Millisecond || o.timeout > time.Minute || o.poll < time.Millisecond || o.poll > time.Hour {
		return errors.New("invalid order maintenance options")
	}
	return nil
}
func main() {
	var o options
	flag.IntVar(&o.batch, "batch-size", 100, "maximum orders per transaction (1-200)")
	flag.DurationVar(&o.timeout, "task-timeout", 10*time.Second, "maximum time for one expiry transaction")
	flag.DurationVar(&o.poll, "poll-interval", 5*time.Second, "delay after each batch, including failures")
	flag.BoolVar(&o.once, "once", false, "process one bounded batch and exit")
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, o); err != nil {
		slog.Error("order maintenance failed")
		os.Exit(1)
	}
}
func run(ctx context.Context, o options) error {
	if err := o.validate(); err != nil {
		return err
	}
	dsn := os.Getenv("GAMEPANEL_DATABASE_URL")
	if dsn == "" {
		return errors.New("global PostgreSQL database required")
	}
	db, err := store.OpenConfigured("", dsn, 2)
	if err != nil {
		return errors.New("open global database failed")
	}
	defer db.Close()
	return maintain(ctx, o, db.ExpirePrepaidOrders, slog.Default())
}

// The composition root supplies the global store; each batch owns its transaction.
// Failed/empty/full batches all wait before retrying, bounding database pressure.
func maintain(ctx context.Context, o options, expire func(context.Context, int) (int, error), logger *slog.Logger) error {
	if err := o.validate(); err != nil {
		return err
	}
	if expire == nil || logger == nil {
		return errors.New("order maintenance dependencies required")
	}
	for {
		if ctx.Err() != nil {
			return nil
		}
		task, cancel := context.WithTimeout(ctx, o.timeout)
		count, err := expire(task, o.batch)
		cancel()
		if ctx.Err() != nil {
			return nil
		}
		if err != nil {
			logger.Warn("order expiry batch failed")
			if o.once {
				return errors.New("order expiry batch failed")
			}
		} else if count > 0 {
			logger.Info("pending orders expired", "count", count)
		}
		if o.once {
			return nil
		}
		timer := time.NewTimer(o.poll)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
}
