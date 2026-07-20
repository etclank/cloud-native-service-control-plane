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
	"strings"
	"testing"
)

func TestTraceRuntimeDisabledWithoutEndpoint(t *testing.T) {
	runtime, err := NewTraceRuntime(context.Background(), TraceConfig{
		ServiceName: "ignored while tracing is disabled",
	})
	if err != nil {
		t.Fatalf("NewTraceRuntime() error = %v", err)
	}
	if runtime.Enabled {
		t.Error("tracing enabled without an endpoint")
	}
	if runtime.Provider == nil || runtime.Propagators == nil || runtime.Shutdown == nil {
		t.Error("disabled tracing runtime is incomplete")
	}
	if err := runtime.Shutdown(context.Background()); err != nil {
		t.Errorf("disabled tracing shutdown error = %v", err)
	}
}

func TestTraceRuntimeRejectsUnsafeConfiguration(t *testing.T) {
	tests := []struct {
		name   string
		config TraceConfig
	}{
		{name: "malformed endpoint", config: TraceConfig{Endpoint: "://invalid"}},
		{name: "unsupported scheme", config: TraceConfig{Endpoint: "ftp://collector:4317"}},
		{name: "endpoint credentials", config: TraceConfig{Endpoint: "https://user:pass@collector:4317"}},
		{name: "endpoint query", config: TraceConfig{Endpoint: "https://collector:4317?token=secret"}},
		{
			name: "unsafe service name",
			config: TraceConfig{
				Endpoint:    "http://collector:4317",
				ServiceName: "unsafe service",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewTraceRuntime(context.Background(), test.config)
			if err == nil {
				t.Error("NewTraceRuntime() returned no error")
			}
			for _, secret := range []string{"user", "pass", "token=secret"} {
				if strings.Contains(err.Error(), secret) {
					t.Errorf("configuration error exposes endpoint credential %q", secret)
				}
			}
		})
	}
}

func TestTraceRuntimeDoesNotContactCollectorAtStartup(t *testing.T) {
	runtime, err := NewTraceRuntime(context.Background(), TraceConfig{
		Endpoint:    "http://127.0.0.1:1",
		ServiceName: testServiceName,
	})
	if err != nil {
		t.Fatalf("NewTraceRuntime() error = %v", err)
	}
	if !runtime.Enabled {
		t.Error("configured tracing is disabled")
	}
	if err := runtime.Shutdown(context.Background()); err != nil {
		t.Errorf("shutdown trace runtime: %v", err)
	}
}
