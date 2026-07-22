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
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

const testServiceName = "test-service"

func TestRequestIDPropagation(t *testing.T) {
	var contextRequestID string
	handler := NewHTTPHandler(http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		contextRequestID = RequestIDFromContext(request.Context())
		writer.WriteHeader(http.StatusNoContent)
	}), HTTPHandlerOptions{ServiceName: "test", Logger: discardLogger()})

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set(RequestIDHeader, "safe-request_ID:123")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if got := response.Header().Get(RequestIDHeader); got != "safe-request_ID:123" {
		t.Errorf("response request ID = %q", got)
	}
	if contextRequestID != "safe-request_ID:123" {
		t.Errorf("context request ID = %q", contextRequestID)
	}
}

func TestInvalidRequestIDsAreReplaced(t *testing.T) {
	tests := []struct {
		name   string
		values []string
	}{
		{name: "missing"},
		{name: "empty", values: []string{""}},
		{name: "whitespace", values: []string{"unsafe request"}},
		{name: "control character", values: []string{"unsafe\trequest"}},
		{name: "too long", values: []string{strings.Repeat("a", MaxRequestIDLength+1)}},
		{name: "multiple", values: []string{"first", "second"}},
	}

	seen := make(map[string]struct{}, len(tests))
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler := NewHTTPHandler(
				http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
					writer.WriteHeader(http.StatusNoContent)
				}),
				HTTPHandlerOptions{ServiceName: "test", Logger: discardLogger()},
			)
			request := httptest.NewRequest(http.MethodGet, "/", nil)
			for _, value := range test.values {
				request.Header.Add(RequestIDHeader, value)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)

			generated := response.Header().Get(RequestIDHeader)
			if len(generated) != 32 || !validRequestID(generated) {
				t.Fatalf("generated request ID = %q", generated)
			}
			if _, duplicate := seen[generated]; duplicate {
				t.Errorf("duplicate generated request ID %q", generated)
			}
			seen[generated] = struct{}{}
		})
	}
}

func TestAccessLogUsesOnlySafeFields(t *testing.T) {
	const (
		secretToken = "secret-authentication-value"
		secretBody  = "secret-request-body"
		resourceID  = "customer-controlled-name"
	)
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	handler := NewHTTPHandler(
		http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writer.WriteHeader(http.StatusUnauthorized)
			_, _ = writer.Write([]byte(`{"error":"unauthorized"}`))
		}),
		HTTPHandlerOptions{
			ServiceName:     testServiceName,
			Logger:          logger,
			RouteNormalizer: testRouteNormalizer,
		},
	)
	request := httptest.NewRequest(
		"ATTACKER-METHOD",
		"/things/"+resourceID+"?token=query-secret",
		strings.NewReader(secretBody),
	)
	request.Header.Set("Authorization", "Bearer "+secretToken)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	logOutput := logs.String()
	for _, forbidden := range []string{
		secretToken,
		secretBody,
		resourceID,
		"query-secret",
		"Authorization",
	} {
		if strings.Contains(logOutput, forbidden) {
			t.Errorf("access log contains forbidden value %q: %s", forbidden, logOutput)
		}
	}
	for _, expected := range []string{
		`"service":"test-service"`,
		`"method":"OTHER"`,
		`"route":"/things/{name}"`,
		`"status_code":401`,
		`"response_size":24`,
		`"request_id":`,
	} {
		if !strings.Contains(logOutput, expected) {
			t.Errorf("access log lacks %q: %s", expected, logOutput)
		}
	}
}

func TestHTTPMetricsUseBoundedLabels(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics, err := NewHTTPMetrics(registry, testServiceName)
	if err != nil {
		t.Fatalf("NewHTTPMetrics() error = %v", err)
	}
	handler := NewHTTPHandler(
		http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writer.WriteHeader(http.StatusCreated)
		}),
		HTTPHandlerOptions{
			ServiceName:     testServiceName,
			Logger:          discardLogger(),
			Metrics:         metrics,
			RouteNormalizer: testRouteNormalizer,
		},
	)
	request := httptest.NewRequest(
		http.MethodPost,
		"/things/private-resource?token=private-query",
		nil,
	)
	request.Header.Set(RequestIDHeader, "private-request-id")
	handler.ServeHTTP(httptest.NewRecorder(), request)

	metricsResponse := httptest.NewRecorder()
	NewMetricsServer(":9090", registry).Handler.ServeHTTP(
		metricsResponse,
		httptest.NewRequest(http.MethodGet, "/metrics", nil),
	)
	output := metricsResponse.Body.String()
	for _, expected := range []string{
		"http_server_requests_total",
		"http_server_request_duration_seconds",
		`method="POST"`,
		`route="/things/{name}"`,
		`status_class="2xx"`,
		`service="test-service"`,
		`http_server_requests_total{method="POST",route="/things/{name}",service="test-service",status_class="2xx"} 1`,
		`http_server_request_duration_seconds_count{method="POST",route="/things/{name}",service="test-service",status_class="2xx"} 1`,
	} {
		if !strings.Contains(output, expected) {
			t.Errorf("metrics lack %q", expected)
		}
	}
	for _, forbidden := range []string{
		"private-resource",
		"private-query",
		"private-request-id",
	} {
		if strings.Contains(output, forbidden) {
			t.Errorf("metrics contain high-cardinality value %q", forbidden)
		}
	}
}

func TestTraceContextAndBaggagePropagation(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	t.Cleanup(func() {
		if err := provider.Shutdown(context.Background()); err != nil {
			t.Errorf("shutdown tracer provider: %v", err)
		}
	})
	propagators := propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)
	var logs bytes.Buffer
	var baggageValue string
	handler := NewHTTPHandler(
		http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			baggageValue = baggage.FromContext(request.Context()).Member("tenant").Value()
			writer.WriteHeader(http.StatusOK)
		}),
		HTTPHandlerOptions{
			ServiceName:     testServiceName,
			Logger:          slog.New(slog.NewJSONHandler(&logs, nil)),
			RouteNormalizer: testRouteNormalizer,
			TracerProvider:  provider,
			Propagators:     propagators,
		},
	)
	request := httptest.NewRequest(
		http.MethodGet,
		"/things/private-resource?token=private-query",
		nil,
	)
	request.Header.Set(
		"traceparent",
		"00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
	)
	request.Header.Set("baggage", "tenant=portfolio")
	handler.ServeHTTP(httptest.NewRecorder(), request)

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("exported spans = %d, want 1", len(spans))
	}
	if spans[0].Name != "GET /things/{name}" {
		t.Errorf("span name = %q", spans[0].Name)
	}
	if got := spans[0].SpanContext.TraceID().String(); got != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Errorf("trace ID = %q", got)
	}
	if baggageValue != "portfolio" {
		t.Errorf("propagated baggage = %q", baggageValue)
	}
	if !strings.Contains(logs.String(), `"trace_id":"4bf92f3577b34da6a3ce929d0e0e4736"`) {
		t.Errorf("access log lacks trace ID: %s", logs.String())
	}
	foundRoute := false
	for _, spanAttribute := range spans[0].Attributes {
		if string(spanAttribute.Key) == "http.route" &&
			spanAttribute.Value.AsString() == "/things/{name}" {
			foundRoute = true
		}
	}
	if !foundRoute {
		t.Error("span lacks normalized http.route attribute")
	}
	for _, spanAttribute := range spans[0].Attributes {
		value := spanAttribute.Value.Emit()
		if strings.Contains(value, "private-resource") ||
			strings.Contains(value, "private-query") {
			t.Errorf("span attribute %q contains high-cardinality request data", spanAttribute.Key)
		}
	}
}

func TestMetricsServerEndpoints(t *testing.T) {
	registry := prometheus.NewRegistry()
	server := NewMetricsServer(":1234", registry)
	if server.ReadHeaderTimeout == 0 || server.ReadTimeout == 0 ||
		server.WriteTimeout == 0 || server.IdleTimeout == 0 ||
		server.MaxHeaderBytes == 0 {
		t.Error("metrics server does not configure defensive HTTP limits")
	}

	health := httptest.NewRecorder()
	server.Handler.ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if health.Code != http.StatusOK || health.Header().Get("Content-Type") != "application/json" {
		t.Errorf("metrics health response = %d, %q", health.Code, health.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(health.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode metrics health: %v", err)
	}
	if body["status"] != "healthy" {
		t.Errorf("metrics health status = %q", body["status"])
	}
}

func testRouteNormalizer(request *http.Request) string {
	if strings.HasPrefix(request.URL.Path, "/things/") {
		return "/things/{name}"
	}
	if request.URL.Path == "/healthz" {
		return "/healthz"
	}

	return "unknown"
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))
}
