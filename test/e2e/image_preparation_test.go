//go:build e2e
// +build e2e

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

package e2e

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDemoHTTPImageCommandsArePortable(t *testing.T) {
	temporaryDirectory := filepath.Join(
		string(filepath.Separator),
		"tmp",
		"demo-http-e2e-test",
	)
	paths := newDemoHTTPImagePaths(temporaryDirectory)
	for _, path := range []string{paths.metadataPath, paths.archivePath} {
		relativePath, err := filepath.Rel(paths.temporaryDirectory, path)
		if err != nil {
			t.Fatalf("resolve temporary path: %v", err)
		}
		if relativePath == ".." ||
			strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) {
			t.Errorf("path %q escapes temporary directory %q", path, temporaryDirectory)
		}
	}

	buildArguments := strings.Join(demoHTTPBuildCommand(paths).Args, " ")
	for _, required := range []string{
		"--platform linux/amd64",
		"--provenance=false",
		"--sbom=false",
		"--build-arg SOURCE_DATE_EPOCH=0",
		"--metadata-file " + paths.metadataPath,
		"--load",
		"--tag " + demoHTTPImageTag,
		"--file images/demo-http/Dockerfile",
	} {
		if !strings.Contains(buildArguments, required) {
			t.Errorf("build arguments omit %q: %s", required, buildArguments)
		}
	}
	for _, forbidden := range []string{"type=oci", "--output", "--push"} {
		if strings.Contains(buildArguments, forbidden) {
			t.Errorf("build arguments contain %q: %s", forbidden, buildArguments)
		}
	}

	saveArguments := strings.Join(demoHTTPSaveCommand(paths).Args, " ")
	if !strings.Contains(
		saveArguments,
		"docker image save --output "+paths.archivePath+" "+demoHTTPImageTag,
	) {
		t.Errorf("unexpected image save arguments: %s", saveArguments)
	}

	proofArguments := strings.Join(
		demoHTTPProofPodCommand(
			"example.com/demo-http@sha256:"+
				strings.Repeat("a", 64),
		).Args,
		" ",
	)
	if !strings.Contains(proofArguments, "--image-pull-policy=Never") {
		t.Errorf("proof Pod does not forbid registry pulls: %s", proofArguments)
	}
}

func TestImageDigestFromBuildMetadata(t *testing.T) {
	imageDigest := "sha256:" + strings.Repeat("a", 64)
	configDigest := "sha256:" + strings.Repeat("b", 64)
	validMetadata := buildMetadata{
		ConfigDigest: configDigest,
		Digest:       imageDigest,
	}
	validMetadata.Descriptor.MediaType =
		"application/vnd.docker.distribution.manifest.v2+json"
	validMetadata.Descriptor.Digest = imageDigest
	validMetadata.Descriptor.Platform.OS = "linux"
	validMetadata.Descriptor.Platform.Architecture = "amd64"

	tests := []struct {
		name     string
		mutate   func(*buildMetadata)
		contents []byte
		wantErr  string
	}{
		{name: "valid"},
		{name: "malformed JSON", contents: []byte("{"), wantErr: "decode Buildx metadata"},
		{
			name:    "missing image digest",
			mutate:  func(metadata *buildMetadata) { metadata.Digest = "" },
			wantErr: "invalid image digest",
		},
		{
			name: "non-sha256 image digest",
			mutate: func(metadata *buildMetadata) {
				metadata.Digest = "sha512:" + strings.Repeat("a", 64)
			},
			wantErr: "invalid image digest",
		},
		{
			name: "descriptor mismatch",
			mutate: func(metadata *buildMetadata) {
				metadata.Descriptor.Digest = "sha256:" + strings.Repeat("c", 64)
			},
			wantErr: "does not match image digest",
		},
		{
			name: "config digest used as image digest",
			mutate: func(metadata *buildMetadata) {
				metadata.ConfigDigest = imageDigest
			},
			wantErr: "equals its config digest",
		},
		{
			name: "wrong platform",
			mutate: func(metadata *buildMetadata) {
				metadata.Descriptor.Platform.Architecture = "arm64"
			},
			wantErr: "want linux/amd64",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			metadata := validMetadata
			if test.mutate != nil {
				test.mutate(&metadata)
			}
			contents := test.contents
			if contents == nil {
				var err error
				contents, err = json.Marshal(metadata)
				if err != nil {
					t.Fatalf("marshal metadata: %v", err)
				}
			}

			metadataPath := filepath.Join(t.TempDir(), "metadata.json")
			if err := os.WriteFile(metadataPath, contents, 0o600); err != nil {
				t.Fatalf("write metadata: %v", err)
			}
			digest, err := imageDigestFromBuildMetadata(metadataPath)
			if test.wantErr == "" {
				if err != nil {
					t.Fatalf("parse valid metadata: %v", err)
				}
				if digest != imageDigest {
					t.Errorf("image digest = %q, want %q", digest, imageDigest)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Errorf("metadata error = %v, want containing %q", err, test.wantErr)
			}
		})
	}

	t.Run("missing file", func(t *testing.T) {
		_, err := imageDigestFromBuildMetadata(
			filepath.Join(t.TempDir(), "missing.json"),
		)
		if err == nil || !strings.Contains(err.Error(), "read Buildx metadata") {
			t.Errorf("missing metadata error = %v", err)
		}
	})
}

func TestDemoImageCommandErrorIncludesStderr(t *testing.T) {
	command := exec.Command(
		"sh", "-c",
		"printf portable-demo-image-stderr >&2; exit 23",
	)
	err := runDemoImageCommand(command, "portable image test")
	if err == nil {
		t.Fatal("command unexpectedly succeeded")
	}
	for _, expected := range []string{
		"portable image test",
		"portable-demo-image-stderr",
	} {
		if !strings.Contains(err.Error(), expected) {
			t.Errorf("command error %q omits %q", err, expected)
		}
	}
}

func TestContainerdImageTargetMatches(t *testing.T) {
	image := "example.com/demo-http@sha256:" + strings.Repeat("a", 64)
	digest := "sha256:" + strings.Repeat("a", 64)
	output := "REF TYPE DIGEST SIZE PLATFORMS LABELS\n" +
		image + " application/vnd.docker.distribution.manifest.v2+json " +
		digest + " 6.2MiB linux/amd64 managed\n"

	if !containerdImageTargetMatches(output, image, digest) {
		t.Error("matching containerd image target was not found")
	}
	if containerdImageTargetMatches(
		output,
		image,
		"sha256:"+strings.Repeat("b", 64),
	) {
		t.Error("mismatched containerd image target was accepted")
	}
}
