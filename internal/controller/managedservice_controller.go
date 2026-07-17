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

package controller

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	platformv1alpha1 "github.com/etclank/cloud-native-service-control-plane/api/v1alpha1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

// ManagedServiceReconciler reconciles a ManagedService object
type ManagedServiceReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// DemoHTTPImage is the administrator-approved immutable image used for the
	// demo-http template.
	DemoHTTPImage string

	// ImagePullSecretName is an optional Secret used to pull managed workload
	// images from a private registry.
	ImagePullSecretName string
}

// +kubebuilder:rbac:groups=platform.eoghanclancy.eu,resources=managedservices,verbs=get;list;watch
// +kubebuilder:rbac:groups=platform.eoghanclancy.eu,resources=managedservices/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=platform.eoghanclancy.eu,resources=managedservices/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the ManagedService object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.24.1/pkg/reconcile
func (r *ManagedServiceReconciler) Reconcile(
	ctx context.Context,
	req ctrl.Request,
) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	managedService := &platformv1alpha1.ManagedService{}
	if err := r.Get(ctx, req.NamespacedName, managedService); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if err := validateImmutableImage(r.DemoHTTPImage); err != nil {
		logger.Error(err, "Invalid operator template configuration")
		return ctrl.Result{}, err
	}

	if err := r.reconcileDeployment(ctx, managedService); err != nil {
		logger.Error(err, "Failed to reconcile Deployment")
		return ctrl.Result{}, err
	}

	if err := r.reconcileService(ctx, managedService); err != nil {
		logger.Error(err, "Failed to reconcile Service")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ManagedServiceReconciler) SetupWithManager(
	mgr ctrl.Manager,
) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&platformv1alpha1.ManagedService{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Named("managedservice").
		Complete(r)
}
