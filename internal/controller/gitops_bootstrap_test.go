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
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/yaml"
)

const (
	argoAPIVersion        = "argoproj.io/v1alpha1"
	argoNamespace         = "argocd"
	clusterServer         = "https://kubernetes.default.svc"
	platformProjectName   = "platform-control-plane"
	platformRepositoryURL = "git@github.com:etclank/cloud-native-service-control-plane.git"
)

type argoProject struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Metadata   struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	} `json:"metadata"`
	Spec struct {
		Description  string   `json:"description"`
		SourceRepos  []string `json:"sourceRepos"`
		Destinations []struct {
			Server    string `json:"server"`
			Namespace string `json:"namespace"`
		} `json:"destinations"`
		ClusterResourceWhitelist   []resourcePermission `json:"clusterResourceWhitelist"`
		NamespaceResourceWhitelist []resourcePermission `json:"namespaceResourceWhitelist"`
	} `json:"spec"`
}

type argoApplication struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Metadata   struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	} `json:"metadata"`
	Spec struct {
		Project string `json:"project"`
		Source  struct {
			RepoURL        string `json:"repoURL"`
			TargetRevision string `json:"targetRevision"`
			Path           string `json:"path"`
		} `json:"source"`
		Destination struct {
			Server    string `json:"server"`
			Namespace string `json:"namespace"`
		} `json:"destination"`
		SyncPolicy struct {
			Automated   map[string]any `json:"automated,omitempty"`
			SyncOptions []string       `json:"syncOptions,omitempty"`
		} `json:"syncPolicy"`
	} `json:"spec"`
}

type resourcePermission struct {
	Group string `json:"group"`
	Kind  string `json:"kind"`
}

func TestPlatformOperatorGitOpsBootstrap(t *testing.T) {
	project := decodeBootstrapResource[argoProject](
		t,
		"platform-control-plane-project.yaml",
	)
	application := decodeBootstrapResource[argoApplication](
		t,
		"platform-operator-application.yaml",
	)
	apiApplication := decodeBootstrapResource[argoApplication](
		t,
		"control-plane-api-application.yaml",
	)

	assertPlatformProject(t, project)
	assertPlatformOperatorApplication(t, application)
	assertControlPlaneAPIApplication(t, apiApplication)

	requiredClusterResources := renderedClusterScopedResources(t)
	projectClusterResources := permissionSet(
		t,
		project.Spec.ClusterResourceWhitelist,
	)
	if !reflect.DeepEqual(projectClusterResources, requiredClusterResources) {
		t.Errorf(
			"cluster resource permissions = %#v, want rendered resources %#v",
			projectClusterResources,
			requiredClusterResources,
		)
	}
}

func assertPlatformProject(t *testing.T, project argoProject) {
	t.Helper()

	if project.APIVersion != argoAPIVersion || project.Kind != "AppProject" {
		t.Errorf("project identity = %s %s", project.APIVersion, project.Kind)
	}
	if project.Metadata.Name != platformProjectName ||
		project.Metadata.Namespace != argoNamespace {
		t.Errorf("project metadata = %#v", project.Metadata)
	}
	if project.Spec.Description == "" {
		t.Error("project description is empty")
	}
	if !reflect.DeepEqual(project.Spec.SourceRepos, []string{platformRepositoryURL}) {
		t.Errorf("project source repositories = %#v", project.Spec.SourceRepos)
	}
	wantDestinations := []struct {
		Server    string `json:"server"`
		Namespace string `json:"namespace"`
	}{
		{Server: clusterServer, Namespace: platformSystemNamespace},
		{Server: clusterServer, Namespace: "applications"},
	}
	if !reflect.DeepEqual(project.Spec.Destinations, wantDestinations) {
		t.Errorf("project destinations = %#v", project.Spec.Destinations)
	}

	permissionSet(t, project.Spec.ClusterResourceWhitelist)
	wantNamespaceResources := map[resourcePermission]struct{}{
		{Group: "", Kind: "ServiceAccount"}:                       {},
		{Group: "", Kind: "Service"}:                              {},
		{Group: "apps", Kind: "Deployment"}:                       {},
		{Group: "cert-manager.io", Kind: "Certificate"}:           {},
		{Group: "networking.k8s.io", Kind: "Ingress"}:             {},
		{Group: "rbac.authorization.k8s.io", Kind: "Role"}:        {},
		{Group: "rbac.authorization.k8s.io", Kind: "RoleBinding"}: {},
		{Group: "traefik.io", Kind: "Middleware"}:                 {},
	}
	gotNamespaceResources := permissionSet(
		t,
		project.Spec.NamespaceResourceWhitelist,
	)
	if !reflect.DeepEqual(gotNamespaceResources, wantNamespaceResources) {
		t.Errorf(
			"namespace resource permissions = %#v, want %#v",
			gotNamespaceResources,
			wantNamespaceResources,
		)
	}
}

func assertPlatformOperatorApplication(t *testing.T, application argoApplication) {
	t.Helper()

	if application.APIVersion != argoAPIVersion ||
		application.Kind != "Application" {
		t.Errorf("application identity = %s %s", application.APIVersion, application.Kind)
	}
	if application.Metadata.Name != platformOperatorName ||
		application.Metadata.Namespace != argoNamespace {
		t.Errorf("application metadata = %#v", application.Metadata)
	}
	if application.Spec.Project != platformProjectName {
		t.Errorf("application project = %q", application.Spec.Project)
	}
	if application.Spec.Source.RepoURL != platformRepositoryURL ||
		application.Spec.Source.TargetRevision != "main" ||
		application.Spec.Source.Path != "config/default" {
		t.Errorf("application source = %#v", application.Spec.Source)
	}
	if application.Spec.Destination.Server != clusterServer ||
		application.Spec.Destination.Namespace != platformSystemNamespace {
		t.Errorf("application destination = %#v", application.Spec.Destination)
	}
	if application.Spec.SyncPolicy.Automated != nil {
		t.Errorf(
			"application has automated synchronization: %#v",
			application.Spec.SyncPolicy.Automated,
		)
	}
	if !reflect.DeepEqual(
		application.Spec.SyncPolicy.SyncOptions,
		[]string{"CreateNamespace=false"},
	) {
		t.Errorf(
			"application sync options = %#v",
			application.Spec.SyncPolicy.SyncOptions,
		)
	}
}

func assertControlPlaneAPIApplication(t *testing.T, application argoApplication) {
	t.Helper()

	if application.APIVersion != argoAPIVersion ||
		application.Kind != "Application" {
		t.Errorf("API application identity = %s %s", application.APIVersion, application.Kind)
	}
	if application.Metadata.Name != "control-plane-api" ||
		application.Metadata.Namespace != argoNamespace {
		t.Errorf("API application metadata = %#v", application.Metadata)
	}
	if application.Spec.Project != platformProjectName {
		t.Errorf("API application project = %q", application.Spec.Project)
	}
	if application.Spec.Source.RepoURL != platformRepositoryURL ||
		application.Spec.Source.TargetRevision != "main" ||
		application.Spec.Source.Path != "kubernetes/platform/control-plane-api" {
		t.Errorf("API application source = %#v", application.Spec.Source)
	}
	if application.Spec.Destination.Server != clusterServer ||
		application.Spec.Destination.Namespace != platformSystemNamespace {
		t.Errorf("API application destination = %#v", application.Spec.Destination)
	}
	if application.Spec.SyncPolicy.Automated != nil {
		t.Errorf(
			"API application has automated synchronization: %#v",
			application.Spec.SyncPolicy.Automated,
		)
	}
	if !reflect.DeepEqual(
		application.Spec.SyncPolicy.SyncOptions,
		[]string{"CreateNamespace=false"},
	) {
		t.Errorf(
			"API application sync options = %#v",
			application.Spec.SyncPolicy.SyncOptions,
		)
	}
}

func permissionSet(
	t *testing.T,
	permissions []resourcePermission,
) map[resourcePermission]struct{} {
	t.Helper()

	result := make(map[resourcePermission]struct{}, len(permissions))
	for _, permission := range permissions {
		if permission.Group == "*" || permission.Kind == "*" {
			t.Errorf("wildcard resource permission is not allowed: %#v", permission)
		}
		if _, exists := result[permission]; exists {
			t.Errorf("duplicate resource permission: %#v", permission)
		}
		result[permission] = struct{}{}
	}

	return result
}

func renderedClusterScopedResources(
	t *testing.T,
) map[resourcePermission]struct{} {
	t.Helper()

	repositoryRoot := filepath.Join("..", "..")
	command := exec.Command(
		filepath.Join(repositoryRoot, "bin", "kustomize"),
		"build",
		filepath.Join(repositoryRoot, "config", "default"),
	)
	rendered, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("render default configuration: %v\n%s", err, rendered)
	}

	resources := make(map[resourcePermission]struct{})
	decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(rendered), 4096)
	for {
		object := &unstructured.Unstructured{}
		err := decoder.Decode(object)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("decode rendered resource: %v", err)
		}
		if object.GetNamespace() != "" || object.GetKind() == "" {
			continue
		}

		groupVersion, err := schema.ParseGroupVersion(object.GetAPIVersion())
		if err != nil {
			t.Fatalf("parse API version %q: %v", object.GetAPIVersion(), err)
		}
		resources[resourcePermission{
			Group: groupVersion.Group,
			Kind:  object.GetKind(),
		}] = struct{}{}
	}

	return resources
}

func decodeBootstrapResource[T any](t *testing.T, filename string) T {
	t.Helper()

	path := filepath.Join("..", "..", "deploy", "gitops", "bootstrap", filename)
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read bootstrap resource %s: %v", filename, err)
	}

	var resource T
	decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(contents), 4096)
	if err := decoder.Decode(&resource); err != nil {
		t.Fatalf("decode bootstrap resource %s: %v", filename, err)
	}

	return resource
}
