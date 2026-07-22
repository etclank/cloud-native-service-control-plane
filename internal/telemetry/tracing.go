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
	"fmt"
	"net/url"
	"os"
	"strings"

	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.39.0"
	"go.opentelemetry.io/otel/trace"
	nooptrace "go.opentelemetry.io/otel/trace/noop"
)

const defaultServiceName = "control-plane-api"

// TraceConfig configures optional OTLP trace export.
type TraceConfig struct {
	Endpoint    string
	ServiceName string
}

// TraceRuntime contains the request tracer and its bounded shutdown hook.
type TraceRuntime struct {
	Provider    trace.TracerProvider
	Propagators propagation.TextMapPropagator
	Shutdown    func(context.Context) error
	Enabled     bool
}

// TraceConfigFromEnvironment reads standard OpenTelemetry trace settings.
func TraceConfigFromEnvironment() TraceConfig {
	return TraceConfig{
		Endpoint:    os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT"),
		ServiceName: os.Getenv("OTEL_SERVICE_NAME"),
	}
}

// NewTraceRuntime configures a no-op provider or an OTLP batch exporter.
func NewTraceRuntime(ctx context.Context, config TraceConfig) (*TraceRuntime, error) {
	propagators := propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)
	if config.Endpoint == "" {
		return &TraceRuntime{
			Provider:    nooptrace.NewTracerProvider(),
			Propagators: propagators,
			Shutdown:    func(context.Context) error { return nil },
		}, nil
	}
	serviceName := config.ServiceName
	if serviceName == "" {
		serviceName = defaultServiceName
	}
	if !validServiceName(serviceName) {
		return nil, fmt.Errorf("OTEL_SERVICE_NAME must use 1-63 safe characters")
	}
	if err := validateTraceEndpoint(config.Endpoint); err != nil {
		return nil, err
	}

	telemetryResource, err := resource.New(
		ctx,
		resource.WithFromEnv(),
		resource.WithTelemetrySDK(),
		resource.WithAttributes(semconv.ServiceName(serviceName)),
	)
	if err != nil {
		return nil, fmt.Errorf("configure telemetry resource: %w", err)
	}
	exporter, err := otlptracegrpc.New(
		ctx,
		otlptracegrpc.WithEndpointURL(config.Endpoint),
	)
	if err != nil {
		return nil, fmt.Errorf("configure OTLP trace exporter")
	}

	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(telemetryResource),
	)

	return &TraceRuntime{
		Provider:    provider,
		Propagators: propagators,
		Shutdown:    provider.Shutdown,
		Enabled:     true,
	}, nil
}

func validateTraceEndpoint(endpoint string) error {
	if len(endpoint) > 2048 {
		return fmt.Errorf("OTLP traces endpoint is invalid")
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" ||
		(parsed.Scheme != "http" && parsed.Scheme != "https") ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("OTLP traces endpoint must be an HTTP(S) URL without credentials")
	}

	return nil
}

func validServiceName(serviceName string) bool {
	if serviceName == "" || len(serviceName) > 63 {
		return false
	}
	for index := range len(serviceName) {
		character := serviceName[index]
		alphaNumeric := (character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9')
		if !alphaNumeric && !strings.ContainsRune("-_.", rune(character)) {
			return false
		}
	}

	return true
}
