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
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	platformv1alpha1 "github.com/etclank/cloud-native-service-control-plane/api/v1alpha1"
)

func TestControllerRuntimeStoreUsesFixedScope(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := platformv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add ManagedService API to scheme: %v", err)
	}

	otherNamespaceService := &platformv1alpha1.ManagedService{
		ObjectMeta: metav1.ObjectMeta{Name: "other", Namespace: "other-namespace"},
		Spec: platformv1alpha1.ManagedServiceSpec{
			Template: platformv1alpha1.ManagedServiceTemplateDemoHTTP,
		},
	}
	kubernetesClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(otherNamespaceService).
		Build()
	store := NewControllerRuntimeStore(kubernetesClient)
	ctx := context.Background()

	created, err := store.Create(ctx, managedServiceTestName, 2, "hello")
	if err != nil {
		t.Fatalf("create ManagedService: %v", err)
	}
	if created.Namespace != ApplicationsNamespace ||
		created.Spec.Template != platformv1alpha1.ManagedServiceTemplateDemoHTTP ||
		created.Spec.Replicas == nil || *created.Spec.Replicas != 2 {
		t.Errorf("created ManagedService = %#v", created)
	}

	stored := &platformv1alpha1.ManagedService{}
	if err := kubernetesClient.Get(
		ctx,
		types.NamespacedName{Name: managedServiceTestName, Namespace: ApplicationsNamespace},
		stored,
	); err != nil {
		t.Fatalf("get created ManagedService: %v", err)
	}

	items, err := store.List(ctx)
	if err != nil {
		t.Fatalf("list ManagedServices: %v", err)
	}
	if len(items) != 1 || items[0].Name != managedServiceTestName {
		t.Errorf("fixed-scope list = %#v", items)
	}

	got, err := store.Get(ctx, managedServiceTestName)
	if err != nil || got.Namespace != ApplicationsNamespace {
		t.Errorf("fixed-scope get = %#v, %v", got, err)
	}
	if err := store.Ready(ctx); err != nil {
		t.Errorf("readiness check: %v", err)
	}
	if err := store.Delete(ctx, managedServiceTestName); err != nil {
		t.Fatalf("delete ManagedService: %v", err)
	}
	if err := kubernetesClient.Get(
		ctx,
		client.ObjectKey{Name: managedServiceTestName, Namespace: ApplicationsNamespace},
		&platformv1alpha1.ManagedService{},
	); client.IgnoreNotFound(err) != nil || err == nil {
		t.Errorf("ManagedService still exists or delete check failed: %v", err)
	}
}
