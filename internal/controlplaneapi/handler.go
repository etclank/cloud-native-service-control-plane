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
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"
)

// ReadinessCheck reports whether the control-plane API is ready to serve.
type ReadinessCheck func(context.Context) error

type apiHandler struct {
	expectedTokenHash [sha256.Size]byte
	readinessCheck    ReadinessCheck
	router            *http.ServeMux
}

type statusResponse struct {
	Status string `json:"status"`
}

type identityResponse struct {
	Service string `json:"service"`
	Version string `json:"version"`
}

type errorResponse struct {
	Error string `json:"error"`
}

// NewHandler constructs the control-plane API handler. A nil readiness check
// reports ready and can be replaced with a Kubernetes-aware check later.
func NewHandler(token string, readinessCheck ReadinessCheck) http.Handler {
	handler := &apiHandler{
		expectedTokenHash: sha256.Sum256([]byte(token)),
		readinessCheck:    readinessCheck,
		router:            http.NewServeMux(),
	}

	handler.router.HandleFunc("/healthz", getOnly(func(
		writer http.ResponseWriter,
		_ *http.Request,
	) {
		writeJSON(writer, http.StatusOK, statusResponse{Status: "healthy"})
	}))
	handler.router.HandleFunc("/readyz", getOnly(handler.serveReadiness))
	handler.router.HandleFunc("/api/v1", getOnly(func(
		writer http.ResponseWriter,
		_ *http.Request,
	) {
		writeJSON(writer, http.StatusOK, identityResponse{
			Service: "control-plane-api",
			Version: "v1",
		})
	}))
	handler.router.HandleFunc("/", func(writer http.ResponseWriter, _ *http.Request) {
		writeJSON(writer, http.StatusNotFound, errorResponse{Error: "not found"})
	})

	return handler
}

func (handler *apiHandler) ServeHTTP(
	writer http.ResponseWriter,
	request *http.Request,
) {
	if isProtectedPath(request.URL.Path) && !handler.authenticated(request) {
		writer.Header().Set("WWW-Authenticate", "Bearer")
		writeJSON(
			writer,
			http.StatusUnauthorized,
			errorResponse{Error: "unauthorized"},
		)

		return
	}

	handler.router.ServeHTTP(writer, request)
}

func (handler *apiHandler) serveReadiness(
	writer http.ResponseWriter,
	request *http.Request,
) {
	if handler.readinessCheck != nil {
		if err := handler.readinessCheck(request.Context()); err != nil {
			writeJSON(
				writer,
				http.StatusServiceUnavailable,
				statusResponse{Status: "not ready"},
			)

			return
		}
	}

	writeJSON(writer, http.StatusOK, statusResponse{Status: "ready"})
}

func (handler *apiHandler) authenticated(request *http.Request) bool {
	values := request.Header.Values("Authorization")
	if len(values) != 1 {
		return false
	}

	scheme, credential, found := strings.Cut(values[0], " ")
	if !found || !strings.EqualFold(scheme, "Bearer") || credential == "" ||
		strings.ContainsAny(credential, " \t\r\n") {
		return false
	}

	presentedTokenHash := sha256.Sum256([]byte(credential))

	return subtle.ConstantTimeCompare(
		handler.expectedTokenHash[:],
		presentedTokenHash[:],
	) == 1
}

func isProtectedPath(path string) bool {
	return path == "/api/v1" || strings.HasPrefix(path, "/api/v1/")
}

func getOnly(next http.HandlerFunc) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			writer.Header().Set("Allow", http.MethodGet)
			writeJSON(
				writer,
				http.StatusMethodNotAllowed,
				errorResponse{Error: "method not allowed"},
			)

			return
		}

		next(writer, request)
	}
}

func writeJSON(writer http.ResponseWriter, statusCode int, response any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.Header().Set("Cache-Control", "no-store")
	writer.WriteHeader(statusCode)

	_ = json.NewEncoder(writer).Encode(response)
}
