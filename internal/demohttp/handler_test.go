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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRootReturnsConfiguredMessage(t *testing.T) {
	response := serveRequest(t, NewHandler("Hello from a test"), http.MethodGet, "/")

	if response.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", response.Code, http.StatusOK)
	}

	var body rootResponse
	decodeResponse(t, response, &body)
	if body.Service != "demo-http" {
		t.Errorf("service = %q, want %q", body.Service, "demo-http")
	}
	if body.Message != "Hello from a test" {
		t.Errorf("message = %q, want %q", body.Message, "Hello from a test")
	}
}

func TestRootEscapesConfiguredMessage(t *testing.T) {
	message := "Hello \"operator\"\nsecond line"
	response := serveRequest(t, NewHandler(message), http.MethodGet, "/")

	rawBody := response.Body.String()
	if !strings.Contains(rawBody, `\"operator\"`) {
		t.Errorf("response body %q does not contain escaped quotes", rawBody)
	}
	if !strings.Contains(rawBody, `\nsecond line`) {
		t.Errorf("response body %q does not contain an escaped newline", rawBody)
	}

	var body rootResponse
	decodeResponse(t, response, &body)
	if body.Message != message {
		t.Errorf("message = %q, want %q", body.Message, message)
	}
}

func TestRootUsesDefaultMessage(t *testing.T) {
	response := serveRequest(t, NewHandler(""), http.MethodGet, "/")

	var body rootResponse
	decodeResponse(t, response, &body)
	if body.Message != DefaultMessage {
		t.Errorf("message = %q, want %q", body.Message, DefaultMessage)
	}
}

func TestHealthResponse(t *testing.T) {
	response := serveRequest(t, NewHandler("test"), http.MethodGet, "/healthz")

	if response.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", response.Code, http.StatusOK)
	}

	var body statusResponse
	decodeResponse(t, response, &body)
	if body.Status != "ok" {
		t.Errorf("status = %q, want %q", body.Status, "ok")
	}
}

func TestReadinessResponse(t *testing.T) {
	response := serveRequest(t, NewHandler("test"), http.MethodGet, "/readyz")

	if response.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", response.Code, http.StatusOK)
	}

	var body statusResponse
	decodeResponse(t, response, &body)
	if body.Status != "ready" {
		t.Errorf("status = %q, want %q", body.Status, "ready")
	}
}

func TestUnknownRouteReturnsNotFound(t *testing.T) {
	response := serveRequest(t, NewHandler("test"), http.MethodGet, "/unknown")

	if response.Code != http.StatusNotFound {
		t.Errorf("status code = %d, want %d", response.Code, http.StatusNotFound)
	}
}

func TestUnsupportedMethodReturnsMethodNotAllowed(t *testing.T) {
	response := serveRequest(t, NewHandler("test"), http.MethodPost, "/")

	if response.Code != http.StatusMethodNotAllowed {
		t.Errorf(
			"status code = %d, want %d",
			response.Code,
			http.StatusMethodNotAllowed,
		)
	}
}

func TestRootResponseHeaders(t *testing.T) {
	response := serveRequest(t, NewHandler("test"), http.MethodGet, "/")

	if contentType := response.Header().Get("Content-Type"); contentType != "application/json" {
		t.Errorf("Content-Type = %q, want %q", contentType, "application/json")
	}
	if cacheControl := response.Header().Get("Cache-Control"); cacheControl != "no-store" {
		t.Errorf("Cache-Control = %q, want %q", cacheControl, "no-store")
	}
}

func TestParsePort(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		want      int
		wantError bool
	}{
		{name: "default", value: "", want: DefaultPort},
		{name: "custom", value: "9090", want: 9090},
		{name: "zero", value: "0", wantError: true},
		{name: "negative", value: "-1", wantError: true},
		{name: "too large", value: "65536", wantError: true},
		{name: "non-numeric", value: "http", wantError: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			port, err := ParsePort(test.value)
			if test.wantError {
				if err == nil {
					t.Fatalf("ParsePort(%q) returned no error", test.value)
				}

				return
			}

			if err != nil {
				t.Fatalf("ParsePort(%q) error = %v", test.value, err)
			}
			if port != test.want {
				t.Errorf("ParsePort(%q) = %d, want %d", test.value, port, test.want)
			}
		})
	}
}

func serveRequest(
	t *testing.T,
	handler http.Handler,
	method string,
	path string,
) *httptest.ResponseRecorder {
	t.Helper()

	request := httptest.NewRequest(method, path, nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	return response
}

func decodeResponse(t *testing.T, response *httptest.ResponseRecorder, target any) {
	t.Helper()

	if err := json.NewDecoder(response.Body).Decode(target); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}
