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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apiMeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	platformv1alpha1 "github.com/etclank/cloud-native-service-control-plane/api/v1alpha1"
)

var _ = Describe("ManagedService status", func() {
	ctx := context.Background()

	It("reports Progressing after initial reconciliation", func() {
		managedService := createReconciliationTestResource(
			ctx,
			"managed-service-status-progressing",
		)
		reconciler := newReconciliationTestReconciler()
		request := managedServiceStatusTestRequest(managedService)

		_, err := reconciler.Reconcile(ctx, request)
		Expect(err).NotTo(HaveOccurred())

		reconciled := getManagedServiceForStatusTest(ctx, request)
		Expect(reconciled.Status.ObservedGeneration).To(Equal(
			reconciled.Generation,
		))
		Expect(reconciled.Status.ReadyReplicas).To(Equal(int32(0)))
		Expect(reconciled.Status.Endpoint).To(Equal(
			"http://managed-service-status-progressing.default.svc.cluster.local",
		))

		expectManagedServiceCondition(
			reconciled,
			platformv1alpha1.ManagedServiceConditionAvailable,
			metav1.ConditionFalse,
			reasonWorkloadProgressing,
		)
		expectManagedServiceCondition(
			reconciled,
			platformv1alpha1.ManagedServiceConditionProgressing,
			metav1.ConditionTrue,
			reasonWorkloadProgressing,
		)
		expectManagedServiceCondition(
			reconciled,
			platformv1alpha1.ManagedServiceConditionDegraded,
			metav1.ConditionFalse,
			reasonReconciliationSucceeded,
		)
	})

	It("reports Available when the Deployment is ready", func() {
		managedService := createReconciliationTestResource(
			ctx,
			"managed-service-status-available",
		)
		reconciler := newReconciliationTestReconciler()
		request := managedServiceStatusTestRequest(managedService)

		_, err := reconciler.Reconcile(ctx, request)
		Expect(err).NotTo(HaveOccurred())

		deployment := &appsv1.Deployment{}
		Expect(k8sClient.Get(
			ctx,
			request.NamespacedName,
			deployment,
		)).To(Succeed())

		deployment.Status.Replicas = 1
		deployment.Status.UpdatedReplicas = 1
		deployment.Status.ReadyReplicas = 1
		deployment.Status.AvailableReplicas = 1
		Expect(k8sClient.Status().Update(ctx, deployment)).To(Succeed())

		_, err = reconciler.Reconcile(ctx, request)
		Expect(err).NotTo(HaveOccurred())

		reconciled := getManagedServiceForStatusTest(ctx, request)
		Expect(reconciled.Status.ReadyReplicas).To(Equal(int32(1)))
		expectManagedServiceCondition(
			reconciled,
			platformv1alpha1.ManagedServiceConditionAvailable,
			metav1.ConditionTrue,
			reasonWorkloadAvailable,
		)
		expectManagedServiceCondition(
			reconciled,
			platformv1alpha1.ManagedServiceConditionProgressing,
			metav1.ConditionFalse,
			reasonWorkloadAvailable,
		)
		expectManagedServiceCondition(
			reconciled,
			platformv1alpha1.ManagedServiceConditionDegraded,
			metav1.ConditionFalse,
			reasonReconciliationSucceeded,
		)
	})

	It("reports Degraded for an invalid configured image", func() {
		managedService := createReconciliationTestResource(
			ctx,
			"managed-service-status-degraded",
		)
		reconciler := newReconciliationTestReconciler()
		reconciler.DemoHTTPImage = "example.com/demo-http:latest"
		request := managedServiceStatusTestRequest(managedService)

		_, err := reconciler.Reconcile(ctx, request)
		Expect(err).To(MatchError(ContainSubstring(
			"immutable @sha256 digest",
		)))

		reconciled := getManagedServiceForStatusTest(ctx, request)
		Expect(reconciled.Status.ObservedGeneration).To(Equal(
			reconciled.Generation,
		))
		Expect(reconciled.Status.ReadyReplicas).To(Equal(int32(0)))
		Expect(reconciled.Status.Endpoint).To(BeEmpty())

		expectManagedServiceCondition(
			reconciled,
			platformv1alpha1.ManagedServiceConditionAvailable,
			metav1.ConditionFalse,
			reasonReconciliationFailed,
		)
		expectManagedServiceCondition(
			reconciled,
			platformv1alpha1.ManagedServiceConditionProgressing,
			metav1.ConditionFalse,
			reasonReconciliationFailed,
		)
		degraded := expectManagedServiceCondition(
			reconciled,
			platformv1alpha1.ManagedServiceConditionDegraded,
			metav1.ConditionTrue,
			reasonTemplateConfigurationInvalid,
		)
		Expect(degraded.Message).To(ContainSubstring(
			"immutable @sha256 digest",
		))

		deployment := &appsv1.Deployment{}
		err = k8sClient.Get(ctx, request.NamespacedName, deployment)
		Expect(apierrors.IsNotFound(err)).To(BeTrue())

		service := &corev1.Service{}
		err = k8sClient.Get(ctx, request.NamespacedName, service)
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})

	It("does not patch unchanged status", func() {
		managedService := createReconciliationTestResource(
			ctx,
			"managed-service-status-idempotent",
		)
		reconciler := newReconciliationTestReconciler()
		request := managedServiceStatusTestRequest(managedService)

		_, err := reconciler.Reconcile(ctx, request)
		Expect(err).NotTo(HaveOccurred())

		first := getManagedServiceForStatusTest(ctx, request)
		resourceVersion := first.ResourceVersion
		conditions := first.Status.Conditions

		_, err = reconciler.Reconcile(ctx, request)
		Expect(err).NotTo(HaveOccurred())

		second := getManagedServiceForStatusTest(ctx, request)
		Expect(second.ResourceVersion).To(Equal(resourceVersion))
		Expect(equality.Semantic.DeepEqual(
			second.Status.Conditions,
			conditions,
		)).To(BeTrue())
	})
})

func managedServiceStatusTestRequest(
	managedService *platformv1alpha1.ManagedService,
) reconcile.Request {
	return reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      managedService.Name,
			Namespace: managedService.Namespace,
		},
	}
}

func getManagedServiceForStatusTest(
	ctx context.Context,
	request reconcile.Request,
) *platformv1alpha1.ManagedService {
	managedService := &platformv1alpha1.ManagedService{}
	Expect(k8sClient.Get(
		ctx,
		request.NamespacedName,
		managedService,
	)).To(Succeed())

	return managedService
}

func expectManagedServiceCondition(
	managedService *platformv1alpha1.ManagedService,
	conditionType string,
	status metav1.ConditionStatus,
	reason string,
) *metav1.Condition {
	condition := apiMeta.FindStatusCondition(
		managedService.Status.Conditions,
		conditionType,
	)
	Expect(condition).NotTo(BeNil())
	Expect(condition.Status).To(Equal(status))
	Expect(condition.Reason).To(Equal(reason))
	Expect(condition.ObservedGeneration).To(Equal(
		managedService.Generation,
	))

	return condition
}
