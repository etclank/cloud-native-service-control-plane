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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	platformv1alpha1 "github.com/etclank/cloud-native-service-control-plane/api/v1alpha1"
)

const (
	reconciliationTestImage   = "example.com/demo-http@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	reconciliationTestSecret  = "ghcr-pull"
	reconciliationTestMessage = "Hello from reconciliation"
)

var _ = Describe("ManagedService child reconciliation", func() {
	ctx := context.Background()

	It("creates secure owned resources idempotently", func() {
		managedService := createReconciliationTestResource(
			ctx,
			"managed-service-children",
		)
		reconciler := newReconciliationTestReconciler()

		request := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      managedService.Name,
				Namespace: managedService.Namespace,
			},
		}

		_, err := reconciler.Reconcile(ctx, request)
		Expect(err).NotTo(HaveOccurred())

		deployment := &appsv1.Deployment{}
		Expect(k8sClient.Get(
			ctx,
			request.NamespacedName,
			deployment,
		)).To(Succeed())

		Expect(metav1.IsControlledBy(
			deployment,
			managedService,
		)).To(BeTrue())

		Expect(deployment.Spec.Replicas).NotTo(BeNil())
		Expect(*deployment.Spec.Replicas).To(Equal(int32(1)))

		podSpec := deployment.Spec.Template.Spec
		Expect(podSpec.AutomountServiceAccountToken).NotTo(BeNil())
		Expect(*podSpec.AutomountServiceAccountToken).To(BeFalse())
		Expect(podSpec.ImagePullSecrets).To(Equal(
			[]corev1.LocalObjectReference{
				{Name: reconciliationTestSecret},
			},
		))
		Expect(podSpec.SecurityContext).NotTo(BeNil())
		Expect(podSpec.SecurityContext.RunAsNonRoot).NotTo(BeNil())
		Expect(*podSpec.SecurityContext.RunAsNonRoot).To(BeTrue())

		Expect(podSpec.Containers).To(HaveLen(1))
		container := podSpec.Containers[0]

		Expect(container.Image).To(Equal(reconciliationTestImage))
		Expect(container.Env).To(ContainElement(corev1.EnvVar{
			Name:  "MESSAGE",
			Value: reconciliationTestMessage,
		}))
		Expect(container.Env).To(ContainElement(corev1.EnvVar{
			Name:  "METRICS_PORT",
			Value: "9090",
		}))
		Expect(container.Env).To(ContainElement(corev1.EnvVar{
			Name:  "OTEL_SERVICE_NAME",
			Value: "demo-http",
		}))
		for _, variable := range container.Env {
			Expect(variable.Name).NotTo(HavePrefix("OTEL_EXPORTER_OTLP"))
		}
		Expect(container.Ports).To(Equal([]corev1.ContainerPort{
			{
				Name:          managedServiceHTTPPortName,
				ContainerPort: 8080,
				Protocol:      corev1.ProtocolTCP,
			},
			{
				Name:          "metrics",
				ContainerPort: 9090,
				Protocol:      corev1.ProtocolTCP,
			},
		}))
		Expect(container.ReadinessProbe).NotTo(BeNil())
		Expect(container.LivenessProbe).NotTo(BeNil())
		Expect(container.ReadinessProbe.HTTPGet).NotTo(BeNil())
		Expect(
			container.ReadinessProbe.HTTPGet.Port.String(),
		).To(Equal(managedServiceHTTPPortName))
		Expect(container.LivenessProbe.HTTPGet).NotTo(BeNil())
		Expect(
			container.LivenessProbe.HTTPGet.Port.String(),
		).To(Equal(managedServiceHTTPPortName))
		Expect(container.SecurityContext).NotTo(BeNil())
		Expect(
			container.SecurityContext.AllowPrivilegeEscalation,
		).NotTo(BeNil())
		Expect(
			*container.SecurityContext.AllowPrivilegeEscalation,
		).To(BeFalse())
		Expect(
			container.SecurityContext.ReadOnlyRootFilesystem,
		).NotTo(BeNil())
		Expect(
			*container.SecurityContext.ReadOnlyRootFilesystem,
		).To(BeTrue())
		Expect(container.SecurityContext.Capabilities).NotTo(BeNil())
		Expect(
			container.SecurityContext.Capabilities.Drop,
		).To(ContainElement(corev1.Capability("ALL")))

		service := &corev1.Service{}
		Expect(k8sClient.Get(
			ctx,
			request.NamespacedName,
			service,
		)).To(Succeed())

		Expect(metav1.IsControlledBy(
			service,
			managedService,
		)).To(BeTrue())
		Expect(service.Spec.Type).To(Equal(corev1.ServiceTypeClusterIP))
		Expect(service.Spec.Ports).To(HaveLen(1))
		Expect(service.Spec.Ports[0].Name).To(Equal(managedServiceHTTPPortName))
		Expect(service.Spec.Ports[0].Port).To(Equal(int32(80)))
		Expect(service.Spec.Ports[0].TargetPort.String()).To(Equal(managedServiceHTTPPortName))

		deploymentResourceVersion := deployment.ResourceVersion
		serviceResourceVersion := service.ResourceVersion

		_, err = reconciler.Reconcile(ctx, request)
		Expect(err).NotTo(HaveOccurred())

		reconciledDeployment := &appsv1.Deployment{}
		Expect(k8sClient.Get(
			ctx,
			request.NamespacedName,
			reconciledDeployment,
		)).To(Succeed())

		reconciledService := &corev1.Service{}
		Expect(k8sClient.Get(
			ctx,
			request.NamespacedName,
			reconciledService,
		)).To(Succeed())

		Expect(
			reconciledDeployment.ResourceVersion,
		).To(Equal(deploymentResourceVersion))
		Expect(
			reconciledService.ResourceVersion,
		).To(Equal(serviceResourceVersion))
	})

	It("corrects drift in managed resources", func() {
		managedService := createReconciliationTestResource(
			ctx,
			"managed-service-drift",
		)
		reconciler := newReconciliationTestReconciler()

		request := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      managedService.Name,
				Namespace: managedService.Namespace,
			},
		}

		_, err := reconciler.Reconcile(ctx, request)
		Expect(err).NotTo(HaveOccurred())

		deployment := &appsv1.Deployment{}
		Expect(k8sClient.Get(
			ctx,
			request.NamespacedName,
			deployment,
		)).To(Succeed())

		driftedReplicas := int32(3)
		deployment.Spec.Replicas = &driftedReplicas
		deployment.Spec.Template.Spec.Containers[0].Image =
			"example.com/drifted:latest"

		Expect(k8sClient.Update(ctx, deployment)).To(Succeed())

		service := &corev1.Service{}
		Expect(k8sClient.Get(
			ctx,
			request.NamespacedName,
			service,
		)).To(Succeed())

		service.Spec.Ports[0].Port = 9999
		service.Spec.Selector["unexpected"] = "drift"

		Expect(k8sClient.Update(ctx, service)).To(Succeed())

		_, err = reconciler.Reconcile(ctx, request)
		Expect(err).NotTo(HaveOccurred())

		correctedDeployment := &appsv1.Deployment{}
		Expect(k8sClient.Get(
			ctx,
			request.NamespacedName,
			correctedDeployment,
		)).To(Succeed())

		Expect(correctedDeployment.Spec.Replicas).NotTo(BeNil())
		Expect(
			*correctedDeployment.Spec.Replicas,
		).To(Equal(int32(1)))
		Expect(
			correctedDeployment.Spec.Template.Spec.Containers[0].Image,
		).To(Equal(reconciliationTestImage))

		correctedService := &corev1.Service{}
		Expect(k8sClient.Get(
			ctx,
			request.NamespacedName,
			correctedService,
		)).To(Succeed())

		Expect(correctedService.Spec.Ports[0].Port).To(Equal(int32(80)))
		Expect(
			correctedService.Spec.Selector,
		).NotTo(HaveKey("unexpected"))
	})
})

func newReconciliationTestReconciler() *ManagedServiceReconciler {
	return &ManagedServiceReconciler{
		Client:              k8sClient,
		Scheme:              k8sClient.Scheme(),
		DemoHTTPImage:       reconciliationTestImage,
		ImagePullSecretName: reconciliationTestSecret,
	}
}

func createReconciliationTestResource(
	ctx context.Context,
	name string,
) *platformv1alpha1.ManagedService {
	managedService := &platformv1alpha1.ManagedService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testResourceNamespace,
		},
		Spec: platformv1alpha1.ManagedServiceSpec{
			Template: platformv1alpha1.ManagedServiceTemplateDemoHTTP,
			Message:  reconciliationTestMessage,
		},
	}

	Expect(k8sClient.Create(ctx, managedService)).To(Succeed())

	DeferCleanup(func() {
		objects := []client.Object{
			&appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: testResourceNamespace,
				},
			},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: testResourceNamespace,
				},
			},
			managedService,
		}

		for _, object := range objects {
			Expect(client.IgnoreNotFound(
				k8sClient.Delete(ctx, object),
			)).To(Succeed())
		}
	})

	return managedService
}
