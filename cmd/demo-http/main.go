/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/etclank/cloud-native-service-control-plane/internal/demohttp"
	"github.com/etclank/cloud-native-service-control-plane/internal/telemetry"
)

const (
	shutdownTimeout = 10 * time.Second
	maxHeaderBytes  = 16 << 10
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)
	if err := run(); err != nil {
		slog.Error("Demo HTTP server failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	port, err := demohttp.ParsePort(os.Getenv("PORT"))
	if err != nil {
		return fmt.Errorf("configure server: %w", err)
	}
	metricsPort, err := demohttp.ParseMetricsPort(os.Getenv("METRICS_PORT"), port)
	if err != nil {
		return fmt.Errorf("configure metrics server: %w", err)
	}

	registry := prometheus.NewRegistry()
	httpMetrics, err := telemetry.NewHTTPMetrics(registry, demohttp.ServiceName)
	if err != nil {
		return fmt.Errorf("configure HTTP metrics: %w", err)
	}
	traceConfig := telemetry.TraceConfigFromEnvironment()
	if traceConfig.ServiceName == "" {
		traceConfig.ServiceName = demohttp.ServiceName
	}
	traceRuntime, err := telemetry.NewTraceRuntime(context.Background(), traceConfig)
	if err != nil {
		return fmt.Errorf("configure tracing: %w", err)
	}

	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	publicHandler := telemetry.NewHTTPHandler(
		demohttp.NewHandler(os.Getenv("MESSAGE")),
		telemetry.HTTPHandlerOptions{
			ServiceName:     demohttp.ServiceName,
			Logger:          slog.Default(),
			Metrics:         httpMetrics,
			RouteNormalizer: demohttp.NormalizeRoute,
			TracerProvider:  traceRuntime.Provider,
			Propagators:     traceRuntime.Propagators,
		},
	)
	publicServer := &http.Server{
		Addr:              fmt.Sprintf(":%d", port),
		Handler:           publicHandler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    maxHeaderBytes,
	}
	metricsServer := telemetry.NewMetricsServer(
		fmt.Sprintf(":%d", metricsPort),
		registry,
	)

	serveErr := telemetry.ServeUntilShutdown(
		ctx,
		slog.Default(),
		shutdownTimeout,
		telemetry.NamedServer{Name: "public demo", Server: publicServer},
		telemetry.NamedServer{Name: "internal metrics", Server: metricsServer},
	)

	traceShutdownCtx, cancelTraceShutdown := context.WithTimeout(
		context.Background(),
		shutdownTimeout,
	)
	defer cancelTraceShutdown()
	if err := traceRuntime.Shutdown(traceShutdownCtx); err != nil {
		serveErr = errors.Join(serveErr, fmt.Errorf("shut down tracing: %w", err))
	}

	return serveErr
}
