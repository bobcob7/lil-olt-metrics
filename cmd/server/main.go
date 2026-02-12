package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/bobcob7/lil-olt-metrics/internal/config"
	"github.com/bobcob7/lil-olt-metrics/internal/ingest"
	"github.com/bobcob7/lil-olt-metrics/internal/query"
	"github.com/bobcob7/lil-olt-metrics/internal/store"
	"github.com/prometheus/prometheus/promql"
	colmetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/encoding/gzip"
)

var (
	version = "dev"
	commit  = "unknown"
	branch  = "unknown"
)

func main() {
	os.Exit(run())
}

func run() int {
	var (
		configPath  string
		showVersion bool
	)
	flag.StringVar(&configPath, "config", "", "path to config file")
	flag.BoolVar(&showVersion, "version", false, "print version and exit")
	flag.Parse()
	if showVersion {
		fmt.Printf("lil-olt-metrics %s (commit=%s, branch=%s)\n", version, commit, branch)
		return 0
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: loading config: %v\n", err)
		return 1
	}
	logger := buildLogger(cfg.Server)
	logger.Info("starting lil-olt-metrics", "version", version, "commit", commit)
	s, err := createStore(logger, cfg)
	if err != nil {
		logger.Error("failed to create store", "error", err)
		return 1
	}
	defer func() { _ = s.Close() }()
	translator := ingest.NewTranslator(logger.With("component", "translator"), cfg.Translation)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	errCh := make(chan error, 3)
	var grpcSrv *grpc.Server
	if cfg.OTLP.GRPC.Enabled {
		grpcSrv = startGRPC(logger, cfg.OTLP.GRPC, translator, s, errCh)
	}
	var otlpHTTPSrv *http.Server
	if cfg.OTLP.HTTP.Enabled {
		otlpHTTPSrv = startOTLPHTTP(logger, cfg.OTLP.HTTP, translator, s, errCh)
	}
	querySrv := startQueryAPI(logger, cfg, s, errCh)
	select {
	case <-ctx.Done():
		logger.Info("received shutdown signal")
	case err := <-errCh:
		logger.Error("server error", "error", err)
		return 1
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if grpcSrv != nil {
		grpcSrv.GracefulStop()
		logger.Info("gRPC server stopped")
	}
	if otlpHTTPSrv != nil {
		if err := otlpHTTPSrv.Shutdown(shutdownCtx); err != nil {
			logger.Error("OTLP HTTP shutdown error", "error", err)
		} else {
			logger.Info("OTLP HTTP server stopped")
		}
	}
	if err := querySrv.Shutdown(shutdownCtx); err != nil {
		logger.Error("query API shutdown error", "error", err)
	} else {
		logger.Info("query API server stopped")
	}
	logger.Info("shutdown complete")
	return 0
}

func createStore(logger *slog.Logger, cfg *config.Config) (store.Store, error) {
	storeLogger := logger.With("component", "store")
	switch cfg.Storage.Engine {
	case "fs":
		return store.NewFSStore(storeLogger, store.FSStoreConfig{
			Path:             cfg.Storage.FS.Path,
			WALSegmentSize:   int64(cfg.Storage.FS.WAL.SegmentSize),
			FlushAge:         cfg.Storage.FS.Compaction.MinBlockDuration.AsDuration(),
			CompactionPeriod: cfg.Storage.FS.Compaction.MinBlockDuration.AsDuration(),
			RetentionAge:     cfg.Retention.Duration.AsDuration(),
			RetentionMaxSize: int64(cfg.Retention.MaxSize),
		})
	case "prometheus":
		return store.NewPrometheusRemote(storeLogger, store.PrometheusRemoteConfig{
			WriteURL: cfg.Storage.Prometheus.WriteURL,
			ReadURL:  cfg.Storage.Prometheus.ReadURL,
			Timeout:  cfg.Storage.Prometheus.Timeout.AsDuration(),
			Username: cfg.Storage.Prometheus.BasicAuth.Username,
			Password: cfg.Storage.Prometheus.BasicAuth.Password,
		}), nil
	case "victoriametrics":
		return store.NewVMRemote(storeLogger, store.VMRemoteConfig{
			WriteURL: cfg.Storage.VictoriaMetrics.WriteURL,
			ReadURL:  cfg.Storage.VictoriaMetrics.ReadURL,
			Timeout:  cfg.Storage.VictoriaMetrics.Timeout.AsDuration(),
			Username: cfg.Storage.VictoriaMetrics.BasicAuth.Username,
			Password: cfg.Storage.VictoriaMetrics.BasicAuth.Password,
		}), nil
	default:
		return store.NewMemStore(storeLogger, cfg.Retention.Duration.AsDuration()), nil
	}
}

func buildLogger(cfg config.ServerConfig) *slog.Logger {
	level := slog.LevelInfo
	switch cfg.LogLevel {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	opts := &slog.HandlerOptions{Level: level}
	var handler slog.Handler
	if cfg.LogFormat == "text" {
		handler = slog.NewTextHandler(os.Stderr, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stderr, opts)
	}
	return slog.New(handler)
}

func startGRPC(logger *slog.Logger, cfg config.OTLPGRPCConfig, translator *ingest.Translator, s store.Store, errCh chan<- error) *grpc.Server {
	grpcLogger := logger.With("component", "otlp-grpc")
	var opts []grpc.ServerOption
	opts = append(opts, grpc.MaxRecvMsgSize(cfg.MaxRecvMsgSize))
	if cfg.Gzip {
		_ = gzip.Name
	}
	srv := grpc.NewServer(opts...)
	handler := ingest.NewGRPCHandler(grpcLogger, translator, s)
	colmetricspb.RegisterMetricsServiceServer(srv, handler)
	lis, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		grpcLogger.Error("failed to listen", "address", cfg.Listen, "error", err)
		go func() { errCh <- fmt.Errorf("gRPC listen: %w", err) }()
		return srv
	}
	grpcLogger.Info("listening", "address", lis.Addr().String())
	go func() {
		if err := srv.Serve(lis); err != nil {
			errCh <- fmt.Errorf("gRPC serve: %w", err)
		}
	}()
	return srv
}

func startOTLPHTTP(logger *slog.Logger, cfg config.OTLPHTTPConfig, translator *ingest.Translator, s store.Store, errCh chan<- error) *http.Server {
	httpLogger := logger.With("component", "otlp-http")
	handler := ingest.NewHTTPHandler(httpLogger, translator, s, cfg.MaxBodySize)
	mux := http.NewServeMux()
	mux.Handle("/v1/metrics", handler)
	srv := &http.Server{
		Addr:    cfg.Listen,
		Handler: mux,
	}
	lis, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		httpLogger.Error("failed to listen", "address", cfg.Listen, "error", err)
		go func() { errCh <- fmt.Errorf("OTLP HTTP listen: %w", err) }()
		return srv
	}
	httpLogger.Info("listening", "address", lis.Addr().String())
	go func() {
		if err := srv.Serve(lis); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("OTLP HTTP serve: %w", err)
		}
	}()
	return srv
}

func startQueryAPI(logger *slog.Logger, cfg *config.Config, s store.Store, errCh chan<- error) *http.Server {
	queryLogger := logger.With("component", "query-api")
	queryable := store.NewQueryable(s)
	engine := promql.NewEngine(promql.EngineOpts{
		Logger:               queryLogger,
		MaxSamples:           cfg.Prometheus.MaxSamples,
		Timeout:              cfg.Prometheus.ReadTimeout.AsDuration(),
		LookbackDelta:        cfg.Prometheus.LookbackDelta.AsDuration(),
		EnableAtModifier:     true,
		EnableNegativeOffset: true,
	})
	api := query.NewAPI(queryLogger, queryable, engine, cfg.Prometheus.LookbackDelta.AsDuration(), query.BuildInfo{
		Version:  version,
		Revision: commit,
		Branch:   branch,
	})
	mux := http.NewServeMux()
	mux.Handle("/", api.Handler())
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	srv := &http.Server{
		Addr:        cfg.Prometheus.Listen,
		Handler:     mux,
		ReadTimeout: cfg.Prometheus.ReadTimeout.AsDuration(),
	}
	lis, err := net.Listen("tcp", cfg.Prometheus.Listen)
	if err != nil {
		queryLogger.Error("failed to listen", "address", cfg.Prometheus.Listen, "error", err)
		go func() { errCh <- fmt.Errorf("query API listen: %w", err) }()
		return srv
	}
	queryLogger.Info("listening", "address", lis.Addr().String())
	go func() {
		if err := srv.Serve(lis); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("query API serve: %w", err)
		}
	}()
	return srv
}
