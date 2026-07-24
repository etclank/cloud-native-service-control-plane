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
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
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
	for _, path := range []string{paths.archivePath} {
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
		"--load",
		"--tag " + demoHTTPImageTag,
		"--file images/demo-http/Dockerfile",
	} {
		if !strings.Contains(buildArguments, required) {
			t.Errorf("build arguments omit %q: %s", required, buildArguments)
		}
	}
	for _, forbidden := range []string{
		"type=oci",
		"--metadata-file",
		"--output",
		"--push",
	} {
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

	containerdArguments := strings.Join(
		containerdImageArchiveCommand("kind-node", demoHTTPImageTag).Args,
		" ",
	)
	for _, required := range []string{
		"docker exec kind-node",
		"ctr -n k8s.io images export",
		"--local",
		"--skip-manifest-json",
		"- " + demoHTTPImageTag,
	} {
		if !strings.Contains(containerdArguments, required) {
			t.Errorf(
				"containerd export arguments omit %q: %s",
				required,
				containerdArguments,
			)
		}
	}
}

func TestRuntimeImageIdentityFromArchive(t *testing.T) {
	archive, expected := validRuntimeImageArchive(t, demoHTTPImageTag)
	identity, err := runtimeImageIdentityFromArchive(
		archive,
		demoHTTPImageTag,
	)
	if err != nil {
		t.Fatalf("parse valid containerd archive: %v", err)
	}
	if identity != expected {
		t.Errorf("runtime identity = %#v, want %#v", identity, expected)
	}

	canonicalImage := demoHTTPImageRepository + "@" + identity.TargetDigest
	canonicalArchive, canonicalExpected := validRuntimeImageArchive(
		t,
		canonicalImage,
	)
	canonicalIdentity, err := runtimeImageIdentityFromArchive(
		canonicalArchive,
		canonicalImage,
	)
	if err != nil {
		t.Fatalf("parse canonical containerd archive: %v", err)
	}
	if canonicalIdentity != canonicalExpected ||
		canonicalIdentity != identity {
		t.Errorf(
			"canonical identity = %#v, want %#v",
			canonicalIdentity,
			identity,
		)
	}
}

func TestHostedBuildxConfigIdentityCannotDefineWorkloadDigest(t *testing.T) {
	configIdentity := "sha256:" + strings.Repeat("f", 64)
	hostedMetadata := map[string]string{
		"containerimage.digest":        configIdentity,
		"containerimage.config.digest": configIdentity,
	}
	metadataContents := mustJSON(t, hostedMetadata)
	if !bytes.Contains(metadataContents, []byte(configIdentity)) {
		t.Fatal("hosted Buildx equality fixture does not contain its config identity")
	}

	archive, _ := validRuntimeImageArchive(t, demoHTTPImageTag)
	identity, err := runtimeImageIdentityFromArchive(
		archive,
		demoHTTPImageTag,
	)
	if err != nil {
		t.Fatalf("derive runtime identity: %v", err)
	}
	if identity.TargetDigest == hostedMetadata["containerimage.digest"] {
		t.Error("runtime identity was derived from Buildx config metadata")
	}
	buildArguments := strings.Join(
		demoHTTPBuildCommand(
			newDemoHTTPImagePaths(t.TempDir()),
		).Args,
		" ",
	)
	if strings.Contains(buildArguments, "--metadata-file") {
		t.Errorf("Buildx metadata remains in the runtime identity path: %s", buildArguments)
	}
}

func TestRuntimeImageIdentityRejectsMalformedStructuredOutput(t *testing.T) {
	t.Run("malformed tar", func(t *testing.T) {
		_, err := runtimeImageIdentityFromArchive(
			[]byte("not a tar archive"),
			demoHTTPImageTag,
		)
		if err == nil {
			t.Fatal("malformed containerd archive was accepted")
		}
	})

	t.Run("missing image target", func(t *testing.T) {
		archive, _ := validRuntimeImageArchive(t, "example.com/other:e2e")
		_, err := runtimeImageIdentityFromArchive(
			archive,
			demoHTTPImageTag,
		)
		if err == nil || !strings.Contains(err.Error(), "0 targets") {
			t.Errorf("missing target error = %v", err)
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

func TestContainerdExportCommandErrorIncludesStderr(t *testing.T) {
	command := exec.Command(
		"sh", "-c",
		"printf structured-containerd-stderr >&2; exit 24",
	)
	_, err := runContainerdImageExportCommand(
		command,
		demoHTTPImageTag,
		"kind-node",
	)
	if err == nil {
		t.Fatal("structured containerd export unexpectedly succeeded")
	}
	for _, expected := range []string{
		demoHTTPImageTag,
		"kind-node",
		"structured-containerd-stderr",
	} {
		if !strings.Contains(err.Error(), expected) {
			t.Errorf("containerd export error %q omits %q", err, expected)
		}
	}
}

func TestValidateRuntimeImageTarget(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*imageDescriptor, map[string][]byte, *imageManifest)
		wantErr string
	}{
		{
			name: "missing target digest",
			mutate: func(target *imageDescriptor, _ map[string][]byte, _ *imageManifest) {
				target.Digest = ""
			},
			wantErr: "target has invalid digest",
		},
		{
			name: "malformed target digest",
			mutate: func(target *imageDescriptor, _ map[string][]byte, _ *imageManifest) {
				target.Digest = "sha256:abc"
			},
			wantErr: "target has invalid digest",
		},
		{
			name: "non-sha256 target digest",
			mutate: func(target *imageDescriptor, _ map[string][]byte, _ *imageManifest) {
				target.Digest = "sha512:" + strings.Repeat("a", 64)
			},
			wantErr: "target has invalid digest",
		},
		{
			name: "config media type as target",
			mutate: func(target *imageDescriptor, _ map[string][]byte, _ *imageManifest) {
				target.MediaType = dockerConfigMediaType
			},
			wantErr: "not an image manifest or index",
		},
		{
			name: "unsupported target media type",
			mutate: func(target *imageDescriptor, _ map[string][]byte, _ *imageManifest) {
				target.MediaType = "application/octet-stream"
			},
			wantErr: "not an image manifest or index",
		},
		{
			name: "target digest equals config digest",
			mutate: func(target *imageDescriptor, blobs map[string][]byte, manifest *imageManifest) {
				target.Digest = manifest.Config.Digest
				blobs[target.Digest] = mustJSON(t, manifest)
			},
			wantErr: "equals its config digest",
		},
		{
			name: "missing config descriptor",
			mutate: func(target *imageDescriptor, blobs map[string][]byte, manifest *imageManifest) {
				manifest.Config = imageDescriptor{}
				blobs[target.Digest] = mustJSON(t, manifest)
			},
			wantErr: "config media type",
		},
		{
			name: "malformed config digest",
			mutate: func(target *imageDescriptor, blobs map[string][]byte, manifest *imageManifest) {
				manifest.Config.Digest = "sha256:abc"
				blobs[target.Digest] = mustJSON(t, manifest)
			},
			wantErr: "config has invalid digest",
		},
		{
			name: "missing config content",
			mutate: func(_ *imageDescriptor, blobs map[string][]byte, manifest *imageManifest) {
				delete(blobs, manifest.Config.Digest)
			},
			wantErr: "config content",
		},
		{
			name: "wrong platform",
			mutate: func(_ *imageDescriptor, blobs map[string][]byte, manifest *imageManifest) {
				blobs[manifest.Config.Digest] = mustJSON(t, imageConfig{
					OS:           "linux",
					Architecture: "arm64",
				})
			},
			wantErr: "want linux/amd64",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			target, blobs, manifest := validRuntimeImageTarget(t)
			test.mutate(&target, blobs, &manifest)
			_, err := validateRuntimeImageTarget(target, blobs)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Errorf("target error = %v, want containing %q", err, test.wantErr)
			}
		})
	}
}

func TestValidateRuntimeImageIndexPlatformSelection(t *testing.T) {
	target, blobs, manifest := validRuntimeImageTarget(t)
	manifestDescriptor := target
	target = imageDescriptor{
		MediaType: ociIndexMediaType,
		Digest:    "sha256:" + strings.Repeat("d", 64),
	}

	t.Run("one linux amd64 manifest", func(t *testing.T) {
		index := imageIndex{
			MediaType: ociIndexMediaType,
			Manifests: []imageDescriptor{manifestDescriptor},
		}
		index.Manifests[0].Platform = &imagePlatform{
			OS:           "linux",
			Architecture: "amd64",
		}
		testBlobs := cloneBlobs(blobs)
		testBlobs[target.Digest] = mustJSON(t, index)
		identity, err := validateRuntimeImageTarget(target, testBlobs)
		if err != nil {
			t.Fatalf("validate indexed image: %v", err)
		}
		if identity.TargetDigest != target.Digest ||
			identity.ConfigDigest != manifest.Config.Digest {
			t.Errorf("indexed identity = %#v", identity)
		}
	})

	t.Run("absent linux amd64 manifest", func(t *testing.T) {
		index := imageIndex{
			MediaType: ociIndexMediaType,
			Manifests: []imageDescriptor{manifestDescriptor},
		}
		index.Manifests[0].Platform = &imagePlatform{
			OS:           "linux",
			Architecture: "arm64",
		}
		testBlobs := cloneBlobs(blobs)
		testBlobs[target.Digest] = mustJSON(t, index)
		_, err := validateRuntimeImageTarget(target, testBlobs)
		if err == nil || !strings.Contains(err.Error(), "0 linux/amd64") {
			t.Errorf("missing platform error = %v", err)
		}
	})

	t.Run("ambiguous linux amd64 manifests", func(t *testing.T) {
		manifestDescriptor.Platform = &imagePlatform{
			OS:           "linux",
			Architecture: "amd64",
		}
		index := imageIndex{
			MediaType: ociIndexMediaType,
			Manifests: []imageDescriptor{
				manifestDescriptor,
				manifestDescriptor,
			},
		}
		testBlobs := cloneBlobs(blobs)
		testBlobs[target.Digest] = mustJSON(t, index)
		_, err := validateRuntimeImageTarget(target, testBlobs)
		if err == nil || !strings.Contains(err.Error(), "2 linux/amd64") {
			t.Errorf("ambiguous platform error = %v", err)
		}
	})
}

func validRuntimeImageArchive(
	t *testing.T,
	image string,
) ([]byte, runtimeImageIdentity) {
	t.Helper()
	target, blobs, manifest := validRuntimeImageTarget(t)
	target.Annotations = map[string]string{
		"io.containerd.image.name": image,
	}
	index := imageIndex{
		MediaType: ociIndexMediaType,
		Manifests: []imageDescriptor{target},
	}

	var archive bytes.Buffer
	writer := tar.NewWriter(&archive)
	writeTarEntry(t, writer, "index.json", mustJSON(t, index))
	for digest, contents := range blobs {
		writeTarEntry(
			t,
			writer,
			"blobs/sha256/"+strings.TrimPrefix(digest, "sha256:"),
			contents,
		)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close runtime archive: %v", err)
	}

	return archive.Bytes(), runtimeImageIdentity{
		TargetDigest:    target.Digest,
		TargetMediaType: target.MediaType,
		ConfigDigest:    manifest.Config.Digest,
		OS:              "linux",
		Architecture:    "amd64",
	}
}

func validRuntimeImageTarget(
	t *testing.T,
) (imageDescriptor, map[string][]byte, imageManifest) {
	t.Helper()
	configContents := mustJSON(t, imageConfig{
		OS:           "linux",
		Architecture: "amd64",
	})
	configDigest := digestFor(configContents)
	layerContents := []byte("realistic compressed layer contents")
	layerDigest := digestFor(layerContents)
	manifest := imageManifest{
		MediaType: dockerManifestMediaType,
		Config: imageDescriptor{
			MediaType: dockerConfigMediaType,
			Digest:    configDigest,
		},
		Layers: []imageDescriptor{{
			MediaType: "application/vnd.docker.image.rootfs.diff.tar.gzip",
			Digest:    layerDigest,
		}},
	}
	manifestContents := mustJSON(t, manifest)
	manifestDigest := digestFor(manifestContents)
	return imageDescriptor{
			MediaType: dockerManifestMediaType,
			Digest:    manifestDigest,
		}, map[string][]byte{
			manifestDigest: manifestContents,
			configDigest:   configContents,
			layerDigest:    layerContents,
		}, manifest
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	contents, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}

	return contents
}

func digestFor(contents []byte) string {
	digest := sha256.Sum256(contents)
	return fmt.Sprintf("sha256:%x", digest)
}

func writeTarEntry(
	t *testing.T,
	writer *tar.Writer,
	name string,
	contents []byte,
) {
	t.Helper()
	if err := writer.WriteHeader(&tar.Header{
		Name: name,
		Mode: 0o600,
		Size: int64(len(contents)),
	}); err != nil {
		t.Fatalf("write tar header: %v", err)
	}
	if _, err := writer.Write(contents); err != nil {
		t.Fatalf("write tar contents: %v", err)
	}
}

func cloneBlobs(blobs map[string][]byte) map[string][]byte {
	clone := make(map[string][]byte, len(blobs))
	for digest, contents := range blobs {
		clone[digest] = contents
	}

	return clone
}
