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
	"errors"
	"io"
	"net/http"
	"slices"
	"strings"
	"unicode/utf8"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"

	platformv1alpha1 "github.com/etclank/cloud-native-service-control-plane/api/v1alpha1"
)

// ReadinessCheck reports whether the control-plane API is ready to serve.
type ReadinessCheck func(context.Context) error

type apiHandler struct {
	expectedTokenHash [sha256.Size]byte
	store             ManagedServiceStore
	readinessCheck    ReadinessCheck
	router            *http.ServeMux
}

const (
	maxRequestBodyBytes     = 4 << 10
	methodNotAllowedMessage = "method not allowed"
	readinessStatusReady    = "ready"
	readinessStatusNotReady = "not ready"
)

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

type createManagedServiceRequest struct {
	Name     string `json:"name"`
	Replicas *int32 `json:"replicas,omitempty"`
	Message  string `json:"message,omitempty"`
}

type managedServiceListResponse struct {
	Items []managedServiceResponse `json:"items"`
}

type managedServiceResponse struct {
	Name               string                    `json:"name"`
	Namespace          string                    `json:"namespace"`
	Template           string                    `json:"template"`
	Replicas           int32                     `json:"replicas"`
	Message            string                    `json:"message,omitempty"`
	ObservedGeneration int64                     `json:"observedGeneration"`
	ReadyReplicas      int32                     `json:"readyReplicas"`
	Endpoint           string                    `json:"endpoint,omitempty"`
	Conditions         []managedServiceCondition `json:"conditions"`
}

type managedServiceCondition struct {
	Type               string                 `json:"type"`
	Status             metav1.ConditionStatus `json:"status"`
	Reason             string                 `json:"reason"`
	Message            string                 `json:"message"`
	ObservedGeneration int64                  `json:"observedGeneration"`
}

// NewHandler constructs the control-plane API handler.
func NewHandler(
	token string,
	store ManagedServiceStore,
	readinessCheck ReadinessCheck,
) http.Handler {
	handler := &apiHandler{
		expectedTokenHash: sha256.Sum256([]byte(token)),
		store:             store,
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
	handler.router.HandleFunc(
		"/api/v1/managed-services",
		handler.serveManagedServiceCollection,
	)
	handler.router.HandleFunc(
		"/api/v1/managed-services/",
		handler.serveManagedService,
	)
	handler.router.HandleFunc("/", func(writer http.ResponseWriter, _ *http.Request) {
		writeJSON(writer, http.StatusNotFound, errorResponse{Error: "not found"})
	})

	return handler
}

func (handler *apiHandler) serveManagedServiceCollection(
	writer http.ResponseWriter,
	request *http.Request,
) {
	if rejectQueryParameters(writer, request) {
		return
	}

	switch request.Method {
	case http.MethodGet:
		handler.listManagedServices(writer, request)
	case http.MethodPost:
		handler.createManagedService(writer, request)
	default:
		writer.Header().Set("Allow", http.MethodGet+", "+http.MethodPost)
		writeJSON(
			writer,
			http.StatusMethodNotAllowed,
			errorResponse{Error: methodNotAllowedMessage},
		)
	}
}

func (handler *apiHandler) createManagedService(
	writer http.ResponseWriter,
	request *http.Request,
) {
	createRequest, statusCode := decodeCreateRequest(writer, request)
	if statusCode != 0 {
		writeJSON(writer, statusCode, requestError(statusCode))

		return
	}

	replicas := defaultReplicas
	if createRequest.Replicas != nil {
		replicas = *createRequest.Replicas
	}
	if !validManagedServiceName(createRequest.Name) || replicas < 1 || replicas > 3 ||
		utf8.RuneCountInString(createRequest.Message) > 120 {
		writeJSON(
			writer,
			http.StatusBadRequest,
			errorResponse{Error: "invalid request"},
		)

		return
	}

	managedService, err := handler.store.Create(
		request.Context(),
		createRequest.Name,
		replicas,
		createRequest.Message,
	)
	if apierrors.IsAlreadyExists(err) {
		writeJSON(
			writer,
			http.StatusConflict,
			errorResponse{Error: "managed service already exists"},
		)

		return
	}
	if err != nil {
		writeInternalError(writer)

		return
	}

	writeJSON(writer, http.StatusCreated, newManagedServiceResponse(managedService))
}

func (handler *apiHandler) listManagedServices(
	writer http.ResponseWriter,
	request *http.Request,
) {
	managedServices, err := handler.store.List(request.Context())
	if err != nil {
		writeInternalError(writer)

		return
	}

	items := make([]managedServiceResponse, 0, len(managedServices))
	for index := range managedServices {
		items = append(items, newManagedServiceResponse(&managedServices[index]))
	}
	slices.SortFunc(items, func(left, right managedServiceResponse) int {
		return strings.Compare(left.Name, right.Name)
	})

	writeJSON(writer, http.StatusOK, managedServiceListResponse{Items: items})
}

func (handler *apiHandler) serveManagedService(
	writer http.ResponseWriter,
	request *http.Request,
) {
	if rejectQueryParameters(writer, request) {
		return
	}

	name := strings.TrimPrefix(request.URL.Path, "/api/v1/managed-services/")
	if !validManagedServiceName(name) {
		writeJSON(
			writer,
			http.StatusBadRequest,
			errorResponse{Error: "invalid managed service name"},
		)

		return
	}

	switch request.Method {
	case http.MethodGet:
		handler.getManagedService(writer, request, name)
	case http.MethodDelete:
		handler.deleteManagedService(writer, request, name)
	default:
		writer.Header().Set("Allow", http.MethodGet+", "+http.MethodDelete)
		writeJSON(
			writer,
			http.StatusMethodNotAllowed,
			errorResponse{Error: methodNotAllowedMessage},
		)
	}
}

func rejectQueryParameters(
	writer http.ResponseWriter,
	request *http.Request,
) bool {
	if request.URL.RawQuery == "" {
		return false
	}

	writeJSON(
		writer,
		http.StatusBadRequest,
		errorResponse{Error: "query parameters are not supported"},
	)

	return true
}

func (handler *apiHandler) getManagedService(
	writer http.ResponseWriter,
	request *http.Request,
	name string,
) {
	managedService, err := handler.store.Get(request.Context(), name)
	if apierrors.IsNotFound(err) {
		writeManagedServiceNotFound(writer)

		return
	}
	if err != nil {
		writeInternalError(writer)

		return
	}

	writeJSON(writer, http.StatusOK, newManagedServiceResponse(managedService))
}

func (handler *apiHandler) deleteManagedService(
	writer http.ResponseWriter,
	request *http.Request,
	name string,
) {
	if err := handler.store.Delete(request.Context(), name); err != nil {
		if apierrors.IsNotFound(err) {
			writeManagedServiceNotFound(writer)

			return
		}

		writeInternalError(writer)

		return
	}

	writer.Header().Set("Content-Type", "application/json")
	writer.Header().Set("Cache-Control", "no-store")
	writer.WriteHeader(http.StatusNoContent)
}

func decodeCreateRequest(
	writer http.ResponseWriter,
	request *http.Request,
) (createManagedServiceRequest, int) {
	request.Body = http.MaxBytesReader(writer, request.Body, maxRequestBodyBytes)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()

	createRequest := createManagedServiceRequest{}
	if err := decoder.Decode(&createRequest); err != nil {
		return createRequest, requestDecodeStatus(err)
	}

	var trailingValue any
	if err := decoder.Decode(&trailingValue); !errors.Is(err, io.EOF) {
		return createRequest, requestDecodeStatus(err)
	}

	return createRequest, 0
}

func requestDecodeStatus(err error) int {
	maxBytesError := &http.MaxBytesError{}
	if errors.As(err, &maxBytesError) {
		return http.StatusRequestEntityTooLarge
	}

	return http.StatusBadRequest
}

func requestError(statusCode int) errorResponse {
	if statusCode == http.StatusRequestEntityTooLarge {
		return errorResponse{Error: "request too large"}
	}

	return errorResponse{Error: "invalid request"}
}

func validManagedServiceName(name string) bool {
	return name != "" && len(validation.IsDNS1123Label(name)) == 0
}

func newManagedServiceResponse(
	managedService *platformv1alpha1.ManagedService,
) managedServiceResponse {
	replicas := defaultReplicas
	if managedService.Spec.Replicas != nil {
		replicas = *managedService.Spec.Replicas
	}

	conditions := make([]managedServiceCondition, 0, len(managedService.Status.Conditions))
	for _, condition := range managedService.Status.Conditions {
		conditions = append(conditions, managedServiceCondition{
			Type:               condition.Type,
			Status:             condition.Status,
			Reason:             condition.Reason,
			Message:            condition.Message,
			ObservedGeneration: condition.ObservedGeneration,
		})
	}
	slices.SortFunc(conditions, func(left, right managedServiceCondition) int {
		return strings.Compare(left.Type, right.Type)
	})

	return managedServiceResponse{
		Name:               managedService.Name,
		Namespace:          ApplicationsNamespace,
		Template:           string(managedService.Spec.Template),
		Replicas:           replicas,
		Message:            managedService.Spec.Message,
		ObservedGeneration: managedService.Status.ObservedGeneration,
		ReadyReplicas:      managedService.Status.ReadyReplicas,
		Endpoint:           managedService.Status.Endpoint,
		Conditions:         conditions,
	}
}

func writeManagedServiceNotFound(writer http.ResponseWriter) {
	writeJSON(
		writer,
		http.StatusNotFound,
		errorResponse{Error: "managed service not found"},
	)
}

func writeInternalError(writer http.ResponseWriter) {
	writeJSON(
		writer,
		http.StatusInternalServerError,
		errorResponse{Error: "internal server error"},
	)
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
				statusResponse{Status: readinessStatusNotReady},
			)

			return
		}
	}

	writeJSON(writer, http.StatusOK, statusResponse{Status: readinessStatusReady})
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
				errorResponse{Error: methodNotAllowedMessage},
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
