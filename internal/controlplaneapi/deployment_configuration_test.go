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
	"errors"
	"io"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/yaml"
)

const (
	controlPlaneAPINamespace = "platform-system"
	certificateKind          = "Certificate"
	ingressKind              = "Ingress"
	middlewareKind           = "Middleware"
	managedServiceRoleName   = "control-plane-api-managedservice"
	productionIssuerName     = "letsencrypt-production"
	publicAPIHostname        = "api.platform.eoghanclancy.eu"
	rateLimitMiddlewareName  = "control-plane-api-rate-limit"
	redirectMiddlewareName   = "control-plane-api-redirect-https"
	serviceAccountKind       = "ServiceAccount"
	tlsSecretName            = "control-plane-api-tls"
	roleKind                 = "Role"
	approvedAPIImage         = "ghcr.io/etclank/cloud-native-service-control-plane-api@sha256:" +
		"604c16f04b00272b7b45072ff0c50c5c2d081fbc4ee795e62a4ed1fc861df36e"
)

var (
	controlPlaneAPIResourceName = "control-plane-" + "api"
	managedServicesResourceName = "managed" + "services"
	healthEndpointPath          = "/" + "healthz"
	readinessEndpointPath       = "/" + "readyz"
)

type renderedResourceKey struct {
	Kind      string
	Namespace string
	Name      string
}

type renderedCertificate struct {
	Spec struct {
		SecretName string   `json:"secretName"`
		DNSNames   []string `json:"dnsNames"`
		IssuerRef  struct {
			Group string `json:"group"`
			Kind  string `json:"kind"`
			Name  string `json:"name"`
		} `json:"issuerRef"`
	} `json:"spec"`
}

type renderedMiddleware struct {
	Spec struct {
		RedirectScheme *struct {
			Scheme    string `json:"scheme"`
			Permanent bool   `json:"permanent"`
		} `json:"redirectScheme,omitempty"`
		RateLimit *struct {
			Average int64  `json:"average"`
			Burst   int64  `json:"burst"`
			Period  string `json:"period"`
		} `json:"rateLimit,omitempty"`
	} `json:"spec"`
}

func TestRenderedControlPlaneAPIConfiguration(t *testing.T) {
	resources := renderControlPlaneAPIResources(t)
	wantKeys := map[renderedResourceKey]struct{}{
		{Kind: "Namespace", Name: ApplicationsNamespace}: {},
		{
			Kind:      serviceAccountKind,
			Namespace: controlPlaneAPINamespace,
			Name:      controlPlaneAPIResourceName,
		}: {},
		{
			Kind:      roleKind,
			Namespace: ApplicationsNamespace,
			Name:      managedServiceRoleName,
		}: {},
		{
			Kind:      "RoleBinding",
			Namespace: ApplicationsNamespace,
			Name:      managedServiceRoleName,
		}: {},
		{
			Kind:      "Service",
			Namespace: controlPlaneAPINamespace,
			Name:      controlPlaneAPIResourceName,
		}: {},
		{
			Kind:      "Deployment",
			Namespace: controlPlaneAPINamespace,
			Name:      controlPlaneAPIResourceName,
		}: {},
		{
			Kind:      certificateKind,
			Namespace: controlPlaneAPINamespace,
			Name:      controlPlaneAPIResourceName,
		}: {},
		{
			Kind:      ingressKind,
			Namespace: controlPlaneAPINamespace,
			Name:      controlPlaneAPIResourceName,
		}: {},
		{
			Kind:      middlewareKind,
			Namespace: controlPlaneAPINamespace,
			Name:      redirectMiddlewareName,
		}: {},
		{
			Kind:      middlewareKind,
			Namespace: controlPlaneAPINamespace,
			Name:      rateLimitMiddlewareName,
		}: {},
	}
	gotKeys := make(map[renderedResourceKey]struct{}, len(resources))
	for key := range resources {
		gotKeys[key] = struct{}{}
	}
	if !reflect.DeepEqual(gotKeys, wantKeys) {
		t.Errorf("rendered resource inventory = %#v, want %#v", gotKeys, wantKeys)
	}

	assertControlPlaneAPIServiceAccount(t, resources)
	assertControlPlaneAPIRBAC(t, resources)
	assertControlPlaneAPIDeployment(t, resources)
	assertControlPlaneAPIService(t, resources)
	assertControlPlaneAPITLS(t, resources)
}

func assertControlPlaneAPIServiceAccount(
	t *testing.T,
	resources map[renderedResourceKey]*unstructured.Unstructured,
) {
	t.Helper()

	serviceAccount := &corev1.ServiceAccount{}
	convertRenderedResource(t, resources, renderedResourceKey{
		Kind:      serviceAccountKind,
		Namespace: controlPlaneAPINamespace,
		Name:      controlPlaneAPIResourceName,
	}, serviceAccount)
	if serviceAccount.AutomountServiceAccountToken == nil ||
		!*serviceAccount.AutomountServiceAccountToken {
		t.Error("control-plane API ServiceAccount does not mount its Kubernetes token")
	}
}

func assertControlPlaneAPIRBAC(
	t *testing.T,
	resources map[renderedResourceKey]*unstructured.Unstructured,
) {
	t.Helper()

	role := &rbacv1.Role{}
	convertRenderedResource(t, resources, renderedResourceKey{
		Kind:      roleKind,
		Namespace: ApplicationsNamespace,
		Name:      managedServiceRoleName,
	}, role)
	wantRules := []rbacv1.PolicyRule{{
		APIGroups: []string{"platform.eoghanclancy.eu"},
		Resources: []string{managedServicesResourceName},
		Verbs:     []string{"create", "get", "list", "delete"},
	}}
	if !reflect.DeepEqual(role.Rules, wantRules) {
		t.Errorf("control-plane API Role rules = %#v, want %#v", role.Rules, wantRules)
	}

	roleBinding := &rbacv1.RoleBinding{}
	convertRenderedResource(t, resources, renderedResourceKey{
		Kind:      "RoleBinding",
		Namespace: ApplicationsNamespace,
		Name:      managedServiceRoleName,
	}, roleBinding)
	wantRoleRef := rbacv1.RoleRef{
		APIGroup: "rbac.authorization.k8s.io",
		Kind:     roleKind,
		Name:     managedServiceRoleName,
	}
	wantSubjects := []rbacv1.Subject{{
		Kind:      serviceAccountKind,
		Name:      controlPlaneAPIResourceName,
		Namespace: controlPlaneAPINamespace,
	}}
	if roleBinding.RoleRef != wantRoleRef ||
		!reflect.DeepEqual(roleBinding.Subjects, wantSubjects) {
		t.Errorf(
			"control-plane API RoleBinding = roleRef %#v, subjects %#v",
			roleBinding.RoleRef,
			roleBinding.Subjects,
		)
	}
}

func assertControlPlaneAPIDeployment(
	t *testing.T,
	resources map[renderedResourceKey]*unstructured.Unstructured,
) {
	t.Helper()

	deployment := &appsv1.Deployment{}
	convertRenderedResource(t, resources, renderedResourceKey{
		Kind:      "Deployment",
		Namespace: controlPlaneAPINamespace,
		Name:      controlPlaneAPIResourceName,
	}, deployment)
	assertDeploymentRollout(t, deployment)
	assertPodSecurityAndIdentity(t, &deployment.Spec.Template.Spec)
	assertAPIContainer(t, &deployment.Spec.Template.Spec.Containers[0])
	assertTokenVolume(t, &deployment.Spec.Template.Spec)
}

func assertDeploymentRollout(t *testing.T, deployment *appsv1.Deployment) {
	t.Helper()

	if deployment.Spec.Replicas == nil || *deployment.Spec.Replicas != 1 ||
		deployment.Spec.RevisionHistoryLimit == nil ||
		*deployment.Spec.RevisionHistoryLimit != 3 ||
		deployment.Spec.ProgressDeadlineSeconds == nil ||
		*deployment.Spec.ProgressDeadlineSeconds != 300 {
		t.Errorf("control-plane API rollout bounds = %#v", deployment.Spec)
	}
	strategy := deployment.Spec.Strategy
	if strategy.Type != appsv1.RollingUpdateDeploymentStrategyType ||
		strategy.RollingUpdate == nil ||
		strategy.RollingUpdate.MaxUnavailable == nil ||
		strategy.RollingUpdate.MaxSurge == nil ||
		*strategy.RollingUpdate.MaxUnavailable != intstr.FromInt32(0) ||
		*strategy.RollingUpdate.MaxSurge != intstr.FromInt32(1) {
		t.Errorf("control-plane API rollout strategy = %#v", strategy)
	}
}

func assertPodSecurityAndIdentity(t *testing.T, podSpec *corev1.PodSpec) {
	t.Helper()

	if podSpec.ServiceAccountName != controlPlaneAPIResourceName ||
		podSpec.AutomountServiceAccountToken == nil ||
		!*podSpec.AutomountServiceAccountToken {
		t.Errorf("control-plane API Pod identity = %#v", podSpec)
	}
	if !reflect.DeepEqual(
		podSpec.ImagePullSecrets,
		[]corev1.LocalObjectReference{{Name: "ghcr-pull"}},
	) {
		t.Errorf("control-plane API imagePullSecrets = %#v", podSpec.ImagePullSecrets)
	}
	securityContext := podSpec.SecurityContext
	if securityContext == nil || securityContext.RunAsNonRoot == nil ||
		!*securityContext.RunAsNonRoot || securityContext.RunAsUser == nil ||
		*securityContext.RunAsUser != 65532 || securityContext.RunAsGroup == nil ||
		*securityContext.RunAsGroup != 65532 || securityContext.FSGroup == nil ||
		*securityContext.FSGroup != 65532 || securityContext.SeccompProfile == nil ||
		securityContext.SeccompProfile.Type != corev1.SeccompProfileTypeRuntimeDefault {
		t.Errorf("control-plane API Pod security context = %#v", securityContext)
	}
	if podSpec.HostNetwork || podSpec.HostPID || podSpec.HostIPC {
		t.Error("control-plane API Pod enables host namespace access")
	}
}

func assertAPIContainer(t *testing.T, container *corev1.Container) {
	t.Helper()

	if container.Name != controlPlaneAPIResourceName || container.Image != approvedAPIImage ||
		container.ImagePullPolicy != corev1.PullIfNotPresent ||
		strings.Contains(container.Image, ":latest") ||
		strings.Contains(container.Image, ":sha-") {
		t.Errorf("control-plane API container image = %q", container.Image)
	}
	wantPorts := []corev1.ContainerPort{
		{
			Name:          "http",
			ContainerPort: 8080,
			Protocol:      corev1.ProtocolTCP,
		},
		{
			Name:          "metrics",
			ContainerPort: 9090,
			Protocol:      corev1.ProtocolTCP,
		},
	}
	if !reflect.DeepEqual(container.Ports, wantPorts) {
		t.Fatalf("control-plane API container ports = %#v", container.Ports)
	}
	for _, port := range container.Ports {
		if port.HostPort != 0 {
			t.Errorf("control-plane API container exposes hostPort %d", port.HostPort)
		}
	}
	wantEnvironment := []corev1.EnvVar{
		{Name: "PORT", Value: "8080"},
		{Name: "METRICS_PORT", Value: "9090"},
		{Name: "OTEL_SERVICE_NAME", Value: controlPlaneAPIResourceName},
		{Name: "API_TOKEN_FILE", Value: "/var/run/secrets/control-plane-api/token"},
	}
	if !reflect.DeepEqual(container.Env, wantEnvironment) {
		t.Errorf("control-plane API environment = %#v", container.Env)
	}
	for _, variable := range container.Env {
		if strings.HasPrefix(variable.Name, "OTEL_EXPORTER_OTLP") {
			t.Errorf("control-plane API configures an OTLP endpoint: %q", variable.Name)
		}
	}
	assertContainerSecurity(t, container.SecurityContext)
	assertContainerProbesAndResources(t, container)
}

func assertContainerSecurity(t *testing.T, securityContext *corev1.SecurityContext) {
	t.Helper()

	if securityContext == nil || securityContext.AllowPrivilegeEscalation == nil ||
		*securityContext.AllowPrivilegeEscalation || securityContext.Privileged == nil ||
		*securityContext.Privileged || securityContext.ReadOnlyRootFilesystem == nil ||
		!*securityContext.ReadOnlyRootFilesystem || securityContext.Capabilities == nil ||
		!reflect.DeepEqual(
			securityContext.Capabilities.Drop,
			[]corev1.Capability{"ALL"},
		) {
		t.Errorf("control-plane API container security context = %#v", securityContext)
	}
}

func assertContainerProbesAndResources(t *testing.T, container *corev1.Container) {
	t.Helper()

	if container.LivenessProbe == nil || container.LivenessProbe.HTTPGet == nil ||
		container.LivenessProbe.HTTPGet.Path != healthEndpointPath ||
		container.LivenessProbe.HTTPGet.Port != intstr.FromString("http") ||
		container.ReadinessProbe == nil || container.ReadinessProbe.HTTPGet == nil ||
		container.ReadinessProbe.HTTPGet.Path != readinessEndpointPath ||
		container.ReadinessProbe.HTTPGet.Port != intstr.FromString("http") {
		t.Errorf(
			"control-plane API probes = liveness %#v, readiness %#v",
			container.LivenessProbe,
			container.ReadinessProbe,
		)
	}
	wantRequests := corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("10m"),
		corev1.ResourceMemory: resource.MustParse("32Mi"),
	}
	wantLimits := corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("100m"),
		corev1.ResourceMemory: resource.MustParse("128Mi"),
	}
	if !reflect.DeepEqual(container.Resources.Requests, wantRequests) ||
		!reflect.DeepEqual(container.Resources.Limits, wantLimits) {
		t.Errorf("control-plane API resources = %#v", container.Resources)
	}
}

func assertTokenVolume(t *testing.T, podSpec *corev1.PodSpec) {
	t.Helper()

	if len(podSpec.Volumes) != 1 || podSpec.Volumes[0].Secret == nil {
		t.Fatalf("control-plane API token volumes = %#v", podSpec.Volumes)
	}
	secret := podSpec.Volumes[0].Secret
	if podSpec.Volumes[0].Name != "api-token" ||
		secret.SecretName != "control-plane-api-token" || len(secret.Items) != 1 ||
		secret.Items[0].Key != "token" || secret.Items[0].Path != "token" ||
		secret.Items[0].Mode == nil || *secret.Items[0].Mode != 0o440 {
		t.Errorf("control-plane API token volume = %#v", podSpec.Volumes[0])
	}
	wantMounts := []corev1.VolumeMount{{
		Name:      "api-token",
		MountPath: "/var/run/secrets/control-plane-api",
		ReadOnly:  true,
	}}
	if !reflect.DeepEqual(podSpec.Containers[0].VolumeMounts, wantMounts) {
		t.Errorf("control-plane API token mounts = %#v", podSpec.Containers[0].VolumeMounts)
	}
	for _, volume := range podSpec.Volumes {
		if volume.HostPath != nil {
			t.Errorf("control-plane API Pod uses hostPath volume %q", volume.Name)
		}
	}
}

func assertControlPlaneAPIService(
	t *testing.T,
	resources map[renderedResourceKey]*unstructured.Unstructured,
) {
	t.Helper()

	service := &corev1.Service{}
	convertRenderedResource(t, resources, renderedResourceKey{
		Kind:      "Service",
		Namespace: controlPlaneAPINamespace,
		Name:      controlPlaneAPIResourceName,
	}, service)
	if service.Spec.Type != corev1.ServiceTypeClusterIP ||
		len(service.Spec.Ports) != 1 || service.Spec.Ports[0].Port != 80 ||
		service.Spec.Ports[0].TargetPort != intstr.FromString("http") {
		t.Errorf("control-plane API Service = %#v", service.Spec)
	}
}

func assertControlPlaneAPITLS(
	t *testing.T,
	resources map[renderedResourceKey]*unstructured.Unstructured,
) {
	t.Helper()

	certificate := &renderedCertificate{}
	convertRenderedResource(t, resources, renderedResourceKey{
		Kind:      certificateKind,
		Namespace: controlPlaneAPINamespace,
		Name:      controlPlaneAPIResourceName,
	}, certificate)
	if certificate.Spec.SecretName != tlsSecretName ||
		!reflect.DeepEqual(certificate.Spec.DNSNames, []string{publicAPIHostname}) ||
		certificate.Spec.IssuerRef.Group != "cert-manager.io" ||
		certificate.Spec.IssuerRef.Kind != "ClusterIssuer" ||
		certificate.Spec.IssuerRef.Name != productionIssuerName {
		t.Errorf("control-plane API Certificate = %#v", certificate.Spec)
	}

	assertControlPlaneAPIMiddlewares(t, resources)
	assertControlPlaneAPIIngress(t, resources)
}

func assertControlPlaneAPIMiddlewares(
	t *testing.T,
	resources map[renderedResourceKey]*unstructured.Unstructured,
) {
	t.Helper()

	redirect := &renderedMiddleware{}
	convertRenderedResource(t, resources, renderedResourceKey{
		Kind:      middlewareKind,
		Namespace: controlPlaneAPINamespace,
		Name:      redirectMiddlewareName,
	}, redirect)
	if redirect.Spec.RedirectScheme == nil ||
		redirect.Spec.RedirectScheme.Scheme != "https" ||
		!redirect.Spec.RedirectScheme.Permanent || redirect.Spec.RateLimit != nil {
		t.Errorf("control-plane API redirect Middleware = %#v", redirect.Spec)
	}

	rateLimit := &renderedMiddleware{}
	convertRenderedResource(t, resources, renderedResourceKey{
		Kind:      middlewareKind,
		Namespace: controlPlaneAPINamespace,
		Name:      rateLimitMiddlewareName,
	}, rateLimit)
	if rateLimit.Spec.RateLimit == nil || rateLimit.Spec.RateLimit.Average != 5 ||
		rateLimit.Spec.RateLimit.Burst != 10 ||
		rateLimit.Spec.RateLimit.Period != "1s" ||
		rateLimit.Spec.RedirectScheme != nil {
		t.Errorf("control-plane API rate-limit Middleware = %#v", rateLimit.Spec)
	}
}

func assertControlPlaneAPIIngress(
	t *testing.T,
	resources map[renderedResourceKey]*unstructured.Unstructured,
) {
	t.Helper()

	ingress := &networkingv1.Ingress{}
	convertRenderedResource(t, resources, renderedResourceKey{
		Kind:      ingressKind,
		Namespace: controlPlaneAPINamespace,
		Name:      controlPlaneAPIResourceName,
	}, ingress)
	wantAnnotations := map[string]string{
		"traefik.ingress.kubernetes.io/router.entrypoints": "web,websecure",
		"traefik.ingress.kubernetes.io/router.middlewares": "platform-system-control-plane-api-redirect-https@kubernetescrd," +
			"platform-system-control-plane-api-rate-limit@kubernetescrd",
	}
	if !reflect.DeepEqual(ingress.Annotations, wantAnnotations) {
		t.Errorf("control-plane API Ingress annotations = %#v", ingress.Annotations)
	}
	if ingress.Spec.IngressClassName == nil ||
		*ingress.Spec.IngressClassName != "traefik" ||
		!reflect.DeepEqual(ingress.Spec.TLS, []networkingv1.IngressTLS{{
			Hosts:      []string{publicAPIHostname},
			SecretName: tlsSecretName,
		}}) {
		t.Errorf("control-plane API Ingress TLS configuration = %#v", ingress.Spec)
	}
	if len(ingress.Spec.Rules) != 1 || ingress.Spec.Rules[0].Host != publicAPIHostname ||
		strings.Contains(ingress.Spec.Rules[0].Host, "*") ||
		ingress.Spec.Rules[0].HTTP == nil ||
		len(ingress.Spec.Rules[0].HTTP.Paths) != 1 {
		t.Fatalf("control-plane API Ingress rules = %#v", ingress.Spec.Rules)
	}
	path := ingress.Spec.Rules[0].HTTP.Paths[0]
	if path.Path != "/" || path.PathType == nil ||
		*path.PathType != networkingv1.PathTypePrefix ||
		path.Backend.Service == nil ||
		path.Backend.Service.Name != controlPlaneAPIResourceName ||
		path.Backend.Service.Port.Number != 80 {
		t.Errorf("control-plane API Ingress path = %#v", path)
	}
}

func renderControlPlaneAPIResources(
	t *testing.T,
) map[renderedResourceKey]*unstructured.Unstructured {
	t.Helper()

	repositoryRoot := filepath.Join("..", "..")
	command := exec.Command(
		filepath.Join(repositoryRoot, "bin", "kustomize"),
		"build",
		filepath.Join(repositoryRoot, "kubernetes", "platform", "control-plane-api"),
	)
	rendered, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("render control-plane API configuration: %v\n%s", err, rendered)
	}

	resources := make(map[renderedResourceKey]*unstructured.Unstructured)
	decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(rendered), 4096)
	for {
		object := &unstructured.Unstructured{}
		err := decoder.Decode(object)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("decode rendered control-plane API resource: %v", err)
		}
		key := renderedResourceKey{
			Kind:      object.GetKind(),
			Namespace: object.GetNamespace(),
			Name:      object.GetName(),
		}
		if _, exists := resources[key]; exists {
			t.Fatalf("duplicate rendered control-plane API resource: %#v", key)
		}
		resources[key] = object
	}

	return resources
}

func convertRenderedResource(
	t *testing.T,
	resources map[renderedResourceKey]*unstructured.Unstructured,
	key renderedResourceKey,
	target any,
) {
	t.Helper()

	object, exists := resources[key]
	if !exists {
		t.Fatalf("rendered control-plane API resource not found: %#v", key)
	}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(
		object.Object,
		target,
	); err != nil {
		t.Fatalf("convert rendered control-plane API resource %#v: %v", key, err)
	}
}
