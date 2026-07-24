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

	corev1 "k8s.io/api/core/v1"
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

func TestProofPodAcceptsHostedRuntimeAliasAfterStructuredEquivalence(
	t *testing.T,
) {
	requestedImage :=
		"example.com/demo-http@sha256:" +
			"279a4d1422d7c12ee85654fdfdf3e22cb123433fb120909ee88dc95f0fdfaf62"
	runtimeImageID :=
		"docker.io/library/import-2026-07-24@sha256:" +
			"cc73a299bb896948d5e790f77be315682cc80263bfd00accf60cb4d8d52eaeea"
	pod := validProofPod(requestedImage, runtimeImageID)
	status, err := validateProofPod(pod, requestedImage)
	if err != nil {
		t.Fatalf("validate hosted proof Pod fixture: %v", err)
	}
	if status.ImageID == requestedImage {
		t.Fatal("hosted runtime image ID unexpectedly equals requested image")
	}

	canonical := validCRIImageIdentity()
	reported := canonical
	reported.DiffIDs = append([]string(nil), canonical.DiffIDs...)
	if err := validateCRIImageEquivalence(canonical, reported); err != nil {
		t.Fatalf("equivalent CRI identities were rejected: %v", err)
	}
}

func TestProofPodRejectsUnprovenRuntimeIdentity(t *testing.T) {
	requestedImage :=
		demoHTTPImageRepository + "@sha256:" + strings.Repeat("a", 64)
	validRuntimeImageID :=
		"docker.io/library/import-2026-07-24@sha256:" +
			strings.Repeat("b", 64)

	t.Run("malformed runtime image ID", func(t *testing.T) {
		pod := validProofPod(requestedImage, "import-2026-07-24:latest")
		_, err := validateProofPod(pod, requestedImage)
		if err == nil || !strings.Contains(err.Error(), "malformed runtime image ID") {
			t.Errorf("runtime image ID error = %v", err)
		}
	})

	t.Run("mutable requested image", func(t *testing.T) {
		pod := validProofPod(requestedImage, validRuntimeImageID)
		_, err := validateProofPod(pod, demoHTTPImageTag)
		if err == nil || !strings.Contains(err.Error(), "canonical digest") {
			t.Errorf("mutable image error = %v", err)
		}
	})

	t.Run("unexpected restart", func(t *testing.T) {
		pod := validProofPod(requestedImage, validRuntimeImageID)
		pod.Status.ContainerStatuses[0].RestartCount = 1
		_, err := validateProofPod(pod, requestedImage)
		if err == nil || !strings.Contains(err.Error(), "restart count") {
			t.Errorf("restart error = %v", err)
		}
	})

	t.Run("not running", func(t *testing.T) {
		pod := validProofPod(requestedImage, validRuntimeImageID)
		pod.Status.ContainerStatuses[0].State.Running = nil
		_, err := validateProofPod(pod, requestedImage)
		if err == nil || !strings.Contains(err.Error(), "not running") {
			t.Errorf("running error = %v", err)
		}
	})

	t.Run("pull policy changed", func(t *testing.T) {
		pod := validProofPod(requestedImage, validRuntimeImageID)
		pod.Spec.Containers[0].ImagePullPolicy = corev1.PullIfNotPresent
		_, err := validateProofPod(pod, requestedImage)
		if err == nil || !strings.Contains(err.Error(), "pull policy") {
			t.Errorf("pull policy error = %v", err)
		}
	})
}

func TestCRIImageEquivalenceRejectsDifferentContent(t *testing.T) {
	canonical := validCRIImageIdentity()
	tests := []struct {
		name   string
		mutate func(*criImageIdentity)
	}{
		{
			name: "different config",
			mutate: func(identity *criImageIdentity) {
				identity.ID = "sha256:" + strings.Repeat("c", 64)
			},
		},
		{
			name: "different layer set",
			mutate: func(identity *criImageIdentity) {
				identity.DiffIDs = []string{
					"sha256:" + strings.Repeat("d", 64),
				}
			},
		},
		{
			name: "wrong platform",
			mutate: func(identity *criImageIdentity) {
				identity.Architecture = "arm64"
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reported := canonical
			reported.DiffIDs = append([]string(nil), canonical.DiffIDs...)
			test.mutate(&reported)
			if err := validateCRIImageEquivalence(
				canonical,
				reported,
			); err == nil {
				t.Fatal("different CRI content was accepted")
			}
		})
	}
}

func TestCRIImageInspectionRequiresStructuredIdentity(t *testing.T) {
	validInspection := criImageInspection{}
	valid := validCRIImageIdentity()
	validInspection.Status.ID = valid.ID
	validInspection.Info.ImageSpec.OS = valid.OS
	validInspection.Info.ImageSpec.Architecture = valid.Architecture
	validInspection.Info.ImageSpec.RootFS.DiffIDs = valid.DiffIDs
	if _, err := criImageIdentityFromJSON(
		mustJSON(t, validInspection),
	); err != nil {
		t.Fatalf("valid CRI inspection rejected: %v", err)
	}

	for _, test := range []struct {
		name   string
		mutate func(*criImageInspection)
	}{
		{
			name: "missing canonical identity",
			mutate: func(inspection *criImageInspection) {
				inspection.Status.ID = ""
			},
		},
		{
			name: "missing runtime root filesystem",
			mutate: func(inspection *criImageInspection) {
				inspection.Info.ImageSpec.RootFS.DiffIDs = nil
			},
		},
		{
			name: "malformed layer identity",
			mutate: func(inspection *criImageInspection) {
				inspection.Info.ImageSpec.RootFS.DiffIDs = []string{"sha256:abc"}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			inspection := validInspection
			inspection.Info.ImageSpec.RootFS.DiffIDs = append(
				[]string(nil),
				validInspection.Info.ImageSpec.RootFS.DiffIDs...,
			)
			test.mutate(&inspection)
			if _, err := criImageIdentityFromJSON(
				mustJSON(t, inspection),
			); err == nil {
				t.Fatal("incomplete CRI inspection was accepted")
			}
		})
	}
}

func TestProofPodWarningEventsAreRejected(t *testing.T) {
	for _, event := range []corev1.Event{
		{Reason: "Pulling", Type: corev1.EventTypeNormal},
		{Reason: "BackOff", Type: corev1.EventTypeWarning},
		{Reason: "Unexpected", Type: corev1.EventTypeWarning},
	} {
		events := &corev1.EventList{Items: []corev1.Event{event}}
		if err := validateProofPodEvents(events); err == nil {
			t.Errorf("event %#v was accepted", event)
		}
	}
	if err := validateProofPodEvents(&corev1.EventList{
		Items: []corev1.Event{{
			Reason: "Started",
			Type:   corev1.EventTypeNormal,
		}},
	}); err != nil {
		t.Errorf("normal startup event rejected: %v", err)
	}
}

func TestCRIImageCommandErrorIncludesStderr(t *testing.T) {
	command := exec.Command(
		"sh", "-c",
		"printf structured-cri-stderr >&2; exit 25",
	)
	_, err := runCRIImageInspectCommand(
		command,
		demoHTTPImageTag,
		"kind-node",
	)
	if err == nil || !strings.Contains(err.Error(), "structured-cri-stderr") {
		t.Errorf("CRI command error = %v", err)
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

func validProofPod(
	requestedImage string,
	runtimeImageID string,
) *corev1.Pod {
	started := true
	return &corev1.Pod{
		Spec: corev1.PodSpec{
			NodeName: "kind-node",
			Containers: []corev1.Container{{
				Name:            "demo-http-image-proof",
				Image:           requestedImage,
				ImagePullPolicy: corev1.PullNever,
			}},
		},
		Status: corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{{
				Name:         "demo-http-image-proof",
				Ready:        true,
				Started:      &started,
				RestartCount: 0,
				Image:        "docker.io/library/import-2026-07-24:latest",
				ImageID:      runtimeImageID,
				ContainerID:  "containerd://" + strings.Repeat("e", 64),
				State: corev1.ContainerState{
					Running: &corev1.ContainerStateRunning{},
				},
			}},
		},
	}
}

func validCRIImageIdentity() criImageIdentity {
	return criImageIdentity{
		ID:           "sha256:" + strings.Repeat("a", 64),
		OS:           "linux",
		Architecture: "amd64",
		DiffIDs: []string{
			"sha256:" + strings.Repeat("b", 64),
			"sha256:" + strings.Repeat("c", 64),
		},
	}
}
