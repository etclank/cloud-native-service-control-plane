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

package demohttp

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestDemoHTTPImageWorkflow(t *testing.T) {
	workflowPath := filepath.Join(
		"..",
		"..",
		".github",
		"workflows",
		"demo-http-image.yml",
	)
	workflowBytes, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatalf("read demo HTTP image workflow: %v", err)
	}
	workflow := string(workflowBytes)

	for _, path := range []string{
		"cmd/demo-http/**",
		"internal/demohttp/**",
		"internal/telemetry/**",
		"go.mod",
		"go.sum",
		"images/demo-http/Dockerfile",
		".github/workflows/demo-http-image.yml",
	} {
		if strings.Count(workflow, "- "+path) != 2 {
			t.Errorf("workflow path %q is not present in push and pull request triggers", path)
		}
	}

	for _, safeguard := range []string{
		"type=sha,format=long,prefix=sha-",
		"if: github.event_name != 'pull_request'",
		"push: ${{ github.event_name != 'pull_request' }}",
	} {
		if !strings.Contains(workflow, safeguard) {
			t.Errorf("demo HTTP image workflow does not contain %q", safeguard)
		}
	}
	if strings.Contains(workflow, ":latest") ||
		strings.Contains(workflow, "type=ref") ||
		strings.Contains(workflow, "format=short") {
		t.Error("demo HTTP image workflow contains a mutable tag configuration")
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
