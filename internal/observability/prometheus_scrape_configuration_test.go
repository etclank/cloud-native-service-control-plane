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
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
)

const standardServiceAccountTokenPath = "/var/run/secrets/kubernetes.io/serviceaccount/token"

type expectedEndpointSliceJob struct {
	namespace string
	keeps     []expectedKeepRule
}

type expectedKeepRule struct {
	sourceLabels []string
	regex        string
}

func TestH84CCandidateScrapeConfigurationMatchesTargetMatrix(t *testing.T) {
	matrix, _ := readPrometheusTargetMatrix(t)
	rendered, resources := renderH84BCandidate(t)
	configuration := candidatePrometheusConfiguration(t, resources)

	global, found, err := unstructured.NestedMap(configuration, "global")
	if err != nil || !found {
		t.Fatalf("Prometheus global configuration: found=%t error=%v", found, err)
	}
	assertValue(t, global, "scrape_interval", matrix.Global.ScrapeInterval)
	assertValue(t, global, "evaluation_interval", matrix.Global.EvaluationInterval)
	assertValue(t, global, "scrape_timeout", defaultScrapeTimeout)

	scrapeConfigs, found, err := unstructured.NestedSlice(configuration, "scrape_configs")
	if err != nil || !found || len(scrapeConfigs) != 6 {
		t.Fatalf("scrape_configs = %#v, found=%t, error=%v", scrapeConfigs, found, err)
	}
	jobs := make(map[string]map[string]any, len(scrapeConfigs))
	for _, rawJob := range scrapeConfigs {
		job, ok := rawJob.(map[string]any)
		if !ok {
			t.Fatalf("scrape job has type %T", rawJob)
		}
		name, _, _ := unstructured.NestedString(job, "job_name")
		if _, duplicate := jobs[name]; duplicate {
			t.Errorf("duplicate scrape job %q", name)
		}
		jobs[name] = job
	}
	wantJobs := acceptedMatrixJobs(matrix)
	if !reflect.DeepEqual(sortedMapKeys(jobs), sortedMapKeys(wantJobs)) {
		t.Errorf("scrape jobs = %#v, want %#v", sortedMapKeys(jobs), sortedMapKeys(wantJobs))
	}

	for name, matrixJob := range wantJobs {
		job := jobs[name]
		assertValue(t, job, "scheme", matrixJob.Scheme)
		assertValue(t, job, "metrics_path", matrixJob.Path)
		assertValue(t, job, "scrape_interval", matrixJob.Interval)
		assertValue(t, job, "scrape_timeout", matrixJob.Timeout)
		interval, intervalErr := time.ParseDuration(matrixJob.Interval)
		timeout, timeoutErr := time.ParseDuration(matrixJob.Timeout)
		if intervalErr != nil || timeoutErr != nil || timeout >= interval {
			t.Errorf("job %q has invalid timing %q/%q", name, matrixJob.Interval, matrixJob.Timeout)
		}
	}

	self := jobs[prometheusSelfTarget]
	staticConfigs, found, err := unstructured.NestedSlice(self, "static_configs")
	if err != nil || !found || len(staticConfigs) != 1 {
		t.Fatalf("Prometheus self static_configs = %#v", staticConfigs)
	}
	staticConfig := staticConfigs[0].(map[string]any)
	if !reflect.DeepEqual(staticConfig["targets"], []any{"127.0.0.1:9090"}) {
		t.Errorf("Prometheus self targets = %#v", staticConfig["targets"])
	}
	if _, found := self["kubernetes_sd_configs"]; found {
		t.Error("Prometheus self uses Kubernetes discovery")
	}

	assertEndpointSliceJobs(t, jobs)
	assertOperatorAuthentication(t, jobs)

	ruleFiles, found, err := unstructured.NestedSlice(configuration, "rule_files")
	if err != nil || !found || len(ruleFiles) != 0 {
		t.Errorf("rule_files = %#v, found=%t, error=%v", ruleFiles, found, err)
	}
	for _, forbidden := range []string{
		"h8-4b-inert-no-targets",
		"job_name: kubelet",
		"job_name: cadvisor",
		"10250",
		"labelmap",
		"remote_write",
		"remote_read",
		"alerting:",
		"federate",
		"prometheus.io/scrape",
	} {
		if strings.Contains(string(rendered), forbidden) {
			t.Errorf("H8.4C candidate contains forbidden configuration %q", forbidden)
		}
	}
}

func TestH84CCandidateMetricsExposureAndTargetPolicies(t *testing.T) {
	rendered, resources := renderH84BCandidate(t)
	if strings.Contains(string(rendered), "10250") {
		t.Error("H8.4C candidate contains deferred kubelet/cAdvisor port 10250")
	}

	collector := &appsv1.DaemonSet{}
	convertResource(
		t,
		resources,
		namespacedKey(daemonSetKind, collectorResourceName),
		collector,
	)
	collectorPorts := collector.Spec.Template.Spec.Containers[0].Ports
	wantCollectorPorts := []corev1.ContainerPort{
		{Name: metricsValue, ContainerPort: 8888, Protocol: corev1.ProtocolTCP},
		{Name: "otlp", ContainerPort: 4317, Protocol: corev1.ProtocolTCP},
		{Name: otlpHTTPValue, ContainerPort: 4318, Protocol: corev1.ProtocolTCP},
	}
	if !reflect.DeepEqual(collectorPorts, wantCollectorPorts) {
		t.Errorf("Collector container ports = %#v, want %#v", collectorPorts, wantCollectorPorts)
	}
	collectorService := &corev1.Service{}
	convertResource(
		t,
		resources,
		namespacedKey(serviceKind, collectorChartName),
		collectorService,
	)
	wantServicePorts := []corev1.ServicePort{
		{Name: "otlp", Protocol: corev1.ProtocolTCP, Port: 4317, TargetPort: intstr.FromString("otlp")},
		{Name: otlpHTTPValue, Protocol: corev1.ProtocolTCP, Port: 4318, TargetPort: intstr.FromString(otlpHTTPValue)},
		{Name: metricsValue, Protocol: corev1.ProtocolTCP, Port: 8888, TargetPort: intstr.FromString(metricsValue)},
	}
	if !reflect.DeepEqual(collectorService.Spec.Ports, wantServicePorts) {
		t.Errorf("Collector Service ports = %#v, want %#v", collectorService.Spec.Ports, wantServicePorts)
	}
	collectorConfigMap := &corev1.ConfigMap{}
	convertResource(
		t,
		resources,
		namespacedKey(configMapKind, collectorResourceName),
		collectorConfigMap,
	)
	collectorConfig := decodeYAML[map[string]any](t, []byte(collectorConfigMap.Data["relay"]))
	metricsAddress, found, err := unstructured.NestedString(
		collectorConfig,
		"service",
		"telemetry",
		"metrics",
		"address",
	)
	if err != nil || !found || metricsAddress != "${env:MY_POD_IP}:8888" {
		t.Errorf(
			"Collector telemetry metrics address = %q, found=%t, error=%v",
			metricsAddress,
			found,
			err,
		)
	}

	prometheusSelector := map[string]string{
		applicationNameLabel:      prometheusChartName,
		applicationInstanceLabel:  observabilityNamespace,
		applicationComponentLabel: serverValue,
	}
	cases := []struct {
		name                 string
		destinationNamespace string
		destinationSelector  map[string]string
		port                 int32
	}{
		{
			"prometheus-kube-state-metrics-egress",
			"",
			map[string]string{
				applicationNameLabel:      kubeStateMetricsChartName,
				applicationInstanceLabel:  observabilityNamespace,
				applicationComponentLabel: metricsValue,
			},
			8080,
		},
		{
			"prometheus-platform-operator-metrics-egress",
			platformSystemNamespace,
			map[string]string{
				applicationNameLabel: "cloud-native-service-control-plane",
				"control-plane":      "controller-manager",
			},
			8443,
		},
		{
			"prometheus-opentelemetry-collector-egress",
			"",
			map[string]string{
				applicationNameLabel:     collectorChartName,
				applicationInstanceLabel: observabilityNamespace,
				"component":              "agent-collector",
			},
			8888,
		},
		{
			"prometheus-control-plane-api-metrics-egress",
			platformSystemNamespace,
			map[string]string{
				applicationNameLabel:      controlPlaneAPITarget,
				applicationComponentLabel: "api",
			},
			9090,
		},
		{
			"prometheus-managed-demo-metrics-egress",
			"applications",
			map[string]string{
				applicationNameLabel:                "managed-service",
				"app.kubernetes.io/managed-by":      platformOperatorTarget,
				"platform.eoghanclancy.eu/template": "demo-http",
			},
			9090,
		},
	}
	for _, testCase := range cases {
		assertExactPrometheusEgressPolicy(
			t,
			resources,
			testCase.name,
			prometheusSelector,
			testCase.destinationNamespace,
			testCase.destinationSelector,
			testCase.port,
		)
	}
	assertExactCollectorMetricsIngress(t, resources, prometheusSelector)
}

func acceptedMatrixJobs(matrix prometheusTargetMatrix) map[string]prometheusTargetDesign {
	jobs := make(map[string]prometheusTargetDesign)
	for _, job := range matrix.Jobs {
		if job.Status == acceptedTargetStatus {
			jobs[job.ID] = job
		}
	}
	return jobs
}

func candidatePrometheusConfiguration(
	t *testing.T,
	resources map[objectKey]*unstructured.Unstructured,
) map[string]any {
	t.Helper()

	configMap := &corev1.ConfigMap{}
	convertResource(
		t,
		resources,
		namespacedKey(configMapKind, prometheusDeploymentName),
		configMap,
	)
	return decodeYAML[map[string]any](t, []byte(configMap.Data["prometheus.yml"]))
}

func assertEndpointSliceJobs(t *testing.T, jobs map[string]map[string]any) {
	t.Helper()

	expected := map[string]expectedEndpointSliceJob{
		kubeStateMetricsChartName: {
			namespace: observabilityNamespace,
			keeps: []expectedKeepRule{
				{[]string{metaKubernetesNamespace}, "^observability$"},
				{[]string{metaKubernetesServiceName}, "^observability-kube-state-metrics$"},
				{[]string{
					metaKubernetesServiceAppName,
					"__meta_kubernetes_service_label_app_kubernetes_io_instance",
					"__meta_kubernetes_service_label_app_kubernetes_io_component",
				}, "^kube-state-metrics;observability;metrics$"},
				{[]string{metaKubernetesEndpointPort}, "^http$"},
				{[]string{metaKubernetesEndpointReady}, readyEndpointRegex},
			},
		},
		platformOperatorTarget: {
			namespace: platformSystemNamespace,
			keeps: []expectedKeepRule{
				{[]string{metaKubernetesNamespace}, "^platform-system$"},
				{[]string{metaKubernetesServiceName}, "^platform-operator-controller-manager-metrics-service$"},
				{[]string{
					metaKubernetesServiceAppName,
					"__meta_kubernetes_service_label_control_plane",
				}, "^cloud-native-service-control-plane;controller-manager$"},
				{[]string{
					metaKubernetesPodAppName,
					"__meta_kubernetes_pod_label_control_plane",
				}, "^cloud-native-service-control-plane;controller-manager$"},
				{[]string{metaKubernetesEndpointPort}, "^https$"},
				{[]string{metaKubernetesEndpointReady}, readyEndpointRegex},
			},
		},
		collectorChartName: {
			namespace: observabilityNamespace,
			keeps: []expectedKeepRule{
				{[]string{metaKubernetesNamespace}, "^observability$"},
				{[]string{metaKubernetesServiceName}, "^opentelemetry-collector$"},
				{[]string{
					metaKubernetesServiceAppName,
					"__meta_kubernetes_service_label_app_kubernetes_io_part_of",
				}, "^opentelemetry-collector;cloud-native-service-control-plane$"},
				{[]string{
					metaKubernetesPodAppName,
					"__meta_kubernetes_pod_label_app_kubernetes_io_instance",
					"__meta_kubernetes_pod_label_component",
				}, "^opentelemetry-collector;observability;agent-collector$"},
				{[]string{metaKubernetesEndpointPort}, metricsPortRegex},
				{[]string{metaKubernetesEndpointReady}, readyEndpointRegex},
			},
		},
		controlPlaneAPITarget: {
			namespace: platformSystemNamespace,
			keeps: []expectedKeepRule{
				{[]string{metaKubernetesNamespace}, "^platform-system$"},
				{[]string{metaKubernetesServiceName}, "^control-plane-api$"},
				{[]string{
					metaKubernetesServiceAppName,
					"__meta_kubernetes_service_label_app_kubernetes_io_part_of",
				}, "^control-plane-api;cloud-native-service-control-plane$"},
				{[]string{
					metaKubernetesPodAppName,
					"__meta_kubernetes_pod_label_app_kubernetes_io_component",
				}, "^control-plane-api;api$"},
				{[]string{metaKubernetesEndpointPort}, metricsPortRegex},
				{[]string{metaKubernetesEndpointReady}, readyEndpointRegex},
			},
		},
		managedDemoTarget: {
			namespace: "applications",
			keeps: []expectedKeepRule{
				{[]string{metaKubernetesNamespace}, "^applications$"},
				{[]string{
					metaKubernetesServiceAppName,
					"__meta_kubernetes_service_label_app_kubernetes_io_managed_by",
					"__meta_kubernetes_service_label_platform_eoghanclancy_eu_template",
				}, "^managed-service;platform-operator;demo-http$"},
				{[]string{
					metaKubernetesPodAppName,
					"__meta_kubernetes_pod_label_app_kubernetes_io_managed_by",
					"__meta_kubernetes_pod_label_platform_eoghanclancy_eu_template",
				}, "^managed-service;platform-operator;demo-http$"},
				{[]string{metaKubernetesEndpointPort}, metricsPortRegex},
				{[]string{metaKubernetesEndpointReady}, readyEndpointRegex},
			},
		},
	}
	for name, want := range expected {
		job := jobs[name]
		discovery, found, err := unstructured.NestedSlice(job, "kubernetes_sd_configs")
		if err != nil || !found || len(discovery) != 1 {
			t.Fatalf("job %q discovery = %#v", name, discovery)
		}
		discoveryConfig := discovery[0].(map[string]any)
		assertValue(t, discoveryConfig, "role", "endpointslice")
		namespaces := discoveryConfig["namespaces"].(map[string]any)
		if !reflect.DeepEqual(namespaces["names"], []any{want.namespace}) {
			t.Errorf("job %q namespaces = %#v", name, namespaces)
		}

		relabelConfigs := job["relabel_configs"].([]any)
		keepRules := make([]expectedKeepRule, 0, len(want.keeps))
		targetLabels := make([]string, 0, 3)
		for _, rawRule := range relabelConfigs {
			rule := rawRule.(map[string]any)
			action, _, _ := unstructured.NestedString(rule, "action")
			switch action {
			case "keep":
				regex, _, _ := unstructured.NestedString(rule, "regex")
				if !strings.HasPrefix(regex, "^") || !strings.HasSuffix(regex, "$") {
					t.Errorf("job %q has unanchored keep regex %q", name, regex)
				}
				keepRules = append(keepRules, expectedKeepRule{
					sourceLabels: anyStrings(rule["source_labels"].([]any)),
					regex:        regex,
				})
			case "replace":
				targetLabel, _, _ := unstructured.NestedString(rule, "target_label")
				targetLabels = append(targetLabels, targetLabel)
			default:
				t.Errorf("job %q has forbidden relabel action %q", name, action)
			}
		}
		if !reflect.DeepEqual(keepRules, want.keeps) {
			t.Errorf("job %q keep rules = %#v, want %#v", name, keepRules, want.keeps)
		}
		if !reflect.DeepEqual(targetLabels, []string{namespaceField, serviceField, "pod"}) {
			t.Errorf("job %q retained labels = %#v", name, targetLabels)
		}
	}
}

func assertOperatorAuthentication(t *testing.T, jobs map[string]map[string]any) {
	t.Helper()

	for name, job := range jobs {
		tokenFile, hasTokenFile := job["bearer_token_file"]
		tlsConfig, hasTLSConfig := job["tls_config"]
		if name != platformOperatorTarget {
			if hasTokenFile || hasTLSConfig {
				t.Errorf("job %q has authentication/TLS configuration", name)
			}
			continue
		}
		if tokenFile != standardServiceAccountTokenPath {
			t.Errorf("operator token file = %#v", tokenFile)
		}
		wantTLS := map[string]any{"insecure_skip_verify": true}
		if !reflect.DeepEqual(tlsConfig, wantTLS) {
			t.Errorf("operator TLS configuration = %#v, want %#v", tlsConfig, wantTLS)
		}
	}
}

func assertExactPrometheusEgressPolicy(
	t *testing.T,
	resources map[objectKey]*unstructured.Unstructured,
	name string,
	prometheusSelector map[string]string,
	destinationNamespace string,
	destinationSelector map[string]string,
	port int32,
) {
	t.Helper()

	destination := networkingv1.NetworkPolicyPeer{
		PodSelector: &metav1.LabelSelector{MatchLabels: destinationSelector},
	}
	if destinationNamespace != "" {
		destination.NamespaceSelector = &metav1.LabelSelector{MatchLabels: map[string]string{
			"kubernetes.io/metadata.name": destinationNamespace,
		}}
	}
	want := networkingv1.NetworkPolicySpec{
		PodSelector: metav1.LabelSelector{MatchLabels: prometheusSelector},
		PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeEgress},
		Egress: []networkingv1.NetworkPolicyEgressRule{{
			To: []networkingv1.NetworkPolicyPeer{destination},
			Ports: []networkingv1.NetworkPolicyPort{{
				Protocol: ptr.To(corev1.ProtocolTCP),
				Port:     ptr.To(intstr.FromInt32(port)),
			}},
		}},
	}
	policy := &networkingv1.NetworkPolicy{}
	convertResource(t, resources, namespacedKey(networkPolicyKind, name), policy)
	if !reflect.DeepEqual(policy.Spec, want) {
		t.Errorf("target egress NetworkPolicy %q = %#v, want %#v", name, policy.Spec, want)
	}
}

func assertExactCollectorMetricsIngress(
	t *testing.T,
	resources map[objectKey]*unstructured.Unstructured,
	prometheusSelector map[string]string,
) {
	t.Helper()

	want := networkingv1.NetworkPolicySpec{
		PodSelector: metav1.LabelSelector{MatchLabels: map[string]string{
			applicationNameLabel:     collectorChartName,
			applicationInstanceLabel: observabilityNamespace,
			"component":              "agent-collector",
		}},
		PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
		Ingress: []networkingv1.NetworkPolicyIngressRule{{
			From: []networkingv1.NetworkPolicyPeer{{
				PodSelector: &metav1.LabelSelector{MatchLabels: prometheusSelector},
			}},
			Ports: []networkingv1.NetworkPolicyPort{{
				Protocol: ptr.To(corev1.ProtocolTCP),
				Port:     ptr.To(intstr.FromInt32(8888)),
			}},
		}},
	}
	policy := &networkingv1.NetworkPolicy{}
	convertResource(
		t,
		resources,
		namespacedKey(networkPolicyKind, "opentelemetry-collector-metrics-ingress"),
		policy,
	)
	if !reflect.DeepEqual(policy.Spec, want) {
		t.Errorf("Collector metrics ingress NetworkPolicy = %#v, want %#v", policy.Spec, want)
	}
}

func anyStrings(values []any) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, value.(string))
	}
	return result
}

func sortedMapKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}
