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
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const (
	operatorImageName = "ghcr.io/etclank/cloud-native-service-control-plane-operator"
	mainBranch        = "main"
)

func TestOperatorImageWorkflow(t *testing.T) {
	workflowPath := filepath.Join(
		"..",
		"..",
		".github",
		"workflows",
		"operator-image.yml",
	)
	workflowBytes, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatalf("read operator image workflow: %v", err)
	}
	workflow := string(workflowBytes)

	required := []string{
		"IMAGE_NAME: " + operatorImageName,
		"type=sha,format=long,prefix=sha-",
		"latest=false",
		"context: .",
		"file: Dockerfile",
	}
	for _, value := range required {
		if !strings.Contains(workflow, value) {
			t.Errorf("operator image workflow does not contain %q", value)
		}
	}

	for _, path := range []string{
		"cmd/main.go",
		"internal/controller/**",
		"internal/telemetry/**",
		"api/**",
		"go.mod",
		"go.sum",
		"Dockerfile",
		".github/workflows/operator-image.yml",
	} {
		if strings.Count(workflow, "- "+path) != 2 {
			t.Errorf("workflow path %q is not present in push and pull request triggers", path)
		}
	}

	pushStart := strings.Index(workflow, "  push:\n")
	pullRequestStart := strings.Index(workflow, "  pull_request:\n")
	if pushStart < 0 || pullRequestStart < 0 || pullRequestStart <= pushStart {
		t.Fatal("operator workflow push trigger could not be isolated")
	}
	pushTrigger := workflow[pushStart:pullRequestStart]
	for _, branch := range []string{
		mainBranch,
		"h7-platform-foundation",
		"h8-observability-foundation",
	} {
		if !strings.Contains(pushTrigger, "      - "+branch+"\n") {
			t.Errorf("operator image push trigger does not include branch %q", branch)
		}
	}

	if strings.Contains(workflow, ":latest") {
		t.Error("operator image workflow contains a mutable latest tag")
	}

	usesPattern := regexp.MustCompile(`(?m)^\s*uses:\s+\S+@([^\s]+)\s+#\s+v\S+$`)
	shaPattern := regexp.MustCompile(`^[0-9a-f]{40}$`)
	usesMatches := usesPattern.FindAllStringSubmatch(workflow, -1)
	if len(usesMatches) != 5 {
		t.Fatalf("immutable action reference count = %d, want 5", len(usesMatches))
	}
	for _, match := range usesMatches {
		if !shaPattern.MatchString(match[1]) {
			t.Errorf("action reference is not a full commit SHA: %q", match[1])
		}
	}
}
