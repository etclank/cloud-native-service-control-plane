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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	platformv1alpha1 "github.com/etclank/cloud-native-service-control-plane/api/v1alpha1"
)

const (
	// ApplicationsNamespace is the only namespace managed by the API.
	ApplicationsNamespace = "applications"
	defaultReplicas       = int32(1)
)

// ManagedServiceStore provides the lifecycle operations exposed by the API.
type ManagedServiceStore interface {
	Create(
		ctx context.Context,
		name string,
		replicas int32,
		message string,
	) (*platformv1alpha1.ManagedService, error)
	List(ctx context.Context) ([]platformv1alpha1.ManagedService, error)
	Get(ctx context.Context, name string) (*platformv1alpha1.ManagedService, error)
	Delete(ctx context.Context, name string) error
	Ready(ctx context.Context) error
}

// ControllerRuntimeStore implements ManagedServiceStore with a Kubernetes
// controller-runtime client.
type ControllerRuntimeStore struct {
	client client.Client
}

// NewControllerRuntimeStore constructs a Kubernetes-backed store.
func NewControllerRuntimeStore(kubernetesClient client.Client) *ControllerRuntimeStore {
	return &ControllerRuntimeStore{client: kubernetesClient}
}

// Create creates a demo-http ManagedService in the fixed applications scope.
func (store *ControllerRuntimeStore) Create(
	ctx context.Context,
	name string,
	replicas int32,
	message string,
) (*platformv1alpha1.ManagedService, error) {
	managedService := &platformv1alpha1.ManagedService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ApplicationsNamespace,
		},
		Spec: platformv1alpha1.ManagedServiceSpec{
			Template: platformv1alpha1.ManagedServiceTemplateDemoHTTP,
			Replicas: &replicas,
			Message:  message,
		},
	}

	if err := store.client.Create(ctx, managedService); err != nil {
		return nil, err
	}

	return managedService, nil
}

// List lists ManagedServices only from the fixed applications scope.
func (store *ControllerRuntimeStore) List(
	ctx context.Context,
) ([]platformv1alpha1.ManagedService, error) {
	managedServices := &platformv1alpha1.ManagedServiceList{}
	if err := store.client.List(
		ctx,
		managedServices,
		client.InNamespace(ApplicationsNamespace),
	); err != nil {
		return nil, err
	}

	return managedServices.Items, nil
}

// Get gets one ManagedService from the fixed applications scope.
func (store *ControllerRuntimeStore) Get(
	ctx context.Context,
	name string,
) (*platformv1alpha1.ManagedService, error) {
	managedService := &platformv1alpha1.ManagedService{}
	if err := store.client.Get(
		ctx,
		types.NamespacedName{
			Name:      name,
			Namespace: ApplicationsNamespace,
		},
		managedService,
	); err != nil {
		return nil, err
	}

	return managedService, nil
}

// Delete requests deletion without bypassing Kubernetes finalizers.
func (store *ControllerRuntimeStore) Delete(
	ctx context.Context,
	name string,
) error {
	return store.client.Delete(ctx, &platformv1alpha1.ManagedService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ApplicationsNamespace,
		},
	})
}

// Ready verifies Kubernetes access within the fixed applications scope.
func (store *ControllerRuntimeStore) Ready(ctx context.Context) error {
	managedServices := &platformv1alpha1.ManagedServiceList{}

	return store.client.List(
		ctx,
		managedServices,
		client.InNamespace(ApplicationsNamespace),
		client.Limit(1),
	)
}
