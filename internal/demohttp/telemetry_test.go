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

package demohttp

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
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/etclank/cloud-native-service-control-plane/internal/telemetry"
)

const (
	testConfiguredMessage = "private configured demo message"
	testTraceID           = "4bf92f3577b34da6a3ce929d0e0e4736"
)

func TestTelemetryRequestIDs(t *testing.T) {
	tests := []struct {
		name      string
		requestID string
		preserved bool
	}{
		{name: "generated when missing"},
		{name: "accepted", requestID: "safe-demo-request:123", preserved: true},
		{name: "whitespace replaced", requestID: "unsafe request"},
		{name: "control replaced", requestID: "unsafe\trequest"},
		{
			name:      "oversized replaced",
			requestID: strings.Repeat("a", telemetry.MaxRequestIDLength+1),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler := newTelemetryHandler(t, "test", nil, nil, nil)
			request := httptest.NewRequest(http.MethodGet, rootRoute, nil)
			if test.requestID != "" {
				request.Header.Set(telemetry.RequestIDHeader, test.requestID)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)

			responseID := response.Header().Get(telemetry.RequestIDHeader)
			if test.preserved {
				if responseID != test.requestID {
					t.Errorf("response request ID = %q, want %q", responseID, test.requestID)
				}

				return
			}
			if len(responseID) != 32 || responseID == test.requestID {
				t.Errorf("replacement request ID = %q", responseID)
			}
		})
	}
}

func TestNormalizeRouteUsesFixedValues(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{path: rootRoute, want: rootRoute},
		{path: healthRoute, want: healthRoute},
		{path: readinessRoute, want: readinessRoute},
		{path: "/metrics", want: unknownRoute},
		{path: "/private-resource", want: unknownRoute},
	}

	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, test.path+"?token=private", nil)
			if route := NormalizeRoute(request); route != test.want {
				t.Errorf("NormalizeRoute(%q) = %q, want %q", test.path, route, test.want)
			}
		})
	}
}

func TestTelemetryPreservesResponseAndUsesSafeLogs(t *testing.T) {
	const bearerValue = "private-bearer-value"
	var logs bytes.Buffer
	handler := newTelemetryHandler(
		t,
		testConfiguredMessage,
		slog.New(slog.NewJSONHandler(&logs, nil)),
		nil,
		nil,
	)
	request := httptest.NewRequest(
		http.MethodGet,
		rootRoute+"?token=private-query",
		strings.NewReader("private-request-body"),
	)
	request.Header.Set("Authorization", "Bearer "+bearerValue)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", response.Code, http.StatusOK)
	}
	var body rootResponse
	decodeResponse(t, response, &body)
	if body.Service != ServiceName || body.Message != testConfiguredMessage {
		t.Errorf("instrumented response = %#v", body)
	}

	logOutput := logs.String()
	for _, expected := range []string{
		`"service":"demo-http"`,
		`"method":"GET"`,
		`"route":"/"`,
		`"status_code":200`,
		`"request_id":`,
		`"response_size":`,
	} {
		if !strings.Contains(logOutput, expected) {
			t.Errorf("access log lacks %q: %s", expected, logOutput)
		}
	}
	for _, forbidden := range []string{
		bearerValue,
		testConfiguredMessage,
		"private-query",
		"private-request-body",
		"Authorization",
	} {
		if strings.Contains(logOutput, forbidden) {
			t.Errorf("access log contains forbidden value %q", forbidden)
		}
	}
}

func TestDemoHTTPMetricsAndListenerBoundary(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics, err := telemetry.NewHTTPMetrics(registry, ServiceName)
	if err != nil {
		t.Fatalf("NewHTTPMetrics() error = %v", err)
	}
	handler := newTelemetryHandler(t, testConfiguredMessage, nil, metrics, nil)
	request := httptest.NewRequest(
		http.MethodGet,
		rootRoute+"?token=private-query",
		nil,
	)
	request.Header.Set(telemetry.RequestIDHeader, "private-request-id")
	handler.ServeHTTP(httptest.NewRecorder(), request)

	publicMetrics := httptest.NewRecorder()
	handler.ServeHTTP(
		publicMetrics,
		httptest.NewRequest(http.MethodGet, "/metrics", nil),
	)
	if publicMetrics.Code != http.StatusNotFound {
		t.Errorf("public /metrics status = %d, want 404", publicMetrics.Code)
	}

	metricsServer := telemetry.NewMetricsServer(":9090", registry)
	internalMetrics := httptest.NewRecorder()
	metricsServer.Handler.ServeHTTP(
		internalMetrics,
		httptest.NewRequest(http.MethodGet, "/metrics", nil),
	)
	if internalMetrics.Code != http.StatusOK {
		t.Fatalf("internal /metrics status = %d", internalMetrics.Code)
	}
	metricsOutput := internalMetrics.Body.String()
	for _, expected := range []string{
		`http_server_requests_total{method="GET",route="/",service="demo-http",status_class="2xx"} 1`,
		`http_server_request_duration_seconds_count{method="GET",route="/",service="demo-http",status_class="2xx"} 1`,
		"http_server_requests_in_flight",
	} {
		if !strings.Contains(metricsOutput, expected) {
			t.Errorf("internal metrics lack %q", expected)
		}
	}
	for _, forbidden := range []string{
		testConfiguredMessage,
		"private-query",
		"private-request-id",
	} {
		if strings.Contains(metricsOutput, forbidden) {
			t.Errorf("metrics contain high-cardinality value %q", forbidden)
		}
	}

	internalHealth := httptest.NewRecorder()
	metricsServer.Handler.ServeHTTP(
		internalHealth,
		httptest.NewRequest(http.MethodGet, healthRoute, nil),
	)
	if internalHealth.Code != http.StatusOK ||
		internalHealth.Header().Get("Content-Type") != "application/json" {
		t.Errorf("internal health response = %d, %q", internalHealth.Code, internalHealth.Body.String())
	}
	var healthBody map[string]string
	if err := json.Unmarshal(internalHealth.Body.Bytes(), &healthBody); err != nil {
		t.Fatalf("decode internal health response: %v", err)
	}
	if healthBody["status"] != "healthy" {
		t.Errorf("internal health status = %q", healthBody["status"])
	}
}

func TestDemoTracePropagationAndSafeAttributes(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	t.Cleanup(func() {
		if err := provider.Shutdown(context.Background()); err != nil {
			t.Errorf("shutdown tracer provider: %v", err)
		}
	})
	var logs bytes.Buffer
	handler := newTelemetryHandler(
		t,
		testConfiguredMessage,
		slog.New(slog.NewJSONHandler(&logs, nil)),
		nil,
		provider,
	)
	request := httptest.NewRequest(
		http.MethodGet,
		rootRoute+"?token=private-query",
		nil,
	)
	request.Header.Set(
		"traceparent",
		"00-"+testTraceID+"-00f067aa0ba902b7-01",
	)
	request.Header.Set("baggage", "portfolio=demo")
	request.Header.Set("Authorization", "Bearer trace-private-bearer")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	var body rootResponse
	decodeResponse(t, response, &body)
	if body.Message != testConfiguredMessage {
		t.Errorf("instrumented response message = %q", body.Message)
	}
	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("exported spans = %d, want 1", len(spans))
	}
	if spans[0].Name != "GET /" || spans[0].SpanContext.TraceID().String() != testTraceID {
		t.Errorf("span identity = %q, %s", spans[0].Name, spans[0].SpanContext.TraceID())
	}
	if !strings.Contains(logs.String(), `"trace_id":"`+testTraceID+`"`) {
		t.Errorf("access log lacks trace ID: %s", logs.String())
	}
	for _, spanAttribute := range spans[0].Attributes {
		value := spanAttribute.Value.Emit()
		if strings.Contains(value, testConfiguredMessage) ||
			strings.Contains(value, "private-query") ||
			strings.Contains(value, "trace-private-bearer") {
			t.Errorf("span attribute %q contains sensitive request data", spanAttribute.Key)
		}
	}
	if strings.Contains(logs.String(), "trace-private-bearer") ||
		strings.Contains(logs.String(), testConfiguredMessage) ||
		strings.Contains(logs.String(), "private-query") {
		t.Errorf("trace-correlated access log contains sensitive data: %s", logs.String())
	}
}

func TestDemoTracingIsDisabledWithoutEndpoint(t *testing.T) {
	runtime, err := telemetry.NewTraceRuntime(context.Background(), telemetry.TraceConfig{
		ServiceName: ServiceName,
	})
	if err != nil {
		t.Fatalf("NewTraceRuntime() error = %v", err)
	}
	if runtime.Enabled {
		t.Error("demo tracing enabled without an OTLP endpoint")
	}
}

func newTelemetryHandler(
	t *testing.T,
	message string,
	logger *slog.Logger,
	metrics *telemetry.HTTPMetrics,
	provider trace.TracerProvider,
) http.Handler {
	t.Helper()

	if logger == nil {
		logger = slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))
	}

	return telemetry.NewHTTPHandler(NewHandler(message), telemetry.HTTPHandlerOptions{
		ServiceName:     ServiceName,
		Logger:          logger,
		Metrics:         metrics,
		RouteNormalizer: NormalizeRoute,
		TracerProvider:  provider,
		Propagators: propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		),
	})
}
