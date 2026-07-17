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
	"errors"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apiMeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	platformv1alpha1 "github.com/etclank/cloud-native-service-control-plane/api/v1alpha1"
)

const (
	reasonWorkloadAvailable            = "WorkloadAvailable"
	reasonWorkloadProgressing          = "WorkloadProgressing"
	reasonReconciliationFailed         = "ReconciliationFailed"
	reasonReconciliationSucceeded      = "ReconciliationSucceeded"
	reasonTemplateConfigurationInvalid = "TemplateConfigurationInvalid"
	reasonDeploymentReconcileFailed    = "DeploymentReconcileFailed"
	reasonServiceReconcileFailed       = "ServiceReconcileFailed"
)

func (r *ManagedServiceReconciler) updateManagedServiceStatus(
	ctx context.Context,
	managedService *platformv1alpha1.ManagedService,
	deployment *appsv1.Deployment,
) error {
	before := managedService.DeepCopy()

	managedService.Status.ObservedGeneration = managedService.Generation
	managedService.Status.ReadyReplicas = deployment.Status.ReadyReplicas
	managedService.Status.Endpoint = fmt.Sprintf(
		"http://%s.%s.svc.cluster.local",
		managedService.Name,
		managedService.Namespace,
	)

	if deployment.Status.ReadyReplicas >= desiredReplicas(managedService) {
		setManagedServiceCondition(
			managedService,
			platformv1alpha1.ManagedServiceConditionAvailable,
			metav1.ConditionTrue,
			reasonWorkloadAvailable,
			"Deployment has the desired number of ready replicas",
		)
		setManagedServiceCondition(
			managedService,
			platformv1alpha1.ManagedServiceConditionProgressing,
			metav1.ConditionFalse,
			reasonWorkloadAvailable,
			"Reconciliation succeeded",
		)
	} else {
		setManagedServiceCondition(
			managedService,
			platformv1alpha1.ManagedServiceConditionAvailable,
			metav1.ConditionFalse,
			reasonWorkloadProgressing,
			"Deployment is waiting for ready replicas",
		)
		setManagedServiceCondition(
			managedService,
			platformv1alpha1.ManagedServiceConditionProgressing,
			metav1.ConditionTrue,
			reasonWorkloadProgressing,
			"Deployment is waiting for ready replicas",
		)
	}

	setManagedServiceCondition(
		managedService,
		platformv1alpha1.ManagedServiceConditionDegraded,
		metav1.ConditionFalse,
		reasonReconciliationSucceeded,
		"Reconciliation succeeded",
	)

	return r.patchManagedServiceStatus(ctx, before, managedService)
}

func (r *ManagedServiceReconciler) recordDegradedStatus(
	ctx context.Context,
	managedService *platformv1alpha1.ManagedService,
	reason string,
	reconcileErr error,
) error {
	before := managedService.DeepCopy()

	managedService.Status.ObservedGeneration = managedService.Generation
	managedService.Status.ReadyReplicas = 0
	managedService.Status.Endpoint = ""

	setManagedServiceCondition(
		managedService,
		platformv1alpha1.ManagedServiceConditionAvailable,
		metav1.ConditionFalse,
		reasonReconciliationFailed,
		"Reconciliation failed",
	)
	setManagedServiceCondition(
		managedService,
		platformv1alpha1.ManagedServiceConditionProgressing,
		metav1.ConditionFalse,
		reasonReconciliationFailed,
		"Reconciliation failed",
	)
	setManagedServiceCondition(
		managedService,
		platformv1alpha1.ManagedServiceConditionDegraded,
		metav1.ConditionTrue,
		reason,
		reconcileErr.Error(),
	)

	if err := r.patchManagedServiceStatus(
		ctx,
		before,
		managedService,
	); err != nil {
		return errors.Join(reconcileErr, err)
	}

	return reconcileErr
}

func setManagedServiceCondition(
	managedService *platformv1alpha1.ManagedService,
	conditionType string,
	status metav1.ConditionStatus,
	reason string,
	message string,
) {
	apiMeta.SetStatusCondition(
		&managedService.Status.Conditions,
		metav1.Condition{
			Type:               conditionType,
			Status:             status,
			Reason:             reason,
			Message:            message,
			ObservedGeneration: managedService.Generation,
		},
	)
}

func (r *ManagedServiceReconciler) patchManagedServiceStatus(
	ctx context.Context,
	before *platformv1alpha1.ManagedService,
	managedService *platformv1alpha1.ManagedService,
) error {
	if equality.Semantic.DeepEqual(before.Status, managedService.Status) {
		return nil
	}

	if err := r.Status().Patch(
		ctx,
		managedService,
		client.MergeFrom(before),
	); err != nil {
		return fmt.Errorf("patch ManagedService status: %w", err)
	}

	return nil
}
