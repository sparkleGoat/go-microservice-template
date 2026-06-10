// Command server is the entrypoint for the microservice.
//
// It demonstrates the production lifecycle a platform team expects from every
// service: load config, initialize observability, serve app + metrics on
// separate listeners, and shut down gracefully on SIGTERM so in-flight requests
// drain before Kubernetes removes the pod.
package main

import (
"context"
"errors"
"log/slog"
"net/http"
"os"
"os/signal"
"sync"
"syscall"

"github.com/prometheus/client_golang/prometheus"
"github.com/prometheus/client_golang/prometheus/promhttp"

"github.com/sparkleGoat/go-microservice-template/internal/config"
"github.com/sparkleGoat/go-microservice-template/internal/health"
"github.com/sparkleGoat/go-microservice-template/internal/httpserver"
"github.com/sparkleGoat/go-microservice-template/internal/observability"
)

// version is overridden at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
if err := run(); err != nil {
slog.Error("fatal", slog.String("error", err.Error()))
os.Exit(1)
}
}

func run() error {
cfg, err := config.Load()
if err != nil {
return err
}

logger := observability.NewLogger(cfg.ServiceName, cfg.Environment, cfg.LogLevel)
logger.Info("starting", slog.String("version", version), slog.String("http_addr", cfg.HTTPAddr))

// Root context cancelled on SIGINT/SIGTERM triggers coordinated shutdown.
ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
defer stop()

shutdownTracing, err := observability.InitTracing(ctx, cfg.ServiceName, cfg.Environment, cfg.OTLPEndpoint)
if err != nil {
return err
}

checker := health.New()
reg := prometheus.NewRegistry()
reg.MustRegister(prometheus.NewGoCollector(), prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))

app := httpserver.New(cfg, logger, checker, reg, version)

appServer := &http.Server{
Addr:         cfg.HTTPAddr,
Handler:      app.Handler(),
ReadTimeout:  cfg.ReadTimeout,
WriteTimeout: cfg.WriteTimeout,
}
metricsServer := &http.Server{
Addr:    cfg.MetricsAddr,
Handler: promhttp.HandlerFor(reg, promhttp.HandlerOpts{}),
}

var wg sync.WaitGroup
wg.Add(2)
go serve(&wg, logger, "app", appServer)
go serve(&wg, logger, "metrics", metricsServer)

// Startup probes/dependencies would be checked here; mark ready once up.
checker.SetReady(true)
logger.Info("ready")

// Block until a signal cancels ctx.
<-ctx.Done()
logger.Info("shutdown signal received, draining")

// Flip readiness first so the load balancer stops sending new traffic while
// existing requests finish.
checker.SetReady(false)

shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
defer cancel()

_ = appServer.Shutdown(shutdownCtx)
_ = metricsServer.Shutdown(shutdownCtx)
if err := shutdownTracing(shutdownCtx); err != nil {
logger.Warn("tracing shutdown", slog.String("error", err.Error()))
}

wg.Wait()
logger.Info("shutdown complete")
return nil
}

func serve(wg *sync.WaitGroup, logger *slog.Logger, name string, srv *http.Server) {
defer wg.Done()
logger.Info("listening", slog.String("server", name), slog.String("addr", srv.Addr))
if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
logger.Error("server failed", slog.String("server", name), slog.String("error", err.Error()))
}
}
