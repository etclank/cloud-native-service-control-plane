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
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

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

const demoHTTPImageRepository = "example.com/demo-http"

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
	By("building the demo-http OCI image for ManagedService tests")
	temporaryDirectory, err := os.MkdirTemp("", "demo-http-e2e-")
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	defer func() {
		ExpectWithOffset(1, os.RemoveAll(temporaryDirectory)).To(Succeed())
	}()

	archivePath := filepath.Join(temporaryDirectory, "demo-http.tar")
	taggedImage := demoHTTPImageRepository + ":e2e"
	cmd := exec.Command(
		"docker", "buildx", "build",
		"--platform", "linux/amd64",
		"--provenance=false",
		"--sbom=false",
		"--build-arg", "SOURCE_DATE_EPOCH=0",
		"--output", "type=oci,dest="+archivePath+",name="+taggedImage,
		"--file", "images/demo-http/Dockerfile",
		".",
	)
	_, err = utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to build demo-http OCI image")

	digest, err := imageDigestFromOCIArchive(archivePath)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	demoHTTPImage = demoHTTPImageRepository + "@" + digest

	By("loading the demo-http OCI image on Kind")
	kindCluster := os.Getenv("KIND_CLUSTER")
	if kindCluster == "" {
		kindCluster = "kind"
	}
	kindBinary := os.Getenv("KIND")
	if kindBinary == "" {
		kindBinary = "kind"
	}
	cmd = exec.Command(
		kindBinary, "load", "image-archive", archivePath,
		"--name", kindCluster,
	)
	_, err = utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to load demo-http OCI image")

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
			taggedImage, demoHTTPImage,
		)
		_, err = utils.Run(cmd)
		ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to register demo-http digest on %s", node)
	}
}

func imageDigestFromOCIArchive(archivePath string) (string, error) {
	archive, err := os.Open(archivePath)
	if err != nil {
		return "", fmt.Errorf("open OCI archive: %w", err)
	}
	defer func() {
		_ = archive.Close()
	}()

	reader := tar.NewReader(archive)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			return "", fmt.Errorf("OCI archive has no index.json")
		}
		if err != nil {
			return "", fmt.Errorf("read OCI archive: %w", err)
		}
		if header.Name != "index.json" {
			continue
		}

		index := struct {
			Manifests []struct {
				Digest   string `json:"digest"`
				Platform struct {
					Architecture string `json:"architecture"`
					OS           string `json:"os"`
				} `json:"platform"`
			} `json:"manifests"`
		}{}
		if err := json.NewDecoder(reader).Decode(&index); err != nil {
			return "", fmt.Errorf("decode OCI index: %w", err)
		}
		if len(index.Manifests) != 1 {
			return "", fmt.Errorf(
				"OCI index has %d manifests, want 1",
				len(index.Manifests),
			)
		}

		manifest := index.Manifests[0]
		if manifest.Platform.OS != "linux" ||
			manifest.Platform.Architecture != "amd64" {
			return "", fmt.Errorf(
				"OCI manifest platform is %s/%s, want linux/amd64",
				manifest.Platform.OS,
				manifest.Platform.Architecture,
			)
		}
		if !regexp.MustCompile(`^sha256:[0-9a-f]{64}$`).MatchString(
			manifest.Digest,
		) {
			return "", fmt.Errorf(
				"OCI manifest has invalid digest %q",
				manifest.Digest,
			)
		}

		return manifest.Digest, nil
	}
}
