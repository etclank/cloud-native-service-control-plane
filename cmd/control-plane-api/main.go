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
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	platformv1alpha1 "github.com/etclank/cloud-native-service-control-plane/api/v1alpha1"
	"github.com/etclank/cloud-native-service-control-plane/internal/controlplaneapi"
	"github.com/etclank/cloud-native-service-control-plane/internal/telemetry"
)

const (
	shutdownTimeout = 10 * time.Second
	maxHeaderBytes  = 16 << 10
	serviceName     = "control-plane-api"
)

type serverSpec struct {
	name   string
	server *http.Server
}

type serverResult struct {
	name string
	err  error
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)
	if err := run(); err != nil {
		slog.Error("Control-plane API server failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	token, err := controlplaneapi.LoadToken(os.Getenv("API_TOKEN_FILE"))
	if err != nil {
		return fmt.Errorf("configure API token: %w", err)
	}

	port, err := controlplaneapi.ParsePort(os.Getenv("PORT"))
	if err != nil {
		return fmt.Errorf("configure server: %w", err)
	}
	metricsPort, err := controlplaneapi.ParseMetricsPort(
		os.Getenv("METRICS_PORT"),
		port,
	)
	if err != nil {
		return fmt.Errorf("configure metrics server: %w", err)
	}

	scheme := runtime.NewScheme()
	if err := platformv1alpha1.AddToScheme(scheme); err != nil {
		return fmt.Errorf("register ManagedService API: %w", err)
	}

	kubernetesConfig, err := ctrl.GetConfig()
	if err != nil {
		return fmt.Errorf("load Kubernetes configuration: %w", err)
	}
	kubernetesClient, err := client.New(
		kubernetesConfig,
		client.Options{Scheme: scheme},
	)
	if err != nil {
		return fmt.Errorf("create Kubernetes client: %w", err)
	}
	store := controlplaneapi.NewControllerRuntimeStore(kubernetesClient)

	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	traceRuntime, err := telemetry.NewTraceRuntime(
		context.Background(),
		telemetry.TraceConfigFromEnvironment(),
	)
	if err != nil {
		return fmt.Errorf("configure tracing: %w", err)
	}

	registry := prometheus.NewRegistry()
	httpMetrics, err := telemetry.NewHTTPMetrics(registry, serviceName)
	if err != nil {
		return fmt.Errorf("configure HTTP metrics: %w", err)
	}
	publicHandler := telemetry.NewHTTPHandler(
		controlplaneapi.NewHandler(token, store, store.Ready),
		telemetry.HTTPHandlerOptions{
			ServiceName:     serviceName,
			Logger:          slog.Default(),
			Metrics:         httpMetrics,
			RouteNormalizer: controlplaneapi.NormalizeRoute,
			TracerProvider:  traceRuntime.Provider,
			Propagators:     traceRuntime.Propagators,
		},
	)

	publicServer := &http.Server{
		Addr:              fmt.Sprintf(":%d", port),
		Handler:           publicHandler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    maxHeaderBytes,
	}
	metricsServer := telemetry.NewMetricsServer(
		fmt.Sprintf(":%d", metricsPort),
		registry,
	)

	serveErr := serveUntilShutdown(ctx, []serverSpec{
		{name: "public API", server: publicServer},
		{name: "internal metrics", server: metricsServer},
	})

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

func serveUntilShutdown(ctx context.Context, servers []serverSpec) error {
	listeners := make([]net.Listener, 0, len(servers))
	for _, spec := range servers {
		listener, err := net.Listen("tcp", spec.server.Addr)
		if err != nil {
			for _, openListener := range listeners {
				_ = openListener.Close()
			}

			return fmt.Errorf("listen for %s HTTP: %w", spec.name, err)
		}
		listeners = append(listeners, listener)
	}

	results := make(chan serverResult, len(servers))
	for index, spec := range servers {
		slog.Info(
			"Starting HTTP server",
			"server", spec.name,
			"address", listeners[index].Addr().String(),
		)
		listener := listeners[index]
		go func() {
			results <- serverResult{name: spec.name, err: spec.server.Serve(listener)}
		}()
	}

	received := 0
	var resultErr error
	select {
	case result := <-results:
		received++
		if result.err != nil && !errors.Is(result.err, http.ErrServerClosed) {
			resultErr = fmt.Errorf("serve %s HTTP: %w", result.name, result.err)
		} else if ctx.Err() == nil {
			resultErr = fmt.Errorf("%s HTTP server stopped unexpectedly", result.name)
		}
	case <-ctx.Done():
		slog.Info("Shutting down HTTP servers")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	for _, spec := range servers {
		if err := spec.server.Shutdown(shutdownCtx); err != nil {
			resultErr = errors.Join(
				resultErr,
				fmt.Errorf("shut down %s HTTP server: %w", spec.name, err),
			)
		}
	}
	for received < len(servers) {
		result := <-results
		received++
		if result.err != nil && !errors.Is(result.err, http.ErrServerClosed) {
			resultErr = errors.Join(
				resultErr,
				fmt.Errorf("serve %s HTTP: %w", result.name, result.err),
			)
		}
	}
	slog.Info("HTTP servers stopped")

	return resultErr
}
