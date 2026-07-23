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
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/utils/ptr"
)

const (
	collectorChartName           = "opentelemetry-collector"
	collectorChartRepository     = "https://open-telemetry.github.io/opentelemetry-helm-charts"
	collectorChartVersion        = "0.165.0"
	collectorAppVersion          = "0.156.0"
	prometheusChartName          = "prometheus"
	prometheusChartVersion       = "29.18.0"
	prometheusAppVersion         = "v3.13.1"
	kubeStateMetricsChartName    = "kube-state-metrics"
	kubeStateMetricsChartVersion = "7.8.1"
	kubeStateMetricsAppVersion   = "2.19.1"
	prometheusChartRepository    = "https://prometheus-community.github.io/helm-charts"
	wrapperChartVersion          = "0.4.0"
	collectorChartSHA256         = "b592ea064d9b906930cac2d22b88eeb1bc82f12d5ed07fd20792de2c051ca3c5"
	prometheusChartSHA256        = "24f5f056dd5cb00e98ffb905c9c2779e810153f1b5a6306bf2cc2c5a4f02a0b9"
	kubeStateMetricsChartSHA256  = "b5a2436bd62226ff30a57b7237eaf2f99bac6be675484c4082bcd4312680de12"
	wrapperChartLockDigest       = "sha256:5aecd00ad60ab3a29591e852480595e8719821f6f45a9a6066ddefe428674197"
	collectorResourceName        = "opentelemetry-collector-agent"
	collectorOTLPReceiver        = "otlp"
	defaultDenyPolicyName        = "observability-default-deny"
	otlpIngressPolicyName        = "opentelemetry-collector-otlp-ingress"
	namespaceKind                = "Namespace"
	networkPolicyKind            = "NetworkPolicy"
	applicationNameLabel         = "app.kubernetes.io/name"
	otlpClientLabel              = "observability.eoghanclancy.eu/otlp-client"
	podSecurityVersion           = "v1.36"
	collectorImage               = "ghcr.io/open-telemetry/opentelemetry-collector-releases/" +
		"opentelemetry-collector-k8s@sha256:" +
		"aa4509d8d72195c8576227fb33932788bc368fb0e81f6520f332130139236b91"
	observabilityNamespace = "observability"
)

type chartMetadata struct {
	APIVersion   string            `json:"apiVersion"`
	Name         string            `json:"name"`
	Version      string            `json:"version"`
	AppVersion   string            `json:"appVersion"`
	Dependencies []chartDependency `json:"dependencies"`
}

type chartDependency struct {
	Name       string `json:"name"`
	Repository string `json:"repository"`
	Version    string `json:"version"`
	Condition  string `json:"condition,omitempty"`
}

type chartLock struct {
	Dependencies []chartDependency `json:"dependencies"`
	Digest       string            `json:"digest"`
}

type objectKey struct {
	Kind      string
	Namespace string
	Name      string
}

func TestObservabilityDependencySupplyChain(t *testing.T) {
	repositoryRoot := filepath.Join("..", "..")
	chartPath := filepath.Join(repositoryRoot, "deploy", "observability", "Chart.yaml")
	lockPath := filepath.Join(repositoryRoot, "deploy", "observability", "Chart.lock")

	chart := decodeYAMLFile[chartMetadata](t, chartPath)
	wantChartDependencies := []chartDependency{
		{
			Name:       collectorChartName,
			Repository: collectorChartRepository,
			Version:    collectorChartVersion,
		},
		{
			Name:       prometheusChartName,
			Repository: prometheusChartRepository,
			Version:    prometheusChartVersion,
			Condition:  "prometheus.enabled",
		},
		{
			Name:       kubeStateMetricsChartName,
			Repository: prometheusChartRepository,
			Version:    kubeStateMetricsChartVersion,
			Condition:  "kube-state-metrics.enabled",
		},
	}
	if chart.APIVersion != "v2" || chart.Name == "" || chart.Version != wrapperChartVersion ||
		!reflect.DeepEqual(chart.Dependencies, wantChartDependencies) {
		t.Errorf("wrapper Chart metadata = %#v", chart)
	}

	lockContents, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatalf("read Chart.lock: %v", err)
	}
	lock := decodeYAML[chartLock](t, lockContents)
	wantLockedDependencies := make([]chartDependency, len(wantChartDependencies))
	for index, dependency := range wantChartDependencies {
		dependency.Condition = ""
		wantLockedDependencies[index] = dependency
	}
	if !reflect.DeepEqual(lock.Dependencies, wantLockedDependencies) ||
		lock.Digest != wrapperChartLockDigest ||
		!regexp.MustCompile(`^sha256:[0-9a-f]{64}$`).MatchString(lock.Digest) {
		t.Errorf("dependency lock = %#v", lock)
	}

	helmPath := filepath.Join(repositoryRoot, "bin", "helm")
	command := exec.Command(
		helmPath,
		"dependency",
		"list",
		filepath.Join(repositoryRoot, "deploy", "observability"),
	)
	dependencyList, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("validate dependency lock: %v\n%s", err, dependencyList)
	}
	for _, dependency := range wantLockedDependencies {
		pattern := regexp.MustCompile(
			`(?m)^` + regexp.QuoteMeta(dependency.Name) + `\s+` +
				regexp.QuoteMeta(dependency.Version) + `\s+` +
				regexp.QuoteMeta(dependency.Repository) + `\s+ok\s*$`,
		)
		if !pattern.Match(dependencyList) {
			t.Errorf("dependency list lacks %#v: %q", dependency, dependencyList)
		}
	}

	archives := []struct {
		name       string
		version    string
		appVersion string
		sha256     string
	}{
		{collectorChartName, collectorChartVersion, collectorAppVersion, collectorChartSHA256},
		{prometheusChartName, prometheusChartVersion, prometheusAppVersion, prometheusChartSHA256},
		{
			kubeStateMetricsChartName,
			kubeStateMetricsChartVersion,
			kubeStateMetricsAppVersion,
			kubeStateMetricsChartSHA256,
		},
	}
	for _, expected := range archives {
		archivePath := filepath.Join(
			repositoryRoot,
			"deploy",
			"observability",
			"charts",
			expected.name+"-"+expected.version+".tgz",
		)
		archive, readErr := os.ReadFile(archivePath)
		if readErr != nil {
			t.Fatalf("read %s chart archive: %v", expected.name, readErr)
		}
		archiveDigest := sha256.Sum256(archive)
		if hex.EncodeToString(archiveDigest[:]) != expected.sha256 {
			t.Errorf("%s chart SHA-256 = %x", expected.name, archiveDigest)
		}

		showChart := exec.Command(helmPath, "show", "chart", archivePath)
		metadata, showErr := showChart.Output()
		if showErr != nil {
			t.Fatalf("read %s chart metadata: %v", expected.name, showErr)
		}
		dependencyChart := decodeYAML[chartMetadata](t, metadata)
		if dependencyChart.Name != expected.name ||
			dependencyChart.Version != expected.version ||
			dependencyChart.AppVersion != expected.appVersion {
			t.Errorf("%s chart metadata = %#v", expected.name, dependencyChart)
		}
	}
}

func TestPrometheusNestedDependencySupplyChain(t *testing.T) {
	repositoryRoot := filepath.Join("..", "..")
	archivePath := filepath.Join(
		repositoryRoot,
		"deploy",
		"observability",
		"charts",
		prometheusChartName+"-"+prometheusChartVersion+".tgz",
	)
	wantFiles := []string{
		"prometheus/Chart.yaml",
		"prometheus/Chart.lock",
		"prometheus/charts/alertmanager/Chart.yaml",
		"prometheus/charts/kube-state-metrics/Chart.yaml",
		"prometheus/charts/prometheus-node-exporter/Chart.yaml",
		"prometheus/charts/prometheus-pushgateway/Chart.yaml",
	}
	files := readTarGzipFiles(t, archivePath, wantFiles)

	parentChart := decodeYAML[chartMetadata](t, files["prometheus/Chart.yaml"])
	wantDeclared := []chartDependency{
		{
			Name:       "alertmanager",
			Repository: prometheusChartRepository,
			Version:    "1.40.*",
			Condition:  "alertmanager.enabled",
		},
		{
			Name:       kubeStateMetricsChartName,
			Repository: prometheusChartRepository,
			Version:    "7.8.*",
			Condition:  "kube-state-metrics.enabled",
		},
		{
			Name:       "prometheus-node-exporter",
			Repository: prometheusChartRepository,
			Version:    "4.56.*",
			Condition:  "prometheus-node-exporter.enabled",
		},
		{
			Name:       "prometheus-pushgateway",
			Repository: prometheusChartRepository,
			Version:    "3.7.*",
			Condition:  "prometheus-pushgateway.enabled",
		},
	}
	if !reflect.DeepEqual(parentChart.Dependencies, wantDeclared) {
		t.Errorf("Prometheus declared dependencies = %#v", parentChart.Dependencies)
	}

	wantLocked := []chartDependency{
		{Name: "alertmanager", Repository: prometheusChartRepository, Version: "1.40.3"},
		{
			Name:       kubeStateMetricsChartName,
			Repository: prometheusChartRepository,
			Version:    kubeStateMetricsChartVersion,
		},
		{
			Name:       "prometheus-node-exporter",
			Repository: prometheusChartRepository,
			Version:    "4.56.1",
		},
		{
			Name:       "prometheus-pushgateway",
			Repository: prometheusChartRepository,
			Version:    "3.7.0",
		},
	}
	nestedLock := decodeYAML[chartLock](t, files["prometheus/Chart.lock"])
	if !reflect.DeepEqual(nestedLock.Dependencies, wantLocked) ||
		nestedLock.Digest != "sha256:076d6886a3e8e27e69e66f8d6559c640788b3a441f18400fb95b3ffd6192f659" {
		t.Errorf("Prometheus nested dependency lock = %#v", nestedLock)
	}

	wantNestedCharts := map[string]chartMetadata{
		"prometheus/charts/alertmanager/Chart.yaml": {
			Name: "alertmanager", Version: "1.40.3", AppVersion: "v0.33.1",
		},
		"prometheus/charts/kube-state-metrics/Chart.yaml": {
			Name: kubeStateMetricsChartName, Version: kubeStateMetricsChartVersion,
			AppVersion: kubeStateMetricsAppVersion,
		},
		"prometheus/charts/prometheus-node-exporter/Chart.yaml": {
			Name: "prometheus-node-exporter", Version: "4.56.1", AppVersion: "1.12.1",
		},
		"prometheus/charts/prometheus-pushgateway/Chart.yaml": {
			Name: "prometheus-pushgateway", Version: "3.7.0", AppVersion: "v1.11.3",
		},
	}
	for path, want := range wantNestedCharts {
		got := decodeYAML[chartMetadata](t, files[path])
		if got.Name != want.Name || got.Version != want.Version || got.AppVersion != want.AppVersion {
			t.Errorf("nested chart %s metadata = %#v", path, got)
		}
	}
}

func TestPrometheusCandidatesRemainDisabledAndImmutable(t *testing.T) {
	repositoryRoot := filepath.Join("..", "..")
	valuesPath := filepath.Join(repositoryRoot, "deploy", "observability", "values.yaml")
	values := decodeYAMLFile[map[string]any](t, valuesPath)

	prometheus := nestedMap(t, values, prometheusChartName)
	assertBoolean(t, prometheus, "enabled", false)
	serverImage := nestedMap(t, nestedMap(t, prometheus, "server"), "image")
	assertValue(t, serverImage, "repository", "quay.io/prometheus/prometheus")
	if _, found := serverImage["tag"]; found {
		t.Error("Prometheus candidate sets a tag even though the chart renders its digest directly")
	}
	assertValue(
		t,
		serverImage,
		"digest",
		"sha256:bd2dcadfb0d1096e2a4c21817ac7af918e2f19ff628e4bf25fd67a924c13dd80",
	)
	assertBoolean(t, nestedMap(t, nestedMap(t, prometheus, "configmapReload"), "prometheus"), "enabled", false)
	for _, component := range []string{
		"alertmanager", "kube-state-metrics", "prometheus-node-exporter", "prometheus-pushgateway",
	} {
		assertBoolean(t, nestedMap(t, prometheus, component), "enabled", false)
	}

	kubeStateMetrics := nestedMap(t, values, kubeStateMetricsChartName)
	assertBoolean(t, kubeStateMetrics, "enabled", false)
	kubeStateMetricsImage := nestedMap(t, kubeStateMetrics, "image")
	assertValue(t, kubeStateMetricsImage, "registry", "registry.k8s.io")
	assertValue(t, kubeStateMetricsImage, "repository", "kube-state-metrics/kube-state-metrics")
	assertValue(t, kubeStateMetricsImage, "tag", "v"+kubeStateMetricsAppVersion)
	assertValue(
		t,
		kubeStateMetricsImage,
		"sha",
		"sha256:7661da8c99b733d43117e4cba12bd9865d335e5777191d0af3d789807aded9f4",
	)
	assertBoolean(t, nestedMap(t, kubeStateMetrics, "kubeRBACProxy"), "enabled", false)

	valuesContents, err := os.ReadFile(valuesPath)
	if err != nil {
		t.Fatalf("read candidate values: %v", err)
	}
	for _, mutable := range []string{
		"quay.io/prometheus/prometheus:latest",
		"registry.k8s.io/kube-state-metrics/kube-state-metrics:latest",
	} {
		if bytes.Contains(valuesContents, []byte(mutable)) {
			t.Errorf("candidate values contain mutable image %q", mutable)
		}
	}
}

func TestReviewedKubernetesAPIEgressDestination(t *testing.T) {
	values := decodeYAMLFile[map[string]any](
		t,
		filepath.Join("..", "..", "deploy", "observability", "values.yaml"),
	)
	destinations := nestedMap(t, values, "networkPolicyDestinations")
	kubernetesAPI := nestedMap(t, destinations, "kubernetesAPI")
	assertValue(t, kubernetesAPI, "cidr", "142.132.178.45/32")
	assertNumber(t, kubernetesAPI, "port", 6443)

	cidr, found, err := unstructured.NestedString(kubernetesAPI, "cidr")
	if err != nil || !found {
		t.Fatalf("read reviewed Kubernetes API CIDR: found=%t error=%v", found, err)
	}
	for _, forbidden := range []string{
		"10.43.0.0/16",
		"10.42.0.0/16",
		"142.132.178.0/24",
		"0.0.0.0/0",
	} {
		if cidr == forbidden {
			t.Errorf("reviewed Kubernetes API destination uses broad CIDR %q", cidr)
		}
	}
}

func TestPrometheusCandidateImagesRenderByDigest(t *testing.T) {
	repositoryRoot := filepath.Join("..", "..")
	command := exec.Command(
		filepath.Join(repositoryRoot, "bin", "helm"),
		"template",
		"observability",
		filepath.Join(repositoryRoot, "deploy", "observability"),
		"--namespace",
		observabilityNamespace,
		"--set",
		"prometheus.enabled=true",
		"--set",
		"kube-state-metrics.enabled=true",
	)
	rendered, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("render disabled Prometheus candidates: %v\n%s", err, rendered)
	}

	imagePattern := regexp.MustCompile(`(?m)^\s*image:\s*"?([^"\s]+)"?\s*$`)
	matches := imagePattern.FindAllSubmatch(rendered, -1)
	images := make([]string, 0, len(matches))
	for _, match := range matches {
		images = append(images, string(match[1]))
	}
	wantImages := []string{
		collectorImage,
		"registry.k8s.io/kube-state-metrics/kube-state-metrics:v2.19.1@sha256:" +
			"7661da8c99b733d43117e4cba12bd9865d335e5777191d0af3d789807aded9f4",
		"quay.io/prometheus/prometheus@sha256:" +
			"bd2dcadfb0d1096e2a4c21817ac7af918e2f19ff628e4bf25fd67a924c13dd80",
	}
	if !reflect.DeepEqual(images, wantImages) {
		t.Errorf("candidate-enabled rendered images = %#v, want %#v", images, wantImages)
	}
	for _, image := range images {
		if strings.Contains(image, ":latest") || !strings.Contains(image, "@sha256:") {
			t.Errorf("candidate-enabled render contains mutable image %q", image)
		}
	}
}

func TestPromtoolPin(t *testing.T) {
	makefile, err := os.ReadFile(filepath.Join("..", "..", "Makefile"))
	if err != nil {
		t.Fatalf("read Makefile: %v", err)
	}
	for _, exact := range []string{
		"PROMTOOL_VERSION ?= v3.13.1",
		"PROMTOOL_LINUX_AMD64_SHA256 ?= 962b812371aff838d152b6ff2d56fdb7a6396f5542f48ebf73421b9721f0d103",
		"https://github.com/prometheus/prometheus/releases/download/$${version}/$${archive}",
	} {
		if !bytes.Contains(makefile, []byte(exact)) {
			t.Errorf("Makefile lacks exact Promtool pin %q", exact)
		}
	}
}

func readTarGzipFiles(t *testing.T, path string, want []string) map[string][]byte {
	t.Helper()

	archive, err := os.Open(path)
	if err != nil {
		t.Fatalf("open chart archive: %v", err)
	}
	defer archive.Close()

	gzipReader, err := gzip.NewReader(archive)
	if err != nil {
		t.Fatalf("open chart gzip stream: %v", err)
	}
	defer gzipReader.Close()

	wanted := make(map[string]struct{}, len(want))
	for _, name := range want {
		wanted[name] = struct{}{}
	}
	found := make(map[string][]byte, len(want))
	tarReader := tar.NewReader(gzipReader)
	for {
		header, nextErr := tarReader.Next()
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			t.Fatalf("read chart tar stream: %v", nextErr)
		}
		if _, ok := wanted[header.Name]; !ok {
			continue
		}
		contents, readErr := io.ReadAll(tarReader)
		if readErr != nil {
			t.Fatalf("read %s from chart archive: %v", header.Name, readErr)
		}
		found[header.Name] = contents
	}
	if len(found) != len(wanted) {
		foundNames := make([]string, 0, len(found))
		for name := range found {
			foundNames = append(foundNames, name)
		}
		slices.Sort(foundNames)
		t.Fatalf("chart archive files found = %v, want %v", foundNames, want)
	}

	return found
}

func TestRenderedCollectorPackage(t *testing.T) {
	resources := renderCollectorResources(t)
	wantInventory := map[objectKey]struct{}{
		{
			Kind: namespaceKind,
			Name: observabilityNamespace,
		}: {},
		{
			Kind:      "ServiceAccount",
			Namespace: observabilityNamespace,
			Name:      collectorChartName,
		}: {},
		{
			Kind:      "ConfigMap",
			Namespace: observabilityNamespace,
			Name:      collectorResourceName,
		}: {},
		{
			Kind:      "DaemonSet",
			Namespace: observabilityNamespace,
			Name:      collectorResourceName,
		}: {},
		{
			Kind:      "Service",
			Namespace: observabilityNamespace,
			Name:      collectorChartName,
		}: {},
		{
			Kind:      networkPolicyKind,
			Namespace: observabilityNamespace,
			Name:      defaultDenyPolicyName,
		}: {},
		{
			Kind:      networkPolicyKind,
			Namespace: observabilityNamespace,
			Name:      otlpIngressPolicyName,
		}: {},
	}
	gotInventory := make(map[objectKey]struct{}, len(resources))
	for key := range resources {
		gotInventory[key] = struct{}{}
	}
	if !reflect.DeepEqual(gotInventory, wantInventory) {
		t.Fatalf("rendered object inventory = %#v, want %#v", gotInventory, wantInventory)
	}
	assertObjectNamespaces(t, resources)
	t.Log("Rendered object inventory: Namespace, ServiceAccount, ConfigMap, Service, DaemonSet, and two NetworkPolicies")

	namespace := &corev1.Namespace{}
	convertResource(t, resources, objectKey{
		Kind: namespaceKind,
		Name: observabilityNamespace,
	}, namespace)
	assertObservabilityNamespace(t, namespace)

	serviceAccount := &corev1.ServiceAccount{}
	convertResource(t, resources, objectKey{
		Kind:      "ServiceAccount",
		Namespace: observabilityNamespace,
		Name:      collectorChartName,
	}, serviceAccount)
	assertCollectorServiceAccount(t, serviceAccount)

	configMap := &corev1.ConfigMap{}
	convertResource(t, resources, objectKey{
		Kind:      "ConfigMap",
		Namespace: observabilityNamespace,
		Name:      collectorResourceName,
	}, configMap)
	assertCollectorConfig(t, configMap.Data["relay"])

	daemonSet := &appsv1.DaemonSet{}
	convertResource(t, resources, objectKey{
		Kind:      "DaemonSet",
		Namespace: observabilityNamespace,
		Name:      collectorResourceName,
	}, daemonSet)
	assertCollectorDaemonSet(t, daemonSet)

	service := &corev1.Service{}
	convertResource(t, resources, objectKey{
		Kind:      "Service",
		Namespace: observabilityNamespace,
		Name:      collectorChartName,
	}, service)
	assertCollectorService(t, service, daemonSet)

	defaultDeny := &networkingv1.NetworkPolicy{}
	convertResource(t, resources, objectKey{
		Kind:      networkPolicyKind,
		Namespace: observabilityNamespace,
		Name:      defaultDenyPolicyName,
	}, defaultDeny)
	assertDefaultDenyPolicy(t, defaultDeny)

	otlpIngress := &networkingv1.NetworkPolicy{}
	convertResource(t, resources, objectKey{
		Kind:      networkPolicyKind,
		Namespace: observabilityNamespace,
		Name:      otlpIngressPolicyName,
	}, otlpIngress)
	assertOTLPIngressPolicy(t, otlpIngress, daemonSet)
}

func assertObjectNamespaces(
	t *testing.T,
	resources map[objectKey]*unstructured.Unstructured,
) {
	t.Helper()

	for key, object := range resources {
		if key.Kind == namespaceKind {
			if object.GetNamespace() != "" {
				t.Errorf("Namespace object has namespace %q", object.GetNamespace())
			}
			continue
		}
		if object.GetNamespace() != observabilityNamespace {
			t.Errorf("rendered object %#v has namespace %q", key, object.GetNamespace())
		}
	}
}

func assertObservabilityNamespace(t *testing.T, namespace *corev1.Namespace) {
	t.Helper()

	wantLabels := map[string]string{
		applicationNameLabel:                         observabilityNamespace,
		"app.kubernetes.io/part-of":                  "cloud-native-service-control-plane",
		"pod-security.kubernetes.io/enforce":         "baseline",
		"pod-security.kubernetes.io/enforce-version": podSecurityVersion,
		"pod-security.kubernetes.io/audit":           "restricted",
		"pod-security.kubernetes.io/audit-version":   podSecurityVersion,
		"pod-security.kubernetes.io/warn":            "restricted",
		"pod-security.kubernetes.io/warn-version":    podSecurityVersion,
	}
	if namespace.Name != observabilityNamespace ||
		!reflect.DeepEqual(namespace.Labels, wantLabels) {
		t.Errorf("observability Namespace = %#v", namespace)
	}
}

func assertCollectorServiceAccount(
	t *testing.T,
	serviceAccount *corev1.ServiceAccount,
) {
	t.Helper()

	if serviceAccount.Name != collectorChartName ||
		serviceAccount.AutomountServiceAccountToken == nil ||
		*serviceAccount.AutomountServiceAccountToken {
		t.Errorf("Collector ServiceAccount = %#v", serviceAccount)
	}
}

func assertCollectorService(
	t *testing.T,
	service *corev1.Service,
	daemonSet *appsv1.DaemonSet,
) {
	t.Helper()

	wantPorts := []corev1.ServicePort{
		{
			Name:       collectorOTLPReceiver,
			Protocol:   corev1.ProtocolTCP,
			Port:       4317,
			TargetPort: intstr.FromString(collectorOTLPReceiver),
		},
		{
			Name:       "otlp-http",
			Protocol:   corev1.ProtocolTCP,
			Port:       4318,
			TargetPort: intstr.FromString("otlp-http"),
		},
	}
	if service.Spec.Type != corev1.ServiceTypeClusterIP ||
		service.Spec.InternalTrafficPolicy == nil ||
		*service.Spec.InternalTrafficPolicy != corev1.ServiceInternalTrafficPolicyCluster ||
		!reflect.DeepEqual(service.Spec.Ports, wantPorts) {
		t.Errorf("Collector Service network contract = %#v", service.Spec)
	}
	if !reflect.DeepEqual(service.Spec.Selector, daemonSet.Spec.Template.Labels) {
		t.Errorf(
			"Collector Service selector = %#v, Pod labels = %#v",
			service.Spec.Selector,
			daemonSet.Spec.Template.Labels,
		)
	}
	if service.Spec.ExternalName != "" || len(service.Spec.ExternalIPs) != 0 ||
		service.Spec.LoadBalancerIP != "" || len(service.Spec.LoadBalancerSourceRanges) != 0 ||
		service.Spec.HealthCheckNodePort != 0 {
		t.Errorf("Collector Service has public exposure fields: %#v", service.Spec)
	}
	for _, port := range service.Spec.Ports {
		if port.NodePort != 0 || !containerHasNamedPort(
			daemonSet.Spec.Template.Spec.Containers[0],
			port.TargetPort.StrVal,
			port.Port,
		) {
			t.Errorf("Collector Service port does not resolve to a named Pod port: %#v", port)
		}
	}
}

func containerHasNamedPort(container corev1.Container, name string, port int32) bool {
	for _, containerPort := range container.Ports {
		if containerPort.Name == name && containerPort.ContainerPort == port &&
			containerPort.Protocol == corev1.ProtocolTCP {
			return true
		}
	}

	return false
}

func assertDefaultDenyPolicy(t *testing.T, policy *networkingv1.NetworkPolicy) {
	t.Helper()

	wantSpec := networkingv1.NetworkPolicySpec{
		PodSelector: metav1.LabelSelector{},
		PolicyTypes: []networkingv1.PolicyType{
			networkingv1.PolicyTypeIngress,
			networkingv1.PolicyTypeEgress,
		},
	}
	if !reflect.DeepEqual(policy.Spec, wantSpec) {
		t.Errorf("default-deny NetworkPolicy = %#v", policy.Spec)
	}
}

func assertOTLPIngressPolicy(
	t *testing.T,
	policy *networkingv1.NetworkPolicy,
	daemonSet *appsv1.DaemonSet,
) {
	t.Helper()

	wantPeers := []networkingv1.NetworkPolicyPeer{
		{
			NamespaceSelector: namespaceNameSelector("platform-system"),
			PodSelector: labelSelector(map[string]string{
				otlpClientLabel: "control-plane-api",
			}),
		},
		{
			NamespaceSelector: namespaceNameSelector("applications"),
			PodSelector: labelSelector(map[string]string{
				otlpClientLabel: "managed-service",
			}),
		},
	}
	wantPorts := []networkingv1.NetworkPolicyPort{
		{Protocol: ptr.To(corev1.ProtocolTCP), Port: ptr.To(intstr.FromInt32(4317))},
		{Protocol: ptr.To(corev1.ProtocolTCP), Port: ptr.To(intstr.FromInt32(4318))},
	}
	wantSpec := networkingv1.NetworkPolicySpec{
		PodSelector: metav1.LabelSelector{
			MatchLabels: daemonSet.Spec.Template.Labels,
		},
		PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
		Ingress: []networkingv1.NetworkPolicyIngressRule{
			{From: wantPeers, Ports: wantPorts},
		},
	}
	if !reflect.DeepEqual(policy.Spec, wantSpec) {
		t.Errorf("Collector OTLP ingress NetworkPolicy = %#v", policy.Spec)
	}
}

func namespaceNameSelector(namespace string) *metav1.LabelSelector {
	return labelSelector(map[string]string{
		"kubernetes.io/metadata.name": namespace,
	})
}

func labelSelector(labels map[string]string) *metav1.LabelSelector {
	return &metav1.LabelSelector{MatchLabels: labels}
}

func TestFirstPartyImageSetUnchanged(t *testing.T) {
	repositoryRoot := filepath.Join("..", "..")
	kustomizePath := filepath.Join(repositoryRoot, "bin", "kustomize")
	manager := renderKustomization(
		t,
		kustomizePath,
		filepath.Join(repositoryRoot, "config", "default"),
	)
	api := renderKustomization(
		t,
		kustomizePath,
		filepath.Join(repositoryRoot, "kubernetes", "platform", "control-plane-api"),
	)

	wantReferences := map[string][]byte{
		"operator": []byte("ghcr.io/etclank/cloud-native-service-control-plane-operator@sha256:" +
			"8b537b3ab1780141b2554e495f7dd72ab36f7eb4d37c7ed396ebfa0d7cc88eb4"),
		"demo": []byte("ghcr.io/etclank/cloud-native-service-control-plane-demo-http@sha256:" +
			"bf9a75e48c4cbe2a14be4c61339115b76c2af11a06bfcc1b560f52ff3ed46e9e"),
		"api": []byte("ghcr.io/etclank/cloud-native-service-control-plane-api@sha256:" +
			"604c16f04b00272b7b45072ff0c50c5c2d081fbc4ee795e62a4ed1fc861df36e"),
	}
	if count := bytes.Count(manager, wantReferences["operator"]); count != 1 {
		t.Errorf("operator image count = %d, want 1", count)
	}
	if count := bytes.Count(manager, wantReferences["demo"]); count != 1 {
		t.Errorf("demo image count = %d, want 1", count)
	}
	if count := bytes.Count(api, wantReferences["api"]); count != 1 {
		t.Errorf("API image count = %d, want 1", count)
	}
}

func assertCollectorDaemonSet(t *testing.T, daemonSet *appsv1.DaemonSet) {
	t.Helper()

	strategy := daemonSet.Spec.UpdateStrategy
	if strategy.Type != appsv1.RollingUpdateDaemonSetStrategyType ||
		strategy.RollingUpdate == nil || strategy.RollingUpdate.MaxUnavailable == nil ||
		*strategy.RollingUpdate.MaxUnavailable != intstr.FromInt32(1) {
		t.Errorf("Collector update strategy = %#v", strategy)
	}

	podSpec := daemonSet.Spec.Template.Spec
	if podSpec.ServiceAccountName != collectorChartName ||
		podSpec.AutomountServiceAccountToken == nil ||
		*podSpec.AutomountServiceAccountToken || len(podSpec.ImagePullSecrets) != 0 {
		t.Errorf("Collector Pod identity = %#v", podSpec)
	}
	if podSpec.HostNetwork || podSpec.HostPID || podSpec.HostIPC ||
		podSpec.ShareProcessNamespace != nil {
		t.Error("Collector Pod enables host or shared process namespaces")
	}
	assertPodSecurity(t, podSpec.SecurityContext)

	if len(podSpec.Containers) != 1 || len(podSpec.InitContainers) != 0 {
		t.Fatalf(
			"Collector containers = %d, init containers = %d",
			len(podSpec.Containers),
			len(podSpec.InitContainers),
		)
	}
	container := podSpec.Containers[0]
	if container.Name != "opentelemetry-collector" ||
		container.Image != collectorImage ||
		!reflect.DeepEqual(container.Command, []string{"/otelcol-k8s"}) ||
		strings.Contains(container.Image, ":0.156.0") ||
		strings.Contains(container.Image, ":latest") {
		t.Errorf("Collector container identity = %#v", container)
	}
	assertContainerPorts(t, container.Ports)
	assertContainerResources(t, container.Resources)
	assertContainerSecurity(t, container.SecurityContext)
	assertNoSecretEnvironment(t, container)
	assertVolumes(t, podSpec.Volumes, container.VolumeMounts)

	t.Logf("Rendered image inventory: %s", container.Image)
	t.Logf(
		"Collector per-node aggregate budget: requests %s CPU/%s memory; limits %s CPU/%s memory",
		container.Resources.Requests.Cpu().String(),
		container.Resources.Requests.Memory().String(),
		container.Resources.Limits.Cpu().String(),
		container.Resources.Limits.Memory().String(),
	)
	t.Log("DaemonSet unavailable-pod budget: maxUnavailable=1")
}

func assertNoSecretEnvironment(t *testing.T, container corev1.Container) {
	t.Helper()

	if len(container.EnvFrom) != 0 {
		t.Errorf("Collector envFrom = %#v", container.EnvFrom)
	}
	for _, environment := range container.Env {
		if environment.ValueFrom != nil && (environment.ValueFrom.SecretKeyRef != nil ||
			environment.ValueFrom.ConfigMapKeyRef != nil) {
			t.Errorf("Collector environment %q uses a Secret or ConfigMap reference", environment.Name)
		}
	}
}

func assertPodSecurity(t *testing.T, securityContext *corev1.PodSecurityContext) {
	t.Helper()

	if securityContext == nil || securityContext.RunAsNonRoot == nil ||
		!*securityContext.RunAsNonRoot || securityContext.RunAsUser == nil ||
		*securityContext.RunAsUser != 10001 || securityContext.RunAsGroup == nil ||
		*securityContext.RunAsGroup != 10001 || securityContext.FSGroup == nil ||
		*securityContext.FSGroup != 10001 || securityContext.SeccompProfile == nil ||
		securityContext.SeccompProfile.Type != corev1.SeccompProfileTypeRuntimeDefault {
		t.Errorf("Collector Pod security context = %#v", securityContext)
	}
}

func assertContainerSecurity(t *testing.T, securityContext *corev1.SecurityContext) {
	t.Helper()

	if securityContext == nil || securityContext.AllowPrivilegeEscalation == nil ||
		*securityContext.AllowPrivilegeEscalation || securityContext.Privileged == nil ||
		*securityContext.Privileged || securityContext.ReadOnlyRootFilesystem == nil ||
		!*securityContext.ReadOnlyRootFilesystem || securityContext.RunAsNonRoot == nil ||
		!*securityContext.RunAsNonRoot || securityContext.RunAsUser == nil ||
		*securityContext.RunAsUser != 10001 || securityContext.RunAsGroup == nil ||
		*securityContext.RunAsGroup != 10001 || securityContext.Capabilities == nil ||
		!reflect.DeepEqual(
			securityContext.Capabilities.Drop,
			[]corev1.Capability{"ALL"},
		) {
		t.Errorf("Collector container security context = %#v", securityContext)
	}
}

func assertContainerPorts(t *testing.T, ports []corev1.ContainerPort) {
	t.Helper()

	wantPorts := []corev1.ContainerPort{
		{Name: collectorOTLPReceiver, ContainerPort: 4317, Protocol: corev1.ProtocolTCP},
		{Name: "otlp-http", ContainerPort: 4318, Protocol: corev1.ProtocolTCP},
	}
	if !reflect.DeepEqual(ports, wantPorts) {
		t.Errorf("Collector ports = %#v, want %#v", ports, wantPorts)
	}
	for _, port := range ports {
		if port.HostPort != 0 {
			t.Errorf("Collector port %q exposes hostPort %d", port.Name, port.HostPort)
		}
	}
}

func assertContainerResources(t *testing.T, resources corev1.ResourceRequirements) {
	t.Helper()

	wantRequests := corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("25m"),
		corev1.ResourceMemory: resource.MustParse("96Mi"),
	}
	wantLimits := corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("200m"),
		corev1.ResourceMemory: resource.MustParse("192Mi"),
	}
	if !reflect.DeepEqual(resources.Requests, wantRequests) ||
		!reflect.DeepEqual(resources.Limits, wantLimits) {
		t.Errorf("Collector resources = %#v", resources)
	}
}

func assertVolumes(
	t *testing.T,
	volumes []corev1.Volume,
	mounts []corev1.VolumeMount,
) {
	t.Helper()

	if len(volumes) != 1 || volumes[0].ConfigMap == nil ||
		volumes[0].HostPath != nil || volumes[0].Secret != nil ||
		volumes[0].EmptyDir != nil || volumes[0].PersistentVolumeClaim != nil {
		t.Errorf("Collector volumes = %#v", volumes)
	}
	if len(mounts) != 1 || mounts[0].Name != volumes[0].Name ||
		mounts[0].MountPath != "/conf" {
		t.Errorf("Collector volume mounts = %#v", mounts)
	}
}

func assertCollectorConfig(t *testing.T, configYAML string) {
	t.Helper()

	configJSON, err := yaml.ToJSON([]byte(configYAML))
	if err != nil {
		t.Fatalf("convert Collector configuration to JSON: %v", err)
	}
	config := map[string]any{}
	if err := json.Unmarshal(configJSON, &config); err != nil {
		t.Fatalf("decode Collector configuration: %v", err)
	}

	assertMapKeys(t, config, []string{"exporters", "extensions", "processors", "receivers", "service"})
	exporters := nestedMap(t, config, "exporters")
	assertMapKeys(t, exporters, []string{"nop"})
	receivers := nestedMap(t, config, "receivers")
	assertMapKeys(t, receivers, []string{collectorOTLPReceiver})
	processors := nestedMap(t, config, "processors")
	assertMapKeys(t, processors, []string{"batch", "memory_limiter"})

	memoryLimiter := nestedMap(t, processors, "memory_limiter")
	assertValue(t, memoryLimiter, "check_interval", "5s")
	assertNumber(t, memoryLimiter, "limit_mib", 144)
	assertNumber(t, memoryLimiter, "spike_limit_mib", 32)
	batch := nestedMap(t, processors, "batch")
	assertValue(t, batch, "timeout", "5s")
	assertNumber(t, batch, "send_batch_size", 512)
	assertNumber(t, batch, "send_batch_max_size", 1024)

	otlp := nestedMap(t, receivers, collectorOTLPReceiver)
	protocols := nestedMap(t, otlp, "protocols")
	assertMapKeys(t, protocols, []string{"grpc", "http"})
	assertValue(t, nestedMap(t, protocols, "grpc"), "endpoint", "${env:MY_POD_IP}:4317")
	assertValue(t, nestedMap(t, protocols, "http"), "endpoint", "${env:MY_POD_IP}:4318")

	service := nestedMap(t, config, "service")
	pipelines := nestedMap(t, service, "pipelines")
	assertMapKeys(t, pipelines, []string{"logs", "metrics", "traces"})
	for _, signal := range []string{"logs", "metrics", "traces"} {
		pipeline := nestedMap(t, pipelines, signal)
		assertStringSlice(t, pipeline, "receivers", []string{collectorOTLPReceiver})
		assertStringSlice(t, pipeline, "processors", []string{"memory_limiter", "batch"})
		assertStringSlice(t, pipeline, "exporters", []string{"nop"})
	}

	forbidden := []string{
		"filelog", "k8s_attributes", "prometheus", "loki", "tempo",
		"sending_queue", "file_storage", "authorization", "bearer",
	}
	for _, value := range forbidden {
		if strings.Contains(strings.ToLower(configYAML), value) {
			t.Errorf("Collector configuration contains forbidden value %q", value)
		}
	}
}

func renderCollectorResources(
	t *testing.T,
) map[objectKey]*unstructured.Unstructured {
	t.Helper()

	repositoryRoot := filepath.Join("..", "..")
	command := exec.Command(
		filepath.Join(repositoryRoot, "bin", "helm"),
		"template",
		"observability",
		filepath.Join(repositoryRoot, "deploy", "observability"),
		"--namespace",
		observabilityNamespace,
	)
	rendered, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("render Collector package: %v\n%s", err, rendered)
	}

	resources := make(map[objectKey]*unstructured.Unstructured)
	decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(rendered), 4096)
	for {
		object := &unstructured.Unstructured{}
		err := decoder.Decode(object)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("decode rendered Collector resource: %v", err)
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
			t.Fatalf("duplicate rendered Collector resource %#v", key)
		}
		resources[key] = object
	}

	return resources
}

func convertResource(
	t *testing.T,
	resources map[objectKey]*unstructured.Unstructured,
	key objectKey,
	target any,
) {
	t.Helper()

	object, exists := resources[key]
	if !exists {
		t.Fatalf("rendered resource %#v is absent", key)
	}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(
		object.Object,
		target,
	); err != nil {
		t.Fatalf("convert rendered resource %#v: %v", key, err)
	}
}

func decodeYAMLFile[T any](t *testing.T, path string) T {
	t.Helper()

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	return decodeYAML[T](t, contents)
}

func decodeYAML[T any](t *testing.T, contents []byte) T {
	t.Helper()

	var result T
	decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(contents), 4096)
	if err := decoder.Decode(&result); err != nil {
		t.Fatalf("decode YAML: %v", err)
	}

	return result
}

func nestedMap(t *testing.T, object map[string]any, field string) map[string]any {
	t.Helper()

	value, found, err := unstructured.NestedMap(object, field)
	if err != nil || !found {
		t.Fatalf("read map field %q: found=%t error=%v", field, found, err)
	}

	return value
}

func assertMapKeys(t *testing.T, object map[string]any, want []string) {
	t.Helper()

	got := make([]string, 0, len(object))
	for key := range object {
		got = append(got, key)
	}
	slices.Sort(got)
	slices.Sort(want)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("map keys = %#v, want %#v", got, want)
	}
}

func assertValue(t *testing.T, object map[string]any, field string, want string) {
	t.Helper()

	got, found, err := unstructured.NestedString(object, field)
	if err != nil || !found || got != want {
		t.Errorf("field %q = %q, found=%t, error=%v, want %q", field, got, found, err, want)
	}
}

func assertBoolean(t *testing.T, object map[string]any, field string, want bool) {
	t.Helper()

	got, found, err := unstructured.NestedBool(object, field)
	if err != nil || !found || got != want {
		t.Errorf("field %q = %t, found=%t, error=%v, want %t", field, got, found, err, want)
	}
}

func assertNumber(t *testing.T, object map[string]any, field string, want float64) {
	t.Helper()

	got, found, err := unstructured.NestedFloat64(object, field)
	if err != nil || !found || got != want {
		t.Errorf("field %q = %f, found=%t, error=%v, want %f", field, got, found, err, want)
	}
}

func assertStringSlice(
	t *testing.T,
	object map[string]any,
	field string,
	want []string,
) {
	t.Helper()

	got, found, err := unstructured.NestedStringSlice(object, field)
	if err != nil || !found || !reflect.DeepEqual(got, want) {
		t.Errorf("field %q = %#v, found=%t, error=%v, want %#v", field, got, found, err, want)
	}
}

func renderKustomization(t *testing.T, binary, path string) []byte {
	t.Helper()

	command := exec.Command(binary, "build", path)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("render Kustomization %s: %v\n%s", path, err, output)
	}

	return output
}
