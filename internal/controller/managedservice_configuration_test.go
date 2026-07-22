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
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/yaml"
)

const (
	platformSystemNamespace = "platform-system"
	approvedDemoHTTPImage   = "ghcr.io/etclank/cloud-native-service-control-plane-demo-http@sha256:" +
		"bf9a75e48c4cbe2a14be4c61339115b76c2af11a06bfcc1b560f52ff3ed46e9e"
	approvedImagePullSecret    = "ghcr-pull"
	approvedOperatorRepository = "ghcr.io/etclank/cloud-native-service-control-plane-operator"
	approvedOperatorImage      = approvedOperatorRepository + "@sha256:" +
		"8b537b3ab1780141b2554e495f7dd72ab36f7eb4d37c7ed396ebfa0d7cc88eb4"
)

func TestRenderedManagerConfiguration(t *testing.T) {
	kustomizePath := filepath.Join("..", "..", "bin", "kustomize")
	configurationPath := filepath.Join(
		"..",
		"..",
		"config",
		"default",
	)
	command := exec.Command(kustomizePath, "build", configurationPath)
	rendered, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("render manager configuration: %v\n%s", err, rendered)
	}

	deployment := findRenderedManagerDeployment(t, rendered)
	if deployment.Namespace != platformSystemNamespace {
		t.Errorf(
			"manager Deployment namespace = %q, want %q",
			deployment.Namespace,
			platformSystemNamespace,
		)
	}
	if _, exists := deployment.Spec.Template.Labels[otlpClientLabel]; exists {
		t.Errorf("operator Pod template has %q", otlpClientLabel)
	}
	wantWorkloadSelector := map[string]string{
		applicationNameLabel: "cloud-native-service-control-plane",
		"control-plane":      "controller-manager",
	}
	if deployment.Spec.Selector == nil ||
		!reflect.DeepEqual(
			deployment.Spec.Selector.MatchLabels,
			wantWorkloadSelector,
		) {
		t.Errorf("operator Deployment selector = %#v", deployment.Spec.Selector)
	}
	operatorDiagnosticLabels := map[string]string{
		applicationNameLabel:                       "h83d-operator-negative",
		"observability.eoghanclancy.eu/validation": "h8.3d",
	}
	if labelSetMatchesSelector(operatorDiagnosticLabels, wantWorkloadSelector) {
		t.Errorf(
			"diagnostic labels %#v match operator Deployment selector %#v",
			operatorDiagnosticLabels,
			wantWorkloadSelector,
		)
	}

	metricsService := findRenderedManagerMetricsService(t, rendered)
	if !reflect.DeepEqual(metricsService.Spec.Selector, wantWorkloadSelector) {
		t.Errorf(
			"operator metrics Service selector = %#v, want %#v",
			metricsService.Spec.Selector,
			wantWorkloadSelector,
		)
	}
	if _, exists := metricsService.Labels[otlpClientLabel]; exists {
		t.Errorf("operator metrics Service metadata has %q", otlpClientLabel)
	}
	if labelSetMatchesSelector(
		operatorDiagnosticLabels,
		metricsService.Spec.Selector,
	) {
		t.Errorf(
			"diagnostic labels %#v match operator metrics Service selector %#v",
			operatorDiagnosticLabels,
			metricsService.Spec.Selector,
		)
	}

	manager := findManagerContainer(t, deployment)
	if manager.Image != approvedOperatorImage {
		t.Errorf("manager image = %q, want %q", manager.Image, approvedOperatorImage)
	}
	if strings.Contains(string(rendered), "controller:latest") {
		t.Error("rendered configuration contains controller:latest")
	}
	if strings.Contains(manager.Image, approvedOperatorRepository+":sha-") {
		t.Errorf("manager uses an operator commit tag at runtime: %q", manager.Image)
	}

	imagePullSecrets := deployment.Spec.Template.Spec.ImagePullSecrets
	if len(imagePullSecrets) != 1 ||
		imagePullSecrets[0].Name != approvedImagePullSecret {
		t.Errorf(
			"manager imagePullSecrets = %#v, want only %q",
			imagePullSecrets,
			approvedImagePullSecret,
		)
	}

	arguments := argumentCounts(manager.Args)
	wantImageArgument := "--demo-http-image=" + approvedDemoHTTPImage
	wantPullSecretArgument := "--managed-service-image-pull-secret=" +
		approvedImagePullSecret

	if arguments[wantImageArgument] != 1 {
		t.Errorf(
			"demo-http image argument count = %d, want 1",
			arguments[wantImageArgument],
		)
	}
	if arguments[wantPullSecretArgument] != 1 {
		t.Errorf(
			"image pull Secret argument count = %d, want 1",
			arguments[wantPullSecretArgument],
		)
	}
	if arguments["--metrics-bind-address=:8443"] != 1 {
		t.Errorf(
			"authenticated metrics listener argument count = %d, want 1",
			arguments["--metrics-bind-address=:8443"],
		)
	}
	if arguments["--metrics-secure=false"] != 0 {
		t.Error("manager disables metrics endpoint authentication")
	}

	for _, argument := range manager.Args {
		if !strings.HasPrefix(argument, "--demo-http-image=") {
			continue
		}

		image := strings.TrimPrefix(argument, "--demo-http-image=")
		if !strings.Contains(image, "@sha256:") {
			t.Errorf("demo-http image is not digest-qualified: %q", image)
		}
		if strings.Contains(image, ":latest") {
			t.Errorf("demo-http image uses a mutable latest tag: %q", image)
		}
		if strings.Contains(image, ":sha-") {
			t.Errorf("demo-http image uses a commit tag at runtime: %q", image)
		}
	}
}

func TestDefaultInstallerPreservesCommittedImage(t *testing.T) {
	repositoryRoot := filepath.Join("..", "..")
	kustomizationPath := filepath.Join(
		repositoryRoot,
		"config",
		"manager",
		"kustomization.yaml",
	)
	installerDirectory := filepath.Join(repositoryRoot, "dist")
	installerPath := filepath.Join(installerDirectory, "install.yaml")

	before, err := os.ReadFile(kustomizationPath)
	if err != nil {
		t.Fatalf("read manager kustomization before installer build: %v", err)
	}
	directoryExisted := true
	if _, err := os.Stat(installerDirectory); errors.Is(err, os.ErrNotExist) {
		directoryExisted = false
	} else if err != nil {
		t.Fatalf("inspect installer directory: %v", err)
	}
	installerBefore, installerReadErr := os.ReadFile(installerPath)
	installerExisted := installerReadErr == nil
	if installerReadErr != nil && !errors.Is(installerReadErr, os.ErrNotExist) {
		t.Fatalf("read existing installer: %v", installerReadErr)
	}
	installerMode := os.FileMode(0o644)
	if installerExisted {
		installerInfo, err := os.Stat(installerPath)
		if err != nil {
			t.Fatalf("inspect existing installer: %v", err)
		}
		installerMode = installerInfo.Mode()
	}
	t.Cleanup(func() {
		if installerExisted {
			if err := os.WriteFile(
				installerPath,
				installerBefore,
				installerMode,
			); err != nil {
				t.Errorf("restore existing installer: %v", err)
			}
		} else if err := os.Remove(installerPath); err != nil &&
			!errors.Is(err, os.ErrNotExist) {
			t.Errorf("remove generated installer: %v", err)
		}
		if !directoryExisted {
			if err := os.Remove(installerDirectory); err != nil &&
				!errors.Is(err, os.ErrNotExist) {
				t.Errorf("remove generated installer directory: %v", err)
			}
		}
	})

	buildInstaller := exec.Command("make", "--no-print-directory", "build-installer")
	buildInstaller.Dir = repositoryRoot
	buildInstaller.Env = environmentWithoutVariable(os.Environ(), "IMG")
	if output, err := buildInstaller.CombinedOutput(); err != nil {
		t.Fatalf("build default installer: %v\n%s", err, output)
	}

	after, err := os.ReadFile(kustomizationPath)
	if err != nil {
		t.Fatalf("read manager kustomization after installer build: %v", err)
	}
	if !bytes.Equal(after, before) {
		t.Error("default installer build modified manager kustomization")
	}

	installer, err := os.ReadFile(installerPath)
	if err != nil {
		t.Fatalf("read generated installer: %v", err)
	}
	if !bytes.Contains(installer, []byte("image: "+approvedOperatorImage)) {
		t.Errorf("generated installer does not use approved image %q", approvedOperatorImage)
	}
	if bytes.Contains(installer, []byte("controller:latest")) {
		t.Error("generated installer contains controller:latest")
	}

	overrideImage := "example.com/operator:test"
	dryRun := exec.Command(
		"make",
		"--no-print-directory",
		"--dry-run",
		"build-installer",
		"IMG="+overrideImage,
	)
	dryRun.Dir = repositoryRoot
	dryRunOutput, err := dryRun.CombinedOutput()
	if err != nil {
		t.Fatalf("dry-run installer image override: %v\n%s", err, dryRunOutput)
	}
	if !bytes.Contains(
		dryRunOutput,
		[]byte("edit set image controller="+overrideImage),
	) {
		t.Errorf("installer dry-run does not retain explicit IMG override:\n%s", dryRunOutput)
	}
}

func findRenderedManagerDeployment(
	t *testing.T,
	rendered []byte,
) *appsv1.Deployment {
	t.Helper()

	decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(rendered), 4096)
	for {
		object := &unstructured.Unstructured{}
		err := decoder.Decode(object)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("decode rendered configuration: %v", err)
		}
		if object.GetKind() != "Deployment" ||
			object.GetName() != "platform-operator-controller-manager" {
			continue
		}

		deployment := &appsv1.Deployment{}
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(
			object.Object,
			deployment,
		); err != nil {
			t.Fatalf("convert rendered manager Deployment: %v", err)
		}

		return deployment
	}

	t.Fatal("rendered configuration has no platform operator manager Deployment")

	return nil
}

func findRenderedManagerMetricsService(
	t *testing.T,
	rendered []byte,
) *corev1.Service {
	t.Helper()

	decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(rendered), 4096)
	for {
		object := &unstructured.Unstructured{}
		err := decoder.Decode(object)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("decode rendered configuration: %v", err)
		}
		if object.GetKind() != "Service" ||
			object.GetName() != "platform-operator-controller-manager-metrics-service" {
			continue
		}

		service := &corev1.Service{}
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(
			object.Object,
			service,
		); err != nil {
			t.Fatalf("convert rendered manager metrics Service: %v", err)
		}

		return service
	}

	t.Fatal("rendered configuration has no platform operator metrics Service")

	return nil
}

func findManagerContainer(
	t *testing.T,
	deployment *appsv1.Deployment,
) *corev1.Container {
	t.Helper()

	for index := range deployment.Spec.Template.Spec.Containers {
		container := &deployment.Spec.Template.Spec.Containers[index]
		if container.Name == "manager" {
			return container
		}
	}

	t.Fatal("rendered manager Deployment has no manager container")

	return nil
}

func argumentCounts(arguments []string) map[string]int {
	counts := make(map[string]int)
	for _, argument := range arguments {
		counts[argument]++
	}

	return counts
}

func environmentWithoutVariable(environment []string, name string) []string {
	filtered := make([]string, 0, len(environment))
	prefix := name + "="
	for _, variable := range environment {
		if !strings.HasPrefix(variable, prefix) {
			filtered = append(filtered, variable)
		}
	}

	return filtered
}

func labelSetMatchesSelector(labels, selector map[string]string) bool {
	for key, value := range selector {
		if labels[key] != value {
			return false
		}
	}

	return true
}
