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

package platformhosting

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/yaml"
)

const (
	argoNamespace        = "argocd"
	clusterServer        = "https://kubernetes.default.svc"
	restrictedPSA        = "restricted"
	smartEnergyNamespace = "smartenergy"
	v136                 = "v1.36"
)

type resourcePermission struct {
	Group string
	Kind  string
}

func TestSmartEnergyNamespaceAndQuota(t *testing.T) {
	namespace := decodeObject(t, filepath.Join(prerequisiteDirectory(), "namespace.yaml"))
	if namespace.GetAPIVersion() != "v1" || namespace.GetKind() != "Namespace" ||
		namespace.GetName() != smartEnergyNamespace || namespace.GetNamespace() != "" {
		t.Errorf("namespace identity = %s %s/%s %s", namespace.GetAPIVersion(), namespace.GetNamespace(), namespace.GetName(), namespace.GetKind())
	}

	labels := namespace.GetLabels()
	wantPSA := map[string]string{
		"pod-security.kubernetes.io/enforce":         restrictedPSA,
		"pod-security.kubernetes.io/enforce-version": v136,
		"pod-security.kubernetes.io/warn":            restrictedPSA,
		"pod-security.kubernetes.io/warn-version":    v136,
		"pod-security.kubernetes.io/audit":           restrictedPSA,
		"pod-security.kubernetes.io/audit-version":   v136,
	}
	for key, want := range wantPSA {
		if got := labels[key]; got != want {
			t.Errorf("namespace label %q = %q, want %q", key, got, want)
		}
	}

	quota := decodeObject(t, filepath.Join(prerequisiteDirectory(), "resource-quota.yaml"))
	if quota.GetAPIVersion() != "v1" || quota.GetKind() != "ResourceQuota" ||
		quota.GetName() != smartEnergyNamespace || quota.GetNamespace() != smartEnergyNamespace {
		t.Errorf("quota identity = %s %s/%s %s", quota.GetAPIVersion(), quota.GetNamespace(), quota.GetName(), quota.GetKind())
	}
	hard, found, err := unstructured.NestedStringMap(quota.Object, "spec", "hard")
	if err != nil || !found {
		t.Fatalf("quota spec.hard: found=%t error=%v", found, err)
	}
	wantHard := map[string]string{
		"requests.cpu":           "500m",
		"limits.cpu":             "1500m",
		"requests.memory":        "896Mi",
		"limits.memory":          "1536Mi",
		"pods":                   "12",
		"persistentvolumeclaims": "3",
		"requests.storage":       "12Gi",
	}
	if !reflect.DeepEqual(hard, wantHard) {
		t.Errorf("quota hard limits = %#v, want %#v", hard, wantHard)
	}
}

func TestSmartEnergyAppProjectIsRestricted(t *testing.T) {
	project := decodeObject(t, filepath.Join(repositoryRoot(), "deploy", "gitops", "bootstrap", "smartenergy-project.yaml"))
	if project.GetAPIVersion() != "argoproj.io/v1alpha1" || project.GetKind() != "AppProject" ||
		project.GetName() != smartEnergyNamespace || project.GetNamespace() != argoNamespace {
		t.Errorf("project identity = %s %s/%s %s", project.GetAPIVersion(), project.GetNamespace(), project.GetName(), project.GetKind())
	}

	sources := nestedStringSlice(t, project.Object, "spec", "sourceRepos")
	wantSources := []string{"https://github.com/etclank/smartenergy-api.git"}
	if !reflect.DeepEqual(sources, wantSources) {
		t.Errorf("project source repositories = %#v, want %#v", sources, wantSources)
	}
	destinations := nestedSlice(t, project.Object, "spec", "destinations")
	wantDestinations := []any{map[string]any{
		"namespace": smartEnergyNamespace,
		"server":    clusterServer,
	}}
	if !reflect.DeepEqual(destinations, wantDestinations) {
		t.Errorf("project destinations = %#v, want %#v", destinations, wantDestinations)
	}
	if _, found, err := unstructured.NestedSlice(project.Object, "spec", "clusterResourceWhitelist"); err != nil || found {
		t.Errorf("clusterResourceWhitelist present: found=%t error=%v", found, err)
	}

	wantPermissions := map[resourcePermission]struct{}{
		{Group: "", Kind: "ConfigMap"}:                      {},
		{Group: "", Kind: "Service"}:                        {},
		{Group: "apps", Kind: "Deployment"}:                 {},
		{Group: "apps", Kind: "StatefulSet"}:                {},
		{Group: "batch", Kind: "Job"}:                       {},
		{Group: "networking.k8s.io", Kind: "Ingress"}:       {},
		{Group: "networking.k8s.io", Kind: "NetworkPolicy"}: {},
		{Group: "cert-manager.io", Kind: "Certificate"}:     {},
		{Group: "traefik.io", Kind: "Middleware"}:           {},
	}
	gotPermissions := permissionSet(t, project.Object)
	if !reflect.DeepEqual(gotPermissions, wantPermissions) {
		t.Errorf("namespace permissions = %#v, want %#v", gotPermissions, wantPermissions)
	}
}

func TestSmartEnergyApplicationIsPinnedAndManual(t *testing.T) {
	application := decodeObject(t, filepath.Join(
		repositoryRoot(), "deploy", "gitops", "bootstrap", "smartenergy-application.yaml",
	))
	if application.GetAPIVersion() != "argoproj.io/v1alpha1" ||
		application.GetKind() != "Application" || application.GetName() != smartEnergyNamespace ||
		application.GetNamespace() != argoNamespace {
		t.Errorf("application identity = %s %s/%s %s", application.GetAPIVersion(), application.GetNamespace(), application.GetName(), application.GetKind())
	}

	spec := application.Object["spec"].(map[string]any)
	if got := nestedString(t, spec, "project"); got != smartEnergyNamespace {
		t.Errorf("application project = %q", got)
	}
	source := spec["source"].(map[string]any)
	if got := nestedString(t, source, "repoURL"); got != "https://github.com/etclank/smartenergy-api.git" {
		t.Errorf("application repository = %q", got)
	}
	if got := nestedString(t, source, "path"); got != "deploy/kubernetes/overlays/production" {
		t.Errorf("application path = %q", got)
	}
	revision := nestedString(t, source, "targetRevision")
	if revision != "6dee39d597ce87620f4f12b17ea0bdc079d029f0" || len(revision) != 40 {
		t.Errorf("application targetRevision = %q", revision)
	}
	for _, forbidden := range []string{"main", "HEAD", "ab4e3f0"} {
		if revision == forbidden {
			t.Errorf("application uses mutable or short revision %q", revision)
		}
	}

	destination := spec["destination"].(map[string]any)
	if got := nestedString(t, destination, "server"); got != clusterServer {
		t.Errorf("application destination server = %q", got)
	}
	if got := nestedString(t, destination, "namespace"); got != smartEnergyNamespace {
		t.Errorf("application destination namespace = %q", got)
	}
	syncPolicy := spec["syncPolicy"].(map[string]any)
	if _, automated := syncPolicy["automated"]; automated {
		t.Error("SmartEnergy Application enables automated synchronization")
	}
	options := syncPolicy["syncOptions"].([]any)
	if !reflect.DeepEqual(options, []any{"CreateNamespace=false"}) {
		t.Errorf("application sync options = %#v", options)
	}
}

func permissionSet(t *testing.T, object map[string]any) map[resourcePermission]struct{} {
	t.Helper()
	entries := nestedSlice(t, object, "spec", "namespaceResourceWhitelist")
	permissions := make(map[resourcePermission]struct{}, len(entries))
	for _, entry := range entries {
		mapping, ok := entry.(map[string]any)
		if !ok {
			t.Fatalf("permission entry has type %T", entry)
		}
		permission := resourcePermission{
			Group: nestedString(t, mapping, "group"),
			Kind:  nestedString(t, mapping, "kind"),
		}
		if permission.Group == "*" || permission.Kind == "*" {
			t.Errorf("wildcard permission: %#v", permission)
		}
		if _, duplicate := permissions[permission]; duplicate {
			t.Errorf("duplicate permission: %#v", permission)
		}
		permissions[permission] = struct{}{}
	}
	return permissions
}

func nestedString(t *testing.T, object map[string]any, fields ...string) string {
	t.Helper()
	value, found, err := unstructured.NestedString(object, fields...)
	if err != nil || !found {
		t.Fatalf("nested string %v: found=%t error=%v", fields, found, err)
	}
	return value
}

func nestedStringSlice(t *testing.T, object map[string]any, fields ...string) []string {
	t.Helper()
	value, found, err := unstructured.NestedStringSlice(object, fields...)
	if err != nil || !found {
		t.Fatalf("nested string slice %v: found=%t error=%v", fields, found, err)
	}
	return value
}

func nestedSlice(t *testing.T, object map[string]any, fields ...string) []any {
	t.Helper()
	value, found, err := unstructured.NestedSlice(object, fields...)
	if err != nil || !found {
		t.Fatalf("nested slice %v: found=%t error=%v", fields, found, err)
	}
	return value
}

func decodeObject(t *testing.T, path string) *unstructured.Unstructured {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read object %s: %v", path, err)
	}
	object := &unstructured.Unstructured{}
	decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(contents), 4096)
	if err := decoder.Decode(object); err != nil {
		t.Fatalf("decode object %s: %v", path, err)
	}
	return object
}

func prerequisiteDirectory() string {
	return filepath.Join(repositoryRoot(), "kubernetes", "platform", "smartenergy-prerequisites")
}

func repositoryRoot() string {
	return filepath.Join("..", "..")
}
