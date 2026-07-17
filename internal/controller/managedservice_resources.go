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
	"encoding/hex"
	"fmt"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	platformv1alpha1 "github.com/etclank/cloud-native-service-control-plane/api/v1alpha1"
)

const (
	managedServiceContainerName = "demo-http"
	managedServiceContainerPort = int32(8080)
	managedServiceServicePort   = int32(80)
)

func validateImmutableImage(image string) error {
	repository, digest, found := strings.Cut(image, "@sha256:")
	if !found || repository == "" {
		return fmt.Errorf(
			"demo-http image must use an immutable @sha256 digest",
		)
	}

	decodedDigest, err := hex.DecodeString(digest)
	if err != nil || len(decodedDigest) != 32 {
		return fmt.Errorf(
			"demo-http image contains an invalid sha256 digest",
		)
	}

	return nil
}

func managedServiceSelector(
	managedService *platformv1alpha1.ManagedService,
) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":     "managed-service",
		"app.kubernetes.io/instance": managedService.Name,
	}
}

func managedServiceLabels(
	managedService *platformv1alpha1.ManagedService,
) map[string]string {
	labels := managedServiceSelector(managedService)
	labels["app.kubernetes.io/managed-by"] = "platform-operator"
	labels["platform.eoghanclancy.eu/template"] = string(
		managedService.Spec.Template,
	)

	return labels
}

func desiredReplicas(
	managedService *platformv1alpha1.ManagedService,
) int32 {
	if managedService.Spec.Replicas == nil {
		return 1
	}

	return *managedService.Spec.Replicas
}

func (r *ManagedServiceReconciler) reconcileDeployment(
	ctx context.Context,
	managedService *platformv1alpha1.ManagedService,
) error {
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      managedService.Name,
			Namespace: managedService.Namespace,
		},
	}

	_, err := controllerutil.CreateOrUpdate(
		ctx,
		r.Client,
		deployment,
		func() error {
			if err := controllerutil.SetControllerReference(
				managedService,
				deployment,
				r.Scheme,
			); err != nil {
				return fmt.Errorf(
					"set Deployment controller reference: %w",
					err,
				)
			}

			deployment.Labels = managedServiceLabels(managedService)
			deployment.Spec.Replicas = ptr.To(
				desiredReplicas(managedService),
			)
			deployment.Spec.Selector = &metav1.LabelSelector{
				MatchLabels: managedServiceSelector(managedService),
			}
			deployment.Spec.Template.Labels =
				managedServiceLabels(managedService)

			imagePullSecrets := []corev1.LocalObjectReference(nil)
			if r.ImagePullSecretName != "" {
				imagePullSecrets = []corev1.LocalObjectReference{
					{Name: r.ImagePullSecretName},
				}
			}

			deployment.Spec.Template.Spec = corev1.PodSpec{
				AutomountServiceAccountToken: ptr.To(false),
				DNSPolicy:                    corev1.DNSClusterFirst,
				EnableServiceLinks:           ptr.To(false),
				ImagePullSecrets:             imagePullSecrets,
				RestartPolicy:                corev1.RestartPolicyAlways,
				SchedulerName:                "default-scheduler",
				SecurityContext: &corev1.PodSecurityContext{
					RunAsGroup:   ptr.To[int64](65532),
					RunAsNonRoot: ptr.To(true),
					RunAsUser:    ptr.To[int64](65532),
					SeccompProfile: &corev1.SeccompProfile{
						Type: corev1.SeccompProfileTypeRuntimeDefault,
					},
				},
				TerminationGracePeriodSeconds: ptr.To[int64](10),
				Containers: []corev1.Container{
					{
						Name:            managedServiceContainerName,
						Image:           r.DemoHTTPImage,
						ImagePullPolicy: corev1.PullIfNotPresent,
						Ports: []corev1.ContainerPort{
							{
								Name:          "http",
								ContainerPort: managedServiceContainerPort,
								Protocol:      corev1.ProtocolTCP,
							},
						},
						Env: []corev1.EnvVar{
							{
								Name:  "MESSAGE",
								Value: managedService.Spec.Message,
							},
						},
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU: resource.MustParse(
									"5m",
								),
								corev1.ResourceMemory: resource.MustParse(
									"16Mi",
								),
							},
							Limits: corev1.ResourceList{
								corev1.ResourceCPU: resource.MustParse(
									"50m",
								),
								corev1.ResourceMemory: resource.MustParse(
									"32Mi",
								),
							},
						},
						SecurityContext: &corev1.SecurityContext{
							AllowPrivilegeEscalation: ptr.To(false),
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{"ALL"},
							},
							ReadOnlyRootFilesystem: ptr.To(true),
							RunAsNonRoot:           ptr.To(true),
						},
						LivenessProbe: managedServiceHTTPProbe(
							"/healthz",
							10,
						),
						ReadinessProbe: managedServiceHTTPProbe(
							"/readyz",
							2,
						),
						TerminationMessagePath:   "/dev/termination-log",
						TerminationMessagePolicy: corev1.TerminationMessageReadFile,
					},
				},
			}

			return nil
		},
	)

	if err != nil {
		return fmt.Errorf("reconcile Deployment: %w", err)
	}

	return nil
}

func managedServiceHTTPProbe(
	path string,
	initialDelaySeconds int32,
) *corev1.Probe {
	return &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path:   path,
				Port:   intstr.FromString("http"),
				Scheme: corev1.URISchemeHTTP,
			},
		},
		InitialDelaySeconds: initialDelaySeconds,
		TimeoutSeconds:      2,
		PeriodSeconds:       5,
		SuccessThreshold:    1,
		FailureThreshold:    6,
	}
}

func (r *ManagedServiceReconciler) reconcileService(
	ctx context.Context,
	managedService *platformv1alpha1.ManagedService,
) error {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      managedService.Name,
			Namespace: managedService.Namespace,
		},
	}

	_, err := controllerutil.CreateOrUpdate(
		ctx,
		r.Client,
		service,
		func() error {
			if err := controllerutil.SetControllerReference(
				managedService,
				service,
				r.Scheme,
			); err != nil {
				return fmt.Errorf(
					"set Service controller reference: %w",
					err,
				)
			}

			service.Labels = managedServiceLabels(managedService)
			service.Spec.Type = corev1.ServiceTypeClusterIP
			service.Spec.Selector =
				managedServiceSelector(managedService)
			service.Spec.Ports = []corev1.ServicePort{
				{
					Name:       "http",
					Port:       managedServiceServicePort,
					Protocol:   corev1.ProtocolTCP,
					TargetPort: intstr.FromString("http"),
				},
			}

			return nil
		},
	)

	if err != nil {
		return fmt.Errorf("reconcile Service: %w", err)
	}

	return nil
}
