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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	platformv1alpha1 "github.com/etclank/cloud-native-service-control-plane/api/v1alpha1"
)

const (
	managedServicesPath    = "/api/v1/managed-services"
	managedServiceTestName = "example"
)

type fakeManagedServiceStore struct {
	createdName     string
	createdReplicas int32
	createdMessage  string
	createResult    *platformv1alpha1.ManagedService
	listResult      []platformv1alpha1.ManagedService
	getResult       *platformv1alpha1.ManagedService
	createErr       error
	listErr         error
	getErr          error
	deleteErr       error
	readyErr        error
	deletedName     string
	gotName         string
	listCalled      bool
}

func TestCreateManagedService(t *testing.T) {
	store := &fakeManagedServiceStore{}
	response := lifecycleRequest(
		t,
		NewHandler(testToken, store, store.Ready),
		http.MethodPost,
		managedServicesPath,
		`{"name":"example","replicas":2,"message":"Hello from the platform"}`,
		true,
	)
	assertStatusCode(t, response, http.StatusCreated)

	if store.createdName != managedServiceTestName || store.createdReplicas != 2 ||
		store.createdMessage != "Hello from the platform" {
		t.Errorf(
			"create input = %q, %d, %q",
			store.createdName,
			store.createdReplicas,
			store.createdMessage,
		)
	}

	var body managedServiceResponse
	decodeJSONResponse(t, response, &body)
	if body.Name != managedServiceTestName || body.Namespace != ApplicationsNamespace ||
		body.Template != string(platformv1alpha1.ManagedServiceTemplateDemoHTTP) ||
		body.Replicas != 2 || body.Message != "Hello from the platform" {
		t.Errorf("create response = %#v", body)
	}
}

func TestCreateManagedServiceDefaultsReplicas(t *testing.T) {
	store := &fakeManagedServiceStore{}
	response := lifecycleRequest(
		t,
		NewHandler(testToken, store, store.Ready),
		http.MethodPost,
		managedServicesPath,
		`{"name":"defaults"}`,
		true,
	)
	assertStatusCode(t, response, http.StatusCreated)
	if store.createdReplicas != defaultReplicas {
		t.Errorf("created replicas = %d, want %d", store.createdReplicas, defaultReplicas)
	}
}

func TestListManagedServicesIsStableAndIntentional(t *testing.T) {
	oneReplica := int32(1)
	store := &fakeManagedServiceStore{
		listResult: []platformv1alpha1.ManagedService{
			newTestManagedService("zeta", &oneReplica),
			newTestManagedService("alpha", nil),
		},
	}
	store.listResult[1].ResourceVersion = "internal-resource-version"
	store.listResult[1].ManagedFields = []metav1.ManagedFieldsEntry{{Manager: "secret-manager"}}
	store.listResult[1].Status.Conditions = []metav1.Condition{
		{Type: "Progressing", Status: metav1.ConditionFalse, Reason: "Done"},
		{Type: "Available", Status: metav1.ConditionTrue, Reason: "Ready"},
	}

	response := lifecycleRequest(
		t,
		NewHandler(testToken, store, store.Ready),
		http.MethodGet,
		managedServicesPath,
		"",
		true,
	)
	assertStatusCode(t, response, http.StatusOK)
	rawBody := response.Body.String()
	for _, forbidden := range []string{
		"resourceVersion",
		"managedFields",
		"creationTimestamp",
		"uid",
		"secret-manager",
	} {
		if strings.Contains(rawBody, forbidden) {
			t.Errorf("public response contains Kubernetes field %q", forbidden)
		}
	}

	var body managedServiceListResponse
	decodeJSONBytes(t, []byte(rawBody), &body)
	if len(body.Items) != 2 || body.Items[0].Name != "alpha" ||
		body.Items[1].Name != "zeta" {
		t.Errorf("list ordering = %#v", body.Items)
	}
	if len(body.Items[0].Conditions) != 2 ||
		body.Items[0].Conditions[0].Type != "Available" ||
		body.Items[0].Conditions[1].Type != "Progressing" {
		t.Errorf("condition ordering = %#v", body.Items[0].Conditions)
	}
}

func TestGetManagedService(t *testing.T) {
	store := &fakeManagedServiceStore{getResult: managedServicePointer(
		newTestManagedService(managedServiceTestName, nil),
	)}
	response := lifecycleRequest(
		t,
		NewHandler(testToken, store, store.Ready),
		http.MethodGet,
		managedServicesPath+"/example",
		"",
		true,
	)
	assertStatusCode(t, response, http.StatusOK)

	var body managedServiceResponse
	decodeJSONResponse(t, response, &body)
	if body.Name != managedServiceTestName || body.Namespace != ApplicationsNamespace {
		t.Errorf("get response = %#v", body)
	}
}

func TestDeleteManagedService(t *testing.T) {
	store := &fakeManagedServiceStore{}
	response := lifecycleRequest(
		t,
		NewHandler(testToken, store, store.Ready),
		http.MethodDelete,
		managedServicesPath+"/example",
		"",
		true,
	)
	assertStatusCode(t, response, http.StatusNoContent)
	if store.deletedName != managedServiceTestName {
		t.Errorf("deleted name = %q, want example", store.deletedName)
	}
	if response.Body.Len() != 0 {
		t.Errorf("delete response body = %q, want empty", response.Body.String())
	}
}

func TestManagedServiceItemRejectsInvalidNameBeforeStoreAccess(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			store := &fakeManagedServiceStore{}
			response := lifecycleRequest(
				t,
				NewHandler(testToken, store, store.Ready),
				method,
				managedServicesPath+"/Invalid_Name",
				"",
				true,
			)
			assertStatusCode(t, response, http.StatusBadRequest)
			if store.gotName != "" || store.deletedName != "" {
				t.Errorf(
					"invalid name reached store: get %q, delete %q",
					store.gotName,
					store.deletedName,
				)
			}
		})
	}
}

func TestLifecycleEndpointsRejectUnsupportedMethods(t *testing.T) {
	tests := []struct {
		name      string
		method    string
		path      string
		wantAllow string
	}{
		{
			name:      "collection",
			method:    http.MethodPut,
			path:      managedServicesPath,
			wantAllow: "GET, POST",
		},
		{
			name:      "item",
			method:    http.MethodPatch,
			path:      managedServicesPath + "/example",
			wantAllow: "GET, DELETE",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := &fakeManagedServiceStore{}
			response := lifecycleRequest(
				t,
				NewHandler(testToken, store, store.Ready),
				test.method,
				test.path,
				"",
				true,
			)
			assertStatusCode(t, response, http.StatusMethodNotAllowed)
			if got := response.Header().Get("Allow"); got != test.wantAllow {
				t.Errorf("Allow header = %q, want %q", got, test.wantAllow)
			}
			var body errorResponse
			decodeJSONResponse(t, response, &body)
		})
	}
}

func TestLifecycleEndpointsRejectQueryParameters(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{
			name: "collection selector",
			path: managedServicesPath + "?namespace=other&limit=1",
		},
		{
			name: "item selector",
			path: managedServicesPath + "/example?fieldSelector=status.readyReplicas%3D1",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := &fakeManagedServiceStore{}
			response := lifecycleRequest(
				t,
				NewHandler(testToken, store, store.Ready),
				http.MethodGet,
				test.path,
				"",
				true,
			)
			assertStatusCode(t, response, http.StatusBadRequest)
			if store.listCalled || store.gotName != "" {
				t.Error("query parameters reached the ManagedService store")
			}
		})
	}
}

func TestManagedServiceConflictAndNotFound(t *testing.T) {
	notFound := apierrors.NewNotFound(
		schema.GroupResource{Group: platformv1alpha1.GroupVersion.Group, Resource: "managedservices"},
		"missing",
	)
	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		store      *fakeManagedServiceStore
		wantStatus int
	}{
		{
			name:   "create conflict",
			method: http.MethodPost,
			path:   managedServicesPath,
			body:   `{"name":"existing"}`,
			store: &fakeManagedServiceStore{createErr: apierrors.NewAlreadyExists(
				schema.GroupResource{
					Group:    platformv1alpha1.GroupVersion.Group,
					Resource: "managedservices",
				},
				"existing",
			)},
			wantStatus: http.StatusConflict,
		},
		{
			name:       "get not found",
			method:     http.MethodGet,
			path:       managedServicesPath + "/missing",
			store:      &fakeManagedServiceStore{getErr: notFound},
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "delete not found",
			method:     http.MethodDelete,
			path:       managedServicesPath + "/missing",
			store:      &fakeManagedServiceStore{deleteErr: notFound},
			wantStatus: http.StatusNotFound,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := lifecycleRequest(
				t,
				NewHandler(testToken, test.store, test.store.Ready),
				test.method,
				test.path,
				test.body,
				true,
			)
			assertStatusCode(t, response, test.wantStatus)
			var body errorResponse
			decodeJSONResponse(t, response, &body)
		})
	}
}

func TestCreateManagedServiceRejectsInvalidFields(t *testing.T) {
	overlongMessage := strings.Repeat("x", 121)
	tests := []struct {
		name string
		body string
	}{
		{name: "missing name", body: `{}`},
		{name: "invalid name", body: `{"name":"Invalid_Name"}`},
		{name: "replicas below minimum", body: `{"name":"example","replicas":0}`},
		{name: "replicas above maximum", body: `{"name":"example","replicas":4}`},
		{name: "overlong message", body: `{"name":"example","message":"` + overlongMessage + `"}`},
		{name: "malformed JSON", body: `{"name":`},
		{name: "multiple values", body: `{"name":"one"} {"name":"two"}`},
		{name: "namespace", body: `{"name":"example","namespace":"other"}`},
		{name: "template", body: `{"name":"example","template":"other"}`},
		{name: "image", body: `{"name":"example","image":"example:latest"}`},
		{name: "metadata", body: `{"name":"example","metadata":{"labels":{"x":"y"}}}`},
		{name: "status", body: `{"name":"example","status":{"readyReplicas":1}}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := &fakeManagedServiceStore{}
			response := lifecycleRequest(
				t,
				NewHandler(testToken, store, store.Ready),
				http.MethodPost,
				managedServicesPath,
				test.body,
				true,
			)
			assertStatusCode(t, response, http.StatusBadRequest)
			if store.createdName != "" {
				t.Errorf("invalid request created %q", store.createdName)
			}
		})
	}
}

func TestCreateManagedServiceRejectsOversizedBody(t *testing.T) {
	store := &fakeManagedServiceStore{}
	body := `{"name":"example","message":"` +
		strings.Repeat("x", maxRequestBodyBytes) + `"}`
	response := lifecycleRequest(
		t,
		NewHandler(testToken, store, store.Ready),
		http.MethodPost,
		managedServicesPath,
		body,
		true,
	)
	assertStatusCode(t, response, http.StatusRequestEntityTooLarge)
}

func TestManagedServiceBackendErrorsAreGeneric(t *testing.T) {
	backendErr := errors.New("sensitive backend connection detail")
	tests := []struct {
		name   string
		method string
		path   string
		body   string
		store  *fakeManagedServiceStore
	}{
		{
			name:   "create",
			method: http.MethodPost,
			path:   managedServicesPath,
			body:   `{"name":"example"}`,
			store:  &fakeManagedServiceStore{createErr: backendErr},
		},
		{
			name:   "list",
			method: http.MethodGet,
			path:   managedServicesPath,
			store:  &fakeManagedServiceStore{listErr: backendErr},
		},
		{
			name:   "get",
			method: http.MethodGet,
			path:   managedServicesPath + "/example",
			store:  &fakeManagedServiceStore{getErr: backendErr},
		},
		{
			name:   "delete",
			method: http.MethodDelete,
			path:   managedServicesPath + "/example",
			store:  &fakeManagedServiceStore{deleteErr: backendErr},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := lifecycleRequest(
				t,
				NewHandler(testToken, test.store, test.store.Ready),
				test.method,
				test.path,
				test.body,
				true,
			)
			assertStatusCode(t, response, http.StatusInternalServerError)
			if strings.Contains(response.Body.String(), backendErr.Error()) {
				t.Error("public error exposed backend details")
			}
		})
	}
}

func TestLifecycleEndpointsRequireAuthentication(t *testing.T) {
	store := &fakeManagedServiceStore{}
	tests := []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodPost, path: managedServicesPath, body: `{"name":"example"}`},
		{method: http.MethodGet, path: managedServicesPath},
		{method: http.MethodGet, path: managedServicesPath + "/example"},
		{method: http.MethodDelete, path: managedServicesPath + "/example"},
	}

	for _, test := range tests {
		response := lifecycleRequest(
			t,
			NewHandler(testToken, store, store.Ready),
			test.method,
			test.path,
			test.body,
			false,
		)
		assertStatusCode(t, response, http.StatusUnauthorized)
	}
}

func TestKubernetesReadinessIsGeneric(t *testing.T) {
	tests := []struct {
		name       string
		readyErr   error
		wantStatus int
		wantBody   string
	}{
		{name: "check succeeds", wantStatus: http.StatusOK, wantBody: readinessStatusReady},
		{
			name:       "check fails",
			readyErr:   errors.New("sensitive Kubernetes failure"),
			wantStatus: http.StatusServiceUnavailable,
			wantBody:   readinessStatusNotReady,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := &fakeManagedServiceStore{readyErr: test.readyErr}
			response := lifecycleRequest(
				t,
				NewHandler(testToken, store, store.Ready),
				http.MethodGet,
				"/readyz",
				"",
				false,
			)
			assertStatusCode(t, response, test.wantStatus)
			rawBody := response.Body.String()
			if strings.Contains(rawBody, "sensitive Kubernetes failure") {
				t.Error("readiness response exposed backend details")
			}
			var body statusResponse
			decodeJSONBytes(t, []byte(rawBody), &body)
			if body.Status != test.wantBody {
				t.Errorf("readiness status = %q, want %q", body.Status, test.wantBody)
			}
		})
	}
}

func (store *fakeManagedServiceStore) Create(
	_ context.Context,
	name string,
	replicas int32,
	message string,
) (*platformv1alpha1.ManagedService, error) {
	store.createdName = name
	store.createdReplicas = replicas
	store.createdMessage = message
	if store.createErr != nil {
		return nil, store.createErr
	}
	if store.createResult != nil {
		return store.createResult, nil
	}

	result := newTestManagedService(name, &replicas)
	result.Spec.Message = message

	return &result, nil
}

func (store *fakeManagedServiceStore) List(
	context.Context,
) ([]platformv1alpha1.ManagedService, error) {
	store.listCalled = true

	return store.listResult, store.listErr
}

func (store *fakeManagedServiceStore) Get(
	_ context.Context,
	name string,
) (*platformv1alpha1.ManagedService, error) {
	store.gotName = name

	return store.getResult, store.getErr
}

func (store *fakeManagedServiceStore) Delete(
	_ context.Context,
	name string,
) error {
	store.deletedName = name

	return store.deleteErr
}

func (store *fakeManagedServiceStore) Ready(context.Context) error {
	return store.readyErr
}

func newTestManagedService(
	name string,
	replicas *int32,
) platformv1alpha1.ManagedService {
	return platformv1alpha1.ManagedService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ApplicationsNamespace,
		},
		Spec: platformv1alpha1.ManagedServiceSpec{
			Template: platformv1alpha1.ManagedServiceTemplateDemoHTTP,
			Replicas: replicas,
		},
	}
}

func managedServicePointer(
	managedService platformv1alpha1.ManagedService,
) *platformv1alpha1.ManagedService {
	return &managedService
}

func lifecycleRequest(
	t *testing.T,
	handler http.Handler,
	method string,
	path string,
	body string,
	authenticated bool,
) *httptest.ResponseRecorder {
	t.Helper()

	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if authenticated {
		request.Header.Set("Authorization", "Bearer "+testToken)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	return response
}

func decodeJSONBytes(t *testing.T, contents []byte, target any) {
	t.Helper()

	if err := json.NewDecoder(bytes.NewReader(contents)).Decode(target); err != nil {
		t.Fatalf("decode JSON response: %v", err)
	}
}
