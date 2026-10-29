package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"foundationdb-dashboard/internal/auth"
	"foundationdb-dashboard/internal/clusters"
	"foundationdb-dashboard/internal/config"
	"foundationdb-dashboard/internal/fdbstore"
	"foundationdb-dashboard/internal/httpapi"
	"foundationdb-dashboard/internal/trend"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, nil)))

	cfg, err := config.FromEnv()
	if err != nil {
		slog.Error("config", "err", err)
		os.Exit(1)
	}
	client, err := fdbstore.Open(cfg)
	if err != nil {
		slog.Error("open cluster", "err", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	samples := trend.New(cfg.TrendFile)
	go trend.Run(ctx, client, samples, 5*time.Second)

	book, err := clusters.Open(cfg.ClustersFile, clusterName(cfg.ConnectionDisplay), cfg.ConnectionDisplay)
	if err != nil {
		slog.Error("clusters", "err", err)
		os.Exit(1)
	}
	if active, ok := book.Active(); ok && active.ID != clusters.BuiltinID {
		if err := client.Use(active.Connection); err != nil {
			slog.Error("restore cluster", "err", err)
			_ = book.Activate(clusters.BuiltinID)
		}
	}

	var gate *auth.Gate
	if cfg.AuthEnabled {
		gate = auth.New(cfg.AuthUser, cfg.AuthPassword)
	}
	server := &http.Server{
		Addr: cfg.Addr,
		Handler: (&httpapi.Server{
			Store:      client,
			Connection: cfg.ConnectionDisplay,
			Trend:      samples,
			Clusters:   book,
			Switch:     client.Use,
			Auth:       gate,
		}).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	slog.Info("listening", "addr", cfg.Addr, "cluster", cfg.ConnectionDisplay, "auth", cfg.AuthEnabled)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("server", "err", err)
		os.Exit(1)
	}
}

func clusterName(connection string) string {
	if i := strings.Index(connection, ":"); i > 0 {
		return connection[:i]
	}
	if connection == "" {
		return "cluster"
	}
	return connection
}
