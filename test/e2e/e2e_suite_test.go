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
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"

	"github.com/etclank/cloud-native-service-control-plane/test/utils"
)

var (
	// managerImage is the manager image to be built and loaded for testing.
	managerImage = "example.com/cloud-native-service-control-plane:v0.0.1"
	// shouldCleanupCertManager tracks whether CertManager was installed by this suite.
	shouldCleanupCertManager = false
	// demoHTTPImage is the immutable local image built and loaded for ManagedService tests.
	demoHTTPImage string
)

const (
	demoHTTPImageRepository = "example.com/demo-http"
	demoHTTPImageTag        = demoHTTPImageRepository + ":e2e"
	demoHTTPProofPodName    = "demo-http-image-proof"
)

var sha256DigestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

type demoHTTPImagePaths struct {
	temporaryDirectory string
	archivePath        string
}

type runtimeImageIdentity struct {
	TargetDigest    string
	TargetMediaType string
	ConfigDigest    string
	OS              string
	Architecture    string
}

type imageDescriptor struct {
	MediaType   string            `json:"mediaType"`
	Digest      string            `json:"digest"`
	Platform    *imagePlatform    `json:"platform,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

type imagePlatform struct {
	Architecture string `json:"architecture"`
	OS           string `json:"os"`
}

type imageIndex struct {
	MediaType string            `json:"mediaType"`
	Manifests []imageDescriptor `json:"manifests"`
}

type imageManifest struct {
	MediaType string            `json:"mediaType"`
	Config    imageDescriptor   `json:"config"`
	Layers    []imageDescriptor `json:"layers"`
}

type imageConfig struct {
	Architecture string `json:"architecture"`
	OS           string `json:"os"`
}

const (
	dockerManifestMediaType = "application/vnd.docker.distribution.manifest.v2+json"
	ociManifestMediaType    = "application/vnd.oci.image.manifest.v1+json"
	dockerIndexMediaType    = "application/vnd.docker.distribution.manifest.list.v2+json"
	ociIndexMediaType       = "application/vnd.oci.image.index.v1+json"
	dockerConfigMediaType   = "application/vnd.docker.container.image.v1+json"
	ociConfigMediaType      = "application/vnd.oci.image.config.v1+json"
)

// TestE2E runs the e2e test suite to validate the solution in an isolated environment.
// The default setup requires Kind and CertManager.
//
// To enable kubectl kuberc (use custom kubectl configurations), set: KUBECTL_KUBERC=true
// By default, kuberc is disabled to ensure consistent test behavior across different environments.
// To skip CertManager installation, set: CERT_MANAGER_INSTALL_SKIP=true
func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	_, _ = fmt.Fprintf(GinkgoWriter, "Starting cloud-native-service-control-plane e2e test suite\n")
	RunSpecs(t, "e2e suite")
}

var _ = BeforeSuite(func() {
	By("building the manager image")
	cmd := exec.Command("make", "docker-build", fmt.Sprintf("IMG=%s", managerImage))
	_, err := utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to build the manager image")

	// TODO(user): If you want to change the e2e test vendor from Kind,
	// ensure the image is built and available, then remove the following block.
	By("loading the manager image on Kind")
	err = utils.LoadImageToKindClusterWithName(managerImage)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to load the manager image into Kind")

	buildAndLoadDemoHTTPImage()

	configureKubectlKubeRC()
	setupCertManager()
})

var _ = AfterSuite(func() {
	teardownCertManager()
})

// Disable kubectl kuberc by default for test isolation.
// This prevents local kubectl configurations from affecting test behavior.
// To enable kuberc, set: KUBECTL_KUBERC=true
func configureKubectlKubeRC() {
	if os.Getenv("KUBECTL_KUBERC") != "true" {
		By("disabling kubectl kuberc for test isolation")
		err := os.Setenv("KUBECTL_KUBERC", "false")
		ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to disable kubectl kuberc")
		_, _ = fmt.Fprintf(GinkgoWriter,
			"kubectl kuberc disabled for consistent test behavior (override with KUBECTL_KUBERC=true)\n")
	} else {
		_, _ = fmt.Fprintf(GinkgoWriter, "kubectl kuberc enabled (KUBECTL_KUBERC=true)\n")
	}
}

// setupCertManager installs CertManager if needed for webhook tests.
// Skips installation if CERT_MANAGER_INSTALL_SKIP=true or if already present.
func setupCertManager() {
	if os.Getenv("CERT_MANAGER_INSTALL_SKIP") == "true" {
		_, _ = fmt.Fprintf(GinkgoWriter, "Skipping CertManager installation (CERT_MANAGER_INSTALL_SKIP=true)\n")
		return
	}

	By("checking if CertManager is already installed")
	if utils.IsCertManagerCRDsInstalled() {
		_, _ = fmt.Fprintf(GinkgoWriter, "CertManager is already installed. Skipping installation.\n")
		return
	}

	// Mark for cleanup before installation to handle interruptions and partial installs.
	shouldCleanupCertManager = true

	By("installing CertManager")
	Expect(utils.InstallCertManager()).To(Succeed(), "Failed to install CertManager")
}

// teardownCertManager uninstalls CertManager if it was installed by setupCertManager.
// This ensures we only remove what we installed.
func teardownCertManager() {
	if !shouldCleanupCertManager {
		_, _ = fmt.Fprintf(GinkgoWriter, "Skipping CertManager cleanup (not installed by this suite)\n")
		return
	}

	By("uninstalling CertManager")
	utils.UninstallCertManager()
}

func buildAndLoadDemoHTTPImage() {
	By("building and loading the demo-http image for ManagedService tests")
	temporaryDirectory, err := os.MkdirTemp("", "demo-http-e2e-")
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	defer func() {
		ExpectWithOffset(1, os.RemoveAll(temporaryDirectory)).To(Succeed())
	}()

	paths := newDemoHTTPImagePaths(temporaryDirectory)
	err = runDemoImageCommand(
		demoHTTPBuildCommand(paths),
		"build and load demo-http image",
	)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	By("exporting the loaded demo-http image as a Docker archive")
	err = runDemoImageCommand(
		demoHTTPSaveCommand(paths),
		"save demo-http image archive",
	)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	By("loading the demo-http Docker archive on Kind")
	kindCluster := os.Getenv("KIND_CLUSTER")
	if kindCluster == "" {
		kindCluster = "kind"
	}
	kindBinary := os.Getenv("KIND")
	if kindBinary == "" {
		kindBinary = "kind"
	}
	cmd := exec.Command(
		kindBinary, "load", "image-archive", paths.archivePath,
		"--name", kindCluster,
	)
	err = runDemoImageCommand(cmd, "load demo-http image archive into Kind")
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	By("registering the immutable demo-http identity on each Kind node")
	cmd = exec.Command(kindBinary, "get", "nodes", "--name", kindCluster)
	output, err := utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to list Kind nodes")
	nodes := utils.GetNonEmptyLines(output)
	ExpectWithOffset(1, nodes).NotTo(BeEmpty())
	var authoritativeIdentity runtimeImageIdentity
	for _, node := range nodes {
		identity, err := runtimeImageIdentityFromNode(node, demoHTTPImageTag)
		ExpectWithOffset(1, err).NotTo(HaveOccurred())
		if authoritativeIdentity.TargetDigest == "" {
			authoritativeIdentity = identity
			demoHTTPImage = demoHTTPImageRepository + "@" + identity.TargetDigest
		} else {
			ExpectWithOffset(1, identity).To(Equal(authoritativeIdentity))
		}

		cmd = exec.Command(
			"docker", "exec", node,
			"ctr", "-n", "k8s.io", "images", "tag",
			demoHTTPImageTag, demoHTTPImage,
		)
		err = runDemoImageCommand(
			cmd,
			"register demo-http digest on "+node,
		)
		ExpectWithOffset(1, err).NotTo(HaveOccurred())

		canonicalIdentity, err := runtimeImageIdentityFromNode(
			node,
			demoHTTPImage,
		)
		ExpectWithOffset(1, err).NotTo(HaveOccurred())
		ExpectWithOffset(1, canonicalIdentity).To(Equal(authoritativeIdentity))

		Eventually(func() error {
			cmd := exec.Command(
				"docker", "exec", node,
				"crictl", "inspecti", demoHTTPImage,
			)
			return runDemoImageCommand(
				cmd,
				"verify demo-http digest through the CRI on "+node,
			)
		}, 30*time.Second, time.Second).Should(Succeed())
	}

	verifyLoadedDemoHTTPImage()
}

func newDemoHTTPImagePaths(temporaryDirectory string) demoHTTPImagePaths {
	return demoHTTPImagePaths{
		temporaryDirectory: temporaryDirectory,
		archivePath:        filepath.Join(temporaryDirectory, "demo-http.tar"),
	}
}

func demoHTTPBuildCommand(paths demoHTTPImagePaths) *exec.Cmd {
	return exec.Command(
		"docker", "buildx", "build",
		"--platform", "linux/amd64",
		"--provenance=false",
		"--sbom=false",
		"--build-arg", "SOURCE_DATE_EPOCH=0",
		"--load",
		"--tag", demoHTTPImageTag,
		"--file", "images/demo-http/Dockerfile",
		".",
	)
}

func demoHTTPSaveCommand(paths demoHTTPImagePaths) *exec.Cmd {
	return exec.Command(
		"docker", "image", "save",
		"--output", paths.archivePath,
		demoHTTPImageTag,
	)
}

func runDemoImageCommand(cmd *exec.Cmd, action string) error {
	if _, err := utils.Run(cmd); err != nil {
		return fmt.Errorf("%s: %w", action, err)
	}

	return nil
}

func containerdImageArchiveCommand(node string, image string) *exec.Cmd {
	return exec.Command(
		"docker", "exec", node,
		"ctr", "-n", "k8s.io", "images", "export",
		"--local", "--skip-manifest-json", "-", image,
	)
}

func runtimeImageIdentityFromNode(
	node string,
	image string,
) (runtimeImageIdentity, error) {
	cmd := containerdImageArchiveCommand(node, image)
	archive, err := runContainerdImageExportCommand(cmd, image, node)
	if err != nil {
		return runtimeImageIdentity{}, err
	}

	return runtimeImageIdentityFromArchive(archive, image)
}

func runContainerdImageExportCommand(
	cmd *exec.Cmd,
	image string,
	node string,
) ([]byte, error) {
	projectDirectory, err := utils.GetProjectDir()
	if err != nil {
		return nil, fmt.Errorf(
			"resolve project directory: %w",
			err,
		)
	}
	cmd.Dir = projectDirectory
	cmd.Env = append(os.Environ(), "GO111MODULE=on")
	_, _ = fmt.Fprintf(
		GinkgoWriter,
		"running structured containerd image export for %q on %q\n",
		image,
		node,
	)
	archive, err := cmd.Output()
	if err != nil {
		stderr := ""
		if exitError, ok := err.(*exec.ExitError); ok {
			stderr = string(exitError.Stderr)
		}
		return nil, fmt.Errorf(
			"export containerd image %q on %q: %s: %w",
			image,
			node,
			stderr,
			err,
		)
	}

	return archive, nil
}

func runtimeImageIdentityFromArchive(
	archive []byte,
	image string,
) (runtimeImageIdentity, error) {
	indexContents, blobs, err := readContainerdImageArchive(archive)
	if err != nil {
		return runtimeImageIdentity{}, err
	}

	index := &imageIndex{}
	if err := json.Unmarshal(indexContents, index); err != nil {
		return runtimeImageIdentity{}, fmt.Errorf(
			"decode containerd image index: %w",
			err,
		)
	}
	if index.MediaType != ociIndexMediaType {
		return runtimeImageIdentity{}, fmt.Errorf(
			"containerd export index media type %q is not %q",
			index.MediaType,
			ociIndexMediaType,
		)
	}

	var targets []imageDescriptor
	for _, descriptor := range index.Manifests {
		if descriptor.Annotations["io.containerd.image.name"] == image {
			targets = append(targets, descriptor)
		}
	}
	if len(targets) != 1 {
		return runtimeImageIdentity{}, fmt.Errorf(
			"containerd export has %d targets for %q, want 1",
			len(targets),
			image,
		)
	}

	return validateRuntimeImageTarget(targets[0], blobs)
}

func readContainerdImageArchive(
	archive []byte,
) ([]byte, map[string][]byte, error) {
	reader := tar.NewReader(bytes.NewReader(archive))
	var indexContents []byte
	blobs := make(map[string][]byte)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, fmt.Errorf(
				"read containerd image archive: %w",
				err,
			)
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}
		contents, err := io.ReadAll(reader)
		if err != nil {
			return nil, nil, fmt.Errorf(
				"read containerd archive entry %q: %w",
				header.Name,
				err,
			)
		}
		switch {
		case header.Name == "index.json":
			indexContents = contents
		case strings.HasPrefix(header.Name, "blobs/sha256/"):
			digest := "sha256:" + strings.TrimPrefix(
				header.Name,
				"blobs/sha256/",
			)
			blobs[digest] = contents
		}
	}
	if len(indexContents) == 0 {
		return nil, nil, fmt.Errorf(
			"containerd image archive has no index.json",
		)
	}

	return indexContents, blobs, nil
}

func validateRuntimeImageTarget(
	target imageDescriptor,
	blobs map[string][]byte,
) (runtimeImageIdentity, error) {
	if err := validateImageDigest("target", target.Digest); err != nil {
		return runtimeImageIdentity{}, err
	}

	manifestDescriptor := target
	switch target.MediaType {
	case dockerManifestMediaType, ociManifestMediaType:
	case dockerIndexMediaType, ociIndexMediaType:
		indexContents, ok := blobs[target.Digest]
		if !ok {
			return runtimeImageIdentity{}, fmt.Errorf(
				"containerd target content %q is unavailable",
				target.Digest,
			)
		}
		index := &imageIndex{}
		if err := json.Unmarshal(indexContents, index); err != nil {
			return runtimeImageIdentity{}, fmt.Errorf(
				"decode containerd target index: %w",
				err,
			)
		}
		var linuxAMD64 []imageDescriptor
		for _, descriptor := range index.Manifests {
			if descriptor.Platform != nil &&
				descriptor.Platform.OS == "linux" &&
				descriptor.Platform.Architecture == "amd64" {
				linuxAMD64 = append(linuxAMD64, descriptor)
			}
		}
		if len(linuxAMD64) != 1 {
			return runtimeImageIdentity{}, fmt.Errorf(
				"containerd target index has %d linux/amd64 manifests, want 1",
				len(linuxAMD64),
			)
		}
		manifestDescriptor = linuxAMD64[0]
	default:
		return runtimeImageIdentity{}, fmt.Errorf(
			"containerd target media type %q is not an image manifest or index",
			target.MediaType,
		)
	}

	if err := validateImageManifestMediaType(
		manifestDescriptor.MediaType,
	); err != nil {
		return runtimeImageIdentity{}, err
	}
	if err := validateImageDigest(
		"manifest",
		manifestDescriptor.Digest,
	); err != nil {
		return runtimeImageIdentity{}, err
	}
	manifestContents, ok := blobs[manifestDescriptor.Digest]
	if !ok {
		return runtimeImageIdentity{}, fmt.Errorf(
			"containerd manifest content %q is unavailable",
			manifestDescriptor.Digest,
		)
	}
	manifest := &imageManifest{}
	if err := json.Unmarshal(manifestContents, manifest); err != nil {
		return runtimeImageIdentity{}, fmt.Errorf(
			"decode containerd image manifest: %w",
			err,
		)
	}
	if manifest.MediaType != "" &&
		manifest.MediaType != manifestDescriptor.MediaType {
		return runtimeImageIdentity{}, fmt.Errorf(
			"containerd manifest media type %q does not match descriptor %q",
			manifest.MediaType,
			manifestDescriptor.MediaType,
		)
	}
	if manifest.Config.MediaType != dockerConfigMediaType &&
		manifest.Config.MediaType != ociConfigMediaType {
		return runtimeImageIdentity{}, fmt.Errorf(
			"containerd config media type %q is not an image config",
			manifest.Config.MediaType,
		)
	}
	if err := validateImageDigest("config", manifest.Config.Digest); err != nil {
		return runtimeImageIdentity{}, err
	}
	if target.Digest == manifest.Config.Digest ||
		manifestDescriptor.Digest == manifest.Config.Digest {
		return runtimeImageIdentity{}, fmt.Errorf(
			"containerd image target or manifest digest equals its config digest",
		)
	}
	configContents, ok := blobs[manifest.Config.Digest]
	if !ok {
		return runtimeImageIdentity{}, fmt.Errorf(
			"containerd config content %q is unavailable",
			manifest.Config.Digest,
		)
	}
	config := &imageConfig{}
	if err := json.Unmarshal(configContents, config); err != nil {
		return runtimeImageIdentity{}, fmt.Errorf(
			"decode containerd image config: %w",
			err,
		)
	}
	if config.OS != "linux" || config.Architecture != "amd64" {
		return runtimeImageIdentity{}, fmt.Errorf(
			"containerd image config platform is %s/%s, want linux/amd64",
			config.OS,
			config.Architecture,
		)
	}
	for _, layer := range manifest.Layers {
		if err := validateImageDigest("layer", layer.Digest); err != nil {
			return runtimeImageIdentity{}, err
		}
		if _, ok := blobs[layer.Digest]; !ok {
			return runtimeImageIdentity{}, fmt.Errorf(
				"containerd layer content %q is unavailable",
				layer.Digest,
			)
		}
	}

	return runtimeImageIdentity{
		TargetDigest:    target.Digest,
		TargetMediaType: target.MediaType,
		ConfigDigest:    manifest.Config.Digest,
		OS:              config.OS,
		Architecture:    config.Architecture,
	}, nil
}

func validateImageDigest(name string, digest string) error {
	if !sha256DigestPattern.MatchString(digest) {
		return fmt.Errorf(
			"containerd %s has invalid digest %q",
			name,
			digest,
		)
	}

	return nil
}

func validateImageManifestMediaType(mediaType string) error {
	if mediaType != dockerManifestMediaType &&
		mediaType != ociManifestMediaType {
		return fmt.Errorf(
			"containerd manifest media type %q is unsupported",
			mediaType,
		)
	}

	return nil
}

func verifyLoadedDemoHTTPImage() {
	By("verifying the digest-addressed demo-http image without registry pulls")
	for _, command := range [][]string{
		{
			"wait", "node", "--all",
			"--for=condition=Ready", "--timeout=1m",
		},
		{
			"rollout", "status", "daemonset/kindnet",
			"--namespace", "kube-system", "--timeout=1m",
		},
		{
			"rollout", "status", "deployment/coredns",
			"--namespace", "kube-system", "--timeout=1m",
		},
	} {
		cmd := exec.Command("kubectl", command...)
		_, err := utils.Run(cmd)
		ExpectWithOffset(1, err).NotTo(HaveOccurred())
	}

	cmd := exec.Command(
		"kubectl", "wait", "serviceaccount/default",
		"--namespace", "kube-system",
		"--for=create", "--timeout=1m",
	)
	_, err := utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	cmd = demoHTTPProofPodCommand(demoHTTPImage)
	_, err = utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to create demo-http image proof Pod")
	defer func() {
		cmd := exec.Command(
			"kubectl", "delete", "pod", demoHTTPProofPodName,
			"--namespace", "kube-system", "--ignore-not-found",
		)
		_, _ = utils.Run(cmd)
	}()

	cmd = exec.Command(
		"kubectl", "wait", "pod/"+demoHTTPProofPodName,
		"--namespace", "kube-system",
		"--for=condition=Ready", "--timeout=1m",
	)
	output, err := utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(
		HaveOccurred(),
		"Digest-addressed demo-http proof Pod did not become Ready: %s",
		output,
	)

	cmd = exec.Command(
		"kubectl", "get", "pod", demoHTTPProofPodName,
		"--namespace", "kube-system", "--output", "json",
	)
	output, err = utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	pod := &corev1.Pod{}
	ExpectWithOffset(1, json.Unmarshal([]byte(output), pod)).To(Succeed())
	ExpectWithOffset(1, pod.Spec.Containers).To(HaveLen(1))
	ExpectWithOffset(1, pod.Spec.Containers[0].Image).To(Equal(demoHTTPImage))
	ExpectWithOffset(1, pod.Spec.Containers[0].ImagePullPolicy).To(
		Equal(corev1.PullNever),
	)
	ExpectWithOffset(1, pod.Status.ContainerStatuses).To(HaveLen(1))
	ExpectWithOffset(1, pod.Status.ContainerStatuses[0].ImageID).To(
		ContainSubstring(strings.TrimPrefix(demoHTTPImage, demoHTTPImageRepository+"@")),
	)
	ExpectWithOffset(1, pod.Status.ContainerStatuses[0].RestartCount).To(BeZero())

	cmd = exec.Command(
		"kubectl", "get", "events",
		"--namespace", "kube-system",
		"--field-selector=involvedObject.kind=Pod,involvedObject.name="+demoHTTPProofPodName,
		"--output", "json",
	)
	output, err = utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	events := &corev1.EventList{}
	ExpectWithOffset(1, json.Unmarshal([]byte(output), events)).To(Succeed())
	for _, event := range events.Items {
		ExpectWithOffset(1, event.Reason).NotTo(Equal("Pulling"))
		ExpectWithOffset(1, event.Type).NotTo(Equal(corev1.EventTypeWarning))
	}
}

func demoHTTPProofPodCommand(image string) *exec.Cmd {
	return exec.Command(
		"kubectl", "run", demoHTTPProofPodName,
		"--namespace", "kube-system",
		"--restart=Never",
		"--image="+image,
		"--image-pull-policy=Never",
	)
}
