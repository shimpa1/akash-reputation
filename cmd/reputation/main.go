// Command reputation is the Akash provider/client reputation API server.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/shimpa1/akash-reputation/internal/api"
	"github.com/shimpa1/akash-reputation/internal/leases"
	"github.com/shimpa1/akash-reputation/internal/store"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	dsn := resolveDSN(log)
	listen := envOr("LISTEN_ADDR", ":8080")
	provider := mustEnv(log, "PROVIDER_ADDR")
	chainID := envOr("CHAIN_ID", "akashnet-2")
	node := os.Getenv("AKASH_NODE")
	bin := envOr("PROVIDER_SERVICES_BIN", "provider-services")
	interval := envDuration(log, "LEASE_SYNC_INTERVAL", 10*time.Minute)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	st, err := openStoreWithRetry(ctx, log, dsn)
	if err != nil {
		log.Error("database unavailable", "err", err)
		os.Exit(1)
	}
	defer st.Close()

	poller := leases.New(leases.Config{
		Bin:      bin,
		Node:     node,
		Provider: provider,
		Interval: interval,
	}, st, log)
	go poller.Run(ctx)

	srv := &api.Server{Store: st, Verifier: poller, Log: log, Provider: provider, ChainID: chainID}
	httpSrv := &http.Server{
		Addr:              listen,
		Handler:           srv.Routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Info("listening", "addr", listen, "provider", provider)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("http server failed", "err", err)
			stop()
		}
	}()

	<-ctx.Done()
	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(shutdownCtx)
}

func openStoreWithRetry(ctx context.Context, log *slog.Logger, dsn string) (*store.Store, error) {
	var lastErr error
	for i := 0; i < 30; i++ {
		st, err := store.Open(ctx, dsn)
		if err == nil {
			return st, nil
		}
		lastErr = err
		log.Warn("waiting for database", "attempt", i+1, "err", err)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	return nil, lastErr
}

// resolveDSN returns DATABASE_URL if set, otherwise assembles a libpq DSN from
// the standard Postgres env vars (POSTGRES_USER/PASSWORD/DB + PGHOST/PGPORT) so
// the deployment can reuse the postgres secret without duplicating the password.
func resolveDSN(log *slog.Logger) string {
	if v := os.Getenv("DATABASE_URL"); v != "" {
		return v
	}
	user := os.Getenv("POSTGRES_USER")
	pass := os.Getenv("POSTGRES_PASSWORD")
	db := os.Getenv("POSTGRES_DB")
	host := envOr("PGHOST", "reputation-postgres")
	port := envOr("PGPORT", "5432")
	if user == "" || db == "" {
		log.Error("set DATABASE_URL, or POSTGRES_USER/POSTGRES_PASSWORD/POSTGRES_DB")
		os.Exit(1)
	}
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", user, pass, host, port, db)
}

func mustEnv(log *slog.Logger, key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Error("required environment variable not set", "key", key)
		os.Exit(1)
	}
	return v
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envDuration(log *slog.Logger, key string, def time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	if secs, err := strconv.Atoi(v); err == nil {
		return time.Duration(secs) * time.Second
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		log.Warn("invalid duration, using default", "key", key, "value", v, "default", def)
		return def
	}
	return d
}
