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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ManagedServiceTemplate identifies an operator-approved workload template.
// The custom resource never accepts an arbitrary container image.
//
// +kubebuilder:validation:Enum=demo-http
type ManagedServiceTemplate string

const (
	// ManagedServiceTemplateDemoHTTP creates the portfolio demo HTTP service.
	ManagedServiceTemplateDemoHTTP ManagedServiceTemplate = "demo-http"
)

const (
	// ManagedServiceConditionAvailable indicates that the workload is ready.
	ManagedServiceConditionAvailable = "Available"

	// ManagedServiceConditionProgressing indicates reconciliation is in progress.
	ManagedServiceConditionProgressing = "Progressing"

	// ManagedServiceConditionDegraded indicates reconciliation has failed.
	ManagedServiceConditionDegraded = "Degraded"
)

// ManagedServiceSpec defines the desired state of ManagedService.
type ManagedServiceSpec struct {
	// Template selects an operator-approved workload implementation.
	Template ManagedServiceTemplate `json:"template"`

	// Replicas is the desired number of workload replicas.
	//
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=3
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// Message is an optional response returned by the demo workload.
	//
	// +kubebuilder:validation:MaxLength=120
	// +optional
	Message string `json:"message,omitempty"`
}

// ManagedServiceStatus defines the observed state of ManagedService.
type ManagedServiceStatus struct {
	// ObservedGeneration is the most recent resource generation processed by
	// the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// ReadyReplicas is the number of workload replicas currently ready.
	// +optional
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// Endpoint is the cluster-internal HTTP endpoint for the managed workload.
	// +optional
	Endpoint string `json:"endpoint,omitempty"`

	// Conditions represent the current reconciliation and availability state.
	//
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=ms
// +kubebuilder:printcolumn:name="Template",type=string,JSONPath=".spec.template"
// +kubebuilder:printcolumn:name="Replicas",type=integer,JSONPath=".spec.replicas"
// +kubebuilder:printcolumn:name="Ready",type=integer,JSONPath=".status.readyReplicas"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=".metadata.creationTimestamp"

// ManagedService is the Schema for the managedservices API
type ManagedService struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of ManagedService
	// +required
	Spec ManagedServiceSpec `json:"spec"`

	// status defines the observed state of ManagedService
	// +optional
	Status ManagedServiceStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// ManagedServiceList contains a list of ManagedService
type ManagedServiceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []ManagedService `json:"items"`
}

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(SchemeGroupVersion, &ManagedService{}, &ManagedServiceList{})
		return nil
	})
}
