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
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/yaml"
)

const (
	collectorChartName       = "opentelemetry-collector"
	collectorChartRepository = "https://open-telemetry.github.io/opentelemetry-helm-charts"
	collectorChartVersion    = "0.165.0"
	collectorAppVersion      = "0.156.0"
	collectorChartSHA256     = "b592ea064d9b906930cac2d22b88eeb1bc82f12d5ed07fd20792de2c051ca3c5"
	collectorResourceName    = "opentelemetry-collector-agent"
	collectorOTLPReceiver    = "otlp"
	collectorImage           = "ghcr.io/open-telemetry/opentelemetry-collector-releases/" +
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

func TestCollectorSupplyChain(t *testing.T) {
	repositoryRoot := filepath.Join("..", "..")
	chartPath := filepath.Join(repositoryRoot, "deploy", "observability", "Chart.yaml")
	lockPath := filepath.Join(repositoryRoot, "deploy", "observability", "Chart.lock")
	archivePath := filepath.Join(
		repositoryRoot,
		"deploy",
		"observability",
		"charts",
		collectorChartName+"-"+collectorChartVersion+".tgz",
	)

	chart := decodeYAMLFile[chartMetadata](t, chartPath)
	wantDependency := chartDependency{
		Name:       collectorChartName,
		Repository: collectorChartRepository,
		Version:    collectorChartVersion,
	}
	if chart.APIVersion != "v2" || chart.Name == "" || chart.Version == "" ||
		!reflect.DeepEqual(chart.Dependencies, []chartDependency{wantDependency}) {
		t.Errorf("wrapper Chart metadata = %#v", chart)
	}

	lockContents, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatalf("read Chart.lock: %v", err)
	}
	lock := decodeYAML[chartLock](t, lockContents)
	if !reflect.DeepEqual(lock.Dependencies, []chartDependency{wantDependency}) ||
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
	fields := strings.Fields(string(dependencyList))
	if len(fields) < 8 || !reflect.DeepEqual(fields[len(fields)-4:], []string{
		collectorChartName,
		collectorChartVersion,
		collectorChartRepository,
		"ok",
	}) {
		t.Errorf("dependency list = %q", dependencyList)
	}

	archive, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatalf("read Collector chart archive: %v", err)
	}
	archiveDigest := sha256.Sum256(archive)
	if hex.EncodeToString(archiveDigest[:]) != collectorChartSHA256 {
		t.Errorf("Collector chart SHA-256 = %x", archiveDigest)
	}

	showChart := exec.Command(helmPath, "show", "chart", archivePath)
	metadata, err := showChart.Output()
	if err != nil {
		t.Fatalf("read dependency chart metadata: %v", err)
	}
	dependencyChart := decodeYAML[chartMetadata](t, metadata)
	if dependencyChart.Name != collectorChartName ||
		dependencyChart.Version != collectorChartVersion ||
		dependencyChart.AppVersion != collectorAppVersion {
		t.Errorf("dependency chart metadata = %#v", dependencyChart)
	}
}

func TestRenderedCollectorPackage(t *testing.T) {
	resources := renderCollectorResources(t)
	wantInventory := map[objectKey]struct{}{
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
	}
	gotInventory := make(map[objectKey]struct{}, len(resources))
	for key := range resources {
		gotInventory[key] = struct{}{}
	}
	if !reflect.DeepEqual(gotInventory, wantInventory) {
		t.Fatalf("rendered object inventory = %#v, want %#v", gotInventory, wantInventory)
	}
	t.Log("Rendered object inventory: ConfigMap/opentelemetry-collector-agent, DaemonSet/opentelemetry-collector-agent")

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
			"a1265442af429340cb6cddf078cdec4b021bceb1d171215ad55fbccf7bfb4391"),
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
	if podSpec.ServiceAccountName != "default" ||
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
