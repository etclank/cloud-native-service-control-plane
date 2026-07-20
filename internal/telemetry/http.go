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

// Package telemetry provides reusable, bounded-cardinality HTTP telemetry.
package telemetry

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	nooptrace "go.opentelemetry.io/otel/trace/noop"
)

const (
	// RequestIDHeader is the HTTP header used to propagate request identifiers.
	RequestIDHeader = "X-Request-ID"
	// MaxRequestIDLength bounds user-supplied request identifiers.
	MaxRequestIDLength = 64

	unknownRoute = "unknown"
	methodLabel  = "method"
	routeLabel   = "route"
)

type requestIDContextKey struct{}

// RouteNormalizer maps a request to a bounded route template. It must never
// return user-controlled path data.
type RouteNormalizer func(*http.Request) string

// HTTPHandlerOptions configures request telemetry middleware.
type HTTPHandlerOptions struct {
	ServiceName     string
	Logger          *slog.Logger
	Metrics         *HTTPMetrics
	RouteNormalizer RouteNormalizer
	TracerProvider  trace.TracerProvider
	Propagators     propagation.TextMapPropagator
}

// NewHTTPHandler wraps next with request IDs, access logs, metrics, and tracing.
func NewHTTPHandler(next http.Handler, options HTTPHandlerOptions) http.Handler {
	logger := options.Logger
	if logger == nil {
		logger = slog.Default()
	}
	normalizeRoute := options.RouteNormalizer
	if normalizeRoute == nil {
		normalizeRoute = func(*http.Request) string { return unknownRoute }
	}
	tracerProvider := options.TracerProvider
	if tracerProvider == nil {
		tracerProvider = nooptrace.NewTracerProvider()
	}
	propagators := options.Propagators
	if propagators == nil {
		propagators = propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		)
	}

	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestID, err := requestID(request.Header.Values(RequestIDHeader))
		if err != nil {
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusInternalServerError)
			_, _ = writer.Write([]byte("{\"error\":\"internal server error\"}\n"))

			return
		}

		writer.Header().Set(RequestIDHeader, requestID)
		ctx := context.WithValue(request.Context(), requestIDContextKey{}, requestID)
		ctx = propagators.Extract(ctx, propagation.HeaderCarrier(request.Header))
		route := safeRoute(normalizeRoute(request))
		method := normalizedMethod(request.Method)
		ctx, span := tracerProvider.Tracer(
			"github.com/etclank/cloud-native-service-control-plane/internal/telemetry",
		).Start(
			ctx,
			method+" "+route,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				attribute.String("http.request.method", method),
				attribute.String("http.route", route),
			),
		)
		request = request.WithContext(ctx)

		recorder := &responseRecorder{ResponseWriter: writer}
		started := time.Now()
		if options.Metrics != nil {
			options.Metrics.requestStarted(method, route)
		}
		defer func() {
			statusCode := recorder.statusCode()
			if options.Metrics != nil {
				options.Metrics.requestFinished(method, route, recorder, started)
			}
			span.SetAttributes(attribute.Int("http.response.status_code", statusCode))
			if statusCode >= http.StatusInternalServerError {
				span.SetStatus(codes.Error, "HTTP 5xx response")
			}

			attributes := []any{
				"service", options.ServiceName,
				"request_id", requestID,
				methodLabel, method,
				routeLabel, route,
				"status_code", statusCode,
				"duration_ms", time.Since(started).Milliseconds(),
				"response_size", recorder.bytesWritten,
			}
			if spanContext := span.SpanContext(); spanContext.IsValid() {
				attributes = append(
					attributes,
					"trace_id", spanContext.TraceID().String(),
				)
			}
			logger.InfoContext(ctx, "Handled HTTP request", attributes...)
			span.End()
		}()

		next.ServeHTTP(recorder, request)
	})
}

// RequestIDFromContext returns the validated or generated request identifier.
func RequestIDFromContext(ctx context.Context) string {
	requestID, _ := ctx.Value(requestIDContextKey{}).(string)

	return requestID
}

func requestID(values []string) (string, error) {
	if len(values) == 1 && validRequestID(values[0]) {
		return values[0], nil
	}

	identifier := make([]byte, 16)
	if _, err := rand.Read(identifier); err != nil {
		return "", fmt.Errorf("generate request ID: %w", err)
	}

	return hex.EncodeToString(identifier), nil
}

func validRequestID(value string) bool {
	if value == "" || len(value) > MaxRequestIDLength {
		return false
	}

	for index := range len(value) {
		character := value[index]
		alphaNumeric := (character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9')
		if !alphaNumeric && !strings.ContainsRune("-_.:", rune(character)) {
			return false
		}
	}

	return true
}

func safeRoute(route string) string {
	if route == "" || len(route) > 128 {
		return unknownRoute
	}
	for index := range len(route) {
		character := route[index]
		if character <= 0x20 || character >= 0x7f {
			return unknownRoute
		}
	}

	return route
}

func normalizedMethod(method string) string {
	switch method {
	case http.MethodConnect,
		http.MethodDelete,
		http.MethodGet,
		http.MethodHead,
		http.MethodOptions,
		http.MethodPatch,
		http.MethodPost,
		http.MethodPut,
		http.MethodTrace:
		return method
	default:
		return "OTHER"
	}
}

type responseRecorder struct {
	http.ResponseWriter
	status       int
	bytesWritten int
}

func (recorder *responseRecorder) WriteHeader(statusCode int) {
	if recorder.status != 0 {
		return
	}
	recorder.status = statusCode
	recorder.ResponseWriter.WriteHeader(statusCode)
}

func (recorder *responseRecorder) Write(body []byte) (int, error) {
	if recorder.status == 0 {
		recorder.WriteHeader(http.StatusOK)
	}

	written, err := recorder.ResponseWriter.Write(body)
	recorder.bytesWritten += written

	return written, err
}

func (recorder *responseRecorder) statusCode() int {
	if recorder.status == 0 {
		return http.StatusOK
	}

	return recorder.status
}

func (recorder *responseRecorder) Unwrap() http.ResponseWriter {
	return recorder.ResponseWriter
}
