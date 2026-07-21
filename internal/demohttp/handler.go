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

// Package demohttp provides the HTTP handler and configuration parsing for the
// demo-http managed workload.
package demohttp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/etclank/cloud-native-service-control-plane/internal/telemetry"
)

const (
	// ServiceName is the stable identity used in responses and telemetry.
	ServiceName = "demo-http"
	// DefaultMessage is returned when MESSAGE is not configured.
	DefaultMessage = "Hello from a ManagedService"
	// DefaultPort is used when PORT is not configured.
	DefaultPort = 8080
	// DefaultMetricsPort is used when METRICS_PORT is not configured.
	DefaultMetricsPort = telemetry.DefaultMetricsPort

	rootRoute      = "/"
	healthRoute    = "/healthz"
	readinessRoute = "/readyz"
	unknownRoute   = "unknown"
)

type rootResponse struct {
	Service string `json:"service"`
	Message string `json:"message"`
}

type statusResponse struct {
	Status string `json:"status"`
}

// NewHandler constructs the demo-http routing handler.
func NewHandler(message string) http.Handler {
	if message == "" {
		message = DefaultMessage
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(writer http.ResponseWriter, _ *http.Request) {
		writeJSON(writer, http.StatusOK, rootResponse{
			Service: ServiceName,
			Message: message,
		})
	})
	mux.HandleFunc("GET /healthz", func(writer http.ResponseWriter, _ *http.Request) {
		writeJSON(writer, http.StatusOK, statusResponse{Status: "ok"})
	})
	mux.HandleFunc("GET /readyz", func(writer http.ResponseWriter, _ *http.Request) {
		writeJSON(writer, http.StatusOK, statusResponse{Status: "ready"})
	})

	return mux
}

// NormalizeRoute maps demo requests to bounded telemetry route names.
func NormalizeRoute(request *http.Request) string {
	switch request.URL.Path {
	case rootRoute, healthRoute, readinessRoute:
		return request.URL.Path
	default:
		return unknownRoute
	}
}

// ParsePort parses PORT, applying the default when the value is empty.
func ParsePort(value string) (int, error) {
	if value == "" {
		return DefaultPort, nil
	}

	port, err := strconv.Atoi(value)
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf(
			"PORT must be an integer between 1 and 65535: %q",
			value,
		)
	}

	return port, nil
}

// ParseMetricsPort validates the separate internal metrics listener port.
func ParseMetricsPort(value string, publicPort int) (int, error) {
	return telemetry.ParseMetricsPort(value, publicPort)
}

func writeJSON(writer http.ResponseWriter, statusCode int, response any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.Header().Set("Cache-Control", "no-store")
	writer.WriteHeader(statusCode)

	_ = json.NewEncoder(writer).Encode(response)
}
