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

package controlplaneapi

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const (
	controlPlaneAPIImage = "ghcr.io/etclank/cloud-native-service-control-plane-api"
	approvedGoBuilder    = "golang:1.26-bookworm@sha256:" +
		"1ecb7edf62a0408027bd5729dfd6b1b8766e578e8df93995b225dfd0944eb651"
	approvedAPIRuntime = "gcr.io/distroless/static-debian12:nonroot@sha256:" +
		"aef9602f8710ec12bde19d593fed1f76c708531bb7aba205110f1029786ead7b"
)

func TestControlPlaneAPIImagePackaging(t *testing.T) {
	workflow := readRepositoryFile(
		t,
		".github",
		"workflows",
		"control-plane-api-image.yml",
	)
	dockerfile := readRepositoryFile(t, "images", "control-plane-api", "Dockerfile")

	workflowRequirements := []string{
		"name: Control-plane API image",
		"IMAGE_NAME: " + controlPlaneAPIImage,
		"type=sha,format=long,prefix=sha-",
		"latest=false",
		"context: .",
		"file: images/control-plane-api/Dockerfile",
		"platforms: linux/amd64",
		"if: github.event_name != 'pull_request'",
		"push: ${{ github.event_name != 'pull_request' }}",
		"cache-from: type=gha,scope=control-plane-api",
		"cache-to: type=gha,mode=max,scope=control-plane-api",
		"sbom: true",
		"provenance: mode=max",
		"org.opencontainers.image.source=${{ github.server_url }}/${{ github.repository }}",
		"org.opencontainers.image.revision=${{ github.sha }}",
		"IMAGE_REFERENCE: ${{ env.IMAGE_NAME }}:sha-${{ github.sha }}",
	}
	for _, requirement := range workflowRequirements {
		if !strings.Contains(workflow, requirement) {
			t.Errorf("image workflow does not contain %q", requirement)
		}
	}

	for _, path := range []string{
		"cmd/control-plane-api/**",
		"internal/controlplaneapi/**",
		"api/**",
		"go.mod",
		"go.sum",
		"images/control-plane-api/Dockerfile",
		".github/workflows/control-plane-api-image.yml",
	} {
		if strings.Count(workflow, "- "+path) != 2 {
			t.Errorf("workflow path %q is not present in push and pull request triggers", path)
		}
	}

	if strings.Contains(workflow, ":latest") ||
		strings.Contains(workflow, "type=ref") ||
		strings.Contains(workflow, "format=short") {
		t.Error("image workflow contains a mutable or short-SHA tag configuration")
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

	dockerfileRequirements := []string{
		"FROM " + approvedGoBuilder + " AS builder",
		"FROM " + approvedAPIRuntime,
		"CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH}",
		"go build -trimpath -ldflags=\"-s -w\" -o /control-plane-api ./cmd/control-plane-api",
		"COPY --from=builder /control-plane-api /control-plane-api",
		"USER 65532:65532",
		"EXPOSE 8080",
		"ENTRYPOINT [\"/control-plane-api\"]",
	}
	for _, requirement := range dockerfileRequirements {
		if !strings.Contains(dockerfile, requirement) {
			t.Errorf("API Dockerfile does not contain %q", requirement)
		}
	}

	finalStage := dockerfile[strings.LastIndex(dockerfile, "FROM "):]
	if strings.Count(finalStage, "COPY ") != 1 || strings.Contains(finalStage, "ADD ") ||
		strings.Contains(strings.ToLower(finalStage), "token") ||
		strings.Contains(strings.ToLower(finalStage), "secret") {
		t.Error("API final image copies content beyond the built binary")
	}
}

func readRepositoryFile(t *testing.T, pathElements ...string) string {
	t.Helper()

	path := filepath.Join(append([]string{"..", ".."}, pathElements...)...)
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	return string(contents)
}
