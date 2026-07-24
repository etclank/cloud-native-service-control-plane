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
	"fmt"
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
	metadataPath       string
	archivePath        string
}

type buildMetadata struct {
	ConfigDigest string           `json:"containerimage.config.digest"`
	Digest       string           `json:"containerimage.digest"`
	Descriptor   *buildDescriptor `json:"containerimage.descriptor"`
}

type buildDescriptor struct {
	MediaType string        `json:"mediaType"`
	Digest    string        `json:"digest"`
	Platform  buildPlatform `json:"platform"`
}

type buildPlatform struct {
	Architecture string `json:"architecture"`
	OS           string `json:"os"`
}

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

	digest, err := imageDigestFromBuildMetadata(paths.metadataPath)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	demoHTTPImage = demoHTTPImageRepository + "@" + digest

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
	for _, node := range nodes {
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

		cmd = exec.Command(
			"docker", "exec", node,
			"ctr", "-n", "k8s.io", "images", "list",
			"name=="+demoHTTPImage,
		)
		output, err = utils.Run(cmd)
		ExpectWithOffset(1, err).NotTo(HaveOccurred())
		ExpectWithOffset(
			1,
			containerdImageTargetMatches(output, demoHTTPImage, digest),
		).To(BeTrue())

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
		metadataPath:       filepath.Join(temporaryDirectory, "metadata.json"),
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
		"--metadata-file", paths.metadataPath,
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

func containerdImageTargetMatches(
	output string,
	image string,
	digest string,
) bool {
	for line := range strings.SplitSeq(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 3 && fields[0] == image && fields[2] == digest {
			return true
		}
	}

	return false
}

func imageDigestFromBuildMetadata(metadataPath string) (string, error) {
	metadataContents, err := os.ReadFile(metadataPath)
	if err != nil {
		return "", fmt.Errorf("read Buildx metadata: %w", err)
	}

	metadata := &buildMetadata{}
	if err := json.Unmarshal(metadataContents, metadata); err != nil {
		return "", fmt.Errorf("decode Buildx metadata: %w", err)
	}

	if !sha256DigestPattern.MatchString(metadata.Digest) {
		return "", fmt.Errorf(
			"Buildx metadata has invalid image digest %q",
			metadata.Digest,
		)
	}
	if metadata.Descriptor != nil {
		if !sha256DigestPattern.MatchString(metadata.Descriptor.Digest) {
			return "", fmt.Errorf(
				"Buildx metadata has invalid descriptor digest %q",
				metadata.Descriptor.Digest,
			)
		}
		if metadata.Descriptor.Digest != metadata.Digest {
			return "", fmt.Errorf(
				"Buildx descriptor digest %q does not match image digest %q",
				metadata.Descriptor.Digest,
				metadata.Digest,
			)
		}
		if metadata.Descriptor.MediaType != "" &&
			metadata.Descriptor.MediaType !=
				"application/vnd.docker.distribution.manifest.v2+json" &&
			metadata.Descriptor.MediaType !=
				"application/vnd.oci.image.manifest.v1+json" {
			return "", fmt.Errorf(
				"Buildx descriptor media type %q is not an image manifest",
				metadata.Descriptor.MediaType,
			)
		}
		if metadata.Descriptor.Platform.OS != "" ||
			metadata.Descriptor.Platform.Architecture != "" {
			if metadata.Descriptor.Platform.OS != "linux" ||
				metadata.Descriptor.Platform.Architecture != "amd64" {
				return "", fmt.Errorf(
					"Buildx descriptor platform is %s/%s, want linux/amd64",
					metadata.Descriptor.Platform.OS,
					metadata.Descriptor.Platform.Architecture,
				)
			}
		}
	}
	if metadata.ConfigDigest != "" &&
		!sha256DigestPattern.MatchString(metadata.ConfigDigest) {
		return "", fmt.Errorf(
			"Buildx metadata has invalid config digest %q",
			metadata.ConfigDigest,
		)
	}
	if metadata.ConfigDigest == metadata.Digest {
		return "", fmt.Errorf(
			"Buildx image digest unexpectedly equals its config digest",
		)
	}

	return metadata.Digest, nil
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
