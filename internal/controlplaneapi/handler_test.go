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

package controlplaneapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

const testToken = "0123456789abcdef0123456789abcdef"

func TestPublicHealthEndpoints(t *testing.T) {
	tests := []struct {
		path       string
		wantStatus string
	}{
		{path: "/healthz", wantStatus: "healthy"},
		{path: "/readyz", wantStatus: "ready"},
	}

	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			response := apiRequest(t, NewHandler(testToken, nil), http.MethodGet, test.path, "")
			assertStatusCode(t, response, http.StatusOK)

			var body statusResponse
			decodeJSONResponse(t, response, &body)
			if body.Status != test.wantStatus {
				t.Errorf("status = %q, want %q", body.Status, test.wantStatus)
			}
		})
	}
}

func TestReadinessCheckCanReportUnavailable(t *testing.T) {
	check := func(context.Context) error {
		return errors.New("not connected")
	}
	response := apiRequest(t, NewHandler(testToken, check), http.MethodGet, "/readyz", "")
	assertStatusCode(t, response, http.StatusServiceUnavailable)

	var body statusResponse
	decodeJSONResponse(t, response, &body)
	if body.Status != "not ready" {
		t.Errorf("status = %q, want %q", body.Status, "not ready")
	}
}

func TestProtectedRouteAcceptsValidCredentials(t *testing.T) {
	response := apiRequest(
		t,
		NewHandler(testToken, nil),
		http.MethodGet,
		"/api/v1",
		"Bearer "+testToken,
	)
	assertStatusCode(t, response, http.StatusOK)

	var body identityResponse
	decodeJSONResponse(t, response, &body)
	if body.Service != "control-plane-api" || body.Version != "v1" {
		t.Errorf("identity response = %#v", body)
	}
}

func TestProtectedRouteRejectsInvalidCredentials(t *testing.T) {
	tests := []struct {
		name          string
		authorization string
	}{
		{name: "missing"},
		{name: "malformed scheme", authorization: "Basic " + testToken},
		{name: "empty bearer", authorization: "Bearer "},
		{name: "wrong token", authorization: "Bearer 9876543210abcdef9876543210abcdef"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := apiRequest(
				t,
				NewHandler(testToken, nil),
				http.MethodGet,
				"/api/v1",
				test.authorization,
			)
			assertStatusCode(t, response, http.StatusUnauthorized)

			var body errorResponse
			decodeJSONResponse(t, response, &body)
			if body.Error != "unauthorized" {
				t.Errorf("error = %q, want generic unauthorized", body.Error)
			}
			if response.Header().Get("WWW-Authenticate") != "Bearer" {
				t.Error("401 response does not advertise Bearer authentication")
			}
		})
	}
}

func TestQueryParameterDoesNotAuthenticate(t *testing.T) {
	response := apiRequest(
		t,
		NewHandler(testToken, nil),
		http.MethodGet,
		"/api/v1?token="+testToken,
		"",
	)
	assertStatusCode(t, response, http.StatusUnauthorized)
}

func TestUnsupportedMethodsReturnJSON(t *testing.T) {
	tests := []struct {
		name          string
		path          string
		authorization string
	}{
		{name: "public", path: "/healthz"},
		{name: "protected", path: "/api/v1", authorization: "Bearer " + testToken},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := apiRequest(
				t,
				NewHandler(testToken, nil),
				http.MethodPost,
				test.path,
				test.authorization,
			)
			assertStatusCode(t, response, http.StatusMethodNotAllowed)
			if response.Header().Get("Allow") != http.MethodGet {
				t.Errorf("Allow = %q, want GET", response.Header().Get("Allow"))
			}

			var body errorResponse
			decodeJSONResponse(t, response, &body)
			if body.Error != "method not allowed" {
				t.Errorf("error = %q", body.Error)
			}
		})
	}
}

func TestUnknownPathsReturnJSON(t *testing.T) {
	tests := []struct {
		name          string
		path          string
		authorization string
	}{
		{name: "public", path: "/unknown"},
		{name: "protected", path: "/api/v1/unknown", authorization: "Bearer " + testToken},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := apiRequest(
				t,
				NewHandler(testToken, nil),
				http.MethodGet,
				test.path,
				test.authorization,
			)
			assertStatusCode(t, response, http.StatusNotFound)

			var body errorResponse
			decodeJSONResponse(t, response, &body)
			if body.Error != "not found" {
				t.Errorf("error = %q", body.Error)
			}
		})
	}
}

func apiRequest(
	t *testing.T,
	handler http.Handler,
	method string,
	path string,
	authorization string,
) *httptest.ResponseRecorder {
	t.Helper()

	request := httptest.NewRequest(method, path, nil)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	return response
}

func assertStatusCode(
	t *testing.T,
	response *httptest.ResponseRecorder,
	want int,
) {
	t.Helper()

	if response.Code != want {
		t.Errorf("status code = %d, want %d", response.Code, want)
	}
}

func decodeJSONResponse(
	t *testing.T,
	response *httptest.ResponseRecorder,
	target any,
) {
	t.Helper()

	if contentType := response.Header().Get("Content-Type"); contentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", contentType)
	}
	if err := json.NewDecoder(response.Body).Decode(target); err != nil {
		t.Fatalf("decode JSON response: %v", err)
	}
}
