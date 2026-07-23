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
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/yaml"
)

const (
	argoAPIVersion            = "argoproj.io/v1alpha1"
	argoNamespace             = "argocd"
	bootstrapPartOf           = "cloud-native-service-control-plane"
	clusterServer             = "https://kubernetes.default.svc"
	observabilityRevision     = "01c2d27b2bc671ce76686ce8d60c6a0c5b70b89d"
	privateRepository         = "git@github.com:etclank/cloud-native-service-control-plane.git"
	observabilityProjectKind  = "AppProject"
	observabilityAppKind      = "Application"
	observabilityPackagePath  = "deploy/observability"
	bootstrapPackagePath      = "deploy/gitops/bootstrap"
	applicationPartOfLabelKey = "app.kubernetes.io/part-of"
)

type resourcePermission struct {
	Group string
	Kind  string
}

func TestObservabilityGitOpsBootstrap(t *testing.T) {
	project := readSingleBootstrapObject(t, "observability-project.yaml")
	application := readSingleBootstrapObject(t, "observability-application.yaml")

	assertBootstrapIdentity(t, project, observabilityProjectKind)
	assertBootstrapIdentity(t, application, observabilityAppKind)
	assertObservabilityProject(t, project)
	assertObservabilityApplication(t, application)

	projectName := nestedString(t, application.Object, "spec", "project")
	if projectName != project.GetName() {
		t.Errorf("Application project = %q, want %q", projectName, project.GetName())
	}
}

func TestObservabilityProjectPermissionsMatchRenderedPackage(t *testing.T) {
	project := readSingleBootstrapObject(t, "observability-project.yaml")
	resources := renderCollectorResources(t)
	if len(resources) != 7 {
		t.Fatalf("rendered resource count = %d, want 7", len(resources))
	}

	wantCluster := make(map[resourcePermission]struct{})
	wantNamespaced := make(map[resourcePermission]struct{})
	for key, object := range resources {
		groupVersion, err := schema.ParseGroupVersion(object.GetAPIVersion())
		if err != nil {
			t.Fatalf("parse apiVersion for %#v: %v", key, err)
		}
		permission := resourcePermission{Group: groupVersion.Group, Kind: object.GetKind()}
		if object.GetNamespace() == "" {
			if key != (objectKey{Kind: namespaceKind, Name: observabilityNamespace}) {
				t.Errorf("unexpected cluster-scoped object: %#v", key)
			}
			wantCluster[permission] = struct{}{}
			continue
		}
		if object.GetNamespace() != observabilityNamespace {
			t.Errorf("object %#v namespace = %q", key, object.GetNamespace())
		}
		wantNamespaced[permission] = struct{}{}
	}

	gotCluster := permissionSet(t, project.Object, "clusterResourceWhitelist")
	gotNamespaced := permissionSet(t, project.Object, "namespaceResourceWhitelist")
	if !reflect.DeepEqual(gotCluster, wantCluster) {
		t.Errorf("cluster permissions = %#v, want %#v", gotCluster, wantCluster)
	}
	if !reflect.DeepEqual(gotNamespaced, wantNamespaced) {
		t.Errorf("namespace permissions = %#v, want %#v", gotNamespaced, wantNamespaced)
	}

	for permission := range gotCluster {
		if permission.Group == "*" || permission.Kind == "*" {
			t.Errorf("wildcard cluster permission: %#v", permission)
		}
	}
	for permission := range gotNamespaced {
		if permission.Group == "*" || permission.Kind == "*" {
			t.Errorf("wildcard namespace permission: %#v", permission)
		}
	}

	t.Log("AppProject permissions cannot restrict Namespace by object name; the immutable chart render contains only Namespace/observability")
}

func TestObservabilityBootstrapIsRepositoryInert(t *testing.T) {
	repositoryRoot := filepath.Join("..", "..")
	bootstrapDirectory := filepath.Join(repositoryRoot, bootstrapPackagePath)
	entries, err := os.ReadDir(bootstrapDirectory)
	if err != nil {
		t.Fatalf("read bootstrap directory: %v", err)
	}
	for _, entry := range entries {
		name := strings.ToLower(entry.Name())
		if name == "kustomization.yaml" || name == "kustomization.yml" {
			t.Errorf("bootstrap aggregator exists: %s", entry.Name())
		}
	}

	assertNoBootstrapAutomation(t, repositoryRoot)
	assertNoBootstrapRootApplication(t, repositoryRoot)

	application := readSingleBootstrapObject(t, "observability-application.yaml")
	syncPolicy := bootstrapNestedMap(t, application.Object, "spec", "syncPolicy")
	if !reflect.DeepEqual(syncPolicy, map[string]any{
		"syncOptions": []any{"CreateNamespace=false"},
	}) {
		t.Errorf("Application syncPolicy = %#v", syncPolicy)
	}
	if len(application.GetFinalizers()) != 0 {
		t.Errorf("Application finalizers = %#v", application.GetFinalizers())
	}
	for key := range application.GetAnnotations() {
		if strings.Contains(key, "hook") || strings.Contains(key, "sync-wave") {
			t.Errorf("Application lifecycle annotation = %q", key)
		}
	}
}

func assertObservabilityProject(t *testing.T, project *unstructured.Unstructured) {
	t.Helper()
	spec := bootstrapNestedMap(t, project.Object, "spec")
	assertMapKeys(t, spec, []string{
		"clusterResourceWhitelist",
		"description",
		"destinations",
		"namespaceResourceWhitelist",
		"sourceRepos",
	})
	wantDescription := "Restricted project for the portfolio observability stack"
	if got := nestedString(t, project.Object, "spec", "description"); got != wantDescription {
		t.Errorf("project description = %q, want %q", got, wantDescription)
	}
	if got := nestedStringSlice(t, project.Object, "spec", "sourceRepos"); !reflect.DeepEqual(got, []string{privateRepository}) {
		t.Errorf("project sourceRepos = %#v", got)
	}
	wantDestinations := []any{map[string]any{
		"namespace": observabilityNamespace,
		serverValue: clusterServer,
	}}
	if got := nestedSlice(t, project.Object, "spec", "destinations"); !reflect.DeepEqual(got, wantDestinations) {
		t.Errorf("project destinations = %#v, want %#v", got, wantDestinations)
	}
	assertFieldsAbsent(t, project.Object, []string{"spec"}, "roles", "syncWindows", "orphanedResources")
	if len(project.GetFinalizers()) != 0 {
		t.Errorf("AppProject finalizers = %#v", project.GetFinalizers())
	}
}

func assertObservabilityApplication(t *testing.T, application *unstructured.Unstructured) {
	t.Helper()
	spec := bootstrapNestedMap(t, application.Object, "spec")
	assertMapKeys(t, spec, []string{"destination", "project", "revisionHistoryLimit", "source", "syncPolicy"})
	source := bootstrapNestedMap(t, spec, "source")
	assertMapKeys(t, source, []string{"helm", "path", "repoURL", "targetRevision"})
	wantSourceFields := map[string]string{
		"repoURL":        privateRepository,
		"targetRevision": observabilityRevision,
		"path":           observabilityPackagePath,
	}
	for field, want := range wantSourceFields {
		if got := nestedString(t, source, field); got != want {
			t.Errorf("Application source.%s = %q, want %q", field, got, want)
		}
	}
	helm := bootstrapNestedMap(t, source, "helm")
	assertMapKeys(t, helm, []string{"passCredentials", "releaseName", "valueFiles"})
	if got := nestedString(t, helm, "releaseName"); got != observabilityNamespace {
		t.Errorf("Helm releaseName = %q", got)
	}
	if got := nestedStringSlice(t, helm, "valueFiles"); !reflect.DeepEqual(got, []string{"values.yaml"}) {
		t.Errorf("Helm valueFiles = %#v", got)
	}
	passCredentials, found, err := unstructured.NestedBool(helm, "passCredentials")
	if err != nil || !found || passCredentials {
		t.Errorf("Helm passCredentials = %t, found %t, error %v", passCredentials, found, err)
	}
	assertFieldsAbsent(t, source, nil, "chart")
	assertFieldsAbsent(t, helm, nil, "values", "valuesObject", "parameters")
	assertFieldsAbsent(t, spec, nil, "sources", "retry")

	wantDestination := map[string]any{
		"namespace": observabilityNamespace,
		serverValue: clusterServer,
	}
	if got := bootstrapNestedMap(t, spec, "destination"); !reflect.DeepEqual(got, wantDestination) {
		t.Errorf("Application destination = %#v, want %#v", got, wantDestination)
	}
	if got := nestedInt64(t, spec, "revisionHistoryLimit"); got != 10 {
		t.Errorf("revisionHistoryLimit = %d, want 10", got)
	}
}

func assertBootstrapIdentity(t *testing.T, object *unstructured.Unstructured, kind string) {
	t.Helper()
	if object.GetAPIVersion() != argoAPIVersion || object.GetKind() != kind ||
		object.GetName() != observabilityNamespace || object.GetNamespace() != argoNamespace {
		t.Errorf("bootstrap identity = %s %s/%s %s", object.GetAPIVersion(), object.GetNamespace(), object.GetName(), object.GetKind())
	}
	wantLabels := map[string]string{applicationPartOfLabelKey: bootstrapPartOf}
	if !reflect.DeepEqual(object.GetLabels(), wantLabels) {
		t.Errorf("bootstrap labels = %#v, want %#v", object.GetLabels(), wantLabels)
	}
}

func permissionSet(t *testing.T, object map[string]any, field string) map[resourcePermission]struct{} {
	t.Helper()
	entries := nestedSlice(t, object, "spec", field)
	permissions := make(map[resourcePermission]struct{}, len(entries))
	for _, entry := range entries {
		mapping, ok := entry.(map[string]any)
		if !ok {
			t.Fatalf("%s entry has type %T", field, entry)
		}
		permission := resourcePermission{
			Group: nestedString(t, mapping, "group"),
			Kind:  nestedString(t, mapping, "kind"),
		}
		if _, duplicate := permissions[permission]; duplicate {
			t.Errorf("duplicate %s entry: %#v", field, permission)
		}
		permissions[permission] = struct{}{}
	}
	return permissions
}

func readSingleBootstrapObject(t *testing.T, name string) *unstructured.Unstructured {
	t.Helper()
	path := filepath.Join("..", "..", bootstrapPackagePath, name)
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(contents), 4096)
	object := &unstructured.Unstructured{}
	if err := decoder.Decode(object); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	additional := &unstructured.Unstructured{}
	if err := decoder.Decode(additional); !errors.Is(err, io.EOF) {
		t.Fatalf("%s contains more than one YAML document", path)
	}
	return object
}

func assertNoBootstrapAutomation(t *testing.T, repositoryRoot string) {
	t.Helper()
	paths := []string{
		filepath.Join(repositoryRoot, "Makefile"),
		filepath.Join(repositoryRoot, ".github", "workflows"),
		filepath.Join(repositoryRoot, "hack"),
		filepath.Join(repositoryRoot, "scripts"),
	}
	for _, path := range paths {
		err := filepath.WalkDir(path, func(filePath string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}
			contents, readErr := os.ReadFile(filePath)
			if readErr != nil {
				return readErr
			}
			if strings.Contains(string(contents), bootstrapPackagePath) {
				t.Errorf("bootstrap path consumed by automation: %s", filePath)
			}
			return nil
		})
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("scan %s: %v", path, err)
		}
	}
}

func assertNoBootstrapRootApplication(t *testing.T, repositoryRoot string) {
	t.Helper()
	err := filepath.WalkDir(repositoryRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			relativePath, relativeErr := filepath.Rel(repositoryRoot, path)
			if relativeErr != nil {
				return relativeErr
			}
			if relativePath == ".git" || relativePath == "bin" ||
				relativePath == filepath.Join("kubernetes", "bootstrap", "argocd", "upstream") {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".yaml") && !strings.HasSuffix(path, ".yml") {
			return nil
		}
		contents, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if !bytes.Contains(contents, []byte("kind: Application")) {
			return nil
		}
		decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(contents), 4096)
		for {
			object := &unstructured.Unstructured{}
			decodeErr := decoder.Decode(object)
			if errors.Is(decodeErr, io.EOF) {
				break
			}
			if decodeErr != nil {
				return decodeErr
			}
			if object.GetKind() == "ApplicationSet" && bytes.Contains(contents, []byte(bootstrapPackagePath)) {
				t.Errorf("ApplicationSet watches bootstrap directory: %s", path)
			}
			if object.GetKind() != observabilityAppKind {
				continue
			}
			sourcePath, found, nestedErr := unstructured.NestedString(object.Object, "spec", "source", "path")
			if nestedErr != nil {
				return nestedErr
			}
			if found && sourcePath == bootstrapPackagePath {
				t.Errorf("Application watches bootstrap directory: %s", path)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan repository Argo consumers: %v", err)
	}
}

func assertFieldsAbsent(t *testing.T, object map[string]any, prefix []string, fields ...string) {
	t.Helper()
	for _, field := range fields {
		path := append(append([]string{}, prefix...), field)
		if _, found, err := unstructured.NestedFieldNoCopy(object, path...); err != nil || found {
			t.Errorf("field %s present or invalid: found %t, error %v", strings.Join(path, "."), found, err)
		}
	}
}

func bootstrapNestedMap(t *testing.T, object map[string]any, fields ...string) map[string]any {
	t.Helper()
	value, found, err := unstructured.NestedMap(object, fields...)
	if err != nil || !found {
		t.Fatalf("read %s: found %t, error %v", strings.Join(fields, "."), found, err)
	}
	return value
}

func nestedSlice(t *testing.T, object map[string]any, fields ...string) []any {
	t.Helper()
	value, found, err := unstructured.NestedSlice(object, fields...)
	if err != nil || !found {
		t.Fatalf("read %s: found %t, error %v", strings.Join(fields, "."), found, err)
	}
	return value
}

func nestedString(t *testing.T, object map[string]any, fields ...string) string {
	t.Helper()
	value, found, err := unstructured.NestedString(object, fields...)
	if err != nil || !found {
		t.Fatalf("read %s: found %t, error %v", strings.Join(fields, "."), found, err)
	}
	return value
}

func nestedStringSlice(t *testing.T, object map[string]any, fields ...string) []string {
	t.Helper()
	value, found, err := unstructured.NestedStringSlice(object, fields...)
	if err != nil || !found {
		t.Fatalf("read %s: found %t, error %v", strings.Join(fields, "."), found, err)
	}
	return value
}

func nestedInt64(t *testing.T, object map[string]any, fields ...string) int64 {
	t.Helper()
	value, found, err := unstructured.NestedInt64(object, fields...)
	if err != nil || !found {
		t.Fatalf("read %s: found %t, error %v", strings.Join(fields, "."), found, err)
	}
	return value
}
