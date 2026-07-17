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
	"strings"
	"testing"
)

const (
	approvedDemoHTTPImage = "ghcr.io/etclank/cloud-native-service-control-plane-demo-http@sha256:" +
		"2d1fc30e0cf75ba9fbe96af176f770524377ee5349acbee9ed94ae13f1143b2f"
	approvedImagePullSecret = "ghcr-pull"
)

func TestManagedServiceManagerConfiguration(t *testing.T) {
	configurationPath := filepath.Join(
		"..",
		"..",
		"config",
		"manager",
		"manager.yaml",
	)
	configuration, err := os.ReadFile(configurationPath)
	if err != nil {
		t.Fatalf("read manager configuration: %v", err)
	}

	arguments := managerArguments(string(configuration))
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

	for argument := range arguments {
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

func managerArguments(configuration string) map[string]int {
	arguments := make(map[string]int)
	for line := range strings.Lines(configuration) {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "- --") {
			arguments[strings.TrimPrefix(line, "- ")]++
		}
	}

	return arguments
}
