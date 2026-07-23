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

package observability

import (
	"bytes"
	"errors"
	"io"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/utils/ptr"
)

const (
	candidateValuesFile            = "values-h8.4b-candidate.yaml"
	prometheusServiceAccount       = "prometheus-server"
	kubeStateMetricsServiceAccount = "kube-state-metrics"
	prometheusDeploymentName       = "observability-prometheus-server"
	kubeStateMetricsDeploymentName = "observability-kube-state-metrics"
	prometheusDiscoveryRole        = "observability-prometheus-discovery"
	prometheusOperatorBinding      = "observability-prometheus-operator-metrics-reader"
	operatorMetricsReaderRole      = "platform-operator-metrics-reader"
	kubernetesAPICIDR              = "142.132.178.45/32"
)

func TestH84BCandidateRenderIsExplicitDeterministicAndBounded(t *testing.T) {
	first, resources := renderH84BCandidate(t)
	second, repeatedResources := renderH84BCandidate(t)
	if !bytes.Equal(first, second) {
		t.Fatal("H8.4B candidate render is not byte-for-byte deterministic")
	}
	if !reflect.DeepEqual(resources, repeatedResources) {
		t.Fatal("H8.4B candidate decoded resources differ between renders")
	}

	wantInventory := map[objectKey]struct{}{
		{Kind: namespaceKind, Name: observabilityNamespace}:                                         {},
		{Kind: networkPolicyKind, Namespace: observabilityNamespace, Name: defaultDenyPolicyName}:   {},
		{Kind: networkPolicyKind, Namespace: observabilityNamespace, Name: otlpIngressPolicyName}:   {},
		{Kind: networkPolicyKind, Namespace: observabilityNamespace, Name: "prometheus-dns-egress"}: {},
		{
			Kind: networkPolicyKind, Namespace: observabilityNamespace,
			Name: "prometheus-kubernetes-api-egress",
		}: {},
		{
			Kind: networkPolicyKind, Namespace: observabilityNamespace,
			Name: "prometheus-kube-state-metrics-egress",
		}: {},
		{
			Kind: networkPolicyKind, Namespace: observabilityNamespace,
			Name: "prometheus-platform-operator-metrics-egress",
		}: {},
		{
			Kind: networkPolicyKind, Namespace: observabilityNamespace,
			Name: "prometheus-opentelemetry-collector-egress",
		}: {},
		{
			Kind: networkPolicyKind, Namespace: observabilityNamespace,
			Name: "opentelemetry-collector-metrics-ingress",
		}: {},
		{
			Kind: networkPolicyKind, Namespace: observabilityNamespace,
			Name: "prometheus-control-plane-api-metrics-egress",
		}: {},
		{
			Kind: networkPolicyKind, Namespace: observabilityNamespace,
			Name: "prometheus-managed-demo-metrics-egress",
		}: {},
		{Kind: networkPolicyKind, Namespace: observabilityNamespace, Name: "kube-state-metrics-dns-egress"}: {},
		{
			Kind: networkPolicyKind, Namespace: observabilityNamespace,
			Name: "kube-state-metrics-kubernetes-api-egress",
		}: {},
		{
			Kind: networkPolicyKind, Namespace: observabilityNamespace,
			Name: "kube-state-metrics-metrics-ingress",
		}: {},
		{Kind: serviceAccountKind, Namespace: observabilityNamespace, Name: collectorChartName}:              {},
		{Kind: serviceAccountKind, Namespace: observabilityNamespace, Name: prometheusServiceAccount}:        {},
		{Kind: serviceAccountKind, Namespace: observabilityNamespace, Name: kubeStateMetricsServiceAccount}:  {},
		{Kind: configMapKind, Namespace: observabilityNamespace, Name: collectorResourceName}:                {},
		{Kind: configMapKind, Namespace: observabilityNamespace, Name: prometheusDeploymentName}:             {},
		{Kind: persistentVolumeClaimKind, Namespace: observabilityNamespace, Name: prometheusDeploymentName}: {},
		{Kind: clusterRoleKind, Name: prometheusDiscoveryRole}:                                               {},
		{Kind: clusterRoleKind, Name: kubeStateMetricsDeploymentName}:                                        {},
		{Kind: clusterRoleBindingKind, Name: prometheusDiscoveryRole}:                                        {},
		{Kind: clusterRoleBindingKind, Name: prometheusOperatorBinding}:                                      {},
		{Kind: clusterRoleBindingKind, Name: kubeStateMetricsDeploymentName}:                                 {},
		{Kind: serviceKind, Namespace: observabilityNamespace, Name: collectorChartName}:                     {},
		{Kind: serviceKind, Namespace: observabilityNamespace, Name: prometheusDeploymentName}:               {},
		{Kind: serviceKind, Namespace: observabilityNamespace, Name: kubeStateMetricsDeploymentName}:         {},
		{Kind: daemonSetKind, Namespace: observabilityNamespace, Name: collectorResourceName}:                {},
		{Kind: deploymentKind, Namespace: observabilityNamespace, Name: prometheusDeploymentName}:            {},
		{Kind: deploymentKind, Namespace: observabilityNamespace, Name: kubeStateMetricsDeploymentName}:      {},
	}
	gotInventory := make(map[objectKey]struct{}, len(resources))
	for key := range resources {
		gotInventory[key] = struct{}{}
	}
	if !reflect.DeepEqual(gotInventory, wantInventory) {
		t.Fatalf("H8.4B candidate inventory = %#v, want %#v", gotInventory, wantInventory)
	}
	for key := range resources {
		if slices.Contains([]string{
			"Secret",
			"Ingress",
			"CustomResourceDefinition",
			"StatefulSet",
			"Pod",
			"Job",
		}, key.Kind) {
			t.Errorf("H8.4B candidate contains forbidden object %#v", key)
		}
		if key.Kind == daemonSetKind && key.Name != collectorResourceName {
			t.Errorf("H8.4B candidate contains unexpected DaemonSet %#v", key)
		}
	}
	if bytes.Contains(first, []byte("targetRevision:")) ||
		bytes.Contains(first, []byte("kind: Application")) ||
		bytes.Contains(first, []byte("kind: AppProject")) {
		t.Error("H8.4B candidate contains GitOps bootstrap configuration")
	}

	t.Logf("H8.4C candidate inventory contains %d exact objects", len(resources))
}

func TestH84BCandidateRuntimeSecurityStorageAndBudget(t *testing.T) {
	_, resources := renderH84BCandidate(t)

	prometheus := &appsv1.Deployment{}
	convertResource(t, resources, namespacedKey(deploymentKind, prometheusDeploymentName), prometheus)
	kubeStateMetrics := &appsv1.Deployment{}
	convertResource(t, resources, namespacedKey(deploymentKind, kubeStateMetricsDeploymentName), kubeStateMetrics)

	assertCandidateDeployment(
		t,
		prometheus,
		prometheusServiceAccount,
		"quay.io/prometheus/prometheus@sha256:"+
			"bd2dcadfb0d1096e2a4c21817ac7af918e2f19ff628e4bf25fd67a924c13dd80",
		"100m",
		"256Mi",
		"400m",
		"512Mi",
		true,
	)
	if prometheus.Spec.Strategy.Type != appsv1.RecreateDeploymentStrategyType ||
		prometheus.Spec.Strategy.RollingUpdate != nil ||
		prometheus.Spec.Template.Spec.TerminationGracePeriodSeconds == nil ||
		*prometheus.Spec.Template.Spec.TerminationGracePeriodSeconds != 300 {
		t.Errorf("Prometheus rollout and shutdown contract = %#v", prometheus.Spec)
	}
	prometheusContainer := prometheus.Spec.Template.Spec.Containers[0]
	wantPrometheusArgs := []string{
		"--storage.tsdb.retention.time=72h",
		"--storage.tsdb.retention.size=2GB",
		"--config.file=/etc/config/prometheus.yml",
		"--storage.tsdb.path=/data",
		"--web.console.libraries=/etc/prometheus/console_libraries",
		"--web.console.templates=/etc/prometheus/consoles",
	}
	if !reflect.DeepEqual(prometheusContainer.Args, wantPrometheusArgs) {
		t.Errorf("Prometheus arguments = %#v, want %#v", prometheusContainer.Args, wantPrometheusArgs)
	}
	if len(prometheusContainer.VolumeMounts) != 2 ||
		prometheusContainer.VolumeMounts[1].Name != "storage-volume" ||
		prometheusContainer.VolumeMounts[1].MountPath != "/data" {
		t.Errorf("Prometheus writable storage mounts = %#v", prometheusContainer.VolumeMounts)
	}
	assertNoHostStorage(t, prometheus.Spec.Template.Spec)

	assertCandidateDeployment(
		t,
		kubeStateMetrics,
		kubeStateMetricsServiceAccount,
		"registry.k8s.io/kube-state-metrics/kube-state-metrics:v2.19.1@sha256:"+
			"7661da8c99b733d43117e4cba12bd9865d335e5777191d0af3d789807aded9f4",
		"25m",
		"32Mi",
		"100m",
		"64Mi",
		false,
	)
	if kubeStateMetrics.Spec.Strategy.Type != appsv1.RecreateDeploymentStrategyType ||
		kubeStateMetrics.Spec.Strategy.RollingUpdate != nil {
		t.Errorf("kube-state-metrics rollout contract = %#v", kubeStateMetrics.Spec.Strategy)
	}
	wantKSMArgs := []string{
		"--port=8080",
		"--resources=daemonsets,deployments,namespaces,nodes,persistentvolumeclaims,pods,replicasets,statefulsets",
	}
	if !reflect.DeepEqual(kubeStateMetrics.Spec.Template.Spec.Containers[0].Args, wantKSMArgs) {
		t.Errorf("kube-state-metrics arguments = %#v", kubeStateMetrics.Spec.Template.Spec.Containers[0].Args)
	}
	assertNoHostStorage(t, kubeStateMetrics.Spec.Template.Spec)

	requestCPU := prometheusContainer.Resources.Requests.Cpu().MilliValue() +
		kubeStateMetrics.Spec.Template.Spec.Containers[0].Resources.Requests.Cpu().MilliValue()
	limitCPU := prometheusContainer.Resources.Limits.Cpu().MilliValue() +
		kubeStateMetrics.Spec.Template.Spec.Containers[0].Resources.Limits.Cpu().MilliValue()
	requestMemory := prometheusContainer.Resources.Requests.Memory().Value() +
		kubeStateMetrics.Spec.Template.Spec.Containers[0].Resources.Requests.Memory().Value()
	limitMemory := prometheusContainer.Resources.Limits.Memory().Value() +
		kubeStateMetrics.Spec.Template.Spec.Containers[0].Resources.Limits.Memory().Value()
	if requestCPU != 125 || limitCPU != 500 ||
		requestMemory != 288*1024*1024 || limitMemory != 576*1024*1024 {
		t.Errorf(
			"H8.4 candidate budget = %dm/%d bytes requests, %dm/%d bytes limits",
			requestCPU,
			requestMemory,
			limitCPU,
			limitMemory,
		)
	}

	pvc := &corev1.PersistentVolumeClaim{}
	convertResource(t, resources, namespacedKey(persistentVolumeClaimKind, prometheusDeploymentName), pvc)
	if pvc.Spec.StorageClassName == nil || *pvc.Spec.StorageClassName != "local-path" ||
		!reflect.DeepEqual(pvc.Spec.AccessModes, []corev1.PersistentVolumeAccessMode{
			corev1.ReadWriteOnce,
		}) ||
		pvc.Spec.Resources.Requests.Storage().String() != "3Gi" {
		t.Errorf("Prometheus PVC = %#v", pvc.Spec)
	}

	assertCandidateService(t, resources, prometheusDeploymentName)
	assertCandidateService(t, resources, kubeStateMetricsDeploymentName)
}

func TestH84CCandidateIdentityAndConfigurationSurface(t *testing.T) {
	rendered, resources := renderH84BCandidate(t)

	for _, name := range []string{prometheusServiceAccount, kubeStateMetricsServiceAccount} {
		serviceAccount := &corev1.ServiceAccount{}
		convertResource(t, resources, namespacedKey(serviceAccountKind, name), serviceAccount)
		if serviceAccount.AutomountServiceAccountToken == nil ||
			!*serviceAccount.AutomountServiceAccountToken {
			t.Errorf("ServiceAccount %q does not explicitly automount its API token", name)
		}
		if len(serviceAccount.Secrets) != 0 || len(serviceAccount.ImagePullSecrets) != 0 {
			t.Errorf("ServiceAccount %q references Secrets: %#v", name, serviceAccount)
		}
	}

	configMap := &corev1.ConfigMap{}
	convertResource(t, resources, namespacedKey(configMapKind, prometheusDeploymentName), configMap)
	for _, absentRuleFile := range []string{
		"alerting_rules.yml",
		"alerts",
		"recording_rules.yml",
		"rules",
	} {
		if _, exists := configMap.Data[absentRuleFile]; exists {
			t.Errorf("Prometheus rule file %q is rendered", absentRuleFile)
		}
	}
	configuration := decodeYAML[map[string]any](t, []byte(configMap.Data["prometheus.yml"]))
	assertMapKeys(t, configuration, []string{"global", "rule_files", "scrape_configs"})
	ruleFiles, found, err := unstructured.NestedSlice(configuration, "rule_files")
	if err != nil || !found || len(ruleFiles) != 0 {
		t.Errorf("Prometheus rule_files = %#v, found=%t, error=%v", ruleFiles, found, err)
	}
	scrapeConfigs, found, err := unstructured.NestedSlice(configuration, "scrape_configs")
	if err != nil || !found || len(scrapeConfigs) != 6 {
		t.Fatalf("scrape_configs = %#v, found=%t, error=%v", scrapeConfigs, found, err)
	}
	for _, forbidden := range []string{
		"remote_write",
		"remote_read",
		"h8-4b-inert-no-targets",
		"job_name: kubelet",
		"job_name: cadvisor",
		"10250",
		"labelmap",
	} {
		if strings.Contains(configMap.Data["prometheus.yml"], forbidden) {
			t.Errorf("inert Prometheus configuration contains %q", forbidden)
		}
	}
	if bytes.Contains(rendered, []byte("--web.enable-lifecycle")) {
		t.Error("Prometheus lifecycle mutation endpoint is enabled")
	}
}

func TestH84BCandidateRBACIsMinimalAndSeparated(t *testing.T) {
	_, resources := renderH84BCandidate(t)

	prometheus := &rbacv1.ClusterRole{}
	convertResource(t, resources, objectKey{Kind: clusterRoleKind, Name: prometheusDiscoveryRole}, prometheus)
	wantPrometheusRules := []rbacv1.PolicyRule{
		{
			APIGroups: []string{""},
			Resources: []string{"pods", "services"},
			Verbs:     []string{getVerb, listVerb, watchVerb},
		},
		{
			APIGroups: []string{"discovery.k8s.io"},
			Resources: []string{"endpointslices"},
			Verbs:     []string{getVerb, listVerb, watchVerb},
		},
	}
	if !reflect.DeepEqual(prometheus.Rules, wantPrometheusRules) {
		t.Errorf("Prometheus discovery RBAC = %#v, want %#v", prometheus.Rules, wantPrometheusRules)
	}

	kubeStateMetrics := &rbacv1.ClusterRole{}
	convertResource(
		t,
		resources,
		objectKey{Kind: clusterRoleKind, Name: kubeStateMetricsDeploymentName},
		kubeStateMetrics,
	)
	wantKSMRules := []rbacv1.PolicyRule{
		{
			APIGroups: []string{""},
			Resources: []string{"namespaces", "nodes", "persistentvolumeclaims", "pods"},
			Verbs:     []string{listVerb, watchVerb},
		},
		{
			APIGroups: []string{"apps"},
			Resources: []string{"daemonsets", "deployments", "replicasets", "statefulsets"},
			Verbs:     []string{listVerb, watchVerb},
		},
	}
	if !reflect.DeepEqual(kubeStateMetrics.Rules, wantKSMRules) {
		t.Errorf("kube-state-metrics RBAC = %#v, want %#v", kubeStateMetrics.Rules, wantKSMRules)
	}
	assertSafeReadOnlyRules(t, append(slices.Clone(prometheus.Rules), kubeStateMetrics.Rules...))

	assertBinding(
		t,
		resources,
		prometheusDiscoveryRole,
		prometheusDiscoveryRole,
		prometheusServiceAccount,
	)
	assertBinding(
		t,
		resources,
		prometheusOperatorBinding,
		operatorMetricsReaderRole,
		prometheusServiceAccount,
	)
	assertBinding(
		t,
		resources,
		kubeStateMetricsDeploymentName,
		kubeStateMetricsDeploymentName,
		kubeStateMetricsServiceAccount,
	)
}

func TestH84BCandidateNetworkPoliciesUseOnlyReviewedBoundaries(t *testing.T) {
	_, resources := renderH84BCandidate(t)

	prometheusSelector := metav1.LabelSelector{MatchLabels: map[string]string{
		"app.kubernetes.io/name":      "prometheus",
		"app.kubernetes.io/instance":  "observability",
		"app.kubernetes.io/component": serverValue,
	}}
	kubeStateMetricsSelector := metav1.LabelSelector{MatchLabels: map[string]string{
		"app.kubernetes.io/name":      "kube-state-metrics",
		"app.kubernetes.io/instance":  "observability",
		"app.kubernetes.io/component": metricsValue,
	}}
	assertDNSPolicy(t, resources, "prometheus-dns-egress", prometheusSelector)
	assertDNSPolicy(t, resources, "kube-state-metrics-dns-egress", kubeStateMetricsSelector)
	assertAPIPolicy(t, resources, "prometheus-kubernetes-api-egress", prometheusSelector)
	assertAPIPolicy(t, resources, "kube-state-metrics-kubernetes-api-egress", kubeStateMetricsSelector)

	ingress := &networkingv1.NetworkPolicy{}
	convertResource(
		t,
		resources,
		namespacedKey(networkPolicyKind, "kube-state-metrics-metrics-ingress"),
		ingress,
	)
	wantIngress := networkingv1.NetworkPolicySpec{
		PodSelector: kubeStateMetricsSelector,
		PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
		Ingress: []networkingv1.NetworkPolicyIngressRule{{
			From: []networkingv1.NetworkPolicyPeer{{PodSelector: &prometheusSelector}},
			Ports: []networkingv1.NetworkPolicyPort{{
				Protocol: ptr.To(corev1.ProtocolTCP),
				Port:     ptr.To(intstr.FromInt32(8080)),
			}},
		}},
	}
	if !reflect.DeepEqual(ingress.Spec, wantIngress) {
		t.Errorf("kube-state-metrics ingress policy = %#v, want %#v", ingress.Spec, wantIngress)
	}

	for key, object := range resources {
		if key.Kind != networkPolicyKind {
			continue
		}
		policy := &networkingv1.NetworkPolicy{}
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(object.Object, policy); err != nil {
			t.Fatalf("convert NetworkPolicy %#v: %v", key, err)
		}
		for _, rule := range policy.Spec.Egress {
			if len(rule.To) == 0 {
				t.Errorf("NetworkPolicy %#v contains unrestricted egress", key)
			}
			for _, peer := range rule.To {
				if peer.IPBlock == nil {
					continue
				}
				if peer.IPBlock.CIDR != kubernetesAPICIDR || len(peer.IPBlock.Except) != 0 {
					t.Errorf("NetworkPolicy %#v contains unreviewed CIDR %#v", key, peer.IPBlock)
				}
			}
			for _, port := range rule.Ports {
				if port.EndPort != nil {
					t.Errorf("NetworkPolicy %#v contains a port range", key)
				}
			}
		}
	}
}

func renderH84BCandidate(
	t *testing.T,
) ([]byte, map[objectKey]*unstructured.Unstructured) {
	t.Helper()

	repositoryRoot := filepath.Join("..", "..")
	command := exec.Command(
		filepath.Join(repositoryRoot, "bin", "helm"),
		"template",
		"observability",
		filepath.Join(repositoryRoot, "deploy", "observability"),
		"--namespace",
		observabilityNamespace,
		"--values",
		filepath.Join(repositoryRoot, "deploy", "observability", candidateValuesFile),
	)
	rendered, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("render H8.4B candidate: %v\n%s", err, rendered)
	}

	resources := make(map[objectKey]*unstructured.Unstructured)
	decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(rendered), 4096)
	for {
		object := &unstructured.Unstructured{}
		decodeErr := decoder.Decode(object)
		if errors.Is(decodeErr, io.EOF) {
			break
		}
		if decodeErr != nil {
			t.Fatalf("decode H8.4B candidate resource: %v", decodeErr)
		}
		if object.GetKind() == "" {
			continue
		}
		key := objectKey{
			Kind:      object.GetKind(),
			Namespace: object.GetNamespace(),
			Name:      object.GetName(),
		}
		if _, exists := resources[key]; exists {
			t.Fatalf("duplicate H8.4B candidate resource %#v", key)
		}
		resources[key] = object
	}

	return rendered, resources
}

func namespacedKey(kind, name string) objectKey {
	return objectKey{Kind: kind, Namespace: observabilityNamespace, Name: name}
}

func assertCandidateDeployment(
	t *testing.T,
	deployment *appsv1.Deployment,
	serviceAccountName,
	image,
	requestCPU,
	requestMemory,
	limitCPU,
	limitMemory string,
	wantFSGroup bool,
) {
	t.Helper()

	if deployment.Spec.Replicas == nil || *deployment.Spec.Replicas != 1 {
		t.Errorf("%s replicas = %#v", deployment.Name, deployment.Spec.Replicas)
	}
	pod := deployment.Spec.Template.Spec
	if pod.ServiceAccountName != serviceAccountName ||
		pod.AutomountServiceAccountToken == nil ||
		!*pod.AutomountServiceAccountToken {
		t.Errorf("%s Pod identity = %#v", deployment.Name, pod)
	}
	if pod.HostNetwork || pod.HostPID || pod.HostIPC || len(pod.ImagePullSecrets) != 0 {
		t.Errorf("%s Pod host/external identity configuration = %#v", deployment.Name, pod)
	}
	assertCandidatePodSecurity(t, deployment.Name, pod.SecurityContext, wantFSGroup)
	if len(pod.Containers) != 1 || len(pod.InitContainers) != 0 {
		t.Fatalf(
			"%s containers = %d, initContainers = %d",
			deployment.Name,
			len(pod.Containers),
			len(pod.InitContainers),
		)
	}
	assertCandidateContainer(
		t,
		deployment.Name,
		pod.Containers[0],
		image,
		requestCPU,
		requestMemory,
		limitCPU,
		limitMemory,
	)
}

func assertCandidatePodSecurity(
	t *testing.T,
	name string,
	securityContext *corev1.PodSecurityContext,
	wantFSGroup bool,
) {
	t.Helper()

	podSecurityIsInvalid := securityContext == nil ||
		securityContext.RunAsNonRoot == nil ||
		!*securityContext.RunAsNonRoot ||
		securityContext.RunAsUser == nil ||
		*securityContext.RunAsUser != 65534 ||
		securityContext.RunAsGroup == nil ||
		*securityContext.RunAsGroup != 65534 ||
		securityContext.SeccompProfile == nil ||
		securityContext.SeccompProfile.Type != corev1.SeccompProfileTypeRuntimeDefault
	if podSecurityIsInvalid {
		t.Errorf("%s Pod security context = %#v", name, securityContext)
		return
	}
	if wantFSGroup {
		if securityContext.FSGroup == nil || *securityContext.FSGroup != 65534 ||
			securityContext.FSGroupChangePolicy == nil ||
			*securityContext.FSGroupChangePolicy != corev1.FSGroupChangeOnRootMismatch {
			t.Errorf("%s PVC ownership context = %#v", name, securityContext)
		}
	} else if securityContext.FSGroup != nil {
		t.Errorf("%s has unnecessary fsGroup %#v", name, securityContext.FSGroup)
	}
}

func assertCandidateContainer(
	t *testing.T,
	name string,
	container corev1.Container,
	image,
	requestCPU,
	requestMemory,
	limitCPU,
	limitMemory string,
) {
	t.Helper()

	if container.Image != image || strings.Contains(container.Image, ":latest") ||
		!strings.Contains(container.Image, "@sha256:") {
		t.Errorf("%s image = %q", name, container.Image)
	}
	assertCandidateContainerSecurity(t, name, container.SecurityContext)
	resources := container.Resources
	if resources.Requests.Cpu().String() != requestCPU ||
		resources.Requests.Memory().String() != requestMemory ||
		resources.Limits.Cpu().String() != limitCPU ||
		resources.Limits.Memory().String() != limitMemory {
		t.Errorf("%s resources = %#v", name, resources)
	}
	for _, port := range container.Ports {
		if port.HostPort != 0 {
			t.Errorf("%s container exposes hostPort %#v", name, port)
		}
	}
}

func assertCandidateContainerSecurity(
	t *testing.T,
	name string,
	security *corev1.SecurityContext,
) {
	t.Helper()

	containerSecurityIsInvalid := security == nil ||
		security.AllowPrivilegeEscalation == nil ||
		*security.AllowPrivilegeEscalation ||
		security.Privileged == nil ||
		*security.Privileged ||
		security.ReadOnlyRootFilesystem == nil ||
		!*security.ReadOnlyRootFilesystem ||
		security.RunAsNonRoot == nil ||
		!*security.RunAsNonRoot ||
		security.RunAsUser == nil ||
		*security.RunAsUser != 65534 ||
		security.RunAsGroup == nil ||
		*security.RunAsGroup != 65534 ||
		security.Capabilities == nil ||
		!reflect.DeepEqual(security.Capabilities.Drop, []corev1.Capability{"ALL"}) ||
		len(security.Capabilities.Add) != 0
	if containerSecurityIsInvalid {
		t.Errorf("%s container security context = %#v", name, security)
	}
}

func assertNoHostStorage(t *testing.T, pod corev1.PodSpec) {
	t.Helper()

	for _, volume := range pod.Volumes {
		if volume.HostPath != nil {
			t.Errorf("Pod contains hostPath volume %#v", volume)
		}
		if volume.Secret != nil || volume.Projected != nil {
			t.Errorf("Pod contains manually configured credential volume %#v", volume)
		}
	}
}

func assertCandidateService(
	t *testing.T,
	resources map[objectKey]*unstructured.Unstructured,
	name string,
) {
	t.Helper()

	service := &corev1.Service{}
	convertResource(t, resources, namespacedKey(serviceKind, name), service)
	if service.Spec.Type != corev1.ServiceTypeClusterIP ||
		len(service.Spec.ExternalIPs) != 0 ||
		service.Spec.LoadBalancerIP != "" ||
		len(service.Spec.LoadBalancerSourceRanges) != 0 ||
		service.Spec.ExternalName != "" ||
		service.Spec.HealthCheckNodePort != 0 {
		t.Errorf("%s Service exposure = %#v", name, service.Spec)
	}
	for _, port := range service.Spec.Ports {
		if port.NodePort != 0 {
			t.Errorf("%s Service exposes NodePort %#v", name, port)
		}
	}
}

func assertSafeReadOnlyRules(t *testing.T, rules []rbacv1.PolicyRule) {
	t.Helper()

	for _, rule := range rules {
		for _, value := range append(
			append(slices.Clone(rule.APIGroups), rule.Resources...),
			rule.Verbs...,
		) {
			if value == "*" {
				t.Errorf("RBAC rule contains wildcard: %#v", rule)
			}
		}
		if len(rule.NonResourceURLs) != 0 || len(rule.ResourceNames) != 0 {
			t.Errorf("discovery RBAC contains unexpected URL/name scope: %#v", rule)
		}
		for _, resourceName := range rule.Resources {
			if resourceName == "secrets" ||
				strings.Contains(resourceName, "exec") ||
				strings.Contains(resourceName, "attach") ||
				strings.Contains(resourceName, "portforward") ||
				strings.Contains(resourceName, "proxy") && resourceName != "nodes/proxy" {
				t.Errorf("RBAC rule contains forbidden resource %q", resourceName)
			}
		}
		for _, verb := range rule.Verbs {
			if !slices.Contains([]string{getVerb, listVerb, watchVerb}, verb) {
				t.Errorf("RBAC rule contains mutating or elevated verb %q", verb)
			}
		}
	}
}

func assertBinding(
	t *testing.T,
	resources map[objectKey]*unstructured.Unstructured,
	name,
	role,
	serviceAccount string,
) {
	t.Helper()

	binding := &rbacv1.ClusterRoleBinding{}
	convertResource(t, resources, objectKey{Kind: clusterRoleBindingKind, Name: name}, binding)
	wantRoleRef := rbacv1.RoleRef{
		APIGroup: rbacv1.GroupName,
		Kind:     clusterRoleKind,
		Name:     role,
	}
	wantSubjects := []rbacv1.Subject{{
		Kind:      serviceAccountKind,
		Name:      serviceAccount,
		Namespace: observabilityNamespace,
	}}
	if !reflect.DeepEqual(binding.RoleRef, wantRoleRef) ||
		!reflect.DeepEqual(binding.Subjects, wantSubjects) {
		t.Errorf("ClusterRoleBinding %q = %#v", name, binding)
	}
}

func assertDNSPolicy(
	t *testing.T,
	resources map[objectKey]*unstructured.Unstructured,
	name string,
	selector metav1.LabelSelector,
) {
	t.Helper()

	policy := &networkingv1.NetworkPolicy{}
	convertResource(t, resources, namespacedKey(networkPolicyKind, name), policy)
	want := networkingv1.NetworkPolicySpec{
		PodSelector: selector,
		PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeEgress},
		Egress: []networkingv1.NetworkPolicyEgressRule{{
			To: []networkingv1.NetworkPolicyPeer{{
				NamespaceSelector: namespaceNameSelector("kube-system"),
				PodSelector: labelSelector(map[string]string{
					"k8s-app": "kube-dns",
				}),
			}},
			Ports: []networkingv1.NetworkPolicyPort{
				{
					Protocol: ptr.To(corev1.ProtocolUDP),
					Port:     ptr.To(intstr.FromInt32(53)),
				},
				{
					Protocol: ptr.To(corev1.ProtocolTCP),
					Port:     ptr.To(intstr.FromInt32(53)),
				},
			},
		}},
	}
	if !reflect.DeepEqual(policy.Spec, want) {
		t.Errorf("DNS NetworkPolicy %q = %#v, want %#v", name, policy.Spec, want)
	}
}

func assertAPIPolicy(
	t *testing.T,
	resources map[objectKey]*unstructured.Unstructured,
	name string,
	selector metav1.LabelSelector,
) {
	t.Helper()

	policy := &networkingv1.NetworkPolicy{}
	convertResource(t, resources, namespacedKey(networkPolicyKind, name), policy)
	want := networkingv1.NetworkPolicySpec{
		PodSelector: selector,
		PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeEgress},
		Egress: []networkingv1.NetworkPolicyEgressRule{{
			To: []networkingv1.NetworkPolicyPeer{{
				IPBlock: &networkingv1.IPBlock{CIDR: kubernetesAPICIDR},
			}},
			Ports: []networkingv1.NetworkPolicyPort{{
				Protocol: ptr.To(corev1.ProtocolTCP),
				Port:     ptr.To(intstr.FromInt32(6443)),
			}},
		}},
	}
	if !reflect.DeepEqual(policy.Spec, want) {
		t.Errorf("API NetworkPolicy %q = %#v, want %#v", name, policy.Spec, want)
	}
}
