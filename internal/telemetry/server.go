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

package telemetry

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	maxMetricsHeaderBytes = 16 << 10
	// DefaultMetricsPort is used when METRICS_PORT is not configured.
	DefaultMetricsPort = 9090
)

// NamedServer identifies an HTTP server in lifecycle logs and errors.
type NamedServer struct {
	Name   string
	Server *http.Server
}

type serverResult struct {
	name string
	err  error
}

// ParseMetricsPort validates the separate internal metrics listener port.
func ParseMetricsPort(value string, publicPort int) (int, error) {
	metricsPort := DefaultMetricsPort
	if value != "" {
		parsedPort, err := strconv.Atoi(value)
		if err != nil || parsedPort < 1 || parsedPort > 65535 {
			return 0, fmt.Errorf(
				"METRICS_PORT must be an integer between 1 and 65535: %q",
				value,
			)
		}
		metricsPort = parsedPort
	}
	if metricsPort == publicPort {
		return 0, fmt.Errorf("METRICS_PORT must differ from PORT")
	}

	return metricsPort, nil
}

// NewMetricsServer constructs the separate internal metrics HTTP server.
func NewMetricsServer(address string, gatherer prometheus.Gatherer) *http.Server {
	router := http.NewServeMux()
	router.Handle("/metrics", promhttp.HandlerFor(gatherer, promhttp.HandlerOpts{}))
	router.HandleFunc("/healthz", func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			writer.Header().Set("Allow", http.MethodGet)
			writer.WriteHeader(http.StatusMethodNotAllowed)

			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintln(writer, `{"status":"healthy"}`)
	})

	return &http.Server{
		Addr:              address,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    maxMetricsHeaderBytes,
	}
}

// ServeUntilShutdown runs all servers and coordinates their bounded shutdown.
func ServeUntilShutdown(
	ctx context.Context,
	logger *slog.Logger,
	shutdownTimeout time.Duration,
	servers ...NamedServer,
) error {
	if logger == nil {
		logger = slog.Default()
	}
	if len(servers) == 0 {
		return fmt.Errorf("at least one HTTP server is required")
	}

	listeners := make([]net.Listener, 0, len(servers))
	for _, spec := range servers {
		if spec.Name == "" || spec.Server == nil {
			for _, openListener := range listeners {
				_ = openListener.Close()
			}

			return fmt.Errorf("HTTP server name and configuration are required")
		}
		listener, err := net.Listen("tcp", spec.Server.Addr)
		if err != nil {
			for _, openListener := range listeners {
				_ = openListener.Close()
			}

			return fmt.Errorf("listen for %s HTTP: %w", spec.Name, err)
		}
		listeners = append(listeners, listener)
	}

	results := make(chan serverResult, len(servers))
	for index, spec := range servers {
		logger.Info(
			"Starting HTTP server",
			"server", spec.Name,
			"address", listeners[index].Addr().String(),
		)
		listener := listeners[index]
		go func() {
			results <- serverResult{name: spec.Name, err: spec.Server.Serve(listener)}
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
		logger.Info("Shutting down HTTP servers")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	for _, spec := range servers {
		if err := spec.Server.Shutdown(shutdownCtx); err != nil {
			resultErr = errors.Join(
				resultErr,
				fmt.Errorf("shut down %s HTTP server: %w", spec.Name, err),
			)
			if closeErr := spec.Server.Close(); closeErr != nil {
				resultErr = errors.Join(
					resultErr,
					fmt.Errorf("close %s HTTP server: %w", spec.Name, closeErr),
				)
			}
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
	logger.Info("HTTP servers stopped")

	return resultErr
}
